---
date: 2026-08-15T23:16:16-07:00
researcher: Codex
git_commit: c450bc87b528ec1894881d169dd324159716470a
branch: main
repository: loomspan
topic: "PR 27 - Framework-owned plan identity and accepted-attempt lineage"
tags: [research, codebase, planning, plan-identity, trace-lineage]
status: complete
last_updated: 2026-08-15
last_updated_by: Codex
last_updated_note: "Recorded the ticket decisions for unsolicited planId handling, missing attempt lineage, ID injection, and producer fixture scope"
---

# Research: PR 27 - Framework-owned plan identity and accepted-attempt lineage

**Date**: 2026-08-15 23:16:16 PDT
**Researcher**: Codex
**Git Commit**: c450bc87b528ec1894881d169dd324159716470a
**Branch**: main
**Repository**: loomspan

## Research Question

Research the current codebase context for `ai/thoughts/tickets/loomspan-framework-pr-27-plan-identity-and-lineage.md`: how plan identity is produced and preserved today, how accepted plans relate to model-attempt and retry evidence, how plan records are attached and serialized, which tests and consumers exercise those behaviors, and how the affected surfaces are currently classified.

## Summary

Today, planning asks the model to author every field of an `ExecutionPlan`, including `planId`. The returned JSON or YAML tree is status-normalized and then deserialized directly into `ExecutionPlan`; the strict planning codecs reject unknown fields. `ExecutionPlan` requires the resulting `planId` to be nonblank, and all four immutable copy operations pass that same value into the replacement instance. Nested YAML-skill execution snapshots and later restores the complete `ExecutionPlan` object, so its existing ID is retained across the nested call.

One `ModelTraceContext` is created before the semantic plan-quality retry loop. It owns a UUID `retrySequenceId`; the provider-attempt advisor generates a new UUID `attemptId` for each physical or semantic attempt and returns the successful attempt map through the model response context. `PlanningAttemptResult` carries that map alongside the parsed plan. Validation, retry, evidence-coverage, and quality-warning records merge the attempt map into their metadata. At the acceptance point, however, `DefaultPlanningService` stores the plan and calls `logPlanCreated` with only the plan, so the accepting attempt map is not forwarded to `PLAN_CREATED`.

`DefaultExecutionTraceRecorder` places both `PLAN_CREATED` and `PLAN_UPDATED` on a selected active frame and records only `planId` as typed metadata. The full `ExecutionPlan` is also serialized as the record `data`, so metadata and data currently derive their `planId` from the same model-authored object. Frame selection scans the active frame stack for the first `PLANNING` frame, then the first `ROOT_MISSION`, then the first non-`MODEL_CALL` frame, with the active frame as the final fallback.

The affected Java declarations are all under `internal` packages and are explicitly classified by the architecture test as technically public internal types, not Application API or Supported SPI. The auto-configuration creates infrastructure beans for `ExecutionStateService` and `PlanningService` without `@ConditionalOnMissingBean`. The plan prompt/response shape is internal model protocol, while canonical plan trace records are an Ephemeral diagnostic format. No `loomspan.*` property, YAML skill manifest field, or supported application-facing Java type defines plan identity.

The current Console is an in-repository diagnostic consumer. Its Go parser retains record metadata and data as opaque `json.RawMessage`, while the browser plan view parses `data.planId` and searches prior `PLAN_CREATED`/`PLAN_UPDATED` records for an equal value. Consequently, additional `PLAN_CREATED` metadata fields pass through ingestion, and plan comparison behavior currently depends on stable plan IDs in record data. No checked-in Console fixture JSON/NDJSON contains plan records; the browser plan tests construct plan records in TypeScript.

## Detailed Findings

### Planning prompt and parse boundary

