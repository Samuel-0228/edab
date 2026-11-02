package auth

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
)

// PostgresStore implements auth.Store using PostgreSQL.
type PostgresStore struct {
	db *sql.DB
}

func NewPostgresStore(db *sql.DB) *PostgresStore {
	return &PostgresStore{db: db}
}

// --- Tenants ---

func (s *PostgresStore) CreateTenant(ctx context.Context, tenant *Tenant) (*Tenant, error) {
	if tenant.ID == "" {
		tenant.ID = uuid.NewString()
	}
	if tenant.Slug == "" {
		tenant.Slug = tenant.Name
	}
	now := time.Now().UTC()
	tenant.CreatedAt = now
	tenant.UpdatedAt = now

	query := `
		INSERT INTO tenants (id, name, slug, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, name, slug, created_at, updated_at;
	`
	var t Tenant
	err := s.db.QueryRowContext(ctx, query, tenant.ID, tenant.Name, tenant.Slug, tenant.CreatedAt, tenant.UpdatedAt).
		Scan(&t.ID, &t.Name, &t.Slug, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		if pqErr, ok := err.(*pq.Error); ok && pqErr.Code == "23505" {
			return nil, ErrTenantSlugExists
		}
		return nil, fmt.Errorf("failed to create tenant: %w", err)
	}
	return &t, nil
}

func (s *PostgresStore) GetTenant(ctx context.Context, id string) (*Tenant, error) {
	query := `SELECT id, name, slug, created_at, updated_at FROM tenants WHERE id = $1;`
	var t Tenant
	err := s.db.QueryRowContext(ctx, query, id).Scan(&t.ID, &t.Name, &t.Slug, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrTenantNotFound
		}
		return nil, fmt.Errorf("failed to get tenant: %w", err)
	}
	return &t, nil
}

func (s *PostgresStore) GetTenantBySlug(ctx context.Context, slug string) (*Tenant, error) {
	query := `SELECT id, name, slug, created_at, updated_at FROM tenants WHERE slug = $1;`
	var t Tenant
	err := s.db.QueryRowContext(ctx, query, slug).Scan(&t.ID, &t.Name, &t.Slug, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrTenantNotFound
		}
		return nil, fmt.Errorf("failed to get tenant by slug: %w", err)
	}
	return &t, nil
}

func (s *PostgresStore) ListTenants(ctx context.Context) ([]*Tenant, error) {
	query := `SELECT id, name, slug, created_at, updated_at FROM tenants ORDER BY created_at ASC;`
	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query tenants: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var list []*Tenant
	for rows.Next() {
		var t Tenant
		if err := rows.Scan(&t.ID, &t.Name, &t.Slug, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan tenant: %w", err)
		}
		list = append(list, &t)
	}
	if list == nil {
		list = []*Tenant{}
	}
	return list, nil
}

func (s *PostgresStore) DeleteTenant(ctx context.Context, id string) error {
	query := `DELETE FROM tenants WHERE id = $1;`
	res, err := s.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete tenant: %w", err)
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return ErrTenantNotFound
	}
	return nil
}

// --- Users & Memberships ---

func (s *PostgresStore) CreateUser(ctx context.Context, user *User) (*User, error) {
	if user.ID == "" {
		user.ID = uuid.NewString()
	}
	now := time.Now().UTC()
	user.CreatedAt = now
	user.UpdatedAt = now

	query := `
		INSERT INTO users (id, email, name, password_hash, is_system, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, email, name, password_hash, is_system, created_at, updated_at;
	`
	var u User
	err := s.db.QueryRowContext(ctx, query, user.ID, user.Email, user.Name, user.PasswordHash, user.IsSystem, user.CreatedAt, user.UpdatedAt).
		Scan(&u.ID, &u.Email, &u.Name, &u.PasswordHash, &u.IsSystem, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		if pqErr, ok := err.(*pq.Error); ok && pqErr.Code == "23505" {
			return nil, ErrUserEmailExists
		}
		return nil, fmt.Errorf("failed to create user: %w", err)
	}
	return &u, nil
}

func (s *PostgresStore) GetUser(ctx context.Context, id string) (*User, error) {
	query := `SELECT id, email, name, password_hash, is_system, created_at, updated_at FROM users WHERE id = $1;`
	var u User
	err := s.db.QueryRowContext(ctx, query, id).Scan(&u.ID, &u.Email, &u.Name, &u.PasswordHash, &u.IsSystem, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		return nil, fmt.Errorf("failed to get user: %w", err)
	}
	return &u, nil
}

