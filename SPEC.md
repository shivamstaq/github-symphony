
# Symphony Service Specification

Status: Final Draft

Purpose: Define a service that orchestrates coding agents to get project work done using GitHub
Projects as the scheduling surface and GitHub Issues as the primary execution object, while
supporting multiple agent runtimes through a portable adapter protocol.

## 1. Problem Statement

Symphony is a long-running automation service that continuously reads work from a GitHub
organization project, resolves executable items to repository-backed GitHub issues, creates an
isolated repository workspace for each work item, and runs a coding-agent session for that work item
inside the workspace.

This version intentionally changes the primary planning surface from a traditional issue tracker to
GitHub-native primitives:

- GitHub Project items are the queue, prioritization, and operational visibility layer.
- GitHub Issues are the durable execution object for most automated work.
- Draft issues are optional intake objects and should normally be converted to issues before
  dispatch.
- Pull requests are downstream execution artifacts and review handoff objects, not the primary work
  item for v3 dispatch.

The service solves seven operational problems:

- It turns issue execution into a repeatable daemon workflow instead of manual scripts.
- It isolates agent execution in per-work-item repository workspaces so agent commands run only
  inside deterministic workspace directories.
- It keeps workflow policy in-repo (`WORKFLOW.md`) so teams version the agent prompt, runtime
  settings, GitHub write-back rules, and harness posture with their code.
- It supports GitHub-native planning by reading from Projects while still executing against Issues.
- It supports multi-repository queues where one GitHub Project can schedule work across many repos.
- It provides enough observability to operate and debug multiple concurrent agent runs.
- It preserves deterministic harness behavior for auth, repository checkout, PR handoff, and GitHub
  side effects.

Important boundary:

- Symphony is a scheduler, runner, webhook consumer, repository workspace manager, and GitHub
  integration harness.
- The coding agent performs work inside the repository workspace.
- Symphony may perform deterministic GitHub write-back actions itself (for example PR upsert,
  project field transitions, issue comments, and status handoff updates) according to workflow
  policy.
- A successful run may end at a workflow-defined handoff state (for example `Human Review`,
  `PR Open`, or `Needs Review`), not necessarily at issue closure.

Implementations are expected to document their trust and safety posture explicitly. This
specification does not require one universal approval or sandbox policy, but it does require that
the harness boundary, GitHub write permissions, workspace isolation rules, and repository/network
exposure are documented clearly.

## 2. Goals and Non-Goals

### 2.1 Goals

- Poll GitHub Projects on a fixed cadence and dispatch work with bounded concurrency.
- Consume GitHub webhooks as a low-latency signal path while retaining polling-based correctness.
- Maintain a single authoritative orchestrator state for dispatch, retries, reconciliation, and
  handoff.
- Resolve project items to executable repository-backed work items deterministically.
- Create deterministic per-work-item repository workspaces and preserve them across runs.
- Stop active runs when project or issue state changes make them ineligible.
- Recover from transient failures with exponential backoff.
- Load runtime behavior from a repository-owned `WORKFLOW.md` contract.
- Support pluggable GitHub authentication: fine-grained Personal Access Token (PAT) for initial
  setup and development, GitHub App installation auth for production. Both share a common auth
  provider interface so the switch is configuration-only.
- Expose operator-visible observability (at minimum structured logs and runtime snapshot APIs).
- Support restart recovery without requiring a heavy persistent database.
- Keep deterministic harness behavior for branch creation, repository sync, PR handoff, and GitHub
  side effects.

### 2.2 Non-Goals

- Rich multi-tenant SaaS control plane.
- Replacing GitHub Projects or GitHub Issues as the source of truth for human planning.
- General-purpose workflow engine or distributed job scheduler.
- Running arbitrary project items without normalization and eligibility rules.
- Mandating one universal dashboard technology for every deployment.
- Mandating maximally permissive agent access to repositories, networks, or GitHub APIs.
- Prescribing a single branch strategy, PR template, or review policy for every team.
- Requiring a large durable database for basic operation.

## 3. System Overview

### 3.1 Main Components

1. `Workflow Loader`
   - Reads `WORKFLOW.md`.
   - Parses YAML front matter and prompt body.
   - Returns `{config, prompt_template}`.

2. `Config Layer`
   - Exposes typed getters for workflow config values.
   - Applies defaults and environment variable indirection.
   - Performs validation used by the orchestrator before dispatch.

3. `GitHub Source Adapter`
   - Fetches candidate project items in configured active values.
   - Resolves project items to backing issues and repository metadata.
   - Fetches current state for specific work items during reconciliation.
   - Normalizes GitHub payloads into a stable `WorkItem` model.

4. `Webhook Ingress`
   - Receives GitHub webhook deliveries.
   - Verifies webhook signatures.
   - Normalizes interesting events into orchestrator refresh signals.
   - Never becomes the sole correctness mechanism.

5. `Orchestrator`
   - Owns the poll tick.
   - Owns the in-memory runtime state.
   - Decides which work items to dispatch, retry, stop, hand off, or release.
   - Tracks session metrics, retry queue state, repository ownership, and write-back outcomes.

6. `Repository Workspace Manager`
   - Maps work items to repository-local workspaces.
   - Ensures the correct repository checkout or worktree exists.
   - Creates deterministic branches for work items.
   - Runs workspace lifecycle hooks.
   - Cleans workspaces for terminal work items when policy requires.

7. `Agent Runner`
   - Prepares repository workspace.
   - Builds prompt from work item + workflow template.
   - Launches the coding-agent adapter client.
   - Streams agent updates back to the orchestrator.
   - Invokes deterministic harness write-back when configured.

8. `GitHub Tool Gateway`
   - Exposes typed GitHub client-side tools to the coding-agent session when enabled.
   - Enforces installation-scoped auth, repo scoping, idempotency, and policy constraints.

9. `Status Surface` (optional)
   - Presents human-readable runtime status (terminal view, HTTP dashboard, or other operator view).

10. `Logging`
    - Emits structured runtime logs to one or more configured sinks.

### 3.2 Abstraction Levels

Symphony is easiest to port when kept in these layers:

1. `Policy Layer` (repo-defined)
   - `WORKFLOW.md` prompt body.
   - Team-specific rules for issue handling, validation, PR handoff, and write-back.

2. `Configuration Layer`
   - Parses front matter into typed runtime settings.
   - Handles defaults, env indirection, repo/path normalization, and GitHub policy settings.

3. `Coordination Layer` (orchestrator)
   - Polling loop, webhook-triggered refresh, eligibility, concurrency, retries, reconciliation,
     handoff, and release.

4. `Execution Layer`
   - Repository/worktree lifecycle, workspace preparation, coding-agent protocol, git operations, and
     deterministic side-effect execution.

5. `Integration Layer`
   - GitHub App auth, GitHub Projects/Issues/PRs adapters, webhook verification, and optional client
     tools.

6. `Observability Layer`
   - Structured logs, runtime snapshots, event timelines, health/error indicators, and optional UI.

### 3.3 External Dependencies

- GitHub API (GraphQL and/or REST).
- GitHub authentication credentials: fine-grained PAT or GitHub App registration + installation.
- Webhook delivery configuration (optional, requires GitHub App or manual webhook setup).
- Local filesystem for repository caches, workspaces, logs, and persistent state.
- `git` CLI for repository synchronization (clone, fetch, worktree, push, branch operations).
- Node.js >= 22 and `tsx` for the Claude Code TypeScript sidecar adapter.
- One or more supported agent runtimes:
  - Claude Code adapter (primary) — TypeScript Agent SDK sidecar via `tsx`
  - OpenCode adapter — ACP over stdio subprocess
  - Codex adapter — `codex app-server` over stdio subprocess
- Host environment authentication for the selected agent runtime and any auxiliary services.
- Optional MCP servers and/or client-side tool providers exposed through the harness.
- Optional SSH or remote execution infrastructure if Appendix A is used.
- Optional observability infrastructure: VictoriaMetrics for metrics storage, Grafana for
  dashboards.

### 3.4 Reference Implementation Profile

This specification remains portable, but the reference implementation profile is:

- Primary runtime: Go 1.24+.
- Module path: `github.com/shivamstaq/github-symphony`.
- Primary service form factor: one long-running daemon binary.
- Claude Code adapter sidecar: TypeScript, run via `tsx`, located at `sidecar/claude/` in the
  repository root.
- HTTP server: Go with `chi` router for API, webhooks, health checks, and Prometheus metrics.
- Dashboard: Grafana consuming VictoriaMetrics (external to the binary).

Rationale:

- The orchestrator is a long-running systems service with process supervision, webhook handling,
  filesystem-heavy repository operations, retries, and streaming stdio protocol handling.
- Go is the preferred implementation language for the primary runtime.
- The Claude Agent SDK is TypeScript/Python-only; a thin TypeScript sidecar over stdio provides
  full SDK feature access (session management, tool handling, MCP, subagents) without reimplementing
  the agent loop in Go. TypeScript is the standardized sidecar language for v1; Python is reserved
  for optional future compatibility.
- Grafana and VictoriaMetrics are lightweight external services for metrics visualization; they are
  not required for orchestrator operation.

### 3.5 Reference Implementation Libraries

The reference implementation uses these Go libraries:

- `google/go-github` (v69+) for GitHub REST API operations.
- Hand-rolled GraphQL queries via `net/http` for GitHub Projects V2 (existing Go GraphQL clients
  do not map well to the Projects V2 schema).
- `gopkg.in/yaml.v3` for YAML front matter parsing.
- `text/template` with `Option("missingkey=error")` for strict prompt rendering.
- `fsnotify/fsnotify` for `WORKFLOW.md` file watching with debounce.
- `log/slog` (stdlib) for structured logging with `key=value` output.
- `os/exec` for subprocess management with explicit pipe control.
- `go-chi/chi` for HTTP routing.
- `go.etcd.io/bbolt` for lightweight persistent state (retry entries, session metadata).
- `golang.org/x/time/rate` or equivalent for GitHub API client-side rate limiting.

### 3.6 Concurrency Model

The orchestrator uses a single-goroutine ownership model:

- One goroutine owns all mutable orchestrator state (no mutexes on the state struct).
- Worker goroutines communicate results back via typed channels.
- The main loop selects on: tick timer, worker result channel, webhook signal channel, config
  reload channel, and shutdown signal channel.
- This is the canonical Go "share by communicating" pattern and guarantees serialized state
  mutations through one authority as required by Section 7.

## 4. Core Domain Model

### 4.1 Entities

#### 4.1.1 WorkItem

Normalized record used by orchestration, prompt rendering, workspace resolution, and observability.

Fields:

- `work_item_id` (string)
  - Stable internal key derived from GitHub project item + backing content.
- `project_id` (string or null)
  - GraphQL node ID for the project when available.
- `project_number` (integer or null)
- `project_item_id` (string)
  - GraphQL node ID for the project item.
- `content_type` (enum)
  - `issue`, `draft_issue`, or `pull_request`.
- `issue_id` (string or null)
  - GraphQL node ID for the backing issue when content type resolves to an issue.
- `issue_number` (integer or null)
- `issue_identifier` (string or null)
  - Human-readable issue key in the form `<owner>/<repo>#<number>`.
- `repository` (object or null)
  - `owner`
  - `name`
  - `full_name`
  - `default_branch`
  - `clone_url_https`
- `title` (string)
- `description` (string or null)
- `state` (string)
  - Backing issue state or equivalent content state (`open`, `closed`, etc.).
- `project_status` (string or null)
  - Value from configured project status field.
- `priority` (integer or null)
  - Normalized dispatch priority derived from configured project field mapping.
- `labels` (list of strings)
  - Normalized to lowercase.
- `assignees` (list of strings)
- `milestone` (string or null)
- `project_fields` (map)
  - Flattened current project field values.
- `blocked_by` (list of blocker refs)
  - Each blocker ref contains:
    - `id` (string or null)
    - `identifier` (string or null)
    - `state` (string or null)
- `sub_issues` (list of child refs)
  - Each child ref contains:
    - `id` (string or null)
    - `identifier` (string or null)
    - `state` (string or null)
- `parent_issue` (object or null)
  - Parent issue ref when this issue is a sub-issue.
- `linked_pull_requests` (list of PR refs)
  - Each PR ref contains:
    - `id`
    - `number`
    - `state`
    - `is_draft`
    - `url`
- `url` (string or null)
- `created_at` (timestamp or null)
- `updated_at` (timestamp or null)

#### 4.1.2 Workflow Definition

Parsed `WORKFLOW.md` payload:

- `config` (map)
  - YAML front matter root object.
- `prompt_template` (string)
  - Markdown body after front matter, trimmed.

#### 4.1.3 Service Config (Typed View)

Typed runtime values derived from `WorkflowDefinition.config` plus environment resolution.

Examples:

- GitHub project selection and auth config
- active and terminal project values
- repository allowlist/denylist
- concurrency limits
- agent adapter/runtime selection, launch config, and timeouts
- git/workspace behavior
- PR handoff/write-back policy
- workspace hooks
- webhook server settings

#### 4.1.4 Repository Binding

Repository metadata and auth context required to prepare a workspace.

Fields (logical):

- `installation_id`
- `repository_id`
- `owner`
- `name`
- `default_branch`
- `clone_url_https`
- `token_expires_at`
- `workspace_repo_cache_path`
- `policy_scope`

#### 4.1.5 Workspace

Repository workspace assigned to one work item.

Fields (logical):

- `path`
- `workspace_key`
- `repo_cache_path`
- `branch_name`
- `base_branch`
- `created_now`
- `created_from_cache` (boolean)

#### 4.1.6 Run Attempt

One execution attempt for one work item.

Fields (logical):

- `work_item_id`
- `project_item_id`
- `issue_identifier`
- `attempt` (integer or null, `null` for first run, `>=1` for retries/continuations)
- `workspace_path`
- `repository_full_name`
- `branch_name`
- `started_at`
- `status`
- `error` (optional)

#### 4.1.7 Live Session (Agent Session Metadata)

State tracked while a coding-agent session is running, regardless of which underlying runtime is
used.

Fields:

- `session_id` (string)
  - Stable Symphony session identifier.
- `native_session_id` (string or null)
  - Provider-native session/thread identifier when exposed.
