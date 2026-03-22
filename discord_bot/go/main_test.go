package main

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	ssmtypes "github.com/aws/aws-sdk-go-v2/service/ssm/types"
)

// mockEC2Client implements EC2API for tests without requiring AWS credentials.
type mockEC2Client struct {
	describeOutput *ec2.DescribeInstancesOutput
	describeErr    error
	startErr       error
	stopErr        error

	startCalled bool
	stopCalled  bool

	// capturedDescribeInput records the last DescribeInstances call for assertions
	capturedDescribeInput *ec2.DescribeInstancesInput
}

func (m *mockEC2Client) DescribeInstances(_ context.Context, input *ec2.DescribeInstancesInput, _ ...func(*ec2.Options)) (*ec2.DescribeInstancesOutput, error) {
	m.capturedDescribeInput = input
	return m.describeOutput, m.describeErr
}

func (m *mockEC2Client) StartInstances(_ context.Context, _ *ec2.StartInstancesInput, _ ...func(*ec2.Options)) (*ec2.StartInstancesOutput, error) {
	m.startCalled = true
	return &ec2.StartInstancesOutput{}, m.startErr
}

func (m *mockEC2Client) StopInstances(_ context.Context, _ *ec2.StopInstancesInput, _ ...func(*ec2.Options)) (*ec2.StopInstancesOutput, error) {
	m.stopCalled = true
	return &ec2.StopInstancesOutput{}, m.stopErr
}

// mockSSMClient implements SSMAPI for tests.
type mockSSMClient struct {
	// params maps SSM parameter name → value; missing key = ParameterNotFound
	params map[string]string
}

func (m *mockSSMClient) GetParameter(_ context.Context, input *ssm.GetParameterInput, _ ...func(*ssm.Options)) (*ssm.GetParameterOutput, error) {
	name := aws.ToString(input.Name)
	val, ok := m.params[name]
	if !ok {
		return nil, &ssmtypes.ParameterNotFound{Message: aws.String("not found")}
	}
	return &ssm.GetParameterOutput{
		Parameter: &ssmtypes.Parameter{Value: aws.String(val)},
	}, nil
}

// helpers for building EC2 describe outputs

func runningInstanceWithID(id, ip string) *ec2.DescribeInstancesOutput {
	launchTime := time.Now().Add(-10 * time.Minute)
	t2micro := ec2types.InstanceTypeT2Micro
	return &ec2.DescribeInstancesOutput{
		Reservations: []ec2types.Reservation{
			{
				Instances: []ec2types.Instance{
					{
						InstanceId:      aws.String(id),
						State:           &ec2types.InstanceState{Name: ec2types.InstanceStateNameRunning},
						InstanceType:    t2micro,
						PublicIpAddress: aws.String(ip),
						LaunchTime:      &launchTime,
					},
				},
			},
		},
	}
}

func stoppedInstanceWithID(id string) *ec2.DescribeInstancesOutput {
	return &ec2.DescribeInstancesOutput{
		Reservations: []ec2types.Reservation{
			{
				Instances: []ec2types.Instance{
					{
						InstanceId:   aws.String(id),
						State:        &ec2types.InstanceState{Name: ec2types.InstanceStateNameStopped},
						InstanceType: ec2types.InstanceTypeT2Micro,
					},
				},
			},
		},
	}
}

func stoppingInstanceWithID(id string) *ec2.DescribeInstancesOutput {
	return &ec2.DescribeInstancesOutput{
		Reservations: []ec2types.Reservation{
			{
				Instances: []ec2types.Instance{
					{
						InstanceId:   aws.String(id),
						State:        &ec2types.InstanceState{Name: ec2types.InstanceStateNameStopping},
						InstanceType: ec2types.InstanceTypeT2Micro,
					},
				},
			},
		},
	}
}

