package notes

import (
	"strings"
	"testing"

	"github.com/nrkno/github-action-semantic-release/internal/conventional"
)

func TestGenerate_EmptyCommits(t *testing.T) {
	commits := []conventional.Commit{}
	prMap := map[string]PR{}

	result := Generate(commits, prMap)

	if len(result.Sections) != 0 {
		t.Errorf("expected 0 sections, got %d", len(result.Sections))
	}
	if result.Body != "" {
		t.Errorf("expected empty body, got %q", result.Body)
	}
}

func TestGenerate_SingleFeat(t *testing.T) {
	commits := []conventional.Commit{
		{
			Type:        conventional.TypeFeat,
			Description: "add new feature",
			SHA:         "abc1234567890",
			RawMessage:  "feat: add new feature",
		},
	}
	prMap := map[string]PR{}

	result := Generate(commits, prMap)

	if len(result.Sections) != 1 {
		t.Errorf("expected 1 section, got %d", len(result.Sections))
	}
	if result.Sections[0].Title != "Features" {
		t.Errorf("expected title 'Features', got %q", result.Sections[0].Title)
	}
	if len(result.Sections[0].Commits) != 1 {
		t.Errorf("expected 1 commit, got %d", len(result.Sections[0].Commits))
	}
	if result.Sections[0].Commits[0] != "- feat: add new feature" {
		t.Errorf("expected '- feat: add new feature', got %q", result.Sections[0].Commits[0])
	}
}

func TestGenerate_SingleFix(t *testing.T) {
	commits := []conventional.Commit{
		{
			Type:        conventional.TypeFix,
			Description: "fix bug",
			SHA:         "abc1234567890",
			RawMessage:  "fix: fix bug",
		},
	}
	prMap := map[string]PR{}

	result := Generate(commits, prMap)

	if len(result.Sections) != 1 {
		t.Errorf("expected 1 section, got %d", len(result.Sections))
	}
	if result.Sections[0].Title != "Bug Fixes" {
		t.Errorf("expected title 'Bug Fixes', got %q", result.Sections[0].Title)
	}
	if len(result.Sections[0].Commits) != 1 {
		t.Errorf("expected 1 commit, got %d", len(result.Sections[0].Commits))
	}
}

func TestGenerate_MixedTypes(t *testing.T) {
	commits := []conventional.Commit{
		{
			Type:        conventional.TypeFeat,
			Description: "add feature 1",
			SHA:         "sha1",
			RawMessage:  "feat: add feature 1",
		},
		{
			Type:        conventional.TypeFix,
			Description: "fix bug",
			SHA:         "sha2",
			RawMessage:  "fix: fix bug",
		},
		{
			Type:        conventional.TypeFeat,
			Description: "add feature 2",
			SHA:         "sha3",
			RawMessage:  "feat: add feature 2",
		},
	}
	prMap := map[string]PR{}

	result := Generate(commits, prMap)

	if len(result.Sections) != 2 {
		t.Errorf("expected 2 sections, got %d", len(result.Sections))
	}

	// Features should come before Bug Fixes
	if result.Sections[0].Title != "Features" {
		t.Errorf("expected first section 'Features', got %q", result.Sections[0].Title)
	}
	if len(result.Sections[0].Commits) != 2 {
		t.Errorf("expected 2 features, got %d", len(result.Sections[0].Commits))
	}

	if result.Sections[1].Title != "Bug Fixes" {
		t.Errorf("expected second section 'Bug Fixes', got %q", result.Sections[1].Title)
	}
	if len(result.Sections[1].Commits) != 1 {
		t.Errorf("expected 1 bug fix, got %d", len(result.Sections[1].Commits))
	}
}

