package cmd

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/spf13/cobra"
)

var archiveCmd = &cobra.Command{
	Use:   "archive <game>",
	Short: "Copy current saves to long-term bucket (does not destroy)",
	Long: `Copy all save files from the game's backup S3 bucket to the long-term
bucket. Does NOT destroy the server. Use 'retire' to archive and destroy.`,
	Args: cobra.ExactArgs(1),
	RunE: runArchive,
}

func runArchive(cmd *cobra.Command, args []string) error {
	game := args[0]
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cfg, err := awsConfig(ctx)
	if err != nil {
		return fmt.Errorf("loading AWS config: %w", err)
	}
	return archiveGame(ctx, s3.NewFromConfig(cfg), cfg.Region, game)
}

// archiveGame copies all objects from the game's backup bucket to the long-term bucket.
// It prefixes the destination keys with a timestamp to avoid collisions.
func archiveGame(ctx context.Context, s3Client s3API, region, game string) error {
	srcBucket := backupBucketName(game, region)
	dstBucket := longtermBucketName(game)
	timestamp := time.Now().UTC().Format("2006-01-02T150405Z")

	fmt.Printf("Archiving %s saves...\n", game)
	fmt.Printf("  Source: s3://%s\n", srcBucket)
	fmt.Printf("  Dest:   s3://%s/%s/\n", dstBucket, timestamp)

	keys, err := listObjects(ctx, s3Client, srcBucket, "")
	if err != nil {
		return fmt.Errorf("listing backup objects: %w", err)
	}
	if len(keys) == 0 {
		fmt.Println("No objects found in backup bucket — nothing to archive.")
		return nil
	}

	var copied, skipped int
	for _, key := range keys {
		// Skip empty "directory" objects (trailing slash)
		if strings.HasSuffix(key, "/") {
			skipped++
			continue
		}
		dstKey := timestamp + "/" + key
		fmt.Printf("  Copying %-60s → %s\n", key, dstKey)
		if err := copyObject(ctx, s3Client, srcBucket, key, dstBucket, dstKey); err != nil {
			return err
		}
		copied++
	}

	fmt.Printf("✓ Archived %d objects (%d skipped)\n", copied, skipped)
	return nil
}
