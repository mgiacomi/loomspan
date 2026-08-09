---
date: 2026-08-08T22:41:55-07:00
researcher: Codex (GPT-5)
git_commit: a0307cfd345ebdc765da2749045628b834a1fb90
branch: main
repository: loomspan
topic: "PR 22 - Make Failed Traces Explorable with Frame-Linked Diagnostics"
tags: [research, codebase, trace, failure, diagnostics, java, go, browser]
status: complete
last_updated: 2026-08-08
last_updated_by: Codex (GPT-5)
---

# Research: PR 22 - Make Failed Traces Explorable with Frame-Linked Diagnostics

**Date**: 2026-08-08T22:41:55-07:00  
**Researcher**: Codex (GPT-5)  
**Git Commit**: `a0307cfd345ebdc765da2749045628b834a1fb90`  
**Branch**: `main`  
**Repository**: `loomspan`

## Research Question

Research the live Loomspan codebase for the implementation-ready ticket
`ai/thoughts/tickets/loomspan-console-pr-22-failed-trace-diagnostics.md`, including the
current Java failure lifecycle and canonical trace producer, Go trace-analysis
validation/index/query pipeline, browser adapter and Trace Explorer consumers,
cross-runtime fixtures and tests, diagnostic-content boundaries, provider/client
attachment seams, and compatibility classification.

## Summary

The live code implements terminal failure identity in two separate places. Java
`TraceCompletion` already requires `terminalFailureId` for `FAILED` and `ABORTED`
and forbids it for `SUCCEEDED`, while Java runtime `ERROR_RECORDED` records carry a
`failureId` but no terminal flag. Go additionally requires the referenced error
fact to contain `metadata.terminal == true`; absence defaults to false. The
Java-owned failed and aborted fixture cases manually add that flag, whereas the
runtime producer does not. This is the exact producer/fixture/consumer split
described by the ticket (`TraceCompletion.java:17-32`,
`DefaultExecutionTraceRecorder.java:140-146`, `failures.go:18-35`,
`ConsoleTraceFixtureCorpusTest.java:470-486`).

Runtime error capture is distributed across catch boundaries. Model and planning
paths retain a throwable only for frame-close metadata, then close their inner
frames. The top-level or nested `ExecutionCoordinator` subsequently creates a new
failure ID and appends `ERROR_RECORDED` against whichever frame is then active.
Tool callbacks and step-validation exhaustion already append error records while
their closest frames are active, but later coordinator boundaries create new
failure IDs for the propagating exception. There is no session-local throwable
identity/cause registry in `LoomspanSession`; its state consists of frame, plan,
usage, trace, observation, and related execution state (`LoomspanSession.java:42-55`,
`ExecutionCoordinator.java:100-145`, `DefaultToolCallbackFactory.java:157-195`,
`StepLoopMissionExecutionEngine.java:444-543`).

Persisted failure payloads contain bounded contextual values chosen by each
caller plus `exceptionType` and a framework-authored safe `message`.
`TraceFailureMetadata` explicitly excludes exception messages and therefore does
not capture causes, suppressed exceptions, or stack traces. No diagnostic DTO,
capture bound, attachment abstraction, or diagnostic kind exists in Java, Go, or
TypeScript today (`TraceFailureMetadata.java:6-28`, `contracts.ts:255-261`).

Go processing streams physical NDJSON, reconstructs chunked payloads into one
payload-store component, builds length-prefixed fact indexes, and publishes a
manifest atomically. `ERROR_RECORDED` indexing currently runs on the physical
envelope before chunk finalization. Consequently a chunked error's `data` is null
at the moment `failureGraph.onErrorRecord` executes; the existing failure index
uses metadata only. The reconstructed JSON is stored once and described by the
payload index, but the failure row does not retain that descriptor
(`processor.go:59-76`, `processor.go:110-120`, `processor.go:162-167`,
`payload.go:13-35`, `processor.go:187-203`).

The analysis service already supplies most lifecycle mechanics needed by a new
bounded diagnostic read: current-target scope, opaque artifact handles, leases,
capacity-accounted components, cancellation, finite ranges, UTF-8 boundary
alignment, and operation/request fingerprints. Its current range identities are
only `PAYLOAD`, `RAW_RECORD`, and `RAW_ARTIFACT`; the cursor operations likewise
distinguish payload/raw-record/raw-artifact ranges. No failure/diagnostic ordinal
range identity exists (`dto.go:219-248`, `query_ranges.go:26-81`,
`cursor.go:29-37`).

