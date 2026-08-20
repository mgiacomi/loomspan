---
date: 2026-08-18T21:45:44-07:00
researcher: GPT-5
git_commit: 6f2fedab9c24969f4eec887d42b8a3c7440e02b6
branch: main
repository: loomspan
topic: "PR 30 — MCP Contract Efficiency and Trace Semantic Corrections"
tags: [research, codebase, mcp, trace-analysis, spring-ai, console]
status: complete
last_updated: 2026-08-18
last_updated_by: GPT-5
---

# Research: PR 30 — MCP Contract Efficiency and Trace Semantic Corrections

**Date**: 2026-08-18T21:45:44-07:00
**Researcher**: GPT-5
**Git Commit**: 6f2fedab9c24969f4eec887d42b8a3c7440e02b6
**Branch**: main
**Repository**: loomspan

## Research Question

Use `ai/commands/1_research_codebase.md` to document the current codebase relevant to `ai/thoughts/tickets/loomspan-console-pr-30-mcp-contract-efficiency-and-trace-semantics.md`.

## Summary

The ticket covers seven behaviors that are all present in the current tree:

1. The twelve installed MCP tools retain complete, constrained input schemas, while their output schemas are currently inferred by the MCP Go SDK from each generic handler's concrete result type. Tool registration does not supply explicit `Tool.OutputSchema` values. The exact serialized discovery response is asserted as 37,788 bytes, but its only size ceiling is the shared 64 KiB compact-response limit (`loomspan-console/internal/mcpadapter/server_test.go:31-96`).
2. `SpringAiProviderIntegration.translate(Throwable)` walks at most twelve causes and recognizes direct `SocketTimeoutException` values as transient `TIMEOUT`, but classifies interruption/cancellation as permanent `UNKNOWN` and has no branch for `InterruptedIOException` or provider-specific timeout wrappers (`loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/springai/SpringAiProviderIntegration.java:230-275`).
3. The tool-exception branch in `StepLoopMissionExecutionEngine` records `STEP_COMPLETED` with `status=failed` and error data, after which the enclosing step frame is closed with outcome `failed` and an `ERROR_RECORDED` fact may also be emitted (`loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/runtime/step/StepLoopMissionExecutionEngine.java:577-590,684-699`). There is no `STEP_FAILED` value in the Java or Go trace vocabularies.
4. Every provider attempt records `MODEL_REQUEST_PREPARED` and `MODEL_REQUEST_SENT` consecutively from the same `requestPayload` before the provider call (`loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/chat/ProviderAttemptCallAdvisor.java:58-72`). The Go attempt graph requires the exact lifecycle `PREPARED -> SENT -> RESPONSE_RECEIVED|ATTEMPT_FAILED` (`loomspan-console/internal/traceanalysis/attempts.go:67-154`).
5. Plan facts currently expose `planId`, record `sequence`, `rootFrameId`, `planningFrameId`, and creation-only accepted attempt/retry identity. `planningFrameId` is initialized from each record's own frame, and `rootFrameId` is assigned by walking that record frame to the topmost frame (`loomspan-console/internal/traceanalysis/record_facts.go:132-141`; `loomspan-console/internal/traceanalysis/query_records.go:212-218,264-291`). Plan lineage is not currently persisted by `planId` during processing.
6. Literal search returns one `SearchResult` per match with a complete opaque `ContentRef` on every content match. MCP and browser map that field directly into each match item (`loomspan-console/internal/traceanalysis/search.go:143-184,321-353`; `loomspan-console/internal/mcpadapter/traces.go:232-252`; `loomspan-console/internal/browserapi/trace_analysis.go:312-338`). There is no page-level content-descriptor collection or page-local content ID.
7. MCP inventory and compact-frame text fallbacks pass optional pointers directly to `%v`, while `fallbackField` accepts only a plain string. Present optional values therefore use Go's pointer formatting in those locations (`loomspan-console/internal/mcpadapter/traces.go:466-485,513-519`).

