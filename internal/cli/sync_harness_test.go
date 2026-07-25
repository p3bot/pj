package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"cuelang.org/go/cue/cuecontext"
)

// gitIn runs a git command with cmd.Dir=dir and returns trimmed stdout, failing on error.
func gitIn(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s (in %s): %v\n%s", strings.Join(args, " "), dir, err, out)
	}
	return strings.TrimSpace(string(out))
}

func configIdentity(t *testing.T, dir string) {
	t.Helper()
	gitIn(t, dir, "config", "user.email", "a@b.c")
	gitIn(t, dir, "config", "user.name", "pj-test")
	gitIn(t, dir, "config", "commit.gpgsign", "false")
}

// newBareRemote creates a bare remote seeded with one commit on a known branch, so every
// clone gets a tracking branch (an upstream) from the start.
func newBareRemote(t *testing.T) string {
	t.Helper()
	base := t.TempDir()
	remote := filepath.Join(base, "remote.git")
	gitIn(t, base, "init", "--bare", "-b", "main", "remote.git")
	seed := filepath.Join(base, "seed")
	gitIn(t, base, "clone", remote, "seed")
	configIdentity(t, seed)
	if err := os.WriteFile(filepath.Join(seed, ".keep"), []byte("seed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitIn(t, seed, "checkout", "-B", "main")
	gitIn(t, seed, "add", "-A")
	gitIn(t, seed, "commit", "-m", "seed")
	gitIn(t, seed, "push", "-u", "origin", "main")
	gitIn(t, remote, "symbolic-ref", "HEAD", "refs/heads/main")
	return remote
}

// machine is one clone of a shared remote with its own machine-local pj state — a
// separate App (config + state dir), the model of a second computer.
type machine struct {
	app   *App
	clone string
}

// cloneMachine clones remote into a fresh working tree with its own pj App.
func cloneMachine(t *testing.T, remote string) *machine {
	t.Helper()
	base := t.TempDir()
	clone := filepath.Join(base, "clone")
	gitIn(t, base, "clone", remote, "clone")
	configIdentity(t, clone)
	app := &App{Ctx: cuecontext.New(), ConfigDir: t.TempDir(), StateDir: t.TempDir()}
	return &machine{app: app, clone: clone}
}

// scopeDir is the on-disk dir the harness scope (wc) occupies in this clone.
func (m *machine) scopeDir() string { return filepath.Join(m.clone, "wc") }

// initScopeAutoCommit registers a fresh auto-commit scope named wc at <clone>/wc. The
// sync harness is single-scope by construction; a shared-repo preflight case that needs a
// divergent sibling builds it directly rather than through this helper.
func (m *machine) initScopeAutoCommit(t *testing.T) string {
	t.Helper()
	dir := m.scopeDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, _, err := run(t, m.app, "scope", "init", dir, "--name", "wc", "--auto-commit"); err != nil {
		t.Fatalf("init auto-commit scope wc: %v", err)
	}
	return dir
}

// importScope registers the already-on-disk wc scope (after a clone brought its files).
func (m *machine) importScope(t *testing.T) string {
	t.Helper()
	dir := m.scopeDir()
	if _, _, err := run(t, m.app, "scope", "import", dir); err != nil {
		t.Fatalf("import scope wc: %v", err)
	}
	return dir
}

// sync runs pj sync on this machine and returns stdout, stderr, and the handler error.
func (m *machine) sync(t *testing.T, args ...string) (string, string, error) {
	t.Helper()
	return run(t, m.app, append([]string{"sync"}, args...)...)
}

// status runs pj status <id> <newStatus> on this machine (a local self-commit), the
// model of an offline edit the next sync will push and the other machine will merge.
func (m *machine) status(t *testing.T, id, newStatus string) {
	t.Helper()
	if _, _, err := run(t, m.app, "status", id, newStatus); err != nil {
		t.Fatalf("status %s %s: %v", id, newStatus, err)
	}
}

// fmStatus reads the status field from a project file's frontmatter.
func fmStatus(t *testing.T, path string) string {
	t.Helper()
	return fmValue(t, path, "status")
}

// findProject locates a project's current on-disk path in a scope dir, root or archive/.
func findProject(t *testing.T, dir, base string) (string, bool) {
	t.Helper()
	if p := filepath.Join(dir, base); fileExistsPath(p) {
		return p, false
	}
	if p := filepath.Join(dir, "archive", base); fileExistsPath(p) {
		return p, true
	}
	return "", false
}

// mustSeedProject returns the current path of the baseline project twoMachines seeds into
// both clones, wherever the run has since moved it (root or archive/), failing if it is
// gone. Every caller wants that one project, so it names it rather than taking a basename
// no caller varies.
func mustSeedProject(t *testing.T, dir string) string {
	t.Helper()
	p, _ := findProject(t, dir, "wc-ab2c-alpha.md")
	if p == "" {
		t.Fatalf("seed project wc-ab2c-alpha.md not found under %s", dir)
	}
	return p
}

func fileExistsPath(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// setStatusLine rewrites the status: line of a project file in place — a direct edit that
// the next sync snapshots (used to force a merge without the terminal-boundary move).
func setStatusLine(t *testing.T, path, status string) {
	t.Helper()
	replaceLinePrefix(t, path, "status:", "status: "+status)
}

// editBody rewrites the seed project body line to newText (a direct body edit that the
// next sync snapshots). The seed bodies all carry the "body line" marker.
func editBody(t *testing.T, path, newText string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	s := strings.Replace(string(data), "body line", newText, 1)
	if err := os.WriteFile(path, []byte(s), 0o644); err != nil {
		t.Fatal(err)
	}
}

func replaceLinePrefix(t *testing.T, path, prefix, replacement string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, prefix) {
			out = append(out, replacement)
		} else {
			out = append(out, line)
		}
	}
	if err := os.WriteFile(path, []byte(strings.Join(out, "\n")), 0o644); err != nil {
		t.Fatal(err)
	}
}

// writeCue overwrites a scope's pj.cue with the given content (a direct config edit).
func writeCue(t *testing.T, dir, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "pj.cue"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// topCommit returns the subject of the clone's HEAD commit.
func topCommit(t *testing.T, clone string) string {
	t.Helper()
	return gitIn(t, clone, "log", "-1", "--format=%s")
}

// twoMachines sets up a shared remote with an auto-commit scope registered and pushed by
// machine A, then a machine B clone that has imported the same scope. Both carry the same
// baseline project, committed and pushed, so either can edit it offline.
func twoMachines(t *testing.T) (a, b *machine, remote string) {
	t.Helper()
	remote = newBareRemote(t)
	a = cloneMachine(t, remote)
	dirA := a.initScopeAutoCommit(t)
	addProject(t, dirA, "wc-ab2c", "alpha", "todo", "a0", "# alpha\n\nbody line\n", false, "")
	if _, _, err := a.sync(t, "--scope", "wc"); err != nil {
		t.Fatalf("machine A initial sync: %v", err)
	}
	b = cloneMachine(t, remote)
	b.importScope(t)
	return a, b, remote
}

// remoteHas reports whether the remote's main branch tree contains a path.
func remoteHas(t *testing.T, remote, repoRelPath string) bool {
	t.Helper()
	out := gitIn(t, remote, "ls-tree", "-r", "--name-only", "main")
	for _, l := range lines(out) {
		if l == repoRelPath {
			return true
		}
	}
	return false
}
