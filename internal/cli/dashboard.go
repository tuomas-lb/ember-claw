package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"github.com/tuomas-lb/ember-claw/internal/k8s"
)

func newDashboardCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "dashboard",
		Short: "Deploy and manage the fleet dashboard web interface",
		Long: `Deploy the PicoClaw fleet dashboard — the web control plane that lists,
deploys, chats with, and streams logs from instances in a namespace. Protect it
with mTLS client certs (see 'eclaw mtls init') and optional Postgres-backed chat
history.`,
	}
	cmd.AddCommand(newDashboardDeployCommand(), newDashboardDeleteCommand())
	return cmd
}

func newDashboardDeployCommand() *cobra.Command {
	var (
		host         string
		image        string
		sidecarImage string
		issuer       string
		class        string
		mtlsCA       string
		withPostgres bool
		storageClass string
	)
	cmd := &cobra.Command{
		Use:   "deploy",
		Short: "Deploy (or update) the fleet dashboard in the target namespace",
		Long: `Deploy the fleet dashboard with a namespace-scoped ServiceAccount/Role,
Service, and Ingress. Recommended: protect it with mTLS via --mtls-ca (the ca.crt
from 'eclaw mtls init') and enable chat persistence with --with-postgres.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if host == "" {
				return fmt.Errorf("--host is required")
			}
			resolvedImage := k8s.DashboardImageDefault(image)
			if resolvedImage == "" {
				return fmt.Errorf("dashboard image required: use --image or set ECLAW_DASHBOARD_IMAGE / IMAGE_REGISTRY")
			}
			// The image the dashboard deploys for NEW instances defaults to the
			// sidecar image in the same registry as the dashboard.
			if sidecarImage == "" {
				sidecarImage = envDefault("", "SIDECAR_IMAGE")
			}
			if sidecarImage == "" {
				sidecarImage = resolveDefaultImage()
			}

			var caPEM []byte
			if mtlsCA != "" {
				data, err := os.ReadFile(mtlsCA)
				if err != nil {
					return fmt.Errorf("read mtls ca: %w", err)
				}
				caPEM = data
			}

			opts := k8s.DashboardOptions{
				Host:         host,
				Image:        resolvedImage,
				SidecarImage: sidecarImage,
				Issuer:       issuer,
				Class:        class,
				MTLSCAPEM:    caPEM,
				WithPostgres: withPostgres,
				StorageClass: storageClass,
			}
			if err := k8sClient.DeployDashboard(context.Background(), opts); err != nil {
				return fmt.Errorf("deploy dashboard: %w", err)
			}
			green := color.New(color.FgGreen).SprintFunc()
			fmt.Printf("%s Dashboard deployed\n", green("✓"))
			fmt.Printf("  URL:   https://%s\n", host)
			if len(caPEM) > 0 {
				fmt.Printf("  Auth:  mTLS client cert required (import client.p12)\n")
			} else {
				color.Yellow("  Warning: no --mtls-ca given — the dashboard has NO client-cert auth and exposes deploy/delete/secret operations. Protect it before exposing publicly.")
			}
			if withPostgres {
				fmt.Printf("  Chat history: persisted in Postgres\n")
			}
			fmt.Printf("  Point DNS for %s at your ingress, then wait for the TLS cert to issue.\n", host)
			return nil
		},
	}
	cmd.Flags().StringVar(&host, "host", "", "Ingress hostname for the dashboard (required)")
	cmd.Flags().StringVar(&image, "image", "", "Dashboard image (or ECLAW_DASHBOARD_IMAGE / IMAGE_REGISTRY)")
	cmd.Flags().StringVar(&sidecarImage, "sidecar-image", "", "Image the dashboard deploys for new instances (or SIDECAR_IMAGE / IMAGE_REGISTRY)")
	cmd.Flags().StringVar(&issuer, "issuer", "letsencrypt-prod", "cert-manager ClusterIssuer for the public TLS cert")
	cmd.Flags().StringVar(&class, "class", "nginx", "Ingress class")
	cmd.Flags().StringVar(&mtlsCA, "mtls-ca", "", "Path to a CA cert (ca.crt) for client-certificate auth (from 'eclaw mtls init')")
	cmd.Flags().BoolVar(&withPostgres, "with-postgres", false, "Deploy Postgres and enable chat history persistence")
	cmd.Flags().StringVar(&storageClass, "storage-class", "", "Storage class for the Postgres PVC (cluster default if empty)")
	return cmd
}

func newDashboardDeleteCommand() *cobra.Command {
	var withPostgres bool
	cmd := &cobra.Command{
		Use:   "delete",
		Short: "Delete the fleet dashboard from the target namespace",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := k8sClient.DeleteDashboard(context.Background(), withPostgres); err != nil {
				return fmt.Errorf("delete dashboard: %w", err)
			}
			color.Green("Dashboard deleted (namespace %s)", namespaceFlag(cmd))
			if withPostgres {
				fmt.Println("  Postgres Deployment/Service removed; PVC + secret retained (chat history).")
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&withPostgres, "with-postgres", false, "Also remove the Postgres Deployment/Service (PVC retained)")
	return cmd
}

// namespaceFlag returns the effective --namespace value for messaging.
func namespaceFlag(cmd *cobra.Command) string {
	if f := cmd.Root().PersistentFlags().Lookup("namespace"); f != nil {
		return f.Value.String()
	}
	return ""
}
