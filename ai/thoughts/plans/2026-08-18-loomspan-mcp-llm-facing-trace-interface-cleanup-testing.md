# LLM-Facing MCP Trace Interface Cleanup Testing Plan

## Change Summary

- Replace the MCP trace workflow `source -> traceId/artifactHandle -> source + artifactHandle` with `traceId` for discovery and every trace inspection operation.
- Add one internal resolver that selects a unique installed or acquirable current-target/imported evidence owner while preserving target rotation, owner isolation, acquisition single-flight, expiry, and per-analysis leases.
- Compact trace inventory to one row per `traceId`, explicit ambiguity, `complete`, and a safe `TRACE_DISCOVERY_INCOMPLETE` limitation.
- Remove target scope, runtime instance, source, artifact, resource, storage, retention, expiry, and size mechanics from all MCP-defined inputs/results/errors and text fallbacks.
- Remove all seven custom resource templates while retaining the same twelve read-only tools and capability IDs.
- Update semantic fixtures, packaged debugging guidance, skill-authoring guidance, Console documentation, and agent-evaluation cases to the trace-ID-first contract.

This is an approved pre-alpha replacement of an **Ephemeral diagnostic format** and **Internal or accidentally exposed implementation**. It is not a compatibility-preserving refactor. Tests must prove that the old MCP surface is absent, not that old and new schemas work simultaneously.

## Impacted Areas

- **Trace contract and handlers**: `loomspan-console/internal/mcpadapter/trace_contracts.go`, `traces.go`, `trace_contracts_test.go`, `traces_test.go`, `trace_range_http_test.go`.
- **Resolution orchestration**: new `loomspan-console/internal/traceresolution/service.go` and `service_test.go`; assembly in `internal/console/service.go`, `internal/agenteval/server.go`, and `internal/mcpadapter/server.go`.
- **Inventory merge and cursor behavior**: `loomspan-console/internal/traceinventory/dto.go`, `service.go`, `cursor.go`, and tests.
- **Cross-cutting MCP projections**: `internal/mcpadapter/contracts.go`, `runtime.go`, `skills.go`, `executions.go`, `activity.go`, golden fixtures, parity tests, and fallback-text tests.
- **Resource removal and discovery**: `internal/mcpadapter/server.go`, `server_test.go`; deletion of `resources.go`, `resources_test.go`, `trace_resources.go`, and `trace_resources_test.go`.
- **Current-run semantic promises**: `internal/mcpadapter/capabilities.go`, `contracts/trace-capabilities.json`, `capabilities_test.go`, and `trace_semantic_fixtures_test.go`.
- **Protected internal lifecycle**: `internal/artifact`, `internal/traceanalysis`, target publication/authentication-generation checks, and joined browser/MCP tests.
- **Protected browser projection**: `internal/browserapi` artifact/observability tests and MCP/browser parity/joined-adapter tests.
- **Agent evaluations**: `loomspan-console/agent-evals/cases`, `internal/agenteval/server_test.go`, and `fixtures_test.go`.
- **Authoring claims**: `ai/skill-authoring/traces-and-debugging.md` and the `Traces and debugging` coverage row in `ai/skill-authoring/README.md`; evidence comes from resolver, inventory, MCP contract, lifecycle, and joined-adapter tests rather than prose-only tests.
- **Documentation and packaged skill**: `loomspan-console/agent-skills/loomspan-runtime-debugging`, `loomspan-console/README.md`, `docs/mcp-client-compatibility.md`, and `mcp-conformance/README.md`.

## Risk Assessment

- **High — ambiguous identity silently resolves to the wrong owner**: a target/imported collision could expose unrelated evidence if target is preferred implicitly. Resolver and inventory tests must require `AMBIGUOUS_TRACE`/`ambiguous: true` and zero analysis calls.
- **High — target rotation publishes mixed evidence**: resolution may capture one target while analysis or response publication occurs after rotation. Tests must cover rotation before resolution, during acquisition, and after analysis but before publication.
- **High — expiry handling bypasses owner/lease guarantees**: retry-by-`traceId` must reuse or reacquire only through the artifact service. Tests must reject a global trace map, handle substitution, or additional lease behavior.
- **High — pagination splits one identity across pages**: installed/catalog or target/imported duplicates must be grouped or suppressed before a page boundary. Cursor fingerprints must still reject incompatible target/installed state.
- **High — internal identifiers leak through another DTO**: direct embedding of `consolecore.StatusSnapshot`, `consolecore.Details`, `live.Continuity`, or nested inventory objects can reintroduce removed fields. A semantic recursive property walker must cover every adapter-owned schema/envelope and report exact paths.
- **High — overly broad leak scanning rejects untrusted evidence**: YAML, activity `details`, record content, diagnostics, payloads, and raw bytes may legitimately contain strings matching forbidden field names. Contract tests must inspect adapter-owned property definitions, not recursively police arbitrary data values.
- **Medium — imported fallback hides target uncertainty**: fallback is allowed only with no selected target or authoritative target `NOT_FOUND`. Authentication, compatibility, target-change, transport, and other inability-to-check errors must not be converted into a successful imported selection.
- **Medium — inventory completeness becomes misleading**: `hasMore` and `complete` are independent. Empty complete results may support “none found”; empty incomplete results may not.
- **Medium — stale opaque-token recovery still mentions handles**: continuation and payload-reference internals remain owner/handle bound, but outward messages must instruct restart/refresh by `traceId` only.
- **Medium — resource deletion removes a tool capability accidentally**: tools and capability membership must remain complete while resource-template discovery becomes empty.
- **Medium — cross-cutting DTO cleanup drops useful facts**: runtime state enums, execution/session/trace facts, activity interval/reset/cursors, compatibility versions, limits, raw-download availability, trace facts, gaps, uncertainties, and range controls must remain.
- **Medium — performance regression**: imported-collision probing must occur only when needed; target-only resolution must not make a redundant `GetTrace` call; concurrent target resolution must retain artifact acquisition single-flight.
- **Low — Java or persisted trace compatibility drift**: no Java production, application-adapter route/problem, canonical NDJSON, or compatibility-marker change is planned. Any such diff is scope expansion and requires a separate coordination decision.

