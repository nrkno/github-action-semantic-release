package env

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoad_AllVarsSet(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "ghp_test123")
	t.Setenv("GITHUB_REPOSITORY", "owner/repo")
	t.Setenv("GITHUB_REF", "refs/heads/main")
	t.Setenv("GITHUB_REF_NAME", "main")
	t.Setenv("GITHUB_SHA", "abc123def456")
	t.Setenv("GITHUB_EVENT_NAME", "push")
	t.Setenv("GITHUB_BASE_REF", "")
	t.Setenv("GITHUB_EVENT_PATH", "/tmp/event.json")
	t.Setenv("GITHUB_OUTPUT", "/tmp/output")
	t.Setenv("GITHUB_SERVER_URL", "https://github.com")
	t.Setenv("GITHUB_API_URL", "https://api.github.com")
	t.Setenv("GITHUB_RUN_ID", "12345")
	t.Setenv("GITHUB_ACTOR", "testuser")
	t.Setenv("GITHUB_ACTIONS", "true")

	ctx := Load()

	if ctx.Token != "ghp_test123" {
		t.Errorf("Token: got %q, want %q", ctx.Token, "ghp_test123")
	}
	if ctx.Repository != "owner/repo" {
		t.Errorf("Repository: got %q, want %q", ctx.Repository, "owner/repo")
	}
	if ctx.Ref != "refs/heads/main" {
		t.Errorf("Ref: got %q, want %q", ctx.Ref, "refs/heads/main")
	}
	if ctx.RefName != "main" {
		t.Errorf("RefName: got %q, want %q", ctx.RefName, "main")
	}
	if ctx.SHA != "abc123def456" {
		t.Errorf("SHA: got %q, want %q", ctx.SHA, "abc123def456")
	}
	if ctx.EventName != "push" {
		t.Errorf("EventName: got %q, want %q", ctx.EventName, "push")
	}
	if ctx.EventPath != "/tmp/event.json" {
		t.Errorf("EventPath: got %q, want %q", ctx.EventPath, "/tmp/event.json")
	}
	if ctx.Output != "/tmp/output" {
		t.Errorf("Output: got %q, want %q", ctx.Output, "/tmp/output")
	}
	if ctx.ServerURL != "https://github.com" {
		t.Errorf("ServerURL: got %q, want %q", ctx.ServerURL, "https://github.com")
	}
	if ctx.APIURL != "https://api.github.com" {
		t.Errorf("APIURL: got %q, want %q", ctx.APIURL, "https://api.github.com")
	}
	if ctx.RunID != "12345" {
		t.Errorf("RunID: got %q, want %q", ctx.RunID, "12345")
	}
	if ctx.Actor != "testuser" {
		t.Errorf("Actor: got %q, want %q", ctx.Actor, "testuser")
	}
	if !ctx.IsGitHubActions {
		t.Errorf("IsGitHubActions: got %v, want true", ctx.IsGitHubActions)
	}
	if !ctx.HasToken {
		t.Errorf("HasToken: got %v, want true", ctx.HasToken)
	}
}

func TestLoad_AllVarsUnset(t *testing.T) {
	// Unset all GITHUB_* vars to test zero values
	t.Setenv("GITHUB_TOKEN", "")
	t.Setenv("GITHUB_REPOSITORY", "")
	t.Setenv("GITHUB_REF", "")
	t.Setenv("GITHUB_REF_NAME", "")
	t.Setenv("GITHUB_SHA", "")
	t.Setenv("GITHUB_EVENT_NAME", "")
	t.Setenv("GITHUB_BASE_REF", "")
	t.Setenv("GITHUB_EVENT_PATH", "")
	t.Setenv("GITHUB_OUTPUT", "")
	t.Setenv("GITHUB_SERVER_URL", "")
	t.Setenv("GITHUB_API_URL", "")
	t.Setenv("GITHUB_RUN_ID", "")
	t.Setenv("GITHUB_ACTOR", "")
	t.Setenv("GITHUB_ACTIONS", "")

	ctx := Load()

	if ctx.Token != "" {
		t.Errorf("Token: got %q, want empty", ctx.Token)
	}
	if ctx.Repository != "" {
		t.Errorf("Repository: got %q, want empty", ctx.Repository)
	}
	if ctx.Ref != "" {
		t.Errorf("Ref: got %q, want empty", ctx.Ref)
	}
	if ctx.RefName != "" {
		t.Errorf("RefName: got %q, want empty", ctx.RefName)
	}
	if ctx.SHA != "" {
		t.Errorf("SHA: got %q, want empty", ctx.SHA)
	}
	if ctx.EventName != "" {
		t.Errorf("EventName: got %q, want empty", ctx.EventName)
	}
	if ctx.BaseRef != "" {
		t.Errorf("BaseRef: got %q, want empty", ctx.BaseRef)
	}
	if ctx.EventPath != "" {
		t.Errorf("EventPath: got %q, want empty", ctx.EventPath)
	}
	if ctx.Output != "" {
		t.Errorf("Output: got %q, want empty", ctx.Output)
	}
	// ServerURL should default even when GITHUB_SERVER_URL is unset
	if ctx.ServerURL != "https://github.com" {
		t.Errorf("ServerURL: got %q, want %q", ctx.ServerURL, "https://github.com")
	}
	// APIURL should default even when GITHUB_API_URL is unset
	if ctx.APIURL != "https://api.github.com" {
		t.Errorf("APIURL: got %q, want %q", ctx.APIURL, "https://api.github.com")
	}
	if ctx.RunID != "" {
		t.Errorf("RunID: got %q, want empty", ctx.RunID)
	}
	if ctx.Actor != "" {
		t.Errorf("Actor: got %q, want empty", ctx.Actor)
	}
	if ctx.IsGitHubActions {
		t.Errorf("IsGitHubActions: got %v, want false", ctx.IsGitHubActions)
	}
	if ctx.PRNumber != 0 {
		t.Errorf("PRNumber: got %d, want 0", ctx.PRNumber)
	}
	if ctx.HasToken {
		t.Errorf("HasToken: got %v, want false", ctx.HasToken)
	}
}

