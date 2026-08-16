# PR 27 - Framework-Owned Plan Identity and Accepted-Attempt Lineage Implementation Plan

## Overview

Move ownership of the existing `planId` from planning-model output to the Loomspan runtime, preserve that framework-generated identity through every recorded plan version, and add the accepting model attempt and retry-sequence identifiers to `PLAN_CREATED`. The change stays inside internal Java implementation and the current-version trace diagnostic contract so later Console MCP and `loomspan-runtime-debugging` work can correlate plan evolution without treating frame placement, record order, or model-authored values as identity.

## Current State Analysis

`DefaultPlanningService` currently asks the model for `planId`, normalizes the returned JSON/YAML tree, and binds it directly to `ExecutionPlan` (`loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/runtime/planning/DefaultPlanningService.java:476-543`, `:609-670`). The strict planning mappers reject unknown properties, but `planId` is presently a recognized required record component. `ExecutionPlan` rejects blank IDs and already preserves its ID through `updateTask`, `withActiveTask`, `clearActiveTask`, and `withStatus` (`loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/core/ExecutionPlan.java:12-47`, `:67-79`).

One `ModelTraceContext` is created before the planning quality retry loop, so all semantic/provider attempts in that planning episode share one `retrySequenceId`. The provider advisor places the successful attempt map in the model result context, and `PlanningAttemptResult` retains it beside the parsed plan (`DefaultPlanningService.java:267-287`, `:383-425`, `:798-803`). Validation, retry, evidence-coverage, and warning records merge that attempt metadata, but the accepted branch discards it when calling `logPlanCreated`; the plan is stored immediately before the current two-argument call (`DefaultPlanningService.java:304-351`).

The plan-created call then crosses `ExecutionStateService`, `DefaultExecutionStateService`, `ExecutionTraceRecorder`, and `DefaultExecutionTraceRecorder`. The recorder emits only `planId` in typed metadata and serializes the complete `ExecutionPlan` as record data (`loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/core/ExecutionTraceRecorder.java:27-29`, `loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/core/DefaultExecutionTraceRecorder.java:73-85`, `loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/runtime/state/ExecutionStateService.java:46-48`, `loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/runtime/state/DefaultExecutionStateService.java:207-217`). `recordOnPlanFrame` independently chooses a planning frame, root mission, non-model frame, or active-frame fallback (`DefaultExecutionTraceRecorder.java:167-190`). That placement remains structural evidence and is not changed by this work.

The Go Console parser retains metadata and data opaquely, while the browser's plan comparison reads `data.planId` and searches prior plan records for the same value (`loomspan-console/internal/traceanalysis/model.go:23-56`, `loomspan-console/internal/traceanalysis/parser.go:239-249`, `loomspan-console/web/src/observability/planComparison.ts:7-53`, `loomspan-console/web/src/observability/TraceRecords.tsx:394-418`). Adding current-version `PLAN_CREATED` metadata fields therefore needs no Console model or parser change, and stable normalized `data.planId` preserves the existing browser join.

## Desired End State

The planning prompt describes model-authored plan content without `planId`. For every parsed proposal, `DefaultPlanningService` obtains a nonblank value from an internal UUID-backed `Supplier<String>`, removes any unsolicited model `planId`, inserts the framework value, and then binds the normalized tree through the existing strict mapper. Thus an unsolicited legacy ID is harmless but cannot become trusted identity, while every other unknown property remains an error.

A rejected proposal may consume an internal candidate ID because `ExecutionPlan` requires one for validation, but it never becomes session plan state and never emits `PLAN_CREATED`. At the acceptance boundary, the service requires the successful response's existing valid attempt context before changing session state. It then stores the accepted plan and emits exactly one `PLAN_CREATED` whose metadata contains the normalized `planId`, accepting `attemptId`, and planning `retrySequenceId`; its data contains the same `planId`. Every later `PLAN_UPDATED` continues to carry only the same `planId` in metadata and normalized plan data.

Verification must establish that independently accepted plans receive distinct IDs even for identical or adversarial model content; immutable copies and nested snapshot/restore retain the appropriate ID; rejected and deterministic evidence-failure proposals emit no created record; ordinary exhausted quality errors remain warning-backed acceptance; missing or invalid acceptance context fails before storage; and a combined trace can be reconstructed solely from framework identifiers across planning and root/nested frame placement.

### Key Discoveries

