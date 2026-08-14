# Loomspan Console PR 18 — Trace-Inspection MCP Surface Testing Plan

## Change Summary

- Add a transport-neutral trace inventory that joins installed target/imported copies with the selected application's current trace catalog without creating an MCP-owned catalog.
- Add enriched canonical-record and frame projections, imported-safe continuations, and opaque ranged content references for reconstructed payloads and failure diagnostics.
- Add six strict read-only MCP trace tools, three target trace resource templates, three imported trace resource templates, deterministic structured/text contracts, and exact shared-domain error mapping.
- Advertise `loomspan.trace-inspection.v1` and `loomspan.raw-artifact-inspection.v1` only with their complete tool and semantic fixture contracts; register raw-artifact inspection unconditionally in the standard binary.
- Preserve one artifact acquisition/import/cache/handle/pin/TTL/removal lifecycle across browser and MCP, including imported inspection without a selected target.
- Select one shared trace range maximum through automated exact UTF-8/base64, cancellation, memory, deadline, concurrency, and SDK/HTTP serialization evidence. The minimum is 16 MiB per source-byte call; the default remains 64 KiB. Representative-client observations happen after implementation completion and are not a gate.
- Update Console and skill-authoring documentation from executable evidence. No Java trace, REST/SSE, public Java API, supported SPI, configuration, or skill-manifest behavior changes.

## Impacted Areas

- **Shared inventory**: new `loomspan-console/internal/traceinventory` DTO, merge, ordering, pagination, and cursor code; browser trace-list routing moves from adapter-local enrichment to the shared service.
- **Trace analysis**: `loomspan-console/internal/traceanalysis` record/frame projections, fact joins, cursor owner keys, opaque `payloadRef`, diagnostic ranges, range limits, and current-run fixtures.
- **Artifact lifecycle**: existing `loomspan-console/internal/artifact` acquisition, import, lookup, leases, TTL, capacity, explicit removal, target invalidation, and shutdown behavior is consumed and regression-tested, not duplicated.
- **MCP adapter**: `loomspan-console/internal/mcpadapter` schemas, DTO mapping, deterministic fallback rendering, handlers, tool/resource registration, URI parsing, capability descriptors/manifest, generation checks, and protocol tests.
- **Composition and integration**: `loomspan-console/internal/console/service.go`, `internal/buildtool/mcp_conformance.go`, test fakes, and production-composition integration tests.
- **Fixtures**: new MCP JSON goldens and semantic capability manifest; existing Java-produced `loomspan-console-fixtures` and browser fixtures remain the source for shared trace semantics and browser compatibility.
- **Documentation evidence**: `loomspan-console/README.md`, `loomspan-console/docs/mcp-client-compatibility.md`, `ai/skill-authoring/traces-and-debugging.md`, and the `Traces and debugging` coverage row in `ai/skill-authoring/README.md`.

## Risk Assessment

- **High — evidence ownership and authority**: an imported handle could accidentally capture/invent a target, a target handle could be opened under the wrong scope, or the adapter could search all owners by handle. Tests must cover every source/identifier branch and wrong-source reuse.
- **High — inventory completeness and pagination**: joining an unordered installed snapshot with a paged application catalog can duplicate, omit, or reorder traces, leak an upstream cursor, or fail all-source discovery when no target exists. Tests must exercise both segments, keyset boundaries, catalog mutation, deduplication, filters, and target/no-target scopes.
- **High — semantic parity**: enriching records or frames in MCP could recalculate or misassociate attempt, retry, validation, failure, payload, search, gap, uncertainty, timing, or usage facts. Tests must start from shared `traceanalysis` results/Java fixtures and compare browser/MCP projections.
- **High — continuation/content-reference binding**: cursors or `payloadRef` values could be reused with a different source, handle, operation, query, range size, or process-local imported owner. Tests must verify strict parsing, token bounds, precedence, opacity, and installed-copy lifetime.
- **High — exact large-range behavior**: UTF-8 boundary adjustment, base64 expansion, JSON/SDK buffering, cancellation, and concurrent calls can lose bytes or exceed memory/deadline constraints. Checksums and actual byte offsets are mandatory at every candidate size.
- **High — shared lifecycle races**: simultaneous browser/MCP acquisition, target rotation, removal, expiry, authentication/key generation changes, and shutdown could publish stale results, double-charge capacity, refresh TTL on failure, or invalidate imported evidence incorrectly.
- **High — unsafe runtime content**: YAML, error text, metadata, records, payloads, or raw NDJSON could be interpreted as another operation, target, URL, filesystem path, command, or credential action. Instrumented tests must prove returned content is inert data at the server boundary.
- **Medium — MCP serialized contract drift**: strict schemas, exact tool count, annotations, text order/escaping, error envelopes, resource MIME/URI shapes, and capability IDs are new externally visible contracts. Goldens and black-box discovery tests protect them.
- **Medium — resource behavior**: resource reads could use a different DTO, bypass the lease/TTL/error path, expose a raw artifact, or require a selected target for imports.
- **Medium — browser regression**: replacing browser-local trace enrichment could change existing JSON fixtures or catalog semantics even though no UI change is intended.
- **Medium — incomplete capability advertisement**: required tools might be present while semantic fixtures are missing, or the raw capability might be tied incorrectly to current runtime state.
- **Medium — diagnostic truthfulness**: missing/unavailable facts could become zero, imported evidence could be described as authenticated target evidence, or the adapter could introduce diagnoses/aggregate health/completeness states.
- **Low — Java public surface**: no Java production type should change. The architecture allowlist remains a negative guard against accidental scope expansion.

