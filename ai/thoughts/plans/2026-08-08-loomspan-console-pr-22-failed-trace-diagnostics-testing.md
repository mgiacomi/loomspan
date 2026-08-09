# PR 22 Failed Trace Diagnostics Testing Plan

## Change Summary

- Replace error-record `terminal` metadata with terminality derived exclusively
  from `TRACE_COMPLETED.terminalFailureId`.
- Record one canonical failure at the closest active frame and reuse its ID as
  the same throwable or a normal cause wrapper propagates.
- Capture one ordinary Java stack trace as bounded UTF-8 text, including explicit
  head/tail truncation when it exceeds 1 MiB.
- Validate chunked error diagnostics after Go payload reconstruction, retain only
  compact descriptors in derived rows, and retrieve one selected diagnostic in
  a deliberate whole-text response.
- Present frame-linked failure evidence and inert diagnostic text in Trace
  Explorer with load, wrap, copy, and download controls.
- Preserve raw attachment download when analysis rejects content, and disclose
  that application exception content is not secret-scanned or redacted.
- Change Java, consumed NDJSON, Go indexes/service, browser DTOs, TypeScript, and
  the Java-owned fixture corpus atomically for the current release. No legacy
  terminal reader, diagnostic cursor, or compatibility shim is tested or kept.

## Impacted Areas

- **Java failure identity and capture**:
  `LoomspanSession`, `ExecutionTraceRecorder`,
  `DefaultExecutionTraceRecorder`, `ExecutionStateService`,
  `DefaultExecutionStateService`, `TraceFailureMetadata`, and new focused
  internal diagnostic/registry types.
- **Java catch/finalization paths**: `DefaultMissionExecutionEngine`,
  `DefaultPlanningService`, `StepLoopMissionExecutionEngine`,
  `DefaultToolCallbackFactory`, `ExecutionCoordinator`,
  `LoomspanSessionRunner`, and journal/finalization fallback behavior.
- **Canonical trace storage**: `DefaultExecutionTraceHandle` envelope/chunk
  behavior and Java trace readers/live projection of the logical error payload.
- **Executable Java/Go contract**: `ConsoleTraceFixtureCorpusTest`, committed
  `loomspan-console-fixtures/traces` and `expected` files, and Go
  `fixture_corpus_test.go`.
- **Go trace analysis**: failure calculation, post-reconstruction diagnostic
  validation, compact index encoding, DTOs, installed bundle ownership, and the
  whole-diagnostic query.
- **Browser transport**: trace-analysis service interface, authenticated browser
  route, DTO mapping, request validation, and exact response contracts.
- **React UI**: Trace Explorer selection, hierarchy failure indicators, failure
  detail, diagnostic rendering/actions, raw-download fallback, security warning,
  keyboard behavior, focus, and accessible labels.
- **Compatibility/security evidence**: exact release-string rejection,
  Application API/SPI architecture boundaries, absence of console credentials
  from diagnostic responses, and current-run trace coherence.
- **Skill-authoring evidence**: closest-frame linkage, terminal/recovered
  distinction, opaque bounded stack text, truncation, deliberate loading,
  missing provider-body limitation, and sensitivity warning described by
  `ai/skill-authoring/traces-and-debugging.md`.

## Risk Assessment

### Highest-risk behaviors

- **Producer/consumer drift**: a hand-built fixture can again disagree with the
  real Java runtime. At least one failed and one aborted artifact must be
  produced through the runtime and consumed byte-for-byte by Go.
- **Wrong failure identity**: repeated catch boundaries can duplicate a failure,
  while overbroad matching can collapse distinct throwables. Identity and bounded
  cause traversal must be tested independently of text equality.
- **Wrong origin frame**: recording after a `finally` block or timeout cleanup can
  attach the failure to a parent or leave it unframed.
- **Concurrency races**: worker/caller timeout cleanup can observe the same
  throwable concurrently. Registry lookup/append must remain one-record-only
  without leaking identity across sessions.
- **Exception semantics regression**: trace capture must not replace the thrown
  object, lose suppressed failures, clear interruption, or change cleanup and
  completion outcomes.
- **Unbounded allocation**: `printStackTrace` capture, cause traversal, chunked
  error validation, and selected retrieval must honor byte/depth/count limits.
- **Chunk envelope misinterpretation**: Go currently indexes the physical
  `data:null` envelope. Diagnostics must be validated only after reconstruction
  without creating a second text store.
- **Terminal-link weakening**: accepting the incident must not admit missing,
  unknown, duplicate, or success-contradicting failure links.
- **Text leakage into broad responses**: diagnostic content must not appear in
  failure pages, frame pages, summaries, manifests, cursors, logs, or generic
  errors.
- **Unsafe UI interpretation**: stack content containing markup, Markdown, URLs,
  control-looking text, or secret-like application values must remain literal
  text. Console pairing/session/CSRF/application-auth credentials must not enter
  diagnostic responses.

### Edge cases

- Failure before any frame, nested-skill failure, model/planning/step/tool
  failure, validation exhaustion, timeout, interruption, cancellation, cleanup
  failure, completion append failure, and journal-projection failure.