The browser adapter exposes shared trace-analysis summaries, frames, records,
failures, payloads, and payload/raw-record ranges. The React explorer is already
hierarchy-first and uses shared selection state for frame, record, and failure
deep links. Failure presentation currently contains IDs and mechanical links but
not exception summaries or diagnostic descriptors/text. Its generic evidence
component renders range content in `<pre>`, while search/wrap/copy/download and
diagnostic-specific truncation presentation do not exist (`router.go:29-47`,
`contracts.ts:222-261`, `TraceExplorer.tsx:347-376`,
`TraceFailureFocus.tsx:9-33`, `TraceEvidenceDetail.tsx:3-6`).

The affected canonical NDJSON and derived analysis representations are classified
by the repository's framework lens as current-release ephemeral diagnostic
formats, not cross-version persisted contracts. Java, Go, TypeScript, fixtures,
and browser behavior are protected in-repository protocol consumers that move in
lockstep behind an exact release-derived `consoleCompatibilityVersion`. The
failure-capture classes and Spring infrastructure beans are internal or
accidentally exposed implementation, not documented Application API or Supported
SPI. The repository contains no `@ConditionalOnMissingBean` replacement seam for
these services (`framework-feature-design-lens.md`,
`LoomspanAutoConfiguration.java:170-182`,
`LoomspanAutoConfigurationBoundaryTest.java:111-123`,
`loomspan-release.properties:1`, `applicationclient/client.go:197-212`).

## Detailed Findings

### 1. Canonical Java terminal-failure contract

#### Completion authority already present

- `TraceCompletion` is an internal record with outcome, terminal usage snapshot,
  nullable `terminalFailureId`, and details. Its compact constructor rejects a
  blank ID, rejects an ID on success, and requires an ID for every non-success
  outcome (`loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/core/TraceCompletion.java:11-35`).
- `metadata()` writes the outcome, usage snapshot, and terminal failure ID to the
  `TRACE_COMPLETED` metadata object (`TraceCompletion.java:38-67`).
- `DefaultExecutionTraceHandle.finalizeTrace` appends that completion as the
  terminal canonical record and adds `errored` and `persistencePolicy`
  (`DefaultExecutionTraceHandle.java:321-359`).
- The Java contract therefore already represents completion-linked terminal
  identity. It does not validate that the ID resolves to exactly one previously
  recorded error fact.

#### Error records as currently produced

- `ExecutionTraceRecorder.recordError` accepts a session, caller-supplied
  `failureId`, and opaque payload (`ExecutionTraceRecorder.java:44`).
- `DefaultExecutionTraceRecorder.recordError` marks the trace errored and records
  only `failureId` in metadata. Because it calls `recordOnActiveFrame`, the
  session's active frame supplies frame identity; if no frame exists the record
  is unframed (`DefaultExecutionTraceRecorder.java:140-168`).
- `DefaultExecutionStateService.logError(session, payload)` creates a fresh UUID
  for every call. Its overload forwards an existing ID without any throwable
  association or deduplication (`DefaultExecutionStateService.java:401-417`).
- `TraceFailureMetadata.addTo` writes exactly `exceptionType` and a nonblank safe
  framework message. Its class documentation states that provider and application
  exception messages are excluded (`TraceFailureMetadata.java:6-28`).
- No Java production write of `ERROR_RECORDED` adds `terminal` metadata. The live
  writes in `ExecutionCoordinator`, `LoomspanSessionRunner`, and
  `LoomspanSession.finalizeTrace` all use only `failureId`
  (`ExecutionCoordinator.java:131-157`, `LoomspanSessionRunner.java:200-231`,
  `LoomspanSession.java:745-766`).

### 2. Current failure propagation and frame ownership

#### Mission/coordinator boundary

- `ExecutionCoordinator.execute` opens the root mission frame before entering the
  mission engine (`ExecutionCoordinator.java:92-104`).
- On any `RuntimeException` or `Error`, it creates a new UUID, appends an error,
  marks the trace errored, and rethrows. At this point inner model, planning,
  step, and tool `finally` blocks have ordinarily closed their frames
  (`ExecutionCoordinator.java:131-145`).
- The same ID is added to the root frame's close metadata and to top-level
  `TraceCompletion`. Nested coordinator invocations record an error and close
  their mission frame but only the top-level invocation finalizes the trace
  (`ExecutionCoordinator.java:165-178`, `ExecutionCoordinator.java:233-244`).
- A cleanup close failure gets a new error only when no execution failure ID is
  already present. Otherwise it is suppressed on the execution failure
  (`ExecutionCoordinator.java:141-164`, `ExecutionCoordinator.java:192-201`).

#### Model and planning frames

- The ordinary mission model opens `MODEL_CALL`, records the throwable in local
  variables, and closes the frame with safe exception metadata; it does not append
  `ERROR_RECORDED` in that catch (`DefaultMissionExecutionEngine.java:145-190`).
