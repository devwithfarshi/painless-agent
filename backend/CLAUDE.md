# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project

`painless-agent` is a self-hosted, autonomous AI agent platform in Go — a Hermes-style agent with persistent memory, tool use, task planning, skill learning, sandboxed code/browser execution, and long-running workflows streamed to a web dashboard. See `md/project-goal.md` for the vision and `md/development-flow.md` for the phased architecture plan (the authoritative design doc — read it before building any `internal/` package).

**Current state: ALL ORDERS COMPLETE (1–8).** `cmd/server/main.go` wires onboarding → config → Postgres → pgvector → migrations → Redis → event emitter → LLM provider (SwappableProvider) → embedder → memory store → skill store → tool engine → TaskStore → ReflectionStore → Reflector → AgentRuntime (with emitter) → scheduler client → HTTP server. The server listens on `:8080` (configurable via `HTTP_ADDR`). `cmd/worker/main.go` is a standalone Asynq worker. `cmd/repl/main.go` is an interactive REPL. The frontend at `frontend/` is a Next.js + Tailwind + shadcn/ui app. The project has a production-grade test suite, Docker build, OpenAPI spec, and load test scripts.

Internal packages built so far:
- `internal/llm` — `LLMProvider` interface; OpenAI, Anthropic, **GitHub Copilot**, **Google Gemini**, **Ollama**, and **OpenRouter** providers; factory (`New`, `NewEmbedder`). All providers wrapped with `RetryProvider` (exponential backoff, 3 retries by default, on HTTP 429/5xx/timeout). `SwappableProvider` wraps any provider behind `sync/atomic` for hot-swap via `POST /api/config/provider`. Gemini uses `google.golang.org/genai v1.59.0`; Ollama and OpenRouter reuse the OpenAI SDK with custom base URLs. `NewGemini` ctx-based constructor; message conversion groups consecutive `RoleTool` messages into a single Gemini "user" content.
- `internal/onboarding` — first-run wizard (`IsFirstRun`, `Run`, `LoadUserName`). Collects user name, provider, credentials interactively; writes `LLM_PROVIDER`/`LLM_MODEL`/API key back into `.env`; saves `~/.painless-agent/config.json`. Subsequent runs skip the wizard. Reset with `rm ~/.painless-agent/config.json`.
- `internal/types` — `Task`/`TaskStep`/`PlanStep`/`Skill` (with `UsageCount int` and `CreatedAt time.Time`)/`SkillStep`
- `internal/store` — `TaskStore`, `ToolLogStore` (writes `tool_logs` table), `ReflectionStore` (writes `reflections` table — `lessons_json` JSONB + `rating` 1–10)
- `internal/agent` — `Planner` (injects tool schemas into prompt), `ContextManager`, `AgentRuntime` with real Think→Act→Observe loop (max 10 tool iterations/step). Interfaces: `MemoryStore`, `SkillStore`, `Reflector`, `ToolEngine` (with `Execute`), `ToolLogStore`. Wire via `runtime.With*` methods.
- `internal/tools` — `Tool` interface, `ToolEngine` registry with output truncation/summarisation. Registered tools: `web_search` (Brave/SerpAPI), `filesystem` (path-allowlisted to `FilesystemRoot`), `http_client` (GET/POST with size + timeout caps), `summarizer` (LLM-backed condense), `memory_store` (agent-triggered memory write), `code_executor` (Docker sandbox), `browser` (headless Chromium via chromedp), `github` (create repo + commit/push via go-github + go-git).
- `internal/memory` — `MemoryStore` interface + `pgMemoryStore`: `Store` embeds content then inserts; `Search` embeds query then `ORDER BY embedding <=> $1::vector LIMIT k` (cosine); `RecentContext` fetches recent rows by task. Uses `pgvector.NewVector().String()` + `::vector` cast (text encoding, no codec registration needed).
- `internal/sandbox` — `Runner` wraps the Docker client. `Run(ctx, RunOpts)` creates and starts a container enforcing `--network=none`, `--memory=256m`, `--cpus=0.5`. `Wait` blocks until exit or timeout. `Logs` demultiplexes stdout/stderr via `stdcopy.StdCopy`. `Remove` force-removes. Image name is always derived from the internal `supportedLanguages` map in `code_executor.go`, never from user input.
- `internal/skills` — `SkillStore` interface (`Match`, `Save`, `IncrementUsage`, `List`, `Get`, `Delete`) + `pgSkillStore`: `Match` embeds goal and does `ORDER BY embedding <=> $1::vector LIMIT 1` — returns nil if distance > threshold; `Save` embeds description and upserts on `name`; same `pgvector.NewVector().String()` + `::vector` pattern as memory.
- `internal/reflection` — `Reflector` implements `agent.Reflector`. `Reflect(ctx, task, steps)` calls the LLM with an `extract_reflection` tool to get `{lessons, rating, promoteToSkill, skillName, skillSteps}`; saves a `reflections` row; if `rating >= threshold && promoteToSkill`, calls `skillStore.Save` to promote the workflow.
- `internal/scheduler` — Asynq-backed `Client` (`Enqueue`, `EnqueueTask` with pre-created task ID) and `Server`. `Runner` interface now includes `RunTask(ctx, taskID, goal)`. `TaskPayload` has an optional `task_id` field — when present, the worker calls `RunTask` instead of `Run` to reuse the pre-created DB row.
- `internal/streaming` — `Emitter`: fans events to in-process `chan Event` subscribers + Redis pub/sub channel `task:<id>`. `Subscribe(taskID)` → in-process (for inline runs). `SubscribeRedis(ctx, taskID)` → Redis pub/sub (for worker-process SSE). `MergeChannels` merges two event channels.
- `internal/api` — chi-based HTTP API. `NewRouter(cfg, handlers, log)` wires all routes. Middleware: `Logging`, `Auth` (X-API-Key / Bearer), `CORS`, `RateLimit` (httprate). Handlers: tasks CRUD + SSE stream, memory search, skills CRUD, config provider, health.
- `frontend/` — Next.js 16 + Tailwind v4 + shadcn/ui. Pages: `/dashboard`, `/tasks`, `/tasks/[id]`, `/memory`, `/skills`. Components: `AgentFeed` (SSE via `useTaskStream`), `TaskCard`, `StepTimeline`, `MemoryCard`. API client at `lib/api.ts`; SSE hook at `lib/sse.ts`. `NEXT_PUBLIC_API_URL` env var (default `http://localhost:8080`). `frontend/Dockerfile` builds a standalone Next.js image; `next.config.ts` has `output: "standalone"`.
- **Test suite (Order 8):** Unit tests: `internal/tools/filesystem_test.go` (traversal, CRUD), `internal/tools/engine_test.go` (truncation, summarisation, registry), `internal/agent/planner_test.go` (parsePlanInput, buildPlannerUserMessage), `internal/agent/context_test.go` (Window trimming, system message preservation), `internal/streaming/emitter_test.go` (fan-out, cancel, slow-subscriber drop). Integration tests in `test/e2e/` require `//go:build integration` tag and running Postgres+Redis: `harness_test.go` (shared DB/Redis setup), `store_test.go` (TaskStore CRUD), `api_test.go` (HTTP handler round-trips via httptest).
- **Production build (Order 8):** `backend/Dockerfile` — multi-stage Go build (golang:1.26-alpine builder → alpine:3.21 runtime). Builds both `cmd/server` and `cmd/worker`; non-root `agent` user; workspace dir pre-created. `infra/docker-compose.override.yml` adds `server`, `worker`, `frontend` services on top of `backend/docker-compose.yml`. `infra/scripts/migrate.sh` and `infra/scripts/seed.sh` are standalone shell scripts for CI / one-off ops.
- **Docs & load tests (Order 8):** `shared/openapi.yaml` — full OpenAPI 3.1 spec for all 12 API endpoints. `infra/loadtest/api.js` — k6 script (50 VUs, p95 < 500 ms threshold, health/list/create coverage). `infra/loadtest/sse.js` — k6 SSE stream smoke test. `README.md` at project root — quick start, architecture, env vars, Docker deployment, load testing instructions.

