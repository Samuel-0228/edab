package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	"kerob/internal/core/auth"
	"kerob/internal/core/workflow"
)

type replayRequest struct {
	BypassIdempotency bool `json:"bypass_idempotency"`
}

type bulkReplayRequest struct {
	EventIDs          []string `json:"event_ids"`
	BypassIdempotency bool     `json:"bypass_idempotency"`
}

type bulkDLQRequest struct {
	IDs []string `json:"ids,omitempty"`
}

// ReplayEvent replays a single ingested event through active matching workflows.
func (h *Handlers) ReplayEvent(w http.ResponseWriter, r *http.Request) {
	if !h.requirePermission(w, r, auth.PermEventsReplay) {
		return
	}
	identity := auth.GetIdentity(r.Context())

	id := r.PathValue("id")
	ev, err := h.events.GetByID(r.Context(), id)
	if err != nil {
		respondError(w, http.StatusNotFound, "event not found")
		return
	}

	if h.multiTenancy && ev.TenantID != "" && ev.TenantID != identity.TenantID {
		respondErrorWithCode(w, http.StatusForbidden, "CROSS_TENANT_DENIED", "cannot replay event belonging to another tenant", nil)
		return
	}

	var req replayRequest
	if r.Body != nil && r.ContentLength > 0 {
		_ = json.NewDecoder(r.Body).Decode(&req)
	}

	targetEv := ev
	if req.BypassIdempotency {
		// Clone event with fresh external_event_id and ID to bypass unique constraint
		cloned := *ev
		cloned.ID = uuid.NewString()
		cloned.ExternalEventID = fmt.Sprintf("%s-replay-%d", ev.ExternalEventID, time.Now().UnixNano())
		cloned.CreatedAt = time.Now().UTC()
		targetEv = &cloned
	}

	executions, err := h.workflowEngine.ProcessEvent(r.Context(), targetEv)
	if err != nil {
		respondError(w, http.StatusInternalServerError, fmt.Sprintf("event replay failed: %s", err.Error()))
		return
	}

	h.recordAudit(r, "event.replayed", "event", ev.ID, map[string]any{
		"target_event_id":    targetEv.ID,
		"bypass_idempotency": req.BypassIdempotency,
		"executions":         len(executions),
	})

	respondJSON(w, http.StatusOK, map[string]any{
		"event_id":           ev.ID,
		"replayed_event_id":  targetEv.ID,
		"bypass_idempotency": req.BypassIdempotency,
		"executions":         executions,
	})
}

// ReplayBulkEvents replays multiple events through the automation engine.
func (h *Handlers) ReplayBulkEvents(w http.ResponseWriter, r *http.Request) {
	if !h.requirePermission(w, r, auth.PermEventsReplay) {
		return
	}
	identity := auth.GetIdentity(r.Context())

	var req bulkReplayRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	if len(req.EventIDs) == 0 {
		respondError(w, http.StatusBadRequest, "event_ids must not be empty")
		return
	}

	type replayResult struct {
		EventID        string                `json:"event_id"`
		ExecutionCount int                   `json:"execution_count"`
		Executions     []*workflow.Execution `json:"executions,omitempty"`
		Error          string                `json:"error,omitempty"`
	}

	results := make([]replayResult, 0, len(req.EventIDs))
	for _, eventID := range req.EventIDs {
		ev, err := h.events.GetByID(r.Context(), eventID)
		if err != nil {
			results = append(results, replayResult{EventID: eventID, Error: "not found"})
			continue
		}

		if h.multiTenancy && ev.TenantID != "" && ev.TenantID != identity.TenantID {
			results = append(results, replayResult{EventID: eventID, Error: "cross-tenant access denied"})
			continue
		}

		targetEv := ev
		if req.BypassIdempotency {
			cloned := *ev
			cloned.ID = uuid.NewString()
			cloned.ExternalEventID = fmt.Sprintf("%s-replay-%d", ev.ExternalEventID, time.Now().UnixNano())
			cloned.CreatedAt = time.Now().UTC()
			targetEv = &cloned
		}

		execs, err := h.workflowEngine.ProcessEvent(r.Context(), targetEv)
		if err != nil {
			results = append(results, replayResult{EventID: eventID, Error: err.Error()})
			continue
		}
		results = append(results, replayResult{
			EventID:        eventID,
			ExecutionCount: len(execs),
			Executions:     execs,
		})
	}

	h.recordAudit(r, "event.bulk_replayed", "event", "bulk", map[string]any{
		"replayed_count":     len(results),
		"bypass_idempotency": req.BypassIdempotency,
	})

	respondJSON(w, http.StatusOK, map[string]any{
		"replayed_count":     len(results),
		"bypass_idempotency": req.BypassIdempotency,
		"results":            results,
	})
}

