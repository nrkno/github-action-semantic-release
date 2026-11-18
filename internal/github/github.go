package github

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	gogithub "github.com/google/go-github/v93/github"
	"golang.org/x/oauth2"
)

// Typed errors for GitHub API responses.
var (
	ErrUnauthorized  = errors.New("unauthorized: GITHUB_TOKEN is missing or expired")
	ErrForbidden     = errors.New("forbidden: GITHUB_TOKEN lacks required permissions")
	ErrNotFound      = errors.New("not found: resource does not exist")
	ErrUnprocessable = errors.New("unprocessable: GitHub API rejected request")
	ErrRateLimited   = errors.New("rate limited: GitHub API rate limit exceeded")
	ErrServerError   = errors.New("server error: GitHub API returned 5xx")
)

// Client wraps the go-github client.
type Client struct {
	client *gogithub.Client
}

// PR represents a GitHub pull request.
type PR struct {
	Number int
	URL    string
	Title  string
}

// Release represents a GitHub release.
type Release struct {
	ID      int64
	TagName string
	HTMLURL string
	Body    string
}

// CreateReleaseOptions for CreateRelease.
type CreateReleaseOptions struct {
	TagName string
	Name    string
	Body    string
}

// NewClient constructs a GitHub API client.
// baseURL is the value of GITHUB_API_URL (may be empty or without trailing slash).
// For the default GitHub.com API URL, the standard client is returned.
// For any other non-empty URL (GHES), WithEnterpriseURLs is used.
// A trailing slash is added automatically — go-github requires it.
func NewClient(token, baseURL string) (*Client, error) {
	httpClient := oauth2.NewClient(context.Background(),
		oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token}))

	opts := []gogithub.ClientOptionsFunc{gogithub.WithHTTPClient(httpClient)}

	if baseURL != "" {
		if !strings.HasSuffix(baseURL, "/") {
			baseURL += "/"
		}
		const ghDefault = "https://api.github.com/"
		if baseURL != ghDefault {
			opts = append(opts, gogithub.WithEnterpriseURLs(baseURL, baseURL))
		}
	}
	ghClient, err := gogithub.NewClient(opts...)
	if err != nil {
		return nil, fmt.Errorf("invalid enterprise GitHub URL %q: %w", baseURL, err)
	}
	return &Client{client: ghClient}, nil
}

// GetReleaseByTag retrieves a release by tag name.
// Returns (Release, nil) on 200, typed errors on other status codes.
func (c *Client) GetReleaseByTag(ctx context.Context, owner, repo, tag string) (*Release, error) {
	rel, resp, err := c.client.Repositories.GetReleaseByTag(ctx, owner, repo, tag)
	if err != nil {
		return nil, mapHTTPError(rawResponse(resp), err)
	}

	return &Release{
		ID:      rel.GetID(),
		TagName: rel.GetTagName(),
		HTMLURL: rel.GetHTMLURL(),
		Body:    rel.GetBody(),
	}, nil
}

// CreateRelease creates a new GitHub release.
// On HTTP 422 with errors[].code == "already_exists", calls GetReleaseByTag and returns that result.
// Returns (Release, nil) on success, typed errors on other failures.
func (c *Client) CreateRelease(ctx context.Context, owner, repo string, opts CreateReleaseOptions) (*Release, error) {
	input := gogithub.CreateReleaseRequest{
		TagName: opts.TagName,
		Name:    &opts.Name,
		Body:    &opts.Body,
	}

	rel, resp, err := c.client.Repositories.CreateRelease(ctx, owner, repo, input)
	if err != nil {
		// Check if this is a 422 with already_exists error code
		if resp != nil && resp.Response != nil && resp.StatusCode == http.StatusUnprocessableEntity {
			if isAlreadyExists(resp.Response) {
				// Recover by fetching the existing release
				return c.GetReleaseByTag(ctx, owner, repo, opts.TagName)
			}
		}
		return nil, mapHTTPError(rawResponse(resp), err)
	}

	return &Release{
		ID:      rel.GetID(),
		TagName: rel.GetTagName(),
		HTMLURL: rel.GetHTMLURL(),
		Body:    rel.GetBody(),
	}, nil
}

// ListPRsForCommit retrieves all PRs associated with a commit SHA.
// Calls GET /repos/{owner}/{repo}/commits/{sha}/pulls
// Returns ([]PR, nil) on success, typed errors on failure.
func (c *Client) ListPRsForCommit(ctx context.Context, owner, repo, sha string) ([]PR, error) {
	opts := &gogithub.ListOptions{PerPage: 100}
	result := []PR{} // Initialize to empty slice, not nil

	for {
		prs, resp, err := c.client.PullRequests.ListPullRequestsWithCommit(ctx, owner, repo, sha, opts)
		if err != nil {
			return nil, mapHTTPError(rawResponse(resp), err)
		}

		for _, pr := range prs {
			result = append(result, PR{
				Number: pr.GetNumber(),
				URL:    pr.GetHTMLURL(),
				Title:  pr.GetTitle(),
			})
		}

		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}

	return result, nil
}

