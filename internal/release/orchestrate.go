package release

import (
	"fmt"
	"strings"

	"github.com/vexil-lang/vexilbot/internal/repoconfig"
)

// BranchName returns the git branch name for a release.
func BranchName(crate, version string) string {
	return fmt.Sprintf("release/%s-v%s", crate, version)
}

// TagName returns the git tag name for a release using the configured format.
func TagName(crate, version, format string) string {
	result := strings.ReplaceAll(format, "{{ crate }}", crate)
	result = strings.ReplaceAll(result, "{{ version }}", version)
	return result
}

// ResolveDependencyOrder returns crate names in topological order (dependencies first).
func ResolveDependencyOrder(crates map[string]repoconfig.CrateEntry) ([]string, error) {
	inDegree := make(map[string]int)
	dependents := make(map[string][]string)

	for name := range crates {
		inDegree[name] = 0
	}

	for name, entry := range crates {
		for _, dep := range entry.DependsOn {
			if _, ok := crates[dep]; ok {
				inDegree[name]++
				dependents[dep] = append(dependents[dep], name)
			}
		}
	}

	var queue []string
	for name, degree := range inDegree {
		if degree == 0 {
			queue = append(queue, name)
		}
	}

	var order []string
	for len(queue) > 0 {
		sortStrings(queue)
		node := queue[0]
		queue = queue[1:]
		order = append(order, node)

		for _, dep := range dependents[node] {
			inDegree[dep]--
			if inDegree[dep] == 0 {
				queue = append(queue, dep)
			}
		}
	}

	if len(order) != len(crates) {
		return nil, fmt.Errorf("cyclic dependency detected in release crates")
	}

	return order, nil
}

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}
