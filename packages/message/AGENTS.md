# packages/message Agent Guide

## Boundary

`message` defines canonical roles and content types used across the runtime.

## Rules

- Keep provider-specific payload details out of this package.
- Add content variants only when they are useful across providers/sessions/frontends.
- Preserve JSON compatibility for durable session storage.

## Quirks

- Providers adapt these types at the boundary; avoid leaking provider-native message types upward.

