package cli

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"
)

func newEditCmd(app *App) *cobra.Command {
	var scope string
	cmd := &cobra.Command{
		Use:   "edit <id> [--scope S]",
		Short: "Open a ticket's file in $EDITOR",
		Long: "Resolve an id to its path and open it in $EDITOR for human editing. It prints\n" +
			"nothing on success (not a path-hand-off verb), never rewrites frontmatter, and\n" +
			"never self-commits — an editor save is an ordinary direct edit. It may open a\n" +
			"parse_error path for repair.",
		Args: usageArgs(cobra.ExactArgs(1)),
		RunE: func(c *cobra.Command, args []string) error {
			return runEdit(app, c, args[0], scope)
		},
	}
	cmd.Flags().StringVar(&scope, "scope", "", "ambient scope for a short id")
	return cmd
}

func runEdit(app *App, c *cobra.Command, idArg, scope string) error {
	e, err := app.openEngine(c)
	if err != nil {
		return err
	}
	defer e.close()

	r, err := e.resolveTicket(c, idArg, scope)
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

	// $EDITOR may carry flags; split into program + args.
	fields := strings.Fields(os.Getenv("EDITOR"))
	if len(fields) == 0 {
		return fmt.Errorf("$EDITOR is not set — set it to your editor to use tk edit")
	}
	// Share stdio: edit is the human-in-the-loop verb.
	args := append(append([]string(nil), fields[1:]...), p.Path)
	ed := exec.Command(fields[0], args...)
	ed.Stdin, ed.Stdout, ed.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := ed.Run(); err != nil {
		return fmt.Errorf("editor exited with an error: %w", err)
	}
	return nil
}
