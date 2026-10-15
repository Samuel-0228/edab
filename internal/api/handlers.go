package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"

	"automation-engine/internal/adapters/destinations"
	"automation-engine/internal/adapters/sources"
	"automation-engine/internal/core/delivery"
	"automation-engine/internal/core/event"
	"automation-engine/internal/core/routing"
	"automation-engine/internal/core/workflow"
)

type Handlers struct {
	sources      sources.Store
	destinations destinations.Store
	routes       routing.Store
	events       event.Store
	deliveries   delivery.Store
	pipeline     *workflow.Pipeline
	sourceReg    *sources.Registry
	destReg      *destinations.Registry
}

func NewHandlers(
	srcStore sources.Store,
	destStore destinations.Store,
	routeStore routing.Store,
	eventStore event.Store,
	deliveryStore delivery.Store,
	pipeline *workflow.Pipeline,
	sourceReg *sources.Registry,
	destReg *destinations.Registry,
) *Handlers {
	return &Handlers{
		sources:      srcStore,
		destinations: destStore,
		routes:       routeStore,
		events:       eventStore,
		deliveries:   deliveryStore,
		pipeline:     pipeline,
		sourceReg:    sourceReg,
		destReg:      destReg,
	}
}

// Health handlers
func (h *Handlers) Health(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, http.StatusOK, map[string]string{
		"status": "healthy",
	})
}

func (h *Handlers) Ready(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, http.StatusOK, map[string]any{
		"status":                  "ready",
		"registered_sources":      h.sourceReg.RegisteredTypes(),
		"registered_destinations": h.destReg.RegisteredTypes(),
	})
}

// Sources handlers
func (h *Handlers) ListSources(w http.ResponseWriter, r *http.Request) {
	list, err := h.sources.List(r.Context())
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if list == nil {
		list = []*sources.Source{}
	}
	respondJSON(w, http.StatusOK, list)
}

func (h *Handlers) CreateSource(w http.ResponseWriter, r *http.Request) {
	var src sources.Source
	if err := json.NewDecoder(r.Body).Decode(&src); err != nil {
		respondError(w, http.StatusBadRequest, "invalid json body: "+err.Error())
		return
	}
	if src.Name == "" || src.AdapterType == "" {
		respondError(w, http.StatusBadRequest, "name and adapter_type are required")
		return
	}

	created, err := h.sources.Create(r.Context(), &src)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusCreated, created)
}

func (h *Handlers) GetSource(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	src, err := h.sources.GetByID(r.Context(), id)
	if errors.Is(err, sources.ErrSourceNotFound) {
		respondError(w, http.StatusNotFound, "source not found")
		return
	}
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, src)
}

func (h *Handlers) UpdateSource(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var src sources.Source
	if err := json.NewDecoder(r.Body).Decode(&src); err != nil {
		respondError(w, http.StatusBadRequest, "invalid json body: "+err.Error())
		return
	}
	src.ID = id

	updated, err := h.sources.Update(r.Context(), &src)
	if errors.Is(err, sources.ErrSourceNotFound) {
		respondError(w, http.StatusNotFound, "source not found")
		return
	}
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, updated)
}

func (h *Handlers) DeleteSource(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	err := h.sources.Delete(r.Context(), id)
	if errors.Is(err, sources.ErrSourceNotFound) {
		respondError(w, http.StatusNotFound, "source not found")
		return
	}
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// IngestRawSourceEvent allows external clients / webhooks to push raw data to a source.
func (h *Handlers) IngestRawSourceEvent(w http.ResponseWriter, r *http.Request) {
	sourceID := r.PathValue("id")
	body, err := io.ReadAll(io.LimitReader(r.Body, 10*1024*1024)) // 10MB limit
	if err != nil {
		respondError(w, http.StatusBadRequest, "failed reading body: "+err.Error())
		return
	}
	if len(body) == 0 {
		respondError(w, http.StatusBadRequest, "empty request body")
		return
	}

	ev, err := h.pipeline.IngestRaw(r.Context(), sourceID, body)
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	respondJSON(w, http.StatusAccepted, ev)
}

// Destinations handlers
func (h *Handlers) ListDestinations(w http.ResponseWriter, r *http.Request) {
	list, err := h.destinations.List(r.Context())
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if list == nil {
		list = []*destinations.Destination{}
	}
	respondJSON(w, http.StatusOK, list)
}

func (h *Handlers) CreateDestination(w http.ResponseWriter, r *http.Request) {
	var dest destinations.Destination
	if err := json.NewDecoder(r.Body).Decode(&dest); err != nil {
		respondError(w, http.StatusBadRequest, "invalid json body: "+err.Error())
		return
	}
	if dest.Name == "" || dest.AdapterType == "" {
		respondError(w, http.StatusBadRequest, "name and adapter_type are required")
		return
	}

	created, err := h.destinations.Create(r.Context(), &dest)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusCreated, created)
}

func (h *Handlers) GetDestination(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	dest, err := h.destinations.GetByID(r.Context(), id)
	if errors.Is(err, destinations.ErrDestinationNotFound) {
		respondError(w, http.StatusNotFound, "destination not found")
		return
	}
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, dest)
}

