# Configuration Reference

Symphony is configured via `.symphony/symphony.yaml`. Run `symphony init` to generate a template.

## `tracker`

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `kind` | string | *required* | `"github"` or `"linear"` |
| `owner` | string | *required* | GitHub org/user login |
| `project_number` | int | *required* | GitHub Project V2 number |
| `project_scope` | string | `"organization"` | `"organization"` or `"user"` |
| `status_field_name` | string | `"Status"` | Project field for dispatch decisions |
| `active_values` | []string | `[Todo, Ready, In Progress]` | Statuses eligible for dispatch |
| `terminal_values` | []string | `[Done, Closed, Cancelled, ...]` | Statuses that stop work |
| `blocked_values` | []string | `[]` | Statuses treated as blocked |
| `priority_field_name` | string | `"Priority"` | Field for priority sorting |
| `priority_value_map` | map | `{}` | Map field values to numeric priority (`{P0: 0, P1: 1}`) |
| `executable_item_types` | []string | `["issue"]` | Item types to dispatch |
| `require_issue_backing` | bool | `true` | Require real issue (not draft) |
| `repo_allowlist` | []string | `[]` | Only dispatch from these repos |
| `repo_denylist` | []string | `[]` | Never dispatch from these repos |
| `required_labels` | []string | `[]` | All must be present on issue |

## `auth`

### `auth.github`

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `mode` | string | `"auto"` | `"pat"`, `"app"`, or `"auto"` |
| `token` | string | | PAT or `$GITHUB_TOKEN` |
| `api_url` | string | `"https://api.github.com"` | GitHub API base URL |
| `app_id` | string | | GitHub App ID |
| `private_key` | string | | GitHub App private key |
| `installation_id` | string | | GitHub App installation ID |
| `webhook_secret` | string | | Webhook signature verification |

### `auth.linear`

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `api_key` | string | | Linear API key or `$LINEAR_API_KEY` |

## `agent`

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `kind` | string | `"claude_code"` | `"claude_code"`, `"opencode"`, or `"codex"` |
| `command` | string | auto-detect | Override binary path |
| `max_concurrent` | int | `10` | Global agent concurrency limit |
| `max_turns` | int | `20` | Max turns per agent session |
| `stall_timeout_ms` | int | `300000` | No activity → kill (0 to disable) |
| `max_retry_backoff_ms` | int | `300000` | Max retry delay cap |
| `max_continuation_retries` | int | `10` | Max retries before `failed` |
| `session_reuse` | bool | `true` | Resume previous session on retry |
| `max_concurrent_by_status` | map | `{}` | Per-status concurrency limits |
| `max_concurrent_by_repo` | map | `{}` | Per-repo concurrency limits |

### `agent.budget`

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `max_cost_per_item_usd` | float | `0` | Per-item cost cap (0 = no limit) |
| `max_cost_total_usd` | float | `0` | Global cost cap |
| `max_tokens_per_item` | int | `0` | Per-item token cap |

### `agent.claude`

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `model` | string | | Model override (e.g., `"sonnet"`, `"opus"`) |
| `permission_profile` | string | | e.g., `"bypassPermissions"` |
| `allowed_tools` | []string | `[]` | Restrict tools (empty = all) |

## `git`

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `branch_prefix` | string | `"symphony/"` | Work branch prefix |
| `fetch_depth` | int | `0` | Clone depth (0 = full) |
| `use_worktrees` | bool | `true` | Use git worktrees (faster) |
| `push_remote` | string | `"origin"` | Remote to push branches |
| `author_name` | string | `"Symphony"` | Git commit author |
| `author_email` | string | `"symphony@noreply.github.com"` | Git commit email |

## `polling`

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `interval_ms` | int | `30000` | Poll interval in milliseconds |

## `pull_request`

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `open_on_success` | bool | `true` | Create PR when agent has commits |
| `draft_by_default` | bool | `true` | Create PRs as draft |
| `reuse_existing` | bool | `true` | Update existing PR instead of creating new |
| `handoff_status` | string | | Project status to set on PR creation |
| `comment_on_issue` | bool | `true` | Comment on issue with PR link |
| `required_checks` | []string | `[]` | CI checks required before handoff |

## `hooks`

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `after_create` | string | | Script to run after workspace created |
| `before_run` | string | | Script to run before agent starts |
| `after_run` | string | | Script to run after agent exits |
| `before_remove` | string | | Script to run before workspace removed |
| `timeout_ms` | int | `60000` | Hook timeout |

## `prompt_routing`

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `field_name` | string | | GitHub Project custom field to route on |
| `routes` | map | `{}` | Field value → template file mapping |
| `default` | string | `"default.md"` | Fallback template |

## `server`

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `port` | int | `9097` | HTTP API port (0 to disable) |
| `host` | string | `"0.0.0.0"` | HTTP API bind address |

## Environment Variables

Any string value can reference an environment variable using `$VAR` syntax:

```yaml
auth:
  github:
    token: $GITHUB_TOKEN
```

The variable is resolved at config load time.
