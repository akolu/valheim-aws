# Bonfire Discord Bot

A Discord bot for controlling game servers on AWS EC2 spot instances.

## Overview

This Discord bot allows your play group to control game servers running on AWS EC2 using slash commands. The bot is implemented as an AWS Lambda function (Go, `provided.al2023` runtime) that communicates with Discord via API Gateway. Each game server requires its own Discord application and bot instance.

## Features

Commands use the format `/<game> <action>`:

- `/<game> status` - Check if the server is running
- `/<game> start` - Start the server (authorized users only)
- `/<game> stop` - Stop the server (authorized users only)
- `/<game> help` - Show available commands

For example: `/valheim start`, `/satisfactory status`

## Prerequisites

- Go 1.25.6 or later
- AWS CLI configured with appropriate credentials
- Terraform installed
- curl (for registering slash commands)
- A deployed game server (you'll need the EC2 instance ID)

## Setup

### Step 1: Create a Discord Application

1. Go to the [Discord Developer Portal](https://discord.com/developers/applications)
2. Click "New Application" and give it a name (e.g., "Bonfire Satisfactory")
3. Note down the **Application ID** and **Public Key** from the "General Information" page
4. Go to "Bot" in the sidebar and click "Add Bot"
5. Click "Reset Token" and copy the **Bot Token** (keep it secret!)
6. Go to "OAuth2 > URL Generator":
   - Select scopes: `bot` and `applications.commands`
   - No bot permissions needed (the bot only responds to slash commands)
7. Copy the generated URL and open it to invite the bot to your Discord server

### Step 2: Configure Environment Variables

```bash
cd discord_bot
cp .env.example .env
```

Edit `.env` with your values:

```bash
# Required: The game this bot controls (e.g., valheim, satisfactory)
GAME_NAME=satisfactory

# Required: From Discord Developer Portal
DISCORD_BOT_TOKEN=your_bot_token
DISCORD_APP_ID=your_application_id
DISCORD_PUBLIC_KEY=your_public_key

# Required for guild commands (recommended for testing)
DISCORD_GUILD_ID=your_discord_server_id
```

To find your Discord Guild ID: Enable Developer Mode in Discord settings, then right-click your server icon and "Copy Server ID".

### Step 3: Register Slash Commands

```bash
./register-commands.sh
```

This registers `/<game>` commands to your Discord server. You should see them appear immediately.

For global commands (all servers, takes up to an hour to propagate):

```bash
./register-commands.sh --global
```

### Step 4: Build the Lambda Package

```bash
cd go
make build
```

This cross-compiles the Go binary for Linux (`GOOS=linux GOARCH=amd64`), producing a `bootstrap` binary, and packages it as `../bonfire_discord_bot.zip` ready for deployment.

The Lambda uses the `provided.al2023` runtime with `bootstrap` as the handler.

### Step 5: Deploy with Terraform

```bash
cd terraform/bot
terraform apply
```

Note the `discord_bot_endpoint` output URL.

### Step 6: Configure Discord Interactions Endpoint

This is the critical step that connects Discord to your Lambda:

1. Go back to [Discord Developer Portal](https://discord.com/developers/applications)
2. Select your application
3. Go to "General Information"
4. Paste the `discord_bot_endpoint` URL into **Interactions Endpoint URL**
5. Click "Save Changes"

Discord will verify the endpoint by sending a PING request. If it fails, check that your Lambda deployed correctly.

## Testing

Try the commands in your Discord server:

```
/satisfactory status
/satisfactory start
/satisfactory stop
```

## Troubleshooting

### Commands Not Appearing

- **Guild commands** appear instantly but only in the specified server
- **Global commands** take up to an hour to propagate
- Ensure you ran `./register-commands.sh` with correct credentials

### "Interaction Failed" Error

- Check that Interactions Endpoint URL is set correctly in Discord Developer Portal
- Verify the Lambda deployed successfully: `cd terraform/bot && terraform output discord_bot_endpoint`
- Check Lambda logs in CloudWatch for errors

### "Invalid Signature" or Verification Failures

- Ensure `DISCORD_PUBLIC_KEY` matches in both `.env` and `terraform.tfvars`
- The public key must be the one from "General Information", not the bot token

### Permission Denied on Start/Stop

- Add your Discord user ID to `discord_authorized_users` in terraform.tfvars
- To find your user ID: Enable Developer Mode in Discord, right-click your name, "Copy User ID"
- Re-run `terraform apply` after updating
- Note: if `discord_authorized_users` is empty, all users are denied

## Development

### Project Structure

- `go/main.go` - Lambda handler for Discord interactions (Go)
- `go/main_test.go` - Unit tests
- `go/Makefile` - Build targets
- `go/go.mod` / `go/go.sum` - Go module dependencies
- `register-commands.sh` - Shell script to register slash commands (requires curl)
- `.env` - Local environment variables (not committed)
- `.env.example` - Template for environment variables

### Available Scripts

- `./register-commands.sh` - Register commands to a specific guild
- `./register-commands.sh --global` - Register global commands (all servers)
- `make build` (in `go/`) - Build and package the Lambda deployment zip

### Runtime Details

- **Runtime**: `provided.al2023`
- **Handler**: `bootstrap`
- **Architecture**: `x86_64` (`GOARCH=amd64`)

### Environment Variables (Lambda)

| Variable            | Required | Description                                      |
|---------------------|----------|--------------------------------------------------|
| `GAME_NAME`         | Yes      | The slash command name (e.g., `satisfactory`)    |
| `DISCORD_PUBLIC_KEY`| Yes      | Ed25519 public key from Discord Developer Portal |
| `INSTANCE_ID`       | Yes      | EC2 instance ID to control                       |
| `AUTHORIZED_USERS`  | No       | Comma-separated Discord user IDs for start/stop  |
| `AWS_REGION`        | No       | AWS region (defaults to `eu-north-1`)            |

### Running Tests

```bash
cd go
go test ./...
```

### Updating Commands

1. Edit command definitions in `register-commands.sh`
2. Run `./register-commands.sh`
3. Update handler logic in `go/main.go` if needed
4. Run `make build` in `go/`
5. Run `terraform apply` to deploy
