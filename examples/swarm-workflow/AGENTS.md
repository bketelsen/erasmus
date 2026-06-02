# examples/swarm-workflow Agent Guide

## Boundary

This example demonstrates an in-process swarm server with named agents coordinated into a plan/review/execute workflow.

## Rules

- Keep provider behavior deterministic and local; the example must run without credentials.
- Use `packages/swarm` for named agent supervision instead of hand-rolled goroutine maps.
- Keep the workflow explicit: planner output feeds reviewer input, and both feed executor input.
- Tests should verify named agents, event logs, and the combined workflow summary.

## Quirks

- This example is a separate Go module and is validated by `make test-examples`.
- The fake streams are role-specific so the example shows orchestration shape, not model quality.
