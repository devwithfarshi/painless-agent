package agent

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/devwithfarshi/painless-agent/internal/llm"
	"github.com/devwithfarshi/painless-agent/internal/store"
	"github.com/devwithfarshi/painless-agent/internal/tools"
	"github.com/devwithfarshi/painless-agent/internal/types"
)

// MemoryStore is implemented in Order 3 (internal/memory). Noop until then.
type MemoryStore interface {
	Store(ctx context.Context, content string, tags []string, taskID uuid.UUID) error
	Search(ctx context.Context, query string, k int) ([]string, error)
	RecentContext(ctx context.Context, taskID uuid.UUID, limit int) ([]string, error)
}

// SkillStore is implemented in Order 5 (internal/skills). Noop until then.
type SkillStore interface {
	Match(ctx context.Context, goal string) (*types.Skill, error)
}

// Reflector is implemented in Order 5 (internal/reflection). Noop until then.
type Reflector interface {
	Reflect(ctx context.Context, task types.Task, steps []types.TaskStep) error
}

// ToolEngine is implemented in Order 3 (internal/tools).
type ToolEngine interface {
	Definitions() []llm.ToolDefinition
	Execute(ctx context.Context, name string, input map[string]any) (string, error)
}

// ToolLogStore persists tool call records (internal/store.ToolLogStore).
type ToolLogStore interface {
	Log(ctx context.Context, taskID, stepID uuid.UUID, toolName string, input map[string]any, output string, durationMs int) error
}

// EventEmitter broadcasts runtime events to SSE subscribers (internal/streaming).
type EventEmitter interface {
	Emit(taskID uuid.UUID, eventType string, payload any)
}

// AgentRuntime orchestrates the plan→think→act→observe loop for a goal.
type AgentRuntime struct {
	provider  llm.LLMProvider
	tasks     *store.TaskStore
	planner   *Planner
	memory    MemoryStore
	skills    SkillStore
	reflector Reflector
	tools     ToolEngine
	toolLogs  ToolLogStore
	emitter   EventEmitter
	log       *slog.Logger
}

// RuntimeConfig holds tunable parameters for the runtime.
type RuntimeConfig struct {
	TokenBudget int // context window token budget for trimming
}

func DefaultRuntimeConfig() RuntimeConfig {
	return RuntimeConfig{TokenBudget: 8000}
}

func New(provider llm.LLMProvider, tasks *store.TaskStore, log *slog.Logger, cfg RuntimeConfig) *AgentRuntime {
	return &AgentRuntime{
		provider:  provider,
		tasks:     tasks,
		planner:   NewPlanner(provider),
		memory:    noopMemory{},
		skills:    noopSkills{},
		reflector: noopReflect{},
		tools:     noopTools{},
		toolLogs:  noopToolLogs{},
		emitter:   noopEmitter{},
		log:       log,
	}
}

// WithMemory wires in the memory store (called by Order 3 bootstrap).
func (r *AgentRuntime) WithMemory(m MemoryStore) *AgentRuntime { r.memory = m; return r }

// WithSkills wires in the skill store (called by Order 5 bootstrap).
func (r *AgentRuntime) WithSkills(s SkillStore) *AgentRuntime { r.skills = s; return r }

// WithReflector wires in the reflector (called by Order 5 bootstrap).
func (r *AgentRuntime) WithReflector(rf Reflector) *AgentRuntime { r.reflector = rf; return r }

// WithTools wires in the tool engine and updates the planner's tool list.
func (r *AgentRuntime) WithTools(t ToolEngine) *AgentRuntime {
	r.tools = t
	r.planner.SetTools(t.Definitions())
	return r
}

// WithToolLogs wires in the tool log store (called by Order 3 bootstrap).
func (r *AgentRuntime) WithToolLogs(tl ToolLogStore) *AgentRuntime { r.toolLogs = tl; return r }

// WithEmitter wires in the event emitter (called by Order 6 bootstrap).
func (r *AgentRuntime) WithEmitter(e EventEmitter) *AgentRuntime { r.emitter = e; return r }

// Run creates a new task for goal and executes the full agent loop.
func (r *AgentRuntime) Run(ctx context.Context, goal string) error {
	task, err := r.tasks.CreateTask(ctx, goal)
	if err != nil {
		return fmt.Errorf("create task: %w", err)
	}
	return r.runTask(ctx, task)
}

