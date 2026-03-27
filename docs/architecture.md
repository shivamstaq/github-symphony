# Architecture

## Overview

Symphony has three adapter layers connected by a central engine:

```
┌──────────┐     ┌──────────┐     ┌──────────┐
│ Tracker  │     │  Agent   │     │ CodeHost │
│(GitHub/  │     │(Claude/  │     │(GitHub)  │
│ Linear)  │     │ Mock)    │     │          │
└────┬─────┘     └────┬─────┘     └────┬─────┘
     │                │                │
     ▼                ▼                ▼
┌─────────────────────────────────────────────┐
│              Engine (Event Loop)            │
│  Single goroutine owns all mutable state    │
│  No mutexes — events processed sequentially │
└─────────────────────────────────────────────┘
     │          │           │           │
     ▼          ▼           ▼           ▼
   FSM      Event Log   State Store   TUI
 (domain)  (events.jsonl) (bbolt)   (Bubble Tea)
```

## FSM (Finite State Machine)

Every work item has exactly one state. Transitions are enforced by a declarative table in `internal/domain/fsm.go`:

```
open ──claim──> queued ──dispatch──> preparing ──workspace_ready──> running
                  ▲                                                    │
                  │ error                              agent_exited_   │
                  │ (has retries)                      with_commits    │
                  │                                         │          │
                  │                                         ▼          │
                  │                                    completed       │
                  │                                         │          │
                  │                                    pr_created      │
                  │                                         │          │
                  │                                         ▼          │
                  │                                    handed_off      │
                  │                                                    │
                  │                    agent_exited_no_commits /        │
                  │                    stall_detected /                 │
                  │                    budget_exceeded                  │
                  │                                         │          │
                  │                                         ▼          │
                  └──retry_manual── needs_human <──────────┘
```

Guard functions control conditional transitions (e.g., `running → queued` requires `has_retries_left`).

## Engine Event Loop

The engine processes events sequentially through a single goroutine:

```go
for event := range eventCh {
    switch event.Type {
    case EvtPollTick:     handlePollTick(ctx)      // fetch + dispatch + stall + reconcile
    case EvtAgentExited:  handleAgentExited(event)  // FSM transition + handoff or retry
    case EvtAgentUpdate:  handleAgentUpdate(event)  // token tracking + budget check
    case EvtPauseRequested: handlePause(event)      // set pause flag
    case EvtRetryDue:     handleRetryDue(event)     // re-dispatch from retry queue
    // ... 21 event types total
    }
}
```

No mutexes are needed because only this goroutine reads/writes the state.

## Adapter Pattern

### Tracker (`internal/tracker/tracker.go`)
- `FetchCandidates(ctx)` — poll for dispatchable items
- `FetchStates(ctx, ids)` — refresh running items
- `ValidateConfig(ctx, input)` — check configured fields exist
- Implementations: `tracker/github/`, `tracker/linear/`, `tracker/mock/`

### Agent (`internal/agent/agent.go`)
- `Start(ctx, config) → *Session` — launch agent process
- Session provides `Updates` channel (progress) and `Done` channel (result)
- Implementations: `agent/claude/`, `agent/mock/`

### CodeHost (`internal/codehost/codehost.go`)
- `UpsertPR(ctx, params)` — create or update pull request
- `UpdateProjectStatus(ctx, params)` — move project item status
- `CommentOnItem(ctx, ref, body)` — post comment
- Implementation: `codehost/github/`

## Event Log

Every FSM transition is appended to `.symphony/state/events.jsonl`:

```json
{"timestamp":"2026-03-27T14:32:01Z","item_id":"42","from":"open","to":"queued","event":"claim"}
{"timestamp":"2026-03-27T14:32:01Z","item_id":"42","from":"queued","to":"preparing","event":"dispatch"}
```

Query with `symphony events --item 42`.

## State Persistence

bbolt store at `.symphony/state/symphony.db` persists:
- **Handoffs** — prevents re-dispatch across restarts
- **Retries** — preserves retry queue with attempt counts
- **Totals** — lifetime token/cost/session counters

Restored on startup, persisted on shutdown and on each handoff.

## Eligibility Rules

Before dispatch, each item passes 13 eligibility checks:
1. Has project item ID and title
2. Dependency data complete (not Pass2Failed)
3. Content type executable (issue, not PR)
4. Project status in active values
5. Not in terminal or blocked status
6. Issue backing required (not draft)
7. Issue state is open
8. Repo in allowlist / not in denylist
9. Required labels present
10. Not already claimed/running/handed off
11. Global concurrency slots available
12. Per-status concurrency limit OK
13. Per-repo concurrency limit OK
14. No open blocking dependencies
15. No open sub-issues (dispatch children instead)
