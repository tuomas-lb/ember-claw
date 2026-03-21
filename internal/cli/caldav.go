package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

// newSetCalDAVCommand creates the "set-caldav" command for configuring CalDAV calendars.
func newSetCalDAVCommand() *cobra.Command {
	var (
		url      string
		username string
		password string
		calName  string
		remove   bool
	)

	cmd := &cobra.Command{
		Use:   "set-caldav <instance>",
		Short: "Add or remove a CalDAV calendar for an instance",
		Long: `Configures a CalDAV calendar as an MCP server in the instance's config.
Each calendar gets its own MCP server entry (caldav-<name>).

Multiple calendars can be added by running this command multiple times
with different --name values.

Examples:
  eclaw set-caldav watcher-1 --name work \
    --url https://caldav.example.com/dav \
    --username user@example.com \
    --password secret

  eclaw set-caldav watcher-1 --name personal \
    --url https://caldav.icloud.com \
    --username appleid@icloud.com \
    --password app-specific-password

  eclaw set-caldav watcher-1 --name work --remove`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			instance := args[0]

			if calName == "" {
				return fmt.Errorf("--name is required (e.g., 'work', 'personal')")
			}

			ctx := context.Background()
			green := color.New(color.FgGreen).SprintFunc()

			// Pull existing config.
			raw, err := k8sClient.PullConfig(ctx, instance)
			if err != nil {
				return fmt.Errorf("pull config for %s: %w", instance, err)
			}

			var cfg map[string]interface{}
			if err := json.Unmarshal(raw, &cfg); err != nil {
				return fmt.Errorf("parse config: %w", err)
			}

			// Navigate to tools.mcp.servers
			tools, _ := cfg["tools"].(map[string]interface{})
			if tools == nil {
				tools = map[string]interface{}{}
				cfg["tools"] = tools
			}
			mcp, _ := tools["mcp"].(map[string]interface{})
			if mcp == nil {
				mcp = map[string]interface{}{"enabled": true, "discovery": map[string]interface{}{"enabled": false}}
				tools["mcp"] = mcp
			}
			servers, _ := mcp["servers"].(map[string]interface{})
			if servers == nil {
				servers = map[string]interface{}{}
				mcp["servers"] = servers
			}

			serverKey := "caldav-" + strings.ToLower(calName)

			if remove {
				delete(servers, serverKey)
				mcp["servers"] = servers

				updated, _ := json.MarshalIndent(cfg, "", "  ")
				if err := k8sClient.PushConfig(ctx, instance, updated); err != nil {
					return fmt.Errorf("push config: %w", err)
				}
				fmt.Printf("%s CalDAV calendar %q removed from %s (pod restarting)\n", green("✓"), calName, instance)
				return nil
			}

			// Adding — validate required fields.
			if url == "" {
				return fmt.Errorf("--url is required (CalDAV server URL)")
			}
			if username == "" {
				return fmt.Errorf("--username is required")
			}
			if password == "" {
				return fmt.Errorf("--password is required")
			}

			servers[serverKey] = map[string]interface{}{
				"enabled": true,
				"type":    "stdio",
				"command": "npx",
				"args":    []string{"caldav-mcp"},
				"env": map[string]string{
					"CALDAV_BASE_URL": url,
					"CALDAV_USERNAME": username,
					"CALDAV_PASSWORD": password,
				},
			}
			mcp["servers"] = servers

			updated, _ := json.MarshalIndent(cfg, "", "  ")
			if err := k8sClient.PushConfig(ctx, instance, updated); err != nil {
				return fmt.Errorf("push config: %w", err)
			}

			fmt.Printf("%s CalDAV calendar %q configured for %s (pod restarting)\n", green("✓"), calName, instance)
			fmt.Printf("  Server: %s\n", url)
			fmt.Printf("  User:   %s\n", username)
			fmt.Printf("  MCP key: %s\n", serverKey)
			return nil
		},
	}

	cmd.Flags().StringVar(&calName, "name", "", "Calendar name (e.g., 'work', 'personal') — used as MCP server key")
	cmd.Flags().StringVar(&url, "url", "", "CalDAV server URL (e.g., https://caldav.example.com/dav)")
	cmd.Flags().StringVar(&username, "username", "", "CalDAV username")
	cmd.Flags().StringVar(&password, "password", "", "CalDAV password")
	cmd.Flags().BoolVar(&remove, "remove", false, "Remove the named calendar instead of adding")

	return cmd
}
