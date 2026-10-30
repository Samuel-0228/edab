package actions

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"kerob/internal/core/workflow"
)

const defaultMaxCallDepth = 5

// WorkflowInvoker defines the contract for chaining execution into another workflow.
type WorkflowInvoker interface {
	InvokeWorkflow(ctx context.Context, targetWorkflowID string, payload map[string]any, metadata map[string]string) (string, error)
}

// CallWorkflowActionConfig parameters for invoking a downstream workflow.
type CallWorkflowActionConfig struct {
	WorkflowID string         `json:"workflow_id"`
	Payload    map[string]any `json:"payload,omitempty"`
	MaxDepth   int            `json:"max_depth,omitempty"`
}

// CallWorkflowActionExecutor invokes a child workflow with cycle and recursion depth guards.
type CallWorkflowActionExecutor struct {
	invoker WorkflowInvoker
}

func NewCallWorkflowActionExecutor(invoker WorkflowInvoker) *CallWorkflowActionExecutor {
	return &CallWorkflowActionExecutor{invoker: invoker}
}

func (c *CallWorkflowActionExecutor) Name() string { return "Call Workflow" }
func (c *CallWorkflowActionExecutor) Type() string { return "call_workflow" }

func (c *CallWorkflowActionExecutor) Validate(config json.RawMessage) error {
	var cfg CallWorkflowActionConfig
	if err := json.Unmarshal(config, &cfg); err != nil {
		return fmt.Errorf("invalid call_workflow config: %w", err)
	}
	if strings.TrimSpace(cfg.WorkflowID) == "" {
		return fmt.Errorf("call_workflow requires 'workflow_id'")
	}
	return nil
}

func (c *CallWorkflowActionExecutor) Execute(ctx context.Context, req workflow.ActionRequest) (workflow.ActionResult, error) {
	var cfg CallWorkflowActionConfig
	if err := json.Unmarshal(req.Config, &cfg); err != nil {
		return workflow.ActionResult{Success: false}, fmt.Errorf("failed to parse call_workflow config: %w", err)
	}

	targetID := strings.TrimSpace(cfg.WorkflowID)
	maxDepth := cfg.MaxDepth
	if maxDepth <= 0 {
		maxDepth = defaultMaxCallDepth
	}

	currentDepth := 0
	callStack := make([]string, 0)

	meta := make(map[string]string)
	if req.Event != nil && len(req.Event.Metadata) > 0 {
		var existingMeta map[string]string
		if err := json.Unmarshal(req.Event.Metadata, &existingMeta); err == nil {
			for k, v := range existingMeta {
				meta[k] = v
			}
		}
	}

	if depthStr, ok := meta["workflow_depth"]; ok && depthStr != "" {
		if d, err := strconv.Atoi(depthStr); err == nil {
			currentDepth = d
		}
	}

	if stackStr, ok := meta["workflow_call_stack"]; ok && stackStr != "" {
		callStack = strings.Split(stackStr, ",")
	}

	// Loop & recursion depth guards
	if currentDepth >= maxDepth {
		return workflow.ActionResult{Success: false}, fmt.Errorf("workflow execution loop guard: max call depth reached (%d >= %d)", currentDepth, maxDepth)
	}

	for _, prevID := range callStack {
		if prevID == targetID {
			return workflow.ActionResult{Success: false}, fmt.Errorf("workflow recursion loop detected: %s already present in call stack [%s]", targetID, strings.Join(callStack, " -> "))
		}
	}

	nextStack := append(callStack, targetID)
	meta["workflow_depth"] = strconv.Itoa(currentDepth + 1)
	meta["workflow_call_stack"] = strings.Join(nextStack, ",")
	meta["parent_execution_id"] = req.ActionID
	meta["parent_workflow_id"] = req.WorkflowID

	payloadToPass := cfg.Payload
	if payloadToPass == nil {
		payloadToPass = req.Payload
	}

	if c.invoker == nil {
		return workflow.ActionResult{Success: false}, fmt.Errorf("workflow invoker is not configured")
	}

	childExecutionID, err := c.invoker.InvokeWorkflow(ctx, targetID, payloadToPass, meta)
	if err != nil {
		return workflow.ActionResult{Success: false}, fmt.Errorf("failed to invoke workflow %s: %w", targetID, err)
	}

	outBytes, _ := json.Marshal(map[string]any{
		"child_workflow_id":  targetID,
		"child_execution_id": childExecutionID,
		"call_depth":         currentDepth + 1,
	})

	return workflow.ActionResult{
		Success:    true,
		ExternalID: childExecutionID,
		Output:     outBytes,
		Metadata: map[string]any{
			"child_execution_id": childExecutionID,
			"call_depth":         currentDepth + 1,
		},
	}, nil
}
