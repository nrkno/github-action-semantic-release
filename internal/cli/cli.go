package cli

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/nrkno/github-action-semantic-release/internal/conventional"
	"github.com/nrkno/github-action-semantic-release/internal/env"
	"github.com/nrkno/github-action-semantic-release/internal/git"
	"github.com/nrkno/github-action-semantic-release/internal/github"
	"github.com/nrkno/github-action-semantic-release/internal/notes"
	"github.com/nrkno/github-action-semantic-release/internal/output"
	"github.com/nrkno/github-action-semantic-release/internal/semver"
	"github.com/spf13/cobra"
)

// Interface injection for testing
type GitClient interface {
	FindLatestAnnotatedTag(tagPrefix string) (*git.Tag, error)
	FindTagByName(name string) (*git.Tag, error)
	FindPreviousAnnotatedTag(current *git.Tag) (*git.Tag, error)
	ListCommitsSinceTag(tag *git.Tag) ([]git.Commit, error)
	ListCommitsSinceRef(refName string) ([]git.Commit, error)
	ListCommitsBetweenTags(from, to *git.Tag) ([]git.Commit, error)
	CreateAnnotatedTag(name, message string) (*git.Tag, error)
	PushTag(ctx context.Context, tagName string, auth git.BasicAuth) error
}

type GitHubClient interface {
	GetReleaseByTag(ctx context.Context, owner, repo, tag string) (*github.Release, error)
	CreateRelease(ctx context.Context, owner, repo string, opts github.CreateReleaseOptions) (*github.Release, error)
	ListPRsForCommit(ctx context.Context, owner, repo, sha string) ([]github.PR, error)
	SearchPRsForCommit(ctx context.Context, query string) ([]github.PR, error)
	PostPRComment(ctx context.Context, owner, repo string, prNumber int, body string) error
	FindPRComment(ctx context.Context, owner, repo string, prNumber int, marker string) (bool, error)
}

