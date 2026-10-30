package workflow

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"kerob/internal/core/conditions"
	"kerob/internal/core/credentials"
	"kerob/internal/core/event"
	"kerob/internal/core/transformations"
)

func TestMemoryVersionStore(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryVersionStore()

	wfID := "wf-version-test"
	v1Snap := []byte(`{"id":"wf-version-test","name":"v1","actions":[]}`)
	v2Snap := []byte(`{"id":"wf-version-test","name":"v2","actions":[]}`)

	v1, err := store.RecordVersion(ctx, wfID, v1Snap)
	if err != nil {
		t.Fatalf("unexpected error recording v1: %v", err)
	}
	if v1.Version != 1 {
		t.Errorf("expected version 1, got %d", v1.Version)
	}

	v2, err := store.RecordVersion(ctx, wfID, v2Snap)
	if err != nil {
		t.Fatalf("unexpected error recording v2: %v", err)
	}
	if v2.Version != 2 {
		t.Errorf("expected version 2, got %d", v2.Version)
	}

	versions, err := store.ListVersions(ctx, wfID)
	if err != nil {
		t.Fatalf("unexpected error listing versions: %v", err)
	}
	if len(versions) != 2 {
		t.Fatalf("expected 2 versions, got %d", len(versions))
	}
	if versions[0].Version != 2 || versions[1].Version != 1 {
		t.Errorf("expected descending order (2, 1), got (%d, %d)", versions[0].Version, versions[1].Version)
	}

	gotV1, err := store.GetVersion(ctx, wfID, 1)
	if err != nil {
		t.Fatalf("unexpected error getting v1: %v", err)
	}
	if string(gotV1.Definition) != string(v1Snap) {
		t.Errorf("definition mismatch: got %s, want %s", gotV1.Definition, v1Snap)
	}

	_, err = store.GetVersion(ctx, wfID, 99)
	if err != ErrVersionNotFound {
		t.Errorf("expected ErrVersionNotFound, got %v", err)
	}
}

func TestWorkflowExportImport(t *testing.T) {
	wf := &Workflow{
		ID:          "wf-export-test",
		Name:        "Export Workflow",
		Description: "A workflow for testing export and import",
		Enabled:     true,
		Trigger: TriggerConfig{
			Type: "webhook",
		},
		Conditions: &conditions.Condition{
			Field:    "payload.event",
			Operator: "equals",
			Value:    "deploy",
		},
		Actions: []ActionStep{
			{
				ID:          "step-1",
				Integration: "http",
				ActionType:  "request",
				Config:      json.RawMessage(`{"url": "https://8.8.8.8/notify"}`),
			},
		},
	}

	jsonBytes, err := ExportWorkflowJSON(wf)
	if err != nil {
		t.Fatalf("ExportWorkflowJSON failed: %v", err)
	}

	importedJSON, err := ImportWorkflowJSON(jsonBytes)
	if err != nil {
		t.Fatalf("ImportWorkflowJSON failed: %v", err)
	}
	if importedJSON.Name != wf.Name {
		t.Errorf("imported name mismatch: got %s, want %s", importedJSON.Name, wf.Name)
	}
	if len(importedJSON.Actions) != 1 || importedJSON.Actions[0].ID != "step-1" {
		t.Errorf("imported actions mismatch: %v", importedJSON.Actions)
	}

	yamlBytes, err := ExportWorkflowYAML(wf)
	if err != nil {
		t.Fatalf("ExportWorkflowYAML failed: %v", err)
	}

	importedYAML, err := ImportWorkflowYAML(yamlBytes)
	if err != nil {
		t.Fatalf("ImportWorkflowYAML failed: %v", err)
	}
	if importedYAML.Name != wf.Name {
		t.Errorf("imported YAML name mismatch: got %s, want %s", importedYAML.Name, wf.Name)
	}
	if importedYAML.Description != wf.Description {
		t.Errorf("imported YAML description mismatch: got %s, want %s", importedYAML.Description, wf.Description)
	}
}

