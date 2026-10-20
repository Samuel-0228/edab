package tests

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"kerob/internal/core/conditions"
	"kerob/internal/core/credentials"
	"kerob/internal/core/event"
	"kerob/internal/core/transformations"
	"kerob/internal/core/workflow"
)

type TestActionSpy struct {
	IntegrationType string
	Calls           []workflow.ActionRequest
	FailCount       int // fail first N calls
}

func (s *TestActionSpy) Name() string { return s.IntegrationType }
func (s *TestActionSpy) Type() string { return s.IntegrationType }
func (s *TestActionSpy) Validate(config json.RawMessage) error { return nil }
func (s *TestActionSpy) Execute(ctx context.Context, req workflow.ActionRequest) (workflow.ActionResult, error) {
	s.Calls = append(s.Calls, req)
	if len(s.Calls) <= s.FailCount {
		return workflow.ActionResult{Success: false}, context.DeadlineExceeded
	}
	return workflow.ActionResult{
		Success:    true,
		ExternalID: "ext-12345",
		Output:     json.RawMessage(`{"status":"success"}`),
	}, nil
}

func TestE2EWebhookToDiscordAutomation(t *testing.T) {
	ctx := context.Background()

	wfStore := workflow.NewMemoryStore()
	execStore := workflow.NewMemoryExecutionStore()
	dlStore := workflow.NewMemoryDeadLetterStore()
	credStore := credentials.NewMemoryStore(credentials.DeriveKey("e2e-secret-key"))

	// Create Discord Credential
	cred, err := credStore.Create(ctx, &credentials.Credential{
		Name:        "Discord Channel Credential",
		Integration: "discord",
	}, map[string]string{
		"webhook_url": "https://discord.com/api/webhooks/mock/test",
	})
	if err != nil {
		t.Fatalf("failed to create credential: %v", err)
	}

	// Register Action Spy
	actionRegistry := workflow.NewActionRegistry()
	discordSpy := &TestActionSpy{IntegrationType: "discord"}
	actionRegistry.Register(discordSpy)

	// Create Workflow: WHEN webhook IF severity == 'critical' THEN send discord message
	_, err = wfStore.Create(ctx, &workflow.Workflow{
		Name:    "Critical Inbound Alerts to Discord",
		Enabled: true,
		Trigger: workflow.TriggerConfig{
			Type:       "webhook",
			EventTypes: []string{"infra.alert"},
		},
		Conditions: &conditions.Condition{
			Field:    "payload.severity",
			Operator: conditions.OpEquals,
			Value:    "critical",
		},
		Transformations: &transformations.Transformation{
			Type: "mapping",
			FieldMaps: map[string]string{
				"alert_msg": "ALERT: Service {{payload.service}} is {{payload.status}}!",
			},
		},
		Actions: []workflow.ActionStep{
			{
				Integration:  "discord",
				ActionType:   "send_message",
				CredentialID: cred.ID,
				Config:       json.RawMessage(`{"content":"{{payload.alert_msg}}"}`),
			},
		},
	})
	if err != nil {
		t.Fatalf("failed to create workflow: %v", err)
	}

	engine := workflow.NewEngine(wfStore, execStore, dlStore, credStore, actionRegistry)

	// 1. Send Non-Matching Event (severity == 'info')
	infoEvent, _ := event.New("wh-source", "evt-info-1", "infra.alert", map[string]any{
		"service":  "auth-api",
		"status":   "restarted",
		"severity": "info",
	}, nil)

	infoExecs, err := engine.ProcessEvent(ctx, infoEvent)
	if err != nil {
		t.Fatalf("ProcessEvent failed: %v", err)
	}
	if len(infoExecs) != 1 {
		t.Fatalf("expected 1 execution, got %d", len(infoExecs))
	}
	if infoExecs[0].Status != workflow.ExecutionSuccess {
		t.Errorf("expected skipped execution to finish with success, got %s", infoExecs[0].Status)
	}
	if len(discordSpy.Calls) != 0 {
		t.Fatalf("expected action NOT called for non-matching severity, got %d calls", len(discordSpy.Calls))
	}

	// 2. Send Matching Event (severity == 'critical')
	critEvent, _ := event.New("wh-source", "evt-crit-1", "infra.alert", map[string]any{
		"service":  "database-cluster",
		"status":   "unreachable",
		"severity": "critical",
	}, nil)

	critExecs, err := engine.ProcessEvent(ctx, critEvent)
	if err != nil {
		t.Fatalf("ProcessEvent failed: %v", err)
	}
	if len(critExecs) != 1 {
		t.Fatalf("expected 1 execution, got %d", len(critExecs))
	}
	if critExecs[0].Status != workflow.ExecutionSuccess {
		t.Errorf("expected execution to succeed, got %s", critExecs[0].Status)
	}
	if len(discordSpy.Calls) != 1 {
		t.Fatalf("expected action called once, got %d calls", len(discordSpy.Calls))
	}

	// Verify action received injected decrypted credentials
	call := discordSpy.Calls[0]
	if call.Credentials["webhook_url"] != "https://discord.com/api/webhooks/mock/test" {
		t.Errorf("expected action to receive decrypted credential, got: %v", call.Credentials)
	}

	// Verify transformed payload received by action
	if call.Payload["alert_msg"] != "ALERT: Service database-cluster is unreachable!" {
		t.Errorf("unexpected transformed alert message: %v", call.Payload["alert_msg"])
	}

	// 3. Strict Idempotency: re-sending the same event produces no new action calls
	dupExecs, err := engine.ProcessEvent(ctx, critEvent)
	if err != nil {
		t.Fatalf("ProcessEvent for duplicate failed: %v", err)
	}
	if len(dupExecs) != 1 || dupExecs[0].ID != critExecs[0].ID {
		t.Fatal("expected duplicate event to return existing execution ID")
	}
	if len(discordSpy.Calls) != 1 {
		t.Fatalf("expected duplicate event NOT to trigger additional action calls, still got %d", len(discordSpy.Calls))
	}

	// 4. Verify granular execution steps recorded in database/store
	storedExec, err := execStore.GetByID(ctx, critExecs[0].ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if len(storedExec.Steps) != 4 {
		t.Fatalf("expected 4 steps (trigger, condition, transform, action), got %d", len(storedExec.Steps))
	}
	expectedStepTypes := []string{"trigger", "condition", "transformation", "action"}
	for i, step := range storedExec.Steps {
		if step.StepType != expectedStepTypes[i] {
			t.Errorf("step[%d] expected type %s, got %s", i, expectedStepTypes[i], step.StepType)
		}
		if step.Status != "success" {
			t.Errorf("step[%d] expected status 'success', got %s", i, step.Status)
		}
	}
}

func TestE2ERetryAndDeadLetterQueue(t *testing.T) {
	ctx := context.Background()

	wfStore := workflow.NewMemoryStore()
	execStore := workflow.NewMemoryExecutionStore()
	dlStore := workflow.NewMemoryDeadLetterStore()
	credStore := credentials.NewMemoryStore(credentials.DeriveKey("e2e-secret-key"))

	actionRegistry := workflow.NewActionRegistry()
	failingSpy := &TestActionSpy{IntegrationType: "telegram", FailCount: 999}
	actionRegistry.Register(failingSpy)

	wf, err := wfStore.Create(ctx, &workflow.Workflow{
		Name:       "Telegram Forwarder with 2 Retries",
		Enabled:    true,
		MaxRetries: 2,
		Trigger: workflow.TriggerConfig{
			Type: "telegram",
		},
		Actions: []workflow.ActionStep{
			{
				Integration: "telegram",
				ActionType:  "send_message",
				Config:      json.RawMessage(`{"chat_id":"-100123"}`),
			},
		},
	})
	if err != nil {
		t.Fatalf("failed to create workflow: %v", err)
	}

	engine := workflow.NewEngine(wfStore, execStore, dlStore, credStore, actionRegistry, workflow.EngineConfig{
		BaseBackoff: 10 * time.Millisecond,
		MaxBackoff:  50 * time.Millisecond,
	})

	ev, _ := event.New("tele-src", "msg-fail-test", "telegram.message", map[string]any{"text": "Forward me"}, nil)

	// Attempt 1 -> Status should be retrying
	execs, _ := engine.ProcessEvent(ctx, ev)
	if len(execs) != 1 {
		t.Fatalf("expected 1 execution")
	}
	if execs[0].Status != workflow.ExecutionRetrying {
		t.Errorf("expected ExecutionRetrying after 1st attempt, got %s", execs[0].Status)
	}

	// Attempt 2 (exhausts max attempts of 2) -> Status should be failed & recorded in dead-letter
	_, _ = engine.ExecuteWorkflow(ctx, wf, ev, execs[0])

	finalExec, _ := execStore.GetByID(ctx, execs[0].ID)
	if finalExec.Status != workflow.ExecutionFailed {
		t.Errorf("expected ExecutionFailed after retries exhausted, got %s", finalExec.Status)
	}

	// Verify Dead Letter record
	deadLetters, err := dlStore.List(ctx, false)
	if err != nil {
		t.Fatalf("dlStore.List failed: %v", err)
	}
	if len(deadLetters) != 1 {
		t.Fatalf("expected 1 record in dead-letter queue, got %d", len(deadLetters))
	}
	dl := deadLetters[0]
	if dl.WorkflowID != wf.ID || dl.ExecutionID != finalExec.ID {
		t.Errorf("dead-letter mismatch: %+v", dl)
	}
}
