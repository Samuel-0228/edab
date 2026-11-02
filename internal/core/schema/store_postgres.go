package schema

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// PostgresStore implements schema.Store on top of PostgreSQL.
type PostgresStore struct {
	db *sql.DB
}

// NewPostgresStore creates a new PostgresStore.
func NewPostgresStore(db *sql.DB) *PostgresStore {
	return &PostgresStore{db: db}
}

func (s *PostgresStore) Register(ctx context.Context, sc *Schema) (*Schema, error) {
	if sc.Name == "" {
		return nil, fmt.Errorf("schema name is required")
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	// Auto-increment version if not set
	if sc.Version <= 0 {
		var maxVer sql.NullInt64
		verQuery := `SELECT MAX(version) FROM event_schemas WHERE name = $1 AND ($2 = '' OR tenant_id = $2)`
		err := tx.QueryRowContext(ctx, verQuery, sc.Name, sc.TenantID).Scan(&maxVer)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("failed to query max schema version: %w", err)
		}
		if maxVer.Valid {
			sc.Version = int(maxVer.Int64) + 1
		} else {
			sc.Version = 1
		}
	}

	if sc.ID == "" {
		sc.ID = uuid.NewString()
	}
	if sc.Type == "" {
		sc.Type = TypeJSONSchema
	}
	if sc.Compatibility == "" {
		sc.Compatibility = CompatibilityBackward
	}
	sc.CreatedAt = time.Now().UTC()

	insertQuery := `
		INSERT INTO event_schemas (id, tenant_id, name, version, schema_type, definition, compatibility, description, created_at)
		VALUES ($1, NULLIF($2, ''), $3, $4, $5, $6, $7, $8, $9)
		RETURNING id, COALESCE(tenant_id, ''), name, version, schema_type, definition, compatibility, COALESCE(description, ''), created_at;
	`

	var created Schema
	err = tx.QueryRowContext(ctx, insertQuery,
		sc.ID, sc.TenantID, sc.Name, sc.Version, string(sc.Type),
		sc.Definition, string(sc.Compatibility), sc.Description, sc.CreatedAt,
	).Scan(
		&created.ID, &created.TenantID, &created.Name, &created.Version,
		&created.Type, &created.Definition, &created.Compatibility, &created.Description,
		&created.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to insert schema: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return &created, nil
}

func (s *PostgresStore) GetByID(ctx context.Context, id string) (*Schema, error) {
	query := `
		SELECT id, COALESCE(tenant_id, ''), name, version, schema_type, definition, compatibility, COALESCE(description, ''), created_at
		FROM event_schemas
		WHERE id = $1;
	`
	var sc Schema
	err := s.db.QueryRowContext(ctx, query, id).Scan(
		&sc.ID, &sc.TenantID, &sc.Name, &sc.Version,
		&sc.Type, &sc.Definition, &sc.Compatibility, &sc.Description,
		&sc.CreatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrSchemaNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query schema by id: %w", err)
	}
	return &sc, nil
}

func (s *PostgresStore) GetLatest(ctx context.Context, tenantID, name string) (*Schema, error) {
	query := `
		SELECT id, COALESCE(tenant_id, ''), name, version, schema_type, definition, compatibility, COALESCE(description, ''), created_at
		FROM event_schemas
		WHERE name = $1 AND ($2 = '' OR tenant_id IS NULL OR tenant_id = $2)
		ORDER BY version DESC
		LIMIT 1;
	`
	var sc Schema
	err := s.db.QueryRowContext(ctx, query, name, tenantID).Scan(
		&sc.ID, &sc.TenantID, &sc.Name, &sc.Version,
		&sc.Type, &sc.Definition, &sc.Compatibility, &sc.Description,
		&sc.CreatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrSchemaNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query latest schema: %w", err)
	}
	return &sc, nil
}

func (s *PostgresStore) GetByVersion(ctx context.Context, tenantID, name string, version int) (*Schema, error) {
	query := `
		SELECT id, COALESCE(tenant_id, ''), name, version, schema_type, definition, compatibility, COALESCE(description, ''), created_at
		FROM event_schemas
		WHERE name = $1 AND version = $2 AND ($3 = '' OR tenant_id IS NULL OR tenant_id = $3)
		LIMIT 1;
	`
	var sc Schema
	err := s.db.QueryRowContext(ctx, query, name, version, tenantID).Scan(
		&sc.ID, &sc.TenantID, &sc.Name, &sc.Version,
		&sc.Type, &sc.Definition, &sc.Compatibility, &sc.Description,
		&sc.CreatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrSchemaNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query schema by version: %w", err)
	}
	return &sc, nil
}

func (s *PostgresStore) List(ctx context.Context, tenantID string) ([]*Schema, error) {
	query := `
		SELECT DISTINCT ON (name) id, COALESCE(tenant_id, ''), name, version, schema_type, definition, compatibility, COALESCE(description, ''), created_at
		FROM event_schemas
		WHERE ($1 = '' OR tenant_id IS NULL OR tenant_id = $1)
		ORDER BY name, version DESC;
	`
	rows, err := s.db.QueryContext(ctx, query, tenantID)
	if err != nil {
		return nil, fmt.Errorf("failed to list schemas: %w", err)
	}
	defer rows.Close()

	var result []*Schema
	for rows.Next() {
		var sc Schema
		if err := rows.Scan(
			&sc.ID, &sc.TenantID, &sc.Name, &sc.Version,
			&sc.Type, &sc.Definition, &sc.Compatibility, &sc.Description,
			&sc.CreatedAt,
		); err != nil {
			return nil, err
		}
		result = append(result, &sc)
	}
	return result, nil
}

func (s *PostgresStore) ListVersions(ctx context.Context, tenantID, name string) ([]*Schema, error) {
	query := `
		SELECT id, COALESCE(tenant_id, ''), name, version, schema_type, definition, compatibility, COALESCE(description, ''), created_at
		FROM event_schemas
		WHERE name = $1 AND ($2 = '' OR tenant_id IS NULL OR tenant_id = $2)
		ORDER BY version ASC;
	`
	rows, err := s.db.QueryContext(ctx, query, name, tenantID)
	if err != nil {
		return nil, fmt.Errorf("failed to list schema versions: %w", err)
	}
	defer rows.Close()

	var result []*Schema
	for rows.Next() {
		var sc Schema
		if err := rows.Scan(
			&sc.ID, &sc.TenantID, &sc.Name, &sc.Version,
			&sc.Type, &sc.Definition, &sc.Compatibility, &sc.Description,
			&sc.CreatedAt,
		); err != nil {
			return nil, err
		}
		result = append(result, &sc)
	}
	return result, nil
}

func (s *PostgresStore) Delete(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM event_schemas WHERE id = $1`, id)
	if err != nil {
		return err
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return ErrSchemaNotFound
	}
	return nil
}