## Commands

Run from `backend/`:

```bash
make up               # docker compose up -d  (Postgres+pgvector on :2345, Redis on :2346)
make down             # docker compose down
make run              # go run ./cmd/server   (requires `make up` first; loads .env)
make worker           # go run ./cmd/worker   (processes agent:run tasks from the Asynq queue)
make repl             # go run ./cmd/repl     (interactive chat with the agent)
make build            # go build -o bin/server ./cmd/server && go build -o bin/worker ./cmd/worker
make tidy             # go mod tidy
make dev              # cd ../frontend && npm run dev  (Next.js dev server on :3000)
make test             # go test ./...  (unit tests, no external deps)
make test-integration # go test -tags integration ./test/e2e/...  (requires make up first)
make loadtest         # k6 run ../infra/loadtest/api.js  (requires running server + k6)

go test ./internal/agent/... -run TestName -v   # single package / single test
```

Copy `.env.example` → `.env` before `make run` (only `DATABASE_URL`/`REDIS_URL` are required — the first-run wizard fills in LLM settings). For the frontend, copy `frontend/.env.local.example` → `frontend/.env.local`. Go 1.26.

## Architecture & conventions

- **Monorepo layout:** `backend/` (Go), `frontend/` (planned Next.js), `infra/` (Docker for sandbox/browser containers), `shared/` (cross-cutting `openapi.yaml` + Go types). The Go module is `github.com/devwithfarshi/painless-agent`, rooted at `backend/`.

