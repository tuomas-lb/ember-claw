package cli

import (
	"context"
	"fmt"

	"github.com/tuomas-lb/ember-claw/internal/k8s"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

func newExposeCommand() *cobra.Command {
	var (
		exposeType string
		nodePort   int32
		host       string
		tls        bool
		issuer     string
		class      string
		path       string
	)

	cmd := &cobra.Command{
		Use:   "expose <name>",
		Short: "Expose a PicoClaw instance externally",
		Long:  "Create external access to an instance's HTTP port (8080) via NodePort, LoadBalancer, or Ingress.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]

			if exposeType == "" {
				return fmt.Errorf("--type is required: must be nodeport, loadbalancer, or ingress")
			}

			if exposeType == "ingress" && host == "" {
				return fmt.Errorf("--host is required for ingress type")
			}

			opts := k8s.ExposeOptions{
				Name:     name,
				Type:     exposeType,
				NodePort: nodePort,
				Host:     host,
				TLS:      tls,
				Issuer:   issuer,
				Class:    class,
				Path:     path,
			}

			result, err := k8sClient.ExposeInstance(context.Background(), opts)
			if err != nil {
				return fmt.Errorf("expose failed: %w", err)
			}

			color.Green("Instance %s exposed successfully", name)
			fmt.Printf("  Type: %s\n", result.Type)
			fmt.Printf("  URL:  %s\n", result.URL)
			if result.NodePort > 0 {
				fmt.Printf("  NodePort: %d\n", result.NodePort)
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&exposeType, "type", "", "Expose type: nodeport, loadbalancer, or ingress (required)")
	cmd.Flags().Int32Var(&nodePort, "port", 0, "Specific NodePort number (only for nodeport type)")
	cmd.Flags().StringVar(&host, "host", "", "Hostname for ingress (required for ingress type)")
	cmd.Flags().BoolVar(&tls, "tls", false, "Enable TLS via cert-manager (only for ingress type)")
	cmd.Flags().StringVar(&issuer, "issuer", "letsencrypt-prod", "cert-manager ClusterIssuer name")
	cmd.Flags().StringVar(&class, "class", "nginx", "Ingress class")
	cmd.Flags().StringVar(&path, "path", "/", "URL path prefix")

	_ = cmd.MarkFlagRequired("type")

	return cmd
}

func newUnexposeCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "unexpose <name>",
		Short: "Remove external access for a PicoClaw instance",
		Long:  "Delete the external Service and Ingress resources for the named instance.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]

			if err := k8sClient.UnexposeInstance(context.Background(), name); err != nil {
				return fmt.Errorf("unexpose failed: %w", err)
			}

			color.Green("External access removed for instance %s", name)
			return nil
		},
	}
}
