package auth

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
)

// MemoryStore provides a thread-safe in-memory store for multi-tenancy, RBAC, API tokens, and audit logging.
type MemoryStore struct {
	mu sync.RWMutex

	tenants       map[string]*Tenant
	tenantsBySlug map[string]string

	users        map[string]*User
	usersByEmail map[string]string

	memberships     map[string]*Membership // key: "tenant_id:user_id"
	projects        map[string]*Project    // key: "tenant_id:project_id"
	apiTokens       map[string]*APIToken   // key: token_id
	tokensByHash    map[string]string      // key: token_hash -> token_id
	auditLogs       []*AuditLog
	latestAuditSeq  map[string]int64
	latestAuditHash map[string]string
}

// NewMemoryStore initializes a new MemoryStore pre-seeded with default tenant, user, and membership.
func NewMemoryStore() *MemoryStore {
	now := time.Now().UTC()
	s := &MemoryStore{
		tenants:         make(map[string]*Tenant),
		tenantsBySlug:   make(map[string]string),
		users:           make(map[string]*User),
		usersByEmail:    make(map[string]string),
		memberships:     make(map[string]*Membership),
		projects:        make(map[string]*Project),
		apiTokens:       make(map[string]*APIToken),
		tokensByHash:    make(map[string]string),
		auditLogs:       make([]*AuditLog, 0),
		latestAuditSeq:  make(map[string]int64),
		latestAuditHash: make(map[string]string),
	}

	// Seed default single-tenant setup
	defaultTenant := &Tenant{
		ID:        "default",
		Name:      "Default Organization",
		Slug:      "default",
		CreatedAt: now,
		UpdatedAt: now,
	}
	s.tenants["default"] = defaultTenant
	s.tenantsBySlug["default"] = "default"

	defaultUser := &User{
		ID:        "default-user",
		Email:     "admin@local",
		Name:      "Default Admin",
		IsSystem:  true,
		CreatedAt: now,
		UpdatedAt: now,
	}
	s.users["default-user"] = defaultUser
	s.usersByEmail["admin@local"] = "default-user"

	defaultMembership := &Membership{
		ID:        "default-membership",
		TenantID:  "default",
		UserID:    "default-user",
		Role:      RoleOwner,
		UserEmail: "admin@local",
		UserName:  "Default Admin",
		CreatedAt: now,
		UpdatedAt: now,
	}
	s.memberships["default:default-user"] = defaultMembership

	defaultProject := &Project{
		ID:          "default",
		TenantID:    "default",
		Name:        "Default Project",
		Description: "Default single-tenant project",
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	s.projects["default:default"] = defaultProject

	return s
}

// --- Tenants ---

func (s *MemoryStore) CreateTenant(ctx context.Context, tenant *Tenant) (*Tenant, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if tenant.Slug == "" {
		tenant.Slug = tenant.Name
	}
	if _, exists := s.tenantsBySlug[tenant.Slug]; exists {
		return nil, ErrTenantSlugExists
	}
	if tenant.ID == "" {
		tenant.ID = uuid.NewString()
	}
	now := time.Now().UTC()
	tenant.CreatedAt = now
	tenant.UpdatedAt = now

	cloned := *tenant
	s.tenants[tenant.ID] = &cloned
	s.tenantsBySlug[tenant.Slug] = tenant.ID

	result := cloned
	return &result, nil
}

func (s *MemoryStore) GetTenant(ctx context.Context, id string) (*Tenant, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	t, ok := s.tenants[id]
	if !ok {
		return nil, ErrTenantNotFound
	}
	cloned := *t
	return &cloned, nil
}

func (s *MemoryStore) GetTenantBySlug(ctx context.Context, slug string) (*Tenant, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	id, ok := s.tenantsBySlug[slug]
	if !ok {
		return nil, ErrTenantNotFound
	}
	t := s.tenants[id]
	cloned := *t
	return &cloned, nil
}

func (s *MemoryStore) ListTenants(ctx context.Context) ([]*Tenant, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var list []*Tenant
	for _, t := range s.tenants {
		cloned := *t
		list = append(list, &cloned)
	}
	return list, nil
}

func (s *MemoryStore) DeleteTenant(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	t, ok := s.tenants[id]
	if !ok {
		return ErrTenantNotFound
	}
	delete(s.tenantsBySlug, t.Slug)
	delete(s.tenants, id)
	return nil
}

// --- Users & Memberships ---

func (s *MemoryStore) CreateUser(ctx context.Context, user *User) (*User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.usersByEmail[user.Email]; exists {
		return nil, ErrUserEmailExists
	}
	if user.ID == "" {
		user.ID = uuid.NewString()
	}
	now := time.Now().UTC()
	user.CreatedAt = now
	user.UpdatedAt = now

	cloned := *user
	s.users[user.ID] = &cloned
	s.usersByEmail[user.Email] = user.ID

	result := cloned
	return &result, nil
}

func (s *MemoryStore) GetUser(ctx context.Context, id string) (*User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	u, ok := s.users[id]
	if !ok {
		return nil, ErrUserNotFound
	}
	cloned := *u
	return &cloned, nil
}

func (s *MemoryStore) GetUserByEmail(ctx context.Context, email string) (*User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	id, ok := s.usersByEmail[email]
	if !ok {
		return nil, ErrUserNotFound
	}
	u := s.users[id]
	cloned := *u
	return &cloned, nil
}

func (s *MemoryStore) CreateMembership(ctx context.Context, m *Membership) (*Membership, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.tenants[m.TenantID]; !ok {
		return nil, ErrTenantNotFound
	}
	u, ok := s.users[m.UserID]
	if !ok {
		return nil, ErrUserNotFound
	}

	key := fmt.Sprintf("%s:%s", m.TenantID, m.UserID)
	if m.ID == "" {
		m.ID = uuid.NewString()
	}
	now := time.Now().UTC()
	m.CreatedAt = now
	m.UpdatedAt = now
	m.UserEmail = u.Email
	m.UserName = u.Name

	cloned := *m
	s.memberships[key] = &cloned

	result := cloned
	return &result, nil
}

func (s *MemoryStore) GetMembership(ctx context.Context, tenantID, userID string) (*Membership, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	key := fmt.Sprintf("%s:%s", tenantID, userID)
	m, ok := s.memberships[key]
	if !ok {
		return nil, ErrMembershipNotFound
	}
	cloned := *m
	return &cloned, nil
}

func (s *MemoryStore) ListMemberships(ctx context.Context, tenantID string) ([]*Membership, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var list []*Membership
	for _, m := range s.memberships {
		if m.TenantID == tenantID {
			cloned := *m
			list = append(list, &cloned)
		}
	}
	return list, nil
}

func (s *MemoryStore) UpdateMembershipRole(ctx context.Context, tenantID, userID string, role Role) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := fmt.Sprintf("%s:%s", tenantID, userID)
	m, ok := s.memberships[key]
	if !ok {
		return ErrMembershipNotFound
	}
	m.Role = role
	m.UpdatedAt = time.Now().UTC()
	return nil
}

