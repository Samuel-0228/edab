package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"kerob/internal/core/event"
)

type mockSagaActionExecutor struct {
	typeName     string
	mu           sync.Mutex
	executedList []string
	shouldFail   map[string]bool
}

func (m *mockSagaActionExecutor) Name() string                          { return "Mock Saga Executor" }
func (m *mockSagaActionExecutor) Type() string                          { return m.typeName }
func (m *mockSagaActionExecutor) Validate(config json.RawMessage) error { return nil }

func (m *mockSagaActionExecutor) Execute(ctx context.Context, req ActionRequest) (ActionResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.executedList = append(m.executedList, req.ActionID)

	if m.shouldFail[req.ActionID] {
		return ActionResult{Success: false}, fmt.Errorf("mock error for %s", req.ActionID)
	}
	outputBytes, _ := json.Marshal(map[string]any{"executed_step": req.ActionID, "timestamp": time.Now().Unix()})
	return ActionResult{
		Success: true,
		Output:  outputBytes,
	}, nil
}

func (m *mockSagaActionExecutor) GetExecuted() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]string, len(m.executedList))
	copy(out, m.executedList)
	return out
}

func setupSagaEngine() (*Engine, *mockSagaActionExecutor, Store, ExecutionStore, ApprovalStore) {
	wfStore := NewMemoryStore()
	execStore := NewMemoryExecutionStore()
	dlStore := NewMemoryDeadLetterStore()
	approvalStore := NewMemoryApprovalStore()

	actions := NewActionRegistry()
	mockExec := &mockSagaActionExecutor{
		typeName:   "test_service",
		shouldFail: make(map[string]bool),
	}
	actions.Register(mockExec)

	mockHTTP := &mockSagaActionExecutor{
		typeName:   "http",
		shouldFail: make(map[string]bool),
	}
	actions.Register(mockHTTP)

	engine := NewEngine(wfStore, execStore, dlStore, nil, actions)
	engine.SetApprovalStore(approvalStore)
	return engine, mockExec, wfStore, execStore, approvalStore
}

