# PR 27 - Framework-Owned Plan Identity and Accepted-Attempt Lineage Testing Plan

## Change Summary

- Remove `planId` from the planning model's requested response shape and assign the existing `ExecutionPlan.planId` from a framework-owned UUID-backed supplier.
- Discard/overwrite an unsolicited model `planId` while retaining strict rejection of every other unknown planning property.
- Require the accepted planning response's existing `attemptId` and `retrySequenceId` before plan storage and `PLAN_CREATED`; do not generate fallback lineage.
- Add `attemptId` and `retrySequenceId` to `PLAN_CREATED` metadata while keeping `PLAN_UPDATED` plan-ID-only.
- Preserve the generated ID through every immutable plan copy, task lifecycle update, and nested plan snapshot/restore.
- Keep the supported Application API, absent SPI, configuration/manifest contracts, frame placement, Console projections, and skill-authoring guidance unchanged.

## Impacted Areas

- **Planning prompt and parse boundary**: `loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/runtime/planning/DefaultPlanningService.java`
- **Normalized plan identity and immutable copies**: `loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/core/ExecutionPlan.java`
- **Existing attempt-map validation**: `loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/core/ModelTraceContext.java`
- **Plan-created state boundary**: `loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/runtime/state/ExecutionStateService.java` and `DefaultExecutionStateService.java`
- **Trace recording boundary**: `loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/core/ExecutionTraceRecorder.java` and `DefaultExecutionTraceRecorder.java`
- **Nested plan storage semantics**: `DefaultExecutionStateService` and `CapabilityExecutionRouter`
- **Current-run trace consumers**: `ExecutionJournalProjector`, `LiveActivityProjector`, the Go trace parser's opaque metadata/data handling, and browser plan comparison through `data.planId`
- **Test interactions and direct internal callers**: `SimpleChatClient`, local planning sequence clients, coordinator/step-loop helpers that return accepted plan content, `ExecutionStateServiceTest`, and direct `logPlanCreated` scaffolding
- **Supported public-surface enforcement**: `LoomspanPublicSurfaceArchitectureTest`

## Risk Assessment

### High-Risk Behaviors

- **Identity trust boundary**: a legacy or adversarial model value could still become normalized plan state if tree mutation occurs after binding, only handles JSON, or does not overwrite all recognized representations.
- **Strict codec regression**: removing `planId` before binding could accidentally turn the planning parser into a general unknown-field sanitizer instead of preserving `FAIL_ON_UNKNOWN_PROPERTIES` for other fields.
- **ID allocation cardinality**: caching or reusing one supplier value could merge independent accepted plans; generating on every immutable copy could split one plan chain.
- **Acceptance atomicity**: validating attempt context after `storePlan` could leave a session plan without a trustworthy `PLAN_CREATED` when context is absent or invalid.
- **Lineage substitution**: the planning service could silently mint fallback attempt/retry IDs rather than using the provider advisor's existing values.
- **Metadata over-copying**: blindly merging the attempt map into `PLAN_CREATED` could add unrequested attempt-number/reason fields or arbitrary keys, creating a wider diagnostic contract than approved.
- **Rejected-state leakage**: a proposal rejected for quality or deterministic evidence coverage could emit `PLAN_CREATED`, become session state, or share the accepted plan's recorded identity.
- **Cross-frame reconstruction**: tests confined to one planning frame could pass while later `PLAN_UPDATED` records fail to preserve the same ID on a root or nested mission frame.
- **Test-double drift**: existing helpers currently return empty response context. Weakening production validation to preserve those tests would invalidate the required acceptance boundary; focused doubles must instead mimic the production advisor result shape.

### Edge Cases

