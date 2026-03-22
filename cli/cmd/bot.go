package cmd

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

const discordAPIBase = "https://discord.com/api/v10"

var botCmd = &cobra.Command{
	Use:   "bot",
	Short: "Manage Discord bot configuration",
}

var botUpdateCmd = &cobra.Command{
	Use:   "update [game]",
	Short: "Update Discord bot interaction endpoint and slash commands",
	Long: `Update the Discord bot interaction endpoint URL and register slash commands.
Safe to re-run at any time — only calls the Discord API if the endpoint has changed.

Without a game argument, updates all games that have enable_discord_bot=true and
Discord credentials configured. With a game argument, targets that game only.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runBotUpdate,
}

func init() {
	botCmd.AddCommand(botUpdateCmd)
}

func runBotUpdate(cmd *cobra.Command, args []string) error {
	var games []string
	if len(args) == 1 {
		games = []string{args[0]}
	} else {
		root, err := findRepoRoot()
		if err != nil {
			return err
		}
		gamesDir := filepath.Join(root, "terraform", "games")
		entries, err := os.ReadDir(gamesDir)
		if err != nil {
			return fmt.Errorf("reading games directory: %w", err)
		}
		for _, e := range entries {
			if e.IsDir() {
				games = append(games, e.Name())
			}
		}
	}

	client := &http.Client{Timeout: 15 * time.Second}
	anyUpdated := false
	for _, game := range games {
		updated, err := botUpdateGame(client, game)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  [%s] error: %v\n", game, err)
			continue
		}
		if updated {
			anyUpdated = true
		}
	}
	if !anyUpdated && len(args) == 0 {
		fmt.Println("No games with Discord bot configured.")
	}
	return nil
}

// botUpdateGame updates the Discord bot for a single game.
// Returns true if any Discord API calls were made.
func botUpdateGame(client httpClient, game string) (bool, error) {
	dir, err := terraformDir(game)
	if err != nil {
		return false, err
	}

	creds, err := readDiscordCreds(dir)
	if err != nil {
		return false, err
	}

	if !creds.enabled {
		return false, nil
	}

	if creds.applicationID == "" || creds.botToken == "" || creds.publicKey == "" {
		return false, fmt.Errorf("enable_discord_bot=true but missing credentials (need discord_application_id, discord_bot_token, discord_public_key)")
	}

	fmt.Printf("[%s] Updating Discord bot...\n", game)

	// Get current endpoint from Terraform output
	endpoint, err := terraformOutputRaw(dir, "discord_bot_endpoint")
	if err != nil {
		return false, fmt.Errorf("getting terraform output discord_bot_endpoint: %w", err)
	}
	if endpoint == "" {
		return false, fmt.Errorf("terraform output discord_bot_endpoint is empty — bot may not be deployed")
	}

	// Update interaction endpoint (no-op if unchanged)
	if err := updateInteractionEndpoint(client, creds, endpoint, game); err != nil {
		return true, err
	}

	// Register slash commands
	if err := registerSlashCommands(client, creds, game); err != nil {
		return true, err
	}

	return true, nil
}

type discordCreds struct {
	enabled       bool
	applicationID string
	botToken      string
	publicKey     string
	guildID       string
}

// readDiscordCreds parses discord credentials from terraform.tfvars in dir.
// Falls back to discord_bot/.env for DISCORD_GUILD_ID if not in tfvars.
func readDiscordCreds(dir string) (discordCreds, error) {
	tfvarsPath := filepath.Join(dir, "terraform.tfvars")
	vals, err := parseTFVars(tfvarsPath)
	if err != nil {
		if os.IsNotExist(err) {
			return discordCreds{}, nil
		}
		return discordCreds{}, fmt.Errorf("reading terraform.tfvars: %w", err)
	}

	creds := discordCreds{
		enabled:       vals["enable_discord_bot"] == "true",
		applicationID: vals["discord_application_id"],
		botToken:      vals["discord_bot_token"],
		publicKey:     vals["discord_public_key"],
		guildID:       vals["discord_guild_id"],
	}

	// Fallback: try discord_bot/.env for DISCORD_GUILD_ID
	if creds.guildID == "" {
		root, _ := findRepoRoot()
		if root != "" {
			envPath := filepath.Join(root, "discord_bot", ".env")
			envVals, _ := parseDotEnv(envPath)
			creds.guildID = envVals["DISCORD_GUILD_ID"]
		}
	}

	return creds, nil
}

// parseTFVars parses key = "value" or key = value lines from a .tfvars file.
func parseTFVars(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return parseTFVarsReader(f), nil
}

func parseTFVarsReader(r io.Reader) map[string]string {
	result := make(map[string]string)
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		idx := strings.Index(line, "=")
		if idx < 0 {
			continue
		}
		key := strings.TrimSpace(line[:idx])
		val := strings.TrimSpace(line[idx+1:])
		// Strip surrounding quotes
		if len(val) >= 2 && val[0] == '"' && val[len(val)-1] == '"' {
			val = val[1 : len(val)-1]
		}
		result[key] = val
	}
	return result
}

// parseDotEnv parses KEY=value lines from a .env file.
func parseDotEnv(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	result := make(map[string]string)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		idx := strings.Index(line, "=")
		if idx < 0 {
			continue
		}
		key := strings.TrimSpace(line[:idx])
		val := strings.TrimSpace(line[idx+1:])
		// Strip surrounding quotes
		if len(val) >= 2 && (val[0] == '"' || val[0] == '\'') && val[len(val)-1] == val[0] {
			val = val[1 : len(val)-1]
		}
		result[key] = val
	}
	return result, nil
}

// terraformOutputRaw runs `terraform output -raw <name>` in dir and returns the value.
func terraformOutputRaw(dir, name string) (string, error) {
	cmd := exec.Command("terraform", "output", "-raw", name)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// httpClient is an interface for making HTTP requests (enables testing).
type httpClient interface {
	Do(req *http.Request) (*http.Response, error)
}

// updateInteractionEndpoint checks the current endpoint via GET /applications/@me
// and patches it if different.
func updateInteractionEndpoint(client httpClient, creds discordCreds, newEndpoint, game string) error {
	type appResponse struct {
		InteractionsEndpointURL string `json:"interactions_endpoint_url"`
	}

	// GET current endpoint
	req, err := http.NewRequest("GET", discordAPIBase+"/applications/@me", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bot "+creds.botToken)

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("GET /applications/@me: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("GET /applications/@me: HTTP %d: %s", resp.StatusCode, body)
	}

	var current appResponse
	if err := json.NewDecoder(resp.Body).Decode(&current); err != nil {
		return fmt.Errorf("parsing GET /applications/@me response: %w", err)
	}

	if current.InteractionsEndpointURL == newEndpoint {
		fmt.Printf("  [%s] Interaction endpoint unchanged — no-op\n", game)
		return nil
	}

	// PATCH to update endpoint
	payload, _ := json.Marshal(map[string]string{
		"interactions_endpoint_url": newEndpoint,
	})
	req, err = http.NewRequest("PATCH", discordAPIBase+"/applications/"+creds.applicationID, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bot "+creds.botToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err = client.Do(req)
	if err != nil {
		return fmt.Errorf("PATCH /applications/%s: %w", creds.applicationID, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("PATCH /applications/%s: HTTP %d: %s", creds.applicationID, resp.StatusCode, body)
	}

	fmt.Printf("  [%s] ✓ Interaction endpoint updated: %s\n", game, newEndpoint)
	return nil
}

type discordCommand struct {
	Name        string                   `json:"name"`
	Description string                   `json:"description"`
	Options     []discordCommandOption   `json:"options,omitempty"`
}

type discordCommandOption struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Type        int    `json:"type"` // 1 = SUB_COMMAND
}

// registerSlashCommands registers /hello and /<game> via PUT bulk endpoint.
func registerSlashCommands(client httpClient, creds discordCreds, game string) error {
	commands := []discordCommand{
		{
			Name:        "hello",
			Description: "Check if the bot is reachable and verify your authorization status",
		},
		{
			Name:        game,
			Description: fmt.Sprintf("Control the %s server", game),
			Options: []discordCommandOption{
				{Name: "status", Description: fmt.Sprintf("Check if the %s server is running", game), Type: 1},
				{Name: "start", Description: fmt.Sprintf("Start the %s server", game), Type: 1},
				{Name: "stop", Description: fmt.Sprintf("Stop the %s server", game), Type: 1},
				{Name: "help", Description: fmt.Sprintf("Show available commands for the %s server", game), Type: 1},
			},
		},
	}

	payload, err := json.Marshal(commands)
	if err != nil {
		return err
	}

	var url string
	if creds.guildID != "" {
		url = fmt.Sprintf("%s/applications/%s/guilds/%s/commands", discordAPIBase, creds.applicationID, creds.guildID)
	} else {
		url = fmt.Sprintf("%s/applications/%s/commands", discordAPIBase, creds.applicationID)
	}

	req, err := http.NewRequest("PUT", url, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bot "+creds.botToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("PUT commands: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("PUT commands: HTTP %d: %s", resp.StatusCode, body)
	}

	scope := "global"
	if creds.guildID != "" {
		scope = "guild " + creds.guildID
	}
	fmt.Printf("  [%s] ✓ Slash commands registered (%s)\n", game, scope)
	return nil
}