### Authoring Claims Requiring Evidence

The updated `traces-and-debugging.md` guidance must be supported by focused executable evidence for:

- `TARGET` versus `IMPORTED` source identity and the absence of a fabricated target scope on imports;
- target-free imported inventory and handle-based inspection;
- separate application-catalog and local-copy availability facts;
- handle/continuation expiration under removal, TTL, target rotation, shutdown, and restart;
- successful reads refreshing shared last use while failures/cancellation do not;
- parsed summary/frame/record/payload inspection as the ordinary path and exact raw ranges as optional storage/parser forensics;
- exact bounded continuation with no cumulative traversal cap while the handle remains valid;
- same-version import compatibility without authenticity, integrity, provenance, or durable-history claims; and
- untrusted/sensitive diagnostic content being returned as inert data rather than server instructions.

Documentation review alone is not evidence. The plan cites the specific service, MCP, parity, lifecycle, range, and security tests below as the anchors for these claims.

### Compatibility Test Scope

| Surface | Test treatment |
| --- | --- |
| Application API | Run `LoomspanPublicSurfaceArchitectureTest` and verify its allowlist is unchanged. No new Java signature or leaked `internal`/`autoconfigure` type is permitted. |
| Supported SPI | Verify no supported SPI or bean replacement point is introduced. Go internal interfaces/fakes may change atomically; no compatibility tests for their old signatures are required. |
| Configuration and manifest contracts | Run existing configuration/build verification. Assert there is no new MCP YAML range/raw toggle and that shared `trace-workspace.max-bytes`, `idle-ttl`, `unlimited`, and `never` behavior still governs both sources. No skill-manifest fixture change is expected. |
| Persisted or serialized contracts | Add exact MCP schema, output, fallback, capability, resource URI/MIME, and error goldens. Preserve the existing Java application REST/SSE/problem/NDJSON fixtures unchanged. |
| Ephemeral diagnostic formats | Test current writer/reader/projector/tool coherence, exact facts and bytes, ordering, explicit failure/uncertainty, source/handle binding, security, and lifecycle. Deliberately reject old imported owner-key cursors and add no historical cursor/payload-reference reader. |
| Internal or accidentally exposed implementation | Update old browser enrichment and Go composition tests atomically. Tests must assert the obsolete adapter-local join and internal imported owner token are absent, not preserved behind fallbacks or dual behavior. |

The Java-to-Go boundary is not changed by this PR. Run the existing Java fixture corpus and Go corpus consumer as regression evidence; do not regenerate fixtures unless implementation reveals a boundary change, in which case stop and obtain an explicit compatibility-marker decision before proceeding.

## Existing Test Coverage

### Coverage to Reuse

- `internal/artifact/acquire_test.go` proves joined acquisition, stable handle reuse, independent waiter cancellation, atomic installation, capacity accounting, and no partial handle publication.
- `internal/artifact/import_test.go` proves same-service imported publication, duplicate rejection, size/cancellation behavior, target-rotation survival, and concurrent import ownership.
- `internal/artifact/expiry_test.go`, `capacity_test.go`, `lease_test.go`, `service_test.go`, `target_owner_test.go`, and `storage_test.go` prove TTL refresh/failure semantics, pinning, LRU/capacity, removal, target invalidation, shutdown, restart non-adoption, storage recovery, and path/credential safety.
- `internal/traceanalysis/calculations_test.go` and `fixture_corpus_test.go` protect hierarchy, timing, usage, attempt/retry/validation/failure relationships, gaps, uncertainties, and current Java/Go semantics.
- `internal/traceanalysis/service_test.go`, `continuation_test.go`, `range_test.go`, `search_test.go`, and diagnostic tests protect query filtering/order, complete finite traversal, cursor fingerprints/error precedence, UTF-8/base64 exactness, raw/payload separation, cancellation, and explicit unknown facts.
- `internal/browserapi/contracts_test.go` pins the browser artifact and trace-analysis DTO corpus byte-for-byte; `artifact_import_test.go` and `trace_analysis_test.go` already prove target-optional imported browser access.
- `internal/console/artifact_integration_test.go` proves production-composition acquisition, validation, authentication-time authorization, target rotation, cleanup, checksum preservation, and shared query service wiring.
- `internal/mcpadapter/contracts_test.go` protects result/error envelopes, safe domain errors, JSON escaping, annotations, generation suppression, and exact golden inventory.
- `internal/mcpadapter/server_test.go`, security/lifecycle/tracker tests, and resource tests protect stateless Streamable HTTP, both supported protocol revisions, authentication/authority/origin ordering, cancellation, multiple clients, no-store behavior, resource error mapping, and shutdown/key-generation invalidation.
- `internal/mcpadapter/parity_test.go` establishes the pattern of mapping one neutral result into browser/MCP facts and tagging tests with approved workflow requirement IDs.
- `go run ./internal/buildtool mcp-conformance` runs the pinned official runner through the production authority/authentication/admission/SDK stack for MCP `2025-11-25` and `2026-07-28`.

### Gaps This PR Must Fill

- No reusable joined target/imported inventory or inventory cursor tests.
- No enriched canonical-record query, frame-attributed gap/uncertainty, or ranged failure-diagnostic `payloadRef` tests.
- Current imported cursors deliberately expose the process-local owner ID when decoded in tests; no adapter-safe direct-token assertion exists.
- No PR 18 tool schemas, handlers, goldens, annotations/discovery assertions, or target-free imported MCP calls.
- No target/imported trace resources or exact JSON materialization tests.
- Capability tests cover required-tool membership only, not semantic fixture completeness.
- No browser/MCP trace parity or joined browser/MCP acquisition/lifecycle integration suite.
- No PR 18-specific adversarial-content authority test.
- No exact 1/4/16/32/64 MiB UTF-8/base64 response harness or peak-memory/deadline evidence. The representative-client matrix is intentionally populated after implementation completion.
- The skill-authoring guide has no executable anchors for the new MCP trace surface.