func TestGenerate_MergeCommitsOmitted(t *testing.T) {
	commits := []conventional.Commit{
		{
			Type:        conventional.TypeFeat,
			Description: "add feature",
			SHA:         "sha1",
			RawMessage:  "feat: add feature",
		},
		{
			Type:        conventional.TypeFeat,
			Description: "merge",
			SHA:         "sha2",
			RawMessage:  "Merge branch 'main' into develop",
		},
		{
			Type:        conventional.TypeFix,
			Description: "fix bug",
			SHA:         "sha3",
			RawMessage:  "fix: fix bug",
		},
	}
	prMap := map[string]PR{}

	result := Generate(commits, prMap)

	// Should have 2 sections (Features, Bug Fixes) with 1 item each
	if len(result.Sections) != 2 {
		t.Errorf("expected 2 sections, got %d", len(result.Sections))
	}
	if len(result.Sections[0].Commits) != 1 {
		t.Errorf("expected 1 feature, got %d", len(result.Sections[0].Commits))
	}
	if len(result.Sections[1].Commits) != 1 {
		t.Errorf("expected 1 bug fix, got %d", len(result.Sections[1].Commits))
	}
}

func TestGenerate_AllMergeCommits(t *testing.T) {
	commits := []conventional.Commit{
		{
			Type:        conventional.TypeFeat,
			Description: "merge",
			SHA:         "sha1",
			RawMessage:  "Merge branch 'main' into develop",
		},
		{
			Type:        conventional.TypeFeat,
			Description: "merge",
			SHA:         "sha2",
			RawMessage:  "Merge pull request #123 from user/branch",
		},
	}
	prMap := map[string]PR{}

	result := Generate(commits, prMap)

	if len(result.Sections) != 0 {
		t.Errorf("expected 0 sections, got %d", len(result.Sections))
	}
	if result.Body != "" {
		t.Errorf("expected empty body, got %q", result.Body)
	}
}

func TestGenerate_PRLinkInjection(t *testing.T) {
	commits := []conventional.Commit{
		{
			Type:        conventional.TypeFeat,
			Description: "add new feature",
			SHA:         "abc1234567890",
			RawMessage:  "feat: add new feature",
		},
	}
	prMap := map[string]PR{
		"abc1234567890": {Number: 123, URL: "https://github.com/org/repo/pull/123"},
	}

	result := Generate(commits, prMap)

	if len(result.Sections) != 1 {
		t.Errorf("expected 1 section, got %d", len(result.Sections))
	}
	if result.Sections[0].Commits[0] != "- feat: add new feature (#123)" {
		t.Errorf("expected '- feat: add new feature (#123)', got %q", result.Sections[0].Commits[0])
	}
}

func TestGenerate_NoPRLink(t *testing.T) {
	commits := []conventional.Commit{
		{
			Type:        conventional.TypeFeat,
			Description: "add new feature",
			SHA:         "abc1234567890",
			RawMessage:  "feat: add new feature",
		},
	}
	prMap := map[string]PR{}

	result := Generate(commits, prMap)

	if result.Sections[0].Commits[0] != "- feat: add new feature" {
		t.Errorf("expected '- feat: add new feature', got %q", result.Sections[0].Commits[0])
	}
}

func TestGenerate_BreakingChange(t *testing.T) {
	commits := []conventional.Commit{
		{
			Type:        conventional.TypeFeat,
			Description: "add breaking feature",
			Breaking:    true,
			SHA:         "sha1",
			RawMessage:  "feat!: add breaking feature",
		},
	}
	prMap := map[string]PR{}

	result := Generate(commits, prMap)

	if len(result.Sections) != 1 {
		t.Errorf("expected 1 section, got %d", len(result.Sections))
	}
	if result.Sections[0].Title != "⚠️ Breaking Changes" {
		t.Errorf("expected title '⚠️ Breaking Changes', got %q", result.Sections[0].Title)
	}
}

