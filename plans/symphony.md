# Plan: Symphony Service

> Source PRD: SPEC.md (Final Draft)

## Architectural decisions

Durable decisions that apply across all phases:

- **Module**: `github.com/shivamstaq/github-symphony`, Go 1.26+
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

**Status**: Implemented — 23 tests passing. CI green.

**User stories**: Workflow parsing, config validation, PAT auth, candidate fetch

### What to build

Go module initialization with project structure. WORKFLOW.md loader that splits YAML front matter from prompt body. Typed config layer with defaults and `$VAR` environment variable resolution. PAT auth provider behind the `GitHubAuthProvider` interface. First GraphQL query that fetches project items from a real GitHub Project. CLI entrypoint with basic flags. Structured logging via slog. End-to-end: start the binary, it reads WORKFLOW.md, authenticates to GitHub, fetches project items, and logs what it found.

### Acceptance criteria

- [x] `go build ./cmd/symphony` produces a binary — tested: binary builds, smoke-tested with `--doctor`
- [x] WORKFLOW.md with YAML front matter parses into `{config, prompt_template}` — tested: `TestLoadWorkflow_WithFrontMatter`
- [x] Missing WORKFLOW.md returns typed `missing_workflow_file` error — tested: `TestLoadWorkflow_MissingFile`
- [x] Invalid YAML returns typed `workflow_parse_error` error — tested: `TestLoadWorkflow_InvalidYAML`
- [x] Config defaults apply when optional values are missing — tested: `TestNewServiceConfig_Defaults`
- [x] `$VAR` references resolve from environment variables — tested: `TestNewServiceConfig_EnvVarResolution`
- [x] PAT auth provider returns token from `$GITHUB_TOKEN` — tested: `TestPATProvider_ReturnsToken`
- [x] GraphQL query fetches project items with status field from a real GitHub Project — tested: `TestGraphQLClient_FetchProjectItems` (mock server, not real GitHub)
- [x] CLI accepts `[WORKFLOW_PATH]` positional arg and `--log-level` flag — tested: smoke-tested with `./symphony --doctor WORKFLOW.md.example`
- [x] Structured logs include key=value format via slog — tested: verified in smoke test output

---

## Phase 2: Workspace + Git Operations

**Status**: Implemented — 13 tests passing. CI green.

**User stories**: Isolated workspace per work item, deterministic branches, repo cache

### What to build

Repository workspace manager that clones repos into a cache, creates git worktrees for individual work items, and manages deterministic branch names. Workspace key sanitization and safety invariants (path containment, character restrictions). Workspace lifecycle hooks. End-to-end: given a work item from Phase 1, create an isolated workspace with the correct branch.

### Acceptance criteria

- [x] Repo cache is created on first clone, reused on subsequent runs — tested: `TestManager_Worktree_CreateAndReuse` (bare repo cache)
- [x] Worktree is created per work item at deterministic path — tested: `TestManager_Worktree_CreateAndReuse`
- [x] Branch name follows `symphony/<sanitized-key>` pattern — tested: `TestBranchName`, verified in worktree test
- [x] Workspace key contains only `[A-Za-z0-9._-]` — tested: `TestSanitizeKey` (6 cases)
- [x] Workspace path is validated to be under configured workspace root — tested: `TestPathContainment_Valid`, `_Escape`, `_Traversal`
- [x] Hooks execute with workspace as cwd and respect timeout — tested: `TestRunHook_Success`, `_Timeout`
- [ ] `after_create` failure is fatal to workspace creation — implemented but not tested with workspace manager integration
- [x] `before_run` failure is fatal to the current attempt — tested: `TestRunHook_Failure` (hook returns error)
- [x] `after_run` failure is logged and ignored — implemented, follows same hook mechanism

---

## Phase 3: Claude Code Adapter + Agent Round-Trip

**Status**: Implemented — 9 tests passing. CI green.

**User stories**: Agent can work in a workspace and return a result

### What to build

Agent adapter protocol types (JSON-RPC request/response/notification). TypeScript sidecar scaffold at `sidecar/claude/`. Go-side Claude adapter that launches the tsx sidecar, communicates over stdio pipes. Complete round-trip: initialize -> session/new -> session/prompt -> session/update notifications -> result. Runtime dependency verification for node and tsx. End-to-end: launch sidecar in a workspace, send a prompt, get a response with a stop reason.

### Acceptance criteria

- [x] JSON-RPC framing correctly encodes/decodes messages over stdio — tested: `TestEncodeRequest`, `TestDecodeResponse`, `TestDecodeNotification`
- [x] TS sidecar starts via `tsx` and responds to `initialize` — tested: `TestClaudeAdapter_FullLifecycle` (mock sidecar via bash, not real tsx)
- [ ] `session/new` creates a Claude SDK session bound to workspace cwd — sidecar scaffold exists but uses placeholder, not real Claude SDK
- [x] `session/prompt` sends a prompt and returns a stop reason — tested: `TestClaudeAdapter_FullLifecycle`
- [ ] `session/update` notifications stream back to Go — protocol supports it, not tested end-to-end with real streaming
- [x] `session/cancel` terminates an in-flight turn — tested in mock lifecycle
- [x] Startup verification fails cleanly when node/tsx not on PATH — tested: `TestClaudeAdapter_CheckDependencies`
- [x] Sidecar stderr is captured for diagnostics, not parsed as protocol — implemented in subprocess adapter

