package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"github.com/tuomas-lb/ember-claw/internal/mtls"
)

func newMTLSCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:         "mtls",
		Annotations: map[string]string{"skipClient": "true"},
		Short:       "Generate mTLS CA and client certificates for protecting web interfaces",
		Long: `Generate the mutual-TLS material used to protect eclaw web interfaces
(dashboard, backlog, instance UIs) behind nginx client-certificate auth.

The CA cert (ca.crt) is what the ingress verifies clients against — pass it to
'eclaw dashboard deploy --mtls-ca ca.crt' or 'eclaw expose --mtls-ca ca.crt'.
The client bundle (client.p12) is imported into your browser / OS keychain.`,
	}
	cmd.AddCommand(newMTLSInitCommand())
	return cmd
}

func newMTLSInitCommand() *cobra.Command {
	var (
		caName   string
		clientCN string
		org      string
		outDir   string
		days     int
		p12Pass  string
	)
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Create a CA + client certificate bundle",
		Long: `Create a self-signed CA and a client certificate signed by it, writing
ca.crt, ca.key, client.crt, client.key and client.p12 to the output directory.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ca, err := mtls.NewCA(caName, org, days)
			if err != nil {
				return err
			}
			client, err := ca.NewClient(clientCN, org, days, p12Pass)
			if err != nil {
				return err
			}
			if err := os.MkdirAll(outDir, 0o755); err != nil {
				return fmt.Errorf("create out dir: %w", err)
			}
			files := map[string][]byte{
				"ca.crt":     ca.CertPEM,
				"ca.key":     ca.KeyPEM,
				"client.crt": client.CertPEM,
				"client.key": client.KeyPEM,
				"client.p12": client.P12,
			}
			for name, data := range files {
				mode := os.FileMode(0o644)
				if name == "ca.key" || name == "client.key" || name == "client.p12" {
					mode = 0o600
				}
				if err := os.WriteFile(filepath.Join(outDir, name), data, mode); err != nil {
					return fmt.Errorf("write %s: %w", name, err)
				}
			}
			green := color.New(color.FgGreen).SprintFunc()
			fmt.Printf("%s mTLS bundle written to %s/\n", green("✓"), outDir)
			fmt.Printf("  ca.crt      — give to the ingress: eclaw dashboard deploy --mtls-ca %s/ca.crt\n", outDir)
			fmt.Printf("  client.p12  — import into your browser / OS keychain (CN=%s)\n", clientCN)
			if p12Pass == "" {
				fmt.Printf("  (client.p12 has no import password)\n")
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&caName, "ca-name", "eclaw mTLS CA", "CA common name")
	cmd.Flags().StringVar(&clientCN, "client", "operator", "Client certificate common name (identity)")
	cmd.Flags().StringVar(&org, "org", "eclaw", "Organization on the certificates")
	cmd.Flags().StringVar(&outDir, "out", ".", "Output directory for the generated files")
	cmd.Flags().IntVar(&days, "days", 3650, "Validity in days")
	cmd.Flags().StringVar(&p12Pass, "p12-password", "", "Export password for client.p12 (default: none)")
	return cmd
}
