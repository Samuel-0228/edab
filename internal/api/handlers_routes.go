package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"kerob/internal/core/auth"
	"kerob/internal/core/routing"
)

// Routes handlers
func (h *Handlers) ListRoutes(w http.ResponseWriter, r *http.Request) {
	if h.routes == nil {
		respondJSON(w, http.StatusOK, []*routing.Route{})
		return
	}
	if !h.requirePermission(w, r, auth.PermIntegrationsRead) {
		return
	}
	identity := auth.GetIdentity(r.Context())

	list, err := h.routes.List(r.Context())
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if list == nil {
		list = []*routing.Route{}
	}

	if h.multiTenancy {
		filtered := make([]*routing.Route, 0, len(list))
		for _, item := range list {
			if item.TenantID == "" || item.TenantID == identity.TenantID {
				filtered = append(filtered, item)
			}
		}
		list = filtered
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
	if !h.requirePermission(w, r, auth.PermIntegrationsWrite) {
		return
	}
	identity := auth.GetIdentity(r.Context())

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
		TenantID:      identity.TenantID,
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

	h.recordAudit(r, "route.created", "route", created.ID, map[string]any{
		"source_id":      created.SourceID,
		"destination_id": created.DestinationID,
	})

	respondJSON(w, http.StatusCreated, created)
}

func (h *Handlers) GetRoute(w http.ResponseWriter, r *http.Request) {
	if h.routes == nil {
		respondError(w, http.StatusInternalServerError, "route store not initialized")
		return
	}
	if !h.requirePermission(w, r, auth.PermIntegrationsRead) {
		return
	}
	identity := auth.GetIdentity(r.Context())

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

	if h.multiTenancy && rt.TenantID != "" && rt.TenantID != identity.TenantID {
		respondErrorWithCode(w, http.StatusForbidden, "CROSS_TENANT_DENIED", "cannot access route in another tenant", nil)
		return
	}

	respondJSON(w, http.StatusOK, rt)
}

func (h *Handlers) UpdateRoute(w http.ResponseWriter, r *http.Request) {
	if h.routes == nil {
		respondError(w, http.StatusInternalServerError, "route store not initialized")
		return
	}
	if !h.requirePermission(w, r, auth.PermIntegrationsWrite) {
		return
	}
	identity := auth.GetIdentity(r.Context())

	id := r.PathValue("id")
	existing, err := h.routes.GetByID(r.Context(), id)
	if errors.Is(err, routing.ErrRouteNotFound) {
		respondError(w, http.StatusNotFound, "route not found")
		return
	}
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if h.multiTenancy && existing.TenantID != "" && existing.TenantID != identity.TenantID {
		respondErrorWithCode(w, http.StatusForbidden, "CROSS_TENANT_DENIED", "cannot modify route in another tenant", nil)
		return
	}

	var rt routing.Route
	if err := json.NewDecoder(r.Body).Decode(&rt); err != nil {
		respondError(w, http.StatusBadRequest, "invalid json body: "+err.Error())
		return
	}
	rt.ID = id
	if rt.TenantID == "" {
		rt.TenantID = existing.TenantID
	}

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

	h.recordAudit(r, "route.updated", "route", updated.ID, map[string]any{
		"source_id":      updated.SourceID,
		"destination_id": updated.DestinationID,
	})

	respondJSON(w, http.StatusOK, updated)
}

func (h *Handlers) PatchRoute(w http.ResponseWriter, r *http.Request) {
	if h.routes == nil {
		respondError(w, http.StatusInternalServerError, "route store not initialized")
		return
	}
	if !h.requirePermission(w, r, auth.PermIntegrationsWrite) {
		return
	}
	identity := auth.GetIdentity(r.Context())

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

	if h.multiTenancy && rt.TenantID != "" && rt.TenantID != identity.TenantID {
		respondErrorWithCode(w, http.StatusForbidden, "CROSS_TENANT_DENIED", "cannot patch route in another tenant", nil)
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

	h.recordAudit(r, "route.patched", "route", updated.ID, map[string]any{
		"source_id":      updated.SourceID,
		"destination_id": updated.DestinationID,
	})

	respondJSON(w, http.StatusOK, updated)
}

func (h *Handlers) DeleteRoute(w http.ResponseWriter, r *http.Request) {
	if h.routes == nil {
		respondError(w, http.StatusInternalServerError, "route store not initialized")
		return
	}
	if !h.requirePermission(w, r, auth.PermIntegrationsWrite) {
		return
	}
	identity := auth.GetIdentity(r.Context())

	id := r.PathValue("id")
	existing, err := h.routes.GetByID(r.Context(), id)
	if errors.Is(err, routing.ErrRouteNotFound) {
		respondError(w, http.StatusNotFound, "route not found")
		return
	}
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if h.multiTenancy && existing.TenantID != "" && existing.TenantID != identity.TenantID {
		respondErrorWithCode(w, http.StatusForbidden, "CROSS_TENANT_DENIED", "cannot delete route in another tenant", nil)
		return
	}

	err = h.routes.Delete(r.Context(), id)
	if errors.Is(err, routing.ErrRouteNotFound) {
		respondError(w, http.StatusNotFound, "route not found")
		return
	}
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	h.recordAudit(r, "route.deleted", "route", id, map[string]any{
		"source_id":      existing.SourceID,
		"destination_id": existing.DestinationID,
	})

	w.WriteHeader(http.StatusNoContent)
}