func (h *Handlers) UpdateDestination(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var dest destinations.Destination
	if err := json.NewDecoder(r.Body).Decode(&dest); err != nil {
		respondError(w, http.StatusBadRequest, "invalid json body: "+err.Error())
		return
	}
	dest.ID = id

	updated, err := h.destinations.Update(r.Context(), &dest)
	if errors.Is(err, destinations.ErrDestinationNotFound) {
		respondError(w, http.StatusNotFound, "destination not found")
		return
	}
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, updated)
}

func (h *Handlers) DeleteDestination(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	err := h.destinations.Delete(r.Context(), id)
	if errors.Is(err, destinations.ErrDestinationNotFound) {
		respondError(w, http.StatusNotFound, "destination not found")
		return
	}
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Routes handlers
func (h *Handlers) ListRoutes(w http.ResponseWriter, r *http.Request) {
	list, err := h.routes.List(r.Context())
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if list == nil {
		list = []*routing.Route{}
	}
	respondJSON(w, http.StatusOK, list)
}

func (h *Handlers) CreateRoute(w http.ResponseWriter, r *http.Request) {
	var route routing.Route
	if err := json.NewDecoder(r.Body).Decode(&route); err != nil {
		respondError(w, http.StatusBadRequest, "invalid json body: "+err.Error())
		return
	}
	if route.SourceID == "" || route.DestinationID == "" {
		respondError(w, http.StatusBadRequest, "source_id and destination_id are required")
		return
	}

	created, err := h.routes.Create(r.Context(), &route)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusCreated, created)
}

func (h *Handlers) GetRoute(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	route, err := h.routes.GetByID(r.Context(), id)
	if errors.Is(err, routing.ErrRouteNotFound) {
		respondError(w, http.StatusNotFound, "route not found")
		return
	}
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, route)
}

func (h *Handlers) UpdateRoute(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var route routing.Route
	if err := json.NewDecoder(r.Body).Decode(&route); err != nil {
		respondError(w, http.StatusBadRequest, "invalid json body: "+err.Error())
		return
	}
	route.ID = id

	updated, err := h.routes.Update(r.Context(), &route)
	if errors.Is(err, routing.ErrRouteNotFound) {
		respondError(w, http.StatusNotFound, "route not found")
		return
	}
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, updated)
}

func (h *Handlers) DeleteRoute(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	err := h.routes.Delete(r.Context(), id)
	if errors.Is(err, routing.ErrRouteNotFound) {
		respondError(w, http.StatusNotFound, "route not found")
		return
	}
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Events handlers
func (h *Handlers) ListEvents(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	limit, _ := strconv.Atoi(query.Get("limit"))
	offset, _ := strconv.Atoi(query.Get("offset"))

	filter := event.ListFilter{
		SourceID:  query.Get("source_id"),
		EventType: query.Get("event_type"),
		Limit:     limit,
		Offset:    offset,
	}

	events, err := h.events.List(r.Context(), filter)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if events == nil {
		events = []*event.Event{}
	}
	respondJSON(w, http.StatusOK, events)
}

func (h *Handlers) GetEvent(w http.ResponseWriter, r *http.Request) {
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

// Direct generic event ingest endpoint
func (h *Handlers) IngestEvent(w http.ResponseWriter, r *http.Request) {
	var req struct {
		SourceID        string          `json:"source_id"`
		ExternalEventID string          `json:"external_event_id"`
		EventType       string          `json:"event_type"`
		Payload         json.RawMessage `json:"payload"`
		Metadata        json.RawMessage `json:"metadata"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid json body: "+err.Error())
		return
	}

	ev, err := event.New(req.SourceID, req.ExternalEventID, req.EventType, req.Payload, req.Metadata)
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	persisted, err := h.pipeline.ProcessEvent(r.Context(), ev)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusAccepted, persisted)
}

// Deliveries handlers
func (h *Handlers) ListDeliveries(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	limit, _ := strconv.Atoi(query.Get("limit"))
	offset, _ := strconv.Atoi(query.Get("offset"))

	filter := delivery.ListFilter{
		EventID:       query.Get("event_id"),
		DestinationID: query.Get("destination_id"),
		Status:        delivery.Status(query.Get("status")),
		Limit:         limit,
		Offset:        offset,
	}

	jobs, err := h.deliveries.List(r.Context(), filter)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if jobs == nil {
		jobs = []*delivery.DeliveryJob{}
	}
	respondJSON(w, http.StatusOK, jobs)
}

func (h *Handlers) GetDelivery(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	job, err := h.deliveries.GetByID(r.Context(), id)
	if errors.Is(err, delivery.ErrJobNotFound) {
		respondError(w, http.StatusNotFound, "delivery job not found")
		return
	}
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, job)
}

func (h *Handlers) RetryDelivery(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	job, err := h.deliveries.ResetForRetry(r.Context(), id)
	if errors.Is(err, delivery.ErrJobNotFound) {
		respondError(w, http.StatusNotFound, "delivery job not found")
		return
	}
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, job)
}

func respondJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

func respondError(w http.ResponseWriter, status int, message string) {
	respondJSON(w, status, map[string]string{
		"error": message,
	})
}
