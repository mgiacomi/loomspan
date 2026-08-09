# PR 22 Failed Trace Diagnostics Implementation Plan

## Overview

Make normally finalized failed and aborted Loomspan traces valid and explorable
without raw-NDJSON inspection. Java will record one bounded stack-trace
diagnostic at the closest frame that observes a throwable, Go will derive
terminality from the completion link and expose deliberate whole-diagnostic
retrieval, and Trace Explorer will present the linked failure as inert text.

This plan uses the simplified ticket contract approved on 2026-08-08. It does
not introduce diagnostic range cursors, speculative provider diagnostic kinds,
a separately wired attachment service, synthetic omission diagnostics, or a
custom search widget.

## Current State Analysis

Java and Go disagree about terminal failures. Java completion already requires a
`terminalFailureId` for `FAILED` and `ABORTED`, while runtime
`ERROR_RECORDED` metadata contains only `failureId`. Go additionally expects the
error metadata to contain `terminal: true`, and the hand-built Java fixture
corpus supplies that field even though the real runtime does not
(`TraceCompletion.java:17-32`, `DefaultExecutionTraceRecorder.java:140-168`,
`failures.go:18-82`, `ConsoleTraceFixtureCorpusTest.java:470-486`).

Failure recording is distributed across runtime catch boundaries. Model and
planning frames ordinarily close before the coordinator writes the canonical
error, while tool and validation paths can write an earlier error with an
unrelated ID. `LoomspanSession` has no throwable-identity registry, so one
propagating failure can produce multiple records and lose its closest frame
(`ExecutionCoordinator.java:100-178`, `DefaultToolCallbackFactory.java:157-195`,
`LoomspanSession.java:42-55`).

Failure payloads deliberately omit exception messages, causes, suppressed
exceptions, and stack traces. Large logical payloads already use canonical
envelope/chunk records, but Go indexes `ERROR_RECORDED` before chunk
reconstruction and therefore cannot inspect a chunked error's data or retain its
payload descriptor (`TraceFailureMetadata.java:6-28`,
`DefaultExecutionTraceHandle.java:398-505`, `processor.go:110-203`).

The console already provides scoped artifact handles, leases, bounded payload
access, failure/frame reverse links, and inert `<pre>` rendering. Failure rows do
not contain diagnostic descriptors, frames do not visibly advertise failures,
and acquisition invalidity does not surface `rawDownloadAvailable` guidance
(`dto.go:149-162`, `TraceExplorer.tsx:189-376`,
`TraceEvidenceDetail.tsx:3-6`, `TraceDetail.tsx:146-190`).

PR 21 is present on the base (`e7c7051`, followed by cleanup `a0307cf`), so
regenerated fixtures must preserve its required `entrySkill` facts. The working
tree was clean at `fee40e8` before these planning-document edits.

## Desired End State

- Every normally finalized failed or aborted runtime trace contains exactly one
  completion-linked terminal failure and is accepted by Go analysis.
- The first catch boundary at the closest meaningful active frame records the
  throwable; later wrappers and propagation boundaries reuse its failure ID.
- Every recorded throwable carries one ordinary Java stack trace captured as
  UTF-8-safe bounded text with explicit truncation.
- Go validates diagnostic shape after reconstructing chunked error payloads,
  derives terminality solely from the completion link, and keeps diagnostic text
  out of summaries, indexes, manifests, and cursors.
- A scoped, leased, cancellable query returns one selected diagnostic in full,
  with a hard 1 MiB response bound and no diagnostic cursor protocol.
- Trace Explorer visibly links frames and failures and lets a developer load,
  wrap, copy, and download inert diagnostic text.
- Java runtime artifacts, the committed Java-owned corpus, Go analysis, browser
  DTOs, and UI tests agree on the same current-release contract.

### Key Discoveries

- `TraceCompletion` already contains the authoritative outcome/link invariant;
  the extra Go error-record terminal flag is redundant and incorrect
  (`TraceCompletion.java:17-67`, `failures.go:18-82`).
- Recording must occur before frame cleanup. Moving only the coordinator logic
  cannot recover model, planning, step, or tool frame ownership after unwind
  (`DefaultMissionExecutionEngine.java:145-190`,
  `DefaultPlanningService.java:383-452`).
