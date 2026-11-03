package governance

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"kerob/internal/core/workflow"
)

// GovernanceEngine enforces enterprise policies across publish-time workflows and runtime actions.
type GovernanceEngine struct {
	store Store
}

// NewGovernanceEngine creates a new governance engine.
func NewGovernanceEngine(store Store) *GovernanceEngine {
	if store == nil {
		store = NewMemoryStore()
	}
	return &GovernanceEngine{store: store}
}

// Store returns the underlying policy store.
func (ge *GovernanceEngine) Store() Store {
	return ge.store
}

// GetPolicy retrieves the policy for the specified tenant.
func (ge *GovernanceEngine) GetPolicy(ctx context.Context, tenantID string) (*Policy, error) {
	if tenantID == "" {
		tenantID = "default"
	}
	return ge.store.GetPolicy(ctx, tenantID)
}

// ValidateWorkflowPublish checks a workflow against the tenant's governance policies before creation or update.
func (ge *GovernanceEngine) ValidateWorkflowPublish(ctx context.Context, tenantID string, wf *workflow.Workflow) []PolicyViolation {
	if wf == nil {
		return nil
	}
	if tenantID == "" {
		tenantID = wf.TenantID
	}
	if tenantID == "" {
		tenantID = "default"
	}

	policy, err := ge.store.GetPolicy(ctx, tenantID)
	if err != nil || policy == nil {
		policy = DefaultPolicy(tenantID)
	}

	var violations []PolicyViolation

	// 1. Max retries rule
	if policy.MaxWorkflowRetries >= 0 && wf.MaxRetries > policy.MaxWorkflowRetries {
		violations = append(violations, PolicyViolation{
			Rule:    "max_retries",
			Field:   "max_retries",
			Message: fmt.Sprintf("workflow max_retries %d exceeds tenant policy limit of %d", wf.MaxRetries, policy.MaxWorkflowRetries),
			Details: map[string]any{"max_retries": wf.MaxRetries, "policy_limit": policy.MaxWorkflowRetries},
		})
	}

	// 2. Blocked keywords in workflow metadata
	for _, kw := range policy.BlockedKeywords {
		trimmedKw := strings.TrimSpace(kw)
		if trimmedKw == "" {
			continue
		}
		lowerKw := strings.ToLower(trimmedKw)
		if strings.Contains(strings.ToLower(wf.Name), lowerKw) {
			violations = append(violations, PolicyViolation{
				Rule:    "blocked_keywords",
				Field:   "name",
				Message: fmt.Sprintf("workflow name contains blocked keyword '%s'", kw),
				Details: map[string]any{"keyword": kw},
			})
		}
		if strings.Contains(strings.ToLower(wf.Description), lowerKw) {
			violations = append(violations, PolicyViolation{
				Rule:    "blocked_keywords",
				Field:   "description",
				Message: fmt.Sprintf("workflow description contains blocked keyword '%s'", kw),
				Details: map[string]any{"keyword": kw},
			})
		}
	}

	// 3. Inspect actions
	hasPriorApproval := false
	for i, act := range wf.Actions {
		fieldPrefix := fmt.Sprintf("actions[%d]", i)

		// Per-step retry limit
		if act.MaxRetries != nil && policy.MaxWorkflowRetries >= 0 && *act.MaxRetries > policy.MaxWorkflowRetries {
			violations = append(violations, PolicyViolation{
				Rule:    "max_retries",
				Field:   fieldPrefix + ".max_retries",
				Message: fmt.Sprintf("step max_retries %d exceeds tenant policy limit of %d", *act.MaxRetries, policy.MaxWorkflowRetries),
				Details: map[string]any{"max_retries": *act.MaxRetries, "policy_limit": policy.MaxWorkflowRetries},
			})
		}

		// Allowed connectors
		if len(policy.AllowedConnectors) > 0 && act.Integration != "" {
			if !isConnectorAllowed(act.Integration, policy.AllowedConnectors) {
				violations = append(violations, PolicyViolation{
					Rule:    "allowed_connectors",
					Field:   fieldPrefix + ".integration",
					Message: fmt.Sprintf("connector '%s' is not in tenant allowed list", act.Integration),
					Details: map[string]any{"integration": act.Integration, "allowed": policy.AllowedConnectors},
				})
			}
		}

		// Outbound domains (static URL inspect)
		if len(policy.AllowedOutboundDomains) > 0 {
			targetURL := extractURLFromConfig(act.Config)
			if targetURL != "" {
				parsed, err := url.Parse(targetURL)
				if err == nil && parsed.Hostname() != "" {
					if !matchDomain(parsed.Hostname(), policy.AllowedOutboundDomains) {
						violations = append(violations, PolicyViolation{
							Rule:    "outbound_domains",
							Field:   fieldPrefix + ".config.url",
							Message: fmt.Sprintf("outbound domain '%s' is not in tenant allowed outbound domains", parsed.Hostname()),
							Details: map[string]any{"domain": parsed.Hostname(), "allowed": policy.AllowedOutboundDomains},
						})
					}
				}
			}
		}

		// Irreversible actions require approval
		if policy.RequireApprovalForIrreversibleActions && isIrreversibleAction(act) {
			if act.Approval == nil && !hasPriorApproval {
				violations = append(violations, PolicyViolation{
					Rule:    "approval_required",
					Field:   fieldPrefix,
					Message: fmt.Sprintf("irreversible action step '%s' requires human approval", act.ID),
					Details: map[string]any{"action_id": act.ID, "action_type": act.ActionType, "integration": act.Integration},
				})
			}
		}

		if act.Type == workflow.StepTypeApproval {
			hasPriorApproval = true
		}

		// Blocked keywords in step name or config
		for _, kw := range policy.BlockedKeywords {
			trimmedKw := strings.TrimSpace(kw)
			if trimmedKw == "" {
				continue
			}
			lowerKw := strings.ToLower(trimmedKw)
			if strings.Contains(strings.ToLower(act.Name), lowerKw) {
				violations = append(violations, PolicyViolation{
					Rule:    "blocked_keywords",
					Field:   fieldPrefix + ".name",
					Message: fmt.Sprintf("step name contains blocked keyword '%s'", kw),
					Details: map[string]any{"keyword": kw},
				})
			}
			if len(act.Config) > 0 && strings.Contains(strings.ToLower(string(act.Config)), lowerKw) {
				violations = append(violations, PolicyViolation{
					Rule:    "blocked_keywords",
					Field:   fieldPrefix + ".config",
					Message: fmt.Sprintf("step configuration contains blocked keyword '%s'", kw),
					Details: map[string]any{"keyword": kw},
				})
			}
		}
	}

	return violations
}

