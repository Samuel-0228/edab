package sdk

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestGoSDKClient(t *testing.T) {
	mux := http.NewServeMux()

	// Mock /api/v1/auth/me
	mux.HandleFunc("GET /api/v1/auth/me", func(w http.ResponseWriter, r *http.Request) {
		if auth := r.Header.Get("Authorization"); auth != "Bearer secret-token" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":"unauthorized"}`))
			return
		}
		if tenant := r.Header.Get("X-Tenant-ID"); tenant != "tenant_prod" {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"missing tenant header"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(Identity{
			UserID:     "usr_123",
			Email:      "operator@example.com",
			Name:       "Test Operator",
			TenantID:   "tenant_prod",
			TenantName: "Production Tenant",
			Role:       RoleAdmin,
			Scopes:     []string{"*"},
		})
	})

	// Mock /api/v1/workflows
	mux.HandleFunc("GET /api/v1/workflows", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]Workflow{
			{
				ID:      "wf_test_1",
				Name:    "Payment Pipeline",
				Enabled: true,
				Trigger: TriggerConfig{Type: "webhook"},
				Actions: []ActionStep{{ID: "step_1", Name: "Charge Card", Integration: "stripe"}},
			},
		})
	})

	// Mock /api/v1/workflows/wf_test_1
	mux.HandleFunc("GET /api/v1/workflows/wf_test_1", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(Workflow{
			ID:      "wf_test_1",
			Name:    "Payment Pipeline",
			Enabled: true,
		})
	})

	// Mock /api/v1/workflows (POST)
	mux.HandleFunc("POST /api/v1/workflows", func(w http.ResponseWriter, r *http.Request) {
		var input WorkflowCreateInput
		_ = json.NewDecoder(r.Body).Decode(&input)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(Workflow{
			ID:      "wf_new",
			Name:    input.Name,
			Enabled: true,
		})
	})

	// Mock /api/v1/executions/exec_1/compensate
	mux.HandleFunc("POST /api/v1/executions/exec_1/compensate", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(Execution{
			ID:     "exec_1",
			Status: ExecutionCompensated,
		})
	})

	// Mock /api/v1/approvals/app_1/approve
	mux.HandleFunc("POST /api/v1/approvals/app_1/approve", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(Approval{
			ID:     "app_1",
			Status: ApprovalApproved,
		})
	})

	// Mock /api/v1/approval-tokens/tok_123
	mux.HandleFunc("GET /api/v1/approval-tokens/tok_123", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":     "app_1",
			"status": "pending",
			"title":  "Transfer Approval",
		})
	})

	// Mock /api/v1/events/ev_1/replay
	mux.HandleFunc("POST /api/v1/events/ev_1/replay", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": "replayed",
			"count":  1,
		})
	})

	// Mock /api/v1/failures/dlq_1/retry
	mux.HandleFunc("POST /api/v1/failures/dlq_1/retry", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(Execution{
			ID:     "exec_retried",
			Status: ExecutionRunning,
		})
	})

	// Mock /api/v1/governance/policy
	mux.HandleFunc("GET /api/v1/governance/policy", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(GovernancePolicy{
			TenantID:        "tenant_test",
			EnforcementMode: "enforce",
		})
	})

	// Mock /api/v1/governance/audit/verify
	mux.HandleFunc("GET /api/v1/governance/audit/verify", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(AuditVerificationResult{
			Valid:        true,
			TenantID:     "tenant_test",
			TotalRecords: 5,
		})
	})

	// Mock /api/v1/governance/policy/validate
	mux.HandleFunc("POST /api/v1/governance/policy/validate", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(PolicyValidationResult{
			Valid:    true,
			TenantID: "tenant_test",
		})
	})

	ts := httptest.NewServer(mux)
	defer ts.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 1. Initialize client
	client, err := NewClient(
		ts.URL,
		WithToken("secret-token"),
		WithTenant("tenant_prod"),
	)
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}

	// 2. Test GetMe
	me, err := client.GetMe(ctx)
	if err != nil {
		t.Fatalf("GetMe failed: %v", err)
	}
	if me.UserID != "usr_123" || me.TenantID != "tenant_prod" {
		t.Fatalf("unexpected Identity: %+v", me)
	}

	// 3. Test ListWorkflows
	wfs, err := client.ListWorkflows(ctx)
	if err != nil {
		t.Fatalf("ListWorkflows failed: %v", err)
	}
	if len(wfs) != 1 || wfs[0].ID != "wf_test_1" {
		t.Fatalf("unexpected workflows: %+v", wfs)
	}

	// 4. Test GetWorkflow
	wf, err := client.GetWorkflow(ctx, "wf_test_1")
	if err != nil {
		t.Fatalf("GetWorkflow failed: %v", err)
	}
	if wf.Name != "Payment Pipeline" {
		t.Fatalf("unexpected workflow: %+v", wf)
	}

	// 5. Test CreateWorkflow
	created, err := client.CreateWorkflow(ctx, WorkflowCreateInput{Name: "New Pipeline"})
	if err != nil {
		t.Fatalf("CreateWorkflow failed: %v", err)
	}
	if created.ID != "wf_new" {
		t.Fatalf("unexpected created workflow: %+v", created)
	}

	// 6. Test CompensateExecution
	compensated, err := client.CompensateExecution(ctx, "exec_1", "Manual rollback")
	if err != nil {
		t.Fatalf("CompensateExecution failed: %v", err)
	}
	if compensated.Status != ExecutionCompensated {
		t.Fatalf("expected compensated status, got %s", compensated.Status)
	}

	// 7. Test Approve
	approved, err := client.Approve(ctx, "app_1", "Looks good")
	if err != nil {
		t.Fatalf("Approve failed: %v", err)
	}
	if approved.Status != ApprovalApproved {
		t.Fatalf("expected approved status, got %s", approved.Status)
	}

	// 8. Test GetApprovalByToken
	tokenPreview, err := client.GetApprovalByToken(ctx, "tok_123")
	if err != nil {
		t.Fatalf("GetApprovalByToken failed: %v", err)
	}
	if tokenPreview["id"] != "app_1" {
		t.Fatalf("unexpected token preview: %+v", tokenPreview)
	}

	// 9. Test ReplayEvent
	replayRes, err := client.ReplayEvent(ctx, "ev_1", true)
	if err != nil {
		t.Fatalf("ReplayEvent failed: %v", err)
	}
	if replayRes["status"] != "replayed" {
		t.Fatalf("unexpected replay result: %+v", replayRes)
	}

	// 10. Test RetryFailure
	retryExec, err := client.RetryFailure(ctx, "dlq_1")
	if err != nil {
		t.Fatalf("RetryFailure failed: %v", err)
	}
	if retryExec.ID != "exec_retried" {
		t.Fatalf("unexpected retry execution: %+v", retryExec)
	}

	// 11. Test Governance Policy and Audit Chain
	policy, err := client.GetGovernancePolicy(ctx)
	if err != nil {
		t.Fatalf("GetGovernancePolicy failed: %v", err)
	}
	if policy.TenantID != "tenant_test" {
		t.Fatalf("unexpected policy tenant: %s", policy.TenantID)
	}

	chainRes, err := client.VerifyAuditChain(ctx)
	if err != nil {
		t.Fatalf("VerifyAuditChain failed: %v", err)
	}
	if !chainRes.Valid {
		t.Fatalf("expected audit chain to be valid")
	}

	valRes, err := client.ValidateWorkflowPolicy(ctx, map[string]any{"name": "Candidate"})
	if err != nil {
		t.Fatalf("ValidateWorkflowPolicy failed: %v", err)
	}
	if !valRes.Valid {
		t.Fatalf("expected valid dry run result")
	}
}

func TestGoSDKErrorHandling(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/workflows/wf_missing", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"code":"NOT_FOUND","error":"workflow not found"}`))
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()

	client, err := NewClient(ts.URL)
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}

	_, err = client.GetWorkflow(context.Background(), "wf_missing")
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	var apiErr *APIError
	if ok := errors.As(err, &apiErr); !ok {
		t.Fatalf("expected *APIError, got %T: %v", err, err)
	}
	if apiErr.StatusCode != 404 || apiErr.Code != "NOT_FOUND" || apiErr.Message != "workflow not found" {
		t.Fatalf("unexpected APIError content: %+v", apiErr)
	}
}
