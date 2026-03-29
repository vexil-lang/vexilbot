// internal/dashboard/server.go
package dashboard

import (
	"context"
	"embed"
	"html/template"
	"net/http"
	"strings"

	"github.com/google/go-github/v68/github"
	"github.com/vexil-lang/vexilbot/internal/serverconfig"
	"github.com/vexil-lang/vexilbot/internal/vexstore"
)

//go:embed templates/*
var templateFS embed.FS

// Deps holds all external dependencies the dashboard needs.
type Deps struct {
	LogStore     *vexstore.AppendStore
	EventStore   *vexstore.AppendStore
	ReleaseStore *vexstore.AppendStore
	DataDir      string
	ServerConfig *serverconfig.Config
	// KnownRepos returns all owner/repo strings seen so far (e.g. "owner/repo").
	KnownRepos func() []string
	// RunRelease creates a release PR for owner/repo/pkg. Returns the PR number.
	RunRelease func(ctx context.Context, owner, repo, pkg string) (int, error)
	// FetchRepoConfig fetches the raw .vexilbot.toml bytes for owner/repo from GitHub.
	FetchRepoConfig func(ctx context.Context, owner, repo string) ([]byte, error)
	// GetInstallationClient returns a GitHub client authenticated as the bot
	// for the given owner/repo.
	GetInstallationClient func(owner, repo string) (*github.Client, error)
	// RunWorkspaceRelease triggers a workspace release for owner/repo.
	// Returns the number of release PRs created.
	RunWorkspaceRelease func(ctx context.Context, owner, repo string) (int, error)
}

// basePage is embedded in every page data struct.
type basePage struct {
	Tab        string
	Repo       string
	KnownRepos []string
}

// Server is the dashboard HTTP server.
type Server struct {
	mux  *http.ServeMux
	tmpl *template.Template
	deps Deps
}

// New creates a dashboard Server with all routes registered.
func New(deps Deps) *Server {
	tmpl := template.Must(
		template.New("").Funcs(template.FuncMap{
			"lower": strings.ToLower,
		}).ParseFS(templateFS, "templates/*.html"),
	)
	s := &Server{mux: http.NewServeMux(), tmpl: tmpl, deps: deps}
	s.mux.HandleFunc("GET /", s.handleRoot)
	s.mux.HandleFunc("GET /logs", s.handleLogs)
	s.mux.HandleFunc("GET /logs-rows", s.handleLogsRows)
	s.mux.HandleFunc("GET /events", s.handleEvents)
	s.mux.HandleFunc("GET /releases", s.handleReleases)
	s.mux.HandleFunc("POST /releases", s.handleReleasesCreate)
	s.mux.HandleFunc("POST /releases/{id}/cancel", s.handleReleasesCancel)
	s.mux.HandleFunc("POST /releases/{id}/confirm", s.handleReleasesConfirm)
	s.mux.HandleFunc("POST /releases/{id}/run", s.handleReleasesRun)
	s.mux.HandleFunc("GET /config", s.handleConfig)
	s.mux.HandleFunc("GET /config/repo", s.handleConfigRepo)
	s.mux.HandleFunc("GET /storage", s.handleStorage)
	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

func (s *Server) handleRoot(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/logs", http.StatusFound)
}

// render executes the named page template.
func (s *Server) render(w http.ResponseWriter, name string, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.tmpl.ExecuteTemplate(w, name, data); err != nil {
		http.Error(w, "template error: "+err.Error(), http.StatusInternalServerError)
	}
}

// base constructs a basePage from the request and the active tab name.
func (s *Server) base(r *http.Request, tab string) basePage {
	return basePage{
		Tab:        tab,
		Repo:       r.URL.Query().Get("repo"),
		KnownRepos: s.deps.KnownRepos(),
	}
}
