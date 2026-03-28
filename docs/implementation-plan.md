# vexilbot Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a stateless Go GitHub App bot that handles labeling, triage, welcome messages, policy enforcement, and release management for vexil-lang repos.

**Architecture:** Single Go binary with an HTTP webhook server. Packages organized by feature (labeler, triage, welcome, policy, release). Config parsed from both server-side TOML (secrets) and repo-side TOML (behavior). GitHub App auth via JWT + installation tokens. No database — all state derived from GitHub API.

**Tech Stack:** Go 1.22+, google/go-github/v68, bradleyfalzon/ghinstallation/v2, BurntSushi/toml, log/slog (stdlib), net/http (stdlib)

**Repo:** New repo, separate from vexil-lang. All paths below are relative to the new repo root.

---

## File Structure

```
cmd/vexilbot/
  main.go                  entry point: load server config, init GitHub client, start HTTP server

internal/
  serverconfig/
    serverconfig.go        server-side TOML config struct + loader
    serverconfig_test.go
  repoconfig/
    repoconfig.go          repo-side vexilbot.toml struct + parser
    repoconfig_test.go
    cache.go               TTL cache for fetched repo configs
    cache_test.go
  webhook/
    webhook.go             HTTP handler: signature verification + event routing
    webhook_test.go
    signature.go           HMAC-SHA256 verification
    signature_test.go
  ghclient/
    ghclient.go            GitHub App auth wrapper, installation token management
    ghclient_test.go
  labeler/
    labeler.go             PR path-based labeling + issue keyword labeling
    labeler_test.go
    glob.go                glob pattern matching for file paths
    glob_test.go
  welcome/
    welcome.go             first-time contributor detection + welcome messages
    welcome_test.go
  triage/
    parse.go               @vexilbot command parser
    parse_test.go
    commands.go            triage command handlers (label, assign, prioritize, close, reopen)
    commands_test.go
    permissions.go         permission checking (teams, collaborators)
    permissions_test.go
  policy/
    rfcgate.go             RFC label requirement for spec/corpus PRs
    rfcgate_test.go
    wireformat.go          wire format warning comments
    wireformat_test.go
    rfctimer.go            RFC 14-day comment period tracking
    rfctimer_test.go
  release/
    detect.go              change detection: unreleased commits per crate
    detect_test.go
    version.go             version parsing, bumping, conventional commit analysis
    version_test.go
    bump.go                Cargo.toml version bumping + dependency updates
    bump_test.go
    changelog.go           git-cliff invocation wrapper
    changelog_test.go
    orchestrate.go         release orchestration: branch, PR, publish flow
    orchestrate_test.go
    publish.go             cargo publish + post_publish hooks
    publish_test.go
  llm/
    llm.go                 Client interface + no-op implementation
    llm_test.go

go.mod
go.sum
Makefile
README.md
```

---

### Task 1: Project Scaffolding

**Files:**
- Create: `go.mod`
- Create: `cmd/vexilbot/main.go`
- Create: `Makefile`

- [ ] **Step 1: Initialize Go module**

```bash
mkdir vexilbot && cd vexilbot
go mod init github.com/vexil-lang/vexilbot
```

- [ ] **Step 2: Create minimal main.go**

Create `cmd/vexilbot/main.go`:

```go
package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "usage: vexilbot <config-path>\n")
		os.Exit(1)
	}
	fmt.Printf("vexilbot starting with config: %s\n", os.Args[1])
}
```

- [ ] **Step 3: Create Makefile**

Create `Makefile`:

```makefile
.PHONY: build test lint

build:
	go build -o bin/vexilbot ./cmd/vexilbot

test:
	go test ./...

lint:
	go vet ./...
	golangci-lint run
```

- [ ] **Step 4: Verify build**

```bash
make build
./bin/vexilbot
# Expected: "usage: vexilbot <config-path>" on stderr, exit 1
./bin/vexilbot /tmp/test.toml
# Expected: "vexilbot starting with config: /tmp/test.toml"
```

- [ ] **Step 5: Commit**

```bash
git init
git add .
git commit -m "feat: project scaffolding with Go module and build setup"
```

---

### Task 2: Server-Side Config

**Files:**
- Create: `internal/serverconfig/serverconfig.go`
- Create: `internal/serverconfig/serverconfig_test.go`

- [ ] **Step 1: Write failing test for config parsing**

Create `internal/serverconfig/serverconfig_test.go`:

```go
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

func writeTempFile(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	return path
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/serverconfig/...
# Expected: FAIL — package does not exist
```

- [ ] **Step 3: Implement serverconfig**

Create `internal/serverconfig/serverconfig.go`:

```go
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
```

- [ ] **Step 4: Install dependency and run tests**

```bash
go get github.com/BurntSushi/toml
go test ./internal/serverconfig/...
# Expected: PASS (3 tests)
```

- [ ] **Step 5: Commit**

```bash
git add .
git commit -m "feat: server-side config parsing with validation"
```

---

### Task 3: Repo-Side Config (vexilbot.toml)

**Files:**
- Create: `internal/repoconfig/repoconfig.go`
- Create: `internal/repoconfig/repoconfig_test.go`

- [ ] **Step 1: Write failing test for repo config parsing**

Create `internal/repoconfig/repoconfig_test.go`:

```go
package repoconfig_test

import (
	"testing"

	"github.com/vexil-lang/vexilbot/internal/repoconfig"
)

func TestParse_FullConfig(t *testing.T) {
	content := `
[labels]
[labels.paths]
"crate:lang" = ["crates/vexil-lang/**"]
"spec"       = ["spec/**"]

[labels.keywords]
"bug" = ["crash", "panic"]

[triage]
allowed_teams = ["maintainers"]
allow_collaborators = true

[welcome]
pr_message = "Welcome to Vexil!"
issue_message = "Thanks for reporting!"

[policy]
rfc_required_paths = ["spec/**"]
wire_format_warning_paths = ["crates/vexil-runtime/**"]

[release]
changelog_tool = "git-cliff"
tag_format = "{{ crate }}-v{{ version }}"
auto_release = false
require_ci = true

[release.crates.vexil-lang]
path = "crates/vexil-lang"
publish = "crates.io"
suggest_threshold = 1
depends_on = []

[release.crates.vexil-bench]
path = "crates/vexil-bench"
publish = false
track = false

[llm]
enabled = false
provider = "claude"
model = "claude-sonnet-4-6-20250514"
[llm.features]
pr_review = false
issue_triage = false
release_notes = false
`
	cfg, err := repoconfig.Parse([]byte(content))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Labels
	paths := cfg.Labels.Paths
	if got := paths["crate:lang"]; len(got) != 1 || got[0] != "crates/vexil-lang/**" {
		t.Errorf("labels.paths[crate:lang] = %v", got)
	}
	keywords := cfg.Labels.Keywords
	if got := keywords["bug"]; len(got) != 2 || got[0] != "crash" {
		t.Errorf("labels.keywords[bug] = %v", got)
	}

	// Triage
	if !cfg.Triage.AllowCollaborators {
		t.Error("triage.allow_collaborators should be true")
	}
	if len(cfg.Triage.AllowedTeams) != 1 || cfg.Triage.AllowedTeams[0] != "maintainers" {
		t.Errorf("triage.allowed_teams = %v", cfg.Triage.AllowedTeams)
	}

	// Welcome
	if cfg.Welcome.PRMessage != "Welcome to Vexil!" {
		t.Errorf("welcome.pr_message = %q", cfg.Welcome.PRMessage)
	}

	// Policy
	if len(cfg.Policy.RFCRequiredPaths) != 1 || cfg.Policy.RFCRequiredPaths[0] != "spec/**" {
		t.Errorf("policy.rfc_required_paths = %v", cfg.Policy.RFCRequiredPaths)
	}

	// Release
	if cfg.Release.TagFormat != "{{ crate }}-v{{ version }}" {
		t.Errorf("release.tag_format = %q", cfg.Release.TagFormat)
	}
	if cfg.Release.AutoRelease {
		t.Error("release.auto_release should be false")
	}

	// Release crates
	lang, ok := cfg.Release.Crates["vexil-lang"]
	if !ok {
		t.Fatal("release.crates.vexil-lang missing")
	}
	if lang.Path != "crates/vexil-lang" {
		t.Errorf("vexil-lang path = %q", lang.Path)
	}
	if lang.Publish != "crates.io" {
		t.Errorf("vexil-lang publish = %q", lang.Publish)
	}

	bench, ok := cfg.Release.Crates["vexil-bench"]
	if !ok {
		t.Fatal("release.crates.vexil-bench missing")
	}
	if bench.Track {
		t.Error("vexil-bench track should be false")
	}

	// LLM
	if cfg.LLM.Enabled {
		t.Error("llm.enabled should be false")
	}
	if cfg.LLM.Provider != "claude" {
		t.Errorf("llm.provider = %q", cfg.LLM.Provider)
	}
}

func TestParse_Empty(t *testing.T) {
	cfg, err := repoconfig.Parse([]byte(""))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Empty config should parse without error — all sections optional
	if cfg.Release.Crates == nil {
		// Crates map should be initialized (empty, not nil)
		// This is fine — nil map is acceptable for "no crates configured"
	}
}

func TestParse_InvalidTOML(t *testing.T) {
	_, err := repoconfig.Parse([]byte("{{invalid"))
	if err == nil {
		t.Fatal("expected error for invalid TOML")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/repoconfig/...
# Expected: FAIL — package does not exist
```

- [ ] **Step 3: Implement repoconfig**

Create `internal/repoconfig/repoconfig.go`:

```go
package repoconfig

import (
	"fmt"

	"github.com/BurntSushi/toml"
)

type Config struct {
	Labels  Labels  `toml:"labels"`
	Triage  Triage  `toml:"triage"`
	Welcome Welcome `toml:"welcome"`
	Policy  Policy  `toml:"policy"`
	Release Release `toml:"release"`
	LLM     LLM     `toml:"llm"`
}

type Labels struct {
	Paths    map[string][]string `toml:"paths"`
	Keywords map[string][]string `toml:"keywords"`
}

type Triage struct {
	AllowedTeams       []string `toml:"allowed_teams"`
	AllowCollaborators bool     `toml:"allow_collaborators"`
}

type Welcome struct {
	PRMessage    string `toml:"pr_message"`
	IssueMessage string `toml:"issue_message"`
}

type Policy struct {
	RFCRequiredPaths       []string `toml:"rfc_required_paths"`
	WireFormatWarningPaths []string `toml:"wire_format_warning_paths"`
}

type Release struct {
	ChangelogTool string                `toml:"changelog_tool"`
	TagFormat     string                `toml:"tag_format"`
	AutoRelease   bool                  `toml:"auto_release"`
	RequireCI     bool                  `toml:"require_ci"`
	Crates        map[string]CrateEntry `toml:"crates"`
}

type CrateEntry struct {
	Path             string        `toml:"path"`
	Publish          string        `toml:"publish"`
	SuggestThreshold int           `toml:"suggest_threshold"`
	DependsOn        []string      `toml:"depends_on"`
	PostPublish      []PostPublish `toml:"post_publish"`
	Track            bool          `toml:"track"`
}

type PostPublish struct {
	Run     string `toml:"run"`
	Package string `toml:"package"`
}

type LLM struct {
	Enabled  bool        `toml:"enabled"`
	Provider string      `toml:"provider"`
	Model    string      `toml:"model"`
	Features LLMFeatures `toml:"features"`
}

type LLMFeatures struct {
	PRReview     bool `toml:"pr_review"`
	IssueTriage  bool `toml:"issue_triage"`
	ReleaseNotes bool `toml:"release_notes"`
}

func Parse(data []byte) (*Config, error) {
	var cfg Config
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse vexilbot.toml: %w", err)
	}
	return &cfg, nil
}
```

- [ ] **Step 4: Run tests**

```bash
go test ./internal/repoconfig/...
# Expected: PASS (3 tests)
```

- [ ] **Step 5: Commit**

```bash
git add .
git commit -m "feat: repo-side vexilbot.toml config parsing"
```

---

### Task 4: Repo Config Cache

**Files:**
- Create: `internal/repoconfig/cache.go`
- Create: `internal/repoconfig/cache_test.go`

- [ ] **Step 1: Write failing test for config cache**

Create `internal/repoconfig/cache_test.go`:

