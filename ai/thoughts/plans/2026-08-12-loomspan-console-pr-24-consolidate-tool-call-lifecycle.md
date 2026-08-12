# PR 24 Consolidate Tool-Call Lifecycle Implementation Plan

## Overview

Replace the adjacent `TOOL_CALL_REQUESTED` / `TOOL_CALL_STARTED` pair with one canonical `TOOL_CALL_STARTED` fact emitted immediately before capability execution. Move every current-release Java, Go, journal, fixture, browser, test, and documentation consumer to that retained fact while preserving tool usage, live activity, terminal result/failure records, planned/unplanned identity, and the existing diagnostic-content trust boundary.

The change is an atomic correction to Loomspan's current-run diagnostic format. It does not introduce compatibility aliases, historical readers, a second schema marker, or a new execution state.

## Current State Analysis

- `DefaultExecutionStateService#logToolCall` and `#logUnplannedToolCall` append `TOOL_CALL_REQUESTED` and `TOOL_CALL_STARTED` consecutively with the same `TaskExecutionEvent` and `ToolTraceContext` (`loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/runtime/state/DefaultExecutionStateService.java:286-304`).
- `DefaultToolCallbackFactory#invokeCapability` resolves plan linkage, opens the `TOOL_INVOCATION` frame, logs the call, and then immediately calls `CapabilityExecutionRouter#execute` (`loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/runtime/tool/DefaultToolCallbackFactory.java:109-140`). There is no request-acceptance, queue, authorization, or dispatch transition between the two current records.
- `ExecutionJournalProjector` creates the developer-facing tool-call entry from `TOOL_CALL_REQUESTED`, including its existing recursive field-name sanitization (`loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/runtime/trace/ExecutionJournalProjector.java:55-129`).
- `LiveActivityProjector` exposes only `TOOL_CALL_STARTED`, increments derived tool-invocation usage only from that record, and excludes logical payload data from bounded live activity (`loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/runtime/observation/LiveActivityProjector.java:22-99`).
- Go mirrors and strictly accepts the obsolete record value through `RecordToolCallRequested` and `knownRecordType` (`loomspan-console/internal/traceanalysis/enums.go:3-61`).
- The browser `ActivityKind` already contains only `TOOL_CALL_STARTED`, `TOOL_CALL_COMPLETED`, and `TOOL_CALL_FAILED`; there is no TypeScript `TOOL_CALL_REQUESTED` contract or label to remove (`loomspan-console/web/src/api/contracts.ts:282-320`).
- `TraceRecords` already provides explicit, paginated, UTF-8-strict raw-record retrieval, inert `<pre>` rendering, plan history lookup, task-title correlation, and record-specific detail actions (`loomspan-console/web/src/observability/TraceRecords.tsx:113-170`, `359-489`, `839-1070`). It has a Tool result inspector but no Tool input inspector.
- Java tests currently construct or assert `TOOL_CALL_REQUESTED` in `ExecutionStateServiceTest`, `ExecutionJournalProjectorTest`, `ExecutionJournalProjectionContractTest`, `LoomspanSessionTest`, and `LoomspanSessionRunnerTest`.
- The committed cross-language corpus contains sixteen valid traces but no tool lifecycle record. Merely regenerating it would produce no tool-related change. New Java-owned planned-success and unplanned-failure fixtures are required to exercise the current-release Java-to-Go boundary.
- `ai/skill-authoring/traces-and-debugging.md` is the routed, source-verified author guidance for traces, but it does not yet state the authoritative tool start/terminal lifecycle or how Tool input is deliberately revealed.

## Desired End State

For every capability attempt that crosses the invocation boundary:

1. Loomspan resolves planned or unplanned linkage and opens the `TOOL_INVOCATION` frame.
2. Loomspan appends exactly one `TOOL_CALL_STARTED` containing the event ID, capability name, optional linked task, arguments, optional note, and `metadata.unplanned: true` for an unplanned call.
3. The start record is visible before `CapabilityExecutionRouter#execute` can produce a side effect or failure.
4. Successful execution appends one `TOOL_CALL_COMPLETED`; failed execution retains the start and appends one `TOOL_CALL_FAILED` plus the existing canonical error facts.
5. The execution journal projects exactly one sanitized tool-call entry from the start record.
6. Live activity emits exactly one bounded "Tool call started" activity without arguments, and tool-invocation usage remains one per attempted invocation.
7. Go accepts the retained current-release record set and no longer enumerates the removed record.
8. The finalized Trace Explorer offers an explicit **Tool input** action on `TOOL_CALL_STARTED`; arguments are not retrieved or rendered until that action is selected.
9. Current Java, Go, browser, fixtures, tests, phase documentation, and skill-authoring guidance describe one start fact followed by success, failure, or unknown terminal evidence.

