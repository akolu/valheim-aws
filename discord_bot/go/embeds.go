package main

import (
	"fmt"
	"math"
	"time"
)

// Brand palette — hex-encoded as decimal ints for Discord embed.color.
// Palette spec: BRAND.md §"Palette" (v1.5).
const (
	colorEmber  = 0xe8793a // lit
	colorSpark  = 0xf2c14e // lighting / relighting
	colorIce    = 0x6b8f9c // cooling / dying down / still lighting
	colorAsh    = 0x9a8e7d // out / banked coals
	colorDanger = 0xc65d4a // errors, unauthorized, not_found, multiple
	colorOk     = 0x8fb369 // healthy neutral info (hello, help)
)

// Brand-voice copy constants. Lowercase, first-person, fire-metaphor per brand §07.
// Grouped by consumer so the review surface is scannable.
//
// Voice rules (BRAND.md §"Voice rules"):
//   1. lowercase default
//   2. no terminal period on short statements
//   3. no exclamation marks
//   4. no emoji as decoration (● is a state-pill glyph, not an emoji)
const (
	// Idempotent Line copy (type 4, flags 64) — see plan § Idempotency.
	// Each includes the `● <label>` state-pill dot per brand Line format.
	copyStartAlreadyRunning    = "fire's already ● burning · lit %s ago · %s"
	copyStartAlreadyRunningWithBackup = "fire's already ● burning · lit %s ago · %s · backup %s ago"
	copyStartAlreadyLighting   = "fire's already ● lighting · started %s ago · ~2 min total"
	copyStartWhileStopping     = "hang on — someone's ● banking the coals · try again in a minute"
	copyStopAlreadyOut         = "fire's already ● out · last burned %s ago"
	copyStopAlreadyOutNever    = "fire's already ● out · never burned"
	copyStopAlreadyDyingDown   = "fire's already ● dying down · saving the world"
	copyStopWhilePending       = "can't bank coals yet · fire's still ● lighting · try again in a minute"

	// Polling in-flight / terminal Hero bodies.
	copyLightingBody         = "lighting the fire… (%s)"
	copyStartInterruptedBody = "someone banked the coals · fire's out"
	copyStartDeadlineBody    = "still lighting — i'll keep an eye on it"
	copyStoppingBody         = "saving the world, banking the coals"
	copyStopInterruptedBody  = "fire's relighting · someone wasn't done"
	copyStopDeadlineBody     = "still dying down — i'll keep an eye on it"

	// Hero footer text — BRAND.md v1.5 treats attribution as metadata (footer),
	// not headline. Use with the embed's native timestamp for "lit by @X · just now".
	// Caller passes a bare "@username" (or fallback) — no format string escapes.
	copyFooterLitBy    = "lit by %s"
	copyFooterPutOutBy = "put out by %s"

	// /status Line one-liners — `<Game> · ● <label> · ...` per BRAND.md §"Line".
	copyStatusRunning         = "%s · ● burning · %s · %s · backup %s ago"
	copyStatusRunningNoBackup = "%s · ● burning · %s · %s · never burned"
	copyStatusPending         = "%s · ● lighting · started %s ago · ~2 min total"
	copyStatusStopping        = "%s · ● dying down · saving the world"
	copyStatusStopped         = "%s · ● out · last burned %s ago"
	copyStatusStoppedNever    = "%s · ● out · never burned"

	// /hello Line. No terminal period per voice rule #2.
	copyHelloKeeper  = "here · ready. you're on the keeper list for this fire"
	copyHelloVisitor = "here · ready. you can watch; ask a keeper for access"

	// /help — plain text block per BRAND.md §"Help (plain text, ephemeral)".
	// No embed chrome, mono text. %s is the game name substituted once per line.
	copyHelpBlock = "bonfire · keeper here.\n\n/%s status    check on it\n/%s start     light the fire\n/%s stop      put it out\n/%s help      this\n\ntip: anyone in the channel can start or stop — it's a shared fire"

	// Alert headlines / hints.
	copyAlertNoSuchFire           = "no such fire"
	copyAlertUnknownCommand       = "no command by that name"
	copyAlertUnknownAction        = "no action by that name"
	copyAlertTwoFires             = "two fires by that name — ask a keeper"
	copyAlertSomethingSideways    = "something went sideways"
	copyAlertUnauthorizedHeadline = "you can't tend this fire"
	copyAlertUnauthorizedBody     = "ask a keeper if you'd like access"
	copyAlertGuildBlocked         = "this bot isn't keeping a fire in this server"

	// Alert hints (monospace footer).
	copyHintTagCollision     = "err · ec2_tag_collision"
	copyHintRoleMissing      = "err · discord_role_missing"
	copyHintEC2ErrorFmt      = "err · ec2_%s"
	copyHintS3ErrorFmt       = "err · s3_%s"
	copyHintLambdaErrorFmt   = "err · lambda_%s"
	copyHintTryStatusFmt     = "try · /%s status"

	// Alert symbols per BRAND.md §"Alert": distinct per kind so refusals
	// don't visually overlap with errors.
	alertSymbolError        = "◬"
	alertSymbolUnauthorized = "◔"
	alertSymbolNotFound     = "◌"

	// State labels (pill text) per BRAND.md §"State vocabulary" v1.5.
	// `running` is "lit" (rename introduced in BRAND v1.4 per changelog —
	// "burning" carried ops/incident baggage: "prod is burning"; applied
	// in code as part of the v1.5 Hero-embed restructure).
	labelRunning  = "lit"
	labelPending  = "lighting"
	labelStopping = "dying down"
	labelStopped  = "out"
	labelUnknown  = "unknown"

	// Field names — running Hero grid.
	fieldAddress = "address"
	fieldUptime  = "uptime"
	fieldBackup  = "backup"
)

