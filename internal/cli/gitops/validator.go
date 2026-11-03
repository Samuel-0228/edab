package gitops

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"kerob/internal/core/workflow"
)

// ValidationError represents an issue found during workflow linting/validation.
type ValidationError struct {
	StepID  string `json:"step_id,omitempty"`
	Message string `json:"message"`
	Fatal   bool   `json:"fatal"`
}

func (e ValidationError) Error() string {
	if e.StepID != "" {
		return fmt.Sprintf("step '%s': %s", e.StepID, e.Message)
	}
	return e.Message
}

var stepRefPattern = regexp.MustCompile(`steps\.([a-zA-Z0-9_\-]+)\.output`)

// ValidateWorkflow performs comprehensive static validation on a workflow definition.
func ValidateWorkflow(wf *workflow.Workflow) []ValidationError {
	var errs []ValidationError

	// 1. Basic required fields
	if strings.TrimSpace(wf.Name) == "" {
		errs = append(errs, ValidationError{Message: "workflow 'name' is required", Fatal: true})
	}
	if strings.TrimSpace(wf.Trigger.Type) == "" {
		errs = append(errs, ValidationError{Message: "trigger 'type' is required", Fatal: true})
	}
	if len(wf.Actions) == 0 {
		errs = append(errs, ValidationError{Message: "workflow must contain at least one action step", Fatal: true})
	}

	// 2. Secret safety validation
	secretViolations := ValidateWorkflowSecrets(wf)
	for _, sv := range secretViolations {
		errs = append(errs, ValidationError{
			StepID:  sv.StepID,
			Message: fmt.Sprintf("security violation: %s", sv.Reason),
			Fatal:   true,
		})
	}

	// 3. Step ID uniqueness, cycle detection, and reference order
	seenSteps := make(map[string]bool)
	stepOrder := make([]string, 0, len(wf.Actions))

	var checkStep func(act *workflow.ActionStep, parentChain []string)
	checkStep = func(act *workflow.ActionStep, parentChain []string) {
		if act == nil {
			return
		}
		stepID := act.ID
		if stepID == "" {
			stepID = act.Name
		}

		if stepID != "" {
			// Check for cycle in parent chain
			for _, p := range parentChain {
				if p == stepID {
					errs = append(errs, ValidationError{
						StepID:  stepID,
						Message: fmt.Sprintf("cyclic dependency detected: step '%s' is nested within itself", stepID),
						Fatal:   true,
					})
					return
				}
			}

			// Check ID uniqueness
			if seenSteps[stepID] {
				errs = append(errs, ValidationError{
					StepID:  stepID,
					Message: fmt.Sprintf("duplicate step ID '%s'", stepID),
					Fatal:   true,
				})
			}
			seenSteps[stepID] = true
			stepOrder = append(stepOrder, stepID)
		}

		// Check template reference validity (must refer to prior steps)
		if len(act.Config) > 0 {
			configStr := string(act.Config)
			matches := stepRefPattern.FindAllStringSubmatch(configStr, -1)
			for _, match := range matches {
				refStepID := match[1]
				if refStepID == stepID {
					errs = append(errs, ValidationError{
						StepID:  stepID,
						Message: fmt.Sprintf("step references its own output '%s'", refStepID),
						Fatal:   true,
					})
				} else if !seenSteps[refStepID] {
					errs = append(errs, ValidationError{
						StepID:  stepID,
						Message: fmt.Sprintf("step references future or non-existent step '%s'", refStepID),
						Fatal:   false,
					})
				}
			}
		}

		// Check compensation step
		if act.Compensation != nil {
			checkStep(act.Compensation, append(parentChain, stepID))
		}
	}

	for i := range wf.Actions {
		checkStep(&wf.Actions[i], nil)
	}

	// 4. Validate schema compliance with internal workflow validator if present
	if err := wf.Validate(); err != nil {
		errs = append(errs, ValidationError{
			Message: fmt.Sprintf("core schema validation: %s", err.Error()),
			Fatal:   true,
		})
	}

	return errs
}

// ParseWorkflowFile reads and parses a workflow definition from raw bytes (supporting both YAML and JSON).
func ParseWorkflowFile(data []byte) (*workflow.Workflow, error) {
	// 1. Try JSON unmarshal
	var wf workflow.Workflow
	if err := json.Unmarshal(data, &wf); err == nil && wf.Name != "" {
		return &wf, nil
	}

	// 2. Try YAML import helper
	imported, err := workflow.ImportWorkflowYAML(data)
	if err == nil && imported.Name != "" {
		return imported, nil
	}

	return nil, fmt.Errorf("file is neither valid workflow JSON nor YAML: %w", err)
}
