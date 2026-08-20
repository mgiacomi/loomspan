# PR 31 MCP Trace Usability, Retry Correctness, and Bounded Delivery Testing Plan

## Change Summary

- Correct trace `retryCount` and frame `directRetryCount` to count validated attempts after the initial attempt rather than retry-sequence identities.
- Add a complete nonzero physical `recordCountsByType` histogram to trace summary while keeping logical failures, validations, gaps, uncertainties, usage completeness, and terminal outcome independent.
- Replace plural frame `outcomes` with optional scalar `outcome` across analysis, MCP, browser API, fixtures, and TypeScript consumers.
- Make COMPACT/DETAILED projection behavior, bounded `inlineContent`, lifecycle timestamps, and pagination semantics discoverable in `tools/list`.
- Allow the first semantic-content or raw-artifact read to omit both `start` and `continuation`, defaulting to source offset zero; continue rejecting both together.
- Replace item-count-only/default fallback clipping with complete-item encoded-response admission, lossless opaque continuations, and identical structured/fallback pages.
- Select safer default range sizes from measured complete MCP results while retaining exact explicit reads through 16 MiB.
- Permit the discovery response to exceed 20 KiB when required semantics need it, but enforce the smallest committed KiB ceiling that fits and never exceed the approved 25 KiB (25,600-byte) maximum.
- Update the Console README, packaged Agent Skill guide, and routed skill-authoring trace guidance using executable behavior as evidence.

## Impacted Areas

- **Trace processing and derived facts**: `loomspan-console/internal/traceanalysis/processor.go`, `attempts.go`, `frames.go`, `model.go`, `manifest.go`, `dto.go`, `query_facts.go`, and `query_frames.go`.
- **Trace traversal and exact reads**: `query_frames.go`, `query_records.go`, `search.go`, `query_ranges.go`, `range.go`, `continuation_test.go`, `service_test.go`, `search_test.go`, and `range_test.go`.
- **Inventory traversal/lifecycle**: `loomspan-console/internal/traceinventory/service.go`, `dto.go`, and `service_test.go`.
- **MCP contract and delivery**: `loomspan-console/internal/mcpadapter/trace_contracts.go`, `traces.go`, `output_schemas.go`, `server.go`, new response-budget helpers, and associated schema/HTTP/adapter tests and snapshots.
- **Browser projection consumers**: `loomspan-console/internal/browserapi/trace_analysis.go`, browser contract/trace-analysis tests, `loomspan-console/browser-fixtures/trace-analysis/{summary,frames}.json`, `loomspan-console/web/src/api/contracts.ts`, `TraceTimeline.tsx`, and related component tests.
- **Same-version fixtures**: `loomspan-console-fixtures/traces/`, `loomspan-console-fixtures/expected/`, Go fixture-corpus tests, and the Java `ConsoleTraceFixtureCorpusTest` coherence check.
- **Author-facing guidance**: `loomspan-console/README.md`, `loomspan-console/agent-skills/loomspan-runtime-debugging/references/mcp-tool-guide.md`, `ai/skill-authoring/traces-and-debugging.md`, and the `Traces and debugging` coverage row in `ai/skill-authoring/README.md`.
- **Evaluation evidence**: PR 31 cases/results under `loomspan-console/agent-evals/` and host observations in `loomspan-console/docs/mcp-client-compatibility.md` or the checked-in response-size note selected during implementation.

## Risk Assessment

### High-risk behavior

- A retry fix can accidentally count initial attempts, count incomplete/unvalidated records, conflate `PLAN_RETRY_REQUESTED`, or misattribute a later attempt when its initial attempt belongs to another frame.
- Histogram collection can omit physical chunk records, double-count reconstructed logical envelopes, accept unknown keys, overflow totals, or tempt adapters to derive unrelated semantic counts from physical records.
- Scalar outcome conversion can preserve the obsolete plural path accidentally, turn absent/blank close status into a fabricated value, or desynchronize MCP and browser consumers.
- Byte admission can advance past a rejected item, duplicate or lose matches after continuation, alter continuation fingerprints, return an empty successful loop, or make inventory `complete` mean pagination completion.
- Independently generated fallback text can drift from structured output, omit facts, exceed the intended result ceiling, or split UTF-8/base64/escaped JSON content.
- Range-default reduction can silently clamp explicit requests, change source offsets, or make continuation reconstruction byte-inexact.
- Discovery prose can exceed the approved 25 KiB ceiling, delete necessary semantics to save bytes, or produce a stale exact snapshot.
- Guidance can describe the intended behavior before focused tests actually establish it.

### Edge cases

