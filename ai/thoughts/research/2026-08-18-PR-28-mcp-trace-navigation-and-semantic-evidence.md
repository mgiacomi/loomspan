---
date: 2026-08-18T09:28:41-07:00
researcher: Codex
git_commit: 23dc8c3a54d745ce9125ad8a4ebabb69c65c8e19
branch: main
repository: loomspan
topic: "PR 28 — MCP Trace Navigation and Semantic Evidence"
tags: [research, codebase, loomspan-console, mcp, trace-inventory, trace-analysis, semantic-content, plan-identity]
status: complete
last_updated: 2026-08-18
last_updated_by: Codex
---

# Research: PR 28 — MCP Trace Navigation and Semantic Evidence

**Date**: 2026-08-18T09:28:41-07:00
**Researcher**: Codex
**Git Commit**: 23dc8c3a54d745ce9125ad8a4ebabb69c65c8e19
**Branch**: main
**Repository**: loomspan

## Research Question

Research the current Loomspan codebase for the implementation surfaces, existing behavior, contracts, consumers, fixtures, tests, and historical context relevant to `ai/thoughts/tickets/loomspan-console-pr-28-mcp-trace-navigation-and-semantic-evidence.md`.

## Summary

The current finalized-trace path is already organized around the architecture named by PR 28:

```text
Java runtime trace producer
  -> canonical NDJSON artifact and application trace catalog
  -> Go artifact import/acquisition and process-local storage
  -> Go trace-analysis parser, indexes, facts, and bounded readers
  -> peer browser and MCP adapters
  -> web UI or stateless MCP client
```

The six existing MCP trace tools are `LOOMSPAN_list_traces`, `LOOMSPAN_get_trace`, `LOOMSPAN_query_trace_frames`, `LOOMSPAN_query_trace_records`, `LOOMSPAN_read_trace_payload`, and optional `LOOMSPAN_read_trace_artifact` (`loomspan-console/internal/mcpadapter/trace_contracts.go:10-16`). The shared Go services already provide opaque owner/handle-bound references, query-bound continuations, bounded range reads, trace-ID resolution, collision handling, and explicit inventory completeness. The current semantic-addressing model, however, is specifically a reconstructed-payload and failure-diagnostic model. Ordinary record `data` remains parsed only as opaque JSON and is reachable through raw record/artifact bytes rather than through a general semantic descriptor (`loomspan-console/internal/traceanalysis/dto.go:84-135`, `loomspan-console/internal/traceanalysis/content_ref.go:13-45`).

Inventory currently accepts only page size and continuation. It consolidates installed target/imported evidence by trace ID, checks target catalog collisions, orders by `finalizedAt` descending, and fingerprints page size plus target scope. Artifact entries already retain an acquisition/admission timestamp as `acquisitionTime` and expose it as `AcquiredAt`, but the unified inventory DTO neither carries nor orders/filters by it. Imported processing validates trace/session identity, outcome, finalization time, persistence policy, and compatibility, but currently does not derive `entrySkill`; imported inventory candidates therefore receive the empty `EntrySkill` stored in imported `TraceMetadata` (`loomspan-console/internal/artifact/import.go:66-70`, `loomspan-console/internal/traceanalysis/processor.go:440-446`).

Frame queries currently have one rich `FrameSummary` projection. The default canonical query returns hierarchy, timing, all usage calculations, skill/outcome landmarks, attempt/retry/validation/failure IDs, gaps, and uncertainties for each frame. `FrameQuery` contains filter, order, page size, and cursor, with all three query inputs included in its continuation fingerprint; there is no projection selector (`loomspan-console/internal/traceanalysis/query_frames.go:43-59`, `loomspan-console/internal/traceanalysis/query_frames.go:233-260`).

Record queries currently default to physical representation. Logical representation suppresses chunk records and labels reconstructed envelope roots `logical`, while ordinary records remain labeled `physical` even during a logical query. `inlinePayload` is a Boolean included in the continuation fingerprint. It only inlines envelope payloads whose individual reconstructed length is at most 8 KiB; there is no aggregate page budget and no omission descriptor beyond any independently materialized payload fact (`loomspan-console/internal/traceanalysis/query_records.go:65-85`, `loomspan-console/internal/traceanalysis/query_records.go:199-226`, `loomspan-console/internal/traceanalysis/limits.go:19-27`).

