package workflow

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"kerob/internal/core/event"
	"kerob/internal/core/transformations"
	"kerob/internal/observability"
)

type stepExecutionOutcome int

const (
	outcomeSuccess stepExecutionOutcome = iota
	outcomeContinue
	outcomeSkipRest
	outcomeFailed
	outcomeWaiting
)

// loadStepStates reconstructs the outcomes of previously completed steps for an execution.
// If an action step previously succeeded or was skipped, its output and status are re-used
// so that retries resume at the failed step rather than re-running already completed actions.
func (e *Engine) loadStepStates(ctx context.Context, execID string) map[string]transformations.StepOutput {
	stepStates := make(map[string]transformations.StepOutput)
	if e.executions == nil || execID == "" {
		return stepStates
	}

	exec, err := e.executions.GetByID(ctx, execID)
	if err != nil || exec == nil {
		return stepStates
	}

	for _, step := range exec.Steps {
		stepKey := step.StepID
		if stepKey == "" {
			stepKey = step.Name
		}
		var errStr string
		if step.Error != nil {
			errStr = *step.Error
		}
		stepStates[stepKey] = transformations.StepOutput{
			Status:     step.Status,
			Output:     step.Output,
			Error:      errStr,
			DurationMs: step.DurationMs,
		}
	}

	return stepStates
}

func (e *Engine) executeStep(
	ctx context.Context,
	wf *Workflow,
	ev *event.Event,
	exec *Execution,
	transformedPayload map[string]any,
	act ActionStep,
	stepStates map[string]transformations.StepOutput,
	renderVars map[string]any,
	start time.Time,
) (stepExecutionOutcome, error) {
	stepStart := time.Now()
	ctx, span := observability.StartSpan(ctx, "step.execution")
	defer func() {
		dur := time.Since(stepStart)
		span.SetAttribute("step.duration_ms", dur.Milliseconds())
		observability.DefaultWorkflowMetrics.RecordStepLatency(wf.ID, act.ID, dur)
		span.End()
	}()
	span.SetAttribute("workflow.id", wf.ID)
	span.SetAttribute("step.id", act.ID)
	span.SetAttribute("step.type", string(act.Type))
	span.SetAttribute("step.integration", act.Integration)

	outcome, err := e.dispatchStep(ctx, wf, ev, exec, transformedPayload, act, stepStates, renderVars, start)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(observability.StatusError, err.Error())
	} else {
		span.SetStatus(observability.StatusOK, "complete")
	}
	return outcome, err
}

func (e *Engine) dispatchStep(
	ctx context.Context,
	wf *Workflow,
	ev *event.Event,
	exec *Execution,
	transformedPayload map[string]any,
	act ActionStep,
	stepStates map[string]transformations.StepOutput,
	renderVars map[string]any,
	start time.Time,
) (stepExecutionOutcome, error) {
	// If the step is an approval step or requires approval before executing
	if act.Type == StepTypeApproval || (act.Approval != nil && stepStates[act.ID].Status != "approved") {
		return e.executeApprovalStep(ctx, wf, ev, exec, transformedPayload, act, stepStates, renderVars, start)
	}

	switch act.Type {
	case StepTypeBranch:
		return e.executeBranchStep(ctx, wf, ev, exec, transformedPayload, act, stepStates, renderVars, start)
	case StepTypeForEach:
		return e.executeForEachStep(ctx, wf, ev, exec, transformedPayload, act, stepStates, renderVars, start)
	case StepTypeDelay, StepTypeWait:
		return e.executeDelayStep(ctx, wf, ev, exec, transformedPayload, act, stepStates, renderVars, start)
	case StepTypeParallel:
		return e.executeParallelStep(ctx, wf, ev, exec, transformedPayload, act, stepStates, renderVars, start)
	default:
		return e.executeActionStep(ctx, wf, ev, exec, transformedPayload, act, stepStates, renderVars, start)
	}
}

