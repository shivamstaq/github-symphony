# Getting Started

## Prerequisites

- **Go 1.22+** — [install](https://go.dev/doc/install)
- **git** — available on PATH
- **Claude Code CLI** — `npm install -g @anthropic-ai/claude-code`
- **GitHub token** — with scopes: `repo`, `project`, `read:org`

## Build

```bash
git clone https://github.com/shivamstaq/github-symphony
cd github-symphony
go build -o symphony ./cmd/symphony/
```

## Prepare a GitHub Project

1. Create a GitHub Project V2 on your org or user account
2. Add a **Status** field (single select) with options: `Todo`, `In Progress`, `Human Review`, `Done`
3. Create 1-2 test issues in a repository
4. Add them to the project, set status to `Todo`

## Initialize

```bash
cd /path/to/your-repo
/path/to/symphony init
```

The wizard will ask for:
- **Tracker type**: `github` or `linear`
- **Owner**: GitHub org or username
- **Project number**: from the project URL (`/projects/42` → `42`)
- **Token**: `$GITHUB_TOKEN` if set, or paste directly
- **Agent**: `claude_code`
- **Max concurrent**: start with `2`

This creates `.symphony/` with config, prompts, and state directories.

## Configure

```bash
export GITHUB_TOKEN=ghp_your_token_here
```

Review and edit `.symphony/symphony.yaml` if needed. Key settings:
- `pull_request.handoff_status: Human Review` — where items go after PR creation
- `agent.max_turns: 20` — max agent turns per session
- `agent.budget.max_tokens_per_item: 500000` — token budget per issue

## Validate

```bash
/path/to/symphony doctor
```

All checks should show `ok`. Fix any `FAIL` items before running.

## Run

```bash
/path/to/symphony run
```

The TUI shows:
- Running agents with phase, tokens, elapsed time
- Retry queue with due times
- Dispatch/handoff/error counters

Press `l` for logs, `Enter` for agent detail, `q` to quit.

## What Happens

1. Symphony polls your GitHub Project every 30s
2. Issues with status `Todo` are dispatched to agents
3. Each agent gets an isolated git workspace (clone/worktree)
4. Agent works on the issue using Claude Code
5. On completion with commits: branch pushed, draft PR created, project status → `Human Review`
6. On failure: retried with exponential backoff (10s → 20s → 40s...)
7. On no progress: escalated to `needs_human` (no wasteful retries)

## Next Steps

- [Configuration Reference](configuration.md) — all symphony.yaml fields
- [Architecture](architecture.md) — how the FSM and engine work
- [Testing](testing.md) — run and write tests
