package healthhttp

import (
	"encoding/json"
	"github.com/HappyHackingSpace/Misko/backend/internal/health/application"
	"net/http"
)

func New(service *application.Service) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, _ *http.Request) { respond(w, http.StatusOK, "ok") })
	mux.HandleFunc("GET /api/ready", func(w http.ResponseWriter, r *http.Request) {
		if !service.Ready(r.Context()) {
			respond(w, http.StatusServiceUnavailable, "unavailable")
			return
		}
		respond(w, http.StatusOK, "ok")
	})
	return mux
}

func respond(w http.ResponseWriter, code int, status string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(struct {
		Status string `json:"status"`
	}{status})
}
