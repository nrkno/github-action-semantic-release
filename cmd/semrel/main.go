package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"runtime"

	"github.com/nrkno/github-action-semantic-release/internal/cli"
	"github.com/nrkno/github-action-semantic-release/internal/git"
	"github.com/nrkno/github-action-semantic-release/internal/github"
)

var (
	version = "dev"
	commit  = "unknown"
	date    = "unknown"
)

func main() {
	// Setup logger (stderr) — level configurable via SEMREL_LOG_LEVEL
	var level slog.LevelVar // zero value = INFO
	if lvlStr := os.Getenv("SEMREL_LOG_LEVEL"); lvlStr != "" {
		if err := level.UnmarshalText([]byte(lvlStr)); err != nil {
			fmt.Fprintf(os.Stderr, "semrel: invalid SEMREL_LOG_LEVEL %q, defaulting to INFO\n", lvlStr)
		}
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: &level,
	}))
	logger.Debug("semrel starting",
		"version", version,
		"commit", commit,
		"built", date,
		"go", runtime.Version(),
	)

	// Create clients - open git repo lazily in each command
	ctx := context.Background()
	var gitClient cli.GitClient
	gitRepo, err := git.OpenRepo(".")
	if err != nil {
		var shallowErr git.ShallowRepoError
		if errors.As(err, &shallowErr) {
			logger.Error("repository is a shallow clone — semrel requires full git history",
				"fix", "add 'fetch-depth: 0' to your actions/checkout step",
			)
			os.Exit(2)
		}
		// For help/version commands, we don't need the repo
		// So we'll create a lazy client and let the command handle it
		logger.Debug("git repo not available", "error", err)
		gitClient = &lazyGitClient{err: err}
	} else {
		gitClient = gitRepo
	}

	githubClient, err := github.NewClient(os.Getenv("GITHUB_TOKEN"), os.Getenv("GITHUB_API_URL"))
	if err != nil {
		logger.Error("failed to create GitHub client", "error", err)
		os.Exit(1)
	}
	if os.Getenv("GITHUB_TOKEN") == "" {
		logger.Warn("GITHUB_TOKEN is not set — GitHub API calls will return 401")
	}

	// Create and execute root command
	root := cli.Root(gitClient, githubClient, logger)
	root.Version = fmt.Sprintf("%s (commit: %s, built: %s)", version, commit, date)

	if err := root.ExecuteContext(ctx); err != nil {
		logger.Error("command failed", "error", err)
		os.Exit(1)
	}
}

// lazyGitClient wraps a git client and defers errors until actual use
type lazyGitClient struct {
	err error
}

func (c *lazyGitClient) FindLatestAnnotatedTag(tagPrefix string) (*git.Tag, error) {
	return nil, c.err
}

func (c *lazyGitClient) FindTagByName(name string) (*git.Tag, error) {
	return nil, c.err
}

func (c *lazyGitClient) FindPreviousAnnotatedTag(current *git.Tag) (*git.Tag, error) {
	return nil, c.err
}

func (c *lazyGitClient) ListCommitsSinceTag(tag *git.Tag) ([]git.Commit, error) {
	return nil, c.err
}

func (c *lazyGitClient) ListCommitsSinceRef(refName string) ([]git.Commit, error) {
	return nil, c.err
}

func (c *lazyGitClient) ListCommitsBetweenTags(from, to *git.Tag) ([]git.Commit, error) {
	return nil, c.err
}

func (c *lazyGitClient) CreateAnnotatedTag(name, message string) (*git.Tag, error) {
	return nil, c.err
}

func (c *lazyGitClient) PushTag(ctx context.Context, tagName string, auth git.BasicAuth) error {
	return c.err
}
