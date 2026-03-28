package serverconfig

import (
	"fmt"
	"os"

	"github.com/BurntSushi/toml"
)

type Config struct {
	Server      Server      `toml:"server"`
	GitHub      GitHub      `toml:"github"`
	Credentials Credentials `toml:"credentials"`
	LLM         LLM         `toml:"llm"`
}

type Server struct {
	Listen        string `toml:"listen"`
	WebhookSecret string `toml:"webhook_secret"`
}

type GitHub struct {
	AppID          int64  `toml:"app_id"`
	PrivateKeyPath string `toml:"private_key_path"`
}

type Credentials struct {
	CargoRegistryToken string `toml:"cargo_registry_token"`
}

type LLM struct {
	AnthropicAPIKey string `toml:"anthropic_api_key"`
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	var cfg Config
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	return &cfg, nil
}

func (c *Config) validate() error {
	if c.Server.WebhookSecret == "" {
		return fmt.Errorf("server.webhook_secret is required")
	}
	if c.GitHub.AppID == 0 {
		return fmt.Errorf("github.app_id is required")
	}
	if c.GitHub.PrivateKeyPath == "" {
		return fmt.Errorf("github.private_key_path is required")
	}
	return nil
}
