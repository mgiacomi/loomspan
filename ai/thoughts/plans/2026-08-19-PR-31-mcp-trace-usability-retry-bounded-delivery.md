# PR 31 MCP Trace Usability, Retry Correctness, and Bounded Delivery Implementation Plan

## Overview

Correct Console retry accounting and make finalized-trace inspection locally understandable and predictably bounded for an MCP consumer that has only `tools/list`. The implementation will keep descriptor-first content, exact opaque reads, read-only/inert evidence handling, and the existing 12-tool/zero-resource surface while adding complete record-type discovery, scalar frame outcomes, and lossless byte-budget pagination.

Use the dedicated testing artifact at `ai/thoughts/plans/2026-08-19-PR-31-mcp-trace-usability-retry-bounded-delivery-testing.md` before implementation. It turns the ticket's required cases into failing-first tests, response-size fixtures, host-evaluation steps, and explicit exit criteria.

## Current State Analysis

- Trace summary `retryCount` is the number of retry-sequence aggregates, not the number of attempts after an initial attempt. `buildAttemptResults` creates one retry aggregate per `retrySequenceId`, and the manifest uses `len(retryResults)` (`loomspan-console/internal/traceanalysis/processor.go:518-577`, `loomspan-console/internal/traceanalysis/processor.go:425-444`).
- Frame `directRetryCount` is derived from distinct frame-associated retry sequence IDs, which is wrong when initial and later attempts in one sequence are attributed to different frames (`loomspan-console/internal/traceanalysis/query_frames.go:212-218`).
- The validated attempt graph already enforces positive consecutive `attemptNumber`, explicit sequence membership, and provider/semantic retry lifecycle rules. `PLAN_RETRY_REQUESTED` is outside the model-attempt lifecycle (`loomspan-console/internal/traceanalysis/attempts.go:67-145`, `loomspan-console/internal/traceanalysis/processor.go:512-516`).
- `COMPACT` is the frame-query default and removes duration, usage, and identity detail from structured output, but the fallback renders missing duration values as `-`; the tool description does not explain the projection boundary (`loomspan-console/internal/traceanalysis/query_frames.go:73-88`, `loomspan-console/internal/traceanalysis/query_frames.go:221-237`, `loomspan-console/internal/mcpadapter/traces.go:489-493`).
- Record queries are descriptor-first. Explicit `inlineContent` already selects complete values in record order under an 8 KiB per-value and 32 KiB aggregate source-byte budget with typed omission reasons, but discovery gives the boolean no explanation (`loomspan-console/internal/traceanalysis/query_records.go:209-238`, `loomspan-console/internal/traceanalysis/query_records.go:383-407`, `loomspan-console/internal/mcpadapter/trace_contracts.go:50-57`).
- Range services naturally begin at zero and preserve exact source bytes, but the MCP schema and handler require exactly one of `start` or `continuation`, rejecting an omitted initial cursor (`loomspan-console/internal/mcpadapter/traces.go:129-140`, `loomspan-console/internal/mcpadapter/traces.go:280-297`).
- Inventory, frame, and record queries stop at complete item boundaries, but only by item count. There is no encoded MCP-result byte boundary. Navigation fallbacks are separately sliced at 64 KiB, so structured output and fallback can describe different pages and the cut can split a line or UTF-8 sequence (`loomspan-console/internal/traceanalysis/query_frames.go:164-209`, `loomspan-console/internal/traceanalysis/query_records.go:150-253`, `loomspan-console/internal/mcpadapter/traces.go:471-516`, `loomspan-console/internal/mcpadapter/traces.go:546-553`).
- The full HTTP `tools/list` response is 20,304 bytes against a 20,480-byte ceiling, leaving only 176 bytes for the missing semantics (`loomspan-console/internal/mcpadapter/server_test.go:31-108`).
- Inventory already stores and filters finalization, target acquisition, and import times independently; only the installed tool description lacks their lifecycle definitions (`loomspan-console/internal/traceinventory/service.go:305-355`, `loomspan-console/internal/traceinventory/service.go:379-412`).
- Processing counts every validated physical NDJSON record and owns a closed 33-value record-type enumeration, but neither the internal manifest nor `LOOMSPAN_get_trace` contains a per-type histogram (`loomspan-console/internal/traceanalysis/index_writer.go:112-124`, `loomspan-console/internal/traceanalysis/enums.go:3-68`, `loomspan-console/internal/traceanalysis/manifest.go:19-42`).
- One frame can close at most once, and only `FRAME_CLOSED.metadata.status` contributes a close outcome. Accepted evidence therefore has zero or one authoritative outcome, despite plural internal and MCP fields (`loomspan-console/internal/traceanalysis/frames.go:87-110`, `loomspan-console/internal/traceanalysis/frames.go:129-151`, `loomspan-console/internal/mcpadapter/trace_contracts.go:125-155`).

## Desired End State

An MCP-only consumer can discover the ordinary finalized-trace workflow, distinguish projection omissions and lifecycle timestamps, request bounded inline content, begin an exact read without spelling `start: 0`, understand retries as attempts after an initial attempt, and use `recordCountsByType` to select notable records without raw-NDJSON scanning.

