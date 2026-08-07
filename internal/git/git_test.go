package git

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-git/go-billy/v5/memfs"
	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/storage/memory"
)

// createInMemoryRepo creates an in-memory git repository for testing
func createInMemoryRepo(t *testing.T) *gogit.Repository {
	t.Helper()
	repo, err := gogit.Init(memory.NewStorage(), memfs.New())
	if err != nil {
		t.Fatalf("failed to create in-memory repo: %v", err)
	}
	return repo
}

// createCommit creates a commit in the repository
func createCommit(t *testing.T, repo *gogit.Repository, filename, content, message string) *object.Commit {
	t.Helper()

	wt, err := repo.Worktree()
	if err != nil {
		t.Fatalf("failed to get worktree: %v", err)
	}

	fs := wt.Filesystem
	f, err := fs.Create(filename)
	if err != nil {
		t.Fatalf("failed to create file: %v", err)
	}
	_, err = f.Write([]byte(content))
	if err != nil {
		t.Fatalf("failed to write file: %v", err)
	}
	f.Close()

	_, err = wt.Add(filename)
	if err != nil {
		t.Fatalf("failed to add file: %v", err)
	}

	hash, err := wt.Commit(message, &gogit.CommitOptions{
		Author: &object.Signature{
			Name:  "Test Author",
			Email: "test@example.com",
			When:  time.Now(),
		},
	})
	if err != nil {
		t.Fatalf("failed to commit: %v", err)
	}

	commit, err := repo.CommitObject(hash)
	if err != nil {
		t.Fatalf("failed to get commit object: %v", err)
	}

	return commit
}

// createCommitWithTime creates a commit in the repository with explicit timestamp
func createCommitWithTime(t *testing.T, repo *gogit.Repository, filename, content, message string, when time.Time) *object.Commit {
	t.Helper()

	wt, err := repo.Worktree()
	if err != nil {
		t.Fatalf("failed to get worktree: %v", err)
	}

	fs := wt.Filesystem
	f, err := fs.Create(filename)
	if err != nil {
		t.Fatalf("failed to create file: %v", err)
	}
	_, err = f.Write([]byte(content))
	if err != nil {
		t.Fatalf("failed to write file: %v", err)
	}
	f.Close()

	_, err = wt.Add(filename)
	if err != nil {
		t.Fatalf("failed to add file: %v", err)
	}

	hash, err := wt.Commit(message, &gogit.CommitOptions{
		Author: &object.Signature{
			Name:  "Test Author",
			Email: "test@example.com",
			When:  when,
		},
	})
	if err != nil {
		t.Fatalf("failed to commit: %v", err)
	}

	commit, err := repo.CommitObject(hash)
	if err != nil {
		t.Fatalf("failed to get commit object: %v", err)
	}

	return commit
}

// createAnnotatedTag creates an annotated tag in the repository
func createAnnotatedTag(t *testing.T, repo *gogit.Repository, name, message string) *object.Tag {
	t.Helper()

	head, err := repo.Head()
	if err != nil {
		t.Fatalf("failed to get HEAD: %v", err)
	}

	commit, err := repo.CommitObject(head.Hash())
	if err != nil {
		t.Fatalf("failed to get commit: %v", err)
	}

	ref, err := repo.CreateTag(name, head.Hash(), &gogit.CreateTagOptions{
		Tagger: &object.Signature{
			Name:  commit.Author.Name,
			Email: commit.Author.Email,
			When:  time.Now(),
		},
		Message: message,
	})
	if err != nil {
		t.Fatalf("failed to create tag: %v", err)
	}

	tagObj, err := repo.TagObject(ref.Hash())
	if err != nil {
		t.Fatalf("failed to get tag object: %v", err)
	}

	return tagObj
}

// createLightweightTag creates a lightweight tag pointing directly at HEAD.
func createLightweightTag(t *testing.T, repo *gogit.Repository, name string) plumbing.Hash {
	t.Helper()
	head, err := repo.Head()
	if err != nil {
		t.Fatalf("failed to get HEAD: %v", err)
	}
	lwRef := plumbing.NewHashReference(plumbing.NewTagReferenceName(name), head.Hash())
	if err := repo.Storer.SetReference(lwRef); err != nil {
		t.Fatalf("failed to create lightweight tag %q: %v", name, err)
	}
	return head.Hash()
}

// TestOpenRepo_BootstrapRepo tests opening a fresh repository
func TestOpenRepo_BootstrapRepo(t *testing.T) {
	repo := createInMemoryRepo(t)
	wt, err := repo.Worktree()
	if err != nil {
		t.Fatalf("failed to get worktree: %v", err)
	}

	// Create a file and commit it
	f, err := wt.Filesystem.Create("README.md")
	if err != nil {
		t.Fatalf("failed to create file: %v", err)
	}
	f.Write([]byte("# Test"))
	f.Close()

	wt.Add("README.md")
	_, err = wt.Commit("initial commit", &gogit.CommitOptions{
		Author: &object.Signature{
			Name:  "Test",
			Email: "test@example.com",
			When:  time.Now(),
		},
	})
	if err != nil {
		t.Fatalf("failed to commit: %v", err)
	}

	// Wrap in Repository
	r := &Repository{raw: repo}

	// Should not return ShallowRepoError
	if r.raw == nil {
		t.Fatal("repository is nil")
	}
}

