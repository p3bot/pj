package cli

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/p3bot/pj/internal/index"
	"github.com/p3bot/pj/internal/registry"
	"github.com/p3bot/pj/internal/token"
	"github.com/p3bot/pj/internal/xdg"
)

func newLensCmd(app *App) *cobra.Command {
	var (
		scope     string
		clearLens bool
	)
	cmd := &cobra.Command{
		Use:   "lens [tags...] | --clear [--scope S]",
		Short: "Set, show, or clear the machine-local default tag view for a scope",
		Long: "A lens is a per-scope, machine-local default tag view. With tags, it sets the\n" +
			"lens; with --clear it removes it; with no arguments it shows the current lens.\n" +
			"list and next apply the lens by default (an untagged project is never hidden;\n" +
			"--no-lens bypasses). A lens tag outside the scope's declared knownTags rides a\n" +
			"schema_warn typo warning but is still allowed.",
		Args: usageArgs(cobra.ArbitraryArgs),
		RunE: func(c *cobra.Command, args []string) error {
			return runLens(app, c, args, scope, clearLens)
		},
	}
	cmd.Flags().StringVar(&scope, "scope", "", "scope to set the lens for (defaults to ambient)")
	cmd.Flags().BoolVar(&clearLens, "clear", false, "clear the lens for the scope")
	return cmd
}

func runLens(app *App, c *cobra.Command, args []string, scopeFlag string, clearLens bool) error {
	if clearLens && len(args) > 0 {
		return usageErrorf("--clear takes no tags")
	}

	e, err := app.openEngine(c)
	if err != nil {
		return err
	}
	defer e.close()

	resolved, err := e.resolveAmbient(scopeFlag)
	if err != nil {
		return err
	}
	scope := resolved.Name

	switch {
	case clearLens:
		return e.writeLens(scope, nil)
	case len(args) == 0:
		stdoutln(c, strings.Join(e.reg.Lens[scope], " "))
		return nil
	default:
		tags := dedupeSorted(args)
		warnUnknownTags(c, e, scope, resolved.Entry.Dir, tags)
		return e.writeLens(scope, tags)
	}
}

// writeLens: machine-global flock spans load-modify-write.
func (e *engine) writeLens(scope string, tags []string) error {
	lock, err := xdg.AcquireConfigLock(e.app.ConfigDir)
	if err != nil {
		return err
	}
	defer func() { _ = lock.Release() }()

	store := registry.NewStore(e.app.Ctx, e.app.ConfigDir)
	reg, err := store.Load()
	if err != nil {
		return err
	}
	if reg.Lens == nil {
		reg.Lens = map[string][]string{}
	}
	if len(tags) == 0 {
		delete(reg.Lens, scope)
	} else {
		reg.Lens[scope] = tags
	}
	return store.WriteLens(reg.Lens)
}

func warnUnknownTags(c *cobra.Command, e *engine, scope, dir string, tags []string) {
	schema := e.rec.SchemaCached(scope, dir)
	if schema == nil || len(schema.KnownTags) == 0 {
		return
	}
	known := map[string]bool{}
	for _, t := range schema.KnownTags {
		known[t] = true
	}
	for _, t := range tags {
		if !known[t] {
			stderrln(c, token.Line(token.SchemaWarn, fmt.Sprintf("%s: tag %q is not in %s knownTags", scope, t, scope)))
		}
	}
}

// passesLens: empty lens shows all; untagged projects are never hidden.
func passesLens(p *index.Project, lens []string) bool {
	if len(lens) == 0 || len(p.Tags) == 0 {
		return true
	}
	set := map[string]bool{}
	for _, t := range lens {
		set[t] = true
	}
	for _, t := range p.Tags {
		if set[t] {
			return true
		}
	}
	return false
}

// lensEcho rides stderr only — never a TSV stdout field.
func lensEcho(lens []string) string {
	return "lens: " + lensBracket(lens)
}

func dedupeSorted(items []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range items {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	sort.Strings(out)
	return out
}
