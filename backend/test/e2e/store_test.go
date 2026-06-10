//go:build integration

package e2e

import (
	"context"
	"testing"

	"github.com/devwithfarshi/painless-agent/internal/store"
	"github.com/devwithfarshi/painless-agent/internal/types"
)

func TestTaskStore_CreateAndGet(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	ts := store.NewTaskStore(h.pool)

	task, err := ts.CreateTask(ctx, "integration test goal")
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if task.ID.String() == "" {
		t.Fatal("task ID is empty")
	}
	if task.Status != types.TaskStatusPending {
		t.Errorf("new task status: got %q, want %q", task.Status, types.TaskStatusPending)
	}

	fetched, err := ts.GetTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if fetched.Goal != task.Goal {
		t.Errorf("goal mismatch: got %q, want %q", fetched.Goal, task.Goal)
	}
}

func TestTaskStore_UpdateStatus(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	ts := store.NewTaskStore(h.pool)

	task, _ := ts.CreateTask(ctx, "status update test")
	if err := ts.UpdateStatus(ctx, task.ID, types.TaskStatusRunning); err != nil {
		t.Fatalf("UpdateStatus: %v", err)
	}

	updated, _ := ts.GetTask(ctx, task.ID)
	if updated.Status != types.TaskStatusRunning {
		t.Errorf("status after update: got %q, want %q", updated.Status, types.TaskStatusRunning)
	}
}

func TestTaskStore_CreateAndListSteps(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	ts := store.NewTaskStore(h.pool)

	task, _ := ts.CreateTask(ctx, "step test")
	step, err := ts.CreateStep(ctx, task.ID, "do something")
	if err != nil {
		t.Fatalf("CreateStep: %v", err)
	}
	if step.TaskID != task.ID {
		t.Errorf("step task ID mismatch")
	}

	steps, err := ts.ListSteps(ctx, task.ID)
	if err != nil {
		t.Fatalf("ListSteps: %v", err)
	}
	if len(steps) != 1 {
		t.Fatalf("got %d steps, want 1", len(steps))
	}
	if steps[0].Description != "do something" {
		t.Errorf("step description: got %q", steps[0].Description)
	}
}

func TestTaskStore_ListTasks(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	ts := store.NewTaskStore(h.pool)

	// Create two tasks so there's always at least 2 in the list.
	_, _ = ts.CreateTask(ctx, "list task A")
	_, _ = ts.CreateTask(ctx, "list task B")

	tasks, err := ts.ListTasks(ctx)
	if err != nil {
		t.Fatalf("ListTasks: %v", err)
	}
	if len(tasks) < 2 {
		t.Errorf("got %d tasks, want at least 2", len(tasks))
	}
}
