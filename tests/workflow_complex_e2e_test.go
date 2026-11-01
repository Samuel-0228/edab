package tests

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"

	"kerob/internal/core/conditions"
	"kerob/internal/core/credentials"
	"kerob/internal/core/event"
	"kerob/internal/core/workflow"
)

type complexSpyExecutor struct {
	mu        sync.Mutex
	name      string
	callCount map[string]int
	failOn    map[string]int // stepID -> fail if callCount <= failOn
}

func newComplexSpy(name string) *complexSpyExecutor {
	return &complexSpyExecutor{
		name:      name,
		callCount: make(map[string]int),
		failOn:    make(map[string]int),
	}
}

func (s *complexSpyExecutor) Name() string                          { return s.name }
func (s *complexSpyExecutor) Type() string                          { return s.name }
func (s *complexSpyExecutor) Validate(config json.RawMessage) error { return nil }

func (s *complexSpyExecutor) Execute(ctx context.Context, req workflow.ActionRequest) (workflow.ActionResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.callCount[req.ActionID]++
	if s.callCount[req.ActionID] <= s.failOn[req.ActionID] {
		return workflow.ActionResult{Success: false}, errors.New("simulated intermittent step failure")
	}

	out := map[string]any{
		"processed_step": req.ActionID,
		"status":         "ok",
	}
	outBytes, _ := json.Marshal(out)
	return workflow.ActionResult{
		Success:    true,
		ExternalID: "ext-" + req.ActionID,
		Output:     outBytes,
	}, nil
}

func (s *complexSpyExecutor) Count(stepID string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.callCount[stepID]
}

