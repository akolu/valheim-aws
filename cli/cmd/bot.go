package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
)

const discordAPIBase = "https://discord.com/api/v10"

var botCmd = &cobra.Command{
	Use:   "bot",
	Short: "Manage Discord bot configuration",
}

var botDeployCmd = &cobra.Command{
	Use:   "deploy",
	Short: "Build, deploy, and register the Discord bot",
	Long: `Build the Lambda binary, apply terraform/bot/, and register Discord commands.

Runs the full deployment pipeline in sequence:
  1. make build        — compiles the Go Lambda binary and packages it into a zip
  2. terraform apply   — deploys the Lambda and API Gateway infrastructure
  3. bot update        — registers slash commands and sets the interaction endpoint

Requires AWS_PROFILE=bonfire-deploy (or equivalent credentials in the environment).`,
	Args: cobra.NoArgs,
	RunE: runBotDeploy,
}

var botUpdateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update Discord bot interaction endpoint and slash commands",
	Long: `Update the Discord bot interaction endpoint URL and register slash commands.
Safe to re-run at any time — only calls the Discord API if the endpoint or commands have changed.

Reads credentials from terraform/bot/terraform.tfvars and endpoint from terraform/bot/ output.
Registers commands for all games in a single global PUT.`,
	Args: cobra.NoArgs,
	RunE: runBotUpdate,
}

func init() {
	botCmd.AddCommand(botDeployCmd)
	botCmd.AddCommand(botUpdateCmd)
}

func runBotDeploy(cmd *cobra.Command, args []string) error {
	if err := checkBotDeployed(); err != nil {
		return err
	}
	root, err := findRepoRoot()
	if err != nil {
		return err
	}

	fmt.Println("==> Building Lambda binary...")
	makeCmd := exec.Command("make", "build")
	makeCmd.Dir = filepath.Join(root, "discord_bot", "go")
	makeCmd.Stdout = os.Stdout
	makeCmd.Stderr = os.Stderr
	if err := makeCmd.Run(); err != nil {
		return fmt.Errorf("make build: %w", err)
	}

	fmt.Println("==> Applying terraform/bot/...")
	botDir := filepath.Join(root, "terraform", "bot")
	if err := runTerraform(botDir, "apply"); err != nil {
		return err
	}

	fmt.Println("==> Registering Discord commands...")
	return runBotUpdate(cmd, args)
}

// checkBotDeployed returns a friendly error if terraform/bot/terraform.tfvars
// does not exist in the repo root — indicating the bot has not been deployed yet.
func checkBotDeployed() error {
	root, err := findRepoRoot()
	if err != nil {
		return err
	}
	tfvarsPath := filepath.Join(root, "terraform", "bot", "terraform.tfvars")
	if _, err := os.Stat(tfvarsPath); os.IsNotExist(err) {
		return fmt.Errorf("Bot not deployed yet — see terraform/bot/ for first-time setup.")
	}
	return nil
}

func runBotUpdate(cmd *cobra.Command, args []string) error {
	if err := checkBotDeployed(); err != nil {
		return err
	}
	root, err := findRepoRoot()
	if err != nil {
		return err
	}

	botDir := filepath.Join(root, "terraform", "bot")

	creds, err := readBotCreds(botDir)
	if err != nil {
		return err
	}

	outputs, err := terraformOutput(botDir)
	if err != nil {
		return fmt.Errorf("getting terraform output: %w", err)
	}
	endpointOutput, ok := outputs["discord_bot_endpoint"]
	if !ok {
		return fmt.Errorf("terraform output discord_bot_endpoint not found — bot may not be deployed")
	}
	endpoint := endpointOutput.String()
	if endpoint == "" {
		return fmt.Errorf("terraform output discord_bot_endpoint is empty — bot may not be deployed")
	}

	games, err := availableGames()
	if err != nil {
		return err
	}

	client := &http.Client{Timeout: 15 * time.Second}

	if err := updateInteractionEndpoint(client, creds, endpoint); err != nil {
		return err
	}

	return registerAllCommands(client, creds, games)
}

