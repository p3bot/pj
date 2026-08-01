// Package flock provides exclusive, blocking POSIX advisory locks via syscall.Flock.
// Unix-only (macOS/Linux); no cross-clone or networked-filesystem coordination.
package flock

import (
	"fmt"
	"os"
	"syscall"
)

// Lock is a held exclusive flock, released exactly once.
type Lock struct {
	f *os.File
}

// Acquire opens path (creating if needed) and takes an exclusive flock, blocking until available.
// The parent directory must already exist.
func Acquire(path string) (*Lock, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open lock %s: %w", path, err)
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("acquire lock %s: %w", path, err)
	}
	return &Lock{f: f}, nil
}

// Release drops the flock and closes the descriptor. Safe on a nil or already-released lock.
func (l *Lock) Release() error {
	if l == nil || l.f == nil {
		return nil
	}
	_ = syscall.Flock(int(l.f.Fd()), syscall.LOCK_UN)
	err := l.f.Close()
	l.f = nil
	return err
}
