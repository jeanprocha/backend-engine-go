package strategytags

import (
	"sync"
	"time"
)

// ListCache holds a TTL snapshot of ListAll for GET /strategy-tags (MVP; Redis pode substituir).
type ListCache struct {
	mu      sync.RWMutex
	rows    []Row
	expires time.Time
	ttl     time.Duration
}

// NewListCache creates a cache; ttl should be positive (ex. 3m).
func NewListCache(ttl time.Duration) *ListCache {
	if ttl <= 0 {
		ttl = 3 * time.Minute
	}
	return &ListCache{ttl: ttl}
}

// Get returns a copy of rows if still fresh.
func (c *ListCache) Get() ([]Row, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.expires.IsZero() || time.Now().After(c.expires) {
		return nil, false
	}
	out := make([]Row, len(c.rows))
	copy(out, c.rows)
	return out, true
}

// Set stores rows and refreshes expiry.
func (c *ListCache) Set(rows []Row) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.rows = make([]Row, len(rows))
	copy(c.rows, rows)
	c.expires = time.Now().Add(c.ttl)
}

// Invalidate forces the next GET to reload from DB.
func (c *ListCache) Invalidate() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.expires = time.Time{}
	c.rows = nil
}
