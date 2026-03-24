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
   │back     │  comment on issue           │
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
- **gh CLI** (optional) — for the agent to post issue comments

## Quick Start

```bash
# 1. Clone and build
git clone https://github.com/shivamstaq/github-symphony.git
cd github-symphony
go build -o symphony ./cmd/symphony

# 2. Create .env with your GitHub token
echo "GITHUB_TOKEN=ghp_your_token_here" > .env

# 3. Authenticate Claude CLI (one-time)
claude login

# 4. Copy and edit the example workflow
cp WORKFLOW.md.example WORKFLOW.md
# Edit WORKFLOW.md: set your owner, project_number, project_scope

# 5. Validate your setup
./symphony --doctor WORKFLOW.md

# 6. Run Symphony
./symphony --port 9097 --log-level info WORKFLOW.md
```

## Setting Up a New Repository

### 1. Create a GitHub Project V2

Go to your GitHub profile → Projects → New project, or use `gh`:

```bash
gh project create --owner YOUR_USER --title "My Project" --format json
```

Note the `number` from the output.

### 2. Configure the Status Field

Your project needs a **Status** field with at least these values:
- **Todo** — items ready for agent execution
- **In Progress** — items currently being worked on
- **Human Review** — items waiting for human review (handoff state)
- **Done** — completed items

### 3. Create Issues and Add to Project

```bash
# Create an issue
gh issue create --repo YOUR_USER/YOUR_REPO \
  --title "Fix the flaky test" \
  --body "The auth module test fails intermittently."

# Add it to your project
gh project item-add PROJECT_NUMBER --owner YOUR_USER \
  --url https://github.com/YOUR_USER/YOUR_REPO/issues/1
```

### 4. Create WORKFLOW.md

Copy `WORKFLOW.md.example` and edit:

```yaml
tracker:
  owner: YOUR_USER          # GitHub username or org
  project_number: 6         # from step 1
  project_scope: user       # or "organization"
```

### 5. Create .env

```bash
echo "GITHUB_TOKEN=ghp_your_fine_grained_pat" > .env
```

Required PAT permissions: **Repository** (read/write), **Projects** (read/write), **Issues** (read/write).

### 6. Run

```bash
./symphony --port 9097 WORKFLOW.md
```

## Configuration Reference

All configuration lives in `WORKFLOW.md` YAML front matter:

| Key | Default | Description |
|-----|---------|-------------|
| `tracker.kind` | required | `github` |
| `tracker.owner` | required | GitHub user or org |
| `tracker.project_number` | required | Project V2 number |
| `tracker.project_scope` | `organization` | `user` or `organization` |
| `tracker.active_values` | `[Todo, Ready, In Progress]` | Project status values for dispatch |
| `tracker.terminal_values` | `[Done, Closed, Cancelled]` | Terminal status values |
| `github.token` | `$GITHUB_TOKEN` | PAT or env var reference |
| `agent.kind` | required | `claude_code`, `opencode`, or `codex` |
| `agent.max_concurrent_agents` | `10` | Maximum parallel workers |
| `agent.max_turns` | `20` | Re-invocations per work item |
| `agent.stall_timeout_ms` | `300000` | Kill stalled workers after 5 min |
| `claude.model` | — | Model override (`sonnet`, `opus`) |
| `claude.permission_profile` | `bypassPermissions` | Claude CLI permission mode |
| `claude.allowed_tools` | all | Restrict agent tools |
| `git.branch_prefix` | `symphony/` | Branch name prefix |
| `polling.interval_ms` | `30000` | Poll interval in milliseconds |
| `pull_request.open_pr_on_success` | `true` | Create PR after agent work |
| `pull_request.draft_by_default` | `true` | Create draft PRs |
| `pull_request.handoff_project_status` | — | Status value for handoff (e.g., `Human Review`) |
| `server.port` | — | HTTP server port (disabled if unset) |

## CLI Reference

```
symphony [WORKFLOW_PATH] [flags]

Arguments:
  WORKFLOW_PATH     Path to WORKFLOW.md (default: ./WORKFLOW.md)

Flags:
  --port PORT       Start HTTP server on PORT
  --log-level LVL   Log level: debug, info, warn, error (default: info)
  --log-format FMT  Log format: text, json (default: text)
  --state-dir PATH  Persistent state directory
  --doctor          Validate config and environment, then exit
```

**Note:** Flags must come BEFORE the positional WORKFLOW_PATH argument.

## HTTP API

