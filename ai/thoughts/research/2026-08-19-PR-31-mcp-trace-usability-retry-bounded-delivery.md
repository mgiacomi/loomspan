---
date: 2026-08-19T22:11:32-07:00
researcher: Codex (GPT-5)
git_commit: 5c49ec9a0e9b4909e6d5bd90f7da9273432221e2
branch: main
repository: loomspan
topic: "PR 31 — MCP Trace Usability, Retry Correctness, and Bounded Delivery"
tags: [research, codebase, loomspan-console, mcp, trace-analysis, trace-inventory, agent-skill]
status: complete
last_updated: 2026-08-19
last_updated_by: Codex (GPT-5)
---

# Research: PR 31 — MCP Trace Usability, Retry Correctness, and Bounded Delivery

**Date**: 2026-08-19 22:11:32 PDT
**Researcher**: Codex (GPT-5)
**Git Commit**: 5c49ec9a0e9b4909e6d5bd90f7da9273432221e2
**Branch**: main
**Repository**: loomspan

## Research Question

Research the current codebase for the implementation ticket
`ai/thoughts/tickets/loomspan-console-pr-31-mcp-trace-usability-retry-correctness-and-bounded-delivery.md`, documenting the live implementation, tests, contracts, consumers, and historical context relevant to retry correctness, MCP trace usability, bounded delivery, lifecycle timestamps, record-type discovery, and frame outcomes.

## Summary

The current Console already provides the six finalized-trace MCP tools, closed record and frame vocabularies, descriptor-first content, opt-in bounded inline content, exact bounded semantic/raw ranges, item-count pagination, opaque continuations/content references, compact authored output schemas, deterministic text fallbacks, read-only annotations, and an Agent Skill that documents the intended investigation workflow. The focused `traceanalysis`, `traceinventory`, and `mcpadapter` test suites pass at the researched commit.

The current retry summary uses retry-sequence cardinality: `buildAttemptResults` emits one `retryResult` for every distinct `retrySequenceId`, and the manifest assigns `RetryCount: len(retryResults)`. Frame `DirectRetryCount` similarly uses the number of distinct retry-sequence IDs associated with the frame. The validated attempt graph already contains the explicit sequence membership and positive consecutive attempt numbers needed to identify later attempts without record adjacency or planning records.

COMPACT is the service default and removes duration, usage, and detailed identity collections from the neutral frame summary. Structured MCP output omits those optional fields, while the text fallback still prints duration fields using `-` for nil. The README and packaged Agent Skill explain COMPACT/DETAILED and `inlineContent`; the installed MCP tool descriptions and generated input-property schemas do not currently include those explanations. The range input schema and handler currently require exactly one of `start` or `continuation`, so an initial read with both omitted is rejected.

Item pages are complete at item boundaries and use `hasMore` plus opaque continuations, but the page boundary is currently only the caller/default item count. There is no encoded-response-byte stopping rule. Navigation text fallbacks are independently cut to 64 KiB and append an omission notice; the structured page is not reduced with the fallback. The repository has an exact 20,304-byte `tools/list` HTTP snapshot against a 20 KiB (20,480-byte) test budget, representative DTO-only frame/record size tests, and exact range HTTP tests, but it does not contain the ticket's full worst-case serialized call-result measurement matrix or recorded Codex/second-host overflow thresholds.

Inventory preserves finalization, acquisition, and import as distinct fields and filter/order axes. The code maps artifact acquisition time to `acquiredAt` for target evidence and to `importedAt` for imported evidence; finalization comes from terminal trace/catalog metadata. README and Agent Skill call the values independent, while the installed list-tool description does not define their lifecycle meanings.

`LOOMSPAN_get_trace` has `recordCount` and semantic aggregate counts but no `recordCountsByType`. The processor sees and indexes every physical record and the code owns a closed 33-value record-type enumeration, but it does not accumulate a type histogram. Frame outcomes are plural throughout the neutral DTO and MCP contract; live validation accepts at most one `FRAME_CLOSED` per frame and only that record contributes a close status, so the current validated graph can expose zero or one distinct outcome, not multiple authoritative close outcomes.

## Current-State Matrix

