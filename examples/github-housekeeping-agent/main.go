package main

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"

	"github.com/bketelsen/erasmus/packages/auth"
	"github.com/bketelsen/erasmus/packages/config"
	"github.com/bketelsen/erasmus/packages/event"
	"github.com/bketelsen/erasmus/packages/harness"
	"github.com/bketelsen/erasmus/packages/model"
	"github.com/bketelsen/erasmus/packages/prompt"
	"github.com/bketelsen/erasmus/packages/provider"
	"github.com/bketelsen/erasmus/packages/provider/githubcopilot"
	"github.com/bketelsen/erasmus/packages/provider/openai"
	"github.com/bketelsen/erasmus/packages/provider/openaicodex"
	"github.com/bketelsen/erasmus/packages/sandbox"
	"github.com/bketelsen/erasmus/packages/session"
	"github.com/bketelsen/erasmus/packages/session/jsonl"
	"github.com/bketelsen/erasmus/packages/tools"
)

const footer = "\n\n---\nAutomated housekeeping scan by erasmus github-housekeeping-agent."

type workflowOptions struct {
	Repo      string
	Config    string
	AuthFile  string
	StateDir  string
	WorkRoot  string
	WorkDir   string
	Provider  string
	Model     string
	Reasoning string
	Max       int
	MaxSteps  int
	KeepWork  bool
	DryRun    bool
}

type improvement struct {
	Title string `json:"title"`
	Body  string `json:"body"`
}

type commandResult struct {
	stdout string
	stderr string
	err    error
}

type commandRunner interface {
	Run(ctx context.Context, dir string, name string, args ...string) commandResult
}

type realRunner struct{}

type agentFunc func(ctx context.Context, cwd, prompt string, sessionPath string) (string, error)

func main() {
	opts := parseFlags(os.Args[1:])
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, opts, realRunner{}); err != nil && !errors.Is(err, context.Canceled) {
		log.Fatal(err)
	}
}

func parseFlags(args []string) workflowOptions {
	fs := flag.NewFlagSet("github-housekeeping-agent", flag.ExitOnError)
	opts := workflowOptions{
		StateDir: filepath.Join(".erasmus", "github-housekeeping-agent"),
		WorkRoot: os.TempDir(),
		Max:      3,
		MaxSteps: 80,
	}
	fs.StringVar(&opts.Repo, "repo", opts.Repo, "target GitHub repository in owner/name form")
	fs.StringVar(&opts.Config, "config", opts.Config, "Erasmus config JSON path; defaults to ERASMUS_CONFIG_FILE or ~/.config/erasmus/config.json")
	fs.StringVar(&opts.AuthFile, "auth-file", opts.AuthFile, "Erasmus auth JSON path; defaults to ERASMUS_AUTH_FILE or ~/.local/share/erasmus/auth.json")
	fs.StringVar(&opts.StateDir, "state", opts.StateDir, "state directory for sessions and run metadata")
	fs.StringVar(&opts.WorkRoot, "work-root", opts.WorkRoot, "directory where temporary clones are created")
	fs.StringVar(&opts.Provider, "provider", opts.Provider, "provider override, for example openai-codex or github-copilot")
	fs.StringVar(&opts.Model, "model", opts.Model, "model override")
	fs.StringVar(&opts.Reasoning, "reasoning", opts.Reasoning, "reasoning override")
	fs.IntVar(&opts.Max, "max", opts.Max, "maximum opportunities to turn into issues and PRs")
	fs.IntVar(&opts.MaxSteps, "max-steps", opts.MaxSteps, "maximum tool/model turns per agent run")
	fs.BoolVar(&opts.KeepWork, "keep-work", false, "keep the temporary clone after exit")
	fs.BoolVar(&opts.DryRun, "dry-run", false, "scan only; do not create issues, branches, or pull requests")
	_ = fs.Parse(args)
	return opts
}

