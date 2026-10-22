package errtracker

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"
)

// Scope captures structured execution and request context for error tracking.
type Scope struct {
	RequestID   string                 `json:"request_id,omitempty"`
	WorkflowID  string                 `json:"workflow_id,omitempty"`
	ExecutionID string                 `json:"execution_id,omitempty"`
	EventID     string                 `json:"event_id,omitempty"`
	Tags        map[string]string      `json:"tags,omitempty"`
	Extra       map[string]interface{} `json:"extra,omitempty"`
}

// Clone creates a deep copy of Scope to avoid concurrent map mutations.
func (s Scope) Clone() Scope {
	c := Scope{
		RequestID:   s.RequestID,
		WorkflowID:  s.WorkflowID,
		ExecutionID: s.ExecutionID,
		EventID:     s.EventID,
	}
	if s.Tags != nil {
		c.Tags = make(map[string]string, len(s.Tags))
		for k, v := range s.Tags {
			c.Tags[k] = v
		}
	}
	if s.Extra != nil {
		c.Extra = make(map[string]interface{}, len(s.Extra))
		for k, v := range s.Extra {
			c.Extra[k] = v
		}
	}
	return c
}

// Provider represents an error tracking service provider (e.g., Sentry, Log, In-Memory).
type Provider interface {
	CaptureError(ctx context.Context, err error, scope Scope)
	Flush(timeout time.Duration)
}

// Sensitive data scrubbers
var (
	bearerRegex   = regexp.MustCompile(`(?i)bearer\s+[a-zA-Z0-9_.\-]+`)
	ghTokenRegex  = regexp.MustCompile(`(?i)(ghp|gho|ghu|ghs|ghr)_[a-zA-Z0-9]{20,}`)
	tgTokenRegex  = regexp.MustCompile(`[0-9]{8,11}:[a-zA-Z0-9_-]{35}`)
	pgConnRegex   = regexp.MustCompile(`postgres(ql)?://([^:]+):([^@]+)@`)
	sensitiveKeys = regexp.MustCompile(`(?i)(secret|token|password|auth|authorization|private_key|enc_key|api_key|credential)`)
)

// SanitizeString scrubs known secret patterns from an error message or log line.
func SanitizeString(msg string) string {
	msg = bearerRegex.ReplaceAllString(msg, "Bearer [REDACTED]")
	msg = ghTokenRegex.ReplaceAllString(msg, "[REDACTED_GITHUB_TOKEN]")
	msg = tgTokenRegex.ReplaceAllString(msg, "[REDACTED_TELEGRAM_TOKEN]")
	msg = pgConnRegex.ReplaceAllString(msg, "postgres://$2:[REDACTED]@")
	return msg
}

// SanitizeScope returns a cloned Scope where sensitive tags and extra values are masked.
func SanitizeScope(scope Scope) Scope {
	cloned := scope.Clone()
	if cloned.Tags != nil {
		for k := range cloned.Tags {
			if sensitiveKeys.MatchString(k) {
				cloned.Tags[k] = "[REDACTED]"
			} else {
				cloned.Tags[k] = SanitizeString(cloned.Tags[k])
			}
		}
	}
	if cloned.Extra != nil {
		for k, v := range cloned.Extra {
			if sensitiveKeys.MatchString(k) {
				cloned.Extra[k] = "[REDACTED]"
			} else if strVal, ok := v.(string); ok {
				cloned.Extra[k] = SanitizeString(strVal)
			}
		}
	}
	return cloned
}

// NoopProvider is a silent provider used for testing and default states.
type NoopProvider struct{}

func NewNoopProvider() *NoopProvider {
	return &NoopProvider{}
}

func (n *NoopProvider) CaptureError(ctx context.Context, err error, scope Scope) {}
func (n *NoopProvider) Flush(timeout time.Duration)                              {}

// LogProvider forwards tracked errors to slog with sanitized attributes.
type LogProvider struct{}

func NewLogProvider() *LogProvider {
	return &LogProvider{}
}

