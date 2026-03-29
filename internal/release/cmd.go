package release

import (
	"context"
	"fmt"
	"log/slog"
	"path"
	"strings"
	"time"

	"github.com/vexil-lang/vexilbot/internal/repoconfig"
)

// ReleaseAPI is the GitHub API surface needed by release commands.
type ReleaseAPI interface {
	GitAPI
	GetFileContent(ctx context.Context, owner, repo, path string) ([]byte, string, error)
	GetFileContentRef(ctx context.Context, owner, repo, path, ref string) ([]byte, string, error)
	GetDefaultBranch(ctx context.Context, owner, repo string) (string, error)
	GetBranchSHA(ctx context.Context, owner, repo, branch string) (string, error)
	CreateBranch(ctx context.Context, owner, repo, branch, sha string) error
	UpdateFile(ctx context.Context, owner, repo, path, message string, content []byte, sha, branch string) error
	CreatePR(ctx context.Context, owner, repo, title, body, head, base string) (int, error)
	MergePR(ctx context.Context, owner, repo string, number int, method string) error
	CreateTag(ctx context.Context, owner, repo, tag, sha string) error
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
			// Skip crates with publish = false
			if crate, ok := cfg.Crates[k]; ok && isPublishDisabled(crate.Publish) {
				continue
			}
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
		if isPublishDisabled(crate.Publish) {
			return api.CreateComment(ctx, owner, repo, issueNumber,
				fmt.Sprintf(":x: **%s** has `publish = false` — skipping release.", name))
		}
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

// releaseItem tracks one crate or package being released.
type releaseItem struct {
	name       string
	kind       string // "cargo" or "npm"
	oldVersion string
	newVersion Version
	commits    []Commit
	bump       BumpLevel
}

// RunWorkspaceRelease creates a single atomic release PR that bumps ALL
// crates and packages with unreleased changes. One PR, one merge, one CI
// run. Version bumps, dependency constraint updates, and changelogs are
// all committed to the same branch.
func RunWorkspaceRelease(ctx context.Context, api ReleaseAPI, owner, repo string, issueNumber int, cfg repoconfig.Release) error {
	if len(cfg.Crates) == 0 && len(cfg.Packages) == 0 {
		return api.CreateComment(ctx, owner, repo, issueNumber,
			"No crates or packages configured under `[release]` in `.vexilbot.toml`.")
	}

	// --- Detect all changes ---
	var items []releaseItem
	crateVersions := make(map[string]Version) // name → new version (for dep constraint updates)

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
			crate := cfg.Crates[name]
			if isPublishDisabled(crate.Publish) {
				continue
			}
			result := results[name]
			if len(result.Commits) == 0 {
				continue
			}
			newVersion, err := computeNewVersion(result)
			if err != nil {
				return fmt.Errorf("compute version for %s: %w", name, err)
			}
			crateVersions[name] = newVersion
			items = append(items, releaseItem{
				name: name, kind: "cargo",
				oldVersion: result.CurrentVersion,
				newVersion: newVersion,
				commits:    result.Commits,
				bump:       result.SuggestedBump,
			})
		}
	}

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
			result := results[name]
			if len(result.Commits) == 0 {
				continue
			}
			newVersion, err := computeNewVersion(result)
			if err != nil {
				return fmt.Errorf("compute version for %s: %w", name, err)
			}
			items = append(items, releaseItem{
				name: name, kind: "npm",
				oldVersion: result.CurrentVersion,
				newVersion: newVersion,
				commits:    result.Commits,
				bump:       result.SuggestedBump,
			})
		}
	}

	if len(items) == 0 {
		return api.CreateComment(ctx, owner, repo, issueNumber,
			"No unreleased changes found — workspace is up to date.")
	}

	// --- Create a single release branch ---
	defaultBranch, err := api.GetDefaultBranch(ctx, owner, repo)
	if err != nil {
		return fmt.Errorf("get default branch: %w", err)
	}
	branchSHA, err := api.GetBranchSHA(ctx, owner, repo, defaultBranch)
	if err != nil {
		return fmt.Errorf("get branch SHA: %w", err)
	}

	releaseBranch := fmt.Sprintf("release/workspace-%s", time.Now().UTC().Format("2006-01-02"))
	if err := api.CreateBranch(ctx, owner, repo, releaseBranch, branchSHA); err != nil {
		return fmt.Errorf("create branch %s: %w", releaseBranch, err)
	}

	// --- Commit all version bumps + dependency updates + changelogs ---
	// Track files we've already updated on this branch (SHA changes after each commit)
	updatedSHAs := make(map[string]string)

	for _, item := range items {
		var filePath string
		var content []byte
		var fileSHA string

		if item.kind == "cargo" {
			crate := cfg.Crates[item.name]
			filePath = crate.Path + "/Cargo.toml"
		} else {
			pkg := cfg.Packages[item.name]
			filePath = pkg.Path + "/package.json"
		}

		// Read the file (use branch version if we've already committed to it)
		content, fileSHA, err = api.GetFileContent(ctx, owner, repo, filePath)
		if err != nil {
			return fmt.Errorf("read %s: %w", filePath, err)
		}
		if sha, ok := updatedSHAs[filePath]; ok {
			fileSHA = sha
			// Re-read from branch to get the latest content
			content, fileSHA, err = getFileFromBranch(ctx, api, owner, repo, filePath, releaseBranch)
			if err != nil {
				return fmt.Errorf("re-read %s from branch: %w", filePath, err)
			}
		}

		// Bump version
		var updated string
		if item.kind == "cargo" {
			updated, err = BumpCargoVersion(string(content), item.newVersion.String())
		} else {
			updated, err = BumpNpmVersion(string(content), item.newVersion.String())
		}
		if err != nil {
			return fmt.Errorf("bump %s version: %w", item.name, err)
		}

		// For crates: also update dependency constraints in the same file
		// (this crate might depend on another crate being bumped in the same release)
		if item.kind == "cargo" {
			for depName, depVersion := range crateVersions {
				if depName == item.name {
					continue
				}
				constraint := fmt.Sprintf("^%s", depVersion)
				if bumped, err := BumpCargoDependency(updated, depName, constraint); err != nil {
					slog.Error("bump dependency constraint", "crate", item.name, "dep", depName, "error", err)
				} else {
					updated = bumped
				}
			}
		}

		commitMsg := fmt.Sprintf("chore(%s): bump to %s", item.name, item.newVersion)
		if err := api.UpdateFile(ctx, owner, repo, filePath, commitMsg, []byte(updated), fileSHA, releaseBranch); err != nil {
			return fmt.Errorf("commit %s version bump: %w", item.name, err)
		}

		// Track that this file was updated (for dependency constraint updates)
		// We need the new SHA for subsequent updates to the same file
		if _, newSHA, err := getFileFromBranch(ctx, api, owner, repo, filePath, releaseBranch); err == nil {
			updatedSHAs[filePath] = newSHA
		}

		// Update dependency constraints in OTHER crates that depend on this one
		if item.kind == "cargo" {
			constraint := fmt.Sprintf("^%s", item.newVersion)
			for depName, depEntry := range cfg.Crates {
				if depName == item.name {
					continue
				}
				isDep := false
				for _, d := range depEntry.DependsOn {
					if d == item.name {
						isDep = true
						break
					}
				}
				if !isDep {
					continue
				}
				depPath := depEntry.Path + "/Cargo.toml"
				var depContent []byte
				var depSHA string
				if _, ok := updatedSHAs[depPath]; ok {
					depContent, depSHA, err = getFileFromBranch(ctx, api, owner, repo, depPath, releaseBranch)
				} else {
					depContent, depSHA, err = api.GetFileContent(ctx, owner, repo, depPath)
				}
				if err != nil {
					continue
				}
				depUpdated, err := BumpCargoDependency(string(depContent), item.name, constraint)
				if err != nil || depUpdated == string(depContent) {
					continue
				}
				depMsg := fmt.Sprintf("chore(%s): update %s dep to %s", depName, item.name, constraint)
				if err := api.UpdateFile(ctx, owner, repo, depPath, depMsg, []byte(depUpdated), depSHA, releaseBranch); err == nil {
					if _, newSHA, err := getFileFromBranch(ctx, api, owner, repo, depPath, releaseBranch); err == nil {
						updatedSHAs[depPath] = newSHA
					}
				}
			}
		}

		// Commit changelog (best-effort)
		var changelogDir string
		if item.kind == "cargo" {
			changelogDir = cfg.Crates[item.name].Path
		} else {
			changelogDir = cfg.Packages[item.name].Path
		}
		changelogPath := changelogDir + "/CHANGELOG.md"
		section := GenerateChangelogSection(item.name, item.newVersion.String(), time.Now(), item.commits)
		var changelogContent string
		var changelogSHA string
		if existing, sha, err := api.GetFileContent(ctx, owner, repo, changelogPath); err == nil {
			changelogContent = PrependChangelog(string(existing), section)
			changelogSHA = sha
		} else {
			changelogContent = "# Changelog\n\n" + section
		}
		clMsg := fmt.Sprintf("chore(%s): update changelog for %s", item.name, item.newVersion)
		_ = api.UpdateFile(ctx, owner, repo, changelogPath, clMsg, []byte(changelogContent), changelogSHA, releaseBranch)
	}

	// --- Open one PR ---
	prTitle, prBody := buildWorkspacePRBody(items)
	prNumber, err := api.CreatePR(ctx, owner, repo, prTitle, prBody, releaseBranch, defaultBranch)
	if err != nil {
		return fmt.Errorf("create PR: %w", err)
	}

	var summary []string
	for _, item := range items {
		summary = append(summary, fmt.Sprintf("- **%s** v%s (%s)", item.name, item.newVersion, item.kind))
	}
	return api.CreateComment(ctx, owner, repo, issueNumber,
		fmt.Sprintf("Created release PR #%d:\n\n%s\n\nMerging it will publish all crates and packages.", prNumber, strings.Join(summary, "\n")))
}