| Ticket area | Current implementation at researched commit | Existing evidence |
| --- | --- | --- |
| Retry accounting | Trace `retryCount` is distinct retry-sequence count; frame `directRetryCount` is distinct frame-associated retry-sequence count. | `processor.go:435`, `query_frames.go:214`, `calculations_test.go:630-651` |
| Frame projections | COMPACT defaults in the service and omits detailed data in structured output; fallback prints nil duration values as `-`. | `query_frames.go:82-88`, `query_frames.go:221-235`, `traces.go:489-493`, `traces_test.go:112-142` |
| Inline content | Descriptor-first default; explicit `inlineContent` uses 8 KiB/value and 32 KiB aggregate source-byte budgets in record order with typed omission reasons. | `query_records.go:209-238`, `query_records.go:383-407`, `limits.go:28-29` |
| Initial range read | MCP requires `start` XOR `continuation`; handler rejects omission of both. | `traces.go:129-140`, `traces.go:287-297`, `trace_contracts_test.go:29-37` |
| Range correctness | Default is 64 KiB, maximum is 16 MiB, oversized explicit requests return `LIMIT_EXCEEDED`, and successful ranges retain exact source offsets with TEXT or BASE64 encoding. | `limits.go:33-39`, `range.go:262-304`, `traces_test.go:23-31` |
| Semantic pagination | Frames and records stop before the next item after reaching item count and produce an opaque continuation. There is no serialized-byte page boundary. | `query_frames.go:165-207`, `query_records.go:150-253` |
| Text fallback delivery | Inventory/frame/record fallbacks are sliced to 64 KiB and state that additional structured items were omitted; range fallback preserves the entire selected content. | `traces.go:546-553`, `traces_test.go:23-50` |
| Discovery budget | Full HTTP `tools/list` is snapshotted at 20,304 bytes under a 20,480-byte budget, leaving 176 bytes under that test ceiling. | `output_schemas.go:12`, `server_test.go:31-108` |
| Lifecycle times | Finalized, acquired, and imported times are independent values and independent query/order dimensions; installed target/import evidence shares a source-time field that is projected according to source. | `traceinventory/dto.go:17-23`, `traceinventory/service.go:305-309`, `traceinventory/service.go:379-412` |
| Record-type discovery | A closed 33-type enumeration and total physical `recordCount` exist; no per-type histogram exists in the manifest, neutral summary, MCP DTO/schema, or fallback. | `enums.go:3-59`, `index_writer.go:112-124`, `trace_contracts.go:100-122` |
| Frame outcomes | Plural sets/arrays are exposed, but duplicate frame close is rejected and only `FRAME_CLOSED` status populates the set, establishing a zero-or-one current invariant. | `frames.go:88-110`, `frames.go:131-151`, `dto.go:75`, `trace_contracts.go:125-161` |

## Detailed Findings

### 1. Trace Processing and Retry Identity

The Java producer creates one `ModelTraceContext` per retry sequence and increments `attemptNumber` for every physical attempt. Attempt one receives `INITIAL`; later attempts receive `PROVIDER_RETRY` or `SEMANTIC_RETRY` according to the provider attempt number (`loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/core/ModelTraceContext.java:60-70`). It records `retrySequenceId`, `attemptId`, `attemptNumber`, `attemptReason`, and `providerAttemptNumber` together (`ModelTraceContext.java:116-123`). The recorder copies that metadata onto request, response, and failure records (`DefaultExecutionTraceRecorder.java:42-62`).

The Go attempt graph consumes only explicit attempt and retry identifiers. A new sequence must begin at attempt/provider number one with reason `INITIAL`. A later attempt must increment `attemptNumber`, follow a terminal response/failure for the previous attempt, and satisfy provider-versus-semantic retry lifecycle rules (`loomspan-console/internal/traceanalysis/attempts.go:67-114`). The graph rejects identity changes and invalid lifecycle repetition (`attempts.go:115-145`). `PLAN_RETRY_REQUESTED` is not part of `isModelRecord`, whose members are only request sent, response received, and model attempt failed (`processor.go:514-516`).

`buildAttemptResults` creates one attempt result per validated attempt and also groups usage by `retrySequenceId`. It appends one retry result the first time it sees each distinct sequence (`processor.go:521-575`). The manifest sets `AttemptCount` from attempt rows and `RetryCount` from the number of these grouped retry rows (`processor.go:426-440`). `GetSummary` reads these manifest values into the neutral `TraceSummary`, after which the MCP adapter copies them without recalculation (`traceanalysis/query_facts.go:24-78`, `mcpadapter/traces.go:383-385`).

