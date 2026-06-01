package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"erasmus/packages/app"
)

func newExtensionCommand() *cobra.Command {
	return &cobra.Command{
		Use:                "extension [list|exec]",
		Short:              "Inspect and run extensions",
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return handleExtension(cmd, args)
		},
	}
}

func handleExtension(cmd *cobra.Command, args []string) error {
	if len(args) >= 2 && args[0] == "list" {
		return app.ExtensionListProcess(context.Background(), cmd.OutOrStdout(), args[1], args[2:]...)
	}
	if len(args) >= 3 && args[0] == "exec" {
		processArgs, commandName, input, err := parseExtensionExecArgs(args[2:])
		if err != nil {
			return err
		}
		return app.ExtensionExecProcess(context.Background(), cmd.OutOrStdout(), args[1], processArgs, commandName, input)
	}
	return fmt.Errorf("usage: erasmus extension list <command> [args...]\n   or: erasmus extension exec <process> [process-args...] -- <command> [input]")
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
