//go:build integration

package main

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-git/go-billy/v5/memfs"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/storer"
	"github.com/go-git/go-git/v5/storage/memory"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	gitpkg "github.com/nrkno/semrel/internal/git"
	githubpkg "github.com/nrkno/semrel/internal/github"
	"github.com/nrkno/semrel/internal/cli"
)

// setupInMemoryRepo creates an in-memory git repository with initial commits.
// Returns a *git.Repository (from go-git) for use in testGitClient.
func setupInMemoryRepo(t *testing.T, commits []struct {
	message string
	time    time.Time
}) *git.Repository {
	// Create in-memory storage and filesystem
	storage := memory.NewStorage()
	fs := memfs.New()

	// Initialize repository
	repo, err := git.Init(storage, fs)
	require.NoError(t, err, "failed to init in-memory repo")

	// Configure user
	cfg, err := repo.Config()
	require.NoError(t, err)
	cfg.User.Name = "Test User"
	cfg.User.Email = "test@example.com"
	err = repo.SetConfig(cfg)
	require.NoError(t, err)

	// Create commits
	wt, err := repo.Worktree()
	require.NoError(t, err)

	for i, commit := range commits {
		// Create a file for each commit
		filename := fmt.Sprintf("file%d.txt", i)
		f, err := fs.Create(filename)
		require.NoError(t, err)
		_, err = f.Write([]byte(fmt.Sprintf("content %d\n", i)))
		require.NoError(t, err)
		f.Close()

		// Add file
		_, err = wt.Add(filename)
		require.NoError(t, err)

		// Commit
		_, err = wt.Commit(commit.message, &git.CommitOptions{
			Author: &object.Signature{
				Name:  "Test User",
				Email: "test@example.com",
				When:  commit.time,
			},
		})
		require.NoError(t, err)
	}

	return repo
}

// mockGitHubServer creates an httptest server that mocks GitHub API responses.
func mockGitHubServer(t *testing.T) (*httptest.Server, *githubpkg.Client) {
	releases := make(map[string]*githubpkg.Release)
	comments := make(map[int][]string) // prNumber -> comments

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Mock GetReleaseByTag
		if r.Method == http.MethodGet && bytes.Contains([]byte(r.URL.Path), []byte("/releases/tags/")) {
			tag := r.URL.Query().Get("tag")
			if tag == "" {
				// Extract from path: /repos/owner/repo/releases/tags/v1.0.0
				parts := bytes.Split([]byte(r.URL.Path), []byte("/"))
				if len(parts) > 0 {
					tag = string(parts[len(parts)-1])
				}
			}

			if rel, ok := releases[tag]; ok {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				fmt.Fprintf(w, `{"id":%d,"tag_name":"%s","body":"%s"}`, rel.ID, rel.TagName, rel.Body)
				return
			}

			w.WriteHeader(http.StatusNotFound)
			fmt.Fprint(w, `{"message":"Not Found"}`)
			return
		}

		// Mock CreateRelease
		if r.Method == http.MethodPost && bytes.Contains([]byte(r.URL.Path), []byte("/releases")) {
			tag := "v0.1.0" // default
			releases[tag] = &githubpkg.Release{
				ID:      1,
				TagName: tag,
				Body:    "Release notes",
			}

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			fmt.Fprintf(w, `{"id":1,"tag_name":"%s","body":"Release notes"}`, tag)
			return
		}

		// Mock ListPRsForCommit
		if r.Method == http.MethodGet && bytes.Contains([]byte(r.URL.Path), []byte("/search/issues")) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, `{"items":[{"number":42,"html_url":"https://github.com/owner/repo/pull/42","title":"Test PR"}]}`)
			return
		}

		// Mock PostPRComment
		if r.Method == http.MethodPost && bytes.Contains([]byte(r.URL.Path), []byte("/issues/")) && bytes.Contains([]byte(r.URL.Path), []byte("/comments")) {
			// Extract PR number from path
			prNum := 42
			comments[prNum] = append(comments[prNum], "comment body")

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			fmt.Fprint(w, `{"id":1,"body":"comment body"}`)
			return
		}

		// Mock FindPRComment
		if r.Method == http.MethodGet && bytes.Contains([]byte(r.URL.Path), []byte("/issues/")) && bytes.Contains([]byte(r.URL.Path), []byte("/comments")) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, `{"items":[]}`)
			return
		}

		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, `{"message":"Not Found"}`)
	}))

	client, err := githubpkg.NewClient("test-token", server.URL)
	require.NoError(t, err)
	return server, client
}

