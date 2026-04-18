package main

import (
	"fmt"
	"math"
	"time"
)

// Brand palette — hex-encoded as decimal ints for Discord embed.color.
// Palette spec: brand book §02 (Bonfire Brand Book v1, April 2026).
const (
	colorEmber  = 0xe8793a // burning / lit
	colorSpark  = 0xf2c14e // lighting / relighting
	colorIce    = 0x6b8f9c // cooling / dying down / still lighting
	colorAsh    = 0x9a8e7d // out / banked coals
	colorDanger = 0xc65d4a // errors, unauthorized, not_found, multiple
	colorOk     = 0x8fb369 // healthy neutral info (hello, help)
)

// Brand-voice copy constants. Lowercase, first-person, fire-metaphor per brand §07.
// Grouped by consumer so the review surface is scannable.
const (
	// Idempotent Line copy (type 4, flags 64) — see plan § Idempotency.
	copyStartAlreadyRunning  = "fire's already burning · lit %s ago · %s"
	copyStartAlreadyLighting = "fire's already lighting · started %s ago · ~2 min total"
	copyStartWhileStopping   = "hang on — someone's banking the coals · try again in a minute"
	copyStopAlreadyOut       = "fire's already out · last burned %s ago"
	copyStopAlreadyOutNever  = "fire's already out · never burned"
	copyStopAlreadyDyingDown = "fire's already dying down · saving the world"
	copyStopWhilePending     = "can't bank coals yet · fire's still lighting · try again in a minute"

	// Polling in-flight / terminal Hero bodies.
	copyLightingBody        = "lighting the fire… (%s)"
	copyStartInterruptedBody = "someone banked the coals · fire's out"
	copyStartDeadlineBody   = "still lighting — i'll keep an eye on it"
	copyStoppingBody        = "saving the world, banking the coals"
	copyStopInterruptedBody = "fire's relighting · someone wasn't done"
	copyStopDeadlineBody    = "still dying down — i'll keep an eye on it"

	// Hero leadlines / attributions.
	copyLeadlineLit      = "<@%s> lit the fire"
	copyLeadlinePutOut   = "put out by <@%s>"
	copyAttributionLitBy = "lit by <@%s>"

	// /status Line one-liners.
	copyStatusRunning   = "%s · burning · %s · %s · backup %s ago"
	copyStatusRunningNoBackup = "%s · burning · %s · %s · never burned"
	copyStatusPending   = "%s · lighting · started %s ago · ~2 min total"
	copyStatusStopping  = "%s · dying down · saving the world"
	copyStatusStopped   = "%s · out · last burned %s ago"
	copyStatusStoppedNever = "%s · out · never burned"

	// /hello Line.
	copyHelloKeeper   = "here · ready. you're on the keeper list for this fire."
	copyHelloVisitor  = "here · ready. you can watch; ask a keeper for access."

	// /help Line.
	copyHelpHeader  = "_%s commands_"
	copyHelpStatus  = "`/%s status` — check on the fire"
	copyHelpStart   = "`/%s start` — light the fire"
	copyHelpStop    = "`/%s stop` — bank the coals"
	copyHelpHello   = "`/%s hello` — say hi"
	copyHelpHelp    = "`/%s help` — this list"

	// Alert headlines / hints.
	copyAlertNoSuchFire       = "no such fire"
	copyAlertTwoFires         = "two fires by that name — ask a keeper"
	copyAlertSomethingSideways = "something went sideways"
	copyAlertUnauthorizedHeadline = "you can't tend this fire"
	copyAlertUnauthorizedBody     = "ask a keeper if you'd like access."
	copyAlertGuildBlocked     = "this bot isn't keeping a fire in this server"

	// Alert hints (monospace footer).
	copyHintTagCollision  = "err · ec2_tag_collision"
	copyHintRoleMissing   = "err · discord_role_missing"
	copyHintEC2ErrorFmt   = "err · ec2_%s"
	copyHintS3ErrorFmt    = "err · s3_%s"
	copyHintTryStatusFmt  = "try · /%s status"

	// State labels (pill text).
	labelRunning   = "burning"
	labelPending   = "lighting"
	labelStopping  = "dying down"
	labelStopped   = "out"
	labelUnknown   = "unknown"

	// Field names — running Hero grid.
	fieldAddress = "address"
	fieldUptime  = "uptime"
	fieldBackup  = "backup"
)