### Contract and Compatibility Expectations

| Surface | Test treatment |
| --- | --- |
| Application API | No planned delta. Do not add Java API compatibility tests solely for this Go MCP change. Confirm no Java production files changed; run `LoomspanPublicSurfaceArchitectureTest` only if they do. |
| Supported SPI | No SPI exists or is added. Resolver types stay under Go `internal/`; no extension-point tests or shims. |
| Configuration and manifest contracts | No executable change. Existing repository suites remain the regression signal; no new manifest compatibility fixture. |
| Persisted or serialized contracts | Canonical NDJSON, same-version portability, and Java-to-Go observability/artifact fixtures remain protected. Full existing tests must stay green; do not add historical MCP readers or old-schema fixtures. |
| Ephemeral diagnostic formats | Primary changed surface. Test current schemas, projections, ordering, ambiguity, completeness, recovery, failure visibility, security, and writer/reader/projector coherence. Old MCP schemas/resources are approved removals. |
| Internal or accidentally exposed implementation | Add focused resolver/inventory tests; update or delete old source/handle/resource tests atomically. Preserve artifact, analysis, browser, target, and authentication lifecycle tests. |

### Authoring Claims Requiring Evidence

- A developer or debugging agent can discover and inspect a unique finalized trace using `traceId` without source or handle decisions.
- A unique imported trace works without a selected target.
- A known target/imported collision is explicit and is never silently resolved.
- Incomplete discovery is distinguishable from pagination and prevents unsafe negative/uniqueness conclusions.
- Safe expiry recovery is transparent when possible; otherwise the caller receives `TRACE_UNAVAILABLE` without handle repair instructions.
- Stale continuations restart the query by `traceId`; stale payload references require a refreshed record descriptor by `traceId`.
- Target scope, instance, and artifact lifecycle remain enforced internally even though no MCP field exposes them.
- Tools alone remain a complete inspection path and no MCP resources are advertised.

Evidence for these claims comes from `internal/traceresolution/service_test.go`, `internal/traceinventory/service_test.go`, `internal/mcpadapter/trace_contracts_test.go`, `traces_test.go`, `trace_semantic_fixtures_test.go`, `server_test.go`, and joined browser/MCP lifecycle tests.

## Existing Test Coverage

Baseline verified on 2026-08-18:

```text
cd loomspan-console
go test ./internal/mcpadapter ./internal/traceinventory ./internal/artifact ./internal/traceanalysis ./internal/agenteval ./internal/browserapi
```

All listed packages passed before adding the new failing tests.

### Coverage to Preserve

- `internal/artifact/acquire_test.go`: concurrent waiter single-flight, installed-handle reuse, cancellation, atomic publication, transfer validation, storage limits, and shutdown.
- `internal/artifact/expiry_test.go` and `lease_test.go`: exact-deadline expiry, fresh reacquisition, deferred removal while leased, idle refresh, and lease ownership.
- `internal/artifact/import_test.go`: validated process-local imports, duplicate rejection, rotation independence, and concurrent duplicate import safety.
- `internal/traceanalysis/service_test.go`, `cursor_test.go`, and `content_ref_test.go`: one lease per analysis, parsed facts, ordering/pagination, owner/handle/query binding, stale cursor rejection, and payload-reference binding.
- `internal/mcpadapter/trace_range_http_test.go`: 16 MiB range bound, exact encoding, serialization deadline, and concurrent-client responsiveness.
- `internal/mcpadapter/lifecycle_test.go` and `security_test.go`: admission, cancellation, credential generation, shutdown, loopback, bearer, Origin, and request limits.
- `internal/mcpadapter/trace_joined_adapters_test.go` and `parity_test.go`: shared internal evidence with browser/MCP semantic alignment and browser-preserved facts.
- `internal/mcpadapter/capabilities_test.go`: complete tool families and reviewed semantic-fixture manifest.
- `internal/agenteval`: production-adapter harness, protected connection, fixture validation, and degradation classifications.
- `internal/browserapi/artifacts_test.go`: browser handle/storage contracts, target-change mapping, enrichment, raw streaming, and cancellation.

### Existing Tests to Replace or Update