func twoInstancesOutput() *ec2.DescribeInstancesOutput {
	return &ec2.DescribeInstancesOutput{
		Reservations: []ec2types.Reservation{
			{
				Instances: []ec2types.Instance{
					{
						InstanceId:   aws.String("i-aaa"),
						State:        &ec2types.InstanceState{Name: ec2types.InstanceStateNameRunning},
						InstanceType: ec2types.InstanceTypeT2Micro,
					},
					{
						InstanceId:   aws.String("i-bbb"),
						State:        &ec2types.InstanceState{Name: ec2types.InstanceStateNameRunning},
						InstanceType: ec2types.InstanceTypeT2Micro,
					},
				},
			},
		},
	}
}

func emptyDescribeOutput() *ec2.DescribeInstancesOutput {
	return &ec2.DescribeInstancesOutput{}
}

// ssmWithGuild returns a mock SSM client with the allowed_guilds entry set.
func ssmWithGuild(guildID string) *mockSSMClient {
	return &mockSSMClient{params: map[string]string{
		"/bonfire/allowed_guilds": guildID,
	}}
}

// ssmWithGuildAndUsers returns a mock SSM client with allowed_guilds and per-game authorized_users.
func ssmWithGuildAndUsers(guildID, game, users string) *mockSSMClient {
	return &mockSSMClient{params: map[string]string{
		"/bonfire/allowed_guilds":              guildID,
		"/bonfire/" + game + "/authorized_users": users,
	}}
}

// --- Signature verification ---

func generateTestKey(t *testing.T) (ed25519.PublicKey, ed25519.PrivateKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}
	return pub, priv
}

func signMessage(priv ed25519.PrivateKey, timestamp, body string) string {
	msg := []byte(timestamp + body)
	sig := ed25519.Sign(priv, msg)
	return hex.EncodeToString(sig)
}

func TestVerifyDiscordRequest_ValidSignature(t *testing.T) {
	pub, priv := generateTestKey(t)
	t.Setenv("DISCORD_PUBLIC_KEY", hex.EncodeToString(pub))

	timestamp := "1234567890"
	body := `{"type":1}`
	sig := signMessage(priv, timestamp, body)

	if !verifyDiscordRequest(sig, timestamp, body) {
		t.Error("valid signature should verify successfully")
	}
}

func TestVerifyDiscordRequest_InvalidSignature(t *testing.T) {
	pub, _ := generateTestKey(t)
	t.Setenv("DISCORD_PUBLIC_KEY", hex.EncodeToString(pub))

	_, wrongPriv := generateTestKey(t)
	timestamp := "1234567890"
	body := `{"type":1}`
	badSig := signMessage(wrongPriv, timestamp, body)

	if verifyDiscordRequest(badSig, timestamp, body) {
		t.Error("signature from wrong key should fail verification")
	}
}

func TestVerifyDiscordRequest_TamperedBody(t *testing.T) {
	pub, priv := generateTestKey(t)
	t.Setenv("DISCORD_PUBLIC_KEY", hex.EncodeToString(pub))

	timestamp := "1234567890"
	body := `{"type":1}`
	sig := signMessage(priv, timestamp, body)

	if verifyDiscordRequest(sig, timestamp, `{"type":2}`) {
		t.Error("signature for original body should not verify against tampered body")
	}
}

func TestVerifyDiscordRequest_InvalidHex(t *testing.T) {
	t.Setenv("DISCORD_PUBLIC_KEY", "not-valid-hex")
	if verifyDiscordRequest("aabbcc", "ts", "body") {
		t.Error("invalid public key hex should return false")
	}
}

// --- PING/PONG via handler ---

func makeSignedRequest(t *testing.T, pub ed25519.PublicKey, priv ed25519.PrivateKey, body string) LambdaRequest {
	t.Helper()
	timestamp := "1234567890"
	sig := signMessage(priv, timestamp, body)
	return LambdaRequest{
		Headers: map[string]string{
			"x-signature-ed25519":   sig,
			"x-signature-timestamp": timestamp,
		},
		Body: body,
	}
}

