package sources

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

type PostgresStore struct {
	db *sql.DB
}

func NewPostgresStore(db *sql.DB) *PostgresStore {
	return &PostgresStore{db: db}
}

func (s *PostgresStore) Create(ctx context.Context, src *Source) (*Source, error) {
	if src.ID == "" {
		src.ID = uuid.NewString()
	}
	now := time.Now().UTC()
	src.CreatedAt = now
	src.UpdatedAt = now

	if len(src.Config) == 0 {
		src.Config = json.RawMessage("{}")
	}

	query := `
		INSERT INTO sources (id, name, adapter_type, config, enabled, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, name, adapter_type, config, enabled, created_at, updated_at;
	`
	var created Source
	err := s.db.QueryRowContext(
		ctx,
		query,
		src.ID,
		src.Name,
		src.AdapterType,
		src.Config,
		src.Enabled,
		src.CreatedAt,
		src.UpdatedAt,
	).Scan(
		&created.ID,
		&created.Name,
		&created.AdapterType,
		&created.Config,
		&created.Enabled,
		&created.CreatedAt,
		&created.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create source: %w", err)
	}
	return &created, nil
}

func (s *PostgresStore) GetByID(ctx context.Context, id string) (*Source, error) {
	query := `
		SELECT id, name, adapter_type, config, enabled, created_at, updated_at
		FROM sources
		WHERE id = $1;
	`
	var src Source
	err := s.db.QueryRowContext(ctx, query, id).Scan(
		&src.ID,
		&src.Name,
		&src.AdapterType,
		&src.Config,
		&src.Enabled,
		&src.CreatedAt,
		&src.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrSourceNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get source: %w", err)
	}
	return &src, nil
}

func (s *PostgresStore) Update(ctx context.Context, src *Source) (*Source, error) {
	src.UpdatedAt = time.Now().UTC()
	if len(src.Config) == 0 {
		src.Config = json.RawMessage("{}")
	}

	query := `
		UPDATE sources
		SET name = $2, adapter_type = $3, config = $4, enabled = $5, updated_at = $6
		WHERE id = $1
		RETURNING id, name, adapter_type, config, enabled, created_at, updated_at;
	`
	var updated Source
	err := s.db.QueryRowContext(
		ctx,
		query,
		src.ID,
		src.Name,
		src.AdapterType,
		src.Config,
		src.Enabled,
		src.UpdatedAt,
	).Scan(
		&updated.ID,
		&updated.Name,
		&updated.AdapterType,
		&updated.Config,
		&updated.Enabled,
		&updated.CreatedAt,
		&updated.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrSourceNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to update source: %w", err)
	}
	return &updated, nil
}

func (s *PostgresStore) Delete(ctx context.Context, id string) error {
	query := `DELETE FROM sources WHERE id = $1;`
	res, err := s.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete source: %w", err)
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to check affected rows: %w", err)
	}
	if rows == 0 {
		return ErrSourceNotFound
	}
	return nil
}

func (s *PostgresStore) List(ctx context.Context) ([]*Source, error) {
	query := `
		SELECT id, name, adapter_type, config, enabled, created_at, updated_at
		FROM sources
		ORDER BY created_at DESC;
	`
	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to list sources: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var list []*Source
	for rows.Next() {
		var src Source
		if err := rows.Scan(
			&src.ID,
			&src.Name,
			&src.AdapterType,
			&src.Config,
			&src.Enabled,
			&src.CreatedAt,
			&src.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan source row: %w", err)
		}
		list = append(list, &src)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}
	return list, nil
}