func (e *Engine) executeActionStep(
	ctx context.Context,
	wf *Workflow,
	ev *event.Event,
	exec *Execution,
	transformedPayload map[string]any,
	act ActionStep,
	stepStates map[string]transformations.StepOutput,
	renderVars map[string]any,
	start time.Time,
) (stepExecutionOutcome, error) {
	if err := ctx.Err(); err != nil {
		_, fErr := e.handleExecutionFailure(ctx, exec, wf, ev, act.ID, err.Error(), start)
		return outcomeFailed, fErr
	}

	actStart := time.Now().UTC()
	stepName := act.Name
	if stepName == "" {
		stepName = fmt.Sprintf("Action: %s (%s)", act.Integration, act.ActionType)
	}

	actionStep := &ExecutionStep{
		ExecutionID: exec.ID,
		StepID:      act.ID,
		StepType:    "action",
		Name:        stepName,
		StartedAt:   actStart,
	}

	// 1. Resolve credentials if configured
	var decryptedCreds map[string]string
	if act.CredentialID != "" && e.creds != nil {
		creds, err := e.creds.GetDecrypted(ctx, act.CredentialID)
		if err != nil {
			errStr := fmt.Sprintf("failed to decrypt action credentials: %v", err)
			actionStep.Status = "failed"
			actionStep.Error = &errStr
			actionStep.CompletedAt = time.Now().UTC()
			actionStep.DurationMs = time.Since(actStart).Milliseconds()
			e.recordStep(ctx, actionStep)

			return e.applyOnError(ctx, exec, wf, ev, act, errStr, start)
		}
		decryptedCreds = creds
	}

	// 2. Interpolate action configuration with event data, step outputs, vault secrets, and loop variables
	renderCtx := transformations.RenderContext{
		Event:     ev,
		Steps:     stepStates,
		Secrets:   decryptedCreds,
		Variables: renderVars,
	}

	renderedConfig, err := transformations.RenderJSONConfigWithContext(act.Config, renderCtx)
	if err != nil {
		errStr := fmt.Sprintf("failed to render action config: %v", err)
		actionStep.Status = "failed"
		actionStep.Error = &errStr
		actionStep.CompletedAt = time.Now().UTC()
		actionStep.DurationMs = time.Since(actStart).Milliseconds()
		e.recordStep(ctx, actionStep)

		return e.applyOnError(ctx, exec, wf, ev, act, errStr, start)
	}

	// 3. Lookup registered action executor
	executor, err := e.actions.Get(act.Integration)
	if err != nil {
		errStr := err.Error()
		actionStep.Status = "failed"
		actionStep.Error = &errStr
		actionStep.CompletedAt = time.Now().UTC()
		actionStep.DurationMs = time.Since(actStart).Milliseconds()
		e.recordStep(ctx, actionStep)

		return e.applyOnError(ctx, exec, wf, ev, act, errStr, start)
	}

	// 4. Enforce action timeout
	timeout := time.Duration(act.TimeoutSec) * time.Second
	if timeout <= 0 {
		timeout = time.Duration(wf.TimeoutSeconds) * time.Second
	}
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	actCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// 5. Circuit Breaker check
	if e.circuitBreakers != nil {
		if cbErr := e.circuitBreakers.Allow(act.Integration); cbErr != nil {
			errStr := cbErr.Error()
			actionStep.Status = "failed"
			actionStep.Error = &errStr
			actionStep.CompletedAt = time.Now().UTC()
			actionStep.DurationMs = time.Since(actStart).Milliseconds()
			e.recordStep(ctx, actionStep)

			return e.applyOnError(ctx, exec, wf, ev, act, errStr, start)
		}
	}

	// 5.5 Governance runtime policy enforcement
	if e.governance != nil {
		if govErr := e.governance.EnforceActionRuntime(ctx, wf.TenantID, act, transformedPayload, renderedConfig); govErr != nil {
			errStr := fmt.Sprintf("governance policy violation: %v", govErr)
			actionStep.Status = "failed"
			actionStep.Error = &errStr
			actionStep.CompletedAt = time.Now().UTC()
			actionStep.DurationMs = time.Since(actStart).Milliseconds()
			e.recordStep(ctx, actionStep)

			return e.applyOnError(ctx, exec, wf, ev, act, errStr, start)
		}
	}

	result, err := executor.Execute(actCtx, ActionRequest{
		WorkflowID:  wf.ID,
		ActionID:    act.ID,
		Event:       ev,
		Config:      renderedConfig,
		Payload:     transformedPayload,
		Credentials: decryptedCreds,
	})

	if e.circuitBreakers != nil {
		if err != nil || !result.Success {
			e.circuitBreakers.RecordFailure(act.Integration)
		} else {
			e.circuitBreakers.RecordSuccess(act.Integration)
		}
	}

	actionStep.CompletedAt = time.Now().UTC()
	actionStep.DurationMs = time.Since(actStart).Milliseconds()

	if err != nil || !result.Success {
		errStr := "action execution failed"
		if err != nil {
			errStr = err.Error()
		}
		actionStep.Status = "failed"
		actionStep.Error = &errStr
		e.recordStep(ctx, actionStep)
		observability.RecordActionFailure()

		stepStates[act.ID] = transformations.StepOutput{
			Status:     "failed",
			Error:      errStr,
			DurationMs: actionStep.DurationMs,
		}

		return e.applyOnError(ctx, exec, wf, ev, act, errStr, start)
	}

	actionStep.Status = "success"
	actionStep.Output = result.Output
	e.recordStep(ctx, actionStep)

	stepStates[act.ID] = transformations.StepOutput{
		Status:     "success",
		Output:     result.Output,
		DurationMs: actionStep.DurationMs,
	}

	return outcomeSuccess, nil
}

