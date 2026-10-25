package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestValidationTable_Workflows(t *testing.T) {
	server, _, _ := setupTestServer()
	handler := server.Handler()

	tests := []struct {
		name          string
		payload       map[string]interface{}
		expectedField string
		expectedMsg   string
	}{
		{
			name: "missing name",
			payload: map[string]interface{}{
				"name": "",
				"trigger": map[string]interface{}{
					"type": "webhook",
				},
				"actions": []map[string]interface{}{
					{"integration": "http", "action_type": "post", "config": map[string]interface{}{"url": "https://example.com"}},
				},
			},
			expectedField: "name",
			expectedMsg:   "required",
		},
		{
			name: "name too long",
			payload: map[string]interface{}{
				"name": strings.Repeat("a", 130),
				"trigger": map[string]interface{}{
					"type": "webhook",
				},
				"actions": []map[string]interface{}{
					{"integration": "http", "action_type": "post", "config": map[string]interface{}{"url": "https://example.com"}},
				},
			},
			expectedField: "name",
			expectedMsg:   "128 characters",
		},
		{
			name: "description too long",
			payload: map[string]interface{}{
				"name":        "valid-name",
				"description": strings.Repeat("d", 520),
				"trigger": map[string]interface{}{
					"type": "webhook",
				},
				"actions": []map[string]interface{}{
					{"integration": "http", "action_type": "post", "config": map[string]interface{}{"url": "https://example.com"}},
				},
			},
			expectedField: "description",
			expectedMsg:   "512 characters",
		},
		{
			name: "invalid trigger type enum",
			payload: map[string]interface{}{
				"name": "valid-name",
				"trigger": map[string]interface{}{
					"type": "carrier_pigeon",
				},
				"actions": []map[string]interface{}{
					{"integration": "http", "action_type": "post", "config": map[string]interface{}{"url": "https://example.com"}},
				},
			},
			expectedField: "trigger.type",
			expectedMsg:   "unsupported trigger type",
		},
		{
			name: "invalid combination: schedule trigger with source_id",
			payload: map[string]interface{}{
				"name": "valid-name",
				"trigger": map[string]interface{}{
					"type":      "schedule",
					"source_id": "src-123",
					"config":    map[string]interface{}{"cron": "0 0 * * *"},
				},
				"actions": []map[string]interface{}{
					{"integration": "http", "action_type": "post", "config": map[string]interface{}{"url": "https://example.com"}},
				},
			},
			expectedField: "trigger.source_id",
			expectedMsg:   "schedule triggers cannot restrict to a source_id",
		},
		{
			name: "schedule trigger missing cron or interval",
			payload: map[string]interface{}{
				"name": "valid-name",
				"trigger": map[string]interface{}{
					"type":   "schedule",
					"config": map[string]interface{}{},
				},
				"actions": []map[string]interface{}{
					{"integration": "http", "action_type": "post", "config": map[string]interface{}{"url": "https://example.com"}},
				},
			},
			expectedField: "trigger.config",
			expectedMsg:   "requires 'cron' or 'interval'",
		},
		{
			name: "empty actions",
			payload: map[string]interface{}{
				"name": "valid-name",
				"trigger": map[string]interface{}{
					"type": "webhook",
				},
				"actions": []map[string]interface{}{},
			},
			expectedField: "actions",
			expectedMsg:   "at least one action",
		},
		{
			name: "invalid action integration enum",
			payload: map[string]interface{}{
				"name": "valid-name",
				"trigger": map[string]interface{}{
					"type": "webhook",
				},
				"actions": []map[string]interface{}{
					{"integration": "telnet", "action_type": "send", "config": map[string]interface{}{}},
				},
			},
			expectedField: "actions[0].integration",
			expectedMsg:   "unsupported action integration",
		},
		{
			name: "empty action_type",
			payload: map[string]interface{}{
				"name": "valid-name",
				"trigger": map[string]interface{}{
					"type": "webhook",
				},
				"actions": []map[string]interface{}{
					{"integration": "http", "action_type": "", "config": map[string]interface{}{"url": "https://example.com"}},
				},
			},
			expectedField: "actions[0].action_type",
			expectedMsg:   "action_type is required",
		},
		{
			name: "http action missing url in config",
			payload: map[string]interface{}{
				"name": "valid-name",
				"trigger": map[string]interface{}{
					"type": "webhook",
				},
				"actions": []map[string]interface{}{
					{"integration": "http", "action_type": "post", "config": map[string]interface{}{"other": "field"}},
				},
			},
			expectedField: "actions[0].config.url",
			expectedMsg:   "http action requires 'url'",
		},
		{
			name: "http action with SSRF loopback violation",
			payload: map[string]interface{}{
				"name": "valid-name",
				"trigger": map[string]interface{}{
					"type": "webhook",
				},
				"actions": []map[string]interface{}{
					{"integration": "http", "action_type": "post", "config": map[string]interface{}{"url": "http://127.0.0.1:8080/internal"}},
				},
			},
			expectedField: "actions[0].config.url",
			expectedMsg:   "SSRF safety",
		},
		{
			name: "nested conditions invalid operator enum",
			payload: map[string]interface{}{
				"name": "valid-name",
				"trigger": map[string]interface{}{
					"type": "webhook",
				},
				"actions": []map[string]interface{}{
					{"integration": "http", "action_type": "post", "config": map[string]interface{}{"url": "https://example.com"}},
				},
				"conditions": map[string]interface{}{
					"field":    "event.level",
					"operator": "is_greater_than_maybe",
					"value":    "error",
				},
			},
			expectedField: "conditions.operator",
			expectedMsg:   "unsupported condition operator",
		},
		{
			name: "nested conditions invalid regex pattern",
			payload: map[string]interface{}{
				"name": "valid-name",
				"trigger": map[string]interface{}{
					"type": "webhook",
				},
				"actions": []map[string]interface{}{
					{"integration": "http", "action_type": "post", "config": map[string]interface{}{"url": "https://example.com"}},
				},
				"conditions": map[string]interface{}{
					"field":    "event.msg",
					"operator": "matches",
					"value":    "[invalid-(regex",
				},
			},
			expectedField: "conditions.value",
			expectedMsg:   "invalid regex pattern",
		},
		{
			name: "nested transformation invalid type enum",
			payload: map[string]interface{}{
				"name": "valid-name",
				"trigger": map[string]interface{}{
					"type": "webhook",
				},
				"actions": []map[string]interface{}{
					{"integration": "http", "action_type": "post", "config": map[string]interface{}{"url": "https://example.com"}},
				},
				"transformations": map[string]interface{}{
					"type": "magical_transform",
				},
			},
			expectedField: "transformations.type",
			expectedMsg:   "unsupported transformation type",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			body, _ := json.Marshal(tc.payload)
			req := httptest.NewRequest(http.MethodPost, "/api/v1/workflows", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("expected status 400 Bad Request, got %d: %s", rec.Code, rec.Body.String())
			}

			var errResp APIError
			if err := json.Unmarshal(rec.Body.Bytes(), &errResp); err != nil {
				t.Fatalf("failed to decode APIError response: %v", err)
			}

			if errResp.Code != "VALIDATION_ERROR" {
				t.Errorf("expected Code 'VALIDATION_ERROR', got %q", errResp.Code)
			}

			found := false
			for _, d := range errResp.Details {
				if d.Field == tc.expectedField && strings.Contains(strings.ToLower(d.Message), strings.ToLower(tc.expectedMsg)) {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("expected field error %q matching %q in details %v", tc.expectedField, tc.expectedMsg, errResp.Details)
			}
		})
	}
}

