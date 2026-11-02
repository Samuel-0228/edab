package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"kerob/internal/core/auth"
	"kerob/internal/core/event"
	"kerob/internal/observability"
)

func (h *Handlers) ListEvents(w http.ResponseWriter, r *http.Request) {
	if h.events == nil {
		respondJSON(w, http.StatusOK, []*event.Event{})
		return
	}
	if !h.requirePermission(w, r, auth.PermEventsRead) {
		return
	}
	identity := auth.GetIdentity(r.Context())

	sourceID := r.URL.Query().Get("source_id")
	eventType := r.URL.Query().Get("event_type")

	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	offset := 0
	if o := r.URL.Query().Get("offset"); o != "" {
		if parsed, err := strconv.Atoi(o); err == nil && parsed >= 0 {
			offset = parsed
		}
	}

	list, err := h.events.List(r.Context(), event.ListFilter{
		SourceID:  sourceID,
		EventType: eventType,
		Limit:     limit,
		Offset:    offset,
	})
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if list == nil {
		list = []*event.Event{}
	}

	if h.multiTenancy {
		filtered := make([]*event.Event, 0, len(list))
		for _, ev := range list {
			if ev.TenantID == "" || ev.TenantID == identity.TenantID {
				filtered = append(filtered, ev)
			}
		}
		list = filtered
	}

	respondJSON(w, http.StatusOK, list)
}

func (h *Handlers) GetEvent(w http.ResponseWriter, r *http.Request) {
	if h.events == nil {
		respondError(w, http.StatusInternalServerError, "event store not initialized")
		return
	}
	if !h.requirePermission(w, r, auth.PermEventsRead) {
		return
	}
	identity := auth.GetIdentity(r.Context())

	id := r.PathValue("id")
	ev, err := h.events.GetByID(r.Context(), id)
	if errors.Is(err, event.ErrEventNotFound) {
		respondError(w, http.StatusNotFound, "event not found")
		return
	}
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if h.multiTenancy && ev.TenantID != "" && ev.TenantID != identity.TenantID {
		respondErrorWithCode(w, http.StatusForbidden, "CROSS_TENANT_DENIED", "cannot access event in another tenant", nil)
		return
	}

	respondJSON(w, http.StatusOK, ev)
}

type ingestEventInput struct {
	SourceID        string          `json:"source_id"`
	ExternalEventID string          `json:"external_event_id"`
	EventType       string          `json:"event_type"`
	Payload         json.RawMessage `json:"payload"`
	Metadata        json.RawMessage `json:"metadata,omitempty"`
}

func (h *Handlers) IngestEvent(w http.ResponseWriter, r *http.Request) {
	if !h.requirePermission(w, r, auth.PermEventsWrite) {
		return
	}
	identity := auth.GetIdentity(r.Context())

	var input ingestEventInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		respondError(w, http.StatusBadRequest, "invalid json body: "+err.Error())
		return
	}

	if valErrs := ValidateEventInput(input.SourceID, input.ExternalEventID, input.EventType, input.Payload); len(valErrs) > 0 {
		respondValidationError(w, r, valErrs)
		return
	}

	if input.EventType == "" {
		input.EventType = "default"
	}

	ev, err := event.New(input.SourceID, input.ExternalEventID, input.EventType, input.Payload, input.Metadata)
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	ev.TenantID = identity.TenantID

	RecordEventProcessed()

	ctx, span := observability.StartSpan(r.Context(), "event.ingestion")
	defer span.End()
	span.SetAttribute("event.id", ev.ID)
	span.SetAttribute("event.source", ev.SourceID)
	span.SetAttribute("event.type", ev.EventType)

	h.recordAudit(r, "event.ingested", "event", ev.ID, map[string]any{
		"source_id":   ev.SourceID,
		"event_type":  ev.EventType,
		"external_id": ev.ExternalEventID,
	})

	if h.workflowEngine != nil {
		execs, err := h.workflowEngine.ProcessEvent(ctx, ev)
		if err != nil {
			respondError(w, http.StatusInternalServerError, err.Error())
			return
		}
		respondJSON(w, http.StatusCreated, map[string]any{
			"event":      ev,
			"executions": len(execs),
		})
		return
	}

	if h.pipeline != nil {
		processed, err := h.pipeline.ProcessEvent(r.Context(), ev)
		if err != nil {
			respondError(w, http.StatusInternalServerError, err.Error())
			return
		}
		respondJSON(w, http.StatusCreated, processed)
		return
	}

	respondError(w, http.StatusInternalServerError, "no pipeline or workflow engine configured")
}
