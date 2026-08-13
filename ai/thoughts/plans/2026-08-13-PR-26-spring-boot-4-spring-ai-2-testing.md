# PR 26 Spring Boot 4 / Spring AI 2 Platform Migration Testing Plan

## Change Summary

- Upgrade the Loomspan reactor from Spring Boot 3.5.11 and Spring AI 1.1.6 to Boot 4.1.0, Framework 7, Spring AI 2.0.0, Java 21, and Jackson 3.
- Replace Spring AI types in mission/planning/step orchestration with a neutral, session-bound `ModelInteraction` boundary.
- Separate Loomspan capability execution from Spring AI `ToolCallback` adaptation.
- Rebuild OpenAI and Anthropic with official SDK clients, use zero-retry Framework 7 templates for Gemini/Ollama, and retain exactly one physical send per Loomspan attempt.
- Adopt exactly one AI 2 `ToolCallingAdvisor`, with semantic validation outside the tool loop and Loomspan physical attempts inside it.
- Replace application-facing Spring AI `@ToolParam` with Loomspan `@SkillParam` without a compatibility bridge.
- Move Loomspan serialization to purpose-owned Jackson 3 codecs while retaining YAML, REST, SSE, artifact, canonical NDJSON, and same-version portable-trace behavior.
- Remove obsolete connection properties, Spring AI 1/Jackson 2/Spring Retry code, fluent client doubles, and compatibility residue.
- Add Spring AI observations without duplicating Loomspan counters, quota accounting, traces, or sensitive-content defaults.
- Verify each mandatory implementation review checkpoint before work resumes and run one independent cumulative review after Phase 7.

## Test Design Principles

- Phase 1 characterization tests must pass on the Boot 3/AI 1 baseline. They establish retained observable behavior before migration; they must not pin incidental mapper, SDK, or internal exception details.
- Each behavior-changing phase begins with a red gate that fails for the intended missing target behavior. Do not weaken, disable, or conditionally skip a gate merely to keep an intermediate commit green.
- Mock provider protocols at the HTTP boundary with deterministic local servers. Handwritten `ChatClient` fluent doubles are migration residue and must not appear in new tests.
- Assert exact counts for physical sends, tool executions, quota reservations, attempt facts, model responses, metrics, and observations. “At least once” is insufficient for retry/recursion tests.
- Exact fixture corpora protect deliberate wire consumers. Internal Spring AI types, old property fields, old packages, and exact framework error messages are approved removals and must not receive dual-behavior tests.
- Tests of traces protect current writer/reader/projector/debugging coherence, security, ordering, and same-version portability. They do not require cross-version readers or historical schemas.
- Every checkpoint review receives the exact test commands/results for its range. Passing an earlier checkpoint does not reduce the final cumulative test scope.

## Impacted Areas

- **Build and modules**: root/starter/sample POMs, Boot Web MVC/security/test modules, dependency management, compiler and packaging.
- **Serialization**: application conversion, skill YAML, planning JSON/YAML, schema trees, canonical NDJSON, trace projection, strict REST/problem/cursor JSON, SSE and artifact payloads.
- **Application API**: `SkillMethod`, new `SkillParam`, reflected method discovery, proxy/interface contracts, schemas, binding, mapped execution, sample services.
- **Configuration contracts**: `LoomspanProperties`, named connections/model aliases, strict validation, generated metadata, sample application configuration.
- **Model boundary**: `ExecutionCoordinator`, mission engines, planning, step loop, attachments, prompts, validation, normalized response content/context.
- **Capability execution**: visibility descriptors, argument contracts, plan linkage, tool quotas, frames, execution routing, results, metrics, failures, trace events.
- **Provider integration**: official OpenAI/Anthropic SDK clients, Gemini/Ollama Framework retry, endpoints/headers, OpenRouter guard, failure translation, `Retry-After`, resource lifecycle and redaction.
- **AI 2 assembly**: option builders, advisors, tool loop, semantic retries, physical attempts, usage extraction, observations, cancellation/timeouts.
- **Auto-configuration**: stable runtime, AI integration, codec wiring, MVC/no-MVC/security conditions, no replacement SPI.
- **Cross-language diagnostics**: Java REST/SSE/artifact/NDJSON producers, fixture corpora, Go acquisition/import/analysis, TypeScript browser contracts and imported-evidence behavior.
- **Documentation and samples**: root/sample documentation and routed skill-authoring guidance for Java parameters, connections, retries, traces, and portability.

## Risk Assessment

### Highest-risk behaviors

- A hidden SDK/framework retry can multiply sends, cost, quota use, trace facts, and side effects below one Loomspan attempt.
- Incorrect advisor ordering can run semantic validators, tools, or physical attempts at the wrong recursion scope, causing duplicate tools or incorrect usage/trace counts.
- Spring AI types can remain leaked through a neutral-looking interface, preserving the upgrade/test coupling the migration is intended to remove.
- Splitting `DefaultToolCallbackFactory` can double or omit plan linkage, quota, frames, metrics, tool terminal facts, failure recording, or nested execution state.
- Jackson 3 defaults can silently alter unknown-field behavior, records, enums, time, null/default omission, number nodes, map ordering, newline-delimited output, cursors, or exact Console fixtures.
- Official SDK path/header behavior can differ from the removed REST clients; OpenRouter may surface partial content from an HTTP-200 error completion unless rejected before decoding.
- Observation auto-configuration can duplicate Loomspan metrics or export prompts, responses, arguments, credentials, headers, or provider bodies.
- Strict property removal can accidentally retain old fields, silently ignore them, or reject surviving fields for the wrong provider.
- Boot 4 modularization can disable MVC/security conditions or alter exact REST/SSE error and lifecycle behavior.

### Important edge cases

- Null/blank model content; missing provider usage; heuristic/unavailable usage; unknown finish reason.
- Provider retry exhaustion, `Retry-After` seconds/date, timeout, connectivity, SSL, interruption before send, interruption during backoff, and cancellation within a tool loop.
- Empty tool surface, one tool, multiple tools, invalid tool arguments, planned and unplanned calls, bound step-loop tasks, tool exceptions/errors, nested YAML skills, and attachment-bearing calls.
- OpenAI base URLs with and without `/v1`; OpenRouter profile on/off; Anthropic custom/beta headers; Gemini API-key and Vertex modes; Ollama native path.
- JSON/YAML unknown fields, nullable values, missing/defaulted fields, enum case, records, Java time, large integers/decimals, insertion order, trailing partial NDJSON records, chunks, and exact newline termination.
- Boot application with MVC/security, servlet application without MVC, and non-web contexts.
- Imported trace with no target, target rotation while imported evidence is open, incompatible release marker, and imported error serialization without owner/target-scope leakage.