The Java producer already implements the PR 27 identity contract needed by PR 28. It removes any model-authored `planId`, supplies a framework ID before mapping the plan, records that ID on creation and updates, and records the accepting `attemptId` and `retrySequenceId` on `PLAN_CREATED` (`loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/runtime/planning/DefaultPlanningService.java:667-688`, `loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/core/DefaultExecutionTraceRecorder.java:73-88`). The Go parser preserves those metadata bytes, but the current `RecordFacts` model has no typed plan fact, and the committed shared fixture corpus contains model/attempt examples but no current plan-chain fixture exercising those fields.

Literal search exists in two forms. The browser-only `Search` service scans raw physical record bytes and reconstructed payload-store bytes with bounded work and continuations. MCP instead exposes `filter.literalText` on record queries, which case-sensitively checks encoded `metadata` and `data` only and returns each complete rich `recordDTO` plus per-record search matches. It does not use the standalone search service, and neither MCP record result nor browser search page carries page-level search coverage semantics (`loomspan-console/internal/traceanalysis/search.go:14-31`, `loomspan-console/internal/traceanalysis/query_records.go:245-304`, `loomspan-console/internal/traceanalysis/record_facts.go:170-185`).

MCP success responses contain structured content plus a text arm. Trace lists, summaries, frames, records, and ranges currently build that text arm by JSON-serializing the entire mapped result, so large results are duplicated (`loomspan-console/internal/mcpadapter/contracts.go:145-148`, `loomspan-console/internal/mcpadapter/traces.go:354-358`). No committed `tools/list` byte budget or serialized-size assertion was found. Existing tests do cover closed/bounded schemas, semantic fixtures, browser/MCP acquisition sharing, range serialization/allocation, safe trace resolution, opaque-token recovery, collision handling, and two protocol paths.

## Detailed Findings

### 1. Unified finalized-trace inventory

`traceinventory.Query` currently contains only `PageSize` and `Continuation`; each result contains an observation time, compact entries, independent `Complete` and `HasMore` facts, limitations, and continuation (`loomspan-console/internal/traceinventory/dto.go:5-35`). Each entry contains trace ID, session ID, entry skill, outcome, finalization time, and an ambiguity flag.

`Service.List` reads the process-local artifact snapshot, selects target-owned entries only from the current target scope, includes imported entries independently of target selection, and groups all installed candidates by trace ID. It marks a candidate ambiguous when both target and imported copies exist, and probes the target catalog for collisions when only an imported copy is installed (`loomspan-console/internal/traceinventory/service.go:77-174`). It emits installed candidates first and then catalog-only candidates while suppressing repeated trace IDs (`loomspan-console/internal/traceinventory/service.go:176-255`).

Installed candidates are sorted by `FinalizedAt` descending, then `TraceID` ascending. The same fields form the installed continuation key (`loomspan-console/internal/traceinventory/service.go:309-337`). Canonical metadata selection for duplicate installed identities also starts with newest `FinalizedAt`, then session, entry skill, and outcome (`loomspan-console/internal/traceinventory/service.go:319-330`).

Inventory continuations encode an installed/application segment, installed key, application cursor, a request fingerprint, and an installed-set fingerprint. The request fingerprint currently covers only page size and target scope; the installed-set fingerprint covers trace identity, owner/handle identities, session, entry skill, outcome, and finalization time (`loomspan-console/internal/traceinventory/cursor.go:25-47`, `loomspan-console/internal/traceinventory/service.go:339-360`). A continuation is rejected if the selected target/page size or installed evidence set changes.

The application-side catalog protocol currently accepts only `pageSize` and `cursor`. Java rejects any other query parameter and its cursor codec records filter value `none` (`loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/observability/web/ObservabilityRestController.java:204-238`, `loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/observability/web/ObservabilityRestController.java:318-325`, `loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/observability/web/ObservabilityCursorCodec.java:46-53`). The Go observability client likewise constructs list URLs with only those two values (`loomspan-console/internal/observability/service.go:204-227`).

Current inventory tests exercise completeness decision cases, trace-ID consolidation before pagination, canonical ambiguity metadata, imported/catalog collisions, full installed pages, precise `hasMore`, selection/set changes across continuation, and target rotation (`loomspan-console/internal/traceinventory/service_test.go:61-235`).

### 2. Artifact admission facts and imported metadata

The artifact entry model already records `acquisitionTime`. It is returned as `AcquiredAt` from acquisition results, lookup results, and storage snapshots (`loomspan-console/internal/artifact/model.go:41-50`, `loomspan-console/internal/artifact/model.go:74-91`, `loomspan-console/internal/artifact/model.go:111-143`). The storage snapshot maps `entry.acquisitionTime` into each `StoredEntry.AcquiredAt`, while lookup returns the same fact without updating last-use time (`loomspan-console/internal/artifact/service.go:359-408`, `loomspan-console/internal/artifact/service.go:419-455`). The inventory builds its entry from lookup metadata but omits lookup `AcquiredAt` (`loomspan-console/internal/traceinventory/service.go:302-306`).

