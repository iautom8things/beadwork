---
id: bw.decision.signal_schema_yaml
status: accepted
date: 2026-07-11
affects:
  - bw.signal.config
  - bw.signal.adoption
---

# Type-definition schema: small YAML, exactly as expressive as the 10 markers

## Context

The schema must express every marker in the normative annex MARKERS.md — pure tokens
(IMPLEMENTED), enum-valued phases (VERIFY: PASS|FAIL), and structured multi-field
payloads with enum routing targets (AUDIT: BOUNCE → implementer|verifier) — without
importing a general schema language. yaml.v3 is already a dependency.

## Decision

`.beadwork/signals.yml` declares types with fields of kind `string`, `int`, `bool`,
or `enum` (with `values`), a `required` flag, and a conditional
`required_when: {field, equals}`. Pure-token types declare no fields. Repo-global and
per-type hooks attach in the same file. No JSON Schema, no nesting, no custom
validators — that exhausts what the annex needs.

## Consequences

- All 10 markers translate mechanically (routing targets become enum fields the
  orchestrator consumes as data, not prose).
- Schemas may require enrich-supplied fields (e.g. `session_id`) because validation
  runs on the final post-enrich payload.
- Anything more expressive (cross-field logic beyond required_when, nested objects)
  is deliberately out — a future need is a v2 conversation, not a v1 extension
  point.