### Contract classification and compatibility expectations

| Surface | Classification | Testing obligation |
| --- | --- | --- |
| Existing seven API types plus `SkillParam` | Application API | Preserve existing API behavior, prove the exact eight-type allowlist and neutral public signatures, and prove the approved `ToolParam` replacement without dual support. |
| Loomspan bean overrides | Supported SPI | Prove none exists: no `@ConditionalOnMissingBean`, empty override allowlist, and no new replacement seam from the auto-configuration split. |
| YAML manifests and named connection/model properties | Configuration and manifest contracts | Keep YAML behavior; prove surviving property binding/metadata and exact rejection of removed/driver-inapplicable fields. |
| REST/SSE/artifact/problem corpora and complete portable NDJSON file | Persisted or serialized contracts for repository-local/same-version consumers | Preserve exact fixtures and exact marker acceptance/rejection. Any fixture delta requires an explicit Java/Go/TS/marker decision rather than test regeneration alone. |
| Current-run trace records, projectors, installed imported evidence, handles/indexes/cursors/browser DTOs | Ephemeral diagnostic formats | Prove current writer/reader/projector/tool/debugging coherence, security, bounded diagnostics, ordering, failure visibility, and transient imported behavior; no historical-reader tests. |
| Spring AI-facing interfaces, versioned packages, old callback factory representation, private mappers, exact framework messages | Internal or accidentally exposed implementation | Replace/remove tests that encode old decomposition; add absence/containment gates, not compatibility tests. |

### Authoring claims requiring executable evidence

- `@SkillParam` supplies parameter descriptions and requiredness across implementation/interface/proxy contracts.
- Model aliases still resolve through named connections to the selected driver/provider model.
- OpenAI/Anthropic removed path/version fields are rejected; Anthropic beta/custom headers use the common headers map.
- One physical attempt equals one provider send and one provider-attempt quota reservation, even inside AI 2 tool and semantic loops.
- Traces distinguish semantic retries, tool-loop turns, provider attempts, tool executions, terminal failure, and usage without changing same-version portability limitations.

## Existing Test Coverage

### Coverage to preserve and port

- `LoomspanPublicSurfaceArchitectureTest`: exact API/framework/internal classification, no SPI, no internal types in API signatures.
- `LoomspanAutoConfigurationBoundaryTest`: empty override allowlist, package-private bean methods, no `@ConditionalOnMissingBean`.
- `LoomspanPropertiesTest` and `ConfigurationMetadataTest`: strict named-connection binding, driver validation, retry defaults, redaction, and metadata.
- `SkillMethodBeanPostProcessorTest` and `SkillMethodTargetDiscoveryIntegrationTests`: method discovery, overloads, interface/proxy/bridge contracts, optional parameters, schema, resources, binding, and invocation.
- `SpringAiV11ProviderIntegrationTest`, `ConnectionProtocolTest`, and `ModelAttemptCallAdvisorIntegrationTest`: one-send controls, provider protocol construction, failure translation, retries, quotas, interruptions, traces, and OpenRouter recovery.
- `SpringAiSkillChatClientFactoryTests`: provider/model/options/advisor selection and step-execution advisor omission; replace implementation-centric assertions with target-boundary tests.
- `MissionExecutionEngineTest`, `PlanningServiceTest`, and `StepLoopMissionExecutionEngineTest`: direct/planned/step execution, prompts, attachments, timeouts, semantic retries, bound tools, planning quality/evidence, failures, and traces.
- `ToolCallbackFactoryTest`: visible definitions, mapped routing, resource arguments, step-loop binding, and tool failure ordering; port to `DefaultCapabilityInvokerTest` plus the Spring adapter test.
- `LinterCallAdvisorTest`, `OutputSchemaCallAdvisorTest`, and evidence advisor tests: semantic validation/retry behavior and mutation facts.
- `ExecutionTraceContractTest`, NDJSON reader/writer/projector tests, and `ConsoleTraceFixtureCorpusTest`: trace ownership, redaction, canonical semantics, corpus bytes, invalid classifications, tool lifecycle, and usage reconciliation.
- `ConsoleRestFixtureCorpusTest`, `ConsoleSseFixtureCorpusTest`, `ConsoleArtifactFixtureCorpusTest`, `ObservabilityRestIntegrationTest`, `ObservabilitySseIntegrationTest`, and `ObservabilityWithoutMvcIntegrationTest`: exact protocol bytes and Boot web behavior.
- `ModelUsageExtractorTest` and `MicrometerUsageMetricsRecorderTest`: exact/heuristic/unavailable usage and Loomspan meters.
- `SupportedSurfaceIntegrationTest`: application invocation through `SkillTemplate` without replacing internal infrastructure.
- Go `internal/applicationclient`, `internal/traceanalysis`, `internal/artifact`, `internal/browserapi` tests and frontend observability tests: exact marker validation, portable import, imported ownership/error scope, target-rotation independence, and presentation.

### Current gaps

- No durable target test proves AI 2 semantic-policy/tool-loop/physical-attempt recursion using real advisors and exact counts.
- OpenAI and Anthropic current tests exercise removed REST clients, not official SDK zero-retry clients.
- Gemini has a direct one-send probe; Ollama does not have a symmetric durable exact-send target test.
- No architecture rule confines all Spring AI integration types to the target subsystem after refactoring.
- No shared neutral capability-invocation contract proves provider-originated and step-loop calls have identical lifecycle semantics without double plan updates.
- Existing mapper tests do not present one explicit matrix across every purpose-owned Jackson 3 codec role.
- No observation integration test proves safe content defaults and non-duplication of Loomspan meters/traces.
- No test yet defines `SkillParam` as the only supported Java parameter annotation.
- No final dependency/source gate distinguishes forbidden Loomspan Jackson 2 usage from the allowed SDK-private transitive codec.
- No single checkpoint manifest records required commands/results and review range for each mandatory pause.

## Bug Reproduction / Failing Test First

This is an architectural migration rather than one existing user-visible bug. Phase 1 therefore adds only green characterization. The first durable red test is the boundary defect the migration is designed to remove.

