package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/spf13/cobra"
)

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Pull latest source and reinstall the CLI",
	Long: `Pull the latest source from origin/main and reinstall the CLI binary.

Runs in sequence:
  1. git pull    — updates the local repository to origin/main
  2. make install — rebuilds and installs the bonfire binary with the new version

Requires the bonfire repository to be accessible (set BONFIRE_REPO_ROOT if needed).`,
	Args: cobra.NoArgs,
	RunE: runUpdate,
}

func runUpdate(cmd *cobra.Command, args []string) error {
	root, err := findRepoRoot()
	if err != nil {
		return err
	}

	fmt.Println("==> Pulling latest source...")
	pullCmd := exec.Command("git", "pull")
	pullCmd.Dir = root
	pullCmd.Stdout = os.Stdout
	pullCmd.Stderr = os.Stderr
	if err := pullCmd.Run(); err != nil {
		return fmt.Errorf("git pull: %w", err)
	}

	fmt.Println("==> Reinstalling bonfire CLI...")
	makeCmd := exec.Command("make", "install")
	makeCmd.Dir = filepath.Join(root, "cli")
	makeCmd.Stdout = os.Stdout
	makeCmd.Stderr = os.Stderr
	if err := makeCmd.Run(); err != nil {
		return fmt.Errorf("make install: %w", err)
	}

	fmt.Println("✓ bonfire updated successfully")
	return nil
}