- JSON and YAML responses with no `planId`.
- JSON and YAML responses with a textual, numeric, blank, or reused unsolicited `planId`; none may control normalized identity or cause rejection by itself.
- An unrelated unknown top-level property in JSON and YAML remains rejected.
- Supplier returns `null`, an empty string, or whitespace; parsing fails before plan state is recorded.
- Two byte-identical accepted responses, including the same unsolicited ID, receive different framework IDs.
- A valid result context versus missing context key, non-map context value, missing `attemptId`, missing `retrySequenceId`, blank identifiers, and otherwise malformed attempt maps.
- One quality-rejected proposal followed by acceptance: distinct attempt IDs, shared retry sequence, only the accepting attempt referenced by `PLAN_CREATED`.
- Exhausted ordinary quality errors remain warning-backed acceptance; exhausted deterministic evidence gaps remain rejection.
- `updateTask`, `withActiveTask`, `clearActiveTask`, and `withStatus` preserve the original plan ID.
- Parent plan snapshot, nested accepted plan, nested updates, restoration, and subsequent parent update each retain the correct distinct identity.
- `PLAN_CREATED.metadata.planId` and `data.planId` agree; every `PLAN_UPDATED` has the same agreement but no copied accepting-attempt fields.
- Plan data large enough to use the existing canonical serialization path remains valid NDJSON; no new historical-reader behavior is expected.

### Compatibility Scope

- **Application API — protected path**: the closed `com.lokiscale.loomspan.api` allowlist must remain unchanged, and no internal type may leak through supported API signatures.
- **Supported SPI — no path exists**: tests must not establish the ID supplier, planning service constructors, state interface, recorder interface, or Spring infrastructure beans as supported replacement points.
- **Configuration and manifest contracts — protected unchanged path**: no `loomspan.*` property or YAML manifest key may be introduced or changed.
- **Persisted or serialized contracts — no historical path**: do not add fixtures or assertions requiring old development traces, model-authored IDs, or missing lineage metadata to remain readable across versions.
- **Ephemeral diagnostic formats — current-run coherence**: test accurate writer/data/metadata relationships, valid serialization, projector recognition, neutral lineage, and existing frame placement in the current checkout.
- **Internal or accidentally exposed implementation — approved removal**: update the two-argument plan-created recording chain and old model-owned prompt behavior atomically. Do not test or retain simultaneous old/new overloads, legacy parsing modes, fallback lineage, or dual record shapes.
- **Java-to-Go boundary**: no coordinated schema change is required because Go retains metadata/data as opaque JSON and the browser already reads `data.planId`. Run existing Go/browser consumer tests as regression canaries; do not add a Console fixture corpus entry in this PR.
- **Skill-authoring claims**: none. The implementation plan classifies skill-authoring documentation impact as “No impact,” so no documentation-evidence tests are required.

## Existing Test Coverage

### Relevant Coverage

- `PlanningServiceTest#initializesPlanFromYamlWithNormalizedStatuses` proves YAML coercion/status normalization but currently asserts adoption of the model-authored ID.
- `PlanningServiceTest#planningCodecRoleRejectsUnknownFieldsInJsonAndYaml` protects strict unknown-field rejection.
- `PlanningServiceTest#planningPromptIncludesToolDescriptionsAndAlignmentRules`, `#planningPromptIncludesSkillPromptBeforePlanningContract`, and related prompt tests provide the existing prompt-capture convention.
- `PlanningServiceTest#retriesSingleToolOverusePlanWhenMultipleVisibleToolsExist`, `#stopsRetryingAfterConfiguredPlanQualityRetryCap`, and `#rejectsContractBackedPlanWhenRequiredEvidenceRemainsUncoveredAfterRetries` cover the retry, warning-acceptance, and deterministic rejection branches.
- `ExecutionPlanTest` currently proves task-list immutability and selective task replacement, but not all identity-preserving copy paths.
- `ExecutionStateServiceTest#managesFramePlanAndJournalWritesThroughSingleBoundary` exercises the state/recorder boundary, while `#restoresParentPlanAfterNestedMissionAndClearsWhenNoParentExists` covers snapshot/restore.
- `ExecutionTraceContractTest#planCreationIsOwnedByPlanningFrameNotNestedModelFrame`, `#planningQualityEventsStayUnderThePlanningFrame`, and `#exhaustedPlanQualityRetriesDegradeToPlanningWarningUnderPlanningFrame` protect current placement and quality-event behavior.
- `ExecutionJournalProjectionContractTest#projectsCanonicalDeveloperFacingJournalFromRepresentativeTraceStream` proves plan creation remains a developer-facing journal entry.
- `LiveActivityProjectorTest` includes `PLAN_CREATED` and `PLAN_UPDATED` in the visible activity vocabulary.
- `LoomspanPublicSurfaceArchitectureTest` protects the exact supported API, absence of SPI, classification of technically public internals, and API signature boundaries.
- Console `planComparison.test.ts` and `TraceRecords.plan.test.tsx` construct plan records and protect browser comparison through `data.planId`; Go `parser_test.go` protects the opaque canonical record envelope.

