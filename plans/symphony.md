# Plan: Symphony Service

> Source PRD: SPEC.md (Final Draft)

## Architectural decisions

Durable decisions that apply across all phases:

- **Module**: `github.com/shivamstaq/github-symphony`, Go 1.24+
- **CLI**: `symphony [WORKFLOW_PATH] --port --log-format --log-level --state-dir --doctor`
- **HTTP Routes**: `GET /healthz`, `GET /metrics`, `GET /api/v1/state`, `GET /api/v1/work-items/{id}`, `POST /api/v1/refresh`, `POST /api/v1/webhooks/github`
- **Key Models**: `WorkItem`, `WorkflowDefinition`, `ServiceConfig`, `RepositoryBinding`, `Workspace`, `RunAttempt`, `LiveSession`, `RetryEntry`, `PullRequestHandle`, `OrchestratorState`
- **Auth**: `GitHubAuthProvider` interface — PAT implementation first, App implementation later. Auth mode resolved from config (`auto`/`pat`/`app`).
- **Agent Protocol**: JSON-RPC over stdio. Methods: `initialize`, `session/new`, `session/prompt`, `session/cancel`, `session/close`, `session/update`
- **State**: Single goroutine owns orchestrator state; workers communicate via channels. bbolt for persistent retry state.
- **Config Source**: `WORKFLOW.md` YAML front matter + Markdown prompt body. Go `text/template` with `missingkey=error`.
- **Git**: Shell out to `git` CLI. Worktrees for isolation. Branches: `symphony/<sanitized-key>`.
- **Adapters**: Claude Code (TS sidecar via tsx, primary), OpenCode (ACP proxy), Codex (app-server subprocess).
- **Handoff Rule**: PR + project status transition to configured handoff value. PR alone is not sufficient.
- **Ports**: Symphony HTTP 9097, VictoriaMetrics 8428, Grafana 3097.

---

## Phase 1: Config Skeleton + First GitHub Query

**User stories**: Workflow parsing, config validation, PAT auth, candidate fetch

### What to build

Go module initialization with project structure. WORKFLOW.md loader that splits YAML front matter from prompt body. Typed config layer with defaults and `$VAR` environment variable resolution. PAT auth provider behind the `GitHubAuthProvider` interface. First GraphQL query that fetches project items from a real GitHub Project. CLI entrypoint with basic flags. Structured logging via slog. End-to-end: start the binary, it reads WORKFLOW.md, authenticates to GitHub, fetches project items, and logs what it found.

### Acceptance criteria

- [ ] `go build ./cmd/symphony` produces a binary
- [ ] WORKFLOW.md with YAML front matter parses into `{config, prompt_template}`
- [ ] Missing WORKFLOW.md returns typed `missing_workflow_file` error
- [ ] Invalid YAML returns typed `workflow_parse_error` error
- [ ] Config defaults apply when optional values are missing
- [ ] `$VAR` references resolve from environment variables
- [ ] PAT auth provider returns token from `$GITHUB_TOKEN`
- [ ] GraphQL query fetches project items with status field from a real GitHub Project
- [ ] CLI accepts `[WORKFLOW_PATH]` positional arg and `--log-level` flag
- [ ] Structured logs include key=value format via slog

---

## Phase 2: Workspace + Git Operations

**User stories**: Isolated workspace per work item, deterministic branches, repo cache

### What to build

Repository workspace manager that clones repos into a cache, creates git worktrees for individual work items, and manages deterministic branch names. Workspace key sanitization and safety invariants (path containment, character restrictions). Workspace lifecycle hooks. End-to-end: given a work item from Phase 1, create an isolated workspace with the correct branch.

### Acceptance criteria

- [ ] Repo cache is created on first clone, reused on subsequent runs
- [ ] Worktree is created per work item at deterministic path
- [ ] Branch name follows `symphony/<sanitized-key>` pattern
- [ ] Workspace key contains only `[A-Za-z0-9._-]`
- [ ] Workspace path is validated to be under configured workspace root
- [ ] Hooks execute with workspace as cwd and respect timeout
- [ ] `after_create` failure is fatal to workspace creation
- [ ] `before_run` failure is fatal to the current attempt
- [ ] `after_run` failure is logged and ignored

