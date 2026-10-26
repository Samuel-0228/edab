package observability

import (
	"testing"
	"time"
)

func TestRecordExecutionMetrics(t *testing.T) {
	DefaultMetrics = Metrics{}

	RecordExecutionStarted()
	RecordExecutionSucceeded()
	RecordExecutionFailed()
	RecordRetry()
	RecordActionFailure()
	RecordExecutionDuration(25 * time.Millisecond)
	RecordEvent()

	if DefaultMetrics.ExecutionsStarted.Load() != 1 ||
		DefaultMetrics.ExecutionsSucceeded.Load() != 1 ||
		DefaultMetrics.ExecutionsFailed.Load() != 1 ||
		DefaultMetrics.Retries.Load() != 1 ||
		DefaultMetrics.ActionFailures.Load() != 1 ||
		DefaultMetrics.ExecutionDurationMs.Load() != 25 ||
		DefaultMetrics.EventsProcessed.Load() != 1 {
		t.Fatalf("unexpected metrics snapshot: started=%d succeeded=%d failed=%d retries=%d action_failures=%d duration=%d events=%d",
			DefaultMetrics.ExecutionsStarted.Load(),
			DefaultMetrics.ExecutionsSucceeded.Load(),
			DefaultMetrics.ExecutionsFailed.Load(),
			DefaultMetrics.Retries.Load(),
			DefaultMetrics.ActionFailures.Load(),
			DefaultMetrics.ExecutionDurationMs.Load(),
			DefaultMetrics.EventsProcessed.Load(),
		)
	}
}