func TestE2E_ComplexPipeline_Branch_ForEach_RetryResume_And_DLQ(t *testing.T) {
	ctx := context.Background()

	wfStore := workflow.NewMemoryStore()
	execStore := workflow.NewMemoryExecutionStore()
	dlStore := workflow.NewMemoryDeadLetterStore()
	credStore := credentials.NewMemoryStore(credentials.DeriveKey("complex-e2e-key-32-bytes-pad!!"))

	registry := workflow.NewActionRegistry()
	spy := newComplexSpy("custom_service")
	// Make step_sync fail on the first attempt
	spy.failOn["step_sync"] = 1
	registry.Register(spy)

	// Define complex multi-step workflow:
	// Step 1: Normal action (step_init)
	// Step 2: Branch step (step_branch) -> evaluates tier == 'premium'
	// Step 3: ForEach step (step_foreach) -> loops over payload.users
	// Step 4: Step that fails on attempt 1 (step_sync)
	// Step 5: Final audit action (step_audit)
	wf, err := wfStore.Create(ctx, &workflow.Workflow{
		Name:       "Complex Pipeline E2E",
		Enabled:    true,
		MaxRetries: 2,
		Trigger: workflow.TriggerConfig{
			Type:       "webhook",
			EventTypes: []string{"account.provision"},
		},
		Actions: []workflow.ActionStep{
			{
				ID:          "step_init",
				Integration: "custom_service",
				ActionType:  "init",
				Config:      json.RawMessage(`{"account_id":"{{payload.account_id}}"}`),
			},
			{
				ID:   "step_branch",
				Type: workflow.StepTypeBranch,
				Cases: []workflow.BranchCase{
					{
						When: &conditions.Condition{
							Field:    "payload.tier",
							Operator: "equals",
							Value:    "premium",
						},
						Steps: []workflow.ActionStep{
							{
								ID:          "step_premium_notify",
								Integration: "custom_service",
								ActionType:  "notify_vip",
								Config:      json.RawMessage(`{"tier":"premium"}`),
							},
						},
					},
				},
				Default: []workflow.ActionStep{
					{
						ID:          "step_standard_notify",
						Integration: "custom_service",
						ActionType:  "notify_std",
					},
				},
			},
			{
				ID:            "step_foreach",
				Type:          workflow.StepTypeForEach,
				Items:         "payload.users",
				ItemVar:       "usr",
				IndexVar:      "idx",
				MaxIterations: 10,
				Steps: []workflow.ActionStep{
					{
						ID:          "step_user_provision",
						Integration: "custom_service",
						ActionType:  "create_user",
						Config:      json.RawMessage(`{"email":"{{usr.email}}"}`),
					},
				},
			},
			{
				ID:          "step_sync",
				Integration: "custom_service",
				ActionType:  "sync_remote",
				Config:      json.RawMessage(`{"init_status":"{{steps.step_init.status}}"}`),
			},
			{
				ID:          "step_audit",
				Integration: "custom_service",
				ActionType:  "audit_log",
			},
		},
	})
	if err != nil {
		t.Fatalf("failed to create workflow: %v", err)
	}

	engine := workflow.NewEngine(wfStore, execStore, dlStore, credStore, registry)

	// Step A: Trigger event with premium tier and 2 users
	ev, _ := event.New("src-1", "evt-prov-1", "account.provision", map[string]any{
		"account_id": "acc-999",
		"tier":       "premium",
		"users": []map[string]any{
			{"email": "alice@example.com"},
			{"email": "bob@example.com"},
		},
	}, nil)

	execs, err := engine.ProcessEvent(ctx, ev)
	if err != nil {
		t.Fatalf("unexpected process error: %v", err)
	}
	if len(execs) != 1 {
		t.Fatalf("expected 1 execution, got %d", len(execs))
	}
	firstExec := execs[0]
	if firstExec.Status != workflow.ExecutionRetrying {
		t.Fatalf("expected first execution to be scheduled for retry (retrying), got: %s", firstExec.Status)
	}

	// Verify step call counts on attempt 1
	if spy.Count("step_init") != 1 {
		t.Errorf("expected step_init called 1 time, got %d", spy.Count("step_init"))
	}
	if spy.Count("step_premium_notify") != 1 {
		t.Errorf("expected step_premium_notify called 1 time, got %d", spy.Count("step_premium_notify"))
	}
	if spy.Count("step_standard_notify") != 0 {
		t.Errorf("expected step_standard_notify NOT called, got %d", spy.Count("step_standard_notify"))
	}
	if spy.Count("step_foreach_0_step_user_provision") != 1 {
		t.Errorf("expected user 0 provisioned 1 time, got %d", spy.Count("step_foreach_0_step_user_provision"))
	}
	if spy.Count("step_foreach_1_step_user_provision") != 1 {
		t.Errorf("expected user 1 provisioned 1 time, got %d", spy.Count("step_foreach_1_step_user_provision"))
	}
	if spy.Count("step_sync") != 1 {
		t.Errorf("expected step_sync called 1 time (failed), got %d", spy.Count("step_sync"))
	}
	if spy.Count("step_audit") != 0 {
		t.Errorf("expected step_audit NOT executed due to prior failure, got %d", spy.Count("step_audit"))
	}

	// Step B: Retry resuming at failed step (strict idempotency check)
	retriedExec, err := engine.ExecuteWorkflow(ctx, wf, ev, firstExec)
	if err != nil {
		t.Fatalf("unexpected error during retry: %v", err)
	}
	if retriedExec.Status != workflow.ExecutionSuccess {
		t.Fatalf("expected retry execution to succeed, got: %s", retriedExec.Status)
	}

	// IDEMPOTENCY VERIFICATION: steps that completed on attempt 1 MUST NOT be re-executed!
	if spy.Count("step_init") != 1 {
		t.Errorf("IDEMPOTENCY VIOLATION: step_init was re-executed! Expected 1, got %d", spy.Count("step_init"))
	}
	if spy.Count("step_premium_notify") != 1 {
		t.Errorf("IDEMPOTENCY VIOLATION: step_premium_notify was re-executed! Expected 1, got %d", spy.Count("step_premium_notify"))
	}
	if spy.Count("step_foreach_0_step_user_provision") != 1 || spy.Count("step_foreach_1_step_user_provision") != 1 {
		t.Errorf("IDEMPOTENCY VIOLATION: step_user_provision re-executed on retry! Expected 1 each")
	}
	// step_sync should now have executed 2 times (1 failed + 1 retry success)
	if spy.Count("step_sync") != 2 {
		t.Errorf("expected step_sync executed 2 times, got %d", spy.Count("step_sync"))
	}
	// step_audit should now have executed 1 time
	if spy.Count("step_audit") != 1 {
		t.Errorf("expected step_audit executed 1 time, got %d", spy.Count("step_audit"))
	}

	// Step C: Test DLQ Entry & Replay with Idempotency Bypass
	// Record an unrecoverable failure in Dead-Letter store
	errReason := "database network partition"
	dlEntry := &workflow.DeadLetterRecord{
		ExecutionID: "exec-dlq-target",
		WorkflowID:  wf.ID,
		EventID:     ev.ID,
		ActionID:    "step_sync",
		Error:       errReason,
	}
	createdDL, err := dlStore.Record(ctx, dlEntry)
	if err != nil {
		t.Fatalf("failed to record dead-letter entry: %v", err)
	}

	// Verify DLQ entry exists
	pendingDLs, _ := dlStore.List(ctx, false)
	if len(pendingDLs) != 1 || pendingDLs[0].ID != createdDL.ID {
		t.Fatalf("expected 1 active dead-letter entry, got: %+v", pendingDLs)
	}

	// Execute DLQ Replay: mark discarded/resolved and re-execute workflow with a new execution record
	err = dlStore.Discard(ctx, createdDL.ID)
	if err != nil {
		t.Fatalf("failed to discard DLQ record: %v", err)
	}

	replayExec, err := execStore.Create(ctx, &workflow.Execution{
		WorkflowID: wf.ID,
		EventID:    ev.ID,
		Status:     workflow.ExecutionPending,
	})
	if err != nil {
		t.Fatalf("failed to create replay execution: %v", err)
	}

	replayed, err := engine.ExecuteWorkflow(ctx, wf, ev, replayExec)
	if err != nil {
		t.Fatalf("DLQ replay execution failed: %v", err)
	}
	if replayed.Status != workflow.ExecutionSuccess {
		t.Fatalf("expected replay status success, got: %s", replayed.Status)
	}

	allDLs, _ := dlStore.List(ctx, true)
	if len(allDLs) != 1 || !allDLs[0].Discarded {
		t.Fatalf("expected DLQ entry marked discarded, got: %+v", allDLs)
	}
}