func TestGenerate_MultipleBreakingChanges(t *testing.T) {
	commits := []conventional.Commit{
		{
			Type:        conventional.TypeFeat,
			Description: "breaking feature 1",
			Breaking:    true,
			SHA:         "sha1",
			RawMessage:  "feat!: breaking feature 1",
		},
		{
			Type:        conventional.TypeFix,
			Description: "breaking fix",
			Breaking:    true,
			SHA:         "sha2",
			RawMessage:  "fix!: breaking fix",
		},
	}
	prMap := map[string]PR{}

	result := Generate(commits, prMap)

	if len(result.Sections) != 1 {
		t.Errorf("expected 1 section, got %d", len(result.Sections))
	}
	if result.Sections[0].Title != "⚠️ Breaking Changes" {
		t.Errorf("expected title '⚠️ Breaking Changes', got %q", result.Sections[0].Title)
	}
	if len(result.Sections[0].Commits) != 2 {
		t.Errorf("expected 2 commits, got %d", len(result.Sections[0].Commits))
	}
}

func TestGenerate_SectionOrdering(t *testing.T) {
	commits := []conventional.Commit{
		{
			Type:        conventional.TypeFix,
			Description: "fix bug",
			SHA:         "sha1",
			RawMessage:  "fix: fix bug",
		},
		{
			Type:        conventional.TypeFeat,
			Description: "add feature",
			Breaking:    true,
			SHA:         "sha2",
			RawMessage:  "feat!: add feature",
		},
		{
			Type:        conventional.TypeFeat,
			Description: "add feature",
			SHA:         "sha3",
			RawMessage:  "feat: add feature",
		},
	}
	prMap := map[string]PR{}

	result := Generate(commits, prMap)

	if len(result.Sections) != 3 {
		t.Errorf("expected 3 sections, got %d", len(result.Sections))
	}

	// Order should be: Breaking Changes, Features, Bug Fixes
	if result.Sections[0].Title != "⚠️ Breaking Changes" {
		t.Errorf("expected first section 'Breaking Changes', got %q", result.Sections[0].Title)
	}
	if result.Sections[1].Title != "Features" {
		t.Errorf("expected second section 'Features', got %q", result.Sections[1].Title)
	}
	if result.Sections[2].Title != "Bug Fixes" {
		t.Errorf("expected third section 'Bug Fixes', got %q", result.Sections[2].Title)
	}
}

func TestGenerate_CommitOrderWithinSection(t *testing.T) {
	commits := []conventional.Commit{
		{
			Type:        conventional.TypeFeat,
			Description: "feature 1",
			SHA:         "sha1",
			RawMessage:  "feat: feature 1",
		},
		{
			Type:        conventional.TypeFeat,
			Description: "feature 2",
			SHA:         "sha2",
			RawMessage:  "feat: feature 2",
		},
		{
			Type:        conventional.TypeFeat,
			Description: "feature 3",
			SHA:         "sha3",
			RawMessage:  "feat: feature 3",
		},
	}
	prMap := map[string]PR{}

	result := Generate(commits, prMap)

	if len(result.Sections[0].Commits) != 3 {
		t.Errorf("expected 3 commits, got %d", len(result.Sections[0].Commits))
	}

	// Check order is preserved
	if result.Sections[0].Commits[0] != "- feat: feature 1" {
		t.Errorf("expected first commit 'feature 1', got %q", result.Sections[0].Commits[0])
	}
	if result.Sections[0].Commits[1] != "- feat: feature 2" {
		t.Errorf("expected second commit 'feature 2', got %q", result.Sections[0].Commits[1])
	}
	if result.Sections[0].Commits[2] != "- feat: feature 3" {
		t.Errorf("expected third commit 'feature 3', got %q", result.Sections[0].Commits[2])
	}
}

func TestGenerate_MarkdownFormatting(t *testing.T) {
	commits := []conventional.Commit{
		{
			Type:        conventional.TypeFeat,
			Description: "add feature",
			SHA:         "sha1",
			RawMessage:  "feat: add feature",
		},
		{
			Type:        conventional.TypeFix,
			Description: "fix bug",
			SHA:         "sha2",
			RawMessage:  "fix: fix bug",
		},
	}
	prMap := map[string]PR{}

	result := Generate(commits, prMap)

	// Check body contains section headers
	if !strings.Contains(result.Body, "## Features") {
		t.Errorf("expected body to contain '## Features', got %q", result.Body)
	}
	if !strings.Contains(result.Body, "## Bug Fixes") {
		t.Errorf("expected body to contain '## Bug Fixes', got %q", result.Body)
	}

	// Check body contains commit lines
	if !strings.Contains(result.Body, "- feat: add feature") {
		t.Errorf("expected body to contain '- feat: add feature', got %q", result.Body)
	}
	if !strings.Contains(result.Body, "- fix: fix bug") {
		t.Errorf("expected body to contain '- fix: fix bug', got %q", result.Body)
	}
}

