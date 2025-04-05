# Valheim Discord Bot

A Discord bot for controlling a Valheim server on AWS Lightsail.

## Overview

This Discord bot allows your play group to control a Valheim server running on AWS Lightsail using slash commands. The bot is implemented as an AWS Lambda function that communicates with Discord via API Gateway.

## Features

- `/valheim_status` - Check if the server is running
- `/valheim_start` - Start the server (authorized users only)
- `/valheim_stop` - Stop the server (authorized users only)
- `/valheim_help` - Show available commands

## Prerequisites

- Node.js 16 or later
- AWS account with Lambda, API Gateway, and Lightsail permissions
- Discord bot token and application ID

## Setup

1. **Create a Discord Application**:

   - Go to the [Discord Developer Portal](https://discord.com/developers/applications)
   - Create a new application
   - Under "Bot", create a bot user
   - Copy the bot token (keep it secret)
   - Under "OAuth2 > URL Generator", select the `bot` and `applications.commands` scopes
   - Invite the bot to your server using the generated URL

2. **Set Up Environment Variables**:

   - Copy `.env.example` to `.env`

   ```bash
   cp .env.example .env
   ```

   - Edit `.env` and fill in your Discord credentials:
     - `DISCORD_BOT_TOKEN` - Your bot's token
     - `DISCORD_APP_ID` - Your application ID (NOT the bot user ID)
     - `DISCORD_GUILD_ID` - (Optional) Server ID for testing
     - `DISCORD_PUBLIC_KEY` - Public key for signature verification
     - `AUTHORIZED_USERS` - (Optional) Comma-separated Discord user IDs allowed to start/stop the server

3. **Register Discord Slash Commands**:

   ```bash
   # Register for a specific guild (faster updates, better for testing)
   npm run register-commands

   # Or register globally (takes up to an hour to appear)
   npm run register-commands:global
   ```

4. **Build the Lambda Package**:

   ```bash
   npm run build
   ```

5. **Deploy the Lambda Function**:
   - Deploy the generated `valheim_discord_bot.zip` package to AWS Lambda using Terraform (see the terraform directory)

## Development

### Project Structure

- `src/index.js` - Lambda function code for handling Discord interactions
- `register-commands.js` - Script to register slash commands with Discord
- `package.json` - Project dependencies and npm scripts
- `.env` - Environment variables (not committed to version control)
- `.env.example` - Template for environment variables

### Available Scripts

- `npm run register-commands` - Register commands to a specific guild (testing)
- `npm run register-commands:global` - Register global commands (production)
- `npm run build` - Build the Lambda deployment package

### Modifying Commands

To change or add commands:

1. Edit the commands in `register-commands.js`
2. Run `npm run register-commands` to update Discord
3. Update the corresponding command handling in `src/index.js`
4. Build the Lambda package with `npm run build`
5. Deploy the updated package

### Local Testing

To test the Lambda function locally:

1. Set up your environment variables in `.env`
2. Use a tool like `aws-sam-local` or AWS SAM CLI to invoke the function

## Troubleshooting

### Command Registration Issues

- **Invalid Token**: Ensure your bot token is correct in the `.env` file
- **Missing Permissions**: Make sure your bot has the `applications.commands` scope
- **Guild vs Global Commands**: Guild commands register instantly but are limited to one server. Global commands can take up to an hour to propagate.

### Discord Verification Failures

- The bot uses Ed25519 signature verification to validate requests from Discord
- If verification fails, ensure your DISCORD_PUBLIC_KEY environment variable is correct
- API Gateway must use Lambda Proxy Integration to pass the raw request body
