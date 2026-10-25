package workflow

import (
	"encoding/json"
	"testing"

	"kerob/internal/core/conditions"
	"kerob/internal/core/transformations"
)

func TestMarshalAndUnmarshalWorkflowComponents(t *testing.T) {
	wf := &Workflow{
		Name:    "Test Marshal",
		Enabled: true,
		Trigger: TriggerConfig{
			Type:       "webhook",
			EventTypes: []string{"test.event"},
			Config:     json.RawMessage(`{"secret":"xyz"}`),
		},
		Conditions: &conditions.Condition{
			Field:    "payload.level",
			Operator: conditions.OpEquals,
			Value:    "critical",
		},
		Transformations: &transformations.Transformation{
			Type:     "template",
			Template: "Message: {{payload.text}}",
		},
		Actions: []ActionStep{
			{
				ID:          "act-1",
				Integration: "discord",
				ActionType:  "send_message",
				Config:      json.RawMessage(`{"content":"test"}`),
			},
		},
	}

	payloads, err := marshalWorkflowComponents(wf)
	if err != nil {
		t.Fatalf("marshalWorkflowComponents failed: %v", err)
	}

	if len(payloads.Trigger) == 0 || len(payloads.Conditions) == 0 || len(payloads.Transformations) == 0 || len(payloads.Actions) == 0 {
		t.Fatalf("expected non-empty JSON components, got: %+v", payloads)
	}

	restored := &Workflow{Name: "Restored"}
	unmarshalWorkflowComponents(restored, payloads.Trigger, payloads.Conditions, payloads.Transformations, payloads.Actions)

	if restored.Trigger.Type != "webhook" {
		t.Errorf("expected trigger type 'webhook', got %s", restored.Trigger.Type)
	}
	if restored.Conditions == nil || restored.Conditions.Field != "payload.level" {
		t.Errorf("expected condition field 'payload.level', got %+v", restored.Conditions)
	}
	if restored.Transformations == nil || restored.Transformations.Template != "Message: {{payload.text}}" {
		t.Errorf("expected transformation template, got %+v", restored.Transformations)
	}
	if len(restored.Actions) != 1 || restored.Actions[0].Integration != "discord" {
		t.Errorf("expected restored action, got %+v", restored.Actions)
	}

	// Test unmarshaling empty / null values safely
	emptyRestored := &Workflow{Name: "Empty Restored"}
	unmarshalWorkflowComponents(emptyRestored, nil, []byte("null"), []byte("{}"), nil)
	if emptyRestored.Conditions != nil {
		t.Errorf("expected nil conditions for 'null' JSON, got %+v", emptyRestored.Conditions)
	}
	if emptyRestored.Transformations != nil {
		t.Errorf("expected nil transformations for '{}' JSON, got %+v", emptyRestored.Transformations)
	}
}

func TestMarshalAndUnmarshalStepIO(t *testing.T) {
	input := map[string]any{"source": "test", "num": 42}
	output := map[string]any{"result": "ok"}

	inJSON, outJSON := marshalStepIO(input, output)
	if len(inJSON) == 0 || len(outJSON) == 0 {
		t.Fatalf("expected non-empty step IO JSON, got in=%s out=%s", inJSON, outJSON)
	}

	step := &ExecutionStep{ID: "step-1"}
	unmarshalStepIO(step, inJSON, outJSON)

	inMap, ok := step.Input.(map[string]any)
	if !ok || inMap["source"] != "test" {
		t.Errorf("unexpected unmarshaled step input: %+v", step.Input)
	}
	outMap, ok := step.Output.(map[string]any)
	if !ok || outMap["result"] != "ok" {
		t.Errorf("unexpected unmarshaled step output: %+v", step.Output)
	}

	// Null / empty IO safety
	emptyStep := &ExecutionStep{ID: "empty-step"}
	unmarshalStepIO(emptyStep, nil, []byte("null"))
	if emptyStep.Input != nil || emptyStep.Output != nil {
		t.Errorf("expected nil IO for null inputs, got in=%+v out=%+v", emptyStep.Input, emptyStep.Output)
	}
}
