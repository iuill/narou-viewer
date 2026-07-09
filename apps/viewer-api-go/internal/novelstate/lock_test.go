package novelstate

import (
	"sync"
	"testing"
)

func TestWithLockRunsBlankNovelImmediately(t *testing.T) {
	called := false
	if err := WithLock(" ", func() error {
		called = true
		return nil
	}); err != nil || !called {
		t.Fatalf("blank novel lock failed: called=%v err=%v", called, err)
	}
}

func TestWithLockSerializesSameNovel(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	done := make(chan struct{})
	go func() {
		_ = WithLock("novel-1", func() error {
			close(entered)
			<-release
			return nil
		})
	}()
	<-entered

	var once sync.Once
	go func() {
		_ = WithLock(" novel-1 ", func() error {
			once.Do(func() { close(done) })
			return nil
		})
	}()
	select {
	case <-done:
		t.Fatal("same novel lock entered before the first holder released it")
	default:
	}
	close(release)
	<-done
}
