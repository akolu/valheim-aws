package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// S3API is the narrow interface the bot needs from S3. Injectable for tests.
type S3API interface {
	ListObjectsV2(ctx context.Context, params *s3.ListObjectsV2Input, optFns ...func(*s3.Options)) (*s3.ListObjectsV2Output, error)
}

// Compile-time assertion that *s3.Client satisfies the narrow S3API interface.
var _ S3API = (*s3.Client)(nil)

// Shared S3 client, initialised once per Lambda warm container.
// Follows the sync.Once pattern established at main.go:138-187.
var (
	s3ClientOnce   sync.Once
	sharedS3Client *s3.Client
	s3ClientErr    error
)

func newS3Client(ctx context.Context) (*s3.Client, error) {
	region := os.Getenv("AWS_REGION")
	if region == "" {
		region = "eu-north-1"
	}
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(region))
	if err != nil {
		return nil, err
	}
	return s3.NewFromConfig(cfg), nil
}

// getS3Client returns the shared S3 client, initialising it on first call.
func getS3Client(ctx context.Context) (*s3.Client, error) {
	s3ClientOnce.Do(func() {
		sharedS3Client, s3ClientErr = newS3Client(ctx)
	})
	return sharedS3Client, s3ClientErr
}

// --- Backup lookup ---

// latestBackupTime returns the LastModified of the newest object in the per-game
// backup bucket (`bonfire-<game>-backups-<region>`), or (nil, nil) if the bucket is
// empty / the prefix has no objects. Never surfaces the S3 error to the user:
// on failure it logs and returns (nil, err) — callers treat this as "never burned".
func latestBackupTime(ctx context.Context, client S3API, game, region string) (*time.Time, error) {
	bucket := fmt.Sprintf("bonfire-%s-backups-%s", game, region)
	out, err := client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
		Bucket: aws.String(bucket),
	})
	if err != nil {
		return nil, err
	}
	var newest *time.Time
	for i := range out.Contents {
		obj := out.Contents[i]
		if obj.LastModified == nil {
			continue
		}
		if newest == nil || obj.LastModified.After(*newest) {
			t := *obj.LastModified
			newest = &t
		}
	}
	return newest, nil
}

// backupElapsedString returns either an elapsed string ("15m ago") or empty when
// the bucket is empty / lookup errors. Callers that distinguish "empty" from
// "error" should use latestBackupTime directly. logTag is the caller's path
// prefix ("[ack] " or "[poll] ") — `backupElapsedString` is used from both.
func backupElapsedString(ctx context.Context, client S3API, game, region, logTag string) string {
	t, err := latestBackupTime(ctx, client, game, region)
	if err != nil {
		log.Printf("%sbackupElapsedString: s3 lookup for game %q failed: %v", logTag, game, err)
		return ""
	}
	if t == nil {
		return ""
	}
	return formatElapsed(time.Since(*t)) + " ago"
}

// --- Discord webhook PATCH client ---

// discordAPIBase is the webhook base URL. Overridable in tests.
var discordAPIBase = "https://discord.com/api/v10"

// webhookPATCHEndpoint returns the URL for editing the original interaction response.
// Shape: https://discord.com/api/v10/webhooks/{application_id}/{interaction_token}/messages/@original
func webhookPATCHEndpoint(appID, token string) string {
	return fmt.Sprintf("%s/webhooks/%s/%s/messages/@original", discordAPIBase, appID, token)
}

// discordPATCHResult captures the outcome of one PATCH attempt for the poller's
// decision-making (retry vs. exit vs. continue).
type discordPATCHResult struct {
	StatusCode int
	// RetryAfter is populated on 429 responses (in seconds, best effort).
	RetryAfter time.Duration
	Err        error
}

// httpDoer is satisfied by *http.Client and by any test stub.
type httpDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

// sharedHTTPClient is reused across poll ticks; 10s per-request timeout leaves
// ample headroom under Discord's 3s/edit floor yet protects against network hangs.
var sharedHTTPClient httpDoer = &http.Client{Timeout: 10 * time.Second}

