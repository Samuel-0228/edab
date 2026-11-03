package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"kerob/internal/core/auth"
	"kerob/internal/core/governance"
)

func TestGovernanceAPI(t *testing.T) {
	_, _, cfg := setupTestServer()

	authStore := auth.NewMemoryStore()
	govStore := governance.NewMemoryStore()
	govEngine := governance.NewGovernanceEngine(govStore)

	cfg.AuthStore = authStore
	cfg.GovernanceEngine = govEngine
	cfg.MultiTenancy = true

	if cfg.WorkflowEngine != nil {
		cfg.WorkflowEngine.SetGovernance(govEngine)
	}

	handlers := NewHandlersFromConfig(cfg)
	srv := NewServer("0", handlers)

	tenantA := "tenant-alpha"
	tenantB := "tenant-beta"

	// Create test identities
	adminA := &auth.Identity{
		TenantID:  tenantA,
		UserID:    "admin-a",
		ActorType: "user",
		Role:      auth.RoleAdmin,
		IsAuth:    true,
	}
	viewerA := &auth.Identity{
		TenantID:  tenantA,
		UserID:    "viewer-a",
		ActorType: "user",
		Role:      auth.RoleViewer,
		IsAuth:    true,
	}
	adminB := &auth.Identity{
		TenantID:  tenantB,
		UserID:    "admin-b",
		ActorType: "user",
		Role:      auth.RoleAdmin,
		IsAuth:    true,
	}

	t.Run("GET default policy", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/governance/policy", nil)
		req = req.WithContext(auth.WithIdentity(req.Context(), adminA))
		rec := httptest.NewRecorder()

		srv.mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
		}

		var pol governance.Policy
		if err := json.Unmarshal(rec.Body.Bytes(), &pol); err != nil {
			t.Fatalf("failed to decode policy: %v", err)
		}
		if pol.TenantID != tenantA {
			t.Errorf("expected tenant %s, got %s", tenantA, pol.TenantID)
		}
		if pol.EnforcementMode != governance.ModeEnforce {
			t.Errorf("expected default enforcement mode 'enforce', got %s", pol.EnforcementMode)
		}
		if pol.DataRetentionDays != 90 {
			t.Errorf("expected 90 retention days, got %d", pol.DataRetentionDays)
		}
	})

	t.Run("PUT policy as Admin vs Viewer RBAC", func(t *testing.T) {
		payload := `{"allowed_connectors":["http","telegram"],"max_workflow_retries":3,"require_approval_irreversible":true}`

		// Viewer attempt (must fail with 403 Forbidden)
		reqViewer := httptest.NewRequest(http.MethodPut, "/api/v1/governance/policy", strings.NewReader(payload))
		reqViewer = reqViewer.WithContext(auth.WithIdentity(reqViewer.Context(), viewerA))
		recViewer := httptest.NewRecorder()

		srv.mux.ServeHTTP(recViewer, reqViewer)
		if recViewer.Code != http.StatusForbidden {
			t.Errorf("expected 403 Forbidden for viewer updating policy, got: %d", recViewer.Code)
		}

		// Admin attempt (must succeed with 200 OK)
		reqAdmin := httptest.NewRequest(http.MethodPut, "/api/v1/governance/policy", strings.NewReader(payload))
		reqAdmin = reqAdmin.WithContext(auth.WithIdentity(reqAdmin.Context(), adminA))
		recAdmin := httptest.NewRecorder()

		srv.mux.ServeHTTP(recAdmin, reqAdmin)
		if recAdmin.Code != http.StatusOK {
			t.Fatalf("expected 200 OK for admin updating policy, got: %d (%s)", recAdmin.Code, recAdmin.Body.String())
		}

		var updated governance.Policy
		_ = json.Unmarshal(recAdmin.Body.Bytes(), &updated)
		if len(updated.AllowedConnectors) != 2 || updated.MaxWorkflowRetries != 3 {
			t.Errorf("policy was not updated correctly: %+v", updated)
		}
	})

	t.Run("POST policy dry-run validate", func(t *testing.T) {
		wfJSON := `{
			"name": "Unapproved Connector Test",
			"trigger": {"type": "webhook"},
			"actions": [
				{"id": "s1", "type": "action", "integration": "discord"}
			]
		}`

		req := httptest.NewRequest(http.MethodPost, "/api/v1/governance/policy/validate", strings.NewReader(wfJSON))
		req = req.WithContext(auth.WithIdentity(req.Context(), viewerA))
		rec := httptest.NewRecorder()

		srv.mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got: %d (%s)", rec.Code, rec.Body.String())
		}

		var res struct {
			Valid      bool                         `json:"valid"`
			Violations []governance.PolicyViolation `json:"violations"`
		}
		_ = json.Unmarshal(rec.Body.Bytes(), &res)
		if res.Valid {
			t.Errorf("expected validation to fail due to unapproved connector 'discord'")
		}
		if len(res.Violations) == 0 || res.Violations[0].Rule != "allowed_connectors" {
			t.Errorf("expected allowed_connectors violation, got: %+v", res.Violations)
		}
	})

	t.Run("Publish-time enforcement blocks non-compliant workflow", func(t *testing.T) {
		wfBody := `{
			"name": "High Retries Workflow",
			"trigger": {"type": "webhook"},
			"max_retries": 10,
			"actions": [
				{"id": "s1", "type": "action", "integration": "http", "action_type": "request", "config": {"url": "https://example.com"}}
			]
		}`

		req := httptest.NewRequest(http.MethodPost, "/api/v1/workflows", strings.NewReader(wfBody))
		req = req.WithContext(auth.WithIdentity(req.Context(), adminA))
		rec := httptest.NewRecorder()

		srv.mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("expected 422 Unprocessable Entity for policy violation, got: %d (%s)", rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), "POLICY_VIOLATION") {
			t.Errorf("expected error code POLICY_VIOLATION, got: %s", rec.Body.String())
		}
	})

	t.Run("Audit chain verification & export", func(t *testing.T) {
		ctx := context.Background()

		// Record 2 audit events for tenantA
		_ = authStore.RecordAuditLog(ctx, &auth.AuditLog{
			TenantID:     tenantA,
			ActorID:      adminA.UserID,
			ActorType:    "user",
			Action:       "policy.test",
			ResourceType: "policy",
			ResourceID:   tenantA,
			CreatedAt:    time.Now().UTC(),
		})
		_ = authStore.RecordAuditLog(ctx, &auth.AuditLog{
			TenantID:     tenantA,
			ActorID:      adminA.UserID,
			ActorType:    "user",
			Action:       "policy.verify",
			ResourceType: "policy",
			ResourceID:   tenantA,
			CreatedAt:    time.Now().UTC(),
		})

		// Verify audit chain endpoint
		verifyReq := httptest.NewRequest(http.MethodGet, "/api/v1/governance/audit/verify", nil)
		verifyReq = verifyReq.WithContext(auth.WithIdentity(verifyReq.Context(), adminA))
		verifyRec := httptest.NewRecorder()

		srv.mux.ServeHTTP(verifyRec, verifyReq)
		if verifyRec.Code != http.StatusOK {
			t.Fatalf("verify chain returned code %d: %s", verifyRec.Code, verifyRec.Body.String())
		}

		var verifyRes governance.AuditVerificationResult
		_ = json.Unmarshal(verifyRec.Body.Bytes(), &verifyRes)
		if !verifyRes.Valid {
			t.Errorf("expected audit chain to be valid, got: %+v", verifyRes)
		}
		if verifyRes.TotalRecords < 2 {
			t.Errorf("expected at least 2 records, got %d", verifyRes.TotalRecords)
		}

		// Export CSV
		csvReq := httptest.NewRequest(http.MethodGet, "/api/v1/audit-logs/export?format=csv", nil)
		csvReq = csvReq.WithContext(auth.WithIdentity(csvReq.Context(), adminA))
		csvRec := httptest.NewRecorder()

		srv.mux.ServeHTTP(csvRec, csvReq)
		if csvRec.Code != http.StatusOK {
			t.Fatalf("export csv returned code %d", csvRec.Code)
		}
		if !strings.Contains(csvRec.Header().Get("Content-Type"), "text/csv") {
			t.Errorf("expected text/csv header, got: %s", csvRec.Header().Get("Content-Type"))
		}
		if !strings.Contains(csvRec.Body.String(), "sequence_num") || !strings.Contains(csvRec.Body.String(), tenantA) {
			t.Errorf("expected CSV body with headers and tenant ID, got: %s", csvRec.Body.String())
		}

		// Export JSON
		jsonReq := httptest.NewRequest(http.MethodGet, "/api/v1/audit-logs/export?format=json", nil)
		jsonReq = jsonReq.WithContext(auth.WithIdentity(jsonReq.Context(), adminA))
		jsonRec := httptest.NewRecorder()

		srv.mux.ServeHTTP(jsonRec, jsonReq)
		if jsonRec.Code != http.StatusOK {
			t.Fatalf("export json returned code %d", jsonRec.Code)
		}
		var exportedLogs []auth.AuditLog
		if err := json.Unmarshal(jsonRec.Body.Bytes(), &exportedLogs); err != nil {
			t.Fatalf("failed to parse exported json logs: %v", err)
		}
		if len(exportedLogs) < 2 {
			t.Errorf("expected exported json logs >= 2, got: %d", len(exportedLogs))
		}
	})

	t.Run("Retention prune endpoint", func(t *testing.T) {
		pruneReq := httptest.NewRequest(http.MethodPost, "/api/v1/governance/retention/prune", nil)
		pruneReq = pruneReq.WithContext(auth.WithIdentity(pruneReq.Context(), adminA))
		pruneRec := httptest.NewRecorder()

		srv.mux.ServeHTTP(pruneRec, pruneReq)
		if pruneRec.Code != http.StatusOK {
			t.Fatalf("retention prune returned code %d: %s", pruneRec.Code, pruneRec.Body.String())
		}

		var pruneRes struct {
			PrunedCount   int64 `json:"pruned_audit_logs"`
			RetentionDays int   `json:"retention_days"`
		}
		_ = json.Unmarshal(pruneRec.Body.Bytes(), &pruneRes)
		if pruneRes.RetentionDays <= 0 {
			t.Errorf("expected valid retention days, got %d", pruneRes.RetentionDays)
		}
	})

	t.Run("Cross-tenant isolation: Tenant B cannot access Tenant A policy", func(t *testing.T) {
		reqB := httptest.NewRequest(http.MethodGet, "/api/v1/governance/policy", nil)
		reqB = reqB.WithContext(auth.WithIdentity(reqB.Context(), adminB))
		recB := httptest.NewRecorder()

		srv.mux.ServeHTTP(recB, reqB)
		if recB.Code != http.StatusOK {
			t.Fatalf("admin B get policy failed: %d", recB.Code)
		}

		var polB governance.Policy
		_ = json.Unmarshal(recB.Body.Bytes(), &polB)
		if polB.TenantID != tenantB {
			t.Errorf("expected tenant B policy, got %s", polB.TenantID)
		}
		// Tenant B has empty connectors (default), whereas Tenant A has [http, telegram]
		if len(polB.AllowedConnectors) != 0 {
			t.Errorf("tenant B should not have tenant A's allowed connectors")
		}
	})
}
