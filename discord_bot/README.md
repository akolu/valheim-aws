# Bonfire Discord Bot

A Discord bot for controlling game servers on AWS EC2 spot instances.

## Overview

This Discord bot allows your play group to control game servers running on AWS EC2 using slash commands. The bot is implemented as an AWS Lambda function that communicates with Discord via API Gateway. Each game server requires its own Discord application and bot instance.

## Features

Commands use the format `/<game> <action>`:

- `/<game> status` - Check if the server is running
- `/<game> start` - Start the server (authorized users only)
- `/<game> stop` - Stop the server (authorized users only)
- `/<game> help` - Show available commands

For example: `/valheim start`, `/satisfactory status`

## Prerequisites

- Node.js 16 or later
- AWS CLI configured with appropriate credentials
- Terraform installed
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
npm install
npm run register-commands
```

This registers `/<game>` commands to your Discord server. You should see them appear immediately.

### Step 4: Build the Lambda Package

```bash
npm run build
```

This creates `bonfire_discord_bot.zip` ready for deployment.

### Step 5: Configure Terraform

Edit your game's `terraform.tfvars` (e.g., `terraform/games/satisfactory/terraform.tfvars`):

```hcl
enable_discord_bot       = true
discord_public_key       = "your_public_key"
discord_application_id   = "your_application_id"
discord_bot_token        = "your_bot_token"
discord_authorized_users = ["your_discord_user_id"]  # Optional
```

### Step 6: Deploy with Terraform

```bash
cd terraform/games/satisfactory  # or your game directory
terraform apply
```

Note the `discord_bot_endpoint` output URL.

### Step 7: Configure Discord Interactions Endpoint

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
- Ensure you ran `npm run register-commands` with correct credentials

### "Interaction Failed" Error

- Check that Interactions Endpoint URL is set correctly in Discord Developer Portal
- Verify the Lambda deployed successfully: `terraform output discord_bot_endpoint`
- Check Lambda logs in CloudWatch for errors

### "Invalid Signature" or Verification Failures

- Ensure `DISCORD_PUBLIC_KEY` matches in both `.env` and `terraform.tfvars`
- The public key must be the one from "General Information", not the bot token

### Permission Denied on Start/Stop

- Add your Discord user ID to `discord_authorized_users` in terraform.tfvars
- To find your user ID: Enable Developer Mode in Discord, right-click your name, "Copy User ID"
- Re-run `terraform apply` after updating

## Development

### Project Structure

- `src/index.js` - Lambda handler for Discord interactions
- `register-commands.js` - Script to register slash commands
- `.env` - Local environment variables (not committed)
- `.env.example` - Template for environment variables

### Available Scripts

- `npm run register-commands` - Register commands to a specific guild
- `npm run register-commands:global` - Register global commands (all servers)
- `npm run build` - Build the Lambda deployment package

### Updating Commands

1. Edit command definitions in `register-commands.js`
2. Run `npm run register-commands`
3. Update handler logic in `src/index.js` if needed
4. Run `npm run build`
5. Run `terraform apply` to deploy
