# PR 30 — MCP Contract Efficiency and Trace Semantic Corrections Implementation Plan

## Overview

Reduce the installed Loomspan Console MCP discovery response from its current 37,788-byte baseline to at most 20 KiB without weakening tool inputs or structured-result correctness, and correct the trace semantics exposed by the 2026-08-18 descriptor-first walkthrough. The implementation will move Java production, the current-version NDJSON vocabulary, Go analysis, browser/MCP adapters, UI projections, fixtures, evaluations, and author-facing debugging guidance together.

The MCP and trace-analysis surfaces are unreleased. Obsolete names and interpretations will be removed in place; this plan does not add aliases, compatibility readers, dual record types, schema-discovery tools, or version negotiation.

## Current State Analysis

The supplied ticket, research, and trace all describe the current commit (`6f2fedab9c24969f4eec887d42b8a3c7440e02b6`). The observed trace contains 104 records and directly demonstrates the producer-side defects:

- Seven physical provider attempts each contain a `MODEL_REQUEST_PREPARED` followed by `MODEL_REQUEST_SENT` with the same attempt identity and equivalent request content. The producer emits both in `ProviderAttemptCallAdvisor` (`loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/chat/ProviderAttemptCallAdvisor.java:58-72`), and Go currently requires both states (`loomspan-console/internal/traceanalysis/attempts.go:67-154`).
- The terminal provider failure is an `OpenAIInvalidDataException("Error reading response")` caused by `InterruptedIOException("timeout")`, then an OkHttp HTTP/2 `CANCEL`. It is recorded as `UNKNOWN`/`DO_NOT_RETRY` because translation recognizes `SocketTimeoutException`, interruption/cancellation, and connectivity failures but not this read-timeout shape (`loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/springai/SpringAiProviderIntegration.java:230-275`).
- Trace sequence 101 is `STEP_COMPLETED` with `metadata.status=failed` and `data.error="Tool execution failed"`. That contradictory event is emitted by the tool-exception path in `StepLoopMissionExecutionEngine` (`loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/runtime/step/StepLoopMissionExecutionEngine.java:684-699`). Model-call and validation failures can close a step frame as failed without any truthful step-terminal record (`StepLoopMissionExecutionEngine.java:450-590`).
- The primary plan is created on its `PLANNING` frame and updated on the top-level `ROOT_MISSION`; the nested plan is created on its nested `PLANNING` frame and updated on the nested `ROOT_MISSION`. Go currently derives `planningFrameId` from whichever record is being queried and `rootFrameId` from the topmost ancestor of that record (`loomspan-console/internal/traceanalysis/record_facts.go:132-141`; `loomspan-console/internal/traceanalysis/query_records.go:212-218,264-291`).

MCP discovery is expensive because generic SDK registration derives the complete result DTO graph as each advertised `outputSchema`. The twelve tools have strict authored/inferred input schemas, but no explicit compact output schemas (`loomspan-console/internal/mcpadapter/server.go:56-73`; `loomspan-console/internal/mcpadapter/traces.go:19-63`). The current protocol test asserts 37,788 bytes and compares only against the unrelated 64 KiB compact-response ceiling (`loomspan-console/internal/mcpadapter/server_test.go:31-96`).

Literal search keeps match-level pagination and exact offsets, but places the same opaque `contentRef` on every content match (`loomspan-console/internal/traceanalysis/search.go:143-190,321-385`). MCP, browser, and TypeScript copy that reference one-for-one (`loomspan-console/internal/mcpadapter/traces.go:232-252`; `loomspan-console/internal/browserapi/trace_analysis.go:312-338`; `loomspan-console/web/src/api/contracts.ts:287-307`).

MCP structured results serialize optional fields correctly, but inventory and compact-frame text fallbacks pass pointers directly to `%v`, producing pointer addresses for present values (`loomspan-console/internal/mcpadapter/traces.go:466-519`).

## Desired End State

After implementation:

1. Every installed tool advertises a compact, authoritative output schema. Tool input schemas remain strict and complete. The typed handler output is validated against both the advertised compact schema and a complete generated schema for the concrete result type.
2. The serialized `tools/list` response has a committed exact snapshot, a recorded exact byte count, and a separate hard budget of 20 KiB or less.
3. A provider read timeout matching the supplied trace is a transient `TIMEOUT` and receives the existing retry policy's decision, while caller cancellation, thread interruption, generic `InterruptedIOException`, HTTP/2 `CANCEL` alone, and unrelated I/O failures do not become retryable timeouts.
4. Successful steps emit exactly one `STEP_COMPLETED`; non-cancellation failures emit exactly one `STEP_FAILED` linked to useful failure identity; aborted/caller-cancelled steps retain abort semantics and do not emit a false failure terminal.
5. Each provider attempt starts with exactly one `MODEL_REQUEST_SENT`; `MODEL_REQUEST_PREPARED` no longer exists in the producer, parser, fixtures, filters, UI, evaluations, or documentation.
6. Every `PLAN_CREATED` and `PLAN_UPDATED` fact exposes stable `traceRootFrameId`, `missionFrameId`, and creation-owned `planningFrameId`. A current-format update without a valid prior creation is rejected as `INVALID_PLAN_LINEAGE` rather than repaired from the update frame.
7. Literal search preserves match-level pagination and all offsets while returning page-local `contentId` values plus a page-level `contentDescriptors` collection that contains each opaque reference once.
8. MCP text fallbacks render optional values deterministically and never contain a Go pointer-address pattern.
9. Java-generated fixtures, Go semantic assertions, browser/MCP DTOs, TypeScript/UI behavior, the packaged debugging Agent Skill, agent-evaluation cases, and skill-authoring guidance use one exact vocabulary.

