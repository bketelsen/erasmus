package config

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
)

// Load reads a JSON config file. Missing files return Defaults.
func Load(ctx context.Context, path string) (Config, error) {
	if err := ctx.Err(); err != nil {
		return Config{}, err
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return Defaults(), nil
	}
	if err != nil {
		return Config{}, err
	}
	if len(data) == 0 {
		return Defaults(), nil
	}
	cfg := Defaults()
	var fileCfg Config
	if err := json.Unmarshal(data, &fileCfg); err != nil {
		return Config{}, err
	}
	return Merge(cfg, fileCfg), nil
}

// Save writes a JSON config file.
func Save(ctx context.Context, path string, cfg Config) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