- `ModelTraceContext.attemptFrom` validates a nonempty production-shaped attempt map but deliberately returns an empty map when the response context or key is absent (`loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/core/ModelTraceContext.java:97-136`). Acceptance therefore needs an explicit required-context check rather than fallback generation.
- The production provider advisor already generates and returns the authoritative attempt map (`loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/chat/ProviderAttemptCallAdvisor.java:47-94`); the planning service must consume it, not mint replacement lineage.
- Nested YAML execution snapshots and restores the whole immutable `ExecutionPlan`, so no new nested-plan identity mechanism is required (`loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/runtime/state/DefaultExecutionStateService.java:167-186`, `loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/core/CapabilityExecutionRouter.java:74-86`).
- `ExecutionJournalProjector` consumes plan record data, and `LiveActivityProjector` already permits `planId`, `attemptId`, and `retrySequenceId` metadata keys while recognizing both plan record types (`loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/runtime/trace/ExecutionJournalProjector.java:70-81`, `loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/runtime/observation/LiveActivityProjector.java:26-41`, `:311-358`).
- The affected public Java declarations are explicitly allowlisted as technically public internal collaboration types, not supported application API or SPI (`loomspan-spring-boot-starter/src/test/java/com/lokiscale/loomspan/architecture/LoomspanPublicSurfaceArchitectureTest.java:48-196`).

## What We're NOT Doing

- Adding `planInstanceId`, `planChainId`, a model-authored identity archive, or any second plan identity.
- Changing plan frame-selection or attachment behavior, route semantics, the session's single-plan slot, nested snapshot/restore, or concurrency behavior.
- Copying accepting-attempt fields onto `PLAN_UPDATED` or inferring that a retry caused a validation issue to be fixed.
- Generating fallback `attemptId` or `retrySequenceId` values when provider attempt context is missing or malformed.
- Adding application API, supported SPI, Spring bean override, configuration property, YAML manifest syntax, or a configurable ID generator.
- Adding MCP typed facts, Console projections, Console fixture-corpus changes, Agent Skill navigation, authoring guidance, compatibility adapters, or historical trace readers.
- Changing quality-warning acceptance or deterministic evidence-coverage rejection semantics.

## Skill-Authoring Documentation Impact

**Impact**: No impact

- **Rationale**: The change is a producer-owned identity and lineage correction inside the ephemeral diagnostic format. It does not change YAML manifest syntax, skill planning instructions, author-visible defaults, validation requirements, execution semantics, or a currently documented MCP/Agent Skill operation. The ticket deliberately reserves plan-navigation guidance and representative Console fixtures for the later MCP/`loomspan-runtime-debugging` work.
- **Documents to update**: None.
- **Supporting evidence**: `ai/skill-authoring/README.md` classifies planning coverage as foundational and trace guidance as source-verified; `ai/skill-authoring/traces-and-debugging.md` documents attempt/retry identity and current-version trace limitations but does not define plan-chain navigation. Producer behavior will be protected by `PlanningServiceTest`, `ExecutionPlanTest`, `ExecutionStateServiceTest`, and `ExecutionTraceContractTest`, plus the unchanged journal/live projectors.
- **Coverage table update**: Not required. No topic is added, no routed authoring task changes, and the coverage/confidence of existing author-facing guidance does not change.
- **LLM-first usability**: Not applicable. No authoring document changes are planned; future plan-navigation guidance remains a separately routed, evidence-backed update after MCP exposes the producer facts.

## Contract and Compatibility Impact