### Key Discoveries

- The retained record is already at the correct pre-router boundary; no runtime method needs to move (`DefaultToolCallbackFactory.java:126-140`).
- Usage has two intentional views: the session quota service counts the attempted call before plan resolution, while live projection derives one tool invocation from `TOOL_CALL_STARTED`. Removing only the duplicate requested trace does not require changing either accounting boundary.
- Planned records omit `metadata.unplanned`; unplanned records set it to `true` and omit `linkedTaskId` (`ToolTraceContext.java:20-35`). Browser parsing must preserve that canonical distinction rather than treating absence as failure.
- The `TaskExecutionEvent` payload already owns the facts required by Tool input: `eventId`, `capabilityName`, `linkedTaskId`, `details`, and `note` (`TaskExecutionEvent.java:9-27`).
- Existing plan correlation can resolve a title when a task ID and owning skill are mechanically available. If that correlation is unavailable, the inspector must show the recorded task ID without inventing a title.
- The Console repository instructions explicitly require Java, Go, TypeScript, fixtures, and semantic tests to move together and forbid aliases or legacy readers. Exact `consoleCompatibilityVersion` matching is the release-level compatibility boundary.

## What We're NOT Doing

- Adding a queue, request acknowledgement, dispatch state, retry state, authorization state, cancellation state, or future request-versus-start lifecycle.
- Moving tool results or failures onto the start record, or merging start and terminal records.
- Inferring success, failure, or cancellation from a start record, frame closure, adjacency, or missing terminal evidence.
- Exposing tool arguments in live activity, metrics, list-row summaries, logs, or automatically opened detail.
- Introducing redaction, secret scanning, content classification, disclosure tiers, or a new trust boundary. Finalized trace input remains authenticated local diagnostic content rendered inertly.
- Adding a legacy enum value, alias, dual writer, compatibility adapter, schema migration, old-trace reader, or historical fixture suite.
- Changing public Application API, Supported SPI, configuration, manifests, tool execution semantics, quotas, metrics, or the product compatibility version.
- Implementing optional start-to-terminal navigation in this PR. Terminal records remain independently inspectable, and absence of a terminal remains unknown.

## Skill-Authoring Documentation Impact

**Impact**: Affected

- **Rationale**: Skill authors diagnosing tool behavior need to know that one start record is the authoritative pre-invocation fact, that planned and unplanned linkage have distinct recorded semantics, that Tool input is deliberately retrieved only from finalized traces, and that a start alone does not establish an outcome. The inspector also makes the existing sensitivity boundary operationally relevant to tool arguments.
- **Documents to update**:
  - `ai/skill-authoring/traces-and-debugging.md`
  - `ai/skill-authoring/README.md`
- **Supporting evidence**: focused Java lifecycle, journal, live-activity, and ordering tests; Java-owned planned/unplanned tool fixtures consumed by Go; browser Tool input tests; `DefaultToolCallbackFactory#invokeCapability`; `DefaultExecutionStateService#logToolCall` / `#logUnplannedToolCall`; and `TraceRecords` detail retrieval tests.
- **Coverage table update**: Required. Extend the existing "Traces and debugging" coverage note to include tool start/terminal interpretation and deliberate input inspection; its coverage status remains source-verified.
- **LLM-first usability**: Add a compact tool-lifecycle subsection and debugging steps using exact record names and normative distinctions. Keep sensitivity guidance in the existing section, link to implementation/test anchors, and avoid duplicating the browser or phase specification.

## Contract and Compatibility Impact

