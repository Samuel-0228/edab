package api

import (
	"encoding/json"
	"errors"
	"kerob/internal/core/routing"
	"net/http"
)

// Routes handlers
func (h *Handlers) ListRoutes(w http.ResponseWriter, r *http.Request) {
	if h.routes == nil {
		respondJSON(w, http.StatusOK, []*routing.Route{})
		return
	}
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

type routeInput struct {
	ID            string          `json:"id,omitempty"`
	SourceID      string          `json:"source_id"`
	DestinationID string          `json:"destination_id"`
	Enabled       *bool           `json:"enabled,omitempty"`
	Filters       json.RawMessage `json:"filters,omitempty"`
}

func (h *Handlers) CreateRoute(w http.ResponseWriter, r *http.Request) {
	if h.routes == nil {
		respondError(w, http.StatusInternalServerError, "route store not initialized")
		return
	}

	var input routeInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		respondError(w, http.StatusBadRequest, "invalid json body: "+err.Error())
		return
	}
	if input.SourceID == "" {
		respondError(w, http.StatusBadRequest, "source_id is required")
		return
	}
	if input.DestinationID == "" {
		respondError(w, http.StatusBadRequest, "destination_id is required")
		return
	}

	enabled := true
	if input.Enabled != nil {
		enabled = *input.Enabled
	}
	filters := json.RawMessage(`{}`)
	if len(input.Filters) > 0 {
		filters = input.Filters
	}

	rt := routing.Route{
		ID:            input.ID,
		SourceID:      input.SourceID,
		DestinationID: input.DestinationID,
		Enabled:       enabled,
		Filters:       filters,
	}

	if valErrs := ValidateRouteInput(&rt); len(valErrs) > 0 {
		respondValidationError(w, r, valErrs)
		return
	}

	created, err := h.routes.Create(r.Context(), &rt)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusCreated, created)
}

func (h *Handlers) GetRoute(w http.ResponseWriter, r *http.Request) {
	if h.routes == nil {
		respondError(w, http.StatusInternalServerError, "route store not initialized")
		return
	}

	id := r.PathValue("id")
	rt, err := h.routes.GetByID(r.Context(), id)
	if errors.Is(err, routing.ErrRouteNotFound) {
		respondError(w, http.StatusNotFound, "route not found")
		return
	}
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, rt)
}

func (h *Handlers) UpdateRoute(w http.ResponseWriter, r *http.Request) {
	if h.routes == nil {
		respondError(w, http.StatusInternalServerError, "route store not initialized")
		return
	}

	id := r.PathValue("id")
	var rt routing.Route
	if err := json.NewDecoder(r.Body).Decode(&rt); err != nil {
		respondError(w, http.StatusBadRequest, "invalid json body: "+err.Error())
		return
	}
	rt.ID = id
	if valErrs := ValidateRouteInput(&rt); len(valErrs) > 0 {
		respondValidationError(w, r, valErrs)
		return
	}

	updated, err := h.routes.Update(r.Context(), &rt)
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

func (h *Handlers) PatchRoute(w http.ResponseWriter, r *http.Request) {
	if h.routes == nil {
		respondError(w, http.StatusInternalServerError, "route store not initialized")
		return
	}

	id := r.PathValue("id")
	rt, err := h.routes.GetByID(r.Context(), id)
	if errors.Is(err, routing.ErrRouteNotFound) {
		respondError(w, http.StatusNotFound, "route not found")
		return
	}
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	var input struct {
		Enabled *bool           `json:"enabled,omitempty"`
		Filters json.RawMessage `json:"filters,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		respondError(w, http.StatusBadRequest, "invalid json body: "+err.Error())
		return
	}

	if input.Enabled != nil {
		rt.Enabled = *input.Enabled
	}
	if len(input.Filters) > 0 {
		rt.Filters = input.Filters
	}

	if valErrs := ValidateRouteInput(rt); len(valErrs) > 0 {
		respondValidationError(w, r, valErrs)
		return
	}

	updated, err := h.routes.Update(r.Context(), rt)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, updated)
}

func (h *Handlers) DeleteRoute(w http.ResponseWriter, r *http.Request) {
	if h.routes == nil {
		respondError(w, http.StatusInternalServerError, "route store not initialized")
		return
	}

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
