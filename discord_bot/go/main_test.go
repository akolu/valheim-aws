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

	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
)

// mockEC2Client implements EC2API for tests without requiring AWS credentials.
type mockEC2Client struct {
	describeOutput *ec2.DescribeInstancesOutput
	describeErr    error
	startErr       error
	stopErr        error

	startCalled bool
	stopCalled  bool
}

func (m *mockEC2Client) DescribeInstances(_ context.Context, _ *ec2.DescribeInstancesInput, _ ...func(*ec2.Options)) (*ec2.DescribeInstancesOutput, error) {
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

func runningInstance(ip string) *ec2.DescribeInstancesOutput {
	launchTime := time.Now().Add(-10 * time.Minute)
	t2micro := ec2types.InstanceTypeT2Micro
	return &ec2.DescribeInstancesOutput{
		Reservations: []ec2types.Reservation{
			{
				Instances: []ec2types.Instance{
					{
						State:           &ec2types.InstanceState{Name: ec2types.InstanceStateNameRunning},
						InstanceType:    t2micro,
						PublicIpAddress: &ip,
						LaunchTime:      &launchTime,
					},
				},
			},
		},
	}
}

func stoppedInstance() *ec2.DescribeInstancesOutput {
	return &ec2.DescribeInstancesOutput{
		Reservations: []ec2types.Reservation{
			{
				Instances: []ec2types.Instance{
					{
						State:        &ec2types.InstanceState{Name: ec2types.InstanceStateNameStopped},
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

// --- isAuthorized ---

func TestIsAuthorized_EmptyDeniesAll(t *testing.T) {
	t.Setenv("AUTHORIZED_USERS", "")
	if isAuthorized("anyuser") {
		t.Error("empty AUTHORIZED_USERS should deny all users")
	}
}

func TestIsAuthorized_SingleUser(t *testing.T) {
	t.Setenv("AUTHORIZED_USERS", "user123")
	if !isAuthorized("user123") {
		t.Error("user123 should be authorized")
	}
	if isAuthorized("other") {
		t.Error("other should not be authorized")
	}
}

func TestIsAuthorized_MultipleUsers(t *testing.T) {
	t.Setenv("AUTHORIZED_USERS", "alice,bob,charlie")
	for _, id := range []string{"alice", "bob", "charlie"} {
		if !isAuthorized(id) {
			t.Errorf("%s should be authorized", id)
		}
	}
	if isAuthorized("dave") {
		t.Error("dave should not be authorized")
	}
}

func TestIsAuthorized_TrimsSpaces(t *testing.T) {
	t.Setenv("AUTHORIZED_USERS", " alice , bob ")
	if !isAuthorized("alice") {
		t.Error("alice (with surrounding spaces in env) should be authorized")
	}
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

	// Sign with a different key
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
			"x-signature-ed25519":  sig,
			"x-signature-timestamp": timestamp,
		},
		Body: body,
	}
}

func TestHandler_Ping(t *testing.T) {
	pub, priv := generateTestKey(t)
	t.Setenv("DISCORD_PUBLIC_KEY", hex.EncodeToString(pub))
	t.Setenv("GAME_NAME", "testgame")

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

// --- Command routing via handleInteraction ---

func interactionWith(gameName, action, userID string) Interaction {
	i := Interaction{
		Type: 2,
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

func TestHandleInteraction_StatusCommand(t *testing.T) {
	t.Setenv("GAME_NAME", "mc")
	t.Setenv("INSTANCE_ID", "i-test")

	mock := &mockEC2Client{describeOutput: stoppedInstance()}
	resp := handleInteraction(context.Background(), interactionWith("mc", "status", ""), mock)

	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	ir := parseInteractionResponse(t, resp)
	if ir.Data == nil || !strings.Contains(ir.Data.Content, "stopped") {
		t.Errorf("expected status to mention 'stopped', got: %v", ir.Data)
	}
	// Should be ephemeral (flags=64)
	if ir.Data.Flags != 64 {
		t.Errorf("expected ephemeral flags=64, got %d", ir.Data.Flags)
	}
}

func TestHandleInteraction_StatusRunning(t *testing.T) {
	t.Setenv("GAME_NAME", "mc")
	t.Setenv("INSTANCE_ID", "i-test")

	mock := &mockEC2Client{describeOutput: runningInstance("1.2.3.4")}
	resp := handleInteraction(context.Background(), interactionWith("mc", "status", ""), mock)

	ir := parseInteractionResponse(t, resp)
	if ir.Data == nil || !strings.Contains(ir.Data.Content, "running") {
		t.Errorf("expected status to mention 'running', got: %v", ir.Data)
	}
	if !strings.Contains(ir.Data.Content, "1.2.3.4") {
		t.Errorf("expected IP address in status, got: %s", ir.Data.Content)
	}
}

func TestHandleInteraction_StartUnauthorized(t *testing.T) {
	t.Setenv("GAME_NAME", "mc")
	t.Setenv("AUTHORIZED_USERS", "admin")
	t.Setenv("INSTANCE_ID", "i-test")

	mock := &mockEC2Client{}
	resp := handleInteraction(context.Background(), interactionWith("mc", "start", "notadmin"), mock)

	ir := parseInteractionResponse(t, resp)
	if ir.Data == nil || !strings.Contains(ir.Data.Content, "permission") {
		t.Errorf("expected permission error, got: %v", ir.Data)
	}
	if mock.startCalled {
		t.Error("StartInstances should not be called for unauthorized user")
	}
	// Should be public (not ephemeral)
	if ir.Data.Flags == 64 {
		t.Error("unauthorized message should be public, not ephemeral")
	}
}

func TestHandleInteraction_StartAuthorized_AlreadyRunning(t *testing.T) {
	t.Setenv("GAME_NAME", "mc")
	t.Setenv("AUTHORIZED_USERS", "admin")
	t.Setenv("INSTANCE_ID", "i-test")

	mock := &mockEC2Client{describeOutput: runningInstance("5.6.7.8")}
	resp := handleInteraction(context.Background(), interactionWith("mc", "start", "admin"), mock)

	ir := parseInteractionResponse(t, resp)
	if ir.Data == nil || !strings.Contains(ir.Data.Content, "already running") {
		t.Errorf("expected 'already running' message, got: %v", ir.Data)
	}
	if mock.startCalled {
		t.Error("StartInstances should not be called when already running")
	}
}

func TestHandleInteraction_StartAuthorized_Stopped(t *testing.T) {
	t.Setenv("GAME_NAME", "mc")
	t.Setenv("AUTHORIZED_USERS", "admin")
	t.Setenv("INSTANCE_ID", "i-test")

	mock := &mockEC2Client{describeOutput: stoppedInstance()}
	resp := handleInteraction(context.Background(), interactionWith("mc", "start", "admin"), mock)

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
	t.Setenv("GAME_NAME", "mc")
	t.Setenv("AUTHORIZED_USERS", "admin")
	t.Setenv("INSTANCE_ID", "i-test")

	mock := &mockEC2Client{}
	resp := handleInteraction(context.Background(), interactionWith("mc", "stop", "notadmin"), mock)

	ir := parseInteractionResponse(t, resp)
	if ir.Data == nil || !strings.Contains(ir.Data.Content, "permission") {
		t.Errorf("expected permission error, got: %v", ir.Data)
	}
	if mock.stopCalled {
		t.Error("StopInstances should not be called for unauthorized user")
	}
}

func TestHandleInteraction_StopAuthorized(t *testing.T) {
	t.Setenv("GAME_NAME", "mc")
	t.Setenv("AUTHORIZED_USERS", "admin")
	t.Setenv("INSTANCE_ID", "i-test")

	mock := &mockEC2Client{}
	resp := handleInteraction(context.Background(), interactionWith("mc", "stop", "admin"), mock)

	if !mock.stopCalled {
		t.Error("StopInstances should be called for authorized user")
	}
	ir := parseInteractionResponse(t, resp)
	if ir.Data == nil || !strings.Contains(ir.Data.Content, "stopping") {
		t.Errorf("expected 'stopping' message, got: %v", ir.Data)
	}
}

func TestHandleInteraction_HelpCommand(t *testing.T) {
	t.Setenv("GAME_NAME", "mc")

	mock := &mockEC2Client{}
	resp := handleInteraction(context.Background(), interactionWith("mc", "help", ""), mock)

	ir := parseInteractionResponse(t, resp)
	if ir.Data == nil || !strings.Contains(ir.Data.Content, "Commands") {
		t.Errorf("expected help text with 'Commands', got: %v", ir.Data)
	}
	if ir.Data.Flags != 64 {
		t.Error("help should be ephemeral")
	}
}

func TestHandleInteraction_UnknownCommand(t *testing.T) {
	t.Setenv("GAME_NAME", "mc")

	mock := &mockEC2Client{}
	resp := handleInteraction(context.Background(), interactionWith("mc", "bogus", ""), mock)

	ir := parseInteractionResponse(t, resp)
	if ir.Data == nil || !strings.Contains(ir.Data.Content, "Unknown") {
		t.Errorf("expected 'Unknown' in response, got: %v", ir.Data)
	}
}

func TestHandleInteraction_WrongGameName(t *testing.T) {
	t.Setenv("GAME_NAME", "mc")

	mock := &mockEC2Client{}
	resp := handleInteraction(context.Background(), interactionWith("othergame", "status", ""), mock)

	ir := parseInteractionResponse(t, resp)
	if ir.Data == nil || !strings.Contains(ir.Data.Content, "Unknown") {
		t.Errorf("expected 'Unknown' for wrong game name, got: %v", ir.Data)
	}
}

func TestHandleInteraction_NonSlashCommand(t *testing.T) {
	t.Setenv("GAME_NAME", "mc")

	mock := &mockEC2Client{}
	interaction := Interaction{Type: 3} // not type 2
	resp := handleInteraction(context.Background(), interaction, mock)

	if resp.StatusCode != 400 {
		t.Errorf("expected 400 for non-slash command, got %d", resp.StatusCode)
	}
}

func TestHandleInteraction_MemberUserFallback(t *testing.T) {
	t.Setenv("GAME_NAME", "mc")
	t.Setenv("AUTHORIZED_USERS", "admin")
	t.Setenv("INSTANCE_ID", "i-test")

	mock := &mockEC2Client{}
	// User ID via Member.User (guild context)
	interaction := Interaction{
		Type: 2,
		Data: InteractionData{
			Name:    "mc",
			Options: []SubCommand{{Name: "stop"}},
		},
		Member: &Member{User: &User{ID: "admin"}},
	}

	resp := handleInteraction(context.Background(), interaction, mock)
	if !mock.stopCalled {
		t.Error("StopInstances should be called when authorized user via Member.User")
	}
	_ = resp
}

func TestHandleInteraction_StatusNotFound(t *testing.T) {
	t.Setenv("GAME_NAME", "mc")
	t.Setenv("INSTANCE_ID", "i-test")

	mock := &mockEC2Client{describeOutput: emptyDescribeOutput()}
	resp := handleInteraction(context.Background(), interactionWith("mc", "status", ""), mock)

	ir := parseInteractionResponse(t, resp)
	if ir.Data == nil || !strings.Contains(ir.Data.Content, "not found") {
		t.Errorf("expected 'not found' for missing instance, got: %v", ir.Data)
	}
}

func TestHandleInteraction_StatusEC2Error(t *testing.T) {
	t.Setenv("GAME_NAME", "mc")
	t.Setenv("INSTANCE_ID", "i-test")

	mock := &mockEC2Client{describeErr: fmt.Errorf("connection refused")}
	resp := handleInteraction(context.Background(), interactionWith("mc", "status", ""), mock)

	ir := parseInteractionResponse(t, resp)
	if ir.Data == nil || !strings.Contains(ir.Data.Content, "Error") {
		t.Errorf("expected error message, got: %v", ir.Data)
	}
}
