//go:build integration

package e2e

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"log/slog"

	"github.com/devwithfarshi/painless-agent/internal/api"
	"github.com/devwithfarshi/painless-agent/internal/api/handlers"
	"github.com/devwithfarshi/painless-agent/internal/store"
	"github.com/devwithfarshi/painless-agent/internal/streaming"
	"github.com/devwithfarshi/painless-agent/internal/types"
	"github.com/devwithfarshi/painless-agent/pkg/config"
)

// buildRouter wires a minimal test router without a scheduler or LLM provider.
func buildRouter(t *testing.T, h *harness) http.Handler {
	t.Helper()
	cfg := &config.Config{
		CORSOrigins:  "*",
		RateLimitRPM: 0, // disabled
	}
	emitter := streaming.New(h.rdb)
	hs := &handlers.Handlers{
		Tasks:   store.NewTaskStore(h.pool),
		Emitter: emitter,
		Pool:    h.pool,
		RDB:     h.rdb,
	}
	return api.NewRouter(cfg, hs, slog.Default())
}

func TestAPI_Health(t *testing.T) {
	h := newHarness(t)
	r := buildRouter(t, h)

	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("health check: got %d, want 200 — body: %s", rr.Code, rr.Body.String())
	}

	var body map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("parse health body: %v", err)
	}
	if body["status"] != "ok" {
		t.Errorf("health status: got %q, want ok", body["status"])
	}
}

func TestAPI_CreateAndGetTask(t *testing.T) {
	h := newHarness(t)
	r := buildRouter(t, h)

	// POST /api/tasks
	payload := `{"goal":"e2e test goal"}`
	req := httptest.NewRequest(http.MethodPost, "/api/tasks", bytes.NewBufferString(payload))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("create task: got %d, want 201 — body: %s", rr.Code, rr.Body.String())
	}

	var task types.Task
	if err := json.Unmarshal(rr.Body.Bytes(), &task); err != nil {
		t.Fatalf("parse create response: %v", err)
	}
	if task.ID.String() == "" {
		t.Fatal("task ID is empty")
	}
	if task.Goal != "e2e test goal" {
		t.Errorf("task goal: got %q", task.Goal)
	}

	// GET /api/tasks/:id
	req = httptest.NewRequest(http.MethodGet, "/api/tasks/"+task.ID.String(), nil)
	rr = httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("get task: got %d — body: %s", rr.Code, rr.Body.String())
	}
	var detail map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &detail); err != nil {
		t.Fatalf("parse get response: %v", err)
	}
	if detail["task"] == nil {
		t.Error("get task response missing 'task' field")
	}
}

func TestAPI_ListTasks(t *testing.T) {
	h := newHarness(t)
	r := buildRouter(t, h)

	req := httptest.NewRequest(http.MethodGet, "/api/tasks", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("list tasks: got %d — body: %s", rr.Code, rr.Body.String())
	}

	var tasks []any
	if err := json.Unmarshal(rr.Body.Bytes(), &tasks); err != nil {
		t.Fatalf("parse list response: %v", err)
	}
}

func TestAPI_CreateTask_MissingGoal(t *testing.T) {
	h := newHarness(t)
	r := buildRouter(t, h)

	req := httptest.NewRequest(http.MethodPost, "/api/tasks", bytes.NewBufferString(`{}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("missing goal: got %d, want 400", rr.Code)
	}
}

func TestAPI_GetTask_NotFound(t *testing.T) {
	h := newHarness(t)
	r := buildRouter(t, h)

	req := httptest.NewRequest(http.MethodGet, "/api/tasks/00000000-0000-0000-0000-000000000000", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("task not found: got %d, want 404", rr.Code)
	}
}

func TestAPI_CancelTask(t *testing.T) {
	h := newHarness(t)
	r := buildRouter(t, h)

	// Create a task first.
	payload := `{"goal":"to be cancelled"}`
	req := httptest.NewRequest(http.MethodPost, "/api/tasks", bytes.NewBufferString(payload))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	var task types.Task
	json.Unmarshal(rr.Body.Bytes(), &task) //nolint

	// DELETE /api/tasks/:id
	req = httptest.NewRequest(http.MethodDelete, "/api/tasks/"+task.ID.String(), nil)
	rr = httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Errorf("cancel task: got %d, want 204 — body: %s", rr.Code, rr.Body.String())
	}
}
