package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// mockHTTPClient implements httpClient for testing Discord API calls.
type mockHTTPClient struct {
	responses []*http.Response
	errors    []error
	requests  []*http.Request
	idx       int
}

func (m *mockHTTPClient) Do(req *http.Request) (*http.Response, error) {
	m.requests = append(m.requests, req)
	i := m.idx
	m.idx++
	if i < len(m.errors) && m.errors[i] != nil {
		return nil, m.errors[i]
	}
	if i < len(m.responses) {
		return m.responses[i], nil
	}
	return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("{}"))}, nil
}

func jsonBody(v interface{}) io.ReadCloser {
	b, _ := json.Marshal(v)
	return io.NopCloser(strings.NewReader(string(b)))
}

// --- commandsChanged tests ---

func TestCommandsChanged_Identical(t *testing.T) {
	cmds := []discordCommand{
		{Name: "valheim", Description: "Control the valheim server", Options: []discordCommandOption{
			{Name: "status", Description: "Check if the valheim server is running", Type: 1},
		}},
	}
	if commandsChanged(cmds, cmds) {
		t.Error("identical commands should not be changed")
	}
}

func TestCommandsChanged_DifferentOrder(t *testing.T) {
	a := []discordCommand{
		{Name: "valheim", Description: "desc", Options: []discordCommandOption{{Name: "status", Description: "d1", Type: 1}}},
		{Name: "minecraft", Description: "desc2", Options: []discordCommandOption{{Name: "start", Description: "d2", Type: 1}}},
	}
	// desired in reverse order — structurally the same set
	b := []discordCommand{
		{Name: "minecraft", Description: "desc2", Options: []discordCommandOption{{Name: "start", Description: "d2", Type: 1}}},
		{Name: "valheim", Description: "desc", Options: []discordCommandOption{{Name: "status", Description: "d1", Type: 1}}},
	}
	if commandsChanged(a, b) {
		t.Error("same commands in different order should not be changed")
	}
}

func TestCommandsChanged_CommandDescriptionChange(t *testing.T) {
	current := []discordCommand{
		{Name: "valheim", Description: "Old description", Options: []discordCommandOption{
			{Name: "status", Description: "Check status", Type: 1},
		}},
	}
	desired := []discordCommand{
		{Name: "valheim", Description: "New description", Options: []discordCommandOption{
			{Name: "status", Description: "Check status", Type: 1},
		}},
	}
	if !commandsChanged(current, desired) {
		t.Error("command description change should be detected")
	}
}

func TestCommandsChanged_OptionDescriptionChange(t *testing.T) {
	current := []discordCommand{
		{Name: "valheim", Description: "desc", Options: []discordCommandOption{
			{Name: "status", Description: "Old option description", Type: 1},
		}},
	}
	desired := []discordCommand{
		{Name: "valheim", Description: "desc", Options: []discordCommandOption{
			{Name: "status", Description: "New option description", Type: 1},
		}},
	}
	if !commandsChanged(current, desired) {
		t.Error("option description change should be detected")
	}
}

func TestCommandsChanged_AddedCommand(t *testing.T) {
	current := []discordCommand{{Name: "valheim", Description: "desc"}}
	desired := []discordCommand{
		{Name: "valheim", Description: "desc"},
		{Name: "minecraft", Description: "desc2"},
	}
	if !commandsChanged(current, desired) {
		t.Error("added command should be detected")
	}
}

// --- parseTFVarsReader tests ---

func TestParseTFVarsReader_BasicValues(t *testing.T) {
	input := `
# comment
world_name  = "MyWorld"
enable_discord_bot = true
discord_application_id = "12345"
discord_bot_token = "token-abc"
`
	vals := parseTFVarsReader(strings.NewReader(input))
	tests := map[string]string{
		"world_name":             "MyWorld",
		"enable_discord_bot":     "true",
		"discord_application_id": "12345",
		"discord_bot_token":      "token-abc",
	}
	for k, want := range tests {
		if got := vals[k]; got != want {
			t.Errorf("parseTFVarsReader[%q] = %q, want %q", k, got, want)
		}
	}
}

