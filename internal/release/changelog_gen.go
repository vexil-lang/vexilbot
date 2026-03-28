package release

import (
	"fmt"
	"strings"
	"time"
)

// GenerateChangelogSection produces a markdown changelog section from conventional commits,
// categorized by type (Breaking / Added / Fixed / Performance / Changed / Documentation).
func GenerateChangelogSection(name, version string, date time.Time, commits []Commit) string {
	type category struct {
		title string
		items []string
	}
	cats := []*category{
		{title: "Breaking Changes"},
		{title: "Added"},
		{title: "Fixed"},
		{title: "Performance"},
		{title: "Changed"},
		{title: "Documentation"},
		{title: "Other"},
	}
	breaking, added, fixed, perf, changed, docs, other :=
		cats[0], cats[1], cats[2], cats[3], cats[4], cats[5], cats[6]

	for _, c := range commits {
		line := c.Message
		if idx := strings.Index(line, "\n"); idx != -1 {
			line = line[:idx]
		}

		if isBreaking(c.Message) {
			breaking.items = append(breaking.items, line)
			continue
		}

		placed := false
		for _, match := range []struct {
			prefix string
			cat    *category
		}{
			{"feat", added},
			{"fix", fixed},
			{"perf", perf},
			{"refactor", changed},
			{"docs", docs},
		} {
			if strings.HasPrefix(line, match.prefix+":") || strings.HasPrefix(line, match.prefix+"(") {
				match.cat.items = append(match.cat.items, line)
				placed = true
				break
			}
		}
		if !placed {
			other.items = append(other.items, line)
		}
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "## [%s] - %s\n\n", version, date.Format("2006-01-02"))
	for _, cat := range cats {
		if len(cat.items) == 0 {
			continue
		}
		fmt.Fprintf(&sb, "### %s\n\n", cat.title)
		for _, item := range cat.items {
			fmt.Fprintf(&sb, "- %s\n", item)
		}
		sb.WriteString("\n")
	}
	return sb.String()
}

// PrependChangelog inserts a new section into an existing CHANGELOG.md,
// after any leading `# Changelog` title line, before the first `## [...]` section.
func PrependChangelog(existing, section string) string {
	lines := strings.Split(existing, "\n")
	insertAt := 0
	for i, line := range lines {
		if strings.HasPrefix(line, "# ") {
			// Skip past the title and any blank lines after it
			insertAt = i + 1
			for insertAt < len(lines) && strings.TrimSpace(lines[insertAt]) == "" {
				insertAt++
			}
			break
		}
		if strings.HasPrefix(line, "## ") {
			insertAt = i
			break
		}
	}

	before := strings.Join(lines[:insertAt], "\n")
	after := strings.Join(lines[insertAt:], "\n")

	var sb strings.Builder
	sb.WriteString(before)
	if before != "" && !strings.HasSuffix(before, "\n") {
		sb.WriteByte('\n')
	}
	sb.WriteString("\n")
	sb.WriteString(section)
	sb.WriteString(after)
	return sb.String()
}
