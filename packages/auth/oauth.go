package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// OAuthProvider configures an OAuth authorization-code PKCE flow.
type OAuthProvider struct {
	Name          string
	AuthURL       string
	TokenURL      string
	ClientID      string
	Scopes        []string
	RedirectHost  string
	RedirectPort  int
	RedirectPath  string
	ExtraAuthArgs map[string]string
}

// OpenAIOAuth is the ChatGPT/Codex subscription OAuth client shape used by Codex CLI.
var OpenAIOAuth = OAuthProvider{
	Name:         "openai",
	AuthURL:      "https://auth.openai.com/oauth/authorize",
	TokenURL:     "https://auth.openai.com/oauth/token",
	ClientID:     "app_EMoamEEZ73f0CkXaXp7hrann",
	Scopes:       []string{"openid", "profile", "email", "offline_access"},
	RedirectHost: "localhost",
	RedirectPort: 1455,
	RedirectPath: "/auth/callback",
	ExtraAuthArgs: map[string]string{
		"id_token_add_organizations": "true",
		"codex_cli_simplified_flow":  "true",
		"originator":                 "erasmus",
	},
}

// PKCE holds a verifier/challenge pair.
type PKCE struct {
	Verifier  string
	Challenge string
}

// NewPKCE generates a S256 PKCE pair.
func NewPKCE() (PKCE, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return PKCE{}, err
	}
	verifier := base64.RawURLEncoding.EncodeToString(buf)
	sum := sha256.Sum256([]byte(verifier))
	return PKCE{Verifier: verifier, Challenge: base64.RawURLEncoding.EncodeToString(sum[:])}, nil
}

// RedirectURI returns the OAuth redirect URI.
func (p OAuthProvider) RedirectURI() string {
	return "http://" + p.RedirectHost + ":" + itoa(p.RedirectPort) + p.RedirectPath
}

// AuthorizeURL builds an authorization URL and state.
func (p OAuthProvider) AuthorizeURL(pkce PKCE) (authURL, state string, err error) {
	state, err = randomHex(16)
	if err != nil {
		return "", "", err
	}
	q := url.Values{}
	q.Set("response_type", "code")
	q.Set("client_id", p.ClientID)
	q.Set("redirect_uri", p.RedirectURI())
	q.Set("scope", strings.Join(p.Scopes, " "))
	q.Set("code_challenge", pkce.Challenge)
	q.Set("code_challenge_method", "S256")
	q.Set("state", state)
	for k, v := range p.ExtraAuthArgs {
		q.Set(k, v)
	}
	return p.AuthURL + "?" + q.Encode(), state, nil
}

// Exchange swaps an authorization code for an OAuth token.
func (p OAuthProvider) Exchange(ctx context.Context, code, state string, pkce PKCE) (*OAuthToken, error) {
	payload := map[string]string{
		"grant_type":    "authorization_code",
		"client_id":     p.ClientID,
		"code":          code,
		"redirect_uri":  p.RedirectURI(),
		"code_verifier": pkce.Verifier,
	}
	return p.doTokenRequest(ctx, payload)
}

// Refresh swaps a refresh token for a fresh OAuth token.
func (p OAuthProvider) Refresh(ctx context.Context, refreshToken string) (*OAuthToken, error) {
	payload := map[string]string{
		"grant_type":    "refresh_token",
		"client_id":     p.ClientID,
		"refresh_token": refreshToken,
	}
	return p.doTokenRequest(ctx, payload)
}

// ExtractOpenAIAccountID parses the chatgpt_account_id claim from an OpenAI id_token JWT.
func ExtractOpenAIAccountID(idToken string) string {
	parts := strings.Split(idToken, ".")
	if len(parts) < 2 {
		return ""
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		payload, err = base64.StdEncoding.DecodeString(parts[1])
		if err != nil {
			return ""
		}
	}
	var body map[string]any
	if err := json.Unmarshal(payload, &body); err != nil {
		return ""
	}
	claim, _ := body["https://api.openai.com/auth"].(map[string]any)
	id, _ := claim["chatgpt_account_id"].(string)
	return id
}

func (p OAuthProvider) doTokenRequest(ctx context.Context, payload map[string]string) (*OAuthToken, error) {
	form := url.Values{}
	for k, v := range payload {
		form.Set(k, v)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.TokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("accept", "application/json")
	req.Header.Set("content-type", "application/x-www-form-urlencoded")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("token request: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("token http %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var raw struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		TokenType    string `json:"token_type"`
		ExpiresIn    int    `json:"expires_in"`
		Scope        string `json:"scope"`
		IDToken      string `json:"id_token"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("parse token response: %w: %s", err, string(body))
	}
	if raw.AccessToken == "" {
		return nil, fmt.Errorf("empty access token in response: %s", string(body))
	}
	tok := &OAuthToken{
		AccessToken:  raw.AccessToken,
		RefreshToken: raw.RefreshToken,
		TokenType:    raw.TokenType,
		Scope:        raw.Scope,
		ClientID:     p.ClientID,
		IDToken:      raw.IDToken,
	}
	if raw.ExpiresIn > 0 {
		tok.Expiry = time.Now().Add(time.Duration(raw.ExpiresIn) * time.Second)
	}
	if raw.IDToken != "" {
		tok.AccountID = ExtractOpenAIAccountID(raw.IDToken)
	}
	return tok, nil
}

func randomHex(n int) (string, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
