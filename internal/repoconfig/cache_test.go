package repoconfig_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/vexil-lang/vexilbot/internal/configoverride"
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

func TestCache_AppliesOverride(t *testing.T) {
	dir := t.TempDir()
	ovPath := configoverride.Path(dir, "owner", "repo")
	if err := configoverride.Save(ovPath, []byte("[welcome]\npr_message = \"from-override\"\n")); err != nil {
		t.Fatal(err)
	}
	calls := 0
	fetch := func(ctx context.Context, owner, repo string) (*repoconfig.Config, error) {
		calls++
		cfg := &repoconfig.Config{}
		cfg.Welcome.PRMessage = "from-base"
		return cfg, nil
	}
	cache := repoconfig.NewCacheWithOverrides(fetch, dir, time.Minute)
	got, err := cache.Get(context.Background(), "owner", "repo")
	if err != nil {
		t.Fatal(err)
	}
	if got.Welcome.PRMessage != "from-override" {
		t.Fatalf("expected from-override, got %q", got.Welcome.PRMessage)
	}
}
