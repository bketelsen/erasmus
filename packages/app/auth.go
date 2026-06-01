package app

import (
	"context"
	"fmt"
	"io"
	"time"

	"erasmus/packages/auth"
)

// Login stores a provider API key.
func Login(ctx context.Context, store auth.Store, provider, apiKey string) error {
	return store.Set(ctx, auth.Credential{Provider: provider, APIKey: apiKey})
}

// LoginOpenAICodexOAuth runs the OpenAI/Codex OAuth loopback login flow.
func LoginOpenAICodexOAuth(ctx context.Context, store auth.Store, out io.Writer) error {
	return LoginOAuth(ctx, store, "openai-codex", auth.OpenAIOAuth, out)
}

// LoginOAuth runs a provider OAuth loopback login flow and stores the token.
func LoginOAuth(ctx context.Context, store auth.Store, providerName string, oauthProvider auth.OAuthProvider, out io.Writer) error {
	pkce, err := auth.NewPKCE()
	if err != nil {
		return err
	}
	authURL, state, err := oauthProvider.AuthorizeURL(pkce)
	if err != nil {
		return err
	}
	server, err := auth.NewCallbackServer(oauthProvider, state)
	if err != nil {
		return err
	}
	defer server.Shutdown()
	if out != nil {
		fmt.Fprintln(out, "Open this URL in your browser:")
		fmt.Fprintln(out, authURL)
		fmt.Fprintln(out, "Waiting for OAuth callback on "+oauthProvider.RedirectURI())
	}
	result, err := server.Result(ctx)
	if err != nil {
		return err
	}
	if result.Err != nil {
		return result.Err
	}
	tok, err := oauthProvider.Exchange(ctx, result.Code, result.State, pkce)
	if err != nil {
		return err
	}
	if tok.AccountID == "" {
		return fmt.Errorf("oauth token response did not include ChatGPT account id")
	}
	return store.Set(ctx, auth.Credential{Provider: providerName, OAuth: tok})
}

// Logout removes provider credentials.
func Logout(ctx context.Context, store auth.Store, provider string) error {
	return store.Delete(ctx, provider)
}

// AuthStatusEntry is safe-to-print credential metadata.
type AuthStatusEntry struct {
	Provider  string
	Method    string
	AccountID string
	Expiry    time.Time
	Expired   bool
}

// String formats credential metadata without exposing secrets.
func (e AuthStatusEntry) String() string {
	out := e.Provider
	if e.Method != "" {
		out += "\t" + e.Method
	}
	if e.AccountID != "" {
		out += "\taccount=" + e.AccountID
	}
	if !e.Expiry.IsZero() {
		out += "\texpires=" + e.Expiry.Format(time.RFC3339)
		if e.Expired {
			out += "\t(expired)"
		}
	}
	return out
}

// AuthStatus reports configured credential providers without exposing secret values.
func AuthStatus(ctx context.Context, store auth.Store) ([]string, error) {
	entries, err := AuthStatusDetails(ctx, store)
	if err != nil {
		return nil, err
	}
	providers := make([]string, 0, len(entries))
	for _, e := range entries {
		providers = append(providers, e.Provider)
	}
	return providers, nil
}

// AuthStatusDetails reports safe credential metadata without exposing secret values.
func AuthStatusDetails(ctx context.Context, store auth.Store) ([]AuthStatusEntry, error) {
	creds, err := store.List(ctx)
	if err != nil {
		return nil, err
	}
	entries := make([]AuthStatusEntry, 0, len(creds))
	for _, c := range creds {
		e := AuthStatusEntry{Provider: c.Provider}
		if c.OAuth != nil {
			e.Method = "oauth"
			e.AccountID = c.OAuth.AccountID
			e.Expiry = c.OAuth.Expiry
			e.Expired = c.OAuth.Expired()
		} else if c.APIKey != "" {
			e.Method = "api-key"
		}
		entries = append(entries, e)
	}
	return entries, nil
}