- `native_turn_id` (string or null)
  - Provider-native turn/message identifier when exposed.
- `adapter_kind` (enum)
  - `codex`, `claude_code`, or `opencode`.
- `adapter_process_pid` (string or null)
  - PID for subprocess/sidecar adapters when available.
- `last_agent_event` (string/enum or null)
- `last_agent_timestamp` (timestamp or null)
- `last_agent_message` (summarized payload)
- `agent_input_tokens` (integer)
- `agent_output_tokens` (integer)
- `agent_total_tokens` (integer)
- `last_reported_input_tokens` (integer)
- `last_reported_output_tokens` (integer)
- `last_reported_total_tokens` (integer)
- `turn_count` (integer)
- `last_github_writeback` (string or null)
- `capabilities` (map)
  - Negotiated adapter capabilities for the live session.

#### 4.1.8 Retry Entry

Scheduled retry state for a work item.

Fields:

- `work_item_id`
- `project_item_id`
- `issue_identifier`
- `attempt` (integer, 1-based for retry queue)
- `due_at_ms` (monotonic clock timestamp)
- `timer_handle` (runtime-specific timer reference)
- `error` (string or null)

#### 4.1.9 Pull Request Handle

Latest known PR associated with a work item during or after a run.

Fields:

- `repository_full_name`
- `branch_name`
- `pull_request_id` (string or null)
- `number` (integer or null)
- `url` (string or null)
- `state` (string or null)
- `is_draft` (boolean or null)

#### 4.1.10 Orchestrator Runtime State

Single authoritative in-memory state owned by the orchestrator.

Fields:

- `poll_interval_ms`
- `max_concurrent_agents`
- `running` (map `work_item_id -> running entry`)
- `claimed` (set of work item IDs reserved/running/retrying)
- `retry_attempts` (map `work_item_id -> RetryEntry`)
- `completed` (set of work item IDs; bookkeeping only, not dispatch gating)
- `agent_totals` (aggregate tokens + runtime seconds + write-back counters)
- `agent_rate_limits` (latest agent-exposed or API-exposed rate-limit snapshot)
- `pending_refresh` (best-effort coalesced refresh flag)
- `recent_webhook_events` (best-effort bounded queue)
- `repo_host_usage` (map `repository_full_name -> running count`)
- `installation_token_cache` (implementation-defined, bounded, in-memory)

### 4.2 Stable Identifiers and Normalization Rules

- `Work Item ID`
  - Use for internal map keys and runtime state.
  - Recommended shape: `github:<project_item_id>:<issue_id or content_type>`.

- `Issue Identifier`
  - Use for human-readable logs and workspace naming.
  - Shape: `<owner>/<repo>#<number>`.

- `Workspace Key`
  - Derive from repository + issue number or project item id.
  - Replace any character not in `[A-Za-z0-9._-]` with `_`.

- `Normalized Issue State`
  - Compare backing issue states after lowercase.

- `Normalized Project Status`
  - Compare configured project field values after lowercase string normalization.

- `Session ID`
  - Use a stable Symphony session identifier.
  - When the provider exposes native thread or turn IDs, preserve them separately as
    `native_session_id` and `native_turn_id`.

## 5. Workflow Specification (Repository Contract)

### 5.1 File Discovery and Path Resolution

Workflow file path precedence:

1. Explicit application/runtime setting (set by CLI startup path).
2. Default: `WORKFLOW.md` in the current process working directory.

Loader behavior:

- If the file cannot be read, return `missing_workflow_file` error.
- The workflow file is expected to be repository-owned and version-controlled.

### 5.2 File Format

`WORKFLOW.md` is a Markdown file with optional YAML front matter.

Design note:

- `WORKFLOW.md` should be self-contained enough to describe and run different workflows (prompt,
  runtime settings, hooks, GitHub auth references, project selection, repository sync policy, PR
  handoff, and tracker/write-back policy) without requiring hidden service-specific configuration.

Parsing rules:

- If file starts with `---`, parse lines until the next `---` as YAML front matter.
- Remaining lines become the prompt body.
- If front matter is absent, treat the entire file as prompt body and use an empty config map.
- YAML front matter must decode to a map/object; non-map YAML is an error.
- Prompt body is trimmed before use.

Returned workflow object:

- `config`: front matter root object.
- `prompt_template`: trimmed Markdown body.

### 5.3 Front Matter Schema

Top-level keys:

- `tracker`
- `github`
- `git`
- `polling`
- `workspace`
- `hooks`
- `agent`
- `codex`
- `claude`
- `opencode`
- `pull_request`
- `server`

Unknown keys should be ignored for forward compatibility.

Note:

- The workflow front matter is extensible. Implementations may define additional top-level keys
  without changing the core schema above.
- Extensions should document their field schema, defaults, validation rules, and whether changes
  apply dynamically or require restart.

#### 5.3.1 `tracker` (object)

Fields:

- `kind` (string)
  - Required for dispatch.
  - Supported value in v3: `github`.
- `owner` (string)
  - Required.
  - GitHub organization or user that owns the project.
- `project_number` (integer)
  - Required for project-driven dispatch.
- `project_scope` (string)
  - Default: `organization`
  - Allowed values: `organization`, `user`.
- `status_field_name` (string)
  - Default: `Status`
- `active_values` (list of strings)
  - Default: `Todo`, `Ready`, `In Progress`
- `terminal_values` (list of strings)
  - Default: `Done`, `Closed`, `Cancelled`, `Canceled`, `Duplicate`
- `priority_field_name` (string, optional)
  - Default: `Priority`
- `priority_value_map` (map `string -> integer`, optional)
  - Used to derive normalized `priority`.
- `executable_item_types` (list of strings)
  - Default: `issue`
- `require_issue_backing` (boolean)
  - Default: `true`
- `allow_draft_issue_conversion` (boolean)
  - Default: `false`
- `repo_allowlist` (list of `owner/name`, optional)
- `repo_denylist` (list of `owner/name`, optional)
- `required_labels` (list of strings, optional)
- `blocked_status_values` (list of strings, optional)
  - Optional extra project values that should fail eligibility immediately.

#### 5.3.2 `github` (object)

Fields:

- `auth_mode` (string)
  - Default: `auto`
  - Allowed values: `pat`, `app`, `auto`.
  - When `auto`, the implementation resolves auth mode by checking which credentials are available:
    - If `github.token` (or `$GITHUB_TOKEN`) is present, use `pat` mode.
    - If `github.app_id` and `github.private_key` are present, use `app` mode.
    - If both are present, prefer `app` mode.
    - If neither is present, fail startup validation.
- `api_url` (string)
  - Default: `https://api.github.com`
  - For GHES, point to the enterprise API base.
- `token` (string or `$VAR`, optional)
  - Personal Access Token for PAT auth mode.
  - Canonical env variable: `GITHUB_TOKEN`.
  - Required when `auth_mode` is `pat` or when `auto` resolves to PAT.
  - Must be a fine-grained PAT with at minimum: repository read/write, project read/write, and
    issues read/write permissions.
- `app_id` (string or `$VAR`, optional)
  - Required when `auth_mode` is `app` or when `auto` resolves to App.
  - Canonical env variable: `GITHUB_APP_ID`.
- `private_key` (string or `$VAR`, optional)
  - Required when `auth_mode` is `app`.
  - PEM content or path depending on implementation policy.
  - Canonical env variable: `GITHUB_APP_PRIVATE_KEY`.
- `webhook_secret` (string or `$VAR`, optional)
  - Canonical env variable: `GITHUB_WEBHOOK_SECRET`.
- `installation_id` (string or `$VAR`, optional)
  - When omitted in `app` mode, the implementation may resolve installation dynamically from
    repo/org context.
  - Ignored in `pat` mode.
- `token_refresh_skew_ms` (integer, optional)
  - Default: `300000`
  - Refresh installation tokens before they expire.
  - Only applicable in `app` mode.
- `graphql_page_size` (integer, optional)
  - Default: `50`
- `request_timeout_ms` (integer, optional)
  - Default: `30000`
- `rate_limit_qps` (integer, optional)
  - Default: `10`
  - Client-side rate limit for GitHub API calls (queries per second).
  - Prevents hitting GitHub's server-side rate limits (5000 req/hour for Apps, varies for PATs).

#### 5.3.3 `git` (object)

Fields:

- `base_branch` (string, optional)
  - When omitted, use repository default branch.
- `branch_prefix` (string)
  - Default: `symphony/`
- `fetch_depth` (integer, optional)
  - Default: `0` (full history)
- `reuse_repo_cache` (boolean)
  - Default: `true`
- `use_worktrees` (boolean)
  - Default: `true`
- `clean_untracked_before_run` (boolean)
  - Default: `false`
- `push_remote_name` (string)
  - Default: `origin`
- `commit_author_name` (string, optional)
- `commit_author_email` (string, optional)

#### 5.3.4 `polling` (object)

Fields:

- `interval_ms` (integer or string integer)
  - Default: `30000`
  - Changes should be re-applied at runtime and affect future tick scheduling without restart.

#### 5.3.5 `workspace` (object)

Fields:

- `root` (path string or `$VAR`)
  - Default: `<system-temp>/symphony_workspaces`
- `repo_cache_dir` (path string or `$VAR`, optional)
  - Default: `<workspace.root>/repo_cache`
- `worktree_dir` (path string or `$VAR`, optional)
  - Default: `<workspace.root>/worktrees`
- `remove_on_terminal` (boolean)
  - Default: `true`

#### 5.3.6 `hooks` (object)

Fields:

- `after_create` (multiline shell script string, optional)
- `before_run` (multiline shell script string, optional)
- `after_run` (multiline shell script string, optional)
- `before_remove` (multiline shell script string, optional)
- `timeout_ms` (integer, optional)
  - Default: `60000`

#### 5.3.7 `agent` (object)

Fields:

- `kind` (string)
  - Required for dispatch.
  - Supported values in v3:
    - `codex`
    - `claude_code`
    - `opencode`
- `launch_mode` (string)
  - Default depends on adapter kind.
  - Supported values:
    - `subprocess_stdio`
    - `sidecar_stdio`
    - `in_process`
  - `claude_code` commonly uses `sidecar_stdio` or `in_process`.
  - `codex` and `opencode` commonly use `subprocess_stdio`.
- `command` (string shell command, optional)
  - Effective default depends on adapter kind:
    - `codex` -> `codex app-server`
    - `opencode` -> `opencode acp`
    - `claude_code` -> implementation-defined wrapper/sidecar when not using `in_process`
- `default_model` (string, optional)
- `max_concurrent_agents` (integer or string integer)
  - Default: `10`
- `max_turns` (integer or string integer)
  - Default: `20`
- `max_retry_backoff_ms` (integer or string integer)
  - Default: `300000`
- `max_concurrent_agents_by_project_status` (map `status_name -> positive integer`)
  - Default: empty map
- `max_concurrent_agents_by_repo` (map `owner/name -> positive integer`)
  - Default: empty map
- `session_reuse_mode` (string)
  - Default: `continue_if_supported`
  - Allowed values: `continue_if_supported`, `single_turn_only`
- `read_timeout_ms` (integer)
  - Default: `5000`
- `turn_timeout_ms` (integer)
  - Default: `3600000`
- `stall_timeout_ms` (integer)
  - Default: `300000`
- `enable_client_tools` (boolean)
  - Default: `true`
- `enable_mcp` (boolean)
  - Default: `true`
- `provider_params` (map, optional)
  - Adapter-specific passthrough values applied at session start when supported.

#### 5.3.8 `codex` (object)

Fields:

- `approval_policy` (pass-through Codex value)
- `thread_sandbox` (pass-through Codex value)
- `turn_sandbox_policy` (pass-through Codex value)
- `listen` (string, optional)
  - Default: `stdio://`
- `schema_bundle_dir` (path, optional)
  - Directory containing JSON Schema or TypeScript artifacts generated from the pinned Codex
    version.
- `provider_params` (map, optional)
  - Additional Codex-specific parameters preserved by the adapter.

#### 5.3.9 `claude` (object)

Fields:

- `adapter_mode` (string)
  - Default: `sdk_sidecar`
  - Allowed values:
    - `sdk_sidecar` — TypeScript Agent SDK sidecar process communicating over stdio. This is the
      primary and recommended mode. The sidecar lives at `sidecar/claude/` in the repository and
      is launched via `tsx`.
    - `cli_fallback` — Shell out to the `claude` CLI with `--print` and JSON output modes. Simpler
      but lacks session reuse, streaming updates, and tool callbacks. Use only when the full SDK
      sidecar is unavailable.
- `sdk_language` (string)
  - Default: `typescript`
  - Allowed values: `typescript`, `python`
  - For v1, `typescript` is required and standardized. `python` is reserved for optional future
    compatibility. Implementations must not require both runtimes simultaneously.
- `sidecar_command` (string, optional)
  - Default: `tsx sidecar/claude/src/index.ts`
  - Override the command used to launch the Claude sidecar process. The command receives the
    workspace path and communicates via stdin/stdout JSON-RPC.
- `model` (string, optional)
  - Overrides `agent.default_model` for the Claude adapter.
- `allowed_tools` (list of strings, optional)
  - Claude SDK tool names or wrapper-defined aliases.
- `mcp_servers` (list, optional)
  - MCP server descriptors or references that the adapter will attach when supported.
- `continue_on_pause_turn` (boolean)
  - Default: `true`
  - The adapter should handle provider pause/continuation flows without surfacing a spurious hard
    failure unless policy requires one.
- `permission_profile` (string or object, optional)
  - Adapter-specific approval/tool gating policy.
- `enable_subagents` (boolean)
  - Default: `false`
- `provider_params` (map, optional)
  - Additional Claude SDK options preserved by the adapter.

#### 5.3.10 `opencode` (object)

Fields:

- `model` (string, optional)
  - Overrides `agent.default_model` for the OpenCode adapter.
- `permission_profile` (map or string, optional)
  - OpenCode/ACP tool permission configuration. Implementations may map values like `allow`,
    `ask`, or `deny` per tool or pattern.
- `config_file` (path, optional)
  - Adapter-visible OpenCode config file when the deployment uses one.
- `resume_session` (boolean)
  - Default: `true`
- `mcp_servers` (list, optional)
  - MCP server descriptors or references exposed to the OpenCode ACP session.
