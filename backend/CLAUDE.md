# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project

`painless-agent` is a self-hosted, autonomous AI agent platform in Go — a Hermes-style agent with persistent memory, tool use, task planning, skill learning, sandboxed code/browser execution, and long-running workflows streamed to a web dashboard. See `md/project-goal.md` for the vision and `md/development-flow.md` for the phased architecture plan (the authoritative design doc — read it before building any `internal/` package).

**Current state:** Orders 1, 2, and 3 complete, plus Copilot provider, first-run onboarding wizard, and terminal REPL. `cmd/server/main.go` wires onboarding → config → Postgres → pgvector → migrations → Redis → LLM provider → embedder → memory store → tool engine → TaskStore → AgentRuntime. `cmd/repl/main.go` is an interactive chat loop with the same tool engine. Set `AGENT_GOAL=<goal>` to run an autonomous task end-to-end.

Internal packages built so far:
- `internal/llm` — `LLMProvider` interface; OpenAI, Anthropic, and **GitHub Copilot** providers; factory. Copilot uses device-code OAuth, Copilot API token exchange/caching, and the OpenAI-compatible `/chat/completions` endpoint. Key exported functions: `ResolveGitHubToken(ctx)`, `ExchangeCopilotToken(ctx, githubToken)`, `FetchCopilotModels(ctx, githubToken)`.
- `internal/onboarding` — first-run wizard (`IsFirstRun`, `Run`, `LoadUserName`). Collects user name, provider, credentials interactively; writes `LLM_PROVIDER`/`LLM_MODEL`/API key back into `.env`; saves `~/.painless-agent/config.json`. Subsequent runs skip the wizard. Reset with `rm ~/.painless-agent/config.json`.
- `internal/types` — `Task`/`TaskStep`/`PlanStep`/`Skill`
- `internal/store` — `TaskStore`, `ToolLogStore` (writes `tool_logs` table)
- `internal/agent` — `Planner` (injects tool schemas into prompt), `ContextManager`, `AgentRuntime` with real Think→Act→Observe loop (max 10 tool iterations/step). Interfaces: `MemoryStore`, `SkillStore`, `Reflector`, `ToolEngine` (with `Execute`), `ToolLogStore`. Wire via `runtime.With*` methods.
- `internal/tools` — `Tool` interface, `ToolEngine` registry with output truncation/summarisation. Registered tools: `web_search` (Brave/SerpAPI), `filesystem` (path-allowlisted to `FilesystemRoot`), `http_client` (GET/POST with size + timeout caps), `summarizer` (LLM-backed condense), `memory_store` (agent-triggered memory write).
- `internal/memory` — `MemoryStore` interface + `pgMemoryStore`: `Store` embeds content then inserts; `Search` embeds query then `ORDER BY embedding <=> $1::vector LIMIT k` (cosine); `RecentContext` fetches recent rows by task. Uses `pgvector.NewVector().String()` + `::vector` cast (text encoding, no codec registration needed).

`internal/skills`, `internal/reflection`, `internal/scheduler`, `internal/streaming`, `internal/sandbox` are still empty — wired via interfaces in the runtime. `frontend/` and `infra/scripts/*.sh` are stubs.

## Commands

Run from `backend/`:

```bash
make up      # docker compose up -d  (Postgres+pgvector on :2345, Redis on :2346)
make down    # docker compose down
make run     # go run ./cmd/server   (requires `make up` first; loads .env)
make repl    # go run ./cmd/repl     (interactive chat with the agent)
make build   # go build -o bin/server ./cmd/server
make tidy    # go mod tidy

go test ./...                        # all tests
go test ./internal/agent/... -run TestName -v   # single package / single test
```

Copy `.env.example` → `.env` before `make run` (only `DATABASE_URL`/`REDIS_URL` are required — the first-run wizard fills in LLM settings). Go 1.26.

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

- **Dependency injection pattern (Order 2):** `AgentRuntime` accepts `MemoryStore`, `SkillStore`, `Reflector`, `ToolEngine`, `ToolLogStore` as interfaces defined in `internal/agent/runtime.go`. No-op implementations live there too. Concrete implementations are wired from `cmd/server/main.go` via `runtime.With*` methods. Never change the `Run` signature; always add capability via the `With*` methods.

- **REPL pattern:** `cmd/repl/main.go` is an interactive terminal chat loop. It uses the same tool engine and memory store as the server but skips the planning phase — direct chat with tool use (Think→Act→Observe, max 10 iterations). Uses `github.com/chzyer/readline` for arrow keys, history, and Ctrl+C handling. Session UUID links all tool_logs; each response is stored to memory. Memory search enriches each prompt with the 3 most relevant past entries.

## Active Plan
Always read ~/.claude/plans/read-md-project-goal-md-and-md-developme-gentle-neumann.md at the start of each session.
Current progress: Orders 1, 2, and 3 complete (plus Copilot provider, onboarding wizard, and terminal REPL added). Starting Order 4.
