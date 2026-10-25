package workflow

import (
	"encoding/json"
	"fmt"

	"kerob/internal/core/conditions"
	"kerob/internal/core/transformations"
)

// workflowJSONPayloads encapsulates the serialized JSON columns for a workflow in PostgreSQL.
type workflowJSONPayloads struct {
	Trigger         []byte
	Conditions      []byte
	Transformations []byte
	Actions         []byte
}

// marshalWorkflowComponents serializes a workflow's trigger, conditions, transformations, and actions to JSON.
func marshalWorkflowComponents(wf *Workflow) (workflowJSONPayloads, error) {
	trigJSON, err := json.Marshal(wf.Trigger)
	if err != nil {
		return workflowJSONPayloads{}, fmt.Errorf("failed to marshal trigger config: %w", err)
	}
	condJSON, err := json.Marshal(wf.Conditions)
	if err != nil {
		return workflowJSONPayloads{}, fmt.Errorf("failed to marshal conditions: %w", err)
	}
	transJSON, err := json.Marshal(wf.Transformations)
	if err != nil {
		return workflowJSONPayloads{}, fmt.Errorf("failed to marshal transformations: %w", err)
	}
	actsJSON, err := json.Marshal(wf.Actions)
	if err != nil {
		return workflowJSONPayloads{}, fmt.Errorf("failed to marshal actions: %w", err)
	}

	return workflowJSONPayloads{
		Trigger:         trigJSON,
		Conditions:      condJSON,
		Transformations: transJSON,
		Actions:         actsJSON,
	}, nil
}

// unmarshalWorkflowComponents populates a workflow's trigger, conditions, transformations, and actions from JSON columns.
func unmarshalWorkflowComponents(wf *Workflow, trigJSON, condJSON, transJSON, actsJSON []byte) {
	if len(trigJSON) > 0 {
		_ = json.Unmarshal(trigJSON, &wf.Trigger)
	}
	if len(condJSON) > 0 && string(condJSON) != "null" && string(condJSON) != "{}" {
		var c conditions.Condition
		if err := json.Unmarshal(condJSON, &c); err == nil {
			wf.Conditions = &c
		}
	}
	if len(transJSON) > 0 && string(transJSON) != "null" && string(transJSON) != "{}" {
		var t transformations.Transformation
		if err := json.Unmarshal(transJSON, &t); err == nil {
			wf.Transformations = &t
		}
	}
	if len(actsJSON) > 0 {
		_ = json.Unmarshal(actsJSON, &wf.Actions)
	}
}

// marshalStepIO serializes execution step input and output data into JSON bytes.
func marshalStepIO(input, output any) (inJSON, outJSON []byte) {
	inJSON, _ = json.Marshal(input)
	outJSON, _ = json.Marshal(output)
	return inJSON, outJSON
}

// unmarshalStepIO deserializes execution step input and output data from JSON bytes.
func unmarshalStepIO(step *ExecutionStep, inJSON, outJSON []byte) {
	if len(inJSON) > 0 && string(inJSON) != "null" {
		_ = json.Unmarshal(inJSON, &step.Input)
	}
	if len(outJSON) > 0 && string(outJSON) != "null" {
		_ = json.Unmarshal(outJSON, &step.Output)
	}
}