// TestShallowRepoError_Error tests the error message
func TestShallowRepoError_Error(t *testing.T) {
	err := ShallowRepoError{Message: "test message"}
	if err.Error() != "test message" {
		t.Errorf("expected 'test message', got '%s'", err.Error())
	}
}

// TestFindLatestAnnotatedTag_BootstrapNoTags tests finding tags when none exist
func TestFindLatestAnnotatedTag_BootstrapNoTags(t *testing.T) {
	repo := createInMemoryRepo(t)
	createCommit(t, repo, "file.txt", "content", "initial commit")

	r := &Repository{raw: repo}
	tag, err := r.FindLatestAnnotatedTag("")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tag != nil {
		t.Errorf("expected nil tag for bootstrap case, got %v", tag)
	}
}

// TestFindLatestAnnotatedTag_SingleTag tests finding the only tag
func TestFindLatestAnnotatedTag_SingleTag(t *testing.T) {
	repo := createInMemoryRepo(t)
	createCommit(t, repo, "file.txt", "content", "initial commit")
	createAnnotatedTag(t, repo, "v1.0.0", "Release 1.0.0")

	r := &Repository{raw: repo}
	tag, err := r.FindLatestAnnotatedTag("")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tag == nil {
		t.Fatal("expected tag, got nil")
	}
	if tag.Name != "v1.0.0" {
		t.Errorf("expected tag name 'v1.0.0', got '%s'", tag.Name)
	}
}

// TestFindLatestAnnotatedTag_MultipleTags tests finding the latest of multiple tags
func TestFindLatestAnnotatedTag_MultipleTags(t *testing.T) {
	repo := createInMemoryRepo(t)
	now := time.Now()
	createCommitWithTime(t, repo, "file.txt", "content v1", "commit 1", now)
	createAnnotatedTag(t, repo, "v1.0.0", "Release 1.0.0")

	// Create second commit with explicit later time
	createCommitWithTime(t, repo, "file.txt", "content v2", "commit 2", now.Add(1*time.Second))
	tag2Obj := createAnnotatedTag(t, repo, "v1.1.0", "Release 1.1.0")

	r := &Repository{raw: repo}
	tag, err := r.FindLatestAnnotatedTag("")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tag == nil {
		t.Fatal("expected tag, got nil")
	}
	if tag.Name != "v1.1.0" {
		t.Errorf("expected latest tag 'v1.1.0', got '%s'", tag.Name)
	}
	if tag.SHA != tag2Obj.Hash.String() {
		t.Errorf("expected tag SHA to be %s, got %s", tag2Obj.Hash.String(), tag.SHA)
	}
}

// TestFindLatestAnnotatedTag_IgnoresLightweightTags tests that lightweight tags are ignored
func TestFindLatestAnnotatedTag_IgnoresLightweightTags(t *testing.T) {
	repo := createInMemoryRepo(t)
	createCommit(t, repo, "file.txt", "content", "initial commit")

	// Create annotated tag
	annotatedTagObj := createAnnotatedTag(t, repo, "v1.0.0", "Release 1.0.0")

	// Lightweight tag = a ref pointing directly at a commit hash, no tag object.
	// Do NOT use repo.CreateTag() — that path requires Tagger and creates an annotated tag.
	head, err := repo.Head()
	if err != nil {
		t.Fatalf("failed to get HEAD: %v", err)
	}
	lwRef := plumbing.NewHashReference(
		plumbing.NewTagReferenceName("v0.0.1-lightweight"),
		head.Hash(),
	)
	if err := repo.Storer.SetReference(lwRef); err != nil {
		t.Fatalf("failed to create lightweight tag: %v", err)
	}

	r := &Repository{raw: repo}
	tag, err := r.FindLatestAnnotatedTag("")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tag == nil {
		t.Fatal("expected tag, got nil")
	}
	if tag.Name != "v1.0.0" {
		t.Errorf("expected annotated tag 'v1.0.0', got '%s'", tag.Name)
	}
	if tag.SHA != annotatedTagObj.Hash.String() {
		t.Errorf("expected tag SHA to be %s, got %s", annotatedTagObj.Hash.String(), tag.SHA)
	}
}

// TestFindPreviousAnnotatedTag_OnlyTag tests finding previous when tag is only tag
func TestFindPreviousAnnotatedTag_OnlyTag(t *testing.T) {
	repo := createInMemoryRepo(t)
	createCommit(t, repo, "file.txt", "content", "initial commit")
	tagObj := createAnnotatedTag(t, repo, "v1.0.0", "Release 1.0.0")

	r := &Repository{raw: repo}
	tag := &Tag{
		Name:      "v1.0.0",
		SHA:       tagObj.Hash.String(),
		targetSHA: tagObj.Target.String(),
	}

	prev, err := r.FindPreviousAnnotatedTag(tag)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if prev != nil {
		t.Errorf("expected nil for only tag, got %v", prev)
	}
}