func TestGenerate_CommitWithScope(t *testing.T) {
	commits := []conventional.Commit{
		{
			Type:        conventional.TypeFeat,
			Scope:       "api",
			Description: "add endpoint",
			SHA:         "sha1",
			RawMessage:  "feat(api): add endpoint",
		},
	}
	prMap := map[string]PR{}

	result := Generate(commits, prMap)

	if result.Sections[0].Commits[0] != "- feat(api): add endpoint" {
		t.Errorf("expected '- feat(api): add endpoint', got %q", result.Sections[0].Commits[0])
	}
}

func TestGenerate_CommitWithScopeAndPR(t *testing.T) {
	commits := []conventional.Commit{
		{
			Type:        conventional.TypeFeat,
			Scope:       "api",
			Description: "add endpoint",
			SHA:         "sha1",
			RawMessage:  "feat(api): add endpoint",
		},
	}
	prMap := map[string]PR{
		"sha1": {Number: 42, URL: "https://github.com/org/repo/pull/42"},
	}

	result := Generate(commits, prMap)

	if result.Sections[0].Commits[0] != "- feat(api): add endpoint (#42)" {
		t.Errorf("expected '- feat(api): add endpoint (#42)', got %q", result.Sections[0].Commits[0])
	}
}

func TestGenerate_PartialPRMap(t *testing.T) {
	commits := []conventional.Commit{
		{
			Type:        conventional.TypeFeat,
			Description: "feature 1",
			SHA:         "sha1",
			RawMessage:  "feat: feature 1",
		},
		{
			Type:        conventional.TypeFeat,
			Description: "feature 2",
			SHA:         "sha2",
			RawMessage:  "feat: feature 2",
		},
	}
	prMap := map[string]PR{
		"sha1": {Number: 10, URL: "https://github.com/org/repo/pull/10"},
	}

	result := Generate(commits, prMap)

	if result.Sections[0].Commits[0] != "- feat: feature 1 (#10)" {
		t.Errorf("expected '- feat: feature 1 (#10)', got %q", result.Sections[0].Commits[0])
	}
	if result.Sections[0].Commits[1] != "- feat: feature 2" {
		t.Errorf("expected '- feat: feature 2', got %q", result.Sections[0].Commits[1])
	}
}

func TestGenerate_OtherCommitTypes(t *testing.T) {
	commits := []conventional.Commit{
		{
			Type:        conventional.TypeChore,
			Description: "update dependencies",
			SHA:         "sha1",
			RawMessage:  "chore: update dependencies",
		},
		{
			Type:        conventional.TypeDocs,
			Description: "update readme",
			SHA:         "sha2",
			RawMessage:  "docs: update readme",
		},
	}
	prMap := map[string]PR{}

	result := Generate(commits, prMap)

	// Other types should be in "Other" section (or omitted, depending on implementation)
	// Based on the spec, we should have an "Other" section
	if len(result.Sections) != 1 {
		t.Errorf("expected 1 section, got %d", len(result.Sections))
	}
	if result.Sections[0].Title != "Other" {
		t.Errorf("expected title 'Other', got %q", result.Sections[0].Title)
	}
	if len(result.Sections[0].Commits) != 2 {
		t.Errorf("expected 2 commits, got %d", len(result.Sections[0].Commits))
	}
}

func TestGenerate_SpecialCharactersInMessage(t *testing.T) {
	commits := []conventional.Commit{
		{
			Type:        conventional.TypeFeat,
			Description: "add feature (with parens) and special chars!",
			SHA:         "sha1",
			RawMessage:  "feat: add feature (with parens) and special chars!",
		},
	}
	prMap := map[string]PR{}

	result := Generate(commits, prMap)

	if result.Sections[0].Commits[0] != "- feat: add feature (with parens) and special chars!" {
		t.Errorf("expected message preserved as-is, got %q", result.Sections[0].Commits[0])
	}
}