func TestHandler_Ping(t *testing.T) {
	pub, priv := generateTestKey(t)
	t.Setenv("DISCORD_PUBLIC_KEY", hex.EncodeToString(pub))

	body := `{"type":1}`
	req := makeSignedRequest(t, pub, priv, body)

	resp, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	var result map[string]int
	if err := json.Unmarshal([]byte(resp.Body), &result); err != nil {
		t.Fatalf("failed to parse response body: %v", err)
	}
	if result["type"] != 1 {
		t.Errorf("expected PONG type 1, got %d", result["type"])
	}
}

func TestHandler_MissingSignatureHeaders(t *testing.T) {
	req := LambdaRequest{
		Headers: map[string]string{},
		Body:    `{"type":1}`,
	}
	resp, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != 401 {
		t.Errorf("expected 401 for missing headers, got %d", resp.StatusCode)
	}
}

func TestHandler_InvalidSignature(t *testing.T) {
	pub, _ := generateTestKey(t)
	_, wrongPriv := generateTestKey(t)
	t.Setenv("DISCORD_PUBLIC_KEY", hex.EncodeToString(pub))

	body := `{"type":1}`
	req := makeSignedRequest(t, pub, wrongPriv, body)

	resp, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != 401 {
		t.Errorf("expected 401 for invalid signature, got %d", resp.StatusCode)
	}
}

// --- handleInteraction helpers ---

func interactionWith(gameName, action, userID, guildID string) Interaction {
	i := Interaction{
		Type:    2,
		GuildID: guildID,
		Data: InteractionData{
			Name:    gameName,
			Options: []SubCommand{{Name: action}},
		},
	}
	if userID != "" {
		i.User = &User{ID: userID}
	}
	return i
}

func parseInteractionResponse(t *testing.T, resp LambdaResponse) InteractionResponse {
	t.Helper()
	var ir InteractionResponse
	if err := json.Unmarshal([]byte(resp.Body), &ir); err != nil {
		t.Fatalf("failed to parse interaction response: %v", err)
	}
	return ir
}

// --- Guild allowlist tests ---

func TestGuildAllowlist_AllowedGuild(t *testing.T) {
	ssmClient := ssmWithGuild("guild123")
	mock := &mockEC2Client{describeOutput: stoppedInstanceWithID("i-test")}

	interaction := interactionWith("valheim", "status", "", "guild123")
	resp := handleInteraction(context.Background(), interaction, mock, ssmClient)

	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	ir := parseInteractionResponse(t, resp)
	if ir.Data != nil && strings.Contains(ir.Data.Content, "not available") {
		t.Errorf("allowlisted guild should not be rejected, got: %s", ir.Data.Content)
	}
}

func TestGuildAllowlist_BlockedGuild(t *testing.T) {
	ssmClient := ssmWithGuild("guild123")
	mock := &mockEC2Client{}

	interaction := interactionWith("valheim", "status", "", "other-guild")
	resp := handleInteraction(context.Background(), interaction, mock, ssmClient)

	if resp.StatusCode != 200 {
		t.Errorf("expected 200 (Discord requires 200 even for rejections), got %d", resp.StatusCode)
	}
	ir := parseInteractionResponse(t, resp)
	if ir.Data == nil || !strings.Contains(ir.Data.Content, "not available") {
		t.Errorf("non-allowlisted guild should be rejected, got: %v", ir.Data)
	}
	// Should be ephemeral
	if ir.Data != nil && ir.Data.Flags != 64 {
		t.Error("guild rejection should be ephemeral (flags=64)")
	}
}