- `provider_params` (map, optional)
  - Additional OpenCode/ACP session parameters preserved by the adapter.

#### 5.3.11 `server` (object, extension)

Fields:

- `port` (integer, optional)
  - HTTP server listen port.
  - When present, the HTTP server starts automatically.
  - CLI `--port` overrides this value.
  - Default: not set (HTTP server disabled unless CLI `--port` is provided).
- `host` (string, optional)
  - Default: `0.0.0.0`
  - Bind address for the HTTP server.
- `read_timeout_ms` (integer, optional)
  - Default: `30000`
- `write_timeout_ms` (integer, optional)
  - Default: `60000`
- `cors_origins` (list of strings, optional)
  - Default: empty (no CORS headers).
  - When set, the server adds CORS headers for the listed origins.

#### 5.3.12 `pull_request` (object)

Fields:

- `open_pr_on_success` (boolean)
  - Default: `true`
- `draft_by_default` (boolean)
  - Default: `true`
- `reuse_existing_pr` (boolean)
  - Default: `true`
- `handoff_project_status` (string, optional)
  - Example: `Human Review`
- `comment_on_issue_with_pr` (boolean)
  - Default: `true`
- `required_before_handoff_checks` (list of strings, optional)
  - Implementation-defined validation names.
- `close_issue_on_merge` (boolean, optional)
  - Default: `false`

### 5.4 Prompt Template Contract

The Markdown body of `WORKFLOW.md` is the per-work-item prompt template.

Template syntax:

- The reference implementation uses Go `text/template` syntax.
- Template delimiters are `{{` and `}}`.
- Nested field access uses dot notation: `{{.work_item.title}}`.
- Conditionals: `{{if .attempt}}retry attempt {{.attempt}}{{end}}`.
- Iteration: `{{range .work_item.labels}}{{.}} {{end}}`.

Rendering requirements:

- Use Go `text/template` with `Option("missingkey=error")`.
- Unknown variables must fail rendering (enforced by `missingkey=error`).
- Unknown functions must fail rendering.

Template input variables:

- `work_item` (object)
  - Includes all normalized work item fields from Section 4.1.1.
- `issue` (object or null)
  - Convenience alias to the backing issue when content type is `issue`.
- `repository` (object or null)
- `attempt` (integer pointer or null; nil for first run, >= 1 for retries)
- `branch_name` (string)
- `base_branch` (string)
- `project_fields` (map)

Example prompt template:

```
You are working on {{.work_item.issue_identifier}}: {{.work_item.title}}

Repository: {{.repository.full_name}}
Branch: {{.branch_name}} (based on {{.base_branch}})

{{if .work_item.description}}
## Issue Description
{{.work_item.description}}
{{end}}

{{if .attempt}}
This is retry attempt {{.attempt}}. Review previous work on the branch before continuing.
{{end}}

{{if .work_item.labels}}Labels: {{range .work_item.labels}}{{.}} {{end}}{{end}}
```

Fallback prompt behavior:

- If the workflow prompt body is empty, the runtime uses the minimal default prompt:
  `You are working on a GitHub issue from a GitHub Project.`
- Workflow file read/parse failures are configuration/validation errors and must not silently fall
  back to a prompt.

### 5.5 Workflow Validation and Error Surface

Error classes:

- `missing_workflow_file`
- `workflow_parse_error`
- `workflow_front_matter_not_a_map`
- `template_parse_error`
- `template_render_error`

Dispatch gating behavior:

- Workflow file read/YAML errors block new dispatches until fixed.
- Template errors fail only the affected run attempt.

## 6. Configuration Specification

### 6.1 Source Precedence and Resolution Semantics

Configuration precedence:

1. Workflow file path selection (runtime setting -> cwd default).
2. YAML front matter values.
3. Environment indirection via `$VAR_NAME` inside selected YAML values.
4. Built-in defaults.

Value coercion semantics:

- Path fields support `~` expansion and env-backed path expansion.
- URI fields should not be rewritten as filesystem paths.
- Secrets must be resolved without logging the secret value.

### 6.2 Dynamic Reload Semantics

Dynamic reload is required:

- The software should watch `WORKFLOW.md` for changes.
- On change, it should re-read and re-apply workflow config and prompt template without restart.
- The software should attempt to adjust live behavior to the new config (polling cadence,
  concurrency limits, project status values, repo allowlists, adapter settings, git/workspace hooks,
  PR handoff behavior, and prompt content for future runs).
- Reloaded config applies to future dispatch, retry scheduling, reconciliation decisions, hook
  execution, GitHub write-back, and agent launches.
- Implementations are not required to restart in-flight agent sessions automatically when config
  changes.
- Invalid reloads should not crash the service; keep operating with the last known good effective
  configuration and emit an operator-visible error.

### 6.3 Dispatch Preflight Validation

Startup validation:

- Validate configuration before starting the scheduling loop.
- If startup validation fails, fail startup and emit an operator-visible error.

Per-tick dispatch validation:

- Re-validate before each dispatch cycle.
- If validation fails, skip dispatch for that tick, keep reconciliation active, and emit an
  operator-visible error.

Validation checks:

- Workflow file can be loaded and parsed.
- `tracker.kind` is present and supported.
- `tracker.owner` and `tracker.project_number` are present.
- GitHub auth is resolvable: either `github.token` is present (PAT mode), or `github.app_id` and
  `github.private_key` are present (App mode), after `$` resolution.
- `agent.kind` is present and supported.
- The effective adapter launch configuration is valid for the chosen runtime.
- If `agent.kind == claude_code`, verify that `node` and `tsx` are available on `$PATH`.
- If `agent.kind == opencode`, verify that `opencode` is available on `$PATH`.
- If `agent.kind == codex`, verify that `codex` is available on `$PATH`.
- If webhook ingress is enabled, `github.webhook_secret` must be present.
- If `pull_request.open_pr_on_success=true`, repository push/write policy must be configured.

### 6.4 Config Fields Summary (Cheat Sheet)

- `tracker.kind`: string, required, `github`
- `tracker.owner`: string, required
- `tracker.project_number`: integer, required
- `tracker.project_scope`: string, default `organization`
- `tracker.status_field_name`: string, default `Status`
- `tracker.active_values`: list of strings, default `["Todo","Ready","In Progress"]`
- `tracker.terminal_values`: list of strings, default `["Done","Closed","Cancelled","Canceled","Duplicate"]`
- `tracker.priority_field_name`: string, default `Priority`
- `tracker.priority_value_map`: optional map
- `tracker.executable_item_types`: list, default `["issue"]`
- `tracker.require_issue_backing`: bool, default `true`
- `tracker.allow_draft_issue_conversion`: bool, default `false`
- `tracker.repo_allowlist`: optional list
- `tracker.repo_denylist`: optional list
- `github.auth_mode`: string, default `auto`, one of `pat`, `app`, `auto`
- `github.api_url`: string, default `https://api.github.com`
- `github.token`: string or `$VAR`, optional (PAT mode)
- `github.app_id`: string or `$VAR`, optional (App mode)
- `github.private_key`: string or `$VAR`, optional (App mode)
- `github.webhook_secret`: string or `$VAR`, optional
- `github.installation_id`: string or `$VAR`, optional (App mode)
- `github.token_refresh_skew_ms`: integer, default `300000` (App mode)
- `github.graphql_page_size`: integer, default `50`
- `github.request_timeout_ms`: integer, default `30000`
- `github.rate_limit_qps`: integer, default `10`
- `git.base_branch`: optional string
- `git.branch_prefix`: string, default `symphony/`
- `git.fetch_depth`: integer, default `0`
- `git.reuse_repo_cache`: bool, default `true`
- `git.use_worktrees`: bool, default `true`
- `workspace.root`: path, default `<system-temp>/symphony_workspaces`
- `workspace.repo_cache_dir`: path, default `<workspace.root>/repo_cache`
- `workspace.worktree_dir`: path, default `<workspace.root>/worktrees`
- `workspace.remove_on_terminal`: bool, default `true`
- `hooks.timeout_ms`: integer, default `60000`
- `agent.kind`: required, one of `codex`, `claude_code`, `opencode`
- `agent.launch_mode`: string, adapter-dependent
- `agent.command`: optional shell command; effective default depends on adapter kind
- `agent.default_model`: optional string
- `agent.max_concurrent_agents`: integer, default `10`
- `agent.max_turns`: integer, default `20`
- `agent.max_retry_backoff_ms`: integer, default `300000`
- `agent.session_reuse_mode`: string, default `continue_if_supported`
- `agent.read_timeout_ms`: integer, default `5000`
- `agent.turn_timeout_ms`: integer, default `3600000`
- `agent.stall_timeout_ms`: integer, default `300000`
- `agent.enable_client_tools`: bool, default `true`
- `agent.enable_mcp`: bool, default `true`
- `agent.provider_params`: optional map
- `codex.approval_policy`: pass-through
- `codex.thread_sandbox`: pass-through
- `codex.turn_sandbox_policy`: pass-through
- `codex.listen`: string, default `stdio://`
- `codex.schema_bundle_dir`: optional path
- `claude.adapter_mode`: string, default `sdk_sidecar`, one of `sdk_sidecar`, `cli_fallback`
- `claude.sdk_language`: string, default `typescript`, one of `typescript`, `python`
- `claude.sidecar_command`: optional string, default `tsx sidecar/claude/src/index.ts`
- `claude.model`: optional string
- `claude.allowed_tools`: optional list
- `claude.mcp_servers`: optional list
- `claude.continue_on_pause_turn`: bool, default `true`
- `claude.permission_profile`: optional string/object
- `claude.enable_subagents`: bool, default `false`
- `opencode.model`: optional string
- `opencode.permission_profile`: optional string/map
- `opencode.config_file`: optional path
- `opencode.resume_session`: bool, default `true`
- `opencode.mcp_servers`: optional list
- `pull_request.open_pr_on_success`: bool, default `true`
- `pull_request.draft_by_default`: bool, default `true`
- `pull_request.reuse_existing_pr`: bool, default `true`
- `pull_request.handoff_project_status`: optional string
- `pull_request.comment_on_issue_with_pr`: bool, default `true`
- `server.port`: optional integer (HTTP server disabled unless set or `--port` CLI flag provided)
- `server.host`: string, default `0.0.0.0`
- `server.read_timeout_ms`: integer, default `30000`
- `server.write_timeout_ms`: integer, default `60000`
- `server.cors_origins`: optional list of strings

## 7. Orchestration State Machine

The orchestrator is the only component that mutates scheduling state. All worker outcomes and
webhook triggers are reported back to it and converted into explicit state transitions.

### 7.1 Work Item Orchestration States

This is not the same as GitHub issue state (`open`, `closed`) or project field values (`Todo`,
`In Progress`, etc.). This is the service's internal claim state.

1. `Unclaimed`
   - Work item is not running and has no retry scheduled.

2. `Claimed`
   - Orchestrator has reserved the work item to prevent duplicate dispatch.
   - In practice, claimed work items are either `Running` or `RetryQueued`.

3. `Running`
   - Worker task exists and the work item is tracked in `running` map.

4. `RetryQueued`
   - Worker is not running, but a retry timer exists.

5. `HandedOff`
   - Work completed a deterministic handoff transition such as PR open + project status move to
     review.
   - The work item is no longer actively executed by the orchestrator, but its latest PR/review
     metadata may still be surfaced.

6. `Released`
   - Claim removed because work item is terminal, non-active, missing, or retry path completed
     without re-dispatch.

Important nuance — persistent autonomous worker model:

Symphony behaves as a persistent autonomous worker on active issues, not a one-shot batch job. It
keeps making progress on a work item until one of these terminal conditions is met:

- The item reaches a deterministic handoff condition (see Section 7.5).
- The item becomes terminal or non-active in the project/issue state.
- A blocker appears (dependency or sub-issue becomes non-terminal).
- A human moves the item out of the execution queue.
- Workflow policy prevents further work.

Specific behavioral rules:

- A successful worker exit does not mean the issue is permanently done.
- Normal completion without handoff is explicitly not terminal. This is expected behavior, not an
  edge case.
- The worker may continue through multiple back-to-back coding-agent turns before it exits.
- After each normal turn completion, the worker re-checks the work item state.
- If the work item is still active and has not reached a deterministic handoff condition, the
  worker starts another turn on the same thread in the same workspace, up to `agent.max_turns`.
- Once the worker exits normally without handoff, the orchestrator schedules a short continuation
  retry (1000 ms) so it can re-check whether the work item remains active and needs another worker
  session. Continuation retries are expected behavior and part of the normal lifecycle.
- If the worker exits after a successful deterministic handoff, the orchestrator records `HandedOff`
  and releases the claim.

### 7.2 Run Attempt Lifecycle

A run attempt transitions through these phases:

1. `PreparingWorkspace`
2. `SyncingRepository`
3. `BuildingPrompt`
4. `LaunchingAgentProcess`
5. `InitializingSession`
6. `StreamingTurn`
7. `ValidatingOutputs`
8. `WritingBackGitHub`
9. `Finishing`
10. `Succeeded`
11. `HandedOffForReview`
12. `Failed`
13. `TimedOut`
14. `Stalled`
15. `CanceledByReconciliation`

### 7.3 Transition Triggers

- `Poll Tick`
  - Reconcile active runs.
  - Validate config.
  - Fetch candidate project items.
  - Dispatch until slots are exhausted.

- `Webhook Delivery`
  - Mark relevant project/issue/PR changes as refresh-worthy.
  - Coalesce refresh signals instead of executing immediate duplicate fetches.

- `Worker Exit (normal)`
  - Remove running entry.
  - Update aggregate runtime totals.
  - Either schedule continuation retry or record handoff depending on outcome.

- `Worker Exit (abnormal)`
  - Remove running entry.
  - Update aggregate runtime totals.
  - Schedule exponential-backoff retry.

- `Codex Update Event`
  - Update live session fields, token counters, and rate limits.

- `Retry Timer Fired`
  - Re-fetch active candidates and attempt re-dispatch, or release claim if no longer eligible.

- `Reconciliation State Refresh`
  - Stop runs whose project value, issue state, or PR state make them ineligible.

- `Stall Timeout`
  - Kill worker and schedule retry.

### 7.4 Idempotency and Recovery Rules

