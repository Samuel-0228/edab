package auth

import (
	"context"
	"testing"
)

func TestRolePermissions(t *testing.T) {
	// Viewer
	if !RoleViewer.HasPermission(PermWorkflowsRead) {
		t.Errorf("Viewer should have PermWorkflowsRead")
	}
	if RoleViewer.HasPermission(PermWorkflowsWrite) {
		t.Errorf("Viewer should not have PermWorkflowsWrite")
	}
	if RoleViewer.HasPermission(PermExecutionsRetry) {
		t.Errorf("Viewer should not have PermExecutionsRetry")
	}

	// Operator
	if !RoleOperator.HasPermission(PermWorkflowsRead) {
		t.Errorf("Operator should have PermWorkflowsRead")
	}
	if !RoleOperator.HasPermission(PermExecutionsRetry) {
		t.Errorf("Operator should have PermExecutionsRetry")
	}
	if !RoleOperator.HasPermission(PermEventsReplay) {
		t.Errorf("Operator should have PermEventsReplay")
	}
	if RoleOperator.HasPermission(PermWorkflowsWrite) {
		t.Errorf("Operator should not have PermWorkflowsWrite")
	}

	// Editor
	if !RoleEditor.HasPermission(PermWorkflowsWrite) {
		t.Errorf("Editor should have PermWorkflowsWrite")
	}
	if !RoleEditor.HasPermission(PermCredentialsWrite) {
		t.Errorf("Editor should have PermCredentialsWrite")
	}
	if RoleEditor.HasPermission(PermMembersManage) {
		t.Errorf("Editor should not have PermMembersManage")
	}

	// Admin
	if !RoleAdmin.HasPermission(PermMembersManage) {
		t.Errorf("Admin should have PermMembersManage")
	}
	if !RoleAdmin.HasPermission(PermTokensManage) {
		t.Errorf("Admin should have PermTokensManage")
	}
	if RoleAdmin.HasPermission(PermTenantManage) {
		t.Errorf("Admin should not have PermTenantManage")
	}

	// Owner
	if !RoleOwner.HasPermission(PermTenantManage) {
		t.Errorf("Owner should have PermTenantManage")
	}
}

func TestScrubAuditDetails(t *testing.T) {
	raw := map[string]any{
		"workflow_name": "Prod Workflow",
		"token":         "super_secret_token_123",
		"auth_header":   "Bearer secret-jwt",
		"nested": map[string]any{
			"api_key":   "sk_live_xyz",
			"public_id": "pub_123",
			"inner": map[string]string{
				"password": "my_password",
				"status":   "active",
			},
		},
		"list": []any{
			map[string]any{"secret_key": "hidden", "item": "safe"},
		},
	}

	scrubbed := ScrubAuditDetails(raw)

	if scrubbed["workflow_name"] != "Prod Workflow" {
		t.Errorf("expected workflow_name to be preserved")
	}
	if scrubbed["token"] != "[REDACTED]" {
		t.Errorf("expected token to be [REDACTED], got %v", scrubbed["token"])
	}
	if scrubbed["auth_header"] != "[REDACTED]" {
		t.Errorf("expected auth_header to be [REDACTED], got %v", scrubbed["auth_header"])
	}

	nested := scrubbed["nested"].(map[string]any)
	if nested["api_key"] != "[REDACTED]" {
		t.Errorf("expected nested api_key to be [REDACTED]")
	}
	if nested["public_id"] != "pub_123" {
		t.Errorf("expected nested public_id to be preserved")
	}

	inner := nested["inner"].(map[string]any)
	if inner["password"] != "[REDACTED]" {
		t.Errorf("expected inner password to be [REDACTED]")
	}
	if inner["status"] != "active" {
		t.Errorf("expected inner status to be preserved")
	}

	list := scrubbed["list"].([]any)
	listItem := list[0].(map[string]any)
	if listItem["secret_key"] != "[REDACTED]" {
		t.Errorf("expected list secret_key to be [REDACTED]")
	}
	if listItem["item"] != "safe" {
		t.Errorf("expected list item to be preserved")
	}
}

func TestGenerateAndVerifyAPIToken(t *testing.T) {
	rawToken, prefix, hash, err := GenerateAPIToken("test")
	if err != nil {
		t.Fatalf("GenerateAPIToken failed: %v", err)
	}
	if prefix == "" || hash == "" || rawToken == "" {
		t.Fatalf("empty token output: raw=%s, prefix=%s, hash=%s", rawToken, prefix, hash)
	}

	if !VerifyAPIToken(rawToken, hash) {
		t.Fatalf("VerifyAPIToken failed for matching token and hash")
	}
	if VerifyAPIToken("krb_wrong_token", hash) {
		t.Fatalf("VerifyAPIToken passed for incorrect token")
	}
}

