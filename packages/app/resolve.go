package app

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/bketelsen/erasmus/packages/auth"
	"github.com/bketelsen/erasmus/packages/config"
	"github.com/bketelsen/erasmus/packages/harness"
	"github.com/bketelsen/erasmus/packages/loop"
	"github.com/bketelsen/erasmus/packages/model"
	"github.com/bketelsen/erasmus/packages/prompt"
	"github.com/bketelsen/erasmus/packages/provider"
	"github.com/bketelsen/erasmus/packages/provider/githubcopilot"
	"github.com/bketelsen/erasmus/packages/provider/openai"
	"github.com/bketelsen/erasmus/packages/provider/openaicodex"
	"github.com/bketelsen/erasmus/packages/sandbox"
	"github.com/bketelsen/erasmus/packages/session"
	"github.com/bketelsen/erasmus/packages/skill"
	"github.com/bketelsen/erasmus/packages/tool"
	"github.com/bketelsen/erasmus/packages/tools"
)

var gitHubCopilotDeviceProvider = auth.DefaultGitHubCopilotDeviceProvider

// ResolveOptions controls app config resolution.
type ResolveOptions struct {
	Config     config.Config
	Session    session.Session
	Stream     provider.StreamFunc
	Catalog    model.Catalog
	Auth       auth.Store
	Skills     []skill.Skill
	ExtraTools tool.Registry
	MaxSteps   int
}

// Resolved is an app-level resolved runtime configuration.
type Resolved struct {
	Config  config.Config
	Model   model.Model
	Tools   tool.Registry
	Harness harness.Config
}

// ResolveHarnessConfig resolves user config into a harness config.
func ResolveHarnessConfig(ctx context.Context, opts ResolveOptions) (Resolved, error) {
	cfg := config.Merge(config.Defaults(), opts.Config)
	catalog := opts.Catalog
	if catalog == nil {
		catalog = model.DefaultCatalog()
	}
	catalog = CatalogFromConfig(cfg, catalog)
	m, err := resolveModel(catalog, cfg.Provider, cfg.Model)
	if err != nil {
		return Resolved{}, err
	}
	stream := opts.Stream
	if stream == nil {
		stream, err = resolveStream(ctx, m, opts.Auth)
		if err != nil {
			return Resolved{}, err
		}
	}
	if opts.Session == nil {
		return Resolved{}, fmt.Errorf("session is required")
	}
	if opts.Auth != nil && m.Provider != "fake" && opts.Stream != nil {
		if _, err := credentialForProvider(ctx, opts.Auth, m.Provider); err != nil {
			return Resolved{}, err
		}
	}
	registry, err := resolveTools(cfg, opts.ExtraTools)
	if err != nil {
		return Resolved{}, err
	}
	hcfg := harness.Config{
		Session:   opts.Session,
		Stream:    stream,
		Model:     m,
		Reasoning: cfg.Reasoning,
		Prompt:    prompt.StaticBuilder{},
		Skills:    opts.Skills,
		Tools:     registry,
		MaxSteps:  opts.MaxSteps,
		ConfirmToolCall: func(context.Context, loop.ToolCallContext) (bool, error) {
			return true, nil
		},
	}
	return Resolved{Config: cfg, Model: m, Tools: registry, Harness: hcfg}, nil
}

func resolveStream(ctx context.Context, m model.Model, store auth.Store) (provider.StreamFunc, error) {
	switch m.Provider {
	case "fake":
		return fakeStream(), nil
	case "openai":
		cred, err := credentialForProvider(ctx, store, m.Provider)
		if err != nil {
			return nil, err
		}
		client, err := openai.New(openai.Config{APIKey: cred.APIKey})
		if err != nil {
			return nil, err
		}
		return client.Stream, nil
	case "openai-codex":
		cred, err := credentialForProvider(ctx, store, m.Provider)
		if err != nil {
			return nil, err
		}
		if cred.OAuth == nil {
			return nil, fmt.Errorf("openai-codex requires OAuth credentials")
		}
		if cred.OAuth.Expired() {
			cred, err = refreshOpenAICodexCredential(ctx, store, cred)
			if err != nil {
				return nil, err
			}
		}
		client, err := openaicodex.New(openaicodex.Config{AccessToken: cred.OAuth.AccessToken, AccountID: cred.OAuth.AccountID})
		if err != nil {
			return nil, err
		}
		return client.Stream, nil
	case "github-copilot":
		cred, err := credentialForProvider(ctx, store, m.Provider)
		if err != nil {
			return nil, err
		}
		if cred.OAuth == nil {
			return nil, fmt.Errorf("github-copilot requires OAuth credentials")
		}
		if cred.OAuth.Expired() {
			cred, err = refreshGitHubCopilotCredential(ctx, store, cred)
			if err != nil {
				return nil, err
			}
		}
		switch {
		case githubCopilotUsesChatCompletions(m.ID):
			baseURL := auth.GitHubCopilotBaseURLFromToken(cred.OAuth.AccessToken)
			client, err := githubcopilot.NewChatCompletions(githubcopilot.Config{AccessToken: cred.OAuth.AccessToken, BaseURL: baseURL})
			if err != nil {
				return nil, err
			}
			return client.Stream, nil
		case githubCopilotUsesResponses(m.ID):
			baseURL := auth.GitHubCopilotBaseURLFromToken(cred.OAuth.AccessToken)
			client, err := githubcopilot.NewResponses(githubcopilot.Config{AccessToken: cred.OAuth.AccessToken, BaseURL: baseURL})
			if err != nil {
				return nil, err
			}
			return client.Stream, nil
		case githubCopilotUsesAnthropicMessages(m.ID):
			baseURL := auth.GitHubCopilotBaseURLFromToken(cred.OAuth.AccessToken)
			client, err := githubcopilot.NewAnthropicMessages(githubcopilot.Config{AccessToken: cred.OAuth.AccessToken, BaseURL: baseURL})
			if err != nil {
				return nil, err
			}
			return client.Stream, nil
		default:
			return nil, fmt.Errorf("github-copilot model %q is not wired yet", m.ID)
		}
	default:
		return nil, fmt.Errorf("provider %q is not wired", m.Provider)
	}
}