func TestGuildAllowlist_EmptyListBlocksAll(t *testing.T) {
	// Empty allowed_guilds = fail closed
	ssmClient := &mockSSMClient{params: map[string]string{
		"/bonfire/allowed_guilds": "",
	}}
	mock := &mockEC2Client{}

	interaction := interactionWith("valheim", "status", "", "any-guild")
	resp := handleInteraction(context.Background(), interaction, mock, ssmClient)

	ir := parseInteractionResponse(t, resp)
	if ir.Data == nil || !strings.Contains(ir.Data.Content, "not available") {
		t.Errorf("empty allowlist should block all guilds, got: %v", ir.Data)
	}
}

func TestGuildAllowlist_AbsentParameterBlocksAll(t *testing.T) {
	// No /bonfire/allowed_guilds parameter at all = fail closed
	ssmClient := &mockSSMClient{params: map[string]string{}}
	mock := &mockEC2Client{}

	interaction := interactionWith("valheim", "status", "", "any-guild")
	resp := handleInteraction(context.Background(), interaction, mock, ssmClient)

	ir := parseInteractionResponse(t, resp)
	if ir.Data == nil || !strings.Contains(ir.Data.Content, "not available") {
		t.Errorf("absent allowlist should block all guilds, got: %v", ir.Data)
	}
}

func TestGuildAllowlist_PingSkipsCheck(t *testing.T) {
	// PING (type=1) is handled before handleInteraction; guild check never fires
	// Test at the handler level: PING should succeed regardless of guild allowlist
	pub, priv := generateTestKey(t)
	t.Setenv("DISCORD_PUBLIC_KEY", hex.EncodeToString(pub))

	body := `{"type":1,"guild_id":"unknown-guild"}`
	req := makeSignedRequest(t, pub, priv, body)

	resp, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("expected 200 for PING, got %d", resp.StatusCode)
	}
	var result map[string]int
	if err := json.Unmarshal([]byte(resp.Body), &result); err != nil {
		t.Fatalf("failed to parse PING response: %v", err)
	}
	if result["type"] != 1 {
		t.Errorf("expected PONG type 1, got %d", result["type"])
	}
}

// --- Tag-based instance discovery tests ---

func TestHandleInteraction_TagDiscovery_Found(t *testing.T) {
	ssmClient := ssmWithGuild("g1")
	mock := &mockEC2Client{describeOutput: stoppedInstanceWithID("i-abc")}

	interaction := interactionWith("valheim", "status", "", "g1")
	resp := handleInteraction(context.Background(), interaction, mock, ssmClient)

	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	ir := parseInteractionResponse(t, resp)
	if ir.Data == nil || !strings.Contains(ir.Data.Content, "stopped") {
		t.Errorf("expected stopped status, got: %v", ir.Data)
	}
	// Verify the EC2 filter used game tag
	if mock.capturedDescribeInput != nil {
		hasGameTag := false
		for _, f := range mock.capturedDescribeInput.Filters {
			if aws.ToString(f.Name) == "tag:Game" {
				for _, v := range f.Values {
					if v == "valheim" {
						hasGameTag = true
					}
				}
			}
		}
		if !hasGameTag {
			t.Error("expected DescribeInstances filter tag:Game=valheim")
		}
	}
}

func TestHandleInteraction_TagDiscovery_NotFound(t *testing.T) {
	ssmClient := ssmWithGuild("g1")
	mock := &mockEC2Client{describeOutput: emptyDescribeOutput()}

	interaction := interactionWith("valheim", "status", "", "g1")
	resp := handleInteraction(context.Background(), interaction, mock, ssmClient)

	ir := parseInteractionResponse(t, resp)
	if ir.Data == nil || !strings.Contains(ir.Data.Content, "No server found") {
		t.Errorf("expected 'No server found' error, got: %v", ir.Data)
	}
}

func TestHandleInteraction_TagDiscovery_Multiple(t *testing.T) {
	ssmClient := ssmWithGuild("g1")
	mock := &mockEC2Client{describeOutput: twoInstancesOutput()}

	interaction := interactionWith("valheim", "status", "", "g1")
	resp := handleInteraction(context.Background(), interaction, mock, ssmClient)

	ir := parseInteractionResponse(t, resp)
	if ir.Data == nil || !strings.Contains(ir.Data.Content, "Multiple instances") {
		t.Errorf("expected 'Multiple instances' error, got: %v", ir.Data)
	}
}

