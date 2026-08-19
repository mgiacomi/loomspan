# PR 28 — MCP Trace Navigation and Semantic Evidence Testing Plan

## Change Summary

- Replace the unreleased payload-only MCP/Console trace contract with descriptor-first semantic content and exact `LOOMSPAN_read_trace_content` reads.
- Add source-aware finalized-trace discovery with independent `evidenceSources`, `acquiredAt`, `importedAt`, and `finalizedAt` facts; bounded filters; three explicit orders; collision-safe matching; and query/evidence-bound continuations.
- Make compact frames the default, retain one explicit detailed projection, and enforce serialized-response budgets.
- Project current producer-owned plan identity and accepted-attempt lineage without inferring legacy facts or execution success.
- Replace rich-record literal matches with compact, bounded-work, coverage-aware search over metadata and available logical content.
- Replace full structured-JSON MCP text duplication with concise fact-complete fallbacks, publish every closed vocabulary, and guard `tools/list` size.
- Update the current Java writer, Go reader/indexes/services, browser/MCP adapters, web consumer, shared fixtures, authoring guidance, packaged Agent Skill, and agent evaluations atomically.

## Impacted Areas

- **Java current trace writer**: `DefaultExecutionTraceHandle`, plan recording contracts, `ExecutionTraceHandleTest`, `ExecutionTraceContractTest`, and `ConsoleTraceFixtureCorpusTest`.
- **Shared Java/Go corpus**: `loomspan-console-fixtures/traces`, `expected`, and corpus inventories/readers.
- **Artifact lifecycle**: target acquisition, import, successful-publication clocks, reuse, cancellation/failure cleanup, expiry/removal, and owner/source classification under `loomspan-console/internal/artifact`.
- **Unified inventory**: source-instance grouping, filters, orders, incomplete work pagination, collision facts, shared-field suppression, and cursor fingerprints under `loomspan-console/internal/traceinventory`.
- **Trace analysis**: frame projections, content descriptors/references/ranges, logical/physical record truthfulness, inline budgets, plan landmarks, search, enums, limits, indexes, and continuations.
- **Adapters and consumers**: browser API/fixtures, MCP schemas/mappers/capabilities/fallbacks, and TypeScript API/trace views.
- **Operational boundaries**: trace-ID resolution, target generation, ownership, artifact expiry, authentication, body/range bounds, stateless MCP protocol behavior, and optional raw capability.
- **Documentation and evaluation**: Console README, `ai/skill-authoring` trace topic/coverage row, packaged runtime-debugging skill, capability drift tests, and deterministic/live agent evaluations.

## Risk Assessment

### Highest-risk behaviors

- **Source/time confusion**: rewriting `finalizedAt`, publishing provisional acquisition/import time, combining target and imported fields into one false match, or allowing filters to silently disambiguate a collision.
- **Unsafe negative conclusions**: treating an empty but work-incomplete inventory/search page as “none,” or losing `complete`, `hasMore`, continuation, and limitation distinctions in adapters/fallbacks.
- **Content corruption or leakage**: returning encoded record envelopes instead of logical `data`, confusing explicit JSON `null` with absence, splitting UTF-8 incorrectly, losing exact byte offsets, exposing internal handles/owners/paths, or partially inlining a value.
- **False plan interpretation**: choosing a nested/rejected/legacy plan by chronology, route, skill name, or model-authored ID; synthesizing version/success/task completion; or attributing acceptance to the wrong attempt.
- **Unbounded responses/work**: calculating detailed frames for compact queries, repeated envelope content, aggregate inline growth, rich search records, duplicated structured JSON, oversized discovery schemas, or unbounded target-catalog scanning.
- **Cross-layer drift**: Java writer, Go parser/index, browser fixtures, MCP schemas, web DTOs, capability manifest, skill guidance, and eval cases describing different contracts.
- **Lifecycle/concurrency regression**: timestamp publication before durable installation, changed capacity/lease behavior, cancelled waiters affecting leaders, reuse changing timestamps, or expiry/reacquisition retaining stale references.

### Edge cases

- Catalog-only `TARGET` candidate with no `acquiredAt`; imported-only candidate with no `acquiredAt`; target-only candidate with no `importedAt`.
- `TARGET`/`IMPORTED` collision with matching or conflicting session, entry skill, outcome, and finalization facts; only one or both instances satisfying a filter.
- Inclusive time bounds, equal timestamps, missing order timestamps, inverted ranges, empty/duplicate/unknown source filters, and evidence-set changes between pages.
- Failed, cancelled, short, long, stale, duplicate, or concurrently joined acquisition/import operations; reuse versus reinstall after removal/expiry.
- Absent `data`, explicit `null`, empty string/object/array, scalar JSON, nested JSON, UTF-8 boundary, non-UTF-8 envelope bytes, unavailable/incomplete content, maximum physical record, stale/wrong-owner/wrong-kind reference, and unrelated trailing artifact bytes.
- Per-value source-byte sizes 8 KiB minus one, exactly 8 KiB, and plus one; aggregate sizes 32 KiB minus one, exactly 32 KiB, and plus one.
- Search matches at buffer/content boundaries, duplicate envelope references, no-match work pages, result-page exhaustion before work exhaustion, case differences, excluded binary/unavailable content, and stale work cursors.
- Maximum MCP page sizes, 30+ frames, 64 records, 16 MiB content ranges, concurrent clients, cancellation, and clients that ignore structured content.

### Contract and compatibility scope

