# Bonfire — Brand Reference

> This is the distilled brand reference for Bonfire. Every user-facing string, color, embed layout, and command description should be traceable back to this file. The full visual brand book lives at `docs/brand/Bonfire Brand Book.html` — open it in a browser when you want to see embeds rendered, avatars at size, or the palette in context. This file is the structured text companion; when in doubt, the book is the source of truth.

---

## The one-line positioning

Bonfire is a **keeper of fires** — a quiet, warm, competent bot that lights and tends game servers on Discord. It does one thing well. It doesn't chatter. It speaks in the register of a friend who takes care of the firepit, not a cloud platform dashboard.

## Voice rules (non-negotiable)

1. **Lowercase default.** Bot copy is lowercase unless a proper noun demands otherwise (`Valheim`, `Midgard`, `@otso`). This applies to commands, embed headlines, error messages, everything.
2. **No period on short statements.** `the fire is burning` — not `the fire is burning.` Periods only on multi-sentence copy.
3. **Personify the fire, not the bot.** Say `the fire is burning` or `bonfire's out`, not `I have started the server` or `the bot is online`. The bot is the keeper; the server is the fire.
4. **Trust the reader.** No "please wait" / "loading..." / "Hello!" — strip politeness rituals. Warmth comes from word choice, not manners.
5. **No exclamation marks. Ever.**
6. **No emoji as decoration.** The only emoji in brand surface is 🔥 in the `bonfire lit` state headline — and even that's optional. Never 🎉, 🚀, 🤖, ✅, ❌.

## Say / Don't say (lexicon)

| Concept   | Say                                 | Don't say                    |
| --------- | ----------------------------------- | ---------------------------- |
| start     | light the fire · light · kindle     | provision · spin up · boot   |
| stop      | put it out · bank · let it go out   | terminate · kill · shut down |
| running   | burning · the fire is on            | UP · ONLINE · active         |
| starting  | lighting · tending · coaxing        | BOOTING · INITIALIZING       |
| stopped   | out · dark · cold                   | OFFLINE · DOWN · dead        |
| latency   | takes a minute · patience           | please wait · loading...     |
| error     | trouble · sideways · didn't take    | ERROR · FAILURE · EXCEPTION  |
| invoker   | lit by @x · tended by @x            | created by · owner           |
| backup    | last backup · last coal saved       | snapshot · AMI               |
| server    | the fire · the server (when plain)  | the instance · EC2 · host    |
| user-role | keeper · tender · whoever has roles | admin · superuser            |
| greeting  | here · ready · here it is           | Hello! I am your assistant!  |

## State vocabulary (canonical)

These are the only states a fire can be in. Don't invent new ones. Map any AWS EC2 state to one of these before rendering.

| State      | Label      | Copy shape                                     |
| ---------- | ---------- | ---------------------------------------------- |
| `running`  | burning    | `burning for 1h 42m · lit by @otso`            |
| `starting` | lighting   | `tending to it · lit 34s ago · usually ~2 min` |
| `stopping` | dying down | `saving the world, banking the coals`          |
| `stopped`  | out        | `last burned 2h ago`                           |
| `error`    | trouble    | `couldn't light the fire`                      |

`pending` → `starting`. `stopping` / `shutting-down` → `stopping`. `terminated` → `stopped`. Anything unexpected → `error`.

---

## Palette

All colors from `canvas/tokens.jsx`. Do not invent new colors; if you need a new state color, mix within the existing chroma band (OKLab 0.12–0.15).

### Core

| Token    | Hex       | Use                                              |
| -------- | --------- | ------------------------------------------------ |
| `ink`    | `#1a1612` | Default embed background, bot avatar background  |
| `dark`   | `#2a221b` | Card background, raised surfaces                 |
| `panel`  | `#3a2f25` | Secondary raised panel                           |
| `border` | `#4a3d30` | Hairline borders                                 |
| `cream`  | `#f0e7d6` | Primary text on dark                             |
| `paper`  | `#e8dfc9` | Off-white (rarely used — inverted surfaces only) |
| `muted`  | `#8a7d6d` | Secondary text                                   |
| `dim`    | `#6b5f50` | Tertiary text, field labels                      |

### State

| Token      | Hex       | State                                    |
| ---------- | --------- | ---------------------------------------- |
| `ember`    | `#e8793a` | `running` — warm orange, the brand color |
| `emberHot` | `#f5a862` | `running` glow accent                    |
| `spark`    | `#f2c14e` | `starting` — kindling yellow             |
| `ice`      | `#6b8f9c` | `stopping` — cooling blue-gray           |
| `ash`      | `#9a8e7d` | `stopped` — cool neutral                 |
| `ok`       | `#8fb369` | success sage                             |
| `danger`   | `#c65d4a` | error / unauthorized                     |

---

## Type

| Role    | Family             | Weights         | Used for                                         |
| ------- | ------------------ | --------------- | ------------------------------------------------ |
| Display | Cormorant Garamond | italic 500      | Headlines, game names in embeds, the wordmark    |
| Body    | Inter Tight        | 400 / 500 / 600 | Body copy, status lines, labels                  |
| Data    | JetBrains Mono     | 400 / 500       | IPs, timestamps, uptimes, state codes, log hints |

**Minimum sizes:** body 11px · label 10px · mono 10px. Below that, text reads as decoration — don't.

---

## Embed surfaces

Four layouts, one plain-text surface. Map commands to surfaces deterministically:

