# PR 28 — MCP Trace Navigation and Semantic Evidence Implementation Plan

## Overview

Replace the unreleased Console trace-inspection contract in place so an MCP client can reliably select finalized evidence, orient through a compact frame projection, find plan/model/tool/validation values as neutral semantic-content descriptors, and read only the selected value. The implementation remains read-only, stateless, trace-ID based, mechanically factual, bounded, and independent of the optional runtime-debugging Agent Skill.

This is one atomic pre-v1 contract correction across the Java trace producer, the Go artifact/inventory/analysis services, browser and MCP adapters, the bundled web client, shared fixtures, documentation, and deterministic evaluations. It does not preserve the payload-only names or current response shapes.

## Current State Analysis

- Unified discovery takes only page size and continuation, exposes only `finalizedAt`, sorts by finalization time, and fingerprints page size plus target scope. The artifact store already records acquisition time, but inventory drops it (`loomspan-console/internal/traceinventory/dto.go:5-35`, `loomspan-console/internal/traceinventory/service.go:309-360`, `loomspan-console/internal/artifact/model.go:41-50`).
- Imported processing validates trace/session identity, outcome, finalization time, policy, and compatibility, but cannot derive `entrySkill` because current `TRACE_STARTED` records contain only `sessionId` plus trace metadata (`loomspan-console/internal/traceanalysis/processor.go:277-303`, `loomspan-console/internal/traceanalysis/processor.go:440-447`, `loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/runtime/trace/DefaultExecutionTraceHandle.java:304-313`).
- Frame queries always materialize the rich `FrameSummary`; the query has no projection selector, so even hierarchy orientation calculates and serializes usage, attempts, retries, validations, failures, gaps, and uncertainties (`loomspan-console/internal/traceanalysis/dto.go:49-81`, `loomspan-console/internal/traceanalysis/query_frames.go:43-97`).
- Record `metadata` and `data` are retained as raw JSON, but the semantic reference/read path covers only reconstructed payloads and failure diagnostics. Logical record queries still label ordinary-data records as physical, and `inlinePayload` applies only an 8 KiB per-payload limit with no aggregate page budget (`loomspan-console/internal/traceanalysis/model.go:23-68`, `loomspan-console/internal/traceanalysis/content_ref.go:13-101`, `loomspan-console/internal/traceanalysis/query_records.go:174-226`).
- The Java producer already writes framework `planId` and accepting attempt/retry identity, but Go does not project typed plan facts and the shared Java/Go corpus has no current-producer plan chain (`loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/runtime/planning/DefaultPlanningService.java:667-688`, `loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/core/DefaultExecutionTraceRecorder.java:73-100`, `loomspan-console/internal/traceanalysis/record_facts.go:132-186`).
- MCP literal search is a record filter over case-sensitive encoded metadata/data and returns rich record DTOs. The browser-only search service separately scans physical records and reconstructed payloads with bounded-work continuations, but neither surface reports a complete search coverage contract (`loomspan-console/internal/traceanalysis/query_records.go:245-304`, `loomspan-console/internal/traceanalysis/search.go:91-274`).
- Successful MCP results duplicate the structured result by JSON-marshalling it into the text arm. There is no committed `tools/list` byte measurement or budget (`loomspan-console/internal/mcpadapter/contracts.go:145-148`, `loomspan-console/internal/mcpadapter/traces.go:354-358`).

## Desired End State

The normal evidence path is:

```text
LOOMSPAN_list_traces (bounded filters and explicit time ordering)
  -> LOOMSPAN_get_trace
  -> LOOMSPAN_query_trace_frames (COMPACT by default)
  -> LOOMSPAN_query_trace_records (descriptors or compact literal matches)
  -> LOOMSPAN_read_trace_content (only selected logical bytes)
```

The same six-tool trace surface remains: `LOOMSPAN_read_trace_content` destructively replaces `LOOMSPAN_read_trace_payload`; no search, plan, model, tool, hierarchy, or diagnosis tool is added. Browser and MCP remain peer adapters over the same neutral services.

The finalized contract uses these decisions:

- Inventory candidates carry three independent classes of fact:
  - `evidenceSources` is a closed, deterministically ordered set containing `TARGET`, `IMPORTED`, or both for a collision, serialized in fixed `TARGET`, then `IMPORTED` order. It describes the path by which evidence reached Console, not artifact authenticity, deployment provenance, or an internal owner/scope.
  - Nullable `acquiredAt` is present only after a target artifact has been successfully fetched and installed from the configured framework application. Nullable `importedAt` is present only after an import has been successfully validated and installed. A target catalog candidate not yet acquired has neither timestamp; failed/cancelled/rejected operations publish no timestamp.
  - `finalizedAt` remains the producer-owned execution-end time and is never rewritten during acquisition or import.
