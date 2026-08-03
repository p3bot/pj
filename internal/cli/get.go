package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/start-cli/pj/internal/index"
	"github.com/start-cli/pj/internal/token"
)

func newGetCmd(app *App) *cobra.Command {
	var scope string
	var content bool
	cmd := &cobra.Command{
		Use:   "get <id> [--content] [--scope S]",
		Short: "Resolve a short or full id to its project file path or contents",
		Long: "By default, print the cleaned absolute path of a project's file. With\n" +
			"--content, print the full file contents on stdout instead (exact bytes).\n" +
			"A short id resolves in the ambient scope (or --scope / PJ_SCOPE); a full\n" +
			"<scope>-<short-id> resolves in any registered scope. get locates for\n" +
			"repair: a project in parse_error quarantine still succeeds (path or\n" +
			"contents), riding parse_error on stderr. A duplicate id is refused.\n" +
			"Pure read; never runs git.",
		Args: usageArgs(cobra.ExactArgs(1)),
		RunE: func(c *cobra.Command, args []string) error {
			return runGet(app, c, args[0], scope, content)
		},
	}
	cmd.Flags().StringVar(&scope, "scope", "", "ambient scope for a short id")
	cmd.Flags().BoolVar(&content, "content", false, "print full file contents instead of the path")
	return cmd
}

func runGet(app *App, c *cobra.Command, idArg, scope string, content bool) error {
	e, err := app.openEngine(c)
	if err != nil {
		return err
	}
	defer e.close()

	r, err := e.resolveProject(c, idArg, scope)
	if err != nil {
		return err
	}
	if len(r.rows) > 1 {
		return duplicateRefusal(r.rows)
	}
	p := r.rows[0]
	if err := ensureFileExists(p); err != nil {
		return err
	}
	if p.ParseError {
		stderrln(c, token.Line(token.ParseError, fmt.Sprintf("%s: %s", p.ID, p.ParseMsg)))
	}
	if content {
		data, err := os.ReadFile(p.Path)
		if err != nil {
			return fmt.Errorf("read %s: %w", p.Path, err)
		}
		if _, err := c.OutOrStdout().Write(data); err != nil {
			return err
		}
		return nil
	}
	stdoutln(c, p.Path)
	return nil
}

// ensureFileExists refuses a stale index row whose file has vanished.
func ensureFileExists(p *index.Project) error {
	if _, err := os.Stat(p.Path); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("project %s resolves to %s but the file is missing — run pj doctor --reindex", p.ID, p.Path)
		}
		return fmt.Errorf("stat %s: %w", p.Path, err)
	}
	return nil
}