// SearchPRsForCommit searches for PRs using a query string.
// Calls GET /search/issues?q=<query>
// Returns ([]PR, nil) on success, typed errors on failure.
func (c *Client) SearchPRsForCommit(ctx context.Context, query string) ([]PR, error) {
	opts := &gogithub.SearchOptions{
		ListOptions: gogithub.ListOptions{PerPage: 100},
	}
	result := []PR{} // Initialize to empty slice, not nil

	for {
		searchResult, resp, err := c.client.Search.Issues(ctx, query, opts)
		if err != nil {
			return nil, mapHTTPError(rawResponse(resp), err)
		}

		for _, issue := range searchResult.Issues {
			result = append(result, PR{
				Number: issue.GetNumber(),
				URL:    issue.GetHTMLURL(),
				Title:  issue.GetTitle(),
			})
		}

		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}

	return result, nil
}

// PostPRComment posts a comment on a pull request.
// Returns nil on success, typed errors on failure.
func (c *Client) PostPRComment(ctx context.Context, owner, repo string, prNumber int, body string) error {
	_, resp, err := c.client.Issues.CreateComment(ctx, owner, repo, prNumber, gogithub.IssueCommentRequest{
		Body: body,
	})
	if err != nil {
		return mapHTTPError(rawResponse(resp), err)
	}
	return nil
}

// FindPRComment searches for an existing comment on a PR by marker string.
// marker: substring to search for in comment body (e.g., "<!-- semrel-notify:v1.4.0 -->")
// Returns (true, nil) if found, (false, nil) if not found, (false, error) on API failure.
func (c *Client) FindPRComment(ctx context.Context, owner, repo string, prNumber int, marker string) (bool, error) {
	opts := &gogithub.IssueListCommentsOptions{
		ListOptions: gogithub.ListOptions{PerPage: 100},
	}

	for {
		comments, resp, err := c.client.Issues.ListComments(ctx, owner, repo, prNumber, opts)
		if err != nil {
			return false, mapHTTPError(rawResponse(resp), err)
		}

		for _, comment := range comments {
			if strings.Contains(comment.GetBody(), marker) {
				return true, nil
			}
		}

		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}

	return false, nil
}

// mapHTTPError converts HTTP status codes and errors to typed errors.
// The response body (up to 4096 bytes) is embedded in the error when non-empty.
func mapHTTPError(resp *http.Response, err error) error {
	if resp == nil {
		// Network or other error
		if err != nil {
			return fmt.Errorf("github api error: %w", err)
		}
		return ErrServerError
	}

	var bodyStr string
	if resp.Body != nil {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		_ = resp.Body.Close()
		bodyStr = strings.TrimSpace(string(data))
	}

	switch resp.StatusCode {
	case http.StatusUnauthorized:
		if bodyStr != "" {
			return fmt.Errorf("%w: %s", ErrUnauthorized, bodyStr)
		}
		return ErrUnauthorized
	case http.StatusForbidden:
		if bodyStr != "" {
			return fmt.Errorf("%w: %s", ErrForbidden, bodyStr)
		}
		return ErrForbidden
	case http.StatusNotFound:
		if bodyStr != "" {
			return fmt.Errorf("%w: %s", ErrNotFound, bodyStr)
		}
		return ErrNotFound
	case http.StatusUnprocessableEntity:
		if bodyStr != "" {
			return fmt.Errorf("%w: %s", ErrUnprocessable, bodyStr)
		}
		return ErrUnprocessable
	case http.StatusTooManyRequests:
		if bodyStr != "" {
			return fmt.Errorf("%w: %s", ErrRateLimited, bodyStr)
		}
		return ErrRateLimited
	case http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		if bodyStr != "" {
			return fmt.Errorf("%w: %s", ErrServerError, bodyStr)
		}
		return ErrServerError
	default:
		if resp.StatusCode >= 500 {
			if bodyStr != "" {
				return fmt.Errorf("%w: %s", ErrServerError, bodyStr)
			}
			return ErrServerError
		}
		if err != nil {
			if bodyStr != "" {
				return fmt.Errorf("github api error (status %d): %w: %s", resp.StatusCode, err, bodyStr)
			}
			return fmt.Errorf("github api error: %w", err)
		}
		if bodyStr != "" {
			return fmt.Errorf("github api error: status %d: %s", resp.StatusCode, bodyStr)
		}
		return fmt.Errorf("github api error: status %d", resp.StatusCode)
	}
}

// isAlreadyExists checks if the 422 response contains an "already_exists" error code.
func isAlreadyExists(resp *http.Response) bool {
	if resp == nil || resp.Body == nil {
		return false
	}

	// Read the response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return false
	}

	// Reset the body for potential re-reading
	resp.Body = io.NopCloser(strings.NewReader(string(body)))

	// Parse the error response
	var errResp struct {
		Errors []struct {
			Code string `json:"code"`
		} `json:"errors"`
	}

	if err := json.Unmarshal(body, &errResp); err != nil {
		return false
	}

	// Check if any error has code "already_exists"
	for _, e := range errResp.Errors {
		if e.Code == "already_exists" {
			return true
		}
	}

	return false
}

// rawResponse returns the underlying *http.Response, or nil when the request
// failed before a response arrived (go-github then returns a nil *Response).
func rawResponse(resp *gogithub.Response) *http.Response {
	if resp == nil {
		return nil
	}
	return resp.Response
}
