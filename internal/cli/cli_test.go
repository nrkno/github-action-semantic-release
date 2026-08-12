package cli

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/nrkno/github-action-semantic-release/internal/conventional"
	"github.com/nrkno/github-action-semantic-release/internal/git"
	"github.com/nrkno/github-action-semantic-release/internal/github"
	"github.com/nrkno/github-action-semantic-release/internal/notes"
	"github.com/nrkno/github-action-semantic-release/internal/semver"
	"github.com/stretchr/testify/require"
)

// Helper to create test tags with unexported targetSHA field
// Note: We can't set unexported fields via reflection in tests, so we'll work around it
// by not testing the TargetSHA() method directly in these tests
func newTestTag(name, sha string) *git.Tag {
	return &git.Tag{
		Name: name,
		SHA:  sha,
	}
}

type mockGitClient struct {
	latestTag          *git.Tag
	latestTagErr       error
	latestTagCalls     int
	findTagByNameTag   *git.Tag
	findTagByNameErr   error
	commits            []git.Commit
	commitsErr         error
	commitsSinceTagCalls int
	commitsSinceRef    []git.Commit
	commitsSinceRefErr error
	sinceRefArg        string
	createdTag         *git.Tag
	createTagErr       error
	pushedTag          string
	pushTagErr         error
	isShallowRepo      bool
	previousTag        *git.Tag
	previousTagErr     error
	commitsBetween     []git.Commit
	commitsBetweenErr  error
}

func (m *mockGitClient) FindLatestAnnotatedTag(tagPrefix string) (*git.Tag, error) {
	m.latestTagCalls++
	if m.isShallowRepo {
		return nil, git.ShallowRepoError{Message: "repository is a shallow clone"}
	}
	return m.latestTag, m.latestTagErr
}

func (m *mockGitClient) FindTagByName(name string) (*git.Tag, error) {
	return m.findTagByNameTag, m.findTagByNameErr
}

func (m *mockGitClient) ListCommitsSinceTag(tag *git.Tag) ([]git.Commit, error) {
	m.commitsSinceTagCalls++
	return m.commits, m.commitsErr
}

func (m *mockGitClient) ListCommitsSinceRef(refName string) ([]git.Commit, error) {
	m.sinceRefArg = refName
	return m.commitsSinceRef, m.commitsSinceRefErr
}

func (m *mockGitClient) CreateAnnotatedTag(name, message string) (*git.Tag, error) {
	if m.createTagErr != nil {
		return nil, m.createTagErr
	}
	m.createdTag = newTestTag(name, "abc123")
	return m.createdTag, nil
}

func (m *mockGitClient) PushTag(ctx context.Context, tagName string, auth git.BasicAuth) error {
	m.pushedTag = tagName
	return m.pushTagErr
}

func (m *mockGitClient) FindPreviousAnnotatedTag(current *git.Tag) (*git.Tag, error) {
	return m.previousTag, m.previousTagErr
}

func (m *mockGitClient) ListCommitsBetweenTags(from, to *git.Tag) ([]git.Commit, error) {
	return m.commitsBetween, m.commitsBetweenErr
}

type mockGitHubClient struct {
	releaseByTag     *github.Release
	releaseByTagErr  error
	createdRelease   *github.Release
	createReleaseErr error
	prs              []github.PR
	prsErr           error
	prsPerSHA        map[string][]github.PR // per-SHA override; if set, returned instead of prs
	searchPRs        []github.PR
	searchPRsErr     error
	commentPosted    bool
	commentPostCount int
	commentErr       error
	commentExists    bool
	findCommentErr   error
}

func (m *mockGitHubClient) GetReleaseByTag(ctx context.Context, owner, repo, tag string) (*github.Release, error) {
	return m.releaseByTag, m.releaseByTagErr
}

func (m *mockGitHubClient) CreateRelease(ctx context.Context, owner, repo string, opts github.CreateReleaseOptions) (*github.Release, error) {
	if m.createReleaseErr != nil {
		return nil, m.createReleaseErr
	}
	m.createdRelease = &github.Release{
		TagName: opts.TagName,
		Body:    opts.Body,
	}
	return m.createdRelease, nil
}

func (m *mockGitHubClient) ListPRsForCommit(ctx context.Context, owner, repo, sha string) ([]github.PR, error) {
	if m.prsPerSHA != nil {
		if prs, ok := m.prsPerSHA[sha]; ok {
			return prs, m.prsErr
		}
		return nil, m.prsErr
	}
	return m.prs, m.prsErr
}

func (m *mockGitHubClient) SearchPRsForCommit(ctx context.Context, query string) ([]github.PR, error) {
	return m.searchPRs, m.searchPRsErr
}

func (m *mockGitHubClient) PostPRComment(ctx context.Context, owner, repo string, prNumber int, body string) error {
	m.commentPosted = true
	m.commentPostCount++
	return m.commentErr
}

func (m *mockGitHubClient) FindPRComment(ctx context.Context, owner, repo string, prNumber int, marker string) (bool, error) {
	return m.commentExists, m.findCommentErr
}

// Tests

func TestLintValidCommits(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	// Hermetic: neutralize CI runner context (tests themselves run inside a
	// pull_request event on GitHub Actions, which would change the lint range).
	t.Setenv("GITHUB_EVENT_NAME", "")
	t.Setenv("GITHUB_BASE_REF", "")

	gitClient := &mockGitClient{
		commits: []git.Commit{
			{
				SHA:      "abc123def456",
				ShortSHA: "abc123d",
				Message:  "feat: add new feature",
			},
			{
				SHA:      "def456ghi789",
				ShortSHA: "def456g",
				Message:  "fix: resolve bug",
			},
		},
	}

	cmd := cmdLint(gitClient, logger)
	cmd.SetArgs([]string{})

	err := cmd.ExecuteContext(context.Background())
	if err != nil {
		t.Errorf("lint with valid commits should not error, got: %v", err)
	}
}

func TestLintInvalidCommits(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	// Hermetic: neutralize CI runner context (see TestLintValidCommits).
	t.Setenv("GITHUB_EVENT_NAME", "")
	t.Setenv("GITHUB_BASE_REF", "")

	gitClient := &mockGitClient{
		commits: []git.Commit{
			{
				SHA:      "abc123def456",
				ShortSHA: "abc123d",
				Message:  "invalid commit message",
			},
		},
	}

	cmd := cmdLint(gitClient, logger)
	cmd.SetArgs([]string{})

	err := cmd.ExecuteContext(context.Background())
	if err == nil {
		t.Error("lint with invalid commits should error")
	}
}

