package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"kerob/internal/core/conditions"
	"kerob/internal/core/event"
	"kerob/internal/core/transformations"
)

// executeBranchStep evaluates conditional cases and executes the matching branch or default steps.
func (e *Engine) executeBranchStep(
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
		stepName = fmt.Sprintf("Branch: %s", act.ID)
	}

	branchStep := &ExecutionStep{
		ExecutionID: exec.ID,
		StepID:      act.ID,
		StepType:    "branch",
		Name:        stepName,
		StartedAt:   stepStart,
	}

	for idx, bCase := range act.Cases {
		matched, err := conditions.Evaluate(bCase.When, ev)
		if err != nil {
			errStr := fmt.Sprintf("branch case condition evaluation failed: %v", err)
			branchStep.Status = "failed"
			branchStep.Error = &errStr
			branchStep.CompletedAt = time.Now().UTC()
			branchStep.DurationMs = time.Since(stepStart).Milliseconds()
			e.recordStep(ctx, branchStep)
			return e.applyOnError(ctx, exec, wf, ev, act, errStr, start)
		}

		if matched {
			branchStep.Status = "success"
			branchStep.Output = map[string]any{"matched_case": idx}
			branchStep.CompletedAt = time.Now().UTC()
			branchStep.DurationMs = time.Since(stepStart).Milliseconds()
			e.recordStep(ctx, branchStep)

			stepStates[act.ID] = transformations.StepOutput{
				Status:     "success",
				Output:     branchStep.Output,
				DurationMs: branchStep.DurationMs,
			}

			return e.executeStepsList(ctx, wf, ev, exec, transformedPayload, bCase.Steps, stepStates, renderVars, start)
		}
	}

	// No case matched: check default steps
	if len(act.Default) > 0 {
		branchStep.Status = "success"
		branchStep.Output = map[string]any{"matched_case": "default"}
		branchStep.CompletedAt = time.Now().UTC()
		branchStep.DurationMs = time.Since(stepStart).Milliseconds()
		e.recordStep(ctx, branchStep)

		stepStates[act.ID] = transformations.StepOutput{
			Status:     "success",
			Output:     branchStep.Output,
			DurationMs: branchStep.DurationMs,
		}

		return e.executeStepsList(ctx, wf, ev, exec, transformedPayload, act.Default, stepStates, renderVars, start)
	}

	// Clean skip when no branch matches and no default is configured
	branchStep.Status = "skipped"
	branchStep.CompletedAt = time.Now().UTC()
	branchStep.DurationMs = time.Since(stepStart).Milliseconds()
	e.recordStep(ctx, branchStep)

	stepStates[act.ID] = transformations.StepOutput{
		Status:     "skipped",
		DurationMs: branchStep.DurationMs,
	}

	return outcomeSuccess, nil
}