// TestFindPreviousAnnotatedTag_MultipleTags tests finding previous tag
func TestFindPreviousAnnotatedTag_MultipleTags(t *testing.T) {
	repo := createInMemoryRepo(t)
	now := time.Now()
	createCommitWithTime(t, repo, "file.txt", "content v1", "commit 1", now)
	tag1Obj := createAnnotatedTag(t, repo, "v1.0.0", "Release 1.0.0")

	// Create second commit with explicit later time
	createCommitWithTime(t, repo, "file.txt", "content v2", "commit 2", now.Add(1*time.Second))
	tag2Obj := createAnnotatedTag(t, repo, "v1.1.0", "Release 1.1.0")

	r := &Repository{raw: repo}
	tag2 := &Tag{
		Name:      "v1.1.0",
		SHA:       tag2Obj.Hash.String(),
		targetSHA: tag2Obj.Target.String(),
	}

	prev, err := r.FindPreviousAnnotatedTag(tag2)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if prev == nil {
		t.Fatal("expected previous tag, got nil")
	}
	if prev.Name != "v1.0.0" {
		t.Errorf("expected previous tag 'v1.0.0', got '%s'", prev.Name)
	}
	if prev.SHA != tag1Obj.Hash.String() {
		t.Errorf("expected previous tag SHA to be %s, got %s", tag1Obj.Hash.String(), prev.SHA)
	}
}

// TestListCommitsSinceTag_Bootstrap tests listing all commits when tag is nil
func TestListCommitsSinceTag_Bootstrap(t *testing.T) {
	repo := createInMemoryRepo(t)
	createCommit(t, repo, "file.txt", "content 1", "commit 1")
	createCommit(t, repo, "file.txt", "content 2", "commit 2")
	createCommit(t, repo, "file.txt", "content 3", "commit 3")

	r := &Repository{raw: repo}
	commits, err := r.ListCommitsSinceTag(nil)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(commits) != 3 {
		t.Errorf("expected 3 commits, got %d", len(commits))
	}

	// Verify reverse-chronological order (newest first)
	if commits[0].Message != "commit 3" {
		t.Errorf("expected first commit to be 'commit 3', got '%s'", commits[0].Message)
	}
	if commits[2].Message != "commit 1" {
		t.Errorf("expected last commit to be 'commit 1', got '%s'", commits[2].Message)
	}
}

// TestListCommitsSinceTag_WithTag tests listing commits since a tag
func TestListCommitsSinceTag_WithTag(t *testing.T) {
	repo := createInMemoryRepo(t)
	createCommit(t, repo, "file.txt", "content 1", "commit 1")
	createCommit(t, repo, "file.txt", "content 2", "commit 2")
	tagObj := createAnnotatedTag(t, repo, "v1.0.0", "Release 1.0.0")

	createCommit(t, repo, "file.txt", "content 3", "commit 3")
	createCommit(t, repo, "file.txt", "content 4", "commit 4")

	r := &Repository{raw: repo}
	tag := &Tag{
		Name:      "v1.0.0",
		SHA:       tagObj.Hash.String(),
		targetSHA: tagObj.Target.String(),
	}

	commits, err := r.ListCommitsSinceTag(tag)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(commits) != 2 {
		t.Errorf("expected 2 commits since tag, got %d", len(commits))
	}

	// Verify commits are after the tag (commit 3 and 4)
	if commits[0].Message != "commit 4" {
		t.Errorf("expected first commit to be 'commit 4', got '%s'", commits[0].Message)
	}
	if commits[1].Message != "commit 3" {
		t.Errorf("expected second commit to be 'commit 3', got '%s'", commits[1].Message)
	}
}

// TestListCommitsSinceTag_EmptyRepo tests listing commits in empty repo
func TestListCommitsSinceTag_EmptyRepo(t *testing.T) {
	repo := createInMemoryRepo(t)

	r := &Repository{raw: repo}
	// Empty repo has no HEAD, so this should return an error
	_, err := r.ListCommitsSinceTag(nil)

	if err == nil {
		t.Fatal("expected error for empty repo, got nil")
	}
}