- Ten independent retry sequences with one successful initial attempt each.
- One sequence with three attempts; multiple sequences with 1/3 attempts; recovered failure with final success.
- Initial and later attempts in the same frame versus different frames.
- Planning retry records with no model retry.
- Physical payload chunks, multiple warning/failure/validation records, and omitted zero-count types.
- Unclosed frame, closed frame with blank status, and duplicate close rejection.
- COMPACT absent optional fields versus legitimate numeric zero in DETAILED.
- Omitted range controls, explicit `start: 0`, continuation-only, both controls, end-of-source, invalid UTF-8 boundary, and binary base64.
- Candidate item exactly below, exactly at, and one byte above a page budget; first-item-too-large behavior.
- Long opaque continuations, JSON control characters, quotes/backslashes, multibyte UTF-8, base64 expansion, and 64-item caller maxima.
- Empty result pages, skipped nonmatching records between accepted matches, search KMP state, page-local content descriptors, and inline aggregate selection after early pagination.
- Target/import inventory collision, incomplete catalog work, acquired-after-finalized ordering, and imported evidence without a target.

### Contract and compatibility scope

| Surface | Test obligation |
| --- | --- |
| Application API | No Java application API change is planned. Do not add compatibility tests or Java signatures. If Java production becomes necessary, stop and revise the implementation/testing plans before editing it. |
| Supported SPI | No supported SPI exists or changes; add no extension-point tests. |
| Configuration and manifest contracts | Preserve the packaged Agent Skill name and `1.0.0` metadata; validate the updated guide through the existing package validator and archive pipeline. YAML skill syntax is unchanged. |
| Persisted or serialized contracts | Preserve raw NDJSON vocabulary and exact `consoleCompatibilityVersion` rejection. The analysis manifest is transient; test the new current-process shape and reject corrupt histograms, but do not add legacy readers or migration tests. |
| Ephemeral diagnostic formats | Test current writer/reader/projector/MCP/browser coherence, diagnostic accuracy, ordering, completeness, security, and exact recovery. Approve removal of plural `outcomes`; do not test old and new MCP/browser fields simultaneously. |
| Internal or accidentally exposed implementation | Replace retry-sequence cardinality, plural outcome storage, and clipped fallback behavior in place. Tests should assert obsolete paths are absent, not preserved behind aliases or fallbacks. |

- **Java-to-Go boundary coordination**: Not required because Java record vocabulary, writer semantics, application-adapter REST/SSE, acquisition/problem boundaries, and compatibility marker do not change. Run the Java fixture corpus unchanged as a coherence check. Test-only expected semantic assertions may be updated only for intentionally changed derived Console facts.
- **Protected paths**: exact same-version artifact validation, closed record vocabulary, opaque owner-bound continuations/content references, read-only annotations, inert diagnostic content, exact explicit ranges, inventory completeness, and 12 tools with zero custom resources/templates.
- **Approved obsolete paths**: retry-sequence count masquerading as `retryCount`, `frame.outcomes`, exactly-one initial range control, and fallback byte slicing. No compatibility aliases or dual behavior are permitted.

## Existing Test Coverage

### Relevant coverage to preserve or extend

- `traceanalysis/calculations_test.go` validates attempt identity/lifecycle, frame hierarchy/duration, usage aggregation, retry-sequence usage, failure recovery, and duplicate frame close rejection.
- `traceanalysis/service_test.go` covers summary mapping, COMPACT/DETAILED frame projection, inline 8/32 KiB boundaries, frame/record pagination, filters, and enriched attempt/retry facts.
- `traceanalysis/continuation_test.go` proves cursor binding, no duplicate/omitted items across current item-count pages, search boundary state, exact range defaults/maxima, UTF-8/base64 offsets, and finite fixture traversal.
- `traceanalysis/range_test.go` covers semantic/raw exact reads, continuations, 16 MiB maximum behavior, cancellation, allocation, encoding, and one-byte lossless traversal.
- `traceanalysis/fixture_corpus_test.go` compares current Go analysis to the Java-produced same-version corpus and exercises plan/content/tool lifecycle facts.
- `traceinventory/service_test.go` covers consolidation, completeness, independent finalized/acquired/imported filtering and ordering, ambiguity, and continuation stability.
- `mcpadapter/trace_contracts_test.go` protects closed input schemas, developer-intent fields, range bounds, and closed vocabularies.
- `mcpadapter/output_schemas_test.go` protects compact discovery schemas and complete generated-output validation.
- `mcpadapter/traces_test.go` covers mapping, deterministic fallbacks, compact omissions, binary inline transport, representative DTO budgets, handler forwarding, and opaque-token recovery.
- `mcpadapter/server_test.go` measures the full HTTP `tools/list`, checks an exact snapshot, asserts all 12 tools, and verifies zero custom resource templates.
- `mcpadapter/trace_range_http_test.go` covers full HTTP range serialization, maximum-range performance/allocation, and exact byte preservation.
- `browserapi/trace_analysis_test.go`, `browserapi/contracts_test.go`, browser fixtures, and `TraceExplorer`/`TraceViews` tests protect browser projection parity and inert rendering.
- `agentskills/validate_test.go` validates the canonical packaged skill and rejects removed/unsafe workflows.

