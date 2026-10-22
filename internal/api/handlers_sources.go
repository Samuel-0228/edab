package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"kerob/internal/adapters/sources"
	"kerob/internal/telegram"
	"net/http"
	"strings"
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

	if valErrs := ValidateSourceInput(&src, h.sourceReg); len(valErrs) > 0 {
		respondValidationError(w, r, valErrs)
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
