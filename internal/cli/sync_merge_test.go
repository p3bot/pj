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

	a.mark(t, "wc-ab2c", "done")
	if _, _, err := a.sync(t, "--scope", "wc"); err != nil {
		t.Fatalf("A sync done: %v", err)
	}

	b.mark(t, "wc-ab2c", "cancelled")
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
	// pj mark instead would self-commit the .md separately and split them across two
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

// A hand-deleted .gitignore meeting a concurrent edit pauses with the delete/edit line
// (not the markers line) on the first pause and every unactioned re-run; removing the file
// then completes the rebase with the deletion recorded.
func TestSyncGitignoreDeleteEditUnactionedThenRemove(t *testing.T) {
	requireGit(t)
	a, b, remote := twoMachines(t)

	if err := os.Remove(filepath.Join(a.scopeDir(), ".gitignore")); err != nil {
		t.Fatal(err)
	}
	if _, _, err := a.sync(t, "--scope", "wc"); err != nil {
		t.Fatalf("A .gitignore delete sync: %v", err)
	}

	appendLine(t, filepath.Join(b.scopeDir(), ".gitignore"), "b-extra/")
	_, firstOut, err := b.sync(t, "--scope", "wc")
	if ExitCodeFromError(err) != exitFailure {
		t.Fatalf(".gitignore delete/edit must pause non-zero, got %v (stderr %q)", err, firstOut)
	}
	assertConfigDeleteEditHandoff(t, firstOut, "wc/.gitignore", true)
	if strings.Contains(firstOut, "resolve the conflict markers") {
		t.Errorf("first pause must not use the markers line for a delete/edit, got %q", firstOut)
	}

	_, secondOut, err := b.sync(t, "--scope", "wc")
	if ExitCodeFromError(err) != exitFailure {
		t.Fatalf("unactioned .gitignore re-run must stay paused, got %v (stderr %q)", err, secondOut)
	}
	assertConfigDeleteEditHandoff(t, secondOut, "wc/.gitignore", true)
	assertSameDeleteEditLine(t, firstOut, secondOut)
	if remoteHas(t, remote, "wc/.gitignore") {
		t.Errorf("unactioned re-run must not resurrect .gitignore on the remote")
	}

	// Human removes the survivor → deletion wins.
	if err := os.Remove(filepath.Join(b.scopeDir(), ".gitignore")); err != nil {
		t.Fatal(err)
	}
	if _, _, err := b.sync(t, "--scope", "wc"); err != nil {
		t.Fatalf("remove resolution should complete: %v", err)
	}
	if remoteHas(t, remote, "wc/.gitignore") {
		t.Errorf("deletion should be recorded on the remote")
	}
}