## Bug Reproduction / Failing Test First

This is a new feature rather than a defect in an existing protected contract. Use a sequence of small red tests, beginning at the lowest owner of each new behavior. Do not first make the MCP adapter assemble target/imported results itself.

### First Red Test: Target-Free Joined Inventory

- **Type**: unit
- **Location**: `loomspan-console/internal/traceinventory/service_test.go`
- **Name**: `TestInventoryAllWithoutTargetReturnsImportedEntriesAndCatalogError`
- **Arrange**: provide a fake artifact snapshot containing one installed imported trace and a target provider whose capture returns the shared no-target error. Use `SourceFilterAll`, page size 64, and a fixed clock.
- **Act**: call the new inventory service once.
- **Assert**: the call succeeds; the imported item has `source=IMPORTED`, no `targetScopeId`, a handle resolved through `Lookup`, and local facts; `applicationCatalog.requested=true`, `available=false`, and contains the unchanged shared no-target error; no target catalog call occurs.
- **Expected failure (pre-fix)**: the `traceinventory` package/service does not exist and current trace discovery captures a target before it can return imported evidence. Add only the minimal test-facing types/fakes needed to compile the red test; do not satisfy it in the MCP adapter.

### Subsequent Red Slices

1. `TestEnrichedRecordsAttachOnlyFactsOwnedByCanonicalRecord` in `internal/traceanalysis/service_test.go` fails because current `RecordSummary` has no enriched fact projection.
2. `TestFailureDiagnosticPayloadReferenceReadsInFiniteRanges` in `internal/traceanalysis/query_diagnostics_test.go` fails because the current diagnostic API materializes the complete text and payload reads accept only payload IDs.
3. `TestMCPServerDiscoversCompletePR18Surface` in `internal/mcpadapter/server_test.go` fails because the server exposes six rather than twelve tools and only one resource template.
4. `TestImportedTraceToolsWorkWithoutSelectedTarget` in `internal/mcpadapter/traces_test.go` fails until the adapter uses `evidence.ForImported()` before target capture.
5. `TestPR18CapabilityConformanceRejectsMissingSemanticFixture` in `internal/mcpadapter/capabilities_test.go` fails because current conformance only checks tool membership.

Commit or otherwise preserve each red result before implementing its slice. A compile failure is acceptable only for the initial addition of a genuinely new package/API; once the minimal skeleton exists, the behavioral assertion must fail rather than the test merely failing to compile.

## Tests to Add/Update

### 1. Inventory Source, Availability, Order, and Continuation Matrix

- **Type**: unit
- **Location**: new `loomspan-console/internal/traceinventory/service_test.go` and `cursor_test.go`
- **Names**:
  - `TestInventoryAllWithoutTargetReturnsImportedEntriesAndCatalogError`
  - `TestInventoryImportedFilterNeverCapturesTarget`
  - `TestInventoryTargetFilterRequiresSelectedTarget`
  - `TestInventoryOrdersInstalledBeforeCatalogAndDeduplicatesInstalledTraces`
  - `TestInventoryInstalledKeysetContinuesWithoutDuplicatesOrOmissions`
  - `TestInventoryApplicationSegmentPreservesOpaqueUpstreamProgress`
  - `TestInventoryMutationBetweenPagesIsWeaklyConsistentWithoutOffsetShift`
  - `TestInventoryCursorBindsFilterPageSizeAndTargetScope`
  - `TestInventoryCursorRejectsMalformedUnknownOversizedAndCrossOperationTokens`
- **What it proves**: `ALL`/`IMPORTED` target-free behavior; `TARGET` target requirement; separate requested/available/error catalog facts; exact item fields; deterministic installed ordering; catalog-only tail; lookup-derived handle; duplicate suppression; mutation semantics; strict cursor opacity/binding/length; no process-local owner or exposed top-level upstream cursor.
- **Fixtures/data**: table-driven fake snapshots with target/imported ties on `finalizedAt`, multiple catalog pages, duplicate trace IDs by owner, fixed timestamps, and controlled mutations after the first page.
- **Mocks**: narrow fake artifact snapshot/lookup, observability catalog, target capture, and fixed clock. Count calls to prove imports do not capture/query a target and catalog pages are fetched lazily.
- **Contract classification**: Internal or accidentally exposed implementation; serialized inventory cursor is an Ephemeral diagnostic format.
- **Compatibility expectation**: current-run diagnostic coherence. No old inventory exists to preserve; no cursor compatibility reader.

### 2. Browser Inventory Contract Regression

- **Type**: unit/integration
- **Location**: `loomspan-console/internal/browserapi/artifacts_test.go`, `contracts_test.go`, and existing `browser-fixtures/artifacts`/target trace fixtures
- **Names**:
  - `TestBrowserTraceListUsesSharedInventoryWithoutChangingResponseContract`
  - `TestBrowserTraceListPreservesApplicationAndLocalAvailabilitySeparately`
  - update `TestBrowserArtifactFixtureCorpusMatchesCommittedInventoryByteForByte`
