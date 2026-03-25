# Symphony

Symphony is a long-running orchestration service that continuously reads work from a GitHub Project, dispatches coding agents to work on issues in isolated repository workspaces, and manages the full lifecycle from dispatch through PR creation and handoff.

## How It Works

```
GitHub Project (Todo/In Progress)
        │
        ▼
   ┌─────────┐     poll every N seconds
   │Symphony │◄────────────────────────────
   │Orchestr.│                             │
   └────┬────┘                             │
        │ dispatch eligible items          │
        ▼                                  │
   ┌─────────┐                             │
   │ Worker  │  clone repo, create branch  │
   │         │  run agent in workspace     │
   └────┬────┘                             │
        │                                  │
        ▼                                  │
   ┌─────────┐                             │
   │ claude  │  read files, edit code,     │
   │   -p    │  run tests, commit changes  │
   └────┬────┘                             │
        │                                  │
        ▼                                  │
   ┌─────────┐                             │
   │Write-   │  push branch, create PR,   │
   │back     │  update project status      │
   └────┬────┘                             │
        │                                  │
        ▼                                  │
   GitHub PR ──── Human Review ────────────┘
```

## Prerequisites

- **Go 1.26+** — for building Symphony
- **git** — for workspace operations
- **Claude CLI** — authenticated locally via `claude login`
- **GitHub PAT** — fine-grained token with repo, project, and issues permissions
- **gh CLI** — for the agent to post workpad comments on issues

## Quick Start

```bash
# 1. Clone and build
git clone https://github.com/shivamstaq/github-symphony.git
cd github-symphony
go build -o symphony ./cmd/symphony

# 2. Create .env in the directory where you'll run Symphony
echo "GITHUB_TOKEN=ghp_your_token_here" > .env

# 3. Authenticate Claude CLI (one-time)
claude login

# 4. Copy and edit the example workflow
cp WORKFLOW.md.example WORKFLOW.md
# Edit WORKFLOW.md: set your tracker.owner, tracker.project_number, tracker.project_scope

# 5. Validate your setup
./symphony --doctor WORKFLOW.md

# 6. Run Symphony
./symphony --port 9097 WORKFLOW.md
```

> **Important:** Symphony loads `.env` from the **current working directory**. Always `cd` into the directory containing your `.env` before running the binary. Flags must come **before** the positional `WORKFLOW_PATH` argument.

## Setting Up a New Repository

### 1. Create a GitHub Project V2

Go to your GitHub profile → Projects → New project, or use `gh`:

```bash
gh project create --owner YOUR_USER --title "My Project" --format json
```

Note the `number` from the output (e.g., `"number": 6`).

### 2. Configure the Status Field

Your project needs a **Status** single-select field with at least these values:

| Status | Purpose |
|--------|---------|
| **Todo** | Items ready for agent execution |
| **In Progress** | Items currently being worked on |
| **Human Review** | Items waiting for human review (handoff state) |
| **Done** | Completed items |

Symphony dispatches items in `active_values` (default: Todo, Ready, In Progress) and ignores items in `terminal_values` (default: Done, Closed, Cancelled). After an agent creates a PR, Symphony moves the item to the `handoff_project_status` value (e.g., Human Review).

### 3. Create Issues and Add to Project

```bash
# Create issues in your repository
gh issue create --repo YOUR_USER/YOUR_REPO \
  --title "Fix the flaky test" \
  --body "The auth module test fails intermittently."

gh issue create --repo YOUR_USER/YOUR_REPO \
  --title "Add input validation" \
  --body "Config parser accepts invalid values silently."

# Add them to your project
gh project item-add PROJECT_NUMBER --owner YOUR_USER \
  --url https://github.com/YOUR_USER/YOUR_REPO/issues/1

gh project item-add PROJECT_NUMBER --owner YOUR_USER \
  --url https://github.com/YOUR_USER/YOUR_REPO/issues/2
```

### 4. Set Up Dependencies (Optional)

Use GitHub's native issue dependencies to control execution order. If issue #2 depends on issue #1:

1. Open issue #2 on GitHub
2. In the sidebar, click "Add blocked by" and select issue #1

Symphony will only dispatch issue #2 after issue #1 is closed. Sub-issues are also respected — a parent issue with open sub-issues won't be dispatched until all children are closed.

### 5. Create WORKFLOW.md

Copy the example and customize:

```bash
cp WORKFLOW.md.example WORKFLOW.md
```

Edit the YAML front matter:

```yaml
---
tracker:
  kind: github
  owner: YOUR_USER            # GitHub username or org
  project_number: 6           # from step 1
  project_scope: user         # "user" or "organization"
  active_values:
    - Todo
    - In Progress
  terminal_values:
    - Done
    - Closed
    - Cancelled
github:
  token: $GITHUB_TOKEN
agent:
  kind: claude_code
  max_concurrent_agents: 3    # parallel agents
  max_turns: 5                # re-invocations per issue
  stall_timeout_ms: 600000    # kill stalled agents after 10 min
claude:
  model: sonnet               # or opus, haiku
  permission_profile: bypassPermissions
git:
  branch_prefix: symphony/
polling:
  interval_ms: 30000          # poll every 30 seconds
pull_request:
  open_pr_on_success: true
  draft_by_default: true
  handoff_project_status: Human Review
  comment_on_issue_with_pr: true
---
```

The Markdown body below the front matter is the **prompt template** sent to the agent. It uses Go template syntax (`{{.work_item.title}}`, etc.) and is rendered per issue with full context. See `WORKFLOW.md.example` for a complete playbook including workpad comments and retry handling.

### 6. Create .env

```bash
echo "GITHUB_TOKEN=ghp_your_fine_grained_pat" > .env
```

Required PAT permissions:
- **Repository**: Read and Write (for cloning, pushing branches, creating PRs)
- **Projects**: Read and Write (for fetching items, updating status fields)
- **Issues**: Read and Write (for posting comments, reading state)

### 7. Validate and Run

```bash
# Validate everything is configured correctly
./symphony --doctor WORKFLOW.md

# Run Symphony (TUI dashboard appears automatically in terminal)
./symphony --port 9097 WORKFLOW.md
```

The TUI shows running agents, retry queue, recent events, and summary stats. Press `q` to quit gracefully.

## What Happens When You Run Symphony

1. **Poll**: Fetches all items from your GitHub Project in active status values
2. **Filter**: Checks eligibility — blocked items, terminal items, already-running items are skipped
3. **Dispatch**: Sends eligible items to worker goroutines (up to `max_concurrent_agents`)
4. **Workspace**: Clones the repository, creates a git worktree with a deterministic branch (`symphony/<owner>_<repo>_<number>`)
5. **CLAUDE.md**: Generates a context file in the workspace with issue details for the agent
6. **Agent**: Invokes `claude -p --output-format json` with the rendered prompt. Claude reads files, edits code, runs tests, and commits changes.
7. **Session**: On continuation turns, uses `--resume <session_id>` so Claude has memory of prior work
8. **Detect**: Checks if the agent created any git commits
9. **Write-back**: If commits exist — pushes the branch, creates/updates a draft PR, comments on the issue, moves the project status to "Human Review"
10. **Handoff**: Marks the item as handed off. Symphony stops dispatching it.
11. **Repeat**: Polls again, picks up newly eligible items

## Configuration Reference

All configuration lives in `WORKFLOW.md` YAML front matter:

| Key | Default | Description |
|-----|---------|-------------|
| `tracker.kind` | required | `github` |
| `tracker.owner` | required | GitHub user or org |
| `tracker.project_number` | required | Project V2 number |
| `tracker.project_scope` | `organization` | `user` or `organization` |
| `tracker.active_values` | `[Todo, Ready, In Progress]` | Project status values eligible for dispatch |
| `tracker.terminal_values` | `[Done, Closed, Cancelled]` | Terminal status values (stop execution) |
| `github.token` | `$GITHUB_TOKEN` | PAT or `$VAR` env reference |
| `agent.kind` | required | `claude_code`, `opencode`, or `codex` |
| `agent.max_concurrent_agents` | `10` | Maximum parallel workers |
| `agent.max_turns` | `20` | Re-invocations per work item before giving up |
| `agent.stall_timeout_ms` | `300000` | Kill stalled workers after this duration |
| `claude.model` | — | Model override (`sonnet`, `opus`, `haiku`) |
| `claude.permission_profile` | `bypassPermissions` | Claude CLI permission mode |
| `claude.allowed_tools` | all | Restrict agent tools (e.g., `[Read, Edit, Bash]`) |
| `git.branch_prefix` | `symphony/` | Branch name prefix |
| `git.use_worktrees` | `true` | Use git worktrees (recommended) |
| `polling.interval_ms` | `30000` | Poll interval in milliseconds |
| `pull_request.open_pr_on_success` | `true` | Create PR after agent commits |
| `pull_request.draft_by_default` | `true` | Create draft PRs |
| `pull_request.handoff_project_status` | — | Status value for handoff (e.g., `Human Review`) |
| `pull_request.comment_on_issue_with_pr` | `true` | Post PR link as issue comment |
| `server.port` | — | HTTP server port (disabled if unset) |

