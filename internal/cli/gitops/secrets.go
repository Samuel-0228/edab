package gitops

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"kerob/internal/core/workflow"
)

// SecretViolation documents a detected plaintext credential in a workflow file.
type SecretViolation struct {
	StepID string `json:"step_id,omitempty"`
	Field  string `json:"field"`
	Reason string `json:"reason"`
}

func (v SecretViolation) String() string {
	if v.StepID != "" {
		return fmt.Sprintf("step '%s' (field '%s'): %s", v.StepID, v.Field, v.Reason)
	}
	return fmt.Sprintf("field '%s': %s", v.Field, v.Reason)
}

var sensitivePatterns = []struct {
	name    string
	pattern *regexp.Regexp
}{
	{"AWS Access Key", regexp.MustCompile(`\bAKIA[0-9A-Z]{16}\b`)},
	{"GitHub Personal Access Token", regexp.MustCompile(`\bghp_[0-9a-zA-Z]{36}\b`)},
	{"GitHub OAuth Token", regexp.MustCompile(`\bgho_[0-9a-zA-Z]{36}\b`)},
	{"Private Key PEM", regexp.MustCompile(`-----BEGIN [A-Z ]*PRIVATE KEY-----`)},
	{"JWT Bearer Token", regexp.MustCompile(`\beyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\b`)},
}

var suspiciousKeys = map[string]bool{
	"password":      true,
	"secret":        true,
	"api_key":       true,
	"apikey":        true,
	"private_key":   true,
	"auth_token":    true,
	"access_token":  true,
	"client_secret": true,
}

// ValidateWorkflowSecrets inspects a workflow definition for accidental hardcoded credentials.
// In Kerob, secrets must be referenced by name (via credential_id or {{ secret.NAME }} templates).
func ValidateWorkflowSecrets(wf *workflow.Workflow) []SecretViolation {
	var violations []SecretViolation

	var inspectValue func(val any, path, stepID string)
	inspectValue = func(val any, path, stepID string) {
		switch v := val.(type) {
		case string:
			// Allow template interpolation syntax: {{ secret.NAME }} or {{ credentials.NAME }}
			if strings.Contains(v, "{{") && strings.Contains(v, "}}") {
				return
			}
			// Check specific patterns
			for _, sp := range sensitivePatterns {
				if sp.pattern.MatchString(v) {
					violations = append(violations, SecretViolation{
						StepID: stepID,
						Field:  path,
						Reason: fmt.Sprintf("detected plaintext %s. Secrets must be referenced by name via credential_id or {{ secret.NAME }}", sp.name),
					})
					return
				}
			}
			// Check suspicious field name
			parts := strings.Split(path, ".")
			lastKey := strings.ToLower(parts[len(parts)-1])
			if suspiciousKeys[lastKey] && len(v) > 6 && !strings.HasPrefix(v, "ref:") && !strings.HasPrefix(v, "env:") {
				violations = append(violations, SecretViolation{
					StepID: stepID,
					Field:  path,
					Reason: fmt.Sprintf("field '%s' contains potential plaintext secret value. Use credential_id or {{ secret.NAME }} instead", lastKey),
				})
			}
		case map[string]any:
			for k, innerVal := range v {
				subPath := k
				if path != "" {
					subPath = path + "." + k
				}
				inspectValue(innerVal, subPath, stepID)
			}
		case []any:
			for i, item := range v {
				subPath := fmt.Sprintf("%s[%d]", path, i)
				inspectValue(item, subPath, stepID)
			}
		}
	}

	// 1. Inspect trigger config
	if len(wf.Trigger.Config) > 0 {
		var trigMap map[string]any
		if err := json.Unmarshal(wf.Trigger.Config, &trigMap); err == nil {
			inspectValue(trigMap, "trigger.config", "")
		}
	}

	// 2. Inspect actions recursively
	var inspectAction func(act *workflow.ActionStep)
	inspectAction = func(act *workflow.ActionStep) {
		if act == nil {
			return
		}
		stepID := act.ID
		if stepID == "" {
			stepID = act.Name
		}
		if len(act.Config) > 0 {
			var actMap map[string]any
			if err := json.Unmarshal(act.Config, &actMap); err == nil {
				inspectValue(actMap, fmt.Sprintf("action.%s.config", stepID), stepID)
			}
		}
		if act.Compensation != nil {
			inspectAction(act.Compensation)
		}
	}

	for i := range wf.Actions {
		inspectAction(&wf.Actions[i])
	}

	return violations
}
