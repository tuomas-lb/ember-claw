package cli

import (
	"os"

	"github.com/tuomas-lb/ember-claw/internal/envfile"
	"github.com/tuomas-lb/ember-claw/internal/k8s"
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

			// Apply env var defaults for flags not explicitly set on the command line.
			applyEnvDefault(cmd, "namespace", "ECLAW_NAMESPACE")
			applyEnvDefault(cmd, "kubeconfig", "ECLAW_KUBECONFIG")

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
		newSetSecretCommand(),
		newSetRegistryCommand(),
		newRestartCommand(),
	)

	return root
}

// applyEnvDefault sets a flag's value from an env var if the flag was not explicitly set.
func applyEnvDefault(cmd *cobra.Command, flagName, envVar string) {
	f := cmd.Root().PersistentFlags().Lookup(flagName)
	if f == nil || f.Changed {
		return
	}
	if v := os.Getenv(envVar); v != "" {
		f.Value.Set(v)
	}
}
