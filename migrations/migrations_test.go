package migrations

import (
	"io/fs"
	"strings"
	"testing"
)

func TestEmbeddedMigrations(t *testing.T) {
	entries, err := fs.ReadDir(FS, ".")
	if err != nil {
		t.Fatalf("failed to read embedded migrations: %v", err)
	}

	if len(entries) == 0 {
		t.Fatal("expected embedded migration files, got 0")
	}

	upMigrations := make(map[string]string)
	downMigrations := make(map[string]string)

	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() {
			continue
		}
		content, err := fs.ReadFile(FS, name)
		if err != nil {
			t.Fatalf("failed to read migration file %s: %v", name, err)
		}
		if len(content) == 0 {
			t.Errorf("migration file %s is unexpectedly empty", name)
		}

		if strings.HasSuffix(name, ".up.sql") {
			upMigrations[name] = string(content)
		} else if strings.HasSuffix(name, ".down.sql") {
			downMigrations[name] = string(content)
		}
	}

	if len(upMigrations) == 0 {
		t.Error("expected at least one .up.sql migration file embedded")
	}

	// Verify 000001 defines foundational tables
	initUp, ok := upMigrations["000001_init.up.sql"]
	if !ok {
		t.Fatal("000001_init.up.sql is required")
	}
	initRequired := []string{"sources", "destinations", "routes", "events", "delivery_jobs"}
	for _, tbl := range initRequired {
		if !strings.Contains(initUp, tbl) {
			t.Errorf("expected 000001_init.up.sql to contain table '%s'", tbl)
		}
	}

	// Verify 000002 defines workflow & integration tables
	wfUp, ok := upMigrations["000002_workflows_and_integrations.up.sql"]
	if !ok {
		t.Fatal("000002_workflows_and_integrations.up.sql is required")
	}
	wfRequired := []string{"workflows", "executions", "execution_steps", "credentials", "dead_letter", "webhooks"}
	for _, tbl := range wfRequired {
		if !strings.Contains(wfUp, tbl) {
			t.Errorf("expected 000002_workflows_and_integrations.up.sql to contain table '%s'", tbl)
		}
	}

	// Verify 000003 defines engine v2 enhancements
	v2Up, ok := upMigrations["000003_workflow_engine_v2.up.sql"]
	if !ok {
		t.Fatal("000003_workflow_engine_v2.up.sql is required")
	}
	v2Required := []string{"workflow_versions", "current_step_id", "step_states", "priority", "rate_limit_per_minute"}
	for _, kw := range v2Required {
		if !strings.Contains(v2Up, kw) {
			t.Errorf("expected 000003_workflow_engine_v2.up.sql to contain '%s'", kw)
		}
	}

	// Verify 000004 defines distributed execution leases
	v4Up, ok := upMigrations["000004_distributed_execution.up.sql"]
	if !ok {
		t.Fatal("000004_distributed_execution.up.sql is required")
	}
	v4Required := []string{"locked_by", "lease_expires_at", "idx_delivery_jobs_lease", "idx_executions_lease"}
	for _, kw := range v4Required {
		if !strings.Contains(v4Up, kw) {
			t.Errorf("expected 000004_distributed_execution.up.sql to contain '%s'", kw)
		}
	}

	// Verify 000005 defines multi-tenancy, RBAC, API tokens, and audit logs
	v5Up, ok := upMigrations["000005_multi_tenancy_and_rbac.up.sql"]
	if !ok {
		t.Fatal("000005_multi_tenancy_and_rbac.up.sql is required")
	}
	v5Required := []string{"tenants", "users", "memberships", "projects", "api_tokens", "audit_logs", "tenant_id"}
	for _, kw := range v5Required {
		if !strings.Contains(v5Up, kw) {
			t.Errorf("expected 000005_multi_tenancy_and_rbac.up.sql to contain '%s'", kw)
		}
	}

	// Verify 000006 defines event streaming, schemas, and dead-letter queue
	v6Up, ok := upMigrations["000006_event_streaming_and_schemas.up.sql"]
	if !ok {
		t.Fatal("000006_event_streaming_and_schemas.up.sql is required")
	}
	v6Required := []string{"event_schemas", "stream_consumer_offsets", "dead_letter_events", "schema_type", "compatibility", "offset_val"}
	for _, kw := range v6Required {
		if !strings.Contains(v6Up, kw) {
			t.Errorf("expected 000006_event_streaming_and_schemas.up.sql to contain '%s'", kw)
		}
	}

	// Verify 000007 defines sagas, compensation, and human approvals
	v7Up, ok := upMigrations["000007_sagas_compensation_and_approvals.up.sql"]
	if !ok {
		t.Fatal("000007_sagas_compensation_and_approvals.up.sql is required")
	}
	v7Required := []string{"workflow_approvals", "token_hash", "approver_roles", "timeout_action", "escalation_roles", "expires_at"}
	for _, kw := range v7Required {
		if !strings.Contains(v7Up, kw) {
			t.Errorf("expected 000007_sagas_compensation_and_approvals.up.sql to contain '%s'", kw)
		}
	}

	// Verify 000008 defines governance policies and audit log chaining
	v8Up, ok := upMigrations["000008_governance_and_policy_engine.up.sql"]
	if !ok {
		t.Fatal("000008_governance_and_policy_engine.up.sql is required")
	}
	v8Required := []string{"governance_policies", "allowed_connectors", "allowed_outbound_domains", "max_workflow_retries", "max_payload_size_bytes", "sequence_num", "prev_hash", "entry_hash"}
	for _, kw := range v8Required {
		if !strings.Contains(v8Up, kw) {
			t.Errorf("expected 000008_governance_and_policy_engine.up.sql to contain '%s'", kw)
		}
	}
}
