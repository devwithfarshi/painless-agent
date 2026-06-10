package streaming

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestEmitter_InProcessDelivery(t *testing.T) {
	e := New(nil) // no Redis
	taskID := uuid.New()

	ch, cancel := e.Subscribe(taskID)
	defer cancel()

	e.Emit(taskID, "step_started", map[string]any{"step": 1})

	select {
	case ev := <-ch:
		if ev.Type != "step_started" {
			t.Errorf("got event type %q, want %q", ev.Type, "step_started")
		}
		if ev.TaskID != taskID.String() {
			t.Errorf("got task ID %q, want %q", ev.TaskID, taskID.String())
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timeout waiting for event")
	}
}

func TestEmitter_CancelRemovesSubscriber(t *testing.T) {
	e := New(nil)
	taskID := uuid.New()

	_, cancel := e.Subscribe(taskID)
	cancel()

	key := taskID.String()
	e.mu.RLock()
	subs := e.subs[key]
	e.mu.RUnlock()
	if len(subs) != 0 {
		t.Errorf("expected 0 subscribers after cancel, got %d", len(subs))
	}
}

func TestEmitter_MultipleSubscribers(t *testing.T) {
	e := New(nil)
	taskID := uuid.New()

	ch1, cancel1 := e.Subscribe(taskID)
	defer cancel1()
	ch2, cancel2 := e.Subscribe(taskID)
	defer cancel2()

	e.Emit(taskID, "task_done", nil)

	for _, ch := range []<-chan Event{ch1, ch2} {
		select {
		case ev := <-ch:
			if ev.Type != "task_done" {
				t.Errorf("got %q, want task_done", ev.Type)
			}
		case <-time.After(100 * time.Millisecond):
			t.Fatal("timeout waiting for event on subscriber")
		}
	}
}

func TestEmitter_UnrelatedTaskNotDelivered(t *testing.T) {
	e := New(nil)
	taskA := uuid.New()
	taskB := uuid.New()

	ch, cancel := e.Subscribe(taskA)
	defer cancel()

	e.Emit(taskB, "step_started", nil)

	select {
	case ev := <-ch:
		t.Errorf("unexpected event for unrelated task: %+v", ev)
	case <-time.After(50 * time.Millisecond):
		// correct: no event delivered
	}
}

func TestEmitter_SlowSubscriberDoesNotBlock(t *testing.T) {
	e := New(nil)
	taskID := uuid.New()

	// Fill the subscriber's buffer completely.
	ch, cancel := e.Subscribe(taskID)
	defer cancel()
	for i := 0; i < cap(ch); i++ {
		e.Emit(taskID, "step_started", nil)
	}

	// One more emit must not block (drop policy).
	done := make(chan struct{})
	go func() {
		e.Emit(taskID, "overflow", nil)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("Emit blocked on full subscriber channel")
	}
}