- **Name**: `springAiTypesAreConfinedToTheIntegrationSubsystem`
- **Type**: architecture test
- **Location**: new `loomspan-spring-boot-starter/src/test/java/com/lokiscale/loomspan/architecture/SpringAiBoundaryArchitectureTest.java`
- **Arrange**: import production classes with ArchUnit; define the allowed Spring AI dependency set as `..internal.springai..`, Spring-facing advisor implementations explicitly owned by that subsystem, and AI auto-configuration wiring. Define orchestration/core/planning/step/attachment/tool-neutral packages as forbidden consumers.
- **Act**: check dependencies on packages matching `org.springframework.ai..`.
- **Assert**: no forbidden production class directly or recursively exposes a Spring AI type in fields, constructors, method signatures, record components, generic arguments, annotations, or implementation imports.
- **Expected failure before Phase 3**: violations include `ExecutionCoordinator`, `MissionExecutionEngine`, `PlanningService`, `MissionUserMessageSender`, `StepLoopMissionExecutionEngine`, `ToolCallbackFactory`, `CapabilityToolDescriptor`, and usage/response helpers.
- **Why this test**: it fails on the current architecture, remains meaningful after implementation, and directly proves the intended dependency boundary without requiring target provider availability.

### Additional phase red gates

- **Phase 2**: after pinning Boot 4.1.0/AI 2.0.0/Jackson 3, target compilation initially fails on removed provider/retry/Web MVC/Jackson APIs. Capture the compiler failure as the phase red baseline; do not create tests for removed symbols.
- **Phase 3**: `providerAndStepLoopInvocationShareOneLifecycleContract` initially fails until `CapabilityInvoker` owns both paths.
- **Phase 4**: provider protocol tests initially fail exact paths/headers/sends until official clients and zero-retry controls are installed.
- **Phase 5**: `semanticRetriesWrapToolLoopAndPhysicalAttemptsRunInsideIt` initially fails until the AI 2 advisor chain is assembled correctly. The Phase 1 Spring AI 1 characterization uses explicit provider tool-call responses and proves four HTTP sends, two outer Loomspan attempt observations, and two callback executions; it must not simulate or claim four inner attempt facts before the AI 2 advisor exists.
- **Phase 6**: `apiPackageContainsExactlyEightApprovedPublicTypes` and `skillParamIsTheOnlySupportedParameterContract` fail until the annotation and processor/sample migration are complete; removed-property rejection tests fail while old fields remain accepted.
- **Phase 7**: residue scans fail until old packages/imports/dependencies/doubles/docs are deleted.

## Tests to Add or Update

## Phase 1: Characterization Before Migration

### 1. `retainedCodecContractsCoverJackson3RiskMatrix`
- **Type**: production-role contract tests across existing codec suites
- **Location**: `ObservabilityJsonCodecContractTest.java` plus focused additions to `YamlSkillCatalogTests`, `PlanningServiceTest`, `NdjsonTraceRecordWriterTest`, active trace-reader tests, and the committed REST/SSE/artifact/trace corpus tests
- **What it proves**: current behavior for records, unknown fields, enum/case, time, null/default omission, numeric nodes, insertion ordering, and newline-delimited output is explicit before mapper replacement, with every row executed through its named Loomspan production codec or committed corpus rather than a generic default mapper.
- **Fixtures/data**: minimal inline JSON/YAML values plus existing committed REST/SSE/artifact/trace corpora; include positive and negative cases.
- **Mocks**: none.
- **Contract classification**: Configuration and manifest contracts; Persisted or serialized contracts; Ephemeral diagnostic formats by codec row.
- **Compatibility expectation**: protected behavior/fixture or current-run diagnostic coherence; each parameter must name which.

### 2. `eachLoomspanAttemptOwnsExactlyOneVisibleProviderSendAndTerminalFact`
- **Type**: integration characterization
- **Location**: extend `ModelAttemptCallAdvisorIntegrationTest.java`
- **What it proves**: prepared/sent/received-or-failed facts, provider quota, metrics, usage, retry sequence, and endpoint count agree exactly for success, retry, exhaustion, interruption-before-send, and interruption-during-backoff.
- **Fixtures/data**: local deterministic chat model/MockWebServer responses; retryable and terminal failures.
- **Mocks**: fake clock/sleeper and real local HTTP counter where protocol behavior matters.
- **Contract classification**: Configuration and manifest contracts plus Ephemeral diagnostic formats.
- **Compatibility expectation**: protected Loomspan attempt semantics/current-run trace coherence.

For the Spring AI 1 baseline's provider-owned tool loop, separately characterize the known containment defect: two semantic calls with one explicit provider-requested tool call each produce four HTTP sends but only two outer Loomspan attempt facts. Phase 5 replaces that behavior and owns the failing/passing assertion that all four AI 2 advisor-loop model turns are physical attempts.

### 3. `portableTraceCorpusRetainsExactCompatibilityMarkerAndGoAdmission`
- **Type**: cross-language contract integration
- **Location**: extend `ConsoleTraceFixtureCorpusTest.java` and Go `internal/traceanalysis/processor_test.go`
- **What it proves**: generated canonical fixtures remain byte-for-byte current; resolved releases require exact `consoleCompatibilityVersion`; matching development is best-effort only; missing/unequal markers fail without fallback.
- **Fixtures/data**: committed valid/invalid trace corpus including portable-import cases.
- **Mocks**: none.
- **Contract classification**: Persisted or serialized contract for the complete same-version file; Ephemeral diagnostic format for installed evidence.
- **Compatibility expectation**: protected exact same-version path; no historical compatibility.

### 4. `openRouterErrorCompletionNeverReturnsPartialContent`
- **Type**: HTTP integration characterization
- **Location**: extend `ConnectionProtocolTest.java` and `ModelAttemptCallAdvisorIntegrationTest.java`
- **What it proves**: opt-in OpenRouter profile rejects HTTP-200 `finish_reason:error`, bounds diagnostics at 1 MiB, closes the response, and either retries or terminates without exposing partial assistant text.
- **Fixtures/data**: small, exactly-at-limit, and over-limit error bodies; success following retry.
- **Mocks**: MockWebServer.
- **Contract classification**: Configuration and manifest contracts; Ephemeral diagnostic formats.
- **Compatibility expectation**: protected operational semantics/security.

## Phase 2: Boot 4 and Jackson 3

### 5. `jackson3PurposeOwnedCodecsPreserveRetainedContracts`
- **Type**: unit/contract
- **Location**: new `internal/serialization/LoomspanJacksonCodecsTest.java`
- **What it proves**: each named codec has only its intended settings and passes the Phase 1 risk matrix; strict observability and YAML unknown-field rules do not bleed into ordinary application conversion.
- **Fixtures/data**: Phase 1 cases reused through codec-specific test sources.
- **Mocks**: Boot `ApplicationContextRunner` for injected application mapper; no mocked mapper behavior.
- **Contract classification**: Configuration and manifest contracts; Persisted or serialized contracts; Ephemeral diagnostic formats.
- **Compatibility expectation**: protected by role; mapper identity itself is internal.

