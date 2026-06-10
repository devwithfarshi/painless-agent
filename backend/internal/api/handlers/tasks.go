package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/devwithfarshi/painless-agent/internal/streaming"
	"github.com/devwithfarshi/painless-agent/internal/types"
)

// ListTasks returns all tasks ordered by creation time descending.
// GET /api/tasks
func (h *Handlers) ListTasks(w http.ResponseWriter, r *http.Request) {
	tasks, err := h.Tasks.ListTasks(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list tasks")
		return
	}
	if tasks == nil {
		tasks = []types.Task{}
	}
	writeJSON(w, http.StatusOK, tasks)
}

// CreateTask creates a task in the DB and enqueues it for processing.
// POST /api/tasks  body: {"goal": "..."}
func (h *Handlers) CreateTask(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Goal string `json:"goal"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Goal == "" {
		writeError(w, http.StatusBadRequest, "goal is required")
		return
	}

	// Pre-create the task so we can return its ID immediately.
	task, err := h.Tasks.CreateTask(r.Context(), body.Goal)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create task")
		return
	}

	// Always execute inline in a background goroutine so `make run` works
	// without a separate worker process. The dedicated `make worker` binary
	// uses its own Asynq consumer loop and does not go through this handler.
	if h.Runtime != nil {
		go func() {
			_ = h.Runtime.RunTask(context.Background(), task.ID, body.Goal)
		}()
	}

	writeJSON(w, http.StatusCreated, task)
}

// GetTask returns task details including steps.
// GET /api/tasks/:id
func (h *Handlers) GetTask(w http.ResponseWriter, r *http.Request) {
	id, err := parseTaskID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid task id")
		return
	}
	task, err := h.Tasks.GetTask(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "task not found")
		return
	}
	steps, err := h.Tasks.ListSteps(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list steps")
		return
	}
	if steps == nil {
		steps = []types.TaskStep{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"task":  task,
		"steps": steps,
	})
}

// CancelTask marks the task as cancelled in the DB.
// DELETE /api/tasks/:id
func (h *Handlers) CancelTask(w http.ResponseWriter, r *http.Request) {
	id, err := parseTaskID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid task id")
		return
	}
	if err := h.Tasks.UpdateStatus(r.Context(), id, types.TaskStatusCancelled); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to cancel task")
		return
	}
	// Emit cancellation event so any open SSE streams can close.
	h.Emitter.Emit(id, "task_failed", map[string]any{"error": "cancelled"})
	w.WriteHeader(http.StatusNoContent)
}

// StreamTask streams live agent events as Server-Sent Events.
// GET /api/tasks/:id/stream
func (h *Handlers) StreamTask(w http.ResponseWriter, r *http.Request) {
	id, err := parseTaskID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid task id")
		return
	}

	// Verify task exists.
	task, err := h.Tasks.GetTask(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "task not found")
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming not supported")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	// If the task already reached a terminal state before the client subscribed,
	// emit a synthetic event so the feed shows something rather than spinning forever.
	switch task.Status {
	case types.TaskStatusCompleted:
		_ = writeSSE(w, flusher, streaming.Event{
			TaskID:  id.String(),
			Type:    streaming.EventTaskDone,
			Payload: map[string]any{"status": "completed"},
		})
		return
	case types.TaskStatusFailed:
		errMsg := task.ErrorMsg
		if errMsg == "" {
			errMsg = "task failed — see steps for details or check worker logs"
		}
		_ = writeSSE(w, flusher, streaming.Event{
			TaskID:  id.String(),
			Type:    streaming.EventTaskFailed,
			Payload: map[string]any{"error": errMsg},
		})
		return
	case types.TaskStatusCancelled:
		_ = writeSSE(w, flusher, streaming.Event{
			TaskID:  id.String(),
			Type:    streaming.EventTaskFailed,
			Payload: map[string]any{"error": "task was cancelled"},
		})
		return
	}

	ctx := r.Context()

	// Subscribe to in-process channel (for inline/same-process runs).
	localCh, cancel := h.Emitter.Subscribe(id)
	defer cancel()

	// Subscribe to Redis for cross-process delivery (worker binary).
	redisCh := h.Emitter.SubscribeRedis(ctx, id)

	// Merge both channels. De-duplication is not needed because the emitter
	// publishes to in-process OR Redis depending on which process runs the task.
	// If somehow both fire (same-process with Redis), the client will just receive
	// a duplicate event — a minor UX issue rather than a correctness problem.
	events := streaming.MergeChannels(ctx, localCh, redisCh)

	for {
		select {
		case ev, ok := <-events:
			if !ok {
				return
			}
			if err := writeSSE(w, flusher, ev); err != nil {
				return
			}
			// Close the stream when the task terminates.
			if ev.Type == streaming.EventTaskDone || ev.Type == streaming.EventTaskFailed {
				return
			}
		case <-ctx.Done():
			return
		}
	}
}

// writeSSE formats and flushes a single SSE event.
func writeSSE(w http.ResponseWriter, flusher http.Flusher, ev streaming.Event) error {
	b, err := json.Marshal(ev)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "data: %s\n\n", b); err != nil {
		return err
	}
	flusher.Flush()
	return nil
}

func parseTaskID(r *http.Request) (uuid.UUID, error) {
	return uuid.Parse(chi.URLParam(r, "id"))
}
