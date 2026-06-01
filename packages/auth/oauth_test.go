package auth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
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

func testIDToken(accountID string) string {
	payload, _ := json.Marshal(map[string]any{"https://api.openai.com/auth": map[string]any{"chatgpt_account_id": accountID}})
	return "hdr." + base64.RawURLEncoding.EncodeToString(payload) + ".sig"
}