### 6. `generatedProtocolCorporaRemainByteForByteUnchangedOnJackson3`
- **Type**: integration/corpus
- **Location**: existing `ConsoleRestFixtureCorpusTest`, `ConsoleSseFixtureCorpusTest`, `ConsoleArtifactFixtureCorpusTest`, `ConsoleTraceFixtureCorpusTest`
- **What it proves**: Boot/Jackson migration creates no implicit Java-to-Go/TypeScript protocol delta or marker change.
- **Fixtures/data**: all committed corpora.
- **Mocks**: existing deterministic producers only.
- **Contract classification**: Persisted or serialized contracts.
- **Compatibility expectation**: protected exact output. A diff is a stop condition, not an automatic fixture update.

### 7. `boot4WebConditionsPreserveMvcSecurityAndNoMvcBehavior`
- **Type**: Spring integration
- **Location**: port/extend `ObservabilityRestIntegrationTest`, `ObservabilitySseIntegrationTest`, `ObservabilityWithoutMvcIntegrationTest`
- **What it proves**: Boot 4 modular imports preserve authentication, exact release identity, no-store, route/method/accept/query behavior, async SSE ownership/capacity/cleanup, and clean absence without MVC.
- **Fixtures/data**: existing application context and MockMvc/SSE fixtures.
- **Mocks**: repository fakes for registries/activity; real Boot test contexts.
- **Contract classification**: Persisted or serialized contracts and Internal composition.
- **Compatibility expectation**: protected web semantics; auto-configuration package identity is approved internal change.

### 8. `loomspanSourceUsesJackson3ApisOnly`
- **Type**: architecture/build gate
- **Location**: new `architecture/JacksonBoundaryArchitectureTest.java` plus Maven dependency audit command
- **What it proves**: Loomspan production/test helpers do not import Jackson 2 databind/core/dataformat APIs; annotations remain allowed; no Boot compatibility module/direct Jackson 2 dependency exists.
- **Fixtures/data**: compiled classes and POM dependency tree.
- **Mocks**: none.
- **Contract classification**: Internal implementation.
- **Compatibility expectation**: approved removal; SDK-private transitive Jackson 2 is explicitly allowed and must not be used by Loomspan code.

## Phase 3: Neutral Model and Capability Boundaries

### 9. `springAiTypesAreConfinedToTheIntegrationSubsystem`
- **Type**: architecture
- **Location**: `SpringAiBoundaryArchitectureTest.java`
- **What it proves**: the primary failing-test-first boundary, including recursive public signature/generic exposure.
- **Fixtures/data**: compiled production classes.
- **Mocks**: none.
- **Contract classification**: Internal or accidentally exposed implementation.
- **Compatibility expectation**: approved removal/containment.

### 10. `modelInteractionRequestsAndResultsAreImmutableAndProviderNeutral`
- **Type**: unit/API-shape
- **Location**: new `internal/model/ModelInteractionContractTest.java`
- **What it proves**: request/result/mode/factory types validate nulls, defensively copy capability/context collections, carry attachments/trace context/planning flag, normalize nullable content, and expose no Spring AI types.
- **Fixtures/data**: mutable source maps/lists changed after construction; standard and step modes.
- **Mocks**: `FakeModelInteraction`, not a Spring AI client double.
- **Contract classification**: Internal implementation.
- **Compatibility expectation**: one coherent new internal boundary; no facade growth.

### 11. `providerAndStepLoopInvocationShareOneLifecycleContract`
- **Type**: parameterized unit/integration
- **Location**: new `internal/runtime/tool/DefaultCapabilityInvokerTest.java`
- **What it proves**: provider-originated unbound calls and explicitly bound step-loop calls share authorization, quota, frame, router, metrics, result/failure facts, while plan linking/completion happens exactly once in the appropriate owner.
- **Fixtures/data**: planned match, unplanned call, bound task, nested YAML capability, mapped Java capability, null arguments, runtime exception, `Error`, interruption.
- **Mocks**: focused fakes for planning/state/usage/metrics/router with ordered event capture; no Spring AI callbacks.
- **Contract classification**: Internal implementation plus Ephemeral diagnostic coherence.
- **Compatibility expectation**: retained Loomspan semantics; callback representation removed.

### 12. `missionPlanningAndStepExecutionUseOnlyModelInteraction`
- **Type**: unit regression suite
- **Location**: port `MissionExecutionEngineTest`, `PlanningServiceTest`, `StepLoopMissionExecutionEngineTest`, coordinator/validator tests
- **What it proves**: all existing direct/planning/step behavior works through `FakeModelInteraction`; response context still links planning/attempt trace facts; attachments and timeouts remain correct.
- **Fixtures/data**: existing scenarios and prompts; scripted fake results/errors/blocking calls.
- **Mocks**: `FakeModelInteraction` and fake `CapabilityInvoker`; delete fluent `ChatClient` doubles.
- **Contract classification**: Internal implementation; Configuration/diagnostic behavior where existing semantics are retained.
- **Compatibility expectation**: approved internal replacement with retained orchestration behavior.

### 13. `capabilityDescriptorsUseNeutralSchemaGeneration`
- **Type**: unit/architecture
- **Location**: update descriptor/input-contract/schema tests
- **What it proves**: generic and reflected schemas remain structurally equivalent and valid without Spring AI schema utilities; descriptions/requiredness/nested/resource shapes remain correct.
- **Fixtures/data**: maps, records, optional parameters, nested resources, strict empty object, additional properties.
- **Mocks**: none.
- **Contract classification**: Application API author semantics and Internal representation.
- **Compatibility expectation**: protected author-facing schemas; internal generator removed.

## Phases 4-5: Providers, Options, Tool Loop, Attempts, Usage, and Observations

### 14. `eachProviderPerformsOneHttpSendPerLoomspanAttempt`
- **Type**: parameterized HTTP integration
- **Location**: new `internal/springai/ProviderHttpAttemptIntegrationTest.java`
- **What it proves**: OpenAI and Anthropic SDK `maxRetries(0)` and Gemini/Ollama Framework `RetryPolicy.withMaxRetries(0)` each issue exactly one request for one Loomspan attempt on retryable failure.
- **Fixtures/data**: provider-shaped 429/5xx and success responses; endpoint request capture for all four drivers.
- **Mocks**: MockWebServer/local HTTP server only; use real target model/client construction.
- **Contract classification**: Configuration and manifest contracts plus Internal provider implementation.
- **Compatibility expectation**: protected one-send semantics.