These surfaces do not touch the closed supported Java Application API. The Java producer types involved are under `com.lokiscale.loomspan.internal` and are explicitly classified as internal or technically public internal machinery by the architecture policy. The trace vocabulary and trace DTOs are current-version ephemeral diagnostic formats. MCP and browser responses are unreleased serialized development contracts whose in-repository consumers, fixtures, tests, agent skill, evaluations, and documentation currently encode the existing vocabulary.

## Detailed Findings

### 1. MCP registration, generated output contracts, and discovery size

#### Installed surface and registration

`NewServer` constructs one stateless MCP server and registers runtime, skill, active-execution, recent-activity, and finalized-trace tool families (`loomspan-console/internal/mcpadapter/server.go:56-73`). The discovery test asserts twelve exact tool names: one runtime tool, two skill tools, two execution tools, one activity tool, and six trace tools (`loomspan-console/internal/mcpadapter/server_test.go:86-96`).

Every tool is read-only, idempotent, non-destructive, and closed-world through the shared annotations (`loomspan-console/internal/mcpadapter/contracts.go:135-143`). Tool handlers return either direct `RuntimeOutput` or a generic `toolEnvelope[T]` containing an optional `result` and optional `error` (`loomspan-console/internal/mcpadapter/contracts.go:17-35,145-157`). `TestToolEnvelopeContainsExactlyOneResultOrError` checks the helper-produced success and error shapes, but it does not validate serialized data against JSON Schema (`loomspan-console/internal/mcpadapter/contracts_test.go:18-34`).

No registration sets `mcp.Tool.OutputSchema`. Runtime registration supplies name, description, and annotations (`loomspan-console/internal/mcpadapter/runtime.go:34-47`). Skills, executions, activity, and trace registrations additionally supply explicit input schemas, while the generic typed handler gives the SDK the result type from which it derives the current output schema (`loomspan-console/internal/mcpadapter/skills.go:27-42`; `loomspan-console/internal/mcpadapter/executions.go:25-40`; `loomspan-console/internal/mcpadapter/activity.go:21-30`; `loomspan-console/internal/mcpadapter/traces.go:19-63`).

#### Input-schema behavior

Input schemas are generated with `jsonschema.For[T]`, then augmented with page bounds, nonblank constraints, enums, and mutually exclusive range-position branches (`loomspan-console/internal/mcpadapter/contracts.go:246-271`; `loomspan-console/internal/mcpadapter/trace_contracts.go:300-338`). Trace registration derives enum contents from authoritative service value lists and applies 1–64 page bounds and 8,192-character token bounds (`loomspan-console/internal/mcpadapter/traces.go:65-139`). Tests verify that trace input schemas are closed, bounded, and have the exact intended public properties and authoritative enum inventories (`loomspan-console/internal/mcpadapter/trace_contracts_test.go:13-106`).

#### Output DTO depth

The generated output graph includes the common error DTO and all concrete nested success DTOs. The trace-record result alone contains evidence, record descriptors, search matches, coverage, pagination, record content descriptors, plan landmarks, attempt/retry/validation/failure facts, diagnostic descriptors, usage, and raw addresses (`loomspan-console/internal/mcpadapter/trace_contracts.go:65-297`). The internal typed DTOs are also used directly by mapping code, for example `mapRecord` populates every fact list and content field (`loomspan-console/internal/mcpadapter/traces.go:418-448`).

There is currently no second compact output-schema representation, no retained explicit full-schema variable beside registration, and no table-driven dual-schema fixture validation. Existing contract tests cover helper envelope behavior, forbidden outward property names, input-schema shape, mapping, fixture semantics, exact discovery bytes, and response bounds (`loomspan-console/internal/mcpadapter/contracts_test.go:18-207`; `loomspan-console/internal/mcpadapter/trace_contracts_test.go:13-124`; `loomspan-console/internal/mcpadapter/trace_semantic_fixtures_test.go`).

#### Discovery and response budgets

