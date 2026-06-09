package handlers

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/devwithfarshi/painless-agent/internal/types"
)

// ListSkills returns all learned skills ordered by usage count.
// GET /api/skills
func (h *Handlers) ListSkills(w http.ResponseWriter, r *http.Request) {
	if h.Skills == nil {
		writeJSON(w, http.StatusOK, []types.Skill{})
		return
	}
	list, err := h.Skills.List(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list skills")
		return
	}
	if list == nil {
		list = []types.Skill{}
	}
	writeJSON(w, http.StatusOK, list)
}

// GetSkill returns a single skill by ID.
// GET /api/skills/:id
func (h *Handlers) GetSkill(w http.ResponseWriter, r *http.Request) {
	if h.Skills == nil {
		writeError(w, http.StatusNotFound, "skills not available")
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid skill id")
		return
	}
	skill, err := h.Skills.Get(r.Context(), id)
	if err != nil || skill == nil {
		writeError(w, http.StatusNotFound, "skill not found")
		return
	}
	writeJSON(w, http.StatusOK, skill)
}

// DeleteSkill removes a learned skill.
// DELETE /api/skills/:id
func (h *Handlers) DeleteSkill(w http.ResponseWriter, r *http.Request) {
	if h.Skills == nil {
		writeError(w, http.StatusNotFound, "skills not available")
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid skill id")
		return
	}
	if err := h.Skills.Delete(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete skill")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
