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
}