### Gaps

- No existing test accepts a model response without `planId` or proves the prompt omits it.
- No existing test proves an unsolicited recognized `planId` is overwritten while unrelated unknown fields remain errors.
- No deterministic supplier coverage proves independent accepted plans receive distinct IDs or rejects invalid supplier output.
- Copy-method coverage does not assert identity preservation for all `ExecutionPlan` transitions.
- Existing test model interactions generally return an empty response context rather than production-shaped attempt metadata.
- No test requires attempt context before session mutation.
- No test asserts `PLAN_CREATED` metadata/data ID agreement or accepting-attempt/retry fields.
- No test asserts that `PLAN_UPDATED` omits accepting-attempt fields.
- No combined trace scenario spans recorded rejected-response evidence, acceptance, a primary root plus nested same-skill competitor, snapshot/restore, and multiple cross-frame updates.
- No current test proves that the primary creation is selected through recorded trace-root/parent lineage, that chain membership remains reconstructable without frame/route/skill-name equality, or that the highest-sequence chain member is the final recorded state.

## Bug Reproduction / Failing Test First

### Failing Test 1: Model-Independent Plan Identity

- **Name**: `acceptsPlanningResponseWithoutPlanIdAndGeneratesFrameworkIdentity`
- **Type**: unit/service test
- **Location**: `loomspan-spring-boot-starter/src/test/java/com/lokiscale/loomspan/internal/runtime/planning/PlanningServiceTest.java`
- **Arrange**: Use the existing public `DefaultPlanningService` constructor. Return valid plan JSON with the `planId` member removed and a valid production-shaped response attempt map. Keep all task/capability fields valid so identity is the only failing condition.
- **Act**: Call `initializePlan`.
- **Assert**: An `ExecutionPlan` is accepted, its `planId` is nonblank, the session contains the same normalized plan, and the emitted `PLAN_CREATED` uses that ID.
- **Expected failure pre-fix**: `treeToValue` creates an `ExecutionPlan` with a missing/null `planId`, and the record constructor rejects it before acceptance. This fails for the intended reason without requiring the not-yet-added injectable supplier constructor.

### Failing Test 2: Accepted-Attempt Lineage

- **Name**: `planCreatedLinksToTheAcceptingAttemptAndRetrySequence`
- **Type**: service/trace contract test
- **Location**: Start in `PlanningServiceTest.java` for the smallest producer path; retain the broader topology in `ExecutionTraceContractTest.java`.
- **Arrange**: Return a currently valid plan response containing a legacy `planId` plus a valid response context with known `attemptId`, `retrySequenceId`, attempt number, reason, and provider-attempt number.
- **Act**: Initialize the plan and locate its single `PLAN_CREATED` record.
- **Assert**: Metadata contains the exact known `attemptId` and `retrySequenceId`, while `metadata.planId` equals `data.planId`.
- **Expected failure pre-fix**: plan creation succeeds, but `DefaultExecutionTraceRecorder` emits only `planId` metadata, so the two lineage assertions fail.

The two tests should be committed or run red before production changes. The first protects identity ownership; the second prevents an implementation that fixes ID generation but still leaves acceptance lineage inferential.

## Tests to Add/Update

### 1. Prompt Excludes Plan Identity

