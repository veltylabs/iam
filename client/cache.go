package client

import (
	"sync"

	"webtyp.com/time"
)

type authzEntry struct {
	userID  string
	scope   []string
	expires time.Unix
}

type scopeCache struct {
	mu      sync.RWMutex
	entries []authzEntry
}

func (c *scopeCache) Set(userID string, scope []string, expires time.Unix) {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := time.NowUnix()
	kept := c.entries[:0]
	for _, e := range c.entries {
		if e.expires > now && e.userID != userID {
			kept = append(kept, e)
		}
	}
	c.entries = append(kept, authzEntry{userID: userID, scope: scope, expires: expires})
}

func (c *scopeCache) Scope(userID string) ([]string, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	now := time.NowUnix()
	for _, e := range c.entries {
		if e.userID == userID && e.expires > now {
			return e.scope, true
		}
	}
	return nil, false
}