- Query order is a closed enum. `FINALIZED_DESC` remains the default. `ACQUIRED_DESC` and `IMPORTED_DESC` put candidates with the selected timestamp first; missing values sort afterward. Source-time ties use `finalizedAt` descending, then `traceId` ascending; finalization ties use `traceId` ascending. Ordering never silently filters candidates.
- Filters include non-empty any-of `sources`, `outcomes`, exact `entrySkill`, exact `sessionId`, and independent `finalizedFrom`/`finalizedTo`, `acquiredFrom`/`acquiredTo`, and `importedFrom`/`importedTo` windows. A source-time filter requires that timestamp, excluding source-exclusive candidates without it while still allowing a collision's applicable source instance to match. All active filters must match the same underlying evidence instance; target and imported fields are never combined to manufacture a match. Every filter, order, page size, work position, and relevant installed-evidence fact participates in continuation fingerprinting.
- A source collision remains one ambiguous candidate. It reports both sources and the applicable source timestamps, but filtering or ordering never chooses an owner. Shared execution fields such as `finalizedAt`, outcome, session, and entry skill are emitted only when the source instances agree; disagreement makes that shared field unavailable with the ambiguity limitation.
- Inventory groups by trace ID, evaluates filters per evidence instance, and emits one candidate if any instance matches while retaining every collision source. If multiple instances match, the greatest applicable timestamp supplies the internal ordering key; that key is never exposed as canonical shared metadata.
- Current Java producers record exact nonblank `entrySkill` in `TRACE_STARTED.metadata.entrySkill`. Imported processing publishes it only after complete validation. Missing/invalid entry skill stays unknown and adds an `IMPORTED_ENTRY_SKILL_UNAVAILABLE` limitation; filenames, model content, target state, and UI state are never fallback sources.
- Frame projection is `COMPACT` by default and `DETAILED` explicitly. Compact items contain `frameId`, `parentFrameId`, frame type, route, open/close timestamps, outcomes, and counts indicating direct attempts, retries, validations, failures, gaps, and uncertainties. Detailed retains the existing full calculations and identifiers. Projection is cursor-bound.
- Every logical record with an explicit `data` member or reconstructed envelope has one `content` descriptor with `role`, `contentType`, `encoding`, `retainedBytes`, `available`, `complete`, `inlineEligibility`, and opaque `contentRef` when readable. Explicit JSON `null` is a present `application/json` value; absent `data` has no descriptor. Envelope roots reuse one descriptor/reference for a shared reconstructed value rather than duplicating bytes.
- Ordinary `data` content is addressed by immutable record sequence and re-decoded from the bounded physical record on each read; it is not duplicated into a second derived store. The returned logical bytes are the preserved JSON value bytes (`application/json`, UTF-8). Reconstructed envelope content and diagnostics continue to use their existing stores behind the generalized reference. The reference kind becomes internal `SEMANTIC_CONTENT`, with a source discriminator that remains opaque to callers.
- `LOOMSPAN_read_trace_content` accepts `contentRef`, `start`, and `maxBytes`. It returns actual range, total logical length, content type, transport encoding, selected-value `complete`/`hasMore`, and a continuation bound to that one value. Raw-artifact `hasMore` continues to mean bytes remain in the artifact and is named/documented as such.
- Record queries default to descriptors only. `inlineContent` is an explicit Boolean. Qualifying complete values no larger than 8 KiB are inlined in record order until a 32 KiB aggregate source-byte budget is exhausted. Later descriptors remain visible with omission reason `PER_VALUE_LIMIT`, `AGGREGATE_LIMIT`, `UNAVAILABLE`, or `INCOMPLETE`; no value is partially inlined. Both fixed limits live in `traceanalysis/limits.go` and `inlineContent` is cursor-bound.
- Plan landmarks expose producer-owned `planId`, record sequence as the ordering/version fact, owning root frame, creation planning frame, and accepting `attemptId`/`retrySequenceId` only where recorded on current `PLAN_CREATED`. No model-authored ID, success, version number, or missing lineage is inferred.
- The closed validation-status vocabulary is `passed`, `retrying`, and `exhausted`, matching all current advisor producers. Record types, frame types, outcomes, validation statuses, frame orders/projections, representations, and inventory orders are declared once in Go, validated at the service boundary, and emitted as MCP schema enums.
- Literal search is the search mode of `LOOMSPAN_query_trace_records`, preserving the six-tool surface. With `filter.literalText`, the service uses the bounded search cursor and returns compact match descriptors instead of rich records. It performs exact case-sensitive byte matching over record metadata JSON and every available logical semantic value (ordinary `data` and reconstructed envelopes once per descriptor); binary and unavailable/incomplete content is excluded with an explicit coverage limitation. The envelope reports query, case sensitivity, `LOGICAL` representation, searched fields, semantic-content coverage, work completion, limitations, result `hasMore`, and continuation. A zero-match page with unfinished work cannot establish absence.
- Navigation structured responses retain all facts, while their line-oriented MCP text fallbacks stay at or below 64 KiB for maximum MCP page sizes and include identity, counts, completeness, continuation, limitations, and compact item/match lines. A content-read fallback includes the selected bounded content once plus at most 4 KiB of navigation metadata; it does not repeat structured JSON.
- A 30-frame compact structured response is at most 64 KiB, a descriptor-only 64-record response is at most 128 KiB on the representative fixture, and serialized `tools/list` is at most 64 KiB. Tests record exact observed bytes and fail above these reviewed ceilings. Source-byte budgets, not token estimates, are the server contract.

### Key Discoveries