The current calculation test explicitly describes and asserts the grouped meaning: two attempts with distinct IDs under one retry sequence produce two attempts and manifest `RetryCount == 1`, with the failure message calling it one retry sequence (`calculations_test.go:630-651`). Existing invalid identity, lifecycle, and numbering tests follow immediately in the same test file (`calculations_test.go:682-732`).

Frame association records distinct `attemptId` and `retrySequenceId` values from every record carrying a frame (`frames.go:129-151`). `populateFrameCounts` assigns direct attempts from `AttemptIDs` and direct retries from `RetrySequenceIDs` (`query_frames.go:211-219`). Because these are separately deduplicated sets, they do not retain which frame-owned attempt number is later than the initial attempt.

### 2. Frame Projection, Time, and Outcome Semantics

`QueryFrames` defaults absent projection to `COMPACT`. COMPACT is restricted to canonical ordering; DETAILED can use canonical, duration-descending, or usage-descending indexes (`query_frames.go:74-107`). Every matching frame is first converted to a complete neutral summary and count fields are populated. COMPACT then nils inclusive/self duration, zeros usage values/completeness, and removes detailed skill, attempt, retry-sequence, validation, failure, gap, and uncertainty collections (`query_frames.go:166-188`, `query_frames.go:221-235`). Hierarchy, frame type, route, open timestamp, close timestamp, outcome, and direct aggregate counts remain.

The MCP mapping uses pointer fields and `omitempty`, so compact structured JSON does not serialize duration or usage fields. Its test asserts absence of `inclusiveDurationMillis`, `directUsage`, and detailed collections while retaining hierarchy and direct counts (`mcpadapter/traces_test.go:112-142`). The authored compact output schema requires only frame ID, child IDs, type, open timestamp, and outcomes; it is intentionally open for the remaining validated DTO fields (`output_schemas.go:163-171`).

The text fallback always formats `closedTimestampMillis`, `inclusiveDurationMillis`, and `selfDurationMillis`. `optionalValue` maps nil to `-` and a numeric zero to `unknown` (`traces.go:489-493`, `traces.go:530-543`). This same fallback is used for both projections and currently has no page-level projection-omission explanation.

The parser converts the trace timestamp into epoch milliseconds; the `Record` model describes it as the record timestamp truncated to milliseconds (`model.go:25-31`). Frame durations are elapsed millisecond subtraction between close and open timestamps, and self duration subtracts complete non-overlapping immediate child intervals (`frames.go:254-305`, `frames.go:342-391`). The nested-frame calculation tests use epoch-second fixture timestamps and assert the derived elapsed durations (`calculations_test.go:1196-1255`).

Frames are opened once and closed at most once. A second close is rejected (`frames.go:88-110`). Only a `FRAME_CLOSED` record adds its metadata `status` to the outcome set (`frames.go:131-151`). Therefore an accepted open-but-not-closed frame has zero outcomes, and an accepted closed frame has at most one distinct status. The Java recorder emits one close record using the caller-supplied close metadata (`DefaultExecutionTraceRecorder.java:33-39`). Despite that invariant, `frameResult`, `FrameSummary`, MCP `frameDTO`, filters, output schema, and fallback all use plural `Outcomes`/`outcomes` (`model.go:188-213`, `dto.go:52-85`, `trace_contracts.go:125-161`).

### 3. Record Queries and Semantic Content

Record queries default to LOGICAL representation. They scan the physical record index, omit physical chunk records in LOGICAL mode, filter each complete decoded record, and stop when the item-count page is full before appending another item (`query_records.go:63-93`, `query_records.go:150-190`). A continuation resumes at the next physical index position (`query_records.go:240-253`).

Semantic content remains descriptor-first because `InlineContent` defaults false. For a reconstructed envelope or ordinary record `data`, the service creates a descriptor containing role, content type, encoding, retained bytes, availability, completeness, inline eligibility, and an opaque content reference (`query_records.go:209-238`). The DTO and MCP mapping preserve those facts (`dto.go:137-158`, `mcpadapter/traces.go:445-454`).

