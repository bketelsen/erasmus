package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// GitHubCopilotClientID is the public OAuth client used by GitHub Copilot Chat.
const GitHubCopilotClientID = "Iv1.b507a08c87ecfe98"

var githubCopilotHeaders = map[string]string{
	"User-Agent":             "GitHubCopilotChat/0.35.0",
	"Editor-Version":         "vscode/1.107.0",
	"Editor-Plugin-Version":  "copilot-chat/0.35.0",
	"Copilot-Integration-Id": "vscode-chat",
}

// GitHubCopilotStaticHeaders returns the static Copilot API headers used by Pi.
func GitHubCopilotStaticHeaders() map[string]string {
	headers := make(map[string]string, len(githubCopilotHeaders))
	for k, v := range githubCopilotHeaders {
		headers[k] = v
	}
	return headers
}

// GitHubCopilotDeviceProvider performs GitHub device-flow login and Copilot token exchange.
type GitHubCopilotDeviceProvider struct {
	DeviceCodeURL   string
	AccessTokenURL  string
	CopilotTokenURL string
	ClientID        string
	HTTPClient      *http.Client
	PollSleep       func(context.Context, time.Duration) error
}

type githubCopilotDeviceCode struct {
	DeviceCode      string
	UserCode        string
	VerificationURI string
	Interval        int
	ExpiresIn       int
}

// DefaultGitHubCopilotDeviceProvider returns the github.com Copilot device-flow provider.
func DefaultGitHubCopilotDeviceProvider() GitHubCopilotDeviceProvider {
	return GitHubCopilotDeviceProvider{
		DeviceCodeURL:   "https://github.com/login/device/code",
		AccessTokenURL:  "https://github.com/login/oauth/access_token",
		CopilotTokenURL: "https://api.github.com/copilot_internal/v2/token",
		ClientID:        GitHubCopilotClientID,
	}
}

// Login runs GitHub device flow and exchanges the resulting GitHub token for a Copilot token.
func (p GitHubCopilotDeviceProvider) Login(ctx context.Context, onAuth func(url, instructions string)) (*OAuthToken, error) {
	p = p.withDefaults()
	device, err := p.startDeviceFlow(ctx)
	if err != nil {
		return nil, err
	}
	if onAuth != nil {
		onAuth(device.VerificationURI, "Enter code: "+device.UserCode)
	}
	githubAccess, err := p.pollForGitHubAccessToken(ctx, device)
	if err != nil {
		return nil, err
	}
	return p.Refresh(ctx, githubAccess)
}

// Refresh exchanges a GitHub access token for a fresh Copilot API token.
func (p GitHubCopilotDeviceProvider) Refresh(ctx context.Context, githubAccessToken string) (*OAuthToken, error) {
	p = p.withDefaults()
	if strings.TrimSpace(githubAccessToken) == "" {
		return nil, fmt.Errorf("github access token is required")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.CopilotTokenURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+githubAccessToken)
	setGitHubCopilotHeaders(req)
	var raw struct {
		Token     string `json:"token"`
		ExpiresAt int64  `json:"expires_at"`
	}
	if err := p.doJSON(req, &raw); err != nil {
		return nil, err
	}
	if raw.Token == "" || raw.ExpiresAt == 0 {
		return nil, fmt.Errorf("invalid Copilot token response fields")
	}
	return &OAuthToken{
		AccessToken:  raw.Token,
		RefreshToken: githubAccessToken,
		TokenType:    "bearer",
		Expiry:       time.Unix(raw.ExpiresAt, 0).Add(-5 * time.Minute),
		ClientID:     p.clientID(),
	}, nil
}

// GitHubCopilotBaseURLFromToken extracts the Copilot API base URL from a Copilot token.
func GitHubCopilotBaseURLFromToken(token string) string {
	for _, part := range strings.Split(token, ";") {
		key, value, ok := strings.Cut(part, "=")
		if !ok || key != "proxy-ep" || value == "" {
			continue
		}
		return "https://" + strings.TrimPrefix(value, "proxy.")
	}
	return ""
}

