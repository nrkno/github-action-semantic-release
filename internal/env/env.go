package env

import (
	"encoding/json"
	"os"
	"strconv"
	"strings"
	"time"
)

// Context holds all GitHub Actions environment variables.
// Missing variables produce zero values (degraded mode).
type Context struct {
	Token           string // GITHUB_TOKEN
	Repository      string // GITHUB_REPOSITORY (format: "owner/repo")
	Ref             string // GITHUB_REF (e.g., "refs/heads/main" or "refs/pull/42/merge")
	RefName         string // GITHUB_REF_NAME (e.g., "main" or "42/merge")
	SHA             string // GITHUB_SHA
	EventName       string // GITHUB_EVENT_NAME (e.g., "pull_request", "push", "release")
	BaseRef         string // GITHUB_BASE_REF (only set on pull_request events)
	EventPath       string // GITHUB_EVENT_PATH (path to event JSON file)
	Output          string // GITHUB_OUTPUT (path to output file)
	ServerURL       string // GITHUB_SERVER_URL (defaults to "https://github.com")
	APIURL          string // GITHUB_API_URL (defaults to "https://api.github.com")
	RunID           string // GITHUB_RUN_ID
	Actor           string // GITHUB_ACTOR
	IsGitHubActions bool   // true if GITHUB_ACTIONS == "true"
	PRNumber        int    // extracted from GITHUB_REF; 0 if not a PR
	HasToken        bool   // true if Token != ""
	PushPayloadLoaded bool // true when a push event payload was read successfully
	PushBefore      string // push event before SHA
	PushAfter       string // push event after SHA
	PushCommits     []PushCommit // commits listed in the push event payload
}

// PushCommit is the subset of a GitHub push event commit used by lint.
type PushCommit struct {
	ID        string
	Message   string
	Timestamp time.Time
}

// Load reads all GITHUB_* environment variables and returns a Context.
// Never errors. Missing vars produce zero values (degraded mode).
func Load() Context {
	ctx := Context{
		Token:      os.Getenv("GITHUB_TOKEN"),
		Repository: os.Getenv("GITHUB_REPOSITORY"),
		Ref:        os.Getenv("GITHUB_REF"),
		RefName:    os.Getenv("GITHUB_REF_NAME"),
		SHA:        os.Getenv("GITHUB_SHA"),
		EventName:  os.Getenv("GITHUB_EVENT_NAME"),
		BaseRef:    os.Getenv("GITHUB_BASE_REF"),
		EventPath:  os.Getenv("GITHUB_EVENT_PATH"),
		Output:     os.Getenv("GITHUB_OUTPUT"),
		RunID:      os.Getenv("GITHUB_RUN_ID"),
		Actor:      os.Getenv("GITHUB_ACTOR"),
	}

	// Set ServerURL with default
	ctx.ServerURL = os.Getenv("GITHUB_SERVER_URL")
	if ctx.ServerURL == "" {
		ctx.ServerURL = "https://github.com"
	}

	// Set APIURL with default
	ctx.APIURL = os.Getenv("GITHUB_API_URL")
	if ctx.APIURL == "" {
		ctx.APIURL = "https://api.github.com"
	}

	// Set IsGitHubActions
	ctx.IsGitHubActions = os.Getenv("GITHUB_ACTIONS") == "true"

	// Set HasToken
	ctx.HasToken = ctx.Token != ""

	// Extract PR number from GITHUB_REF
	ctx.PRNumber = prNumberFromRef(ctx.Ref)

	if ctx.EventName == "push" {
		ctx.PushBefore, ctx.PushAfter, ctx.PushCommits, ctx.PushPayloadLoaded = loadPushPayload(ctx.EventPath)
	}

	return ctx
}

type pushEventPayload struct {
	Before  string `json:"before"`
	After   string `json:"after"`
	Commits []struct {
		ID        string    `json:"id"`
		Message   string    `json:"message"`
		Timestamp time.Time `json:"timestamp"`
	} `json:"commits"`
}

func loadPushPayload(path string) (before, after string, commits []PushCommit, loaded bool) {
	if path == "" {
		return "", "", nil, false
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return "", "", nil, false
	}

	var payload pushEventPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return "", "", nil, false
	}

	out := make([]PushCommit, 0, len(payload.Commits))
	for _, commit := range payload.Commits {
		out = append(out, PushCommit{
			ID:        commit.ID,
			Message:   commit.Message,
			Timestamp: commit.Timestamp,
		})
	}

	return payload.Before, payload.After, out, true
}

// prNumberFromRef extracts the PR number from GITHUB_REF.
// GITHUB_REF format on PR: "refs/pull/<N>/merge"
// Returns N as int, or 0 if not a PR ref or on parse error.
func prNumberFromRef(ref string) int {
	// Check if this is a pull request ref
	if !strings.HasPrefix(ref, "refs/pull/") {
		return 0
	}

	// Remove "refs/pull/" prefix
	remainder := strings.TrimPrefix(ref, "refs/pull/")

	// Split by "/" to get the PR number (before the next slash)
	parts := strings.Split(remainder, "/")
	if len(parts) == 0 {
		return 0
	}

	// Try to parse the first part as an integer
	prNum, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0
	}

	return prNum
}
