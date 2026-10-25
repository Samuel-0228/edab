package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"kerob/internal/core/workflow"
)

func TestExecutionsAPI(t *testing.T) {
	server, _, cfg := setupTestServer()
	handler := server.Handler()
	ctx := context.Background()

	// Seed executions
	exec1, err := cfg.Executions.Create(ctx, &workflow.Execution{
		WorkflowID:  "wf-1",
		EventID:     "ev-1",
		Status:      workflow.ExecutionSuccess,
		Attempts:    1,
		MaxAttempts: 3,
	})
	if err != nil {
		t.Fatalf("failed to seed execution: %v", err)
	}

	exec2, _ := cfg.Executions.Create(ctx, &workflow.Execution{
		WorkflowID:  "wf-2",
		EventID:     "ev-2",
		Status:      workflow.ExecutionFailed,
		Attempts:    3,
		MaxAttempts: 3,
	})

	// 1. List all executions
	reqList := httptest.NewRequest(http.MethodGet, "/api/v1/executions", nil)
	wList := httptest.NewRecorder()
	handler.ServeHTTP(wList, reqList)
	if wList.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", wList.Code)
	}
	var list []*workflow.Execution
	_ = json.Unmarshal(wList.Body.Bytes(), &list)
	if len(list) != 2 {
		t.Errorf("expected 2 executions, got %d", len(list))
	}

	// 2. Filter executions by status
	reqFilter := httptest.NewRequest(http.MethodGet, "/api/v1/executions?status=failed&workflow_id=wf-2&limit=10&offset=0", nil)
	wFilter := httptest.NewRecorder()
	handler.ServeHTTP(wFilter, reqFilter)
	if wFilter.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", wFilter.Code)
	}
	var filtered []*workflow.Execution
	_ = json.Unmarshal(wFilter.Body.Bytes(), &filtered)
	if len(filtered) != 1 || filtered[0].ID != exec2.ID {
		t.Errorf("expected filtered execution %s, got %+v", exec2.ID, filtered)
	}

	// 3. Get execution by ID
	reqGet := httptest.NewRequest(http.MethodGet, "/api/v1/executions/"+exec1.ID, nil)
	wGet := httptest.NewRecorder()
	handler.ServeHTTP(wGet, reqGet)
	if wGet.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", wGet.Code)
	}

	// 4. Get non-existent execution -> 404
	req404 := httptest.NewRequest(http.MethodGet, "/api/v1/executions/non-existent-id", nil)
	w404 := httptest.NewRecorder()
	handler.ServeHTTP(w404, req404)
	if w404.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w404.Code)
	}

	// 5. Retry execution
	reqRetry := httptest.NewRequest(http.MethodPost, "/api/v1/executions/"+exec2.ID+"/retry", nil)
	wRetry := httptest.NewRecorder()
	handler.ServeHTTP(wRetry, reqRetry)
	if wRetry.Code != http.StatusOK {
		t.Fatalf("expected 200 for retry, got %d: %s", wRetry.Code, wRetry.Body.String())
	}

	// 6. Retry non-existent execution -> 404
	reqRetry404 := httptest.NewRequest(http.MethodPost, "/api/v1/executions/non-existent-id/retry", nil)
	wRetry404 := httptest.NewRecorder()
	handler.ServeHTTP(wRetry404, reqRetry404)
	if wRetry404.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for retry non-existent, got %d", wRetry404.Code)
	}
}