- `TestTraceToolSchemasAreClosedBoundedAndUseSettledBranches` currently protects `source`, `artifactHandle`, and the old get-trace one-of branch. Replace those assertions with the new trace-ID-only schema contract while retaining closure and bounds.
- `TestImportedTraceToolsWorkWithoutSelectedTarget` currently supplies `IMPORTED` plus a handle. Keep the scenario but route every operation by `traceId` through a fake resolver.
- Delete `TestGetTraceRejectsInvalidSourceIdentifierBranches`; replace it with blank/oversized `traceId` and forbidden-property schema cases.
- `TestTraceCapabilityTargetAcquisitionFactsContinuationsAndBothSources` should become trace-ID resolution/continuation/fact projection coverage without caller-visible sources.
- `traceinventory/service_test.go` source-filter, storage-rich result, and catalog-status assertions encode approved obsolete internals. Rework them around unified inventory, compact completeness, ambiguity, and cursor stability.
- `server_test.go` currently requires seven templates and reads a skill resource. Replace those expectations with zero templates and successful equivalent tool calls.
- Delete `resources_test.go` and `trace_resources_test.go` with their production files; do not retain URI compatibility tests.
- Runtime, skill, execution, activity, contract, parity, and golden-fixture tests must stop expecting scope/instance/resource fields while continuing to prove retained domain facts.
- Semantic fixture IDs `trace.source-binding`, source-oriented acquisition, and artifact-expiry repair must be replaced atomically in both Go and `contracts/trace-capabilities.json`.

### Coverage Gaps

- No existing unit seam resolves only `traceId` across installed current-target evidence, imported evidence, and target acquisition.
- No test currently prevents a target/imported collision from being emitted or selected as two independent candidates.
- No semantic adapter-wide forbidden-property walker distinguishes owned DTO properties from arbitrary untrusted data.
- No current inventory result has `complete`, compact limitations, or explicit ambiguity.
- No current tests require trace-ID recovery wording for stale continuations/payload references or unrecoverable expiry.
- No test currently proves zero advertised resource templates while preserving all twelve tools/capabilities.

## Bug Reproduction / Failing Test First

- **Name**: `TestTraceToolSchemasExposeOnlyDeveloperIntentIdentity`
- **Type**: unit/contract
- **Location**: `loomspan-console/internal/mcpadapter/trace_contracts_test.go`
- **Arrange**: build the actual input schemas with the same helpers used by `addTraceTools`; normalize each schema into a property-name set plus required fields.
- **Act**: inspect the six trace schemas.
- **Assert**:
  - `LOOMSPAN_list_traces` has exactly `pageSize` and `continuation`.
  - Each of the other five tools has required bounded nonblank `traceId` and its existing question-specific fields.
  - None has `sourceFilter`, `source`, or `artifactHandle`.
  - Existing enum, page, token, range, and exactly-one `start`/`continuation` constraints remain.
- **Expected failure pre-fix**: the test reports exact extra/missing property paths because current schemas contain `sourceFilter`, `source`, and `artifactHandle`; `get_trace` also permits an artifact-handle branch, while the other trace tools lack `traceId`.
- **Why first**: it compiles against the current code, runs without filesystem/network fixtures, and directly proves the rejected model-facing contract before any new resolver type exists.
- **Contract classification**: Ephemeral diagnostic formats.
- **Compatibility expectation**: approved removal of the old pre-alpha MCP contract; no dual schema.

After capturing the red result, add two more contract sentinels before production changes:

1. `TestMCPDefinedContractsContainNoRejectedProperties` should fail with exact DTO/schema paths.
2. Update the existing server discovery test to expect zero resource templates; it should fail with the current count of seven.

Do not commit a phase where tests expect both seven and zero templates or accept both source/handle and trace-ID inputs.

## Tests to Add/Update

### 1. Trace input schemas are trace-ID only

- **Name**: `TestTraceToolSchemasExposeOnlyDeveloperIntentIdentity`
- **Type**: unit/contract
- **Location**: `loomspan-console/internal/mcpadapter/trace_contracts_test.go`
- **What it proves**: exact allowed/required properties for all six trace tools; unchanged bounds and question-specific controls; absence of source and handle routing.
- **Fixtures/data**: generated Go JSON schemas only.
- **Mocks**: none.
- **Contract classification**: Ephemeral diagnostic formats.
- **Compatibility expectation**: approved removal.

### 2. Adapter-owned schemas and envelopes reject every removed property

- **Name**: `TestMCPDefinedContractsContainNoRejectedProperties`
- **Type**: unit/contract
- **Location**: `loomspan-console/internal/mcpadapter/contracts_test.go`
- **What it proves**: no adapter-owned input/output/error property is named `sourceFilter`, evidence-routing `source`, `artifactHandle`, `targetScopeId`, `instanceId`, `resourceUri`, or `resources`; failure includes the exact JSON path.
- **Fixtures/data**: all twelve advertised input schemas plus representative success/error values for runtime, skills, executions, activity, trace inventory, summary, frames, records, payload range, and raw range.
- **Mocks**: existing fake providers/handlers.
- **Special boundary**: walker stops at explicitly arbitrary/untrusted values (`yaml`, activity `details`, record/payload/diagnostic/raw content) and a companion case places forbidden spellings inside those values to prove they are preserved rather than rewritten.
- **Contract classification**: Ephemeral diagnostic formats.
- **Compatibility expectation**: approved removal plus current-run security coherence.

### 3. Resolver rejects invalid identity before collaborators