Import preflight obtains trace and session identity from the validated first record. The process-local entry is created with those two fields and the current service clock as `acquisitionTime`; after complete processing, the processor supplies validated outcome, `finalizedAt`, and persistence policy (`loomspan-console/internal/artifact/import.go:35-85`, `loomspan-console/internal/traceanalysis/processor.go:277-303`, `loomspan-console/internal/traceanalysis/processor.go:440-447`). The processor does not currently populate `TraceMetadata.EntrySkill`. Target acquisitions receive entry skill from the authoritative Java trace-detail response, whose validator requires it (`loomspan-console/internal/observability/dto.go:80-93`, `loomspan-console/internal/observability/service.go:313-322`).

Import tests cover processor-owned metadata publication, duplicate identity, size bounds, cancellation, target rotation independence, and concurrent duplicate admission (`loomspan-console/internal/artifact/import_test.go:23-135`). Artifact storage is current-process state: service shutdown and scope invalidation remove entries and bundle directories rather than adopting them on restart (`loomspan-console/internal/artifact/service.go:515-546`).

### 3. Current frame orientation contract

The only frame output type is `FrameSummary`. Its compact structural fields and rich calculated fields coexist in one DTO: parent/children, type, route, open/close timestamps, inclusive/self duration, direct/descendant/inclusive usage and completeness, skill names, outcomes, attempt IDs, retry-sequence IDs, validation statuses, failure IDs, gaps, and uncertainties (`loomspan-console/internal/traceanalysis/dto.go:49-81`).

`FrameQuery` supports filters and the `CANONICAL`, `DURATION_DESC`, and `USAGE_DESC` orders. Canonical order preserves first-open order; the other orders read processor-created indexes (`loomspan-console/internal/traceanalysis/query_frames.go:11-23`, `loomspan-console/internal/traceanalysis/query_frames.go:220-230`). The default order is canonical, and the continuation fingerprint includes normalized filter, order, and page size. `frameResultToSummary` always maps the complete rich projection (`loomspan-console/internal/traceanalysis/query_frames.go:64-97`, `loomspan-console/internal/traceanalysis/query_frames.go:233-260`).

The MCP `frameDTO` serializes the complete `FrameSummary`, and `LOOMSPAN_query_trace_frames` defaults to a 64-item page because the adapter supplies 64 when the input page size is omitted (`loomspan-console/internal/mcpadapter/trace_contracts.go:105-139`, `loomspan-console/internal/mcpadapter/traces.go:133-162`, `loomspan-console/internal/mcpadapter/traces.go:277-284`). The browser web client separately requests pages of 100 using the same shared service (`loomspan-console/web/src/api/client.ts:257-258`).

Frame tests cover hierarchy, duration, usage, deep nesting, identity validation, every supported ordering, pagination, filter behavior, cursor fingerprint changes, explicit cross-references, and unknown/completeness semantics (`loomspan-console/internal/traceanalysis/calculations_test.go:145-1385`, `loomspan-console/internal/traceanalysis/service_test.go:199-408`). There is no current frame projection field or projection-size regression test.

### 4. Record representation and semantic content

The parser retains each record's `metadata` and `data` as opaque JSON alongside the fields it consumes structurally (`loomspan-console/internal/traceanalysis/model.go:23-68`, `loomspan-console/internal/traceanalysis/parser_test.go:253-270`). A record is treated as an envelope only when its metadata marks `payloadChunked`; the payload assembler reconstructs those chunks into the derived payload store (`loomspan-console/internal/traceanalysis/parser.go:251-262`, `loomspan-console/internal/traceanalysis/parser.go:424-472`).

`RecordSummary` always carries a raw NDJSON address. Its optional inline value is named `InlinePayload`, and its typed fact collections currently cover attempts, retries, validations, failures, reconstructed payloads, and literal matches. There is no general record-data descriptor or plan fact (`loomspan-console/internal/traceanalysis/dto.go:84-135`). `PayloadDescriptor` contains payload ID, opaque payload reference, sequence, content type, chunk count, store offset, and retained store length (`loomspan-console/internal/traceanalysis/dto.go:231-242`).