### Key Discoveries

- The MCP Go SDK accepts an explicit `Tool.OutputSchema` and validates typed handler output against it. Supplying a compact schema therefore gives the advertised-schema check; Loomspan must add a wrapper that independently validates the same serialized output against `jsonschema.For[Out]` to retain full DTO validation.
- `github.com/google/jsonschema-go` is already present through the MCP SDK and supports schema resolution and instance validation, so this work needs no second JSON Schema implementation (`loomspan-console/go.mod:7-18`).
- The Java journal is a projection rather than a mirror of all trace records. PR 30 nevertheless requires failed-step visibility, so `STEP_FAILED` should become an error-level journal entry while `ERROR_RECORDED` remains separate diagnostic evidence (`loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/runtime/trace/ExecutionJournalProjector.java:75-86`).
- The Go processor already persists per-record fact rows after the complete frame graph is validated (`loomspan-console/internal/traceanalysis/processor.go:328-394`). Plan landmarks can be calculated once during processing and persisted with the owning record instead of adding query-page-dependent derivation.
- Search continuations already count matches and preserve KMP state. Page-local content normalization can occur only over the returned matches and therefore does not scan additional source bytes or change work/completeness semantics (`loomspan-console/internal/traceanalysis/search.go:374-385`).
- The Console is pre-release, its trace catalog is process-local, and exact `consoleCompatibilityVersion` equality keeps packaged Java and Go consumers in lockstep (`loomspan-console/AGENTS.md:3-14`). No migration or compatibility shim is justified.

## What We're NOT Doing

- Adding a schema-description tool, MCP resource, external schema URL, `$ref` service, or output-schema negotiation.
- Weakening input schemas, renaming fields to one-letter abbreviations, removing useful tool descriptions, or using compression/minification to conceal discovery size.
- Adding new MCP tools, raw-artifact fallbacks, or cross-trace semantic search.
- Treating every `InterruptedIOException` or OkHttp `CANCEL` as a timeout.
- Treating caller cancellation or thread interruption as `STEP_FAILED`.
- Preserving `MODEL_REQUEST_PREPARED`, failed `STEP_COMPLETED`, plan `rootFrameId`, or per-match `contentRef` through aliases or compatibility parsing.
- Redesigning the general trace envelope, opaque-reference security, KMP search algorithm, match pagination, trace selection, or Console catalog persistence.
- Adding a supported Java API/SPI, bean replacement contract, or configuration property.
- Adding historical/cross-version trace readers or manually versioning development fixtures independently of the existing exact release marker.

## Skill-Authoring Documentation Impact

**Impact**: Affected

- **Rationale**: Skill authors and debugging agents currently learn a three-event provider request lifecycle, choose plans using `rootFrameId`, and follow a `contentRef` directly from each literal match. PR 30 changes those author-facing debugging semantics and adds truthful step-failure and typed timeout evidence.
- **Documents to update**: `ai/skill-authoring/traces-and-debugging.md`; `loomspan-console/agent-skills/loomspan-runtime-debugging/references/mcp-tool-guide.md`; and the relevant packaged skill/evaluation references. The top-level `SKILL.md` needs a terminology pass only if its generic “content reference” language becomes ambiguous; it does not need a new workflow.
- **Supporting evidence**: `ProviderAttemptCallAdvisor` and `ModelAttemptCallAdvisorIntegrationTest`; `StepLoopMissionExecutionEngine`, `LiveActivityProjector`, and their focused tests; `SpringAiProviderIntegrationTest`; `ConsoleTraceFixtureCorpusTest` plus Go `fixture_corpus_test.go`; Go search tests; MCP/browser adapter contract tests; and the regenerated `current-plan-semantic-evidence` corpus.
- **Coverage table update**: Not required. The README already routes this behavior to “Traces and debugging,” whose coverage remains source-verified; no topic boundary or confidence classification changes. The routed document itself must be updated atomically.
- **LLM-first usability**: Replace obsolete terms rather than layering corrections. Use a compact decision table for plan frame identities, state the one-send attempt lifecycle, explain that search `contentId` is page-local and cannot be read directly, and keep the opaque descriptor `contentRef` as the only input to exact content reads. Preserve explicit completeness, sensitivity, and current-version limitations.

The packaged Agent Skill remains version `1.0.0` for this unreleased in-place correction. It is not a target-negotiation mechanism, and no legacy skill behavior is retained.

## Contract and Compatibility Impact

