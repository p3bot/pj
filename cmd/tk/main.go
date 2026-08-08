// Command tk is the agent ticket management CLI.
// Entry point only: run the command tree, map signal/error to exit code, exit.
// All command logic lives in internal/cli.
package main

import (
	"os"

	"github.com/p3bot/tk/internal/cli"
)

func main() {
	err := cli.Execute()

	// Interrupt (POSIX 128+signum) takes precedence over handler error mapping.
	if code := cli.SignalExitCode(); code != 0 {
		os.Exit(code)
	}
	if err != nil {
		cli.PrintError(err)
		os.Exit(cli.ExitCodeFromError(err))
	}
}
