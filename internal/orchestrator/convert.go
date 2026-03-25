package orchestrator

import ghub "github.com/shivamstaq/github-symphony/internal/github"

// ConvertNormalizedItem converts a github.NormalizedItem to an orchestrator.WorkItem.
func ConvertNormalizedItem(n ghub.NormalizedItem) WorkItem {
	item := WorkItem{
		WorkItemID:      n.WorkItemID,
		ProjectItemID:   n.ProjectItemID,
		ContentType:     n.ContentType,
		IssueID:         n.IssueID,
		IssueNumber:     n.IssueNumber,
		IssueIdentifier: n.IssueIdentifier,
		Title:           n.Title,
		Description:     n.Description,
		State:           n.State,
		ProjectStatus:   n.ProjectStatus,
		Priority:        n.Priority,
		Labels:          n.Labels,
		Assignees:       n.Assignees,
		Milestone:       n.Milestone,
		URL:             n.URL,
		CreatedAt:       n.CreatedAt,
		UpdatedAt:       n.UpdatedAt,
		Pass2Failed:     n.Pass2Failed,
	}

	if n.Repository != nil {
		item.Repository = &Repository{
			Owner:         n.Repository.Owner,
			Name:          n.Repository.Name,
			FullName:      n.Repository.FullName,
			DefaultBranch: n.Repository.DefaultBranch,
			CloneURLHTTPS: n.Repository.CloneURLHTTPS,
		}
	}

	for _, b := range n.BlockedBy {
		item.BlockedBy = append(item.BlockedBy, BlockerRef{ID: b.ID, Identifier: b.Identifier, State: b.State})
	}
	for _, s := range n.SubIssues {
		item.SubIssues = append(item.SubIssues, ChildRef{ID: s.ID, Identifier: s.Identifier, State: s.State})
	}
	for _, p := range n.LinkedPRs {
		item.LinkedPRs = append(item.LinkedPRs, PRRef{ID: p.ID, Number: p.Number, State: p.State, IsDraft: p.IsDraft, URL: p.URL})
	}

	return item
}

// ConvertNormalizedItems converts a slice.
func ConvertNormalizedItems(items []ghub.NormalizedItem) []WorkItem {
	result := make([]WorkItem, len(items))
	for i, n := range items {
		result[i] = ConvertNormalizedItem(n)
	}
	return result
}
