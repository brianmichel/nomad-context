package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

// Version describes the application version. Overridden via -ldflags.
var Version = "dev"

func newVersionCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the nomad-context version",
		RunE: func(cmd *cobra.Command, _ []string) error {
			fmt.Fprintln(cmd.OutOrStdout(), formatVersionOutput(cmd))
			return nil
		},
	}
}

func formatVersionOutput(cmd *cobra.Command) string {
	name := "nomad-context"
	if cmd != nil && cmd.Root() != nil && cmd.Root().Name() != "" {
		name = cmd.Root().Name()
	}
	return fmt.Sprintf("%s version %s", name, Version)
}
