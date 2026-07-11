---
id: bw.decision.signal_config_working_tree
status: accepted
date: 2026-07-11
affects:
  - bw.signal.config
  - bw.signal.pipeline
  - bw.signal.adoption
---

# Signal config lives in the working tree, never on the beadwork branch

## Context

Beadwork had no repo-level config; the existing `.bwconfig` rides the beadwork branch
and syncs via a `config` intent. Signal type definitions and hook wiring together
decide what code executes on emit, so their placement is a trust decision. Three
placements were evaluated: beadwork branch (synced), working tree, and a split
(types synced, hook paths local).

## Decision

Type definitions AND hook wiring both live in `.beadwork/signals.yml` +
`.beadwork/hooks/` in the working tree, checked into `main` and reviewed via PR —
never on the beadwork branch. "Unskippable" means `bw signal emit` offers no bypass
of the host's configured pipeline, not that every clone runs identical hooks.

## Consequences

- `bw sync` (and a `git pull` of the beadwork branch) can never alter emit behavior;
  all signal config flows through the same trust gate as the repo's code. The split
  option is rejected because synced type schemas alone could still redirect which
  gate path runs.
- Config absent = feature dormant (full additivity); config present but malformed =
  fail-closed CONFIG ERROR on `bw signal *` commands only.
- Different hosts may legitimately run different hooks (capability differences);
  provenance remains detectable-not-cryptographic per the spec.
- Sync/replay grows no config-merge concern; signal *records* still ride the branch.
