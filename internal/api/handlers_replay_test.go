package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"kerob/internal/core/conditions"
	"kerob/internal/core/event"
	"kerob/internal/core/transformations"
	"kerob/internal/core/workflow"
)

func TestWorkflowVersionsAndRollbackAPI(t *testing.T) {
	server, _, cfg := setupTestServer()
	handler := server.Handler()
	ctx := context.Background()

	// 1. Create a workflow via POST
	createBody := `{
		"name": "Versioned Workflow",
		"enabled": true,
		"trigger": {"type": "webhook"},
		"actions": [{"integration": "discord", "action_type": "send_message", "config": {"content": "v1"}}]
	}`
	reqCreate := httptest.NewRequest(http.MethodPost, "/api/v1/workflows", strings.NewReader(createBody))
	wCreate := httptest.NewRecorder()
	handler.ServeHTTP(wCreate, reqCreate)
	if wCreate.Code != http.StatusCreated {
		t.Fatalf("expected 201 created, got %d: %s", wCreate.Code, wCreate.Body.String())
	}

	var createdWf workflow.Workflow
	if err := json.Unmarshal(wCreate.Body.Bytes(), &createdWf); err != nil {
		t.Fatalf("failed to decode created workflow: %v", err)
	}

	// 2. Update the workflow (creates version 2)
	updateBody := `{
		"name": "Versioned Workflow Updated",
		"enabled": true,
		"trigger": {"type": "webhook"},
		"actions": [{"integration": "discord", "action_type": "send_message", "config": {"content": "v2"}}]
	}`
	reqUpdate := httptest.NewRequest(http.MethodPut, "/api/v1/workflows/"+createdWf.ID, strings.NewReader(updateBody))
	wUpdate := httptest.NewRecorder()
	handler.ServeHTTP(wUpdate, reqUpdate)
	if wUpdate.Code != http.StatusOK {
		t.Fatalf("expected 200 on update, got %d: %s", wUpdate.Code, wUpdate.Body.String())
	}

	// 3. List versions
	reqListVer := httptest.NewRequest(http.MethodGet, "/api/v1/workflows/"+createdWf.ID+"/versions", nil)
	wListVer := httptest.NewRecorder()
	handler.ServeHTTP(wListVer, reqListVer)
	if wListVer.Code != http.StatusOK {
		t.Fatalf("expected 200 listing versions, got %d", wListVer.Code)
	}

	var versions []*workflow.WorkflowVersion
	if err := json.Unmarshal(wListVer.Body.Bytes(), &versions); err != nil {
		t.Fatalf("failed to decode versions: %v", err)
	}
	if len(versions) < 2 {
		t.Fatalf("expected at least 2 versions, got %d", len(versions))
	}

	// 4. Get specific version (version 1)
	reqGetVer := httptest.NewRequest(http.MethodGet, "/api/v1/workflows/"+createdWf.ID+"/versions/1", nil)
	wGetVer := httptest.NewRecorder()
	handler.ServeHTTP(wGetVer, reqGetVer)
	if wGetVer.Code != http.StatusOK {
		t.Fatalf("expected 200 getting version 1, got %d", wGetVer.Code)
	}

	// 5. Diff versions 1 and 2
	reqDiff := httptest.NewRequest(http.MethodGet, "/api/v1/workflows/"+createdWf.ID+"/diff?from=1&to=2", nil)
	wDiff := httptest.NewRecorder()
	handler.ServeHTTP(wDiff, reqDiff)
	if wDiff.Code != http.StatusOK {
		t.Fatalf("expected 200 diffing versions, got %d", wDiff.Code)
	}

	// 6. Rollback to version 1
	reqRollback := httptest.NewRequest(http.MethodPost, "/api/v1/workflows/"+createdWf.ID+"/rollback/1", nil)
	wRollback := httptest.NewRecorder()
	handler.ServeHTTP(wRollback, reqRollback)
	if wRollback.Code != http.StatusOK {
		t.Fatalf("expected 200 rollbacking to version 1, got %d: %s", wRollback.Code, wRollback.Body.String())
	}

	// Verify current state is restored to "Versioned Workflow"
	restored, err := cfg.Workflows.GetByID(ctx, createdWf.ID)
	if err != nil {
		t.Fatalf("failed to get workflow: %v", err)
	}
	if restored.Name != "Versioned Workflow" {
		t.Errorf("expected rolled back name 'Versioned Workflow', got %s", restored.Name)
	}

	// 7. Export workflow JSON
	reqExportJSON := httptest.NewRequest(http.MethodGet, "/api/v1/workflows/"+createdWf.ID+"/export", nil)
	wExportJSON := httptest.NewRecorder()
	handler.ServeHTTP(wExportJSON, reqExportJSON)
	if wExportJSON.Code != http.StatusOK {
		t.Fatalf("expected 200 exporting json, got %d", wExportJSON.Code)
	}

	// 8. Export workflow YAML
	reqExportYAML := httptest.NewRequest(http.MethodGet, "/api/v1/workflows/"+createdWf.ID+"/export?format=yaml", nil)
	wExportYAML := httptest.NewRecorder()
	handler.ServeHTTP(wExportYAML, reqExportYAML)
	if wExportYAML.Code != http.StatusOK {
		t.Fatalf("expected 200 exporting yaml, got %d", wExportYAML.Code)
	}

	// 9. Import workflow
	importJSON := `{
		"name": "Imported Flow",
		"enabled": true,
		"trigger": {"type": "webhook"},
		"actions": [{"integration": "telegram", "action_type": "send_message", "config": {"text": "imported"}}]
	}`
	reqImport := httptest.NewRequest(http.MethodPost, "/api/v1/workflows/import", strings.NewReader(importJSON))
	wImport := httptest.NewRecorder()
	handler.ServeHTTP(wImport, reqImport)
	if wImport.Code != http.StatusCreated {
		t.Fatalf("expected 201 importing workflow, got %d: %s", wImport.Code, wImport.Body.String())
	}

	// 10. List templates
	reqTemplates := httptest.NewRequest(http.MethodGet, "/api/v1/templates", nil)
	wTemplates := httptest.NewRecorder()
	handler.ServeHTTP(wTemplates, reqTemplates)
	if wTemplates.Code != http.StatusOK {
		t.Fatalf("expected 200 listing templates, got %d", wTemplates.Code)
	}
}

