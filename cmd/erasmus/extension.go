package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/bketelsen/erasmus/packages/app"
)

func newExtensionCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "extension",
		Short: "Inspect and run extensions",
		RunE: func(cmd *cobra.Command, args []string) error {
			return extensionUsageError()
		},
	}
	cmd.AddCommand(newExtensionDoctorCommand(), newExtensionListCommand(), newExtensionExecCommand())
	return cmd
}

func newExtensionDoctorCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Diagnose configured extensions",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig(context.Background())
			if err != nil {
				return err
			}
			return app.ExtensionDoctorConfigured(context.Background(), cmd.OutOrStdout(), cfg)
		},
	}
}

func newExtensionListCommand() *cobra.Command {
	return &cobra.Command{
		Use:                "list <command> [args...]",
		Short:              "Inspect one extension process",
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) < 1 {
				return extensionUsageError()
			}
			return app.ExtensionListProcess(context.Background(), cmd.OutOrStdout(), args[0], args[1:]...)
		},
	}
}

func newExtensionExecCommand() *cobra.Command {
	return &cobra.Command{
		Use:                "exec <process> [process-args...] -- <command> [input]",
		Short:              "Run one registered extension command",
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) < 2 {
				return extensionUsageError()
			}
			processArgs, commandName, input, err := parseExtensionExecArgs(args[1:])
			if err != nil {
				return err
			}
			return app.ExtensionExecProcess(context.Background(), cmd.OutOrStdout(), args[0], processArgs, commandName, input)
		},
	}
}

func extensionUsageError() error {
	return fmt.Errorf("usage: erasmus extension doctor\n   or: erasmus extension list <command> [args...]\n   or: erasmus extension exec <process> [process-args...] -- <command> [input]")
}

func parseExtensionExecArgs(args []string) (processArgs []string, commandName string, input string, err error) {
	sep := -1
	for i, arg := range args {
		if arg == "--" {
			sep = i
			break
		}
	}
	if sep < 0 || sep+1 >= len(args) {
		return nil, "", "", fmt.Errorf("usage: erasmus extension exec <process> [process-args...] -- <command> [input]")
	}
	processArgs = append([]string(nil), args[:sep]...)
	commandName = args[sep+1]
	input = strings.Join(args[sep+2:], " ")
	return processArgs, commandName, input, nil
}
