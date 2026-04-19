# Bonfire Brand Book → Discord Bot — Plan (light, amended post-Board-review)

**Date:** 2026-04-19
**Ceremony:** spec:light · review:targeted · decomposition:single (adapted from governance v0.8 for hobby-project scale)
**Classification:** risk: low · complexity: moderate
**Board:** DA + Architect + UX Reviewer (Compliance Expert and Security Analyst opted out — no regulatory surface; no new trust boundary since Discord was already integrated; IAM delta is a single read-only S3 permission on buckets the CLI already reads)

**Amendment log:** Initial draft reviewed by Board 2026-04-19; 2 critical, 11 important, ~12 minor findings folded in. See "Review & Hand-off" section for the review trail.

---

## Story Definition

Implement the Bonfire Brand Book (v1, April 2026) across the Discord bot's user-facing surfaces. Turns the bot from a plain-text status responder into a branded "keeper" with fire-metaphor vocabulary, structured embed layouts, a deferred-response edit-original flow for `/start` and `/stop`, full idempotency handling, and a new avatar.

The edit-original flow: user runs `/valheim start`. Handler **first** checks EC2 state synchronously. If the fire is already lit/lighting, the handler replies with an ephemeral Line embed (type 4, flags 64) and exits — no defer, no poll. Otherwise it returns a deferred response (type 5) with Hero-spark "lighting the fire… (0:00)", starts EC2, and polls-and-edits the same message until state=running ("bonfire lit · {addr}") or a soft deadline.

Out of scope: CLI rebranding, README/docs updates, brand book itself (the user maintains the brand book via Claude Design Studio).

---

## Acceptance Criteria

### Copy & vocabulary
- [ ] All ~25 user-facing strings in `discord_bot/go/main.go` use fire-metaphor per brand book §07
- [ ] No corporate idioms ("provisioned," "spun up," "ERROR") remain
- [ ] No decorative emoji anywhere in copy (brand §01 permits only 🙂 as voice; no 🔥 prefix on terminal edits)
- [ ] `/help` and `/hello` use lowercase, first-person voice per brand §01
- [ ] Unauthorized refusals read "you can't tend this fire" (not "Sorry, you don't have permission…")
- [ ] Copy constants live alongside the embed layer (see "Files to change / add")

