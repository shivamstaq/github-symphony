---
tracker:
  kind: github
  owner: shivamstaq
  project_number: 6
  project_scope: user
github:
  token: $GITHUB_TOKEN
agent:
  kind: claude_code
  max_concurrent_agents: 2
  max_turns: 1
polling:
  interval_ms: 5000
pull_request:
  open_pr_on_success: false
  draft_by_default: true
---
You are working on {{.work_item.issue_identifier}}: {{.work_item.title}}

Repository: {{.repository.full_name}}
Branch: {{.branch_name}}