- Planning opens a `PLANNING` frame around the planning workflow, and each plan
  request opens a nested `MODEL_CALL`. Both catches retain/rethrow the throwable
  and both finally blocks close their respective frames without recording an
  error fact (`DefaultPlanningService.java:113-142`,
  `DefaultPlanningService.java:383-452`).
- Step-model calls follow the same pattern inside an active `STEP_EXECUTION`
  frame: the model frame is closed on exception, then the step frame is closed on
  propagation (`StepLoopMissionExecutionEngine.java:547-600`,
  `StepLoopMissionExecutionEngine.java:535-543`).
- Timeout and interruption occur in the caller after worker cancellation and
  best-effort frame cleanup. The thrown timeout wrapper is later observed by the
  coordinator (`DefaultMissionExecutionEngine.java:210-236`,
  `StepLoopMissionExecutionEngine.java:273-300`).

#### Tool and validation paths that already record below the root

- `DefaultToolCallbackFactory` catches a tool exception while the tool frame is
  active. It writes `TOOL_CALL_FAILED`, then creates a separate UUID-backed
  `ERROR_RECORDED`, rethrows, and finally closes the tool frame
  (`DefaultToolCallbackFactory.java:157-195`).
- Step invalid-action exhaustion, final-response validation exhaustion, and step
  validation exhaustion call `recordTerminalFailure` before throwing. That helper
  creates an error via `ExecutionStateService.logError`, so the record is attached
  to the active step frame (`StepLoopMissionExecutionEngine.java:450-509`,
  `StepLoopMissionExecutionEngine.java:752-762`).
- These lower records have independent UUIDs. When the same exception reaches a
  coordinator, the coordinator creates another ID and the completion links to the
  coordinator's ID. There is no throwable-identity or cause-chain lookup in these
  paths.

#### Runner and finalization fallback paths

- `LoomspanSessionRunner` stores an uncaught failure, preserves propagation, and
  finalizes in `finally` (`LoomspanSessionRunner.java:114-187`).
- If no coordinator already completed the trace, the runner creates a new
  terminal error. With no open frame it is unframed. If frames remain open, it
  creates an `IllegalStateException`, appends an error against the active frame,
  finalizes failed, and rethrows the cleanup failure
  (`LoomspanSessionRunner.java:190-241`).
- `LoomspanSession.finalizeTrace` handles execution-journal projection failure by
  reusing an existing completion ID when one exists, or creating one otherwise,
  appending an unframed error directly to the trace handle, then changing the
  effective completion to failed (`LoomspanSession.java:718-770`).
- `LoomspanSession` currently has no failure registry field. The class's private
  state includes the frame deque and execution state but no map keyed by
  `Throwable` identity (`LoomspanSession.java:42-55`).

### 3. Current diagnostic content and canonical payload storage

- There is no `diagnostics` field, diagnostic record/type, diagnostic count, or
  per-error aggregate bound in the Java source.
- `Throwable.printStackTrace` is not used by trace recording. Repository searches
  show stack traces only through ordinary logging/testing facilities, not as
  canonical error payloads.
- Error payload shape is caller-owned. Coordinator payloads include skill name,
  objective, exception type, and safe message; tool payloads include tool and
  linked task context; step-validation payloads contain phase and summary text
  (`ExecutionCoordinator.java:247-262`, `DefaultToolCallbackFactory.java:182-190`,
  `StepLoopMissionExecutionEngine.java:752-762`).
- Canonical trace storage converts payloads to Jackson `JsonNode`. Values whose
  serialized Java string length exceeds 4096 are represented by an envelope with
  `payloadId`, `chunkCount`, `payloadChunked`, and `contentType`; the envelope has
  null `data`, followed by `PAYLOAD_CHUNK_APPENDED` records
  (`DefaultExecutionTraceHandle.java:398-455`).
- Chunk splitting is currently by Java string indices and 4096-character
  substrings, not by a UTF-8 byte budget (`DefaultExecutionTraceHandle.java:42`,
  `DefaultExecutionTraceHandle.java:477-505`).
- The logical record is published to the optional observation path only after the
  envelope and all chunks append successfully, so live projection sees the
  reconstructed logical `JsonNode` (`DefaultExecutionTraceHandle.java:437-455`).

### 4. Provider and client evidence seams in the live Java integration

- `OpenAiConnectionChatModelFactory` uses the Spring AI 1.1.6
  `OpenAiApi.builder()` with API key, optional base URL/completions path, and
  headers, then supplies the built API to `OpenAiChatModel.builder()`
  (`OpenAiConnectionChatModelFactory.java:24-45`, root `pom.xml:50`).
- That factory does not configure a custom `RestClient`, response decoder, error
  handler, or response-body interceptor. No alternate OpenAI transport/decoder
  construction seam exists elsewhere in repository production code.