- **Name**: `TestResolveRejectsBlankWhitespaceAndOversizedTraceIDBeforeLookup`
- **Type**: unit
- **Location**: new `loomspan-console/internal/traceresolution/service_test.go`
- **What it proves**: trace ID is nonblank and bounded before target capture, lookup, catalog, or acquisition.
- **Fixtures/data**: empty string, whitespace-only string, exactly accepted boundary, and one over `maxTraceTokenLength`.
- **Mocks**: call-counting artifact, catalog, and target fakes that fail the test if invoked for invalid input.
- **Contract classification**: Internal or accidentally exposed implementation.
- **Compatibility expectation**: current-run diagnostic coherence.

### 4. Resolver decision table for installed evidence

- **Name**: `TestResolveInstalledEvidenceDecisionTable`
- **Type**: table-driven unit
- **Location**: `loomspan-console/internal/traceresolution/service_test.go`
- **What it proves**:
  - unique current-target installation is reused without catalog or acquisition;
  - unique import succeeds without a selected target;
  - current-target plus import returns `AMBIGUOUS_TRACE` and no analysis/acquisition;
  - stale/expired lookup entries are not treated as usable;
  - a target entry for a non-current scope is never considered.
- **Fixtures/data**: `artifact.LookupResult` values with distinct owners/handles and call-count expectations.
- **Mocks**: narrow resolver collaborator fakes.
- **Contract classification**: Internal or accidentally exposed implementation.
- **Compatibility expectation**: protected owner/target safety.

### 5. Resolver acquisition and imported-fallback decision table

- **Name**: `TestResolveMissingInstalledEvidenceDecisionTable`
- **Type**: table-driven unit
- **Location**: `loomspan-console/internal/traceresolution/service_test.go`
- **What it proves**:
  - no import + selected target uses `Acquire` directly and does not redundantly call `GetTrace`;
  - import + authoritative target `NOT_FOUND` returns the import without acquisition;
  - import + successful target `GetTrace` returns `AMBIGUOUS_TRACE` without downloading;
  - import + authentication, compatibility, unavailable, limit, cancellation, or target-change error returns that error rather than silently falling back;
  - no target and no import returns `TRACE_UNAVAILABLE`;
  - target `NOT_FOUND` or unrecoverable `ARTIFACT_EXPIRED` with no import maps to `TRACE_UNAVAILABLE` with no handle wording;
  - other actionable target errors retain their stable code and allowed recovery details.
- **Fixtures/data**: one row per domain code and exact safe message assertions for ambiguity/unavailability.
- **Mocks**: call-counting fake target, artifacts, and catalog.
- **Contract classification**: Internal or accidentally exposed implementation.
- **Compatibility expectation**: current-run diagnostic coherence.

### 6. Resolver target-rotation and cancellation safety

- **Name**: `TestResolveNeverReturnsEvidenceAcrossTargetRotation`
- **Type**: unit/concurrency
- **Location**: `loomspan-console/internal/traceresolution/service_test.go`
- **What it proves**: rotation before capture, while catalog probing, and during acquisition yields `TARGET_CHANGED`/cancellation and no resolved value; imported evidence remains usable only in the explicitly allowed no-target/authoritative-not-found cases.
- **Fixtures/data**: controllable barriers/channels around fake catalog/acquisition, two target scopes.
- **Mocks**: synchronized target/artifact/catalog fakes; no sleeps.
- **Contract classification**: Internal or accidentally exposed implementation.
- **Compatibility expectation**: protected target isolation.

### 7. Unified inventory compactness and completeness

- **Name**: `TestInventoryCompletenessDecisionTable`
- **Type**: table-driven unit
- **Location**: `loomspan-console/internal/traceinventory/service_test.go`
- **What it proves**:
  - no selected target returns installed imports with `complete: true` because imports are the available evidence family;
  - selected target + successful installed/catalog checks returns `complete: true`;
  - selected target + catalog/probe failure returns useful items, `complete: false`, and exactly one `TRACE_DISCOVERY_INCOMPLETE` limitation/message;
  - `hasMore` remains independent of `complete`;
  - empty complete and empty incomplete results are distinguishable.
- **Fixtures/data**: compact entries, empty/nonempty catalogs, each upstream failure class.
- **Mocks**: extended fake catalog with `ListTraces` and `GetTrace`; fake target/artifact storage.
- **Contract classification**: Ephemeral diagnostic formats.
- **Compatibility expectation**: current-run diagnostic coherence.

### 8. Inventory emits one row per identity and never splits collisions

- **Name**: `TestInventoryConsolidatesTraceIdentityBeforePagination`
- **Type**: unit
- **Location**: `loomspan-console/internal/traceinventory/service_test.go`
- **What it proves**:
  - installed target/import owners with one ID become one `ambiguous: true` row;
  - imported/catalog collision is detected by `GetTrace` and appears once;
  - catalog suppresses every installed ID, not only target-installed IDs;
  - page size one cannot place ownership duplicates on separate pages;
  - ordinary unique rows retain deterministic installed-first/recent-first-within-segment order.
- **Fixtures/data**: equal and different finalized times, page size one, target/import/catalog duplicates, unique neighbors.
- **Mocks**: fake storage lookup and catalog with exact call counts.
- **Contract classification**: Ephemeral diagnostic formats.
- **Compatibility expectation**: current-run diagnostic coherence.

### 9. Inventory cursors remain opaque, bounded, and state-bound

