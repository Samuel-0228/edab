package api

import (
	"net/http"

	"kerob/internal/adapters"
)

func (h *Handlers) ListIntegrations(w http.ResponseWriter, r *http.Request) {
	catalog := adapters.Catalog()
	respondJSON(w, http.StatusOK, catalog)
}

func (h *Handlers) GetIntegration(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	for _, item := range adapters.Catalog() {
		if item.ID == id {
			respondJSON(w, http.StatusOK, item)
			return
		}
	}
	respondError(w, http.StatusNotFound, "integration not found")
}
