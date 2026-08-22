---
date: 2026-08-21T13:25:33-07:00
researcher: Codex (GPT-5)
git_commit: fb1f02f10a274f86d3fa37f2ad1d9f70937ac91e
branch: main
repository: loomspan
topic: "PR 34 — Active-Execution MCP Inspection Ergonomics and Evidence Semantics"
tags: [research, codebase, loomspan-console, mcp, active-executions, live-activity, usage, agent-skill]
status: complete
last_updated: 2026-08-21
last_updated_by: Codex (GPT-5)
---

# Research: PR 34 — Active-Execution MCP Inspection Ergonomics and Evidence Semantics

**Date**: 2026-08-21 13:25:33 PDT
**Researcher**: Codex (GPT-5)
**Git Commit**: fb1f02f10a274f86d3fa37f2ad1d9f70937ac91e
**Branch**: main
**Repository**: loomspan

## Research Question

Research the five problem statements and ten required questions in
`ai/thoughts/tickets/loomspan-console-pr-34-active-execution-mcp-inspection.md`
against the current production code, tests, fixtures, documentation, roadmap,
canonical portable Agent Skill, and Java-to-Go observability boundary. Document
the existing active-execution discovery, activity continuation and coverage,
usage accrual, completion-race, client, packaging, and evaluation semantics.

## Summary

The existing MCP result DTOs already contain the facts needed to orient a full
bounded active-execution page. A structured-output client receives session and
trace identity, sequence and time facts, elapsed time, entry skill, status,
phase, summary, full bounded active path, usage, and configured limits from
`LOOMSPAN_list_executions`; `LOOMSPAN_get_execution` uses the same execution
DTO and contributes only a later individual observation. The advertised compact
schemas expose substantially fewer named fields, and the list text fallback is
also intentionally concise. Consequently, structured-output clients can use the
list plus one bounded activity call per execution without a detail call for DTO
completeness, but schema-driven clients cannot discover many of those fields
and text-only clients need a detail call for sequence, elapsed time, path,
usage, and limits.

The exact current `tools/list` HTTP JSON-RPC response is 23,495 bytes against
the executable 23,552-byte ceiling, leaving 57 bytes. The checked-in client
compatibility document still records 23,390 bytes, while the exact snapshot and
test now establish 23,495. Output schemas account for 12,172 bytes of the
current response, input schemas 6,413 bytes, and description members 2,774
bytes. Isolated additions show that only small single-property additions fit
without an offset elsewhere; nested path, coverage, and limit shapes do not fit
in the remaining headroom as direct additions.

Activity uses a shared bounded global ring. `hasMore` reports another matching
item retained now. The MCP continuation has a second operational role: after a
page with `hasMore: false`, it remains a forward checkpoint and can later be
reused to retrieve newly admitted matching activity. When an activity call
returns no matching items, MCP advances that checkpoint to the shared
continuity interval's last cursor. This behavior follows from production code,
but no focused MCP test currently reuses a continuation after `hasMore: false`.

`beginningUnavailable` is a global retained-window fact. It is true when the
selected start is not the ring beginning, any global eviction occurred, or the
current interval has a reset. Session filtering does not maintain a per-session
admission boundary or completeness fact and does not interpret
`TRACE_STARTED`. The current live state therefore cannot authoritatively state
that one selected execution is complete from its start. It can report the
global interval, returned cursor range, resets, and the presence of retained
session events, including a retained start event.

The historical provider-attempt observation is explained by a deterministic
cross-component omission. Java session usage and the live projector count every
physical provider request before the request activity is published, but the
Java observability REST `Usage` and `QuotaLimits` records omit
`providerAttempts` and `maxProviderAttempts`. Go defines those fields as plain
integers, decodes the absent JSON members as zero, and republishes zero through
MCP. This is not projector timing. During a real in-flight provider request,
Java live state has at least one provider attempt while model-call, response,
token, and response-precision counters can correctly remain zero.

The canonical skill is version 1.0.1 and is validated as an exact six-file
package. Its active workflow is the single-execution `WF-SLOW-EXECUTION` route;
it has no explicit full-page multi-execution route, forward-checkpoint polling
procedure, or completion-between-calls handoff. The 36 evaluation cases include
one live `slow-execution` case backed by one active execution. The evaluation
server special-cases only that live case, and the fixed 28-run release matrix
uses the same single-execution workflow. The multi-execution, globally evicted
but session-start-retained, genuinely incomplete session, second-observation,
and completion-race cases described by PR 34 are not present in the current
corpus.

## Detailed Findings

### 1. Complete active-execution and activity fields versus compact discovery

The complete active execution is one shared `executionDTO`. List and detail
results both embed that same type, and `mapExecution` supplies every field for
both handlers (`loomspan-console/internal/mcpadapter/contracts.go:65-99`,
`loomspan-console/internal/mcpadapter/contracts.go:208-222`).

| Result shape | Named in compact discovery | Present in complete structured content but not named in compact discovery |
| --- | --- | --- |
| Execution list item | `sessionId`, `traceId`, `entrySkill`, `status`, `phase` | `lastCanonicalSequence`, `startedAt`, `updatedAt`, `elapsedMillis`, `summary`, `activePath`, `totalFrameDepth`, `activePathTruncated`, `usage`, `configuredLimits` |
| Execution detail | `sessionId`, `traceId`, `status`, `phase`, open `activePath`, open `usage`, open `configuredLimits` | `lastCanonicalSequence`, `startedAt`, `updatedAt`, `elapsedMillis`, `entrySkill`, `summary`, `totalFrameDepth`, `activePathTruncated`; child field names inside the three open shapes are also unnamed |
| Activity item | `cursor`, `sessionId`, `traceId`, `timestamp`, `kind`, `summary`, `details` | `canonicalSequence`, `executionStatus`, `frameId`, `parentFrameId`, `frameType`, `route` |
| Activity result | `observedAt`, `items`, `hasMore`, `continuation`, `beginningUnavailable` | `returnedCursorRange` and `continuity`, including their cursor, interval, observation, and reset children |

