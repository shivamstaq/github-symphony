// Package linear implements the tracker.Tracker interface for Linear.
package linear

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const apiURL = "https://api.linear.app/graphql"

// Client is a Linear GraphQL API client.
type Client struct {
	apiKey     string
	httpClient *http.Client
}

// NewClient creates a Linear API client.
func NewClient(apiKey string) *Client {
	return &Client{
		apiKey: apiKey,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// graphqlRequest is the JSON body for a GraphQL request.
type graphqlRequest struct {
	Query     string         `json:"query"`
	Variables map[string]any `json:"variables,omitempty"`
}

// graphqlResponse is the JSON response from a GraphQL request.
type graphqlResponse struct {
	Data   json.RawMessage `json:"data"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

// Query executes a GraphQL query and unmarshals the data field into target.
func (c *Client) Query(ctx context.Context, query string, vars map[string]any, target any) error {
	body, err := json.Marshal(graphqlRequest{Query: query, Variables: vars})
	if err != nil {
		return fmt.Errorf("marshal query: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", apiURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("linear API request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != 200 {
		return fmt.Errorf("linear API error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var gqlResp graphqlResponse
	if err := json.Unmarshal(respBody, &gqlResp); err != nil {
		return fmt.Errorf("unmarshal response: %w", err)
	}

	if len(gqlResp.Errors) > 0 {
		return fmt.Errorf("linear GraphQL error: %s", gqlResp.Errors[0].Message)
	}

	if target != nil {
		if err := json.Unmarshal(gqlResp.Data, target); err != nil {
			return fmt.Errorf("unmarshal data: %w", err)
		}
	}

	return nil
}