func (s *MemoryStore) DeleteMembership(ctx context.Context, tenantID, userID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := fmt.Sprintf("%s:%s", tenantID, userID)
	if _, ok := s.memberships[key]; !ok {
		return ErrMembershipNotFound
	}
	delete(s.memberships, key)
	return nil
}

// --- Projects ---

func (s *MemoryStore) CreateProject(ctx context.Context, p *Project) (*Project, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.tenants[p.TenantID]; !ok {
		return nil, ErrTenantNotFound
	}
	if p.ID == "" {
		p.ID = uuid.NewString()
	}
	now := time.Now().UTC()
	p.CreatedAt = now
	p.UpdatedAt = now

	key := fmt.Sprintf("%s:%s", p.TenantID, p.ID)
	cloned := *p
	s.projects[key] = &cloned

	result := cloned
	return &result, nil
}

func (s *MemoryStore) GetProject(ctx context.Context, tenantID, id string) (*Project, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	key := fmt.Sprintf("%s:%s", tenantID, id)
	p, ok := s.projects[key]
	if !ok {
		return nil, ErrProjectNotFound
	}
	cloned := *p
	return &cloned, nil
}

func (s *MemoryStore) ListProjects(ctx context.Context, tenantID string) ([]*Project, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var list []*Project
	for _, p := range s.projects {
		if p.TenantID == tenantID {
			cloned := *p
			list = append(list, &cloned)
		}
	}
	return list, nil
}

func (s *MemoryStore) DeleteProject(ctx context.Context, tenantID, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := fmt.Sprintf("%s:%s", tenantID, id)
	if _, ok := s.projects[key]; !ok {
		return ErrProjectNotFound
	}
	delete(s.projects, key)
	return nil
}

// --- API Tokens ---