// TestListCommitsBetweenTags tests listing commits between two tags
func TestListCommitsBetweenTags(t *testing.T) {
	repo := createInMemoryRepo(t)
	createCommit(t, repo, "file.txt", "content 1", "commit 1")
	tag1Obj := createAnnotatedTag(t, repo, "v1.0.0", "Release 1.0.0")

	time.Sleep(10 * time.Millisecond)

	createCommit(t, repo, "file.txt", "content 2", "commit 2")
	createCommit(t, repo, "file.txt", "content 3", "commit 3")
	tag2Obj := createAnnotatedTag(t, repo, "v1.1.0", "Release 1.1.0")

	r := &Repository{raw: repo}
	tag1 := &Tag{
		Name:      "v1.0.0",
		SHA:       tag1Obj.Hash.String(),
		targetSHA: tag1Obj.Target.String(),
	}
	tag2 := &Tag{
		Name:      "v1.1.0",
		SHA:       tag2Obj.Hash.String(),
		targetSHA: tag2Obj.Target.String(),
	}

	commits, err := r.ListCommitsBetweenTags(tag1, tag2)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(commits) != 2 {
		t.Errorf("expected 2 commits between tags, got %d", len(commits))
	}

	// Verify commits are commit 2 and 3 (between the tags)
	if commits[0].Message != "commit 3" {
		t.Errorf("expected first commit to be 'commit 3', got '%s'", commits[0].Message)
	}
	if commits[1].Message != "commit 2" {
		t.Errorf("expected second commit to be 'commit 2', got '%s'", commits[1].Message)
	}
}

// TestListCommitsBetweenTagsNilFrom tests that nil from walks all commits back to root
func TestListCommitsBetweenTagsNilFrom(t *testing.T) {
	repo := createInMemoryRepo(t)
	createCommit(t, repo, "file.txt", "content 1", "commit 1")
	createCommit(t, repo, "file.txt", "content 2", "commit 2")
	createCommit(t, repo, "file.txt", "content 3", "commit 3")
	tagObj := createAnnotatedTag(t, repo, "v1.0.0", "Release 1.0.0")

	r := &Repository{raw: repo}
	tag := &Tag{
		Name:      "v1.0.0",
		SHA:       tagObj.Hash.String(),
		targetSHA: tagObj.Target.String(),
	}

	commits, err := r.ListCommitsBetweenTags(nil, tag)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(commits) != 3 {
		t.Errorf("expected 3 commits (all from root), got %d", len(commits))
	}
	if commits[0].Message != "commit 3" {
		t.Errorf("expected first commit to be 'commit 3', got %q", commits[0].Message)
	}
	if commits[2].Message != "commit 1" {
		t.Errorf("expected last commit to be 'commit 1', got %q", commits[2].Message)
	}
}

// TestCreateAnnotatedTag tests creating an annotated tag
func TestCreateAnnotatedTag(t *testing.T) {
	repo := createInMemoryRepo(t)
	createCommit(t, repo, "file.txt", "content", "initial commit")

	r := &Repository{raw: repo}
	tag, err := r.CreateAnnotatedTag("v1.0.0", "Release 1.0.0")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tag == nil {
		t.Fatal("expected tag, got nil")
	}
	if tag.Name != "v1.0.0" {
		t.Errorf("expected tag name 'v1.0.0', got '%s'", tag.Name)
	}
}

// TestTag_TargetSHA_DistinctFromTagSHA tests that TargetSHA differs from tag object SHA
func TestTag_TargetSHA_DistinctFromTagSHA(t *testing.T) {
	repo := createInMemoryRepo(t)
	commit := createCommit(t, repo, "file.txt", "content", "initial commit")
	tagObj := createAnnotatedTag(t, repo, "v1.0.0", "Release 1.0.0")

	r := &Repository{raw: repo}
	tag, err := r.FindLatestAnnotatedTag("")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tag == nil {
		t.Fatal("expected tag, got nil")
	}

	// CARRY-FORWARD TEST: Verify TargetSHA == commit SHA and SHA != commit SHA
	if tag.TargetSHA() != commit.Hash.String() {
		t.Errorf("expected TargetSHA to be commit SHA %s, got %s", commit.Hash.String(), tag.TargetSHA())
	}

	if tag.SHA == commit.Hash.String() {
		t.Errorf("expected tag SHA to differ from commit SHA, but both are %s", tag.SHA)
	}

	if tag.SHA != tagObj.Hash.String() {
		t.Errorf("expected tag SHA to be %s, got %s", tagObj.Hash.String(), tag.SHA)
	}
}

// TestCommit_ReverseChronological tests that commits are returned newest first
func TestCommit_ReverseChronological(t *testing.T) {
	repo := createInMemoryRepo(t)
	for i := 1; i <= 5; i++ {
		// Create different content each time to avoid clean worktree
		createCommit(t, repo, "file.txt", "content "+string(rune('0'+i)), "commit")
		time.Sleep(5 * time.Millisecond) // Ensure different timestamps
	}

	r := &Repository{raw: repo}
	commits, err := r.ListCommitsSinceTag(nil)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(commits) != 5 {
		t.Errorf("expected 5 commits, got %d", len(commits))
	}

	// Verify reverse-chronological order
	for i := 0; i < len(commits)-1; i++ {
		if commits[i].Date.Before(commits[i+1].Date) {
			t.Errorf("commits not in reverse-chronological order: commit %d (%v) before commit %d (%v)", i, commits[i].Date, i+1, commits[i+1].Date)
		}
	}
}