- The artifact store already carries an acquisition clock fact, but currently uses the same internal name for target and imported entries and inventory omits it. PR 28 must capture the immutable instant at successful publication and project it as `acquiredAt` or `importedAt` according to evidence owner kind (`loomspan-console/internal/artifact/service.go:359-408`, `loomspan-console/internal/traceinventory/service.go:302-306`).
- The current payload/diagnostic reader already has correct selected-value continuation behavior; generalization should preserve its owner/evidence/content binding instead of creating another reader model (`loomspan-console/internal/traceanalysis/query_ranges.go:12-163`).
- A representative plan chain totals about 19 KiB while every member is below 8 KiB, which is why the 32 KiB aggregate limit supports the normal plan walkthrough but still bounds larger model-exchange pages (`ai/thoughts/phases/loomspan_llm_trace_understanding_workflows.md:757-793`).
- Browser fixtures are byte-for-byte contracts and MCP fixtures assert semantic parity, so DTO replacement must update both adapters and their committed evidence atomically (`loomspan-console/internal/browserapi/contracts_test.go:138-169`, `loomspan-console/internal/mcpadapter/trace_semantic_fixtures_test.go:93-675`).

## What We're NOT Doing

- No specialized plan, model, tool, advisor, hierarchy, usage, retry, failure, comparison, or diagnosis tool.
- No mutable current/selected trace, client-shared selection state, or caller-visible artifact owner/handle/scope/path.
- No fuzzy, semantic/vector, case-insensitive, unbounded, or cross-trace search.
- No model-authored plan-ID compatibility, legacy trace inference, fallback reader, alias, deprecated payload field/tool, overload, or dual behavior.
- No durable Console catalog, imported metadata backfill, restart adoption, cross-version trace migration, or historical analytics.
- No MCP resources, prompts, write operations, sampling, elicitation, live-tail transition, or arbitrary client filesystem paths.
- No unrelated web redesign; browser work is limited to shared contract correctness and existing trace views.

## Skill-Authoring Documentation Impact

**Impact**: Affected

- **Rationale**: Skill authors use trace guidance to debug plans, attempts, tools, model output, and evidence. The author-facing workflow changes from payload-only/raw fallback to inventory filtering, compact orientation, typed plan lineage, descriptor-first semantic content, exact content reads, and coverage-aware negative search.
- **Documents to update**: `ai/skill-authoring/traces-and-debugging.md`, `ai/skill-authoring/README.md`, `loomspan-console/agent-skills/loomspan-runtime-debugging/SKILL.md`, and `loomspan-console/agent-skills/loomspan-runtime-debugging/references/mcp-tool-guide.md`.
- **Supporting evidence**: the current-producer plan fixture in `loomspan-console-fixtures/`, Go `fixture_corpus_test.go`, MCP semantic fixtures and schema tests, content/range/search tests, capability manifest tests, and PR-28 agent-eval cases.
- **Coverage table update**: Required. Keep “Traces and debugging” source-verified, but update its note to name source-aware finalized/acquired/imported discovery, compact/detailed projections, typed plan/attempt lineage, generalized `contentRef`, descriptor-first limits, and coverage-aware search. Routing does not change because the existing trace topic remains the correct entry point.
- **LLM-first usability**: Keep one routed trace topic and one packaged MCP guide. Use exact field/tool names, a compact decision table for discovery/orientation/query/read/search, explicit negative-result stopping rules, and stable implementation/test anchors. Do not copy full JSON schemas or repeat narrative history.

## Contract and Compatibility Impact

| Surface | Classification and supporting evidence | Planned compatibility treatment |
| --- | --- | --- |
| Application API | The eight allowlisted `com.lokiscale.loomspan.api` types are unaffected (`LoomspanPublicSurfaceArchitectureTest.java:27-37`). | Preserve exactly; run the architecture test. |
| Supported SPI | Loomspan exposes no supported SPI and this work adds none (`LoomspanPublicSurfaceArchitectureTest.java:316-321`). | No change. |
| Configuration and manifest contracts | No YAML or `loomspan.*` key changes. Packaged debugging-skill frontmatter/capabilities and documented MCP workflow do change. | Update skill guidance, capability manifest, README, and drift tests atomically. |
| Persisted or serialized contracts | No new durable/cross-version format. Complete artifacts remain portable only for exact `consoleCompatibilityVersion`; development builds have no compatibility promise. | Do not add migration/backfill. Keep the compatibility marker value unchanged for this unreleased atomic update, regenerate current fixtures, and remove obsolete fixture shapes. |
| Ephemeral diagnostic formats | `TRACE_STARTED.entrySkill`, typed plan facts, generalized content descriptors/references, search coverage, and frame/record projections change the current writer-reader-projector contract. | Change Java producer, Go reader/indexes, fixtures, adapters, docs, and tests together; preserve ordering, completeness, redaction/sensitivity, and owner binding. |
| Internal or accidentally exposed implementation | Go DTOs/services/browser routes, MCP pre-v1 schemas, web contracts, derived indexes, cursors, and internal Java trace types change. | Replace atomically; delete payload-only names and old DTO behavior rather than preserving bridges. |

