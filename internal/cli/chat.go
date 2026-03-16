package cli

import (
	"context"
	"fmt"
	"io"

	"github.com/chzyer/readline"
	"github.com/spf13/cobra"

	emberclaw "github.com/LastBotInc/ember-claw/gen/emberclaw/v1"
	"github.com/LastBotInc/ember-claw/internal/grpcclient"
	"github.com/LastBotInc/ember-claw/internal/k8s"
)

func newChatCommand() *cobra.Command {
	var message string

	cmd := &cobra.Command{
		Use:   "chat <name>",
		Short: "Chat with a running PicoClaw instance",
		Long: `Chat with a running PicoClaw instance.

Without --message, opens an interactive readline prompt (Ctrl+C or Ctrl+D to exit).
With --message, sends a single query and prints the response.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runChat(cmd.Context(), k8sClient, args[0], message)
		},
	}

	cmd.Flags().StringVarP(&message, "message", "m", "", "Single-shot message (non-interactive)")

	return cmd
}

// runChat is the core implementation of the chat subcommand.
// It wires FindRunningPod -> PortForwardPod -> DialSidecar -> interactive or single-shot.
func runChat(ctx context.Context, client *k8s.Client, instanceName, message string) error {
	// Step 1: Find the running pod for the named instance.
	fmt.Printf("Connecting to %s...\n", instanceName)

	podName, err := client.FindRunningPod(ctx, instanceName)
	if err != nil {
		return fmt.Errorf("instance %q not found or not running: %w", instanceName, err)
	}

	// Step 2: Establish in-process port-forward to the sidecar's gRPC port.
	pf, err := client.PortForwardPod(ctx, podName, 50051)
	if err != nil {
		return fmt.Errorf("port-forward to %q failed: %w", instanceName, err)
	}
	defer close(pf.StopChan)

	// Step 3: Dial gRPC via the forwarded local port.
	svcClient, grpcConn, err := grpcclient.DialSidecar(ctx, pf.LocalPort)
	if err != nil {
		return fmt.Errorf("gRPC dial failed: %w", err)
	}
	defer grpcConn.Close()

	if message != "" {
		// Step 4a: Single-shot mode (-m flag).
		return runSingleShot(ctx, svcClient, instanceName, message)
	}

	// Step 4b: Interactive mode.
	return runInteractive(ctx, svcClient, instanceName)
}

// runSingleShot sends one Query RPC and prints the response.
func runSingleShot(ctx context.Context, svcClient emberclaw.PicoClawServiceClient, instanceName, message string) error {
	resp, err := svcClient.Query(ctx, &emberclaw.QueryRequest{
		Message:    message,
		SessionKey: instanceName,
	})
	if err != nil {
		return fmt.Errorf("query failed: %w", err)
	}
	if resp.Error != "" {
		return fmt.Errorf("instance error: %s", resp.Error)
	}
	fmt.Println(resp.Text)
	return nil
}

// runInteractive opens a readline REPL and streams messages via the Chat bidi RPC.
func runInteractive(ctx context.Context, svcClient emberclaw.PicoClawServiceClient, instanceName string) error {
	rl, err := readline.New(fmt.Sprintf("[%s]> ", instanceName))
	if err != nil {
		return fmt.Errorf("readline init failed: %w", err)
	}
	defer rl.Close()

	stream, err := svcClient.Chat(ctx)
	if err != nil {
		return fmt.Errorf("failed to open chat stream: %w", err)
	}
	defer stream.CloseSend() //nolint:errcheck

	fmt.Println("Connected. Type messages or Ctrl+C to exit.")

	for {
		line, err := rl.Readline()
		if err == readline.ErrInterrupt || err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("readline error: %w", err)
		}
		if line == "" {
			continue
		}

		if err := stream.Send(&emberclaw.ChatRequest{
			Message:    line,
			SessionKey: instanceName,
		}); err != nil {
			return fmt.Errorf("send failed: %w", err)
		}

		resp, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("recv failed: %w", err)
		}
		fmt.Println(resp.Text)
	}

	return nil
}
