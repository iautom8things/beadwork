# Signal Store

Durable, append-only, immutable storage of successful signals on the beadwork branch,
with a replayable `signal` intent and concurrency at parity with comment writes.

```yaml spec-meta
id: bw.signal.store
kind: module
status: draft
summary: Immutable per-ticket signal records under signals/<ticket>/<NNNN>.json with a replayable signal intent and CAS-retry concurrency.
surface:
  - internal/issue/signal.go
  - internal/intent/intent.go
  - cmd/bw/signal.go
decisions:
  - bw.decision.signal_storage_intent
  - bw.decision.signal_concurrency_cas
```

## Requirements

```yaml spec-requirements
- id: bw.signal.store.immutable_snapshot
  statement: A stored signal shall be a self-contained JSON snapshot (seq, type, ticket, final payload, store-assigned emitted_at) written once to signals/<ticket-id>/<NNNN>.json and never modified afterward.
  priority: must
  stability: stable
- id: bw.signal.store.store_assigned_sequence
  statement: The per-ticket sequence number shall be assigned by the store (derived from the ticket's existing signal files), never supplied by the emitter.
  priority: must
  stability: stable
- id: bw.signal.store.intent_verb
  statement: Every stored signal shall be committed with a self-describing intent line of the form "signal <ticket-id> <type> <path>".
  priority: must
  stability: stable
- id: bw.signal.store.replay_loud_fail
  statement: Sync replay of a signal intent shall re-stage the original record blob (recovered from the current tree or the pre-reset object database) and shall fail the replay loudly if the blob is unreachable, never silently dropping a signal.
  priority: must
  stability: stable
- id: bw.signal.store.replay_no_hooks
  statement: Sync replay of a signal intent shall never re-run the hook pipeline.
  priority: must
  stability: stable
- id: bw.signal.store.concurrent_parity
  statement: Concurrent emits (same or different tickets, one host) shall neither lose nor corrupt any signal record, using the same CAS-plus-retry commit path comments use.
  priority: must
  stability: stable
- id: bw.signal.store.history_outlives_definitions
  statement: Stored signals shall be returned by queries and bw show as-emitted even when their type has been deleted or its schema changed, without re-validation.
  priority: must
  stability: stable
```

## Scenarios

```yaml spec-scenarios
- id: bw.signal.store.emit_and_reread
  given:
    - a repo defining a verify type
  when:
    - bw signal emit bw-x verify --field phase=PASS succeeds
  then:
    - signals/bw-x/0001.json exists on the beadwork branch containing seq 1, type verify, the payload, and a store-assigned emitted_at
    - the commit message's first intent line is "signal bw-x verify signals/bw-x/0001.json"
  covers:
    - bw.signal.store.immutable_snapshot
    - bw.signal.store.store_assigned_sequence
    - bw.signal.store.intent_verb
- id: bw.signal.store.concurrent_same_ticket
  given:
    - two emits racing on the same ticket
  when:
    - both run to completion
  then:
    - two records exist with distinct sequence numbers
    - neither record's content is lost or interleaved
  covers:
    - bw.signal.store.concurrent_parity
- id: bw.signal.store.replay_survives_sync
  given:
    - a locally emitted signal and a diverged remote beadwork branch
  when:
    - bw sync rebases and replays intents
  then:
    - the signal record is re-staged byte-identical from the object database
    - no hook executes during replay
    - a missing blob aborts the replay with a loud error
  covers:
    - bw.signal.store.replay_loud_fail
    - bw.signal.store.replay_no_hooks
- id: bw.signal.store.orphaned_type_returned
  given:
    - a stored verify signal whose type was later removed from .beadwork/signals.yml
  when:
    - bw signal query or bw show reads the ticket
  then:
    - the record is returned as-emitted without re-validation
  covers:
    - bw.signal.store.history_outlives_definitions
```

## Verification

```yaml spec-verification
- kind: command
  target: go test ./test/ -run TestSignalEmitAndReread
  execute: false
  covers:
    - bw.signal.store.immutable_snapshot
    - bw.signal.store.store_assigned_sequence
    - bw.signal.store.intent_verb
- kind: command
  target: go test ./internal/issue/ -run TestSignalConcurrentEmits
  execute: false
  covers:
    - bw.signal.store.concurrent_parity
- kind: command
  target: go test ./internal/intent/ -run TestReplaySignal
  execute: false
  covers:
    - bw.signal.store.replay_loud_fail
    - bw.signal.store.replay_no_hooks
- kind: command
  target: go test ./cmd/bw/ -run TestSignalHistoryOutlivesDefinitions
  execute: false
  covers:
    - bw.signal.store.history_outlives_definitions
```
