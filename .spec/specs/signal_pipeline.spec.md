# Signal Hook Pipeline

The unskippable, user-defined hook pipeline on emission: enrich → validate → gate →
on-blocked → post-emit, with fail-closed malfunction semantics and never-hang execution.

```yaml spec-meta
id: bw.signal.pipeline
kind: module
status: draft
summary: Four-moment host-executable hook pipeline with stdin-JSON I/O, exit-code verdicts, fail-closed malfunctions, and per-hook timeouts.
surface:
  - internal/signal/hooks.go
  - cmd/bw/signal.go
decisions:
  - bw.decision.signal_hook_io
  - bw.decision.signal_concurrency_cas
```

## Requirements

```yaml spec-requirements
- id: bw.signal.pipeline.unskippable
  statement: bw signal emit shall offer no flag or environment switch that bypasses the repo's configured hook pipeline.
  priority: must
  stability: stable
- id: bw.signal.pipeline.emit_sequence
  statement: Emission shall execute exactly in the order type-existence check, enrich, full-schema validation of the post-enrich payload, gate, durable store, post-emit.
  priority: must
  stability: stable
- id: bw.signal.pipeline.optional_moments_and_scopes
  statement: Hooks shall attach per signal type or repo-global, each of the four moments shall be independently optional, and a config with zero hooks shall be valid.
  priority: must
  stability: stable
- id: bw.signal.pipeline.stdin_json_io
  statement: Hooks shall receive the signal record as JSON on stdin with stdin closed after writing, and an enrich hook exiting 0 with a JSON object on stdout shall have that object replace the payload (empty stdout leaves it unchanged).
  priority: must
  stability: stable
- id: bw.signal.pipeline.gate_block_refuses
  statement: A gate exiting 1 shall refuse the emission with the gate's output as the reason, exit non-zero, persist nothing, and fire on-blocked hooks.
  priority: must
  stability: stable
- id: bw.signal.pipeline.malfunction_fails_closed
  statement: An enrich or gate hook that crashes (exit >= 2 or killed), times out, or (for enrich) emits non-JSON stdout shall refuse the emission with a malfunction error distinguishable from a deliberate gate block, persisting nothing and not firing on-blocked.
  priority: must
  stability: stable
- id: bw.signal.pipeline.on_blocked_isolated
  statement: An on-blocked hook's own failure shall be reported without changing the refusal outcome.
  priority: must
  stability: stable
- id: bw.signal.pipeline.post_emit_after_store
  statement: Post-emit hooks shall run only after the signal is durably committed, and their failure shall surface a warning while the emit still reports the signal as stored and exits 0.
  priority: must
  stability: stable
- id: bw.signal.pipeline.hooks_once_outside_retry
  statement: The hook pipeline shall run exactly once per emit invocation, outside the storage commit's CAS retry loop.
  priority: must
  stability: stable
- id: bw.signal.pipeline.never_hang
  statement: Each hook shall run under a configurable timeout (default 30s) with captured stdout/stderr and no TTY access, so a hanging or prompting hook is killed and classified as a malfunction rather than hanging bw.
  priority: must
  stability: stable
```

## Scenarios

```yaml spec-scenarios
- id: bw.signal.pipeline.attribution_pattern
  given:
    - a repo-global enrich hook that stamps payload.session_id from BW_SESSION_ID and exits 1 when it is absent
    - a schema requiring session_id
  when:
    - bw signal emit runs with BW_SESSION_ID set
  then:
    - the stored payload contains the stamped session_id
    - the same emit without BW_SESSION_ID is refused before the gate runs
  covers:
    - bw.signal.pipeline.stdin_json_io
    - bw.signal.pipeline.emit_sequence
- id: bw.signal.pipeline.gate_blocks_red_tests
  given:
    - a completed type whose gate script exits 1 with failing test output
  when:
    - bw signal emit bw-x completed runs
  then:
    - the emit exits non-zero labeled BLOCKED with the gate output
    - signals/bw-x/ contains no new record
    - the on-blocked hook fired once
  covers:
    - bw.signal.pipeline.gate_block_refuses
- id: bw.signal.pipeline.broken_gate_not_bypass
  given:
    - a gate script that exits 2 (crash) and another that sleeps past the timeout
  when:
    - emits run against each
  then:
    - both are refused labeled MALFUNCTION, distinct from BLOCKED
    - nothing is stored and on-blocked does not fire
    - the timed-out hook is killed within hook_timeout
  covers:
    - bw.signal.pipeline.malfunction_fails_closed
    - bw.signal.pipeline.never_hang
- id: bw.signal.pipeline.post_emit_failure_still_stored
  given:
    - a post-emit notify hook that exits 1
  when:
    - an otherwise-valid emit runs
  then:
    - the signal record is durably stored
    - the command prints a WARNING naming the failed hook and exits 0
  covers:
    - bw.signal.pipeline.post_emit_after_store
- id: bw.signal.pipeline.cas_retry_single_hook_run
  given:
    - an emit whose storage commit loses the ref CAS race once
  when:
    - the commit retries and succeeds
  then:
    - every hook ran exactly once
  covers:
    - bw.signal.pipeline.hooks_once_outside_retry
- id: bw.signal.pipeline.no_bypass_flag
  given:
    - a repo with a configured gate
  when:
    - bw signal emit --help is inspected and emit is invoked with any documented flag combination
  then:
    - no flag or env switch skips the pipeline
  covers:
    - bw.signal.pipeline.unskippable
    - bw.signal.pipeline.optional_moments_and_scopes
- id: bw.signal.pipeline.on_blocked_crash_isolated
  given:
    - a gate that blocks (exit 1) and an on-blocked hook that itself crashes
  when:
    - the emit runs
  then:
    - the refusal outcome is unchanged (BLOCKED, non-zero exit, nothing stored)
    - the on-blocked failure is reported on stderr
  covers:
    - bw.signal.pipeline.on_blocked_isolated
```

## Verification

```yaml spec-verification
- kind: command
  target: go test ./internal/signal/ -run TestPipelineEmitSequence
  execute: false
  covers:
    - bw.signal.pipeline.emit_sequence
    - bw.signal.pipeline.optional_moments_and_scopes
- kind: command
  target: go test ./internal/signal/ -run TestHookStdinJSONContract
  execute: false
  covers:
    - bw.signal.pipeline.stdin_json_io
- kind: command
  target: go test ./test/ -run TestSignalGateBlocks
  execute: false
  covers:
    - bw.signal.pipeline.gate_block_refuses
- kind: command
  target: go test ./test/ -run TestSignalHookMalfunction
  execute: false
  covers:
    - bw.signal.pipeline.malfunction_fails_closed
- kind: command
  target: go test ./internal/signal/ -run TestOnBlockedFailureIsolated
  execute: false
  covers:
    - bw.signal.pipeline.on_blocked_isolated
- kind: command
  target: go test ./test/ -run TestSignalPostEmitWarning
  execute: false
  covers:
    - bw.signal.pipeline.post_emit_after_store
- kind: command
  target: go test ./cmd/bw/ -run TestSignalHooksRunOnceUnderRetry
  execute: false
  covers:
    - bw.signal.pipeline.hooks_once_outside_retry
- kind: command
  target: go test ./internal/signal/ -run TestHookTimeoutKilled
  execute: false
  covers:
    - bw.signal.pipeline.never_hang
- kind: command
  target: go test ./cmd/bw/ -run TestSignalEmitNoBypassFlag
  execute: false
  covers:
    - bw.signal.pipeline.unskippable
```
