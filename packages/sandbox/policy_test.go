package sandbox_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/bketelsen/erasmus/packages/sandbox"
)

func TestResolveAllowsPathInsideRoot(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "file.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	p, err := sandbox.New(root)
	if err != nil {
		t.Fatal(err)
	}

	got, err := p.Resolve("file.txt")
	if err != nil {
		t.Fatal(err)
	}
	want, err := filepath.EvalSymlinks(filepath.Join(root, "file.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestResolveRejectsParentEscape(t *testing.T) {
	root := t.TempDir()
	p, err := sandbox.New(root)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := p.Resolve("../outside.txt"); err == nil {
		t.Fatal("expected escape error")
	}
}

func TestResolveRejectsSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "link")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	p, err := sandbox.New(root)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := p.Resolve("link/secret.txt"); err == nil {
		t.Fatal("expected symlink escape error")
	}
}