- **Name**: `planningPromptDoesNotAskTheModelForPlanId`
- **Type**: unit/service test
- **Location**: `loomspan-spring-boot-starter/src/test/java/com/lokiscale/loomspan/internal/runtime/planning/PlanningServiceTest.java`
- **What it proves**: Captured planning instructions contain the exact remaining plan/task contract, including `taskId`, but neither a `planId` property nor prose assigning unique plan identity to the model.
- **Fixtures/data**: Existing `SimpleChatClient` prompt capture and a valid response without `planId`.
- **Mocks**: No framework mocks; use a focused model interaction with valid attempt context.
- **Contract classification**: Internal or accidentally exposed implementation.
- **Compatibility expectation**: Approved removal of the old internal model protocol; no test should preserve the old prompt shape.

### 2. JSON/YAML Framework Identity Normalization

- **Names**:
  - `acceptsJsonAndYamlPlansWithoutPlanId`
  - `overwritesUnsolicitedJsonAndYamlPlanId`
  - update `planningCodecRoleRejectsUnknownFieldsInJsonAndYaml`
- **Type**: parameterized unit/service tests or compact loop-based tests matching current conventions
- **Location**: `PlanningServiceTest.java`
- **What it proves**: Missing `planId` is normal; textual, numeric, blank, and repeated unsolicited values cannot become normalized identity and do not alone reject a valid proposal; another unknown property still fails under strict binding.
- **Fixtures/data**: Equivalent JSON/YAML plan bodies with absent, textual, numeric, and blank candidate IDs; deterministic framework supplier values for exact assertions.
- **Mocks**: Inject the package-private deterministic supplier and return production-shaped attempt context.
- **Contract classification**: Internal or accidentally exposed implementation.
- **Compatibility expectation**: Approved removal of model-owned identity while preserving the strict planning-codec invariant.

### 3. Supplier Validity and Allocation Cardinality

- **Names**:
  - `rejectsNullOrBlankFrameworkPlanId`
  - `identicalAcceptedResponsesReceiveDistinctFrameworkPlanIds`
- **Type**: unit/service test
- **Location**: `PlanningServiceTest.java`
- **What it proves**: Every candidate framework ID is nonblank; two independent acceptances consume distinct supplier values even when proposal bytes and unsolicited model IDs are identical; rejected candidates do not appear as recorded plan state.
- **Fixtures/data**: A deque supplier such as `plan-framework-1`, `plan-framework-2`; null/empty/whitespace suppliers; one identical valid plan payload.
- **Mocks**: No UUID mocking. Inject a deterministic `Supplier<String>` through the internal constructor.
- **Contract classification**: Internal or accidentally exposed implementation.
- **Compatibility expectation**: Current internal behavior only; the supplier must not become a supported constructor, Spring bean, SPI, or configuration contract.

### 4. Immutable Copy Identity

- **Name**: `allImmutablePlanCopiesPreservePlanId`
- **Type**: unit test
- **Location**: `loomspan-spring-boot-starter/src/test/java/com/lokiscale/loomspan/internal/core/ExecutionPlanTest.java`
- **What it proves**: `updateTask`, `withActiveTask`, `clearActiveTask`, and `withStatus` return plans with exactly the original framework ID while changing only their intended fields.
- **Fixtures/data**: One plan with at least two tasks and a fixed framework ID.
- **Mocks**: None.
- **Contract classification**: Internal or accidentally exposed implementation.
- **Compatibility expectation**: Protect the one-chain invariant, not the technical constructor as an SPI.

### 5. Production-Shaped Planning Test Doubles

- **Name**: helper update rather than a standalone behavioral test; retain one canary assertion in `recordsPlanningTraceWithRealProviderMetadata`
- **Type**: test infrastructure plus integration canary
- **Locations**:
  - `loomspan-spring-boot-starter/src/test/java/com/lokiscale/loomspan/internal/runtime/SimpleChatClient.java`
  - Local sequence model interactions in `PlanningServiceTest.java` and `ExecutionTraceContractTest.java`
  - Planning-capable helpers in `FakeCoordinatorChatClient`, `StepLoopMissionExecutionEngineTest`, and any additional call sites found by `rg`
