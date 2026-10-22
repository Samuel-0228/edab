package api

import (
	"context"
	"crypto/subtle"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
)

type Server struct {
	httpServer *http.Server
	mux        *http.ServeMux
}

func NewServer(port string, handlers *Handlers) *Server {
	return NewServerWithOptions(port, handlers, ServerOptions{})
}

// ServerOptions controls deployment-sensitive HTTP protections.
type ServerOptions struct {
	APIToken string
}

func NewServerWithOptions(port string, handlers *Handlers, options ServerOptions) *Server {
	mux := http.NewServeMux()

	// Health and Observability
	mux.HandleFunc("GET /health", handlers.Health)
	mux.HandleFunc("GET /ready", handlers.Ready)
	mux.HandleFunc("GET /metrics", handlers.Metrics)

	// Dashboard UI & Static Assets
	mux.HandleFunc("GET /{$}", handlers.DashboardUI)
	mux.HandleFunc("GET /ui", handlers.DashboardUI)
	mux.HandleFunc("GET /ui/{path...}", handlers.DashboardUI)
	mux.HandleFunc("GET /kerob_logo.png", handlers.DashboardLogo)
	mux.HandleFunc("GET /favicon.ico", handlers.DashboardLogo)

	// Overview API
	mux.HandleFunc("GET /api/v1/overview", handlers.Overview)
	mux.HandleFunc("GET /api/v1/stats", handlers.Overview)

	// Workflows API
	mux.HandleFunc("GET /api/v1/workflows", handlers.ListWorkflows)
	mux.HandleFunc("POST /api/v1/workflows", handlers.CreateWorkflow)
	mux.HandleFunc("GET /api/v1/workflows/{id}", handlers.GetWorkflow)
	mux.HandleFunc("PUT /api/v1/workflows/{id}", handlers.UpdateWorkflow)
	mux.HandleFunc("PATCH /api/v1/workflows/{id}", handlers.PatchWorkflow)
	mux.HandleFunc("DELETE /api/v1/workflows/{id}", handlers.DeleteWorkflow)
	mux.HandleFunc("POST /api/v1/workflows/{id}/test", handlers.TestWorkflow)

	// Integrations Catalog
	mux.HandleFunc("GET /api/v1/integrations", handlers.ListIntegrations)
	mux.HandleFunc("GET /api/v1/integrations/{id}", handlers.GetIntegration)

	// Credentials API (AES-256 encrypted at rest, never exposes raw secrets)
	mux.HandleFunc("GET /api/v1/credentials", handlers.ListCredentials)
	mux.HandleFunc("POST /api/v1/credentials", handlers.CreateCredential)
	mux.HandleFunc("GET /api/v1/credentials/{id}", handlers.GetCredential)
	mux.HandleFunc("PUT /api/v1/credentials/{id}", handlers.UpdateCredential)
	mux.HandleFunc("DELETE /api/v1/credentials/{id}", handlers.DeleteCredential)

	// Executions API
	mux.HandleFunc("GET /api/v1/executions", handlers.ListExecutions)
	mux.HandleFunc("GET /api/v1/executions/{id}", handlers.GetExecution)
	mux.HandleFunc("POST /api/v1/executions/{id}/retry", handlers.RetryExecution)

	// Failures / Dead-Letter Queue
	mux.HandleFunc("GET /api/v1/failures", handlers.ListFailures)
	mux.HandleFunc("GET /api/v1/failures/{id}", handlers.GetFailure)
	mux.HandleFunc("POST /api/v1/failures/{id}/discard", handlers.DiscardFailure)
	mux.HandleFunc("POST /api/v1/failures/{id}/retry", handlers.RetryFailure)

	// Webhooks Endpoints
	mux.HandleFunc("GET /api/v1/webhooks", handlers.ListWebhooks)
	mux.HandleFunc("POST /api/v1/webhooks", handlers.CreateWebhook)
	mux.HandleFunc("GET /api/v1/webhooks/{id}", handlers.GetWebhook)
	mux.HandleFunc("PUT /api/v1/webhooks/{id}", handlers.UpdateWebhook)
	mux.HandleFunc("DELETE /api/v1/webhooks/{id}", handlers.DeleteWebhook)
	mux.HandleFunc("POST /webhooks/{id}", handlers.ReceiveWebhook)
	mux.HandleFunc("POST /api/v1/webhooks/{id}/ingest", handlers.ReceiveWebhook)

	// Events API
	mux.HandleFunc("GET /api/v1/events", handlers.ListEvents)
	mux.HandleFunc("POST /api/v1/events", handlers.IngestEvent)
	mux.HandleFunc("GET /api/v1/events/{id}", handlers.GetEvent)

	// Legacy routes for backwards compatibility
	mux.HandleFunc("GET /sources", handlers.ListSources)
	mux.HandleFunc("POST /sources", handlers.CreateSource)
	mux.HandleFunc("GET /sources/{id}", handlers.GetSource)
	mux.HandleFunc("PUT /sources/{id}", handlers.UpdateSource)
	mux.HandleFunc("PATCH /sources/{id}", handlers.PatchSource)
	mux.HandleFunc("DELETE /sources/{id}", handlers.DeleteSource)
	mux.HandleFunc("GET /sources/{id}/access", handlers.CheckSourceAccess)
	mux.HandleFunc("POST /sources/{id}/sync", handlers.SyncSource)
	mux.HandleFunc("POST /sources/{id}/ingest", handlers.IngestRawSourceEvent)

	mux.HandleFunc("GET /destinations", handlers.ListDestinations)
	mux.HandleFunc("POST /destinations", handlers.CreateDestination)
	mux.HandleFunc("GET /destinations/{id}", handlers.GetDestination)
	mux.HandleFunc("PUT /destinations/{id}", handlers.UpdateDestination)
	mux.HandleFunc("PATCH /destinations/{id}", handlers.PatchDestination)
	mux.HandleFunc("DELETE /destinations/{id}", handlers.DeleteDestination)
	mux.HandleFunc("GET /destinations/{id}/access", handlers.CheckDestinationAccess)

	mux.HandleFunc("GET /routes", handlers.ListRoutes)
	mux.HandleFunc("POST /routes", handlers.CreateRoute)
	mux.HandleFunc("GET /routes/{id}", handlers.GetRoute)
	mux.HandleFunc("PUT /routes/{id}", handlers.UpdateRoute)
	mux.HandleFunc("PATCH /routes/{id}", handlers.PatchRoute)
	mux.HandleFunc("DELETE /routes/{id}", handlers.DeleteRoute)

	mux.HandleFunc("GET /events", handlers.ListEvents)
	mux.HandleFunc("POST /events", handlers.IngestEvent)
	mux.HandleFunc("GET /events/{id}", handlers.GetEvent)

	mux.HandleFunc("GET /deliveries", handlers.ListDeliveries)
	mux.HandleFunc("GET /deliveries/failed", handlers.ListFailedDeliveries)
	mux.HandleFunc("GET /deliveries/stats", handlers.DeliveryStats)
	mux.HandleFunc("GET /deliveries/{id}", handlers.GetDelivery)
	mux.HandleFunc("POST /deliveries/{id}/retry", handlers.RetryDelivery)

	loggedHandler := requestIDMiddleware(loggingMiddleware(securityMiddleware(authMiddleware(mux, options.APIToken))))

	return &Server{
		httpServer: &http.Server{
			Addr:         ":" + port,
			Handler:      loggedHandler,
			ReadTimeout:  15 * time.Second,
			WriteTimeout: 15 * time.Second,
			IdleTimeout:  60 * time.Second,
		},
		mux: mux,
	}
}