func TestParseTFVarsReader_CommentedOut(t *testing.T) {
	input := `
# enable_discord_bot = true
# discord_bot_token  = "should-not-appear"
world_name = "test"
`
	vals := parseTFVarsReader(strings.NewReader(input))
	if _, ok := vals["enable_discord_bot"]; ok {
		t.Error("commented-out enable_discord_bot should not be parsed")
	}
	if vals["world_name"] != "test" {
		t.Errorf("world_name = %q, want %q", vals["world_name"], "test")
	}
}

func TestParseTFVarsReader_UnquotedValue(t *testing.T) {
	input := `enable_discord_bot = false`
	vals := parseTFVarsReader(strings.NewReader(input))
	if vals["enable_discord_bot"] != "false" {
		t.Errorf("enable_discord_bot = %q, want %q", vals["enable_discord_bot"], "false")
	}
}

func TestParseTFVarsReader_Empty(t *testing.T) {
	vals := parseTFVarsReader(strings.NewReader(""))
	if len(vals) != 0 {
		t.Errorf("expected empty map, got %v", vals)
	}
}

// --- updateInteractionEndpoint tests ---

func TestUpdateInteractionEndpoint_NoChange(t *testing.T) {
	endpoint := "https://example.com/bot"
	client := &mockHTTPClient{
		responses: []*http.Response{
			{
				StatusCode: 200,
				Body:       jsonBody(map[string]string{"interactions_endpoint_url": endpoint}),
			},
		},
	}
	creds := discordCreds{
		applicationID: "app-123",
		botToken:      "tok",
	}
	if err := updateInteractionEndpoint(client, creds, endpoint); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should only have made 1 request (GET), no PATCH
	if len(client.requests) != 1 {
		t.Errorf("expected 1 request (GET only), got %d", len(client.requests))
	}
	if client.requests[0].Method != "GET" {
		t.Errorf("expected GET, got %s", client.requests[0].Method)
	}
}

func TestUpdateInteractionEndpoint_Updates(t *testing.T) {
	oldEndpoint := "https://old.example.com/bot"
	newEndpoint := "https://new.example.com/bot"
	client := &mockHTTPClient{
		responses: []*http.Response{
			{
				StatusCode: 200,
				Body:       jsonBody(map[string]string{"interactions_endpoint_url": oldEndpoint}),
			},
			{
				StatusCode: 200,
				Body:       io.NopCloser(strings.NewReader("{}")),
			},
		},
	}
	creds := discordCreds{
		applicationID: "app-123",
		botToken:      "tok",
	}
	if err := updateInteractionEndpoint(client, creds, newEndpoint); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(client.requests) != 2 {
		t.Errorf("expected 2 requests (GET + PATCH), got %d", len(client.requests))
	}
	if client.requests[1].Method != "PATCH" {
		t.Errorf("second request should be PATCH, got %s", client.requests[1].Method)
	}
}

func TestUpdateInteractionEndpoint_GetError(t *testing.T) {
	client := &mockHTTPClient{
		responses: []*http.Response{
			{StatusCode: 401, Body: io.NopCloser(strings.NewReader(`{"message":"401: Unauthorized"}`))},
		},
	}
	creds := discordCreds{applicationID: "app-123", botToken: "bad-token"}
	err := updateInteractionEndpoint(client, creds, "https://example.com/bot")
	if err == nil {
		t.Fatal("expected error on 401, got nil")
	}
}

func TestUpdateInteractionEndpoint_PatchError(t *testing.T) {
	client := &mockHTTPClient{
		responses: []*http.Response{
			{
				StatusCode: 200,
				Body:       jsonBody(map[string]string{"interactions_endpoint_url": "https://old.example.com"}),
			},
			{
				StatusCode: 400,
				Body:       io.NopCloser(strings.NewReader(`{"message":"invalid endpoint"}`)),
			},
		},
	}
	creds := discordCreds{applicationID: "app-123", botToken: "tok"}
	err := updateInteractionEndpoint(client, creds, "https://new.example.com")
	if err == nil {
		t.Fatal("expected error on PATCH 400, got nil")
	}
}

// --- registerAllCommands tests ---

