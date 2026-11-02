package workflow

import "time"

// ExecutionEventListener receives callbacks for workflow execution lifecycle transitions
// and granular step execution records.
type ExecutionEventListener interface {
	OnExecutionStarted(exec *Execution)
	OnExecutionStep(exec *Execution, step *ExecutionStep)
	OnExecutionCompleted(exec *Execution)
	OnExecutionFailed(exec *Execution, errStr string)
	OnExecutionRetrying(exec *Execution, nextAttempt time.Time)
	OnExecutionPaused(exec *Execution)
	OnExecutionResumed(exec *Execution)
	OnExecutionCancelled(exec *Execution)
	OnExecutionWaitingApproval(exec *Execution, approval *Approval)
	OnExecutionCompensating(exec *Execution)
	OnExecutionCompensated(exec *Execution)
}