// EnforceActionRuntime performs pre-execution validation of an action step against the tenant policy.
func (ge *GovernanceEngine) EnforceActionRuntime(
	ctx context.Context,
	tenantID string,
	act workflow.ActionStep,
	payload map[string]any,
	renderedConfig json.RawMessage,
) error {
	if tenantID == "" {
		tenantID = "default"
	}

	policy, err := ge.store.GetPolicy(ctx, tenantID)
	if err != nil || policy == nil {
		policy = DefaultPolicy(tenantID)
	}

	var violations []PolicyViolation

	// 1. Connector check
	if len(policy.AllowedConnectors) > 0 && act.Integration != "" {
		if !isConnectorAllowed(act.Integration, policy.AllowedConnectors) {
			violations = append(violations, PolicyViolation{
				Rule:    "allowed_connectors",
				Field:   "integration",
				Message: fmt.Sprintf("connector '%s' is prohibited by tenant policy", act.Integration),
				Details: map[string]any{"integration": act.Integration, "allowed": policy.AllowedConnectors},
			})
		}
	}

	// 2. Outbound domain check on rendered config
	if len(policy.AllowedOutboundDomains) > 0 {
		targetURL := extractURLFromConfig(renderedConfig)
		if targetURL != "" {
			parsed, err := url.Parse(targetURL)
			if err == nil && parsed.Hostname() != "" {
				if !matchDomain(parsed.Hostname(), policy.AllowedOutboundDomains) {
					violations = append(violations, PolicyViolation{
						Rule:    "outbound_domains",
						Field:   "url",
						Message: fmt.Sprintf("destination host '%s' is not in tenant allowed outbound domains", parsed.Hostname()),
						Details: map[string]any{"domain": parsed.Hostname(), "allowed": policy.AllowedOutboundDomains},
					})
				}
			}
		}
	}

	// 3. Payload size check
	if policy.MaxPayloadSizeBytes > 0 && payload != nil {
		payloadBytes, err := json.Marshal(payload)
		if err == nil && int64(len(payloadBytes)) > policy.MaxPayloadSizeBytes {
			violations = append(violations, PolicyViolation{
				Rule:    "payload_size",
				Field:   "payload",
				Message: fmt.Sprintf("action payload size (%d bytes) exceeds tenant policy limit of %d bytes", len(payloadBytes), policy.MaxPayloadSizeBytes),
				Details: map[string]any{"payload_bytes": len(payloadBytes), "max_allowed_bytes": policy.MaxPayloadSizeBytes},
			})
		}
	}

	if len(violations) > 0 {
		if policy.EnforcementMode == ModeEnforce {
			return &PolicyViolationError{
				TenantID:   tenantID,
				Violations: violations,
			}
		}
		// In audit mode, violations are permitted at runtime but can be recorded
	}

	return nil
}

