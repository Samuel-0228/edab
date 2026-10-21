package destinations

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

func (s *PostgresStore) Create(ctx context.Context, dest *Destination) (*Destination, error) {
	if dest.ID == "" {
		dest.ID = uuid.NewString()
	}
	now := time.Now().UTC()
	dest.CreatedAt = now
	dest.UpdatedAt = now

	if len(dest.Config) == 0 {
		dest.Config = json.RawMessage("{}")
	}

	query := `
		INSERT INTO destinations (id, name, adapter_type, config, enabled, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, name, adapter_type, config, enabled, created_at, updated_at;
	`
	var created Destination
	err := s.db.QueryRowContext(
		ctx,
		query,
		dest.ID,
		dest.Name,
		dest.AdapterType,
		dest.Config,
		dest.Enabled,
		dest.CreatedAt,
		dest.UpdatedAt,
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
		return nil, fmt.Errorf("failed to create destination: %w", err)
	}
	return &created, nil
}

func (s *PostgresStore) GetByID(ctx context.Context, id string) (*Destination, error) {
	query := `
		SELECT id, name, adapter_type, config, enabled, created_at, updated_at
		FROM destinations
		WHERE id = $1;
	`
	var dest Destination
	err := s.db.QueryRowContext(ctx, query, id).Scan(
		&dest.ID,
		&dest.Name,
		&dest.AdapterType,
		&dest.Config,
		&dest.Enabled,
		&dest.CreatedAt,
		&dest.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrDestinationNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get destination: %w", err)
	}
	return &dest, nil
}

func (s *PostgresStore) Update(ctx context.Context, dest *Destination) (*Destination, error) {
	dest.UpdatedAt = time.Now().UTC()
	if len(dest.Config) == 0 {
		dest.Config = json.RawMessage("{}")
	}

	query := `
		UPDATE destinations
		SET name = $2, adapter_type = $3, config = $4, enabled = $5, updated_at = $6
		WHERE id = $1
		RETURNING id, name, adapter_type, config, enabled, created_at, updated_at;
	`
	var updated Destination
	err := s.db.QueryRowContext(
		ctx,
		query,
		dest.ID,
		dest.Name,
		dest.AdapterType,
		dest.Config,
		dest.Enabled,
		dest.UpdatedAt,
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
		return nil, ErrDestinationNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to update destination: %w", err)
	}
	return &updated, nil
}

func (s *PostgresStore) Delete(ctx context.Context, id string) error {
	query := `DELETE FROM destinations WHERE id = $1;`
	res, err := s.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete destination: %w", err)
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to check affected rows: %w", err)
	}
	if rows == 0 {
		return ErrDestinationNotFound
	}
	return nil
}

func (s *PostgresStore) List(ctx context.Context) ([]*Destination, error) {
	query := `
		SELECT id, name, adapter_type, config, enabled, created_at, updated_at
		FROM destinations
		ORDER BY created_at DESC;
	`
	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to list destinations: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var list []*Destination
	for rows.Next() {
		var dest Destination
		if err := rows.Scan(
			&dest.ID,
			&dest.Name,
			&dest.AdapterType,
			&dest.Config,
			&dest.Enabled,
			&dest.CreatedAt,
			&dest.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan destination row: %w", err)
		}
		list = append(list, &dest)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}
	return list, nil
}
