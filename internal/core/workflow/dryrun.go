package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"kerob/internal/core/conditions"
	"kerob/internal/core/event"
	"kerob/internal/core/transformations"
)

// DryRunStepTrace holds debug trace information for a single step in a dry-run test.
type DryRunStepTrace struct {
	StepID         string          `json:"step_id"`
	StepType       string          `json:"step_type"`
	Name           string          `json:"name"`
	Status         string          `json:"status"` // "success", "skipped", "failed"
	Input          any             `json:"input,omitempty"`
	RenderedConfig json.RawMessage `json:"rendered_config,omitempty"`
	Output         any             `json:"output,omitempty"`
	Error          string          `json:"error,omitempty"`
	DurationMs     int64           `json:"duration_ms"`
}

// DryRunResult holds the complete evaluation trace for a workflow dry-run.
type DryRunResult struct {
	WorkflowID         string            `json:"workflow_id"`
	EventID            string            `json:"event_id"`
	Status             string            `json:"status"` // "success", "conditions_failed", "trigger_mismatch"
	TriggerMatched     bool              `json:"trigger_matched"`
	ConditionsPassed   bool              `json:"conditions_passed"`
	TransformedPayload map[string]any    `json:"transformed_payload,omitempty"`
	Traces             []DryRunStepTrace `json:"traces"`
	TotalDurationMs    int64             `json:"total_duration_ms"`
}

// ExecuteDryRun evaluates a workflow's trigger, conditions, transforms, and action configs
// without executing external outbound network calls when mockActions is true.
func (e *Engine) ExecuteDryRun(ctx context.Context, wf *Workflow, ev *event.Event, mockActions bool) (*DryRunResult, error) {
	start := time.Now()
	res := &DryRunResult{
		WorkflowID: wf.ID,
		EventID:    ev.ID,
		Traces:     make([]DryRunStepTrace, 0),
	}

	// Step 1: Trigger Matching
	trigStart := time.Now()
	res.TriggerMatched = wf.Trigger.Matches(ev)
	res.Traces = append(res.Traces, DryRunStepTrace{
		StepID:     "trigger",
		StepType:   "trigger",
		Name:       fmt.Sprintf("Trigger: %s", wf.Trigger.Type),
		Status:     boolStatus(res.TriggerMatched),
		Input:      map[string]any{"source_id": ev.SourceID, "event_type": ev.EventType},
		Output:     map[string]any{"matched": res.TriggerMatched},
		DurationMs: time.Since(trigStart).Milliseconds(),
	})

	if !res.TriggerMatched {
		res.Status = "trigger_mismatch"
		res.TotalDurationMs = time.Since(start).Milliseconds()
		return res, nil
	}

	// Step 2: Conditions Evaluation
	condStart := time.Now()
	res.ConditionsPassed = true
	if wf.Conditions != nil {
		passed, err := conditions.Evaluate(wf.Conditions, ev)
		if err != nil {
			res.Traces = append(res.Traces, DryRunStepTrace{
				StepID:     "condition",
				StepType:   "condition",
				Name:       "IF Conditions",
				Status:     "failed",
				Error:      err.Error(),
				DurationMs: time.Since(condStart).Milliseconds(),
			})
			res.ConditionsPassed = false
			res.Status = "conditions_failed"
			res.TotalDurationMs = time.Since(start).Milliseconds()
			return res, nil
		}
		res.ConditionsPassed = passed
		res.Traces = append(res.Traces, DryRunStepTrace{
			StepID:     "condition",
			StepType:   "condition",
			Name:       "IF Conditions",
			Status:     boolStatus(passed),
			Input:      wf.Conditions,
			Output:     map[string]any{"passed": passed},
			DurationMs: time.Since(condStart).Milliseconds(),
		})
	}

	if !res.ConditionsPassed {
		res.Status = "conditions_failed"
		res.TotalDurationMs = time.Since(start).Milliseconds()
		return res, nil
	}

	// Step 3: Transformation
	transStart := time.Now()
	var rawPayload map[string]any
	_ = json.Unmarshal(ev.Payload, &rawPayload)
	transformed := rawPayload

	if wf.Transformations != nil {
		out, err := transformations.Transform(wf.Transformations, ev)
		if err != nil {
			res.Traces = append(res.Traces, DryRunStepTrace{
				StepID:     "transformation",
				StepType:   "transformation",
				Name:       fmt.Sprintf("TRANSFORM: %s", wf.Transformations.Type),
				Status:     "failed",
				Error:      err.Error(),
				DurationMs: time.Since(transStart).Milliseconds(),
			})
			res.Status = "transform_failed"
			res.TotalDurationMs = time.Since(start).Milliseconds()
			return res, nil
		}
		transformed = out
		res.TransformedPayload = out
		res.Traces = append(res.Traces, DryRunStepTrace{
			StepID:     "transformation",
			StepType:   "transformation",
			Name:       fmt.Sprintf("TRANSFORM: %s", wf.Transformations.Type),
			Status:     "success",
			Input:      wf.Transformations,
			Output:     out,
			DurationMs: time.Since(transStart).Milliseconds(),
		})
	} else {
		res.TransformedPayload = transformed
	}

	// Step 4: Step Trace & Action Config Rendering
	stepStates := make(map[string]transformations.StepOutput)
	renderVars := make(map[string]any)

	for i, act := range wf.Actions {
		stepStart := time.Now()
		stepID := act.ID
		if stepID == "" {
			stepID = fmt.Sprintf("step_%d", i+1)
		}

		renderCtx := transformations.RenderContext{
			Event:     ev,
			Steps:     stepStates,
			Secrets:   nil, // Never expose decrypted secrets in dry-run traces
			Variables: renderVars,
		}

		renderedConfig, err := transformations.RenderJSONConfigWithContext(act.Config, renderCtx)
		if err != nil {
			res.Traces = append(res.Traces, DryRunStepTrace{
				StepID:     stepID,
				StepType:   "action",
				Name:       act.Name,
				Status:     "failed",
				Error:      err.Error(),
				DurationMs: time.Since(stepStart).Milliseconds(),
			})
			break
		}

		trace := DryRunStepTrace{
			StepID:         stepID,
			StepType:       "action",
			Name:           act.Name,
			Status:         "success",
			RenderedConfig: renderedConfig,
			Output: map[string]any{
				"mocked":      mockActions,
				"integration": act.Integration,
				"action_type": act.ActionType,
			},
			DurationMs: time.Since(stepStart).Milliseconds(),
		}

		if !mockActions {
			// Actually execute via registry
			executor, err := e.actions.Get(act.Integration)
			if err != nil {
				trace.Status = "failed"
				trace.Error = err.Error()
			} else {
				actRes, execErr := executor.Execute(ctx, ActionRequest{
					WorkflowID: wf.ID,
					ActionID:   stepID,
					Event:      ev,
					Config:     renderedConfig,
					Payload:    transformed,
				})
				if execErr != nil || !actRes.Success {
					trace.Status = "failed"
					if execErr != nil {
						trace.Error = execErr.Error()
					}
				} else {
					trace.Output = actRes.Output
				}
			}
		}

		res.Traces = append(res.Traces, trace)
		stepStates[stepID] = transformations.StepOutput{
			Status:     trace.Status,
			Output:     trace.Output,
			DurationMs: trace.DurationMs,
		}
	}

	res.Status = "success"
	res.TotalDurationMs = time.Since(start).Milliseconds()
	return res, nil
}

func boolStatus(b bool) string {
	if b {
		return "success"
	}
	return "skipped"
}