The opaque reference format currently has two kinds: `PAYLOAD` and `FAILURE_DIAGNOSTIC`. It embeds schema, evidence source, opaque artifact handle, kind, and kind-specific identifier, and validation binds it to the requested evidence source and handle (`loomspan-console/internal/traceanalysis/content_ref.go:13-45`, `loomspan-console/internal/traceanalysis/content_ref.go:64-101`). The MCP projection deliberately removes payload ID and store offsets, returning `payloadRef`, sequence, content type, chunk count, and total length (`loomspan-console/internal/mcpadapter/trace_contracts.go:193-200`, `loomspan-console/internal/mcpadapter/traces.go:320-322`).

Logical queries omit `PAYLOAD_CHUNK_APPENDED` records. Only an envelope root is labeled `logical`; any non-envelope record is labeled `physical` by `recordToSummary`, even when the query representation is `LOGICAL` (`loomspan-console/internal/traceanalysis/query_records.go:14-24`, `loomspan-console/internal/traceanalysis/query_records.go:174-188`, `loomspan-console/internal/traceanalysis/query_records.go:331-353`).

`inlinePayload` is page-wide and Boolean. It is part of the continuation fingerprint. During materialization it considers envelope records only and inlines a reconstructed value only when that individual value is no larger than `maxInlinePayloadBytes` (8 KiB). Every qualifying envelope on the page can be inlined; there is no aggregate inline counter (`loomspan-console/internal/traceanalysis/query_records.go:43-59`, `loomspan-console/internal/traceanalysis/query_records.go:199-226`, `loomspan-console/internal/traceanalysis/limits.go:25-27`). UTF-8 values map to MCP text and non-UTF-8 values map to base64 (`loomspan-console/internal/mcpadapter/traces.go:326-335`).

`LOOMSPAN_read_trace_payload` accepts a returned `payloadRef` and reads either a reconstructed payload or failure diagnostic. Payload range fingerprints include source, payload ID, and maximum bytes; diagnostic fingerprints include the complete reference and maximum bytes. Their `hasMore` is relative to the selected payload/diagnostic length (`loomspan-console/internal/traceanalysis/query_ranges.go:12-103`, `loomspan-console/internal/traceanalysis/query_ranges.go:105-163`). Raw record reading is a separate browser service bounded by one record, while MCP exposes only raw artifact offsets through `LOOMSPAN_read_trace_artifact` (`loomspan-console/internal/traceanalysis/query_ranges.go:173-239`, `loomspan-console/internal/mcpadapter/server.go:55-59`, `loomspan-console/internal/mcpadapter/traces.go:197-243`).

### 5. Plan identity and accepted-attempt lineage

During plan parsing, `DefaultPlanningService` removes any supplied `planId` and inserts one obtained from the framework supplier. When validation accepts the plan, it requires the model attempt's `attemptId` and `retrySequenceId`, stores the plan, and records creation (`loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/runtime/planning/DefaultPlanningService.java:350-359`, `loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/runtime/planning/DefaultPlanningService.java:667-688`).

`DefaultExecutionTraceRecorder` writes `planId`, accepting `attemptId`, and accepting `retrySequenceId` into `PLAN_CREATED` metadata. `PLAN_UPDATED` carries the same plan object's `planId`. Tool inputs/outputs and the plan object itself are supplied as record payload/data (`loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/core/DefaultExecutionTraceRecorder.java:73-100`). `ExecutionPlan` preserves the ID across task, active-task, and status updates (`loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/core/ExecutionPlan.java:12-79`).

Java contract tests assert rejected-versus-accepted attempt identity, framework IDs across primary and nested plan chains, creation metadata, and update identity (`loomspan-spring-boot-starter/src/test/java/com/lokiscale/loomspan/internal/runtime/trace/ExecutionTraceContractTest.java:180-360`). The current Go `knownRecordType` vocabulary recognizes both plan record types, and record filters can select them, but `materializeRecordFacts` does not extract plan metadata into typed facts (`loomspan-console/internal/traceanalysis/enums.go:18-37`, `loomspan-console/internal/traceanalysis/record_facts.go:132-186`). Thus current MCP output exposes plan records as record identity/raw address plus any generic attempt/retry/failure/payload facts; ordinary plan `data` is not a semantic content descriptor.

