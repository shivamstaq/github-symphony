# Symphony

Symphony is an orchestration service that polls GitHub Project V2 boards (or Linear), dispatches AI coding agents (Claude Code, OpenCode, Codex) to work on issues in isolated git workspaces, and manages the full lifecycle from dispatch through PR creation and handoff.

## Architecture

```
Tracker (GitHub/Linear)     Agent (Claude/OpenCode/Codex)     CodeHost (GitHub)
        │                            │                              │
        ▼                            ▼                              ▼
   ┌─────────────────────────────────────────────────────────────────────┐
   │                          Engine (Event Loop)                       │
   │  FSM: open → queued → preparing → running → completed → handed_off│
   │  Handlers: dispatch, retry, stall, budget, reconcile, handoff     │
   │  Event Log: .symphony/state/events.jsonl (append-only audit trail)│
   └─────────────────────────────────────────────────────────────────────┘
        │              │              │               │
   Workspace      Prompt Router    State Store     TUI Dashboard
   (git clone/    (.symphony/      (bbolt,         (Bubble Tea,
    worktree)      prompts/)        crash-safe)     3 views)
```

## Quick Start

```bash
# Build
go build -o symphony ./cmd/symphony/

# Initialize in your repo
cd /path/to/your-repo
./symphony init

# Set your GitHub token
export GITHUB_TOKEN=ghp_...

# Validate
./symphony doctor

# Run
./symphony run
```

## CLI Commands

```
symphony init                         # Interactive setup wizard
symphony run [--mock]                 # Start orchestrator with TUI
symphony doctor                       # Validate config + environment
symphony status                       # One-shot JSON state dump
symphony attach <item-id>             # Connect to agent PTY
symphony logs [--follow] [--agent X]  # Tail structured logs
symphony pause <item-id>              # Pause agent between turns
symphony resume <item-id>             # Resume paused agent
symphony kill <item-id>               # Force-stop agent
symphony events [--item X]            # Query FSM event log
symphony config validate              # Check symphony.yaml
symphony config show                  # Show resolved config
```

## Configuration

Symphony uses a `.symphony/` directory per repository:

```
.symphony/
├── symphony.yaml          # Configuration
├── prompts/               # Prompt templates (routed by project field)
│   └── default.md
├── state/
│   ├── symphony.db        # Persistent state (bbolt)
│   └── events.jsonl       # FSM audit trail
├── logs/
│   ├── orchestrator.jsonl # Structured orchestrator logs
│   └── agents/            # Per-agent session logs + PTY capture
└── sockets/               # Unix sockets for `symphony attach`
```

See [docs/configuration.md](docs/configuration.md) for the full schema reference.

## FSM States

| State | Description |
|-------|-------------|
| `open` | In tracker, not yet picked up |
| `queued` | Claimed, awaiting dispatch slot |
| `preparing` | Workspace being created |
| `running` | Agent actively working |
| `paused` | Between-turn pause |
| `completed` | Agent finished with commits |
| `handed_off` | PR created, status updated |
| `needs_human` | No progress / stall / budget exceeded |
| `failed` | Unrecoverable (max retries exhausted) |

## Supported Integrations

| Layer | Supported | Planned |
|-------|-----------|---------|
| **Tracker** | GitHub Projects V2, Linear | Jira, GitLab |
| **Agent** | Claude Code | OpenCode, Codex |
| **Code Host** | GitHub | GitLab |

## Documentation

- [Getting Started](docs/getting-started.md)
- [Configuration Reference](docs/configuration.md)
- [Architecture](docs/architecture.md)
- [Testing](docs/testing.md)
- [End-to-End Testing Guide](docs/testing-e2e.md)
