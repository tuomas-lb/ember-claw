package cli

import (
	"github.com/LastBotInc/ember-claw/internal/envfile"
	"github.com/LastBotInc/ember-claw/internal/k8s"
	"github.com/spf13/cobra"
)

// k8sClient is the package-level Kubernetes client, initialized in PersistentPreRunE.
var k8sClient *k8s.Client

// NewRootCommand creates the root eclaw Cobra command with persistent flags and all subcommands.
func NewRootCommand() *cobra.Command {
	var kubeconfig string
	var namespace string

	root := &cobra.Command{
		Use:   "eclaw",
		Short: "Manage PicoClaw instances on Kubernetes",
		Long:  "eclaw is a CLI for deploying and managing PicoClaw AI instances on Kubernetes.",
		// PersistentPreRunE initialises the k8s client for every subcommand that
		// needs it (all except help and completion).
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			// Load .env file (does not override existing env vars).
			_ = envfile.Load(".env")

			// Skip client creation for commands that don't need cluster access.
			switch cmd.Name() {
			case "help", "completion", "models":
				return nil
			}
			var err error
			k8sClient, err = k8s.NewClient(kubeconfig, namespace)
			return err
		},
	}

	root.PersistentFlags().StringVar(&kubeconfig, "kubeconfig", "", "Path to kubeconfig file (defaults to KUBECONFIG env var, then ~/.kube/config)")
	root.PersistentFlags().StringVar(&namespace, "namespace", "picoclaw", "Kubernetes namespace for PicoClaw instances")

	root.AddCommand(
		newDeployCommand(),
		newListCommand(),
		newDeleteCommand(),
		newStatusCommand(),
		newLogsCommand(),
		newChatCommand(),
		newModelsCommand(),
	)

	return root
}