- **What it proves**: browser handlers stop owning the join but preserve the established JSON fields, opaque handles, source facts, and no side-effect/last-use behavior. No second browser-only ordering or availability meaning remains.
- **Fixtures/data**: current browser goldens plus pages containing installed/non-installed target entries; imported entries remain covered by Trace Storage fixtures rather than being falsely inserted into the target-only route if that route's contract is unchanged.
- **Mocks**: replace separate observability/artifact fakes at the handler boundary with one fake shared inventory result; retain lower-level service tests for merge semantics.
- **Contract classification**: Persisted or serialized contracts for embedded browser JSON; Internal implementation for handler wiring.
- **Compatibility expectation**: protected in-repository browser contract; obsolete adapter-local join is removed without dual behavior.

### 3. Enriched Records and Frame Limitation Facts

- **Type**: unit
- **Location**: `loomspan-console/internal/traceanalysis/service_test.go`, `calculations_test.go`, `continuation_test.go`, and `fixture_corpus_test.go`
- **Names**:
  - `TestEnrichedRecordsAttachOnlyFactsOwnedByCanonicalRecord`
  - `TestEnrichedRecordFactsAreNonNilAndCanonicallyOrdered`
  - `TestEnrichedRecordPaginationHasNoFactDuplicatesOrOmissions`
  - `TestEnrichedRecordFiltersUseRecordedIdentityNotAdjacency`
  - `TestFramesAttachRecordedGapAndUncertaintyKindsWithoutPropagation`
  - `TestEnrichedQueriesPreserveUnknownAndUnavailableRatherThanZero`
- **What it proves**: attempts, retries, validations, failures, payloads, and search matches are joined once below adapters to their owning canonical record; unrelated records remain empty; frames receive only mechanically attributable gaps/uncertainties; existing hierarchy/timing/usage meanings remain unchanged; no diagnosis or aggregate completeness appears.
- **Fixtures/data**: `runtime-terminal-failure`, `validation-exhaustion`, `advisor-retry`, `nested-retry-sequences`, `chunked-payload`, `missing-response-usage`, `unattributed-usage`, and deterministic records with misleading adjacency/equal attempt numbers.
- **Mocks**: real temporary artifact service and real processed fixture bundles; avoid mocking fact indexes so joins exercise persisted component readers.
- **Contract classification**: Ephemeral diagnostic formats.
- **Compatibility expectation**: current writer/reader/projector coherence; update current DTOs atomically, with no parallel legacy record query mode.

### 4. Imported-Safe Trace Cursors and Opaque Payload References

- **Type**: unit
- **Location**: `loomspan-console/internal/traceanalysis/cursor_test.go`, `continuation_test.go`, `query_diagnostics_test.go`, and `range_test.go`
- **Names**:
  - `TestImportedTraceCursorContainsAdapterSafeOwnerAndNoProcessOwnerID`
  - `TestTraceCursorBindsSourceHandleOperationQueryAndPosition`
  - `TestTraceCursorRejectsOldImportedOwnerKeyWithoutLegacyFallback`
  - `TestPayloadReferenceRoundTripsPayloadAndFailureDiagnosticKinds`
  - `TestPayloadReferenceBindsSourceAndArtifactHandle`
  - `TestFailureDiagnosticPayloadReferenceReadsInFiniteRanges`
  - `TestPayloadAndDiagnosticReferencesCannotBeInterchanged`
  - `TestPayloadReferenceRejectsUnknownMalformedOversizedAndCrossArtifactTokens`
- **What it proves**: direct MCP-safe trace cursors reveal no process-local imported owner ID; target cursors remain scope-bound; strict unpadded-base64url and unknown-field rules hold; `payloadRef` is opaque, typed, source/handle-bound, and can address both reconstructed payloads and diagnostics; large diagnostics are continuable and UTF-8 safe.
- **Fixtures/data**: imported `runtime-terminal-failure` and `chunked-payload`; a generated multi-range UTF-8 stack diagnostic; a binary reconstructed payload; wrong source/handle/query/token mutations.
- **Mocks**: real artifact leases with fixed owner/handle data; decode only inside package tests to assert internal opacity requirements.
- **Contract classification**: Ephemeral diagnostic formats.
- **Compatibility expectation**: intentional current-process cursor break. Old `IMPORTED:<owner-id>` tokens must fail; no shim or dual decoder.

### 5. Six MCP Tool Schemas, Goldens, and Fallbacks

- **Type**: unit/adapter integration
- **Location**: new `internal/mcpadapter/trace_contracts_test.go`, `traces_test.go`, existing `contracts_test.go`, and new `internal/mcpadapter/testdata/trace-*.json`
- **Names**:
  - `TestPR18TraceToolGoldenCorpusMatchesCommittedInventory`
  - `TestTraceToolSchemasRejectUnknownFieldsAndInvalidEnums`
  - `TestGetTraceSchemaRequiresAllowedSourceIdentifierBranches`
  - `TestTraceRangeSchemasRequireStartOrContinuationAndBoundTokens`
  - `TestTraceTextFallbacksAreDeterministicEscapedFactCompleteAndNonDiagnostic`
  - update `TestMCPGoldenInventoryContainsOnlyImplementedSurface`
  - update `TestMCPToolsUseReadOnlyClosedWorldAnnotations`
- **What it proves**: exact six input/result pairs, common evidence context, inventory fields, non-null fact arrays, range offsets/encoding/content, resource links, result/error exclusivity, safe errors, JSON escaping/order/final newline, and all twelve tool annotations/discovery.
- **Fixtures/data**: goldens for target/imported success, target-free list/get, empty/final/continued/max pages, UTF-8/base64 ranges, each valid one-of branch, and representative domain errors. Use fixed clocks and stable IDs.
- **Mocks**: mapping tests use neutral DTOs directly; handler tests use narrow service fakes. Do not recreate trace calculations in expected-value builders—read shared fixture expectations.
- **Contract classification**: Persisted or serialized contracts.
- **Compatibility expectation**: new protected MCP contract is introduced atomically. Existing six PR 17 goldens remain unchanged except the exact inventory list expands intentionally.