// Root returns the root cobra command
func Root(gitClient GitClient, githubClient GitHubClient, logger *slog.Logger) *cobra.Command {
	root := &cobra.Command{
		Use:           "semrel",
		Short:         "Semantic release automation",
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	root.AddCommand(cmdLint(gitClient, logger))
	root.AddCommand(cmdRelease(gitClient, githubClient, logger))
	root.AddCommand(cmdNotify(gitClient, githubClient, logger))
	root.AddCommand(cmdNotes(gitClient, githubClient, logger))

	return root
}

// cmdLint validates conventional commits in a range
func cmdLint(gitClient GitClient, logger *slog.Logger) *cobra.Command {
	var fromRef, toRef string

	cmd := &cobra.Command{
		Use:   "lint",
		Short: "Validate conventional commits",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Load environment
			ghEnv := env.Load()

			// Load optional .semrelrc.yml from working directory.
			// Absent file → DefaultLintOptions (all rules at defaults).
			// Present file → overrides only the fields specified.
			wd := os.Getenv("INPUT_WORKING_DIRECTORY")
			if wd == "" {
				wd = "."
			}
			cfgLoaded, cfgErr := conventional.LoadConfig(filepath.Join(wd, ".semrelrc.yml"))
			if cfgErr != nil {
				logger.Error("failed to load .semrelrc.yml", "error", cfgErr)
				return cfgErr
			}
			cfg := conventional.DefaultConfig()
			if cfgLoaded != nil {
				cfg = *cfgLoaded
				logger.Debug("loaded .semrelrc.yml",
					"path", filepath.Join(wd, ".semrelrc.yml"),
					"tag_prefix", cfg.TagPrefix,
					"release_branches", cfg.ReleaseBranches,
					"initial_version", cfg.InitialVersion,
				)
			}
			lintOpts := conventional.LintOptions{
				CapitalFirstLetter: cfg.Lint.Rules.CapitalFirstLetter,
				RequireScope:       cfg.Lint.Rules.RequireScope,
				AllowedTypes:       cfg.CommitTypes.AllowedTypes,
			}

			// Determine lint range based on context
			var commits []git.Commit
			var err error
			commitsResolved := false
			switch ghEnv.EventName {
			case "pull_request":
				// PR context: base → HEAD
				if fromRef == "" {
					fromRef = ghEnv.BaseRef
				}
				if toRef == "" {
					toRef = "HEAD"
				}
			case "push":
				// Push context: validate exactly the commits in the push payload when available.
				if fromRef == "" && toRef == "" && ghEnv.PushPayloadLoaded {
					fromRef = ghEnv.PushBefore
					toRef = ghEnv.PushAfter
					commits = commitsFromPushPayload(ghEnv.PushCommits)
					commitsResolved = true
				} else {
					if fromRef == "" {
						tag, err := gitClient.FindLatestAnnotatedTag(cfg.TagPrefix)
						if err != nil {
							logger.Error("failed to find latest tag", "error", err)
							return err
						}
						if tag != nil {
							fromRef = tag.Name
						}
					}
					if toRef == "" {
						toRef = "HEAD"
					}
				}
			case "release":
				// Release context: previous tag → HEAD
				if fromRef == "" {
					tag, err := gitClient.FindLatestAnnotatedTag(cfg.TagPrefix)
					if err != nil {
						logger.Error("failed to find latest tag", "error", err)
						return err
					}
					if tag != nil {
						fromRef = tag.Name
					}
				}
				if toRef == "" {
					toRef = "HEAD"
				}
			default:
				// Bootstrap: all commits
				if fromRef == "" {
					fromRef = ""
				}
				if toRef == "" {
					toRef = "HEAD"
				}
			}

			// List commits in range
			if !commitsResolved {
				if fromRef == "" {
					// Bootstrap: all commits
					commits, err = gitClient.ListCommitsSinceTag(nil)
				} else {
					// fromRef..HEAD — on pull_request events fromRef is the base
					// branch (lint only the PR's own commits; history already on
					// the base branch is out of scope), on release it is the
					// latest tag, and --from-ref overrides automatic detection.
					commits, err = gitClient.ListCommitsSinceRef(fromRef)
				}
			}

			if err != nil {
				logger.Error("failed to list commits", "error", err)
				return err
			}

			logger.Info("lint range",
				"event", ghEnv.EventName,
				"from", fromRef,
				"to", toRef,
				"commit_count", len(commits),
			)

			// Convert to RawCommit for validation
			rawCommits := make([]conventional.RawCommit, len(commits))
			for i, c := range commits {
				rawCommits[i] = conventional.RawCommit{
					SHA:     c.SHA,
					Message: c.Message,
				}
			}

			// Validate
			violations := conventional.ValidateAll(rawCommits, lintOpts)

			if len(violations) > 0 {
				for _, v := range violations {
					logger.Error("commit violation",
						"sha", v.ShortSHA,
						"rule", v.Rule,
						"message", v.RawMessage,
						"example", v.Example,
					)
				}
				return fmt.Errorf("%d commit violation(s) found", len(violations))
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&fromRef, "from-ref", "", "commit range start")
	cmd.Flags().StringVar(&toRef, "to-ref", "", "commit range end")

	return cmd
}

// cmdRelease creates a release and tag
func cmdRelease(gitClient GitClient, githubClient GitHubClient, logger *slog.Logger) *cobra.Command {
	var dryRun bool

	cmd := &cobra.Command{
		Use:   "release",
		Short: "Create a release and tag",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()

			// Step 1: Load config
			wd := os.Getenv("INPUT_WORKING_DIRECTORY")
			if wd == "" {
				wd = "."
			}
			cfgLoaded, cfgErr := conventional.LoadConfig(filepath.Join(wd, ".semrelrc.yml"))
			if cfgErr != nil {
				logger.Error("failed to load .semrelrc.yml", "error", cfgErr)
				return cfgErr
			}
			cfg := conventional.DefaultConfig()
			if cfgLoaded != nil {
				cfg = *cfgLoaded
				logger.Debug("loaded .semrelrc.yml",
					"path", filepath.Join(wd, ".semrelrc.yml"),
					"tag_prefix", cfg.TagPrefix,
					"release_branches", cfg.ReleaseBranches,
					"initial_version", cfg.InitialVersion,
				)
			}

			// Step 2: Validate InitialVersion early (before any git/GitHub I/O)
			if _, err := semver.ParseVersion(cfg.InitialVersion); err != nil {
				return fmt.Errorf(".semrelrc.yml: invalid initial-version %q: %w", cfg.InitialVersion, err)
			}

			// Load environment
			ghEnv := env.Load()

			// Step 3: Branch guard (skipped in dry-run so PR branches can preview)
			if !dryRun {
				if len(cfg.ReleaseBranches) > 0 {
					allowed := false
					for _, pattern := range cfg.ReleaseBranches {
						if ok, _ := path.Match(pattern, ghEnv.RefName); ok {
							allowed = true
							break
						}
					}
					if !allowed {
						logger.Info("skipping release: branch not in release-branches",
							"branch", ghEnv.RefName,
							"release-branches", cfg.ReleaseBranches,
						)
						return outputReleaseFields(ghEnv.Output, semver.Version{}, cfg.TagPrefix, false)
					}
				}
			}

			// Parse repository
			parts := strings.Split(ghEnv.Repository, "/")
			if len(parts) != 2 {
				return fmt.Errorf("invalid repository format: %s", ghEnv.Repository)
			}
			owner, repo := parts[0], parts[1]

			// Step 4: Find latest tag using configured prefix
			t0 := time.Now()
			latestTag, err := gitClient.FindLatestAnnotatedTag(cfg.TagPrefix)
			if err != nil {
				logger.Error("failed to find latest tag", "error", err)
				return err
			}
			logger.Debug("FindLatestAnnotatedTag",
				"duration_ms", time.Since(t0).Milliseconds(),
				"found", latestTag != nil,
			)
			if latestTag != nil && !latestTag.IsAnnotated {
				logger.Warn("latest tag is a lightweight tag — semrel works best with annotated tags. "+
					"The next release will create an annotated tag going forward. "+
					"No action required, but consider running: git tag -a "+latestTag.Name+" <sha> -m 'migration' to convert.",
					"tag", latestTag.Name)
			}

			// List commits since tag
			t1 := time.Now()
			commits, err := gitClient.ListCommitsSinceTag(latestTag)
			if err != nil {
				logger.Error("failed to list commits", "error", err)
				return err
			}
			logger.Debug("ListCommitsSinceTag",
				"duration_ms", time.Since(t1).Milliseconds(),
				"count", len(commits),
			)

			// Parse commits
			var parsedCommits []conventional.Commit
			commitTypes := []string{}
			for _, c := range commits {
				parsed, err := conventional.Parse(conventional.RawCommit{
					SHA:     c.SHA,
					Message: c.Message,
				})
				if err != nil {
					logger.Warn("failed to parse commit", "sha", c.ShortSHA, "error", err)
					continue
				}
				parsed.SHA = c.SHA
				parsed.ShortSHA = c.ShortSHA
				parsedCommits = append(parsedCommits, parsed)
				commitTypes = append(commitTypes, string(parsed.Type))
				// Step 5: Inject "breaking-change" sentinel for breaking commits
				if parsed.Breaking {
					commitTypes = append(commitTypes, "breaking-change")
				}
			}

			// Log 1: commits in release
			logger.Info("commits in release",
				"count", len(parsedCommits),
				"feat", countByType(parsedCommits, conventional.TypeFeat),
				"fix", countByType(parsedCommits, conventional.TypeFix),
				"breaking", countBreaking(parsedCommits),
			)

			// Step 6: Detect bump type with configured rules
			bump := semver.DetectBumpType(commitTypes, cfg.BumpRules)

			// Calculate next version
			var currentVersion semver.Version
			var nextVersion semver.Version
			if latestTag == nil {
				// Step 7: Bootstrap using configured InitialVersion
				baseVersion, _ := semver.ParseVersion(cfg.InitialVersion) // safe: validated above
				nextVersion = semver.NextVersion(baseVersion, bump)
				currentVersion = baseVersion // set before BumpNone check so output is correct

				if bump == semver.BumpNone {
					logger.Info("no release: no bump-worthy commits",
						"total_commits", len(parsedCommits),
						"feat", countByType(parsedCommits, conventional.TypeFeat),
						"fix", countByType(parsedCommits, conventional.TypeFix),
						"chore", countByType(parsedCommits, conventional.TypeChore),
					)
					return outputReleaseFields(ghEnv.Output, currentVersion, cfg.TagPrefix, false)
				}

				// Only log "bootstrapping" when we are actually about to create a release
				logger.Info("no prior annotated tags found — bootstrapping version",
					"initial-version", cfg.InitialVersion,
					"version", semver.FormatTagWithPrefix(nextVersion, cfg.TagPrefix),
				)
			} else {
				// Step 8: Parse tag using configured prefix
				currentVersion, err = semver.ParseVersionFromTag(latestTag.Name, cfg.TagPrefix)
				if err != nil {
					logger.Error("failed to parse version", "error", err)
					return err
				}
				nextVersion = semver.NextVersion(currentVersion, bump)

				if bump == semver.BumpNone {
					logger.Info("no release: no bump-worthy commits",
						"total_commits", len(parsedCommits),
						"feat", countByType(parsedCommits, conventional.TypeFeat),
						"fix", countByType(parsedCommits, conventional.TypeFix),
						"chore", countByType(parsedCommits, conventional.TypeChore),
					)
					return outputReleaseFields(ghEnv.Output, currentVersion, cfg.TagPrefix, false)
				}
			}

			// Log 2: bump detected (only reached when bump != BumpNone)
			logger.Info("bump detected",
				"type", bump.String(),
				"from", semver.FormatTagWithPrefix(currentVersion, cfg.TagPrefix),
				"to", semver.FormatTagWithPrefix(nextVersion, cfg.TagPrefix),
			)

			// Step 9: Format version tag with configured prefix
			versionTag := semver.FormatTagWithPrefix(nextVersion, cfg.TagPrefix)

			// Fetch PRs for all commits in this release (used in logs 3/4 and release notes)
			prMap := fetchPRsForCommits(ctx, githubClient, owner, repo, parsedCommits, logger)

			// Log 3: PRs included in the release — one line per unique PR
			if len(prMap) > 0 {
				type prEntry struct {
					pr          github.PR
					commitCount int
				}
				uniquePRs := make(map[int]prEntry)
				for _, pr := range prMap {
					e := uniquePRs[pr.Number]
					e.pr = pr
					e.commitCount++
					uniquePRs[pr.Number] = e
				}
				for _, e := range uniquePRs {
					logger.Info("PR in release",
						"pr", fmt.Sprintf("#%d", e.pr.Number),
						"title", e.pr.Title,
						"commits", e.commitCount,
					)
				}
			}

			// Log 4: release triggered by PR or commit
			if triggerCommit := findTriggerCommit(parsedCommits, bump); triggerCommit != nil {
				if pr, ok := prMap[triggerCommit.SHA]; ok {
					logger.Info("release triggered by PR",
						"pr", fmt.Sprintf("#%d", pr.Number),
						"title", pr.Title,
						"url", pr.URL,
					)
				} else {
					logger.Info("release triggered by commit",
						"sha", triggerCommit.ShortSHA,
						"message", commitSubject(triggerCommit.RawMessage),
					)
				}
			}

			// Convert github.PR map to notes.PR map for release notes generation
			notesPRMap := githubPRMapToNotesPRMap(prMap)

			// Idempotency ladder
			// Rung 1: Check if release already exists
			existingRelease, err := githubClient.GetReleaseByTag(ctx, owner, repo, versionTag)
			if err == nil && existingRelease != nil {
				// Release exists, noop
				logger.Info("release already exists", "tag", versionTag)
				return outputReleaseFields(ghEnv.Output, nextVersion, cfg.TagPrefix, false)
			}

			// Rung 2: Check whether the computed next-version tag already exists.
			// This handles the retry case: tag was pushed in a previous interrupted
			// run, but the GitHub release was not yet created.
			existingVersionTag, err := gitClient.FindTagByName(versionTag)
			if err != nil {
				logger.Error("failed to look up version tag", "tag", versionTag, "error", err)
				return err
			}
			if existingVersionTag != nil {
				// The next-version tag already exists — check it points to HEAD
				headCommits, err := gitClient.ListCommitsSinceTag(nil)
				if err != nil || len(headCommits) == 0 {
					return fmt.Errorf("could not determine HEAD SHA")
				}
				headSHA := headCommits[0].SHA
				if existingVersionTag.TargetSHA() != headSHA {
					// Tag points to a different commit — genuine conflict
					return fmt.Errorf("tag %s exists but points to different commit: %s vs %s",
						versionTag, existingVersionTag.TargetSHA()[:7], headSHA[:7])
				}
				// Tag already at HEAD — just create the GitHub release (idempotent retry)
				if !dryRun {
					releaseNotes := generateReleaseNotes(parsedCommits, notesPRMap)
					release, err := githubClient.CreateRelease(ctx, owner, repo, github.CreateReleaseOptions{
						TagName: versionTag,
						Name:    versionTag,
						Body:    releaseNotes,
					})
					if err != nil {
						logger.Error("failed to create release for existing tag", "error", err)
						return err
					}
					// Log 7: GitHub release created
					logger.Info("created GitHub release",
						"tag", release.TagName,
						"url", release.HTMLURL,
					)
				}
				if !dryRun {
					logger.Info("created release for existing tag", "tag", versionTag)
				} else {
					logger.Info("dry-run: tag already exists at HEAD, no release created", "tag", versionTag)
				}
				return outputReleaseFields(ghEnv.Output, nextVersion, cfg.TagPrefix, !dryRun)
			}
			// Rung 3 (full flow) falls through here

			// Rung 3: Full flow - create tag, push, then create release.
			// Order is critical: PushTag MUST come before CreateRelease.
			// If CreateRelease fires first, GitHub auto-creates a lightweight
			// remote tag pointing at the commit. go-git then fails pushing the
			// local annotated tag object over it ("object not found").
			if !dryRun {
				releaseNotes := generateReleaseNotes(parsedCommits, notesPRMap)
				tagMessage := fmt.Sprintf("Release %s\n\n%s", versionTag, releaseNotes)

				logger.Info("proceeding with full release", "tag", versionTag)

				// 1. Create local annotated tag
				tag, err := gitClient.CreateAnnotatedTag(versionTag, tagMessage)
				if err != nil {
					logger.Error("failed to create tag", "error", err)
					return err
				}
				// Log 5: annotated tag created
				logger.Info("created annotated tag",
					"tag", tag.Name,
					"commit", tag.TargetSHA()[:7],
				)

				// 2. Push tag to remote BEFORE creating the release
				auth := git.BasicAuth{
					Username: "x-access-token",
					Password: ghEnv.Token,
				}
				err = gitClient.PushTag(ctx, versionTag, auth)
				if err != nil {
					logger.Error("failed to push tag",
						"tag", versionTag,
						"remote", "origin",
						"auth_user", auth.Username,
						"token_present", ghEnv.Token != "",
						"error", err,
					)
					return err
				}
				// Log 6: tag pushed
				logger.Info("pushed tag to origin", "tag", versionTag)

				// 3. Create release — tag now exists on remote as annotated
				release, err := githubClient.CreateRelease(ctx, owner, repo, github.CreateReleaseOptions{
					TagName: versionTag,
					Name:    versionTag,
					Body:    releaseNotes,
				})
				if err != nil {
					logger.Error("failed to create release", "error", err)
					return err
				}
				// Log 7: GitHub release created
				logger.Info("created GitHub release",
					"tag", release.TagName,
					"url", release.HTMLURL,
				)
			}

			if !dryRun {
				logger.Info("release created", "tag", versionTag)
			} else {
				logger.Info("dry-run: computed next version, no release created", "tag", versionTag)
			}
			return outputReleaseFields(ghEnv.Output, nextVersion, cfg.TagPrefix, !dryRun)
		},
	}

	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "preview mode: skip branch guard, compute next version, write output fields, but perform no writes")

	return cmd
}

