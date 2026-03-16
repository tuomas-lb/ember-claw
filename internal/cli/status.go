package cli

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
)

func newStatusCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status <name>",
		Short: "Show the status of a PicoClaw instance",
		Long:  "Display deployment status, pod phase, provider, model, and resource details for a PicoClaw instance.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]

			status, err := k8sClient.GetInstanceStatus(context.Background(), name)
			if err != nil {
				return fmt.Errorf("get status failed: %w", err)
			}

			fmt.Printf("%-16s %s\n", "Name:", status.Name)
			fmt.Printf("%-16s %s\n", "Deployment:", status.DeploymentName)
			fmt.Printf("%-16s %d/%d\n", "Ready:", status.ReadyReplicas, status.DesiredReplicas)
			fmt.Printf("%-16s %s\n", "Pod Phase:", status.PodPhase)
			fmt.Printf("%-16s %s\n", "Provider:", status.Provider)
			fmt.Printf("%-16s %s\n", "Model:", status.Model)
			fmt.Printf("%-16s %s\n", "Age:", formatAge(status.Age))

			return nil
		},
	}
	return cmd
}