The compact declarations are in
`loomspan-console/internal/mcpadapter/output_schemas.go:165-185`. They use open
objects at the result/item layers, so a complete result may legally carry the
unadvertised members (`loomspan-console/internal/mcpadapter/output_schemas.go:105-110`,
`loomspan-console/internal/mcpadapter/output_schemas.go:129-145`). Independently,
the server derives a full schema from every typed result and validates complete
output before publication (`loomspan-console/internal/mcpadapter/output_schemas.go:17-60`).
An unadvertised field is therefore undiscoverable from the compact schema, not
absent from a successful structured result.

The complete active path has `frameId`, `frameType`, and `route`. Complete usage
has `skillInvocations`, `toolInvocations`, `linterRetries`, `modelCalls`,
`providerAttempts`, `promptUnits`, `completionUnits`, `usageUnits`,
`exactModelResponses`, `heuristicModelResponses`, and
`unavailableModelResponses`. Complete configured limits have the corresponding
six `max*` values, including `maxProviderAttempts`
(`loomspan-console/internal/mcpadapter/executions.go:158-178`). None of these
child names is discoverable on the list schema; detail advertises their parent
containers only as open shapes.

For ordinary orientation as defined by the ticket, identity, entry skill,
status, and phase are already discoverable. Elapsed time, latest canonical
sequence, active path/route and truncation state, usage and limits, activity
canonical sequence/frame/route identity, returned cursor range, and continuity
coverage exist only in complete results. Summary is named only for activity,
not for the execution list/detail compact shapes.

### 2. Exact `tools/list` budget and measured contributions

`maxToolsListResponseBytes` is `23 << 10`, or 23,552 bytes
(`loomspan-console/internal/mcpadapter/output_schemas.go:13`). The exact compact
UTF-8 serialization of the current committed full HTTP JSON-RPC snapshot is
23,495 bytes. `server_test.go` protects both the exact snapshot and this size
(`loomspan-console/internal/mcpadapter/server_test.go:33-34`,
`loomspan-console/internal/mcpadapter/server_test.go:89-108`). Current headroom
is 57 bytes.

The current response decomposes into a 78-byte envelope with an empty tools
array, 23,406 bytes of serialized tool objects, and 11 comma bytes. Across the
tool objects, description members occupy 2,774 bytes, input-schema values 6,413
bytes, and output-schema values 12,172 bytes.

| Tool | Complete tool object | Description member | Input schema | Output schema |
| --- | ---: | ---: | ---: | ---: |
| `LOOMSPAN_get_execution` | 1,271 | 111 | 208 | 783 |
| `LOOMSPAN_get_execution_activity` | 1,566 | 121 | 470 | 797 |
| `LOOMSPAN_get_runtime` | 914 | 104 | 46 | 597 |
| `LOOMSPAN_get_skill` | 998 | 119 | 204 | 510 |
| `LOOMSPAN_get_trace` | 3,510 | 351 | 163 | 2,831 |
| `LOOMSPAN_list_executions` | 1,312 | 134 | 334 | 673 |
| `LOOMSPAN_list_skills` | 1,205 | 133 | 330 | 575 |
| `LOOMSPAN_list_traces` | 2,142 | 396 | 857 | 722 |
| `LOOMSPAN_query_trace_frames` | 2,844 | 525 | 1,179 | 966 |
| `LOOMSPAN_query_trace_records` | 4,481 | 445 | 1,745 | 2,116 |
| `LOOMSPAN_read_trace_artifact` | 1,545 | 168 | 401 | 801 |
| `LOOMSPAN_read_trace_content` | 1,618 | 167 | 476 | 801 |

Fresh isolated mutations of the exact snapshot establish the byte effect of
individual compact-schema names without changing current behavior:

| Isolated optional property | Byte delta | Fits current 57-byte headroom |
| --- | ---: | --- |
| List `lastCanonicalSequence` | +43 | Yes |
| List `startedAt` or `updatedAt` | +30 each | Yes individually |
| List `elapsedMillis` | +35 | Yes |
| List `summary` | +28 | Yes |
| List open `activePath` | +84 | No |
| List `totalFrameDepth` | +37 | Yes |
| List `activePathTruncated` | +41 | Yes |
| List open `usage` | +54 | Yes |
| List open `configuredLimits` | +65 | No |
| Activity `canonicalSequence` | +39 | Yes |
| Activity `executionStatus` | +36 | Yes |
| Activity `frameId` | +28 | Yes |
| Activity `parentFrameId` | +34 | Yes |
| Activity `frameType` | +30 | Yes |
| Activity `route` | +26 | Yes |
| Activity open `returnedCursorRange` | +68 | No |
| Activity open `continuity` | +59 | No |

Required-property entries add more bytes. For example, required list
`lastCanonicalSequence` is +67, and even two optional list properties
`summary` plus `elapsedMillis` are +63. The 57-byte headroom permits 57
additional ASCII description bytes in total if no other serialized member
changes. The repository history records a 37,788-byte full-schema response
before compaction and a 20,304-byte compact response at PR 30; the test also
retains an earlier 34,371-byte baseline
(`loomspan-console/internal/mcpadapter/server_test.go:32-35`).