// executeForEachStep iterates over an array in the payload/step outputs with a safety cap.
func (e *Engine) executeForEachStep(
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
		stepName = fmt.Sprintf("ForEach: %s", act.ID)
	}

	forStep := &ExecutionStep{
		ExecutionID: exec.ID,
		StepID:      act.ID,
		StepType:    "for_each",
		Name:        stepName,
		StartedAt:   stepStart,
	}

	renderCtx := transformations.RenderContext{
		Event:     ev,
		Steps:     stepStates,
		Variables: renderVars,
	}

	rawItems, ok := transformations.ResolvePath(act.Items, renderCtx)
	var items []any
	if ok && rawItems != nil {
		switch v := rawItems.(type) {
		case []any:
			items = v
		case json.RawMessage:
			_ = json.Unmarshal(v, &items)
		case []byte:
			_ = json.Unmarshal(v, &items)
		}
	}

	maxIter := act.MaxIterations
	if maxIter <= 0 {
		maxIter = 100
	}
	if maxIter > 1000 {
		maxIter = 1000
	}
	if len(items) > maxIter {
		slog.Warn("for_each items length exceeds max_iterations cap; capping iterations",
			slog.String("workflow_id", wf.ID),
			slog.String("step_id", act.ID),
			slog.Int("total_items", len(items)),
			slog.Int("cap", maxIter),
		)
		items = items[:maxIter]
	}

	itemVar := act.ItemVar
	if itemVar == "" {
		itemVar = "item"
	}
	idxVar := act.IndexVar
	if idxVar == "" {
		idxVar = "index"
	}

	for idx, itm := range items {
		childVars := make(map[string]any, len(renderVars)+2)
		for k, v := range renderVars {
			childVars[k] = v
		}
		childVars[itemVar] = itm
		childVars[idxVar] = idx

		for sIdx, sub := range act.Steps {
			subCopy := sub
			if subCopy.ID == "" {
				subCopy.ID = fmt.Sprintf("%s_%d_step_%d", act.ID, idx, sIdx+1)
			} else {
				subCopy.ID = fmt.Sprintf("%s_%d_%s", act.ID, idx, sub.ID)
			}

			if prev, ok := stepStates[subCopy.ID]; ok && (prev.Status == "success" || prev.Status == "skipped") {
				continue
			}

			outcome, err := e.executeStep(ctx, wf, ev, exec, transformedPayload, subCopy, stepStates, childVars, start)
			if outcome == outcomeFailed {
				forStep.Status = "failed"
				forStep.CompletedAt = time.Now().UTC()
				forStep.DurationMs = time.Since(stepStart).Milliseconds()
				e.recordStep(ctx, forStep)
				return outcomeFailed, err
			}
			if outcome == outcomeSkipRest {
				break
			}
			if outcome == outcomeWaiting {
				return outcomeWaiting, nil
			}
		}
	}

	forStep.Status = "success"
	forStep.Output = map[string]any{"count": len(items)}
	forStep.CompletedAt = time.Now().UTC()
	forStep.DurationMs = time.Since(stepStart).Milliseconds()
	e.recordStep(ctx, forStep)

	stepStates[act.ID] = transformations.StepOutput{
		Status:     "success",
		Output:     forStep.Output,
		DurationMs: forStep.DurationMs,
	}

	return outcomeSuccess, nil
}

// executeParallelStep runs independent actions concurrently with a configurable concurrency limit.
func (e *Engine) executeParallelStep(
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
		stepName = fmt.Sprintf("Parallel: %s", act.ID)
	}

	parStep := &ExecutionStep{
		ExecutionID: exec.ID,
		StepID:      act.ID,
		StepType:    "parallel",
		Name:        stepName,
		StartedAt:   stepStart,
	}

	concurrency := act.Concurrency
	if concurrency <= 0 {
		concurrency = len(act.Steps)
	}
	if concurrency <= 0 {
		concurrency = 1
	}
	if concurrency > 20 {
		concurrency = 20
	}

	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var firstErr error
	finalOutcome := outcomeSuccess

	for i, sub := range act.Steps {
		subCopy := sub
		if subCopy.ID == "" {
			subCopy.ID = fmt.Sprintf("%s_branch_%d", act.ID, i+1)
		}

		if prev, ok := stepStates[subCopy.ID]; ok && (prev.Status == "success" || prev.Status == "skipped") {
			continue
		}

		mu.Lock()
		localStates := make(map[string]transformations.StepOutput, len(stepStates))
		for k, v := range stepStates {
			localStates[k] = v
		}
		mu.Unlock()

		wg.Add(1)
		go func(s ActionStep, states map[string]transformations.StepOutput) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			branchStates := make(map[string]transformations.StepOutput, len(states))
			for k, v := range states {
				branchStates[k] = v
			}

			outcome, err := e.executeStep(ctx, wf, ev, exec, transformedPayload, s, branchStates, renderVars, start)
			mu.Lock()
			defer mu.Unlock()
			if outcome == outcomeFailed || err != nil {
				if firstErr == nil {
					firstErr = err
					finalOutcome = outcomeFailed
				}
			}
			for k, v := range branchStates {
				stepStates[k] = v
			}
		}(subCopy, localStates)
	}

	wg.Wait()

	parStep.CompletedAt = time.Now().UTC()
	parStep.DurationMs = time.Since(stepStart).Milliseconds()

	if finalOutcome == outcomeFailed {
		errStr := "one or more parallel steps failed"
		if firstErr != nil {
			errStr = firstErr.Error()
		}
		parStep.Status = "failed"
		parStep.Error = &errStr
		e.recordStep(ctx, parStep)
		return e.applyOnError(ctx, exec, wf, ev, act, errStr, start)
	}

	parStep.Status = "success"
	parStep.Output = map[string]any{"branches_count": len(act.Steps)}
	e.recordStep(ctx, parStep)

	stepStates[act.ID] = transformations.StepOutput{
		Status:     "success",
		Output:     parStep.Output,
		DurationMs: parStep.DurationMs,
	}

	return outcomeSuccess, nil
}

