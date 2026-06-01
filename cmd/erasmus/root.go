package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"erasmus/packages/app"
	"erasmus/packages/model"
)

func newRootCommand() *cobra.Command {
	root := &cobra.Command{
		Use:           "erasmus",
		Short:         "Go-native agent harness and terminal product",
		Version:       version,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return fmt.Errorf("no command specified\ntry: erasmus --version\ntry: erasmus run \"hello\"\ntry: erasmus models\ntry: erasmus skills\ntry: erasmus rpc < requests.jsonl\ntry: erasmus swarm run \"hello\"\ntry: erasmus tui")
		},
	}
	bindRootSettings(root)
	root.SetVersionTemplate("erasmus {{.Version}}\n")
	root.AddCommand(
		newVersionCommand(),
		newRunCommand(),
		newRPCCommand(),
		newSwarmCommand(),
		newTUICommand(),
		newSkillCommand(),
		newSkillsCommand(),
		newModelsCommand(),
		newSessionsCommand(),
		newSessionCommand(),
		newExtensionCommand(),
		newLoginCommand(),
		newLogoutCommand(),
		newConfigCommand(),
		newAuthCommand(),
	)
	return root
}

func newVersionCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version and exit",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			_, err := fmt.Fprintln(cmd.OutOrStdout(), "erasmus "+version)
			return err
		},
	}
}

func newRunCommand() *cobra.Command {
	var opts app.RunOptions
	cmd := &cobra.Command{
		Use:   "run [flags] <prompt>",
		Short: "Run one prompt",
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Prompt = strings.Join(args, " ")
			opts.Out = cmd.OutOrStdout()
			cfg, err := loadConfig(context.Background())
			if err != nil {
				return err
			}
			return app.RunConfigured(context.Background(), opts, cfg, authStore())
		},
	}
	addRunFlags(cmd, &opts)
	return cmd
}

func addRunFlags(cmd *cobra.Command, opts *app.RunOptions) {
	cmd.Flags().StringVar(&opts.SessionPath, "session", "", "durable JSONL session path")
	cmd.Flags().BoolVar(&opts.MemorySession, "memory", false, "use an in-memory session")
	cmd.Flags().BoolVar(&opts.ShowTools, "show-tools", false, "show concise tool execution markers")
}

func newRPCCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "rpc",
		Short: "Run JSON-lines RPC server",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig(context.Background())
			if err != nil {
				return err
			}
			return app.RunRPCConfigured(context.Background(), os.Stdin, cmd.OutOrStdout(), cfg, authStore())
		},
	}
}

func newSkillCommand() *cobra.Command {
	return &cobra.Command{
		Use:                "skill <name> [input]",
		Short:              "Invoke a skill through the fake provider",
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) < 1 {
				return fmt.Errorf("usage: erasmus skill <name> [input]")
			}
			prompt, err := app.InvokeSkill(context.Background(), "", args[0], strings.Join(args[1:], " "))
			if err != nil {
				return err
			}
			return app.RunFake(context.Background(), app.RunOptions{Prompt: prompt, Out: cmd.OutOrStdout()})
		},
	}
}

func newSkillsCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "skills",
		Short: "List discovered skills",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			skills, err := app.DiscoverSkills(context.Background(), "")
			if err != nil {
				return err
			}
			for _, s := range skills {
				fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\n", s.Name, s.Description)
			}
			return nil
		},
	}
}

func newModelsCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "models",
		Short: "List models",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()
			cfg, err := loadConfig(context.Background())
			if err != nil {
				return err
			}
			catalog, err := app.CatalogFromCache(ctx, cfg, model.NewFileCache(app.DefaultModelCachePath()), model.DefaultCatalog())
			if err != nil {
				return err
			}
			for _, m := range app.Models(catalog) {
				fmt.Fprintf(cmd.OutOrStdout(), "%s/%s\t%s\n", m.Provider, m.ID, m.DisplayName)
			}
			return nil
		},
	}
	cmd.AddCommand(newModelsRefreshCommand())
	return cmd
}

func newModelsRefreshCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "refresh [provider]",
		Short: "Refresh cached provider models",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			provider := "fake"
			if len(args) > 0 {
				provider = args[0]
			}
			models, err := app.RefreshModelCacheWithAuth(context.Background(), provider, model.NewFileCache(app.DefaultModelCachePath()), authStore())
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "refreshed %d models for %s\n", len(models), provider)
			return nil
		},
	}
}
