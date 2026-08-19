package git

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/storer"
	"github.com/go-git/go-git/v5/plumbing/transport/http"

	internalSemver "github.com/nrkno/github-action-semantic-release/internal/semver"
)

// Commit represents a git commit
type Commit struct {
	SHA      string    // full commit SHA
	ShortSHA string    // first 7 chars
	Author   string
	Date     time.Time
	Message  string
}

// Tag represents a git tag (annotated or lightweight).
type Tag struct {
	Name        string // tag name (e.g., "v1.0.0")
	SHA         string // tag object SHA (annotated) or commit SHA (lightweight)
	targetSHA   string // commit SHA that this tag points to
	IsAnnotated bool   // true = annotated tag object; false = lightweight tag
}

// TargetSHA returns the commit SHA that this tag points to (distinct from tag object SHA)
func (t *Tag) TargetSHA() string {
	return t.targetSHA
}

// NewTag constructs a Tag with all fields set. Intended for use in tests.
func NewTag(name, sha, targetSHA string) *Tag {
	return &Tag{
		Name:        name,
		SHA:         sha,
		targetSHA:   targetSHA,
		IsAnnotated: false,
	}
}

// Repository wraps a go-git repository
type Repository struct {
	raw *gogit.Repository
}

// ShallowRepoError is returned when a shallow clone is detected
type ShallowRepoError struct {
	Message string
}

func (e ShallowRepoError) Error() string {
	return e.Message
}

// BasicAuth holds HTTPS authentication credentials
type BasicAuth struct {
	Username string
	Password string
}

// OpenRepo opens a git repository at the given path.
// Returns ShallowRepoError if the repo is a shallow clone.
func OpenRepo(path string) (*Repository, error) {
	// Check for .git/shallow file first
	shallowPath := path + "/.git/shallow"
	if _, err := os.Stat(shallowPath); err == nil {
		return nil, ShallowRepoError{Message: "repository is a shallow clone"}
	}

	repo, err := gogit.PlainOpen(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open repository: %w", err)
	}

	// Check if go-git detects a shallow repository via ShallowStorer
	shallow, err := repo.Storer.(storer.ShallowStorer).Shallow()
	if err == nil && len(shallow) > 0 {
		return nil, ShallowRepoError{Message: "repository is a shallow clone"}
	}

	return &Repository{raw: repo}, nil
}

