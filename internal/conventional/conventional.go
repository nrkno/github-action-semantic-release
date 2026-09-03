package conventional

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"
)

// CommitType represents the commit type
type CommitType string

// Valid commit types
const (
	TypeFeat     CommitType = "feat"
	TypeFix      CommitType = "fix"
	TypeChore    CommitType = "chore"
	TypeDocs     CommitType = "docs"
	TypeCI       CommitType = "ci"
	TypeRefactor CommitType = "refactor"
	TypeTest     CommitType = "test"
	TypePerf     CommitType = "perf"
	TypeBuild    CommitType = "build"
	TypeRevert   CommitType = "revert"
)

// Commit represents a parsed conventional commit
type Commit struct {
	Type        CommitType
	Scope       string // empty if not present
	Breaking    bool   // true if "!" or BREAKING CHANGE footer
	Description string
	Body        string
	Footers     string // raw footer text
	RawMessage  string // original commit message
	SHA         string // set by caller after parsing
	ShortSHA    string // first 7 chars of SHA
}

// Violation represents a validation failure
type Violation struct {
	SHA        string // commit SHA
	ShortSHA   string // first 7 chars
	RawMessage string // original message
	Rule       string // e.g., "invalid-type", "empty-description", "trailing-period"
	Example    string // corrected example
}

// RawCommit is input to parsing/validation
type RawCommit struct {
	SHA     string
	Message string
}

var validTypes = map[CommitType]bool{
	TypeFeat:     true,
	TypeFix:      true,
	TypeChore:    true,
	TypeDocs:     true,
	TypeCI:       true,
	TypeRefactor: true,
	TypeTest:     true,
	TypePerf:     true,
	TypeBuild:    true,
	TypeRevert:   true,
}

// Parse parses a single commit message and returns a Commit or error.
// Returns error only on structural unparseable input (e.g., completely empty).
// Violations are NOT errors — they are collected by ValidateAll.
func Parse(raw RawCommit) (Commit, error) {
	if strings.TrimSpace(raw.Message) == "" {
		return Commit{}, fmt.Errorf("empty commit message")
	}

	lines := strings.Split(raw.Message, "\n")
	firstLine := lines[0]

	commit := Commit{
		RawMessage: raw.Message,
		SHA:        raw.SHA,
	}

	if len(raw.SHA) >= 7 {
		commit.ShortSHA = raw.SHA[:7]
	}

	// Parse first line: type(scope)!: description
	if !strings.Contains(firstLine, ":") {
		return commit, nil // Will be caught by validation
	}

	colonIdx := strings.Index(firstLine, ":")
	header := firstLine[:colonIdx]
	descPart := firstLine[colonIdx+1:]

	// Extract description (trim leading space)
	commit.Description = strings.TrimLeft(descPart, " ")

	// Parse header: type(scope)! or type(scope) or type!
	breaking := strings.HasSuffix(header, "!")
	if breaking {
		header = header[:len(header)-1]
	}
	commit.Breaking = breaking

	// Parse type and scope
	if strings.Contains(header, "(") {
		parenIdx := strings.Index(header, "(")
		typeStr := header[:parenIdx]
		scopePart := header[parenIdx+1:]

		if strings.Contains(scopePart, ")") {
			closeParenIdx := strings.Index(scopePart, ")")
			commit.Scope = scopePart[:closeParenIdx]
		}

		commit.Type = CommitType(typeStr)
	} else {
		commit.Type = CommitType(header)
	}

	// Parse body and footers
	if len(lines) > 1 {
		// Find blank line separator
		bodyStart := -1
		for i := 1; i < len(lines); i++ {
			if strings.TrimSpace(lines[i]) == "" {
				bodyStart = i
				break
			}
		}

		if bodyStart == -1 {
			// No blank line, everything after first line is body
			commit.Body = strings.Join(lines[1:], "\n")
		} else {
			// Body is between first line and first blank line
			if bodyStart > 1 {
				commit.Body = strings.Join(lines[1:bodyStart], "\n")
			}

			// Footers are after the blank line
			if bodyStart+1 < len(lines) {
				commit.Footers = strings.Join(lines[bodyStart+1:], "\n")
				commit.Footers = strings.TrimSpace(commit.Footers)

				// Check for BREAKING CHANGE footer
				if strings.Contains(commit.Footers, "BREAKING CHANGE:") {
					commit.Breaking = true
				}
			}
		}
	}

	return commit, nil
}

