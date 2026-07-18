package statebarrier

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"golang.org/x/sys/unix"
)

const (
	ViewerAPILockRelativePath = "state/.viewer-api-writer.lock"
)

var ErrWriterActive = errors.New("state writer is active")

type Lock struct {
	mu   sync.Mutex
	file *os.File
	path string
}

func AcquireViewerAPI(dataDir string) (*Lock, error) {
	return Acquire(filepath.Join(filepath.Clean(dataDir), filepath.FromSlash(ViewerAPILockRelativePath)))
}

func Acquire(path string) (*Lock, error) {
	path = filepath.Clean(path)
	parent := filepath.Dir(path)
	if err := os.MkdirAll(parent, 0o700); err != nil {
		return nil, err
	}
	parentInfo, err := os.Lstat(parent)
	if err != nil || parentInfo.Mode()&os.ModeSymlink != 0 || !parentInfo.IsDir() {
		return nil, errors.Join(err, fmt.Errorf("writer lock parent must be a non-symlink directory: %s", parent))
	}
	fd, err := unix.Open(path, unix.O_RDWR|unix.O_CREAT|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open writer lock %s: %w", path, err)
	}
	file := os.NewFile(uintptr(fd), path)
	if file == nil {
		_ = unix.Close(fd)
		return nil, fmt.Errorf("open writer lock %s: invalid file descriptor", path)
	}
	closeOnError := func(err error) (*Lock, error) {
		_ = file.Close()
		return nil, err
	}
	info, err := file.Stat()
	if err != nil {
		return closeOnError(err)
	}
	if !info.Mode().IsRegular() {
		return closeOnError(fmt.Errorf("writer lock is not a regular file: %s", path))
	}
	if err := file.Chmod(0o600); err != nil {
		return closeOnError(err)
	}
	if err := unix.Flock(fd, unix.LOCK_EX|unix.LOCK_NB); err != nil {
		if errors.Is(err, unix.EWOULDBLOCK) || errors.Is(err, unix.EAGAIN) {
			return closeOnError(fmt.Errorf("%w: %s", ErrWriterActive, path))
		}
		return closeOnError(fmt.Errorf("lock writer barrier %s: %w", path, err))
	}
	return &Lock{file: file, path: path}, nil
}

func (l *Lock) Close() error {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file == nil {
		return nil
	}
	fd := int(l.file.Fd())
	unlockErr := unix.Flock(fd, unix.LOCK_UN)
	closeErr := l.file.Close()
	l.file = nil
	return errors.Join(unlockErr, closeErr)
}
