package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"kerob/internal/core/auth"
)

// recordAudit records an audit event to the persistent auth store with secrets scrubbed.
func (h *Handlers) recordAudit(r *http.Request, action, resourceType, resourceID string, details map[string]any) {
	if h.authStore == nil {
		return
	}
	id := auth.GetIdentity(r.Context())
	tenantID := id.TenantID
	if tenantID == "" {
		tenantID = "default"
	}
	actorID := id.UserID
	if actorID == "" {
		actorID = "anonymous"
	}
	actorType := id.ActorType
	if actorType == "" {
		actorType = "system"
	}

	log := &auth.AuditLog{
		TenantID:     tenantID,
		ActorID:      actorID,
		ActorType:    actorType,
		Action:       action,
		ResourceType: resourceType,
		ResourceID:   resourceID,
		Details:      auth.ScrubAuditDetails(details),
		IPAddress:    r.RemoteAddr,
		CreatedAt:    time.Now().UTC(),
	}

	_ = h.authStore.RecordAuditLog(r.Context(), log)
}

// requirePermission verifies that the caller has the required RBAC permission.
func (h *Handlers) requirePermission(w http.ResponseWriter, r *http.Request, perm auth.Permission) bool {
	id := auth.GetIdentity(r.Context())
	if !h.multiTenancy && !id.IsAuth {
		return true
	}
	if !id.HasPermission(perm) {
		h.recordAudit(r, "auth.forbidden", "permission", string(perm), map[string]any{
			"required_permission": string(perm),
			"current_role":        string(id.Role),
			"path":                r.URL.Path,
			"method":              r.Method,
		})
		respondErrorWithCode(w, http.StatusForbidden, "PERMISSION_DENIED",
			fmt.Sprintf("role '%s' lacks required permission '%s'", id.Role, perm), nil)
		return false
	}
	return true
}

// Me returns the caller's authenticated identity, tenant, and role.
func (h *Handlers) Me(w http.ResponseWriter, r *http.Request) {
	id := auth.GetIdentity(r.Context())
	respondJSON(w, http.StatusOK, map[string]any{
		"tenant_id":  id.TenantID,
		"project_id": id.ProjectID,
		"user_id":    id.UserID,
		"actor_type": id.ActorType,
		"role":       id.Role,
		"scopes":     id.Scopes,
		"is_auth":    id.IsAuth,
	})
}

// --- Tenant Management ---

func (h *Handlers) ListTenants(w http.ResponseWriter, r *http.Request) {
	if h.authStore == nil {
		respondJSON(w, http.StatusOK, []*auth.Tenant{})
		return
	}
	id := auth.GetIdentity(r.Context())
	if !h.requirePermission(w, r, auth.PermAuditRead) {
		return
	}

	list, err := h.authStore.ListTenants(r.Context())
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Non-owners only see their own tenant
	if id.Role != auth.RoleOwner {
		filtered := make([]*auth.Tenant, 0)
		for _, t := range list {
			if t.ID == id.TenantID {
				filtered = append(filtered, t)
			}
		}
		list = filtered
	}

	respondJSON(w, http.StatusOK, list)
}

type tenantInput struct {
	Name string `json:"name"`
	Slug string `json:"slug"`
}

func (h *Handlers) CreateTenant(w http.ResponseWriter, r *http.Request) {
	if h.authStore == nil {
		respondError(w, http.StatusInternalServerError, "auth store not initialized")
		return
	}
	if !h.requirePermission(w, r, auth.PermTenantManage) {
		return
	}

	var in tenantInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		respondErrorWithCode(w, http.StatusBadRequest, "MALFORMED_JSON", "invalid json: "+err.Error(), nil)
		return
	}
	if strings.TrimSpace(in.Name) == "" {
		respondError(w, http.StatusBadRequest, "tenant name is required")
		return
	}

	t := &auth.Tenant{
		Name: strings.TrimSpace(in.Name),
		Slug: strings.TrimSpace(in.Slug),
	}
	created, err := h.authStore.CreateTenant(r.Context(), t)
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	h.recordAudit(r, "tenant.created", "tenant", created.ID, map[string]any{
		"name": created.Name,
		"slug": created.Slug,
	})

	respondJSON(w, http.StatusCreated, created)
}

