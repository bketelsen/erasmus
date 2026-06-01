// Package sandbox provides filesystem and process policy helpers for tools.
package sandbox

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Policy constrains tool filesystem access to a working directory tree.
type Policy struct {
	Root string
}

// New creates a sandbox policy rooted at root. Empty root means current working directory.
func New(root string) (Policy, error) {
	if root == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return Policy{}, err
		}
		root = cwd
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return Policy{}, err
	}
	real, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return Policy{}, err
	}
	return Policy{Root: real}, nil
}

// Resolve validates a user path and returns an absolute path inside the sandbox root.
func (p Policy) Resolve(path string) (string, error) {
	if p.Root == "" {
		return "", fmt.Errorf("sandbox root is empty")
	}
	if strings.TrimSpace(path) == "" {
		return "", fmt.Errorf("path is required")
	}

	candidate := path
	if !filepath.IsAbs(candidate) {
		candidate = filepath.Join(p.Root, candidate)
	}
	abs, err := filepath.Abs(candidate)
	if err != nil {
		return "", err
	}

	// Resolve the existing parent so symlinked directories cannot escape the root.
	check := abs
	for {
		if _, err := os.Lstat(check); err == nil {
			resolved, err := filepath.EvalSymlinks(check)
			if err != nil {
				return "", err
			}
			if check == abs {
				abs = resolved
			} else {
				rel, err := filepath.Rel(check, abs)
				if err != nil {
					return "", err
				}
				abs = filepath.Join(resolved, rel)
			}
			break
		}
		parent := filepath.Dir(check)
		if parent == check {
			return "", fmt.Errorf("no existing parent for %q", path)
		}
		check = parent
	}

	if !inside(p.Root, abs) {
		return "", fmt.Errorf("path %q escapes sandbox root %q", path, p.Root)
	}
	return abs, nil
}

func inside(root, path string) bool {
	if path == root {
		return true
	}
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, "../") && !filepath.IsAbs(rel)
}