| Surface | Test treatment |
| --- | --- |
| Application API | Protected. `LoomspanPublicSurfaceArchitectureTest` must continue to prove exactly eight approved API types, no leaked internal/autoconfigure signature types, and no accidental extension points. |
| Supported SPI | Protected absence. Continue proving no supported SPI package/type exists. |
| Configuration and manifest contracts | No YAML/application-property change. Protect packaged Agent Skill frontmatter, capability membership, release packaging, and source-verified authoring claims. |
| Persisted or serialized contracts | No historical/cross-version compatibility suite. Preserve exact current `consoleCompatibilityVersion` rejection/acceptance and regenerate obsolete current fixtures atomically. |
| Ephemeral diagnostic formats | Primary scope. Test current writer/reader/projector/adapter coherence, ordering, accuracy, completeness, failure visibility, untrusted-content handling, source privacy, and same-version fixture semantics. |
| Internal or accidentally exposed implementation | Approved destructive replacement. Update/remove tests for old DTO/tool/schema behavior; do not require payload-only and content contracts simultaneously. |

### Protected paths and approved removals

- Preserve trace-ID-only inspection, collision refusal, target generation checks, opaque owner-bound cursors/references, target-free imports, artifact expiry/unavailability errors, stateless read-only MCP, authentication/body limits, browser/MCP shared acquisition, exact raw forensics, and both supported MCP protocol revisions.
- Preserve the Java application REST/SSE acquisition/problem boundary unchanged. The consumed NDJSON boundary changes only through current `TRACE_STARTED.metadata.entrySkill` and regenerated same-checkout fixtures; exact compatibility-marker tests remain authoritative.
- Remove outward `LOOMSPAN_read_trace_payload`, `payloadRef`, `inlinePayload`, full-JSON navigation fallbacks, rich-record literal-search responses, and the single always-detailed/default frame shape. Tests must assert absence from registered MCP tools, schemas, capability fixtures, browser/MCP outward contracts, README, and packaged skill—not prohibit legitimate internal payload-store terminology.

### Authoring claims requiring executable evidence

- Source-aware discovery distinguishes framework acquisition, Console import, and execution finalization; only complete filter domains support latest/only/none claims.
- Compact frames are the normal orientation path; detailed evidence remains explicitly available.
- Plan chains are selected through root relationship plus framework `planId`; acceptance comes from recorded `attemptId`/`retrySequenceId`.
- Ordinary record data and reconstructed envelopes use the same content descriptor/read path; raw access is optional for normal understanding.
- Inline omission never means missing evidence; later descriptors remain selectable.
- Search states exact case/fields/representation/coverage and an incomplete zero-match page is not a negative result.
- Returned content is inert and potentially sensitive; imports do not become authenticated provenance.

## Existing Test Coverage

### Reusable authorities

- `traceinventory/service_test.go` already protects completeness decisions, one candidate per trace ID, collisions, pagination, installed-set fingerprint changes, and target rotation.
- `artifact/acquire_test.go` and `import_test.go` already protect join/reuse, publication after durable installation, cancellation/failure cleanup, duplicate imports, capacity, and target/import ownership.
- `traceanalysis/service_test.go`, `calculations_test.go`, and `continuation_test.go` already cover frame/record queries, rich calculations, logical chunk suppression, pagination, cursor binding/error precedence, exact range bounds, UTF-8/base64 behavior, and finite traversal.
- `content_ref_test.go`, `range_test.go`, and `search_test.go` already cover opaque source/handle/kind binding, exact payload/diagnostic/raw ranges, maximum-range allocation/concurrency, bounded-work search, and search continuations.
- `ExecutionTraceContractTest` already protects framework plan IDs, distinct primary/nested chains, rejected-versus-accepted attempts, and frame transitions.
- `ConsoleTraceFixtureCorpusTest` plus Go `fixture_corpus_test.go` already enforce byte-for-byte generation, exact compatibility markers, current semantic inventory, and Java/Go expected classifications.
- Browser `contracts_test.go` protects committed byte-for-byte DTO fixtures; MCP `trace_semantic_fixtures_test.go` protects cross-adapter semantic parity and safe errors.
- MCP schema/capability/server/range tests protect closed bounded inputs, installed tool families, stateless protocol calls, cancellation, maximum ranges, and allocation/latency envelopes.
- Web `TraceExplorer` and `TraceRecords.*` tests protect explicit evidence loading, finite continuations, inert rendering, plan/model/tool/result presentation, and raw forensic views.
- Buildtool and agent-eval tests protect canonical skill packaging/document links, sanitized stable evidence identifiers, production-adapter cases, and unsupported-claim/forbidden-operation scoring.

### Coverage gaps to close

- No source-specific inventory timestamps/filters/orders or same-instance collision matching.
- No producer/imported `entrySkill` corpus contract.
- No compact frame projection or response-size guard.
- No general ordinary-`data` content descriptor/read path, aggregate inline limit, or generalized content-reference vocabulary.
- No Go typed plan landmark/cross-language current plan fixture.
- MCP search bypasses the bounded search service and lacks coverage metadata/compact matches.
- No concise fallback or `tools/list` serialized-size baseline/budget.
- No PR-28 agent cases for final plans, accepted attempts, semantic tool/model/output content, imported-time discovery, or safe literal negatives.

## Bug Reproduction / Failing Test First

- **Name**: `TestLogicalRecordDataIsRepresentedAsReadableSemanticContent`
- **Type**: Go unit/service test
- **Location**: `loomspan-console/internal/traceanalysis/service_test.go`
- **Arrange**:
  - Install a minimal valid trace containing `TRACE_STARTED`, one `PLAN_CREATED` with ordinary JSON `data`, and `TRACE_COMPLETED`.
  - Query records with logical representation and `types=[PLAN_CREATED]`.
