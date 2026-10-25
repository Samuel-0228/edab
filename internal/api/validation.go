package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"kerob/internal/adapters/destinations"
	"kerob/internal/adapters/sources"
	"kerob/internal/adapters/ssrf"
	"kerob/internal/core/conditions"
	"kerob/internal/core/credentials"
	"kerob/internal/core/routing"
	"kerob/internal/core/transformations"
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
	requestID := w.Header().Get("X-Request-ID")
	if status >= http.StatusInternalServerError {
		slog.Error("request failed",
			slog.Int("status", status),
			slog.String("request_id", requestID),
			slog.String("code", code),
			slog.String("error", message),
		)
		message = "internal server error"
		if code == "ERROR" {
			code = "INTERNAL_ERROR"
		}
	}
	resp := APIError{
		Error:     message,
		Code:      code,
		RequestID: requestID,
		Details:   details,
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

// ValidateCondition recursively validates a condition's structure, operator enum, and regexes.
func ValidateCondition(c *conditions.Condition, path string) []FieldError {
	if c == nil {
		return nil
	}
	var errs []FieldError

	validOperators := map[conditions.Operator]bool{
		conditions.OpEquals:         true,
		conditions.OpNotEquals:      true,
		conditions.OpContains:       true,
		conditions.OpNotContains:    true,
		conditions.OpStartsWith:     true,
		conditions.OpEndsWith:       true,
		conditions.OpMatches:        true,
		conditions.OpExists:         true,
		conditions.OpNotExists:      true,
		conditions.OpGreaterThan:    true,
		conditions.OpLessThan:       true,
		conditions.OpGreaterOrEqual: true,
		conditions.OpLessOrEqual:    true,
		conditions.OpIn:             true,
		conditions.OpNotIn:          true,
	}

	validLogicals := map[conditions.LogicalOp]bool{
		conditions.LogicalAnd: true,
		conditions.LogicalOr:  true,
		conditions.LogicalNot: true,
		"":                    true,
	}

	if !validLogicals[c.Logical] {
		errs = append(errs, FieldError{
			Field:   path + ".logical",
			Message: fmt.Sprintf("unsupported logical operator %q; must be 'and', 'or', or 'not'", c.Logical),
		})
	}

	if c.Operator != "" {
		if !validOperators[c.Operator] {
			errs = append(errs, FieldError{
				Field:   path + ".operator",
				Message: fmt.Sprintf("unsupported condition operator %q", c.Operator),
			})
		}
		if c.Operator == conditions.OpMatches {
			pattern, ok := c.Value.(string)
			if !ok || pattern == "" {
				errs = append(errs, FieldError{
					Field:   path + ".value",
					Message: "matches operator requires non-empty regex pattern string in 'value'",
				})
			} else if _, err := regexp.Compile(pattern); err != nil {
				errs = append(errs, FieldError{
					Field:   path + ".value",
					Message: fmt.Sprintf("invalid regex pattern: %v", err),
				})
			}
		}
	}

	if len(c.Field) > 128 {
		errs = append(errs, FieldError{
			Field:   path + ".field",
			Message: "field path exceeds 128 characters",
		})
	}

	for i, rule := range c.Rules {
		rulePath := fmt.Sprintf("%s.rules[%d]", path, i)
		errs = append(errs, ValidateCondition(&rule, rulePath)...)
	}

	return errs
}

// ValidateTransformation validates transformation type enum, output key, and template syntax.
func ValidateTransformation(t *transformations.Transformation) []FieldError {
	if t == nil {
		return nil
	}
	var errs []FieldError

	validTypes := map[string]bool{
		"template":    true,
		"mapping":     true,
		"json":        true,
		"passthrough": true,
		"":            true,
	}

	if !validTypes[t.Type] {
		errs = append(errs, FieldError{
			Field:   "transformations.type",
			Message: fmt.Sprintf("unsupported transformation type %q; supported: template, mapping, json, passthrough", t.Type),
		})
	}

	if len(t.OutputKey) > 64 {
		errs = append(errs, FieldError{
			Field:   "transformations.output_key",
			Message: "output_key cannot exceed 64 characters",
		})
	}

	if t.Type == "json" && strings.TrimSpace(t.Template) != "" {
		// Verify raw template isn't malformed JSON structure when literal JSON
		trimmed := strings.TrimSpace(t.Template)
		if !strings.Contains(trimmed, "{{") {
			var js map[string]interface{}
			if err := json.Unmarshal([]byte(trimmed), &js); err != nil {
				errs = append(errs, FieldError{
					Field:   "transformations.template",
					Message: "json template is not valid JSON syntax: " + err.Error(),
				})
			}
		}
	}

	return errs
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

// ValidateWebhookInput validates webhook registration request data.
func ValidateWebhookInput(wh *webhooks.Webhook) []FieldError {
	var errs []FieldError

	name := strings.TrimSpace(wh.Name)
	if name == "" {
		errs = append(errs, FieldError{Field: "name", Message: "webhook name is required"})
	} else if len(name) > 128 {
		errs = append(errs, FieldError{Field: "name", Message: "webhook name cannot exceed 128 characters"})
	}

	if wh.RateLimitPerMinute < 0 || wh.RateLimitPerMinute > 100000 {
		errs = append(errs, FieldError{Field: "rate_limit_per_minute", Message: "rate_limit_per_minute must be between 0 and 100000"})
	}

	if len(wh.SecretToken) > 128 {
		errs = append(errs, FieldError{Field: "secret_token", Message: "secret_token cannot exceed 128 characters"})
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
	} else {
		for k, v := range data {
			if strings.TrimSpace(k) == "" {
				errs = append(errs, FieldError{Field: "data", Message: "credential data keys cannot be empty"})
			}
			if strings.TrimSpace(v) == "" {
				errs = append(errs, FieldError{Field: fmt.Sprintf("data[%s]", k), Message: "credential value cannot be empty"})
			}
		}
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
		validStandard := map[string]bool{"mock": true, "webhook": true, "stdout": true, "telegram": true, "discord": true}
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
	if r.SourceID != "" && r.SourceID == r.DestinationID {
		errs = append(errs, FieldError{Field: "destination_id", Message: "source_id and destination_id cannot be identical"})
	}

	if len(r.Filters) > 0 && string(r.Filters) != "null" {
		var js map[string]interface{}
		if err := json.Unmarshal(r.Filters, &js); err != nil {
			errs = append(errs, FieldError{Field: "filters", Message: "filters must be a valid JSON object"})
		}
	}

	return errs
}

// ValidateEventInput validates event ingestion payload boundary.
func ValidateEventInput(sourceID, externalID, eventType string, payload []byte) []FieldError {
	var errs []FieldError

	if strings.TrimSpace(sourceID) == "" {
		errs = append(errs, FieldError{Field: "source_id", Message: "source_id is required"})
	} else if len(sourceID) > 128 {
		errs = append(errs, FieldError{Field: "source_id", Message: "source_id cannot exceed 128 characters"})
	}

	if strings.TrimSpace(externalID) == "" {
		errs = append(errs, FieldError{Field: "external_event_id", Message: "external_event_id is required"})
	} else if len(externalID) > 256 {
		errs = append(errs, FieldError{Field: "external_event_id", Message: "external_event_id cannot exceed 256 characters"})
	}

	if len(eventType) > 128 {
		errs = append(errs, FieldError{Field: "event_type", Message: "event_type cannot exceed 128 characters"})
	}

	if len(payload) > 0 && string(payload) != "null" {
		var js map[string]interface{}
		if err := json.Unmarshal(payload, &js); err != nil {
			errs = append(errs, FieldError{Field: "payload", Message: "payload must be a valid JSON object: " + err.Error()})
		}
	}

	return errs
}
