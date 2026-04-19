package main

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	lambdaapi "github.com/aws/aws-sdk-go-v2/service/lambda"
	lambdatypes "github.com/aws/aws-sdk-go-v2/service/lambda/types"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
)

// Discord interaction type constants
const (
	discordPing                   = 1
	discordSlashCommand           = 2
	discordChannelMessage         = 4
	discordDeferredChannelMessage = 5
	discordEphemeralFlag          = 64
)

// EC2API is the interface for EC2 operations used by this bot.
// Using an interface allows tests to inject a mock without AWS credentials.
type EC2API interface {
	DescribeInstances(ctx context.Context, params *ec2.DescribeInstancesInput, optFns ...func(*ec2.Options)) (*ec2.DescribeInstancesOutput, error)
	StartInstances(ctx context.Context, params *ec2.StartInstancesInput, optFns ...func(*ec2.Options)) (*ec2.StartInstancesOutput, error)
	StopInstances(ctx context.Context, params *ec2.StopInstancesInput, optFns ...func(*ec2.Options)) (*ec2.StopInstancesOutput, error)
}

// SSMAPI is the interface for SSM operations used by this bot.
type SSMAPI interface {
	GetParameter(ctx context.Context, params *ssm.GetParameterInput, optFns ...func(*ssm.Options)) (*ssm.GetParameterOutput, error)
}

// LambdaAPI is the narrow interface the bot needs from Lambda — only self-invoke.
// Injectable for tests; in prod the concrete *lambdaapi.Client satisfies it.
type LambdaAPI interface {
	Invoke(ctx context.Context, params *lambdaapi.InvokeInput, optFns ...func(*lambdaapi.Options)) (*lambdaapi.InvokeOutput, error)
}

// selfPollEvent is the payload shape for async self-invoke that kicks off the
// polling phase of a /start or /stop. The top-level "source" field is a
// distinctive marker that API Gateway Discord interaction payloads cannot
// produce; the handler entry uses it to branch between the ack and poll paths.
//
// See plan Amendment 2 § "Architecture".
type selfPollEvent struct {
	Source           string       `json:"source"` // always "self-poll"
	InteractionToken string       `json:"interaction_token"`
	ApplicationID    string       `json:"application_id"`
	Game             string       `json:"game"`
	User             selfPollUser `json:"user"`
	Action           string       `json:"action"` // "start" | "stop"
	InstanceID       string       `json:"instance_id"`
	EnqueuedAt       string       `json:"enqueued_at"` // RFC3339Nano, for queue-latency observability
}

type selfPollUser struct {
	ID       string `json:"id"`
	Username string `json:"username,omitempty"`
}

const selfPollSource = "self-poll"

// Lambda proxy request/response types
type LambdaRequest struct {
	Headers map[string]string `json:"headers"`
	Body    string            `json:"body"`
}

type LambdaResponse struct {
	StatusCode int               `json:"statusCode"`
	Headers    map[string]string `json:"headers"`
	Body       string            `json:"body"`
}

// Discord interaction types
type Interaction struct {
	Type    int             `json:"type"`
	GuildID string          `json:"guild_id"`
	Token   string          `json:"token"`
	Data    InteractionData `json:"data"`
	Member  *Member         `json:"member"`
	User    *User           `json:"user"`
}

type Member struct {
	User *User `json:"user"`
}

type InteractionData struct {
	Name    string       `json:"name"`
	Options []SubCommand `json:"options"`
}

type SubCommand struct {
	Name string `json:"name"`
}

type User struct {
	ID string `json:"id"`
}

type InteractionResponse struct {
	Type int                      `json:"type"`
	Data *InteractionResponseData `json:"data,omitempty"`
}

type InteractionResponseData struct {
	Content string  `json:"content,omitempty"`
	Flags   int     `json:"flags,omitempty"`
	Embeds  []Embed `json:"embeds,omitempty"`
}

func jsonResponse(statusCode int, body interface{}) LambdaResponse {
	b, err := json.Marshal(body)
	if err != nil {
		log.Printf("[shared] jsonResponse: marshal error: %v", err)
		return LambdaResponse{
			StatusCode: 500,
			Headers:    map[string]string{"Content-Type": "application/json"},
			Body:       `{"error":"internal server error"}`,
		}
	}
	return LambdaResponse{
		StatusCode: statusCode,
		Headers:    map[string]string{"Content-Type": "application/json"},
		Body:       string(b),
	}
}

func verifyDiscordRequest(signature, timestamp, body string) bool {
	pubKeyHex := os.Getenv("DISCORD_PUBLIC_KEY")
	pubKeyBytes, err := hex.DecodeString(pubKeyHex)
	if err != nil {
		log.Printf("[ack] verifyDiscordRequest: decode public key: %v", err)
		return false
	}
	sigBytes, err := hex.DecodeString(signature)
	if err != nil {
		log.Printf("[ack] verifyDiscordRequest: decode signature: %v", err)
		return false
	}
	message := []byte(timestamp + body)
	return ed25519.Verify(ed25519.PublicKey(pubKeyBytes), message, sigBytes)
}

