const AWS = require('aws-sdk');
const nacl = require('tweetnacl');

// Discord verification
const verifyDiscordRequest = async (signature, timestamp, body) => {
  try {
    const message = Buffer.from(timestamp + body);
    const signatureBuffer = Buffer.from(signature, 'hex');
    const publicKeyBuffer = Buffer.from(process.env.DISCORD_PUBLIC_KEY, 'hex');
    
    return nacl.sign.detached.verify(
      message,
      signatureBuffer,
      publicKeyBuffer
    );
  } catch (err) {
    console.error('Error verifying signature:', err);
    return false;
  }
};

// Helper for sending responses to Discord
const createJSONResponse = (statusCode, body) => {
  return {
    statusCode,
    headers: {
      'Content-Type': 'application/json',
    },
    body: JSON.stringify(body),
  };
};

// Lightsail operations
const lightsail = new AWS.Lightsail({ region: process.env.AWS_REGION || 'eu-north-1' });

const getInstanceState = async () => {
  const params = {
    instanceName: process.env.INSTANCE_NAME || 'valheim-server',
  };
  
  try {
    const response = await lightsail.getInstanceState(params).promise();
    return response.state.name;
  } catch (error) {
    console.error('Error getting instance state:', error);
    throw error;
  }
};

const startInstance = async () => {
  const params = {
    instanceName: process.env.INSTANCE_NAME || 'valheim-server',
  };
  
  try {
    await lightsail.startInstance(params).promise();
    return 'Server is starting...';
  } catch (error) {
    console.error('Error starting instance:', error);
    throw error;
  }
};

const stopInstance = async () => {
  const params = {
    instanceName: process.env.INSTANCE_NAME || 'valheim-server',
  };
  
  try {
    await lightsail.stopInstance(params).promise();
    return 'Server is stopping...';
  } catch (error) {
    console.error('Error stopping instance:', error);
    throw error;
  }
};

// Command handlers
const handleStatusCommand = async () => {
  try {
    const state = await getInstanceState();
    return {
      type: 4, // CHANNEL_MESSAGE_WITH_SOURCE
      data: {
        content: `Server is currently **${state}**`,
        flags: 64 // Ephemeral flag - makes the message only visible to the caller
      }
    };
  } catch (error) {
    return {
      type: 4,
      data: {
        content: `Error checking server status: ${error.message}`,
        flags: 64 // Ephemeral flag for errors too
      }
    };
  }
};

const handleStartCommand = async (userId) => {
  // Check if user is authorized (optional)
  const authorizedUsers = (process.env.AUTHORIZED_USERS || '').split(',');
  if (authorizedUsers.length > 0 && !authorizedUsers.includes(userId)) {
    return {
      type: 4,
      data: {
        content: `Sorry, you don't have permission to start the server.`
      }
    };
  }
  
  try {
    await startInstance();
    return {
      type: 4,
      data: {
        content: `Server is starting. It will take approximately 2-3 minutes to be available.`
      }
    };
  } catch (error) {
    return {
      type: 4,
      data: {
        content: `Error starting server: ${error.message}`
      }
    };
  }
};

const handleStopCommand = async (userId) => {
  // Check if user is authorized (optional)
  const authorizedUsers = (process.env.AUTHORIZED_USERS || '').split(',');
  if (authorizedUsers.length > 0 && !authorizedUsers.includes(userId)) {
    return {
      type: 4,
      data: {
        content: `Sorry, you don't have permission to stop the server.`
      }
    };
  }
  
  try {
    await stopInstance();
    return {
      type: 4,
      data: {
        content: `Server is stopping. Thank you for saving AWS costs!`
      }
    };
  } catch (error) {
    return {
      type: 4,
      data: {
        content: `Error stopping server: ${error.message}`
      }
    };
  }
};

const handleHelpCommand = async () => {
  return {
    type: 4, // CHANNEL_MESSAGE_WITH_SOURCE
    data: {
      content: `**Available Commands**:
• \`/valheim_status\` - Check if the server is running
• \`/valheim_start\` - Start the server
• \`/valheim_stop\` - Stop the server
• \`/valheim_help\` - Show this help message`,
      flags: 64 // Ephemeral flag - makes the message only visible to the caller
    }
  };
};

// Handle Discord Interaction
const handleInteraction = async (interaction) => {
  // Verify this is a slash command
  if (interaction.type !== 2) { // INTERACTION_TYPE.APPLICATION_COMMAND
    return createJSONResponse(400, { error: 'Not a slash command' });
  }

  // Extract the command name and user ID  
  const commandName = interaction.data.name;
  const userId = interaction.member?.user?.id || interaction.user?.id;
  
  // Handle different commands
  switch (commandName) {
    case 'valheim_status':
      return await handleStatusCommand();
      
    case 'valheim_start':
      return await handleStartCommand(userId);
      
    case 'valheim_stop':
      return await handleStopCommand(userId);
      
    case 'valheim_help':
      return await handleHelpCommand();
      
    default:
      return {
        type: 4,
        data: {
          content: `Unknown command: ${commandName}`
        }
      };
  }
};

// Main Lambda handler
exports.handler = async (event) => {
  try {
    // Extract request information from API Gateway event
    const { headers, body: rawBody } = event;
    
    // Verify the Discord signature
    const signature = headers['x-signature-ed25519'] || headers['X-Signature-Ed25519'];
    const timestamp = headers['x-signature-timestamp'] || headers['X-Signature-Timestamp'];
    
    // For Discord's security requirements
    if (!signature || !timestamp || !rawBody) {
      return createJSONResponse(401, { error: 'Missing signature headers' });
    }
    
    // Verify the signature
    const isValid = await verifyDiscordRequest(signature, timestamp, rawBody);
    if (!isValid) {
      return createJSONResponse(401, { error: 'Invalid signature' });
    }

    // Parse the request body
    const body = JSON.parse(rawBody);
    
    // Handle Discord PING check
    if (body.type === 1) { // INTERACTION_TYPE.PING
      return createJSONResponse(200, { type: 1 }); // INTERACTION_RESPONSE_TYPE.PONG
    }
    
    // Process the interaction
    const response = await handleInteraction(body);
    return createJSONResponse(200, response);
    
  } catch (error) {
    console.error('Error processing request:', error);
    return createJSONResponse(500, { error: 'Internal server error' });
  }
}; 