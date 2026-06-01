package model

// DefaultCatalog returns the built-in model catalog used by early app resolution.
func DefaultCatalog() StaticCatalog {
	return StaticCatalog{
		Models: []Model{
			{Provider: "fake", ID: "echo", DisplayName: "Fake Echo", ContextWindow: 128000, MaxOutput: 4096, Source: "builtin"},
			{Provider: "openai", ID: "gpt-4o-mini", DisplayName: "GPT-4o mini", ContextWindow: 128000, MaxOutput: 16384, Source: "builtin"},
			codexModel("gpt-5.5", "GPT-5.5"),
			codexModel("gpt-5.4", "GPT-5.4"),
			codexModel("gpt-5.4-mini", "GPT-5.4 mini"),
			codexModel("gpt-5.4-nano", "GPT-5.4 nano"),
			codexModel("gpt-5.3-codex", "GPT-5.3 Codex"),
			codexModel("gpt-5.2", "GPT-5.2"),
			codexModel("gpt-5.1", "GPT-5.1"),
			copilotClaudeModel("claude-haiku-4.5", "Claude Haiku 4.5", 200000, 64000),
			copilotClaudeModel("claude-opus-4.5", "Claude Opus 4.5", 200000, 32000),
			copilotClaudeModel("claude-opus-4.6", "Claude Opus 4.6", 1000000, 32000),
			copilotClaudeModel("claude-opus-4.7", "Claude Opus 4.7", 200000, 32000),
			copilotClaudeModel("claude-opus-4.8", "Claude Opus 4.8", 200000, 64000),
			copilotClaudeModel("claude-sonnet-4.5", "Claude Sonnet 4.5", 200000, 32000),
			copilotClaudeModel("claude-sonnet-4.6", "Claude Sonnet 4.6", 1000000, 32000),
			copilotModel("gemini-2.5-pro", "Gemini 2.5 Pro", 128000, 64000, false),
			copilotModel("gemini-3-flash-preview", "Gemini 3 Flash", 128000, 64000, true),
			copilotModel("gemini-3.1-pro-preview", "Gemini 3.1 Pro Preview", 200000, 64000, true),
			copilotModel("gemini-3.5-flash", "Gemini 3.5 Flash", 200000, 64000, true),
			copilotModel("gpt-4.1", "GPT-4.1", 128000, 16384, false),
			copilotModel("gpt-4o", "GPT-4o", 128000, 4096, false),
			copilotModel("gpt-5-mini", "GPT-5-mini", 264000, 64000, true),
			copilotModel("gpt-5.2", "GPT-5.2", 400000, 128000, true),
			copilotModel("gpt-5.2-codex", "GPT-5.2-Codex", 400000, 128000, true),
			copilotModel("gpt-5.3-codex", "GPT-5.3-Codex", 400000, 128000, true),
			copilotModel("gpt-5.4", "GPT-5.4", 400000, 128000, true),
			copilotModel("gpt-5.4-mini", "GPT-5.4 Mini", 400000, 128000, true),
			copilotModel("gpt-5.5", "GPT-5.5", 400000, 128000, true),
			copilotModel("grok-code-fast-1", "Grok Code Fast 1", 128000, 64000, true),
		},
		Defaults: map[string]string{"fake": "echo", "openai": "gpt-4o-mini", "openai-codex": "gpt-5.5", "github-copilot": "gpt-5.5"},
	}
}

func codexModel(id, displayName string) Model {
	return Model{
		Provider:      "openai-codex",
		ID:            id,
		DisplayName:   displayName,
		ContextWindow: 400000,
		MaxOutput:     100000,
		Reasoning:     true,
		Source:        "builtin",
	}
}

func copilotClaudeModel(id, displayName string, contextWindow, maxOutput int) Model {
	return copilotModel(id, displayName, contextWindow, maxOutput, true)
}

func copilotModel(id, displayName string, contextWindow, maxOutput int, reasoning bool) Model {
	return Model{
		Provider:      "github-copilot",
		ID:            id,
		DisplayName:   displayName,
		ContextWindow: contextWindow,
		MaxOutput:     maxOutput,
		Reasoning:     reasoning,
		Source:        "builtin",
	}
}