func TestBackwardCompatibility_V011_WorkflowJSON(t *testing.T) {
	ctx := context.Background()

	// Legacy Kerob v0.1.1 workflow JSON payload format (single action array, no multi-step fields)
	v011JSON := `{
		"id": "wf-v011-legacy-1",
		"name": "Legacy v0.1.1 GitHub Release to Slack",
		"description": "Workflow created under Kerob v0.1.1 specification",
		"enabled": true,
		"trigger": {
			"type": "webhook",
			"event_types": ["github.release"]
		},
		"conditions": {
			"field": "payload.action",
			"operator": "equals",
			"value": "published"
		},
		"transformations": {
			"type": "template",
			"template": "[Release {{payload.release.tag_name}}] {{payload.release.name}}"
		},
		"actions": [
			{
				"integration": "slack",
				"action_type": "post_message",
				"config": {
					"channel": "#announcements",
					"text": "{{payload.text}}"
				}
			}
		],
		"max_retries": 3,
		"version": 1
	}`

	var wf workflow.Workflow
	if err := json.Unmarshal([]byte(v011JSON), &wf); err != nil {
		t.Fatalf("failed to parse v0.1.1 workflow JSON: %v", err)
	}

	if wf.Name != "Legacy v0.1.1 GitHub Release to Slack" {
		t.Errorf("unexpected name: %s", wf.Name)
	}
	if len(wf.Actions) != 1 || wf.Actions[0].Integration != "slack" {
		t.Fatalf("expected 1 legacy slack action, got: %+v", wf.Actions)
	}

	wfStore := workflow.NewMemoryStore()
	execStore := workflow.NewMemoryExecutionStore()
	dlStore := workflow.NewMemoryDeadLetterStore()
	credStore := credentials.NewMemoryStore(credentials.DeriveKey("v011-test-key-32bytes-padding!!"))
	registry := workflow.NewActionRegistry()

	var receivedConfig map[string]any
	slackSpy := newComplexSpy("slack")
	registry.Register(slackSpy)

	_, err := wfStore.Create(ctx, &wf)
	if err != nil {
		t.Fatalf("failed to store legacy v0.1.1 workflow: %v", err)
	}

	engine := workflow.NewEngine(wfStore, execStore, dlStore, credStore, registry)

	// Emit matching release event
	ev, _ := event.New("src-gh", "evt-rel-9", "github.release", map[string]any{
		"action": "published",
		"release": map[string]any{
			"tag_name": "v0.1.1",
			"name":     "Initial General Availability",
		},
	}, nil)

	execs, err := engine.ProcessEvent(ctx, ev)
	if err != nil {
		t.Fatalf("failed to execute legacy workflow: %v", err)
	}
	if len(execs) != 1 || execs[0].Status != workflow.ExecutionSuccess {
		t.Fatalf("expected successful execution of legacy workflow, got: %+v", execs)
	}

	// Verify action was executed
	storedWF, err := wfStore.GetByID(ctx, wf.ID)
	if err != nil {
		t.Fatalf("failed to retrieve stored workflow: %v", err)
	}
	if slackSpy.Count(storedWF.Actions[0].ID) != 1 {
		t.Errorf("expected legacy single-action executed, got count: %d", slackSpy.Count(storedWF.Actions[0].ID))
	}
	_ = receivedConfig
}
