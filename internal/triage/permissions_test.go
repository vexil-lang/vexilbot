package triage_test

import (
	"context"
	"testing"

	"github.com/vexil-lang/vexilbot/internal/repoconfig"
	"github.com/vexil-lang/vexilbot/internal/triage"
)

type mockGitHub struct {
	teamMembers   map[string][]string
	collaborators map[string]string
}

func (m *mockGitHub) IsTeamMember(ctx context.Context, org, teamSlug, user string) (bool, error) {
	members := m.teamMembers[teamSlug]
	for _, member := range members {
		if member == user {
			return true, nil
		}
	}
	return false, nil
}

func (m *mockGitHub) GetCollaboratorPermission(ctx context.Context, owner, repo, user string) (string, error) {
	perm, ok := m.collaborators[user]
	if !ok {
		return "none", nil
	}
	return perm, nil
}

func TestCheckPermission_TeamMember(t *testing.T) {
	gh := &mockGitHub{
		teamMembers: map[string][]string{
			"maintainers": {"alice"},
		},
	}
	cfg := repoconfig.Triage{
		AllowedTeams:       []string{"maintainers"},
		AllowCollaborators: false,
	}

	allowed, err := triage.CheckPermission(context.Background(), gh, cfg, "org", "repo", "alice")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !allowed {
		t.Error("expected alice to be allowed as team member")
	}
}

func TestCheckPermission_Collaborator(t *testing.T) {
	gh := &mockGitHub{
		collaborators: map[string]string{
			"bob": "write",
		},
	}
	cfg := repoconfig.Triage{
		AllowedTeams:       []string{},
		AllowCollaborators: true,
	}

	allowed, err := triage.CheckPermission(context.Background(), gh, cfg, "org", "repo", "bob")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !allowed {
		t.Error("expected bob to be allowed as collaborator with write access")
	}
}

func TestCheckPermission_ReadOnlyCollaborator(t *testing.T) {
	gh := &mockGitHub{
		collaborators: map[string]string{
			"eve": "read",
		},
	}
	cfg := repoconfig.Triage{
		AllowCollaborators: true,
	}

	allowed, err := triage.CheckPermission(context.Background(), gh, cfg, "org", "repo", "eve")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if allowed {
		t.Error("read-only collaborator should not be allowed")
	}
}

func TestCheckPermission_NotAllowed(t *testing.T) {
	gh := &mockGitHub{
		teamMembers:   map[string][]string{},
		collaborators: map[string]string{},
	}
	cfg := repoconfig.Triage{
		AllowedTeams:       []string{"maintainers"},
		AllowCollaborators: true,
	}

	allowed, err := triage.CheckPermission(context.Background(), gh, cfg, "org", "repo", "stranger")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if allowed {
		t.Error("stranger should not be allowed")
	}
}
