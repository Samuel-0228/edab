package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"kerob/internal/core/auth"
)

// OperationsStream establishes a real-time Server-Sent Events (SSE) stream
// for live workflow execution, step completion, pause/resume, and DLQ lifecycle updates.
func (h *Handlers) OperationsStream(w http.ResponseWriter, r *http.Request) {
	if !h.requirePermission(w, r, auth.PermExecutionsRead) {
		return
	}
	identity := auth.GetIdentity(r.Context())

	flusher, ok := w.(http.Flusher)
	if !ok {
		respondError(w, http.StatusInternalServerError, "streaming unsupported by server")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	tenantID := ""
	if h.multiTenancy && identity != nil {
		tenantID = identity.TenantID
	}

	if h.liveHub == nil {
		h.liveHub = NewLiveHub()
	}

	sub, unsubscribe := h.liveHub.Subscribe(tenantID)
	defer unsubscribe()

	// Initial greeting & reconnection hint
	_, _ = fmt.Fprintf(w, "retry: 3000\n\n")
	connInfo, _ := json.Marshal(map[string]any{
		"status":    "connected",
		"tenant_id": tenantID,
		"time":      time.Now().UTC(),
	})
	_, _ = fmt.Fprintf(w, "event: connected\ndata: %s\n\n", connInfo)
	flusher.Flush()

	// Periodic keepalive ticker to prevent intermediate load balancer timeouts
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			_, _ = fmt.Fprintf(w, ": keepalive %d\n\n", time.Now().Unix())
			flusher.Flush()
		case evt, ok := <-sub.Events:
			if !ok {
				return
			}
			payload, err := json.Marshal(evt)
			if err != nil {
				continue
			}
			_, _ = fmt.Fprintf(w, "id: %d\nevent: %s\ndata: %s\n\n", evt.SequenceID, evt.Type, payload)
			flusher.Flush()
		}
	}
}
