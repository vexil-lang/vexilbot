package repoconfig

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// OverrideMerger is a function that applies local overrides on top of a fetched config.
// Implementations should return a (possibly new) *Config with overrides applied.
type OverrideMerger func(owner, repo string, cfg *Config) (*Config, error)

type Fetcher func(ctx context.Context, owner, repo string) (*Config, error)

type cacheEntry struct {
	config    *Config
	fetchedAt time.Time
}

type Cache struct {
	fetcher  Fetcher
	ttl      time.Duration
	mu       sync.RWMutex
	entries  map[string]cacheEntry
	override OverrideMerger // if non-nil, applied after each Get
}

func NewCache(fetcher Fetcher, ttl time.Duration) *Cache {
	return &Cache{
		fetcher: fetcher,
		ttl:     ttl,
		entries: make(map[string]cacheEntry),
	}
}

// NewCacheWithOverrides creates a Cache that applies local override files
// (from dataDir/overrides/<owner>-<repo>.toml) on top of fetched configs.
func NewCacheWithOverrides(fetcher Fetcher, dataDir string, ttl time.Duration) *Cache {
	c := NewCache(fetcher, ttl)
	if MakeFilesystemOverrideMerger != nil {
		c.SetOverrideMerger(MakeFilesystemOverrideMerger(dataDir))
	}
	return c
}

// SetOverrideMerger installs a custom override merger on the cache.
// This allows callers to inject the configoverride package without creating an import cycle.
func (c *Cache) SetOverrideMerger(fn OverrideMerger) {
	c.override = fn
}

// MakeFilesystemOverrideMerger is set by the configoverride package at init time
// to avoid an import cycle between repoconfig and configoverride.
// It must be set before any Cache with overrides is used.
var MakeFilesystemOverrideMerger func(dataDir string) OverrideMerger

func (c *Cache) Get(ctx context.Context, owner, repo string) (*Config, error) {
	key := fmt.Sprintf("%s/%s", owner, repo)

	c.mu.RLock()
	entry, ok := c.entries[key]
	c.mu.RUnlock()

	if ok && time.Since(entry.fetchedAt) < c.ttl {
		return c.applyOverride(owner, repo, entry.config)
	}

	cfg, err := c.fetcher(ctx, owner, repo)
	if err != nil {
		return nil, err
	}

	c.mu.Lock()
	c.entries[key] = cacheEntry{config: cfg, fetchedAt: time.Now()}
	c.mu.Unlock()

	return c.applyOverride(owner, repo, cfg)
}

// applyOverride applies the override merger (if any) on top of cfg.
func (c *Cache) applyOverride(owner, repo string, cfg *Config) (*Config, error) {
	if c.override == nil {
		return cfg, nil
	}
	merged, err := c.override(owner, repo, cfg)
	if err != nil {
		return nil, fmt.Errorf("apply override for %s/%s: %w", owner, repo, err)
	}
	return merged, nil
}