| Surface | Classification and supporting evidence | Planned compatibility treatment |
| --- | --- | --- |
| Application API | No impact. No ordinary application entry point, return type, or documented application-facing behavior changes. | Preserve unchanged. |
| Supported SPI | No impact. The affected recorder/state/router types live under internal packages, and the architecture allowlist describes cross-package internal collaboration rather than supported replacement points. | Preserve supported SPI unchanged; add no extension point. |
| Configuration and manifest contracts | No impact. No `loomspan.*` property, YAML field, validation rule, default, or manifest behavior changes. | Preserve unchanged. |
| Persisted or serialized contracts | No deliberately durable contract is affected. Retained raw trace files are explicitly not protected cross-version interchange. | No migration or historical reader. |
| Ephemeral diagnostic formats | Affected. Java writes, Go consumes, fixtures encode, journal/live project, and the browser inspects the current-release NDJSON vocabulary. | Intentionally remove `TOOL_CALL_REQUESTED`; keep writer, reader, projectors, fixtures, security boundary, ordering, and terminal visibility coherent in one release. |
| Internal or accidentally exposed implementation | Affected. Remove the public-but-internal enum member and `ExecutionTraceRecorder#recordToolRequested` method plus every in-repository construction/assertion. | Atomic removal with no overload, alias, fallback, adapter, bridge, or dual behavior. |

- **Evidence of supported contracts**: The framework design lens, Console `AGENTS.md`, exact release-string compatibility check, Java-owned semantic fixture corpus, and routed trace/debugging guidance establish current-release coherence and diagnostic security as protected goals. They do not establish historical trace compatibility.
- **Intended breaks**: Current raw traces contain one fewer record per tool invocation, and a current reader no longer accepts `TOOL_CALL_REQUESTED`. This is the approved pre-1.0 correction.
- **In-repository consumers to update**: Java enum, recorder interface/implementation, state service, journal and live projectors, Java tests with manually constructed traces, Go enum acceptance, Java/Go fixture corpus, browser record details/tests, phase documentation, fixture documentation, and skill-authoring guidance.
- **Public-surface delta**: Remove `TraceRecordType.TOOL_CALL_REQUESTED` and `ExecutionTraceRecorder#recordToolRequested` from technically public internal packages. Add no public type, signature, constructor, Spring bean, or `@ConditionalOnMissingBean` extension point.
- **Shim decision**: **No shim.** There is no protected request-accepted state or cross-version trace contract, and exact Java/Go release matching makes mixed behavior unsupported.
- **Java-to-Go boundary coordination**: **Required.** Java writer vocabulary, Go `knownRecordType`, Java-generated NDJSON fixtures, Go fixture parsing, expected semantic artifacts, tests, and documentation must ship together. `consoleCompatibilityVersion` remains the exact product release; no independent marker changes.

## Implementation Approach

Remove the obsolete concept at its source, then update projections and focused Java tests before synchronizing the strict Go vocabulary and adding missing cross-language lifecycle fixtures. Build the browser inspector on the retained record using the existing explicit raw-reader and plan-correlation machinery. Finish by updating routed guidance and running the complete Java/Go/browser verification sequence twice where generated artifacts are involved.

Each phase is independently verifiable during development, but the complete set is one atomic PR and must not be shipped partially.

## Phase 1: Canonicalize Java Tool Invocation Records

### Overview

Make `TOOL_CALL_STARTED` the single authoritative pre-invocation record, migrate Java projections, and protect ordering, payload, journal, live, usage, success, and failure semantics with focused tests.

### Changes Required

#### 1. Remove the obsolete Java record and recorder method

**Files**:

- `loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/core/TraceRecordType.java`
- `loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/core/ExecutionTraceRecorder.java`
- `loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/core/DefaultExecutionTraceRecorder.java`

**Changes**:

- Delete `TOOL_CALL_REQUESTED`.
- Delete `recordToolRequested` from the internal recorder contract and its default implementation.
- Keep `recordToolStarted`, `recordToolCompleted`, and `recordToolFailed` payload and metadata ownership unchanged.
- Update exhaustive enum switches caused by the removal; do not add a replacement enum or alias.

#### 2. Emit one start at the existing pre-router seam

**Files**:

- `loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/runtime/state/DefaultExecutionStateService.java`
- `loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/runtime/tool/DefaultToolCallbackFactory.java` (ordering remains structurally unchanged; test-visible edits only if needed)

**Changes**:

- Change both `logToolCall` and `logUnplannedToolCall` to call only `recordToolStarted` with the existing `TaskExecutionEvent`.
- Preserve resolution of `linkedTaskId`, frame opening, unplanned note creation, session quota recording, router invocation, completion/failure handling, and frame closure.
- Do not emit a start when execution is rejected before the callback reaches the existing logging seam.

The protected sequence is:

```text
resolve plan linkage -> open TOOL_INVOCATION frame -> append TOOL_CALL_STARTED -> router.execute
```

#### 3. Move the journal projection and preserve live semantics

**Files**:

- `loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/runtime/trace/ExecutionJournalProjector.java`
- `loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/runtime/observation/LiveActivityProjector.java`

**Changes**:

- Project the existing `TOOL_CALL` / `UNPLANNED_TOOL_EXECUTION` journal entry from `TOOL_CALL_STARTED`.
- Preserve capability, linked task, arguments, note, and recursive sensitive-field sanitization in the journal payload.
- Remove the obsolete enum from `LiveActivityProjector#phase`; retain `TOOL_CALL_STARTED` visibility, summary, bounded detail allowlist, payload exclusion, and one derived usage increment.
- Do not deduplicate records by adjacency or event ID; there will be one canonical start record by construction.

#### 4. Rewrite and extend focused Java tests

**Files**:

- `loomspan-spring-boot-starter/src/test/java/com/lokiscale/loomspan/internal/runtime/state/ExecutionStateServiceTest.java`
- `loomspan-spring-boot-starter/src/test/java/com/lokiscale/loomspan/internal/runtime/tool/ToolCallbackFactoryTest.java`
- `loomspan-spring-boot-starter/src/test/java/com/lokiscale/loomspan/internal/runtime/tool/ToolCallLifecycleTraceTest.java` (new focused integration test if keeping the full trace assertions separate is clearer)
- `loomspan-spring-boot-starter/src/test/java/com/lokiscale/loomspan/internal/runtime/trace/ExecutionJournalProjectorTest.java`
- `loomspan-spring-boot-starter/src/test/java/com/lokiscale/loomspan/internal/runtime/trace/ExecutionJournalProjectionContractTest.java`
- `loomspan-spring-boot-starter/src/test/java/com/lokiscale/loomspan/internal/runtime/observation/LiveActivityProjectorTest.java`
- `loomspan-spring-boot-starter/src/test/java/com/lokiscale/loomspan/internal/core/LoomspanSessionTest.java`
- `loomspan-spring-boot-starter/src/test/java/com/lokiscale/loomspan/internal/core/LoomspanSessionRunnerTest.java`

**Changes**:

- Replace manually constructed requested records with started records where they represent an invocation.
- Assert a planned invocation produces exactly one start with capability, event ID, linked task, arguments, and no unplanned flag.
- Assert an unplanned invocation produces exactly one start with `unplanned: true`, no linked task, arguments, event ID, and the recorded note.
- Use ordered verification across the state service and router, or inspect the session inside the router answer, to prove the start exists before capability side effects.
- Cover one success (`STARTED` then `COMPLETED`) and one thrown capability (`STARTED` retained then `FAILED`) on the same tool frame.
- Verify session tool-call accounting and metrics record one attempt/outcome, while live projection derives one tool invocation and exposes no arguments.
- Verify the journal emits one sanitized call entry per start and preserves planned/unplanned classification without legacy message inference.
- Remove obsolete recorder stubs and assertions instead of preserving both behaviors.

### Success Criteria

#### Automated Verification

- [x] Java production and test sources compile with the enum member and recorder method removed.
- [x] Planned and unplanned focused tests assert exactly one canonical start and the complete retained payload facts.
- [x] Ordering tests prove `TOOL_CALL_STARTED` precedes `CapabilityExecutionRouter#execute`.
- [x] Success and failure tests retain exactly one appropriate terminal tool record.
- [x] Journal tests produce one sanitized tool-call entry from the start record.
- [x] Live projection tests produce one bounded start activity without arguments and increment tool usage once.
- [x] Starter module tests pass: `\.\mvnw.cmd -pl loomspan-spring-boot-starter test -DfailIfNoTests=false`.

#### Manual Verification

- [x] Review a focused planned and unplanned trace sequence and confirm there is no unsupported lifecycle transition between frame open and start.
- [x] Confirm failure evidence does not imply that frame closure itself is a terminal result.

## Phase 2: Synchronize Go and the Java-Owned Trace Corpus

