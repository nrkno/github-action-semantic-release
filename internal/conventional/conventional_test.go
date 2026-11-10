package conventional

import (
	"testing"
)

func TestValidateAll_AllowedTypes_CustomTypeAllowed(t *testing.T) {
	commits := []RawCommit{{SHA: "abc1234567890", Message: "custom: foo"}}
	opts := LintOptions{
		CapitalFirstLetter: false,
		RequireScope:       false,
		AllowedTypes:       []CommitType{"feat", "fix", "custom"},
	}
	violations := ValidateAll(commits, opts)
	if len(violations) != 0 {
		t.Errorf("ValidateAll() returned %d violations, want 0; violations: %v", len(violations), violations)
	}
}

func TestValidateAll_AllowedTypes_UnknownTypeRejected(t *testing.T) {
	commits := []RawCommit{{SHA: "abc1234567890", Message: "chore: foo"}}
	opts := LintOptions{
		CapitalFirstLetter: false,
		RequireScope:       false,
		AllowedTypes:       []CommitType{"feat"},
	}
	violations := ValidateAll(commits, opts)
	if len(violations) != 1 {
		t.Fatalf("ValidateAll() returned %d violations, want 1", len(violations))
	}
	if violations[0].Rule != "invalid-type" {
		t.Errorf("Rule = %q, want invalid-type", violations[0].Rule)
	}
}

func TestValidateAll_AllowedTypes_Empty_UsesBuiltins(t *testing.T) {
	// AllowedTypes=nil → falls back to built-in validTypes
	commits := []RawCommit{
		{SHA: "abc1234567890", Message: "feat: add login"},
		{SHA: "def1234567890", Message: "custom: should fail"},
	}
	opts := LintOptions{
		CapitalFirstLetter: false,
		RequireScope:       false,
		AllowedTypes:       nil,
	}
	violations := ValidateAll(commits, opts)
	if len(violations) != 1 {
		t.Fatalf("ValidateAll() returned %d violations, want 1", len(violations))
	}
	if violations[0].Rule != "invalid-type" {
		t.Errorf("Rule = %q, want invalid-type", violations[0].Rule)
	}
}

func TestParse_ValidCommits(t *testing.T) {
	tests := []struct {
		name     string
		raw      RawCommit
		wantType CommitType
		wantDesc string
		wantScope string
		wantBreak bool
	}{
		{
			name: "simple feat",
			raw: RawCommit{
				SHA:     "abc1234567890def",
				Message: "feat: add login",
			},
			wantType: TypeFeat,
			wantDesc: "add login",
			wantScope: "",
			wantBreak: false,
		},
		{
			name: "fix with scope",
			raw: RawCommit{
				SHA:     "abc1234567890def",
				Message: "fix(auth): null pointer",
			},
			wantType: TypeFix,
			wantDesc: "null pointer",
			wantScope: "auth",
			wantBreak: false,
		},
		{
			name: "breaking with exclamation",
			raw: RawCommit{
				SHA:     "abc1234567890def",
				Message: "chore!: drop go 1.20",
			},
			wantType: TypeChore,
			wantDesc: "drop go 1.20",
			wantScope: "",
			wantBreak: true,
		},
		{
			name: "breaking with scope and exclamation",
			raw: RawCommit{
				SHA:     "abc1234567890def",
				Message: "feat(api)!: new endpoint",
			},
			wantType: TypeFeat,
			wantDesc: "new endpoint",
			wantScope: "api",
			wantBreak: true,
		},
		{
			name: "revert commit",
			raw: RawCommit{
				SHA:     "abc1234567890def",
				Message: "revert: feat: old feature",
			},
			wantType: TypeRevert,
			wantDesc: "feat: old feature",
			wantScope: "",
			wantBreak: false,
		},
		{
			name: "docs type",
			raw: RawCommit{
				SHA:     "abc1234567890def",
				Message: "docs: update readme",
			},
			wantType: TypeDocs,
			wantDesc: "update readme",
			wantScope: "",
			wantBreak: false,
		},
		{
			name: "ci type",
			raw: RawCommit{
				SHA:     "abc1234567890def",
				Message: "ci: add github action",
			},
			wantType: TypeCI,
			wantDesc: "add github action",
			wantScope: "",
			wantBreak: false,
		},
		{
			name: "refactor with scope",
			raw: RawCommit{
				SHA:     "abc1234567890def",
				Message: "refactor(parser): simplify logic",
			},
			wantType: TypeRefactor,
			wantDesc: "simplify logic",
			wantScope: "parser",
			wantBreak: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Parse(tt.raw)
			if err != nil {
				t.Fatalf("Parse() error = %v, want nil", err)
			}
			if got.Type != tt.wantType {
				t.Errorf("Type = %v, want %v", got.Type, tt.wantType)
			}
			if got.Description != tt.wantDesc {
				t.Errorf("Description = %q, want %q", got.Description, tt.wantDesc)
			}
			if got.Scope != tt.wantScope {
				t.Errorf("Scope = %q, want %q", got.Scope, tt.wantScope)
			}
			if got.Breaking != tt.wantBreak {
				t.Errorf("Breaking = %v, want %v", got.Breaking, tt.wantBreak)
			}
		})
	}
}