// A .gitignore delete/edit the human modifies stages as their resolution and completes.
func TestSyncGitignoreDeleteEditModifiedResumes(t *testing.T) {
	requireGit(t)
	a, b, remote := twoMachines(t)

	if err := os.Remove(filepath.Join(a.scopeDir(), ".gitignore")); err != nil {
		t.Fatal(err)
	}
	if _, _, err := a.sync(t, "--scope", "wc"); err != nil {
		t.Fatalf("A .gitignore delete sync: %v", err)
	}

	appendLine(t, filepath.Join(b.scopeDir(), ".gitignore"), "b-extra/")
	if _, _, err := b.sync(t, "--scope", "wc"); ExitCodeFromError(err) != exitFailure {
		t.Fatalf("expected .gitignore delete/edit pause, got %v", err)
	}

	resolved := ".pj.lock\nb-extra/\nhuman-kept/\n"
	if err := os.WriteFile(filepath.Join(b.scopeDir(), ".gitignore"), []byte(resolved), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := b.sync(t, "--scope", "wc"); err != nil {
		t.Fatalf("modified .gitignore re-run should complete: %v", err)
	}
	if !remoteHas(t, remote, "wc/.gitignore") {
		t.Errorf("the human's modified .gitignore should land on the remote")
	}
	if got := readFile(t, filepath.Join(b.scopeDir(), ".gitignore")); got != resolved {
		t.Errorf("staged content = %q, want %q", got, resolved)
	}
}

// A .gitignore delete/edit resolved with git add never reaches the resumed stop.
func TestSyncGitignoreDeleteEditGitAddResumes(t *testing.T) {
	requireGit(t)
	a, b, remote := twoMachines(t)

	if err := os.Remove(filepath.Join(a.scopeDir(), ".gitignore")); err != nil {
		t.Fatal(err)
	}
	if _, _, err := a.sync(t, "--scope", "wc"); err != nil {
		t.Fatalf("A .gitignore delete sync: %v", err)
	}

	appendLine(t, filepath.Join(b.scopeDir(), ".gitignore"), "b-extra/")
	if _, _, err := b.sync(t, "--scope", "wc"); ExitCodeFromError(err) != exitFailure {
		t.Fatalf("expected .gitignore delete/edit pause, got %v", err)
	}

	before := readFile(t, filepath.Join(b.scopeDir(), ".gitignore"))
	gitIn(t, b.clone, "add", "--", "wc/.gitignore")
	if _, _, err := b.sync(t, "--scope", "wc"); err != nil {
		t.Fatalf("git-add .gitignore resolution should complete: %v", err)
	}
	if !remoteHas(t, remote, "wc/.gitignore") {
		t.Errorf("the git-add-ed .gitignore should land on the remote")
	}
	if got := readFile(t, filepath.Join(b.scopeDir(), ".gitignore")); got != before {
		t.Errorf("git add keeps exact content; worktree drifted")
	}
}

// An unactioned pj.cue delete/edit at a stop that also has an unmerged project .md under
// that scope keeps the project file unmerged on the next pj sync (schema fail-closed) —
// it must not field-merge under the worktree survivor.
func TestSyncCueDeleteEditKeepsProjectFailClosed(t *testing.T) {
	requireGit(t)
	a, b, _ := twoMachines(t)

	// A's hand commit deletes pj.cue and edits the project (pj sync would refuse a
	// root whose schema will not evaluate, so the deletion arrives only by hand commit).
	dirA := a.scopeDir()
	if err := os.Remove(filepath.Join(dirA, "pj.cue")); err != nil {
		t.Fatal(err)
	}
	setStatusLine(t, mustSeedProject(t, dirA), "in-progress")
	gitIn(t, a.clone, "add", "-A")
	gitIn(t, a.clone, "commit", "-m", "hand: delete pj.cue and edit project")
	gitIn(t, a.clone, "push")

	// B modifies pj.cue (so the delete meets an edit) and the project differently —
	// both conflict at the same stop when B syncs.
	dirB := b.scopeDir()
	writeCue(t, dirB, "name: \"wc\"\nautoCommit: true\nfields: {b: {type: \"string\"}}\n")
	pB := mustSeedProject(t, dirB)
	setStatusLine(t, pB, "review")
	_, firstOut, err := b.sync(t, "--scope", "wc")
	if ExitCodeFromError(err) != exitFailure {
		t.Fatalf("pj.cue delete/edit must pause non-zero, got %v (stderr %q)", err, firstOut)
	}
	assertConfigDeleteEditHandoff(t, firstOut, "wc/pj.cue", false)
	if !strings.Contains(firstOut, "config_unparseable:") {
		t.Errorf("pj.cue delete/edit should ride config_unparseable, got %q", firstOut)
	}
	if !frontmatterHasMarkers(pB) {
		t.Errorf("project .md must stay unmerged while pj.cue is an open delete/edit:\n%s", readFile(t, pB))
	}

	// Unactioned re-run: still fail-closed on the project — never field-merge under the
	// worktree survivor while the schema deletion stands open.
	_, secondOut, err := b.sync(t, "--scope", "wc")
	if ExitCodeFromError(err) != exitFailure {
		t.Fatalf("unactioned pj.cue re-run must stay paused, got %v (stderr %q)", err, secondOut)
	}
	assertConfigDeleteEditHandoff(t, secondOut, "wc/pj.cue", false)
	assertSameDeleteEditLine(t, firstOut, secondOut)
	if !strings.Contains(secondOut, "not merged") || !strings.Contains(secondOut, "pj.cue is conflicted") {
		t.Errorf("project pass must report schema fail-closed, got %q", secondOut)
	}
	if !frontmatterHasMarkers(pB) {
		t.Errorf("project .md must still carry markers after unactioned re-run:\n%s", readFile(t, pB))
	}
}

// assertConfigDeleteEditHandoff checks a config-file delete/edit line: path, reader-term
// side naming, and the ways out (three for .gitignore; edit/git-add only for pj.cue).
// Way-out checks use path-qualified or "edit it" phrases so narrative "edited it" cannot
// satisfy them alone.
func assertConfigDeleteEditHandoff(t *testing.T, errOut, path string, threeWays bool) {
	t.Helper()
	if !strings.Contains(errOut, "delete/edit") {
		t.Errorf("expected delete/edit handoff, got %q", errOut)
	}
	if !strings.Contains(errOut, path) {
		t.Errorf("expected path %q in handoff, got %q", path, errOut)
	}
	if strings.Contains(errOut, "resolve the conflict markers") {
		t.Errorf("delete/edit must not use the markers line, got %q", errOut)
	}
	if !strings.Contains(errOut, "incoming side") && !strings.Contains(errOut, "this machine's replayed commit") {
		t.Errorf("handoff must name the deleting side in reader terms, got %q", errOut)
	}
	if !strings.Contains(errOut, "git add") {
		t.Errorf("handoff must name git add as a way out, got %q", errOut)
	}
	if threeWays {
		for _, way := range []string{"remove " + path, "edit it"} {
			if !strings.Contains(errOut, way) {
				t.Errorf(".gitignore handoff must name %q as a way out, got %q", way, errOut)
			}
		}
		return
	}
	// pj.cue: edit <path> or git add — never offer remove as a resolution.
	if !strings.Contains(errOut, "edit "+path) {
		t.Errorf("pj.cue handoff must name edit %q as a way out, got %q", path, errOut)
	}
	if strings.Contains(errOut, "remove "+path) || strings.Contains(errOut, "— remove") {
		t.Errorf("pj.cue handoff must not offer removal as a way out, got %q", errOut)
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
