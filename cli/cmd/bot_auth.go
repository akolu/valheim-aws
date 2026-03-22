package cmd

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/ssm/types"
	"github.com/spf13/cobra"
)

// ssmAPI is the subset of the SSM client API used by this package.
type ssmAPI interface {
	GetParameter(ctx context.Context, params *ssm.GetParameterInput, optFns ...func(*ssm.Options)) (*ssm.GetParameterOutput, error)
	PutParameter(ctx context.Context, params *ssm.PutParameterInput, optFns ...func(*ssm.Options)) (*ssm.PutParameterOutput, error)
	DeleteParameter(ctx context.Context, params *ssm.DeleteParameterInput, optFns ...func(*ssm.Options)) (*ssm.DeleteParameterOutput, error)
}

var botGrantCmd = &cobra.Command{
	Use:   "grant <game> <user_id>",
	Short: "Grant a user access to a game",
	Args:  cobra.ExactArgs(2),
	RunE:  runBotGrant,
}

var botRevokeCmd = &cobra.Command{
	Use:   "revoke <game> <user_id>",
	Short: "Revoke a user's access to a game",
	Args:  cobra.ExactArgs(2),
	RunE:  runBotRevoke,
}

var botTrustCmd = &cobra.Command{
	Use:   "trust <guild_id>",
	Short: "Add a guild to the allowed guilds list",
	Args:  cobra.ExactArgs(1),
	RunE:  runBotTrust,
}

var botUntrustCmd = &cobra.Command{
	Use:   "untrust <guild_id>",
	Short: "Remove a guild from the allowed guilds list",
	Args:  cobra.ExactArgs(1),
	RunE:  runBotUntrust,
}

func init() {
	botCmd.AddCommand(botGrantCmd)
	botCmd.AddCommand(botRevokeCmd)
	botCmd.AddCommand(botTrustCmd)
	botCmd.AddCommand(botUntrustCmd)
}

func runBotGrant(cmd *cobra.Command, args []string) error {
	if err := checkBotDeployed(); err != nil {
		return err
	}
	game, userID := args[0], args[1]
	ctx := context.Background()
	cfg, err := awsConfig(ctx)
	if err != nil {
		return err
	}
	client := ssm.NewFromConfig(cfg)
	paramPath := fmt.Sprintf("/bonfire/%s/authorized_users", game)
	return ssmListAdd(ctx, client, paramPath, userID, func(n int) {
		fmt.Printf("✓ granted user %s access to %s (%d users total)\n", userID, game, n)
	})
}

func runBotRevoke(cmd *cobra.Command, args []string) error {
	if err := checkBotDeployed(); err != nil {
		return err
	}
	game, userID := args[0], args[1]
	ctx := context.Background()
	cfg, err := awsConfig(ctx)
	if err != nil {
		return err
	}
	client := ssm.NewFromConfig(cfg)
	paramPath := fmt.Sprintf("/bonfire/%s/authorized_users", game)
	return ssmListRemove(ctx, client, paramPath, userID, func(n int) {
		fmt.Printf("✓ revoked user %s access to %s (%d users total)\n", userID, game, n)
	})
}

func runBotTrust(cmd *cobra.Command, args []string) error {
	if err := checkBotDeployed(); err != nil {
		return err
	}
	guildID := args[0]
	ctx := context.Background()
	cfg, err := awsConfig(ctx)
	if err != nil {
		return err
	}
	client := ssm.NewFromConfig(cfg)
	return ssmListAdd(ctx, client, "/bonfire/allowed_guilds", guildID, func(n int) {
		fmt.Printf("✓ trusted guild %s (%d guilds total)\n", guildID, n)
	})
}

func runBotUntrust(cmd *cobra.Command, args []string) error {
	if err := checkBotDeployed(); err != nil {
		return err
	}
	guildID := args[0]
	ctx := context.Background()
	cfg, err := awsConfig(ctx)
	if err != nil {
		return err
	}
	client := ssm.NewFromConfig(cfg)
	return ssmListRemove(ctx, client, "/bonfire/allowed_guilds", guildID, func(n int) {
		fmt.Printf("✓ untrusted guild %s (%d guilds total)\n", guildID, n)
	})
}

// ssmListAdd adds value to the comma-separated list at paramPath.
// If the parameter doesn't exist, it's created with just value.
// Idempotent: if value is already present, onSuccess is called with current count.
func ssmListAdd(ctx context.Context, client ssmAPI, paramPath, value string, onSuccess func(int)) error {
	entries, err := ssmGetList(ctx, client, paramPath)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e == value {
			onSuccess(len(entries))
			return nil
		}
	}
	entries = append(entries, value)
	return ssmPutList(ctx, client, paramPath, entries, onSuccess)
}

// ssmListRemove removes value from the comma-separated list at paramPath.
// If the parameter becomes empty, it's deleted.
// Idempotent: if value is not present, onSuccess is called with current count.
func ssmListRemove(ctx context.Context, client ssmAPI, paramPath, value string, onSuccess func(int)) error {
	entries, err := ssmGetList(ctx, client, paramPath)
	if err != nil {
		return err
	}
	filtered := make([]string, 0, len(entries))
	for _, e := range entries {
		if e != value {
			filtered = append(filtered, e)
		}
	}
	if len(filtered) == len(entries) {
		// Not found — idempotent success
		onSuccess(len(filtered))
		return nil
	}
	if len(filtered) == 0 {
		_, err := client.DeleteParameter(ctx, &ssm.DeleteParameterInput{
			Name: aws.String(paramPath),
		})
		if err != nil {
			return fmt.Errorf("deleting SSM parameter %s: %w", paramPath, err)
		}
		onSuccess(0)
		return nil
	}
	return ssmPutList(ctx, client, paramPath, filtered, onSuccess)
}

// ssmGetList reads the comma-separated list from paramPath.
// Returns an empty slice if the parameter doesn't exist.
func ssmGetList(ctx context.Context, client ssmAPI, paramPath string) ([]string, error) {
	out, err := client.GetParameter(ctx, &ssm.GetParameterInput{
		Name:           aws.String(paramPath),
		WithDecryption: aws.Bool(false),
	})
	if err != nil {
		var notFound *types.ParameterNotFound
		if errors.As(err, &notFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading SSM parameter %s: %w", paramPath, err)
	}
	raw := aws.ToString(out.Parameter.Value)
	if raw == "" {
		return nil, nil
	}
	parts := strings.Split(raw, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			result = append(result, t)
		}
	}
	return result, nil
}

// ssmPutList writes entries as a comma-separated string to paramPath.
func ssmPutList(ctx context.Context, client ssmAPI, paramPath string, entries []string, onSuccess func(int)) error {
	_, err := client.PutParameter(ctx, &ssm.PutParameterInput{
		Name:      aws.String(paramPath),
		Value:     aws.String(strings.Join(entries, ",")),
		Type:      types.ParameterTypeString,
		Overwrite: aws.Bool(true),
	})
	if err != nil {
		return fmt.Errorf("writing SSM parameter %s: %w", paramPath, err)
	}
	onSuccess(len(entries))
	return nil
}