- Same throwable observed repeatedly; wrapper whose direct/deep cause was
  recorded; equal type/message on distinct objects; mutual cause cycle; chain
  deeper than the registry bound; same throwable identity in different sessions.
- Recovered error followed by success; recovered and terminal errors in one
  trace; `FAILED` and `ABORTED`; prepared/sent attempt without a response.
- Stack below, exactly at, and above the 1 MiB byte cap; multibyte characters and
  surrogate pairs at head/tail boundaries; cause and suppressed sections in the
  retained tail; omission marker included in the bound.
- Inline versus chunked error payload; missing/empty/oversized diagnostics;
  missing/wrong field types; blank/oversized kind or media type; invalid UTF-8;
  nonpositive or over-limit `captureLimitBytes`; text longer than the declared
  limit; count 16 versus 17; aggregate at versus above 4 MiB; repeated and
  unknown kinds.
- Unknown failure, negative/out-of-range ordinal, expired handle, changed target
  scope, cancellation before/during decoding, corrupted index/descriptor
  mismatch, and exactly 1 MiB diagnostic response.
- UI selection changes while a diagnostic request is pending, stale-scope
  response, copy/download failures, unknown kind, truncated/nontruncated text,
  terminal/recovered indicators, and invalid artifact with/without raw download.

### Compatibility scope

- **Application API — protected**: existing architecture tests must continue to
  prove no failure registry, `Throwable`, or diagnostic builder leaks into
  `com.lokiscale.loomspan.api`.
- **Supported SPI — protected absence**: auto-configuration/public-surface tests
  continue to prove there is no supported replacement SPI or
  `@ConditionalOnMissingBean` seam for failure capture.
- **Configuration and manifest contracts — no change**: no new properties,
  manifest fields, size knobs, or sensitivity modes. Existing manifest tests do
  not need duplicate PR-specific cases.
- **Persisted/serialized current-release paths — coordinated change**: canonical
  NDJSON, Go installed components, browser JSON, and TypeScript DTOs move
  together. Exact `consoleCompatibilityVersion` rejection remains protected.
- **Ephemeral diagnostic formats — current-run coherence**: test the one new
  writer/reader/projector/analyzer/browser contract, not readability of prior
  `metadata.terminal` artifacts.
- **Internal or accidentally exposed implementation — approved removal**:
  remove tests that require error-record `terminal`, duplicate-ID collapse, or
  caller-generated independent failure IDs. Do not add simultaneous old/new
  expectations, overload tests, fallback readers, or diagnostic-range tests.

## Existing Test Coverage

### Java

- `LoomspanSessionRunnerTest#recordsFailedTerminalStatusWhenStandaloneRunnerActionFailsBeforeOpeningFrames`
  already proves a pre-frame failure is finalized and its error/completion IDs
  match. It does not inspect stack diagnostics or cross into Go.
- `ExecutionCoordinatorTest#marksTopLevelTraceErroredWhenMissionExecutionThrows`,
  `#preservesMissionFailureWhenCleanupAlsoFails`, and the aborted-completion
  coverage protect top-level outcome, cleanup suppression, and linkage, but not
  closest-frame identity reuse.
- `MissionExecutionEngineTest#recordsFailedModelFrameStatusWhenMissionCallThrows`
  protects failed model-frame closure; it does not currently require an error
  record while that frame is active.
- `PlanningServiceTest` covers planning/model frame lifecycle and retry
  exhaustion but has no closest-frame canonical failure assertion.
- `StepLoopMissionExecutionEngineTest#surfacesToolFailureAsExplicitTerminalFailure`
  and `#recordsTerminalFailureWhenInvalidActionRetriesAreExhausted` cover current
  failure facts but currently allow independent IDs/preconstructed payloads.
- `ExecutionTraceHandleTest#publishesCompleteLogicalChunkedRecordOnlyAfterAllChunksSucceed`
  protects canonical chunk publication, but not a large diagnostic payload.
- `ConsoleTraceFixtureCorpusTest` proves deterministic Java-owned fixtures and
  Java-side semantic invariants; current failed fixtures manually add
  `terminal: true` and are not produced through the real runner.

### Go and browser adapter

- `calculations_test.go` covers terminal resolution, recovered/terminal
  separation, missing responses, and structural failure relationships. Its
  current valid cases encode the obsolete error-record terminal flag and allow
  identical duplicate IDs to collapse.
- `payload_test.go` covers streamed text/JSON reconstruction, invalid chunks,
  UTF-8/content type, and large-payload storage behavior, but not semantic
  diagnostic validation after reconstruction.
- `fixture_corpus_test.go#TestFixtureCorpusMatchesJavaExpectedSemantics` is the
  existing cross-language oracle and is the correct place to consume the new
  runtime-produced artifacts.
- `service_test.go#TestServiceQueryFailures` and
  `#TestFrameQueriesExposeUsageCompletenessAndRecordedCrossReferences` cover
  compact failure/frame facts, leases, scopes, pagination, and handles. No
  selected-diagnostic operation exists.
