package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A one-sided pj status done lands uncontested on the other machine after sync.
func TestSyncOneSidedStatusLandsUncontested(t *testing.T) {
	requireGit(t)
	a, b, _ := twoMachines(t)

	a.status(t, "wc-ab2c", "done")
	if _, _, err := a.sync(t, "--scope", "wc"); err != nil {
		t.Fatalf("A sync: %v", err)
	}

	if _, _, err := b.sync(t, "--scope", "wc"); err != nil {
		t.Fatalf("B sync: %v", err)
	}
	path, archived := findProject(t, b.scopeDir(), "wc-ab2c-alpha.md")
	if path == "" || !archived {
		t.Fatalf("the completed project should have landed under archive/ on B")
	}
	if st := fmStatus(t, path); st != "done" {
		t.Errorf("B should see status done, got %q", st)
	}
}

// A rebase replaying several local commits that conflicts at more than one stop is carried
// to completion by a single pj sync — each auto-resolvable stop is merged, staged, and
// continued without a human handoff — then the run pushes.
func TestSyncMultiStopRebaseCompletes(t *testing.T) {
	requireGit(t)
	remote := newBareRemote(t)
	a := cloneMachine(t, remote)
	dirA := a.initScopeAutoCommit(t)
	addProject(t, dirA, "wc-ab2c", "one", "todo", "a0", "# One\n", false, "")
	addProject(t, dirA, "wc-cd3e", "two", "todo", "a1", "# Two\n", false, "")
	if _, _, err := a.sync(t, "--scope", "wc"); err != nil {
		t.Fatalf("A seed sync: %v", err)
	}
	b := cloneMachine(t, remote)
	dirB := b.importScope(t)

	// A advances both projects to in-progress in two commits, then pushes.
	a.status(t, "wc-ab2c", "in-progress")
	a.status(t, "wc-cd3e", "in-progress")
	if _, _, err := a.sync(t, "--scope", "wc"); err != nil {
		t.Fatalf("A sync: %v", err)
	}

	// B moves both to a different non-terminal status in two local commits: each conflicts
	// with A's change at a separate rebase stop, and both auto-resolve by LWW.
	b.status(t, "wc-ab2c", "review")
	b.status(t, "wc-cd3e", "review")
	_, errOut, err := b.sync(t, "--scope", "wc")
	if err != nil {
		t.Fatalf("B multi-stop sync should complete in one invocation: %v (stderr %q)", err, errOut)
	}
	if strings.Contains(errOut, "paused") {
		t.Errorf("an auto-resolvable multi-stop rebase must not report a human handoff, got %q", errOut)
	}
	p1, _ := findProject(t, dirB, "wc-ab2c-one.md")
	p2, _ := findProject(t, dirB, "wc-cd3e-two.md")
	if fmStatus(t, p1) == "todo" || fmStatus(t, p2) == "todo" {
		t.Errorf("both projects should have merged past the base status")
	}
	if n := gitIn(t, b.clone, "rev-list", "--count", "@{u}..HEAD"); n != "0" {
		t.Errorf("B should be fully pushed after the multi-stop sync, unpushed=%s", n)
	}
}