func TestEventReplayAndDLQBulkAPI(t *testing.T) {
	server, _, cfg := setupTestServer()
	handler := server.Handler()
	ctx := context.Background()

	// Ingest an event
	ev := &event.Event{
		ID:              "evt-replay-test-1",
		SourceID:        "src-1",
		EventType:       "webhook",
		ExternalEventID: "ext-replay-1",
		Payload:         json.RawMessage(`{"status":"ok","count":42}`),
		CreatedAt:       time.Now().UTC(),
	}
	_, _, _ = cfg.Events.Create(ctx, ev)

	// Replay event without bypass
	reqReplay := httptest.NewRequest(http.MethodPost, "/api/v1/events/"+ev.ID+"/replay", strings.NewReader(`{"bypass_idempotency": false}`))
	wReplay := httptest.NewRecorder()
	handler.ServeHTTP(wReplay, reqReplay)
	if wReplay.Code != http.StatusOK {
		t.Fatalf("expected 200 replaying event, got %d: %s", wReplay.Code, wReplay.Body.String())
	}

	// Replay event with bypass_idempotency
	reqReplayBypass := httptest.NewRequest(http.MethodPost, "/api/v1/events/"+ev.ID+"/replay", strings.NewReader(`{"bypass_idempotency": true}`))
	wReplayBypass := httptest.NewRecorder()
	handler.ServeHTTP(wReplayBypass, reqReplayBypass)
	if wReplayBypass.Code != http.StatusOK {
		t.Fatalf("expected 200 replaying event with bypass, got %d: %s", wReplayBypass.Code, wReplayBypass.Body.String())
	}

	// Replay bulk
	bulkReq := `{"event_ids": ["` + ev.ID + `"], "bypass_idempotency": true}`
	reqBulk := httptest.NewRequest(http.MethodPost, "/api/v1/events/replay-bulk", strings.NewReader(bulkReq))
	wBulk := httptest.NewRecorder()
	handler.ServeHTTP(wBulk, reqBulk)
	if wBulk.Code != http.StatusOK {
		t.Fatalf("expected 200 bulk replaying events, got %d: %s", wBulk.Code, wBulk.Body.String())
	}

	// Create a dead-letter item to test bulk retry / discard
	errMsg := "downstream timeout"
	exec := &workflow.Execution{
		ID:         "exec-dlq-1",
		WorkflowID: "wf-1",
		EventID:    ev.ID,
		Status:     workflow.ExecutionFailed,
		CreatedAt:  time.Now().UTC(),
		Error:      &errMsg,
	}
	_, _ = cfg.Executions.Create(ctx, exec)

	dlRecord := &workflow.DeadLetterRecord{
		ID:          "dlq-rec-1",
		ExecutionID: exec.ID,
		WorkflowID:  "wf-1",
		EventID:     ev.ID,
		Error:       "downstream timeout",
		FailedAt:    time.Now().UTC(),
	}
	_, _ = cfg.DeadLetters.Record(ctx, dlRecord)

	// Bulk retry DLQ
	reqDLQRetry := httptest.NewRequest(http.MethodPost, "/api/v1/dead-letter/retry-bulk", strings.NewReader(`{"ids": ["dlq-rec-1"]}`))
	wDLQRetry := httptest.NewRecorder()
	handler.ServeHTTP(wDLQRetry, reqDLQRetry)
	if wDLQRetry.Code != http.StatusOK {
		t.Fatalf("expected 200 retrying DLQ bulk, got %d: %s", wDLQRetry.Code, wDLQRetry.Body.String())
	}

	// Create another DLQ record to test discard
	dlRecord2 := &workflow.DeadLetterRecord{
		ID:          "dlq-rec-2",
		ExecutionID: exec.ID,
		WorkflowID:  "wf-1",
		EventID:     ev.ID,
		Error:       "unrecoverable error",
		FailedAt:    time.Now().UTC(),
	}
	_, _ = cfg.DeadLetters.Record(ctx, dlRecord2)

	// Bulk discard DLQ
	reqDLQDiscard := httptest.NewRequest(http.MethodPost, "/api/v1/dead-letter/discard-bulk", strings.NewReader(`{"ids": ["dlq-rec-2"]}`))
	wDLQDiscard := httptest.NewRecorder()
	handler.ServeHTTP(wDLQDiscard, reqDLQDiscard)
	if wDLQDiscard.Code != http.StatusOK {
		t.Fatalf("expected 200 discarding DLQ bulk, got %d: %s", wDLQDiscard.Code, wDLQDiscard.Body.String())
	}
}

