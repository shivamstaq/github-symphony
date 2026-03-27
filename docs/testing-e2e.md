# End-to-End Testing Guide

Complete instructions for testing Symphony on a new repository.

## Prerequisites

- Go 1.22+
- git
- Claude Code CLI: `npm install -g @anthropic-ai/claude-code`
- GitHub token with scopes: `repo`, `project`, `read:org`
- A GitHub repository you can create branches/PRs on

## Step 1: Build

```bash
git clone https://github.com/shivamstaq/github-symphony
cd github-symphony
go build -o symphony ./cmd/symphony/
```

## Step 2: Prepare a Test GitHub Project

1. Go to your GitHub org/user → **Projects** → **New Project**
2. Add a **Status** field (single select) with options:
   - `Todo`
   - `In Progress`
   - `Human Review`
   - `Done`
3. Create 2-3 test issues in a test repository (simple issues like "Add a README section")
4. Add them to the project, set status to **Todo**

## Step 3: Initialize Symphony

```bash
cd /path/to/your/test-repo
/path/to/symphony init
```

Answer the prompts:
- Tracker: `github`
- Owner: your org or username
- Project number: from the project URL
- Token: `$GITHUB_TOKEN` (if set in env)
- Agent: `claude_code`
- Max concurrent: `2`

## Step 4: Configure

```bash
export GITHUB_TOKEN=ghp_your_token_here
cat .symphony/symphony.yaml
```

Verify the config looks correct. Optionally edit:
```yaml
pull_request:
  handoff_status: Human Review    # must match a project status option
agent:
  max_turns: 10                    # lower for testing
  budget:
    max_tokens_per_item: 100000    # cap for safety
```

## Step 5: Validate

```bash
/path/to/symphony doctor
```

Expected:
```
Symphony Doctor
===============
  ok   .symphony/ directory exists
  ok   symphony.yaml found
  ok   symphony.yaml parsed successfully
  ok   config validation passed
  ok   prompts/ directory exists
  ok   state/ directory exists
  ok   default prompt template: default.md
  ok   agent binary: /usr/local/bin/claude
  ok   git available
  ok   GitHub token configured

Tracker: github | Agent: claude_code | Max concurrent: 2
```

## Step 6: Run

```bash
/path/to/symphony run
```

The TUI launches showing:
- Agent count, uptime, and metrics
- Running agents with phase, tokens, elapsed time
- Retry queue (if any)

## Step 7: Verify Dispatch

Watch the TUI:
1. After the first poll (30s), eligible issues appear as running agents
2. Each agent shows its phase: `preparing` → `running`
3. Token count increases as the agent works

## Step 8: Verify PR Creation

Check GitHub:
1. A branch named `symphony/owner_repo_N` should be pushed
2. A draft PR should be created
3. A comment on the issue links to the PR
4. The project status should move to `Human Review`

## Step 9: Test Error Paths

### Stall Detection
```yaml
# In symphony.yaml, set:
agent:
  stall_timeout_ms: 60000    # 1 minute
```
Kill a `claude` process manually while it's running. Watch the TUI — the agent should be detected as stalled and transition to `needs_human`.

### Budget Exceeded
```yaml
agent:
  budget:
    max_tokens_per_item: 50000
```
Run an issue that requires more work. When tokens exceed the limit, the agent stops and the item transitions to `needs_human`.

### Reconciliation
While an agent is running, close the issue on GitHub. On the next poll, Symphony detects the closure and cancels the agent.

## Step 10: Inspect Events

```bash
/path/to/symphony events
```

Output shows the full FSM audit trail:
```
14:32:01  org/repo#42            open -> queued          [claim]
14:32:01  org/repo#42            queued -> preparing     [dispatch]
14:32:02  org/repo#42            preparing -> running    [workspace_ready]
14:35:12  org/repo#42            running -> completed    [agent_exited_with_commits]
14:35:13  org/repo#42            completed -> handed_off [pr_created]
```

## Step 11: Test Mock Mode

```bash
/path/to/symphony run --mock
```

Uses a mock agent that simulates successful completion with commits. Useful for testing the orchestration logic without consuming API tokens.

## Step 12: Cleanup

```bash
# Remove symphony state
rm -rf .symphony/

# Clean up test branches
git push origin --delete symphony/owner_repo_1 symphony/owner_repo_2

# Close test PRs on GitHub
```

## Troubleshooting

| Issue | Solution |
|-------|----------|
| `no .symphony/symphony.yaml found` | Run `symphony init` first |
| `tracker.kind is required` | Edit symphony.yaml, add `tracker.kind: github` |
| `no credentials found` | Set `$GITHUB_TOKEN` or configure `auth.github.token` |
| `agent binary not found` | Install Claude Code: `npm i -g @anthropic-ai/claude-code` |
| TUI shows no agents | Check poll interval, verify issues are in `Todo` status |
| Agent exits with no commits | Check issue description — may be too vague for agent |
| PR not created | Check `pull_request.open_on_success: true` in config |