The protocol test records `prePR28ToolsListResponseBytes = 34371` and `expectedToolsListResponseBytes = 37788`. It first compares discovery to `traceanalysis.MaxCompactResponseBytes`, then separately asserts exact equality to 37,788 (`loomspan-console/internal/mcpadapter/server_test.go:31-35,86-93`). `MaxCompactResponseBytes` is the general 64 KiB compact-response bound, also used to truncate navigation text fallbacks (`loomspan-console/internal/traceanalysis/limits.go:34`; `loomspan-console/internal/mcpadapter/traces.go:521-528`). There is no dedicated discovery-budget constant in the current tree.

### 2. Provider timeout classification and downstream retry facts

`SpringAiProviderIntegration` creates an internal `ProviderConnectionRuntime` whose failure translator is the private `translate` method. The method traverses the cause chain with identity-cycle protection and a depth limit of twelve (`loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/springai/SpringAiProviderIntegration.java:230-235`). Its current ordered cases are:

- return already normalized `ProviderCallException` details;
- translate Google and Anthropic typed service exceptions;
- translate Spring `RestClientResponseException` values by HTTP status;
- classify `InterruptedException`, `CancellationException`, and `SSLException` as permanent `UNKNOWN`;
- classify `SocketTimeoutException` as transient `TIMEOUT`; and
- classify connection, socket, EOF, and unknown-host failures as transient `CONNECTIVITY` (`SpringAiProviderIntegration.java:236-273`).

An unrelated generic `IOException` falls through to `ProviderFailureDetails.unknown()` (`SpringAiProviderIntegration.java:273-275`). Current Spring AI tests cover typed Google HTTP failures, a nested `SocketException`, permanent TLS classification, and an unrelated `IOException`; they do not construct `InterruptedIOException` or the observed OpenAI wrapper chain (`loomspan-spring-boot-starter/src/test/java/com/lokiscale/loomspan/internal/springai/SpringAiProviderIntegrationTest.java:80-107`).

`ProviderAttemptCallAdvisor` feeds translated details into `ProviderRetryDecider`, then writes classification, category, retry decision, delay, delay source, and optional HTTP/provider identity into `MODEL_ATTEMPT_FAILED` metadata (`loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/chat/ProviderAttemptCallAdvisor.java:73-91,108-124`). The decider retries only `TRANSIENT` classifications when policy is enabled and attempts remain; otherwise it produces `DO_NOT_RETRY` or `ATTEMPTS_EXHAUSTED` (`loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/provider/ProviderRetryDecider.java:9-31`).

Go's attempt parser validates those exact metadata vocabularies, including the invariant that `RETRY` requires `TRANSIENT` and a non-`NONE` delay source (`loomspan-console/internal/traceanalysis/attempts.go:126-149`). Record facts then expose them as `AttemptSummary`, and MCP/browser DTOs serialize them for typed inspection (`loomspan-console/internal/traceanalysis/record_facts.go:143-152`; `loomspan-console/internal/mcpadapter/trace_contracts.go:173-190`; `loomspan-console/internal/browserapi/trace_analysis.go:164-173,521-542`). Live projection also presents bounded retry facts and uses them to form the provider-attempt summary (`loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/runtime/observation/LiveActivityProjector.java:354-356`; `loomspan-spring-boot-starter/src/test/java/com/lokiscale/loomspan/internal/runtime/observation/LiveActivityProjectorTest.java:290-309`).

### 3. Step terminal records and projections

The Java trace enum contains `STEP_STARTED`, action proposed/validated/rejected, and `STEP_COMPLETED`, but not `STEP_FAILED` (`loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/core/TraceRecordType.java:26-35`). The Go closed enum mirrors that vocabulary and advertises it through `RecordTypeValues`, which also feeds MCP's `filter.types` input enum (`loomspan-console/internal/traceanalysis/enums.go:33-65`; `loomspan-console/internal/mcpadapter/traces.go:84-90`).

