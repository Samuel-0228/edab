package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"kerob/internal/core/event"
	"kerob/internal/core/schema"
	"kerob/internal/core/streaming"
	"kerob/internal/core/workflow"
)

func TestSchemasAndStreamingAPI(t *testing.T) {
	_, _, cfg := setupTestServer()

	schemaStore := schema.NewMemoryStore()
	schemaReg := schema.NewRegistry(schemaStore)
	streamAdapter := streaming.NewMemoryStreamAdapter(2)
	offsetStore := streaming.NewMemoryOffsetStore()
	dlqStore := streaming.NewMemoryDeadLetterStore()

	handlers := NewHandlersFromConfig(Config{
		Workflows:      cfg.Workflows,
		Executions:     cfg.Executions,
		DeadLetters:    cfg.DeadLetters,
		WorkflowEngine: cfg.WorkflowEngine,
		ActionReg:      cfg.ActionReg,
		Creds:          cfg.Creds,
		WebhookStore:   cfg.WebhookStore,
		Events:         cfg.Events,

		SchemaRegistry: schemaReg,
		StreamAdapter:  streamAdapter,
		StreamOffsets:  offsetStore,
		StreamDLQStore: dlqStore,
	})
	srv := NewServer("0", handlers)

	// 1. POST /api/v1/schemas (Register Schema v1)
	schemaV1 := map[string]any{
		"name":        "order.created",
		"schema_type": "json_schema",
		"definition": `{
			"type": "object",
			"required": ["order_id", "amount"],
			"properties": {
				"order_id": { "type": "string" },
				"amount": { "type": "number" }
			}
		}`,
		"compatibility": "BACKWARD",
	}
	bodyBytes, _ := json.Marshal(schemaV1)
	req1 := httptest.NewRequest(http.MethodPost, "/api/v1/schemas", bytes.NewReader(bodyBytes))
	req1.Header.Set("Content-Type", "application/json")
	rr1 := httptest.NewRecorder()
	srv.mux.ServeHTTP(rr1, req1)
	if rr1.Code != http.StatusCreated {
		t.Fatalf("RegisterSchema returned %d: %s", rr1.Code, rr1.Body.String())
	}
	var createdSc schema.Schema
	_ = json.Unmarshal(rr1.Body.Bytes(), &createdSc)
	if createdSc.Version != 1 {
		t.Fatalf("expected version 1, got %d", createdSc.Version)
	}

	// 2. GET /api/v1/schemas (List)
	req2 := httptest.NewRequest(http.MethodGet, "/api/v1/schemas", nil)
	rr2 := httptest.NewRecorder()
	srv.mux.ServeHTTP(rr2, req2)
	if rr2.Code != http.StatusOK {
		t.Fatalf("ListSchemas returned %d", rr2.Code)
	}
	var schemaList []*schema.Schema
	_ = json.Unmarshal(rr2.Body.Bytes(), &schemaList)
	if len(schemaList) != 1 {
		t.Fatalf("expected 1 schema, got %d", len(schemaList))
	}

	// 3. POST /api/v1/schemas/{name}/validate (Validate payload)
	validateValid := map[string]any{
		"payload": map[string]any{
			"order_id": "ord_999",
			"amount":   149.99,
		},
	}
	valBytes, _ := json.Marshal(validateValid)
	req3 := httptest.NewRequest(http.MethodPost, "/api/v1/schemas/order.created/validate", bytes.NewReader(valBytes))
	req3.Header.Set("Content-Type", "application/json")
	rr3 := httptest.NewRecorder()
	srv.mux.ServeHTTP(rr3, req3)
	if rr3.Code != http.StatusOK {
		t.Fatalf("ValidatePayload returned %d: %s", rr3.Code, rr3.Body.String())
	}
	var valResp map[string]any
	_ = json.Unmarshal(rr3.Body.Bytes(), &valResp)
	if valResp["valid"] != true {
		t.Fatalf("expected valid payload to pass, got: %v", valResp)
	}

	// 4. POST /api/v1/streams/publish (Valid Stream Event)
	streamInput := map[string]any{
		"topic":        "orders",
		"ordering_key": "tenant-xyz",
		"event_type":   "order.created",
		"payload": map[string]any{
			"order_id": "ord_999",
			"amount":   149.99,
		},
	}
	pubBytes, _ := json.Marshal(streamInput)
	req4 := httptest.NewRequest(http.MethodPost, "/api/v1/streams/publish", bytes.NewReader(pubBytes))
	req4.Header.Set("Content-Type", "application/json")
	rr4 := httptest.NewRecorder()
	srv.mux.ServeHTTP(rr4, req4)
	if rr4.Code != http.StatusAccepted {
		t.Fatalf("PublishStream returned %d: %s", rr4.Code, rr4.Body.String())
	}
	var pubRes streaming.PublishResult
	_ = json.Unmarshal(rr4.Body.Bytes(), &pubRes)
	if pubRes.Topic != "orders" {
		t.Fatalf("expected topic 'orders', got %s", pubRes.Topic)
	}

	// 5. POST /api/v1/streams/publish (Invalid Payload Rejected by Schema)
	streamInvalid := map[string]any{
		"topic":        "orders",
		"ordering_key": "tenant-xyz",
		"event_type":   "order.created",
		"payload": map[string]any{
			"amount": "not-a-number",
		},
	}
	invBytes, _ := json.Marshal(streamInvalid)
	req5 := httptest.NewRequest(http.MethodPost, "/api/v1/streams/publish", bytes.NewReader(invBytes))
	req5.Header.Set("Content-Type", "application/json")
	rr5 := httptest.NewRecorder()
	srv.mux.ServeHTTP(rr5, req5)
	if rr5.Code != http.StatusBadRequest {
		t.Fatalf("expected schema validation failure HTTP 400, got %d", rr5.Code)
	}

	// 6. GET /api/v1/streams/offsets
	_ = offsetStore.CommitOffset(context.Background(), streaming.ConsumerOffset{
		GroupID:     "test-group",
		Topic:       "orders",
		Partition:   0,
		Offset:      42,
		CommittedAt: time.Now().UTC(),
	})
	req6 := httptest.NewRequest(http.MethodGet, "/api/v1/streams/offsets?group_id=test-group", nil)
	rr6 := httptest.NewRecorder()
	srv.mux.ServeHTTP(rr6, req6)
	if rr6.Code != http.StatusOK {
		t.Fatalf("GetStreamOffsets returned %d: %s", rr6.Code, rr6.Body.String())
	}
	var offsets []streaming.ConsumerOffset
	_ = json.Unmarshal(rr6.Body.Bytes(), &offsets)
	if len(offsets) != 1 || offsets[0].Offset != 42 {
		t.Fatalf("expected offset 42, got %v", offsets)
	}

	// 7. GET /api/v1/streams/dlq and POST /api/v1/streams/dlq/{id}/retry
	dlqItem, _ := dlqStore.Store(context.Background(), &streaming.DeadLetterEvent{
		Topic:       "orders",
		EventType:   "order.created",
		Payload:     json.RawMessage(`{"order_id": "ord_dlq", "amount": 200}`),
		ErrorReason: "downstream transient error",
	})

	req7 := httptest.NewRequest(http.MethodGet, "/api/v1/streams/dlq?topic=orders", nil)
	rr7 := httptest.NewRecorder()
	srv.mux.ServeHTTP(rr7, req7)
	if rr7.Code != http.StatusOK {
		t.Fatalf("ListStreamDLQ returned %d: %s", rr7.Code, rr7.Body.String())
	}
	var dlqList []*streaming.DeadLetterEvent
	_ = json.Unmarshal(rr7.Body.Bytes(), &dlqList)
	if len(dlqList) != 1 {
		t.Fatalf("expected 1 DLQ event, got %d", len(dlqList))
	}

	retryURL := fmt.Sprintf("/api/v1/streams/dlq/%s/retry", dlqItem.ID)
	req8 := httptest.NewRequest(http.MethodPost, retryURL, nil)
	rr8 := httptest.NewRecorder()
	srv.mux.ServeHTTP(rr8, req8)
	if rr8.Code != http.StatusOK {
		t.Fatalf("RetryStreamDLQ returned %d: %s", rr8.Code, rr8.Body.String())
	}

	// 8. POST /api/v1/streams/replay
	// Setup workflow to receive replayed events
	_, _ = cfg.Workflows.Create(context.Background(), &workflow.Workflow{
		Name:    "Order Created Workflow",
		Enabled: true,
		Trigger: workflow.TriggerConfig{
			Type:       "order.created",
			EventTypes: []string{"order.created"},
		},
		Actions: []workflow.ActionStep{
			{ID: "s1", Name: "Action", Type: "action", Integration: "http", ActionType: "post"},
		},
	})
	evHistorical, _ := event.New("source-test", "ext-hist-1", "order.created", map[string]any{"order_id": "ord_1", "amount": 50}, nil)
	_, _, _ = cfg.Events.Create(context.Background(), evHistorical)

	replayBody, _ := json.Marshal(map[string]any{
		"event_type":         "order.created",
		"bypass_idempotency": true,
	})
	req9 := httptest.NewRequest(http.MethodPost, "/api/v1/streams/replay", bytes.NewReader(replayBody))
	req9.Header.Set("Content-Type", "application/json")
	rr9 := httptest.NewRecorder()
	srv.mux.ServeHTTP(rr9, req9)
	if rr9.Code != http.StatusOK {
		t.Fatalf("ReplayStream returned %d: %s", rr9.Code, rr9.Body.String())
	}
	var repRes streaming.ReplayResult
	_ = json.Unmarshal(rr9.Body.Bytes(), &repRes)
	if repRes.TotalMatched < 1 {
		t.Fatalf("expected at least 1 matched replay event, got %d", repRes.TotalMatched)
	}
}
