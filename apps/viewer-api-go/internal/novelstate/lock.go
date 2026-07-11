package novelstate

import (
	"strings"
	"sync"
)

var locks sync.Map

// WithLock serializes state mutations for a novel across character, term, and
// extraction state stores.
func WithLock(novelID string, fn func() error) error {
	key := strings.TrimSpace(novelID)
	if key == "" {
		return fn()
	}
	value, _ := locks.LoadOrStore(key, &sync.Mutex{})
	mu := value.(*sync.Mutex)
	mu.Lock()
	defer mu.Unlock()
	return fn()
}
