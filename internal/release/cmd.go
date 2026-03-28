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

// RunStatus posts a release status comment listing unreleased changes for all
// configured crates and npm packages.
func RunStatus(ctx context.Context, api ReleaseAPI, owner, repo string, issueNumber int, cfg repoconfig.Release) error {
	if len(cfg.Crates) == 0 && len(cfg.Packages) == 0 {
		return api.CreateComment(ctx, owner, repo, issueNumber,
			"No crates or packages configured under `[release]` in `.vexilbot.toml`.")
	}

	combined := make(map[string]ChangeResult)

	if len(cfg.Crates) > 0 {
		results, err := DetectChanges(ctx, api, owner, repo, cfg.TagFormat, cfg.Crates)
		if err != nil {
			return fmt.Errorf("detect crate changes: %w", err)
		}
		for k, v := range results {
			combined[k] = v
		}
	}

	if len(cfg.Packages) > 0 {
		results, err := DetectPackageChanges(ctx, api, owner, repo, cfg.TagFormat, cfg.Packages)
		if err != nil {
			return fmt.Errorf("detect package changes: %w", err)
		}
		for k, v := range results {
			combined[k] = v
		}
	}

	return api.CreateComment(ctx, owner, repo, issueNumber, FormatStatus(combined))
}

// RunRelease creates a release PR for the named crate or npm package.
// It checks crates first, then packages.
func RunRelease(ctx context.Context, api ReleaseAPI, owner, repo, name string, issueNumber int, cfg repoconfig.Release) error {
	if crate, ok := cfg.Crates[name]; ok {
		return runCargoRelease(ctx, api, owner, repo, name, issueNumber, crate, cfg)
	}
	if pkg, ok := cfg.Packages[name]; ok {
		return runNpmRelease(ctx, api, owner, repo, name, issueNumber, pkg, cfg)
	}
	return api.CreateComment(ctx, owner, repo, issueNumber,
		fmt.Sprintf(":x: **%s** not found in `.vexilbot.toml` release config (checked `crates` and `packages`).", name))
}

func runCargoRelease(ctx context.Context, api ReleaseAPI, owner, repo, name string, issueNumber int, crate repoconfig.CrateEntry, cfg repoconfig.Release) error {
	results, err := DetectChanges(ctx, api, owner, repo, cfg.TagFormat, cfg.Crates)
	if err != nil {
		return fmt.Errorf("detect changes: %w", err)
	}
	result := results[name]
	if len(result.Commits) == 0 {
		return api.CreateComment(ctx, owner, repo, issueNumber,
			fmt.Sprintf("No unreleased changes for **%s** — nothing to release.", name))
	}

	newVersion, err := computeNewVersion(result)
	if err != nil {
		return err
	}

	filePath := crate.Path + "/Cargo.toml"
	content, fileSHA, err := api.GetFileContent(ctx, owner, repo, filePath)
	if err != nil {
		return fmt.Errorf("read %s: %w", filePath, err)
	}
	updated, err := BumpCargoVersion(string(content), newVersion.String())
	if err != nil {
		return fmt.Errorf("bump version: %w", err)
	}

	return openReleasePR(ctx, api, owner, repo, name, issueNumber, filePath, updated, fileSHA, result, newVersion)
}

func runNpmRelease(ctx context.Context, api ReleaseAPI, owner, repo, name string, issueNumber int, pkg repoconfig.PackageEntry, cfg repoconfig.Release) error {
	results, err := DetectPackageChanges(ctx, api, owner, repo, cfg.TagFormat, cfg.Packages)
	if err != nil {
		return fmt.Errorf("detect changes: %w", err)
	}
	result := results[name]
	if len(result.Commits) == 0 {
		return api.CreateComment(ctx, owner, repo, issueNumber,
			fmt.Sprintf("No unreleased changes for **%s** — nothing to release.", name))
	}

	newVersion, err := computeNewVersion(result)
	if err != nil {
		return err
	}

	filePath := pkg.Path + "/package.json"
	content, fileSHA, err := api.GetFileContent(ctx, owner, repo, filePath)
	if err != nil {
		return fmt.Errorf("read %s: %w", filePath, err)
	}
	updated, err := BumpNpmVersion(string(content), newVersion.String())
	if err != nil {
		return fmt.Errorf("bump version: %w", err)
	}

	return openReleasePR(ctx, api, owner, repo, name, issueNumber, filePath, updated, fileSHA, result, newVersion)
}

func computeNewVersion(result ChangeResult) (Version, error) {
	var current Version
	if result.CurrentVersion != "" {
		var err error
		current, err = ParseVersion(result.CurrentVersion)
		if err != nil {
			return Version{}, fmt.Errorf("parse current version %q: %w", result.CurrentVersion, err)
		}
	}
	return current.Bump(result.SuggestedBump), nil
}

func openReleasePR(ctx context.Context, api ReleaseAPI, owner, repo, name string, issueNumber int, filePath, updated, fileSHA string, result ChangeResult, newVersion Version) error {
	defaultBranch, err := api.GetDefaultBranch(ctx, owner, repo)
	if err != nil {
		return fmt.Errorf("get default branch: %w", err)
	}
	branchSHA, err := api.GetBranchSHA(ctx, owner, repo, defaultBranch)
	if err != nil {
		return fmt.Errorf("get branch SHA: %w", err)
	}

	releaseBranch := BranchName(name, newVersion.String())
	if err := api.CreateBranch(ctx, owner, repo, releaseBranch, branchSHA); err != nil {
		return fmt.Errorf("create branch %s: %w", releaseBranch, err)
	}

	commitMsg := fmt.Sprintf("chore(%s): bump version to %s", name, newVersion)
	if err := api.UpdateFile(ctx, owner, repo, filePath, commitMsg, []byte(updated), fileSHA, releaseBranch); err != nil {
		return fmt.Errorf("commit version bump: %w", err)
	}

	prTitle := fmt.Sprintf("release: %s v%s", name, newVersion)
	prBody := buildPRBody(name, result.CurrentVersion, newVersion.String(), result)
	prNumber, err := api.CreatePR(ctx, owner, repo, prTitle, prBody, releaseBranch, defaultBranch)
	if err != nil {
		return fmt.Errorf("create PR: %w", err)
	}

	return api.CreateComment(ctx, owner, repo, issueNumber,
		fmt.Sprintf("Created release PR #%d for **%s** v%s (%s bump).", prNumber, name, newVersion, result.SuggestedBump))
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
