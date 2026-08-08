package cli

import "github.com/spf13/cobra"

// newScopeCmd: bare `tk scope` lists; unknown subcommand is usage (not silent list).
func newScopeCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "scope",
		Aliases: []string{"scopes"},
		Short:   "Manage scopes — register, address, and inspect ticket containers",
		Long: "A scope is a directory of ticket markdown files plus its tk.cue. Scope\n" +
			"administration registers scopes on this machine, rebinds their paths, and\n" +
			"lists them. Bare `tk scope` runs `list`.",
		Args: cobra.ArbitraryArgs,
		RunE: func(c *cobra.Command, args []string) error {
			if len(args) > 0 {
				return usageErrorf("unknown scope subcommand %q; run `tk scope --help` for the available subcommands", args[0])
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