### Current gaps

- No case distinguishes retry-sequence cardinality from actual retries across many single-attempt sequences.
- No direct retry test attributes attempt one and attempt two in one sequence to different frames.
- No physical record histogram exists or is checked against every physical record.
- Tests preserve plural frame outcomes rather than proving the zero-or-one scalar invariant.
- Range contract tests require exactly one initial control and therefore encode the behavior to remove.
- Discovery tests expose `inlineContent`, COMPACT/DETAILED enums, and lifecycle properties without proving locally understandable descriptions.
- Existing size tests serialize neutral DTOs or fallback text separately; they do not measure full MCP success responses for the ticket matrix.
- No encoded-byte page admission or oversized-single-item behavior exists.
- Existing continuation tests cover item-count pages, not byte-budget stops at each candidate boundary.
- Fallback tests assert independent clipping, not structured/fallback page equivalence.
- No checked-in Codex/second-host threshold or tools-only versus skill-assisted PR 31 walkthrough exists.

## Bug Reproduction / Failing Test First

- **Name**: `TestRetryCountCountsAttemptsAfterInitialNotRetrySequences`
- **Type**: Unit
- **Location**: `loomspan-console/internal/traceanalysis/calculations_test.go`
- **Arrange**: Build one valid finalized trace with ten independent retry sequences. Each sequence contains exactly one `MODEL_REQUEST_SENT` followed by one successful `MODEL_RESPONSE_RECEIVED`, all with `attemptNumber=1`, `attemptReason=INITIAL`, and distinct explicit IDs.
- **Act**: Process the trace and read the manifest/summary.
- **Assert**: `attemptCount == 10`, `retryCount == 0`, and all retry-sequence usage aggregates remain available for their existing purpose.
- **Expected failure (pre-fix)**: Current `RetryCount: len(retryResults)` returns `10`, proving that sequence count is being reported as actual retries.
- **Mocks**: None. Use the existing in-memory artifact/test trace builders so the real validator and processor run.

Add this as the first red test. Before production changes, add the independent failing contract tests for scalar outcome, omitted initial range controls, histogram availability, COMPACT fallback omission, and structured/fallback page coherence. Do not bundle all defects into one opaque integration failure.

## Tests to Add/Update

### 1. `TestRetryCountCountsAttemptsAfterInitialNotRetrySequences`

- **Type**: Unit, table-driven
- **Location**: `loomspan-console/internal/traceanalysis/calculations_test.go`
- **What it proves**: The formula yields 1/0, 2/1, 3/2, ten independent 10/0, and mixed 4/2; `PLAN_RETRY_REQUESTED` does not affect model retry counts; recovered failure can coexist with trace success.
- **Fixtures/data**: Existing valid record builders with explicit attempt/retry identity; add a planning retry record in the non-effect row.
- **Mocks**: None.
- **Contract classification**: Ephemeral diagnostic formats.
- **Compatibility expectation**: Current-run diagnostic coherence; replace the obsolete sequence-cardinality assertion rather than preserve it.

### 2. `TestFrameDirectRetryCountUsesAttributedLaterAttempts`

- **Type**: Unit/integration
- **Location**: `loomspan-console/internal/traceanalysis/calculations_test.go` and `service_test.go`
- **What it proves**: A retry in the same frame counts once; a later attempt in frame B counts only in B even if its initial attempt is in frame A; a frame containing only an initial attempt reports zero.
- **Fixtures/data**: Valid two-frame traces with one shared `retrySequenceId` and explicit attempt numbers.
- **Mocks**: None; query the persisted frame projection through the real analysis service.
- **Contract classification**: Ephemeral diagnostic formats.
- **Compatibility expectation**: Correct the current field value without renaming it.

### 3. `TestPhysicalRecordHistogramIsCompleteAndSumsToRecordCount`

- **Type**: Unit/integration
- **Location**: `loomspan-console/internal/traceanalysis/processor_test.go`, `index_test.go`, and `fixture_corpus_test.go`
- **What it proves**: Every validated physical record, including payload chunks, increments exactly one closed record-type bucket; zero types are omitted; the overflow-safe sum equals `recordCount`; warning, validation, attempt failure, tool failure, and terminal counts are exact.
- **Fixtures/data**: Representative synthetic valid trace plus every valid committed corpus trace. Use physical NDJSON line counts as the independent oracle for corpus assertions.
- **Mocks**: In-memory artifact sink only.
- **Contract classification**: Ephemeral diagnostic formats for the trace; Internal implementation for the manifest.
- **Compatibility expectation**: Current-run writer/reader coherence; no durable manifest migration.

