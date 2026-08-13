# PR 26 Spring Boot 4 / Spring AI 2 Platform Migration Implementation Plan

## Overview

Migrate the two-module Loomspan reactor atomically from Spring Boot 3.5.11 and Spring AI 1.1.6 to Spring Boot 4.1.0, Spring Framework 7, Spring AI 2.0.0, Java 21, and Jackson 3. At the same time, replace the porous Spring AI 1 integration with one narrow, session-bound model-interaction boundary, adopt Spring AI 2's tool loop and official provider clients, preserve Loomspan's retry/validation/trace semantics, and remove obsolete compatibility surface.

The implementation remains one PR with seven reviewable milestones. Intermediate commits are not supported platform states and must not be merged independently.

## Current State Analysis

- The parent reactor pins Boot 3.5.11 and Spring AI 1.1.6; the starter directly declares four Spring AI providers and Jackson 2, and both modules still use the Boot 3 `spring-boot-starter-web` name (`pom.xml:45-73`, `loomspan-spring-boot-starter/pom.xml:19-103`, `loomspan-sample/pom.xml:19-45`).
- Thirty-one production files import Spring AI. `ExecutionCoordinator` constructs a `ChatClient` and `List<ToolCallback>` and passes them through mission, planning, attachment, and step-loop services (`ExecutionCoordinator.java:31-43`, `ExecutionCoordinator.java:110-128`). Ten tests reproduce Spring AI's fluent client interfaces as handwritten doubles.
- Provider construction is concentrated in the versioned `internal.springai.v1_1.SpringAiV11ProviderIntegration`. The removed OpenAI/Anthropic REST APIs, Spring Retry, old option values, and Boot 3 MVC imports cause the first target-platform compile failures (`SpringAiV11ProviderIntegration.java:60-161`).
- `ProviderAttemptCallAdvisor` is the authoritative physical-attempt boundary: it reserves quota, emits attempt trace facts, applies Loomspan retry policy, normalizes failures, and accounts usage (`ProviderAttemptCallAdvisor.java:42-102`). Provider-native retry must remain disabled beneath it.
- `DefaultToolCallbackFactory` currently combines a Spring AI callback adapter with Loomspan-owned capability execution, plan linking, quota, metrics, frames, results, and failures (`DefaultToolCallbackFactory.java:62-184`). Step-loop execution calls those callbacks directly, spreading the provider representation into orchestration.
- Twenty-eight production files use Jackson directly across distinct codec roles. Canonical NDJSON, REST, SSE, artifact, YAML, planning, and validation behavior is protected by focused tests and exact corpora rather than by mapper identity.
- The deliberate application API contains seven types and there is no supported Loomspan SPI or bean override (`LoomspanPublicSurfaceArchitectureTest.java:29-40`, `LoomspanAutoConfigurationBoundaryTest.java:49-114`). Spring AI-facing public internals are technical exposure, not supported contracts.
- The PR 25 review established that a complete canonical trace is a same-version portable persisted artifact, gated by exact `consoleCompatibilityVersion`. Imported owner IDs, handles, catalogs, continuations, indexes, and browser DTOs remain ephemeral and must not be promoted into platform contracts.
- The Boot 3 / AI 1 starter baseline passed 836 tests on research commit `6b1200879abd83ff37847f3e6a5b0cfd0c27c03b`.

## Desired End State

- The reactor builds on Java 21 with Boot 4.1.0, Framework 7, Spring AI 2.0.0, and Jackson 3.
- Mission, planning, step, attachment, and core packages import no Spring AI request, response, client, message, metadata, tool, provider, or option types.
- A session-bound `ModelInteraction` receives Loomspan-owned request data and returns normalized content plus Loomspan attempt context. Its Spring AI implementation privately constructs the `ChatClient`, callbacks, options, advisor chain, and provider models.
- A neutral `CapabilityInvoker` owns tool quota, plan linkage, frames, execution, metrics, results, and failures. Both the Spring AI callback adapter and the step loop delegate to it.
- Exactly one `ToolCallingAdvisor` executes the generic tool loop. Semantic validators wrap it; `ProviderAttemptCallAdvisor` runs inside it; every provider layer performs exactly one HTTP send per Loomspan attempt.
- OpenAI and Anthropic use their official SDK-backed Spring AI 2 models with SDK retries set to zero. Gemini and Ollama use Framework 7 retry templates configured with `RetryPolicy.withMaxRetries(0)`.
- Loomspan code uses Jackson 3 APIs and purpose-owned codecs. No direct Jackson 2 dependency, Loomspan Jackson 2 import, or Boot Jackson 2 compatibility module remains; the OpenAI SDK's private transitive Jackson 2 codec is allowed.
- `@SkillParam(description = ..., required = ...)` replaces application-facing Spring AI `@ToolParam` everywhere in the repository, with no bridge or dual annotation support.
- Named connection properties retain `driver`, `base-url`, `api-key`, `headers`, `provider-retry`, OpenAI organization/project/profile, and existing Gemini modes. OpenAI/Anthropic completion paths and Anthropic typed version/beta properties are removed atomically. Anthropic beta/custom values use the common static-header map.
- Canonical trace, portable NDJSON, REST, SSE, artifact, and problem semantics remain byte/fixture compatible for the same compatibility marker. No Console change or marker bump is planned.
- Spring AI observations are enabled with content export disabled by default and coexist with, rather than replace or duplicate, Loomspan domain metrics, quotas, journal, portable trace, and diagnostics.

### Key Discoveries

- Spring AI 2.0.0 and Boot 4.1.0 are the current stable target releases, and Spring AI 2 explicitly supports Boot 4.0/4.1.
- AI 2's `ToolCallingAdvisor` default order is `Ordered.HIGHEST_PRECEDENCE + 300`; ordering changes whether another advisor runs once per semantic call or once per tool-loop model turn.
- The completed retry probes prove OpenAI/Anthropic SDK `maxRetries=0` and Framework 7 `RetryPolicy.withMaxRetries(0)` each produce one physical send.
- The OpenRouter SDK path can otherwise surface partial content from an HTTP-200 `finish_reason:error`; the existing bounded pre-decode rejection must be ported.
- `ChatClientBuilderConfigurer` only applies global customizers. Using it would silently expand Loomspan's configuration contract, so Loomspan will construct its builders explicitly.
- `CapabilityToolDescriptor` is already close to the needed neutral capability representation, but its generic-schema creation currently imports Spring AI and must move to the Jackson 3 schema boundary (`CapabilityToolDescriptor.java:9-29`).
- Portable trace import protects the complete external NDJSON file, not Console's transient installed representation. Jackson migration must preserve the exact Java-to-Go corpus and compatibility marker.