// TestE2ELintValidCommits tests lint with valid conventional commits
func TestE2ELintValidCommits(t *testing.T) {
	// No t.Parallel(): t.Setenv is required to neutralize the CI runner's own
	// pull_request context, which would otherwise change the lint range.
	t.Setenv("GITHUB_EVENT_NAME", "")
	t.Setenv("GITHUB_BASE_REF", "")

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	// Setup in-memory repo with valid commits
	rawRepo := setupInMemoryRepo(t, []struct {
		message string
		time    time.Time
	}{
		{
			message: "feat: add new feature",
			time:    time.Now().Add(-2 * time.Hour),
		},
		{
			message: "fix: resolve bug",
			time:    time.Now().Add(-1 * time.Hour),
		},
	})

	// Create mock git client
	gitClient := &testGitClient{rawRepo: rawRepo}

	// Create root command
	root := cli.Root(gitClient, &testGitHubClient{}, logger)

	// Execute via root command
	var stdout, stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs([]string{"lint"})

	err := root.ExecuteContext(context.Background())
	assert.NoError(t, err, "lint with valid commits should succeed")
}

// TestE2ELintInvalidCommits tests lint with invalid commits
func TestE2ELintInvalidCommits(t *testing.T) {
	// No t.Parallel(): see TestE2ELintValidCommits.
	t.Setenv("GITHUB_EVENT_NAME", "")
	t.Setenv("GITHUB_BASE_REF", "")

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	// Setup in-memory repo with invalid commits
	rawRepo := setupInMemoryRepo(t, []struct {
		message string
		time    time.Time
	}{
		{
			message: "invalid commit message without type",
			time:    time.Now(),
		},
	})

	gitClient := &testGitClient{rawRepo: rawRepo}

	root := cli.Root(gitClient, &testGitHubClient{}, logger)

	var stdout, stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs([]string{"lint"})

	err := root.ExecuteContext(context.Background())
	assert.Error(t, err, "lint with invalid commits should fail")
}

func TestE2ELintPushUsesEventPayloadCommits(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	rawRepo := setupInMemoryRepo(t, []struct {
		message string
		time    time.Time
	}{
		{
			message: "historical non conventional commit",
			time:    time.Now().Add(-2 * time.Hour),
		},
	})

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

	gitClient := &testGitClient{rawRepo: rawRepo}
	root := cli.Root(gitClient, &testGitHubClient{}, logger)

	var stdout, stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs([]string{"lint"})

	err := root.ExecuteContext(context.Background())

	assert.NoError(t, err, "push lint should validate event payload commits, not historical repository commits")
}

// TestE2EReleaseNewVersion tests release creating a new version
func TestE2EReleaseNewVersion(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	// Setup environment
	t.Setenv("GITHUB_REPOSITORY", "owner/repo")
	t.Setenv("GITHUB_TOKEN", "test-token")

	// Setup in-memory repo with feature commit (bootstrap case)
	rawRepo := setupInMemoryRepo(t, []struct {
		message string
		time    time.Time
	}{
		{
			message: "feat: initial feature",
			time:    time.Now(),
		},
	})

	gitClient := &testGitClient{rawRepo: rawRepo}
	githubClient := &testGitHubClient{}

	root := cli.Root(gitClient, githubClient, logger)

	var stdout, stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs([]string{"release", "--dry-run"})

	err := root.ExecuteContext(context.Background())
	assert.NoError(t, err, "release should succeed")
}

// TestE2EReleaseIdempotent tests release idempotency
func TestE2EReleaseIdempotent(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	t.Setenv("GITHUB_REPOSITORY", "owner/repo")
	t.Setenv("GITHUB_TOKEN", "test-token")

	// Setup repo with existing tag
	rawRepo := setupInMemoryRepo(t, []struct {
		message string
		time    time.Time
	}{
		{
			message: "feat: initial feature",
			time:    time.Now(),
		},
	})

	// Create a tag
	head, err := rawRepo.Head()
	require.NoError(t, err)
	_, err = rawRepo.CreateTag("v0.1.0", head.Hash(), &git.CreateTagOptions{
		Tagger: &object.Signature{
			Name:  "Test User",
			Email: "test@example.com",
			When:  time.Now(),
		},
		Message: "Release v0.1.0",
	})
	require.NoError(t, err)

	gitClient := &testGitClient{rawRepo: rawRepo}
	githubClient := &testGitHubClient{
		releaseExists: true,
	}

	root := cli.Root(gitClient, githubClient, logger)

	var stdout, stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs([]string{"release"})

	err = root.ExecuteContext(context.Background())
	assert.NoError(t, err, "release idempotent should succeed")
}

