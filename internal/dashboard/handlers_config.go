// internal/dashboard/handlers_config.go
package dashboard

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/vexil-lang/vexilbot/internal/serverconfig"
)

type configPageData struct {
	basePage
	ServerTOML string
}

func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	s.render(w, "config", configPageData{
		basePage:   s.base(r, "server-config"),
		ServerTOML: redactedServerTOML(s.deps.ServerConfig),
	})
}

// redactedServerTOML renders the server config as TOML-like text with secrets replaced by [redacted].
func redactedServerTOML(cfg *serverconfig.Config) string {
	if cfg == nil {
		return ""
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "[server]\n")
	fmt.Fprintf(&sb, "listen = %q\n", cfg.Server.Listen)
	fmt.Fprintf(&sb, "webhook_secret = \"[redacted]\"\n")
	fmt.Fprintf(&sb, "bot_name = %q\n", cfg.Server.BotName)
	fmt.Fprintf(&sb, "data_dir = %q\n", cfg.Server.DataDir)
	fmt.Fprintf(&sb, "dashboard_port = %d\n", cfg.Server.DashboardPort)
	fmt.Fprintf(&sb, "\n[github]\n")
	fmt.Fprintf(&sb, "app_id = %d\n", cfg.GitHub.AppID)
	fmt.Fprintf(&sb, "private_key_path = \"[redacted]\"\n")
	fmt.Fprintf(&sb, "\n[credentials]\n")
	if cfg.Credentials.CargoRegistryToken != "" {
		fmt.Fprintf(&sb, "cargo_registry_token = \"[redacted]\"\n")
	}
	fmt.Fprintf(&sb, "\n[llm]\n")
	if cfg.LLM.AnthropicAPIKey != "" {
		fmt.Fprintf(&sb, "anthropic_api_key = \"[redacted]\"\n")
	}
	return sb.String()
}
