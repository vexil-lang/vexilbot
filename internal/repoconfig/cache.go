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