Every default MCP success response is below a committed serialized-result budget chosen with at least 25% headroom below the lowest observed host overflow/collapse threshold. List and query operations stop before a complete item would exceed that budget, return `hasMore=true`, and resume losslessly from an opaque continuation. Structured output and fallback cover the identical page. Explicit exact range requests remain complete up to 16 MiB and continue to reject larger requests rather than clamp them.

The scalar frame field is optional `outcome`: it is absent when no nonblank close status exists and contains the sole authoritative close status otherwise. There is no plural compatibility alias.

### Key Discoveries

- Current code and the supplied research are aligned at commit `5c49ec9a0e9b4909e6d5bd90f7da9273432221e2`; focused trace-analysis, inventory, and MCP adapter tests passed during research.
- Correct retry membership can be derived entirely from validated attempt results with `attemptNumber > 1`; no Java producer change or record adjacency inference is needed.
- The response-size problem is the complete MCP result (structured envelope plus deterministic text block), not the neutral DTO alone. Budget admission must therefore be driven by the adapter's exact encoded contribution while traversal remains responsible for cursor position.
- The existing compact authored schemas and complete generated output validation are separate layers. New discovery wording must fit the compact layer without weakening complete runtime validation (`loomspan-console/internal/mcpadapter/output_schemas.go:14-48`).
- The packaged Agent Skill and `ai/skill-authoring/traces-and-debugging.md` already own the richer investigation workflow, so updates should refine those routed topics rather than add a help tool or duplicate a new narrative.

## What We're NOT Doing

- Adding `LOOMSPAN_help`, a narrative summary/diagnosis tool, MCP resources, resource templates, or streaming responses.
- Making raw-artifact traversal the normal semantic workflow.
- Automatically inlining tiny content, changing descriptor-first defaults, or exposing `contentRef`/continuation encoding.
- Renaming `retryCount`, `directRetryCount`, or `acquiredAt`, or adding retry-sequence count as a replacement metric.
- Treating `PLAN_RETRY_REQUESTED` as a model/provider retry.
- Adding warning prefixes, a curated problem list, or a composite `clean`/`unclean` verdict to trace summary.
- Redesigning literal search, raw trace timestamps, the 16 MiB explicit range maximum, evidence ownership, acquisition, or inert/read-only security behavior.
- Adding a Java API, Java SPI, Spring bean override contract, legacy reader, compatibility alias, or cross-version trace migration.
- Promising that a host will never alter a deliberately large explicit range response after Loomspan returns it.

## Skill-Authoring Documentation Impact

**Impact**: Affected

- **Rationale**: Skill authors use the packaged runtime-debugging skill and `ai/skill-authoring/traces-and-debugging.md` to interpret retries, lifecycle times, frame projections, record discovery, and content reads. Corrected retry cardinality, `recordCountsByType`, scalar `outcome`, optional initial range controls, and byte-budget pagination change author-facing debugging guidance even though YAML skill syntax and execution behavior do not change.
- **Documents to update**: `ai/skill-authoring/traces-and-debugging.md`, `ai/skill-authoring/README.md`, and `loomspan-console/agent-skills/loomspan-runtime-debugging/references/mcp-tool-guide.md`. The packaged `SKILL.md` already routes mechanics to the guide and needs no procedural change.
- **Supporting evidence**: Focused attempt/frame calculation tests, fixture-corpus histogram assertions, MCP input/output schema tests, full-call response-budget tests, exact range tests, inventory ordering tests, Agent Skill packaging validation, and the tools-only/skill-assisted evaluation records.
- **Coverage table update**: Required. Keep the `Traces and debugging` topic at `Source-verified`, but update its coverage note to include actual-retry cardinality, record-type histogram discovery, scalar close outcome, and complete byte-budgeted traversal. Routing does not change.
- **LLM-first usability**: Keep immediate mechanics in concise tool descriptions; keep investigative judgment in the routed trace topic and packaged guide. Use exact field names, formulas, omission/recovery tables, and explicit limitations. Do not repeat the full tool guide in the knowledge base or add historical walkthrough narrative.

## Contract and Compatibility Impact

