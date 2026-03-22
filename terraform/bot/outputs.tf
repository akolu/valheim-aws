output "discord_bot_endpoint" {
  description = "API Gateway invoke URL to set as the Discord interaction endpoint"
  value       = "${aws_api_gateway_stage.bot.invoke_url}/"
}
