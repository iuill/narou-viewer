package characters

import (
	"strings"
	"sync"
)

var novelStateLocks sync.Map

func withNovelStateLock(novelID string, fn func() error) error {
	key := strings.TrimSpace(novelID)
	if key == "" {
		return fn()
	}
	value, _ := novelStateLocks.LoadOrStore(key, &sync.Mutex{})
	mu := value.(*sync.Mutex)
	mu.Lock()
	defer mu.Unlock()
	return fn()
}
