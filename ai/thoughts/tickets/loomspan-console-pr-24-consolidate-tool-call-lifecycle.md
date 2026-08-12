# PR 24 - Consolidate Duplicate Tool-Call Lifecycle Records

## Status

Implementation-ready ticket. Codebase and trace-contract impact verified on
2026-08-11 against the Loomspan repository and the representative trace
`1810c589-7b7d-4df7-a242-f8bfa6a24033.ndjson`. No implementation has started.

This is intentionally separate from the Trace Explorer record-inspector work.
It changes the Java trace writer, live activity contract, usage projection,
execution journal, Go analysis vocabulary, browser contracts, tests, fixtures,
and observability documentation in lockstep.

## Outcome

One canonical `TOOL_CALL_STARTED` record represents the point immediately
before Loomspan invokes a capability. It carries the tool identity, planned or
unplanned task relationship, input arguments, optional note, and event ID.

`TOOL_CALL_REQUESTED` is removed. A single tool invocation no longer emits two
adjacent records containing the same event and arguments without an observable
lifecycle transition between them.

The console presents the retained record as **Tool input** and keeps arguments
behind an explicit user action. `TOOL_CALL_COMPLETED` and `TOOL_CALL_FAILED`
remain the authoritative terminal records for the invocation.

## Triggering evidence and verified current behavior

`DefaultExecutionStateService.logToolCall` and `logUnplannedToolCall` currently
call `recordToolRequested` and `recordToolStarted` consecutively with the same
`TaskExecutionEvent`. No validation, queueing, authorization, dispatch, or
capability boundary exists between those calls.

The representative trace confirms that each pair has:

- the same tool frame;
- the same event ID;
- the same capability name and linked task ID;
- the same arguments and note; and
- timestamps generally separated by only one to four milliseconds.

The records nevertheless have different downstream consumers:

- `TOOL_CALL_REQUESTED` supplies execution-journal tool-call entries;
- `TOOL_CALL_STARTED` supplies live activity and increments tool-invocation
  usage;
- Go and TypeScript enumerate both record kinds; and
- Java tests explicitly assert that both records are emitted.

This makes removal a bounded but cross-component contract correction rather
than a browser-only cleanup.

## Resolved design

### 1. Retain `TOOL_CALL_STARTED`

Keep `TOOL_CALL_STARTED` because it already represents the actual pre-invocation
boundary used by live activity and usage accounting. Emit it once, immediately
before `CapabilityExecutionRouter.execute` is called and after Loomspan has:

- resolved planned versus unplanned execution;
- established the linked task when one exists;
- opened the `TOOL_INVOCATION` frame; and
- completed all framework checks required before entering the capability.

Do not emit it if the capability will not be invoked. The record does not claim
that the capability completed, produced output, or succeeded.

Preserve its current payload facts:

- `eventId`;
- `capabilityName`;
- optional `linkedTaskId`;
- `details.arguments`;
- optional `note`; and
- `metadata.unplanned` when no task is linked.

### 2. Remove `TOOL_CALL_REQUESTED` completely

Remove the enum value, writer method, calls, readers, tests, fixtures, labels,
and documentation for `TOOL_CALL_REQUESTED`. Do not retain an alias, emit both
forms, or teach readers to translate the removed record.

There is currently no authoritative “request accepted but not started” state.
Keeping the name for a hypothetical future queue or dispatch boundary would
preserve an unsupported concept. If Loomspan later gains a real asynchronous
request lifecycle, design a new record from that evidence rather than reserving
this one.

### 3. Move journal projection to the retained record

Change `ExecutionJournalProjector` so `TOOL_CALL_STARTED` produces the existing
tool-call journal entry. Preserve its current sanitized summary, planned or
unplanned classification, capability name, linked task, arguments, and note.

The journal must produce exactly one entry per invocation. It must not infer
requests from adjacency or synthesize a second lifecycle state.

### 4. Preserve usage and live semantics

Continue incrementing tool-invocation usage from `TOOL_CALL_STARTED`. Continue
projecting one bounded live “Tool call started” activity without arguments.

No counter, quota, metric, or live-activity meaning changes in this ticket.
The number of canonical trace records decreases, but the number of tool
invocations does not.

### 5. Present tool input from `TOOL_CALL_STARTED`

Add a finalized-trace **Tool input** inspector for `TOOL_CALL_STARTED` showing:

- tool/capability name;
- resolved task title and task ID when mechanically available;
- planned or unplanned status;
- pretty-formatted arguments;
- optional note; and
- event ID as secondary diagnostic information.

An unplanned call must be visibly identified and must state that no plan task
was linked. Do not characterize it as invalid or failed unless another record
does so.

Arguments remain collapsed until explicitly requested. Render them as inert
text/JSON and rely on the writer's existing trace-safe input handling and the
console's authenticated local diagnostic boundary. This ticket does not add a
new redaction policy or display arguments in live activity.

Where mechanically correlated by the same tool frame, the inspector may link
to `TOOL_CALL_COMPLETED` or `TOOL_CALL_FAILED`. Missing terminal evidence stays
unknown; do not infer outcome from frame closure or record proximity.

## Framework feature-design lens assessment

### Developer problem

Developers currently see two records that appear to describe different phases
but contain identical evidence and are emitted from one uninterrupted method.
This increases trace noise and makes the lifecycle harder to explain.

### Framework responsibility

The runtime owns invocation boundaries and is the only authority able to say
when execution begins. Consolidation improves framework diagnostics regardless
of model capability and removes an inaccurate distinction rather than hiding
behavior.

### Dataflow, trust, and safeguards

No new business input, trusted execution metadata, mutable state, or public
framework concept is introduced. Existing tool arguments and task linkage move
unchanged to the one retained diagnostic fact. Ordering, planned/unplanned
identity, failure visibility, and full result ownership remain explicit.

Sensitive arguments remain deliberate finalized-trace detail and are not
promoted into live summaries, logs, metrics, list rows, or derived indexes.

### Evidence proportionality and alternatives

The duplication is systematic across every tool invocation, not isolated to
one sample. Keeping both records with only one rich console inspector would
reduce UI duplication but leave the misleading trace lifecycle and ongoing
cross-component cost. Moving `TOOL_CALL_STARTED` to a later point is not useful:
the current seam immediately before `execute` is already the real start
boundary.

The smallest coherent correction is therefore one retained start record and
one atomic downstream migration.

## Contract and compatibility

| Surface | Classification and evidence | Treatment |
| --- | --- | --- |
| `TraceRecordType` and NDJSON record vocabulary | Ephemeral diagnostic format consumed by current Java, Go, browser, fixtures, and debugging tools. | Remove `TOOL_CALL_REQUESTED` atomically. |
| Execution journal projection | Internal diagnostic projection with in-repository consumers and tests. | Project the existing tool-call entry from `TOOL_CALL_STARTED`. |
| Live activity kind | Current-run observability contract used by Java SSE, Go, and browser. | Preserve `TOOL_CALL_STARTED` and its meaning unchanged. |
| Tool usage accounting | Runtime/observability behavior used by summaries and quotas. | Continue counting from `TOOL_CALL_STARTED`; no semantic change. |
| Browser record inspector | Console diagnostic experience. | Add Tool input to the retained record only. |
| Application API, Supported SPI, configuration, and manifests | No deliberate surface is affected. | No public-surface delta. |

### Breaking-change decision

This intentionally changes the current trace record sequence and removes one
ephemeral record kind. Loomspan's pre-1.0 policy permits an atomic correction
that improves current-run diagnostic coherence. Update all in-repository
writers, consumers, fixtures, tests, and documentation together.

### Shim decision

No shim. Do not keep the old enum, dual-write, translate old traces, or accept a
legacy record solely for historical readability. Previously retained raw trace
files are not a protected cross-version interchange format.

### Compatibility-marker decision

Do not introduce or independently increment a trace-schema marker.
`consoleCompatibilityVersion` remains the exact Java/Go release version and the
coordinated components ship under the same release. The semantic change is
therefore represented by the containing product version, not a second marker.

## Implementation map

### Java writer and projections

- Remove `TOOL_CALL_REQUESTED` from `TraceRecordType`.
- Remove `recordToolRequested` from `ExecutionTraceRecorder` and
  `DefaultExecutionTraceRecorder`.
- Update `DefaultExecutionStateService.logToolCall` and
  `logUnplannedToolCall` to emit only `recordToolStarted`.
- Verify the emission remains immediately before capability execution.
- Move `ExecutionJournalProjector` tool-call projection from requested to
  started.
- Preserve `LiveActivityProjector` usage and activity behavior.
- Remove or rewrite Java tests that require both records.