## What We're NOT Doing

- No Boot 3/4, Spring AI 1/2, or Jackson 2/3 dual support.
- No deprecated overload, annotation alias, legacy reader, fallback mapper, version adapter, or provider-construction bridge.
- No native structured-output migration, tool search, new providers, new mission behavior, new Console capability, or PR 16-20 feature work.
- No general Loomspan facade mirroring Spring AI; neutral types exist only for Loomspan orchestration semantics and dependency isolation.
- No global `ChatClientBuilderConfigurer` customizer inheritance and no new supported bean replacement SPI.
- No change to canonical trace, REST/SSE, acquisition, artifact, problem, or browser contracts unless a retained fixture demonstrates an unavoidable Boot/Jackson conflict. Such a conflict stops implementation for an explicit producer/consumer decision rather than triggering an implicit compatibility path.
- No removal of the provider SDK's private transitive Jackson 2 implementation jars.

## Skill-Authoring Documentation Impact

**Impact**: Affected

- **Rationale**: Skill authors must replace `@ToolParam` with `@SkillParam`; application owners must remove obsolete OpenAI/Anthropic path/version properties; Anthropic beta/custom headers move to the common header map; and retry/trace guidance must accurately describe the AI 2 tool loop without changing Loomspan's one-send attempt semantics.
- **Documents to update**: `ai/skill-authoring/mental-model.md`, `ai/skill-authoring/model-selection-and-connections.md`, and `ai/skill-authoring/traces-and-debugging.md`; also update root `README.md` and `loomspan-sample/README.md` as maintained application documentation.
- **Supporting evidence**: `SkillMethodBeanPostProcessorTest`, `SkillMethodTargetDiscoveryIntegrationTests`, the five sample `@SkillMethod` services, `LoomspanPropertiesTest`, `ConfigurationMetadataTest`, provider request/counting tests, advisor-order integration tests, `ModelAttemptCallAdvisorIntegrationTest`, canonical trace tests, and the Java/Go fixture corpus.
- **Coverage table update**: Not required. The routed topic boundaries and confidence levels remain the same; their current content changes in place. Update `ai/skill-authoring/README.md` only if implementation materially changes a topic boundary or confidence level.
- **LLM-first usability**: Keep the model/connection table authoritative and compact; state removed fields explicitly; show one minimal `@SkillParam` example at the Java boundary; distinguish model interactions, tool-loop turns, semantic retries, and physical sends; retain the exact same-version portability and sensitive-content limitations without duplicating them across topics.

## Contract and Compatibility Impact

| Surface | Classification and supporting evidence | Planned compatibility treatment |
| --- | --- | --- |
| Application API | Preserve the seven allowlisted API types. Add `SkillParam`; remove application dependence on Spring AI `ToolParam`. `SkillTemplate`, execution events, and exception behavior remain protected by architecture/integration/sample evidence. | Intentional atomic source break for annotated Java skill parameters; preserve all other useful API behavior. |
| Supported SPI | No supported SPI exists; empty override allowlist and prohibition of `@ConditionalOnMissingBean` are explicit. | No impact and no new SPI. Keep all new boundary beans framework-owned. |
| Configuration and manifest contracts | YAML syntax/defaults remain. Named connection concepts remain, but strict properties remove OpenAI/Anthropic completion paths and Anthropic version/beta fields; common headers become the escape hatch for supported custom headers. | Explicit pre-1.0 atomic break. Update properties, validation, metadata, sample config, fixtures, tests, and docs together; unknown removed fields fail startup. |
| Persisted or serialized contracts | REST/SSE/artifact/problem corpora and the complete same-version portable NDJSON file have protected Java/Go/TS consumers. | Preserve fixtures and exact compatibility marker. Stop for an explicit lockstep decision if Jackson 3 forces a delta. |
| Ephemeral diagnostic formats | Current-run trace projection, installed imported evidence, handles, indexes, cursors, and browser DTOs remain diagnostic/process-local. | Preserve usefulness, ordering, redaction, bounded diagnostics, and writer/reader coherence; do not promise cross-version readability. |
| Internal or accidentally exposed implementation | Spring AI-facing interfaces, versioned provider package, callback factory representation, private mapper construction, auto-configuration decomposition, exact framework exception text, and fluent client doubles have no supported consumer. | Replace/remove atomically and update all repository callers/tests. No compatibility layer. |

- **Evidence of supported contracts**: API architecture allowlist, no-SPI bean boundary test, root/sample/skill-authoring documentation, strict configuration metadata tests, supported-surface integration test, exact REST/SSE/artifact/NDJSON corpora, Go processor/import tests, and sample consumers.
- **Intended breaks**: add `SkillParam` and remove `ToolParam`; remove `loomspan.connections.*.openai.chat-completions-path`, `*.anthropic.completions-path`, `*.anthropic.version`, and `*.anthropic.beta-version`; replace internal Spring AI-facing types and versioned packages; stop treating exact internal provider/framework messages as contracts.
- **In-repository consumers to update**: all starter production/test imports listed by the research inventories; the five sample Java services; `application.yml`; root/sample/skill-authoring docs; generated metadata and its assertions; architecture allowlists; provider, mission, planning, step, trace, and corpus tests.
- **Public-surface delta**: add public runtime parameter annotation `com.lokiscale.loomspan.api.SkillParam` with `String description() default ""` and `boolean required() default true`; remove no other supported API type. Add internal/public-for-composition auto-configuration and model-boundary types to the technical-exposure allowlist only. No conditional replacement bean is added.
- **Shim decision**: **No shim.** The repository is pre-1.0, all consumers are in scope, `ToolParam` and the removed property fields are deliberately redesigned, and dual annotation/property behavior would retain obsolete Spring AI 1 coupling.
- **Java-to-Go boundary coordination**: **Not required for the planned design.** Retained corpus tests must prove no REST/SSE/artifact/problem/NDJSON delta and no `consoleCompatibilityVersion` change. If any exact fixture changes, implementation stops and the plan/ticket is amended with synchronized Java, Go, TypeScript, fixture, documentation, and marker decisions.

