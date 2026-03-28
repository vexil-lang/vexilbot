package release

import (
	"context"
	"fmt"
	"strings"

	"github.com/vexil-lang/vexilbot/internal/repoconfig"
)

// ReleaseAPI is the GitHub API surface needed by release commands.
type ReleaseAPI interface {
	GitAPI
	GetFileContent(ctx context.Context, owner, repo, path string) ([]byte, string, error)
	GetDefaultBranch(ctx context.Context, owner, repo string) (string, error)
	GetBranchSHA(ctx context.Context, owner, repo, branch string) (string, error)
	CreateBranch(ctx context.Context, owner, repo, branch, sha string) error
	UpdateFile(ctx context.Context, owner, repo, path, message string, content []byte, sha, branch string) error
	CreatePR(ctx context.Context, owner, repo, title, body, head, base string) (int, error)
	CreateComment(ctx context.Context, owner, repo string, number int, body string) error
}

// RunStatus posts a release status comment listing unreleased changes per crate.
func RunStatus(ctx context.Context, api ReleaseAPI, owner, repo string, issueNumber int, cfg repoconfig.Release) error {
	if len(cfg.Crates) == 0 {
		return api.CreateComment(ctx, owner, repo, issueNumber,
			"No crates configured under `[release.crates]` in `.vexilbot.toml`.")
	}
	results, err := DetectChanges(ctx, api, owner, repo, cfg.TagFormat, cfg.Crates)
	if err != nil {
		return fmt.Errorf("detect changes: %w", err)
	}
	return api.CreateComment(ctx, owner, repo, issueNumber, FormatStatus(results))
}

// RunRelease creates a release PR for the named crate with a semver bump
// derived from conventional commits since the last tag.
func RunRelease(ctx context.Context, api ReleaseAPI, owner, repo, crateName string, issueNumber int, cfg repoconfig.Release) error {
	crate, ok := cfg.Crates[crateName]
	if !ok {
		return api.CreateComment(ctx, owner, repo, issueNumber,
			fmt.Sprintf(":x: Crate **%s** not found in `.vexilbot.toml` release config.", crateName))
	}

	results, err := DetectChanges(ctx, api, owner, repo, cfg.TagFormat, cfg.Crates)
	if err != nil {
		return fmt.Errorf("detect changes: %w", err)
	}

	result := results[crateName]
	if len(result.Commits) == 0 {
		return api.CreateComment(ctx, owner, repo, issueNumber,
			fmt.Sprintf("No unreleased changes for **%s** — nothing to release.", crateName))
	}

	var currentVersion Version
	if result.CurrentVersion != "" {
		currentVersion, err = ParseVersion(result.CurrentVersion)
		if err != nil {
			return fmt.Errorf("parse current version %q: %w", result.CurrentVersion, err)
		}
	}
	newVersion := currentVersion.Bump(result.SuggestedBump)

	cargoPath := crate.Path + "/Cargo.toml"
	content, fileSHA, err := api.GetFileContent(ctx, owner, repo, cargoPath)
	if err != nil {
		return fmt.Errorf("read %s: %w", cargoPath, err)
	}

	updated, err := BumpCargoVersion(string(content), newVersion.String())
	if err != nil {
		return fmt.Errorf("bump version in %s: %w", cargoPath, err)
	}

	defaultBranch, err := api.GetDefaultBranch(ctx, owner, repo)
	if err != nil {
		return fmt.Errorf("get default branch: %w", err)
	}

	branchSHA, err := api.GetBranchSHA(ctx, owner, repo, defaultBranch)
	if err != nil {
		return fmt.Errorf("get branch SHA: %w", err)
	}

	releaseBranch := BranchName(crateName, newVersion.String())
	if err := api.CreateBranch(ctx, owner, repo, releaseBranch, branchSHA); err != nil {
		return fmt.Errorf("create branch %s: %w", releaseBranch, err)
	}

	commitMsg := fmt.Sprintf("chore(%s): bump version to %s", crateName, newVersion)
	if err := api.UpdateFile(ctx, owner, repo, cargoPath, commitMsg, []byte(updated), fileSHA, releaseBranch); err != nil {
		return fmt.Errorf("commit version bump: %w", err)
	}

	prTitle := fmt.Sprintf("release: %s v%s", crateName, newVersion)
	prBody := buildPRBody(crateName, result.CurrentVersion, newVersion.String(), result)
	prNumber, err := api.CreatePR(ctx, owner, repo, prTitle, prBody, releaseBranch, defaultBranch)
	if err != nil {
		return fmt.Errorf("create PR: %w", err)
	}

	return api.CreateComment(ctx, owner, repo, issueNumber,
		fmt.Sprintf("Created release PR #%d for **%s** v%s (%s bump).", prNumber, crateName, newVersion, result.SuggestedBump))
}

func buildPRBody(crate, fromVersion, toVersion string, result ChangeResult) string {
	from := "initial"
	if fromVersion != "" {
		from = "v" + fromVersion
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "## Release %s v%s\n\n", crate, toVersion)
	fmt.Fprintf(&sb, "Bump %s → v%s (%s)\n\n", from, toVersion, result.SuggestedBump)
	sb.WriteString("### Commits\n\n")
	for _, c := range result.Commits {
		msg := c.Message
		if idx := strings.Index(msg, "\n"); idx != -1 {
			msg = msg[:idx] // first line only
		}
		if len(msg) > 72 {
			msg = msg[:72] + "…"
		}
		fmt.Fprintf(&sb, "- %s\n", msg)
	}
	sb.WriteString("\n---\n_Merging this PR will trigger `cargo publish` via GitHub Actions._")
	return sb.String()
}
