package cli

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

// gmailMailbox represents a single mailbox entry inside GMAIL_MCP_CONFIG.
type gmailMailbox struct {
	Name     string `json:"name"`
	Email    string `json:"email"`
	Password string `json:"password"`
	Host     string `json:"host,omitempty"`
	Port     int    `json:"port,omitempty"`
}

// gmailMCPConfig is the JSON structure stored in the GMAIL_MCP_CONFIG env var.
type gmailMCPConfig struct {
	Mailboxes []gmailMailbox `json:"mailboxes"`
}

// newSetGmailCommand creates the "set-gmail" command for configuring Gmail mailboxes.
func newSetGmailCommand() *cobra.Command {
	var (
		name     string
		email    string
		password string
		remove   bool
	)

	cmd := &cobra.Command{
		Use:   "set-gmail <instance>",
		Short: "Add or remove a Gmail mailbox for an instance",
		Long: `Configures a Gmail mailbox in the instance's gmail MCP server config.
All mailboxes share a single "gmail" MCP server entry. The server reads
mailbox credentials from the GMAIL_MCP_CONFIG environment variable.

Multiple mailboxes can be added by running this command multiple times
with different --name values.

Examples:
  eclaw set-gmail watcher-1 --name work \
    --email user@gmail.com \
    --password abcd-efgh-ijkl-mnop

  eclaw set-gmail watcher-1 --name personal \
    --email me@gmail.com \
    --password xxxx-xxxx-xxxx-xxxx

  eclaw set-gmail watcher-1 --name work --remove`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			instance := args[0]

			if name == "" {
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

			// Navigate to tools.mcp.servers (create path if missing).
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

			const serverKey = "gmail"

			// Parse existing GMAIL_MCP_CONFIG from the server entry (if any).
			var gmailCfg gmailMCPConfig
			if existing, ok := servers[serverKey].(map[string]interface{}); ok {
				if envMap, ok := existing["env"].(map[string]interface{}); ok {
					if cfgStr, ok := envMap["GMAIL_MCP_CONFIG"].(string); ok {
						_ = json.Unmarshal([]byte(cfgStr), &gmailCfg)
					}
				}
			}

			if remove {
				// Remove the named mailbox.
				filtered := make([]gmailMailbox, 0, len(gmailCfg.Mailboxes))
				for _, mb := range gmailCfg.Mailboxes {
					if mb.Name != name {
						filtered = append(filtered, mb)
					}
				}

				if len(filtered) == 0 {
					// No mailboxes left — remove entire server entry.
					delete(servers, serverKey)
				} else {
					gmailCfg.Mailboxes = filtered
					cfgJSON, _ := json.Marshal(gmailCfg)
					serverEntry, _ := servers[serverKey].(map[string]interface{})
					envMap, _ := serverEntry["env"].(map[string]interface{})
					envMap["GMAIL_MCP_CONFIG"] = string(cfgJSON)
				}
				mcp["servers"] = servers

				updated, _ := json.MarshalIndent(cfg, "", "  ")
				if err := k8sClient.PushConfig(ctx, instance, updated); err != nil {
					return fmt.Errorf("push config: %w", err)
				}
				fmt.Printf("%s Gmail mailbox %q removed from %s (pod restarting)\n", green("\u2713"), name, instance)
				return nil
			}

			// Adding — validate required fields.
			if email == "" {
				return fmt.Errorf("--email is required (Gmail address)")
			}
			if password == "" {
				return fmt.Errorf("--password is required (Gmail app password)")
			}

			// Add or update the mailbox entry by name.
			found := false
			for i, mb := range gmailCfg.Mailboxes {
				if mb.Name == name {
					gmailCfg.Mailboxes[i].Email = email
					gmailCfg.Mailboxes[i].Password = password
					found = true
					break
				}
			}
			if !found {
				gmailCfg.Mailboxes = append(gmailCfg.Mailboxes, gmailMailbox{
					Name:     name,
					Email:    email,
					Password: password,
				})
			}

			cfgJSON, _ := json.Marshal(gmailCfg)

			servers[serverKey] = map[string]interface{}{
				"enabled": true,
				"type":    "stdio",
				"command": "npx",
				"args":    []interface{}{"gmail-mcp"},
				"env": map[string]interface{}{
					"GMAIL_MCP_CONFIG": string(cfgJSON),
				},
			}
			mcp["servers"] = servers

			updated, _ := json.MarshalIndent(cfg, "", "  ")
			if err := k8sClient.PushConfig(ctx, instance, updated); err != nil {
				return fmt.Errorf("push config: %w", err)
			}

			fmt.Printf("%s Gmail mailbox %q configured for %s (pod restarting)\n", green("\u2713"), name, instance)
			fmt.Printf("  Email:   %s\n", email)
			fmt.Printf("  MCP key: %s\n", serverKey)
			return nil
		},
	}

	cmd.Flags().StringVar(&name, "name", "", "Mailbox name (e.g., 'work', 'personal') -- identifies this mailbox")
	cmd.Flags().StringVar(&email, "email", "", "Gmail address (e.g., user@gmail.com)")
	cmd.Flags().StringVar(&password, "password", "", "Gmail app password (16 characters)")
	cmd.Flags().BoolVar(&remove, "remove", false, "Remove the named mailbox instead of adding")

	return cmd
}