func TestWorkflowDryRunEndpoint(t *testing.T) {
	server, _, cfg := setupTestServer()
	handler := server.Handler()
	ctx := context.Background()

	wf, err := cfg.Workflows.Create(ctx, &workflow.Workflow{
		Name:    "Dry Run API Workflow",
		Enabled: true,
		Trigger: workflow.TriggerConfig{Type: "webhook"},
		Conditions: &conditions.Condition{
			Field: "payload.level", Operator: "equals", Value: "critical",
		},
		Transformations: &transformations.Transformation{
			Type: "mapping",
			FieldMaps: map[string]string{
				"alert": "Priority: {{payload.level}}",
			},
		},
		Actions: []workflow.ActionStep{
			{
				ID:          "act-1",
				Integration: "telegram",
				ActionType:  "send_message",
				Config:      json.RawMessage(`{"text": "{{transformed.alert}}"}`),
			},
		},
	})
	if err != nil {
		t.Fatalf("failed to create workflow: %v", err)
	}

	// 1. Dry run matching event
	testBodyMatch := `{"event": {"event_type": "webhook", "payload": {"level": "critical"}}}`
	reqMatch := httptest.NewRequest(http.MethodPost, "/api/v1/workflows/"+wf.ID+"/test", strings.NewReader(testBodyMatch))
	wMatch := httptest.NewRecorder()
	handler.ServeHTTP(wMatch, reqMatch)
	if wMatch.Code != http.StatusOK {
		t.Fatalf("expected 200 for matching dry run, got %d: %s", wMatch.Code, wMatch.Body.String())
	}

	var trace workflow.DryRunResult
	if err := json.Unmarshal(wMatch.Body.Bytes(), &trace); err != nil {
		t.Fatalf("failed to parse dry run trace: %v", err)
	}
	if !trace.TriggerMatched || !trace.ConditionsPassed {
		t.Errorf("expected trigger and conditions to pass, got %+v", trace)
	}
	if trace.TransformedPayload["alert"] != "Priority: critical" {
		t.Errorf("unexpected transformed alert: %v", trace.TransformedPayload)
	}

	// 2. Dry run non-matching event
	testBodyMismatch := `{"event": {"event_type": "webhook", "payload": {"level": "info"}}}`
	reqMismatch := httptest.NewRequest(http.MethodPost, "/api/v1/workflows/"+wf.ID+"/test", strings.NewReader(testBodyMismatch))
	wMismatch := httptest.NewRecorder()
	handler.ServeHTTP(wMismatch, reqMismatch)
	if wMismatch.Code != http.StatusOK {
		t.Fatalf("expected 200 for mismatch dry run, got %d: %s", wMismatch.Code, wMismatch.Body.String())
	}

	var traceFail workflow.DryRunResult
	if err := json.Unmarshal(wMismatch.Body.Bytes(), &traceFail); err != nil {
		t.Fatalf("failed to parse dry run trace: %v", err)
	}
	if traceFail.ConditionsPassed {
		t.Errorf("expected conditions to fail for 'info' level")
	}
}
