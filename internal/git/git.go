package git

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
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
	SHA      string // full commit SHA
	ShortSHA string // first 7 chars
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
	var excluded map[plumbing.Hash]bool
	if tag != nil {
		tagHash := plumbing.NewHash(tag.TargetSHA())
		tagObject, err := r.raw.Storer.EncodedObject(plumbing.CommitObject, tagHash)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve tag target: %w", err)
		}
		if tagObject.Type() != plumbing.CommitObject {
			return nil, fmt.Errorf("tag target %s is not a commit", tagHash)
		}
		// Preserve existing off-branch behavior: an unreachable selected tag falls
		// back to all HEAD history rather than changing release eligibility.
		ancestors, err := r.ancestorSet(tagHash)
		if err != nil {
			return nil, fmt.Errorf("failed to collect tag ancestry: %w", err)
		}
		// Determine reachability from tag side: HEAD belongs to set iff tag is its ancestor.
		headAncestors, err := r.ancestorSet(head.Hash())
		if err != nil {
			return nil, fmt.Errorf("failed to collect HEAD ancestry: %w", err)
		}
		if _, reachable := headAncestors[tagHash]; reachable {
			excluded = ancestors
		}
	}
	return r.commitsFromHead(head.Hash(), excluded)
}

// HeadSHA returns the hash recorded by HEAD without walking commit history.
func (r *Repository) HeadSHA() (string, error) {
	head, err := r.raw.Head()
	if err != nil {
		return "", fmt.Errorf("failed to get HEAD: %w", err)
	}
	return head.Hash().String(), nil
}

// commitParents reads only the tree and contiguous parent headers from a commit object.
func (r *Repository) commitParents(hash plumbing.Hash) (_ []plumbing.Hash, returnErr error) {
	obj, err := r.raw.Storer.EncodedObject(plumbing.CommitObject, hash)
	if err != nil {
		return nil, err
	}
	reader, err := obj.Reader()
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := reader.Close(); err != nil {
			returnErr = errors.Join(returnErr, fmt.Errorf("failed to close commit object reader: %w", err))
		}
	}()
	// Keep read-ahead small: the parser stops at the parent block, though the buffer
	// may prefetch a bounded prefix beyond that point.
	s := bufio.NewReaderSize(reader, 128)
	key, separator, err := readHeaderKey(s)
	if err != nil {
		return nil, err
	}
	if key != "tree" || separator != ' ' {
		return nil, fmt.Errorf("malformed commit %s: missing tree header", hash)
	}
	if _, err := readHeaderHash(s); err != nil {
		return nil, fmt.Errorf("malformed commit %s: invalid tree hash: %w", hash, err)
	}
	var parents []plumbing.Hash
	for {
		key, separator, err = readHeaderKey(s)
		if err == io.EOF && key == "parent" {
			return nil, fmt.Errorf("malformed commit %s: missing parent hash", hash)
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if key != "parent" {
			break
		}
		if separator != ' ' {
			return nil, fmt.Errorf("malformed commit %s: missing parent hash", hash)
		}
		parent, err := readHeaderHash(s)
		if err != nil {
			return nil, fmt.Errorf("malformed commit %s: invalid parent hash: %w", hash, err)
		}
		parents = append(parents, parent)
	}
	return parents, nil
}

// readHeaderKey reads only through the header key separator, never the value.
// Oversized unknown keys are treated as non-parent headers without consuming their payload.
func readHeaderKey(r *bufio.Reader) (string, byte, error) {
	key := make([]byte, 0, 16)
	for len(key) <= 32 {
		b, err := r.ReadByte()
		if err != nil {
			return string(key), 0, err
		}
		if b == ' ' || b == '\n' {
			return string(key), b, nil
		}
		key = append(key, b)
	}
	return string(key), 0, nil
}

func readHeaderHash(r *bufio.Reader) (plumbing.Hash, error) {
	value := make([]byte, 0, len(plumbing.ZeroHash.String()))
	for {
		b, err := r.ReadByte()
		if err == io.EOF {
			break
		}
		if err != nil {
			return plumbing.ZeroHash, err
		}
		if b == '\n' {
			break
		}
		value = append(value, b)
		if len(value) > len(plumbing.ZeroHash.String()) {
			return plumbing.ZeroHash, fmt.Errorf("invalid hash length")
		}
	}
	return parseHeaderHash(string(value))
}

func parseHeaderHash(value string) (plumbing.Hash, error) {
	if !plumbing.IsHash(value) {
		return plumbing.ZeroHash, fmt.Errorf("invalid object ID")
	}
	return plumbing.NewHash(value), nil
}

func (r *Repository) ancestorSet(start plumbing.Hash) (map[plumbing.Hash]bool, error) {
	seen := make(map[plumbing.Hash]bool)
	queue := []plumbing.Hash{start}
	for len(queue) > 0 {
		hash := queue[len(queue)-1]
		queue = queue[:len(queue)-1]
		if seen[hash] {
			continue
		}
		seen[hash] = true
		parents, err := r.commitParents(hash)
		if err != nil {
			return nil, fmt.Errorf("read commit %s parents: %w", hash, err)
		}
		queue = append(queue, parents...)
	}
	return seen, nil
}

func (r *Repository) commitsFromHead(head plumbing.Hash, excluded map[plumbing.Hash]bool) ([]Commit, error) {
	if excluded == nil {
		excluded = make(map[plumbing.Hash]bool)
	}
	if excluded[head] {
		return []Commit{}, nil
	}
	commit, err := r.raw.CommitObject(head)
	if err != nil {
		return nil, fmt.Errorf("failed to read HEAD commit: %w", err)
	}
	iter := object.NewCommitIterCTime(commit, nil, hashesFromSet(excluded))
	var commits []Commit
	err = iter.ForEach(func(c *object.Commit) error {
		commits = append(commits, Commit{SHA: c.Hash.String(), ShortSHA: c.Hash.String()[:7], Author: c.Author.Name, Date: c.Author.When, Message: c.Message})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to iterate commits: %w", err)
	}
	return commits, nil
}

func hashesFromSet(set map[plumbing.Hash]bool) []plumbing.Hash {
	hashes := make([]plumbing.Hash, 0, len(set))
	for hash := range set {
		hashes = append(hashes, hash)
	}
	return hashes
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
		"refs/tags/" + refName,
	}
	for _, candidate := range candidates {
		ref, err := r.raw.Reference(plumbing.ReferenceName(candidate), true)
		if err != nil {
			continue
		}
		hash := ref.Hash()
		if tagObj, tagErr := r.raw.TagObject(hash); tagErr == nil {
			hash = tagObj.Target
		}
		obj, err := r.raw.Storer.EncodedObject(plumbing.CommitObject, hash)
		if err != nil || obj.Type() != plumbing.CommitObject {
			return plumbing.ZeroHash, fmt.Errorf("ref %q does not point to a commit", refName)
		}
		return hash, nil
	}
	// Preserve support for abbreviated SHAs and other go-git revision syntax,
	// while keeping ordinary ref resolution from decoding the full base commit.
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

	excluded, err := r.ancestorSet(baseHash)
	if err != nil {
		return nil, fmt.Errorf("failed to collect base commits: %w", err)
	}
	return r.commitsFromHead(head.Hash(), excluded)
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
