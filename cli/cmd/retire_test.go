package cmd

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/s3"
)

func TestRetireGame_ArchiveFails(t *testing.T) {
	client := &mockS3{
		listFunc: func(_ context.Context, _ *s3.ListObjectsV2Input, _ ...func(*s3.Options)) (*s3.ListObjectsV2Output, error) {
			return nil, errors.New("NoSuchBucket")
		},
	}
	err := retireGame(context.Background(), client, "eu-north-1", "valheim", strings.NewReader(""))
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
	err := retireGame(context.Background(), client, "eu-north-1", "nonexistent-game", strings.NewReader(""))
	if err == nil {
		t.Fatal("expected error for missing terraform dir, got nil")
	}
}

func TestRetireGame_ConfirmationAbort(t *testing.T) {
	repoRoot := t.TempDir()
	gameDir := filepath.Join(repoRoot, "terraform", "games", "valheim")
	if err := os.MkdirAll(gameDir, 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("BONFIRE_REPO_ROOT", repoRoot)

	// Mock plan to succeed without running real terraform
	oldPlan := tfPlanDestroyFn
	tfPlanDestroyFn = func(dir, planFile string) error { return nil }
	defer func() { tfPlanDestroyFn = oldPlan }()

	client := &mockS3{}
	err := retireGame(context.Background(), client, "eu-north-1", "valheim", strings.NewReader("wrong-name\n"))
	if err != nil {
		t.Fatalf("expected no error on abort, got: %v", err)
	}
}

func TestRetireGame_ConfirmationMatch(t *testing.T) {
	repoRoot := t.TempDir()
	gameDir := filepath.Join(repoRoot, "terraform", "games", "valheim")
	if err := os.MkdirAll(gameDir, 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("BONFIRE_REPO_ROOT", repoRoot)

	var planCalled, applyCalled bool
	oldPlan := tfPlanDestroyFn
	oldApply := tfApplyPlanFn
	tfPlanDestroyFn = func(dir, planFile string) error { planCalled = true; return nil }
	tfApplyPlanFn = func(dir, planFile string) error { applyCalled = true; return nil }
	defer func() { tfPlanDestroyFn = oldPlan; tfApplyPlanFn = oldApply }()

	client := &mockS3{}
	err := retireGame(context.Background(), client, "eu-north-1", "valheim", strings.NewReader("valheim\n"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !planCalled {
		t.Error("expected terraform plan -destroy to be called")
	}
	if !applyCalled {
		t.Error("expected terraform apply to be called")
	}
}
