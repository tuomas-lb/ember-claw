package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"
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
			table.Header("NAME", "STATUS", "READY", "AGE")

			rows := make([][]string, 0, len(instances))
			for _, inst := range instances {
				status := "Pending"
				if inst.ReadyReplicas >= inst.DesiredReplicas && inst.DesiredReplicas > 0 {
					status = "Running"
				} else if inst.ReadyReplicas > 0 {
					status = "Degraded"
				}
				rows = append(rows, []string{
					inst.Name,
					status,
					fmt.Sprintf("%d/%d", inst.ReadyReplicas, inst.DesiredReplicas),
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