// Lambda client pooling via sync.Once
//
// AWS Lambda reuses the same process (and its global variables) across multiple
// invocations when the execution environment is "warm". Creating an SDK client
// on every invocation wastes time and network connections; creating it once and
// reusing it is idiomatic Lambda Go.
//
// sync.Once guarantees the initialisation function runs exactly once, even if
// multiple goroutines call getEC2Client/getSSMClient concurrently on the first
// warm-up. The client and any initialisation error are stored in the paired
// package-level variables (sharedEC2Client/ec2ClientErr) so subsequent calls
// return immediately without re-initialising.
//
// The same pattern is used for the S3 client in polling.go (getS3Client).
var (
	ec2ClientOnce      sync.Once
	ssmClientOnce      sync.Once
	lambdaClientOnce   sync.Once
	sharedEC2Client    *ec2.Client
	sharedSSMClient    *ssm.Client
	sharedLambdaClient *lambdaapi.Client
	ec2ClientErr       error
	ssmClientErr       error
	lambdaClientErr    error
)

func newEC2Client(ctx context.Context) (*ec2.Client, error) {
	region := os.Getenv("AWS_REGION")
	if region == "" {
		region = "eu-north-1"
	}
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(region))
	if err != nil {
		return nil, err
	}
	return ec2.NewFromConfig(cfg), nil
}

func newSSMClient(ctx context.Context) (*ssm.Client, error) {
	region := os.Getenv("AWS_REGION")
	if region == "" {
		region = "eu-north-1"
	}
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(region))
	if err != nil {
		return nil, err
	}
	return ssm.NewFromConfig(cfg), nil
}

// getEC2Client returns the shared EC2 client, initialising it on the first call
// (sync.Once ensures a single initialisation even under concurrent invocations).
func getEC2Client(ctx context.Context) (*ec2.Client, error) {
	ec2ClientOnce.Do(func() {
		sharedEC2Client, ec2ClientErr = newEC2Client(ctx)
	})
	return sharedEC2Client, ec2ClientErr
}

// getSSMClient returns the shared SSM client, initialising it on the first call
// (sync.Once ensures a single initialisation even under concurrent invocations).
func getSSMClient(ctx context.Context) (*ssm.Client, error) {
	ssmClientOnce.Do(func() {
		sharedSSMClient, ssmClientErr = newSSMClient(ctx)
	})
	return sharedSSMClient, ssmClientErr
}

func newLambdaClient(ctx context.Context) (*lambdaapi.Client, error) {
	region := os.Getenv("AWS_REGION")
	if region == "" {
		region = "eu-north-1"
	}
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(region))
	if err != nil {
		return nil, err
	}
	return lambdaapi.NewFromConfig(cfg), nil
}

// getLambdaClient returns the shared Lambda client (used by the ack path to
// self-invoke the poll flow). Same sync.Once pattern as EC2/SSM.
func getLambdaClient(ctx context.Context) (*lambdaapi.Client, error) {
	lambdaClientOnce.Do(func() {
		sharedLambdaClient, lambdaClientErr = newLambdaClient(ctx)
	})
	return sharedLambdaClient, lambdaClientErr
}

// getSSMParam reads an SSM parameter; returns "" if the parameter does not exist.
func getSSMParam(ctx context.Context, client SSMAPI, name string) (string, error) {
	out, err := client.GetParameter(ctx, &ssm.GetParameterInput{
		Name: aws.String(name),
	})
	if err != nil {
		// Treat ParameterNotFound as an absent value, not an error
		if strings.Contains(err.Error(), "ParameterNotFound") {
			return "", nil
		}
		return "", err
	}
	if out.Parameter == nil || out.Parameter.Value == nil {
		return "", nil
	}
	return *out.Parameter.Value, nil
}

// checkGuildAllowlist checks whether guildID is in the /bonfire/allowed_guilds SSM list.
// An empty list or absent parameter fails closed (returns false).
func checkGuildAllowlist(ctx context.Context, guildID string, ssmClient SSMAPI) bool {
	raw, err := getSSMParam(ctx, ssmClient, "/bonfire/allowed_guilds")
	if err != nil || raw == "" {
		return false
	}
	for _, id := range strings.Split(raw, ",") {
		if strings.TrimSpace(id) == guildID {
			return true
		}
	}
	return false
}

