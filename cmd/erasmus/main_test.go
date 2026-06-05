package main

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/bketelsen/erasmus/packages/auth"
	"github.com/bketelsen/erasmus/packages/config"
	"github.com/bketelsen/erasmus/packages/model"
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

func TestModelsRefreshFakePopulatesDefaultCache(t *testing.T) {
	root := t.TempDir()
	t.Setenv("ERASMUS_CONFIG_FILE", "")
	t.Setenv("ERASMUS_AUTH_FILE", "")
	t.Setenv("XDG_CACHE_HOME", filepath.Join(root, "cache"))

	cmd := newRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"models", "refresh", "fake"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); !strings.Contains(got, "refreshed 1 models for fake") {
		t.Fatalf("refresh output = %q", got)
	}

	out.Reset()
	cmd = newRootCommand()
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"models"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); !strings.Contains(got, "fake/echo\tFake Echo") {
		t.Fatalf("models output = %q", got)
	}
}

func TestModelsRefreshGitHubCopilotPopulatesDefaultCache(t *testing.T) {
	root := t.TempDir()
	t.Setenv("ERASMUS_CONFIG_FILE", "")
	t.Setenv("ERASMUS_AUTH_FILE", "")
	t.Setenv("XDG_CACHE_HOME", filepath.Join(root, "cache"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(root, "data"))

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/models" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"claude-sonnet-4.5"},{"id":"gpt-5.3-codex"}]}`))
	}))
	defer server.Close()
	oldDefaultClient := http.DefaultClient
	http.DefaultClient = server.Client()
	defer func() { http.DefaultClient = oldDefaultClient }()

	cmd := newRootCommand()
	if err := authStore().Set(context.Background(), auth.Credential{Provider: "github-copilot", OAuth: &auth.OAuthToken{AccessToken: "copilot-token;proxy-ep=" + strings.TrimPrefix(server.URL, "https://") + ";"}}); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"models", "refresh", "github-copilot"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); !strings.Contains(got, "refreshed 2 models for github-copilot") {
		t.Fatalf("refresh output = %q", got)
	}

	out.Reset()
	cmd = newRootCommand()
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"models"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	if !strings.Contains(got, "github-copilot/claude-sonnet-4.5\tClaude Sonnet 4.5") || !strings.Contains(got, "github-copilot/gpt-5.3-codex\tGPT-5.3-Codex") {
		t.Fatalf("models output = %q", got)
	}
}

func TestExtensionDoctorCommandUsesConfiguredExtensions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("script test")
	}
	root := t.TempDir()
	t.Setenv("ERASMUS_CONFIG_FILE", "")
	t.Setenv("ERASMUS_AUTH_FILE", "")
	t.Setenv("XDG_STATE_HOME", filepath.Join(root, "state"))
	extPath := filepath.Join(root, "doctor-ext.py")
	script := `#!/usr/bin/env python3
import json, sys
print(json.dumps({"type":"hello","data":{"name":"doctor-cli","version":"1"}}), flush=True)
print(json.dumps({"type":"register_tool","data":{"name":"echo_ext","description":"echo extension","schema":{"type":"object"}}}), flush=True)
for line in sys.stdin:
    pass
`
	if err := os.WriteFile(extPath, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	cfgPath := filepath.Join(root, "config.json")
	if err := config.Save(context.Background(), cfgPath, config.Config{Extensions: []config.ExtensionConfig{{Command: extPath}}}); err != nil {
		t.Fatal(err)
	}
	cmd := newRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--config", cfgPath, "extension", "doctor"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	for _, want := range []string{"extension 1\tOK\t" + extPath, "protocol\tdoctor-cli\t1", "tool\techo_ext\techo extension"} {
		if !strings.Contains(got, want) {
			t.Fatalf("doctor output missing %q:\n%s", want, got)
		}
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