// patchOriginalMessage issues a single PATCH to edit the original interaction message.
// Callers handle the result (429 retry, 404 exit-clean, 5xx skip, 2xx success) themselves
// because retry/exit policy differs between mid-poll and terminal-PATCH contexts.
func patchOriginalMessage(ctx context.Context, client httpDoer, appID, token string, body WebhookEditBody) discordPATCHResult {
	b, err := json.Marshal(body)
	if err != nil {
		return discordPATCHResult{Err: fmt.Errorf("marshal webhook body: %w", err)}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPatch, webhookPATCHEndpoint(appID, token), bytes.NewReader(b))
	if err != nil {
		return discordPATCHResult{Err: fmt.Errorf("build PATCH request: %w", err)}
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return discordPATCHResult{Err: fmt.Errorf("discord PATCH: %w", err)}
	}
	defer resp.Body.Close()
	// Drain to let the connection be reused.
	_, _ = io.Copy(io.Discard, resp.Body)

	result := discordPATCHResult{StatusCode: resp.StatusCode}
	if resp.StatusCode == http.StatusTooManyRequests {
		if retryHeader := resp.Header.Get("Retry-After"); retryHeader != "" {
			if secs, err := strconv.ParseFloat(retryHeader, 64); err == nil {
				result.RetryAfter = time.Duration(secs * float64(time.Second))
			}
		}
	}
	return result
}

// --- Polling loop ---

// pollConfig collects the knobs the poll loop needs; grouped so the main.go
// caller doesn't thread 10 args into a single function.
type pollConfig struct {
	Game         string
	Action       string // "start" or "stop"
	UserID       string // Discord user ID — used for leadline/attribution
	InstanceID   string
	AppID        string
	Token        string
	Region       string
	EC2Client    EC2API
	S3Client     S3API
	HTTPClient   httpDoer
	// EC2PollInterval is how often we hit DescribeInstances. Default 5s.
	EC2PollInterval time.Duration
	// DiscordPatchFloor is the minimum time between PATCHes (absent a state change). Default 20s.
	DiscordPatchFloor time.Duration
	// DeadlineReserve is the slack reserved for the terminal PATCH after the main loop exits. Default 10s.
	DeadlineReserve time.Duration
}

// withDefaults fills in sensible defaults for the timing fields.
func (c *pollConfig) withDefaults() {
	if c.EC2PollInterval == 0 {
		c.EC2PollInterval = 5 * time.Second
	}
	if c.DiscordPatchFloor == 0 {
		c.DiscordPatchFloor = 20 * time.Second
	}
	if c.DeadlineReserve == 0 {
		c.DeadlineReserve = 10 * time.Second
	}
	if c.HTTPClient == nil {
		c.HTTPClient = sharedHTTPClient
	}
}

// pollStartFlow polls EC2 until the instance reaches a terminal state for a /start
// action, editing the original Discord message on state changes and every
// DiscordPatchFloor. Returns when the loop terminates (success, interrupted,
// deadline, or unrecoverable error).
//
// Terminal states (see plan § Deferred-response flow — /start):
//   running            → success; ember Hero with ADDRESS + UPTIME + BACKUP
//   stopping/stopped   → "someone banked the coals"; ash Hero, not an error
//   ctx deadline       → "still lighting — i'll keep an eye on it"; ice Hero
//
// The pre-defer synchronous state check in the handler guarantees we only enter
// this loop from state=stopped; the poller therefore expects to observe
// stopped → pending → running.
func pollStartFlow(ctx context.Context, cfg pollConfig, startedAt time.Time) {
	cfg.withDefaults()
	runPollLoop(ctx, cfg, startedAt, buildStartInFlight, buildStartTerminal)
}

