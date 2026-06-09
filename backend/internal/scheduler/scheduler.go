package scheduler

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
)

const TaskTypeAgentRun = "agent:run"

// TaskPayload is the JSON body of an agent:run task.
// TaskID is optionally set by the HTTP API so the task is pre-created in the DB.
type TaskPayload struct {
	Goal   string `json:"goal"`
	TaskID string `json:"task_id,omitempty"`
}

// Runner is the minimal interface the scheduler needs from the agent runtime.
type Runner interface {
	Run(ctx context.Context, goal string) error
	RunTask(ctx context.Context, taskID uuid.UUID, goal string) error
}

// Client wraps asynq.Client for enqueueing agent tasks.
type Client struct {
	client *asynq.Client
}

// NewClient creates a scheduler client that enqueues tasks to Redis.
func NewClient(redisURL string) (*Client, error) {
	opt, err := asynq.ParseRedisURI(redisURL)
	if err != nil {
		return nil, fmt.Errorf("parse redis uri: %w", err)
	}
	return &Client{client: asynq.NewClient(opt)}, nil
}

// Enqueue submits a one-off agent run task (no pre-created task ID).
func (c *Client) Enqueue(ctx context.Context, goal string, opts ...asynq.Option) (*asynq.TaskInfo, error) {
	return c.enqueue(ctx, TaskPayload{Goal: goal}, opts...)
}

// EnqueueTask submits a task that already exists in the DB.
// The worker will call RunTask with the given taskID instead of creating a new one.
func (c *Client) EnqueueTask(ctx context.Context, taskID uuid.UUID, goal string, opts ...asynq.Option) (*asynq.TaskInfo, error) {
	return c.enqueue(ctx, TaskPayload{Goal: goal, TaskID: taskID.String()}, opts...)
}

func (c *Client) enqueue(ctx context.Context, p TaskPayload, opts ...asynq.Option) (*asynq.TaskInfo, error) {
	payload, err := json.Marshal(p)
	if err != nil {
		return nil, fmt.Errorf("marshal payload: %w", err)
	}
	info, err := c.client.EnqueueContext(ctx, asynq.NewTask(TaskTypeAgentRun, payload), opts...)
	if err != nil {
		return nil, fmt.Errorf("enqueue task: %w", err)
	}
	return info, nil
}

// Close releases the client connection.
func (c *Client) Close() error { return c.client.Close() }

// Server wraps asynq.Server for processing agent tasks.
type Server struct {
	server  *asynq.Server
	runtime Runner
}

// NewServer creates a worker server that processes agent:run tasks.
// concurrency controls how many tasks run in parallel.
func NewServer(redisURL string, runtime Runner, concurrency int) (*Server, error) {
	opt, err := asynq.ParseRedisURI(redisURL)
	if err != nil {
		return nil, fmt.Errorf("parse redis uri: %w", err)
	}
	if concurrency <= 0 {
		concurrency = 1
	}
	srv := asynq.NewServer(opt, asynq.Config{
		Concurrency: concurrency,
		Queues:      map[string]int{"default": 1},
	})
	return &Server{server: srv, runtime: runtime}, nil
}

// Start registers the task handler and begins processing. Blocks until Stop is called.
func (s *Server) Start() error {
	mux := asynq.NewServeMux()
	mux.HandleFunc(TaskTypeAgentRun, s.processTask)
	return s.server.Run(mux)
}

// Stop gracefully shuts down the worker.
func (s *Server) Stop() { s.server.Shutdown() }

func (s *Server) processTask(ctx context.Context, t *asynq.Task) error {
	var p TaskPayload
	if err := json.Unmarshal(t.Payload(), &p); err != nil {
		return fmt.Errorf("unmarshal payload: %w", err)
	}
	if p.TaskID != "" {
		id, err := uuid.Parse(p.TaskID)
		if err == nil {
			return s.runtime.RunTask(ctx, id, p.Goal)
		}
	}
	return s.runtime.Run(ctx, p.Goal)
}
