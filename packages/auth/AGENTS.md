# packages/auth Agent Guide

## Boundary

`auth` owns provider credentials, file-backed and memory stores, OAuth callback helpers, and OpenAI OAuth token refresh primitives.

## Rules

- Do not print CLI output here; return structured errors and values for callers.
- Keep credential persistence independent from provider clients.
- Tests should use memory stores or temp files.

## Quirks

- OpenAI Codex OAuth refresh is httptest-covered, but live refresh with an actually expired token remains unvalidated.
- Preserve fallback behavior where `openai-codex` can use stored OpenAI OAuth credentials when appropriate.