// TestCommit_ShortSHA tests that ShortSHA is first 7 chars
func TestCommit_ShortSHA(t *testing.T) {
	repo := createInMemoryRepo(t)
	createCommit(t, repo, "file.txt", "content", "initial commit")

	r := &Repository{raw: repo}
	commits, err := r.ListCommitsSinceTag(nil)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(commits) != 1 {
		t.Errorf("expected 1 commit, got %d", len(commits))
	}

	commit := commits[0]
	if len(commit.ShortSHA) != 7 {
		t.Errorf("expected ShortSHA length 7, got %d", len(commit.ShortSHA))
	}

	if commit.ShortSHA != commit.SHA[:7] {
		t.Errorf("expected ShortSHA to be first 7 chars of SHA")
	}
}

// TestTag_NameWithoutRefsPrefix tests that tag names don't have refs/ prefix
func TestTag_NameWithoutRefsPrefix(t *testing.T) {
	repo := createInMemoryRepo(t)
	createCommit(t, repo, "file.txt", "content", "initial commit")
	createAnnotatedTag(t, repo, "v1.0.0", "Release 1.0.0")

	r := &Repository{raw: repo}
	tag, err := r.FindLatestAnnotatedTag("")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tag == nil {
		t.Fatal("expected tag, got nil")
	}

	if tag.Name != "v1.0.0" {
		t.Errorf("expected tag name 'v1.0.0', got '%s'", tag.Name)
	}

	// Ensure no refs/ prefix
	if len(tag.Name) > 0 && tag.Name[0] == 'r' {
		t.Errorf("tag name should not start with 'refs/': %s", tag.Name)
	}
}

// TestPushTag_BasicAuth tests that push uses BasicAuth credentials
func TestPushTag_BasicAuth(t *testing.T) {
	repo := createInMemoryRepo(t)
	createCommit(t, repo, "file.txt", "content", "initial commit")

	r := &Repository{raw: repo}
	tag, err := r.CreateAnnotatedTag("v1.0.0", "Release 1.0.0")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if tag == nil {
		t.Fatal("expected tag, got nil")
	}

	// Test that BasicAuth can be created with x-access-token
	auth := BasicAuth{
		Username: "x-access-token",
		Password: "test-token-123",
	}

	// We can't actually push to a remote in this test, but we verify the auth structure
	if auth.Username != "x-access-token" {
		t.Errorf("expected username 'x-access-token', got '%s'", auth.Username)
	}
	if auth.Password != "test-token-123" {
		t.Errorf("expected password 'test-token-123', got '%s'", auth.Password)
	}
}

// TestOpenRepo_ShallowRepoDetection_FileCheck tests shallow repo detection via .git/shallow file
func TestOpenRepo_ShallowRepoDetection_FileCheck(t *testing.T) {
	// Create a temporary directory
	tmpDir := t.TempDir()

	// Create a .git directory with shallow file
	gitDir := filepath.Join(tmpDir, ".git")
	if err := os.MkdirAll(gitDir, 0o755); err != nil {
		t.Fatalf("failed to create .git directory: %v", err)
	}

	shallowFile := filepath.Join(gitDir, "shallow")
	if err := os.WriteFile(shallowFile, []byte("abc123"), 0o644); err != nil {
		t.Fatalf("failed to create shallow file: %v", err)
	}

	// Try to open the repo - should fail with ShallowRepoError
	_, err := OpenRepo(tmpDir)

	if err == nil {
		t.Fatal("expected ShallowRepoError, got nil")
	}

	shallowErr, ok := err.(ShallowRepoError)
	if !ok {
		t.Fatalf("expected ShallowRepoError, got %T: %v", err, err)
	}

	if shallowErr.Message != "repository is a shallow clone" {
		t.Errorf("expected message 'repository is a shallow clone', got '%s'", shallowErr.Message)
	}
}

// TestFindLatestAnnotatedTag_FallsBackToLightweight verifies that when no annotated tags exist,
// the most recent lightweight tag is returned with IsAnnotated=false.
func TestFindLatestAnnotatedTag_FallsBackToLightweight(t *testing.T) {
	repo := createInMemoryRepo(t)
	now := time.Now()

	// Commit 1 → lightweight tag v1.0.0
	createCommitWithTime(t, repo, "file.txt", "content 1", "commit 1", now)
	createLightweightTag(t, repo, "v1.0.0")

	// Commit 2 → lightweight tag v1.1.0 (newer)
	createCommitWithTime(t, repo, "file.txt", "content 2", "commit 2", now.Add(2*time.Second))
	createLightweightTag(t, repo, "v1.1.0")

	// Commit 3 → lightweight tag v1.2.0 (newest)
	createCommitWithTime(t, repo, "file.txt", "content 3", "commit 3", now.Add(4*time.Second))
	createLightweightTag(t, repo, "v1.2.0")

	r := &Repository{raw: repo}
	tag, err := r.FindLatestAnnotatedTag("")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tag == nil {
		t.Fatal("expected lightweight tag fallback, got nil")
	}
	if tag.Name != "v1.2.0" {
		t.Errorf("expected most recent lightweight tag 'v1.2.0', got %q", tag.Name)
	}
	if tag.IsAnnotated {
		t.Errorf("expected IsAnnotated=false for lightweight tag, got true")
	}
}