func TestLoad_PRRefParsing(t *testing.T) {
	tests := []struct {
		name     string
		ref      string
		wantPR   int
		wantName string
	}{
		{
			name:     "PR ref",
			ref:      "refs/pull/42/merge",
			wantPR:   42,
			wantName: "42/merge",
		},
		{
			name:     "Push ref",
			ref:      "refs/heads/main",
			wantPR:   0,
			wantName: "main",
		},
		{
			name:     "Tag ref",
			ref:      "refs/tags/v1.0.0",
			wantPR:   0,
			wantName: "v1.0.0",
		},
		{
			name:     "Malformed PR ref",
			ref:      "refs/pull/abc/merge",
			wantPR:   0,
			wantName: "abc/merge",
		},
		{
			name:     "PR ref with large number",
			ref:      "refs/pull/9999/merge",
			wantPR:   9999,
			wantName: "9999/merge",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("GITHUB_REF", tt.ref)
			t.Setenv("GITHUB_REF_NAME", tt.wantName)

			ctx := Load()

			if ctx.PRNumber != tt.wantPR {
				t.Errorf("PRNumber: got %d, want %d", ctx.PRNumber, tt.wantPR)
			}
			if ctx.RefName != tt.wantName {
				t.Errorf("RefName: got %q, want %q", ctx.RefName, tt.wantName)
			}
		})
	}
}

func TestLoad_IsGitHubActions(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  bool
	}{
		{
			name:  "true",
			value: "true",
			want:  true,
		},
		{
			name:  "false",
			value: "false",
			want:  false,
		},
		{
			name:  "unset",
			value: "",
			want:  false,
		},
		{
			name:  "other value",
			value: "yes",
			want:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("GITHUB_ACTIONS", tt.value)

			ctx := Load()

			if ctx.IsGitHubActions != tt.want {
				t.Errorf("IsGitHubActions: got %v, want %v", ctx.IsGitHubActions, tt.want)
			}
		})
	}
}

func TestLoad_HasToken(t *testing.T) {
	tests := []struct {
		name  string
		token string
		want  bool
	}{
		{
			name:  "with token",
			token: "ghp_secret123",
			want:  true,
		},
		{
			name:  "empty token",
			token: "",
			want:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("GITHUB_TOKEN", tt.token)

			ctx := Load()

			if ctx.HasToken != tt.want {
				t.Errorf("HasToken: got %v, want %v", ctx.HasToken, tt.want)
			}
		})
	}
}

func TestLoad_ServerURLDefault(t *testing.T) {
	t.Setenv("GITHUB_SERVER_URL", "")

	ctx := Load()

	if ctx.ServerURL != "https://github.com" {
		t.Errorf("ServerURL: got %q, want %q", ctx.ServerURL, "https://github.com")
	}
}

func TestLoad_ServerURLCustom(t *testing.T) {
	t.Setenv("GITHUB_SERVER_URL", "https://github.enterprise.com")

	ctx := Load()

	if ctx.ServerURL != "https://github.enterprise.com" {
		t.Errorf("ServerURL: got %q, want %q", ctx.ServerURL, "https://github.enterprise.com")
	}
}

func TestLoad_APIURLDefault(t *testing.T) {
	t.Setenv("GITHUB_API_URL", "")

	ctx := Load()

	if ctx.APIURL != "https://api.github.com" {
		t.Errorf("APIURL: got %q, want %q", ctx.APIURL, "https://api.github.com")
	}
}

func TestLoad_APIURLCustom(t *testing.T) {
	t.Setenv("GITHUB_API_URL", "https://api.github.enterprise.com")

	ctx := Load()

	if ctx.APIURL != "https://api.github.enterprise.com" {
		t.Errorf("APIURL: got %q, want %q", ctx.APIURL, "https://api.github.enterprise.com")
	}
}