// isAuthorizedSSM checks whether userID is in the /bonfire/<game>/authorized_users SSM list.
// An empty or absent parameter denies all (fail closed), matching prior AUTHORIZED_USERS behavior.
func isAuthorizedSSM(ctx context.Context, userID, game string, ssmClient SSMAPI) bool {
	raw, err := getSSMParam(ctx, ssmClient, "/bonfire/"+game+"/authorized_users")
	if err != nil || raw == "" {
		return false
	}
	for _, uid := range strings.Split(raw, ",") {
		if strings.TrimSpace(uid) == userID {
			return true
		}
	}
	return false
}

type instanceInfo struct {
	InstanceID   string
	State        string
	PublicIP     string
	InstanceType string
	LaunchTime   *time.Time
}

// findInstanceByGame discovers the EC2 instance for a game via tag:Game and tag:Project=bonfire.
// Returns State "not_found" if none match, "multiple" if more than one matches.
// Terminated instances are excluded.
func findInstanceByGame(ctx context.Context, client EC2API, game string) (instanceInfo, error) {
	out, err := client.DescribeInstances(ctx, &ec2.DescribeInstancesInput{
		Filters: []ec2types.Filter{
			{Name: aws.String("tag:Game"), Values: []string{game}},
			{Name: aws.String("tag:Project"), Values: []string{"bonfire"}},
			{Name: aws.String("instance-state-name"), Values: []string{"pending", "running", "stopping", "stopped"}},
		},
	})
	if err != nil {
		return instanceInfo{}, err
	}

	var instances []ec2types.Instance
	for _, r := range out.Reservations {
		instances = append(instances, r.Instances...)
	}

	if len(instances) == 0 {
		return instanceInfo{State: "not_found"}, nil
	}
	if len(instances) > 1 {
		return instanceInfo{State: "multiple"}, nil
	}

	inst := instances[0]
	info := instanceInfo{
		State:        string(inst.State.Name),
		InstanceType: string(inst.InstanceType),
		LaunchTime:   inst.LaunchTime,
	}
	if inst.InstanceId != nil {
		info.InstanceID = *inst.InstanceId
	}
	if inst.PublicIpAddress != nil {
		info.PublicIP = *inst.PublicIpAddress
	} else {
		info.PublicIP = "N/A"
	}
	return info, nil
}

func startInstance(ctx context.Context, client EC2API, instanceID string) error {
	_, err := client.StartInstances(ctx, &ec2.StartInstancesInput{
		InstanceIds: []string{instanceID},
	})
	return err
}

func stopInstance(ctx context.Context, client EC2API, instanceID string) error {
	_, err := client.StopInstances(ctx, &ec2.StopInstancesInput{
		InstanceIds: []string{instanceID},
	})
	return err
}

// ephemeralResponse returns a type 4 ephemeral response with plain-text content.
// Used by /help — the brand ships it as a mono text block, not an embed
// (BRAND.md §"Help").
func ephemeralResponse(content string) InteractionResponse {
	return InteractionResponse{
		Type: discordChannelMessage,
		Data: &InteractionResponseData{Content: content, Flags: discordEphemeralFlag},
	}
}

// awsRegion returns the region the Lambda runs in, with the default used elsewhere.
func awsRegion() string {
	if r := os.Getenv("AWS_REGION"); r != "" {
		return r
	}
	return "eu-north-1"
}

// discordAppID returns the Discord application ID from the environment.
// Used by the polling loop's PATCH URL; empty in tests.
func discordAppID() string {
	return os.Getenv("DISCORD_APP_ID")
}

// selfFunctionName returns the Lambda function name to self-invoke. Lambda
// auto-populates AWS_LAMBDA_FUNCTION_NAME in the runtime env.
func selfFunctionName() string {
	return os.Getenv("AWS_LAMBDA_FUNCTION_NAME")
}

// dispatchSelfPoll marshals and async-invokes the bot Lambda with a self-poll
// event. Returns the invoke error (nil on success). The ack handler uses this
// to hand off the polling phase before returning the Discord type-5 response.
//
// Per AWS docs (Lambda Invoke action reference), `InvocationType: Event`
// returns **exactly** 202 on successful async queue acceptance — any other
// status indicates the queue refused the event, and FunctionError on a 2xx
// means the function was invoked synchronously by mistake. Both are treated
// as dispatch failures.
func dispatchSelfPoll(ctx context.Context, client LambdaAPI, functionName string, event selfPollEvent) error {
	payload, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal self-poll event: %w", err)
	}
	out, err := client.Invoke(ctx, &lambdaapi.InvokeInput{
		FunctionName:   aws.String(functionName),
		InvocationType: lambdatypes.InvocationTypeEvent,
		Payload:        payload,
	})
	if err != nil {
		return fmt.Errorf("lambda invoke: %w", err)
	}
	if out.StatusCode != 202 {
		return fmt.Errorf("lambda invoke expected 202, got %d", out.StatusCode)
	}
	if out.FunctionError != nil && *out.FunctionError != "" {
		return fmt.Errorf("lambda invoke returned FunctionError: %s", *out.FunctionError)
	}
	return nil
}