// TestLintPullRequestUsesBaseRef is the regression test for the lint-range bug:
// on pull_request events lint must validate baseRef..HEAD, not latestTag..HEAD.
// The mock's tag-range commits contain a non-conventional commit (simulating a
// bad squash merge already on main); the baseRef range contains only the PR's
// own conventional commits. Lint must pass and must query the base ref.
func TestLintPullRequestUsesBaseRef(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	t.Setenv("GITHUB_EVENT_NAME", "pull_request")
	t.Setenv("GITHUB_BASE_REF", "main")

	gitClient := &mockGitClient{
		// latestTag..HEAD range — contains history that must NOT be linted
		commits: []git.Commit{
			{
				SHA:      "badbadbadbad",
				ShortSHA: "badbadb",
				Message:  "Multiple interface support (#159)",
			},
		},
		// baseRef..HEAD range — the PR's own commits
		commitsSinceRef: []git.Commit{
			{
				SHA:      "abc123def456",
				ShortSHA: "abc123d",
				Message:  "feat: add new feature",
			},
		},
	}

	cmd := cmdLint(gitClient, logger)
	cmd.SetArgs([]string{})

	err := cmd.ExecuteContext(context.Background())
	if err != nil {
		t.Errorf("lint on pull_request should only validate baseRef..HEAD, got error: %v", err)
	}
	if gitClient.sinceRefArg != "main" {
		t.Errorf("expected ListCommitsSinceRef called with 'main', got %q", gitClient.sinceRefArg)
	}
}

// TestLintPullRequestBaseRefViolations verifies violations in the PR's own
// commits still fail the lint when using the baseRef range.
func TestLintPullRequestBaseRefViolations(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	t.Setenv("GITHUB_EVENT_NAME", "pull_request")
	t.Setenv("GITHUB_BASE_REF", "main")

	gitClient := &mockGitClient{
		commitsSinceRef: []git.Commit{
			{
				SHA:      "abc123def456",
				ShortSHA: "abc123d",
				Message:  "not a conventional commit",
			},
		},
	}

	cmd := cmdLint(gitClient, logger)
	cmd.SetArgs([]string{})

	err := cmd.ExecuteContext(context.Background())
	if err == nil {
		t.Error("lint should fail when PR commits have violations")
	}
}

func TestLintPushUsesEventPayloadCommits(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	payload := `{
		"before": "1111111111111111111111111111111111111111",
		"after": "2222222222222222222222222222222222222222",
		"commits": [
			{
				"id": "2222222222222222222222222222222222222222",
				"message": "chore: patch azure devops to latest release",
				"timestamp": "2026-08-12T06:44:19Z"
			}
		]
	}`
	path := filepath.Join(t.TempDir(), "event.json")
	require.NoError(t, os.WriteFile(path, []byte(payload), 0o600))
	t.Setenv("GITHUB_EVENT_NAME", "push")
	t.Setenv("GITHUB_EVENT_PATH", path)
	t.Setenv("GITHUB_BASE_REF", "")

	gitClient := &mockGitClient{
		commits: []git.Commit{{
			SHA:      "badbadbadbad",
			ShortSHA: "badbadb",
			Message:  "historical non conventional commit",
		}},
	}
	cmd := cmdLint(gitClient, logger)
	cmd.SetArgs([]string{})

	err := cmd.ExecuteContext(context.Background())

	require.NoError(t, err)
	require.Equal(t, 0, gitClient.latestTagCalls)
	require.Equal(t, 0, gitClient.commitsSinceTagCalls)
}

func TestLintPushPayloadViolationsFail(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	payload := `{
		"before": "1111111111111111111111111111111111111111",
		"after": "2222222222222222222222222222222222222222",
		"commits": [
			{
				"id": "2222222222222222222222222222222222222222",
				"message": "small adjustment",
				"timestamp": "2026-08-12T06:44:19Z"
			}
		]
	}`
	path := filepath.Join(t.TempDir(), "event.json")
	require.NoError(t, os.WriteFile(path, []byte(payload), 0o600))
	t.Setenv("GITHUB_EVENT_NAME", "push")
	t.Setenv("GITHUB_EVENT_PATH", path)
	t.Setenv("GITHUB_BASE_REF", "")

	cmd := cmdLint(&mockGitClient{}, logger)
	cmd.SetArgs([]string{})

	err := cmd.ExecuteContext(context.Background())

	require.Error(t, err)
	require.Contains(t, err.Error(), "commit violation")
}

func TestLintPushExplicitFromRefOverridesPayload(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	payload := `{
		"before": "1111111111111111111111111111111111111111",
		"after": "2222222222222222222222222222222222222222",
		"commits": [
			{
				"id": "2222222222222222222222222222222222222222",
				"message": "bad pushed message",
				"timestamp": "2026-08-12T06:44:19Z"
			}
		]
	}`
	path := filepath.Join(t.TempDir(), "event.json")
	require.NoError(t, os.WriteFile(path, []byte(payload), 0o600))
	t.Setenv("GITHUB_EVENT_NAME", "push")
	t.Setenv("GITHUB_EVENT_PATH", path)
	t.Setenv("GITHUB_BASE_REF", "")

	gitClient := &mockGitClient{
		commitsSinceRef: []git.Commit{{
			SHA:      "abc123def456",
			ShortSHA: "abc123d",
			Message:  "fix: valid override range",
		}},
	}
	cmd := cmdLint(gitClient, logger)
	cmd.SetArgs([]string{"--from-ref", "v1.0.0"})

	err := cmd.ExecuteContext(context.Background())

	require.NoError(t, err)
	require.Equal(t, "v1.0.0", gitClient.sinceRefArg)
}

func TestLintShallowRepo(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	t.Setenv("GITHUB_EVENT_NAME", "push")
	t.Setenv("GITHUB_EVENT_PATH", "") // neutralize CI runner's event payload

	gitClient := &mockGitClient{
		isShallowRepo: true,
		latestTagErr:  git.ShallowRepoError{Message: "repository is a shallow clone"},
	}

	cmd := cmdLint(gitClient, logger)
	cmd.SetArgs([]string{})

	// Redirect stderr to capture error output
	oldStderr := os.Stderr
	_, w, _ := os.Pipe()
	os.Stderr = w

	err := cmd.ExecuteContext(context.Background())

	w.Close()
	os.Stderr = oldStderr

	if err == nil {
		t.Error("lint on shallow repo should error")
	}
}