// --- SSM authorization tests ---

func TestSSMAuth_Present_Authorized(t *testing.T) {
	ssmClient := ssmWithGuildAndUsers("g1", "valheim", "admin,user2")
	mock := &mockEC2Client{describeOutput: stoppedInstanceWithID("i-test")}

	interaction := interactionWith("valheim", "start", "admin", "g1")
	resp := handleInteraction(context.Background(), interaction, mock, ssmClient)

	if !mock.startCalled {
		t.Error("StartInstances should be called for authorized user")
	}
	_ = resp
}

func TestSSMAuth_Present_Unauthorized(t *testing.T) {
	ssmClient := ssmWithGuildAndUsers("g1", "valheim", "admin")
	mock := &mockEC2Client{}

	interaction := interactionWith("valheim", "start", "stranger", "g1")
	resp := handleInteraction(context.Background(), interaction, mock, ssmClient)

	ir := parseInteractionResponse(t, resp)
	if ir.Data == nil || !strings.Contains(ir.Data.Content, "permission") {
		t.Errorf("expected permission error, got: %v", ir.Data)
	}
	if mock.startCalled {
		t.Error("StartInstances should not be called for unauthorized user")
	}
}

func TestSSMAuth_Absent_DeniesAll(t *testing.T) {
	// No authorized_users param = deny all
	ssmClient := ssmWithGuild("g1")
	mock := &mockEC2Client{}

	interaction := interactionWith("valheim", "start", "admin", "g1")
	resp := handleInteraction(context.Background(), interaction, mock, ssmClient)

	ir := parseInteractionResponse(t, resp)
	if ir.Data == nil || !strings.Contains(ir.Data.Content, "permission") {
		t.Errorf("absent authorized_users should deny all, got: %v", ir.Data)
	}
	if mock.startCalled {
		t.Error("StartInstances should not be called when authorized_users is absent")
	}
}

// --- Status command ---

func TestHandleInteraction_StatusCommand(t *testing.T) {
	ssmClient := ssmWithGuild("g1")
	mock := &mockEC2Client{describeOutput: stoppedInstanceWithID("i-test")}

	resp := handleInteraction(context.Background(), interactionWith("mc", "status", "", "g1"), mock, ssmClient)

	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	ir := parseInteractionResponse(t, resp)
	if ir.Data == nil || !strings.Contains(ir.Data.Content, "stopped") {
		t.Errorf("expected status to mention 'stopped', got: %v", ir.Data)
	}
	if ir.Data.Flags != 64 {
		t.Errorf("expected ephemeral flags=64, got %d", ir.Data.Flags)
	}
}

func TestHandleInteraction_StatusRunning(t *testing.T) {
	ssmClient := ssmWithGuild("g1")
	mock := &mockEC2Client{describeOutput: runningInstanceWithID("i-test", "1.2.3.4")}

	resp := handleInteraction(context.Background(), interactionWith("mc", "status", "", "g1"), mock, ssmClient)

	ir := parseInteractionResponse(t, resp)
	if ir.Data == nil || !strings.Contains(ir.Data.Content, "running") {
		t.Errorf("expected status to mention 'running', got: %v", ir.Data)
	}
	if !strings.Contains(ir.Data.Content, "1.2.3.4") {
		t.Errorf("expected IP address in status, got: %s", ir.Data.Content)
	}
}

// --- Start/Stop commands ---