// getFileFromBranch reads a file from a specific branch.
func getFileFromBranch(ctx context.Context, api ReleaseAPI, owner, repo, filePath, branch string) ([]byte, string, error) {
	return api.GetFileContentRef(ctx, owner, repo, filePath, branch)
}

func buildWorkspacePRBody(items []releaseItem) (string, string) {
	var maxBump BumpLevel
	for _, item := range items {
		if item.bump > maxBump {
			maxBump = item.bump
		}
	}

	title := "release: workspace"
	var sb strings.Builder
	sb.WriteString("## Workspace Release\n\n")
	sb.WriteString("| Crate/Package | Version | Bump | Commits |\n")
	sb.WriteString("|---|---|---|---|\n")
	for _, item := range items {
		from := "initial"
		if item.oldVersion != "" {
			from = item.oldVersion
		}
		sb.WriteString(fmt.Sprintf("| %s | %s → %s | %s | %d |\n",
			item.name, from, item.newVersion, item.bump, len(item.commits)))
	}
	sb.WriteString("\n")

	for _, item := range items {
		sb.WriteString(fmt.Sprintf("### %s v%s\n\n", item.name, item.newVersion))
		sb.WriteString(GenerateChangelogSection(item.name, item.newVersion.String(), time.Now(), item.commits))
		sb.WriteString("\n")
	}

	sb.WriteString("---\n_Merging this PR will trigger `cargo publish` and `npm publish` via GitHub Actions._")
	return title, sb.String()
}