- Validation exhaustion currently records a synthetic error before constructing
  the exception it throws. Those paths must construct once, record that exact
  throwable, and rethrow it so identity-based deduplication works
  (`StepLoopMissionExecutionEngine.java:444-509`,
  `StepLoopMissionExecutionEngine.java:752-762`).
- A chunked `ERROR_RECORDED` envelope reaches failure indexing with `data: null`;
  diagnostic validation and descriptor attachment require a bounded
  post-reconstruction join (`processor.go:110-203`, `payload.go:38-169`).
- The 1 MiB canonical per-diagnostic limit permits a single deliberate response;
  a new range/cursor family would add contract surface without increasing the
  safety bound (`limits.go:9-33`, `query_ranges.go:20-81`).

## What We're NOT Doing

- Provider retry, fallback, recovery, or OpenRouter-aware response decoding.
- Provider, client-library, or transport diagnostic producers or official kind
  names; PR 23 owns the first producer backed by a real evidence seam.
- A separately wired diagnostic attachment service or public diagnostic API.
- Synthetic diagnostics describing omitted diagnostics or a multi-source
  attachment policy.
- Diagnostic range sources, cursors, byte-offset continuation, or a custom
  in-page search widget.
- Structured stack-frame/cause/suppressed-exception DTOs or stack parsing.
- Secret detection, redaction, configurable sensitivity modes, or suppression of
  application exception messages.
- Best-effort parsing of malformed traces, legacy readers, trace migrations, or
  durable cross-process history.
- Synthetic error frames, source-code links, IDE links, MCP adapter changes, or
  diagnosis/root-cause inference.

## Skill-Authoring Documentation Impact

**Impact**: Affected

- **Rationale**: Skill authors are already routed to trace guidance when
  diagnosing terminal failures. This change adds closest-frame failure evidence,
  ordinary stack text, explicit truncation, deliberate loading, and a material
  sensitivity warning because exception messages and causes become recorded
  application content.
- **Documents to update**: `ai/skill-authoring/traces-and-debugging.md`.
- **Supporting evidence**: Java failure lifecycle and bounded-capture tests,
  runtime-produced Java/Go fixtures, Go diagnostic validation/query tests, and
  Trace Explorer component tests.
- **Coverage table update**: Not required. `README.md` already routes terminal
  failure diagnosis to the source-verified traces topic, and neither its task
  boundary nor confidence changes.
- **LLM-first usability**: Add a compact failure-inspection procedure and an
  explicit limitations/sensitivity block. Preserve the distinction between
  recorded evidence and inferred cause, state that serialized traces are
  current-checkout diagnostics, and name PR 22's lack of provider-body evidence
  without adding historical narrative.

## Contract and Compatibility Impact

| Surface | Classification and supporting evidence | Planned compatibility treatment |
| --- | --- | --- |
| Application API | No affected type is in the supported skill-author API. Failure capture stays under `com.lokiscale.loomspan.internal`; `LoomspanSessionRunner` is public only for internal package collaboration per `LoomspanPublicSurfaceArchitectureTest`. | Preserve the application API; expose no `Throwable`, registry, diagnostic builder, or capture method to skill code. |
| Supported SPI | `ExecutionTraceRecorder` and `ExecutionStateService` are technically public internal interfaces and framework-owned infrastructure beans, not documented replacement seams (`LoomspanAutoConfigurationBoundaryTest.java:111-123`). | Change internal signatures atomically; add no overload, replacement bean, or compatibility bridge. |
| Configuration and manifest contracts | No `loomspan.*` property or YAML field governs diagnostic capture. Fixed bounds are canonical trace rules, not author configuration. | No configuration or manifest change. Do not introduce sensitivity or size knobs. |
| Persisted or serialized contracts | Application artifact bytes, consumed canonical NDJSON, Go bundle rows, browser JSON, and TypeScript DTOs change for the exact current release. The release-derived `consoleCompatibilityVersion` requires exact Java/Go agreement. | Change all current-release producers and consumers atomically. Do not support prior-release artifacts or migrate catalogs. |
| Ephemeral diagnostic formats | `ERROR_RECORDED`, completion linkage, chunk reconstruction, failure index rows, diagnostic descriptors/results, fixtures, and UI presentation are affected current-run diagnostics. | Replace terminal-flag semantics in place, validate one coherent format, preserve diagnostic accuracy/security, and regenerate current fixtures. |
| Internal or accidentally exposed implementation | Session throwable state, recorder/state-service methods, Go calculation structs/index layout, service methods, browser handlers, and component state change. | Prefer one centralized design and remove obsolete paths. No aliases, dual terminality, legacy readers, or diagnostic-range machinery. |

