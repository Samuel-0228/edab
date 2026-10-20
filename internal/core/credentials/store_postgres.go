package credentials

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
	db  *sql.DB
	key []byte
}

func NewPostgresStore(db *sql.DB, key []byte) *PostgresStore {
	return &PostgresStore{
		db:  db,
		key: key,
	}
}

func (s *PostgresStore) Create(ctx context.Context, cred *Credential, rawValues map[string]string) (*Credential, error) {
	if cred.Name == "" {
		return nil, fmt.Errorf("credential name is required")
	}
	if cred.Integration == "" {
		return nil, fmt.Errorf("credential integration is required")
	}

	if cred.ID == "" {
		cred.ID = uuid.NewString()
	}
	now := time.Now().UTC()
	cred.CreatedAt = now
	cred.UpdatedAt = now
	cred.KeyID = "default"

	cred.MaskedFields = make(map[string]string)
	for k, v := range rawValues {
		cred.MaskedFields[k] = MaskSecret(v)
	}

	rawBytes, err := json.Marshal(rawValues)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal credential data: %w", err)
	}

	ciphertext, nonce, err := Encrypt(s.key, rawBytes)
	if err != nil {
		return nil, fmt.Errorf("encryption error: %w", err)
	}
	cred.EncryptedData = ciphertext
	cred.Nonce = nonce

	maskedJSON, err := json.Marshal(cred.MaskedFields)
	if err != nil {
		return nil, err
	}

	query := `
		INSERT INTO credentials (id, name, integration, encrypted_data, nonce, key_id, masked_fields, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`
	_, err = s.db.ExecContext(ctx, query,
		cred.ID,
		cred.Name,
		cred.Integration,
		cred.EncryptedData,
		cred.Nonce,
		cred.KeyID,
		maskedJSON,
		cred.CreatedAt,
		cred.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to insert credential: %w", err)
	}

	return cred, nil
}

func (s *PostgresStore) GetByID(ctx context.Context, id string) (*Credential, error) {
	query := `
		SELECT id, name, integration, encrypted_data, nonce, key_id, masked_fields, created_at, updated_at
		FROM credentials
		WHERE id = $1
	`
	var c Credential
	var maskedJSON []byte
	err := s.db.QueryRowContext(ctx, query, id).Scan(
		&c.ID,
		&c.Name,
		&c.Integration,
		&c.EncryptedData,
		&c.Nonce,
		&c.KeyID,
		&maskedJSON,
		&c.CreatedAt,
		&c.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrCredentialNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get credential: %w", err)
	}

	if len(maskedJSON) > 0 {
		_ = json.Unmarshal(maskedJSON, &c.MaskedFields)
	}
	return &c, nil
}

func (s *PostgresStore) GetDecrypted(ctx context.Context, id string) (map[string]string, error) {
	cred, err := s.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	plaintext, err := Decrypt(s.key, cred.EncryptedData, cred.Nonce)
	if err != nil {
		return nil, err
	}

	var res map[string]string
	if err := json.Unmarshal(plaintext, &res); err != nil {
		return nil, fmt.Errorf("failed to unmarshal decrypted credentials: %w", err)
	}
	return res, nil
}

func (s *PostgresStore) List(ctx context.Context) ([]*Credential, error) {
	query := `
		SELECT id, name, integration, key_id, masked_fields, created_at, updated_at
		FROM credentials
		ORDER BY created_at DESC
	`
	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to list credentials: %w", err)
	}
	defer rows.Close()

	var list []*Credential
	for rows.Next() {
		var c Credential
		var maskedJSON []byte
		if err := rows.Scan(
			&c.ID,
			&c.Name,
			&c.Integration,
			&c.KeyID,
			&maskedJSON,
			&c.CreatedAt,
			&c.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan credential row: %w", err)
		}
		if len(maskedJSON) > 0 {
			_ = json.Unmarshal(maskedJSON, &c.MaskedFields)
		}
		list = append(list, &c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if list == nil {
		list = []*Credential{}
	}
	return list, nil
}

func (s *PostgresStore) Update(ctx context.Context, cred *Credential, rawValues map[string]string) (*Credential, error) {
	existing, err := s.GetByID(ctx, cred.ID)
	if err != nil {
		return nil, err
	}

	existing.Name = cred.Name
	existing.Integration = cred.Integration
	existing.UpdatedAt = time.Now().UTC()

	var query string
	var args []any

	if len(rawValues) > 0 {
		existing.MaskedFields = make(map[string]string)
		for k, v := range rawValues {
			existing.MaskedFields[k] = MaskSecret(v)
		}
		rawBytes, err := json.Marshal(rawValues)
		if err != nil {
			return nil, err
		}
		ciphertext, nonce, err := Encrypt(s.key, rawBytes)
		if err != nil {
			return nil, err
		}
		existing.EncryptedData = ciphertext
		existing.Nonce = nonce

		maskedJSON, err := json.Marshal(existing.MaskedFields)
		if err != nil {
			return nil, err
		}

		query = `
			UPDATE credentials
			SET name = $1, integration = $2, encrypted_data = $3, nonce = $4, masked_fields = $5, updated_at = $6
			WHERE id = $7
		`
		args = []any{existing.Name, existing.Integration, existing.EncryptedData, existing.Nonce, maskedJSON, existing.UpdatedAt, existing.ID}
	} else {
		query = `
			UPDATE credentials
			SET name = $1, integration = $2, updated_at = $3
			WHERE id = $4
		`
		args = []any{existing.Name, existing.Integration, existing.UpdatedAt, existing.ID}
	}

	res, err := s.db.ExecContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to update credential: %w", err)
	}
	rowsAffected, _ := res.RowsAffected()
	if rowsAffected == 0 {
		return nil, ErrCredentialNotFound
	}

	return existing, nil
}

func (s *PostgresStore) Delete(ctx context.Context, id string) error {
	query := `DELETE FROM credentials WHERE id = $1`
	res, err := s.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete credential: %w", err)
	}
	rowsAffected, _ := res.RowsAffected()
	if rowsAffected == 0 {
		return ErrCredentialNotFound
	}
	return nil
}