- **What it proves**: Each planning response carries `ModelTraceContext.RESPONSE_ATTEMPT_CONTEXT_KEY` with the request trace context's `nextAttempt()` map. Multiple semantic responses use distinct attempt IDs and share the request's retry-sequence ID.
- **Fixtures/data**: The production map shape: `retrySequenceId`, `attemptId`, positive `attemptNumber`, valid `attemptReason`, and positive `providerAttemptNumber`.
- **Mocks**: Test interactions simulate only the already-defined provider advisor output; do not mock or alter production `ModelTraceContext` validation.
- **Contract classification**: Internal or accidentally exposed implementation.
- **Compatibility expectation**: Remove empty-context assumptions from accepted-plan tests; do not weaken production validation for legacy doubles.

### 6. Required Acceptance Context and Atomic Failure

- **Names**:
  - `missingAcceptedAttemptContextFailsBeforePlanStorage`
  - `invalidAcceptedAttemptContextFailsBeforePlanStorage`
- **Type**: unit/service tests
- **Location**: `PlanningServiceTest.java`
- **What it proves**: Missing context/key, a non-map value, absent or blank `attemptId`, and absent or blank `retrySequenceId` fail before `storePlan`; the session remains empty, no `PLAN_CREATED` exists, and no fallback identifier appears.
- **Fixtures/data**: Otherwise valid plan content and deliberately malformed result-context maps. Use representative cases rather than testing every Java map type.
- **Mocks**: A deliberately context-free or malformed `ModelInteraction`; do not use the updated shared success helper.
- **Contract classification**: Internal or accidentally exposed implementation.
- **Compatibility expectation**: Approved removal of empty-context acceptance; visible failure replaces synthetic repair.

### 7. Plan-Created and Plan-Updated Metadata Contract

- **Name**: `recordsAcceptedAttemptOnPlanCreationButNotPlanUpdates`
- **Type**: unit/state-recorder contract test
- **Location**: `loomspan-spring-boot-starter/src/test/java/com/lokiscale/loomspan/internal/runtime/state/ExecutionStateServiceTest.java`
- **What it proves**: `PLAN_CREATED` contains exactly the framework `planId`, accepting `attemptId`, accepting `retrySequenceId`, and normal recorder-added metadata such as `recordedAt`; data carries the same plan ID. `PLAN_UPDATED` carries the same metadata/data plan ID without accepting-attempt fields.
- **Fixtures/data**: Fixed plan, fixed attempt relationship map, fixed clock, and one updated immutable plan.
- **Mocks**: Use `DefaultExecutionStateService`/`DefaultExecutionTraceRecorder`; update the existing anonymous recorder only for signature compilation and rollback behavior.
- **Contract classification**: Ephemeral diagnostic formats.
- **Compatibility expectation**: Current-run diagnostic coherence; do not preserve the old two-argument signature or old creation shape.

### 8. Nested Snapshot/Restore Identity

- **Name**: strengthen `restoresParentPlanAfterNestedMissionAndClearsWhenNoParentExists`, or add `nestedPlanSnapshotRestorePreservesDistinctPlanIdentities`
- **Type**: unit/state integration test
- **Location**: `ExecutionStateServiceTest.java`
- **What it proves**: The parent snapshot retains its framework ID, a nested accepted plan has another ID, restoring the snapshot returns the parent ID, and later parent updates remain on the parent chain.
- **Fixtures/data**: Two fixed plans with different IDs and at least one update on each.
- **Mocks**: None; use the real state service and immutable plans.
- **Contract classification**: Internal or accidentally exposed implementation.
- **Compatibility expectation**: Protect current single-slot snapshot semantics without adding a plan aggregate or parallel sibling model.

### 9. Rejection and Warning Acceptance Semantics

- **Names/updates**:
  - strengthen `retriesSingleToolOverusePlanWhenMultipleVisibleToolsExist`
  - strengthen `stopsRetryingAfterConfiguredPlanQualityRetryCap`
  - strengthen `rejectsContractBackedPlanWhenRequiredEvidenceRemainsUncoveredAfterRetries`
