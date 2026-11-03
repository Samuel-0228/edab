package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"kerob/internal/core/auth"
	"kerob/internal/core/governance"
	"kerob/internal/core/workflow"
)

// GetGovernancePolicy returns the active governance policy for the caller's tenant.
func (h *Handlers) GetGovernancePolicy(w http.ResponseWriter, r *http.Request) {
	if !h.requirePermission(w, r, auth.PermPolicyRead) {
		return
	}
	identity := auth.GetIdentity(r.Context())
	tenantID := identity.TenantID
	if tenantID == "" {
		tenantID = "default"
	}

	policy, err := h.governanceEngine.GetPolicy(r.Context(), tenantID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to get governance policy: "+err.Error())
		return
	}

	respondJSON(w, http.StatusOK, policy)
}

// UpdateGovernancePolicy creates or updates the governance policy for the caller's tenant.
func (h *Handlers) UpdateGovernancePolicy(w http.ResponseWriter, r *http.Request) {
	if !h.requirePermission(w, r, auth.PermPolicyWrite) {
		return
	}
	identity := auth.GetIdentity(r.Context())
	tenantID := identity.TenantID
	if tenantID == "" {
		tenantID = "default"
	}

	var p governance.Policy
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		respondErrorWithCode(w, http.StatusBadRequest, "MALFORMED_JSON", "invalid json: "+err.Error(), nil)
		return
	}

	// Always bind policy to the authenticated tenant
	p.TenantID = tenantID

	updated, err := h.governanceEngine.Store().UpsertPolicy(r.Context(), &p)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to save policy: "+err.Error())
		return
	}

	h.recordAudit(r, "governance.policy_updated", "policy", tenantID, map[string]any{
		"enforcement_mode":       updated.EnforcementMode,
		"max_retries":            updated.MaxWorkflowRetries,
		"allowed_connectors":     updated.AllowedConnectors,
		"allowed_outbound":       updated.AllowedOutboundDomains,
		"retention_days":         updated.DataRetentionDays,
		"require_approval_irrev": updated.RequireApprovalForIrreversibleActions,
	})

	respondJSON(w, http.StatusOK, updated)
}

// ValidateWorkflowPolicy validates a candidate workflow JSON definition against tenant policy.
func (h *Handlers) ValidateWorkflowPolicy(w http.ResponseWriter, r *http.Request) {
	if !h.requirePermission(w, r, auth.PermPolicyRead) && !h.requirePermission(w, r, auth.PermWorkflowsRead) {
		return
	}
	identity := auth.GetIdentity(r.Context())
	tenantID := identity.TenantID
	if tenantID == "" {
		tenantID = "default"
	}

	var wf workflow.Workflow
	if err := json.NewDecoder(r.Body).Decode(&wf); err != nil {
		respondErrorWithCode(w, http.StatusBadRequest, "MALFORMED_JSON", "invalid json: "+err.Error(), nil)
		return
	}

	if wf.TenantID == "" {
		wf.TenantID = tenantID
	}

	violations := h.governanceEngine.ValidateWorkflowPublish(r.Context(), tenantID, &wf)
	if violations == nil {
		violations = []governance.PolicyViolation{}
	}

	respondJSON(w, http.StatusOK, map[string]any{
		"valid":      len(violations) == 0,
		"violations": violations,
		"tenant_id":  tenantID,
	})
}

// VerifyAuditChain validates the cryptographic integrity of the tenant's audit trail.
func (h *Handlers) VerifyAuditChain(w http.ResponseWriter, r *http.Request) {
	if !h.requirePermission(w, r, auth.PermAuditRead) {
		return
	}
	identity := auth.GetIdentity(r.Context())
	tenantID := identity.TenantID
	if tenantID == "" {
		tenantID = "default"
	}

	logs, err := h.authStore.ListAuditLogs(r.Context(), tenantID, auth.AuditFilter{Limit: 1000})
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to query audit logs: "+err.Error())
		return
	}

	result := governance.VerifyAuditChain(tenantID, logs)
	respondJSON(w, http.StatusOK, result)
}