// --- handler paths ---

func handleStatusCommand(ctx context.Context, client EC2API, s3Client S3API, gameName string) InteractionResponse {
	info, err := findInstanceByGame(ctx, client, gameName)
	if err != nil {
		log.Printf("[ack] handleStatusCommand: EC2 error for game %q: %v", gameName, err)
		return ephemeralEmbedResponse(alertEmbed(
			copyAlertSomethingSideways,
			"i couldn't check on the fire just now.",
			fmt.Sprintf(copyHintEC2ErrorFmt, "describe_instances"),
		))
	}
	if info.State == "not_found" {
		return ephemeralEmbedResponse(alertEmbedNotFound(
			copyAlertNoSuchFire,
			"no fire here by that name",
			fmt.Sprintf(copyHintTryStatusFmt, gameName),
		))
	}
	if info.State == "multiple" {
		return ephemeralEmbedResponse(alertEmbed(
			copyAlertTwoFires,
			"i found more than one — that shouldn't happen.",
			copyHintTagCollision,
		))
	}

	switch info.State {
	case "running":
		uptime := formatElapsed(elapsedSince(info.LaunchTime))
		backup := ""
		if s3Client != nil {
			backup = backupElapsedString(ctx, s3Client, gameName, awsRegion(), "[ack] ")
		}
		var body string
		if backup == "" {
			body = fmt.Sprintf(copyStatusRunningNoBackup, gameName, info.PublicIP, uptime)
		} else {
			body = fmt.Sprintf(copyStatusRunning, gameName, info.PublicIP, uptime, backup)
		}
		return ephemeralEmbedResponse(lineEmbed("running", body))
	case "pending":
		elapsed := formatElapsed(elapsedSince(info.LaunchTime))
		return ephemeralEmbedResponse(lineEmbed("pending", fmt.Sprintf(copyStatusPending, gameName, elapsed)))
	case "stopping":
		return ephemeralEmbedResponse(lineEmbed("stopping", fmt.Sprintf(copyStatusStopping, gameName)))
	case "stopped":
		backupStr := ""
		if s3Client != nil {
			backupStr = backupElapsedString(ctx, s3Client, gameName, awsRegion(), "[ack] ")
		}
		if backupStr == "" {
			return ephemeralEmbedResponse(lineEmbed("stopped", fmt.Sprintf(copyStatusStoppedNever, gameName)))
		}
		return ephemeralEmbedResponse(lineEmbed("stopped", fmt.Sprintf(copyStatusStopped, gameName, backupStr)))
	}
	// Unknown state — treat as stopped-ish.
	return ephemeralEmbedResponse(lineEmbed("", fmt.Sprintf("%s · unknown state", gameName)))
}

