package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"
	"github.com/tuomas-lb/ember-claw/internal/k8s"
)

func newListCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List all managed PicoClaw instances",
		Long:  "List all PicoClaw instances managed by eclaw in the configured namespace.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			instances, err := k8sClient.ListInstances(context.Background())
			if err != nil {
				return fmt.Errorf("list failed: %w", err)
			}

			if len(instances) == 0 {
				fmt.Println("No PicoClaw instances found")
				return nil
			}

			table := tablewriter.NewTable(os.Stdout)
			table.Header("NAME", "STATUS", "READY", "RESTARTS", "AGE")

			rows := make([][]string, 0, len(instances))
			for _, inst := range instances {
				status := instanceStatus(inst)
				rows = append(rows, []string{
					inst.Name,
					status,
					fmt.Sprintf("%d/%d", inst.ReadyReplicas, inst.DesiredReplicas),
					fmt.Sprintf("%d", inst.Restarts),
					formatAge(inst.Age),
				})
			}
			if err := table.Bulk(rows); err != nil {
				return fmt.Errorf("table bulk: %w", err)
			}

			return table.Render()
		},
	}
	return cmd
}

// instanceStatus returns a human-readable status string from container-level info.
// Priority: container waiting reason > container state > pod phase > deployment ready count.
func instanceStatus(inst k8s.InstanceSummary) string {
	// Container-level state is the most accurate (CrashLoopBackOff, ImagePullBackOff, etc.)
	if inst.ContainerState != "" && inst.ContainerState != "Running" {
		return inst.ContainerState
	}
	if inst.ContainerState == "Running" && inst.ReadyReplicas >= inst.DesiredReplicas && inst.DesiredReplicas > 0 {
		return "Running"
	}
	if inst.PodPhase == "Running" {
		return "Running"
	}
	if inst.ReadyReplicas >= inst.DesiredReplicas && inst.DesiredReplicas > 0 {
		return "Running"
	}
	if inst.ReadyReplicas > 0 {
		return "Degraded"
	}
	if inst.PodPhase != "" {
		return string(inst.PodPhase)
	}
	return "Pending"
}