### 6. MCP Source and Operation Branch Matrix

- **Type**: unit/adapter integration
- **Location**: `loomspan-console/internal/mcpadapter/traces_test.go`, `trace_queries_test.go`, and `trace_ranges_test.go`
- **Names**:
  - `TestGetTraceSourceIdentifierBranchMatrix`
  - `TestImportedTraceToolsWorkWithoutSelectedTarget`
  - `TestTargetTraceIDAcquiresButHandleBranchesNeverAcquire`
  - `TestTraceToolsRejectWrongSourceHandleAndCursor`
  - `TestTargetTracePublicationRejectsRotatedScope`
  - `TestImportedPublicationDoesNotRequireOrInventTargetScope`
  - `TestTraceResultIsSuppressedAfterAuthenticationGenerationChange`
- **What it proves**: exactly three valid get branches; only `TARGET + traceId` acquires; downstream operations use source+handle; imported operations never capture a target; target results recheck scope; all results recheck MCP generation; handles are never searched across owners.
- **Fixtures/data**: table-driven source/identifier combinations and fake service responses containing recognizable source/scope/handle values.
- **Mocks**: counting target, inventory, artifact, acquisition, and trace-analysis fakes; controllable blocking fake for rotation/generation races.
- **Contract classification**: Persisted or serialized MCP contract plus Internal adapter wiring.
- **Compatibility expectation**: exact new behavior; no permissive alias, inferred source, or client-supplied target scope.

### 7. Trace Resource URI and Materialization Contract

- **Type**: unit/adapter integration
- **Location**: new `loomspan-console/internal/mcpadapter/trace_resources_test.go` and existing `resources_test.go`
- **Names**:
  - `TestTraceResourceURIsRejectNoncanonicalOrUnsafeForms`
  - `TestTargetTraceResourcesMaterializeToolDTOsAsApplicationJSON`
  - `TestImportedTraceResourcesWorkWithoutSelectedTarget`
  - `TestTraceResourceExactFrameAndRecordRequireOneResult`
  - `TestTraceResourceErrorsPreserveSharedDomainEnvelope`
  - `TestTraceResourceSuccessRefreshesTTLButFailureAndCancellationDoNot`
  - `TestNoRawArtifactResourceIsRegistered`
- **What it proves**: all six templates, canonical path encoding, handle/frame/sequence validation, no query/fragment/userinfo, `application/json`, exact DTO reuse, logical record/no large expansion, shared lease/error semantics, target-free imports, and tool-complete/resource-optional behavior.
- **Fixtures/data**: Unicode frame ID, canonical/noncanonical escapes, malformed handle, zero/negative sequence, exact one-item/zero-item/multiple-item service results, expired/removed handle errors.
- **Mocks**: parser tests are pure; materialization tests use controlled analysis services and a real artifact lease clock where TTL behavior matters.
- **Contract classification**: Persisted or serialized resource contracts; Ephemeral diagnostic lifecycle.
- **Compatibility expectation**: new resource contract; existing skill resource behavior remains protected and unchanged.

### 8. Capability Tool and Semantic Manifest Conformance

- **Type**: unit/integration
- **Location**: `loomspan-console/internal/mcpadapter/capabilities_test.go`, `trace_semantic_fixtures_test.go`, and `contracts/trace-capabilities.json`
- **Names**:
  - `TestRuntimeCapabilitiesMatchCompletePR18ToolFamilies`
  - `TestCapabilityManifestMatchesProductionDescriptorsAndRegisteredTools`
  - `TestCapabilityConformanceRejectsEveryMissingRequiredTool`
  - `TestPR18CapabilityConformanceRejectsMissingSemanticFixture`
  - `TestRawArtifactCapabilityIsAdvertisedRegardlessOfRuntimeState`
- **What it proves**: the two capability IDs, exact tool families, stable semantic fixture IDs, full execution of required fixtures, and unconditional standard-binary raw capability. Removing a tool or semantic fixture independently fails conformance.
- **Fixtures/data**: reviewed JSON manifest entries for target acquisition, target-free import, source binding, availability, parity, facts, continuation, lifecycle, cancellation, concurrency, schemas/errors, and raw exactness/no acquisition.
- **Mocks**: use the assembled in-memory MCP server for lifecycle, cancellation, concurrency, and raw lifecycle/error fixtures; projection fixtures start from one transport-neutral result. Only negative conformance tests substitute a missing tool/fixture set.
- **Contract classification**: Persisted or serialized contracts.
- **Compatibility expectation**: a capability is protected only when its complete v1 promise is present. Runtime data unavailability never changes advertisement.

### 9. Browser/MCP Trace Parity and Workflow Evidence

- **Type**: integration
- **Location**: `loomspan-console/internal/mcpadapter/parity_test.go`, trace fixture tests, and workflow-tagged cases
- **Names**:
  - `TestBrowserAndMCPPreserveSameTraceInventoryFacts`
  - `TestBrowserAndMCPPreserveSameTraceSummaryFrameAndRecordFacts`
  - `TestBrowserAndMCPPreserveSameTraceRangeBytesAndOffsets`
  - `TestBrowserAndMCPPreserveSameTraceDomainErrorMeanings`
  - `TestWFFailedExecutionEvidenceIsCompletelyAddressable`
  - `TestWFExpensiveExecutionUsageIsMechanicalAndNotDoubleCounted`
  - `TestWFUnfamiliarSkillPathPreservesHierarchyAndRegisteredNames`
