package observability

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

type spanContextKey struct{}

// StatusCode represents OpenTelemetry span completion status.
type StatusCode int

const (
	StatusUnset StatusCode = 0
	StatusOK    StatusCode = 1
	StatusError StatusCode = 2
)

// Span represents an active trace span.
type Span interface {
	SetAttribute(key string, val any)
	SetStatus(code StatusCode, description string)
	RecordError(err error)
	End()
	TraceID() string
	SpanID() string
}

type noOpSpan struct{}

func (s *noOpSpan) SetAttribute(key string, val any)       {}
func (s *noOpSpan) SetStatus(code StatusCode, desc string) {}
func (s *noOpSpan) RecordError(err error)                  {}
func (s *noOpSpan) End()                                   {}
func (s *noOpSpan) TraceID() string                        { return "" }
func (s *noOpSpan) SpanID() string                         { return "" }

type otlpSpanRecord struct {
	TraceID      string         `json:"traceId"`
	SpanID       string         `json:"spanId"`
	ParentSpanID string         `json:"parentSpanId,omitempty"`
	Name         string         `json:"name"`
	StartTime    int64          `json:"startTimeUnixNano"`
	EndTime      int64          `json:"endTimeUnixNano"`
	Attributes   map[string]any `json:"attributes,omitempty"`
	StatusCode   int            `json:"statusCode"`
	StatusDesc   string         `json:"statusDescription,omitempty"`
}

type activeOtlpSpan struct {
	record   otlpSpanRecord
	exporter *otlpExporter
	mu       sync.Mutex
}

func (s *activeOtlpSpan) SetAttribute(key string, val any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.record.Attributes == nil {
		s.record.Attributes = make(map[string]any)
	}
	s.record.Attributes[key] = val
}

func (s *activeOtlpSpan) SetStatus(code StatusCode, desc string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.record.StatusCode = int(code)
	s.record.StatusDesc = desc
}

func (s *activeOtlpSpan) RecordError(err error) {
	if err == nil {
		return
	}
	s.SetStatus(StatusError, err.Error())
	s.SetAttribute("error", true)
	s.SetAttribute("error.message", err.Error())
}

func (s *activeOtlpSpan) End() {
	s.mu.Lock()
	s.record.EndTime = time.Now().UnixNano()
	rec := s.record
	s.mu.Unlock()

	if s.exporter != nil {
		s.exporter.enqueue(rec)
	}
}

func (s *activeOtlpSpan) TraceID() string { return s.record.TraceID }
func (s *activeOtlpSpan) SpanID() string  { return s.record.SpanID }

type otlpExporter struct {
	endpoint   string
	httpClient *http.Client
	ch         chan otlpSpanRecord
	stopOnce   sync.Once
	done       chan struct{}
	wg         sync.WaitGroup
}

func newOtlpExporter(endpoint string, client *http.Client) *otlpExporter {
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Second}
	}
	ep := strings.TrimRight(endpoint, "/")
	if !strings.HasSuffix(ep, "/v1/traces") {
		ep = ep + "/v1/traces"
	}
	e := &otlpExporter{
		endpoint:   ep,
		httpClient: client,
		ch:         make(chan otlpSpanRecord, 500),
		done:       make(chan struct{}),
	}
	e.wg.Add(1)
	go e.worker()
	return e
}

func (e *otlpExporter) enqueue(rec otlpSpanRecord) {
	select {
	case e.ch <- rec:
	default:
		// Drop if buffer full to avoid blocking hot path
	}
}

func (e *otlpExporter) worker() {
	defer e.wg.Done()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	var batch []otlpSpanRecord
	for {
		select {
		case rec := <-e.ch:
			batch = append(batch, rec)
			if len(batch) >= 50 {
				e.flush(batch)
				batch = nil
			}
		case <-ticker.C:
			if len(batch) > 0 {
				e.flush(batch)
				batch = nil
			}
		case <-e.done:
			// Drain remaining
			for {
				select {
				case rec := <-e.ch:
					batch = append(batch, rec)
				default:
					if len(batch) > 0 {
						e.flush(batch)
					}
					return
				}
			}
		}
	}
}

func (e *otlpExporter) flush(batch []otlpSpanRecord) {
	if len(batch) == 0 {
		return
	}
	payload := map[string]any{
		"resourceSpans": []map[string]any{
			{
				"resource": map[string]any{
					"attributes": []map[string]any{
						{"key": "service.name", "value": map[string]string{"stringValue": "kerob"}},
					},
				},
				"scopeSpans": []map[string]any{
					{
						"scope": map[string]string{"name": "kerob"},
						"spans": batch,
					},
				},
			},
		},
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return
	}
	req, err := http.NewRequest(http.MethodPost, e.endpoint, bytes.NewReader(data))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := e.httpClient.Do(req)
	if err == nil && resp != nil {
		_ = resp.Body.Close()
	}
}

func (e *otlpExporter) shutdown() {
	e.stopOnce.Do(func() {
		close(e.done)
		e.wg.Wait()
	})
}

var (
	globalExporterMu sync.RWMutex
	globalExporter   *otlpExporter
)

// InitTracing initializes the global tracer. If endpoint is empty, it checks OTEL_EXPORTER_OTLP_ENDPOINT.
func InitTracing(endpoint string, client *http.Client) func() {
	if endpoint == "" {
		endpoint = os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")
	}

	globalExporterMu.Lock()
	if globalExporter != nil {
		globalExporter.shutdown()
		globalExporter = nil
	}

	if endpoint != "" {
		globalExporter = newOtlpExporter(endpoint, client)
	}
	globalExporterMu.Unlock()

	return func() {
		globalExporterMu.Lock()
		defer globalExporterMu.Unlock()
		if globalExporter != nil {
			globalExporter.shutdown()
			globalExporter = nil
		}
	}
}

// StartSpan creates a new child span within context.
func StartSpan(ctx context.Context, spanName string) (context.Context, Span) {
	globalExporterMu.RLock()
	exp := globalExporter
	globalExporterMu.RUnlock()

	if exp == nil {
		return ctx, &noOpSpan{}
	}

	var traceID, parentSpanID string
	if parent, ok := ctx.Value(spanContextKey{}).(Span); ok && parent != nil && parent.TraceID() != "" {
		traceID = parent.TraceID()
		parentSpanID = parent.SpanID()
	} else {
		traceID = randomHex(16) // 128-bit
	}
	spanID := randomHex(8) // 64-bit

	span := &activeOtlpSpan{
		record: otlpSpanRecord{
			TraceID:      traceID,
			SpanID:       spanID,
			ParentSpanID: parentSpanID,
			Name:         spanName,
			StartTime:    time.Now().UnixNano(),
			Attributes:   make(map[string]any),
		},
		exporter: exp,
	}

	newCtx := context.WithValue(ctx, spanContextKey{}, span)
	return newCtx, span
}

func randomHex(bytesLen int) string {
	b := make([]byte, bytesLen)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// SpanFromContext retrieves the active span from context, if any.
func SpanFromContext(ctx context.Context) Span {
	if s, ok := ctx.Value(spanContextKey{}).(Span); ok {
		return s
	}
	return &noOpSpan{}
}
