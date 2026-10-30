package workflow

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
)

var ErrVersionNotFound = errors.New("workflow version not found")

// WorkflowVersion represents an immutable point-in-time snapshot of a workflow definition.
type WorkflowVersion struct {
	ID         string          `json:"id"`
	WorkflowID string          `json:"workflow_id"`
	Version    int             `json:"version"`
	Definition json.RawMessage `json:"definition"`
	CreatedAt  time.Time       `json:"created_at"`
}

// VersionStore defines persistence operations for workflow audit versions.
type VersionStore interface {
	RecordVersion(ctx context.Context, workflowID string, definition json.RawMessage) (*WorkflowVersion, error)
	ListVersions(ctx context.Context, workflowID string) ([]*WorkflowVersion, error)
	GetVersion(ctx context.Context, workflowID string, version int) (*WorkflowVersion, error)
}

// MemoryVersionStore implements an in-memory VersionStore for testing and offline runs.
type MemoryVersionStore struct {
	mu       sync.RWMutex
	versions map[string][]*WorkflowVersion
}

// NewMemoryVersionStore creates a new in-memory version store.
func NewMemoryVersionStore() *MemoryVersionStore {
	return &MemoryVersionStore{
		versions: make(map[string][]*WorkflowVersion),
	}
}

func (s *MemoryVersionStore) RecordVersion(ctx context.Context, workflowID string, definition json.RawMessage) (*WorkflowVersion, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	history := s.versions[workflowID]
	nextVer := len(history) + 1

	v := &WorkflowVersion{
		ID:         uuid.NewString(),
		WorkflowID: workflowID,
		Version:    nextVer,
		Definition: definition,
		CreatedAt:  time.Now().UTC(),
	}

	s.versions[workflowID] = append(s.versions[workflowID], v)
	return v, nil
}

func (s *MemoryVersionStore) ListVersions(ctx context.Context, workflowID string) ([]*WorkflowVersion, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	history, exists := s.versions[workflowID]
	if !exists {
		return []*WorkflowVersion{}, nil
	}

	res := make([]*WorkflowVersion, len(history))
	for i, v := range history {
		res[len(history)-1-i] = v
	}
	return res, nil
}

func (s *MemoryVersionStore) GetVersion(ctx context.Context, workflowID string, version int) (*WorkflowVersion, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	history := s.versions[workflowID]
	for _, v := range history {
		if v.Version == version {
			return v, nil
		}
	}
	return nil, ErrVersionNotFound
}

// PostgresVersionStore persists workflow versions to PostgreSQL.
type PostgresVersionStore struct {
	db *sql.DB
}

// NewPostgresVersionStore creates a new Postgres version store.
func NewPostgresVersionStore(db *sql.DB) *PostgresVersionStore {
	return &PostgresVersionStore{db: db}
}

func (s *PostgresVersionStore) RecordVersion(ctx context.Context, workflowID string, definition json.RawMessage) (*WorkflowVersion, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var nextVer int
	queryVer := `SELECT COALESCE(MAX(version), 0) + 1 FROM workflow_versions WHERE workflow_id = $1`
	if err := tx.QueryRowContext(ctx, queryVer, workflowID).Scan(&nextVer); err != nil {
		return nil, fmt.Errorf("failed to determine next version: %w", err)
	}

	v := &WorkflowVersion{
		ID:         uuid.NewString(),
		WorkflowID: workflowID,
		Version:    nextVer,
		Definition: definition,
		CreatedAt:  time.Now().UTC(),
	}

	insertQuery := `
		INSERT INTO workflow_versions (id, workflow_id, version, definition, created_at)
		VALUES ($1, $2, $3, $4, $5)
	`
	if _, err := tx.ExecContext(ctx, insertQuery, v.ID, v.WorkflowID, v.Version, v.Definition, v.CreatedAt); err != nil {
		return nil, fmt.Errorf("failed to insert workflow version: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit version: %w", err)
	}

	return v, nil
}

func (s *PostgresVersionStore) ListVersions(ctx context.Context, workflowID string) ([]*WorkflowVersion, error) {
	query := `
		SELECT id, workflow_id, version, definition, created_at
		FROM workflow_versions
		WHERE workflow_id = $1
		ORDER BY version ASC
	`
	rows, err := s.db.QueryContext(ctx, query, workflowID)
	if err != nil {
		return nil, fmt.Errorf("failed to query workflow versions: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var versions []*WorkflowVersion
	for rows.Next() {
		var v WorkflowVersion
		if err := rows.Scan(&v.ID, &v.WorkflowID, &v.Version, &v.Definition, &v.CreatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan workflow version: %w", err)
		}
		versions = append(versions, &v)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error reading workflow versions: %w", err)
	}
	return versions, nil
}

func (s *PostgresVersionStore) GetVersion(ctx context.Context, workflowID string, version int) (*WorkflowVersion, error) {
	query := `
		SELECT id, workflow_id, version, definition, created_at
		FROM workflow_versions
		WHERE workflow_id = $1 AND version = $2
	`
	var v WorkflowVersion
	err := s.db.QueryRowContext(ctx, query, workflowID, version).Scan(
		&v.ID, &v.WorkflowID, &v.Version, &v.Definition, &v.CreatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrVersionNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query workflow version: %w", err)
	}
	return &v, nil
}
