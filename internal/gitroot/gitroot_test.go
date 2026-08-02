package gitroot

import (
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/start-cli/pj/internal/pathutil"
	"github.com/start-cli/pj/internal/testgit"
)

func TestRepoRootInsideRepo(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	dir := t.TempDir()
	testgit.Run(t, dir, "init")
	sub := filepath.Join(dir, "a", "b")
	if err := exec.Command("mkdir", "-p", sub).Run(); err != nil {
		t.Fatal(err)
	}

	root, ok := RepoRoot(sub)
	if !ok {
		t.Fatal("expected a git-root")
	}
	// macOS /var → /private/var and similar: RepoRoot is already canonical.
	if root != pathutil.Canonical(dir) {
		t.Errorf("RepoRoot=%q want %q", root, pathutil.Canonical(dir))
	}
}

func TestRepoRootOutsideRepo(t *testing.T) {
	dir := t.TempDir()
	if root, ok := RepoRoot(dir); ok {
		t.Errorf("expected no git-root outside a repo, got %q", root)
	}
}

func TestRepoRootMissingDir(t *testing.T) {
	if root, ok := RepoRoot(filepath.Join(t.TempDir(), "does-not-exist")); ok {
		t.Errorf("expected no git-root for a missing dir, got %q", root)
	}
}

func TestRepoRootForNewMissingDescendant(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	repo := t.TempDir()
	testgit.Run(t, repo, "init")
	missing := filepath.Join(repo, "a", "b", "c")
	root, ok := RepoRootForNew(missing)
	if !ok {
		t.Fatal("expected a git-root for a missing descendant of a repo")
	}
	if root != pathutil.Canonical(repo) {
		t.Errorf("RepoRootForNew=%q want %q", root, pathutil.Canonical(repo))
	}
}

func TestRepoRootForNewOutsideRepo(t *testing.T) {
	if root, ok := RepoRootForNew(filepath.Join(t.TempDir(), "x", "y")); ok {
		t.Errorf("expected no git-root, got %q", root)
	}
}
