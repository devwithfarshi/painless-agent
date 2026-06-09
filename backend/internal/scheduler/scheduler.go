package scheduler

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hibiken/asynq"
)

const TaskTypeAgentRun = "agent:run"

// TaskPayload is the JSON body of an agent:run task.
type TaskPayload struct {
	Goal string `json:"goal"`
}

// Runner is the minimal interface the scheduler needs from the agent runtime.
type Runner interface {
	Run(ctx context.Context, goal string) error
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

// Enqueue submits a one-off agent run task. Returns task info on success.
func (c *Client) Enqueue(ctx context.Context, goal string, opts ...asynq.Option) (*asynq.TaskInfo, error) {
	payload, err := json.Marshal(TaskPayload{Goal: goal})
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
	return s.runtime.Run(ctx, p.Goal)
}
