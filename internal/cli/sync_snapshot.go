package cli

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/start-cli/pj/internal/git"
	"github.com/start-cli/pj/internal/selfcommit"
	"github.com/start-cli/pj/internal/token"
)

// dirtyProject is one dirty allowlisted path with the porcelain status code that decides
// its single-path commit message class and the scope that owns it.
type dirtyProject struct {
	path  string
	code  string
	scope string
}

// snapshot is step 1: scoped to each participating auto-commit dir (never the whole
// tree, never a co-located non-auto-commit scope's dir), stage every dirty path matching
// the closed allowlist and make one commit for the whole snapshot. A deleted allowlisted
// path is staged as a deletion so the tree converges with disk — sync mirrors what the
// human left, it never authors or polices deletions. Non-allowlist residue is warned and
// left uncommitted. The whole step runs under sync's held git-root lock, so it commits
// through the self-commit core, not the acquiring wrapper.
func (e *engine) snapshot(c *cobra.Command, t syncTarget, rep *syncReport) error {
	ctx := c.Context()
	var staged []dirtyProject
	var allowlisted []string
	for _, p := range t.participants {
		entries, err := git.DirtyEntries(ctx, t.root, p.dir)
		if err != nil {
			return err
		}
		var residue []string
		for _, ent := range entries {
			if skipSnapshotPath(ent.Path) {
				continue // .pj.lock: gitignored, skipped defensively regardless
			}
			if isAllowlistedScopeFile(ent.Path, p.dir) {
				allowlisted = append(allowlisted, ent.Path)
				staged = append(staged, dirtyProject{path: ent.Path, code: ent.Code, scope: p.name})
			} else {
				residue = append(residue, ent.Path)
			}
		}
		if len(residue) > 0 {
			rep.residueN += len(residue)
			stderrln(c, token.Line(token.NonAllowlist, fmt.Sprintf(
				"%d path(s) under %s not committed — move or remove; see pj doctor", len(residue), p.dir)))
			for _, r := range residue {
				stderrln(c, "  "+r)
			}
		}
	}

	if len(allowlisted) == 0 {
		return nil // nothing dirty to snapshot; the fetch/integrate still runs
	}
	rep.snapshotN = len(allowlisted)
	return selfcommit.CommitPathsCore(ctx, selfcommit.BatchRequest{
		StateDir: e.app.StateDir,
		GitRoot:  t.root,
		Message:  snapshotMessage(staged),
		Paths:    allowlisted,
	})
}

// skipSnapshotPath reports whether a dirty path is the per-scope lock file, which is
// gitignored and never committed — skipped defensively even if it somehow surfaces.
func skipSnapshotPath(path string) bool {
	return filepath.Base(path) == scopeLockName
}

// snapshotMessage is one summary for the whole snapshot — pj: sync <n> path(s) — except
// when the snapshot is exactly one path, which takes that path's class-specific form,
// matching what the write verbs already produce for a single file. Never one commit per
// path: the single snapshot commit keeps the other machine's rebase from replaying a
// pile of tiny commits.
func snapshotMessage(staged []dirtyProject) string {
	if len(staged) != 1 {
		return fmt.Sprintf("pj: sync %d path(s)", len(staged))
	}
	d := staged[0]
	base := filepath.Base(d.path)
	switch base {
	case "pj.cue":
		return "pj: config " + d.scope
	case ".gitignore":
		return "pj: gitignore " + d.scope
	}
	fullID, slug := parseProjectBasename(base)
	switch {
	case strings.ContainsRune(d.code, 'D'):
		return "pj: remove " + fullID
	case d.code == "??":
		if slug != "" {
			return "pj: add " + fullID + " " + slug
		}
		return "pj: add " + fullID
	default:
		return "pj: edit " + fullID
	}
}

// parseProjectBasename splits a project basename <scope>-<short>[-<slug>].md into its
// full id and slug. Scope names and short-ids carry no hyphen, so the id is exactly the
// first two segments and any remainder is the slug.
func parseProjectBasename(base string) (fullID, slug string) {
	stem := strings.TrimSuffix(base, ".md")
	parts := strings.SplitN(stem, "-", 3)
	if len(parts) < 2 {
		return stem, ""
	}
	fullID = parts[0] + "-" + parts[1]
	if len(parts) == 3 {
		slug = parts[2]
	}
	return fullID, slug
}