// RunTask executes the agent loop for a pre-existing task (used by the HTTP API
// worker path where the task is created before enqueueing).
func (r *AgentRuntime) RunTask(ctx context.Context, taskID uuid.UUID, goal string) error {
	task, err := r.tasks.GetTask(ctx, taskID)
	if err != nil {
		// Fall back to creating a new task if the ID was not pre-created.
		r.log.Warn("pre-created task not found, creating new", "task_id", taskID, "error", err)
		newTask, err2 := r.tasks.CreateTask(ctx, goal)
		if err2 != nil {
			return fmt.Errorf("create fallback task: %w", err2)
		}
		task = newTask
	}
	return r.runTask(ctx, task)
}

// runTask is the shared implementation for Run and RunTask.
func (r *AgentRuntime) runTask(ctx context.Context, task types.Task) error {
	r.log.Info("task created", "task_id", task.ID, "goal", task.Goal)

	// Embed the task ID in context so tools (e.g. filesystem) can record provenance.
	ctx = tools.WithTaskID(ctx, task.ID)

	if err := r.tasks.UpdateStatus(ctx, task.ID, types.TaskStatusRunning); err != nil {
		return fmt.Errorf("set task running: %w", err)
	}
	r.emitter.Emit(task.ID, "task_started", map[string]any{"goal": task.Goal})

	// Pull relevant memory context and matching skill (noops until Order 3/5).
	memCtx, _ := r.memory.RecentContext(ctx, task.ID, 5)
	skill, _ := r.skills.Match(ctx, task.Goal)

	// Plan the goal into steps.
	planSteps, err := r.planner.Plan(ctx, task.Goal, memCtx, skill)
	if err != nil {
		r.log.Error("plan failed", "task_id", task.ID, "error", err)
		_ = r.tasks.UpdateStatus(ctx, task.ID, types.TaskStatusFailed)
		_ = r.tasks.SetError(ctx, task.ID, err.Error())
		r.emitter.Emit(task.ID, "task_failed", map[string]any{"error": err.Error()})
		return fmt.Errorf("plan: %w", err)
	}
	r.log.Info("plan ready", "task_id", task.ID, "steps", len(planSteps))

	if err := r.tasks.SetPlan(ctx, task.ID, planSteps); err != nil {
		return fmt.Errorf("persist plan: %w", err)
	}

	// Create persistent step records so the task is resumable.
	dbSteps := make([]types.TaskStep, 0, len(planSteps))
	for _, ps := range planSteps {
		s, err := r.tasks.CreateStep(ctx, task.ID, ps.Description)
		if err != nil {
			return fmt.Errorf("create step: %w", err)
		}
		dbSteps = append(dbSteps, s)
	}

	// Execute loop — Think → Act → Observe per step.
	ctxMgr := NewContextManager(8000)
	ctxMgr.Add(llm.Message{
		Role:    llm.RoleSystem,
		Content: executionSystemPrompt(task.Goal),
	})

	for i, step := range dbSteps {
		// Idempotency: skip already-completed steps on restart.
		if step.Status == types.StepStatusCompleted {
			r.log.Info("skipping completed step", "step_id", step.ID)
			continue
		}

		r.log.Info("executing step", "step", i+1, "step_id", step.ID, "description", step.Description)
		r.emitter.Emit(task.ID, "step_started", map[string]any{
			"step_id":     step.ID,
			"step_index":  i + 1,
			"description": step.Description,
		})

		if err := r.tasks.UpdateStep(ctx, step.ID, types.StepStatusRunning, ""); err != nil {
			return fmt.Errorf("mark step running: %w", err)
		}

		ctxMgr.Add(llm.Message{
			Role:    llm.RoleUser,
			Content: fmt.Sprintf("Execute step %d: %s", i+1, step.Description),
		})

		output, err := r.executeStep(ctx, task, step, ctxMgr)
		if err != nil {
			_ = r.tasks.UpdateStep(ctx, step.ID, types.StepStatusFailed, err.Error())
			r.emitter.Emit(task.ID, "step_failed", map[string]any{
				"step_id": step.ID,
				"error":   err.Error(),
			})
			r.log.Error("step failed", "step_id", step.ID, "error", err)
			continue
		}

		if err := r.tasks.UpdateStep(ctx, step.ID, types.StepStatusCompleted, output); err != nil {
			return fmt.Errorf("persist step output: %w", err)
		}

		r.emitter.Emit(task.ID, "step_done", map[string]any{
			"step_id": step.ID,
			"output":  output,
		})

		// Store step result in memory (skip if output is too short to embed).
		if len(output) >= 20 {
			if merr := r.memory.Store(ctx, output, nil, task.ID); merr != nil {
				r.log.Warn("store step memory failed", "step_id", step.ID, "error", merr)
			}
		}

		r.log.Info("step complete", "step_id", step.ID)
	}

	// Reflect on the completed task (noop until Order 5).
	latestSteps, _ := r.tasks.ListSteps(ctx, task.ID)
	_ = r.reflector.Reflect(ctx, task, latestSteps)

	if err := r.tasks.UpdateStatus(ctx, task.ID, types.TaskStatusCompleted); err != nil {
		return fmt.Errorf("mark task complete: %w", err)
	}
	r.emitter.Emit(task.ID, "task_done", map[string]any{"task_id": task.ID})
	r.log.Info("task complete", "task_id", task.ID)
	return nil
}