// BulkRetryDeadLetters retries dead-letter executions.
func (h *Handlers) BulkRetryDeadLetters(w http.ResponseWriter, r *http.Request) {
	if !h.requirePermission(w, r, auth.PermDLQRetry) {
		return
	}
	identity := auth.GetIdentity(r.Context())

	var req bulkDLQRequest
	if r.Body != nil && r.ContentLength > 0 {
		_ = json.NewDecoder(r.Body).Decode(&req)
	}

	records, err := h.deadLetters.List(r.Context(), false)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	idMap := make(map[string]bool)
	for _, id := range req.IDs {
		idMap[id] = true
	}

	retried := 0
	for _, rec := range records {
		if len(idMap) > 0 && !idMap[rec.ID] {
			continue
		}

		recTenant := rec.TenantID
		if recTenant == "" && h.workflows != nil {
			if wf, err := h.workflows.GetByID(r.Context(), rec.WorkflowID); err == nil && wf != nil {
				recTenant = wf.TenantID
			}
		}
		if h.multiTenancy && recTenant != "" && recTenant != identity.TenantID {
			continue
		}

		if _, err := h.executions.ResetForRetry(r.Context(), rec.ExecutionID); err != nil {
			continue
		}
		_ = h.deadLetters.Discard(r.Context(), rec.ID)
		retried++
	}

	h.recordAudit(r, "dlq.bulk_retried", "dead_letter", "bulk", map[string]any{
		"retried_count": retried,
	})

	respondJSON(w, http.StatusOK, map[string]any{
		"retried_count": retried,
	})
}

// BulkDiscardDeadLetters discards dead-letter executions permanently.
func (h *Handlers) BulkDiscardDeadLetters(w http.ResponseWriter, r *http.Request) {
	if !h.requirePermission(w, r, auth.PermDLQRetry) {
		return
	}
	identity := auth.GetIdentity(r.Context())

	var req bulkDLQRequest
	if r.Body != nil && r.ContentLength > 0 {
		_ = json.NewDecoder(r.Body).Decode(&req)
	}

	records, err := h.deadLetters.List(r.Context(), false)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	idMap := make(map[string]bool)
	for _, id := range req.IDs {
		idMap[id] = true
	}

	discarded := 0
	for _, rec := range records {
		if len(idMap) > 0 && !idMap[rec.ID] {
			continue
		}

		recTenant := rec.TenantID
		if recTenant == "" && h.workflows != nil {
			if wf, err := h.workflows.GetByID(r.Context(), rec.WorkflowID); err == nil && wf != nil {
				recTenant = wf.TenantID
			}
		}
		if h.multiTenancy && recTenant != "" && recTenant != identity.TenantID {
			continue
		}

		_ = h.deadLetters.Discard(r.Context(), rec.ID)
		errStr := rec.Error
		_ = h.executions.UpdateStatus(r.Context(), rec.ExecutionID, workflow.ExecutionCancelled, &errStr, 0)
		discarded++
	}

	h.recordAudit(r, "dlq.bulk_discarded", "dead_letter", "bulk", map[string]any{
		"discarded_count": discarded,
	})

	respondJSON(w, http.StatusOK, map[string]any{
		"discarded_count": discarded,
	})
}