- **Migrations are embedded and auto-applied.** `migrations/*.sql` are goose-format (`-- +goose Up`/`Down`) embedded via `//go:embed` (`migrations/migrations.go`) and run on every startup by `db.Migrate` in `pkg/db/db.go`. To add a schema change, drop a new numbered `.sql` file in `migrations/` — no separate migrate command. `pgvector` is enabled programmatically at boot (`db.EnablePgVector`), not in a migration.

- **The LLM provider interface is the central design decision.** Every component must call the `LLMProvider` interface (`Complete`/`Stream`/`Embed`) — nothing calls OpenAI/Anthropic/Gemini/Ollama SDKs directly. Embeddings are 1536-dim to match the `memory.embedding VECTOR(1536)` column. Default to the latest Claude models for the Anthropic provider.

- **Memory & skills both use pgvector similarity search** (`embedding <=> $1::vector`, cosine via `ivfflat`). Memory has three layers: in-process working context, episodic (Postgres rows), semantic (vector search). Skills are distilled from successful tasks by the reflection system when rating is high.

- **Task persistence model:** `tasks` → `task_steps` (one-to-many). Steps store status/output so a crashed task is resumable — skip already-`completed` steps on restart. `tool_logs` and `reflections` link back to tasks/steps. The `set_updated_at` trigger maintains `updated_at` on `tasks` and `task_steps`.

- **Agent workspace:** `backend/workspace/` is the filesystem tool's root (gitignored contents, `.gitkeep` tracked). Override with `FILESYSTEM_ROOT` env var.

- **Sandbox security (when building `tools`/`sandbox`):** code execution runs in ephemeral Docker containers with `--network=none --memory=256m`, a hard timeout, and never lets user input choose the image name. The filesystem tool validates paths against `FilesystemRoot` — rejects any `..` traversal.

- **Streaming:** progress is emitted as events, fanned out in-process and published to Redis (`task:<id>` channels) for multi-instance, exposed over SSE at `GET /api/tasks/:id/stream`.

- **Config & logging:** all config via env (`pkg/config`, fails fast on missing `DATABASE_URL`/`REDIS_URL`). Logging via `slog` (`pkg/logger`) — JSON in `production`, text otherwise. Wrap errors with context (`fmt.Errorf("...: %w", err)`), the established style throughout `pkg/`.

- **LLM SDK versions and patterns (Order 2):** OpenAI uses `github.com/openai/openai-go/v3` (package `openai`); Anthropic uses `github.com/anthropics/anthropic-sdk-go` (package `anthropic`). Both `openai.Client` and `anthropic.Client` are **value types** (not pointers) — `NewClient` returns by value. OpenAI tools use `[]openai.ChatCompletionToolUnionParam{OfFunction: ...}`; response tool calls come back as `[]ChatCompletionMessageToolCallUnion` with direct `.ID`/`.Function.Name`/`.Function.Arguments` fields. Anthropic tools use `[]anthropic.ToolUnionParam{OfTool: &ToolParam{...}}`; response tool-use blocks are accessed via `block.AsAny().(anthropic.ToolUseBlock)`.

