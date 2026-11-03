package governance

import (
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strconv"
	"time"

	"kerob/internal/core/auth"
)

const GenesisHash = "0000000000000000000000000000000000000000000000000000000000000000"

// ComputeAuditEntryHash computes a deterministic cryptographic SHA-256 hash
// linking the record to the previous audit entry in the tenant's chain.
func ComputeAuditEntryHash(log *auth.AuditLog) string {
	if log == nil {
		return ""
	}

	detailsBytes, err := json.Marshal(log.Details)
	if err != nil {
		detailsBytes = []byte("{}")
	}

	h := sha256.New()
	fmt.Fprintf(h, "%s|%d|%s|%s|%s|%s|%s|%s|%s|%d",
		log.TenantID,
		log.SequenceNum,
		log.PrevHash,
		log.ActorID,
		log.ActorType,
		log.Action,
		log.ResourceType,
		log.ResourceID,
		string(detailsBytes),
		log.CreatedAt.UTC().UnixNano(),
	)

	return hex.EncodeToString(h.Sum(nil))
}

// AuditVerificationResult contains the cryptographic verification outcome for a tenant's audit trail.
type AuditVerificationResult struct {
	Valid            bool      `json:"valid"`
	TenantID         string    `json:"tenant_id"`
	TotalRecords     int       `json:"total_records"`
	VerifiedAt       time.Time `json:"verified_at"`
	BrokenAtSequence *int64    `json:"broken_at_sequence,omitempty"`
	ErrorMessage     string    `json:"error_message,omitempty"`
}

// VerifyAuditChain validates that a sequence of audit logs forms an unbroken, untampered hash chain.
func VerifyAuditChain(tenantID string, logs []*auth.AuditLog) *AuditVerificationResult {
	now := time.Now().UTC()
	res := &AuditVerificationResult{
		Valid:        true,
		TenantID:     tenantID,
		TotalRecords: len(logs),
		VerifiedAt:   now,
	}

	if len(logs) == 0 {
		return res
	}

	// Sort logs ascending by SequenceNum
	sorted := make([]*auth.AuditLog, len(logs))
	copy(sorted, logs)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].SequenceNum < sorted[j].SequenceNum
	})

	for i, entry := range sorted {
		if entry.TenantID != tenantID {
			res.Valid = false
			seq := entry.SequenceNum
			res.BrokenAtSequence = &seq
			res.ErrorMessage = fmt.Sprintf("cross-tenant entry found at sequence %d: %s != %s", seq, entry.TenantID, tenantID)
			return res
		}

		// Recompute hash
		computedHash := ComputeAuditEntryHash(entry)
		if entry.EntryHash != "" && entry.EntryHash != computedHash {
			res.Valid = false
			seq := entry.SequenceNum
			res.BrokenAtSequence = &seq
			res.ErrorMessage = fmt.Sprintf("tampered entry hash at sequence %d: expected %s, recorded %s", seq, computedHash, entry.EntryHash)
			return res
		}

		if i > 0 {
			prev := sorted[i-1]
			// Continuity of sequence
			if entry.SequenceNum != prev.SequenceNum+1 {
				res.Valid = false
				seq := entry.SequenceNum
				res.BrokenAtSequence = &seq
				res.ErrorMessage = fmt.Sprintf("broken sequence continuity between %d and %d", prev.SequenceNum, entry.SequenceNum)
				return res
			}

			// Cryptographic link
			if entry.PrevHash != prev.EntryHash {
				res.Valid = false
				seq := entry.SequenceNum
				res.BrokenAtSequence = &seq
				res.ErrorMessage = fmt.Sprintf("hash chain broken at sequence %d: prev_hash %s does not match previous entry_hash %s", seq, entry.PrevHash, prev.EntryHash)
				return res
			}
		} else {
			// First entry in chain
			if entry.PrevHash == "" {
				entry.PrevHash = GenesisHash
			}
		}
	}

	return res
}

// ExportAuditLogsJSON writes audit logs formatted as a JSON array.
func ExportAuditLogsJSON(w io.Writer, logs []*auth.AuditLog) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(logs)
}

// ExportAuditLogsCSV writes audit logs formatted as standard RFC-4180 CSV.
func ExportAuditLogsCSV(w io.Writer, logs []*auth.AuditLog) error {
	cw := csv.NewWriter(w)
	defer cw.Flush()

	header := []string{
		"id", "tenant_id", "sequence_num", "prev_hash", "entry_hash",
		"actor_id", "actor_type", "action", "resource_type", "resource_id",
		"details", "ip_address", "created_at",
	}
	if err := cw.Write(header); err != nil {
		return err
	}

	for _, l := range logs {
		detailsBytes, _ := json.Marshal(l.Details)
		record := []string{
			l.ID,
			l.TenantID,
			strconv.FormatInt(l.SequenceNum, 10),
			l.PrevHash,
			l.EntryHash,
			l.ActorID,
			l.ActorType,
			l.Action,
			l.ResourceType,
			l.ResourceID,
			string(detailsBytes),
			l.IPAddress,
			l.CreatedAt.UTC().Format(time.RFC3339Nano),
		}
		if err := cw.Write(record); err != nil {
			return err
		}
	}

	return cw.Error()
}
