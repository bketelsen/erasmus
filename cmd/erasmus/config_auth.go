package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"erasmus/packages/app"
	"erasmus/packages/config"
)

func newLoginCommand() *cobra.Command {
	oauth := false
	cmd := &cobra.Command{
		Use:   "login <provider> [api-key]",
		Short: "Store provider credentials",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 1 && (args[0] == "openai-codex" || args[0] == "openai" && oauth) {
				if err := app.LoginOpenAICodexOAuth(context.Background(), authStore(), cmd.OutOrStdout()); err != nil {
					return err
				}
				fmt.Fprintln(cmd.OutOrStdout(), "saved OAuth credentials for openai-codex")
				return nil
			}
			if len(args) != 2 {
				return fmt.Errorf("usage: erasmus login <provider> <api-key>\n   or: erasmus login openai-codex")
			}
			if err := app.Login(context.Background(), authStore(), args[0], args[1]); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "saved credentials for", args[0])
			return nil
		},
	}
	cmd.Flags().BoolVar(&oauth, "oauth", false, "use OAuth flow for openai")
	return cmd
}

func newLogoutCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "logout <provider>",
		Short: "Remove provider credentials",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := app.Logout(context.Background(), authStore(), args[0]); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "removed credentials for", args[0])
			return nil
		},
	}
}

func newConfigCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Read or update config",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return configGet(cmd)
		},
	}
	cmd.AddCommand(
		&cobra.Command{
			Use:   "get",
			Short: "Print effective config",
			Args:  cobra.NoArgs,
			RunE: func(cmd *cobra.Command, args []string) error {
				return configGet(cmd)
			},
		},
		&cobra.Command{
			Use:   "set <key> <value>",
			Short: "Set a config value",
			Args:  cobra.ExactArgs(2),
			RunE: func(cmd *cobra.Command, args []string) error {
				return configSet(cmd, args[0], args[1])
			},
		},
	)
	return cmd
}

func newAuthCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auth",
		Short: "Inspect auth status",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return authStatus(cmd)
		},
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "status",
		Short: "Print auth status",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return authStatus(cmd)
		},
	})
	return cmd
}

func authStatus(cmd *cobra.Command) error {
	entries, err := app.AuthStatusDetails(context.Background(), authStore())
	if err != nil {
		return err
	}
	if len(entries) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "no credentials configured")
		return nil
	}
	for _, entry := range entries {
		fmt.Fprintln(cmd.OutOrStdout(), entry.String())
	}
	return nil
}

func configGet(cmd *cobra.Command) error {
	cfg, err := loadConfig(context.Background())
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	fmt.Fprintln(cmd.OutOrStdout(), string(data))
	return nil
}

func configSet(cmd *cobra.Command, key, value string) error {
	patch, err := configPatch(key, value)
	if err != nil {
		return err
	}
	cfg, err := app.ConfigSet(context.Background(), configPath(), patch)
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	fmt.Fprintln(cmd.OutOrStdout(), string(data))
	return nil
}

func configPatch(key, value string) (config.Config, error) {
	switch key {
	case "provider":
		return config.Config{Provider: value}, nil
	case "model":
		return config.Config{Model: value}, nil
	case "reasoning":
		return config.Config{Reasoning: value}, nil
	case "cwd":
		return config.Config{CWD: value}, nil
	case "theme":
		return config.Config{Theme: value}, nil
	case "tools":
		if value == "" {
			return config.Config{Tools: []string{}}, nil
		}
		return config.Config{Tools: strings.Split(value, ",")}, nil
	case "no_tools":
		return config.Config{NoTools: value == "true" || value == "1" || value == "yes"}, nil
	case "extension":
		return config.Config{Extensions: []config.ExtensionConfig{{Command: value}}}, nil
	case "extensions":
		if value == "" {
			return config.Config{Extensions: []config.ExtensionConfig{}}, nil
		}
		parts := strings.Split(value, ",")
		exts := make([]config.ExtensionConfig, 0, len(parts))
		for _, part := range parts {
			if strings.TrimSpace(part) != "" {
				exts = append(exts, config.ExtensionConfig{Command: strings.TrimSpace(part)})
			}
		}
		return config.Config{Extensions: exts}, nil
	default:
		return config.Config{}, fmt.Errorf("unknown config key %q", key)
	}
}
