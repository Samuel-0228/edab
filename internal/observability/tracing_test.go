package observability

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

func TestNoOpTracing(t *testing.T) {
	// Ensure tracing is uninitialized
	shutdown := InitTracing("", nil)
	defer shutdown()

	ctx, span := StartSpan(context.Background(), "test.noop")
	span.SetAttribute("key", "val")
	span.SetStatus(StatusOK, "ok")
	span.RecordError(errors.New("fail"))
	span.End()

	if span.TraceID() != "" || span.SpanID() != "" {
		t.Errorf("expected empty IDs from noOpSpan, got trace=%s, span=%s", span.TraceID(), span.SpanID())
	}
	fromCtx := SpanFromContext(ctx)
	if fromCtx.TraceID() != "" {
		t.Errorf("expected no-op span from context")
	}
}

func TestOtlpTracingExportAndHierarchy(t *testing.T) {
	var mu sync.Mutex
	var receivedPayloads []map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var p map[string]any
		if err := json.Unmarshal(body, &p); err == nil {
			mu.Lock()
			receivedPayloads = append(receivedPayloads, p)
			mu.Unlock()
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	shutdown := InitTracing(srv.URL, srv.Client())
	defer shutdown()

	ctx := context.Background()

	// 1. Root span: event.ingestion
	ctx, spanRoot := StartSpan(ctx, "event.ingestion")
	spanRoot.SetAttribute("event.id", "evt-123")
	spanRoot.SetAttribute("event.type", "order.created")

	// 2. Child span: workflow.execution
	ctxChild, spanChild := StartSpan(ctx, "workflow.execution")
	spanChild.SetAttribute("workflow.id", "wf-456")

	// 3. Grandchild span: step.execution
	_, spanGrandchild := StartSpan(ctxChild, "step.execution")
	spanGrandchild.SetAttribute("step.id", "step-1")
	spanGrandchild.RecordError(errors.New("step timeout"))
	spanGrandchild.End()

	spanChild.SetStatus(StatusOK, "complete")
	spanChild.End()

	spanRoot.End()

	// Flush exporter
	shutdown()

	mu.Lock()
	defer mu.Unlock()

	if len(receivedPayloads) == 0 {
		t.Fatalf("expected at least 1 trace payload delivered to collector")
	}

	// Verify trace hierarchy across spans
	var foundRoot, foundChild, foundGrandchild bool
	for _, p := range receivedPayloads {
		rsList, _ := p["resourceSpans"].([]any)
		for _, rs := range rsList {
			rsMap, _ := rs.(map[string]any)
			ssList, _ := rsMap["scopeSpans"].([]any)
			for _, ss := range ssList {
				ssMap, _ := ss.(map[string]any)
				spans, _ := ssMap["spans"].([]any)
				for _, sp := range spans {
					sMap, _ := sp.(map[string]any)
					name, _ := sMap["name"].(string)
					traceID, _ := sMap["traceId"].(string)
					parentID, _ := sMap["parentSpanId"].(string)

					if traceID != spanRoot.TraceID() {
						t.Errorf("expected all spans to share traceID %s, got %s", spanRoot.TraceID(), traceID)
					}

					switch name {
					case "event.ingestion":
						foundRoot = true
						if parentID != "" {
							t.Errorf("expected root span to have no parent, got %s", parentID)
						}
					case "workflow.execution":
						foundChild = true
						if parentID != spanRoot.SpanID() {
							t.Errorf("expected child to have root span parent %s, got %s", spanRoot.SpanID(), parentID)
						}
					case "step.execution":
						foundGrandchild = true
						if parentID != spanChild.SpanID() {
							t.Errorf("expected grandchild to have child span parent %s, got %s", spanChild.SpanID(), parentID)
						}
					}
				}
			}
		}
	}

	if !foundRoot || !foundChild || !foundGrandchild {
		t.Errorf("expected all 3 spans found in export: root=%v, child=%v, grandchild=%v", foundRoot, foundChild, foundGrandchild)
	}
}