- **Evidence of supported contracts**: The updated ticket, framework design lens,
  architecture allowlist/tests, exact compatibility check, current in-repository
  consumers, and Java-owned executable corpus.
- **Intended breaks**: Remove `metadata.terminal` from canonical error fixtures
  and Go interpretation; reject duplicate failure IDs instead of collapsing
  them; extend failure/browser DTOs with diagnostic descriptors; add one bounded
  diagnostic retrieval operation.
- **In-repository consumers to update**: Java runtime capture paths, trace/journal
  projection, Java tests and fixture generator, committed traces/expected files,
  Go processor/index/query/browser layers, TypeScript contracts/client, Trace
  Explorer components/tests, security warning, fixture README, and skill-author
  trace guidance.
- **Public-surface delta**: No Application API or Supported SPI additions. Public
  internal Java method signatures may change in place; the transport-neutral Go
  service and browser adapter gain one diagnostic query/result; TypeScript gains
  descriptor/result types.
- **Shim decision**: **No shim.** Loomspan is pre-1.0, the affected trace is a
  current-run diagnostic format, exact release compatibility keeps Java and Go
  together, and the console catalog is empty after restart.
- **Java-to-Go boundary coordination**: **Required.** Consumed canonical NDJSON
  changes in Java, while the application artifact endpoint continues streaming
  finalized bytes unchanged. Java writers/readers, Java-generated fixtures, Go
  reconstruction/validation/indexes, expected results, queries, browser DTOs,
  and documentation must ship together.

## Implementation Approach

Centralize throwable identity and diagnostic capture behind one internal
failure-recording operation. Store the registry on `LoomspanSession` using
identity semantics, traverse causes with identity-based cycle detection and a
small fixed depth bound, and append only on first observation. Existing catch
boundaries call that operation before closing or unwinding their active frame;
coordinator and runner boundaries resolve and reuse the returned ID.

Capture `Throwable.printStackTrace(PrintWriter)` through a bounded writer that
never constructs an unbounded intermediate string. Keep a substantially larger
UTF-8-safe head than tail, account for the omission marker inside the 1 MiB
limit, and emit one `JAVA_STACK_TRACE` descriptor/value.

In Go, retain the chunked error's existing payload descriptor, then validate the
bounded logical error after payload finalization. Build compact failure rows with
context and diagnostic descriptors but no text. A selected diagnostic query
decodes the one bounded logical error object and returns one complete diagnostic;
it does not create another persistent component or cursor family.

Implement and verify each phase test-first. Before source implementation, create
the dedicated testing-plan artifact with `ai/commands/3_testing_plan.md` so the
failing tests, fixture regeneration order, and exit criteria are explicit.

## Phase 1: Centralize Java Failure Identity and Bounded Diagnostics

### Overview

Introduce the internal data and state needed to capture one stack diagnostic and
reuse one failure ID without changing exception propagation.

### Changes Required

#### 1. Diagnostic value and bounded stack capture

**Files**:

- `loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/core/TraceFailureMetadata.java`
- New focused internal types under
  `loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/core/`
- Focused tests under the matching `src/test/java` package

**Changes**:

- Define the ordered diagnostic value with exactly `kind`, `contentType`, `text`,
  `truncated`, and `captureLimitBytes`.
- Introduce only `JAVA_STACK_TRACE` and
  `text/plain; charset=utf-8`; keep kinds as bounded strings rather than a closed
  cross-language enum.
- Capture `printStackTrace` through a bounded `Writer`/encoder, retaining a
  larger head and a root-cause-preserving tail with an explicit in-band marker.
  Include the marker in the 1 MiB byte budget and never split a UTF-8 code point
  or surrogate pair.
- Preserve existing safe/context fields while adding the diagnostics array.
- Prove exception message, causes, suppressed exceptions, ordinary whitespace,
  Unicode boundaries, head/tail truncation, and exact byte limits.

