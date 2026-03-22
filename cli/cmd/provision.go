package cmd

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/spf13/cobra"
)

var restoreFlag bool

var provisionCmd = &cobra.Command{
	Use:   "provision <game>",
	Short: "Provision a game server (terraform init + apply)",
	Long: `Provision a game server by running terraform init and apply for the
specified game workspace. With --restore, lists available saves in the
long-term bucket and restores the selected save after provisioning.`,
	Args: cobra.ExactArgs(1),
	RunE: runProvision,
}

func init() {
	provisionCmd.Flags().BoolVar(&restoreFlag, "restore", false, "Restore a save from the long-term bucket after provisioning")
}

func runProvision(cmd *cobra.Command, args []string) error {
	game := args[0]
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	dir, err := terraformDir(game)
	if err != nil {
		return err
	}

	fmt.Printf("Provisioning %s...\n", game)
	if err := terraformInit(dir); err != nil {
		return err
	}
	if err := terraformApply(dir); err != nil {
		return err
	}
	fmt.Printf("✓ %s provisioned\n", game)

	if restoreFlag {
		cfg, err := awsConfig(ctx)
		if err != nil {
			return fmt.Errorf("loading AWS config: %w", err)
		}
		if err := restoreFromLongterm(ctx, s3.NewFromConfig(cfg), game, os.Stdin); err != nil {
			return err
		}
	}

	return nil
}

// restoreFromLongterm lists saves and prompts the user to choose one.
// The reader parameter allows injection of input for testing.
func restoreFromLongterm(ctx context.Context, s3Client s3API, game string, reader io.Reader) error {
	ltBucket := longtermBucketName(game)
	fmt.Printf("\nFetching available saves from s3://%s...\n", ltBucket)

	keys, err := listObjects(ctx, s3Client, ltBucket, "")
	if err != nil {
		return fmt.Errorf("listing saves: %w", err)
	}
	if len(keys) == 0 {
		fmt.Println("No saves found in long-term bucket.")
		return nil
	}

	fmt.Println("Available saves:")
	for i, k := range keys {
		fmt.Printf("  [%d] %s\n", i+1, k)
	}
	fmt.Printf("Select save to restore [1-%d] (or 0 to skip): ", len(keys))

	scanner := bufio.NewScanner(reader)
	scanner.Scan()
	input := strings.TrimSpace(scanner.Text())
	choice, err := strconv.Atoi(input)
	if err != nil || choice == 0 {
		fmt.Println("Skipping restore.")
		return nil
	}
	if choice < 1 || choice > len(keys) {
		return fmt.Errorf("invalid selection: %d", choice)
	}

	selectedKey := keys[choice-1]
	fmt.Printf("Selected: %s\n", selectedKey)
	fmt.Println("(Save file is available in the long-term bucket; manual restore to instance is required.)")
	fmt.Printf("  Source: s3://%s/%s\n", ltBucket, selectedKey)

	return nil
}