// ValidateAll validates all commits and returns a slice of violations.
// Never stops at first violation — collects ALL.
// Returns empty slice (not nil) when all commits are valid.
// An optional LintOptions argument overrides the default rule set.
func ValidateAll(commits []RawCommit, opts ...LintOptions) []Violation {
	opt := DefaultLintOptions()
	if len(opts) > 0 {
		opt = opts[0]
	}

	// Use AllowedTypes if provided (len > 0); otherwise fall back to built-in validTypes.
	typeSet := validTypes
	if len(opt.AllowedTypes) > 0 {
		typeSet = make(map[CommitType]bool, len(opt.AllowedTypes))
		for _, t := range opt.AllowedTypes {
			typeSet[t] = true
		}
	}

	violations := []Violation{} // Initialize as empty slice, not nil

	for _, raw := range commits {
		// Skip merge commits
		if IsMergeCommit(raw.Message) {
			continue
		}

		commit, err := Parse(raw)
		if err != nil {
			// Structural error
			violations = append(violations, Violation{
				SHA:        raw.SHA,
				ShortSHA:   shortSHA(raw.SHA),
				RawMessage: raw.Message,
				Rule:       "parse-error",
				Example:    "feat: description",
			})
			continue
		}

		// Validate type
		if commit.Type == "" {
			violations = append(violations, Violation{
				SHA:        raw.SHA,
				ShortSHA:   shortSHA(raw.SHA),
				RawMessage: raw.Message,
				Rule:       "missing-type",
				Example:    "feat: " + commit.Description,
			})
		} else if !typeSet[commit.Type] {
			violations = append(violations, Violation{
				SHA:        raw.SHA,
				ShortSHA:   shortSHA(raw.SHA),
				RawMessage: raw.Message,
				Rule:       "invalid-type",
				Example:    "feat: " + commit.Description,
			})
		}

		// Validate description
		if commit.Description == "" {
			violations = append(violations, Violation{
				SHA:        raw.SHA,
				ShortSHA:   shortSHA(raw.SHA),
				RawMessage: raw.Message,
				Rule:       "empty-description",
				Example:    string(commit.Type) + ": description",
			})
		} else {
			// Check for trailing period
			if strings.HasSuffix(commit.Description, ".") {
				example := string(commit.Type)
				if commit.Scope != "" {
					example += "(" + commit.Scope + ")"
				}
				if commit.Breaking {
					example += "!"
				}
				example += ": " + strings.TrimSuffix(commit.Description, ".")

				violations = append(violations, Violation{
					SHA:        raw.SHA,
					ShortSHA:   shortSHA(raw.SHA),
					RawMessage: raw.Message,
					Rule:       "trailing-period",
					Example:    example,
				})
			}

			// Check for capital first letter (warning, but still a violation)
			if opt.CapitalFirstLetter && len(commit.Description) > 0 && unicode.IsUpper(rune(commit.Description[0])) {
				example := string(commit.Type)
				if commit.Scope != "" {
					example += "(" + commit.Scope + ")"
				}
				if commit.Breaking {
					example += "!"
				}
				example += ": " + strings.ToLower(commit.Description[:1]) + commit.Description[1:]

				violations = append(violations, Violation{
					SHA:        raw.SHA,
					ShortSHA:   shortSHA(raw.SHA),
					RawMessage: raw.Message,
					Rule:       "capital-first-letter",
					Example:    example,
				})
			}
		}

		if opt.RequireScope && commit.Scope == "" {
			violations = append(violations, Violation{
				SHA:        raw.SHA,
				ShortSHA:   shortSHA(raw.SHA),
				RawMessage: raw.Message,
				Rule:       "missing-scope",
				Example:    string(commit.Type) + "(scope): " + commit.Description,
			})
		}
	}

	return violations
}

// mergePattern matches GitHub Actions auto-generated merge commits:
// "Merge {sha} into {sha}" (generated when CI runs on a PR).
var mergePattern = regexp.MustCompile(`(?i)^Merge [0-9a-f]+ into [0-9a-f]+`)

// IsMergeCommit returns true for any merge-commit message variant:
//   - "Merge branch …"      (local git merge)
//   - "Merge pull request …" (GitHub PR merge)
//   - "Merge {sha} into {sha}" (GitHub Actions CI merge commit)
//   - "Merge remote-tracking branch …"
//   - "Merge refs/…"         (bulldozer / policy-bot branch updates)
func IsMergeCommit(message string) bool {
	trimmed := strings.TrimSpace(message)
	return strings.HasPrefix(trimmed, "Merge branch") ||
		strings.HasPrefix(trimmed, "Merge pull request") ||
		strings.HasPrefix(trimmed, "Merge remote-tracking branch") ||
		strings.HasPrefix(trimmed, "Merge refs/") ||
		mergePattern.MatchString(trimmed)
}

// shortSHA returns the first 7 characters of a SHA
func shortSHA(sha string) string {
	if len(sha) >= 7 {
		return sha[:7]
	}
	return sha
}