// Discord embed types — minimal subset needed for webhook PATCH bodies.
// https://discord.com/developers/docs/resources/channel#embed-object
//
// Timestamp (ISO8601) renders natively in Discord as relative time next to
// the footer ("Footer text · just now" / "· 2m ago" / etc.), per BRAND.md
// v1.5 Hero-attribution metadata treatment.
type Embed struct {
	Title       string       `json:"title,omitempty"`
	Description string       `json:"description,omitempty"`
	Color       int          `json:"color,omitempty"`
	Fields      []EmbedField `json:"fields,omitempty"`
	Footer      *EmbedFooter `json:"footer,omitempty"`
	Timestamp   string       `json:"timestamp,omitempty"`
}

type EmbedField struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Inline bool   `json:"inline,omitempty"`
}

type EmbedFooter struct {
	Text string `json:"text"`
}

// WebhookEditBody is the payload for `PATCH /webhooks/{app_id}/{token}/messages/@original`.
// Content is the plain-text portion (usually empty — we use embeds).
// Flags is only honored on send, not on edit — kept here for symmetry only.
type WebhookEditBody struct {
	Content string  `json:"content,omitempty"`
	Embeds  []Embed `json:"embeds,omitempty"`
}

// stateColor maps an EC2 state string to the brand palette color.
func stateColor(state string) int {
	switch state {
	case "running":
		return colorEmber
	case "pending":
		return colorSpark
	case "stopping":
		return colorIce
	case "stopped":
		return colorAsh
	default:
		return colorDanger
	}
}

// stateLabel maps an EC2 state string to the brand pill label.
func stateLabel(state string) string {
	switch state {
	case "running":
		return labelRunning
	case "pending":
		return labelPending
	case "stopping":
		return labelStopping
	case "stopped":
		return labelStopped
	default:
		return labelUnknown
	}
}

// heroTitle composes the Hero embed title: "<game> · <pill>".
// Kept here so copy changes have a single home.
func heroTitle(game, state string) string {
	return fmt.Sprintf("%s · %s", game, stateLabel(state))
}

// heroEmbed builds a branded Hero embed (public, used for /start and /stop edits).
// `body` is the description text — typically a narrative line like "lighting
// the fire… (5s)" or "fire's out". Attribution is NOT part of the description
// in BRAND.md v1.5; use withFooter to add a footer-line attribution.
func heroEmbed(game, state, body string) Embed {
	return Embed{
		Title:       heroTitle(game, state),
		Description: body,
		Color:       stateColor(state),
	}
}

// heroEmbedWithLabel lets the caller override both label and color — used for
// the /start soft-deadline Hero (ice color + "lighting" label). Without this
// escape hatch, we'd paint "dying down" on a still-lighting fire, which is
// internally contradictory (the body says "still lighting…").
func heroEmbedWithLabel(game, label string, color int, body string) Embed {
	return Embed{
		Title:       fmt.Sprintf("%s · %s", game, label),
		Description: body,
		Color:       color,
	}
}

// heroEmbedRunning builds the 3-field running-state Hero: ADDRESS full-width top row,
// UPTIME + BACKUP side-by-side bottom row (WORLD omitted — see Key Decision #2).
// backup is the raw elapsed string ("15m") — this builder appends " ago" for
// display, consistent with copy strings that read "backup %s ago". Empty backup
// omits the field entirely (empty-bucket fallback).
// No description — attribution lives in the footer (set via withFooter by the caller).
func heroEmbedRunning(game, addr, uptime, backup string) Embed {
	fields := []EmbedField{
		{Name: fieldAddress, Value: "`" + addr + "`", Inline: false},
		{Name: fieldUptime, Value: uptime, Inline: true},
	}
	if backup != "" {
		fields = append(fields, EmbedField{Name: fieldBackup, Value: backup + " ago", Inline: true})
	}
	return Embed{
		Title:  heroTitle(game, "running"),
		Color:  colorEmber,
		Fields: fields,
	}
}