func TestParse_MultilineCommits(t *testing.T) {
	tests := []struct {
		name      string
		raw       RawCommit
		wantType  CommitType
		wantDesc  string
		wantBody  string
		wantBreak bool
		wantFooters string
	}{
		{
			name: "with body",
			raw: RawCommit{
				SHA: "abc1234567890def",
				Message: `feat: add login

This adds a new login feature with OAuth support.`,
			},
			wantType: TypeFeat,
			wantDesc: "add login",
			wantBody: "This adds a new login feature with OAuth support.",
			wantBreak: false,
		},
		{
			name: "with body and footers",
			raw: RawCommit{
				SHA: "abc1234567890def",
				Message: `feat: add login

This adds a new login feature.

Reviewed-by: John Doe
Fixes: #123`,
			},
			wantType: TypeFeat,
			wantDesc: "add login",
			wantBody: "This adds a new login feature.",
			wantBreak: false,
		},
		{
			name: "with BREAKING CHANGE footer",
			raw: RawCommit{
				SHA: "abc1234567890def",
				Message: `fix: typo

BREAKING CHANGE: removes X`,
			},
			wantType: TypeFix,
			wantDesc: "typo",
			wantBreak: true,
		},
		{
			name: "with BREAKING CHANGE and body",
			raw: RawCommit{
				SHA: "abc1234567890def",
				Message: `feat: new API

This is a breaking change.

BREAKING CHANGE: api change`,
			},
			wantType: TypeFeat,
			wantDesc: "new API",
			wantBreak: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Parse(tt.raw)
			if err != nil {
				t.Fatalf("Parse() error = %v, want nil", err)
			}
			if got.Type != tt.wantType {
				t.Errorf("Type = %v, want %v", got.Type, tt.wantType)
			}
			if got.Description != tt.wantDesc {
				t.Errorf("Description = %q, want %q", got.Description, tt.wantDesc)
			}
			if got.Breaking != tt.wantBreak {
				t.Errorf("Breaking = %v, want %v", got.Breaking, tt.wantBreak)
			}
		})
	}
}

func TestValidateAll_InvalidType(t *testing.T) {
	tests := []struct {
		name    string
		commits []RawCommit
		wantLen int
	}{
		{
			name: "invalid type feature",
			commits: []RawCommit{
				{SHA: "abc1234567890def", Message: "feature: add"},
			},
			wantLen: 1,
		},
		{
			name: "invalid type FIX uppercase",
			commits: []RawCommit{
				{SHA: "abc1234567890def", Message: "FIX: typo"},
			},
			wantLen: 1,
		},
		{
			name: "invalid type FEAT uppercase",
			commits: []RawCommit{
				{SHA: "abc1234567890def", Message: "FEAT: add"},
			},
			wantLen: 1,
		},
		{
			name: "invalid type unknown",
			commits: []RawCommit{
				{SHA: "abc1234567890def", Message: "unknown: add"},
			},
			wantLen: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ValidateAll(tt.commits)
			if len(got) != tt.wantLen {
				t.Errorf("ValidateAll() returned %d violations, want %d", len(got), tt.wantLen)
			}
			if tt.wantLen > 0 && got[0].Rule != "invalid-type" {
				t.Errorf("Rule = %q, want invalid-type", got[0].Rule)
			}
		})
	}
}

