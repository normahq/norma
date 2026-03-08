package main

import "github.com/spf13/cobra"

func playgroundCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:          "playground",
		Short:        "Experimental playground commands for agent integrations",
		SilenceUsage: true,
	}
	cmd.AddCommand(playgroundGeminiACPCmd())
	return cmd
}