- **Type**: service integration tests
- **Location**: `PlanningServiceTest.java`
- **What it proves**: A quality-rejected proposal remains attempt-linked model-response evidence but has no creation record; ordinary exhausted quality errors yield warnings and then one accepted identified creation record whose `attemptId` and `retrySequenceId` match the accepting warning evidence; deterministic evidence exhaustion leaves no session plan or creation record.
- **Fixtures/data**: Existing weak/corrected/repeated plan payloads updated to make model `planId` absent or adversarial, deterministic ID sequences, and production-shaped attempt sequences.
- **Mocks**: Existing sequence planning interaction updated to return attempt context.
- **Contract classification**: Ephemeral diagnostic formats and Internal or accidentally exposed implementation.
- **Compatibility expectation**: Preserve existing warning-versus-rejection semantics while removing model-owned identity.

### 10. Accepted Retry Lineage

- **Name**: `validationRetryAndPlanCreationShareExplicitLineage`
- **Type**: service/trace integration test
- **Location**: `PlanningServiceTest.java` or `ExecutionTraceContractTest.java`; keep one authoritative assertion set rather than duplicating the full topology
- **What it proves**: Rejected and accepting `MODEL_RESPONSE_RECEIVED` records have different attempt IDs; all attempts and validation/retry records use one retry-sequence ID; `PLAN_CREATED` references only the accepting attempt and that same sequence; the rejected response has no recorded plan identity; no record asserts that the retry caused a fix.
- **Fixtures/data**: One weak response followed by one accepted response, with deterministic attempt maps derived from one request `ModelTraceContext`.
- **Mocks**: Sequence model interaction only.
- **Contract classification**: Ephemeral diagnostic formats.
- **Compatibility expectation**: Current-run diagnostic accuracy and neutral relationships.

### 11. Combined Multi-Plan, Cross-Frame Trace Contract

- **Name**: `planChainsRemainDistinctAndJoinableAcrossRejectedAttemptsAndFrameTransitions`
- **Type**: integration/producer trace-contract test
- **Location**: `loomspan-spring-boot-starter/src/test/java/com/lokiscale/loomspan/internal/runtime/trace/ExecutionTraceContractTest.java`
- **What it proves**:
  - A primary root mission and nested same-skill root mission have separate competing planning frames, so route and skill-name equality cannot identify the primary plan.
  - The primary creation is selected through the trace's recorded root and parent-frame lineage.
  - The primary and nested plans receive distinct framework IDs despite identical accepted proposal bytes and the same unsolicited candidate ID.
  - One rejected primary proposal remains as attempt-linked `MODEL_RESPONSE_RECEIVED` content and emits validation/retry evidence but no `PLAN_CREATED`.
  - Each accepted creation record links to its accepting attempt and retry sequence.
  - At least two primary updates emitted after planning closes attach to the primary root frame, nested updates attach to the nested root frame, and every record retains metadata/data chain identity.
  - Snapshot/restore returns to the parent chain.
  - Reconstructing membership requires only selected creation structure plus `planId`, not route names, skill names, equal frame IDs, timestamps, or adjacency; the highest canonical sequence within each selected chain is its final recorded state.
- **Fixtures/data**: Fixed clock, deterministic supplier sequence, one rejected plus accepted primary response, byte-identical accepted nested response, a primary and nested same-skill root/planning frame topology, at least two primary updates, and one or more nested updates.
- **Mocks**: Reuse the `ProviderAttemptCallAdvisor`-backed test assembly pattern from `ModelAttemptCallAdvisorIntegrationTest` so model response records and returned attempt context share the production attempt boundary. Use deterministic provider responses, the real state/trace recorder, and the deterministic plan-ID supplier; do not manually append `MODEL_RESPONSE_RECEIVED` around a context-only sequence double.
- **Contract classification**: Ephemeral diagnostic formats.
- **Compatibility expectation**: Current-run writer/reader/projector coherence only; no historical trace fixture or fallback reader.

### 12. Canonical Serialization and Projector Recognition

- **Names/updates**:
  - update `ExecutionJournalProjectionContractTest#projectsCanonicalDeveloperFacingJournalFromRepresentativeTraceStream`
  - add or update a focused `LiveActivityProjectorTest` plan-record case if existing generic coverage does not assert the enriched metadata remains bounded/visible
  - exercise canonical NDJSON in the combined trace test or a focused writer/reader assertion