### Overview

Update the strict current-release Go vocabulary and add executable cross-language tool lifecycle evidence that the existing corpus currently lacks.

### Changes Required

#### 1. Remove Go acceptance of the obsolete record

**Files**:

- `loomspan-console/internal/traceanalysis/enums.go`
- Relevant existing Go enum/parser tests under `loomspan-console/internal/traceanalysis/` if compilation or exhaustive known-value coverage requires an update

**Changes**:

- Delete `RecordToolCallRequested` and remove it from `knownRecordType`.
- Keep `RecordToolCallStarted`, completed, and failed unchanged.
- Rely on the existing generic unsupported-enum test for strict rejection; do not retain the removed vocabulary in a legacy-specific test or translation path.

#### 2. Add planned-success and unplanned-failure fixtures

**Files**:

- `loomspan-spring-boot-starter/src/test/java/com/lokiscale/loomspan/internal/runtime/trace/ConsoleTraceFixtureCorpusTest.java`
- `loomspan-console-fixtures/traces/planned-tool-success.ndjson` (generated)
- `loomspan-console-fixtures/expected/planned-tool-success.json` (generated)
- `loomspan-console-fixtures/traces/unplanned-tool-failure.ndjson` (generated)
- `loomspan-console-fixtures/expected/unplanned-tool-failure.json` (generated)
- `loomspan-console/internal/traceanalysis/fixture_corpus_test.go` only if explicit inventory or semantic comparison support must be extended
- `loomspan-console-fixtures/README.md`

**Changes**:

- Extend the Java corpus generator's valid inventory with two deterministic tool cases.
- Generate records through the Java state/recorder boundary, including tool frames, start payloads, terminal records, completion outcome, failure linkage, and a terminal usage snapshot with one tool invocation.
- Planned success must contain one start and one completion with stable capability/task/frame facts and arguments.
- Unplanned failure must contain one start and one failure with `unplanned: true`, no linked task, the unplanned note, preserved failure visibility, and a failed completion linked to the canonical error.
- Add focused corpus assertions for lifecycle cardinality, ordering, frame identity, linkage, and terminal usage without adding UI-specific expected models.
- Regenerate all committed artifacts, inspect the diff, and run regeneration a second time to prove determinism.
- Update fixture counts and the representative matrix in the README to name the new tool lifecycle coverage.

### Success Criteria

#### Automated Verification

- [x] Go's current-release enum set compiles without the removed value.
- [x] Both new fixtures are generated by Java and accepted by Go.
- [x] Planned success and unplanned failure each contain one start and one correct terminal tool record.
- [x] Frame hierarchy, task linkage, failure linkage, and terminal usage remain coherent.
- [x] Java fixture verification passes before regeneration: `\.\mvnw.cmd -pl loomspan-spring-boot-starter test -Dtest=ConsoleTraceFixtureCorpusTest -DfailIfNoTests=false`.
- [x] Intentional regeneration passes: `\.\mvnw.cmd -pl loomspan-spring-boot-starter test -Dtest=ConsoleTraceFixtureCorpusTest -Dloomspan.console.fixtures.regenerate=true -DfailIfNoTests=false`.
- [x] A second regeneration produces no fixture diff.
- [x] Go tests pass from `loomspan-console`: `go test ./...`.

#### Manual Verification

- [x] Inspect the generated NDJSON rather than accepting broad sequence/count churn; changes are limited to the new cases and expected current-release vocabulary updates.
- [x] Confirm expected artifacts remain semantic analysis outputs and do not introduce a second trace or UI model.

## Phase 3: Add Explicit Tool Input Inspection and Update Guidance

### Overview

Expose the retained start payload as deliberate finalized-trace detail, preserve live-content minimization, and document the lifecycle for skill authors and Console maintainers.

### Changes Required

#### 1. Add the Tool input record detail

**Files**:

- `loomspan-console/web/src/observability/TraceRecords.tsx`
- `loomspan-console/web/src/observability/TraceRecords.toolInput.test.tsx` (new focused component test)
- Existing shared styles only if the current `trace-step-expanded`, fact-list, note, and `<pre>` styles are insufficient

**Changes**:

- Add a `tool-input` detail shape to the existing record-detail union and cache path.
- Parse `TOOL_CALL_STARTED` only after the developer selects **Tool input**, using `readCompleteRecord` and its strict bounded continuation handling.
- Validate and display:
  - capability/tool name;
  - planned or unplanned status;
  - task ID and mechanically resolved task title when available;
  - arguments using `prettyValue`, supporting JSON objects, arrays, scalars, JSON-encoded strings, and plain text;
  - optional note; and
  - event ID as secondary diagnostic identity.
- Treat `metadata.unplanned: true` with no linked task as valid unplanned execution and explain that no plan task was linked. Do not label it invalid or failed.
- For planned calls, use the recorded task ID and existing plan-history/task-title helpers when the owning skill can be correlated. Show the ID without inventing a title when it cannot.
- Render arguments as inert text in `<pre>`. Do not use HTML/Markdown interpretation or copy arguments into row text, automatic detail, live activity, or browser state.
- Do not add optional terminal correlation in this PR; a missing terminal remains unknown.

#### 2. Protect browser and live behavior

**Files**:

- `loomspan-console/web/src/observability/TraceRecords.toolInput.test.tsx`
- `loomspan-console/web/src/activity/activityPresentation.test.ts`
- Java `LiveActivityProjectorTest` from Phase 1

**Changes**:

- Assert raw input is not requested or rendered before the Tool input button is activated.
- Cover paginated raw retrieval, pretty JSON, plain/scalar input, note/event ID, planned title/ID, and unplanned messaging.
- Assert malicious-looking argument strings remain text and do not create HTML.
- Assert the action and expanded region have correct `aria-expanded`, `aria-controls`, region labels, keyboard activation, loading status, and error alert semantics.
- Preserve the concise live start row and verify arguments are absent from live presentation.

#### 3. Update phase, fixture, and skill-authoring documentation

**Files**:

- `ai/thoughts/phases/loomspan_console_phase_2_ui_console.md`
- `ai/skill-authoring/traces-and-debugging.md`
- `ai/skill-authoring/README.md`
- `loomspan-console-fixtures/README.md` (fixture-specific edits may be completed in Phase 2)

**Changes**:

- Document Tool input as explicit finalized-trace detail on `TOOL_CALL_STARTED` while retaining the existing live row without input.
- Add a compact author-facing lifecycle: start immediately before invocation, planned/unplanned linkage, completed/failed terminal ownership, and unknown outcome when terminal evidence is missing.
- State that tool arguments are authenticated diagnostic content, deliberately loaded, inertly rendered, and not newly redacted by this change.
- Add the Java lifecycle, fixture corpus, and browser inspector tests as stable implementation/test anchors.
- Update the README coverage note without changing routing or claiming historical trace compatibility.
- Leave Phase 1's existing live visibility and security language unchanged where it already accurately describes the retained start record and tool-input trust boundary.

### Success Criteria

#### Automated Verification

- [x] `TOOL_CALL_STARTED` rows expose an explicit, initially collapsed Tool input action.
- [x] Browser tests cover JSON, scalar/plain text, planned, unplanned, loading, failure, and accessibility behavior.
- [x] Arguments remain absent from live activity and are not fetched before explicit selection.
- [x] Tool input is rendered inertly and no new redaction/display policy is introduced.
- [x] Browser and Go verification passes: `go run ./internal/buildtool verify` from `loomspan-console`.
- [x] Skill-authoring claims are supported by the cited focused tests, fixtures, and production anchors.
- [x] `ai/skill-authoring/README.md` coverage remains correctly routed and source-verified.

#### Manual Verification

- [ ] In Trace Explorer, open planned and unplanned start records using keyboard-only navigation and confirm labels, focus, and expanded content are understandable.
- [x] Confirm the record row and live activity never reveal arguments before the deliberate action.
- [x] Confirm a start with no terminal evidence is not described as successful or failed.

## Phase 4: Integrated Verification and Removal Audit

### Overview

Verify the complete atomic change, inspect generated artifacts and public surfaces, and prove that no obsolete production path or compatibility machinery remains.

### Changes Required

No new feature work is expected. Resolve only defects or plan mismatches found by the full verification and diff audit.

### Success Criteria

#### Automated Verification