func run(ctx context.Context, opts workflowOptions, runner commandRunner) error {
	if strings.TrimSpace(opts.Repo) == "" {
		return fmt.Errorf("--repo is required")
	}
	if opts.Max <= 0 || opts.Max > 3 {
		opts.Max = 3
	}
	if err := os.MkdirAll(opts.StateDir, 0o755); err != nil {
		return err
	}
	workDir, err := cloneRepository(ctx, opts, runner)
	if err != nil {
		return err
	}
	if !opts.KeepWork {
		defer os.RemoveAll(filepath.Dir(workDir))
	}
	opts.WorkDir = workDir

	cfg, store, err := loadRuntimeConfig(ctx, opts)
	if err != nil {
		return err
	}
	agent := func(ctx context.Context, cwd, promptText string, sessionPath string) (string, error) {
		return runHarnessAgent(ctx, cwd, promptText, sessionPath, cfg, store, opts.MaxSteps)
	}

	openIssueTitles, err := listTitles(ctx, runner, "gh", "issue", "list", "--repo", opts.Repo, "--state", "open", "--limit", "100", "--json", "title")
	if err != nil {
		return err
	}
	openPRTitles, err := listTitles(ctx, runner, "gh", "pr", "list", "--repo", opts.Repo, "--state", "open", "--limit", "100", "--json", "title")
	if err != nil {
		return err
	}

	scanSession := filepath.Join(opts.StateDir, safeName(opts.Repo)+"-scan.jsonl")
	output, err := agent(ctx, workDir, buildScanPrompt(opts.Repo, openIssueTitles, openPRTitles, opts.Max), scanSession)
	if err != nil {
		return err
	}
	improvements := parseImprovements(output, opts.Max)
	if len(improvements) == 0 {
		log.Printf("no housekeeping opportunities found for %s", opts.Repo)
		return nil
	}
	if opts.DryRun {
		for _, item := range improvements {
			fmt.Printf("%s\n\n%s\n\n", item.Title, item.Body)
		}
		return nil
	}
	created, err := createIssuesAndPRs(ctx, opts, improvements, runner, agent)
	if err != nil {
		return err
	}
	log.Printf("created %d pull request(s) for %s", created, opts.Repo)
	return nil
}

func cloneRepository(ctx context.Context, opts workflowOptions, runner commandRunner) (string, error) {
	root, err := os.MkdirTemp(opts.WorkRoot, "erasmus-housekeeping-*")
	if err != nil {
		return "", err
	}
	workDir := filepath.Join(root, "repo")
	result := runner.Run(ctx, root, "gh", "repo", "clone", opts.Repo, "repo")
	if result.err != nil {
		_ = os.RemoveAll(root)
		return "", commandError("clone repository", result)
	}
	return workDir, nil
}

func loadRuntimeConfig(ctx context.Context, opts workflowOptions) (config.Config, auth.Store, error) {
	configPath := opts.Config
	if configPath == "" {
		configPath = os.Getenv("ERASMUS_CONFIG_FILE")
	}
	if configPath == "" {
		configPath = filepath.Join(xdgConfigHome(), "erasmus", "config.json")
	}
	authPath := opts.AuthFile
	if authPath == "" {
		authPath = os.Getenv("ERASMUS_AUTH_FILE")
	}
	if authPath == "" {
		authPath = filepath.Join(xdgDataHome(), "erasmus", "auth.json")
	}
	cfg, err := config.Load(ctx, configPath)
	if err != nil {
		return config.Config{}, nil, err
	}
	var override config.Config
	if opts.Provider != "" {
		override.Provider = opts.Provider
	}
	if opts.Model != "" {
		override.Model = opts.Model
	}
	if opts.Reasoning != "" {
		override.Reasoning = opts.Reasoning
	}
	cfg = config.Merge(cfg, override)
	return cfg, auth.NewFileStore(authPath), nil
}