// handleStartCommand — synchronous state check + idempotency; on the transition
// path (state=stopped) it calls StartInstances, dispatches a self-poll invoke,
// and returns the Discord deferred response. Polling runs in a separate Lambda
// invocation (see handleSelfPoll) — this handler never blocks.
func handleStartCommand(
	ctx context.Context,
	ec2Client EC2API,
	ssmClient SSMAPI,
	s3Client S3API,
	lambdaClient LambdaAPI,
	functionName, userID, gameName, interactionToken string,
) InteractionResponse {
	if !isAuthorizedSSM(ctx, userID, gameName, ssmClient) {
		return ephemeralEmbedResponse(alertEmbedUnauthorized(
			copyAlertUnauthorizedHeadline,
			copyAlertUnauthorizedBody,
			copyHintRoleMissing,
		))
	}

	info, err := findInstanceByGame(ctx, ec2Client, gameName)
	if err != nil {
		log.Printf("[ack] handleStartCommand: EC2 error finding instance for game %q: %v", gameName, err)
		return ephemeralEmbedResponse(alertEmbed(
			copyAlertSomethingSideways,
			"i couldn't light the fire just now.",
			fmt.Sprintf(copyHintEC2ErrorFmt, "describe_instances"),
		))
	}
	if info.State == "not_found" {
		return ephemeralEmbedResponse(alertEmbedNotFound(
			copyAlertNoSuchFire,
			"no fire here by that name",
			fmt.Sprintf(copyHintTryStatusFmt, gameName),
		))
	}
	if info.State == "multiple" {
		return ephemeralEmbedResponse(alertEmbed(
			copyAlertTwoFires,
			"i found more than one — that shouldn't happen.",
			copyHintTagCollision,
		))
	}

	// Idempotency: state ∈ {running, pending, stopping} → ephemeral Line, no start, no self-invoke.
	switch info.State {
	case "running":
		elapsed := formatElapsed(elapsedSince(info.LaunchTime))
		// Include backup trailer when we have one — the running Line format is
		// `... lit X ago · addr · backup Y ago` per BRAND.md §"Line".
		backup := ""
		if s3Client != nil {
			backup = backupElapsedString(ctx, s3Client, gameName, awsRegion(), "[ack] ")
		}
		if backup != "" {
			return ephemeralEmbedResponse(lineEmbed("running", fmt.Sprintf(copyStartAlreadyRunningWithBackup, elapsed, info.PublicIP, backup)))
		}
		return ephemeralEmbedResponse(lineEmbed("running", fmt.Sprintf(copyStartAlreadyRunning, elapsed, info.PublicIP)))
	case "pending":
		elapsed := formatElapsed(elapsedSince(info.LaunchTime))
		return ephemeralEmbedResponse(lineEmbed("pending", fmt.Sprintf(copyStartAlreadyLighting, elapsed)))
	case "stopping":
		return ephemeralEmbedResponse(lineEmbed("stopping", copyStartWhileStopping))
	}

	// state == "stopped": call StartInstances, self-invoke poll, return deferred ACK.
	if err := startInstance(ctx, ec2Client, info.InstanceID); err != nil {
		log.Printf("[ack] handleStartCommand: EC2 error starting instance %q for game %q: %v", info.InstanceID, gameName, err)
		return ephemeralEmbedResponse(alertEmbed(
			copyAlertSomethingSideways,
			"i couldn't light the fire just now.",
			fmt.Sprintf(copyHintEC2ErrorFmt, "start_instances"),
		))
	}

	event := selfPollEvent{
		Source:           selfPollSource,
		InteractionToken: interactionToken,
		ApplicationID:    discordAppID(),
		Game:             gameName,
		User:             selfPollUser{ID: userID},
		Action:           "start",
		InstanceID:       info.InstanceID,
		EnqueuedAt:       time.Now().UTC().Format(time.RFC3339Nano),
	}
	if err := dispatchSelfPoll(ctx, lambdaClient, functionName, event); err != nil {
		log.Printf("[ack] handleStartCommand: self-invoke failed for game %q: %v", gameName, err)
		return ephemeralEmbedResponse(alertEmbed(
			copyAlertSomethingSideways,
			"i lit the fire but couldn't queue the tending.",
			fmt.Sprintf(copyHintLambdaErrorFmt, "invoke_failed"),
		))
	}
	return deferredResponse()
}

// handleStopCommand mirrors handleStartCommand for /stop.
func handleStopCommand(
	ctx context.Context,
	ec2Client EC2API,
	ssmClient SSMAPI,
	s3Client S3API,
	lambdaClient LambdaAPI,
	functionName, userID, gameName, interactionToken string,
) InteractionResponse {
	if !isAuthorizedSSM(ctx, userID, gameName, ssmClient) {
		return ephemeralEmbedResponse(alertEmbedUnauthorized(
			copyAlertUnauthorizedHeadline,
			copyAlertUnauthorizedBody,
			copyHintRoleMissing,
		))
	}

	info, err := findInstanceByGame(ctx, ec2Client, gameName)
	if err != nil {
		log.Printf("[ack] handleStopCommand: EC2 error finding instance for game %q: %v", gameName, err)
		return ephemeralEmbedResponse(alertEmbed(
			copyAlertSomethingSideways,
			"i couldn't bank the coals just now.",
			fmt.Sprintf(copyHintEC2ErrorFmt, "describe_instances"),
		))
	}
	if info.State == "not_found" {
		return ephemeralEmbedResponse(alertEmbedNotFound(
			copyAlertNoSuchFire,
			"no fire here by that name",
			fmt.Sprintf(copyHintTryStatusFmt, gameName),
		))
	}
	if info.State == "multiple" {
		return ephemeralEmbedResponse(alertEmbed(
			copyAlertTwoFires,
			"i found more than one — that shouldn't happen.",
			copyHintTagCollision,
		))
	}

	// Idempotency.
	switch info.State {
	case "stopped":
		backup := ""
		if s3Client != nil {
			backup = backupElapsedString(ctx, s3Client, gameName, awsRegion(), "[ack] ")
		}
		if backup == "" {
			return ephemeralEmbedResponse(lineEmbed("stopped", copyStopAlreadyOutNever))
		}
		return ephemeralEmbedResponse(lineEmbed("stopped", fmt.Sprintf(copyStopAlreadyOut, backup)))
	case "stopping":
		return ephemeralEmbedResponse(lineEmbed("stopping", copyStopAlreadyDyingDown))
	case "pending":
		return ephemeralEmbedResponse(lineEmbed("pending", copyStopWhilePending))
	}

	// state == "running": call StopInstances, self-invoke poll, return deferred ACK.
	if err := stopInstance(ctx, ec2Client, info.InstanceID); err != nil {
		log.Printf("[ack] handleStopCommand: EC2 error stopping instance %q for game %q: %v", info.InstanceID, gameName, err)
		return ephemeralEmbedResponse(alertEmbed(
			copyAlertSomethingSideways,
			"i couldn't bank the coals just now.",
			fmt.Sprintf(copyHintEC2ErrorFmt, "stop_instances"),
		))
	}

	event := selfPollEvent{
		Source:           selfPollSource,
		InteractionToken: interactionToken,
		ApplicationID:    discordAppID(),
		Game:             gameName,
		User:             selfPollUser{ID: userID},
		Action:           "stop",
		InstanceID:       info.InstanceID,
		EnqueuedAt:       time.Now().UTC().Format(time.RFC3339Nano),
	}
	if err := dispatchSelfPoll(ctx, lambdaClient, functionName, event); err != nil {
		log.Printf("[ack] handleStopCommand: self-invoke failed for game %q: %v", gameName, err)
		return ephemeralEmbedResponse(alertEmbed(
			copyAlertSomethingSideways,
			"i banked the coals but couldn't queue the tending.",
			fmt.Sprintf(copyHintLambdaErrorFmt, "invoke_failed"),
		))
	}
	return deferredResponse()
}

