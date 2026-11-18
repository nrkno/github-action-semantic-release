package conventional

import (
	"errors"
	"fmt"
	"os"

	"go.yaml.in/yaml/v3"
)

// BumpLevel is a string alias for bump-rules map values.
// Valid values: "major", "minor", "patch", "none".
// Type alias (not new type) avoids import of internal/semver.
type BumpLevel = string

const (
	BumpLevelMajor BumpLevel = "major"
	BumpLevelMinor BumpLevel = "minor"
	BumpLevelPatch BumpLevel = "patch"
	BumpLevelNone  BumpLevel = "none"
)

// BumpRules maps commit-type strings (and the sentinel "breaking-change")
// to bump levels. Absent keys default to BumpLevelNone at runtime.
// "breaking-change" is the sentinel for commits with a "!" suffix or
// "BREAKING CHANGE:" footer — it is NOT a conventional commit type.
type BumpRules map[string]BumpLevel

// CommitTypesConfig controls which conventional-commit types semrel lint accepts.
type CommitTypesConfig struct {
	// ExtraTypes adds types on top of the built-in set.
	ExtraTypes []string `yaml:"extra-types"`
	// AllowedTypes replaces the built-in set entirely when len > 0.
	// nil or empty (len == 0) falls through to the built-in defaults.
	AllowedTypes []CommitType `yaml:"allowed-types"`
}

// LintRules controls which lint rules are enforced by ValidateAll.
type LintRules struct {
	// CapitalFirstLetter fails commits whose description starts with an
	// uppercase letter. Default: true.
	CapitalFirstLetter bool `yaml:"capital-first-letter"`
	// RequireScope fails commits that have no scope. Default: false.
	RequireScope bool `yaml:"require-scope"`
}

// LintConfig is the [lint] section of .semrelrc.yml.
type LintConfig struct {
	Rules LintRules `yaml:"rules"`
}

// Config is the root structure of .semrelrc.yml.
type Config struct {
	Lint LintConfig `yaml:"lint"`

	// BumpRules maps commit types to version bump levels.
	// Default: {"breaking-change":"major","feat":"minor","fix":"patch"}.
	// Absent keys default to "none". A bare null YAML key restores defaults.
	// To freeze all bumps, set each key to "none" explicitly:
	//   bump-rules: {breaking-change: none, feat: none, fix: none}
	BumpRules BumpRules `yaml:"bump-rules"`

	// ReleaseBranches lists branch patterns (path.Match syntax) on which
	// semrel release will proceed. Defaults to ["main","master"].
	// Note: '*' does not cross '/' boundaries.
	ReleaseBranches []string `yaml:"release-branches"`

	// TagPrefix is prepended to version numbers when forming git tags.
	// Default: "v" → produces "v1.2.3". Set "" for bare "1.2.3" tags.
	// CRITICAL: the default MUST be "v", not "". An empty default silently
	// strips the v-prefix from all future tags on repos without a config file.
	TagPrefix string `yaml:"tag-prefix"`

	// CommitTypes controls the allowed commit type set for semrel lint.
	CommitTypes CommitTypesConfig `yaml:"commit-types"`

	// InitialVersion is the baseline for the bootstrap case (no existing tags).
	// Default: "0.0.0". The detected bump is applied on top of this value.
	// Must be a valid semver string. Validated by cli.go (not LoadConfig).
	InitialVersion string `yaml:"initial-version"`
}

// DefaultConfig returns the hardcoded defaults (current behaviour — no file needed).
func DefaultConfig() Config {
	return Config{
		Lint: LintConfig{
			Rules: LintRules{
				CapitalFirstLetter: true,
				RequireScope:       false,
			},
		},
		BumpRules: BumpRules{
			"breaking-change": BumpLevelMajor,
			"feat":            BumpLevelMinor,
			"fix":             BumpLevelPatch,
		},
		ReleaseBranches: []string{"main", "master"},
		TagPrefix:       "v", // MUST be "v", never "". See field doc above.
		CommitTypes:     CommitTypesConfig{},
		InitialVersion:  "0.0.0",
	}
}

// LintOptions is derived from Config.Lint.Rules and passed to ValidateAll.
type LintOptions struct {
	CapitalFirstLetter bool
	RequireScope       bool
	// AllowedTypes restricts which commit types ValidateAll accepts.
	// nil or empty (len == 0) falls back to the built-in validTypes map.
	AllowedTypes []CommitType
}

// DefaultLintOptions returns options matching the hardcoded defaults.
func DefaultLintOptions() LintOptions {
	d := DefaultConfig()
	return LintOptions{
		CapitalFirstLetter: d.Lint.Rules.CapitalFirstLetter,
		RequireScope:       d.Lint.Rules.RequireScope,
		AllowedTypes:       nil,
	}
}

// LoadConfig reads .semrelrc.yml from path.
//   - Returns (nil, nil) if the file does not exist — caller uses DefaultConfig().
//   - Returns (nil, err) if the file exists but contains malformed YAML.
//   - Returns (*Config, nil) on success.
//
// IMPORTANT: YAML is decoded onto a pre-initialised DefaultConfig() struct so
// that fields absent from the file retain their default values (not zero values).
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("read .semrelrc.yml: %w", err)
	}
	cfg := DefaultConfig() // decode onto defaults, not zero-value
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("malformed .semrelrc.yml: %w", err)
	}

	// C1: Nil/empty guard — yaml.v3 zeroes the map on a bare null key.
	if len(cfg.BumpRules) == 0 {
		cfg.BumpRules = DefaultConfig().BumpRules
	}

	// mn1: Validate BumpRules values.
	validBumpLevels := map[string]bool{"major": true, "minor": true, "patch": true, "none": true}
	for k, v := range cfg.BumpRules {
		if !validBumpLevels[v] {
			return nil, fmt.Errorf("malformed .semrelrc.yml: invalid bump level %q for type %q (must be major|minor|patch|none)", v, k)
		}
	}

	return &cfg, nil
}
