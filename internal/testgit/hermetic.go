// Package testgit runs git in tests under a hermetic config env.
// Production code must keep the user's real config (credentials, SSH, hooks).
//
// Two entry points cover every git invocation in tests:
//   - Hermetic: before production internal/git (or gitroot) runs under a test
//   - Run / Combined / CombinedEnv / AllowFailure / CombinedAllowFailure: every test-spawned git subprocess
package testgit

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

// Hermetic clears global/system git config and templates for this test process so
// host ~/.gitconfig cannot leak into git. Local repo config set after init still applies.
//
// Call from requireGit (or equivalent) before exercising production code that shells
// out to git. Test-side spawns should use Run/Combined instead of calling Hermetic alone.
//
// Safe only outside t.Parallel. Unix-only OS support matches the product (linux || darwin).
func Hermetic(t *testing.T) {
	t.Helper()
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_SYSTEM", os.DevNull)
	// Empty means "no templates" (avoids default sample hooks under .git/hooks).
	t.Setenv("GIT_TEMPLATE_DIR", "")
}

// Run runs git in dir with hermetic env and fatals on non-zero exit.
func Run(t *testing.T, dir string, args ...string) {
	t.Helper()
	_ = Combined(t, dir, args...)
}

// Combined is Run but returns trimmed combined stdout+stderr.
func Combined(t *testing.T, dir string, args ...string) string {
	t.Helper()
	return CombinedEnv(t, dir, nil, args...)
}

// CombinedEnv is Combined with extra environment entries (e.g. GIT_AUTHOR_DATE=…).
func CombinedEnv(t *testing.T, dir string, extraEnv []string, args ...string) string {
	t.Helper()
	out, err := run(t, dir, extraEnv, args...)
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return out
}

// AllowFailure runs git with hermetic env and returns the process error (nil on success).
// Use for expected non-zero exits (e.g. rebase pausing on conflict) when output is unused.
func AllowFailure(t *testing.T, dir string, extraEnv []string, args ...string) error {
	t.Helper()
	_, err := run(t, dir, extraEnv, args...)
	return err
}

// CombinedAllowFailure is AllowFailure but also returns trimmed combined stdout+stderr.
// Use when a non-zero exit is acceptable and the caller needs the output (e.g. empty-repo log).
func CombinedAllowFailure(t *testing.T, dir string, args ...string) (string, error) {
	t.Helper()
	return run(t, dir, nil, args...)
}

func run(t *testing.T, dir string, extraEnv []string, args ...string) (string, error) {
	t.Helper()
	Hermetic(t)
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if len(extraEnv) > 0 {
		cmd.Env = append(os.Environ(), extraEnv...)
	}
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}
