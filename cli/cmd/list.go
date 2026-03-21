package cmd

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/spf13/cobra"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List all games with their provisioned state and current IP",
	Args:  cobra.NoArgs,
	RunE:  runList,
}

func runList(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

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

// gameInstanceInfo queries EC2 for the instance associated with a game workspace
// by looking up instances tagged with tag:Game=<game> and tag:Project=bonfire.
func gameInstanceInfo(ctx context.Context, ec2Client *ec2.Client, game string) (string, string) {
	resp, err := ec2Client.DescribeInstances(ctx, &ec2.DescribeInstancesInput{
		Filters: []ec2types.Filter{
			{
				Name:   aws.String("tag:Game"),
				Values: []string{game},
			},
			{
				Name:   aws.String("tag:Project"),
				Values: []string{"bonfire"},
			},
		},
	})
	if err != nil {
		return "error", "-"
	}

	for _, r := range resp.Reservations {
		for _, i := range r.Instances {
			if i.State != nil && i.State.Name == ec2types.InstanceStateNameTerminated {
				continue
			}
			state := "unknown"
			if i.State != nil {
				state = string(i.State.Name)
			}
			ip := "-"
			if i.PublicIpAddress != nil {
				ip = *i.PublicIpAddress
			}
			return state, ip
		}
	}
	return "not-provisioned", "-"
}