func TestValidationTable_Sources(t *testing.T) {
	server, _, _ := setupTestServer()
	handler := server.Handler()

	tests := []struct {
		name          string
		payload       map[string]interface{}
		expectedField string
		expectedMsg   string
	}{
		{
			name: "missing source name",
			payload: map[string]interface{}{
				"name":         "",
				"adapter_type": "webhook",
			},
			expectedField: "name",
			expectedMsg:   "required",
		},
		{
			name: "source name exceeds limit",
			payload: map[string]interface{}{
				"name":         strings.Repeat("s", 130),
				"adapter_type": "webhook",
			},
			expectedField: "name",
			expectedMsg:   "128 characters",
		},
		{
			name: "missing adapter_type",
			payload: map[string]interface{}{
				"name":         "test-src",
				"adapter_type": "",
			},
			expectedField: "adapter_type",
			expectedMsg:   "required",
		},
		{
			name: "unregistered adapter_type enum",
			payload: map[string]interface{}{
				"name":         "test-src",
				"adapter_type": "unregistered_external_src",
			},
			expectedField: "adapter_type",
			expectedMsg:   "unregistered",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			body, _ := json.Marshal(tc.payload)
			req := httptest.NewRequest(http.MethodPost, "/api/v1/sources", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("expected status 400 Bad Request, got %d: %s", rec.Code, rec.Body.String())
			}

			var errResp APIError
			if err := json.Unmarshal(rec.Body.Bytes(), &errResp); err != nil {
				t.Fatalf("failed to decode APIError: %v", err)
			}

			if errResp.Code != "VALIDATION_ERROR" {
				t.Errorf("expected Code 'VALIDATION_ERROR', got %q", errResp.Code)
			}

			found := false
			for _, d := range errResp.Details {
				if d.Field == tc.expectedField && strings.Contains(strings.ToLower(d.Message), strings.ToLower(tc.expectedMsg)) {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("expected field error %q matching %q in details %v", tc.expectedField, tc.expectedMsg, errResp.Details)
			}
		})
	}
}