func (p GitHubCopilotDeviceProvider) startDeviceFlow(ctx context.Context) (githubCopilotDeviceCode, error) {
	form := url.Values{}
	form.Set("client_id", p.clientID())
	form.Set("scope", "read:user")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.DeviceCodeURL, strings.NewReader(form.Encode()))
	if err != nil {
		return githubCopilotDeviceCode{}, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", githubCopilotHeaders["User-Agent"])
	var raw struct {
		DeviceCode      string `json:"device_code"`
		UserCode        string `json:"user_code"`
		VerificationURI string `json:"verification_uri"`
		Interval        int    `json:"interval"`
		ExpiresIn       int    `json:"expires_in"`
	}
	if err := p.doJSON(req, &raw); err != nil {
		return githubCopilotDeviceCode{}, err
	}
	if raw.DeviceCode == "" || raw.UserCode == "" || raw.VerificationURI == "" || raw.Interval <= 0 || raw.ExpiresIn <= 0 {
		return githubCopilotDeviceCode{}, fmt.Errorf("invalid device code response fields")
	}
	return githubCopilotDeviceCode{DeviceCode: raw.DeviceCode, UserCode: raw.UserCode, VerificationURI: raw.VerificationURI, Interval: raw.Interval, ExpiresIn: raw.ExpiresIn}, nil
}

func (p GitHubCopilotDeviceProvider) pollForGitHubAccessToken(ctx context.Context, device githubCopilotDeviceCode) (string, error) {
	deadline := time.Now().Add(time.Duration(device.ExpiresIn) * time.Second)
	interval := time.Duration(device.Interval) * time.Second
	if interval < time.Second {
		interval = time.Second
	}
	for time.Now().Before(deadline) {
		if err := p.sleep(ctx, interval); err != nil {
			return "", err
		}
		form := url.Values{}
		form.Set("client_id", p.clientID())
		form.Set("device_code", device.DeviceCode)
		form.Set("grant_type", "urn:ietf:params:oauth:grant-type:device_code")
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.AccessTokenURL, strings.NewReader(form.Encode()))
		if err != nil {
			return "", err
		}
		req.Header.Set("Accept", "application/json")
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("User-Agent", githubCopilotHeaders["User-Agent"])
		var raw struct {
			AccessToken      string `json:"access_token"`
			Error            string `json:"error"`
			ErrorDescription string `json:"error_description"`
			Interval         int    `json:"interval"`
		}
		if err := p.doJSON(req, &raw); err != nil {
			return "", err
		}
		if raw.AccessToken != "" {
			return raw.AccessToken, nil
		}
		switch raw.Error {
		case "authorization_pending", "":
			continue
		case "slow_down":
			if raw.Interval > 0 {
				interval = time.Duration(raw.Interval) * time.Second
			} else {
				interval += 5 * time.Second
			}
			continue
		default:
			if raw.ErrorDescription != "" {
				return "", fmt.Errorf("device flow failed: %s: %s", raw.Error, raw.ErrorDescription)
			}
			return "", fmt.Errorf("device flow failed: %s", raw.Error)
		}
	}
	return "", fmt.Errorf("device flow timed out")
}

func (p GitHubCopilotDeviceProvider) withDefaults() GitHubCopilotDeviceProvider {
	defaults := DefaultGitHubCopilotDeviceProvider()
	if p.DeviceCodeURL == "" {
		p.DeviceCodeURL = defaults.DeviceCodeURL
	}
	if p.AccessTokenURL == "" {
		p.AccessTokenURL = defaults.AccessTokenURL
	}
	if p.CopilotTokenURL == "" {
		p.CopilotTokenURL = defaults.CopilotTokenURL
	}
	if p.ClientID == "" {
		p.ClientID = defaults.ClientID
	}
	if p.HTTPClient == nil {
		p.HTTPClient = http.DefaultClient
	}
	return p
}

func (p GitHubCopilotDeviceProvider) clientID() string {
	if p.ClientID != "" {
		return p.ClientID
	}
	return GitHubCopilotClientID
}

func (p GitHubCopilotDeviceProvider) sleep(ctx context.Context, d time.Duration) error {
	if p.PollSleep != nil {
		return p.PollSleep(ctx, d)
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (p GitHubCopilotDeviceProvider) doJSON(req *http.Request, out any) error {
	resp, err := p.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("github copilot auth request failed: %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("parse github copilot auth response: %w: %s", err, string(body))
	}
	return nil
}

func setGitHubCopilotHeaders(req *http.Request) {
	for k, v := range GitHubCopilotStaticHeaders() {
		req.Header.Set(k, v)
	}
}
