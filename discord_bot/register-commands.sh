#!/usr/bin/env bash
# Register Discord slash commands via the Discord REST API.
#
# Usage:
#   ./register-commands.sh           # Register guild commands (default)
#   ./register-commands.sh --global  # Register global commands (all servers)
#
# When using --global and DISCORD_GUILD_ID is set, guild commands are cleared
# first to avoid duplicates.
#
# Required env vars: DISCORD_BOT_TOKEN, DISCORD_APP_ID, GAME_NAME
# Required for guild commands: DISCORD_GUILD_ID

set -euo pipefail

# Load .env if present
if [ -f "$(dirname "$0")/.env" ]; then
  # shellcheck disable=SC1091
  source "$(dirname "$0")/.env"
fi

IS_GLOBAL=false
for arg in "$@"; do
  if [ "$arg" = "--global" ]; then
    IS_GLOBAL=true
  fi
done

# Validate required env vars
if [ -z "${GAME_NAME:-}" ]; then
  echo "Error: GAME_NAME environment variable is required" >&2
  echo "Set GAME_NAME in your .env file (e.g., GAME_NAME=valheim)" >&2
  exit 1
fi

if [ -z "${DISCORD_BOT_TOKEN:-}" ] || [ -z "${DISCORD_APP_ID:-}" ]; then
  echo "Error: Required environment variables missing" >&2
  echo "Please set DISCORD_BOT_TOKEN and DISCORD_APP_ID in your .env file" >&2
  exit 1
fi

if [ "$IS_GLOBAL" = false ] && [ -z "${DISCORD_GUILD_ID:-}" ]; then
  echo "Error: DISCORD_GUILD_ID required for guild commands" >&2
  echo "Set DISCORD_GUILD_ID in your .env file or use --global for global commands" >&2
  exit 1
fi

# Build the JSON payload for the single /<game> command with all subcommands
COMMANDS_JSON=$(cat <<EOF
[
  {
    "name": "${GAME_NAME}",
    "description": "Control the ${GAME_NAME} server",
    "options": [
      {"name": "status", "description": "Check if the ${GAME_NAME} server is running", "type": 1},
      {"name": "start",  "description": "Start the ${GAME_NAME} server",               "type": 1},
      {"name": "stop",   "description": "Stop the ${GAME_NAME} server",                "type": 1},
      {"name": "help",   "description": "Show available commands for the ${GAME_NAME} server", "type": 1},
      {"name": "hello",  "description": "Check if the bot is reachable and verify your authorization status", "type": 1}
    ]
  }
]
EOF
)

DISCORD_API="https://discord.com/api/v10"

# Helper: make a Discord API call, print response body, exit non-zero on HTTP error
discord_put() {
  local url="$1"
  local body="$2"
  local description="$3"

  http_code=$(curl -s -o /tmp/discord_response.json -w "%{http_code}" \
    -X PUT "$url" \
    -H "Authorization: Bot ${DISCORD_BOT_TOKEN}" \
    -H "Content-Type: application/json" \
    -d "$body")

  if [ "$http_code" -ge 200 ] && [ "$http_code" -lt 300 ]; then
    echo "$description succeeded (HTTP ${http_code})"
  else
    echo "Error: $description failed (HTTP ${http_code})" >&2
    case "$http_code" in
      401) echo "Invalid bot token" >&2 ;;
      403) echo "Missing permissions - check your bot scopes" >&2 ;;
      404) echo "Not found - check your application/guild IDs" >&2 ;;
      *)   cat /tmp/discord_response.json >&2; echo >&2 ;;
    esac
    return 1
  fi
}

# If --global and GUILD_ID is set, clear guild commands first to avoid duplicates
if [ "$IS_GLOBAL" = true ] && [ -n "${DISCORD_GUILD_ID:-}" ]; then
  echo "Registering global commands, clearing guild commands from ${DISCORD_GUILD_ID} to avoid duplicates..."
  if discord_put \
    "${DISCORD_API}/applications/${DISCORD_APP_ID}/guilds/${DISCORD_GUILD_ID}/commands" \
    "[]" \
    "Clear guild commands"; then
    echo "✓ Guild commands cleared successfully"
  else
    echo "⚠ Failed to clear guild commands — you may need to delete them manually" >&2
  fi
fi

echo "Registering 1 slash command..."

if [ "$IS_GLOBAL" = true ]; then
  URL="${DISCORD_API}/applications/${DISCORD_APP_ID}/commands"
else
  URL="${DISCORD_API}/applications/${DISCORD_APP_ID}/guilds/${DISCORD_GUILD_ID}/commands"
fi

if discord_put "$URL" "$COMMANDS_JSON" "Register commands"; then
  # Count registered commands from response
  count=$(grep -o '"id"' /tmp/discord_response.json | wc -l | tr -d ' ')
  echo "✅ Success! Registered ${count} commands"
  # Print command names
  names=$(grep -o '"name":"[^"]*"' /tmp/discord_response.json | cut -d: -f2 | tr -d '"' | paste -sd ', ')
  echo "Commands: ${names}"
  if [ "$IS_GLOBAL" = true ]; then
    echo ""
    echo "Note: Global commands can take up to an hour to appear in all servers"
  fi
fi