### 4. `TestManifestRejectsInvalidRecordHistogram`

- **Type**: Unit
- **Location**: `loomspan-console/internal/traceanalysis/index_test.go` or `service_test.go`
- **What it proves**: Unknown record-type keys, zero/negative values, integer overflow, and a sum different from `recordCount` are rejected before partial evidence is published.
- **Fixtures/data**: Corrupted transient manifest variants derived from a valid bundle.
- **Mocks**: Existing artifact component test doubles.
- **Contract classification**: Internal or accidentally exposed implementation.
- **Compatibility expectation**: Validate only the new manifest shape; do not accept the obsolete shape through a legacy reader.

### 5. `TestTraceSummaryHistogramSchemaAndFallbackAreClosedCompleteAndDeterministic`

- **Type**: Contract/unit
- **Location**: `loomspan-console/internal/mcpadapter/output_schemas_test.go`, `trace_contracts_test.go`, and `traces_test.go`
- **What it proves**: `recordCountsByType` accepts every current closed key with nonnegative integers, rejects unknown/non-integer/negative values, omits zero types, and emits every nonzero entry in canonical enum order in fallback. Semantic counts/outcome are mapped independently.
- **Fixtures/data**: Summary with several deliberately nonalphabetical record types and differing physical/logical counts.
- **Mocks**: Fake trace-analysis summary for handler mapping; schema validation is real.
- **Contract classification**: Ephemeral diagnostic formats.
- **Compatibility expectation**: Add the current-version field coherently to structured output, compact schema, fallback, snapshot, and guidance.

### 6. `TestFrameOutcomeIsOptionalScalarAcrossAnalysisMCPAndBrowser`

- **Type**: Unit/contract/integration
- **Location**: `traceanalysis/calculations_test.go`, `mcpadapter/traces_test.go`, `mcpadapter/output_schemas_test.go`, `browserapi/trace_analysis_test.go`, `browserapi/contracts_test.go`, and `web/src/observability/TraceViews.test.tsx`
- **What it proves**: No close/blank status produces absent or `null` outcome according to each established adapter convention; one close status produces one scalar; duplicate close remains invalid; the old `outcomes` property is rejected/absent; browser timeline failure state uses the scalar.
- **Fixtures/data**: Open frame, blank-status close, failed close, and duplicate-close trace; update `browser-fixtures/trace-analysis/frames.json`.
- **Mocks**: Existing fake browser/MCP analysis services; core invariant uses real processor.
- **Contract classification**: Ephemeral diagnostic formats.
- **Compatibility expectation**: Approved removal of plural shape with no alias or simultaneous old/new behavior.

### 7. `TestCompactAndDetailedFrameContractsExposeProjectionSemantics`

- **Type**: Unit/contract
- **Location**: `traceanalysis/service_test.go`, `mcpadapter/traces_test.go`, `mcpadapter/server_test.go`, and the tools-list snapshot
- **What it proves**: Omitted projection defaults to COMPACT; COMPACT retains hierarchy/counts but omits duration/usage/detail fields; fallback states `omittedByProjection=COMPACT` once and prints no duration sentinel; DETAILED returns known inclusive/self duration, usage, retry identities, validations, failures, gaps, and uncertainties.
- **Fixtures/data**: Existing nested-frame and incomplete-frame fixtures.
- **Mocks**: Real analysis projection; adapter mapping may use the existing fake service.
- **Contract classification**: Ephemeral diagnostic formats.
- **Compatibility expectation**: Preserve the projection field/default while making exclusions explicit.

### 8. `TestTraceTimestampEpochMillisAndFrameDurationsElapsedMillis`

- **Type**: Unit
- **Location**: `traceanalysis/parser_test.go` and `calculations_test.go`
- **What it proves**: Fractional epoch-second trace timestamps convert to the exact epoch millisecond value, while inclusive/self durations are elapsed differences for nested frames and never treated as epoch or monotonic ticks.
- **Fixtures/data**: Fixed timestamps around a known UTC instant and the existing nested-frame duration fixture.
- **Mocks**: None.
- **Contract classification**: Ephemeral diagnostic formats.
- **Compatibility expectation**: Preserve current correct mechanics and protect the new documentation claim.

### 9. `TestTraceRangeSchemaAndHandlersDefaultInitialReadToZero`

- **Type**: Contract/integration
- **Location**: `mcpadapter/trace_contracts_test.go`, `mcpadapter/traces_test.go`, and `mcpadapter/trace_range_http_test.go`
- **What it proves**: Both read tools accept neither cursor control and start at zero; explicit `start: 0` is equivalent; continuation-only resumes; supplying both returns `INVALID_ARGUMENT`; negative explicit starts and invalid continuations remain rejected.
- **Fixtures/data**: One semantic content descriptor and one raw artifact with more than one default page.
- **Mocks**: Handler unit uses fake analysis; HTTP case uses the real MCP server/trace service harness.
- **Contract classification**: Ephemeral diagnostic formats.
- **Compatibility expectation**: Approved relaxation of initial input; preserve exact continuation binding and explicit-start validation.