// cmdNotify posts release comments on all PRs included in the release.
// Designed to be triggered by on: release: types: [published] — NOT push events.
// Required env: SEMREL_TAG (the released tag, e.g. "v1.3.0")
// Optional env: SEMREL_RELEASE_URL (constructed from GITHUB_SERVER_URL if absent)
func cmdNotify(gitClient GitClient, githubClient GitHubClient, logger *slog.Logger) *cobra.Command {
	return &cobra.Command{
		Use:   "notify",
		Short: "Post release comments on all PRs included in the release",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			ghEnv := env.Load()

			// SEMREL_TAG is required — absent means this wasn't triggered by a real release
			semrelTag := os.Getenv("SEMREL_TAG")
			if semrelTag == "" {
				logger.Info("skipping notify: SEMREL_TAG not set")
				return nil
			}

			// Parse owner/repo
			parts := strings.Split(ghEnv.Repository, "/")
			if len(parts) != 2 {
				return fmt.Errorf("invalid repository format: %s", ghEnv.Repository)
			}
			owner, repo := parts[0], parts[1]

			// Build release URL
			releaseURL := os.Getenv("SEMREL_RELEASE_URL")
			if releaseURL == "" {
				releaseURL = fmt.Sprintf("%s/%s/releases/tag/%s",
					ghEnv.ServerURL, ghEnv.Repository, semrelTag)
			}

			// Load config for TagPrefix
			wd := os.Getenv("INPUT_WORKING_DIRECTORY")
			if wd == "" {
				wd = "."
			}
			cfgLoaded, cfgErr := conventional.LoadConfig(filepath.Join(wd, ".semrelrc.yml"))
			if cfgErr != nil {
				logger.Error("failed to load .semrelrc.yml", "error", cfgErr)
				return cfgErr
			}
			cfg := conventional.DefaultConfig()
			if cfgLoaded != nil {
				cfg = *cfgLoaded
				logger.Debug("loaded .semrelrc.yml",
					"path", filepath.Join(wd, ".semrelrc.yml"),
					"tag_prefix", cfg.TagPrefix,
					"release_branches", cfg.ReleaseBranches,
					"initial_version", cfg.InitialVersion,
				)
			}
			_ = cfg // TagPrefix reserved for future filtering

			// Find the released tag in git
			releasedTag, err := gitClient.FindTagByName(semrelTag)
			if err != nil {
				return fmt.Errorf("failed to look up released tag %q: %w", semrelTag, err)
			}
			if releasedTag == nil {
				return fmt.Errorf("released tag %q not found in repository — ensure fetch-tags: true in checkout", semrelTag)
			}

			// Find the previous tag (nil = first release)
			prevTag, err := gitClient.FindPreviousAnnotatedTag(releasedTag)
			if err != nil {
				return fmt.Errorf("failed to find previous tag: %w", err)
			}

			// List all commits in this release
			commits, err := gitClient.ListCommitsBetweenTags(prevTag, releasedTag)
			if err != nil {
				return fmt.Errorf("failed to list commits: %w", err)
			}

			logger.Info("commits in release for notification",
				"tag", semrelTag,
				"prev_tag", func() string {
					if prevTag != nil {
						return prevTag.Name
					}
					return "(none)"
				}(),
				"count", len(commits),
			)

			// Collect unique PRs across all commits
			prMap := make(map[int]github.PR)
			apiErrors := 0
			for _, commit := range commits {
				prs, err := githubClient.ListPRsForCommit(ctx, owner, repo, commit.SHA)
				if err != nil {
					logger.Warn("failed to list PRs for commit", "sha", commit.ShortSHA, "error", err)
					apiErrors++
					continue
				}
				for _, pr := range prs {
					prMap[pr.Number] = pr
				}
			}

			if len(prMap) == 0 {
				if apiErrors > 0 && apiErrors == len(commits) {
					logger.Warn("no PRs notified — all PR lookups failed",
						"api_errors", apiErrors, "commits_checked", len(commits))
				} else if apiErrors > 0 {
					logger.Warn("no PRs associated with release — partial lookup failures",
						"api_errors", apiErrors, "commits_checked", len(commits))
				} else {
					logger.Info("no PRs associated with release", "tag", semrelTag)
				}
				return nil
			}

			// Post idempotent comment on each PR
			marker := fmt.Sprintf("<!-- semrel-notify:%s -->", semrelTag)
			body := fmt.Sprintf("%s\n🎉 This pull request has been included in release [%s](%s).",
				marker, semrelTag, releaseURL)

			notified := 0
			for _, pr := range prMap {
				found, err := githubClient.FindPRComment(ctx, owner, repo, pr.Number, marker)
				if err != nil {
					logger.Error("failed to check existing comment", "pr", pr.Number, "error", err)
					return err
				}
				if found {
					logger.Info("comment already exists, skipping", "pr", pr.Number)
					continue
				}
				if err := githubClient.PostPRComment(ctx, owner, repo, pr.Number, body); err != nil {
					logger.Error("failed to post comment", "pr", pr.Number, "error", err)
					return err
				}
				logger.Info("posted release comment", "pr", pr.Number, "tag", semrelTag)
				notified++
			}
			logger.Info("notification complete", "prs_notified", notified, "tag", semrelTag)

			return nil
		},
	}
}