func TestGenerate_EmptyPRMap(t *testing.T) {
	commits := []conventional.Commit{
		{
			Type:        conventional.TypeFeat,
			Description: "add feature",
			SHA:         "sha1",
			RawMessage:  "feat: add feature",
		},
	}
	prMap := map[string]PR{}

	result := Generate(commits, prMap)

	if result.Sections[0].Commits[0] != "- feat: add feature" {
		t.Errorf("expected no PR link, got %q", result.Sections[0].Commits[0])
	}
}

func TestGenerate_ComplexScenario(t *testing.T) {
	commits := []conventional.Commit{
		{
			Type:        conventional.TypeFeat,
			Scope:       "api",
			Description: "add new endpoint",
			Breaking:    true,
			SHA:         "sha1",
			RawMessage:  "feat(api)!: add new endpoint",
		},
		{
			Type:        conventional.TypeFeat,
			Description: "add feature",
			SHA:         "sha2",
			RawMessage:  "feat: add feature",
		},
		{
			Type:        conventional.TypeFix,
			Description: "fix critical bug",
			SHA:         "sha3",
			RawMessage:  "fix: fix critical bug",
		},
		{
			Type:        conventional.TypeFeat,
			Description: "merge",
			SHA:         "sha4",
			RawMessage:  "Merge branch 'develop'",
		},
		{
			Type:        conventional.TypeChore,
			Description: "update deps",
			SHA:         "sha5",
			RawMessage:  "chore: update deps",
		},
	}
	prMap := map[string]PR{
		"sha1": {Number: 100, URL: "https://github.com/org/repo/pull/100"},
		"sha2": {Number: 101, URL: "https://github.com/org/repo/pull/101"},
		"sha3": {Number: 102, URL: "https://github.com/org/repo/pull/102"},
	}

	result := Generate(commits, prMap)

	// Should have 4 sections: Breaking Changes, Features, Bug Fixes, Other
	if len(result.Sections) != 4 {
		t.Errorf("expected 4 sections, got %d", len(result.Sections))
	}

	// Check section order and content
	if result.Sections[0].Title != "⚠️ Breaking Changes" {
		t.Errorf("expected first section 'Breaking Changes', got %q", result.Sections[0].Title)
	}
	if len(result.Sections[0].Commits) != 1 {
		t.Errorf("expected 1 breaking change, got %d", len(result.Sections[0].Commits))
	}
	if result.Sections[0].Commits[0] != "- feat(api): add new endpoint (#100)" {
		t.Errorf("expected breaking change with PR link, got %q", result.Sections[0].Commits[0])
	}

	if result.Sections[1].Title != "Features" {
		t.Errorf("expected second section 'Features', got %q", result.Sections[1].Title)
	}
	if len(result.Sections[1].Commits) != 1 {
		t.Errorf("expected 1 feature, got %d", len(result.Sections[1].Commits))
	}

	if result.Sections[2].Title != "Bug Fixes" {
		t.Errorf("expected third section 'Bug Fixes', got %q", result.Sections[2].Title)
	}
	if len(result.Sections[2].Commits) != 1 {
		t.Errorf("expected 1 bug fix, got %d", len(result.Sections[2].Commits))
	}

	if result.Sections[3].Title != "Other" {
		t.Errorf("expected fourth section 'Other', got %q", result.Sections[3].Title)
	}
	if len(result.Sections[3].Commits) != 1 {
		t.Errorf("expected 1 other commit, got %d", len(result.Sections[3].Commits))
	}

	// Merge commit should not appear
	for _, section := range result.Sections {
		for _, commit := range section.Commits {
			if strings.Contains(commit, "merge") {
				t.Errorf("merge commit should not appear in output: %q", commit)
			}
		}
	}
}
