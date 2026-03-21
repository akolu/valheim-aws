package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFindRepoRoot(t *testing.T) {
	// Set BONFIRE_REPO_ROOT to the test fixtures directory
	repoRoot := t.TempDir()
	gamesDir := filepath.Join(repoRoot, "terraform", "games")
	if err := os.MkdirAll(gamesDir, 0755); err != nil {
		t.Fatal(err)
	}

	t.Setenv("BONFIRE_REPO_ROOT", repoRoot)
	got, err := findRepoRoot()
	if err != nil {
		t.Fatalf("findRepoRoot() error: %v", err)
	}
	if got != repoRoot {
		t.Errorf("findRepoRoot() = %q, want %q", got, repoRoot)
	}
}

func TestTerraformDir_GameNotFound(t *testing.T) {
	repoRoot := t.TempDir()
	gamesDir := filepath.Join(repoRoot, "terraform", "games")
	if err := os.MkdirAll(gamesDir, 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("BONFIRE_REPO_ROOT", repoRoot)

	_, err := terraformDir("nonexistent-game")
	if err == nil {
		t.Error("terraformDir() expected error for nonexistent game, got nil")
	}
}

func TestTerraformDir_GameFound(t *testing.T) {
	repoRoot := t.TempDir()
	gameDir := filepath.Join(repoRoot, "terraform", "games", "valheim")
	if err := os.MkdirAll(gameDir, 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("BONFIRE_REPO_ROOT", repoRoot)

	got, err := terraformDir("valheim")
	if err != nil {
		t.Fatalf("terraformDir() error: %v", err)
	}
	if got != gameDir {
		t.Errorf("terraformDir() = %q, want %q", got, gameDir)
	}
}

func TestAvailableGames(t *testing.T) {
	repoRoot := t.TempDir()
	for _, game := range []string{"valheim", "satisfactory"} {
		dir := filepath.Join(repoRoot, "terraform", "games", game)
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("BONFIRE_REPO_ROOT", repoRoot)

	games, err := availableGames()
	if err != nil {
		t.Fatalf("availableGames() error: %v", err)
	}
	if len(games) != 2 {
		t.Errorf("availableGames() returned %d games, want 2: %v", len(games), games)
	}
}

func TestBucketNames(t *testing.T) {
	tests := []struct {
		game    string
		region  string
		backup  string
		longterm string
	}{
		{"valheim", "eu-north-1", "bonfire-valheim-backups-eu-north-1", "valheim-long-term-backups"},
		{"satisfactory", "us-east-1", "bonfire-satisfactory-backups-us-east-1", "satisfactory-long-term-backups"},
	}
	for _, tt := range tests {
		if got := backupBucketName(tt.game, tt.region); got != tt.backup {
			t.Errorf("backupBucketName(%q, %q) = %q, want %q", tt.game, tt.region, got, tt.backup)
		}
		if got := longtermBucketName(tt.game); got != tt.longterm {
			t.Errorf("longtermBucketName(%q) = %q, want %q", tt.game, got, tt.longterm)
		}
	}
}