## CLI Reference

```
symphony [flags] [WORKFLOW_PATH]

Arguments:
  WORKFLOW_PATH       Path to WORKFLOW.md (default: ./WORKFLOW.md)

Flags:
  --port PORT         Start HTTP server on PORT
  --log-level LVL     Log level: debug, info, warn, error (default: info)
  --log-format FMT    Log format: text, json (default: text)
  --state-dir PATH    Persistent state directory
  --doctor            Validate config and environment, then exit
  --no-tui            Disable TUI dashboard, use plain log output
```

**Flags must come before the positional WORKFLOW_PATH argument** (Go `flag` package limitation).

### Examples

```bash
# Validate setup (checks config, GitHub connectivity, claude binary)
./symphony --doctor WORKFLOW.md

# Run with TUI dashboard + HTTP API
./symphony --port 9097 WORKFLOW.md

# Run without TUI (for CI/Docker/piped output)
./symphony --no-tui --log-format json --port 9097 WORKFLOW.md

# Debug mode (verbose logging)
./symphony --port 9097 --log-level debug WORKFLOW.md
```

## TUI Dashboard

When running in a terminal, Symphony displays a live Bubble Tea dashboard:

```
🎵 Symphony                              Uptime: 00:14:32
Agents: 2/5 running  │  Dispatched: 7  │  Handed Off: 3
──────────────────────────────────────────────────────────
RUNNING AGENTS
  Issue                Phase          Time     Tokens
  ──────────────────────────────────────────────────────
  org/repo#4           streaming_turn 3m12s    12.4k
  org/repo#7           launching      0m48s    3.1k

RETRY QUEUE
  org/repo#1 → due in 8s (attempt 2)

RECENT EVENTS
  09:14:32  org/repo#4   PR created → pull/12
  09:14:01  org/repo#7   Workspace created (worktree)
  09:13:12  org/repo#1   Blocked by #4 (state: open)
──────────────────────────────────────────────────────────
[q] Quit  [r] Refresh
```

Disable with `--no-tui` for plain log output.

## HTTP API

