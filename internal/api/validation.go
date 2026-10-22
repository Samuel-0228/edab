package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"kerob/internal/adapters/destinations"
	"kerob/internal/adapters/sources"
	"kerob/internal/adapters/ssrf"
	"kerob/internal/core/credentials"
	"kerob/internal/core/routing"
	"kerob/internal/core/webhooks"
	"kerob/internal/core/workflow"
)

type contextKey string

const requestIDKey contextKey = "request_id"

func getRequestID(r *http.Request) string {
	if r == nil {
		return ""
	}
	if id, ok := r.Context().Value(requestIDKey).(string); ok && id != "" {
		return id
	}
	return r.Header.Get("X-Request-ID")
}

// FieldError represents a specific invalid parameter or payload field.
type FieldError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// APIError is the structured JSON error response returned by Kerob API endpoints.
type APIError struct {
	Error     string       `json:"error"`
	Code      string       `json:"code"`
	RequestID string       `json:"request_id,omitempty"`
	Details   []FieldError `json:"details,omitempty"`
}

func respondErrorWithCode(w http.ResponseWriter, status int, code, message string, details []FieldError) {
	resp := APIError{
		Error:   message,
		Code:    code,
		Details: details,
	}
	respondJSON(w, status, resp)
}

func respondValidationError(w http.ResponseWriter, r *http.Request, details []FieldError) {
	reqID := getRequestID(r)
	resp := APIError{
		Error:     "validation failed: one or more fields are invalid",
		Code:      "VALIDATION_ERROR",
		RequestID: reqID,
		Details:   details,
	}
	respondJSON(w, http.StatusBadRequest, resp)
}

var uuidRegex = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