## Implementation Approach

Create a small neutral boundary under `com.lokiscale.loomspan.internal.model`:

- `ModelInteractionFactory#create(LoomspanSession, YamlSkillDefinition, List<CapabilityMetadata>, Authentication, ModelInteractionMode)` returns a mission-scoped `ModelInteraction`.
- `ModelInteractionMode` has `STANDARD` and `STEP_EXECUTION`; the latter omits final-response semantic validators while preserving provider attempts and tool calling.
- `ModelInteraction#execute(ModelInteractionRequest)` performs one logical model interaction. `ModelInteractionRequest` carries the system prompt, `RenderedMissionInput`, `ModelTraceContext`, and a `planningCall` flag. `ModelInteractionResult` carries normalized nullable content and immutable Loomspan request context, including the attempt map used by planning traces.
- `ModelInteraction#capabilities()` exposes immutable neutral `CapabilityMetadata`/`CapabilityToolDescriptor` values for prompt and validation logic; it never exposes executable provider callbacks.
- `CapabilityInvoker#invoke(CapabilityMetadata, Map<String,Object>, LoomspanSession, YamlSkillDefinition, Authentication, String linkedTaskId)` owns the existing Loomspan tool lifecycle. A Spring AI callback adapter and `StepLoopMissionExecutionEngine` both call it.

Implement the boundary under unversioned `com.lokiscale.loomspan.internal.springai` with `SpringAiModelInteractionFactory`, `SpringAiModelInteraction`, `SpringAiToolCallbackAdapter`, `SpringAiProviderIntegration`, and `SpringAiChatOptionsContributor`. The contributor selects a provider-specific AI 2 option builder from `EffectiveSkillExecutionConfiguration`; it does not accept or return a built AI 1 `ChatOptions` value. Delete `internal.springai.v1_1`, `SkillChatClientFactory`, `SkillChatOptionsAdapter`, `ToolCallbackFactory`, `ContractAwareToolCallback`, and Spring-AI-specific message-sender abstractions after all callers migrate.

Split composition into `LoomspanAutoConfiguration` for stable runtime/application services, `LoomspanAiAutoConfiguration` for providers/model interaction/advisors/observations, and `LoomspanJacksonAutoConfiguration` for framework-owned codec roles. These remain internal composition, are registered in `AutoConfiguration.imports`, and do not create replacement seams.

### Independent Review and Pause Policy

Implementation must pause at the review boundaries below. Each review is performed in a separate fresh context with `ai/commands/5_code_review.md`; the implementation context must not review its own work or continue while a checkpoint review is outstanding.

For every checkpoint:

1. Finish the phase's automated verification and record the exact commands/results.
2. Preserve a precise Git review range (start commit and checkpoint commit) that contains only the checkpoint's intended implementation scope. Include staged, unstaged, and untracked files if the checkpoint has not yet been committed.
3. Start a separate review context with the ticket, this implementation plan, the dedicated testing plan, the review range, and the checkpoint scope named below.
4. State explicitly that Phases 1-6 are intermediate, non-mergeable platform states. The reviewer should assess the checkpoint's correctness and its effect on retained contracts without reporting work explicitly assigned to later phases as missing from the final PR.
5. Wait for the review disposition. Do not begin the next implementation phase until every P0-P2 finding is fixed and re-reviewed, or the user explicitly records a different disposition. Resolve P3 findings immediately when they affect later phases; otherwise record their approved final-PR disposition.
6. After fixes, rerun the checkpoint's relevant verification and retain both the original findings and closure evidence for the final cumulative review.

Required checkpoints:

| Boundary | Fresh-context review scope | Resume condition |
| --- | --- | --- |
| After Phase 1 | Characterization quality: retained contracts, exact fixture meaning, provider-send/tool-loop counts, and whether tests fail for the intended regression rather than pinning incidental implementation. | Phase 1 findings closed; characterized contracts are reliable enough to guide migration. |
| After Phase 2 | Boot 4/Jackson 3 dependency and module changes, codec ownership, exact JSON/YAML/NDJSON/REST/SSE/artifact behavior, and absence of an implicit Console protocol change. | Phase 2 findings closed; retained fixtures and dependency boundaries are green. |
| After Phase 3 | Neutral model-interaction and capability-invocation architecture, Spring AI containment, lifecycle/immutability, test-double replacement, and preservation of tool accounting/trace semantics. | Phase 3 findings closed; provider and advisor work may safely build on the new boundary. |
| After Phase 5, covering Phases 4-5 together | Complete provider execution path: official clients, connection translation, exact sends, OpenRouter rejection, option builders, tool advisor singularity/order, semantic/physical retry scopes, usage, observations, cancellation, and sensitive data. Phase 4 alone is not reviewed as a standalone supported design because its safety depends on Phase 5 assembly. | All combined Phase 4-5 findings closed and the integrated provider/advisor test matrix is green. |
| After Phase 6 | `SkillParam` source break, strict configuration break, auto-configuration split, metadata, samples, migration guidance, and skill-authoring documentation accuracy/LLM-first usability. | Phase 6 findings closed; the author-facing state is coherent before residue removal. |
| After Phase 7 | Final cumulative review of the entire branch from the intended merge base through `HEAD`, independent of prior checkpoint conclusions. Validate all ticket, implementation-plan, and testing-plan criteria plus cross-phase composition, deletion, dependencies, protocols, docs, and full verification. | Final disposition is `Approve`, or `Approve with follow-ups` only when the user explicitly accepts the non-blocking follow-ups. The PR is not merge-ready on `Request changes`. |