- **GitHub Copilot provider patterns:** Uses `GET https://api.github.com/copilot_internal/v2/token` to exchange a GitHub OAuth token for a short-lived Copilot API token (cached until 30 s before expiry). Chat completions go to `POST https://api.githubcopilot.com/chat/completions` (OpenAI-compatible). Models list from `GET https://api.githubcopilot.com/models`. **Required headers on every Copilot API request:** `Copilot-Integration-Id: vscode-chat` (GitHub validates this against a whitelist — custom IDs get HTTP 400), `Editor-Version: painless-agent/0.1.0`, `Openai-Intent: conversation-panel`. Device-code flow tokens (`ghu_`, `gho_`, `github_pat_`) are accepted; classic PATs (`ghp_`) are rejected. Tokens stored at `~/.painless-agent/copilot_token` (mode 0600). `slow_down` poll responses carry an `interval` field — always adopt that value, never just increment by 1.

- **Onboarding patterns:** `internal/onboarding.IsFirstRun()` checks `~/.painless-agent/config.json` for `setup_done: true`. `Run(ctx, envPath)` is interactive (reads `os.Stdin`, writes `os.Stdout`). After collecting choices it calls `updateEnvFile` which updates keys in-place preserving comments, and uncommments `# KEY=` lines. User name is persisted to the JSON profile; LLM settings go into `.env`. `LoadUserName()` is called from `main.go` to inject `AGENT_USER_NAME` before `config.Load()`.

- **Tool engine patterns (Order 3):** `tools.NewEngine(maxOutput, summarizeFn)` creates the registry. `Register(Tool)` panics on duplicate name. `Execute(ctx, name, input)` returns tool errors as content strings (so the LLM can react) — only system-level errors (unknown tool) are Go errors. Output exceeding `maxOutput` bytes is summarised via the summarizeFn before being returned to the LLM. `ToolEngine` satisfies `agent.ToolEngine` (interface with `Definitions()` + `Execute()`). `WithTools(engine)` on the runtime also calls `planner.SetTools(defs)` so the planner prompt lists available tools.

- **Memory store patterns (Order 3):** `memory.NewPgMemoryStore(pool, embedder)` returns a `MemoryStore`. Vectors are formatted as `pgvector.NewVector(v).String()` (`[x,y,z,...]`) and passed with `::vector` cast — no codec registration required. Embedder is always OpenAI (`llm.NewEmbedder(cfg)`); if `OPENAI_API_KEY` is absent the memory store is skipped and the agent runs without memory (graceful degradation).

- **Config additions (Orders 2–3):** `UserName` (from `AGENT_USER_NAME`), `BraveAPIKey`, `SerpAPIKey`, `FilesystemRoot` (default `"workspace"`), `ToolMaxOutputKB` (default 32), `ToolTimeoutSecs` (default 30), `HTTPMaxBodyKB` (default 256). Missing `.env` is tolerated — only `DATABASE_URL`/`REDIS_URL` are hard-required. Comment out `AGENT_GOAL` in `.env` to stop the auto-task on startup.

- **Config additions (Order 4):** `DockerHost` (optional, defaults to `DOCKER_HOST` env / system socket), `CodeExecTimeoutSecs` (default 30), `GitHubToken`. All three are optional — tools are skipped gracefully if unavailable.

- **Sandbox patterns (Order 4):** `sandbox.NewRunner()` returns `(*Runner, error)` — error means Docker isn't available; caller should skip tool registration and log a warning. `Runner.Run` returns the container ID; caller must always `defer runner.Remove(context.Background(), id)` to prevent leaks. The `code_executor` tool uses a fixed `supportedLanguages` map to derive the image name — user input **never** influences the Docker image used (sandbox-escape prevention).

- **Browser tool patterns (Order 4):** `tools.NewBrowserTool(workspaceRoot)` creates a shared `chromedp.ExecAllocator` (Chrome is launched lazily on first Execute call). Call `browserTool.Close()` on server shutdown to clean up the Chrome process. Screenshots are saved as `screenshot_<ms>.png` in the workspace directory. Each `Execute` call gets its own isolated tab; there is no cross-call browser state.

- **GitHub tool patterns (Order 4):** `tools.NewGitHubTool(token)` wraps `go-github/v73` (REST) + `go-git/v5` (git ops). `create_repo` creates under the authenticated user (empty owner = ""). `commit_and_push` resolves owner via `Users.Get("")` if not provided, shallow-clones to a temp dir, writes files, commits, pushes, and removes the temp dir. File paths are validated against the repo tmpDir root to reject traversal.