When `--port` is set, Symphony exposes:

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/healthz` | GET | Health check with uptime and status |
| `/metrics` | GET | Prometheus-format metrics |
| `/api/v1/state` | GET | Full orchestrator runtime snapshot |
| `/api/v1/work-items/{id}` | GET | Single work item details |
| `/api/v1/refresh` | POST | Trigger immediate reconciliation |
| `/api/v1/webhooks/github` | POST | GitHub webhook ingress |

## Agent Workflow

When Symphony dispatches an issue to the Claude CLI adapter:

1. **Workspace created** — repo cloned, deterministic branch checked out
2. **Prompt rendered** — WORKFLOW.md template filled with issue context
3. **Claude invoked** — `claude -p --output-format json --permission-mode bypassPermissions`
4. **Agent works** — reads files, edits code, runs tests, commits changes
5. **Result parsed** — JSON output with stop_reason, usage, cost
6. **Write-back** — if commits exist: push branch, create/update PR, comment on issue
7. **Continuation** — if still active: schedule re-invocation; if handed off: release

The agent sees the full issue description, branch name, attempt number, and the prompt template from WORKFLOW.md. It can use `gh` CLI (from Bash tool) to post workpad comments on the issue for progress tracking.

## Authentication

### GitHub (Symphony itself)

Symphony uses a **fine-grained Personal Access Token** for all GitHub API operations (fetching projects, creating PRs, posting comments). Set `GITHUB_TOKEN` in your `.env` file. No GitHub App registration required.

### Agent Adapters (Claude, OpenCode, Codex)

Symphony does **not** manage agent API keys. Each adapter subprocess inherits the parent process's full environment and uses its own locally-configured credentials:

- **Claude Code**: Uses the `claude` CLI which authenticates via `claude login` (OAuth, stored in `~/.claude`). No `ANTHROPIC_API_KEY` required unless you specifically set one.
- **OpenCode**: Uses `opencode acp` with its own local config.
- **Codex**: Uses `codex app-server` with its own local auth.

## Adapters

### Claude Code (Primary)

Uses the `claude` CLI directly:
```
claude -p --output-format json --permission-mode bypassPermissions --model sonnet
```

- Requires: `claude` on PATH, authenticated via `claude login`
- No API key management — uses local OAuth credentials
- Full tool access (Read, Edit, Write, Bash, Glob, Grep)

### OpenCode

Uses `opencode acp` over JSON-RPC stdio. Requires OpenCode installed and configured.

### Codex

Uses `codex app-server` over JSON-RPC stdio. Requires Codex installed and configured.

## Architecture

```
cmd/symphony/main.go           CLI entrypoint, wiring, signal handling
internal/
  config/                       WORKFLOW.md loader, typed config, validation, file watcher
  orchestrator/                 Poll loop, dispatch, eligibility, retry, reconciliation
    worker.go                   Multi-turn agent execution loop
    source_bridge.go            GitHub → orchestrator type bridge
  github/                       GraphQL queries, PR/comment write-back, auth, tools
  adapter/                      Agent adapter protocol + Claude CLI adapter
  workspace/                    Git clone/worktree, branches, hooks, push
  prompt/                       Template rendering
  state/                        bbolt persistent state
  server/                       HTTP API, Prometheus metrics, health check
  webhook/                      GitHub webhook signature verification
  logging/                      Structured log setup
  ssh/                          SSH worker extension (stub)
```

## Testing

```bash
# Unit tests (no external dependencies)
go test ./... -count=1

# Integration tests (requires GITHUB_TOKEN in .env)
cd test/integration && go test -tags=integration -v -count=1

# Lint
golangci-lint run
```

## Docker

```bash
# Build image
docker build -t symphony .

# Run with docker-compose (includes VictoriaMetrics + Grafana)
docker compose up -d

# Grafana dashboard at http://localhost:3097 (admin/admin)
# VictoriaMetrics at http://localhost:8428
# Symphony API at http://localhost:9097
```

## Observability

Symphony exposes 12 Prometheus metrics at `/metrics`:

- `symphony_active_runs` — current running workers
- `symphony_max_concurrent_agents` — configured limit
- `symphony_retry_queue_depth` — pending retries
- `symphony_tokens_total{direction}` — token usage (input/output/total)
- `symphony_sessions_started_total` — total agent sessions
- `symphony_github_writebacks_total` — PR/comment operations
- `symphony_dispatches_total` — total dispatches
- `symphony_work_item_state{state}` — work items by state
- `symphony_errors_total` — error count
- `symphony_pr_handoffs_total` — successful handoffs

## License

See [LICENSE](LICENSE).