### Go and browser

- Remove `RecordToolCallRequested` from Go trace-analysis enums and any switch
  handling or fixture expectations.
- Keep the `TOOL_CALL_STARTED` live DTO/activity kind unchanged.
- Remove `TOOL_CALL_REQUESTED` from TypeScript trace-record contracts and
  labels.
- Add the Tool input inspector to `TOOL_CALL_STARTED` and reuse the existing
  strict, paginated raw-record reader and plan-task correlation helpers.
- Update activity, Trace Explorer, accessibility, and contract tests.

### Fixtures and documentation

- Regenerate Java-owned trace fixtures and inspect the sequence/count changes.
- Update phase documents and any trace examples that enumerate visible or
  canonical record kinds.
- Ensure debugging guidance describes one start fact followed by completion or
  failure.

## Required tests

### Java

- A planned invocation emits exactly one `TOOL_CALL_STARTED` with arguments,
  event ID, capability, and linked task.
- An unplanned invocation emits exactly one start record with `unplanned: true`,
  no linked task, and its recorded note.
- No `TOOL_CALL_REQUESTED` record exists in runtime-produced traces.
- The start record precedes capability side effects.
- A capability failure still retains its start record and one
  `TOOL_CALL_FAILED` record.
- A success retains one start and one completed record.
- Usage increments exactly once per attempted invocation.
- The execution journal contains exactly one sanitized tool-call entry.
- Live activity contains exactly one tool-start activity without arguments.

### Java/Go contract

- Regenerated success, failure, planned, and unplanned traces are accepted.
- Tool-invocation counts remain unchanged even though physical record counts
  decrease.
- Frame hierarchy, task linkage, and completion/failure correlation remain
  unchanged.
- No reader requires adjacency to identify an invocation or outcome.

### Browser

- Tool input shows pretty-formatted JSON arguments from `TOOL_CALL_STARTED`.
- Plain text/scalar argument values remain readable and inert.
- Planned calls show the mechanically resolved task title and ID.
- Unplanned calls are visibly labeled without being called failures.
- Arguments are not rendered until the developer selects Tool input.
- Live activity still shows a concise start row without input content.
- Keyboard, focus, and screen-reader labels cover the new inspector.

## Acceptance signals

- Every tool invocation emits one start record rather than an adjacent
  requested/started duplicate pair.
- Tool usage, live monitoring, execution journal entries, planned/unplanned
  linkage, and completion/failure evidence remain correct.
- The console exposes tool arguments once, from the retained authoritative
  start record.
- Java, Go, TypeScript, regenerated fixtures, and documentation contain no
  `TOOL_CALL_REQUESTED` vocabulary.
- No public API/SPI, configuration, manifest, legacy reader, or new compatibility
  marker is introduced.

## Guardrails

- Do not merge start and completion into one record.
- Do not move tool output onto the start record.
- Do not infer success from a start record or frame closure.
- Do not count both frame opening and start as separate tool invocations.
- Do not expose arguments in live activity, metrics, list rows, or logs.
- Do not add a legacy alias, dual writer, migration reader, or historical
  fixture suite.
- Preserve unrelated user-authored changes in the working tree.

## Verification sequence

```powershell
.\mvnw.cmd -pl loomspan-spring-boot-starter test -DfailIfNoTests=false
.\mvnw.cmd -pl loomspan-spring-boot-starter test -Dtest=ConsoleTraceFixtureCorpusTest -Dloomspan.console.fixtures.regenerate=true -DfailIfNoTests=false
Set-Location loomspan-console
go test ./...
go run ./internal/buildtool verify
$env:PATH = "C:\msys64\mingw64\bin;" + $env:PATH
$env:CGO_ENABLED = "1"
go test -race ./...
```

Inspect regenerated artifacts rather than accepting broad sequence churn. The
expected structural difference is removal of one requested record per tool
invocation, plus downstream sequence and record-count changes.

## Out of scope

- Adding asynchronous tool queues, dispatch acknowledgements, or a future true
  request-versus-start lifecycle.
- Changing tool execution, retry, authorization, timeout, or cancellation
  semantics.
- Redesigning the tool invocation frame.
- Displaying inputs in live activity.
- New sensitive-data classification or redaction policy.
- Changing `TOOL_CALL_COMPLETED`, `TOOL_CALL_FAILED`, or `STEP_COMPLETED`
  ownership beyond required correlation updates.
