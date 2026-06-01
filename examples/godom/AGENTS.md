# examples/godom Agent Guide

## Boundary

This example demonstrates Erasmus as an embeddable browser UI using `github.com/anupshinde/godom`.

## Rules

- Keep example code focused on demonstrating harness APIs, not inventing new runtime behavior.
- Avoid deadlocks by not calling `Refresh()` indirectly while godom holds component locks.
- Synchronous handlers should mutate fields directly; goroutines may refresh/status-update after handlers return.

## Quirks

- This example is a separate Go module.
- Planned examples include a focused godom chat assistant and a daemon-based autonomous agent.

