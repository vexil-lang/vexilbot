// internal/dashboard/handlers_config_overrides.go
package dashboard

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/vexil-lang/vexilbot/internal/configoverride"
)

// ---- Repo config ----

type repoConfigPageData struct {
	basePage
	BaseToml     string
	OverrideToml string
	SavedMsg     string
	ErrorMsg     string
}

func (s *Server) handleConfigRepo(w http.ResponseWriter, r *http.Request) {
	bp := s.base(r, "repo-config")
	owner, repo, ok := splitRepo(bp.Repo)
	if !ok {
		s.render(w, "repo_config.html", repoConfigPageData{basePage: bp, ErrorMsg: "select a repo first"})
		return
	}
	rawBase, err := s.deps.FetchRepoConfig(r.Context(), owner, repo)
	if err != nil {
		s.render(w, "repo_config.html", repoConfigPageData{basePage: bp, ErrorMsg: "fetch config: " + err.Error()})
		return
	}
	ovPath := configoverride.Path(s.deps.DataDir, owner, repo)
	ovData, _ := configoverride.Load(ovPath)
	savedMsg := ""
	if r.URL.Query().Get("saved") == "1" {
		savedMsg = "Saved."
	}
	s.render(w, "repo_config.html", repoConfigPageData{
		basePage:     bp,
		BaseToml:     string(rawBase),
		OverrideToml: string(ovData),
		SavedMsg:     savedMsg,
	})
}

func (s *Server) handleConfigRepoOverridesSave(w http.ResponseWriter, r *http.Request) {
	bp := s.base(r, "repo-config")
	owner, repo, ok := splitRepo(bp.Repo)
	if !ok {
		http.Error(w, "repo required", http.StatusBadRequest)
		return
	}
	body := r.FormValue("override_toml")
	ovPath := configoverride.Path(s.deps.DataDir, owner, repo)
	if err := configoverride.Save(ovPath, []byte(body)); err != nil {
		s.render(w, "repo_config.html", repoConfigPageData{basePage: bp, ErrorMsg: "save: " + err.Error()})
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/config/repo?repo=%s&saved=1", bp.Repo), http.StatusSeeOther)
}

func (s *Server) handleConfigRepoOverridesDelete(w http.ResponseWriter, r *http.Request) {
	bp := s.base(r, "repo-config")
	owner, repo, ok := splitRepo(bp.Repo)
	if !ok {
		http.Error(w, "repo required", http.StatusBadRequest)
		return
	}
	ovPath := configoverride.Path(s.deps.DataDir, owner, repo)
	if err := configoverride.Delete(ovPath); err != nil {
		http.Error(w, "delete: "+err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/config/repo?repo=%s", bp.Repo), http.StatusSeeOther)
}

// splitRepo splits "owner/repo" into owner, repo. Returns ok=false if malformed.
func splitRepo(ownerRepo string) (owner, repo string, ok bool) {
	parts := strings.SplitN(ownerRepo, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", false
	}
	return parts[0], parts[1], true
}
