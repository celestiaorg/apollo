package cmd

import (
	"github.com/spf13/cobra"
)

func NewRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "apollo",
		Short: "Apollo CLI",
		Long:  `Apollo CLI is a tool to run and manage a local Celestia devnet.`,
	}

	// Add subcommands
	cmd.AddCommand(NewUpCmd())
	cmd.AddCommand(NewDownCmd())

	return cmd
}
