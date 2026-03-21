package cli

import (
	"context"
	"fmt"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

func newSetRegistryCommand() *cobra.Command {
	var username string
	var password string

	cmd := &cobra.Command{
		Use:   "set-registry <server>",
		Short: "Configure registry pull credentials for the namespace",
		Long: `Create or update a Kubernetes docker-registry secret so pods can pull
images from a private container registry. The secret is automatically
referenced by all subsequent deploys.

Example:
  eclaw set-registry registry.example.com --username admin --password secret`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			server := args[0]
			if username == "" || password == "" {
				return fmt.Errorf("--username and --password are required")
			}

			if err := k8sClient.SetRegistryCredentials(context.Background(), server, username, password); err != nil {
				return fmt.Errorf("set registry credentials: %w", err)
			}

			color.Green("Registry credentials for %s configured", server)
			fmt.Println("All future deploys will use these credentials to pull images.")
			return nil
		},
	}

	cmd.Flags().StringVar(&username, "username", "", "Registry username")
	cmd.Flags().StringVar(&password, "password", "", "Registry password or token")

	return cmd
}