- `DefaultPlanningService.buildPlanningPrompt` defines the exact model response shape and includes `"planId": "<unique string>"` (`loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/runtime/planning/DefaultPlanningService.java:476`, `:523`).
- Each response is passed to `parsePlan`, which unwraps a fence, parses JSON with YAML fallback, normalizes plan/task status spellings, and converts the tree directly to `ExecutionPlan` (`DefaultPlanningService.java:609-622`, `:643-702`). There is no separate proposal type or identity-normalization step.
- `LoomspanJacksonCodecs` builds both planning mappers with `FAIL_ON_UNKNOWN_PROPERTIES` enabled (`loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/serialization/LoomspanJacksonCodecs.java:34-37`). Thus an unsolicited top-level field that is not an `ExecutionPlan` component is rejected under the current policy, while `planId` is a recognized and required component.
- Current tests demonstrate YAML coercion of numeric `planId` to string and assert the model value `"12345"` becomes the runtime ID (`loomspan-spring-boot-starter/src/test/java/com/lokiscale/loomspan/internal/runtime/planning/PlanningServiceTest.java:126-143`). Separate tests assert that unknown JSON and YAML fields fail parsing (`PlanningServiceTest.java:147-168`).

### Runtime identity and immutable plan copies

- `ExecutionPlan` is an immutable record whose compact constructor rejects null or blank `planId` (`loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/core/ExecutionPlan.java:12-27`).
- `updateTask`, `withActiveTask`, `clearActiveTask`, and `withStatus` all construct or delegate to a new `ExecutionPlan` using the current `planId` (`ExecutionPlan.java:35-47`, `:67-79`). No other production `ExecutionPlan` copy methods exist.
- Task lifecycle methods in `DefaultPlanningService` use those copy methods, store the new plan, and emit `PLAN_UPDATED` (`DefaultPlanningService.java:158-264`). This is the production path by which task start, completion, and failure preserve plan identity.
- `DefaultExecutionStateService.snapshotPlan` places the current immutable plan object in a `PlanSnapshot`; `restorePlan` reinstalls that object or clears the single session slot (`loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/runtime/state/DefaultExecutionStateService.java:167-186`).
- `CapabilityExecutionRouter` wraps an unmapped nested YAML-skill invocation with snapshot/restore in `try/finally` (`loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/core/CapabilityExecutionRouter.java:74-86`). `ExecutionStateServiceTest` verifies restoration of a parent plan and clearing when no parent existed (`loomspan-spring-boot-starter/src/test/java/com/lokiscale/loomspan/internal/runtime/state/ExecutionStateServiceTest.java:100-117`).

### Attempt and retry lineage available at acceptance

- `initializePlanWithQualityChecks` creates one `ModelTraceContext` before entering its `while (true)` semantic-retry loop (`DefaultPlanningService.java:267-287`). The same context is passed into every `requestPlanAttempt` call.
- `ModelTraceContext` generates its `retrySequenceId` once with `UUID.randomUUID()` and generates a fresh `attemptId` in each `nextAttempt` call. Its attempt map contains `retrySequenceId`, `attemptId`, `attemptNumber`, `attemptReason`, and `providerAttemptNumber` (`loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/core/ModelTraceContext.java:24-37`, `:60-70`, `:116-123`).
- `ProviderAttemptCallAdvisor` calls `nextAttempt` for each provider attempt, uses the attempt map on model request/response trace records, and attaches the successful map to response context under `RESPONSE_ATTEMPT_CONTEXT_KEY` (`loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/chat/ProviderAttemptCallAdvisor.java:47-94`).
- `requestPlanAttempt` extracts that response-context map and stores it in `PlanningAttemptResult` next to the parsed plan (`DefaultPlanningService.java:383-425`, `:798-803`).
- Plan validation, retry request, evidence-coverage, and warning methods add the full attempt map to their metadata (`DefaultPlanningService.java:305-346`, `:354-379`, `:448-469`). This makes their existing relationship to model attempts explicit.
- The accepted branch stores the plan and calls `executionStateService.logPlanCreated(session, attemptResult.plan())`; it does not pass `attemptResult.modelAttempt()` (`DefaultPlanningService.java:337-351`). `ExecutionStateService`, `ExecutionTraceRecorder`, and both implementations currently define plan-created methods with only `(session, plan)` (`loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/runtime/state/ExecutionStateService.java:46`, `DefaultExecutionStateService.java:207-211`, `loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/core/ExecutionTraceRecorder.java:27`).

