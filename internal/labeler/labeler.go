package labeler

import (
	"sort"
	"strings"

	"github.com/vexil-lang/vexilbot/internal/repoconfig"
)

// MatchPathLabels returns labels whose glob patterns match any of the changed files.
func MatchPathLabels(cfg repoconfig.Labels, changedFiles []string) []string {
	matched := make(map[string]bool)

	for label, patterns := range cfg.Paths {
		for _, pattern := range patterns {
			for _, file := range changedFiles {
				if MatchGlob(pattern, file) {
					matched[label] = true
					break
				}
			}
			if matched[label] {
				break
			}
		}
	}

	labels := make([]string, 0, len(matched))
	for l := range matched {
		labels = append(labels, l)
	}
	sort.Strings(labels)
	return labels
}

// MatchKeywordLabels returns labels whose keywords appear in the title or body.
func MatchKeywordLabels(cfg repoconfig.Labels, title, body string) []string {
	text := strings.ToLower(title + " " + body)
	matched := make(map[string]bool)

	for label, keywords := range cfg.Keywords {
		for _, kw := range keywords {
			if strings.Contains(text, strings.ToLower(kw)) {
				matched[label] = true
				break
			}
		}
	}

	labels := make([]string, 0, len(matched))
	for l := range matched {
		labels = append(labels, l)
	}
	sort.Strings(labels)
	return labels
}
