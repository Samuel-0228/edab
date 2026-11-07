package ai

import (
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"kerob/internal/adapters/ssrf"
	"kerob/internal/core/conditions"
	"kerob/internal/core/transformations"
	"kerob/internal/core/workflow"
)

var (
	dangerousKeywords = []string{
		"exec", "shell", "bash", "sh", "zsh", "powershell", "cmd",
		"python", "ruby", "eval", "system", "spawn", "sudo",
		"/etc/passwd", "/etc/shadow", ".ssh", "rm -rf", "curl | sh",
		"169.254.169.254", "metadata.google.internal",
	}

	dangerousActionTypes = []string{
		"execute_command", "run_script", "shell_exec", "eval",
		"system_exec", "spawn_process", "bash", "sh",
	}

	validTransformationTypes = map[string]bool{
		"template":    true,
		"mapping":     true,
		"json":        true,
		"passthrough": true,
		"pick":        true,
		"omit":        true,
		"merge":       true,
		"compute":     true,
	}

	validOperators = map[conditions.Operator]bool{
		conditions.OpEquals:         true,
		conditions.OpNotEquals:      true,
		conditions.OpContains:       true,
		conditions.OpNotContains:    true,
		conditions.OpStartsWith:     true,
		conditions.OpEndsWith:       true,
		conditions.OpMatches:        true,
		conditions.OpRegex:          true,
		conditions.OpExists:         true,
		conditions.OpNotExists:      true,
		conditions.OpGreaterThan:    true,
		conditions.OpLessThan:       true,
		conditions.OpGreaterOrEqual: true,
		conditions.OpLessOrEqual:    true,
		conditions.OpBetween:        true,
		conditions.OpIn:             true,
		conditions.OpNotIn:          true,
		conditions.OpIsEmpty:        true,
		conditions.OpIsNotEmpty:     true,
		conditions.OpTimeWindow:     true,
	}
)

// ValidateProposal performs strict, deterministic safety and structural validation
// on AI-generated content before it can be presented to users for review or approval.
func ValidateProposal(task TaskType, rawContent json.RawMessage, allowedConnectors []string) ValidationResult {
	res := ValidationResult{Valid: true}

	if len(rawContent) == 0 {
		res.Valid = false
		res.Errors = append(res.Errors, "AI content is empty")
		return res
	}

	// 1. Scan for dangerous shell commands, SSRF endpoints, and arbitrary execution strings
	rawStr := strings.ToLower(string(rawContent))
	for _, kw := range dangerousKeywords {
		if strings.Contains(rawStr, kw) {
			res.Valid = false
			res.Errors = append(res.Errors, fmt.Sprintf("prohibited dangerous keyword or command pattern detected: %q", kw))
		}
	}

	// 2. Task-specific structural and safety validation
	switch task {
	case TaskDraftWorkflow:
		validateWorkflowProposal(&res, rawContent, allowedConnectors)

	case TaskGenerateConditions:
		validateConditionsProposal(&res, rawContent)

	case TaskSuggestTransformation:
		validateTransformationProposal(&res, rawContent)

	case TaskMapFields:
		validateFieldMappingProposal(&res, rawContent)

	case TaskExplainFailure:
		validateExplanationProposal(&res, rawContent)

	default:
		res.Valid = false
		res.Errors = append(res.Errors, fmt.Sprintf("unsupported AI task type: %s", task))
	}

	return res
}

func validateWorkflowProposal(res *ValidationResult, raw json.RawMessage, allowedConnectors []string) {
	var wf workflow.Workflow
	if err := json.Unmarshal(raw, &wf); err != nil {
		res.Valid = false
		res.Errors = append(res.Errors, fmt.Sprintf("invalid workflow JSON structure: %v", err))
		return
	}

	if strings.TrimSpace(wf.Name) == "" {
		res.Valid = false
		res.Errors = append(res.Errors, "workflow name is required")
	}

	if strings.TrimSpace(wf.Trigger.Type) == "" {
		res.Valid = false
		res.Errors = append(res.Errors, "workflow trigger.type is required")
	}

	if len(wf.Actions) == 0 {
		res.Valid = false
		res.Errors = append(res.Errors, "workflow must define at least one action step")
	}

	allowedSet := make(map[string]bool)
	for _, c := range allowedConnectors {
		allowedSet[strings.ToLower(strings.TrimSpace(c))] = true
	}

	for i, act := range wf.Actions {
		stepIdx := fmt.Sprintf("actions[%d]", i)

		// Check for prohibited action execution types
		lowerActionType := strings.ToLower(act.ActionType)
		for _, dangerous := range dangerousActionTypes {
			if lowerActionType == dangerous {
				res.Valid = false
				res.Errors = append(res.Errors, fmt.Sprintf("%s specifies prohibited dangerous action type %q", stepIdx, act.ActionType))
			}
		}

		// Connector whitelist check if specified
		if len(allowedSet) > 0 && act.Integration != "" {
			if !allowedSet[strings.ToLower(act.Integration)] && !allowedSet["*"] {
				res.Valid = false
				res.Errors = append(res.Errors, fmt.Sprintf("%s uses unapproved connector %q", stepIdx, act.Integration))
			}
		}

		// URL SSRF check on configs
		if len(act.Config) > 0 {
			var cfg map[string]any
			if err := json.Unmarshal(act.Config, &cfg); err == nil {
				for _, urlKey := range []string{"url", "endpoint", "webhook_url"} {
					if val, ok := cfg[urlKey]; ok {
						if uStr, ok := val.(string); ok && uStr != "" {
							if err := validateURLSafe(uStr); err != nil {
								res.Valid = false
								res.Errors = append(res.Errors, fmt.Sprintf("%s configuration contains unsafe URL: %v", stepIdx, err))
							}
						}
					}
				}
			}
		}
	}
}

