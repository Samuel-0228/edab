package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"kerob/internal/core/event"
)

func (h *Handlers) ListEvents(w http.ResponseWriter, r *http.Request) {
	if h.events == nil {
		respondJSON(w, http.StatusOK, []*event.Event{})
		return
	}

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

	respondJSON(w, http.StatusOK, list)
}

func (h *Handlers) GetEvent(w http.ResponseWriter, r *http.Request) {
	if h.events == nil {
		respondError(w, http.StatusInternalServerError, "event store not initialized")
		return
	}

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
	var input ingestEventInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		respondError(w, http.StatusBadRequest, "invalid json body: "+err.Error())
		return
	}

	if input.SourceID == "" {
		respondError(w, http.StatusBadRequest, "source_id is required")
		return
	}
	if input.ExternalEventID == "" {
		respondError(w, http.StatusBadRequest, "external_event_id is required")
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

	RecordEventProcessed()

	if h.workflowEngine != nil {
		execs, err := h.workflowEngine.ProcessEvent(r.Context(), ev)
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