func TestReleaseNewVersion(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	t.Setenv("GITHUB_REPOSITORY", "owner/repo")

	gitClient := &mockGitClient{
		latestTag: nil, // Bootstrap case
		commits: []git.Commit{
			{
				SHA:      "abc123def456",
				ShortSHA: "abc123d",
				Message:  "feat: initial feature",
			},
		},
	}

	githubClient := &mockGitHubClient{
		releaseByTagErr: github.ErrNotFound,
	}

	cmd := cmdRelease(gitClient, githubClient, logger)
	cmd.SetArgs([]string{"--dry-run"})

	err := cmd.ExecuteContext(context.Background())
	if err != nil {
		t.Errorf("release should not error, got: %v", err)
	}
}

func TestReleaseIdempotentExists(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	t.Setenv("GITHUB_REPOSITORY", "owner/repo")

	gitClient := &mockGitClient{
		latestTag: newTestTag("v0.0.1", "abc123"),
		commits:   []git.Commit{},
	}

	githubClient := &mockGitHubClient{
		releaseByTag: &github.Release{
			TagName: "v0.0.1",
		},
	}

	cmd := cmdRelease(gitClient, githubClient, logger)
	cmd.SetArgs([]string{})

	err := cmd.ExecuteContext(context.Background())
	if err != nil {
		t.Errorf("release idempotent (exists) should not error, got: %v", err)
	}
}

func TestNotifySkippedWhenTagNotSet(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	// SEMREL_TAG deliberately not set
	t.Setenv("GITHUB_REPOSITORY", "owner/repo")

	gitClient := &mockGitClient{}
	githubClient := &mockGitHubClient{}

	root := Root(gitClient, githubClient, logger)
	root.SetArgs([]string{"notify"})

	err := root.ExecuteContext(context.Background())
	if err != nil {
		t.Errorf("notify without SEMREL_TAG should not error, got: %v", err)
	}
	if githubClient.commentPostCount != 0 {
		t.Errorf("expected 0 comments posted, got %d", githubClient.commentPostCount)
	}
}

func TestNotifyPostsCommentOnAllPRs(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	t.Setenv("SEMREL_TAG", "v1.3.0")
	t.Setenv("GITHUB_REPOSITORY", "owner/repo")

	gitClient := &mockGitClient{
		findTagByNameTag: newTestTag("v1.3.0", "sha-released"),
		previousTag:      nil,
		commitsBetween: []git.Commit{
			{SHA: "sha1", ShortSHA: "sha1"},
			{SHA: "sha2", ShortSHA: "sha2"},
		},
	}
	githubClient := &mockGitHubClient{
		prsPerSHA: map[string][]github.PR{
			"sha1": {{Number: 1, URL: "https://github.com/owner/repo/pull/1"}},
			"sha2": {{Number: 2, URL: "https://github.com/owner/repo/pull/2"}},
		},
		commentExists: false,
	}

	root := Root(gitClient, githubClient, logger)
	root.SetArgs([]string{"notify"})

	err := root.ExecuteContext(context.Background())
	if err != nil {
		t.Errorf("notify should not error, got: %v", err)
	}
	if githubClient.commentPostCount != 2 {
		t.Errorf("expected 2 comments posted, got %d", githubClient.commentPostCount)
	}
}

func TestNotifyDeduplicatesPRs(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	t.Setenv("SEMREL_TAG", "v1.3.0")
	t.Setenv("GITHUB_REPOSITORY", "owner/repo")

	gitClient := &mockGitClient{
		findTagByNameTag: newTestTag("v1.3.0", "sha-released"),
		previousTag:      nil,
		commitsBetween: []git.Commit{
			{SHA: "sha1", ShortSHA: "sha1"},
			{SHA: "sha2", ShortSHA: "sha2"},
		},
	}
	pr1 := github.PR{Number: 1, URL: "https://github.com/owner/repo/pull/1"}
	githubClient := &mockGitHubClient{
		prsPerSHA: map[string][]github.PR{
			"sha1": {pr1},
			"sha2": {pr1}, // same PR for both commits
		},
		commentExists: false,
	}

	root := Root(gitClient, githubClient, logger)
	root.SetArgs([]string{"notify"})

	err := root.ExecuteContext(context.Background())
	if err != nil {
		t.Errorf("notify should not error, got: %v", err)
	}
	if githubClient.commentPostCount != 1 {
		t.Errorf("expected 1 comment posted (deduplicated), got %d", githubClient.commentPostCount)
	}
}

func TestNotifyIdempotent(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	t.Setenv("SEMREL_TAG", "v1.3.0")
	t.Setenv("GITHUB_REPOSITORY", "owner/repo")

	gitClient := &mockGitClient{
		findTagByNameTag: newTestTag("v1.3.0", "sha-released"),
		previousTag:      nil,
		commitsBetween: []git.Commit{
			{SHA: "sha1", ShortSHA: "sha1"},
		},
	}
	githubClient := &mockGitHubClient{
		prsPerSHA: map[string][]github.PR{
			"sha1": {{Number: 1, URL: "https://github.com/owner/repo/pull/1"}},
		},
		commentExists: true, // marker already present
	}

	root := Root(gitClient, githubClient, logger)
	root.SetArgs([]string{"notify"})

	err := root.ExecuteContext(context.Background())
	if err != nil {
		t.Errorf("idempotent notify should not error, got: %v", err)
	}
	if githubClient.commentPostCount != 0 {
		t.Errorf("expected 0 comments posted (already exists), got %d", githubClient.commentPostCount)
	}
}