| Surface | Classification and supporting evidence | Planned compatibility treatment |
| --- | --- | --- |
| Application API | No affected application-facing type. The closed API is the allowlisted top-level surface under `com.lokiscale.loomspan.api`; all changed Java types are under `internal`. | Preserve the allowlisted API exactly; add no allowlist entry. |
| Supported SPI | No supported SPI exists for planning, state, trace recording, or plan-ID generation. Public internal interfaces/constructors and Spring infrastructure beans are technical composition seams only. | Atomically change internal call signatures and test doubles; do not add overloads, replacement beans, or an ID-supplier SPI. |
| Configuration and manifest contracts | No `loomspan.*` property or YAML manifest field controls `planId`, attempt linkage, or ID generation. The planning model prompt is an internal model protocol rather than skill manifest syntax. | Preserve all developer and skill-author configuration. Do not expose the supplier through configuration or Spring. |
| Persisted or serialized contracts | No durable or cross-version plan format is promised. A saved canonical trace is readable only under the exact current `consoleCompatibilityVersion`. | No migration or legacy reader. Keep current writer/reader coherence and reject unsupported historical assumptions. |
| Ephemeral diagnostic formats | `PLAN_CREATED` and `PLAN_UPDATED` metadata/data are current-version diagnostic facts consumed by Java projectors and Console. `PLAN_CREATED` gains trusted accepting-attempt fields and both record types move from model-authored to framework-authored `planId`. | Make the producer, Java projections, opaque Go ingestion, browser plan join, and tests coherent in one repository revision. Preserve data/metadata ID agreement, redaction boundaries, and neutral non-causal semantics. |
| Internal or accidentally exposed implementation | `DefaultPlanningService`, `ExecutionPlan`, `ModelTraceContext`, `ExecutionStateService`, `DefaultExecutionStateService`, `ExecutionTraceRecorder`, `DefaultExecutionTraceRecorder`, constructors, and test helpers are explicitly internal despite public modifiers. | Replace obsolete internal signatures and model-owned identity behavior atomically. Preserve only useful public constructors whose signatures do not need to change; add no compatibility shim. |

- **Evidence of supported contracts**: The architecture allowlists and README define the small Application API; neither lists planning/state/trace internals as supported API or SPI. The framework design lens classifies traces as current-version ephemeral diagnostics. Existing Console parsing and browser plan comparison are verified in-repository consumers that must remain coherent, not evidence of a cross-version promise.
- **Intended breaks**: The internal planning response shape no longer requires model `planId`; model-supplied values are ignored. The internal plan-created state/recorder method signatures gain accepting-attempt context. The current-version `PLAN_CREATED` metadata shape gains `attemptId` and `retrySequenceId`, and `planId` semantics become framework-owned.
- **In-repository consumers to update**: Production constructors/callers in `DefaultPlanningService`, `ExecutionStateService`, `DefaultExecutionStateService`, `ExecutionTraceRecorder`, and `DefaultExecutionTraceRecorder`; planning/state/trace test doubles and fixtures; `ExecutionPlanTest`; `PlanningServiceTest`; `ExecutionStateServiceTest`; `ExecutionTraceContractTest`; any direct `logPlanCreated` caller such as step-loop test scaffolding; architecture and journal projection verification. Console production code and fixture corpora require verification but no planned edit.
- **Public-surface delta**: No type, constructor, method, Spring bean, configuration key, or extension point is added to supported Java API/SPI. Technically public internal interface method signatures change atomically; existing public `DefaultPlanningService` construction remains available and continues to select a UUID-backed supplier internally.
- **Shim decision**: **No shim.** No supported contract protects the old model-owned ID or two-argument internal recording chain, and the repository is pre-1.0. Keeping overloads or dual record behavior would preserve precisely the ambiguous lineage this ticket removes.
- **Java-to-Go boundary coordination**: **Not required.** The consumed NDJSON envelope does not change, and Go retains record metadata/data as opaque JSON. Additional metadata keys and normalized `data.planId` remain valid under current parsing. The Console browser already joins plan versions through `data.planId`; Java contract coverage and existing Console tests verify coherence, while new cross-language representative fixtures remain explicitly out of scope.

## Implementation Approach

Keep `ExecutionPlan` as the sole normalized plan representation. Introduce the framework ID at the narrow parse-normalization boundary, before strict binding, so no temporary sentinel or additional proposal identity can escape. Use a package-private injectable supplier for deterministic focused tests while keeping existing production constructors and auto-configuration behavior unchanged.

Treat accepted-attempt context as a precondition of the acceptance transaction. Validate and reduce the already-returned attempt map to the two relationship fields before `storePlan`, then pass those fields through the internal state and trace-recorder chain. The recorder constructs `PLAN_CREATED` metadata from the plan ID plus those fields; updates remain plan-ID-only. Do not change `ModelTraceContext` generation or frame selection.

Tests should first encode the new trust and transaction boundaries, then update shared/local planning model doubles to return production-shaped response context. The combined Java trace-contract scenario should exercise a primary mission plus a nested same-skill mission with competing planning frames, one rejected proposal retained only as attempt-linked model-response evidence, identical/adversarial proposal content, multiple cross-frame updates, explicit acceptance joins, and final-state selection by the highest canonical sequence within the selected `planId` chain.