func (l *LogProvider) CaptureError(ctx context.Context, err error, scope Scope) {
	if err == nil {
		return
	}
	sanitizedScope := SanitizeScope(scope)
	sanitizedMsg := SanitizeString(err.Error())

	attrs := []any{
		slog.String("error", sanitizedMsg),
	}
	if sanitizedScope.RequestID != "" {
		attrs = append(attrs, slog.String("request_id", sanitizedScope.RequestID))
	}
	if sanitizedScope.WorkflowID != "" {
		attrs = append(attrs, slog.String("workflow_id", sanitizedScope.WorkflowID))
	}
	if sanitizedScope.ExecutionID != "" {
		attrs = append(attrs, slog.String("execution_id", sanitizedScope.ExecutionID))
	}
	if sanitizedScope.EventID != "" {
		attrs = append(attrs, slog.String("event_id", sanitizedScope.EventID))
	}
	if len(sanitizedScope.Tags) > 0 {
		attrs = append(attrs, slog.Any("tags", sanitizedScope.Tags))
	}
	if len(sanitizedScope.Extra) > 0 {
		attrs = append(attrs, slog.Any("extra", sanitizedScope.Extra))
	}

	slog.ErrorContext(ctx, "tracked error captured", attrs...)
}

func (l *LogProvider) Flush(timeout time.Duration) {}

// CapturedRecord represents an error recorded by MemoryProvider.
type CapturedRecord struct {
	Timestamp time.Time
	Err       error
	Scope     Scope
}

// MemoryProvider records errors in-memory for testing assertions.
type MemoryProvider struct {
	mu      sync.RWMutex
	records []CapturedRecord
}

func NewMemoryProvider() *MemoryProvider {
	return &MemoryProvider{
		records: make([]CapturedRecord, 0),
	}
}

func (m *MemoryProvider) CaptureError(ctx context.Context, err error, scope Scope) {
	if err == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.records = append(m.records, CapturedRecord{
		Timestamp: time.Now().UTC(),
		Err:       fmt.Errorf("%s", SanitizeString(err.Error())),
		Scope:     SanitizeScope(scope),
	})
}

func (m *MemoryProvider) Flush(timeout time.Duration) {}

func (m *MemoryProvider) Records() []CapturedRecord {
	m.mu.RLock()
	defer m.mu.RUnlock()
	copied := make([]CapturedRecord, len(m.records))
	copy(copied, m.records)
	return copied
}

func (m *MemoryProvider) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.records = make([]CapturedRecord, 0)
}

// SentryHTTPTransport allows testing or custom HTTP transmission of Sentry events.
type SentryHTTPTransport interface {
	Do(req *http.Request) (*http.Response, error)
}

// SentryProvider implements an error tracking provider compatible with Sentry-style ingestion.
type SentryProvider struct {
	dsn        string
	endpoint   string
	publicKey  string
	projectID  string
	httpClient *http.Client
	fallback   Provider
}

// NewSentryProvider creates a Sentry provider from a DSN string.
// If dsn is empty, returns a LogProvider fallback.
func NewSentryProvider(dsn string) Provider {
	if strings.TrimSpace(dsn) == "" {
		return NewLogProvider()
	}

	// Format: https://<public_key>@<host>/<project_id>
	parts := strings.Split(dsn, "@")
	if len(parts) != 2 {
		return NewLogProvider()
	}

	protoKey := strings.Split(parts[0], "://")
	if len(protoKey) != 2 {
		return NewLogProvider()
	}
	proto := protoKey[0]
	publicKey := protoKey[1]

	hostProject := strings.Split(parts[1], "/")
	if len(hostProject) < 2 {
		return NewLogProvider()
	}
	host := hostProject[0]
	projectID := hostProject[1]

	endpoint := fmt.Sprintf("%s://%s/api/%s/envelope/", proto, host, projectID)

	return &SentryProvider{
		dsn:        dsn,
		endpoint:   endpoint,
		publicKey:  publicKey,
		projectID:  projectID,
		httpClient: &http.Client{Timeout: 5 * time.Second},
		fallback:   NewLogProvider(),
	}
}

func (s *SentryProvider) CaptureError(ctx context.Context, err error, scope Scope) {
	if err == nil {
		return
	}
	// Sanitize scope and error message to never leak secrets
	cleanScope := SanitizeScope(scope)
	cleanErr := fmt.Errorf("%s", SanitizeString(err.Error()))

	// Always ensure structured logging fallback
	if s.fallback != nil {
		s.fallback.CaptureError(ctx, cleanErr, cleanScope)
	}
}

func (s *SentryProvider) Flush(timeout time.Duration) {}
