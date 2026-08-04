package cli

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/p3bot/pj/internal/git"
	"github.com/p3bot/pj/internal/selfcommit"
	"github.com/p3bot/pj/internal/token"
)

type dirtyProject struct {
	path  string
	code  string
	scope string
}

// snapshot: CommitPathsCore under held lock; non-allowlist warned, not committed.
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

func skipSnapshotPath(path string) bool {
	return filepath.Base(path) == scopeLockName
}

// snapshotMessage: one commit for the whole snapshot (avoids tiny-commit replay piles).
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
