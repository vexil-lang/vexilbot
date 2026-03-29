package release

import (
	"context"
	"fmt"
	"sort"

	"github.com/vexil-lang/vexilbot/internal/repoconfig"
)

type Commit struct {
	SHA     string
	Message string
	Files   []string
}

type ChangeResult struct {
	CrateName      string
	CurrentVersion string
	Commits        []Commit
	SuggestedBump  BumpLevel
}

type GitAPI interface {
	ListTags(ctx context.Context, owner, repo string) ([]string, error)
	CommitsSinceTag(ctx context.Context, owner, repo, tag, path string) ([]Commit, error)
}

// DetectChanges checks each configured crate for unreleased commits.
func DetectChanges(
	ctx context.Context,
	api GitAPI,
	owner, repo string,
	tagFormat string,
	crates map[string]repoconfig.CrateEntry,
) (map[string]ChangeResult, error) {
	paths := make(map[string]string, len(crates))
	for name, c := range crates {
		paths[name] = c.Path
	}
	return detectByPaths(ctx, api, owner, repo, tagFormat, paths)
}

// DetectPackageChanges checks each configured npm package for unreleased commits.
func DetectPackageChanges(
	ctx context.Context,
	api GitAPI,
	owner, repo string,
	tagFormat string,
	packages map[string]repoconfig.PackageEntry,
) (map[string]ChangeResult, error) {
	paths := make(map[string]string, len(packages))
	for name, p := range packages {
		paths[name] = p.Path
	}
	return detectByPaths(ctx, api, owner, repo, tagFormat, paths)
}

func detectByPaths(
	ctx context.Context,
	api GitAPI,
	owner, repo string,
	tagFormat string,
	namePaths map[string]string,
) (map[string]ChangeResult, error) {
	tags, err := api.ListTags(ctx, owner, repo)
	if err != nil {
		return nil, fmt.Errorf("list tags: %w", err)
	}

	results := make(map[string]ChangeResult)

	for name, path := range namePaths {
		latestTag, latestVersion := findLatestTag(tags, name, tagFormat)

		commits, err := api.CommitsSinceTag(ctx, owner, repo, latestTag, path)
		if err != nil {
			return nil, fmt.Errorf("commits for %s: %w", name, err)
		}

		messages := make([]string, len(commits))
		for i, c := range commits {
			messages[i] = c.Message
		}

		results[name] = ChangeResult{
			CrateName:      name,
			CurrentVersion: latestVersion,
			Commits:        commits,
			SuggestedBump:  SuggestBump(messages),
		}
	}

	return results, nil
}

func findLatestTag(tags []string, crate, format string) (string, string) {
	type tagVersion struct {
		tag     string
		version Version
		raw     string
	}

	var matches []tagVersion
	for _, tag := range tags {
		ver, ok := ExtractTagVersion(tag, crate, format)
		if !ok {
			continue
		}
		parsed, err := ParseVersion(ver)
		if err != nil {
			continue
		}
		matches = append(matches, tagVersion{tag, parsed, ver})
	}

	if len(matches) == 0 {
		return "", ""
	}

	sort.Slice(matches, func(i, j int) bool {
		a, b := matches[i].version, matches[j].version
		if a.Major != b.Major {
			return a.Major > b.Major
		}
		if a.Minor != b.Minor {
			return a.Minor > b.Minor
		}
		return a.Patch > b.Patch
	})

	return matches[0].tag, matches[0].raw
}

// PackageStatus summarises the release status for one package/crate.
type PackageStatus struct {
	Name    string
	Version string
	Bump    string // "major", "minor", "patch", or "none"
	Commits int
}

// GetStatus returns release status for all configured crates and packages in cfg.
func GetStatus(ctx context.Context, api GitAPI, owner, repo string, cfg repoconfig.Release) ([]PackageStatus, error) {
	tagFmt := cfg.TagFormat
	if tagFmt == "" {
		tagFmt = "{name}-v{version}"
	}
	var results []PackageStatus
	if len(cfg.Crates) > 0 {
		changes, err := DetectChanges(ctx, api, owner, repo, tagFmt, cfg.Crates)
		if err != nil {
			return nil, err
		}
		for name, cr := range changes {
			results = append(results, PackageStatus{
				Name:    name,
				Version: cr.CurrentVersion,
				Bump:    bumpLevelStr(cr.SuggestedBump, len(cr.Commits)),
				Commits: len(cr.Commits),
			})
		}
	}
	if len(cfg.Packages) > 0 {
		changes, err := DetectPackageChanges(ctx, api, owner, repo, tagFmt, cfg.Packages)
		if err != nil {
			return nil, err
		}
		for name, cr := range changes {
			results = append(results, PackageStatus{
				Name:    name,
				Version: cr.CurrentVersion,
				Bump:    bumpLevelStr(cr.SuggestedBump, len(cr.Commits)),
				Commits: len(cr.Commits),
			})
		}
	}
	sort.Slice(results, func(i, j int) bool { return results[i].Name < results[j].Name })
	return results, nil
}

func bumpLevelStr(b BumpLevel, commits int) string {
	if commits == 0 {
		return "none"
	}
	switch b {
	case BumpMajor:
		return "major"
	case BumpMinor:
		return "minor"
	default:
		return "patch"
	}
}

// FormatStatus produces a markdown summary of unreleased changes.
func FormatStatus(results map[string]ChangeResult) string {
	var lines []string
	for name, r := range results {
		if len(r.Commits) == 0 {
			continue
		}
		line := fmt.Sprintf("- **%s** (v%s): %d unreleased commit(s), suggested bump: **%s**",
			name, r.CurrentVersion, len(r.Commits), r.SuggestedBump)
		lines = append(lines, line)
	}

	if len(lines) == 0 {
		return "All crates are up to date. No unreleased changes."
	}

	sort.Strings(lines)
	result := "### Release Status\n\n"
	for _, l := range lines {
		result += l + "\n"
	}
	return result
}
