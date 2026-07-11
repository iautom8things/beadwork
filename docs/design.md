# Design

## Storage

All data lives on a git orphan branch (`beadwork`), manipulated directly in the git object database via [go-git](https://github.com/go-git/go-git). Nothing touches your working tree or index.

Each issue is a JSON file. Structural relationships (status, labels, dependencies) are encoded as zero-byte marker files in a directory hierarchy:

```
issues/
  bw-a1b2.json
status/
  open/
    bw-a1b2          (0 bytes)
labels/
  bug/
    bw-a1b2          (0 bytes)
blocks/
  bw-a1b2/
    bw-c3d4          (0 bytes)
parent/
  bw-a1b2/
    bw-c3d4          (0 bytes)
```

Every listing query is a directory read. Parent-child relationships use the same marker pattern, with cycle detection preventing circular hierarchies. Two agents working on different issues never touch the same file.

## Attachments

Arbitrary binary or text blobs may be stored alongside an issue under the
`attachments/<ticket-id>/` tree:

```
attachments/
  bw-a1b2/
    design.png
    notes/background.md
```

The path after `<ticket-id>/` is stored verbatim — no normalization, no
basename flattening. Nested paths are allowed. This mirrors the existing
marker-tree convention (e.g. `blocks/<blocker>/<blocked>`).

Reads go through `store.GetAttachment(ticketID, path)`, which walks the
current Beadwork tree and returns the blob bytes; a sentinel
`ErrAttachmentNotFound` is returned when the path is absent. Writes go
through the internal `store.Attach(ticketID, storedPath, content)` helper,
which stages the blob and appends an `attach` intent line (see below).

## Signals

Repo-defined signals are immutable JSON records stored under
`signals/<ticket-id>/`:

```
signals/
  bw-a1b2/
    0001.json
    0002.json
```

Each record is a self-contained snapshot containing the store-assigned
sequence number, signal type, ticket id, final payload, and store-assigned
`emitted_at` timestamp. Sequence numbers are zero-padded and derived by
reading the ticket's existing signal directory immediately before staging the
record, so retrying after a ref-CAS conflict takes the next available number.

Signal type definitions are read from `.beadwork/signals.yml` in the repository
working tree, never from the beadwork branch. Repos without that file have no
defined signal types; malformed config fails closed with a `CONFIG ERROR`.
`bw signal emit` always runs the configured hook pipeline. Repo-global hooks live
under top-level `hooks`; a type's hooks live below that type. Commands may be a
single executable path or a list. Global enrich hooks run before type enrich
hooks; type gate hooks run before global gates. `on-blocked` and `post-emit` are
independently optional, as are all other moments. `hook_timeout` is a positive Go
duration and defaults to `30s`.

Each hook inherits the parent environment, runs with the repository root as its
working directory, and receives `BW_SIGNAL_TYPE`, `BW_SIGNAL_TICKET`,
`BW_SIGNAL_MOMENT`, and `BW_SIGNAL_CALLER_CWD` — the directory the emitting
command was invoked from. In a linked worktree the repository root resolves to
the primary checkout, so a gate that verifies the emitted tree (compile, test,
ancestry against `HEAD`) must `cd "$BW_SIGNAL_CALLER_CWD"` first; the hook's
own working directory stays the repository root so relative hook paths resolve
consistently. Its stdin is the JSON signal (`type`, `ticket`, and `payload`)
and is closed after the record is written. stdout and stderr are captured, never
connected to the terminal. Enrich stdout on exit 0 is either empty (no change) or
a JSON object replacing the payload. The replacement is schema-validated before
any gate or store operation.

| Exit/result | Enrich | Gate | on-blocked / post-emit |
| --- | --- | --- | --- |
| 0 | allow; JSON stdout replaces payload | allow | success |
| 1 | MALFUNCTION | BLOCKED; combined output is the reason | isolated failure / WARNING |
| 2+, signal, timeout | MALFUNCTION | MALFUNCTION | isolated failure / WARNING |

BLOCKED runs `on-blocked`; MALFUNCTION does not. Both fail closed before storage.
Hooks run once outside ref-CAS retries. After durable storage, post-emit failures
print a warning but do not turn a successful emit into a failure.

Signal configuration is introspectable without reading YAML by hand. `bw signal
types` preserves its default bare-name output for scripts; `bw signal types
--verbose` and `bw signal types --json` include fields, enum domains, required
and required_when modifiers, effective hook_timeout, and inherited plus per-type
hooks in the order Beadwork will execute them. `bw signal show <type>` renders
the same detail for one type. `bw signal validate <type> --field k=v` performs
schema-only validation and stores nothing; adding `--run-hooks` dry-runs enrich,
final validation, and gate with `BW_SIGNAL_DRY_RUN=1` in the hook environment,
still storing nothing and still suppressing on-blocked and post-emit hooks.

## Sync

Every CLI operation commits with a structured message that doubles as a replayable intent log:

```
create bw-a1b2 p1 task "Fix auth bug"
close bw-a1b2 reason="completed"
link bw-a1b2 blocks bw-c3d4
delete bw-a1b2
comment bw-a1b2 "Fixed in latest deploy"
attach bw-a1b2 design.png
signal bw-a1b2 verify signals/bw-a1b2/0001.json
```

### The `attach` intent

```
attach <ticket-id> <path-verbatim>
```

Tokens are separated by a single space. `<ticket-id>` matches the existing
ticket-id format (e.g. `bw-[a-z0-9]+`). `<path-verbatim>` is the rest of
the line up to newline — it may contain `/` and `.`, must not contain a
trailing whitespace character, and must not embed a newline.

Multiple `attach` lines may appear in a single commit message, after the
primary intent line:

```
review bw-parent: create bw-review, move parent to in_progress
attach bw-review apps/octopus/lib/foo.ex
attach bw-review apps/octopus/lib/bar.ex
```

**Replay semantics.** Given `attach <ticket-id> <path>` in a commit message
being replayed: look up the blob oid at `attachments/<ticket-id>/<path>` in
the pre-replay commit tree (git keeps objects in the object database even
after a ref reset). Re-stage the tree entry at that path with that blob
oid. If the blob is missing from the ODB, the replay fails loudly with an
error — attachments are never silently dropped.

### The `signal` intent

```
signal <ticket-id> <type> <path>
```

`<path>` is the stored record path, for example
`signals/bw-a1b2/0001.json`. Replay mirrors attachment recovery: the original
JSON blob is read from the current tree or the pre-reset source commit and
re-staged byte-for-byte. Replay does not read `.beadwork/signals.yml`, re-run
validation, or execute hooks. If the blob cannot be recovered, replay fails
loudly instead of silently dropping the signal.

### Signal query cursors

`bw signal query --ticket <id> --since <commit> --json` walks beadwork commits
after the supplied commit hash and returns matching immutable snapshots oldest
first. Its response cursor is the current beadwork branch head. Callers persist
and supply that cursor; beadwork core stores no poll state. Local compare-and-swap
ref updates provide exactly-once ordering on one host, including across unrelated
commits. Sync may rewrite hashes, so cross-machine cursor correctness is not
promised.

`bw sync` fetches, rebases, and pushes. If rebase conflicts, it replays intents from commit messages against the current remote state. No merge drivers, no lock files, no custom conflict resolution.
