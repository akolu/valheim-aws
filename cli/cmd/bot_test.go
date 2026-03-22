package cmd

import (
	"encoding/json"
	"io"
	"net/http"
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