- **Act**: Read the single returned record, then follow its generalized content reference with a deliberately small range.
- **Assert**:
  - The record is labeled logical rather than physical.
  - It exposes one available, complete `application/json` content descriptor for the exact `data` value.
  - The bounded reader reconstructs only that value and returns selected-value `hasMore`, not backing-artifact continuation.
- **Expected failure before implementation**: current code labels the ordinary-data record `physical`, exposes no general descriptor/reference, and therefore cannot perform the semantic read without raw-record/artifact access.
- **Cost and isolation**: no browser, MCP, Java process, or large corpus is required. Add this red test before DTO/reader implementation; if the new descriptor type must be scaffolded for compilation, keep the assertion failing on current materialization until the behavior lands.

## Failing-First Sequence by Implementation Phase

1. Add the primary traceanalysis test above and record its failure.
2. Add `writesEntrySkillOnTraceStarted` in `ExecutionTraceHandleTest`; it fails because current start metadata omits the field.
3. Add inventory source/publication/order tests using the new DTO scaffold; they fail because inventory drops the existing lifecycle time and has no filters/orders.
4. Add `TestCompactFrameProjectionIsDefaultAndDoesNotReadDetailedIndexes`; it fails because only the rich projector exists.
5. Add MCP discovery/schema tests for `LOOMSPAN_read_trace_content`, nested enums, concise fallback, and size ceilings; they fail against the current payload-only/full-JSON contract.
6. Add browser/web/agent fixture expectations only after the neutral service contract stabilizes; regenerate committed fixtures once, never make tests accept both old and new shapes.

## Tests to Add or Update

### 1. Current producer entry skill and plan-chain corpus

- **Names**:
  - Java `writesEntrySkillOnTraceStarted`
  - Java `currentPlanSemanticEvidenceFixtureContainsDistinctJoinableChains`
  - Go `TestFixtureCorpusExposesCurrentEntrySkillPlanIdentityAndAcceptedAttempt`
- **Type**: Java unit/contract plus cross-language integration fixture
- **Location**: `ExecutionTraceHandleTest.java`, `ExecutionTraceContractTest.java`, `ConsoleTraceFixtureCorpusTest.java`, `loomspan-console-fixtures/`, and `traceanalysis/fixture_corpus_test.go`
- **What it proves**:
  - Current producer records exact nonblank entry skill on `TRACE_STARTED`.
  - Primary and nested same-skill plan chains remain distinct across rejected/accepted attempts and updates.
  - Creation records carry framework plan ID and accepting attempt/retry IDs; updates retain the ID; rejected proposals never become recorded plans.
  - Java-generated bytes and Go expected semantics agree for the current compatibility marker.
- **Fixtures/data**: deterministic synthetic current-producer trace with primary/nested planning frames, a rejected response, accepted creation, multiple updates exceeding aggregate inline pressure, ordinary/envelope model content, tool input/output, advisor mutation, validation, structured output, explicit null, and terminal facts.
- **Mocks**: Java deterministic clock/IDs and existing fixture builders; no external provider or target.
- **Contract classification**: Ephemeral diagnostic format; Application API boundary checked separately.
- **Compatibility expectation**: current writer/reader coherence only; regenerate old current fixtures, no legacy plan or entry-skill inference.

### 2. Compatibility marker and public Java boundary

- **Names**:
  - Existing `portableTraceCorpusRetainsExactCompatibilityMarker`
  - Existing invalid compatibility-marker corpus cases
  - Existing `apiPackageContainsExactlyEightApprovedPublicTypes`, `noSupportedSpiPackageOrTypeExists`, and `apiSignaturesRecursivelyExcludeInternalAndAutoconfigureTypes`
- **Type**: Java architecture/fixture regression
- **Location**: `ConsoleTraceFixtureCorpusTest.java`, `LoomspanPublicSurfaceArchitectureTest.java`, and current fixture expected classifications
- **What it proves**: exact same-version admission rules remain; `development` retains only its documented best-effort behavior; no public API/SPI/signature delta leaks from internal producer work.
- **Fixtures/data**: regenerated valid traces plus missing/blank/non-string/mismatched compatibility cases already in the corpus.
- **Mocks**: none beyond existing fixture generation.
- **Contract classification**: Application API, Supported SPI, and narrow same-version serialized diagnostic portability.
- **Compatibility expectation**: protected Java boundary; no historical trace reader or compatibility shim.

### 3. Successful-publication clocks and lifecycle semantics

- **Names**:
  - `TestAcquirePublishesAcquiredTimeOnlyWithUsableArtifactAndPreservesItOnReuse`
  - `TestImportPublishesImportedTimeOnlyAfterValidationAndPreservesItOnReuse`
  - `TestReinstallAfterRemovalReceivesNewSourceTime`
  - Extend cancellation/short/long/stale/duplicate/concurrent tests to assert no provisional inventory timestamp leaks
- **Type**: Go unit/concurrency
- **Location**: `artifact/acquire_test.go`, `import_test.go`, `service_test.go`, and expiry/removal tests
- **What it proves**: the clock is captured at successful publication, reuse is immutable, failed/cancelled/rejected work publishes nothing, concurrent joiners observe one value, and a genuinely new installation receives a new value.
- **Fixtures/data**: injected clock with distinct start/process/publication/reinstall instants; target/import owners; successful, cancelled, invalid, duplicate, expiry/removal cases.
- **Mocks**: existing fake downloader/processor/storage and deterministic clock; synchronize with channels rather than sleeps.
- **Contract classification**: Internal process-local lifecycle feeding the diagnostic inventory.
- **Compatibility expectation**: approved in-memory replacement; preserve capacity, cleanup, lease, and join semantics.

### 4. Source-aware inventory filters and ordering