Normal final-response steps record one `STEP_COMPLETED` before returning (`StepLoopMissionExecutionEngine.java:565-573`). Successful tool steps likewise record `STEP_COMPLETED` with a bounded result preview (`StepLoopMissionExecutionEngine.java:701-712`). In the tool invocation catch block, however, `planningService.markToolFailed` is followed by another `STEP_COMPLETED` whose metadata includes `status=failed` and whose data says `Tool execution failed` (`StepLoopMissionExecutionEngine.java:684-690`). The exception then propagates to the enclosing step catch, which sets the step-frame close status to `failed` unless caller-owned cleanup or thread interruption makes it `aborted`; it also records a failure when cleanup is not caller-owned (`StepLoopMissionExecutionEngine.java:577-590`).

`LiveActivityProjector` includes `STEP_COMPLETED` in its visible set, maps it to `ExecutionActivityKind.STEP_COMPLETED`, assigns phase `STEP`, and summarizes it as “Step completed” (`loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/runtime/observation/LiveActivityProjector.java:21-37,303-323,328-367`). The Java activity enum, Go live enum, TypeScript API union/labels, and UI presentation all contain `STEP_COMPLETED` and no failed-step counterpart (`loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/runtime/observation/ExecutionActivityKind.java:3-23`; `loomspan-console/internal/live/dto.go:19-36`; `loomspan-console/web/src/api/contracts.ts:313-350`; `loomspan-console/web/src/activity/activityPresentation.ts:101-106`).

The journal vocabulary has tool failure and generic error entries but no step terminal entry. Its contract test explicitly ignores raw model request/response records, illustrating that journal vocabulary is a projection rather than a mirror of every trace record (`loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/core/JournalEntryType.java:3-15`; `loomspan-spring-boot-starter/src/test/java/com/lokiscale/loomspan/internal/runtime/trace/ExecutionJournalProjectionContractTest.java:128-140`). Browser record presentation treats `STEP_COMPLETED` as a selectable “Step result” record (`loomspan-console/web/src/observability/TraceRecords.tsx:1147-1165`).

### 4. Duplicate provider-request events and attempt reconstruction

`ProviderAttemptCallAdvisor` builds `requestPayload` once per call and invokes both `recordModelRequestPrepared` and `recordModelRequestSent` with the same object immediately after reserving provider-attempt quota (`ProviderAttemptCallAdvisor.java:58-72,149-163`). `DefaultExecutionStateService` delegates those calls to separate recorder methods, and `DefaultExecutionTraceRecorder` appends distinct record types against the same frame (`loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/runtime/state/DefaultExecutionStateService.java:239-261`; `loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/core/DefaultExecutionTraceRecorder.java:42-54`).

The Go attempt graph treats both events as required lifecycle states. A new attempt may begin on `MODEL_REQUEST_PREPARED`; `MODEL_REQUEST_SENT` is accepted only after prepared; response/failure is accepted only after both (`loomspan-console/internal/traceanalysis/attempts.go:97-154`). Processor dispatch sends all four lifecycle record types through this graph (`loomspan-console/internal/traceanalysis/processor.go:203-213,504-508`). The Java/Go enum pair, fixture corpus, parser tests, MCP semantic fixtures, agent-evaluation fixture, browser record UI, and authoring documentation all contain the prepared value.

Java integration tests currently assert equal prepared/sent cardinality for failure, retry recovery, and retry exhaustion, while provider-attempt usage is counted from the physical attempt boundary (`loomspan-spring-boot-starter/src/test/java/com/lokiscale/loomspan/internal/chat/ModelAttemptCallAdvisorIntegrationTest.java:240-246,281-300,350-367`). Live activity does not expose prepared; it exposes sent and increments provider-attempt projection usage on sent (`LiveActivityProjector.java:21-37,103-111`). The browser record UI nevertheless recognizes both prepared and sent as model requests and gives prepared a separate “Prepared request” presentation (`loomspan-console/web/src/observability/TraceRecords.tsx:1147-1155`; `loomspan-console/web/src/observability/TraceRecords.model.test.tsx:84-94`).

