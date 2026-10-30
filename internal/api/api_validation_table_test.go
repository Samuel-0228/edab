package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"kerob/internal/adapters/destinations"
	"kerob/internal/adapters/sources"
	"kerob/internal/core/credentials"
	"kerob/internal/core/routing"
	"kerob/internal/core/webhooks"
	"kerob/internal/core/workflow"
)

func TestAPIValidationTableDriven(t *testing.T) {
	wfStore := workflow.NewMemoryStore()
	whStore := webhooks.NewMemoryStore()
	credStore := credentials.NewMemoryStore(credentials.DeriveKey("test-secret-key-32-bytes-long!"))
	srcStore := sources.NewMemoryStore()
	destStore := destinations.NewMemoryStore()
	routeStore := routing.NewMemoryStore()
	srcReg := sources.NewRegistry()
	destReg := destinations.NewRegistry()

	handlers := NewHandlersFromConfig(Config{
		Workflows:    wfStore,
		WebhookStore: whStore,
		Creds:        credStore,
		Sources:      srcStore,
		Destinations: destStore,
		Routes:       routeStore,
		SourceReg:    srcReg,
		DestReg:      destReg,
	})
	server := NewServer("0", handlers)
	handler := server.Handler()

	tests := []struct {
		name           string
		method         string
		path           string
		body           string
		expectedStatus int
		expectedError  string
	}{
		{
			name:           "Create workflow with invalid JSON",
			method:         http.MethodPost,
			path:           "/api/v1/workflows",
			body:           `{invalid json`,
			expectedStatus: http.StatusBadRequest,
			expectedError:  "invalid json body",
		},
		{
			name:           "Create workflow missing name",
			method:         http.MethodPost,
			path:           "/api/v1/workflows",
			body:           `{"trigger":{"type":"webhook"},"actions":[{"integration":"discord","action_type":"send_message"}]}`,
			expectedStatus: http.StatusBadRequest,
			expectedError:  "workflow name is required",
		},
		{
			name:           "Create workflow missing trigger type",
			method:         http.MethodPost,
			path:           "/api/v1/workflows",
			body:           `{"name":"Invalid Workflow","trigger":{},"actions":[{"integration":"discord","action_type":"send_message"}]}`,
			expectedStatus: http.StatusBadRequest,
			expectedError:  "workflow trigger.type is required",
		},
		{
			name:           "Create workflow missing actions",
			method:         http.MethodPost,
			path:           "/api/v1/workflows",
			body:           `{"name":"No Actions Workflow","trigger":{"type":"webhook"},"actions":[]}`,
			expectedStatus: http.StatusBadRequest,
			expectedError:  "workflow must define at least one action",
		},
		{
			name:           "Create workflow with action missing integration",
			method:         http.MethodPost,
			path:           "/api/v1/workflows",
			body:           `{"name":"Bad Action","trigger":{"type":"webhook"},"actions":[{"action_type":"send_message"}]}`,
			expectedStatus: http.StatusBadRequest,
			expectedError:  "integration is required",
		},
		{
			name:           "Create workflow with unsupported trigger type",
			method:         http.MethodPost,
			path:           "/api/v1/workflows",
			body:           `{"name":"Bad Trigger","trigger":{"type":"bogus_trigger"},"actions":[{"integration":"discord","action_type":"send_message","config":{"content":"hi"}}]}`,
			expectedStatus: http.StatusBadRequest,
			expectedError:  "unsupported trigger type",
		},
		{
			name:           "Create workflow schedule trigger missing cron/interval",
			method:         http.MethodPost,
			path:           "/api/v1/workflows",
			body:           `{"name":"Bad Schedule","trigger":{"type":"schedule","config":{}},"actions":[{"integration":"discord","action_type":"send_message","config":{"content":"hi"}}]}`,
			expectedStatus: http.StatusBadRequest,
			expectedError:  "schedule trigger requires 'cron' or 'interval'",
		},
		{
			name:           "Create workflow with SSRF URL in HTTP action",
			method:         http.MethodPost,
			path:           "/api/v1/workflows",
			body:           `{"name":"SSRF Test","trigger":{"type":"webhook"},"actions":[{"integration":"http","action_type":"request","config":{"url":"http://169.254.169.254/latest/meta-data"}}]}`,
			expectedStatus: http.StatusBadRequest,
			expectedError:  "violates SSRF safety",
		},
		{
			name:           "Create webhook missing name",
			method:         http.MethodPost,
			path:           "/api/v1/webhooks",
			body:           `{"name":""}`,
			expectedStatus: http.StatusBadRequest,
			expectedError:  "webhook name is required",
		},
		{
			name:           "Create webhook invalid JSON",
			method:         http.MethodPost,
			path:           "/api/v1/webhooks",
			body:           `{bad json`,
			expectedStatus: http.StatusBadRequest,
			expectedError:  "invalid json body",
		},
		{
			name:           "Create credential missing name",
			method:         http.MethodPost,
			path:           "/api/v1/credentials",
			body:           `{"name":"","integration":"discord","values":{"token":"abc"}}`,
			expectedStatus: http.StatusBadRequest,
			expectedError:  "credential name is required",
		},
		{
			name:           "Create credential unsupported integration",
			method:         http.MethodPost,
			path:           "/api/v1/credentials",
			body:           `{"name":"test cred","integration":"unknown_service","values":{"token":"abc"}}`,
			expectedStatus: http.StatusBadRequest,
			expectedError:  "unsupported integration",
		},
		{
			name:           "Create credential empty values",
			method:         http.MethodPost,
			path:           "/api/v1/credentials",
			body:           `{"name":"test cred","integration":"discord","values":{}}`,
			expectedStatus: http.StatusBadRequest,
			expectedError:  "credential data map cannot be empty",
		},
		{
			name:           "Create source unregistered adapter type",
			method:         http.MethodPost,
			path:           "/sources",
			body:           `{"name":"test source","adapter_type":"nonexistent_adapter"}`,
			expectedStatus: http.StatusBadRequest,
			expectedError:  "unregistered source adapter type",
		},
		{
			name:           "Create destination unregistered adapter type",
			method:         http.MethodPost,
			path:           "/destinations",
			body:           `{"name":"test dest","adapter_type":"nonexistent_dest"}`,
			expectedStatus: http.StatusBadRequest,
			expectedError:  "unregistered destination adapter type",
		},
		{
			name:           "Create route invalid UUID",
			method:         http.MethodPost,
			path:           "/routes",
			body:           `{"source_id":"invalid-uuid","destination_id":"123"}`,
			expectedStatus: http.StatusBadRequest,
			expectedError:  "must be a valid UUID",
		},
		{
			name:           "Get non-existent workflow returns 404",
			method:         http.MethodGet,
			path:           "/api/v1/workflows/non-existent-uuid",
			body:           "",
			expectedStatus: http.StatusNotFound,
			expectedError:  "workflow not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, strings.NewReader(tt.body))
			if tt.body != "" {
				req.Header.Set("Content-Type", "application/json")
			}
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)

			if w.Code != tt.expectedStatus {
				t.Fatalf("expected status %d, got %d: %s", tt.expectedStatus, w.Code, w.Body.String())
			}

			if tt.expectedError != "" && !strings.Contains(w.Body.String(), tt.expectedError) {
				t.Errorf("expected error %q in response, got %s", tt.expectedError, w.Body.String())
			}
		})
	}
}