| Surface | Classification and supporting evidence | Planned compatibility treatment |
| --- | --- | --- |
| Application API | No affected type is in the closed `com.lokiscale.loomspan.api` allowlist; all planned production changes are under `loomspan-console/internal`. | No impact; add no Java application-facing type or signature. |
| Supported SPI | Loomspan has no supported Java SPI or internal bean-replacement surface, and this work adds none. | No impact. |
| Configuration and manifest contracts | Packaged Agent Skill metadata remains unchanged, but its maintained debugging guidance and the authoring knowledge-base coverage statement change. No YAML skill syntax, validation, or runtime manifest behavior changes. | Update the routed guidance, package validation, and coverage row atomically; preserve the Agent Skill name and `1.0.0` version metadata. |
| Persisted or serialized contracts | Raw NDJSON vocabulary and producer metadata are unchanged. Same-version portable artifacts remain gated by exact `consoleCompatibilityVersion`. The derived Go manifest gains histogram/count data but is explicitly same-process implementation state. | No cross-version migration or compatibility-marker change. Reprocess current evidence under the new binary; update same-version fixtures atomically. |
| Ephemeral diagnostic formats | MCP retry values change meaning to their ticket-defined natural meaning; `outcomes` intentionally becomes optional scalar `outcome`; `recordCountsByType` is added; initial range controls become optional; default page/range sizing and discovery descriptions change. | Intentional coherent current-version break. Update DTOs, compact/full schemas, fallbacks, snapshots, README, Agent Skill, authoring guide, fixtures, and evaluations together. Preserve names and domain errors except for the typed oversized-single-item path. |
| Internal or accidentally exposed implementation | Attempt/frame aggregation, analysis manifest, query request admission hooks, budget accounting, fallback builders, and opaque cursor position logic change. | Replace in place and update all Go callers/tests. Do not add aliases, dual DTOs, or legacy cursor/content-reference behavior. |

- **Evidence of supported contracts**: The approved ticket, Console README, installed MCP `tools/list` snapshot and schema tests, packaged Agent Skill, authoring knowledge base, and fixture/evaluation consumers establish the intended current-version diagnostic behavior. The Java public-surface allowlist establishes that no Java application API is involved.
- **Intended breaks**: `frame.outcomes: string[]` becomes optional `frame.outcome: string`; retry fields retain their names but correct their values; default responses may contain fewer than the requested maximum item count when the encoded-result budget is reached. All are approved by the ticket and surfaced through discovery/documentation.
- **In-repository consumers to update**: Trace processor/query tests, semantic fixture expectations, MCP DTO/schema/fallback tests, HTTP snapshots, response-size fixtures, inventory tests, shared browser API adapters, browser fixtures, TypeScript contracts/components/tests, Console README, packaged Agent Skill guide, `ai/skill-authoring`, and agent-evaluation cases/results.
- **Public-surface delta**: No Java types, signatures, constructors, Spring beans, or extension points are added or removed. The installed MCP diagnostic surface adds `recordCountsByType`, replaces `outcomes` with `outcome`, and relaxes the initial range input shape.
- **Shim decision**: **No shim.** Console is pre-release, the MCP/trace formats are current-version diagnostics, exact producer/consumer version coordination is already required, and repository guidance prefers one coherent contract. A plural alias or dual retry behavior would preserve no identified protected cross-version consumer.
- **Java-to-Go boundary coordination**: **Not required.** Existing Java records already carry validated attempt number/reason, frame close status, and every physical record type needed by this work. Do not change `consoleCompatibilityVersion`. Run the Java fixture corpus as a coherence check and update fixture expectations only where derived Console output changes.

## Implementation Approach

Use the validated Go analysis graph as the sole authority for retry and record facts, then map those facts outward without adapter recalculation. Introduce transport-neutral page-admission hooks in inventory/frame/record traversal: the MCP adapter supplies an exact encoded-cost function for a prospective complete item (structured JSON contribution plus its fallback line and conservative continuation/envelope reserve), while the service owns scan position and stops before advancing past a rejected candidate. The admission policy is not part of query canonicalization or opaque continuation fingerprints because it is a server release constant, not caller intent.

Establish budgets from checked-in full-call measurements before fixing ordinary call-result constants. Measure the complete JSON-RPC MCP success response for every ticket case and host behavior for Codex plus a second supported host when available. Select the lowest observed host threshold, reserve at least 25%, and commit operation-class budgets no larger than that safe ceiling. Raise the separate `tools/list` discovery ceiling only as much as the required semantics need, up to an approved maximum of 25 KiB (25,600 bytes); do not delete required meaning merely to retain 20 KiB. Use the same accounting helpers in production admission and tests so the asserted budget is the enforced budget.

For default exact reads, commit a conservative source-byte default that fits the safe result ceiling under worst-case JSON escaping and base64 expansion with both structured content and fallback. Omitted `maxBytes` uses that default; explicit `maxBytes` continues to request the complete caller-selected range up to 16 MiB.

## Phase 1: Establish Failing Contracts and Serialized Budget Evidence

### Overview

Capture the current defects and full serialized response sizes before changing behavior. This phase supplies the executable baseline and the deterministic budget-selection evidence required by later phases.

### Changes Required

#### 1. Dedicated testing plan and failing-first matrix

**Files**: `ai/thoughts/plans/2026-08-19-PR-31-mcp-trace-usability-retry-bounded-delivery-testing.md`, plus focused tests under `loomspan-console/internal/traceanalysis/`, `loomspan-console/internal/traceinventory/`, and `loomspan-console/internal/mcpadapter/`

**Changes**:

- Use `ai/commands/3_testing_plan.md` to enumerate the ticket cases, the first expected failing assertion for each workstream, exact commands, fixtures, and exit criteria.
- Add table-driven failing tests for retry formulas, cross-frame retry attribution, record histograms, scalar outcome, optional initial range controls, projection fallback semantics, independent lifecycle times, and item-boundary budget pagination.
- Preserve and rerun invalid attempt identity/numbering/lifecycle coverage as non-regression tests.