// Discord embed types — minimal subset needed for webhook PATCH bodies.
// https://discord.com/developers/docs/resources/channel#embed-object
type Embed struct {
	Title       string       `json:"title,omitempty"`
	Description string       `json:"description,omitempty"`
	Color       int          `json:"color,omitempty"`
	Fields      []EmbedField `json:"fields,omitempty"`
	Footer      *EmbedFooter `json:"footer,omitempty"`
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

// heroEmbed builds a branded Hero embed (public, used for successful /start and /stop edits).
// leadline is an active-voice headline; attribution is an under-field byline shown on running state.
// addr / uptime / backup are only rendered when the state is running (or when provided, for flex).
func heroEmbed(game, state, leadline, body string) Embed {
	return Embed{
		Title:       heroTitle(game, state),
		Description: composeHeroDescription(leadline, body),
		Color:       stateColor(state),
	}
}

// heroEmbedRunning builds the 3-field running-state Hero: ADDRESS full-width top row,
// UPTIME + BACKUP side-by-side bottom row (WORLD omitted — see Key Decision #2).
// backup may be empty — the BACKUP field is then omitted (per BACKUP empty-bucket fallback).
func heroEmbedRunning(game, leadline, attribution, addr, uptime, backup string) Embed {
	fields := []EmbedField{
		{Name: fieldAddress, Value: "`" + addr + "`", Inline: false},
		{Name: fieldUptime, Value: uptime, Inline: true},
	}
	if backup != "" {
		fields = append(fields, EmbedField{Name: fieldBackup, Value: backup, Inline: true})
	}
	return Embed{
		Title:       heroTitle(game, "running"),
		Description: composeHeroDescription(leadline, attribution),
		Color:       colorEmber,
		Fields:      fields,
	}
}

// composeHeroDescription renders the two Hero description slots (leadline then byline/body).
// One blank line between them to match the brand's visual hierarchy.
func composeHeroDescription(leadline, second string) string {
	switch {
	case leadline == "" && second == "":
		return ""
	case leadline == "":
		return second
	case second == "":
		return leadline
	default:
		return leadline + "\n\n_" + second + "_"
	}
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

// alertEmbed builds an Alert (ephemeral) embed: danger-colored bar, two-line headline+body,
// monospace hint footer. hint should follow the `err · <code>` or `try · <cmd>` patterns.
func alertEmbed(headline, body, hint string) Embed {
	return Embed{
		Title:       headline,
		Description: body,
		Color:       colorDanger,
		Footer:      &EmbedFooter{Text: hint},
	}
}

// --- Response helpers (build on top of existing ephemeralResponse/publicResponse shapes) ---

// ephemeralEmbedResponse returns an immediate type 4 response with embeds, flags=64.
// Used when the handler's synchronous state check lets it short-circuit (idempotency, /status, /help, /hello, Alerts).
func ephemeralEmbedResponse(embed Embed) InteractionResponse {
	return InteractionResponse{
		Type: discordChannelMessage,
		Data: &InteractionResponseData{
			Flags:  discordEphemeralFlag,
			Embeds: []Embed{embed},
		},
	}
}

// publicEmbedResponse returns an immediate type 4 response with a public embed.
// (Rarely used directly — /start and /stop defer instead; kept for symmetry and tests.)
func publicEmbedResponse(embed Embed) InteractionResponse {
	return InteractionResponse{
		Type: discordChannelMessage,
		Data: &InteractionResponseData{
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
