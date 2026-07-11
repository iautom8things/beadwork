# Signal Type Configuration

Repo-level signal type definitions: the schema language, where config lives, and the
fail-closed rules. Beadwork ships zero built-in signal types — the namespace is 100%
repo-defined via `.beadwork/signals.yml` in the working tree.

```yaml spec-meta
id: bw.signal.config
kind: policy
status: draft
summary: Repo-defined signal types in working-tree .beadwork/signals.yml with fail-closed parsing and full payload schema enforcement.
surface:
  - internal/signal/config.go
  - internal/signal/schema.go
  - internal/signal/validate.go
decisions:
  - bw.decision.signal_config_working_tree
  - bw.decision.signal_schema_yaml
```

## Requirements

```yaml spec-requirements
- id: bw.signal.config.working_tree_source
  statement: Signal type definitions and hook wiring shall be read from .beadwork/signals.yml in the repository working tree, never from the beadwork branch.
  priority: must
  stability: stable
- id: bw.signal.config.dormant_when_absent
  statement: When no .beadwork/signals.yml exists, every bw command shall behave byte-for-byte as it does without the signals feature, and bw signal emit shall refuse any type with a "no signal types defined" validation error.
  priority: must
  stability: stable
- id: bw.signal.config.fail_closed_malformed
  statement: When .beadwork/signals.yml exists but is malformed (YAML parse error, invalid schema definition, or duplicate type names), bw signal commands shall refuse with a CONFIG ERROR message distinct from validation and gate refusals, and shall never fall back to treating the repo as having no types defined.
  priority: must
  stability: stable
- id: bw.signal.config.undefined_type_refused
  statement: Emitting a signal type not defined in the repo config shall be refused with a validation error before any hook runs.
  priority: must
  stability: stable
- id: bw.signal.config.schema_expressiveness
  statement: The type-definition schema shall support field types string, int, bool, and enum (with a values domain), with required and required_when (conditional on another field's value) modifiers, and pure-token types with no fields.
  priority: must
  stability: stable
- id: bw.signal.config.full_payload_validation
  statement: The final post-enrich payload shall be validated against the full type schema (required fields, required_when conditions, enum domains, field types), and a payload failing validation shall never be stored.
  priority: must
  stability: stable
- id: bw.signal.config.zero_builtin_types
  statement: Beadwork core shall define no built-in signal types.
  priority: must
  stability: stable
```

## Scenarios

```yaml spec-scenarios
- id: bw.signal.config.deferral_escape_hatch_closed
  given:
    - a repo whose .beadwork/signals.yml defines types completed, verify, and blocked
  when:
    - an agent runs bw signal emit bw-x deferral
  then:
    - the emit exits non-zero with a VALIDATION error naming the undefined type
    - no hook runs and nothing is stored
  covers:
    - bw.signal.config.undefined_type_refused
    - bw.signal.config.zero_builtin_types
- id: bw.signal.config.dormant_repo
  given:
    - a repo with beadwork initialized and no .beadwork/signals.yml
  when:
    - any bw command runs, including bw signal emit bw-x completed
  then:
    - non-signal commands behave exactly as before the feature existed
    - the emit is refused with "no signal types defined"
  covers:
    - bw.signal.config.dormant_when_absent
    - bw.signal.config.working_tree_source
- id: bw.signal.config.duplicate_type_fails_closed
  given:
    - a .beadwork/signals.yml declaring the type verify twice
  when:
    - bw signal emit bw-x verify --field phase=PASS runs
  then:
    - the command exits non-zero with a CONFIG ERROR naming the duplicate
    - the error is not a validation or gate refusal
    - nothing is stored
  covers:
    - bw.signal.config.fail_closed_malformed
- id: bw.signal.config.enum_domain_enforced
  given:
    - a verify type whose phase field is enum [PASS, FAIL], required
  when:
    - bw signal emit bw-x verify --field phase=MAYBE runs
  then:
    - the emit is refused with a VALIDATION error naming the allowed values
  covers:
    - bw.signal.config.schema_expressiveness
    - bw.signal.config.full_payload_validation
- id: bw.signal.config.required_when_routing
  given:
    - an audit type where target is enum [implementer, verifier] with required_when phase=BOUNCE
  when:
    - bw signal emit bw-x audit --field phase=BOUNCE runs without a target field
  then:
    - the emit is refused for the missing conditionally-required field
  covers:
    - bw.signal.config.schema_expressiveness
```

## Verification

```yaml spec-verification
- kind: command
  target: go test ./internal/signal/ -run TestConfigWorkingTreeSource
  execute: true
  covers:
    - bw.signal.config.working_tree_source
- kind: command
  target: go test ./test/ -run TestSignalAdditivityDormantRepo
  execute: true
  covers:
    - bw.signal.config.dormant_when_absent
- kind: command
  target: go test ./internal/signal/ -run TestConfigFailClosedMalformed
  execute: true
  covers:
    - bw.signal.config.fail_closed_malformed
- kind: command
  target: go test ./cmd/bw/ -run TestSignalEmitUndefinedTypeRefused
  execute: true
  covers:
    - bw.signal.config.undefined_type_refused
    - bw.signal.config.zero_builtin_types
- kind: command
  target: go test ./internal/signal/ -run TestSchemaExpressiveness
  execute: true
  covers:
    - bw.signal.config.schema_expressiveness
- kind: command
  target: go test ./internal/signal/ -run TestValidateFinalPayload
  execute: true
  covers:
    - bw.signal.config.full_payload_validation
```