- `ModelAttemptCallAdvisor` records prepared and sent request facts before
  `chain.nextCall(request)`. It records response and usage facts only after that
  call returns a `ChatClientResponse`; it has no catch block and observes no raw
  failure body (`ModelAttemptCallAdvisor.java:39-78`).
- Therefore the in-repository model tracing seam preserves the supported missing
  response fact when `chain.nextCall` throws, but the live code contains no place
  that receives an OpenRouter error-shaped response body before Spring AI's
  conversion failure.

### 5. Go terminal validation and failure indexing

#### Existing interpretation

- `failureGraph.onErrorRecord` ignores an error without `failureId`; otherwise it
  reads `metadata.terminal`, defaulting absence to false. Its indexed fact includes
  failure ID, terminal flag, sequence, timestamp, record type, frame, route,
  attempt, retry, and validation status (`failures.go:18-40`).
- Repeated identical failure IDs are currently collapsed to the first fact and
  accepted if their terminal flags agree; only contradictory flags are rejected
  (`failures.go:41-49`).
- `validateTerminalLink` forbids a completion ID on success and requires failed or
  aborted completion IDs to resolve to an indexed fact whose error-record
  `Terminal` value is true (`failures.go:63-82`).
- The processor invokes this validation after proving a single final completion
  and extracting its outcome (`processor.go:205-239`).

#### Frame and failure relationships

- The frame graph collects `failureId` from framed `ERROR_RECORDED` records into
  each frame's `FailureIDs`; it does not derive failure linkage from exception
  text, adjacency, or close metadata (`frames.go:136-150`).
- `frameResult` and public neutral `FrameSummary` expose these failure IDs beside
  skill, outcome, attempt, retry, and validation relationships
  (`model.go:167-204`, `dto.go:48-79`).
- The failure index contains only `ERROR_RECORDED` facts in live code. Failed tool
  and frame-close records remain queryable as records but are not added to
  `failureGraph` (`processor.go:162-167`).
- The installed failure fact has no exception type, contextual summary, or
  payload descriptor (`model.go:193-204`, `dto.go:149-162`).

### 6. Chunk reconstruction and the failure-payload indexing boundary

- `Processor.Process` opens `payloads.store` first, then streams raw records
  through validator, frame, attempt, failure, and usage calculations while the
  payload assembler writes reconstructed bytes to the component
  (`processor.go:59-87`).
- An envelope is registered, then still flows through semantic calculations. A
  chunk record is appended to the store and returns early from the callback
  (`processor.go:110-120`). Thus an `ERROR_RECORDED` envelope reaches
  `onErrorRecord` with its identifiers and metadata but null `data`.
- `payloadAssembler` retains descriptors keyed by payload ID, validates chunk
  count/order/content type, streams decoded content to the store, and records
  final offsets and lengths only during `finalize` (`payload.go:38-50`,
  `payload.go:65-169`).
- Reconstructed `application/json` is validated as exactly one bounded-depth JSON
  value and reconstructed `text/plain` is validated as UTF-8 without materializing
  the whole payload (`payload.go:235-360`).
- After payload finalization and store close, the processor writes the failure and
  payload indexes as separate components. No current post-reconstruction pass
  joins a failure fact to a payload descriptor (`processor.go:187-203`,
  `processor.go:286-308`).
- The Go parser bounds one physical line at 1 MiB and JSON depth at 128; inline
  payload access is capped at 8 KiB and one range call at 1 MiB
  (`limits.go:9-33`).

### 7. Analysis query and lifecycle ownership

- Every neutral query result carries `targetScopeId`, the opaque artifact handle,
  trace ID, and session ID (`dto.go:8-20`).
- Failure queries are finite and continuable. They canonicalize page size, acquire
  the current-scope lease, validate cursor operation/fingerprint, honor context
  cancellation, read the failure index, and return a scoped page
  (`query_facts.go:363-420`).
- Payload range reads validate the requested maximum, validate a payload-range
  cursor before lease acquisition, acquire the handle lease, resolve the payload
  descriptor, fingerprint payload ID/start/max bytes, validate continuation, and
  seek the shared payload store (`query_ranges.go:20-81`, `range.go:37-88`).
- The neutral `ByteRangeResult` explicitly names source, byte offsets, total
  length, media type, UTF-8 text or base64 encoding, content, and continuation
  (`dto.go:219-258`).
- Existing range source values are `PAYLOAD`, `RAW_RECORD`, and `RAW_ARTIFACT`.
  Existing cursor operations independently bind `PAYLOAD_RANGE`,
  `RAW_RECORD_RANGE`, and `RAW_ARTIFACT_RANGE` (`dto.go:236-248`,
  `cursor.go:29-37`).
