package api

import (
	"context"
	"crypto/subtle"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"
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

	// Health check endpoints
	mux.HandleFunc("GET /health", handlers.Health)
	mux.HandleFunc("GET /ready", handlers.Ready)

	// Sources endpoints
	mux.HandleFunc("GET /sources", handlers.ListSources)
	mux.HandleFunc("POST /sources", handlers.CreateSource)
	mux.HandleFunc("GET /sources/{id}", handlers.GetSource)
	mux.HandleFunc("PUT /sources/{id}", handlers.UpdateSource)
	mux.HandleFunc("PATCH /sources/{id}", handlers.PatchSource)
	mux.HandleFunc("DELETE /sources/{id}", handlers.DeleteSource)
	mux.HandleFunc("GET /sources/{id}/access", handlers.CheckSourceAccess)
	mux.HandleFunc("POST /sources/{id}/sync", handlers.SyncSource)
	mux.HandleFunc("POST /sources/{id}/ingest", handlers.IngestRawSourceEvent)

	// Destinations endpoints
	mux.HandleFunc("GET /destinations", handlers.ListDestinations)
	mux.HandleFunc("POST /destinations", handlers.CreateDestination)
	mux.HandleFunc("GET /destinations/{id}", handlers.GetDestination)
	mux.HandleFunc("PUT /destinations/{id}", handlers.UpdateDestination)
	mux.HandleFunc("PATCH /destinations/{id}", handlers.PatchDestination)
	mux.HandleFunc("DELETE /destinations/{id}", handlers.DeleteDestination)
	mux.HandleFunc("GET /destinations/{id}/access", handlers.CheckDestinationAccess)

	// Routes endpoints
	mux.HandleFunc("GET /routes", handlers.ListRoutes)
	mux.HandleFunc("POST /routes", handlers.CreateRoute)
	mux.HandleFunc("GET /routes/{id}", handlers.GetRoute)
	mux.HandleFunc("PUT /routes/{id}", handlers.UpdateRoute)
	mux.HandleFunc("PATCH /routes/{id}", handlers.PatchRoute)
	mux.HandleFunc("DELETE /routes/{id}", handlers.DeleteRoute)

	// Events endpoints
	mux.HandleFunc("GET /events", handlers.ListEvents)
	mux.HandleFunc("POST /events", handlers.IngestEvent)
	mux.HandleFunc("GET /events/{id}", handlers.GetEvent)

	// Deliveries endpoints
	mux.HandleFunc("GET /deliveries", handlers.ListDeliveries)
	mux.HandleFunc("GET /deliveries/failed", handlers.ListFailedDeliveries)
	mux.HandleFunc("GET /deliveries/stats", handlers.DeliveryStats)
	mux.HandleFunc("GET /deliveries/{id}", handlers.GetDelivery)
	mux.HandleFunc("POST /deliveries/{id}/retry", handlers.RetryDelivery)

	// Management Dashboard UI
	mux.HandleFunc("GET /{$}", handlers.DashboardUI)
	mux.HandleFunc("GET /ui", handlers.DashboardUI)

	loggedHandler := loggingMiddleware(securityMiddleware(authMiddleware(mux, options.APIToken)))

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
		isWebhookIngest := r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/ingest")
		if token == "" || r.URL.Path == "/health" || r.URL.Path == "/ready" || isWebhookIngest {
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

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{
			ResponseWriter: w,
			statusCode:     http.StatusOK,
		}

		next.ServeHTTP(rec, r)

		// Don't spam health check polling in logs
		if r.URL.Path != "/health" && r.URL.Path != "/ready" {
			slog.Info("http request",
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.Int("status", rec.statusCode),
				slog.Duration("duration", time.Since(start)),
				slog.String("remote_addr", r.RemoteAddr),
			)
		}
	})
}
