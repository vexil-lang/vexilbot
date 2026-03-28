package serverconfig_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/vexil-lang/vexilbot/internal/serverconfig"
)

func TestLoad_ValidConfig(t *testing.T) {
	content := `
[server]
listen = "127.0.0.1:8080"
webhook_secret = "whsec_test123"
bot_name = "vexil-bot"

[github]
app_id = 12345
private_key_path = "/etc/vexilbot/app.pem"

[credentials]
cargo_registry_token = "crt_abc123"

[llm]
anthropic_api_key = "sk-ant-test"
`
	path := writeTempFile(t, content)
	cfg, err := serverconfig.Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Server.Listen != "127.0.0.1:8080" {
		t.Errorf("listen = %q, want %q", cfg.Server.Listen, "127.0.0.1:8080")
	}
	if cfg.Server.WebhookSecret != "whsec_test123" {
		t.Errorf("webhook_secret = %q, want %q", cfg.Server.WebhookSecret, "whsec_test123")
	}
	if cfg.GitHub.AppID != 12345 {
		t.Errorf("app_id = %d, want %d", cfg.GitHub.AppID, 12345)
	}
	if cfg.GitHub.PrivateKeyPath != "/etc/vexilbot/app.pem" {
		t.Errorf("private_key_path = %q, want %q", cfg.GitHub.PrivateKeyPath, "/etc/vexilbot/app.pem")
	}
	if cfg.Credentials.CargoRegistryToken != "crt_abc123" {
		t.Errorf("cargo_registry_token = %q, want %q", cfg.Credentials.CargoRegistryToken, "crt_abc123")
	}
	if cfg.LLM.AnthropicAPIKey != "sk-ant-test" {
		t.Errorf("anthropic_api_key = %q, want %q", cfg.LLM.AnthropicAPIKey, "sk-ant-test")
	}
	if cfg.Server.BotName != "vexil-bot" {
		t.Errorf("bot_name = %q, want %q", cfg.Server.BotName, "vexil-bot")
	}
}

func TestLoad_MissingRequiredFields(t *testing.T) {
	content := `
[server]
listen = "127.0.0.1:8080"
`
	path := writeTempFile(t, content)
	_, err := serverconfig.Load(path)
	if err == nil {
		t.Fatal("expected error for missing required fields, got nil")
	}
}

func TestLoad_FileNotFound(t *testing.T) {
	_, err := serverconfig.Load("/nonexistent/path.toml")
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}

func TestDashboardPortDefault(t *testing.T) {
	content := `
[server]
listen = "0.0.0.0:8080"
webhook_secret = "s"
bot_name = "vexilbot"
[github]
app_id = 1
private_key_path = "/tmp/key"
`
	path := writeTempFile(t, content)
	cfg, err := serverconfig.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Server.DashboardPort != 8081 {
		t.Errorf("want DashboardPort=8081, got %d", cfg.Server.DashboardPort)
	}
}

func TestDashboardPortConfigured(t *testing.T) {
	content := `
[server]
listen = "0.0.0.0:8080"
webhook_secret = "s"
bot_name = "vexilbot"
dashboard_port = 9090
[github]
app_id = 1
private_key_path = "/tmp/key"
`
	path := writeTempFile(t, content)
	cfg, err := serverconfig.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Server.DashboardPort != 9090 {
		t.Errorf("want DashboardPort=9090, got %d", cfg.Server.DashboardPort)
	}
}

func writeTempFile(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	return path
}