- **Name**: `TestUnifiedInventoryContinuationRejectsChangedSelectionAndInstalledSet`
- **Type**: unit
- **Location**: `loomspan-console/internal/traceinventory/service_test.go` and existing `cursor` tests
- **What it proves**: wrong page size/fingerprint, target rotation, relevant installed-set removal, malformed token, unknown fields, and oversized token return `INVALID_CURSOR`; valid keyset continuation does not repeat/skip unaffected identities when a newer item is inserted.
- **Fixtures/data**: existing composite cursor fixtures adapted to unified query (no source filter).
- **Mocks**: mutable artifact snapshot and target fake.
- **Contract classification**: Internal or accidentally exposed implementation.
- **Compatibility expectation**: protected opaque-token safety, approved removal of source-filter fingerprinting.

### 10. Every trace handler resolves by trace ID exactly once

- **Name**: `TestTraceHandlersResolveTraceIDAndPreserveQuestionSpecificRequests`
- **Type**: table-driven unit
- **Location**: `loomspan-console/internal/mcpadapter/traces_test.go`
- **What it proves**: get, frames, records, payload, and raw handlers call the resolver once with the supplied `traceId`, pass the returned reference/handle internally, preserve filters/order/representation/inline/page/range/continuation fields, and perform exactly one analysis operation.
- **Fixtures/data**: fake resolved target and imported values; representative query/range inputs.
- **Mocks**: fake resolver and existing fake trace-analysis service with call/request capture.
- **Contract classification**: Ephemeral diagnostic formats.
- **Compatibility expectation**: approved outward removal with protected internal handle-based analysis.

### 11. Trace results retain semantic evidence and compact identity

- **Name**: `TestTraceResultsExposeCompactIdentityAndRetainDiagnosticFacts`
- **Type**: unit/golden
- **Location**: `loomspan-console/internal/mcpadapter/traces_test.go`
- **What it proves**: every trace result has only `traceId`, `sessionId`, and `observedAt` for identity while retaining summary counts/usage/gaps/uncertainties, frame facts, enriched record facts, bounded content, range positions, and continuations.
- **Fixtures/data**: existing rich fake `TraceSummary`, frame, record, payload, and raw results.
- **Mocks**: fake resolver/analysis.
- **Contract classification**: Ephemeral diagnostic formats.
- **Compatibility expectation**: current-run diagnostic coherence.

### 12. Stale continuation and content reference recovery is trace-ID based

- **Name**: `TestTraceOpaqueTokenErrorsGiveTraceIDRecoveryWithoutHandleTerms`
- **Type**: unit/integration
- **Location**: `loomspan-console/internal/mcpadapter/traces_test.go` and `trace_semantic_fixtures_test.go`
- **What it proves**:
  - owner/handle/query mismatch remains rejected;
  - stale/invalid continuation tells the caller to restart the query by `traceId`;
  - stale/mismatched payload reference tells the caller to re-query the record descriptor by `traceId`;
  - structured and text errors contain no handle/scope/instance fields or repair instructions.
- **Fixtures/data**: real trace-analysis cursor/content-reference tokens created for a different owner/handle/query.
- **Mocks**: real artifact/analysis harness for integration branch; fake domain errors for exact adapter mapping branch.
- **Contract classification**: Ephemeral diagnostic formats.
- **Compatibility expectation**: protected opaque-token binding with approved recovery-message replacement.

### 13. Expiry safely reacquires or reports trace unavailable

- **Name**: `TestTraceIDResolutionRecoversOnlyThroughArtifactLifecycle`
- **Type**: integration
- **Location**: `loomspan-console/internal/mcpadapter/trace_semantic_fixtures_test.go`
- **What it proves**: unpinned expired target evidence is reacquired through existing acquisition; pinned/unrecoverable expiry becomes `TRACE_UNAVAILABLE`; no stale handle is remapped; imported expiry does not invent a target owner.
- **Fixtures/data**: existing real semantic harness with controllable artifact clock/TTL and target fixture stream.
- **Mocks**: real artifact and trace-analysis services; controlled target loader/opener.
- **Contract classification**: Ephemeral diagnostic formats.
- **Compatibility expectation**: protected lifecycle safety.

### 14. Concurrent trace-ID calls retain single-flight and one lease per analysis

- **Name**: `TestConcurrentTraceIDCallsShareAcquisitionAndLeaseIndependently`
- **Type**: integration/concurrency
- **Location**: `loomspan-console/internal/mcpadapter/trace_semantic_fixtures_test.go` or `trace_joined_adapters_test.go`
- **What it proves**: concurrent first calls for one target trace produce one artifact acquisition/capacity charge, then each analysis call owns/closes its own lease; cancellation of one waiter does not cancel remaining callers.
- **Fixtures/data**: real artifact service and synchronized stream opener; existing capacity counters.
- **Mocks**: barrier-controlled loader/opener; no wall-clock sleeps.
- **Contract classification**: Internal or accidentally exposed implementation.
- **Compatibility expectation**: protected acquisition/lease behavior.

### 15. Target publication and MCP authentication generation remain enforced invisibly