- Reconstructed payload bytes are persisted once in `payloads.store`; indexes
  retain offsets/lengths. Small explicit record queries may inline at most 8 KiB,
  while larger payloads remain range-addressed (`payload.go:13-35`,
  `query_records.go:183-200`, `query_records.go:330-360`).

### 8. Browser API and Trace Explorer consumption

#### Adapter boundary

- `browserapi.TraceAnalysisService` is the adapter-facing interface over the
  transport-neutral Go service. It exposes summary, frame, record, attempt, retry,
  validation, failure, payload, gap, uncertainty, usage, search, payload range,
  and raw-record range operations (`browserapi/router.go:29-47`).
- The router registers separate POST routes for those analysis operations,
  including `/failures`, `/payload-range`, and `/raw-record-range`
  (`browserapi/router.go:132-162`).
- `failureDTO` mirrors the current neutral fact and contains no payload or
  diagnostic fields (`browserapi/trace_analysis.go:451-462`).
- Range responses serialize the neutral content and offsets into a shared browser
  shape (`browserapi/trace_analysis.go:303-347`).

#### TypeScript contracts and client

- `TraceFailure` contains terminal status, sequence/timestamp, record/frame/route,
  and attempt/retry/validation links only (`web/src/api/contracts.ts:255-257`).
- `TraceFrame.failureIds` supplies the reverse frame-to-failure link
  (`contracts.ts:233-243`).
- `TraceRange` is shared by payload and raw-record requests. The client fixes each
  browser range request at 65,536 bytes (`contracts.ts:249-261`,
  `api/client.ts:209-225`).
- `BrowserErrorDetails` already carries optional `rawDownloadAvailable`
  (`contracts.ts:53-61`). The Go acquisition and trace-analysis invalidity paths
  populate the corresponding domain detail when raw download remains available
  (`artifact/acquire.go:150-162`, `traceanalysis/errors.go:9-13`).

#### Current failure-focused presentation

- `TraceExplorer` defaults failed/aborted traces to the summary's
  `terminalFailureId`, loads additional failure pages until that ID is found, and
  loads its related frame by `failureId` (`TraceExplorer.tsx:189-268`,
  `TraceExplorer.tsx:325-329`).
- The explorer preserves one browser-owned selection across hierarchy, timeline,
  usage, and records. It already exposes breadcrumbs and related-frame navigation
  (`TraceExplorer.tsx:269-293`, `TraceExplorer.tsx:347-376`).
- `TraceFailureFocus` displays terminal outcome, IDs, record, frame, route,
  skill names, attempts/retries/validation, duration, usage, gaps, and navigation
  buttons. It explicitly says the view relates recorded evidence and does not
  identify root cause (`TraceFailureFocus.tsx:9-33`).
- `TraceHierarchy` renders frame type/route/timing with tree keyboard navigation.
  It does not visibly render `failureIds` (`TraceHierarchy.tsx:4-29`).
- `TraceRecords` lists failures by ID and terminal flag and can navigate to a
  mechanically related frame. Payloads and raw records have explicit read actions
  (`TraceRecords.tsx:4-13`).
- `TraceEvidenceDetail` renders fetched content as inert React text inside
  `<pre>` and supports only next-range and clear controls
  (`TraceEvidenceDetail.tsx:3-6`).
- `TraceDetail` keeps acquisition and raw attachment download as separate actions.
  Acquisition errors currently render only `acquireError.message`; the component
  does not inspect `rawDownloadAvailable` when choosing its guidance
  (`TraceDetail.tsx:102-123`, `TraceDetail.tsx:146-190`).

### 9. Tests and executable fixtures

#### Java runtime tests

- `LoomspanSessionRunnerTest` proves an uncaught failure produces failed
  completion, that an error exists, and that its `failureId` equals the completion
  ID. It asserts the safe exception type/message but not `terminal` metadata or
  stack diagnostics (`LoomspanSessionRunnerTest.java:116-134`).
- `ExecutionCoordinatorTest` similarly asserts aborted completion has a nonblank
  terminal ID and that some error record has the same ID
  (`ExecutionCoordinatorTest.java:1346-1356`).
- Step-loop tests assert that validation exhaustion creates an error containing
  the expected summary. They do not exercise throwable identity reuse across
  catch boundaries (`StepLoopMissionExecutionEngineTest.java:854-886`).
- Trace handle tests cover chunking and trace persistence independently of a
  diagnostic schema (`ExecutionTraceHandleTest.java`).

#### Java-owned cross-language corpus

- `ConsoleTraceFixtureCorpusTest` is the generator and Java semantic reference for
  committed `loomspan-console-fixtures/traces/*.ndjson` and
  `expected/*.json` (`ConsoleTraceFixtureCorpusTest.java:433-456`).