func runHarnessAgent(ctx context.Context, cwd, promptText string, sessionPath string, cfg config.Config, store auth.Store, maxSteps int) (string, error) {
	cfg.CWD = cwd
	sess, err := jsonl.Open(sessionPath, session.Metadata{ID: filepath.Base(sessionPath), CWD: cwd})
	if err != nil {
		return "", err
	}
	defer sess.Close(ctx)
	m, err := resolveModel(cfg)
	if err != nil {
		return "", err
	}
	stream, err := resolveStream(ctx, m, store)
	if err != nil {
		return "", err
	}
	policy, err := sandbox.New(cwd)
	if err != nil {
		return "", err
	}
	h, err := harness.New(ctx, harness.Config{
		Session:   sess,
		Stream:    stream,
		Model:     m,
		Reasoning: cfg.Reasoning,
		Prompt: prompt.StaticBuilder{Base: strings.Join([]string{
			"You are Erasmus, a Go-native coding agent running in an unattended housekeeping workflow.",
			"Be conservative, inspect before changing files, and keep changes tightly scoped to the user's requested cleanup.",
		}, "\n")},
		Tools:    tools.DefaultRegistry(policy),
		MaxSteps: maxSteps,
	})
	if err != nil {
		return "", err
	}
	events, err := h.Prompt(ctx, promptText, harness.PromptOptions{})
	if err != nil {
		return "", err
	}
	var out strings.Builder
	for ev := range events {
		if delta, ok := ev.(event.MessageDelta); ok {
			out.WriteString(delta.Text)
			fmt.Print(delta.Text)
		}
	}
	if err := h.Wait(ctx); err != nil {
		return out.String(), err
	}
	fmt.Println()
	return out.String(), nil
}

func resolveModel(cfg config.Config) (model.Model, error) {
	providerID := cfg.Provider
	if providerID == "" {
		providerID = "fake"
	}
	catalog := model.DefaultCatalog()
	if cfg.Model == "" {
		return catalog.Default(providerID)
	}
	m, err := catalog.Find(providerID, cfg.Model)
	if err == nil {
		return m, nil
	}
	switch providerID {
	case "openai", "openai-codex", "github-copilot":
		return model.Model{Provider: providerID, ID: cfg.Model, DisplayName: cfg.Model, Source: "explicit"}, nil
	default:
		return model.Model{}, err
	}
}

func resolveStream(ctx context.Context, m model.Model, store auth.Store) (provider.StreamFunc, error) {
	switch m.Provider {
	case "fake":
		return fakeStream(), nil
	case "openai":
		cred, err := credentialForProvider(ctx, store, "openai")
		if err != nil {
			return nil, err
		}
		client, err := openai.New(openai.Config{APIKey: cred.APIKey})
		if err != nil {
			return nil, err
		}
		return client.Stream, nil
	case "openai-codex":
		cred, err := credentialForProvider(ctx, store, "openai-codex")
		if err != nil {
			return nil, err
		}
		if cred.OAuth == nil {
			return nil, fmt.Errorf("openai-codex requires OAuth credentials")
		}
		client, err := openaicodex.New(openaicodex.Config{AccessToken: cred.OAuth.AccessToken, AccountID: cred.OAuth.AccountID})
		if err != nil {
			return nil, err
		}
		return client.Stream, nil
	case "github-copilot":
		cred, err := credentialForProvider(ctx, store, "github-copilot")
		if err != nil {
			return nil, err
		}
		if cred.OAuth == nil {
			return nil, fmt.Errorf("github-copilot requires OAuth credentials")
		}
		switch {
		case strings.HasPrefix(strings.ToLower(m.ID), "claude-"):
			client, err := githubcopilot.NewAnthropicMessages(githubcopilot.Config{AccessToken: cred.OAuth.AccessToken, BaseURL: auth.GitHubCopilotBaseURLFromToken(cred.OAuth.AccessToken)})
			if err != nil {
				return nil, err
			}
			return client.Stream, nil
		case strings.HasPrefix(strings.ToLower(m.ID), "gpt-5"):
			client, err := githubcopilot.NewResponses(githubcopilot.Config{AccessToken: cred.OAuth.AccessToken, BaseURL: auth.GitHubCopilotBaseURLFromToken(cred.OAuth.AccessToken)})
			if err != nil {
				return nil, err
			}
			return client.Stream, nil
		default:
			client, err := githubcopilot.NewChatCompletions(githubcopilot.Config{AccessToken: cred.OAuth.AccessToken, BaseURL: auth.GitHubCopilotBaseURLFromToken(cred.OAuth.AccessToken)})
			if err != nil {
				return nil, err
			}
			return client.Stream, nil
		}
	default:
		return nil, fmt.Errorf("provider %q is not wired", m.Provider)
	}
}

