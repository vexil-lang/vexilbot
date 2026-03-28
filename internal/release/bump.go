package release

import (
	"fmt"
	"regexp"
	"strings"
)

var workspaceVersionRe = regexp.MustCompile(`(?m)^version\.workspace\s*=\s*true`)

// BumpCargoVersion replaces the version in a Cargo.toml [package] section.
func BumpCargoVersion(content string, newVersion string) (string, error) {
	if workspaceVersionRe.MatchString(content) {
		return "", fmt.Errorf("version is inherited from workspace — bump the workspace Cargo.toml instead")
	}

	// Find [package] section and replace version = "x.y.z" within it only
	sections := splitSections(content)
	replaced := false

	packageVersionRe := regexp.MustCompile(`^(version\s*=\s*)"([^"]+)"`)

	for i, section := range sections {
		if isPackageSection(section) {
			lines := strings.Split(section, "\n")
			for j, line := range lines {
				if packageVersionRe.MatchString(line) && !replaced {
					lines[j] = packageVersionRe.ReplaceAllString(line, `${1}"`+newVersion+`"`)
					replaced = true
					break
				}
			}
			sections[i] = strings.Join(lines, "\n")
			break
		}
	}

	if !replaced {
		return "", fmt.Errorf("no version field found in [package] section")
	}

	return strings.Join(sections, ""), nil
}

// BumpCargoDependency updates a dependency version in a Cargo.toml.
func BumpCargoDependency(content string, depName, newVersion string) (string, error) {
	pattern := regexp.MustCompile(
		fmt.Sprintf(`(%s\s*=\s*\{[^}]*version\s*=\s*)"([^"]+)"`, regexp.QuoteMeta(depName)),
	)

	if !pattern.MatchString(content) {
		return content, nil
	}

	result := pattern.ReplaceAllString(content, `${1}"`+newVersion+`"`)
	return result, nil
}

func splitSections(content string) []string {
	var sections []string
	lines := strings.Split(content, "\n")
	current := ""

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[") && !strings.HasPrefix(trimmed, "[[") && current != "" {
			sections = append(sections, current)
			current = ""
		}
		// Don't append a trailing newline to the very last element if it's empty
		// (preserves the original file's trailing newline exactly)
		if i == len(lines)-1 && line == "" {
			break
		}
		current += line + "\n"
	}
	if current != "" {
		sections = append(sections, current)
	}
	return sections
}

func isPackageSection(section string) bool {
	for _, line := range strings.Split(section, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "[package]" {
			return true
		}
		if strings.HasPrefix(trimmed, "[") {
			return false
		}
	}
	return false
}
