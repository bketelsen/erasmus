package prompt_test

import (
	"context"
	"strings"
	"testing"

	"erasmus/packages/prompt"
	"erasmus/packages/skill"
)

func TestStaticBuilderIncludesSkills(t *testing.T) {
	got, err := (prompt.StaticBuilder{}).Build(context.Background(), prompt.Input{Skills: []skill.Skill{{Name: "review", Description: "Review code"}}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "review: Review code") {
		t.Fatalf("prompt = %q", got)
	}
}
