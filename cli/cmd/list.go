package cmd

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/spf13/cobra"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List all games with their provisioned state and current IP",
	Args:  cobra.NoArgs,
	RunE:  runList,
}

func runList(cmd *cobra.Command, args []string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	games, err := availableGames()
	if err != nil {
		return err
	}
	if len(games) == 0 {
		fmt.Println("No games found.")
		return nil
	}

	cfg, err := awsConfig(ctx)
	if err != nil {
		return fmt.Errorf("loading AWS config: %w", err)
	}
	ec2Client := ec2.NewFromConfig(cfg)

	fmt.Printf("%-20s %-16s %-18s\n", "GAME", "STATE", "IP")
	fmt.Printf("%-20s %-16s %-18s\n", "----", "-----", "--")

	for _, game := range games {
		state, ip := gameInstanceInfo(ctx, ec2Client, game)
		fmt.Printf("%-20s %-16s %-18s\n", game, state, ip)
	}
	return nil
}

// gameInstanceInfo queries EC2 for the instance associated with a game workspace.
// Errors are written to stderr; the returned ip uses "-" when not available.
func gameInstanceInfo(ctx context.Context, ec2Client ec2API, game string) (state, ip string) {
	_, state, ip, err := describeGameInstance(ctx, ec2Client, game)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error querying EC2 for game %s: %v\n", game, err)
		return "error", "-"
	}
	if state == "" {
		return "not-provisioned", "-"
	}
	if ip == "-" || ip == "" {
		ip = "-"
	}
	return state, ip
}
