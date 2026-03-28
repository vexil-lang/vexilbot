package release

import (
	"context"
	"fmt"
)

type CmdRunner interface {
	Run(ctx context.Context, dir string, name string, args ...string) (string, error)
}

// GenerateChangelog runs git-cliff to produce a changelog for a crate release.
func GenerateChangelog(
	ctx context.Context,
	runner CmdRunner,
	repoDir string,
	crate string,
	sinceTag string,
	newVersion string,
) (string, error) {
	args := []string{
		"--config", "cliff.toml",
		"--tag", fmt.Sprintf("%s-v%s", crate, newVersion),
		"--unreleased",
	}

	if sinceTag != "" {
		args = append(args, sinceTag+"..HEAD")
	}

	output, err := runner.Run(ctx, repoDir, "git-cliff", args...)
	if err != nil {
		return "", fmt.Errorf("git-cliff: %w", err)
	}

	return output, nil
}