### 15. `loomspanRetryCountsMatchSendsQuotasMetricsAndTraceFactsForEveryProvider`
- **Type**: parameterized end-to-end integration
- **Location**: port/extend `ModelAttemptCallAdvisorIntegrationTest.java`
- **What it proves**: for 1, 2, and exhausted attempts, endpoint sends equal provider-attempt quota reservations, attempt numbers, prepared/sent/terminal facts, metric increments, and returned-response usage; there is one terminal failure category.
- **Fixtures/data**: retryable then success, terminal 4xx, repeated 429, `Retry-After` numeric/date, timeout/connectivity, cancellation.
- **Mocks**: MockWebServer and deterministic sleeper/clock; real advisor/provider chain.
- **Contract classification**: Configuration contract and Ephemeral diagnostic formats.
- **Compatibility expectation**: protected operational semantics/current trace.

### 16. `officialClientsComposeSupportedPathsIdentityAndHeaders`
- **Type**: HTTP integration
- **Location**: replace cases in `ConnectionProtocolTest.java` with target `internal/springai/ProviderProtocolIntegrationTest.java`
- **What it proves**: OpenAI `/chat/completions`, Anthropic `/v1/messages`, base URLs, organization/project, general headers, Anthropic beta headers, Gemini modes, and Ollama `/api/chat` are translated exactly as approved without obsolete path/version properties.
- **Fixtures/data**: base URLs with/without trailing/version paths and captured headers; never assert secret values in failure output.
- **Mocks**: MockWebServer.
- **Contract classification**: Configuration and manifest contracts.
- **Compatibility expectation**: surviving fields protected; removed fields intentionally absent.

### 17. `openRouterGuardRejectsErrorCompletionBeforeSdkDecode`
- **Type**: HTTP integration
- **Location**: target provider protocol/integration tests
- **What it proves**: Phase 1 OpenRouter behavior on the official SDK path, including no partial success, bounded valid UTF-8 diagnostics, response closure, retry classification, and profile opt-in.
- **Fixtures/data**: success, unknown finish reason, `finish_reason:error`, malformed/large body, profile disabled.
- **Mocks**: MockWebServer with connection reuse/close assertions where available.
- **Contract classification**: Configuration contract and Ephemeral diagnostics.
- **Compatibility expectation**: protected behavior/security.

### 18. `providerOptionContributorsApplyImmutableTargetOptions`
- **Type**: unit/integration
- **Location**: new `internal/springai/SpringAiChatOptionsContributorTest.java`
- **What it proves**: provider model, GPT-5 temperature default, reasoning effort, Anthropic/Gemini thinking budgets, and Ollama model are applied through AI 2 builders without mutating shared defaults or accepting AI 1 built values.
- **Fixtures/data**: all drivers, null/allowed thinking levels, reused connection with distinct model aliases.
- **Mocks**: inspect built request options through a recording model boundary, not fluent client interfaces.
- **Contract classification**: Configuration and manifest contracts; Internal implementation.
- **Compatibility expectation**: retained model/thinking semantics; old adapter removed.

### 19. `semanticRetriesWrapToolLoopAndPhysicalAttemptsRunInsideIt`
- **Type**: full advisor integration
- **Location**: new `internal/springai/AdvisorRecursionIntegrationTest.java`
- **What it proves**: two semantic attempts, each with one tool call, produce exactly four model turns, four inner physical-attempt observations/sends, and two tool executions; semantic validators execute twice, not four times; tool results are not duplicated.
- **Fixtures/data**: scripted real AI 2 advisor/model exchange: tool-call response then final invalid response on each semantic attempt, with final passing response as appropriate.
- **Mocks**: deterministic recording `ChatModel` or local protocol server behind real `ChatClient`/advisors; real `ToolCallingAdvisor`, Loomspan semantic and attempt advisors.
- **Contract classification**: Internal implementation plus Ephemeral diagnostic formats.
- **Compatibility expectation**: target operational invariant.

### 20. `clientAssemblyContainsExactlyOneToolCallingAdvisor`
- **Type**: integration/behavioral
- **Location**: `AdvisorRecursionIntegrationTest.java` and `SpringAiModelInteractionFactoryTest.java`
- **What it proves**: auto/explicit registration cannot create zero or duplicate tool loops; standard mode includes semantic advisors while step mode omits only final-response validators.
- **Fixtures/data**: tool surface empty/nonempty, standard/step mode, custom observation registry.
- **Mocks**: recording model and capability invoker; assert behavior/counts rather than private advisor list only.
- **Contract classification**: Internal implementation.
- **Compatibility expectation**: one coherent target behavior.

### 21. `toolAndSemanticFailuresPreserveCancellationFailureAndTraceOwnership`
- **Type**: integration
- **Location**: advisor recursion, mission, step-loop, and trace contract tests
- **What it proves**: tool exception, semantic exhaustion, provider exhaustion, interruption before send/backoff/tool turn, mission timeout, and attachment failure close the correct frames, preserve interrupt, create no phantom attempt/tool terminal, and link one terminal failure.
- **Fixtures/data**: scripted failures at each recursion boundary.
- **Mocks**: blocking/cancellable recording model and capability invoker; real execution state/trace projector where possible.
- **Contract classification**: Ephemeral diagnostic formats and Internal execution.
- **Compatibility expectation**: current-run accuracy/failure visibility.

### 22. `springAiObservationsAreSafeAndDoNotDuplicateLoomspanAccounting`
- **Type**: Micrometer integration
- **Location**: new `internal/springai/SpringAiObservationIntegrationTest.java`; extend `MicrometerUsageMetricsRecorderTest.java`
- **What it proves**: conventional chat/model/tool/HTTP observations propagate through the selected registry; prompt, completion, tool arguments, API keys, header values, base URLs, and provider bodies are absent by default; Loomspan counters/attempt facts increment exactly once.
- **Fixtures/data**: one success, tool call, provider retry, semantic retry, and failure; secret canaries in every sensitive source.
- **Mocks**: `SimpleMeterRegistry`/`TestObservationRegistry`, MockWebServer, real advisor chain.
- **Contract classification**: Ephemeral diagnostic/operational formats and Internal implementation.
- **Compatibility expectation**: additive safe observations; no duplicate Loomspan semantics.

### 23. `providerResourcesAreReusedAtConnectionScopeAndClosedAtContextShutdown`
- **Type**: lifecycle integration
- **Location**: new `internal/springai/ProviderClientLifecycleIntegrationTest.java`
- **What it proves**: HTTP/provider model resources are not rebuilt per model turn, mission-scoped callbacks do not leak across sessions, and owned resources close once when the application context shuts down or construction fails.
- **Fixtures/data**: multiple aliases/skills on one connection, two sessions, failed second connection construction, context close.
- **Mocks**: counting SDK/HTTP client factory seam internal to tests or MockWebServer connection observations; no supported replacement bean.
- **Contract classification**: Internal implementation.
- **Compatibility expectation**: target lifecycle/performance correctness.

