package cmd

import (
	"fmt"
	"regexp"
	"strconv"
)

var gameNameRe = regexp.MustCompile(`^[a-z0-9-]+$`)

// validateGameName checks that game is safe to interpolate into S3 bucket names,
// SSM paths, and filesystem paths. Valid names match ^[a-z0-9-]+$.
func validateGameName(game string) error {
	if game == "" {
		return fmt.Errorf("game name must not be empty")
	}
	if !gameNameRe.MatchString(game) {
		return fmt.Errorf("invalid game name %q: must match ^[a-z0-9-]+$ (lowercase letters, digits, and hyphens only)", game)
	}
	return nil
}

// validateDiscordID checks that id is a valid Discord snowflake (numeric, non-empty).
func validateDiscordID(id, kind string) error {
	if id == "" {
		return fmt.Errorf("%s must not be empty", kind)
	}
	if _, err := strconv.ParseUint(id, 10, 64); err != nil {
		return fmt.Errorf("invalid %s %q: Discord IDs are numeric snowflakes (e.g. 123456789012345678)", kind, id)
	}
	return nil
}