- **Dependency injection pattern (Order 2):** `AgentRuntime` accepts `MemoryStore`, `SkillStore`, `Reflector`, `ToolEngine`, `ToolLogStore` as interfaces defined in `internal/agent/runtime.go`. No-op implementations live there too. Concrete implementations are wired from `cmd/server/main.go` via `runtime.With*` methods. Never change the `Run` signature; always add capability via the `With*` methods.

- **Skill store patterns (Order 5):** `skills.NewPgSkillStore(pool, embedder, threshold)` returns a `SkillStore`. `Match` embeds the goal, queries `ORDER BY embedding <=> $1::vector LIMIT 1`, and returns nil if distance > threshold. `Save` embeds the skill description and upserts on `name` (idempotent). Both use the same `pgvector.NewVector(v).String()` + `::vector` cast as the memory store. Skill store is skipped gracefully if the embedder is unavailable.

- **Reflection patterns (Order 5):** `reflection.New(provider, reflectionStore, skillStore, ratingThreshold)` returns a `*Reflector` that satisfies `agent.Reflector`. `Reflect(ctx, task, steps)` calls the LLM with an `extract_reflection` tool (structured JSON output), saves a `reflections` row, and calls `skillStore.Save` when `rating >= ratingThreshold && promoteToSkill`. Reflection is non-fatal — errors are returned but the task is still marked complete.

- **Scheduler patterns (Order 5):** `scheduler.NewClient(redisURL)` enqueues `agent:run` tasks; `scheduler.NewServer(redisURL, runtime, concurrency)` processes them. Both parse the Redis URI via `asynq.ParseRedisURI`. The server's `AGENT_GOAL` path enqueues via the client (falls back to inline run if enqueue fails). `cmd/worker/main.go` is the long-running consumer — wire everything identically to the server then call `srv.Start()` (blocks until shutdown).

- **Config additions (Order 5):** `SkillMatchThreshold` (default 0.3 — cosine distance ceiling; lower = stricter), `ReflectionRatingThreshold` (default 7 — minimum rating 1–10 to promote to skill), `QueueConcurrency` (default 1 — worker goroutines). All three are optional with sensible defaults.

- **Config additions (Order 6):** `HTTPAddr` (default `:8080`), `APIKey` (optional — if set, all API requests need `X-API-Key: <key>` or `Authorization: Bearer <key>`), `CORSOrigins` (default `*`), `RateLimitRPM` (default 120 — per-IP; 0 = disabled).

- **Streaming patterns (Order 6):** `streaming.New(rdb)` creates the emitter. `Emit(taskID, type, payload)` fans to in-process subscribers and publishes to Redis `task:<id>`. `Subscribe(taskID)` returns `(chan Event, cancelFn)` for same-process delivery. `SubscribeRedis(ctx, taskID)` returns a channel fed from Redis pub/sub (for SSE when the runtime runs in a separate worker process). `MergeChannels(ctx, a, b)` merges two channels for the SSE handler. The `EventEmitter` interface added to `agent.AgentRuntime` (wired via `WithEmitter`). `noopEmitter{}` default. Events: `task_started`, `step_started`, `tool_called`, `step_done`, `step_failed`, `task_done`, `task_failed`.

- **API patterns (Order 6):** `api.NewRouter(cfg, handlers, log)` returns a chi router. `handlers.Handlers` struct holds `Tasks`, `Skills`, `Memory`, `Scheduler`, `Emitter`, `Pool`, `RDB`. SSE handler (`GET /api/tasks/:id/stream`) subscribes to both in-process and Redis channels, merges them, writes `data: <json>\n\n` events, closes on `task_done`/`task_failed` or client disconnect. `POST /api/tasks` pre-creates the task in DB, enqueues via `schedClient.EnqueueTask(ctx, taskID, goal)` so the returned ID is usable immediately. `scheduler.Client.EnqueueTask` adds `task_id` to the Asynq payload; `processTask` calls `runtime.RunTask(ctx, taskID, goal)` when `task_id` is present.

- **Frontend patterns (Order 6):** `lib/api.ts` exports `api.{tasks,memory,skills,health}` REST client. `lib/sse.ts` exports `useTaskStream(taskId)` hook that opens an `EventSource` and returns `{events, connected, done, reset}`. Pages use server components for initial data (tasks list, skills list) and client components for interactive parts (task creation, SSE feed). `NEXT_PUBLIC_API_URL` defaults to `http://localhost:8080`.