- **Names**:
  - `TestInventoryProjectsTargetImportedAndCatalogOnlySourceFacts`
  - `TestInventoryFiltersOneEvidenceInstanceWithoutCombiningCollisionFields`
  - `TestInventorySourceFilterFindsButNeverDisambiguatesCollision`
  - `TestInventorySuppressesConflictingSharedMetadataOnAmbiguousCandidate`
  - `TestInventoryTimeWindowsAreInclusiveRequireApplicableTimestampAndRejectInversion`
  - `TestInventoryOrdersFinalizedAcquiredAndImportedMissingLastWithStableTies`
- **Type**: Go unit/service table tests
- **Location**: `traceinventory/service_test.go`
- **What it proves**:
  - `evidenceSources` is fixed `TARGET`, then `IMPORTED`; source-specific timestamps appear only where applicable.
  - All active filters match one instance; target and imported facts cannot manufacture a result.
  - Any matching instance emits one grouped candidate retaining full ambiguity; filtering never selects an owner.
  - Shared session/entry-skill/outcome/finalization is omitted on disagreement.
  - Source-time filters exclude instances without the relevant timestamp; all three orders, missing-last behavior, greatest matching timestamp, and tie-breakers are deterministic.
- **Fixtures/data**: installed target/import entries, target catalog-only entries, same-trace collisions with selectively matching/conflicting metadata, exact-bound instants, equal/missing timestamps.
- **Mocks**: fake artifact snapshot, target catalog, scope selection, and deterministic observation time.
- **Contract classification**: Internal/unreleased Console product protocol and ephemeral diagnostic discovery.
- **Compatibility expectation**: new coherent contract; preserve one-candidate/collision refusal and completeness semantics.

### 5. Bounded inventory work and continuation fingerprints

- **Names**:
  - `TestInventoryCanReturnEmptyIncompletePageWhileCatalogFilterWorkRemains`
  - `TestInventoryContinuationBindsSourcesTimesOrderPageWorkAndEvidenceSet`
  - Extend `TestInventoryCompletenessDecisionTable` and `TestInventoryHasMoreRequiresAnUnlistedCatalogTrace`
- **Type**: Go unit/service integration
- **Location**: `traceinventory/service_test.go`, `cursor.go` tests
- **What it proves**: one upstream catalog page is processed per call; zero results plus `hasMore=true` does not imply none; only domain exhaustion permits `complete=true`; changing any normalized filter/order/page/work/source timestamp/installed set invalidates continuation without leaking cursor internals.
- **Fixtures/data**: multi-page catalog where matches occur late, incomplete/upstream-failure cases, changing installed evidence between calls.
- **Mocks**: counting fake catalog and deterministic cursor codec/clock.
- **Contract classification**: Ephemeral diagnostic discovery and internal opaque continuation.
- **Compatibility expectation**: preserve safe stale/target-changed error precedence; no unbounded scan.

### 6. Imported entry-skill publication and limitation

- **Names**:
  - `TestProcessorPublishesValidatedTraceStartedEntrySkill`
  - `TestImportWithoutValidEntrySkillRemainsUsableAndReportsLimitation`
  - `TestImportedEntrySkillNeverUsesFilenameModelTextOrTargetState`
- **Type**: Go unit/integration
- **Location**: `traceanalysis/processor_test.go`, `artifact/import_test.go`, `traceinventory/service_test.go`
- **What it proves**: only validated current trace evidence supplies imported entry skill; missing/invalid evidence does not invalidate otherwise valid import and produces `IMPORTED_ENTRY_SKILL_UNAVAILABLE`; no alternate inference source is consulted.
- **Fixtures/data**: valid, missing, blank/non-string entry-skill start metadata plus misleading filename/data/target values.
- **Mocks**: existing import processor and fake target metadata.
- **Contract classification**: Ephemeral diagnostic format and internal inventory projection.
- **Compatibility expectation**: current-run usefulness without legacy inference or schema adapter.

### 7. Compact and detailed frame projections

- **Names**:
  - `TestCompactFrameProjectionIsDefaultAndDoesNotReadDetailedIndexes`
  - `TestDetailedFrameProjectionRetainsAllMechanicalEvidence`
  - `TestFrameContinuationCannotCrossProjection`
  - `TestThirtyFrameCompactProjectionSerializesBelow64KiB`
- **Type**: Go unit/service and serialization regression
- **Location**: `traceanalysis/service_test.go`, `calculations_test.go`, `continuation_test.go`, and adapter size tests
- **What it proves**: compact contains only identity/parent/type/route/open-close/outcome and direct landmark counts; detailed retains duration/usage/IDs/validations/failures/gaps/uncertainties; compact succeeds with detailed-index readers instrumented to fail/count; projection is fingerprint-bound; representative response meets budget.
- **Fixtures/data**: deterministic 30+ frame nested fixture with attempts/retries/validation/failure/gap/uncertainty and usage.
- **Mocks**: instrumented detailed-index/component reader; real serializer for byte count.
- **Contract classification**: Ephemeral diagnostic projection.
- **Compatibility expectation**: approved default-shape replacement; preserve detailed facts explicitly, not old default behavior.

### 8. General content descriptor matrix

- **Names**:
  - `TestLogicalRecordContentDescriptorsCoverOrdinaryAndEnvelopeValues`
  - `TestContentDescriptorDistinguishesAbsentNullEmptyScalarObjectArrayTextAndBinary`
  - `TestUnavailableAndIncompleteContentRemainExplicitAndSelectableWhenPossible`
  - `TestEnvelopeRecordsReuseOneSemanticDescriptorReference`