Checkpoint reviews supplement rather than partition the final review. The Phase 7 reviewer must reconstruct and inspect the complete branch and must not treat earlier review approvals as proof that cross-phase composition is correct.

## Phase 1: Freeze Contracts and Target Counts

### Overview

Make current retained behavior executable before changing dependencies.

### Changes Required

#### 1. Serialization and protocol characterization
**Files**: existing YAML, JSON, NDJSON, trace, REST, SSE, artifact, and fixture-corpus tests under `loomspan-spring-boot-starter/src/test`; `loomspan-console-fixtures/**`

- Add missing read/write fixtures for record binding, unknown fields, enum/case behavior, null/default omission, Java time, numeric types, insertion ordering, newline termination, partial active records, and planning JSON/YAML.
- Treat existing REST/SSE/artifact/trace fixture files as retained outputs; do not rewrite them to accommodate Jackson 3.
- Add a portable-trace assertion that the exact compatibility marker remains unchanged and that the complete Java fixture is accepted by the Go processor.

#### 2. Attempt, OpenRouter, and advisor recursion characterization
**Files**: `ModelAttemptCallAdvisorIntegrationTest.java`, `SpringAiV11ProviderIntegrationTest.java`, `ConnectionProtocolTest.java`, advisor tests, and new target-neutral contract tests

- Add exact HTTP counters for each provider baseline. On the Spring AI 1 baseline, characterize two semantic attempts with one explicit provider-requested tool call each as four HTTP sends, two Loomspan outer attempt observations, and two callback executions. Phase 5 owns the AI 2 red gate and target assertion that all four tool-loop model turns become observable physical attempts inside the tool advisor.
- Pin OpenRouter HTTP-200 error completion as a failure with no surfaced partial content and at most 1 MiB of safe diagnostic body.
- Assert every counted send has one quota reservation and one prepared/sent/received-or-failed trace sequence.

#### 3. API/configuration characterization
**Files**: `LoomspanPublicSurfaceArchitectureTest.java`, `LoomspanAutoConfigurationBoundaryTest.java`, `ConfigurationMetadataTest.java`, `LoomspanPropertiesTest.java`, `SkillMethodBeanPostProcessorTest.java`, `SkillMethodTargetDiscoveryIntegrationTests.java`

- Freeze the seven existing supported API types, no-SPI rule, current `ToolParam` semantics, and exact old property acceptance/rejection before replacing them.

### Success Criteria

#### Automated Verification
- [x] Characterization suite passes on the old baseline: `.\mvnw.cmd -pl loomspan-spring-boot-starter test -DfailIfNoTests=false`.
- [x] Go consumes the unchanged trace corpus: `cd loomspan-console; go test ./internal/traceanalysis ./internal/artifact ./internal/applicationclient`.
- [x] Exact send/tool/attempt assertions fail if a hidden retry or advisor-scope multiplier is introduced.

#### Manual Verification
- [x] Review fixture additions as retained contracts rather than snapshots of incidental mapper/provider objects.
- [x] Confirm no new compatibility promise is attached to ephemeral Console state.

### Mandatory Pause: Phase 1 Review

- [x] Stop implementation and run the **After Phase 1** fresh-context review defined in the Independent Review and Pause Policy.
- [x] Record the review range, disposition, findings, fixes, re-review evidence, and rerun verification before starting Phase 2.

Checkpoint evidence pending independent review:

- Review base: `6b1200879abd83ff37847f3e6a5b0cfd0c27c03b` on `main`; checkpoint changes are the Phase 1 staged/unstaged/untracked plan and characterization files reported by `git status --short` (no checkpoint commit yet).
- Focused characterization: 41 tests passed.
- Full starter baseline: 845 tests passed after review fixes.
- Corrected Go gate: `internal/traceanalysis`, `internal/artifact`, and `internal/applicationclient` passed.
- `git diff --check` passed.
- Fixture review: additions pin complete portable-file bytes/marker, NDJSON framing, and retained codec behavior; they do not promote installed/imported Console state, handles, catalogs, or owner identities to compatibility contracts.
- Initial review disposition: `Request changes` with four P2 findings. Fixes replace the synthetic loop with explicit OpenAI tool-call responses and a real callback, exercise named production codec roles, add exact provider-retry exhaustion facts/metrics, and close/test rejected OpenRouter responses. Focused re-verification passed 42 tests; full starter re-verification passed 845 tests; corrected Go gate and `git diff --check` passed. Fresh-context re-review is pending.
- User disposition: on 2026-08-13 the user explicitly directed implementation to move to the next phase without a fresh-context re-review after reviewing the closure evidence above. Phase 2 resumed under the Independent Review and Pause Policy's explicit-user-disposition exception.

---

## Phase 2: Move to Boot 4 Modules and Purpose-Owned Jackson 3 Codecs

### Overview

Change the build baseline and serialization APIs while keeping Phase 1 fixtures green.

### Changes Required

#### 1. Maven baseline
**Files**: `pom.xml`, `loomspan-spring-boot-starter/pom.xml`, `loomspan-sample/pom.xml`

- Pin Boot `4.1.0` and Spring AI `2.0.0`; retain Java 21.
- Replace `spring-boot-starter-web` with `spring-boot-starter-webmvc` and use Boot 4 modular test/security artifacts and moved packages.
- Replace direct `com.fasterxml.jackson` databind/YAML dependencies with Jackson 3 coordinates managed by Boot. Do not add the Boot Jackson 2 compatibility module.

#### 2. Codec ownership
**Files**: new `internal/serialization/LoomspanJacksonCodecs.java`; new `LoomspanJacksonAutoConfiguration.java`; all 28 production Jackson users from the codebase map

- Provide explicit application-conversion, skill-YAML, planning JSON/YAML, schema-tree, canonical trace, and strict observability codec roles.
- Use Boot's Jackson 3 mapper for ordinary application conversion and explicit builders for deterministic/strict wire roles.
- Preserve Jackson annotations under `com.fasterxml.jackson.annotation`; migrate databind, JSON, YAML, node, and module APIs to `tools.jackson.*`.
- Inject codecs into planning, skill catalog, validation, trace, and web components; remove ad hoc static/private mapper construction.

