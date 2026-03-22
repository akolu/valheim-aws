# Single Shared Discord Bot Design

**Goal:** Replace the current per-game Discord bot architecture (one Lambda + Discord app per game) with a single shared Lambda that handles all games.

**Motivation:** As more games are added, managing N Discord applications, N bot tokens, and N Lambdas becomes increasingly tedious. A single bot is simpler to operate and cheaper to run.

**Status:** Design complete. Not yet implemented — this is a planned architectural refactor (bo-ggp).

---

## Current Architecture (per-game)

```
terraform/games/valheim/     → EC2 + S3 backup + Lambda + API GW + Discord app A
terraform/games/satisfactory/ → EC2 + S3 backup + Lambda + API GW + Discord app B
terraform/games/factorio/    → EC2 + S3 backup + Lambda + API GW + Discord app C
```

Each Lambda has hardcoded env vars: `GAME_NAME`, `INSTANCE_ID`, `DISCORD_PUBLIC_KEY`, `AUTHORIZED_USERS`.

## Target Architecture (single bot)

```
terraform/games/valheim/      → EC2 + S3 backup only (no Discord resources)
terraform/games/satisfactory/ → EC2 + S3 backup only
terraform/games/factorio/     → EC2 + S3 backup only
terraform/bot/                → single Lambda + API GW + one Discord app (all games)
```

---

## Key Design Decisions

### Instance Discovery — EC2 Tags

The Lambda infers the target game from the slash command name (`/valheim start` → game=valheim) and queries EC2 at invocation time:

```
tag:Game=valheim
tag:Project=bonfire
```

This pattern is already used in the CLI (`status.go`, `list.go`). No `INSTANCE_ID` env var needed. Adding a new game requires no Lambda redeployment — the new EC2 instance is discovered automatically.

### Authorization — SSM Parameter Store

Per-game authorized user lists stored in SSM Standard Parameters (free tier):

```
/bonfire/valheim/authorized_users       → "discord_id1,discord_id2"
/bonfire/satisfactory/authorized_users  → "discord_id3,discord_id4"
/bonfire/factorio/authorized_users      → "discord_id1,discord_id3"
```

The Lambda reads the relevant parameter at invocation time — changes take effect immediately without redeployment. Update via CLI:

```bash
aws ssm put-parameter --name /bonfire/valheim/authorized_users \
  --value "id1,id2,id3" --type String --overwrite
```

Or via `bonfire bot update valheim --authorized-users id1,id2,id3` (future CLI enhancement).

**Why SSM over EC2 tags:** Tags require update regardless, so the "stateless" benefit of tags doesn't apply. SSM is the conventional place for per-resource config, is IAM-controllable, and is free at this scale.

**Behavior:** If parameter is absent or empty → allow all users (same as current default).

### Per-game Discord Guild

Each game can live in a different Discord server. Store guild IDs in SSM:

```
/bonfire/valheim/guild_id       → "discord_guild_id_1"
/bonfire/satisfactory/guild_id  → "discord_guild_id_2"
```

The Lambda uses the guild ID to scope responses if needed. Slash commands are registered per guild via `bonfire bot update`.

### Terraform Workspace

New `terraform/bot/` workspace — same pattern as `terraform/archive/`. Independent state, survives per-game `terraform destroy`. Contains:
- Lambda function (single Go binary handling all games)
- API Gateway
- IAM role with EC2 describe + start/stop permissions (all games) + SSM read permissions under `/bonfire/`

Per-game stacks: remove `enable_discord_bot`, `discord_public_key`, `discord_application_id`, `discord_bot_token`, `discord_authorized_users` variables and all associated resources.

### Lambda Environment Variables (new)

| Variable | Purpose |
|----------|---------|
| `DISCORD_PUBLIC_KEY` | Single Discord app public key for signature verification |
| `AWS_REGION` | Region for EC2 + SSM queries |

Everything else (game name, instance ID, authorized users, guild ID) is dynamic — inferred from the command or read from SSM/EC2 at invocation time.

---

## Migration Path

1. Deploy `terraform/bot/` with the new shared Lambda
2. Register all game slash commands to the single Discord app (`bonfire bot update`)
3. Update the Discord app's Interactions Endpoint URL (via portal or `bonfire bot update`)
4. Verify all games respond correctly in Discord
5. Remove Discord resources from per-game stacks: delete variables + resources, `terraform apply` each game
6. Decommission old per-game Discord apps in the Developer Portal

---

## Go Lambda Changes

- Remove `GAME_NAME` env var — infer from interaction command name
- Remove `INSTANCE_ID` env var — query EC2 by `tag:Game` + `tag:Project=bonfire`
- Remove per-game `AUTHORIZED_USERS` env var — read from SSM `/bonfire/<game>/authorized_users`
- Single binary handles N games without configuration changes when games are added/removed

---

## CLI Impact

`bonfire bot update [game]` (bo-07d, already implemented) works with this architecture unchanged — it registers commands and syncs the endpoint for the single Discord app. Minor enhancement needed: `--authorized-users` flag to update SSM parameters directly.

`bonfire bot deploy` was explicitly deferred (YAGNI) — the single bot design makes it even less necessary since first-time setup is a one-time operation for the shared bot, not per-game.