func TestLoad_RepositoryFormat(t *testing.T) {
	t.Setenv("GITHUB_REPOSITORY", "owner/repo")

	ctx := Load()

	if ctx.Repository != "owner/repo" {
		t.Errorf("Repository: got %q, want %q", ctx.Repository, "owner/repo")
	}
}

func TestPRNumberFromRef(t *testing.T) {
	tests := []struct {
		name string
		ref  string
		want int
	}{
		{
			name: "valid PR ref",
			ref:  "refs/pull/42/merge",
			want: 42,
		},
		{
			name: "valid PR ref with large number",
			ref:  "refs/pull/9999/merge",
			want: 9999,
		},
		{
			name: "PR ref with head instead of merge",
			ref:  "refs/pull/42/head",
			want: 42,
		},
		{
			name: "push ref",
			ref:  "refs/heads/main",
			want: 0,
		},
		{
			name: "tag ref",
			ref:  "refs/tags/v1.0.0",
			want: 0,
		},
		{
			name: "malformed PR ref with non-numeric",
			ref:  "refs/pull/abc/merge",
			want: 0,
		},
		{
			name: "malformed PR ref with special chars",
			ref:  "refs/pull/42-beta/merge",
			want: 0,
		},
		{
			name: "empty ref",
			ref:  "",
			want: 0,
		},
		{
			name: "incomplete PR ref",
			ref:  "refs/pull/42",
			want: 42,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := prNumberFromRef(tt.ref)
			if got != tt.want {
				t.Errorf("prNumberFromRef(%q): got %d, want %d", tt.ref, got, tt.want)
			}
		})
	}
}

func TestLoad_Idempotent(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "ghp_test123")
	t.Setenv("GITHUB_REPOSITORY", "owner/repo")
	t.Setenv("GITHUB_REF", "refs/pull/42/merge")

	ctx1 := Load()
	ctx2 := Load()

	if ctx1.Token != ctx2.Token {
		t.Errorf("Token mismatch: %q vs %q", ctx1.Token, ctx2.Token)
	}
	if ctx1.Repository != ctx2.Repository {
		t.Errorf("Repository mismatch: %q vs %q", ctx1.Repository, ctx2.Repository)
	}
	if ctx1.PRNumber != ctx2.PRNumber {
		t.Errorf("PRNumber mismatch: %d vs %d", ctx1.PRNumber, ctx2.PRNumber)
	}
}

func TestLoad_PushEventPayload(t *testing.T) {
	payload := `{
		"before": "1111111111111111111111111111111111111111",
		"after": "2222222222222222222222222222222222222222",
		"commits": [
			{
				"id": "2222222222222222222222222222222222222222",
				"message": "feat: add thing",
				"timestamp": "2026-08-12T06:44:19Z"
			}
		]
	}`
	path := filepath.Join(t.TempDir(), "event.json")
	if err := os.WriteFile(path, []byte(payload), 0o600); err != nil {
		t.Fatalf("failed to write event payload: %v", err)
	}

	t.Setenv("GITHUB_EVENT_NAME", "push")
	t.Setenv("GITHUB_EVENT_PATH", path)

	ctx := Load()

	if !ctx.PushPayloadLoaded {
		t.Fatal("expected push payload to be loaded")
	}
	if ctx.PushBefore != "1111111111111111111111111111111111111111" {
		t.Errorf("PushBefore: got %q", ctx.PushBefore)
	}
	if ctx.PushAfter != "2222222222222222222222222222222222222222" {
		t.Errorf("PushAfter: got %q", ctx.PushAfter)
	}
	if len(ctx.PushCommits) != 1 {
		t.Fatalf("expected 1 pushed commit, got %d", len(ctx.PushCommits))
	}
	commit := ctx.PushCommits[0]
	if commit.ID != "2222222222222222222222222222222222222222" {
		t.Errorf("ID: got %q", commit.ID)
	}
	if commit.Message != "feat: add thing" {
		t.Errorf("Message: got %q", commit.Message)
	}
	wantTime := time.Date(2026, 8, 12, 6, 44, 19, 0, time.UTC)
	if !commit.Timestamp.Equal(wantTime) {
		t.Errorf("Timestamp: got %v, want %v", commit.Timestamp, wantTime)
	}
}

func TestLoad_PushEventPayloadMissingFileDoesNotFail(t *testing.T) {
	t.Setenv("GITHUB_EVENT_NAME", "push")
	t.Setenv("GITHUB_EVENT_PATH", filepath.Join(t.TempDir(), "missing.json"))

	ctx := Load()

	if ctx.PushPayloadLoaded {
		t.Fatal("expected missing push payload not to be loaded")
	}
	if len(ctx.PushCommits) != 0 {
		t.Fatalf("expected no pushed commits, got %d", len(ctx.PushCommits))
	}
}