The shared Java/Go fixture corpus is rooted at `loomspan-console-fixtures/traces` and `loomspan-console-fixtures/expected`. `ConsoleTraceFixtureCorpusTest` generates/validates Java-side fixture semantics, while Go `TestFixtureCorpusMatchesJavaExpectedSemantics` consumes the same corpus (`loomspan-spring-boot-starter/src/test/java/com/lokiscale/loomspan/internal/runtime/trace/ConsoleTraceFixtureCorpusTest.java:1`, `loomspan-console/internal/traceanalysis/fixture_corpus_test.go:67`). The present committed traces exercise attempts, retries, advisor mutations, tools, payload envelopes, failures, usage, and invalidity cases. No committed trace currently contains `PLAN_CREATED` or `PLAN_UPDATED`, so the current producer plan-lineage contract is covered in Java tests but not yet in the cross-language corpus.

### 6. Filter vocabulary and literal search

The Go parser has closed current-release vocabularies for 33 record types, seven frame types, three outcomes, and three usage precision values (`loomspan-console/internal/traceanalysis/enums.go:3-130`). Record-query validation rejects any unknown `filter.types` value. Frame-query validation similarly rejects unknown frame types and orders (`loomspan-console/internal/traceanalysis/query_records.go:307-328`, `loomspan-console/internal/traceanalysis/query_frames.go:64-87`). Validation-status values are compared as strings and have no closed validator in these query paths.

The MCP schema explicitly enumerates only top-level frame `order` and record `representation`. The generated nested `filter.types`, `filter.validationStatus`, and frame filter vocabularies are not enriched with enum arrays (`loomspan-console/internal/mcpadapter/traces.go:36-51`, `loomspan-console/internal/mcpadapter/trace_contracts.go:256-271`). Existing schema tests assert closed object shapes, top-level bounds, required arguments, and absence of internal identity fields (`loomspan-console/internal/mcpadapter/trace_contracts_test.go:13-75`).

MCP literal search is currently the record filter. It checks whether the exact query bytes occur in JSON-encoded `metadata` or `data`; matching is case-sensitive because it uses `bytes.Contains`. Returned match offsets are relative to the selected encoded field (`loomspan-console/internal/traceanalysis/query_records.go:245-304`, `loomspan-console/internal/traceanalysis/record_facts.go:170-185`). Because the match is part of record selection, results are the full rich record DTO, and page continuation represents record-index pagination rather than a separate bounded-search work state.

The standalone `Search` service, currently exposed to the browser but not MCP, scans physical raw record bytes first and reconstructed payload-store bytes second. It is exact byte/case-sensitive matching, caps each call at 8 MiB or 10,000 processed records, and can return an empty page with `hasMore=true` and a cursor carrying record/payload byte progress (`loomspan-console/internal/traceanalysis/search.go:14-31`, `loomspan-console/internal/traceanalysis/search.go:91-174`, `loomspan-console/internal/traceanalysis/search.go:176-274`, `loomspan-console/internal/traceanalysis/limits.go:37-50`). Its result includes sequence, record type/frame when scanning a physical record, match offset/length, and searched field, but its generic `Page` has no coverage metadata (`loomspan-console/internal/traceanalysis/dto.go:244-262`). Browser routing maps it at `/api/console/v1/traces/analysis/search` (`loomspan-console/internal/browserapi/router.go:192-197`, `loomspan-console/internal/browserapi/trace_analysis.go:315-337`).

### 7. Browser, MCP, and web consumers

The browser `TraceAnalysisService` interface includes summary, frames, records, specialized attempt/retry/validation/failure/payload/gap/uncertainty queries, standalone search, payload ranges, and raw-record ranges. It intentionally does not register a raw-artifact range route (`loomspan-console/internal/browserapi/router.go:35-55`, `loomspan-console/internal/browserapi/trace_analysis_test.go:151-157`). MCP uses the same summary/frame/record/payload services but presents a smaller, general six-tool trace surface (`loomspan-console/internal/mcpadapter/server.go:49-59`).

The web client requests logical records, canonical frame pages, payload ranges by payload ID, raw-record ranges by sequence, and the standalone search endpoint. Its TypeScript contracts mirror the existing rich frame and record DTOs (`loomspan-console/web/src/api/client.ts:256-273`, `loomspan-console/web/src/api/contracts.ts:257-304`). Web component tests separately exercise plan, model, tool input, step, failure, and raw record display based on those browser DTOs (`loomspan-console/web/src/observability/TraceRecords.plan.test.tsx:1`, `loomspan-console/web/src/observability/TraceRecords.model.test.tsx:1`, `loomspan-console/web/src/observability/TraceRecords.toolInput.test.tsx:1`).

