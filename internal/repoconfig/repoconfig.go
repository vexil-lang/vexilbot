package repoconfig

import (
	"fmt"

	"github.com/BurntSushi/toml"
)

type Config struct {
	Labels  Labels  `toml:"labels"`
	Triage  Triage  `toml:"triage"`
	Welcome Welcome `toml:"welcome"`
	Policy  Policy  `toml:"policy"`
	Release Release `toml:"release"`
	LLM     LLM     `toml:"llm"`
}

type Labels struct {
	Paths    map[string][]string `toml:"paths"`
	Keywords map[string][]string `toml:"keywords"`
}

type Triage struct {
	AllowedTeams       []string `toml:"allowed_teams"`
	AllowCollaborators bool     `toml:"allow_collaborators"`
}

type Welcome struct {
	PRMessage    string `toml:"pr_message"`
	IssueMessage string `toml:"issue_message"`
}

type Policy struct {
	RFCRequiredPaths       []string `toml:"rfc_required_paths"`
	WireFormatWarningPaths []string `toml:"wire_format_warning_paths"`
}

type Release struct {
	ChangelogTool string                 `toml:"changelog_tool"`
	TagFormat     string                 `toml:"tag_format"`
	AutoRelease   bool                   `toml:"auto_release"`
	RequireCI     bool                   `toml:"require_ci"`
	Crates        map[string]CrateEntry  `toml:"crates"`
	Packages      map[string]PackageEntry `toml:"packages"`
}

type PackageEntry struct {
	Path             string        `toml:"path"`
	Publish          interface{}   `toml:"publish"` // "npmjs" or false
	SuggestThreshold int           `toml:"suggest_threshold"`
	DependsOn        []string      `toml:"depends_on"`
}

type CrateEntry struct {
	Path             string        `toml:"path"`
	Publish          interface{}   `toml:"publish"`
	SuggestThreshold int           `toml:"suggest_threshold"`
	DependsOn        []string      `toml:"depends_on"`
	PostPublish      []PostPublish `toml:"post_publish"`
	Track            bool          `toml:"track"`
}

type PostPublish struct {
	Run     string `toml:"run"`
	Package string `toml:"package"`
}

type LLM struct {
	Enabled  bool        `toml:"enabled"`
	Provider string      `toml:"provider"`
	Model    string      `toml:"model"`
	Features LLMFeatures `toml:"features"`
}

type LLMFeatures struct {
	PRReview     bool `toml:"pr_review"`
	IssueTriage  bool `toml:"issue_triage"`
	ReleaseNotes bool `toml:"release_notes"`
}

func Parse(data []byte) (*Config, error) {
	var cfg Config
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse vexilbot.toml: %w", err)
	}
	return &cfg, nil
}