#### 2. Full MCP response measurement harness

**Files**: `loomspan-console/internal/mcpadapter/traces_test.go`, `loomspan-console/internal/mcpadapter/server_test.go`, new focused test helpers/fixtures under `loomspan-console/internal/mcpadapter/testdata/`, and `loomspan-console/docs/mcp-client-compatibility.md`

**Changes**:

- Serialize actual MCP/JSON-RPC success responses, not neutral DTOs, for `tools/list`, 64 inventory items, 64 COMPACT frames, 64 DETAILED frames, 64 record descriptors, the maximum inline page, default semantic range, and default raw range.
- Include the one text content block and structured envelope in every measurement. Add adversarial multibyte UTF-8, JSON escaping, and base64 fixtures.
- Record exact baseline bytes and the post-change selected budgets in a checked-in test table or implementation note. Keep fixtures deterministic and free of credentials, absolute paths, or sensitive trace content.
- Exercise the Codex MCP host used by the walkthrough and a second supported host if available. Record the first collapse/truncation/overflow behavior and the selected safe ceiling (lowest observed threshold minus at least 25% headroom). Mark a genuinely unavailable second host as `Not run`; do not fabricate a threshold.
- Retain the 12-tool assertion, zero-resource/template assertion, and exact snapshot. Set `maxToolsListResponseBytes` to the smallest committed KiB ceiling that contains the final response, with 25 KiB (25,600 bytes) as the approved maximum, and assert both the exact response size and that ceiling.

### Success Criteria

#### Automated Verification

- [x] Baseline size table contains every ticket response class and measures the complete MCP result.
- [ ] New semantic tests fail for the current retry, histogram, plural outcome, omitted-range-control, projection fallback, and clipped-fallback behavior before implementation.
- [x] Existing focused suites still establish the unaffected baseline: `go test ./internal/traceanalysis ./internal/traceinventory ./internal/mcpadapter` from `loomspan-console/`.

#### Manual Verification

- [ ] Codex host threshold behavior and the exact client/build are recorded without credentials or full sensitive payloads.
- [ ] A second supported host is measured when available, otherwise its absence is explicitly recorded.
- [ ] The selected safe result ceiling has at least 25% headroom below the lowest observed threshold.

---

## Phase 2: Correct Retry, Histogram, and Frame Outcome Facts

### Overview

Fix authoritative analysis facts before changing MCP presentation. Keep retry-sequence usage aggregation intact while separating it from actual retry cardinality.

### Changes Required

#### 1. Actual retry accounting

**Files**: `loomspan-console/internal/traceanalysis/processor.go`, `loomspan-console/internal/traceanalysis/model.go`, `loomspan-console/internal/traceanalysis/frames.go`, `loomspan-console/internal/traceanalysis/query_frames.go`, `loomspan-console/internal/traceanalysis/calculations_test.go`, `loomspan-console/internal/traceanalysis/service_test.go`

**Changes**:

- Compute trace retries as the count of validated `attemptResult` entries whose `AttemptNumber > 1`, equivalent to `sum(max(0, attemptsInSequence-1))`. Continue building retry-sequence rows for usage grouping; never use their cardinality for `retryCount`.
- After attempt results exist, associate later-attempt IDs back to their explicitly recorded frames. Persist a direct retry-attempt count (or internal retry-attempt identity set) with each frame result so COMPACT can retain the count without retaining DETAILED identity arrays.
- Do not derive frame retries by subtracting frame-local sequences from attempts. This preserves correctness when attempt one and attempt two belong to different frames.
- Add the full ticket matrix: 1/0, 2/1, 3/2, ten independent 10/0, mixed sequences, same-frame and cross-frame attribution, and a `PLAN_RETRY_REQUESTED` non-effect.

#### 2. Complete physical record histogram

**Files**: `loomspan-console/internal/traceanalysis/processor.go`, `loomspan-console/internal/traceanalysis/manifest.go`, `loomspan-console/internal/traceanalysis/dto.go`, `loomspan-console/internal/traceanalysis/query_facts.go`, `loomspan-console/internal/traceanalysis/service.go`, processor/index/fixture tests

**Changes**:

- Increment a `map[TraceRecordType]int64` once for every validated physical record immediately alongside record-index insertion and before physical-only chunk records leave semantic processing.
- Persist only nonzero entries in the analysis manifest and copy them into `TraceSummary` without reinterpretation.
- Validate on manifest read that keys belong to the closed enumeration, values are positive integers, and their overflow-safe sum equals `recordCount`. Omitted known keys mean zero.
- Keep outcome, terminal failure, retries, logical failures, validations, gaps, uncertainties, and usage completeness independent from the physical histogram.
- Test exact counts across warnings, validations, model failures, tool failures, terminal records, chunks, recovered failures, and the complete semantic fixture corpus.

#### 3. Scalar authoritative frame outcome

