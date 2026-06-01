package main

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"erasmus/packages/config"
	"erasmus/packages/model"
)

func TestRootCommandVersionOutput(t *testing.T) {
	cmd := newRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"version"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if got, want := out.String(), "erasmus 0.1.0-dev\n"; got != want {
		t.Fatalf("version output = %q, want %q", got, want)
	}
}

func TestRootCommandVersionFlagOutput(t *testing.T) {
	cmd := newRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--version"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if got, want := out.String(), "erasmus 0.1.0-dev\n"; got != want {
		t.Fatalf("version flag output = %q, want %q", got, want)
	}
}

func TestRootCommandUsesCobraFlagsForPrimaryCommands(t *testing.T) {
	cmd := newRootCommand()
	for _, name := range []string{"run", "tui", "swarm"} {
		child, _, err := cmd.Find([]string{name})
		if err != nil {
			t.Fatalf("find %s: %v", name, err)
		}
		if child.DisableFlagParsing {
			t.Fatalf("%s still disables Cobra flag parsing", name)
		}
	}
}

func TestRootCommandConfigFlagSelectsConfigFile(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "custom-config.json")
	t.Setenv("ERASMUS_CONFIG_FILE", "")
	t.Setenv("ERASMUS_AUTH_FILE", "")
	cmd := newRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--config", cfgPath, "config", "set", "provider", "flag-provider"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"provider": "flag-provider"`) {
		t.Fatalf("config set output = %q", out.String())
	}
}

func TestConfigGetMergesViperEnvironment(t *testing.T) {
	t.Setenv("ERASMUS_CONFIG_FILE", filepath.Join(t.TempDir(), "config.json"))
	t.Setenv("ERASMUS_PROVIDER", "env-provider")
	t.Setenv("ERASMUS_MODEL", "env-model")
	cmd := newRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"config", "get"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	if !strings.Contains(got, `"provider": "env-provider"`) || !strings.Contains(got, `"model": "env-model"`) {
		t.Fatalf("config get output = %q", got)
	}
}

func TestRootCommandAuthFileFlagSelectsAuthStore(t *testing.T) {
	authPath := filepath.Join(t.TempDir(), "custom-auth.json")
	t.Setenv("ERASMUS_CONFIG_FILE", filepath.Join(t.TempDir(), "config.json"))
	t.Setenv("ERASMUS_AUTH_FILE", "")
	cmd := newRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--auth-file", authPath, "login", "fake", "key"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "saved credentials for fake") {
		t.Fatalf("login output = %q", out.String())
	}

	out.Reset()
	cmd = newRootCommand()
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--auth-file", authPath, "auth", "status"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "fake") {
		t.Fatalf("auth status output = %q", out.String())
	}
}

func TestModelsCommandListsCurrentCodexDefault(t *testing.T) {
	cmd := newRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"models"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	if !strings.Contains(got, "openai-codex/gpt-5.5") || !strings.Contains(got, "openai-codex/gpt-5.4-mini") {
		t.Fatalf("models output = %q", got)
	}
}

func TestModelsCommandIncludesConfiguredModelOverrides(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "config.json")
	t.Setenv("ERASMUS_CONFIG_FILE", "")
	t.Setenv("ERASMUS_AUTH_FILE", "")
	if err := config.Save(context.Background(), cfgPath, config.Config{Models: []model.Model{
		{Provider: "openai-codex", ID: "codex-custom", DisplayName: "Custom Codex", Source: "user"},
	}}); err != nil {
		t.Fatal(err)
	}

	cmd := newRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--config", cfgPath, "models"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); !strings.Contains(got, "openai-codex/codex-custom\tCustom Codex") {
		t.Fatalf("models output = %q", got)
	}
}

func TestModelsCommandIncludesCachedModels(t *testing.T) {
	root := t.TempDir()
	t.Setenv("ERASMUS_CONFIG_FILE", "")
	t.Setenv("ERASMUS_AUTH_FILE", "")
	t.Setenv("XDG_CACHE_HOME", filepath.Join(root, "cache"))
	cache := model.NewFileCache(filepath.Join(root, "cache", "erasmus", "models.json"))
	if err := cache.PutProvider(context.Background(), "openai-codex", []model.Model{
		{Provider: "openai-codex", ID: "gpt-5.6-codex", DisplayName: "GPT-5.6 Codex"},
	}, time.Now()); err != nil {
		t.Fatal(err)
	}

	cmd := newRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"models"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); !strings.Contains(got, "openai-codex/gpt-5.6-codex\tGPT-5.6 Codex") {
		t.Fatalf("models output = %q", got)
	}
}

func TestDefaultStorageUsesXDGDirectories(t *testing.T) {
	root := t.TempDir()
	t.Setenv("ERASMUS_CONFIG_FILE", "")
	t.Setenv("ERASMUS_AUTH_FILE", "")
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(root, "data"))
	newRootCommand()
	if got, want := configPath(), filepath.Join(root, "config", "erasmus", "config.json"); got != want {
		t.Fatalf("configPath() = %q, want %q", got, want)
	}
	if got, want := authStorePath(), filepath.Join(root, "data", "erasmus", "auth.json"); got != want {
		t.Fatalf("authStorePath() = %q, want %q", got, want)
	}
}
