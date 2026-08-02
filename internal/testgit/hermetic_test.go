package testgit

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestHermeticNullsGlobalAndSystemConfig(t *testing.T) {
	// Plant a hostile global config, then Hermetic must override it for the process.
	hostile := filepath.Join(t.TempDir(), "gitconfig")
	if err := os.WriteFile(hostile, []byte("[commit]\n\tgpgsign = true\n[core]\n\thooksPath = /nonexistent/hooks\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_CONFIG_GLOBAL", hostile)
	t.Setenv("GIT_CONFIG_SYSTEM", hostile)
	t.Setenv("GIT_TEMPLATE_DIR", filepath.Join(t.TempDir(), "templates"))

	Hermetic(t)

	if got := os.Getenv("GIT_CONFIG_GLOBAL"); got != os.DevNull {
		t.Errorf("GIT_CONFIG_GLOBAL = %q want %q", got, os.DevNull)
	}
	if got := os.Getenv("GIT_CONFIG_SYSTEM"); got != os.DevNull {
		t.Errorf("GIT_CONFIG_SYSTEM = %q want %q", got, os.DevNull)
	}
	if got := os.Getenv("GIT_TEMPLATE_DIR"); got != "" {
		t.Errorf("GIT_TEMPLATE_DIR = %q want empty", got)
	}
}

func TestHermeticAllowsCommitDespiteHostileGlobal(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}

	hostileDir := t.TempDir()
	hooks := filepath.Join(hostileDir, "hooks")
	if err := os.MkdirAll(hooks, 0o755); err != nil {
		t.Fatal(err)
	}
	// A pre-commit that always fails — would block commits if global hooksPath were honoured.
	hook := filepath.Join(hooks, "pre-commit")
	if err := os.WriteFile(hook, []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := filepath.Join(hostileDir, "gitconfig")
	content := "[commit]\n\tgpgsign = true\n[core]\n\thooksPath = " + hooks + "\n"
	if err := os.WriteFile(cfg, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_CONFIG_GLOBAL", cfg)
	t.Setenv("GIT_CONFIG_SYSTEM", os.DevNull)

	Hermetic(t)

	repo := t.TempDir()
	Run(t, repo, "init")
	Run(t, repo, "config", "user.email", "a@b.c")
	Run(t, repo, "config", "user.name", "pj-test")
	// Intentionally do not set commit.gpgsign=false: global true must not apply.
	if err := os.WriteFile(filepath.Join(repo, "f"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	Run(t, repo, "add", "f")
	Run(t, repo, "commit", "-m", "ok")
}

func TestCombinedAllowFailureReturnsOutputAndError(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	repo := t.TempDir()
	Run(t, repo, "init")
	// No commits: log fails, but soft path must not fatal the test.
	out, err := CombinedAllowFailure(t, repo, "log", "--format=%s")
	if err == nil {
		t.Fatal("expected non-zero exit on empty-repo log")
	}
	_ = out // stderr may explain the empty history; callers discard it on error
	// Success path still returns trimmed output.
	Run(t, repo, "config", "user.email", "a@b.c")
	Run(t, repo, "config", "user.name", "pj-test")
	if err := os.WriteFile(filepath.Join(repo, "f"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	Run(t, repo, "add", "f")
	Run(t, repo, "commit", "-m", "hello")
	out, err = CombinedAllowFailure(t, repo, "log", "--format=%s")
	if err != nil {
		t.Fatalf("log after commit: %v", err)
	}
	if out != "hello" {
		t.Errorf("log = %q want hello", out)
	}
}
