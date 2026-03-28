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
func MaybeWelcomePR(ctx context.Context, api ContribAPI, owner, repo, user string, number int, message string) error {
	if message == "" {
		return nil
	}
	isFirst, err := isFirstTimeContributor(ctx, api, owner, repo, user)
	if err != nil {
		return err
	}
	if !isFirst {
		return nil
	}
	return api.CreateComment(ctx, owner, repo, number, message)
}

// MaybeWelcomeIssue posts a welcome message on an issue if the author is a first-time contributor.
func MaybeWelcomeIssue(ctx context.Context, api ContribAPI, owner, repo, user string, number int, message string) error {
	if message == "" {
		return nil
	}
	isFirst, err := isFirstTimeContributor(ctx, api, owner, repo, user)
	if err != nil {
		return err
	}
	if !isFirst {
		return nil
	}
	return api.CreateComment(ctx, owner, repo, number, message)
}

func isFirstTimeContributor(ctx context.Context, api ContribAPI, owner, repo, user string) (bool, error) {
	prs, err := api.CountUserPRs(ctx, owner, repo, user)
	if err != nil {
		return false, err
	}
	if prs > 0 {
		return false, nil
	}

	issues, err := api.CountUserIssues(ctx, owner, repo, user)
	if err != nil {
		return false, err
	}
	return issues == 0, nil
}
