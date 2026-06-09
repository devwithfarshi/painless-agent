package streaming

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

const channelPrefix = "task:"

// Event is an agent runtime event broadcast to SSE subscribers.
type Event struct {
	TaskID  string `json:"task_id"`
	Type    string `json:"type"`
	Payload any    `json:"payload,omitempty"`
}

// Well-known event types emitted by the agent runtime.
const (
	EventTaskStarted  = "task_started"
	EventStepStarted  = "step_started"
	EventToolCalled   = "tool_called"
	EventToolResult   = "tool_result"
	EventStepDone     = "step_done"
	EventStepFailed   = "step_failed"
	EventTaskDone     = "task_done"
	EventTaskFailed   = "task_failed"
)

// Emitter fans events out to in-process subscribers and Redis pub/sub.
// In-process delivery is used for inline task runs (server + worker in same process).
// Redis delivery enables cross-process SSE (separate worker binary).
type Emitter struct {
	rdb  *redis.Client
	mu   sync.RWMutex
	subs map[string][]chan Event // taskID → subscribers
}

// New returns an Emitter. rdb may be nil (disables Redis fan-out).
func New(rdb *redis.Client) *Emitter {
	return &Emitter{
		rdb:  rdb,
		subs: make(map[string][]chan Event),
	}
}

// Emit dispatches an event to all in-process subscribers for the task
// and publishes to Redis so other server instances can deliver it over SSE.
func (e *Emitter) Emit(taskID uuid.UUID, eventType string, payload any) {
	ev := Event{TaskID: taskID.String(), Type: eventType, Payload: payload}
	key := taskID.String()

	// Fan out to in-process subscribers (for inline/same-process runs).
	e.mu.RLock()
	chans := make([]chan Event, len(e.subs[key]))
	copy(chans, e.subs[key])
	e.mu.RUnlock()
	for _, ch := range chans {
		select {
		case ch <- ev:
		default: // drop if subscriber is slow rather than blocking the runtime
		}
	}

	// Publish to Redis for cross-process SSE delivery.
	if e.rdb != nil {
		b, _ := json.Marshal(ev)
		_ = e.rdb.Publish(context.Background(), channelPrefix+key, string(b)).Err()
	}
}

// Subscribe registers an in-process subscriber for a task's events.
// Returns the channel and a cancel func that must be called to avoid leaks.
func (e *Emitter) Subscribe(taskID uuid.UUID) (<-chan Event, func()) {
	ch := make(chan Event, 64)
	key := taskID.String()

	e.mu.Lock()
	e.subs[key] = append(e.subs[key], ch)
	e.mu.Unlock()

	return ch, func() {
		e.mu.Lock()
		defer e.mu.Unlock()
		for i, c := range e.subs[key] {
			if c == ch {
				e.subs[key] = append(e.subs[key][:i], e.subs[key][i+1:]...)
				break
			}
		}
		if len(e.subs[key]) == 0 {
			delete(e.subs, key)
		}
		close(ch)
	}
}

// SubscribeRedis subscribes to the Redis pub/sub channel for a task.
// Useful for SSE when the runtime runs in a separate worker process.
// The returned channel is closed when ctx is cancelled or Redis disconnects.
func (e *Emitter) SubscribeRedis(ctx context.Context, taskID uuid.UUID) <-chan Event {
	out := make(chan Event, 64)
	if e.rdb == nil {
		close(out)
		return out
	}
	ps := e.rdb.Subscribe(ctx, channelPrefix+taskID.String())
	go func() {
		defer close(out)
		defer ps.Close() //nolint:errcheck
		for {
			select {
			case msg, ok := <-ps.Channel():
				if !ok {
					return
				}
				var ev Event
				if err := json.Unmarshal([]byte(msg.Payload), &ev); err != nil {
					continue
				}
				select {
				case out <- ev:
				case <-ctx.Done():
					return
				}
			case <-ctx.Done():
				return
			}
		}
	}()
	return out
}

// MergeChannels fans two event channels into one. Closes out when both sources close.
func MergeChannels(ctx context.Context, a, b <-chan Event) <-chan Event {
	out := make(chan Event, 128)
	var wg sync.WaitGroup
	forward := func(src <-chan Event) {
		defer wg.Done()
		for {
			select {
			case ev, ok := <-src:
				if !ok {
					return
				}
				select {
				case out <- ev:
				case <-ctx.Done():
					return
				}
			case <-ctx.Done():
				return
			}
		}
	}
	wg.Add(2)
	go forward(a)
	go forward(b)
	go func() {
		wg.Wait()
		close(out)
	}()
	return out
}