### Acceptance and rejection behavior

- The service allows one semantic plan-quality retry (`MAX_PLAN_QUALITY_RETRIES = 1`) and uses the same retry loop for ordinary quality errors and deterministic evidence-coverage gaps (`DefaultPlanningService.java:56`, `:304-325`).
- If deterministic evidence coverage is still incomplete after the retry allowance, the service records a final `PLAN_VALIDATION_FAILED` event and throws before storing or logging a plan (`DefaultPlanningService.java:328-335`). `PlanningServiceTest` asserts that the session plan remains empty in this case (`PlanningServiceTest.java:653-669`).
- If ordinary quality errors remain after the retry allowance, they are recorded as `PLAN_QUALITY_WARNING`, after which the plan is stored and `PLAN_CREATED` is emitted (`DefaultPlanningService.java:337-351`). The focused planning and trace contract tests verify the single retry, warning records, and planning-frame placement (`PlanningServiceTest.java:523-568`; `loomspan-spring-boot-starter/src/test/java/com/lokiscale/loomspan/internal/runtime/trace/ExecutionTraceContractTest.java:198-229`).
- A rejected first proposal is parsed into an in-memory `ExecutionPlan` and identified in validation/retry records by its attempt metadata, but the only `PLAN_CREATED` call occurs after the loop reaches an accepted branch. No plan record is emitted from the retry branch (`DefaultPlanningService.java:287-351`).

### Trace creation, attachment, and serialization

- `DefaultExecutionTraceRecorder.recordPlanCreated` and `recordPlanUpdated` create metadata containing only `planId` and pass the full plan as payload (`loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/core/DefaultExecutionTraceRecorder.java:73-85`).
- `recordOnPlanFrame` selects the first active `PLANNING` frame, otherwise the first `ROOT_MISSION`, otherwise the first non-`MODEL_CALL` frame; if none is found, normal active-frame recording is used (`DefaultExecutionTraceRecorder.java:167-190`). `ExecutionTraceContractTest` asserts creation is attached to the planning frame rather than its nested model frame (`ExecutionTraceContractTest.java:150-169`).
- `DefaultExecutionTraceHandle` converts the payload object to `JsonNode`; an ordinary record retains that node, while a large payload becomes a chunked envelope plus payload chunks (`loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/runtime/trace/DefaultExecutionTraceHandle.java:443-499`, `:599-610`). `NdjsonTraceRecordWriter` writes each physical `TraceRecord` as one JSON line (`loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/runtime/trace/NdjsonTraceRecordWriter.java:32-44`).
- Because both metadata and data are derived from the same `ExecutionPlan` argument today, a normal `PLAN_CREATED` or `PLAN_UPDATED` record carries the same model-authored value at `metadata.planId` and `data.planId`. Current tests exercise the record type and frame placement but do not directly assert metadata/data agreement or accepted-attempt fields.
- `ExecutionJournalProjector` maps both plan record types to developer-facing journal entries using sanitized `record.data()`; it does not use plan metadata (`loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/runtime/trace/ExecutionJournalProjector.java:70-81`). `LiveActivityProjector` recognizes both types as visible planning activity and assigns fixed summaries (`loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/runtime/observation/LiveActivityProjector.java:311-358`).

### In-repository Console consumer

