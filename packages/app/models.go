package app

import "erasmus/packages/model"

// Models returns all known models from catalog, using the default catalog when nil.
func Models(catalog model.Catalog) []model.Model {
	if catalog == nil {
		catalog = model.DefaultCatalog()
	}
	return catalog.List()
}
