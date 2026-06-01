# packages/config Agent Guide

## Boundary

`config` defines the persisted user config shape and load/save helpers.

## Rules

- Keep this package data-focused. Resolution belongs in `packages/app`.
- Preserve JSON compatibility unless a migration path is added.
- Do not read auth credentials here.

## Quirks

- Empty config should merge cleanly with defaults in `packages/app`.