// executeDelayStep pauses execution via scheduled resume without blocking worker threads.
func (e *Engine) executeDelayStep(
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
		stepName = fmt.Sprintf("Delay: %s", act.ID)
	}

	var d time.Duration
	if act.Duration != "" {
		parsed, err := time.ParseDuration(act.Duration)
		if err == nil {
			d = parsed
		}
	}
	if d <= 0 && act.DurationSec > 0 {
		d = time.Duration(act.DurationSec) * time.Second
	}

	// If already completed in a prior run
	if prev, ok := stepStates[act.ID]; ok && prev.Status == "success" {
		return outcomeSuccess, nil
	}

	// If resuming and the delay period has already passed
	if exec.CurrentStepID == act.ID && (time.Now().UTC().After(exec.NextAttemptAt) || time.Now().UTC().Equal(exec.NextAttemptAt)) {
		delayStep := &ExecutionStep{
			ExecutionID: exec.ID,
			StepID:      act.ID,
			StepType:    "delay",
			Name:        stepName,
			Status:      "success",
			StartedAt:   stepStart,
			CompletedAt: time.Now().UTC(),
			DurationMs:  d.Milliseconds(),
		}
		e.recordStep(ctx, delayStep)

		stepStates[act.ID] = transformations.StepOutput{
			Status:     "success",
			DurationMs: d.Milliseconds(),
		}
		return outcomeSuccess, nil
	}

	if d <= 0 {
		stepStates[act.ID] = transformations.StepOutput{Status: "success"}
		return outcomeSuccess, nil
	}

	// First time reaching delay step: schedule resume without blocking worker
	resumeAt := time.Now().UTC().Add(d)
	exec.NextAttemptAt = resumeAt
	exec.CurrentStepID = act.ID
	exec.Status = ExecutionWaiting
	e.updateStatus(ctx, exec.ID, ExecutionWaiting, nil, time.Since(start).Milliseconds())

	delayStep := &ExecutionStep{
		ExecutionID: exec.ID,
		StepID:      act.ID,
		StepType:    "delay",
		Name:        stepName,
		Status:      "waiting",
		StartedAt:   stepStart,
	}
	e.recordStep(ctx, delayStep)

	return outcomeWaiting, nil
}

// executeStepsList executes a sequential sequence of steps (such as in a branch case or default).
func (e *Engine) executeStepsList(
	ctx context.Context,
	wf *Workflow,
	ev *event.Event,
	exec *Execution,
	transformedPayload map[string]any,
	steps []ActionStep,
	stepStates map[string]transformations.StepOutput,
	renderVars map[string]any,
	start time.Time,
) (stepExecutionOutcome, error) {
	for i, step := range steps {
		if step.ID == "" {
			step.ID = fmt.Sprintf("substep_%d", i+1)
		}

		if prev, ok := stepStates[step.ID]; ok && (prev.Status == "success" || prev.Status == "skipped") {
			continue
		}

		outcome, err := e.executeStep(ctx, wf, ev, exec, transformedPayload, step, stepStates, renderVars, start)
		if outcome == outcomeFailed {
			return outcomeFailed, err
		}
		if outcome == outcomeSkipRest {
			return outcomeSkipRest, nil
		}
		if outcome == outcomeWaiting {
			return outcomeWaiting, nil
		}
	}
	return outcomeSuccess, nil
}