- The orchestrator serializes state mutations through one authority to avoid duplicate dispatch.
- `claimed` and `running` checks are required before launching any worker.
- Reconciliation runs before dispatch on every tick.
- Restart recovery is GitHub-driven and filesystem-driven.
- Startup cleanup removes stale terminal workspaces for project items or issues already terminal.
- Deterministic GitHub write-back operations should use idempotency keys or natural dedupe rules
  wherever possible.

### 7.5 Deterministic Handoff Condition

A work item transitions to `HandedOff` only when a deterministic handoff condition is satisfied.

PR creation alone is not sufficient for handoff. The default deterministic handoff rule requires
both:

1. A pull request exists or was created/updated for the work item during this run.
2. The project status field was moved to the configured `pull_request.handoff_project_status` value
   (for example `Human Review`).

Optionally, for stronger guarantees:

3. All checks listed in `pull_request.required_before_handoff_checks` are satisfied, if configured.

Handoff strength levels:

- `PR only` = progress artifact, not handoff. The agent may open draft or partial PRs during
  intermediate turns. This alone must not trigger `HandedOff`.
- `PR + project status transition to configured handoff value` = standard handoff. This is the
  default deterministic rule.
- `PR + handoff status + required checks` = strong handoff. Use when CI/test gating is required
  before declaring the work item ready for human review.

Rationale:

- PR creation by itself is too weak because agents may open draft or partial PRs during
  intermediate work.
- The project status field provides an explicit, auditable workflow transition.
- Required checks make the handoff operationally safe when teams care about CI/test gating.
- GitHub Projects are designed around custom fields and workflow status tracking, making the
  status field the natural handoff signal.

If `pull_request.handoff_project_status` is not configured, the worker never triggers `HandedOff`
state. The work item continues through continuation retries until it becomes terminal or non-active
through external state changes.

### 7.6 Graceful Shutdown and Signal Handling

The service must handle `SIGTERM` and `SIGINT` for graceful shutdown.

Shutdown sequence:

1. Stop accepting new dispatches. Cancel the poll tick timer.
2. Stop the webhook ingress server (drain in-flight requests).
3. For each running worker:
   a. Send `session/cancel` to the adapter.
   b. Wait up to a configurable grace period (default: `30000 ms`) for clean worker exit.
   c. If the worker does not exit within the grace period, force-kill the subprocess.
4. Persist current retry state to the local state store (bbolt).
5. Stop the HTTP server (drain in-flight requests).
6. Flush and close log sinks.
7. Exit with code 0.

If a second `SIGTERM`/`SIGINT` arrives during graceful shutdown, force-exit immediately with
code 1.

Invariant: no orphaned agent subprocesses after shutdown. The implementation must use OS-level
process group management or `kill-on-parent-exit` (`Pdeathsig` on Linux) to enforce this.

## 8. Polling, Scheduling, and Reconciliation

### 8.1 Poll Loop

At startup, the service validates config, performs startup cleanup, schedules an immediate tick, and
then repeats every `polling.interval_ms`.

The effective poll interval should be updated when workflow config changes are re-applied.

Tick sequence:

1. Reconcile running work items.
2. Run dispatch preflight validation.
3. Fetch candidate project items from GitHub using configured project and status field rules.
4. Sort work items by dispatch priority.
5. Dispatch eligible work items while slots remain.
6. Notify observability/status consumers of state changes.

If per-tick validation fails, dispatch is skipped for that tick, but reconciliation still happens
first.

### 8.2 Candidate Selection Rules

A work item is dispatch-eligible only if all are true:

- It has `project_item_id`, `title`, and enough metadata to resolve a repository-backed issue.
- Its project status is in `active_values` and not in `terminal_values`.
- Its content type is in `executable_item_types`.
- If `tracker.require_issue_backing=true`, the project item resolves to a backing issue.
- The backing issue is open.
- The repository is allowed by repo allowlist/denylist policy.
- Required labels, if configured, are present.
- It is not already in `running`.
- It is not already in `claimed`.
- Global concurrency slots are available.
- Per-project-status concurrency slots are available.
- Per-repository concurrency slots are available.
- Blocker rule passes:
  - If the issue has dependencies or parent/child blockers that are non-terminal, do not dispatch.
- Existing PR policy passes:
  - If an open PR already exists and workflow policy says this item is already handed off, do not
    dispatch.

Sorting order (stable intent):

1. `priority` ascending
2. `created_at` oldest first
3. `issue_identifier` or fallback `project_item_id` lexicographic tie-breaker

### 8.3 Concurrency Control

Global limit:

- `available_slots = max(max_concurrent_agents - running_count, 0)`

Per-project-status limit:

- `max_concurrent_agents_by_project_status[status]` if present
- otherwise fallback to global limit

Per-repository limit:

- `max_concurrent_agents_by_repo[repo]` if present
- otherwise unrestricted beyond global limit

The runtime counts work items by their current tracked project status and repository in the
`running` map.

### 8.4 Retry and Backoff

Retry entry creation:

- Cancel any existing retry timer for the same work item.
- Store `attempt`, `issue_identifier`, `error`, `due_at_ms`, and timer handle.

Backoff formula:

- Normal continuation retries after a clean worker exit use a short fixed delay of `1000` ms.
- Failure-driven retries use
  `delay = min(10000 * 2^(attempt - 1), agent.max_retry_backoff_ms)`.

Retry handling behavior:

1. Fetch current candidate project items.
2. Find the specific work item by `work_item_id` or `project_item_id`.
3. If not found, release claim.
4. If found and still candidate-eligible:
   - Dispatch if slots are available.
   - Otherwise requeue with error `no available orchestrator slots`.
5. If found but no longer active, release claim.

### 8.5 Active Run Reconciliation

Reconciliation runs every tick and has three parts.

Part A: Stall detection

- For each running work item, compute `elapsed_ms` since:
  - `last_agent_timestamp` if any event has been seen, else
  - `started_at`
- If `elapsed_ms > agent.stall_timeout_ms`, terminate the worker and queue a retry.
- If `stall_timeout_ms <= 0`, skip stall detection.

Part B: GitHub state refresh

- Refresh current project item state and backing issue state for all running work items.
- For each running work item:
  - If project status is terminal or issue state is terminal: terminate worker and clean workspace
    when configured.
  - If project status is still active and issue is still open: update the in-memory work item
    snapshot.
  - If project status is neither active nor terminal: terminate worker without terminal cleanup.
  - If the work item moved to a blocked state: terminate worker and release or retry according to
    workflow policy.
  - If a PR handoff state already exists externally: terminate worker and mark as handed off.

Part C: Coalesced webhook refresh

- If webhook ingress has queued a refresh flag, consume it and run one extra reconciliation-aware
  fetch without duplicating concurrent refresh work.

### 8.6 Startup Terminal Workspace Cleanup

When the service starts:

1. Query GitHub for project items in terminal values and/or issues already closed for the configured
   project.
2. For each returned work item, remove the corresponding workspace directory when
   `workspace.remove_on_terminal=true`.
3. If the terminal-items fetch fails, log a warning and continue startup.

## 9. Workspace, Repository, and Safety

### 9.1 Workspace Layout

Workspace root:

- `workspace.root`

Recommended internal layout:

- `<workspace.repo_cache_dir>/<owner>/<repo>` for reusable repository caches
- `<workspace.worktree_dir>/<workspace_key>` for per-work-item worktrees or isolated clones

Per-work-item workspace path:

- `<workspace.worktree_dir>/<sanitized_owner>_<sanitized_repo>_<sanitized_issue_or_item>`

Workspace persistence:

- Workspaces are reused across runs for the same work item.
- Successful runs do not auto-delete workspaces unless policy explicitly requires it.

### 9.2 Workspace Creation and Reuse

Input: `repository`, `issue_number`, `project_item_id`

Algorithm summary:

1. Resolve repository binding and installation-scoped auth.
2. Sanitize repository + issue/item metadata to `workspace_key`.
3. Ensure repo cache exists (clone or fetch).
4. Create or reuse isolated worktree/workspace for the work item.
5. Compute base branch and deterministic work branch.
6. Ensure workspace exists as a directory.
7. Mark `created_now=true` only if the workspace was created during this call.
8. If `created_now=true`, run `after_create` hook if configured.

Recommended branch naming:

- `<git.branch_prefix><sanitized_issue_or_item>`

### 9.3 Repository Synchronization

The spec requires deterministic repository preparation.

Conforming behavior:

- The workspace must contain the correct repository for the work item.
- The implementation must fetch current remote state before dispatching a new run attempt.
- The workspace must be on the deterministic work branch for the work item.
- Base branch selection is:
  1. `git.base_branch` if configured
  2. repository default branch otherwise

Failure handling:

- Repo clone/fetch/auth failures return an error for the current attempt.
- Reused workspaces should not be destructively reset on failure unless that policy is explicitly
  chosen and documented.

### 9.4 Workspace Hooks

Supported hooks:

- `hooks.after_create`
- `hooks.before_run`
- `hooks.after_run`
- `hooks.before_remove`

Execution contract:

- Execute in a local shell context appropriate to the host OS, with the workspace directory as
  `cwd`.
- Hook timeout uses `hooks.timeout_ms`; default `60000 ms`.
- Log hook start, failures, and timeouts.

Failure semantics:

- `after_create` failure or timeout is fatal to workspace creation.
- `before_run` failure or timeout is fatal to the current run attempt.
- `after_run` failure or timeout is logged and ignored.
- `before_remove` failure or timeout is logged and ignored.

### 9.5 Safety Invariants

Invariant 1: Run the coding agent only in the per-work-item workspace path.

- Before launching the coding-agent adapter or subprocess, validate:
  - `cwd == workspace_path`

Invariant 2: Workspace path must stay inside configured workspace roots.

- Normalize both paths to absolute.
- Require `workspace_path` to have `workspace.worktree_dir` as a prefix directory.

Invariant 3: Workspace key is sanitized.

- Only `[A-Za-z0-9._-]` allowed in workspace names.

Invariant 4: Repository binding must match work item metadata.

- The repository checked out in the workspace must match the normalized work item repository.

Invariant 5: Branch naming is deterministic.

- The workspace branch must be the derived work-item branch, not an arbitrary branch.

Invariant 6: GitHub credentials used for repository access must be installation-scoped and refreshable.

## 10. Agent Adapter Protocol (Portable Coding Agent Integration)

This section defines the language-neutral contract between Symphony and any supported coding-agent
runtime.

Design intent:

- Symphony orchestration must remain agent-agnostic.
- Adapter-specific behavior is isolated behind one normalized protocol.
- Adapter-specific features remain available through negotiated capabilities and provider-specific
  parameters.
- OpenCode can often be spoken to nearly directly via ACP.
- Codex and Claude integrations are expressed as adapters that map their native runtime behavior
  into the same normalized contract.

### 10.1 Terminology

- `Adapter`
  - The Symphony-facing integration layer for one agent runtime.
- `Provider`
  - The underlying agent runtime (`codex`, `claude_code`, or `opencode`).
- `Session`
  - A long-lived conversation/work context inside one adapter connection.
- `Prompt Turn`
  - One unit of work submitted to an existing session.
- `Normalized Protocol`
  - The vendor-neutral JSON-RPC contract defined in this section.
- `Provider Params`
  - Adapter-specific values passed through without polluting the core contract.
- `_meta`
  - Reserved extension map for tracing, observability, and future evolution.

### 10.2 Protocol Shape and Transport

Normative protocol shape:

- JSON-RPC-style requests, responses, and notifications.
- For stdio transports, messages are UTF-8 JSON objects delimited by newline.
- Stdout carries protocol messages only.
- Stderr may be used for diagnostics and must not be parsed as protocol.
- Implementations may omit the literal `"jsonrpc":"2.0"` field on the wire when the selected
  adapter runtime uses that convention, but the request/response semantics remain JSON-RPC-like.

Normative logical methods:

- `initialize`
- `session/new`
- `session/prompt`
- `session/cancel`
- `session/close`
- `session/update` (notification)
- `session/request_permission` (adapter -> client request)
- `session/request_input` (adapter -> client request)
- `session/request_tool` (adapter -> client request)
- `session/respond_tool`
- `session/respond_input`

Protocol design note:

- This contract intentionally mirrors ACP method structure where practical because OpenCode already
  speaks ACP natively and because ACP provides a good baseline for sessions, prompt turns,
  cancellation, permission requests, and streamed updates.
- Codex and Claude adapters must map their provider-specific semantics into this contract even when
  their native surfaces differ.

### 10.3 Launch Contract

Launch behavior depends on adapter kind.

Common requirements:

- The adapter must always receive the absolute repository workspace path.
- The coding agent must only act inside the per-work-item workspace.
- When the adapter is subprocess/sidecar based, Symphony launches it using the configured
  `agent.command`.
- When `launch_mode` is `subprocess_stdio` or `sidecar_stdio`, the default invocation is
  `bash -lc <agent.command>` with separate stdout/stderr streams.
- When `launch_mode` is `in_process`, the implementation may call the adapter as a library but must
  still honor the same logical request/response contract.

Recommended additional process settings:

- Max line size: 10 MB
- Kill-on-parent-exit where supported
- Explicit workspace cwd validation before first prompt turn

### 10.4 Initialization and Capability Negotiation

`initialize` establishes the connection, protocol version, and feature support.

Example request:

```json
{
  "id": 1,
  "method": "initialize",
  "params": {
    "protocolVersion": 1,
    "clientInfo": {
      "name": "symphony",
      "version": "3.0"
    },
    "requestedProvider": "codex",
    "clientCapabilities": {
      "toolExecution": true,
      "permissionHandling": true,
      "userInputHandling": true,
      "mcp": true,
      "images": false,
      "audio": false
    },
    "_meta": {
      "traceId": "tr_123"
    }
  }
}
```

Example response:

```json
{
  "id": 1,
  "result": {
    "protocolVersion": 1,
    "provider": "codex",
    "adapterInfo": {
      "name": "symphony-codex-adapter",
      "version": "1.0.0"
    },
    "capabilities": {
      "sessionReuse": true,
      "permissionRequests": true,
      "toolRequests": true,
      "inputRequests": true,
      "mcp": true,
      "tokenUsage": true,
      "rateLimits": true,
      "images": false,
      "subagents": false
    },
    "authMethods": []
  }
}
```