- **Name**: `TestTraceHandlerSuppressesResultAfterTargetOrAuthenticationGenerationChanges`
- **Type**: unit/integration
- **Location**: `loomspan-console/internal/mcpadapter/traces_test.go` and existing server lifecycle tests
- **What it proves**: rotation after resolution/analysis but before publication returns `TARGET_CHANGED`; credential regeneration suppresses the admitted result; neither error leaks old/current scope or instance identifiers.
- **Fixtures/data**: controllable fake analysis and existing authenticator generation helpers.
- **Mocks**: target context with rotation barrier; fake credentials.
- **Contract classification**: Internal or accidentally exposed implementation.
- **Compatibility expectation**: protected security path.

### 16. Runtime, skills, executions, activity, and errors use allowlisted projections

- **Names**:
  - `TestRuntimeOutputGoldenAndTextAgreeWithoutOwnershipIdentifiers`
  - `TestListAndGetSkillsGoldenContainNoResourceOrOwnershipFields`
  - `TestExecutionGoldensRetainSessionAndTraceWithoutOwnershipFields`
  - `TestExecutionActivityMapsContinuityWithoutOwnershipFields`
  - `TestDomainErrorMapsOnlyAllowedRecoveryDetails`
- **Type**: unit/golden
- **Locations**: existing `runtime_test.go`, `skills_test.go`, `executions_test.go`, `activity_test.go`, `contracts_test.go`, and `testdata/*.json`
- **What they prove**: adapter-owned projections retain observation/state/domain facts but omit scope/instance/resource fields; error details retain compatibility versions, limit name/value, and raw-download availability while removing current target and transport category.
- **Fixtures/data**: existing golden fixtures plus a fully populated internal status/details/continuity value to ensure denylisted internal fields do not leak.
- **Mocks**: existing providers/live service.
- **Contract classification**: Ephemeral diagnostic formats.
- **Compatibility expectation**: approved removal with retained domain meaning.

### 17. Arbitrary untrusted values are not rewritten by cleanup

- **Name**: `TestRejectedPropertySpellingsInsideUntrustedEvidenceRoundTripUnchanged`
- **Type**: unit/integration
- **Location**: `loomspan-console/internal/mcpadapter/contracts_test.go`, `skills_test.go`, and semantic fixture suite
- **What it proves**: YAML, activity `details`, record content, diagnostic text, payload text/base64, and raw bytes containing strings like `artifactHandle` or `targetScopeId` are returned unchanged and inert.
- **Fixtures/data**: adversarial sentinel values in each arbitrary content family.
- **Mocks**: existing fake services/semantic harness.
- **Contract classification**: Ephemeral diagnostic formats.
- **Compatibility expectation**: protected diagnostic accuracy and security boundary.

### 18. Server advertises twelve tools, unchanged capabilities, and zero resources

- **Name**: update `TestCompatible2025ProtocolInitializesListsAndCallsRealRuntimeTool` and `TestStatelessStreamableHTTPInitializesListsAndCallsRuntime`
- **Type**: HTTP/SDK integration
- **Location**: `loomspan-console/internal/mcpadapter/server_test.go`
- **What it proves**: both supported MCP revisions initialize; exactly twelve tool names/annotations remain; six capability IDs remain; `resources/templates/list` returns zero Loomspan templates; equivalent skill and trace evidence remains callable through tools.
- **Fixtures/data**: existing protected stateless HTTP server and skill/trace fixtures.
- **Mocks**: production MCP SDK server with test collaborators.
- **Contract classification**: Ephemeral diagnostic formats.
- **Compatibility expectation**: approved resource removal; protected tool/capability path.

### 19. Capability semantic manifest describes trace-ID behavior

- **Name**: keep `TestPR18CapabilityManifestMatchesReviewedDescriptorAndRejectsIndependentGaps` and update `TestPR18SemanticFixtures`
- **Type**: unit/integration manifest suite
- **Location**: `internal/mcpadapter/capabilities_test.go`, `trace_semantic_fixtures_test.go`, and `contracts/trace-capabilities.json`
- **What it proves**: every advertised trace semantic fixture ID maps to a real test; new fixtures cover trace-ID resolution, ambiguity, completeness, unavailable evidence, opaque-token recovery, lifecycle, concurrency, joined adapters, and schema errors; removed source/resource semantics are absent.
- **Fixtures/data**: reviewed JSON capability manifest and real semantic harness.
- **Mocks**: existing harness.
- **Contract classification**: Ephemeral diagnostic formats.
- **Compatibility expectation**: current-run diagnostic coherence.

### 20. Browser contracts remain storage/owner rich and behaviorally unchanged

- **Names**:
  - update `TestBrowserAndMCPJoinOneAcquisitionHandleAndCapacityCharge`
  - retain browser artifact/enrichment/raw-download tests
  - update MCP/browser parity tests to compare retained semantic facts while allowing intentional projection differences
- **Type**: integration/regression
- **Location**: `internal/mcpadapter/trace_joined_adapters_test.go`, `parity_test.go`, and `internal/browserapi/*_test.go`
- **What it proves**: browser APIs still accept/return source, scope, handle, storage, and retention facts; MCP omits them; both share the same installed artifact and trace semantic data.
- **Fixtures/data**: existing browser fixtures and real artifact service.
- **Mocks**: existing paired browser router/MCP server harness.
- **Contract classification**: Internal or accidentally exposed implementation (protected in-repository browser consumer).
- **Compatibility expectation**: protected browser path; approved MCP projection break only.

### 21. Old resource paths and tests are removed atomically