#### 2. Session-local identity registry and recorder contract

**Files**:

- `loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/core/LoomspanSession.java`
- `loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/core/ExecutionTraceRecorder.java`
- `loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/core/DefaultExecutionTraceRecorder.java`
- `loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/runtime/state/ExecutionStateService.java`
- `loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/runtime/state/DefaultExecutionStateService.java`
- `loomspan-spring-boot-starter/src/test/java/com/lokiscale/loomspan/internal/runtime/state/ExecutionStateServiceTest.java`
- `loomspan-spring-boot-starter/src/test/java/com/lokiscale/loomspan/internal/core/LoomspanSessionTest.java`

**Changes**:

- Add session-owned, nonstatic identity state mapping observed throwable objects
  to the first recorded failure ID and origin-frame identity.
- Replace caller-generated UUID/error writes with one internal
  `recordFailure(session, throwable, context)` operation that returns the stable
  ID and appends only once.
- On a new wrapper, traverse causes with an identity visited set and a fixed
  depth bound; reuse the first already-recorded cause ID and associate the
  wrapper for faster later lookup. Never match by type, message, or stack text.
- Keep distinct throwable objects distinct when neither is an identity/cause
  match. Terminate safely for cyclic or very deep cause graphs.
- Ensure tracing failures cannot replace, suppress, mutate, or change the
  throwable returned to existing callers.
- Keep the registry and capture types internal and discard them with the session.

### Success Criteria

#### Automated Verification

- [x] New bounded-capture tests fail before implementation and pass afterward.
- [x] Same-object and wrapper-cause propagation return one stable failure ID.
- [x] Equal-looking distinct throwables remain distinct; cyclic/deep causes
  terminate deterministically.
- [x] A captured stack is valid UTF-8, never exceeds 1 MiB, and contains the
  required truncation marker/state when shortened.
- [x] Focused Java tests pass:
  `./mvnw.cmd -pl loomspan-spring-boot-starter test -Dtest=ExecutionStateServiceTest,LoomspanSessionTest,LoomspanSessionRunnerTest -DfailIfNoTests=false`.

#### Manual Verification

- [x] Review the new Java types to confirm none enter `com.lokiscale.loomspan.api`
  or become a Spring replacement seam.
- [x] Inspect a representative captured stack and confirm it remains familiar
  JVM text with useful head and root-cause tail context.

---

## Phase 2: Record at the Closest Frame and Finalize with One Failure ID

### Overview

Route every required runtime failure seam through the centralized recorder before
frame cleanup, while preserving existing cleanup and propagation behavior.

### Changes Required

#### 1. Model, planning, step, and tool frame capture

**Files**:

- `loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/runtime/DefaultMissionExecutionEngine.java`
- `loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/runtime/planning/DefaultPlanningService.java`
- `loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/runtime/step/StepLoopMissionExecutionEngine.java`
- `loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/runtime/tool/DefaultToolCallbackFactory.java`
- Corresponding focused runtime tests

**Changes**:

- Record model/planning/step/tool throwables while their closest frame remains
  active, then close the frame using the returned failure ID.
- For validation/linter/output exhaustion, construct the terminal exception
  first, record that exact instance at the owning frame, and throw the same
  object. Remove the independent pre-throw UUID/error path.
- Record timeout, interruption, and cancellation wrappers before caller cleanup
  changes frame state, while preserving interrupt status and cancellation order.
- Do not invent a frame when failure occurs before the first frame opens.

#### 2. Coordinator, runner, and finalization reuse

**Files**:

- `loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/core/ExecutionCoordinator.java`
- `loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/core/LoomspanSessionRunner.java`
- `loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/core/LoomspanSession.java`
- `loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/core/TraceCompletion.java`
- `loomspan-spring-boot-starter/src/test/java/com/lokiscale/loomspan/internal/core/ExecutionCoordinatorTest.java`
- `loomspan-spring-boot-starter/src/test/java/com/lokiscale/loomspan/internal/core/LoomspanSessionRunnerTest.java`

**Changes**:

- Resolve propagated errors through the registry rather than creating a new
  UUID. Use the returned ID consistently in frame-close metadata and the final
  completion.
