package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/start-cli/pj/internal/gitstate"
)

// An empty auto-commit eligible set is not a failure: exit 0 with a note.
func TestSyncEmptyEligibleSetExitsZero(t *testing.T) {
	app := newApp(t)
	out, errOut, err := run(t, app, "sync", "--all")
	if err != nil {
		t.Fatalf("empty --all must exit 0, got %v", err)
	}
	if !strings.Contains(errOut+out, "nothing to sync") {
		t.Errorf("empty set should note nothing to sync, got %q / %q", out, errOut)
	}
}

// When every registered auto-commit scope is unreachable, --all still surfaces the
// unreachable_scope: note (an unmounted drive is not silently swallowed) rather than a
// bare, misleading "no auto-commit git-roots registered". Unreachable is a non-fatal
// skip, so the run still exits 0.
func TestSyncAllUnreachableStillReportsAndExitsZero(t *testing.T) {
	app := newApp(t)
	dir := filepath.Join(t.TempDir(), "wc")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, _, err := run(t, app, "scope", "init", dir, "--name", "wc", "--auto-commit"); err != nil {
		t.Fatalf("init auto-commit scope: %v", err)
	}
	// Make the dir unreachable after registration (the unmounted-drive model).
	if err := os.RemoveAll(dir); err != nil {
		t.Fatal(err)
	}

	out, errOut, err := run(t, app, "sync", "--all")
	if err != nil {
		t.Fatalf("all-unreachable --all must still exit 0, got %v", err)
	}
	if !strings.Contains(errOut, "unreachable_scope:") || !strings.Contains(errOut, "wc") {
		t.Errorf("the unreachable scope must be reported, not swallowed, got %q", errOut)
	}
	if strings.Contains(errOut+out, "no auto-commit git-roots registered") {
		t.Errorf("the message must not claim nothing is registered when a scope is merely unreachable, got %q / %q", out, errOut)
	}
}