func TestRegisterAllCommands_GlobalScope(t *testing.T) {
	client := &mockHTTPClient{
		responses: []*http.Response{
			// GET: no current commands → commands differ → PUT
			{StatusCode: 200, Body: io.NopCloser(strings.NewReader("[]"))},
			// PUT: success
			{StatusCode: 200, Body: io.NopCloser(strings.NewReader("[]"))},
		},
	}
	creds := discordCreds{
		applicationID: "app-123",
		botToken:      "tok",
	}
	if err := registerAllCommands(client, creds, []string{"valheim"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(client.requests) != 2 {
		t.Fatalf("expected 2 requests (GET + PUT), got %d", len(client.requests))
	}
	// Both GET and PUT use the global commands URL (no guild)
	if strings.Contains(client.requests[0].URL.String(), "guilds") {
		t.Errorf("expected global URL for GET, got %s", client.requests[0].URL)
	}
	if client.requests[1].Method != "PUT" {
		t.Errorf("second request method = %q, want PUT", client.requests[1].Method)
	}
	if strings.Contains(client.requests[1].URL.String(), "guilds") {
		t.Errorf("expected global URL for PUT, got %s", client.requests[1].URL)
	}
}

// TestRegisterAllCommands_NoOp verifies that no PUT is made when current
// commands already match what we would register.
func TestRegisterAllCommands_NoOp(t *testing.T) {
	current := []map[string]interface{}{
		{"id": "1", "name": "valheim", "description": "Control the valheim server", "options": []map[string]interface{}{
			{"id": "2", "name": "status", "type": 1, "description": "Check if the valheim server is running"},
			{"id": "3", "name": "start", "type": 1, "description": "Start the valheim server"},
			{"id": "4", "name": "stop", "type": 1, "description": "Stop the valheim server"},
			{"id": "5", "name": "help", "type": 1, "description": "Show available commands for the valheim server"},
			{"id": "6", "name": "hello", "type": 1, "description": "Check if the bot is reachable and verify your authorization status"},
		}},
	}
	client := &mockHTTPClient{
		responses: []*http.Response{
			{StatusCode: 200, Body: jsonBody(current)},
		},
	}
	creds := discordCreds{applicationID: "app-123", botToken: "tok"}
	if err := registerAllCommands(client, creds, []string{"valheim"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Only GET, no PUT
	if len(client.requests) != 1 {
		t.Errorf("expected 1 request (GET only), got %d", len(client.requests))
	}
	if client.requests[0].Method != "GET" {
		t.Errorf("expected GET, got %s", client.requests[0].Method)
	}
}

// TestRegisterAllCommands_UpdatesWhenDifferent verifies that a PUT is made
// when current commands differ.
func TestRegisterAllCommands_UpdatesWhenDifferent(t *testing.T) {
	current := []map[string]interface{}{
		{"id": "1", "name": "valheim"},
	}
	client := &mockHTTPClient{
		responses: []*http.Response{
			{StatusCode: 200, Body: jsonBody(current)},
			{StatusCode: 200, Body: io.NopCloser(strings.NewReader("[]"))},
		},
	}
	creds := discordCreds{applicationID: "app-123", botToken: "tok"}
	if err := registerAllCommands(client, creds, []string{"valheim"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(client.requests) != 2 {
		t.Errorf("expected 2 requests (GET + PUT), got %d", len(client.requests))
	}
	if client.requests[1].Method != "PUT" {
		t.Errorf("expected PUT for second request, got %s", client.requests[1].Method)
	}
}

// TestRegisterAllCommands_MultiGame verifies that all games are included in a
// single PUT with the correct command structure.
func TestRegisterAllCommands_MultiGame(t *testing.T) {
	var gotBody []byte
	client := &mockHTTPClient{
		responses: []*http.Response{
			{StatusCode: 200, Body: io.NopCloser(strings.NewReader("[]"))},
		},
	}
	captureClient := &captureBodyClient{inner: client, capturedBody: &gotBody}

	creds := discordCreds{applicationID: "app-123", botToken: "tok"}
	games := []string{"valheim", "minecraft"}
	if err := registerAllCommands(captureClient, creds, games); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var commands []discordCommand
	if err := json.Unmarshal(gotBody, &commands); err != nil {
		t.Fatalf("parsing command payload: %v", err)
	}

	// Expect one command per game (hello is a subcommand of each)
	if len(commands) != 2 {
		t.Fatalf("expected 2 commands (one per game), got %d", len(commands))
	}

	byName := map[string]discordCommand{}
	for _, c := range commands {
		byName[c.Name] = c
	}

	for _, game := range games {
		c, ok := byName[game]
		if !ok {
			t.Errorf("missing command for game %q", game)
			continue
		}
		subNames := map[string]bool{}
		for _, opt := range c.Options {
			subNames[opt.Name] = true
		}
		for _, want := range []string{"status", "start", "stop", "help", "hello"} {
			if !subNames[want] {
				t.Errorf("missing subcommand %q in /%s options", want, game)
			}
		}
	}
}

// TestRegisterAllCommands_SingleGamePayload verifies the command payload for a single game.
func TestRegisterAllCommands_SingleGamePayload(t *testing.T) {
	var gotBody []byte
	client := &mockHTTPClient{
		responses: []*http.Response{
			{StatusCode: 200, Body: io.NopCloser(strings.NewReader("[]"))},
		},
	}
	captureClient := &captureBodyClient{inner: client, capturedBody: &gotBody}

	creds := discordCreds{applicationID: "app-123", botToken: "tok"}
	if err := registerAllCommands(captureClient, creds, []string{"valheim"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var commands []discordCommand
	if err := json.Unmarshal(gotBody, &commands); err != nil {
		t.Fatalf("parsing command payload: %v", err)
	}
	if len(commands) != 1 {
		t.Fatalf("expected 1 command, got %d", len(commands))
	}

	if commands[0].Name != "valheim" {
		t.Errorf("commands[0].Name = %q, want %q", commands[0].Name, "valheim")
	}
	subNames := map[string]bool{}
	for _, opt := range commands[0].Options {
		subNames[opt.Name] = true
	}
	for _, want := range []string{"status", "start", "stop", "help", "hello"} {
		if !subNames[want] {
			t.Errorf("missing subcommand %q in /valheim options", want)
		}
	}
}

func TestRegisterAllCommands_HTTPError(t *testing.T) {
	client := &mockHTTPClient{
		responses: []*http.Response{
			{StatusCode: 403, Body: io.NopCloser(strings.NewReader(`{"message":"missing permissions"}`))},
		},
	}
	creds := discordCreds{applicationID: "app-123", botToken: "tok"}
	err := registerAllCommands(client, creds, []string{"valheim"})
	if err == nil {
		t.Fatal("expected error on 403, got nil")
	}
}

// captureBodyClient wraps an httpClient to capture the request body.
type captureBodyClient struct {
	inner        httpClient
	capturedBody *[]byte
}

func (c *captureBodyClient) Do(req *http.Request) (*http.Response, error) {
	if req.Body != nil {
		b, _ := io.ReadAll(req.Body)
		*c.capturedBody = b
		req.Body = io.NopCloser(strings.NewReader(string(b)))
	}
	return c.inner.Do(req)
}

// --- checkBotDeployed tests ---

func TestCheckBotDeployed_NotDeployed(t *testing.T) {
	repoRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repoRoot, "terraform", "games"), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("BONFIRE_REPO_ROOT", repoRoot)

	err := checkBotDeployed()
	if err == nil {
		t.Fatal("expected error when terraform.tfvars missing, got nil")
	}
}

func TestCheckBotDeployed_Deployed(t *testing.T) {
	repoRoot := t.TempDir()
	botDir := filepath.Join(repoRoot, "terraform", "bot")
	if err := os.MkdirAll(botDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(repoRoot, "terraform", "games"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(botDir, "terraform.tfvars"), []byte(`discord_application_id = "123"`), 0644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("BONFIRE_REPO_ROOT", repoRoot)

	if err := checkBotDeployed(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// setupBotDeployRoot creates a minimal fake repo root with terraform.tfvars and
// returns its path. Callers must set BONFIRE_REPO_ROOT themselves.
func setupBotDeployRoot(t *testing.T) string {
	t.Helper()
	repoRoot := t.TempDir()
	for _, dir := range []string{
		filepath.Join(repoRoot, "terraform", "games"),
		filepath.Join(repoRoot, "terraform", "bot"),
		filepath.Join(repoRoot, "discord_bot", "go"),
	} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
	}
	tfvars := filepath.Join(repoRoot, "terraform", "bot", "terraform.tfvars")
	if err := os.WriteFile(tfvars, []byte(`discord_application_id = "123"`+"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	return repoRoot
}

// writeFakeScript writes an executable shell script to dir/name that exits with exitCode.
func writeFakeScript(t *testing.T, dir, name string, exitCode int) {
	t.Helper()
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	content := "#!/bin/sh\nexit " + strings.TrimSpace(strings.Repeat("0", 0)+fmt.Sprintf("%d", exitCode)) + "\n"
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(content), 0755); err != nil {
		t.Fatal(err)
	}
}

// --- runBotDeploy edge case tests ---

// TestRunBotDeploy_MissingTFVars verifies that the friendly "not deployed yet"
// error is returned before any subprocess is launched.
func TestRunBotDeploy_MissingTFVars(t *testing.T) {
	repoRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repoRoot, "terraform", "games"), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("BONFIRE_REPO_ROOT", repoRoot)

	err := runBotDeploy(nil, nil)
	if err == nil {
		t.Fatal("expected error when terraform.tfvars missing, got nil")
	}
	if !strings.Contains(err.Error(), "Bot not deployed yet") {
		t.Errorf("error should mention 'Bot not deployed yet', got: %v", err)
	}
}

// TestRunBotDeploy_MakeNotFound verifies that an error is returned when the
// make binary is not present in PATH.
func TestRunBotDeploy_MakeNotFound(t *testing.T) {
	repoRoot := setupBotDeployRoot(t)
	t.Setenv("BONFIRE_REPO_ROOT", repoRoot)
	// Point PATH to an empty directory so make (and terraform) are not found.
	emptyBin := t.TempDir()
	t.Setenv("PATH", emptyBin)

	err := runBotDeploy(nil, nil)
	if err == nil {
		t.Fatal("expected error when make not in PATH, got nil")
	}
}

// TestRunBotDeploy_MakeBuildFails_StopsPipeline verifies that a non-zero exit
// from `make build` causes an error and prevents terraform from running.
func TestRunBotDeploy_MakeBuildFails_StopsPipeline(t *testing.T) {
	repoRoot := setupBotDeployRoot(t)
	t.Setenv("BONFIRE_REPO_ROOT", repoRoot)

	// Create a bin dir with a fake make that exits 1, but no terraform.
	// If runBotDeploy incorrectly calls terraform after make fails, the test
	// would still pass here — but the pipeline would continue past the error,
	// which is wrong. We verify the error message identifies make as the failure.
	binDir := t.TempDir()
	writeFakeScript(t, binDir, "make", 1)
	t.Setenv("PATH", binDir)

	err := runBotDeploy(nil, nil)
	if err == nil {
		t.Fatal("expected error when make build fails, got nil")
	}
	if !strings.Contains(err.Error(), "make build") {
		t.Errorf("error should mention 'make build', got: %v", err)
	}
}

// TestRunBotDeploy_TerraformNotFound verifies that an error is returned when
// terraform is not present in PATH (after make build succeeds).
func TestRunBotDeploy_TerraformNotFound(t *testing.T) {
	repoRoot := setupBotDeployRoot(t)
	t.Setenv("BONFIRE_REPO_ROOT", repoRoot)

	// Fake make that exits 0 (build succeeds), but no terraform binary.
	binDir := t.TempDir()
	writeFakeScript(t, binDir, "make", 0)
	t.Setenv("PATH", binDir)

	err := runBotDeploy(nil, nil)
	if err == nil {
		t.Fatal("expected error when terraform not in PATH, got nil")
	}
	// The error should come from terraform, not make.
	if strings.Contains(err.Error(), "make build") {
		t.Errorf("error should not be from make build: %v", err)
	}
}
