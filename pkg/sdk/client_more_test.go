package sdk

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClientMoreEndpoints(t *testing.T) {
	mux := http.NewServeMux()

	// Tenant routes
	mux.HandleFunc("GET /api/v1/tenants", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]Tenant{{ID: "t-1", Name: "Tenant 1"}})
	})
	mux.HandleFunc("GET /api/v1/tenants/t-1", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(Tenant{ID: "t-1", Name: "Tenant 1"})
	})
	mux.HandleFunc("POST /api/v1/tenants", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(Tenant{ID: "t-new", Name: "New Tenant"})
	})
	mux.HandleFunc("GET /api/v1/members", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]Member{{UserID: "u-1", Role: RoleAdmin}})
	})
	mux.HandleFunc("GET /api/v1/tokens", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]APIToken{{ID: "tok-1", Name: "Token 1"}})
	})
	mux.HandleFunc("POST /api/v1/tokens", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(APIToken{ID: "tok-new", Name: "Tok", Token: "sec-tok"})
	})
	mux.HandleFunc("DELETE /api/v1/tokens/tok-1", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	// Workflow CRUD & versions
	mux.HandleFunc("PUT /api/v1/workflows/wf-1", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(Workflow{ID: "wf-1", Name: "Updated"})
	})
	mux.HandleFunc("PATCH /api/v1/workflows/wf-1", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(Workflow{ID: "wf-1", Enabled: false})
	})
	mux.HandleFunc("DELETE /api/v1/workflows/wf-1", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("POST /api/v1/workflows/wf-1/test", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok"})
	})
	mux.HandleFunc("GET /api/v1/workflows/wf-1/versions", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]WorkflowVersion{{Version: 1}})
	})
	mux.HandleFunc("GET /api/v1/workflows/wf-1/versions/1", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(WorkflowVersion{Version: 1})
	})
	mux.HandleFunc("POST /api/v1/workflows/wf-1/rollback/1", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"workflow": Workflow{ID: "wf-1"}})
	})
	mux.HandleFunc("GET /api/v1/workflows/wf-1/diff", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(WorkflowDiff{WorkflowID: "wf-1", FromVersion: 1, ToVersion: 2})
	})
	mux.HandleFunc("GET /api/v1/workflows/wf-1/export", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"schema_version":"1.0"}`))
	})
	mux.HandleFunc("POST /api/v1/workflows/import", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(Workflow{ID: "wf-imported"})
	})

	// Execution lifecycle
	mux.HandleFunc("GET /api/v1/executions", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]Execution{{ID: "exec-1"}})
	})
	mux.HandleFunc("GET /api/v1/executions/exec-1", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(Execution{ID: "exec-1", Steps: []ExecutionStep{{ID: "step-1"}}})
	})
	mux.HandleFunc("POST /api/v1/executions/exec-1/retry", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(Execution{ID: "exec-1", Status: ExecutionRunning})
	})
	mux.HandleFunc("POST /api/v1/executions/exec-1/pause", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(Execution{ID: "exec-1", Status: ExecutionPaused})
	})
	mux.HandleFunc("POST /api/v1/executions/exec-1/resume", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(Execution{ID: "exec-1", Status: ExecutionRunning})
	})
	mux.HandleFunc("POST /api/v1/executions/exec-1/cancel", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(Execution{ID: "exec-1", Status: ExecutionCancelled})
	})
	mux.HandleFunc("GET /api/v1/executions/exec-1/logs", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]ExecutionStep{{ID: "step-1", Status: "success"}})
	})

	// Approvals & Events
	mux.HandleFunc("GET /api/v1/approvals", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]Approval{{ID: "app-1"}})
	})
	mux.HandleFunc("GET /api/v1/approvals/app-1", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(Approval{ID: "app-1"})
	})
	mux.HandleFunc("POST /api/v1/approvals/app-1/reject", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(Approval{ID: "app-1", Status: ApprovalRejected})
	})
	mux.HandleFunc("POST /api/v1/approval-tokens/tok-1", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(Approval{ID: "app-1", Status: ApprovalApproved})
	})
	mux.HandleFunc("POST /api/v1/events", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(EventRecord{ID: "evt-1"})
	})
	mux.HandleFunc("GET /api/v1/events/evt-1", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(EventRecord{ID: "evt-1"})
	})
	mux.HandleFunc("GET /api/v1/events", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]EventRecord{{ID: "evt-1"}})
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	client, err := NewClient(
		server.URL,
		WithToken("test-token"),
		WithTenant("tenant-alpha"),
		WithHTTPClient(server.Client()),
		WithUserAgent("custom-sdk/1.0"),
	)
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}

	if client.Token() != "test-token" {
		t.Errorf("unexpected token: %s", client.Token())
	}
	if client.TenantID() != "tenant-alpha" {
		t.Errorf("unexpected tenant: %s", client.TenantID())
	}
	if client.Endpoint() != server.URL {
		t.Errorf("unexpected endpoint: %s", client.Endpoint())
	}

	ctx := context.Background()

	// Tenancy checks
	tenants, err := client.ListTenants(ctx)
	if err != nil || len(tenants) != 1 {
		t.Fatalf("ListTenants failed: %v", err)
	}
	_, err = client.GetTenant(ctx, "t-1")
	if err != nil {
		t.Fatalf("GetTenant failed: %v", err)
	}
	_, err = client.CreateTenant(ctx, "New Tenant", "new-tenant")
	if err != nil {
		t.Fatalf("CreateTenant failed: %v", err)
	}
	members, err := client.ListMembers(ctx)
	if err != nil || len(members) != 1 {
		t.Fatalf("ListMembers failed: %v", err)
	}
	tokens, err := client.ListAPITokens(ctx)
	if err != nil || len(tokens) != 1 {
		t.Fatalf("ListAPITokens failed: %v", err)
	}
	_, err = client.CreateAPIToken(ctx, "Tok", []string{"read"})
	if err != nil {
		t.Fatalf("CreateAPIToken failed: %v", err)
	}
	if err := client.RevokeAPIToken(ctx, "tok-1"); err != nil {
		t.Fatalf("RevokeAPIToken failed: %v", err)
	}

	// Workflow checks
	_, err = client.UpdateWorkflow(ctx, "wf-1", WorkflowCreateInput{Name: "Updated"})
	if err != nil {
		t.Fatalf("UpdateWorkflow failed: %v", err)
	}
	_, err = client.PatchWorkflow(ctx, "wf-1", map[string]any{"enabled": false})
	if err != nil {
		t.Fatalf("PatchWorkflow failed: %v", err)
	}
	_, err = client.TestWorkflow(ctx, "wf-1", map[string]any{"data": 1})
	if err != nil {
		t.Fatalf("TestWorkflow failed: %v", err)
	}
	_, err = client.ListWorkflowVersions(ctx, "wf-1")
	if err != nil {
		t.Fatalf("ListWorkflowVersions failed: %v", err)
	}
	_, err = client.GetWorkflowVersion(ctx, "wf-1", 1)
	if err != nil {
		t.Fatalf("GetWorkflowVersion failed: %v", err)
	}
	_, err = client.RollbackWorkflow(ctx, "wf-1", 1)
	if err != nil {
		t.Fatalf("RollbackWorkflow failed: %v", err)
	}
	_, err = client.DiffWorkflowVersions(ctx, "wf-1", 1, 2)
	if err != nil {
		t.Fatalf("DiffWorkflowVersions failed: %v", err)
	}
	_, err = client.ExportWorkflow(ctx, "wf-1", "json")
	if err != nil {
		t.Fatalf("ExportWorkflow failed: %v", err)
	}
	_, err = client.ImportWorkflow(ctx, []byte(`{"id":"wf-imported"}`))
	if err != nil {
		t.Fatalf("ImportWorkflow failed: %v", err)
	}
	if err := client.DeleteWorkflow(ctx, "wf-1"); err != nil {
		t.Fatalf("DeleteWorkflow failed: %v", err)
	}

	// Execution checks
	execs, err := client.ListExecutions(ctx)
	if err != nil || len(execs) != 1 {
		t.Fatalf("ListExecutions failed: %v", err)
	}
	_, err = client.GetExecution(ctx, "exec-1")
	if err != nil {
		t.Fatalf("GetExecution failed: %v", err)
	}
	_, err = client.RetryExecution(ctx, "exec-1")
	if err != nil {
		t.Fatalf("RetryExecution failed: %v", err)
	}
	_, err = client.PauseExecution(ctx, "exec-1")
	if err != nil {
		t.Fatalf("PauseExecution failed: %v", err)
	}
	_, err = client.ResumeExecution(ctx, "exec-1")
	if err != nil {
		t.Fatalf("ResumeExecution failed: %v", err)
	}
	_, err = client.CancelExecution(ctx, "exec-1")
	if err != nil {
		t.Fatalf("CancelExecution failed: %v", err)
	}
	logs, err := client.GetExecutionLogs(ctx, "exec-1")
	if err != nil || len(logs) != 1 {
		t.Fatalf("GetExecutionLogs failed: %v", err)
	}

	// Approvals & Events
	apps, err := client.ListApprovals(ctx)
	if err != nil || len(apps) != 1 {
		t.Fatalf("ListApprovals failed: %v", err)
	}
	_, err = client.GetApproval(ctx, "app-1")
	if err != nil {
		t.Fatalf("GetApproval failed: %v", err)
	}
	_, err = client.Reject(ctx, "app-1", "rejected note")
	if err != nil {
		t.Fatalf("Reject failed: %v", err)
	}
	_, err = client.DecideApprovalByToken(ctx, "tok-1", "approved", "note", "admin")
	if err != nil {
		t.Fatalf("DecideApprovalByToken failed: %v", err)
	}
	_, err = client.IngestEvent(ctx, "source-1", "test.event", map[string]any{"ok": true})
	if err != nil {
		t.Fatalf("IngestEvent failed: %v", err)
	}
	_, err = client.GetEvent(ctx, "evt-1")
	if err != nil {
		t.Fatalf("GetEvent failed: %v", err)
	}
	events, err := client.ListEvents(ctx, 10, 0)
	if err != nil || len(events) != 1 {
		t.Fatalf("ListEvents failed: %v", err)
	}

	// APIError check
	apiErr := &APIError{StatusCode: 404, Message: "not found", Code: "NOT_FOUND"}
	if apiErr.Error() == "" {
		t.Error("expected non-empty APIError string")
	}
}