- **Evidence of supported contracts**: Java allowlist/architecture tests protect the application API; the approved ticket protects the intended pre-v1 replacement; Console README, packaged skill, capability manifest, browser fixtures, MCP semantic fixtures, and Java/Go corpus identify in-repository consumers.
- **Intended breaks**: `LOOMSPAN_read_trace_payload` becomes `LOOMSPAN_read_trace_content`; `payloadRef`/`inlinePayload` become `contentRef`/`inlineContent`; frame default/output and record/search result shapes change; inventory gains evidence-source, finalized/acquired/imported time, filtering, and ordering facts; text fallbacks cease mirroring structured JSON.
- **In-repository consumers to update**: Go services/adapters/tests, web client/contracts/components/tests, Java producer/tests, shared traces/expected files, browser fixtures, MCP capability fixture, README, authoring guidance, packaged Agent Skill, and deterministic eval cases.
- **Public-surface delta**: None in supported Java API; no Java SPI, public signature, or Spring replacement point is added or removed.
- **Shim decision**: **No shim.** The ticket explicitly approves an in-place unreleased contract replacement, there is no protected released MCP/Console consumer, and the repository policy prefers one coherent contract.
- **Java-to-Go boundary coordination**: **Required.** `TRACE_STARTED.metadata.entrySkill` and the current plan-chain fixture change consumed NDJSON. Update `DefaultExecutionTraceHandle`, Java trace contract/corpus tests, committed trace/expected fixtures, Go parser/processor/corpus tests, compatibility documentation, and all projections in the same change. Application REST/SSE query parameters do not change: inventory filtering remains in the Go unified inventory over its installed snapshot plus the bounded target catalog pages it already fetches.

## Implementation Approach

Build from executable contracts inward. First lock the new producer evidence and shared fixture. Then update inventory and trace-analysis neutral services, including cursor fingerprints and bounds. Only after those services are stable should browser/web and MCP projections replace their pre-v1 shapes. Finish with concise fallbacks, discovery-size guards, documentation, evaluations, and the live acceptance walkthrough.

Filtering remains in the unified Go inventory rather than extending the protected Java trace-list REST API. Each call processes only a bounded target-catalog page plus the bounded installed snapshot; the opaque continuation carries both result position and remaining catalog work. Empty result pages may therefore have `hasMore=true` while filtering work remains, and only exhaustion of the requested domain permits `complete=true`. Consolidation, collision marking, filtering, and selected ordering happen before an item is emitted. This keeps one coherent filter contract without an unbounded internal scan or a Java application-protocol change.

## Phase 1: Establish Current-Producer Identity Evidence

### Overview

Make current trace artifacts self-describing for imported entry-skill discovery and add a sanitized plan-chain fixture that exercises framework plan identity, accepting attempt lineage, content representations, and aggregate inline pressure.

### Changes Required

#### 1. Java trace producer and contract tests

**Files**: `loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/runtime/trace/DefaultExecutionTraceHandle.java`, `loomspan-spring-boot-starter/src/test/java/com/lokiscale/loomspan/internal/runtime/trace/ExecutionTraceHandleTest.java`, `ExecutionTraceContractTest.java`

**Changes**:

- Add exact `entrySkill` to `TRACE_STARTED.metadata`; keep session identity in `data` and do not duplicate target/UI provenance.
- Assert nonblank recorded identity and existing framework-owned `planId`, creation `attemptId`, and `retrySequenceId` behavior.
- Assert rejected proposals never become plan landmarks and updates retain the creation plan ID.

#### 2. Shared Java/Go semantic corpus

**Files**: `loomspan-spring-boot-starter/src/test/java/com/lokiscale/loomspan/internal/runtime/trace/ConsoleTraceFixtureCorpusTest.java`, `loomspan-console-fixtures/traces/`, `loomspan-console-fixtures/expected/`, `loomspan-console-fixtures/README.md`, `loomspan-console/internal/traceanalysis/fixture_corpus_test.go`

**Changes**:

- Generate a deterministic current-producer fixture containing primary and nested same-skill plan chains, rejected then accepted attempts, plan updates, ordinary and envelope model values, tool input/output, advisor mutation, validation, structured output, explicit JSON null, and enough small plan versions to cross the aggregate-inline boundary.
- Regenerate affected valid fixtures with `entrySkill`; add invalid/missing-entry-skill cases whose import remains usable but reports unknown entry skill.
- Record expected plan IDs, sequences, root/planning frames, accepting lineage, and semantic-content facts without copying sensitive live trace text.

### Success Criteria

#### Automated Verification

- [x] Producer contract tests pass: `.\mvnw.cmd -pl loomspan-spring-boot-starter test -Dtest=ExecutionTraceHandleTest,ExecutionTraceContractTest,ConsoleTraceFixtureCorpusTest -DfailIfNoTests=false`
- [x] The Go corpus consumes the regenerated current fixture: `Set-Location loomspan-console; go test ./internal/traceanalysis -run FixtureCorpus`
- [x] Supported Java surface remains closed: `.\mvnw.cmd -pl loomspan-spring-boot-starter test -Dtest=LoomspanPublicSurfaceArchitectureTest -DfailIfNoTests=false`

#### Manual Verification

- [x] Inspect the fixture once to confirm primary/nested topology cannot be solved by route, skill name, first match, or model-authored candidate ID.
- [x] Confirm fixture content is synthetic, deterministic, and free of live customer/application data.

---

## Phase 2: Implement Filtered, Time-Explicit Inventory

### Overview

Make finalized-trace selection reliable for “recently finalized,” “just acquired from the framework,” and “just imported into Console” workflows while retaining collision safety and honest completeness.

