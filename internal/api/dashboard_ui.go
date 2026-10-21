package api

import (
	_ "embed"
	"net/http"
)

//go:embed dashboard.html
var KerobDashboardHTML string

//go:embed kerob_logo.png
var KerobLogoPNG []byte

func (h *Handlers) DashboardUI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(KerobDashboardHTML))
}

func (h *Handlers) DashboardLogo(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(KerobLogoPNG)
}
