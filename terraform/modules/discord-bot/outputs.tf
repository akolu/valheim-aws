output "discord_bot_url" {
  description = "The URL to use for Discord interactions endpoint"
  value       = aws_apigatewayv2_api.discord_api_gateway.api_endpoint
}

output "discord_bot_name" {
  description = "The name of the Discord bot Lambda function"
  value       = aws_lambda_function.discord_bot.function_name
}

output "setup_instructions" {
  description = "Instructions for setting up the Discord bot"
  value       = <<-EOT
    To set up your Discord bot:
    
    1. Go to Discord Developer Portal (https://discord.com/developers/applications)
    2. Create a new application or use an existing one
    3. Under "Bot", create a bot user and copy the token
    4. Under "General Information", copy the Application ID and Public Key
    5. Set up slash commands by using the Discord API or a tool like discord.js
    6. Configure your application's Interactions Endpoint URL to: ${aws_apigatewayv2_api.discord_api_gateway.api_endpoint}
    7. Invite the bot to your server with appropriate permissions
  EOT
}