// cmdNotes generates release notes
func cmdNotes(gitClient GitClient, githubClient GitHubClient, logger *slog.Logger) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "notes",
		Short: "Generate release notes",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()

			// Load config
			wd := os.Getenv("INPUT_WORKING_DIRECTORY")
			if wd == "" {
				wd = "."
			}
			notesCfg, notesCfgErr := conventional.LoadConfig(filepath.Join(wd, ".semrelrc.yml"))
			if notesCfgErr != nil {
				logger.Error("failed to load .semrelrc.yml", "error", notesCfgErr)
				return notesCfgErr
			}
			notesCfgResolved := conventional.DefaultConfig()
			if notesCfg != nil {
				notesCfgResolved = *notesCfg
				logger.Debug("loaded .semrelrc.yml",
					"path", filepath.Join(wd, ".semrelrc.yml"),
					"tag_prefix", notesCfgResolved.TagPrefix,
					"release_branches", notesCfgResolved.ReleaseBranches,
					"initial_version", notesCfgResolved.InitialVersion,
				)
			}

			// Load environment
			ghEnv := env.Load()

			// Parse repository
			parts := strings.Split(ghEnv.Repository, "/")
			if len(parts) != 2 {
				return fmt.Errorf("invalid repository format: %s", ghEnv.Repository)
			}
			owner, repo := parts[0], parts[1]

			// Find latest tag using configured prefix
			latestTag, err := gitClient.FindLatestAnnotatedTag(notesCfgResolved.TagPrefix)
			if err != nil {
				logger.Error("failed to find latest tag", "error", err)
				return err
			}

			// List commits since tag
			commits, err := gitClient.ListCommitsSinceTag(latestTag)
			if err != nil {
				logger.Error("failed to list commits", "error", err)
				return err
			}

			// Parse commits
			var parsedCommits []conventional.Commit
			for _, c := range commits {
				parsed, err := conventional.Parse(conventional.RawCommit{
					SHA:     c.SHA,
					Message: c.Message,
				})
				if err != nil {
					logger.Warn("failed to parse commit", "sha", c.ShortSHA, "error", err)
					continue
				}
				parsed.SHA = c.SHA
				parsed.ShortSHA = c.ShortSHA
				parsedCommits = append(parsedCommits, parsed)
			}

			// Build PR map
			prMap := make(map[string]notes.PR)
			for _, commit := range parsedCommits {
				prs, err := githubClient.ListPRsForCommit(ctx, owner, repo, commit.SHA)
				if err != nil {
					logger.Warn("failed to list PRs for commit", "sha", commit.ShortSHA, "error", err)
					continue
				}
				for _, pr := range prs {
					prMap[commit.SHA] = notes.PR{
						Number: pr.Number,
						URL:    pr.URL,
					}
				}
			}

			// Generate notes
			releaseNotes := notes.Generate(parsedCommits, prMap)

			// Output
			if _, err := fmt.Fprint(cmd.OutOrStdout(), releaseNotes.Body); err != nil {
				logger.Warn("failed to write notes", "error", err)
			}

			// Also write to GITHUB_OUTPUT if set
			if ghEnv.Output != "" {
				err := output.WriteFields(ghEnv.Output, map[string]string{
					"notes": releaseNotes.Body,
				})
				if err != nil {
					logger.Error("failed to write output", "error", err)
					return err
				}
			}

			return nil
		},
	}

	return cmd
}