**Files**: `loomspan-console/internal/traceanalysis/frames.go`, `loomspan-console/internal/traceanalysis/model.go`, `loomspan-console/internal/traceanalysis/dto.go`, `loomspan-console/internal/traceanalysis/query_frames.go`, `loomspan-console/internal/browserapi/trace_analysis.go`, `loomspan-console/web/src/api/contracts.ts`, `loomspan-console/web/src/observability/TraceTimeline.tsx`, related browser fixtures/components/tests, and frame calculation/query tests

**Changes**:

- Replace the frame outcome set/slice with one optional scalar populated only by the sole accepted `FRAME_CLOSED.metadata.status`.
- Keep duplicate close rejection. A frame without a close or with an accepted blank status has no outcome; do not synthesize `UNKNOWN`.
- Match the existing singular `FrameFilter.Outcome` directly against the scalar.
- Update the shared browser adapter and TypeScript trace-analysis contract from `outcomes: string[]` to `outcome: string | null`; update timeline state derivation and fixtures/tests atomically so the browser and MCP projections remain coherent.
- Add explicit zero-or-one invariant tests and preserve duplicate-close rejection.

### Success Criteria

#### Automated Verification

- [x] Retry matrix and invalid-attempt non-regression tests pass: `go test ./internal/traceanalysis -run 'Retry|Attempt|Frame'`.
- [x] Histogram keys are closed, nonzero, nonnegative in outward form, and sum exactly to physical `recordCount` for all fixture traces.
- [x] A recovered failed attempt can coexist with `outcome=SUCCEEDED` without changing logical `failureCount` or adding a composite verdict.
- [x] Frame outcome tests prove zero-or-one cardinality and singular filtering.

#### Manual Verification

- [ ] Inspect one evaluation trace with ten independent successful initial attempts and confirm `attemptCount=10`, `retryCount=0`.
- [ ] Inspect one cross-frame retry sequence and confirm only the frame containing the later attempt receives `directRetryCount=1`.

---

## Phase 3: Make the MCP Trace Contract Locally Understandable

### Overview

Map corrected facts to the installed MCP contract, make projection/content/range/lifecycle behavior discoverable, and keep `tools/list` under the approved 25 KiB maximum using the smallest ceiling that fits.

### Changes Required

#### 1. Summary histogram and scalar outcome DTO/schema/fallback

**Files**: `loomspan-console/internal/mcpadapter/trace_contracts.go`, `loomspan-console/internal/mcpadapter/traces.go`, `loomspan-console/internal/mcpadapter/output_schemas.go`, `loomspan-console/internal/mcpadapter/trace_contracts_test.go`, `loomspan-console/internal/mcpadapter/output_schemas_test.go`, `loomspan-console/internal/mcpadapter/traces_test.go`

**Changes**:

- Add `recordCountsByType` to `traceSummaryDTO`, mapping all nonzero processor counts.
- Constrain the complete output schema to the current closed record-type keys and nonnegative integer values; reject unknown keys, fractions, strings, and negative values. Keep the compact discovery schema concise while naming the field and its omitted-means-zero semantics in the tool description.
- Emit every nonzero histogram entry in closed-enum order in `LOOMSPAN_get_trace` fallback.
- Replace `frameDTO.Outcomes []string`/`outcomes` with optional scalar `Outcome *string`/`outcome` throughout mapping, authored schema, generated schema, fallback, snapshot, and semantic fixtures. Do not emit a deprecated plural alias.

#### 2. Projection and time semantics

**Files**: `loomspan-console/internal/mcpadapter/traces.go`, frame input schema preparation helpers, `loomspan-console/internal/mcpadapter/testdata/tools-list-response.json`, MCP contract tests

**Changes**:

- State locally that frame queries default to COMPACT, COMPACT is orientation and omits duration/usage detail, and DETAILED supplies duration, usage attribution, retry identities, validations, failures, gaps, and uncertainties.
- Keep projection-excluded structured fields absent according to the existing `omitempty` convention.
- For COMPACT fallback, omit per-frame duration/usage fields and add `omittedByProjection=COMPACT` once in the page header. DETAILED fallback prints real elapsed millisecond values; never print `-` as a duration sentinel.
- Add source-level and adapter tests proving record timestamps are epoch milliseconds and durations are elapsed milliseconds, including nested known inclusive/self durations.

#### 3. Inline content and lifecycle discovery

**Files**: `loomspan-console/internal/mcpadapter/traces.go`, input schema preparation helpers, `loomspan-console/internal/traceinventory/dto.go`, MCP discovery tests and snapshot

**Changes**:

- Add compact `inlineContent` schema/tool prose: explicit opt-in, complete eligible values only, 8 KiB per value, 32 KiB aggregate source bytes, record-order selection, descriptor retention, and typed omission reasons.
- Keep descriptor-first default and opaque `contentRef` wording. Do not describe internal encoding.
- Define `finalizedAt`, `acquiredAt`, and `importedAt` in the list-traces description, explicitly allowing acquisition after finalization. Associate `FINALIZED_DESC`, `ACQUIRED_DESC`, and `IMPORTED_DESC` with their corresponding lifecycle question.
- Shorten duplicated range/pagination prose through shared internal schema-description helpers, then allow the complete discovery response to grow above 20 KiB when the required semantics still need it. Keep the final response at or below the approved 25 KiB ceiling without removing any operation or required meaning.

