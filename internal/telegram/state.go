package telegram

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"sync"
)

// StateStore persists listener progress so that a restart resumes from the
// last processed Telegram update instead of replaying (or losing) updates.
type StateStore interface {
	GetLastUpdateID(ctx context.Context) (int, error)
	SetLastUpdateID(ctx context.Context, id int) error
}

// PostgresStateStore stores the listener offset in the database.
type PostgresStateStore struct {
	db *sql.DB
}

// NewPostgresStateStore creates a PostgreSQL-backed state store.
func NewPostgresStateStore(db *sql.DB) *PostgresStateStore {
	return &PostgresStateStore{db: db}
}

// GetLastUpdateID returns the last confirmed Telegram update id (0 if none).
func (s *PostgresStateStore) GetLastUpdateID(ctx context.Context) (int, error) {
	var last int64
	err := s.db.QueryRowContext(ctx, `SELECT last_update_id FROM telegram_update_state WHERE id = 1;`).Scan(&last)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("failed to read telegram update state: %w", err)
	}
	return int(last), nil
}

// SetLastUpdateID persists the last confirmed Telegram update id.
func (s *PostgresStateStore) SetLastUpdateID(ctx context.Context, id int) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO telegram_update_state (id, last_update_id, updated_at)
		VALUES (1, $1, NOW())
		ON CONFLICT (id) DO UPDATE SET last_update_id = EXCLUDED.last_update_id, updated_at = NOW();
	`, id)
	if err != nil {
		return fmt.Errorf("failed to persist telegram update state: %w", err)
	}
	return nil
}

// PublicChannelStateStore persists the last scraped message ID for public channels.
type PublicChannelStateStore interface {
	GetLastMessageID(ctx context.Context, channel string) (int, error)
	SetLastMessageID(ctx context.Context, channel string, id int) error
}

// GetLastMessageID returns the last processed message id for a public channel (0 if none).
func (s *PostgresStateStore) GetLastMessageID(ctx context.Context, channel string) (int, error) {
	var last int
	err := s.db.QueryRowContext(ctx, `SELECT last_message_id FROM public_channel_state WHERE channel = $1;`, strings.ToLower(channel)).Scan(&last)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("failed to read public channel state for %s: %w", channel, err)
	}
	return last, nil
}

// SetLastMessageID persists the last processed message id for a public channel.
func (s *PostgresStateStore) SetLastMessageID(ctx context.Context, channel string, id int) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO public_channel_state (channel, last_message_id, updated_at)
		VALUES ($1, $2, NOW())
		ON CONFLICT (channel) DO UPDATE SET last_message_id = EXCLUDED.last_message_id, updated_at = NOW();
	`, strings.ToLower(channel), id)
	if err != nil {
		return fmt.Errorf("failed to persist public channel state for %s: %w", channel, err)
	}
	return nil
}

// MemoryStateStore is an in-memory StateStore used by tests.
type MemoryStateStore struct {
	mu           sync.Mutex
	id           int
	channelPosts map[string]int
}

// NewMemoryStateStore creates an empty in-memory state store.
func NewMemoryStateStore() *MemoryStateStore {
	return &MemoryStateStore{
		channelPosts: make(map[string]int),
	}
}

func (m *MemoryStateStore) GetLastUpdateID(ctx context.Context) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.id, nil
}

func (m *MemoryStateStore) SetLastUpdateID(ctx context.Context, id int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.id = id
	return nil
}

func (m *MemoryStateStore) GetLastMessageID(ctx context.Context, channel string) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.channelPosts[strings.ToLower(channel)], nil
}

func (m *MemoryStateStore) SetLastMessageID(ctx context.Context, channel string, id int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.channelPosts == nil {
		m.channelPosts = make(map[string]int)
	}
	m.channelPosts[strings.ToLower(channel)] = id
	return nil
}