#### 3. Boot 4 web/test imports
**Files**: `LoomspanObservabilityWebAutoConfiguration.java`, `ObservabilityWithoutMvcIntegrationTest.java`, `ObservabilityRestIntegrationTest.java`, `ObservabilitySseIntegrationTest.java`, related web slices

- Update moved MVC/security/test auto-configuration imports and keep servlet-conditional behavior, programmatic routes, security, and no-store semantics unchanged.

### Success Criteria

#### Automated Verification
- [ ] Dependency resolution and compile succeed on the pinned platform: `.\mvnw.cmd -DskipTests verify`.
- [ ] Jackson/YAML/trace/web contract suites pass with unchanged fixtures.
- [ ] No Loomspan Jackson 2 APIs remain: `rg "com\\.fasterxml\\.jackson\\.(core|databind|dataformat)" loomspan-spring-boot-starter/src` returns no API imports (annotations are allowed).
- [ ] No direct Jackson 2 dependency or compatibility module appears in `mvn dependency:tree`; OpenAI SDK-private transitives are documented exceptions.

#### Manual Verification
- [ ] Inspect canonical NDJSON bytes and representative REST/SSE fixtures for accidental ordering, omission, time, or newline changes.
- [ ] Confirm the sample starts far enough to bind configuration under Boot 4 before provider calls are enabled.

### Mandatory Pause: Phase 2 Review

- [ ] Stop implementation and run the **After Phase 2** fresh-context review defined in the Independent Review and Pause Policy.
- [ ] Close the review and rerun the affected serialization, protocol, dependency, and Boot integration gates before starting Phase 3.

---

## Phase 3: Establish Neutral Model and Capability Boundaries

### Overview

Stop orchestration from transporting Spring AI clients, callbacks, and responses before rebuilding provider behavior.

### Changes Required

#### 1. Neutral model interaction types
**Files**: new `internal/model/ModelInteraction*.java`; `ExecutionCoordinator.java`; `MissionExecutionEngine.java`; `DefaultMissionExecutionEngine.java`; `PlanningService.java`; `DefaultPlanningService.java`; `StepLoopMissionExecutionEngine.java`; `PlanQualityValidator.java`; `StepPromptBuilder.java`; `StepActionValidator.java`

- Add the concrete factory, mode, request, result, and interaction types described above.
- Change orchestration signatures to accept `ModelInteraction` and neutral capability descriptors.
- Normalize response content and attempt context at the boundary; delete downstream `ChatResponse`, `ChatClientResponse`, message, generation, and tool-definition extraction.
- Replace the ten fluent `ChatClient` doubles with small `FakeModelInteraction` fixtures that return content/context directly.

#### 2. Neutral capability execution
**Files**: new `internal/runtime/tool/CapabilityInvoker.java` and `DefaultCapabilityInvoker.java`; `DefaultToolCallbackFactory.java`; `StepLoopMissionExecutionEngine.java`; tool lifecycle tests

- Move quota, task linkage, frame, router, metrics, result, and failure behavior from the Spring callback factory into `DefaultCapabilityInvoker`.
- Pass `linkedTaskId` explicitly for step-loop calls; provider-originated calls pass no explicit task and retain current plan matching.
- Change step-loop tool selection/validation to neutral descriptors and invoke capabilities without JSON round-tripping through `ToolCallback.call`.

#### 3. Neutral schema generation
**Files**: `CapabilityToolDescriptor.java`, `SkillMethodBeanPostProcessor.java`, `ToolCallbackInputContracts.java`

- Remove Spring AI schema generation from `CapabilityToolDescriptor`; generate/validate schema through the Jackson 3 schema role and keep descriptor content provider-neutral.

### Success Criteria

#### Automated Verification
- [ ] Architecture tests reject Spring AI imports outside `internal.springai`, the Spring-facing advisor implementations, and auto-configuration integration classes.
- [ ] Mission, planning, direct execution, step loop, nested capability, attachment, validation, and trace tests pass through `FakeModelInteraction`.
- [ ] Tool lifecycle tests prove provider-loop and step-loop invocations produce the same quota, plan, frame, metric, result, and failure semantics without double accounting.

#### Manual Verification
- [ ] Review the boundary to ensure it models Loomspan interactions, not a second general-purpose AI client API.
- [ ] Confirm application API and YAML author mental models are unchanged by the internal refactor.

### Mandatory Pause: Phase 3 Review

- [ ] Stop implementation and run the **After Phase 3** fresh-context review defined in the Independent Review and Pause Policy.
- [ ] Close architectural, lifecycle, accounting, and trace findings before any provider implementation is built on the boundary.

---

## Phase 4: Rebuild Provider Integration and Connection Translation

### Overview

Replace removed Spring AI 1 provider APIs and guarantee one send per Loomspan attempt.

### Changes Required

#### 1. Spring AI 2 provider construction
**Files**: new `internal/springai/SpringAiProviderIntegration.java`; moved `ProviderConnectionRuntime.java`; `NamedAiConnectionRegistry.java`; provider tests; delete `internal/springai/v1_1/**`

- Build OpenAI and Anthropic Spring AI models with their official Java SDK clients and SDK `maxRetries(0)`.
- Build Gemini and Ollama with Framework 7 core retry templates using `RetryPolicy.withMaxRetries(0)`.
- Preserve failure categorization, `Retry-After`, timeout/connectivity/SSL/interruption handling, safe construction errors, and endpoint identity without exposing credentials.

#### 2. Connection property redesign
**Files**: `LoomspanProperties.java`, `additional-spring-configuration-metadata.json`, `LoomspanPropertiesTest.java`, `ConfigurationMetadataTest.java`, `loomspan-sample/src/main/resources/application.yml`

- Remove OpenAI chat-completions path and all Anthropic nested path/version/beta fields/classes.
- Retain common base URL, API key, headers, and retry; retain OpenAI organization/project/profile and Gemini mode fields.
- Permit common static headers for OpenAI and Anthropic (including `anthropic-beta`); continue rejecting them for Gemini/Ollama until those drivers have a deliberate header contract.
- Keep strict unknown-field validation so old properties fail with their full property paths.

