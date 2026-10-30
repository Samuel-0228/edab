package api

import (
	"fmt"
	"net/http"
	"strings"

	"kerob/internal/observability"
)

// RecordWorkflowExecution increments execution counters.
func RecordWorkflowExecution(status string) {
	if status == "success" {
		observability.RecordExecutionSucceeded()
	} else if status == "failed" {
		observability.RecordExecutionFailed()
	}
}

// RecordEventProcessed increments the events counter.
func RecordEventProcessed() {
	observability.RecordEvent()
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

	var b strings.Builder
	fmt.Fprintf(&b, "# HELP kerob_workflow_executions_total Total number of workflow executions\n")
	fmt.Fprintf(&b, "# TYPE kerob_workflow_executions_total counter\n")
	started := observability.DefaultMetrics.ExecutionsStarted.Load()
	fmt.Fprintf(&b, "kerob_workflow_executions_total %d\n\n", started)

	fmt.Fprintf(&b, "# HELP kerob_workflow_successes_total Total successful workflow executions\n")
	fmt.Fprintf(&b, "# TYPE kerob_workflow_successes_total counter\n")
	fmt.Fprintf(&b, "kerob_workflow_successes_total %d\n\n", observability.DefaultMetrics.ExecutionsSucceeded.Load())

	fmt.Fprintf(&b, "# HELP kerob_workflow_failures_total Total failed workflow executions\n")
	fmt.Fprintf(&b, "# TYPE kerob_workflow_failures_total counter\n")
	fmt.Fprintf(&b, "kerob_workflow_failures_total %d\n\n", observability.DefaultMetrics.ExecutionsFailed.Load())

	fmt.Fprintf(&b, "# HELP kerob_events_processed_total Total normalized events ingested\n")
	fmt.Fprintf(&b, "# TYPE kerob_events_processed_total counter\n")
	fmt.Fprintf(&b, "kerob_events_processed_total %d\n\n", observability.DefaultMetrics.EventsProcessed.Load())

	fmt.Fprintf(&b, "# HELP kerob_workflow_retries_total Total scheduled workflow retries\n")
	fmt.Fprintf(&b, "# TYPE kerob_workflow_retries_total counter\n")
	fmt.Fprintf(&b, "kerob_workflow_retries_total %d\n\n", observability.DefaultMetrics.Retries.Load())

	fmt.Fprintf(&b, "# HELP kerob_action_failures_total Total failed actions\n")
	fmt.Fprintf(&b, "# TYPE kerob_action_failures_total counter\n")
	fmt.Fprintf(&b, "kerob_action_failures_total %d\n\n", observability.DefaultMetrics.ActionFailures.Load())

	fmt.Fprintf(&b, "# HELP kerob_workflow_execution_duration_milliseconds_total Total execution duration\n")
	fmt.Fprintf(&b, "# TYPE kerob_workflow_execution_duration_milliseconds_total counter\n")
	fmt.Fprintf(&b, "kerob_workflow_execution_duration_milliseconds_total %d\n\n", observability.DefaultMetrics.ExecutionDurationMs.Load())

	fmt.Fprintf(&b, "# HELP kerob_active_workflows Number of active workflows\n")
	fmt.Fprintf(&b, "# TYPE kerob_active_workflows gauge\n")
	fmt.Fprintf(&b, "kerob_active_workflows %d\n\n", activeWorkflows)

	fmt.Fprintf(&b, "# HELP kerob_dead_letter_count Number of unresolved dead-letter failures\n")
	fmt.Fprintf(&b, "# TYPE kerob_dead_letter_count gauge\n")
	fmt.Fprintf(&b, "kerob_dead_letter_count %d\n\n", deadLetters)

	fmt.Fprintf(&b, "# HELP kerob_circuit_breaker_open_total Total number of open circuit breakers\n")
	fmt.Fprintf(&b, "# TYPE kerob_circuit_breaker_open_total gauge\n")
	openCBs := 0
	if h.workflowEngine != nil && h.workflowEngine.CircuitBreakers() != nil {
		for _, state := range h.workflowEngine.CircuitBreakers().Status() {
			if state == "open" {
				openCBs++
			}
		}
	}
	fmt.Fprintf(&b, "kerob_circuit_breaker_open_total %d\n", openCBs)

	_, _ = w.Write([]byte(b.String()))
}