`loomspan-console/docs/mcp-client-compatibility.md:18-24` currently records
23,390 bytes against the same ceiling. The executable snapshot and fresh test
at this checkout establish 23,495 bytes, 105 bytes above that documented
measurement.

### 3. Structured-output and text-fallback client experience

Every successful tool call publishes deterministic text alongside the typed
result envelope (`loomspan-console/internal/mcpadapter/contracts.go:148-150`).
The SDK exposes the typed envelope as structured content, which is asserted by
the black-box server test (`loomspan-console/internal/mcpadapter/server_test.go:220`).

Structured-output clients receive the complete list DTO, so a list result
already has the same execution fact shape as detail. The committed list and
detail goldens demonstrate the complete shapes in
`loomspan-console/internal/mcpadapter/testdata/executions-list.json` and
`loomspan-console/internal/mcpadapter/testdata/execution-detail.json`.

The list text reports top-level observation, count, pagination, and per-item
session ID, trace ID, entry skill, status, phase, and summary. It omits sequence,
time/elapsed, active path, usage, and limits
(`loomspan-console/internal/mcpadapter/executions.go:112-127`). The detail text
renders the full active snapshot, including every path, usage, and limit field
(`loomspan-console/internal/mcpadapter/executions.go:130-179`). A text-only
client therefore needs one detail call per selected execution to recover those
ordinary orientation facts.

Activity structured content preserves all item identities and the bounded,
arbitrary `details` object (`loomspan-console/internal/mcpadapter/contracts.go:225-244`).
Activity text reports result-level cursor range, continuity/reset,
`beginningUnavailable`, backlog state, and continuation, while an item line is
limited to cursor, timestamp, kind, and summary
(`loomspan-console/internal/mcpadapter/activity.go:96-131`). It omits item
session/trace identity, canonical sequence, execution status, frame identity,
route, and arbitrary details. Tests protect both complete 64-item structured
content and the absence of detail leakage in text
(`loomspan-console/internal/mcpadapter/activity_test.go:22-66`,
`loomspan-console/internal/mcpadapter/activity_test.go:223-244`).

### 4. Execution-list pagination semantics

The MCP list defaults omitted `pageSize` to 16, accepts 1 through 64, unwraps
its own continuation to the application cursor, and emits another continuation
only when the application says `hasMore` and supplies `nextCursor`
(`loomspan-console/internal/mcpadapter/executions.go:16-30`,
`loomspan-console/internal/mcpadapter/executions.go:42-85`). The application
page's separate `resumeCursor` is decoded by Go but not exposed or used by the
MCP execution list (`loomspan-console/internal/observability/dto.go:95-110`,
`loomspan-console/internal/mcpadapter/executions.go:61-78`).

Application pagination is newest-first keyset pagination. The first page
captures the active registry's highest ordinal as a high-water mark. A
continuation binds application instance, collection, high-water mark, and an
exclusive `beforeOrdinal`; subsequent pages include only ordinals at or below
that high-water mark and below the prior boundary
(`loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/observability/web/ObservabilityRestController.java:103-134`,
`loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/observability/web/ObservabilityCursorCodec.java:29-101`,
`loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/runtime/observation/InMemoryActiveExecutionRegistry.java:58-81`).
Executions started after page one are excluded. An existing session keeps one
ordinal while its values update, and a session may complete and disappear, so
multi-page traversal has stable keyset membership boundaries but is not an
atomic snapshot of values or current membership
(`loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/runtime/observation/InMemoryActiveExecutionRegistry.java:29-49`).

Tests protect stable ordinals, newest-first order, exclusion of later starts,
high-water traversal, cross-instance `STALE_CURSOR`, impossible
`INVALID_CURSOR`, the MCP default page size, empty final pages, and 64 complete
items (`loomspan-spring-boot-starter/src/test/java/com/lokiscale/loomspan/internal/runtime/observation/InMemoryActiveExecutionRegistryTest.java:16-86`,
`loomspan-spring-boot-starter/src/test/java/com/lokiscale/loomspan/internal/observability/web/ObservabilityRestIntegrationTest.java:246-291`,
`loomspan-console/internal/mcpadapter/executions_test.go:43-54`,
`loomspan-console/internal/mcpadapter/executions_test.go:84-130`).

### 5. Activity backlog pagination, forward resumption, empty results, and stale state

Console maintains one shared ring of at most 2,048 envelopes or 8 MiB.
Admission requires strictly increasing positive global cursors; exact duplicate
envelopes are ignored, conflicting cursor reuse and regression are rejected,
and global eviction drops the oldest items and records an eviction fact
(`loomspan-console/internal/live/service.go:20-30`,
`loomspan-console/internal/live/service.go:605-655`). A reset clears the ring,
changes the interval ID, and records the reset cause for scope rotation,
instance change, or an upstream stale cursor
(`loomspan-console/internal/live/service.go:138-173`,
`loomspan-console/internal/live/service.go:243-260`,
`loomspan-console/internal/live/service.go:309-331`).

Without a cursor, `Recent` returns the newest suffix: the last `limit` global
items or the last `limit` items matching a session. It does not expose older
retained matching items as backlog, and the result has `hasMore: false`
(`loomspan-console/internal/live/service.go:682-685`,
`loomspan-console/internal/live/service.go:704-744`). With a found cursor, the
scan begins strictly after it, skips unrelated sessions, returns the first
`limit` matches, and sets `hasMore` only after encountering another currently
retained matching item (`loomspan-console/internal/live/service.go:686-729`).
This is the current retained-backlog meaning documented in
`loomspan-console/README.md:266-277`.