// withFooter attaches a footer text + timestamp to an embed. Empty footerText
// leaves the embed unchanged (no footer slot, no timestamp). Discord renders
// footer text next to relative timestamp ("lit by @X · just now").
func withFooter(e Embed, footerText string, ts time.Time) Embed {
	if footerText == "" {
		return e
	}
	e.Footer = &EmbedFooter{Text: footerText}
	if !ts.IsZero() {
		e.Timestamp = ts.UTC().Format(time.RFC3339)
	}
	return e
}

// userLabel formats a footer-safe user reference. Discord mention syntax
// (`<@ID>`) does NOT resolve in footer text — it'd render literally. So we use
// plain "@username" when available, "someone" as a last-resort fallback.
func userLabel(username string) string {
	if username == "" {
		return "someone"
	}
	return "@" + username
}

// lineEmbed builds a Line (ephemeral) embed: just a colored left bar and a description.
// Used for /status, /hello, /help, and all idempotency races — so the brand's state-color
// still lands via the bar even though the text is plain.
func lineEmbed(state, body string) Embed {
	return Embed{
		Description: body,
		Color:       stateColor(state),
	}
}

// alertEmbed builds an Alert (ephemeral) embed for the `error` kind:
// danger-colored bar, ◬ symbol, two-line headline+body, monospace hint footer.
// Use this for infra errors and server-side failures.
// For refusals (unauthorized / not_found), use the ash-colored variants below —
// the brand's friendly-refusal intent (BRAND.md §"Alert") is lost if every
// alert paints red.
func alertEmbed(headline, body, hint string) Embed {
	return Embed{
		Title:       alertSymbolError + " " + headline,
		Description: body,
		Color:       colorDanger,
		Footer:      &EmbedFooter{Text: hint},
	}
}

// alertEmbedUnauthorized builds an Alert for the `unauthorized` kind:
// ash-colored bar (gentle refusal), ◔ symbol.
func alertEmbedUnauthorized(headline, body, hint string) Embed {
	return Embed{
		Title:       alertSymbolUnauthorized + " " + headline,
		Description: body,
		Color:       colorAsh,
		Footer:      &EmbedFooter{Text: hint},
	}
}

// alertEmbedNotFound builds an Alert for the `not_found` kind:
// ash-colored bar (gentle redirect), ◌ symbol.
// Used for missing games, unknown commands, unknown actions — none are user
// failures, just gentle "try one of these instead" responses.
func alertEmbedNotFound(headline, body, hint string) Embed {
	return Embed{
		Title:       alertSymbolNotFound + " " + headline,
		Description: body,
		Color:       colorAsh,
		Footer:      &EmbedFooter{Text: hint},
	}
}

// --- Response helpers ---

// ephemeralEmbedResponse returns an immediate type 4 response with embeds, flags=64.
// Used when the handler's synchronous state check lets it short-circuit (idempotency, /status, /hello, Alerts).
func ephemeralEmbedResponse(embed Embed) InteractionResponse {
	return InteractionResponse{
		Type: discordChannelMessage,
		Data: &InteractionResponseData{
			Flags:  discordEphemeralFlag,
			Embeds: []Embed{embed},
		},
	}
}

// deferredResponse returns type 5 (DeferredChannelMessageWithSource) — public.
// Discord shows "Bot is thinking…" until the handler PATCHes the original message.
func deferredResponse() InteractionResponse {
	return InteractionResponse{
		Type: discordDeferredChannelMessage,
	}
}

// --- Elapsed / uptime formatting ---

// formatElapsed renders a duration as a human-friendly, low-ceremony string:
//   < 60s       → "12s"
//   < 60m       → "4m"
//   < 24h       → "1h 42m"
//   ≥ 24h       → "3d 2h"
//
// Deliberately lossy — this is chat UI, not telemetry.
func formatElapsed(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	sec := int(math.Round(d.Seconds()))
	if sec < 60 {
		return fmt.Sprintf("%ds", sec)
	}
	minTotal := sec / 60
	if minTotal < 60 {
		return fmt.Sprintf("%dm", minTotal)
	}
	hours := minTotal / 60
	mins := minTotal % 60
	if hours < 24 {
		if mins == 0 {
			return fmt.Sprintf("%dh", hours)
		}
		return fmt.Sprintf("%dh %dm", hours, mins)
	}
	days := hours / 24
	h := hours % 24
	if h == 0 {
		return fmt.Sprintf("%dd", days)
	}
	return fmt.Sprintf("%dd %dh", days, h)
}

// elapsedSince returns the duration since t, or 0 if t is nil.
func elapsedSince(t *time.Time) time.Duration {
	if t == nil {
		return 0
	}
	return time.Since(*t)
}
