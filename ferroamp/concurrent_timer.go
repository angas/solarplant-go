package ferroamp

import (
	"sync"
	"time"
)

type ConcurrentTimer struct {
	at time.Time
	mu sync.RWMutex
}

func (t *ConcurrentTimer) Reset() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.at = time.Now()
}

func (t *ConcurrentTimer) Elapsed() time.Duration {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return time.Since(t.at)
}
