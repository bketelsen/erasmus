# packages/provider/fake Agent Guide

## Boundary

`fake` provides deterministic provider behavior for tests and smoke paths.

## Rules

- Keep it network-free and credential-free.
- Maintain stable outputs used by smoke tests.
- Add scripted behavior here when tests need provider events without real APIs.

## Quirks

- Fake-provider behavior must not depend on local config/auth.

