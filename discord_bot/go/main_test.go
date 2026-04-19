package main

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	lambdaapi "github.com/aws/aws-sdk-go-v2/service/lambda"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	ssmtypes "github.com/aws/aws-sdk-go-v2/service/ssm/types"
)

// mockEC2Client implements EC2API for tests without requiring AWS credentials.
// describeOutputs is a sequence of outputs returned on successive DescribeInstances
// calls, simulating a state transition over the poll loop. The last element is
// reused once exhausted.
type mockEC2Client struct {
	describeOutput  *ec2.DescribeInstancesOutput
	describeOutputs []*ec2.DescribeInstancesOutput
	describeIdx     int
	describeErr     error
	startErr        error
	stopErr         error

	startCalled bool
	stopCalled  bool

	// capturedDescribeInput records the last DescribeInstances call for assertions
	capturedDescribeInput *ec2.DescribeInstancesInput
}

func (m *mockEC2Client) DescribeInstances(_ context.Context, input *ec2.DescribeInstancesInput, _ ...func(*ec2.Options)) (*ec2.DescribeInstancesOutput, error) {
	m.capturedDescribeInput = input
	if m.describeErr != nil {
		return nil, m.describeErr
	}
	if len(m.describeOutputs) > 0 {
		if m.describeIdx >= len(m.describeOutputs) {
			m.describeIdx = len(m.describeOutputs) - 1
		}
		out := m.describeOutputs[m.describeIdx]
		m.describeIdx++
		return out, nil
	}
	return m.describeOutput, nil
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

// mockS3Client implements S3API for tests.
type mockS3Client struct {
	output *s3.ListObjectsV2Output
	err    error
	calls  int32
}

func (m *mockS3Client) ListObjectsV2(_ context.Context, _ *s3.ListObjectsV2Input, _ ...func(*s3.Options)) (*s3.ListObjectsV2Output, error) {
	atomic.AddInt32(&m.calls, 1)
	if m.err != nil {
		return nil, m.err
	}
	return m.output, nil
}

// mockLambdaClient implements LambdaAPI for tests of the self-invoke dispatch path.
type mockLambdaClient struct {
	invokeErr    error
	invokeStatus int32 // if 0, treated as 202 (success) on no-err

	mu                 sync.Mutex
	invokeCalled       bool
	invokeCalls        int
	lastInvokeInput    *lambdaapi.InvokeInput
	lastInvokePayload  []byte
}

func (m *mockLambdaClient) Invoke(_ context.Context, input *lambdaapi.InvokeInput, _ ...func(*lambdaapi.Options)) (*lambdaapi.InvokeOutput, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.invokeCalled = true
	m.invokeCalls++
	m.lastInvokeInput = input
	if input != nil && input.Payload != nil {
		m.lastInvokePayload = append([]byte(nil), input.Payload...)
	}
	if m.invokeErr != nil {
		return nil, m.invokeErr
	}
	status := m.invokeStatus
	if status == 0 {
		status = 202
	}
	return &lambdaapi.InvokeOutput{StatusCode: status}, nil
}

// callHandler marshals a LambdaRequest to JSON and invokes the top-level
// handler, which now takes json.RawMessage. Used by the PING / signature /
// ack-path tests.
func callHandler(t *testing.T, req LambdaRequest) (LambdaResponse, error) {
	t.Helper()
	raw, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal LambdaRequest: %v", err)
	}
	return handler(context.Background(), raw)
}