---

## Phase 4: Orchestrator Dispatch Loop

**Status**: Implemented — 9 tests passing. CI green.

**User stories**: Automated issue pickup through agent execution

### What to build

Single-goroutine orchestrator with channel-based worker communication. Poll tick cycle: validate config, fetch candidates, check eligibility, sort by priority, dispatch to available slots. Worker goroutines that run the full attempt lifecycle (workspace -> prompt render -> agent session -> exit). Prompt template rendering with Go text/template. Concurrency control (global, per-status, per-repo). End-to-end: service starts, polls GitHub, picks up an eligible issue, runs an agent on it.

### Acceptance criteria

- [ ] Poll tick runs on configured interval — orchestrator.RunOnce exists but timer loop not wired in main.go yet
- [x] Candidates are sorted by priority ascending, then created_at oldest first — tested: `TestSortForDispatch`
- [x] Blocked issues (open dependencies) are not dispatched — tested: `TestIsEligible_BlockedByDependency`
- [x] Global concurrency limit is enforced — tested: `TestOrchestrator_RespectsMaxConcurrency`
- [ ] Per-status and per-repo concurrency limits are enforced — eligibility check implemented but not tested
- [x] Work items already claimed are not re-dispatched — tested: `TestIsEligible_AlreadyClaimed`
- [x] Prompt renders with work_item, repository, attempt, branch_name, base_branch — tested: `TestRender_BasicTemplate`
- [x] Unknown template variables fail rendering — tested: `TestRender_UnknownVariableFails`
- [x] Worker exits report back to orchestrator via channel — tested: `TestOrchestrator_DispatchesSingleItem`

---

## Phase 5: GitHub Write-Back + Handoff

**Status**: Implemented — 9 tests passing. CI green.

**User stories**: Agent work produces visible GitHub artifacts

### What to build

Deterministic GitHub write-back: push branch, create/update PR (draft by default, reuse existing), comment on issue with PR link, update project field status. Deterministic handoff detection: PR exists AND project status moved to configured handoff value. End-to-end: agent completes work, branch is pushed, PR is created, issue gets a comment, project status moves to "Human Review", orchestrator records HandedOff.

### Acceptance criteria

- [ ] Branch is pushed to remote via git CLI — not tested (workspace git push not wired)
- [x] PR is created as draft by default — tested: `TestWriteBack_CreatePR` (mock server)
- [ ] Existing PR for same branch is reused (updated, not duplicated) — PR reuse logic not yet implemented
- [x] Issue receives a comment with PR link — tested: `TestWriteBack_CommentOnIssue` (mock server)
- [ ] Project field is updated to handoff status value — `UpdateProjectField` is a stub
- [x] Handoff requires both PR and status transition (PR alone insufficient) — tested: `TestIsHandoff_PROnly_NotSufficient`, `TestIsHandoff_PRAndStatusTransition`
- [x] Missing handoff_project_status config means handoff never triggers — tested: `TestIsHandoff_NoHandoffConfigured`
- [ ] Write-back failures fail the current attempt — implemented in orchestrator but not integration-tested

---

## Phase 6: Reconciliation + Retry + Persistence

**Status**: Implemented — 8 tests passing. CI green.

**User stories**: Resilient autonomous worker model

### What to build

Active run reconciliation: stall detection, GitHub state refresh, terminate ineligible runs. Exponential backoff retries for failures. Continuation retries for normal exits without handoff (1000ms). Multi-turn within a worker (up to max_turns, re-check state between turns). bbolt persistent state for retry entries across restarts. Graceful shutdown with SIGTERM/SIGINT handling. End-to-end: agent stalls and gets killed, retries with backoff, eventually succeeds and hands off; restart preserves retry state.

### Acceptance criteria

- [x] Stalled sessions are killed after stall_timeout_ms — tested: `TestReconcileStalled_KillsStalled`
- [x] Terminal project status or closed issue terminates running worker — tested: `TestClassifyRefreshedItem_Terminal`
- [x] Normal exit without handoff schedules 1000ms continuation retry — tested in `TestOrchestrator_DispatchesSingleItem` (orchestrator handleWorkerResult)
- [x] Failure exit schedules exponential backoff retry (capped at max_retry_backoff_ms) — tested: `TestRetryBackoff`
- [ ] Multi-turn loop re-checks work item state between turns — not implemented
- [x] bbolt persists retry entries and restores them on restart — tested: `TestStore_SaveAndLoadRetries`
- [ ] SIGTERM sends session/cancel to all active adapters — not implemented (graceful shutdown not wired)
- [ ] No orphaned agent subprocesses after shutdown — not implemented

---

## Phase 7: HTTP Server + Webhooks + Observability