// A body-only conflict leaves a paused, reported rebase with clean field-merged
// frontmatter and markers only in the body; the next pj sync after resolution completes.
func TestSyncBodyConflictPausesThenResumes(t *testing.T) {
	requireGit(t)
	a, b, remote := twoMachines(t)

	pA, _ := findProject(t, a.scopeDir(), "wc-ab2c-alpha.md")
	editBody(t, pA, "A version of the body")
	if _, _, err := a.sync(t, "--scope", "wc"); err != nil {
		t.Fatalf("A body edit sync: %v", err)
	}

	pB, _ := findProject(t, b.scopeDir(), "wc-ab2c-alpha.md")
	editBody(t, pB, "B version of the body")
	_, errOut, err := b.sync(t, "--scope", "wc")
	if ExitCodeFromError(err) != exitFailure {
		t.Fatalf("body conflict must pause non-zero, got %v (stderr %q)", err, errOut)
	}
	if !strings.Contains(errOut, "body conflict") {
		t.Errorf("a body conflict should be reported, got %q", errOut)
	}
	if frontmatterHasMarkers(pB) {
		t.Errorf("frontmatter must be clean field-merged, not carry markers:\n%s", readFile(t, pB))
	}
	if !hasConflictMarker([]byte(readFile(t, pB))) {
		t.Errorf("the body should carry conflict markers awaiting the human:\n%s", readFile(t, pB))
	}

	// The human resolves the body in place, then re-syncs.
	resolved := stripConflictMarkers(readFile(t, pB), "resolved body")
	if err := os.WriteFile(pB, []byte(resolved), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := b.sync(t, "--scope", "wc"); err != nil {
		t.Fatalf("post-resolution sync should complete: %v", err)
	}
	if n := gitIn(t, b.clone, "rev-list", "--count", "@{u}..HEAD"); n != "0" {
		t.Errorf("B should be fully pushed after resolving the body, unpushed=%s", n)
	}
	_ = remote
}

// A pj sync entered while the git-root is mid-rebase makes no commit before resuming: an
// unrelated dirty allowlisted file present at resume is left uncommitted until the rebase
// completes, and the same invocation then snapshots it and pushes — never a snapshot
// commit on the temporary HEAD (which would orphan it, so its presence on the remote is
// the proof it was not).
func TestSyncMidRebaseEntryMakesNoCommitBeforeResume(t *testing.T) {
	requireGit(t)
	a, b, remote := twoMachines(t)

	pA, _ := findProject(t, a.scopeDir(), "wc-ab2c-alpha.md")
	editBody(t, pA, "A version")
	if _, _, err := a.sync(t, "--scope", "wc"); err != nil {
		t.Fatalf("A sync: %v", err)
	}
	pB, _ := findProject(t, b.scopeDir(), "wc-ab2c-alpha.md")
	editBody(t, pB, "B version")
	if _, _, err := b.sync(t, "--scope", "wc"); ExitCodeFromError(err) != exitFailure {
		t.Fatalf("expected a paused body conflict, got %v", err)
	}

	// The human resolves the body AND leaves an unrelated dirty allowlisted file.
	resolved := stripConflictMarkers(readFile(t, pB), "resolved body")
	if err := os.WriteFile(pB, []byte(resolved), 0o644); err != nil {
		t.Fatal(err)
	}
	addProject(t, b.scopeDir(), "wc-ff88", "extra", "todo", "a5", "# Extra\n", false, "")

	if _, _, err := b.sync(t, "--scope", "wc"); err != nil {
		t.Fatalf("resume + snapshot sync should complete: %v", err)
	}
	if !remoteHas(t, remote, "wc/wc-ff88-extra.md") {
		t.Errorf("the unrelated file must be snapshotted after the rebase completed and pushed (never orphaned on temp HEAD)")
	}
}

// A hand-deletion meeting a concurrent edit pauses the rebase reporting the deleted path
// and the surviving status; it completes on the next sync after the human removes the file.
func TestSyncDeleteEditPausesThenResumes(t *testing.T) {
	requireGit(t)
	a, b, remote := twoMachines(t)

	pA, _ := findProject(t, a.scopeDir(), "wc-ab2c-alpha.md")
	if err := os.Remove(pA); err != nil {
		t.Fatal(err)
	}
	if _, _, err := a.sync(t, "--scope", "wc"); err != nil {
		t.Fatalf("A delete sync: %v", err)
	}

	b.status(t, "wc-ab2c", "in-progress")
	_, errOut, err := b.sync(t, "--scope", "wc")
	if ExitCodeFromError(err) != exitFailure {
		t.Fatalf("delete/edit must pause non-zero, got %v (stderr %q)", err, errOut)
	}
	if !strings.Contains(errOut, "delete/edit") || !strings.Contains(errOut, "in-progress") {
		t.Errorf("delete/edit should report the surviving status, got %q", errOut)
	}
	assertDeleteEditHandoff(t, errOut, "wc/wc-ab2c-alpha.md", "in-progress")

	// The human removes the file, then re-syncs.
	pB, _ := findProject(t, b.scopeDir(), "wc-ab2c-alpha.md")
	if pB != "" {
		if err := os.Remove(pB); err != nil {
			t.Fatal(err)
		}
	}
	if _, _, err := b.sync(t, "--scope", "wc"); err != nil {
		t.Fatalf("post-resolution delete sync should complete: %v", err)
	}
	if remoteHas(t, remote, "wc/wc-ab2c-alpha.md") {
		t.Errorf("the removed project should not be on the remote after resolution")
	}
}

// An unactioned re-run after a delete/edit pause must not stage the survivor: the rebase
// stays paused, the same handoff is re-reported, and the remote still carries the deletion.
func TestSyncDeleteEditUnactionedRerunPauses(t *testing.T) {
	requireGit(t)
	a, b, remote := twoMachines(t)

	pA, _ := findProject(t, a.scopeDir(), "wc-ab2c-alpha.md")
	if err := os.Remove(pA); err != nil {
		t.Fatal(err)
	}
	if _, _, err := a.sync(t, "--scope", "wc"); err != nil {
		t.Fatalf("A delete sync: %v", err)
	}

	b.status(t, "wc-ab2c", "in-progress")
	_, firstOut, err := b.sync(t, "--scope", "wc")
	if ExitCodeFromError(err) != exitFailure {
		t.Fatalf("delete/edit must pause non-zero, got %v (stderr %q)", err, firstOut)
	}
	assertDeleteEditHandoff(t, firstOut, "wc/wc-ab2c-alpha.md", "in-progress")

	// Nothing touched — the next pj sync must re-pause, not silently resurrect the file.
	_, secondOut, err := b.sync(t, "--scope", "wc")
	if ExitCodeFromError(err) != exitFailure {
		t.Fatalf("unactioned re-run must stay paused, got %v (stderr %q)", err, secondOut)
	}
	assertDeleteEditHandoff(t, secondOut, "wc/wc-ab2c-alpha.md", "in-progress")
	assertSameDeleteEditLine(t, firstOut, secondOut)
	if remoteHas(t, remote, "wc/wc-ab2c-alpha.md") {
		t.Errorf("unactioned re-run must not push the resurrected survivor; remote still carries the deletion")
	}
}

// A delete/edit the human modifies stages as their resolution and completes the rebase.
func TestSyncDeleteEditModifiedResumes(t *testing.T) {
	requireGit(t)
	a, b, remote := twoMachines(t)

	pA, _ := findProject(t, a.scopeDir(), "wc-ab2c-alpha.md")
	if err := os.Remove(pA); err != nil {
		t.Fatal(err)
	}
	if _, _, err := a.sync(t, "--scope", "wc"); err != nil {
		t.Fatalf("A delete sync: %v", err)
	}

	b.status(t, "wc-ab2c", "in-progress")
	if _, _, err := b.sync(t, "--scope", "wc"); ExitCodeFromError(err) != exitFailure {
		t.Fatalf("expected delete/edit pause, got %v", err)
	}

	pB := mustSeedProject(t, b.scopeDir())
	editBody(t, pB, "human kept and edited the survivor")
	if _, _, err := b.sync(t, "--scope", "wc"); err != nil {
		t.Fatalf("modified-survivor re-run should complete: %v", err)
	}
	if !remoteHas(t, remote, "wc/wc-ab2c-alpha.md") {
		t.Errorf("the human's modified survivor should land on the remote")
	}
	if !strings.Contains(readFile(t, mustSeedProject(t, b.scopeDir())), "human kept and edited the survivor") {
		t.Errorf("the human's content should be what was staged")
	}
}

// A delete/edit the human resolves with git add never reaches the resumed stop: the path
// is no longer unmerged, so the rebase continues with that exact content.
func TestSyncDeleteEditGitAddResumes(t *testing.T) {
	requireGit(t)
	a, b, remote := twoMachines(t)

	pA, _ := findProject(t, a.scopeDir(), "wc-ab2c-alpha.md")
	if err := os.Remove(pA); err != nil {
		t.Fatal(err)
	}
	if _, _, err := a.sync(t, "--scope", "wc"); err != nil {
		t.Fatalf("A delete sync: %v", err)
	}

	b.status(t, "wc-ab2c", "in-progress")
	if _, _, err := b.sync(t, "--scope", "wc"); ExitCodeFromError(err) != exitFailure {
		t.Fatalf("expected delete/edit pause, got %v", err)
	}

	pB := mustSeedProject(t, b.scopeDir())
	before := readFile(t, pB)
	gitIn(t, b.clone, "add", "--", filepath.Join("wc", filepath.Base(pB)))
	if _, _, err := b.sync(t, "--scope", "wc"); err != nil {
		t.Fatalf("git-add resolution re-run should complete: %v", err)
	}
	if !remoteHas(t, remote, "wc/wc-ab2c-alpha.md") {
		t.Errorf("the git-add-ed survivor should land on the remote")
	}
	if got := readFile(t, mustSeedProject(t, b.scopeDir())); got != before {
		t.Errorf("git add keeps exact content; worktree drifted")
	}
}

// Mirrored delete/edit: this machine hand-deletes (surviving side is stage 2, the
// incoming edit). An unactioned re-run must pause and re-report, never abort on a stage read.
func TestSyncDeleteEditMirroredUnactionedRerunPauses(t *testing.T) {
	requireGit(t)
	a, b, remote := twoMachines(t)

	// A edits and pushes first — its content becomes the incoming survivor on B.
	a.status(t, "wc-ab2c", "review")
	if _, _, err := a.sync(t, "--scope", "wc"); err != nil {
		t.Fatalf("A edit sync: %v", err)
	}

	// B hand-deletes and snapshots the deletion, then fetches A's edit → delete/edit
	// with stage 2 present (incoming) and stage 3 absent (B's deletion).
	pB, _ := findProject(t, b.scopeDir(), "wc-ab2c-alpha.md")
	if err := os.Remove(pB); err != nil {
		t.Fatal(err)
	}
	_, firstOut, err := b.sync(t, "--scope", "wc")
	if ExitCodeFromError(err) != exitFailure {
		t.Fatalf("mirrored delete/edit must pause non-zero, got %v (stderr %q)", err, firstOut)
	}
	assertDeleteEditHandoff(t, firstOut, "wc/wc-ab2c-alpha.md", "review")
	if !strings.Contains(firstOut, "this machine's replayed commit") {
		t.Errorf("mirrored case should name this machine as the deleting side, got %q", firstOut)
	}

	_, secondOut, err := b.sync(t, "--scope", "wc")
	if ExitCodeFromError(err) != exitFailure {
		t.Fatalf("unactioned mirrored re-run must stay paused, got %v (stderr %q)", err, secondOut)
	}
	assertDeleteEditHandoff(t, secondOut, "wc/wc-ab2c-alpha.md", "review")
	assertSameDeleteEditLine(t, firstOut, secondOut)
	// Remote still has A's edit (the survivor was never pushed from B's resurrection path).
	if !remoteHas(t, remote, "wc/wc-ab2c-alpha.md") {
		t.Errorf("remote should still carry A's edit while B's delete/edit is paused")
	}
}

// A delete/edit whose surviving side carries unparseable frontmatter fail-closes at the
// first pause and every unactioned re-run — never a delete/edit line with an empty status.
func TestSyncDeleteEditUnparseableSurvivorFailClosed(t *testing.T) {
	requireGit(t)
	a, b, _ := twoMachines(t)

	pA, _ := findProject(t, a.scopeDir(), "wc-ab2c-alpha.md")
	if err := os.Remove(pA); err != nil {
		t.Fatal(err)
	}
	if _, _, err := a.sync(t, "--scope", "wc"); err != nil {
		t.Fatalf("A delete sync: %v", err)
	}

	// B's concurrent "edit" is a mangled frontmatter blob the merge cannot parse.
	pB := mustSeedProject(t, b.scopeDir())
	broken := "---\nid: wc-ab2c\nstatus: [unterminated\norder: a0\n---\n# alpha\n\nbody line\n"
	if err := os.WriteFile(pB, []byte(broken), 0o644); err != nil {
		t.Fatal(err)
	}
	_, firstOut, err := b.sync(t, "--scope", "wc")
	if ExitCodeFromError(err) != exitFailure {
		t.Fatalf("unparseable survivor must pause non-zero, got %v (stderr %q)", err, firstOut)
	}
	if !strings.Contains(firstOut, "config_unparseable:") || !strings.Contains(firstOut, "unparseable") {
		t.Errorf("first pause must fail-closed naming the parse fault, got %q", firstOut)
	}
	if strings.Contains(firstOut, "delete/edit") {
		t.Errorf("unparseable survivor must not be reported as delete/edit, got %q", firstOut)
	}

	_, secondOut, err := b.sync(t, "--scope", "wc")
	if ExitCodeFromError(err) != exitFailure {
		t.Fatalf("unactioned re-run must stay paused, got %v (stderr %q)", err, secondOut)
	}
	if !strings.Contains(secondOut, "config_unparseable:") || !strings.Contains(secondOut, "unparseable") {
		t.Errorf("re-run must re-report the same fail-closed handoff, got %q", secondOut)
	}
	if strings.Contains(secondOut, "delete/edit") {
		t.Errorf("re-run must not reclassify as delete/edit, got %q", secondOut)
	}
}

// assertDeleteEditHandoff checks the project-.md delete/edit line: path, side naming in
// reader terms (never ours/theirs), surviving status, and the three ways out.
func assertDeleteEditHandoff(t *testing.T, errOut, path, survivingStatus string) {
	t.Helper()
	if !strings.Contains(errOut, "delete/edit") {
		t.Errorf("expected delete/edit handoff, got %q", errOut)
	}
	if !strings.Contains(errOut, path) {
		t.Errorf("expected path %q in handoff, got %q", path, errOut)
	}
	if !strings.Contains(errOut, survivingStatus) {
		t.Errorf("expected surviving status %q in handoff, got %q", survivingStatus, errOut)
	}
	if strings.Contains(errOut, " ours") || strings.Contains(errOut, " theirs") ||
		strings.Contains(errOut, "the ours") || strings.Contains(errOut, "the theirs") {
		t.Errorf("handoff must not name sides as ours/theirs, got %q", errOut)
	}
	if !strings.Contains(errOut, "incoming side") && !strings.Contains(errOut, "this machine's replayed commit") {
		t.Errorf("handoff must name the deleting side in reader terms, got %q", errOut)
	}
	for _, way := range []string{"remove " + path, "edit it", "git add"} {
		if !strings.Contains(errOut, way) {
			t.Errorf("handoff must name %q as a way out, got %q", way, errOut)
		}
	}
	if strings.Contains(errOut, "restore or remove") {
		t.Errorf("handoff must not use the old restore-or-remove wording, got %q", errOut)
	}
}

// assertSameDeleteEditLine requires the first pause and an unactioned re-run to print the
// same delete/edit handoff line (not merely the same class of message).
func assertSameDeleteEditLine(t *testing.T, firstOut, secondOut string) {
	t.Helper()
	first, second := extractDeleteEditLine(firstOut), extractDeleteEditLine(secondOut)
	if first == "" || second == "" {
		t.Fatalf("missing delete/edit line: first=%q second=%q", first, second)
	}
	if first != second {
		t.Errorf("first pause and re-run must print the same delete/edit line:\n  first:  %s\n  second: %s", first, second)
	}
}

func extractDeleteEditLine(errOut string) string {
	for _, line := range strings.Split(errOut, "\n") {
		if strings.Contains(line, "delete/edit") {
			return strings.TrimSpace(line)
		}
	}
	return ""
}

// A lone hand-deleted project file commits as pj: remove <id>; several dirty paths commit
// as pj: sync <n> path(s).
func TestSyncSnapshotMessageClasses(t *testing.T) {
	requireGit(t)
	remote := newBareRemote(t)
	m := cloneMachine(t, remote)
	dir := m.initScopeAutoCommit(t)

	// Several dirty paths: pj.cue + .gitignore (from init) + one project → summary message.
	addProject(t, dir, "wc-ab2c", "alpha", "todo", "a0", "# Alpha\n", false, "")
	if _, _, err := m.sync(t, "--scope", "wc"); err != nil {
		t.Fatalf("multi-path sync: %v", err)
	}
	if got := topCommit(t, m.clone); !strings.HasPrefix(got, "pj: sync ") || !strings.HasSuffix(got, "path(s)") {
		t.Errorf("a multi-path snapshot should use the summary message, got %q", got)
	}

	// A lone deletion → pj: remove <id>.
	if err := os.Remove(filepath.Join(dir, "wc-ab2c-alpha.md")); err != nil {
		t.Fatal(err)
	}
	if _, _, err := m.sync(t, "--scope", "wc"); err != nil {
		t.Fatalf("delete sync: %v", err)
	}
	if got := topCommit(t, m.clone); got != "pj: remove wc-ab2c" {
		t.Errorf("a lone hand-deletion should commit as pj: remove wc-ab2c, got %q", got)
	}
}

// A conflict on a file pj neither owns nor can classify (a human-authored file at the
// git-root, outside any scope dir) pauses the rebase with the path named, so the closing
// "resolve the file(s) above" line is actionable rather than pointing at nothing.
func TestSyncUnownableConflictNamesThePath(t *testing.T) {
	requireGit(t)
	a, b, _ := twoMachines(t)

	// Both machines add the same root-level non-pj file with different content — an add/add
	// conflict on a path pj can neither own nor classify.
	if err := os.WriteFile(filepath.Join(a.clone, "shared.txt"), []byte("A version\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitIn(t, a.clone, "add", "shared.txt")
	gitIn(t, a.clone, "commit", "-m", "add shared (A)")
	if _, _, err := a.sync(t, "--scope", "wc"); err != nil {
		t.Fatalf("A sync: %v", err)
	}

	if err := os.WriteFile(filepath.Join(b.clone, "shared.txt"), []byte("B version\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitIn(t, b.clone, "add", "shared.txt")
	gitIn(t, b.clone, "commit", "-m", "add shared (B)")

	_, errOut, err := b.sync(t, "--scope", "wc")
	if ExitCodeFromError(err) != exitFailure {
		t.Fatalf("an unownable conflict must pause non-zero, got %v (stderr %q)", err, errOut)
	}
	if !strings.Contains(errOut, "unresolvable conflict") || !strings.Contains(errOut, "shared.txt") {
		t.Errorf("the paused rebase must name the unresolvable path, got %q", errOut)
	}
}

// stripConflictMarkers removes git conflict-marker lines and their hunk contents, leaving
// a single resolved body line — the human's in-file resolution.
func stripConflictMarkers(content, resolvedBody string) string {
	var out []string
	skip := false
	replaced := false
	for _, line := range strings.Split(content, "\n") {
		switch {
		case strings.HasPrefix(line, "<<<<<<<"):
			skip = true
			if !replaced {
				out = append(out, resolvedBody)
				replaced = true
			}
		case strings.HasPrefix(line, "======="):
			// stay skipping through the second hunk
		case strings.HasPrefix(line, ">>>>>>>"):
			skip = false
		case !skip:
			out = append(out, line)
		}
	}
	return strings.Join(out, "\n")
}