// Helper functions

func countByType(commits []conventional.Commit, t conventional.CommitType) int {
	n := 0
	for _, c := range commits {
		if c.Type == t {
			n++
		}
	}
	return n
}

func commitsFromPushPayload(pushCommits []env.PushCommit) []git.Commit {
	commits := make([]git.Commit, 0, len(pushCommits))
	for _, c := range pushCommits {
		shortSHA := c.ID
		if len(shortSHA) > 7 {
			shortSHA = shortSHA[:7]
		}
		commits = append(commits, git.Commit{
			SHA:      c.ID,
			ShortSHA: shortSHA,
			Date:     c.Timestamp,
			Message:  c.Message,
		})
	}
	return commits
}

func countBreaking(commits []conventional.Commit) int {
	n := 0
	for _, c := range commits {
		if c.Breaking {
			n++
		}
	}
	return n
}

func fetchPRsForCommits(ctx context.Context, gh GitHubClient, owner, repo string, commits []conventional.Commit, logger *slog.Logger) map[string]github.PR {
	prMap := make(map[string]github.PR)
	for _, commit := range commits {
		prs, err := gh.ListPRsForCommit(ctx, owner, repo, commit.SHA)
		if err != nil {
			logger.Warn("failed to list PRs for commit", "sha", commit.ShortSHA, "error", err)
			continue
		}
		if len(prs) > 0 {
			prMap[commit.SHA] = prs[0]
			logger.Debug("commit associated with PR",
				"sha", commit.ShortSHA,
				"pr", fmt.Sprintf("#%d", prs[0].Number),
			)
		}
	}
	return prMap
}

