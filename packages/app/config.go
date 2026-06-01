package app

import (
	"context"

	"github.com/bketelsen/erasmus/packages/config"
)

// ConfigGet loads config from path.
func ConfigGet(ctx context.Context, path string) (config.Config, error) {
	return config.Load(ctx, path)
}

// ConfigSet loads config, applies a patch, saves it, and returns the result.
func ConfigSet(ctx context.Context, path string, patch config.Config) (config.Config, error) {
	current, err := config.Load(ctx, path)
	if err != nil {
		return config.Config{}, err
	}
	updated := config.Merge(current, patch)
	if err := config.Save(ctx, path, updated); err != nil {
		return config.Config{}, err
	}
	return updated, nil
}