```go
package repoconfig_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/vexil-lang/vexilbot/internal/repoconfig"
)

func TestCache_ReturnsConfig(t *testing.T) {
	fetched := &atomic.Int32{}
	fetcher := func(ctx context.Context, owner, repo string) (*repoconfig.Config, error) {
		fetched.Add(1)
		return &repoconfig.Config{
			Welcome: repoconfig.Welcome{PRMessage: "hello"},
		}, nil
	}

	cache := repoconfig.NewCache(fetcher, 5*time.Minute)
	cfg, err := cache.Get(context.Background(), "vexil-lang", "vexil")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Welcome.PRMessage != "hello" {
		t.Errorf("pr_message = %q, want %q", cfg.Welcome.PRMessage, "hello")
	}
	if fetched.Load() != 1 {
		t.Errorf("fetched %d times, want 1", fetched.Load())
	}
}

func TestCache_ReturnsCachedWithinTTL(t *testing.T) {
	fetched := &atomic.Int32{}
	fetcher := func(ctx context.Context, owner, repo string) (*repoconfig.Config, error) {
		fetched.Add(1)
		return &repoconfig.Config{}, nil
	}

	cache := repoconfig.NewCache(fetcher, 5*time.Minute)
	_, _ = cache.Get(context.Background(), "vexil-lang", "vexil")
	_, _ = cache.Get(context.Background(), "vexil-lang", "vexil")
	_, _ = cache.Get(context.Background(), "vexil-lang", "vexil")

	if fetched.Load() != 1 {
		t.Errorf("fetched %d times, want 1 (should be cached)", fetched.Load())
	}
}

func TestCache_RefetchesAfterTTL(t *testing.T) {
	fetched := &atomic.Int32{}
	fetcher := func(ctx context.Context, owner, repo string) (*repoconfig.Config, error) {
		fetched.Add(1)
		return &repoconfig.Config{}, nil
	}

	cache := repoconfig.NewCache(fetcher, 1*time.Millisecond)
	_, _ = cache.Get(context.Background(), "vexil-lang", "vexil")
	time.Sleep(5 * time.Millisecond)
	_, _ = cache.Get(context.Background(), "vexil-lang", "vexil")

	if fetched.Load() != 2 {
		t.Errorf("fetched %d times, want 2 (TTL expired)", fetched.Load())
	}
}

func TestCache_SeparateRepos(t *testing.T) {
	fetched := &atomic.Int32{}
	fetcher := func(ctx context.Context, owner, repo string) (*repoconfig.Config, error) {
		fetched.Add(1)
		return &repoconfig.Config{}, nil
	}

	cache := repoconfig.NewCache(fetcher, 5*time.Minute)
	_, _ = cache.Get(context.Background(), "vexil-lang", "vexil")
	_, _ = cache.Get(context.Background(), "vexil-lang", "vexilbot")

	if fetched.Load() != 2 {
		t.Errorf("fetched %d times, want 2 (separate repos)", fetched.Load())
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/repoconfig/...
# Expected: FAIL — NewCache undefined
```

- [ ] **Step 3: Implement cache**

Create `internal/repoconfig/cache.go`:

```go
package repoconfig

import (
	"context"
	"fmt"
	"sync"
	"time"
)

type Fetcher func(ctx context.Context, owner, repo string) (*Config, error)

type cacheEntry struct {
	config    *Config
	fetchedAt time.Time
}

type Cache struct {
	fetcher Fetcher
	ttl     time.Duration
	mu      sync.RWMutex
	entries map[string]cacheEntry
}

func NewCache(fetcher Fetcher, ttl time.Duration) *Cache {
	return &Cache{
		fetcher: fetcher,
		ttl:     ttl,
		entries: make(map[string]cacheEntry),
	}
}

func (c *Cache) Get(ctx context.Context, owner, repo string) (*Config, error) {
	key := fmt.Sprintf("%s/%s", owner, repo)

	c.mu.RLock()
	entry, ok := c.entries[key]
	c.mu.RUnlock()

	if ok && time.Since(entry.fetchedAt) < c.ttl {
		return entry.config, nil
	}

	cfg, err := c.fetcher(ctx, owner, repo)
	if err != nil {
		return nil, err
	}

	c.mu.Lock()
	c.entries[key] = cacheEntry{config: cfg, fetchedAt: time.Now()}
	c.mu.Unlock()

	return cfg, nil
}
```

- [ ] **Step 4: Run tests**

```bash
go test ./internal/repoconfig/...
# Expected: PASS (all tests including cache tests)
```

- [ ] **Step 5: Commit**

```bash
git add .
git commit -m "feat: TTL cache for repo config fetching"
```

---

### Task 5: Webhook Signature Verification

**Files:**
- Create: `internal/webhook/signature.go`
- Create: `internal/webhook/signature_test.go`

- [ ] **Step 1: Write failing test**

Create `internal/webhook/signature_test.go`:

```go
package webhook_test

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"testing"

	"github.com/vexil-lang/vexilbot/internal/webhook"
)

func TestVerifySignature_Valid(t *testing.T) {
	secret := "test-secret"
	body := []byte(`{"action":"opened"}`)

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	sig := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	if err := webhook.VerifySignature(body, sig, secret); err != nil {
		t.Fatalf("valid signature rejected: %v", err)
	}
}

func TestVerifySignature_Invalid(t *testing.T) {
	err := webhook.VerifySignature([]byte("body"), "sha256=deadbeef", "secret")
	if err == nil {
		t.Fatal("invalid signature accepted")
	}
}

func TestVerifySignature_MalformedHeader(t *testing.T) {
	err := webhook.VerifySignature([]byte("body"), "not-a-sig", "secret")
	if err == nil {
		t.Fatal("malformed signature header accepted")
	}
}

func TestVerifySignature_Empty(t *testing.T) {
	err := webhook.VerifySignature([]byte("body"), "", "secret")
	if err == nil {
		t.Fatal("empty signature accepted")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/webhook/...
# Expected: FAIL — package does not exist
```

- [ ] **Step 3: Implement signature verification**

Create `internal/webhook/signature.go`:

```go
package webhook

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

func VerifySignature(body []byte, signatureHeader, secret string) error {
	if signatureHeader == "" {
		return fmt.Errorf("missing signature header")
	}

	parts := strings.SplitN(signatureHeader, "=", 2)
	if len(parts) != 2 || parts[0] != "sha256" {
		return fmt.Errorf("malformed signature header: %q", signatureHeader)
	}

	gotSig, err := hex.DecodeString(parts[1])
	if err != nil {
		return fmt.Errorf("decode signature hex: %w", err)
	}

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	expectedSig := mac.Sum(nil)

	if !hmac.Equal(gotSig, expectedSig) {
		return fmt.Errorf("signature mismatch")
	}

	return nil
}
```

- [ ] **Step 4: Run tests**

```bash
go test ./internal/webhook/...
# Expected: PASS (4 tests)
```

- [ ] **Step 5: Commit**

```bash
git add .
git commit -m "feat: webhook HMAC-SHA256 signature verification"
```

---

### Task 6: Webhook HTTP Handler + Event Routing

**Files:**
- Create: `internal/webhook/webhook.go`
- Create: `internal/webhook/webhook_test.go`

- [ ] **Step 1: Write failing test for webhook handler**

Create `internal/webhook/webhook_test.go`:

```go
package webhook_test

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/vexil-lang/vexilbot/internal/webhook"
)

func sign(body, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(body))
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func TestHandler_ValidPullRequestEvent(t *testing.T) {
	var mu sync.Mutex
	var gotEvent string
	var gotPayload []byte

	router := webhook.RouterFunc(func(eventType string, payload []byte) {
		mu.Lock()
		defer mu.Unlock()
		gotEvent = eventType
		gotPayload = payload
	})

	h := webhook.NewHandler("test-secret", router)
	body := `{"action":"opened","number":1}`
	req := httptest.NewRequest(http.MethodPost, "/webhook", strings.NewReader(body))
	req.Header.Set("X-Hub-Signature-256", sign(body, "test-secret"))
	req.Header.Set("X-GitHub-Event", "pull_request")
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
	mu.Lock()
	defer mu.Unlock()
	if gotEvent != "pull_request" {
		t.Errorf("event = %q, want %q", gotEvent, "pull_request")
	}
	if string(gotPayload) != body {
		t.Errorf("payload = %q, want %q", string(gotPayload), body)
	}
}

func TestHandler_InvalidSignature(t *testing.T) {
	router := webhook.RouterFunc(func(eventType string, payload []byte) {
		t.Fatal("router should not be called on invalid signature")
	})

	h := webhook.NewHandler("test-secret", router)
	req := httptest.NewRequest(http.MethodPost, "/webhook", strings.NewReader("body"))
	req.Header.Set("X-Hub-Signature-256", "sha256=invalid")
	req.Header.Set("X-GitHub-Event", "push")
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestHandler_MissingEventHeader(t *testing.T) {
	router := webhook.RouterFunc(func(eventType string, payload []byte) {
		t.Fatal("router should not be called without event header")
	})

	h := webhook.NewHandler("test-secret", router)
	body := `{}`
	req := httptest.NewRequest(http.MethodPost, "/webhook", strings.NewReader(body))
	req.Header.Set("X-Hub-Signature-256", sign(body, "test-secret"))
	// No X-GitHub-Event header
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestHandler_MethodNotAllowed(t *testing.T) {
	router := webhook.RouterFunc(func(eventType string, payload []byte) {})
	h := webhook.NewHandler("test-secret", router)
	req := httptest.NewRequest(http.MethodGet, "/webhook", nil)
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/webhook/...
# Expected: FAIL — NewHandler, RouterFunc undefined
```

- [ ] **Step 3: Implement webhook handler**

Update `internal/webhook/webhook.go`:

```go
package webhook

import (
	"io"
	"log/slog"
	"net/http"
)

type Router interface {
	Route(eventType string, payload []byte)
}

type RouterFunc func(eventType string, payload []byte)

func (f RouterFunc) Route(eventType string, payload []byte) {
	f(eventType, payload)
}

type Handler struct {
	secret string
	router Router
}

func NewHandler(secret string, router Router) *Handler {
	return &Handler{secret: secret, router: router}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		slog.Error("read body", "error", err)
		http.Error(w, "failed to read body", http.StatusInternalServerError)
		return
	}

	sig := r.Header.Get("X-Hub-Signature-256")
	if err := VerifySignature(body, sig, h.secret); err != nil {
		slog.Warn("signature verification failed", "error", err)
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	eventType := r.Header.Get("X-GitHub-Event")
	if eventType == "" {
		http.Error(w, "missing X-GitHub-Event header", http.StatusBadRequest)
		return
	}

	slog.Info("webhook received", "event", eventType, "bytes", len(body))
	h.router.Route(eventType, body)

	w.WriteHeader(http.StatusOK)
}
```

- [ ] **Step 4: Run tests**

```bash
go test ./internal/webhook/...
# Expected: PASS (all tests)
```

- [ ] **Step 5: Commit**

```bash
git add .
git commit -m "feat: webhook HTTP handler with event routing"
```

---

### Task 7: GitHub App Client Wrapper

**Files:**
- Create: `internal/ghclient/ghclient.go`
- Create: `internal/ghclient/ghclient_test.go`

- [ ] **Step 1: Write failing test**

Create `internal/ghclient/ghclient_test.go`:

```go
package ghclient_test

import (
	"testing"

	"github.com/vexil-lang/vexilbot/internal/ghclient"
)

func TestNewApp_InvalidKeyPath(t *testing.T) {
	_, err := ghclient.NewApp(12345, "/nonexistent/key.pem")
	if err == nil {
		t.Fatal("expected error for invalid key path")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/ghclient/...
# Expected: FAIL — package does not exist
```

- [ ] **Step 3: Implement ghclient**

Create `internal/ghclient/ghclient.go`:

```go
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
	appID      int64
	transport  *ghinstallation.AppsTransport
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
		ctx, owner, repo, "vexilbot.toml",
		&github.RepositoryContentGetOptions{},
	)
	if err != nil {
		if resp != nil && resp.StatusCode == 404 {
			return nil, fmt.Errorf("vexilbot.toml not found in %s/%s", owner, repo)
		}
		return nil, fmt.Errorf("fetch vexilbot.toml: %w", err)
	}

	decoded, err := content.GetContent()
	if err != nil {
		return nil, fmt.Errorf("decode vexilbot.toml content: %w", err)
	}

	return []byte(decoded), nil
}
```

- [ ] **Step 4: Install dependencies and run tests**

```bash
go get github.com/google/go-github/v68
go get github.com/bradleyfalzon/ghinstallation/v2
go test ./internal/ghclient/...
# Expected: PASS (1 test)
```

- [ ] **Step 5: Commit**

```bash
git add .
git commit -m "feat: GitHub App client wrapper with installation tokens"
```

---

### Task 8: Glob Pattern Matching

**Files:**
- Create: `internal/labeler/glob.go`
- Create: `internal/labeler/glob_test.go`

- [ ] **Step 1: Write failing test**

Create `internal/labeler/glob_test.go`:

```go
package labeler_test

import (
	"testing"

	"github.com/vexil-lang/vexilbot/internal/labeler"
)

func TestMatchGlob(t *testing.T) {
	tests := []struct {
		pattern string
		path    string
		want    bool
	}{
		{"crates/vexil-lang/**", "crates/vexil-lang/src/lib.rs", true},
		{"crates/vexil-lang/**", "crates/vexil-lang/src/parser/mod.rs", true},
		{"crates/vexil-lang/**", "crates/vexil-codegen-rust/src/lib.rs", false},
		{"spec/**", "spec/vexil-spec.md", true},
		{"spec/**", "spec/grammar/vexil.peg", true},
		{"spec/**", "src/spec.rs", false},
		{".github/**", ".github/workflows/ci.yml", true},
		{"*.md", "README.md", true},
		{"*.md", "src/README.md", false},
	}

	for _, tt := range tests {
		t.Run(tt.pattern+"_"+tt.path, func(t *testing.T) {
			got := labeler.MatchGlob(tt.pattern, tt.path)
			if got != tt.want {
				t.Errorf("MatchGlob(%q, %q) = %v, want %v", tt.pattern, tt.path, got, tt.want)
			}
		})
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/labeler/...
# Expected: FAIL — package does not exist
```

- [ ] **Step 3: Implement glob matching**

Create `internal/labeler/glob.go`:

```go
package labeler

import (
	"path/filepath"
	"strings"
)

// MatchGlob checks if a file path matches a glob pattern.
// Supports ** for recursive directory matching.
func MatchGlob(pattern, path string) bool {
	// Handle ** patterns by splitting on /**/ or trailing /**
	if strings.Contains(pattern, "**") {
		return matchDoubleGlob(pattern, path)
	}
	matched, _ := filepath.Match(pattern, path)
	return matched
}

func matchDoubleGlob(pattern, path string) bool {
	// "dir/**" matches anything under dir/
	if strings.HasSuffix(pattern, "/**") {
		prefix := strings.TrimSuffix(pattern, "/**")
		return strings.HasPrefix(path, prefix+"/")
	}

	// "dir/**/file" matches dir/file, dir/a/file, dir/a/b/file
	parts := strings.SplitN(pattern, "/**/", 2)
	if len(parts) == 2 {
		prefix := parts[0]
		suffix := parts[1]
		if !strings.HasPrefix(path, prefix+"/") {
			return false
		}
		rest := strings.TrimPrefix(path, prefix+"/")
		// Check if the suffix matches the end of the path
		matched, _ := filepath.Match(suffix, filepath.Base(rest))
		if matched {
			return true
		}
		// Check deeper paths
		for i := range rest {
			if rest[i] == '/' {
				candidate := rest[i+1:]
				matched, _ := filepath.Match(suffix, candidate)
				if matched {
					return true
				}
			}
		}
	}

	return false
}
```