func TestValidationTable_Destinations(t *testing.T) {
	server, _, _ := setupTestServer()
	handler := server.Handler()

	tests := []struct {
		name          string
		payload       map[string]interface{}
		expectedField string
		expectedMsg   string
	}{
		{
			name: "missing destination name",
			payload: map[string]interface{}{
				"name":         "",
				"adapter_type": "stdout",
			},
			expectedField: "name",
			expectedMsg:   "required",
		},
		{
			name: "destination name exceeds limit",
			payload: map[string]interface{}{
				"name":         strings.Repeat("d", 130),
				"adapter_type": "stdout",
			},
			expectedField: "name",
			expectedMsg:   "128 characters",
		},
		{
			name: "missing adapter_type",
			payload: map[string]interface{}{
				"name":         "test-dst",
				"adapter_type": "",
			},
			expectedField: "adapter_type",
			expectedMsg:   "required",
		},
		{
			name: "unregistered destination adapter_type enum",
			payload: map[string]interface{}{
				"name":         "test-dst",
				"adapter_type": "unregistered_external_sink",
			},
			expectedField: "adapter_type",
			expectedMsg:   "unregistered",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			body, _ := json.Marshal(tc.payload)
			req := httptest.NewRequest(http.MethodPost, "/api/v1/destinations", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("expected status 400 Bad Request, got %d: %s", rec.Code, rec.Body.String())
			}

			var errResp APIError
			if err := json.Unmarshal(rec.Body.Bytes(), &errResp); err != nil {
				t.Fatalf("failed to decode APIError: %v", err)
			}

			if errResp.Code != "VALIDATION_ERROR" {
				t.Errorf("expected Code 'VALIDATION_ERROR', got %q", errResp.Code)
			}

			found := false
			for _, d := range errResp.Details {
				if d.Field == tc.expectedField && strings.Contains(strings.ToLower(d.Message), strings.ToLower(tc.expectedMsg)) {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("expected field error %q matching %q in details %v", tc.expectedField, tc.expectedMsg, errResp.Details)
			}
		})
	}
}

