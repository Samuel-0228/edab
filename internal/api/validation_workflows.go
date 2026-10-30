package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"kerob/internal/adapters/ssrf"
	"kerob/internal/core/workflow"
)

// ValidateWorkflowInput validates workflow payload boundary conditions.
func ValidateWorkflowInput(wf *workflow.Workflow, isPatch bool) []FieldError {
	var errs []FieldError

	if !isPatch || wf.Name != "" {
		name := strings.TrimSpace(wf.Name)
		if name == "" {
			errs = append(errs, FieldError{Field: "name", Message: "workflow name is required and cannot be empty"})
		} else if len(name) > 128 {
			errs = append(errs, FieldError{Field: "name", Message: "workflow name cannot exceed 128 characters"})
		}
	}

	if len(wf.Description) > 512 {
		errs = append(errs, FieldError{Field: "description", Message: "workflow description cannot exceed 512 characters"})
	}

	if wf.MaxRetries < 0 || wf.MaxRetries > 20 {
		errs = append(errs, FieldError{Field: "max_retries", Message: "max_retries must be between 0 and 20"})
	}

	if wf.TimeoutSeconds < 0 || wf.TimeoutSeconds > 3600 {
		errs = append(errs, FieldError{Field: "timeout_seconds", Message: "timeout_seconds must be between 0 and 3600"})
	}

	if !isPatch || wf.Trigger.Type != "" {
		triggerType := strings.ToLower(strings.TrimSpace(wf.Trigger.Type))
		validTriggers := map[string]bool{
			"webhook": true, "schedule": true, "telegram": true, "github": true, "rss": true, "discord": true,
		}
		if triggerType == "" {
			errs = append(errs, FieldError{Field: "trigger.type", Message: "workflow trigger.type is required"})
		} else if !validTriggers[triggerType] {
			errs = append(errs, FieldError{Field: "trigger.type", Message: fmt.Sprintf("unsupported trigger type %q; must be one of: webhook, schedule, telegram, github, rss, discord", wf.Trigger.Type)})
		}

		// Invalid combination: schedule trigger cannot specify external source_id
		if triggerType == "schedule" && strings.TrimSpace(wf.Trigger.SourceID) != "" {
			errs = append(errs, FieldError{Field: "trigger.source_id", Message: "schedule triggers cannot restrict to a source_id"})
		}

		if triggerType == "schedule" {
			var cfg map[string]interface{}
			if len(wf.Trigger.Config) > 0 {
				_ = json.Unmarshal(wf.Trigger.Config, &cfg)
			}
			cronStr, _ := cfg["cron"].(string)
			intervalStr, _ := cfg["interval"].(string)
			if strings.TrimSpace(cronStr) == "" && strings.TrimSpace(intervalStr) == "" {
				errs = append(errs, FieldError{Field: "trigger.config", Message: "schedule trigger requires 'cron' or 'interval' in config"})
			}
		}
	}

	// Validate nested conditions if present
	if wf.Conditions != nil {
		errs = append(errs, ValidateCondition(wf.Conditions, "conditions")...)
	}

	// Validate nested transformations if present
	if wf.Transformations != nil {
		errs = append(errs, ValidateTransformation(wf.Transformations)...)
	}

	if !isPatch || len(wf.Actions) > 0 {
		if len(wf.Actions) == 0 {
			errs = append(errs, FieldError{Field: "actions", Message: "workflow must define at least one action"})
		} else {
			for i, act := range wf.Actions {
				errs = append(errs, validateActionStepItem(act, fmt.Sprintf("actions[%d]", i))...)
			}
		}
	}

	return errs
}

