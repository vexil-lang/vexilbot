package release

import (
	"context"
	"fmt"
	"path"
	"strings"
	"time"

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
func RunRelease(ctx context.Context, api ReleaseAPI, owner, repo, name string, issueNumber int, cfg repoconfig.Release) error {
	if crate, ok := cfg.Crates[name]; ok {
		prNum, err := createCratePR(ctx, api, owner, repo, name, crate, cfg)
		if err != nil {
			return err
		}
		return api.CreateComment(ctx, owner, repo, issueNumber,
			fmt.Sprintf("Created release PR #%d for **%s**.", prNum, name))
	}
	if pkg, ok := cfg.Packages[name]; ok {
		prNum, err := createNpmPR(ctx, api, owner, repo, name, pkg, cfg)
		if err != nil {
			return err
		}
		return api.CreateComment(ctx, owner, repo, issueNumber,
			fmt.Sprintf("Created release PR #%d for **%s**.", prNum, name))
	}
	return api.CreateComment(ctx, owner, repo, issueNumber,
		fmt.Sprintf(":x: **%s** not found in `.vexilbot.toml` release config (checked `crates` and `packages`).", name))
}

// RunWorkspaceRelease creates one release PR per crate/package that has
// unreleased commits, respecting dependency order within each ecosystem.
func RunWorkspaceRelease(ctx context.Context, api ReleaseAPI, owner, repo string, issueNumber int, cfg repoconfig.Release) error {
	if len(cfg.Crates) == 0 && len(cfg.Packages) == 0 {
		return api.CreateComment(ctx, owner, repo, issueNumber,
			"No crates or packages configured under `[release]` in `.vexilbot.toml`.")
	}

	var lines []string

	// --- Crates in dependency order ---
	if len(cfg.Crates) > 0 {
		results, err := DetectChanges(ctx, api, owner, repo, cfg.TagFormat, cfg.Crates)
		if err != nil {
			return fmt.Errorf("detect crate changes: %w", err)
		}
		order, err := ResolveDependencyOrder(cfg.Crates)
		if err != nil {
			return fmt.Errorf("resolve crate dependency order: %w", err)
		}
		for _, name := range order {
			if len(results[name].Commits) == 0 {
				continue
			}
			prNum, err := createCratePR(ctx, api, owner, repo, name, cfg.Crates[name], cfg)
			if err != nil {
				return fmt.Errorf("release crate %s: %w", name, err)
			}
			lines = append(lines, fmt.Sprintf("- #%d **%s** (cargo)", prNum, name))
		}
	}

	// --- Packages in dependency order ---
	if len(cfg.Packages) > 0 {
		results, err := DetectPackageChanges(ctx, api, owner, repo, cfg.TagFormat, cfg.Packages)
		if err != nil {
			return fmt.Errorf("detect package changes: %w", err)
		}
		order, err := ResolvePkgDependencyOrder(cfg.Packages)
		if err != nil {
			return fmt.Errorf("resolve package dependency order: %w", err)
		}
		for _, name := range order {
			if len(results[name].Commits) == 0 {
				continue
			}
			prNum, err := createNpmPR(ctx, api, owner, repo, name, cfg.Packages[name], cfg)
			if err != nil {
				return fmt.Errorf("release package %s: %w", name, err)
			}
			lines = append(lines, fmt.Sprintf("- #%d **%s** (npm)", prNum, name))
		}
	}

	if len(lines) == 0 {
		return api.CreateComment(ctx, owner, repo, issueNumber,
			"No unreleased changes found — workspace is up to date.")
	}
	return api.CreateComment(ctx, owner, repo, issueNumber,
		fmt.Sprintf("Created %d release PR(s):\n\n%s", len(lines), strings.Join(lines, "\n")))
}

// --- internal helpers ---

func createCratePR(ctx context.Context, api ReleaseAPI, owner, repo, name string, crate repoconfig.CrateEntry, cfg repoconfig.Release) (int, error) {
	results, err := DetectChanges(ctx, api, owner, repo, cfg.TagFormat, cfg.Crates)
	if err != nil {
		return 0, fmt.Errorf("detect changes: %w", err)
	}
	result := results[name]
	if len(result.Commits) == 0 {
		return 0, fmt.Errorf("no unreleased commits for %s", name)
	}
	newVersion, err := computeNewVersion(result)
	if err != nil {
		return 0, err
	}
	filePath := crate.Path + "/Cargo.toml"
	content, fileSHA, err := api.GetFileContent(ctx, owner, repo, filePath)
	if err != nil {
		return 0, fmt.Errorf("read %s: %w", filePath, err)
	}
	updated, err := BumpCargoVersion(string(content), newVersion.String())
	if err != nil {
		return 0, fmt.Errorf("bump version: %w", err)
	}
	return openReleasePR(ctx, api, owner, repo, name, filePath, updated, fileSHA, result, newVersion, "cargo publish")
}

func createNpmPR(ctx context.Context, api ReleaseAPI, owner, repo, name string, pkg repoconfig.PackageEntry, cfg repoconfig.Release) (int, error) {
	results, err := DetectPackageChanges(ctx, api, owner, repo, cfg.TagFormat, cfg.Packages)
	if err != nil {
		return 0, fmt.Errorf("detect changes: %w", err)
	}
	result := results[name]
	if len(result.Commits) == 0 {
		return 0, fmt.Errorf("no unreleased commits for %s", name)
	}
	newVersion, err := computeNewVersion(result)
	if err != nil {
		return 0, err
	}
	filePath := pkg.Path + "/package.json"
	content, fileSHA, err := api.GetFileContent(ctx, owner, repo, filePath)
	if err != nil {
		return 0, fmt.Errorf("read %s: %w", filePath, err)
	}
	updated, err := BumpNpmVersion(string(content), newVersion.String())
	if err != nil {
		return 0, fmt.Errorf("bump version: %w", err)
	}
	return openReleasePR(ctx, api, owner, repo, name, filePath, updated, fileSHA, result, newVersion, "npm publish")
}

// openReleasePR creates a release branch, commits the version bump and an
// updated CHANGELOG.md, then opens the PR. Returns the PR number.
func openReleasePR(ctx context.Context, api ReleaseAPI, owner, repo, name, filePath, updated, fileSHA string, result ChangeResult, newVersion Version, publishHint string) (int, error) {
	defaultBranch, err := api.GetDefaultBranch(ctx, owner, repo)
	if err != nil {
		return 0, fmt.Errorf("get default branch: %w", err)
	}
	branchSHA, err := api.GetBranchSHA(ctx, owner, repo, defaultBranch)
	if err != nil {
		return 0, fmt.Errorf("get branch SHA: %w", err)
	}

	releaseBranch := BranchName(name, newVersion.String())
	if err := api.CreateBranch(ctx, owner, repo, releaseBranch, branchSHA); err != nil {
		return 0, fmt.Errorf("create branch %s: %w", releaseBranch, err)
	}

	// Commit version bump
	commitMsg := fmt.Sprintf("chore(%s): bump version to %s", name, newVersion)
	if err := api.UpdateFile(ctx, owner, repo, filePath, commitMsg, []byte(updated), fileSHA, releaseBranch); err != nil {
		return 0, fmt.Errorf("commit version bump: %w", err)
	}

	// Commit CHANGELOG.md
	section := GenerateChangelogSection(name, newVersion.String(), time.Now(), result.Commits)
	changelogPath := path.Join(path.Dir(filePath), "CHANGELOG.md")
	var changelogContent string
	var changelogSHA string
	if existing, sha, err := api.GetFileContent(ctx, owner, repo, changelogPath); err == nil {
		changelogContent = PrependChangelog(string(existing), section)
		changelogSHA = sha
	} else {
		changelogContent = "# Changelog\n\n" + section
	}
	clMsg := fmt.Sprintf("chore(%s): update changelog for %s", name, newVersion)
	// Best-effort — don't fail the release if changelog commit fails
	_ = api.UpdateFile(ctx, owner, repo, changelogPath, clMsg, []byte(changelogContent), changelogSHA, releaseBranch)

	prTitle := fmt.Sprintf("release: %s v%s", name, newVersion)
	prBody := buildPRBody(name, result.CurrentVersion, newVersion.String(), result, publishHint)
	prNumber, err := api.CreatePR(ctx, owner, repo, prTitle, prBody, releaseBranch, defaultBranch)
	if err != nil {
		return 0, fmt.Errorf("create PR: %w", err)
	}
	return prNumber, nil
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

func buildPRBody(name, fromVersion, toVersion string, result ChangeResult, publishHint string) string {
	from := "initial"
	if fromVersion != "" {
		from = "v" + fromVersion
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "## Release %s v%s\n\n", name, toVersion)
	fmt.Fprintf(&sb, "Bump %s → v%s (%s)\n\n", from, toVersion, result.SuggestedBump)
	sb.WriteString(GenerateChangelogSection(name, toVersion, time.Now(), result.Commits))
	fmt.Fprintf(&sb, "---\n_Merging this PR will trigger `%s` via GitHub Actions._", publishHint)
	return sb.String()
}