// ExportAuditLogs exports the caller tenant's audit logs in JSON or CSV format.
func (h *Handlers) ExportAuditLogs(w http.ResponseWriter, r *http.Request) {
	if !h.requirePermission(w, r, auth.PermAuditExport) && !h.requirePermission(w, r, auth.PermAuditRead) {
		return
	}
	identity := auth.GetIdentity(r.Context())
	tenantID := identity.TenantID
	if tenantID == "" {
		tenantID = "default"
	}

	format := strings.ToLower(r.URL.Query().Get("format"))
	if format == "" {
		format = "json"
	}

	filter := auth.AuditFilter{
		ResourceType: r.URL.Query().Get("resource_type"),
		ResourceID:   r.URL.Query().Get("resource_id"),
		ActorID:      r.URL.Query().Get("actor_id"),
		Action:       r.URL.Query().Get("action"),
		Limit:        5000,
	}

	if fromStr := r.URL.Query().Get("from"); fromStr != "" {
		if t, err := time.Parse(time.RFC3339, fromStr); err == nil {
			filter.From = t
		}
	}
	if toStr := r.URL.Query().Get("to"); toStr != "" {
		if t, err := time.Parse(time.RFC3339, toStr); err == nil {
			filter.To = t
		}
	}

	logs, err := h.authStore.ListAuditLogs(r.Context(), tenantID, filter)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to query audit logs: "+err.Error())
		return
	}

	h.recordAudit(r, "audit.exported", "audit_log", tenantID, map[string]any{
		"format": format,
		"count":  len(logs),
	})

	if format == "csv" {
		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		w.Header().Set("Content-Disposition", "attachment; filename=\"audit_logs.csv\"")
		if err := governance.ExportAuditLogsCSV(w, logs); err != nil {
			respondError(w, http.StatusInternalServerError, "failed to export csv: "+err.Error())
		}
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename=\"audit_logs.json\"")
	if err := governance.ExportAuditLogsJSON(w, logs); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to export json: "+err.Error())
	}
}

// PruneRetention triggers a cleanup of historical audit records and executions exceeding policy retention.
func (h *Handlers) PruneRetention(w http.ResponseWriter, r *http.Request) {
	if !h.requirePermission(w, r, auth.PermPolicyWrite) && !h.requirePermission(w, r, auth.PermTenantManage) {
		return
	}
	identity := auth.GetIdentity(r.Context())
	tenantID := identity.TenantID
	if tenantID == "" {
		tenantID = "default"
	}

	policy, err := h.governanceEngine.GetPolicy(r.Context(), tenantID)
	if err != nil || policy == nil {
		policy = governance.DefaultPolicy(tenantID)
	}

	days := policy.DataRetentionDays
	if days <= 0 {
		days = 90
	}
	cutoff := time.Now().UTC().AddDate(0, 0, -days)

	prunedCount, err := h.authStore.PruneAuditLogs(r.Context(), tenantID, cutoff)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to prune audit retention: "+err.Error())
		return
	}

	h.recordAudit(r, "governance.retention_pruned", "retention", tenantID, map[string]any{
		"pruned_audit_logs": prunedCount,
		"retention_days":    days,
		"cutoff":            cutoff,
	})

	respondJSON(w, http.StatusOK, map[string]any{
		"pruned_audit_logs": prunedCount,
		"retention_days":    days,
		"cutoff":            cutoff,
		"tenant_id":         tenantID,
	})
}

func respondPolicyViolations(w http.ResponseWriter, violations []governance.PolicyViolation) {
	fieldErrs := make([]FieldError, len(violations))
	for i, v := range violations {
		fieldErrs[i] = FieldError{
			Field:   v.Field,
			Message: v.Message,
		}
	}
	respondErrorWithCode(w, http.StatusUnprocessableEntity, "POLICY_VIOLATION", "workflow violates tenant governance policies", fieldErrs)
}