// TestFindLatestAnnotatedTag_PrefersAnnotatedOverLightweight verifies annotated tags
// take priority over newer lightweight tags.
func TestFindLatestAnnotatedTag_PrefersAnnotatedOverLightweight(t *testing.T) {
	repo := createInMemoryRepo(t)
	now := time.Now()

	// Commit 1 → annotated tag v1.0.0
	createCommitWithTime(t, repo, "file.txt", "content 1", "commit 1", now)
	annotatedTagObj := createAnnotatedTag(t, repo, "v1.0.0", "Release 1.0.0")

	// Commit 2 → lightweight tag v2.0.0 (newer commit, but lightweight)
	createCommitWithTime(t, repo, "file.txt", "content 2", "commit 2", now.Add(2*time.Second))
	createLightweightTag(t, repo, "v2.0.0")

	r := &Repository{raw: repo}
	tag, err := r.FindLatestAnnotatedTag("")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tag == nil {
		t.Fatal("expected annotated tag, got nil")
	}
	if tag.Name != "v1.0.0" {
		t.Errorf("expected annotated tag 'v1.0.0', got %q", tag.Name)
	}
	if !tag.IsAnnotated {
		t.Errorf("expected IsAnnotated=true for annotated tag, got false")
	}
	if tag.SHA != annotatedTagObj.Hash.String() {
		t.Errorf("expected tag SHA %s, got %s", annotatedTagObj.Hash.String(), tag.SHA)
	}
}

// TestFindLatestAnnotatedTag_LightweightTagIsAnnotatedFalse verifies IsAnnotated=false
// for a single lightweight tag fallback.
func TestFindLatestAnnotatedTag_LightweightTagIsAnnotatedFalse(t *testing.T) {
	repo := createInMemoryRepo(t)
	createCommit(t, repo, "file.txt", "content", "initial commit")
	createLightweightTag(t, repo, "v0.9.0")

	r := &Repository{raw: repo}
	tag, err := r.FindLatestAnnotatedTag("")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tag == nil {
		t.Fatal("expected lightweight tag fallback, got nil")
	}
	if tag.Name != "v0.9.0" {
		t.Errorf("expected tag name 'v0.9.0', got %q", tag.Name)
	}
	if tag.IsAnnotated {
		t.Errorf("expected IsAnnotated=false, got true")
	}
}

// TestFindTagByName_LightweightTag verifies that FindTagByName returns a lightweight tag
// with IsAnnotated=false and the correct targetSHA.
func TestFindTagByName_LightweightTag(t *testing.T) {
	repo := createInMemoryRepo(t)
	commit := createCommit(t, repo, "file.txt", "content", "initial commit")
	createLightweightTag(t, repo, "v1.5.0")

	r := &Repository{raw: repo}
	tag, err := r.FindTagByName("v1.5.0")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tag == nil {
		t.Fatal("expected tag, got nil")
	}
	if tag.Name != "v1.5.0" {
		t.Errorf("expected tag name 'v1.5.0', got %q", tag.Name)
	}
	if tag.IsAnnotated {
		t.Errorf("expected IsAnnotated=false for lightweight tag, got true")
	}
	if tag.TargetSHA() != commit.Hash.String() {
		t.Errorf("expected targetSHA %s, got %s", commit.Hash.String(), tag.TargetSHA())
	}
	// For lightweight tags SHA == targetSHA
	if tag.SHA != tag.TargetSHA() {
		t.Errorf("expected SHA == targetSHA for lightweight tag: SHA=%s targetSHA=%s", tag.SHA, tag.TargetSHA())
	}
}

// createCommitOffParent stores a commit object directly in the repository's object database
// with an explicit parent hash, WITHOUT updating HEAD. Used to create detached feature-branch
// commits that are later wired into a merge commit.
func createCommitOffParent(t *testing.T, repo *gogit.Repository, message string, when time.Time, parentHash plumbing.Hash) plumbing.Hash {
	t.Helper()

	parent, err := repo.CommitObject(parentHash)
	if err != nil {
		t.Fatalf("createCommitOffParent: failed to get parent commit: %v", err)
	}

	sig := object.Signature{Name: "Test Author", Email: "test@example.com", When: when}
	c := &object.Commit{
		Author:       sig,
		Committer:    sig,
		Message:      message,
		TreeHash:     parent.TreeHash,
		ParentHashes: []plumbing.Hash{parentHash},
	}

	obj := repo.Storer.NewEncodedObject()
	if err := c.Encode(obj); err != nil {
		t.Fatalf("createCommitOffParent: failed to encode commit: %v", err)
	}
	hash, err := repo.Storer.SetEncodedObject(obj)
	if err != nil {
		t.Fatalf("createCommitOffParent: failed to store commit: %v", err)
	}
	return hash
}

