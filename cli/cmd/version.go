package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// Version is embedded at build time via ldflags:
//
//	go build -ldflags "-X github.com/bonfire/cli/cmd.Version=$(git rev-parse HEAD)"
var Version = "dev"

type versionCheckResult struct {
	behind bool
	latest string
}

var versionCheckCh <-chan versionCheckResult

func init() {
	rootCmd.PersistentPreRun = func(cmd *cobra.Command, args []string) {
		// Skip version check when running update — it's about to pull anyway.
		if cmd.Name() == "update" {
			return
		}
		versionCheckCh = startVersionCheck()
	}
	rootCmd.PersistentPostRun = func(cmd *cobra.Command, args []string) {
		if versionCheckCh == nil {
			return
		}
		select {
		case result := <-versionCheckCh:
			if result.behind {
				fmt.Fprintf(os.Stderr, "\nNotice: bonfire update available (latest: %s). Run 'bonfire update' to upgrade.\n", result.latest)
			}
		case <-time.After(2 * time.Second):
			// version check timed out — skip silently
		}
	}
}

// startVersionCheck launches a background git fetch + compare and returns a
// channel that receives the result. The channel is buffered so the goroutine
// never blocks if the result is never read.
func startVersionCheck() <-chan versionCheckResult {
	ch := make(chan versionCheckResult, 1)
	go func() {
		ch <- checkVersion()
	}()
	return ch
}

// checkVersion fetches origin/main and compares it to the embedded Version.
// Returns a zero-value result (not behind) on any error so failures are silent.
func checkVersion() versionCheckResult {
	if Version == "dev" {
		return versionCheckResult{}
	}

	root, err := findRepoRoot()
	if err != nil {
		return versionCheckResult{}
	}

	fetchCmd := exec.Command("git", "fetch", "origin", "main", "--quiet")
	fetchCmd.Dir = root
	if err := fetchCmd.Run(); err != nil {
		return versionCheckResult{}
	}

	revCmd := exec.Command("git", "rev-parse", "origin/main")
	revCmd.Dir = root
	out, err := revCmd.Output()
	if err != nil {
		return versionCheckResult{}
	}

	latest := strings.TrimSpace(string(out))
	if latest == "" || latest == Version {
		return versionCheckResult{}
	}
	// Show just the short hash (first 8 chars) in the notice.
	short := latest
	if len(short) > 8 {
		short = short[:8]
	}
	return versionCheckResult{behind: true, latest: short}
}