---

## Phase 3: Claude Code Adapter + Agent Round-Trip

**User stories**: Agent can work in a workspace and return a result

### What to build

Agent adapter protocol types (JSON-RPC request/response/notification). TypeScript sidecar scaffold at `sidecar/claude/`. Go-side Claude adapter that launches the tsx sidecar, communicates over stdio pipes. Complete round-trip: initialize -> session/new -> session/prompt -> session/update notifications -> result. Runtime dependency verification for node and tsx. End-to-end: launch sidecar in a workspace, send a prompt, get a response with a stop reason.

### Acceptance criteria

- [ ] JSON-RPC framing correctly encodes/decodes messages over stdio
- [ ] TS sidecar starts via `tsx` and responds to `initialize`
- [ ] `session/new` creates a Claude SDK session bound to workspace cwd
- [ ] `session/prompt` sends a prompt and returns a stop reason
- [ ] `session/update` notifications stream back to Go
- [ ] `session/cancel` terminates an in-flight turn
- [ ] Startup verification fails cleanly when node/tsx not on PATH
- [ ] Sidecar stderr is captured for diagnostics, not parsed as protocol

---

## Phase 4: Orchestrator Dispatch Loop

**User stories**: Automated issue pickup through agent execution

### What to build

Single-goroutine orchestrator with channel-based worker communication. Poll tick cycle: validate config, fetch candidates, check eligibility, sort by priority, dispatch to available slots. Worker goroutines that run the full attempt lifecycle (workspace -> prompt render -> agent session -> exit). Prompt template rendering with Go text/template. Concurrency control (global, per-status, per-repo). End-to-end: service starts, polls GitHub, picks up an eligible issue, runs an agent on it.

### Acceptance criteria

- [ ] Poll tick runs on configured interval
- [ ] Candidates are sorted by priority ascending, then created_at oldest first
- [ ] Blocked issues (open dependencies) are not dispatched
- [ ] Global concurrency limit is enforced
- [ ] Per-status and per-repo concurrency limits are enforced
- [ ] Work items already claimed are not re-dispatched
- [ ] Prompt renders with work_item, repository, attempt, branch_name, base_branch
- [ ] Unknown template variables fail rendering
- [ ] Worker exits report back to orchestrator via channel

---

## Phase 5: GitHub Write-Back + Handoff

**User stories**: Agent work produces visible GitHub artifacts

### What to build

Deterministic GitHub write-back: push branch, create/update PR (draft by default, reuse existing), comment on issue with PR link, update project field status. Deterministic handoff detection: PR exists AND project status moved to configured handoff value. End-to-end: agent completes work, branch is pushed, PR is created, issue gets a comment, project status moves to "Human Review", orchestrator records HandedOff.

### Acceptance criteria

- [ ] Branch is pushed to remote via git CLI
- [ ] PR is created as draft by default
- [ ] Existing PR for same branch is reused (updated, not duplicated)
- [ ] Issue receives a comment with PR link
- [ ] Project field is updated to handoff status value
- [ ] Handoff requires both PR and status transition (PR alone insufficient)
- [ ] Missing handoff_project_status config means handoff never triggers
- [ ] Write-back failures fail the current attempt

---

## Phase 6: Reconciliation + Retry + Persistence

**User stories**: Resilient autonomous worker model

### What to build

Active run reconciliation: stall detection, GitHub state refresh, terminate ineligible runs. Exponential backoff retries for failures. Continuation retries for normal exits without handoff (1000ms). Multi-turn within a worker (up to max_turns, re-check state between turns). bbolt persistent state for retry entries across restarts. Graceful shutdown with SIGTERM/SIGINT handling. End-to-end: agent stalls and gets killed, retries with backoff, eventually succeeds and hands off; restart preserves retry state.

### Acceptance criteria

