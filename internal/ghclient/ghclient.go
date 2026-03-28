package ghclient

import (
	"context"
	"fmt"
	"net/http"
	"os"

	"github.com/bradleyfalzon/ghinstallation/v2"
	"github.com/google/go-github/v68/github"
)

type App struct {
	appID     int64
	transport *ghinstallation.AppsTransport
}

func NewApp(appID int64, privateKeyPath string) (*App, error) {
	keyData, err := os.ReadFile(privateKeyPath)
	if err != nil {
		return nil, fmt.Errorf("read private key: %w", err)
	}

	transport, err := ghinstallation.NewAppsTransport(http.DefaultTransport, appID, keyData)
	if err != nil {
		return nil, fmt.Errorf("create app transport: %w", err)
	}

	return &App{
		appID:     appID,
		transport: transport,
	}, nil
}

func (a *App) InstallationClient(installationID int64) *github.Client {
	transport := ghinstallation.NewFromAppsTransport(a.transport, installationID)
	return github.NewClient(&http.Client{Transport: transport})
}

func (a *App) FetchRepoConfig(ctx context.Context, client *github.Client, owner, repo string) ([]byte, error) {
	content, _, resp, err := client.Repositories.GetContents(
		ctx, owner, repo, ".vexilbot.toml",
		&github.RepositoryContentGetOptions{},
	)
	if err != nil {
		if resp != nil && resp.StatusCode == 404 {
			return nil, fmt.Errorf(".vexilbot.toml not found in %s/%s", owner, repo)
		}
		return nil, fmt.Errorf("fetch .vexilbot.toml: %w", err)
	}

	decoded, err := content.GetContent()
	if err != nil {
		return nil, fmt.Errorf("decode .vexilbot.toml content: %w", err)
	}

	return []byte(decoded), nil
}
