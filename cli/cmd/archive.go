package cmd

import (
	"context"
	"fmt"
	"strings"
	"time"
)

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