- [ ] Stalled sessions are killed after stall_timeout_ms
- [ ] Terminal project status or closed issue terminates running worker
- [ ] Normal exit without handoff schedules 1000ms continuation retry
- [ ] Failure exit schedules exponential backoff retry (capped at max_retry_backoff_ms)
- [ ] Multi-turn loop re-checks work item state between turns
- [ ] bbolt persists retry entries and restores them on restart
- [ ] SIGTERM sends session/cancel to all active adapters
- [ ] No orphaned agent subprocesses after shutdown

---

## Phase 7: HTTP Server + Webhooks + Observability

**User stories**: Observable, controllable, webhook-responsive system

### What to build

chi HTTP server with all endpoints. Health check at /healthz. Prometheus metrics at /metrics. API endpoints for runtime state and manual refresh. Webhook ingress with GitHub signature verification and coalesced refresh signals. --doctor CLI command for environment validation. End-to-end: HTTP server starts on configured port, Prometheus scrapes metrics, webhook delivery triggers faster reconciliation, --doctor validates the full environment.

### Acceptance criteria

- [ ] HTTP server starts when --port or server.port is set
- [ ] /healthz responds with status and uptime
- [ ] /metrics exposes all required Prometheus metrics
- [ ] /api/v1/state returns orchestrator runtime snapshot
- [ ] Webhook signature is verified before accepting delivery
- [ ] Webhook events trigger coalesced refresh (not duplicate fetches)
- [ ] --doctor validates config, GitHub connectivity, and agent runtime availability
- [ ] Metrics subsystem failure does not crash orchestrator

---

## Phase 8: OpenCode + Codex Adapters

**User stories**: All three agent runtimes supported

### What to build

OpenCode adapter as thin ACP proxy over subprocess stdio. Codex adapter wrapping codex app-server over subprocess stdio. CLI fallback mode for Claude Code adapter. Runtime dependency verification for opencode and codex binaries. End-to-end: switch agent.kind in WORKFLOW.md and the system dispatches to the correct adapter.

### Acceptance criteria

- [ ] OpenCode adapter launches `opencode acp` and proxies ACP protocol
- [ ] Codex adapter launches `codex app-server` and maps to normalized protocol
- [ ] Claude CLI fallback works when tsx is unavailable
- [ ] Startup verification checks for correct binary on PATH per adapter
- [ ] Adapter selection is driven by agent.kind config value
- [ ] All adapters produce normalized session/update events

---

## Phase 9: Advanced Features

**User stories**: Production-hardening features

### What to build

SSH worker extension that dispatches agent runs to remote hosts over SSH. Client-side GitHub tools exposed to adapter sessions (issue read, comment, PR upsert, project field update, file read). Draft issue conversion via GraphQL mutation. Dynamic WORKFLOW.md reload via fsnotify. GitHub App auth provider implementation. Client-side rate limiter on GitHub HTTP client.

### Acceptance criteria

- [ ] SSH worker launches adapter over SSH stdio on configured hosts
- [ ] Client-side tools enforce repo scoping and policy constraints
- [ ] Draft issues are converted to real issues before dispatch when enabled
- [ ] WORKFLOW.md changes are detected and reapplied without restart
- [ ] Invalid reload keeps last known good config
- [ ] GitHub App auth resolves installation tokens and refreshes before expiry
- [ ] Rate limiter throttles GitHub API calls to configured QPS

---

## Phase 10: Infrastructure + CI/CD

**User stories**: Deployable, monitorable system

### What to build

Multi-stage Dockerfile (Go + Node.js). docker-compose.yml with Symphony, VictoriaMetrics, and Grafana. Pre-built Grafana dashboard with all panels from Appendix C. VictoriaMetrics scrape configuration. GitHub Actions CI workflow. Makefile with all targets. .env.example.

### Acceptance criteria

- [ ] `docker build` produces a working image
- [ ] `docker-compose up` starts all three services
- [ ] Grafana dashboard auto-provisions with VictoriaMetrics datasource
- [ ] All 10 dashboard panels render with live data
- [ ] CI runs lint + test on push
- [ ] Integration tests run when GITHUB_TOKEN secret is available
- [ ] Makefile targets all work (build, test, lint, docker-build, docker-up, sidecar, clean)