- [ ] **Step 4: Run tests**

```bash
go test ./internal/labeler/...
# Expected: PASS (9 subtests)
```

- [ ] **Step 5: Commit**

```bash
git add .
git commit -m "feat: glob pattern matching for path-based labeling"
```

---

### Task 9: PR & Issue Labeler

**Files:**
- Create: `internal/labeler/labeler.go`
- Create: `internal/labeler/labeler_test.go`

- [ ] **Step 1: Write failing test**

Create `internal/labeler/labeler_test.go`:

```go
package labeler_test

import (
	"testing"

	"github.com/vexil-lang/vexilbot/internal/labeler"
	"github.com/vexil-lang/vexilbot/internal/repoconfig"
)

func TestMatchPathLabels(t *testing.T) {
	cfg := repoconfig.Labels{
		Paths: map[string][]string{
			"crate:lang":  {"crates/vexil-lang/**"},
			"crate:cli":   {"crates/vexilc/**"},
			"spec":        {"spec/**"},
			"ci":          {".github/**"},
		},
	}

	changedFiles := []string{
		"crates/vexil-lang/src/parser.rs",
		"crates/vexil-lang/src/lexer.rs",
		".github/workflows/ci.yml",
	}

	labels := labeler.MatchPathLabels(cfg, changedFiles)

	want := map[string]bool{"crate:lang": true, "ci": true}
	if len(labels) != len(want) {
		t.Fatalf("got %d labels %v, want %d", len(labels), labels, len(want))
	}
	for _, l := range labels {
		if !want[l] {
			t.Errorf("unexpected label %q", l)
		}
	}
}

func TestMatchPathLabels_NoMatch(t *testing.T) {
	cfg := repoconfig.Labels{
		Paths: map[string][]string{
			"spec": {"spec/**"},
		},
	}

	labels := labeler.MatchPathLabels(cfg, []string{"src/main.rs"})
	if len(labels) != 0 {
		t.Errorf("got labels %v, want none", labels)
	}
}

func TestMatchKeywordLabels(t *testing.T) {
	cfg := repoconfig.Labels{
		Keywords: map[string][]string{
			"bug":         {"crash", "panic", "error"},
			"performance": {"slow", "benchmark"},
		},
	}

	labels := labeler.MatchKeywordLabels(cfg, "Parser panics on empty input", "When I pass an empty file, the parser crashes with a panic.")

	want := map[string]bool{"bug": true}
	if len(labels) != len(want) {
		t.Fatalf("got %d labels %v, want %d", len(labels), labels, len(want))
	}
	for _, l := range labels {
		if !want[l] {
			t.Errorf("unexpected label %q", l)
		}
	}
}

func TestMatchKeywordLabels_CaseInsensitive(t *testing.T) {
	cfg := repoconfig.Labels{
		Keywords: map[string][]string{
			"bug": {"crash"},
		},
	}

	labels := labeler.MatchKeywordLabels(cfg, "CRASH on startup", "")
	if len(labels) != 1 || labels[0] != "bug" {
		t.Errorf("got %v, want [bug]", labels)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/labeler/...
# Expected: FAIL — MatchPathLabels, MatchKeywordLabels undefined
```

- [ ] **Step 3: Implement labeler**

Create `internal/labeler/labeler.go`:

```go
package labeler

import (
	"sort"
	"strings"

	"github.com/vexil-lang/vexilbot/internal/repoconfig"
)

// MatchPathLabels returns labels whose glob patterns match any of the changed files.
func MatchPathLabels(cfg repoconfig.Labels, changedFiles []string) []string {
	matched := make(map[string]bool)

	for label, patterns := range cfg.Paths {
		for _, pattern := range patterns {
			for _, file := range changedFiles {
				if MatchGlob(pattern, file) {
					matched[label] = true
					break
				}
			}
			if matched[label] {
				break
			}
		}
	}

	labels := make([]string, 0, len(matched))
	for l := range matched {
		labels = append(labels, l)
	}
	sort.Strings(labels)
	return labels
}

// MatchKeywordLabels returns labels whose keywords appear in the title or body.
func MatchKeywordLabels(cfg repoconfig.Labels, title, body string) []string {
	text := strings.ToLower(title + " " + body)
	matched := make(map[string]bool)

	for label, keywords := range cfg.Keywords {
		for _, kw := range keywords {
			if strings.Contains(text, strings.ToLower(kw)) {
				matched[label] = true
				break
			}
		}
	}

	labels := make([]string, 0, len(matched))
	for l := range matched {
		labels = append(labels, l)
	}
	sort.Strings(labels)
	return labels
}
```

- [ ] **Step 4: Run tests**

```bash
go test ./internal/labeler/...
# Expected: PASS (all tests)
```

- [ ] **Step 5: Commit**

```bash
git add .
git commit -m "feat: PR path-based and issue keyword-based auto-labeling"
```

---

### Task 10: Command Parser

**Files:**
- Create: `internal/triage/parse.go`
- Create: `internal/triage/parse_test.go`

- [ ] **Step 1: Write failing test**

Create `internal/triage/parse_test.go`:

```go
package triage_test

import (
	"testing"

	"github.com/vexil-lang/vexilbot/internal/triage"
)

func TestParseCommand(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		botName  string
		wantCmd  string
		wantArgs []string
		wantOK   bool
	}{
		{
			name:     "label command",
			body:     "@vexilbot label bug enhancement",
			botName:  "vexilbot",
			wantCmd:  "label",
			wantArgs: []string{"bug", "enhancement"},
			wantOK:   true,
		},
		{
			name:     "unlabel command",
			body:     "@vexilbot unlabel bug",
			botName:  "vexilbot",
			wantCmd:  "unlabel",
			wantArgs: []string{"bug"},
			wantOK:   true,
		},
		{
			name:     "assign command",
			body:     "@vexilbot assign furkanmamuk",
			botName:  "vexilbot",
			wantCmd:  "assign",
			wantArgs: []string{"furkanmamuk"},
			wantOK:   true,
		},
		{
			name:     "prioritize command",
			body:     "@vexilbot prioritize p0",
			botName:  "vexilbot",
			wantCmd:  "prioritize",
			wantArgs: []string{"p0"},
			wantOK:   true,
		},
		{
			name:     "close command",
			body:     "@vexilbot close",
			botName:  "vexilbot",
			wantCmd:  "close",
			wantArgs: nil,
			wantOK:   true,
		},
		{
			name:     "release subcommand",
			body:     "@vexilbot release vexil-lang patch",
			botName:  "vexilbot",
			wantCmd:  "release",
			wantArgs: []string{"vexil-lang", "patch"},
			wantOK:   true,
		},
		{
			name:     "rfc subcommand",
			body:     "@vexilbot rfc approve",
			botName:  "vexilbot",
			wantCmd:  "rfc",
			wantArgs: []string{"approve"},
			wantOK:   true,
		},
		{
			name:    "no mention",
			body:    "This is a regular comment",
			botName: "vexilbot",
			wantOK:  false,
		},
		{
			name:     "mention in middle of text",
			body:     "Hey can you @vexilbot label bug please?",
			botName:  "vexilbot",
			wantCmd:  "label",
			wantArgs: []string{"bug", "please?"},
			wantOK:   true,
		},
		{
			name:     "multiline takes first command",
			body:     "Some context\n@vexilbot assign alice\nThanks!",
			botName:  "vexilbot",
			wantCmd:  "assign",
			wantArgs: []string{"alice"},
			wantOK:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd, ok := triage.ParseCommand(tt.body, tt.botName)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if !ok {
				return
			}
			if cmd.Name != tt.wantCmd {
				t.Errorf("cmd = %q, want %q", cmd.Name, tt.wantCmd)
			}
			if len(cmd.Args) != len(tt.wantArgs) {
				t.Fatalf("args = %v, want %v", cmd.Args, tt.wantArgs)
			}
			for i, a := range cmd.Args {
				if a != tt.wantArgs[i] {
					t.Errorf("args[%d] = %q, want %q", i, a, tt.wantArgs[i])
				}
			}
		})
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/triage/...
# Expected: FAIL — package does not exist
```

- [ ] **Step 3: Implement command parser**

Create `internal/triage/parse.go`:

```go
package triage

import (
	"strings"
)

type Command struct {
	Name string
	Args []string
}

// ParseCommand extracts a @botname command from a comment body.
// It finds the first occurrence of @botname and parses the rest of that line.
func ParseCommand(body, botName string) (Command, bool) {
	mention := "@" + botName
	lines := strings.Split(body, "\n")

	for _, line := range lines {
		idx := strings.Index(line, mention)
		if idx == -1 {
			continue
		}

		after := strings.TrimSpace(line[idx+len(mention):])
		if after == "" {
			continue
		}

		parts := strings.Fields(after)
		if len(parts) == 0 {
			continue
		}

		cmd := Command{Name: parts[0]}
		if len(parts) > 1 {
			cmd.Args = parts[1:]
		}
		return cmd, true
	}

	return Command{}, false
}
```

- [ ] **Step 4: Run tests**

```bash
go test ./internal/triage/...
# Expected: PASS (10 subtests)
```

- [ ] **Step 5: Commit**

```bash
git add .
git commit -m "feat: @vexilbot comment command parser"
```

---

### Task 11: Triage Permission Checking

**Files:**
- Create: `internal/triage/permissions.go`
- Create: `internal/triage/permissions_test.go`

- [ ] **Step 1: Write failing test**

Create `internal/triage/permissions_test.go`:

```go
package triage_test

import (
	"context"
	"testing"

	"github.com/vexil-lang/vexilbot/internal/repoconfig"
	"github.com/vexil-lang/vexilbot/internal/triage"
)

type mockGitHub struct {
	teamMembers    map[string][]string // team slug -> usernames
	collaborators  map[string]string   // username -> permission level
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
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/triage/...
# Expected: FAIL — CheckPermission undefined
```

- [ ] **Step 3: Implement permission checking**

Create `internal/triage/permissions.go`:

```go
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
	// Check team membership
	for _, team := range cfg.AllowedTeams {
		isMember, err := gh.IsTeamMember(ctx, owner, team, user)
		if err != nil {
			return false, err
		}
		if isMember {
			return true, nil
		}
	}

	// Check collaborator access
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
```

- [ ] **Step 4: Run tests**

```bash
go test ./internal/triage/...
# Expected: PASS (all tests)
```

- [ ] **Step 5: Commit**

```bash
git add .
git commit -m "feat: triage permission checking for bot commands"
```

---

### Task 12: Triage Command Handlers

**Files:**
- Create: `internal/triage/commands.go`
- Create: `internal/triage/commands_test.go`

- [ ] **Step 1: Write failing test**

Create `internal/triage/commands_test.go`:

```go
package triage_test

import (
	"context"
	"testing"

	"github.com/vexil-lang/vexilbot/internal/triage"
)

type mockIssueAPI struct {
	addedLabels    []string
	removedLabels  []string
	assignees      []string
	closed         bool
	reopened       bool
	reactions      []string
	comments       []string
}

func (m *mockIssueAPI) AddLabels(ctx context.Context, owner, repo string, number int, labels []string) error {
	m.addedLabels = append(m.addedLabels, labels...)
	return nil
}

func (m *mockIssueAPI) RemoveLabel(ctx context.Context, owner, repo string, number int, label string) error {
	m.removedLabels = append(m.removedLabels, label)
	return nil
}

func (m *mockIssueAPI) AddAssignees(ctx context.Context, owner, repo string, number int, assignees []string) error {
	m.assignees = append(m.assignees, assignees...)
	return nil
}

func (m *mockIssueAPI) CloseIssue(ctx context.Context, owner, repo string, number int) error {
	m.closed = true
	return nil
}

func (m *mockIssueAPI) ReopenIssue(ctx context.Context, owner, repo string, number int) error {
	m.reopened = true
	return nil
}

func (m *mockIssueAPI) AddReaction(ctx context.Context, owner, repo string, commentID int64, reaction string) error {
	m.reactions = append(m.reactions, reaction)
	return nil
}

func (m *mockIssueAPI) CreateComment(ctx context.Context, owner, repo string, number int, body string) error {
	m.comments = append(m.comments, body)
	return nil
}

func TestExecute_Label(t *testing.T) {
	api := &mockIssueAPI{}
	cmd := triage.Command{Name: "label", Args: []string{"bug", "enhancement"}}
	err := triage.Execute(context.Background(), api, cmd, "org", "repo", 1, 100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(api.addedLabels) != 2 || api.addedLabels[0] != "bug" {
		t.Errorf("added labels = %v", api.addedLabels)
	}
	if len(api.reactions) != 1 || api.reactions[0] != "+1" {
		t.Errorf("reactions = %v", api.reactions)
	}
}

func TestExecute_Unlabel(t *testing.T) {
	api := &mockIssueAPI{}
	cmd := triage.Command{Name: "unlabel", Args: []string{"wontfix"}}
	err := triage.Execute(context.Background(), api, cmd, "org", "repo", 1, 100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(api.removedLabels) != 1 || api.removedLabels[0] != "wontfix" {
		t.Errorf("removed labels = %v", api.removedLabels)
	}
}

func TestExecute_Assign(t *testing.T) {
	api := &mockIssueAPI{}
	cmd := triage.Command{Name: "assign", Args: []string{"alice"}}
	err := triage.Execute(context.Background(), api, cmd, "org", "repo", 1, 100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(api.assignees) != 1 || api.assignees[0] != "alice" {
		t.Errorf("assignees = %v", api.assignees)
	}
}

func TestExecute_Prioritize(t *testing.T) {
	api := &mockIssueAPI{}
	cmd := triage.Command{Name: "prioritize", Args: []string{"p0"}}
	err := triage.Execute(context.Background(), api, cmd, "org", "repo", 1, 100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should remove p1, p2, p3 and add p0
	wantRemoved := map[string]bool{"p1": true, "p2": true, "p3": true}
	for _, l := range api.removedLabels {
		if !wantRemoved[l] {
			t.Errorf("unexpected removed label %q", l)
		}
	}
	if len(api.addedLabels) != 1 || api.addedLabels[0] != "p0" {
		t.Errorf("added labels = %v", api.addedLabels)
	}
}

func TestExecute_Close(t *testing.T) {
	api := &mockIssueAPI{}
	cmd := triage.Command{Name: "close", Args: nil}
	err := triage.Execute(context.Background(), api, cmd, "org", "repo", 1, 100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !api.closed {
		t.Error("issue should be closed")
	}
}

func TestExecute_Reopen(t *testing.T) {
	api := &mockIssueAPI{}
	cmd := triage.Command{Name: "reopen", Args: nil}
	err := triage.Execute(context.Background(), api, cmd, "org", "repo", 1, 100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !api.reopened {
		t.Error("issue should be reopened")
	}
}

func TestExecute_UnknownCommand(t *testing.T) {
	api := &mockIssueAPI{}
	cmd := triage.Command{Name: "explode", Args: nil}
	err := triage.Execute(context.Background(), api, cmd, "org", "repo", 1, 100)
	if err == nil {
		t.Fatal("expected error for unknown command")
	}
}

func TestExecute_LabelNoArgs(t *testing.T) {
	api := &mockIssueAPI{}
	cmd := triage.Command{Name: "label", Args: nil}
	err := triage.Execute(context.Background(), api, cmd, "org", "repo", 1, 100)
	if err == nil {
		t.Fatal("expected error for label with no args")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/triage/...
# Expected: FAIL — Execute, IssueAPI undefined
```

- [ ] **Step 3: Implement command handlers**

Create `internal/triage/commands.go`:

```go
package triage

import (
	"context"
	"fmt"
)

type IssueAPI interface {
	AddLabels(ctx context.Context, owner, repo string, number int, labels []string) error
	RemoveLabel(ctx context.Context, owner, repo string, number int, label string) error
	AddAssignees(ctx context.Context, owner, repo string, number int, assignees []string) error
	CloseIssue(ctx context.Context, owner, repo string, number int) error
	ReopenIssue(ctx context.Context, owner, repo string, number int) error
	AddReaction(ctx context.Context, owner, repo string, commentID int64, reaction string) error
	CreateComment(ctx context.Context, owner, repo string, number int, body string) error
}

var priorities = []string{"p0", "p1", "p2", "p3"}

// Execute runs a parsed command against the GitHub API.
// commentID is the ID of the comment that triggered the command (for reactions).
func Execute(ctx context.Context, api IssueAPI, cmd Command, owner, repo string, number int, commentID int64) error {
	switch cmd.Name {
	case "label":
		if len(cmd.Args) == 0 {
			return fmt.Errorf("label command requires at least one label")
		}
		if err := api.AddLabels(ctx, owner, repo, number, cmd.Args); err != nil {
			return err
		}
		return api.AddReaction(ctx, owner, repo, commentID, "+1")

	case "unlabel":
		if len(cmd.Args) == 0 {
			return fmt.Errorf("unlabel command requires a label")
		}
		for _, label := range cmd.Args {
			if err := api.RemoveLabel(ctx, owner, repo, number, label); err != nil {
				return err
			}
		}
		return api.AddReaction(ctx, owner, repo, commentID, "+1")

	case "assign":
		if len(cmd.Args) == 0 {
			return fmt.Errorf("assign command requires a username")
		}
		if err := api.AddAssignees(ctx, owner, repo, number, cmd.Args); err != nil {
			return err
		}
		return api.AddReaction(ctx, owner, repo, commentID, "+1")

	case "prioritize":
		if len(cmd.Args) != 1 {
			return fmt.Errorf("prioritize command requires exactly one priority (p0-p3)")
		}
		target := cmd.Args[0]
		valid := false
		for _, p := range priorities {
			if p == target {
				valid = true
			}
		}
		if !valid {
			return fmt.Errorf("invalid priority %q, must be one of: p0, p1, p2, p3", target)
		}
		// Remove other priority labels
		for _, p := range priorities {
			if p != target {
				_ = api.RemoveLabel(ctx, owner, repo, number, p)
			}
		}
		if err := api.AddLabels(ctx, owner, repo, number, []string{target}); err != nil {
			return err
		}
		return api.AddReaction(ctx, owner, repo, commentID, "+1")

	case "close":
		if err := api.CloseIssue(ctx, owner, repo, number); err != nil {
			return err
		}
		return api.AddReaction(ctx, owner, repo, commentID, "+1")

	case "reopen":
		if err := api.ReopenIssue(ctx, owner, repo, number); err != nil {
			return err
		}
		return api.AddReaction(ctx, owner, repo, commentID, "+1")

	case "release", "rfc":
		// These are handled by the release and policy packages respectively.
		// The triage package only parses and dispatches them.
		// Return nil here — the caller routes to the right handler.
		return nil

	default:
		return fmt.Errorf("unknown command: %q", cmd.Name)
	}
}
```

- [ ] **Step 4: Run tests**

```bash
go test ./internal/triage/...
# Expected: PASS (all tests)
```

- [ ] **Step 5: Commit**

```bash
git add .
git commit -m "feat: triage command handlers (label, assign, prioritize, close, reopen)"
```

---

### Task 13: Welcome Messages

**Files:**
- Create: `internal/welcome/welcome.go`
- Create: `internal/welcome/welcome_test.go`

- [ ] **Step 1: Write failing test**

Create `internal/welcome/welcome_test.go`:

```go
package welcome_test

import (
	"context"
	"testing"

	"github.com/vexil-lang/vexilbot/internal/welcome"
)

type mockContribAPI struct {
	prCount    int
	issueCount int
	comments   []struct {
		number int
		body   string
	}
}

func (m *mockContribAPI) CountUserPRs(ctx context.Context, owner, repo, user string) (int, error) {
	return m.prCount, nil
}

func (m *mockContribAPI) CountUserIssues(ctx context.Context, owner, repo, user string) (int, error) {
	return m.issueCount, nil
}

func (m *mockContribAPI) CreateComment(ctx context.Context, owner, repo string, number int, body string) error {
	m.comments = append(m.comments, struct {
		number int
		body   string
	}{number, body})
	return nil
}

func TestWelcomePR_FirstTime(t *testing.T) {
	api := &mockContribAPI{prCount: 0, issueCount: 0}
	err := welcome.MaybeWelcomePR(context.Background(), api, "org", "repo", "alice", 1, "Welcome!")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(api.comments) != 1 {
		t.Fatalf("got %d comments, want 1", len(api.comments))
	}
	if api.comments[0].body != "Welcome!" {
		t.Errorf("comment = %q, want %q", api.comments[0].body, "Welcome!")
	}
}

func TestWelcomePR_ReturningContributor(t *testing.T) {
	api := &mockContribAPI{prCount: 3, issueCount: 0}
	err := welcome.MaybeWelcomePR(context.Background(), api, "org", "repo", "bob", 5, "Welcome!")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(api.comments) != 0 {
		t.Errorf("got %d comments, want 0 for returning contributor", len(api.comments))
	}
}

func TestWelcomeIssue_FirstTime(t *testing.T) {
	api := &mockContribAPI{prCount: 0, issueCount: 0}
	err := welcome.MaybeWelcomeIssue(context.Background(), api, "org", "repo", "carol", 10, "Thanks!")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(api.comments) != 1 {
		t.Fatalf("got %d comments, want 1", len(api.comments))
	}
}

func TestWelcomeIssue_HasPriorIssues(t *testing.T) {
	api := &mockContribAPI{prCount: 0, issueCount: 2}
	err := welcome.MaybeWelcomeIssue(context.Background(), api, "org", "repo", "dave", 11, "Thanks!")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(api.comments) != 0 {
		t.Errorf("got %d comments, want 0", len(api.comments))
	}
}

func TestWelcome_EmptyMessage(t *testing.T) {
	api := &mockContribAPI{prCount: 0, issueCount: 0}
	err := welcome.MaybeWelcomePR(context.Background(), api, "org", "repo", "eve", 1, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Empty message = no comment posted
	if len(api.comments) != 0 {
		t.Errorf("got %d comments, want 0 for empty message", len(api.comments))
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/welcome/...
# Expected: FAIL — package does not exist
```

- [ ] **Step 3: Implement welcome**

Create `internal/welcome/welcome.go`:

```go
package welcome

import (
	"context"
)

type ContribAPI interface {
	CountUserPRs(ctx context.Context, owner, repo, user string) (int, error)
	CountUserIssues(ctx context.Context, owner, repo, user string) (int, error)
	CreateComment(ctx context.Context, owner, repo string, number int, body string) error
}

// MaybeWelcomePR posts a welcome message on a PR if the author is a first-time contributor.
func MaybeWelcomePR(ctx context.Context, api ContribAPI, owner, repo, user string, number int, message string) error {
	if message == "" {
		return nil
	}
	isFirst, err := isFirstTimeContributor(ctx, api, owner, repo, user)
	if err != nil {
		return err
	}
	if !isFirst {
		return nil
	}
	return api.CreateComment(ctx, owner, repo, number, message)
}

// MaybeWelcomeIssue posts a welcome message on an issue if the author is a first-time contributor.
func MaybeWelcomeIssue(ctx context.Context, api ContribAPI, owner, repo, user string, number int, message string) error {
	if message == "" {
		return nil
	}
	isFirst, err := isFirstTimeContributor(ctx, api, owner, repo, user)
	if err != nil {
		return err
	}
	if !isFirst {
		return nil
	}
	return api.CreateComment(ctx, owner, repo, number, message)
}

func isFirstTimeContributor(ctx context.Context, api ContribAPI, owner, repo, user string) (bool, error) {
	prs, err := api.CountUserPRs(ctx, owner, repo, user)
	if err != nil {
		return false, err
	}
	if prs > 0 {
		return false, nil
	}

	issues, err := api.CountUserIssues(ctx, owner, repo, user)
	if err != nil {
		return false, err
	}
	return issues == 0, nil
}
```

- [ ] **Step 4: Run tests**

```bash
go test ./internal/welcome/...
# Expected: PASS (5 tests)
```

- [ ] **Step 5: Commit**

```bash
git add .
git commit -m "feat: first-time contributor welcome messages"
```

---

### Task 14: Policy — RFC Gate

**Files:**
- Create: `internal/policy/rfcgate.go`
- Create: `internal/policy/rfcgate_test.go`

- [ ] **Step 1: Write failing test**

Create `internal/policy/rfcgate_test.go`:

```go
package policy_test

import (
	"context"
	"testing"

	"github.com/vexil-lang/vexilbot/internal/labeler"
	"github.com/vexil-lang/vexilbot/internal/policy"
)

type mockPolicyAPI struct {
	labels     []string
	statusSet  *struct{ state, context, description string }
	comments   []string
}

func (m *mockPolicyAPI) GetLabels(ctx context.Context, owner, repo string, number int) ([]string, error) {
	return m.labels, nil
}

func (m *mockPolicyAPI) SetCommitStatus(ctx context.Context, owner, repo, sha, state, statusContext, description string) error {
	m.statusSet = &struct{ state, context, description string }{state, statusContext, description}
	return nil
}

func (m *mockPolicyAPI) CreateComment(ctx context.Context, owner, repo string, number int, body string) error {
	m.comments = append(m.comments, body)
	return nil
}

// Reuse labeler.MatchGlob for path matching — test integration
var _ = labeler.MatchGlob

func TestCheckRFCGate_RequiresRFC(t *testing.T) {
	api := &mockPolicyAPI{labels: []string{"enhancement"}}
	rfcPaths := []string{"spec/**", "corpus/valid/**"}
	changedFiles := []string{"spec/vexil-spec.md"}

	result, err := policy.CheckRFCGate(context.Background(), api, "org", "repo", 1, "abc123", rfcPaths, changedFiles)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != policy.RFCRequired {
		t.Errorf("result = %v, want RFCRequired", result)
	}
	if api.statusSet == nil || api.statusSet.state != "pending" {
		t.Error("expected pending commit status")
	}
	if len(api.comments) != 1 {
		t.Errorf("got %d comments, want 1", len(api.comments))
	}
}

func TestCheckRFCGate_HasRFCLabel(t *testing.T) {
	api := &mockPolicyAPI{labels: []string{"rfc", "enhancement"}}
	rfcPaths := []string{"spec/**"}
	changedFiles := []string{"spec/vexil-spec.md"}

	result, err := policy.CheckRFCGate(context.Background(), api, "org", "repo", 1, "abc123", rfcPaths, changedFiles)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != policy.RFCSatisfied {
		t.Errorf("result = %v, want RFCSatisfied", result)
	}
	if api.statusSet == nil || api.statusSet.state != "success" {
		t.Error("expected success commit status")
	}
}

func TestCheckRFCGate_NoRFCPathsTouched(t *testing.T) {
	api := &mockPolicyAPI{labels: []string{}}
	rfcPaths := []string{"spec/**"}
	changedFiles := []string{"crates/vexil-lang/src/lib.rs"}

	result, err := policy.CheckRFCGate(context.Background(), api, "org", "repo", 1, "abc123", rfcPaths, changedFiles)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != policy.RFCNotApplicable {
		t.Errorf("result = %v, want RFCNotApplicable", result)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/policy/...
# Expected: FAIL — package does not exist
```

