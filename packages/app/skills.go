package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"erasmus/packages/skill"
)

// SkillDirs returns configured skill discovery directories.
func SkillDirs(cwd string) []string {
	var dirs []string
	if env := os.Getenv("ERASMUS_SKILL_DIR"); env != "" {
		dirs = append(dirs, strings.Split(env, string(os.PathListSeparator))...)
	}
	if cwd == "" {
		if got, err := os.Getwd(); err == nil {
			cwd = got
		}
	}
	if cwd != "" {
		dirs = append(dirs, filepath.Join(cwd, ".erasmus", "skills"))
	}
	return dirs
}

// DiscoverSkills discovers skills from the default app locations.
func DiscoverSkills(ctx context.Context, cwd string) ([]skill.Skill, error) {
	return skill.Discover(ctx, SkillDirs(cwd)...)
}

// InvokeSkill discovers a skill and formats an invocation prompt.
func InvokeSkill(ctx context.Context, cwd, name, input string) (string, error) {
	if name == "" {
		return "", fmt.Errorf("skill name is required")
	}
	skills, err := DiscoverSkills(ctx, cwd)
	if err != nil {
		return "", err
	}
	for _, s := range skills {
		if s.Name == name {
			return skill.FormatInvocation(s, input), nil
		}
	}
	return "", fmt.Errorf("skill %q not found", name)
}