## Phase 6: Application API, Configuration, Auto-Configuration, Samples, and Guidance

### 24. `apiPackageContainsExactlyEightApprovedPublicTypes`
- **Type**: architecture
- **Location**: update `LoomspanPublicSurfaceArchitectureTest.java`
- **What it proves**: the seven existing API types plus `SkillParam` are the only public application API; neutral/internal/Spring types do not leak into their signatures.
- **Fixtures/data**: compiled classes/reflection.
- **Mocks**: none.
- **Contract classification**: Application API.
- **Compatibility expectation**: approved additive API plus intentional external annotation replacement.

### 25. `skillParamIsTheOnlySupportedParameterContract`
- **Type**: unit/integration matrix
- **Location**: update `SkillMethodBeanPostProcessorTest.java` and `SkillMethodTargetDiscoveryIntegrationTests.java`; add `SkillParamTest.java`
- **What it proves**: runtime retention/parameter target, default/explicit description and requiredness, implementation/interface precedence, equivalent/conflicting contracts, JDK/CGLIB proxies, generic bridges, parameter names, nested schemas, binding, and invocation all use `SkillParam`; Spring AI `ToolParam` is absent.
- **Fixtures/data**: port all current annotation cases plus blank description, optional parameter, conflicting interface contracts, resource/nested record parameters.
- **Mocks**: real Spring test contexts for proxy discovery; no Spring AI annotation.
- **Contract classification**: Application API.
- **Compatibility expectation**: approved source break; no dual annotation behavior.

### 26. `removedProviderPropertiesAreRejectedAndSurvivingSurfaceIsDocumentedExactly`
- **Type**: configuration binding/metadata
- **Location**: update `LoomspanPropertiesTest.java`, `ConfigurationMetadataTest.java`
- **What it proves**: the four removed property paths are absent from metadata and fail strict startup binding with full paths; surviving common/OpenAI/Gemini/retry fields bind; headers are accepted for OpenAI/Anthropic and rejected for Gemini/Ollama.
- **Fixtures/data**: property maps for every driver and removed field, sensitive canaries, invalid cross-driver blocks.
- **Mocks**: Boot binder/application context.
- **Contract classification**: Configuration and manifest contracts.
- **Compatibility expectation**: explicit atomic break and protected surviving surface.

### 27. `splitAutoConfigurationOwnsAllBeansWithoutCreatingReplacementSpi`
- **Type**: Spring/architecture integration
- **Location**: update auto-configuration tests and `LoomspanAutoConfigurationBoundaryTest.java`
- **What it proves**: core/AI/Jackson configurations load in correct order, each bean has one owner, methods remain package-private, override allowlist remains empty, and no `@ConditionalOnMissingBean` appears.
- **Fixtures/data**: non-web, MVC, missing provider, multiple connections, observation registry absent/present.
- **Mocks**: `ApplicationContextRunner` with real configurations.
- **Contract classification**: Supported SPI (absence) and Internal implementation.
- **Compatibility expectation**: no new SPI; internal split allowed.

### 28. `supportedSurfaceInvokesMappedSkillThroughSkillParamOnBoot4`
- **Type**: supported-surface integration
- **Location**: update `SupportedSurfaceIntegrationTest.java` and sample smoke tests
- **What it proves**: an application configures a named connection, invokes an LLM-backed YAML entry through `SkillTemplate`, and reaches a mapped `@SkillMethod`/`@SkillParam` leaf without internal bean replacement.
- **Fixtures/data**: tested YAML skill/sample method/local provider fixture.
- **Mocks**: local HTTP model endpoint; real Boot 4 application context.
- **Contract classification**: Application API and Configuration/manifest contracts.
- **Compatibility expectation**: protected application entry behavior with approved annotation change.

### 29. `skillAuthoringClaimsMatchExecutableBehavior`
- **Type**: evidence mapping/checklist enforced through focused tests, not prose parsing
- **Location**: evidence references recorded in the updated docs and checkpoint review; underlying tests 15, 19, 22, 25, 26, and portable corpus test
- **What it proves**: every material new guidance claim has a named behavioral test/sample/source anchor; README routing/coverage changes only if topic boundary/confidence changes.
- **Fixtures/data**: updated docs plus cited tests and sample config.
- **Mocks**: none.
- **Contract classification**: Application API, Configuration contracts, Ephemeral diagnostics.
- **Compatibility expectation**: author-facing guidance matches the coherent target only.

## Phase 7: Removal and Atomic Release Gates

### 30. `obsoletePlatformTypesAndExtensionPathsAreAbsent`
- **Type**: architecture/source/dependency gate
- **Location**: architecture tests plus verification script/commands
- **What it proves**: no `internal.springai.v1_1`, `SpringAiV11`, `SkillChatClientFactory`, `SkillChatOptionsAdapter`, old callback factory/wrappers, `ToolParam`, Spring Retry, Loomspan Jackson 2 API, removed metadata/property/docs, or fluent client fake remains; no compatibility module/bridge exists.
- **Fixtures/data**: compiled classes, repository search, dependency tree.
- **Mocks**: none.
- **Contract classification**: Internal or accidentally exposed implementation plus approved Application/configuration break.
- **Compatibility expectation**: approved removal; simultaneous old/new behavior is a failure.

### 31. `completeReactorAndConsoleCorpusRemainCoherent`
- **Type**: full build/cross-language/E2E gate
- **Location**: all modules and Console suites
- **What it proves**: one atomic Boot 4/AI 2/Jackson 3 state passes Java, Go, TypeScript, fixtures, sample packaging, and protocol tests together.
- **Fixtures/data**: complete repository corpus.
- **Mocks**: deterministic provider fixtures for automated gates; live providers only for manual checks.
- **Contract classification**: all affected categories.
- **Compatibility expectation**: final target state only.

### 32. `portableImportRemainsIndependentOfTargetLifecycle`
- **Type**: Go/browser/frontend regression
- **Location**: existing artifact import/browser API/Trace Explorer/diagnostic tests
- **What it proves**: no-target import succeeds for matching traces; imported errors omit owner/target scope; target rotation does not refetch/reset imported evidence; imported state remains process-local and is not adopted after restart.
- **Fixtures/data**: current valid/mismatched portable trace fixtures and opaque owner IDs.
- **Mocks**: existing Go services and frontend API mocks.
- **Contract classification**: Persisted complete trace plus Ephemeral installed evidence.
- **Compatibility expectation**: preserve PR 25 behavior/security.

## Checkpoint Test and Review Gates