MCP does not publish the live service's `NextCursor`. It creates its opaque
continuation from the last returned matching item, or, when the result contains
no matching items, from the current continuity interval's last global cursor
(`loomspan-console/internal/mcpadapter/activity.go:66-86`). Therefore:

- `hasMore: true` means another matching item is retained now and the token
  pages through that backlog.
- `hasMore: false` does not invalidate the token. Reusing it later reads newly
  admitted matching activity after the checkpoint.
- An empty result can still return a nonempty checkpoint. Unrelated activity
  can advance that checkpoint to the shared interval's latest cursor.
- An initial empty ring has a non-null empty `items` collection, no backlog,
  continuity facts, and no checkpoint unless the interval has a last cursor.
- A cursor absent from the current ring returns an empty result with
  `beginningUnavailable: true`; it is not exposed as a stale-continuation
  domain error by the live service
  (`loomspan-console/internal/live/service.go:658-699`).

There is no focused MCP test that reuses a continuation after a successful
`hasMore: false` call. Existing tests cover suffix selection, filtering,
empty non-null items, reset/old-cursor limitations, and complete 64-item MCP
delivery (`loomspan-console/internal/live/coordinator_test.go:84-104`,
`loomspan-console/internal/live/coordinator_test.go:187-233`,
`loomspan-console/internal/live/coordinator_test.go:253-279`,
`loomspan-console/internal/mcpadapter/activity_test.go:153-244`).

MCP continuations are strict base64url JSON v1 tokens capped at 8,192 bytes and
bound to the operation kind and target scope; activity additionally binds the
exact session ID. Malformed, wrong-kind, or wrong-session tokens are
`INVALID_ARGUMENT`; tokens from a prior target scope are `TARGET_CHANGED`
(`loomspan-console/internal/mcpadapter/continuation.go:14-112`). Each execution
operation captures scope before work and rechecks it before publication. An
application cursor is separately bound to the application instance, where a
different instance is `STALE_CURSOR`. Upstream activity stale-cursor handling
is internal: the coordinator resets, rebaselines, and reconnects instead of
returning that application cursor condition through MCP.

### 6. Session-specific beginning coverage

Session filtering traverses the shared global ring and retains no per-session
ring, admission boundary, start cursor, or completeness state
(`loomspan-console/internal/live/service.go:68-90`,
`loomspan-console/internal/live/service.go:658-752`).
`beginningUnavailable` is calculated as selected start offset greater than
zero, any global eviction, or an interval reset. It is not conditioned on the
selected session and does not inspect `TRACE_STARTED`
(`loomspan-console/internal/live/service.go:745-750`). A missing supplied cursor
sets it unconditionally.

The active baseline begins from the application's current active page and
resume cursor. An execution can already be active when that baseline is
established, so its earlier events may precede the Console interval
(`loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/observability/web/ObservabilityRestController.java:126-132`,
`loomspan-console/internal/live/service.go:409-439`). A retained
`TRACE_STARTED` is direct evidence that the start item is retained, but current
state has no authoritative fact establishing that no reset, admission gap, or
earlier session event was missed. The available coverage vocabulary is only:

- global continuity interval ID, first/last cursor, observation time, and reset;
- returned first/last cursor;
- the global `beginningUnavailable` Boolean; and
- the actual retained filtered items.

The README describes this as a limitation of the returned interval
(`loomspan-console/README.md:350-357`) and the recent endpoint as a suffix of a
shared ring (`loomspan-console/README.md:424-447`). There is no current focused
test for a newly started session whose retained `TRACE_STARTED` follows
unrelated global eviction.

### 7. Active usage producer, projector, REST, Go, and MCP chain

Java's immutable `SessionUsageSnapshot` contains `providerAttempts`
(`loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/runtime/usage/SessionUsageSnapshot.java:8-19`).
The live projector increments that counter for every canonical
`MODEL_REQUEST_SENT` (`loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/runtime/observation/LiveActivityProjector.java:76-112`).
The provider advisor reserves one attempt immediately before each physical
provider send, records the request, and on failure records the failed attempt
before optionally retrying. On success it records the response before applying
session response usage (`loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/chat/ProviderAttemptCallAdvisor.java:58-96`).

Every appended canonical logical record is synchronously published to the
observation layer. The projector updates state, creates the activity, then
creates the snapshot; the observation handle replaces the active registry
snapshot before appending the corresponding replay activity
(`loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/runtime/trace/DefaultExecutionTraceHandle.java:444-520`,
`loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/runtime/observation/LiveActivityProjector.java:48-73`,
`loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/runtime/observation/DefaultExecutionObservationHandle.java:51-94`).
The request snapshot therefore contains its incremented attempt before the
corresponding request activity becomes visible.

The loss occurs at the REST DTO boundary. `ObservabilityDtos.Usage` omits
`providerAttempts`, and `QuotaLimits` omits `maxProviderAttempts`
(`loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/observability/web/dto/ObservabilityDtos.java:33-49`).
`ObservabilityDtoMapper.active` consequently cannot map the two existing source
values (`loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/observability/web/ObservabilityDtoMapper.java:38-52`).
Go declares both as non-null integer fields
(`loomspan-console/internal/observability/dto.go:38-58`); absent JSON members
become zero, active DTO validation does not inspect usage/limits
(`loomspan-console/internal/observability/service.go:297-310`), and MCP copies
and renders the zero values (`loomspan-console/internal/mcpadapter/contracts.go:208-222`,
`loomspan-console/internal/mcpadapter/executions.go:158-178`).

