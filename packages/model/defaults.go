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
		},
		Defaults: map[string]string{"fake": "echo", "openai": "gpt-4o-mini", "openai-codex": "gpt-5.5"},
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