| Pause | Required green test scope before review | Evidence passed to fresh review context |
| --- | --- | --- |
| After Phase 1 | Starter full baseline; characterization/provider-count/OpenRouter/codec/corpus tests; focused Go corpus/import tests | Exact Git range, command log, new fixture rationale, retained-contract map, known target-red tests not yet introduced |
| After Phase 2 | Target compile; Jackson codec matrix; all Java corpora; Boot MVC/SSE/no-MVC tests; Jackson/dependency gates; focused Go corpus | Git range, dependency tree, fixture diff (expected empty), compile/test log, any SDK-private Jackson 2 exception |
| After Phase 3 | Spring AI boundary red-to-green evidence; model contract; capability lifecycle; all ported mission/planning/step/tool/trace tests | Git range, initial architecture failures, green architecture results, deleted doubles inventory, lifecycle count assertions |
| After Phase 5 for Phases 4-5 | All provider protocol/count/retry/OpenRouter/options/advisor/usage/observation/lifecycle tests plus orchestration regressions | Combined Phase 4-5 range, captured provider requests, exact count table, recursion trace, sensitive-canary results, residual live-provider limits |
| After Phase 6 | API/config/metadata/auto-configuration/supported-surface/sample tests; docs evidence map and residue scan limited to maintained files | Git range, public-surface diff, property migration matrix, metadata output, sample result, routed-doc review evidence |
| After Phase 7 | Clean full reactor, full Go/frontend suites, corpora, dependency/source scans, manual results available, `git diff --check` | Merge-base-to-HEAD range, all checkpoint closures, full command log, manual matrix, residual risks; final reviewer reruns independently |

No implementation phase may resume until the implementation plan's review pause policy is satisfied. Test success does not substitute for review disposition, and checkpoint approval does not substitute for the final cumulative test/review gate.

## How to Run

Run commands from `C:\opendev\code\loomspan` unless a command changes directory. PowerShell syntax is shown.

### Baseline and focused Java tests

```powershell
.\mvnw.cmd -pl loomspan-spring-boot-starter test -DfailIfNoTests=false
.\mvnw.cmd -pl loomspan-spring-boot-starter -Dtest=LoomspanPublicSurfaceArchitectureTest,LoomspanAutoConfigurationBoundaryTest,SpringAiBoundaryArchitectureTest,JacksonBoundaryArchitectureTest test
.\mvnw.cmd -pl loomspan-spring-boot-starter '-Dtest=ObservabilityJsonCodecContractTest,YamlSkillCatalogTests,PlanningServiceTest,NdjsonTraceRecordWriterTest,NdjsonExecutionTraceReaderTest,LoomspanJacksonCodecsTest,ConsoleRestFixtureCorpusTest,ConsoleSseFixtureCorpusTest,ConsoleArtifactFixtureCorpusTest,ConsoleTraceFixtureCorpusTest' test
.\mvnw.cmd -pl loomspan-spring-boot-starter -Dtest=ProviderHttpAttemptIntegrationTest,ProviderProtocolIntegrationTest,ModelAttemptCallAdvisorIntegrationTest,AdvisorRecursionIntegrationTest,SpringAiObservationIntegrationTest test
.\mvnw.cmd -pl loomspan-spring-boot-starter -Dtest=ModelInteractionContractTest,DefaultCapabilityInvokerTest,MissionExecutionEngineTest,PlanningServiceTest,StepLoopMissionExecutionEngineTest,ExecutionTraceContractTest test
.\mvnw.cmd -pl loomspan-spring-boot-starter -Dtest=SkillParamTest,SkillMethodBeanPostProcessorTest,SkillMethodTargetDiscoveryIntegrationTests,LoomspanPropertiesTest,ConfigurationMetadataTest,SupportedSurfaceIntegrationTest test
```

Test selectors referring to new classes become runnable in their owning phase. Before creation, use the existing class names listed under Existing Test Coverage.

### Full Java and dependency gates

```powershell
.\mvnw.cmd clean verify
.\mvnw.cmd -pl loomspan-sample -am verify
.\mvnw.cmd -pl loomspan-spring-boot-starter dependency:tree -Dverbose
```

Review the dependency tree for:

- Boot 4.1.0 and Spring AI 2.0.0 alignment;
- no direct Loomspan Jackson 2 dependency;
- no Boot Jackson 2 compatibility module;
- no Spring Retry dependency used by Loomspan;
- any Jackson 2 jars are reachable only through an official provider SDK private codec.

### Source and residue gates

```powershell
rg "org\.springframework\.ai" loomspan-spring-boot-starter/src/main/java
rg "com\.fasterxml\.jackson\.(core|databind|dataformat)" loomspan-spring-boot-starter/src
rg "ToolParam|SpringAiV11|internal\.springai\.v1_1|SkillChatClientFactory|SkillChatOptionsAdapter|org\.springframework\.retry" loomspan-spring-boot-starter loomspan-sample README.md ai/skill-authoring
rg "chat-completions-path|anthropic\.(completions-path|version|beta-version)" loomspan-spring-boot-starter loomspan-sample README.md ai/skill-authoring
```

Interpretation:

- Spring AI imports may appear only in the approved integration/adapter/advisor/AI auto-configuration locations enforced by architecture tests.
- `com.fasterxml.jackson.annotation` imports are allowed; Jackson 2 databind/core/dataformat APIs are not.
- Historical ticket/research/plan documents may describe removed names. Maintained production, tests, samples, generated metadata, root/sample README, and `ai/skill-authoring` guidance must not.

### Console and cross-language gates

```powershell
Push-Location loomspan-console
go test ./internal/applicationclient ./internal/traceanalysis ./internal/artifact ./internal/browserapi ./internal/console
go test ./...
Pop-Location

Push-Location loomspan-console/web
npm run typecheck
npm test -- --run
Pop-Location
```

If the repository's Console verification instructions require race/E2E gates in the implementation checkout, add:

```powershell
Push-Location loomspan-console
go test -race ./...
Pop-Location
```

Run browser E2E using the repository's existing command discovered in that checkout; do not invent a new runner or silently mark it passed when unavailable.

### Diff hygiene

```powershell
git diff --check
git status --short
```

## Test Environment and Fixtures