### 5. Current plan landmark derivation

At the producer, `PLAN_CREATED` records framework `planId`, accepted `attemptId`, and `retrySequenceId`; `PLAN_UPDATED` records only `planId` (`loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/core/DefaultExecutionTraceRecorder.java:73-89`). Both pass through `recordOnPlanFrame`, which chooses the first active planning frame, otherwise the first root mission, otherwise the first non-model frame, and finally the active frame with no explicit frame override (`DefaultExecutionTraceRecorder.java:170-193`). The Java trace contract test demonstrates creation on planning frames and later updates on the corresponding root-mission frames (`loomspan-spring-boot-starter/src/test/java/com/lokiscale/loomspan/internal/runtime/trace/ExecutionTraceContractTest.java:328-385`).

The processor does not build a persisted plan-lineage index. During each record query, `materializeRecordFacts` independently creates a `PlanLandmark` for every creation or update, sets `PlanningFrameID` to that record's `FrameID`, and copies attempt/retry only from creation (`loomspan-console/internal/traceanalysis/record_facts.go:132-141`). `QueryRecords` separately loads all frame parents, walks each frame to the topmost root, and assigns that root to the plan fact for the current record (`loomspan-console/internal/traceanalysis/query_records.go:150,212-218,264-291`). Consequently the current `RootFrameID` means topmost trace-frame ancestor, while current `PlanningFrameID` means the individual plan record's attached frame.

The core DTO, MCP DTO, browser DTO, and TypeScript contract all expose the same two names (`loomspan-console/internal/traceanalysis/dto.go:170-177`; `loomspan-console/internal/mcpadapter/trace_contracts.go:235-242`; `loomspan-console/internal/browserapi/trace_analysis.go:491-498`; `loomspan-console/web/src/api/contracts.ts:279-288`). MCP and browser map from the shared `PlanLandmark`, preserving semantic parity (`loomspan-console/internal/mcpadapter/traces.go:418-423`; `loomspan-console/internal/browserapi/trace_analysis.go:115-131`).

The cross-language `current-plan-semantic-evidence` fixture has a primary planning frame beneath `mission-root` and a nested planning frame beneath `nested-skill`, itself beneath the same `mission-root` (`loomspan-spring-boot-starter/src/test/java/com/lokiscale/loomspan/internal/runtime/trace/ConsoleTraceFixtureCorpusTest.java:795-827`). Go assertions currently expect both plans' `RootFrameID` to be `mission-root`, while their creation `PlanningFrameID` values differ (`loomspan-console/internal/traceanalysis/fixture_corpus_test.go:169-195`). The agent evaluation for the final primary plan explicitly says “select by rootFrameId and planId” (`loomspan-console/agent-evals/cases/final-primary-plan.json:1`).

### 6. Literal-search result shape and pagination

`Service.Search` performs bounded, continuable, case-sensitive byte matching across record metadata/data and reconstructed complete text payloads. It carries KMP state through a semantic value, enforces per-call byte/record work bounds, and returns match-level pagination (`loomspan-console/internal/traceanalysis/search.go:14-35,93-124,166-190,321-385`). Search results contain sequence, record type, frame, exact byte offset and length, searched field, and a semantic `ContentRef` for content matches (`loomspan-console/internal/traceanalysis/dto.go:368-382`).

For ordinary record data, the content reference is encoded once while constructing the field but copied into every match from that field (`loomspan-console/internal/traceanalysis/search.go:143-181`). For reconstructed payloads, the opaque content reference is encoded at every match and stored directly on the match (`search.go:337-351`). Continuations store scan position and KMP state, and `pageSize` is applied to the number of match items (`search.go:180-185,346-351,374-385`).

