package schema

import (
	"errors"
	"time"
)

var (
	ErrSchemaNotFound     = errors.New("schema not found")
	ErrSchemaConflict     = errors.New("schema version already exists")
	ErrIncompatibleSchema = errors.New("new schema is incompatible with existing version")
	ErrValidationFailed   = errors.New("payload failed schema validation")
	ErrInvalidSchemaType  = errors.New("unsupported schema type")
	ErrInvalidDefinition  = errors.New("invalid schema definition syntax")
)

// SchemaType defines supported schema serialization formats.
type SchemaType string

const (
	TypeJSONSchema SchemaType = "json_schema"
	TypeProtobuf   SchemaType = "protobuf"
)

// CompatibilityMode defines version evolution rules.
type CompatibilityMode string

const (
	// Backward: Consumers with new schema can process data produced with old schema.
	CompatibilityBackward CompatibilityMode = "BACKWARD"
	// Forward: Consumers with old schema can process data produced with new schema.
	CompatibilityForward CompatibilityMode = "FORWARD"
	// Full: Both Backward and Forward compatible.
	CompatibilityFull CompatibilityMode = "FULL"
	// None: No compatibility checks enforced.
	CompatibilityNone CompatibilityMode = "NONE"
)

// Schema represents a registered event schema definition.
type Schema struct {
	ID            string            `json:"id"`
	TenantID      string            `json:"tenant_id,omitempty"`
	Name          string            `json:"name"` // e.g. "github.issue.opened", "payment.processed"
	Version       int               `json:"version"`
	Type          SchemaType        `json:"schema_type"`
	Definition    string            `json:"definition"`
	Compatibility CompatibilityMode `json:"compatibility"`
	Description   string            `json:"description,omitempty"`
	CreatedAt     time.Time         `json:"created_at"`
}

// ValidationError detail represents a specific schema violation.
type ValidationError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
	Rule    string `json:"rule,omitempty"`
}

func (e ValidationError) Error() string {
	return e.Field + ": " + e.Message
}