// MaskExecution returns a deep-copy of the Execution with PII and secrets masked according to tenant policy.
func (ge *GovernanceEngine) MaskExecution(tenantID string, exec *workflow.Execution) *workflow.Execution {
	if exec == nil {
		return nil
	}
	if tenantID == "" {
		tenantID = exec.TenantID
	}
	if tenantID == "" {
		tenantID = "default"
	}

	policy, _ := ge.store.GetPolicy(context.Background(), tenantID)
	if policy == nil {
		policy = DefaultPolicy(tenantID)
	}

	cp := *exec
	if len(exec.Steps) > 0 {
		cp.Steps = make([]workflow.ExecutionStep, len(exec.Steps))
		for i, s := range exec.Steps {
			cp.Steps[i] = *ge.MaskStep(tenantID, &s)
		}
	}
	return &cp
}

// MaskStep masks sensitive data and PII in an individual execution step.
func (ge *GovernanceEngine) MaskStep(tenantID string, step *workflow.ExecutionStep) *workflow.ExecutionStep {
	if step == nil {
		return nil
	}
	if tenantID == "" {
		tenantID = "default"
	}

	policy, _ := ge.store.GetPolicy(context.Background(), tenantID)
	if policy == nil {
		policy = DefaultPolicy(tenantID)
	}

	cp := *step
	cp.Input = MaskValue(step.Input, policy.MaskPII, policy.MaskSecrets)
	cp.Output = MaskValue(step.Output, policy.MaskPII, policy.MaskSecrets)
	if step.Error != nil {
		maskedErr := MaskString(*step.Error, policy.MaskPII, policy.MaskSecrets)
		cp.Error = &maskedErr
	}
	return &cp
}

// MaskData masks arbitrary data structures based on the tenant's policy.
func (ge *GovernanceEngine) MaskData(tenantID string, data any) any {
	if tenantID == "" {
		tenantID = "default"
	}
	policy, _ := ge.store.GetPolicy(context.Background(), tenantID)
	if policy == nil {
		policy = DefaultPolicy(tenantID)
	}
	return MaskValue(data, policy.MaskPII, policy.MaskSecrets)
}

// MaskString masks a string based on the tenant's policy.
func (ge *GovernanceEngine) MaskString(tenantID string, s string) string {
	if tenantID == "" {
		tenantID = "default"
	}
	policy, _ := ge.store.GetPolicy(context.Background(), tenantID)
	if policy == nil {
		policy = DefaultPolicy(tenantID)
	}
	return MaskString(s, policy.MaskPII, policy.MaskSecrets)
}

// Helpers

func isConnectorAllowed(connector string, allowed []string) bool {
	if len(allowed) == 0 {
		return true
	}
	lower := strings.ToLower(strings.TrimSpace(connector))
	for _, item := range allowed {
		clean := strings.ToLower(strings.TrimSpace(item))
		if clean == "*" || clean == lower {
			return true
		}
	}
	return false
}

func matchDomain(targetHost string, allowedPatterns []string) bool {
	if len(allowedPatterns) == 0 {
		return true
	}
	host := strings.ToLower(strings.TrimSpace(targetHost))
	if idx := strings.Index(host, ":"); idx != -1 {
		host = host[:idx]
	}

	for _, pattern := range allowedPatterns {
		pat := strings.ToLower(strings.TrimSpace(pattern))
		if pat == "*" {
			return true
		}
		if pat == host {
			return true
		}
		if strings.HasPrefix(pat, "*.") {
			root := pat[2:]
			if host == root || strings.HasSuffix(host, "."+root) {
				return true
			}
		}
	}
	return false
}

func extractURLFromConfig(cfg json.RawMessage) string {
	if len(cfg) == 0 {
		return ""
	}
	var m map[string]any
	if err := json.Unmarshal(cfg, &m); err != nil {
		return ""
	}
	for _, k := range []string{"url", "endpoint", "webhook_url", "target_url", "uri"} {
		if val, ok := m[k]; ok {
			if s, ok := val.(string); ok && s != "" {
				return s
			}
		}
	}
	return ""
}

func isIrreversibleAction(act workflow.ActionStep) bool {
	// Step explicitly marked irreversible in config
	if len(act.Config) > 0 {
		var cfg map[string]any
		if err := json.Unmarshal(act.Config, &cfg); err == nil {
			if irr, ok := cfg["irreversible"].(bool); ok && irr {
				return true
			}
		}
	}

	// Action type or step name indicating destructive / permanent changes
	destructiveKeywords := []string{
		"delete",
		"destroy",
		"drop",
		"terminate",
		"purge",
		"charge",
		"refund",
		"payment",
		"transfer_funds",
	}

	lowerType := strings.ToLower(act.ActionType)
	lowerName := strings.ToLower(act.Name)

	for _, kw := range destructiveKeywords {
		if strings.Contains(lowerType, kw) || strings.Contains(lowerName, kw) {
			return true
		}
	}

	return false
}
