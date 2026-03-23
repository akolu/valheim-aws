package cmd

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/spf13/cobra"
)

// Package-level vars for testability.
var (
	tfPlanDestroyFn = terraformPlanDestroy
	tfApplyPlanFn   = terraformApplyPlan
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
	if err := validateGameName(game); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cfg, err := awsConfig(ctx)
	if err != nil {
		return fmt.Errorf("loading AWS config: %w", err)
	}
	return retireGame(ctx, s3.NewFromConfig(cfg), cfg.Region, game, os.Stdin)
}

// retireGame archives saves then destroys infrastructure. Accepts a client and stdin for testability.
func retireGame(ctx context.Context, s3Client s3API, region, game string, stdin io.Reader) error {
	// Step 1: Archive saves to long-term bucket
	fmt.Printf("Step 1/3: Archiving %s saves before retire...\n", game)
	if err := archiveGame(ctx, s3Client, region, game); err != nil {
		return fmt.Errorf("archive failed: %w", err)
	}

	// Step 2: Empty the backup bucket so terraform destroy can remove it
	// (force_destroy=false is intentional; we empty it manually after archiving)
	backupBucket := backupBucketName(game, region)
	fmt.Printf("\nStep 2/3: Emptying backup bucket %s...\n", backupBucket)
	if err := emptyVersionedBucket(ctx, s3Client, backupBucket); err != nil {
		return fmt.Errorf("emptying backup bucket: %w", err)
	}

	// Step 3: Plan the destroy and confirm before applying
	dir, err := terraformDir(game)
	if err != nil {
		return err
	}

	planFile, err := os.CreateTemp("", fmt.Sprintf("bonfire-destroy-%s-*.tfplan", game))
	if err != nil {
		return fmt.Errorf("creating plan file: %w", err)
	}
	planFile.Close()
	planPath := planFile.Name()
	defer os.Remove(planPath)

	fmt.Printf("\nStep 3/3: Planning %s infrastructure destruction...\n", game)
	if err := tfPlanDestroyFn(dir, planPath); err != nil {
		return fmt.Errorf("terraform plan -destroy failed: %w", err)
	}

	fmt.Printf("\nType the game name to confirm destroy: ")
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

	fmt.Printf("\n✓ %s retired (saves archived, infrastructure destroyed)\n", game)
	return nil
}