- **Name**: covered by server discovery and repository leak audit; delete resource-specific unit tests rather than renaming them into compatibility tests.
- **Type**: integration plus static review
- **Location**: `internal/mcpadapter/server_test.go`; delete `resources_test.go` and `trace_resources_test.go`.
- **What it proves**: no resource registration/URI helper/result link survives and no replacement `loomspan://traces/{traceId}` exists.
- **Fixtures/data**: resource-template list from real server.
- **Mocks**: none beyond server harness.
- **Contract classification**: Internal or accidentally exposed implementation.
- **Compatibility expectation**: approved removal.

### 22. Agent-evaluation cases require only the new model-facing concepts

- **Names**:
  - `TestEvaluationCasesAreVersionedUniqueAndWorkflowLinked` (updated expected case set)
  - `TestTraceInterfaceEvaluationCasesUseOnlyLLMFacingIdentifiers` (new)
  - `TestEvaluationServerRunsTraceIDOnlyMCPScenarios` (new/update)
- **Type**: unit/integration
- **Location**: `internal/agenteval/fixtures_test.go`, `server_test.go`, and `agent-evals/cases/*.json`
- **What it proves**: ambiguity case exists; evidence expiry expects reacquisition/`TRACE_UNAVAILABLE`; target change retains safety without IDs; compatibility/auth/activity cases no longer require scope/instance; MCP-without-skill relies only on tools; the production evaluation adapter accepts trace-ID-only calls.
- **Fixtures/data**: updated cases plus new `ambiguous-trace.json`; existing trace corpus.
- **Mocks**: production evaluation server's deterministic target/import fixtures.
- **Contract classification**: Ephemeral diagnostic formats.
- **Compatibility expectation**: current-run diagnostic coherence and approved removal.

### 23. Range bounds, parsed facts, canonical artifacts, and Java-to-Go fixtures remain unchanged

- **Name**: retain existing artifact/analysis/range/fixture suites without old MCP identity assertions.
- **Type**: regression/integration
- **Location**: `internal/artifact`, `internal/traceanalysis`, `internal/mcpadapter/trace_range_http_test.go`, Console fixture corpus tests.
- **What it proves**: 16 MiB per-call maximum, exact bytes/base64, parser semantics, trace facts, gaps/uncertainties, same-version portability, and application acquisition inputs remain intact.
- **Fixtures/data**: existing canonical NDJSON and application REST/artifact fixtures.
- **Mocks**: existing fixture harnesses.
- **Contract classification**: Persisted or serialized contracts plus Ephemeral diagnostic formats.
- **Compatibility expectation**: protected path; no Java-to-Go boundary change.

## Test Implementation Order

1. Run and record the green baseline focused suites.
2. Add `TestTraceToolSchemasExposeOnlyDeveloperIntentIdentity`; run it alone and record the expected red property diff.
3. Add the recursive forbidden-property test and zero-resource discovery expectation; confirm both fail for the intended old surface.
4. Add the `traceresolution` service skeleton plus decision-table unit tests; implement until only resolver tests pass.
5. Rewrite inventory tests for unified compact semantics, then implement inventory changes.
6. Update handler unit tests and schemas to trace-ID-only; keep semantic fixture tests red until end-to-end mapping is complete.
7. Update cross-cutting DTO/golden tests and error/continuity allowlists.
8. Remove resources and old resource tests atomically; update server discovery tests.
9. Update semantic capability manifest, joined-adapter, concurrency, expiry, and stale-token integration tests.
10. Update agent-evaluation fixtures/server tests and documentation evidence anchors.
11. Run focused packages, full Go tests, conformance, static leak audit, and manual MCP walkthroughs.

Each intermediate red run must fail for the next intentionally unimplemented behavior, not from stale fixture compilation or simultaneous old/new expectations.

## How to Run

Run from `loomspan-console/` unless stated otherwise.

### Baseline and Failing-Test Capture

```text
go test ./internal/mcpadapter ./internal/traceinventory ./internal/artifact ./internal/traceanalysis ./internal/agenteval ./internal/browserapi
go test ./internal/mcpadapter -run '^TestTraceToolSchemasExposeOnlyDeveloperIntentIdentity$' -count=1
```

The first command should be green before test edits. The second must be red against the old implementation and include the unexpected/missing schema properties.

### Focused Development Loops

```text
go test ./internal/traceresolution -count=1
go test ./internal/traceinventory -count=1
go test ./internal/mcpadapter -count=1
go test ./internal/artifact ./internal/traceanalysis -count=1
go test ./internal/agenteval ./internal/browserapi -count=1
```

Use the race detector for the new orchestration and existing concurrent acquisition/adapter tests:

```text
go test -race ./internal/traceresolution ./internal/traceinventory ./internal/artifact ./internal/mcpadapter
```

### Full Verification

```text
go test ./...
go run ./internal/buildtool mcp-conformance
```

No profiles or environment variables are required for deterministic Go tests. The conformance harness creates an isolated profile/credential and invokes its pinned runner; it requires the repository's normal Go/runner dependencies and must not print or pass the generated key on a command line.

If shared browser code or fixtures change:

```text
cd web
npm run typecheck
npm test
```

Return to the repository root for the final scope check. Java testing is not required when the diff contains no Java production or Java-to-Go fixture changes. If such a file appears unexpectedly, stop and reassess boundary coordination; after approval run at minimum:

```text
./mvnw -pl loomspan-spring-boot-starter -Dtest=LoomspanPublicSurfaceArchitectureTest test
```

### Static Contract Audit

Search the MCP adapter, packaged skill, Console MCP docs, skill-authoring guide, and agent-evaluation cases for:

```text
sourceFilter
artifactHandle
targetScopeId
instanceId
resourceUri
ARTIFACT_EXPIRED
loomspan://targets/
loomspan://imports/
```

Inspect every remaining occurrence. Allowed hits are internal lifecycle/browser contracts, arbitrary untrusted fixture content, historical ticket/research/plan artifacts, or explicit negative assertions. A bare `source` search is too broad; the semantic schema walker is authoritative for rejecting an MCP evidence-routing property named `source`.

## Manual Verification

1. Inspect actual `tools/list` JSON for all trace schemas and confirm every operation after listing requires `traceId` plus only question-specific fields.
2. Inspect `resources/templates/list` and confirm no Loomspan custom template is advertised.
3. Execute target trace, target-free imported trace, missing trace, target/import collision, expired trace, target rotation, stale continuation, and stale payload-reference scenarios through the production MCP adapter.
4. For every scenario, compare structured JSON and text fallback for the same domain facts and absence of rejected identifiers.
5. Verify complete versus incomplete empty inventory conclusions and `hasMore` independence.
6. Read one large payload and one raw artifact through multiple bounded ranges; confirm exact content and continuation behavior.
7. Use the packaged skill once and run an MCP-only equivalent once; both paths must select tools by `traceId` without resource/handle/source instructions.
8. Record the representative live-LLM walkthrough described by the ticket after the PR's deterministic tests pass. This is product-evaluation evidence, not a substitute for automated correctness tests and not a reason to broaden this PR.

## Exit Criteria

- [x] The focused pre-change baseline is recorded green.
- [x] `TestTraceToolSchemasExposeOnlyDeveloperIntentIdentity` is captured red against the old implementation for the expected source/handle/trace-ID property mismatch.
- [x] The recursive contract and zero-resource tests are captured red against the old adapter before production changes.
- [x] All resolver decision-table tests pass, including installed reuse, target-free import, acquisition, authoritative not-found fallback, ambiguity, unavailable evidence, expiry translation, cancellation, and target rotation.
- [x] Resolver collaborator call counts prove no redundant target metadata probe for target-only acquisition and no analysis/acquisition after known ambiguity.
- [x] Inventory tests prove one row per `traceId`, ambiguity before page boundaries, deterministic pagination, compact limitations, and independent `complete`/`hasMore` semantics.
- [x] All five trace inspection tools resolve by `traceId` exactly once and retain their filters, representation, pagination, and range controls.
- [x] Stale continuation/payload-reference and expiry errors provide trace-ID recovery without handle/scope/instance terms.
- [x] Runtime, skill, execution, activity, inventory, trace, and error structured/text results contain no rejected adapter-owned properties.
- [x] Arbitrary untrusted YAML/activity/record/diagnostic/payload/raw content containing rejected spellings round-trips unchanged.
- [x] Exactly twelve tools and the same capability IDs remain; no custom resource template or replacement resource URI is advertised.
- [x] Old source/handle/resource tests and production paths are removed rather than retained behind aliases, optional legacy fields, fallbacks, or dual behavior.
- [x] Artifact acquisition, expiry, lease, import, capacity, and cancellation suites remain green.
- [x] Target publication and MCP authentication-generation checks remain green without leaking internal identifiers.
- [x] Browser APIs retain their current source/scope/handle/storage behavior and joined browser/MCP tests share one underlying artifact.
- [x] Canonical NDJSON, application REST/artifact fixtures, exact compatibility-marker behavior, range bounds, and parsed diagnostic facts remain green without a Java-to-Go protocol change.
- [x] Updated semantic capability manifest and every referenced semantic fixture pass.
- [x] Agent-evaluation cases load, include explicit ambiguity, and no longer require removed identifier classes or resource/handle repair.
- [x] Tests cited as evidence for `ai/skill-authoring/traces-and-debugging.md` establish every updated trace-ID workflow and recovery claim.
- [x] `go test -race ./internal/traceresolution ./internal/traceinventory ./internal/artifact ./internal/mcpadapter` passes.
- [x] `go test ./...` passes.
- [x] `go run ./internal/buildtool mcp-conformance` passes for both supported protocol revisions.
- [x] Static contract audit classifies every remaining removed-term occurrence as allowed internal/browser/history/arbitrary-data/negative-test usage.
- [ ] Manual production-adapter scenarios and structured/text inspection are complete.
- [x] No Java production, public API, SPI, configuration/manifest, Java-to-Go route/problem, or canonical trace-format delta was introduced. If one appears, this plan is no longer sufficient and implementation stops for a coordinated scope decision.

## References

- Implementation plan: `ai/thoughts/plans/2026-08-18-loomspan-mcp-llm-facing-trace-interface-cleanup.md`
- Ticket: `ai/thoughts/tickets/loomspan-mcp-llm-facing-trace-interface-cleanup.md`
- Research: `ai/thoughts/research/2026-08-18-loomspan-mcp-llm-facing-trace-interface-cleanup.md`
- Skill-authoring guidance: `ai/skill-authoring/traces-and-debugging.md`
- Framework compatibility lens: `ai/thoughts/framework-feature-design-lens.md`
