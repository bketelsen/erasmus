package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bketelsen/erasmus/packages/auth"
)

func TestParseImprovementsExtractsFencedJSONAndCapsAtThree(t *testing.T) {
	output := "```json\n" + `{
  "improvements": [
    {"title":"Remove dead helper","body":"packages/a.go:12 has an unused helper."},
    {"title":"Consolidate duplicate parsing","body":"packages/a.go:20 and packages/b.go:30 duplicate parsing."},
    {"title":"Handle boundary error","body":"cmd/x.go:44 ignores an error."},
    {"title":"Trim stale TODO","body":"packages/c.go:9 has a stale TODO."}
  ]
}` + "\n```"

	got := parseImprovements(output, 3)

	if len(got) != 3 {
		t.Fatalf("len = %d, want 3", len(got))
	}
	if got[0].Title != "Remove dead helper" || got[2].Title != "Handle boundary error" {
		t.Fatalf("improvements = %#v", got)
	}
}

func TestBuildScanPromptKeepsScopeToHousekeeping(t *testing.T) {
	prompt := buildScanPrompt("owner/repo", []string{"Existing issue"}, []string{"Existing PR"}, 3)

	for _, want := range []string{
		"up to 3",
		"housekeeping and cleanup",
		"Do NOT suggest new features",
		"Do NOT suggest rewrites",
		"Existing issue",
		"Existing PR",
		`"improvements"`,
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q:\n%s", want, prompt)
		}
	}
}

func TestCreateIssueThenPullRequestForEachImplementedImprovement(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "repo")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	runner := &recordingRunner{
		responses: map[string]commandResult{
			"gh issue create --repo owner/repo --title Remove dead helper --body body": {
				stdout: "https://github.com/owner/repo/issues/42\n",
			},
			"git rev-parse --abbrev-ref origin/HEAD": {stdout: "origin/main\n"},
			"git status --porcelain":                 {stdout: " M stale.go\n"},
			"git rev-parse --short HEAD":             {stdout: "abc123\n"},
		},
	}
	agent := func(ctx context.Context, cwd, prompt string, sessionPath string) (string, error) {
		if !strings.Contains(prompt, "Remove dead helper") || !strings.Contains(prompt, "Issue: https://github.com/owner/repo/issues/42") {
			t.Fatalf("implementation prompt missing issue context:\n%s", prompt)
		}
		return "implemented", nil
	}

	created, err := createIssuesAndPRs(context.Background(), workflowOptions{
		Repo:     "owner/repo",
		WorkDir:  repo,
		StateDir: filepath.Join(root, "state"),
		Max:      3,
	}, []improvement{{Title: "Remove dead helper", Body: "body"}}, runner, agent)
	if err != nil {
		t.Fatal(err)
	}
	if created != 1 {
		t.Fatalf("created = %d, want 1", created)
	}

	joined := strings.Join(runner.commands, "\n")
	for _, want := range []string{
		"gh issue create --repo owner/repo --title Remove dead helper --body body",
		"git checkout main",
		"git checkout -B erasmus/housekeeping-42",
		"git add -A",
		"git commit -m chore: remove dead helper",
		"git push -u origin erasmus/housekeeping-42",
		"gh pr create --repo owner/repo --head erasmus/housekeeping-42 --base main --title chore: Remove dead helper --body body\n\nCloses #42",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("commands missing %q:\n%s", want, joined)
		}
	}
}

func TestListTitlesParsesGitHubJSON(t *testing.T) {
	data, err := json.Marshal([]map[string]any{{"title": "One"}, {"title": "Two"}})
	if err != nil {
		t.Fatal(err)
	}
	got, err := parseTitles(data)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(got, ",") != "One,Two" {
		t.Fatalf("titles = %#v", got)
	}
}

func TestCredentialForProviderRefreshesExpiredCodexOAuth(t *testing.T) {
	oldProvider := auth.OpenAIOAuth
	defer func() { auth.OpenAIOAuth = oldProvider }()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		if r.Form.Get("grant_type") != "refresh_token" || r.Form.Get("refresh_token") != "old-refresh" {
			t.Fatalf("form = %v", r.Form)
		}
		_, _ = w.Write([]byte(`{"access_token":"new-access","expires_in":3600}`))
	}))
	defer server.Close()
	auth.OpenAIOAuth.TokenURL = server.URL

	store := auth.NewMemoryStore()
	if err := store.Set(context.Background(), auth.Credential{
		Provider: "openai-codex",
		OAuth: &auth.OAuthToken{
			AccessToken:  "old-access",
			RefreshToken: "old-refresh",
			AccountID:    "acct-1",
			Expiry:       time.Now().Add(-time.Hour),
		},
	}); err != nil {
		t.Fatal(err)
	}

	cred, err := credentialForProvider(context.Background(), store, "openai-codex")
	if err != nil {
		t.Fatal(err)
	}
	if cred.OAuth.AccessToken != "new-access" || cred.OAuth.RefreshToken != "old-refresh" || cred.OAuth.AccountID != "acct-1" {
		t.Fatalf("credential = %#v", cred.OAuth)
	}
}

type recordingRunner struct {
	commands  []string
	responses map[string]commandResult
}

func (r *recordingRunner) Run(ctx context.Context, dir string, name string, args ...string) commandResult {
	cmd := strings.Join(append([]string{name}, args...), " ")
	r.commands = append(r.commands, cmd)
	if result, ok := r.responses[cmd]; ok {
		return result
	}
	return commandResult{}
}
