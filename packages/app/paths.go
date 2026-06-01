package app

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
)

func xdgStateHome() string {
	if dir := os.Getenv("XDG_STATE_HOME"); dir != "" {
		return dir
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		return filepath.Join(home, ".local", "state")
	}
	return filepath.Join(os.TempDir(), "erasmus", "state")
}

func xdgCacheHome() string {
	if dir := os.Getenv("XDG_CACHE_HOME"); dir != "" {
		return dir
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		return filepath.Join(home, ".cache")
	}
	return filepath.Join(os.TempDir(), "erasmus", "cache")
}

func stateProjectKey(cwd string) string {
	if cwd == "" {
		if got, err := os.Getwd(); err == nil {
			cwd = got
		}
	}
	if cwd == "" {
		cwd = "."
	}
	cleaned := filepath.Clean(cwd)
	base := safeStateName(filepath.Base(cleaned))
	sum := sha256.Sum256([]byte(cleaned))
	return base + "-" + hex.EncodeToString(sum[:])[:12]
}

func safeStateName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" || name == "." || name == string(filepath.Separator) {
		return "workspace"
	}
	var b strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "workspace"
	}
	return out
}
