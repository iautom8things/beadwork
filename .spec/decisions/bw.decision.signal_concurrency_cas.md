---
id: bw.decision.signal_concurrency_cas
status: accepted
date: 2026-07-11
affects:
  - bw.signal.store
  - bw.signal.pipeline
---

# Concurrency via the existing ref-CAS + commitWithRetry; hooks run once, outside the retry

## Context

The spec's bar is parity with concurrent comment writes. Beadwork commits every
mutation through a compare-and-swap on `refs/heads/beadwork` (`casUpdateRef` refusing
with `ErrRefMoved`), and `commitWithRetry` reopens the store and re-runs an `apply`
closure with jittered backoff — exactly how comments already achieve safety.

## Decision

Route only the emit's storage commit through `commitWithRetry`, with an `apply`
closure that re-derives the per-ticket sequence number from a fresh directory read
each attempt and re-stages the already-validated, already-enriched record bytes. The
hook pipeline (enrich → validate → gate) runs exactly once, before and outside the
retry loop. No new locking is introduced.

## Consequences

- Parity with comments is met (arguably exceeded — no shared mutable array). A CAS
  loser re-reads, takes the next sequence number, and re-commits; no record is lost
  or corrupted.
- Gates that are expensive (a test suite) never re-run on CAS contention — re-running
  a gate up to 12 times would have been a serious defect.
- The retry loop cannot change the payload: what retries is a byte re-stage, so a
  stored record is always the one the pipeline approved.
