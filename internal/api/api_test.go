package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"kerob/internal/adapters/destinations"
	"kerob/internal/adapters/sources"
)

func TestHealthAndReadyEndpoints(t *testing.T) {
	handlers := &Handlers{
		sourceReg: sources.NewRegistry(),
		destReg:   destinations.NewRegistry(),
	}
	server := NewServer("0", handlers)

	handler := server.Handler()

	// 1. Health check
	reqHealth := httptest.NewRequest(http.MethodGet, "/health", nil)
	wHealth := httptest.NewRecorder()
	handler.ServeHTTP(wHealth, reqHealth)

	if wHealth.Code != http.StatusOK {
		t.Errorf("expected /health status 200, got %d", wHealth.Code)
	}

	var healthBody map[string]string
	if err := json.Unmarshal(wHealth.Body.Bytes(), &healthBody); err != nil || healthBody["status"] != "healthy" {
		t.Errorf("unexpected health response: %v", wHealth.Body.String())
	}

	// 2. Ready check
	reqReady := httptest.NewRequest(http.MethodGet, "/ready", nil)
	wReady := httptest.NewRecorder()
	handler.ServeHTTP(wReady, reqReady)

	if wReady.Code != http.StatusOK {
		t.Errorf("expected /ready status 200, got %d", wReady.Code)
	}

	var readyBody map[string]any
	if err := json.Unmarshal(wReady.Body.Bytes(), &readyBody); err != nil || readyBody["status"] != "ready" {
		t.Errorf("unexpected ready response: %v", wReady.Body.String())
	}

	// 3. Not Found check
	reqNotFound := httptest.NewRequest(http.MethodGet, "/unregistered/path", nil)
	wNotFound := httptest.NewRecorder()
	handler.ServeHTTP(wNotFound, reqNotFound)

	if wNotFound.Code != http.StatusNotFound {
		t.Errorf("expected 404 for unknown route, got %d", wNotFound.Code)
	}
}

func TestRespondJSONAndErrorHelpers(t *testing.T) {
	// respondJSON
	w1 := httptest.NewRecorder()
	respondJSON(w1, http.StatusAccepted, map[string]string{"foo": "bar"})
	if w1.Code != http.StatusAccepted {
		t.Errorf("expected status 202, got %d", w1.Code)
	}

	if w1.Header().Get("Content-Type") != "application/json" {
		t.Errorf("expected application/json header, got %s", w1.Header().Get("Content-Type"))
	}

	// respondError
	w2 := httptest.NewRecorder()
	respondError(w2, http.StatusBadRequest, "invalid payload format")
	if w2.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w2.Code)
	}
	var errResp map[string]string
	if err := json.Unmarshal(w2.Body.Bytes(), &errResp); err != nil || errResp["error"] != "invalid payload format" {
		t.Errorf("unexpected error payload: %v", w2.Body.String())
	}
}
