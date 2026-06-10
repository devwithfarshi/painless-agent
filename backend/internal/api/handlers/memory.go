package handlers

import (
	"fmt"
	"net/http"
)

// ListMemory returns the most recent memories in insertion order.
// GET /api/memory?limit=<n>
func (h *Handlers) ListMemory(w http.ResponseWriter, r *http.Request) {
	limit := 50
	if lStr := r.URL.Query().Get("limit"); lStr != "" {
		if n, err := fmt.Sscanf(lStr, "%d", &limit); n != 1 || err != nil || limit < 1 {
			limit = 50
		}
	}

	rows, err := h.Pool.Query(r.Context(),
		`SELECT COALESCE(content,''), created_at::text
		 FROM memory ORDER BY created_at DESC LIMIT $1`, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "query failed")
		return
	}
	defer rows.Close()

	type memRow struct {
		Content   string `json:"content"`
		CreatedAt string `json:"created_at"`
	}
	results := []memRow{}
	for rows.Next() {
		var m memRow
		if err := rows.Scan(&m.Content, &m.CreatedAt); err == nil && m.Content != "" {
			results = append(results, m)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"results": results})
}

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

	strs, err := h.Memory.Search(r.Context(), q, k)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "search failed")
		return
	}
	type memRow struct {
		Content   string `json:"content"`
		CreatedAt string `json:"created_at"`
	}
	results := make([]memRow, 0, len(strs))
	for _, s := range strs {
		results = append(results, memRow{Content: s})
	}
	writeJSON(w, http.StatusOK, map[string]any{"results": results})
}