The committed Java-generated active REST fixtures omit the two provider fields,
while the derived MCP goldens contain zero
(`loomspan-console-fixtures/application-rest/active-execution-detail.json:1`,
`loomspan-console-fixtures/application-rest/active-executions-page.json:1`,
`loomspan-console/internal/mcpadapter/testdata/execution-detail.json:1`,
`loomspan-console/internal/mcpadapter/testdata/executions-list.json:1`). The
fixture constructor itself uses a 10-member usage record and five-member limit
record (`loomspan-spring-boot-starter/src/test/java/com/lokiscale/loomspan/internal/observability/web/ConsoleRestFixtureCorpusTest.java:64-87`).

Provider integration tests prove the upstream executable semantics: one
transient failure and retry produces two provider calls, two provider attempts,
one model call, two request records, one failure, and one response; exhaustion
produces three attempts, zero model calls, and three request/failure records
(`loomspan-spring-boot-starter/src/test/java/com/lokiscale/loomspan/internal/chat/ModelAttemptCallAdvisorIntegrationTest.java:252-299`,
`loomspan-spring-boot-starter/src/test/java/com/lokiscale/loomspan/internal/chat/ModelAttemptCallAdvisorIntegrationTest.java:397-419`).
Current projector unit tests separately cover a response without a request and
a failure without a request, but no current deterministic integration test
spans advisor through observation registry, REST DTO, and Go
(`loomspan-spring-boot-starter/src/test/java/com/lokiscale/loomspan/internal/runtime/observation/LiveActivityProjectorTest.java:228-251`,
`loomspan-spring-boot-starter/src/test/java/com/lokiscale/loomspan/internal/runtime/observation/LiveActivityProjectorTest.java:304-323`).

### 8. Exact active usage accrual and zero/limit semantics

| Counter/fact | Current accrual point |
| --- | --- |
| `skillInvocations` | Session usage increments when a mission/skill begins; live projection increments on `FRAME_OPENED` for root-mission or skill-execution frames (`DefaultSessionUsageService.java:33-40`; `LiveActivityProjector.java:79-91`). |
| `toolInvocations` | Session usage increments before tool execution; live projection increments on canonical `TOOL_CALL_STARTED`. Tool completion/failure does not add usage (`DefaultSessionUsageService.java:83-90`; `LiveActivityProjector.java:97-100`). |
| `linterRetries` | Session usage increments for a linter outcome of `RETRYING`; live projection increments on `PLAN_RETRY_REQUESTED` (`DefaultSessionUsageService.java:93-110`; `LiveActivityProjector.java:101-104`). |
| `providerAttempts` | Reserved immediately before every physical send and projected on every `MODEL_REQUEST_SENT`; quota rejection does not increment it (`DefaultSessionUsageService.java:53-74`; `ProviderAttemptCallAdvisor.java:58-64`). |
| `modelCalls` and response precision | Increment only on `MODEL_RESPONSE_RECEIVED`. Provider attempt failures do not increment them (`LiveActivityProjector.java:105-108`). |
| Prompt/completion/usage units | Accrue with a model response. Missing usable/estimable usage records one `UNAVAILABLE` response and zero units (`LiveActivityProjector.java:387-400`; `SessionUsageSnapshot.java:89-103`). |
| `elapsedMillis` | Calculated on each REST read as `max(0, observedAt - startedAt)`, saturating at `Long.MAX_VALUE`; it is not a stored accumulating usage counter (`ObservabilityDtoMapper.java:26-37`). |
| `updatedAt` | Timestamp of the latest canonical record (`LiveActivityProjector.java:206-220`). |

Quota configuration accepts nonnegative integers. Defaults are 64 skill, 128
tool, 32 linter retry, 64 model call, 192 provider attempt, and 200,000 usage
units (`loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/autoconfigure/LoomspanProperties.java:323-342`).
Enforcement treats zero or a negative internal value as disabled; accepted
configuration is nonnegative, making configured zero the represented sentinel
for disabled/unlimited enforcement
(`loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/runtime/usage/DefaultSessionUsageService.java:57-67`,
`loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/runtime/usage/DefaultSessionUsageService.java:142-147`).
The provider property metadata documents zero as disabling that limit
(`loomspan-spring-boot-starter/src/main/resources/META-INF/additional-spring-configuration-metadata.json:104-107`).

REST/MCP usage and limit values are plain integers without availability
discriminators. A genuine all-zero live snapshot is expected before the
relevant canonical facts. While a provider request is in flight, model-call,
token, and response-precision counters can correctly be zero, but Java's
provider-attempt count is already positive. At the current REST boundary,
`providerAttempts: 0` and `maxProviderAttempts: 0` cannot distinguish omitted
data from observed zero or a genuinely disabled provider-attempt limit.

SSE activity carries no usage. Go list/detail reads fetch REST directly. The
live coordinator uses REST pages as a baseline and SSE to remove completed
sessions, but does not update usage counters from SSE
(`loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/observability/web/dto/ObservabilityDtos.java:87-107`,
`loomspan-console/internal/observability/service.go:98-148`,
`loomspan-console/internal/live/service.go:506-546`). Active REST limits are
mapped from current runtime quota configuration at read time, whereas finalized
trace configured limits are an immutable run-start snapshot
(`loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/observability/web/ObservabilityRestController.java:121-145`,
`loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/runtime/trace/ConfiguredLimitsSnapshot.java:10-53`).

