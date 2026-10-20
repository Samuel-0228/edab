package api

import (
	"fmt"
	"net/http"
	"sync/atomic"
)

// MetricsTracker records application execution statistics for Prometheus scraping.
type MetricsTracker struct {
	workflowExecutions atomic.Uint64
	workflowSuccesses  atomic.Uint64
	workflowFailures   atomic.Uint64
	eventsProcessed    atomic.Uint64
}

var globalMetrics = &MetricsTracker{}

// RecordWorkflowExecution increments execution counters.
func RecordWorkflowExecution(status string) {
	globalMetrics.workflowExecutions.Add(1)
	if status == "success" {
		globalMetrics.workflowSuccesses.Add(1)
	} else if status == "failed" {
		globalMetrics.workflowFailures.Add(1)
	}
}

// RecordEventProcessed increments the events counter.
func RecordEventProcessed() {
	globalMetrics.eventsProcessed.Add(1)
}

func (h *Handlers) Metrics(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")

	activeWorkflows := 0
	if h.workflows != nil {
		if list, err := h.workflows.ListActive(r.Context()); err == nil {
			activeWorkflows = len(list)
		}
	}

	deadLetters := 0
	if h.deadLetters != nil {
		if dl, err := h.deadLetters.List(r.Context(), false); err == nil {
			deadLetters = len(dl)
		}
	}

	fmt.Fprintf(w, "# HELP kerob_workflow_executions_total Total number of workflow executions\n")
	fmt.Fprintf(w, "# TYPE kerob_workflow_executions_total counter\n")
	fmt.Fprintf(w, "kerob_workflow_executions_total %d\n\n", globalMetrics.workflowExecutions.Load())

	fmt.Fprintf(w, "# HELP kerob_workflow_successes_total Total successful workflow executions\n")
	fmt.Fprintf(w, "# TYPE kerob_workflow_successes_total counter\n")
	fmt.Fprintf(w, "kerob_workflow_successes_total %d\n\n", globalMetrics.workflowSuccesses.Load())

	fmt.Fprintf(w, "# HELP kerob_workflow_failures_total Total failed workflow executions\n")
	fmt.Fprintf(w, "# TYPE kerob_workflow_failures_total counter\n")
	fmt.Fprintf(w, "kerob_workflow_failures_total %d\n\n", globalMetrics.workflowFailures.Load())

	fmt.Fprintf(w, "# HELP kerob_events_processed_total Total normalized events ingested\n")
	fmt.Fprintf(w, "# TYPE kerob_events_processed_total counter\n")
	fmt.Fprintf(w, "kerob_events_processed_total %d\n\n", globalMetrics.eventsProcessed.Load())

	fmt.Fprintf(w, "# HELP kerob_active_workflows Number of active workflows\n")
	fmt.Fprintf(w, "# TYPE kerob_active_workflows gauge\n")
	fmt.Fprintf(w, "kerob_active_workflows %d\n\n", activeWorkflows)

	fmt.Fprintf(w, "# HELP kerob_dead_letter_count Number of unresolved dead-letter failures\n")
	fmt.Fprintf(w, "# TYPE kerob_dead_letter_count gauge\n")
	fmt.Fprintf(w, "kerob_dead_letter_count %d\n", deadLetters)
}