### 10. `TestInlineContentDiscoveryMatchesEnforcedSelectionAndOmissions`

- **Type**: Unit/contract
- **Location**: `traceanalysis/service_test.go`, `mcpadapter/trace_contracts_test.go`, `mcpadapter/server_test.go`, and tools-list snapshot
- **What it proves**: Discovery states descriptor-first default, explicit opt-in, 8 KiB per-value limit, 32 KiB aggregate source-byte budget, record-order selection, complete-value-only behavior, and typed omissions; executable selection matches the prose exactly.
- **Fixtures/data**: Values just below/at/above per-value and aggregate limits, unavailable/incomplete/binary content, and a page shortened by response admission.
- **Mocks**: None for service selection; schema/snapshot test uses installed server.
- **Contract classification**: Configuration and manifest contracts for maintained Agent Skill guidance; Ephemeral diagnostic formats for MCP behavior.
- **Compatibility expectation**: Preserve descriptor-first behavior and opaque references; no implicit tiny auto-inline.

### 11. `TestTraceInventoryLifecycleDiscoveryMatchesIndependentFactsAndOrderings`

- **Type**: Unit/contract
- **Location**: `traceinventory/service_test.go`, `mcpadapter/server_test.go`, and tools-list snapshot
- **What it proves**: Earlier finalization plus later acquisition remains two values; imported time stays separate; each filter/order uses only its matching lifecycle fact; discovery defines all three and associates the three `*_DESC` values with the correct question.
- **Fixtures/data**: Extend `TestCatalogTargetGainsAcquiredTimeWithoutChangingFinalization`, `TestOldFinalizedImportIsSelectedByImportedFactsAndOrder`, and independent-time fixtures.
- **Mocks**: Existing fake installed catalog/application source.
- **Contract classification**: Ephemeral diagnostic formats.
- **Compatibility expectation**: Preserve field names and ordering enums; improve local semantics only.

### 12. `TestToolsListContainsRequiredTraceSemanticsWithinApprovedDiscoveryCeiling`

- **Type**: Full HTTP contract/snapshot
- **Location**: `loomspan-console/internal/mcpadapter/server_test.go` and `testdata/tools-list-response.json`
- **What it proves**: The exact JSON-RPC discovery response contains 12 tools, zero resources/templates, read-only annotations, workflow/projection/inline/lifecycle/range semantics, and no obsolete `outcomes`; its exact byte size fits the smallest committed KiB ceiling and never exceeds 25,600 bytes.
- **Fixtures/data**: Installed production server; updated exact snapshot.
- **Mocks**: Existing target application fake only; MCP serialization is real.
- **Contract classification**: Ephemeral diagnostic formats.
- **Compatibility expectation**: A ceiling above 20 KiB is approved when semantics require it; required meaning must not be removed merely to retain the old budget.

### 13. `TestFullSerializedTraceSuccessResponsesMeetCommittedBudgets`

- **Type**: Full HTTP integration
- **Location**: New focused MCP response-budget test file under `loomspan-console/internal/mcpadapter/`
- **What it proves**: Complete MCP success responses—including structured output and the single fallback text block—fit committed ordinary-call budgets for 64 inventory items, 64 COMPACT frames, 64 DETAILED frames, 64 descriptors, maximum inline page, default semantic range, and default raw range.
- **Fixtures/data**: Deterministic worst-case values with maximum allowed IDs, continuations, JSON escaping, multibyte UTF-8, and binary content; checked-in measurement table with exact bytes.
- **Mocks**: Real MCP HTTP serialization over deterministic fake inventory/analysis services, or real trace services where cursor behavior is material.
- **Contract classification**: Ephemeral diagnostic formats.
- **Compatibility expectation**: Current-host usability guard; not a protocol-wide host guarantee.

### 14. `TestEncodedPageBudgetAdmitsOnlyWholeItems`

- **Type**: Unit, table-driven
- **Location**: New response-budget helper test under `loomspan-console/internal/mcpadapter/`
- **What it proves**: Fixed envelope/fallback/continuation overhead and per-item contributions are counted exactly; below/at/above boundaries behave deterministically; UTF-8, quotes, control characters, backslashes, and base64 use serialized byte counts; each candidate is encoded once.
- **Fixtures/data**: Injected tiny budgets and synthetic DTO/fallback lines.
- **Mocks**: None; inject the budget constant/cost encoder directly.
- **Contract classification**: Internal or accidentally exposed implementation.
- **Compatibility expectation**: One coherent calculator shared by production and budget assertions.