func TestBuiltinTemplates(t *testing.T) {
	templates := BuiltinTemplates()
	if len(templates) < 3 {
		t.Fatalf("expected at least 3 builtin templates, got %d", len(templates))
	}

	var foundRSS, foundGitHub bool
	for _, tmpl := range templates {
		if tmpl.ID == "rss_to_telegram" {
			foundRSS = true
			if tmpl.Workflow.Trigger.Type != "rss" {
				t.Errorf("expected rss trigger, got %s", tmpl.Workflow.Trigger.Type)
			}
		}
		if tmpl.ID == "github_to_slack" {
			foundGitHub = true
			if tmpl.Workflow.Trigger.Type != "github" {
				t.Errorf("expected github trigger, got %s", tmpl.Workflow.Trigger.Type)
			}
		}
	}
	if !foundRSS || !foundGitHub {
		t.Errorf("expected to find rss_to_telegram and github_to_slack templates")
	}
}

func setupTestEngine() *Engine {
	wfStore := NewMemoryStore()
	execStore := NewMemoryExecutionStore()
	dlStore := NewMemoryDeadLetterStore()
	credStore := credentials.NewMemoryStore(credentials.DeriveKey("test-dry-run-key-1234567890123"))
	actionReg := NewActionRegistry()
	return NewEngine(wfStore, execStore, dlStore, credStore, actionReg)
}

func TestExecuteDryRun(t *testing.T) {
	ctx := context.Background()
	engine := setupTestEngine()

	wf := &Workflow{
		ID:      "wf-dryrun-test",
		Name:    "Dry Run Test",
		Enabled: true,
		Trigger: TriggerConfig{
			Type: "webhook",
		},
		Conditions: &conditions.Condition{
			Field:    "payload.status",
			Operator: "equals",
			Value:    "active",
		},
		Transformations: &transformations.Transformation{
			Type: "mapping",
			FieldMaps: map[string]string{
				"alert_msg": "System is {{payload.status}} for {{payload.service}}",
			},
		},
		Actions: []ActionStep{
			{
				ID:          "step-1",
				Integration: "http",
				ActionType:  "request",
				Config:      json.RawMessage(`{"url": "https://8.8.8.8/test", "body": "{{transformed.alert_msg}}"}`),
			},
		},
	}

	ev := &event.Event{
		ID:        "evt-dryrun-1",
		SourceID:  "src-1",
		EventType: "webhook",
		Payload:   json.RawMessage(`{"status":"active","service":"billing"}`),
		CreatedAt: time.Now().UTC(),
	}

	trace, err := engine.ExecuteDryRun(ctx, wf, ev, true)
	if err != nil {
		t.Fatalf("ExecuteDryRun failed: %v", err)
	}

	if !trace.TriggerMatched {
		t.Errorf("expected trigger to match")
	}
	if !trace.ConditionsPassed {
		t.Errorf("expected conditions to pass")
	}
	if trace.TransformedPayload == nil {
		t.Fatalf("expected transformed payload, got nil")
	}
	if trace.TransformedPayload["alert_msg"] != "System is active for billing" {
		t.Errorf("unexpected transformed message: %v", trace.TransformedPayload["alert_msg"])
	}
	if len(trace.Traces) == 0 {
		t.Fatalf("expected traces to be populated")
	}
}

func TestExecuteDryRunConditionFailed(t *testing.T) {
	ctx := context.Background()
	engine := setupTestEngine()

	wf := &Workflow{
		ID:      "wf-dryrun-fail",
		Name:    "Dry Run Condition Fail",
		Enabled: true,
		Trigger: TriggerConfig{
			Type: "webhook",
		},
		Conditions: &conditions.Condition{
			Field:    "payload.status",
			Operator: "equals",
			Value:    "inactive",
		},
		Actions: []ActionStep{
			{ID: "act-1", Integration: "http", ActionType: "request", Config: json.RawMessage(`{}`)},
		},
	}

	ev := &event.Event{
		ID:        "evt-dryrun-2",
		EventType: "webhook",
		Payload:   json.RawMessage(`{"status":"active"}`),
	}

	trace, err := engine.ExecuteDryRun(ctx, wf, ev, true)
	if err != nil {
		t.Fatalf("ExecuteDryRun failed: %v", err)
	}
	if trace.ConditionsPassed {
		t.Errorf("expected conditions to fail")
	}
}