| Surface | Classification and supporting evidence | Planned compatibility treatment |
| --- | --- | --- |
| Application API | No affected type is in the closed `com.lokiscale.loomspan.api` allowlist (`LoomspanPublicSurfaceArchitectureTest`). | Preserve; public-surface delta is zero. |
| Supported SPI | No affected supported extension or bean-replacement point exists. The technically public Spring AI integration/advisor types are allowlisted internal collaboration machinery. | Preserve; do not create an SPI. |
| Configuration and manifest contracts | Existing provider-retry properties determine the decision after classification, but no key, default, manifest syntax, or validation rule changes. | Preserve keys/defaults; update only debugging guidance about observed retry facts. |
| Persisted or serialized contracts | MCP and browser JSON are unreleased serialized adapter contracts. Console catalogs, indexes, continuations, and imported analysis state are process-local. | Break MCP/browser DTOs atomically in place; no migration or dual schema. |
| Ephemeral diagnostic formats | Java trace record vocabulary removes `MODEL_REQUEST_PREPARED`, adds `STEP_FAILED`, changes plan facts, and regenerates the current-version corpus. | Update writer, reader, indexes, fixtures, filters, projections, docs, and evaluations together; reject obsolete/currently malformed forms. |
| Internal or accidentally exposed implementation | Provider translation, advisors, recorder/state interfaces, step engine, Go indexes/DTOs, and adapter helpers are internal. | Remove obsolete methods/enum values and update all repository consumers atomically. |

- **Evidence of supported contracts**: The Java API architecture allowlist, repository guidance, the ticket's explicit pre-release decision, same-version trace guidance, exact MCP discovery tests, generated corpus, browser/TypeScript contracts, and packaged Agent Skill/evaluations.
- **Intended breaks**: Remove `MODEL_REQUEST_PREPARED`; replace failed `STEP_COMPLETED` with `STEP_FAILED`; replace plan `rootFrameId` with `traceRootFrameId` plus `missionFrameId`; replace match-level `contentRef` with `contentId` plus page descriptors; and compact advertised MCP output schemas while retaining structured JSON.
- **In-repository consumers to update**: Java recorder/state/advisor/engine/projections/tests; Java-generated trace fixtures and expected analyses; Go enums, parser, processor, attempts, plan/search facts and tests; MCP/browser contracts and tests; TypeScript contracts, presentations, and component tests; agent fixtures/cases; authoring and packaged-skill documentation.
- **Public-surface delta**: No supported Java types, methods, constructors, or Spring extension points are added or removed. Internal public methods for `recordModelRequestPrepared` are removed and internal enum values change.
- **Shim decision**: **No shim.** The affected protocols are unreleased/current-version diagnostics with no protected external consumer, and repository policy requires one coherent contract.
- **Java-to-Go boundary coordination**: **Required.** Java trace vocabulary/metadata, the generated NDJSON/expected corpus, Go parsing/indexing, MCP/browser DTOs, TypeScript consumers, tests, and documentation must ship in one repository change.
- **Compatibility-marker decision**: Keep the existing exact release-derived `consoleCompatibilityVersion`; do not introduce a PR-specific schema version or range. Packaged releases remain exact-match. Development builds make no cross-commit compatibility promise, so regeneration and atomic update are sufficient.

## Implementation Approach

Use producer-owned truth and one transport-neutral Go analysis model:

```text
Java producer semantics
  -> current-version NDJSON vocabulary and metadata
     -> generated cross-language corpus
        -> Go validation, attempts, plan lineage, and search pages
           -> browser REST + TypeScript presentation
           -> MCP compact discovery + full structured-result validation
           -> packaged debugging skill and authoring guidance
```

MCP schemas will use a shared typed registration helper. Each registration supplies its strict input schema and an authored compact output schema. The helper derives and resolves the full `Out` schema once, wraps the handler to marshal the actual result exactly as it will become `structuredContent`, validates that JSON against the full schema, and then delegates to `mcp.AddTool`; the SDK performs the second validation against the explicit compact schema. Both validation failures are server defects, not domain-error envelopes.

Plan lineage will be built during processing. A plan tracker records creation and update records by `planId`; after the frame graph is validated, it resolves the creation frame's topmost trace root and nearest `ROOT_MISSION` ancestor, persists a `PlanLandmark` for each plan record, and rejects missing/duplicate/invalid creation lineage as `INVALID_PLAN_LINEAGE`. Query code will read the persisted landmark without walking the current record's frame hierarchy.

Search will return a dedicated search page type. Matches in one page receive deterministic `c1`, `c2`, … IDs in first-match order for unique non-empty opaque references. Metadata matches have no content ID. `contentDescriptors` is present as an empty array for a completed/continuable page with no content matches; MCP ordinary record-query results omit it. Continuations and `pageSize` continue to count match items.

## Phase 1: Compact and Validate MCP Output Contracts

### Overview

Replace SDK-inferred advertised result graphs with compact authored schemas for all twelve installed tools while retaining full typed-result validation, exact discovery regression coverage, and complete inputs.

### Changes Required

#### 1. Shared typed tool registration and schema builders

**Files**:

- `loomspan-console/internal/mcpadapter/contracts.go`
- `loomspan-console/internal/mcpadapter/output_schemas.go` (new)
- `loomspan-console/internal/mcpadapter/output_schemas_test.go` (new)

**Changes**:

- Add a generic registration helper that accepts the `mcp.Tool`, compact output schema, and typed handler.
- Derive `jsonschema.For[Out]` once and validate every serialized handler result against that complete schema before the SDK validates it against the compact schema.
- Centralize small schema constructors for closed objects, required properties, arrays, pagination, evidence identity, and the exclusive success/domain-error envelope.
- Model the envelope with `oneOf`: exactly one of `result` or `error` is required and the other is forbidden. Keep the complete error code/message/details DTO internally; advertise the stable decision fields and allow richer details without re-enumerating them per tool.
- Keep schemas authoritative: fields declared by the compact schema must match real JSON names/types/requiredness, and intentionally open nested evidence/fact objects must use explicit `additionalProperties` rather than vague prose.

#### 2. Compact schema per installed tool

**Files**:

- `loomspan-console/internal/mcpadapter/runtime.go`
- `loomspan-console/internal/mcpadapter/skills.go`
- `loomspan-console/internal/mcpadapter/executions.go`
- `loomspan-console/internal/mcpadapter/activity.go`
- `loomspan-console/internal/mcpadapter/traces.go`
- `loomspan-console/internal/mcpadapter/trace_contracts.go`

**Changes**:

- Route every registration through the shared helper and set explicit compact output schemas.
- Preserve stable decision/navigation fields:
  - runtime capabilities and target/status decisions;
  - list item identities, observation/completeness facts, `hasMore`, and continuation;
  - skill YAML identity;
  - execution/session/trace identity, status, phase, path, usage, and limits;
  - activity cursor/identity/time/kind plus continuity/beginning availability;
  - trace evidence identity, outcome, counts, roots, gaps/uncertainties, and usage completeness;
  - frame identity/hierarchy/projection and page controls;
  - record identity, representation/content navigation, typed-fact existence, literal-search coverage, page-local content descriptors, and page controls;
  - exact range offsets, total length, encoding/content, and continuation.
- Keep tool descriptions focused on selection and safety; do not restate nested fields removed from discovery.

#### 3. Exact discovery snapshot and dedicated budget

**Files**:

- `loomspan-console/internal/mcpadapter/server_test.go`
- `loomspan-console/internal/mcpadapter/testdata/tools-list-response.json` (new exact wire snapshot)

**Changes**:

- Compare the serialized `tools/list` body byte-for-byte with a committed snapshot and retain exact tool-name/input-schema assertions.
- Introduce a dedicated `20 << 10` discovery budget in `mcpadapter`; do not reuse `traceanalysis.MaxCompactResponseBytes`.
- Keep the exact post-change byte count adjacent to the test failure and assert the explanatory budget separately.
- Add negative compact-schema tests for both/neither envelope branches and missing core identities/pagination/completeness fields.
- Add a table covering every tool's real success shape and every enveloped tool's domain-error shape; validate each against both compact and full schemas.

### Success Criteria

#### Automated Verification

- [x] All twelve tools retain their complete current input property sets, constraints, enums, and requiredness.
- [x] Every success/domain-error result fixture validates against the compact and full schemas; malformed envelopes/core navigation omissions fail.
- [x] The exact discovery snapshot matches and the serialized response is no more than 20 KiB.
- [x] Focused MCP tests pass: `cd loomspan-console; go test ./internal/mcpadapter`.

#### Manual Verification

- [x] Inspect the snapshot and confirm reductions come from deliberate schema shape, not minified names, removed input constraints, or lost tool-selection descriptions.
- [ ] Confirm a client displays all twelve tools and complete inputs without truncation.

---

## Phase 2: Correct Java Provider and Step Trace Semantics

### Overview

Make the Java producer emit truthful timeout, step-terminal, and provider-request facts before updating downstream consumers.

### Changes Required

#### 1. Narrow provider read-timeout recognition

**Files**:

- `loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/springai/SpringAiProviderIntegration.java`
- `loomspan-spring-boot-starter/src/test/java/com/lokiscale/loomspan/internal/springai/SpringAiProviderIntegrationTest.java`
- `loomspan-spring-boot-starter/src/test/java/com/lokiscale/loomspan/internal/chat/ModelAttemptCallAdvisorIntegrationTest.java`

**Changes**:

- Add one causal-chain helper for provider read deadlines. Classify an `InterruptedIOException` as transient `TIMEOUT` only when it has a timeout/timed-out message, the chain is not a caller `CancellationException`/`InterruptedException`, and the current thread is not interrupted. Recognize the supplied OpenAI `Error reading response` wrapper shape, but do not depend on OkHttp `CANCEL` as the deciding fact.
- Preserve the existing higher-priority normalized provider/HTTP/TLS and direct `SocketTimeoutException` cases.
- Test the exact OpenAI wrapper/cause chain, direct socket timeout, direct provider `InterruptedIOException("timeout")`, caller cancellation, interrupted thread, non-timeout `InterruptedIOException`, isolated HTTP/2 cancellation, and unrelated `IOException`.
- Exercise the public integration path through `ProviderConnectionRuntime`/advisor so `MODEL_ATTEMPT_FAILED` records `TRANSIENT`, `TIMEOUT`, and the existing policy's `RETRY` or `ATTEMPTS_EXHAUSTED` decision.

#### 2. Add a truthful failed-step terminal

**Files**:

- `loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/core/TraceRecordType.java`
- `loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/runtime/step/StepLoopMissionExecutionEngine.java`
- `loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/runtime/observation/ExecutionActivityKind.java`
- `loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/runtime/observation/LiveActivityProjector.java`
- `loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/core/JournalEntryType.java`
- `loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/runtime/trace/ExecutionJournalProjector.java`
- Focused step, activity, journal, and trace contract tests under `loomspan-spring-boot-starter/src/test/java/com/lokiscale/loomspan/internal/`

**Changes**:

- Add `STEP_FAILED`; keep `STEP_COMPLETED` only for successful final-response/tool results.
- Centralize failed terminal emission in the outer step failure path so model, validation, and tool failures all emit once. Remove the tool catch's false completion event.
- Call `recordFailure` before emitting `STEP_FAILED`, reuse the returned stable `failureId`, and include step number/action plus known task/tool/exception identity and a bounded message. Join to provider category through the failure/attempt identity rather than duplicating provider classification logic in the step engine.
- When caller cleanup owns cancellation or the current thread is interrupted, close the frame as aborted and emit neither `STEP_FAILED` nor `STEP_COMPLETED`.
- Project `STEP_FAILED` as an error activity with a “Step failed” summary and add an error-level `STEP_FAILURE` journal entry. Keep `TOOL_CALL_FAILED` and `ERROR_RECORDED` as their distinct tool/diagnostic evidence.
- Test exactly one terminal record for successful tool, successful final response, model failure, tool failure, validation exhaustion, and no terminal pair for caller abort.

#### 3. Remove the fictitious prepared-request phase

**Files**:

- `loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/chat/ProviderAttemptCallAdvisor.java`
- `loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/core/TraceRecordType.java`
- `loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/core/ExecutionTraceRecorder.java`
- `loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/core/DefaultExecutionTraceRecorder.java`
- `loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/runtime/state/ExecutionStateService.java`
- `loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/runtime/state/DefaultExecutionStateService.java`
- `ModelAttemptCallAdvisorIntegrationTest`, `ExecutionStateServiceTest`, `ExecutionTraceContractTest`, `ExecutionTraceBoundaryCleanupTest`, and affected projection tests.

**Changes**:

- Remove `MODEL_REQUEST_PREPARED` and all recorder/state methods and call sites.
- Emit `MODEL_REQUEST_SENT` once after quota reservation/attempt identity creation and immediately before `chain.nextCall`; this remains the truthful boundary for a failure thrown by the provider chain.
- Update cardinality, payload, quota, retry, failure, and cleanup tests to assert one sent event per physical attempt.

### Success Criteria

#### Automated Verification

- [x] Timeout translation and attempt-fact integration tests cover all required positive and negative cases.
- [x] Step tests prove truthful one-of completed/failed/aborted semantics across model, tool, and validation paths.
- [x] Java sources/tests outside historical thought documents contain no `MODEL_REQUEST_PREPARED` reference.
- [x] Focused Java tests pass with a repository-root command such as: `./mvnw.cmd -pl loomspan-spring-boot-starter test "-Dtest=SpringAiProviderIntegrationTest,ModelAttemptCallAdvisorIntegrationTest,StepLoopMissionExecutionEngineTest,LiveActivityProjectorTest,ExecutionJournalProjectorTest,ExecutionJournalProjectionContractTest,ExecutionTraceContractTest,ExecutionTraceBoundaryCleanupTest" -DfailIfNoTests=false`.
- [x] Supported-surface classification remains unchanged: `./mvnw.cmd -pl loomspan-spring-boot-starter test -Dtest=LoomspanPublicSurfaceArchitectureTest -DfailIfNoTests=false`.

#### Manual Verification

- [ ] Compare a reproduced OpenAI read timeout with the supplied trace and confirm its typed attempt fact is sufficient without opening the stack diagnostic.
- [ ] Inspect live/journal summaries for a failed tool step and confirm none say “Step completed.”

---

## Phase 3: Rebuild Go Attempt, Plan, and Search Semantics

### Overview

Update the current-version Go processor to consume the smaller trace vocabulary, persist stable producer-owned plan lineage, and return normalized literal-search pages.

### Changes Required

#### 1. Closed trace vocabulary and attempt graph

**Files**:

- `loomspan-console/internal/traceanalysis/enums.go`
- `loomspan-console/internal/traceanalysis/attempts.go`
- `loomspan-console/internal/traceanalysis/processor.go`
- `loomspan-console/internal/traceanalysis/calculations_test.go`
- `loomspan-console/internal/traceanalysis/processor_test.go`
- `loomspan-console/internal/traceanalysis/service_test.go`

**Changes**:

- Remove `MODEL_REQUEST_PREPARED`, add `STEP_FAILED`, and let `MODEL_REQUEST_SENT` create an attempt.
- Require response/failure only after sent, retain attempt/retry identity and physical-attempt invariants, and keep missing-terminal gap detection.
- Reject obsolete prepared records as unsupported current-format values; do not accept both lifecycles.

#### 2. Processor-owned plan lineage

**Files**:

- `loomspan-console/internal/traceanalysis/plans.go` (new)
- `loomspan-console/internal/traceanalysis/errors.go`
- `loomspan-console/internal/traceanalysis/model.go`
- `loomspan-console/internal/traceanalysis/dto.go`
- `loomspan-console/internal/traceanalysis/processor.go`
- `loomspan-console/internal/traceanalysis/record_facts.go`
- `loomspan-console/internal/traceanalysis/query_records.go`
- `loomspan-console/internal/traceanalysis/manifest.go` only if a new persisted component is required; prefer existing per-record facts.
- Plan-focused processor, query, continuation, and fixture tests.

