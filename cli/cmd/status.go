package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status <game>",
	Short: "Show current state of a specific game server",
	Long: `Show the current state of a game server, including IP address,
EC2 instance state, and the last backup timestamp in S3.`,
	Args: cobra.ExactArgs(1),
	RunE: runStatus,
}

func runStatus(cmd *cobra.Command, args []string) error {
	game := args[0]
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cfg, err := awsConfig(ctx)
	if err != nil {
		return fmt.Errorf("loading AWS config: %w", err)
	}

	ec2Client := ec2.NewFromConfig(cfg)
	s3Client := s3.NewFromConfig(cfg)
	region := cfg.Region

	fmt.Printf("Status: %s\n", game)
	fmt.Println(strings.Repeat("-", 40))

	// EC2 instance info
	instanceID, state, ip, err := describeGameInstance(ctx, ec2Client, game)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error querying EC2 instance for game %s: %v\n", game, err)
		fmt.Printf("  Instance: error\n")
	} else if instanceID == "" {
		fmt.Printf("  Instance: not provisioned\n")
	} else {
		fmt.Printf("  Instance ID:    %s\n", instanceID)
		fmt.Printf("  Instance State: %s\n", state)
		fmt.Printf("  Public IP:      %s\n", ip)
	}

	// Last backup in S3
	backupBucket := backupBucketName(game, region)
	lastBackup, err := latestObjectByPrefix(ctx, s3Client, backupBucket, "")
	if err != nil {
		fmt.Fprintf(os.Stderr, "error querying S3 backups for game %s: %v\n", game, err)
		fmt.Printf("  Last Backup:    error\n")
	} else if lastBackup == "" {
		fmt.Printf("  Last Backup:    none\n")
	} else {
		fmt.Printf("  Last Backup:    s3://%s/%s\n", backupBucket, lastBackup)
	}

	return nil
}

// describeGameInstance finds the EC2 instance for a game by tag.
func describeGameInstance(ctx context.Context, ec2Client ec2API, game string) (id, state, ip string, err error) {
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
		return "", "", "", err
	}

	for _, r := range resp.Reservations {
		for _, i := range r.Instances {
			if i.State != nil && i.State.Name == ec2types.InstanceStateNameTerminated {
				continue
			}
			instanceID := ""
			if i.InstanceId != nil {
				instanceID = *i.InstanceId
			}
			stateName := "unknown"
			if i.State != nil {
				stateName = string(i.State.Name)
			}
			publicIP := "-"
			if i.PublicIpAddress != nil {
				publicIP = *i.PublicIpAddress
			}
			return instanceID, stateName, publicIP, nil
		}
	}
	return "", "", "", nil
}
