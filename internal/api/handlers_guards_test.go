package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNilStoreGuards(t *testing.T) {
	// Handlers with empty/nil stores must handle requests gracefully without panic
	emptyHandlers := &Handlers{}
	server := NewServer("0", emptyHandlers)
	handler := server.Handler()

	endpoints := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/v1/workflows"},
		{http.MethodGet, "/api/v1/executions"},
		{http.MethodGet, "/api/v1/executions/123"},
		{http.MethodPost, "/api/v1/executions/123/retry"},
		{http.MethodGet, "/api/v1/failures"},
		{http.MethodGet, "/api/v1/failures/123"},
		{http.MethodPost, "/api/v1/failures/123/retry"},
		{http.MethodPost, "/api/v1/failures/123/discard"},
		{http.MethodGet, "/api/v1/webhooks"},
		{http.MethodGet, "/api/v1/webhooks/123"},
		{http.MethodGet, "/api/v1/credentials"},
		{http.MethodGet, "/api/v1/credentials/123"},
		{http.MethodGet, "/api/v1/events"},
		{http.MethodGet, "/api/v1/events/123"},
		{http.MethodGet, "/sources"},
		{http.MethodGet, "/sources/123"},
		{http.MethodGet, "/destinations"},
		{http.MethodGet, "/destinations/123"},
		{http.MethodGet, "/routes"},
		{http.MethodGet, "/routes/123"},
		{http.MethodGet, "/deliveries"},
		{http.MethodGet, "/deliveries/failed"},
		{http.MethodGet, "/deliveries/stats"},
		{http.MethodGet, "/deliveries/123"},
		{http.MethodPost, "/deliveries/123/retry"},
	}

	for _, ep := range endpoints {
		req := httptest.NewRequest(ep.method, ep.path, nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		if w.Code != http.StatusOK && w.Code != http.StatusInternalServerError {
			t.Errorf("expected 200 or 500 for %s %s with nil store, got %d", ep.method, ep.path, w.Code)
		}
	}
}