func validateConditionsProposal(res *ValidationResult, raw json.RawMessage) {
	var cond conditions.Condition
	if err := json.Unmarshal(raw, &cond); err != nil {
		res.Valid = false
		res.Errors = append(res.Errors, fmt.Sprintf("invalid condition JSON: %v", err))
		return
	}

	validateConditionNode(res, &cond, "root")
}

func validateConditionNode(res *ValidationResult, c *conditions.Condition, path string) {
	if c == nil {
		return
	}

	if len(c.Rules) > 0 {
		if c.Logical != conditions.LogicalAnd && c.Logical != conditions.LogicalOr && c.Logical != conditions.LogicalNot {
			res.Valid = false
			res.Errors = append(res.Errors, fmt.Sprintf("%s: logical operator must be 'and', 'or', or 'not', got %q", path, c.Logical))
		}
		for i := range c.Rules {
			validateConditionNode(res, &c.Rules[i], fmt.Sprintf("%s.rules[%d]", path, i))
		}
		return
	}

	if c.Field == "" {
		res.Valid = false
		res.Errors = append(res.Errors, fmt.Sprintf("%s: condition field is required", path))
	}

	if !validOperators[c.Operator] {
		res.Valid = false
		res.Errors = append(res.Errors, fmt.Sprintf("%s: unrecognized condition operator %q", path, c.Operator))
	}
}

func validateTransformationProposal(res *ValidationResult, raw json.RawMessage) {
	var t transformations.Transformation
	if err := json.Unmarshal(raw, &t); err != nil {
		res.Valid = false
		res.Errors = append(res.Errors, fmt.Sprintf("invalid transformation JSON: %v", err))
		return
	}

	if t.Type == "" {
		res.Valid = false
		res.Errors = append(res.Errors, "transformation type is required")
		return
	}

	if !validTransformationTypes[t.Type] {
		res.Valid = false
		res.Errors = append(res.Errors, fmt.Sprintf("unrecognized transformation type %q", t.Type))
	}
}

func validateFieldMappingProposal(res *ValidationResult, raw json.RawMessage) {
	var mapping map[string]string
	if err := json.Unmarshal(raw, &mapping); err != nil {
		res.Valid = false
		res.Errors = append(res.Errors, fmt.Sprintf("invalid field mapping JSON: %v", err))
		return
	}

	if len(mapping) == 0 {
		res.Valid = false
		res.Errors = append(res.Errors, "field mapping must contain at least one key-value mapping")
		return
	}

	validKeyRegex := regexp.MustCompile(`^[a-zA-Z0-9_.-]+$`)
	for k, v := range mapping {
		if !validKeyRegex.MatchString(k) {
			res.Valid = false
			res.Errors = append(res.Errors, fmt.Sprintf("invalid mapping target key identifier: %q", k))
		}
		if strings.TrimSpace(v) == "" {
			res.Warnings = append(res.Warnings, fmt.Sprintf("target key %q has empty mapping expression", k))
		}
	}
}

func validateExplanationProposal(res *ValidationResult, raw json.RawMessage) {
	var exp struct {
		Summary     string   `json:"summary"`
		RootCause   string   `json:"root_cause"`
		NextSteps   []string `json:"next_steps"`
		Remediation string   `json:"remediation,omitempty"`
	}
	if err := json.Unmarshal(raw, &exp); err != nil {
		res.Valid = false
		res.Errors = append(res.Errors, fmt.Sprintf("invalid failure explanation JSON: %v", err))
		return
	}

	if strings.TrimSpace(exp.Summary) == "" && strings.TrimSpace(exp.RootCause) == "" {
		res.Valid = false
		res.Errors = append(res.Errors, "explanation must include summary or root cause")
	}
}

func validateURLSafe(rawURL string) error {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return fmt.Errorf("malformed URL: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("URL scheme must be http or https")
	}
	host := strings.ToLower(parsed.Hostname())
	if host == "localhost" || strings.HasSuffix(host, ".localhost") || host == "127.0.0.1" || host == "169.254.169.254" || host == "metadata.google.internal" {
		return ssrf.ErrBlockedHost
	}
	return nil
}
