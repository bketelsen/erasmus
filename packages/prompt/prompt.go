// Package prompt builds system prompts from runtime context.
package prompt

import (
	"context"
	"strings"

	"erasmus/packages/model"
	"erasmus/packages/session"
	"erasmus/packages/skill"
	"erasmus/packages/tool"
)

// Builder builds a system prompt.
type Builder interface {
	Build(ctx context.Context, in Input) (string, error)
}

// Input is the context available to a system prompt builder.
type Input struct {
	Model       model.Model
	Reasoning   string
	ActiveTools []tool.Tool
	Skills      []skill.Skill
	SessionMeta session.Metadata
}

// StaticBuilder returns fixed base text plus basic runtime context.
type StaticBuilder struct {
	Base string
}

// Build returns a simple useful system prompt.
func (b StaticBuilder) Build(ctx context.Context, in Input) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	base := strings.TrimSpace(b.Base)
	if base == "" {
		base = "You are Erasmus, a Go-native coding agent. Be concise, accurate, and helpful."
	}
	var parts []string
	parts = append(parts, base)
	if in.SessionMeta.CWD != "" {
		parts = append(parts, "Workspace: "+in.SessionMeta.CWD)
	}
	if len(in.ActiveTools) > 0 {
		names := make([]string, 0, len(in.ActiveTools))
		for _, t := range in.ActiveTools {
			if t != nil {
				names = append(names, t.Name())
			}
		}
		if len(names) > 0 {
			parts = append(parts, "Available tools: "+strings.Join(names, ", "))
		}
	}
	if len(in.Skills) > 0 {
		var b strings.Builder
		b.WriteString("Available skills:")
		for _, s := range in.Skills {
			b.WriteString("\n- ")
			b.WriteString(s.Name)
			if s.Description != "" {
				b.WriteString(": ")
				b.WriteString(s.Description)
			}
		}
		parts = append(parts, b.String())
	}
	return strings.Join(parts, "\n\n"), nil
}
