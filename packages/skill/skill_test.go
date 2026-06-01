package skill_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"erasmus/packages/skill"
)

func TestDiscoverAndFormatInvocation(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "review.md"), []byte("# Review code\nCheck correctness."), 0o644); err != nil {
		t.Fatal(err)
	}
	skills, err := skill.Discover(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(skills) != 1 {
		t.Fatalf("skills len = %d", len(skills))
	}
	if skills[0].Name != "review" || skills[0].Description != "Review code" || skills[0].Body != "Check correctness." {
		t.Fatalf("skill = %+v", skills[0])
	}
	prompt := skill.FormatInvocation(skills[0], "main.go")
	if !strings.Contains(prompt, "Use skill: review") || !strings.Contains(prompt, "main.go") {
		t.Fatalf("prompt = %q", prompt)
	}
}
