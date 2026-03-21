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
	"time"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
)

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
	Type   int             `json:"type"`
	Data   InteractionData `json:"data"`
	Member *Member         `json:"member"`
	User   *User           `json:"user"`
}

type Member struct {
	User *User `json:"user"`
}

type InteractionData struct {
	Name    string        `json:"name"`
	Options []SubCommand  `json:"options"`
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
	b, _ := json.Marshal(body)
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

func getEC2Client(ctx context.Context) (*ec2.Client, error) {
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

type instanceInfo struct {
	State        string
	PublicIP     string
	InstanceType string
	LaunchTime   *time.Time
}

func getInstanceState(ctx context.Context) (instanceInfo, error) {
	client, err := getEC2Client(ctx)
	if err != nil {
		return instanceInfo{}, err
	}
	instanceID := os.Getenv("INSTANCE_ID")
	out, err := client.DescribeInstances(ctx, &ec2.DescribeInstancesInput{
		InstanceIds: []string{instanceID},
	})
	if err != nil {
		return instanceInfo{}, err
	}
	if len(out.Reservations) == 0 || len(out.Reservations[0].Instances) == 0 {
		return instanceInfo{State: "not_found"}, nil
	}
	inst := out.Reservations[0].Instances[0]
	info := instanceInfo{
		State:        string(inst.State.Name),
		InstanceType: string(inst.InstanceType),
		LaunchTime:   inst.LaunchTime,
	}
	if inst.PublicIpAddress != nil {
		info.PublicIP = *inst.PublicIpAddress
	} else {
		info.PublicIP = "N/A"
	}
	return info, nil
}

func startInstance(ctx context.Context) error {
	client, err := getEC2Client(ctx)
	if err != nil {
		return err
	}
	_, err = client.StartInstances(ctx, &ec2.StartInstancesInput{
		InstanceIds: []string{os.Getenv("INSTANCE_ID")},
	})
	return err
}

func stopInstance(ctx context.Context) error {
	client, err := getEC2Client(ctx)
	if err != nil {
		return err
	}
	_, err = client.StopInstances(ctx, &ec2.StopInstancesInput{
		InstanceIds: []string{os.Getenv("INSTANCE_ID")},
	})
	return err
}

func isAuthorized(userID string) bool {
	// Matches original JS behavior: empty AUTHORIZED_USERS denies all
	// JS: ''.split(',') = [''] → length > 0, guard fires → everyone denied
	raw := os.Getenv("AUTHORIZED_USERS")
	if raw == "" {
		return false
	}
	for _, uid := range strings.Split(raw, ",") {
		if strings.TrimSpace(uid) == userID {
			return true
		}
	}
	return false
}

func ephemeralResponse(content string) InteractionResponse {
	return InteractionResponse{
		Type: 4,
		Data: &InteractionResponseData{Content: content, Flags: 64},
	}
}

func publicResponse(content string) InteractionResponse {
	return InteractionResponse{
		Type: 4,
		Data: &InteractionResponseData{Content: content},
	}
}

func handleStatusCommand(ctx context.Context) InteractionResponse {
	info, err := getInstanceState(ctx)
	if err != nil {
		return ephemeralResponse(fmt.Sprintf("Error checking server status: %v", err))
	}
	if info.State == "not_found" {
		return ephemeralResponse("Server instance not found. Please check your configuration.")
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

func handleStartCommand(ctx context.Context, userID string) InteractionResponse {
	if !isAuthorized(userID) {
		return publicResponse("Sorry, you don't have permission to start the server.")
	}
	info, err := getInstanceState(ctx)
	if err != nil {
		return publicResponse(fmt.Sprintf("Error starting server: %v", err))
	}
	if info.State == "running" {
		return publicResponse(fmt.Sprintf("Server is already running.\n🖥️ **IP Address**: %s", info.PublicIP))
	}
	if err := startInstance(ctx); err != nil {
		return publicResponse(fmt.Sprintf("Error starting server: %v", err))
	}
	return publicResponse("Server is starting. It will take approximately 2-3 minutes to be available.")
}

func handleStopCommand(ctx context.Context, userID string) InteractionResponse {
	if !isAuthorized(userID) {
		return publicResponse("Sorry, you don't have permission to stop the server.")
	}
	if err := stopInstance(ctx); err != nil {
		return publicResponse(fmt.Sprintf("Error stopping server: %v", err))
	}
	return publicResponse("Server is stopping. Thank you for saving AWS costs!")
}

func handleHelpCommand() InteractionResponse {
	gameName := os.Getenv("GAME_NAME")
	displayName := strings.ToUpper(gameName[:1]) + gameName[1:]
	helpText := fmt.Sprintf("**%s Server Commands:**\n`/%s status` - Check server status\n`/%s start` - Start the server\n`/%s stop` - Stop the server\n`/%s help` - Show this help message",
		displayName, gameName, gameName, gameName, gameName)
	return ephemeralResponse(helpText)
}

func handleInteraction(ctx context.Context, interaction Interaction) LambdaResponse {
	if interaction.Type != 2 {
		return jsonResponse(400, map[string]string{"error": "Not a slash command"})
	}

	gameName := os.Getenv("GAME_NAME")
	if interaction.Data.Name != gameName {
		resp, _ := json.Marshal(ephemeralResponse("Unknown command"))
		return jsonResponse(200, json.RawMessage(resp))
	}

	if len(interaction.Data.Options) == 0 {
		resp, _ := json.Marshal(ephemeralResponse("Unknown action"))
		return jsonResponse(200, json.RawMessage(resp))
	}

	action := interaction.Data.Options[0].Name
	userID := ""
	if interaction.Member != nil && interaction.Member.User != nil {
		userID = interaction.Member.User.ID
	} else if interaction.User != nil {
		userID = interaction.User.ID
	}

	var interactionResp InteractionResponse
	switch action {
	case "status":
		interactionResp = handleStatusCommand(ctx)
	case "start":
		interactionResp = handleStartCommand(ctx, userID)
	case "stop":
		interactionResp = handleStopCommand(ctx, userID)
	case "help":
		interactionResp = handleHelpCommand()
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
		Type        int         `json:"type"`
		Interaction Interaction `json:"-"`
	}
	if err := json.Unmarshal([]byte(req.Body), &body); err != nil {
		log.Printf("Error parsing body: %v", err)
		return jsonResponse(500, map[string]string{"error": "Internal server error"}), nil
	}

	// PING
	if body.Type == 1 {
		return jsonResponse(200, map[string]int{"type": 1}), nil
	}

	var interaction Interaction
	if err := json.Unmarshal([]byte(req.Body), &interaction); err != nil {
		log.Printf("Error parsing interaction: %v", err)
		return jsonResponse(500, map[string]string{"error": "Internal server error"}), nil
	}

	return handleInteraction(ctx, interaction), nil
}

func main() {
	gameName := os.Getenv("GAME_NAME")
	if gameName == "" {
		log.Fatal("GAME_NAME environment variable is required")
	}
	lambda.Start(handler)
}
