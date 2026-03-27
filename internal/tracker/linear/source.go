package linear

import (
	"context"
	"fmt"

	"github.com/shivamstaq/github-symphony/internal/domain"
	"github.com/shivamstaq/github-symphony/internal/tracker"
)

// Source implements tracker.Tracker for Linear.
type Source struct {
	client *Client
	teamID string
}

// NewSource creates a Linear tracker source.
func NewSource(apiKey, teamID string) *Source {
	return &Source{
		client: NewClient(apiKey),
		teamID: teamID,
	}
}

const issuesQuery = `
query TeamIssues($teamId: String!, $after: String) {
  team(id: $teamId) {
    issues(first: 50, after: $after, orderBy: updatedAt) {
      nodes {
        id
        identifier
        title
        description
        priority
        url
        createdAt
        updatedAt
        branchName
        state { id name type }
        labels { nodes { name } }
        assignee { name }
        team { id key name }
        project { id name }
        relations {
          nodes {
            type
            relatedIssue {
              id
              identifier
              state { id name type }
            }
          }
        }
      }
      pageInfo { hasNextPage endCursor }
    }
  }
}
`

func (s *Source) FetchCandidates(ctx context.Context) ([]domain.WorkItem, error) {
	var allItems []domain.WorkItem
	var cursor string

	for {
		vars := map[string]any{"teamId": s.teamID}
		if cursor != "" {
			vars["after"] = cursor
		}

		var resp IssuesResponse
		if err := s.client.Query(ctx, issuesQuery, vars, &resp); err != nil {
			return nil, fmt.Errorf("fetch linear issues: %w", err)
		}

		for _, node := range resp.Team.Issues.Nodes {
			allItems = append(allItems, Normalize(node))
		}

		if !resp.Team.Issues.PageInfo.HasNextPage {
			break
		}
		cursor = resp.Team.Issues.PageInfo.EndCursor
	}

	return allItems, nil
}

const issuesByIDsQuery = `
query IssuesByIds($ids: [String!]!) {
  issues(filter: { id: { in: $ids } }) {
    nodes {
      id
      identifier
      title
      description
      priority
      url
      createdAt
      updatedAt
      branchName
      state { id name type }
      labels { nodes { name } }
      assignee { name }
      team { id key name }
      project { id name }
      relations {
        nodes {
          type
          relatedIssue {
            id
            identifier
            state { id name type }
          }
        }
      }
    }
  }
}
`

func (s *Source) FetchStates(ctx context.Context, ids []string) ([]domain.WorkItem, error) {
	// Extract Linear IDs from composite IDs (strip "linear:" prefix)
	linearIDs := make([]string, 0, len(ids))
	for _, id := range ids {
		if len(id) > 7 && id[:7] == "linear:" {
			linearIDs = append(linearIDs, id[7:])
		} else {
			linearIDs = append(linearIDs, id)
		}
	}

	vars := map[string]any{"ids": linearIDs}
	var resp IssuesByIDsResponse
	if err := s.client.Query(ctx, issuesByIDsQuery, vars, &resp); err != nil {
		return nil, fmt.Errorf("fetch linear issue states: %w", err)
	}

	items := make([]domain.WorkItem, len(resp.Issues.Nodes))
	for i, node := range resp.Issues.Nodes {
		items[i] = Normalize(node)
	}
	return items, nil
}

func (s *Source) ValidateConfig(ctx context.Context, input tracker.ValidationInput) ([]tracker.ValidationProblem, error) {
	// Fetch team workflow states
	var resp WorkflowStatesResponse
	statesQuery := `query TeamStates($teamId: String!) {
		team(id: $teamId) {
			states { nodes { id name type } }
		}
	}`

	if err := s.client.Query(ctx, statesQuery, map[string]any{"teamId": s.teamID}, &resp); err != nil {
		return nil, fmt.Errorf("fetch workflow states: %w", err)
	}

	// Build set of existing state names
	existing := make(map[string]bool)
	for _, state := range resp.Team.States.Nodes {
		existing[state.Name] = true
	}

	var problems []tracker.ValidationProblem

	// Check active values
	for _, v := range input.ActiveValues {
		if !existing[v] {
			problems = append(problems, tracker.ValidationProblem{
				Kind:   tracker.ProblemMissingStatus,
				Name:   v,
				CanFix: false, // Linear states are workflow-defined, can't auto-create
			})
		}
	}

	// Check terminal values
	for _, v := range input.TerminalValues {
		if !existing[v] {
			problems = append(problems, tracker.ValidationProblem{
				Kind:   tracker.ProblemMissingStatus,
				Name:   v,
				CanFix: false,
			})
		}
	}

	return problems, nil
}

func (s *Source) CreateMissingFields(_ context.Context, _ []tracker.ValidationProblem) error {
	return fmt.Errorf("linear does not support auto-creating workflow states — configure them in Linear settings")
}

// Compile-time interface check.
var _ tracker.Tracker = (*Source)(nil)