func TestValidateAll_MissingType(t *testing.T) {
	tests := []struct {
		name    string
		commits []RawCommit
		wantLen int
		wantRule string
	}{
		{
			name: "no type",
			commits: []RawCommit{
				{SHA: "abc1234567890def", Message: "add user login"},
			},
			wantLen: 2, // missing-type and empty-description
			wantRule: "missing-type",
		},
		{
			name: "colon only",
			commits: []RawCommit{
				{SHA: "abc1234567890def", Message: ": description"},
			},
			wantLen: 1,
			wantRule: "missing-type",
		},
		{
			name: "scope but no type",
			commits: []RawCommit{
				{SHA: "abc1234567890def", Message: "(scope): description"},
			},
			wantLen: 1,
			wantRule: "missing-type",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ValidateAll(tt.commits)
			if len(got) != tt.wantLen {
				t.Errorf("ValidateAll() returned %d violations, want %d", len(got), tt.wantLen)
			}
			// Check that at least one violation has the expected rule
			found := false
			for _, v := range got {
				if v.Rule == tt.wantRule {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("Rule %q not found in violations", tt.wantRule)
			}
		})
	}
}

func TestValidateAll_EmptyDescription(t *testing.T) {
	tests := []struct {
		name    string
		commits []RawCommit
		wantLen int
	}{
		{
			name: "feat with empty description",
			commits: []RawCommit{
				{SHA: "abc1234567890def", Message: "feat: "},
			},
			wantLen: 1,
		},
		{
			name: "fix with empty description",
			commits: []RawCommit{
				{SHA: "abc1234567890def", Message: "fix: "},
			},
			wantLen: 1,
		},
		{
			name: "chore with empty description",
			commits: []RawCommit{
				{SHA: "abc1234567890def", Message: "chore: "},
			},
			wantLen: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ValidateAll(tt.commits)
			if len(got) != tt.wantLen {
				t.Errorf("ValidateAll() returned %d violations, want %d", len(got), tt.wantLen)
			}
			if tt.wantLen > 0 && got[0].Rule != "empty-description" {
				t.Errorf("Rule = %q, want empty-description", got[0].Rule)
			}
		})
	}
}

func TestValidateAll_TrailingPeriod(t *testing.T) {
	tests := []struct {
		name    string
		commits []RawCommit
		wantLen int
	}{
		{
			name: "feat with trailing period",
			commits: []RawCommit{
				{SHA: "abc1234567890def", Message: "feat: add login."},
			},
			wantLen: 1,
		},
		{
			name: "fix with trailing period",
			commits: []RawCommit{
				{SHA: "abc1234567890def", Message: "fix: typo."},
			},
			wantLen: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ValidateAll(tt.commits)
			if len(got) != tt.wantLen {
				t.Errorf("ValidateAll() returned %d violations, want %d", len(got), tt.wantLen)
			}
			if tt.wantLen > 0 && got[0].Rule != "trailing-period" {
				t.Errorf("Rule = %q, want trailing-period", got[0].Rule)
			}
		})
	}
}

func TestValidateAll_CapitalFirstLetter(t *testing.T) {
	tests := []struct {
		name    string
		commits []RawCommit
		wantLen int
	}{
		{
			name: "feat with capital first letter",
			commits: []RawCommit{
				{SHA: "abc1234567890def", Message: "feat: Add login"},
			},
			wantLen: 1,
		},
		{
			name: "fix with capital first letter",
			commits: []RawCommit{
				{SHA: "abc1234567890def", Message: "fix: Null pointer"},
			},
			wantLen: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ValidateAll(tt.commits)
			if len(got) != tt.wantLen {
				t.Errorf("ValidateAll() returned %d violations, want %d", len(got), tt.wantLen)
			}
			if tt.wantLen > 0 && got[0].Rule != "capital-first-letter" {
				t.Errorf("Rule = %q, want capital-first-letter", got[0].Rule)
			}
		})
	}
}

func TestValidateAll_BreakingChangeFooter(t *testing.T) {
	tests := []struct {
		name       string
		commits    []RawCommit
		wantBreak  bool
		wantViolations int
	}{
		{
			name: "fix with BREAKING CHANGE footer",
			commits: []RawCommit{
				{SHA: "abc1234567890def", Message: "fix: typo\n\nBREAKING CHANGE: removes X"},
			},
			wantBreak: true,
			wantViolations: 0,
		},
		{
			name: "feat with BREAKING CHANGE footer",
			commits: []RawCommit{
				{SHA: "abc1234567890def", Message: "feat: new\n\nBREAKING CHANGE: api change"},
			},
			wantBreak: true,
			wantViolations: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			violations := ValidateAll(tt.commits)
			if len(violations) != tt.wantViolations {
				t.Errorf("ValidateAll() returned %d violations, want %d", len(violations), tt.wantViolations)
			}

			commit, err := Parse(tt.commits[0])
			if err != nil {
				t.Fatalf("Parse() error = %v, want nil", err)
			}
			if commit.Breaking != tt.wantBreak {
				t.Errorf("Breaking = %v, want %v", commit.Breaking, tt.wantBreak)
			}
		})
	}
}

