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
		log.Printf("jsonResponse: marshal error: %v", err)
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
		log.Printf("Error decoding public key: %v", err)
		return false
	}
	sigBytes, err := hex.DecodeString(signature)
	if err != nil {
		log.Printf("Error decoding signature: %v", err)
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
	ec2ClientOnce   sync.Once
	ssmClientOnce   sync.Once
	sharedEC2Client *ec2.Client
	sharedSSMClient *ssm.Client
	ec2ClientErr    error
	ssmClientErr    error
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
// Preserved for backwards-compatibility with tests and a handful of simple paths;
// new code should prefer ephemeralEmbedResponse from embeds.go for brand-colored bars.
func ephemeralResponse(content string) InteractionResponse {
	return InteractionResponse{
		Type: discordChannelMessage,
		Data: &InteractionResponseData{Content: content, Flags: discordEphemeralFlag},
	}
}

// publicResponse returns a type 4 public response with plain-text content.
// Preserved for tests; new branded paths use publicEmbedResponse.
func publicResponse(content string) InteractionResponse {
	return InteractionResponse{
		Type: discordChannelMessage,
		Data: &InteractionResponseData{Content: content},
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

// --- handler paths ---

func handleStatusCommand(ctx context.Context, client EC2API, s3Client S3API, gameName string) InteractionResponse {
	info, err := findInstanceByGame(ctx, client, gameName)
	if err != nil {
		log.Printf("handleStatusCommand: EC2 error for game %q: %v", gameName, err)
		return ephemeralEmbedResponse(alertEmbed(
			copyAlertSomethingSideways,
			"i couldn't check on the fire just now.",
			fmt.Sprintf(copyHintEC2ErrorFmt, "describe_instances"),
		))
	}
	if info.State == "not_found" {
		return ephemeralEmbedResponse(alertEmbed(
			copyAlertNoSuchFire,
			"no fire here by that name.",
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
			backup = backupElapsedString(ctx, s3Client, gameName, awsRegion())
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
			backupStr = backupElapsedString(ctx, s3Client, gameName, awsRegion())
		}
		if backupStr == "" {
			return ephemeralEmbedResponse(lineEmbed("stopped", fmt.Sprintf(copyStatusStoppedNever, gameName)))
		}
		return ephemeralEmbedResponse(lineEmbed("stopped", fmt.Sprintf(copyStatusStopped, gameName, backupStr)))
	}
	// Unknown state — treat as stopped-ish.
	return ephemeralEmbedResponse(lineEmbed("", fmt.Sprintf("%s · unknown state", gameName)))
}

// handleStartCommand — synchronous state check + idempotency; returns a
// deferred response when the state is "stopped" so the poller can take over.
// If a poller should run, it is returned via pollRunner (non-nil) and the caller
// must invoke it after the HTTP response is written.
func handleStartCommand(
	ctx context.Context,
	ec2Client EC2API,
	ssmClient SSMAPI,
	s3Client S3API,
	userID, gameName, interactionToken string,
) (InteractionResponse, pollRunner) {
	if !isAuthorizedSSM(ctx, userID, gameName, ssmClient) {
		return ephemeralEmbedResponse(alertEmbed(
			copyAlertUnauthorizedHeadline,
			copyAlertUnauthorizedBody,
			copyHintRoleMissing,
		)), nil
	}

	info, err := findInstanceByGame(ctx, ec2Client, gameName)
	if err != nil {
		log.Printf("handleStartCommand: EC2 error finding instance for game %q: %v", gameName, err)
		return ephemeralEmbedResponse(alertEmbed(
			copyAlertSomethingSideways,
			"i couldn't light the fire just now.",
			fmt.Sprintf(copyHintEC2ErrorFmt, "describe_instances"),
		)), nil
	}
	if info.State == "not_found" {
		return ephemeralEmbedResponse(alertEmbed(
			copyAlertNoSuchFire,
			"no fire here by that name.",
			fmt.Sprintf(copyHintTryStatusFmt, gameName),
		)), nil
	}
	if info.State == "multiple" {
		return ephemeralEmbedResponse(alertEmbed(
			copyAlertTwoFires,
			"i found more than one — that shouldn't happen.",
			copyHintTagCollision,
		)), nil
	}

	// Idempotency: state ∈ {running, pending, stopping} → ephemeral Line, no start, no defer.
	switch info.State {
	case "running":
		elapsed := formatElapsed(elapsedSince(info.LaunchTime))
		return ephemeralEmbedResponse(lineEmbed("running", fmt.Sprintf(copyStartAlreadyRunning, elapsed, info.PublicIP))), nil
	case "pending":
		elapsed := formatElapsed(elapsedSince(info.LaunchTime))
		return ephemeralEmbedResponse(lineEmbed("pending", fmt.Sprintf(copyStartAlreadyLighting, elapsed))), nil
	case "stopping":
		return ephemeralEmbedResponse(lineEmbed("stopping", copyStartWhileStopping)), nil
	}

	// state == "stopped": call StartInstances, defer, spawn poller.
	if err := startInstance(ctx, ec2Client, info.InstanceID); err != nil {
		log.Printf("handleStartCommand: EC2 error starting instance %q for game %q: %v", info.InstanceID, gameName, err)
		return ephemeralEmbedResponse(alertEmbed(
			copyAlertSomethingSideways,
			"i couldn't light the fire just now.",
			fmt.Sprintf(copyHintEC2ErrorFmt, "start_instances"),
		)), nil
	}

	cfg := pollConfig{
		Game:       gameName,
		Action:     "start",
		UserID:     userID,
		InstanceID: info.InstanceID,
		AppID:      discordAppID(),
		Token:      interactionToken,
		Region:     awsRegion(),
		EC2Client:  ec2Client,
		S3Client:   s3Client,
	}
	runner := func(pollCtx context.Context) {
		pollStartFlow(pollCtx, cfg, time.Now())
	}
	return deferredResponse(), runner
}

// handleStopCommand mirrors handleStartCommand for /stop.
func handleStopCommand(
	ctx context.Context,
	ec2Client EC2API,
	ssmClient SSMAPI,
	s3Client S3API,
	userID, gameName, interactionToken string,
) (InteractionResponse, pollRunner) {
	if !isAuthorizedSSM(ctx, userID, gameName, ssmClient) {
		return ephemeralEmbedResponse(alertEmbed(
			copyAlertUnauthorizedHeadline,
			copyAlertUnauthorizedBody,
			copyHintRoleMissing,
		)), nil
	}

	info, err := findInstanceByGame(ctx, ec2Client, gameName)
	if err != nil {
		log.Printf("handleStopCommand: EC2 error finding instance for game %q: %v", gameName, err)
		return ephemeralEmbedResponse(alertEmbed(
			copyAlertSomethingSideways,
			"i couldn't bank the coals just now.",
			fmt.Sprintf(copyHintEC2ErrorFmt, "describe_instances"),
		)), nil
	}
	if info.State == "not_found" {
		return ephemeralEmbedResponse(alertEmbed(
			copyAlertNoSuchFire,
			"no fire here by that name.",
			fmt.Sprintf(copyHintTryStatusFmt, gameName),
		)), nil
	}
	if info.State == "multiple" {
		return ephemeralEmbedResponse(alertEmbed(
			copyAlertTwoFires,
			"i found more than one — that shouldn't happen.",
			copyHintTagCollision,
		)), nil
	}

	// Idempotency.
	switch info.State {
	case "stopped":
		backup := ""
		if s3Client != nil {
			backup = backupElapsedString(ctx, s3Client, gameName, awsRegion())
		}
		if backup == "" {
			return ephemeralEmbedResponse(lineEmbed("stopped", copyStopAlreadyOutNever)), nil
		}
		return ephemeralEmbedResponse(lineEmbed("stopped", fmt.Sprintf(copyStopAlreadyOut, backup))), nil
	case "stopping":
		return ephemeralEmbedResponse(lineEmbed("stopping", copyStopAlreadyDyingDown)), nil
	case "pending":
		return ephemeralEmbedResponse(lineEmbed("pending", copyStopWhilePending)), nil
	}

	// state == "running": call StopInstances, defer, spawn poller.
	if err := stopInstance(ctx, ec2Client, info.InstanceID); err != nil {
		log.Printf("handleStopCommand: EC2 error stopping instance %q for game %q: %v", info.InstanceID, gameName, err)
		return ephemeralEmbedResponse(alertEmbed(
			copyAlertSomethingSideways,
			"i couldn't bank the coals just now.",
			fmt.Sprintf(copyHintEC2ErrorFmt, "stop_instances"),
		)), nil
	}

	cfg := pollConfig{
		Game:       gameName,
		Action:     "stop",
		UserID:     userID,
		InstanceID: info.InstanceID,
		AppID:      discordAppID(),
		Token:      interactionToken,
		Region:     awsRegion(),
		EC2Client:  ec2Client,
		S3Client:   s3Client,
	}
	runner := func(pollCtx context.Context) {
		pollStopFlow(pollCtx, cfg, time.Now())
	}
	return deferredResponse(), runner
}

// pollRunner is the closure spawned after the Discord 3s-ack response is sent.
// The Lambda handler blocks on the runner (via WaitGroup) so the process stays
// alive until the poll loop is finished and the terminal PATCH is issued.
type pollRunner func(ctx context.Context)

func handleHelpCommand(gameName string) InteractionResponse {
	if gameName == "" {
		return ephemeralEmbedResponse(alertEmbed(
			"unknown command",
			"ask a keeper for the right command.",
			"try · /<game> help",
		))
	}
	lines := []string{
		fmt.Sprintf(copyHelpHeader, gameName),
		fmt.Sprintf(copyHelpStatus, gameName),
		fmt.Sprintf(copyHelpStart, gameName),
		fmt.Sprintf(copyHelpStop, gameName),
		fmt.Sprintf(copyHelpHello, gameName),
		fmt.Sprintf(copyHelpHelp, gameName),
	}
	return ephemeralEmbedResponse(lineEmbed("", strings.Join(lines, "\n")))
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

// handleInteraction dispatches a single Discord slash command and returns both
// the immediate InteractionResponse and an optional pollRunner that must run
// after the HTTP response has been written back to Discord.
//
// Kept synchronous (no spawning) so tests can assert responses without
// coordinating goroutines; the caller (handler) manages the WaitGroup.
func handleInteraction(ctx context.Context, interaction Interaction, ec2Client EC2API, ssmClient SSMAPI, s3Client S3API) (LambdaResponse, pollRunner) {
	if interaction.Type != discordSlashCommand {
		return jsonResponse(400, map[string]string{"error": "Not a slash command"}), nil
	}

	// Guild allowlist check — reject requests from non-allowlisted guilds (fail closed)
	if !checkGuildAllowlist(ctx, interaction.GuildID, ssmClient) {
		return jsonResponse(200, ephemeralEmbedResponse(alertEmbed(
			copyAlertGuildBlocked,
			"this guild isn't on the allowlist.",
			"err · guild_not_allowed",
		))), nil
	}

	userID := ""
	if interaction.Member != nil && interaction.Member.User != nil {
		userID = interaction.Member.User.ID
	} else if interaction.User != nil {
		userID = interaction.User.ID
	}

	gameName := interaction.Data.Name

	if len(interaction.Data.Options) == 0 {
		return jsonResponse(200, ephemeralEmbedResponse(alertEmbed(
			"unknown action",
			"try one of: status, start, stop, help, hello.",
			fmt.Sprintf(copyHintTryStatusFmt, gameName),
		))), nil
	}

	action := interaction.Data.Options[0].Name

	var (
		interactionResp InteractionResponse
		runner          pollRunner
	)
	switch action {
	case "status":
		interactionResp = handleStatusCommand(ctx, ec2Client, s3Client, gameName)
	case "start":
		interactionResp, runner = handleStartCommand(ctx, ec2Client, ssmClient, s3Client, userID, gameName, interaction.Token)
	case "stop":
		interactionResp, runner = handleStopCommand(ctx, ec2Client, ssmClient, s3Client, userID, gameName, interaction.Token)
	case "help":
		interactionResp = handleHelpCommand(gameName)
	case "hello":
		interactionResp = handleHelloCommand(ctx, ssmClient, userID, gameName)
	default:
		interactionResp = ephemeralEmbedResponse(alertEmbed(
			"unknown command",
			"ask a keeper for the right command.",
			fmt.Sprintf(copyHintTryStatusFmt, gameName),
		))
	}

	return jsonResponse(200, interactionResp), runner
}

func handler(ctx context.Context, req LambdaRequest) (LambdaResponse, error) {
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
		log.Printf("Error parsing body: %v", err)
		return jsonResponse(500, map[string]string{"error": "Internal server error"}), nil
	}

	// PING — skip guild check, respond immediately
	if body.Type == discordPing {
		return jsonResponse(200, map[string]int{"type": discordPing}), nil
	}

	var interaction Interaction
	if err := json.Unmarshal([]byte(req.Body), &interaction); err != nil {
		log.Printf("Error parsing interaction: %v", err)
		return jsonResponse(500, map[string]string{"error": "Internal server error"}), nil
	}

	ec2Client, err := getEC2Client(ctx)
	if err != nil {
		log.Printf("Error creating EC2 client: %v", err)
		return jsonResponse(500, map[string]string{"error": "Internal server error"}), nil
	}

	ssmClient, err := getSSMClient(ctx)
	if err != nil {
		log.Printf("Error creating SSM client: %v", err)
		return jsonResponse(500, map[string]string{"error": "Internal server error"}), nil
	}

	// S3 client — best-effort; BACKUP field is optional and we don't want a
	// transient S3 config failure to break /start or /stop.
	//
	// Typed as the S3API interface (not *s3.Client) so we can pass a bare `nil`
	// on error — passing a typed-nil *s3.Client through the interface would
	// create a non-nil S3API with a nil concrete value, breaking the `if
	// s3Client != nil` checks in the handlers.
	var s3Client S3API
	if c, err := getS3Client(ctx); err != nil {
		log.Printf("Error creating S3 client (backup lookups will be skipped): %v", err)
	} else {
		s3Client = c
	}

	resp, runner := handleInteraction(ctx, interaction, ec2Client, ssmClient, s3Client)

	// If the handler returned a deferred response, run the poll loop before
	// letting the Lambda exit. The loop PATCHes the original message; once it
	// returns we can let the Lambda process shut down.
	if runner != nil {
		// The poll loop uses its own derived context off `ctx` so it honours the
		// Lambda deadline; we just wait for it to finish here.
		runner(ctx)
	}

	return resp, nil
}

// Compile-time assertions that *ec2.Client and *ssm.Client satisfy their interfaces.
var _ EC2API = (*ec2.Client)(nil)
var _ SSMAPI = (*ssm.Client)(nil)

func main() {
	lambda.Start(handler)
}