// executeStep runs the Think→Act→Observe inner loop for a single step.
// The LLM may call tools zero or more times before producing a final answer.
func (r *AgentRuntime) executeStep(ctx context.Context, task types.Task, step types.TaskStep, ctxMgr *ContextManager) (string, error) {
	const maxIter = 10
	for iter := 0; iter < maxIter; iter++ {
		resp, err := r.provider.Complete(ctx, llm.CompletionRequest{
			Messages:    ctxMgr.Window(),
			Tools:       r.tools.Definitions(),
			Temperature: 0.7,
			MaxTokens:   2048,
		})
		if err != nil {
			return "", fmt.Errorf("llm complete: %w", err)
		}

		if len(resp.ToolCalls) == 0 {
			// Final answer — add to context and return.
			ctxMgr.Add(llm.Message{Role: llm.RoleAssistant, Content: resp.Content})
			return resp.Content, nil
		}

		// Add the assistant's tool-call message to context before executing.
		ctxMgr.Add(llm.Message{
			Role:      llm.RoleAssistant,
			Content:   resp.Content,
			ToolCalls: resp.ToolCalls,
		})

		// Execute each tool call and append results.
		for _, tc := range resp.ToolCalls {
			start := time.Now()
			result, execErr := r.tools.Execute(ctx, tc.Name, tc.Input)
			durationMs := int(time.Since(start).Milliseconds())

			if execErr != nil {
				result = fmt.Sprintf("tool %q unavailable: %s", tc.Name, execErr)
			}

			r.emitter.Emit(task.ID, "tool_called", map[string]any{
				"step_id":  step.ID,
				"tool":     tc.Name,
				"duration": durationMs,
			})

			_ = r.toolLogs.Log(ctx, task.ID, step.ID, tc.Name, tc.Input, result, durationMs)

			ctxMgr.Add(llm.Message{
				Role:       llm.RoleTool,
				Content:    result,
				ToolCallID: tc.ID,
				Name:       tc.Name,
			})

			r.log.Info("tool called", "tool", tc.Name, "step_id", step.ID, "duration_ms", durationMs)
		}
	}
	return "", fmt.Errorf("tool loop exceeded %d iterations", maxIter)
}

func executionSystemPrompt(goal string) string {
	return fmt.Sprintf(`You are an autonomous AI agent executing a task step-by-step.
Overall goal: %s

RULES — follow these strictly:
1. You MUST call a tool to complete each step. Never answer from your training data alone.
2. Steps that say "search" or "research" → call web_search.
3. Steps that say "write", "create", or "save" a file → call filesystem.
4. Steps that say "store", "remember", or "save to memory" → call memory_store.
5. Steps that say "run" or "execute" code → call code_executor.
6. After receiving a tool result, produce a concise summary as your final answer for the step.
7. If a tool returns an error, report the error clearly and do not fabricate results.

Do not describe what you will do — just do it by calling the appropriate tool.`, goal)
}

// ── No-op implementations ─────────────────────────────────────────────────────

type noopMemory struct{}

func (noopMemory) Store(_ context.Context, _ string, _ []string, _ uuid.UUID) error { return nil }
func (noopMemory) Search(_ context.Context, _ string, _ int) ([]string, error)      { return nil, nil }
func (noopMemory) RecentContext(_ context.Context, _ uuid.UUID, _ int) ([]string, error) {
	return nil, nil
}

type noopSkills struct{}

func (noopSkills) Match(_ context.Context, _ string) (*types.Skill, error) { return nil, nil }

type noopReflect struct{}

func (noopReflect) Reflect(_ context.Context, _ types.Task, _ []types.TaskStep) error { return nil }

type noopTools struct{}

func (noopTools) Definitions() []llm.ToolDefinition { return nil }
func (noopTools) Execute(_ context.Context, name string, _ map[string]any) (string, error) {
	return "", fmt.Errorf("tool %q not available", name)
}

type noopToolLogs struct{}

func (noopToolLogs) Log(_ context.Context, _, _ uuid.UUID, _ string, _ map[string]any, _ string, _ int) error {
	return nil
}

type noopEmitter struct{}

func (noopEmitter) Emit(_ uuid.UUID, _ string, _ any) {}