- **Type**: Go unit/service table tests
- **Location**: `traceanalysis/service_test.go`, `payload_test.go`, `processor_test.go`
- **What it proves**: one neutral descriptor contract covers plan/model/tool/advisor/validation/structured/future ordinary values and reconstructed envelopes; absent differs from explicit null; type, encoding, retained length, availability, completeness, inline eligibility, and reference are truthful; no duplicate reconstructed prompt bytes.
- **Fixtures/data**: table of JSON forms, UTF-8 text, binary envelope, missing/incomplete chunks, current plan semantic fixture, and multiple current ordinary-data record types that have no type-specific content reader, proving the transport stays generic when the closed record vocabulary grows later.
- **Mocks**: installed immutable artifact and real parser/index writers.
- **Contract classification**: Ephemeral diagnostic format.
- **Compatibility expectation**: replace payload-only outward DTO; retain current parser invalidity rules.

### 9. Exact generalized content references and reads

- **Names**:
  - Rename/extend `TestPayloadReferenceBindsSourceHandleAndKind` to `TestContentReferenceBindsSourceHandleKindAndSelectedValue`
  - `TestOrdinaryDataContentReadReturnsOnlyPreservedJSONValue`
  - `TestContentContinuationTraversesExactlyOneSelectedValue`
  - `TestTrailingArtifactBytesDoNotSetSelectedValueHasMore`
  - Retain UTF-8/base64, expiry, cancellation, deadline, and 16 MiB range tests under generalized names
- **Type**: Go unit/service, concurrency, and allocation regression
- **Location**: `content_ref_test.go`, `range_test.go`, `continuation_test.go`, `query_ranges.go` tests
- **What it proves**: ordinary `data` is re-decoded from one bounded indexed record; envelope/diagnostic sources use the same outward reference; references cannot cross trace/source/owner/handle/kind/value; range offsets and content are exact; continuation describes only selected content; binary/invalid UTF-8 uses exact base64; last-use, expiry, and cancellation behavior remain safe.
- **Fixtures/data**: values at one byte, rune boundaries, maximum record/range sizes, multiple records after selected content, stale and forged references.
- **Mocks**: existing artifact lease/component readers, cancellation hooks, and allocation measurement.
- **Contract classification**: Ephemeral diagnostic content plus internal opaque authority boundary.
- **Compatibility expectation**: protected security/error semantics; approved outward rename with no payload alias.

### 10. Deterministic inline budgets

- **Names**:
  - `TestInlineContentPerValueBoundaryBelowAtAndAbove8KiB`
  - `TestInlineContentAggregateBoundaryBelowAtAndAbove32KiB`
  - `TestAggregateInlineSelectionIsRecordOrderedAndNeverPartial`
  - `TestInlineOmissionRetainsReadableDescriptorAndReason`
- **Type**: Go unit/service table tests
- **Location**: `traceanalysis/service_test.go`, `continuation_test.go`, `limits.go` boundary tests
- **What it proves**: source bytes—not tokens/base64 size—drive both limits; exactly-at values qualify; crossing either limit yields the correct omission reason; earlier deterministic records consume aggregate budget; later descriptors remain; continuation binds `inlineContent`; no partial content or silent clamp occurs.
- **Fixtures/data**: precisely generated byte lengths accounting for JSON encoding, multiple individually small plan versions totaling 32 KiB boundaries, binary envelope values.
- **Mocks**: none; use real serializer/content materializer.
- **Contract classification**: Ephemeral diagnostic response bound.
- **Compatibility expectation**: approved replacement of Boolean payload behavior.

### 11. Typed plan landmarks without inference

- **Names**:
  - `TestPlanLandmarksJoinPrimaryAndNestedChainsByFrameworkPlanID`
  - `TestPlanCreationCarriesRecordedAcceptingAttemptAndRetryOnly`
  - `TestPlanUpdatesUseSequenceOrderingWithoutSyntheticVersionOrSuccess`
  - `TestLegacyOrRejectedPlanContentProducesNoInferredLandmarks`
- **Type**: Go unit/corpus integration
- **Location**: `record_facts.go` tests, `service_test.go`, `fixture_corpus_test.go`, MCP semantic fixtures
- **What it proves**: root/planning ancestry is mechanically derived; creation and updates join through producer ID across frame transitions; accepted lineage is recorded, not chronological; record sequence is the sole version/order fact; nested/rejected/model-authored IDs cannot win; task states/outcome do not become plan success.
- **Fixtures/data**: current representative fixture plus explicit legacy/rejected/adversarial records.
- **Mocks**: none beyond fixture installation.
- **Contract classification**: Ephemeral diagnostic format.
- **Compatibility expectation**: current producer coherence; legacy facts remain absent, no shim.

### 12. Closed vocabularies and service-boundary validation

- **Names**:
  - `TestClosedTraceVocabulariesHaveOneAuthoritativeInventory`
  - Extend `TestTraceToolSchemasAreClosedBoundedAndUseSettledBranches`
  - `TestUnknownNestedEnumsFailBeforeServiceExecution`
- **Type**: Go unit/schema contract
- **Location**: `traceanalysis/enums.go` tests, `mcpadapter/trace_contracts_test.go`
- **What it proves**: all 33 record types, seven frame types, three outcomes, `passed/retrying/exhausted`, frame orders/projections, logical/physical representation, evidence sources, and inventory orders are both service-validated and schema-enumerated from checked authoritative sets; empty/unknown values fail consistently.
- **Fixtures/data**: each accepted value and one unknown per vocabulary; nested arrays/objects and boundary page/range values.
- **Mocks**: handler spy proving invalid MCP input never reaches service.
- **Contract classification**: Internal/unreleased MCP product protocol.
- **Compatibility expectation**: new closed contract; do not retain stringly legacy acceptance.