- **Type**: contract/regression tests
- **Locations**:
  - `loomspan-spring-boot-starter/src/test/java/com/lokiscale/loomspan/internal/runtime/trace/ExecutionJournalProjectionContractTest.java`
  - `loomspan-spring-boot-starter/src/test/java/com/lokiscale/loomspan/internal/runtime/observation/LiveActivityProjectorTest.java`
  - `ExecutionTraceContractTest.java` or existing NDJSON reader/writer tests
- **What it proves**: Enriched creation metadata does not change sanitized journal plan data or activity classification; current trace serialization remains valid; record data still exposes normalized `planId` for consumers.
- **Fixtures/data**: Representative `PLAN_CREATED` with all three relationship fields and related `PLAN_UPDATED`.
- **Mocks**: None beyond existing projector test builders.
- **Contract classification**: Ephemeral diagnostic formats.
- **Compatibility expectation**: Current-version coherence, diagnostic usefulness, and bounded metadata; no old-shape fixture preservation.

### 13. Public Surface and Absence of New Extension Points

- **Names**: existing `LoomspanPublicSurfaceArchitectureTest` suite, especially `apiPackageContainsExactlyEightApprovedPublicTypes`, `everyExternallyAccessibleTopLevelTypeIsClassified`, `noSupportedSpiPackageOrTypeExists`, and `apiSignaturesRecursivelyExcludeInternalAndAutoconfigureTypes`
- **Type**: architecture test
- **Location**: `loomspan-spring-boot-starter/src/test/java/com/lokiscale/loomspan/architecture/LoomspanPublicSurfaceArchitectureTest.java`
- **What it proves**: No supported API type, SPI package/type, or leaked internal signature is introduced. The supplier remains internal and is not exposed through a new application-facing constructor/type.
- **Fixtures/data**: Existing architecture allowlists; they should remain unchanged unless a technically public internal signature needs only its existing classification, not a supported-surface addition.
- **Mocks**: None.
- **Contract classification**: Application API, Supported SPI, and Internal or accidentally exposed implementation.
- **Compatibility expectation**: Preserve the closed Application API and absence of SPI; atomically change internal signatures without a compatibility overload.

### 14. Console Consumer Regression Canaries

- **Names**: existing Go `parser_test.go`, browser `planComparison.test.ts`, and `TraceRecords.plan.test.tsx`
- **Type**: cross-component regression tests; verification-only unless a failure exposes an implementation-plan discrepancy
- **Locations**:
  - `loomspan-console/internal/traceanalysis/parser_test.go`
  - `loomspan-console/web/src/observability/planComparison.test.ts`
  - `loomspan-console/web/src/observability/TraceRecords.plan.test.tsx`
- **What it proves**: Additional metadata keys remain opaque to Go parsing, and browser comparison continues to join full plan snapshots by stable `data.planId` without depending on creation/update frame equality.
- **Fixtures/data**: Existing constructed plan records. Do not add or revise `loomspan-console-fixtures` for this producer PR.
- **Mocks**: Existing browser API/component mocks only.
- **Contract classification**: Ephemeral diagnostic formats.
- **Compatibility expectation**: Current-repository consumer coherence, not Java-to-Go historical compatibility.

## How to Run

Run commands from `C:\opendev\code\loomspan` unless a different working directory is stated. No credentials, network services, Spring profile, or external model provider are required; all planning interactions are test doubles.

### Red/Green Sequence

1. Add only the two failing-first tests and confirm each fails for its stated reason:

   ```powershell
   .\mvnw.cmd -pl loomspan-spring-boot-starter "-Dtest=PlanningServiceTest#acceptsPlanningResponseWithoutPlanIdAndGeneratesFrameworkIdentity+planCreatedLinksToTheAcceptingAttemptAndRetrySequence" test
   ```

   If Surefire method selection with `+` behaves differently in the active PowerShell/Maven environment, run the two methods separately. Do not accept compilation failure from a not-yet-added test-only constructor as the red result.

