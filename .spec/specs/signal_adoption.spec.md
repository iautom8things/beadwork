# Signal Adoption (Dual-Mode Skills Migration)

The v1 acceptance bar: on one signal-adopted repo, the full MARKERS.md pipeline runs
end-to-end in signals mode, proving the prose-parse mechanisms are deletable. The
skills run dual-mode during migration.

```yaml spec-meta
id: bw.signal.adoption
kind: workflow
status: draft
summary: Dual-mode orchestrate-epic/implement skills consuming typed signals on one adopted repo, with all 10 MARKERS.md markers defined as signal types.
surface:
  - ~/Documents/my-vault/LLMs/skills/orchestrate-epic/SKILL.md
  - ~/Documents/my-vault/LLMs/skills/implement/SKILL.md
  - <adopted-repo>/.beadwork/signals.yml
  - <adopted-repo>/.beadwork/hooks/
decisions:
  - bw.decision.signal_config_working_tree
```

## Requirements

```yaml spec-requirements
- id: bw.signal.adoption.all_markers_defined
  statement: The adopted repo's .beadwork/signals.yml shall define every marker in the normative annex MARKERS.md (all 10) as a signal type, with routing targets expressed as enum payload fields.
  priority: must
  stability: stable
- id: bw.signal.adoption.dual_mode_skills
  statement: The orchestrate-epic and implement skills shall consume and emit typed signals on repos that define signal types, and shall fall back to comment-marker behavior unchanged on repos that do not.
  priority: must
  stability: stable
- id: bw.signal.adoption.zero_prose_parse
  statement: In signals mode, the skills shall make no use of the comment-marker prose-parse mechanisms (first-non-blank-non-quoted-line rules, quote stripping, jq comment polling, last_bounce_handled_at debounce).
  priority: must
  stability: stable
- id: bw.signal.adoption.end_to_end_proof
  statement: A real epic on the adopted repo shall run the full pipeline end-to-end in signals mode, including a VERIFY FAIL, an AUDIT BOUNCE routed to the implementer via the typed target field, and a gated completed emission.
  priority: must
  stability: stable
```

## Scenarios

```yaml spec-scenarios
- id: bw.signal.adoption.bounce_loop_in_signals_mode
  given:
    - an adopted repo defining all 10 marker types with an enrich attribution hook and a completed gate
    - the comment-marker parser disabled
  when:
    - /orchestrate-epic drives an epic through a VERIFY FAIL then AUDIT BOUNCE to implementer then completed loop
  then:
    - every phase transition is driven solely by typed signal records returned from since-cursor polls
    - no prose-parse mechanism executes
  covers:
    - bw.signal.adoption.end_to_end_proof
    - bw.signal.adoption.zero_prose_parse
    - bw.signal.adoption.all_markers_defined
- id: bw.signal.adoption.non_adopted_repo_unchanged
  given:
    - a repo with no signal types defined
  when:
    - /implement and /orchestrate-epic run a ticket
  then:
    - comment-marker emission and polling behave exactly as before
  covers:
    - bw.signal.adoption.dual_mode_skills
```

## Verification

```yaml spec-verification
- kind: command
  target: 'manual: end-to-end epic run on the adopted repo in signals mode (VERIFY FAIL -> AUDIT BOUNCE -> completed) with the comment parser disabled, recorded on the epic ticket'
  execute: false
  covers:
    - bw.signal.adoption.end_to_end_proof
    - bw.signal.adoption.zero_prose_parse
    - bw.signal.adoption.all_markers_defined
- kind: source_file
  target: ~/Documents/my-vault/LLMs/skills/orchestrate-epic/SKILL.md
  execute: true
  covers:
    - bw.signal.adoption.dual_mode_skills
```