#### 3. OpenRouter pre-decode guard
**Files**: `SpringAiProviderIntegration.java`, `ConnectionProtocolTest.java`, provider integration tests

- Port the opt-in OkHttp response interceptor to the official SDK path, bound body capture at 1 MiB, reject error completions before decode, close responses correctly, and normalize the result as a provider failure.

### Success Criteria

#### Automated Verification
- [x] OpenAI, Anthropic, Gemini, and Ollama fixtures each observe one HTTP send for one Loomspan attempt.
- [x] Multi-attempt Loomspan retry tests observe exactly the configured number of sends, quotas, metrics, trace attempts, and one terminal category.
- [x] OpenRouter error-completion tests reject partial content and preserve bounded diagnostics.
- [x] Removed properties are absent from generated metadata and rejected by strict binding; retained provider configurations start successfully.

#### Manual Verification
- [ ] Inspect request paths and headers for official OpenAI, OpenRouter, Anthropic, Gemini API-key/Vertex, and Ollama configurations.
- [ ] Confirm logs/exceptions do not reveal API keys, header values, base URLs, or credential content.

There is no independent pause after Phase 4. Continue directly into Phase 5 so the provider construction and its client/advisor/tool-loop assembly can be reviewed as one complete execution path.

---

## Phase 5: Assemble AI 2 Options, Tool Loop, Advisors, and Observations

### Overview

Implement the Spring side of the model boundary and prove every recursive scope.

### Changes Required

#### 1. Model interaction implementation and option contributors
**Files**: new `SpringAiModelInteractionFactory.java`, `SpringAiModelInteraction.java`, `SpringAiChatOptionsContributor.java`; replace `SpringAiSkillChatClientFactory.java`, `SkillChatClientFactory.java`, and `SkillChatOptionsAdapter.java`

- Build one client per effective skill execution configuration using provider-specific immutable option builders.
- Preserve OpenAI model/reasoning/temperature behavior, Anthropic/Gemini thinking budgets, and Ollama model selection.
- Explicitly supply observation registry/conventions and advisor components; do not apply `ChatClientBuilderConfigurer`.

#### 2. Spring tool adapter and advisor order
**Files**: new `SpringAiToolCallbackAdapter.java`; `ProviderAttemptCallAdvisor.java`; semantic advisor classes/resolver; delete old callback wrappers/factory

- Convert neutral capabilities into Spring AI callbacks only inside `internal.springai`; delegate execution to `CapabilityInvoker`.
- Install exactly one `ToolCallingAdvisor` (use automatic registration only when the integration test proves singularity; otherwise register explicitly and disable duplicate auto-registration).
- Assign/test relative scopes: semantic output/linter/evidence policy outside tool calling, physical attempt advisor inside tool calling, provider retry disabled beneath it.
- Preserve step execution's intentional omission of final-response validators without removing tool or physical-attempt behavior.

#### 3. Usage and observations
**Files**: `ModelUsageExtractor.java` or replacement under `internal.springai`; `MicrometerUsageMetricsRecorder.java`; new observation configuration/tests

- Normalize only content, usage, finish reason, tool calls, and attempt context needed by Loomspan.
- Enable conventional Spring AI chat/model/tool/HTTP observations with prompt/completion/tool-argument content disabled by default.
- Assert Spring observations do not replace or double Loomspan counters, quotas, attempt facts, or portable trace records.

### Success Criteria

#### Automated Verification
- [x] The durable recursion test observes two semantic attempts, four model turns/physical attempts, and two tool executions.
- [x] Advisor-chain tests find exactly one selected tool advisor and verify relative order by behavior, not numeric constants alone.
- [x] Direct, planning, step-loop, attachment, semantic retry, tool failure, cancellation, and quota tests pass on AI 2.
- [x] Observation tests verify propagation and safe defaults while Loomspan metric/trace counts remain unchanged.

#### Manual Verification
- [ ] Inspect one trace containing a tool call plus semantic retry and confirm the model-turn, provider-attempt, tool, validator, and usage narrative is understandable.
- [ ] Confirm no prompt, completion, tool arguments, credentials, or provider bodies are exported by default observations.

### Mandatory Pause: Combined Phase 4-5 Review

- [x] Stop implementation and run the **After Phase 5, covering Phases 4-5 together** fresh-context review defined in the Independent Review and Pause Policy.
- [x] Close all provider, retry, advisor, tool-loop, usage, observation, lifecycle, and sensitive-data findings and rerun the integrated matrix before starting Phase 6.

Review disposition: `Request changes` with five P2 findings. The fixes preserve explicit-null tool arguments through both provider and deterministic step-loop paths; wire named Jackson codec roles into every production serialization boundary; replace Spring AI schema infrastructure with Loomspan-owned Jackson 3 schema generation; exercise standard, step, empty-tool, semantic-retry, and real tool-loop behavior through the production assembler; and cover success, tool, provider-retry, semantic-retry, and terminal-failure observation/accounting paths with sensitive-data canaries. Re-verification passed the 846-test starter suite, all Console Go packages, the public-surface and Spring-AI containment gates, and `git diff --check`.

---

## Phase 6: Complete API, Auto-Configuration, Sample, and Documentation Migration

### Overview

Finish the deliberate author-facing break and production composition on the new platform.

### Changes Required

#### 1. `SkillParam` application API
**Files**: new `api/SkillParam.java`; `SkillMethodBeanPostProcessor.java`; processor/discovery/API/architecture tests; five sample services

- Add runtime parameter-target annotation with `description` defaulting to empty and `required` defaulting to true.
- Preserve interface/implementation contract selection, conflict detection, parameter-name resolution, optionality, nested schema, binding, proxy, bridge, and invocation behavior.
- Remove every `ToolParam` import and all dual-annotation handling.

#### 2. Auto-configuration split
**Files**: `LoomspanAutoConfiguration.java`; new `LoomspanAiAutoConfiguration.java`; `LoomspanJacksonAutoConfiguration.java`; `AutoConfiguration.imports`; auto-configuration and architecture tests

