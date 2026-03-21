package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
)

var retireCmd = &cobra.Command{
	Use:   "retire <game>",
	Short: "Archive saves then destroy the game server (end of season)",
	Long: `Archive all save files to the long-term bucket, then run terraform
destroy. This is the 'end of season' command. The long-term bucket is
preserved; only the game server and backup bucket are destroyed.`,
	Args: cobra.ExactArgs(1),
	RunE: runRetire,
}

func runRetire(cmd *cobra.Command, args []string) error {
	game := args[0]
	ctx := context.Background()

	// Step 1: Archive saves to long-term bucket
	fmt.Printf("Step 1/2: Archiving %s saves before retire...\n", game)
	if err := archiveGame(ctx, game); err != nil {
		return fmt.Errorf("archive failed: %w", err)
	}

	// Step 2: Terraform destroy
	dir, err := terraformDir(game)
	if err != nil {
		return err
	}
	fmt.Printf("\nStep 2/2: Destroying %s infrastructure...\n", game)
	if err := terraformDestroy(dir); err != nil {
		return fmt.Errorf("terraform destroy failed: %w", err)
	}

	fmt.Printf("\n✓ %s retired (saves archived, infrastructure destroyed)\n", game)
	return nil
}
