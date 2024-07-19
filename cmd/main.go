package main

import (
	"log"

	cmd "github.com/celestiaorg/apollo/cmd/subcommands"
	"github.com/spf13/cobra"
)

func main() {
	rootCmd := NewRootCmd()

	if err := rootCmd.Execute(); err != nil {
		log.Fatal(err)
	}
}

func NewRootCmd() *cobra.Command {
	command := &cobra.Command{
		Use:   "apollo",
		Short: "Apollo CLI",
		Long:  `Apollo CLI is a tool to run and manage a local Celestia devnet.`,
	}

	command.AddCommand(cmd.NewUpCmd())
	command.AddCommand(cmd.NewDownCmd())

	return command
}
