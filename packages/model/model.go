// Package model defines provider-independent model metadata and usage types.
package model

import (
	"context"
	"fmt"
	"time"
)

// Pricing describes model cost in provider-defined currency units per million tokens.
type Pricing struct {
	InputPerMTok  float64 `json:"input_per_mtok,omitempty"`
	OutputPerMTok float64 `json:"output_per_mtok,omitempty"`
}

// Model describes a model available through a provider.
type Model struct {
	ID            string    `json:"id"`
	Provider      string    `json:"provider"`
	DisplayName   string    `json:"display_name,omitempty"`
	ContextWindow int       `json:"context_window,omitempty"`
	MaxOutput     int       `json:"max_output,omitempty"`
	Reasoning     bool      `json:"reasoning,omitempty"`
	Source        string    `json:"source,omitempty"`
	Pricing       Pricing   `json:"pricing,omitempty"`
	DiscoveredAt  time.Time `json:"discovered_at,omitempty"`
}

// Usage records token usage for one response or a cumulative session total.
type Usage struct {
	InputTokens      int `json:"input_tokens,omitempty"`
	OutputTokens     int `json:"output_tokens,omitempty"`
	ReasoningTokens  int `json:"reasoning_tokens,omitempty"`
	CacheReadTokens  int `json:"cache_read_tokens,omitempty"`
	CacheWriteTokens int `json:"cache_write_tokens,omitempty"`
}

// Catalog resolves model metadata.
type Catalog interface {
	Find(provider, id string) (Model, error)
	List() []Model
	ListProvider(provider string) []Model
	Default(provider string) (Model, error)
}

// Discoverer discovers currently available models for a provider.
type Discoverer interface {
	DiscoverModels(ctx context.Context, provider string) ([]Model, error)
}

// StaticCatalog is an in-memory catalog useful for tests and early CLI wiring.
type StaticCatalog struct {
	Models   []Model
	Defaults map[string]string
}

// Find returns a model by provider and ID.
func (c StaticCatalog) Find(provider, id string) (Model, error) {
	for _, m := range c.Models {
		if m.Provider == provider && m.ID == id {
			return m, nil
		}
	}
	return Model{}, fmt.Errorf("model %q/%q not found", provider, id)
}

// List returns all models.
func (c StaticCatalog) List() []Model {
	out := make([]Model, len(c.Models))
	copy(out, c.Models)
	return out
}

// ListProvider returns all models for a provider.
func (c StaticCatalog) ListProvider(provider string) []Model {
	var out []Model
	for _, m := range c.Models {
		if m.Provider == provider {
			out = append(out, m)
		}
	}
	return out
}

// Default returns the configured default model for a provider, or the first provider model.
func (c StaticCatalog) Default(provider string) (Model, error) {
	if id := c.Defaults[provider]; id != "" {
		return c.Find(provider, id)
	}
	for _, m := range c.Models {
		if m.Provider == provider {
			return m, nil
		}
	}
	return Model{}, fmt.Errorf("no default model for provider %q", provider)
}
