package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/vexil-lang/vexilbot/internal/ghclient"
	"github.com/vexil-lang/vexilbot/internal/labeler"
	"github.com/vexil-lang/vexilbot/internal/policy"
	"github.com/vexil-lang/vexilbot/internal/release"
	"github.com/vexil-lang/vexilbot/internal/repoconfig"
	"github.com/vexil-lang/vexilbot/internal/serverconfig"
	"github.com/vexil-lang/vexilbot/internal/triage"
	"github.com/vexil-lang/vexilbot/internal/webhook"
	"github.com/vexil-lang/vexilbot/internal/welcome"
)

// installationStore tracks the GitHub App installation ID for each owner/repo.
// It is populated on each incoming webhook so that the config fetcher can
// obtain an installation-scoped client without a separate API round-trip.
type installationStore struct {
	mu      sync.RWMutex
	entries map[string]int64
}

func (s *installationStore) set(owner, repo string, id int64) {
	s.mu.Lock()
	s.entries[owner+"/"+repo] = id
	s.mu.Unlock()
}

func (s *installationStore) get(owner, repo string) (int64, bool) {
	s.mu.RLock()
	id, ok := s.entries[owner+"/"+repo]
	s.mu.RUnlock()
	return id, ok
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "usage: vexilbot <config-path>\n")
		os.Exit(1)
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	cfg, err := serverconfig.Load(os.Args[1])
	if err != nil {
		slog.Error("load config", "error", err)
		os.Exit(1)
	}

	app, err := ghclient.NewApp(cfg.GitHub.AppID, cfg.GitHub.PrivateKeyPath)
	if err != nil {
		slog.Error("create github app", "error", err)
		os.Exit(1)
	}

	store := &installationStore{entries: make(map[string]int64)}

	configCache := repoconfig.NewCache(func(ctx context.Context, owner, repo string) (*repoconfig.Config, error) {
		id, ok := store.get(owner, repo)
		if !ok {
			return nil, fmt.Errorf("no installation ID known for %s/%s", owner, repo)
		}
		client := app.InstallationClient(id)
		data, err := app.FetchRepoConfig(ctx, client, owner, repo)
		if err != nil {
			return nil, err
		}
		return repoconfig.Parse(data)
	}, 5*time.Minute)

	dispatcher := webhook.NewDispatcher()

	dispatcher.OnPullRequest(func(ev webhook.PullRequestEvent) {
		if ev.Action != "opened" && ev.Action != "synchronize" {
			return
		}
		go func() {
			ctx := context.Background()
			store.set(ev.Owner, ev.Repo, ev.InstallationID)

			repoCfg, err := configCache.Get(ctx, ev.Owner, ev.Repo)
			if err != nil {
				slog.Error("get repo config", "owner", ev.Owner, "repo", ev.Repo, "error", err)
				return
			}

			adapter := &ghAdapter{client: app.InstallationClient(ev.InstallationID)}

			files, err := adapter.ListPRFiles(ctx, ev.Owner, ev.Repo, ev.Number)
			if err != nil {
				slog.Error("list PR files", "owner", ev.Owner, "repo", ev.Repo, "pr", ev.Number, "error", err)
				return
			}

			if labels := labeler.MatchPathLabels(repoCfg.Labels, files); len(labels) > 0 {
				if err := adapter.AddLabels(ctx, ev.Owner, ev.Repo, ev.Number, labels); err != nil {
					slog.Error("add path labels", "error", err)
				}
			}

			if _, err := policy.CheckRFCGate(ctx, adapter, ev.Owner, ev.Repo, ev.Number, ev.HeadSHA, repoCfg.Policy.RFCRequiredPaths, files); err != nil {
				slog.Error("check RFC gate", "error", err)
			}

			if _, err := policy.CheckWireFormatWarning(ctx, adapter, ev.Owner, ev.Repo, ev.Number, repoCfg.Policy.WireFormatWarningPaths, files); err != nil {
				slog.Error("check wire format warning", "error", err)
			}

			if ev.Action == "opened" {
				if err := welcome.MaybeWelcomePR(ctx, adapter, ev.Owner, ev.Repo, ev.UserLogin, ev.Number, repoCfg.Welcome.PRMessage); err != nil {
					slog.Error("welcome PR", "error", err)
				}
			}
		}()
	})

	dispatcher.OnIssueComment(func(ev webhook.IssueCommentEvent) {
		if ev.Action != "created" {
			return
		}
		go func() {
			ctx := context.Background()
			store.set(ev.Owner, ev.Repo, ev.InstallationID)

			cmd, ok := triage.ParseCommand(ev.CommentBody, cfg.Server.BotName)
			if !ok {
				return
			}

			repoCfg, err := configCache.Get(ctx, ev.Owner, ev.Repo)
			if err != nil {
				slog.Error("get repo config", "owner", ev.Owner, "repo", ev.Repo, "error", err)
				return
			}

			adapter := &ghAdapter{client: app.InstallationClient(ev.InstallationID)}

			allowed, err := triage.CheckPermission(ctx, adapter, repoCfg.Triage, ev.Owner, ev.Repo, ev.CommentUser)
			if err != nil {
				slog.Error("check permission", "user", ev.CommentUser, "error", err)
				return
			}
			if !allowed {
				slog.Info("command denied: insufficient permission", "user", ev.CommentUser, "cmd", cmd.Name)
				return
			}

			if cmd.Name == "release" {
				handleRelease(ctx, adapter, repoCfg, ev.Owner, ev.Repo, ev.IssueNumber, cmd.Args)
				return
			}

			if err := triage.Execute(ctx, adapter, cmd, ev.Owner, ev.Repo, ev.IssueNumber, ev.CommentID); err != nil {
				slog.Error("execute command", "cmd", cmd.Name, "user", ev.CommentUser, "error", err)
			}
		}()
	})

	dispatcher.OnIssues(func(ev webhook.IssuesEvent) {
		if ev.Action != "opened" {
			return
		}
		go func() {
			ctx := context.Background()
			store.set(ev.Owner, ev.Repo, ev.InstallationID)

			repoCfg, err := configCache.Get(ctx, ev.Owner, ev.Repo)
			if err != nil {
				slog.Error("get repo config", "owner", ev.Owner, "repo", ev.Repo, "error", err)
				return
			}

			adapter := &ghAdapter{client: app.InstallationClient(ev.InstallationID)}

			if labels := labeler.MatchKeywordLabels(repoCfg.Labels, ev.Title, ev.Body); len(labels) > 0 {
				if err := adapter.AddLabels(ctx, ev.Owner, ev.Repo, ev.Number, labels); err != nil {
					slog.Error("add keyword labels", "error", err)
				}
			}

			if err := welcome.MaybeWelcomeIssue(ctx, adapter, ev.Owner, ev.Repo, ev.UserLogin, ev.Number, repoCfg.Welcome.IssueMessage); err != nil {
				slog.Error("welcome issue", "error", err)
			}
		}()
	})

	handler := webhook.NewHandler(cfg.Server.WebhookSecret, dispatcher)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", healthHandler)
	mux.Handle("POST /webhook", handler)

	slog.Info("vexilbot starting", "listen", cfg.Server.Listen)
	if err := http.ListenAndServe(cfg.Server.Listen, mux); err != nil {
		slog.Error("server error", "error", err)
		os.Exit(1)
	}
}

func handleRelease(ctx context.Context, adapter *ghAdapter, repoCfg *repoconfig.Config, owner, repo string, issueNumber int, args []string) {
	subCmd := "status"
	if len(args) > 0 {
		subCmd = args[0]
	}

	var err error
	switch subCmd {
	case "status":
		err = release.RunStatus(ctx, adapter, owner, repo, issueNumber, repoCfg.Release)
	default:
		err = release.RunRelease(ctx, adapter, owner, repo, subCmd, issueNumber, repoCfg.Release)
	}
	if err != nil {
		slog.Error("release command", "sub", subCmd, "error", err)
	}
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}