### 13. Coverage-aware compact literal search

- **Names**:
  - `TestRecordLiteralModeSearchesMetadataAndLogicalContentExactlyOnce`
  - `TestSearchEnvelopeStatesCaseFieldsRepresentationCoverageAndWorkCompletion`
  - `TestSearchReturnsCompactStableMatchesWithoutRichRecordFacts`
  - `TestZeroMatchPageWithRemainingWorkCannotRepresentCompleteNegative`
  - `TestSearchLimitationsReportBinaryIncompleteAndUnavailableContent`
  - Extend boundary-spanning, page-size, KMP, and stale-continuation tests
- **Type**: Go unit/service integration
- **Location**: `traceanalysis/search_test.go`, `continuation_test.go`, `query_records.go` tests
- **What it proves**: exact case-sensitive bytes cover metadata, ordinary data, and reconstructed envelope content once; compact matches carry record/content identifiers and offsets; page envelope is present even when empty; result pagination and bounded work resume without duplicates/omissions; exclusions are explicit; no full record DTO is returned.
- **Fixtures/data**: `INC-2401` in metadata/ordinary/envelope, lower-case variant, boundary-spanning content, duplicate payload reference, binary/unavailable/incomplete values, late/no matches beyond one work budget.
- **Mocks**: real search/index implementation with small injected test work limits where supported; otherwise deterministic corpus sized to current limits.
- **Contract classification**: Ephemeral diagnostic search projection.
- **Compatibility expectation**: replace current rich record-filter search; preserve mechanical exact matching and bounded work.

### 14. Browser contract, parity, and web behavior

- **Names**:
  - Update `TestBrowserTraceAnalysisFixtureCorpusMatchesCommittedInventoryByteForByte`
  - Extend MCP/browser semantic fixture parity for inventory, frames, descriptors, reads, plan facts, and search coverage
  - Web tests: `usesCompactFramesForHierarchyAndDetailedFramesForRichViews`, `readsPlanModelToolAndStructuredOutputThroughContentRef`, `rendersOmittedUnavailableAndIncompleteContentWithoutGuessing`, and `preservesSearchCoverageAcrossWorkContinuation`
- **Type**: Go adapter integration plus TypeScript component/client tests
- **Location**: `browserapi/contracts_test.go`, `trace_analysis_test.go`, `mcpadapter/trace_semantic_fixtures_test.go`, `web/src/api/client.test.ts`, `TraceExplorer.test.tsx`, and relevant `TraceRecords.*.test.tsx`
- **What it proves**: both adapters project identical neutral facts/errors; browser fixtures are regenerated once; web requests the correct projection, reads selected content, keeps raw envelope inspection deliberate, renders content inertly, and does not infer absence/success from omitted/incomplete facts.
- **Fixtures/data**: shared semantic fixture plus malicious-looking tool arguments, stale content continuation, imported evidence across target rotation, and raw forensic case.
- **Mocks**: existing fake browser fetch/service and React Testing Library; no live server for component tests.
- **Contract classification**: Internal Console application protocol with verified in-repository consumers.
- **Compatibility expectation**: atomic DTO replacement; preserve security/session/scope and raw-route behavior.

### 15. MCP tool replacement, capability membership, and safe errors

- **Names**:
  - `TestTraceToolSurfaceReplacesPayloadReaderWithoutChangingToolCount`
  - Update `TestCapabilityDescriptorsMatchInstalledToolFamilies` and manifest tests
  - Update handler resolution/opaque-token/ambiguity tests for `contentRef`
  - Retain `TestBrowserAndMCPJoinOneAcquisitionHandleAndCapacityCharge`
- **Type**: Go unit/HTTP integration
- **Location**: `mcpadapter/server_test.go`, `capabilities_test.go`, `traces_test.go`, `trace_joined_adapters_test.go`, and `contracts/trace-capabilities.json`
- **What it proves**: exactly five parsed trace tools plus optional raw remain; new content reader is registered under `loomspan.trace-inspection.v1`; payload reader/fields are absent; capability IDs do not change; target/import/ambiguity/unavailable/expired/target-changed/stale-reference recovery remains trace-ID based and never exposes handles/scopes/paths.
- **Fixtures/data**: real stateless HTTP initialization/list/call flows for both supported revisions and existing safe-error fakes.
- **Mocks**: current fake resolver/services for unit errors; production assembled server for HTTP tests.
- **Contract classification**: Configuration/manifest contract for packaged capability guidance plus internal pre-v1 MCP protocol.
- **Compatibility expectation**: capability family protected, tool member replaced intentionally with no alias.

### 16. Concise fallbacks and response/discovery budgets

- **Names**:
  - `TestNavigationFallbackIsFactCompleteLineOrientedAndBelow64KiB`
  - `TestContentFallbackIncludesSelectedBytesOnceWithAtMost4KiBMetadata`
  - `TestStructuredAndTextArmsAgreeOnIdentityCountsCompletenessAndLimitations`
  - `TestToolsListRecordsBaselineAndRemainsBelow64KiB`
  - `TestDescriptorPageBelow128KiBAndCompactFramesBelow64KiB`
- **Type**: Go serialization/HTTP performance regression
- **Location**: `mcpadapter/traces_test.go`, `trace_range_http_test.go`, `server_test.go`, and semantic fixture tests
- **What it proves**: navigation text does not marshal structured JSON; clients ignoring structured data retain safe navigation/recovery; content appears once; untrusted trace text remains labeled inert data; current exact baseline/final bytes are logged in assertion failures; chosen ceilings and 16 MiB range allocation/deadline/concurrency envelopes hold.
- **Fixtures/data**: maximum MCP pages, 30-frame fixture, 64 descriptor records, maximum content range, adversarial multiline/instruction-looking data, complete/incomplete/limited pages.
- **Mocks**: real SDK/HTTP serializer and production handlers; deterministic fixtures.
- **Contract classification**: Internal MCP transport and ephemeral diagnostic response bounds.
- **Compatibility expectation**: approved fallback replacement; preserve non-structured-client usability.

