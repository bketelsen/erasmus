package auth

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

// FileStore persists credentials to a JSON file.
type FileStore struct {
	mu   sync.Mutex
	path string
}

// NewFileStore creates a file-backed auth store.
func NewFileStore(path string) *FileStore { return &FileStore{path: path} }

// Set stores a credential.
func (s *FileStore) Set(ctx context.Context, c Credential) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	mem, err := s.load()
	if err != nil {
		return err
	}
	if err := mem.Set(ctx, c); err != nil {
		return err
	}
	return s.save(ctx, mem)
}

// Get gets a credential.
func (s *FileStore) Get(ctx context.Context, provider string) (Credential, error) {
	if err := ctx.Err(); err != nil {
		return Credential{}, err
	}
	mem, err := s.load()
	if err != nil {
		return Credential{}, err
	}
	return mem.Get(ctx, provider)
}

// Delete deletes a credential.
func (s *FileStore) Delete(ctx context.Context, provider string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	mem, err := s.load()
	if err != nil {
		return err
	}
	if err := mem.Delete(ctx, provider); err != nil {
		return err
	}
	return s.save(ctx, mem)
}

// List lists credentials.
func (s *FileStore) List(ctx context.Context) ([]Credential, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	mem, err := s.load()
	if err != nil {
		return nil, err
	}
	return mem.List(ctx)
}

func (s *FileStore) load() (*MemoryStore, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	mem := NewMemoryStore()
	data, err := os.ReadFile(s.path)
	if os.IsNotExist(err) {
		return mem, nil
	}
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return mem, nil
	}
	var creds []Credential
	if err := json.Unmarshal(data, &creds); err != nil {
		return nil, err
	}
	for _, c := range creds {
		mem.creds[c.Provider] = c
	}
	return mem, nil
}

func (s *FileStore) save(ctx context.Context, mem *MemoryStore) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	creds, err := mem.List(ctx)
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(creds, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0o600)
}

var _ Store = (*FileStore)(nil)
