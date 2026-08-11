package notes

import (
	"fmt"
	"strings"

	"github.com/nrkno/github-action-semantic-release/internal/conventional"
)

// PR represents a pull request link
type PR struct {
	Number int
	URL    string
}

// Section represents a group of commits by type (e.g., "Features", "Bug Fixes")
type Section struct {
	Title   string   // e.g., "Features", "Bug Fixes", "Breaking Changes"
	Commits []string // formatted commit lines
}

// ReleaseNotes represents the complete release notes
type ReleaseNotes struct {
	Sections []Section
	Body     string // markdown-formatted body
}

// sectionKey represents the internal ordering of sections
type sectionKey int

const (
	sectionBreaking sectionKey = 0
	sectionFeatures sectionKey = 1
	sectionBugFixes sectionKey = 2
	sectionOther    sectionKey = 3
)

// Generate creates release notes from a list of commits.
// prMap: map[commitSHA]PR for linking commits to PRs (pre-fetched by CLI layer)
// Returns ReleaseNotes with sections grouped by commit type, sorted by importance
func Generate(commits []conventional.Commit, prMap map[string]PR) ReleaseNotes {
	// Filter out merge commits and group by section
	sections := make(map[sectionKey][]string)

	for _, commit := range commits {
		// Skip merge commits
		if conventional.IsMergeCommit(commit.RawMessage) {
			continue
		}

		// Get PR link if available
		var pr *PR
		if p, ok := prMap[commit.SHA]; ok {
			pr = &p
		}

		// Format the commit line
		line := formatCommitLine(commit, pr)

		// Determine section
		var key sectionKey
		if commit.Breaking {
			key = sectionBreaking
		} else if commit.Type == conventional.TypeFeat {
			key = sectionFeatures
		} else if commit.Type == conventional.TypeFix {
			key = sectionBugFixes
		} else {
			key = sectionOther
		}

		sections[key] = append(sections[key], line)
	}

	// Build ordered sections
	var result []Section
	sectionOrder := []sectionKey{sectionBreaking, sectionFeatures, sectionBugFixes, sectionOther}

	for _, key := range sectionOrder {
		if lines, ok := sections[key]; ok && len(lines) > 0 {
			title := sectionTitle(key)
			result = append(result, Section{
				Title:   title,
				Commits: lines,
			})
		}
	}

	// Build markdown body
	body := buildMarkdownBody(result)

	return ReleaseNotes{
		Sections: result,
		Body:     body,
	}
}

// formatCommitLine formats a single commit line with optional PR link
// Example: "- feat: add new feature (#123)" or "- fix: bug fix"
func formatCommitLine(commit conventional.Commit, pr *PR) string {
	typeStr := string(commit.Type)
	if commit.Scope != "" {
		typeStr = fmt.Sprintf("%s(%s)", typeStr, commit.Scope)
	}

	line := fmt.Sprintf("- %s: %s", typeStr, commit.Description)

	if pr != nil {
		line = fmt.Sprintf("%s (#%d)", line, pr.Number)
	}

	return line
}

// sectionTitle returns the display title for a section
func sectionTitle(key sectionKey) string {
	switch key {
	case sectionBreaking:
		return "⚠️ Breaking Changes"
	case sectionFeatures:
		return "Features"
	case sectionBugFixes:
		return "Bug Fixes"
	case sectionOther:
		return "Other"
	default:
		return "Other"
	}
}

// buildMarkdownBody builds the markdown-formatted body from sections
func buildMarkdownBody(sections []Section) string {
	if len(sections) == 0 {
		return ""
	}

	var parts []string
	for _, section := range sections {
		parts = append(parts, fmt.Sprintf("## %s", section.Title))
		parts = append(parts, section.Commits...)
		parts = append(parts, "") // blank line between sections
	}

	// Join and trim trailing newlines
	body := strings.Join(parts, "\n")
	return strings.TrimSpace(body)
}
