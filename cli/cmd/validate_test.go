package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// setupGamesDir creates a temp repo root with the given game directories and
// sets BONFIRE_REPO_ROOT so availableGames() uses it.
func setupGamesDir(t *testing.T, games []string) {
	t.Helper()
	repoRoot := t.TempDir()
	for _, game := range games {
		dir := filepath.Join(repoRoot, "terraform", "games", game)
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("BONFIRE_REPO_ROOT", repoRoot)
}

func TestValidateGameName(t *testing.T) {
	setupGamesDir(t, []string{"valheim", "satisfactory"})

	// Format-valid names that are also in the available games list.
	valid := []string{"valheim", "satisfactory"}
	for _, name := range valid {
		if err := validateGameName(name); err != nil {
			t.Errorf("validateGameName(%q) returned unexpected error: %v", name, err)
		}
	}

	// Format-invalid names: rejected before the availability check.
	formatInvalid := []string{
		"",
		"Valheim",
		"my_game",
		"my game",
		"../etc",
		"game!",
		"GAME",
		"game name",
	}
	for _, name := range formatInvalid {
		if err := validateGameName(name); err == nil {
			t.Errorf("validateGameName(%q) expected error, got nil", name)
		}
	}

	// Format-valid but not in the available games list.
	unknownGames := []string{"minecraft", "my-game", "game1", "a1b2-c3"}
	for _, name := range unknownGames {
		err := validateGameName(name)
		if err == nil {
			t.Errorf("validateGameName(%q) expected error for unknown game, got nil", name)
			continue
		}
		if !strings.Contains(err.Error(), "unknown game") {
			t.Errorf("validateGameName(%q) error = %q, want it to contain %q", name, err.Error(), "unknown game")
		}
	}
}

func TestValidateGameName_HelpfulError(t *testing.T) {
	setupGamesDir(t, []string{"factorio", "satisfactory", "valheim"})

	err := validateGameName("bonfire")
	if err == nil {
		t.Fatal("validateGameName(\"bonfire\") expected error, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, `"bonfire"`) {
		t.Errorf("error missing game name: %q", msg)
	}
	for _, game := range []string{"factorio", "satisfactory", "valheim"} {
		if !strings.Contains(msg, game) {
			t.Errorf("error missing available game %q: %q", game, msg)
		}
	}
}

func TestValidateDiscordID(t *testing.T) {
	valid := []string{"123456789012345678", "1", "999999999999999999"}
	for _, id := range valid {
		if err := validateDiscordID(id, "user_id"); err != nil {
			t.Errorf("validateDiscordID(%q) returned unexpected error: %v", id, err)
		}
	}

	invalid := []string{
		"",
		"abc",
		"123abc",
		"12.34",
		"-1",
		"123 456",
	}
	for _, id := range invalid {
		if err := validateDiscordID(id, "user_id"); err == nil {
			t.Errorf("validateDiscordID(%q) expected error, got nil", id)
		}
	}
}