func (e *Engine) applyOnError(
	ctx context.Context,
	exec *Execution,
	wf *Workflow,
	ev *event.Event,
	act ActionStep,
	errStr string,
	start time.Time,
) (stepExecutionOutcome, error) {
	switch act.OnError {
	case OnErrorContinue:
		slog.Warn("action failed but on_error is continue; proceeding to next step",
			slog.String("workflow_id", wf.ID),
			slog.String("action_id", act.ID),
			slog.String("error", errStr),
		)
		return outcomeContinue, nil

	case OnErrorSkipRest:
		slog.Info("action failed with on_error=skip_rest; finishing workflow without further steps",
			slog.String("workflow_id", wf.ID),
			slog.String("action_id", act.ID),
			slog.String("error", errStr),
		)
		return outcomeSkipRest, nil

	default: // OnErrorFail or empty
		exec.CurrentStepID = act.ID
		failedExec, err := e.handleExecutionFailure(ctx, exec, wf, ev, act.ID, errStr, start)
		if failedExec != nil {
			exec.Status = failedExec.Status
			exec.NextAttemptAt = failedExec.NextAttemptAt
			exec.Error = failedExec.Error
		}
		return outcomeFailed, err
	}
}

func (e *Engine) executeApprovalStep(
	ctx context.Context,
	wf *Workflow,
	ev *event.Event,
	exec *Execution,
	transformedPayload map[string]any,
	act ActionStep,
	stepStates map[string]transformations.StepOutput,
	renderVars map[string]any,
	start time.Time,
) (stepExecutionOutcome, error) {
	stepStart := time.Now().UTC()
	stepName := act.Name
	if stepName == "" {
		if act.Approval != nil && act.Approval.Title != "" {
			stepName = act.Approval.Title
		} else {
			stepName = fmt.Sprintf("Approval: %s", act.ID)
		}
	}

	// 1. Check if approval record already exists in store
	if e.approvals != nil {
		app, err := e.approvals.GetByExecutionAndStep(ctx, exec.ID, act.ID)
		if err == nil && app != nil {
			switch app.Status {
			case ApprovalApproved:
				appStep := &ExecutionStep{
					ExecutionID: exec.ID,
					StepID:      act.ID,
					StepType:    "approval",
					Name:        stepName,
					Status:      "approved",
					Output: map[string]any{
						"approved_by":   app.ApprovedBy,
						"decision_note": app.DecisionNote,
						"status":        string(app.Status),
					},
					StartedAt:   app.CreatedAt,
					CompletedAt: app.UpdatedAt,
					DurationMs:  app.UpdatedAt.Sub(app.CreatedAt).Milliseconds(),
				}
				e.recordStep(ctx, appStep)
				stepStates[act.ID] = transformations.StepOutput{
					Status:     "approved",
					Output:     appStep.Output,
					DurationMs: appStep.DurationMs,
				}

				// If this was an action with an approval check, now run the underlying action!
				if act.Type != "" && act.Type != StepTypeApproval {
					return e.executeActionStep(ctx, wf, ev, exec, transformedPayload, act, stepStates, renderVars, start)
				}
				return outcomeSuccess, nil

			case ApprovalRejected, ApprovalExpired:
				reason := fmt.Sprintf("human approval %s", app.Status)
				if app.DecisionNote != nil && *app.DecisionNote != "" {
					reason = fmt.Sprintf("%s: %s", reason, *app.DecisionNote)
				}
				appStep := &ExecutionStep{
					ExecutionID: exec.ID,
					StepID:      act.ID,
					StepType:    "approval",
					Name:        stepName,
					Status:      "rejected",
					Error:       &reason,
					StartedAt:   app.CreatedAt,
					CompletedAt: app.UpdatedAt,
					DurationMs:  app.UpdatedAt.Sub(app.CreatedAt).Milliseconds(),
				}
				e.recordStep(ctx, appStep)
				stepStates[act.ID] = transformations.StepOutput{
					Status:     "rejected",
					Error:      reason,
					DurationMs: appStep.DurationMs,
				}

				// Approval rejection is a terminal operator decision: trigger saga compensation immediately
				if e.HasCompensableSteps(wf, exec) {
					compExec, cErr := e.CompensateExecution(ctx, exec.ID, reason)
					if cErr == nil && compExec != nil {
						exec.Status = compExec.Status
						return outcomeFailed, fmt.Errorf("%s", reason)
					}
				}
				exec.Status = ExecutionFailed
				_ = e.executions.UpdateStatus(ctx, exec.ID, ExecutionFailed, &reason, time.Since(start).Milliseconds())
				return outcomeFailed, fmt.Errorf("%s", reason)

			default: // ApprovalPending or ApprovalEscalated
				return outcomeWaiting, nil
			}
		}
	}

	// 2. First time encountering approval: generate cryptographic token & persist approval
	token, tokenHash, tErr := GenerateApprovalToken()
	if tErr != nil {
		return outcomeFailed, fmt.Errorf("failed to generate secure approval token: %w", tErr)
	}

	title := stepName
	desc := ""
	var approverRoles []string
	var approverUsers []string
	var escalationRoles []string
	var escalationContact string
	timeoutAction := "reject"
	timeoutDur := 24 * time.Hour

	if act.Approval != nil {
		if act.Approval.Title != "" {
			title = act.Approval.Title
		}
		desc = act.Approval.Description
		approverRoles = act.Approval.ApproverRoles
		approverUsers = act.Approval.ApproverUsers
		escalationRoles = act.Approval.EscalationRoles
		escalationContact = act.Approval.EscalationContact
		if act.Approval.TimeoutAction != "" {
			timeoutAction = act.Approval.TimeoutAction
		}
		if act.Approval.TimeoutSeconds > 0 {
			timeoutDur = time.Duration(act.Approval.TimeoutSeconds) * time.Second
		} else if act.Approval.Timeout != "" {
			if parsed, pErr := time.ParseDuration(act.Approval.Timeout); pErr == nil && parsed > 0 {
				timeoutDur = parsed
			}
		}
	}

	newApp := &Approval{
		TenantID:          exec.TenantID,
		ExecutionID:       exec.ID,
		StepID:            act.ID,
		WorkflowID:        wf.ID,
		Title:             title,
		Description:       desc,
		Token:             token,
		TokenHash:         tokenHash,
		Status:            ApprovalPending,
		ApproverRoles:     approverRoles,
		ApproverUsers:     approverUsers,
		ExpiresAt:         time.Now().UTC().Add(timeoutDur),
		TimeoutAction:     timeoutAction,
		EscalationRoles:   escalationRoles,
		EscalationContact: escalationContact,
	}

	var createdApp *Approval
	if e.approvals != nil {
		created, cErr := e.approvals.Create(ctx, newApp)
		if cErr != nil {
			return outcomeFailed, fmt.Errorf("failed to persist approval record: %w", cErr)
		}
		createdApp = created
	} else {
		createdApp = newApp
	}

	// Update execution status to waiting_approval
	exec.CurrentStepID = act.ID
	exec.Status = ExecutionWaitingApproval
	e.updateStatus(ctx, exec.ID, ExecutionWaitingApproval, nil, time.Since(start).Milliseconds())

	// Record initial ExecutionStep with status waiting_approval
	appStep := &ExecutionStep{
		ExecutionID: exec.ID,
		StepID:      act.ID,
		StepType:    "approval",
		Name:        stepName,
		Status:      "waiting_approval",
		StartedAt:   stepStart,
		Input: map[string]any{
			"approval_id":    createdApp.ID,
			"title":          createdApp.Title,
			"approver_roles": createdApp.ApproverRoles,
			"expires_at":     createdApp.ExpiresAt,
		},
	}
	e.recordStep(ctx, appStep)

	if e.listener != nil {
		e.listener.OnExecutionWaitingApproval(exec, createdApp)
	}

	slog.Info("workflow execution entered durable waiting_approval state",
		slog.String("workflow_id", wf.ID),
		slog.String("execution_id", exec.ID),
		slog.String("approval_id", createdApp.ID),
		slog.Time("expires_at", createdApp.ExpiresAt),
	)

	return outcomeWaiting, nil
}