**Changes**:

- Add `CategoryInvalidPlanLineage = "INVALID_PLAN_LINEAGE"`.
- Track creation/update records by nonblank producer `planId`. Reject an update before creation, duplicate creation for one ID, creation outside a valid `PLANNING` frame, or creation without a `ROOT_MISSION` ancestor/top-level root.
- After frame validation, derive once from the creation frame:
  - `traceRootFrameId`: topmost ancestor;
  - `missionFrameId`: nearest ancestor whose frame type is `ROOT_MISSION`;
  - `planningFrameId`: creation record's `PLANNING` frame.
- Persist those three identities on every plan record's fact row. Preserve `attemptId`/`retrySequenceId` only on the creation record where they were recorded.
- Replace `PlanLandmark.RootFrameID` with `TraceRootFrameID` and `MissionFrameID`; remove query-time `loadFrameRoots` assignment used only for plan facts.
- Keep multiple `planId` lineages independent even when they share a trace root.

#### 3. Page-local search content descriptors

**Files**:

- `loomspan-console/internal/traceanalysis/dto.go`
- `loomspan-console/internal/traceanalysis/search.go`
- `loomspan-console/internal/traceanalysis/search_test.go`
- `loomspan-console/internal/traceanalysis/continuation_test.go`
- `loomspan-console/internal/traceanalysis/fixture_corpus_test.go`

**Changes**:

- Introduce a dedicated `SearchPage` and `SearchContentDescriptor` rather than adding search-only fields to generic pages.
- Replace `SearchResult.ContentRef` with optional `ContentID`.
- At page finalization, assign `c1`, `c2`, … in first-occurrence order for each unique non-empty opaque reference; emit `{contentId, contentRef}` once in `ContentDescriptors`. Metadata-only matches carry no ID.
- Keep descriptor construction page-local and after bounded scanning so it cannot change KMP state, byte/record work bounds, case behavior, continuation fingerprints, or match count.
- Test repeated matches in one value, several values, metadata plus content, a descriptor repeated across offsets, continuation between pages, no matches, complete negative evidence, and serialized reference uniqueness.

### Success Criteria

#### Automated Verification

- [x] Attempt parser tests accept only `SENT -> RESPONSE|FAILED` and reject the removed vocabulary.
- [x] Plan tests prove all three frame IDs are stable on creation and updates for primary/nested/multiple plans and reject malformed lineage as `INVALID_PLAN_LINEAGE`.
- [x] Search tests preserve every offset and match-level continuation while serializing each opaque reference once per page.
- [x] Focused Go analysis tests pass: `cd loomspan-console; go test ./internal/traceanalysis`.

#### Manual Verification

- [x] Inspect a nested plan fact and confirm the owning nested mission is visible without caller hierarchy reconstruction.
- [x] Inspect a repeated-content search page and confirm `contentId` values are short/page-local and cannot be confused with exact-read references.

---

## Phase 4: Align MCP, Browser, Live, and UI Projections

### Overview

Map the corrected neutral semantics into both adapters and user-visible projections without independently re-deriving them.

### Changes Required

#### 1. MCP and browser trace DTOs

**Files**:

- `loomspan-console/internal/mcpadapter/trace_contracts.go`
- `loomspan-console/internal/mcpadapter/traces.go`
- `loomspan-console/internal/mcpadapter/traces_test.go`
- `loomspan-console/internal/mcpadapter/trace_semantic_fixtures_test.go`
- `loomspan-console/internal/mcpadapter/trace_joined_adapters_test.go`
- `loomspan-console/internal/browserapi/router.go`
- `loomspan-console/internal/browserapi/trace_analysis.go`
- `loomspan-console/internal/browserapi/trace_analysis_test.go`
- `loomspan-console/internal/browserapi/contracts_test.go`

**Changes**:

- Replace plan `rootFrameId` with `traceRootFrameId` and `missionFrameId`; preserve `planningFrameId`, `planId`, sequence, and creation-only attempt/retry fields.
- Return literal matches with `contentId` and return `contentDescriptors` once per search page in MCP and browser responses.
- In MCP's dual-mode record result, omit `contentDescriptors` for ordinary record pages but include a non-nil empty array for a literal page with no content matches.
- Keep browser/MCP joined semantic tests over the same trace-analysis result.

#### 2. Optional-value fallback formatting

**Files**:

- `loomspan-console/internal/mcpadapter/traces.go`
- `loomspan-console/internal/mcpadapter/traces_test.go`
- `loomspan-console/internal/mcpadapter/contracts_test.go`

**Changes**:

- Add one generic optional formatter used by all pointer-valued fallback fields.
- Render nil as `-`, a present blank string/zero timestamp as `unknown`, a present string through the existing bounded/escaped field formatter, and a present timestamp as UTC RFC3339Nano.
- Replace direct `%v` rendering for inventory outcome/timestamps and compact frame parent/closed/duration values; exhaustively search fallback builders for other pointer sites.
- Add table tests and reject `0x[0-9a-fA-F]+` in serialized fallback text.

