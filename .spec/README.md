# `.spec`

This folder is the repo-local Spec Led layer for beadwork.

## Canonical Layout

- `README.md` (authored)
- `AGENTS.md` (authored, local operating guidance for agents working in this folder)
- `decisions/README.md` (authored ADR guidance for this workspace)
- `decisions/*.md` (authored durable cross-cutting ADRs)
- `specs/*.spec.md` (authored subject specs)

## Core Loop

1. Read the subject spec(s) named in your ticket's `Advances:` line before editing code.
2. Make the smallest code, docs, or test change that moves the behavior.
3. Add or tighten the smallest proof when behavior changed — flip the covering
   verification stub from `execute: false` to `execute: true` once the named test exists
   and passes.
4. Keep subject specs current-truth: when a touched surface's behavior changes, update
   the requirement statements in the same change.
5. Before declaring a ticket done, run the project verification command:

   ```
   make preflight
   ```

   (fmt-check, tidy-check, build, vet, staticcheck, test with coverage — the same
   sequence CI runs.) Targeted iteration uses `make test` or
   `go test ./internal/<pkg>/...`.

This is a Go project without specled_ex tooling: there is no `mix spec.*` machinery
here. Specs are validated by review and by keeping `covers:` ids in sync with real
test names.
