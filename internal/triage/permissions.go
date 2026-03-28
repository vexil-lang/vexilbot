package triage

import (
	"context"

	"github.com/vexil-lang/vexilbot/internal/repoconfig"
)

type GitHubPermissions interface {
	IsTeamMember(ctx context.Context, org, teamSlug, user string) (bool, error)
	GetCollaboratorPermission(ctx context.Context, owner, repo, user string) (string, error)
}

// CheckPermission returns true if the user is authorized to run bot commands.
func CheckPermission(
	ctx context.Context,
	gh GitHubPermissions,
	cfg repoconfig.Triage,
	owner, repo, user string,
) (bool, error) {
	for _, team := range cfg.AllowedTeams {
		isMember, err := gh.IsTeamMember(ctx, owner, team, user)
		if err != nil {
			return false, err
		}
		if isMember {
			return true, nil
		}
	}

	if cfg.AllowCollaborators {
		perm, err := gh.GetCollaboratorPermission(ctx, owner, repo, user)
		if err != nil {
			return false, err
		}
		if perm == "admin" || perm == "write" {
			return true, nil
		}
	}

	return false, nil
}
