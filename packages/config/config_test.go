package config_test

import (
	"context"
	"path/filepath"
	"testing"

	"erasmus/packages/config"
	"erasmus/packages/model"
)

func TestLoadSaveConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	ctx := context.Background()
	cfg := config.Config{Provider: "fake", Model: "echo", Tools: []string{"read"}}
	if err := config.Save(ctx, path, cfg); err != nil {
		t.Fatal(err)
	}
	got, err := config.Load(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Provider != "fake" || got.Model != "echo" || len(got.Tools) != 1 || got.Tools[0] != "read" {
		t.Fatalf("config = %+v", got)
	}
}

func TestLoadMissingReturnsDefaults(t *testing.T) {
	got, err := config.Load(context.Background(), filepath.Join(t.TempDir(), "missing.json"))
	if err != nil {
		t.Fatal(err)
	}
	if got.Provider != "fake" || got.Model != "echo" {
		t.Fatalf("config = %+v", got)
	}
}

func TestLoadSaveConfigPreservesModelOverrides(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	ctx := context.Background()
	cfg := config.Config{
		Provider: "openai-codex",
		Model:    "codex-custom",
		Models: []model.Model{
			{Provider: "openai-codex", ID: "codex-custom", DisplayName: "Custom Codex", ContextWindow: 1234, MaxOutput: 567, Reasoning: true, Source: "user"},
		},
	}
	if err := config.Save(ctx, path, cfg); err != nil {
		t.Fatal(err)
	}
	got, err := config.Load(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Models) != 1 {
		t.Fatalf("models = %+v, want one override", got.Models)
	}
	m := got.Models[0]
	if m.Provider != "openai-codex" || m.ID != "codex-custom" || m.DisplayName != "Custom Codex" || !m.Reasoning || m.Source != "user" {
		t.Fatalf("model override = %+v", m)
	}
}
