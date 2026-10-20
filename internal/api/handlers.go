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
	"kerob/internal/core/event"
	"kerob/internal/core/routing"
	"kerob/internal/core/workflow"
	"kerob/internal/telegram"
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
	telegramAPI  telegram.API
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

// SetTelegramAPI configures the Telegram API client for live access checks.
func (h *Handlers) SetTelegramAPI(api telegram.API) {
	h.telegramAPI = api
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

func (h *Handlers) PatchSource(w http.ResponseWriter, r *http.Request) {
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
		if err := json.Unmarshal(src.Config, &cfg); err != nil {
			respondJSON(w, http.StatusOK, telegram.AccessInfo{Accessible: false, Error: "invalid config: " + err.Error()})
			return
		}
		ch := telegram.NormalizeChannelName(cfg.ChannelUsername)
		scraper := telegram.NewPublicScraper()
		posts, err := scraper.FetchPosts(r.Context(), ch)
		if err != nil {
			respondJSON(w, http.StatusOK, telegram.AccessInfo{
				Accessible: false,
				Title:      "@" + ch,
				Type:       "public_channel",
				Error:      "Failed to access public channel: " + err.Error(),
			})
			return
		}
		respondJSON(w, http.StatusOK, telegram.AccessInfo{
			Accessible: true,
			Title:      "@" + ch,
			Type:       "public_channel",
			Status:     fmt.Sprintf("Reachable (%d recent posts detected)", len(posts)),
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

	var req struct {
		MessageIDs []int `json:"message_ids"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	if len(req.MessageIDs) == 0 {
		respondError(w, http.StatusBadRequest, "message_ids array is required for historical sync")
		return
	}

	var cfg sources.TelegramSourceConfig
	_ = json.Unmarshal(src.Config, &cfg)
	chatID, err := telegram.ParseChatID(cfg.TelegramChatID)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid source chat id: "+err.Error())
		return
	}

	var enqueued int
	for _, msgID := range req.MessageIDs {
		rawPost := telegram.RawPost{
			ChatID: chatID,
			Messages: []json.RawMessage{
				[]byte(fmt.Sprintf(`{"message_id":%d,"chat":{"id":%d}}`, msgID, chatID)),
			},
		}
		rawBytes, _ := json.Marshal(rawPost)
		if _, err := h.pipeline.IngestRaw(r.Context(), src.ID, rawBytes); err == nil {
			enqueued++
		}
	}

	respondJSON(w, http.StatusAccepted, map[string]any{
		"source_id": id,
		"enqueued":  enqueued,
		"total":     len(req.MessageIDs),
	})
}

// IngestRawSourceEvent allows external clients / webhooks to push raw data to a source.
func (h *Handlers) IngestRawSourceEvent(w http.ResponseWriter, r *http.Request) {
	sourceID := r.PathValue("id")
	src, err := h.sources.GetByID(r.Context(), sourceID)
	if errors.Is(err, sources.ErrSourceNotFound) {
		respondError(w, http.StatusNotFound, "source not found")
		return
	}
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to load source")
		return
	}
	if src.AdapterType != "webhook" {
		respondError(w, http.StatusForbidden, "raw ingress is only available for webhook sources")
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 10*1024*1024)) // 10MB limit
	if err != nil {
		respondError(w, http.StatusBadRequest, "failed reading body: "+err.Error())
		return
	}
	if len(body) == 0 {
		respondError(w, http.StatusBadRequest, "empty request body")
		return
	}
	if src.AdapterType == "webhook" {
		if err := sources.ValidateRequest(src.Config, r.Header, body); err != nil {
			respondError(w, http.StatusUnauthorized, err.Error())
			return
		}
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

type destinationInput struct {
	ID             string          `json:"id,omitempty"`
	Name           string          `json:"name"`
	AdapterType    string          `json:"adapter_type,omitempty"`
	TelegramChatID string          `json:"telegram_chat_id,omitempty"`
	Config         json.RawMessage `json:"config,omitempty"`
	Enabled        *bool           `json:"enabled,omitempty"`
}

func (h *Handlers) CreateDestination(w http.ResponseWriter, r *http.Request) {
	var input destinationInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		respondError(w, http.StatusBadRequest, "invalid json body: "+err.Error())
		return
	}
	if input.Name == "" {
		respondError(w, http.StatusBadRequest, "name is required")
		return
	}

	// Automatic convenience detection for Telegram destinations
	if input.AdapterType == "" && input.TelegramChatID != "" {
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

func (h *Handlers) PatchDestination(w http.ResponseWriter, r *http.Request) {
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

func (h *Handlers) PatchRoute(w http.ResponseWriter, r *http.Request) {
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

	var input struct {
		Enabled *bool           `json:"enabled,omitempty"`
		Filters json.RawMessage `json:"filters,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		respondError(w, http.StatusBadRequest, "invalid json body: "+err.Error())
		return
	}

	if input.Enabled != nil {
		route.Enabled = *input.Enabled
	}
	if len(input.Filters) > 0 {
		route.Filters = input.Filters
	}

	updated, err := h.routes.Update(r.Context(), route)
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

func (h *Handlers) ListFailedDeliveries(w http.ResponseWriter, r *http.Request) {
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
	stats, err := h.deliveries.Stats(r.Context())
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, stats)
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

// DashboardUI serves the technical operator interface requested in Requirement 24.
func (h *Handlers) DashboardUI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(dashboardHTML))
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

const dashboardHTML = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>Kerob - Telegram Mirroring Engine</title>
  <style>
    :root {
      --bg: #0d1117;
      --card: #161b22;
      --border: #30363d;
      --text: #c9d1d9;
      --text-muted: #8b949e;
      --accent: #58a6ff;
      --success: #3fb950;
      --warning: #d29922;
      --danger: #f85149;
    }
    * { box-sizing: border-box; margin: 0; padding: 0; }
    body {
      background: var(--bg);
      color: var(--text);
      font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "Helvetica Neue", Arial, monospace;
      padding: 24px;
      line-height: 1.5;
    }
    header {
      display: flex;
      justify-content: space-between;
      align-items: center;
      padding-bottom: 20px;
      border-bottom: 1px solid var(--border);
      margin-bottom: 24px;
    }
    h1 { font-size: 20px; font-weight: 600; letter-spacing: -0.5px; }
    .badge {
      display: inline-block;
      padding: 2px 8px;
      border-radius: 12px;
      font-size: 11px;
      font-weight: 600;
      text-transform: uppercase;
    }
    .badge-ok { background: rgba(63, 185, 80, 0.2); color: var(--success); border: 1px solid var(--success); }
    .badge-fail { background: rgba(248, 81, 73, 0.2); color: var(--danger); border: 1px solid var(--danger); }
    .grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(220px, 1fr)); gap: 16px; margin-bottom: 24px; }
    .stat-card {
      background: var(--card);
      border: 1px solid var(--border);
      border-radius: 6px;
      padding: 16px;
    }
    .stat-label { font-size: 12px; color: var(--text-muted); text-transform: uppercase; }
    .stat-value { font-size: 28px; font-weight: 700; margin-top: 4px; }
    .panels { display: grid; grid-template-columns: 1fr 1fr; gap: 20px; margin-bottom: 24px; }
    @media(max-width: 900px) { .panels { grid-template-columns: 1fr; } }
    .panel {
      background: var(--card);
      border: 1px solid var(--border);
      border-radius: 6px;
      padding: 18px;
    }
    .panel h2 { font-size: 14px; text-transform: uppercase; color: var(--text-muted); margin-bottom: 12px; letter-spacing: 0.5px; }
    table { width: 100%; border-collapse: collapse; font-size: 13px; }
    th { text-align: left; padding: 8px 6px; color: var(--text-muted); border-bottom: 1px solid var(--border); }
    td { padding: 10px 6px; border-bottom: 1px solid var(--border); }
    .btn {
      background: var(--card);
      border: 1px solid var(--border);
      color: var(--accent);
      padding: 4px 10px;
      font-size: 12px;
      border-radius: 4px;
      cursor: pointer;
    }
    .btn:hover { background: var(--border); }
    .btn-danger { color: var(--danger); }
  </style>
</head>
<body>
  <header>
    <div>
      <h1>Kerob Telegram Mirroring Engine</h1>
      <span style="font-size: 12px; color: var(--text-muted);">Multi-Channel Autonomous Channel/Group Synchronizer</span>
    </div>
    <span class="badge badge-ok">System Active</span>
  </header>

  <div class="grid">
    <div class="stat-card">
      <div class="stat-label">Delivered</div>
      <div class="stat-value" id="stat-delivered" style="color: var(--success);">-</div>
    </div>
    <div class="stat-card">
      <div class="stat-label">Pending</div>
      <div class="stat-value" id="stat-pending" style="color: var(--accent);">-</div>
    </div>
    <div class="stat-card">
      <div class="stat-label">Failed</div>
      <div class="stat-value" id="stat-failed" style="color: var(--danger);">-</div>
    </div>
    <div class="stat-card">
      <div class="stat-label">Total Jobs</div>
      <div class="stat-value" id="stat-total">-</div>
    </div>
  </div>

  <div class="panels">
    <div class="panel">
      <h2>Monitored Sources</h2>
      <table>
        <thead><tr><th>Name</th><th>Chat ID</th><th>Status</th></tr></thead>
        <tbody id="sources-table"><tr><td colspan="3">Loading...</td></tr></tbody>
      </table>
    </div>
    <div class="panel">
      <h2>Destinations</h2>
      <table>
        <thead><tr><th>Name</th><th>Chat ID</th><th>Status</th></tr></thead>
        <tbody id="destinations-table"><tr><td colspan="3">Loading...</td></tr></tbody>
      </table>
    </div>
  </div>

  <div class="panels">
    <div class="panel" style="grid-column: 1 / -1;">
      <h2>Active Mirroring Routes</h2>
      <table>
        <thead><tr><th>Route ID</th><th>Source</th><th></th><th>Destination</th><th>Status</th></tr></thead>
        <tbody id="routes-table"><tr><td colspan="5">Loading...</td></tr></tbody>
      </table>
    </div>
  </div>

  <div class="panel">
    <h2>Recent Deliveries</h2>
    <table>
      <thead><tr><th>Job ID</th><th>Event ID</th><th>Destination</th><th>Status</th><th>Attempts</th><th>Action</th></tr></thead>
      <tbody id="deliveries-table"><tr><td colspan="6">Loading...</td></tr></tbody>
    </table>
  </div>

  <script>
    async function refresh() {
      try {
        const [statsRes, srcRes, destRes, routesRes, delivRes] = await Promise.all([
          fetch('/deliveries/stats').then(r => r.json()),
          fetch('/sources').then(r => r.json()),
          fetch('/destinations').then(r => r.json()),
          fetch('/routes').then(r => r.json()),
          fetch('/deliveries?limit=10').then(r => r.json())
        ]);

        document.getElementById('stat-delivered').textContent = statsRes.delivered || 0;
        document.getElementById('stat-pending').textContent = (statsRes.pending || 0) + (statsRes.processing || 0);
        document.getElementById('stat-failed').textContent = (statsRes.failed || 0) + (statsRes.dead_letter || 0);
        document.getElementById('stat-total').textContent = statsRes.total || 0;

        const srcMap = {};
        const srcBody = document.getElementById('sources-table');
        srcBody.innerHTML = srcRes.map(s => {
          srcMap[s.id] = s.name;
          const chatID = s.config?.channel_username ? ('@' + s.config.channel_username) : (s.config?.telegram_chat_id || '-');
          const typeBadge = s.adapter_type === 'telegram_public' ? '<span class="badge" style="border:1px solid var(--accent);color:var(--accent)">Public</span>' : '<span class="badge">Bot</span>';
          return ` + "`" + `<tr>
            <td><strong>${s.name}</strong> ${typeBadge}</td>
            <td><code>${chatID}</code></td>
            <td><span class="badge ${s.enabled ? 'badge-ok' : 'badge-fail'}">${s.enabled ? 'Enabled' : 'Disabled'}</span></td>
          </tr>` + "`" + `;
        }).join('') || '<tr><td colspan="3">No monitored sources</td></tr>';

        const destMap = {};
        const destBody = document.getElementById('destinations-table');
        destBody.innerHTML = destRes.map(d => {
          destMap[d.id] = d.name;
          const chatID = d.config?.telegram_chat_id || '-';
          return ` + "`" + `<tr>
            <td><strong>${d.name}</strong></td>
            <td><code>${chatID}</code></td>
            <td><span class="badge ${d.enabled ? 'badge-ok' : 'badge-fail'}">${d.enabled ? 'Enabled' : 'Disabled'}</span></td>
          </tr>` + "`" + `;
        }).join('') || '<tr><td colspan="3">No destinations</td></tr>';

        const routeBody = document.getElementById('routes-table');
        routeBody.innerHTML = routesRes.map(r => {
          const sName = srcMap[r.source_id] || r.source_id;
          const dName = destMap[r.destination_id] || r.destination_id;
          return ` + "`" + `<tr>
            <td><code>${r.id.substring(0,8)}</code></td>
            <td><strong>${sName}</strong></td>
            <td style="color: var(--accent); font-weight:bold;">&rarr;</td>
            <td><strong>${dName}</strong></td>
            <td><span class="badge ${r.enabled ? 'badge-ok' : 'badge-fail'}">${r.enabled ? 'Active' : 'Paused'}</span></td>
          </tr>` + "`" + `;
        }).join('') || '<tr><td colspan="5">No routes configured</td></tr>';

        const delivBody = document.getElementById('deliveries-table');
        delivBody.innerHTML = delivRes.map(j => {
          const dName = destMap[j.destination_id] || j.destination_id.substring(0,8);
          const isFailed = j.status === 'failed' || j.status === 'dead_letter';
          return ` + "`" + `<tr>
            <td><code>${j.id.substring(0,8)}</code></td>
            <td><code>${j.event_id.substring(0,8)}</code></td>
            <td>${dName}</td>
            <td><span class="badge ${j.status === 'delivered' ? 'badge-ok' : (isFailed ? 'badge-fail' : '')}">${j.status}</span></td>
            <td>${j.attempts} / ${j.max_attempts}</td>
            <td>${isFailed ? '<button class="btn" onclick="retryJob(\'' + j.id + '\')">Retry</button>' : '-'}</td>
          </tr>` + "`" + `;
        }).join('') || '<tr><td colspan="6">No recent deliveries</td></tr>';

      } catch (e) {
        console.error('refresh error', e);
      }
    }

    async function retryJob(id) {
      await fetch('/deliveries/' + id + '/retry', { method: 'POST' });
      refresh();
    }

    refresh();
    setInterval(refresh, 5000);
  </script>
</body>
</html>
`
