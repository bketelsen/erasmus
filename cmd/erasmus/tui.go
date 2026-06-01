package main

import (
	"context"
	"os"

	"github.com/spf13/cobra"

	"github.com/bketelsen/erasmus/packages/app"
)

func newTUICommand() *cobra.Command {
	var opts app.TUIOptions
	cmd := &cobra.Command{
		Use:   "tui [flags]",
		Short: "Start terminal UI",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig(context.Background())
			if err != nil {
				return err
			}
			opts.In = os.Stdin
			opts.Out = cmd.OutOrStdout()
			return app.RunTUIConfigured(context.Background(), opts, cfg, authStore())
		},
	}
	cmd.Flags().StringVar(&opts.SessionPath, "session", "", "durable JSONL session path")
	cmd.Flags().BoolVar(&opts.MemorySession, "memory", false, "use an in-memory session")
	cmd.Flags().StringVar(&opts.Theme, "theme", "", "theme override")
	return cmd
}
