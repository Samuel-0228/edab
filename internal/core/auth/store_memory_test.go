package auth

import (
	"context"
	"testing"
	"time"
)

func TestMemoryStoreFull(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()

	// 1. Tenants
	tenant, err := store.CreateTenant(ctx, &Tenant{
		Name: "Acme Corp",
		Slug: "acme-corp",
	})
	if err != nil {
		t.Fatalf("CreateTenant failed: %v", err)
	}

	// Duplicate slug
	_, err = store.CreateTenant(ctx, &Tenant{Name: "Acme Duplicate", Slug: "acme-corp"})
	if err != ErrTenantSlugExists {
		t.Errorf("expected ErrTenantSlugExists, got %v", err)
	}

	// GetTenant & GetTenantBySlug
	gotT, err := store.GetTenant(ctx, tenant.ID)
	if err != nil || gotT.Name != "Acme Corp" {
		t.Fatalf("GetTenant failed: %v", err)
	}
	gotSlug, err := store.GetTenantBySlug(ctx, "acme-corp")
	if err != nil || gotSlug.ID != tenant.ID {
		t.Fatalf("GetTenantBySlug failed: %v", err)
	}

	tenants, err := store.ListTenants(ctx)
	if err != nil || len(tenants) < 2 {
		t.Fatalf("ListTenants failed: %v", err)
	}

	// 2. Users & Memberships
	user, err := store.CreateUser(ctx, &User{
		Email: "alice@acme.corp",
		Name:  "Alice",
	})
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}

	// Duplicate email
	_, err = store.CreateUser(ctx, &User{Email: "alice@acme.corp"})
	if err != ErrUserEmailExists {
		t.Errorf("expected ErrUserEmailExists, got %v", err)
	}

	byEmail, err := store.GetUserByEmail(ctx, "alice@acme.corp")
	if err != nil || byEmail.ID != user.ID {
		t.Fatalf("GetUserByEmail failed: %v", err)
	}

	_, err = store.CreateMembership(ctx, &Membership{
		TenantID: tenant.ID,
		UserID:   user.ID,
		Role:     RoleAdmin,
	})
	if err != nil {
		t.Fatalf("CreateMembership failed: %v", err)
	}

	gotMem, err := store.GetMembership(ctx, tenant.ID, user.ID)
	if err != nil || gotMem.Role != RoleAdmin {
		t.Fatalf("GetMembership failed: %v", err)
	}

	mems, err := store.ListMemberships(ctx, tenant.ID)
	if err != nil || len(mems) != 1 {
		t.Fatalf("ListMemberships failed: %v", err)
	}

	err = store.UpdateMembershipRole(ctx, tenant.ID, user.ID, RoleOperator)
	if err != nil {
		t.Fatalf("UpdateMembershipRole failed: %v", err)
	}

	// 3. Projects
	proj, err := store.CreateProject(ctx, &Project{
		TenantID: tenant.ID,
		Name:     "Project Alpha",
	})
	if err != nil {
		t.Fatalf("CreateProject failed: %v", err)
	}

	gotProj, err := store.GetProject(ctx, tenant.ID, proj.ID)
	if err != nil || gotProj.Name != "Project Alpha" {
		t.Fatalf("GetProject failed: %v", err)
	}

	projs, err := store.ListProjects(ctx, tenant.ID)
	if err != nil || len(projs) != 1 {
		t.Fatalf("ListProjects failed: %v", err)
	}

	if err := store.DeleteProject(ctx, tenant.ID, proj.ID); err != nil {
		t.Fatalf("DeleteProject failed: %v", err)
	}

	// 4. API Tokens
	exp := time.Now().UTC().Add(1 * time.Hour)
	tok, err := store.CreateAPIToken(ctx, &APIToken{
		TenantID:  tenant.ID,
		Name:      "CI Token",
		TokenHash: "hash-12345",
		ExpiresAt: &exp,
	})
	if err != nil {
		t.Fatalf("CreateAPIToken failed: %v", err)
	}

	gotTok, err := store.GetAPITokenByHash(ctx, "hash-12345")
	if err != nil || gotTok.ID != tok.ID {
		t.Fatalf("GetAPITokenByHash failed: %v", err)
	}

	if err := store.TouchAPIToken(ctx, tok.ID); err != nil {
		t.Fatalf("TouchAPIToken failed: %v", err)
	}

	toks, err := store.ListAPITokens(ctx, tenant.ID)
	if err != nil || len(toks) != 1 {
		t.Fatalf("ListAPITokens failed: %v", err)
	}

	if err := store.RevokeAPIToken(ctx, tenant.ID, tok.ID); err != nil {
		t.Fatalf("RevokeAPIToken failed: %v", err)
	}

	// 5. Audit Logs
	err = store.RecordAuditLog(ctx, &AuditLog{
		TenantID: tenant.ID,
		ActorID:  user.ID,
		Action:   "workflow.created",
	})
	if err != nil {
		t.Fatalf("RecordAuditLog failed: %v", err)
	}

	logs, err := store.ListAuditLogs(ctx, tenant.ID, AuditFilter{Limit: 10})
	if err != nil || len(logs) != 1 {
		t.Fatalf("ListAuditLogs failed: %v", err)
	}

	pruned, err := store.PruneAuditLogs(ctx, tenant.ID, time.Now().UTC().Add(1*time.Minute))
	if err != nil || pruned != 1 {
		t.Fatalf("PruneAuditLogs failed: %v, pruned=%d", err, pruned)
	}

	// Cleanup
	if err := store.DeleteMembership(ctx, tenant.ID, user.ID); err != nil {
		t.Fatalf("DeleteMembership failed: %v", err)
	}
	if err := store.DeleteTenant(ctx, tenant.ID); err != nil {
		t.Fatalf("DeleteTenant failed: %v", err)
	}
}
