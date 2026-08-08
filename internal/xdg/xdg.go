// Package xdg resolves tk's machine-local XDG config/state dirs and the
// machine-global flock over the config tier.
//
// Config uses ${XDG_CONFIG_HOME:-~/.config}/tk directly rather than
// os.UserConfigDir, which returns the wrong location on macOS.
package xdg

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

const lockName = ".tk.lock"

// ConfigDir returns $XDG_CONFIG_HOME/tk or ~/.config/tk. Path is not created.
func ConfigDir() (string, error) {
	if base := os.Getenv("XDG_CONFIG_HOME"); base != "" {
		return filepath.Join(base, "tk"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory for XDG config: %w", err)
	}
	return filepath.Join(home, ".config", "tk"), nil
}

// StateDir returns $XDG_STATE_HOME/tk or ~/.local/state/tk (SQLite index; not synced).
func StateDir() (string, error) {
	if base := os.Getenv("XDG_STATE_HOME"); base != "" {
		return filepath.Join(base, "tk"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory for XDG state: %w", err)
	}
	return filepath.Join(home, ".local", "state", "tk"), nil
}

// Lock is a held machine-global flock over the XDG config tier.
type Lock struct {
	f *os.File
}

// AcquireConfigLock takes the exclusive flock at <configDir>/.tk.lock, creating
// the directory if needed. Hold across the full registry read-modify-write so two
// concurrent registrations cannot both pass a check the other's write then invalidates.
func AcquireConfigLock(configDir string) (*Lock, error) {
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		return nil, fmt.Errorf("create XDG config directory %s: %w", configDir, err)
	}
	p := filepath.Join(configDir, lockName)
	f, err := os.OpenFile(p, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open config lock %s: %w", p, err)
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("acquire config lock %s: %w", p, err)
	}
	return &Lock{f: f}, nil
}

// Release drops the flock and closes the underlying descriptor.
func (l *Lock) Release() error {
	if l == nil || l.f == nil {
		return nil
	}
	_ = syscall.Flock(int(l.f.Fd()), syscall.LOCK_UN)
	err := l.f.Close()
	l.f = nil
	return err
}
