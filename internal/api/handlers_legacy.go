package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"kerob/internal/adapters/destinations"
	"kerob/internal/adapters/sources"
	"kerob/internal/core/delivery"
	"kerob/internal/core/routing"
	"kerob/internal/telegram"
)

// Sources handlers
func (h *Handlers) ListSources(w http.ResponseWriter, r *http.Request) {
	if h.sources == nil {
		respondJSON(w, http.StatusOK, []*sources.Source{})
		return
	}
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

type sourceInput struct {
	ID              string          `json:"id,omitempty"`
	Name            string          `json:"name"`
	AdapterType     string          `json:"adapter_type,omitempty"`
	TelegramChatID  string          `json:"telegram_chat_id,omitempty"`
	ChannelUsername string          `json:"channel_username,omitempty"`
	Config          json.RawMessage `json:"config,omitempty"`
	Enabled         *bool           `json:"enabled,omitempty"`
}

func (h *Handlers) CreateSource(w http.ResponseWriter, r *http.Request) {
	if h.sources == nil {
		respondError(w, http.StatusInternalServerError, "source store not initialized")
		return
	}

	var input sourceInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		respondError(w, http.StatusBadRequest, "invalid json body: "+err.Error())
		return
	}
	if input.Name == "" {
		respondError(w, http.StatusBadRequest, "name is required")
		return
	}

	// Automatic convenience detection for Telegram sources
	if input.ChannelUsername != "" {
		input.AdapterType = "telegram_public"
		input.Config = json.RawMessage(fmt.Sprintf(`{"channel_username":%q}`, telegram.NormalizeChannelName(input.ChannelUsername)))
	} else if input.TelegramChatID != "" {
		trimmed := strings.TrimSpace(input.TelegramChatID)
		if strings.HasPrefix(trimmed, "@") || strings.Contains(trimmed, "t.me/") {
			input.AdapterType = "telegram_public"
			input.Config = json.RawMessage(fmt.Sprintf(`{"channel_username":%q}`, telegram.NormalizeChannelName(trimmed)))
		} else if input.AdapterType == "" {
			input.AdapterType = "telegram"
			input.Config = json.RawMessage(fmt.Sprintf(`{"telegram_chat_id":%q}`, trimmed))
		}
	}

	if input.AdapterType == "" {
		respondError(w, http.StatusBadRequest, "adapter_type or telegram_chat_id is required")
		return
	}
	if len(input.Config) == 0 {
		input.Config = json.RawMessage(`{}`)
	}
	enabled := true
	if input.Enabled != nil {
		enabled = *input.Enabled
	}

	src := sources.Source{
		ID:          input.ID,
		Name:        input.Name,
		AdapterType: input.AdapterType,
		Config:      input.Config,
		Enabled:     enabled,
	}

	created, err := h.sources.Create(r.Context(), &src)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusCreated, created)
}