### 17. Documentation, packaged skill, and deterministic agent-eval evidence

- **Names**:
  - Extend `TestReleaseAndAuthoringDocumentationReferenceCanonicalSkillContract`
  - `TestPR28EvaluationCasesCoverDescriptorFirstWorkflowsAndForbiddenInferences`
  - Extend `TestTraceInterfaceEvaluationCasesUseOnlyLLMFacingIdentifiers`
  - Extend scorer tests for incomplete negatives, raw ordinary reads, first-match plan selection, and last-inline-value errors
- **Type**: Go build/document drift, fixture/schema, and scoring tests
- **Location**: `internal/buildtool/projectdeclarations_test.go`, `internal/agenteval/*_test.go`, `agent-evals/cases`, schema/rubric, authoring docs, and packaged skill
- **What it proves**: routed authoring guidance and packaged skill name only the new workflow; coverage row remains source-verified; evals include imported-time selection, final plan, accepted attempt, failed-before-acceptance, tool/model/structured content, positive/negative search, and large compact navigation; records stay sanitized and use only model-facing identifiers; unsafe shortcuts fail scoring.
- **Fixtures/data**: authoritative shared trace fixture references rather than copied NDJSON; deterministic expected facts and forbidden-claim/action lists.
- **Mocks**: existing isolated production MCP eval server; no model call for fixture/scorer unit tests.
- **Contract classification**: Configuration/manifest contract and author-facing debugging guidance.
- **Compatibility expectation**: packaged capability family/skill validation protected; prose follows executable new contract only.

### 18. Protocol, security, race, and full-system regression

- **Names**: retain both real MCP protocol initialization/list/call tests, cancellation/concurrent-client tests, authentication/body/session tests, raw capability degradation, exact range allocation tests, and add race coverage to changed artifact/index paths.
- **Type**: HTTP integration, conformance, race, frontend e2e, and live acceptance
- **Location**: `mcpadapter/server_test.go`, security tests, `mcp-conformance`, browser security tests, artifact/traceanalysis packages, Playwright suite, and dated `agent-evals/results` record
- **What it proves**: the feature remains stateless/read-only/bounded, works on both protocol revisions, protects credentials and ownership, degrades without raw capability, survives cancellation/concurrency, and answers all eight live ticket questions without raw NDJSON or truncated navigation.
- **Fixtures/data**: production adapter, protected temporary credentials, current representative trace plus pre-acceptance failure and mixed target/import inventory.
- **Mocks**: none for conformance/live acceptance; isolated loopback services and protected temporary profile only.
- **Contract classification**: Protected protocol/security behavior plus ephemeral diagnostic acceptance.
- **Compatibility expectation**: preserve protocol and authority boundaries while replacing only the approved trace contract.

## Fixture and Measurement Matrix

| Fixture/case | Primary obligations |
| --- | --- |
| Current primary+nested plan trace | entry skill, framework plan IDs, accepted/rejected attempts, frame transitions, ordinary/envelope content, aggregate inline pressure, final model/structured/tool content |
| Failed before plan acceptance | complete empty plan query and no accepted-plan inference |
| Mixed target/import inventory | all sources/timestamps/orders/filters, old finalized/new import, catalog-only target, same-instance filter rule, collisions |
| Content shape table | absent/null/empty/scalar/object/array/text/binary, exact length/type/encoding/availability/completeness |
| Large 30+ frame trace | compact versus detailed, skipped rich indexes, 64 KiB frame budget |
| 64-record descriptor page | descriptor-first behavior, 32 KiB inline aggregate, 128 KiB response budget |
| Search work fixture | metadata/ordinary/envelope matches, case, duplicates, late/no match, binary/unavailable limitations, work continuation |
| Adversarial inert content | fallback/web/agent does not execute or reinterpret trace data; no credentials/paths/raw sensitive content in eval record |

Measurements must report exact serialized source bytes for:

- current pre-change and final `tools/list` responses;
- every live MCP call in the eight acceptance walkthroughs;
- the 30-frame compact result and fallback;
- the 64-record descriptor result and fallback;
- total inline source bytes per result; and
- maximum selected-content result, allocation, latency, and concurrent-client run.

## How to Run

### Toolchains and environment

- Use the repository-pinned JDK/Maven wrapper, Go 1.26.5 toolchain, Node 24.18.0, and npm 12.0.2.
- Unit/integration tests need no external application, model provider, or persistent credentials.
- MCP conformance requires the pinned local runner dependencies managed by the buildtool.
- Race tests on Windows require `C:\msys64\mingw64\bin` and `CGO_ENABLED=1` as already documented.
- Live agent evaluations use an isolated production adapter and protected temporary output directory. Never print or pass the generated key on a command line; do not commit raw payloads, headers, absolute paths, or credentials.

### Red tests before implementation

```powershell
Set-Location loomspan-console
go test ./internal/traceanalysis -run TestLogicalRecordDataIsRepresentedAsReadableSemanticContent -count=1
Set-Location ..
.\mvnw.cmd -pl loomspan-spring-boot-starter test '-Dtest=ExecutionTraceHandleTest#writesEntrySkillOnTraceStarted' -DfailIfNoTests=false
```

Record the expected failures in the implementation work log. Do not weaken the assertions to make the current implementation pass.

### Focused implementation loops

