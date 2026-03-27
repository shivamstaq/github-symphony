package linear

import (
	"github.com/shivamstaq/github-symphony/internal/domain"
)

// Normalize converts a Linear IssueNode to the canonical domain.WorkItem.
func Normalize(issue IssueNode) domain.WorkItem {
	labels := make([]string, len(issue.Labels.Nodes))
	for i, l := range issue.Labels.Nodes {
		labels[i] = l.Name
	}

	var assignees []string
	if issue.Assignee != nil {
		assignees = []string{issue.Assignee.Name}
	}

	// Map Linear priority (1=urgent..4=low) to our model
	var priority *int
	if issue.Priority > 0 {
		p := issue.Priority
		priority = &p
	}

	// Map Linear state type to open/closed
	issueState := "open"
	if issue.State.Type == "completed" || issue.State.Type == "cancelled" {
		issueState = "closed"
	}

	// Build blockers from relations
	var blockedBy []domain.BlockerRef
	for _, rel := range issue.Relations.Nodes {
		if rel.Type == "blocked_by" {
			blockedBy = append(blockedBy, domain.BlockerRef{
				ID:         rel.RelatedIssue.ID,
				Identifier: rel.RelatedIssue.Identifier,
				State:      rel.RelatedIssue.State.Name,
			})
		}
	}

	item := domain.WorkItem{
		WorkItemID:      "linear:" + issue.ID,
		ProjectItemID:   issue.ID,
		ContentType:     "issue",
		IssueID:         issue.ID,
		IssueIdentifier: issue.Identifier,
		Title:           issue.Title,
		Description:     issue.Description,
		State:           issueState,
		ProjectStatus:   issue.State.Name,
		Priority:        priority,
		Labels:          labels,
		Assignees:       assignees,
		BlockedBy:       blockedBy,
		URL:             issue.URL,
		CreatedAt:       issue.CreatedAt,
		UpdatedAt:       issue.UpdatedAt,
		ProjectFields: map[string]any{
			"team":       issue.Team.Key,
			"state_type": issue.State.Type,
		},
	}

	if issue.Project != nil {
		item.ProjectFields["project"] = issue.Project.Name
	}

	return item
}