- Keep stable runtime and public `SkillTemplate` composition in the core configuration.
- Move provider registry, Spring model interaction, AI 2 options/advisors/tools, and observations into AI configuration; keep codec roles in Jackson configuration.
- Preserve the no-override rule and update the technical public-type allowlist with reasons for any public-for-Spring classes.

#### 3. Maintained samples and documentation
**Files**: `README.md`, `loomspan-sample/README.md`, sample source/config, `ai/skill-authoring/mental-model.md`, `model-selection-and-connections.md`, `traces-and-debugging.md`

- Replace old platform names, properties, provider paths, retry descriptions, and `ToolParam` examples.
- Explain `@SkillParam`, official client path composition, common Anthropic headers, exact one-send ownership, and the distinction among one model interaction, tool-loop model turns, semantic retries, and physical attempts.
- Retain same-version portable-trace limitations, exact marker behavior, transient imported evidence, sensitive-content warnings, and no provenance/authenticity claim.

### Success Criteria

#### Automated Verification
- [x] Supported-surface integration invokes an LLM-backed YAML entry skill with a mapped `@SkillMethod`/`@SkillParam` leaf and no internal bean replacement.
- [x] `rg "ToolParam|SpringAiV11|spring-ai.*1\\.1|spring-boot-starter-web<|chat-completions-path|anthropic\\.(completions-path|version|beta-version)"` finds no maintained code/config/doc residue except historical ticket/research references.
- [x] Generated configuration metadata exactly lists surviving fields.
- [x] Skill-authoring claims are backed by the cited tests, samples, properties, and production paths.

#### Manual Verification
- [ ] Run the sample with one OpenAI-compatible configuration and one local/native provider configuration; invoke a representative mapped leaf, direct skill, and planning skill.
- [ ] Have a fresh-context reviewer follow only the routed authoring docs to configure a connection, annotate a Java parameter, and explain a retry trace without source inspection.

### Mandatory Pause: Phase 6 Review

Implementation evidence: `SkillParam` is the eighth allowlisted application API type and is the sole parameter metadata contract in production, tests, and all five sample services. Core, AI, Jackson, and web auto-configuration bean factories have distinct owners with package-private methods and no replacement SPI. Strict binding rejects the removed provider fields; generated metadata is asserted against the exact surviving connection/model property set and its path/header semantics. The supported-surface test performs a real local provider tool loop into a mapped `@SkillMethod`/`@SkillParam` leaf. Routed authoring guidance now documents official SDK path composition, common Anthropic headers, retry/turn/attempt terminology, same-version trace portability, transient imports, sensitivity, and provenance limitations with executable anchors. Reactor verification passed 852 starter tests and 76 sample tests with one opt-in live-provider test skipped; the maintained-source residue scan and `git diff --check` passed.

- [x] Stop implementation and run the **After Phase 6** fresh-context review defined in the Independent Review and Pause Policy.
- [x] Close API/configuration compatibility, metadata, composition, sample, migration, and documentation findings before final residue removal.

Review disposition: `Request changes` with two P2 findings. The fixes make generated metadata describe the actual OpenAI `/chat/completions` and Anthropic `/v1/messages` composition rules and the OpenAI/Anthropic static-header scope, with exact semantic assertions in `ConfigurationMetadataTest`. Optional primitive `@SkillParam(required = false)` declarations are now rejected during target discovery with an actionable boxed-type/required-parameter remedy, covered by a focused registration test and the routed skill-authoring guidance. A clean focused build passed 20 tests; full reactor re-verification passed 852 starter tests and 76 sample tests with the single opt-in live-provider test skipped; the maintained-source residue scan and `git diff --check` passed.

---

## Phase 7: Remove Residue and Run Atomic Release Gates

### Overview

Delete obsolete paths, enforce architecture/dependency boundaries, and verify the whole repository as one supported state.

### Changes Required

#### 1. Deletion and enforcement
**Files**: obsolete Spring AI 1 provider/chat/tool classes and fluent test doubles; POMs; architecture tests

- Delete versioned AI 1 packages, old factories/adapters/callback wrappers, Spring Retry usage, Jackson 2 mapper helpers, removed property types, and obsolete tests.
- Add ArchUnit/text/dependency assertions for Spring AI containment, no Loomspan Jackson 2 API, no Spring Retry, no old packages, one supported API allowlist, and no conditional replacement beans.

#### 2. Full cross-repository verification
**Files**: no new behavior; verification only

- Run the full Maven reactor and sample package/start gates.
- Re-run Java fixture corpora and Go Console acquisition/analysis tests even though no protocol change is planned.
- Run Console frontend typecheck/tests for the retained browser contract and inspect the final dependency tree.

### Success Criteria

#### Automated Verification
- [x] Full reactor: `.\mvnw.cmd clean verify`.
- [x] Starter focused/full: `.\mvnw.cmd -pl loomspan-spring-boot-starter test -DfailIfNoTests=false`.
- [x] Sample with dependencies: `.\mvnw.cmd -pl loomspan-sample -am verify`.
- [x] Console backend: `cd loomspan-console; go test ./...`.
- [x] Console frontend: `cd loomspan-console/web; npm run typecheck; npm test -- --run`.
- [x] `git diff --check` passes.
- [x] Dependency and source scans prove no forbidden direct API/dependency/package residue; the only allowed Jackson 2 artifacts are SDK-private transitives recorded in the dependency audit.

Implementation evidence: obsolete Spring AI 1 provider, factory, callback, attachment-sender, versioned package, and test paths are deleted. Remaining handwritten Spring AI fluent-client doubles and their default-method helpers were replaced with neutral `ModelInteraction` fakes or real `ChatClient` instances backed by deterministic `ChatModel` responses. `PlatformResidueArchitectureTest` now enforces old-type and Spring Retry absence, Jackson 2 API exclusion, Spring AI package containment, and the neutral test-double boundary. The clean reactor and standalone starter/sample gates passed 855 starter tests and 76 sample tests with only the opt-in live-provider smoke test skipped. All Console Go packages passed; frontend typecheck and 378 tests across 43 files passed. The dependency tree is aligned to Boot 4.1.0 and Spring AI 2.0.0 with no Spring Retry, direct Loomspan Jackson 2 dependency, or Boot compatibility module; Jackson 2 implementation artifacts are confined to the official OpenAI, Anthropic, and Google provider SDK graphs, while `jackson-annotations` remains the Jackson 3 API's required annotation artifact. Maintained-source/property residue scans and `git diff --check` passed.