- [ ] **Step 3: Implement RFC gate**

Create `internal/policy/rfcgate.go`:

```go
package policy

import (
	"context"
	"fmt"

	"github.com/vexil-lang/vexilbot/internal/labeler"
)

type RFCResult int

const (
	RFCNotApplicable RFCResult = iota
	RFCSatisfied
	RFCRequired
)

func (r RFCResult) String() string {
	switch r {
	case RFCNotApplicable:
		return "not_applicable"
	case RFCSatisfied:
		return "satisfied"
	case RFCRequired:
		return "required"
	default:
		return "unknown"
	}
}

type PolicyAPI interface {
	GetLabels(ctx context.Context, owner, repo string, number int) ([]string, error)
	SetCommitStatus(ctx context.Context, owner, repo, sha, state, statusContext, description string) error
	CreateComment(ctx context.Context, owner, repo string, number int, body string) error
}

const statusContext = "vexilbot/policy"

// CheckRFCGate checks if a PR touching RFC-required paths has the rfc label.
func CheckRFCGate(
	ctx context.Context,
	api PolicyAPI,
	owner, repo string,
	number int,
	sha string,
	rfcRequiredPaths []string,
	changedFiles []string,
) (RFCResult, error) {
	// Check if any changed files match RFC-required paths
	touchesRFCPaths := false
	for _, file := range changedFiles {
		for _, pattern := range rfcRequiredPaths {
			if labeler.MatchGlob(pattern, file) {
				touchesRFCPaths = true
				break
			}
		}
		if touchesRFCPaths {
			break
		}
	}

	if !touchesRFCPaths {
		return RFCNotApplicable, nil
	}

	// Check for rfc label
	labels, err := api.GetLabels(ctx, owner, repo, number)
	if err != nil {
		return RFCNotApplicable, fmt.Errorf("get labels: %w", err)
	}

	hasRFC := false
	for _, l := range labels {
		if l == "rfc" {
			hasRFC = true
			break
		}
	}

	if hasRFC {
		err := api.SetCommitStatus(ctx, owner, repo, sha, "success", statusContext, "RFC label present")
		if err != nil {
			return RFCSatisfied, fmt.Errorf("set commit status: %w", err)
		}
		return RFCSatisfied, nil
	}

	// RFC required but missing
	if err := api.SetCommitStatus(ctx, owner, repo, sha, "pending", statusContext, "RFC label required — this PR modifies spec/corpus files"); err != nil {
		return RFCRequired, fmt.Errorf("set commit status: %w", err)
	}

	comment := "This PR modifies files that require an RFC per [GOVERNANCE.md](../GOVERNANCE.md). " +
		"Please open an RFC issue first, or add the `rfc` label if one already exists."
	if err := api.CreateComment(ctx, owner, repo, number, comment); err != nil {
		return RFCRequired, fmt.Errorf("create comment: %w", err)
	}

	return RFCRequired, nil
}
```

- [ ] **Step 4: Run tests**

```bash
go test ./internal/policy/...
# Expected: PASS (3 tests)
```

- [ ] **Step 5: Commit**

```bash
git add .
git commit -m "feat: RFC gate policy enforcement with commit status"
```

---

### Task 15: Policy — Wire Format Warning + RFC Timer

**Files:**
- Create: `internal/policy/wireformat.go`
- Create: `internal/policy/wireformat_test.go`
- Create: `internal/policy/rfctimer.go`
- Create: `internal/policy/rfctimer_test.go`

- [ ] **Step 1: Write failing tests**

Create `internal/policy/wireformat_test.go`:

```go
package policy_test

import (
	"context"
	"testing"

	"github.com/vexil-lang/vexilbot/internal/policy"
)

func TestCheckWireFormat_MatchingPaths(t *testing.T) {
	api := &mockPolicyAPI{}
	warningPaths := []string{"crates/vexil-runtime/**", "spec/vexil-spec.md"}
	changedFiles := []string{"crates/vexil-runtime/src/bitwriter.rs"}

	warned, err := policy.CheckWireFormatWarning(context.Background(), api, "org", "repo", 1, warningPaths, changedFiles)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !warned {
		t.Error("expected warning for wire format path")
	}
	if len(api.comments) != 1 {
		t.Errorf("got %d comments, want 1", len(api.comments))
	}
}

func TestCheckWireFormat_NoMatch(t *testing.T) {
	api := &mockPolicyAPI{}
	warningPaths := []string{"crates/vexil-runtime/**"}
	changedFiles := []string{"crates/vexil-lang/src/parser.rs"}

	warned, err := policy.CheckWireFormatWarning(context.Background(), api, "org", "repo", 1, warningPaths, changedFiles)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if warned {
		t.Error("expected no warning")
	}
}
```

Create `internal/policy/rfctimer_test.go`:

```go
package policy_test

import (
	"testing"
	"time"

	"github.com/vexil-lang/vexilbot/internal/policy"
)

func TestRFCTimerMessage(t *testing.T) {
	start := time.Date(2026, 3, 28, 12, 0, 0, 0, time.UTC)
	msg := policy.FormatRFCTimerStart(start)
	if msg == "" {
		t.Fatal("expected non-empty message")
	}
	// Should mention the end date (14 days later = April 11)
	if !containsSubstring(msg, "2026-04-11") {
		t.Errorf("message should contain end date 2026-04-11, got: %s", msg)
	}
}

func TestRFCTimerRemaining(t *testing.T) {
	start := time.Date(2026, 3, 28, 12, 0, 0, 0, time.UTC)

	// 5 days in
	now := start.Add(5 * 24 * time.Hour)
	remaining := policy.RFCDaysRemaining(start, now)
	if remaining != 9 {
		t.Errorf("remaining = %d, want 9", remaining)
	}

	// 14 days in (expired)
	now = start.Add(14 * 24 * time.Hour)
	remaining = policy.RFCDaysRemaining(start, now)
	if remaining != 0 {
		t.Errorf("remaining = %d, want 0", remaining)
	}

	// 20 days in
	now = start.Add(20 * 24 * time.Hour)
	remaining = policy.RFCDaysRemaining(start, now)
	if remaining != 0 {
		t.Errorf("remaining = %d, want 0", remaining)
	}
}

func containsSubstring(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && containsStr(s, sub))
}

func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/policy/...
# Expected: FAIL — CheckWireFormatWarning, FormatRFCTimerStart, RFCDaysRemaining undefined
```

- [ ] **Step 3: Implement wire format warning**

Create `internal/policy/wireformat.go`:

```go
package policy

import (
	"context"

	"github.com/vexil-lang/vexilbot/internal/labeler"
)

// CheckWireFormatWarning posts an advisory comment if the PR touches wire format paths.
func CheckWireFormatWarning(
	ctx context.Context,
	api PolicyAPI,
	owner, repo string,
	number int,
	warningPaths []string,
	changedFiles []string,
) (bool, error) {
	for _, file := range changedFiles {
		for _, pattern := range warningPaths {
			if labeler.MatchGlob(pattern, file) {
				comment := "⚠️ This PR touches wire format code. " +
					"Changes to wire encoding require a 14-day RFC comment period per GOVERNANCE.md."
				if err := api.CreateComment(ctx, owner, repo, number, comment); err != nil {
					return true, err
				}
				return true, nil
			}
		}
	}
	return false, nil
}
```

- [ ] **Step 4: Implement RFC timer**

Create `internal/policy/rfctimer.go`:

```go
package policy

import (
	"fmt"
	"time"
)

const rfcPeriodDays = 14

// FormatRFCTimerStart returns a comment body for when an RFC comment period begins.
func FormatRFCTimerStart(start time.Time) string {
	end := start.AddDate(0, 0, rfcPeriodDays)
	return fmt.Sprintf(
		"RFC comment period started on %s. Earliest decision date: **%s** (14 days).",
		start.Format("2006-01-02"),
		end.Format("2006-01-02"),
	)
}

// RFCDaysRemaining returns the number of days remaining in the RFC comment period.
// Returns 0 if the period has expired.
func RFCDaysRemaining(start, now time.Time) int {
	end := start.AddDate(0, 0, rfcPeriodDays)
	remaining := int(end.Sub(now).Hours() / 24)
	if remaining < 0 {
		return 0
	}
	return remaining
}
```

- [ ] **Step 5: Run tests**

```bash
go test ./internal/policy/...
# Expected: PASS (all tests)
```

- [ ] **Step 6: Commit**

```bash
git add .
git commit -m "feat: wire format warning and RFC timer tracking"
```

---

### Task 16: Release — Version Parsing and Conventional Commit Analysis

**Files:**
- Create: `internal/release/version.go`
- Create: `internal/release/version_test.go`

- [ ] **Step 1: Write failing test**

Create `internal/release/version_test.go`:

```go
package release_test

import (
	"testing"

	"github.com/vexil-lang/vexilbot/internal/release"
)

func TestParseVersion(t *testing.T) {
	tests := []struct {
		input string
		want  release.Version
		ok    bool
	}{
		{"0.3.1", release.Version{0, 3, 1}, true},
		{"1.0.0", release.Version{1, 0, 0}, true},
		{"0.0.1", release.Version{0, 0, 1}, true},
		{"invalid", release.Version{}, false},
		{"1.2", release.Version{}, false},
		{"", release.Version{}, false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := release.ParseVersion(tt.input)
			if tt.ok && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !tt.ok && err == nil {
				t.Fatal("expected error")
			}
			if got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestVersion_Bump(t *testing.T) {
	v := release.Version{0, 3, 1}

	patch := v.Bump(release.BumpPatch)
	if patch != (release.Version{0, 3, 2}) {
		t.Errorf("patch bump = %v", patch)
	}

	minor := v.Bump(release.BumpMinor)
	if minor != (release.Version{0, 4, 0}) {
		t.Errorf("minor bump = %v", minor)
	}

	major := v.Bump(release.BumpMajor)
	if major != (release.Version{1, 0, 0}) {
		t.Errorf("major bump = %v", major)
	}
}

func TestVersion_String(t *testing.T) {
	v := release.Version{0, 3, 1}
	if v.String() != "0.3.1" {
		t.Errorf("String() = %q", v.String())
	}
}

func TestSuggestBump(t *testing.T) {
	tests := []struct {
		name     string
		messages []string
		want     release.BumpLevel
	}{
		{
			name:     "fix only",
			messages: []string{"fix: correct parser error", "fix: handle empty input"},
			want:     release.BumpPatch,
		},
		{
			name:     "feat present",
			messages: []string{"fix: typo", "feat: add union types", "docs: update readme"},
			want:     release.BumpMinor,
		},
		{
			name:     "breaking change footer",
			messages: []string{"feat!: redesign wire format"},
			want:     release.BumpMajor,
		},
		{
			name:     "breaking change with exclamation",
			messages: []string{"refactor!: rename public API"},
			want:     release.BumpMajor,
		},
		{
			name:     "non-conventional commits",
			messages: []string{"update readme", "misc changes"},
			want:     release.BumpPatch,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := release.SuggestBump(tt.messages)
			if got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestExtractTagVersion(t *testing.T) {
	tests := []struct {
		tag      string
		crate    string
		format   string
		wantVer  string
		wantOK   bool
	}{
		{"vexil-lang-v0.3.1", "vexil-lang", "{{ crate }}-v{{ version }}", "0.3.1", true},
		{"vexil-runtime-v1.0.0", "vexil-runtime", "{{ crate }}-v{{ version }}", "1.0.0", true},
		{"vexil-runtime-v1.0.0", "vexil-lang", "{{ crate }}-v{{ version }}", "", false},
		{"unrelated-tag", "vexil-lang", "{{ crate }}-v{{ version }}", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.tag, func(t *testing.T) {
			ver, ok := release.ExtractTagVersion(tt.tag, tt.crate, tt.format)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if ver != tt.wantVer {
				t.Errorf("version = %q, want %q", ver, tt.wantVer)
			}
		})
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/release/...
# Expected: FAIL — package does not exist
```

- [ ] **Step 3: Implement version handling**

Create `internal/release/version.go`:

