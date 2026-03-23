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
	tfPlanFn = terraformPlan
	tfInitFn = terraformInit
)

var provisionCmd = &cobra.Command{
	Use:   "provision <game>",
	Short: "Provision a game server (terraform init + plan + apply)",
	Long: `Provision a game server by running terraform init, plan, and apply for the
specified game workspace. The terraform plan is shown for review before any
infrastructure changes are made. If an archive exists in the long-term bucket,
the latest save is selected automatically and its location is printed. If no
archive exists, the server starts fresh.`,
	Args: cobra.ExactArgs(1),
	RunE: runProvision,
}

func runProvision(cmd *cobra.Command, args []string) error {
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
	return provisionGame(ctx, s3.NewFromConfig(cfg), cfg.Region, game, os.Stdin)
}

// provisionGame runs terraform init, plans, prompts for confirmation, then applies.
// Accepts a client and stdin for testability.
func provisionGame(ctx context.Context, s3Client s3API, region, game string, stdin io.Reader) error {
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
	return autoRestoreFromLongterm(ctx, s3Client, game, region)
}

// autoRestoreFromLongterm checks for an existing archive in the long-term bucket
// and, if found, copies the latest snapshot into the short-term backup bucket so
// the game server's existing restore mechanism picks it up on first boot.
// No user interaction is required — the latest snapshot is selected automatically.
func autoRestoreFromLongterm(ctx context.Context, s3Client s3API, game, region string) error {
	ltBucket := longtermBucketName(game)
	fmt.Printf("\nChecking for existing archive in s3://%s...\n", ltBucket)

	latestKey, err := latestObjectByPrefix(ctx, s3Client, ltBucket, "")
	if err != nil {
		return fmt.Errorf("checking for archive: %w", err)
	}

	if latestKey == "" {
		fmt.Println("No archive found — starting fresh.")
		return nil
	}

	// Extract the timestamp prefix (first path segment) and reconstruct as a
	// directory prefix (with trailing slash) for listing.
	archivePrefix := strings.SplitN(latestKey, "/", 2)[0] + "/"
	fmt.Printf("Restoring from long-term backup: s3://%s/%s\n", ltBucket, archivePrefix)

	keys, err := listObjects(ctx, s3Client, ltBucket, archivePrefix)
	if err != nil {
		return fmt.Errorf("listing archive objects: %w", err)
	}

	dstBucket := backupBucketName(game, region)
	fmt.Printf("  Copying to s3://%s...\n", dstBucket)

	for _, key := range keys {
		// Strip the timestamp prefix to get the short-term bucket key.
		dstKey := strings.TrimPrefix(key, archivePrefix)
		if dstKey == "" || strings.HasSuffix(key, "/") {
			continue
		}
		if err := copyObject(ctx, s3Client, ltBucket, key, dstBucket, dstKey); err != nil {
			return err
		}
	}

	fmt.Printf("✓ Long-term backup restored to s3://%s\n", dstBucket)
	return nil
}