#### 3. Browser/live TypeScript vocabulary and presentation

**Files**:

- `loomspan-console/internal/live/dto.go`
- Live service/coordinator tests that enumerate activity kinds.
- `loomspan-console/web/src/api/contracts.ts`
- `loomspan-console/web/src/activity/activityPresentation.ts` and activity tests/components.
- `loomspan-console/web/src/observability/TraceRecords.tsx`
- `TraceRecords.model.test.tsx`, `TraceRecords.results.test.tsx`, and affected trace-view tests.
- Relevant browser fixtures and E2E expectations.

**Changes**:

- Add `STEP_FAILED` activity vocabulary/presentation as error evidence and retain `STEP_COMPLETED` as non-error success.
- Remove prepared-request record labels/tests; retain sent request as the one model-request presentation.
- Update plan/search TypeScript contracts and components to use the new fields and descriptor lookup.
- Ensure exact content reads use the descriptor's opaque `contentRef`, never the page-local `contentId`.

### Success Criteria

#### Automated Verification

- [x] MCP and browser contract tests prove semantic parity for plans and search pages.
- [x] Fallback tests cover nil, unknown, present strings/timestamps/durations and contain no pointer addresses.
- [x] UI tests show `STEP_FAILED` as an error, show `STEP_COMPLETED` only for success, remove prepared-request presentation, and resolve search descriptors correctly.
- [x] Focused adapter/live tests pass: `cd loomspan-console; go test ./internal/mcpadapter ./internal/browserapi ./internal/live`.
- [x] Frontend typecheck/tests pass through `cd loomspan-console; go run ./internal/buildtool verify` (also runs the canonical Go suite and Agent Skill validation).

#### Manual Verification

- [x] Browser and MCP show the same three plan frame identities and the same search descriptor relationships.
- [x] Inventory/compact-frame fallback text is readable for present/absent values and contains no implementation address.

---

## Phase 5: Regenerate Cross-Language Evidence and Complete Acceptance

### Overview

Regenerate the producer-owned fixture corpus, update executable guidance/evaluations, remove all obsolete vocabulary, and run the fresh-client walkthrough.

### Changes Required

#### 1. Java-generated fixture corpus

**Files**:

- `loomspan-spring-boot-starter/src/test/java/com/lokiscale/loomspan/internal/runtime/trace/ConsoleTraceFixtureCorpusTest.java`
- Generated `loomspan-console-fixtures/traces/*.ndjson`
- Generated `loomspan-console-fixtures/expected/*.json`
- `loomspan-console/internal/traceanalysis/fixture_corpus_test.go`

**Changes**:

- Add/adjust producer cases for the observed timeout and retry fact, successful/failed step terminals, single request-sent events, primary/nested/multiple plan lineages, malformed update-before-create, and repeated-content search.
- Regenerate through the intentional Java workflow; do not hand-edit generated files to satisfy Go.
- Review the full corpus diff as a current-version contract change and preserve LF line endings.

#### 2. Agent Skill, evaluations, and authoring guidance

**Files**:

- `ai/skill-authoring/traces-and-debugging.md`
- `loomspan-console/agent-skills/loomspan-runtime-debugging/references/mcp-tool-guide.md`
- `loomspan-console/agent-skills/loomspan-runtime-debugging/SKILL.md` only if the generic content-reference wording needs disambiguation.
- `loomspan-console/agent-evals/cases/final-primary-plan.json`
- Add/update a nested-plan/search/timeout case where existing cases do not exercise the corrected semantics.
- `loomspan-console/agent-evals/fixtures/composite-adversarial.ndjson`
- Evaluation harness expectations affected by exact vocabulary.

**Changes**:

- Teach the one-send provider attempt lifecycle; `STEP_FAILED`; typed timeout/retry facts; and the plan landmark decision table.
- Teach literal-search `contentId -> page descriptor -> opaque contentRef -> read` and explicitly prohibit passing a `contentId` to `LOOMSPAN_read_trace_content`.
- Replace “primary root plus planId”/`rootFrameId` instructions with `traceRootFrameId`, `missionFrameId`, and `planId` selection rules.
- Remove the prepared record from adversarial/evaluation fixtures while preserving the inert-content test.
- Keep guidance compact, self-contained, source-anchored, and explicit about completeness/current-version limitations.

#### 3. Exhaustive obsolete-vocabulary audit and acceptance record

**Files**:

- All production, test, fixture, browser, MCP, Agent Skill, and evaluation files found by repository-wide search.
- The appropriate PR notes/evaluation record location selected during implementation; do not alter historical ticket/research documents merely to erase historical evidence.

**Changes**:

- Search non-historical sources for `MODEL_REQUEST_PREPARED`, failed interpretations of `STEP_COMPLETED`, plan `rootFrameId`, and match-level `contentRef`.
- Run a fresh stateless MCP connection without seeding the trace ID. Record exact discovery bytes and the client's approximate token count, then complete the ticket's ten-step walkthrough.
- Keep historical `ai/thoughts/tickets/` and `ai/thoughts/research/` references as dated evidence rather than rewriting them to look current.

### Success Criteria

#### Automated Verification