func TestNotifySkipsCommitsWithNoPRs(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	t.Setenv("SEMREL_TAG", "v1.3.0")
	t.Setenv("GITHUB_REPOSITORY", "owner/repo")

	gitClient := &mockGitClient{
		findTagByNameTag: newTestTag("v1.3.0", "sha-released"),
		previousTag:      nil,
		commitsBetween: []git.Commit{
			{SHA: "sha-a", ShortSHA: "sha-a"},
			{SHA: "sha-b", ShortSHA: "sha-b"},
		},
	}
	githubClient := &mockGitHubClient{
		prsPerSHA: map[string][]github.PR{
			"sha-a": {{Number: 1, URL: "https://github.com/owner/repo/pull/1"}},
			// sha-b returns empty (no PR)
		},
		commentExists: false,
	}

	root := Root(gitClient, githubClient, logger)
	root.SetArgs([]string{"notify"})

	err := root.ExecuteContext(context.Background())
	if err != nil {
		t.Errorf("notify should not error when some commits have no PRs, got: %v", err)
	}
	if githubClient.commentPostCount != 1 {
		t.Errorf("expected 1 comment posted (only PR from sha-a), got %d", githubClient.commentPostCount)
	}
}

func TestNotesGeneration(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	t.Setenv("GITHUB_REPOSITORY", "owner/repo")

	gitClient := &mockGitClient{
		latestTag: newTestTag("v0.0.1", "abc123"),
		commits: []git.Commit{
			{
				SHA:      "abc123def456",
				ShortSHA: "abc123d",
				Message:  "feat: add feature",
			},
		},
	}

	githubClient := &mockGitHubClient{
		prs: []github.PR{
			{
				Number: 42,
				URL:    "https://github.com/owner/repo/pull/42",
				Title:  "Add feature",
			},
		},
	}

	cmd := cmdNotes(gitClient, githubClient, logger)
	cmd.SetArgs([]string{})

	// Capture stdout
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := cmd.ExecuteContext(context.Background())

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	if err != nil {
		t.Errorf("notes should not error, got: %v", err)
	}

	if output == "" {
		t.Error("notes should produce output")
	}
}