func credentialForProvider(ctx context.Context, store auth.Store, providerName string) (auth.Credential, error) {
	cred, err := store.Get(ctx, providerName)
	if err != nil {
		return auth.Credential{}, err
	}
	if cred.OAuth == nil || !cred.OAuth.Expired() {
		return cred, nil
	}
	if cred.OAuth.RefreshToken == "" {
		return auth.Credential{}, fmt.Errorf("%s OAuth token is expired and has no refresh token", providerName)
	}
	var tok *auth.OAuthToken
	switch providerName {
	case "openai-codex":
		tok, err = auth.OpenAIOAuth.Refresh(ctx, cred.OAuth.RefreshToken)
	case "github-copilot":
		tok, err = auth.DefaultGitHubCopilotDeviceProvider().Refresh(ctx, cred.OAuth.RefreshToken)
	default:
		return cred, nil
	}
	if err != nil {
		return auth.Credential{}, err
	}
	if tok.RefreshToken == "" {
		tok.RefreshToken = cred.OAuth.RefreshToken
	}
	if tok.AccountID == "" {
		tok.AccountID = cred.OAuth.AccountID
	}
	if tok.IDToken == "" {
		tok.IDToken = cred.OAuth.IDToken
	}
	cred.OAuth = tok
	if err := store.Set(ctx, cred); err != nil {
		return auth.Credential{}, err
	}
	return cred, nil
}

func fakeStream() provider.StreamFunc {
	return func(ctx context.Context, req provider.Request) (<-chan provider.Event, error) {
		ch := make(chan provider.Event, 3)
		ch <- provider.MessageStart{MessageID: "github-housekeeping-fake"}
		ch <- provider.TextDelta{Text: "```json\n{\"improvements\":[]}\n```"}
		ch <- provider.MessageEnd{StopReason: "end_turn"}
		close(ch)
		return ch, nil
	}
}

func listTitles(ctx context.Context, runner commandRunner, name string, args ...string) ([]string, error) {
	result := runner.Run(ctx, "", name, args...)
	if result.err != nil {
		return nil, commandError("list GitHub titles", result)
	}
	return parseTitles([]byte(result.stdout))
}

func parseTitles(data []byte) ([]string, error) {
	var rows []struct {
		Title string `json:"title"`
	}
	if err := json.Unmarshal(data, &rows); err != nil {
		return nil, err
	}
	out := make([]string, 0, len(rows))
	for _, row := range rows {
		if strings.TrimSpace(row.Title) != "" {
			out = append(out, row.Title)
		}
	}
	return out, nil
}

func createIssuesAndPRs(ctx context.Context, opts workflowOptions, improvements []improvement, runner commandRunner, agent agentFunc) (int, error) {
	base, err := defaultBranch(ctx, opts.WorkDir, runner)
	if err != nil {
		return 0, err
	}
	created := 0
	for _, item := range improvements {
		issueURL, issueNumber, err := createIssue(ctx, opts, runner, item)
		if err != nil {
			return created, err
		}
		branch := "erasmus/housekeeping-" + strconv.Itoa(issueNumber)
		if err := runGit(ctx, runner, opts.WorkDir, "checkout", base); err != nil {
			return created, err
		}
		if err := runGit(ctx, runner, opts.WorkDir, "checkout", "-B", branch); err != nil {
			return created, err
		}
		sessionPath := filepath.Join(opts.StateDir, safeName(opts.Repo)+"-"+strconv.Itoa(issueNumber)+".jsonl")
		if _, err := agent(ctx, opts.WorkDir, buildImplementationPrompt(opts.Repo, item, issueURL), sessionPath); err != nil {
			return created, err
		}
		dirty, err := hasChanges(ctx, runner, opts.WorkDir)
		if err != nil {
			return created, err
		}
		if !dirty {
			log.Printf("no changes produced for issue #%d: %s", issueNumber, item.Title)
			continue
		}
		if err := runGit(ctx, runner, opts.WorkDir, "add", "-A"); err != nil {
			return created, err
		}
		if err := runGit(ctx, runner, opts.WorkDir, "commit", "-m", commitMessage(item.Title)); err != nil {
			return created, err
		}
		if err := runGit(ctx, runner, opts.WorkDir, "push", "-u", "origin", branch); err != nil {
			return created, err
		}
		prBody := strings.TrimSpace(item.Body) + "\n\nCloses #" + strconv.Itoa(issueNumber) + footer
		result := runner.Run(ctx, opts.WorkDir, "gh", "pr", "create", "--repo", opts.Repo, "--head", branch, "--base", base, "--title", "chore: "+item.Title, "--body", prBody)
		if result.err != nil {
			return created, commandError("create pull request", result)
		}
		created++
	}
	return created, nil
}

