package main

import (
	"context"

	"github.com/spf13/cobra"

	"erasmus/packages/app"
)

func newSessionsCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "sessions",
		Short: "List sessions",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return app.PrintSessions(context.Background(), cmd.OutOrStdout(), "")
		},
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "list [dir]",
		Short: "List sessions",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dir := ""
			if len(args) == 1 {
				dir = args[0]
			}
			return app.PrintSessions(context.Background(), cmd.OutOrStdout(), dir)
		},
	})
	return cmd
}

func newSessionCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "session",
		Short: "Inspect a session",
		Args:  cobra.NoArgs,
	}
	cmd.AddCommand(
		&cobra.Command{
			Use:   "show <path>",
			Short: "Show a session transcript",
			Args:  cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				return app.PrintSessionShow(context.Background(), cmd.OutOrStdout(), args[0])
			},
		},
		&cobra.Command{
			Use:   "tree <path>",
			Short: "Show a session tree",
			Args:  cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				return app.PrintSessionTree(context.Background(), cmd.OutOrStdout(), args[0])
			},
		},
	)
	return cmd
}
