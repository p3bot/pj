package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A both-sides terminal status change produces status_conflict in the file (clean YAML,
// base status), pj sync refuses --continue while the key is present, and after the human
// resolves it in-file the next sync completes and pushes.
func TestSyncBothSidesTerminalStatusDispute(t *testing.T) {
	requireGit(t)
	a, b, remote := twoMachines(t)

	a.status(t, "wc-ab2c", "done")
	if _, _, err := a.sync(t, "--scope", "wc"); err != nil {
		t.Fatalf("A sync done: %v", err)
	}

	b.status(t, "wc-ab2c", "cancelled")
	_, errOut, err := b.sync(t, "--scope", "wc")
	if ExitCodeFromError(err) != exitFailure {
		t.Fatalf("both-sides terminal dispute must pause non-zero, got %v (stderr %q)", err, errOut)
	}
	if !strings.Contains(errOut, "status_conflict:") {
		t.Fatalf("dispute should ride status_conflict, got %q", errOut)
	}

	path, _ := findProject(t, b.scopeDir(), "wc-ab2c-alpha.md")
	if path == "" {
		t.Fatal("the disputed project file should still exist on B")
	}
	if !frontmatterHasStatusConflict(path) {
		t.Errorf("the file should carry status_conflict, content:\n%s", readFile(t, path))
	}
	if st := fmStatus(t, path); st != "todo" {
		t.Errorf("the merged file should carry the base status todo, got %q", st)
	}

	// A second sync while the key is present still refuses to continue.
	_, errOut, err = b.sync(t, "--scope", "wc")
	if ExitCodeFromError(err) != exitFailure {
		t.Fatalf("sync must keep refusing while status_conflict is present, got %v", err)
	}
	if !strings.Contains(errOut, "status_conflict") {
		t.Errorf("the persistent dispute should still be reported, got %q", errOut)
	}

	// The human resolves in-file: pick a status, delete status_conflict.
	resolveStatusConflict(t, path, "done")
	if _, _, err := b.sync(t, "--scope", "wc"); err != nil {
		t.Fatalf("post-resolution sync should complete: %v", err)
	}
	if !remoteHas(t, remote, "wc/archive/wc-ab2c-alpha.md") {
		t.Errorf("the resolved terminal project should land under archive/ on the remote")
	}
}

// resolveStatusConflict rewrites a project file's frontmatter to a single chosen status
// with the status_conflict key removed — the in-file resolution a human performs.
func resolveStatusConflict(t *testing.T, path, finalStatus string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, line := range strings.Split(string(data), "\n") {
		switch {
		case strings.HasPrefix(line, "status_conflict:"):
			continue
		case strings.HasPrefix(line, "status:"):
			out = append(out, "status: "+finalStatus)
		default:
			out = append(out, line)
		}
	}
	if err := os.WriteFile(path, []byte(strings.Join(out, "\n")), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// A pj.cue conflict pauses the rebase before that scope's conflicted project .md are
// field-merged (fail-closed); after the human resolves pj.cue in-file, the next sync
// field-merges those .md through the driver, stages them, and completes.
func TestSyncCueConflictPausesThenResumeMergesMD(t *testing.T) {
	requireGit(t)
	a, b, remote := twoMachines(t)

	dirA := a.scopeDir()
	writeCue(t, dirA, "name: \"wc\"\nautoCommit: true\nfields: {a: {type: \"string\"}}\n")
	setStatusLine(t, mustSeedProject(t, dirA), "in-progress")
	if _, _, err := a.sync(t, "--scope", "wc"); err != nil {
		t.Fatalf("A sync: %v", err)
	}

	dirB := b.scopeDir()
	writeCue(t, dirB, "name: \"wc\"\nautoCommit: true\nfields: {b: {type: \"string\"}}\n")
	pB := mustSeedProject(t, dirB)
	setStatusLine(t, pB, "review")
	_, errOut, err := b.sync(t, "--scope", "wc")
	if ExitCodeFromError(err) != exitFailure {
		t.Fatalf("a pj.cue conflict must pause non-zero, got %v (stderr %q)", err, errOut)
	}
	if !strings.Contains(errOut, "config_unparseable:") {
		t.Errorf("a conflicted pj.cue should ride config_unparseable, got %q", errOut)
	}
	if !frontmatterHasMarkers(pB) {
		t.Errorf("the project .md must be left unmerged (markers in frontmatter) while pj.cue is conflicted:\n%s", readFile(t, pB))
	}

	// The human resolves pj.cue in-file (both fields kept), then re-syncs.
	writeCue(t, dirB, "name: \"wc\"\nautoCommit: true\nfields: {a: {type: \"string\"}, b: {type: \"string\"}}\n")
	if _, _, err := b.sync(t, "--scope", "wc"); err != nil {
		t.Fatalf("post-resolution sync should field-merge the .md and complete: %v", err)
	}
	if frontmatterHasMarkers(pB) {
		t.Errorf("after resume the .md frontmatter must be field-merged clean:\n%s", readFile(t, pB))
	}
	if st := fmStatus(t, pB); st == "todo" {
		t.Errorf("the .md status should have merged past the base, got %q", st)
	}
	if n := gitIn(t, b.clone, "rev-list", "--count", "@{u}..HEAD"); n != "0" {
		t.Errorf("B should be fully pushed after the pj.cue resume, unpushed=%s", n)
	}
	_ = remote
}

// A conflicted .gitignore pauses on its own account but grants no schema authority: the
// scope's project .md are still field-merged at that same stop, and the scope is not put
// into the config_unparseable state on the wire when its pj.cue is perfectly readable.
// Folding .gitignore in with pj.cue would fail-close the .md and cost a second round trip.
func TestSyncGitignoreConflictDoesNotBlockProjectMerges(t *testing.T) {
	requireGit(t)
	a, b, _ := twoMachines(t)

	// Both files are edited in place and left dirty, so each machine's sync sweeps them into
	// one snapshot commit. That is what puts .gitignore and the project .md at the *same*
	// rebase stop — the only arrangement where the gating actually bites. Going through
	// pj status instead would self-commit the .md separately and split them across two
	// stops, where a .gitignore conflict could never have blocked the merge anyway.
	appendLine(t, filepath.Join(a.scopeDir(), ".gitignore"), "a-only/")
	setStatusLine(t, mustSeedProject(t, a.scopeDir()), "in-progress")
	if _, _, err := a.sync(t, "--scope", "wc"); err != nil {
		t.Fatalf("A sync: %v", err)
	}

	appendLine(t, filepath.Join(b.scopeDir(), ".gitignore"), "b-only/")
	setStatusLine(t, mustSeedProject(t, b.scopeDir()), "review")
	_, errOut, err := b.sync(t, "--scope", "wc")
	if ExitCodeFromError(err) != exitFailure {
		t.Fatalf("a conflicted .gitignore must pause non-zero, got %v (stderr %q)", err, errOut)
	}
	if !strings.Contains(errOut, ".gitignore") {
		t.Errorf("the conflicted .gitignore should be named, got %q", errOut)
	}
	if strings.Contains(errOut, "config_unparseable:") {
		t.Errorf("a .gitignore conflict is not a schema failure and must not ride config_unparseable, got %q", errOut)
	}

	pB := mustSeedProject(t, b.scopeDir())
	if frontmatterHasMarkers(pB) {
		t.Errorf("the project .md must still be field-merged — .gitignore gates nothing:\n%s", readFile(t, pB))
	}

	// Resolving the one genuinely conflicted file completes the stop in a single re-run.
	if err := os.WriteFile(filepath.Join(b.scopeDir(), ".gitignore"), []byte(".pj.lock\na-only/\nb-only/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := b.sync(t, "--scope", "wc"); err != nil {
		t.Fatalf("sync after resolving .gitignore should complete: %v", err)
	}
	if n := gitIn(t, b.clone, "rev-list", "--count", "@{u}..HEAD"); n != "0" {
		t.Errorf("B should be fully pushed after the .gitignore resolution, unpushed=%s", n)
	}
}

// appendLine adds a line at the end of a file — two machines appending different lines to
// the same file conflict on the trailing hunk.
func appendLine(t *testing.T, path, line string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(data, []byte(line+"\n")...), 0o644); err != nil {
		t.Fatal(err)
	}
}

// A same-id add/add arriving through the rebase ends with both files kept (one renamed),
// edge_verify emitted for inbound edges to the collided id in this invocation, and the
// rebase continued — never a field-merge, never a human handoff.
func TestSyncAddAddRenameEmitsEdgeVerify(t *testing.T) {
	requireGit(t)
	remote := newBareRemote(t)
	a := cloneMachine(t, remote)
	dirA := a.initScopeAutoCommit(t)
	// A referrer that depends on the id the two adds will collide on.
	addProject(t, dirA, "wc-zz99", "ref", "todo", "a0", "# Ref\n", false, "depends: [wc-ab2c]\n")
	if _, _, err := a.sync(t, "--scope", "wc"); err != nil {
		t.Fatalf("A seed sync: %v", err)
	}
	b := cloneMachine(t, remote)
	dirB := b.importScope(t)

	// Both machines create a project with the same id AND slug (so the paths coincide) but
	// different content — a genuine add/add collision, not two edits to one project.
	addProject(t, dirA, "wc-ab2c", "alpha", "todo", "a1", "# A body\n", false, "")
	if _, _, err := a.sync(t, "--scope", "wc"); err != nil {
		t.Fatalf("A add sync: %v", err)
	}
	addProject(t, dirB, "wc-ab2c", "alpha", "todo", "a2", "# B body\n", false, "")
	out, errOut, err := b.sync(t, "--scope", "wc")
	if err != nil {
		t.Fatalf("add/add rename should auto-resolve and complete: %v (stderr %q)", err, errOut)
	}
	if !strings.Contains(out, "repaired add/add") {
		t.Errorf("the rename repair should be reported, got %q", out)
	}
	if !strings.Contains(out+errOut, "edge_verify:") || !strings.Contains(out+errOut, "wc-zz99") {
		t.Errorf("edge_verify should surface the inbound edge from wc-zz99, got %q / %q", out, errOut)
	}
	// Both files survive on the remote: the kept id plus a renamed loser.
	names := gitIn(t, remote, "ls-tree", "-r", "--name-only", "main")
	n := strings.Count(names, "wc/wc-ab2c")
	if n < 2 {
		t.Errorf("both collided files should be kept (one renamed), got tree:\n%s", names)
	}
	_ = dirB
}

// The sync integrity step repairs a duplicate id that arrives through the rebase and was
// absent before the fetch — over the merged tree, with no separate pj doctor run.
func TestSyncIntegrityRepairsDuplicateFromRebase(t *testing.T) {
	requireGit(t)
	remote := newBareRemote(t)
	a := cloneMachine(t, remote)
	dirA := a.initScopeAutoCommit(t)
	if _, _, err := a.sync(t, "--scope", "wc"); err != nil {
		t.Fatalf("A seed sync: %v", err)
	}
	b := cloneMachine(t, remote)
	dirB := b.importScope(t)

	// Same id, different slug → different paths, so they land as two files (no path
	// conflict) and the duplicate only exists after the merge.
	addProject(t, dirA, "wc-cd3e", "beta", "todo", "a1", "# Beta\n", false, "")
	if _, _, err := a.sync(t, "--scope", "wc"); err != nil {
		t.Fatalf("A add sync: %v", err)
	}
	addProject(t, dirB, "wc-cd3e", "gamma", "todo", "a2", "# Gamma\n", false, "")
	out, errOut, err := b.sync(t, "--scope", "wc")
	if err != nil {
		t.Fatalf("integrity repair sync should complete: %v (stderr %q)", err, errOut)
	}
	if !strings.Contains(out, "repaired duplicate id") {
		t.Errorf("the sync integrity step should repair the duplicate id, got %q", out)
	}
	if n := gitIn(t, b.clone, "rev-list", "--count", "@{u}..HEAD"); n != "0" {
		t.Errorf("B should be fully pushed after the integrity repair, unpushed=%s", n)
	}
	_ = dirB
}

// pj sync --all visits every git-root: a sync_disabled root is reported and the run exits
// non-zero, but a healthy root still syncs and pushes — one stale repo strands no other.
func TestSyncAllIsolatesFailingRoot(t *testing.T) {
	requireGit(t)
	app := newApp(t)

	// A healthy auto-commit root with an upstream.
	remote := newBareRemote(t)
	base := t.TempDir()
	wcClone := filepath.Join(base, "clone")
	gitIn(t, base, "clone", remote, "clone")
	configIdentity(t, wcClone)
	wcDir := filepath.Join(wcClone, "wc")
	if err := os.MkdirAll(wcDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, _, err := run(t, app, "scope", "init", wcDir, "--name", "wc", "--auto-commit"); err != nil {
		t.Fatalf("init wc: %v", err)
	}
	addProject(t, wcDir, "wc-ab2c", "alpha", "todo", "a0", "# Alpha\n", false, "")

	// A second auto-commit root with NO upstream: sync_disabled.
	xyRepo := t.TempDir()
	gitIn(t, xyRepo, "init", "-b", "main")
	configIdentity(t, xyRepo)
	xyDir := filepath.Join(xyRepo, "xy")
	if err := os.MkdirAll(xyDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, _, err := run(t, app, "scope", "init", xyDir, "--name", "xy", "--auto-commit"); err != nil {
		t.Fatalf("init xy: %v", err)
	}
	addProject(t, xyDir, "xy-ab2c", "beta", "todo", "a0", "# Beta\n", false, "")

	_, errOut, err := run(t, app, "sync", "--all")
	if ExitCodeFromError(err) != exitFailure {
		t.Fatalf("--all with a failing root must exit non-zero, got %v", err)
	}
	if !strings.Contains(errOut, "sync_disabled:") || !strings.Contains(errOut, "xy") {
		t.Errorf("the failing xy root should ride sync_disabled, got %q", errOut)
	}
	if !remoteHas(t, remote, "wc/wc-ab2c-alpha.md") {
		t.Errorf("the healthy wc root must still sync and push despite xy failing")
	}
}