// RunPostMerge handles post-merge actions for a release PR: creates per-crate
// git tags and runs cargo publish in dependency order. Called when a PR with
// branch matching "release/workspace-*" is merged.
func RunPostMerge(ctx context.Context, api ReleaseAPI, runner CmdRunner, owner, repo string, prNumber int, mergeSHA string, cfg repoconfig.Release) error {
	// Detect which crates have new versions by reading their Cargo.toml from the merged commit
	var published []string
	var tagged []string

	order, err := ResolveDependencyOrder(cfg.Crates)
	if err != nil {
		return fmt.Errorf("resolve dependency order: %w", err)
	}

	for _, name := range order {
		crate, ok := cfg.Crates[name]
		if !ok || isPublishDisabled(crate.Publish) {
			continue
		}

		// Read the crate's version from the merged code
		filePath := crate.Path + "/Cargo.toml"
		content, _, err := api.GetFileContent(ctx, owner, repo, filePath)
		if err != nil {
			slog.Error("read Cargo.toml for post-merge", "crate", name, "error", err)
			continue
		}

		version := extractCargoVersion(string(content))
		if version == "" {
			slog.Error("could not extract version", "crate", name)
			continue
		}

		tag := TagName(name, version, cfg.TagFormat)

		// Create the git tag
		if err := api.CreateTag(ctx, owner, repo, tag, mergeSHA); err != nil {
			slog.Error("create tag", "tag", tag, "error", err)
		} else {
			tagged = append(tagged, tag)
			slog.Info("created tag", "tag", tag)
		}

		// Run cargo publish (best-effort — don't stop on failure)
		if err := PublishCrate(ctx, runner, ".", crate.Path, ""); err != nil {
			slog.Error("cargo publish", "crate", name, "error", err)
		} else {
			published = append(published, name)
			slog.Info("published crate", "name", name, "version", version)
		}

		// Brief pause for crates.io indexing
		time.Sleep(5 * time.Second)
	}

	// Build summary
	var sb strings.Builder
	sb.WriteString("## Post-Merge Release\n\n")
	if len(tagged) > 0 {
		sb.WriteString("**Tags created:**\n")
		for _, t := range tagged {
			sb.WriteString(fmt.Sprintf("- `%s`\n", t))
		}
		sb.WriteString("\n")
	}
	if len(published) > 0 {
		sb.WriteString("**Published to crates.io:**\n")
		for _, p := range published {
			sb.WriteString(fmt.Sprintf("- %s\n", p))
		}
		sb.WriteString("\n")
	}
	sb.WriteString("_Tags trigger cargo-dist (vexilc binaries) and npm publish (@vexil-lang/runtime) via GitHub Actions._")

	return api.CreateComment(ctx, owner, repo, prNumber, sb.String())
}