func (h *Handlers) GetTenant(w http.ResponseWriter, r *http.Request) {
	if h.authStore == nil {
		respondError(w, http.StatusInternalServerError, "auth store not initialized")
		return
	}
	id := auth.GetIdentity(r.Context())
	targetID := r.PathValue("id")
	if targetID == "" {
		targetID = id.TenantID
	}

	if targetID != id.TenantID && id.Role != auth.RoleOwner {
		respondErrorWithCode(w, http.StatusForbidden, "CROSS_TENANT_DENIED", "cannot access another tenant", nil)
		return
	}

	t, err := h.authStore.GetTenant(r.Context(), targetID)
	if err != nil {
		respondError(w, http.StatusNotFound, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, t)
}

// --- Membership & RBAC Management ---

func (h *Handlers) ListMembers(w http.ResponseWriter, r *http.Request) {
	if h.authStore == nil {
		respondJSON(w, http.StatusOK, []*auth.Membership{})
		return
	}
	if !h.requirePermission(w, r, auth.PermMembersManage) {
		return
	}

	id := auth.GetIdentity(r.Context())
	list, err := h.authStore.ListMemberships(r.Context(), id.TenantID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, list)
}

type memberInput struct {
	Email string    `json:"email"`
	Name  string    `json:"name"`
	Role  auth.Role `json:"role"`
}

func (h *Handlers) CreateMember(w http.ResponseWriter, r *http.Request) {
	if h.authStore == nil {
		respondError(w, http.StatusInternalServerError, "auth store not initialized")
		return
	}
	if !h.requirePermission(w, r, auth.PermMembersManage) {
		return
	}

	var in memberInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		respondErrorWithCode(w, http.StatusBadRequest, "MALFORMED_JSON", "invalid json: "+err.Error(), nil)
		return
	}
	if strings.TrimSpace(in.Email) == "" {
		respondError(w, http.StatusBadRequest, "email is required")
		return
	}
	if !in.Role.IsValid() {
		respondError(w, http.StatusBadRequest, "invalid role (must be owner, admin, editor, operator, viewer)")
		return
	}

	id := auth.GetIdentity(r.Context())
	// Find or create user
	user, err := h.authStore.GetUserByEmail(r.Context(), in.Email)
	if err != nil || user == nil {
		userName := in.Name
		if userName == "" {
			userName = strings.Split(in.Email, "@")[0]
		}
		user, err = h.authStore.CreateUser(r.Context(), &auth.User{
			Email: in.Email,
			Name:  userName,
		})
		if err != nil {
			respondError(w, http.StatusBadRequest, "failed to create user: "+err.Error())
			return
		}
	}

	mem, err := h.authStore.CreateMembership(r.Context(), &auth.Membership{
		TenantID: id.TenantID,
		UserID:   user.ID,
		Role:     in.Role,
	})
	if err != nil {
		respondError(w, http.StatusBadRequest, "failed to create membership: "+err.Error())
		return
	}

	h.recordAudit(r, "member.created", "member", mem.UserID, map[string]any{
		"email": user.Email,
		"role":  string(mem.Role),
	})

	respondJSON(w, http.StatusCreated, mem)
}

type updateRoleInput struct {
	Role auth.Role `json:"role"`
}

func (h *Handlers) UpdateMemberRole(w http.ResponseWriter, r *http.Request) {
	if h.authStore == nil {
		respondError(w, http.StatusInternalServerError, "auth store not initialized")
		return
	}
	if !h.requirePermission(w, r, auth.PermMembersManage) {
		return
	}

	targetUserID := r.PathValue("id")
	if targetUserID == "" {
		respondError(w, http.StatusBadRequest, "member user id is required")
		return
	}

	var in updateRoleInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		respondErrorWithCode(w, http.StatusBadRequest, "MALFORMED_JSON", "invalid json: "+err.Error(), nil)
		return
	}
	if !in.Role.IsValid() {
		respondError(w, http.StatusBadRequest, "invalid role")
		return
	}

	id := auth.GetIdentity(r.Context())
	if err := h.authStore.UpdateMembershipRole(r.Context(), id.TenantID, targetUserID, in.Role); err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	h.recordAudit(r, "member.role_updated", "member", targetUserID, map[string]any{
		"new_role": string(in.Role),
	})

	respondJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

