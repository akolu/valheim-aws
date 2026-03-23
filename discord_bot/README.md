# Bonfire Discord Bot

A Discord bot for controlling game servers on AWS EC2 spot instances.

## Overview

This Discord bot allows your play group to control game servers running on AWS EC2 using slash commands. The bot is implemented as a single shared AWS Lambda function (Go, `provided.al2023` runtime) that handles all games. It communicates with Discord via API Gateway.

## Features

Commands use the format `/<game> <action>`:

- `/<game> status` - Check if the server is running
- `/<game> start` - Start the server (authorized users only)
- `/<game> stop` - Stop the server (authorized users only)
- `/<game> help` - Show available commands
- `/<game> hello` - Say hello

For example: `/valheim start`, `/satisfactory status`, `/valheim hello`

## Prerequisites

- Go 1.25.6 or later
- AWS CLI configured with appropriate credentials
- Terraform installed
- The `bonfire` CLI installed (`make install` in `cli/`)

## Setup

### Step 1: Create a Discord Application

1. Go to the [Discord Developer Portal](https://discord.com/developers/applications)
2. Click "New Application" and give it a name (e.g., "Bonfire")
3. Note down the **Application ID** and **Public Key** from the "General Information" page
4. Go to "Bot" in the sidebar and click "Add Bot"
5. Click "Reset Token" and copy the **Bot Token** (keep it secret!)
6. Go to "OAuth2 > URL Generator":
   - Select scopes: `bot` and `applications.commands`
   - No bot permissions needed (the bot only responds to slash commands)
7. Copy the generated URL and open it to invite the bot to your Discord server

### Step 2: Configure terraform.tfvars

Create `terraform/bot/terraform.tfvars` with your Discord credentials and game server details:

```hcl
discord_public_key    = "your_public_key_from_developer_portal"
discord_bot_token     = "your_bot_token"
discord_app_id        = "your_application_id"
discord_guild_id      = "your_discord_server_id"  # for guild commands (instant)
```

To find your Discord Guild ID: Enable Developer Mode in Discord settings, then right-click your server icon and "Copy Server ID".

### Step 3: Deploy

```bash
AWS_PROFILE=bonfire-deploy bonfire bot deploy
```

This runs the full pipeline: build Lambda binary → `terraform apply` → register slash commands → set Discord interaction endpoint.

To update slash commands or the interaction endpoint without redeploying the Lambda:

```bash
AWS_PROFILE=bonfire-deploy bonfire bot update
```

## Testing

Try the commands in your Discord server:

```
/satisfactory status
/satisfactory start
/satisfactory stop
/satisfactory hello
```

## Troubleshooting

### Commands Not Appearing

- **Guild commands** appear instantly but only in the specified server
- **Global commands** take up to an hour to propagate
- Run `bonfire bot update` to re-register commands

### "Interaction Failed" Error

- Check that Interactions Endpoint URL is set correctly in Discord Developer Portal
- Verify the Lambda deployed successfully: `cd terraform/bot && terraform output discord_bot_endpoint`
- Check Lambda logs in CloudWatch for errors

### "Invalid Signature" or Verification Failures

- Ensure `DISCORD_PUBLIC_KEY` matches in both `terraform.tfvars` and Discord Developer Portal "General Information"
- The public key must be the one from "General Information", not the bot token

### Permission Denied on Start/Stop

- Add your Discord user ID to `discord_authorized_users` in terraform.tfvars
- To find your user ID: Enable Developer Mode in Discord, right-click your name, "Copy User ID"
- Re-run `bonfire bot deploy` after updating
- Note: if `discord_authorized_users` is empty, all users are denied

## Development

### Project Structure

- `go/main.go` - Lambda handler for Discord interactions (Go)
- `go/main_test.go` - Unit tests
- `go/Makefile` - Build targets
- `go/go.mod` / `go/go.sum` - Go module dependencies

### Runtime Details

- **Runtime**: `provided.al2023`
- **Handler**: `bootstrap`
- **Architecture**: `x86_64` (`GOARCH=amd64`)

### Environment Variables (Lambda)

| Variable              | Required | Description                                      |
|-----------------------|----------|--------------------------------------------------|
| `DISCORD_PUBLIC_KEY`  | Yes      | Ed25519 public key from Discord Developer Portal |
| `AWS_REGION`          | No       | Injected automatically by the Lambda runtime     |

### Running Tests

```bash
cd go
go test ./...
```

### Updating Commands

1. Edit command definitions in `go/main.go`
2. Run `bonfire bot update` to re-register with Discord
3. Run `bonfire bot deploy` to rebuild and redeploy the Lambda if handler logic changed