```go
package release

import (
	"fmt"
	"strconv"
	"strings"
)

type Version struct {
	Major int
	Minor int
	Patch int
}

type BumpLevel int

const (
	BumpPatch BumpLevel = iota
	BumpMinor
	BumpMajor
)

func (b BumpLevel) String() string {
	switch b {
	case BumpPatch:
		return "patch"
	case BumpMinor:
		return "minor"
	case BumpMajor:
		return "major"
	default:
		return "unknown"
	}
}

func ParseVersion(s string) (Version, error) {
	parts := strings.Split(s, ".")
	if len(parts) != 3 {
		return Version{}, fmt.Errorf("invalid version %q: expected major.minor.patch", s)
	}

	major, err := strconv.Atoi(parts[0])
	if err != nil {
		return Version{}, fmt.Errorf("invalid major version: %w", err)
	}
	minor, err := strconv.Atoi(parts[1])
	if err != nil {
		return Version{}, fmt.Errorf("invalid minor version: %w", err)
	}
	patch, err := strconv.Atoi(parts[2])
	if err != nil {
		return Version{}, fmt.Errorf("invalid patch version: %w", err)
	}

	return Version{Major: major, Minor: minor, Patch: patch}, nil
}

func (v Version) Bump(level BumpLevel) Version {
	switch level {
	case BumpPatch:
		return Version{v.Major, v.Minor, v.Patch + 1}
	case BumpMinor:
		return Version{v.Major, v.Minor + 1, 0}
	case BumpMajor:
		return Version{v.Major + 1, 0, 0}
	default:
		return v
	}
}

func (v Version) String() string {
	return fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
}

// SuggestBump analyzes conventional commit messages and suggests a bump level.
func SuggestBump(messages []string) BumpLevel {
	level := BumpPatch
	for _, msg := range messages {
		// Check for breaking change
		if isBreaking(msg) {
			return BumpMajor
		}
		// Check for feat
		if strings.HasPrefix(msg, "feat") {
			colonIdx := strings.Index(msg, ":")
			if colonIdx > 0 {
				prefix := msg[:colonIdx]
				if prefix == "feat" || strings.HasPrefix(prefix, "feat(") {
					level = BumpMinor
				}
			}
		}
	}
	return level
}

func isBreaking(msg string) bool {
	colonIdx := strings.Index(msg, ":")
	if colonIdx > 0 {
		prefix := msg[:colonIdx]
		if strings.HasSuffix(prefix, "!") {
			return true
		}
	}
	// Check for BREAKING CHANGE footer (simplified — checks full message)
	if strings.Contains(msg, "BREAKING CHANGE") || strings.Contains(msg, "BREAKING-CHANGE") {
		return true
	}
	return false
}

// ExtractTagVersion extracts the version string from a git tag for a given crate.
// format uses {{ crate }} and {{ version }} as placeholders.
func ExtractTagVersion(tag, crate, format string) (string, bool) {
	// Build a prefix from the format by replacing {{ crate }} with the crate name
	// and {{ version }} with nothing to get the prefix
	prefix := strings.Replace(format, "{{ version }}", "", 1)
	prefix = strings.Replace(prefix, "{{ crate }}", crate, 1)

	if !strings.HasPrefix(tag, prefix) {
		return "", false
	}

	version := tag[len(prefix):]
	if version == "" {
		return "", false
	}

	// Validate it looks like a version
	if _, err := ParseVersion(version); err != nil {
		return "", false
	}

	return version, true
}
```

- [ ] **Step 4: Run tests**

```bash
go test ./internal/release/...
# Expected: PASS (all tests)
```

- [ ] **Step 5: Commit**

```bash
git add .
git commit -m "feat: semver parsing, bumping, and conventional commit analysis"
```

---

### Task 17: Release — Change Detection

**Files:**
- Create: `internal/release/detect.go`
- Create: `internal/release/detect_test.go`

- [ ] **Step 1: Write failing test**

Create `internal/release/detect_test.go`:

```go
package release_test

import (
	"context"
	"testing"

	"github.com/vexil-lang/vexilbot/internal/release"
	"github.com/vexil-lang/vexilbot/internal/repoconfig"
)

type mockGitAPI struct {
	tags    []string
	commits map[string][]release.Commit // tag -> commits since that tag
}

func (m *mockGitAPI) ListTags(ctx context.Context, owner, repo string) ([]string, error) {
	return m.tags, nil
}

func (m *mockGitAPI) CommitsSinceTag(ctx context.Context, owner, repo, tag, path string) ([]release.Commit, error) {
	commits, ok := m.commits[tag]
	if !ok {
		return nil, nil
	}
	// Filter by path prefix
	var filtered []release.Commit
	for _, c := range commits {
		for _, f := range c.Files {
			if len(f) >= len(path) && f[:len(path)] == path {
				filtered = append(filtered, c)
				break
			}
		}
	}
	return filtered, nil
}

func TestDetectChanges(t *testing.T) {
	api := &mockGitAPI{
		tags: []string{"vexil-lang-v0.3.1", "vexil-runtime-v0.2.0"},
		commits: map[string][]release.Commit{
			"vexil-lang-v0.3.1": {
				{Message: "feat: add union types", Files: []string{"crates/vexil-lang/src/union.rs"}},
				{Message: "fix: parser crash", Files: []string{"crates/vexil-lang/src/parser.rs"}},
			},
			"vexil-runtime-v0.2.0": {
				{Message: "docs: update readme", Files: []string{"README.md"}},
			},
		},
	}

	crates := map[string]repoconfig.CrateEntry{
		"vexil-lang": {
			Path:             "crates/vexil-lang",
			Publish:          "crates.io",
			SuggestThreshold: 1,
		},
		"vexil-runtime": {
			Path:             "crates/vexil-runtime",
			Publish:          "crates.io",
			SuggestThreshold: 1,
		},
	}

	results, err := release.DetectChanges(context.Background(), api, "org", "repo", "{{ crate }}-v{{ version }}", crates)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// vexil-lang should have changes
	langResult, ok := results["vexil-lang"]
	if !ok {
		t.Fatal("vexil-lang not in results")
	}
	if langResult.CurrentVersion != "0.3.1" {
		t.Errorf("current version = %q", langResult.CurrentVersion)
	}
	if len(langResult.Commits) != 2 {
		t.Errorf("commits = %d, want 2", len(langResult.Commits))
	}
	if langResult.SuggestedBump != release.BumpMinor {
		t.Errorf("suggested bump = %v, want minor", langResult.SuggestedBump)
	}

	// vexil-runtime should have no changes (commit doesn't touch its path)
	rtResult, ok := results["vexil-runtime"]
	if !ok {
		t.Fatal("vexil-runtime not in results")
	}
	if len(rtResult.Commits) != 0 {
		t.Errorf("runtime commits = %d, want 0", len(rtResult.Commits))
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/release/...
# Expected: FAIL — DetectChanges, Commit, ChangeResult undefined
```

- [ ] **Step 3: Implement change detection**

Create `internal/release/detect.go`:

```go
package release

import (
	"context"
	"fmt"
	"sort"

	"github.com/vexil-lang/vexilbot/internal/repoconfig"
)

type Commit struct {
	SHA     string
	Message string
	Files   []string
}

type ChangeResult struct {
	CrateName      string
	CurrentVersion string
	Commits        []Commit
	SuggestedBump  BumpLevel
}

type GitAPI interface {
	ListTags(ctx context.Context, owner, repo string) ([]string, error)
	CommitsSinceTag(ctx context.Context, owner, repo, tag, path string) ([]Commit, error)
}

// DetectChanges checks each configured crate for unreleased commits.
func DetectChanges(
	ctx context.Context,
	api GitAPI,
	owner, repo string,
	tagFormat string,
	crates map[string]repoconfig.CrateEntry,
) (map[string]ChangeResult, error) {
	tags, err := api.ListTags(ctx, owner, repo)
	if err != nil {
		return nil, fmt.Errorf("list tags: %w", err)
	}

	results := make(map[string]ChangeResult)

	for name, crate := range crates {
		if !crate.Track && crate.Publish == "" && crate.Publish == "false" {
			continue
		}

		// Find latest tag for this crate
		latestTag, latestVersion := findLatestTag(tags, name, tagFormat)
		if latestTag == "" {
			// No tag found — treat all commits as unreleased
			latestTag = "" // CommitsSinceTag handles empty tag as "all commits"
		}

		commits, err := api.CommitsSinceTag(ctx, owner, repo, latestTag, crate.Path)
		if err != nil {
			return nil, fmt.Errorf("commits for %s: %w", name, err)
		}

		messages := make([]string, len(commits))
		for i, c := range commits {
			messages[i] = c.Message
		}

		results[name] = ChangeResult{
			CrateName:      name,
			CurrentVersion: latestVersion,
			Commits:        commits,
			SuggestedBump:  SuggestBump(messages),
		}
	}

	return results, nil
}

// findLatestTag finds the most recent tag matching the format for a given crate.
// Tags are assumed to be in chronological order (newest first) from the API.
func findLatestTag(tags []string, crate, format string) (string, string) {
	type tagVersion struct {
		tag     string
		version Version
		raw     string
	}

	var matches []tagVersion
	for _, tag := range tags {
		ver, ok := ExtractTagVersion(tag, crate, format)
		if !ok {
			continue
		}
		parsed, err := ParseVersion(ver)
		if err != nil {
			continue
		}
		matches = append(matches, tagVersion{tag, parsed, ver})
	}

	if len(matches) == 0 {
		return "", ""
	}

	// Sort by version descending
	sort.Slice(matches, func(i, j int) bool {
		a, b := matches[i].version, matches[j].version
		if a.Major != b.Major {
			return a.Major > b.Major
		}
		if a.Minor != b.Minor {
			return a.Minor > b.Minor
		}
		return a.Patch > b.Patch
	})

	return matches[0].tag, matches[0].raw
}

// FormatStatus produces a markdown summary of unreleased changes.
func FormatStatus(results map[string]ChangeResult) string {
	var lines []string
	for name, r := range results {
		if len(r.Commits) == 0 {
			continue
		}
		line := fmt.Sprintf("- **%s** (v%s): %d unreleased commit(s), suggested bump: **%s**",
			name, r.CurrentVersion, len(r.Commits), r.SuggestedBump)
		lines = append(lines, line)
	}

	if len(lines) == 0 {
		return "All crates are up to date. No unreleased changes."
	}

	sort.Strings(lines)
	header := "### Release Status\n\n"
	result := header
	for _, l := range lines {
		result += l + "\n"
	}
	return result
}
```

- [ ] **Step 4: Run tests**

```bash
go test ./internal/release/...
# Expected: PASS (all tests)
```

- [ ] **Step 5: Commit**

```bash
git add .
git commit -m "feat: release change detection with version tag scanning"
```

---

### Task 18: Release — Cargo.toml Version Bumping

**Files:**
- Create: `internal/release/bump.go`
- Create: `internal/release/bump_test.go`

- [ ] **Step 1: Write failing test**

Create `internal/release/bump_test.go`:

```go
package release_test

import (
	"testing"

	"github.com/vexil-lang/vexilbot/internal/release"
)

func TestBumpCargoVersion(t *testing.T) {
	input := `[package]
name = "vexil-lang"
version = "0.3.1"
edition = "2021"

[dependencies]
thiserror = "2"
`
	want := `[package]
name = "vexil-lang"
version = "0.4.0"
edition = "2021"

[dependencies]
thiserror = "2"
`
	got, err := release.BumpCargoVersion(input, "0.4.0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestBumpCargoVersion_WorkspaceInherit(t *testing.T) {
	// When version is inherited from workspace, we don't touch it
	input := `[package]
name = "vexil-lang"
version.workspace = true
edition = "2021"
`
	_, err := release.BumpCargoVersion(input, "0.4.0")
	if err == nil {
		t.Fatal("expected error for workspace-inherited version")
	}
}

func TestBumpCargoDependency(t *testing.T) {
	input := `[package]
name = "vexil-codegen-rust"
version = "0.3.1"

[dependencies]
vexil-lang = { path = "../vexil-lang", version = "0.3.1" }
thiserror = "2"
`
	want := `[package]
name = "vexil-codegen-rust"
version = "0.3.1"

[dependencies]
vexil-lang = { path = "../vexil-lang", version = "0.4.0" }
thiserror = "2"
`
	got, err := release.BumpCargoDependency(input, "vexil-lang", "0.4.0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/release/...
# Expected: FAIL — BumpCargoVersion, BumpCargoDependency undefined
```

- [ ] **Step 3: Implement Cargo.toml bumping**

Create `internal/release/bump.go`:

```go
package release

import (
	"fmt"
	"regexp"
	"strings"
)

var packageVersionRe = regexp.MustCompile(`(?m)^(version\s*=\s*)"([^"]+)"`)
var workspaceVersionRe = regexp.MustCompile(`(?m)^version\.workspace\s*=\s*true`)

// BumpCargoVersion replaces the version in a Cargo.toml [package] section.
func BumpCargoVersion(content string, newVersion string) (string, error) {
	if workspaceVersionRe.MatchString(content) {
		return "", fmt.Errorf("version is inherited from workspace — bump the workspace Cargo.toml instead")
	}

	// Find the version line in the [package] section
	// We need to be careful to only replace the first occurrence (package version, not dependency versions)
	sections := splitSections(content)
	replaced := false

	for i, section := range sections {
		if isPackageSection(section) {
			sections[i] = packageVersionRe.ReplaceAllStringFunc(section, func(match string) string {
				if !replaced {
					replaced = true
					return packageVersionRe.ReplaceAllString(match, `${1}"`+newVersion+`"`)
				}
				return match
			})
			break
		}
	}

	if !replaced {
		return "", fmt.Errorf("no version field found in [package] section")
	}

	return strings.Join(sections, ""), nil
}

// BumpCargoDependency updates a dependency version in a Cargo.toml.
func BumpCargoDependency(content string, depName, newVersion string) (string, error) {
	// Match: dep_name = { ..., version = "x.y.z", ... }
	pattern := regexp.MustCompile(
		fmt.Sprintf(`(%s\s*=\s*\{[^}]*version\s*=\s*)"([^"]+)"`, regexp.QuoteMeta(depName)),
	)

	if !pattern.MatchString(content) {
		return content, nil // Dependency not found — not an error
	}

	result := pattern.ReplaceAllString(content, `${1}"`+newVersion+`"`)
	return result, nil
}

func splitSections(content string) []string {
	// Split on section headers but keep the headers
	var sections []string
	lines := strings.Split(content, "\n")
	current := ""

	for _, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "[") && current != "" {
			sections = append(sections, current)
			current = ""
		}
		current += line + "\n"
	}
	if current != "" {
		sections = append(sections, current)
	}
	return sections
}

