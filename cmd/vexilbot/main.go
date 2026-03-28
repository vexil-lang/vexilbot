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

	vexil "github.com/vexil-lang/vexil/packages/runtime-go"
	"github.com/vexil-lang/vexilbot/internal/dashboard"
	"github.com/vexil-lang/vexilbot/internal/ghclient"
	"github.com/vexil-lang/vexilbot/internal/labeler"
	"github.com/vexil-lang/vexilbot/internal/logstore"
	"github.com/vexil-lang/vexilbot/internal/policy"
	"github.com/vexil-lang/vexilbot/internal/release"
	"github.com/vexil-lang/vexilbot/internal/repoconfig"
	"github.com/vexil-lang/vexilbot/internal/serverconfig"
	"github.com/vexil-lang/vexilbot/internal/triage"
	"github.com/vexil-lang/vexilbot/internal/vexstore"
	"github.com/vexil-lang/vexilbot/internal/vexstore/gen/logentry"
	"github.com/vexil-lang/vexilbot/internal/vexstore/gen/scheduledrelease"
	"github.com/vexil-lang/vexilbot/internal/vexstore/gen/webhookevent"
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

func (s *installationStore) list() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]string, 0, len(s.entries))
	for k := range s.entries {
		out = append(out, k)
	}
	return out
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "usage: vexilbot <config-path>\n")
		os.Exit(1)
	}

	cfg, err := serverconfig.Load(os.Args[1])
	if err != nil {
		fmt.Fprintf(os.Stderr, "load config: %v\n", err)
		os.Exit(1)
	}

	if err := os.MkdirAll(cfg.Server.DataDir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "create data dir: %v\n", err)
		os.Exit(1)
	}
	logStore, err := vexstore.OpenAppendStore(cfg.Server.DataDir+"/logs.vxb", logentry.SchemaHash)
	if err != nil {
		fmt.Fprintf(os.Stderr, "open log store: %v\n", err)
		os.Exit(1)
	}
	defer logStore.Close()
	eventStore, err := vexstore.OpenAppendStore(cfg.Server.DataDir+"/events.vxb", webhookevent.SchemaHash)
	if err != nil {
		fmt.Fprintf(os.Stderr, "open event store: %v\n", err)
		os.Exit(1)
	}
	defer eventStore.Close()
	scheduledRelStore, err := vexstore.OpenAppendStore(cfg.Server.DataDir+"/scheduled_releases.vxb", scheduledrelease.SchemaHash)
	if err != nil {
		fmt.Fprintf(os.Stderr, "open scheduled release store: %v\n", err)
		os.Exit(1)
	}
	defer scheduledRelStore.Close()
	logger := slog.New(logstore.NewHandler(logStore, slog.NewJSONHandler(os.Stdout, nil)))
	slog.SetDefault(logger)

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

	runRelease := func(ctx context.Context, owner, repo, pkg string) (int, error) {
		id, ok := store.get(owner, repo)
		if !ok {
			return 0, fmt.Errorf("no installation ID known for %s/%s", owner, repo)
		}
		adapter := &ghAdapter{client: app.InstallationClient(id)}
		repoCfg, err := configCache.Get(ctx, owner, repo)
		if err != nil {
			return 0, fmt.Errorf("get repo config: %w", err)
		}
		return release.RunReleaseNow(ctx, adapter, owner, repo, pkg, repoCfg.Release)
	}

	fetchRepoConfig := func(ctx context.Context, owner, repo string) ([]byte, error) {
		id, ok := store.get(owner, repo)
		if !ok {
			return nil, fmt.Errorf("no installation ID known for %s/%s", owner, repo)
		}
		client := app.InstallationClient(id)
		return app.FetchRepoConfig(ctx, client, owner, repo)
	}

	dispatcher := webhook.NewDispatcher()

	dispatcher.OnPullRequest(func(ev webhook.PullRequestEvent) {
		if ev.Action != "opened" && ev.Action != "synchronize" {
			return
		}
		go func() {
			ctx := context.Background()
			store.set(ev.Owner, ev.Repo, ev.InstallationID)
			wev := &webhookevent.WebhookEvent{
				Ts:     uint64(time.Now().UnixNano()),
				Kind:   webhookevent.EventKindPullRequest,
				Owner:  ev.Owner,
				Repo:   ev.Repo,
				Action: ev.Action,
			}
			bw := vexil.NewBitWriter()
			if wev.Pack(bw) == nil {
				_ = eventStore.Append(bw.Finish())
			}

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
			wev := &webhookevent.WebhookEvent{
				Ts:     uint64(time.Now().UnixNano()),
				Kind:   webhookevent.EventKindIssueComment,
				Owner:  ev.Owner,
				Repo:   ev.Repo,
				Action: ev.Action,
			}
			bw := vexil.NewBitWriter()
			if wev.Pack(bw) == nil {
				_ = eventStore.Append(bw.Finish())
			}

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
			wev := &webhookevent.WebhookEvent{
				Ts:     uint64(time.Now().UnixNano()),
				Kind:   webhookevent.EventKindIssues,
				Owner:  ev.Owner,
				Repo:   ev.Repo,
				Action: ev.Action,
			}
			bw := vexil.NewBitWriter()
			if wev.Pack(bw) == nil {
				_ = eventStore.Append(bw.Finish())
			}

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

	dispatcher.OnPush(func(ev webhook.PushEvent) {
		go func() {
			ctx := context.Background()
			store.set(ev.Owner, ev.Repo, ev.InstallationID)
			wev := &webhookevent.WebhookEvent{
				Ts:     uint64(time.Now().UnixNano()),
				Kind:   webhookevent.EventKindPush,
				Owner:  ev.Owner,
				Repo:   ev.Repo,
				Action: "",
			}
			bw := vexil.NewBitWriter()
			if wev.Pack(bw) == nil {
				_ = eventStore.Append(bw.Finish())
			}
			slog.InfoContext(ctx, "push event recorded", "owner", ev.Owner, "repo", ev.Repo)
		}()
	})

	handler := webhook.NewHandler(cfg.Server.WebhookSecret, dispatcher)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", healthHandler)
	mux.Handle("POST /webhook", handler)

	if cfg.Server.DashboardPort != 0 {
		dashSrv := dashboard.New(dashboard.Deps{
			LogStore:        logStore,
			EventStore:      eventStore,
			ReleaseStore:    scheduledRelStore,
			DataDir:         cfg.Server.DataDir,
			ServerConfig:    cfg,
			KnownRepos:      store.list,
			RunRelease:      runRelease,
			FetchRepoConfig: fetchRepoConfig,
		})
		dashAddr := fmt.Sprintf(":%d", cfg.Server.DashboardPort)
		go func() {
			slog.Info("dashboard starting", "listen", dashAddr)
			if err := http.ListenAndServe(dashAddr, dashSrv); err != nil {
				slog.Error("dashboard error", "error", err)
			}
		}()
	}

	slog.Info("vexilbot starting", "listen", cfg.Server.Listen)
	if err := http.ListenAndServe(cfg.Server.Listen, mux); err != nil {
		slog.Error("server error", "error", err)
		os.Exit(1)
	}
}

func handleRelease(ctx context.Context, adapter *ghAdapter, repoCfg *repoconfig.Config, owner, repo string, issueNumber int, args []string) {
	var err error
	switch {
	case len(args) == 0:
		err = release.RunWorkspaceRelease(ctx, adapter, owner, repo, issueNumber, repoCfg.Release)
	case args[0] == "status":
		err = release.RunStatus(ctx, adapter, owner, repo, issueNumber, repoCfg.Release)
	default:
		err = release.RunRelease(ctx, adapter, owner, repo, args[0], issueNumber, repoCfg.Release)
	}
	if err != nil {
		slog.Error("release command", "args", args, "error", err)
	}
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}
