package cli

import (
	"fmt"
	"os"

	"github.com/LastBotInc/ember-claw/internal/providers"
	"github.com/fatih/color"
	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"
)

func newModelsCommand() *cobra.Command {
	var (
		provider string
		apiKey   string
	)

	cmd := &cobra.Command{
		Use:   "models",
		Short: "List available models from a provider",
		Long: `List available models from an AI provider. This also validates
your API key — if the key is invalid, you'll get an authentication error.

Supported providers: openai, gemini, anthropic, groq, deepseek, openrouter`,
		RunE: func(cmd *cobra.Command, args []string) error {
			models, err := providers.ListModels(cmd.Context(), provider, apiKey)
			if err != nil {
				return fmt.Errorf("list models: %w", err)
			}

			if len(models) == 0 {
				fmt.Println("No models found.")
				return nil
			}

			color.Green("API key valid. %d models available:\n", len(models))

			table := tablewriter.NewTable(os.Stdout)
			table.Header("MODEL ID", "DISPLAY NAME")

			rows := make([][]string, 0, len(models))
			for _, m := range models {
				rows = append(rows, []string{m.ID, m.DisplayName})
			}
			if err := table.Bulk(rows); err != nil {
				return fmt.Errorf("table bulk: %w", err)
			}

			return table.Render()
		},
	}

	cmd.Flags().StringVar(&provider, "provider", "", "AI provider (openai, gemini, anthropic, groq, deepseek, openrouter)")
	cmd.Flags().StringVar(&apiKey, "api-key", "", "API key for the provider")

	_ = cmd.MarkFlagRequired("provider")
	_ = cmd.MarkFlagRequired("api-key")

	return cmd
}