- Preserve a recovered lower-level failure as a recorded nonterminal fact when
  a later successful completion does not link it.
- Record a cleanup/finalization throwable only when it becomes the effective
  terminal failure; otherwise retain existing suppressed-exception semantics.
- Preserve runner fallback behavior for pre-frame and unexpectedly open-frame
  failures without duplicating a failure already recorded below.
- Keep `TraceCompletion` as the sole Java terminal-outcome authority.

### Success Criteria

#### Automated Verification

- [x] Model and tool failures are recorded against their active frame before it
  closes.
- [x] Nested propagation, a normal wrapper, mission boundary, and runner boundary
  produce one error record and one completion-linked ID.
- [x] Failed and aborted completions reference exactly one existing failure;
  success references none and may retain recovered error facts.
- [x] Pre-frame, timeout/interruption, cleanup, and journal-projection failure
  tests preserve completion and propagation semantics.
- [x] Java starter tests pass:
  `./mvnw.cmd -pl loomspan-spring-boot-starter test -DfailIfNoTests=false`.

#### Manual Verification

- [x] Inspect representative failure records and confirm no synthetic error frame
  or duplicate root-level error was introduced.
- [x] Confirm original throwables, suppression, interrupt status, and cleanup
  behavior are unchanged apart from trace evidence.

---

## Phase 3: Regenerate the Contract and Build Go Diagnostic Analysis

### Overview

Make runtime-produced artifacts the executable Java/Go contract, correct Go
terminality, validate diagnostics after payload reconstruction, and expose one
bounded whole-diagnostic operation.

### Changes Required

#### 1. Java-owned corpus and real runtime fixtures

**Files**:

- `loomspan-spring-boot-starter/src/test/java/com/lokiscale/loomspan/internal/runtime/trace/ConsoleTraceFixtureCorpusTest.java`
- `loomspan-console-fixtures/traces/*.ndjson`
- `loomspan-console-fixtures/expected/*.json`
- `loomspan-console-fixtures/README.md`

**Changes**:

- Remove manually authored `terminal` fields from valid error metadata; retain
  resolvable completion links.
- Add at least one real failing and one real aborted runtime artifact produced
  through `LoomspanSessionRunner` or an equivalent complete skill path, rather
  than hand-authoring the terminal relationship.
- Cover unframed and recovered errors plus large/truncated stack diagnostics.
- Add deterministic invalid cases for missing/blank fields, invalid UTF-8,
  incorrect capture bounds, count/aggregate overflow, unknown terminal ID,
  duplicate failure ID, failure without a terminal link, and success with a
  terminal link.
- Preserve PR 21 `entrySkill` facts and LF line endings; inspect corpus churn and
  require a second regeneration to be diff-free.

#### 2. Post-reconstruction failure validation and compact indexing

**Files**:

- `loomspan-console/internal/traceanalysis/failures.go`
- `loomspan-console/internal/traceanalysis/model.go`
- `loomspan-console/internal/traceanalysis/processor.go`
- `loomspan-console/internal/traceanalysis/payload.go`
- `loomspan-console/internal/traceanalysis/index_format.go`
- `loomspan-console/internal/traceanalysis/dto.go`
- Related calculation, processor, payload, index, fixture, and service tests

**Changes**:

- Remove error-record `terminal` parsing. After the unique final completion is
  known, require its ID to resolve to exactly one canonical error and derive
  terminal status for that row only.
- Reject every duplicate failure ID, including byte-identical duplicates,
  instead of silently keeping the first.
- Retain the error record's existing reconstructed payload descriptor, then
  materialize at most the canonical 4 MiB logical error object after payload
  finalization to validate context and diagnostic metadata/text.
- Validate required bounded kind/content type/text/truncation/capture-limit
  fields, UTF-8 text, at most 1 MiB per diagnostic, 16 items, and 4 MiB aggregate.
  Accept unknown nonblank kinds and repeated kinds as opaque ordered content.
- Store exception type/context summary and diagnostic descriptors in the failure
  index. Descriptors include ordinal, kind, media type, truncation, declared
  limit, and decoded byte length, but never diagnostic text.
- Keep one reconstructed JSON copy in the existing payload store; do not add a
  diagnostic blob component or text to manifests/list rows/cursors.

#### 3. Whole-diagnostic service query