func (h *Handlers) GetSource(w http.ResponseWriter, r *http.Request) {
	if h.sources == nil {
		respondError(w, http.StatusInternalServerError, "source store not initialized")
		return
	}

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
	if h.sources == nil {
		respondError(w, http.StatusInternalServerError, "source store not initialized")
		return
	}

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

func (h *Handlers) PatchSource(w http.ResponseWriter, r *http.Request) {
	if h.sources == nil {
		respondError(w, http.StatusInternalServerError, "source store not initialized")
		return
	}

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

	var input struct {
		Name            *string         `json:"name,omitempty"`
		Enabled         *bool           `json:"enabled,omitempty"`
		TelegramChatID  *string         `json:"telegram_chat_id,omitempty"`
		ChannelUsername *string         `json:"channel_username,omitempty"`
		Config          json.RawMessage `json:"config,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		respondError(w, http.StatusBadRequest, "invalid json body: "+err.Error())
		return
	}

	if input.Name != nil {
		src.Name = *input.Name
	}
	if input.Enabled != nil {
		src.Enabled = *input.Enabled
	}
	if input.ChannelUsername != nil {
		src.AdapterType = "telegram_public"
		src.Config = json.RawMessage(fmt.Sprintf(`{"channel_username":%q}`, telegram.NormalizeChannelName(*input.ChannelUsername)))
	} else if input.TelegramChatID != nil {
		trimmed := strings.TrimSpace(*input.TelegramChatID)
		if strings.HasPrefix(trimmed, "@") || strings.Contains(trimmed, "t.me/") {
			src.AdapterType = "telegram_public"
			src.Config = json.RawMessage(fmt.Sprintf(`{"channel_username":%q}`, telegram.NormalizeChannelName(trimmed)))
		} else {
			src.Config = json.RawMessage(fmt.Sprintf(`{"telegram_chat_id":%q}`, trimmed))
		}
	} else if len(input.Config) > 0 {
		src.Config = input.Config
	}

	updated, err := h.sources.Update(r.Context(), src)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, updated)
}

func (h *Handlers) DeleteSource(w http.ResponseWriter, r *http.Request) {
	if h.sources == nil {
		respondError(w, http.StatusInternalServerError, "source store not initialized")
		return
	}

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

func (h *Handlers) CheckSourceAccess(w http.ResponseWriter, r *http.Request) {
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

	if src.AdapterType == "telegram_public" {
		var cfg sources.TelegramPublicSourceConfig
		_ = json.Unmarshal(src.Config, &cfg)
		respondJSON(w, http.StatusOK, map[string]any{
			"accessible":       true,
			"type":             "telegram_public",
			"channel_username": cfg.ChannelUsername,
			"status":           "public_viewable",
		})
		return
	}

	if src.AdapterType != "telegram" {
		respondJSON(w, http.StatusOK, map[string]any{"accessible": true, "type": src.AdapterType})
		return
	}
	if h.telegramAPI == nil {
		respondJSON(w, http.StatusOK, telegram.AccessInfo{Accessible: false, Error: "telegram api client not initialized"})
		return
	}

	var cfg sources.TelegramSourceConfig
	if err := json.Unmarshal(src.Config, &cfg); err != nil {
		respondJSON(w, http.StatusOK, telegram.AccessInfo{Accessible: false, Error: "invalid config: " + err.Error()})
		return
	}
	chatID, err := telegram.ParseChatID(cfg.TelegramChatID)
	if err != nil {
		respondJSON(w, http.StatusOK, telegram.AccessInfo{Accessible: false, Error: err.Error()})
		return
	}

	info := telegram.CheckAccess(h.telegramAPI, chatID)
	respondJSON(w, http.StatusOK, info)
}

func (h *Handlers) SyncSource(w http.ResponseWriter, r *http.Request) {
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

	if !src.Enabled {
		respondError(w, http.StatusBadRequest, "cannot sync disabled source")
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{
		"status":  "synchronized",
		"message": "source triggers active",
	})
}

func (h *Handlers) IngestRawSourceEvent(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	data, err := io.ReadAll(io.LimitReader(r.Body, 1024*1024))
	if err != nil {
		respondError(w, http.StatusBadRequest, "failed to read request body: "+err.Error())
		return
	}
	if len(data) == 0 {
		respondError(w, http.StatusBadRequest, "body cannot be empty")
		return
	}

	ev, err := h.pipeline.IngestRaw(r.Context(), id, data)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	respondJSON(w, http.StatusAccepted, ev)
}

// Destinations handlers
func (h *Handlers) ListDestinations(w http.ResponseWriter, r *http.Request) {
	if h.destinations == nil {
		respondJSON(w, http.StatusOK, []*destinations.Destination{})
		return
	}
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

type destinationInput struct {
	ID             string          `json:"id,omitempty"`
	Name           string          `json:"name"`
	AdapterType    string          `json:"adapter_type,omitempty"`
	TelegramChatID string          `json:"telegram_chat_id,omitempty"`
	Config         json.RawMessage `json:"config,omitempty"`
	Enabled        *bool           `json:"enabled,omitempty"`
}

func (h *Handlers) CreateDestination(w http.ResponseWriter, r *http.Request) {
	if h.destinations == nil {
		respondError(w, http.StatusInternalServerError, "destination store not initialized")
		return
	}

	var input destinationInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		respondError(w, http.StatusBadRequest, "invalid json body: "+err.Error())
		return
	}
	if input.Name == "" {
		respondError(w, http.StatusBadRequest, "name is required")
		return
	}

	if input.TelegramChatID != "" && input.AdapterType == "" {
		input.AdapterType = "telegram"
		input.Config = json.RawMessage(fmt.Sprintf(`{"telegram_chat_id":%q}`, input.TelegramChatID))
	}
	if input.AdapterType == "" {
		respondError(w, http.StatusBadRequest, "adapter_type or telegram_chat_id is required")
		return
	}
	if len(input.Config) == 0 {
		input.Config = json.RawMessage(`{}`)
	}
	enabled := true
	if input.Enabled != nil {
		enabled = *input.Enabled
	}

	dest := destinations.Destination{
		ID:          input.ID,
		Name:        input.Name,
		AdapterType: input.AdapterType,
		Config:      input.Config,
		Enabled:     enabled,
	}

	created, err := h.destinations.Create(r.Context(), &dest)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusCreated, created)
}

func (h *Handlers) GetDestination(w http.ResponseWriter, r *http.Request) {
	if h.destinations == nil {
		respondError(w, http.StatusInternalServerError, "destination store not initialized")
		return
	}

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
	if h.destinations == nil {
		respondError(w, http.StatusInternalServerError, "destination store not initialized")
		return
	}

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

func (h *Handlers) PatchDestination(w http.ResponseWriter, r *http.Request) {
	if h.destinations == nil {
		respondError(w, http.StatusInternalServerError, "destination store not initialized")
		return
	}

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

	var input struct {
		Name           *string         `json:"name,omitempty"`
		Enabled        *bool           `json:"enabled,omitempty"`
		TelegramChatID *string         `json:"telegram_chat_id,omitempty"`
		Config         json.RawMessage `json:"config,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		respondError(w, http.StatusBadRequest, "invalid json body: "+err.Error())
		return
	}

	if input.Name != nil {
		dest.Name = *input.Name
	}
	if input.Enabled != nil {
		dest.Enabled = *input.Enabled
	}
	if input.TelegramChatID != nil {
		dest.Config = json.RawMessage(fmt.Sprintf(`{"telegram_chat_id":%q}`, *input.TelegramChatID))
	} else if len(input.Config) > 0 {
		dest.Config = input.Config
	}

	updated, err := h.destinations.Update(r.Context(), dest)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, updated)
}