## Phase 1: Establish Framework-Owned Plan Identity

### Overview

Remove plan identity from the model contract and normalize every proposal into an `ExecutionPlan` with a framework-supplied, validated ID while retaining strict treatment of all unrelated unknown fields.

### Changes Required

#### 1. Planning prompt, supplier wiring, and tree normalization

**File**: `loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/runtime/planning/DefaultPlanningService.java`

**Changes**:

- Remove `planId` from the exact JSON structure requested by `buildPlanningPrompt`; leave task IDs model-authored and all other planning constraints unchanged.
- Add an internal `Supplier<String>` field. Existing public constructors delegate to `UUID.randomUUID().toString()` so `LoomspanAutoConfiguration` and ordinary internal callers need no new dependency.
- Extend the package-private focused-test constructor to accept the supplier and require it non-null. Replace the old package-private signature atomically rather than retaining a compatibility overload unless a current in-repository caller proves it necessary.
- During `normalizePlanTree`, remove any existing top-level `planId`, obtain one supplier value for that proposal, require it to be nonblank, and insert it before `treeToValue`. This permits a legacy/unsolicited `planId` while preserving `FAIL_ON_UNKNOWN_PROPERTIES` for every other field.
- Do not store or trace the discarded value outside the already-recorded raw model response.

```java
ObjectNode planNode = requirePlanObject(tree);
planNode.remove("planId");
planNode.put("planId", requireNonBlank(planIdSupplier.get(), "planId"));
```

The exact helper shape may follow current class conventions; the invariant is one trusted supplied value per parsed proposal and no model override path.

#### 2. Identity-focused planning and immutable-copy coverage

**Files**:

- `loomspan-spring-boot-starter/src/test/java/com/lokiscale/loomspan/internal/runtime/planning/PlanningServiceTest.java`
- `loomspan-spring-boot-starter/src/test/java/com/lokiscale/loomspan/internal/core/ExecutionPlanTest.java`

**Changes**:

- Replace assertions that adopt YAML/numeric model IDs with deterministic supplier assertions.
- Add JSON/YAML cases where `planId` is absent and acceptance succeeds.
- Add a legacy/adversarial supplied `planId` case proving it is overwritten without rejecting the proposal, and retain JSON/YAML tests proving other unknown fields still fail.
- Capture the planning system prompt and assert its requested response shape no longer contains `planId` while it still contains `taskId`.
- Inject a deterministic sequence supplier and prove byte-identical independently accepted proposals receive distinct IDs; add null/blank supplier failure coverage.
- Expand `ExecutionPlanTest` to assert ID preservation through `updateTask`, `withActiveTask`, `clearActiveTask`, and `withStatus`.

### Success Criteria

#### Automated Verification

- [x] Focused identity/parser tests pass: `.\mvnw.cmd -pl loomspan-spring-boot-starter "-Dtest=PlanningServiceTest,ExecutionPlanTest" test`
- [x] A response without `planId` accepts a deterministic framework ID in both JSON and YAML paths.
- [x] An unsolicited `planId` is never the normalized ID, while every other unknown property remains rejected.
- [x] Identical accepted responses receive distinct deterministic IDs and all immutable plan copies preserve their original ID.

#### Manual Verification

- [x] Review the emitted planning prompt text and confirm only plan identity—not task identity or other response requirements—was removed from model responsibility.
- [x] Review normalization order and confirm a discarded model value cannot reach normalized session state, typed metadata, or a second retained identity field.

---

## Phase 2: Make Accepted-Attempt Lineage a Required Creation Fact

### Overview

Require the successful response's existing model attempt context before session mutation, then carry only its authoritative relationship fields into `PLAN_CREATED`.

### Changes Required

#### 1. Acceptance-boundary validation and call-chain threading

**Files**:

- `loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/runtime/planning/DefaultPlanningService.java`
- `loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/runtime/state/ExecutionStateService.java`
- `loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/runtime/state/DefaultExecutionStateService.java`
- `loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/core/ExecutionTraceRecorder.java`
- `loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/core/DefaultExecutionTraceRecorder.java`

**Changes**:

- In the accepted branch, validate that `PlanningAttemptResult.modelAttempt()` contains nonblank string `attemptId` and `retrySequenceId` before `storePlan` and before any `PLAN_CREATED` call. Reuse the validation already performed by `ModelTraceContext.attemptFrom` for a present map; add the missing-map precondition without synthesizing IDs.
- Pass the validated attempt relationship through `logPlanCreated` and `recordPlanCreated`; atomically update implementations and in-repository callers/test recorders.
- Have `DefaultExecutionTraceRecorder` build `PLAN_CREATED` metadata containing `planId`, `attemptId`, and `retrySequenceId`. Copy only the two required relationship values from attempt context; do not blindly merge attempt number, reason, provider-attempt number, or arbitrary keys.
- Keep `recordPlanUpdated` and `recordOnPlanFrame` unchanged apart from any shared validation helper needed for consistent nonblank IDs.
- Preserve the existing order and semantics of validation/retry/warning records. Ordinary exhausted quality errors still reach acceptance; deterministic evidence-coverage exhaustion still throws before the acceptance precondition and session mutation.

```java
Map<String, Object> acceptedAttempt = requireAcceptedAttempt(attemptResult.modelAttempt());
executionStateService.storePlan(session, attemptResult.plan());
executionStateService.logPlanCreated(session, attemptResult.plan(), acceptedAttempt);
```

#### 2. Production-shaped focused test interactions

**Files**:

- `loomspan-spring-boot-starter/src/test/java/com/lokiscale/loomspan/internal/runtime/SimpleChatClient.java`
- `loomspan-spring-boot-starter/src/test/java/com/lokiscale/loomspan/internal/runtime/planning/PlanningServiceTest.java`
- `loomspan-spring-boot-starter/src/test/java/com/lokiscale/loomspan/internal/runtime/trace/ExecutionTraceContractTest.java`
- Other planning-specific local model doubles discovered by call-site search, including step-loop/coordinator test helpers when they can return an accepted plan.

**Changes**:

- Return `ModelInteractionResult` context under `ModelTraceContext.RESPONSE_ATTEMPT_CONTEXT_KEY` with the request's trace context and a production-shaped attempt map. For sequence doubles, call `request.traceContext().nextAttempt()` once per response so semantic retries have distinct attempt IDs and one shared retry-sequence ID.
- Retain deliberately context-free doubles only in tests that assert the new failure behavior.
- Avoid weakening production acceptance to accommodate legacy empty test context.

#### 3. State and recorder contract tests

**Files**:

- `loomspan-spring-boot-starter/src/test/java/com/lokiscale/loomspan/internal/runtime/state/ExecutionStateServiceTest.java`
- `loomspan-spring-boot-starter/src/test/java/com/lokiscale/loomspan/internal/runtime/trace/ExecutionJournalProjectionContractTest.java`
- Any direct `logPlanCreated` test callers found by `rg`, including `StepLoopMissionExecutionEngineTest` scaffolding.

**Changes**:

- Update recording test doubles and direct callers for the new internal signature.
- Assert `PLAN_CREATED.metadata.planId == PLAN_CREATED.data.planId`, and assert the exact accepting `attemptId` and `retrySequenceId` values are present.
- Confirm `PLAN_UPDATED` metadata/data agreement and absence of duplicated accepting-attempt fields.
- Keep journal projection assertions over sanitized plan data and demonstrate that the new metadata does not change journal entry type/content behavior.
- Add missing and malformed attempt-context cases proving the plan is not stored, no `PLAN_CREATED` is emitted, and no fallback lineage appears.

### Success Criteria

#### Automated Verification

- [x] Focused acceptance/recording tests pass: `.\mvnw.cmd -pl loomspan-spring-boot-starter "-Dtest=PlanningServiceTest,ExecutionStateServiceTest,ExecutionJournalProjectionContractTest" test`
- [x] Every accepted `PLAN_CREATED` contains matching metadata/data `planId` plus the accepting attempt and retry-sequence IDs.
- [x] Missing or invalid acceptance context fails before session plan storage and emits no `PLAN_CREATED` or synthetic lineage.
- [x] `PLAN_UPDATED` retains matching metadata/data `planId` without accepting-attempt duplication.

#### Manual Verification

- [x] Trace a production-shaped accepted result from `ProviderAttemptCallAdvisor` through planning, state, and recorder code and confirm the original identifiers are reused unchanged.
- [x] Review the acceptance branch ordering and confirm no exception caused by missing lineage can leave an accepted-looking plan in the session without its creation record.

---

## Phase 3: Prove Complete Plan Chains and Repository Coherence

### Overview

