package app_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bketelsen/erasmus/packages/app"
)

func TestDiscoverAndInvokeSkills(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, ".erasmus", "skills")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "review.md"), []byte("# Review code\nCheck things."), 0o644); err != nil {
		t.Fatal(err)
	}
	skills, err := app.DiscoverSkills(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if len(skills) != 1 || skills[0].Name != "review" {
		t.Fatalf("skills = %+v", skills)
	}
	prompt, err := app.InvokeSkill(context.Background(), root, "review", "main.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(prompt, "Use skill: review") || !strings.Contains(prompt, "main.go") {
		t.Fatalf("prompt = %q", prompt)
	}
}