func authMiddleware(next http.Handler, token string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		isWebhookIngest := r.Method == http.MethodPost && (strings.HasPrefix(r.URL.Path, "/webhooks/") || strings.HasSuffix(r.URL.Path, "/ingest"))
		isUI := r.URL.Path == "/" || r.URL.Path == "/ui" || strings.HasPrefix(r.URL.Path, "/ui/")
		isHealth := r.URL.Path == "/health" || r.URL.Path == "/ready" || r.URL.Path == "/metrics"

		if token == "" || isHealth || isUI || isWebhookIngest {
			next.ServeHTTP(w, r)
			return
		}

		provided := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
		if provided == "" || subtle.ConstantTimeCompare([]byte(provided), []byte(token)) != 1 {
			w.Header().Set("WWW-Authenticate", `Bearer realm="kerob"`)
			respondError(w, http.StatusUnauthorized, "authentication required")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func securityMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}

func (s *Server) Start() error {
	slog.Info("REST API server listening", slog.String("addr", s.httpServer.Addr))
	if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("http server failure: %w", err)
	}
	return nil
}

func (s *Server) Shutdown(ctx context.Context) error {
	slog.Info("shutting down REST API server")
	return s.httpServer.Shutdown(ctx)
}

func (s *Server) Handler() http.Handler {
	return s.httpServer.Handler
}

type statusRecorder struct {
	http.ResponseWriter
	statusCode int
}

func (rec *statusRecorder) WriteHeader(code int) {
	rec.statusCode = code
	rec.ResponseWriter.WriteHeader(code)
}

func requestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqID := r.Header.Get("X-Request-ID")
		if reqID == "" {
			reqID = uuid.NewString()
		}
		w.Header().Set("X-Request-ID", reqID)
		ctx := context.WithValue(r.Context(), requestIDKey, reqID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{
			ResponseWriter: w,
			statusCode:     http.StatusOK,
		}
		next.ServeHTTP(rec, r)
		duration := time.Since(start)

		slog.Info("http request",
			slog.String("request_id", getRequestID(r)),
			slog.String("method", r.Method),
			slog.String("path", r.URL.Path),
			slog.Int("status", rec.statusCode),
			slog.Duration("duration", duration),
			slog.String("remote_addr", r.RemoteAddr),
		)
	})
}