// pollStopFlow mirrors pollStartFlow for /stop.
//
// Terminal states (see plan § Deferred-response flow — /stop):
//   stopped            → success; ash Hero "put out by @user"
//   pending/running    → "fire's relighting · someone wasn't done"; spark Hero, not an error
//   ctx deadline       → "still dying down — i'll keep an eye on it"; ice Hero
func pollStopFlow(ctx context.Context, cfg pollConfig, startedAt time.Time) {
	cfg.withDefaults()
	runPollLoop(ctx, cfg, startedAt, buildStopInFlight, buildStopTerminal)
}

// buildInFlight returns the embed shown while the action is in progress.
// Called on poll ticks; body updates the elapsed timer.
type buildInFlightFn func(cfg pollConfig, elapsed time.Duration, currentState string) Embed

// buildTerminal returns the terminal embed for the given terminal state.
// `deadline` is true when the loop exited due to ctx deadline (soft deadline message path).
type buildTerminalFn func(ctx context.Context, cfg pollConfig, info instanceInfo, deadline bool) Embed

func runPollLoop(
	ctx context.Context,
	cfg pollConfig,
	startedAt time.Time,
	buildInFlight buildInFlightFn,
	buildTerminal buildTerminalFn,
) {
	// Derive the poll ctx from the Lambda ctx minus the reserve for the terminal PATCH.
	pollCtx, cancel := deriveDeadline(ctx, cfg.DeadlineReserve)
	defer cancel()

	// Initial "in-flight" edit (T+~0.4s) — gives the user immediate visual feedback.
	initialState := "pending"
	if cfg.Action == "stop" {
		initialState = "stopping"
	}
	initialEmbed := buildInFlight(cfg, 0, initialState)
	if res := patchOriginalMessage(pollCtx, cfg.HTTPClient, cfg.AppID, cfg.Token, WebhookEditBody{Embeds: []Embed{initialEmbed}}); res.Err != nil {
		log.Printf("[poll] runPollLoop: initial PATCH error: %v", res.Err)
	} else if res.StatusCode == http.StatusNotFound {
		log.Printf("[poll] runPollLoop: initial PATCH returned 404 — original message gone; exiting")
		return
	}

	var (
		lastObservedState = initialState
		lastPatchAt       = time.Now()
		deadlineHit       = false
		lastSeenInfo      instanceInfo
	)

	ticker := time.NewTicker(cfg.EC2PollInterval)
	defer ticker.Stop()

	terminalStates := startTerminals
	if cfg.Action == "stop" {
		terminalStates = stopTerminals
	}

	for {
		select {
		case <-pollCtx.Done():
			log.Printf("[poll] runPollLoop: context deadline reached for %s %s", cfg.Action, cfg.Game)
			deadlineHit = true
			// fall through to terminal PATCH using a fresh short-lived ctx
		case <-ticker.C:
			info, err := findInstanceByGame(pollCtx, cfg.EC2Client, cfg.Game)
			if err != nil {
				// Log and skip; next tick retries naturally.
				log.Printf("[poll] runPollLoop: DescribeInstances error for %s: %v", cfg.Game, err)
				continue
			}
			lastSeenInfo = info

			// Terminal?
			if _, terminal := terminalStates[info.State]; terminal {
				// Issue terminal PATCH with a fresh short-lived context so the
				// final message always ships even if the main ctx is at the edge.
				finalCtx, finalCancel := context.WithTimeout(context.Background(), cfg.DeadlineReserve)
				embed := buildTerminal(finalCtx, cfg, info, false)
				patchWithRetry(finalCtx, cfg.HTTPClient, cfg.AppID, cfg.Token, WebhookEditBody{Embeds: []Embed{embed}})
				finalCancel()
				log.Printf("[poll] runPollLoop: %s %s reached terminal state %q", cfg.Action, cfg.Game, info.State)
				return
			}

			// Not terminal — decide whether to emit an interstitial PATCH.
			stateChanged := info.State != lastObservedState
			pulseDue := time.Since(lastPatchAt) >= cfg.DiscordPatchFloor
			if stateChanged || pulseDue {
				elapsed := time.Since(startedAt)
				embed := buildInFlight(cfg, elapsed, info.State)
				res := patchOriginalMessage(pollCtx, cfg.HTTPClient, cfg.AppID, cfg.Token, WebhookEditBody{Embeds: []Embed{embed}})
				if res.StatusCode == http.StatusNotFound {
					log.Printf("[poll] runPollLoop: mid-poll PATCH returned 404 — original message gone; exiting")
					return
				}
				// 429: sleep for Retry-After and loop; next tick naturally retries.
				if res.StatusCode == http.StatusTooManyRequests && res.RetryAfter > 0 {
					log.Printf("[poll] runPollLoop: 429 from discord; sleeping %s", res.RetryAfter)
					timer := time.NewTimer(res.RetryAfter)
					select {
					case <-timer.C:
					case <-pollCtx.Done():
						timer.Stop()
					}
				}
				if res.Err != nil {
					log.Printf("[poll] runPollLoop: mid-poll PATCH error: %v", res.Err)
				}
				lastObservedState = info.State
				lastPatchAt = time.Now()
			}
			continue
		}
		break
	}

	// We get here when pollCtx.Done() fired.
	if deadlineHit {
		finalCtx, finalCancel := context.WithTimeout(context.Background(), cfg.DeadlineReserve)
		defer finalCancel()
		embed := buildTerminal(finalCtx, cfg, lastSeenInfo, true)
		patchWithRetry(finalCtx, cfg.HTTPClient, cfg.AppID, cfg.Token, WebhookEditBody{Embeds: []Embed{embed}})
	}
}

