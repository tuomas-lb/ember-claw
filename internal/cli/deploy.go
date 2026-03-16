package cli

import (
	"context"
	"fmt"
	"regexp"

	"github.com/LastBotInc/ember-claw/internal/k8s"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

// validInstanceName matches a valid DNS subdomain component for a PicoClaw instance name.
var validInstanceName = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,61}[a-z0-9]$`)

func newDeployCommand() *cobra.Command {
	var (
		provider      string
		apiKey        string
		model         string
		image         string
		cpuRequest    string
		cpuLimit      string
		memoryRequest string
		memoryLimit   string
		storageSize   string
		storageClass  string
		customEnv     map[string]string
	)

	cmd := &cobra.Command{
		Use:   "deploy <name>",
		Short: "Deploy a new PicoClaw instance",
		Long:  "Deploy a PicoClaw instance with the given name onto the configured Kubernetes cluster.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]

			if !validInstanceName.MatchString(name) {
				return fmt.Errorf("invalid instance name %q: must match ^[a-z0-9][a-z0-9-]{0,61}[a-z0-9]$", name)
			}

			opts := k8s.DeployOptions{
				Name:          name,
				Provider:      provider,
				APIKey:        apiKey,
				Model:         model,
				Image:         image,
				CPURequest:    cpuRequest,
				CPULimit:      cpuLimit,
				MemoryRequest: memoryRequest,
				MemoryLimit:   memoryLimit,
				StorageSize:   storageSize,
				StorageClass:  storageClass,
				CustomEnv:     customEnv,
			}

			if err := k8sClient.DeployInstance(context.Background(), opts); err != nil {
				return fmt.Errorf("deploy failed: %w", err)
			}

			color.Green("Instance %s deployed successfully", name)
			return nil
		},
	}

	cmd.Flags().StringVar(&provider, "provider", "", "AI provider (e.g. anthropic, openai)")
	cmd.Flags().StringVar(&apiKey, "api-key", "", "API key for the AI provider")
	cmd.Flags().StringVar(&model, "model", "", "Model identifier (e.g. claude-opus-4-5)")
	cmd.Flags().StringVar(&image, "image", "reg.r.lastbot.com/ember-claw-sidecar:latest", "Container image for the sidecar")
	cmd.Flags().StringVar(&cpuRequest, "cpu-request", "100m", "CPU request for the instance pod")
	cmd.Flags().StringVar(&cpuLimit, "cpu-limit", "500m", "CPU limit for the instance pod")
	cmd.Flags().StringVar(&memoryRequest, "memory-request", "128Mi", "Memory request for the instance pod")
	cmd.Flags().StringVar(&memoryLimit, "memory-limit", "512Mi", "Memory limit for the instance pod")
	cmd.Flags().StringVar(&storageSize, "storage-size", "1Gi", "PVC storage size")
	cmd.Flags().StringVar(&storageClass, "storage-class", "", "Storage class for the PVC (uses cluster default if empty)")
	cmd.Flags().StringToStringVar(&customEnv, "env", nil, "Additional environment variables (key=value pairs, can be repeated)")

	_ = cmd.MarkFlagRequired("provider")
	_ = cmd.MarkFlagRequired("api-key")
	_ = cmd.MarkFlagRequired("model")

	return cmd
}