### 9. Full-page review, detail calls, and completion races

For a structured-output client, the list plus one bounded activity call per
execution contains the complete defined orientation DTO and one current recent
progress/coverage page. List maps every item through the same `mapExecution`
used by detail (`loomspan-console/internal/mcpadapter/executions.go:61-78`,
`loomspan-console/internal/mcpadapter/executions.go:96-102`). A detail call is a
later freshness/existence observation, not a richer projection. One activity
call is bounded to 64 complete items. If it returns `hasMore: true`, the call is
not the complete retained matching backlog; that is an explicit coverage fact.
Text-only clients need detail because list text omits the orientation fields
described above.

Java holds terminal `TRACE_COMPLETED` activity until close. For a successful
close, the trace catalog is published first when retained; then terminal
activity is enriched with application availability and published; finally the
active registry entry is removed. A core-finalization failure publishes
`EXECUTION_OBSERVATION_ENDED` with unavailable trace evidence before removal
(`loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/runtime/observation/LiveActivityProjector.java:69-74`,
`loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/runtime/observation/DefaultExecutionObservationHandle.java:97-214`).
Accordingly, a session returned by list can be absent from a later detail call.

Activity lookup filters the retained ring and does not require current active
registry membership, so it can still return retained terminal activity after
the registry entry disappears (`loomspan-console/internal/mcpadapter/activity.go:31-49`,
`loomspan-console/internal/live/service.go:704-729`). The already returned
`traceId` is the exact identity accepted by finalized-trace resolution, which
may transparently acquire target evidence
(`loomspan-console/internal/mcpadapter/traces.go:177-198`,
`loomspan-console/internal/traceresolution/service.go:46-131`). If unique safe
evidence is unavailable, the domain result is `TRACE_UNAVAILABLE`; absence from
active state is not proof that finalized evidence exists
(`loomspan-console/internal/traceresolution/service.go:138-146`).

The product currently documents trace inspection by `traceId`, provisional
active snapshots, and `TRACE_UNAVAILABLE` recovery
(`loomspan-console/README.md:305-309`, `loomspan-console/README.md:350-357`).
The browser exposes trace inspection only when terminal activity reports
availability (`loomspan-console/README.md:455-461`). The portable skill likewise
uses `traceId` for finalized evidence and preserves `TARGET_CHANGED`, but its
slow workflow contains no explicit bounded completion-between-list/detail/activity
handoff (`loomspan-console/agent-skills/loomspan/SKILL.md:76-81`,
`loomspan-console/agent-skills/loomspan/references/debugging-playbooks.md:13-19`,
`loomspan-console/agent-skills/loomspan/references/mcp-tool-guide.md:98-106`).

### 10. Canonical skill, validation, evaluations, docs, and fixtures

The authoritative skill is
`loomspan-console/agent-skills/loomspan/`. Its metadata version is 1.0.1
(`loomspan-console/agent-skills/loomspan/SKILL.md:1-7`). Runtime validation
requires exactly `SKILL.md` and five named references, validates the exact
version, limits the instruction body to 5,000 estimated tokens and individual
files to 80 KiB, requires exactly one link to every reference, rejects extra or
linked package files, and rejects embedded endpoints, credentials/headers, and
generated trace data (`loomspan-console/internal/agentskills/validate.go:18-30`,
`loomspan-console/internal/agentskills/validate.go:52-90`,
`loomspan-console/internal/agentskills/validate.go:120-162`,
`loomspan-console/internal/agentskills/validate.go:221-246`). Package tests
protect the canonical package and unsafe/nonportable variants
(`loomspan-console/internal/agentskills/validate_test.go:11-41`). Release
packaging validates the source package and copies those exact files to
`skills/loomspan/` (`loomspan-console/internal/buildtool/package.go:70-95`).

Current active guidance is split across:

- bootstrap and evidence boundaries in `SKILL.md`;
- one selected-execution slow workflow in
  `references/debugging-playbooks.md:13-19`;
- global live evidence/continuation statements in
  `references/runtime-model.md:9-30`;
- general continuation guidance in
  `references/mcp-tool-guide.md:88-106`; and
- missing-is-unknown and untrusted-content rules in
  `references/evidence-and-confidence.md` and
  `references/common-failure-patterns.md`.

The skill does not currently contain a multi-execution route, explicit
continuation-as-future-checkpoint procedure, session-specific coverage
vocabulary, active usage accrual table, or completion-race handoff. It does
state that live conclusions are provisional, missing usage is unknown rather
than zero, a quiet window is not stuckness, continuations are opaque, and
returned content is untrusted evidence
(`loomspan-console/agent-skills/loomspan/SKILL.md:74-104`).

The evaluation directory currently has 36 case files. `slow-execution.json` is
the only active progress case and names one session, latest sequence,
`beginningUnavailable`, provisional evidence, and the limitation that activity
is not durable or lossless history
(`loomspan-console/agent-evals/cases/slow-execution.json:1`). The evaluation
server special-cases that case and serves the one-execution Java REST fixtures
plus SSE replay (`loomspan-console/internal/agenteval/server.go:154-155`,
`loomspan-console/internal/agenteval/server.go:199-282`). The current fixed
release matrix contains 28 Codex CLI and Claude Code runs and includes the same
single-execution slow case (`loomspan-console/internal/agenteval/score.go:51-97`).