Browser fixture contracts are byte-for-byte committed under `loomspan-console/browser-fixtures/trace-analysis`, and `TestBrowserTraceAnalysisFixtureCorpusMatchesCommittedInventoryByteForByte` is their executable authority (`loomspan-console/internal/browserapi/contracts_test.go:138-169`). MCP has a semantic fixture suite that compares browser and MCP projections for shared facts and verifies content references/ranges and error semantics (`loomspan-console/internal/mcpadapter/trace_semantic_fixtures_test.go:93-675`). `TestBrowserAndMCPJoinOneAcquisitionHandleAndCapacityCharge` confirms that both adapters resolve through the centralized artifact service (`loomspan-console/internal/mcpadapter/trace_joined_adapters_test.go:86-223`).

### 8. MCP fallback, discovery, capabilities, and protocol tests

Every successful MCP result is returned as a structured envelope plus one `TextContent` fallback (`loomspan-console/internal/mcpadapter/contracts.go:145-148`). Trace result fallbacks call `json.Marshal` over the full mapped result; range fallback also includes the full bounded content (`loomspan-console/internal/mcpadapter/traces.go:246-247`, `loomspan-console/internal/mcpadapter/traces.go:354-358`). Current tests require bounded range content to survive in the fallback and measure maximum-range serialization and allocation behavior (`loomspan-console/internal/mcpadapter/traces_test.go:21-31`, `loomspan-console/internal/mcpadapter/trace_range_http_test.go:50-170`).

The `loomspan.trace-inspection.v1` capability is defined by the five parsed trace tools, while `loomspan.raw-artifact-inspection.v1` is defined only by the raw reader. Capability tests verify tool membership, semantic-fixture membership, and a reviewed JSON manifest (`loomspan-console/internal/mcpadapter/capabilities.go:10-34`, `loomspan-console/internal/mcpadapter/capabilities_test.go:15-103`, `loomspan-console/internal/mcpadapter/contracts/trace-capabilities.json:1`). The current repository has no committed serialized `tools/list` byte measurement or discovery-size budget assertion.

MCP is assembled as a stateless Streamable HTTP server with read-only annotations, bounded request bodies, authentication generation checks, and no resources/prompts surface (`loomspan-console/internal/mcpadapter/server.go:61-73`). Server tests exercise a compatible 2025 protocol initialization/list/call flow and the SDK's current stateless path; the pinned conformance harness runs its applicable foundation scenarios across both supported revisions (`loomspan-console/internal/mcpadapter/server_test.go:30-107`, `loomspan-console/internal/mcpadapter/server_test.go:109-225`, `loomspan-console/mcp-conformance/README.md:1-15`).

### 9. Documentation and agent-evaluation surfaces

The Console README documents the current six trace tools, independent `complete`/`hasMore` semantics, trace-ID resolution, opaque continuation/reference recovery, 64-item MCP pages, 64 KiB default and 16 MiB maximum reads, and full fact-complete text fallbacks (`loomspan-console/README.md:238-305`). The runtime-debugging skill currently starts with all required capability families, routes mainly to failure/latency/usage/path playbooks, and names payload references/ranges in its evidence guidance (`loomspan-console/agent-skills/loomspan-runtime-debugging/SKILL.md:10-76`). Its MCP guide documents `list traces -> select traceId -> inspect/query/read by traceId` and describes payload references only for reconstructed payloads or diagnostics (`loomspan-console/agent-skills/loomspan-runtime-debugging/references/mcp-tool-guide.md:17-57`).

The current deterministic agent-eval cases cover failed, slow, expensive, unfamiliar-path, ambiguity, evidence expiry/unavailability, target/authentication/protocol conditions, capability degradation, restart, skill-without-MCP, MCP-without-skill, and adversarial data. There is no committed PR-28-specific final-plan, accepted-attempt, tool-content, structured-output, or literal-search case in `loomspan-console/agent-evals/cases`.

## Contract Classification

The repository's canonical categories come from `ai/thoughts/framework-feature-design-lens.md:15-43`.

