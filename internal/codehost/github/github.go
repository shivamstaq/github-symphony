// Package github implements codehost.CodeHost for GitHub.
package github

import (
	"context"
	"fmt"

	"github.com/shivamstaq/github-symphony/internal/codehost"
	gh "github.com/shivamstaq/github-symphony/internal/github"
)

// Host implements codehost.CodeHost by delegating to github.WriteBack and github.GraphQLClient.
type Host struct {
	wb     *gh.WriteBack
	client *gh.GraphQLClient
}

// New creates a GitHub CodeHost adapter.
func New(baseURL, token string) *Host {
	graphqlEndpoint := baseURL + "/graphql"
	return &Host{
		wb:     gh.NewWriteBack(baseURL, graphqlEndpoint, token),
		client: gh.NewGraphQLClient(graphqlEndpoint, token),
	}
}

func (h *Host) UpsertPR(ctx context.Context, params codehost.PRParams) (*codehost.PRResult, error) {
	result, err := h.wb.UpsertPR(ctx, gh.PRParams{
		Owner:      params.Owner,
		Repo:       params.Repo,
		Title:      params.Title,
		Body:       params.Body,
		HeadBranch: params.HeadBranch,
		BaseBranch: params.BaseBranch,
		Draft:      params.Draft,
	})
	if err != nil {
		return nil, fmt.Errorf("github codehost: upsert PR: %w", err)
	}
	return &codehost.PRResult{
		Number:  result.Number,
		URL:     result.URL,
		State:   result.State,
		IsDraft: result.IsDraft,
		Created: result.Created,
	}, nil
}

func (h *Host) CommentOnItem(ctx context.Context, ref codehost.ItemRef, body string) (string, error) {
	return h.wb.CommentOnIssue(ctx, ref.Owner, ref.Repo, ref.Number, body)
}

func (h *Host) UpdateProjectStatus(ctx context.Context, params codehost.StatusUpdateParams) error {
	return h.wb.UpdateProjectField(ctx, params.ProjectID, params.ItemID, params.FieldID, params.OptionID)
}

func (h *Host) FetchProjectMeta(ctx context.Context, params codehost.ProjectMetaParams) (*codehost.ProjectMeta, error) {
	meta, err := h.client.FetchProjectFieldMeta(ctx, params.Owner, params.ProjectNumber, params.Scope, params.FieldName)
	if err != nil {
		return nil, fmt.Errorf("github codehost: fetch project meta: %w", err)
	}
	return &codehost.ProjectMeta{
		ProjectID: meta.ProjectID,
		FieldID:   meta.FieldID,
		Options:   meta.Options,
	}, nil
}

// Compile-time interface check.
var _ codehost.CodeHost = (*Host)(nil)