- `trace_analysis_test.go#TestTraceAnalysisRoutesRequireSessionButNotCSRF`,
  `#TestTraceAnalysisMapsConfiguredLimitsAndDirectFailureRelationships`, and
  browser contract fixtures protect adapter authentication/mapping but have no
  diagnostic endpoint or DTO.
- `applicationclient/client_test.go#TestClientConsumesCommittedInstanceFixtureOnlyAfterExactCompatibility`
  protects the exact Java/Go release gate and must remain passing unchanged.

### React

- `TraceExplorer.test.tsx#failure focus selects the recorded terminal failure and never loads raw payloads`
  covers paginated terminal selection and current failure/frame links.
- `TraceViews.test.tsx#hierarchy supports semantic expansion and parent keyboard navigation`
  covers tree accessibility; `#evidence detail renders text inertly and exposes continuation`
  establishes literal `<pre>` rendering for existing ranges.
- `TraceDetail.test.tsx` covers separate acquire/raw-download actions but not the
  `rawDownloadAvailable` error detail.
- `Overview.test.tsx#overview presents established HTTP facts and safe incompatibility details`
  covers the existing unencrypted warning surface, not diagnostic sensitivity.

### Coverage gaps

- No current test drives exact bytes from a real failing Java runtime through Go.
- No identity/cause registry, bounded stack writer, diagnostic schema, compact
  descriptor, selected retrieval, or diagnostic browser action exists.
- No test proves that the same failure remains attached to the inner frame after
  propagation, or that a concurrent timeout path records once.
- No test proves diagnostic text stays out of broad responses/components while
  remaining available deliberately.
- Existing authoring guidance lacks executable evidence for closest-frame stack
  inspection, truncation, and sensitivity limitations.

## Bug Reproduction / Failing Test First

- **Name**: `TestTerminalFailureDerivesFromCompletionLinkWithoutErrorTerminalMetadata`
- **Type**: Go unit
- **Location**: `loomspan-console/internal/traceanalysis/calculations_test.go`
- **Arrange**: Build the smallest otherwise-valid calculation input containing
  one `ERROR_RECORDED` with only `metadata.failureId = "failure-1"`, followed by
  one `TRACE_COMPLETED` with outcome `FAILED` and
  `terminalFailureId = "failure-1"`. Do not add `metadata.terminal`.
- **Act**: Run the existing failure calculation/final terminal-link validation
  used by the processor.
- **Assert**: Calculation succeeds, emits one failure row, and derives
  `Terminal == true` for `failure-1` from the completion link.
- **Fixtures/data**: Inline records using the existing calculation-test helpers;
  no committed fixture and no filesystem/network setup.
- **Mocks**: None.
- **Expected failure (pre-fix)**: current `validateTerminalLink` sees the absent
  error-record flag as false and returns `INVALID_TERMINAL_FAILURE`.
- **Why first**: It is deterministic, inexpensive, and isolates the precise
  invalid assumption. It does not replace the required runtime-produced
  Java/Go conformance fixture.

Run only this red test before implementation:

```powershell
Set-Location loomspan-console
go test ./internal/traceanalysis -run '^TestTerminalFailureDerivesFromCompletionLinkWithoutErrorTerminalMetadata$' -count=1
```

## Tests to Add or Update

### 1. Completion-Link Terminality Matrix

- **Names**:
  `TestTerminalFailureDerivesFromCompletionLinkWithoutErrorTerminalMetadata`,
  `TestTerminalFailureRejectsMissingUnknownAndDuplicateLinks`, and update
  `TestFailuresKeepRecoveredErrorsSeparateFromTerminalFailure`.
- **Type**: Go unit, table-driven.
- **Location**: `loomspan-console/internal/traceanalysis/calculations_test.go`.
- **What it proves**: Success forbids a link; failed/aborted require one nonblank
  link; the link resolves to exactly one `ERROR_RECORDED`; only that row is
  terminal; recovered errors remain false; every duplicate ID is invalid; an
  obsolete error-record `terminal` field is neither required nor authoritative.
- **Fixtures/data**: Minimal inline record sequences covering success, failure,
  abort, recovered+terminal, missing, blank, unknown, duplicate, and formerly
  contradictory flags.
- **Mocks**: None.
- **Contract classification**: Ephemeral diagnostic formats.
- **Compatibility expectation**: Current-run diagnostic coherence; approved
  removal of the old terminal flag and duplicate-collapse behavior.

### 2. Session Failure Identity and Concurrency

- **Names**: `recordsThrowableOnceAcrossRepeatedObservations`,
  `reusesRecordedCauseForNormalWrapper`,
  `keepsEqualLookingDistinctThrowablesSeparate`,
  `boundsCyclicAndDeepCauseTraversal`,
  `isolatesThrowableIdentityAcrossSessions`, and
  `concurrentObservationsAppendOneFailure`.
- **Type**: Java unit.
- **Location**: a focused new test beside the session-local registry under
  `loomspan-spring-boot-starter/src/test/java/com/lokiscale/loomspan/internal/core/`,
  with state-service integration assertions in `ExecutionStateServiceTest`.
- **What it proves**: Identity, wrapper reuse, non-text matching, depth/cycle
  termination, per-session lifetime, atomic first append, stable ID, and stable
  origin frame under worker/caller races.