func (s *MemoryStore) CreateAPIToken(ctx context.Context, token *APIToken) (*APIToken, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.tenants[token.TenantID]; !ok {
		return nil, ErrTenantNotFound
	}
	if token.ID == "" {
		token.ID = uuid.NewString()
	}
	token.CreatedAt = time.Now().UTC()

	cloned := *token
	s.apiTokens[token.ID] = &cloned
	s.tokensByHash[token.TokenHash] = token.ID

	result := cloned
	return &result, nil
}

func (s *MemoryStore) GetAPITokenByHash(ctx context.Context, tokenHash string) (*APIToken, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	id, ok := s.tokensByHash[tokenHash]
	if !ok {
		return nil, ErrTokenNotFound
	}
	token := s.apiTokens[id]
	if token.ExpiresAt != nil && token.ExpiresAt.Before(time.Now().UTC()) {
		return nil, ErrTokenExpired
	}
	cloned := *token
	return &cloned, nil
}

func (s *MemoryStore) ListAPITokens(ctx context.Context, tenantID string) ([]*APIToken, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var list []*APIToken
	for _, tok := range s.apiTokens {
		if tok.TenantID == tenantID {
			cloned := *tok
			list = append(list, &cloned)
		}
	}
	return list, nil
}

func (s *MemoryStore) RevokeAPIToken(ctx context.Context, tenantID, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	tok, ok := s.apiTokens[id]
	if !ok || tok.TenantID != tenantID {
		return ErrTokenNotFound
	}
	delete(s.tokensByHash, tok.TokenHash)
	delete(s.apiTokens, id)
	return nil
}

func (s *MemoryStore) TouchAPIToken(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	tok, ok := s.apiTokens[id]
	if !ok {
		return ErrTokenNotFound
	}
	now := time.Now().UTC()
	tok.LastUsedAt = &now
	return nil
}

// --- Audit Logs ---

func (s *MemoryStore) RecordAuditLog(ctx context.Context, log *AuditLog) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if log.ID == "" {
		log.ID = uuid.NewString()
	}
	if log.CreatedAt.IsZero() {
		log.CreatedAt = time.Now().UTC()
	}
	if log.TenantID == "" {
		log.TenantID = "default"
	}

	// Always scrub details before saving to prevent leak of secrets or credentials
	log.Details = ScrubAuditDetails(log.Details)

	if log.SequenceNum <= 0 {
		s.latestAuditSeq[log.TenantID]++
		log.SequenceNum = s.latestAuditSeq[log.TenantID]

		prevHash := s.latestAuditHash[log.TenantID]
		if prevHash == "" {
			prevHash = "0000000000000000000000000000000000000000000000000000000000000000"
		}
		log.PrevHash = prevHash
		log.EntryHash = computeAuditEntryHash(log)
		s.latestAuditHash[log.TenantID] = log.EntryHash
	}

	cloned := *log
	s.auditLogs = append(s.auditLogs, &cloned)
	return nil
}

func (s *MemoryStore) ListAuditLogs(ctx context.Context, tenantID string, filter AuditFilter) ([]*AuditLog, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []*AuditLog
	for i := len(s.auditLogs) - 1; i >= 0; i-- {
		l := s.auditLogs[i]
		if l.TenantID != tenantID {
			continue
		}
		if filter.ResourceType != "" && l.ResourceType != filter.ResourceType {
			continue
		}
		if filter.ResourceID != "" && l.ResourceID != filter.ResourceID {
			continue
		}
		if filter.ActorID != "" && l.ActorID != filter.ActorID {
			continue
		}
		if filter.Action != "" && l.Action != filter.Action {
			continue
		}
		if !filter.From.IsZero() && l.CreatedAt.Before(filter.From) {
			continue
		}
		if !filter.To.IsZero() && l.CreatedAt.After(filter.To) {
			continue
		}
		cloned := *l
		result = append(result, &cloned)
	}

	if filter.Offset > 0 {
		if filter.Offset >= len(result) {
			return []*AuditLog{}, nil
		}
		result = result[filter.Offset:]
	}
	if filter.Limit > 0 && len(result) > filter.Limit {
		result = result[:filter.Limit]
	}

	return result, nil
}

func (s *MemoryStore) PruneAuditLogs(_ context.Context, tenantID string, before time.Time) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var kept []*AuditLog
	var pruned int64
	for _, l := range s.auditLogs {
		if (tenantID == "" || l.TenantID == tenantID) && l.CreatedAt.Before(before) {
			pruned++
		} else {
			kept = append(kept, l)
		}
	}
	s.auditLogs = kept
	return pruned, nil
}
