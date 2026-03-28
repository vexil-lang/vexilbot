package release

import (
	"fmt"
	"regexp"
)

var npmVersionRe = regexp.MustCompile(`("version"\s*:\s*)"[^"]*"`)

// BumpNpmVersion replaces the version field in a package.json file,
// preserving all other formatting.
func BumpNpmVersion(content, newVersion string) (string, error) {
	replaced := false
	result := npmVersionRe.ReplaceAllStringFunc(content, func(match string) string {
		if replaced {
			return match
		}
		replaced = true
		return npmVersionRe.ReplaceAllString(match, `${1}"`+newVersion+`"`)
	})
	if !replaced {
		return "", fmt.Errorf("no version field found in package.json")
	}
	return result, nil
}
