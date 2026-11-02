package workflow

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrApprovalNotFound      = errors.New("approval not found")
	ErrApprovalAlreadyClosed = errors.New("approval has already been decided or expired")
	ErrApprovalUnauthorized  = errors.New("user is not authorized to decide on this approval")
	ErrApprovalTokenInvalid  = errors.New("invalid approval token")
)

// ApprovalStatus represents the lifecycle state of a human approval step.
type ApprovalStatus string

const (
	ApprovalPending   ApprovalStatus = "pending"
	ApprovalApproved  ApprovalStatus = "approved"
	ApprovalRejected  ApprovalStatus = "rejected"
	ApprovalExpired   ApprovalStatus = "expired"
	ApprovalEscalated ApprovalStatus = "escalated"
)

// ApprovalConfig defines declarative human approval requirements for an ActionStep.
type ApprovalConfig struct {
	Title             string   `json:"title"`
	Description       string   `json:"description,omitempty"`
	ApproverRoles     []string `json:"approver_roles,omitempty"`   // e.g. ["owner", "admin", "operator"]
	ApproverUsers     []string `json:"approver_users,omitempty"`   // specific user IDs or emails
	TimeoutSeconds    int      `json:"timeout_seconds,omitempty"`  // default: 86400 (24h)
	Timeout           string   `json:"timeout,omitempty"`          // e.g. "24h", "2h", "30m"
	TimeoutAction     string   `json:"timeout_action,omitempty"`   // "reject" (default), "escalate", "approve"
	EscalationRoles   []string `json:"escalation_roles,omitempty"` // roles allowed upon escalation
	EscalationContact string   `json:"escalation_contact,omitempty"`
}

// Approval represents a durable human approval record stored in persistence.
type Approval struct {
	ID                string         `json:"id"`
	TenantID          string         `json:"tenant_id,omitempty"`
	ExecutionID       string         `json:"execution_id"`
	StepID            string         `json:"step_id"`
	WorkflowID        string         `json:"workflow_id"`
	Title             string         `json:"title"`
	Description       string         `json:"description,omitempty"`
	Token             string         `json:"token,omitempty"` // Public cryptographic token
	TokenHash         string         `json:"-"`               // SHA-256 hash for secure DB lookup
	Status            ApprovalStatus `json:"status"`
	ApproverRoles     []string       `json:"approver_roles,omitempty"`
	ApproverUsers     []string       `json:"approver_users,omitempty"`
	ApprovedBy        *string        `json:"approved_by,omitempty"`
	DecisionNote      *string        `json:"decision_note,omitempty"`
	ExpiresAt         time.Time      `json:"expires_at"`
	TimeoutAction     string         `json:"timeout_action"`
	EscalationRoles   []string       `json:"escalation_roles,omitempty"`
	EscalationContact string         `json:"escalation_contact,omitempty"`
	CreatedAt         time.Time      `json:"created_at"`
	UpdatedAt         time.Time      `json:"updated_at"`
}

// GenerateApprovalToken generates a cryptographically random token and its SHA-256 hash.
func GenerateApprovalToken() (rawToken string, tokenHash string, err error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", "", fmt.Errorf("failed to generate secure random token: %w", err)
	}
	rawToken = hex.EncodeToString(bytes)
	tokenHash = HashApprovalToken(rawToken)
	return rawToken, tokenHash, nil
}

// HashApprovalToken computes the hex SHA-256 hash of an approval token.
func HashApprovalToken(token string) string {
	hash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hash[:])
}

// CanApprove checks if a given user ID and roles are authorized to decide this approval.
func (a *Approval) CanApprove(userID string, userRoles []string) bool {
	// If no restrictions are set, any authenticated user can approve
	if len(a.ApproverRoles) == 0 && len(a.ApproverUsers) == 0 {
		return true
	}

	// Check user ID match
	if userID != "" {
		for _, u := range a.ApproverUsers {
			if strings.EqualFold(u, userID) {
				return true
			}
		}
	}

	// Check roles match (including escalation roles if escalated)
	rolesToCheck := a.ApproverRoles
	if a.Status == ApprovalEscalated && len(a.EscalationRoles) > 0 {
		rolesToCheck = append(rolesToCheck, a.EscalationRoles...)
	}

	for _, ur := range userRoles {
		// Owner and admin roles always have supervisor approval permissions
		if ur == "owner" || ur == "admin" {
			return true
		}
		for _, reqRole := range rolesToCheck {
			if strings.EqualFold(ur, reqRole) {
				return true
			}
		}
	}

	return false
}

// ApprovalFilter defines filter criteria for querying approvals.
type ApprovalFilter struct {
	TenantID    string
	ExecutionID string
	WorkflowID  string
	Status      ApprovalStatus
	Limit       int
	Offset      int
}

// ApprovalStore provides persistence operations for human approvals.
type ApprovalStore interface {
	Create(ctx context.Context, app *Approval) (*Approval, error)
	GetByID(ctx context.Context, id string) (*Approval, error)
	GetByTokenHash(ctx context.Context, tokenHash string) (*Approval, error)
	GetByExecutionAndStep(ctx context.Context, executionID, stepID string) (*Approval, error)
	List(ctx context.Context, filter ApprovalFilter) ([]*Approval, error)
	UpdateDecision(ctx context.Context, id string, status ApprovalStatus, decidedBy string, note string) (*Approval, error)
	UpdateStatus(ctx context.Context, id string, status ApprovalStatus) error
	ListExpiredPending(ctx context.Context, now time.Time) ([]*Approval, error)
	Delete(ctx context.Context, id string) error
}