func (s *PostgresStore) GetUserByEmail(ctx context.Context, email string) (*User, error) {
	query := `SELECT id, email, name, password_hash, is_system, created_at, updated_at FROM users WHERE email = $1;`
	var u User
	err := s.db.QueryRowContext(ctx, query, email).Scan(&u.ID, &u.Email, &u.Name, &u.PasswordHash, &u.IsSystem, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		return nil, fmt.Errorf("failed to get user by email: %w", err)
	}
	return &u, nil
}

func (s *PostgresStore) CreateMembership(ctx context.Context, m *Membership) (*Membership, error) {
	if m.ID == "" {
		m.ID = uuid.NewString()
	}
	now := time.Now().UTC()
	m.CreatedAt = now
	m.UpdatedAt = now

	query := `
		INSERT INTO memberships (id, tenant_id, user_id, role, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, tenant_id, user_id, role, created_at, updated_at;
	`
	var res Membership
	err := s.db.QueryRowContext(ctx, query, m.ID, m.TenantID, m.UserID, string(m.Role), m.CreatedAt, m.UpdatedAt).
		Scan(&res.ID, &res.TenantID, &res.UserID, &res.Role, &res.CreatedAt, &res.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("failed to create membership: %w", err)
	}
	// Fetch user details for convenience
	u, err := s.GetUser(ctx, res.UserID)
	if err == nil && u != nil {
		res.UserEmail = u.Email
		res.UserName = u.Name
	}
	return &res, nil
}

func (s *PostgresStore) GetMembership(ctx context.Context, tenantID, userID string) (*Membership, error) {
	query := `
		SELECT m.id, m.tenant_id, m.user_id, m.role, m.created_at, m.updated_at, u.email, u.name
		FROM memberships m
		JOIN users u ON m.user_id = u.id
		WHERE m.tenant_id = $1 AND m.user_id = $2;
	`
	var m Membership
	err := s.db.QueryRowContext(ctx, query, tenantID, userID).
		Scan(&m.ID, &m.TenantID, &m.UserID, &m.Role, &m.CreatedAt, &m.UpdatedAt, &m.UserEmail, &m.UserName)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrMembershipNotFound
		}
		return nil, fmt.Errorf("failed to get membership: %w", err)
	}
	return &m, nil
}