- **Fixtures/data**: Identity-linked throwable graphs, two equal-message
  exceptions, mutual cause cycle, over-bound chain, two test sessions, latches,
  and virtual threads using existing session test builders.
- **Mocks**: A recording `ExecutionTraceRecorder` test double that captures
  append count, payload, active frame, and returned ID; no Spring context.
- **Contract classification**: Internal or accidentally exposed implementation.
- **Compatibility expectation**: Approved atomic replacement of caller-generated
  independent IDs; no old overload/fallback behavior.

### 3. Bounded Ordinary Java Stack Capture

- **Names**: `capturesOrdinaryStackMessageCauseAndSuppressedText`,
  `retainsUtf8SafeHeadAndTailWithinOneMiB`,
  `doesNotTruncateAtOrBelowLimit`, and
  `streamsOversizedThrowableWithoutUnboundedIntermediate`.
- **Type**: Java unit.
- **Location**: a focused new test beside the bounded diagnostic writer/value
  under `.../src/test/java/com/lokiscale/loomspan/internal/core/`.
- **What it proves**: Exact five-field diagnostic shape, only
  `JAVA_STACK_TRACE`, ordinary `printStackTrace` ordering/whitespace, cause and
  suppressed sections, UTF-8 validity, byte-exact cap, marker-in-budget,
  substantially larger head than tail, correct `truncated`, and streaming writes.
- **Fixtures/data**: Nested/suppressed throwable, multibyte boundary strings,
  values just below/equal/above 1 MiB, and a throwable/test writer that emits a
  very large stack in small writes rather than constructing one test string.
- **Mocks**: None; use deterministic throwable/test writer helpers.
- **Contract classification**: Ephemeral diagnostic formats.
- **Compatibility expectation**: Current-run diagnostic usefulness and bounded
  security behavior; no structured stack schema.

### 4. Closest Model and Planning Frame Capture

- **Names**: update
  `MissionExecutionEngineTest#recordsFailedModelFrameStatusWhenMissionCallThrows`
  to require the canonical error before model-frame closure; add
  `PlanningServiceTest#recordsPlanningModelFailureOnActiveModelFrame`.
- **Type**: Java integration/unit at existing runtime service boundaries.
- **Location**: `MissionExecutionEngineTest.java` and `PlanningServiceTest.java`.
- **What it proves**: The error record uses the active model frame ID/type/route,
  precedes `FRAME_CLOSED`, contains one stack diagnostic, and the exact thrown
  object continues upward.
- **Fixtures/data**: Existing failing `ChatClient`/response stubs and in-memory
  trace handle; planning case includes the outer planning frame to prove the
  inner model frame wins.
- **Mocks**: Existing Spring AI client mocks/stubs; no network/provider call.
- **Contract classification**: Ephemeral diagnostic formats.
- **Compatibility expectation**: Current-run frame accuracy; old root-only
  association is intentionally removed.

### 5. Closest Step, Tool, and Validation Capture

- **Names**: update
  `StepLoopMissionExecutionEngineTest#surfacesToolFailureAsExplicitTerminalFailure`
  and `#recordsTerminalFailureWhenInvalidActionRetriesAreExhausted`; add focused
  output-schema/linter exhaustion coverage; add
  `ToolCallbackFactoryTest#recordsToolThrowableOnceOnActiveToolFrame`.
- **Type**: Java integration/unit.
- **Location**: `StepLoopMissionExecutionEngineTest.java` and
  `ToolCallbackFactoryTest.java`.
- **What it proves**: Tool/model/validation errors attach to the closest owning
  frame; validation constructs, records, and throws the same exception; later
  coordinator propagation does not append another error; frame-close metadata
  and completion use the same ID.
- **Fixtures/data**: Existing bound-tool callbacks, invalid step actions, linter
  and schema retry-exhaustion stubs, plus trace-record ordering assertions.
- **Mocks**: Existing model/tool test doubles; capture thrown object identity
  rather than matching message text.
- **Contract classification**: Ephemeral diagnostic formats.
- **Compatibility expectation**: Current-run failure visibility; remove tests
  that accept independent pre-throw and coordinator IDs.

### 6. Runner, Timeout, Abort, Cleanup, and Recovery Semantics

- **Names**: extend
  `LoomspanSessionRunnerTest#recordsFailedTerminalStatusWhenStandaloneRunnerActionFailsBeforeOpeningFrames`,
  `ExecutionCoordinatorTest#preservesMissionFailureWhenCleanupAlsoFails`, and
  mission timeout tests; add `propagatedFailureKeepsClosestOriginAndStableId`,
  `recoveredFailureIsNotCompletionLinked`, and
  `abortedCompletionLinksRecordedFailureOnce`.
- **Type**: Java integration.
- **Location**: `LoomspanSessionRunnerTest.java`,
  `ExecutionCoordinatorTest.java`, and `MissionExecutionEngineTest.java`.
- **What it proves**: Pre-frame errors remain unframed; timeout/interruption is
  recorded before cleanup moves frame state; abort gets one resolvable failure;
  cleanup remains suppressed unless it becomes terminal; recovered errors are
  retained but omitted from successful completion; original exception and
  interrupt status are preserved.
