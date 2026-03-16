package cache

import (
	"sync"
	"time"
)

type entry struct {
	ids       map[string]struct{}
	expiresAt time.Time
}

type GrantsCache struct {
	ttl   time.Duration
	mu    sync.RWMutex
	items map[string]entry
}

func NewGrantsCache(ttl time.Duration) *GrantsCache {
	return &GrantsCache{
		ttl:   ttl,
		items: map[string]entry{},
	}
}

func (c *GrantsCache) Get(key string) (map[string]struct{}, bool) {
	c.mu.RLock()
	item, ok := c.items[key]
	c.mu.RUnlock()
	if !ok || time.Now().After(item.expiresAt) {
		if ok {
			c.mu.Lock()
			delete(c.items, key)
			c.mu.Unlock()
		}
		return nil, false
	}
	copy := make(map[string]struct{}, len(item.ids))
	for k := range item.ids {
		copy[k] = struct{}{}
	}
	return copy, true
}

func (c *GrantsCache) Set(key string, ids map[string]struct{}) {
	copy := make(map[string]struct{}, len(ids))
	for k := range ids {
		copy[k] = struct{}{}
	}
	c.mu.Lock()
	c.items[key] = entry{ids: copy, expiresAt: time.Now().Add(c.ttl)}
	c.mu.Unlock()
}

func (c *GrantsCache) InvalidatePrefix(prefix string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for key := range c.items {
		if len(key) >= len(prefix) && key[:len(prefix)] == prefix {
			delete(c.items, key)
		}
	}
}
