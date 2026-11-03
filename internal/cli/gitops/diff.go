package gitops

import (
	"encoding/json"
	"fmt"
	"strings"

	"kerob/internal/core/workflow"
)

// WorkflowDiffResult contains the computed differences between local and remote definitions.
type WorkflowDiffResult struct {
	WorkflowID string   `json:"workflow_id"`
	Name       string   `json:"name"`
	HasChanges bool     `json:"has_changes"`
	Changes    []string `json:"changes"`
	Unified    string   `json:"unified"`
}

// DiffWorkflow compares a local workflow specification with its existing remote state.
func DiffWorkflow(local *workflow.Workflow, remote *workflow.Workflow) WorkflowDiffResult {
	res := WorkflowDiffResult{
		Name: local.Name,
	}

	if remote == nil {
		res.WorkflowID = local.ID
		if res.WorkflowID == "" {
			res.WorkflowID = "new"
		}
		res.HasChanges = true
		res.Changes = []string{"New workflow (not present on remote server)"}
		res.Unified = fmt.Sprintf("+ [NEW WORKFLOW] %s (trigger: %s, actions: %d)", local.Name, local.Trigger.Type, len(local.Actions))
		return res
	}

	res.WorkflowID = remote.ID
	var changes []string
	var unified strings.Builder

	unified.WriteString(fmt.Sprintf("--- remote/%s (%s)\n", remote.ID, remote.Name))
	unified.WriteString(fmt.Sprintf("+++ local/%s (%s)\n", local.Name, local.Name))

	if local.Name != remote.Name {
		changes = append(changes, fmt.Sprintf("Name: %q -> %q", remote.Name, local.Name))
		unified.WriteString(fmt.Sprintf("- name: %q\n+ name: %q\n", remote.Name, local.Name))
	}
	if local.Description != remote.Description {
		changes = append(changes, fmt.Sprintf("Description: %q -> %q", remote.Description, local.Description))
		unified.WriteString(fmt.Sprintf("- description: %q\n+ description: %q\n", remote.Description, local.Description))
	}
	if local.Enabled != remote.Enabled {
		changes = append(changes, fmt.Sprintf("Enabled: %t -> %t", remote.Enabled, local.Enabled))
		unified.WriteString(fmt.Sprintf("- enabled: %t\n+ enabled: %t\n", remote.Enabled, local.Enabled))
	}
	if local.Trigger.Type != remote.Trigger.Type {
		changes = append(changes, fmt.Sprintf("Trigger Type: %q -> %q", remote.Trigger.Type, local.Trigger.Type))
		unified.WriteString(fmt.Sprintf("- trigger.type: %q\n+ trigger.type: %q\n", remote.Trigger.Type, local.Trigger.Type))
	}

	// Compare actions
	if len(local.Actions) != len(remote.Actions) {
		changes = append(changes, fmt.Sprintf("Action Count: %d steps -> %d steps", len(remote.Actions), len(local.Actions)))
		unified.WriteString(fmt.Sprintf("- actions count: %d\n+ actions count: %d\n", len(remote.Actions), len(local.Actions)))
	} else {
		for i := range local.Actions {
			la := &local.Actions[i]
			ra := &remote.Actions[i]
			if la.ID != ra.ID || la.Integration != ra.Integration || la.ActionType != ra.ActionType || la.Name != ra.Name {
				changes = append(changes, fmt.Sprintf("Step %d (%s) modified", i+1, la.Name))
				unified.WriteString(fmt.Sprintf("@@ Step %d: %s @@\n", i+1, la.Name))
				if la.Name != ra.Name {
					unified.WriteString(fmt.Sprintf("-  name: %q\n+  name: %q\n", ra.Name, la.Name))
				}
				if la.Integration != ra.Integration {
					unified.WriteString(fmt.Sprintf("-  integration: %q\n+  integration: %q\n", ra.Integration, la.Integration))
				}
				if la.ActionType != ra.ActionType {
					unified.WriteString(fmt.Sprintf("-  action_type: %q\n+  action_type: %q\n", ra.ActionType, la.ActionType))
				}
			} else if string(la.Config) != string(ra.Config) {
				changes = append(changes, fmt.Sprintf("Step %d (%s) config modified", i+1, la.Name))
				unified.WriteString(fmt.Sprintf("@@ Step %d (%s) Config @@\n", i+1, la.Name))
				unified.WriteString(fmt.Sprintf("-  config: %s\n+  config: %s\n", string(ra.Config), string(la.Config)))
			}
		}
	}

	// Compare JSON snapshots if no top-level changes identified yet
	if len(changes) == 0 {
		localBytes, _ := json.Marshal(local)
		remoteBytes, _ := json.Marshal(remote)
		if string(localBytes) != string(remoteBytes) {
			changes = append(changes, "Metadata or optional parameters modified")
			unified.WriteString("~ Parameters updated\n")
		}
	}

	res.HasChanges = len(changes) > 0
	res.Changes = changes
	res.Unified = unified.String()
	return res
}
