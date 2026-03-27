# Testing

## Run All Tests

```bash
go test ./... -count=1
```

## Test Categories

### FSM Property Tests (`test/property/`)

```bash
go test ./test/property/ -v
```

Generates 55,000 random event sequences and verifies invariants:
- No panics on any sequence
- Items always in exactly one valid state
- `needs_human` only reachable via expected events
- `handed_off` only reachable via `pr_created`
- Terminal states only exit via specific events
- Event log replay produces identical final state

### Scenario Tests (`test/scenario/`)

```bash
go test ./test/scenario/ -v
```

7 named scenarios testing full engine cycles:

| Scenario | What It Tests |
|----------|---------------|
| HappyPath | dispatch → agent completes → PR → handed_off |
| RetryExhaustion | agent fails → retry queued with backoff |
| NoCommitsEscalation | agent exits without commits → needs_human (NOT retry) |
| StallRecovery | no activity beyond timeout → needs_human, worker killed |
| ReconcileClosed | issue closed externally → agent cancelled |
| BudgetExceeded | tokens over limit → needs_human |
| HandedOffNotRedispatched | second poll does NOT re-dispatch handed-off item |

### Tracker Contract Tests (`internal/tracker/`)

```bash
go test ./internal/tracker/ -v
```

Shared behavioral tests that any tracker implementation must satisfy:
- FetchCandidates returns items with WorkItemID and Title
- FetchStates returns matching items, empty for unknown IDs
- ValidateConfig returns no error for valid input

### Unit Tests

Colocated in each package:
- `internal/domain/fsm_test.go` — 22 valid transitions, 12 invalid, invariants
- `internal/config/symphony_config_test.go` — parsing, defaults, env vars, validation
- `internal/engine/*_test.go` — eligibility, retry backoff, stall, budget, reconcile, handoff
- `internal/prompt/router_test.go` — field-based routing, case insensitivity
- `internal/tracker/linear/normalize_test.go` — Linear → domain.WorkItem conversion

## Writing New Tests

### Add a Scenario Test

```go
// test/scenario/my_test.go
func TestScenario_MyCase(t *testing.T) {
    item := makeItem("100", 100)
    h := NewHarness(t, HarnessConfig{
        Items: []domain.WorkItem{item},
        Agent: agentmock.NewSuccessAgent(), // or NewFailAgent, NewNoCommitsAgent
    })
    defer h.Cleanup()

    h.PollOnce()
    h.WaitForState("100", domain.StateHandedOff, 3*time.Second)
    h.AssertState("100", domain.StateHandedOff)
}
```

### Add a FSM Transition

1. Add the transition to `transitionTable` in `internal/domain/fsm.go`
2. Add a test case in `TestTransition_ValidTransitions`
3. Run `go test ./test/property/` — property tests will validate invariants
4. Update `AllEvents` if you added a new event type

### Add a Tracker Implementation

1. Create `internal/tracker/KIND/source.go` implementing `tracker.Tracker`
2. Create `internal/tracker/KIND/register.go` with `init()` calling `tracker.Register()`
3. Add contract test: run `contractSuite(t, "kind", yourTracker)` in `contract_test.go`
4. Add blank import in `cmd/symphony/run.go`: `_ "internal/tracker/KIND"`