Negotiation requirements:

- The client specifies the newest protocol version it supports.
- The adapter returns the negotiated version and its supported capabilities.
- Unknown capabilities must be ignored.
- Unsupported requested provider kind is a typed startup error.

### 10.5 Session Creation and Prompt Turns

#### 10.5.1 `session/new`

Creates a new session bound to one workspace and one work item.

Required parameters:

- `cwd`
- `title`
- `model` (when applicable)
- `provider`
- `providerParams` (optional map)
- `mcpServers` (optional)
- `tools` (optional client-exposed tool descriptors)

Example:

```json
{
  "id": 2,
  "method": "session/new",
  "params": {
    "cwd": "/abs/symphony/worktrees/org_repo_123",
    "title": "org/repo#123: Fix flaky test",
    "provider": "opencode",
    "model": "sonnet",
    "tools": [
      {
        "name": "github.comment_issue",
        "description": "Post a deterministic issue comment",
        "inputSchema": {
          "type": "object",
          "properties": {
            "body": { "type": "string" }
          },
          "required": ["body"]
        }
      }
    ],
    "providerParams": {
      "opencode": {
        "permission_profile": {
          "bash": "ask",
          "edit": "allow"
        }
      }
    }
  }
}
```

Response:

```json
{
  "id": 2,
  "result": {
    "sessionId": "sess_123",
    "native": {
      "providerSessionId": "native_456"
    }
  }
}
```

#### 10.5.2 `session/prompt`

Starts one prompt turn inside an existing session.

Required parameters:

- `sessionId`
- `input` (list of content blocks; text required baseline)
- `continuation` (boolean)
- `providerParams` (optional map for this turn only)

Example:

```json
{
  "id": 3,
  "method": "session/prompt",
  "params": {
    "sessionId": "sess_123",
    "continuation": false,
    "input": [
      {
        "type": "text",
        "text": "Read the failing test and fix the root cause. Run tests before handoff."
      }
    ]
  }
}
```

Completion:

- A prompt turn completes when the adapter returns a `result` for the request.
- Streaming progress arrives separately via `session/update`.
- `session/prompt` result must include a stop reason.

Example result:

```json
{
  "id": 3,
  "result": {
    "stopReason": "completed",
    "summary": "Applied a fix and passed targeted tests."
  }
}
```

Stop reasons:

- `completed`
- `failed`
- `cancelled`
- `timed_out`
- `stalled`
- `input_required`
- `handoff`

### 10.6 Streaming Updates

Adapters emit `session/update` notifications for all incremental progress.

Example:

```json
{
  "method": "session/update",
  "params": {
    "sessionId": "sess_123",
    "update": {
      "kind": "progress",
      "message": "Running test suite",
      "native": {
        "event": "tool_call_update"
      }
    }
  }
}
```

Recommended update kinds:

- `session_started`
- `assistant_text`
- `progress`
- `tool_call_started`
- `tool_call_completed`
- `tool_call_failed`
- `approval_auto_approved`
- `approval_requested`
- `input_requested`
- `warning`
- `notification`
- `token_usage`
- `rate_limits`
- `completed`
- `failed`
- `malformed`

Rules:

- Adapters should surface native event names inside `update.native` when useful.
- Orchestrator logic must depend only on normalized kinds, not on humanized text.
- Unknown update kinds must be logged and ignored unless they violate correctness assumptions.

### 10.7 Permissions, Tool Calls, and User Input

The normalized protocol supports three classes of callback from adapter to Symphony.

#### 10.7.1 Permission request

```json
{
  "id": 7,
  "method": "session/request_permission",
  "params": {
    "sessionId": "sess_123",
    "permission": {
      "kind": "tool_execution",
      "toolName": "bash",
      "reason": "Run pytest for changed module"
    },
    "options": [
      { "id": "allow-once", "kind": "allow_once", "label": "Allow once" },
      { "id": "reject-once", "kind": "reject_once", "label": "Reject" }
    ]
  }
}
```

The client responds with a selected outcome or cancellation.

#### 10.7.2 Tool request

```json
{
  "id": 8,
  "method": "session/request_tool",
  "params": {
    "sessionId": "sess_123",
    "toolCallId": "call_001",
    "tool": {
      "name": "github.update_project_status",
      "input": {
        "status": "Human Review"
      }
    }
  }
}
```

The client responds via `session/respond_tool` with `success`, `output`, and optional structured
error payload.

#### 10.7.3 Input request

```json
{
  "id": 9,
  "method": "session/request_input",
  "params": {
    "sessionId": "sess_123",
    "reason": "Need confirmation before rewriting generated snapshots"
  }
}
```

Policy requirements:

- Requests for permission, tools, or input must never leave a run stuck indefinitely.
- The implementation may auto-approve, deny, or escalate according to configured harness policy.
- Unsupported tool names must return structured failure and allow the run to continue where safe.

### 10.8 Cancellation and Shutdown

- `session/cancel` cancels an in-flight prompt turn.
- `session/close` ends the session and releases provider-native resources.
- If the adapter process exits unexpectedly, the current run attempt fails.
- If `agent.stall_timeout_ms <= 0`, stall detection is disabled.

### 10.9 Timeouts and Error Mapping

Timeouts:

- `agent.read_timeout_ms`
- `agent.turn_timeout_ms`
- `agent.stall_timeout_ms`

Recommended normalized error categories:

- `adapter_not_found`
- `invalid_workspace_cwd`
- `response_timeout`
- `turn_timeout`
- `process_exit`
- `response_error`
- `turn_failed`
- `turn_cancelled`
- `turn_input_required`
- `permission_denied`
- `unsupported_tool_call`
- `provider_auth_required`

### 10.10 Token and Rate-Limit Telemetry

Adapters should expose token usage and rate-limit signals when the provider supports them.

Rules:

- Prefer absolute totals when the provider emits them.
- When only delta events exist, the adapter must normalize them before they reach orchestration.
- Provider-native payloads may be preserved inside `native`, but orchestration totals must use the
  normalized counters.

### 10.11 Adapter-Specific Feature Envelope

To preserve provider-specific features without polluting the core contract:

- `providerParams.<provider>` may appear in `session/new` and `session/prompt`.
- `native` may appear in responses and updates.
- `_meta` may carry tracing and implementation-specific metadata.

Unknown provider params:

- Must be ignored by other adapters.
- Should fail fast when provided to the selected adapter but unsupported by that adapter and the
  implementation chooses strict validation.

### 10.12 Codex Adapter Profile

Underlying runtime:

- Codex app-server.

Reference mapping:

- Adapter launch commonly uses `codex app-server`.
- Native transport is JSON-RPC-like over stdio or websocket.
- `session/new` typically maps to Codex `initialize`, `initialized`, and `thread/start`.
- `session/prompt` typically maps to Codex `turn/start`.
- Session reuse maps to Codex thread reuse.
- Codex-native approvals, sandbox policies, token usage, and rate-limit telemetry should be exposed
  through normalized capability flags and updates.
- Exact Codex payload shapes are version-specific and should come from the generated schema bundle for the
  pinned Codex version.

Codex-specific provider params may include:

- `approval_policy`
- `thread_sandbox`
- `turn_sandbox_policy`

### 10.13 Claude Code Adapter Profile

Underlying runtime:

- Official programmable surface is the Claude Agent SDK (TypeScript).
- The reference adapter is a TypeScript sidecar process located at `sidecar/claude/` in the
  repository root.
- The sidecar communicates with the Go orchestrator over stdio using the normalized JSON-RPC
  protocol.

Sidecar architecture:

- The sidecar is launched via `tsx sidecar/claude/src/index.ts` (or the value of
  `claude.sidecar_command`).
- The sidecar is a thin bridge (~200-300 lines of TypeScript) that:
  - Receives normalized JSON-RPC requests on stdin.
  - Creates and manages Claude Agent SDK sessions.
  - Translates SDK events into `session/update` notifications on stdout.
  - Handles tool callbacks, permission requests, and MCP integration.
  - Returns `session/prompt` results with normalized stop reasons.
- The Go side manages process lifecycle, stdin/stdout pipes, and protocol framing.
- Stderr from the sidecar is captured for diagnostics and must not be parsed as protocol.

Fallback mode:

- When `claude.adapter_mode` is `cli_fallback`, the adapter shells out to the `claude` CLI with
  `--print` and JSON output modes.
- CLI fallback lacks session reuse, streaming updates, and tool callbacks.
- Use only when the full SDK sidecar is unavailable (no Node.js/tsx on the host).

Reference mapping (sidecar mode):

- `session/new` creates a Claude SDK client/session with workspace cwd, allowed tools, MCP servers,
  and provider params.
- `session/prompt` maps to one SDK query turn in the same client/session.
- Session reuse maps to continuing the same Claude SDK conversation.
- Claude SDK messages and result events are translated into `session/update` notifications.
- Built-in SDK tools execute natively inside the sidecar; client-exposed GitHub tools are surfaced
  via MCP/custom tool integration or explicit `session/request_tool` callbacks to the Go host.
- Provider pause/continuation flows are handled inside the sidecar and surfaced as one logical
  Symphony turn unless policy requires user intervention.

Claude-specific provider params may include:

- `allowed_tools`
- `permission_profile`
- `enable_subagents`

Runtime dependency verification:

- At startup, the implementation must verify that `node` (>= 22) and `tsx` are available on
  `$PATH` when the Claude Code adapter is selected.
- If verification fails, emit a typed error and fail startup.

### 10.14 OpenCode Adapter Profile

Underlying runtime:

- OpenCode via ACP.

Reference mapping:

- When feasible, the adapter may proxy OpenCode ACP nearly directly because OpenCode already speaks
  ACP over JSON-RPC via stdio.
- `initialize`, `session/new`, `session/prompt`, `session/cancel`, and `session/update` map closely
  to ACP native methods and notifications.
- Permission requests and streamed updates should preserve ACP-native structure inside `native`
  while also emitting normalized kinds.
- MCP servers may be passed through using ACP session setup fields.

OpenCode-specific provider params may include:

- `permission_profile`
- `resume_session`
- `config_file`

Runtime dependency verification:

- At startup, the implementation must verify that `opencode` is available on `$PATH` when the
  OpenCode adapter is selected.
- If verification fails, emit a typed error and fail startup.

### 10.15 Agent Runner Contract

The `Agent Runner` wraps workspace + prompt + adapter session + deterministic GitHub write-back.

Behavior:

1. Create/reuse workspace for work item.
2. Synchronize repository and ensure deterministic branch.
3. Build prompt from workflow template.
4. Start or reuse adapter session.
5. Forward normalized adapter events to orchestrator.
6. Resolve permission, tool, and input requests according to harness policy.
7. On success, optionally run deterministic validation and GitHub write-back.
8. On any error, fail the worker attempt.

## 11. GitHub Integration Contract

### 11.1 Required Operations

An implementation must support these adapter operations:

1. `fetch_candidate_work_items()`
   - Return project items in configured active values for the configured project.

2. `fetch_terminal_work_items()`
   - Used for startup terminal cleanup.

3. `fetch_work_item_states_by_ids(work_item_ids)`
   - Used for active-run reconciliation.

4. `resolve_repository_binding(work_item)`
   - Resolve installation-scoped repository metadata and auth context.

5. `upsert_pull_request(work_item, branch, base_branch, options)`
   - Deterministic PR handoff operation when configured.

6. `update_project_status(work_item, new_value)`
   - Deterministic project field write-back when configured.

7. `comment_on_issue(work_item, body)`
   - Deterministic issue comment write-back when configured.

8. `convert_draft_issue(project_item_id, repository_id)`
   - Converts a draft project item to a real issue using the
     `convertProjectV2DraftIssueItemToIssue` GraphQL mutation.
   - Called during dispatch when `tracker.allow_draft_issue_conversion=true` and the candidate item
     is a draft issue.
   - After conversion, the item must be re-fetched and re-resolved to obtain the new backing issue
     metadata before dispatch proceeds.

### 11.2 GitHub Auth Requirements

The implementation must support two auth modes behind a common `GitHubAuthProvider` interface:

```
type GitHubAuthProvider interface {
    // Token returns a valid token for the given repository context.
    Token(ctx, repo) -> (token, error)
    // HTTPClient returns an authenticated HTTP client for the given repository.
    HTTPClient(ctx, repo) -> (client, error)
    // Mode returns the active auth mode ("pat" or "app").
    Mode() -> string
}
```

#### 11.2.1 PAT Auth Mode

- Uses a fine-grained Personal Access Token provided via `$GITHUB_TOKEN` environment variable.
- The same token is returned for all repository contexts.
- Required PAT permissions: repository read/write, project read/write, issues read/write.
- No token refresh is needed (PATs have fixed expiry managed externally).
- PAT mode is the recommended starting point for development and initial deployment.

#### 11.2.2 App Auth Mode

- Uses GitHub App credentials (app ID + private key) for API access.
- Uses installation-scoped tokens for repository and API operations.
- Refresh tokens before expiry according to `github.token_refresh_skew_ms`.
- Token refresh failures must surface as typed operational errors.
- App mode is the recommended production auth model.

#### 11.2.3 Auth Mode Resolution

- When `github.auth_mode` is `auto` (default), check for available credentials:
  - If `github.token` resolves, use PAT mode.
  - If `github.app_id` and `github.private_key` resolve, use App mode.
  - If both are present, prefer App mode.
  - If neither is present, fail startup validation.
- The rest of the codebase must only interact with `GitHubAuthProvider` and must not assume which
  auth mode is active.

#### 11.2.4 Client-Side Rate Limiting

- The GitHub HTTP client must implement a client-side token-bucket rate limiter.
- The default rate is `github.rate_limit_qps` queries per second (default: 10).
- This prevents hitting GitHub's server-side rate limits (5000 req/hour for Apps, varies for PATs).
- When the rate limit is exhausted, requests block until a token is available.
- Rate limit state is observable via Prometheus metrics.

### 11.3 Query Semantics

GitHub-specific requirements for `tracker.kind == "github"`:

- Candidate fetch must read from the configured GitHub Project.
- Project field selection and normalization must isolate the configured status field and any
  configured priority field.