// FindLatestTag finds the tag with the highest semver value across all tags
// (annotated and lightweight). If tagPrefix is non-empty, only tags with that
// prefix are considered. Tags whose names do not parse as valid semver after
// prefix stripping are silently skipped. Returns nil, nil when no parseable
// semver tags exist (bootstrap case).
func (r *Repository) FindLatestTag(tagPrefix string) (*Tag, error) {
	iter, err := r.raw.Tags()
	if err != nil {
		return nil, fmt.Errorf("failed to list tags: %w", err)
	}

	type candidate struct {
		tag     *Tag
		version internalSemver.Version
	}

	var candidates []candidate
	err = iter.ForEach(func(ref *plumbing.Reference) error {
		name := ref.Name().Short()
		// Apply prefix filter first — skip tags that don't carry the prefix.
		if tagPrefix != "" && !strings.HasPrefix(name, tagPrefix) {
			return nil
		}

		var tag *Tag
		// Try annotated tag first.
		if obj, aerr := r.raw.TagObject(ref.Hash()); aerr == nil {
			tag = &Tag{
				Name:        name,
				SHA:         obj.Hash.String(),
				targetSHA:   obj.Target.String(),
				IsAnnotated: true,
			}
		} else {
			// Lightweight tag — must resolve directly to a commit.
			commit, cerr := r.raw.CommitObject(ref.Hash())
			if cerr != nil {
				// Neither annotated nor a direct commit pointer — skip silently.
				return nil
			}
			tag = &Tag{
				Name:        name,
				SHA:         ref.Hash().String(),
				targetSHA:   commit.Hash.String(),
				IsAnnotated: false,
			}
		}

		// Parse tag name through the internal semver package.
		// Non-semver names are silently dropped per invariant 5.
		ver, perr := internalSemver.ParseVersionFromTag(name, tagPrefix)
		if perr != nil {
			return nil
		}

		candidates = append(candidates, candidate{tag: tag, version: ver})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to iterate tags: %w", err)
	}

	if len(candidates) == 0 {
		return nil, nil // bootstrap: no parseable semver tags
	}

	// Sort descending by semver value. sort.Slice is not stable, so both
	// tiebreaker keys are applied in a single comparator to guarantee
	// deterministic ordering when two tags parse to equal semver
	// (e.g. v4.0 and v4.0.0 are equal under Masterminds/semver).
	sort.Slice(candidates, func(i, j int) bool {
		vi, vj := candidates[i].version, candidates[j].version
		if vi.Major != vj.Major {
			return vi.Major > vj.Major
		}
		if vi.Minor != vj.Minor {
			return vi.Minor > vj.Minor
		}
		if vi.Patch != vj.Patch {
			return vi.Patch > vj.Patch
		}
		// Semver equal — prefer the tag with more dot-separated components
		// (v4.0.0 beats v4.0) then fall back to lexicographic descending.
		di := strings.Count(candidates[i].tag.Name, ".")
		dj := strings.Count(candidates[j].tag.Name, ".")
		if di != dj {
			return di > dj
		}
		return candidates[i].tag.Name > candidates[j].tag.Name
	})

	return candidates[0].tag, nil
}

// NewFromRaw wraps a *gogit.Repository for use in tests.
// Not for production use.
func NewFromRaw(r *gogit.Repository) *Repository {
	return &Repository{raw: r}
}

// FindPreviousAnnotatedTag finds the annotated tag before the given tag.
// Returns nil, nil if the given tag is the only tag.
func (r *Repository) FindPreviousAnnotatedTag(current *Tag) (*Tag, error) {
	tags, err := r.raw.Tags()
	if err != nil {
		return nil, fmt.Errorf("failed to list tags: %w", err)
	}

	var annotatedTags []*Tag
	err = tags.ForEach(func(ref *plumbing.Reference) error {
		// Only process annotated tags (tag objects), not lightweight tags
		obj, err := r.raw.TagObject(ref.Hash())
		if err != nil {
			// Not an annotated tag (lightweight tag), skip
			return nil
		}

		// Get the target commit SHA
		targetSHA := obj.Target.String()

		tag := &Tag{
			Name:      ref.Name().Short(),
			SHA:       obj.Hash.String(),
			targetSHA: targetSHA,
		}
		annotatedTags = append(annotatedTags, tag)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to iterate tags: %w", err)
	}

	// Sort by target commit date (most recent first)
	sort.Slice(annotatedTags, func(i, j int) bool {
		commitI, errI := r.raw.CommitObject(plumbing.NewHash(annotatedTags[i].targetSHA))
		commitJ, errJ := r.raw.CommitObject(plumbing.NewHash(annotatedTags[j].targetSHA))
		if errI != nil || errJ != nil {
			return false
		}
		return commitI.Author.When.After(commitJ.Author.When)
	})

	// Find the current tag in the sorted list
	currentIdx := -1
	for i, tag := range annotatedTags {
		if tag.SHA == current.SHA {
			currentIdx = i
			break
		}
	}

	if currentIdx == -1 || currentIdx == len(annotatedTags)-1 {
		// Current tag not found or is the last (oldest) tag
		return nil, nil
	}

	return annotatedTags[currentIdx+1], nil
}

// ListCommitsSinceTag lists all commits from HEAD back to (but not including) the tag's target commit.
// If tag is nil, returns all commits from HEAD (bootstrap case).
// Returns commits in reverse-chronological order (newest first).
func (r *Repository) ListCommitsSinceTag(tag *Tag) ([]Commit, error) {
	head, err := r.raw.Head()
	if err != nil {
		return nil, fmt.Errorf("failed to get HEAD: %w", err)
	}

	iter, err := r.raw.Log(&gogit.LogOptions{
		From:  head.Hash(),
		Order: gogit.LogOrderCommitterTime,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create log iterator: %w", err)
	}
	defer iter.Close()

	var commits []Commit
	var stopHash plumbing.Hash
	if tag != nil {
		stopHash = plumbing.NewHash(tag.TargetSHA())
	}

	err = iter.ForEach(func(c *object.Commit) error {
		// Skip the tag's target commit itself
		if tag != nil && c.Hash == stopHash {
			return storer.ErrStop
		}

		commit := Commit{
			SHA:      c.Hash.String(),
			ShortSHA: c.Hash.String()[:7],
			Author:   c.Author.Name,
			Date:     c.Author.When,
			Message:  c.Message,
		}
		commits = append(commits, commit)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to iterate commits: %w", err)
	}

	return commits, nil
}

// resolveRefToCommit resolves a ref name (branch, remote branch, tag, or SHA)
// to a commit hash. Bare branch names fall back to origin/<name>, which is the
// normal state in an actions/checkout PR workspace where the base branch only
// exists as a remote-tracking ref. Annotated tags are peeled to their target commit.
func (r *Repository) resolveRefToCommit(refName string) (plumbing.Hash, error) {
	candidates := []string{
		refName,
		"refs/remotes/origin/" + refName,
		"refs/heads/" + refName,
	}
	for _, candidate := range candidates {
		hash, err := r.raw.ResolveRevision(plumbing.Revision(candidate))
		if err != nil {
			continue
		}
		// Peel annotated tag objects to their target commit
		if tagObj, tagErr := r.raw.TagObject(*hash); tagErr == nil {
			target := tagObj.Target
			return target, nil
		}
		return *hash, nil
	}
	return plumbing.ZeroHash, fmt.Errorf("could not resolve ref %q (tried %v)", refName, candidates)
}

// ListCommitsSinceRef lists commits reachable from HEAD but not from refName —
// equivalent to `git log refName..HEAD`. Used on pull_request events to lint
// only the PR's own commits (base ref → HEAD), excluding history already on
// the base branch (including commits reachable via merge-commit parents, as in
// GitHub's refs/pull/N/merge checkout).
// Returns commits in reverse-chronological order (newest first).
func (r *Repository) ListCommitsSinceRef(refName string) ([]Commit, error) {
	head, err := r.raw.Head()
	if err != nil {
		return nil, fmt.Errorf("failed to get HEAD: %w", err)
	}

	baseHash, err := r.resolveRefToCommit(refName)
	if err != nil {
		return nil, err
	}

	// Build the exclusion set: every commit reachable from the base ref.
	// A stop-hash walk is not sufficient here — on merge commits the base
	// history is reachable via the second parent and would leak past a
	// single stop point.
	excluded := make(map[plumbing.Hash]struct{})
	baseIter, err := r.raw.Log(&gogit.LogOptions{From: baseHash})
	if err != nil {
		return nil, fmt.Errorf("failed to walk base ref %q: %w", refName, err)
	}
	err = baseIter.ForEach(func(c *object.Commit) error {
		excluded[c.Hash] = struct{}{}
		return nil
	})
	baseIter.Close()
	if err != nil {
		return nil, fmt.Errorf("failed to collect base commits: %w", err)
	}

	iter, err := r.raw.Log(&gogit.LogOptions{
		From:  head.Hash(),
		Order: gogit.LogOrderCommitterTime,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create log iterator: %w", err)
	}
	defer iter.Close()

	var commits []Commit
	err = iter.ForEach(func(c *object.Commit) error {
		if _, ok := excluded[c.Hash]; ok {
			// Reachable from base — skip, but keep walking: with
			// committer-time ordering, PR commits may interleave with
			// base commits.
			return nil
		}
		commits = append(commits, Commit{
			SHA:      c.Hash.String(),
			ShortSHA: c.Hash.String()[:7],
			Author:   c.Author.Name,
			Date:     c.Author.When,
			Message:  c.Message,
		})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to iterate commits: %w", err)
	}

	return commits, nil
}

// ListCommitsBetweenTags lists commits between two tags (exclusive of from, inclusive of to).
// If from is nil, returns all commits from to back to the repository root (bootstrap case).
// Returns commits in reverse-chronological order.
func (r *Repository) ListCommitsBetweenTags(from, to *Tag) ([]Commit, error) {
	toHash := plumbing.NewHash(to.TargetSHA())

	iter, err := r.raw.Log(&gogit.LogOptions{
		From:  toHash,
		Order: gogit.LogOrderCommitterTime,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create log iterator: %w", err)
	}
	defer iter.Close()

	var commits []Commit
	var fromHash plumbing.Hash
	if from != nil {
		fromHash = plumbing.NewHash(from.TargetSHA())
	}

	err = iter.ForEach(func(c *object.Commit) error {
		// Stop when we reach the from commit (exclusive); skip if from is nil (walk all)
		if from != nil && c.Hash == fromHash {
			return storer.ErrStop
		}

		commit := Commit{
			SHA:      c.Hash.String(),
			ShortSHA: c.Hash.String()[:7],
			Author:   c.Author.Name,
			Date:     c.Author.When,
			Message:  c.Message,
		}
		commits = append(commits, commit)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to iterate commits: %w", err)
	}

	return commits, nil
}

// FindTagByName looks up a tag by name.
// Supports both annotated and lightweight tags.
// Returns (nil, nil) if the tag does not exist.
func (r *Repository) FindTagByName(name string) (*Tag, error) {
	ref, err := r.raw.Tag(name)
	if err != nil {
		return nil, nil // tag does not exist
	}
	// Annotated tag: the ref points to a tag object
	obj, err := r.raw.TagObject(ref.Hash())
	if err == nil {
		return &Tag{
			Name:        name,
			SHA:         obj.Hash.String(),
			targetSHA:   obj.Target.String(),
			IsAnnotated: true,
		}, nil
	}
	// Lightweight tag: the ref points directly to a commit
	commit, err := r.raw.CommitObject(ref.Hash())
	if err != nil {
		return nil, nil // ref exists but is not a usable tag
	}
	return &Tag{
		Name:        name,
		SHA:         ref.Hash().String(),
		targetSHA:   commit.Hash.String(),
		IsAnnotated: false,
	}, nil
}

// CreateAnnotatedTag creates an annotated tag at HEAD.
// message: tag message (e.g., "chore(release): 1.0.0")
func (r *Repository) CreateAnnotatedTag(name, message string) (*Tag, error) {
	head, err := r.raw.Head()
	if err != nil {
		return nil, fmt.Errorf("failed to get HEAD: %w", err)
	}

	// Get the commit object to extract author info
	commit, err := r.raw.CommitObject(head.Hash())
	if err != nil {
		return nil, fmt.Errorf("failed to get commit object: %w", err)
	}

	// Create annotated tag
	ref, err := r.raw.CreateTag(name, head.Hash(), &gogit.CreateTagOptions{
		Tagger: &object.Signature{
			Name:  commit.Author.Name,
			Email: commit.Author.Email,
			When:  time.Now(),
		},
		Message: message,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create tag: %w", err)
	}

	// Get the tag object to extract SHA
	tagObj, err := r.raw.TagObject(ref.Hash())
	if err != nil {
		return nil, fmt.Errorf("failed to get tag object: %w", err)
	}

	tag := &Tag{
		Name:        name,
		SHA:         tagObj.Hash.String(),
		targetSHA:   tagObj.Target.String(),
		IsAnnotated: true,
	}

	return tag, nil
}

// PushTag pushes a tag to the remote repository.
// auth: BasicAuth{Username, Password} for HTTPS authentication
func (r *Repository) PushTag(ctx context.Context, tagName string, auth BasicAuth) error {
	// Create go-git BasicAuth from our BasicAuth
	gitAuth := &http.BasicAuth{
		Username: auth.Username,
		Password: auth.Password,
	}

	err := r.raw.PushContext(ctx, &gogit.PushOptions{
		RemoteName: "origin",
		RefSpecs: []config.RefSpec{
			config.RefSpec(fmt.Sprintf("refs/tags/%s:refs/tags/%s", tagName, tagName)),
		},
		Auth: gitAuth,
	})
	if err != nil {
		return fmt.Errorf("failed to push tag: %w", err)
	}

	return nil
}