| Surface | Technical exposure and current evidence | Current classification |
| --- | --- | --- |
| Eight allowlisted `com.lokiscale.loomspan.api` types | Closed allowlist and README support statement; none of the PR-28 producer types are in these signatures (`LoomspanPublicSurfaceArchitectureTest.java:27-37`, `README.md:157`) | Application API; no current PR-28 surface delta |
| Loomspan-specific SPI | Architecture test asserts no SPI package/type (`LoomspanPublicSurfaceArchitectureTest.java:316-321`) | Supported SPI: none |
| `DefaultPlanningService`, `ExecutionStateService`, `DefaultExecutionTraceRecorder`, `ExecutionPlan`, `TraceRecordType` | Several are technically public and some are Spring beans/interfaces/constructors, all under `com.lokiscale.loomspan.internal`; relevant bean methods have no `@ConditionalOnMissingBean` (`LoomspanAutoConfiguration.java:315-329`) | Internal or accidentally exposed implementation |
| Skill YAML/application `loomspan.*` configuration | PR 28 does not presently name a new skill YAML property or application configuration key | Configuration and manifest contracts: no live configuration delta identified |
| Portable runtime-debugging skill frontmatter/capability guidance and `trace-capabilities.json` | Shipped/embedded client guidance and capability-to-tool manifest with executable drift tests | Configuration and manifest contracts for the packaged debugging experience; also documentation of the unreleased MCP surface |
| Canonical NDJSON records and `consoleCompatibilityVersion` | Java writes; Go imports/parses; shared fixture corpus validates the current producer/consumer pair. A complete artifact is portable only to the exact same compatibility version | Ephemeral diagnostic format, with narrow same-version portable artifact behavior; not a cross-version persisted interchange contract |
| Go derived indexes, manifests, opaque handles/references/cursors, imported catalog | Process-local, owner/operation/request-bound, regenerated from the artifact, removed on shutdown/expiry | Internal or accidentally exposed implementation / ephemeral current-process diagnostics |
| Browser `/api/console/v1/...` DTOs and routes | Used by the bundled TypeScript web application and byte-for-byte browser fixtures | Internal Console application contract with verified in-repository consumers; not supported Java Application API/SPI |
| MCP tool names, schemas, structured/text result shapes, capability IDs | Documented, schema-tested, semantically fixture-tested, and advertised to MCP clients; roadmap explicitly says the surface is unreleased development work | Technically exposed unreleased internal product protocol; supported status for a released compatibility promise is not established |
| Java observability REST/SSE acquisition/problem/NDJSON boundary | Go observability/acquisition clients and committed application REST/SSE/artifact fixtures consume it; Java and Go tests verify it | Protected cross-component protocol consumer requiring coordinated producer/consumer fixture changes when its fields/query behavior change |

No compatibility shim is present in the current PR-27 plan producer path. Historical roadmap and ticket text explicitly classify this next MCP/Console work as an in-place pre-v1 contract replacement, while the public Java allowlist remains unchanged (`ai/thoughts/phases/2026-08-15-loomspan-llm-trace-understanding-roadmap.md:24-35`).

## Executable Authorities and Fixture Inventory

- `loomspan-console/internal/traceinventory/service_test.go` — unified discovery, completeness, collision, ordering/pagination, and stale-selection behavior.
- `loomspan-console/internal/artifact/import_test.go` — imported admission, validated metadata publication, bounds, ownership, and concurrency.
- `loomspan-console/internal/traceanalysis/service_test.go` and `continuation_test.go` — frame/record projections, filters, logical/physical representation, pages, fingerprints, ranges, and owner binding.
- `loomspan-console/internal/traceanalysis/search_test.go` — raw-record and reconstructed-payload literal search, pages, work continuations, and limits.
- `loomspan-console/internal/traceanalysis/content_ref_test.go` and `range_test.go` — opaque reference ownership/kinds and exact bounded range behavior.
- `loomspan-console/internal/traceanalysis/fixture_corpus_test.go` plus `loomspan-spring-boot-starter/.../ConsoleTraceFixtureCorpusTest.java` — cross-language trace semantic corpus under `loomspan-console-fixtures/`.
- `loomspan-console/internal/mcpadapter/trace_contracts_test.go` — MCP schema closure, bounds, required arguments, and model-facing identity.
- `loomspan-console/internal/mcpadapter/trace_semantic_fixtures_test.go` — shared browser/MCP semantic facts, opaque references, ranges, and safe domain errors.
- `loomspan-console/internal/mcpadapter/trace_range_http_test.go` — serialized maximum-range correctness, deadline, concurrency, and allocation envelope.
- `loomspan-console/internal/browserapi/contracts_test.go` — byte-for-byte browser fixtures under `loomspan-console/browser-fixtures/`.
- `loomspan-console/internal/mcpadapter/server_test.go` and `loomspan-console/mcp-conformance/` — protocol initialization/list/call behavior and pinned conformance scenarios.
- `loomspan-spring-boot-starter/src/test/java/com/lokiscale/loomspan/architecture/LoomspanPublicSurfaceArchitectureTest.java` — executable Java API/SPI classification authority.

