package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"kerob/internal/core/auth"
	"kerob/internal/core/event"
	"kerob/internal/core/schema"
	"kerob/internal/core/streaming"
)

// -------------------------------------------------------------------------
// Schema Registry Handlers
// -------------------------------------------------------------------------

type registerSchemaInput struct {
	Name          string                   `json:"name"`
	Type          schema.SchemaType        `json:"schema_type"`
	Definition    string                   `json:"definition"`
	Compatibility schema.CompatibilityMode `json:"compatibility,omitempty"`
	Description   string                   `json:"description,omitempty"`
}

// RegisterSchema registers a new schema version after verifying compatibility.
func (h *Handlers) RegisterSchema(w http.ResponseWriter, r *http.Request) {
	if !h.requirePermission(w, r, auth.PermEventsWrite) {
		return
	}
	identity := auth.GetIdentity(r.Context())

	var input registerSchemaInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		respondError(w, http.StatusBadRequest, "invalid json body: "+err.Error())
		return
	}

	if input.Name == "" {
		respondError(w, http.StatusBadRequest, "schema name is required")
		return
	}
	if input.Definition == "" {
		respondError(w, http.StatusBadRequest, "schema definition is required")
		return
	}

	sc := &schema.Schema{
		TenantID:      identity.TenantID,
		Name:          input.Name,
		Type:          input.Type,
		Definition:    input.Definition,
		Compatibility: input.Compatibility,
		Description:   input.Description,
	}

	created, err := h.SchemaRegistry().RegisterSchema(r.Context(), sc)
	if err != nil {
		respondError(w, http.StatusBadRequest, fmt.Sprintf("schema registration failed: %s", err.Error()))
		return
	}

	h.recordAudit(r, "schema.registered", "schema", created.ID, map[string]any{
		"name":          created.Name,
		"version":       created.Version,
		"compatibility": created.Compatibility,
	})

	respondJSON(w, http.StatusCreated, created)
}

// ListSchemas lists the latest versions of all registered schemas.
func (h *Handlers) ListSchemas(w http.ResponseWriter, r *http.Request) {
	if !h.requirePermission(w, r, auth.PermEventsRead) {
		return
	}
	identity := auth.GetIdentity(r.Context())

	list, err := h.SchemaRegistry().Store().List(r.Context(), identity.TenantID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if list == nil {
		list = []*schema.Schema{}
	}
	respondJSON(w, http.StatusOK, list)
}

// GetSchema returns the latest version of a schema by name.
func (h *Handlers) GetSchema(w http.ResponseWriter, r *http.Request) {
	if !h.requirePermission(w, r, auth.PermEventsRead) {
		return
	}
	identity := auth.GetIdentity(r.Context())
	name := r.PathValue("name")

	sc, err := h.SchemaRegistry().Store().GetLatest(r.Context(), identity.TenantID, name)
	if err != nil {
		respondError(w, http.StatusNotFound, "schema not found")
		return
	}
	respondJSON(w, http.StatusOK, sc)
}

// ListSchemaVersions returns all historical versions for a schema name.
func (h *Handlers) ListSchemaVersions(w http.ResponseWriter, r *http.Request) {
	if !h.requirePermission(w, r, auth.PermEventsRead) {
		return
	}
	identity := auth.GetIdentity(r.Context())
	name := r.PathValue("name")

	versions, err := h.SchemaRegistry().Store().ListVersions(r.Context(), identity.TenantID, name)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if versions == nil {
		versions = []*schema.Schema{}
	}
	respondJSON(w, http.StatusOK, versions)
}

// GetSchemaVersion returns a specific version of a schema.
func (h *Handlers) GetSchemaVersion(w http.ResponseWriter, r *http.Request) {
	if !h.requirePermission(w, r, auth.PermEventsRead) {
		return
	}
	identity := auth.GetIdentity(r.Context())
	name := r.PathValue("name")
	verStr := r.PathValue("version")

	ver, err := strconv.Atoi(verStr)
	if err != nil || ver <= 0 {
		respondError(w, http.StatusBadRequest, "invalid version number")
		return
	}

	sc, err := h.SchemaRegistry().Store().GetByVersion(r.Context(), identity.TenantID, name, ver)
	if err != nil {
		respondError(w, http.StatusNotFound, "schema version not found")
		return
	}
	respondJSON(w, http.StatusOK, sc)
}

// ValidatePayloadAgainstSchema validates an arbitrary payload against a schema.
func (h *Handlers) ValidatePayloadAgainstSchema(w http.ResponseWriter, r *http.Request) {
	if !h.requirePermission(w, r, auth.PermEventsRead) {
		return
	}
	identity := auth.GetIdentity(r.Context())
	name := r.PathValue("name")

	var reqBody struct {
		Payload any `json:"payload"`
	}
	if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
		respondError(w, http.StatusBadRequest, "invalid json body: "+err.Error())
		return
	}

	payloadBytes, _ := json.Marshal(reqBody.Payload)
	valErrs, err := h.SchemaRegistry().ValidateEvent(r.Context(), identity.TenantID, name, payloadBytes)
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	isValid := len(valErrs) == 0
	respondJSON(w, http.StatusOK, map[string]any{
		"valid":  isValid,
		"errors": valErrs,
	})
}

// -------------------------------------------------------------------------
// Event Streaming Handlers
// -------------------------------------------------------------------------

type publishStreamInput struct {
	Topic       string            `json:"topic"`
	OrderingKey string            `json:"ordering_key,omitempty"`
	SourceID    string            `json:"source_id,omitempty"`
	EventType   string            `json:"event_type"`
	Payload     any               `json:"payload"`
	Metadata    map[string]any    `json:"metadata,omitempty"`
	Headers     map[string]string `json:"headers,omitempty"`
}

