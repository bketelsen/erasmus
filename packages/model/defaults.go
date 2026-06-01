package model

// DefaultCatalog returns the built-in model catalog used by early app resolution.
func DefaultCatalog() StaticCatalog {
	return StaticCatalog{
		Models: []Model{
			{Provider: "fake", ID: "echo", DisplayName: "Fake Echo", ContextWindow: 128000, MaxOutput: 4096, Source: "builtin"},
			{Provider: "openai", ID: "gpt-4o-mini", DisplayName: "GPT-4o mini", ContextWindow: 128000, MaxOutput: 16384, Source: "builtin"},
			{Provider: "openai-codex", ID: "gpt-5.5", DisplayName: "GPT-5.5", ContextWindow: 400000, MaxOutput: 100000, Reasoning: true, Source: "builtin"},
			{Provider: "openai-codex", ID: "gpt-5.4", DisplayName: "GPT-5.4", ContextWindow: 400000, MaxOutput: 100000, Reasoning: true, Source: "builtin"},
			{Provider: "openai-codex", ID: "gpt-5.4-mini", DisplayName: "GPT-5.4 mini", ContextWindow: 400000, MaxOutput: 100000, Reasoning: true, Source: "builtin"},
			{Provider: "openai-codex", ID: "gpt-5.3-codex", DisplayName: "GPT-5.3 Codex", ContextWindow: 400000, MaxOutput: 100000, Reasoning: true, Source: "builtin"},
			{Provider: "openai-codex", ID: "gpt-5.3-codex-spark", DisplayName: "GPT-5.3 Codex Spark", ContextWindow: 400000, MaxOutput: 100000, Reasoning: true, Source: "builtin"},
			{Provider: "openai-codex", ID: "gpt-5.2", DisplayName: "GPT-5.2", ContextWindow: 400000, MaxOutput: 100000, Reasoning: true, Source: "builtin"},
		},
		Defaults: map[string]string{"fake": "echo", "openai": "gpt-4o-mini", "openai-codex": "gpt-5.5"},
	}
}
