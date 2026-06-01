package model_test

import (
	"slices"
	"testing"

	"github.com/bketelsen/erasmus/packages/model"
)

func TestDefaultCatalogCodexModels(t *testing.T) {
	catalog := model.DefaultCatalog()
	defaultModel, err := catalog.Default("openai-codex")
	if err != nil {
		t.Fatal(err)
	}
	if defaultModel.ID != "gpt-5.5" {
		t.Fatalf("openai-codex default = %q, want gpt-5.5", defaultModel.ID)
	}
	want := []string{"gpt-5.5", "gpt-5.4", "gpt-5.4-mini", "gpt-5.4-nano", "gpt-5.3-codex", "gpt-5.2", "gpt-5.1"}
	var got []string
	for _, m := range catalog.ListProvider("openai-codex") {
		got = append(got, m.ID)
	}
	if !slices.Equal(got, want) {
		t.Fatalf("openai-codex models = %v, want %v", got, want)
	}
}

func TestDefaultCatalogGitHubCopilotModels(t *testing.T) {
	catalog := model.DefaultCatalog()
	defaultModel, err := catalog.Default("github-copilot")
	if err != nil {
		t.Fatal(err)
	}
	if defaultModel.ID != "gpt-5.5" {
		t.Fatalf("github-copilot default = %q, want gpt-5.5", defaultModel.ID)
	}
	want := []string{
		"claude-haiku-4.5",
		"claude-opus-4.5",
		"claude-opus-4.6",
		"claude-opus-4.7",
		"claude-opus-4.8",
		"claude-sonnet-4.5",
		"claude-sonnet-4.6",
		"gemini-2.5-pro",
		"gemini-3-flash-preview",
		"gemini-3.1-pro-preview",
		"gemini-3.5-flash",
		"gpt-4.1",
		"gpt-4o",
		"gpt-5-mini",
		"gpt-5.2",
		"gpt-5.2-codex",
		"gpt-5.3-codex",
		"gpt-5.4",
		"gpt-5.4-mini",
		"gpt-5.5",
		"grok-code-fast-1",
	}
	var got []string
	for _, m := range catalog.ListProvider("github-copilot") {
		got = append(got, m.ID)
	}
	if !slices.Equal(got, want) {
		t.Fatalf("github-copilot models = %v, want %v", got, want)
	}
	for _, id := range []string{"claude-sonnet-4.5", "gpt-5.3-codex", "gpt-5.5"} {
		m, err := catalog.Find("github-copilot", id)
		if err != nil {
			t.Fatal(err)
		}
		if !m.Reasoning || m.ContextWindow == 0 || m.MaxOutput == 0 || m.Source != "builtin" {
			t.Fatalf("bad metadata for %s: %+v", id, m)
		}
	}
}