When requested, inline selection walks returned records in page order. Values over 8 KiB get `PER_VALUE_LIMIT`; otherwise a value that would exceed the 32 KiB aggregate source-byte budget gets `AGGREGATE_LIMIT`; eligible complete values are copied in full (`query_records.go:383-407`). Binary inline bytes are base64-encoded only at MCP mapping time (`traces.go:445-454`), and the mapping test covers this transport representation (`traces_test.go:160-165`).

The generated input schema exposes an `inlineContent` boolean property because it is present on `queryTraceRecordsInput`, and a contract test includes the property in the exact developer-intent surface (`trace_contracts.go:47-57`, `trace_contracts_test.go:70-106`). No description is assigned to that property. The installed tool description only says descriptor-first records or literal matches (`traces.go:43-49`). The README gives the 8/32 KiB behavior and omission semantics (`README.md:304-313`), and the packaged Agent Skill says inline content is bounded per value and page (`agent-skills/loomspan-runtime-debugging/references/mcp-tool-guide.md:32-39`).

Opaque content-reference encoding is owned by `traceanalysis/content_ref.go`; outward code treats it only as a bounded string and maps stale/invalid references to re-query guidance (`mcpadapter/traces.go:339-356`). Continuations likewise include operation, owner, handle, query fingerprint, and position in internal code while remaining an opaque outward string.

### 4. Exact Range Reads

Both read tools share `traceRangeInput`. `prepareRangeSchema` adds an exact-one-of branch for `start` and `continuation`; semantic reads additionally require `contentRef` (`mcpadapter/traces.go:129-140`). The handler duplicates the shape check, rejecting both controls and rejecting omission of both (`traces.go:280-297`). An explicit zero is represented by a non-nil pointer and reaches the service as start zero.

The transport-neutral range service itself naturally uses zero when no cursor changes `req.Start`, and `resolveRangeBounds` applies the 64 KiB default when `MaxBytes` is absent/zero (`query_ranges.go:15-70`, `range.go:262-291`). A maximum over 16 MiB returns `LIMIT_EXCEEDED`; a request shorter than the remaining content is not clamped to the maximum, while a request reaching the end is shortened only to actual remaining source length (`range.go:262-291`).

For text content, a complete valid UTF-8 requested slice is returned as TEXT. A slice that is not complete valid UTF-8 is returned as BASE64 without dropping bytes or moving its reported source offsets (`range.go:294-304`). Results expose `actualStart`, `actualEnd`, `totalLength`, `hasMore`, and a continuation when source bytes remain (`range.go:44-104`, `range.go:188-248`). The range fallback places the complete returned content after the exact boundary header and is not passed through navigation-text bounding (`mcpadapter/traces.go:329-331`).

### 5. Item Pagination and Serialized Response Bounds

The MCP adapter accepts page sizes from one through 64 and defaults an omitted trace page size to 64 (`traces.go:359-368`). Trace inventory also defaults to 64 (`traceinventory/service.go:17-20`, `traceinventory/service.go:269-302`). Frame and record query services append whole DTO items and set `hasMore` before the next complete matching item, so their structured outputs do not split a JSON item (`query_frames.go:165-208`, `query_records.go:154-253`).

There is no encoded MCP-response-byte accumulator in inventory, frame, record, or search traversal. Caller `pageSize` is the only semantic item boundary. The constants `MaxCompactResponseBytes` (64 KiB) and `MaxDescriptorResponseBytes` (128 KiB) appear in representative DTO tests, not in item-page construction (`limits.go:28-31`, `mcpadapter/traces_test.go:92-110`).

Text fallback is built after the complete structured page. Inventory, frame, and record fallbacks call `boundedNavigationText`, which slices the encoded Go string at 64 KiB and appends “additional structured items omitted from text fallback” (`mcpadapter/traces.go:471-516`, `traces.go:546-553`). This can make the fallback describe fewer items than the structured page and the slice position is not aligned to a fallback line or UTF-8 boundary. The existing test asserts only that the result remains within the compact byte bound (`traces_test.go:34-50`).

Every success is returned as one MCP text content block plus the structured success envelope (`contracts.go:145-148`). Complete generated output DTOs are validated before publication even though authored discovery schemas are compact (`output_schemas.go:14-48`).

The full HTTP `tools/list` test records:

- 12 tools;
- a 20,480-byte discovery ceiling;
- an exact current serialized response of 20,304 bytes;
- a previous 34,371-byte reference; and
- byte-for-byte equality with `internal/mcpadapter/testdata/tools-list-response.json`.

These assertions are in `mcpadapter/server_test.go:31-108`. The current snapshot exposes COMPACT/DETAILED as enum values and `inlineContent` as an undescribed boolean, while its frame and record tool prose is the short text from `traces.go:39-49`.

Current response-size coverage does not include the ticket's enumerated full serialized MCP success results (structured output plus text fallback) for 64 inventory items, 64 compact/detailed frames, 64 descriptors, maximum inline page, or default content/raw ranges. The representative test serializes domain DTOs only (`traces_test.go:92-110`); range HTTP tests verify byte preservation rather than a host budget (`trace_range_http_test.go:93`, `trace_range_http_test.go:211-246`). No checked-in result records a Codex overflow threshold or a second supported host threshold.

### 6. Inventory Lifecycle Times

The inventory query has separate finalized, acquired, and imported ranges and exposes `FINALIZED_DESC`, `ACQUIRED_DESC`, and `IMPORTED_DESC` (`traceinventory/dto.go:17-47`). An installed artifact instance stores terminal `FinalizedAt` separately from the artifact lookup's `AcquiredAt`. For target evidence, the source time projects to `acquiredAt`; for imported evidence, the same storage acquisition fact projects to `importedAt` (`traceinventory/service.go:305-309`, `traceinventory/service.go:379-412`). Target catalog-only entries have finalization but no acquisition time.

Matching applies finalized filters to execution finalization, acquired filters only to target source time, and imported filters only to imported source time (`service.go:330-355`). Sort selection likewise uses the relevant source time only for the corresponding source and retains finalization as the secondary ordering fact (`service.go:461-481`, `service.go:515-533`). The MCP DTO returns all three optional times without inference (`mcpadapter/trace_contracts.go:73-85`, `traces.go:369-378`).

README says the fields are independent (`README.md:288-296`), and the Agent Skill repeats that none substitutes for another (`mcp-tool-guide.md:75-82`). The installed list-tool description says only that it lists finalized traces and distinguishes `complete` from pagination; generated date-time properties have no descriptions (`mcpadapter/traces.go:22-27`).

### 7. Trace Summary and Record-Type Discovery

The processor appends one record index row before skipping physical-only chunk records from semantic calculation. `appendRecordRow` increments `recordCount`, so the manifest count covers every validated physical NDJSON record (`processor.go:170-193`, `index_writer.go:112-124`). The closed enumeration contains 33 current record types and rejects unknown values (`enums.go:3-68`).

The processor does not maintain a map keyed by `TraceRecordType`. The manifest has only total `RecordCount` and semantic counts (`manifest.go:19-42`). `TraceSummary`, `traceSummaryDTO`, the compact output schema, mapper, and fallback likewise contain total record count but no histogram (`dto.go:23-48`, `trace_contracts.go:100-122`, `output_schemas.go:156-161`, `traces.go:383-385`, `traces.go:483-486`). The closed record-type list is currently exposed only as the `filter.types` enum on `LOOMSPAN_query_trace_records` (`traces.go:85-92`).

Semantic counts remain separate calculations: attempts/retry sequences come from the attempt graph, failures from the failure graph, gaps/uncertainties from frame analysis, and terminal outcome from `TRACE_COMPLETED` (`processor.go:297-365`, `processor.go:426-440`). Physical record types such as `PLAN_RETRY_REQUESTED` are therefore not used to populate current retry summary fields.

### 8. MCP Contract, Security, and Agent Skill Responsibilities

All 12 tools receive read-only, idempotent, non-destructive, closed-world annotations. The server test verifies these flags and verifies that no custom resource templates are advertised (`server_test.go:180-218`). Tool success/domain failures use one structured envelope arm and one deterministic text block (`contracts.go:145-161`). Returned trace content is labeled inert diagnostic data in tool descriptions and range fallback; the Agent Skill has a dedicated untrusted-content section (`agent-skills/loomspan-runtime-debugging/SKILL.md:66-77`).

