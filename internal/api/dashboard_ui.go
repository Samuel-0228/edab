package api

import (
	_ "embed"
	"net/http"
)

//go:embed dashboard.html
var KerobDashboardHTML string

func (h *Handlers) DashboardUI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(KerobDashboardHTML))
}
