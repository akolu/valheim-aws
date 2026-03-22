package cmd

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/spf13/cobra"
)

var provisionCmd = &cobra.Command{
	Use:   "provision <game>",
	Short: "Provision a game server (terraform init + apply)",
	Long: `Provision a game server by running terraform init and apply for the
specified game workspace. If an archive exists in the long-term bucket, the
latest save is selected automatically and its location is printed. If no
archive exists, the server starts fresh.`,
	Args: cobra.ExactArgs(1),
	RunE: runProvision,
}

func runProvision(cmd *cobra.Command, args []string) error {
	game := args[0]
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	dir, err := terraformDir(game)
	if err != nil {
		return err
	}

	fmt.Printf("Provisioning %s...\n", game)
	if err := terraformInit(dir); err != nil {
		return err
	}
	if err := terraformApply(dir); err != nil {
		return err
	}
	fmt.Printf("✓ %s provisioned\n", game)

	cfg, err := awsConfig(ctx)
	if err != nil {
		return fmt.Errorf("loading AWS config: %w", err)
	}
	return autoRestoreFromLongterm(ctx, s3.NewFromConfig(cfg), game)
}

// autoRestoreFromLongterm checks for an existing archive in the long-term bucket
// and prints the location of the latest save if one is found. No user interaction
// is required — if an archive exists, the latest is selected automatically.
func autoRestoreFromLongterm(ctx context.Context, s3Client s3API, game string) error {
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

	// Extract the timestamp prefix (first path segment).
	archivePrefix := strings.SplitN(latestKey, "/", 2)[0]
	fmt.Printf("✓ Archive found: %s\n", archivePrefix)
	fmt.Printf("  Source: s3://%s/%s/\n", ltBucket, archivePrefix)
	fmt.Println("  (Copy files from this prefix to restore last known state.)")
	return nil
}
