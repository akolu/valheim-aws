# bonfire CLI

The `bonfire` CLI manages game servers: provisioning, status, backups, and Discord bot configuration.

## Project Layout

```
cli/
├── main.go          — entry point, registers root command
├── go.mod / go.sum  — module dependencies
├── Makefile         — build/install targets
└── cmd/
    ├── root.go      — root cobra command
    ├── aws.go       — shared AWS helpers (config, S3, EC2 primitives)
    ├── list.go      — `bonfire list`
    ├── status.go    — `bonfire status <game>`
    ├── provision.go — `bonfire provision <game>`
    ├── retire.go    — `bonfire retire <game>`
    ├── archive.go   — long-term archive helpers
    ├── bot.go       — `bonfire bot deploy` / `bonfire bot update` (bot deployment and command registration)
    ├── bot_auth.go  — `bonfire bot grant/revoke/trust/untrust` (SSM-backed ACLs)
    └── terraform.go — terraform invocation helpers, parseTFVars, availableGames
```

## How Commands Are Wired

Each command is a `*cobra.Command` defined in its own file. Commands register
themselves onto their parent in `init()` functions:

```
rootCmd  (root.go)
├── list      (list.go)
├── status    (status.go)
├── provision (provision.go)
├── retire    (retire.go)
└── bot       (bot.go)
    ├── deploy   (bot.go)
    ├── update   (bot.go)
    ├── grant    (bot_auth.go)
    ├── revoke   (bot_auth.go)
    ├── trust    (bot_auth.go)
    └── untrust  (bot_auth.go)
```

`main.go` calls `cmd.Execute()`, which invokes cobra's dispatch.

## Running Tests

```bash
go test ./...
```

All tests are in `cmd/` alongside the code they test (`*_test.go` files).
Tests use in-process mocks for AWS clients — no real AWS calls are made.

## Building and Installing

```bash
make install   # go install → installs bonfire to $GOPATH/bin
make build     # go build  → produces ./bonfire binary
```

Ensure `$GOPATH/bin` (usually `~/go/bin`) is on your `$PATH`.

## Bot Commands

### `bonfire bot deploy`

Runs the full bot deployment pipeline in sequence:

1. `make build` — compiles the Go Lambda binary and packages it into a zip
2. `terraform apply` — deploys the Lambda and API Gateway infrastructure (`terraform/bot/`)
3. `bot update` — registers slash commands and sets the Discord interaction endpoint

**Requires:** `terraform/bot/terraform.tfvars` (created during first-time setup). If the file
is missing the command exits with a friendly error explaining that the bot has not been
deployed yet.

**Pipeline stops on failure** — if `make build` fails, terraform is not invoked. If terraform
fails, the Discord API update is not attempted.

```bash
AWS_PROFILE=bonfire-deploy bonfire bot deploy
```

### `bonfire bot update`

Updates the Discord interaction endpoint and re-registers slash commands without rebuilding
or re-deploying the Lambda. Safe to re-run at any time — skips the Discord API call if
neither the endpoint nor the command list has changed.

## Key Environment Variables

| Variable | Purpose | Default |
|---|---|---|
| `BONFIRE_REPO_ROOT` | Path to repo root (where `terraform/` lives) | Auto-detected |
| `AWS_PROFILE` | AWS credentials profile | `bonfire-deploy` |
| `AWS_REGION` | AWS region | `eu-north-1` |
