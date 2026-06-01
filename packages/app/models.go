package app

import (
	"context"
	"path/filepath"

	"erasmus/packages/config"
	"erasmus/packages/model"
)

// Models returns all known models from catalog, using the default catalog when nil.
func Models(catalog model.Catalog) []model.Model {
	if catalog == nil {
		catalog = model.DefaultCatalog()
	}
	return catalog.List()
}

// CatalogFromConfig returns a catalog with user-configured model metadata merged in.
func CatalogFromConfig(cfg config.Config, base model.Catalog) model.Catalog {
	return CatalogFromSources(cfg, nil, base)
}

// CatalogFromSources returns a catalog with cached and user-configured model metadata merged in.
func CatalogFromSources(cfg config.Config, cached []model.Model, base model.Catalog) model.Catalog {
	if base == nil {
		base = model.DefaultCatalog()
	}
	models := base.List()
	index := make(map[string]int, len(models))
	for i, m := range models {
		index[modelKey(m.Provider, m.ID)] = i
	}
	models = mergeModels(models, index, cached)
	for _, override := range cfg.Models {
		if override.Provider == "" || override.ID == "" {
			continue
		}
		if override.DisplayName == "" {
			override.DisplayName = override.ID
		}
		if override.Source == "" {
			override.Source = "user"
		}
		key := modelKey(override.Provider, override.ID)
		if i, ok := index[key]; ok {
			models[i] = override
			continue
		}
		index[key] = len(models)
		models = append(models, override)
	}
	return model.StaticCatalog{Models: models}
}

// CatalogFromCache returns a catalog merged with model cache entries and config overrides.
func CatalogFromCache(ctx context.Context, cfg config.Config, cache model.Cache, base model.Catalog) (model.Catalog, error) {
	var cached []model.Model
	if cache != nil {
		var err error
		cached, err = cache.List(ctx)
		if err != nil {
			return nil, err
		}
	}
	return CatalogFromSources(cfg, cached, base), nil
}

// DefaultModelCachePath returns the default user-level model cache path.
func DefaultModelCachePath() string {
	return filepath.Join(xdgCacheHome(), "erasmus", "models.json")
}

func mergeModels(models []model.Model, index map[string]int, incoming []model.Model) []model.Model {
	for _, m := range incoming {
		if m.Provider == "" || m.ID == "" {
			continue
		}
		if m.DisplayName == "" {
			m.DisplayName = m.ID
		}
		if m.Source == "" {
			m.Source = "cache"
		}
		key := modelKey(m.Provider, m.ID)
		if i, ok := index[key]; ok {
			models[i] = m
			continue
		}
		index[key] = len(models)
		models = append(models, m)
	}
	return models
}

func modelKey(provider, id string) string {
	return provider + "\x00" + id
}