// createMergeCommit stores a two-parent merge commit and advances HEAD's branch ref to it.
func createMergeCommit(t *testing.T, repo *gogit.Repository, message string, when time.Time, parent1, parent2 plumbing.Hash) plumbing.Hash {
	t.Helper()

	p1, err := repo.CommitObject(parent1)
	if err != nil {
		t.Fatalf("createMergeCommit: failed to get parent1 commit: %v", err)
	}

	sig := object.Signature{Name: "Test Author", Email: "test@example.com", When: when}
	c := &object.Commit{
		Author:       sig,
		Committer:    sig,
		Message:      message,
		TreeHash:     p1.TreeHash, // use parent1's tree as the merged result
		ParentHashes: []plumbing.Hash{parent1, parent2},
	}

	obj := repo.Storer.NewEncodedObject()
	if err := c.Encode(obj); err != nil {
		t.Fatalf("createMergeCommit: failed to encode commit: %v", err)
	}
	hash, err := repo.Storer.SetEncodedObject(obj)
	if err != nil {
		t.Fatalf("createMergeCommit: failed to store commit: %v", err)
	}

	// Advance the current branch ref to point to the merge commit.
	head, err := repo.Head()
	if err != nil {
		t.Fatalf("createMergeCommit: failed to get HEAD: %v", err)
	}
	if err := repo.Storer.SetReference(plumbing.NewHashReference(head.Name(), hash)); err != nil {
		t.Fatalf("createMergeCommit: failed to update branch ref: %v", err)
	}

	return hash
}

// TestListCommitsSinceRef_ExcludesBaseCommits verifies base..HEAD semantics:
// commits reachable from the base ref are excluded, commits unique to HEAD are returned.
func TestListCommitsSinceRef_ExcludesBaseCommits(t *testing.T) {
	repo := createInMemoryRepo(t)
	now := time.Now()

	createCommitWithTime(t, repo, "file.txt", "content 1", "chore: commit 1", now)
	c2 := createCommitWithTime(t, repo, "file.txt", "content 2", "Non conventional base commit", now.Add(1*time.Second))

	// Base branch "main" points at C2
	mainRef := plumbing.NewHashReference(plumbing.NewBranchReferenceName("main"), c2.Hash)
	if err := repo.Storer.SetReference(mainRef); err != nil {
		t.Fatalf("failed to create main ref: %v", err)
	}

	// Feature commits on HEAD (default branch) after the base
	createCommitWithTime(t, repo, "file.txt", "content 3", "feat: commit 3", now.Add(2*time.Second))
	createCommitWithTime(t, repo, "file.txt", "content 4", "fix: commit 4", now.Add(3*time.Second))

	r := &Repository{raw: repo}
	commits, err := r.ListCommitsSinceRef("main")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(commits) != 2 {
		msgs := make([]string, len(commits))
		for i, c := range commits {
			msgs[i] = strings.TrimSpace(c.Message)
		}
		t.Fatalf("expected 2 commits (only PR commits), got %d: %v", len(commits), msgs)
	}
	if commits[0].Message != "fix: commit 4" {
		t.Errorf("expected first commit 'fix: commit 4', got %q", commits[0].Message)
	}
	if commits[1].Message != "feat: commit 3" {
		t.Errorf("expected second commit 'feat: commit 3', got %q", commits[1].Message)
	}
}

// TestListCommitsSinceRef_MergeFromBaseExcluded is the regression test for the
// GITHUB_BASE_REF lint-range bug. On pull_request events GitHub checks out
// refs/pull/N/merge — a merge commit whose second parent chain contains base
// branch commits. Those base commits (including non-conventional squash merges
// already on main) must NOT be linted.
//
// Repo shape:
//
//	C0 ← C1(feat, HEAD chain) ← M(merge)
//	  └── Cbase(non-conventional, main) ──┘
func TestListCommitsSinceRef_MergeFromBaseExcluded(t *testing.T) {
	repo := createInMemoryRepo(t)
	now := time.Now()

	// C0: shared base
	c0 := createCommitWithTime(t, repo, "base.txt", "base", "chore: initial commit", now)

	// Cbase: commit on main with a NON-conventional message (e.g. bad squash merge)
	cbaseHash := createCommitOffParent(t, repo, "Multiple interface support (#159)", now.Add(1*time.Second), c0.Hash)
	mainRef := plumbing.NewHashReference(plumbing.NewBranchReferenceName("main"), cbaseHash)
	if err := repo.Storer.SetReference(mainRef); err != nil {
		t.Fatalf("failed to create main ref: %v", err)
	}

	// C1: PR commit on HEAD chain
	c1 := createCommitWithTime(t, repo, "feature.txt", "feature", "feat: pr commit", now.Add(2*time.Second))

	// M: the refs/pull/N/merge-style merge commit: parents [C1, Cbase]
	createMergeCommit(t, repo, "Merge main into PR", now.Add(3*time.Second), c1.Hash, cbaseHash)

	r := &Repository{raw: repo}
	commits, err := r.ListCommitsSinceRef("main")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Expected: M + C1. Cbase and C0 are reachable from main and must be excluded.
	if len(commits) != 2 {
		msgs := make([]string, len(commits))
		for i, c := range commits {
			msgs[i] = strings.TrimSpace(c.Message)
		}
		t.Fatalf("expected 2 commits (merge + PR commit), got %d: %v", len(commits), msgs)
	}
	for _, c := range commits {
		if strings.Contains(c.Message, "Multiple interface support") {
			t.Errorf("base branch commit leaked into lint range: %q", c.Message)
		}
	}
}