// TestE2ENotifyNewComment tests notify posting a new comment on a PR
func TestE2ENotifyNewComment(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	t.Setenv("GITHUB_REPOSITORY", "owner/repo")
	t.Setenv("SEMREL_TAG", "v0.1.0")
	t.Setenv("SEMREL_RELEASE_URL", "https://github.com/owner/repo/releases/tag/v0.1.0")

	releasedTag := gitpkg.NewTag("v0.1.0", "abc1234abc1234abc1234abc1234abc1234abc12", "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef")
	gitClient := &testGitClient{
		tagsByName: map[string]*gitpkg.Tag{"v0.1.0": releasedTag},
		commitsForRange: []gitpkg.Commit{
			{SHA: "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef", ShortSHA: "deadbee", Message: "feat: add feature"},
		},
	}
	githubClient := &testGitHubClient{
		commentExists: false,
		prsForCommit:  []githubpkg.PR{{Number: 42, URL: "https://github.com/owner/repo/pull/42", Title: "Test PR"}},
	}

	root := cli.Root(gitClient, githubClient, logger)

	var stdout, stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs([]string{"notify"})

	err := root.ExecuteContext(context.Background())
	assert.NoError(t, err, "notify should succeed")
	assert.True(t, githubClient.commentPosted, "comment should be posted")
}

// TestE2ENotifyIdempotent tests notify with existing comment — must not double-post
func TestE2ENotifyIdempotent(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	t.Setenv("GITHUB_REPOSITORY", "owner/repo")
	t.Setenv("SEMREL_TAG", "v0.1.0")
	t.Setenv("SEMREL_RELEASE_URL", "https://github.com/owner/repo/releases/tag/v0.1.0")

	releasedTag := gitpkg.NewTag("v0.1.0", "abc1234abc1234abc1234abc1234abc1234abc12", "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef")
	gitClient := &testGitClient{
		tagsByName: map[string]*gitpkg.Tag{"v0.1.0": releasedTag},
		commitsForRange: []gitpkg.Commit{
			{SHA: "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef", ShortSHA: "deadbee", Message: "feat: add feature"},
		},
	}
	githubClient := &testGitHubClient{
		commentExists: true, // marker already on PR
		prsForCommit:  []githubpkg.PR{{Number: 42, URL: "https://github.com/owner/repo/pull/42", Title: "Test PR"}},
	}

	root := cli.Root(gitClient, githubClient, logger)

	var stdout, stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs([]string{"notify"})

	err := root.ExecuteContext(context.Background())
	assert.NoError(t, err, "notify idempotent should succeed")
	assert.False(t, githubClient.commentPosted, "comment should not be posted again")
}

// TestE2ENotesGeneration tests notes generation with PR links
func TestE2ENotesGeneration(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	t.Setenv("GITHUB_REPOSITORY", "owner/repo")

	rawRepo := setupInMemoryRepo(t, []struct {
		message string
		time    time.Time
	}{
		{
			message: "feat: add feature",
			time:    time.Now(),
		},
		{
			message: "fix: resolve bug",
			time:    time.Now().Add(-1 * time.Hour),
		},
	})

	gitClient := &testGitClient{rawRepo: rawRepo}
	githubClient := &testGitHubClient{
		prsForCommit: []githubpkg.PR{
			{Number: 42, URL: "https://github.com/owner/repo/pull/42", Title: "Add feature"},
		},
	}

	root := cli.Root(gitClient, githubClient, logger)

	var stdout, stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs([]string{"notes"})

	err := root.ExecuteContext(context.Background())
	assert.NoError(t, err, "notes should succeed")

	output := stdout.String()
	assert.NotEmpty(t, output, "notes should produce output")
}