Final cumulative review disposition: `Request changes` with two P2 findings. Anthropic request options now pair low, medium, and high thinking budgets with provider-valid `maxTokens` values (`1024/4096`, `4096/8192`, and `8192/16384`); option-level and captured `/v1/messages` request-body tests assert the exact values and strict inequality for all three levels. OpenRouter success-body inspection now reads only `DIAGNOSTIC_LIMIT_BYTES + 1` through the response stream, closes the original body before rejection or rebuilding, and rejects a virtual streaming oversized body without consuming it. Focused provider/options/protocol verification passed 20 tests. A clean reactor after the production fixes passed 859 starter tests and 76 sample tests with the opt-in live-provider test skipped; the final full starter run including the captured Anthropic request matrix passed 862 tests. Final re-review remains pending.

#### Manual Verification
- [ ] Start the Boot 4 sample and exercise representative OpenAI/OpenRouter, Anthropic, Gemini, and Ollama configurations available to the reviewer.
- [ ] Verify one direct mission, planning mission, tool call, semantic retry, provider retry, terminal failure, REST/SSE observation, trace save, and same-version Console import.
- [ ] Confirm target rotation does not affect imported evidence and imported errors expose no target scope/owner ID, preserving the closed PR 25 review findings.
- [ ] Review the final PR as one atomic platform state; no intermediate commit is described as independently supported.

### Mandatory Pause: Final Cumulative Review

- [ ] Stop implementation and run the **After Phase 7** fresh-context review against the intended merge base and the complete branch.
- [ ] Do not declare the PR complete or merge-ready until final review findings are closed and the final disposition satisfies the Independent Review and Pause Policy.

## Testing Strategy

Use the dedicated testing plan at `ai/thoughts/plans/2026-08-13-PR-26-spring-boot-4-spring-ai-2-testing.md`. It defines the named test classes, phase-specific red gates, fixtures, commands, checkpoint evidence packages, manual matrix, and exit criteria that must be satisfied before implementation and at every mandatory review pause.

### Unit Tests

- `SkillParam` discovery, inheritance, conflicts, descriptions, requiredness, schemas, binding, proxy/bridge handling, and invocation.
- Neutral request/result immutability, model modes, capability descriptors, capability invocation lifecycle, and normalized usage/context.
- Provider option translation, connection validation, failure classification, retry delays, redaction, and OpenRouter body bounds.
- Jackson 3 role-specific record/tree/YAML/NDJSON behavior.

### Integration Tests

- Exact provider HTTP send counts and Loomspan quota/trace/metric counts for all four drivers.
- AI 2 semantic/tool/attempt recursion and exactly one tool advisor.
- Direct, planning, nested, step-loop, attachment, cancellation, timeout, and semantic-validation paths.
- Boot 4 auto-configuration with and without MVC/security/observations.
- Supported application surface and sample startup.
- Exact Java REST/SSE/artifact/trace corpora plus Go same-version portable import.

### Manual Testing Steps

1. Start the sample on Boot 4.1.0 and verify configuration metadata/validation messages for both retained and removed fields.
2. Execute representative direct and planned skills with a tool call and inspect Loomspan trace plus Spring observations.
3. Force provider retry and OpenRouter error-completion cases and compare physical sends, quotas, trace facts, diagnostics, and terminal categories.
4. Save the complete trace, import it in Console with no target connected, inspect it, rotate a target, and verify imported state remains stable and process-local.

## Performance Considerations

- Eliminating hidden provider retries prevents multiplicative network latency and quota use.
- The session-bound interaction should build/reuse provider model resources through the named connection registry while keeping per-skill client/options/advisors immutable; do not create a new HTTP client for each model turn.
- Tool-callback conversion should happen once per mission-scoped interaction, not once per tool-loop turn.
- Purpose-owned Jackson codecs should be immutable and reused; canonical readers remain streaming and bounded, and OpenRouter diagnostics remain capped at 1 MiB.
- Observation integration must not duplicate expensive content capture or serialize sensitive prompt/tool bodies by default.

## Migration Notes

- This is an intentional pre-1.0 source/configuration break with no runtime migration layer.
- Application code replaces `org.springframework.ai.tool.annotation.ToolParam` with `com.lokiscale.loomspan.api.SkillParam` in the same change.
- Remove the four obsolete provider fields before starting on the new version. Use `headers.anthropic-beta` for Anthropic beta features and rely on the official clients' fixed endpoint composition below `base-url`.
- Existing YAML skill manifests, model aliases, canonical traces, REST/SSE consumers, and Console same-version import require no planned data migration.
- Implementation must occur in an isolated branch/worktree based on the intended integration commit and must not absorb unrelated PR 16-20 changes.
- Rollback is the whole PR/release. Intermediate commits are review aids, not deployable rollback points.

## References

- Ticket: `ai/thoughts/tickets/loomspan-platform-pr-26-spring-boot-4-spring-ai-2.md`
- Testing plan: `ai/thoughts/plans/2026-08-13-PR-26-spring-boot-4-spring-ai-2-testing.md`
- Fresh codebase map: `ai/thoughts/research/2026-08-12-PR-26-spring-boot-4-spring-ai-2-codebase-map.md`
- Platform experiments: `ai/thoughts/research/2026-08-12-spring-platform-upgrade-research.md`
- Contract policy: `ai/thoughts/framework-feature-design-lens.md`
- Portable trace review task: `thread://019ff924-677b-7ab1-bf83-f30750b678e3`
- Spring Boot 4.1.0 project/reference: `https://spring.io/projects/spring-boot/`
- Spring AI 2.0.0 GA and Boot 4/Jackson 3 baseline: `https://spring.io/blog/2026/06/12/spring-ai-2-0-0-GA-available-now/`
- Spring AI 2 upgrade/tool/observability references linked from `ai/thoughts/research/2026-08-12-spring-platform-upgrade-research.md`
