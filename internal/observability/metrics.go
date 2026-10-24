package observability

import (
	"sync/atomic"
	"time"
)

// Metrics contains process-local counters and duration totals. Exporters can
// expose these values without coupling the engine to a metrics vendor.
type Metrics struct {
	ExecutionsStarted   atomic.Uint64
	ExecutionsSucceeded atomic.Uint64
	ExecutionsFailed    atomic.Uint64
	Retries             atomic.Uint64
	EventsProcessed     atomic.Uint64
	ActionFailures      atomic.Uint64
	ExecutionDurationMs atomic.Uint64
}

var DefaultMetrics Metrics

func RecordExecutionStarted()   { DefaultMetrics.ExecutionsStarted.Add(1) }
func RecordExecutionSucceeded() { DefaultMetrics.ExecutionsSucceeded.Add(1) }
func RecordExecutionFailed()    { DefaultMetrics.ExecutionsFailed.Add(1) }
func RecordRetry()              { DefaultMetrics.Retries.Add(1) }
func RecordEvent()              { DefaultMetrics.EventsProcessed.Add(1) }
func RecordActionFailure()      { DefaultMetrics.ActionFailures.Add(1) }
func RecordExecutionDuration(d time.Duration) {
	DefaultMetrics.ExecutionDurationMs.Add(uint64(d.Milliseconds()))
}