| Command                 | Surface   | Visibility | Why                                          |
| ----------------------- | --------- | ---------- | -------------------------------------------- |
| `/<game> start`         | **Hero**  | public     | Broadcast — the channel wants to know        |
| `/<game> stop`          | **Hero**  | public     | Broadcast — same                             |
| `/<game> status`        | **Line**  | ephemeral  | Personal check-in, don't clutter the channel |
| `/<game> help`          | **Help**  | ephemeral  | Plain text, no embed chrome                  |
| _authorization failure_ | **Alert** | ephemeral  | Gentle refusal, not punishment               |
| _not_found_             | **Alert** | ephemeral  | Same                                         |
| _idempotency race_      | **Line**  | ephemeral  | "it's already burning, here's the address"   |
| _AWS / Discord errors_  | **Alert** | ephemeral  | Error class                                  |

### Hero (broadcast)

Large typographic embed. Full palette of fields when `running`: **ADDRESS · UPTIME · BACKUP**. No WORLD field — it's decorative, rots on rename, deferred. Reference implementation: `canvas/embeds_v2.jsx::HeroEmbed`.

Structure:

- 3px top accent strip (state color)
- Leadline (mono, 10px, all caps): `${invoker} lit the fire`
- Game name (Cormorant Garamond italic 44px) + state pill inline
- Soft radial ember glow bottom-right at 0.18 opacity when running/starting, 0.06 otherwise
- Field grid (only when running) — 2 columns, ADDRESS accented in ember mono

### Card (balanced default)

Compact embed with left 4px accent strip + field list. Same fields as Hero (ADDRESS · UPTIME · BACKUP). Use when Hero feels too grand and Line feels too terse. Reference: `canvas/embeds_v2.jsx::CardEmbed`.

### Line (ephemeral, minimal)

Single row, all mono, state dot + label inline. Use for `/status` and idempotent races. Reference: `canvas/embeds_v2.jsx::LineEmbed`.

Format:

```
Valheim · ● burning · 13.48.12.34:2456 · 1h 42m · backup 15m ago
```

### Alert (errors, refusals)

Left 4px accent (`danger` for errors, `ash` for refusals). Symbol + italic headline + body + mono hint. Reference: `canvas/embeds_v2.jsx::AlertEmbed`.

Three kinds:

- `error` — `◬` symbol, `danger` color, "something went sideways"
- `unauthorized` — `◔` symbol, `ash` color, "you can't tend this fire"
- `not_found` — `◌` symbol, `ash` color, "no such fire"

### Help (plain text, ephemeral)

**No embed chrome.** Mono text block. Commands listed in order of frequency (status first). No capitalization, no terminal period.

```
bonfire · keeper here.

/valheim start    light the fire
/valheim stop     put it out
/valheim status   check on it
/valheim help     this

tip: anyone in the channel can start or stop — it's a shared fire.
```

---

## Commands

### Slash-menu descriptions

Descriptions appear in Discord's autocomplete — first brand surface a new player sees, before any embed. Write them in voice.

| Command          | Description (what Discord shows) |
| ---------------- | -------------------------------- |
| `/<game> status` | `check on the fire`              |
| `/<game> start`  | `light the fire`                 |
| `/<game> stop`   | `put it out`                     |
| `/<game> help`   | `what you can ask me`            |

Rules:

- No capitalization, no terminal period
- Read as what the bot _offers_, not what the command _does_
- Order subcommands by frequency (`status` → `start` → `stop` → `help`)

### Registration model

Commands are registered **per-guild, not globally**. `/bonfire/allowed_guilds` SSM is the source of truth; the CLI is the enforcement point.

- Adding a guild to the allowlist triggers `PUT /applications/{app}/guilds/{g}/commands` registering only the games that guild is authorized for.
- Removing triggers `DELETE`.
- A guild allowlisted for only Valheim sees only `/valheim` in its slash menu. No other games clutter the autocomplete.
- Instant updates (guild commands propagate within seconds, unlike global's ~1 hour).

CLI shape:

```
bonfire guild authorize <guild_id> valheim
  → adds to /bonfire/allowed_guilds
  → PUT /applications/{app}/guilds/{g}/commands
  → /valheim appears in that guild's slash menu
```

---

## Avatar

Two-state system. Both at 128/256/512/1024 in `docs/brand/avatar/`.

- `bonfire-lit-*.png` — canonical state, campfire with flame. Default Discord bot avatar.
- `bonfire-unlit-*.png` — dormant state, logs + faint ember. Optional: swap via Discord's bot-avatar API as a state signal (if implementing: only when _all_ fires in a guild are stopped).

Upload `bonfire-lit-512.png` to Discord's bot profile. 1024 is acceptable if you want crispness at larger sizes.

---

## Things that are intentionally not here

- **Player count** — not surfaced today; adding it requires game-specific queries (Valheim RCON, Factorio RCON, Satisfactory has no native player count). Deferred.
- **World name** — decorative, rots on rename, not worth the SSM parameter. Deferred.
- **ETA for cold start** — bot says flat "~2 min". No real signal to improve on this without EC2 status-check polling, which isn't worth the Lambda time.
- **Stop confirmation** — single-tap stop today. Adding an "are you sure" flow is in `docs/brand/Design Backlog.html` (item 06).

When you find yourself wanting to add one of these, check the Design Backlog first — we decided against each for a reason.

---

## Versioning

This file mirrors the brand book version. Currently **v1.3**. Brand book is at the same version.

When editing the brand — a new state, a new surface, a voice-rule change — edit both this file and the HTML book, and bump the version in both places. Never let them drift.