- **Fixtures/data**: Existing latch-controlled blocking mission, injected trace
  append/finalization failures, nested coordinator/session, and recording handle.
- **Mocks**: Existing executor, trace-handle, and state-service test doubles.
- **Contract classification**: Internal or accidentally exposed implementation.
- **Compatibility expectation**: Preserve exception/cleanup behavior while
  atomically replacing failure-record mechanics.

### 7. Canonical Chunk Publication for Large Diagnostics

- **Names**: add
  `ExecutionTraceHandleTest#publishesChunkedErrorDiagnosticAsOneLogicalRecord`
  and update NDJSON reader tests for the logical reconstructed diagnostic.
- **Type**: Java unit/integration.
- **Location**: `ExecutionTraceHandleTest.java` and
  `NdjsonExecutionTraceReaderTest.java`.
- **What it proves**: A large error becomes one envelope plus ordered chunks;
  physical records remain monotonic; `data` is null only on the envelope; live
  observation and the Java reader see one complete logical error after all chunks;
  partial chunk failure publishes no logical record.
- **Fixtures/data**: Deterministic diagnostic above the 4,096-character chunk
  threshold and injected envelope/middle/final chunk write failures.
- **Mocks**: Existing recording observation handle and failing trace store.
- **Contract classification**: Ephemeral diagnostic formats.
- **Compatibility expectation**: Preserve existing canonical chunk integrity for
  the new payload shape.

### 8. Runtime-Produced Cross-Language Corpus

- **Names**: extend `ConsoleTraceFixtureCorpusTest` inventory/invariant tests with
  `runtime-terminal-failure` and `runtime-terminal-abort`; consume them through
  `TestFixtureCorpusMatchesJavaExpectedSemantics`.
- **Type**: Cross-runtime integration fixture.
- **Location**: Java corpus generator, committed
  `loomspan-console-fixtures/{traces,expected}`, and Go
  `fixture_corpus_test.go`.
- **What it proves**: Exact bytes from real Java failure/abort finalization have
  PR 21 `entrySkill`, complete frames, one completion-linked terminal error with
  no error `terminal` flag, one bounded stack descriptor, and a supported
  prepared/sent-without-response gap; Go installs a normal bundle and matches
  expected facts.
- **Fixtures/data**: Deterministic clock/IDs/model failure and abort path using
  the real runner/trace handle. Hand-built fixtures remain only for isolated
  malformed mutations.
- **Mocks**: Deterministic Java model/executor dependencies; no network. Go reads
  committed bytes without mocking the processor.
- **Contract classification**: Persisted or serialized contracts for the exact
  current release, carrying an ephemeral diagnostic format.
- **Compatibility expectation**: Java-to-Go boundary coordination; regenerate in
  place, preserve exact release gating, and do not retain prior terminal fixtures.

### 9. Diagnostic Schema and Post-Reconstruction Validation Matrix

- **Names**: `TestProcessorValidatesDiagnosticsAfterChunkReconstruction` and
  `TestProcessorRejectsMalformedFailureDiagnostics`.
- **Type**: Go unit/integration, table-driven.
- **Location**: `payload_test.go`, `processor_test.go`, and/or a focused new
  `diagnostics_test.go` in `internal/traceanalysis`.
- **What it proves**: Inline and chunked errors use the reconstructed object;
  required fields/types, bounded kind/content type, UTF-8, positive canonical
  capture limit, text-within-declared-limit, 1 MiB item, 16-item count, and 4 MiB
  aggregate are enforced; unknown and repeated nonblank kinds remain valid;
  malformed artifacts fail before bundle publication.
- **Fixtures/data**: Boundary tables using production constants at below/equal/
  above values, chunk envelope+chunks, invalid raw UTF-8, unknown/repeated kinds,
  and a sink snapshot checked after failure.
- **Mocks**: In-memory artifact source/sink used by current processor tests.
- **Contract classification**: Ephemeral diagnostic formats.
- **Compatibility expectation**: One strict current-version schema; no fallback
  for missing diagnostics or old terminal metadata.

### 10. Compact Failure Index and Single-Copy Storage

- **Names**: `TestFailureIndexStoresDescriptorsWithoutDiagnosticText` and
  `TestLargeDiagnosticUsesOnlyExistingPayloadStore`.
- **Type**: Go unit/integration.
- **Location**: `index_test.go`, `bundle_test.go`, and `service_test.go`.
- **What it proves**: Failure rows retain context, payload descriptor, and ordered
  diagnostic descriptors; no text appears in the failure index, frame index,
  manifest, summary, cursor, or extra component; bundle accounting includes only
  the existing payload store and compact indexes.
- **Fixtures/data**: Unique sentinel text near 1 MiB in a chunked error; scan all
  installed components and serialized broad DTOs for absence/presence.
- **Mocks**: Existing artifact sink/installed lease helpers.
- **Contract classification**: Ephemeral diagnostic formats.
- **Compatibility expectation**: Current-run boundedness and no duplicate blob
  representation.

### 11. Whole-Diagnostic Service Lifecycle

