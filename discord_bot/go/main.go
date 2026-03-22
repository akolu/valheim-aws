package main

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"math"
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
	discordPing           = 1
	discordSlashCommand   = 2
	discordChannelMessage = 4
	discordEphemeralFlag  = 64
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
	Type int                     `json:"type"`
	Data *InteractionResponseData `json:"data,omitempty"`
}

type InteractionResponseData struct {
	Content string `json:"content"`
	Flags   int    `json:"flags,omitempty"`
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

// Package-level client singletons for connection reuse across warm Lambda invocations.
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

func getEC2Client(ctx context.Context) (*ec2.Client, error) {
	ec2ClientOnce.Do(func() {
		sharedEC2Client, ec2ClientErr = newEC2Client(ctx)
	})
	return sharedEC2Client, ec2ClientErr
}

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

func ephemeralResponse(content string) InteractionResponse {
	return InteractionResponse{
		Type: discordChannelMessage,
		Data: &InteractionResponseData{Content: content, Flags: discordEphemeralFlag},
	}
}

func publicResponse(content string) InteractionResponse {
	return InteractionResponse{
		Type: discordChannelMessage,
		Data: &InteractionResponseData{Content: content},
	}
}

func handleStatusCommand(ctx context.Context, client EC2API, gameName string) InteractionResponse {
	info, err := findInstanceByGame(ctx, client, gameName)
	if err != nil {
		return ephemeralResponse(fmt.Sprintf("Error checking server status: %v", err))
	}
	if info.State == "not_found" {
		return ephemeralResponse(fmt.Sprintf("No server found for %s.", gameName))
	}
	if info.State == "multiple" {
		return ephemeralResponse(fmt.Sprintf("Multiple instances found for %s — ambiguous.", gameName))
	}
	msg := fmt.Sprintf("Server is currently **%s**\n", info.State)
	if info.State == "running" && info.LaunchTime != nil {
		uptime := int(math.Round(time.Since(*info.LaunchTime).Minutes()))
		msg += fmt.Sprintf("🖥️ **IP Address**: %s\n", info.PublicIP)
		msg += fmt.Sprintf("⚙️ **Instance Type**: %s\n", info.InstanceType)
		msg += fmt.Sprintf("⏱️ **Uptime**: %d minutes", uptime)
	}
	return ephemeralResponse(msg)
}

func handleStartCommand(ctx context.Context, ec2Client EC2API, ssmClient SSMAPI, userID, gameName string) InteractionResponse {
	if !isAuthorizedSSM(ctx, userID, gameName, ssmClient) {
		return publicResponse("Sorry, you don't have permission to start the server.")
	}
	info, err := findInstanceByGame(ctx, ec2Client, gameName)
	if err != nil {
		return publicResponse(fmt.Sprintf("Error starting server: %v", err))
	}
	if info.State == "not_found" {
		return ephemeralResponse(fmt.Sprintf("No server found for %s.", gameName))
	}
	if info.State == "multiple" {
		return ephemeralResponse(fmt.Sprintf("Multiple instances found for %s — ambiguous.", gameName))
	}
	if info.State == "running" {
		return publicResponse(fmt.Sprintf("Server is already running.\n🖥️ **IP Address**: %s", info.PublicIP))
	}
	if err := startInstance(ctx, ec2Client, info.InstanceID); err != nil {
		return publicResponse(fmt.Sprintf("Error starting server: %v", err))
	}
	return publicResponse("Server is starting. It will take approximately 2-3 minutes to be available.")
}

func handleStopCommand(ctx context.Context, ec2Client EC2API, ssmClient SSMAPI, userID, gameName string) InteractionResponse {
	if !isAuthorizedSSM(ctx, userID, gameName, ssmClient) {
		return publicResponse("Sorry, you don't have permission to stop the server.")
	}
	info, err := findInstanceByGame(ctx, ec2Client, gameName)
	if err != nil {
		return publicResponse(fmt.Sprintf("Error stopping server: %v", err))
	}
	if info.State == "not_found" {
		return ephemeralResponse(fmt.Sprintf("No server found for %s.", gameName))
	}
	if info.State == "multiple" {
		return ephemeralResponse(fmt.Sprintf("Multiple instances found for %s — ambiguous.", gameName))
	}
	if info.State == "stopped" || info.State == "stopping" {
		return publicResponse("Server is already stopped.")
	}
	if err := stopInstance(ctx, ec2Client, info.InstanceID); err != nil {
		return publicResponse(fmt.Sprintf("Error stopping server: %v", err))
	}
	return publicResponse("Server is stopping. Thank you for saving AWS costs!")
}

func handleHelpCommand(gameName string) InteractionResponse {
	if gameName == "" {
		return ephemeralResponse("Unknown command")
	}
	displayName := strings.ToUpper(gameName[:1]) + gameName[1:]
	helpText := fmt.Sprintf("**%s Server Commands:**\n`/%s status` - Check server status\n`/%s start` - Start the server\n`/%s stop` - Stop the server\n`/%s help` - Show this help message",
		displayName, gameName, gameName, gameName, gameName)
	return ephemeralResponse(helpText)
}

func handleHelloCommand(ctx context.Context, ssmClient SSMAPI, userID, gameName string) InteractionResponse {
	authStatus := "not authorized"
	if isAuthorizedSSM(ctx, userID, gameName, ssmClient) {
		authStatus = "authorized"
	}
	msg := fmt.Sprintf("👋 Bot is reachable!\n👤 **Your user ID**: %s\n🔐 **Authorization**: %s\n🎮 **Game**: %s",
		userID, authStatus, gameName)
	return ephemeralResponse(msg)
}

func handleInteraction(ctx context.Context, interaction Interaction, ec2Client EC2API, ssmClient SSMAPI) LambdaResponse {
	if interaction.Type != discordSlashCommand {
		return jsonResponse(400, map[string]string{"error": "Not a slash command"})
	}

	// Guild allowlist check — reject requests from non-allowlisted guilds (fail closed)
	if !checkGuildAllowlist(ctx, interaction.GuildID, ssmClient) {
		return jsonResponse(200, ephemeralResponse("This bot is not available in this server."))
	}

	userID := ""
	if interaction.Member != nil && interaction.Member.User != nil {
		userID = interaction.Member.User.ID
	} else if interaction.User != nil {
		userID = interaction.User.ID
	}

	gameName := interaction.Data.Name

	if len(interaction.Data.Options) == 0 {
		return jsonResponse(200, ephemeralResponse("Unknown action"))
	}

	action := interaction.Data.Options[0].Name

	var interactionResp InteractionResponse
	switch action {
	case "status":
		interactionResp = handleStatusCommand(ctx, ec2Client, gameName)
	case "start":
		interactionResp = handleStartCommand(ctx, ec2Client, ssmClient, userID, gameName)
	case "stop":
		interactionResp = handleStopCommand(ctx, ec2Client, ssmClient, userID, gameName)
	case "help":
		interactionResp = handleHelpCommand(gameName)
	case "hello":
		interactionResp = handleHelloCommand(ctx, ssmClient, userID, gameName)
	default:
		interactionResp = ephemeralResponse("Unknown command")
	}

	return jsonResponse(200, interactionResp)
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

	return handleInteraction(ctx, interaction, ec2Client, ssmClient), nil
}

// Compile-time assertions that *ec2.Client and *ssm.Client satisfy their interfaces.
var _ EC2API = (*ec2.Client)(nil)
var _ SSMAPI = (*ssm.Client)(nil)

func main() {
	lambda.Start(handler)
}
