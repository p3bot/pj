package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/start-cli/pj/internal/status"
)

func newMarkCmd(app *App) *cobra.Command {
	var scope string
	cmd := &cobra.Command{
		Use:   "mark <id> <status> [--scope S]",
		Short: "Mark a project's status (blocked / done / in-progress / …)",
		Long: "Rewrite a project's status. When the new status crosses the terminal boundary\n" +
			"(non-terminal ↔ terminal) the file is renamed between the dir root and archive/\n" +
			"in the same write, and the post-move absolute path is printed. Statuses are\n" +
			"labels: any known status (built-in or CUE custom) is accepted; an unknown one is\n" +
			"a usage error. An auto-commit scope self-commits the change when a git-root\n" +
			"exists. A quarantined or duplicate-id project is refused with no write.\n" +
			"For a scope pulse (counts, next, integrity), use `pj status`.",
		Args: usageArgs(cobra.ExactArgs(2)),
		RunE: func(c *cobra.Command, args []string) error {
			return runMark(app, c, args[0], args[1], scope)
		},
	}
	cmd.Flags().StringVar(&scope, "scope", "", "ambient scope for a short id")
	return cmd
}

func runMark(app *App, c *cobra.Command, idArg, newStatus, scopeFlag string) error {
	form, ok := parseIDArg(idArg)
	if !ok {
		return usageErrorf("%q is not a valid project id", idArg)
	}

	e, err := app.openEngine(c)
	if err != nil {
		return err
	}
	defer e.close()

	scope, err := e.scopeForID(idArg, form, scopeFlag)
	if err != nil {
		return err
	}
	entry, registered := e.reg.Scopes[scope]
	if !registered {
		return fmt.Errorf("unknown project id %q: scope %q is not registered here", idArg, scope)
	}
	dir := entry.Dir

	lock, err := acquireScopeLock(dir)
	if err != nil {
		return err
	}
	defer func() { _ = lock.Release() }()

	ctx := c.Context()
	res, err := e.reconcileResult(single(scope, dir))
	if err != nil {
		return err
	}
	if err := refuseUnusableScope(res, scope, dir); err != nil {
		return err
	}
	schema := res.Schema(scope)
	custom := schemaCustom(schema)
	if !status.IsKnown(newStatus, custom) {
		return usageErrorf("%q is not a known status for scope %q", newStatus, scope)
	}
	autoCommit := schemaAutoCommit(schema)
	root, hasRoot := gitRootFor(dir)
	if err := checkMidRebase(ctx, scope, autoCommit, root, hasRoot); err != nil {
		return err
	}
	e.printWarnings(c, res.Warnings)

	p, err := e.resolveWriteRow(scope, idArg, form)
	if err != nil {
		return err
	}

	m, body, err := readProjectFile(p.Path)
	if err != nil {
		return err
	}
	wasTerminal := status.IsTerminal(m.Status, custom)
	nowTerminal := status.IsTerminal(newStatus, custom)
	m.Status = newStatus

	newPath, oldPath := p.Path, ""
	if wasTerminal != nowTerminal {
		newPath, err = terminalLocation(dir, filepath.Base(p.Path), nowTerminal)
		if err != nil {
			return err
		}
		oldPath = p.Path
	}

	// Write then rename: crash never leaves two same-id files (layout drift, not a collision).
	if err := writeProjectFile(p.Path, m, body); err != nil {
		return err
	}
	if oldPath != "" && oldPath != newPath {
		if err := os.Rename(oldPath, newPath); err != nil {
			return fmt.Errorf("move %s to %s: %w", oldPath, newPath, err)
		}
	}
	if err := e.rec.SyncPaths(scope, writtenPaths(newPath, oldPath)); err != nil {
		return err
	}

	// Keep historical "-> status" commit subject shape.
	message := fmt.Sprintf("pj: %s -> %s", p.ID, newStatus)
	if err := e.completeStateDurability(ctx, c, scope, dir, autoCommit, message, newPath, oldPath, root, hasRoot); err != nil {
		return err
	}

	out, err := absPath(newPath)
	if err != nil {
		return err
	}
	stdoutln(c, out)
	return nil
}