func findTriggerCommit(commits []conventional.Commit, bump semver.BumpType) *conventional.Commit {
	for i := range commits {
		switch bump {
		case semver.BumpMajor:
			if commits[i].Breaking {
				return &commits[i]
			}
		case semver.BumpMinor:
			if commits[i].Type == conventional.TypeFeat {
				return &commits[i]
			}
		case semver.BumpPatch:
			if commits[i].Type == conventional.TypeFix {
				return &commits[i]
			}
		}
	}
	return nil
}

func githubPRMapToNotesPRMap(m map[string]github.PR) map[string]notes.PR {
	out := make(map[string]notes.PR, len(m))
	for k, pr := range m {
		out[k] = notes.PR{Number: pr.Number, URL: pr.URL}
	}
	return out
}

func generateReleaseNotes(commits []conventional.Commit, prMap map[string]notes.PR) string {
	if prMap == nil {
		prMap = make(map[string]notes.PR)
	}
	releaseNotes := notes.Generate(commits, prMap)
	return releaseNotes.Body
}

// commitSubject returns the first line of a commit message (the subject).
func commitSubject(message string) string {
	if i := strings.Index(message, "\n"); i != -1 {
		return message[:i]
	}
	return message
}

func outputReleaseFields(outputFile string, version semver.Version, tagPrefix string, released bool) error {
	fields := map[string]string{
		"version":  version.String(),
		"tag":      semver.FormatTagWithPrefix(version, tagPrefix),
		"major":    fmt.Sprintf("%d", version.Major),
		"minor":    fmt.Sprintf("%d", version.Minor),
		"patch":    fmt.Sprintf("%d", version.Patch),
		"released": fmt.Sprintf("%v", released),
	}
	return output.WriteFields(outputFile, fields)
}
