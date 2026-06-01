# cmd/erasmus Agent Guide

## Boundary

This package is the executable CLI entrypoint. It should parse command shape and delegate real behavior to `packages/app` and lower runtime packages.

## Rules

- Use Fang v2 over Cobra for command execution.
- Keep runtime semantics out of this package.
- Preserve existing smoke command behavior when refactoring CLI structure.
- Keep command-specific files small and delegate shared behavior to `packages/app`.
- Use Cobra/pflag flags for command options and Viper-backed root settings for config/env overrides.

## Quirks

- `--version` intentionally prints `erasmus 0.1.0-dev` for backward compatibility.
- The `skill` command accepts arbitrary prompt text; be careful before adding flags that could consume user input.