func TestValidateAll_BreakingWithExclamation(t *testing.T) {
	tests := []struct {
		name    string
		commits []RawCommit
		wantLen int
	}{
		{
			name: "feat with exclamation",
			commits: []RawCommit{
				{SHA: "abc1234567890def", Message: "feat!: new API"},
			},
			wantLen: 0,
		},
		{
			name: "fix with scope and exclamation",
			commits: []RawCommit{
				{SHA: "abc1234567890def", Message: "fix(scope)!: breaking fix"},
			},
			wantLen: 0,
		},
		{
			name: "chore with exclamation",
			commits: []RawCommit{
				{SHA: "abc1234567890def", Message: "chore!: drop support"},
			},
			wantLen: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ValidateAll(tt.commits)
			if len(got) != tt.wantLen {
				t.Errorf("ValidateAll() returned %d violations, want %d", len(got), tt.wantLen)
			}
		})
	}
}

func TestValidateAll_ScopeVariations(t *testing.T) {
	tests := []struct {
		name    string
		commits []RawCommit
		wantLen int
	}{
		{
			name: "simple scope",
			commits: []RawCommit{
				{SHA: "abc1234567890def", Message: "feat(scope): desc"},
			},
			wantLen: 0,
		},
		{
			name: "scope with hyphen",
			commits: []RawCommit{
				{SHA: "abc1234567890def", Message: "feat(api-v2): desc"},
			},
			wantLen: 0,
		},
		{
			name: "scope with slash",
			commits: []RawCommit{
				{SHA: "abc1234567890def", Message: "feat(auth/jwt): desc"},
			},
			wantLen: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ValidateAll(tt.commits)
			if len(got) != tt.wantLen {
				t.Errorf("ValidateAll() returned %d violations, want %d", len(got), tt.wantLen)
			}
		})
	}
}