**Files**:

- `loomspan-console/internal/traceanalysis/service.go`
- `loomspan-console/internal/traceanalysis/query_facts.go` or a focused new
  `query_diagnostics.go`
- `loomspan-console/internal/traceanalysis/dto.go`
- Focused service/query tests

**Changes**:

- Add request/result types bound to artifact handle, failure ID, and diagnostic
  ordinal; return descriptor metadata and complete text in one response.
- Reuse current target-scope validation, leases, capacity accounting,
  cancellation, installed component ownership, and error taxonomy.
- Re-read and decode the one bounded logical error from its payload descriptor,
  verify the selected descriptor still matches the indexed fact, and cap returned
  text at the canonical 1 MiB limit.
- Add no diagnostic `RangeSource`, cursor operation, continuation fingerprint,
  or persistent decoded copy.

### Success Criteria

#### Automated Verification

- [x] The runtime-produced failed and aborted Java fixtures are accepted by Go
  and derive exactly one terminal failure at the closest recorded frame.
- [x] Missing model response after prepared/sent remains an analysis gap rather
  than invalid content.
- [x] Terminal-link and malformed-diagnostic cases fail atomically with stable
  categories; unknown/repeated kinds remain retrievable.
- [x] Large diagnostic text appears once in the payload store and not in compact
  indexes, manifests, summaries, or cursors.
- [x] Whole-diagnostic retrieval is scope/handle/failure/ordinal bound, cancelled
  promptly, rejects stale or mismatched artifacts, and never returns over 1 MiB.
- [x] Fixture regeneration passes:
  `./mvnw.cmd -pl loomspan-spring-boot-starter test -Dtest=ConsoleTraceFixtureCorpusTest -Dloomspan.console.fixtures.regenerate=true -DfailIfNoTests=false`.
- [x] Go tests pass from `loomspan-console`: `go test ./...`.

#### Manual Verification

- [x] Inspect regenerated fixtures and confirm diffs are limited to PR 22
  failure/diagnostic facts while preserving PR 21 identity.
- [x] Regenerate the corpus twice and confirm the second run produces no diff.
- [x] Inspect bundle components and confirm no second diagnostic-text copy was
  introduced.

---

## Phase 4: Expose and Present Frame-Linked Diagnostics

### Overview

Carry compact descriptors and deliberate retrieval through the browser adapter,
then make failure evidence visible and usable without raw records.

### Changes Required

#### 1. Browser API and TypeScript client

**Files**:

- `loomspan-console/internal/browserapi/router.go`
- `loomspan-console/internal/browserapi/trace_analysis.go`
- `loomspan-console/internal/browserapi/trace_analysis_test.go`
- `loomspan-console/internal/browserapi/contracts_test.go`
- `loomspan-console/web/src/api/contracts.ts`
- `loomspan-console/web/src/api/client.ts`

**Changes**:

- Extend failure DTOs with exception/context fields and ordered diagnostic
  descriptors, never text.
- Add one authenticated POST handler/client operation for selected diagnostic
  retrieval. Map the neutral service result directly and preserve scope/handle
  validation and existing browser error behavior.
- Keep raw payload, raw-record, raw-artifact, and diagnostic identities distinct;
  diagnostics do not enter the existing range contract.

#### 2. Trace Explorer failure and diagnostic experience

**Files**:

- `loomspan-console/web/src/observability/TraceExplorer.tsx`
- `loomspan-console/web/src/observability/TraceFailureFocus.tsx`
- `loomspan-console/web/src/observability/TraceHierarchy.tsx`
- `loomspan-console/web/src/observability/TraceEvidenceDetail.tsx` or a focused
  diagnostic component
- `loomspan-console/web/src/observability/TraceRecords.tsx`
- `loomspan-console/web/src/observability/TraceExplorer.test.tsx`
- `loomspan-console/web/src/observability/TraceViews.test.tsx`
- `loomspan-console/web/src/styles/index.css`

**Changes**:

- Visibly mark hierarchy frames with associated failures and support navigation
  from frame to failure and back without changing hierarchy-first selection.
- Show terminal/recovered state, exception/context summary, frame/route, existing
  attempt/retry/validation links, and diagnostic descriptors.
- Fetch text only when its load control is activated. Render it as React text in
  `<pre>`, never as HTML or Markdown.
