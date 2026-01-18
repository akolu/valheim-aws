#!/usr/bin/env node

/**
 * Simple script to register slash commands with Discord.
 * 
 * Usage:
 *   npm run register-commands
 *   npm run register-commands:global
 * 
 * When using --global flag, this will automatically delete any guild-specific commands
 * since they would be duplicates of the global commands.
 */

// Load environment variables from .env file
require('dotenv').config();

const { REST, Routes } = require('discord.js');

// Get command line arguments
const isGlobal = process.argv.includes('--global');

// Get game name from environment (must be after dotenv.config())
const gameName = process.env.GAME_NAME || 'valheim';

// Check for required environment variables
const { DISCORD_BOT_TOKEN, DISCORD_APP_ID, DISCORD_GUILD_ID } = process.env;

if (!DISCORD_BOT_TOKEN || !DISCORD_APP_ID) {
  console.error('Error: Required environment variables missing');
  console.error('Please set DISCORD_BOT_TOKEN and DISCORD_APP_ID in your .env file');
  process.exit(1);
}

if (!isGlobal && !DISCORD_GUILD_ID) {
  console.error('Error: DISCORD_GUILD_ID required for guild commands');
  console.error('Set DISCORD_GUILD_ID in your .env file or use --global for global commands');
  process.exit(1);
}

// Define slash commands
const commands = [
  {
    name: 'server',
    description: 'Control game servers',
    options: [
      {
        name: gameName,
        description: `Control the ${gameName} server`,
        type: 2, // SUB_COMMAND_GROUP
        options: [
          {
            name: 'status',
            description: `Check if the ${gameName} server is running`,
            type: 1, // SUB_COMMAND
          },
          {
            name: 'start',
            description: `Start the ${gameName} server`,
            type: 1,
          },
          {
            name: 'stop',
            description: `Stop the ${gameName} server`,
            type: 1,
          },
          {
            name: 'help',
            description: `Show available commands for the ${gameName} server`,
            type: 1,
          },
        ],
      },
    ],
  },
];

// Initialize Discord REST API client
const rest = new REST({ version: '10' }).setToken(DISCORD_BOT_TOKEN);

// Register the commands
(async () => {
  try {
    // If registering global commands and GUILD_ID is available, delete guild commands first
    if (isGlobal && DISCORD_GUILD_ID) {
      console.log(`Registering global commands, clearing guild commands from ${DISCORD_GUILD_ID} to avoid duplicates...`);
      
      try {
        await rest.put(
          Routes.applicationGuildCommands(DISCORD_APP_ID, DISCORD_GUILD_ID),
          { body: [] }
        );
        console.log('✓ Guild commands cleared successfully');
      } catch (guildError) {
        console.warn('⚠️ Failed to clear guild commands:', guildError.message);
        console.warn('You may need to manually delete guild commands if they exist');
      }
    }
    
    console.log(`Registering ${commands.length} slash commands...`);
    
    // Route depends on whether we're registering global or guild commands
    const route = isGlobal
      ? Routes.applicationCommands(DISCORD_APP_ID)
      : Routes.applicationGuildCommands(DISCORD_APP_ID, DISCORD_GUILD_ID);
    
    // PUT completely replaces all commands
    const data = await rest.put(route, { body: commands });
    
    console.log(`✅ Success! Registered ${data.length} commands`);
    console.log(`Commands: ${data.map(cmd => cmd.name).join(', ')}`);
    
    if (isGlobal) {
      console.log('\nNote: Global commands can take up to an hour to appear in all servers');
    }
  } catch (error) {
    console.error('❌ Error registering commands:');
    if (error.status === 401) {
      console.error('Invalid bot token');
    } else if (error.status === 403) {
      console.error('Missing permissions - check your bot scopes');
    } else if (error.status === 404) {
      console.error('Not found - check your application/guild IDs');
    } else {
      console.error(error);
    }
  }
})(); 