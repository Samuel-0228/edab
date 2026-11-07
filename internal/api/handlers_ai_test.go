package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"kerob/internal/core/ai"
	"kerob/internal/core/auth"
)

func TestAIHandlers(t *testing.T) {
	_, _, cfg := setupTestServer()

	authStore := auth.NewMemoryStore()
	aiStore := ai.NewMemoryStore()
	mockProvider := ai.NewMockProvider()

	aiSvc := ai.NewService(ai.ServiceConfig{
		Enabled:             false, // start disabled by default
		Provider:            mockProvider,
		Store:               aiStore,
		MaxTokensPerRequest: 1000,
		MonthlyTokenLimit:   10000,
		Timeout:             5 * time.Second,
	})

	cfg.AuthStore = authStore
	cfg.AIService = aiSvc
	cfg.MultiTenancy = true

	handlers := NewHandlersFromConfig(cfg)
	srv := NewServer("0", handlers)

	tenantA := "tenant-alpha"
	tenantB := "tenant-beta"

	adminA := &auth.Identity{
		TenantID:  tenantA,
		UserID:    "admin-user",
		ActorType: "user",
		Role:      auth.RoleAdmin,
		IsAuth:    true,
	}
	viewerA := &auth.Identity{
		TenantID:  tenantA,
		UserID:    "viewer-user",
		ActorType: "user",
		Role:      auth.RoleViewer,
		IsAuth:    true,
	}
	adminB := &auth.Identity{
		TenantID:  tenantB,
		UserID:    "admin-b-user",
		ActorType: "user",
		Role:      auth.RoleAdmin,
		IsAuth:    true,
	}

	t.Run("GET /api/v1/ai/status when disabled by default", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/v1/ai/status", nil)
		req = req.WithContext(auth.WithIdentity(req.Context(), adminA))
		w := httptest.NewRecorder()
		srv.mux.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d: %s", w.Code, w.Body.String())
		}
		var status ai.AIStatusResponse
		if err := json.NewDecoder(w.Body).Decode(&status); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if status.Enabled {
			t.Errorf("expected AI to be disabled by default")
		}
	})

	t.Run("AI drafting forbidden when disabled", func(t *testing.T) {
		payload := `{"prompt": "Sync GitHub issues to Telegram"}`
		req := httptest.NewRequest("POST", "/api/v1/ai/draft-workflow", strings.NewReader(payload))
		req = req.WithContext(auth.WithIdentity(req.Context(), adminA))
		w := httptest.NewRecorder()
		srv.mux.ServeHTTP(w, req)

		if w.Code != http.StatusForbidden {
			t.Fatalf("expected 403 Forbidden when AI is disabled, got %d: %s", w.Code, w.Body.String())
		}
	})

	// Now enable AI assistance
	aiSvc.SetEnabled(true)

	var proposalID string

	t.Run("POST /api/v1/ai/draft-workflow successfully creates proposal", func(t *testing.T) {
		payload := `{"prompt": "Send notification on webhook events", "allowed_connectors": ["http", "telegram", "webhook"]}`
		req := httptest.NewRequest("POST", "/api/v1/ai/draft-workflow", strings.NewReader(payload))
		req = req.WithContext(auth.WithIdentity(req.Context(), adminA))
		w := httptest.NewRecorder()
		srv.mux.ServeHTTP(w, req)

		if w.Code != http.StatusCreated {
			t.Fatalf("expected 201 Created, got %d: %s", w.Code, w.Body.String())
		}

		var p ai.Proposal
		if err := json.NewDecoder(w.Body).Decode(&p); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if p.ID == "" {
			t.Error("expected non-empty proposal ID")
		}
		if p.Task != ai.TaskDraftWorkflow {
			t.Errorf("expected task draft_workflow, got %s", p.Task)
		}
		if p.Status != ai.StatusPendingApproval {
			t.Errorf("expected status pending_approval, got %s", p.Status)
		}
		if !p.Validation.Valid {
			t.Errorf("expected proposal to be valid, got errors: %v", p.Validation.Errors)
		}
		proposalID = p.ID
	})

	t.Run("POST /api/v1/ai/map-fields", func(t *testing.T) {
		payload := `{"description": "Map customer payload", "sample_payload": {"user_email": "test@example.com"}}`
		req := httptest.NewRequest("POST", "/api/v1/ai/map-fields", strings.NewReader(payload))
		req = req.WithContext(auth.WithIdentity(req.Context(), adminA))
		w := httptest.NewRecorder()
		srv.mux.ServeHTTP(w, req)

		if w.Code != http.StatusCreated {
			t.Fatalf("expected 201 Created, got %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("POST /api/v1/ai/generate-conditions", func(t *testing.T) {
		payload := `{"criteria": "Only allow status eq completed"}`
		req := httptest.NewRequest("POST", "/api/v1/ai/generate-conditions", strings.NewReader(payload))
		req = req.WithContext(auth.WithIdentity(req.Context(), adminA))
		w := httptest.NewRecorder()
		srv.mux.ServeHTTP(w, req)

		if w.Code != http.StatusCreated {
			t.Fatalf("expected 201 Created, got %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("POST /api/v1/ai/suggest-transformations", func(t *testing.T) {
		payload := `{"goal": "Convert timestamp to ISO", "input_payload": {"ts": 123456789}}`
		req := httptest.NewRequest("POST", "/api/v1/ai/suggest-transformations", strings.NewReader(payload))
		req = req.WithContext(auth.WithIdentity(req.Context(), adminA))
		w := httptest.NewRecorder()
		srv.mux.ServeHTTP(w, req)

		if w.Code != http.StatusCreated {
			t.Fatalf("expected 201 Created, got %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("POST /api/v1/ai/explain-failure", func(t *testing.T) {
		payload := `{"workflow_id": "wf-1", "execution_id": "exec-1", "step_id": "http-1", "error": "timeout connecting to server"}`
		req := httptest.NewRequest("POST", "/api/v1/ai/explain-failure", strings.NewReader(payload))
		req = req.WithContext(auth.WithIdentity(req.Context(), adminA))
		w := httptest.NewRecorder()
		srv.mux.ServeHTTP(w, req)

		if w.Code != http.StatusCreated {
			t.Fatalf("expected 201 Created, got %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("GET /api/v1/ai/proposals and GET /api/v1/ai/proposals/{id}", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/v1/ai/proposals", nil)
		req = req.WithContext(auth.WithIdentity(req.Context(), adminA))
		w := httptest.NewRecorder()
		srv.mux.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d: %s", w.Code, w.Body.String())
		}
		var list []*ai.Proposal
		if err := json.NewDecoder(w.Body).Decode(&list); err != nil {
			t.Fatalf("failed to decode list: %v", err)
		}
		if len(list) < 5 {
			t.Errorf("expected at least 5 proposals, got %d", len(list))
		}

		// Get single proposal
		reqSingle := httptest.NewRequest("GET", "/api/v1/ai/proposals/"+proposalID, nil)
		reqSingle = reqSingle.WithContext(auth.WithIdentity(reqSingle.Context(), adminA))
		wSingle := httptest.NewRecorder()
		srv.mux.ServeHTTP(wSingle, reqSingle)

		if wSingle.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d: %s", wSingle.Code, wSingle.Body.String())
		}
	})

	t.Run("Cross-tenant isolation on proposals", func(t *testing.T) {
		// Tenant B attempts to read Tenant A's proposal
		req := httptest.NewRequest("GET", "/api/v1/ai/proposals/"+proposalID, nil)
		req = req.WithContext(auth.WithIdentity(req.Context(), adminB))
		w := httptest.NewRecorder()
		srv.mux.ServeHTTP(w, req)

		if w.Code != http.StatusNotFound {
			t.Errorf("expected 404 Not Found for cross-tenant access, got %d", w.Code)
		}
	})

	t.Run("Human-in-the-loop: viewer cannot accept proposal (requires PermWorkflowsWrite)", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/api/v1/ai/proposals/"+proposalID+"/accept", strings.NewReader(`{}`))
		req = req.WithContext(auth.WithIdentity(req.Context(), viewerA))
		w := httptest.NewRecorder()
		srv.mux.ServeHTTP(w, req)

		if w.Code != http.StatusForbidden {
			t.Errorf("expected 403 Forbidden for viewer accepting proposal, got %d", w.Code)
		}
	})

	t.Run("POST /api/v1/ai/proposals/{id}/accept with workflow instantiation", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/api/v1/ai/proposals/"+proposalID+"/accept?create_workflow=true", strings.NewReader(`{}`))
		req = req.WithContext(auth.WithIdentity(req.Context(), adminA))
		w := httptest.NewRecorder()
		srv.mux.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d: %s", w.Code, w.Body.String())
		}

		var resp map[string]any
		if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		appliedID, ok := resp["applied_workflow_id"].(string)
		if !ok || appliedID == "" {
			t.Errorf("expected applied_workflow_id to be populated")
		}

		// Verify workflow was actually created in store
		wf, err := cfg.Workflows.GetByID(context.Background(), appliedID)
		if err != nil || wf == nil {
			t.Errorf("expected created workflow to exist in store: %v", err)
		} else if wf.TenantID != tenantA {
			t.Errorf("expected workflow tenant to be %s, got %s", tenantA, wf.TenantID)
		}
	})

	t.Run("Accepting already accepted proposal returns 409 Conflict", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/api/v1/ai/proposals/"+proposalID+"/accept", strings.NewReader(`{}`))
		req = req.WithContext(auth.WithIdentity(req.Context(), adminA))
		w := httptest.NewRecorder()
		srv.mux.ServeHTTP(w, req)

		if w.Code != http.StatusConflict {
			t.Errorf("expected 409 Conflict for already decided proposal, got %d", w.Code)
		}
	})

	t.Run("POST /api/v1/ai/proposals/{id}/reject", func(t *testing.T) {
		// Create a new proposal to reject
		p, err := aiSvc.SuggestTransformation(context.Background(), tenantA, "admin-user", ai.SuggestTransformationRequest{
			Goal:         "Format names",
			InputPayload: map[string]any{"name": "kerob"},
		})
		if err != nil {
			t.Fatalf("failed to create proposal: %v", err)
		}

		req := httptest.NewRequest("POST", "/api/v1/ai/proposals/"+p.ID+"/reject", strings.NewReader(`{"reason": "Not needed"}`))
		req = req.WithContext(auth.WithIdentity(req.Context(), adminA))
		w := httptest.NewRecorder()
		srv.mux.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d: %s", w.Code, w.Body.String())
		}

		var rejected ai.Proposal
		if err := json.NewDecoder(w.Body).Decode(&rejected); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if rejected.Status != ai.StatusRejected {
			t.Errorf("expected status rejected, got %s", rejected.Status)
		}
	})
}