// handleHelpCommand returns the plain-text Help surface per BRAND.md §"Help":
// no embed chrome, mono text block, ephemeral. This is the one command where
// Content (not an embed) is the correct shape — the brand asks for it explicitly.
func handleHelpCommand(gameName string) InteractionResponse {
	if gameName == "" {
		return ephemeralEmbedResponse(alertEmbedNotFound(
			copyAlertUnknownCommand,
			"ask a keeper for the right command",
			"try · /<game> help",
		))
	}
	return ephemeralResponse(fmt.Sprintf(copyHelpBlock, gameName, gameName, gameName, gameName))
}

func handleHelloCommand(ctx context.Context, ssmClient SSMAPI, userID, gameName string) InteractionResponse {
	var msg string
	if isAuthorizedSSM(ctx, userID, gameName, ssmClient) {
		msg = copyHelloKeeper
	} else {
		msg = copyHelloVisitor
	}
	return ephemeralEmbedResponse(lineEmbed("", msg))
}

// handleInteraction dispatches a single Discord slash command. Returns the
// LambdaResponse that will be written back to Discord via API Gateway.
//
// The /start and /stop transition paths dispatch an async self-invoke
// internally (see handleStartCommand / handleStopCommand) so this handler
// never blocks on background work.
func handleInteraction(
	ctx context.Context,
	interaction Interaction,
	ec2Client EC2API,
	ssmClient SSMAPI,
	s3Client S3API,
	lambdaClient LambdaAPI,
	functionName string,
) LambdaResponse {
	if interaction.Type != discordSlashCommand {
		return jsonResponse(400, map[string]string{"error": "Not a slash command"})
	}

	// Guild allowlist check — reject requests from non-allowlisted guilds (fail closed).
	// Framed as gentle refusal, not error: the guild owner just hasn't registered yet.
	if !checkGuildAllowlist(ctx, interaction.GuildID, ssmClient) {
		return jsonResponse(200, ephemeralEmbedResponse(alertEmbedNotFound(
			copyAlertGuildBlocked,
			"this guild isn't on the allowlist",
			"err · guild_not_allowed",
		)))
	}

	userID := ""
	if interaction.Member != nil && interaction.Member.User != nil {
		userID = interaction.Member.User.ID
	} else if interaction.User != nil {
		userID = interaction.User.ID
	}

	gameName := interaction.Data.Name

	if len(interaction.Data.Options) == 0 {
		return jsonResponse(200, ephemeralEmbedResponse(alertEmbedNotFound(
			copyAlertUnknownAction,
			"try one of: status, start, stop, help, hello",
			fmt.Sprintf(copyHintTryStatusFmt, gameName),
		)))
	}

	action := interaction.Data.Options[0].Name

	var interactionResp InteractionResponse
	switch action {
	case "status":
		interactionResp = handleStatusCommand(ctx, ec2Client, s3Client, gameName)
	case "start":
		interactionResp = handleStartCommand(ctx, ec2Client, ssmClient, s3Client, lambdaClient, functionName, userID, gameName, interaction.Token)
	case "stop":
		interactionResp = handleStopCommand(ctx, ec2Client, ssmClient, s3Client, lambdaClient, functionName, userID, gameName, interaction.Token)
	case "help":
		interactionResp = handleHelpCommand(gameName)
	case "hello":
		interactionResp = handleHelloCommand(ctx, ssmClient, userID, gameName)
	default:
		interactionResp = ephemeralEmbedResponse(alertEmbedNotFound(
			copyAlertUnknownCommand,
			"ask a keeper for the right command",
			fmt.Sprintf(copyHintTryStatusFmt, gameName),
		))
	}

	return jsonResponse(200, interactionResp)
}

