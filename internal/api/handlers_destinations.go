package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"kerob/internal/adapters/destinations"
	"kerob/internal/telegram"
	"net/http"
)

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
