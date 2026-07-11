---
id: bw.decision.signal_storage_intent
status: accepted
date: 2026-07-11
affects:
  - bw.signal.store
  - bw.signal.query
---

# Signals are per-record files with a self-describing replayable intent verb

## Context

Signals need durable, append-only, immutable storage in the git-backed store, at
concurrency parity with comments, and must survive `bw sync` intent replay. Comments
serialize into a single JSON array per issue; attachments store per-blob files under
`attachments/<id>/<path>` with a loud-fail `attach` replay intent.

## Decision

Each signal is an immutable self-contained JSON snapshot at
`signals/<ticket-id>/<NNNN>.json`, where `NNNN` is a store-assigned, zero-padded
per-ticket sequence derived from a fresh directory read (deterministic under
`BW_CLOCK`, unlike timestamp names). Every emit commits with the self-describing
intent line `signal <ticket-id> <type> <path>`. Replay mirrors `replayAttach`:
recover the blob from the current tree or the pre-reset object database, re-stage
it, and fail loudly if unreachable. Replay never re-runs hooks.

## Consequences

- Immutability and history-outliving-definitions fall out structurally: the record
  embeds the resolved type and final payload and references nothing in config.
- The self-describing intent line lets since-cursor queries filter by ticket/type
  from commit messages without reading blobs.
- Per-record files mean emits on different tickets never touch the same path, and
  same-ticket emits contend only on the sequence number.
- A signal can never be silently dropped by sync; a lost blob aborts replay loudly.