The README documents the ordinary workflow as list traces, get trace, compact frames, descriptor/search records, selected content read (`README.md:238-254`, `README.md:288-313`). The Agent Skill supplies richer selection and evidence judgment, capability handling, join guidance, error recovery, and raw-forensics boundaries (`agent-skills/loomspan-runtime-debugging/SKILL.md:12-64`, `references/mcp-tool-guide.md:1-82`). Tool descriptions currently communicate immediate purpose and some pagination/range facts, but do not locally state the entire workflow, COMPACT-versus-DETAILED omissions, inline budgets, or lifecycle time definitions (`mcpadapter/traces.go:22-63`).

The current code introduces no MCP resources or templates and no application-facing Java API. The affected Go packages are under `loomspan-console/internal`, and the Java producer types involved in the trace protocol are under `com.lokiscale.loomspan.internal`.

## Contract and Compatibility Classification

The repository design lens classifies surfaces as Application API, Supported SPI, Configuration and manifest contracts, Persisted or serialized contracts, Ephemeral diagnostic formats, or Internal/accidentally exposed implementation (`ai/thoughts/framework-feature-design-lens.md:19-30`). Applied to the researched code:

| Surface | Current classification and evidence | Current consumers |
| --- | --- | --- |
| Public Java Application API | No affected type is in the allowlisted `com.lokiscale.loomspan.api` surface. Producer types are internal. | None added by this ticket area. |
| Supported Java SPI | No supported SPI or bean-replacement surface is involved. | None. |
| MCP tool names, input/output fields, errors, descriptions, annotations | Installed, documented serialized diagnostic contract for Console clients; advertised by capability and snapshotted in tests. | MCP hosts/agents, Console Agent Skill, README examples/tests. |
| Agent Skill YAML/Markdown | Configuration/manifest and packaged guidance contract. | Agent-Skill-capable MCP clients and packaging tests. |
| Raw NDJSON trace records and same-version portable artifact | Ephemeral diagnostic format with a narrow same-`consoleCompatibilityVersion` transfer contract. | Java writer, Go parser/indexer, Java fixture corpus, import/export path. |
| Go analysis manifest, indexes, handles, continuations, content-reference encoding | Internal current-process implementation. Manifest source explicitly says it is not durable state. | Console services within the same process. |
| README behavior | Consumer-facing documentation evidence for the MCP and same-version diagnostic contracts. | Developers configuring and using Console. |

The Java-to-Go boundary is protected by the Java writer's record vocabulary/metadata, the Go closed enum and validator, the same compatibility marker, and the Java-produced semantic fixture corpus. `TraceRecordType` is mirrored in Go (`enums.go:3-59`); `consoleCompatibilityVersion` is written in Java and checked during Go preflight/full processing (`DefaultExecutionTraceHandle.java:306`, `processor.go:50-78`, `processor.go:139-158`). The internal Go manifest explicitly remains a same-process format (`manifest.go:14-17`).

No Java production type needs to change to express the ticket's current Go/MCP concerns because attempt number/reason and frame close status already cross the trace boundary. If Java record vocabulary or producer semantics were changed later, the Go enum/validator, compatibility decision, Java fixture corpus, Go fixture consumer, MCP schemas, README, and Agent Skill are the in-repository coordinated consumers.

## Architecture Documentation

The current finalized-trace path is:

```text
Java internal trace writer
  -> same-version NDJSON artifact
  -> Go Processor validates every physical record
  -> immutable manifest + record/frame/attempt/retry/failure/payload indexes
  -> trace inventory/resolution selects one installed evidence owner
  -> transport-neutral traceanalysis query service
  -> MCP DTO mapping + compact authored output schema + text fallback
  -> read-only MCP client
```

Trace processing is a one-pass validation/index construction followed by immutable indexed reads. Adapters map authoritative analysis values and do not recalculate them. Inventory is separate from analysis: it merges installed and target catalog evidence, exposes completeness/ambiguity, and resolves by `traceId` before analysis obtains an artifact lease. Continuations and content references bind transient progress/selection to the current process, evidence owner, handle, operation, and query state while presenting only opaque strings externally.

Structured output has two schema layers: complete generated DTO validation before publication and compact authored schemas in `tools/list`. This preserves runtime validation while lowering discovery size (`output_schemas.go:14-48`). Text fallbacks are created independently from the same mapped DTO result.

