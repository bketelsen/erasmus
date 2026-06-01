package main

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/bketelsen/erasmus/packages/app"
	"github.com/bketelsen/erasmus/packages/auth"
	"github.com/bketelsen/erasmus/packages/config"
)

type cliSettings struct {
	v *viper.Viper
}

var activeSettings = newCLISettings()

func newCLISettings() *cliSettings {
	v := viper.New()
	v.SetEnvPrefix("ERASMUS")
	v.SetEnvKeyReplacer(strings.NewReplacer("-", "_", ".", "_"))
	v.AutomaticEnv()
	v.SetDefault("config_file", filepath.Join(xdgConfigHome(), "erasmus", "config.json"))
	v.SetDefault("auth_file", filepath.Join(xdgDataHome(), "erasmus", "auth.json"))
	_ = v.BindEnv("config_file", "ERASMUS_CONFIG_FILE")
	_ = v.BindEnv("auth_file", "ERASMUS_AUTH_FILE")
	return &cliSettings{v: v}
}

func bindRootSettings(root *cobra.Command) {
	activeSettings = newCLISettings()
	flags := root.PersistentFlags()
	flags.String("config", "", "config file path")
	flags.String("auth-file", "", "auth credentials file path")
	flags.String("provider", "", "provider override")
	flags.String("model", "", "model override")
	flags.String("reasoning", "", "reasoning override")
	flags.String("cwd", "", "working directory override")
	flags.StringSlice("tool", nil, "enabled tool override (repeatable or comma-separated)")
	flags.Bool("no-tools", false, "disable configured tools")

	_ = activeSettings.v.BindPFlag("config_file", flags.Lookup("config"))
	_ = activeSettings.v.BindPFlag("auth_file", flags.Lookup("auth-file"))
	_ = activeSettings.v.BindPFlag("provider", flags.Lookup("provider"))
	_ = activeSettings.v.BindPFlag("model", flags.Lookup("model"))
	_ = activeSettings.v.BindPFlag("reasoning", flags.Lookup("reasoning"))
	_ = activeSettings.v.BindPFlag("cwd", flags.Lookup("cwd"))
	_ = activeSettings.v.BindPFlag("tools", flags.Lookup("tool"))
	_ = activeSettings.v.BindPFlag("no_tools", flags.Lookup("no-tools"))
}

func loadConfig(ctx context.Context) (config.Config, error) {
	cfg, err := app.ConfigGet(ctx, configPath())
	if err != nil {
		return config.Config{}, err
	}
	return config.Merge(cfg, activeSettings.configOverrides()), nil
}

func (s *cliSettings) configOverrides() config.Config {
	var out config.Config
	if s.v.IsSet("provider") {
		out.Provider = s.v.GetString("provider")
	}
	if s.v.IsSet("model") {
		out.Model = s.v.GetString("model")
	}
	if s.v.IsSet("reasoning") {
		out.Reasoning = s.v.GetString("reasoning")
	}
	if s.v.IsSet("cwd") {
		out.CWD = s.v.GetString("cwd")
	}
	if s.v.IsSet("tools") {
		out.Tools = s.v.GetStringSlice("tools")
	}
	if s.v.IsSet("no_tools") {
		out.NoTools = s.v.GetBool("no_tools")
	}
	return out
}

func configPath() string {
	return activeSettings.v.GetString("config_file")
}

func authStorePath() string {
	return activeSettings.v.GetString("auth_file")
}

func authStore() auth.Store {
	return auth.NewFileStore(authStorePath())
}
