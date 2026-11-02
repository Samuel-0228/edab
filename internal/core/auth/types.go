package auth

import (
	"context"
	"strings"
	"time"
)

// Role defines the authorization rank and capability set for a tenant membership or API token.
type Role string

const (
	RoleOwner    Role = "owner"
	RoleAdmin    Role = "admin"
	RoleEditor   Role = "editor"
	RoleOperator Role = "operator"
	RoleViewer   Role = "viewer"
)

// Permission defines a specific granular operational capability.
type Permission string

const (
	// Workflows
	PermWorkflowsRead    Permission = "workflows:read"
	PermWorkflowsWrite   Permission = "workflows:write"
	PermWorkflowsDelete  Permission = "workflows:delete"
	PermWorkflowsExecute Permission = "workflows:execute"

	// Integrations & Catalog
	PermIntegrationsRead  Permission = "integrations:read"
	PermIntegrationsWrite Permission = "integrations:write"

	// Credentials
	PermCredentialsRead   Permission = "credentials:read"
	PermCredentialsWrite  Permission = "credentials:write"
	PermCredentialsDelete Permission = "credentials:delete"

	// Events & Deliveries
	PermEventsRead   Permission = "events:read"
	PermEventsWrite  Permission = "events:write"
	PermEventsReplay Permission = "events:replay"

	// Executions & Logs
	PermExecutionsRead  Permission = "executions:read"
	PermExecutionsRetry Permission = "executions:retry"

	// Dead-Letter Failures
	PermDLQRead  Permission = "dlq:read"
	PermDLQRetry Permission = "dlq:retry"

	// Webhooks
	PermWebhooksRead  Permission = "webhooks:read"
	PermWebhooksWrite Permission = "webhooks:write"

	// Audit Logs
	PermAuditRead Permission = "audit:read"

	// Administrative Management
	PermMembersManage Permission = "members:manage"
	PermTokensManage  Permission = "tokens:manage"
	PermTenantManage  Permission = "tenant:manage"
)

// ValidRoles returns all standard recognized roles.
func ValidRoles() []Role {
	return []Role{RoleOwner, RoleAdmin, RoleEditor, RoleOperator, RoleViewer}
}

// IsValid reports whether the role is a recognized RBAC role.
func (r Role) IsValid() bool {
	switch r {
	case RoleOwner, RoleAdmin, RoleEditor, RoleOperator, RoleViewer:
		return true
	default:
		return false
	}
}

// Rank returns a numeric precedence for role comparison.
func (r Role) Rank() int {
	switch r {
	case RoleOwner:
		return 5
	case RoleAdmin:
		return 4
	case RoleEditor:
		return 3
	case RoleOperator:
		return 2
	case RoleViewer:
		return 1
	default:
		return 0
	}
}

// HasPermission checks if the role natively satisfies the requested permission.
func (r Role) HasPermission(p Permission) bool {
	switch r {
	case RoleOwner:
		// Owner has all permissions
		return true

	case RoleAdmin:
		// Admin has all except tenant ownership destruction/transfer
		return p != PermTenantManage

	case RoleEditor:
		// Editor has read, write, execute, retry, replay across resources, but no member/token management
		switch p {
		case PermMembersManage, PermTokensManage, PermTenantManage:
			return false
		default:
			return true
		}

	case RoleOperator:
		// Operator has all viewer reads plus execution triggers, retries, and event replay
		switch p {
		case PermWorkflowsRead, PermIntegrationsRead, PermCredentialsRead,
			PermEventsRead, PermExecutionsRead, PermDLQRead, PermWebhooksRead,
			PermAuditRead, PermWorkflowsExecute, PermEventsWrite,
			PermEventsReplay, PermExecutionsRetry, PermDLQRetry:
			return true
		default:
			return false
		}

	case RoleViewer:
		// Viewer has read-only access (credentials returned only with masked values)
		switch p {
		case PermWorkflowsRead, PermIntegrationsRead, PermCredentialsRead,
			PermEventsRead, PermExecutionsRead, PermDLQRead, PermWebhooksRead,
			PermAuditRead:
			return true
		default:
			return false
		}

	default:
		return false
	}
}

// Tenant represents an isolated customer organization or namespace.
type Tenant struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Slug      string    `json:"slug"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// User represents an identity that can belong to one or more tenants.
type User struct {
	ID           string    `json:"id"`
	Email        string    `json:"email"`
	Name         string    `json:"name"`
	PasswordHash string    `json:"-"`
	IsSystem     bool      `json:"is_system"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// Membership binds a User to a Tenant with a specific assigned Role.
