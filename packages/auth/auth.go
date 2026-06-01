// Package auth defines provider credential storage and resolution interfaces.
package auth

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// Credential is a provider credential value.
type Credential struct {
	Provider string      `json:"provider"`
	APIKey   string      `json:"api_key,omitempty"`
	OAuth    *OAuthToken `json:"oauth,omitempty"`
}

// OAuthToken is a subscription OAuth credential.
type OAuthToken struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token,omitempty"`
	TokenType    string    `json:"token_type,omitempty"`
	Expiry       time.Time `json:"expiry,omitempty"`
	Scope        string    `json:"scope,omitempty"`
	ClientID     string    `json:"client_id,omitempty"`
	IDToken      string    `json:"id_token,omitempty"`
	AccountID    string    `json:"account_id,omitempty"`
}

// Expired reports whether the token is expired with a small safety margin.
func (t *OAuthToken) Expired() bool {
	if t == nil || t.Expiry.IsZero() {
		return false
	}
	return time.Now().After(t.Expiry.Add(-60 * time.Second))
}

// Store stores and resolves credentials.
type Store interface {
	Set(ctx context.Context, c Credential) error
	Get(ctx context.Context, provider string) (Credential, error)
	Delete(ctx context.Context, provider string) error
	List(ctx context.Context) ([]Credential, error)
}

// MemoryStore is an in-memory auth store for tests and early app wiring.
type MemoryStore struct {
	mu    sync.Mutex
	creds map[string]Credential
}

// NewMemoryStore creates an empty memory auth store.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{creds: map[string]Credential{}}
}

// Set stores a credential.
func (s *MemoryStore) Set(ctx context.Context, c Credential) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if c.Provider == "" {
		return fmt.Errorf("provider is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.creds[c.Provider] = c
	return nil
}

// Get returns a credential by provider.
func (s *MemoryStore) Get(ctx context.Context, provider string) (Credential, error) {
	if err := ctx.Err(); err != nil {
		return Credential{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.creds[provider]
	if !ok {
		return Credential{}, fmt.Errorf("credential for provider %q not found", provider)
	}
	return c, nil
}

// Delete removes a credential.
func (s *MemoryStore) Delete(ctx context.Context, provider string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.creds, provider)
	return nil
}

// List returns all credentials.
func (s *MemoryStore) List(ctx context.Context) ([]Credential, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Credential, 0, len(s.creds))
	for _, c := range s.creds {
		out = append(out, c)
	}
	return out, nil
}

var _ Store = (*MemoryStore)(nil)
