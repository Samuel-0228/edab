package ai

import (
	"context"
	"errors"
)

var (
	ErrProposalNotFound = errors.New("ai proposal not found")
	ErrProposalDecided  = errors.New("ai proposal has already been decided")
	ErrAIForbidden      = errors.New("ai assistance is disabled or tenant has opted out")
)

// Store defines persistent storage operations for AI proposals and decision tracking.
type Store interface {
	CreateProposal(ctx context.Context, p *Proposal) (*Proposal, error)
	GetProposal(ctx context.Context, tenantID, id string) (*Proposal, error)
	ListProposals(ctx context.Context, tenantID string, limit, offset int) ([]*Proposal, error)
	UpdateProposalStatus(ctx context.Context, tenantID, id string, status ProposalStatus, decidedBy string) (*Proposal, error)
}
