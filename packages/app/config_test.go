package app_test

import (
	"context"
	"path/filepath"
	"testing"

	"erasmus/packages/app"
	"erasmus/packages/config"
)

func TestConfigServices(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "config.json")
	updated, err := app.ConfigSet(ctx, path, config.Config{Provider: "fake", Tools: []string{"read"}})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Provider != "fake" || len(updated.Tools) != 1 {
		t.Fatalf("updated = %+v", updated)
	}
	got, err := app.ConfigGet(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Provider != "fake" || len(got.Tools) != 1 || got.Tools[0] != "read" {
		t.Fatalf("got = %+v", got)
	}
}