- The Go trace-analysis model intentionally retains metadata and data as opaque `json.RawMessage` (`loomspan-console/internal/traceanalysis/model.go:23-56`), and the parser copies both values verbatim after validating the outer record (`loomspan-console/internal/traceanalysis/parser.go:239-249`). Added keys inside a valid metadata object do not require a Go struct field.
- The browser converts record data into a `PlanSnapshot` and reads optional `planId` from the plan object (`loomspan-console/web/src/observability/planComparison.ts:7-53`).
- When displaying a `PLAN_UPDATED` record, it queries earlier `PLAN_CREATED` and `PLAN_UPDATED` records and selects the latest earlier snapshot whose `data.planId` equals the update's ID (`loomspan-console/web/src/observability/TraceRecords.tsx:394-418`, `:976-1001`). The search is not restricted to one frame.
- The Console record DTO exposes record descriptors rather than typed plan metadata (`loomspan-console/web/src/api/contracts.ts:278-283`). Current typed attempt/retry DTOs are separate query projections (`contracts.ts:286-303`).
- Repository search found browser unit tests that construct plan records and plan snapshots, but no checked-in `.json` or `.ndjson` fixture under `loomspan-console-fixtures`, `loomspan-console`, or starter test resources containing `PLAN_CREATED`, `PLAN_UPDATED`, or `planId`.

### Tests and verified usage inventory

- Planning tests currently rely broadly on authored plan IDs in helper JSON and Java `ExecutionPlan` instances (`PlanningServiceTest.java:55`, `:698`, `:745`, `:792`, `:922`).
- `ExecutionPlanTest` verifies task immutability and selective task updating, but does not assert ID preservation across all copy methods (`loomspan-spring-boot-starter/src/test/java/com/lokiscale/loomspan/internal/core/ExecutionPlanTest.java:13-45`).
- `ExecutionStateServiceTest` exercises the plan-created boundary and nested plan restoration (`ExecutionStateServiceTest.java:50-117`). Its test recorder implements the current two-argument `recordPlanCreated` and `recordPlanUpdated` methods (`ExecutionStateServiceTest.java:304-323`).
- `ExecutionTraceContractTest` covers planning-frame ownership and quality/retry event placement (`ExecutionTraceContractTest.java:150-229`).
- `ExecutionJournalProjectionContractTest` supplies a representative `PLAN_CREATED` with `metadata.planId` and full plan data, and verifies that it projects to `JournalEntryType.PLAN_CREATED` (`loomspan-spring-boot-starter/src/test/java/com/lokiscale/loomspan/internal/runtime/trace/ExecutionJournalProjectionContractTest.java:33-115`).
- Additional verified consumers are `ExecutionJournalProjector`, `LiveActivityProjector`, session JSON/journal tests, coordinator tests, browser activity presentation, and browser plan rendering/comparison. None currently treats `attemptId` or `retrySequenceId` on `PLAN_CREATED` as a typed plan fact.

### Configuration, beans, documentation, and public declarations

- `LoomspanAutoConfiguration` declares `ExecutionStateService` and `PlanningService` as infrastructure beans and constructs their default implementations directly (`loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/autoconfigure/LoomspanAutoConfiguration.java:315-330`). There is no `@ConditionalOnMissingBean` on these or other beans in the class.
- `DefaultExecutionStateService` constructs `DefaultExecutionTraceRecorder` internally unless a recorder is supplied through its public constructor (`DefaultExecutionStateService.java:40-54`). The recorder is not independently declared as a Spring bean.
- `ExecutionPlan`, `ModelTraceContext`, `ExecutionTraceRecorder`, `DefaultExecutionTraceRecorder`, `PlanningService`, `DefaultPlanningService`, `ExecutionStateService`, `DefaultExecutionStateService`, and `PlanSnapshot` are public declarations under `com.lokiscale.loomspan.internal`. The closed architecture allowlist explicitly labels these as technically public internal types used for cross-package collaboration (`loomspan-spring-boot-starter/src/test/java/com/lokiscale/loomspan/architecture/LoomspanPublicSurfaceArchitectureTest.java:48-196`).
- Repository documentation outside `ai/thoughts/` does not describe `ExecutionPlan`, `planId`, `PLAN_CREATED`, or `PLAN_UPDATED`. No configuration property or YAML skill manifest key controls plan identity or the plan-created relationship fields.

## Contract Classification

