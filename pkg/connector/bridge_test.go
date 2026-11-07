package connector

import (
	"context"
	"encoding/json"
	"testing"

	"kerob/internal/adapters"
	"kerob/internal/core/event"
	"kerob/internal/core/workflow"
)

type actionMockConnector struct {
	mockConnector
}

func (a *actionMockConnector) ExecuteAction(ctx context.Context, req ActionRequest) (ActionResult, error) {
	return ActionResult{
		Success: true,
		Output: map[string]any{
			"status": "ok",
		},
	}, nil
}

func TestBridge(t *testing.T) {
	conn := &actionMockConnector{mockConnector{id: "mock_action"}}

	// ToAdapterManifest
	m := ToAdapterManifest(conn)
	if m.ID != "mock_action" {
		t.Errorf("expected ID mock_action, got %s", m.ID)
	}

	// ActionBridge
	bridge, err := NewActionBridge(conn)
	if err != nil {
		t.Fatalf("NewActionBridge failed: %v", err)
	}

	if bridge.Name() == "" || bridge.Type() != "mock_action" {
		t.Errorf("unexpected bridge name or type: %s, %s", bridge.Name(), bridge.Type())
	}

	if err := bridge.Validate(json.RawMessage(`{"key":"val"}`)); err != nil {
		t.Errorf("Validate failed: %v", err)
	}
	if err := bridge.Validate(nil); err != nil {
		t.Errorf("Validate nil failed: %v", err)
	}

	res, err := bridge.Execute(context.Background(), workflow.ActionRequest{
		WorkflowID: "wf-1",
		ActionID:   "act-1",
		Event: &event.Event{
			Payload: json.RawMessage(`{"evt":"test"}`),
		},
	})
	if err != nil || !res.Success {
		t.Fatalf("Execute failed: %v", err)
	}

	// BridgeToSystem
	adapterReg := adapters.NewRegistry()
	actionReg := workflow.NewActionRegistry()

	if err := BridgeToSystem(conn, adapterReg, actionReg); err != nil {
		t.Fatalf("BridgeToSystem failed: %v", err)
	}
	if err := BridgeToSystem(nil, adapterReg, actionReg); err == nil {
		t.Error("expected error for nil connector")
	}

	// Non-action connector
	nonAction := &mockConnector{id: "plain"}
	_, err = NewActionBridge(nonAction)
	if err == nil {
		t.Error("expected error creating ActionBridge for non-action provider")
	}
}
