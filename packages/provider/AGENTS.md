# packages/provider Agent Guide

## Boundary

`provider` defines provider-neutral stream interfaces, requests, events, and options.

## Rules

- Keep concrete provider HTTP/API code in subpackages.
- Provider events are normalized for `loop`; do not expose frontend behavior here.
- Requests should use canonical `message`, `model`, and `tool` types.

## Quirks

- Tool call support must be represented as normalized provider events before the loop sees it.

