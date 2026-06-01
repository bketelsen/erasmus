# Providers

Erasmus resolves provider credentials, model metadata, and provider streams in the app layer. The same provider setup is used by one-shot CLI runs, the TUI, RPC, and swarm frontends.

## Model Listing And Refresh

List known built-in, cached, and configured models:

```sh
erasmus models
```

Refresh account-visible models for providers that support discovery:

```sh
erasmus models refresh openai
erasmus models refresh github-copilot
```

The model cache is stored under `$XDG_CACHE_HOME/erasmus/models.json`, or `~/.cache/erasmus/models.json` when `XDG_CACHE_HOME` is unset. User-configured model overrides still take precedence over cached and built-in metadata.

## OpenAI API Key

Store an OpenAI API key:

```sh
erasmus login openai "$OPENAI_API_KEY"
```

Refresh available OpenAI API models:

```sh
erasmus models refresh openai
```

Run a prompt:

```sh
erasmus --provider openai --model gpt-4o-mini run "hello"
```

This path uses OpenAI Chat Completions-compatible streaming and supports provider-native tool calls.

## OpenAI Codex Subscription

Store ChatGPT/Codex OAuth credentials:

```sh
erasmus login openai-codex
```

Open the printed browser URL, complete login, then verify:

```sh
erasmus auth status
```

Run a prompt:

```sh
erasmus --provider openai-codex --model gpt-5.5 run "hello"
```

Codex model discovery is not currently wired to a provider endpoint. Erasmus uses a built-in static Codex catalog plus local user model overrides. OAuth refresh is handled automatically when a stored token expires and a refresh token is available.

## GitHub Copilot

GitHub Copilot support is experimental because it relies on GitHub Copilot API behavior that is not documented as a public inference API.

Store Copilot credentials with GitHub device login:

```sh
erasmus login github-copilot
```

Open the printed GitHub device URL, enter the code, and return to the terminal. Erasmus stores both the Copilot API token and the GitHub access token needed to refresh it.

Refresh account-visible Copilot models:

```sh
erasmus models refresh github-copilot
erasmus models | grep github-copilot
```

The refresh call uses the Copilot `/models` endpoint with the same static headers used by Pi. Known model IDs keep Erasmus' built-in metadata; unknown account-visible IDs are cached with basic metadata so they can still be selected explicitly.

Recommended smoke checks:

```sh
erasmus --provider github-copilot --model gpt-4.1 run "Reply with only: copilot chat ok"
erasmus --provider github-copilot --model gpt-5.3-codex run "Reply with only: copilot responses ok"
erasmus --provider github-copilot --model claude-sonnet-4.5 run "Reply with only: copilot claude ok"
```

Model routing:

```text
gpt-4.x, gpt-4o, Gemini, Grok   OpenAI Chat Completions-compatible path
gpt-5*                          OpenAI Responses-compatible path
claude-*                        Anthropic Messages-compatible path
```

Copilot API tokens are refreshed automatically during runtime resolution and `models refresh` when the stored token is expired. If refresh fails or the GitHub access token is missing, re-login:

```sh
erasmus logout github-copilot
erasmus login github-copilot
erasmus models refresh github-copilot
```

## Auth Inspection

Show configured credentials without printing secrets:

```sh
erasmus auth status
```

Remove credentials:

```sh
erasmus logout openai
erasmus logout openai-codex
erasmus logout github-copilot
```