func (s *PostgresStore) ListMemberships(ctx context.Context, tenantID string) ([]*Membership, error) {
	query := `
		SELECT m.id, m.tenant_id, m.user_id, m.role, m.created_at, m.updated_at, u.email, u.name
		FROM memberships m
		JOIN users u ON m.user_id = u.id
		WHERE m.tenant_id = $1
		ORDER BY m.created_at ASC;
	`
	rows, err := s.db.QueryContext(ctx, query, tenantID)
	if err != nil {
		return nil, fmt.Errorf("failed to list memberships: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var list []*Membership
	for rows.Next() {
		var m Membership
		if err := rows.Scan(&m.ID, &m.TenantID, &m.UserID, &m.Role, &m.CreatedAt, &m.UpdatedAt, &m.UserEmail, &m.UserName); err != nil {
			return nil, fmt.Errorf("failed to scan membership: %w", err)
		}
		list = append(list, &m)
	}
	if list == nil {
		list = []*Membership{}
	}
	return list, nil
}

func (s *PostgresStore) UpdateMembershipRole(ctx context.Context, tenantID, userID string, role Role) error {
	query := `UPDATE memberships SET role = $1, updated_at = NOW() WHERE tenant_id = $2 AND user_id = $3;`
	res, err := s.db.ExecContext(ctx, query, string(role), tenantID, userID)
	if err != nil {
		return fmt.Errorf("failed to update membership role: %w", err)
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return ErrMembershipNotFound
	}
	return nil
}

func (s *PostgresStore) DeleteMembership(ctx context.Context, tenantID, userID string) error {
	query := `DELETE FROM memberships WHERE tenant_id = $1 AND user_id = $2;`
	res, err := s.db.ExecContext(ctx, query, tenantID, userID)
	if err != nil {
		return fmt.Errorf("failed to delete membership: %w", err)
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return ErrMembershipNotFound
	}
	return nil
}

// --- Projects ---

func (s *PostgresStore) CreateProject(ctx context.Context, p *Project) (*Project, error) {
	if p.ID == "" {
		p.ID = uuid.NewString()
	}
	now := time.Now().UTC()
	p.CreatedAt = now
	p.UpdatedAt = now

	query := `
		INSERT INTO projects (id, tenant_id, name, description, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, tenant_id, name, description, created_at, updated_at;
	`
	var res Project
	err := s.db.QueryRowContext(ctx, query, p.ID, p.TenantID, p.Name, p.Description, p.CreatedAt, p.UpdatedAt).
		Scan(&res.ID, &res.TenantID, &res.Name, &res.Description, &res.CreatedAt, &res.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("failed to create project: %w", err)
	}
	return &res, nil
}

func (s *PostgresStore) GetProject(ctx context.Context, tenantID, id string) (*Project, error) {
	query := `SELECT id, tenant_id, name, description, created_at, updated_at FROM projects WHERE tenant_id = $1 AND id = $2;`
	var p Project
	err := s.db.QueryRowContext(ctx, query, tenantID, id).
		Scan(&p.ID, &p.TenantID, &p.Name, &p.Description, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrProjectNotFound
		}
		return nil, fmt.Errorf("failed to get project: %w", err)
	}
	return &p, nil
}

func (s *PostgresStore) ListProjects(ctx context.Context, tenantID string) ([]*Project, error) {
	query := `SELECT id, tenant_id, name, description, created_at, updated_at FROM projects WHERE tenant_id = $1 ORDER BY created_at ASC;`
	rows, err := s.db.QueryContext(ctx, query, tenantID)
	if err != nil {
		return nil, fmt.Errorf("failed to list projects: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var list []*Project
	for rows.Next() {
		var p Project
		if err := rows.Scan(&p.ID, &p.TenantID, &p.Name, &p.Description, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan project: %w", err)
		}
		list = append(list, &p)
	}
	if list == nil {
		list = []*Project{}
	}
	return list, nil
}

func (s *PostgresStore) DeleteProject(ctx context.Context, tenantID, id string) error {
	query := `DELETE FROM projects WHERE tenant_id = $1 AND id = $2;`
	res, err := s.db.ExecContext(ctx, query, tenantID, id)
	if err != nil {
		return fmt.Errorf("failed to delete project: %w", err)
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return ErrProjectNotFound
	}
	return nil
}

// --- API Tokens ---

func (s *PostgresStore) CreateAPIToken(ctx context.Context, token *APIToken) (*APIToken, error) {
	if token.ID == "" {
		token.ID = uuid.NewString()
	}
	token.CreatedAt = time.Now().UTC()

	scopesJSON, err := json.Marshal(token.Scopes)
	if err != nil {
		scopesJSON = []byte("[]")
	}

	query := `
		INSERT INTO api_tokens (id, tenant_id, project_id, name, token_prefix, token_hash, role, scopes, last_used_at, expires_at, created_by, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		RETURNING id, tenant_id, project_id, name, token_prefix, token_hash, role, scopes, last_used_at, expires_at, created_by, created_at;
	`
	var res APIToken
	var rawScopes []byte
	err = s.db.QueryRowContext(ctx, query,
		token.ID, token.TenantID, token.ProjectID, token.Name, token.TokenPrefix, token.TokenHash,
		string(token.Role), scopesJSON, token.LastUsedAt, token.ExpiresAt, token.CreatedBy, token.CreatedAt,
	).Scan(
		&res.ID, &res.TenantID, &res.ProjectID, &res.Name, &res.TokenPrefix, &res.TokenHash,
		&res.Role, &rawScopes, &res.LastUsedAt, &res.ExpiresAt, &res.CreatedBy, &res.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create api token: %w", err)
	}
	_ = json.Unmarshal(rawScopes, &res.Scopes)
	return &res, nil
}

func (s *PostgresStore) GetAPITokenByHash(ctx context.Context, tokenHash string) (*APIToken, error) {
	query := `
		SELECT id, tenant_id, project_id, name, token_prefix, token_hash, role, scopes, last_used_at, expires_at, created_by, created_at
		FROM api_tokens
		WHERE token_hash = $1;
	`
	var res APIToken
	var rawScopes []byte
	err := s.db.QueryRowContext(ctx, query, tokenHash).Scan(
		&res.ID, &res.TenantID, &res.ProjectID, &res.Name, &res.TokenPrefix, &res.TokenHash,
		&res.Role, &rawScopes, &res.LastUsedAt, &res.ExpiresAt, &res.CreatedBy, &res.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrTokenNotFound
		}
		return nil, fmt.Errorf("failed to get api token: %w", err)
	}
	_ = json.Unmarshal(rawScopes, &res.Scopes)

	if res.ExpiresAt != nil && res.ExpiresAt.Before(time.Now().UTC()) {
		return nil, ErrTokenExpired
	}
	return &res, nil
}

func (s *PostgresStore) ListAPITokens(ctx context.Context, tenantID string) ([]*APIToken, error) {
	query := `
		SELECT id, tenant_id, project_id, name, token_prefix, token_hash, role, scopes, last_used_at, expires_at, created_by, created_at
		FROM api_tokens
		WHERE tenant_id = $1
		ORDER BY created_at DESC;
	`
	rows, err := s.db.QueryContext(ctx, query, tenantID)
	if err != nil {
		return nil, fmt.Errorf("failed to list api tokens: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var list []*APIToken
	for rows.Next() {
		var res APIToken
		var rawScopes []byte
		if err := rows.Scan(
			&res.ID, &res.TenantID, &res.ProjectID, &res.Name, &res.TokenPrefix, &res.TokenHash,
			&res.Role, &rawScopes, &res.LastUsedAt, &res.ExpiresAt, &res.CreatedBy, &res.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan api token: %w", err)
		}
		_ = json.Unmarshal(rawScopes, &res.Scopes)
		list = append(list, &res)
	}
	if list == nil {
		list = []*APIToken{}
	}
	return list, nil
}

func (s *PostgresStore) RevokeAPIToken(ctx context.Context, tenantID, id string) error {
	query := `DELETE FROM api_tokens WHERE tenant_id = $1 AND id = $2;`
	res, err := s.db.ExecContext(ctx, query, tenantID, id)
	if err != nil {
		return fmt.Errorf("failed to revoke api token: %w", err)
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return ErrTokenNotFound
	}
	return nil
}

func (s *PostgresStore) TouchAPIToken(ctx context.Context, id string) error {
	query := `UPDATE api_tokens SET last_used_at = NOW() WHERE id = $1;`
	_, err := s.db.ExecContext(ctx, query, id)
	return err
}

// --- Audit Logs ---

func (s *PostgresStore) RecordAuditLog(ctx context.Context, log *AuditLog) error {
	if log.ID == "" {
		log.ID = uuid.NewString()
	}
	if log.CreatedAt.IsZero() {
		log.CreatedAt = time.Now().UTC()
	}

	// Scrub any sensitive secrets or tokens from details
	scrubbed := ScrubAuditDetails(log.Details)
	detailsJSON, err := json.Marshal(scrubbed)
	if err != nil {
		detailsJSON = []byte("{}")
	}

	query := `
		INSERT INTO audit_logs (id, tenant_id, actor_id, actor_type, action, resource_type, resource_id, details, ip_address, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10);
	`
	_, err = s.db.ExecContext(ctx, query,
		log.ID, log.TenantID, log.ActorID, log.ActorType, log.Action,
		log.ResourceType, log.ResourceID, detailsJSON, log.IPAddress, log.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to record audit log: %w", err)
	}
	return nil
}

func (s *PostgresStore) ListAuditLogs(ctx context.Context, tenantID string, filter AuditFilter) ([]*AuditLog, error) {
	query := `
		SELECT id, tenant_id, actor_id, actor_type, action, resource_type, resource_id, details, ip_address, created_at
		FROM audit_logs
		WHERE tenant_id = $1
		  AND ($2 = '' OR resource_type = $2)
		  AND ($3 = '' OR resource_id = $3)
		  AND ($4 = '' OR actor_id = $4)
		  AND ($5 = '' OR action = $5)
		ORDER BY created_at DESC
		LIMIT $6 OFFSET $7;
	`
	limit := filter.Limit
	if limit <= 0 || limit > 100 {
		limit = 50
	}

	rows, err := s.db.QueryContext(ctx, query,
		tenantID, filter.ResourceType, filter.ResourceID, filter.ActorID, filter.Action,
		limit, filter.Offset,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to list audit logs: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var list []*AuditLog
	for rows.Next() {
		var l AuditLog
		var rawDetails []byte
		if err := rows.Scan(
			&l.ID, &l.TenantID, &l.ActorID, &l.ActorType, &l.Action,
			&l.ResourceType, &l.ResourceID, &rawDetails, &l.IPAddress, &l.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan audit log: %w", err)
		}
		_ = json.Unmarshal(rawDetails, &l.Details)
		list = append(list, &l)
	}
	if list == nil {
		list = []*AuditLog{}
	}
	return list, nil
}
