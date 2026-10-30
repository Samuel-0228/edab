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
			validIntegrations := map[string]bool{
				"http": true, "discord": true, "github": true, "telegram": true,
			}
			for i, act := range wf.Actions {
				integ := strings.ToLower(strings.TrimSpace(act.Integration))
				if integ == "" {
					errs = append(errs, FieldError{Field: fmt.Sprintf("actions[%d].integration", i), Message: "action integration is required"})
				} else if !validIntegrations[integ] {
					errs = append(errs, FieldError{Field: fmt.Sprintf("actions[%d].integration", i), Message: fmt.Sprintf("unsupported action integration %q; supported: http, discord, github, telegram", act.Integration)})
				}

				if strings.TrimSpace(act.ActionType) == "" {
					errs = append(errs, FieldError{Field: fmt.Sprintf("actions[%d].action_type", i), Message: "action_type is required"})
				} else if len(act.ActionType) > 64 {
					errs = append(errs, FieldError{Field: fmt.Sprintf("actions[%d].action_type", i), Message: "action_type cannot exceed 64 characters"})
				}

				if act.TimeoutSec < 0 || act.TimeoutSec > 3600 {
					errs = append(errs, FieldError{Field: fmt.Sprintf("actions[%d].timeout_seconds", i), Message: "timeout_seconds must be between 0 and 3600"})
				}

				if len(act.Config) == 0 || string(act.Config) == "null" {
					errs = append(errs, FieldError{Field: fmt.Sprintf("actions[%d].config", i), Message: "action config must be a valid JSON object"})
				} else {
					var rawMap map[string]interface{}
					if err := json.Unmarshal(act.Config, &rawMap); err != nil {
						errs = append(errs, FieldError{Field: fmt.Sprintf("actions[%d].config", i), Message: "action config is malformed JSON: " + err.Error()})
					} else if integ == "http" {
						urlVal, _ := rawMap["url"].(string)
						urlVal = strings.TrimSpace(urlVal)
						if urlVal == "" {
							errs = append(errs, FieldError{Field: fmt.Sprintf("actions[%d].config.url", i), Message: "http action requires 'url'"})
						} else {
							parsed, err := url.Parse(urlVal)
							if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
								errs = append(errs, FieldError{Field: fmt.Sprintf("actions[%d].config.url", i), Message: "url must be a valid http or https URL"})
							} else if err := ssrf.ValidateURL(context.Background(), urlVal); err != nil {
								errs = append(errs, FieldError{Field: fmt.Sprintf("actions[%d].config.url", i), Message: "url violates SSRF safety: " + err.Error()})
							}
						}
					}
				}
			}
		}
	}

	return errs
}