MCP's literal branch maps `page.Items` one-for-one into `matches`, adds one page-level `search` coverage object, and exposes `hasMore` and the opaque continuation (`loomspan-console/internal/mcpadapter/traces.go:232-252`; `loomspan-console/internal/mcpadapter/trace_contracts.go:218-226,271-287`). Browser search performs the same direct mapping and returns the same coverage facts (`loomspan-console/internal/browserapi/trace_analysis.go:312-338,612-639`). TypeScript's `TraceSearchResult` likewise owns an optional `contentRef` (`loomspan-console/web/src/api/contracts.ts:287-307`).

Existing search tests protect payload matching, structural filtering, honest unfinished and complete negative pages, match-level continuation, non-nil empty items, and nonnegative offsets (`loomspan-console/internal/traceanalysis/search_test.go:18-69,128-157,191-221,352-397`). Browser tests assert direct `contentRef` propagation and search coverage/continuation (`loomspan-console/internal/browserapi/trace_analysis_test.go:285-308`). No current response type contains page-local content IDs or a distinct page-level content-descriptor collection.

### 7. MCP text fallback optional-value rendering

Trace inventory DTO fields for session, skill, outcome, and three timestamps are pointers (`loomspan-console/internal/mcpadapter/trace_contracts.go:74-84`). Frame parent ID and closed/duration fields are also pointers (`trace_contracts.go:124-139`). Structured JSON respects `omitempty` and is not affected by Go pointer stringification.

The inventory fallback formats `Outcome`, `FinalizedAt`, `AcquiredAt`, and `ImportedAt` with `%v`; the compact frame fallback does the same for `ParentFrameID` (`loomspan-console/internal/mcpadapter/traces.go:466-485`). `fallbackField` only truncates a concrete string at 512 bytes and cannot format pointers or timestamps (`traces.go:513-519`). Current fallback tests cover maximum navigation size, descriptor payload size, content preservation, and escaping, but contain no nil/present optional-value table and no pointer-address rejection (`loomspan-console/internal/mcpadapter/traces_test.go:22-78`; `loomspan-console/internal/mcpadapter/contracts_test.go:52-90`).

## Contract and Compatibility Classification

| Surface | Current classification | Evidence and current consumers |
| --- | --- | --- |
| Types in `com.lokiscale.loomspan.api` | Application API | Closed allowlist in `LoomspanPublicSurfaceArchitectureTest`; none of the ticket's production types are in this package (`loomspan-spring-boot-starter/src/test/java/com/lokiscale/loomspan/architecture/LoomspanPublicSurfaceArchitectureTest.java:27-37`). |
| Spring provider translator, retry policy, trace recorder, state service, step engine, trace enums, advisors | Internal or accidentally exposed implementation | All are under `com.lokiscale.loomspan.internal`. Technically public provider/integration/advisor types are explicitly allowlisted as internal collaboration machinery (`LoomspanPublicSurfaceArchitectureTest.java:48-94`). No supported SPI is identified. |
| `loomspan.*` provider retry properties | Configuration and manifest contracts | The existing retry policy is built from `LoomspanProperties.ProviderRetryProperties`; the ticket changes classification feeding that policy, not the property names/defaults shown in the researched code (`ProviderRetryPolicy.java:7-25`). |
| Canonical Java trace records and NDJSON | Ephemeral diagnostic formats | Authoring guidance states traces describe the current checkout/current run and are not a durable cross-version API; same-version portability is guarded by exact `consoleCompatibilityVersion` (`ai/skill-authoring/traces-and-debugging.md:35-48`). |
| Generated fixture corpus | Executable same-version diagnostic fixture | Java generates it and Go validates it byte-for-byte; repository guidance directs atomic regeneration rather than compatibility readers (`loomspan-console/AGENTS.md:26-39`; `ai/skill-authoring/traces-and-debugging.md:198-215`). |
| Browser trace-analysis REST JSON | Unreleased serialized application-adapter contract | Browser handler DTOs and TypeScript contracts consume the shared Go analysis model (`loomspan-console/internal/browserapi/trace_analysis.go`; `loomspan-console/web/src/api/contracts.ts:255-307`). Historical workflow policy requires browser/MCP semantic parity. |
| MCP `tools/list`, tool inputs, structured results, text fallback | Unreleased serialized MCP development contract | Exact discovery snapshot, strict input-schema tests, typed handlers, agent skill, eval cases, and docs establish current behavior. Roadmap explicitly says the surface is unreleased and moves atomically (`ai/thoughts/phases/2026-08-15-loomspan-llm-trace-understanding-roadmap.md:230-267`). |
| Agent skill/evaluation vocabulary | In-repository consumer and executable guidance | Tool guide, debugging guidance, eval cases, and fixtures name current record fields and content references (`ai/skill-authoring/traces-and-debugging.md:54-97`; `loomspan-console/agent-evals/cases/final-primary-plan.json:1`). |
| Persisted/cross-version Console state | No affected supported contract found | The Console trace catalog is in-memory and the repository policy requires exact Java/Go release compatibility; imported analysis handles and continuations are current-process data (`loomspan-console/AGENTS.md:3-14`; framework design lens). |