func defaultBranch(ctx context.Context, workDir string, runner commandRunner) (string, error) {
	result := runner.Run(ctx, workDir, "git", "rev-parse", "--abbrev-ref", "origin/HEAD")
	if result.err != nil {
		return "", commandError("resolve default branch", result)
	}
	branch := strings.TrimSpace(result.stdout)
	branch = strings.TrimPrefix(branch, "origin/")
	if branch == "" {
		return "", fmt.Errorf("default branch was empty")
	}
	return branch, nil
}

func createIssue(ctx context.Context, opts workflowOptions, runner commandRunner, item improvement) (string, int, error) {
	body := strings.TrimSpace(item.Body)
	result := runner.Run(ctx, opts.WorkDir, "gh", "issue", "create", "--repo", opts.Repo, "--title", item.Title, "--body", body)
	if result.err != nil {
		return "", 0, commandError("create issue", result)
	}
	url := strings.TrimSpace(result.stdout)
	number, err := issueNumber(url)
	if err != nil {
		return "", 0, err
	}
	return url, number, nil
}

func hasChanges(ctx context.Context, runner commandRunner, workDir string) (bool, error) {
	result := runner.Run(ctx, workDir, "git", "status", "--porcelain")
	if result.err != nil {
		return false, commandError("check git status", result)
	}
	return strings.TrimSpace(result.stdout) != "", nil
}

func runGit(ctx context.Context, runner commandRunner, workDir string, args ...string) error {
	result := runner.Run(ctx, workDir, "git", args...)
	if result.err != nil {
		return commandError("git "+strings.Join(args, " "), result)
	}
	return nil
}

func issueNumber(url string) (int, error) {
	match := regexp.MustCompile(`/issues/([0-9]+)`).FindStringSubmatch(strings.TrimSpace(url))
	if len(match) != 2 {
		return 0, fmt.Errorf("could not parse issue number from %q", url)
	}
	return strconv.Atoi(match[1])
}

func buildScanPrompt(fullName string, openIssueTitles []string, openPRTitles []string, max int) string {
	return strings.Join([]string{
		"You are analyzing the repository " + fullName + " for housekeeping and cleanup opportunities.",
		"",
		"Read the codebase thoroughly. If `AGENTS.md`, `OVERVIEW.md`, or similar architecture notes exist, read them first for repository-specific guidance.",
		"",
		"Find up to " + strconv.Itoa(max) + " meaningful opportunities such as:",
		"- Code that could be consolidated because duplicate or near-duplicate logic exists",
		"- Overcomplicated code that could be simplified without changing behavior",
		"- Dead code, unused exports, or unused dependencies",
		"- Performance issues or inefficiencies in existing paths",
		"- Security concerns",
		"- Missing error handling at system boundaries",
		"- Stale TODOs or FIXMEs that should be addressed",
		"",
		"Guidelines:",
		"- Be conservative. Only suggest improvements that provide clear, tangible value.",
		"- Do NOT suggest new features.",
		"- Do NOT suggest rewrites.",
		"- Do NOT suggest stylistic changes, comment additions, type annotations, docstrings, or documentation-only work.",
		"- Do NOT suggest broad formatting or dependency churn.",
		"- \"No improvements found\" is acceptable. Do not manufacture suggestions.",
		"- Group related cleanup into one suggestion when it should be addressed together.",
		"- Each suggestion must be specific and actionable, referencing exact files and line numbers.",
		"",
		"The following issues are already open in this repository. Do NOT re-suggest these:",
		formatList(openIssueTitles),
		"",
		"The following pull requests are already open in this repository. Do NOT re-suggest these:",
		formatList(openPRTitles),
		"",
		"Respond with ONLY a JSON block in this exact format, no other text:",
		"```json",
		`{"improvements":[{"title":"Short descriptive title in imperative mood","body":"Detailed description with file references, what to change, and why"}]}`,
		"```",
		"",
		"If no improvements are worth suggesting, respond with:",
		"```json",
		`{"improvements":[]}`,
		"```",
	}, "\n")
}

