package github

import (
	"github.com/shivamstaq/github-symphony/internal/domain"
	gh "github.com/shivamstaq/github-symphony/internal/github"
)

// ToDomainWorkItem converts a github.NormalizedItem to the canonical domain.WorkItem.
// The types are field-aligned by design, so this is a direct struct copy.
func ToDomainWorkItem(n gh.NormalizedItem) domain.WorkItem {
	item := domain.WorkItem{
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
		item.Repository = &domain.Repository{
			Owner:         n.Repository.Owner,
			Name:          n.Repository.Name,
			FullName:      n.Repository.FullName,
			DefaultBranch: n.Repository.DefaultBranch,
			CloneURLHTTPS: n.Repository.CloneURLHTTPS,
		}
	}

	for _, b := range n.BlockedBy {
		item.BlockedBy = append(item.BlockedBy, domain.BlockerRef{
			ID:         b.ID,
			Identifier: b.Identifier,
			State:      b.State,
		})
	}

	for _, c := range n.SubIssues {
		item.SubIssues = append(item.SubIssues, domain.ChildRef{
			ID:         c.ID,
			Identifier: c.Identifier,
			State:      c.State,
		})
	}

	for _, p := range n.LinkedPRs {
		item.LinkedPRs = append(item.LinkedPRs, domain.PRRef{
			ID:      p.ID,
			Number:  p.Number,
			State:   p.State,
			IsDraft: p.IsDraft,
			URL:     p.URL,
		})
	}

	return item
}