func TestRootCommand(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	gitClient := &mockGitClient{}
	githubClient := &mockGitHubClient{}

	cmd := Root(gitClient, githubClient, logger)

	if cmd == nil {
		t.Fatal("Root should return a command")
	}

	if cmd.Use != "semrel" {
		t.Errorf("Root command should be 'semrel', got: %s", cmd.Use)
	}

	// Check that subcommands are registered
	subcommands := []string{"lint", "release", "notify", "notes"}
	for _, subcmd := range subcommands {
		found := false
		for _, cmd := range cmd.Commands() {
			if cmd.Name() == subcmd {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Root should have subcommand: %s", subcmd)
		}
	}
}

func TestOutputReleaseFields(t *testing.T) {
	tmpfile, err := os.CreateTemp("", "semrel-test-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpfile.Name())
	tmpfile.Close()

	version := semver.Version{Major: 1, Minor: 2, Patch: 3}
	err = outputReleaseFields(tmpfile.Name(), version, "v", true)
	if err != nil {
		t.Errorf("outputReleaseFields should not error, got: %v", err)
	}

	content, err := os.ReadFile(tmpfile.Name())
	if err != nil {
		t.Fatal(err)
	}

	output := string(content)
	if !bytes.Contains([]byte(output), []byte("version=1.2.3")) {
		t.Errorf("output should contain version, got: %s", output)
	}
	if !bytes.Contains([]byte(output), []byte("tag=v1.2.3")) {
		t.Errorf("output should contain tag, got: %s", output)
	}
}

func TestOutputReleaseFields_TagPrefix(t *testing.T) {
	tmpfile, err := os.CreateTemp("", "semrel-test-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpfile.Name())
	tmpfile.Close()

	version := semver.Version{Major: 1, Minor: 2, Patch: 3}
	err = outputReleaseFields(tmpfile.Name(), version, "release-", true)
	if err != nil {
		t.Errorf("outputReleaseFields should not error, got: %v", err)
	}

	content, err := os.ReadFile(tmpfile.Name())
	if err != nil {
		t.Fatal(err)
	}

	got := string(content)
	if !bytes.Contains([]byte(got), []byte("tag=release-1.2.3")) {
		t.Errorf("output should contain tag=release-1.2.3, got: %s", got)
	}
}

func TestGenerateReleaseNotes(t *testing.T) {
	commits := []conventional.Commit{
		{
			Type:        conventional.TypeFeat,
			Description: "add feature",
			SHA:         "abc123",
		},
		{
			Type:        conventional.TypeFix,
			Description: "fix bug",
			SHA:         "def456",
		},
	}

	prMap := map[string]notes.PR{
		"abc123": {Number: 1, URL: "https://github.com/owner/repo/pull/1"},
		"def456": {Number: 2, URL: "https://github.com/owner/repo/pull/2"},
	}

	result := generateReleaseNotes(commits, prMap)
	if result == "" {
		t.Error("generateReleaseNotes should produce output")
	}

	if !bytes.Contains([]byte(result), []byte("add feature")) {
		t.Errorf("release notes should contain commit description, got: %s", result)
	}
}

// TestReleaseRung2TagExistsAtHead verifies the idempotent retry path:
// FindTagByName returns a tag whose targetSHA matches HEAD → release is created without re-tagging.
func TestReleaseRung2TagExistsAtHead(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	t.Setenv("GITHUB_REPOSITORY", "owner/repo")

	headSHA := "aabbccddeeff00112233445566778899aabbccdd"

	// Simulate: latestTag is v0.0.1, HEAD has one new feat commit → next version is v0.0.2.
	// FindTagByName("v0.0.2") returns a tag pointing at headSHA (previous interrupted run).
	gitClient := &mockGitClient{
		latestTag: newTestTag("v0.0.1", "oldsha1"),
		commits: []git.Commit{
			{
				SHA:      headSHA,
				ShortSHA: headSHA[:7],
				Message:  "feat: new feature",
			},
		},
		// FindTagByName returns a tag at headSHA — simulates interrupted run
		findTagByNameTag: git.NewTag("v0.0.2", "tagobjectsha", headSHA),
	}

	githubClient := &mockGitHubClient{
		releaseByTagErr: github.ErrNotFound, // Rung 1: no release yet
	}

	cmd := cmdRelease(gitClient, githubClient, logger)
	// dry-run so CreateRelease mock path is skipped (we just check no error)
	cmd.SetArgs([]string{"--dry-run"})

	err := cmd.ExecuteContext(context.Background())
	if err != nil {
		t.Errorf("Rung 2 retry path should not error, got: %v", err)
	}
}

// TestReleaseRung2TagDoesNotExist verifies the normal path:
// FindTagByName returns nil → falls through to full release (Rung 3).
func TestReleaseRung2TagDoesNotExist(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	t.Setenv("GITHUB_REPOSITORY", "owner/repo")

	gitClient := &mockGitClient{
		latestTag: newTestTag("v0.0.1", "oldsha1"),
		commits: []git.Commit{
			{
				SHA:      "aabbccddeeff00112233445566778899aabbccdd",
				ShortSHA: "aabbccd",
				Message:  "feat: new feature",
			},
		},
		findTagByNameTag: nil, // tag does not exist yet
	}

	githubClient := &mockGitHubClient{
		releaseByTagErr: github.ErrNotFound,
	}

	cmd := cmdRelease(gitClient, githubClient, logger)
	cmd.SetArgs([]string{"--dry-run"})

	err := cmd.ExecuteContext(context.Background())
	if err != nil {
		t.Errorf("normal release path (Rung 3) should not error, got: %v", err)
	}
}

func TestCountByType(t *testing.T) {
	commits := []conventional.Commit{
		{Type: conventional.TypeFeat},
		{Type: conventional.TypeFeat},
		{Type: conventional.TypeFix},
		{Type: conventional.TypeChore},
	}

	if got := countByType(commits, conventional.TypeFeat); got != 2 {
		t.Errorf("countByType feat: want 2, got %d", got)
	}
	if got := countByType(commits, conventional.TypeFix); got != 1 {
		t.Errorf("countByType fix: want 1, got %d", got)
	}
	if got := countByType(commits, conventional.TypeDocs); got != 0 {
		t.Errorf("countByType docs: want 0, got %d", got)
	}
}

func TestCountBreaking(t *testing.T) {
	commits := []conventional.Commit{
		{Type: conventional.TypeFeat, Breaking: true},
		{Type: conventional.TypeFix, Breaking: false},
		{Type: conventional.TypeFeat, Breaking: true},
	}

	if got := countBreaking(commits); got != 2 {
		t.Errorf("countBreaking: want 2, got %d", got)
	}
}

func TestFindTriggerCommit(t *testing.T) {
	commits := []conventional.Commit{
		{Type: conventional.TypeChore, Breaking: false, SHA: "aaa"},
		{Type: conventional.TypeFeat, Breaking: false, SHA: "bbb"},
		{Type: conventional.TypeFix, Breaking: false, SHA: "ccc"},
		{Type: conventional.TypeFeat, Breaking: true, SHA: "ddd"},
	}

	// BumpMajor → first breaking commit
	trigger := findTriggerCommit(commits, semver.BumpMajor)
	if trigger == nil || trigger.SHA != "ddd" {
		t.Errorf("BumpMajor trigger: want sha=ddd, got %v", trigger)
	}

	// BumpMinor → first feat commit
	trigger = findTriggerCommit(commits, semver.BumpMinor)
	if trigger == nil || trigger.SHA != "bbb" {
		t.Errorf("BumpMinor trigger: want sha=bbb, got %v", trigger)
	}

	// BumpPatch → first fix commit
	trigger = findTriggerCommit(commits, semver.BumpPatch)
	if trigger == nil || trigger.SHA != "ccc" {
		t.Errorf("BumpPatch trigger: want sha=ccc, got %v", trigger)
	}

	// BumpNone → nil
	trigger = findTriggerCommit(commits, semver.BumpNone)
	if trigger != nil {
		t.Errorf("BumpNone trigger: want nil, got %v", trigger)
	}
}

func TestGithubPRMapToNotesPRMap(t *testing.T) {
	input := map[string]github.PR{
		"sha1": {Number: 1, URL: "https://github.com/owner/repo/pull/1", Title: "feat: one"},
		"sha2": {Number: 2, URL: "https://github.com/owner/repo/pull/2", Title: "fix: two"},
	}

	out := githubPRMapToNotesPRMap(input)
	if len(out) != 2 {
		t.Errorf("expected 2 entries, got %d", len(out))
	}
	if out["sha1"].Number != 1 || out["sha1"].URL != input["sha1"].URL {
		t.Errorf("sha1 entry mismatch: %+v", out["sha1"])
	}
	if out["sha2"].Number != 2 {
		t.Errorf("sha2 number mismatch: %+v", out["sha2"])
	}
}

func TestBumpTypeString(t *testing.T) {
	cases := []struct {
		bump semver.BumpType
		want string
	}{
		{semver.BumpMajor, "major"},
		{semver.BumpMinor, "minor"},
		{semver.BumpPatch, "patch"},
		{semver.BumpNone, "none"},
	}
	for _, tc := range cases {
		if got := tc.bump.String(); got != tc.want {
			t.Errorf("BumpType(%d).String() = %q, want %q", tc.bump, got, tc.want)
		}
	}
}

// ---------- New config-wiring tests ----------

// TestCmdRelease_BranchGuard_NotInList verifies that with --dry-run, the branch
// guard is skipped entirely so PR branches can preview the next version.
// A feat commit triggers a minor bump (0.0.0 → 0.1.0), proving the branch
// guard was skipped — not merely that BumpNone returned released=false.
func TestCmdRelease_BranchGuard_NotInList(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	t.Setenv("GITHUB_REPOSITORY", "owner/repo")
	t.Setenv("GITHUB_REF_NAME", "feature/foo")

	// Write a temp .semrelrc.yml with release-branches: [main]
	tmpDir := t.TempDir()
	rcPath := tmpDir + "/.semrelrc.yml"
	if err := os.WriteFile(rcPath, []byte("release-branches: [main]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("INPUT_WORKING_DIRECTORY", tmpDir)

	// Output file so outputReleaseFields doesn't fail
	outFile, err := os.CreateTemp("", "semrel-out-")
	require.NoError(t, err)
	defer os.Remove(outFile.Name())
	outFile.Close()
	t.Setenv("GITHUB_OUTPUT", outFile.Name())

	gitClient := &mockGitClient{
		latestTag: nil, // bootstrap: no tags
		commits: []git.Commit{
			{SHA: "abc123def456", ShortSHA: "abc123d", Message: "feat: new feature"},
		},
	}
	githubClient := &mockGitHubClient{
		releaseByTagErr: github.ErrNotFound,
	}

	cmd := cmdRelease(gitClient, githubClient, logger)
	cmd.SetArgs([]string{"--dry-run"})

	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Errorf("dry-run on non-release branch should not error, got: %v", err)
	}

	// Confirm no tag or release was created
	if gitClient.createdTag != nil {
		t.Error("no tag should have been created for dry-run")
	}

	// Output should contain a computed version (proving the branch guard was
	// skipped and version computation happened) with released=false.
	content, err := os.ReadFile(outFile.Name())
	require.NoError(t, err)
	require.Contains(t, string(content), "version=0.1.0",
		"branch guard must be skipped so version is computed (bootstrap 0.0.0 + minor)")
	require.Contains(t, string(content), "released=false")
}

// TestCmdRelease_BranchGuard_Allowed verifies that a branch in ReleaseBranches proceeds normally.
func TestCmdRelease_BranchGuard_Allowed(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	t.Setenv("GITHUB_REPOSITORY", "owner/repo")
	t.Setenv("GITHUB_REF_NAME", "main")

	tmpDir := t.TempDir()
	rcPath := tmpDir + "/.semrelrc.yml"
	if err := os.WriteFile(rcPath, []byte("release-branches: [main, master]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("INPUT_WORKING_DIRECTORY", tmpDir)

	outFile, err := os.CreateTemp("", "semrel-out-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(outFile.Name())
	outFile.Close()
	t.Setenv("GITHUB_OUTPUT", outFile.Name())

	gitClient := &mockGitClient{
		latestTag: nil,
		commits: []git.Commit{
			{SHA: "abc123def456", ShortSHA: "abc123d", Message: "feat: new feature"},
		},
	}
	githubClient := &mockGitHubClient{
		releaseByTagErr: github.ErrNotFound,
	}

	cmd := cmdRelease(gitClient, githubClient, logger)
	cmd.SetArgs([]string{"--dry-run"})

	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Errorf("allowed branch should proceed without error, got: %v", err)
	}
}

// TestCmdRelease_BranchGuard_GlobMatch verifies path.Match glob patterns work.
func TestCmdRelease_BranchGuard_GlobMatch(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	t.Setenv("GITHUB_REPOSITORY", "owner/repo")
	t.Setenv("GITHUB_REF_NAME", "releases/v2")

	tmpDir := t.TempDir()
	rcPath := tmpDir + "/.semrelrc.yml"
	if err := os.WriteFile(rcPath, []byte("release-branches: [\"releases/*\"]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("INPUT_WORKING_DIRECTORY", tmpDir)

	outFile, err := os.CreateTemp("", "semrel-out-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(outFile.Name())
	outFile.Close()
	t.Setenv("GITHUB_OUTPUT", outFile.Name())

	gitClient := &mockGitClient{
		latestTag: nil,
		commits: []git.Commit{
			{SHA: "abc123def456", ShortSHA: "abc123d", Message: "feat: glob match"},
		},
	}
	githubClient := &mockGitHubClient{
		releaseByTagErr: github.ErrNotFound,
	}

	cmd := cmdRelease(gitClient, githubClient, logger)
	cmd.SetArgs([]string{"--dry-run"})

	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Errorf("glob-matched branch should be allowed, got: %v", err)
	}
}

// TestCmdRelease_BreakingChange_Injection verifies that a commit with Breaking=true
// causes "breaking-change" to be injected into commitTypes → BumpMajor.
func TestCmdRelease_BreakingChange_Injection(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	t.Setenv("GITHUB_REPOSITORY", "owner/repo")
	t.Setenv("GITHUB_REF_NAME", "main")

	outFile, err := os.CreateTemp("", "semrel-out-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(outFile.Name())
	outFile.Close()
	t.Setenv("GITHUB_OUTPUT", outFile.Name())

	gitClient := &mockGitClient{
		latestTag: newTestTag("v1.0.0", "oldsha"),
		commits: []git.Commit{
			{SHA: "aabbcc112233", ShortSHA: "aabbcc1", Message: "feat!: breaking API change\n\nBREAKING CHANGE: removed endpoint"},
		},
	}
	githubClient := &mockGitHubClient{
		releaseByTagErr: github.ErrNotFound,
	}

	cmd := cmdRelease(gitClient, githubClient, logger)
	cmd.SetArgs([]string{"--dry-run"})

	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Errorf("breaking change commit should not error, got: %v", err)
	}

	// The output file should contain version=2.0.0 (major bump from v1.0.0)
	content, err := os.ReadFile(outFile.Name())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(content, []byte("version=2.0.0")) {
		t.Errorf("expected major bump to 2.0.0, got: %s", content)
	}
}

// TestCmdRelease_CustomBumpRules verifies that custom BumpRules override the defaults.
func TestCmdRelease_CustomBumpRules(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	t.Setenv("GITHUB_REPOSITORY", "owner/repo")
	t.Setenv("GITHUB_REF_NAME", "main")

	tmpDir := t.TempDir()
	rcPath := tmpDir + "/.semrelrc.yml"
	// docs→minor; docs commit should produce a minor bump
	if err := os.WriteFile(rcPath, []byte("bump-rules:\n  docs: minor\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("INPUT_WORKING_DIRECTORY", tmpDir)

	outFile, err := os.CreateTemp("", "semrel-out-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(outFile.Name())
	outFile.Close()
	t.Setenv("GITHUB_OUTPUT", outFile.Name())

	gitClient := &mockGitClient{
		latestTag: newTestTag("v1.0.0", "oldsha"),
		commits: []git.Commit{
			{SHA: "aabbcc112233", ShortSHA: "aabbcc1", Message: "docs: update readme"},
		},
	}
	githubClient := &mockGitHubClient{
		releaseByTagErr: github.ErrNotFound,
	}

	cmd := cmdRelease(gitClient, githubClient, logger)
	cmd.SetArgs([]string{"--dry-run"})

	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Errorf("custom bump rules should not error, got: %v", err)
	}

	content, err := os.ReadFile(outFile.Name())
	if err != nil {
		t.Fatal(err)
	}
	// docs→minor from v1.0.0 → v1.1.0
	if !bytes.Contains(content, []byte("version=1.1.0")) {
		t.Errorf("expected minor bump to 1.1.0 with custom docs rule, got: %s", content)
	}
}

// TestCmdRelease_Bootstrap_CustomInitialVersion verifies InitialVersion="2.0.0"
// with a feat commit → nextVersion=2.1.0.
func TestCmdRelease_Bootstrap_CustomInitialVersion(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	t.Setenv("GITHUB_REPOSITORY", "owner/repo")
	t.Setenv("GITHUB_REF_NAME", "main")

	tmpDir := t.TempDir()
	rcPath := tmpDir + "/.semrelrc.yml"
	if err := os.WriteFile(rcPath, []byte("initial-version: \"2.0.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("INPUT_WORKING_DIRECTORY", tmpDir)

	outFile, err := os.CreateTemp("", "semrel-out-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(outFile.Name())
	outFile.Close()
	t.Setenv("GITHUB_OUTPUT", outFile.Name())

	gitClient := &mockGitClient{
		latestTag: nil, // bootstrap: no tags
		commits: []git.Commit{
			{SHA: "aabb112233cc", ShortSHA: "aabb112", Message: "feat: initial feature"},
		},
	}
	githubClient := &mockGitHubClient{
		releaseByTagErr: github.ErrNotFound,
	}

	cmd := cmdRelease(gitClient, githubClient, logger)
	cmd.SetArgs([]string{"--dry-run"})

	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Errorf("bootstrap with custom initial-version should not error, got: %v", err)
	}

	content, err := os.ReadFile(outFile.Name())
	if err != nil {
		t.Fatal(err)
	}
	// feat on 2.0.0 → 2.1.0
	if !bytes.Contains(content, []byte("version=2.1.0")) {
		t.Errorf("expected 2.1.0 with initial-version=2.0.0 + feat, got: %s", content)
	}
}

// TestCmdRelease_Bootstrap_DefaultInitialVersion verifies that with no config,
// InitialVersion="0.0.0" + fix → nextVersion=0.0.1.
func TestCmdRelease_Bootstrap_DefaultInitialVersion(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	t.Setenv("GITHUB_REPOSITORY", "owner/repo")
	t.Setenv("GITHUB_REF_NAME", "main")

	outFile, err := os.CreateTemp("", "semrel-out-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(outFile.Name())
	outFile.Close()
	t.Setenv("GITHUB_OUTPUT", outFile.Name())

	gitClient := &mockGitClient{
		latestTag: nil, // bootstrap: no tags
		commits: []git.Commit{
			{SHA: "aabb112233cc", ShortSHA: "aabb112", Message: "fix: initial fix"},
		},
	}
	githubClient := &mockGitHubClient{
		releaseByTagErr: github.ErrNotFound,
	}

	cmd := cmdRelease(gitClient, githubClient, logger)
	cmd.SetArgs([]string{"--dry-run"})

	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Errorf("default bootstrap should not error, got: %v", err)
	}

	content, err := os.ReadFile(outFile.Name())
	if err != nil {
		t.Fatal(err)
	}
	// fix on 0.0.0 → 0.0.1
	if !bytes.Contains(content, []byte("version=0.0.1")) {
		t.Errorf("expected 0.0.1 with default initial-version + fix, got: %s", content)
	}
}

// TestCmdRelease_TagPrefix_Custom verifies that a custom TagPrefix is used in version tag formatting.
func TestCmdRelease_TagPrefix_Custom(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	t.Setenv("GITHUB_REPOSITORY", "owner/repo")
	t.Setenv("GITHUB_REF_NAME", "main")

	tmpDir := t.TempDir()
	rcPath := tmpDir + "/.semrelrc.yml"
	if err := os.WriteFile(rcPath, []byte("tag-prefix: \"release-\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("INPUT_WORKING_DIRECTORY", tmpDir)

	outFile, err := os.CreateTemp("", "semrel-out-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(outFile.Name())
	outFile.Close()
	t.Setenv("GITHUB_OUTPUT", outFile.Name())

	gitClient := &mockGitClient{
		latestTag: newTestTag("release-1.0.0", "oldsha"),
		commits: []git.Commit{
			{SHA: "aabb112233cc", ShortSHA: "aabb112", Message: "feat: new feature"},
		},
	}
	githubClient := &mockGitHubClient{
		releaseByTagErr: github.ErrNotFound,
	}

	cmd := cmdRelease(gitClient, githubClient, logger)
	cmd.SetArgs([]string{"--dry-run"})

	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Errorf("custom tag prefix should not error, got: %v", err)
	}

	content, err := os.ReadFile(outFile.Name())
	if err != nil {
		t.Fatal(err)
	}
	// feat on release-1.0.0 → release-1.1.0
	if !bytes.Contains(content, []byte("tag=release-1.1.0")) {
		t.Errorf("expected tag=release-1.1.0, got: %s", content)
	}
}

// TestCmdRelease_InvalidInitialVersion verifies that an invalid initial-version
// returns an error before any git/GitHub I/O.
func TestCmdRelease_InvalidInitialVersion(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	t.Setenv("GITHUB_REPOSITORY", "owner/repo")

	tmpDir := t.TempDir()
	rcPath := tmpDir + "/.semrelrc.yml"
	if err := os.WriteFile(rcPath, []byte("initial-version: \"not-semver\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("INPUT_WORKING_DIRECTORY", tmpDir)

	gitClient := &mockGitClient{}
	githubClient := &mockGitHubClient{}

	cmd := cmdRelease(gitClient, githubClient, logger)
	cmd.SetArgs([]string{"--dry-run"})

	err := cmd.ExecuteContext(context.Background())
	if err == nil {
		t.Error("invalid initial-version should return an error")
	}

	// No git I/O should have occurred
	if gitClient.latestTag != nil || gitClient.pushedTag != "" {
		t.Error("no git I/O should happen when initial-version is invalid")
	}
}

// TestRelease_BumpNone_NoRelease verifies that when latestTag != nil and all commits
// are chore-only (bump=BumpNone), the short-circuit fires, no tag is created,
// and released=false is written to GITHUB_OUTPUT.
func TestRelease_BumpNone_NoRelease(t *testing.T) {
	t.Setenv("GITHUB_REPOSITORY", "owner/repo")
	t.Setenv("GITHUB_REF_NAME", "main") // branch guard must pass so BumpNone short-circuit is exercised
	outFile, err := os.CreateTemp(t.TempDir(), "gh-output")
	require.NoError(t, err)
	t.Setenv("GITHUB_OUTPUT", outFile.Name())

	gitClient := &mockGitClient{
		latestTag: git.NewTag("v1.2.3", "abc1234abc1234abc1234abc1234abc1234abc1234", "abc1234abc1234abc1234abc1234abc1234abc1234"),
		commits:   []git.Commit{{SHA: "aabbccdd", ShortSHA: "aabbccd", Message: "chore: update deps"}},
	}
	githubClient := &mockGitHubClient{
		releaseByTagErr: github.ErrNotFound, // no release exists
	}

	root := Root(gitClient, githubClient, slog.New(slog.NewTextHandler(os.Stderr, nil)))
	root.SetArgs([]string{"release"})
	err = root.ExecuteContext(context.Background())
	require.NoError(t, err, "BumpNone short-circuit should return nil")

	// No tag should have been created
	require.Nil(t, gitClient.createdTag, "no tag should be created for BumpNone")

	// GITHUB_OUTPUT should contain released=false
	content, err := os.ReadFile(outFile.Name())
	require.NoError(t, err)
	require.Contains(t, string(content), "released=false")
}

// TestDryRun_NonReleaseBranch_ComputesVersion verifies that --dry-run on a
// non-release branch skips the branch guard and computes the projected next
// version from conventional commits.
func TestDryRun_NonReleaseBranch_ComputesVersion(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	t.Setenv("GITHUB_REPOSITORY", "owner/repo")
	t.Setenv("GITHUB_REF_NAME", "feature/test-dry-run")

	tmpDir := t.TempDir()
	rcPath := tmpDir + "/.semrelrc.yml"
	if err := os.WriteFile(rcPath, []byte("release-branches: [main]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("INPUT_WORKING_DIRECTORY", tmpDir)

	outFile, err := os.CreateTemp("", "semrel-out-")
	require.NoError(t, err)
	defer os.Remove(outFile.Name())
	outFile.Close()
	t.Setenv("GITHUB_OUTPUT", outFile.Name())

	gitClient := &mockGitClient{
		latestTag: nil, // bootstrap: no tags
		commits: []git.Commit{
			{SHA: "aabb112233cc", ShortSHA: "aabb112", Message: "feat: new feature"},
		},
	}
	githubClient := &mockGitHubClient{
		releaseByTagErr: github.ErrNotFound,
	}

	cmd := cmdRelease(gitClient, githubClient, logger)
	cmd.SetArgs([]string{"--dry-run"})

	err = cmd.ExecuteContext(context.Background())
	require.NoError(t, err)
	require.Nil(t, gitClient.createdTag, "no tag should be created in dry-run")
	require.Nil(t, githubClient.createdRelease, "no release should be created in dry-run")

	content, err := os.ReadFile(outFile.Name())
	require.NoError(t, err)
	require.Contains(t, string(content), "version=0.1.0", "next minor version should be computed")
	require.Contains(t, string(content), "released=false")
}

// TestDryRun_NonReleaseBranch_BumpNone_OutputsCurrentVersion verifies that
// --dry-run with BumpNone on a non-release branch outputs the current version
// with released=false.
func TestDryRun_NonReleaseBranch_BumpNone_OutputsCurrentVersion(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	t.Setenv("GITHUB_REPOSITORY", "owner/repo")
	t.Setenv("GITHUB_REF_NAME", "feature/no-bump")

	tmpDir := t.TempDir()
	rcPath := tmpDir + "/.semrelrc.yml"
	if err := os.WriteFile(rcPath, []byte("release-branches: [main]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("INPUT_WORKING_DIRECTORY", tmpDir)

	outFile, err := os.CreateTemp("", "semrel-out-")
	require.NoError(t, err)
	defer os.Remove(outFile.Name())
	outFile.Close()
	t.Setenv("GITHUB_OUTPUT", outFile.Name())

	gitClient := &mockGitClient{
		latestTag: git.NewTag("v1.2.3", "abc1234abc1234abc1234abc1234abc1234abc1234", "abc1234abc1234abc1234abc1234abc1234abc1234"),
		commits:   []git.Commit{{SHA: "aabbccdd", ShortSHA: "aabbccd", Message: "chore: update deps"}},
	}
	githubClient := &mockGitHubClient{
		releaseByTagErr: github.ErrNotFound,
	}

	cmd := cmdRelease(gitClient, githubClient, logger)
	cmd.SetArgs([]string{"--dry-run"})

	err = cmd.ExecuteContext(context.Background())
	require.NoError(t, err)
	require.Nil(t, gitClient.createdTag, "no tag should be created in dry-run")
	require.Nil(t, githubClient.createdRelease, "no release should be created in dry-run")

	content, err := os.ReadFile(outFile.Name())
	require.NoError(t, err)
	require.Contains(t, string(content), "version=1.2.3", "current version should be output for BumpNone")
	require.Contains(t, string(content), "released=false")
}

// TestDryRun_NoWrites verifies that --dry-run performs zero write operations:
// no tag creation, no tag push, no GitHub release creation.
func TestDryRun_NoWrites(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	t.Setenv("GITHUB_REPOSITORY", "owner/repo")
	t.Setenv("GITHUB_REF_NAME", "feature/preview")

	tmpDir := t.TempDir()
	rcPath := tmpDir + "/.semrelrc.yml"
	if err := os.WriteFile(rcPath, []byte("release-branches: [main]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("INPUT_WORKING_DIRECTORY", tmpDir)

	outFile, err := os.CreateTemp("", "semrel-out-")
	require.NoError(t, err)
	defer os.Remove(outFile.Name())
	outFile.Close()
	t.Setenv("GITHUB_OUTPUT", outFile.Name())

	gitClient := &mockGitClient{
		latestTag: nil,
		commits: []git.Commit{
			{SHA: "aabb112233cc", ShortSHA: "aabb112", Message: "feat: new feature"},
		},
	}
	githubClient := &mockGitHubClient{
		releaseByTagErr: github.ErrNotFound,
	}

	cmd := cmdRelease(gitClient, githubClient, logger)
	cmd.SetArgs([]string{"--dry-run"})

	err = cmd.ExecuteContext(context.Background())
	require.NoError(t, err)
	require.Nil(t, gitClient.createdTag, "no tag should be created in dry-run")
	require.Empty(t, gitClient.pushedTag, "no tag should be pushed in dry-run")
	require.Nil(t, githubClient.createdRelease, "no release should be created in dry-run")

	content, err := os.ReadFile(outFile.Name())
	require.NoError(t, err)
	require.Contains(t, string(content), "released=false")
}