### Public declarations, constructors, beans, and extension points

`SpringAiProviderIntegration` and `ProviderAttemptCallAdvisor` are public Java classes with public constructors, but the architecture allowlist describes them as framework-owned internal assembly boundaries (`LoomspanPublicSurfaceArchitectureTest.java:81,94`). The researched trace enums, state service, recorder, engine, and provider retry types likewise reside under `internal`. No affected production type is in the Application API allowlist, and no affected supported SPI, `@ConditionalOnMissingBean` replacement point, or application bean override contract was found. The current public-surface delta represented by the ticket is therefore zero at the supported Java API/SPI layer.

### Protected protocol consumers and coordinated semantics

The protected in-repository chain for trace-semantic changes is:

```text
Java execution/provider producer
  -> canonical NDJSON record vocabulary and metadata
     -> Java-generated fixture corpus
        -> Go parser, validation, indexes, facts, plans, attempts, search
           -> browser REST DTOs -> TypeScript UI
           -> MCP DTOs, schemas, fallbacks
           -> agent skill, evaluations, and developer documentation
```

The Console's exact `consoleCompatibilityVersion` policy is the current compatibility marker for Java-to-Go trace artifacts. The browser and MCP adapters are peers over the same analysis service rather than independent semantic implementations (`ai/thoughts/phases/2026-08-15-loomspan-llm-trace-understanding-roadmap.md:170-200`).

## Code References

- `loomspan-console/internal/mcpadapter/server.go:56-73` — server construction and tool-family registration.
- `loomspan-console/internal/mcpadapter/server_test.go:31-96` — exact 37,788-byte discovery snapshot and 64 KiB ceiling.
- `loomspan-console/internal/mcpadapter/contracts.go:17-35,135-157,246-271` — generic envelope, annotations, result helpers, and common input schema builders.
- `loomspan-console/internal/mcpadapter/traces.go:19-139` — six trace tool registrations and strict trace input schemas.
- `loomspan-console/internal/mcpadapter/trace_contracts.go:164-297` — deeply typed trace result DTO graph.
- `loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/springai/SpringAiProviderIntegration.java:230-275` — current cause-chain translation.
- `loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/provider/ProviderRetryDecider.java:9-31` — transient-only retry decision.
- `loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/chat/ProviderAttemptCallAdvisor.java:58-124` — duplicate request records and failure fact production.
- `loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/runtime/step/StepLoopMissionExecutionEngine.java:565-590,653-712` — normal and failed step terminal emission.
- `loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/core/DefaultExecutionTraceRecorder.java:73-89,170-193` — plan metadata and record-frame selection.
- `loomspan-console/internal/traceanalysis/attempts.go:67-154` — required four-record attempt vocabulary and lifecycle validation.
- `loomspan-console/internal/traceanalysis/record_facts.go:132-177` — per-record plan and typed fact materialization.
- `loomspan-console/internal/traceanalysis/query_records.go:212-218,264-291` — current topmost-root assignment.
- `loomspan-console/internal/traceanalysis/search.go:143-190,321-385` — per-match content references and continuation.
- `loomspan-console/internal/mcpadapter/traces.go:466-519` — current text fallback formatting.
- `loomspan-console/internal/browserapi/trace_analysis.go:312-338,491-498,612-639` — browser plan/search serialized contracts.
- `loomspan-console/web/src/api/contracts.ts:255-307,313-350` — TypeScript plan, search, and live activity contracts.
- `loomspan-spring-boot-starter/src/test/java/com/lokiscale/loomspan/architecture/LoomspanPublicSurfaceArchitectureTest.java:27-94` — executable Java API/internal classification.