- **REPL pattern:** `cmd/repl/main.go` is an interactive terminal chat loop. It uses the same tool engine, memory store, and skill store as the server but skips the planning phase — direct chat with tool use (Think→Act→Observe, max 10 iterations). Uses `github.com/chzyer/readline` for arrow keys, history, and Ctrl+C handling. Session UUID links all tool_logs; each response is stored to memory. Memory search enriches each prompt with the 3 most relevant past entries. Skill lookup runs on every message — prints `★ skill: <name>` and injects the skill steps when a match is found.

- **New providers (Order 7):** Six providers total: `openai`, `anthropic`, `copilot`, `gemini`, `ollama`, `openrouter`. Gemini uses `google.golang.org/genai v1.59.0` — `genai.NewClient(ctx, &ClientConfig{APIKey, Backend: BackendGeminiAPI})`, `client.Models.GenerateContent`. Ollama and OpenRouter reuse `OpenAIProvider` with custom base URLs (`option.WithBaseURL`). OpenRouter also requires `HTTP-Referer` and `X-Title` headers. Default models: gemini=`gemini-2.0-flash`, ollama=`llama3.2`, openrouter=`meta-llama/llama-3.3-70b-instruct`. Config keys: `GEMINI_API_KEY`, `OLLAMA_BASE_URL` (default `http://localhost:11434/v1`), `OPENROUTER_API_KEY`.

- **Retry patterns (Order 7):** `llm.WithRetry(p, maxRetries)` wraps any `LLMProvider`. Retries on `HTTP 429`, `HTTP 500`, `HTTP 502`, `HTTP 503`, `HTTP 504`, `connection refused`, `EOF`, `timeout`. `math/rand/v2.N` for jitter. Exponential base (1s, 2s, 4s…, capped at 30s). `context.Canceled`/`context.DeadlineExceeded` are not retried. `llm.New(cfg)` always wraps with retry; controlled by `LLM_MAX_RETRIES` (default 3).

- **Hot-swap patterns (Order 7):** `llm.NewSwappable(provider)` wraps any `LLMProvider` behind `sync/atomic.Pointer[LLMProvider]`. `Swap(p)` atomically replaces the provider; in-flight calls finish against the old provider. `cmd/server/main.go` wraps the initial provider in `SwappableProvider` before passing to runtime, planner, reflector, and summarizer — all share the same pointer so a single `Swap` call affects all. `handlers.Handlers` now has `Provider *llm.SwappableProvider` and `Cfg *config.Config`; `SetProvider` builds a new provider from the request body and calls `Provider.Swap`. No restart required.

- **ContextManager fix (Order 7):** `ContextManager.Window()` now drops messages at turn boundaries (user → next user) instead of one at a time, preventing orphaned `RoleTool` messages from appearing at the start of the context window. This fixes `HTTP 400: messages with role 'tool' must be a response to a preceeding message with 'tool_calls'` that occurred when browser-tool results flooded the context. Token estimate now also counts tool call name lengths.

- **Config additions (Order 7):** `GeminiAPIKey`, `OllamaBaseURL` (default `http://localhost:11434/v1`), `OpenRouterAPIKey`, `LLMMaxRetries` (default 3). Validation extended to new providers. Onboarding wizard updated to offer all 6 providers including Gemini, Ollama, OpenRouter setup steps.

- **Test patterns (Order 8):** Unit tests use only stdlib `testing` + temp dirs — no mocks or external deps. Integration tests live in `test/e2e/` behind `//go:build integration`; `newHarness(t)` auto-loads `.env`, connects to real Postgres/Redis, and runs goose migrations — skips if env vars absent. E2E HTTP tests use `httptest.NewRecorder` + `httptest.NewRequest` against the real chi router (no network). The `contains` helper in `filesystem_test.go` is a package-local string search to avoid the standard library `strings` import conflict.

- **Production deployment patterns (Order 8):** `docker build -t painless-agent-server backend/` builds a ~20 MB Alpine image. To run the full production stack: `docker compose -f backend/docker-compose.yml -f infra/docker-compose.override.yml up -d`. The worker service overrides `ENTRYPOINT` with `["./worker"]`. The frontend service uses `output: "standalone"` in `next.config.ts` to copy only required files into the runner stage.

## Active Plan
All orders 1–8 complete. See ~/.claude/plans/read-md-project-goal-md-and-md-developme-gentle-neumann.md for full history.
The project is feature-complete per the development-flow.md specification.