func TestHandleInteraction_StartUnauthorized(t *testing.T) {
	ssmClient := ssmWithGuildAndUsers("g1", "mc", "admin")
	mock := &mockEC2Client{}

	resp := handleInteraction(context.Background(), interactionWith("mc", "start", "notadmin", "g1"), mock, ssmClient)

	ir := parseInteractionResponse(t, resp)
	if ir.Data == nil || !strings.Contains(ir.Data.Content, "permission") {
		t.Errorf("expected permission error, got: %v", ir.Data)
	}
	if mock.startCalled {
		t.Error("StartInstances should not be called for unauthorized user")
	}
	if ir.Data.Flags == 64 {
		t.Error("unauthorized message should be public, not ephemeral")
	}
}

func TestHandleInteraction_StartAuthorized_AlreadyRunning(t *testing.T) {
	ssmClient := ssmWithGuildAndUsers("g1", "mc", "admin")
	mock := &mockEC2Client{describeOutput: runningInstanceWithID("i-test", "5.6.7.8")}

	resp := handleInteraction(context.Background(), interactionWith("mc", "start", "admin", "g1"), mock, ssmClient)

	ir := parseInteractionResponse(t, resp)
	if ir.Data == nil || !strings.Contains(ir.Data.Content, "already running") {
		t.Errorf("expected 'already running' message, got: %v", ir.Data)
	}
	if mock.startCalled {
		t.Error("StartInstances should not be called when already running")
	}
}

func TestHandleInteraction_StartAuthorized_Stopped(t *testing.T) {
	ssmClient := ssmWithGuildAndUsers("g1", "mc", "admin")
	mock := &mockEC2Client{describeOutput: stoppedInstanceWithID("i-test")}

	resp := handleInteraction(context.Background(), interactionWith("mc", "start", "admin", "g1"), mock, ssmClient)

	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	if !mock.startCalled {
		t.Error("StartInstances should be called for authorized user starting stopped server")
	}
	ir := parseInteractionResponse(t, resp)
	if ir.Data == nil || !strings.Contains(ir.Data.Content, "starting") {
		t.Errorf("expected 'starting' message, got: %v", ir.Data)
	}
}

func TestHandleInteraction_StopUnauthorized(t *testing.T) {
	ssmClient := ssmWithGuildAndUsers("g1", "mc", "admin")
	mock := &mockEC2Client{}

	resp := handleInteraction(context.Background(), interactionWith("mc", "stop", "notadmin", "g1"), mock, ssmClient)

	ir := parseInteractionResponse(t, resp)
	if ir.Data == nil || !strings.Contains(ir.Data.Content, "permission") {
		t.Errorf("expected permission error, got: %v", ir.Data)
	}
	if mock.stopCalled {
		t.Error("StopInstances should not be called for unauthorized user")
	}
}

func TestHandleInteraction_StopAuthorized(t *testing.T) {
	ssmClient := ssmWithGuildAndUsers("g1", "mc", "admin")
	mock := &mockEC2Client{describeOutput: runningInstanceWithID("i-test", "1.2.3.4")}

	resp := handleInteraction(context.Background(), interactionWith("mc", "stop", "admin", "g1"), mock, ssmClient)

	if !mock.stopCalled {
		t.Error("StopInstances should be called for authorized user stopping running server")
	}
	ir := parseInteractionResponse(t, resp)
	if ir.Data == nil || !strings.Contains(ir.Data.Content, "stopping") {
		t.Errorf("expected 'stopping' message, got: %v", ir.Data)
	}
}

func TestHandleInteraction_StopAuthorized_AlreadyStopped(t *testing.T) {
	ssmClient := ssmWithGuildAndUsers("g1", "mc", "admin")
	mock := &mockEC2Client{describeOutput: stoppedInstanceWithID("i-test")}

	resp := handleInteraction(context.Background(), interactionWith("mc", "stop", "admin", "g1"), mock, ssmClient)

	ir := parseInteractionResponse(t, resp)
	if ir.Data == nil || !strings.Contains(ir.Data.Content, "already stopped") {
		t.Errorf("expected 'already stopped' message, got: %v", ir.Data)
	}
	if mock.stopCalled {
		t.Error("StopInstances should not be called when server is already stopped")
	}
}