func isValidUUID(u string) bool {
	return uuidRegex.MatchString(strings.TrimSpace(u))
}

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

	if !isPatch || wf.Trigger.Type != "" {
		triggerType := strings.ToLower(strings.TrimSpace(wf.Trigger.Type))
		validTriggers := map[string]bool{
			"webhook": true, "schedule": true, "telegram": true, "github": true, "rss": true,
		}
		if triggerType == "" {
			errs = append(errs, FieldError{Field: "trigger.type", Message: "workflow trigger.type is required"})
		} else if !validTriggers[triggerType] {
			errs = append(errs, FieldError{Field: "trigger.type", Message: fmt.Sprintf("unsupported trigger type %q; must be one of: webhook, schedule, telegram, github, rss", wf.Trigger.Type)})
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

// ValidateWebhookInput validates webhook registration request data.
func ValidateWebhookInput(wh *webhooks.Webhook) []FieldError {
	var errs []FieldError

	name := strings.TrimSpace(wh.Name)
	if name == "" {
		errs = append(errs, FieldError{Field: "name", Message: "webhook name is required"})
	} else if len(name) > 128 {
		errs = append(errs, FieldError{Field: "name", Message: "webhook name cannot exceed 128 characters"})
	}

	if wh.RateLimitPerMinute < 0 {
		errs = append(errs, FieldError{Field: "rate_limit_per_minute", Message: "rate_limit_per_minute cannot be negative"})
	}

	return errs
}

// ValidateCredentialInput validates credential creation/update input.
func ValidateCredentialInput(c *credentials.Credential, data map[string]string) []FieldError {
	var errs []FieldError

	name := strings.TrimSpace(c.Name)
	if name == "" {
		errs = append(errs, FieldError{Field: "name", Message: "credential name is required"})
	} else if len(name) > 128 {
		errs = append(errs, FieldError{Field: "name", Message: "credential name cannot exceed 128 characters"})
	}

	integ := strings.ToLower(strings.TrimSpace(c.Integration))
	validIntegrations := map[string]bool{
		"http": true, "discord": true, "github": true, "telegram": true, "custom": true,
	}
	if integ == "" {
		errs = append(errs, FieldError{Field: "integration", Message: "credential integration is required"})
	} else if !validIntegrations[integ] {
		errs = append(errs, FieldError{Field: "integration", Message: fmt.Sprintf("unsupported integration %q", c.Integration)})
	}

	if len(data) == 0 {
		errs = append(errs, FieldError{Field: "data", Message: "credential data map cannot be empty"})
	}

	return errs
}

// ValidateSourceInput validates external source configuration.
func ValidateSourceInput(s *sources.Source, reg *sources.Registry) []FieldError {
	var errs []FieldError

	name := strings.TrimSpace(s.Name)
	if name == "" {
		errs = append(errs, FieldError{Field: "name", Message: "source name is required"})
	} else if len(name) > 128 {
		errs = append(errs, FieldError{Field: "name", Message: "source name cannot exceed 128 characters"})
	}

	adapterType := strings.TrimSpace(s.AdapterType)
	if adapterType == "" {
		errs = append(errs, FieldError{Field: "adapter_type", Message: "adapter_type is required"})
	} else {
		registered := false
		if reg != nil {
			for _, t := range reg.RegisteredTypes() {
				if t == adapterType {
					registered = true
					break
				}
			}
		}
		validStandard := map[string]bool{"mock": true, "webhook": true, "telegram": true, "telegram_public": true}
		if !registered && !validStandard[adapterType] {
			errs = append(errs, FieldError{Field: "adapter_type", Message: fmt.Sprintf("unregistered source adapter type %q", adapterType)})
		}
	}

	if len(s.Config) > 0 && string(s.Config) != "null" {
		var js map[string]interface{}
		if err := json.Unmarshal(s.Config, &js); err != nil {
			errs = append(errs, FieldError{Field: "config", Message: "config must be a valid JSON object"})
		}
	}

	return errs
}

// ValidateDestinationInput validates external destination configuration.
func ValidateDestinationInput(d *destinations.Destination, reg *destinations.Registry) []FieldError {
	var errs []FieldError

	name := strings.TrimSpace(d.Name)
	if name == "" {
		errs = append(errs, FieldError{Field: "name", Message: "destination name is required"})
	} else if len(name) > 128 {
		errs = append(errs, FieldError{Field: "name", Message: "destination name cannot exceed 128 characters"})
	}

	adapterType := strings.TrimSpace(d.AdapterType)
	if adapterType == "" {
		errs = append(errs, FieldError{Field: "adapter_type", Message: "adapter_type is required"})
	} else {
		registered := false
		if reg != nil {
			for _, t := range reg.RegisteredTypes() {
				if t == adapterType {
					registered = true
					break
				}
			}
		}
		validStandard := map[string]bool{"mock": true, "webhook": true, "stdout": true, "telegram": true}
		if !registered && !validStandard[adapterType] {
			errs = append(errs, FieldError{Field: "adapter_type", Message: fmt.Sprintf("unregistered destination adapter type %q", adapterType)})
		}
	}

	if len(d.Config) > 0 && string(d.Config) != "null" {
		var js map[string]interface{}
		if err := json.Unmarshal(d.Config, &js); err != nil {
			errs = append(errs, FieldError{Field: "config", Message: "config must be a valid JSON object"})
		}
	}

	return errs
}

// ValidateRouteInput validates routing configuration.
func ValidateRouteInput(r *routing.Route) []FieldError {
	var errs []FieldError

	if !isValidUUID(r.SourceID) {
		errs = append(errs, FieldError{Field: "source_id", Message: "source_id must be a valid UUID"})
	}
	if !isValidUUID(r.DestinationID) {
		errs = append(errs, FieldError{Field: "destination_id", Message: "destination_id must be a valid UUID"})
	}

	if len(r.Filters) > 0 && string(r.Filters) != "null" {
		var js map[string]interface{}
		if err := json.Unmarshal(r.Filters, &js); err != nil {
			errs = append(errs, FieldError{Field: "filters", Message: "filters must be a valid JSON object"})
		}
	}

	return errs
}