Add regression coverage for copy/snapshot behavior, rejection versus warning acceptance, and a representative multi-plan trace topology, then verify the supported public surface and all current producer consumers.

### Changes Required

#### 1. Snapshot/restore and acceptance-semantics regressions

**Files**:

- `loomspan-spring-boot-starter/src/test/java/com/lokiscale/loomspan/internal/runtime/state/ExecutionStateServiceTest.java`
- `loomspan-spring-boot-starter/src/test/java/com/lokiscale/loomspan/internal/runtime/planning/PlanningServiceTest.java`

**Changes**:

- Strengthen nested snapshot/restore assertions so parent and nested accepted plans retain their distinct framework IDs across replacement and restoration.
- Assert a rejected validation proposal remains untrusted `MODEL_RESPONSE_RECEIVED` content linked by its `attemptId`, produces validation/retry evidence, and produces no `PLAN_CREATED` or recorded plan identity.
- Retain and sharpen exhausted ordinary-quality coverage to show warning evidence followed by an identified `PLAN_CREATED`; assert the warning and creation share the accepting `attemptId` and `retrySequenceId`.
- Retain deterministic evidence-coverage exhaustion coverage and assert it leaves the session empty with no `PLAN_CREATED`.

#### 2. Combined Java producer trace-contract scenario

**File**: `loomspan-spring-boot-starter/src/test/java/com/lokiscale/loomspan/internal/runtime/trace/ExecutionTraceContractTest.java`

**Changes**:

- Build one scenario with a primary root mission and a nested invocation of the same skill, each with its own competing planning frame and independently accepted plan. Use identical accepted proposal content and optionally the same unsolicited model `planId`, but inject distinct framework IDs. Select the primary creation through the trace's recorded root and parent-frame lineage, never by route or skill-name uniqueness.
- Include one rejected primary proposal before acceptance as `MODEL_RESPONSE_RECEIVED` content. Reuse the existing `ProviderAttemptCallAdvisor`-backed integration pattern so the response record and returned attempt context come from the same production boundary rather than manually appending a synthetic response around a context-only planning double. Assert rejected and accepting attempts have different `attemptId` values but share the primary planning episode's `retrySequenceId`; validation and retry records join to that sequence, the rejected proposal produces no plan record, and only the accepting attempt joins from `PLAN_CREATED`.
- Emit at least two primary-plan updates after the planning frame has closed so updates land on the primary root-mission frame; also update the nested chain on its corresponding root frame. Assert membership follows `planId`, not frame type, route, skill name, sequence adjacency, or proposal content, then select the highest-sequence member of each chosen chain as its final recorded state.
- Assert each created/updated record has metadata/data ID agreement, independent plans do not collide, and nested snapshot/restore returns the parent plan with its original identity.
- Serialize/read the trace through the existing canonical path to demonstrate valid NDJSON and unchanged journal/live plan-record recognition. Do not add a Console fixture corpus entry.

#### 3. Architecture and in-repository consumer verification

**Files**:

- `loomspan-spring-boot-starter/src/test/java/com/lokiscale/loomspan/architecture/LoomspanPublicSurfaceArchitectureTest.java`
- Existing Java journal/live projection tests and Console parser/browser plan-comparison tests (verification only unless a regression is exposed).

**Changes**:

- Keep the supported API allowlist unchanged and verify technically public internal type classification remains accurate.
- Confirm no Spring bean, configuration property, manifest field, or public API type was introduced for the ID supplier.
- Run existing Console-side tests only if current repository conventions expose a focused command during implementation; no Java-to-Go schema edit or fixture update is planned.

### Success Criteria

#### Automated Verification

- [x] Combined trace and architecture tests pass: `.\mvnw.cmd -pl loomspan-spring-boot-starter "-Dtest=ExecutionTraceContractTest,LoomspanPublicSurfaceArchitectureTest" test`
- [x] All focused producer/projector tests pass together: `.\mvnw.cmd -pl loomspan-spring-boot-starter "-Dtest=PlanningServiceTest,ExecutionPlanTest,ExecutionStateServiceTest,ExecutionTraceContractTest,ExecutionJournalProjectionContractTest,LoomspanPublicSurfaceArchitectureTest" test`
- [x] Full starter module passes: `.\mvnw.cmd -pl loomspan-spring-boot-starter test`
- [x] `LoomspanPublicSurfaceArchitectureTest` reports no supported API addition or internal-type classification drift.
- [x] The representative trace proves primary-root/parent-lineage selection against a nested same-skill competitor, distinct plan chains, recorded rejected-proposal evidence, accepted-attempt/retry/warning joins, multiple cross-frame updates, highest-sequence final-state selection, metadata/data agreement, and valid canonical serialization.