When `--port` is set, Symphony exposes:

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/healthz` | GET | Health check with uptime, auth mode, running count, last poll time |
| `/metrics` | GET | Prometheus-format metrics (12 `symphony_*` metrics) |
| `/api/v1/state` | GET | Full orchestrator runtime snapshot (JSON) |
| `/api/v1/work-items/{id}` | GET | Single work item details |
| `/api/v1/refresh` | POST | Trigger immediate reconciliation |
| `/api/v1/webhooks/github` | POST | GitHub webhook ingress (requires `github.webhook_secret`) |

## Authentication

### GitHub (Symphony itself)

Symphony uses a **fine-grained Personal Access Token** for all GitHub API operations. Set `GITHUB_TOKEN` in your `.env` file. No GitHub App registration required.

### Agent Adapters

Symphony does **not** manage agent API keys. Each agent subprocess inherits the full parent environment and uses its own local credentials:

| Adapter | Auth Method | Setup |
|---------|-------------|-------|
| **Claude Code** | Local OAuth via `~/.claude` | Run `claude login` once |
| **OpenCode** | Local config | Configure `opencode` |
| **Codex** | Local config | Configure `codex` |

No `ANTHROPIC_API_KEY` is required — the `claude` CLI handles auth via its own OAuth flow.

## Session Preservation

Symphony preserves Claude session IDs across continuation turns. When an issue is re-invoked:

1. The previous `session_id` is loaded from the persistent store (bbolt)
2. Claude is invoked with `--resume <session_id>`
3. Claude has full memory of prior conversation, tool results, and file changes

Additionally, a `CLAUDE.md` file is generated in each workspace with issue context. Claude reads this automatically on every invocation, providing persistent instructions even without session resumption.

## Dependency Handling

Symphony respects two types of GitHub issue relationships:

### Blocking Dependencies
If issue A is blocked by issue B (via GitHub's "Blocked by" feature), Symphony will **not dispatch A** until B is closed. All blockers must be closed for an item to be eligible.

### Parent/Sub-Issues
If a parent issue has open sub-issues, Symphony skips the parent and dispatches eligible sub-issues instead. When all sub-issues are closed, the parent becomes eligible with enriched context about the completed child work.

## Observability

### Prometheus Metrics (`/metrics`)

12 metrics exposed in Prometheus text format:

| Metric | Type | Description |
|--------|------|-------------|
| `symphony_active_runs` | gauge | Current running workers |
| `symphony_max_concurrent_agents` | gauge | Configured concurrency limit |
| `symphony_retry_queue_depth` | gauge | Pending retries |
| `symphony_tokens_total{direction}` | counter | Token usage (input/output/total) |
| `symphony_sessions_started_total` | counter | Total agent sessions |
| `symphony_github_writebacks_total` | counter | PR/comment operations |
| `symphony_dispatches_total` | counter | Total dispatches |
| `symphony_work_item_state{state}` | gauge | Items by state |
| `symphony_errors_total` | counter | Error count |
| `symphony_pr_handoffs_total` | counter | Successful handoffs |

### Docker Observability Stack

```bash
docker compose up -d
```

Starts 5 services:
- **Symphony** (port 9097) — orchestrator with HTTP API
- **VictoriaMetrics** (port 8428) — metrics storage (Prometheus-compatible)
- **VictoriaLogs** (port 9428) — searchable log storage
- **Vector** — log collector (reads Symphony JSON logs, pushes to VictoriaLogs)
- **Grafana** (port 3097, admin/admin) — dashboards for metrics + logs

Query logs in Grafana using LogsQL:
```
work_item_id:"github:PVTI_xxx"     # All logs for a work item
log.level:error                      # All errors
msg:"claude CLI"                     # Agent activity
```

## Architecture

```
cmd/symphony/main.go           CLI entrypoint, wiring, signal handling, TUI launch
internal/
  config/                       WORKFLOW.md loader, typed config, validation, file watcher
  orchestrator/                 Poll loop, dispatch, eligibility, retry, reconciliation
    worker.go                   Multi-turn agent execution loop + session preservation
    events.go                   Event bus for TUI and logging
    source_bridge.go            GitHub → orchestrator type bridge
  github/                       GraphQL queries, PR/comment write-back, auth, tools
  adapter/                      Claude CLI adapter (exec + JSON parse)
  workspace/                    Git clone/worktree, branches, hooks, push
  prompt/                       Go template rendering with missingkey=error
  state/                        bbolt persistent state (retries, sessions, totals)
  server/                       HTTP API, Prometheus metrics, health check
  webhook/                      GitHub webhook signature verification
  tui/                          Bubble Tea terminal dashboard
  tracker/                      Abstract interfaces for multi-backend support
  logging/                      Structured log setup
  ssh/                          SSH worker extension (stub)
```

## Testing

```bash
# Unit tests (no external dependencies)
go test ./... -count=1

# Integration tests (requires GITHUB_TOKEN — loads from .env two levels up)
cd test/integration && go test -tags=integration -v -count=1

# Lint
golangci-lint run
```

## Docker

```bash
# Build image (Go binary + git, no Node.js needed)
docker build -t symphony .

# Run full stack with observability
docker compose up -d

# View logs
docker compose logs -f symphony

# Stop
docker compose down
```

## Guardrails

Symphony includes multiple safety mechanisms:

- **Max continuation retries** (default 10): Prevents infinite re-dispatch loops
- **Continuation backoff** (5s → 10s → 20s → 30s): Avoids rapid-fire re-invocations
- **Stall detection**: Kills workers silent for longer than `stall_timeout_ms`
- **Incomplete data rejection**: Items with failed dependency fetches are skipped
- **Token sanitization**: Git auth tokens are masked in all log output
- **Clone mutex**: Prevents concurrent bare repo clones to the same cache path
- **Context cancellation**: Workers stop promptly on SIGTERM; Claude processes are killed
- **Handoff on PR creation**: PR creation unconditionally triggers handoff (prevents re-dispatch loops)
- **Eligibility checks**: 10+ rules checked before dispatch (active status, open state, not blocked, not claimed, slots available, per-repo limits, per-status limits, sub-issue check, Pass2 data completeness)

## License

See [LICENSE](LICENSE).
