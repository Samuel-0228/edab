package api

import (
	"errors"
	"kerob/internal/core/delivery"
	"net/http"
	"strconv"
)

// Deliveries handlers
func (h *Handlers) ListDeliveries(w http.ResponseWriter, r *http.Request) {
	if h.deliveries == nil {
		respondJSON(w, http.StatusOK, []*delivery.DeliveryJob{})
		return
	}

	filter := delivery.ListFilter{}
	if destID := r.URL.Query().Get("destination_id"); destID != "" {
		filter.DestinationID = destID
	}
	if evID := r.URL.Query().Get("event_id"); evID != "" {
		filter.EventID = evID
	}
	if status := r.URL.Query().Get("status"); status != "" {
		filter.Status = delivery.Status(status)
	}
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if limit, err := strconv.Atoi(limitStr); err == nil && limit > 0 {
			filter.Limit = limit
		}
	}
	if offsetStr := r.URL.Query().Get("offset"); offsetStr != "" {
		if offset, err := strconv.Atoi(offsetStr); err == nil && offset >= 0 {
			filter.Offset = offset
		}
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

func (h *Handlers) ListFailedDeliveries(w http.ResponseWriter, r *http.Request) {
	if h.deliveries == nil {
		respondJSON(w, http.StatusOK, []*delivery.DeliveryJob{})
		return
	}

	failed, err := h.deliveries.List(r.Context(), delivery.ListFilter{Status: delivery.StatusFailed})
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	dead, err := h.deliveries.List(r.Context(), delivery.ListFilter{Status: delivery.StatusDeadLetter})
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	combined := append(failed, dead...)
	if combined == nil {
		combined = []*delivery.DeliveryJob{}
	}
	respondJSON(w, http.StatusOK, combined)
}

func (h *Handlers) DeliveryStats(w http.ResponseWriter, r *http.Request) {
	if h.deliveries == nil {
		respondJSON(w, http.StatusOK, delivery.Stats{})
		return
	}

	stats, err := h.deliveries.Stats(r.Context())
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, stats)
}

func (h *Handlers) GetDelivery(w http.ResponseWriter, r *http.Request) {
	if h.deliveries == nil {
		respondError(w, http.StatusInternalServerError, "delivery store not initialized")
		return
	}

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
	if h.deliveries == nil {
		respondError(w, http.StatusInternalServerError, "delivery store not initialized")
		return
	}

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