#### Manual Verification

- [x] Inspect the representative trace as a graph of recorded identifiers and confirm the primary plan is selected through recorded trace roots and parent-frame lineage, not route or skill name, then followed across frames solely by framework `planId` with the highest-sequence chain member treated as final.
- [x] Confirm the accepting attempt is mechanically joinable through `attemptId` and validation/retry history through `retrySequenceId`, without making a causal claim about why the accepted proposal changed.
- [x] Confirm no source or documentation change introduces `planId` as skill input, configuration, API, SPI, or model responsibility.

---

## Testing Strategy

Create a dedicated testing plan with `ai/commands/3_testing_plan.md` before implementation. It should designate the prompt/parser ownership tests and missing-attempt acceptance test as failing-first boundaries, then layer state/recorder and combined trace coverage.

### Unit Tests

- Prompt omits `planId`; JSON/YAML parsing works without it.
- Unsolicited `planId` is overwritten; unrelated unknown fields still fail.
- Supplier rejects null/blank values and yields distinct IDs for independent proposals.
- Every immutable `ExecutionPlan` copy method preserves identity.
- Accepted-attempt extraction requires `attemptId` and `retrySequenceId` and copies only those fields into creation metadata.
- State/trace recorder metadata and data agree for creation and update records.

### Integration and Contract Tests

- Validation failure followed by acceptance: a rejected `MODEL_RESPONSE_RECEIVED` proposal with its attempt ID, distinct accepting attempt ID, shared retry sequence, and one created plan linked to the accepting attempt only.
- Ordinary quality exhaustion: warnings followed by an accepted identified plan, with warning and creation joined through the accepting attempt/retry identifiers.
- Deterministic evidence-coverage exhaustion: no stored plan or creation record.
- Missing/invalid result context: failure before state mutation and no synthetic identifiers.
- Primary plus nested same-skill plan topology: primary selection through trace-root/parent lineage, independent framework IDs for identical proposal content, preserved snapshot/restore, multiple updates across frame transitions, and highest-sequence final-state selection within each chain.
- Canonical trace remains valid NDJSON; journal/live activity and opaque Console ingestion semantics remain coherent.

### Manual Testing Steps

1. Capture the exact planning prompt generated for a representative skill and verify it delegates task content but not plan identity.
2. Read the combined test trace and follow `PLAN_CREATED.planId` through every update without using frame placement as chain membership.
3. Join the selected creation record to the accepting model attempt and retry/validation records by identifiers, and verify rejected content has no recorded plan identity.

## Performance Considerations

One UUID generation and a small top-level tree mutation are added per parsed proposal. Semantic retries may allocate IDs that are discarded before acceptance, which is intentional and bounded by existing retry limits. Creation metadata gains two short strings. No new indexes, scans, retained aggregates, frame traversal, or Console processing are introduced, so runtime and artifact-size impact should be negligible.

## Migration Notes

No migration or compatibility shim is planned. The repository is pre-1.0, the changed Java seams are internal, and trace records are current-version ephemeral diagnostics. Tests and in-repository callers move atomically to the new prompt, internal method signatures, and trace semantics. Artifacts from older development revisions retain model-authored IDs and missing acceptance fields; any degraded heuristic support belongs to later MCP/skill product work and must not be implemented in this producer ticket.

## References

- Original ticket: `ai/thoughts/tickets/loomspan-framework-pr-27-plan-identity-and-lineage.md`
- Related research: `ai/thoughts/research/2026-08-15-PR-27-plan-identity-and-lineage.md`
- Framework compatibility lens: `ai/thoughts/framework-feature-design-lens.md`
- LLM trace-understanding roadmap: `ai/thoughts/phases/2026-08-15-loomspan-llm-trace-understanding-roadmap.md`
- Plan workflow requirements: `ai/thoughts/phases/loomspan_llm_trace_understanding_workflows.md`
- Skill-authoring routing and trace guidance: `ai/skill-authoring/README.md`, `ai/skill-authoring/source-verification.md`, `ai/skill-authoring/traces-and-debugging.md`
