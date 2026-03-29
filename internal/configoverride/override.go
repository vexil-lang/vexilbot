package configoverride

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
	"github.com/vexil-lang/vexilbot/internal/repoconfig"
)

// Path returns the canonical override file path for a repo.
func Path(dataDir, owner, repo string) string {
	return filepath.Join(dataDir, "overrides", owner+"-"+repo+".toml")
}

// ServerPath returns the override file path for server hot-editable fields.
func ServerPath(dataDir string) string {
	return filepath.Join(dataDir, "overrides", "server.toml")
}

// Load reads the override file. Returns nil, nil if the file does not exist.
func Load(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	return data, err
}

// Save writes content to path, creating parent directories as needed.
func Save(path string, content []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, content, 0o644)
}

// Delete removes the override file. Does not error if it does not exist.
func Delete(path string) error {
	err := os.Remove(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

// Merge applies the override file at path on top of base.
// If the file does not exist, base is returned unchanged (as a copy).
// Merge is section-level: if a section key is present in the override file,
// the whole section replaces the base section.
func Merge(base *repoconfig.Config, path string) (*repoconfig.Config, error) {
	data, err := Load(path)
	if err != nil {
		return nil, err
	}
	result := *base // shallow copy
	if data == nil {
		return &result, nil
	}
	var ov repoconfig.Config
	meta, err := toml.Decode(string(data), &ov)
	if err != nil {
		return nil, err
	}
	if meta.IsDefined("labels") {
		result.Labels = ov.Labels
	}
	if meta.IsDefined("welcome") {
		// Merge Welcome field by field
		if ov.Welcome.PRMessage != "" {
			result.Welcome.PRMessage = ov.Welcome.PRMessage
		}
		if ov.Welcome.IssueMessage != "" {
			result.Welcome.IssueMessage = ov.Welcome.IssueMessage
		}
	}
	if meta.IsDefined("policy") {
		result.Policy = ov.Policy
	}
	if meta.IsDefined("triage") {
		result.Triage = ov.Triage
	}
	if meta.IsDefined("release") {
		result.Release = ov.Release
	}
	return &result, nil
}