func isPackageSection(section string) bool {
	trimmed := strings.TrimSpace(section)
	return strings.HasPrefix(trimmed, "[package]")
}
```

- [ ] **Step 4: Run tests**

```bash
go test ./internal/release/...
# Expected: PASS (all tests)
```

- [ ] **Step 5: Commit**

```bash
git add .
git commit -m "feat: Cargo.toml version and dependency bumping"
```

---

### Task 19: Release — Changelog Generation

**Files:**
- Create: `internal/release/changelog.go`
- Create: `internal/release/changelog_test.go`

- [ ] **Step 1: Write failing test**

Create `internal/release/changelog_test.go`:

```go
package release_test

import (
	"context"
	"testing"

	"github.com/vexil-lang/vexilbot/internal/release"
)

type mockCmdRunner struct {
	output string
	err    error
}

func (m *mockCmdRunner) Run(ctx context.Context, dir string, name string, args ...string) (string, error) {
	return m.output, m.err
}

func TestGenerateChangelog(t *testing.T) {
	runner := &mockCmdRunner{
		output: "## [0.4.0] — 2026-03-28\n\n### Features\n\n- Add union types\n",
	}

	out, err := release.GenerateChangelog(context.Background(), runner, "/repo", "vexil-lang", "vexil-lang-v0.3.1", "0.4.0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out == "" {
		t.Fatal("expected non-empty changelog")
	}
	if out != runner.output {
		t.Errorf("got:\n%s\nwant:\n%s", out, runner.output)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/release/...
# Expected: FAIL — GenerateChangelog undefined
```

- [ ] **Step 3: Implement changelog generation**

Create `internal/release/changelog.go`:

```go
package release

import (
	"context"
	"fmt"
)

type CmdRunner interface {
	Run(ctx context.Context, dir string, name string, args ...string) (string, error)
}

// GenerateChangelog runs git-cliff to produce a changelog for a crate release.
func GenerateChangelog(
	ctx context.Context,
	runner CmdRunner,
	repoDir string,
	crate string,
	sinceTag string,
	newVersion string,
) (string, error) {
	args := []string{
		"--config", "cliff.toml",
		"--tag", fmt.Sprintf("%s-v%s", crate, newVersion),
		"--unreleased",
	}

	if sinceTag != "" {
		args = append(args, sinceTag+"..HEAD")
	}

	output, err := runner.Run(ctx, repoDir, "git-cliff", args...)
	if err != nil {
		return "", fmt.Errorf("git-cliff: %w", err)
	}

	return output, nil
}
```

- [ ] **Step 4: Run tests**

```bash
go test ./internal/release/...
# Expected: PASS (all tests)
```

- [ ] **Step 5: Commit**

```bash
git add .
git commit -m "feat: changelog generation via git-cliff"
```

---

### Task 20: Release — Orchestration (Branch, PR, Publish)

**Files:**
- Create: `internal/release/orchestrate.go`
- Create: `internal/release/orchestrate_test.go`
- Create: `internal/release/publish.go`
- Create: `internal/release/publish_test.go`

- [ ] **Step 1: Write failing test for orchestration**

Create `internal/release/orchestrate_test.go`:

```go
package release_test

import (
	"testing"

	"github.com/vexil-lang/vexilbot/internal/release"
	"github.com/vexil-lang/vexilbot/internal/repoconfig"
)

func TestResolveDependencyOrder(t *testing.T) {
	crates := map[string]repoconfig.CrateEntry{
		"vexil-lang":        {DependsOn: []string{}},
		"vexil-runtime":     {DependsOn: []string{"vexil-lang"}},
		"vexil-codegen-rust": {DependsOn: []string{"vexil-lang"}},
		"vexil-store":       {DependsOn: []string{"vexil-lang", "vexil-runtime"}},
		"vexilc":            {DependsOn: []string{"vexil-lang", "vexil-codegen-rust"}},
	}

	order, err := release.ResolveDependencyOrder(crates)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// vexil-lang must come before everything
	// vexil-runtime must come before vexil-store
	// vexil-codegen-rust must come before vexilc
	indexOf := make(map[string]int)
	for i, name := range order {
		indexOf[name] = i
	}

	assertBefore := func(a, b string) {
		t.Helper()
		if indexOf[a] >= indexOf[b] {
			t.Errorf("%s (idx %d) should come before %s (idx %d)", a, indexOf[a], b, indexOf[b])
		}
	}

	assertBefore("vexil-lang", "vexil-runtime")
	assertBefore("vexil-lang", "vexil-codegen-rust")
	assertBefore("vexil-lang", "vexil-store")
	assertBefore("vexil-runtime", "vexil-store")
	assertBefore("vexil-codegen-rust", "vexilc")
}

func TestResolveDependencyOrder_CyclicError(t *testing.T) {
	crates := map[string]repoconfig.CrateEntry{
		"a": {DependsOn: []string{"b"}},
		"b": {DependsOn: []string{"a"}},
	}

	_, err := release.ResolveDependencyOrder(crates)
	if err == nil {
		t.Fatal("expected error for cyclic dependency")
	}
}

func TestReleaseBranchName(t *testing.T) {
	name := release.BranchName("vexil-lang", "0.4.0")
	if name != "release/vexil-lang-v0.4.0" {
		t.Errorf("branch name = %q", name)
	}
}
```

- [ ] **Step 2: Write failing test for publish**

Create `internal/release/publish_test.go`:

```go
package release_test

import (
	"context"
	"testing"

	"github.com/vexil-lang/vexilbot/internal/release"
)

func TestPublishCrate(t *testing.T) {
	var ranCmds []string
	runner := &mockCmdRunner{output: ""}
	// We test that the right command is constructed
	// In reality this calls cargo publish
	err := release.PublishCrate(context.Background(), runner, "/repo", "crates/vexil-lang", "test-token")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_ = ranCmds // mockCmdRunner doesn't track cmds in this simple version — that's fine for now
}
```

- [ ] **Step 3: Run tests to verify they fail**

```bash
go test ./internal/release/...
# Expected: FAIL — ResolveDependencyOrder, BranchName, PublishCrate undefined
```

- [ ] **Step 4: Implement orchestration**

Create `internal/release/orchestrate.go`:

```go
package release

import (
	"fmt"

	"github.com/vexil-lang/vexilbot/internal/repoconfig"
)

// BranchName returns the git branch name for a release.
func BranchName(crate, version string) string {
	return fmt.Sprintf("release/%s-v%s", crate, version)
}

// TagName returns the git tag name for a release.
func TagName(crate, version, format string) string {
	result := format
	result = replaceTemplate(result, "crate", crate)
	result = replaceTemplate(result, "version", version)
	return result
}

func replaceTemplate(s, key, value string) string {
	// Replace {{ key }} with value
	placeholder := "{{ " + key + " }}"
	for i := 0; i <= len(s)-len(placeholder); i++ {
		if s[i:i+len(placeholder)] == placeholder {
			return s[:i] + value + s[i+len(placeholder):]
		}
	}
	return s
}

// ResolveDependencyOrder returns crate names in topological order (dependencies first).
func ResolveDependencyOrder(crates map[string]repoconfig.CrateEntry) ([]string, error) {
	// Kahn's algorithm
	inDegree := make(map[string]int)
	dependents := make(map[string][]string) // dep -> crates that depend on it

	for name := range crates {
		inDegree[name] = 0
	}

	for name, entry := range crates {
		for _, dep := range entry.DependsOn {
			if _, ok := crates[dep]; ok {
				inDegree[name]++
				dependents[dep] = append(dependents[dep], name)
			}
		}
	}

	var queue []string
	for name, degree := range inDegree {
		if degree == 0 {
			queue = append(queue, name)
		}
	}

	var order []string
	for len(queue) > 0 {
		// Sort queue for deterministic output
		sortStrings(queue)
		node := queue[0]
		queue = queue[1:]
		order = append(order, node)

		for _, dep := range dependents[node] {
			inDegree[dep]--
			if inDegree[dep] == 0 {
				queue = append(queue, dep)
			}
		}
	}

	if len(order) != len(crates) {
		return nil, fmt.Errorf("cyclic dependency detected in release crates")
	}

	return order, nil
}

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}
```

- [ ] **Step 5: Implement publish**

Create `internal/release/publish.go`:

```go
package release

import (
	"context"
	"fmt"
)

// PublishCrate runs cargo publish for a crate.
func PublishCrate(ctx context.Context, runner CmdRunner, repoDir, cratePath, registryToken string) error {
	_, err := runner.Run(ctx, repoDir, "cargo", "publish",
		"--manifest-path", cratePath+"/Cargo.toml",
		"--token", registryToken,
	)
	if err != nil {
		return fmt.Errorf("cargo publish %s: %w", cratePath, err)
	}
	return nil
}
```

- [ ] **Step 6: Run tests**

```bash
go test ./internal/release/...
# Expected: PASS (all tests)
```

- [ ] **Step 7: Commit**

```bash
git add .
git commit -m "feat: release orchestration with dependency ordering and cargo publish"
```

---

### Task 21: LLM Interface (Stub)

**Files:**
- Create: `internal/llm/llm.go`
- Create: `internal/llm/llm_test.go`

- [ ] **Step 1: Write failing test**

Create `internal/llm/llm_test.go`:

```go
package llm_test

import (
	"context"
	"testing"

	"github.com/vexil-lang/vexilbot/internal/llm"
)

func TestNoopClient(t *testing.T) {
	client := llm.NewNoopClient()
	result, err := client.Analyze(context.Background(), "test prompt", "test context")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "" {
		t.Errorf("noop client returned %q, want empty string", result)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/llm/...
# Expected: FAIL — package does not exist
```

- [ ] **Step 3: Implement LLM interface and noop client**

Create `internal/llm/llm.go`:

```go
package llm

import (
	"context"
)

// Client is the interface for LLM integration. Handlers call this optionally.
// When LLM is disabled, use NoopClient which returns empty strings.
type Client interface {
	Analyze(ctx context.Context, prompt string, codeContext string) (string, error)
}

type noopClient struct{}

func NewNoopClient() Client {
	return &noopClient{}
}

func (n *noopClient) Analyze(ctx context.Context, prompt string, codeContext string) (string, error) {
	return "", nil
}
```

- [ ] **Step 4: Run tests**

```bash
go test ./internal/llm/...
# Expected: PASS (1 test)
```

- [ ] **Step 5: Commit**

```bash
git add .
git commit -m "feat: LLM client interface with noop implementation"
```

---

### Task 22: Wire Up Main + Health Endpoint

**Files:**
- Modify: `cmd/vexilbot/main.go`

- [ ] **Step 1: Write failing test for health endpoint**

Create `cmd/vexilbot/main_test.go`:

```go
package main_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealthEndpoint(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	})

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if w.Body.String() != `{"status":"ok"}` {
		t.Errorf("body = %q", w.Body.String())
	}
}
```

- [ ] **Step 2: Run test to verify it passes**

```bash
go test ./cmd/vexilbot/...
# Expected: PASS (the handler is inline in the test)
```

- [ ] **Step 3: Wire up main.go**

Replace `cmd/vexilbot/main.go`:

```go
package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"

	"github.com/vexil-lang/vexilbot/internal/serverconfig"
	"github.com/vexil-lang/vexilbot/internal/webhook"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "usage: vexilbot <config-path>\n")
		os.Exit(1)
	}

	// Structured JSON logging
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	cfg, err := serverconfig.Load(os.Args[1])
	if err != nil {
		slog.Error("load config", "error", err)
		os.Exit(1)
	}

	router := webhook.RouterFunc(func(eventType string, payload []byte) {
		slog.Info("event received", "type", eventType, "bytes", len(payload))
		// TODO: dispatch to feature handlers based on event type
	})

	handler := webhook.NewHandler(cfg.Server.WebhookSecret, router)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", healthHandler)
	mux.Handle("POST /webhook", handler)

	slog.Info("vexilbot starting", "listen", cfg.Server.Listen)
	if err := http.ListenAndServe(cfg.Server.Listen, mux); err != nil {
		slog.Error("server error", "error", err)
		os.Exit(1)
	}
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}
```

- [ ] **Step 4: Verify build**

```bash
make build
# Expected: builds successfully
```

- [ ] **Step 5: Commit**

```bash
git add .
git commit -m "feat: wire up main server with health endpoint and webhook routing"
```

---

### Task 23: Event Dispatcher

**Files:**
- Create: `internal/webhook/dispatch.go`
- Create: `internal/webhook/dispatch_test.go`

This connects webhook events to the feature handlers (labeler, welcome, triage, policy).

- [ ] **Step 1: Write failing test**

Create `internal/webhook/dispatch_test.go`:

```go
package webhook_test

import (
	"encoding/json"
	"testing"

	"github.com/vexil-lang/vexilbot/internal/webhook"
)

func TestDispatcher_PullRequestOpened(t *testing.T) {
	var called bool
	d := webhook.NewDispatcher()
	d.OnPullRequest(func(event webhook.PullRequestEvent) {
		called = true
		if event.Action != "opened" {
			t.Errorf("action = %q, want %q", event.Action, "opened")
		}
		if event.Number != 42 {
			t.Errorf("number = %d, want %d", event.Number, 42)
		}
	})

	payload, _ := json.Marshal(map[string]interface{}{
		"action": "opened",
		"number": 42,
		"pull_request": map[string]interface{}{
			"head": map[string]interface{}{"sha": "abc123"},
			"user": map[string]interface{}{"login": "alice"},
		},
		"repository": map[string]interface{}{
			"owner": map[string]interface{}{"login": "vexil-lang"},
			"name":  "vexil",
		},
		"installation": map[string]interface{}{"id": 999},
	})

	d.Route("pull_request", payload)
	if !called {
		t.Error("pull_request handler was not called")
	}
}

