package api

import (
	"encoding/json"
	"fmt"
	"strings"

	"kerob/internal/adapters/destinations"
	"kerob/internal/adapters/sources"
	"kerob/internal/core/credentials"
	"kerob/internal/core/routing"
	"kerob/internal/core/webhooks"
)

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