// handler is the Lambda entry point. It handles two event shapes:
//
//  1. API Gateway HTTP v2 request (a Discord interaction) — the "[ack]" path.
//     Verifies signature, dispatches the slash command. /start and /stop
//     transition paths asynchronously self-invoke this same Lambda with a
//     self-poll event, then return the type-5 deferred ACK immediately.
//
//  2. Self-poll event — the "[poll]" path. Runs the poll loop which PATCHes
//     the original Discord message as EC2 transitions, using the token from
//     the event. INVARIANT: self-poll branch never dispatches self-invoke.
//
// A raw JSON peek on top-level "source" routes between the two.
//
// Trust boundary (Security Analyst review): the self-poll branch skips the
// Ed25519 signature check because self-poll events originate from the Lambda
// itself via lambda:InvokeFunction (IAM-scoped to this function's own ARN).
// API Gateway HTTP v2 (AWS_PROXY, payload v1.0) wraps an attacker's body in
// {"headers":{...},"body":"...",...} — a top-level "source" field from the
// attacker's JSON sits inside `body` as a string and cannot reach the peek
// struct. We *also* explicitly reject self-poll events carrying API Gateway
// wrapping fields (headers/requestContext) as defense-in-depth against any
// future payload-format change or operator mis-routing.
func handler(ctx context.Context, raw json.RawMessage) (LambdaResponse, error) {
	var peek struct {
		Source         string            `json:"source"`
		Headers        map[string]string `json:"headers,omitempty"`
		RequestContext json.RawMessage   `json:"requestContext,omitempty"`
	}
	if err := json.Unmarshal(raw, &peek); err == nil && peek.Source == selfPollSource {
		// Defense-in-depth: a legitimate self-poll event (produced by
		// dispatchSelfPoll) never contains API Gateway fields. Their presence
		// signals an attacker trying to trip the signature-verification bypass.
		if len(peek.Headers) > 0 || len(peek.RequestContext) > 0 {
			log.Printf("[shared] handler: rejecting self-poll event that carries API Gateway wrapping fields (possible signature-bypass attempt)")
			return jsonResponse(400, map[string]string{"error": "bad request"}), nil
		}
		return handleSelfPoll(ctx, raw)
	}

	// Default: API Gateway HTTP v2 request with a Discord interaction in the body.
	var req LambdaRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		log.Printf("[ack] handler: unmarshal LambdaRequest: %v", err)
		return jsonResponse(400, map[string]string{"error": "bad request"}), nil
	}
	return handleAckRequest(ctx, req)
}

// handleAckRequest is the fast, 3s-window path for Discord interactions.
// It verifies the signature, resolves shared clients, dispatches the command,
// and returns. Never blocks on background work — the /start and /stop
// transition paths enqueue a self-poll via Lambda async invoke before returning.
func handleAckRequest(ctx context.Context, req LambdaRequest) (LambdaResponse, error) {
	signature := req.Headers["x-signature-ed25519"]
	if signature == "" {
		signature = req.Headers["X-Signature-Ed25519"]
	}
	timestamp := req.Headers["x-signature-timestamp"]
	if timestamp == "" {
		timestamp = req.Headers["X-Signature-Timestamp"]
	}

	if signature == "" || timestamp == "" || req.Body == "" {
		return jsonResponse(401, map[string]string{"error": "Missing signature headers"}), nil
	}

	if !verifyDiscordRequest(signature, timestamp, req.Body) {
		return jsonResponse(401, map[string]string{"error": "Invalid signature"}), nil
	}

	var body struct {
		Type int `json:"type"`
	}
	if err := json.Unmarshal([]byte(req.Body), &body); err != nil {
		log.Printf("[ack] handleAckRequest: parse body: %v", err)
		return jsonResponse(500, map[string]string{"error": "Internal server error"}), nil
	}

	// PING — skip guild check, respond immediately
	if body.Type == discordPing {
		return jsonResponse(200, map[string]int{"type": discordPing}), nil
	}

	var interaction Interaction
	if err := json.Unmarshal([]byte(req.Body), &interaction); err != nil {
		log.Printf("[ack] handleAckRequest: parse interaction: %v", err)
		return jsonResponse(500, map[string]string{"error": "Internal server error"}), nil
	}

	ec2Client, err := getEC2Client(ctx)
	if err != nil {
		log.Printf("[ack] handleAckRequest: EC2 client init: %v", err)
		return jsonResponse(500, map[string]string{"error": "Internal server error"}), nil
	}
	ssmClient, err := getSSMClient(ctx)
	if err != nil {
		log.Printf("[ack] handleAckRequest: SSM client init: %v", err)
		return jsonResponse(500, map[string]string{"error": "Internal server error"}), nil
	}
	lambdaClient, err := getLambdaClient(ctx)
	if err != nil {
		log.Printf("[ack] handleAckRequest: Lambda client init: %v", err)
		return jsonResponse(500, map[string]string{"error": "Internal server error"}), nil
	}

	// S3 client — best-effort; BACKUP field is optional and we don't want a
	// transient S3 config failure to break /start or /stop.
	// Typed as the S3API interface (not *s3.Client) so we can pass a bare `nil`
	// on error — a typed-nil *s3.Client through the interface would create a
	// non-nil S3API with a nil concrete value, breaking `if s3Client != nil`.
	var s3Client S3API
	if c, err := getS3Client(ctx); err != nil {
		log.Printf("[ack] handleAckRequest: S3 client init (backup lookups will be skipped): %v", err)
	} else {
		s3Client = c
	}

	return handleInteraction(ctx, interaction, ec2Client, ssmClient, s3Client, lambdaClient, selfFunctionName()), nil
}

