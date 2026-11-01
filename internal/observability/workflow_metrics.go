package observability

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

var defaultLatencyBuckets = []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0, 2.5, 5.0, 10.0}

// StepLatencyHistogram tracks latency distributions for action steps.
type StepLatencyHistogram struct {
	mu      sync.Mutex
	Buckets []float64
	Counts  []uint64
	Count   uint64
	Sum     float64
}

// NewStepLatencyHistogram creates a latency histogram initialized with default buckets.
func NewStepLatencyHistogram() *StepLatencyHistogram {
	b := make([]float64, len(defaultLatencyBuckets))
	copy(b, defaultLatencyBuckets)
	return &StepLatencyHistogram{
		Buckets: b,
		Counts:  make([]uint64, len(defaultLatencyBuckets)),
	}
}

// Observe records a latency duration in seconds.
func (h *StepLatencyHistogram) Observe(seconds float64) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.Count++
	h.Sum += seconds
	for i, bucket := range h.Buckets {
		if seconds <= bucket {
			h.Counts[i]++
		}
	}
}

// WorkflowMetricsRegistry tracks per-workflow execution counts and step latency histograms.
type WorkflowMetricsRegistry struct {
	mu            sync.RWMutex
	executions    map[string]map[string]uint64
	stepLatencies map[string]map[string]*StepLatencyHistogram
}

// DefaultWorkflowMetrics is the singleton per-workflow metrics registry.
var DefaultWorkflowMetrics = NewWorkflowMetricsRegistry()

// NewWorkflowMetricsRegistry constructs an initialized metrics registry.
func NewWorkflowMetricsRegistry() *WorkflowMetricsRegistry {
	return &WorkflowMetricsRegistry{
		executions:    make(map[string]map[string]uint64),
		stepLatencies: make(map[string]map[string]*StepLatencyHistogram),
	}
}

// RecordWorkflowExecution records a workflow completion status.
func (r *WorkflowMetricsRegistry) RecordWorkflowExecution(workflowID, status string) {
	if workflowID == "" {
		workflowID = "unknown"
	}
	if status == "" {
		status = "unknown"
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	statusMap, exists := r.executions[workflowID]
	if !exists {
		statusMap = make(map[string]uint64)
		r.executions[workflowID] = statusMap
	}
	statusMap[status]++
}

// RecordStepLatency records duration of an executed step.
func (r *WorkflowMetricsRegistry) RecordStepLatency(workflowID, stepID string, duration time.Duration) {
	if workflowID == "" {
		workflowID = "unknown"
	}
	if stepID == "" {
		stepID = "unknown"
	}

	r.mu.Lock()
	stepMap, exists := r.stepLatencies[workflowID]
	if !exists {
		stepMap = make(map[string]*StepLatencyHistogram)
		r.stepLatencies[workflowID] = stepMap
	}
	hist, exists := stepMap[stepID]
	if !exists {
		hist = NewStepLatencyHistogram()
		stepMap[stepID] = hist
	}
	r.mu.Unlock()

	hist.Observe(duration.Seconds())
}

// Reset clears all metrics for clean test isolation.
func (r *WorkflowMetricsRegistry) Reset() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.executions = make(map[string]map[string]uint64)
	r.stepLatencies = make(map[string]map[string]*StepLatencyHistogram)
}

// ExportPrometheus renders Prometheus exposition formatted metrics into a strings.Builder.
func (r *WorkflowMetricsRegistry) ExportPrometheus(b *strings.Builder) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if len(r.executions) > 0 {
		fmt.Fprintf(b, "# HELP kerob_workflow_execution_total Per-workflow execution counts by status\n")
		fmt.Fprintf(b, "# TYPE kerob_workflow_execution_total counter\n")

		wfIDs := make([]string, 0, len(r.executions))
		for wfID := range r.executions {
			wfIDs = append(wfIDs, wfID)
		}
		sort.Strings(wfIDs)

		for _, wfID := range wfIDs {
			statuses := make([]string, 0, len(r.executions[wfID]))
			for st := range r.executions[wfID] {
				statuses = append(statuses, st)
			}
			sort.Strings(statuses)

			for _, st := range statuses {
				count := r.executions[wfID][st]
				fmt.Fprintf(b, "kerob_workflow_execution_total{workflow_id=\"%s\",status=\"%s\"} %d\n", wfID, st, count)
			}
		}
		b.WriteString("\n")
	}

	if len(r.stepLatencies) > 0 {
		fmt.Fprintf(b, "# HELP kerob_step_latency_seconds Step latency distribution in seconds\n")
		fmt.Fprintf(b, "# TYPE kerob_step_latency_seconds histogram\n")

		wfIDs := make([]string, 0, len(r.stepLatencies))
		for wfID := range r.stepLatencies {
			wfIDs = append(wfIDs, wfID)
		}
		sort.Strings(wfIDs)

		for _, wfID := range wfIDs {
			stepIDs := make([]string, 0, len(r.stepLatencies[wfID]))
			for sID := range r.stepLatencies[wfID] {
				stepIDs = append(stepIDs, sID)
			}
			sort.Strings(stepIDs)

			for _, sID := range stepIDs {
				hist := r.stepLatencies[wfID][sID]
				hist.mu.Lock()
				for i, bucket := range hist.Buckets {
					fmt.Fprintf(b, "kerob_step_latency_seconds_bucket{workflow_id=\"%s\",step_id=\"%s\",le=\"%g\"} %d\n",
						wfID, sID, bucket, hist.Counts[i])
				}
				fmt.Fprintf(b, "kerob_step_latency_seconds_bucket{workflow_id=\"%s\",step_id=\"%s\",le=\"+Inf\"} %d\n",
					wfID, sID, hist.Count)
				fmt.Fprintf(b, "kerob_step_latency_seconds_sum{workflow_id=\"%s\",step_id=\"%s\"} %.6f\n",
					wfID, sID, hist.Sum)
				fmt.Fprintf(b, "kerob_step_latency_seconds_count{workflow_id=\"%s\",step_id=\"%s\"} %d\n",
					wfID, sID, hist.Count)
				hist.mu.Unlock()
			}
		}
		b.WriteString("\n")
	}
}