| Surface | Current classification | Evidence and consumers |
| --- | --- | --- |
| Supported application-facing Java API | No affected type | The closed API allowlist contains only top-level `com.lokiscale.loomspan.api` types; all affected types are listed as technically public internal implementation. |
| Supported SPI | None | The interfaces and constructors are public for internal cross-package composition. Auto-configuration supplies infrastructure beans without a documented replacement contract or `@ConditionalOnMissingBean`. |
| Configuration and manifest contracts | No affected field | No `loomspan.*` property or YAML skill syntax exposes `planId`, plan-record metadata, or ID generation. |
| Persisted or serialized contracts | No cross-version plan contract identified | The trace can be saved as a same-`consoleCompatibilityVersion` diagnostic artifact, but current repository policy does not classify plan records as cross-version archival interchange. |
| Ephemeral diagnostic formats | `PLAN_CREATED`, `PLAN_UPDATED`, their metadata, and serialized `ExecutionPlan` data | Produced by the starter, projected into the journal/live activity, parsed by Console, and used by the browser plan comparison view. |
| Internal or accidentally exposed implementation | Planning prompt/parser, `ExecutionPlan`, attempt context, state service, recorder interfaces/constructors, plan snapshot | Explicit architecture-test classification and internal package location. |

### Protected protocol consumers and observable semantics

- The Java canonical trace writer and reader consume the record envelope and serialized plan data.
- The Java journal projection consumes plan `data` and preserves both plan record types as journal entries.
- The Java live activity projection consumes record types and exposes plan-created/updated activity.
- Console Go acquisition/analysis validates the outer NDJSON structure and retains plan metadata/data opaquely.
- Console browser plan rendering consumes full plan `data`; plan evolution comparison joins versions by `data.planId` across frames.
- Observable semantics in current tests include strict plan parsing, nonblank plan IDs, preservation through immutable copies, planning-frame attachment for creation, root/fallback attachment for later records when planning is no longer active, valid NDJSON serialization, and recognition by journal/live/UI projections.

## Architecture Documentation

The current producer flow is:

```text
planning prompt (model authors planId)
  -> ModelTraceContext (one retrySequenceId for the episode)
  -> provider advisor (one attemptId per attempt)
  -> model response context
  -> parse directly into ExecutionPlan
  -> quality/evidence validation records include attempt metadata
  -> accepted plan stored in session's single plan slot
  -> PLAN_CREATED(planId only in typed metadata; full plan in data)
  -> immutable task/status copies preserve planId
  -> PLAN_UPDATED(planId only in typed metadata; full plan in data)
```

The plan's object identity is not used as its chain key; the `planId` string is copied into every immutable version. Frame attachment supplies structural scope independently of that string. The accepted attempt is present in `PlanningAttemptResult` at the store/log call site, but the current logging interfaces do not accept it.

## Historical Context (from `ai/thoughts/` and Git)

- `ai/thoughts/framework-feature-design-lens.md` classifies traces as Ephemeral diagnostic formats and distinguishes public visibility from supported API/SPI status. It also identifies framework-owned identity and trusted execution metadata as runtime responsibilities.
- `ai/thoughts/phases/2026-08-15-loomspan-llm-trace-understanding-roadmap.md` records plan identity and accepted-attempt lineage as a producer prerequisite for later MCP and runtime-debugging skill work. It describes frame lineage as the way to select the relevant creation record and `planId` as the intended cross-frame chain identity.
- `ai/thoughts/phases/loomspan_llm_trace_understanding_workflows.md` defines the intended plan-evolution evidence path and a future representative fixture containing primary/nested plans, rejected attempts, cross-frame updates, and distinct framework IDs.
- Git blame shows `ExecutionPlan.planId` and its copy behavior originated on 2026-03-20 (`f20eec2d`); plan metadata and `recordOnPlanFrame` originated on 2026-03-25 (`9630979d`); semantic/provider attempt lineage was integrated into planning records on 2026-07-24 (`98799ce9`). The current acceptance call still follows the earlier two-argument plan-created path.

## Related Research

No prior document exists in `ai/thoughts/research/` for this topic. Related current design artifacts are:

- `ai/thoughts/tickets/loomspan-framework-pr-27-plan-identity-and-lineage.md`
- `ai/thoughts/phases/2026-08-15-loomspan-llm-trace-understanding-roadmap.md`
- `ai/thoughts/phases/loomspan_llm_trace_understanding_workflows.md`
- `ai/thoughts/framework-feature-design-lens.md`

## Open Questions

- No unresolved product or identity semantics remain after the ticket clarification recorded in the 23:28 follow-up below.
- Implementation still has to realize and verify the selected internal supplier wiring, required attempt-context failure behavior, attempt-aware focused test doubles, and combined Java producer trace-contract scenario. These are implementation and coverage tasks with ticket-defined outcomes, not open design questions.

## Follow-up Research 2026-08-15 23:22:24 PDT

### Clarification requested

The question was whether the intended work keeps `planId` and transfers its management from the LLM to the framework. The answer is yes. Reading all three documents in `ai/thoughts/phases/` confirms a consistent target model:

```text
model response/proposal (does not own planId)
  -> validation and acceptance decision
  -> framework mints the accepted plan's planId
  -> accepted ExecutionPlan carries that planId
  -> PLAN_CREATED metadata and data carry that same planId
  -> every PLAN_UPDATED version preserves that same planId
```

A rejected proposal remains model-response content associated with its `attemptId`; it does not become recorded plan state and has no recorded plan identity.

### Evidence from the phase documents

- The roadmap says, without introducing a replacement identity, that “the existing `planId` becomes a framework-generated identity for an accepted plan and all of its updates,” that the model no longer controls it, and that no second `planInstanceId` or plan-chain identity is added (`ai/thoughts/phases/2026-08-15-loomspan-llm-trace-understanding-roadmap.md:471-478`).
- The roadmap assigns two different roles to structure and identity: mission/frame lineage selects the relevant `PLAN_CREATED`, then the framework-owned `planId` establishes membership across later updates and frame transitions (`2026-08-15-loomspan-llm-trace-understanding-roadmap.md:480-486`).
- The workflow catalog calls `planId` a framework-generated identity minted for one recorded accepted plan and preserved across every `PLAN_UPDATED` (`ai/thoughts/phases/loomspan_llm_trace_understanding_workflows.md:250-260`).
- The catalog explicitly states that the target producer prompt does not ask the model for `planId`; rejected proposals use `attemptId` and receive no recorded plan identity (`loomspan_llm_trace_understanding_workflows.md:272-280`).
- Requirement `LLM-WF-PL-R7` states that the producer owns `planId`, the model cannot choose or override it, and updates preserve the accepted plan's value (`loomspan_llm_trace_understanding_workflows.md:319-327`).
- The planned representative fixture must prove distinct framework-generated IDs even when model proposal content is identical or contains the same unsolicited candidate ID (`loomspan_llm_trace_understanding_workflows.md:937-947`; `2026-08-15-loomspan-llm-trace-understanding-roadmap.md:631-638`). This treats an unsolicited model value as adversarial proposal content, not as an alternative identity source.
- `ai/thoughts/phases/loomspan_console_workflows.md` defines the existing browser/Console workflow and evidence boundaries but contains no competing plan identity, plan SPI, or model-owned plan-ID contract.

### Meaning of the parser choice

The ticket's “ignore or reject” language concerns only an unsolicited input property from the model:

- **Ignore** means discard or overwrite the model-supplied candidate value and materialize the accepted `ExecutionPlan` with the framework-generated value.
- **Reject** means treat the unsolicited property as invalid proposal shape under the selected parsing contract.

Both behaviors would retain `ExecutionPlan.planId`, result in the framework owning the accepted value, and prevent the model from choosing or overriding recorded identity. The phase documents do not select between those parser behaviors. The ticket has now resolved that discretion by selecting discard/overwrite for `planId` while retaining strict rejection for every other unknown field.

## Follow-up Research 2026-08-15 23:24:21 PDT

### Remaining-question disposition

There are no other unresolved product or identity semantics in the researched scope. One defensive implementation discretion and two implementation/coverage mechanics remain:

1. **Unsolicited model field:** ignoring/overwriting versus rejecting a model-supplied `planId` is not a normal-path product choice. It is permitted defensive handling for output that violates the new prompt contract; either implementation must prove the model value cannot become recorded identity.
2. **Attempt-aware test setup:** focused planning tests need a way to return the already-defined production attempt context so assertions can join the accepted plan to its attempt and retry sequence. This does not require a new identifier or production semantic.
3. **Representative trace fixture:** the combined nested/sequential-plan and rejection topology does not exist in current fixtures. Its expected relationships are already defined, so fixture construction does not require another design decision.

The following matters are settled by the ticket or phase documents and do not remain open:

- keep one identity named `planId`;
- generate it in the framework for accepted recorded plan state;
- preserve it in `ExecutionPlan`, `PLAN_CREATED`, and every `PLAN_UPDATED`;
- use frame lineage to select the relevant creation record, but not as chain identity;
- put accepting `attemptId` and `retrySequenceId` on `PLAN_CREATED`, but not on every update;
- emit no `PLAN_CREATED` for rejected proposals or deterministic evidence-coverage failure;
- continue accepting exhausted ordinary quality errors with warning evidence;
- retain the current single-plan slot and nested snapshot/restore model;
- make no Application API, Supported SPI, configuration, manifest, MCP, skill, or frame-placement redesign in this ticket; and
- add no compatibility shim for the internal Java call-chain or pre-release ephemeral trace revision.

ID allocation timing and the exact supplier injection point remain ordinary internal implementation choices because the ticket already fixes their observable constraints: the recorded accepted plan must receive a nonblank collision-resistant framework ID, independently accepted plans must differ, and the model must never select or override it.

## Follow-up Research 2026-08-15 23:26:06 PDT

### Why model output might still contain `planId`

The ticket does remove `planId` from the planning prompt. A model following the new prompt will not return it. The ticket mentions a supplied value only to define defensive behavior for nonconforming output, such as:

- a model reproducing the prior response shape learned from earlier context;
- a stale recorded response or test fixture passed through the new parser;
- a model adding an unrequested identity-like field; or
- any other response that violates the requested structure.

This case exists to prove the trust boundary, not because the framework continues asking for the field. The ordinary path is:

```text
prompt omits planId
  -> model returns plan content without planId
  -> framework accepts the proposal
  -> framework supplies planId
  -> accepted ExecutionPlan and trace records carry that framework value
```

The ticket now selects discard/overwrite so implementation and review use one behavior: an unsolicited `planId` does not by itself reject an otherwise valid proposal, every other unknown field retains strict rejection, and tests establish that model output never chooses or overrides the accepted plan's ID.

## Follow-up Research 2026-08-15 23:28:28 PDT

### Ticket clarification for implementation and review

`ai/thoughts/tickets/loomspan-framework-pr-27-plan-identity-and-lineage.md` now records these decisions explicitly:

1. The prompt omits `planId`; the existing `ExecutionPlan.planId` remains and is populated by the framework.
2. An unsolicited model `planId` is discarded/overwritten rather than causing rejection by itself. All other unknown fields retain the strict planning-codec behavior. The unsolicited value can remain only as untrusted raw model-response evidence and is never copied into normalized plan identity.
3. `DefaultPlanningService` uses an internal `Supplier<String>` that defaults to UUID generation; focused tests inject deterministic distinct values. This supplier is not a Spring bean, configuration property, supported API, or SPI.
4. The accepting response must carry the existing valid `attemptId` and `retrySequenceId`. Missing or invalid context fails before plan storage and `PLAN_CREATED`; the planning service does not generate fallback lineage IDs.
5. Focused planning test doubles supply production-shaped attempt context.
6. This producer ticket adds a combined Java trace-contract scenario for primary/nested or sequential plans, identical proposal content, rejection, cross-frame updates, and accepting-attempt lineage. Console fixture changes remain with the later MCP/skill work.

The ticket's acceptance criteria and guardrails now repeat the externally observable portions of these decisions so an alternate implementation cannot silently change them during review.
