package cli

import (
	"context"
	"fmt"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

func newRestartCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "restart <name>",
		Short: "Restart a PicoClaw instance",
		Long:  "Trigger a rolling restart of the instance's pod. Useful for recovering from error states or picking up a new image.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]

			if err := k8sClient.RestartInstance(context.Background(), name); err != nil {
				return fmt.Errorf("restart failed: %w", err)
			}

			color.Green("Instance %s restarting", name)
			return nil
		},
	}
}