- Add wrap/no-wrap, clipboard copy, and UTF-8 text-download actions. Keep
  truncation visible outside the text and rely on browser-native find.
- Preserve keyboard tree behavior, focus restoration, accessible names/status,
  and generic labels for unknown kinds.

#### 3. Invalid-artifact guidance and sensitivity warning

**Files**:

- `loomspan-console/web/src/observability/TraceDetail.tsx`
- `loomspan-console/web/src/observability/TraceDetail.test.tsx`
- `loomspan-console/web/src/target/Overview.tsx`
- Corresponding component tests/styles

**Changes**:

- When acquisition/analysis invalidity includes
  `rawDownloadAvailable: true`, explicitly retain and explain the raw attachment
  download action rather than displaying only the validation message.
- Update the authenticated local-console warning to say trace error details now
  include application exception messages, causes, suppressed exceptions, and
  stack text that Loomspan does not secret-scan or redact.
- Preserve the separation between `Acquire for analysis` and
  `Download raw attachment` established by commit `fee40e8`.

### Success Criteria

#### Automated Verification

- [x] Adapter contract tests prove descriptors stay compact and diagnostic text
  appears only in the explicit retrieval response.
- [x] A failed trace deep-loads its terminal failure and linked frame even when
  failure pages are paginated.
- [x] Frame/failure navigation is bidirectional; terminal and recovered states
  remain distinct.
- [x] Diagnostic content is inert and preserves whitespace; wrap, copy,
  download, truncation, unknown-kind, keyboard, focus, and screen-reader cases
  pass component tests.
- [x] `rawDownloadAvailable: true` produces explicit raw-download guidance.
- [x] From `loomspan-console`, browser and repository verification passes:
  `go run ./internal/buildtool verify`.

#### Manual Verification

- [ ] Acquire a runtime-produced failed trace and navigate model/step/root frames
  to the closest linked failure without opening raw NDJSON.
- [ ] Load a large truncated stack, use browser find, toggle wrapping, copy it,
  and download byte-valid UTF-8 text.
- [ ] Confirm markup-like exception text renders literally and cannot create DOM
  elements or links.
- [ ] Reject a deliberately invalid artifact and confirm raw attachment download
  remains available and clearly distinct from acquisition.

---

## Phase 5: Synchronize Author Guidance and Complete Verification

### Overview

Document the observable failure model and sensitivity boundary, then execute the
full cross-runtime verification sequence.

### Changes Required

#### 1. LLM-first traces and debugging guidance

**File**: `ai/skill-authoring/traces-and-debugging.md`

**Changes**:

- Explain that terminality comes from the final completion link and that earlier
  recorded failures can be recovered/nonterminal.
- Add a concise procedure for navigating from a failure to its originating frame
  and deliberately loading the bounded stack diagnostic.
- State that the stack is opaque recorded evidence, not parsed or inferred root
  cause, and that missing provider bodies remain unavailable in PR 22.
- State the 1 MiB truncation behavior and the application-content sensitivity
  boundary, including the lack of secret detection/redaction.
- Add stable named anchors for the failure recorder tests, runtime fixture corpus,
  Go diagnostic query tests, and Trace Explorer tests. Do not add duplicated
  implementation narrative.
- Leave the README routing and coverage classification unchanged after confirming
  the topic remains source-verified and within the same task boundary.

#### 2. Full repository verification

**Files**: No production changes; verification and diff review only.

**Changes**:

- Run focused tests during each phase, then the complete Java, corpus, Go,
  build-tool, and race-detector sequence.
- Review public-surface architecture tests, fixture determinism/LF endings,
  generated web artifacts, and the final diff for accidental provider, range,
  search, compatibility, or redaction machinery.

### Success Criteria

#### Automated Verification

- [x] Full Java starter suite passes:
  `./mvnw.cmd -pl loomspan-spring-boot-starter test -DfailIfNoTests=false`.
- [x] Intentional corpus regeneration passes and a second run is clean:
  `./mvnw.cmd -pl loomspan-spring-boot-starter test -Dtest=ConsoleTraceFixtureCorpusTest -Dloomspan.console.fixtures.regenerate=true -DfailIfNoTests=false`.
