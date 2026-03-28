package policy

import (
	"context"

	"github.com/vexil-lang/vexilbot/internal/labeler"
)

// CheckWireFormatWarning posts an advisory comment if the PR touches wire format paths.
func CheckWireFormatWarning(
	ctx context.Context,
	api PolicyAPI,
	owner, repo string,
	number int,
	warningPaths []string,
	changedFiles []string,
) (bool, error) {
	for _, file := range changedFiles {
		for _, pattern := range warningPaths {
			if labeler.MatchGlob(pattern, file) {
				comment := "⚠️ This PR touches wire format code. " +
					"Changes to wire encoding require a 14-day RFC comment period per GOVERNANCE.md."
				if err := api.CreateComment(ctx, owner, repo, number, comment); err != nil {
					return true, err
				}
				return true, nil
			}
		}
	}
	return false, nil
}