func TestSagaCompensationReverseOrder(t *testing.T) {
	ctx := context.Background()
	engine, mockExec, wfStore, execStore, _ := setupSagaEngine()

	// Workflow with Step 1 (charge card, compensable), Step 2 (provision, compensable), Step 3 (fail)
	wf := &Workflow{
		ID:      "wf_saga_1",
		Name:    "Order Checkout Saga",
		Enabled: true,
		Trigger: TriggerConfig{Type: "webhook"},
		Actions: []ActionStep{
			{
				ID:          "step_1_charge",
				Name:        "Charge Payment Card",
				Integration: "test_service",
				ActionType:  "charge",
				Config:      json.RawMessage(`{"amount": 100}`),
				Compensation: &ActionStep{
					ID:          "comp_step_1",
					Name:        "Refund Payment Card",
					Integration: "test_service",
					ActionType:  "refund",
					Config:      json.RawMessage(`{"amount": 100}`),
				},
			},
			{
				ID:          "step_2_provision",
				Name:        "Provision Server",
				Integration: "test_service",
				ActionType:  "provision",
				Config:      json.RawMessage(`{"server": "prod-1"}`),
				Compensation: &ActionStep{
					ID:          "comp_step_2",
					Name:        "Deprovision Server",
					Integration: "test_service",
					ActionType:  "deprovision",
					Config:      json.RawMessage(`{"server": "prod-1"}`),
				},
			},
			{
				ID:          "step_3_notify",
				Name:        "Send SMS Confirmation",
				Integration: "test_service",
				ActionType:  "sms",
				Config:      json.RawMessage(`{"phone": "123"}`),
			},
		},
		MaxRetries:     1,
		TimeoutSeconds: 5,
	}

	_, err := wfStore.Create(ctx, wf)
	if err != nil {
		t.Fatalf("failed to create workflow: %v", err)
	}

	// Make Step 3 fail
	mockExec.shouldFail["step_3_notify"] = true

	ev := &event.Event{
		ID:        "ev_checkout_1",
		SourceID:  "web",
		EventType: "order.placed",
		Payload:   json.RawMessage(`{"order_id": "ord_123"}`),
	}

	results, err := engine.ProcessEvent(ctx, ev)
	if err != nil {
		t.Fatalf("ProcessEvent failed: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 execution result, got %d", len(results))
	}

	exec := results[0]
	// Step 3 failed -> retries exhausted (0) -> compensation triggered automatically -> compensated!
	if exec.Status != ExecutionCompensated {
		t.Errorf("expected execution status %s, got %s", ExecutionCompensated, exec.Status)
	}

	// Verify order of executions:
	// step_1_charge -> step_2_provision -> step_3_notify (failed) -> comp_step_2 (deprovision first!) -> comp_step_1 (refund second!)
	executed := mockExec.GetExecuted()
	expectedOrder := []string{"step_1_charge", "step_2_provision", "step_3_notify", "compensate_step_2_provision", "compensate_step_1_charge"}

	if len(executed) != len(expectedOrder) {
		t.Fatalf("expected executions %v, got %v", expectedOrder, executed)
	}
	for i, exp := range expectedOrder {
		if executed[i] != exp {
			t.Errorf("execution index %d: expected %s, got %s", i, exp, executed[i])
		}
	}

	// Inspect execution steps history in store
	savedExec, err := execStore.GetByID(ctx, exec.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	var compSteps []ExecutionStep
	for _, s := range savedExec.Steps {
		if s.StepType == "compensation" {
			compSteps = append(compSteps, s)
		}
	}
	if len(compSteps) != 2 {
		t.Fatalf("expected 2 compensation step records, got %d", len(compSteps))
	}
	if compSteps[0].StepID != "compensate_step_2_provision" || compSteps[0].Status != "success" {
		t.Errorf("first compensation step mismatch: %+v", compSteps[0])
	}
	if compSteps[1].StepID != "compensate_step_1_charge" || compSteps[1].Status != "success" {
		t.Errorf("second compensation step mismatch: %+v", compSteps[1])
	}
}

func TestSagaCompensationFailure(t *testing.T) {
	ctx := context.Background()
	engine, mockExec, wfStore, _, _ := setupSagaEngine()

	wf := &Workflow{
		ID:      "wf_saga_fail",
		Name:    "Failing Compensation Saga",
		Enabled: true,
		Trigger: TriggerConfig{Type: "webhook"},
		Actions: []ActionStep{
			{
				ID:          "step_a",
				Name:        "Charge",
				Integration: "test_service",
				ActionType:  "charge",
				Compensation: &ActionStep{
					ID:          "comp_a",
					Name:        "Refund",
					Integration: "test_service",
					ActionType:  "refund",
				},
			},
			{
				ID:          "step_b",
				Name:        "Fail Step",
				Integration: "test_service",
				ActionType:  "fail",
			},
		},
		MaxRetries: 1,
	}

	_, _ = wfStore.Create(ctx, wf)
	mockExec.shouldFail["step_b"] = true
	// Make compensation step fail
	mockExec.shouldFail["compensate_step_a"] = true

	ev := &event.Event{
		ID:        "ev_fail_1",
		SourceID:  "web",
		EventType: "test",
		Payload:   json.RawMessage(`{}`),
	}

	results, _ := engine.ProcessEvent(ctx, ev)
	if len(results) != 1 {
		t.Fatalf("expected 1 execution")
	}

	exec := results[0]
	if exec.Status != ExecutionCompensationFailed {
		t.Errorf("expected execution status %s, got %s", ExecutionCompensationFailed, exec.Status)
	}
}

func TestHumanApprovalLifecycle(t *testing.T) {
	ctx := context.Background()
	engine, mockExec, wfStore, execStore, approvalStore := setupSagaEngine()

	wf := &Workflow{
		ID:      "wf_approval_1",
		Name:    "High Value Wire Transfer",
		Enabled: true,
		Trigger: TriggerConfig{Type: "webhook"},
		Actions: []ActionStep{
			{
				ID:          "step_1_validate",
				Name:        "Validate Balance",
				Integration: "test_service",
				ActionType:  "validate",
			},
			{
				ID:   "step_2_manager_approval",
				Type: StepTypeApproval,
				Name: "Require CFO Approval",
				Approval: &ApprovalConfig{
					Title:          "Approve $1,000,000 Wire Transfer",
					Description:    "Requires finance director or CFO authorization",
					ApproverRoles:  []string{"owner", "admin"},
					TimeoutSeconds: 3600,
					TimeoutAction:  "reject",
				},
			},
			{
				ID:          "step_3_execute_wire",
				Name:        "Dispatch Wire Payment",
				Integration: "test_service",
				ActionType:  "wire",
			},
		},
		MaxRetries: 1,
	}

	_, err := wfStore.Create(ctx, wf)
	if err != nil {
		t.Fatalf("Create workflow failed: %v", err)
	}

	ev := &event.Event{
		ID:        "ev_wire_1",
		SourceID:  "banking",
		EventType: "wire.requested",
		Payload:   json.RawMessage(`{"amount": 1000000}`),
	}

	// 1. Process Event -> Reaches approval step and enters durable waiting_approval state
	results, err := engine.ProcessEvent(ctx, ev)
	if err != nil {
		t.Fatalf("ProcessEvent failed: %v", err)
	}
	exec := results[0]
	if exec.Status != ExecutionWaitingApproval {
		t.Fatalf("expected status %s, got %s", ExecutionWaitingApproval, exec.Status)
	}

	// Verify only step 1 executed so far; step 3 has NOT run
	executed := mockExec.GetExecuted()
	if len(executed) != 1 || executed[0] != "step_1_validate" {
		t.Fatalf("expected only step 1 executed, got: %v", executed)
	}

	// 2. Inspect persisted Approval record
	app, err := approvalStore.GetByExecutionAndStep(ctx, exec.ID, "step_2_manager_approval")
	if err != nil {
		t.Fatalf("approval record not found: %v", err)
	}
	if app.Status != ApprovalPending {
		t.Errorf("expected approval status pending, got %s", app.Status)
	}
	if app.Token == "" || app.TokenHash == "" {
		t.Errorf("expected approval token and hash to be generated")
	}

	// Verify token hash lookup
	byHash, err := approvalStore.GetByTokenHash(ctx, app.TokenHash)
	if err != nil || byHash.ID != app.ID {
		t.Errorf("token hash lookup failed")
	}

	// Verify authorization permissions
	if !app.CanApprove("user_admin", []string{"admin"}) {
		t.Errorf("admin user should be authorized to approve")
	}
	if app.CanApprove("user_viewer", []string{"viewer"}) {
		t.Errorf("viewer should NOT be authorized to approve")
	}

	// 3. Human Decides: Approve
	decidedBy := "cfo@company.com"
	note := "Approved after review"
	updatedApp, err := approvalStore.UpdateDecision(ctx, app.ID, ApprovalApproved, decidedBy, note)
	if err != nil {
		t.Fatalf("UpdateDecision failed: %v", err)
	}
	if updatedApp.Status != ApprovalApproved {
		t.Fatalf("expected updated status approved, got %s", updatedApp.Status)
	}

	// 4. Resume Workflow Execution
	resumedExec, err := engine.ExecuteWorkflow(ctx, wf, ev, exec)
	if err != nil {
		t.Fatalf("Resume execution failed: %v", err)
	}
	if resumedExec.Status != ExecutionSuccess {
		t.Errorf("expected execution to succeed after approval, got %s", resumedExec.Status)
	}

	// Verify step 3 has now executed!
	executedAfter := mockExec.GetExecuted()
	if len(executedAfter) != 2 || executedAfter[1] != "step_3_execute_wire" {
		t.Errorf("expected step 3 to execute after approval, got: %v", executedAfter)
	}

	// Inspect execution steps history in store
	finalExec, _ := execStore.GetByID(ctx, exec.ID)
	var foundApprovedStep bool
	for _, st := range finalExec.Steps {
		if st.StepID == "step_2_manager_approval" && st.Status == "approved" {
			foundApprovedStep = true
			break
		}
	}
	if !foundApprovedStep {
		t.Errorf("expected step_2_manager_approval to be recorded as approved in execution steps")
	}
}

func TestHumanApprovalRejectionTriggersSaga(t *testing.T) {
	ctx := context.Background()
	engine, mockExec, wfStore, _, approvalStore := setupSagaEngine()

	wf := &Workflow{
		ID:      "wf_approval_reject_saga",
		Name:    "Approval Rejection Saga",
		Enabled: true,
		Trigger: TriggerConfig{Type: "webhook"},
		Actions: []ActionStep{
			{
				ID:          "step_1_reserve_funds",
				Name:        "Reserve Funds",
				Integration: "test_service",
				ActionType:  "reserve",
				Compensation: &ActionStep{
					ID:          "comp_release_funds",
					Name:        "Release Reserved Funds",
					Integration: "test_service",
					ActionType:  "release",
				},
			},
			{
				ID:   "step_2_approval",
				Type: StepTypeApproval,
				Name: "Manager Approval",
			},
			{
				ID:          "step_3_charge",
				Name:        "Commit Charge",
				Integration: "test_service",
				ActionType:  "commit",
			},
		},
		MaxRetries: 1,
	}

	_, _ = wfStore.Create(ctx, wf)

	ev := &event.Event{
		ID:        "ev_test_reject",
		SourceID:  "web",
		EventType: "payment",
		Payload:   json.RawMessage(`{}`),
	}

	results, _ := engine.ProcessEvent(ctx, ev)
	exec := results[0]
	if exec.Status != ExecutionWaitingApproval {
		t.Fatalf("expected status waiting_approval")
	}

	// Verify step 1 ran
	executed := mockExec.GetExecuted()
	if len(executed) != 1 || executed[0] != "step_1_reserve_funds" {
		t.Fatalf("expected step 1 to execute")
	}

	// Reject approval
	app, _ := approvalStore.GetByExecutionAndStep(ctx, exec.ID, "step_2_approval")
	_, err := approvalStore.UpdateDecision(ctx, app.ID, ApprovalRejected, "auditor", "Suspicious activity detected")
	if err != nil {
		t.Fatalf("UpdateDecision failed: %v", err)
	}

	// Resuming execution with rejection triggers Saga compensation!
	resumedExec, _ := engine.ExecuteWorkflow(ctx, wf, ev, exec)
	if resumedExec.Status != ExecutionCompensated {
		t.Errorf("expected execution status %s, got %s", ExecutionCompensated, resumedExec.Status)
	}

	// Verify compensation was executed
	allExecuted := mockExec.GetExecuted()
	expected := []string{"step_1_reserve_funds", "compensate_step_1_reserve_funds"}
	if len(allExecuted) != len(expected) || allExecuted[1] != expected[1] {
		t.Errorf("expected compensation %v, got %v", expected, allExecuted)
	}
}

func TestApprovalTimeoutAndEscalation(t *testing.T) {
	ctx := context.Background()
	engine, _, wfStore, _, approvalStore := setupSagaEngine()

	wf := &Workflow{
		ID:      "wf_esc_test",
		Name:    "Escalation Test",
		Enabled: true,
		Trigger: TriggerConfig{Type: "webhook"},
		Actions: []ActionStep{
			{
				ID:   "step_approval_esc",
				Type: StepTypeApproval,
				Name: "Escalating Approval",
				Approval: &ApprovalConfig{
					Title:           "Urgent Approval",
					ApproverRoles:   []string{"operator"},
					EscalationRoles: []string{"admin", "owner"},
					TimeoutSeconds:  1, // 1 second timeout
					TimeoutAction:   "escalate",
				},
			},
		},
		MaxRetries: 1,
	}

	_, _ = wfStore.Create(ctx, wf)
	ev := &event.Event{ID: "ev_esc", SourceID: "test", EventType: "test", Payload: json.RawMessage(`{}`)}
	results, _ := engine.ProcessEvent(ctx, ev)
	exec := results[0]

	app, _ := approvalStore.GetByExecutionAndStep(ctx, exec.ID, "step_approval_esc")

	// Set expiration in the past to simulate timeout
	app.ExpiresAt = time.Now().UTC().Add(-10 * time.Minute)
	_ = approvalStore.Delete(ctx, app.ID)
	_, _ = approvalStore.Create(ctx, app)

	// Check expired approvals
	processed, err := engine.CheckExpiredApprovals(ctx)
	if err != nil {
		t.Fatalf("CheckExpiredApprovals failed: %v", err)
	}
	if len(processed) != 1 {
		t.Fatalf("expected 1 processed approval, got %d", len(processed))
	}

	// Verify it escalated
	escalatedApp, _ := approvalStore.GetByID(ctx, app.ID)
	if escalatedApp.Status != ApprovalEscalated {
		t.Errorf("expected status %s, got %s", ApprovalEscalated, escalatedApp.Status)
	}

	// Verify escalation roles can now approve
	if !escalatedApp.CanApprove("admin_user", []string{"admin"}) {
		t.Errorf("escalation role 'admin' should now be authorized to approve")
	}
}
