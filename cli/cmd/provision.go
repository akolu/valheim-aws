package cmd

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

// Package-level vars for testability.
var (
	tfPlanFn = terraformPlan
	tfInitFn = terraformInit
)

var provisionCmd = &cobra.Command{
	Use:   "provision <game>",
	Short: "Provision a game server (terraform init + plan + apply)",
	Long: `Provision a game server by running terraform init, plan, and apply for the
specified game workspace. The terraform plan is shown for review before any
infrastructure changes are made. The game server restores from long-term backup
automatically on first boot if an archive exists.`,
	Args: cobra.ExactArgs(1),
	RunE: runProvision,
}

func runProvision(cmd *cobra.Command, args []string) error {
	game := args[0]
	if err := validateGameName(game); err != nil {
		return err
	}
	return provisionGame(game, os.Stdin)
}

// provisionGame runs terraform init, plans, prompts for confirmation, then applies.
// Accepts stdin for testability.
func provisionGame(game string, stdin io.Reader) error {
	dir, err := terraformDir(game)
	if err != nil {
		return err
	}

	fmt.Printf("Initializing %s...\n", game)
	if err := tfInitFn(dir); err != nil {
		return err
	}

	planFile, err := os.CreateTemp("", fmt.Sprintf("bonfire-provision-%s-*.tfplan", game))
	if err != nil {
		return fmt.Errorf("creating plan file: %w", err)
	}
	planFile.Close()
	planPath := planFile.Name()
	defer os.Remove(planPath)

	fmt.Printf("\nPlanning %s infrastructure...\n", game)
	if err := tfPlanFn(dir, planPath); err != nil {
		return fmt.Errorf("terraform plan failed: %w", err)
	}

	fmt.Printf("\nType the game name to confirm: ")
	reader := bufio.NewReader(stdin)
	input, err := reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return fmt.Errorf("reading confirmation: %w", err)
	}
	input = strings.TrimSpace(input)

	if input != game {
		fmt.Println("Aborted.")
		return nil
	}

	if err := tfApplyPlanFn(dir, planPath); err != nil {
		return fmt.Errorf("terraform apply failed: %w", err)
	}

	fmt.Printf("✓ %s provisioned\n", game)
	return nil
}