// TestE2EFullWorkflow tests complete lint → release → notify → notes flow
func TestE2EFullWorkflow(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	t.Setenv("GITHUB_REPOSITORY", "owner/repo")
	t.Setenv("GITHUB_TOKEN", "test-token")
	t.Setenv("GITHUB_EVENT_NAME", "push")

	// Setup repo with valid commits
	rawRepo := setupInMemoryRepo(t, []struct {
		message string
		time    time.Time
	}{
		{
			message: "feat: add feature",
			time:    time.Now().Add(-2 * time.Hour),
		},
		{
			message: "fix: resolve bug",
			time:    time.Now().Add(-1 * time.Hour),
		},
	})

	gitClient := &testGitClient{rawRepo: rawRepo}
	githubClient := &testGitHubClient{}

	root := cli.Root(gitClient, githubClient, logger)

	// Step 1: Lint
	var stdout, stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs([]string{"lint"})

	err := root.ExecuteContext(context.Background())
	assert.NoError(t, err, "lint should succeed")

	// Step 2: Release
	root = cli.Root(gitClient, githubClient, logger)
	stdout.Reset()
	stderr.Reset()
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs([]string{"release", "--dry-run"})

	err = root.ExecuteContext(context.Background())
	assert.NoError(t, err, "release should succeed")

	// Step 3: Notes
	root = cli.Root(gitClient, githubClient, logger)
	stdout.Reset()
	stderr.Reset()
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs([]string{"notes"})

	err = root.ExecuteContext(context.Background())
	assert.NoError(t, err, "notes should succeed")

	output := stdout.String()
	assert.NotEmpty(t, output, "notes should produce output")
}

// testGitClient implements cli.GitClient for testing
type testGitClient struct {
	rawRepo        *git.Repository
	tagsByName     map[string]*gitpkg.Tag
	commitsForRange []gitpkg.Commit
}

func (c *testGitClient) FindLatestAnnotatedTag(tagPrefix string) (*gitpkg.Tag, error) {
	if c.rawRepo == nil {
		return nil, nil
	}

	tags, err := c.rawRepo.Tags()
	if err != nil {
		return nil, err
	}

	var latestTag *gitpkg.Tag
	var latestTime time.Time

	err = tags.ForEach(func(ref *plumbing.Reference) error {
		// Only process annotated tags
		obj, err := c.rawRepo.TagObject(ref.Hash())
		if err != nil {
			return nil
		}

		// Get the target commit to compare dates
		commit, err := c.rawRepo.CommitObject(obj.Target)
		if err != nil {
			return nil
		}

		if latestTag == nil || commit.Author.When.After(latestTime) {
			latestTime = commit.Author.When
			latestTag = &gitpkg.Tag{
				Name: ref.Name().Short(),
				SHA:  obj.Hash.String(),
			}
		}
		return nil
	})

	return latestTag, err
}

func (c *testGitClient) ListCommitsSinceTag(tag *gitpkg.Tag) ([]gitpkg.Commit, error) {
	if c.rawRepo == nil {
		return []gitpkg.Commit{}, nil
	}

	head, err := c.rawRepo.Head()
	if err != nil {
		return nil, err
	}

	iter, err := c.rawRepo.Log(&git.LogOptions{From: head.Hash()})
	if err != nil {
		return nil, err
	}
	defer iter.Close()

	var commits []gitpkg.Commit
	var stopHash plumbing.Hash
	if tag != nil {
		stopHash = plumbing.NewHash(tag.SHA)
	}

	err = iter.ForEach(func(c *object.Commit) error {
		// Skip the tag's target commit itself
		if tag != nil && c.Hash == stopHash {
			return storer.ErrStop
		}

		commits = append(commits, gitpkg.Commit{
			SHA:      c.Hash.String(),
			ShortSHA: c.Hash.String()[:7],
			Author:   c.Author.Name,
			Date:     c.Author.When,
			Message:  c.Message,
		})
		return nil
	})
	return commits, err
}

