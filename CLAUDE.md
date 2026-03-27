# CLAUDE.md — Symphony Project Conventions

## Package Structure

```
internal/
  domain/         # Shared types (WorkItem, FSM) — zero external deps
  engine/         # Central event loop, handlers, state — the orchestrator core
  tracker/KIND/   # Tracker adapters (github/, linear/, mock/)
  codehost/KIND/  # Code host adapters (github/)
  agent/KIND/     # Agent adapters (claude/, mock/)
  config/         # symphony.yaml parser + validator
  prompt/         # Template rendering + field-based routing
  workspace/      # Git clone/worktree management
  state/          # bbolt persistent store
  logging/        # JSONL file logger
  server/         # HTTP API (healthz, metrics, control endpoints)
  tui/views/      # Bubble Tea TUI with 3 view modes
```

## Key Conventions

- **Interfaces in root packages, implementations in sub-packages**: `tracker/tracker.go` defines the interface, `tracker/github/source.go` implements it.
- **`domain.WorkItem` is canonical**: All packages import `domain`, never define their own work item type.
- **No mutexes in engine**: The event loop goroutine owns all mutable state. External callers use `Emit()` to send events.
- **FSM enforcement**: All state changes go through `domain.Transition()`. Invalid transitions are logged errors.
- **Event log is append-only**: `events.jsonl` records every FSM transition for debugging and replay.

## Testing Patterns

- **Property tests** (`test/property/`): Random event sequences verify FSM invariants (55k sequences).
- **Scenario tests** (`test/scenario/`): Named end-to-end paths through the engine with mock adapters.
- **Contract tests** (`internal/tracker/contract_test.go`): Shared behavioral tests for all tracker implementations.
- **Unit tests**: Colocated `*_test.go` files for eligibility, handoff, reconcile, retry, config, prompt routing.
- **Mock adapters**: `agent/mock/`, `tracker/mock/` — configurable behavior for testing.

## Build & Test

```bash
go build ./...                    # Build everything
go test ./... -count=1            # Run all tests
go test ./test/property/ -v       # FSM property tests
go test ./test/scenario/ -v       # Scenario tests
```

## Error Handling

- Wrap errors with `fmt.Errorf("context: %w", err)` for stack traces.
- GitHub API errors use typed error types in `internal/github/errors.go`.
- Agent failures trigger FSM transitions (`error` event), not panics.
- Budget/stall violations transition to `needs_human`, not retry.

## Config

- Config lives in `.symphony/symphony.yaml` (per-repo).
- Environment variables resolved via `$VAR` syntax at load time.
- Required fields validated by `ValidateSymphonyConfig()`.
- `symphony doctor` checks config, credentials, and binary availability.
