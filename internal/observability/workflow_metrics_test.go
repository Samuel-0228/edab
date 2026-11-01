package observability

import (
	"strings"
	"testing"
	"time"
)

func TestWorkflowMetricsRegistry(t *testing.T) {
	reg := NewWorkflowMetricsRegistry()

	// 1. Record executions
	reg.RecordWorkflowExecution("wf-alpha", "success")
	reg.RecordWorkflowExecution("wf-alpha", "success")
	reg.RecordWorkflowExecution("wf-alpha", "failed")
	reg.RecordWorkflowExecution("wf-beta", "success")

	// 2. Record step latencies
	reg.RecordStepLatency("wf-alpha", "step-http", 15*time.Millisecond)
	reg.RecordStepLatency("wf-alpha", "step-http", 45*time.Millisecond)
	reg.RecordStepLatency("wf-alpha", "step-format", 2*time.Millisecond)

	// 3. Export to Prometheus format
	var b strings.Builder
	reg.ExportPrometheus(&b)
	out := b.String()

	if !strings.Contains(out, `kerob_workflow_execution_total{workflow_id="wf-alpha",status="failed"} 1`) {
		t.Errorf("expected wf-alpha failed count in output, got: %s", out)
	}
	if !strings.Contains(out, `kerob_workflow_execution_total{workflow_id="wf-alpha",status="success"} 2`) {
		t.Errorf("expected wf-alpha success count in output, got: %s", out)
	}
	if !strings.Contains(out, `kerob_workflow_execution_total{workflow_id="wf-beta",status="success"} 1`) {
		t.Errorf("expected wf-beta success count in output, got: %s", out)
	}
	if !strings.Contains(out, `kerob_step_latency_seconds_count{workflow_id="wf-alpha",step_id="step-http"} 2`) {
		t.Errorf("expected step-http latency count 2, got: %s", out)
	}
	if !strings.Contains(out, `kerob_step_latency_seconds_bucket{workflow_id="wf-alpha",step_id="step-format",le="0.005"} 1`) {
		t.Errorf("expected 2ms latency in 0.005 bucket, got: %s", out)
	}

	// 4. Reset
	reg.Reset()
	var bEmpty strings.Builder
	reg.ExportPrometheus(&bEmpty)
	if bEmpty.Len() != 0 {
		t.Errorf("expected empty string after reset, got: %s", bEmpty.String())
	}
}

func TestWorkflowMetricsDefaults(t *testing.T) {
	reg := NewWorkflowMetricsRegistry()
	reg.RecordWorkflowExecution("", "")
	reg.RecordStepLatency("", "", 10*time.Millisecond)

	var b strings.Builder
	reg.ExportPrometheus(&b)
	out := b.String()

	if !strings.Contains(out, `workflow_id="unknown"`) {
		t.Errorf("expected unknown workflow_id for empty inputs, got: %s", out)
	}
}