func TestIsMergeCommit(t *testing.T) {
	tests := []struct {
		name    string
		message string
		want    bool
	}{
		{
			name:    "Merge branch",
			message: "Merge branch feature",
			want:    true,
		},
		{
			name:    "Merge pull request",
			message: "Merge pull request #42 from user/branch",
			want:    true,
		},
		{
			name:    "Merge branch with quotes",
			message: "Merge branch 'main'",
			want:    true,
		},
		{
			name:    "feat commit",
			message: "feat: add feature",
			want:    false,
		},
		{
			name:    "fix commit",
			message: "fix: typo",
			want:    false,
		},
		{
			name:    "merge with leading whitespace",
			message: "  Merge branch feature",
			want:    true,
		},
		{
			name:    "GitHub Actions CI merge commit (sha into sha)",
			message: "Merge 30200f20fcea3c9781f1b8a3d664ffcb93907c27 into ba5266b339a6155fe4acf82be393a1a3d9a54cba",
			want:    true,
		},
		{
			name:    "GitHub Actions CI merge commit short SHAs",
			message: "Merge abc1234 into def5678",
			want:    true,
		},
		{
			name:    "Merge remote-tracking branch",
			message: "Merge remote-tracking branch 'origin/main'",
			want:    true,
		},
		{
			name:    "bulldozer merge refs/heads into branch",
			message: "Merge refs/heads/main into renovate/ghcr.io-actions-actions-runner-2.x",
			want:    true,
		},
		{
			name:    "bulldozer merge refs/heads with newline",
			message: "Merge refs/heads/main into renovate/ghcr.io-actions-actions-runner-2.x\n",
			want:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsMergeCommit(tt.message)
			if got != tt.want {
				t.Errorf("IsMergeCommit() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestValidateAll_MergeCommitsSkipped(t *testing.T) {
	tests := []struct {
		name    string
		commits []RawCommit
		wantLen int
	}{
		{
			name: "merge branch is skipped",
			commits: []RawCommit{
				{SHA: "abc1234567890def", Message: "Merge branch feature"},
			},
			wantLen: 0,
		},
		{
			name: "merge pull request is skipped",
			commits: []RawCommit{
				{SHA: "abc1234567890def", Message: "Merge pull request #42 from user/branch"},
			},
			wantLen: 0,
		},
		{
			name: "regular commit mixed with merge",
			commits: []RawCommit{
				{SHA: "abc1234567890def", Message: "Merge branch feature"},
				{SHA: "def1234567890abc", Message: "feat: add"},
			},
			wantLen: 0,
		},
		{
			name: "bulldozer refs/heads merge is skipped",
			commits: []RawCommit{
				{SHA: "abc1234567890def", Message: "Merge refs/heads/main into renovate/ghcr.io-actions-actions-runner-2.x"},
			},
			wantLen: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ValidateAll(tt.commits)
			if len(got) != tt.wantLen {
				t.Errorf("ValidateAll() returned %d violations, want %d", len(got), tt.wantLen)
			}
		})
	}
}

func TestValidateAll_RevertCommits(t *testing.T) {
	tests := []struct {
		name    string
		commits []RawCommit
		wantLen int
	}{
		{
			name: "revert feat",
			commits: []RawCommit{
				{SHA: "abc1234567890def", Message: "revert: feat: add login"},
			},
			wantLen: 0,
		},
		{
			name: "revert fix",
			commits: []RawCommit{
				{SHA: "abc1234567890def", Message: "revert: fix: null pointer"},
			},
			wantLen: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ValidateAll(tt.commits)
			if len(got) != tt.wantLen {
				t.Errorf("ValidateAll() returned %d violations, want %d", len(got), tt.wantLen)
			}
		})
	}
}

func TestValidateAll_CollectAllViolations(t *testing.T) {
	// Test that ValidateAll collects ALL violations, not just the first
	commits := []RawCommit{
		{SHA: "abc1234567890def", Message: "feat: add login"},
		{SHA: "def1234567890abc", Message: "FIX: typo"},
		{SHA: "ghi1234567890def", Message: "chore: cleanup"},
	}

	violations := ValidateAll(commits)
	// Second commit has invalid type (uppercase FIX)
	if len(violations) != 1 {
		t.Errorf("ValidateAll() returned %d violations, want 1", len(violations))
	}
	if violations[0].Rule != "invalid-type" {
		t.Errorf("Rule = %q, want invalid-type", violations[0].Rule)
	}
}

func TestValidateAll_MultipleViolationsSingleCommit(t *testing.T) {
	// Test that ValidateAll collects multiple violations from a single commit
	commits := []RawCommit{
		{SHA: "abc1234567890def", Message: "FEAT: Add feature."},
	}

	violations := ValidateAll(commits)
	// Should have: invalid-type, capital-first-letter, trailing-period
	if len(violations) != 3 {
		t.Errorf("ValidateAll() returned %d violations, want 3", len(violations))
	}

	rules := make(map[string]bool)
	for _, v := range violations {
		rules[v.Rule] = true
	}

	if !rules["invalid-type"] {
		t.Error("Missing invalid-type violation")
	}
	if !rules["capital-first-letter"] {
		t.Error("Missing capital-first-letter violation")
	}
	if !rules["trailing-period"] {
		t.Error("Missing trailing-period violation")
	}
}

func TestValidateAll_EmptySlice(t *testing.T) {
	violations := ValidateAll([]RawCommit{})
	if violations == nil {
		t.Error("ValidateAll() returned nil, want empty slice")
	}
	if len(violations) != 0 {
		t.Errorf("ValidateAll() returned %d violations, want 0", len(violations))
	}
}

func TestValidateAll_AllValid(t *testing.T) {
	commits := []RawCommit{
		{SHA: "abc1234567890def", Message: "feat: add login"},
		{SHA: "def1234567890abc", Message: "fix(auth): null pointer"},
		{SHA: "ghi1234567890def", Message: "chore!: drop go 1.20"},
	}

	violations := ValidateAll(commits)
	if len(violations) != 0 {
		t.Errorf("ValidateAll() returned %d violations, want 0", len(violations))
	}
}

func TestParse_EdgeCases(t *testing.T) {
	tests := []struct {
		name    string
		raw     RawCommit
		wantErr bool
	}{
		{
			name:    "empty message",
			raw:     RawCommit{SHA: "abc1234567890def", Message: ""},
			wantErr: true,
		},
		{
			name:    "whitespace only",
			raw:     RawCommit{SHA: "abc1234567890def", Message: "   \n  \n  "},
			wantErr: true,
		},
		{
			name:    "very long description",
			raw:     RawCommit{SHA: "abc1234567890def", Message: "feat: " + string(make([]byte, 1000))},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parse(tt.raw)
			if (err != nil) != tt.wantErr {
				t.Errorf("Parse() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestCommitFields(t *testing.T) {
	// Test that all Commit fields are properly set
	raw := RawCommit{
		SHA:     "abc1234567890def",
		Message: "feat(scope): description",
	}

	commit, err := Parse(raw)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if commit.SHA != "abc1234567890def" {
		t.Errorf("SHA = %q, want abc1234567890def", commit.SHA)
	}
	if commit.ShortSHA != "abc1234" {
		t.Errorf("ShortSHA = %q, want abc1234", commit.ShortSHA)
	}
	if commit.RawMessage != "feat(scope): description" {
		t.Errorf("RawMessage = %q, want feat(scope): description", commit.RawMessage)
	}
}

func TestViolationFields(t *testing.T) {
	// Test that all Violation fields are properly set
	commits := []RawCommit{
		{SHA: "abc1234567890def", Message: "invalid: description"},
	}

	violations := ValidateAll(commits)
	if len(violations) != 1 {
		t.Fatalf("ValidateAll() returned %d violations, want 1", len(violations))
	}

	v := violations[0]
	if v.SHA != "abc1234567890def" {
		t.Errorf("SHA = %q, want abc1234567890def", v.SHA)
	}
	if v.ShortSHA != "abc1234" {
		t.Errorf("ShortSHA = %q, want abc1234", v.ShortSHA)
	}
	if v.RawMessage != "invalid: description" {
		t.Errorf("RawMessage = %q, want invalid: description", v.RawMessage)
	}
	if v.Rule == "" {
		t.Error("Rule is empty")
	}
	if v.Example == "" {
		t.Error("Example is empty")
	}
}

func TestAllCommitTypes(t *testing.T) {
	// Test that all 10 commit types are valid
	types := []CommitType{
		TypeFeat, TypeFix, TypeChore, TypeDocs, TypeCI,
		TypeRefactor, TypeTest, TypePerf, TypeBuild, TypeRevert,
	}

	for _, ct := range types {
		raw := RawCommit{
			SHA:     "abc1234567890def",
			Message: string(ct) + ": test",
		}

		violations := ValidateAll([]RawCommit{raw})
		if len(violations) != 0 {
			t.Errorf("Type %q produced violations: %v", ct, violations)
		}
	}
}

func TestParse_ScopeWithSpecialChars(t *testing.T) {
	tests := []struct {
		name      string
		message   string
		wantScope string
	}{
		{
			name:      "scope with hyphen",
			message:   "feat(api-v2): desc",
			wantScope: "api-v2",
		},
		{
			name:      "scope with slash",
			message:   "feat(auth/jwt): desc",
			wantScope: "auth/jwt",
		},
		{
			name:      "scope with underscore",
			message:   "feat(user_auth): desc",
			wantScope: "user_auth",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw := RawCommit{SHA: "abc1234567890def", Message: tt.message}
			commit, err := Parse(raw)
			if err != nil {
				t.Fatalf("Parse() error = %v", err)
			}
			if commit.Scope != tt.wantScope {
				t.Errorf("Scope = %q, want %q", commit.Scope, tt.wantScope)
			}
		})
	}
}

func TestValidateAll_ExampleField(t *testing.T) {
	// Test that Example field contains corrected version
	tests := []struct {
		name       string
		message    string
		wantExample string
	}{
		{
			name:       "invalid type example",
			message:    "FEATURE: add",
			wantExample: "feat: add",
		},
		{
			name:       "trailing period example",
			message:    "feat: add.",
			wantExample: "feat: add",
		},
		{
			name:       "capital letter example",
			message:    "feat: Add",
			wantExample: "feat: add",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			commits := []RawCommit{
				{SHA: "abc1234567890def", Message: tt.message},
			}
			violations := ValidateAll(commits)
			if len(violations) == 0 {
				t.Fatal("Expected violations but got none")
			}
			// Find the violation with the expected example
			found := false
			for _, v := range violations {
				if v.Example == tt.wantExample {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("Example %q not found in violations", tt.wantExample)
			}
		})
	}
}
