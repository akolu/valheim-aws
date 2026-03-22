package cmd

import (
	"testing"
)

func TestValidateGameName(t *testing.T) {
	valid := []string{"valheim", "minecraft", "my-game", "game1", "a1b2-c3"}
	for _, name := range valid {
		if err := validateGameName(name); err != nil {
			t.Errorf("validateGameName(%q) returned unexpected error: %v", name, err)
		}
	}

	invalid := []string{
		"",
		"Valheim",
		"my_game",
		"my game",
		"../etc",
		"game!",
		"GAME",
		"game name",
	}
	for _, name := range invalid {
		if err := validateGameName(name); err == nil {
			t.Errorf("validateGameName(%q) expected error, got nil", name)
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