func (h *Handlers) DeleteDestination(w http.ResponseWriter, r *http.Request) {
	if h.destinations == nil {
		respondError(w, http.StatusInternalServerError, "destination store not initialized")
		return
	}

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

func (h *Handlers) CheckDestinationAccess(w http.ResponseWriter, r *http.Request) {
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

	if dest.AdapterType != "telegram" {
		respondJSON(w, http.StatusOK, map[string]any{"accessible": true, "type": dest.AdapterType})
		return
	}
	if h.telegramAPI == nil {
		respondJSON(w, http.StatusOK, telegram.AccessInfo{Accessible: false, Error: "telegram api client not initialized"})
		return
	}

	var cfg destinations.TelegramDestinationConfig
	if err := json.Unmarshal(dest.Config, &cfg); err != nil {
		respondJSON(w, http.StatusOK, telegram.AccessInfo{Accessible: false, Error: "invalid config: " + err.Error()})
		return
	}
	chatID, err := telegram.ParseChatID(cfg.TelegramChatID)
	if err != nil {
		respondJSON(w, http.StatusOK, telegram.AccessInfo{Accessible: false, Error: err.Error()})
		return
	}

	info := telegram.CheckAccess(h.telegramAPI, chatID)
	respondJSON(w, http.StatusOK, info)
}

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
