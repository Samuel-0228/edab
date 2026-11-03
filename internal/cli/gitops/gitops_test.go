package gitops

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"kerob/internal/core/workflow"
	"kerob/pkg/sdk"
)

func TestSecretScanner(t *testing.T) {
	// 1. Safe workflow with template reference and credential_id
	safeWf := &workflow.Workflow{
		Name:    "Safe Workflow",
		Trigger: workflow.TriggerConfig{Type: "webhook"},
		Actions: []workflow.ActionStep{
			{
				ID:           "step_charge",
				Name:         "Charge Stripe",
				Integration:  "stripe",
				ActionType:   "charge",
				CredentialID: "stripe_live_key",
				Config:       json.RawMessage(`{"amount": 1000, "token": "{{ secret.STRIPE_TOKEN }}"}`),
			},
		},
	}
	violationsSafe := ValidateWorkflowSecrets(safeWf)
	if len(violationsSafe) != 0 {
		t.Fatalf("expected 0 violations for safe workflow, got %d: %+v", len(violationsSafe), violationsSafe)
	}

	// 2. Unsafe workflow with plaintext AWS access key
	unsafeAWS := &workflow.Workflow{
		Name:    "Unsafe AWS",
		Trigger: workflow.TriggerConfig{Type: "webhook"},
		Actions: []workflow.ActionStep{
			{
				ID:          "step_s3",
				Name:        "Upload S3",
				Integration: "s3",
				Config:      json.RawMessage(`{"api_key": "AKIAIOSFODNN7EXAMPLE"}`),
			},
		},
	}
	violationsAWS := ValidateWorkflowSecrets(unsafeAWS)
	if len(violationsAWS) == 0 {
		t.Fatal("expected secret violations for plaintext AWS key, got 0")
	}
	if !stringsContains(violationsAWS[0].Reason, "AWS Access Key") {
		t.Fatalf("expected AWS Access Key in reason, got %s", violationsAWS[0].Reason)
	}

	// 3. Unsafe workflow with plaintext GitHub PAT
	unsafeGH := &workflow.Workflow{
		Name:    "Unsafe GitHub",
		Trigger: workflow.TriggerConfig{Type: "webhook"},
		Actions: []workflow.ActionStep{
			{
				ID:          "step_gh",
				Name:        "Create Issue",
				Integration: "github",
				Config:      json.RawMessage(`{"token": "ghp_123456789012345678901234567890123456"}`),
			},
		},
	}
	violationsGH := ValidateWorkflowSecrets(unsafeGH)
	if len(violationsGH) == 0 {
		t.Fatal("expected secret violations for GitHub PAT, got 0")
	}

	// 4. Unsafe workflow with literal secret key
	unsafeKey := &workflow.Workflow{
		Name:    "Unsafe Secret Key",
		Trigger: workflow.TriggerConfig{Type: "webhook"},
		Actions: []workflow.ActionStep{
			{
				ID:          "step_api",
				Name:        "Call API",
				Integration: "http",
				Config:      json.RawMessage(`{"password": "supersecretpassword123"}`),
			},
		},
	}
	violationsKey := ValidateWorkflowSecrets(unsafeKey)
	if len(violationsKey) == 0 {
		t.Fatal("expected violation for hardcoded password field, got 0")
	}
}

func TestWorkflowValidator(t *testing.T) {
	// Missing name & actions
	invalidWf := &workflow.Workflow{}
	errs := ValidateWorkflow(invalidWf)
	if len(errs) == 0 {
		t.Fatal("expected validation errors, got 0")
	}

	// Cyclic dependency
	cyclicWf := &workflow.Workflow{
		Name:    "Cyclic Workflow",
		Trigger: workflow.TriggerConfig{Type: "webhook"},
		Actions: []workflow.ActionStep{
			{
				ID:   "step_a",
				Name: "Step A",
				Compensation: &workflow.ActionStep{
					ID:   "step_a",
					Name: "Step A",
				},
			},
		},
	}
	cyclicErrs := ValidateWorkflow(cyclicWf)
	hasCycle := false
	for _, e := range cyclicErrs {
		if stringsContains(e.Message, "cyclic") || stringsContains(e.Message, "duplicate") {
			hasCycle = true
			break
		}
	}
	if !hasCycle {
		t.Fatalf("expected cycle error, got: %+v", cyclicErrs)
	}
}

