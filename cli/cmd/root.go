package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "bonfire",
	Short: "Bonfire - game server lifecycle management CLI",
	Long: `Bonfire is an operator CLI for managing game server infrastructure.
It handles provisioning, archiving, and retiring game servers via Terraform
and AWS S3.`,
}

// Execute runs the root command.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.AddCommand(provisionCmd)
	rootCmd.AddCommand(retireCmd)
	rootCmd.AddCommand(listCmd)
	rootCmd.AddCommand(statusCmd)
	rootCmd.AddCommand(botCmd)
}
