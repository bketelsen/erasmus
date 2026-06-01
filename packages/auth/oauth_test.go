package auth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestOpenAIAuthorizeURL(t *testing.T) {
	pkce := PKCE{Verifier: "verifier", Challenge: "challenge"}
	u, state, err := OpenAIOAuth.AuthorizeURL(pkce)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"https://auth.openai.com/oauth/authorize?", "client_id=app_EMoamEEZ73f0CkXaXp7hrann", "code_challenge=challenge", "codex_cli_simplified_flow=true", "originator=erasmus"} {
		if !strings.Contains(u, want) {
			t.Fatalf("url missing %q: %s", want, u)
		}
	}
	if state == "" {
		t.Fatal("expected state")
	}
}

func TestOAuthExchange(t *testing.T) {
	var body string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method %s", r.Method)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		body = r.Form.Encode()
		w.Header().Set("content-type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"access","refresh_token":"refresh","token_type":"Bearer","expires_in":3600,"scope":"openid","id_token":"` + testIDToken("acct-456") + `"}`))
	}))
	defer srv.Close()

	provider := OpenAIOAuth
	provider.TokenURL = srv.URL
	provider.RedirectPort = 9999
	tok, err := provider.Exchange(context.Background(), "code-123", "state", PKCE{Verifier: "verifier", Challenge: "challenge"})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"grant_type=authorization_code", "client_id=" + OpenAIOAuth.ClientID, "code=code-123", "code_verifier=verifier"} {
		if !strings.Contains(body, want) {
			t.Fatalf("token body missing %q: %s", want, body)
		}
	}
	if tok.AccessToken != "access" || tok.RefreshToken != "refresh" || tok.AccountID != "acct-456" || tok.IDToken == "" {
		t.Fatalf("bad token: %+v", tok)
	}
	if tok.Expiry.IsZero() {
		t.Fatal("expected expiry")
	}
}

func TestExtractOpenAIAccountID(t *testing.T) {
	jwt := testIDToken("acct-123")
	if got := ExtractOpenAIAccountID(jwt); got != "acct-123" {
		t.Fatalf("got %q", got)
	}
	if got := ExtractOpenAIAccountID("bad"); got != "" {
		t.Fatalf("bad got %q", got)
	}
}

func TestGitHubCopilotStaticHeaders(t *testing.T) {
	headers := GitHubCopilotStaticHeaders()
	want := map[string]string{
		"User-Agent":             "GitHubCopilotChat/0.35.0",
		"Editor-Version":         "vscode/1.107.0",
		"Editor-Plugin-Version":  "copilot-chat/0.35.0",
		"Copilot-Integration-Id": "vscode-chat",
	}
	for name, value := range want {
		if headers[name] != value {
			t.Fatalf("%s = %q, want %q", name, headers[name], value)
		}
	}
	headers["User-Agent"] = "changed"
	if GitHubCopilotStaticHeaders()["User-Agent"] != want["User-Agent"] {
		t.Fatal("headers helper returned mutable package state")
	}
}

func TestGitHubCopilotBaseURLFromToken(t *testing.T) {
	token := "tid=1;exp=999;proxy-ep=proxy.individual.githubcopilot.com;"
	if got := GitHubCopilotBaseURLFromToken(token); got != "https://api.individual.githubcopilot.com" {
		t.Fatalf("base URL = %q", got)
	}
	enterpriseToken := "tid=1;proxy-ep=proxy.enterprise.githubcopilot.com;"
	if got := GitHubCopilotBaseURLFromToken(enterpriseToken); got != "https://api.enterprise.githubcopilot.com" {
		t.Fatalf("enterprise base URL = %q", got)
	}
	if got := GitHubCopilotBaseURLFromToken("tid=1;"); got != "" {
		t.Fatalf("base URL for missing proxy endpoint = %q", got)
	}
}

func TestGitHubCopilotDeviceLogin(t *testing.T) {
	ctx := context.Background()

	var deviceBody, tokenBody string
	tokenPolls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("content-type", "application/json")
		switch r.URL.Path {
		case "/login/device/code":
			if err := r.ParseForm(); err != nil {
				t.Fatal(err)
			}
			deviceBody = r.Form.Encode()
			_, _ = w.Write([]byte(`{"device_code":"device-123","user_code":"ABCD-EFGH","verification_uri":"https://github.com/login/device","interval":1,"expires_in":60}`))
		case "/login/oauth/access_token":
			if err := r.ParseForm(); err != nil {
				t.Fatal(err)
			}
			tokenBody = r.Form.Encode()
			tokenPolls++
			if tokenPolls == 1 {
				_, _ = w.Write([]byte(`{"error":"authorization_pending"}`))
				return
			}
			_, _ = w.Write([]byte(`{"access_token":"github-access","token_type":"bearer","scope":"read:user"}`))
		case "/copilot_internal/v2/token":
			if got := r.Header.Get("Authorization"); got != "Bearer github-access" {
				t.Fatalf("authorization = %q", got)
			}
			wantHeaders := map[string]string{
				"User-Agent":             "GitHubCopilotChat/0.35.0",
				"Editor-Version":         "vscode/1.107.0",
				"Editor-Plugin-Version":  "copilot-chat/0.35.0",
				"Copilot-Integration-Id": "vscode-chat",
			}
			for name, want := range wantHeaders {
				if got := r.Header.Get(name); got != want {
					t.Fatalf("%s = %q, want %q", name, got, want)
				}
			}
			_, _ = w.Write([]byte(`{"token":"tid=1;exp=999;proxy-ep=proxy.individual.githubcopilot.com;","expires_at":4102444800}`))
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	provider := GitHubCopilotDeviceProvider{
		DeviceCodeURL:   srv.URL + "/login/device/code",
		AccessTokenURL:  srv.URL + "/login/oauth/access_token",
		CopilotTokenURL: srv.URL + "/copilot_internal/v2/token",
		PollSleep:       func(context.Context, time.Duration) error { return nil },
	}
	var authURL, instructions string
	tok, err := provider.Login(ctx, func(url, text string) {
		authURL = url
		instructions = text
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(deviceBody, "client_id="+GitHubCopilotClientID) || !strings.Contains(deviceBody, "scope=read%3Auser") {
		t.Fatalf("device body = %q", deviceBody)
	}
	for _, want := range []string{"client_id=" + GitHubCopilotClientID, "device_code=device-123", "grant_type=urn%3Aietf%3Aparams%3Aoauth%3Agrant-type%3Adevice_code"} {
		if !strings.Contains(tokenBody, want) {
			t.Fatalf("token body missing %q: %s", want, tokenBody)
		}
	}
	if authURL != "https://github.com/login/device" || !strings.Contains(instructions, "ABCD-EFGH") {
		t.Fatalf("auth prompt = %q %q", authURL, instructions)
	}
	if tok.AccessToken != "tid=1;exp=999;proxy-ep=proxy.individual.githubcopilot.com;" || tok.RefreshToken != "github-access" {
		t.Fatalf("token = %+v", tok)
	}
	if tok.ClientID != GitHubCopilotClientID || tok.TokenType != "bearer" || tok.Expiry.IsZero() {
		t.Fatalf("token metadata = %+v", tok)
	}
}

func testIDToken(accountID string) string {
	payload, _ := json.Marshal(map[string]any{"https://api.openai.com/auth": map[string]any{"chatgpt_account_id": accountID}})
	return "hdr." + base64.RawURLEncoding.EncodeToString(payload) + ".sig"
}
