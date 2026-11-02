package auth

import (
	"context"
	"errors"
)

var (
	ErrTenantNotFound     = errors.New("tenant not found")
	ErrTenantSlugExists   = errors.New("tenant slug already exists")
	ErrUserNotFound       = errors.New("user not found")
	ErrUserEmailExists    = errors.New("user email already exists")
	ErrMembershipNotFound = errors.New("membership not found")
	ErrProjectNotFound    = errors.New("project not found")
	ErrTokenNotFound      = errors.New("api token not found")
	ErrTokenExpired       = errors.New("api token has expired")
	ErrUnauthorized       = errors.New("authentication required")
	ErrForbidden          = errors.New("access denied: insufficient permissions")
	ErrCrossTenantDenied  = errors.New("access denied: cross-tenant access prohibited")
)

// Store defines the persistent storage operations for multi-tenancy, RBAC, service accounts, and audit logs.
type Store interface {
	// Tenants
	CreateTenant(ctx context.Context, tenant *Tenant) (*Tenant, error)
	GetTenant(ctx context.Context, id string) (*Tenant, error)
	GetTenantBySlug(ctx context.Context, slug string) (*Tenant, error)
	ListTenants(ctx context.Context) ([]*Tenant, error)
	DeleteTenant(ctx context.Context, id string) error

	// Users & Memberships
	CreateUser(ctx context.Context, user *User) (*User, error)
	GetUser(ctx context.Context, id string) (*User, error)
	GetUserByEmail(ctx context.Context, email string) (*User, error)
	CreateMembership(ctx context.Context, m *Membership) (*Membership, error)
	GetMembership(ctx context.Context, tenantID, userID string) (*Membership, error)
	ListMemberships(ctx context.Context, tenantID string) ([]*Membership, error)
	UpdateMembershipRole(ctx context.Context, tenantID, userID string, role Role) error
	DeleteMembership(ctx context.Context, tenantID, userID string) error

	// Projects
	CreateProject(ctx context.Context, p *Project) (*Project, error)
	GetProject(ctx context.Context, tenantID, id string) (*Project, error)
	ListProjects(ctx context.Context, tenantID string) ([]*Project, error)
	DeleteProject(ctx context.Context, tenantID, id string) error

	// API Tokens (Service Accounts)
	CreateAPIToken(ctx context.Context, token *APIToken) (*APIToken, error)
	GetAPITokenByHash(ctx context.Context, tokenHash string) (*APIToken, error)
	ListAPITokens(ctx context.Context, tenantID string) ([]*APIToken, error)
	RevokeAPIToken(ctx context.Context, tenantID, id string) error
	TouchAPIToken(ctx context.Context, id string) error

	// Audit Logs
	RecordAuditLog(ctx context.Context, log *AuditLog) error
	ListAuditLogs(ctx context.Context, tenantID string, filter AuditFilter) ([]*AuditLog, error)
}
