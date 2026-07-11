---
id: bw.decision.signal_cursor_commit_hash
status: accepted
date: 2026-07-11
affects:
  - bw.signal.query
---

# The poll cursor is a beadwork-branch commit hash; core never persists it

## Context

Polling needs a store-assigned ordering key with single-host exactly-once semantics.
Beadwork already walks the branch history for `bw recap` via `CommitsSince(hash)`,
and the ref CAS guarantees a linear, fork-free chain on one host.

## Decision

`bw signal query --since <hash>` walks `CommitsSince`, filters commits by their
self-describing `signal` intent lines, reads matched blobs, and returns records
oldest-first plus a new cursor equal to the current branch head. Core does not
persist the cursor — the poller passes it in and stores it wherever it likes.

## Consequences

- Monotonic exactly-once on one host follows from the CAS: every commit attaches to
  the live tip, so the walk from HEAD to the cursor sees each later commit once.
- No emitter-supplied value can influence ordering.
- Cross-machine correctness under `bw sync` (rebase rewrites hashes) is explicitly
  deferred to v2 pubsub — matching the real single-machine pipeline today.
- Core stores no state whose meaning it must know (the small-core litmus test);
  recap's convenience ref remains recap's own affair.
