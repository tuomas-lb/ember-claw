package cli

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

func newKubeconfigCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "kubeconfig",
		Short: "Manage kubeconfig for eclaw",
		Long: `Manage the kubeconfig used by eclaw.

Kubeconfig resolution order:
  1. --kubeconfig flag (explicit path)
  2. ECLAW_KUBECONFIG env var / .env file
  3. KUBECONFIG env var (standard kubectl behavior)
  4. KUBECONFIG_BASE64 env var (base64-encoded, for CI/automation)
  5. ~/.kube/config (default)`,
	}

	cmd.AddCommand(newKubeconfigSetCommand())
	cmd.AddCommand(newKubeconfigShowCommand())

	return cmd
}

func newKubeconfigSetCommand() *cobra.Command {
	var outputPath string

	cmd := &cobra.Command{
		Use:   "set <BASE64_STRING>",
		Short: "Decode and save a base64-encoded kubeconfig",
		Long: `Decode a base64-encoded kubeconfig string and save it to disk.

This is useful for CI/automation or when you receive a kubeconfig as a
base64 string (e.g., from Rancher, cloud providers, or CI secrets).

The decoded kubeconfig is saved to ~/.kube/config by default, or to the
path specified by --output.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			b64 := args[0]

			decoded, err := base64.StdEncoding.DecodeString(b64)
			if err != nil {
				return fmt.Errorf("invalid base64: %w", err)
			}

			if outputPath == "" {
				home, err := os.UserHomeDir()
				if err != nil {
					return fmt.Errorf("cannot determine home directory: %w", err)
				}
				outputPath = filepath.Join(home, ".kube", "config")
			}

			// Ensure parent directory exists.
			dir := filepath.Dir(outputPath)
			if err := os.MkdirAll(dir, 0700); err != nil {
				return fmt.Errorf("cannot create directory %s: %w", dir, err)
			}

			// Write with restrictive permissions (owner-only read/write).
			if err := os.WriteFile(outputPath, decoded, 0600); err != nil {
				return fmt.Errorf("cannot write kubeconfig: %w", err)
			}

			green := color.New(color.FgGreen).SprintFunc()
			fmt.Printf("%s Kubeconfig saved to %s\n", green("✓"), outputPath)
			return nil
		},
	}

	cmd.Flags().StringVarP(&outputPath, "output", "o", "", "Output path (default: ~/.kube/config)")

	return cmd
}

func newKubeconfigShowCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "show",
		Short: "Show which kubeconfig eclaw would use",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Check resolution order and report which one would be used.
			checks := []struct {
				label string
				path  string
			}{
				{"--kubeconfig flag", cmd.Root().PersistentFlags().Lookup("kubeconfig").Value.String()},
				{"ECLAW_KUBECONFIG env", os.Getenv("ECLAW_KUBECONFIG")},
				{"KUBECONFIG env", os.Getenv("KUBECONFIG")},
				{"KUBECONFIG_BASE64 env", os.Getenv("KUBECONFIG_BASE64")},
			}

			for _, c := range checks {
				if c.path != "" {
					label := c.label
					if c.label == "KUBECONFIG_BASE64 env" {
						fmt.Printf("  %s: %s (base64, %d bytes)\n", label, "set", len(c.path))
					} else {
						fmt.Printf("  %s: %s\n", label, c.path)
					}
				}
			}

			// Check default path.
			home, _ := os.UserHomeDir()
			defaultPath := filepath.Join(home, ".kube", "config")
			if _, err := os.Stat(defaultPath); err == nil {
				fmt.Printf("  default: %s (exists)\n", defaultPath)
			} else {
				fmt.Printf("  default: %s (not found)\n", defaultPath)
			}

			// Show which would actually be used.
			fmt.Println()
			active := resolveKubeconfig(cmd)
			green := color.New(color.FgGreen).SprintFunc()
			fmt.Printf("%s Active kubeconfig: %s\n", green("→"), active)

			return nil
		},
	}
}

// resolveKubeconfig returns the kubeconfig path that would be used, following resolution order.
func resolveKubeconfig(cmd *cobra.Command) string {
	if f := cmd.Root().PersistentFlags().Lookup("kubeconfig"); f != nil && f.Changed {
		return f.Value.String()
	}
	if v := os.Getenv("ECLAW_KUBECONFIG"); v != "" {
		return v
	}
	if v := os.Getenv("KUBECONFIG"); v != "" {
		return v
	}
	if v := os.Getenv("KUBECONFIG_BASE64"); v != "" {
		return "(decoded from KUBECONFIG_BASE64)"
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".kube", "config")
}
