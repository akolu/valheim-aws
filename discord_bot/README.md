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
- `/<game> hello` - Check bot connectivity and your authorization status

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
discord_public_key     = "your_public_key_from_developer_portal"
discord_bot_token      = "your_bot_token"
discord_application_id = "your_application_id"
```

### Step 3: Avatar upload (one-time)

The bot ships with a branded avatar in `discord_bot/assets/avatar/`. Upload it once:

1. Go to the [Discord Developer Portal](https://discord.com/developers/applications) → your app → **Bot** → **Icon**
2. Drag-drop `discord_bot/assets/avatar/bonfire-lit-512.png` into the icon slot
3. Click **Save Changes**

The `lit` and `unlit` variants in 128/256/512/1024 are provided for future use (stateful avatar swaps, marketing, etc.) but Discord only uses one icon at a time.

### Step 4: Deploy

```bash
AWS_PROFILE=bonfire-deploy bonfire bot deploy
```

This runs the full pipeline: build Lambda binary → `terraform apply` → register slash commands → set Discord interaction endpoint.

To update slash commands or the interaction endpoint without redeploying the Lambda:

```bash
AWS_PROFILE=bonfire-deploy bonfire bot update
```

### Step 5: Post-Deployment Authorization

After deployment, configure who can use the bot via the `bonfire bot` CLI:

**Allow your Discord server (guild) to use the bot:**

```bash
bonfire bot trust <guild_id>
```

To find your guild ID: Enable Developer Mode in Discord settings, then right-click your server icon and "Copy Server ID".

**Grant users permission to start/stop servers:**

```bash
bonfire bot grant <game> <user_id>
```

To find a user ID: Enable Developer Mode in Discord, right-click the user's name, "Copy User ID".

For example:

```bash
bonfire bot trust 123456789012345678
bonfire bot grant satisfactory 987654321098765432
```

## Authorization

The bot uses an SSM-backed ACL system:

- **Guild allowlist** — stored at `/bonfire/allowed_guilds` in SSM. Only guilds in this list can use the bot. Add guilds with `bonfire bot trust <guild_id>`.
- **Per-game user lists** — stored at `/bonfire/<game>/authorized_users` in SSM. Only users in this list can start/stop servers for that game. Manage with `bonfire bot grant/revoke <game> <user_id>`.

Commands are registered **globally** (not guild-specific) and are available in all servers where the bot is invited. Access control is enforced at runtime via the allowlists above.

Use `/<game> hello` to check bot connectivity and see your authorization status for a game.

## Testing

Try the commands in your Discord server:

```
/satisfactory hello
/satisfactory status
/satisfactory start
/satisfactory stop
/satisfactory hello
```

`/<game> hello` is a good first test — it shows whether the bot is reachable and whether you are authorized.

## Troubleshooting

### Commands Not Appearing

- Global commands can take up to an hour to propagate after registration
- Commands are registered globally and will appear in all servers where the bot is invited
- Run `bonfire bot update` to re-register commands

### "Interaction Failed" Error

- Check that Interactions Endpoint URL is set correctly in Discord Developer Portal
- Verify the Lambda deployed successfully: `cd terraform/bot && terraform output discord_bot_endpoint`
- Check Lambda logs in CloudWatch for errors

### "Invalid Signature" or Verification Failures

- Ensure `DISCORD_PUBLIC_KEY` matches in both `terraform.tfvars` and Discord Developer Portal "General Information"
- The public key must be the one from "General Information", not the bot token

### Permission Denied on Start/Stop

- Grant your Discord user ID access using the CLI:
  ```bash
  bonfire bot grant <game> <user_id>
  ```
- To find your user ID: Enable Developer Mode in Discord, right-click your name, "Copy User ID"
- Use `/<game> hello` to confirm your authorization status after granting access

### Bot Not Available in This Server

If the bot responds with "This bot is not available in this server", the guild has not been added to the allowlist:

```bash
bonfire bot trust <guild_id>
```

To find your guild ID: Enable Developer Mode in Discord settings, right-click your server icon and "Copy Server ID".

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

| Variable              | Required | Description                                                                                |
|-----------------------|----------|--------------------------------------------------------------------------------------------|
| `DISCORD_PUBLIC_KEY`  | Yes      | Ed25519 public key from Discord Developer Portal                                           |
| `DISCORD_APP_ID`      | Yes      | Discord application ID — used to construct webhook PATCH URLs for deferred `/start` / `/stop` responses |
| `AWS_REGION`          | No       | Injected automatically by the Lambda runtime                                               |

### Running Tests

```bash
cd go
go test ./...
```

### Updating Commands

1. Edit command definitions in `go/main.go`
2. Run `bonfire bot update` to re-register with Discord
3. Run `bonfire bot deploy` to rebuild and redeploy the Lambda if handler logic changed
