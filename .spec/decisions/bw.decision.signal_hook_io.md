---
id: bw.decision.signal_hook_io
status: accepted
date: 2026-07-11
affects:
  - bw.signal.pipeline
---

# Hook I/O: stdin JSON in, stdout JSON (enrich) out, exit-code verdicts

## Context

Hooks are arbitrary host executables that must receive structured payloads, let
enrich mutate them, let gates refuse with a reason, distinguish deliberate blocks
from malfunctions, and never hang or prompt (beadwork's TTY-gated never-prompt
invariant).

## Decision

- The signal record is written to the hook's stdin as JSON, then stdin is closed.
  `BW_SIGNAL_TYPE`, `BW_SIGNAL_TICKET`, `BW_SIGNAL_MOMENT` ride the environment as
  convenience scalars; the parent environment is inherited (the attribution pattern
  reads spawner-controlled vars). CWD is the repo root.
- Enrich: exit 0 + JSON object on stdout replaces the payload; exit 0 + empty stdout
  leaves it unchanged; non-zero exit or non-JSON stdout is a malfunction.
- Gate: exit 0 allows; exit 1 is a deliberate block (stdout+stderr become the refusal
  reason, on-blocked fires); exit ≥ 2, killed, or timeout is a malfunction (distinct
  label, on-blocked does not fire). Both refuse and store nothing.
- on-blocked failure is reported without changing the refusal; post-emit runs after
  the durable commit and its failure warns while the emit exits 0.
- Every hook runs under a per-hook deadline (`hook_timeout`, default 30s):
  SIGTERM, grace, SIGKILL, then malfunction. stdout/stderr are captured buffers,
  never the TTY.

## Consequences

- A broken guardrail can never be a bypass (fail closed), yet thrash counters see
  only policy blocks, never infra failures.
- Nested payloads survive intact (stdin JSON is authoritative; env carries scalars
  only), and a hook reading stdin to EOF cannot deadlock.
- Post-emit failure ≠ emit failure, so agents have no reason to retry a stored
  signal — the no-double-store guarantee is structural.