Case validation currently accepts four named workflow IDs and requirement
prefixes through PR 31; it requires nonempty unique oracle collections and
existing repository-relative fixture sources
(`loomspan-console/internal/agenteval/fixtures.go:89-125`). The current
multi-execution and lifecycle scenarios are absent from the fixtures, live eval
server, required-case assertions, release matrix, and evaluation README.

The affected documentation and evidence inventory is:

| Area | Current authorities/consumers |
| --- | --- |
| MCP discovery/result contract | `internal/mcpadapter/output_schemas.go`, `contracts.go`, `executions.go`, `activity.go`; `output_schemas_test.go`, `server_test.go`, `executions_test.go`, `activity_test.go`, `parity_test.go`; `testdata/tools-list-response.json` and result goldens |
| Shared live semantics | `internal/live/service.go`, browser activity provider/presentation, coordinator/service tests, `README.md` live activity and MCP sections |
| Java-Go active REST/SSE protocol | `ObservabilityDtos.java`, `ObservabilityDtoMapper.java`, controller/projector/usage code; Java mapper/REST/fixture tests; `loomspan-console-fixtures/application-rest/*`; Go observability DTO/service tests |
| Portable skill | All six canonical files, `internal/agentskills` validation/tests, build pipeline and archive/smoke byte-identity tests, root/release README version text |
| Agent evaluation | `agent-evals/README.md`, cases and fixture sources, `internal/agenteval` loader/server/scorer/tests, committed result summaries when actual clients run |
| Compatibility evidence | `docs/mcp-client-compatibility.md`, exact tool-list snapshot and ceiling tests, MCP conformance and black-box protocol revision tests |
| Roadmap/history | Active trace-understanding roadmap and the PR 34 ticket brief |

The author-facing `ai/skill-authoring/` knowledge base routes runtime diagnosis
to the packaged Agent Skill and deliberately does not duplicate its playbooks
(`ai/skill-authoring/README.md:41-64`,
`ai/skill-authoring/traces-and-debugging.md:10-32`). It describes provider
attempt and usage semantics, but the requested work concerns Console/runtime
debugging guidance rather than Loomspan YAML authoring syntax or application
input/output contracts.

## Architecture Documentation

### Contract classification

The classifications below use the categories from
`ai/thoughts/framework-feature-design-lens.md`.

| Surface | Current classification and evidence | Protected in-repository consumers |
| --- | --- | --- |
| Top-level Java application types in `com.lokiscale.loomspan.api` | **Application API**. Closed allowlist is executable authority. This research found no active-inspection dependency or signature delta in that package. | Public-surface architecture test and README API summary |
| Java SPI | **Supported SPI** category has no current Loomspan surface affected here. No SPI or bean replacement contract participates. | Closed public-surface policy |
| `loomspan.session.quotas.*` properties and zero/default behavior | **Configuration and manifest contracts**. Property metadata and binding/defaults establish user-visible configuration semantics. `LoomspanProperties` Java signatures are autoconfiguration machinery, not Application API or SPI. | Configuration binding, metadata, usage enforcement, docs/tests |
| Portable Agent Skill metadata and six-file package | User-facing released debugging contract and package manifest behavior; independently versioned at 1.0.1. It is not a Java application API. | Skill validator, official `skills-ref`, packaging/smoke tests, client/evaluation guidance |
| Active snapshots, activity, continuations, trace projections | **Ephemeral diagnostic formats**. Current-process, bounded, target-bound, and exact-release evidence rather than durable history or cross-version interchange. | Browser, MCP, live coordinator, tests and fixtures |
| Java observability REST/SSE consumed by Console | **Internal or accidentally exposed implementation** as Java signatures, plus an exact-release cross-component serialized protocol. It is version-locked rather than a general public API. | Go observability client, Java/Go fixtures, REST/SSE tests, compatibility-version checks |
| MCP names, inputs, compact schemas, structured results, text, continuation behavior, and domain errors | Deliberately supported pre-v1 Console diagnostic contract built over ephemeral current-process evidence. It changes in place before v1, with server, tests, fixtures, docs, skill, evaluations, and browser consumers kept atomic. | MCP SDK/HTTP clients, canonical skill, snapshot/golden/conformance tests, client compatibility evidence |
| Projector, registries, usage service, observation handles, Go live/adapter implementation types | **Internal or accidentally exposed implementation**. Public Java modifiers do not establish supported API. | Internal construction sites and focused tests |

`LoomspanPublicSurfaceArchitectureTest` explicitly classifies technically public
internal observation/usage/REST types—including `ActiveExecutionSnapshot`,
`LiveActivityProjector`, `DefaultSessionUsageService`, `SessionUsageService`,
`SessionUsageSnapshot`, `ObservabilityDtoMapper`, and `ObservabilityDtos`—as
internal rather than Application API
(`loomspan-spring-boot-starter/src/test/java/com/lokiscale/loomspan/LoomspanPublicSurfaceArchitectureTest.java:48-67`,
`loomspan-spring-boot-starter/src/test/java/com/lokiscale/loomspan/LoomspanPublicSurfaceArchitectureTest.java:208-242`).
No unexpected supported Java API or SPI surface was found.

### Data and control flow

```text
session usage reservation / canonical trace record
                    |
                    v
          LiveActivityProjector
     updates snapshot before activity replay
                    |
        +-----------+-----------+
        |                       |
        v                       v
active registry / REST      SSE activity ring
        |                       |
        v                       v
Go observability DTO        Go live.Service
        |                       |
        +-----------+-----------+
                    v
       MCP execution and activity adapters
          structured result + text fallback
```