- **What it proves**: adapters differ only in protocol wrapper/presentation; source identity, handles, availability, outcome, terminal failure, hierarchy, durations, usage, attempts/retries/validation/failures, gaps/uncertainties, record order, payload bytes, and shared errors agree. Approved workflows are possible through general primitives without scenario tools or diagnosis.
- **Fixtures/data**: `runtime-terminal-failure`, `nested-frame-usage`, `unattributed-usage`, `repeated-skill-invocations`, `validation-exhaustion`, and `chunked-payload`, plus browser fixture DTOs.
- **Mocks**: use real shared trace-analysis results and feed them into both mappings. Do not compare separately hand-assembled browser/MCP fixtures.
- **Contract classification**: Ephemeral diagnostic formats and Persisted or serialized adapter contracts.
- **Compatibility expectation**: current-run cross-adapter coherence; no requirement for byte-identical wrappers.

### 10. Joined Acquisition, Lease, TTL, Removal, and Rotation

- **Type**: integration/race
- **Location**: `loomspan-console/internal/console/artifact_integration_test.go`, `internal/mcpadapter/server_test.go`, and focused artifact tests where lower-level instrumentation belongs
- **Names**:
  - `TestBrowserAndMCPJoinOneAcquisitionHandleAndCapacityCharge`
  - `TestBrowserAndMCPCancelWaitersIndependently`
  - `TestBrowserAndMCPSharePinsAndSuccessfulUseTTL`
  - `TestTraceReadFailureAndCancellationDoNotRefreshTTL`
  - `TestTTLCapacityAndExplicitRemovalInvalidateBothAdapters`
  - `TestTargetRotationInvalidatesTargetEvidenceAndPreservesImports`
  - `TestImportedEvidenceDisappearsOnExpiryRemovalShutdownAndRestart`
  - `TestMultipleMCPClientsAndBrowserDoNotShareCancellationState`
- **What it proves**: one download/copy/processor/handle/capacity charge; waiter independence; pin-safe cleanup; last-use rules; shared invalidation; source-selective rotation; imported ordinary lifecycle; multi-client isolation; no MCP lifecycle owner.
- **Fixtures/data**: blocking artifact stream, fake clock, small finite capacity, short TTL, target and imported copies of distinct fixtures, checksummed raw bytes.
- **Mocks**: production `artifact.Service`, `traceanalysis.Service`, target context, browser router, and MCP server; fake only the upstream application stream/catalog and clock.
- **Contract classification**: Ephemeral diagnostic formats; Configuration contract regression for shared workspace settings.
- **Compatibility expectation**: preserve existing shared lifecycle; no second cache, clock, capacity rule, or MCP-only eviction path.

### 11. Errors, Malformed Evidence, Cancellation, and Inert Content

- **Type**: unit/integration/security
- **Location**: `internal/mcpadapter` trace/security tests, `internal/console/artifact_integration_test.go`, and existing trace parser/processor suites
- **Names**:
  - `TestTraceToolsPreserveEverySharedDomainErrorMeaning`
  - `TestMalformedTruncatedIncompatibleAndOversizedArtifactsPublishNoTraceResult`
  - `TestTraceCancellationClosesReadersSuppressesResultsAndReleasesPins`
  - `TestTraceContentCannotTriggerServerAuthorityOrAnotherOperation`
  - `TestTraceResultsAndErrorsNeverLeakPathsCredentialsOrInternalOwnerIDs`
- **What it proves**: exact errors for invalid arguments/cursors/artifacts, incompatibility, expiry/removal/in-use, limits, storage failure, target state, and sanitized console failure; no partial semantic evidence; prompt-injection boundary; no secret/path/internal cause leakage; cancellation through SDK/service readers.
- **Fixtures/data**: existing invalid corpus; malicious strings in skill YAML, route, error message, metadata, record data, payload, and raw NDJSON that name tools, URLs, local paths, commands, headers, and credential operations.
- **Mocks**: instrument every possible target/network/filesystem/config/credential/execution-control callback with counters that must remain zero except for the explicitly requested read path.
- **Contract classification**: Ephemeral diagnostic formats and Persisted or serialized MCP error contract.
- **Compatibility expectation**: preserve safe shared errors and current diagnostic visibility. Do not add redaction of application data or new inferred error meanings.

### 12. Exact Range Limit, Memory, Deadline, and Concurrency Matrix

- **Type**: unit/integration/benchmark
- **Location**: `loomspan-console/internal/traceanalysis/range_test.go`, `continuation_test.go`, new `range_benchmark_test.go`, and MCP HTTP integration tests
- **Names**:
  - `TestRangeCandidateSizesRoundTripUTF8AndWorstCaseBase64Exactly`
  - `TestRangeCandidateSizesContinueWithoutGapOrOverlap`
  - `TestRangeCancellationAtEveryCandidateSizeStopsPromptly`
  - `TestRangeOneByteOverSelectedMaximumReturnsLimitExceeded`
  - `TestMCPRangeSerializationMeetsDeadlineUnderBrowserConcurrency`
  - `BenchmarkMCPRangeMaterialization`
