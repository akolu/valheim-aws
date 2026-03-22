const AWS = require('aws-sdk');
const nacl = require('tweetnacl');

// Game name from environment variable (required)
const GAME_NAME = process.env.GAME_NAME;
if (!GAME_NAME) {
  throw new Error('GAME_NAME environment variable is required');
}

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

// EC2 operations
const ec2 = new AWS.EC2({ region: process.env.AWS_REGION || 'eu-north-1' });

const getInstanceState = async () => {
  const params = {
    InstanceIds: [process.env.INSTANCE_ID]
  };
  
  try {
    const response = await ec2.describeInstances(params).promise();
    if (response.Reservations.length > 0 && 
        response.Reservations[0].Instances.length > 0) {
      const instance = response.Reservations[0].Instances[0];
      return {
        state: instance.State.Name,
        publicIp: instance.PublicIpAddress || 'N/A',
        instanceType: instance.InstanceType,
        launchTime: instance.LaunchTime
      };
    }
    return { state: 'not_found' };
  } catch (error) {
    console.error('Error getting instance state:', error);
    throw error;
  }
};

const startInstance = async () => {
  const params = {
    InstanceIds: [process.env.INSTANCE_ID]
  };
  
  try {
    await ec2.startInstances(params).promise();
    return 'Server is starting...';
  } catch (error) {
    console.error('Error starting instance:', error);
    throw error;
  }
};

const stopInstance = async () => {
  const params = {
    InstanceIds: [process.env.INSTANCE_ID]
  };
  
  try {
    await ec2.stopInstances(params).promise();
    return 'Server is stopping...';
  } catch (error) {
    console.error('Error stopping instance:', error);
    throw error;
  }
};

// Command handlers
const handleStatusCommand = async () => {
  try {
    const instanceInfo = await getInstanceState();
    
    let statusMessage = '';
    if (instanceInfo.state === 'not_found') {
      statusMessage = 'Server instance not found. Please check your configuration.';
    } else {
      statusMessage = `Server is currently **${instanceInfo.state}**\n`;
      
      if (instanceInfo.state === 'running') {
        const uptime = Math.round((new Date() - new Date(instanceInfo.launchTime)) / (60 * 1000));
        statusMessage += `🖥️ **IP Address**: ${instanceInfo.publicIp}\n`;
        statusMessage += `⚙️ **Instance Type**: ${instanceInfo.instanceType}\n`;
        statusMessage += `⏱️ **Uptime**: ${uptime} minutes`;
      }
    }
    
    return {
      type: 4, // CHANNEL_MESSAGE_WITH_SOURCE
      data: {
        content: statusMessage,
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
    // Check if instance exists and its state
    const instanceInfo = await getInstanceState();
    
    // If instance is already running
    if (instanceInfo.state === 'running') {
      return {
        type: 4,
        data: {
          content: `Server is already running.\n🖥️ **IP Address**: ${instanceInfo.publicIp}`
        }
      };
    }
    
    // If instance exists but stopped, start it
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
  const displayName = GAME_NAME.charAt(0).toUpperCase() + GAME_NAME.slice(1);
  const helpText = `**${displayName} Server Commands:**
\`/${GAME_NAME} status\` - Check server status
\`/${GAME_NAME} start\` - Start the server
\`/${GAME_NAME} stop\` - Stop the server
\`/${GAME_NAME} help\` - Show this help message`;

  return {
    type: 4, // CHANNEL_MESSAGE_WITH_SOURCE
    data: {
      content: helpText,
      flags: 64 // Ephemeral flag - makes the message only visible to the caller
    }
  };
};

// Helper to format error responses
const formatResponse = (content, ephemeral = true) => {
  return {
    type: 4, // CHANNEL_MESSAGE_WITH_SOURCE
    data: {
      content,
      flags: ephemeral ? 64 : 0
    }
  };
};

// Handle Discord Interaction
const handleInteraction = async (interaction) => {
  // Verify this is a slash command
  if (interaction.type !== 2) { // INTERACTION_TYPE.APPLICATION_COMMAND
    return createJSONResponse(400, { error: 'Not a slash command' });
  }

  const { name, options } = interaction.data;
  const userId = interaction.member?.user?.id || interaction.user?.id;

  // Command is /<game> <action> (e.g., /valheim start, /satisfactory status)
  if (name === GAME_NAME) {
    const subcommand = options?.[0];
    if (!subcommand) {
      return formatResponse('Unknown action', true);
    }

    const action = subcommand.name;

    switch (action) {
      case 'status':
        return await handleStatusCommand();
      case 'start':
        return await handleStartCommand(userId);
      case 'stop':
        return await handleStopCommand(userId);
      case 'help':
        return await handleHelpCommand();
      default:
        return formatResponse('Unknown command', true);
    }
  }

  return formatResponse('Unknown command', true);
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