2. Run focused identity, acceptance, state, trace, projector, and architecture tests while implementing:

   ```powershell
   .\mvnw.cmd -pl loomspan-spring-boot-starter "-Dtest=PlanningServiceTest,ExecutionPlanTest,ExecutionStateServiceTest,ExecutionTraceContractTest,ExecutionJournalProjectionContractTest,LiveActivityProjectorTest,LoomspanPublicSurfaceArchitectureTest" test
   ```

3. Run the complete starter module after focused tests pass:

   ```powershell
   .\mvnw.cmd -pl loomspan-spring-boot-starter test
   ```

### Console Consumer Canaries

These commands verify unchanged current-repository consumers; they are not a request to add Console features or fixtures.

```powershell
Set-Location -LiteralPath 'C:\opendev\code\loomspan\loomspan-console'
go test ./internal/traceanalysis

Set-Location -LiteralPath 'C:\opendev\code\loomspan\loomspan-console\web'
npm test -- src/observability/planComparison.test.ts src/observability/TraceRecords.plan.test.tsx
```

Return to the repository root before any subsequent Maven command.

### Final Repository Checks

```powershell
Set-Location -LiteralPath 'C:\opendev\code\loomspan'
.\mvnw.cmd -pl loomspan-spring-boot-starter "-Dtest=LoomspanPublicSurfaceArchitectureTest" test
.\mvnw.cmd -pl loomspan-spring-boot-starter test
git diff --check
```

## Exit Criteria

- [x] Both failing-first tests are observed failing against the pre-fix behavior for the intended assertions, not because of unrelated setup, compilation, or missing context-helper errors.
- [x] The prompt no longer asks the model for `planId`, and JSON/YAML responses without it are accepted with a nonblank framework value.
- [x] Textual, numeric, blank, repeated, and adversarial unsolicited model IDs never become normalized or recorded identity and do not alone reject an otherwise valid plan.
- [x] JSON/YAML unknown properties other than `planId` remain rejected by the strict planning codec.
- [x] Null/blank supplier output fails, and independent identical accepted proposals receive distinct framework IDs.
- [x] `updateTask`, `withActiveTask`, `clearActiveTask`, `withStatus`, task lifecycle updates, and nested snapshot/restore preserve the correct existing plan ID.
- [x] Accepted-plan test doubles return production-shaped attempt context; no production fallback or weakened validation exists solely for test compatibility.
- [x] Missing or invalid accepting-attempt context fails before plan storage and `PLAN_CREATED`, with no fallback lineage identifier.
- [x] `PLAN_CREATED` metadata contains the normalized plan ID plus the exact accepting attempt/retry identifiers, and its data contains the same plan ID.
- [x] `PLAN_UPDATED` metadata/data preserve the same plan ID and do not duplicate accepting-attempt fields.
- [x] Rejected proposal content remains only in its attempt-linked model response, related validation/retry evidence may reference that attempt, and deterministic evidence-coverage failures emit no `PLAN_CREATED`; ordinary exhausted quality errors retain warning evidence whose accepting attempt/retry identifiers match the following identified creation record.
- [x] The combined Java trace scenario proves primary-root/parent-lineage selection against a nested same-skill competitor, distinct independent chains, recorded rejected-proposal evidence, accepted-attempt/retry joins, multiple cross-frame updates, snapshot/restore, metadata/data agreement, highest-sequence final-state selection, and reconstruction without route/frame/skill-name/order identity assumptions.
- [x] Canonical trace serialization remains valid NDJSON, and journal/live projectors continue recognizing plan records without exposing a causal conclusion.
- [x] Console Go/browser regression canaries pass without a production schema, projection, or fixture-corpus change.
- [x] `LoomspanPublicSurfaceArchitectureTest` passes with no new supported Application API, SPI, configuration surface, or leaked internal type.
- [x] The obsolete model-owned prompt behavior and old internal two-argument plan-created chain are absent; there is no overload, fallback reader, dual record shape, or compatibility shim preserving them.
- [x] No `ai/skill-authoring/` change is present because the approved documentation impact is “No impact.”
- [x] All focused tests and the complete `loomspan-spring-boot-starter` module pass.
- [x] Manual review confirms recorded identifiers express identity/membership only and do not assert that a retry fixed a validation issue.
- [x] `git diff --check` reports no whitespace errors.
