// Package skill defines reusable prompt skills.
package skill

import (
	"context"
	"os"
	"path/filepath"
	"strings"
)

// Skill is a named prompt resource.
type Skill struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Body        string `json:"body"`
	Source      string `json:"source,omitempty"`
}

// Discover loads markdown skills from dirs. File basename is the skill name.
func Discover(ctx context.Context, dirs ...string) ([]Skill, error) {
	var out []Skill
	for _, dir := range dirs {
		if dir == "" {
			continue
		}
		entries, err := os.ReadDir(dir)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, err
		}
		for _, entry := range entries {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			if entry.IsDir() || filepath.Ext(entry.Name()) != ".md" {
				continue
			}
			path := filepath.Join(dir, entry.Name())
			data, err := os.ReadFile(path)
			if err != nil {
				return nil, err
			}
			name := strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))
			desc, body := parseSkillBody(string(data))
			out = append(out, Skill{Name: name, Description: desc, Body: body, Source: path})
		}
	}
	return out, nil
}

func parseSkillBody(body string) (description, cleaned string) {
	lines := strings.Split(body, "\n")
	if len(lines) > 0 && strings.HasPrefix(lines[0], "# ") {
		description = strings.TrimSpace(strings.TrimPrefix(lines[0], "# "))
		return description, strings.TrimSpace(strings.Join(lines[1:], "\n"))
	}
	return "", strings.TrimSpace(body)
}

// FormatInvocation formats a skill invocation prompt.
func FormatInvocation(s Skill, input string) string {
	var b strings.Builder
	b.WriteString("Use skill: ")
	b.WriteString(s.Name)
	if s.Description != "" {
		b.WriteString(" - ")
		b.WriteString(s.Description)
	}
	b.WriteString("\n\n")
	b.WriteString(s.Body)
	if strings.TrimSpace(input) != "" {
		b.WriteString("\n\nUser input:\n")
		b.WriteString(input)
	}
	return b.String()
}