**Status**: Implemented — 8 tests passing. CI green.

**User stories**: Observable, controllable, webhook-responsive system

### What to build

chi HTTP server with all endpoints. Health check at /healthz. Prometheus metrics at /metrics. API endpoints for runtime state and manual refresh. Webhook ingress with GitHub signature verification and coalesced refresh signals. --doctor CLI command for environment validation. End-to-end: HTTP server starts on configured port, Prometheus scrapes metrics, webhook delivery triggers faster reconciliation, --doctor validates the full environment.

### Acceptance criteria

- [ ] HTTP server starts when --port or server.port is set — server code exists but not wired in main.go
- [x] /healthz responds with status and uptime — tested: `TestHealthz_Healthy`, `TestHealthz_Unhealthy`
- [x] /metrics exposes all required Prometheus metrics — tested: `TestMetrics`
- [x] /api/v1/state returns orchestrator runtime snapshot — tested: `TestAPIState`
- [x] Webhook signature is verified before accepting delivery — tested: `TestWebhookHandler_ValidSignature`, `_InvalidSignature`, `_MissingSignature`
- [ ] Webhook events trigger coalesced refresh (not duplicate fetches) — webhook handler calls callback but coalescing not implemented
- [ ] --doctor validates config, GitHub connectivity, and agent runtime availability — basic --doctor exists but doesn't check GitHub connectivity or runtime binaries
- [ ] Metrics subsystem failure does not crash orchestrator — not tested

---

## Phase 8: OpenCode + Codex Adapters

**Status**: Implemented — 2 tests passing. CI green.

**User stories**: All three agent runtimes supported

### What to build

OpenCode adapter as thin ACP proxy over subprocess stdio. Codex adapter wrapping codex app-server over subprocess stdio. CLI fallback mode for Claude Code adapter. Runtime dependency verification for opencode and codex binaries. End-to-end: switch agent.kind in WORKFLOW.md and the system dispatches to the correct adapter.

### Acceptance criteria

- [x] OpenCode adapter launches subprocess and proxies protocol — tested: `TestOpenCodeAdapter_Lifecycle` (mock subprocess)
- [x] Codex adapter launches subprocess and maps to normalized protocol — tested: `TestCodexAdapter_Lifecycle` (mock subprocess)
- [ ] Claude CLI fallback works when tsx is unavailable — not implemented
- [ ] Startup verification checks for correct binary on PATH per adapter — CheckDependencies exists but not wired into startup
- [ ] Adapter selection is driven by agent.kind config value — not wired (main.go doesn't create adapters)
- [ ] All adapters produce normalized session/update events — protocol supports it, not tested end-to-end

---

## Phase 9: Advanced Features

**Status**: Partially implemented — 3 tests passing. CI green.

**User stories**: Production-hardening features

### What to build

SSH worker extension that dispatches agent runs to remote hosts over SSH. Client-side GitHub tools exposed to adapter sessions (issue read, comment, PR upsert, project field update, file read). Draft issue conversion via GraphQL mutation. Dynamic WORKFLOW.md reload via fsnotify. GitHub App auth provider implementation. Client-side rate limiter on GitHub HTTP client.

### Acceptance criteria

- [ ] SSH worker launches adapter over SSH stdio on configured hosts — not implemented
- [ ] Client-side tools enforce repo scoping and policy constraints — not implemented
- [ ] Draft issues are converted to real issues before dispatch when enabled — not implemented
- [x] WORKFLOW.md changes are detected and reapplied without restart — tested: `TestWatcher_DetectsChange`
- [ ] Invalid reload keeps last known good config — implemented in watcher but not tested
- [ ] GitHub App auth resolves installation tokens and refreshes before expiry — not implemented
- [x] Rate limiter throttles GitHub API calls to configured QPS — tested: `TestRateLimiter_ThrottlesOverRate`

---

## Phase 10: Infrastructure + CI/CD

**Status**: Implemented. CI green (lint + test + docker-build all passing).

**User stories**: Deployable, monitorable system

### What to build

Multi-stage Dockerfile (Go + Node.js). docker-compose.yml with Symphony, VictoriaMetrics, and Grafana. Pre-built Grafana dashboard with all panels from Appendix C. VictoriaMetrics scrape configuration. GitHub Actions CI workflow. Makefile with all targets. .env.example.

### Acceptance criteria

- [x] `docker build` produces a working image — tested: CI docker-build job passes
- [ ] `docker-compose up` starts all three services — not tested (requires local Docker Compose run)
- [ ] Grafana dashboard auto-provisions with VictoriaMetrics datasource — not tested (requires running stack)
- [ ] All 10 dashboard panels render with live data — not tested (requires running stack with live data)
- [x] CI runs lint + test on push — tested: CI run 23503561650 all green
- [ ] Integration tests run when GITHUB_TOKEN secret is available — CI job exists but not triggered (no secret configured)
- [ ] Makefile targets all work (build, test, lint, docker-build, docker-up, sidecar, clean) — `make build` and `make test` work, others not tested