// extractCargoVersion reads the version from a Cargo.toml string.
func extractCargoVersion(content string) string {
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "version") && strings.Contains(trimmed, "=") {
			parts := strings.SplitN(trimmed, "=", 2)
			if len(parts) == 2 {
				v := strings.TrimSpace(parts[1])
				v = strings.Trim(v, `"`)
				return v
			}
		}
	}
	return ""
}

// --- internal helpers ---

// fileUpdate holds a pending file change to commit to the release branch.
type fileUpdate struct {
	path    string
	content string
	sha     string
	message string
}

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

	// Bump the crate's own version
	filePath := crate.Path + "/Cargo.toml"
	content, fileSHA, err := api.GetFileContent(ctx, owner, repo, filePath)
	if err != nil {
		return 0, fmt.Errorf("read %s: %w", filePath, err)
	}
	updated, err := BumpCargoVersion(string(content), newVersion.String())
	if err != nil {
		return 0, fmt.Errorf("bump version: %w", err)
	}

	// Collect dependent crate Cargo.toml updates — any crate that lists this
	// crate in depends_on needs its version constraint updated.
	versionConstraint := fmt.Sprintf("^%s", newVersion)
	var depUpdates []fileUpdate
	for depName, depEntry := range cfg.Crates {
		if depName == name {
			continue
		}
		for _, dep := range depEntry.DependsOn {
			if dep == name {
				depPath := depEntry.Path + "/Cargo.toml"
				depContent, depSHA, err := api.GetFileContent(ctx, owner, repo, depPath)
				if err != nil {
					continue // best-effort: skip if file can't be read
				}
				depUpdated, err := BumpCargoDependency(string(depContent), name, versionConstraint)
				if err != nil || depUpdated == string(depContent) {
					continue // no change needed or error
				}
				depUpdates = append(depUpdates, fileUpdate{
					path:    depPath,
					content: depUpdated,
					sha:     depSHA,
					message: fmt.Sprintf("chore(%s): update %s dependency to %s", depName, name, versionConstraint),
				})
				break
			}
		}
	}

	return openReleasePR(ctx, api, owner, repo, name, filePath, updated, fileSHA, result, newVersion, "cargo publish", depUpdates)
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
	return openReleasePR(ctx, api, owner, repo, name, filePath, updated, fileSHA, result, newVersion, "npm publish", nil)
}

// openReleasePR creates a release branch, commits the version bump, any
// dependency constraint updates, and an updated CHANGELOG.md, then opens
// the PR. Returns the PR number.
func openReleasePR(ctx context.Context, api ReleaseAPI, owner, repo, name, filePath, updated, fileSHA string, result ChangeResult, newVersion Version, publishHint string, depUpdates []fileUpdate) (int, error) {
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

	// Commit dependency constraint updates in dependent crates
	for _, du := range depUpdates {
		_ = api.UpdateFile(ctx, owner, repo, du.path, du.message, []byte(du.content), du.sha, releaseBranch)
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

// isPublishDisabled checks if a CrateEntry's Publish field is false.
// Publish is interface{} — can be string ("crates.io") or bool (false).
func isPublishDisabled(publish interface{}) bool {
	if publish == nil {
		return false // nil means default (publishable)
	}
	if b, ok := publish.(bool); ok {
		return !b
	}
	return false // string value means a registry name
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