func TestHandleInteraction_StopAuthorized_AlreadyStopping(t *testing.T) {
	ssmClient := ssmWithGuildAndUsers("g1", "mc", "admin")
	mock := &mockEC2Client{describeOutput: stoppingInstanceWithID("i-test")}

	resp := handleInteraction(context.Background(), interactionWith("mc", "stop", "admin", "g1"), mock, ssmClient)

	ir := parseInteractionResponse(t, resp)
	if ir.Data == nil || !strings.Contains(ir.Data.Content, "already stopped") {
		t.Errorf("expected 'already stopped' message, got: %v", ir.Data)
	}
	if mock.stopCalled {
		t.Error("StopInstances should not be called when server is already stopping")
	}
}

// --- Help command ---

func TestHandleInteraction_HelpCommand(t *testing.T) {
	ssmClient := ssmWithGuild("g1")
	mock := &mockEC2Client{}

	resp := handleInteraction(context.Background(), interactionWith("mc", "help", "", "g1"), mock, ssmClient)

	ir := parseInteractionResponse(t, resp)
	if ir.Data == nil || !strings.Contains(ir.Data.Content, "Commands") {
		t.Errorf("expected help text with 'Commands', got: %v", ir.Data)
	}
	if ir.Data.Flags != 64 {
		t.Error("help should be ephemeral")
	}
	// Game name should appear in help text
	if !strings.Contains(ir.Data.Content, "mc") {
		t.Errorf("expected game name in help text, got: %s", ir.Data.Content)
	}
}

// --- Hello command ---

func TestHandleInteraction_HelloCommand_Authorized(t *testing.T) {
	ssmClient := ssmWithGuildAndUsers("g1", "mc", "user42")
	mock := &mockEC2Client{}

	interaction := Interaction{
		Type:    2,
		GuildID: "g1",
		Data:    InteractionData{Name: "mc", Options: []SubCommand{{Name: "hello"}}},
		User:    &User{ID: "user42"},
	}
	resp := handleInteraction(context.Background(), interaction, mock, ssmClient)

	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	ir := parseInteractionResponse(t, resp)
	if ir.Data == nil {
		t.Fatal("expected response data, got nil")
	}
	if !strings.Contains(ir.Data.Content, "reachable") {
		t.Errorf("expected 'reachable' in response, got: %s", ir.Data.Content)
	}
	if !strings.Contains(ir.Data.Content, "user42") {
		t.Errorf("expected user ID in response, got: %s", ir.Data.Content)
	}
	if strings.Contains(ir.Data.Content, "not authorized") {
		t.Errorf("authorized user should not show 'not authorized', got: %s", ir.Data.Content)
	}
	if !strings.Contains(ir.Data.Content, "mc") {
		t.Errorf("expected game name in response, got: %s", ir.Data.Content)
	}
	if ir.Data.Flags != 64 {
		t.Error("hello response should be ephemeral (flags=64)")
	}
	if mock.startCalled || mock.stopCalled {
		t.Error("hello command should not make any EC2 calls")
	}
}

func TestHandleInteraction_HelloCommand_NotAuthorized(t *testing.T) {
	ssmClient := ssmWithGuildAndUsers("g1", "mc", "admin")
	mock := &mockEC2Client{}

	interaction := Interaction{
		Type:    2,
		GuildID: "g1",
		Data:    InteractionData{Name: "mc", Options: []SubCommand{{Name: "hello"}}},
		User:    &User{ID: "stranger"},
	}
	resp := handleInteraction(context.Background(), interaction, mock, ssmClient)

	ir := parseInteractionResponse(t, resp)
	if ir.Data == nil {
		t.Fatal("expected response data, got nil")
	}
	if !strings.Contains(ir.Data.Content, "not authorized") {
		t.Errorf("expected 'not authorized' for non-authorized user, got: %s", ir.Data.Content)
	}
	if !strings.Contains(ir.Data.Content, "stranger") {
		t.Errorf("expected user ID in response, got: %s", ir.Data.Content)
	}
}