- **Java**: Java 21 or newer, Maven wrapper, no external provider credentials for automated tests.
- **Provider HTTP tests**: MockWebServer/local loopback endpoints with provider-shaped requests/responses. Disable all SDK/framework retries beneath Loomspan and use short deterministic timeouts.
- **Time/retry tests**: inject deterministic clock/sleeper/random jitter sources where existing seams permit; never use long real sleeps.
- **Observation tests**: `SimpleMeterRegistry` or Micrometer test registry plus a test observation registry; put unique secret canaries in prompts, completions, arguments, keys, headers, URLs, and bodies and assert absence.
- **Serialization fixtures**: committed corpus remains authoritative. New minimal fixtures must state their codec role and protected semantics.
- **Go/TypeScript**: use committed trace/application corpora and existing frontend mocks. No live Java target is required for unit/corpus gates.
- **Live manual providers**: optional credentials/endpoints supplied externally and never committed. Record which of OpenAI/OpenRouter, Anthropic, Gemini API-key/Vertex, and Ollama were actually exercised; unavailable providers remain a declared residual risk, not an automated-test waiver.

## Manual Verification Matrix

| Scenario | Expected evidence |
| --- | --- |
| Boot 4 sample startup | Configuration binds, generated metadata matches, MVC/security routes initialize, no old-property warnings/fallbacks. |
| Direct skill with mapped Java leaf | `SkillTemplate` entry works; `SkillParam` descriptions/requiredness reach the provider schema; one tool lifecycle and correct result. |
| Planning/step skill | Planning prompt sees neutral descriptors; bound tool executes once; plan/frame/trace ownership is correct. |
| Semantic retry plus tool loop | Trace shows semantic attempts outside tool turns, physical attempts inside, no repeated completed tool work where policy forbids it. |
| Provider retry | Actual endpoint sends equal quota/attempt facts and terminal classification; no SDK multiplier. |
| OpenRouter HTTP-200 error | Partial content never succeeds; diagnostic is bounded; retry/terminal behavior is visible. |
| Spring observations | Conventional spans/meters exist; Loomspan counters remain single; sensitive canaries absent. |
| REST/SSE | Authentication, no-store, exact release identity, stream replay/capacity/cleanup, and problem responses remain current. |
| Trace save/import with no target | Matching complete file opens; incompatible marker fails; imported evidence is labeled/transient. |
| Target rotation while imported trace open | No imported refetch/reset; loaded diagnostics remain; errors reveal no imported owner or target scope. |
| Context shutdown | Provider resources close once; no session callbacks/credentials leak into subsequent contexts. |

## Exit Criteria

### Phase and checkpoint criteria

- [ ] Phase 1 characterization passes on the unmodified Boot 3/AI 1 baseline and contains no target implementation assumptions.
- [ ] Every phase red gate is captured failing for the intended reason before its implementation and passes afterward.
- [ ] The mandatory reviews after Phases 1, 2, 3, combined 4-5, 6, and 7 receive the required test evidence.
- [ ] No next phase begins while its prior review is outstanding or has unresolved P0-P2 findings, unless the user explicitly records another disposition.
- [ ] Fixes from checkpoint reviews receive focused regression tests when the finding represents a repeatable defect, and all affected gates are rerun.

### Automated correctness criteria

- [ ] Full reactor passes on Boot 4.1.0/Spring AI 2.0.0/Java 21/Jackson 3: `.\mvnw.cmd clean verify`.
- [ ] Sample packages with the reactor: `.\mvnw.cmd -pl loomspan-sample -am verify`.
- [ ] All four providers prove one physical HTTP send per Loomspan attempt and exact multi-attempt counts.
- [ ] OpenRouter error completions never surface partial success and diagnostics remain bounded/safe.
- [ ] Advisor recursion proves semantic policy outside one tool loop and physical attempts inside it with exact counts.
- [ ] Direct, planning, step, nested, attachment, timeout, cancellation, quota, tool failure, semantic failure, and terminal trace paths pass through the neutral boundary.
- [ ] Capability invocation lifecycle is equivalent for provider and step-loop entry without duplicate accounting or plan updates.
- [ ] Jackson 3 codec matrix and exact REST/SSE/artifact/NDJSON corpora pass without fixture or compatibility-marker changes.
- [ ] Boot 4 MVC/security/no-MVC and supported-surface integration tests pass.
- [ ] Spring observations propagate safely and do not duplicate Loomspan counters/traces/quotas.
- [ ] Go backend, full Go suite, frontend typecheck, and frontend tests pass.
- [ ] Architecture, dependency, source, and residue gates pass.

### Contract and documentation criteria

- [ ] Existing seven application API types retain protected behavior; `SkillParam` is the eighth allowlisted type and the only supported parameter contract.
- [ ] No supported SPI or conditional replacement bean is introduced.
- [ ] YAML manifests and surviving named connection/model fields retain tested behavior.
- [ ] Removed provider fields and `ToolParam` are absent and rejected/unsupported, not retained behind aliases, fallbacks, or dual behavior.
- [ ] Complete same-version portable trace and REST/SSE/artifact/problem consumers remain coherent; installed imported state remains ephemeral.
- [ ] Current-run traces preserve ordering, diagnostic usefulness, failure visibility, redaction boundaries, bounded retrieval, tool lifecycle, and usage reconciliation.
- [ ] Updated `mental-model.md`, `model-selection-and-connections.md`, and `traces-and-debugging.md` claims are supported by the named focused tests, sample, configuration metadata, and production paths.
- [ ] Skill-authoring README routing/coverage is changed only if its topic boundary or confidence actually changes.

### Manual and final review criteria

- [ ] Available live-provider scenarios are completed and unavailable ones are named as residual risk.
- [ ] Boot 4 sample, representative direct/planned/tool/retry/failure flows, observations, REST/SSE, trace save/import, and target-rotation checks are completed.
- [ ] `git diff --check` passes and the worktree scope contains no unrelated PR 16-20 changes.
- [ ] The final separate-context `5_code_review.md` review independently covers merge-base-to-HEAD, reruns appropriate verification, and reaches `Approve`, or `Approve with follow-ups` only with explicit user acceptance.
- [ ] The PR is treated as one atomic supported platform state; intermediate checkpoint commits are not described or released as supported combinations.

## References

- Ticket: `ai/thoughts/tickets/loomspan-platform-pr-26-spring-boot-4-spring-ai-2.md`
- Implementation plan: `ai/thoughts/plans/2026-08-13-PR-26-spring-boot-4-spring-ai-2.md`
- Codebase map: `ai/thoughts/research/2026-08-12-PR-26-spring-boot-4-spring-ai-2-codebase-map.md`
- Platform research: `ai/thoughts/research/2026-08-12-spring-platform-upgrade-research.md`
- Contract policy: `ai/thoughts/framework-feature-design-lens.md`
- Skill-authoring routing/evidence: `ai/skill-authoring/README.md`, `source-verification.md`, `mental-model.md`, `model-selection-and-connections.md`, `traces-and-debugging.md`
- Portable trace review task: `thread://019ff924-677b-7ab1-bf83-f30750b678e3`