// readBotCreds reads Discord credentials from terraform/bot/terraform.tfvars.
func readBotCreds(dir string) (discordCreds, error) {
	tfvarsPath := filepath.Join(dir, "terraform.tfvars")
	vals, err := parseTFVars(tfvarsPath)
	if err != nil {
		return discordCreds{}, fmt.Errorf("reading terraform/bot/terraform.tfvars: %w", err)
	}
	creds := discordCreds{
		applicationID: vals["discord_application_id"],
		botToken:      vals["discord_bot_token"],
		publicKey:     vals["discord_public_key"],
	}
	if creds.applicationID == "" || creds.botToken == "" || creds.publicKey == "" {
		return discordCreds{}, fmt.Errorf("missing credentials in terraform/bot/terraform.tfvars (need discord_application_id, discord_bot_token, discord_public_key)")
	}
	return creds, nil
}

type discordCreds struct {
	applicationID string
	botToken      string
	publicKey     string
}

// httpClient is an interface for making HTTP requests (enables testing).
type httpClient interface {
	Do(req *http.Request) (*http.Response, error)
}

// updateInteractionEndpoint checks the current endpoint via GET /applications/@me
// and patches it if different.
func updateInteractionEndpoint(client httpClient, creds discordCreds, newEndpoint string) error {
	type appResponse struct {
		InteractionsEndpointURL string `json:"interactions_endpoint_url"`
	}

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
		fmt.Println("Interaction endpoint unchanged — no-op")
		return nil
	}

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

	fmt.Printf("✓ Interaction endpoint updated: %s\n", newEndpoint)
	return nil
}

type discordCommand struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Options     []discordCommandOption `json:"options,omitempty"`
}

type discordCommandOption struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Type        int    `json:"type"` // 1 = SUB_COMMAND
}

// registerAllCommands registers /<game> for each game via a single global PUT.
// Each game command includes status/start/stop/help/hello subcommands.
// First checks current commands via GET and skips the PUT if unchanged.
func registerAllCommands(client httpClient, creds discordCreds, games []string) error {
	var commands []discordCommand
	for _, game := range games {
		commands = append(commands, discordCommand{
			Name:        game,
			Description: fmt.Sprintf("Control the %s server", game),
			Options: []discordCommandOption{
				{Name: "status", Description: fmt.Sprintf("Check if the %s server is running", game), Type: 1},
				{Name: "start", Description: fmt.Sprintf("Start the %s server", game), Type: 1},
				{Name: "stop", Description: fmt.Sprintf("Stop the %s server", game), Type: 1},
				{Name: "help", Description: fmt.Sprintf("Show available commands for the %s server", game), Type: 1},
				{Name: "hello", Description: "Check if the bot is reachable and verify your authorization status", Type: 1},
			},
		})
	}

	url := fmt.Sprintf("%s/applications/%s/commands", discordAPIBase, creds.applicationID)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bot "+creds.botToken)

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("GET commands: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("GET commands: HTTP %d: %s", resp.StatusCode, body)
	}

	var current []discordCommand
	if err := json.NewDecoder(resp.Body).Decode(&current); err != nil {
		return fmt.Errorf("parsing GET commands response: %w", err)
	}

	if !commandsChanged(current, commands) {
		fmt.Printf("Slash commands unchanged — no-op (global, %d games)\n", len(games))
		return nil
	}

	payload, err := json.Marshal(commands)
	if err != nil {
		return err
	}

	req, err = http.NewRequest("PUT", url, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bot "+creds.botToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err = client.Do(req)
	if err != nil {
		return fmt.Errorf("PUT commands: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("PUT commands: HTTP %d: %s", resp.StatusCode, body)
	}

	fmt.Printf("✓ Slash commands registered (global, %d games)\n", len(games))
	return nil
}

// commandsChanged reports whether the desired commands differ from current
// in terms of command names, descriptions, and subcommand option names and descriptions.
func commandsChanged(current, desired []discordCommand) bool {
	if len(current) != len(desired) {
		return true
	}
	currentByName := make(map[string]discordCommand, len(current))
	for _, c := range current {
		currentByName[c.Name] = c
	}
	for _, d := range desired {
		cur, ok := currentByName[d.Name]
		if !ok {
			return true
		}
		if cur.Description != d.Description {
			return true
		}
		if len(cur.Options) != len(d.Options) {
			return true
		}
		curOptsByName := make(map[string]discordCommandOption, len(cur.Options))
		for _, o := range cur.Options {
			curOptsByName[o.Name] = o
		}
		for _, o := range d.Options {
			curOpt, found := curOptsByName[o.Name]
			if !found || curOpt.Description != o.Description {
				return true
			}
		}
	}
	return false
}