### Changes Required

#### 1. Inventory query, entry, order, and cursor contracts

**Files**: `loomspan-console/internal/traceinventory/dto.go`, `service.go`, `cursor.go`, `service_test.go`

**Changes**:

- Add output `evidenceSources`, input `sources`, the three decided orders, the three independent time-window families, exact identity/outcome filters, and closed boundary validation. `sources` is non-empty any-of and deduplicated canonically; use RFC 3339 instants with inclusive lower and upper bounds, rejecting an inverted range.
- Capture the internal installed timestamp only when processing has completed and the artifact becomes usable. Project it as `acquiredAt` for target-owned entries and `importedAt` for imported entries. Reuse of the same installed entry preserves it; removal/expiry followed by successful reacquisition or reimport creates a new value.
- Mark catalog-only candidates with source `TARGET` and no `acquiredAt`. Never fabricate `importedAt`, translate finalization time into an availability time, or use request/upload start time for successful publication.
- Group by trace ID and mark ambiguity before pagination. Evaluate every active filter against one evidence instance; emit one grouped candidate if any instance matches, retain all collision sources, and never combine facts across instances. Apply the selected missing-last order to matching instances, using their greatest applicable timestamp, then the documented tie-breakers.
- For a collision, retain both source kinds and source timestamps. Publish shared session/entry-skill/outcome/finalization fields only if the evidence instances agree; otherwise leave the field unknown under the ambiguity limitation. A source filter may find the candidate but never makes it inspectable or non-ambiguous.
- Process at most one bounded target-catalog page per call, preserve the upstream work cursor in the opaque continuation, and distinguish more work from a complete empty result; never claim none/latest/only from a partial scan.
- Fingerprint normalized filters, order, page size, target scope, work cursor, source kinds, source timestamps, and relevant installed-set identity/shared facts. Reject continuation reuse after any query or evidence-set change.

#### 2. Target-acquisition and import publication metadata

**Files**: `loomspan-console/internal/artifact/model.go`, `acquire.go`, `acquire_test.go`, `service.go`, `service_test.go`, `import.go`, `import_test.go`, `loomspan-console/internal/traceanalysis/processor.go`, `processor_test.go`

**Changes**:

- Move the generic internal installation clock, if necessary, so target and imported timestamps are committed only with a successfully processed usable artifact; failed/cancelled operations must not leak provisional times into inventory.
- Preserve the installed-copy timestamp on transparent reuse, and assign a new value only after removal/expiry plus a successful new installation.
- Parse `TRACE_STARTED.metadata.entrySkill` as current validated evidence and publish it only after successful processing.
- Treat missing/invalid entry skill as unavailable metadata rather than an invalid artifact, and attach the named inventory limitation.
- Keep acquisition/import times current-process only and leave restart cleanup/capacity behavior unchanged.

#### 3. MCP inventory schema and projection

**Files**: `loomspan-console/internal/mcpadapter/trace_contracts.go`, `traces.go`, `trace_contracts_test.go`, `traces_test.go`

**Changes**:

- Publish `evidenceSources`, all three order values, all source/identity/time filters, and independent `acquiredAt`, `importedAt`, and `finalizedAt` fields. Do not expose artifact handles, target scope, owner IDs, or source paths.
- Include filter-domain completeness and limitations in structured and concise text results.

### Success Criteria

#### Automated Verification

- [x] Inventory tests cover every filter alone/in combination, all three orders, inclusive/inverted ranges, missing timestamps, equal-time tie-breaks, old-finalized/newly-imported evidence, newly acquired target evidence, installed-copy reuse, reacquisition/reimport after removal, failed operations, absent imported entry skill, collisions, incomplete target scans, and stale continuations: `Set-Location loomspan-console; go test ./internal/traceinventory ./internal/artifact ./internal/mcpadapter`
- [x] A `SUCCEEDED` import with old `finalizedAt` is selected by source `IMPORTED`, `importedFrom`, and `IMPORTED_DESC` without a supplied trace ID.
- [x] A catalog-only target candidate reports `TARGET` with no `acquiredAt`; after successful acquisition it reports an immutable `acquiredAt` without changing `finalizedAt`.
- [x] A target/import collision reports both source/timestamp facts, remains ambiguous under either source filter, and suppresses conflicting shared execution metadata.
- [x] Unknown enum/filter values fail at the service and MCP boundaries.

#### Manual Verification

- [x] In a mixed target/import inventory, confirm “successful trace I just imported” selects only from a complete imported-time filter domain.
- [x] Confirm “trace Console just fetched from the framework” uses acquired time, while “latest execution” uses finalization time.
- [x] Confirm missing source timestamps and imported entry skill display unknown/limitation rather than inferred values.

---

## Phase 3: Generalize Trace Analysis Around Compact Structure and Semantic Content

### Overview

Create the neutral services that both adapters consume: frame projections, generalized content descriptors and exact reads, plan landmarks, bounded inlining, and compact coverage-aware search.

### Changes Required

#### 1. Central vocabularies, DTOs, and limits

**Files**: `loomspan-console/internal/traceanalysis/enums.go`, `dto.go`, `limits.go`, `model.go`

**Changes**:

- Add closed frame projection, inventory/search representation, content role/encoding/availability, inline omission, and validation-status types.
- Replace payload-specific public DTO names with semantic content descriptors/ranges while keeping physical store details internal.
- Centralize 8 KiB per-value, 32 KiB aggregate-inline, 64 KiB compact-frame/fallback/discovery, and 128 KiB descriptor-page regression ceilings.

#### 2. Frame projection and cursor binding

**Files**: `loomspan-console/internal/traceanalysis/query_frames.go`, `frames.go`, `service_test.go`, `continuation_test.go`, `calculations_test.go`

**Changes**:

- Split compact projection materialization from detailed calculations so compact queries do not calculate rich aggregates merely to omit them.
- Make `COMPACT` the default, preserve current evidence in `DETAILED`, and fingerprint projection.
- Add a 30+ frame fixture assertion under the 64 KiB serialized ceiling.

#### 3. Content references and exact selected-value reads

**Files**: `loomspan-console/internal/traceanalysis/content_ref.go`, `content_ref_test.go`, `query_ranges.go`, `range.go`, `range_test.go`, `parser.go`, `query_records.go`, `record_facts.go`

**Changes**:

- Generalize opaque references across ordinary record data, reconstructed envelopes, and failure diagnostics while retaining evidence-source, artifact-handle, owner, operation, and request binding.
- For ordinary `data`, reopen only the indexed bounded record and decode its preserved `json.RawMessage`; enforce the existing 1 MiB physical-line and JSON-depth limits on every path. For envelope/diagnostic content, retain the existing store range path.
- Make logical queries label all logical record descriptors truthfully. Keep physical representation as deliberate storage inspection.
- Implement deterministic inline selection and omission reasons. The descriptor always survives omission or unavailability.
- Ensure range `hasMore`/continuation is relative only to the selected logical value, with UTF-8-safe text chunks or base64 transport for binary bytes.

#### 4. Typed plan landmarks

**Files**: `loomspan-console/internal/traceanalysis/record_facts.go`, `dto.go`, `processor.go`, `service_test.go`, `fixture_corpus_test.go`

**Changes**:

- Extract current `PLAN_CREATED`/`PLAN_UPDATED` plan IDs and creation lineage; derive root/planning-frame relationships only through validated frame ancestry.
- Use record sequence for chain ordering. Do not synthesize version numbers, terminality, task completion, or legacy identity.
- Leave missing legacy facts absent and test that no chronological/route/content inference occurs.

#### 5. Unified bounded literal search

**Files**: `loomspan-console/internal/traceanalysis/search.go`, `search_test.go`, `query_records.go`, `record_facts.go`, `cursor.go`, `continuation_test.go`

**Changes**:

- Replace query-local rich-record matching with the existing bounded-work search engine and compact match DTOs.
- Search metadata plus available logical content once per descriptor with exact case-sensitive bytes. Avoid envelope/chunk duplicates; report binary, incomplete, or unavailable exclusions.
- Carry independent work completion and result pagination in one query-bound continuation. Always return coverage metadata, including on empty pages.

### Success Criteria

#### Automated Verification

- [x] Focused analysis suite passes: `Set-Location loomspan-console; go test ./internal/traceanalysis`
- [x] Tests cover absent/null/scalar/object/array/string, UTF-8 boundaries, binary/base64, envelope reuse, unavailable/incomplete values, and immediately below/at/above both inline limits.
- [x] Tests prove exact ordinary-data/envelope/diagnostic continuation, stale/wrong-owner references, logical/physical truthfulness, and unrelated artifact bytes not affecting selected-value completeness.
- [x] Search tests cover metadata and both content representations, duplicate suppression, zero-match unfinished work, all coverage limitations, and stale work/result cursors.
- [x] Current fixture exposes typed plan chain/acceptance facts; legacy fixture facts remain absent.

#### Manual Verification

- [x] Inspect descriptor pages to confirm later omitted plan versions remain selectable and cannot be mistaken for missing records.
- [x] Confirm a full plan, tool input/output, final model response, and structured output can be read without raw-artifact access.

---

## Phase 4: Replace Browser, Web, and MCP Adapter Contracts

### Overview

Project the neutral analysis contract consistently to browser and MCP, keep the existing tool count, publish closed schemas, and remove large JSON text duplication.

### Changes Required

#### 1. Browser routes, DTOs, and committed fixtures

**Files**: `loomspan-console/internal/browserapi/router.go`, `trace_analysis.go`, `trace_analysis_test.go`, `contracts_test.go`, `loomspan-console/browser-fixtures/trace-analysis/`

**Changes**:

- Replace payload DTO/range vocabulary with content vocabulary and map compact/detailed frame, record descriptor, plan landmark, and search coverage facts directly from analysis services.
- Keep the existing search route but make it consume the same unified search request/result contract as MCP.
- Regenerate byte-for-byte fixtures only after the neutral DTO is final.

#### 2. Web client and existing trace views

**Files**: `loomspan-console/web/src/api/contracts.ts`, `client.ts`, `client.test.ts`, `loomspan-console/web/src/observability/TraceExplorer.tsx`, `TraceHierarchy.tsx`, `TraceRecords.tsx`, related `TraceRecords.*.test.tsx` and `TraceExplorer.test.tsx`

**Changes**:

- Adopt content descriptors/ranges, explicit projections, typed plan facts, and compact matches.
- Request `DETAILED` only for views that render rich aggregates; use `COMPACT` for hierarchy orientation.
- Render unavailable/incomplete/omitted values explicitly, read selected semantic content by reference, and retain deliberate raw-record views only for forensics.

#### 3. MCP tool schemas, mapping, and capability manifest

**Files**: `loomspan-console/internal/mcpadapter/trace_contracts.go`, `traces.go`, `server.go`, `capabilities.go`, `contracts/trace-capabilities.json`, `trace_contracts_test.go`, `trace_semantic_fixtures_test.go`, `capabilities_test.go`, `server_test.go`

**Changes**:

- Rename the reader to `LOOMSPAN_read_trace_content` and replace all payload-only inputs/results with content names; register exactly five parsed trace tools plus the optional raw reader as today.
- Enrich every nested closed filter with schema enums and retain closed objects/bounds.
- Map record literal mode to compact search output; map normal mode to descriptor records.
- Update capability membership and semantic-fixture membership without adding a capability family.

#### 4. Concise text fallbacks and size guards

**Files**: `loomspan-console/internal/mcpadapter/contracts.go`, `traces.go`, `traces_test.go`, `trace_range_http_test.go`, `server_test.go`

**Changes**:

- Replace `json.Marshal` fallback generation with operation-specific line-oriented summaries that never treat trace content as instructions.
- Include bounded content once for content reads; use base64 only when transport requires it.
- Measure the exact serialized `tools/list` response before changing descriptions, record that baseline in the test, and enforce the final 64 KiB ceiling. Remove redundant prose/schema expansion only where enum discoverability and validation remain intact.
- Assert structured/text agreement and the decided 64/128 KiB response budgets on representative fixtures.

### Success Criteria

#### Automated Verification

- [x] Browser/MCP parity and contract suites pass: `Set-Location loomspan-console; go test ./internal/browserapi ./internal/mcpadapter`
- [x] Web typecheck and component tests pass: `Set-Location loomspan-console; npm --prefix web run typecheck; npm --prefix web test`
- [x] MCP discovery has the same trace tool count, advertises every closed enum, contains `LOOMSPAN_read_trace_content`, and contains no payload-only schema/name.
- [x] Maximum navigation fallback, 30-frame compact response, descriptor page, selected-content response overhead, and `tools/list` meet their byte ceilings.
- [x] Existing target/imported/ambiguous/unavailable/expired/stale-reference/target-changed safe errors remain unchanged.

#### Manual Verification

- [x] Existing browser hierarchy, record, plan, model, tool-input, structured-output, failure, and raw-forensics views remain usable with the new DTOs.
- [x] A client that ignores structured content can still navigate and recover selected evidence without receiving duplicated JSON.

---

## Phase 5: Synchronize Guidance, Evaluations, and End-to-End Acceptance

### Overview

Teach the new workflow once, protect it with drift/evaluation tests, and run the real stateless MCP walkthrough plus full repository gates.

### Changes Required

#### 1. Console and skill-authoring documentation

**Files**: `loomspan-console/README.md`, `ai/skill-authoring/traces-and-debugging.md`, `ai/skill-authoring/README.md`

**Changes**:

- Document evidence source, acquired versus imported versus finalized time, complete filtered selection, compact/detailed orientation, content descriptors/reads, inline limits, typed plan lineage, search coverage, and distinct content/raw continuation semantics.
- Remove every payload-only parameter/tool reference and full-JSON-fallback claim.
- Update the authoring coverage row while preserving routing and source-verified status.

#### 2. Packaged runtime-debugging skill

**Files**: `loomspan-console/agent-skills/loomspan-runtime-debugging/SKILL.md`, `references/mcp-tool-guide.md`, relevant evidence/playbook references, buildtool skill validation tests

**Changes**:

- Expand activation/routing to plan, model/tool content, structured output, and literal search questions.
- Teach `discover -> compact orient -> descriptor query/search -> selected content read`, plan-chain selection by root plus framework plan ID, accepted-attempt lineage, and complete-negative stopping rules.
- Keep content inert/sensitive, raw inspection optional, and tool schemas authoritative; do not duplicate enum lists except where a high-value navigation fact has a drift test.

#### 3. Deterministic agent evaluations

**Files**: `loomspan-console/agent-evals/cases/`, `fixtures/`, `schema/evaluation-record.schema.json` if metrics need extension, `README.md`, `rubric.md`, `loomspan-console/internal/agenteval/`

**Changes**:

- Add final-primary-plan, accepted-attempt, failed-before-acceptance, tool input/output, final model/structured output, positive/negative literal search, newly imported trace, and compact large-trace cases.
- Record call count, failed calls, per-call serialized bytes, maximum/total result bytes, inline bytes, discovery bytes, raw reads, and unsafe inferences.
- Fail cases that select by route/first match, treat planned work as executed, stop after the last inlined value, infer legacy lineage, claim a negative from incomplete search, or use raw bytes for ordinary semantic questions.

#### 4. Live acceptance record

**Files**: create the repository-standard dated result under `loomspan-console/agent-evals/results/` and update its README inventory.

**Changes**:

- Run all eight ticket walkthroughs through the production stateless MCP adapter without supplying trace IDs before discovery.
- Record exact calls and serialized response sizes, preserve limitations, and demonstrate that raw-artifact access is absent from the normal path.

