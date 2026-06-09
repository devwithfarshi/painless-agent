package handlers

import (
	"fmt"
	"net/http"
)

// SearchMemory performs semantic search over stored memories.
// GET /api/memory/search?q=<query>&k=<limit>
func (h *Handlers) SearchMemory(w http.ResponseWriter, r *http.Request) {
	if h.Memory == nil {
		writeJSON(w, http.StatusOK, map[string]any{"results": []string{}})
		return
	}

	q := r.URL.Query().Get("q")
	if q == "" {
		writeError(w, http.StatusBadRequest, "q parameter is required")
		return
	}

	k := 10
	if kStr := r.URL.Query().Get("k"); kStr != "" {
		if n, err := fmt.Sscanf(kStr, "%d", &k); n != 1 || err != nil || k < 1 {
			k = 10
		}
	}

	results, err := h.Memory.Search(r.Context(), q, k)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "search failed")
		return
	}
	if results == nil {
		results = []string{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"results": results})
}
