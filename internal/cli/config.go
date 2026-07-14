package cli

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

// newConfigCommand creates the "config" command with pull/push subcommands.
func newConfigCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Manage PicoClaw instance configuration",
		Long:  "Pull, push, or edit the raw config.json for a PicoClaw instance.",
	}

	cmd.AddCommand(newConfigPullCommand())
	cmd.AddCommand(newConfigPushCommand())

	return cmd
}

// newConfigPullCommand creates the "config pull" subcommand.
func newConfigPullCommand() *cobra.Command {
	var outputFile string

	cmd := &cobra.Command{
		Use:   "pull <name>",
		Short: "Download instance config.json to a local file",
		Long:  "Retrieves the config.json from the instance's K8s secret and writes it locally.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]

			ctx := context.Background()
			raw, err := k8sClient.PullConfig(ctx, name)
			if err != nil {
				return fmt.Errorf("pull config for %s: %w", name, err)
			}

			if outputFile == "" {
				outputFile = name + "-config.json"
			}

			if err := os.WriteFile(outputFile, raw, 0644); err != nil {
				return fmt.Errorf("write %s: %w", outputFile, err)
			}

			green := color.New(color.FgGreen).SprintFunc()
			fmt.Printf("%s Config saved to %s\n", green("✓"), outputFile)
			fmt.Printf("  Edit the file, then push with: eclaw config push %s %s\n", name, outputFile)
			return nil
		},
	}

	cmd.Flags().StringVarP(&outputFile, "output", "o", "", "Output file path (default: <name>-config.json)")
	return cmd
}

// newConfigPushCommand creates the "config push" subcommand.
func newConfigPushCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "push <name> [file]",
		Short: "Upload a local config.json to an instance and restart",
		Long: `Writes the config.json to the instance's K8s secret and triggers a pod restart.
If no file is specified, reads from <name>-config.json.`,
		Args: cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			inputFile := name + "-config.json"
			if len(args) > 1 {
				inputFile = args[1]
			}

			raw, err := os.ReadFile(inputFile)
			if err != nil {
				return fmt.Errorf("read %s: %w", inputFile, err)
			}

			ctx := context.Background()
			if err := k8sClient.PushConfig(ctx, name, raw); err != nil {
				return fmt.Errorf("push config for %s: %w", name, err)
			}

			green := color.New(color.FgGreen).SprintFunc()
			fmt.Printf("%s Config pushed to %s (pod restarting)\n", green("✓"), name)
			return nil
		},
	}

	return cmd
}

// newSetTelegramCommand creates the "set-telegram" convenience command.
func newSetTelegramCommand() *cobra.Command {
	var token string
	var allowFrom []string

	cmd := &cobra.Command{
		Use:   "set-telegram <name>",
		Short: "Configure Telegram channel for an instance",
		Long: `Patches the instance's config.json to enable the Telegram channel with the
specified bot token and allowed user IDs, then restarts the pod.

Get your bot token from @BotFather on Telegram.
Get your user ID from @userinfobot on Telegram.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]

			if token == "" {
				return fmt.Errorf("--token is required (get it from @BotFather on Telegram)")
			}
			if len(allowFrom) == 0 {
				return fmt.Errorf("--allow-from is required (your Telegram user ID from @userinfobot)")
			}

			// Flatten comma-separated values.
			var userIDs []string
			for _, v := range allowFrom {
				for _, id := range strings.Split(v, ",") {
					id = strings.TrimSpace(id)
					if id != "" {
						userIDs = append(userIDs, id)
					}
				}
			}

			ctx := context.Background()
			if err := k8sClient.SetTelegram(ctx, name, token, userIDs); err != nil {
				return fmt.Errorf("set telegram for %s: %w", name, err)
			}

			green := color.New(color.FgGreen).SprintFunc()
			fmt.Printf("%s Telegram configured for %s (pod restarting)\n", green("✓"), name)
			fmt.Printf("  Bot token: %s...%s\n", token[:4], token[len(token)-4:])
			fmt.Printf("  Allowed users: %s\n", strings.Join(userIDs, ", "))
			return nil
		},
	}

	cmd.Flags().StringVar(&token, "token", "", "Telegram bot token from @BotFather (required)")
	cmd.Flags().StringSliceVar(&allowFrom, "allow-from", nil, "Telegram user IDs allowed to use the bot (required, comma-separated)")

	return cmd
}

// newSetWhatsAppCommand creates the "set-whatsapp" convenience command.
func newSetWhatsAppCommand() *cobra.Command {
	var allowFrom []string

	cmd := &cobra.Command{
		Use:   "set-whatsapp <name>",
		Short: "Configure the native WhatsApp channel for an instance",
		Long: `Patches the instance's config.json to enable the native WhatsApp channel
(whatsmeow) and restarts the pod. The session store lives on the instance PVC so
the linked device survives restarts.

Requires a sidecar image built with the "whatsapp_native" build tag.

After the pod restarts, run 'eclaw logs <name>' and scan the QR code with
WhatsApp on your phone (Settings → Linked Devices → Link a device).

Note: native WhatsApp uses the unofficial WhatsApp Web protocol. Link a
dedicated number, not a personal/business-critical one — it may be banned.

--allow-from takes phone numbers (with country code, digits only) permitted to
message the bot.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			if len(allowFrom) == 0 {
				return fmt.Errorf("--allow-from is required (phone numbers allowed to message the bot, e.g. 358401234567)")
			}

			var numbers []string
			for _, v := range allowFrom {
				for _, id := range strings.Split(v, ",") {
					id = strings.TrimSpace(id)
					if id != "" {
						numbers = append(numbers, id)
					}
				}
			}

			ctx := context.Background()
			if err := k8sClient.SetWhatsApp(ctx, name, numbers); err != nil {
				return fmt.Errorf("set whatsapp for %s: %w", name, err)
			}

			green := color.New(color.FgGreen).SprintFunc()
			fmt.Printf("%s WhatsApp (native) configured for %s (pod restarting)\n", green("✓"), name)
			fmt.Printf("  Allowed numbers: %s\n", strings.Join(numbers, ", "))
			fmt.Printf("  Next: run 'eclaw logs %s' and scan the QR code from WhatsApp → Linked Devices.\n", name)
			return nil
		},
	}

	cmd.Flags().StringSliceVar(&allowFrom, "allow-from", nil, "Phone numbers allowed to message the bot (required, comma-separated, country code + digits)")

	return cmd
}