func TestWorkflowDiff(t *testing.T) {
	local := &workflow.Workflow{
		ID:      "wf_1",
		Name:    "Order Pipeline v2",
		Enabled: true,
		Trigger: workflow.TriggerConfig{Type: "webhook"},
		Actions: []workflow.ActionStep{
			{ID: "step_charge", Name: "Charge Card"},
			{ID: "step_ship", Name: "Ship Item"},
		},
	}

	// 1. Diff against nil (new workflow)
	diffNew := DiffWorkflow(local, nil)
	if !diffNew.HasChanges || len(diffNew.Changes) == 0 {
		t.Fatalf("expected new workflow diff, got %+v", diffNew)
	}

	// 2. Diff against existing remote
	remote := &workflow.Workflow{
		ID:      "wf_1",
		Name:    "Order Pipeline v1",
		Enabled: true,
		Trigger: workflow.TriggerConfig{Type: "webhook"},
		Actions: []workflow.ActionStep{
			{ID: "step_charge", Name: "Charge Card"},
		},
	}
	diffModified := DiffWorkflow(local, remote)
	if !diffModified.HasChanges {
		t.Fatal("expected changes between v1 and v2, got none")
	}
	if len(diffModified.Changes) < 2 {
		t.Fatalf("expected name and action count changes, got: %+v", diffModified.Changes)
	}
}

func TestGitOpsDeploy(t *testing.T) {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/v1/workflows", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]sdk.Workflow{
			{
				ID:      "wf_existing",
				Name:    "Existing Pipeline",
				Enabled: true,
				Trigger: sdk.TriggerConfig{Type: "webhook"},
				Actions: []sdk.ActionStep{{ID: "s1", Name: "Step 1"}},
			},
		})
	})

	mux.HandleFunc("POST /api/v1/workflows", func(w http.ResponseWriter, r *http.Request) {
		var input sdk.WorkflowCreateInput
		_ = json.NewDecoder(r.Body).Decode(&input)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(sdk.Workflow{
			ID:      "wf_created_new",
			Name:    input.Name,
			Enabled: true,
		})
	})

	mux.HandleFunc("PUT /api/v1/workflows/wf_existing", func(w http.ResponseWriter, r *http.Request) {
		var input sdk.WorkflowCreateInput
		_ = json.NewDecoder(r.Body).Decode(&input)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(sdk.Workflow{
			ID:      "wf_existing",
			Name:    input.Name,
			Enabled: true,
		})
	})

	ts := httptest.NewServer(mux)
	defer ts.Close()

	client, err := sdk.NewClient(ts.URL)
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}

	tmpDir := t.TempDir()

	// File 1: New workflow
	newWfContent := `{
		"name": "Brand New Pipeline",
		"enabled": true,
		"trigger": {"type": "webhook"},
		"actions": [{"id": "s1", "name": "Step One", "integration": "http", "action_type": "request"}]
	}`
	_ = os.WriteFile(filepath.Join(tmpDir, "wf_new.json"), []byte(newWfContent), 0644)

	// File 2: Existing modified workflow (YAML)
	existingWfContent := `
name: "Existing Pipeline"
enabled: true
trigger:
  type: "webhook"
actions:
  - id: "s1"
    name: "Step 1 Modified"
    integration: "http"
    action_type: "request"
  - id: "s2"
    name: "Step 2"
    integration: "http"
    action_type: "request"
`
	_ = os.WriteFile(filepath.Join(tmpDir, "wf_existing.yaml"), []byte(existingWfContent), 0644)

	// 1. Dry-run deployment
	dryReport, err := DeployPath(context.Background(), client, tmpDir, DeployOptions{DryRun: true})
	if err != nil {
		t.Fatalf("DeployPath dry-run failed: %v", err)
	}
	if !dryReport.DryRun || len(dryReport.Workflows) != 2 {
		t.Fatalf("unexpected dry-run report: %+v", dryReport)
	}

	// 2. Actual deployment
	liveReport, err := DeployPath(context.Background(), client, tmpDir, DeployOptions{DryRun: false})
	if err != nil {
		t.Fatalf("DeployPath live failed: %v", err)
	}
	if !liveReport.Success {
		t.Fatalf("expected success, got: %+v", liveReport)
	}
	if len(liveReport.Workflows) != 2 {
		t.Fatalf("expected 2 deployed workflows, got %d", len(liveReport.Workflows))
	}
}

func stringsContains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 || (len(s) > 0 && len(substr) > 0 && (s[:len(substr)] == substr || stringsContains(s[1:], substr))))
}