- **What it proves**: exact source-byte behavior at 1/4/16/32 MiB and conditionally 64 MiB; actual offsets; checksum equality; UTF-8/base64 representation; continuation; no silent clamp; prompt cancellation; bounded server memory/latency while browser and multiple MCP clients read concurrently.
- **Fixtures/data**: deterministic repetitive valid UTF-8 with multibyte boundary positions; deterministic arbitrary bytes that force base64; generated artifact/payload lengths one byte below, at, and one byte above each candidate/final limit. Record source checksum and returned source-byte count, not encoded JSON size alone.
- **Mocks**: real SDK/HTTP serialization and real artifact readers over temporary files. Use a fixed/no-op network because ranges read the installed copy. Benchmarks report allocations and bytes/op.
- **Contract classification**: Ephemeral diagnostic format and new serialized MCP framing contract.
- **Compatibility expectation**: one evidence-backed shared maximum at least 16 MiB; no retained 1 MiB default-by-accident or MCP-only lower clamp.

### 13. Official Protocol and Assembled Server Regression

- **Type**: integration/e2e
- **Location**: `loomspan-console/internal/mcpadapter/server_test.go`, `internal/buildtool/mcp_conformance.go`, and pinned conformance harness
- **Names**:
  - `TestMCPServerDiscoversCompletePR18Surface`
  - extend both supported-protocol black-box initialization/list/call tests
  - retain all security/lifecycle/tracker tests
- **What it proves**: twelve tools, seven total resource templates (one skill plus six trace), both new capabilities, structured outputs, stateless Streamable HTTP, request cancellation, no-store headers, authentication generation, and protocol negotiation remain correct through production assembly.
- **Fixtures/data**: production test server with deterministic target/imported evidence; pinned official conformance revision.
- **Mocks**: official runner through the production credential-injecting proxy; no fixture-only production tools/prompts/resources.
- **Contract classification**: Persisted or serialized MCP protocol/surface contracts.
- **Compatibility expectation**: preserve supported MCP `2025-11-25` and `2026-07-28`; no SDK-specific types leak below the adapter.

### 14. Post-Implementation Representative Local Client Matrix

- **Type**: post-implementation manual compatibility observation
- **Location**: `loomspan-console/docs/mcp-client-compatibility.md`
- **Matrix**: then-current stable local Codex desktop/CLI/IDE surfaces, Claude Code, Antigravity, Cursor, and Devin Desktop/Windsurf/Cascade or local Devin CLI.
- **What it proves**: authenticated connection; twelve-tool/schema discovery; structured and deterministic text rendering; shared domain `isError`; all six trace calls; target and no-target import flows; resource discovery/read when supported; 64-item continuation; every candidate UTF-8/base64 range; cancellation/reconnect where supported.
- **Fixtures/data**: one selected-target fixture server plus one imported same-version failure/chunked artifact; generated range artifacts with known checksums.
- **Mocks**: none at the client boundary. Use an isolated temporary Console profile/key, never a repository credential or committed authorization header.
- **Contract classification**: Persisted or serialized MCP contract compatibility observation.
- **Compatibility expectation**: one common surface. Record date, build, OS, configuration, observed protocol when available, and result for completed observations. Hosted clients remain out of scope. Results inform later compatibility work and do not reopen or block this implementation.

### 15. Documentation Evidence and Boundary Guards

- **Type**: documentation review plus automated architecture/build regression
- **Location**: `ai/skill-authoring/traces-and-debugging.md`, `ai/skill-authoring/README.md`, `loomspan-console/README.md`, existing Java architecture test, and build declaration tests
- **Names/checks**:
  - existing `LoomspanPublicSurfaceArchitectureTest`
  - existing build/declaration verification
  - manual LLM-first routed-document checklist
- **What it proves**: no Java application API/SPI/config/manifest expansion; no raw toggle; exact MCP/debugging claims point to passing tests; coverage row remains source-verified; an LLM reading routed guidance can distinguish source, availability, lifetime, ordinary versus raw inspection, uncertainty, sensitivity, and limitations.
- **Fixtures/data**: final tool/capability manifest, MCP goldens, client matrix, and passing test names become documentation anchors. Do not duplicate large example payloads in prose.
- **Mocks**: none.
- **Contract classification**: Application API negative guard, Configuration and manifest contract regression, and authoring documentation evidence.
- **Compatibility expectation**: supported Java surface and configuration remain unchanged; authoring guidance changes only after behavior is executable.

## How to Run

Run commands from the stated directory. Automated tests use temporary workspaces and fake upstream servers; they must not require a real Loomspan credential or write client keys into the repository.

### Red/Green Focused Sequence

From `loomspan-console`:

```powershell
go test ./internal/traceinventory
go test ./internal/traceanalysis
go test ./internal/mcpadapter
go test ./internal/browserapi ./internal/artifact
go test ./internal/console
```

Use `-run TestName -count=1` while developing an individual red slice. Preserve at least one observed pre-fix failure for each major slice in the PR notes; do not commit expected-failure skips.

### Full Console Verification

From `loomspan-console`:

```powershell
gofmt -w <changed-go-files>
go test ./...
go vet ./...
go run ./internal/buildtool verify
go run ./internal/buildtool mcp-conformance
npm --prefix web run typecheck
npm --prefix web run test
npm --prefix web run test:e2e
```

`go run ./internal/buildtool mcp-conformance` runs `npm ci` in an isolated harness and requires network/package access when the pinned dependencies are not already available.

### Race Verification

On the documented Windows development machine, from `loomspan-console`:

```powershell
$env:PATH = "C:\msys64\mingw64\bin;" + $env:PATH
$env:CGO_ENABLED = "1"
go test -race ./...
```

On other platforms, run `CGO_ENABLED=1 go test -race ./...` with a supported C compiler. The joined acquisition, lease, target-rotation, cancellation, multiple-client, and shutdown tests must run under the race detector.

### Java/Go Fixture and Public-Surface Regression

