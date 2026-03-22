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
		"world_name":            "MyWorld",
		"enable_discord_bot":    "true",
		"discord_application_id": "12345",
		"discord_bot_token":     "token-abc",
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
	if err := updateInteractionEndpoint(client, creds, endpoint, "valheim"); err != nil {
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
	if err := updateInteractionEndpoint(client, creds, newEndpoint, "valheim"); err != nil {
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
	err := updateInteractionEndpoint(client, creds, "https://example.com/bot", "valheim")
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
	err := updateInteractionEndpoint(client, creds, "https://new.example.com", "valheim")
	if err == nil {
		t.Fatal("expected error on PATCH 400, got nil")
	}
}

// --- registerSlashCommands tests ---

func TestRegisterSlashCommands_GuildScope(t *testing.T) {
	client := &mockHTTPClient{
		responses: []*http.Response{
			{StatusCode: 200, Body: io.NopCloser(strings.NewReader("[]"))},
		},
	}
	creds := discordCreds{
		applicationID: "app-123",
		botToken:      "tok",
		guildID:       "guild-456",
	}
	if err := registerSlashCommands(client, creds, "valheim"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(client.requests) != 1 {
		t.Fatalf("expected 1 request, got %d", len(client.requests))
	}
	req := client.requests[0]
	if req.Method != "PUT" {
		t.Errorf("method = %q, want PUT", req.Method)
	}
	if !strings.Contains(req.URL.String(), "guilds/guild-456/commands") {
		t.Errorf("expected guild URL, got %s", req.URL)
	}
}

func TestRegisterSlashCommands_GlobalScope(t *testing.T) {
	client := &mockHTTPClient{
		responses: []*http.Response{
			{StatusCode: 200, Body: io.NopCloser(strings.NewReader("[]"))},
		},
	}
	creds := discordCreds{
		applicationID: "app-123",
		botToken:      "tok",
		// no guildID
	}
	if err := registerSlashCommands(client, creds, "valheim"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	req := client.requests[0]
	if strings.Contains(req.URL.String(), "guilds") {
		t.Errorf("expected global URL (no guild), got %s", req.URL)
	}
}

func TestRegisterSlashCommands_CommandPayload(t *testing.T) {
	var gotBody []byte
	client := &mockHTTPClient{
		responses: []*http.Response{
			{StatusCode: 200, Body: io.NopCloser(strings.NewReader("[]"))},
		},
	}
	// Capture request body by wrapping
	realClient := client
	captureClient := &captureBodyClient{inner: realClient, capturedBody: &gotBody}

	creds := discordCreds{applicationID: "app-123", botToken: "tok", guildID: "g1"}
	if err := registerSlashCommands(captureClient, creds, "valheim"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var commands []discordCommand
	if err := json.Unmarshal(gotBody, &commands); err != nil {
		t.Fatalf("parsing command payload: %v", err)
	}
	if len(commands) != 2 {
		t.Fatalf("expected 2 commands, got %d", len(commands))
	}

	// First command: /hello
	if commands[0].Name != "hello" {
		t.Errorf("commands[0].Name = %q, want %q", commands[0].Name, "hello")
	}

	// Second command: /valheim with 4 subcommands
	if commands[1].Name != "valheim" {
		t.Errorf("commands[1].Name = %q, want %q", commands[1].Name, "valheim")
	}
	subNames := map[string]bool{}
	for _, opt := range commands[1].Options {
		subNames[opt.Name] = true
	}
	for _, want := range []string{"status", "start", "stop", "help"} {
		if !subNames[want] {
			t.Errorf("missing subcommand %q in /valheim options", want)
		}
	}
}

func TestRegisterSlashCommands_HTTPError(t *testing.T) {
	client := &mockHTTPClient{
		responses: []*http.Response{
			{StatusCode: 403, Body: io.NopCloser(strings.NewReader(`{"message":"missing permissions"}`))},
		},
	}
	creds := discordCreds{applicationID: "app-123", botToken: "tok"}
	err := registerSlashCommands(client, creds, "valheim")
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