// ListCommitsSinceRef returns commits reachable from HEAD but not from refName
// (base..HEAD semantics), mirroring internal/git.Repository.
func (c *testGitClient) ListCommitsSinceRef(refName string) ([]gitpkg.Commit, error) {
	if c.rawRepo == nil {
		return []gitpkg.Commit{}, nil
	}

	head, err := c.rawRepo.Head()
	if err != nil {
		return nil, err
	}

	baseHash, err := c.rawRepo.ResolveRevision(plumbing.Revision(refName))
	if err != nil {
		return nil, fmt.Errorf("could not resolve ref %q: %w", refName, err)
	}

	excluded := make(map[plumbing.Hash]struct{})
	baseIter, err := c.rawRepo.Log(&git.LogOptions{From: *baseHash})
	if err != nil {
		return nil, err
	}
	err = baseIter.ForEach(func(bc *object.Commit) error {
		excluded[bc.Hash] = struct{}{}
		return nil
	})
	baseIter.Close()
	if err != nil {
		return nil, err
	}

	iter, err := c.rawRepo.Log(&git.LogOptions{From: head.Hash()})
	if err != nil {
		return nil, err
	}
	defer iter.Close()

	var commits []gitpkg.Commit
	err = iter.ForEach(func(hc *object.Commit) error {
		if _, ok := excluded[hc.Hash]; ok {
			return nil
		}
		commits = append(commits, gitpkg.Commit{
			SHA:      hc.Hash.String(),
			ShortSHA: hc.Hash.String()[:7],
			Author:   hc.Author.Name,
			Date:     hc.Author.When,
			Message:  hc.Message,
		})
		return nil
	})
	return commits, err
}

func (c *testGitClient) CreateAnnotatedTag(name, message string) (*gitpkg.Tag, error) {
	if c.rawRepo == nil {
		return nil, fmt.Errorf("repo not available")
	}

	head, err := c.rawRepo.Head()
	if err != nil {
		return nil, err
	}

	commit, err := c.rawRepo.CommitObject(head.Hash())
	if err != nil {
		return nil, err
	}

	ref, err := c.rawRepo.CreateTag(name, head.Hash(), &git.CreateTagOptions{
		Tagger: &object.Signature{
			Name:  commit.Author.Name,
			Email: commit.Author.Email,
			When:  time.Now(),
		},
		Message: message,
	})
	if err != nil {
		return nil, err
	}

	tagObj, err := c.rawRepo.TagObject(ref.Hash())
	if err != nil {
		return nil, err
	}

	return &gitpkg.Tag{
		Name: name,
		SHA:  tagObj.Hash.String(),
	}, nil
}

func (c *testGitClient) PushTag(ctx context.Context, tagName string, auth gitpkg.BasicAuth) error {
	return nil
}

func (c *testGitClient) FindTagByName(name string) (*gitpkg.Tag, error) {
	if c.tagsByName != nil {
		if tag, ok := c.tagsByName[name]; ok {
			return tag, nil
		}
	}
	return nil, nil
}

func (c *testGitClient) FindPreviousAnnotatedTag(current *gitpkg.Tag) (*gitpkg.Tag, error) {
	return nil, nil // bootstrap: no previous tag
}

func (c *testGitClient) ListCommitsBetweenTags(from, to *gitpkg.Tag) ([]gitpkg.Commit, error) {
	if c.commitsForRange != nil {
		return c.commitsForRange, nil
	}
	return nil, nil
}

// testGitHubClient implements cli.GitHubClient for testing
type testGitHubClient struct {
	releaseExists   bool
	commentExists   bool
	commentPosted   bool
	prsForCommit    []githubpkg.PR
}

func (c *testGitHubClient) GetReleaseByTag(ctx context.Context, owner, repo, tag string) (*githubpkg.Release, error) {
	if c.releaseExists {
		return &githubpkg.Release{
			ID:      1,
			TagName: tag,
			Body:    "Release notes",
		}, nil
	}
	return nil, githubpkg.ErrNotFound
}

func (c *testGitHubClient) CreateRelease(ctx context.Context, owner, repo string, opts githubpkg.CreateReleaseOptions) (*githubpkg.Release, error) {
	return &githubpkg.Release{
		ID:      1,
		TagName: opts.TagName,
		Body:    opts.Body,
	}, nil
}

func (c *testGitHubClient) ListPRsForCommit(ctx context.Context, owner, repo, sha string) ([]githubpkg.PR, error) {
	return c.prsForCommit, nil
}

func (c *testGitHubClient) SearchPRsForCommit(ctx context.Context, query string) ([]githubpkg.PR, error) {
	return c.prsForCommit, nil
}

func (c *testGitHubClient) PostPRComment(ctx context.Context, owner, repo string, prNumber int, body string) error {
	c.commentPosted = true
	return nil
}

func (c *testGitHubClient) FindPRComment(ctx context.Context, owner, repo string, prNumber int, marker string) (bool, error) {
	return c.commentExists, nil
}
