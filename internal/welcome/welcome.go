package welcome

import (
	"context"
)

type ContribAPI interface {
	CountUserPRs(ctx context.Context, owner, repo, user string) (int, error)
	CountUserIssues(ctx context.Context, owner, repo, user string) (int, error)
	CreateComment(ctx context.Context, owner, repo string, number int, body string) error
}

// MaybeWelcomePR posts a welcome message on a PR if the author is a first-time contributor.
// CountUserPRs is expected to include the current PR in its count, so > 1 means prior PRs exist.
func MaybeWelcomePR(ctx context.Context, api ContribAPI, owner, repo, user string, number int, message string) error {
	if message == "" {
		return nil
	}
	prs, err := api.CountUserPRs(ctx, owner, repo, user)
	if err != nil {
		return err
	}
	if prs > 1 {
		return nil
	}
	issues, err := api.CountUserIssues(ctx, owner, repo, user)
	if err != nil {
		return err
	}
	if issues > 0 {
		return nil
	}
	return api.CreateComment(ctx, owner, repo, number, message)
}

// MaybeWelcomeIssue posts a welcome message on an issue if the author is a first-time contributor.
// CountUserIssues is expected to include the current issue in its count, so > 1 means prior issues exist.
func MaybeWelcomeIssue(ctx context.Context, api ContribAPI, owner, repo, user string, number int, message string) error {
	if message == "" {
		return nil
	}
	prs, err := api.CountUserPRs(ctx, owner, repo, user)
	if err != nil {
		return err
	}
	if prs > 0 {
		return nil
	}
	issues, err := api.CountUserIssues(ctx, owner, repo, user)
	if err != nil {
		return err
	}
	if issues > 1 {
		return nil
	}
	return api.CreateComment(ctx, owner, repo, number, message)
}
