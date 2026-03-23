package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProvisionGame_TerraformDirNotFound(t *testing.T) {
	repoRoot := t.TempDir()
	gamesDir := filepath.Join(repoRoot, "terraform", "games")
	if err := os.MkdirAll(gamesDir, 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("BONFIRE_REPO_ROOT", repoRoot)

	err := provisionGame("nonexistent-game", strings.NewReader(""))
	if err == nil {
		t.Fatal("expected error for missing terraform dir, got nil")
	}
}

func TestProvisionGame_ConfirmationAbort(t *testing.T) {
	repoRoot := t.TempDir()
	gameDir := filepath.Join(repoRoot, "terraform", "games", "valheim")
	if err := os.MkdirAll(gameDir, 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("BONFIRE_REPO_ROOT", repoRoot)

	oldInit := tfInitFn
	oldPlan := tfPlanFn
	tfInitFn = func(dir string) error { return nil }
	tfPlanFn = func(dir, planFile string) error { return nil }
	defer func() { tfInitFn = oldInit; tfPlanFn = oldPlan }()

	err := provisionGame("valheim", strings.NewReader("wrong-name\n"))
	if err != nil {
		t.Fatalf("expected no error on abort, got: %v", err)
	}
}

func TestProvisionGame_ConfirmationMatch(t *testing.T) {
	repoRoot := t.TempDir()
	gameDir := filepath.Join(repoRoot, "terraform", "games", "valheim")
	if err := os.MkdirAll(gameDir, 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("BONFIRE_REPO_ROOT", repoRoot)

	var initCalled, planCalled, applyCalled bool
	oldInit := tfInitFn
	oldPlan := tfPlanFn
	oldApply := tfApplyPlanFn
	tfInitFn = func(dir string) error { initCalled = true; return nil }
	tfPlanFn = func(dir, planFile string) error { planCalled = true; return nil }
	tfApplyPlanFn = func(dir, planFile string) error { applyCalled = true; return nil }
	defer func() { tfInitFn = oldInit; tfPlanFn = oldPlan; tfApplyPlanFn = oldApply }()

	err := provisionGame("valheim", strings.NewReader("valheim\n"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !initCalled {
		t.Error("expected terraform init to be called")
	}
	if !planCalled {
		t.Error("expected terraform plan to be called")
	}
	if !applyCalled {
		t.Error("expected terraform apply to be called")
	}
}