The REST branch supplies active sequence/path/usage/limit snapshots. The SSE
branch supplies bounded progress and continuity, not usage. The provider-attempt
loss occurs in the Java REST DTO mapping before either Go or MCP receives the
active snapshot.

## Historical Context (from `ai/thoughts/`)

- `ai/thoughts/tickets/loomspan-console-pr-34-active-execution-mcp-inspection.md`
  preserves the sanitized 2026-08-21 motivating session. Its installed user
  skill reported 1.0.0; the repository's current canonical authority is 1.0.1.
  The live session itself is historical evidence, not a reproducible fixture.
- `ai/thoughts/phases/2026-08-15-loomspan-llm-trace-understanding-roadmap.md:27-74`
  keeps progressive skill routing active and calls for concise capability and
  question routes without reproducing full schemas.
- `ai/thoughts/phases/2026-08-15-loomspan-llm-trace-understanding-roadmap.md:76-107`
  leaves active-to-finalized trace handoff as an open lifecycle decision.
- `ai/thoughts/phases/2026-08-15-loomspan-llm-trace-understanding-roadmap.md:109-154`
  calls for a small reproducible pre-v1 tools-only and skill-assisted evaluation
  baseline with call, byte, continuation, overflow, and semantic-error evidence.
- `ai/thoughts/phases/2026-08-15-loomspan-llm-trace-understanding-roadmap.md:185-191`
  defers speculative workflow-specific tools until measured evidence shows that
  existing neutral primitives are insufficient.

No earlier research document exists in `ai/thoughts/research/` at this
checkout. Live production code, executable tests, and current fixtures were
therefore the primary sources; the ticket and active roadmap supplied historical
and product context.

## Verification Performed

- `go test ./internal/mcpadapter -run TestCompatible2025ProtocolInitializesListsAndCallsRealRuntimeTool -count=1`
  passed and reproduced the exact discovery snapshot/ceiling.
- `go test ./internal/live ./internal/mcpadapter ./internal/observability ./internal/traceresolution`
  passed.
- `./mvnw.cmd -pl loomspan-spring-boot-starter "-Dtest=SessionUsageServiceTest,LiveActivityProjectorTest,ObservabilityDtoMapperTest,ConsoleRestFixtureCorpusTest,ModelAttemptCallAdvisorIntegrationTest" test`
  passed 35 tests with zero failures or errors.
- `go test ./internal/observability ./internal/mcpadapter` passed.

These were focused research checks. No production implementation, fixture,
schema, skill, evaluation, or documentation file was changed by the research.

## Code References

- `loomspan-console/internal/mcpadapter/output_schemas.go:13-60` — discovery
  ceiling, compact schemas, and complete typed-output validation.
- `loomspan-console/internal/mcpadapter/output_schemas.go:165-185` — compact
  active list/detail/activity property declarations.
- `loomspan-console/internal/mcpadapter/contracts.go:65-138` — complete active
  execution and activity result DTOs.
- `loomspan-console/internal/mcpadapter/executions.go:42-179` — list/detail
  pagination, shared DTO mapping, and divergent text detail.
- `loomspan-console/internal/mcpadapter/activity.go:31-131` — activity scope,
  forward checkpoint construction, result mapping, and safe text fallback.
- `loomspan-console/internal/mcpadapter/continuation.go:14-112` — opaque token
  validation and operation/scope/session binding.
- `loomspan-console/internal/live/service.go:605-752` — shared ring admission,
  filtering, pagination, and global beginning-availability calculation.
- `loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/runtime/observation/LiveActivityProjector.java:48-120`
  — canonical-record-to-live-state usage and terminal projection.
- `loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/observability/web/dto/ObservabilityDtos.java:33-49`
  — REST usage/limit records that omit provider-attempt fields.
- `loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/observability/web/ObservabilityDtoMapper.java:26-52`
  — elapsed-time calculation and REST active snapshot mapping.
- `loomspan-console/internal/observability/dto.go:38-58` — Go integer fields
  that turn absent provider values into zero.
- `loomspan-console/agent-skills/loomspan/SKILL.md:18-104` — current discovery,
  single-investigation routing, evidence, and untrusted-content guidance.
- `loomspan-console/internal/agentskills/validate.go:18-246` — exact skill
  package/version/content validation.
- `loomspan-console/internal/agenteval/server.go:154-282` — current single live
  evaluation target.
- `loomspan-console/internal/agenteval/score.go:51-97` — fixed 28-run release
  matrix.

## Related Research

No prior documents were present in `ai/thoughts/research/` at this checkout.

## Open Questions

The current code and tests establish the facts above. The following contract
choices remain unresolved in the ticket and roadmap rather than answered by
existing implementation:

1. Which subset of orientation names is carried by compact schemas, tool
   descriptions, deterministic text, and skill routing while maintaining a
   newly selected discovery ceiling.
2. Whether activity exposes one continuation with documented dual use or adds
   distinct backlog/checkpoint vocabulary.
3. Which smallest explicit coverage vocabulary represents global-window limits
   and the absence of an authoritative session-from-start fact.
4. How the exact-release Java REST DTO and all generated fixtures represent
   provider attempt usage and disabled provider limits at the Go/MCP boundary.
5. The bounded skill route and evaluation contract for a full active page,
   second observation, retained-backlog traversal, global eviction, genuine
   session incompleteness, and completion between calls.
6. The exact pre-v1 `tools/list` ceiling and client/model evidence used to
   justify it, because the current response has 57 bytes of executable
   headroom and the compatibility document's byte count differs from the
   checked-in snapshot.
