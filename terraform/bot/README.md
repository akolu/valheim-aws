# terraform/bot

Provisions the shared Bonfire Discord bot infrastructure on AWS.

## What it creates

- **Lambda function** (`bonfire_bot`) — Go binary that handles Discord interactions for all games
- **API Gateway HTTP API (v2)** — receives POST requests from Discord and forwards to Lambda
- **IAM role + policy** — least-privilege permissions for the Lambda (EC2, SSM, CloudWatch Logs, SQS DLQ)
- **CloudWatch Log Groups** — Lambda logs and API Gateway access logs (14-day retention)
- **SQS Dead Letter Queue** (`bonfire_bot_dlq`) — captures failed Lambda invocations for inspection

## Prerequisites

The Lambda zip must be built before running `terraform apply`:

```bash
cd discord_bot/go
make build   # outputs bonfire_discord_bot.zip to discord_bot/
```

Terraform reads the zip from `../../discord_bot/bonfire_discord_bot.zip` relative to this directory.

## Credentials

Copy `terraform.tfvars.example` to `terraform.tfvars` and fill in:

| Variable | Used by |
|----------|---------|
| `discord_public_key` | Terraform (Lambda env var for signature verification) |
| `discord_application_id` | `bonfire bot update` CLI only — not used by Terraform |
| `discord_bot_token` | `bonfire bot update` CLI only — not used by Terraform |

`terraform.tfvars` is gitignored. Never commit it.

## How to apply

```bash
cd terraform/bot
terraform init
terraform plan
terraform apply
```

## Post-apply steps

After a successful `terraform apply`, register Discord slash commands:

```bash
bonfire bot update
```

This reads `discord_application_id` and `discord_bot_token` from `terraform.tfvars` and
registers all slash commands with the Discord API.

Set the Discord interaction endpoint URL to the value of `discord_bot_endpoint` output:

```bash
terraform output discord_bot_endpoint
```

Paste that URL into the Discord Developer Portal under your application's "Interactions Endpoint URL".