## Architecture Documentation

The current separation of responsibility is consistent across code and documentation:

- Java owns authoritative execution recording, finalized trace catalog metadata, artifact publication, and the current trace schema.
- The Go artifact service owns process-local admission, immutable installed copies, capacity, expiry, leases, opaque handles, and target/imported ownership.
- The Go trace-analysis service owns parsing, same-version validation, derived indexes, mechanical relationships/calculations, opaque continuations/content references, and bounded byte reads.
- Browser and MCP adapters map the same neutral facts. They may expose different operation groupings, but shared facts and errors are not recalculated by the adapters.
- MCP remains stateless, trace-ID based, read-only, and hides target scope, owner IDs, artifact handles, source paths, and storage topology.
- The Agent Skill is optional navigation/evidence guidance. MCP tools are the complete inspection transport.

The current normal parsed path is therefore:

```text
list compact trace candidates
  -> resolve one traceId
  -> get summary
  -> query rich frames or records
  -> read a reconstructed payload/diagnostic by payloadRef
```

Ordinary record data currently branches out of that semantic path into browser raw-record reads or MCP raw-artifact reads.

## Historical Context (from ai/thoughts/)

- `ai/thoughts/phases/2026-08-15-loomspan-llm-trace-understanding-roadmap.md:24-55` records the completed trace-ID cleanup, unreleased/pre-v1 posture, and target descriptor-first semantic-content experience.
- `ai/thoughts/phases/2026-08-15-loomspan-llm-trace-understanding-roadmap.md:202-225` records PR 27's framework-owned plan identity and accepted-attempt lineage as the downstream producer contract.
- `ai/thoughts/phases/2026-08-15-loomspan-llm-trace-understanding-roadmap.md:563-607` records tool/schema cost, explicit search semantics, compact frame orientation, and range-completeness workstreams.
- `ai/thoughts/phases/2026-08-15-loomspan-llm-trace-understanding-roadmap.md:632-652` classifies the live observations that became PR 28: ordinary content not exposed, large fallback duplication, missing enum publication, rich frame cost, inventory ambiguity, aggregate inline growth, and unconsumed plan identity.
- `ai/thoughts/phases/loomspan_llm_trace_understanding_workflows.md:226-310` defines plan selection as mission lineage followed by framework plan identity and accepted-attempt linkage, with legacy evidence explicitly degraded.
- `ai/thoughts/phases/loomspan_llm_trace_understanding_workflows.md:720-850` defines the descriptor-first content and complete bounded selection semantics used by the ticket.
- `ai/thoughts/phases/loomspan_skill_mcp_questions.md:521-669` preserves the two live walkthroughs and measured response sizes that motivated the accepted ticket.
- `ai/thoughts/framework-feature-design-lens.md:15-43` classifies traces as ephemeral diagnostics, allows only exact same-version portable artifact use, and distinguishes technical exposure from supported API/SPI status.

## Related Research

No prior documents existed under `ai/thoughts/research/` when this research was performed. The active roadmap, workflow catalog, question inventory, framework design lens, and PR-28 ticket are the related historical sources listed above.

## Open Questions

These are areas where the current repository does not yet contain a settled executable fact:

1. The ticket requires an explicit ordering/admission fact for unified inventory, but the current code only exposes both `FinalizedAt` and artifact `AcquiredAt`; no PR-28 inventory field name or ordering enum exists yet.
2. The current artifact parser does not define which validated trace record establishes imported `entrySkill`; the committed trace schema has no processor-owned derivation rule for it.
3. The exact per-value, aggregate inline, compact-frame response, MCP fallback, and `tools/list` serialized byte budgets are not yet present in centralized constants or committed measurements.
4. The exact semantic descriptor field names, content-kind vocabulary, ordinary-`data` storage/index representation, and generalized reader input name have not yet been encoded in live DTOs.
5. The exact typed plan landmark DTO, including whether ordering is expressed as sequence alone or an additional version fact, is not yet present in Go analysis contracts.
6. Validation-status accepted values are used by producer records and tests, but no single closed Go/MCP query enum is currently declared.
7. Browser and MCP currently use different literal-search entry points. The repository has not yet encoded the PR-28 unified search envelope and coverage policy.
8. The current application trace-list REST boundary has no server-side filters. The repository does not yet establish whether PR-28 filters remain entirely in the Go unified inventory or extend that protected Java-to-Go protocol.
9. No current-producer plan-chain trace has yet been added to the shared Java/Go fixture corpus, so its final sanitized topology and size are not established in the repository.
