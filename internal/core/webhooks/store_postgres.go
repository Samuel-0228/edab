package webhooks

import (
	"context"
	"database/sql"
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

func (s *PostgresStore) Create(ctx context.Context, wh *Webhook) (*Webhook, error) {
	if wh.Name == "" {
		return nil, fmt.Errorf("webhook name is required")
	}
	if wh.ID == "" {
		wh.ID = uuid.NewString()
	}
	if wh.RateLimitPerMinute <= 0 {
		wh.RateLimitPerMinute = 60
	}
	now := time.Now().UTC()
	wh.CreatedAt = now
	wh.UpdatedAt = now

	query := `
		INSERT INTO webhooks (id, name, secret_token, rate_limit_per_minute, enabled, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`
	_, err := s.db.ExecContext(ctx, query,
		wh.ID,
		wh.Name,
		wh.SecretToken,
		wh.RateLimitPerMinute,
		wh.Enabled,
		wh.CreatedAt,
		wh.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to insert webhook: %w", err)
	}

	return wh, nil
}

func (s *PostgresStore) GetByID(ctx context.Context, id string) (*Webhook, error) {
	query := `
		SELECT id, name, secret_token, rate_limit_per_minute, enabled, created_at, updated_at
		FROM webhooks
		WHERE id = $1
	`
	var wh Webhook
	err := s.db.QueryRowContext(ctx, query, id).Scan(
		&wh.ID,
		&wh.Name,
		&wh.SecretToken,
		&wh.RateLimitPerMinute,
		&wh.Enabled,
		&wh.CreatedAt,
		&wh.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrWebhookNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get webhook: %w", err)
	}
	return &wh, nil
}

func (s *PostgresStore) List(ctx context.Context) ([]*Webhook, error) {
	query := `
		SELECT id, name, secret_token, rate_limit_per_minute, enabled, created_at, updated_at
		FROM webhooks
		ORDER BY created_at DESC
	`
	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to list webhooks: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var list []*Webhook
	for rows.Next() {
		var wh Webhook
		if err := rows.Scan(
			&wh.ID,
			&wh.Name,
			&wh.SecretToken,
			&wh.RateLimitPerMinute,
			&wh.Enabled,
			&wh.CreatedAt,
			&wh.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan webhook row: %w", err)
		}
		list = append(list, &wh)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if list == nil {
		list = []*Webhook{}
	}
	return list, nil
}

func (s *PostgresStore) Update(ctx context.Context, wh *Webhook) (*Webhook, error) {
	wh.UpdatedAt = time.Now().UTC()
	query := `
		UPDATE webhooks
		SET name = $1, secret_token = $2, rate_limit_per_minute = $3, enabled = $4, updated_at = $5
		WHERE id = $6
	`
	res, err := s.db.ExecContext(ctx, query,
		wh.Name,
		wh.SecretToken,
		wh.RateLimitPerMinute,
		wh.Enabled,
		wh.UpdatedAt,
		wh.ID,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to update webhook: %w", err)
	}
	rowsAffected, _ := res.RowsAffected()
	if rowsAffected == 0 {
		return nil, ErrWebhookNotFound
	}
	return wh, nil
}

func (s *PostgresStore) Delete(ctx context.Context, id string) error {
	query := `DELETE FROM webhooks WHERE id = $1`
	res, err := s.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete webhook: %w", err)
	}
	rowsAffected, _ := res.RowsAffected()
	if rowsAffected == 0 {
		return ErrWebhookNotFound
	}
	return nil
}
