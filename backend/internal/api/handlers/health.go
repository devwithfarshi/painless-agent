package handlers

import (
	"net/http"
)

// Health checks infrastructure connectivity and returns service status.
// GET /api/health
func (h *Handlers) Health(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	status := map[string]string{
		"status": "ok",
		"db":     "ok",
		"redis":  "ok",
	}
	code := http.StatusOK

	if err := pingDB(ctx, h.Pool); err != nil {
		status["status"] = "degraded"
		status["db"] = "unreachable"
		code = http.StatusServiceUnavailable
	}

	if err := pingRedis(ctx, h.RDB); err != nil {
		status["status"] = "degraded"
		status["redis"] = "unreachable"
		code = http.StatusServiceUnavailable
	}

	writeJSON(w, code, status)
}