func TestHandleInteraction_HelloCommand_ViaGuildMember(t *testing.T) {
	ssmClient := ssmWithGuildAndUsers("g1", "mc", "guilduser")
	mock := &mockEC2Client{}

	interaction := Interaction{
		Type:    2,
		GuildID: "g1",
		Data:    InteractionData{Name: "mc", Options: []SubCommand{{Name: "hello"}}},
		Member:  &Member{User: &User{ID: "guilduser"}},
	}
	resp := handleInteraction(context.Background(), interaction, mock, ssmClient)

	ir := parseInteractionResponse(t, resp)
	if ir.Data == nil {
		t.Fatal("expected response data, got nil")
	}
	if !strings.Contains(ir.Data.Content, "guilduser") {
		t.Errorf("expected guild member user ID in response, got: %s", ir.Data.Content)
	}
	if strings.Contains(ir.Data.Content, "not authorized") {
		t.Errorf("authorized guild member should not see 'not authorized', got: %s", ir.Data.Content)
	}
}

// --- Edge cases ---

func TestHandleInteraction_UnknownAction(t *testing.T) {
	ssmClient := ssmWithGuild("g1")
	mock := &mockEC2Client{}

	resp := handleInteraction(context.Background(), interactionWith("mc", "bogus", "", "g1"), mock, ssmClient)

	ir := parseInteractionResponse(t, resp)
	if ir.Data == nil || !strings.Contains(ir.Data.Content, "Unknown") {
		t.Errorf("expected 'Unknown' in response, got: %v", ir.Data)
	}
}

func TestHandleInteraction_NonSlashCommand(t *testing.T) {
	mock := &mockEC2Client{}
	ssmClient := &mockSSMClient{params: map[string]string{}}
	interaction := Interaction{Type: 3}
	resp := handleInteraction(context.Background(), interaction, mock, ssmClient)

	if resp.StatusCode != 400 {
		t.Errorf("expected 400 for non-slash command, got %d", resp.StatusCode)
	}
}

func TestHandleInteraction_MemberUserFallback(t *testing.T) {
	ssmClient := ssmWithGuildAndUsers("g1", "mc", "admin")
	mock := &mockEC2Client{describeOutput: runningInstanceWithID("i-test", "1.2.3.4")}

	interaction := Interaction{
		Type:    2,
		GuildID: "g1",
		Data: InteractionData{
			Name:    "mc",
			Options: []SubCommand{{Name: "stop"}},
		},
		Member: &Member{User: &User{ID: "admin"}},
	}

	resp := handleInteraction(context.Background(), interaction, mock, ssmClient)
	if !mock.stopCalled {
		t.Error("StopInstances should be called when authorized user via Member.User")
	}
	_ = resp
}

func TestHandleInteraction_StatusNotFound(t *testing.T) {
	ssmClient := ssmWithGuild("g1")
	mock := &mockEC2Client{describeOutput: emptyDescribeOutput()}

	resp := handleInteraction(context.Background(), interactionWith("mc", "status", "", "g1"), mock, ssmClient)

	ir := parseInteractionResponse(t, resp)
	if ir.Data == nil || !strings.Contains(ir.Data.Content, "No server found") {
		t.Errorf("expected 'No server found' for missing instance, got: %v", ir.Data)
	}
}

func TestHandleInteraction_StatusEC2Error(t *testing.T) {
	ssmClient := ssmWithGuild("g1")
	mock := &mockEC2Client{describeErr: fmt.Errorf("connection refused")}

	resp := handleInteraction(context.Background(), interactionWith("mc", "status", "", "g1"), mock, ssmClient)

	ir := parseInteractionResponse(t, resp)
	if ir.Data == nil || !strings.Contains(ir.Data.Content, "Error") {
		t.Errorf("expected error message, got: %v", ir.Data)
	}
}