## Tests and Fixtures

Relevant current executable coverage includes:

- Attempt graph identity, lifecycle, retry-sequence grouping, and usage aggregation in `internal/traceanalysis/calculations_test.go`.
- Query defaults, compact/detailed fields, continuations, and exact range behavior in `internal/traceanalysis/service_test.go`, `continuation_test.go`, and `range_test.go`.
- Inline reference security and stale-reference handling in `content_ref_test.go` and MCP adapter tests.
- Inventory source merging, independent filters/orderings, completeness, ambiguity, and continuation stability in `internal/traceinventory/service_test.go`.
- Closed authored schemas and exact developer-intent properties in `internal/mcpadapter/trace_contracts_test.go` and `output_schemas_test.go`.
- Full HTTP `tools/list` byte size and snapshot in `internal/mcpadapter/server_test.go` plus `testdata/tools-list-response.json`.
- Representative structured DTO budgets, fallback bounds, compact omission, binary inline transport, and range fallback completeness in `internal/mcpadapter/traces_test.go`.
- Java-to-Go semantic fixture parity in `internal/mcpadapter/trace_semantic_fixtures_test.go` and `internal/traceanalysis/fixture_corpus_test.go`, backed by the Java `ConsoleTraceFixtureCorpusTest`.

Verification run during this research:

```text
go test ./internal/traceanalysis ./internal/traceinventory ./internal/mcpadapter
```

All three packages passed at commit `5c49ec9a0e9b4909e6d5bd90f7da9273432221e2`.

## Historical Context (from ai/thoughts/)

The trace-understanding workflow catalog established semantic content retrieval as a general opaque `contentRef` contract and specified descriptors with role, type, logical byte length, inline eligibility/content, incompleteness, and deterministic per-value/aggregate omission (`ai/thoughts/phases/loomspan_llm_trace_understanding_workflows.md:730-765`). It also defined a representative five-call plan-evolution path ending with `query_trace_records(... inlineContent=true)` when content fits the aggregate budget, while treating additional bounded reads as successful behavior rather than hidden cost (`loomspan_llm_trace_understanding_workflows.md:912-942`).

The 2026-08-15 roadmap recorded the same content abstraction, same per-value/aggregate bounds, optional raw-forensics capability, and explicit content encoding/length/completeness requirements (`ai/thoughts/phases/2026-08-15-loomspan-llm-trace-understanding-roadmap.md:474-514`). Its tool-efficiency workstream called for measuring total `tools/list` bytes, repeated schema definitions, concise identifier/content descriptions, compact projections, and client behavior after failures (`2026-08-15-loomspan-llm-trace-understanding-roadmap.md:563-576`).

The design lens establishes that trace schemas and projection behavior are ephemeral diagnostic formats whose diagnostic accuracy, ordering, failure visibility, security, and current writer/reader/projector coherence remain protected. It also states that a portable artifact is accepted only with the exact same `consoleCompatibilityVersion`, while derived indexes, handles, continuations, and imported catalogs remain ephemeral current-process formats (`ai/thoughts/framework-feature-design-lens.md:19-55`).

## Related Research

No prior file exists under `ai/thoughts/research/` in this working tree. Related design material is:

- `ai/thoughts/phases/loomspan_llm_trace_understanding_workflows.md`
- `ai/thoughts/phases/2026-08-15-loomspan-llm-trace-understanding-roadmap.md`
- `ai/thoughts/phases/loomspan_console_workflows.md`
- `ai/thoughts/framework-feature-design-lens.md`

## Open Questions

- The repository does not record the client/host overflow threshold observed by the two external walkthroughs, so the host boundary cannot be derived from current tests.
- The ticket's full serialized worst-case response matrix has not been checked in; current size tests cover full `tools/list`, representative DTO bodies, fallback-only bounds, and exact range transport separately.
- No current checked-in evaluation records a tools-only versus Agent-Skill-assisted walkthrough on this commit.
- Closed frame validation establishes at most one close record, but it does not require nonblank close `status` in `onFrameClosed`; accepted artifacts can therefore expose zero outcomes for an unclosed frame and can also leave the outcome set empty when a close record's metadata has no status.
- The current branch contains the PR 31 ticket as an untracked working-tree file; it was treated as the supplied research prompt and was not modified.
