# packages/provider/openaicodex Agent Guide

## Boundary

`openaicodex` adapts OpenAI Codex subscription Responses into normalized provider streams.

## Rules

- OAuth storage/refresh belongs in `auth`/`app`; this package receives an access token/account configuration.
- Preserve resumed-session history encoding, including assistant messages as `output_text`.
- Normalize provider-native tool calls into `provider.Event`.

## Quirks

- Codex OAuth refresh is httptest-covered but live expired-token refresh remains unvalidated.

