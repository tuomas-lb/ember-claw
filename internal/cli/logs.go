package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
)

func newLogsCommand() *cobra.Command {
	var follow bool
	var tail int64

	cmd := &cobra.Command{
		Use:   "logs <name>",
		Short: "Stream logs from a PicoClaw instance",
		Long:  "Fetch or stream logs from the running pod of a PicoClaw instance.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]

			// Use a cancellable context so Ctrl+C causes a clean exit.
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			// Cancel context on interrupt or termination signals.
			sigs := make(chan os.Signal, 1)
			signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
			go func() {
				<-sigs
				cancel()
			}()

			rc, err := k8sClient.GetInstanceLogs(ctx, name, follow, tail)
			if err != nil {
				return fmt.Errorf("get logs failed: %w", err)
			}
			defer rc.Close()

			if _, err := io.Copy(os.Stdout, rc); err != nil {
				// Ignore context-cancelled errors — they are caused by Ctrl+C.
				if ctx.Err() != nil {
					return nil
				}
				return fmt.Errorf("streaming logs: %w", err)
			}
			return nil
		},
	}

	cmd.Flags().BoolVarP(&follow, "follow", "f", false, "Stream (tail) the logs continuously")
	cmd.Flags().Int64Var(&tail, "tail", 100, "Number of recent log lines to show")

	return cmd
}