- **Names**: `TestServiceGetFailureDiagnostic`,
  `TestServiceGetFailureDiagnosticRejectsMismatchedIdentity`, and
  `TestServiceGetFailureDiagnosticHonorsCancellationAndLeaseExpiry`.
- **Type**: Go service integration.
- **Location**: `service_test.go` or focused `query_diagnostics_test.go`.
- **What it proves**: Exact selected text and descriptor are returned once; the
  request is bound to scope, handle, failure ID, and ordinal; unknown/negative/
  out-of-range selections fail precisely; stale handles and target rotation do
  not disclose text; cancellation stops work; response never exceeds 1 MiB; no
  cursor or range source is created.
- **Fixtures/data**: Installed artifacts with two ordered descriptors including
  an unknown repeated kind, one maximum-size diagnostic, corrupt descriptor
  mismatch, expiring lease, changed scope, and cancelled context.
- **Mocks**: Existing target scope/artifact service test infrastructure; use a
  blocking reader only for deterministic cancellation.
- **Contract classification**: Persisted or serialized current-release service
  DTO over an ephemeral diagnostic format.
- **Compatibility expectation**: New current-release operation; explicitly no
  diagnostic range/cursor compatibility path.

### 12. Browser Adapter Contract and Authorization

- **Names**: `TestTraceAnalysisMapsFailureDiagnosticDescriptors`,
  `TestTraceAnalysisGetsSelectedDiagnosticOnly`, and extend
  `TestTraceAnalysisRoutesRequireSessionButNotCSRF`.
- **Type**: Go HTTP adapter unit/integration.
- **Location**: `internal/browserapi/trace_analysis_test.go` and
  `contracts_test.go`.
- **What it proves**: Failure pages contain descriptor metadata but not text;
  the diagnostic POST maps request/result exactly; it requires the browser
  session, follows existing trace-analysis CSRF policy, rejects malformed bodies,
  and never registers a diagnostic range or raw-artifact route.
- **Fixtures/data**: Fake neutral service with sentinel descriptor/text, missing
  session, malformed/oversized ordinal/body, changed scope, and domain errors.
- **Mocks**: Existing fake `TraceAnalysisService`; assert exact method arguments
  and serialized JSON inventory.
- **Contract classification**: Persisted or serialized current-release browser
  DTO.
- **Compatibility expectation**: Atomic Go/browser/TypeScript update; broad DTOs
  remain bounded.

### 13. Trace Explorer Frame/Failure Navigation

- **Names**: extend
  `failure focus selects the recorded terminal failure and never loads raw payloads`;
  add `frame failure indicator navigates to recovered and terminal evidence` and
  `failure navigation restores hierarchy focus`.
- **Type**: React component integration.
- **Location**: `TraceExplorer.test.tsx` and `TraceViews.test.tsx`.
- **What it proves**: Failed traces retain full model/step/root hierarchy;
  failure-bearing frames are visibly and accessibly marked; navigation works in
  both directions; terminal/recovered states stay distinct; paginated deep links
  still resolve; selecting a failure does not eagerly request diagnostic text.
- **Fixtures/data**: Mock summary/frame/failure pages with terminal and recovered
  failures, deep ancestry, pagination, and unframed failure.
- **Mocks**: Existing mocked API module; assert diagnostic API call count is zero
  until explicit load.
- **Contract classification**: Ephemeral diagnostic formats.
- **Compatibility expectation**: Preserve hierarchy-first current-run behavior
  while adding direct failure visibility.

### 14. Inert Diagnostic Loading and Actions

- **Names**: `loadsSelectedDiagnosticDeliberatelyAndRendersItInertly`,
  `togglesWrappingWithoutChangingText`,
  `copiesAndDownloadsExactUtf8Diagnostic`,
  `keepsTruncationVisibleAndLabelsUnknownKindGenerically`, and
  `ignoresStaleDiagnosticResponseAfterSelectionOrScopeChange`.
- **Type**: React component integration.
- **Location**: a focused diagnostic component test plus
  `TraceExplorer.test.tsx`/`TraceViews.test.tsx`.
- **What it proves**: Explicit load only; literal `<pre>` text with preserved
  whitespace and no HTML/Markdown/link interpretation; browser-native find can
  see the complete loaded text; wrap changes presentation only; clipboard and
  Blob download bytes match UTF-8 input; truncation status stays external;
  controls have accessible names/status; stale results do not replace selection.
- **Fixtures/data**: Markup-like and Markdown-like text, multibyte characters,
  long/truncated diagnostic, unknown kind, pending/error/scope-change states.
- **Mocks**: Mock diagnostic API, `navigator.clipboard.writeText`,
  `URL.createObjectURL`/`revokeObjectURL`, and anchor click; do not mock React's
  text escaping.
- **Contract classification**: Ephemeral diagnostic formats.
- **Compatibility expectation**: Current-run diagnostic safety and usability;
  no custom search or range continuation behavior.

### 15. Invalid Analysis Raw-Download Fallback and Warning

- **Names**: add
  `TraceDetail.test.tsx#analysis invalidity preserves raw download when available`
  and `Overview.test.tsx#warning discloses unredacted application diagnostics`.