func TestMemoryStoreTenancyAndRBAC(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()

	// 1. Verify default seeded tenant & user
	defTenant, err := store.GetTenant(ctx, "default")
	if err != nil || defTenant.Slug != "default" {
		t.Fatalf("expected default tenant, got: %v", err)
	}
	defUser, err := store.GetUser(ctx, "default-user")
	if err != nil || defUser.Email != "admin@local" {
		t.Fatalf("expected default user, got: %v", err)
	}
	defMem, err := store.GetMembership(ctx, "default", "default-user")
	if err != nil || defMem.Role != RoleOwner {
		t.Fatalf("expected default membership with owner role, got: %v", err)
	}

	// 2. Create second tenant
	tenantB, err := store.CreateTenant(ctx, &Tenant{Name: "Acme Corp", Slug: "acme"})
	if err != nil {
		t.Fatalf("CreateTenant failed: %v", err)
	}
	userB, err := store.CreateUser(ctx, &User{Email: "alice@acme.com", Name: "Alice"})
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}
	memB, err := store.CreateMembership(ctx, &Membership{
		TenantID: tenantB.ID,
		UserID:   userB.ID,
		Role:     RoleAdmin,
	})
	if err != nil || memB.Role != RoleAdmin {
		t.Fatalf("CreateMembership failed: %v", err)
	}

	// 3. Create API Token for Acme
	_, prefix, hash, _ := GenerateAPIToken("acme")
	tok, err := store.CreateAPIToken(ctx, &APIToken{
		TenantID:    tenantB.ID,
		Name:        "CI/CD Token",
		TokenPrefix: prefix,
		TokenHash:   hash,
		Role:        RoleEditor,
		Scopes:      []string{"workflows:*"},
	})
	if err != nil {
		t.Fatalf("CreateAPIToken failed: %v", err)
	}

	// Lookup token by hash
	foundTok, err := store.GetAPITokenByHash(ctx, hash)
	if err != nil || foundTok.ID != tok.ID {
		t.Fatalf("GetAPITokenByHash failed: %v", err)
	}
	if foundTok.TenantID != tenantB.ID {
		t.Errorf("token tenant mismatch")
	}

	// Touch token
	_ = store.TouchAPIToken(ctx, tok.ID)
	touched, _ := store.GetAPITokenByHash(ctx, hash)
	if touched.LastUsedAt == nil {
		t.Errorf("expected LastUsedAt to be populated after touch")
	}

	// 4. Audit Log recording
	err = store.RecordAuditLog(ctx, &AuditLog{
		TenantID:     tenantB.ID,
		ActorID:      userB.ID,
		ActorType:    "user",
		Action:       "workflow.create",
		ResourceType: "workflow",
		ResourceID:   "wf-101",
		Details: map[string]any{
			"workflow_name": "Daily Sync",
			"secret_token":  "super_confidential_123",
		},
		IPAddress: "127.0.0.1",
	})
	if err != nil {
		t.Fatalf("RecordAuditLog failed: %v", err)
	}

	logs, err := store.ListAuditLogs(ctx, tenantB.ID, AuditFilter{})
	if err != nil || len(logs) != 1 {
		t.Fatalf("expected 1 audit log for Acme, got %d (err: %v)", len(logs), err)
	}
	if logs[0].Details["secret_token"] != "[REDACTED]" {
		t.Errorf("expected secret_token to be [REDACTED] in audit log, got %v", logs[0].Details["secret_token"])
	}

	// Verify Tenant Isolation: Default tenant has 0 audit logs
	defLogs, _ := store.ListAuditLogs(ctx, "default", AuditFilter{})
	if len(defLogs) != 0 {
		t.Errorf("expected 0 audit logs for default tenant, got %d", len(defLogs))
	}
}

func TestIdentityCapabilities(t *testing.T) {
	id := &Identity{
		TenantID:  "tenant-a",
		UserID:    "user-1",
		ActorType: "api_token",
		Role:      RoleViewer,
		Scopes:    []string{"workflows:read", "executions:retry"},
		IsAuth:    true,
	}

	if !id.CanAccessTenant("tenant-a") {
		t.Errorf("should allow access to own tenant")
	}
	if id.CanAccessTenant("tenant-b") {
		t.Errorf("should deny cross-tenant access to tenant-b")
	}

	// Viewer cannot retry executions via role, but this token has explicit scope "executions:retry"
	if !id.HasPermission(PermExecutionsRetry) {
		t.Errorf("expected explicit scope executions:retry to be permitted")
	}

	// Should not have PermWorkflowsWrite
	if id.HasPermission(PermWorkflowsWrite) {
		t.Errorf("should not have PermWorkflowsWrite")
	}
}
