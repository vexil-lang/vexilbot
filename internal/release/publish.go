package release

import (
	"context"
	"fmt"
)

// PublishCrate runs cargo publish for a crate.
func PublishCrate(ctx context.Context, runner CmdRunner, repoDir, cratePath, registryToken string) error {
	_, err := runner.Run(ctx, repoDir, "cargo", "publish",
		"--manifest-path", cratePath+"/Cargo.toml",
		"--token", registryToken,
	)
	if err != nil {
		return fmt.Errorf("cargo publish %s: %w", cratePath, err)
	}
	return nil
}