- Project items must be resolved to their backing content.
- The adapter must normalize issue state, project field values, dependencies/sub-issues, linked PRs,
  labels, assignees, and repository metadata into the domain model.
- Pagination is required for candidate fetches.
- Network timeout uses `github.request_timeout_ms`.

Query strategy (two-pass):

- **Pass 1**: Fetch project items with status field values, content type, and basic content metadata
  (issue number, repository owner/name, state). This is a lightweight query that pages through all
  project items and applies initial eligibility filtering (active status, correct content type,
  open issue state, repo allowlist/denylist).
- **Pass 2**: For items that pass initial eligibility, fetch full issue details in a batched query:
  dependencies, sub-issues, linked PRs, labels, assignees, milestone, description, and repository
  metadata (default branch, clone URL).

Rationale: the two-pass approach avoids fetching expensive nested data (dependencies, sub-issues,
linked PRs) for project items that will be filtered out by status, content type, or repo policy.
This significantly reduces GitHub API usage for large projects.

### 11.4 Normalization Rules

Normalization should produce fields listed in Section 4.1.1.

Additional normalization details:

- `labels` -> lowercase strings
- `priority` -> integer only
- `project_fields` -> string-friendly map for template compatibility
- `blocked_by` -> derived from GitHub issue dependencies and equivalent relations
- `sub_issues` -> derived from GitHub sub-issue hierarchy
- `issue_identifier` -> `<owner>/<repo>#<number>`

### 11.5 Error Handling Contract

Recommended error categories:

- `unsupported_tracker_kind`
- `missing_github_credentials` (neither PAT nor App credentials are present)
- `missing_github_app_id`
- `missing_github_private_key`
- `missing_github_token`
- `github_auth_error`
- `github_pat_insufficient_permissions`
- `github_installation_resolution_error`
- `github_api_request`
- `github_api_status`
- `github_api_rate_limited`
- `github_graphql_errors`
- `github_unknown_payload`
- `github_webhook_signature_invalid`
- `github_repo_not_allowed`

Orchestrator behavior on GitHub errors:

- Candidate fetch failure: log and skip dispatch for this tick.
- Running-state refresh failure: log and keep active workers running.
- Startup terminal cleanup failure: log warning and continue startup.
- Webhook verification failure: reject delivery and do not mutate runtime state.
- PR/comment/project write-back failure: fail the current attempt unless workflow policy says the
  write-back is best-effort.

### 11.6 GitHub Write-Back Boundary

Unlike v1, deterministic GitHub write-back is part of the harness contract.

Allowed write-back categories may include:

- Issue comments
- Project field updates
- Label updates
- PR create/update
- Branch push support required for PR upsert

Business-rule-heavy issue triage beyond the configured workflow remains out of scope. The service
should expose deterministic write primitives; it should not silently invent repository business
logic.



### 11.7 Optional GitHub Client-Side Tool Extension

An implementation may expose a limited set of typed GitHub client-side tools to the adapter
session when `agent.enable_client_tools=true`.

Goals:

- give the agent deterministic, policy-guarded GitHub read/write primitives
- avoid exposing raw long-lived credentials to the agent
- keep write operations repo-scoped, installation-scoped, and auditable
- make write operations idempotent when practical

Recommended baseline tool set:

1. `github_issue_read`
   - Purpose: fetch canonical issue details for the current work item or a referenced issue.
   - Input:
     - `repository`
     - `issue_number`
   - Output:
     - structured issue payload

2. `github_issue_comment`
   - Purpose: append a deterministic issue comment.
   - Input:
     - `repository`
     - `issue_number`
     - `body`
     - `idempotency_key` (optional but recommended)
   - Output:
     - `success`
     - `comment_url` or structured error

3. `github_project_update_field`
   - Purpose: update one configured project field for the current project item.
   - Input:
     - `project_item_id`
     - `field_name`
     - `value`
     - `idempotency_key` (optional)
   - Output:
     - `success`
     - updated field payload or structured error

4. `github_pull_request_upsert`
   - Purpose: create or update a PR for the current work branch.
   - Input:
     - `repository`
     - `base_branch`
     - `head_branch`
     - `title`
     - `body`
     - `draft`
     - `idempotency_key` (optional)
   - Output:
     - `success`
     - `pull_request_url`
     - `pull_request_number`
     - `created`/`updated`

5. `github_repo_read_file`
   - Purpose: read repository file contents through the GitHub API rather than raw shell access when
     policy requires it.
   - Input:
     - `repository`
     - `path`
     - `ref` (optional)
   - Output:
     - structured file payload or error

Tool policy requirements:

- tools must use installation-scoped auth
- tools must not expose raw tokens to the agent
- tools must enforce repo allowlist/denylist policy
- write tools should reject operations outside the current configured project/repository scope unless
  explicitly allowed
- unsupported tool names should return structured failure without stalling the session

### 11.8 Webhook Ingress Contract

Webhook ingress is optional but strongly recommended.

Behavior:

- verify webhook signature before accepting the delivery
- record minimal delivery metadata for observability
- normalize relevant deliveries into a bounded queue of refresh hints
- never rely on webhook-only correctness

Relevant event families may include:

- `issues`
- `issue_comment`
- `issue_dependencies`
- `sub_issues`
- `pull_request`
- `projects_v2`
- `projects_v2_item`

Operational rule:

- webhook deliveries should trigger faster reconciliation, but polling remains the authoritative
  correctness path

## 12. Prompt Construction and Context Assembly

### 12.1 Inputs

Inputs to prompt rendering:

- `workflow.prompt_template`
- normalized `work_item`
- normalized `repository`
- optional `attempt`
- deterministic branch/base-branch metadata

### 12.2 Rendering Rules

- Render with strict variable checking.
- Preserve nested arrays/maps (labels, blockers, sub-issues, project fields, linked PRs).

### 12.3 Retry/Continuation Semantics

`attempt` should be passed to the template because the workflow prompt may provide different
instructions for:

- first run
- continuation run after a successful prior session
- retry after error/timeout/stall
- rerun after review feedback

### 12.4 Failure Semantics

If prompt rendering fails:

- Fail the run attempt immediately.
- Let the orchestrator treat it like any other worker failure.

## 13. Logging, Status, and Observability

### 13.1 Logging Conventions

Required context fields for work-item-related logs:

- `work_item_id`
- `project_item_id`
- `issue_identifier`
- `repository`

Required context for coding-agent session lifecycle logs:

- `session_id`

Message formatting requirements:

- Use stable `key=value` phrasing.
- Include action outcome.
- Include concise failure reason when present.
- Avoid logging large raw payloads unless necessary.

### 13.2 Logging Outputs and Sinks

The spec does not prescribe where logs must go.

Requirements:

- Operators must be able to see startup, validation, auth, dispatch, and write-back failures
  without attaching a debugger.
- If a configured log sink fails, the service should continue running when possible.

### 13.3 Runtime Snapshot / Monitoring Interface

If the implementation exposes a synchronous runtime snapshot, it should return:

- `running`
- `retrying`
- `agent_totals`
  - `input_tokens`
  - `output_tokens`
  - `total_tokens`
  - `seconds_running`
  - `github_writebacks`
  - `sessions_started`
- `rate_limits`
- `pending_refresh`

### 13.4 Optional Human-Readable Status Surface

A human-readable status surface is optional and implementation-defined.

If present, it should draw from orchestrator state/metrics only and must not be required for
correctness.

### 13.5 Session Metrics and Token Accounting

Token accounting rules:

- Prefer absolute provider/session totals when available.
- Ignore delta-style payloads unless the adapter normalizes them to cumulative values.
- Accumulate aggregate totals in orchestrator state.
- Track deterministic GitHub write-back counts separately from agent token metrics.
- Preserve provider-native token/rate-limit payloads under `native` when useful for debugging, but
  keep orchestration math strictly on normalized counters.

### 13.6 Optional Humanized Agent Event Summaries

Humanized summaries of raw agent protocol events are optional.

If implemented:

- Treat them as observability-only output.
- Do not make orchestrator logic depend on humanized strings.

### 13.7 HTTP Server

The HTTP server provides API, webhook ingress, health checks, and Prometheus metrics.

The HTTP server is implemented using the `chi` router and shares a single listener for all
endpoints. It is not required for orchestrator correctness — the orchestrator operates correctly
without it.

Enablement:

- Start the HTTP server when a CLI `--port` argument is provided.
- Start the HTTP server when `server.port` is present in `WORKFLOW.md` front matter.
- CLI `--port` overrides `server.port`.
- Recommended default port: `9097`.

Required endpoints:

- `GET /healthz` — health check endpoint (see Section 13.9).
- `GET /metrics` — Prometheus metrics endpoint (see Section 13.8).

Recommended API endpoints:

- `GET /api/v1/state` — full orchestrator runtime snapshot.
- `GET /api/v1/work-items/<encoded-id>` — single work item details.
- `POST /api/v1/refresh` — trigger immediate reconciliation.
- `POST /api/v1/webhooks/github` — webhook ingress (if enabled).

### 13.8 Prometheus Metrics

The implementation must expose Prometheus-format metrics at `GET /metrics`.

Required metrics:

- `symphony_active_runs` (gauge) — current count of running work items.
- `symphony_max_concurrent_agents` (gauge) — configured concurrency limit.
- `symphony_dispatches_total` (counter) — total dispatches, labeled by outcome.
- `symphony_work_item_state` (gauge) — count of work items by orchestration state.
- `symphony_tokens_total` (counter) — cumulative token usage, labeled by direction
  (input/output).
- `symphony_github_api_duration_seconds` (histogram) — GitHub API call latency.
- `symphony_github_api_rate_limit_remaining` (gauge) — remaining GitHub API rate limit.
- `symphony_retry_queue_depth` (gauge) — current retry queue size.
- `symphony_errors_total` (counter) — errors labeled by category (workspace, github, adapter,
  writeback).
- `symphony_agent_session_duration_seconds` (histogram) — agent session wall-clock duration.
- `symphony_pr_handoffs_total` (counter) — successful PR handoff count.
- `symphony_github_writebacks_total` (counter) — total GitHub write-back operations, labeled by
  type (pr, comment, project_field, label).

Metrics must not affect orchestrator correctness. If the metrics subsystem fails, the orchestrator
must continue operating.

### 13.9 Health Check Endpoint

The implementation must expose a health check at `GET /healthz`.

Response:

- `200 OK` with JSON body when the service is healthy:
  ```json
  {
    "status": "ok",
    "uptime_seconds": 3600,
    "auth_mode": "pat",
    "running_count": 3,
    "last_poll_at": "2026-03-24T10:00:00Z"
  }
  ```
- `503 Service Unavailable` when the service is unhealthy (e.g., config validation failing,
  GitHub auth broken).

The health check is intended for process supervisors (systemd, Docker, Kubernetes) and must
respond quickly (< 100 ms) without blocking on external calls.

## 14. Failure Model and Recovery Strategy

### 14.1 Failure Classes

1. `Workflow/Config Failures`
   - Missing `WORKFLOW.md`
   - Invalid YAML front matter
   - Missing GitHub App config
   - Unsupported tracker kind
   - Missing or invalid agent adapter launch configuration

2. `GitHub/Auth Failures`
   - PAT missing or invalid
   - PAT insufficient permissions
   - App auth failure
   - Installation resolution failure
   - Installation token refresh failure
   - Webhook signature failure
   - API transport or GraphQL errors
   - Client-side rate limit exhaustion

3. `Workspace/Repository Failures`
   - Workspace directory creation failure
   - Repository clone/fetch/auth failure
   - Invalid workspace path configuration
   - Hook timeout/failure

4. `Agent Session Failures`
   - Runtime dependency missing (`node`/`tsx`/`opencode`/`codex` not on `$PATH`)
   - Sidecar process launch failure
   - Startup handshake failure
   - Turn failed/cancelled
   - Turn timeout
   - User input requested
   - Subprocess exit
   - Stalled session

5. `Write-Back Failures`
   - Branch push failure
   - PR upsert failure
   - Project field update failure
   - Issue comment failure

6. `Observability Failures`
   - Snapshot timeout
   - Dashboard render errors
   - Log sink failure

### 14.2 Recovery Behavior

- Dispatch validation failures:
  - Skip new dispatches.
  - Keep service alive.
  - Continue reconciliation where possible.

- Worker failures:
  - Convert to retries with exponential backoff.

- GitHub candidate-fetch failures:
  - Skip this tick.
  - Try again on next tick.

- Reconciliation refresh failures:
  - Keep current workers.
  - Retry on next tick.

- Write-back failures:
  - Fail or warn according to workflow policy, but never silently succeed.

- Dashboard/log failures:
  - Do not crash the orchestrator.

### 14.3 Partial State Recovery (Restart)

The orchestrator uses a hybrid in-memory + lightweight persistent state model.

In-memory state (lost on restart):

- Running worker handles and session metadata.
- Live token/rate-limit snapshots.
- Coalesced webhook refresh flags.

Persistent state (survives restart via bbolt):

- Retry queue entries (work item ID, attempt number, error, scheduled time).
- Last-known session metadata for work items (session ID, native IDs, last status).
- Aggregate token counters.

The persistent store uses bbolt (`go.etcd.io/bbolt`), an embedded key-value database stored at
`<state_dir>/symphony.db` (default: `<workspace.root>/.symphony/symphony.db`, overridable via
CLI `--state-dir`).

After restart:

- Restore retry entries from bbolt and re-schedule timers.
- No running sessions are assumed recoverable (subprocesses are gone).
- Service also recovers by:
  - startup terminal workspace cleanup
  - fresh polling of active project items
  - re-dispatching eligible work items
- Polling-based recovery remains the authoritative correctness path; bbolt persistence is an
  optimization that avoids losing retry backoff state.

### 14.4 Operator Intervention Points

Operators can control behavior by:

- Editing `WORKFLOW.md`
- Changing project field values in GitHub Projects
- Editing issue state, labels, assignees, dependencies, or PR state in GitHub
- Restarting the service for process recovery or deployment

## 15. Security and Operational Safety

### 15.1 Trust Boundary Assumption

Each implementation defines its own trust boundary.

Operational safety requirements:

- State clearly whether the service is intended for trusted environments, more restrictive
  environments, or both.
- State clearly whether it relies on auto-approved actions, operator approvals, stricter sandboxing,
  or external isolation.

