# `.spec` Agent Guide

Use this folder to maintain authored Spec Led Development subjects for beadwork.

## First Read

1. Read `.spec/README.md`.
2. Read `.spec/decisions/README.md` and any ADRs that affect the subject you are changing.
3. Read the current `.spec/specs/*.spec.md` files before editing.

## Working Rules

- Keep one subject per file.
- Put normative statements in `spec-requirements`; every `must` is falsifiable.
- Add `spec-scenarios` when `given` / `when` / `then` improves clarity.
- Add `spec-meta.decisions` only when a subject depends on a durable cross-cutting ADR.
- Keep ADRs in `.spec/decisions/*.md` for cross-cutting policy only.
- Verification stubs start `execute: false`; flip to `execute: true` only when the
  named test exists and passes. That flip is the ONLY spec edit allowed inside an
  implementation ticket unless the ticket says otherwise.
- Keep verification targets repository-root-relative.
- Use Git history and pull requests as the change log; keep `.spec` current-state only.
- Project verification command (final tier, run-until-green before IMPLEMENTED):

  ```
  make preflight
  ```

- Iteration tier: `make test` or targeted `go test ./internal/<pkg>/... -run <Test>`.