### 15. `TestByteBudgetPaginationReconstructsInventoryFramesRecordsAndSearch`

- **Type**: Integration, table-driven
- **Location**: `traceinventory/service_test.go`, `traceanalysis/continuation_test.go`, and `search_test.go`
- **What it proves**: For a forced stop before every item boundary, following opaque continuations reconstructs the original ordered result exactly once; caller `pageSize` is a maximum; the rejected candidate is not skipped; filters/order/projection/representation remain fingerprint-bound.
- **Fixtures/data**: At least four matching items per operation with nonmatching rows between matches; all frame orders; logical/physical records; literal match spanning search chunks.
- **Mocks**: Injected admission callback/cost threshold; real cursor encoding and services.
- **Contract classification**: Ephemeral diagnostic formats.
- **Compatibility expectation**: Preserve opaque continuation security and query binding while adding a server-owned stop condition.

### 16. `TestByteBudgetPaginationPreservesInventoryCompletenessAndInlineSearchState`

- **Type**: Integration
- **Location**: `traceinventory/service_test.go`, `traceanalysis/service_test.go`, and `search_test.go`
- **What it proves**: Inventory `complete` remains independent of `hasMore`; shortened record pages apply the 32 KiB inline aggregate only to accepted records; typed inline omissions remain correct; page-local search descriptors and KMP progress resume without loss or stale joins.
- **Fixtures/data**: Incomplete catalog discovery, multiple inline-eligible descriptors around the aggregate limit, repeated content IDs, and a boundary-spanning literal.
- **Mocks**: Existing catalog fake plus injected admission callback.
- **Contract classification**: Ephemeral diagnostic formats.
- **Compatibility expectation**: Preserve completeness/security semantics while reducing page size.

### 17. `TestSingleOversizedSemanticItemReturnsLimitExceededWithoutAdvancing`

- **Type**: Integration/contract
- **Location**: `traceanalysis/service_test.go` and MCP adapter budget/handler tests
- **What it proves**: A first semantic item larger than an empty-page budget returns `LIMIT_EXCEEDED`, no partial structured/fallback item, and actionable narrowing/COMPACT/descriptor guidance; retrying with a representation that fits starts with the same item.
- **Fixtures/data**: Oversized DETAILED frame and oversized semantic record representation under an injected small budget.
- **Mocks**: Real query traversal with injected admission policy; fake resolver only at handler layer.
- **Contract classification**: Ephemeral diagnostic formats.
- **Compatibility expectation**: Use the existing error class; add no alternate error code or silent representation change.

### 18. `TestStructuredAndFallbackPagesDescribeExactlyTheSameItems`

- **Type**: Contract/unit
- **Location**: `mcpadapter/traces_test.go` and full HTTP response-budget tests
- **What it proves**: Inventory/frame/record/search fallback headers match structured `hasMore` and continuation; every accepted item appears once in both views; no unaccepted or partial item appears; all histogram entries are present; every line is complete valid UTF-8; obsolete clipping notice/function is absent.
- **Fixtures/data**: Pages stopped by count, stopped by bytes, final pages, escaped identifiers, and maximum opaque tokens.
- **Mocks**: Adapter-level DTOs plus one full HTTP case per operation family.
- **Contract classification**: Ephemeral diagnostic formats.
- **Compatibility expectation**: Approved removal of independently clipped fallback behavior.

### 19. `TestDefaultRangeResultsFitBudgetAndContinuationsReconstructExactBytes`

- **Type**: Unit/full HTTP integration
- **Location**: `traceanalysis/range_test.go`, `continuation_test.go`, and `mcpadapter/trace_range_http_test.go`
- **What it proves**: The selected default source-byte limit fits the complete MCP budget under worst-case escaped TEXT and BASE64; repeated default reads reconstruct exact retained bytes and source offsets; explicit legal requests remain complete; 16 MiB + 1 returns `LIMIT_EXCEEDED` rather than clamp.
- **Fixtures/data**: ASCII, worst-case JSON control characters, multibyte rune boundary, invalid UTF-8, binary bytes, exact end, and content longer than two default pages.
- **Mocks**: Real artifact/trace service and real MCP HTTP serialization.
- **Contract classification**: Ephemeral diagnostic formats.
- **Compatibility expectation**: Default may shrink; exact explicit maximum and source-byte semantics remain protected.

### 20. `TestBrowserTraceProjectionUsesScalarOutcomeAndCurrentSummaryFacts`