// patchWithRetry issues a terminal PATCH and retries once synchronously on transient failure.
func patchWithRetry(ctx context.Context, client httpDoer, appID, token string, body WebhookEditBody) {
	res := patchOriginalMessage(ctx, client, appID, token, body)
	if res.Err == nil && res.StatusCode >= 200 && res.StatusCode < 300 {
		return
	}
	// 404 → message gone; nothing to retry.
	if res.StatusCode == http.StatusNotFound {
		log.Printf("[poll] patchWithRetry: terminal PATCH returned 404; giving up")
		return
	}
	// Back off briefly, then retry once.
	sleep := 500 * time.Millisecond
	if res.StatusCode == http.StatusTooManyRequests && res.RetryAfter > 0 {
		sleep = res.RetryAfter
	}
	timer := time.NewTimer(sleep)
	select {
	case <-timer.C:
	case <-ctx.Done():
		timer.Stop()
		log.Printf("[poll] patchWithRetry: ctx cancelled before retry")
		return
	}
	res2 := patchOriginalMessage(ctx, client, appID, token, body)
	if res2.Err != nil {
		log.Printf("[poll] patchWithRetry: terminal PATCH retry failed: %v", res2.Err)
	} else if res2.StatusCode >= 400 {
		log.Printf("[poll] patchWithRetry: terminal PATCH retry got status %d", res2.StatusCode)
	}
}

// deriveDeadline returns a child context with a deadline that is the parent
// deadline minus the reserve, or the parent unchanged if the parent has no deadline
// (e.g. in tests with context.Background()).
func deriveDeadline(parent context.Context, reserve time.Duration) (context.Context, context.CancelFunc) {
	if dl, ok := parent.Deadline(); ok {
		adjusted := dl.Add(-reserve)
		if time.Until(adjusted) > 0 {
			return context.WithDeadline(parent, adjusted)
		}
	}
	return context.WithCancel(parent)
}

// Terminal-state sets for each action's poll.
//
// `not_found` / `multiple` are included for both actions: if the instance
// disappears or splits mid-poll (tag race, manual termination, operator
// intervention), we exit cleanly with an interrupted message rather than
// spinning the full 170s deadline (DA finding #4).
var (
	startTerminals = map[string]struct{}{
		"running":   {},
		"stopping":  {},
		"stopped":   {},
		"not_found": {},
		"multiple":  {},
	}
	stopTerminals = map[string]struct{}{
		"stopped":   {},
		"running":   {},
		"pending":   {},
		"not_found": {},
		"multiple":  {},
	}
)