func (h *Handlers) DeleteMember(w http.ResponseWriter, r *http.Request) {
	if h.authStore == nil {
		respondError(w, http.StatusInternalServerError, "auth store not initialized")
		return
	}
	if !h.requirePermission(w, r, auth.PermMembersManage) {
		return
	}

	targetUserID := r.PathValue("id")
	id := auth.GetIdentity(r.Context())

	if err := h.authStore.DeleteMembership(r.Context(), id.TenantID, targetUserID); err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	h.recordAudit(r, "member.deleted", "member", targetUserID, nil)
	respondJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// --- API Tokens (Service Accounts) ---

func (h *Handlers) ListAPITokens(w http.ResponseWriter, r *http.Request) {
	if h.authStore == nil {
		respondJSON(w, http.StatusOK, []*auth.APIToken{})
		return
	}
	if !h.requirePermission(w, r, auth.PermTokensManage) {
		return
	}

	id := auth.GetIdentity(r.Context())
	list, err := h.authStore.ListAPITokens(r.Context(), id.TenantID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, list)
}

type apiTokenCreateInput struct {
	Name      string    `json:"name"`
	Role      auth.Role `json:"role"`
	Scopes    []string  `json:"scopes"`
	ProjectID *string   `json:"project_id,omitempty"`
	ExpiresIn string    `json:"expires_in,omitempty"` // e.g. "720h" (30 days)
}

type apiTokenCreateResponse struct {
	*auth.APIToken
	RawToken string `json:"raw_token"`
}

func (h *Handlers) CreateAPIToken(w http.ResponseWriter, r *http.Request) {
	if h.authStore == nil {
		respondError(w, http.StatusInternalServerError, "auth store not initialized")
		return
	}
	if !h.requirePermission(w, r, auth.PermTokensManage) {
		return
	}

	var in apiTokenCreateInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		respondErrorWithCode(w, http.StatusBadRequest, "MALFORMED_JSON", "invalid json: "+err.Error(), nil)
		return
	}
	if strings.TrimSpace(in.Name) == "" {
		respondError(w, http.StatusBadRequest, "token name is required")
		return
	}
	if in.Role == "" {
		in.Role = auth.RoleOperator
	}
	if !in.Role.IsValid() {
		respondError(w, http.StatusBadRequest, "invalid role")
		return
	}

	id := auth.GetIdentity(r.Context())

	// Token role cannot exceed creator's role rank
	if in.Role.Rank() > id.Role.Rank() {
		respondError(w, http.StatusBadRequest, "cannot grant a role higher than creator's role")
		return
	}

	var expiresAt *time.Time
	if in.ExpiresIn != "" {
		d, err := time.ParseDuration(in.ExpiresIn)
		if err == nil && d > 0 {
			exp := time.Now().UTC().Add(d)
			expiresAt = &exp
		}
	}

	rawToken, prefix, hash, err := auth.GenerateAPIToken("live")
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to generate token: "+err.Error())
		return
	}

	tok := &auth.APIToken{
		TenantID:    id.TenantID,
		ProjectID:   in.ProjectID,
		Name:        strings.TrimSpace(in.Name),
		TokenPrefix: prefix,
		TokenHash:   hash,
		Role:        in.Role,
		Scopes:      in.Scopes,
		ExpiresAt:   expiresAt,
		CreatedBy:   id.UserID,
	}

	created, err := h.authStore.CreateAPIToken(r.Context(), tok)
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	h.recordAudit(r, "token.created", "api_token", created.ID, map[string]any{
		"token_name":   created.Name,
		"token_prefix": created.TokenPrefix,
		"role":         string(created.Role),
		"scopes":       created.Scopes,
	})

	respondJSON(w, http.StatusCreated, apiTokenCreateResponse{
		APIToken: created,
		RawToken: rawToken,
	})
}

func (h *Handlers) RevokeAPIToken(w http.ResponseWriter, r *http.Request) {
	if h.authStore == nil {
		respondError(w, http.StatusInternalServerError, "auth store not initialized")
		return
	}
	if !h.requirePermission(w, r, auth.PermTokensManage) {
		return
	}

	tokenID := r.PathValue("id")
	id := auth.GetIdentity(r.Context())

	if err := h.authStore.RevokeAPIToken(r.Context(), id.TenantID, tokenID); err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	h.recordAudit(r, "token.revoked", "api_token", tokenID, nil)
	respondJSON(w, http.StatusOK, map[string]string{"status": "revoked"})
}

// --- Audit Logs ---

func (h *Handlers) ListAuditLogs(w http.ResponseWriter, r *http.Request) {
	if h.authStore == nil {
		respondJSON(w, http.StatusOK, []*auth.AuditLog{})
		return
	}
	if !h.requirePermission(w, r, auth.PermAuditRead) {
		return
	}

	id := auth.GetIdentity(r.Context())
	filter := auth.AuditFilter{
		ResourceType: r.URL.Query().Get("resource_type"),
		ResourceID:   r.URL.Query().Get("resource_id"),
		ActorID:      r.URL.Query().Get("actor_id"),
		Action:       r.URL.Query().Get("action"),
	}

	list, err := h.authStore.ListAuditLogs(r.Context(), id.TenantID, filter)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, list)
}
