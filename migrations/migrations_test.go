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

	var hasUpSQL bool
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasSuffix(name, ".up.sql") {
			hasUpSQL = true
			content, err := fs.ReadFile(FS, name)
			if err != nil {
				t.Fatalf("failed to read migration file %s: %v", name, err)
			}
			if len(content) == 0 {
				t.Errorf("migration file %s is unexpectedly empty", name)
			}
			// Verify key tables are defined
			str := string(content)
			requiredTables := []string{"events", "sources", "destinations", "routes", "delivery_jobs"}
			for _, table := range requiredTables {
				if !strings.Contains(str, table) {
					t.Errorf("expected migration %s to contain table '%s'", name, table)
				}
			}
		}
	}

	if !hasUpSQL {
		t.Error("expected at least one .up.sql migration file embedded")
	}
}