func TestValidationTable_Routes(t *testing.T) {
	server, _, _ := setupTestServer()
	handler := server.Handler()

	validUUID := "00000000-0000-0000-0000-000000000001"

	tests := []struct {
		name          string
		payload       map[string]interface{}
		expectedField string
		expectedMsg   string
	}{
		{
			name: "invalid source_id UUID format",
			payload: map[string]interface{}{
				"source_id":      "not-a-uuid",
				"destination_id": validUUID,
			},
			expectedField: "source_id",
			expectedMsg:   "must be a valid UUID",
		},
		{
			name: "invalid destination_id UUID format",
			payload: map[string]interface{}{
				"source_id":      validUUID,
				"destination_id": "not-a-uuid",
			},
			expectedField: "destination_id",
			expectedMsg:   "must be a valid UUID",
		},
		{
			name: "invalid combination: source_id equals destination_id",
			payload: map[string]interface{}{
				"source_id":      validUUID,
				"destination_id": validUUID,
			},
			expectedField: "destination_id",
			expectedMsg:   "cannot be identical",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			body, _ := json.Marshal(tc.payload)
			req := httptest.NewRequest(http.MethodPost, "/api/v1/routes", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("expected status 400 Bad Request, got %d: %s", rec.Code, rec.Body.String())
			}

			var errResp APIError
			if err := json.Unmarshal(rec.Body.Bytes(), &errResp); err != nil {
				t.Fatalf("failed to decode APIError: %v", err)
			}

			if errResp.Code != "VALIDATION_ERROR" {
				t.Errorf("expected Code 'VALIDATION_ERROR', got %q", errResp.Code)
			}

			found := false
			for _, d := range errResp.Details {
				if d.Field == tc.expectedField && strings.Contains(strings.ToLower(d.Message), strings.ToLower(tc.expectedMsg)) {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("expected field error %q matching %q in details %v", tc.expectedField, tc.expectedMsg, errResp.Details)
			}
		})
	}
}

func TestValidationTable_Credentials(t *testing.T) {
	server, _, _ := setupTestServer()
	handler := server.Handler()

	tests := []struct {
		name          string
		payload       map[string]interface{}
		expectedField string
		expectedMsg   string
	}{
		{
			name: "missing name",
			payload: map[string]interface{}{
				"name":        "",
				"integration": "http",
				"values":      map[string]string{"token": "xyz"},
			},
			expectedField: "name",
			expectedMsg:   "required",
		},
		{
			name: "unsupported integration enum",
			payload: map[string]interface{}{
				"name":        "my-cred",
				"integration": "quantum_bridge",
				"values":      map[string]string{"token": "xyz"},
			},
			expectedField: "integration",
			expectedMsg:   "unsupported integration",
		},
		{
			name: "empty data map",
			payload: map[string]interface{}{
				"name":        "my-cred",
				"integration": "http",
				"values":      map[string]string{},
			},
			expectedField: "data",
			expectedMsg:   "cannot be empty",
		},
		{
			name: "empty credential value in data map",
			payload: map[string]interface{}{
				"name":        "my-cred",
				"integration": "http",
				"values":      map[string]string{"api_key": ""},
			},
			expectedField: "data[api_key]",
			expectedMsg:   "cannot be empty",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			body, _ := json.Marshal(tc.payload)
			req := httptest.NewRequest(http.MethodPost, "/api/v1/credentials", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("expected status 400 Bad Request, got %d: %s", rec.Code, rec.Body.String())
			}

			var errResp APIError
			if err := json.Unmarshal(rec.Body.Bytes(), &errResp); err != nil {
				t.Fatalf("failed to decode APIError: %v", err)
			}

			if errResp.Code != "VALIDATION_ERROR" {
				t.Errorf("expected Code 'VALIDATION_ERROR', got %q", errResp.Code)
			}

			found := false
			for _, d := range errResp.Details {
				if d.Field == tc.expectedField && strings.Contains(strings.ToLower(d.Message), strings.ToLower(tc.expectedMsg)) {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("expected field error %q matching %q in details %v", tc.expectedField, tc.expectedMsg, errResp.Details)
			}
		})
	}
}