// PublishStream publishes an event onto a partitioned stream broker.
func (h *Handlers) PublishStream(w http.ResponseWriter, r *http.Request) {
	if !h.requirePermission(w, r, auth.PermEventsWrite) {
		return
	}
	identity := auth.GetIdentity(r.Context())

	var input publishStreamInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		respondError(w, http.StatusBadRequest, "invalid json body: "+err.Error())
		return
	}

	if input.EventType == "" {
		respondError(w, http.StatusBadRequest, "event_type is required")
		return
	}
	if input.SourceID == "" {
		input.SourceID = "stream.ingest"
	}
	if input.Topic == "" {
		input.Topic = "events"
	}

	// 1. Build normalized Event
	ev, err := event.New(
		input.SourceID,
		fmt.Sprintf("stream-%s-%d", input.EventType, time.Now().UnixNano()),
		input.EventType,
		input.Payload,
		input.Metadata,
	)
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	ev.TenantID = identity.TenantID

	// 2. Validate against Schema if registered
	if valErrs, err := h.SchemaRegistry().ValidateEvent(r.Context(), identity.TenantID, ev.EventType, ev.Payload); err != nil || len(valErrs) > 0 {
		errMsg := "schema validation failed"
		if err != nil {
			errMsg = err.Error()
		}
		respondJSON(w, http.StatusBadRequest, map[string]any{
			"error":   errMsg,
			"details": valErrs,
		})
		return
	}

	// 3. Publish to Stream Broker
	producer, err := h.StreamAdapter().Producer()
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to acquire stream producer: "+err.Error())
		return
	}

	res, err := producer.Publish(r.Context(), &streaming.Record{
		Topic:       input.Topic,
		OrderingKey: input.OrderingKey,
		Event:       ev,
		Headers:     input.Headers,
		Timestamp:   time.Now().UTC(),
	})
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to publish stream record: "+err.Error())
		return
	}

	h.recordAudit(r, "stream.published", "stream", res.ID, map[string]any{
		"topic":        res.Topic,
		"partition":    res.Partition,
		"offset":       res.Offset,
		"event_id":     ev.ID,
		"ordering_key": input.OrderingKey,
	})

	respondJSON(w, http.StatusAccepted, res)
}

// GetStreamOffsets inspects consumer group offsets.
func (h *Handlers) GetStreamOffsets(w http.ResponseWriter, r *http.Request) {
	if !h.requirePermission(w, r, auth.PermEventsRead) {
		return
	}

	groupID := r.URL.Query().Get("group_id")
	if groupID == "" {
		groupID = "kerob-engine-workers"
	}

	offsets, err := h.OffsetStore().ListOffsets(r.Context(), groupID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if offsets == nil {
		offsets = []streaming.ConsumerOffset{}
	}

	respondJSON(w, http.StatusOK, offsets)
}

// ReplayStream replays historical stream events by time range, source, or event type.
func (h *Handlers) ReplayStream(w http.ResponseWriter, r *http.Request) {
	if !h.requirePermission(w, r, auth.PermEventsReplay) {
		return
	}
	identity := auth.GetIdentity(r.Context())

	var filter streaming.ReplayFilter
	if err := json.NewDecoder(r.Body).Decode(&filter); err != nil {
		respondError(w, http.StatusBadRequest, "invalid replay filter json: "+err.Error())
		return
	}

	filter.TenantID = identity.TenantID
	res, err := h.StreamReplayEngine().ReplayEvents(r.Context(), filter)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "stream replay error: "+err.Error())
		return
	}

	h.recordAudit(r, "stream.replayed", "stream", "bulk", map[string]any{
		"matched":  res.TotalMatched,
		"replayed": res.Replayed,
	})

	respondJSON(w, http.StatusOK, res)
}

// ListStreamDLQ lists dead-letter queue records from the stream.
func (h *Handlers) ListStreamDLQ(w http.ResponseWriter, r *http.Request) {
	if !h.requirePermission(w, r, auth.PermDLQRead) {
		return
	}
	identity := auth.GetIdentity(r.Context())

	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))

	var resolved *bool
	if r.URL.Query().Has("resolved") {
		b := r.URL.Query().Get("resolved") == "true"
		resolved = &b
	}

	list, err := h.DeadLetterStore().List(r.Context(), streaming.DLQFilter{
		TenantID: identity.TenantID,
		Topic:    r.URL.Query().Get("topic"),
		Resolved: resolved,
		Limit:    limit,
		Offset:   offset,
	})
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if list == nil {
		list = []*streaming.DeadLetterEvent{}
	}

	respondJSON(w, http.StatusOK, list)
}

// RetryStreamDLQ retries a dead-letter event through the automation engine.
func (h *Handlers) RetryStreamDLQ(w http.ResponseWriter, r *http.Request) {
	if !h.requirePermission(w, r, auth.PermDLQRetry) {
		return
	}

	id := r.PathValue("id")
	ev, execs, err := h.StreamReplayEngine().ReplayDeadLetterEvent(r.Context(), id)
	if err != nil {
		respondError(w, http.StatusBadRequest, "failed to retry dead-letter event: "+err.Error())
		return
	}

	h.recordAudit(r, "stream.dlq_retried", "dlq", id, map[string]any{
		"event_id":   ev.ID,
		"executions": len(execs),
	})

	respondJSON(w, http.StatusOK, map[string]any{
		"event":      ev,
		"executions": execs,
		"status":     "resolved",
	})
}
