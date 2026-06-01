# packages/model Agent Guide

## Boundary

`model` defines provider-independent model metadata, usage accounting, and catalog interfaces/defaults.

## Rules

- Keep this package independent from auth and provider clients.
- Defaults should be deterministic and test-friendly.
- Reasoning capability metadata belongs here, not in frontends.

## Quirks

- The current catalog is static and intentionally small; live catalog/user override work belongs behind the catalog interface.

