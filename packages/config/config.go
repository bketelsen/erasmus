// Package config defines user/app configuration shapes.
package config

import "github.com/bketelsen/erasmus/packages/model"

// Config is the user-facing app configuration.
type Config struct {
	Provider   string            `json:"provider,omitempty"`
	Model      string            `json:"model,omitempty"`
	Reasoning  string            `json:"reasoning,omitempty"`
	Tools      []string          `json:"tools,omitempty"`
	NoTools    bool              `json:"no_tools,omitempty"`
	CWD        string            `json:"cwd,omitempty"`
	Theme      string            `json:"theme,omitempty"`
	Models     []model.Model     `json:"models,omitempty"`
	Extensions []ExtensionConfig `json:"extensions,omitempty"`
}

// ExtensionConfig describes one JSON-line extension subprocess.
type ExtensionConfig struct {
	Command string   `json:"command"`
	Args    []string `json:"args,omitempty"`
}

// Defaults returns a minimal useful default config.
func Defaults() Config {
	return Config{Provider: "fake", Model: "echo"}
}

// Merge overlays non-zero values from override onto base.
func Merge(base, override Config) Config {
	out := base
	if override.Provider != "" {
		out.Provider = override.Provider
	}
	if override.Model != "" {
		out.Model = override.Model
	}
	if override.Reasoning != "" {
		out.Reasoning = override.Reasoning
	}
	if override.Tools != nil {
		out.Tools = append([]string(nil), override.Tools...)
	}
	if override.NoTools {
		out.NoTools = true
	}
	if override.CWD != "" {
		out.CWD = override.CWD
	}
	if override.Theme != "" {
		out.Theme = override.Theme
	}
	if override.Models != nil {
		out.Models = append([]model.Model(nil), override.Models...)
	}
	if override.Extensions != nil {
		out.Extensions = append([]ExtensionConfig(nil), override.Extensions...)
	}
	return out
}