func githubCopilotUsesChatCompletions(modelID string) bool {
	id := strings.ToLower(modelID)
	return !strings.HasPrefix(id, "gpt-5") && !strings.HasPrefix(id, "claude-")
}

func githubCopilotUsesResponses(modelID string) bool {
	return strings.HasPrefix(strings.ToLower(modelID), "gpt-5")
}

func githubCopilotUsesAnthropicMessages(modelID string) bool {
	return strings.HasPrefix(strings.ToLower(modelID), "claude-")
}

func refreshOpenAICodexCredential(ctx context.Context, store auth.Store, cred auth.Credential) (auth.Credential, error) {
	if cred.OAuth.RefreshToken == "" {
		return auth.Credential{}, fmt.Errorf("openai-codex OAuth token is expired and has no refresh token")
	}
	tok, err := auth.OpenAIOAuth.Refresh(ctx, cred.OAuth.RefreshToken)
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

func refreshGitHubCopilotCredential(ctx context.Context, store auth.Store, cred auth.Credential) (auth.Credential, error) {
	if cred.OAuth.RefreshToken == "" {
		return auth.Credential{}, fmt.Errorf("github-copilot OAuth token is expired and has no GitHub access token")
	}
	provider := gitHubCopilotDeviceProvider()
	githubToken := cred.OAuth.RefreshToken
	githubRefreshToken := cred.OAuth.GitHubRefreshToken
	githubTokenExpiry := cred.OAuth.GitHubTokenExpiry

	// Renew the short-lived GitHub user token (ghu_) from the durable refresh
	// token (ghr_) when it has expired, so re-running the CLI login is only
	// needed when the refresh token itself lapses (~6 months).
	if githubRefreshToken != "" && cred.OAuth.GitHubTokenExpired() {
		user, err := provider.RefreshGitHubToken(ctx, githubRefreshToken)
		if err != nil {
			return auth.Credential{}, err
		}
		githubToken = user.AccessToken
		githubTokenExpiry = user.Expiry
		if user.RefreshToken != "" {
			githubRefreshToken = user.RefreshToken
		}
	}

	tok, err := provider.Refresh(ctx, githubToken)
	if err != nil {
		return auth.Credential{}, err
	}
	if tok.RefreshToken == "" {
		tok.RefreshToken = githubToken
	}
	tok.GitHubRefreshToken = githubRefreshToken
	tok.GitHubTokenExpiry = githubTokenExpiry
	cred.OAuth = tok
	if err := store.Set(ctx, cred); err != nil {
		return auth.Credential{}, err
	}
	return cred, nil
}

func credentialForProvider(ctx context.Context, store auth.Store, providerID string) (auth.Credential, error) {
	if store == nil {
		return auth.Credential{}, fmt.Errorf("auth store is required for provider %q", providerID)
	}
	cred, err := store.Get(ctx, providerID)
	if err == nil {
		return cred, nil
	}
	if providerID == "openai-codex" {
		cred, fallbackErr := store.Get(ctx, "openai")
		if fallbackErr == nil && cred.OAuth != nil {
			return cred, nil
		}
	}
	return auth.Credential{}, err
}

func resolveModel(catalog model.Catalog, providerID, modelID string) (model.Model, error) {
	if providerID == "" {
		providerID = "fake"
	}
	if modelID == "" {
		return catalog.Default(providerID)
	}
	m, err := catalog.Find(providerID, modelID)
	if err == nil {
		return m, nil
	}
	if allowsExplicitModelID(providerID) {
		return model.Model{Provider: providerID, ID: modelID, DisplayName: modelID, Source: "explicit"}, nil
	}
	return model.Model{}, err
}

func allowsExplicitModelID(providerID string) bool {
	switch providerID {
	case "openai", "openai-codex", "github-copilot":
		return true
	default:
		return false
	}
}

func resolveTools(cfg config.Config, extra tool.Registry) (tool.Registry, error) {
	if cfg.NoTools {
		return tool.NewRegistry(), nil
	}
	cwd := cfg.CWD
	if cwd == "" {
		var err error
		cwd, err = os.Getwd()
		if err != nil {
			return nil, err
		}
	}
	policy, err := sandbox.New(cwd)
	if err != nil {
		return nil, err
	}
	base := tools.DefaultRegistry(policy)
	if extra != nil {
		all := append(base.List(), extra.List()...)
		base = tool.NewRegistry(all...)
	}
	return tool.Select(base, cfg.Tools), nil
}
