package cmd

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/s3"
)

func TestRetireGame_ArchiveFails(t *testing.T) {
	client := &mockS3{
		listFunc: func(_ context.Context, _ *s3.ListObjectsV2Input, _ ...func(*s3.Options)) (*s3.ListObjectsV2Output, error) {
			return nil, errors.New("NoSuchBucket")
		},
	}
	err := retireGame(context.Background(), client, "eu-north-1", "valheim")
	if err == nil {
		t.Fatal("expected error when archive fails, got nil")
	}
}

func TestRetireGame_TerraformDirNotFound(t *testing.T) {
	// Archive succeeds (empty bucket), but terraform dir doesn't exist
	repoRoot := t.TempDir()
	gamesDir := filepath.Join(repoRoot, "terraform", "games")
	if err := os.MkdirAll(gamesDir, 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("BONFIRE_REPO_ROOT", repoRoot)

	client := &mockS3{} // empty bucket — archive succeeds with nothing to copy
	err := retireGame(context.Background(), client, "eu-north-1", "nonexistent-game")
	if err == nil {
		t.Fatal("expected error for missing terraform dir, got nil")
	}
}