// handleSelfPoll runs the poll loop for a /start or /stop that the ack path
// kicked off via async self-invoke. It never dispatches another self-invoke.
//
// INVARIANT: self-poll branch never dispatches self-invoke. The ack→poll
// hand-off is one-way; any retry policy lives at the Lambda async-invoke
// config layer (maximum_retry_attempts = 0 in terraform/bot/), not in code.
func handleSelfPoll(ctx context.Context, raw json.RawMessage) (LambdaResponse, error) {
	var event selfPollEvent
	if err := json.Unmarshal(raw, &event); err != nil {
		log.Printf("[poll] handleSelfPoll: unmarshal event: %v", err)
		return jsonResponse(400, map[string]string{"error": "bad self-poll event"}), nil
	}

	// Field validation — a malformed self-poll event (missing game, token,
	// app_id, action) would otherwise slip into pollStartFlow with empty
	// values; the 404 from a malformed webhook URL masks the root cause and
	// costs up to 180s of Lambda wallclock. Validate up front and drop the
	// event with a 400 — from Lambda's view this is a successful async
	// invocation (error=nil), so the event is silently dropped rather than
	// routed to the DLQ. That's intentional: malformed self-poll payloads
	// are a code bug on the ack side; retrying them or preserving them in
	// SQS has no recovery path. CloudWatch log is the forensic trail.
	if event.Game == "" || event.InteractionToken == "" || event.ApplicationID == "" || event.Action == "" {
		log.Printf("[poll] handleSelfPoll: missing required field(s) in self-poll event: game=%q token_present=%t app_id=%q action=%q",
			event.Game, event.InteractionToken != "", event.ApplicationID, event.Action)
		return jsonResponse(400, map[string]string{"error": "missing required field"}), nil
	}

	// Queue-latency observability (Architect risk #4): record how long the event
	// sat in the Lambda async queue before this invocation picked it up. Drift
	// eats into the 15-min Discord interaction-token budget.
	enqueuedAt := time.Now()
	if t, err := time.Parse(time.RFC3339Nano, event.EnqueuedAt); err == nil {
		enqueuedAt = t
		log.Printf("[poll] handleSelfPoll: queue_latency_ms=%d game=%s action=%s", time.Since(t).Milliseconds(), event.Game, event.Action)
	} else {
		log.Printf("[poll] handleSelfPoll: parse enqueued_at %q failed: %v (continuing with now())", event.EnqueuedAt, err)
	}

	ec2Client, err := getEC2Client(ctx)
	if err != nil {
		log.Printf("[poll] handleSelfPoll: EC2 client init: %v", err)
		return jsonResponse(500, map[string]string{"error": "Internal server error"}), nil
	}
	var s3Client S3API
	if c, err := getS3Client(ctx); err != nil {
		log.Printf("[poll] handleSelfPoll: S3 client init (backup field will be omitted): %v", err)
	} else {
		s3Client = c
	}

	cfg := pollConfig{
		Game:       event.Game,
		Action:     event.Action,
		UserID:     event.User.ID,
		InstanceID: event.InstanceID,
		AppID:      event.ApplicationID,
		Token:      event.InteractionToken,
		Region:     awsRegion(),
		EC2Client:  ec2Client,
		S3Client:   s3Client,
	}

	switch event.Action {
	case "start":
		pollStartFlow(ctx, cfg, enqueuedAt)
	case "stop":
		pollStopFlow(ctx, cfg, enqueuedAt)
	default:
		log.Printf("[poll] handleSelfPoll: unknown action %q", event.Action)
		return jsonResponse(400, map[string]string{"error": "unknown action"}), nil
	}

	return jsonResponse(200, map[string]string{"status": "ok"}), nil
}

// Compile-time assertions that SDK clients satisfy the narrow interfaces.
var _ EC2API = (*ec2.Client)(nil)
var _ SSMAPI = (*ssm.Client)(nil)
var _ LambdaAPI = (*lambdaapi.Client)(nil)

func main() {
	lambda.Start(handler)
}