#### 4. Optional initial range controls

**Files**: `loomspan-console/internal/mcpadapter/traces.go`, `loomspan-console/internal/mcpadapter/trace_contracts.go`, `loomspan-console/internal/mcpadapter/trace_contracts_test.go`, range HTTP tests

**Changes**:

- Change the range schema from exactly-one-of to at-most-one-of `start` and `continuation`; neither means the first read at source offset zero.
- Change handler validation to reject only the simultaneous controls. Preserve explicit nonnegative start validation, including explicit zero, and continuation binding/recovery.
- Apply identical behavior to semantic content and raw artifact reads.

### Success Criteria

#### Automated Verification

- [x] Complete and compact schemas expose `recordCountsByType` and optional scalar `outcome`; complete validation rejects invalid histogram keys/values.
- [x] COMPACT/DETAILED structured and fallback tests prove omissions and detailed duration/usage availability without sentinels.
- [x] Discovery contains `COMPACT`, `DETAILED`, `duration`, `usage`, inline limits/omission, and all three lifecycle meanings near their operations.
- [x] Omitted range controls and explicit `start: 0` are equivalent; both controls return `INVALID_ARGUMENT`.
- [x] Full HTTP `tools/list` remains at or below 25,600 bytes, uses the smallest committed KiB ceiling that fits the required semantics, and matches its updated snapshot.

#### Manual Verification

- [ ] A consumer shown only `tools/list` can explain how to obtain duration detail and bounded inline content.
- [ ] The same consumer can explain why `acquiredAt > finalizedAt` is valid and select the matching inventory ordering.

---

## Phase 4: Enforce Complete Byte-Budgeted Delivery

### Overview

Replace independent fallback clipping with lossless server-side page admission at complete item boundaries. Caller `pageSize` remains a maximum; the encoded result budget may end a page earlier.

### Changes Required

#### 1. Shared complete-result budget accounting

**Files**: new internal helpers under `loomspan-console/internal/mcpadapter/`, `loomspan-console/internal/traceanalysis/limits.go`, MCP adapter budget tests

**Changes**:

- Commit separate ordinary navigation/descriptor result ceilings only after Phase 1 measurement. Each ceiling must be at most 75% of the lowest observed host threshold and must cover the serialized MCP result including structured output, text content, envelope metadata, JSON escaping, continuation reserve, and base64 expansion where applicable.
- Implement an incremental exact-cost accumulator: encode each mapped structured item once, encode its complete fallback line once, and add fixed array/envelope plus conservative `hasMore`/continuation overhead. Avoid repeatedly serializing the whole prefix.
- Allow tests to inject small budgets so every boundary, a just-below item, a just-above item, multibyte text, escaped text, and base64 can be exercised deterministically.
- If one semantic item cannot fit an empty page, return the existing typed `LIMIT_EXCEEDED` class with recovery to narrow the filter, use COMPACT, or retrieve content through its descriptor. Never serialize a partial item and do not add another error code.

#### 2. Transport-neutral admission with exact continuation position

**Files**: `loomspan-console/internal/traceanalysis/query_frames.go`, `loomspan-console/internal/traceanalysis/query_records.go`, literal-search traversal as applicable, `loomspan-console/internal/traceinventory/service.go`, their continuation/pagination tests

**Changes**:

- Add private/internal page-admission callbacks to inventory, frame, record, and literal-search requests. The adapter supplies encoded contribution cost; services retain authority over scan position, matching, and opaque continuation creation.
- Do not include the callback or release budget in the canonical caller query fingerprint. Preserve page size, filters, projection, representation, inline choice, owner, handle, and operation binding.
- Before appending a candidate, ask whether the complete item plus required page overhead fits. On rejection, stop before that item, set `hasMore=true`, and encode a continuation that resumes with it (or safely rechecks skipped nonmatches without losing/duplicating a match).
- Preserve inline selection order and its independent 8/32 KiB source-byte rules. If byte admission returns a shorter record page, recompute aggregate inline selection only over the returned page and retain typed omission reasons.
- Preserve inventory `complete` independently from `hasMore`; byte pagination must not turn incomplete discovery into a false negative/uniqueness claim.

#### 3. Fact-complete fallback from the accepted page

**Files**: `loomspan-console/internal/mcpadapter/traces.go`, fallback tests

**Changes**:

- Delete `boundedNavigationText` slicing for inventory/frame/record results.
- Generate the deterministic fallback from exactly the accepted structured page and include the same `hasMore` and continuation. Every fallback line is whole and valid UTF-8.
- Keep exact range fallback complete; it is never passed through navigation clipping.

#### 4. Safe default exact-range sizing

**Files**: `loomspan-console/internal/traceanalysis/limits.go`, `loomspan-console/internal/traceanalysis/range.go`, `loomspan-console/internal/mcpadapter/trace_contracts.go`, range tests and tool descriptions

