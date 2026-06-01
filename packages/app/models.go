package app

import (
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
	if base == nil {
		base = model.DefaultCatalog()
	}
	models := base.List()
	index := make(map[string]int, len(models))
	for i, m := range models {
		index[modelKey(m.Provider, m.ID)] = i
	}
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

func modelKey(provider, id string) string {
	return provider + "\x00" + id
}
