package errtracker

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestSanitizeString(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "no secrets",
			input:    "regular error message occurred",
			expected: "regular error message occurred",
		},
		{
			name:     "bearer token",
			input:    "request failed: Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.token",
			expected: "request failed: Authorization: Bearer [REDACTED]",
		},
		{
			name:     "github personal access token",
			input:    "git operation failed with ghp_1234567890abcdefghijklmnopqrstuvwxyz",
			expected: "git operation failed with [REDACTED_GITHUB_TOKEN]",
		},
		{
			name:     "telegram bot token",
			input:    "call to https://api.telegram.org/bot123456789:ABCdefGHIjklMNOpqrsTUVwxyz123456789/getMe failed",
			expected: "call to https://api.telegram.org/bot[REDACTED_TELEGRAM_TOKEN]/getMe failed",
		},
		{
			name:     "postgres db connection string with password",
			input:    "connection refused to postgres://kerob_user:superSecretPass123@localhost:5432/kerob",
			expected: "connection refused to postgres://kerob_user:[REDACTED]@localhost:5432/kerob",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := SanitizeString(tc.input)
			if !strings.Contains(got, "[REDACTED") && strings.Contains(tc.input, "Secret") {
				t.Fatalf("expected redacted string, got: %s", got)
			}
			if got != tc.expected {
				t.Errorf("SanitizeString(%q) = %q; want %q", tc.input, got, tc.expected)
			}
		})
	}
}

func TestSanitizeScope(t *testing.T) {
	scope := Scope{
		RequestID:   "req-1",
		WorkflowID:  "wf-1",
		ExecutionID: "exec-1",
		Tags: map[string]string{
			"env":       "production",
			"api_token": "secret-value-123",
		},
		Extra: map[string]interface{}{
			"details":       "failed with token Bearer abc12345678",
			"user_password": "my_plaintext_password",
		},
	}

	clean := SanitizeScope(scope)
	if clean.Tags["api_token"] != "[REDACTED]" {
		t.Errorf("expected api_token to be redacted, got: %s", clean.Tags["api_token"])
	}
	if clean.Extra["user_password"] != "[REDACTED]" {
		t.Errorf("expected user_password to be redacted, got: %v", clean.Extra["user_password"])
	}
	detailsStr, _ := clean.Extra["details"].(string)
	if !strings.Contains(detailsStr, "Bearer [REDACTED]") {
		t.Errorf("expected details to be sanitized, got: %v", clean.Extra["details"])
	}
}

func TestMemoryProvider(t *testing.T) {
	ctx := context.Background()
	provider := NewMemoryProvider()

	if len(provider.Records()) != 0 {
		t.Fatalf("expected empty records")
	}

	testErr := errors.New("something went wrong with Bearer secretToken123")
	provider.CaptureError(ctx, testErr, Scope{
		RequestID:  "req-100",
		WorkflowID: "wf-200",
		Tags: map[string]string{
			"api_key": "top-secret",
		},
	})

	records := provider.Records()
	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}

	record := records[0]
	if strings.Contains(record.Err.Error(), "secretToken123") {
		t.Errorf("secret not redacted from recorded error: %v", record.Err)
	}
	if record.Scope.Tags["api_key"] != "[REDACTED]" {
		t.Errorf("api_key tag not redacted: %v", record.Scope.Tags)
	}
	if record.Scope.RequestID != "req-100" || record.Scope.WorkflowID != "wf-200" {
		t.Errorf("scope metadata lost: %+v", record.Scope)
	}

	provider.Reset()
	if len(provider.Records()) != 0 {
		t.Errorf("expected reset to clear records")
	}
}

func TestNoopAndLogProviders(t *testing.T) {
	ctx := context.Background()

	noop := NewNoopProvider()
	noop.CaptureError(ctx, errors.New("noop error"), Scope{})
	noop.Flush(time.Second)

	logProv := NewLogProvider()
	logProv.CaptureError(ctx, nil, Scope{})
	logProv.CaptureError(ctx, errors.New("log error Bearer secret123"), Scope{RequestID: "r1"})
	logProv.Flush(time.Second)
}

func TestSentryProvider(t *testing.T) {
	ctx := context.Background()

	// Empty DSN returns LogProvider
	p1 := NewSentryProvider("")
	if p1 == nil {
		t.Fatal("expected non-nil provider")
	}

	// Valid format
	p2 := NewSentryProvider("https://public_key@sentry.example.com/12345")
	p2.CaptureError(ctx, errors.New("sentry test error"), Scope{RequestID: "req-1"})
	p2.CaptureError(ctx, nil, Scope{})
	p2.Flush(time.Second)

	// Malformed DSN falls back to LogProvider
	p3 := NewSentryProvider("invalid-dsn-string")
	p3.CaptureError(ctx, errors.New("fallback test"), Scope{})
}

func TestGlobalProvider(t *testing.T) {
	ctx := context.Background()

	mem := NewMemoryProvider()
	SetGlobalProvider(mem)

	CaptureGlobal(ctx, errors.New("global critical error"), Scope{RequestID: "req-global-1"})
	records := mem.Records()
	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}
	if records[0].Scope.RequestID != "req-global-1" {
		t.Errorf("unexpected request ID: %s", records[0].Scope.RequestID)
	}

	SetGlobalProvider(nil)
	CaptureGlobal(ctx, errors.New("noop test"), Scope{})
}
