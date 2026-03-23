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
    ├── update.go    — `bonfire update` (pull latest source and reinstall CLI)
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
├── update    (update.go)
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

## Commands

### `bonfire list`

Lists all games with their current EC2 instance state and public IP.

```bash
bonfire list
```

**Output columns:**

| Column | Description |
|--------|-------------|
| `GAME` | Game name (from terraform workspace) |
| `STATE` | EC2 instance state: `running`, `stopped`, `not-provisioned`, etc. |
| `IP` | Public IP address, or `-` if not available |

**Example output:**
```
GAME                 STATE            IP
----                 -----            --
valheim              running          13.48.12.34
minecraft            not-provisioned  -
```

---

### `bonfire status <game>`

Shows detailed status for a single game server.

```bash
bonfire status valheim
```

**Output fields:**

| Field | Description |
|-------|-------------|
| `Instance ID` | EC2 instance ID (e.g. `i-0abc123`) |
| `Instance State` | EC2 state: `running`, `stopped`, `pending`, etc. |
| `Public IP` | Current public IP, or `-` if stopped |
| `Last Backup` | S3 path of the most recent backup in `bonfire-<game>-backups-<region>` |
| `Long-term Archives` | Count and latest snapshot timestamp in `<game>-long-term-backups` |

**Example output:**
```
Status: valheim
----------------------------------------
  Instance ID:    i-0abc1234def56789
  Instance State: running
  Public IP:      13.48.12.34
  Last Backup:    s3://bonfire-valheim-backups-eu-north-1/world.db.gz
  Long-term Archives: 3 snapshots, latest 2026-03-15T120000Z
```

---

### `bonfire provision <game>`

Provisions a game server via terraform. If a long-term archive exists, the latest
snapshot is automatically restored so the server has existing save data on first boot.

```bash
bonfire provision valheim
```

**What it does:**
1. Runs `terraform init` and `terraform apply` for the game workspace
2. Checks `<game>-long-term-backups` S3 bucket for an existing archive
3. If an archive is found, copies the latest snapshot into the short-term backup bucket
   (`bonfire-<game>-backups-<region>`) — the game server picks it up automatically on boot
4. If no archive is found, the server starts fresh

The restore step is fully automatic — no prompts. The most recent snapshot is always used.

---

### `bonfire retire <game>`

Archives all save files to long-term storage, then destroys the game server infrastructure.
This is the "end of season" command.

```bash
bonfire retire valheim
```

**What it does:**
1. Copies all objects from the short-term backup bucket into `<game>-long-term-backups`
   under a timestamped prefix (e.g. `2026-03-21T150405Z/`)
2. Runs `terraform plan -destroy` and shows the planned destruction
3. Prompts you to type the game name to confirm
4. Runs `terraform apply` on the destroy plan

**Confirmation prompt:**
```
Type the game name to confirm destroy: valheim
```
Type the exact game name and press Enter to proceed. Any other input aborts with no changes.

The long-term bucket is **not** destroyed — only the EC2 instance and short-term backup bucket
are removed. Save data is preserved for future re-provisioning.

---

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
bonfire bot deploy
```

---

### `bonfire bot update`

Updates the Discord interaction endpoint and re-registers slash commands without rebuilding
or re-deploying the Lambda. Safe to re-run at any time — skips the Discord API call if
neither the endpoint nor the command list has changed.

```bash
bonfire bot update
```

---

### `bonfire bot grant <game> <user_id>`

Grants a Discord user access to a game's bot commands. Writes to SSM parameter
`/bonfire/<game>/authorized_users` as a comma-separated list of user IDs.

```bash
bonfire bot grant valheim 123456789012345678
```

**Idempotent** — running it again for an already-granted user is safe.

**Finding a Discord user ID:** In Discord, enable Developer Mode (Settings → Advanced),
then right-click any user and select "Copy User ID".

**SSM path:** `/bonfire/<game>/authorized_users`

---

### `bonfire bot revoke <game> <user_id>`

Revokes a Discord user's access to a game's bot commands. Removes the user ID from
`/bonfire/<game>/authorized_users`. If the list becomes empty, the SSM parameter is deleted.

```bash
bonfire bot revoke valheim 123456789012345678
```

**Idempotent** — revoking a user who was never granted access is safe.

---

### `bonfire bot trust <guild_id>`

Adds a Discord guild (server) to the global allowed-guilds list. The bot only responds
to commands from trusted guilds. Writes to SSM parameter `/bonfire/allowed_guilds`.

```bash
bonfire bot trust 987654321098765432
```

**Finding a guild ID:** In Discord, enable Developer Mode (Settings → Advanced),
then right-click any server name and select "Copy Server ID".

**SSM path:** `/bonfire/allowed_guilds`

---

### `bonfire bot untrust <guild_id>`

Removes a Discord guild from the allowed-guilds list. The bot will stop responding to
commands from that guild. If the list becomes empty, the SSM parameter is deleted.

```bash
bonfire bot untrust 987654321098765432
```

---

### `bonfire update`

Pulls the latest source from `origin/main` and reinstalls the CLI binary:

1. `git pull` — updates the local repository
2. `make install` — rebuilds and installs the `bonfire` binary

```bash
bonfire update
```

---

### Version and update checks

The `bonfire` binary embeds the git commit hash at build time via ldflags:

```bash
go build -ldflags "-X github.com/bonfire/cli/cmd.Version=$(git rev-parse HEAD)"
```

On every command run (except `bonfire update`), bonfire checks `origin/main` in the
background and prints a notice after the command completes if a newer version is available:

```
Notice: bonfire update available (latest: a1b2c3d4). Run 'bonfire update' to upgrade.
```

The check is non-blocking: if it takes more than 2 seconds, it is silently skipped.
Development builds (where `Version` is `"dev"`) never show the update notice.

---

## Key Environment Variables

| Variable | Purpose | Default |
|---|---|---|
| `BONFIRE_REPO_ROOT` | Path to repo root (where `terraform/` lives) | Auto-detected |
| `AWS_PROFILE` | AWS credentials profile | `bonfire-deploy` |
| `AWS_REGION` | AWS region | `eu-north-1` |

`AWS_PROFILE` defaults to `bonfire-deploy` if not set in the environment. To use a
different profile for a single command, prefix it:

```bash
AWS_PROFILE=my-profile bonfire list
```
