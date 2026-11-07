package ai

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
)

// MemoryStore provides an in-memory thread-safe implementation of Store.
type MemoryStore struct {
	mu        sync.RWMutex
	proposals map[string]*Proposal // key: "tenant_id:proposal_id"
}

// NewMemoryStore initializes an in-memory store for AI proposals.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		proposals: make(map[string]*Proposal),
	}
}

func (s *MemoryStore) CreateProposal(_ context.Context, p *Proposal) (*Proposal, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if p.ID == "" {
		p.ID = uuid.NewString()
	}
	if p.CreatedAt.IsZero() {
		p.CreatedAt = time.Now().UTC()
	}
	if p.Status == "" {
		p.Status = StatusPendingApproval
	}

	key := fmt.Sprintf("%s:%s", p.TenantID, p.ID)
	cloned := cloneProposal(p)
	s.proposals[key] = cloned

	return cloneProposal(cloned), nil
}

func (s *MemoryStore) GetProposal(_ context.Context, tenantID, id string) (*Proposal, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	key := fmt.Sprintf("%s:%s", tenantID, id)
	p, ok := s.proposals[key]
	if !ok {
		return nil, ErrProposalNotFound
	}
	return cloneProposal(p), nil
}

func (s *MemoryStore) ListProposals(_ context.Context, tenantID string, limit, offset int) ([]*Proposal, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var list []*Proposal
	for _, p := range s.proposals {
		if p.TenantID == tenantID {
			list = append(list, cloneProposal(p))
		}
	}

	sort.Slice(list, func(i, j int) bool {
		return list[i].CreatedAt.After(list[j].CreatedAt)
	})

	if offset > 0 {
		if offset >= len(list) {
			return []*Proposal{}, nil
		}
		list = list[offset:]
	}
	if limit > 0 && len(list) > limit {
		list = list[:limit]
	}

	return list, nil
}

func (s *MemoryStore) UpdateProposalStatus(_ context.Context, tenantID, id string, status ProposalStatus, decidedBy string) (*Proposal, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := fmt.Sprintf("%s:%s", tenantID, id)
	p, ok := s.proposals[key]
	if !ok {
		return nil, ErrProposalNotFound
	}

	if p.Status != StatusPendingApproval {
		return nil, ErrProposalDecided
	}

	now := time.Now().UTC()
	p.Status = status
	p.DecidedAt = &now
	if decidedBy != "" {
		p.DecidedBy = &decidedBy
	}

	return cloneProposal(p), nil
}

func cloneProposal(p *Proposal) *Proposal {
	if p == nil {
		return nil
	}
	cp := *p
	if len(p.Content) > 0 {
		cp.Content = append([]byte(nil), p.Content...)
	}
	cp.Warnings = append([]string(nil), p.Warnings...)
	cp.Validation.Errors = append([]string(nil), p.Validation.Errors...)
	cp.Validation.Warnings = append([]string(nil), p.Validation.Warnings...)
	return &cp
}
