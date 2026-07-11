# Signal Query Surface

Structured polling and human rendering: since-cursor queries over the beadwork branch
commit chain, `bw show` signal trails, and discovery surfaces.

```yaml spec-meta
id: bw.signal.query
kind: module
status: draft
summary: Since-cursor JSON query over CommitsSince with single-host exactly-once semantics, plus bw show SIGNALS rendering and help/prime discovery.
surface:
  - cmd/bw/signal_query.go
  - cmd/bw/show.go
  - internal/md/render.go
  - prompts/prime.md
decisions:
  - bw.decision.signal_cursor_commit_hash
```

## Requirements

```yaml spec-requirements
- id: bw.signal.query.since_cursor
  statement: bw signal query shall accept a --since cursor that is a beadwork-branch commit hash, return matching signal records oldest-first with a new cursor equal to the branch head, and support filtering by ticket and type.
  priority: must
  stability: stable
- id: bw.signal.query.exactly_once_single_host
  statement: On a single host, polling with the returned cursor shall yield every subsequently stored signal exactly once — never missed, never duplicated — including across interleaved non-signal commits.
  priority: must
  stability: stable
- id: bw.signal.query.store_assigned_cursor
  statement: The cursor's ordering key shall be assigned by the store; no emitter-supplied value shall influence query ordering.
  priority: must
  stability: stable
- id: bw.signal.query.core_stateless_cursor
  statement: Core shall not persist the poll cursor; the caller owns cursor persistence.
  priority: must
  stability: stable
- id: bw.signal.query.show_rendering
  statement: bw show <id> shall render the ticket's signals chronologically by sequence in a SIGNALS section (selectable via --only signals), rendering signals whose type definition no longer exists from their stored snapshot with a type-not-defined note rather than dropping or erroring.
  priority: must
  stability: stable
- id: bw.signal.query.discovery
  statement: bw --help shall list a Signals command group, and prime output shall mention signals only when the repo defines signal types.
  priority: should
  stability: evolving
```

## Scenarios

```yaml spec-scenarios
- id: bw.signal.query.poll_loop
  given:
    - cursor c0 taken before three verify signals are emitted with comments and status changes interleaved
  when:
    - bw signal query --ticket bw-x --since c0 --json runs, then runs again with the returned cursor
  then:
    - the first poll returns exactly the three records oldest-first plus a new cursor
    - the second poll returns an empty list
  covers:
    - bw.signal.query.since_cursor
    - bw.signal.query.exactly_once_single_host
- id: bw.signal.query.emitter_cannot_reorder
  given:
    - an emit whose payload contains timestamp-like fields chosen by the emitter
  when:
    - signals are queried with a since-cursor
  then:
    - ordering follows the store's commit chain, unaffected by payload contents
  covers:
    - bw.signal.query.store_assigned_cursor
    - bw.signal.query.core_stateless_cursor
- id: bw.signal.query.orphaned_type_render
  given:
    - a stored audit signal whose type was removed from .beadwork/signals.yml
  when:
    - bw show bw-x runs
  then:
    - the SIGNALS section renders the record with a type-not-defined note
    - the command does not error
  covers:
    - bw.signal.query.show_rendering
- id: bw.signal.query.dormant_discovery
  given:
    - a repo with no signal types defined
  when:
    - prime output is generated
  then:
    - no signals paragraph appears
  covers:
    - bw.signal.query.discovery
```

## Verification

```yaml spec-verification
- kind: command
  target: go test ./test/ -run TestSignalQuerySinceCursorExactlyOnce
  execute: true
  covers:
    - bw.signal.query.since_cursor
    - bw.signal.query.exactly_once_single_host
- kind: command
  target: go test ./cmd/bw/ -run TestSignalQueryStoreAssignedCursor
  execute: true
  covers:
    - bw.signal.query.store_assigned_cursor
    - bw.signal.query.core_stateless_cursor
- kind: command
  target: go test ./cmd/bw/ -run TestShowSignalsSection
  execute: true
  covers:
    - bw.signal.query.show_rendering
- kind: command
  target: go test ./cmd/bw/ -run TestUsageListsSignalGroup
  execute: true
  covers:
    - bw.signal.query.discovery
- kind: command
  target: go test ./cmd/bw/ -run TestPrimeMentionsSignalsWhenDefined
  execute: true
  covers:
    - bw.signal.query.discovery
```