- Valid `terminal-failure`, `terminal-abort`, and `validation-exhaustion` cases
  manually write `terminal: true`. `nonterminal-error-then-success` manually
  writes `terminal: false` (`ConsoleTraceFixtureCorpusTest.java:470-486`,
  `ConsoleTraceFixtureCorpusTest.java:507-540`).
- The invalid corpus includes an unresolved completion link but does not currently
  include missing/duplicate/contradictory terminal-link variants or malformed
  diagnostics (`ConsoleTraceFixtureCorpusTest.java:1137-1140`).
- The corpus contains hand-constructed valid traces. It does not run a failing
  `LoomspanSessionRunner` artifact through the Go processor.

#### Go and browser tests

- Go calculation tests require a failed completion to resolve to an error fact
  carrying terminal true and reject success with a terminal ID
  (`calculations_test.go:361-400`).
- Go tests separately index recovered and terminal errors using explicit false and
  true flags (`calculations_test.go:746-760`).
- Service tests prove framed errors populate both the failure index and frame
  reverse links (`service_test.go:260-299`).
- `fixture_corpus_test.go` reads every Java-owned fixture, processes it, and
  compares the manifest/index result to the committed expected result
  (`fixture_corpus_test.go:38-57`, `fixture_corpus_test.go:152-170`,
  `fixture_corpus_test.go:302-315`).
- Browser tests cover deep loading of a paginated terminal failure and its frame
  linkage, using current `TraceFailure` objects without diagnostic descriptors
  (`TraceExplorer.test.tsx:207-211`).
- Trace-detail tests cover distinct acquire and raw-download actions but not
  raw-download guidance derived from an acquisition invalidity
  (`TraceDetail.test.tsx:162-186`).

### 10. Configuration, manifests, documentation, and observable security policy

- No `loomspan.*` configuration property or YAML skill-manifest field currently
  governs failure diagnostics, capture limits, stack traces, or diagnostic
  sensitivity. The ticket therefore touches no existing configuration/manifest
  contract in live code.
- `ExecutionTraceProperties` controls trace location and persistence behavior, not
  error payload shape (`autoconfigure/ExecutionTraceProperties.java`).
- The browser currently warns when target transport is unencrypted, stating that
  the application key and observability data travel without encryption
  (`web/src/target/Overview.tsx:1`).
- Historical console policy states that console authentication secrets are never
  returned, while Loomspan does not detect or redact secrets embedded by the
  observed application in trace records, payloads, tools, errors, or metadata
  (`ai/thoughts/phases/loomspan_console_phase_1_observability_foundation.md:372`,
  `ai/thoughts/phases/loomspan_console_phase_1_observability_foundation.md:487-495`).
- Live repository documentation/UI does not currently enumerate exception
  messages or stack-trace blobs as recorded application diagnostic content.

## Contract and Compatibility Classification

### Application API

- No ticket-targeted failure recorder, diagnostic attachment, Go query, or browser
  DTO is in `com.lokiscale.loomspan.api` or documented as an ordinary skill
  developer entry point.
- `LoomspanSessionRunner` is public Java and is injected into the framework's
  `SkillTemplate`, but the architecture allowlist classifies it as public only for
  internal cross-package collaboration (`LoomspanPublicSurfaceArchitectureTest.java:82-83`,
  `LoomspanAutoConfiguration.java:286-299`).
- Evidence of a deliberately supported Application API for direct failure capture
  was not found.

### Supported SPI

- `ExecutionTraceRecorder` and `ExecutionStateService` are public interfaces in
  internal packages. `DefaultExecutionTraceRecorder`, `TraceFailureMetadata`, and
  related types are public for internal subsystem collaboration according to the
  architecture allowlist (`LoomspanPublicSurfaceArchitectureTest.java:92-126`).
- Spring creates `LoomspanSessionRunner` and `ExecutionStateService` as
  `ROLE_INFRASTRUCTURE` beans (`LoomspanAutoConfiguration.java:170-182`,
  `LoomspanAutoConfiguration.java:334-339`).
- The auto-configuration architecture test asserts there are no production
  `@ConditionalOnMissingBean` replacement seams and explicitly classifies these
  factories/types as framework-owned (`LoomspanAutoConfigurationBoundaryTest.java:28-40`,
  `LoomspanAutoConfigurationBoundaryTest.java:111-123`).
- No documentation or allowlist evidence establishes these failure-capture
  interfaces/constructors as a Supported SPI.

### Configuration and manifest contracts

- No existing property, YAML field, default, or author-facing validation rule
  describes diagnostics. Trace persistence and observability security settings
  remain separate existing configuration contracts.