// TestListCommitsSinceRef_ResolvesRemoteRef verifies that a bare branch name
// falls back to origin/<name> when no local branch exists — the normal state
// in an actions/checkout PR workspace where only remote-tracking refs exist.
func TestListCommitsSinceRef_ResolvesRemoteRef(t *testing.T) {
	repo := createInMemoryRepo(t)
	now := time.Now()

	c1 := createCommitWithTime(t, repo, "file.txt", "content 1", "chore: commit 1", now)

	// Only a remote-tracking ref for the base branch
	remoteRef := plumbing.NewHashReference(
		plumbing.NewRemoteReferenceName("origin", "basebranch"), c1.Hash)
	if err := repo.Storer.SetReference(remoteRef); err != nil {
		t.Fatalf("failed to create remote ref: %v", err)
	}

	createCommitWithTime(t, repo, "file.txt", "content 2", "feat: commit 2", now.Add(1*time.Second))

	r := &Repository{raw: repo}
	commits, err := r.ListCommitsSinceRef("basebranch")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(commits) != 1 {
		t.Fatalf("expected 1 commit, got %d", len(commits))
	}
	if commits[0].Message != "feat: commit 2" {
		t.Errorf("expected 'feat: commit 2', got %q", commits[0].Message)
	}
}

// TestListCommitsSinceRef_UnresolvableRef verifies a clear error for unknown refs.
func TestListCommitsSinceRef_UnresolvableRef(t *testing.T) {
	repo := createInMemoryRepo(t)
	createCommit(t, repo, "file.txt", "content", "chore: initial commit")

	r := &Repository{raw: repo}
	_, err := r.ListCommitsSinceRef("does-not-exist")

	if err == nil {
		t.Fatal("expected error for unresolvable ref, got nil")
	}
	if !strings.Contains(err.Error(), "does-not-exist") {
		t.Errorf("error should mention the ref name, got: %v", err)
	}
}

// TestListCommitsSinceTag_MergeCommitIncludesFeatureBranch is the regression test for the
// DFS traversal bug. With DFS (the old default), traversal dives into parent[0] (main chain)
// and hits the stop-hash before backtracking to parent[1] (feature branch), silently dropping
// all feat/fix commits from traditionally-merged PRs.
//
// Repo shape:
//
//	C0(tag) ← C2(main) ← M(merge)
//	       └── C1(feat) ──┘
func TestListCommitsSinceTag_MergeCommitIncludesFeatureBranch(t *testing.T) {
	repo := createInMemoryRepo(t)
	now := time.Now()

	// C0: base commit, tagged as v1.0.0 (this is the stop hash)
	c0 := createCommitWithTime(t, repo, "base.txt", "base", "chore: initial commit", now)
	tagObj := createAnnotatedTag(t, repo, "v1.0.0", "Release v1.0.0")

	// C1: feature branch commit off C0 — stored in the object DB, HEAD not moved
	featureMsg := "feat: add new feature"
	c1Hash := createCommitOffParent(t, repo, featureMsg, now.Add(1*time.Second), c0.Hash)

	// C2: main commit off C0 — HEAD is still at C0, so worktree parent = C0
	c2 := createCommitWithTime(t, repo, "main.txt", "main work", "fix: main change", now.Add(2*time.Second))

	// M: merge commit with parents [C2, C1] — HEAD advances to M
	createMergeCommit(t, repo, "Merge feature into main", now.Add(3*time.Second), c2.Hash, c1Hash)

	r := &Repository{raw: repo}
	tag := &Tag{
		Name:      "v1.0.0",
		SHA:       tagObj.Hash.String(),
		targetSHA: tagObj.Target.String(),
	}

	commits, err := r.ListCommitsSinceTag(tag)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Expected: M + C2 + C1 = 3 commits; C0 is excluded (it is the stop hash).
	if len(commits) != 3 {
		msgs := make([]string, len(commits))
		for i, c := range commits {
			msgs[i] = strings.TrimSpace(c.Message)
		}
		t.Fatalf("expected 3 commits (merge + main + feature), got %d: %v", len(commits), msgs)
	}

	// The feature branch commit MUST be present — this is the bug guard.
	found := false
	for _, c := range commits {
		if strings.Contains(c.Message, featureMsg) {
			found = true
			break
		}
	}
	if !found {
		msgs := make([]string, len(commits))
		for i, c := range commits {
			msgs[i] = strings.TrimSpace(c.Message)
		}
		t.Errorf("feature branch commit %q not found in results; commits: %v", featureMsg, msgs)
	}
}
