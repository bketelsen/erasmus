# packages/provider/openai Agent Guide

## Boundary

`openai` adapts OpenAI API-key Chat Completions into normalized provider streams.

## Rules

- Keep API-key credential handling in `auth`/`app`; this package receives configured client inputs.
- Normalize streamed text, usage, and provider-native tool calls into `provider.Event`.
- Cover protocol behavior with httptest, not live credentials.

## Quirks

- Live OpenAI API-key validation is blocked until credentials are available.

