package repoconfig_test

import (
	"testing"

	"github.com/vexil-lang/vexilbot/internal/repoconfig"
)

func TestParse_FullConfig(t *testing.T) {
	content := `
[labels]
[labels.paths]
"crate:lang" = ["crates/vexil-lang/**"]
"spec"       = ["spec/**"]

[labels.keywords]
"bug" = ["crash", "panic"]

[triage]
allowed_teams = ["maintainers"]
allow_collaborators = true

[welcome]
pr_message = "Welcome to Vexil!"
issue_message = "Thanks for reporting!"

[policy]
rfc_required_paths = ["spec/**"]
wire_format_warning_paths = ["crates/vexil-runtime/**"]

[release]
changelog_tool = "git-cliff"
tag_format = "{{ crate }}-v{{ version }}"
auto_release = false
require_ci = true

[release.crates.vexil-lang]
path = "crates/vexil-lang"
publish = "crates.io"
suggest_threshold = 1
depends_on = []

[release.crates.vexil-bench]
path = "crates/vexil-bench"
publish = false
track = false

[llm]
enabled = false
provider = "claude"
model = "claude-sonnet-4-6-20250514"
[llm.features]
pr_review = false
issue_triage = false
release_notes = false
`
	cfg, err := repoconfig.Parse([]byte(content))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	paths := cfg.Labels.Paths
	if got := paths["crate:lang"]; len(got) != 1 || got[0] != "crates/vexil-lang/**" {
		t.Errorf("labels.paths[crate:lang] = %v", got)
	}
	keywords := cfg.Labels.Keywords
	if got := keywords["bug"]; len(got) != 2 || got[0] != "crash" {
		t.Errorf("labels.keywords[bug] = %v", got)
	}
	if !cfg.Triage.AllowCollaborators {
		t.Error("triage.allow_collaborators should be true")
	}
	if len(cfg.Triage.AllowedTeams) != 1 || cfg.Triage.AllowedTeams[0] != "maintainers" {
		t.Errorf("triage.allowed_teams = %v", cfg.Triage.AllowedTeams)
	}
	if cfg.Welcome.PRMessage != "Welcome to Vexil!" {
		t.Errorf("welcome.pr_message = %q", cfg.Welcome.PRMessage)
	}
	if len(cfg.Policy.RFCRequiredPaths) != 1 || cfg.Policy.RFCRequiredPaths[0] != "spec/**" {
		t.Errorf("policy.rfc_required_paths = %v", cfg.Policy.RFCRequiredPaths)
	}
	if cfg.Release.TagFormat != "{{ crate }}-v{{ version }}" {
		t.Errorf("release.tag_format = %q", cfg.Release.TagFormat)
	}
	if cfg.Release.AutoRelease {
		t.Error("release.auto_release should be false")
	}
	lang, ok := cfg.Release.Crates["vexil-lang"]
	if !ok {
		t.Fatal("release.crates.vexil-lang missing")
	}
	if lang.Path != "crates/vexil-lang" {
		t.Errorf("vexil-lang path = %q", lang.Path)
	}
	if lang.Publish != "crates.io" {
		t.Errorf("vexil-lang publish = %q", lang.Publish)
	}
	bench, ok := cfg.Release.Crates["vexil-bench"]
	if !ok {
		t.Fatal("release.crates.vexil-bench missing")
	}
	if bench.Track {
		t.Error("vexil-bench track should be false")
	}
	if cfg.LLM.Enabled {
		t.Error("llm.enabled should be false")
	}
	if cfg.LLM.Provider != "claude" {
		t.Errorf("llm.provider = %q", cfg.LLM.Provider)
	}
}

func TestParse_Empty(t *testing.T) {
	cfg, err := repoconfig.Parse([]byte(""))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_ = cfg
}

func TestParse_InvalidTOML(t *testing.T) {
	_, err := repoconfig.Parse([]byte("{{invalid"))
	if err == nil {
		t.Fatal("expected error for invalid TOML")
	}
}