- **Type**: React component unit/integration.
- **Location**: `TraceDetail.test.tsx` and `target/Overview.test.tsx`.
- **What it proves**: `rawDownloadAvailable: true` keeps the confirmation-gated
  raw attachment path beside the analysis error; false/absent does not invent
  availability; acquire and raw download remain separate; warning names
  exception messages/causes/stack content and lack of secret scanning/redaction.
- **Fixtures/data**: `BrowserAPIError` details with true/false/absent values and
  selected unencrypted/encrypted targets.
- **Mocks**: Existing artifact API mocks and raw-download link behavior.
- **Contract classification**: Ephemeral diagnostic formats.
- **Compatibility expectation**: Preserve the protected raw pass-through action
  and make the changed sensitivity boundary visible.

### 16. Acquisition-to-Browser Diagnostic Integration

- **Name**: `TestAcquireRuntimeFailedTraceAndReadFailureDiagnostic`.
- **Type**: Go integration.
- **Location**: `loomspan-console/internal/console/artifact_integration_test.go`.
- **What it proves**: A Java-compatible target streams the committed
  runtime-failure fixture; central acquisition installs it; failure summary and
  frame linkage are available; the authenticated browser diagnostic operation
  returns exact text; raw download remains an independent fresh upstream stream.
- **Fixtures/data**: Reuse the committed runtime-failure NDJSON and exact
  compatible instance response; compare raw checksum and installed analysis
  semantics.
- **Mocks**: `httptest` Java target only; use real acquisition, processor,
  artifact service, browser router, and scoped handle.
- **Contract classification**: Persisted or serialized current-release
  Java-to-Go/browser boundary.
- **Compatibility expectation**: Protected exact release gate and atomic
  current-release consumer coherence; no historical reader.

### 17. Security and Public-Boundary Regressions

- **Names**: extend console security integration with
  `TestDiagnosticRouteDoesNotLeakConsoleCredentials`; rerun existing
  `LoomspanPublicSurfaceArchitectureTest`,
  `LoomspanAutoConfigurationBoundaryTest`, and
  `TestClientConsumesCommittedInstanceFixtureOnlyAfterExactCompatibility`.
- **Type**: Java architecture and Go integration regression.
- **Location**: existing architecture tests,
  `loomspan-console/internal/console/security_integration_test.go`, and
  `internal/applicationclient/client_test.go`.
- **What it proves**: No diagnostic/throwable/registry type leaks into the
  Application API or Supported SPI; no replacement bean appears; exact release
  mismatch is still rejected before acquisition; pairing cookies, CSRF tokens,
  application authentication headers, and workspace paths do not appear in
  diagnostic JSON/headers/log-facing errors. Application-owned text inside the
  artifact is returned exactly and is intentionally not redacted.
- **Fixtures/data**: Distinct sentinels for application diagnostic content and
  each console credential/header/path so absence assertions cannot alias.
- **Mocks**: Existing `httptest` target and authenticated browser session setup.
- **Contract classification**: Application API, Supported SPI, and persisted or
  serialized exact-release boundary.
- **Compatibility expectation**: Preserve deliberate public/security boundaries;
  approved internal signature changes receive no compatibility overload.

## Authoring Claims and Evidence Map

| Planned guidance claim | Required executable evidence |
| --- | --- |
| Failed/aborted terminality comes from the completion link; recovered errors can coexist with success | Tests 1, 6, and 8 |
| Failure evidence points to the closest frame and does not invent an error frame | Tests 4, 5, 6, 8, and 13 |
| One propagating throwable keeps one failure ID without text matching | Tests 2, 5, and 6 |
| The Java stack is ordinary opaque text containing message/cause/suppressed details | Tests 3, 8, 9, and 14 |
| Diagnostic capture/retrieval is bounded and truncation is explicit | Tests 3, 9, 11, and 14 |
| Diagnostic text loads deliberately and does not inflate broad analysis responses | Tests 10, 12, 13, and 14 |
| Missing provider response/body remains an explicit gap rather than invented evidence | Tests 8 and 9; assert only `JAVA_STACK_TRACE` is produced by Java |
| Application diagnostic content may be sensitive and is not secret-scanned/redacted, while console credentials remain protected | Tests 15 and 17 |
| Trace formats are current-checkout diagnostics, not a durable historical API | Corpus replacement and absence checks in Tests 8, 11, 12, and 17 |

## How to Run

### Prerequisites

- Run from `C:\opendev\code\loomspan` in PowerShell unless a command changes
  directory.
- Java 21 is used through the checked-in Maven wrapper.
- Go and the web toolchain are exercised through repository-standard commands.
- Fixture files must remain LF-only.
- Race tests require MSYS2 GCC at `C:\msys64\mingw64\bin` and
  `CGO_ENABLED=1`.
- Tests must use deterministic clocks/IDs and local fakes; no OpenRouter or other
  external provider credentials/network calls are required.

### Red test

```powershell
Set-Location loomspan-console
go test ./internal/traceanalysis -run '^TestTerminalFailureDerivesFromCompletionLinkWithoutErrorTerminalMetadata$' -count=1
Set-Location ..
```

