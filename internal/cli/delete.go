package cli

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

func newDeleteCommand() *cobra.Command {
	var purge bool

	cmd := &cobra.Command{
		Use:   "delete <name>",
		Short: "Delete a PicoClaw instance",
		Long:  "Delete a PicoClaw instance by removing its Deployment, Service, and Secret. The PVC is preserved unless --purge is specified.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]

			if err := k8sClient.DeleteInstance(context.Background(), name); err != nil {
				return fmt.Errorf("delete failed: %w", err)
			}

			fmt.Printf("Instance %s deleted\n", name)

			if purge {
				color.Yellow("WARNING: --purge will permanently destroy all data stored in the PVC for instance %q.", name)
				color.Red("This action cannot be undone.")
				fmt.Print("Confirm? [y/N]: ")

				reader := bufio.NewReader(os.Stdin)
				answer, err := reader.ReadString('\n')
				if err != nil {
					return fmt.Errorf("reading confirmation: %w", err)
				}
				answer = strings.TrimSpace(strings.ToLower(answer))

				if answer != "y" {
					fmt.Println("Aborted. PVC retained.")
					return nil
				}

				if err := k8sClient.DeletePVC(context.Background(), name); err != nil {
					return fmt.Errorf("purge PVC failed: %w", err)
				}
				fmt.Printf("PVC for instance %s deleted\n", name)
			} else {
				fmt.Printf("Note: PVC for instance %q is retained. Use --purge to remove it.\n", name)
			}

			return nil
		},
	}

	cmd.Flags().BoolVar(&purge, "purge", false, "Also delete the PVC (permanent data loss)")

	return cmd
}