- [x] From `loomspan-console`, `go test ./...` passes.
- [x] From `loomspan-console`, `go run ./internal/buildtool verify` passes.
- [x] With MSYS2 GCC available, `CGO_ENABLED=1 go test -race ./...` passes after
  prepending `C:\msys64\mingw64\bin` to `PATH`.
- [x] Skill-authoring claims are supported by the cited tests, fixtures, and
  production anchors and satisfy the README's LLM-first standard.

#### Manual Verification

- [x] Review the final feature against every ticket acceptance signal and
  guardrail.
- [x] Confirm no Application API, Supported SPI, manifest, or configuration
  contract was added.
- [x] Confirm no legacy terminal flag, diagnostic cursor/range, speculative
  provider kind, synthetic omission item, or custom search implementation
  remains in the diff.

## Testing Strategy

Create the detailed testing artifact with `ai/commands/3_testing_plan.md` before
implementation. It should order tests so the original runtime-producer/Go-reader
mismatch fails first, followed by closest-frame identity, bounded capture,
post-reconstruction validation, deliberate retrieval, and UI behavior.

### Unit Tests

- Java identity registry, wrapper traversal, cycles/depth, distinct throwables,
  bounded UTF-8 writer, truncation, and propagation invariance.
- Go completion-link derivation, duplicate IDs, diagnostic schema/bounds,
  post-chunk indexing, descriptor compactness, and selected retrieval.
- Browser DTO mapping and React rendering/actions/accessibility.

### Integration Tests

- Real failed and aborted Java runtime artifacts processed through the committed
  Java/Go corpus.
- Failure-frame reverse linkage and missing model-response gap preservation.
- Browser acquisition, failure deep-linking, whole-diagnostic retrieval, and raw
  attachment fallback.

### Manual Testing Steps

1. Run a skill whose model call throws after request preparation/sending.
2. Acquire the trace and confirm the entire pre-failure hierarchy is present.
3. Navigate to the originating frame and terminal failure in both directions.
4. Load the Java stack, verify truncation state, wrapping, browser find, copy,
   and text download.
5. Repeat with a recovered error and confirm it is nonterminal.
6. Open an invalid artifact and confirm raw download remains available when the
   domain detail allows it.

## Performance Considerations

- Java stack capture must be streaming and hard-capped; do not allocate an
  unbounded `StringWriter` or traverse an unbounded cause graph.
- Ordinary failure pages, frame rows, summaries, manifests, and cursors remain
  small because they carry descriptors only.
- Go may materialize one logical error object up to the canonical 4 MiB bound at
  indexing and deliberate retrieval. Retrieval happens once per user action,
  returns at most 1 MiB, and creates no persistent duplicate.
- Reuse the existing payload store, bundle capacity accounting, artifact lease,
  cancellation, and target scope. Do not add a cache unless measurements show
  repeated deliberate decoding is a real problem.
- Browser memory is bounded by one selected diagnostic plus existing explorer
  pages; changing selections should release the previous text state.

## Migration Notes

No migration or compatibility shim is required. The console accepts only the
exact release-derived `consoleCompatibilityVersion`, the in-memory catalog is
empty after restart, and traces are current-run diagnostics. Regenerate the
current fixture corpus and change Java, Go, browser, and documentation contracts
atomically. Manually retained prior-release attachments remain downloadable raw
but are not made analyzable through a legacy reader.

## References

- Original ticket: `ai/thoughts/tickets/loomspan-console-pr-22-failed-trace-diagnostics.md`
- Related research: `ai/thoughts/research/2026-08-08-failed-trace-diagnostics.md`
- Framework lens: `ai/thoughts/framework-feature-design-lens.md`
- Console repository guidance: `loomspan-console/AGENTS.md`
- Skill-authoring routing: `ai/skill-authoring/README.md`
- Skill-authoring topic: `ai/skill-authoring/traces-and-debugging.md`
- Java failure producer: `loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/core/DefaultExecutionTraceRecorder.java:140-168`
- Go terminal validation: `loomspan-console/internal/traceanalysis/failures.go:18-82`
- Go payload reconstruction: `loomspan-console/internal/traceanalysis/processor.go:110-203`
- Existing browser failure focus: `loomspan-console/web/src/observability/TraceFailureFocus.tsx:9-33`
