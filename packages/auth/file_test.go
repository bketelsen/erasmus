package auth_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/bketelsen/erasmus/packages/auth"
)

func TestFileStorePersists(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "auth.json")
	store := auth.NewFileStore(path)
	if err := store.Set(ctx, auth.Credential{Provider: "p", APIKey: "k"}); err != nil {
		t.Fatal(err)
	}
	reopened := auth.NewFileStore(path)
	cred, err := reopened.Get(ctx, "p")
	if err != nil {
		t.Fatal(err)
	}
	if cred.APIKey != "k" {
		t.Fatalf("api key = %q", cred.APIKey)
	}
	if err := reopened.Delete(ctx, "p"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Get(ctx, "p"); err == nil {
		t.Fatal("expected deleted credential to be missing")
	}
}
