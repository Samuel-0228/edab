package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"kerob/internal/core/workflow"
)

func TestFailuresAPI(t *testing.T) {
	server, _, cfg := setupTestServer()
	handler := server.Handler()
	ctx := context.Background()

	// Seed an execution first so retry finds it
	_, err := cfg.Executions.Create(ctx, &workflow.Execution{
		ID:          "exec-1",
		WorkflowID:  "wf-1",
		EventID:     "ev-1",
		Status:      workflow.ExecutionFailed,
		Attempts:    3,
		MaxAttempts: 3,
	})
	if err != nil {
		t.Fatalf("failed to seed execution: %v", err)
	}

	// Seed dead-letter record
	rec, err := cfg.DeadLetters.Record(ctx, &workflow.DeadLetterRecord{
		ExecutionID: "exec-1",
		WorkflowID:  "wf-1",
		EventID:     "ev-1",
		Error:       "connection reset by peer",
	})
	if err != nil {
		t.Fatalf("failed to record dead letter: %v", err)
	}

	// 1. List failures
	reqList := httptest.NewRequest(http.MethodGet, "/api/v1/failures", nil)
	wList := httptest.NewRecorder()
	handler.ServeHTTP(wList, reqList)
	if wList.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", wList.Code)
	}
	var list []*workflow.DeadLetterRecord
	_ = json.Unmarshal(wList.Body.Bytes(), &list)
	if len(list) != 1 {
		t.Fatalf("expected 1 failure record, got %d", len(list))
	}

	// 2. Get failure by ID
	reqGet := httptest.NewRequest(http.MethodGet, "/api/v1/failures/"+rec.ID, nil)
	wGet := httptest.NewRecorder()
	handler.ServeHTTP(wGet, reqGet)
	if wGet.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", wGet.Code)
	}

	// 3. Get non-existent failure -> 404
	req404 := httptest.NewRequest(http.MethodGet, "/api/v1/failures/non-existent-id", nil)
	w404 := httptest.NewRecorder()
	handler.ServeHTTP(w404, req404)
	if w404.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w404.Code)
	}

	// 4. Retry failure
	reqRetry := httptest.NewRequest(http.MethodPost, "/api/v1/failures/"+rec.ID+"/retry", nil)
	wRetry := httptest.NewRecorder()
	handler.ServeHTTP(wRetry, reqRetry)
	if wRetry.Code != http.StatusOK {
		t.Fatalf("expected 200 for retry failure, got %d: %s", wRetry.Code, wRetry.Body.String())
	}

	// 5. Discard failure
	reqDiscard := httptest.NewRequest(http.MethodPost, "/api/v1/failures/"+rec.ID+"/discard", nil)
	wDiscard := httptest.NewRecorder()
	handler.ServeHTTP(wDiscard, reqDiscard)
	if wDiscard.Code != http.StatusOK {
		t.Fatalf("expected 200 for discard failure, got %d: %s", wDiscard.Code, wDiscard.Body.String())
	}

	// 6. Discard non-existent failure -> 404
	reqDiscard404 := httptest.NewRequest(http.MethodPost, "/api/v1/failures/non-existent-id/discard", nil)
	wDiscard404 := httptest.NewRecorder()
	handler.ServeHTTP(wDiscard404, reqDiscard404)
	if wDiscard404.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for discard non-existent, got %d", wDiscard404.Code)
	}
}
