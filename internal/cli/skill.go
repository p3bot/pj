package cli

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/start-cli/pj/internal/skill"
)

// skillInstallRefuse is shared by install/list/uninstall (hard refuse, no tree write).
const skillInstallRefuse = "not implemented in v1 — use 'pj skill' to print the workflow; persistent install is planned via agentdex skills directories"

func newSkillCmd(_ *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "skill",
		Short: "Print the agent skill contract (or refuse install placeholders)",
		Long: "Print the locked agent skill contract to stdout as agent-facing workflow\n" +
			"markdown. No ambient scope is required and nothing is written into any tree.\n\n" +
			"The install family (install / list / uninstall) is reserved for a future\n" +
			"agentdex-backed persistent install; each hard-refuses in v1 with a clear\n" +
			"message so agents do not invent paths.",
		Args: usageArgs(cobra.NoArgs),
		RunE: func(c *cobra.Command, _ []string) error {
			// Surface write failure (EPIPE) like meta / query --schema.
			_, err := fmt.Fprint(c.OutOrStdout(), skill.Text())
			return err
		},
	}
	cmd.AddCommand(
		newSkillRefuseCmd("install", "Install the skill into an agent skills directory (not implemented in v1)"),
		newSkillRefuseCmd("list", "List installed skill copies (not implemented in v1)"),
		newSkillRefuseCmd("uninstall", "Remove an installed skill copy (not implemented in v1)"),
	)
	return cmd
}

func newSkillRefuseCmd(use, short string) *cobra.Command {
	return &cobra.Command{
		Use:   use,
		Short: short,
		Args:  usageArgs(cobra.NoArgs),
		RunE: func(*cobra.Command, []string) error {
			return &ExitError{
				Code:  exitFailure,
				Plain: true,
				Err:   errors.New(skillInstallRefuse),
			}
		},
	}
}
