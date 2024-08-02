package cmd

import (
	"fmt"
	"net/http"

	"github.com/spf13/cobra"
)

func NewDownCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "down",
		Short: "Shuts down the Apollo network.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return ShutdownNode()
		},
	}

	return cmd
}

func ShutdownNode() error {
	resp, err := http.Get("http://localhost:8080/shutdown/")
	if err != nil {
		return fmt.Errorf("failed to call shutdown endpoint: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("shutdown endpoint returned status: %s", resp.Status)
	}

	fmt.Println("apollo network shut down successfully")
	return nil
}
