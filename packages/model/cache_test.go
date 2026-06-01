package model_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"erasmus/packages/model"
)

func TestFileCacheRoundTripsProviderModels(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "models.json")
	cache := model.NewFileCache(path)
	discoveredAt := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	err := cache.PutProvider(ctx, "openai-codex", []model.Model{
		{Provider: "openai-codex", ID: "gpt-5.6-codex", DisplayName: "GPT-5.6 Codex", Source: "live"},
	}, discoveredAt)
	if err != nil {
		t.Fatal(err)
	}

	got, err := model.NewFileCache(path).ListProvider(ctx, "openai-codex")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("models = %+v", got)
	}
	if got[0].ID != "gpt-5.6-codex" || got[0].Source != "cache" || got[0].DiscoveredAt != discoveredAt {
		t.Fatalf("cached model = %+v", got[0])
	}
}

func TestFileCacheMissingFileIsEmpty(t *testing.T) {
	got, err := model.NewFileCache(filepath.Join(t.TempDir(), "missing.json")).List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("models = %+v, want none", got)
	}
}
