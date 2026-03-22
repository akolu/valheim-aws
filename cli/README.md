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
    ├── bot.go       — `bonfire bot update` (Discord command registration)
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

## Key Environment Variables

| Variable | Purpose | Default |
|---|---|---|
| `BONFIRE_REPO_ROOT` | Path to repo root (where `terraform/` lives) | Auto-detected |
| `AWS_PROFILE` | AWS credentials profile | `bonfire-deploy` |
| `AWS_REGION` | AWS region | `eu-north-1` |