```powershell
Set-Location loomspan-console
go test ./internal/artifact ./internal/traceinventory -count=1
go test ./internal/traceanalysis -count=1
go test ./internal/browserapi ./internal/mcpadapter -count=1
go test ./internal/agenteval ./internal/buildtool -count=1
npm --prefix web run typecheck
npm --prefix web test
Set-Location ..
.\mvnw.cmd -pl loomspan-spring-boot-starter test '-Dtest=ExecutionTraceHandleTest,ExecutionTraceContractTest,ConsoleTraceFixtureCorpusTest,LoomspanPublicSurfaceArchitectureTest' -DfailIfNoTests=false
```

### Full automated gates

```powershell
Set-Location loomspan-console
go test ./...
go run ./internal/buildtool verify
go run ./internal/buildtool mcp-conformance
npm --prefix web run test:e2e
Set-Location ..
.\mvnw.cmd -pl loomspan-spring-boot-starter test '-Dtest=ConsoleTraceFixtureCorpusTest,LoomspanPublicSurfaceArchitectureTest' -DfailIfNoTests=false
```

### Race gate for changed storage/index paths

```powershell
Set-Location loomspan-console
$env:PATH = "C:\msys64\mingw64\bin;" + $env:PATH
$env:CGO_ENABLED = "1"
go test -race ./...
```

### Live acceptance

Run the repository's protected agent-eval workflow from `loomspan-console/` for each new case, then score and summarize the complete required matrix as documented in `agent-evals/README.md`. In addition to the eight ticket prompts, manually verify target acquisition gains `acquiredAt` only after successful installation and that “latest execution” still uses `finalizedAt`.

## Exit Criteria

- [ ] The primary ordinary-data semantic-content test and phase-specific red tests are committed first and observed failing for the documented reasons.
- [x] Current Java producer fixtures contain exact entry skill, framework plan identity, accepted-attempt lineage, and the representative semantic topology; Java generation and Go expected semantics agree byte-for-byte/current-version.
- [x] Every source/outcome/identity/time filter, all three orders, same-instance matching, collision behavior, missing values, inclusive bounds, deterministic ties, bounded catalog work, and continuation mutation are covered.
- [x] Successful-publication clocks are immutable on reuse, renewed only by a new installation, absent after failed/cancelled work, and source-projected as `acquiredAt` or `importedAt` without altering `finalizedAt`.
- [x] Compact frames are the default, do not read detailed calculation indexes, retain sufficient landmarks, bind continuations, and meet the 64 KiB budget; detailed frames retain all existing mechanical evidence.
- [x] Ordinary data, envelopes, and diagnostics expose one truthful content contract across all shape/availability/encoding cases; exact selected-value traversal and security/error precedence pass.
- [x] Per-value and aggregate inline boundaries pass below/at/above tests, never partially inline, and retain descriptors/references with deterministic omission reasons.
- [x] Typed plan tests reject chronology/route/skill/model-ID inference, preserve primary/nested chains, and never synthesize plan success or legacy lineage.
- [x] Search covers metadata and available logical content exactly once, emits compact matches plus complete coverage metadata, and cannot turn an unfinished empty page into a negative conclusion.
- [x] MCP schemas advertise every closed vocabulary and bound; browser/MCP facts and errors agree; the web client consumes the new contract and keeps content inert.
- [x] `LOOMSPAN_read_trace_payload`, `payloadRef`, and `inlinePayload` are absent from outward registered tools/schemas/capability fixtures/docs/skill, with no compatibility alias or dual behavior; legitimate internal payload-store names are not falsely prohibited.
- [x] Navigation fallbacks are fact-complete and concise, content is included once, exact byte measurements are recorded, and the 64 KiB/128 KiB/discovery/range performance budgets pass.
- [x] Changed authoring guidance is supported by the cited producer/corpus/inventory/content/plan/search/schema/eval tests; the README coverage row and packaged skill remain synchronized and LLM-first.
- [x] Protected Java API/no-SPI architecture tests, exact compatibility-marker cases, unchanged application REST/SSE boundary, trace-ID resolution, source/owner security, target generation, raw forensics, protocol revisions, conformance, cancellation, and concurrent-client tests pass.
- [x] All focused, full verify, MCP conformance, frontend e2e, and warranted race commands pass without flakes or leaked temporary credentials/files.
- [ ] All eight live walkthroughs complete through `discover -> compact orient -> descriptor query/search -> selected content read`, with no ordinary raw-artifact reads, no truncated navigation, and exact call/byte measurements recorded.
- [ ] The failed-before-acceptance walkthrough stops on complete empty evidence; the imported-trace walkthrough uses `IMPORTED`/`importedAt`; the target-acquisition check uses `acquiredAt`; none rewrites or substitutes `finalizedAt`.
- [ ] The implementation, regenerated fixtures, browser contract files, MCP capability manifest, web assets, documentation, skill package, and eval inventory land atomically, with no unresolved test skips, placeholder assertions, historical fixture reader, or compatibility shim.

## References

- Implementation plan: `ai/thoughts/plans/2026-08-18-PR-28-mcp-trace-navigation-and-semantic-evidence.md`
- Ticket: `ai/thoughts/tickets/loomspan-console-pr-28-mcp-trace-navigation-and-semantic-evidence.md`
- Research: `ai/thoughts/research/2026-08-18-PR-28-mcp-trace-navigation-and-semantic-evidence.md`
- Framework contract lens: `ai/thoughts/framework-feature-design-lens.md`
- Skill-authoring trace guidance: `ai/skill-authoring/traces-and-debugging.md`
- Packaged skill guide: `loomspan-console/agent-skills/loomspan-runtime-debugging/references/mcp-tool-guide.md`