### 15.2 Filesystem and Repository Safety Requirements

Mandatory:

- Workspace path must remain under configured workspace roots.
- Coding-agent cwd must be the per-work-item workspace path.
- Workspace directory names must use sanitized identifiers.
- Repository checkout must match the normalized work item repository.

Recommended hardening:

- Run under a dedicated OS user.
- Restrict workspace root permissions.
- Prefer isolated worktrees or isolated clones.
- Restrict git credential exposure to installation-scoped tokens only.

### 15.3 Secret Handling

- Support `$VAR` indirection in workflow config.
- Do not log private keys, tokens, or secret env values.
- Refresh installation tokens before expiry.
- Erase transient credentials from process memory/logging surfaces when practical.

### 15.4 Hook Script Safety

Workspace hooks are arbitrary shell scripts from `WORKFLOW.md`.

Implications:

- Hooks are fully trusted configuration.
- Hook output should be truncated in logs.
- Hook timeouts are required.

### 15.5 Harness Hardening Guidance

Running coding agents against repositories, issues, PR comments, project fields, and other GitHub
content can be dangerous. A permissive deployment can lead to data leaks, destructive mutations, or
machine compromise if the agent is induced to execute harmful commands or use overly-powerful
integrations.

Possible hardening measures include:

- Tightening Codex approval and sandbox settings instead of running with a maximally permissive
  configuration.
- Adding external isolation layers such as OS/container/VM sandboxing, network restrictions, or
  separate credentials beyond built-in agent policy controls.
- Filtering which GitHub projects, repos, labels, item types, or project statuses are eligible for
  dispatch.
- Reducing the set of client-side tools, credentials, filesystem paths, and network destinations
  available to the agent to the minimum needed.
- Treating issue bodies, comments, PR comments, and repo content as semi-trusted inputs rather than
  authoritative workflow instructions.
- Separating read tools from write tools and making write tools idempotent and policy-guarded.
- Recording write-back provenance so operators can tell whether the harness or the agent performed a
  change.

## 16. Reference Algorithms (Language-Agnostic)

### 16.1 Service Startup

```text
function start_service():
  configure_logging()
  start_observability_outputs()
  start_workflow_watch(on_change=reload_and_reapply_workflow)
  start_optional_github_webhook_server()

  state = {
    poll_interval_ms: get_config_poll_interval_ms(),
    max_concurrent_agents: get_config_max_concurrent_agents(),
    running: {},
    claimed: set(),
    retry_attempts: {},
    completed: set(),
    agent_totals: {input_tokens: 0, output_tokens: 0, total_tokens: 0, seconds_running: 0, github_writebacks: 0},
    agent_rate_limits: null,
    pending_refresh: false,
    recent_webhook_events: []
  }

  validation = validate_dispatch_config()
  if validation is not ok:
    log_validation_error(validation)
    fail_startup(validation)

  startup_terminal_workspace_cleanup()
  schedule_tick(delay_ms=0)

  event_loop(state)
```

### 16.2 Poll-and-Dispatch Tick

```text
on_tick(state):
  state = reconcile_running_work_items(state)

  validation = validate_dispatch_config()
  if validation is not ok:
    log_validation_error(validation)
    notify_observers()
    schedule_tick(state.poll_interval_ms)
    return state

  work_items = github.fetch_candidate_work_items()
  if work_items failed:
    log_github_error()
    notify_observers()
    schedule_tick(state.poll_interval_ms)
    return state

  for work_item in sort_for_dispatch(work_items):
    if no_available_slots(state):
      break

    if should_dispatch(work_item, state):
      state = dispatch_work_item(work_item, state, attempt=null)

  notify_observers()
  schedule_tick(state.poll_interval_ms)
  return state
```

### 16.3 Reconcile Active Runs

```text
function reconcile_running_work_items(state):
  state = reconcile_stalled_runs(state)

  running_ids = keys(state.running)
  if running_ids is empty and state.pending_refresh == false:
    return state

  refreshed = github.fetch_work_item_states_by_ids(running_ids)
  if refreshed failed:
    log_debug("keep workers running")
    return state

  for work_item in refreshed:
    if work_item.project_status in terminal_values or work_item.state is terminal:
      state = terminate_running_work_item(state, work_item.work_item_id, cleanup_workspace=true)
    else if work_item.project_status in active_values and work_item.state is active:
      state.running[work_item.work_item_id].work_item = work_item
    else:
      state = terminate_running_work_item(state, work_item.work_item_id, cleanup_workspace=false)

  state.pending_refresh = false
  return state
```

### 16.4 Dispatch One Work Item

```text
function dispatch_work_item(work_item, state, attempt):
  worker = spawn_worker(
    fn -> run_agent_attempt(work_item, attempt, parent_orchestrator_pid) end
  )

  if worker spawn failed:
    return schedule_retry(state, work_item.work_item_id, next_attempt(attempt), {
      issue_identifier: work_item.issue_identifier,
      error: "failed to spawn agent"
    })

  state.running[work_item.work_item_id] = {
    worker_handle,
    monitor_handle,
    issue_identifier: work_item.issue_identifier,
    repository: work_item.repository.full_name,
    work_item,
    session_id: null,
    adapter_process_pid: null,
    last_agent_message: null,
    last_agent_event: null,
    last_agent_timestamp: null,
    agent_input_tokens: 0,
    agent_output_tokens: 0,
    agent_total_tokens: 0,
    last_reported_input_tokens: 0,
    last_reported_output_tokens: 0,
    last_reported_total_tokens: 0,
    retry_attempt: normalize_attempt(attempt),
    started_at: now_utc()
  }

  state.claimed.add(work_item.work_item_id)
  state.retry_attempts.remove(work_item.work_item_id)
  return state
```

### 16.5 Worker Attempt (Workspace + Prompt + Agent + Write-Back)

```text
function run_agent_attempt(work_item, attempt, orchestrator_channel):
  workspace = workspace_manager.create_for_work_item(work_item)
  if workspace failed:
    fail_worker("workspace error")

  if run_hook("before_run", workspace.path) failed:
    fail_worker("before_run hook error")

  session = app_server.start_session(workspace=workspace.path)
  if session failed:
    run_hook_best_effort("after_run", workspace.path)
    fail_worker("agent session startup error")

  max_turns = config.agent.max_turns
  turn_number = 1

  while true:
    prompt = build_turn_prompt(workflow_template, work_item, attempt, turn_number, max_turns)
    if prompt failed:
      app_server.stop_session(session)
      run_hook_best_effort("after_run", workspace.path)
      fail_worker("prompt error")

    turn_result = app_server.run_turn(
      session=session,
      prompt=prompt,
      work_item=work_item,
      on_message=(msg) -> send(orchestrator_channel, {agent_update, work_item.work_item_id, msg})
    )

    if turn_result failed:
      app_server.stop_session(session)
      run_hook_best_effort("after_run", workspace.path)
      fail_worker("agent turn error")

    refreshed_work_item = github.fetch_work_item_states_by_ids([work_item.work_item_id])
    if refreshed_work_item failed:
      app_server.stop_session(session)
      run_hook_best_effort("after_run", workspace.path)
      fail_worker("work item state refresh error")

    work_item = refreshed_work_item[0] or work_item

    if work_item is not active:
      break

    if handoff_already_exists(work_item):
      break

    if turn_number >= max_turns:
      break

    turn_number = turn_number + 1

  app_server.stop_session(session)

  if should_perform_writeback(work_item, workspace):
    writeback_result = github_writeback(work_item, workspace)
    if writeback_result failed:
      run_hook_best_effort("after_run", workspace.path)
      fail_worker("github writeback error")

  run_hook_best_effort("after_run", workspace.path)
  exit_normal()
```

### 16.6 Worker Exit and Retry Handling

```text
on_worker_exit(work_item_id, reason, state):
  running_entry = state.running.remove(work_item_id)
  state = add_runtime_seconds_to_totals(state, running_entry)

  if reason == handed_off:
    state.completed.add(work_item_id)
    state.claimed.remove(work_item_id)
    state = record_handoff(state, work_item_id)
  else if reason == normal:
    state.completed.add(work_item_id)
    state = schedule_retry(state, work_item_id, 1, {
      issue_identifier: running_entry.issue_identifier,
      delay_type: continuation
    })
  else:
    state = schedule_retry(state, work_item_id, next_attempt_from(running_entry), {
      issue_identifier: running_entry.issue_identifier,
      error: format("worker exited: %reason")
    })

  notify_observers()
  return state
```

## 17. Test and Validation Matrix

### 17.1 Workflow and Config Parsing

- Workflow file path precedence works
- Workflow file changes are detected and trigger re-read/re-apply without restart
- Invalid workflow reload keeps last known good effective configuration
- Missing `WORKFLOW.md` returns typed error
- Invalid YAML front matter returns typed error
- Config defaults apply when optional values are missing
- `tracker.kind` validation enforces `github`
- GitHub App config works including `$VAR` indirection
- Prompt template renders `work_item`, `repository`, and `attempt`
- Prompt rendering fails on unknown variables

### 17.2 Workspace Manager, Git, and Safety

- Deterministic workspace path per repository + issue/item
- Missing repo cache is created
- Existing repo cache is reused
- Deterministic branch creation works
- Existing workspace is reused
- `after_create` hook runs only on new workspace creation
- `before_run` hook runs before each attempt and failures abort the attempt
- `after_run` hook runs after each attempt and failures are logged and ignored
- `before_remove` hook runs on cleanup and failures are ignored
- Workspace path sanitization and root containment invariants are enforced before agent launch
- Agent launch uses workspace cwd
- Repository checked out in the workspace matches the normalized work item repository

### 17.3 GitHub Source Adapter and Auth

- Candidate fetch uses configured owner/project_number/status field rules
- Two-pass GraphQL: first pass gets items + status, second pass gets full details for eligible
- Pagination preserves order across multiple pages
- Project items resolve to backing issues correctly
- Draft issues are rejected or converted according to workflow config
- Draft issue conversion via `convertProjectV2DraftIssueItemToIssue` works when enabled
- Dependencies and sub-issues are normalized
- Labels are normalized to lowercase
- Linked PRs are normalized
- PAT auth mode works with fine-grained token
- App auth mode works with installation token refresh
- Auth mode auto-detection resolves correctly (PAT vs App vs both)
- Client-side rate limiter throttles requests correctly
- Error mapping for request errors, GraphQL errors, malformed payloads, rate limits, and auth
  failures

### 17.4 Orchestrator Dispatch, Reconciliation, and Retry

- Dispatch sort order is priority then oldest creation time
- Blocked issue is not eligible
- Active work item refresh updates running entry state
- Non-active project value stops running agent without terminal cleanup
- Terminal project value or closed issue stops running agent and cleans workspace when configured
- Reconciliation with no running items is a no-op
- Normal worker exit schedules short continuation retry
- Handoff worker exit releases claim
- Abnormal worker exit increments retries with exponential backoff
- Retry backoff cap uses configured `agent.max_retry_backoff_ms`
- Stall detection kills stalled sessions and schedules retry
- Slot exhaustion requeues retries with explicit error reason
- Deterministic handoff requires PR + project status transition (PR alone insufficient)
- Strong handoff with required checks prevents premature handoff
- Missing `handoff_project_status` config means handoff never triggers

### 17.5 Agent Adapter Protocol and Runtime Adapters

Core normalized protocol:

- `initialize` negotiates protocol version and capabilities
- `session/new` creates a session bound to the workspace
- `session/prompt` drives one prompt turn and returns a normalized stop reason
- `session/cancel` aborts an in-flight turn
- `session/update` streams progress, tool updates, usage, and warnings
- Permission requests, tool requests, and input requests are resolved according to policy
- Partial JSON lines are buffered until newline for stdio transports
- Stdout and stderr are handled separately
- Usage and rate-limit payloads are normalized correctly

Codex adapter:

- Launch command uses workspace cwd and invokes the configured effective Codex adapter command
- Codex initialize/thread-start/turn-start flows map correctly into normalized session methods
- Generated schema artifacts for the pinned Codex version are used when strict field-level
  validation is enabled

Claude Code adapter:

- TypeScript sidecar launches via `tsx` and establishes stdio JSON-RPC communication
- Claude SDK session creation and continuation map correctly into normalized session methods
- Built-in tools, MCP/custom tools, and pause/continuation flows are handled without losing
  orchestration correctness
- CLI fallback mode works when sidecar is unavailable
- Startup verification checks for `node` (>= 22) and `tsx` on `$PATH`

OpenCode adapter:

- ACP initialization, session creation, prompting, cancellation, and updates map correctly into the
  normalized session methods
- ACP permission requests and streamed updates are translated into normalized events without losing
  provider-native detail
- Startup verification checks for `opencode` on `$PATH`

### 17.6 GitHub Write-Back and Harness Tools

- PR upsert is idempotent under rerun conditions
- Issue comment write-back is idempotent or naturally deduped
- Project field update uses configured field semantics
- Unsupported GitHub client-side tools return structured failure without stalling
- Read-only and write tool policies are enforced separately when implemented

### 17.7 Observability

- Validation failures are operator-visible
- Structured logging includes work item/session context fields
- Logging sink failures do not crash orchestration
- Token aggregation remains correct across repeated agent updates
- If a status surface is implemented, it is driven from orchestrator state and does not affect
  correctness

### 17.8 CLI and Host Lifecycle

- CLI accepts an optional positional workflow path argument
- CLI uses `./WORKFLOW.md` when no workflow path argument is provided
- CLI errors on nonexistent explicit workflow path or missing default `./WORKFLOW.md`
- CLI surfaces startup failure cleanly
- CLI exits with success when application starts and shuts down normally
- CLI exits nonzero when startup fails or the host process exits abnormally
- `--doctor` flag validates config, checks GitHub connectivity, and verifies agent runtime
  availability without starting the orchestrator
- `--port` flag enables HTTP server and overrides `server.port`
- `--log-format` flag accepts `text` or `json`
- `--log-level` flag accepts `debug`, `info`, `warn`, `error`
- `--state-dir` flag overrides persistent state directory
- SIGTERM/SIGINT triggers graceful shutdown per Section 7.6
- Second SIGTERM/SIGINT during shutdown triggers immediate exit