**Changes**:

- Replace the current 64 KiB default with the largest conservative committed source-byte default proven to keep worst-case TEXT/BASE64 complete MCP results within the safe default ceiling. Account for duplicate structured/fallback content and worst-case JSON escaping.
- Preserve the 16 MiB explicit maximum. Omitted `maxBytes` uses the safe default; explicit values return the complete requested bounded range or `LIMIT_EXCEEDED` above the maximum.
- Prove default continuation traversal reconstructs exact retained bytes with unchanged `actualStart`, `actualEnd`, `totalLength`, encoding, and source offsets.

### Success Criteria

#### Automated Verification

- [x] Full MCP results for every default worst case fit their committed operation budget, including fallback overhead.
- [x] Boundary tests force early pagination after every possible item and reconstruct the original ordered inventory/frame/record/search result without loss or duplication.
- [x] No encoded JSON item, descriptor, fallback line, UTF-8 sequence, or base64 value is split.
- [x] Structured output and fallback contain exactly the same accepted items and continuation state.
- [x] Explicit oversized ranges fail; default semantic/raw continuation traversal reconstructs exact source bytes.
- [x] Focused race-safe pagination/continuation tests pass, followed by `go test ./...` and `go run ./internal/buildtool verify` from `loomspan-console/`.

#### Manual Verification

- [ ] Ordinary default trace inspection calls remain below the measured host overflow/collapse threshold in Codex.
- [x] A deliberately large explicit range is documented as caller-controlled and still arrives complete from Loomspan, regardless of host presentation.

---

## Phase 5: Synchronize Consumer Guidance, Fixtures, and Evaluations

### Overview

Update every in-repository consumer of the current-version diagnostic contract and validate both tools-only discoverability and skill-assisted efficiency.

### Changes Required

#### 1. Console and author-facing documentation

**Files**: `loomspan-console/README.md`, `ai/skill-authoring/traces-and-debugging.md`, `ai/skill-authoring/README.md`, `loomspan-console/agent-skills/loomspan-runtime-debugging/references/mcp-tool-guide.md`

**Changes**:

- Document retry formulas/examples, cross-frame direct attribution, and the fact that `PLAN_RETRY_REQUESTED` is unrelated to model/provider retry count.
- Document `recordCountsByType` as a complete nonzero physical histogram whose omitted known keys mean zero; direct consumers to query selected types for paginated details and keep semantic counts/outcome independent.
- Update frame guidance to optional scalar `outcome`, COMPACT/DETAILED omissions, and epoch-versus-elapsed millisecond terminology.
- Document descriptor/inline/exact-read choices, omitted initial cursor behavior, caller page size as a maximum, byte-budget early pagination, continuation recovery, default range size, and explicit large-range responsibility.
- Define finalized/acquired/imported times once in each routed authority and preserve their independent filtering/ordering semantics.
- Update the authoring README coverage note without changing routing or overstating broader limits/quotas coverage.

#### 2. Snapshots, fixtures, packaging, and cross-language coherence

**Files**: `loomspan-console/internal/mcpadapter/testdata/tools-list-response.json`, affected `loomspan-console-fixtures/expected/*.json`, `loomspan-console/browser-fixtures/trace-analysis/summary.json`, `loomspan-console/browser-fixtures/trace-analysis/frames.json`, `loomspan-console/internal/browserapi/`, affected TypeScript contracts/components/tests under `loomspan-console/web/src/`, and Agent Skill validation tests

**Changes**:

- Update all plural outcome, retry expectation, histogram, range-schema, fallback, and page-size snapshots atomically.
- Preserve LF endings in committed fixture files.
- Run Agent Skill reference validation and archive/package tests through the repository build tool.
- Run `ConsoleTraceFixtureCorpusTest` without regeneration first. Regenerate committed fixtures only when the testing plan identifies intentional derived expectation changes; do not change Java producer vocabulary or compatibility version.

#### 3. Tools-only and skill-assisted walkthroughs

**Files**: relevant cases under `loomspan-console/agent-evals/cases/`, sanitized records under a dated `loomspan-console/agent-evals/results/` directory, generated summary, `loomspan-console/docs/mcp-client-compatibility.md`

**Changes**:

- Update/add PR 31 cases that ask the same finalized-trace question with MCP tools only and with the packaged Agent Skill.
- Record native calls, serialized response sizes, overflow incidents, continuations, semantic claims, raw-artifact fallbacks, and final answer using the existing harness. Do not commit placeholder or fabricated runs.
- Require the tools-only evaluator to find zero retries for independent initial attempts, request DETAILED for duration, use inline/exact semantic content, distinguish lifecycle times, use the histogram to select records, and complete pagination without raw NDJSON.
- Require the skill-assisted run to preserve the same facts/limitations with fewer unnecessary calls. Treat unfavorable completed runs as evidence, not infrastructure failures to discard.

### Success Criteria

#### Automated Verification

