# Package Boundary

`packages/provider/githubcopilot` adapts GitHub Copilot API-compatible endpoints into the provider event contract.

## Rules

- Treat this provider as experimental because it relies on GitHub Copilot internal API behavior.
- Reuse lower-level OpenAI/Anthropic-compatible adapters where possible; keep Copilot-specific code focused on auth token use, static headers, endpoint selection, and routing.
- Keep Pi-compatible static headers sourced from `packages/auth.GitHubCopilotStaticHeaders`.
- Do not add UI, config-file, or storage behavior here; those belong in `packages/app`, `cmd/erasmus`, or `packages/auth`.
