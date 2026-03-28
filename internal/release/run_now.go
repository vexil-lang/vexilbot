package release

import (
	"context"
	"errors"
	"fmt"

	"github.com/vexil-lang/vexilbot/internal/repoconfig"
)

// ErrNotFound is returned by RunReleaseNow when the package/crate name is not
// found in the repo config.
var ErrNotFound = errors.New("package not found in release config")

// RunReleaseNow creates a release PR for the named crate or npm package and
// returns the new PR number. Unlike RunRelease, it does not post a comment to
// any GitHub issue. Intended for dashboard "Run now" actions.
func RunReleaseNow(ctx context.Context, api ReleaseAPI, owner, repo, name string, cfg repoconfig.Release) (int, error) {
	if crate, ok := cfg.Crates[name]; ok {
		return createCratePR(ctx, api, owner, repo, name, crate, cfg)
	}
	if pkg, ok := cfg.Packages[name]; ok {
		return createNpmPR(ctx, api, owner, repo, name, pkg, cfg)
	}
	return 0, fmt.Errorf("%w: %s", ErrNotFound, name)
}
