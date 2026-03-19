package cli

import (
	"context"
	"fmt"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

func newSetSecretCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "set-secret <name> <key> <value>",
		Short: "Set an environment variable in an instance's Secret",
		Long: `Add or update a key-value pair in the instance's Kubernetes Secret.
The instance pod is automatically restarted to pick up the change.

Examples:
  eclaw set-secret test-claw-1 TELEGRAM_BOT_TOKEN abc123
  eclaw set-secret research LINEAR_API_KEY lin_api_xxx
  eclaw set-secret my-agent SLACK_BOT_TOKEN xoxb-xxx`,
		Args: cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			instanceName := args[0]
			key := args[1]
			value := args[2]

			if err := k8sClient.SetSecret(context.Background(), instanceName, key, value); err != nil {
				return fmt.Errorf("set secret failed: %w", err)
			}

			color.Green("Secret %s set on %s (pod restarting)", key, instanceName)
			return nil
		},
	}
	return cmd
}
