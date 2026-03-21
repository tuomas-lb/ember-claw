package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/tuomas-lb/ember-claw/internal/k8s"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

// validInstanceName matches a valid DNS subdomain component for a PicoClaw instance name.
var validInstanceName = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,61}[a-z0-9]$`)

// envDefault returns the flag value if non-empty, otherwise the env var value.
func envDefault(flagVal, envVar string) string {
	if flagVal != "" {
		return flagVal
	}
	return os.Getenv(envVar)
}

// resolveDefaultImage builds the default container image reference from IMAGE_REGISTRY.
// Resolution order: IMAGE_REGISTRY env var > first registry in ~/.docker/config.json auths.
// Returns empty string if no registry is configured (deploy will fail with a helpful error).
func resolveDefaultImage() string {
	registry := os.Getenv("IMAGE_REGISTRY")
	if registry == "" {
		registry = detectDockerRegistry()
	}
	if registry == "" {
		return ""
	}
	return registry + "/" + k8s.DefaultServiceName + ":" + k8s.DefaultImageTag
}

// detectDockerRegistry reads ~/.docker/config.json and returns the first configured
// registry from the auths section, or empty string if none found.
func detectDockerRegistry() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	data, err := os.ReadFile(home + "/.docker/config.json")
	if err != nil {
		return ""
	}
	var cfg struct {
		Auths map[string]any `json:"auths"`
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return ""
	}
	for host := range cfg.Auths {
		// Skip Docker Hub (too generic to be the right registry).
		if host == "https://index.docker.io/v1/" || host == "docker.io" {
			continue
		}
		return host
	}
	return ""
}

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
		linearAPIKey  string
		linearTeamID  string
		slackBotToken string
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

			// Apply env var defaults for flags not explicitly set.
			provider = envDefault(provider, "ECLAW_PROVIDER")
			model = envDefault(model, "ECLAW_MODEL")
			if image == "" {
				image = envDefault("", "ECLAW_IMAGE")
			}
			// If ECLAW_IMAGE has no registry prefix (no "/"), prepend IMAGE_REGISTRY.
			if image != "" && !strings.Contains(image, "/") {
				registry := os.Getenv("IMAGE_REGISTRY")
				if registry != "" {
					image = registry + "/" + image
				}
			}
			if image == "" {
				image = resolveDefaultImage()
			}
			if image == "" {
				return fmt.Errorf("container image required: set IMAGE_REGISTRY or ECLAW_IMAGE in .env, or use --image flag")
			}

			if provider == "" {
				return fmt.Errorf("provider required: use --provider or set ECLAW_PROVIDER in .env")
			}
			if model == "" {
				return fmt.Errorf("model required: use --model or set ECLAW_MODEL in .env")
			}

			resolvedKey, err := resolveAPIKey(apiKey, provider)
			if err != nil {
				return err
			}

			opts := k8s.DeployOptions{
				Name:          name,
				Provider:      provider,
				APIKey:        resolvedKey,
				Model:         model,
				Image:         image,
				CPURequest:    cpuRequest,
				CPULimit:      cpuLimit,
				MemoryRequest: memoryRequest,
				MemoryLimit:   memoryLimit,
				StorageSize:   storageSize,
				StorageClass:  storageClass,
				CustomEnv:     customEnv,
				LinearAPIKey:  envDefault(linearAPIKey, "LINEAR_API_KEY"),
				LinearTeamID:  envDefault(linearTeamID, "LINEAR_TEAM_ID"),
				SlackBotToken: envDefault(slackBotToken, "SLACK_BOT_TOKEN"),
			}

			if err := k8sClient.DeployInstance(context.Background(), opts); err != nil {
				return fmt.Errorf("deploy failed: %w", err)
			}

			color.Green("Instance %s deployed successfully", name)
			return nil
		},
	}

	cmd.Flags().StringVar(&provider, "provider", "", "AI provider (or ECLAW_PROVIDER env)")
	cmd.Flags().StringVar(&apiKey, "api-key", "", "API key (or <PROVIDER>_API_KEY env)")
	cmd.Flags().StringVar(&model, "model", "", "Model identifier (or ECLAW_MODEL env)")
	cmd.Flags().StringVar(&image, "image", "", "Container image (or ECLAW_IMAGE / IMAGE_REGISTRY env)")
	cmd.Flags().StringVar(&cpuRequest, "cpu-request", "100m", "CPU request for the instance pod")
	cmd.Flags().StringVar(&cpuLimit, "cpu-limit", "500m", "CPU limit for the instance pod")
	cmd.Flags().StringVar(&memoryRequest, "memory-request", "128Mi", "Memory request for the instance pod")
	cmd.Flags().StringVar(&memoryLimit, "memory-limit", "512Mi", "Memory limit for the instance pod")
	cmd.Flags().StringVar(&storageSize, "storage-size", "1Gi", "PVC storage size")
	cmd.Flags().StringVar(&storageClass, "storage-class", "", "Storage class for the PVC (uses cluster default if empty)")
	cmd.Flags().StringToStringVar(&customEnv, "env", nil, "Additional environment variables (key=value pairs, can be repeated)")

	// Integration tool flags (optional — also read from env vars)
	cmd.Flags().StringVar(&linearAPIKey, "linear-api-key", "", "Linear API key (or LINEAR_API_KEY env)")
	cmd.Flags().StringVar(&linearTeamID, "linear-team-id", "", "Linear team UUID (or LINEAR_TEAM_ID env)")
	cmd.Flags().StringVar(&slackBotToken, "slack-bot-token", "", "Slack bot token (or SLACK_BOT_TOKEN env)")

	return cmd
}