- The ticket's fixed diagnostic limits therefore map to an internal canonical
  trace rule in the current repository state, not to an existing configurable
  contract.

### Persisted or serialized contracts

- The application observability REST/SSE DTOs and acquisition metadata are
  serialized cross-language boundaries for the exact current release.
- Go derived bundle components and browser JSON DTOs are serialized inside the
  console process/artifact lifecycle, but the project does not treat them as
  cross-release durable formats.
- The independently observable compatibility marker comes from filtered project
  version metadata and Go rejects a different value exactly
  (`META-INF/loomspan-release.properties:1`, `applicationclient/client.go:197-212`).

### Ephemeral diagnostic formats

- Canonical NDJSON traces, `ERROR_RECORDED`, chunk envelopes, payload store/index,
  failure index, analysis manifest, neutral query DTOs, browser DTOs, and committed
  executable fixture corpus are the ticket's central affected surfaces.
- The framework lens classifies execution traces as current-run debugging formats
  whose writer, reader, projector, fixtures, and debugging consumers remain
  coherent, without requiring historical readers
  (`ai/thoughts/framework-feature-design-lens.md`, “Current Pre-1.0 Compatibility Posture”).
- Protected in-repository consumers are Java trace/journal readers and live
  projection, Java fixture generation/reference calculation, Go acquisition and
  analysis, Go bundle queries, browser API mapping, TypeScript client contracts,
  Trace Explorer, and future MCP consumers of the shared Go service.

### Internal or accidentally exposed implementation

- Session failure state, throwable association, stack capture, attachment
  builders, Java trace recorder/state service, Go processor graphs/index rows,
  cursor operations, and React component state are internal implementation.
- Public Java modifiers, public constructors, interfaces, infrastructure beans,
  and tests are technical exposure/evidence of existing behavior; repository
  architecture tests explicitly distinguish them from replacement SPIs.

### Compatibility marker and fixture coordination

- Java/Go compatibility is exact string equality on the release-derived
  `consoleCompatibilityVersion`; there is no schema-specific compatibility number
  in the trace itself.
- The console `AGENTS.md` states Java, Go, TypeScript, fixtures, and semantic tests
  move atomically and that the in-memory trace catalog starts empty after restart.
- The current repository therefore has one current-release reader/writer set and
  no legacy failure reader, trace migration, or dual terminality path.

## Code References

- `loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/core/TraceFailureMetadata.java:6-28` — current safe failure payload fields and explicit message exclusion.
- `loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/core/DefaultExecutionTraceRecorder.java:140-168` — active-frame error recording with only `failureId` metadata.
- `loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/core/TraceCompletion.java:17-67` — Java completion-linked terminal failure invariants and serialization.
- `loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/core/ExecutionCoordinator.java:100-178` — root-boundary error creation, frame close, and top-level finalization.
- `loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/core/LoomspanSessionRunner.java:190-241` — standalone/unframed and open-frame fallback finalization.
- `loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/core/LoomspanSession.java:718-770` — journal-projection failure path.
- `loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/runtime/planning/DefaultPlanningService.java:383-452` — planning-model failure frame lifecycle.
- `loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/runtime/step/StepLoopMissionExecutionEngine.java:390-600` — step and step-model frame lifecycle.
- `loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/runtime/tool/DefaultToolCallbackFactory.java:157-195` — closest-frame tool error recording.
- `loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/runtime/trace/DefaultExecutionTraceHandle.java:398-505` — canonical envelope/chunk storage and logical publication.
- `loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/autoconfigure/OpenAiConnectionChatModelFactory.java:24-45` — current Spring AI OpenAI construction seam.
- `loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/chat/ModelAttemptCallAdvisor.java:39-78` — request/sent/response trace lifecycle around `chain.nextCall`.
- `loomspan-console/internal/traceanalysis/failures.go:18-82` — error terminal flag parsing and completion-link validation.
- `loomspan-console/internal/traceanalysis/processor.go:59-76` — payload store and analysis graph construction.
- `loomspan-console/internal/traceanalysis/processor.go:110-167` — envelope/chunk flow and pre-reconstruction failure indexing.
- `loomspan-console/internal/traceanalysis/processor.go:187-239` — payload finalization followed by completion and terminal-link validation.
- `loomspan-console/internal/traceanalysis/payload.go:13-169` — payload descriptors, streamed assembly, and store offsets.
- `loomspan-console/internal/traceanalysis/dto.go:149-162` — current neutral failure summary.
- `loomspan-console/internal/traceanalysis/dto.go:219-258` — shared byte-range model and current source identities.
- `loomspan-console/internal/traceanalysis/query_facts.go:363-420` — scoped, leased, continuable failure query.
- `loomspan-console/internal/traceanalysis/query_ranges.go:20-81` — payload-range target binding and continuation fingerprint.
- `loomspan-console/internal/browserapi/router.go:29-47` — browser adapter's shared trace-analysis service boundary.
- `loomspan-console/internal/browserapi/trace_analysis.go:451-462` — current browser failure DTO.
- `loomspan-console/web/src/api/contracts.ts:222-261` — browser trace-analysis contracts.
- `loomspan-console/web/src/observability/TraceExplorer.tsx:189-376` — failure deep loading, selection, and hierarchy-first views.
- `loomspan-console/web/src/observability/TraceFailureFocus.tsx:9-33` — current terminal failure evidence panel.
- `loomspan-console/web/src/observability/TraceDetail.tsx:146-190` — distinct acquisition/raw download actions and current error presentation.
- `loomspan-spring-boot-starter/src/test/java/com/lokiscale/loomspan/internal/runtime/trace/ConsoleTraceFixtureCorpusTest.java:433-629` — Java-owned valid fixture generation.
- `loomspan-console/internal/traceanalysis/fixture_corpus_test.go:38-57` — Go expected fixture shape.