From the repository root on Windows:

```powershell
.\mvnw.cmd -pl loomspan-spring-boot-starter test -Dtest=ConsoleTraceFixtureCorpusTest -DfailIfNoTests=false
.\mvnw.cmd -q -Dtest=LoomspanPublicSurfaceArchitectureTest test
bash ai/scripts/spec_metadata.sh
```

Do not use the fixture-regeneration flag for this PR unless a separately approved Java-to-Go boundary change is made. A normal corpus run must leave `loomspan-console-fixtures` unchanged.

### Range Evidence

Run the exact-range tests without cache and record allocations:

```powershell
go test -count=1 -run "TestRangeCandidateSizes|TestMCPRangeSerialization" ./internal/traceanalysis ./internal/mcpadapter
go test -run "^$" -bench BenchmarkMCPRangeMaterialization -benchmem ./internal/traceanalysis ./internal/mcpadapter
```

Record source size, encoding, encoded response size, elapsed time, allocations/peak process memory, cancellation latency, concurrency, and checksum result for 1/4/16/32 MiB and the attempted 64 MiB case.

### Post-Implementation Manual Client Environment

This section is observational and runs after the automated exit criteria are
complete. It does not gate the PR, merge, or release.

- Use an isolated Console profile and transient workspace.
- Store the MCP key only in each client's documented protected/environment-backed user configuration. Prefer `LOOMSPAN_MCP_ACCESS_KEY` where supported.
- Never place the key in a repository file, URL, command argument, shell-history example, screenshot, fixture, or test log.
- Record no application or MCP credential in the compatibility document.
- Test on the operating systems/client builds recorded in the matrix; hosted clients are explicitly not applicable to loopback reachability.

## Exit Criteria

- [ ] The first target-free joined-inventory test exists and is observed failing before implementation, followed by passing behavior after the shared service is implemented.
- [ ] Each subsequent major slice—enriched facts/content references, six-tool discovery, target-free imported MCP access, and semantic capability manifest—has a focused red test before its implementation.
- [x] All focused and full Go tests pass post-fix, including `go test ./...`, `go vet ./...`, and `go run ./internal/buildtool verify`.
- [x] The race suite passes for joined acquisition, leases, cancellation, target rotation, multiple clients, authentication generation, and shutdown.
- [x] Official MCP conformance passes for both supported protocol revisions with the pinned runner.
- [x] Tool discovery returns exactly twelve tools and resource discovery returns exactly seven templates; every tool has the settled annotations and strict schema.
- [x] Both new capability IDs match the reviewed manifest, every required semantic fixture executes, and independent missing-tool/missing-fixture mutations fail conformance.
- [x] `ALL` and `IMPORTED` work without a selected target; `TARGET` requires one; imported evidence never carries/captures a target scope; wrong-source handles/cursors/references fail precisely.
- [x] Inventory traversal is deterministic and finite, suppresses installed/catalog duplicates, binds continuations to query/scope, and exposes catalog requested/available/error separately from local availability.
- [x] Enriched records/frames expose all required existing facts without adapter recomputation, adjacency inference, diagnosis, importance ranking, or aggregate completeness.
- [x] Payload and diagnostic references are opaque/source/handle-bound, and trace cursors expose no process-local imported owner ID. Old internal imported cursor forms are rejected without fallback.
- [x] Target/imported summary/frame/record resources materialize the same JSON DTOs and shared errors as tools; no raw-artifact resource exists.
- [ ] Browser and MCP agree on source identity, handles, availability, hierarchy, timing, usage, attempts, retries, validation, failures, gaps, uncertainties, ranges, and domain errors for the same neutral results/Java fixtures.
- [ ] Simultaneous browser/MCP acquisition produces one upstream stream, validation, installed copy, handle, and capacity charge; waiter cancellation remains independent.
- [ ] Successful reads refresh the shared TTL; failures and cancellations do not. TTL, capacity, removal, target rotation, shutdown, and restart invalidate exactly the intended source owners.
- [ ] Malformed, truncated, incompatible, oversized, unsafe, expired, removed, and in-use evidence fails with precise shared meanings and never publishes partial semantic results.
- [ ] Adversarial runtime content triggers no server-side shell, filesystem, network, credential, target-selection, configuration, execution-control, or additional MCP operation, and results leak no Console secrets, paths, internal causes, or imported owner IDs.
- [ ] Exact range tests reconstruct UTF-8 and worst-case base64 bytes without gaps/overlap at 1/4/16/32 MiB and the attempted 64 MiB case; one byte over the final maximum returns `LIMIT_EXCEEDED` without silent clamping.
- [ ] The selected shared maximum passes automated exactness, cancellation, deadline, bounded-memory, and simultaneous browser/MCP checks and is at least 16 MiB.
- [x] Java fixture corpus tests pass without regeneration or diff, exact release-string incompatibility tests remain green, and no Java-to-Go protocol change was introduced.
- [x] `LoomspanPublicSurfaceArchitectureTest` passes with an unchanged supported Java allowlist; no Java API, SPI, Spring extension point, configuration key, manifest field, raw toggle, legacy reader, shim, or dual behavior was added.
- [x] Existing browser fixture contracts and PR 17 MCP runtime/skill/execution/activity behavior remain green except for the intentional additive tool/resource/capability inventories.
- [x] Tests cited as evidence for `ai/skill-authoring/traces-and-debugging.md` all pass, the README coverage row is updated, and the changed guidance satisfies the LLM-first routed-document checklist.
- [ ] Manual target and target-free imported investigations, resource reads, broad continuations, lifecycle invalidation, and inert-content checks are complete.