type Membership struct {
	ID        string    `json:"id"`
	TenantID  string    `json:"tenant_id"`
	UserID    string    `json:"user_id"`
	Role      Role      `json:"role"`
	UserEmail string    `json:"user_email,omitempty"`
	UserName  string    `json:"user_name,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Project represents a logical sub-workspace within a Tenant.
type Project struct {
	ID          string    `json:"id"`
	TenantID    string    `json:"tenant_id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// APIToken represents a machine identity (service account) with scoped capabilities.
type APIToken struct {
	ID          string     `json:"id"`
	TenantID    string     `json:"tenant_id"`
	ProjectID   *string    `json:"project_id,omitempty"`
	Name        string     `json:"name"`
	TokenPrefix string     `json:"token_prefix"`
	TokenHash   string     `json:"-"`
	Role        Role       `json:"role"`
	Scopes      []string   `json:"scopes,omitempty"`
	LastUsedAt  *time.Time `json:"last_used_at,omitempty"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
	CreatedBy   string     `json:"created_by"`
	CreatedAt   time.Time  `json:"created_at"`
}

// AuditLog captures an immutable historical record of an authenticated activity or mutation.
type AuditLog struct {
	ID           string         `json:"id"`
	TenantID     string         `json:"tenant_id"`
	ActorID      string         `json:"actor_id"`
	ActorType    string         `json:"actor_type"` // "user", "api_token", "system"
	Action       string         `json:"action"`     // e.g. "workflow.create", "credential.decrypt"
	ResourceType string         `json:"resource_type"`
	ResourceID   string         `json:"resource_id"`
	Details      map[string]any `json:"details,omitempty"`
	IPAddress    string         `json:"ip_address,omitempty"`
	CreatedAt    time.Time      `json:"created_at"`
}

// AuditFilter defines query filtering options for audit logs.
type AuditFilter struct {
	ResourceType string
	ResourceID   string
	ActorID      string
	Action       string
	From         time.Time
	To           time.Time
	Limit        int
	Offset       int
}

// Identity represents the authenticated caller context attached to an HTTP request.
type Identity struct {
	TenantID  string   `json:"tenant_id"`
	ProjectID string   `json:"project_id,omitempty"`
	UserID    string   `json:"user_id"`
	ActorType string   `json:"actor_type"` // "user", "api_token", "system", "anonymous"
	Role      Role     `json:"role"`
	Scopes    []string `json:"scopes,omitempty"`
	IsAuth    bool     `json:"is_auth"`
}

// HasPermission checks if the identity satisfies the requested permission either via role or scoped permission.
func (id *Identity) HasPermission(p Permission) bool {
	if id == nil || !id.IsAuth {
		return false
	}
	// Check explicit scopes on API tokens if populated
	if len(id.Scopes) > 0 {
		permStr := string(p)
		for _, s := range id.Scopes {
			if s == "*" || s == permStr {
				return true
			}
			// Prefix wildcard e.g. "workflows:*"
			if strings.HasSuffix(s, ":*") {
				prefix := strings.TrimSuffix(s, ":*")
				if strings.HasPrefix(permStr, prefix+":") {
					return true
				}
			}
		}
		return false
	}
	return id.Role.HasPermission(p)
}

// CanAccessTenant ensures identity belongs to the target tenant.
func (id *Identity) CanAccessTenant(targetTenantID string) bool {
	if id == nil || !id.IsAuth {
		return false
	}
	if targetTenantID == "" {
		return true
	}
	return id.TenantID == targetTenantID
}

// Default single-tenant fallback identity for backward-compatible development.
var DefaultIdentity = &Identity{
	TenantID:  "default",
	ProjectID: "default",
	UserID:    "default-user",
	ActorType: "system",
	Role:      RoleOwner,
	IsAuth:    true,
}

type authContextKey struct{}

// WithIdentity returns a child context populated with the given Identity.
func WithIdentity(ctx context.Context, id *Identity) context.Context {
	return context.WithValue(ctx, authContextKey{}, id)
}

// GetIdentity extracts the Identity from the context, falling back to DefaultIdentity if omitted.
func GetIdentity(ctx context.Context) *Identity {
	if val := ctx.Value(authContextKey{}); val != nil {
		if id, ok := val.(*Identity); ok && id != nil {
			return id
		}
	}
	return DefaultIdentity
}