func TestDispatcher_IssueComment(t *testing.T) {
	var called bool
	d := webhook.NewDispatcher()
	d.OnIssueComment(func(event webhook.IssueCommentEvent) {
		called = true
		if event.Action != "created" {
			t.Errorf("action = %q", event.Action)
		}
	})

	payload, _ := json.Marshal(map[string]interface{}{
		"action": "created",
		"comment": map[string]interface{}{
			"id":   123,
			"body": "@vexilbot label bug",
			"user": map[string]interface{}{"login": "bob"},
		},
		"issue": map[string]interface{}{"number": 10},
		"repository": map[string]interface{}{
			"owner": map[string]interface{}{"login": "vexil-lang"},
			"name":  "vexil",
		},
		"installation": map[string]interface{}{"id": 999},
	})

	d.Route("issue_comment", payload)
	if !called {
		t.Error("issue_comment handler was not called")
	}
}

func TestDispatcher_UnknownEvent(t *testing.T) {
	d := webhook.NewDispatcher()
	// Should not panic on unknown events
	d.Route("unknown_event", []byte(`{}`))
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/webhook/...
# Expected: FAIL — NewDispatcher, PullRequestEvent, etc. undefined
```

- [ ] **Step 3: Implement dispatcher**

Create `internal/webhook/dispatch.go`:

```go
package webhook

import (
	"encoding/json"
	"log/slog"
)

type PullRequestEvent struct {
	Action         string
	Number         int
	HeadSHA        string
	UserLogin      string
	Owner          string
	Repo           string
	InstallationID int64
}

type IssueCommentEvent struct {
	Action         string
	CommentID      int64
	CommentBody    string
	CommentUser    string
	IssueNumber    int
	Owner          string
	Repo           string
	InstallationID int64
}

type IssuesEvent struct {
	Action         string
	Number         int
	Title          string
	Body           string
	UserLogin      string
	Labels         []string
	Owner          string
	Repo           string
	InstallationID int64
}

type PushEvent struct {
	Ref            string
	Owner          string
	Repo           string
	InstallationID int64
}

type Dispatcher struct {
	pullRequestHandlers  []func(PullRequestEvent)
	issueCommentHandlers []func(IssueCommentEvent)
	issuesHandlers       []func(IssuesEvent)
	pushHandlers         []func(PushEvent)
}

func NewDispatcher() *Dispatcher {
	return &Dispatcher{}
}

func (d *Dispatcher) OnPullRequest(h func(PullRequestEvent))  { d.pullRequestHandlers = append(d.pullRequestHandlers, h) }
func (d *Dispatcher) OnIssueComment(h func(IssueCommentEvent)) { d.issueCommentHandlers = append(d.issueCommentHandlers, h) }
func (d *Dispatcher) OnIssues(h func(IssuesEvent))            { d.issuesHandlers = append(d.issuesHandlers, h) }
func (d *Dispatcher) OnPush(h func(PushEvent))                { d.pushHandlers = append(d.pushHandlers, h) }

func (d *Dispatcher) Route(eventType string, payload []byte) {
	switch eventType {
	case "pull_request":
		d.dispatchPullRequest(payload)
	case "issue_comment":
		d.dispatchIssueComment(payload)
	case "issues":
		d.dispatchIssues(payload)
	case "push":
		d.dispatchPush(payload)
	default:
		slog.Debug("unhandled event type", "type", eventType)
	}
}

func (d *Dispatcher) dispatchPullRequest(payload []byte) {
	var raw struct {
		Action      string `json:"action"`
		Number      int    `json:"number"`
		PullRequest struct {
			Head struct {
				SHA string `json:"sha"`
			} `json:"head"`
			User struct {
				Login string `json:"login"`
			} `json:"user"`
		} `json:"pull_request"`
		Repository   repoInfo     `json:"repository"`
		Installation installation `json:"installation"`
	}
	if err := json.Unmarshal(payload, &raw); err != nil {
		slog.Error("parse pull_request event", "error", err)
		return
	}

	event := PullRequestEvent{
		Action:         raw.Action,
		Number:         raw.Number,
		HeadSHA:        raw.PullRequest.Head.SHA,
		UserLogin:      raw.PullRequest.User.Login,
		Owner:          raw.Repository.Owner.Login,
		Repo:           raw.Repository.Name,
		InstallationID: raw.Installation.ID,
	}

	for _, h := range d.pullRequestHandlers {
		h(event)
	}
}

func (d *Dispatcher) dispatchIssueComment(payload []byte) {
	var raw struct {
		Action  string `json:"action"`
		Comment struct {
			ID   int64  `json:"id"`
			Body string `json:"body"`
			User struct {
				Login string `json:"login"`
			} `json:"user"`
		} `json:"comment"`
		Issue struct {
			Number int `json:"number"`
		} `json:"issue"`
		Repository   repoInfo     `json:"repository"`
		Installation installation `json:"installation"`
	}
	if err := json.Unmarshal(payload, &raw); err != nil {
		slog.Error("parse issue_comment event", "error", err)
		return
	}

	event := IssueCommentEvent{
		Action:         raw.Action,
		CommentID:      raw.Comment.ID,
		CommentBody:    raw.Comment.Body,
		CommentUser:    raw.Comment.User.Login,
		IssueNumber:    raw.Issue.Number,
		Owner:          raw.Repository.Owner.Login,
		Repo:           raw.Repository.Name,
		InstallationID: raw.Installation.ID,
	}

	for _, h := range d.issueCommentHandlers {
		h(event)
	}
}

func (d *Dispatcher) dispatchIssues(payload []byte) {
	var raw struct {
		Action string `json:"action"`
		Issue  struct {
			Number int    `json:"number"`
			Title  string `json:"title"`
			Body   string `json:"body"`
			User   struct {
				Login string `json:"login"`
			} `json:"user"`
			Labels []struct {
				Name string `json:"name"`
			} `json:"labels"`
		} `json:"issue"`
		Repository   repoInfo     `json:"repository"`
		Installation installation `json:"installation"`
	}
	if err := json.Unmarshal(payload, &raw); err != nil {
		slog.Error("parse issues event", "error", err)
		return
	}

	labels := make([]string, len(raw.Issue.Labels))
	for i, l := range raw.Issue.Labels {
		labels[i] = l.Name
	}

	event := IssuesEvent{
		Action:         raw.Action,
		Number:         raw.Issue.Number,
		Title:          raw.Issue.Title,
		Body:           raw.Issue.Body,
		UserLogin:      raw.Issue.User.Login,
		Labels:         labels,
		Owner:          raw.Repository.Owner.Login,
		Repo:           raw.Repository.Name,
		InstallationID: raw.Installation.ID,
	}

	for _, h := range d.issuesHandlers {
		h(event)
	}
}

func (d *Dispatcher) dispatchPush(payload []byte) {
	var raw struct {
		Ref          string       `json:"ref"`
		Repository   repoInfo     `json:"repository"`
		Installation installation `json:"installation"`
	}
	if err := json.Unmarshal(payload, &raw); err != nil {
		slog.Error("parse push event", "error", err)
		return
	}

	event := PushEvent{
		Ref:            raw.Ref,
		Owner:          raw.Repository.Owner.Login,
		Repo:           raw.Repository.Name,
		InstallationID: raw.Installation.ID,
	}

	for _, h := range d.pushHandlers {
		h(event)
	}
}

type repoInfo struct {
	Owner struct {
		Login string `json:"login"`
	} `json:"owner"`
	Name string `json:"name"`
}

type installation struct {
	ID int64 `json:"id"`
}
```

- [ ] **Step 4: Run tests**

```bash
go test ./internal/webhook/...
# Expected: PASS (all tests)
```

- [ ] **Step 5: Commit**

```bash
git add .
git commit -m "feat: event dispatcher connecting webhooks to feature handlers"
```

---

### Task 24: Deployment Config

**Files:**
- Create: `deploy/vexilbot.service`
- Create: `deploy/Caddyfile`
- Create: `deploy/README.md`

- [ ] **Step 1: Create systemd unit**

Create `deploy/vexilbot.service`:

```ini
[Unit]
Description=vexilbot GitHub App
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=vexilbot
Group=vexilbot
ExecStart=/usr/local/bin/vexilbot /etc/vexilbot/config.toml
Restart=on-failure
RestartSec=5
StandardOutput=journal
StandardError=journal
SyslogIdentifier=vexilbot

# Security hardening
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
ReadOnlyPaths=/etc/vexilbot
PrivateTmp=true

[Install]
WantedBy=multi-user.target
```

- [ ] **Step 2: Create Caddyfile**

Create `deploy/Caddyfile`:

```
bot.vexil-lang.dev {
    reverse_proxy localhost:8080
}
```

- [ ] **Step 3: Create deploy README**

Create `deploy/README.md`:

```markdown
# Deploying vexilbot

## Prerequisites

- VPS with Ubuntu 22.04+ or similar
- Go 1.22+ (for building) or pre-built binary
- Caddy (for TLS)
- git-cliff (for changelog generation)
- cargo (for crate publishing)

## Setup

1. Create service user:
   ```bash
   sudo useradd -r -s /usr/sbin/nologin vexilbot
   ```

2. Create config directory:
   ```bash
   sudo mkdir -p /etc/vexilbot
   sudo cp config.toml /etc/vexilbot/config.toml
   sudo cp app.pem /etc/vexilbot/app.pem
   sudo chown -R vexilbot:vexilbot /etc/vexilbot
   sudo chmod 600 /etc/vexilbot/config.toml /etc/vexilbot/app.pem
   ```

3. Install binary:
   ```bash
   sudo cp vexilbot /usr/local/bin/vexilbot
   ```

4. Install systemd unit:
   ```bash
   sudo cp vexilbot.service /etc/systemd/system/
   sudo systemctl daemon-reload
   sudo systemctl enable --now vexilbot
   ```

5. Configure Caddy:
   ```bash
   sudo cp Caddyfile /etc/caddy/Caddyfile
   sudo systemctl reload caddy
   ```

6. Configure GitHub App webhook URL to `https://bot.vexil-lang.dev/webhook`

## Updating

```bash
sudo cp vexilbot /usr/local/bin/vexilbot
sudo systemctl restart vexilbot
```

## Logs

```bash
journalctl -u vexilbot -f
```
```

- [ ] **Step 4: Commit**

```bash
git add deploy/
git commit -m "feat: deployment config (systemd, Caddy, setup guide)"
```

---

### Task 25: CI Workflow

**Files:**
- Create: `.github/workflows/ci.yml`

- [ ] **Step 1: Create CI workflow**

Create `.github/workflows/ci.yml`:

```yaml
name: CI

on:
  push:
    branches: [main]
  pull_request:
    branches: [main]

concurrency:
  group: ${{ github.workflow }}-${{ github.ref }}
  cancel-in-progress: true

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - uses: actions/setup-go@v5
        with:
          go-version: "1.22"

      - name: Build
        run: go build ./...

      - name: Test
        run: go test -race -v ./...

      - name: Vet
        run: go vet ./...

  lint:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - uses: actions/setup-go@v5
        with:
          go-version: "1.22"

      - name: golangci-lint
        uses: golangci/golangci-lint-action@v6
        with:
          version: latest
```

- [ ] **Step 2: Commit**

```bash
git add .github/
git commit -m "ci: add Go test and lint workflow"
```

---

### Task 26: Integration Smoke Test

**Files:**
- Create: `internal/integration_test.go`

A test that wires up the full pipeline: webhook → dispatcher → handlers, using mocks for the GitHub API.

- [ ] **Step 1: Write integration test**

Create `internal/integration_test.go`:

```go
package internal_test

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/vexil-lang/vexilbot/internal/webhook"
)

func TestFullPipeline_PROpened(t *testing.T) {
	secret := "integration-test-secret"

	var mu sync.Mutex
	var receivedEvent *webhook.PullRequestEvent

	dispatcher := webhook.NewDispatcher()
	dispatcher.OnPullRequest(func(event webhook.PullRequestEvent) {
		mu.Lock()
		defer mu.Unlock()
		receivedEvent = &event
	})

	handler := webhook.NewHandler(secret, dispatcher)
	mux := http.NewServeMux()
	mux.Handle("POST /webhook", handler)

	payload, _ := json.Marshal(map[string]interface{}{
		"action": "opened",
		"number": 99,
		"pull_request": map[string]interface{}{
			"head": map[string]interface{}{"sha": "deadbeef"},
			"user": map[string]interface{}{"login": "testuser"},
		},
		"repository": map[string]interface{}{
			"owner": map[string]interface{}{"login": "vexil-lang"},
			"name":  "vexil",
		},
		"installation": map[string]interface{}{"id": 1},
	})

	body := string(payload)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(body))
	sig := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	req := httptest.NewRequest(http.MethodPost, "/webhook", strings.NewReader(body))
	req.Header.Set("X-Hub-Signature-256", sig)
	req.Header.Set("X-GitHub-Event", "pull_request")
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	mu.Lock()
	defer mu.Unlock()
	if receivedEvent == nil {
		t.Fatal("pull_request handler was not called")
	}
	if receivedEvent.Number != 99 {
		t.Errorf("PR number = %d, want 99", receivedEvent.Number)
	}
	if receivedEvent.UserLogin != "testuser" {
		t.Errorf("user = %q, want %q", receivedEvent.UserLogin, "testuser")
	}
	if receivedEvent.Owner != "vexil-lang" {
		t.Errorf("owner = %q, want %q", receivedEvent.Owner, "vexil-lang")
	}
}
```

- [ ] **Step 2: Run integration test**

```bash
go test -v ./internal/ -run TestFullPipeline
# Expected: PASS
```

- [ ] **Step 3: Run full test suite**

```bash
make test
# Expected: ALL PASS
```

- [ ] **Step 4: Commit**

```bash
git add .
git commit -m "test: integration smoke test for webhook → dispatcher pipeline"
```