Record the expected pre-fix `INVALID_TERMINAL_FAILURE` failure, then keep the
test unchanged while implementing the correction.

### Focused Java loops

```powershell
.\mvnw.cmd -pl loomspan-spring-boot-starter test -Dtest=LoomspanSessionRunnerTest,ExecutionCoordinatorTest,ExecutionStateServiceTest,MissionExecutionEngineTest,PlanningServiceTest,StepLoopMissionExecutionEngineTest,ToolCallbackFactoryTest,ExecutionTraceHandleTest,NdjsonExecutionTraceReaderTest -DfailIfNoTests=false
```

### Fixture contract

```powershell
.\mvnw.cmd -pl loomspan-spring-boot-starter test -Dtest=ConsoleTraceFixtureCorpusTest -DfailIfNoTests=false
.\mvnw.cmd -pl loomspan-spring-boot-starter test -Dtest=ConsoleTraceFixtureCorpusTest -Dloomspan.console.fixtures.regenerate=true -DfailIfNoTests=false
git diff --check -- loomspan-console-fixtures
```

Run the regeneration command a second time and require no new diff.

### Focused Go and browser loops

```powershell
Set-Location loomspan-console
go test ./internal/traceanalysis ./internal/browserapi ./internal/console ./internal/applicationclient
go run ./internal/buildtool verify
Set-Location ..
```

`go run ./internal/buildtool verify` is the repository-standard web typecheck,
unit-test, build, formatting, and generated-asset verification boundary; do not
replace it with an ad hoc npm-only sequence for final verification.

### Full verification

```powershell
.\mvnw.cmd -pl loomspan-spring-boot-starter test -DfailIfNoTests=false
Set-Location loomspan-console
go test ./...
go run ./internal/buildtool verify
$env:PATH = "C:\msys64\mingw64\bin;" + $env:PATH
$env:CGO_ENABLED = "1"
go test -race ./...
Set-Location ..
```

## Manual Verification

1. Run a deterministic local skill whose model call throws after request
   preparation/sending and acquire the finalized trace in Console.
2. Confirm the explorer shows the complete model/step/root hierarchy and that
   the failure indicator appears on the closest model frame.
3. Navigate frame → failure → frame and verify terminal status, contextual
   summary, attempt/retry/validation links, and absence of speculative root-cause
   language.
4. Confirm no stack text loads until selecting `Load diagnostic`.
5. Load a representative and a truncated stack; verify literal whitespace,
   browser find, wrap/no-wrap, exact clipboard text, UTF-8 text download, and the
   persistent truncation notice.
6. Open a recovered-error/success trace and confirm the failure is nonterminal
   and completion has no terminal link.
7. Attempt analysis of a deliberately invalid artifact carrying
   `rawDownloadAvailable: true`; verify the raw attachment remains available and
   separately confirmation-gated.
8. Inspect the target warning and skill-authoring trace guidance for the explicit
   application-content sensitivity/no-redaction limitation.
9. Inspect regenerated fixtures and installed bundle components to verify no old
   error `terminal` field, no diagnostic range/cursor, no duplicate diagnostic
   component, and no unrelated PR 21 `entrySkill` churn.

## Exit Criteria

- [ ] The minimal Go red test exists, fails pre-fix with
  `INVALID_TERMINAL_FAILURE`, and passes unchanged post-fix.
- [ ] Real Java runtime-produced failed and aborted artifacts are committed and
  accepted by Go with complete closest-frame linkage and the expected missing
  model-response gap.
- [ ] Java tests prove identity/cause deduplication, concurrency isolation,
  closest-frame ordering, pre-frame/recovered/timeout/cleanup behavior, original
  exception propagation, and bounded ordinary stack capture.
- [ ] Go tests prove completion-derived terminality, strict duplicate/link
  rejection, post-reconstruction diagnostic validation, compact descriptors,
  one-copy storage, and bounded whole-diagnostic lifecycle behavior.
- [ ] Browser adapter and React tests prove deliberate loading, bidirectional
  navigation, inert rendering, wrap/copy/download, truncation, unknown kinds,
  stale response handling, keyboard/focus/accessible labels, and raw-download
  fallback.
- [ ] Tests cited by `ai/skill-authoring/traces-and-debugging.md` establish every
  claim in the Authoring Claims and Evidence Map.
- [ ] Exact Java/Go `consoleCompatibilityVersion` rejection, Application API
  allowlist, absence of Supported SPI/replacement beans, and console credential
  non-disclosure remain passing.
- [ ] Obsolete error-record terminal semantics and duplicate-collapse tests are
  removed or rewritten; no simultaneous old/new reader, overload, shim,
  diagnostic range/cursor, speculative provider kind, omission diagnostic, or
  custom search test remains.
- [ ] Corpus regeneration is deterministic on the second run, preserves LF line
  endings and PR 21 `entrySkill`, and contains only intended PR 22 contract
  changes.
- [ ] Full Java, Go, build-tool, and Go race suites pass with no leaked goroutine,
  handle, lease, temporary component, or stale browser state.
- [ ] Manual verification is complete and confirms diagnostic usefulness,
  security warning accuracy, and raw-download independence.
