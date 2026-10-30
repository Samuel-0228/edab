package api

import (
	"log/slog"
	"net/http"
	"regexp"
	"strings"
)

type contextKey string

const requestIDKey contextKey = "request_id"

func getRequestID(r *http.Request) string {
	if r == nil {
		return ""
	}
	if id, ok := r.Context().Value(requestIDKey).(string); ok && id != "" {
		return id
	}
	return r.Header.Get("X-Request-ID")
}

// FieldError represents a specific invalid parameter or payload field.
type FieldError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// APIError is the structured JSON error response returned by Kerob API endpoints.
type APIError struct {
	Error     string       `json:"error"`
	Code      string       `json:"code"`
	RequestID string       `json:"request_id,omitempty"`
	Details   []FieldError `json:"details,omitempty"`
}

func respondErrorWithCode(w http.ResponseWriter, status int, code, message string, details []FieldError) {
	requestID := w.Header().Get("X-Request-ID")
	if status >= http.StatusInternalServerError {
		slog.Error("request failed",
			slog.Int("status", status),
			slog.String("request_id", requestID),
			slog.String("code", code),
			slog.String("error", message),
		)
		message = "internal server error"
		if code == "ERROR" {
			code = "INTERNAL_ERROR"
		}
	}
	resp := APIError{
		Error:     message,
		Code:      code,
		RequestID: requestID,
		Details:   details,
	}
	respondJSON(w, status, resp)
}

func respondValidationError(w http.ResponseWriter, r *http.Request, details []FieldError) {
	reqID := getRequestID(r)
	resp := APIError{
		Error:     "validation failed: one or more fields are invalid",
		Code:      "VALIDATION_ERROR",
		RequestID: reqID,
		Details:   details,
	}
	respondJSON(w, http.StatusBadRequest, resp)
}

var uuidRegex = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

func isValidUUID(u string) bool {
	return uuidRegex.MatchString(strings.TrimSpace(u))
}
