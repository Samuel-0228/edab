package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"kerob/internal/core/webhooks"
)

func (h *Handlers) ListWebhooks(w http.ResponseWriter, r *http.Request) {
	if h.webhookStore == nil {
		respondJSON(w, http.StatusOK, []*webhooks.Webhook{})
		return
	}

	list, err := h.webhookStore.List(r.Context())
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if list == nil {
		list = []*webhooks.Webhook{}
	}
	respondJSON(w, http.StatusOK, list)
}

func (h *Handlers) CreateWebhook(w http.ResponseWriter, r *http.Request) {
	if h.webhookStore == nil {
		respondError(w, http.StatusInternalServerError, "webhook store not initialized")
		return
	}

	var wh webhooks.Webhook
	if err := json.NewDecoder(r.Body).Decode(&wh); err != nil {
		respondErrorWithCode(w, http.StatusBadRequest, "MALFORMED_JSON", "invalid json body: "+err.Error(), nil)
		return
	}

	if valErrs := ValidateWebhookInput(&wh); len(valErrs) > 0 {
		respondValidationError(w, r, valErrs)
		return
	}

	created, err := h.webhookStore.Create(r.Context(), &wh)
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	respondJSON(w, http.StatusCreated, created)
}

func (h *Handlers) GetWebhook(w http.ResponseWriter, r *http.Request) {
	if h.webhookStore == nil {
		respondError(w, http.StatusInternalServerError, "webhook store not initialized")
		return
	}

	id := r.PathValue("id")
	wh, err := h.webhookStore.GetByID(r.Context(), id)
	if errors.Is(err, webhooks.ErrWebhookNotFound) {
		respondError(w, http.StatusNotFound, "webhook not found")
		return
	}
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, wh)
}

func (h *Handlers) UpdateWebhook(w http.ResponseWriter, r *http.Request) {
	if h.webhookStore == nil {
		respondError(w, http.StatusInternalServerError, "webhook store not initialized")
		return
	}

	id := r.PathValue("id")
	var wh webhooks.Webhook
	if err := json.NewDecoder(r.Body).Decode(&wh); err != nil {
		respondErrorWithCode(w, http.StatusBadRequest, "MALFORMED_JSON", "invalid json body: "+err.Error(), nil)
		return
	}
	wh.ID = id

	if valErrs := ValidateWebhookInput(&wh); len(valErrs) > 0 {
		respondValidationError(w, r, valErrs)
		return
	}

	updated, err := h.webhookStore.Update(r.Context(), &wh)
	if errors.Is(err, webhooks.ErrWebhookNotFound) {
		respondError(w, http.StatusNotFound, "webhook not found")
		return
	}
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, updated)
}

func (h *Handlers) DeleteWebhook(w http.ResponseWriter, r *http.Request) {
	if h.webhookStore == nil {
		respondError(w, http.StatusInternalServerError, "webhook store not initialized")
		return
	}

	id := r.PathValue("id")
	err := h.webhookStore.Delete(r.Context(), id)
	if errors.Is(err, webhooks.ErrWebhookNotFound) {
		respondError(w, http.StatusNotFound, "webhook not found")
		return
	}
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// ReceiveWebhook handles generic inbound HTTP webhooks at POST /webhooks/{id}
func (h *Handlers) ReceiveWebhook(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	// 1. Fetch webhook config
	if h.webhookStore == nil {
		respondError(w, http.StatusInternalServerError, "webhook store not initialized")
		return
	}

	wh, err := h.webhookStore.GetByID(r.Context(), id)
	if errors.Is(err, webhooks.ErrWebhookNotFound) {
		respondError(w, http.StatusNotFound, "webhook endpoint not found")
		return
	}
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if !wh.Enabled {
		respondError(w, http.StatusForbidden, "webhook endpoint is disabled")
		return
	}

	// 2. Authenticate secret token if configured
	if err := webhooks.Authenticate(wh, r); err != nil {
		respondError(w, http.StatusUnauthorized, "invalid webhook secret token")
		return
	}

	// 3. Read body with 1MB limit
	bodyBytes, err := io.ReadAll(io.LimitReader(r.Body, webhooks.MaxWebhookPayloadBytes+1))
	if err != nil {
		respondError(w, http.StatusBadRequest, "failed to read webhook body: "+err.Error())
		return
	}
	if len(bodyBytes) == 0 {
		respondErrorWithCode(w, http.StatusBadRequest, "EMPTY_PAYLOAD", "webhook body cannot be empty", nil)
		return
	}
	if len(bodyBytes) > webhooks.MaxWebhookPayloadBytes {
		respondError(w, http.StatusRequestEntityTooLarge, "payload exceeds maximum limit of 1MB")
		return
	}

	// Validate JSON syntax if JSON header or non-empty body
	contentType := r.Header.Get("Content-Type")
	if strings.Contains(contentType, "application/json") || len(bodyBytes) > 0 {
		var js json.RawMessage
		if err := json.Unmarshal(bodyBytes, &js); err != nil {
			respondErrorWithCode(w, http.StatusBadRequest, "MALFORMED_JSON", "webhook payload is not valid JSON: "+err.Error(), nil)
			return
		}
	}

	// 4. Normalize to Event
	ev, err := webhooks.NormalizeWebhookEvent(wh, bodyBytes, r.Header)
	if err != nil {
		respondError(w, http.StatusBadRequest, "event normalization failed: "+err.Error())
		return
	}

	// Record event metric
	RecordEventProcessed()

	// 5. Ingest into workflow engine or pipeline
	var executionCount int
	if h.workflowEngine != nil {
		execs, err := h.workflowEngine.ProcessEvent(r.Context(), ev)
		if err != nil {
			respondError(w, http.StatusInternalServerError, "workflow execution failed: "+err.Error())
			return
		}
		executionCount = len(execs)
	} else if h.pipeline != nil {
		_, err := h.pipeline.ProcessEvent(r.Context(), ev)
		if err != nil {
			respondError(w, http.StatusInternalServerError, "pipeline execution failed: "+err.Error())
			return
		}
	}

	respondJSON(w, http.StatusAccepted, map[string]any{
		"status":      "accepted",
		"event_id":    ev.ID,
		"event_type":  ev.EventType,
		"source_id":   ev.SourceID,
		"occurred_at": ev.CreatedAt,
		"executions":  executionCount,
	})
}