func buildImplementationPrompt(fullName string, item improvement, issueURL string) string {
	return strings.Join([]string{
		"You are implementing one housekeeping improvement in the repository " + fullName + ".",
		"",
		"Issue: " + issueURL,
		"",
		"Improvement: " + item.Title,
		item.Body,
		"",
		"Read `AGENTS.md`, `OVERVIEW.md`, or similar architecture notes first when they exist.",
		"Implement only this cleanup. Do not add features, rewrite unrelated code, or perform documentation-only work.",
		"Run focused tests when practical. Leave changed files in the working tree; the outer workflow will commit, push, and open the pull request.",
	}, "\n")
}

func parseImprovements(output string, max int) []improvement {
	raw := extractJSON(output)
	if raw == "" {
		return nil
	}
	var parsed struct {
		Improvements []improvement `json:"improvements"`
	}
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return nil
	}
	out := make([]improvement, 0, len(parsed.Improvements))
	for _, item := range parsed.Improvements {
		item.Title = strings.TrimSpace(item.Title)
		item.Body = strings.TrimSpace(item.Body)
		if item.Title == "" || item.Body == "" {
			continue
		}
		out = append(out, item)
		if len(out) == max {
			break
		}
	}
	return out
}

func extractJSON(output string) string {
	fence := regexp.MustCompile("(?s)```json\\s*(.*?)```").FindStringSubmatch(output)
	if len(fence) == 2 {
		return strings.TrimSpace(fence[1])
	}
	raw := regexp.MustCompile(`(?s)\{.*"improvements".*\}`).FindString(output)
	return strings.TrimSpace(raw)
}

func formatList(items []string) string {
	if len(items) == 0 {
		return "  (none)"
	}
	lines := make([]string, 0, len(items))
	for _, item := range items {
		lines = append(lines, "  - "+item)
	}
	return strings.Join(lines, "\n")
}

func commitMessage(title string) string {
	title = strings.TrimSpace(title)
	if title == "" {
		return "chore: housekeeping cleanup"
	}
	return "chore: " + strings.ToLower(title[:1]) + title[1:]
}

func safeName(value string) string {
	clean := strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' {
			return r
		}
		return '-'
	}, value)
	clean = strings.Trim(clean, "-")
	if clean != "" {
		return clean
	}
	sum := sha1.Sum([]byte(value))
	return hex.EncodeToString(sum[:])[:12]
}

func (realRunner) Run(ctx context.Context, dir string, name string, args ...string) commandResult {
	cmd := exec.CommandContext(ctx, name, args...)
	if dir != "" {
		cmd.Dir = dir
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return commandResult{stdout: stdout.String(), stderr: stderr.String(), err: err}
}

func commandError(action string, result commandResult) error {
	detail := strings.TrimSpace(result.stderr)
	if detail == "" {
		detail = strings.TrimSpace(result.stdout)
	}
	if detail == "" {
		return fmt.Errorf("%s: %w", action, result.err)
	}
	return fmt.Errorf("%s: %w: %s", action, result.err, detail)
}

func xdgConfigHome() string {
	if value := os.Getenv("XDG_CONFIG_HOME"); value != "" {
		return value
	}
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, ".config")
	}
	return "."
}

func xdgDataHome() string {
	if value := os.Getenv("XDG_DATA_HOME"); value != "" {
		return value
	}
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, ".local", "share")
	}
	return "."
}