## Architecture Documentation

```text
Java execution catch boundary
  -> ExecutionStateService / ExecutionTraceRecorder
  -> LoomspanSession + DefaultExecutionTraceHandle
  -> canonical NDJSON (logical data or envelope + chunks)
  -> authenticated application raw-artifact stream
  -> Go central artifact acquisition
  -> Processor streaming validation + payload reconstruction
  -> immutable bundle components
       record index
       frame index
       failure index
       payload index + single payload store
       usage/gap/uncertainty indexes
       manifest
  -> transport-neutral TraceAnalysis Service (scope + handle + lease + cursor)
  -> browser API adapter
  -> TypeScript client
  -> hierarchy-first Trace Explorer
```

Canonical records are authored only in Java. The application adapter streams the
unchanged finalized artifact; it does not reinterpret error payloads. Go owns
current-release semantic validation and derived indexes. Browser and future MCP
adapters consume Go's shared service rather than parsing NDJSON or stack traces.
Large logical content is stored in the bundle payload store and exposed through
explicit bounded requests. Raw attachment download remains a separate upstream
pass-through from local acquisition and analysis.

## Historical Context (from `ai/thoughts/`)

- `ai/thoughts/framework-feature-design-lens.md` — classifies execution traces as
  current-run ephemeral diagnostics and requires coordinated analysis of public
  exposure, supported contracts, fixtures, consumers, and compatibility markers.
- `ai/thoughts/phases/loomspan_console_phase_1_observability_foundation.md:363-372`
  — established append integrity, logical-payload projection, failure isolation,
  bounded resources, and the console-secret/application-content disclosure rule.
- `ai/thoughts/phases/loomspan_console_phase_2_ui_console.md:691-776` — established
  hierarchy-first inspection, one shared Go analysis service, progressive raw
  detail, finite ranges, and mechanical failure navigation.
- `ai/thoughts/phases/loomspan_console_workflows.md:101-135` — established the
  failed-execution workflow: acquire deliberately, open a failure-focused view,
  and navigate recorded evidence without presenting speculative causality.
- `ai/thoughts/phases/2026-07-23-loomspan-console-implementation-roadmap.md:101-129`
  — places the canonical trace, centralized artifact service, analysis service,
  explorer, and future MCP adapter in their dependency sequence.
- `ai/thoughts/tickets/loomspan-console-pr-22-failed-trace-diagnostics.md` — records
  the resolved current ticket contract, bounds, implementation map, fixture set,
  tests, and guardrails used as the research prompt.

## Related Research

No earlier document exists under `ai/thoughts/research/` in the current checkout.
The directly related historical design sources are listed above.

## Open Questions

- The repository's current `OpenAiApi` construction does not expose a raw
  error-shaped provider response. Whether Spring AI 1.1.6 offers an additional
  supported builder-level error-body hook not used by this repository is not
  established by in-repository code; external framework API research was not part
  of this codebase-only investigation.
- The ticket follows PR 21, but the current checkout is `main` at
  `a0307cfd345ebdc765da2749045628b834a1fb90`. The ticket says PR 21 changes
  application trace identity while preserving canonical NDJSON. This research
  records the current checkout and does not establish whether all PR 21 changes
  are already present beyond the observed `entrySkill` and exact compatibility
  contracts.
- Historical Phase 2 text describes a broader flat failure-reference list that
  includes failed tool and frame-close records, while live Go code indexes only
  `ERROR_RECORDED`. The ticket's resolved design and implementation map focus on
  canonical `ERROR_RECORDED` failure facts; the current live behavior is the
  latter.