### 17.9 Doctor Validation

The `--doctor` flag runs comprehensive environment validation and exits:

- Workflow file can be loaded and parsed
- All required config fields are present after `$` resolution
- GitHub auth credentials are valid (test API call)
- GitHub Project is accessible with configured owner/project_number
- `git` CLI is available and functional
- Agent runtime binary is available on `$PATH`:
  - `claude_code`: verify `node` (>= 22) and `tsx`
  - `opencode`: verify `opencode`
  - `codex`: verify `codex`
- Workspace root directory is writable
- State directory is writable
- If `server.port` or `--port` is set, verify the port is available

Output format: one line per check with pass/fail/skip status and diagnostic detail on failure.
Exit code 0 if all checks pass, nonzero otherwise.

### 17.10 Graceful Shutdown

- SIGTERM/SIGINT stops new dispatches immediately
- Active agent sessions receive `session/cancel`
- Workers exit within grace period or are force-killed
- Retry state is persisted to bbolt before exit
- HTTP server drains in-flight requests
- No orphaned agent subprocesses after exit

### 17.11 Metrics and Health

- `/healthz` responds within 100 ms without external calls
- `/metrics` exposes all required Prometheus metrics from Section 13.8
- Metrics subsystem failure does not crash the orchestrator
- Token counters are consistent across agent updates

## 18. Implementation Checklist (Definition of Done)

### 18.1 Core Orchestration

- `WORKFLOW.md` loader with YAML front matter + prompt body split
- Typed config layer with defaults and `$VAR` resolution
- Dynamic workflow watch/reload/re-apply via `fsnotify` with debounce
- Polling orchestrator with single-goroutine state ownership and channel-based worker communication
- Dispatch with priority sorting, eligibility checks, and bounded concurrency
- Continuation retries (normal exit without handoff schedules 1000 ms retry)
- Exponential backoff retries for failures
- Reconciliation that stops runs on terminal/non-active GitHub state
- Deterministic handoff detection (PR + project status transition per Section 7.5)
- Graceful shutdown with SIGTERM/SIGINT handling per Section 7.6
- Persistent retry state via bbolt

### 18.2 GitHub Integration

- GitHub source adapter with candidate fetch (two-pass GraphQL) + state refresh + terminal fetch
- Pluggable auth: PAT mode (`$GITHUB_TOKEN`) and GitHub App mode behind `GitHubAuthProvider`
  interface
- Client-side token-bucket rate limiter on GitHub HTTP client
- PR upsert, issue comment, project field update write-back operations
- Typed GitHub client-side tools (issue read, comment, project field update, PR upsert, file read)
- Webhook ingress with signature verification and coalesced refresh signals
- Draft issue conversion via `convertProjectV2DraftIssueItemToIssue` mutation when
  `allow_draft_issue_conversion=true`

### 18.3 Workspace and Git

- Repository workspace manager with deterministic workspaces and branches
- Git CLI operations: clone, fetch, worktree add, branch, push
- Workspace lifecycle hooks (after_create, before_run, after_run, before_remove)
- Terminal workspace cleanup
- Safety invariants: path containment, key sanitization, repo binding match, deterministic branches

### 18.4 Agent Adapters

- Portable agent adapter protocol implementation (JSON-RPC over stdio)
- Claude Code adapter: TypeScript Agent SDK sidecar via `tsx` (primary) + CLI fallback mode
- OpenCode adapter: thin ACP proxy over subprocess stdio
- Codex adapter: `codex app-server` over subprocess stdio
- Runtime dependency verification at startup for selected adapter
- All three adapters conforming in a single deployment

### 18.5 Prompt and Template

- Strict prompt rendering with Go `text/template` and `missingkey=error`
- Template variables: `work_item`, `issue`, `repository`, `attempt`, `branch_name`, `base_branch`,
  `project_fields`
- Fallback prompt when body is empty

### 18.6 Observability

- Structured logs via `log/slog` with work-item/session context fields
- HTTP server with `chi` router serving API, webhooks, health, and metrics
- Prometheus metrics at `/metrics` per Section 13.8
- Health check at `/healthz` per Section 13.9
- Runtime snapshot API at `/api/v1/state`

### 18.7 CLI

- `symphony [WORKFLOW_PATH]` with positional argument
- `--port`, `--log-format`, `--log-level`, `--state-dir` flags
- `--doctor` flag for comprehensive environment validation per Section 17.9

### 18.8 Infrastructure

- Dockerfile with multi-stage build: Go binary + Node.js 22 runtime + tsx + TS sidecar
- `docker-compose.yml` with Symphony, VictoriaMetrics, and Grafana
- Pre-built Grafana dashboard JSON
- VictoriaMetrics scrape configuration
- GitHub Actions CI workflow (lint, test, docker build, gated integration tests)
- Makefile with targets: build, test, test-integration, lint, docker-build, docker-up, doctor,
  sidecar, clean
- SSH worker extension (Appendix A)

### 18.9 Testing

- Unit tests for config parsing, workspace key sanitization, eligibility logic, prompt rendering,
  retry backoff math, handoff condition evaluation
- Integration tests against real GitHub (test organization/repo with project)
- Integration tests with mock agent subprocess for adapter protocol
- Custom error types for all failure categories in Section 14.1

### 18.10 Operational Validation Before Production

- Run real integration checks with valid GitHub credentials and webhook configuration
- Verify repository sync and branch/PR behavior on the target host
- Verify hook execution and workflow path resolution on the target OS/shell environment
- Verify HTTP server bind behavior and webhook signature validation
- Run `--doctor` on the target environment and resolve all failures

## Appendix A. SSH Worker Extension (Optional)

This appendix describes a common extension profile in which Symphony keeps one central orchestrator
but executes worker runs on one or more remote hosts over SSH.

### A.1 Execution Model

- The orchestrator remains the single source of truth for polling, claims, retries, and
  reconciliation.
- `worker.ssh_hosts` provides candidate SSH destinations.
- Each worker run is assigned to one host at a time.
- `workspace.root` is interpreted on the remote host.
- The coding-agent adapter is launched over SSH stdio instead of as a local subprocess, so the
  orchestrator still owns the session lifecycle.
- Continuation turns inside one worker lifetime should stay on the same host and workspace.

### A.2 Scheduling Notes

- SSH hosts may be treated as a pool for dispatch.
- Implementations may prefer the previously used host on retries.
- A shared per-host cap may be applied.
- When all SSH hosts are at capacity, dispatch should wait rather than silently falling back.
- Transparent rerun on another host after side effects should be treated as a new attempt.

### A.3 Problems to Consider

- Remote environment drift
- Workspace locality
- Path and command safety
- Startup and failover semantics
- Host health and saturation
- Cleanup and observability

## Appendix B. Docker and Containerization

### B.1 Dockerfile

The reference Dockerfile uses multi-stage builds:

```dockerfile
# Stage 1: Build Go binary
FROM golang:1.24 AS go-builder
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /symphony ./cmd/symphony

# Stage 2: Runtime
FROM node:22-slim
RUN npm install -g tsx
WORKDIR /app
COPY --from=go-builder /symphony /usr/local/bin/symphony
COPY sidecar/ ./sidecar/
RUN cd sidecar/claude && npm install
ENTRYPOINT ["symphony"]
```

The runtime image uses `node:22-slim` as the base because the Claude Code sidecar requires
Node.js and tsx. The Go binary is statically compiled and copied in.

### B.2 Docker Compose

The reference `docker-compose.yml` provides a complete deployment with observability:

```yaml
services:
  symphony:
    build: .
    env_file: .env
    volumes:
      - ./WORKFLOW.md:/app/WORKFLOW.md:ro
      - workspaces:/app/workspaces
      - state:/app/.symphony
    ports:
      - "9097:9097"
    command: ["--port", "9097", "--state-dir", "/app/.symphony"]
    restart: unless-stopped

  victoriametrics:
    image: victoriametrics/victoria-metrics:latest
    ports:
      - "8428:8428"
    volumes:
      - vm-data:/storage
      - ./deploy/victoriametrics/scrape.yml:/etc/prometheus/scrape.yml:ro
    command:
      - -promscrape.config=/etc/prometheus/scrape.yml
      - -retentionPeriod=30d
    restart: unless-stopped

  grafana:
    image: grafana/grafana:latest
    ports:
      - "3097:3000"
    volumes:
      - ./deploy/grafana/provisioning:/etc/grafana/provisioning:ro
      - ./deploy/grafana/dashboards:/var/lib/grafana/dashboards:ro
      - grafana-data:/var/lib/grafana
    environment:
      - GF_SECURITY_ADMIN_PASSWORD=admin
      - GF_USERS_ALLOW_SIGN_UP=false
    restart: unless-stopped

volumes:
  workspaces:
  state:
  vm-data:
  grafana-data:
```

Port assignments:

- Symphony HTTP: `9097`
- VictoriaMetrics: `8428` (standard)
- Grafana: `3097` (maps container 3000 to host 3097)

### B.3 VictoriaMetrics Scrape Config

```yaml
scrape_configs:
  - job_name: symphony
    scrape_interval: 15s
    static_configs:
      - targets: ["symphony:9097"]
```

## Appendix C. Observability Stack

### C.1 Architecture

The reference observability stack uses VictoriaMetrics + Grafana:

- **VictoriaMetrics** (single-node) replaces Prometheus as the metrics store. It is a single
  binary, uses ~7x less RAM than Prometheus, supports PromQL natively, and provides built-in
  long-term retention with downsampling.
- **Grafana** consumes VictoriaMetrics as a Prometheus-compatible data source and renders
  pre-built dashboards.
- **Symphony** exposes `/metrics` in Prometheus exposition format.

Rationale:

- Symphony is a single daemon — Prometheus's service discovery and alertmanager are unnecessary
  overhead.
- VictoriaMetrics is a drop-in Prometheus replacement (accepts remote_write, supports PromQL,
  scrapes /metrics endpoints).
- The docker-compose stack stays at 3 containers: symphony, victoriametrics, grafana.

### C.2 Pre-Built Grafana Dashboard

The repository ships a pre-built Grafana dashboard JSON at
`deploy/grafana/dashboards/symphony.json`.

Dashboard panels:

1. **Active Runs** (gauge) — current running count vs `max_concurrent_agents`.
2. **Dispatch Rate** (counter/rate) — dispatches per minute over time.
3. **Work Item State Distribution** (bar chart) — counts by orchestration state (running,
   retrying, handed off, released).
4. **Token Usage** (counter/rate) — input/output/total tokens over time.
5. **GitHub API Latency** (histogram) — p50/p95/p99 request duration.
6. **GitHub API Rate Limit Remaining** (gauge) — remaining rate limit budget.
7. **Retry Queue Depth** (gauge) — current retry queue size over time.
8. **Error Rate by Category** (counter/rate) — workspace, GitHub, adapter, write-back errors.
9. **Agent Session Duration** (histogram) — distribution of session wall-clock time.
10. **PR Handoff Success Rate** (counter/rate) — successful handoffs over time.

### C.3 Grafana Provisioning

The repository ships Grafana provisioning configuration:

- `deploy/grafana/provisioning/datasources/victoriametrics.yml` — auto-configures VictoriaMetrics
  as the default Prometheus data source.
- `deploy/grafana/provisioning/dashboards/default.yml` — auto-loads dashboard JSON from
  `/var/lib/grafana/dashboards/`.

## Appendix D. CI/CD Pipeline

### D.1 GitHub Actions Workflow

The repository ships `.github/workflows/ci.yml` with the following jobs:

1. **lint** — runs `golangci-lint` on Go code.
2. **test** — runs `go test ./...` (unit tests, no external dependencies).
3. **docker-build** — validates the Docker image builds successfully.
4. **integration** — runs integration tests against real GitHub. Gated behind a `GITHUB_TOKEN`
   repository secret. Triggered on manual dispatch or when a specific label is added to a PR.

### D.2 Makefile Targets

```makefile
build              # go build -o bin/symphony ./cmd/symphony
test               # go test ./...
test-integration   # go test -tags=integration ./... (requires GITHUB_TOKEN)
lint               # golangci-lint run
sidecar            # cd sidecar/claude && npm install
docker-build       # docker build -t symphony .
docker-up          # docker-compose up -d
doctor             # bin/symphony --doctor
clean              # rm -rf bin/ sidecar/claude/node_modules
```

## Appendix E. Project Directory Layout

```
github-symphony/
├── cmd/
│   └── symphony/
│       └── main.go                    # CLI entrypoint, flag parsing, signal handling
├── internal/
│   ├── config/                        # WORKFLOW.md loader, typed config, validation, file watcher
│   ├── orchestrator/                  # poll loop, state machine, dispatch, reconciliation, retry
│   ├── github/                        # source adapter, auth providers, GraphQL, write-back, tools
│   ├── workspace/                     # repo cache, worktrees, branches, hooks, safety invariants
│   ├── adapter/                       # portable protocol types, JSON-RPC framing
│   │   ├── claude/                    # Claude Code Go-side adapter (launches sidecar)
│   │   ├── opencode/                  # OpenCode ACP proxy adapter
│   │   └── codex/                     # Codex app-server adapter
│   ├── prompt/                        # template rendering
│   ├── state/                         # bbolt persistent state store
│   ├── server/                        # HTTP API, health, metrics (chi router)
│   ├── webhook/                       # webhook ingress, signature verification
│   ├── ssh/                           # SSH worker extension (Appendix A)
│   └── logging/                       # structured log setup, slog configuration
├── sidecar/
│   └── claude/
│       ├── package.json
│       ├── tsconfig.json
│       └── src/
│           └── index.ts               # Claude Agent SDK bridge (~200-300 lines)
├── deploy/
│   ├── grafana/
│   │   ├── provisioning/
│   │   │   ├── datasources/
│   │   │   │   └── victoriametrics.yml
│   │   │   └── dashboards/
│   │   │       └── default.yml
│   │   └── dashboards/
│   │       └── symphony.json
│   └── victoriametrics/
│       └── scrape.yml
├── .github/
│   └── workflows/
│       └── ci.yml
├── Dockerfile
├── docker-compose.yml
├── Makefile
├── go.mod
├── go.sum
├── SPEC.md
├── WORKFLOW.md                        # example/default workflow file
├── .env.example
└── README.md
```