func TestValidationTable_Webhooks(t *testing.T) {
	server, _, _ := setupTestServer()
	handler := server.Handler()

	tests := []struct {
		name          string
		payload       map[string]interface{}
		expectedField string
		expectedMsg   string
	}{
		{
			name: "missing name",
			payload: map[string]interface{}{
				"name": "",
			},
			expectedField: "name",
			expectedMsg:   "required",
		},
		{
			name: "name too long",
			payload: map[string]interface{}{
				"name": strings.Repeat("w", 130),
			},
			expectedField: "name",
			expectedMsg:   "128 characters",
		},
		{
			name: "negative rate limit",
			payload: map[string]interface{}{
				"name":                  "test-hook",
				"rate_limit_per_minute": -5,
			},
			expectedField: "rate_limit_per_minute",
			expectedMsg:   "between 0 and 100000",
		},
		{
			name: "excessive rate limit",
			payload: map[string]interface{}{
				"name":                  "test-hook",
				"rate_limit_per_minute": 200000,
			},
			expectedField: "rate_limit_per_minute",
			expectedMsg:   "between 0 and 100000",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			body, _ := json.Marshal(tc.payload)
			req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("expected status 400 Bad Request, got %d: %s", rec.Code, rec.Body.String())
			}

			var errResp APIError
			if err := json.Unmarshal(rec.Body.Bytes(), &errResp); err != nil {
				t.Fatalf("failed to decode APIError: %v", err)
			}

			if errResp.Code != "VALIDATION_ERROR" {
				t.Errorf("expected Code 'VALIDATION_ERROR', got %q", errResp.Code)
			}

			found := false
			for _, d := range errResp.Details {
				if d.Field == tc.expectedField && strings.Contains(strings.ToLower(d.Message), strings.ToLower(tc.expectedMsg)) {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("expected field error %q matching %q in details %v", tc.expectedField, tc.expectedMsg, errResp.Details)
			}
		})
	}
}

func TestValidationTable_Events(t *testing.T) {
	server, _, _ := setupTestServer()
	handler := server.Handler()

	tests := []struct {
		name          string
		payload       map[string]interface{}
		expectedField string
		expectedMsg   string
	}{
		{
			name: "missing source_id",
			payload: map[string]interface{}{
				"source_id":         "",
				"external_event_id": "ext-1",
				"payload":           map[string]interface{}{"k": "v"},
			},
			expectedField: "source_id",
			expectedMsg:   "required",
		},
		{
			name: "missing external_event_id",
			payload: map[string]interface{}{
				"source_id":         "src-1",
				"external_event_id": "",
				"payload":           map[string]interface{}{"k": "v"},
			},
			expectedField: "external_event_id",
			expectedMsg:   "required",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			body, _ := json.Marshal(tc.payload)
			req := httptest.NewRequest(http.MethodPost, "/api/v1/events", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("expected status 400 Bad Request, got %d: %s", rec.Code, rec.Body.String())
			}

			var errResp APIError
			if err := json.Unmarshal(rec.Body.Bytes(), &errResp); err != nil {
				t.Fatalf("failed to decode APIError: %v", err)
			}

			if errResp.Code != "VALIDATION_ERROR" {
				t.Errorf("expected Code 'VALIDATION_ERROR', got %q", errResp.Code)
			}

			found := false
			for _, d := range errResp.Details {
				if d.Field == tc.expectedField && strings.Contains(strings.ToLower(d.Message), strings.ToLower(tc.expectedMsg)) {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("expected field error %q matching %q in details %v", tc.expectedField, tc.expectedMsg, errResp.Details)
			}
		})
	}
}