- **Type**: Go contract plus TypeScript unit
- **Location**: `browserapi/trace_analysis_test.go`, `browserapi/contracts_test.go`, `browser-fixtures/trace-analysis/`, `web/src/api/contracts.ts`, `TraceViews.test.tsx`, and `TraceExplorer.test.tsx`
- **What it proves**: Browser DTO/fixtures consume only scalar `outcome`, timeline state remains correct, null/absent remains unknown, and shared summary retry values remain corrected. `recordCountsByType` remains scoped to `LOOMSPAN_get_trace`; the browser summary keeps its deliberate existing field set rather than acquiring an unrequested UI contract.
- **Fixtures/data**: Failed, completed, and no-outcome frames; corrected summary fixture.
- **Mocks**: Existing browser fake analysis service and frontend API mocks.
- **Contract classification**: Ephemeral diagnostic formats.
- **Compatibility expectation**: Atomic in-repository update; no plural TypeScript alias.

### 21. `TestCanonicalRuntimeDebuggingSkillMatchesVerifiedTraceSemantics`

- **Type**: Package validation plus evidence audit
- **Location**: `internal/agentskills/validate_test.go`; behavior tests cited above remain the primary evidence
- **What it proves**: The canonical skill still validates with name/version `1.0.0`, contains no removed raw-first/help workflow, and routes mechanics to the updated guide. Assertions should check stable prohibited/required workflow anchors, not duplicate every prose sentence.
- **Fixtures/data**: Canonical packaged Agent Skill directory.
- **Mocks**: None.
- **Contract classification**: Configuration and manifest contracts.
- **Compatibility expectation**: Preserve metadata and packaging; update guidance atomically with executable behavior.

### 22. PR 31 tools-only and skill-assisted evaluation cases

- **Type**: Manual model evaluation recorded through the existing harness
- **Location**: `loomspan-console/agent-evals/cases/`, dated `agent-evals/results/`, generated summary, and client compatibility/measurement note
- **What it proves**: A tools-only consumer discovers DETAILED, inline/exact reads, zero actual retries, lifecycle meanings, histogram-directed queries, and continuation recovery without raw NDJSON; the skill-assisted run reaches the same evidence with fewer unnecessary calls and preserves limitations.
- **Fixtures/data**: A sanitized same-version trace with independent initial attempts, a later acquired time, notable record types, nested duration, and selected content large enough to exercise inline omission/range continuation.
- **Mocks**: Production-adapter MCP evaluation server; actual supported client/model builds.
- **Contract classification**: Ephemeral diagnostic formats and Configuration/manifest guidance.
- **Compatibility expectation**: Record completed unfavorable outcomes; never fabricate runs or discard them as infrastructure failures.

## Authoring Claims and Evidence Map

| Guidance claim | Required executable evidence |
| --- | --- |
| A retry is an attempt after the initial attempt; sequence count is different | Tests 1 and 2 plus fixture-corpus retry cases |
| `recordCountsByType` is complete for physical records and omitted keys mean zero | Tests 3, 4, and 5 |
| COMPACT omits duration/usage detail; DETAILED provides it | Tests 7 and 8 |
| `inlineContent` is explicit, record-ordered, and bounded 8/32 KiB with typed omissions | Test 10 and post-admission state in Test 16 |
| Initial exact reads may omit both controls and begin at zero | Test 9 |
| Caller `pageSize` is a maximum and byte-budget pagination is lossless | Tests 13 through 18 |
| Default reads are bounded while explicit legal ranges remain exact | Test 19 |
| Finalized, acquired, and imported times are independent | Test 11 |
| Frame close outcome is zero-or-one scalar | Tests 6 and 20 |

Do not add tests that merely search prose for every sentence. Use the underlying behavior tests above, then keep lightweight package/routing checks for the maintained Agent Skill artifact.

## How to Run

### Failing-first checkpoints

From `loomspan-console/`:

```text
go test ./internal/traceanalysis -run TestRetryCountCountsAttemptsAfterInitialNotRetrySequences
go test ./internal/mcpadapter -run 'TestTraceRangeSchemaAndHandlersDefaultInitialReadToZero|TestFrameOutcomeIsOptionalScalar|TestStructuredAndFallbackPagesDescribeExactlyTheSameItems'
```

Record the expected pre-fix assertions. After implementation, the same commands must pass; do not weaken expected values to match current defects.

### Focused Go verification

From `loomspan-console/`:

```text
gofmt -w <changed Go files>
go test ./internal/traceanalysis ./internal/traceinventory ./internal/mcpadapter ./internal/browserapi ./internal/agentskills
go test ./internal/traceanalysis -run 'Retry|Attempt|Histogram|Frame|Range|Pagination|Search'
go test ./internal/mcpadapter -run 'Trace|ToolsList|OutputSchema|Budget|Fallback'
go test ./internal/traceinventory -run 'Inventory|Lifecycle|Pagination|Budget'
go vet ./...
```

### Frontend and repository verification

Use the pinned Go 1.26.5, Node 24.18.0, and npm 12.0.2 toolchains expected by the build tool. From `loomspan-console/`:

```text
npm --prefix web run typecheck
npm --prefix web run test -- src/observability/TraceViews.test.tsx src/observability/TraceExplorer.test.tsx
go test ./...
go run ./internal/buildtool verify
```

`go run ./internal/buildtool verify` performs locked frontend install, Agent Skill validation, frontend typecheck/coverage/build, asset-manifest verification, and the complete Go test suite.

### Race detector

On the documented Windows environment, from `loomspan-console/`:

```powershell
$env:PATH = "C:\msys64\mingw64\bin;" + $env:PATH
$env:CGO_ENABLED = "1"
go test -race ./...
```

### Java fixture coherence

From the repository root:

```powershell
.\mvnw.cmd -pl loomspan-spring-boot-starter test -Dtest=ConsoleTraceFixtureCorpusTest -DfailIfNoTests=false
```

Do not regenerate first. If intentional derived fixture expectations changed, review the diff, then regenerate once with:

```powershell
.\mvnw.cmd -pl loomspan-spring-boot-starter test -Dtest=ConsoleTraceFixtureCorpusTest -Dloomspan.console.fixtures.regenerate=true -DfailIfNoTests=false
```

Rerun the non-regeneration command afterward. Raw NDJSON vocabulary and `consoleCompatibilityVersion` must remain unchanged.

### Manual host and agent evaluation

Use a temporary protected output directory and never print or commit its MCP key:

```text
go run ./internal/buildtool agent-eval serve --case CASE_ID --output TEMP_DIR
go run ./internal/buildtool agent-eval record --session TEMP_DIR --client-events CLIENT_EVENTS.json --answer ANSWER.txt --output RECORD.json
go run ./internal/buildtool agent-eval score --record RECORD.json
go run ./internal/buildtool agent-eval summarize --results agent-evals/results/DATE
```

Run fresh tools-only and skill-assisted conversations. Record exact client/model builds, full serialized response byte counts, continuation use, overflow/collapse incidents, raw-read fallbacks, and incorrect semantic claims. Measure Codex and a second supported host when available; mark unavailable host rows `Not run` instead of inventing data.

## Exit Criteria

- [x] The minimal retry reproduction exists, fails with `retryCount=10` before the fix, and passes with `retryCount=0` afterward.
- [x] All retry formula, same-frame/cross-frame attribution, planning-record non-effect, and invalid attempt lifecycle tests pass.
- [x] Every valid fixture histogram sums to physical `recordCount`; corrupt/unknown histogram forms are rejected; semantic summary fields remain independent.
- [x] Frame `outcomes` is absent from current analysis/MCP/browser/TypeScript contracts and fixtures; optional scalar `outcome` and duplicate-close rejection are covered.
- [x] COMPACT/DETAILED structured output, fallback, and discovery agree; no projection-excluded duration sentinel remains.
- [x] `inlineContent` discovery matches the executable 8/32 KiB selection and omission behavior.
- [x] Both exact-read tools accept omitted initial controls as offset zero, reject both controls, and preserve explicit/continued exactness.
- [x] Full serialized default MCP results—not DTOs alone—fit committed operation budgets with measured headroom.
- [x] Forced byte-budget pagination at every candidate boundary reconstructs inventory, frames, records, and search without loss, duplication, stale joins, or cursor weakening.
- [x] Structured output and fallback describe the identical page with whole UTF-8 lines/items and the same continuation.
- [x] A single oversized semantic item returns typed `LIMIT_EXCEEDED`; no partial serialization or new error code exists.
- [x] Default semantic/raw range traversal reconstructs exact bytes; explicit legal ranges remain complete; oversized explicit ranges fail rather than clamp.
- [x] The exact `tools/list` snapshot contains all required semantics, all 12 read-only tools, and no resources/templates; it uses the smallest committed KiB ceiling that fits and is no larger than 25,600 bytes.
- [x] Browser API, fixtures, TypeScript contracts, and timeline behavior use only scalar `outcome` and corrected shared facts.
- [x] Tests cited as evidence for changed skill-authoring guidance establish every material author-facing claim; Agent Skill metadata remains `1.0.0`; the README coverage row remains accurately `Source-verified`.
- [x] Same-version Java fixture corpus passes without a Java production or compatibility-marker change; no legacy trace reader or old/new DTO shim is added.
- [x] Focused Go tests, frontend tests/typecheck, `go vet ./...`, `go test ./...`, `go run ./internal/buildtool verify`, and supported race tests pass.
- [ ] Codex host measurement is recorded with at least 25% headroom for ordinary defaults; a second host is measured when available or explicitly marked `Not run`.
- [ ] Tools-only and skill-assisted PR 31 evaluation records are sanitized, complete, scored, and retained even when unfavorable.
- [ ] Manual walkthroughs retrieve semantic content without ordinary raw-NDJSON traversal, make no retry/timestamp/projection/lifecycle/completeness error, and resume every partial response explicitly.