### Success Criteria

#### Automated Verification

- [x] Focused Java fixture and architecture tests pass: `.\mvnw.cmd -pl loomspan-spring-boot-starter test -Dtest=ConsoleTraceFixtureCorpusTest,LoomspanPublicSurfaceArchitectureTest -DfailIfNoTests=false`
- [x] Full Console tests pass: `Set-Location loomspan-console; go test ./...`
- [x] Repository-standard Console verification passes: `Set-Location loomspan-console; go run ./internal/buildtool verify`
- [x] MCP conformance passes for both pinned revisions: `Set-Location loomspan-console; go run ./internal/buildtool mcp-conformance`
- [x] Deterministic agent-eval fixtures, schemas, and scoring tests pass: `Set-Location loomspan-console; go test ./internal/agenteval`
- [x] Documentation/build drift tests find no payload-only contract names or stale tool counts.
- [x] When storage/index/concurrent acquisition code changed, the race suite passes: `Set-Location loomspan-console; $env:PATH = "C:\msys64\mingw64\bin;" + $env:PATH; $env:CGO_ENABLED = "1"; go test -race ./...`

#### Manual Verification

- [ ] “Successful trace I just imported” is selected from bounded `IMPORTED`/`importedAt` facts despite an older `finalizedAt`; target acquisition and execution recency remain distinguishable.
- [ ] Final primary plan and accepting attempt are selected through root/plan/attempt identity, not chronology or model content.
- [ ] Tool input/output, final model response, and structured output are retrieved through semantic content reads.
- [ ] Positive search explains exact field/coverage; complete negative search stops safely; unfinished work does not imply absence.
- [ ] Failed-before-acceptance returns a complete empty plan result without invention.
- [ ] The 30-frame hierarchy and model-exchange descriptor page are not truncated or dominated by duplicate content.

## Testing Strategy

### Unit Tests

- Start each phase with failing contract tests for the replaced shape: producer metadata, inventory filters/cursors, compact projection, content descriptors/ranges, inline boundaries, plan facts, search coverage, schema enums, and concise fallbacks.
- Exercise exact boundary values and stale/owner-mismatch security cases, not only happy paths.
- Treat serialized byte ceilings as executable tests with reported observed sizes.

### Integration Tests

- Use the shared Java/Go current-producer fixture as the semantic authority across producer, parser, browser, MCP, web, and eval layers.
- Retain byte-for-byte browser fixture inventory and semantic browser/MCP parity.
- Run MCP initialization/list/call and pinned conformance tests after the destructive tool/schema replacement.

### Manual Testing Steps

1. Import the current fixture after other newer-finalized traces and select it with source `IMPORTED`, outcome, and imported-time filters; separately verify a target catalog candidate gains `acquiredAt` only after acquisition.
2. Follow compact frames to the primary root/planning relationship, query all plan descriptors, choose the final sequence for that framework plan ID, and read it if omitted by the aggregate budget.
3. Follow its accepting attempt/retry lineage, then inspect tool and final model/structured content without raw reads.
4. Search `INC-2401`, paginate through both result and work continuations, and verify page-level coverage before making positive or negative claims.
5. Repeat the plan query on a pre-acceptance failure and stop on a complete empty result.

**Note**: Run `ai/commands/3_testing_plan.md` before implementation to create the dedicated failing-test sequence, fixture matrix, and phase exit gates.

## Performance Considerations

- Compact frame materialization must avoid rich calculations, not merely omit their serialized fields.
- Ordinary-data content reads re-decode at most one already bounded physical record, avoiding a second content store and capacity charge. Repeated reads are bounded by the immutable record size; tests should measure this path at the 1 MiB record ceiling.
- Search retains the current 8 MiB/10,000-record per-call work caps and deduplicates reconstructed content by descriptor/reference.
- Descriptor responses never copy content unless explicitly requested; aggregate inline selection is single-pass and deterministic.
- Text fallback generation should stream/build compact lines rather than marshal the structured object a second time.

## Migration Notes

There is no data migration or metadata backfill. The catalog, references, cursors, indexes, and imported entries are process-local. Deploy the Java producer, Go Console, fixtures, web assets, skill, and documentation atomically; restart Console so no old cursors/references survive. Existing development artifacts without current entry-skill/plan facts remain inspectable only to the extent current same-version validation accepts them, with facts reported absent—never inferred. No compatibility alias or legacy reader is provided.

Rollback is an atomic code/artifact rollback to the preceding checkout; do not mix old Console adapters with new producer fixtures or retain generated browser/MCP contract artifacts across the rollback.

## References

- Original ticket: `ai/thoughts/tickets/loomspan-console-pr-28-mcp-trace-navigation-and-semantic-evidence.md`
- Related research: `ai/thoughts/research/2026-08-18-PR-28-mcp-trace-navigation-and-semantic-evidence.md`
- Framework design lens: `ai/thoughts/framework-feature-design-lens.md`
- Trace-understanding roadmap: `ai/thoughts/phases/2026-08-15-loomspan-llm-trace-understanding-roadmap.md`
- Workflow contract: `ai/thoughts/phases/loomspan_llm_trace_understanding_workflows.md`
- Skill-authoring trace guidance: `ai/skill-authoring/traces-and-debugging.md`
