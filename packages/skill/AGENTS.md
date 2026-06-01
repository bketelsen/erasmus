# packages/skill Agent Guide

## Boundary

`skill` discovers and represents skills as prompt resources.

## Rules

- Keep skills as data/resources; invocation formatting belongs near prompt/app behavior.
- Discovery should be deterministic and testable with temp directories.
- Do not tie skills to a specific frontend.

## Quirks

- Extension-provided skills are a future possibility; keep the type simple enough to accept multiple sources.

