package labeler

import (
	"path"
	"strings"
)

// MatchGlob checks if a file path matches a glob pattern.
// Supports ** for recursive directory matching.
// Always uses forward slashes as path separator.
func MatchGlob(pattern, filePath string) bool {
	// Normalize to forward slashes
	pattern = strings.ReplaceAll(pattern, "\\", "/")
	filePath = strings.ReplaceAll(filePath, "\\", "/")

	if strings.Contains(pattern, "**") {
		return matchDoubleGlob(pattern, filePath)
	}
	matched, _ := path.Match(pattern, filePath)
	return matched
}

func matchDoubleGlob(pattern, filePath string) bool {
	if strings.HasSuffix(pattern, "/**") {
		prefix := strings.TrimSuffix(pattern, "/**")
		return strings.HasPrefix(filePath, prefix+"/")
	}

	parts := strings.SplitN(pattern, "/**/", 2)
	if len(parts) == 2 {
		prefix := parts[0]
		suffix := parts[1]
		if !strings.HasPrefix(filePath, prefix+"/") {
			return false
		}
		rest := strings.TrimPrefix(filePath, prefix+"/")
		matched, _ := path.Match(suffix, path.Base(rest))
		if matched {
			return true
		}
		for i := range rest {
			if rest[i] == '/' {
				candidate := rest[i+1:]
				matched, _ := path.Match(suffix, candidate)
				if matched {
					return true
				}
			}
		}
	}

	return false
}