func validateActionStepItem(act workflow.ActionStep, prefix string) []FieldError {
	var errs []FieldError

	if act.OnError != "" && act.OnError != workflow.OnErrorFail && act.OnError != workflow.OnErrorContinue && act.OnError != workflow.OnErrorSkipRest {
		errs = append(errs, FieldError{Field: fmt.Sprintf("%s.on_error", prefix), Message: fmt.Sprintf("invalid on_error %q; must be one of: %s, %s, %s", act.OnError, workflow.OnErrorFail, workflow.OnErrorContinue, workflow.OnErrorSkipRest)})
	}

	if act.MaxRetries != nil && (*act.MaxRetries < 0 || *act.MaxRetries > 20) {
		errs = append(errs, FieldError{Field: fmt.Sprintf("%s.max_retries", prefix), Message: "max_retries must be between 0 and 20"})
	}

	if act.BackoffSec < 0 || act.BackoffSec > 3600 {
		errs = append(errs, FieldError{Field: fmt.Sprintf("%s.backoff_seconds", prefix), Message: "backoff_seconds must be between 0 and 3600"})
	}

	if act.TimeoutSec < 0 || act.TimeoutSec > 3600 {
		errs = append(errs, FieldError{Field: fmt.Sprintf("%s.timeout_seconds", prefix), Message: "timeout_seconds must be between 0 and 3600"})
	}

	switch act.Type {
	case workflow.StepTypeBranch:
		if len(act.Cases) == 0 && len(act.Default) == 0 {
			errs = append(errs, FieldError{Field: fmt.Sprintf("%s.cases", prefix), Message: "branch step must define at least one case or default"})
		}
		for cIdx, c := range act.Cases {
			if c.When != nil {
				errs = append(errs, ValidateCondition(c.When, fmt.Sprintf("%s.cases[%d].when", prefix, cIdx))...)
			}
			for sIdx, sub := range c.Steps {
				errs = append(errs, validateActionStepItem(sub, fmt.Sprintf("%s.cases[%d].steps[%d]", prefix, cIdx, sIdx))...)
			}
		}
		for sIdx, sub := range act.Default {
			errs = append(errs, validateActionStepItem(sub, fmt.Sprintf("%s.default[%d]", prefix, sIdx))...)
		}

	case workflow.StepTypeForEach:
		if strings.TrimSpace(act.Items) == "" {
			errs = append(errs, FieldError{Field: fmt.Sprintf("%s.items", prefix), Message: "for_each step requires 'items' expression"})
		}
		if len(act.Steps) == 0 {
			errs = append(errs, FieldError{Field: fmt.Sprintf("%s.steps", prefix), Message: "for_each step must define at least one child step"})
		}
		for sIdx, sub := range act.Steps {
			errs = append(errs, validateActionStepItem(sub, fmt.Sprintf("%s.steps[%d]", prefix, sIdx))...)
		}

	case workflow.StepTypeParallel:
		if len(act.Steps) == 0 {
			errs = append(errs, FieldError{Field: fmt.Sprintf("%s.steps", prefix), Message: "parallel step must define at least one child step"})
		}
		for sIdx, sub := range act.Steps {
			errs = append(errs, validateActionStepItem(sub, fmt.Sprintf("%s.steps[%d]", prefix, sIdx))...)
		}

	case workflow.StepTypeDelay, workflow.StepTypeWait:
		if strings.TrimSpace(act.Duration) == "" && act.DurationSec <= 0 {
			errs = append(errs, FieldError{Field: fmt.Sprintf("%s.duration", prefix), Message: "delay step requires duration (e.g. '5s') or duration_seconds"})
		}

	default: // standard action
		validIntegrations := map[string]bool{
			"http": true, "discord": true, "github": true, "telegram": true,
		}
		integ := strings.ToLower(strings.TrimSpace(act.Integration))
		if integ == "" {
			errs = append(errs, FieldError{Field: fmt.Sprintf("%s.integration", prefix), Message: "action integration is required"})
		} else if !validIntegrations[integ] {
			errs = append(errs, FieldError{Field: fmt.Sprintf("%s.integration", prefix), Message: fmt.Sprintf("unsupported action integration %q; supported: http, discord, github, telegram", act.Integration)})
		}

		if strings.TrimSpace(act.ActionType) == "" {
			errs = append(errs, FieldError{Field: fmt.Sprintf("%s.action_type", prefix), Message: "action_type is required"})
		} else if len(act.ActionType) > 64 {
			errs = append(errs, FieldError{Field: fmt.Sprintf("%s.action_type", prefix), Message: "action_type cannot exceed 64 characters"})
		}

		if len(act.Config) == 0 || string(act.Config) == "null" {
			errs = append(errs, FieldError{Field: fmt.Sprintf("%s.config", prefix), Message: "action config must be a valid JSON object"})
		} else {
			var rawMap map[string]interface{}
			if err := json.Unmarshal(act.Config, &rawMap); err != nil {
				errs = append(errs, FieldError{Field: fmt.Sprintf("%s.config", prefix), Message: "action config is malformed JSON: " + err.Error()})
			} else if integ == "http" {
				urlVal, _ := rawMap["url"].(string)
				urlVal = strings.TrimSpace(urlVal)
				if urlVal == "" {
					errs = append(errs, FieldError{Field: fmt.Sprintf("%s.config.url", prefix), Message: "http action requires 'url'"})
				} else {
					parsed, err := url.Parse(urlVal)
					if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
						errs = append(errs, FieldError{Field: fmt.Sprintf("%s.config.url", prefix), Message: "url must be a valid http or https URL"})
					} else if err := ssrf.ValidateURL(context.Background(), urlVal); err != nil {
						errs = append(errs, FieldError{Field: fmt.Sprintf("%s.config.url", prefix), Message: "url violates SSRF safety: " + err.Error()})
					}
				}
			}
		}
	}

	return errs
}
