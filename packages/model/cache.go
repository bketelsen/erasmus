package model

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// Cache stores provider-discovered model metadata.
type Cache interface {
	List(ctx context.Context) ([]Model, error)
	ListProvider(ctx context.Context, provider string) ([]Model, error)
	PutProvider(ctx context.Context, provider string, models []Model, discoveredAt time.Time) error
}

// FileCache stores model catalog entries in a JSON file.
type FileCache struct {
	path string
}

// NewFileCache returns a file-backed model cache.
func NewFileCache(path string) *FileCache {
	return &FileCache{path: path}
}

type cacheFile struct {
	Providers map[string]cachedProvider `json:"providers,omitempty"`
}

type cachedProvider struct {
	DiscoveredAt time.Time `json:"discovered_at,omitempty"`
	Models       []Model   `json:"models,omitempty"`
}

// List returns all cached models.
func (c *FileCache) List(ctx context.Context) ([]Model, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	data, err := c.read()
	if err != nil {
		return nil, err
	}
	var out []Model
	providers := make([]string, 0, len(data.Providers))
	for provider := range data.Providers {
		providers = append(providers, provider)
	}
	sort.Strings(providers)
	for _, provider := range providers {
		out = append(out, cachedModels(provider, data.Providers[provider])...)
	}
	return out, nil
}

// ListProvider returns cached models for one provider.
func (c *FileCache) ListProvider(ctx context.Context, provider string) ([]Model, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	data, err := c.read()
	if err != nil {
		return nil, err
	}
	return cachedModels(provider, data.Providers[provider]), nil
}

// PutProvider replaces cached models for one provider.
func (c *FileCache) PutProvider(ctx context.Context, provider string, models []Model, discoveredAt time.Time) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	data, err := c.read()
	if err != nil {
		return err
	}
	if data.Providers == nil {
		data.Providers = map[string]cachedProvider{}
	}
	copied := make([]Model, 0, len(models))
	for _, m := range models {
		if m.ID == "" {
			continue
		}
		if m.Provider == "" {
			m.Provider = provider
		}
		copied = append(copied, m)
	}
	data.Providers[provider] = cachedProvider{DiscoveredAt: discoveredAt, Models: copied}
	if err := os.MkdirAll(filepath.Dir(c.path), 0o755); err != nil {
		return err
	}
	bytes, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(c.path, append(bytes, '\n'), 0o600)
}

func (c *FileCache) read() (cacheFile, error) {
	if c == nil || c.path == "" {
		return cacheFile{}, nil
	}
	bytes, err := os.ReadFile(c.path)
	if errors.Is(err, os.ErrNotExist) {
		return cacheFile{}, nil
	}
	if err != nil {
		return cacheFile{}, err
	}
	var data cacheFile
	if err := json.Unmarshal(bytes, &data); err != nil {
		return cacheFile{}, err
	}
	return data, nil
}

func cachedModels(provider string, cached cachedProvider) []Model {
	out := make([]Model, 0, len(cached.Models))
	for _, m := range cached.Models {
		if m.Provider == "" {
			m.Provider = provider
		}
		m.Source = "cache"
		if m.DiscoveredAt.IsZero() {
			m.DiscoveredAt = cached.DiscoveredAt
		}
		out = append(out, m)
	}
	return out
}
