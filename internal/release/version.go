package release

import (
	"fmt"
	"strconv"
	"strings"
)

type Version struct {
	Major int
	Minor int
	Patch int
}

type BumpLevel int

const (
	BumpPatch BumpLevel = iota
	BumpMinor
	BumpMajor
)

func (b BumpLevel) String() string {
	switch b {
	case BumpPatch:
		return "patch"
	case BumpMinor:
		return "minor"
	case BumpMajor:
		return "major"
	default:
		return "unknown"
	}
}

func ParseVersion(s string) (Version, error) {
	parts := strings.Split(s, ".")
	if len(parts) != 3 {
		return Version{}, fmt.Errorf("invalid version %q: expected major.minor.patch", s)
	}

	major, err := strconv.Atoi(parts[0])
	if err != nil {
		return Version{}, fmt.Errorf("invalid major version: %w", err)
	}
	minor, err := strconv.Atoi(parts[1])
	if err != nil {
		return Version{}, fmt.Errorf("invalid minor version: %w", err)
	}
	patch, err := strconv.Atoi(parts[2])
	if err != nil {
		return Version{}, fmt.Errorf("invalid patch version: %w", err)
	}

	return Version{Major: major, Minor: minor, Patch: patch}, nil
}

func (v Version) Bump(level BumpLevel) Version {
	switch level {
	case BumpPatch:
		return Version{v.Major, v.Minor, v.Patch + 1}
	case BumpMinor:
		return Version{v.Major, v.Minor + 1, 0}
	case BumpMajor:
		return Version{v.Major + 1, 0, 0}
	default:
		return v
	}
}

func (v Version) String() string {
	return fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
}

// SuggestBump analyzes conventional commit messages and suggests a bump level.
func SuggestBump(messages []string) BumpLevel {
	level := BumpPatch
	for _, msg := range messages {
		if isBreaking(msg) {
			return BumpMajor
		}
		colonIdx := strings.Index(msg, ":")
		if colonIdx > 0 {
			prefix := msg[:colonIdx]
			if prefix == "feat" || strings.HasPrefix(prefix, "feat(") {
				level = BumpMinor
			}
		}
	}
	return level
}

func isBreaking(msg string) bool {
	colonIdx := strings.Index(msg, ":")
	if colonIdx > 0 {
		prefix := msg[:colonIdx]
		if strings.HasSuffix(prefix, "!") {
			return true
		}
	}
	if strings.Contains(msg, "BREAKING CHANGE") || strings.Contains(msg, "BREAKING-CHANGE") {
		return true
	}
	return false
}

// ExtractTagVersion extracts the version string from a git tag for a given crate.
func ExtractTagVersion(tag, crate, format string) (string, bool) {
	prefix := strings.Replace(format, "{{ version }}", "", 1)
	prefix = strings.Replace(prefix, "{{ crate }}", crate, 1)

	if !strings.HasPrefix(tag, prefix) {
		return "", false
	}

	version := tag[len(prefix):]
	if version == "" {
		return "", false
	}

	if _, err := ParseVersion(version); err != nil {
		return "", false
	}

	return version, true
}