func s3OutputWithLastModified(at time.Time) *s3.ListObjectsV2Output {
	return &s3.ListObjectsV2Output{
		Contents: []s3types.Object{
			{Key: aws.String("valheim_backup_latest.tar.gz"), LastModified: aws.Time(at)},
		},
	}
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

func pendingInstanceWithID(id string) *ec2.DescribeInstancesOutput {
	launchTime := time.Now().Add(-30 * time.Second)
	return &ec2.DescribeInstancesOutput{
		Reservations: []ec2types.Reservation{
			{
				Instances: []ec2types.Instance{
					{
						InstanceId:   aws.String(id),
						State:        &ec2types.InstanceState{Name: ec2types.InstanceStateNamePending},
						InstanceType: ec2types.InstanceTypeT2Micro,
						LaunchTime:   &launchTime,
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
		"/bonfire/allowed_guilds":                guildID,
		"/bonfire/" + game + "/authorized_users": users,
	}}
}

// firstEmbed returns the first embed in the response, or fails the test.
func firstEmbed(t *testing.T, ir InteractionResponse) Embed {
	t.Helper()
	if ir.Data == nil || len(ir.Data.Embeds) == 0 {
		t.Fatalf("expected at least one embed in response, got: %+v", ir.Data)
	}
	return ir.Data.Embeds[0]
}

// embedBody returns the Title + Description + Footer concatenated so tests can
// make substring assertions on the full visible surface of an embed.
func embedBody(e Embed) string {
	parts := []string{e.Title, e.Description}
	for _, f := range e.Fields {
		parts = append(parts, f.Name, f.Value)
	}
	if e.Footer != nil {
		parts = append(parts, e.Footer.Text)
	}
	return strings.Join(parts, " · ")
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

	resp, err := callHandler(t, req)
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
	resp, err := callHandler(t, req)
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

	resp, err := callHandler(t, req)
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

// runHandle is a convenience wrapper for tests that don't care about the
// self-invoke path — injects a recording lambda mock that is verified only
// when tests explicitly check it.
func runHandle(ctx context.Context, interaction Interaction, ec2Client EC2API, ssmClient SSMAPI) LambdaResponse {
	return handleInteraction(ctx, interaction, ec2Client, ssmClient, nil, &mockLambdaClient{}, "bonfire_bot")
}

// --- Guild allowlist tests ---

func TestGuildAllowlist_AllowedGuild(t *testing.T) {
	ssmClient := ssmWithGuild("guild123")
	mock := &mockEC2Client{describeOutput: stoppedInstanceWithID("i-test")}

	interaction := interactionWith("valheim", "status", "", "guild123")
	resp := runHandle(context.Background(), interaction, mock, ssmClient)

	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	ir := parseInteractionResponse(t, resp)
	// Allowlisted guild should NOT get the "not a fire in this server" alert.
	if ir.Data != nil && len(ir.Data.Embeds) > 0 && strings.Contains(embedBody(ir.Data.Embeds[0]), copyAlertGuildBlocked) {
		t.Errorf("allowlisted guild should not be rejected, got: %s", embedBody(ir.Data.Embeds[0]))
	}
}

func TestGuildAllowlist_BlockedGuild(t *testing.T) {
	ssmClient := ssmWithGuild("guild123")
	mock := &mockEC2Client{}

	interaction := interactionWith("valheim", "status", "", "other-guild")
	resp := runHandle(context.Background(), interaction, mock, ssmClient)

	if resp.StatusCode != 200 {
		t.Errorf("expected 200 (Discord requires 200 even for rejections), got %d", resp.StatusCode)
	}
	ir := parseInteractionResponse(t, resp)
	embed := firstEmbed(t, ir)
	if !strings.Contains(embedBody(embed), copyAlertGuildBlocked) {
		t.Errorf("non-allowlisted guild should be rejected with guild-blocked copy, got: %s", embedBody(embed))
	}
	if ir.Data.Flags != discordEphemeralFlag {
		t.Error("guild rejection should be ephemeral (flags=64)")
	}
}

func TestGuildAllowlist_EmptyListBlocksAll(t *testing.T) {
	ssmClient := &mockSSMClient{params: map[string]string{
		"/bonfire/allowed_guilds": "",
	}}
	mock := &mockEC2Client{}

	interaction := interactionWith("valheim", "status", "", "any-guild")
	resp := runHandle(context.Background(), interaction, mock, ssmClient)

	ir := parseInteractionResponse(t, resp)
	if !strings.Contains(embedBody(firstEmbed(t, ir)), copyAlertGuildBlocked) {
		t.Errorf("empty allowlist should block all guilds")
	}
}

func TestGuildAllowlist_AbsentParameterBlocksAll(t *testing.T) {
	ssmClient := &mockSSMClient{params: map[string]string{}}
	mock := &mockEC2Client{}

	interaction := interactionWith("valheim", "status", "", "any-guild")
	resp := runHandle(context.Background(), interaction, mock, ssmClient)

	ir := parseInteractionResponse(t, resp)
	if !strings.Contains(embedBody(firstEmbed(t, ir)), copyAlertGuildBlocked) {
		t.Errorf("absent allowlist should block all guilds")
	}
}

func TestGuildAllowlist_PingSkipsCheck(t *testing.T) {
	pub, priv := generateTestKey(t)
	t.Setenv("DISCORD_PUBLIC_KEY", hex.EncodeToString(pub))

	body := `{"type":1,"guild_id":"unknown-guild"}`
	req := makeSignedRequest(t, pub, priv, body)

	resp, err := callHandler(t, req)
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

// --- Tag-based instance discovery ---

func TestHandleInteraction_TagDiscovery_Found(t *testing.T) {
	ssmClient := ssmWithGuild("g1")
	mock := &mockEC2Client{describeOutput: stoppedInstanceWithID("i-abc")}

	interaction := interactionWith("valheim", "status", "", "g1")
	resp := runHandle(context.Background(), interaction, mock, ssmClient)

	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	ir := parseInteractionResponse(t, resp)
	embed := firstEmbed(t, ir)
	// Stopped state: pill should read "out".
	if !strings.Contains(embedBody(embed), labelStopped) {
		t.Errorf("expected stopped state in embed, got: %s", embedBody(embed))
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
	resp := runHandle(context.Background(), interaction, mock, ssmClient)

	ir := parseInteractionResponse(t, resp)
	embed := firstEmbed(t, ir)
	if !strings.Contains(embedBody(embed), copyAlertNoSuchFire) {
		t.Errorf("expected 'no such fire' Alert, got: %s", embedBody(embed))
	}
	if embed.Footer == nil || !strings.Contains(embed.Footer.Text, "try · /valheim status") {
		t.Errorf("expected try-hint footer, got: %+v", embed.Footer)
	}
}

func TestHandleInteraction_TagDiscovery_Multiple(t *testing.T) {
	ssmClient := ssmWithGuild("g1")
	mock := &mockEC2Client{describeOutput: twoInstancesOutput()}

	interaction := interactionWith("valheim", "status", "", "g1")
	resp := runHandle(context.Background(), interaction, mock, ssmClient)

	ir := parseInteractionResponse(t, resp)
	embed := firstEmbed(t, ir)
	if !strings.Contains(embedBody(embed), copyAlertTwoFires) {
		t.Errorf("expected 'two fires' Alert, got: %s", embedBody(embed))
	}
	if embed.Footer == nil || embed.Footer.Text != copyHintTagCollision {
		t.Errorf("expected ec2_tag_collision hint, got: %+v", embed.Footer)
	}
}

// --- SSM authorization ---

func TestSSMAuth_Present_Authorized(t *testing.T) {
	ssmClient := ssmWithGuildAndUsers("g1", "valheim", "admin,user2")
	mock := &mockEC2Client{describeOutput: stoppedInstanceWithID("i-test")}

	interaction := interactionWith("valheim", "start", "admin", "g1")
	lambdaMock := &mockLambdaClient{}
	resp := handleInteraction(context.Background(), interaction, mock, ssmClient, nil, lambdaMock, "bonfire_bot")
	// Authorized + stopped → StartInstances is called, self-invoke dispatched, deferred response returned.
	if !mock.startCalled {
		t.Error("StartInstances should be called for authorized user")
	}
	if !lambdaMock.invokeCalled {
		t.Error("expected self-invoke dispatch for authorized /start of stopped server")
	}
	ir := parseInteractionResponse(t, resp)
	if ir.Type != discordDeferredChannelMessage {
		t.Errorf("expected type 5 deferred response, got %d", ir.Type)
	}
}

func TestSSMAuth_Present_Unauthorized(t *testing.T) {
	ssmClient := ssmWithGuildAndUsers("g1", "valheim", "admin")
	mock := &mockEC2Client{}

	interaction := interactionWith("valheim", "start", "stranger", "g1")
	resp := runHandle(context.Background(), interaction, mock, ssmClient)

	ir := parseInteractionResponse(t, resp)
	embed := firstEmbed(t, ir)
	if !strings.Contains(embedBody(embed), copyAlertUnauthorizedHeadline) {
		t.Errorf("expected 'can't tend this fire' alert, got: %s", embedBody(embed))
	}
	if mock.startCalled {
		t.Error("StartInstances should not be called for unauthorized user")
	}
}

func TestSSMAuth_Absent_DeniesAll(t *testing.T) {
	ssmClient := ssmWithGuild("g1")
	mock := &mockEC2Client{}

	interaction := interactionWith("valheim", "start", "admin", "g1")
	resp := runHandle(context.Background(), interaction, mock, ssmClient)

	ir := parseInteractionResponse(t, resp)
	embed := firstEmbed(t, ir)
	if !strings.Contains(embedBody(embed), copyAlertUnauthorizedHeadline) {
		t.Errorf("absent authorized_users should deny all")
	}
	if mock.startCalled {
		t.Error("StartInstances should not be called when authorized_users is absent")
	}
}

// --- Status command ---

func TestHandleInteraction_StatusCommand(t *testing.T) {
	ssmClient := ssmWithGuild("g1")
	mock := &mockEC2Client{describeOutput: stoppedInstanceWithID("i-test")}

	resp := runHandle(context.Background(), interactionWith("mc", "status", "", "g1"), mock, ssmClient)

	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	ir := parseInteractionResponse(t, resp)
	embed := firstEmbed(t, ir)
	if !strings.Contains(embedBody(embed), labelStopped) {
		t.Errorf("expected stopped state label, got: %s", embedBody(embed))
	}
	if ir.Data.Flags != discordEphemeralFlag {
		t.Errorf("expected ephemeral flags=64, got %d", ir.Data.Flags)
	}
}

func TestHandleInteraction_StatusRunning(t *testing.T) {
	ssmClient := ssmWithGuild("g1")
	mock := &mockEC2Client{describeOutput: runningInstanceWithID("i-test", "1.2.3.4")}

	resp := runHandle(context.Background(), interactionWith("mc", "status", "", "g1"), mock, ssmClient)

	ir := parseInteractionResponse(t, resp)
	embed := firstEmbed(t, ir)
	if !strings.Contains(embedBody(embed), labelRunning) {
		t.Errorf("expected burning label, got: %s", embedBody(embed))
	}
	if !strings.Contains(embedBody(embed), "1.2.3.4") {
		t.Errorf("expected IP address in status, got: %s", embedBody(embed))
	}
	// Brand palette: running is ember.
	if embed.Color != colorEmber {
		t.Errorf("expected ember color for running, got 0x%x", embed.Color)
	}
}

func TestHandleInteraction_StatusStoppedNeverBurned(t *testing.T) {
	ssmClient := ssmWithGuild("g1")
	mock := &mockEC2Client{describeOutput: stoppedInstanceWithID("i-test")}
	s3Mock := &mockS3Client{output: &s3.ListObjectsV2Output{}} // empty bucket

	resp := handleInteraction(context.Background(), interactionWith("mc", "status", "", "g1"), mock, ssmClient, s3Mock, &mockLambdaClient{}, "bonfire_bot")
	ir := parseInteractionResponse(t, resp)
	embed := firstEmbed(t, ir)
	if !strings.Contains(embedBody(embed), "never burned") {
		t.Errorf("expected 'never burned' fallback for empty bucket, got: %s", embedBody(embed))
	}
}

// --- Start/Stop idempotency ---

func TestHandleInteraction_StartUnauthorized(t *testing.T) {
	ssmClient := ssmWithGuildAndUsers("g1", "mc", "admin")
	mock := &mockEC2Client{}

	resp := runHandle(context.Background(), interactionWith("mc", "start", "notadmin", "g1"), mock, ssmClient)

	ir := parseInteractionResponse(t, resp)
	embed := firstEmbed(t, ir)
	if !strings.Contains(embedBody(embed), copyAlertUnauthorizedHeadline) {
		t.Errorf("expected unauthorized alert, got: %s", embedBody(embed))
	}
	if mock.startCalled {
		t.Error("StartInstances should not be called for unauthorized user")
	}
	// Alert should be ephemeral per brand (lives next to the other quiet refusals).
	if ir.Data.Flags != discordEphemeralFlag {
		t.Error("unauthorized alert should be ephemeral")
	}
}

func TestHandleInteraction_StartAuthorized_AlreadyRunning(t *testing.T) {
	ssmClient := ssmWithGuildAndUsers("g1", "mc", "admin")
	mock := &mockEC2Client{describeOutput: runningInstanceWithID("i-test", "5.6.7.8")}

	resp := runHandle(context.Background(), interactionWith("mc", "start", "admin", "g1"), mock, ssmClient)

	ir := parseInteractionResponse(t, resp)
	embed := firstEmbed(t, ir)
	if !strings.Contains(embedBody(embed), "fire's already ● burning") {
		t.Errorf("expected 'fire's already ● burning' idempotency copy, got: %s", embedBody(embed))
	}
	if !strings.Contains(embedBody(embed), "5.6.7.8") {
		t.Errorf("expected bare address in body, got: %s", embedBody(embed))
	}
	if mock.startCalled {
		t.Error("StartInstances should not be called when already running")
	}
	if ir.Data.Flags != discordEphemeralFlag {
		t.Error("idempotent already-running message should be ephemeral")
	}
}

func TestHandleInteraction_StartAuthorized_AlreadyLighting(t *testing.T) {
	ssmClient := ssmWithGuildAndUsers("g1", "mc", "admin")
	mock := &mockEC2Client{describeOutput: pendingInstanceWithID("i-test")}

	resp := runHandle(context.Background(), interactionWith("mc", "start", "admin", "g1"), mock, ssmClient)

	ir := parseInteractionResponse(t, resp)
	embed := firstEmbed(t, ir)
	if !strings.Contains(embedBody(embed), "fire's already ● lighting") {
		t.Errorf("expected 'fire's already ● lighting' idempotency copy, got: %s", embedBody(embed))
	}
	if mock.startCalled {
		t.Error("StartInstances should not be called when already pending")
	}
}

func TestHandleInteraction_StartWhileStopping(t *testing.T) {
	ssmClient := ssmWithGuildAndUsers("g1", "mc", "admin")
	mock := &mockEC2Client{describeOutput: stoppingInstanceWithID("i-test")}

	resp := runHandle(context.Background(), interactionWith("mc", "start", "admin", "g1"), mock, ssmClient)

	ir := parseInteractionResponse(t, resp)
	embed := firstEmbed(t, ir)
	if !strings.Contains(embedBody(embed), "hang on") {
		t.Errorf("expected 'hang on' copy when /start while stopping, got: %s", embedBody(embed))
	}
	if mock.startCalled {
		t.Error("StartInstances should not be called while stopping")
	}
}

func TestHandleInteraction_StartAuthorized_Stopped_DefersAndStarts(t *testing.T) {
	ssmClient := ssmWithGuildAndUsers("g1", "mc", "admin")
	mock := &mockEC2Client{describeOutput: stoppedInstanceWithID("i-test")}
	lambdaMock := &mockLambdaClient{}

	resp := handleInteraction(context.Background(), interactionWith("mc", "start", "admin", "g1"), mock, ssmClient, nil, lambdaMock, "bonfire_bot")

	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	if !mock.startCalled {
		t.Error("StartInstances should be called for authorized user starting stopped server")
	}
	if !lambdaMock.invokeCalled {
		t.Fatal("expected self-invoke dispatch for stopped→start transition")
	}
	ir := parseInteractionResponse(t, resp)
	if ir.Type != discordDeferredChannelMessage {
		t.Errorf("expected deferred (type 5) response, got type %d", ir.Type)
	}
}

func TestHandleInteraction_StopUnauthorized(t *testing.T) {
	ssmClient := ssmWithGuildAndUsers("g1", "mc", "admin")
	mock := &mockEC2Client{}

	resp := runHandle(context.Background(), interactionWith("mc", "stop", "notadmin", "g1"), mock, ssmClient)

	ir := parseInteractionResponse(t, resp)
	embed := firstEmbed(t, ir)
	if !strings.Contains(embedBody(embed), copyAlertUnauthorizedHeadline) {
		t.Errorf("expected unauthorized alert, got: %s", embedBody(embed))
	}
	if mock.stopCalled {
		t.Error("StopInstances should not be called for unauthorized user")
	}
}

func TestHandleInteraction_StopAuthorized_DefersAndStops(t *testing.T) {
	ssmClient := ssmWithGuildAndUsers("g1", "mc", "admin")
	mock := &mockEC2Client{describeOutput: runningInstanceWithID("i-test", "1.2.3.4")}
	lambdaMock := &mockLambdaClient{}

	resp := handleInteraction(context.Background(), interactionWith("mc", "stop", "admin", "g1"), mock, ssmClient, nil, lambdaMock, "bonfire_bot")

	if !mock.stopCalled {
		t.Error("StopInstances should be called for authorized user stopping running server")
	}
	if !lambdaMock.invokeCalled {
		t.Fatal("expected self-invoke dispatch for /stop of running server")
	}
	ir := parseInteractionResponse(t, resp)
	if ir.Type != discordDeferredChannelMessage {
		t.Errorf("expected deferred response, got type %d", ir.Type)
	}
}

func TestHandleInteraction_StopAuthorized_AlreadyOut(t *testing.T) {
	ssmClient := ssmWithGuildAndUsers("g1", "mc", "admin")
	mock := &mockEC2Client{describeOutput: stoppedInstanceWithID("i-test")}

	resp := runHandle(context.Background(), interactionWith("mc", "stop", "admin", "g1"), mock, ssmClient)

	ir := parseInteractionResponse(t, resp)
	embed := firstEmbed(t, ir)
	if !strings.Contains(embedBody(embed), "fire's already ● out") {
		t.Errorf("expected 'fire's already ● out' copy, got: %s", embedBody(embed))
	}
	if mock.stopCalled {
		t.Error("StopInstances should not be called when already stopped")
	}
}

func TestHandleInteraction_StopAuthorized_AlreadyDyingDown(t *testing.T) {
	ssmClient := ssmWithGuildAndUsers("g1", "mc", "admin")
	mock := &mockEC2Client{describeOutput: stoppingInstanceWithID("i-test")}

	resp := runHandle(context.Background(), interactionWith("mc", "stop", "admin", "g1"), mock, ssmClient)

	ir := parseInteractionResponse(t, resp)
	embed := firstEmbed(t, ir)
	if !strings.Contains(embedBody(embed), "fire's already ● dying down") {
		t.Errorf("expected 'fire's already ● dying down' idempotency copy, got: %s", embedBody(embed))
	}
	if mock.stopCalled {
		t.Error("StopInstances should not be called when already stopping")
	}
}

func TestHandleInteraction_StopWhilePending(t *testing.T) {
	ssmClient := ssmWithGuildAndUsers("g1", "mc", "admin")
	mock := &mockEC2Client{describeOutput: pendingInstanceWithID("i-test")}

	resp := runHandle(context.Background(), interactionWith("mc", "stop", "admin", "g1"), mock, ssmClient)

	ir := parseInteractionResponse(t, resp)
	embed := firstEmbed(t, ir)
	if !strings.Contains(embedBody(embed), "can't bank coals yet") {
		t.Errorf("expected 'can't bank coals yet' copy, got: %s", embedBody(embed))
	}
	if mock.stopCalled {
		t.Error("StopInstances should not be called while pending")
	}
}

// --- Help command ---

func TestHandleInteraction_HelpCommand(t *testing.T) {
	ssmClient := ssmWithGuild("g1")
	mock := &mockEC2Client{}

	resp := runHandle(context.Background(), interactionWith("mc", "help", "", "g1"), mock, ssmClient)

	ir := parseInteractionResponse(t, resp)
	// Per BRAND.md §"Help", help is plain mono Content — no embed chrome.
	if ir.Data.Content == "" {
		t.Fatal("expected plain text Content in help response, got empty")
	}
	if len(ir.Data.Embeds) != 0 {
		t.Errorf("help should have no embeds (BRAND.md §Help says plain text), got %d", len(ir.Data.Embeds))
	}
	if ir.Data.Flags != discordEphemeralFlag {
		t.Error("help should be ephemeral")
	}
	if !strings.Contains(ir.Data.Content, "light the fire") {
		t.Errorf("expected 'light the fire' in help, got: %s", ir.Data.Content)
	}
	if !strings.Contains(ir.Data.Content, "/mc status") {
		t.Errorf("expected '/mc status' in help, got: %s", ir.Data.Content)
	}
	if !strings.Contains(ir.Data.Content, "bonfire · keeper here") {
		t.Errorf("expected brand-book help opener, got: %s", ir.Data.Content)
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
	resp := runHandle(context.Background(), interaction, mock, ssmClient)

	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	ir := parseInteractionResponse(t, resp)
	embed := firstEmbed(t, ir)
	// Brand §01: non-judgemental phrasing, warm greeting.
	if !strings.Contains(embedBody(embed), "here · ready") {
		t.Errorf("expected brand greeting, got: %s", embedBody(embed))
	}
	if !strings.Contains(embedBody(embed), "keeper list") {
		t.Errorf("authorized user should see keeper-list phrasing, got: %s", embedBody(embed))
	}
	if ir.Data.Flags != discordEphemeralFlag {
		t.Error("hello response should be ephemeral")
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
	resp := runHandle(context.Background(), interaction, mock, ssmClient)

	ir := parseInteractionResponse(t, resp)
	embed := firstEmbed(t, ir)
	// Softened phrasing per plan — no "not authorized" label.
	if strings.Contains(embedBody(embed), "not authorized") {
		t.Errorf("hello should not use judgemental 'not authorized' label, got: %s", embedBody(embed))
	}
	if !strings.Contains(embedBody(embed), "watch") {
		t.Errorf("unauthorized hello should use softened copy, got: %s", embedBody(embed))
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
	resp := runHandle(context.Background(), interaction, mock, ssmClient)

	ir := parseInteractionResponse(t, resp)
	embed := firstEmbed(t, ir)
	if !strings.Contains(embedBody(embed), "keeper list") {
		t.Errorf("authorized guild member should see keeper-list phrasing, got: %s", embedBody(embed))
	}
}

// --- Edge cases ---

func TestHandleInteraction_UnknownAction(t *testing.T) {
	ssmClient := ssmWithGuild("g1")
	mock := &mockEC2Client{}

	resp := runHandle(context.Background(), interactionWith("mc", "bogus", "", "g1"), mock, ssmClient)

	ir := parseInteractionResponse(t, resp)
	embed := firstEmbed(t, ir)
	body := strings.ToLower(embedBody(embed))
	if !strings.Contains(body, "no command") && !strings.Contains(body, "no action") {
		t.Errorf("expected 'no command' / 'no action' refusal copy, got: %s", embedBody(embed))
	}
}

func TestHandleInteraction_NonSlashCommand(t *testing.T) {
	mock := &mockEC2Client{}
	ssmClient := &mockSSMClient{params: map[string]string{}}
	interaction := Interaction{Type: 3}
	resp := handleInteraction(context.Background(), interaction, mock, ssmClient, nil, &mockLambdaClient{}, "bonfire_bot")

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

	lambdaMock := &mockLambdaClient{}
	resp := handleInteraction(context.Background(), interaction, mock, ssmClient, nil, lambdaMock, "bonfire_bot")
	if !mock.stopCalled {
		t.Error("StopInstances should be called when authorized user via Member.User")
	}
	if !lambdaMock.invokeCalled {
		t.Error("expected self-invoke dispatch for /stop of running server")
	}
	_ = resp
}

func TestHandleInteraction_StatusNotFound(t *testing.T) {
	ssmClient := ssmWithGuild("g1")
	mock := &mockEC2Client{describeOutput: emptyDescribeOutput()}

	resp := runHandle(context.Background(), interactionWith("mc", "status", "", "g1"), mock, ssmClient)

	ir := parseInteractionResponse(t, resp)
	embed := firstEmbed(t, ir)
	if !strings.Contains(embedBody(embed), copyAlertNoSuchFire) {
		t.Errorf("expected 'no such fire' alert, got: %s", embedBody(embed))
	}
}

func TestHandleInteraction_StatusEC2Error(t *testing.T) {
	ssmClient := ssmWithGuild("g1")
	mock := &mockEC2Client{describeErr: fmt.Errorf("AccessDenied: User is not authorized")}

	resp := runHandle(context.Background(), interactionWith("mc", "status", "", "g1"), mock, ssmClient)

	ir := parseInteractionResponse(t, resp)
	embed := firstEmbed(t, ir)
	if !strings.Contains(embedBody(embed), copyAlertSomethingSideways) {
		t.Errorf("expected 'something went sideways' alert, got: %s", embedBody(embed))
	}
	if strings.Contains(embedBody(embed), "AccessDenied") {
		t.Errorf("raw AWS error must not be exposed to Discord, got: %s", embedBody(embed))
	}
}

func TestHandleInteraction_StartEC2Error_FindInstance(t *testing.T) {
	ssmClient := ssmWithGuildAndUsers("g1", "mc", "admin")
	mock := &mockEC2Client{describeErr: fmt.Errorf("AccessDenied: User is not authorized")}

	resp := runHandle(context.Background(), interactionWith("mc", "start", "admin", "g1"), mock, ssmClient)

	ir := parseInteractionResponse(t, resp)
	embed := firstEmbed(t, ir)
	if !strings.Contains(embedBody(embed), copyAlertSomethingSideways) {
		t.Errorf("expected generic sideways alert, got: %s", embedBody(embed))
	}
	if strings.Contains(embedBody(embed), "AccessDenied") {
		t.Errorf("raw AWS error must not be exposed to Discord, got: %s", embedBody(embed))
	}
}

func TestHandleInteraction_StartEC2Error_StartInstance(t *testing.T) {
	ssmClient := ssmWithGuildAndUsers("g1", "mc", "admin")
	mock := &mockEC2Client{
		describeOutput: stoppedInstanceWithID("i-test"),
		startErr:       fmt.Errorf("AccessDenied: User is not authorized to call ec2:StartInstances"),
	}

	resp := runHandle(context.Background(), interactionWith("mc", "start", "admin", "g1"), mock, ssmClient)

	ir := parseInteractionResponse(t, resp)
	embed := firstEmbed(t, ir)
	if !strings.Contains(embedBody(embed), copyAlertSomethingSideways) {
		t.Errorf("expected generic sideways alert, got: %s", embedBody(embed))
	}
	if strings.Contains(embedBody(embed), "AccessDenied") {
		t.Errorf("raw AWS error must not be exposed to Discord, got: %s", embedBody(embed))
	}
}

func TestHandleInteraction_StopEC2Error_FindInstance(t *testing.T) {
	ssmClient := ssmWithGuildAndUsers("g1", "mc", "admin")
	mock := &mockEC2Client{describeErr: fmt.Errorf("AccessDenied: User is not authorized")}

	resp := runHandle(context.Background(), interactionWith("mc", "stop", "admin", "g1"), mock, ssmClient)

	ir := parseInteractionResponse(t, resp)
	embed := firstEmbed(t, ir)
	if !strings.Contains(embedBody(embed), copyAlertSomethingSideways) {
		t.Errorf("expected generic sideways alert, got: %s", embedBody(embed))
	}
	if strings.Contains(embedBody(embed), "AccessDenied") {
		t.Errorf("raw AWS error must not be exposed to Discord, got: %s", embedBody(embed))
	}
}

func TestHandleInteraction_StopEC2Error_StopInstance(t *testing.T) {
	ssmClient := ssmWithGuildAndUsers("g1", "mc", "admin")
	mock := &mockEC2Client{
		describeOutput: runningInstanceWithID("i-test", "1.2.3.4"),
		stopErr:        fmt.Errorf("AccessDenied: User is not authorized to call ec2:StopInstances"),
	}

	resp := runHandle(context.Background(), interactionWith("mc", "stop", "admin", "g1"), mock, ssmClient)

	ir := parseInteractionResponse(t, resp)
	embed := firstEmbed(t, ir)
	if !strings.Contains(embedBody(embed), copyAlertSomethingSideways) {
		t.Errorf("expected generic sideways alert, got: %s", embedBody(embed))
	}
	if strings.Contains(embedBody(embed), "AccessDenied") {
		t.Errorf("raw AWS error must not be exposed to Discord, got: %s", embedBody(embed))
	}
}

// --- Embed / color palette tests ---

func TestEmbedPalette_StateColors(t *testing.T) {
	cases := []struct {
		state string
		want  int
	}{
		{"running", colorEmber},
		{"pending", colorSpark},
		{"stopping", colorIce},
		{"stopped", colorAsh},
		{"unknown", colorDanger},
	}
	for _, c := range cases {
		if got := stateColor(c.state); got != c.want {
			t.Errorf("stateColor(%q) = 0x%x, want 0x%x", c.state, got, c.want)
		}
	}
}

func TestFormatElapsed(t *testing.T) {
	cases := []struct {
		dur  time.Duration
		want string
	}{
		{5 * time.Second, "5s"},
		{59 * time.Second, "59s"},
		{90 * time.Second, "1m"}, // 1.5 minutes rounds to 1m
		{5 * time.Minute, "5m"},
		{90 * time.Minute, "1h 30m"},
		{2 * time.Hour, "2h"},
		{25 * time.Hour, "1d 1h"},
		{48 * time.Hour, "2d"},
	}
	for _, c := range cases {
		if got := formatElapsed(c.dur); got != c.want {
			t.Errorf("formatElapsed(%s) = %q, want %q", c.dur, got, c.want)
		}
	}
}

// --- Backup lookup ---

func TestBackupLookup_HappyPath(t *testing.T) {
	at := time.Now().Add(-15 * time.Minute)
	mock := &mockS3Client{output: s3OutputWithLastModified(at)}
	got := backupElapsedString(context.Background(), mock, "valheim", "eu-north-1", "[test] ")
	if !strings.HasSuffix(got, "ago") {
		t.Errorf("expected elapsed string with 'ago' suffix, got %q", got)
	}
}

func TestBackupLookup_EmptyBucket(t *testing.T) {
	mock := &mockS3Client{output: &s3.ListObjectsV2Output{}}
	got := backupElapsedString(context.Background(), mock, "valheim", "eu-north-1", "[test] ")
	if got != "" {
		t.Errorf("empty bucket should yield empty string, got %q", got)
	}
}

func TestBackupLookup_S3Error(t *testing.T) {
	mock := &mockS3Client{err: fmt.Errorf("AccessDenied")}
	got := backupElapsedString(context.Background(), mock, "valheim", "eu-north-1", "[test] ")
	if got != "" {
		t.Errorf("S3 error should yield empty string (logged, not surfaced), got %q", got)
	}
}

// --- Polling loop ---

// stubHTTPClient records PATCH requests so the poll loop's behaviour can be
// asserted without hitting the Discord network.
type stubHTTPClient struct {
	mu       sync.Mutex
	requests []recordedRequest
	// responses are returned in order; the last one is reused once exhausted.
	responses []stubResponse
	idx       int
}

type recordedRequest struct {
	URL  string
	Body WebhookEditBody
}

type stubResponse struct {
	StatusCode int
	RetryAfter string
}

func (s *stubHTTPClient) Do(req *http.Request) (*http.Response, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var body WebhookEditBody
	if req.Body != nil {
		b, _ := io.ReadAll(req.Body)
		_ = json.Unmarshal(b, &body)
	}
	s.requests = append(s.requests, recordedRequest{URL: req.URL.String(), Body: body})

	resp := stubResponse{StatusCode: 200}
	if len(s.responses) > 0 {
		if s.idx >= len(s.responses) {
			s.idx = len(s.responses) - 1
		}
		resp = s.responses[s.idx]
		s.idx++
	}
	header := http.Header{}
	if resp.RetryAfter != "" {
		header.Set("Retry-After", resp.RetryAfter)
	}
	return &http.Response{
		StatusCode: resp.StatusCode,
		Header:     header,
		Body:       io.NopCloser(strings.NewReader("{}")),
	}, nil
}

func (s *stubHTTPClient) Count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.requests)
}

func (s *stubHTTPClient) At(i int) recordedRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.requests[i]
}

func TestPollStart_ReachesRunningTerminal(t *testing.T) {
	launchTime := time.Now().Add(-2 * time.Minute)
	runningOut := &ec2.DescribeInstancesOutput{
		Reservations: []ec2types.Reservation{{
			Instances: []ec2types.Instance{{
				InstanceId:      aws.String("i-abc"),
				State:           &ec2types.InstanceState{Name: ec2types.InstanceStateNameRunning},
				InstanceType:    ec2types.InstanceTypeT2Micro,
				PublicIpAddress: aws.String("10.0.0.1"),
				LaunchTime:      &launchTime,
			}},
		}},
	}
	mock := &mockEC2Client{
		describeOutputs: []*ec2.DescribeInstancesOutput{
			pendingInstanceWithID("i-abc"),
			runningOut,
		},
	}
	http := &stubHTTPClient{}

	cfg := pollConfig{
		Game:              "valheim",
		Action:            "start",
		UserID:            "user42",
		InstanceID:        "i-abc",
		AppID:             "app123",
		Token:             "tok",
		Region:            "eu-north-1",
		EC2Client:         mock,
		HTTPClient:        http,
		EC2PollInterval:   20 * time.Millisecond,
		DiscordPatchFloor: 1 * time.Millisecond,
		DeadlineReserve:   100 * time.Millisecond,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	pollStartFlow(ctx, cfg, time.Now())

	if http.Count() == 0 {
		t.Fatal("expected at least one PATCH to Discord")
	}
	// Last request should be the terminal "running" embed (ember).
	last := http.At(http.Count() - 1)
	if len(last.Body.Embeds) == 0 {
		t.Fatal("terminal PATCH had no embeds")
	}
	final := last.Body.Embeds[0]
	if final.Color != colorEmber {
		t.Errorf("terminal running embed should be ember, got 0x%x", final.Color)
	}
	// Address should appear as a field.
	found := false
	for _, f := range final.Fields {
		if f.Name == fieldAddress {
			found = true
			if !strings.Contains(f.Value, "10.0.0.1") {
				t.Errorf("address field should contain IP, got %q", f.Value)
			}
		}
	}
	if !found {
		t.Error("running terminal should have ADDRESS field")
	}
}

func TestPollStart_InterruptedByStop(t *testing.T) {
	mock := &mockEC2Client{
		describeOutputs: []*ec2.DescribeInstancesOutput{
			pendingInstanceWithID("i-abc"),
			stoppingInstanceWithID("i-abc"),
		},
	}
	http := &stubHTTPClient{}
	cfg := pollConfig{
		Game: "valheim", Action: "start", UserID: "u", InstanceID: "i-abc",
		AppID: "app", Token: "tok", Region: "eu-north-1",
		EC2Client:         mock,
		HTTPClient:        http,
		EC2PollInterval:   10 * time.Millisecond,
		DiscordPatchFloor: 1 * time.Millisecond,
		DeadlineReserve:   50 * time.Millisecond,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	pollStartFlow(ctx, cfg, time.Now())

	last := http.At(http.Count() - 1)
	if last.Body.Embeds[0].Color != colorAsh {
		t.Errorf("interrupted /start should end in ash color, got 0x%x", last.Body.Embeds[0].Color)
	}
	if !strings.Contains(last.Body.Embeds[0].Description, "banked the coals") {
		t.Errorf("expected 'banked the coals' copy, got %q", last.Body.Embeds[0].Description)
	}
}

func TestPollStart_Deadline(t *testing.T) {
	// EC2 never transitions; the ctx deadline fires first.
	mock := &mockEC2Client{describeOutput: pendingInstanceWithID("i-abc")}
	http := &stubHTTPClient{}
	cfg := pollConfig{
		Game: "valheim", Action: "start", UserID: "u", InstanceID: "i-abc",
		AppID: "app", Token: "tok", Region: "eu-north-1",
		EC2Client:         mock,
		HTTPClient:        http,
		EC2PollInterval:   20 * time.Millisecond,
		DiscordPatchFloor: 1 * time.Millisecond,
		DeadlineReserve:   30 * time.Millisecond,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	pollStartFlow(ctx, cfg, time.Now())

	if http.Count() == 0 {
		t.Fatal("expected at least one PATCH")
	}
	last := http.At(http.Count() - 1)
	if last.Body.Embeds[0].Color != colorIce {
		t.Errorf("deadline path should emit ice-colored message, got 0x%x", last.Body.Embeds[0].Color)
	}
	if !strings.Contains(last.Body.Embeds[0].Description, "still lighting") {
		t.Errorf("expected 'still lighting' copy, got %q", last.Body.Embeds[0].Description)
	}
}

func TestPollStop_ReachesStoppedTerminal(t *testing.T) {
	mock := &mockEC2Client{
		describeOutputs: []*ec2.DescribeInstancesOutput{
			stoppingInstanceWithID("i-abc"),
			stoppedInstanceWithID("i-abc"),
		},
	}
	http := &stubHTTPClient{}
	cfg := pollConfig{
		Game: "valheim", Action: "stop", UserID: "user42", InstanceID: "i-abc",
		AppID: "app", Token: "tok", Region: "eu-north-1",
		EC2Client:         mock,
		HTTPClient:        http,
		EC2PollInterval:   10 * time.Millisecond,
		DiscordPatchFloor: 1 * time.Millisecond,
		DeadlineReserve:   50 * time.Millisecond,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	pollStopFlow(ctx, cfg, time.Now())

	last := http.At(http.Count() - 1)
	if last.Body.Embeds[0].Color != colorAsh {
		t.Errorf("terminal stopped should be ash, got 0x%x", last.Body.Embeds[0].Color)
	}
	if !strings.Contains(last.Body.Embeds[0].Description, "put out by") {
		t.Errorf("expected 'put out by' leadline, got %q", last.Body.Embeds[0].Description)
	}
}

func TestWebhookPATCHEndpoint_Shape(t *testing.T) {
	got := webhookPATCHEndpoint("123", "tok")
	want := "https://discord.com/api/v10/webhooks/123/tok/messages/@original"
	if got != want {
		t.Errorf("webhookPATCHEndpoint = %q, want %q", got, want)
	}
}

// --- Amendment 2: async self-invoke handler routing ---

func TestHandler_SelfPollEventShape_DispatchesToPollLoop(t *testing.T) {
	// A self-poll event with all required fields present but an unknown action
	// should be routed to handleSelfPoll, which returns 400 "unknown action".
	// This proves the router took the self-poll branch AND that field
	// validation didn't short-circuit before reaching the action switch.
	raw := json.RawMessage(`{"source":"self-poll","game":"mc","interaction_token":"t","application_id":"a","action":"bogus"}`)
	resp, err := handler(context.Background(), raw)
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	if resp.StatusCode != 400 {
		t.Errorf("expected 400 for unknown self-poll action, got %d — body: %s", resp.StatusCode, resp.Body)
	}
	if !strings.Contains(resp.Body, "unknown action") {
		t.Errorf("expected 'unknown action' in self-poll dispatch body, got: %s", resp.Body)
	}
}

func TestHandler_InteractionEventShape_DispatchesToAck(t *testing.T) {
	// A plain API Gateway-style LambdaRequest (no "source" field) should be routed
	// to the ack path — which without signature headers returns 401.
	req := LambdaRequest{
		Headers: map[string]string{}, // missing signature → ack path rejects with 401
		Body:    `{"type":1}`,
	}
	resp, err := callHandler(t, req)
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	if resp.StatusCode != 401 {
		t.Errorf("expected 401 (ack path, missing signature), got %d — body: %s", resp.StatusCode, resp.Body)
	}
	// The ack path's 401 body includes "Missing signature headers"; the self-poll
	// path never produces that string. Dispatch to ack is confirmed.
	if !strings.Contains(resp.Body, "signature") {
		t.Errorf("expected ack-path signature error in body, got: %s", resp.Body)
	}
}

func TestHandler_SelfPollBranch_DoesNotSelfInvoke(t *testing.T) {
	// Structural guarantee: the self-poll branch doesn't receive a LambdaAPI
	// client, so it structurally cannot call Invoke. This test calls
	// handleSelfPoll directly with a malformed event and asserts the branch
	// returns a response without reaching any Lambda SDK code path.
	raw := json.RawMessage(`{"source":"self-poll","action":"bogus","game":"mc"}`)
	resp, err := handleSelfPoll(context.Background(), raw)
	if err != nil {
		t.Fatalf("handleSelfPoll returned error: %v", err)
	}
	// 400 for unknown action — reached the terminal return without touching a lambda client.
	if resp.StatusCode != 400 {
		t.Errorf("expected 400, got %d — body: %s", resp.StatusCode, resp.Body)
	}
}

func TestHandler_TransitionPath_AcksAfterSelfInvoke(t *testing.T) {
	// Transition path: /start on a stopped instance dispatches exactly one
	// self-invoke with a well-shaped payload and returns Discord type-5 ACK.
	ssmClient := ssmWithGuildAndUsers("g1", "mc", "admin")
	mock := &mockEC2Client{describeOutput: stoppedInstanceWithID("i-test")}
	lambdaMock := &mockLambdaClient{}

	resp := handleInteraction(context.Background(), interactionWith("mc", "start", "admin", "g1"), mock, ssmClient, nil, lambdaMock, "bonfire_bot")

	if lambdaMock.invokeCalls != 1 {
		t.Fatalf("expected exactly 1 Invoke call, got %d", lambdaMock.invokeCalls)
	}
	if lambdaMock.lastInvokeInput == nil {
		t.Fatal("expected Invoke input to be recorded")
	}
	if aws.ToString(lambdaMock.lastInvokeInput.FunctionName) != "bonfire_bot" {
		t.Errorf("expected FunctionName bonfire_bot, got %q", aws.ToString(lambdaMock.lastInvokeInput.FunctionName))
	}
	if string(lambdaMock.lastInvokeInput.InvocationType) != "Event" {
		t.Errorf("expected InvocationType Event, got %q", lambdaMock.lastInvokeInput.InvocationType)
	}
	// Payload shape check
	var payload selfPollEvent
	if err := json.Unmarshal(lambdaMock.lastInvokePayload, &payload); err != nil {
		t.Fatalf("unmarshal self-poll payload: %v", err)
	}
	if payload.Source != selfPollSource {
		t.Errorf("expected source %q, got %q", selfPollSource, payload.Source)
	}
	if payload.Game != "mc" || payload.Action != "start" || payload.User.ID != "admin" {
		t.Errorf("self-poll payload mismatched: %+v", payload)
	}
	if payload.InstanceID != "i-test" {
		t.Errorf("expected instance id i-test, got %q", payload.InstanceID)
	}
	if payload.EnqueuedAt == "" {
		t.Error("expected EnqueuedAt to be set")
	}
	// Response shape
	ir := parseInteractionResponse(t, resp)
	if ir.Type != discordDeferredChannelMessage {
		t.Errorf("expected type 5 deferred response, got %d", ir.Type)
	}
}

func TestHandler_SelfInvokeFailure_ReturnsAlert(t *testing.T) {
	// If the Lambda self-invoke errors (throttle, permission denied, etc.),
	// the ack path returns a type-4 ephemeral Alert with a lambda_invoke hint.
	ssmClient := ssmWithGuildAndUsers("g1", "mc", "admin")
	mock := &mockEC2Client{describeOutput: stoppedInstanceWithID("i-test")}
	lambdaMock := &mockLambdaClient{invokeErr: fmt.Errorf("AccessDenied: user not authorized to invoke function")}

	resp := handleInteraction(context.Background(), interactionWith("mc", "start", "admin", "g1"), mock, ssmClient, nil, lambdaMock, "bonfire_bot")

	ir := parseInteractionResponse(t, resp)
	if ir.Type != discordChannelMessage {
		t.Errorf("expected type 4 channel message on self-invoke failure, got %d", ir.Type)
	}
	if ir.Data == nil || ir.Data.Flags != discordEphemeralFlag {
		t.Errorf("expected ephemeral flag 64 on Alert, got flags: %v", ir.Data)
	}
	embed := firstEmbed(t, ir)
	if !strings.Contains(embedBody(embed), copyAlertSomethingSideways) {
		t.Errorf("expected 'something went sideways' headline, got: %s", embedBody(embed))
	}
	// Hint footer should name the lambda_invoke failure kind.
	hint := ""
	if embed.Footer != nil {
		hint = embed.Footer.Text
	}
	if !strings.Contains(hint, "lambda_invoke") {
		t.Errorf("expected hint containing 'lambda_invoke', got: %q", hint)
	}
}

// --- Round 1 fix-pass tests ---

func TestAlertEmbed_KindColoring(t *testing.T) {
	cases := []struct {
		name       string
		build      func() Embed
		wantColor  int
		wantSymbol string
	}{
		{"error", func() Embed { return alertEmbed("h", "b", "err · x") }, colorDanger, alertSymbolError},
		{"unauthorized", func() Embed { return alertEmbedUnauthorized("h", "b", "err · x") }, colorAsh, alertSymbolUnauthorized},
		{"not_found", func() Embed { return alertEmbedNotFound("h", "b", "err · x") }, colorAsh, alertSymbolNotFound},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.build()
			if got.Color != tc.wantColor {
				t.Errorf("color: want %x, got %x", tc.wantColor, got.Color)
			}
			if !strings.HasPrefix(got.Title, tc.wantSymbol) {
				t.Errorf("title: expected symbol %q prefix, got %q", tc.wantSymbol, got.Title)
			}
		})
	}
}

func TestHandleSelfPoll_MissingField_Returns400(t *testing.T) {
	cases := []struct {
		name    string
		payload string
	}{
		{"missing game", `{"source":"self-poll","interaction_token":"t","application_id":"a","action":"start"}`},
		{"missing token", `{"source":"self-poll","game":"mc","application_id":"a","action":"start"}`},
		{"missing app_id", `{"source":"self-poll","game":"mc","interaction_token":"t","action":"start"}`},
		{"missing action", `{"source":"self-poll","game":"mc","interaction_token":"t","application_id":"a"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := handleSelfPoll(context.Background(), json.RawMessage(tc.payload))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if resp.StatusCode != 400 {
				t.Errorf("expected 400, got %d — body: %s", resp.StatusCode, resp.Body)
			}
			if !strings.Contains(resp.Body, "missing required field") {
				t.Errorf("expected 'missing required field' in body, got: %s", resp.Body)
			}
		})
	}
}

func TestPollTerminals_IncludeNotFoundAndMultiple(t *testing.T) {
	for _, state := range []string{"not_found", "multiple"} {
		if _, ok := startTerminals[state]; !ok {
			t.Errorf("startTerminals should include %q", state)
		}
		if _, ok := stopTerminals[state]; !ok {
			t.Errorf("stopTerminals should include %q", state)
		}
	}
}

func TestBuildStartTerminal_Deadline_UsesLightingLabelAndIceColor(t *testing.T) {
	cfg := pollConfig{Game: "valheim", Action: "start"}
	got := buildStartTerminal(context.Background(), cfg, instanceInfo{}, true)
	// Title should have "lighting" label (from "still lighting" body, UX I5).
	if !strings.Contains(got.Title, labelPending) {
		t.Errorf("deadline Hero title should contain %q label, got %q", labelPending, got.Title)
	}
	if got.Color != colorIce {
		t.Errorf("deadline Hero color should be ice %x, got %x", colorIce, got.Color)
	}
	if !strings.Contains(got.Description, "still lighting") {
		t.Errorf("expected 'still lighting' body, got: %s", got.Description)
	}
}

func TestBuildStartTerminal_NotFound_AlertEmbed(t *testing.T) {
	cfg := pollConfig{Game: "valheim", Action: "start"}
	got := buildStartTerminal(context.Background(), cfg, instanceInfo{State: "not_found"}, false)
	if !strings.Contains(got.Title, copyAlertNoSuchFire) {
		t.Errorf("expected 'no such fire' alert for not_found terminal, got: %s", got.Title)
	}
}

func TestHandleStartCommand_AlreadyRunning_IncludesBackup(t *testing.T) {
	// UX M2 — the `/start while running` idempotency Line should include the
	// backup trailer when s3 returns a recent backup.
	ssmClient := ssmWithGuildAndUsers("g1", "mc", "admin")
	mock := &mockEC2Client{describeOutput: runningInstanceWithID("i-test", "1.2.3.4")}
	s3Mock := &mockS3Client{output: s3OutputWithLastModified(time.Now().Add(-15 * time.Minute))}
	resp := handleInteraction(context.Background(), interactionWith("mc", "start", "admin", "g1"), mock, ssmClient, s3Mock, &mockLambdaClient{}, "bonfire_bot")
	ir := parseInteractionResponse(t, resp)
	embed := firstEmbed(t, ir)
	if !strings.Contains(embedBody(embed), "backup") {
		t.Errorf("expected backup trailer in /start-while-running Line, got: %s", embedBody(embed))
	}
}

func TestDispatchSelfPoll_Non202Status_ReturnsError(t *testing.T) {
	mock := &mockLambdaClient{invokeStatus: 500}
	err := dispatchSelfPoll(context.Background(), mock, "bonfire_bot", selfPollEvent{Source: selfPollSource, Game: "mc"})
	if err == nil {
		t.Fatal("expected error for non-202 status")
	}
	if !strings.Contains(err.Error(), "202") {
		t.Errorf("expected error mentioning 202, got: %v", err)
	}
}