// --- In-flight / terminal embed builders ---

func buildStartInFlight(cfg pollConfig, elapsed time.Duration, _ string) Embed {
	body := fmt.Sprintf(copyLightingBody, formatElapsed(elapsed))
	return heroEmbed(cfg.Game, "pending", fmt.Sprintf(copyLeadlineLit, cfg.UserID), body)
}

func buildStartTerminal(ctx context.Context, cfg pollConfig, info instanceInfo, deadline bool) Embed {
	if deadline {
		// Soft deadline during /start: ice color + "lighting" label so title
		// matches the "still lighting…" body. Using state="stopping" would
		// paint ice but show "dying down" — internally contradictory (UX I5).
		return heroEmbedWithLabel(cfg.Game, labelPending, colorIce, "", copyStartDeadlineBody)
	}
	switch info.State {
	case "running":
		addr := info.PublicIP
		if addr == "" {
			addr = "N/A"
		}
		uptime := formatElapsed(elapsedSince(info.LaunchTime))
		backup := lookupBackup(ctx, cfg)
		return heroEmbedRunning(
			cfg.Game,
			fmt.Sprintf(copyLeadlineLit, cfg.UserID),
			fmt.Sprintf(copyAttributionLitBy, cfg.UserID),
			addr, uptime, backup,
		)
	case "stopping", "stopped":
		return heroEmbed(cfg.Game, "stopped", "", copyStartInterruptedBody)
	case "not_found":
		// Instance vanished mid-poll — tag was removed or instance terminated.
		return alertEmbed(copyAlertNoSuchFire, "the fire's gone — did someone retire the game?", fmt.Sprintf(copyHintTryStatusFmt, cfg.Game))
	case "multiple":
		return alertEmbed(copyAlertTwoFires, "i found more than one — that shouldn't happen", copyHintTagCollision)
	}
	// Unknown terminal — fall back to ash.
	return heroEmbed(cfg.Game, "stopped", "", copyStartInterruptedBody)
}

func buildStopInFlight(cfg pollConfig, _ time.Duration, _ string) Embed {
	return heroEmbed(cfg.Game, "stopping", "", copyStoppingBody)
}

func buildStopTerminal(_ context.Context, cfg pollConfig, info instanceInfo, deadline bool) Embed {
	if deadline {
		// /stop soft-deadline: state="stopping" already produces ice + "dying
		// down" consistently with the "still dying down" body — no override needed.
		return heroEmbed(cfg.Game, "stopping", "", copyStopDeadlineBody)
	}
	switch info.State {
	case "stopped":
		return heroEmbed(cfg.Game, "stopped", fmt.Sprintf(copyLeadlinePutOut, cfg.UserID), "")
	case "pending", "running":
		return heroEmbed(cfg.Game, "pending", "", copyStopInterruptedBody)
	case "not_found":
		return alertEmbed(copyAlertNoSuchFire, "the fire's gone mid-bank — did someone retire the game?", fmt.Sprintf(copyHintTryStatusFmt, cfg.Game))
	case "multiple":
		return alertEmbed(copyAlertTwoFires, "i found more than one — that shouldn't happen", copyHintTagCollision)
	}
	return heroEmbed(cfg.Game, "stopped", "", copyStopInterruptedBody)
}

// lookupBackup swallows errors and returns "" on empty-bucket / failure.
// The Hero running-state builder then omits the BACKUP field.
func lookupBackup(ctx context.Context, cfg pollConfig) string {
	if cfg.S3Client == nil || cfg.Region == "" {
		return ""
	}
	return backupElapsedString(ctx, cfg.S3Client, cfg.Game, cfg.Region, "[poll] ")
}

