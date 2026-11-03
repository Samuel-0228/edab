package gitops

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"kerob/internal/core/workflow"
	"kerob/pkg/sdk"
)

// DeployOptions specifies parameters for GitOps workflow deployment.
type DeployOptions struct {
	DryRun            bool
	RequireApproval   bool
	Approved          bool
	RollbackOnFailure bool
	ApprovalReason    string
}

// WorkflowDeployResult records the outcome of deploying a single workflow.
type WorkflowDeployResult struct {
	FilePath   string             `json:"file_path"`
	WorkflowID string             `json:"workflow_id"`
	Name       string             `json:"name"`
	Action     string             `json:"action"` // "created", "updated", "unchanged", "skipped"
	Diff       WorkflowDiffResult `json:"diff"`
	Error      string             `json:"error,omitempty"`
}

// DeployReport summarizes a GitOps deployment run across one or more workflow specifications.
type DeployReport struct {
	Success   bool                   `json:"success"`
	DryRun    bool                   `json:"dry_run"`
	Workflows []WorkflowDeployResult `json:"workflows"`
	Summary   string                 `json:"summary"`
}

// DeployPath executes a GitOps deployment targeting a specific file or a directory of workflow definitions.
func DeployPath(ctx context.Context, client *sdk.Client, targetPath string, opts DeployOptions) (*DeployReport, error) {
	var files []string

	info, err := os.Stat(targetPath)
	if err != nil {
		return nil, fmt.Errorf("failed to access deployment path %q: %w", targetPath, err)
	}

	if info.IsDir() {
		err := filepath.WalkDir(targetPath, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}
			ext := strings.ToLower(filepath.Ext(path))
			if ext == ".yaml" || ext == ".yml" || ext == ".json" {
				files = append(files, path)
			}
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("failed to scan workflow directory %q: %w", targetPath, err)
		}
	} else {
		files = append(files, targetPath)
	}

	if len(files) == 0 {
		return &DeployReport{
			Success: true,
			DryRun:  opts.DryRun,
			Summary: fmt.Sprintf("No workflow files found in %s", targetPath),
		}, nil
	}

	// Fetch existing remote workflows to match by ID or Name
	remoteWorkflows, err := client.ListWorkflows(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to query remote workflows: %w", err)
	}
	remoteByID := make(map[string]*sdk.Workflow)
	remoteByName := make(map[string]*sdk.Workflow)
	for i := range remoteWorkflows {
		remoteByID[remoteWorkflows[i].ID] = &remoteWorkflows[i]
		remoteByName[remoteWorkflows[i].Name] = &remoteWorkflows[i]
	}

	var results []WorkflowDeployResult
	createdCount := 0
	updatedCount := 0
	unchangedCount := 0
	errorCount := 0

	// Track applied modifications for rollback on failure
	var history []appliedChange

	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			errorCount++
			results = append(results, WorkflowDeployResult{
				FilePath: file,
				Action:   "skipped",
				Error:    fmt.Sprintf("read error: %s", err.Error()),
			})
			continue
		}

		wf, err := ParseWorkflowFile(data)
		if err != nil {
			errorCount++
			results = append(results, WorkflowDeployResult{
				FilePath: file,
				Action:   "skipped",
				Error:    fmt.Sprintf("parse error: %s", err.Error()),
			})
			continue
		}

		// Validation & Secret Safety Scan
		valErrors := ValidateWorkflow(wf)
		var fatalMsgs []string
		for _, ve := range valErrors {
			if ve.Fatal {
				fatalMsgs = append(fatalMsgs, ve.Error())
			}
		}
		if len(fatalMsgs) > 0 {
			errorCount++
			results = append(results, WorkflowDeployResult{
				FilePath: file,
				Name:     wf.Name,
				Action:   "skipped",
				Error:    fmt.Sprintf("validation failed: %s", strings.Join(fatalMsgs, "; ")),
			})
			continue
		}

		// Find remote match
		var remote *sdk.Workflow
		if wf.ID != "" && remoteByID[wf.ID] != nil {
			remote = remoteByID[wf.ID]
		} else if remoteByName[wf.Name] != nil {
			remote = remoteByName[wf.Name]
		}

		// Convert remote SDK model to workflow.Workflow for diffing
		var remoteWf *workflow.Workflow
		if remote != nil {
			remBytes, _ := os.ReadFile(file) // temporary buffer
			remBytes, _ = os.ReadFile(file)
			_ = remBytes
			remoteWf = &workflow.Workflow{
				ID:          remote.ID,
				Name:        remote.Name,
				Description: remote.Description,
				Enabled:     remote.Enabled,
				Trigger: workflow.TriggerConfig{
					Type:       remote.Trigger.Type,
					SourceID:   remote.Trigger.SourceID,
					EventTypes: remote.Trigger.EventTypes,
					Config:     remote.Trigger.Config,
				},
				Actions: make([]workflow.ActionStep, len(remote.Actions)),
			}
			for idx, a := range remote.Actions {
				remoteWf.Actions[idx] = workflow.ActionStep{
					ID:           a.ID,
					Name:         a.Name,
					Type:         a.Type,
					Integration:  a.Integration,
					ActionType:   a.ActionType,
					CredentialID: a.CredentialID,
					Config:       a.Config,
					TimeoutSec:   a.TimeoutSeconds,
				}
			}
		}

		diff := DiffWorkflow(wf, remoteWf)

		if !diff.HasChanges {
			unchangedCount++
			results = append(results, WorkflowDeployResult{
				FilePath:   file,
				WorkflowID: remote.ID,
				Name:       wf.Name,
				Action:     "unchanged",
				Diff:       diff,
			})
			continue
		}

		if opts.DryRun {
			act := "updated"
			if remote == nil {
				act = "created"
			}
			results = append(results, WorkflowDeployResult{
				FilePath:   file,
				WorkflowID: diff.WorkflowID,
				Name:       wf.Name,
				Action:     act,
				Diff:       diff,
			})
			continue
		}

		if opts.RequireApproval && !opts.Approved {
			errorCount++
			results = append(results, WorkflowDeployResult{
				FilePath:   file,
				WorkflowID: diff.WorkflowID,
				Name:       wf.Name,
				Action:     "skipped",
				Diff:       diff,
				Error:      "requires explicit deployment approval (--approve)",
			})
			continue
		}

		// Perform API deployment mutation
		// Map workflow.Workflow to sdk.WorkflowCreateInput
		inputActions := make([]sdk.ActionStep, len(wf.Actions))
		for idx, a := range wf.Actions {
			inputActions[idx] = sdk.ActionStep{
				ID:             a.ID,
				Name:           a.Name,
				Type:           a.Type,
				Integration:    a.Integration,
				ActionType:     a.ActionType,
				CredentialID:   a.CredentialID,
				Config:         a.Config,
				TimeoutSeconds: a.TimeoutSec,
			}
		}

		input := sdk.WorkflowCreateInput{
			Name:        wf.Name,
			Description: wf.Description,
			Enabled:     &wf.Enabled,
			Trigger: sdk.TriggerConfig{
				Type:       wf.Trigger.Type,
				SourceID:   wf.Trigger.SourceID,
				EventTypes: wf.Trigger.EventTypes,
				Config:     wf.Trigger.Config,
			},
			Actions: inputActions,
		}

		if remote != nil {
			// Update existing
			updated, err := client.UpdateWorkflow(ctx, remote.ID, input)
			if err != nil {
				errorCount++
				results = append(results, WorkflowDeployResult{
					FilePath:   file,
					WorkflowID: remote.ID,
					Name:       wf.Name,
					Action:     "skipped",
					Diff:       diff,
					Error:      err.Error(),
				})
				if opts.RollbackOnFailure {
					rollbackHistory(ctx, client, history)
					return &DeployReport{
						Success:   false,
						DryRun:    false,
						Workflows: results,
						Summary:   fmt.Sprintf("Deployment failed at %s. Rollback triggered: %s", file, err.Error()),
					}, nil
				}
				continue
			}
			updatedCount++
			history = append(history, appliedChange{id: remote.ID, wasCreated: false})
			results = append(results, WorkflowDeployResult{
				FilePath:   file,
				WorkflowID: updated.ID,
				Name:       updated.Name,
				Action:     "updated",
				Diff:       diff,
			})
		} else {
			// Create new
			created, err := client.CreateWorkflow(ctx, input)
			if err != nil {
				errorCount++
				results = append(results, WorkflowDeployResult{
					FilePath: file,
					Name:     wf.Name,
					Action:   "skipped",
					Diff:     diff,
					Error:    err.Error(),
				})
				if opts.RollbackOnFailure {
					rollbackHistory(ctx, client, history)
					return &DeployReport{
						Success:   false,
						DryRun:    false,
						Workflows: results,
						Summary:   fmt.Sprintf("Deployment failed at %s. Rollback triggered: %s", file, err.Error()),
					}, nil
				}
				continue
			}
			createdCount++
			history = append(history, appliedChange{id: created.ID, wasCreated: true})
			results = append(results, WorkflowDeployResult{
				FilePath:   file,
				WorkflowID: created.ID,
				Name:       created.Name,
				Action:     "created",
				Diff:       diff,
			})
		}
	}

	summary := fmt.Sprintf("Processed %d workflow(s): %d created, %d updated, %d unchanged, %d error(s).",
		len(files), createdCount, updatedCount, unchangedCount, errorCount)

	return &DeployReport{
		Success:   errorCount == 0,
		DryRun:    opts.DryRun,
		Workflows: results,
		Summary:   summary,
	}, nil
}

type appliedChange struct {
	id          string
	wasCreated  bool
	previousVer int
}

func rollbackHistory(ctx context.Context, client *sdk.Client, history []appliedChange) {
	for i := len(history) - 1; i >= 0; i-- {
		item := history[i]
		if item.wasCreated {
			_ = client.DeleteWorkflow(ctx, item.id)
		} else if item.previousVer > 0 {
			_, _ = client.RollbackWorkflow(ctx, item.id, item.previousVer)
		}
	}
}
