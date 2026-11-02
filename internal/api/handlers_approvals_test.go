package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"kerob/internal/core/auth"
	"kerob/internal/core/event"
	"kerob/internal/core/workflow"
)

type mockApprovalActionExecutor struct{}

func (m *mockApprovalActionExecutor) Name() string                          { return "Mock Executor" }
func (m *mockApprovalActionExecutor) Type() string                          { return "mock" }
func (m *mockApprovalActionExecutor) Validate(config json.RawMessage) error { return nil }
func (m *mockApprovalActionExecutor) Execute(ctx context.Context, req workflow.ActionRequest) (workflow.ActionResult, error) {
	return workflow.ActionResult{
		Success: true,
		Output:  json.RawMessage(`{"status":"ok"}`),
	}, nil
}

func TestApprovalsAndCompensationAPI(t *testing.T) {
	_, _, cfg := setupTestServer()

	cfg.ActionReg.Register(&mockApprovalActionExecutor{})

	approvalStore := workflow.NewMemoryApprovalStore()
	if cfg.WorkflowEngine != nil {
		cfg.WorkflowEngine.SetApprovalStore(approvalStore)
	}

	handlers := NewHandlersFromConfig(Config{
		Workflows:      cfg.Workflows,
		Executions:     cfg.Executions,
		DeadLetters:    cfg.DeadLetters,
		WorkflowEngine: cfg.WorkflowEngine,
		ActionReg:      cfg.ActionReg,
		Creds:          cfg.Creds,
		WebhookStore:   cfg.WebhookStore,
		Events:         cfg.Events,
		Approvals:      approvalStore,
	})
	srv := NewServer("0", handlers)

	// Create a test workflow with human approval and compensation
	wf, err := cfg.Workflows.Create(context.Background(), &workflow.Workflow{
		ID:      "wf_api_saga",
		Name:    "Payment With Approval",
		Enabled: true,
		Trigger: workflow.TriggerConfig{Type: "webhook"},
		Actions: []workflow.ActionStep{
			{
				ID:          "step_charge",
				Name:        "Charge Payment",
				Integration: "mock",
				ActionType:  "charge",
				Compensation: &workflow.ActionStep{
					ID:          "step_refund",
					Name:        "Refund Payment",
					Integration: "mock",
					ActionType:  "refund",
				},
			},
			{
				ID:   "step_approval",
				Type: workflow.StepTypeApproval,
				Name: "Compliance Approval",
				Approval: &workflow.ApprovalConfig{
					Title:         "Approve Payment Release",
					ApproverRoles: []string{"admin", "owner"},
					TimeoutAction: "reject",
				},
			},
		},
		MaxRetries: 1,
	})
	if err != nil {
		t.Fatalf("failed to create workflow: %v", err)
	}

	ev := &event.Event{
		ID:        "ev_saga_api",
		SourceID:  "web",
		EventType: "checkout",
		Payload:   json.RawMessage(`{"amount": 500}`),
	}
	_, _, _ = cfg.Events.Create(context.Background(), ev)

	// Trigger workflow
	results, err := cfg.WorkflowEngine.ProcessEvent(context.Background(), ev)
	if err != nil {
		t.Fatalf("ProcessEvent failed: %v", err)
	}
	exec := results[0]
	if exec.Status != workflow.ExecutionWaitingApproval {
		t.Fatalf("expected status waiting_approval, got %s", exec.Status)
	}

	// 1. GET /api/v1/approvals (List Approvals)
	reqList := httptest.NewRequest(http.MethodGet, "/api/v1/approvals?status=pending", nil)
	recList := httptest.NewRecorder()
	srv.mux.ServeHTTP(recList, reqList)
	if recList.Code != http.StatusOK {
		t.Fatalf("List approvals returned %d: %s", recList.Code, recList.Body.String())
	}

	var approvals []*workflow.Approval
	_ = json.Unmarshal(recList.Body.Bytes(), &approvals)
	if len(approvals) != 1 {
		t.Fatalf("expected 1 approval, got %d", len(approvals))
	}
	app := approvals[0]
	if app.ExecutionID != exec.ID || app.StepID != "step_approval" {
		t.Fatalf("unexpected approval data: %+v", app)
	}

	// 2. GET /api/v1/approvals/{id}
	reqGet := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/approvals/%s", app.ID), nil)
	recGet := httptest.NewRecorder()
	srv.mux.ServeHTTP(recGet, reqGet)
	if recGet.Code != http.StatusOK {
		t.Fatalf("Get approval returned %d", recGet.Code)
	}

	// 3. GET /api/v1/approval-tokens/{token} (Public Token Preview)
	reqToken := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/approval-tokens/%s", app.Token), nil)
	recToken := httptest.NewRecorder()
	srv.mux.ServeHTTP(recToken, reqToken)
	if recToken.Code != http.StatusOK {
		t.Fatalf("Get approval by token returned %d: %s", recToken.Code, recToken.Body.String())
	}
	var preview map[string]any
	_ = json.Unmarshal(recToken.Body.Bytes(), &preview)
	if preview["id"] != app.ID || preview["status"] != "pending" {
		t.Fatalf("invalid token preview: %+v", preview)
	}

	// 4. POST /api/v1/approval-tokens/{token} (Token Decision: Approve)
	tokenDecision := map[string]any{
		"decision": "approve",
		"note":     "Approved via secure token",
		"approver": "director@company.com",
	}
	tokenBody, _ := json.Marshal(tokenDecision)
	reqTokenDecide := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/approval-tokens/%s", app.Token), bytes.NewReader(tokenBody))
	recTokenDecide := httptest.NewRecorder()
	srv.mux.ServeHTTP(recTokenDecide, reqTokenDecide)
	if recTokenDecide.Code != http.StatusOK {
		t.Fatalf("Decide by token returned %d: %s", recTokenDecide.Code, recTokenDecide.Body.String())
	}

	var decidedApp workflow.Approval
	_ = json.Unmarshal(recTokenDecide.Body.Bytes(), &decidedApp)
	if decidedApp.Status != workflow.ApprovalApproved {
		t.Fatalf("expected approved status, got %s", decidedApp.Status)
	}

	// 5. POST /api/v1/executions/{id}/compensate (Operator-Triggered Rollback)
	compensateBody := map[string]any{
		"reason": "Customer requested full order cancellation",
	}
	compBytes, _ := json.Marshal(compensateBody)
	reqComp := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/executions/%s/compensate", exec.ID), bytes.NewReader(compBytes))
	recComp := httptest.NewRecorder()
	srv.mux.ServeHTTP(recComp, reqComp)
	if recComp.Code != http.StatusOK {
		t.Fatalf("Compensate returned %d: %s", recComp.Code, recComp.Body.String())
	}

	var compExec workflow.Execution
	_ = json.Unmarshal(recComp.Body.Bytes(), &compExec)
	if compExec.Status != workflow.ExecutionCompensated {
		t.Fatalf("expected execution status %s, got %s", workflow.ExecutionCompensated, compExec.Status)
	}

	// 6. Test Authorized Approval and Rejection Endpoints
	// Create another approval
	app2, _ := approvalStore.Create(context.Background(), &workflow.Approval{
		ExecutionID:   exec.ID,
		StepID:        "step_approval_2",
		WorkflowID:    wf.ID,
		Title:         "Second Approval",
		ApproverRoles: []string{"admin"},
		Status:        workflow.ApprovalPending,
		ExpiresAt:     time.Now().UTC().Add(1 * time.Hour),
	})

	// Test Approve with context
	ctxWithAdmin := auth.WithIdentity(context.Background(), &auth.Identity{
		UserID: "usr_admin",
		Role:   auth.RoleAdmin,
		IsAuth: true,
	})
	reqApprove := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/approvals/%s/approve", app2.ID), bytes.NewReader([]byte(`{"note": "admin approved"}`)))
	reqApprove = reqApprove.WithContext(ctxWithAdmin)
	recApprove := httptest.NewRecorder()
	srv.mux.ServeHTTP(recApprove, reqApprove)
	if recApprove.Code != http.StatusOK {
		t.Fatalf("Approve returned %d: %s", recApprove.Code, recApprove.Body.String())
	}
}