// A push that does not land records last-push-error under the git-root ops state, and the
// next successful push clears it — the marker pj doctor and every complete-state write verb
// read to warn that work is stranded on this machine (requirement 6). A pre-push hook that
// rejects is the stand-in for a remote that refuses; removing it is the operator's fix.
//
// This covers the second-push exit only. The two race-retry exits (the re-integrate pausing
// or erroring after a rejected first push) share recordPushFailure but need a genuine
// fetch→push race to reach, which a hook cannot stage on its own.
func TestSyncPushFailureRecordsAndClearsLastPushError(t *testing.T) {
	requireGit(t)
	remote := newBareRemote(t)
	m := cloneMachine(t, remote)
	dir := m.initScopeAutoCommit(t)
	addProject(t, dir, "wc-ab2c", "alpha", "todo", "a0", "# Alpha\n", false, "")

	hook := filepath.Join(m.clone, ".git", "hooks", "pre-push")
	if err := os.WriteFile(hook, []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	_, errOut, err := m.sync(t, "--scope", "wc")
	if ExitCodeFromError(err) != exitFailure {
		t.Fatalf("a rejected push must exit non-zero, got %v (stderr %q)", err, errOut)
	}
	if !strings.Contains(errOut, "last_push_error:") {
		t.Errorf("a push that did not land should ride last_push_error, got %q", errOut)
	}
	if _, present := gitstate.ReadLastPushError(m.app.StateDir, m.clone); !present {
		t.Fatal("the failed push must be recorded, so doctor and the write verbs can warn about stranded work")
	}

	// The operator fixes the remote; the next successful push clears the record.
	if err := os.Remove(hook); err != nil {
		t.Fatal(err)
	}
	if _, _, err := m.sync(t, "--scope", "wc"); err != nil {
		t.Fatalf("sync after the remote is fixed should push: %v", err)
	}
	if detail, present := gitstate.ReadLastPushError(m.app.StateDir, m.clone); present {
		t.Errorf("a successful push must clear the record, still present: %q", detail)
	}
}

// A lone auto-commit scope whose pj.cue will not parse is surfaced under --all with
// config_unparseable and exits non-zero, not silently dropped. With no healthy sibling in
// its repo, no per-root preflight ever reaches it, so allSelection must surface it
// directly rather than skip it and misreport the fleet as "nothing to sync".
func TestSyncAllLoneUnparseableConfigSurfaces(t *testing.T) {
	app := newApp(t)
	dir := filepath.Join(t.TempDir(), "wc")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, _, err := run(t, app, "scope", "init", dir, "--name", "wc", "--auto-commit"); err != nil {
		t.Fatalf("init auto-commit scope: %v", err)
	}
	// Break pj.cue after registration: a field type the schema rejects, so it will not parse.
	if err := os.WriteFile(filepath.Join(dir, "pj.cue"), []byte("name: \"wc\"\nautoCommit: true\nfields: {x: {type: \"float\"}}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	out, errOut, err := run(t, app, "sync", "--all")
	if ExitCodeFromError(err) != exitFailure {
		t.Fatalf("a lone unparseable scope under --all must exit non-zero, got %v", err)
	}
	if !strings.Contains(errOut, "config_unparseable:") || !strings.Contains(errOut, "wc") {
		t.Errorf("the unparseable scope must ride config_unparseable naming it, got %q", errOut)
	}
	if strings.Contains(errOut+out, "nothing to sync") {
		t.Errorf("must not misreport as nothing to sync when a scope is merely misconfigured, got %q / %q", out, errOut)
	}
}

// An ambient scope whose pj.cue will not parse is refused with config_unparseable even when
// it has no git-root, not misreported as a missing repository. With a git-root the per-root
// preflight covers this; with none there is no preflight to run, so the ambient path must
// refuse on its own — otherwise the same scope is diagnosed as config_unparseable under
// --all and as sync_disabled under --scope.
func TestSyncAmbientUnparseableConfigNoRepoRefusesOnConfig(t *testing.T) {
	app := newApp(t)
	dir := filepath.Join(t.TempDir(), "wc")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, _, err := run(t, app, "scope", "init", dir, "--name", "wc", "--auto-commit"); err != nil {
		t.Fatalf("init auto-commit scope: %v", err)
	}
	// Break pj.cue after registration: a field type the schema rejects.
	if err := os.WriteFile(filepath.Join(dir, "pj.cue"), []byte("name: \"wc\"\nautoCommit: true\nfields: {x: {type: \"float\"}}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, errOut, err := run(t, app, "sync", "--scope", "wc")
	if ExitCodeFromError(err) != exitFailure {
		t.Fatalf("an unparseable ambient config must refuse non-zero, got %v", err)
	}
	all := err.Error() + errOut
	if !strings.Contains(all, "config_unparseable:") {
		t.Errorf("the broken pj.cue should be named by config_unparseable, got %v / %q", err, errOut)
	}
	if strings.Contains(all, "sync_disabled:") {
		t.Errorf("a broken pj.cue must not be reported as a missing git repository, got %v / %q", err, errOut)
	}
}

// A non-auto-commit ambient scope is refused with the mode-named error, non-zero.
func TestSyncNonAutoCommitAmbientRefuses(t *testing.T) {
	requireGit(t)
	app := newApp(t)
	initGitScope(t, app, "rd", false) // repo-driven
	_, errOut, err := run(t, app, "sync", "--scope", "rd")
	if ExitCodeFromError(err) != exitFailure {
		t.Fatalf("non-auto-commit ambient sync must refuse non-zero, got %v", err)
	}
	if !strings.Contains(err.Error()+errOut, "repo-driven") {
		t.Errorf("refuse should name the repo-driven mode, got %v / %q", err, errOut)
	}
}

// A plain-files ambient scope names the other mode.
func TestSyncPlainFilesAmbientRefuses(t *testing.T) {
	app := newApp(t)
	dir := filepath.Join(t.TempDir(), "pf")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, _, err := run(t, app, "scope", "init", dir, "--name", "pf"); err != nil {
		t.Fatalf("init plain scope: %v", err)
	}
	_, _, err := run(t, app, "sync", "--scope", "pf")
	if ExitCodeFromError(err) != exitFailure {
		t.Fatalf("plain-files ambient sync must refuse non-zero, got %v", err)
	}
	if !strings.Contains(err.Error(), "plain-files") {
		t.Errorf("refuse should name the plain-files mode, got %v", err)
	}
}

// --all wins over an ambient selector: a non-auto-commit PJ_SCOPE under --all does not
// trigger the mode refuse; with no auto-commit scopes it exits 0.
func TestSyncAllWinsOverAmbientSelector(t *testing.T) {
	requireGit(t)
	app := newApp(t)
	initGitScope(t, app, "rd", false)
	t.Setenv("PJ_SCOPE", "rd")
	_, errOut, err := run(t, app, "sync", "--all")
	if err != nil {
		t.Fatalf("--all with a non-auto-commit ambient must not refuse: %v", err)
	}
	if !strings.Contains(errOut, "nothing to sync") {
		t.Errorf("--all over only non-auto-commit scopes exits 0 with a note, got %q", errOut)
	}
}

// An auto-commit scope with a git-root but no upstream reports sync_disabled, non-zero.
func TestSyncAutoCommitNoUpstreamRidesSyncDisabled(t *testing.T) {
	requireGit(t)
	app := newApp(t)
	initGitScope(t, app, "wc", true) // a repo, but no remote
	_, errOut, err := run(t, app, "sync", "--scope", "wc")
	if ExitCodeFromError(err) != exitFailure {
		t.Fatalf("no-upstream sync must be non-zero, got %v", err)
	}
	if !strings.Contains(errOut, "sync_disabled:") {
		t.Errorf("no upstream should ride sync_disabled, got %q", errOut)
	}
}

// A planned auto-commit scope with no git-root at all rides sync_disabled, non-zero.
func TestSyncPlannedNoGitRootRidesSyncDisabled(t *testing.T) {
	app := newApp(t)
	dir := filepath.Join(t.TempDir(), "wc")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, _, err := run(t, app, "scope", "init", dir, "--name", "wc", "--auto-commit"); err != nil {
		t.Fatalf("init planned auto-commit: %v", err)
	}
	_, errOut, err := run(t, app, "sync", "--scope", "wc")
	if ExitCodeFromError(err) != exitFailure {
		t.Fatalf("planned no-repo sync must be non-zero, got %v", err)
	}
	if !strings.Contains(errOut, "sync_disabled:") {
		t.Errorf("planned no-repo sync should ride sync_disabled, got %q", errOut)
	}
}

// Preflight refuses the whole git-root when siblings disagree on autoCommit.
func TestSyncPreflightRefusesAutoCommitMismatch(t *testing.T) {
	requireGit(t)
	app := newApp(t)
	dir, repo := initGitScope(t, app, "wc", true)
	addProject(t, dir, "wc-ab2c", "x", "todo", "a0", "# X\n", false, "")
	sib := filepath.Join(repo, "sib")
	if err := os.MkdirAll(sib, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, _, err := run(t, app, "scope", "init", sib, "--name", "sib", "--code-root", sib, "--auto-commit"); err != nil {
		t.Fatalf("init sibling: %v", err)
	}
	// Diverge autoCommit after registration — exactly what init would have refused.
	if err := os.WriteFile(filepath.Join(sib, "pj.cue"), []byte("name: \"sib\"\nautoCommit: false\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, errOut, err := run(t, app, "sync", "--scope", "wc")
	if ExitCodeFromError(err) != exitFailure {
		t.Fatalf("mismatch preflight must refuse non-zero, got %v", err)
	}
	if !strings.Contains(errOut, "auto_commit_mismatch:") {
		t.Errorf("mismatch should ride auto_commit_mismatch, got %q", errOut)
	}
}

// Preflight refuses the whole git-root for an unparseable sibling pj.cue.
func TestSyncPreflightRefusesUnparseableSibling(t *testing.T) {
	requireGit(t)
	app := newApp(t)
	dir, repo := initGitScope(t, app, "wc", true)
	addProject(t, dir, "wc-ab2c", "x", "todo", "a0", "# X\n", false, "")
	sib := filepath.Join(repo, "sib")
	if err := os.MkdirAll(sib, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, _, err := run(t, app, "scope", "init", sib, "--name", "sib", "--code-root", sib, "--auto-commit"); err != nil {
		t.Fatalf("init sibling: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sib, "pj.cue"), []byte("name: \"sib\"\nautoCommit: true\nfields: {x: {type: \"float\"}}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, errOut, err := run(t, app, "sync", "--scope", "wc")
	if ExitCodeFromError(err) != exitFailure {
		t.Fatalf("unparseable sibling preflight must refuse non-zero, got %v", err)
	}
	if !strings.Contains(errOut, "config_unparseable:") || !strings.Contains(errOut, "sib") {
		t.Errorf("unparseable sibling should ride config_unparseable naming sib, got %q", errOut)
	}
}

// Preflight refuses the whole git-root for a name-drifted sibling, naming the recovery.
func TestSyncPreflightRefusesNameDriftedSibling(t *testing.T) {
	requireGit(t)
	app := newApp(t)
	dir, repo := initGitScope(t, app, "wc", true)
	addProject(t, dir, "wc-ab2c", "x", "todo", "a0", "# X\n", false, "")
	sib := filepath.Join(repo, "sib")
	if err := os.MkdirAll(sib, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, _, err := run(t, app, "scope", "init", sib, "--name", "sib", "--code-root", sib, "--auto-commit"); err != nil {
		t.Fatalf("init sibling: %v", err)
	}
	// Rewrite pj.cue's name so the registry key (sib) and the on-disk name (drifted) disagree.
	if err := os.WriteFile(filepath.Join(sib, "pj.cue"), []byte("name: \"drifted\"\nautoCommit: true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, errOut, err := run(t, app, "sync", "--scope", "wc")
	if ExitCodeFromError(err) != exitFailure {
		t.Fatalf("drifted sibling preflight must refuse non-zero, got %v", err)
	}
	if !strings.Contains(errOut, "name_drift:") || !strings.Contains(errOut, "pj scope forget") {
		t.Errorf("drifted sibling should ride name_drift with the recovery, got %q", errOut)
	}
}

// The happy path: snapshot only allowlisted dirt in one commit, warn non_allowlist on
// residue (AGENTS.md) without committing it, and push.
func TestSyncSnapshotsAllowlistWarnsResidueAndPushes(t *testing.T) {
	requireGit(t)
	remote := newBareRemote(t)
	m := cloneMachine(t, remote)
	dir := m.initScopeAutoCommit(t)
	addProject(t, dir, "wc-ab2c", "alpha", "todo", "a0", "# Alpha\n", false, "")
	if err := os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte("# handoff\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, errOut, err := m.sync(t, "--scope", "wc")
	if err != nil {
		t.Fatalf("sync: %v (stderr %q)", err, errOut)
	}
	if !strings.Contains(errOut, "non_allowlist:") {
		t.Errorf("AGENTS.md residue should ride non_allowlist, got %q", errOut)
	}
	if !remoteHas(t, remote, "wc/wc-ab2c-alpha.md") {
		t.Error("the project file should be pushed")
	}
	if !remoteHas(t, remote, "wc/pj.cue") {
		t.Error("pj.cue should be pushed")
	}
	if remoteHas(t, remote, "wc/AGENTS.md") {
		t.Error("AGENTS.md must never be committed or pushed")
	}
}

// A read-only machine that only pulls still fetches and skips the push, exiting 0.
func TestSyncReadOnlyMachinePullsAndSkipsPush(t *testing.T) {
	requireGit(t)
	a, b, _ := twoMachines(t)
	_ = a
	// B has nothing local to push; sync just fetches and is up to date.
	_, errOut, err := b.sync(t, "--scope", "wc")
	if err != nil {
		t.Fatalf("read-only sync: %v (stderr %q)", err, errOut)
	}
	if strings.Contains(errOut, "sync_disabled:") {
		t.Errorf("a healthy read-only sync should not ride sync_disabled, got %q", errOut)
	}
}

// The lock span releases on the success path: a pj mark on the same scope after sync
// acquires the very locks sync held and completes, rather than deadlocking on a leaked
// scope or git-root lock. The multi-sync tests already prove release on the paused path.
func TestSyncReleasesLocksForSubsequentWrite(t *testing.T) {
	requireGit(t)
	_, b, _ := twoMachines(t)
	if _, _, err := b.sync(t, "--scope", "wc"); err != nil {
		t.Fatalf("B sync: %v", err)
	}
	// A write verb takes the scope lock then the git-root lock — the same span sync held.
	// If sync leaked either, this blocks forever; it returning is the release proof.
	if _, _, err := run(t, b.app, "mark", "wc-ab2c", "in-progress"); err != nil {
		t.Fatalf("mark after sync must acquire the released locks and complete: %v", err)
	}
}

// Two auto-commit scopes sharing one git-root both participate in a single ambient sync:
// the preflight passes (they agree on autoCommit), sync acquires both scope locks plus the
// git-root lock, sweeps both dirs into one snapshot commit, and pushes both. Every other
// multi-scope test diverges the sibling to force a preflight refuse, so this is the only
// case that exercises the participant loop, the multi-lock span, and the cross-dir snapshot
// on the success path.
func TestSyncTwoScopesShareGitRootSnapshotOneCommit(t *testing.T) {
	requireGit(t)
	remote := newBareRemote(t)
	m := cloneMachine(t, remote)

	// wc and xy are two auto-commit scopes in sibling dirs of the same clone (one git-root).
	wcDir := filepath.Join(m.clone, "wc")
	xyDir := filepath.Join(m.clone, "xy")
	for name, dir := range map[string]string{"wc": wcDir, "xy": xyDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if _, _, err := run(t, m.app, "scope", "init", dir, "--name", name, "--code-root", dir, "--auto-commit"); err != nil {
			t.Fatalf("init %s: %v", name, err)
		}
	}
	addProject(t, wcDir, "wc-ab2c", "alpha", "todo", "a0", "# Alpha\n", false, "")
	addProject(t, xyDir, "xy-cd3e", "beta", "todo", "a0", "# Beta\n", false, "")

	// Ambient sync on wc targets the shared git-root, so xy participates too.
	if _, errOut, err := m.sync(t, "--scope", "wc"); err != nil {
		t.Fatalf("two-scope sync should complete: %v (stderr %q)", err, errOut)
	}

	// One snapshot commit carries both dirs, and both projects reach the remote.
	if got := topCommit(t, m.clone); !strings.HasPrefix(got, "pj: sync ") {
		t.Errorf("both dirs should ride one snapshot commit, got %q", got)
	}
	if !remoteHas(t, remote, "wc/wc-ab2c-alpha.md") {
		t.Error("wc's project should be pushed")
	}
	if !remoteHas(t, remote, "xy/xy-cd3e-beta.md") {
		t.Error("xy's project should be pushed in the same sync")
	}
}
