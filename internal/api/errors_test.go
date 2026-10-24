package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRespondErrorRedactsInternalDetailsAndCorrelatesRequest(t *testing.T) {
	rec := httptest.NewRecorder()
	rec.Header().Set("X-Request-ID", "req-123")

	respondError(rec, http.StatusInternalServerError, "database password=secret should not be exposed")

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}

	var response APIError
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Error != "internal server error" || response.Code != "INTERNAL_ERROR" {
		t.Fatalf("unexpected safe error response: %+v", response)
	}
	if response.RequestID != "req-123" {
		t.Fatalf("expected request id, got %q", response.RequestID)
	}
}
