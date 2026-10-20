package api

import (
	"net/http"
	"time"

	"kerob/internal/adapters"
	"kerob/internal/core/delivery"
	"kerob/internal/core/workflow"
)

type OverviewResponse struct {
	Status               string                         `json:"status"`
	ActiveWorkflows      int                            `json:"active_workflows"`
	TotalWorkflows       int                            `json:"total_workflows"`
	ExecutionsTotal      int                            `json:"executions_total"`
	ExecutionsSuccess    int                            `json:"executions_success"`
	ExecutionsFailed     int                            `json:"executions_failed"`
	SuccessRate          float64                        `json:"success_rate"`
	DeadLetterCount      int                            `json:"dead_letter_count"`
	ConnectedCredentials int                            `json:"connected_credentials"`
	RegisteredAdapters   int                            `json:"registered_adapters"`
	RecentExecutions     []*workflow.Execution          `json:"recent_executions"`
	Catalog              []adapters.IntegrationMetadata `json:"catalog"`
	UptimeSeconds        int64                          `json:"uptime_seconds"`
}

var serverStartTime = time.Now()

func (h *Handlers) Overview(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	totalWorkflows := 0
	activeWorkflows := 0
	if h.workflows != nil {
		if all, err := h.workflows.List(ctx); err == nil {
			totalWorkflows = len(all)
			for _, wf := range all {
				if wf.Enabled {
					activeWorkflows++
				}
			}
		}
	}

	execSuccess := 0
	execFailed := 0
	execTotal := 0
	var recentExecs []*workflow.Execution

	if h.executions != nil {
		if list, err := h.executions.List(ctx, workflow.ExecutionFilter{Limit: 10}); err == nil {
			recentExecs = list
		}
		if all, err := h.executions.List(ctx, workflow.ExecutionFilter{Limit: 500}); err == nil {
			execTotal = len(all)
			for _, e := range all {
				if e.Status == workflow.ExecutionSuccess {
					execSuccess++
				} else if e.Status == workflow.ExecutionFailed {
					execFailed++
				}
			}
		}
	} else if h.deliveries != nil {
		// Fallback to legacy delivery stats if executions store is not initialized
		if stats, err := h.deliveries.Stats(ctx); err == nil {
			execTotal = stats.Total
			execSuccess = stats.Delivered
			execFailed = stats.Failed + stats.DeadLetter
		}
	}

	successRate := 100.0
	if execTotal > 0 {
		successRate = (float64(execSuccess) / float64(execTotal)) * 100.0
	}

	deadLetters := 0
	if h.deadLetters != nil {
		if dl, err := h.deadLetters.List(ctx, false); err == nil {
			deadLetters = len(dl)
		}
	} else if h.deliveries != nil {
		if failed, err := h.deliveries.List(ctx, delivery.ListFilter{Status: delivery.StatusDeadLetter}); err == nil {
			deadLetters = len(failed)
		}
	}

	credCount := 0
	if h.creds != nil {
		if creds, err := h.creds.List(ctx); err == nil {
			credCount = len(creds)
		}
	}

	catalog := adapters.Catalog()

	resp := OverviewResponse{
		Status:               "operational",
		ActiveWorkflows:      activeWorkflows,
		TotalWorkflows:       totalWorkflows,
		ExecutionsTotal:      execTotal,
		ExecutionsSuccess:    execSuccess,
		ExecutionsFailed:     execFailed,
		SuccessRate:          successRate,
		DeadLetterCount:      deadLetters,
		ConnectedCredentials: credCount,
		RegisteredAdapters:   len(catalog),
		RecentExecutions:     recentExecs,
		Catalog:              catalog,
		UptimeSeconds:        int64(time.Since(serverStartTime).Seconds()),
	}

	respondJSON(w, http.StatusOK, resp)
}
