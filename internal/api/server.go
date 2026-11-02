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
	"kerob/internal/core/auth"
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
	APIToken            string
	AuthStore           auth.Store
	MultiTenancyEnabled bool
	DefaultTenantID     string
}

func NewServerWithOptions(port string, handlers *Handlers, options ServerOptions) *Server {
	mux := http.NewServeMux()

	// Health and Observability
	mux.HandleFunc("GET /health", handlers.Health)
	mux.HandleFunc("GET /ready", handlers.Ready)
	mux.HandleFunc("GET /metrics", handlers.Metrics)

	// Auth, Multi-Tenancy & RBAC
	mux.HandleFunc("GET /api/v1/auth/me", handlers.Me)
	mux.HandleFunc("GET /api/v1/tenants", handlers.ListTenants)
	mux.HandleFunc("POST /api/v1/tenants", handlers.CreateTenant)
	mux.HandleFunc("GET /api/v1/tenants/{id}", handlers.GetTenant)

	mux.HandleFunc("GET /api/v1/members", handlers.ListMembers)
	mux.HandleFunc("POST /api/v1/members", handlers.CreateMember)
	mux.HandleFunc("PATCH /api/v1/members/{id}", handlers.UpdateMemberRole)
	mux.HandleFunc("DELETE /api/v1/members/{id}", handlers.DeleteMember)

	mux.HandleFunc("GET /api/v1/tenants/current/members", handlers.ListMembers)
	mux.HandleFunc("POST /api/v1/tenants/current/members", handlers.CreateMember)
	mux.HandleFunc("PATCH /api/v1/tenants/current/members/{id}", handlers.UpdateMemberRole)
	mux.HandleFunc("PUT /api/v1/tenants/current/members/{id}", handlers.UpdateMemberRole)
	mux.HandleFunc("DELETE /api/v1/tenants/current/members/{id}", handlers.DeleteMember)

	mux.HandleFunc("GET /api/v1/tokens", handlers.ListAPITokens)
	mux.HandleFunc("POST /api/v1/tokens", handlers.CreateAPIToken)
	mux.HandleFunc("DELETE /api/v1/tokens/{id}", handlers.RevokeAPIToken)

	mux.HandleFunc("GET /api/v1/audit-logs", handlers.ListAuditLogs)

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
	mux.HandleFunc("GET /api/v1/workflows/{id}/versions", handlers.ListWorkflowVersions)
	mux.HandleFunc("GET /api/v1/workflows/{id}/versions/{version}", handlers.GetWorkflowVersion)
	mux.HandleFunc("POST /api/v1/workflows/{id}/rollback/{version}", handlers.RollbackWorkflow)
	mux.HandleFunc("GET /api/v1/workflows/{id}/diff", handlers.DiffWorkflowVersions)
	mux.HandleFunc("GET /api/v1/workflows/{id}/export", handlers.ExportWorkflow)
	mux.HandleFunc("POST /api/v1/workflows/import", handlers.ImportWorkflow)
	mux.HandleFunc("GET /api/v1/templates", handlers.ListTemplates)

	// Integrations Catalog
	mux.HandleFunc("GET /api/v1/integrations", handlers.ListIntegrations)
	mux.HandleFunc("GET /api/v1/integrations/{id}", handlers.GetIntegration)
	mux.HandleFunc("GET /api/v1/integrations/{id}/schema", handlers.GetIntegrationSchema)

	// Connector SDK Catalog & Dynamic Health
	mux.HandleFunc("GET /api/v1/connectors", handlers.ListConnectors)
	mux.HandleFunc("GET /api/v1/connectors/{id}", handlers.GetConnector)
	mux.HandleFunc("GET /api/v1/connectors/{id}/health", handlers.CheckConnectorHealth)

	// OAuth2 Authorization & Token Lifecycle
	mux.HandleFunc("GET /api/v1/oauth/{connector}/authorize", handlers.AuthorizeOAuth)
	mux.HandleFunc("GET /api/v1/oauth/{connector}/callback", handlers.CallbackOAuth)
	mux.HandleFunc("POST /api/v1/oauth/{connector}/callback", handlers.CallbackOAuth)
	mux.HandleFunc("POST /api/v1/oauth/{connector}/refresh", handlers.RefreshOAuth)
	mux.HandleFunc("POST /api/v1/oauth/{connector}/revoke", handlers.RevokeOAuth)

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
	mux.HandleFunc("POST /api/v1/executions/{id}/cancel", handlers.CancelExecution)
	mux.HandleFunc("POST /api/v1/executions/{id}/pause", handlers.PauseExecution)
	mux.HandleFunc("POST /api/v1/executions/{id}/resume", handlers.ResumeExecution)
	mux.HandleFunc("POST /api/v1/executions/{id}/compensate", handlers.CompensateExecution)

	// Sagas & Human Approvals
	mux.HandleFunc("GET /api/v1/approvals", handlers.ListApprovals)
	mux.HandleFunc("GET /api/v1/approvals/{id}", handlers.GetApproval)
	mux.HandleFunc("POST /api/v1/approvals/{id}/approve", handlers.ApproveApproval)
	mux.HandleFunc("POST /api/v1/approvals/{id}/reject", handlers.RejectApproval)
	mux.HandleFunc("GET /api/v1/approval-tokens/{token}", handlers.GetApprovalByToken)
	mux.HandleFunc("POST /api/v1/approval-tokens/{token}", handlers.DecideApprovalByToken)

	// Real-time Operations Streaming
	mux.HandleFunc("GET /api/v1/operations/stream", handlers.OperationsStream)

	// Schema Registry
	mux.HandleFunc("POST /api/v1/schemas", handlers.RegisterSchema)
	mux.HandleFunc("GET /api/v1/schemas", handlers.ListSchemas)
	mux.HandleFunc("GET /api/v1/schemas/{name}", handlers.GetSchema)
	mux.HandleFunc("GET /api/v1/schemas/{name}/versions", handlers.ListSchemaVersions)
	mux.HandleFunc("GET /api/v1/schemas/{name}/versions/{version}", handlers.GetSchemaVersion)
	mux.HandleFunc("POST /api/v1/schemas/{name}/validate", handlers.ValidatePayloadAgainstSchema)

	// Partitioned Event Streaming & Replay
	mux.HandleFunc("POST /api/v1/streams/publish", handlers.PublishStream)
	mux.HandleFunc("GET /api/v1/streams/offsets", handlers.GetStreamOffsets)
	mux.HandleFunc("POST /api/v1/streams/replay", handlers.ReplayStream)
	mux.HandleFunc("GET /api/v1/streams/dlq", handlers.ListStreamDLQ)
	mux.HandleFunc("POST /api/v1/streams/dlq/{id}/retry", handlers.RetryStreamDLQ)

	// Failures / Dead-Letter Queue
	mux.HandleFunc("GET /api/v1/failures", handlers.ListFailures)
	mux.HandleFunc("GET /api/v1/failures/{id}", handlers.GetFailure)
	mux.HandleFunc("POST /api/v1/failures/{id}/discard", handlers.DiscardFailure)
	mux.HandleFunc("POST /api/v1/failures/{id}/retry", handlers.RetryFailure)
	mux.HandleFunc("POST /api/v1/failures/retry-bulk", handlers.BulkRetryDeadLetters)
	mux.HandleFunc("POST /api/v1/failures/discard-bulk", handlers.BulkDiscardDeadLetters)
	mux.HandleFunc("POST /api/v1/dead-letter/retry-bulk", handlers.BulkRetryDeadLetters)
	mux.HandleFunc("POST /api/v1/dead-letter/discard-bulk", handlers.BulkDiscardDeadLetters)

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
	mux.HandleFunc("POST /api/v1/events/{id}/replay", handlers.ReplayEvent)
	mux.HandleFunc("POST /api/v1/events/replay-bulk", handlers.ReplayBulkEvents)

	// Sources API
	mux.HandleFunc("GET /api/v1/sources", handlers.ListSources)
	mux.HandleFunc("POST /api/v1/sources", handlers.CreateSource)
	mux.HandleFunc("GET /api/v1/sources/{id}", handlers.GetSource)
	mux.HandleFunc("PUT /api/v1/sources/{id}", handlers.UpdateSource)
	mux.HandleFunc("PATCH /api/v1/sources/{id}", handlers.PatchSource)
	mux.HandleFunc("DELETE /api/v1/sources/{id}", handlers.DeleteSource)
	mux.HandleFunc("GET /api/v1/sources/{id}/access", handlers.CheckSourceAccess)
	mux.HandleFunc("POST /api/v1/sources/{id}/sync", handlers.SyncSource)
	mux.HandleFunc("POST /api/v1/sources/{id}/ingest", handlers.IngestRawSourceEvent)

	// Destinations API
	mux.HandleFunc("GET /api/v1/destinations", handlers.ListDestinations)
	mux.HandleFunc("POST /api/v1/destinations", handlers.CreateDestination)
	mux.HandleFunc("GET /api/v1/destinations/{id}", handlers.GetDestination)
	mux.HandleFunc("PUT /api/v1/destinations/{id}", handlers.UpdateDestination)
	mux.HandleFunc("PATCH /api/v1/destinations/{id}", handlers.PatchDestination)
	mux.HandleFunc("DELETE /api/v1/destinations/{id}", handlers.DeleteDestination)
	mux.HandleFunc("GET /api/v1/destinations/{id}/access", handlers.CheckDestinationAccess)

	// Routes API
	mux.HandleFunc("GET /api/v1/routes", handlers.ListRoutes)
	mux.HandleFunc("POST /api/v1/routes", handlers.CreateRoute)
	mux.HandleFunc("GET /api/v1/routes/{id}", handlers.GetRoute)
	mux.HandleFunc("PUT /api/v1/routes/{id}", handlers.UpdateRoute)
	mux.HandleFunc("PATCH /api/v1/routes/{id}", handlers.PatchRoute)
	mux.HandleFunc("DELETE /api/v1/routes/{id}", handlers.DeleteRoute)

	// Deliveries API
	mux.HandleFunc("GET /api/v1/deliveries", handlers.ListDeliveries)
	mux.HandleFunc("GET /api/v1/deliveries/failed", handlers.ListFailedDeliveries)
	mux.HandleFunc("GET /api/v1/deliveries/stats", handlers.DeliveryStats)
	mux.HandleFunc("GET /api/v1/deliveries/{id}", handlers.GetDelivery)
	mux.HandleFunc("POST /api/v1/deliveries/{id}/retry", handlers.RetryDelivery)

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

	loggedHandler := requestIDMiddleware(loggingMiddleware(securityMiddleware(authMiddleware(mux, options))))

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

func authMiddleware(next http.Handler, options ServerOptions) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		isWebhookIngest := r.Method == http.MethodPost && (strings.HasPrefix(r.URL.Path, "/webhooks/") || strings.HasSuffix(r.URL.Path, "/ingest"))
		isUI := r.URL.Path == "/" || r.URL.Path == "/ui" || strings.HasPrefix(r.URL.Path, "/ui/") || r.URL.Path == "/kerob_logo.png" || r.URL.Path == "/favicon.ico"
		isHealth := r.URL.Path == "/health" || r.URL.Path == "/ready" || r.URL.Path == "/metrics"

		if isHealth || isUI || isWebhookIngest {
			next.ServeHTTP(w, r)
			return
		}

		provided := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
		if provided == "" {
			provided = strings.TrimSpace(r.URL.Query().Get("token"))
		}
		requestedTenant := strings.TrimSpace(r.Header.Get("X-Tenant-ID"))
		if requestedTenant == "" {
			requestedTenant = options.DefaultTenantID
			if requestedTenant == "" {
				requestedTenant = "default"
			}
		}

		// 1. Single-tenant development mode (multi-tenancy disabled)
		if !options.MultiTenancyEnabled {
			// If a database API token was explicitly provided in dev mode, authenticate it
			if options.AuthStore != nil && strings.HasPrefix(provided, "krb_") {
				tokenHash := auth.HashToken(provided)
				tok, err := options.AuthStore.GetAPITokenByHash(r.Context(), tokenHash)
				if err == nil && tok != nil {
					_ = options.AuthStore.TouchAPIToken(r.Context(), tok.ID)
					projID := ""
					if tok.ProjectID != nil {
						projID = *tok.ProjectID
					}
					ctx := auth.WithIdentity(r.Context(), &auth.Identity{
						TenantID:  tok.TenantID,
						ProjectID: projID,
						UserID:    tok.Name,
						ActorType: "api_token",
						Role:      tok.Role,
						Scopes:    tok.Scopes,
						IsAuth:    true,
					})
					next.ServeHTTP(w, r.WithContext(ctx))
					return
				}
			}

			// If static API_TOKEN is required in single-tenant mode
			if options.APIToken != "" {
				if provided == "" || subtle.ConstantTimeCompare([]byte(provided), []byte(options.APIToken)) != 1 {
					w.Header().Set("WWW-Authenticate", `Bearer realm="kerob"`)
					respondError(w, http.StatusUnauthorized, "authentication required")
					return
				}
			}

			// Default development identity with owner permissions
			ctx := auth.WithIdentity(r.Context(), &auth.Identity{
				TenantID:  requestedTenant,
				UserID:    "default-user",
				ActorType: "system",
				Role:      auth.RoleOwner,
				IsAuth:    true,
			})
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}

		// 2. Multi-tenancy enabled: strict authentication required
		if provided == "" {
			w.Header().Set("WWW-Authenticate", `Bearer realm="kerob"`)
			respondErrorWithCode(w, http.StatusUnauthorized, "UNAUTHORIZED", "authentication token required", nil)
			return
		}

		// Check static master token (acts as system Owner across tenants)
		if options.APIToken != "" && subtle.ConstantTimeCompare([]byte(provided), []byte(options.APIToken)) == 1 {
			ctx := auth.WithIdentity(r.Context(), &auth.Identity{
				TenantID:  requestedTenant,
				UserID:    "admin",
				ActorType: "system",
				Role:      auth.RoleOwner,
				IsAuth:    true,
			})
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}

		// Check database API tokens
		if options.AuthStore != nil && strings.HasPrefix(provided, "krb_") {
			tokenHash := auth.HashToken(provided)
			tok, err := options.AuthStore.GetAPITokenByHash(r.Context(), tokenHash)
			if err != nil || tok == nil {
				w.Header().Set("WWW-Authenticate", `Bearer realm="kerob"`)
				respondErrorWithCode(w, http.StatusUnauthorized, "INVALID_TOKEN", "invalid or expired api token", nil)
				return
			}

			_ = options.AuthStore.TouchAPIToken(r.Context(), tok.ID)
			projID := ""
			if tok.ProjectID != nil {
				projID = *tok.ProjectID
			}
			ctx := auth.WithIdentity(r.Context(), &auth.Identity{
				TenantID:  tok.TenantID,
				ProjectID: projID,
				UserID:    tok.Name,
				ActorType: "api_token",
				Role:      tok.Role,
				Scopes:    tok.Scopes,
				IsAuth:    true,
			})
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}

		w.Header().Set("WWW-Authenticate", `Bearer realm="kerob"`)
		respondErrorWithCode(w, http.StatusUnauthorized, "UNAUTHORIZED", "invalid authentication credentials", nil)
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

func (rec *statusRecorder) Flush() {
	if f, ok := rec.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (rec *statusRecorder) Unwrap() http.ResponseWriter {
	return rec.ResponseWriter
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