- [x] `go test ./...` passes from `loomspan-console/`.
- [x] `go vet ./...` passes from `loomspan-console/`.
- [x] `go run ./internal/buildtool verify` passes, including formatting, schemas, Agent Skill validation, snapshots, and packaging checks.
- [x] `go test -race ./...` passes with the repository-documented MSYS2/gcc setup where the environment supports it.
- [x] `./mvnw.cmd -pl loomspan-spring-boot-starter test -Dtest=ConsoleTraceFixtureCorpusTest -DfailIfNoTests=false` passes from the repository root.
- [x] No Java production type is changed. If implementation discovers that this assumption is false, stop and revise the contract classification/plan before touching Java rather than expanding scope implicitly.
- [x] Updated skill-authoring claims are backed by the cited focused tests/fixtures and meet the README's LLM-first standard.

#### Manual Verification

- [ ] MCP-only walkthrough completes the semantic workflow without raw artifact reads and without an incorrect retry, timestamp, projection, omission, or completeness claim.
- [ ] Skill-assisted walkthrough completes with fewer unnecessary content calls while preserving evidence limitations.
- [ ] Neither walkthrough encounters overflow on ordinary defaults in the measured Codex host; all partial pages/ranges are resumed explicitly.
- [x] README, `tools/list`, fallback text, packaged Agent Skill, authoring guide, and evaluation rubric use the same terminology and limits.

---

## Testing Strategy

### Unit Tests

- Table-driven attempt/retry cardinality and same-/cross-frame attribution.
- Physical histogram accumulation, closed-key/value validation, sum invariant, and independence from semantic counts.
- Zero-or-one frame close outcome and singular filter behavior.
- COMPACT omission versus DETAILED duration/usage mapping and epoch/elapsed time semantics.
- Inline per-value/aggregate selection and omission reasons after page admission.
- At-most-one range controls, default zero, exact offset/encoding behavior, and oversized explicit range rejection.
- Incremental encoded-cost calculations with JSON escaping, UTF-8, base64, continuation overhead, and oversized single items.

### Integration and Contract Tests

- Full HTTP `tools/list` snapshot/size/semantic presence with 12 tools and no resources/templates.
- Full serialized MCP success envelopes for all worst-case defaults.
- Lossless inventory/frame/record/search reconstruction through byte-budget continuations.
- Structured/fallback page equivalence and deterministic histogram/fallback ordering.
- Inventory lifecycle filter/order independence.
- Same-version Java fixture corpus and Agent Skill archive/reference validation.

### Manual Testing Steps

1. Measure baseline and final complete MCP responses in the target Codex host and one other supported host when available.
2. Run the tools-only evaluation from a fresh conversation and record every call/continuation/overflow decision.
3. Run the same prompt with the packaged Agent Skill from a fresh conversation and compare call efficiency and evidence handling.
4. Explicitly request a large legal range to confirm Loomspan returns it completely while documenting that host presentation is caller responsibility.

## Performance Considerations

- Keep trace processing one-pass and bounded: histogram accumulation adds one closed-enum map increment per physical record.
- Retry/frame attribution should use an `attemptId -> attemptNumber` lookup and frame-local attempt IDs, avoiding sequence rescans or quadratic joins.
- Encoded page admission should marshal each candidate structured item and fallback line once and maintain running byte totals. Do not repeatedly serialize every accumulated prefix.
- Byte budgeting may return fewer than `pageSize` items and increase calls for very large pages, but prevents host overflow and preserves lossless recovery. Default sizes should optimize the ordinary semantic workflow, not bulk export.
- Keep range reads streaming/bounded and do not allocate the 16 MiB maximum unless explicitly requested.

## Migration Notes

- No durable data migration or backfill is required. Analysis manifests, indexes, handles, continuations, and imported catalogs are transient and are rebuilt by the current process.
- Existing continuations/content references may become stale across the new binary exactly as current recovery guidance already describes; callers restart the query or re-query the descriptor by `traceId`.
- MCP consumers must move atomically from plural `outcomes` to optional scalar `outcome`. No alias or deprecation window is planned.
- Same-version portable trace acceptance and `consoleCompatibilityVersion` do not change because the raw producer/consumer protocol is unchanged.

## References

- Original ticket: `ai/thoughts/tickets/loomspan-console-pr-31-mcp-trace-usability-retry-correctness-and-bounded-delivery.md`
- Related research: `ai/thoughts/research/2026-08-19-PR-31-mcp-trace-usability-retry-bounded-delivery.md`
- Dedicated testing plan: `ai/thoughts/plans/2026-08-19-PR-31-mcp-trace-usability-retry-bounded-delivery-testing.md`
- Framework compatibility lens: `ai/thoughts/framework-feature-design-lens.md`
- Skill-authoring routing and standards: `ai/skill-authoring/README.md`
- Source verification protocol: `ai/skill-authoring/source-verification.md`
- Current author-facing trace guidance: `ai/skill-authoring/traces-and-debugging.md`
- Packaged Agent Skill guide: `loomspan-console/agent-skills/loomspan-runtime-debugging/references/mcp-tool-guide.md`
- Console repository guidance and verification commands: `loomspan-console/AGENTS.md`