- [x] Regenerate fixtures: `./mvnw.cmd -pl loomspan-spring-boot-starter test -Dtest=ConsoleTraceFixtureCorpusTest -Dloomspan.console.fixtures.regenerate=true -DfailIfNoTests=false`.
- [x] Verify Java corpus without regeneration: `./mvnw.cmd -pl loomspan-spring-boot-starter test -Dtest=ConsoleTraceFixtureCorpusTest -DfailIfNoTests=false`.
- [x] Run the complete Console suite: `cd loomspan-console; go test ./...`.
- [x] Run canonical Console verification: `cd loomspan-console; go run ./internal/buildtool verify`.
- [x] Run `go test -race ./...` only if implementation introduces shared mutable schema caches or concurrent validation/registration state; prefer immutable registration-time schemas so the race run is not otherwise required.
- [x] Repository-wide searches find no obsolete vocabulary outside explicitly historical documents and intentional negative tests.
- [x] `LoomspanPublicSurfaceArchitectureTest` still passes after all production-type changes.

#### Manual Verification

- [ ] Fresh client sees the complete `tools/list`, at most 20 KiB, with recorded exact bytes and approximate tokens.
- [ ] The client discovers the newest trace without a seeded ID and completes compact orientation without raw NDJSON.
- [x] Primary and nested plans expose stable trace-root, mission, and planning-frame identities.
- [ ] Timeout facts say `TIMEOUT` and show the policy-derived retry decision without diagnostic reading.
- [x] Failed steps have `STEP_FAILED`, failed frames, and no contradictory completion.
- [x] Each attempt has one sent request and no prepared request.
- [x] Repeated literal matches retain all offsets, use one descriptor per content value, and support exact reading from the descriptor.
- [x] Inventory/frame fallback text contains values or explicit `-`/`unknown`, never pointer addresses.
- [x] A complete negative search still reports explicit coverage and `workComplete=true`.

## Testing Strategy

Create the dedicated testing-plan artifact with `ai/commands/3_testing_plan.md` before implementation. It should sequence failing tests before production changes for each correction and retain these high-level layers:

### Unit Tests

- Compact/full JSON Schema validation, envelope exclusivity, required navigation fields, and discovery budget/snapshot.
- Timeout causal-chain recognition and cancellation/interruption negatives.
- Step terminal one-of behavior and projection summaries.
- Sent-first attempt state machine and removed-enum rejection.
- Plan tracker identity derivation and invalid-lineage category.
- Search descriptor normalization, exact offsets, continuation, and empty pages.
- Optional fallback rendering and pointer-address rejection.

### Integration and Contract Tests

- Advisor failure facts plus retry decisions.
- Step engine + live activity + journal projection.
- Java producer corpus + Go byte/semantic fixture validation.
- Go neutral model + MCP/browser joined adapter parity.
- TypeScript contracts and record/activity presentation.
- Exact MCP protocol discovery and real tool success/domain-error calls.

### Manual Testing Steps

1. Restart the development Java target and Console after automated tests.
2. Connect a fresh stateless MCP client and record discovery bytes/token observation.
3. Execute the ticket's unseeded ten-step trace walkthrough.
4. Save only non-sensitive acceptance evidence; do not record MCP credentials, absolute private paths, full diagnostic payloads, or unrelated returned content.

## Performance Considerations

- Compact advertised schemas remove the dominant discovery cost while full schemas are resolved once at tool registration. Per-call full validation adds one bounded structured-result marshal/validation pass; measure it in existing handler tests and avoid shared mutable caches.
- Search normalization is O(matches on the returned page), bounded by `pageSize <= 64`, and must not scan beyond existing byte/record work limits.
- Removing `MODEL_REQUEST_PREPARED` eliminates one record and duplicate payload storage/search work per provider attempt.
- Persisting plan landmarks increases per-plan record facts by two frame identifiers but removes repeated query-time hierarchy loading/walking for plan facts.
- Page-local IDs shorten positive search responses substantially when many offsets belong to the same content value; descriptor count is bounded by match count.

## Migration Notes

No data migration, catalog backfill, compatibility shim, or legacy reader is needed. Console catalogs and derived indexes are transient, and imported artifacts must match the exact packaged `consoleCompatibilityVersion`. Existing development traces using removed vocabulary may stop opening after this change; reproduce them with the current checkout or retain the original matching Console build for historical inspection.

Fixtures are regenerated atomically from Java. The existing release-derived compatibility marker remains authoritative; do not manually edit it or add a second schema marker for PR 30.

## References

- Original ticket: `ai/thoughts/tickets/loomspan-console-pr-30-mcp-contract-efficiency-and-trace-semantics.md`
- Research: `ai/thoughts/research/2026-08-18-PR-30-mcp-contract-efficiency-and-trace-semantics.md`
- Supplied trace evidence: `ai/thoughts/research/6777e217-03af-4a7d-bc2a-c59798fb8f36..ndjson`
- Framework compatibility lens: `ai/thoughts/framework-feature-design-lens.md`
- Console repository policy: `loomspan-console/AGENTS.md`
- Trace authoring guidance: `ai/skill-authoring/traces-and-debugging.md`
- MCP debugging guide: `loomspan-console/agent-skills/loomspan-runtime-debugging/references/mcp-tool-guide.md`
- Trace-understanding roadmap: `ai/thoughts/phases/2026-08-15-loomspan-llm-trace-understanding-roadmap.md`