## Architecture Documentation

The current architecture is producer-first and transport-neutral. Java owns canonical trace production and physical provider-attempt facts. A generated corpus carries those semantics into Go. Go validates the closed vocabulary, materializes indexes and typed facts, and exposes one shared analysis service. Browser and MCP adapters map that service into different protocol envelopes without independently deriving failure, plan, search, or hierarchy semantics.

MCP discovery currently combines authored names/descriptions/input schemas with SDK-generated output schemas from typed handlers. MCP runtime success and domain-error responses use a structured result plus a bounded text fallback. Finalized trace operations use `traceId` outwardly while target scope, acquisition, evidence owner, artifact handle, and content-reference encoding remain internal.

Trace content uses opaque, scope-and-artifact-bound references. Record queries provide per-record content descriptors; literal search provides per-match references; exact reads accept only the opaque content reference. Search pagination is match-based and independent of the number of unique content values.

Plan state identity is producer-owned through `planId`, but the currently returned frame landmarks are query-time derivations from each individual record. Attempt state identity is producer-owned through `attemptId` and `retrySequenceId`, with Go validating a fixed record lifecycle and retry invariants.

## Historical Context (from ai/thoughts/)

- `ai/thoughts/framework-feature-design-lens.md` classifies trace representations as ephemeral diagnostics, reserves Application API status for deliberately supported entry points, and requires evidence rather than visibility to establish an API or SPI.
- `ai/thoughts/phases/2026-08-15-loomspan-llm-trace-understanding-roadmap.md:170-215` records the peer-adapter architecture and PR-27 plan identity contract.
- `ai/thoughts/phases/2026-08-15-loomspan-llm-trace-understanding-roadmap.md:230-267` records the current capability families, tool membership, unreleased status, and no-resource design.
- `ai/thoughts/phases/2026-08-15-loomspan-llm-trace-understanding-roadmap.md:480-530` records general semantic content, plan correlation by producer `planId`, and nested root-mission behavior.
- `ai/thoughts/phases/loomspan_llm_trace_understanding_workflows.md:693-755` records envelope-level search coverage, exact match offsets, negative-evidence rules, and opaque content descriptors.
- `ai/thoughts/phases/loomspan_skill_mcp_questions.md:523-526` identifies the successful post-PR-28 walkthrough as the source of the PR-30 correction ticket.
- `ai/skill-authoring/traces-and-debugging.md:89-97` currently documents prepared, sent, and received as the provider-attempt request lifecycle.

## Related Research

No prior documents were present in `ai/thoughts/research/` at research time. The closest live design and workflow context is listed in the Historical Context section.

## Open Questions

- The observed 37,788-byte discovery response and its component breakdown are recorded in the ticket and exact byte test; the current repository does not persist a separate generated discovery snapshot file that attributes bytes per individual field.
- The exact concrete exception classes and message/cause values from trace `6777e217-03af-4a7d-bc2a-c59798fb8f36` are described in the ticket, but that live trace artifact is not committed as a repository fixture.
- The current fixture corpus has no dedicated provider-read-timeout case with the observed OpenAI wrapper chain.
- No current plan fixture contains a malformed `PLAN_UPDATED` without a preceding creation lineage because plan lineage is not yet a processor-owned validated relation.
- No current MCP output-schema validator is selected in the implementation; the code presently relies on SDK type-derived schemas and ordinary Go serialization.
