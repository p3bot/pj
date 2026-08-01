package cli

import "github.com/spf13/cobra"

// newScopeCmd: bare `pj scope` lists; unknown subcommand is usage (not silent list).
func newScopeCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "scope",
		Aliases: []string{"scopes"},
		Short:   "Manage scopes — register, address, and inspect project containers",
		Long: "A scope is a directory of project markdown files plus its pj.cue. Scope\n" +
			"administration registers scopes on this machine, rebinds their paths, and\n" +
			"lists them. Bare `pj scope` runs `list`.",
		Args: cobra.ArbitraryArgs,
		RunE: func(c *cobra.Command, args []string) error {
			if len(args) > 0 {
				return usageErrorf("unknown scope subcommand %q; run `pj scope --help` for the available subcommands", args[0])
			}
			return runScopeList(app, c)
		},
	}
	cmd.AddCommand(
		newScopeInitCmd(app),
		newScopeImportCmd(app),
		newScopeRebindCmd(app),
		newScopeForgetCmd(app),
		newScopeListCmd(app),
		newScopeRenameCmd(app),
	)
	return cmd
}