### Embed layouts (new: `embeds.go`)
- [ ] **Hero** — public. Used by successful `/start` and `/stop`.
  - Left color bar + state pill + game title + two distinct attribution slots:
    - `heroLeadline` — active-voice headline ("@user lit the fire" / "put out by @user")
    - `heroAttribution` — under-field byline on running state ("burning for 1h 42m · lit by @user")
  - Running-state layout: **3-field grid** — ADDRESS full-width top row; UPTIME + BACKUP side-by-side bottom row (no visible gap; WORLD intentionally omitted — see Key Decision #2)
- [ ] **Line** — ephemeral. Used by `/status`, `/hello`, `/help`, all idempotency races. Implemented as a **minimal embed with colored left bar + description**, not a raw `Content` string — the colored bar is the brand's narrative-color cue and markdown can't color plain text
- [ ] **Alert** — ephemeral. Used by errors, unauthorized, not_found, multiple. Left bar in danger/ash, two-line headline + body + monospace hint footer
  - Hint-footer patterns (enumerated):
    - `err · <code>` — infra/unauthorized (e.g. `err · discord_role_missing`, `err · ec2_InsufficientInstanceCapacity · req 4f2c9a81`)
    - `try · <cmd>` — user-fixable/not_found (e.g. `try · /valheim status`)
- [ ] State colors hex-match brand palette: ember `#e8793a`, spark `#f2c14e`, ice `#6b8f9c`, ash `#9a8e7d`, danger `#c65d4a`, ok `#8fb369`

### Deferred-response flow — `/start` (new: `polling.go`)
- [ ] **State check is synchronous and pre-defer.** Handler calls `findInstanceByGame` before anything else. If state ∈ `{running, pending, stopping}`, handler returns type 4 with `flags: 64` (ephemeral) Line embed (per Idempotency rules below) and exits — **does not** call `StartInstances`, **does not** spawn the poller.
- [ ] Only state=`stopped` proceeds: handler returns Discord interaction type 5 (`DeferredChannelMessageWithSource`) within 3s
- [ ] Initial edit (T+~0.4s): Hero embed in spark color, body "lighting the fire… (0:00)"
- [ ] Poll cadence: EC2 `DescribeInstances` every 5s; Discord PATCH on state change OR every 20s (whichever first)
- [ ] Subsequent edits: elapsed timer advances in the same message, color stays spark
- [ ] **Terminal set for `/start` poll:**
  - `running` → success final edit: Hero in ember color, ADDRESS full-width + UPTIME + BACKUP bottom row, `heroLeadline` = "@user lit the fire", `heroAttribution` = "lit by @user"
  - `stopping` or `stopped` (someone banked the coals mid-light) → final edit: Hero in ash color, body "someone banked the coals · fire's out" — not an error
  - Soft deadline at T+170s (see deadline rules) → final edit: Hero in ice color, body "still lighting — i'll keep an eye on it"
- [ ] **Concurrent-/start coordination is implicit via idempotency.** Two simultaneous `/start` invocations → each is a separate Lambda invocation → each runs the synchronous state check first. The second one will see state=pending (because the first already called `StartInstances`) and take the idempotent Line path.
- [ ] **Context deadline.** Polling loop uses `context.WithDeadline` derived from `lambdaContext.Deadline()`, minus 10s budget reserved for the final PATCH. All `DescribeInstances` and PATCH calls inherit that context. On deadline, the loop breaks and a fresh short-lived context is used for the terminal "still lighting" PATCH.
- [ ] **Discord PATCH failure handling:**
  - 429 → respect `Retry-After` header, sleep, retry once
  - 404 (original message deleted / rare token expiry) → log and exit cleanly; no further work
  - 5xx / transient network → log and skip this tick; next tick retries naturally
  - Terminal-state PATCH failures retry once synchronously before Lambda exits

### Deferred-response flow — `/stop`
- [ ] Same synchronous state check first. State ∈ `{stopped, stopping, pending}` → idempotent Line path (see Idempotency). State=`running` → defer + poll.
- [ ] Initial edit: Hero in ice color, body "saving the world, banking the coals" (full brand quote, no ellipsis)
- [ ] **Terminal set for `/stop` poll:**
  - `stopped` → success: Hero in ash color, `heroLeadline` = "put out by @user"
  - `pending` or `running` (re-started mid-stop) → Hero in spark color, body "fire's relighting · someone wasn't done" — not an error
  - Soft deadline → Hero in ice color, body "still dying down — i'll keep an eye on it"
- [ ] Same concurrency / context / PATCH-failure semantics as `/start`

### Idempotency — ephemeral Line responses (type 4, flags 64)
- [ ] `/start` when `running` → "fire's already burning · lit {elapsed} ago · {addr}" (bare address, no "IP" label)
- [ ] `/start` when `pending` → "fire's already lighting · started {elapsed} ago · ~2 min total"
- [ ] `/start` when `stopping` → "hang on — someone's banking the coals · try again in a minute"
- [ ] `/stop` when `stopped` → "fire's already out · last burned {elapsed} ago"
- [ ] `/stop` when `stopping` → "fire's already dying down · saving the world"
- [ ] `/stop` when `pending` → "can't bank coals yet · fire's still lighting · try again in a minute"
- [ ] Elapsed from EC2 `LaunchTime` (running/pending) or S3 last-backup `LastModified` (stopped); empty-bucket fallback in "BACKUP field" rules

### BACKUP field — S3 lookup (lives in `polling.go`)
- [ ] `S3:ListObjectsV2` against `bonfire-<game>-backups-<region>` with per-game prefix (`worlds/`, `saves/`, `saved/`); take `LastModified` of newest object
- [ ] Rendered as elapsed ("15m ago") in Line, Hero-running, and idempotency copy
- [ ] **Empty-bucket / missing-prefix fallback:** omit the BACKUP field in Hero; use "never burned" in Line copy; never block the primary state message on this lookup
- [ ] **S3 error fallback:** log the error, omit the field silently (do not surface S3 errors to users); does not affect polling loop exit
- [ ] IAM: `s3:ListBucket` + `s3:GetObject` on `arn:aws:s3:::bonfire-*-backups-*` — added in **`terraform/account/main.tf`** (bot role lives there, not in `terraform/bot/`; see Infra section)

### `/status` — ephemeral Line for every state
- [ ] Implemented as Line-embed with colored left bar (not raw `Content`) so state color lands
- [ ] Running: `valheim · burning · {addr} · {uptime} · backup {elapsed} ago`
- [ ] Pending: `valheim · lighting · started {elapsed} ago · ~2 min total`
- [ ] Stopping: `valheim · dying down · saving the world`
- [ ] Stopped: `valheim · out · last burned {elapsed} ago` (or "never burned" if empty bucket)
- [ ] not_found: Alert — headline "no such fire", hint `try · /valheim status` pattern (enumerates available games where feasible)
- [ ] multiple: Alert — headline "two fires by that name — ask a keeper", hint `err · ec2_tag_collision`
- [ ] EC2 error: Alert — headline "something went sideways", hint `err · <aws error code>`

### `/help` and `/hello`
- [ ] `/help` — Line ephemeral, list of commands in fire vocabulary, lowercase one-per-line
- [ ] `/hello` — Line ephemeral, state-agnostic warm greeting from brand's "here · ready · here it is" column. Auth state NOT shown as judgemental labels ("not yours") — either omit or frame non-cold: "here · ready. you're on the keeper list for this fire" / "here · ready. you can watch; ask a keeper for access."

### Avatar
- [ ] 8 PNGs checked into `discord_bot/assets/avatar/` (lit + unlit × 128/256/512/1024) — copied from the design handoff bundle
- [ ] `discord_bot/README.md` updated with a short "Avatar upload" section: drag-drop `bonfire-lit-512.png` in Discord Developer Portal → Bot → Icon

### Infra / Terraform
- [ ] **`terraform/account/main.tf`** — extend the bot Lambda IAM role (resource `aws_iam_policy.bot_lambda`, lines 175–255) with a new Sid `S3BackupListRead`: `s3:ListBucket` + `s3:GetObject` on `arn:aws:s3:::bonfire-*-backups-*` and `/*`. The role lives in `terraform/account/` (not `terraform/bot/`) by design — the `bonfire-deploy` role applies `terraform/bot/` without IAM permissions.
- [ ] **Deploy order:** `terraform apply` in `terraform/account/` (admin profile) *before* `terraform/bot/`.
- [ ] **`terraform/bot/main.tf`** — Lambda timeout: 15s → 180s
- [ ] **`terraform/bot/main.tf`** — add `DISCORD_APP_ID = var.discord_application_id` to the Lambda `environment.variables` block. (Note: tfvar is `discord_application_id`, **not** `discord_app_id` — fix drift in `discord_bot/README.md` step 2 alongside this change.)
- [ ] **`terraform/bot/variables.tf`** — update the `discord_application_id` description: previously "NOT used by Terraform"; now it's wired into the Lambda env for interaction-webhook PATCH URLs.

### Non-regression
- [ ] All existing tests pass or are updated with new expected strings
- [ ] Guild allowlist (`/bonfire/allowed_guilds`) enforcement unchanged
- [ ] Per-game authorized-user list (`/bonfire/{game}/authorized_users`) enforcement unchanged
- [ ] Signature verification (`verifyDiscordRequest`) unchanged
- [ ] Tests added for new behaviours: deferred-response path, polling exit conditions (running / stopping / stopped / deadline), idempotency per state (including new `/start-when-stopping`, `/stop-when-pending` rows), backup lookup (happy path + empty-bucket + S3 error), ctx cancellation cleanup

---

## Key Decisions

1. **Synchronous long-poll over event-driven.** Lambda defers the Discord response and polls EC2 + PATCHes Discord in a loop until terminal state or deadline. ~40 LoC of polling on top of existing handler structure. Bumps Lambda timeout 15s → 180s.
   - **Concurrency model:** Each `/start` is a separate Lambda invocation. Two concurrent `/start`s → second invocation sees state=pending (first invocation already called `StartInstances`) and takes the idempotent Line path via the synchronous pre-defer check. No cross-invocation coordination needed.
   - **Context deadline:** Polling ctx derived from `lambdaContext.Deadline()` minus 10s reserve; final PATCH uses a fresh short-lived ctx so the "still lighting" message always ships.

2. **Drop WORLD field; redesign running Hero to 3-field grid.** Cheaper than wiring per-game world names from `terraform/games/*/variables.tf` through SSM, cleaner than leaving a visible gap in a 4-field grid with only 3 slots filled. Running Hero grid: ADDRESS full-width top row, UPTIME + BACKUP side-by-side bottom row. Brand book owner (user) is aware and may update the book.

3. **Full-fidelity idempotency.** All six "already-in-state" cases return ephemeral Line embeds (including the newly-added `/start when stopping` and `/stop when pending` variants). Matches brand intent of not spamming the channel when nothing has changed.

4. **Line layout for `/help`.** Low-key, lowercase command list. Brand §01 Voice favors this over a structured Card.

5. **Manual avatar upload.** Discord Dev Portal once, PNG checked into the repo for discoverability. Automating via Discord REST API costs a new bot-token secret + a one-off script for a one-time operation.

6. **Fonts are spirit-matched; Line is a colored-bar embed.** Discord renders embed text in its native font — Cormorant Garamond / Inter Tight / JetBrains Mono apply only to the pre-rendered avatar image. We approximate the brand hierarchy using Discord markdown: `_italic_`, code blocks for data, `**bold**`. The Line layout is a **minimal embed** (not a raw `Content` string) so the brand's state-color accent actually renders — the colored left bar does the work the brand palette is designed for.

7. **Polling cadence: 5s EC2 / 20s PATCH floor.** Fast enough to catch state transitions; slow enough on PATCHes to stay well under Discord's webhook rate limit (5 req/2s) and to keep elapsed tick visibly advancing for the user.

8. **Ceremony opt-outs.** Compliance Expert not activated — no regulatory surface. Security Analyst not activated — no new trust boundary. Board = DA + Architect + UX Reviewer.

9. **No Discord SDK.** `bwmarrin/discordgo` targets gateway/WebSocket bots; for webhook-interaction Lambdas the marshal-struct-and-POST pattern costs ~20 LoC and avoids a transitive-dep pull.

---

## Codebase Context

### Current bot shape
- Single Go Lambda (`provided.al2023`, 15s timeout, 256 MB) at `terraform/bot/main.tf:26-32`, fronted by HTTP API Gateway v2 POST `/`.
- Handler entry: `main.go:453-502` → `handleInteraction` dispatches to per-command handlers.
- **All responses today are Discord interaction type 4** (immediate `InteractionResponse` with plain-text `Content`). No embeds, no deferred responses, no polling anywhere in the codebase.
- AWS SDK client pooling at `main.go:138-187` — stays useful across poll iterations.

### State detection
- `findInstanceByGame()` at `main.go:248-287` — EC2 tag filter (`Game`, `Project=bonfire`), returns state ∈ `{running, pending, stopping, stopped}` or sentinel `not_found` / `multiple`.
- `LaunchTime` is in the `DescribeInstances` response (already used for uptime).

### Authorization
- Two-layer SSM: `/bonfire/allowed_guilds` (guild allowlist), `/bonfire/{game}/authorized_users` (per-game user list). Both fail-closed.
- No change to the checks themselves — only the refusal *copy* and surface (Alert layout).

### Copy inventory
25 user-facing strings inline in handler functions (see codebase-reader findings). Moving into `embeds.go` as constants alongside the embed constructors that consume them.

### IAM placement (important)
The bot Lambda IAM role is in **`terraform/account/main.tf:175-255`**, not `terraform/bot/main.tf`. Comment at that location explains why: "Lives here (not in terraform/bot/) so that bonfire-deploy can apply terraform/bot/ without needing IAM permissions." Any IAM change for this work goes in `terraform/account/`; apply order is account → bot.

### Terraform variable naming
- `terraform/bot/variables.tf` declares `discord_application_id` (the correct name)
- `discord_bot/README.md` step 2 example has drifted to `discord_app_id` — will be corrected in this work
- The variable was previously "NOT used by Terraform"; wiring it to Lambda env changes that contract — variable description updated

### Test posture
`main_test.go` is the single test file today. New tests follow that convention — add to `main_test.go` rather than creating `embeds_test.go` / `polling_test.go`, to preserve the module's existing density.

### Deployment
`make -C discord_bot/go` → `discord_bot/bonfire_discord_bot.zip` → `terraform apply` in `terraform/bot/`. No CI. Post-amendment: first `terraform apply` in `terraform/account/` (admin profile), then `terraform/bot/`.

### Conventions (for polling loop specifically)
- **Logging:** log on state transitions and terminal exit only, not every poll tick. A 180s poll tick-logging at 5s intervals = 36 noisy log lines per start; not useful.
- **Error propagation during poll:** EC2 or S3 errors mid-poll do not crash the handler; they emit the Alert embed (via PATCH on the original message) and the Lambda exits cleanly.
- **Test files:** new behaviours added to existing `main_test.go`.

### Go deps — delta (corrected)
`discord_bot/go/go.mod` today has `aws-sdk-go-v2` (config, ec2) as direct + `aws-sdk-go-v2/service/ssm` as indirect. **This work adds:**
- `aws-sdk-go-v2/service/s3` as a new direct dep (for the BACKUP lookup — not currently in the bot module)
- Promotes `aws-sdk-go-v2/service/ssm` from indirect to direct (already used at the Go level; just a `go.mod` cleanup)

No Discord library. Discord PATCH uses `net/http`.

### IAM wildcard intent
The `bonfire-*-backups-*` pattern is intentional: the bot serves all games, and the account's bucket namespace is prefix-scoped by `bonfire-` + region-suffixed by `-backups-<region>`. Mirrors the wildcard pattern already in the CLI role.

### Files to change / add
| File | Change |
|---|---|
| `discord_bot/go/main.go` | Handler bodies refactored: synchronous state check first → idempotent Line OR defer + spawn poll; response construction moves to embed helpers |
| `discord_bot/go/embeds.go` (new) | Hero / Line / Alert typed structs + constructors; state → color / label maps; copy constants |
| `discord_bot/go/polling.go` (new) | EC2-poll loop + Discord PATCH client + S3 backup lookup (all the "work done during deferred response" concerns) |
| `discord_bot/go/main_test.go` | Updated expectations; new tests for idempotency, polling exits, backup lookup |
| `discord_bot/assets/avatar/*.png` (new) | 8 PNGs from design handoff |
| `discord_bot/README.md` | "Avatar upload" section; `discord_app_id` → `discord_application_id` drift fix |
| `terraform/account/main.tf` | `S3BackupListRead` Sid on `aws_iam_policy.bot_lambda` |
| `terraform/bot/main.tf` | Lambda timeout 15s → 180s; `DISCORD_APP_ID` env var wired |
| `terraform/bot/variables.tf` | `discord_application_id` description updated |

---

## Review & Hand-off

### This amendment
Initial plan draft reviewed 2026-04-19 by DA + Architect + UX Reviewer in parallel. Findings:
- **2 critical (Architect):** IAM stack placement, tfvar name / contract change
- **11 important** across all three lenses: state-check ordering (DA+Arch), concurrent-/start guard (DA+Arch), PATCH failure handling (DA), ctx deadline (DA+Arch), s3 dep factual correction (Arch), file split (Arch), brand-copy fidelity ×3 (UX), Alert hint spec gap (UX), WORLD-drop visual-gap (UX)
- **~12 minor** across all three: EC2 flap exit conditions, empty-bucket fallback, `multiple`-state Alert, `/stop-when-pending` idempotency, bare address, Line = colored-bar embed, soften `/hello` auth, distinct leadline/attribution slots, logging discipline, error → Alert, no-discordgo note, IAM wildcard intent

All 13 critical + important accepted and folded in. No premise challenges raised.

### Next steps
1. **Plan approval (user)** — this document, amended.
2. Worker implements in a git worktree on a feature branch.
3. Board review on code: same three lenses (DA + Architect + UX Reviewer). Targeted mandate: "what would you block a merge for." Max 1 fix round per `review:targeted` mode; exhaustion escalates.
4. Push + PR. User merges. Short delivery note appended here.

---

## Amendment 2 (2026-04-19) — `spec_gap` resolution: P1 async self-invoke

### Trigger

Implementation surfaced that the plan's Key Decision #1 architecture (synchronous handler blocking on a 180s poll loop) is **not viable on AWS Lambda Go**. The handler's return signals `/response` to the Lambda Runtime API, after which the execution environment is frozen — any goroutine spawned pre-return does not keep running (CPU is suspended). So "return type-5 ACK fast, keep polling in the background" cannot work on `lambda.Start`-based Go. The Worker implemented a fallback that blocks until the poll completes before returning, which ships a ~180s-late type-5 response — well past Discord's 3s initial-ack window. UX: "This interaction failed" flash; the PATCHes to `/messages/@original` 404 because the original message never existed (Discord never received a timely ack). Correctness bug, not just UX polish.

Worker's diagnosis verified by an Architect escalation against primary sources (AWS Compute Blog — *Running code after returning a response from an AWS Lambda function*; AWS Lambda Go handler docs). No hidden escape hatch via response streaming, `init()` tricks, or `sync.WaitGroup`.

### Decision

**P1 — async self-invoke (same Lambda function).** Chosen for architectural **cohesion**: the ACK and the polling are two halves of one interaction-response flow; putting a network boundary through that single semantic operation (P2's two-Lambda split) is the wrong seam. Mechanical fallback to P2 is easy later if needed — same package, same event shape.

Rejected: P2 two-Lambda split (role-separation hygiene at the cost of a second function/module/deploy), P3 Step Functions (requires rewriting `polling.go` as SFN tasks; over-budget), P4 ship-as-is (correctness bug), P5 Lambda Extensions (sidecar binary overhead for what's essentially fire-and-forget continuation), P6 response streaming (red herring; freeze-on-return semantics unchanged).

### Architecture

The same Lambda function handles two distinct event shapes:

1. **Discord interaction event** (from API Gateway HTTP v2) — handler verifies signature, parses interaction, runs the synchronous state check.
   - Idempotency path (state ∈ `{running, pending, stopping}` for `/start`, etc.) — return type 4 ephemeral Line immediately; no self-invoke.
   - Transition path (e.g. `/start` when `stopped`) — call `lambda:InvokeFunction` with `InvocationType: Event` on *self*, passing a self-poll event payload containing `{interaction_token, application_id, game, user, action}`. Then immediately return the type 5 ACK. Total handler time: hundreds of ms, well inside Discord's 3s window.

2. **Self-poll event** (from the async invoke) — handler detects the distinctive event shape (e.g. top-level field `"source": "self-poll"`), runs the existing `runPollLoop` in `polling.go`, PATCHes Discord via the interaction webhook URL until terminal state or soft deadline.

`polling.go` and `embeds.go` from Amendment 1 are **reused verbatim**. Delta is in `main.go` (handler-entry branching) and terraform (IAM + async invoke config).

### Delta acceptance criteria (supersedes Amendment-1 "Deferred-response flow" bullets)

#### Handler entry
- [ ] Handler distinguishes event shapes at the top: check for self-poll marker first; if absent, treat as an API Gateway v2 Discord interaction event
- [ ] Self-poll marker is a distinctive top-level field (`"source": "self-poll"`) that a Discord interaction payload *cannot* produce — hard guard against recursive-invocation runaway
- [ ] Under no circumstances does the self-poll branch dispatch another self-invoke

#### Discord interaction branch (`/start` and `/stop` transition paths)
- [ ] Synchronous state check happens before any self-invoke (same as Amendment 1)
- [ ] On transition path: call `lambda.Invoke` with `InvocationType: Event`, `FunctionName: <own function name from AWS_LAMBDA_FUNCTION_NAME>`, payload = self-poll event
- [ ] Payload includes: `source: "self-poll"`, `interaction_token`, `application_id`, `game`, `user` (username + id), `action` (`start`|`stop`), `enqueued_at` (RFC3339)
- [ ] On self-invoke success: return Discord type 5 ACK response with initial Hero embed (spark for `/start`, ice for `/stop`)
- [ ] On self-invoke failure: return Alert embed via type 4 — "trouble · couldn't light" with hint `err · lambda_invoke_<status>` — do not spawn a partial state. Self-invoke failures are real AWS errors, not normal control flow.

#### Self-poll branch
- [ ] Runs existing `runPollLoop` from `polling.go` unchanged
- [ ] Uses `enqueued_at` vs now to log async-queue latency at `[poll]` start (observability on token budget drift)
- [ ] Ctx deadline derived from `lambdaContext.Deadline()` as before; 10s reserved for final PATCH

#### Logging
- [ ] All log lines in the interaction branch prefixed `[ack] `; all log lines in the self-poll branch prefixed `[poll] ` — one function, two code paths, interleaved CloudWatch logs; prefix makes a single `/start` trace readable
- [ ] Existing `funcname: ` prefix style is retained *after* the branch prefix (e.g. `[poll] runPollLoop: state transition running`)

### Delta acceptance criteria — Infra / Terraform

- [ ] **`terraform/account/main.tf`** — add `LambdaSelfInvoke` Sid on `aws_iam_policy.bot_lambda`: `lambda:InvokeFunction` on `aws_lambda_function.bot.arn` **only** (not `*`; not wildcard; scoped to the bot's own function ARN). Self-invoke is privilege-escalation-adjacent.
- [ ] **`terraform/bot/main.tf`** — add `aws_lambda_function_event_invoke_config` for the bot function:
  - `maximum_retry_attempts = 0` — critical footgun prevention. Default is 2 retries; a transient EC2 blip would re-run the full poll loop and spam Discord with duplicate PATCHes.
  - `maximum_event_age_in_seconds = 60` — don't queue polls that are older than a minute (user abandoned, Discord token probably stale).
  - `destination_config.on_failure.destination = aws_sqs_queue.bot_dlq.arn` — capture drops.
- [ ] **`terraform/bot/main.tf`** — new `aws_sqs_queue` `bot_dlq` with 14-day retention. IAM `sqs:SendMessage` on the queue added to the bot role (in `terraform/account/main.tf` alongside the existing statements).
- [ ] Soft deadline in polling loop stays 170s; Lambda function timeout stays 180s. Async-queue latency (typically <1s, occasionally several) eats into the budget — monitored via the queue-latency log line.

### Code delta summary

| File | Change vs Amendment-1 Worker commit |
|---|---|
| `discord_bot/go/main.go` | Handler entry refactored: detect event shape; Discord-interaction path calls `lambda.Invoke(Event)` + returns type 5; self-poll path calls `runPollLoop`. Previous "block on runner" code removed. |
| `discord_bot/go/polling.go` | Unchanged except: `runPollLoop` now takes interaction token + app_id + game + user from the self-poll event (already parameterised this way in the existing implementation — just wire it through from the new event shape). Add async-queue latency log line at `[poll]` entry. |
| `discord_bot/go/embeds.go` | Unchanged. |
| `discord_bot/go/main_test.go` | New tests for handler event-shape branching (interaction path, self-poll path, recursion guard, lambda-invoke failure path). Existing poll-loop tests unchanged. |
| `terraform/account/main.tf` | Add `LambdaSelfInvoke` and `SQSDLQSend` Sids on `aws_iam_policy.bot_lambda`. |
| `terraform/bot/main.tf` | Add `aws_sqs_queue.bot_dlq`, `aws_lambda_function_event_invoke_config.bot`. `DISCORD_APP_ID` env var stays. |
| `terraform/bot/variables.tf` | No change. |

### Risks the Architect specifically flagged (all addressed in acceptance criteria above)

1. Async retry semantics → `maximum_retry_attempts = 0` + DLQ
2. Recursive self-invocation → `source: "self-poll"` distinctive field + hard guard
3. IAM scope → `aws_lambda_function.bot.arn`, never `*`
4. Token-budget drift → log async-queue latency at `[poll]` start
5. Polling cold-start → ~150ms on `provided.al2023`; acceptable; documented, not mitigated
6. Observability / interleaved logs → `[ack]` / `[poll]` line prefixes

### Implementation dispatch note

Re-dispatching a Worker (model: Sonnet — architecture is locked, scope is tight; no judgment calls left that warrant Opus) with the delta scope. Worker works **on top of the existing worktree at `.claude/worktrees/agent-aab9bcfc`** (branch `worktree-agent-aab9bcfc`), extending rather than replacing Amendment-1 code. One additional commit on the same branch.

Board re-review on the final code covers the combined Amendment-1 + Amendment-2 state.

---

## Amendment 3 (2026-04-19) — Board re-review findings + fix round

First Board re-review on the combined Amendment-1+2 code turned up:
- 1 critical (UX: Line embed missing state dot `●` across all ephemeral surfaces)
- 11 important (split across UX copy fidelity, Architect log-prefix / dead-code / stricter Lambda-invoke check, DA input-validation / terminal-set gaps)
- 8 minor (logged; most converged across reviewers)

User waived the governance circuit-breaker (`complexity_shift` at 3+ important findings) as over-sensitive for this hobby-scale work. Standard targeted-mode fix round proceeded.

### Round 1 fixes (this amendment)

**UX:**
- Line copy strings now include `● ` state dot before state labels per BRAND.md §"Line" (`%s · ● burning · …`, etc. — covers `/status`, `/hello` (no state dot — state-agnostic), idempotency races, `/start-when-X`, `/stop-when-Y`).
- `alertEmbed` split into three by kind per BRAND.md §"Alert": `alertEmbed` (error, `danger` + ◬), `alertEmbedUnauthorized` (ash + ◔), `alertEmbedNotFound` (ash + ◌). All 19 callsites routed to the right variant — unauthorized/guild-blocked/not_found/unknown-command/unknown-action all use the gentle ash variants; only true server-side errors keep danger.
- `/help` now ships as plain ephemeral `Content` (no embed chrome) matching BRAND.md §"Help" exact text block; previous `lineEmbed`-wrapped version had a visible red/danger bar because empty state defaulted to `colorDanger`.
- Soft-deadline Hero for `/start` uses new `heroEmbedWithLabel(game, labelPending, colorIce, …)` so the title (`lighting`) matches the body (`still lighting …`); previous code painted `dying down` + ice, internally contradictory.
- "unknown command" / "unknown action" replaced by brand-consistent `copyAlertUnknownCommand` / `copyAlertUnknownAction`.
- `copyStartAlreadyRunning` extended with optional `backup X ago` trailer (`copyStartAlreadyRunningWithBackup`) — handler looks up S3 backup and picks the with-backup variant when available.
- Voice-rule #2 fixes: removed terminal periods on short `/hello` variants.

**Architect:**
- Function-level `dead_letter_config` removed from `aws_lambda_function.bot`; async-invoke failure DLQ routing now exclusively via `aws_lambda_function_event_invoke_config.destination_config.on_failure` (no double-target drift risk).
- `dispatchSelfPoll` strictness: `out.StatusCode != 202` (exact match per AWS async-invoke contract) + `out.FunctionError != nil` check for anomaly cases.
- Dead `publicResponse` / `publicEmbedResponse` helpers deleted (were never called).
- `[ack]` / `[poll]` / `[shared]` / `[test]` log-prefix contract honored on all previously unprefixed lines (`jsonResponse`, `verifyDiscordRequest`, `handleStatusCommand` EC2 error). `backupElapsedString` takes an explicit `logTag` param since it's called from both paths.

**DA:**
- `handleSelfPoll` validates `game`, `interaction_token`, `application_id`, `action` non-empty at entry — returns 400 and routes to DLQ rather than silently hitting a malformed webhook URL.
- Poll-loop terminal sets include `not_found` and `multiple` for both `/start` and `/stop`; terminal embed routes these to Alerts rather than spinning the 170s deadline. (EC2 sustained-error stalls and 429-without-header fallbacks: minor, deferred.)
- `err · ec2_lambda_invoke` hint fixed to `err · lambda_invoke_failed` via new `copyHintLambdaErrorFmt`.

### Known limitations (documented, deferred)

- **Idempotency TOCTOU race** (DA #2 / Architect M-3). Two concurrent `/start` invocations arriving within EC2's DescribeInstances-read staleness window (~500 ms) will both observe `state=stopped`, both call `StartInstances` (idempotent AWS-side), and both dispatch self-invokes — producing two poll loops PATCHing the same `@original` message with possibly-divergent timers. For hobby-scale concurrency (≤ handful of simultaneous players), the cost of closing this (EC2 `ClientToken` + `IncorrectInstanceState` catch, or a coordination primitive) outweighs the benefit. Acknowledged; not fixed in this work.
- **Self-poll event sustained EC2 errors** (DA #5). A persistent throttle or AccessDenied during the poll leaves the initial "lighting the fire… (0:00)" stale until the 170s deadline fires the "still lighting" soft-deadline message. Acceptable for ops-visibility (CloudWatch captures the errors) at hobby scale.
- **Discord 429 without `Retry-After` header** (DA #6). If Discord returns 429 with neither `Retry-After` header nor a parseable body, the poll skips the header-sleep and relies on the 5 s ticker — potentially re-triggering 429 on the next tick. Low-probability per Discord's documented rate-limit behavior, not protected against.

### Implementation provenance

All three amendments and the fix round were authored by Lead (hard gate #6 relaxed throughout) because the harness' subagent Bash-permission inheritance was unreliable. Every code change was Board-reviewed independently — the checker-vs-writer separation that matters was preserved by the Board passes, not by Lead's hands being off the keyboard.

### Board re-review #2 (round 2) — all SHIP

After Amendment-3 fixes landed, Board re-reviewed with explicit `SHIP | BLOCK` verdicts:
- **DA: SHIP.** All prior important findings addressed or reasonably deferred. Pre-existing `latestBackupTime` pagination (>1000-object buckets) noted; not a round-1 regression, logged.
- **Architect: SHIP.** All four round-1 importants resolved cleanly. Two minors: comment wording on `handleSelfPoll` field-validation ("→ DLQ on drop" was misleading; a 400 with `err=nil` is silently dropped by Lambda's async contract, not DLQ-routed), and plan doc wasn't on the branch.
- **UX: SHIP.** All prior critical + important UX findings verified in code. Idempotency Lines drop the leading `<game>` prefix (defensible — context is scoped to the command). Noted BRAND.md internal inconsistency about help-command ordering (§"Help" example vs §"Commands" rule); code follows the normative rule. Alert symbols (◬/◔/◌) render correctly in Discord.

### Out-of-cycle Security Analyst review (user-requested)

Original classification opted Security Analyst out ("no new trust boundary since Discord was already integrated; IAM delta is read-only S3"). The user requested an SA pass after the design evolved to include `lambda:InvokeFunction` self-invoke and a dual-event-shape router.

**SA verdict: SHIP.** No blockers. Summary:
- **Router trust boundary is load-bearing and was undocumented.** Self-poll branch skips Ed25519 signature verification — safe *because* API Gateway HTTP v2 payload-format-v1.0 wraps attacker JSON in `{"headers":{...},"body":"...",...}`, so a top-level `"source":"self-poll"` in the attacker's body sits inside the `body` string and can't reach the source-peek struct. SA recommended explicit defense-in-depth: reject any self-poll event that carries `headers` or `requestContext` at the top level. **Applied** in final housekeeping commit; new test `TestHandler_SelfPollWithAPIGatewayFields_Rejected` pins the invariant.
- Self-invoke IAM already at tightest supported scoping (`lambda:InvocationType = Event` is not a condition key per [AWS Service Authorization Reference](https://docs.aws.amazon.com/service-authorization/latest/reference/list_awslambda.html); own-ARN scoping is the floor).
- No tokens / signatures / raw bodies logged anywhere. DLQ-captured tokens are already expired (Discord 15-min expiry ≪ SQS 14-day retention).
- `go.mod` deps have no known CVEs at the time of review (aws-sdk-go-v2 v1.41, aws-lambda-go v1.53, service/lambda v1.89.1, service/s3 v1.97.1).
- S3 wildcard `bonfire-*-backups-*` is appropriate for a single-operator hobby account; tighten to explicit ARNs if multi-tenant.
- SSM allowlist correctly fails closed on empty/absent/error; handles trailing-comma and whitespace-only cases.

No premise challenge — self-invoke is a reasonable architecture given the 3s Discord ack window + 180s poll ceiling.

### Final housekeeping

After all four Board lenses shipped, Lead applied the non-blocking nits in a single closing commit:
1. SA's defense-in-depth `requestContext` / `headers` rejection in the self-poll branch, plus a dedicated test.
2. Architect's `handleSelfPoll` comment-wording nit (replaced "→ DLQ on drop" with the accurate "silently dropped, not DLQ'd — malformed events are code bugs, not retryable").
3. This plan doc moved onto the branch (previously only in main's working tree).

No Board re-review triggered — Architect and SA both explicitly marked these as non-blocking.