- [x] Full starter tests pass: `\.\mvnw.cmd -pl loomspan-spring-boot-starter test -DfailIfNoTests=false`.
- [x] Fixture corpus verification and deterministic regeneration pass.
- [x] From `loomspan-console`, `go test ./...` passes.
- [x] From `loomspan-console`, `go run ./internal/buildtool verify` passes.
- [x] With MSYS2 gcc on `PATH` and `CGO_ENABLED=1`, `go test -race ./...` passes.
- [x] `git diff --check` passes.
- [x] An obsolete-vocabulary search across source, active tests, generated fixtures, routed guidance, and phase docs returns no matches; historical ticket and plan documents and ignored build outputs are intentionally excluded:

  ```powershell
  rg -n "TOOL_CALL_REQUESTED|recordToolRequested" `
    loomspan-spring-boot-starter/src `
    loomspan-console/internal `
    loomspan-console/web `
    loomspan-console-fixtures `
    ai/skill-authoring `
    ai/thoughts/phases
  ```

- [x] Full-diff public-surface inspection finds no new public type, leaked internal signature, constructor, Spring bean, `@ConditionalOnMissingBean`, alias, adapter, fallback, legacy reader, or compatibility marker.

#### Manual Verification

- [x] Inspect every generated fixture change and confirm line endings remain LF.
- [x] Compare tool invocation counts before/after using equivalent executions; only physical trace record counts decrease.
- [x] Confirm success/failure correlation, planned/unplanned linkage, and frame hierarchy remain mechanically supported without adjacency inference.
- [x] Review the skill-authoring update against the LLM-first acceptance questions in `ai/skill-authoring/README.md`.

## Testing Strategy

### Unit Tests

- Java state/recorder tests for exact start cardinality and payload facts.
- Java journal tests for projection ownership, classification, and recursive sanitization.
- Java live projector tests for bounded input-free activity and one usage increment.
- Go enum/parser tests for the current-release accepted vocabulary.
- React component tests for explicit retrieval, parsing, display, inertness, and accessibility.

### Integration Tests

- Java callback/state/router integration for pre-side-effect ordering and success/failure lifecycle.
- Java-generated planned-success and unplanned-failure fixtures consumed by the Go analysis processor.
- Full browser build/typecheck/test verification through the repository build tool.
- Race-enabled Go test suite.

### Manual Testing Steps

1. Acquire or generate finalized traces containing planned success and unplanned failure tool calls.
2. Confirm each invocation has one start and one appropriate terminal record on the same tool frame.
3. Open Tool input and verify the expected identity, linkage, note, and arguments.
4. Verify arguments are absent before selection and absent from live activity.
5. Inspect a start without terminal evidence and confirm the UI makes no outcome claim.
6. Repeat the inspector flow using keyboard navigation and a screen reader or accessibility tree.

The dedicated testing-plan workflow in `ai/commands/3_testing_plan.md` should be run before implementation to map these behaviors to exact test names, failing-first expectations, mocks, fixtures, and exit criteria.

## Performance Considerations

- Removing one append per invocation reduces trace bytes, record parsing, indexing, and list cardinality.
- Tool input continues to use caller-triggered bounded range retrieval; no argument payload is added to initial record pages or live activity.
- Plan/task correlation must retain bounded paginated APIs and avoid an unbounded browser-side fetch outside the existing continuation contract.
- No new background work, index, cache, or retained browser state is introduced.

## Migration Notes

- No migration is provided. Previously retained traces containing `TOOL_CALL_REQUESTED` are not a protected cross-version interchange format and may be rejected by the current Go reader.
- Java and Go ship under the same exact `consoleCompatibilityVersion`; deploy them together.
- Regenerated fixtures replace the current-release contract rather than creating historical copies.
- Rollback is by reverting the complete atomic PR and its generated artifacts, not by enabling dual writing or reading.

## References

- Original ticket: `ai/thoughts/tickets/loomspan-console-pr-24-consolidate-tool-call-lifecycle.md`
- Planning workflow: `ai/commands/2_create_plan.md`
- Testing-plan workflow: `ai/commands/3_testing_plan.md`
- Framework design lens: `ai/thoughts/framework-feature-design-lens.md`
- Console repository instructions: `loomspan-console/AGENTS.md`
- Trace fixture contract: `loomspan-console-fixtures/README.md`
- Skill-authoring routing: `ai/skill-authoring/README.md`
- Skill-authoring trace guidance: `ai/skill-authoring/traces-and-debugging.md`
