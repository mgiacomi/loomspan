---
date: 2026-08-12T23:50:08-07:00
researcher: Codex
git_commit: 6b1200879abd83ff37847f3e6a5b0cfd0c27c03b
branch: main
repository: loomspan
topic: "PR 26 Spring Boot 4 / Spring AI 2 platform migration as-is code and consumer map"
tags: [research, codebase, spring-boot, spring-ai, jackson, providers, observability, console]
status: complete
last_updated: 2026-08-12
last_updated_by: Codex
---

# Research: PR 26 Spring Boot 4 / Spring AI 2 Platform Migration

**Date**: 2026-08-12 23:50:08 PDT
**Researcher**: Codex (GPT-5)
**Git Commit**: `6b1200879abd83ff37847f3e6a5b0cfd0c27c03b`
**Branch**: `main`
**Repository**: `loomspan`

## Research Question

Produce the fresh, as-is code and consumer map required by `ai/thoughts/tickets/loomspan-platform-pr-26-spring-boot-4-spring-ai-2.md`, using the completed platform experiments as historical evidence and classifying framework surfaces with `ai/thoughts/framework-feature-design-lens.md`.

## Summary

The live repository is a two-module Java 21 Maven reactor: `loomspan-spring-boot-starter` contains the framework and `loomspan-sample` is the runnable application. Dependency management currently pins Spring Boot 3.5.11 and Spring AI 1.1.6; the starter directly depends on the four Spring AI provider modules and Jackson 2 databind/YAML, while both modules still use the Boot 3 `spring-boot-starter-web` artifact (`pom.xml:45-50`, `loomspan-spring-boot-starter/pom.xml:19-103`, `loomspan-sample/pom.xml:19-45`).

The deliberately supported Java application surface is already narrow. Repository architecture tests and documentation identify exactly seven `com.lokiscale.loomspan.api` types: `SkillTemplate`, `SkillExecutionView`, `SkillExecutionEvent`, `SkillMethod`, `SkillException`, `SkillInputValidationException`, and `SkillInputValidationIssue`. The same evidence explicitly identifies no supported Loomspan SPI or bean override. All current framework beans are framework-owned and an architecture test rejects every production `@ConditionalOnMissingBean` seam (`README.md:157-159`, `loomspan-spring-boot-starter/src/test/java/com/lokiscale/loomspan/architecture/LoomspanPublicSurfaceArchitectureTest.java:29-40`, `loomspan-spring-boot-starter/src/test/java/com/lokiscale/loomspan/architecture/LoomspanAutoConfigurationBoundaryTest.java:49-114`).

Spring AI types are nevertheless broad inside the implementation. Fresh inventory finds 31 production Java files importing Spring AI. `ExecutionCoordinator` creates a `ChatClient` and a list of Spring AI `ToolCallback` values, then passes both through `MissionExecutionEngine`, `PlanningService`, step-loop prompting/validation, message sending, usage extraction, and tool execution. Ten test files implement parts of Spring AI's fluent `ChatClient` interfaces as handwritten doubles. These are technically exposed internal seams, not supported SPIs (`ExecutionCoordinator.java:31-43`, `ExecutionCoordinator.java:110-128`, `MissionExecutionEngine.java:12-23`, `PlanningService.java:16-24`, `ToolCallbackFactory.java:12-19`).

Current provider construction is centralized in the version-scoped `SpringAiV11ProviderIntegration`. It builds OpenAI and Anthropic through Spring AI 1 REST API builders, Gemini through Google's SDK client, and Ollama through `OllamaApi`. Each underlying framework/SDK retry is set to one send, while `ProviderAttemptCallAdvisor` owns the Loomspan retry loop, provider-attempt quota reservation, request/response/failure trace events, normalized failure categories, and usage recording (`SpringAiV11ProviderIntegration.java:60-161`, `ProviderAttemptCallAdvisor.java:42-102`). OpenRouter's opt-in compatibility profile intercepts successful HTTP responses and rejects `finish_reason: error` before normal decoding, with a 1 MiB bounded diagnostic capture (`SpringAiV11ProviderIntegration.java:98-108`, `SpringAiV11ProviderIntegration.java:243-302`).

The current serialization footprint is also broad: 28 production Java files use Jackson directly. The roles include Boot-provided conversion for application input and `SkillTemplate`, private JSON/YAML mappers for skill manifests and planning, schema and evidence validation, provider diagnostics, canonical NDJSON trace writing/reading, journal projection, and a deliberately strict REST/cursor codec. Canonical REST, SSE, artifact, and trace corpora connect the Java producer to the Go Console and its TypeScript UI. The Console's own repository instructions require Java, Go, TypeScript, fixtures, and semantic tests to move together for protocol changes and use exact `consoleCompatibilityVersion` equality (`loomspan-console/AGENTS.md:1-18`).

The fresh baseline run `./mvnw.cmd -pl loomspan-spring-boot-starter test -DfailIfNoTests=false` completed successfully with 836 tests, zero failures, zero errors, and zero skips. This establishes current Boot 3 / AI 1 behavior; it does not exercise the target platform. The previously completed target experiments remain the evidence for Boot 4.1 / AI 2.0 / Jackson 3 framework behavior and were not repeated.

## Detailed Findings

### 1. Module and dependency baseline

- The parent compiles with Java release 21 and imports the Spring Boot 3.5.11 and Spring AI 1.1.6 BOMs (`pom.xml:45-50`, `pom.xml:62-73`).
- The starter directly declares `spring-ai-model`, `spring-ai-client-chat`, `spring-ai-openai`, `spring-ai-anthropic`, `spring-ai-google-genai`, and `spring-ai-ollama` (`loomspan-spring-boot-starter/pom.xml:19-45`).
- Direct Loomspan Jackson dependencies are `com.fasterxml.jackson.core:jackson-databind` and `com.fasterxml.jackson.dataformat:jackson-dataformat-yaml` (`loomspan-spring-boot-starter/pom.xml:47-53`).
- The starter's test stack and the sample application use `spring-boot-starter-web`; these are the named web starter locations affected by Boot 4 module renaming (`loomspan-spring-boot-starter/pom.xml:91-103`, `loomspan-sample/pom.xml:25-31`).
- Boot auto-configuration registration currently lists `LoomspanAutoConfiguration` and `LoomspanObservabilityWebAutoConfiguration` (`loomspan-spring-boot-starter/src/main/resources/META-INF/spring/org.springframework.boot.autoconfigure.AutoConfiguration.imports:1-2`).
- The production source has one direct Boot 3 Web MVC auto-configuration import in `LoomspanObservabilityWebAutoConfiguration`; web tests also import the Boot 3 servlet security and Web MVC auto-configuration packages (`LoomspanObservabilityWebAutoConfiguration.java:24`, `ObservabilityWithoutMvcIntegrationTest.java:7-9`, `ObservabilityRestIntegrationTest.java:7-8`).

### 2. Public declarations, supported surface, and extension status

#### Application API

The deliberate Application API is the seven-type allowlist described above. Its main entry point is:

- `SkillTemplate.invoke` has object/map overloads and optional `Consumer<SkillExecutionView>` observation (`SkillTemplate.java:6-14`).
- `SkillMethod` is a runtime method annotation with a required `description` (`SkillMethod.java:8-13`).
- `SkillExecutionView` carries a session ID and immutable list of current execution events (`SkillExecutionView.java:6-17`).
- `SkillExecutionEvent` carries timestamp, level, type, diagnostic details, frame ID, and route; its constructor recursively copies supported scalar/map/list/array detail values (`SkillExecutionEvent.java:12-75`).
- The exception surface is `SkillException`, `SkillInputValidationException`, and `SkillInputValidationIssue` (`SkillException.java:3-13`, `SkillInputValidationException.java:5-18`, `SkillInputValidationIssue.java:3`).

Evidence of deliberate support is cumulative: the architecture allowlist, README statement, `SupportedSurfaceIntegrationTest`, sample controllers that inject only `SkillTemplate`, and skill-authoring documentation. The integration test configures a local OpenAI-compatible connection and invokes an LLM-backed YAML skill without replacing internal beans (`README.md:135-159`, `loomspan-spring-boot-starter/src/test/java/com/lokiscale/loomspan/integration/SupportedSurfaceIntegrationTest.java:47-104`).

#### Supported SPI

There is no current supported Loomspan SPI. The bean-boundary allowlist is empty, production `@ConditionalOnMissingBean` is forbidden, and README guidance says application tests should not replace resolvers, coordinators, chat-client factories, registries, or virtual-file-system beans (`LoomspanAutoConfigurationBoundaryTest.java:49-114`, `README.md:157-159`). Public internal interfaces and constructors therefore show technical exposure and testability, not supported replacement contracts.

#### Framework integration and technically public internals

The architecture allowlist separately recognizes five public framework-integration types: both auto-configurations, `LoomspanProperties`, `ExecutionTraceProperties`, and `AiDriver`. It also maintains a reasoned list of public types under `.internal`, including provider, chat, runtime, trace, and web components that are public only for cross-package or Spring composition (`LoomspanPublicSurfaceArchitectureTest.java:38-249`). This is direct evidence that their visibility does not classify them as Application API or Supported SPI.

### 3. Configuration and manifest contracts

`LoomspanProperties` is strict `@ConfigurationProperties(prefix = "loomspan", ignoreUnknownFields = false)` configuration. It owns session limits, skill locations, observability, named connections, and model aliases (`LoomspanProperties.java:23-45`). Current connection/model fields are:

| Area | Current fields and defaults | Current consumers |
| --- | --- | --- |
| Session | `max-depth=32`, `mission-timeout=60s`, attachment maximum 20 MiB, and quotas for skill/tool/linter/model/provider/usage counts | Session runner, attachment materialization, usage/quota service (`LoomspanProperties.java:278-346`) |
| Skills | `locations`, defaulting to `classpath:/skills/**/*.yaml` | `YamlSkillCatalog` (`LoomspanProperties.java:350-357`, `LoomspanAutoConfiguration.java:181-186`) |
| Observability | enablement, API key, completion grace TTL, trace catalog metadata TTL | Core and servlet observability auto-configuration (`LoomspanProperties.java:361-390`) |
| Connection common | `driver`, `base-url`, `api-key`, static `headers`, provider retry | Registry and provider integration (`LoomspanProperties.java:400-430`) |
| OpenAI | organization ID, project ID, chat-completions path, compatibility profile (`OPENROUTER`) | OpenAI builder and OpenRouter interceptor (`LoomspanProperties.java:443-466`, `SpringAiV11ProviderIntegration.java:90-114`) |
| Anthropic | completions path, version, beta version | Anthropic REST API builder (`LoomspanProperties.java:492-502`, `SpringAiV11ProviderIntegration.java:116-129`) |
| Gemini | Vertex mode, project, location, credentials URI | Google SDK client builder (`LoomspanProperties.java:505-518`, `SpringAiV11ProviderIntegration.java:131-142`) |
| Model alias | connection, provider model, allowed thinking levels | YAML resolution and provider-specific options (`LoomspanProperties.java:521-535`, `SpringAiSkillChatClientFactory.java:86-124`) |

Validation requires provider-appropriate fields, rejects inapplicable provider blocks, validates headers, requires exactly one Gemini credential mode, verifies model-to-connection references, and validates provider retry ranges (`LoomspanProperties.java:101-261`). Generated configuration metadata is asserted by `ConfigurationMetadataTest`; README, sample `application.yml`, sample README, and `ai/skill-authoring/model-selection-and-connections.md` are maintained author-facing consumers (`ConfigurationMetadataTest.java:14-22`, `README.md:77-128`, `loomspan-sample/src/main/resources/application.yml:19-66`, `ai/skill-authoring/model-selection-and-connections.md:18-91`).

YAML manifests are a separate configuration/manifest contract. `YamlSkillCatalog` loads configured resource patterns, parses with a YAML mapper, validates unknown fields and cross-skill references, and builds `YamlSkillDefinition` values (`YamlSkillCatalog.java:53-90`, `YamlSkillCatalog.java:266-328`). The format covers public identity and description, LLM model or Java mapping, allowed skills, role rules, planning settings, input/output schemas, evidence annotations, and linter settings. The sample contains both LLM-backed and mapped manifests, and the starter has valid/invalid fixture directories used by 89 catalog tests recorded in the earlier experiment inventory.

### 4. Current Spring AI execution flow

The live call path is:

```text
application -> SkillTemplate
  -> ExecutionCoordinator
    -> SkillChatClientFactory -> Spring AI ChatClient
    -> ToolSurfaceService -> Loomspan capability metadata
    -> ToolCallbackFactory -> Spring AI ToolCallback list
    -> MissionExecutionEngine
       -> optional PlanningService
       -> message sender / step loop
       -> ChatClient response
```

`ExecutionCoordinator` owns access checks, mission frame lifecycle, trace finalization, engine selection, and construction of the two Spring AI values passed into orchestration (`ExecutionCoordinator.java:60-176`). For normal execution it uses `DefaultMissionExecutionEngine`; explicit planning mode selects `StepLoopMissionExecutionEngine` and a client with final-response validators omitted (`ExecutionCoordinator.java:107-128`, `SpringAiSkillChatClientFactory.java:82-91`).

Spring AI integration types currently appear in these internal interface signatures:

- `SkillChatClientFactory` returns `ChatClient` (`SkillChatClientFactory.java:6-15`).
- `MissionExecutionEngine.executeMission` receives `ChatClient` and `List<ToolCallback>` (`MissionExecutionEngine.java:12-23`).
- `PlanningService.initializePlan` receives both types (`PlanningService.java:16-24`).
- `MissionUserMessageSender.send` receives both and returns `ChatClient.CallResponseSpec` (`MissionUserMessageSender.java:11-20`).
- `ToolCallbackFactory` returns `List<ToolCallback>` (`ToolCallbackFactory.java:12-19`).
- Planning quality, step prompting, step validation, and usage extraction also directly consume Spring AI tool/response types (`PlanQualityValidator.java:28-32`, `StepPromptBuilder.java:38-84`, `StepActionValidator.java:41-145`, `ModelUsageExtractor.java:11-75`).

These interfaces reside under `.internal`, are included among technically public implementation types by the architecture test, have no supported bean-override entry point, and are not used by the sample as application APIs. Their current classification is **Internal or accidentally exposed implementation**.

### 5. Chat options and advisor assembly

`SpringAiSkillChatClientFactory` resolves an effective skill configuration to a provider runtime, chooses a provider-specific `SkillChatOptionsAdapter`, creates built Spring AI 1 option values, resolves semantic advisors, appends one `ProviderAttemptCallAdvisor`, and applies all of them as defaults on a `ChatClient.Builder` (`SpringAiSkillChatClientFactory.java:82-124`).

Provider-specific option behavior currently includes:

- OpenAI provider model, GPT-5 temperature default, and string reasoning effort (`SpringAiSkillChatClientFactory.java:159-191`).
- Anthropic thinking type and a low/medium/high token budget (`SpringAiSkillChatClientFactory.java:194-213`).
- Gemini `includeThoughts` plus the same token budgets (`SpringAiSkillChatClientFactory.java:216-237`).
- Ollama provider model only (`SpringAiSkillChatClientFactory.java:240-254`).

`DefaultSkillAdvisorResolver` conditionally assembles output-schema, evidence-contract, and regex-linter advisors from each YAML definition and records their request/response mutations in Loomspan's execution state (`DefaultSkillAdvisorResolver.java:53-143`). The current order values are linter `DEFAULT_CHAT_MEMORY_PRECEDENCE_ORDER - 100`, output schema `- 90`, evidence `- 80`, and provider attempt `LOWEST_PRECEDENCE - 1` (`LinterCallAdvisor.java:131-134`, `OutputSchemaCallAdvisor.java:169-172`, `EvidenceContractCallAdvisor.java:109-112`, `ProviderAttemptCallAdvisor.java:30`). Step execution removes all three final-response semantic validators from the default advisor list (`SpringAiSkillChatClientFactory.java:126-143`).

There is no current Loomspan `ToolCallingAdvisor`; Spring AI 1's model/tool behavior is relied upon after request tool callbacks are attached. The completed AI 2 experiment, rather than current production code, establishes the ticket's settled target ordering of semantic policy outside the AI 2 tool loop and Loomspan physical attempts inside it.

### 6. Capability discovery, `@ToolParam`, and tool execution

`SkillMethodBeanPostProcessor` discovers Spring beans with Loomspan `@SkillMethod`, resolves proxy/interface contracts, derives parameter names/descriptions/requiredness, builds JSON schemas and input contracts, binds JSON-like arguments, and registers internal `beanName#methodName` targets. It currently reads Spring AI `@ToolParam` from implementation or interface parameters for descriptions and optionality (`SkillMethodBeanPostProcessor.java:43-68`, `SkillMethodBeanPostProcessor.java:176-234`, `SkillMethodBeanPostProcessor.java:274-296`, `SkillMethodBeanPostProcessor.java:470-501`).

The fresh usage inventory finds 64 `ToolParam` text matches across production samples and starter tests. Application-facing annotations occur in five sample services; processor and discovery tests cover inheritance, conflicting contracts, requiredness, schema generation, binding, resource-backed references, and invocation (`loomspan-sample/src/main/java/com/lokiscale/loomspan/sample/incident/IncidentTelemetryService.java`, `loomspan-sample/src/main/java/com/lokiscale/loomspan/sample/insurance/ClaimsHistoryService.java`, `loomspan-sample/src/main/java/com/lokiscale/loomspan/sample/insurance/InsurancePolicyService.java`, `loomspan-sample/src/main/java/com/lokiscale/loomspan/sample/support/SupportCrmService.java`, `loomspan-sample/src/main/java/com/lokiscale/loomspan/sample/travel/TravelCatalogService.java`, `SkillMethodBeanPostProcessorTest.java:35-301`, `SkillMethodTargetDiscoveryIntegrationTests.java:169-224`).

`DefaultToolCallbackFactory` converts Loomspan `CapabilityMetadata` to `FunctionToolCallback` instances, preserves the Loomspan input contract through `ContractAwareToolCallback`, and routes invocation back through `CapabilityExecutionRouter` (`DefaultToolCallbackFactory.java:62-91`). Its invocation wrapper owns tool quota accounting, planning task linkage, tool frame lifecycle, unplanned-call recording, access-aware capability execution, metrics, results, failures, and canonical trace facts (`DefaultToolCallbackFactory.java:93-184`). Thus provider-facing callback representation is Spring AI-owned today, while discovery, visibility, authorization, binding, execution, quotas, and traces are Loomspan-owned behaviors.

### 7. Provider construction, retries, failures, and OpenRouter

`NamedAiConnectionRegistry` constructs all named connections at startup through `SpringAiV11ProviderIntegration` and sanitizes construction failures (`LoomspanAutoConfiguration.java:395-408`, `NamedAiConnectionRegistry.java:20-64`). Each `ProviderConnectionRuntime` groups the `ChatModel`, driver, exact attempt-ownership marker, Loomspan retry policy, and failure translator (`ProviderConnectionRuntime.java:8-27`).

Current provider construction is:

| Driver | Current client path | One-send control | Connection translation |
| --- | --- | --- | --- |
| OpenAI | Spring AI 1 `OpenAiApi` + `OpenAiChatModel` | Spring Retry template with one attempt | API key, base URL, headers, organization, project, optional completions path; `/v1` path adjustment; OpenRouter interceptor (`SpringAiV11ProviderIntegration.java:90-114`) |
| Anthropic | Spring AI 1 `AnthropicApi` + `AnthropicChatModel` | Same one-attempt template | API key, base URL, completions path, typed version and beta values (`SpringAiV11ProviderIntegration.java:116-129`) |
| Gemini | Google GenAI SDK client + Spring AI model | SDK HTTP attempts set to one and Spring Retry one attempt | API key mode or Vertex project/location/optional credentials resource (`SpringAiV11ProviderIntegration.java:131-142`, `SpringAiV11ProviderIntegration.java:153-161`) |
| Ollama | Spring AI 1 `OllamaApi` + model | Spring Retry one attempt | Base URL and capturing error handler (`SpringAiV11ProviderIntegration.java:144-151`) |

`ProviderAttemptCallAdvisor` performs `runtime.retryPolicy().maxAttempts()` iterations. Before each physical call it checks interruption, reserves the provider-attempt quota, allocates attempt identity, and records request-prepared/request-sent. On failure it translates the exception, decides retry/stop and delay, records metrics and a failed-attempt trace, then sleeps interruptibly if retrying. On success it attaches attempt context, extracts exact or heuristic usage, records response and session usage, and returns (`ProviderAttemptCallAdvisor.java:42-102`).

Failure translation recognizes normalized provider exceptions, Google `ApiException`, Spring `RestClientResponseException`, cancellation/interruption/SSL, timeout, and connectivity causes. It classifies selected HTTP status codes, parses numeric or RFC 1123 `Retry-After`, and bounds diagnostic bodies (`SpringAiV11ProviderIntegration.java:163-239`, `SpringAiV11ProviderIntegration.java:320-378`). The integration tests verify exact retry sequences, quota alignment, interruption behavior, and visible endpoint counts; the OpenRouter test observes two endpoint calls for one retryable error completion followed by success (`ModelAttemptCallAdvisorIntegrationTest.java:62-409`).

### 8. Usage, quotas, metrics, traces, and observations

`ModelUsageExtractor` reads Spring AI `ChatResponse` metadata. Positive provider token values produce `EXACT` usage; otherwise prompt/response text is estimated at roughly four characters per unit, or marked unavailable when no content exists (`ModelUsageExtractor.java:11-75`).

`DefaultSessionUsageService` maintains session-scoped skill, tool, linter retry, model-call, provider-attempt, and total-usage counts and enforces the corresponding `loomspan.session.quotas.*` values. Provider attempts are reserved before send so rejected quota attempts do not create a physical call (`DefaultSessionUsageService.java:25-126`). `MicrometerUsageMetricsRecorder` publishes Loomspan domain counters for skill calls, model calls/units, provider attempts, tool calls/accuracy, linter outcomes, and guardrail trips (`MicrometerUsageMetricsRecorder.java:17-86`).

Loomspan's current observation subsystem is its own execution-observation handle, active-execution registry, live activity projector/replay buffer, finalized trace catalog, and web adapter. There are no production imports of Spring AI observation conventions or `ObservationRegistry`. This makes Spring AI conventional observations absent from current behavior; the ticket's observation integration is a settled target input, while current Loomspan metrics and diagnostics remain separately implemented.

The canonical trace writer uses a private module-discovering Jackson mapper, appends one UTF-8 JSON record plus newline, and creates parent directories (`NdjsonTraceRecordWriter.java:13-42`). The reader streams line by line, tolerates a trailing partial active record, reconstructs chunked payloads by payload ID/index, and preserves partial chunks when an active trace is incomplete (`NdjsonExecutionTraceReader.java:18-182`). Trace contract tests assert frame ownership, failure placement/redaction, plan ownership, and absence of duplicate model events (`ExecutionTraceContractTest.java:43-230`).

Under the framework lens, the execution trace is an **Ephemeral diagnostic format** for current-run debugging. A complete canonical trace file is also the narrowly portable durable object accepted only by a Console with the exact same compatibility marker. It is not a general cross-version persisted history contract.

### 9. Jackson roles and observable serialization behavior

Fresh inventory finds 28 production files with Jackson usage. The current roles are:

| Codec role | Current owners | Observable consumers/tests |
| --- | --- | --- |
| Application input and API event projection | Boot mapper injected into `DefaultSkillTemplate`, `SkillExecutionViewMapper`, `SkillMethodBeanPostProcessor`, `SkillInputContractResolver` | `DefaultSkillTemplateTest`, application API value tests, supported-surface integration |
| YAML skill manifests | Private `YamlSkillCatalog` YAML mapper; `YamlSkillDefinition` copy mapper | Valid/invalid YAML resource corpus and catalog tests (`YamlSkillCatalog.java:53-90`, `YamlSkillCatalog.java:1013-1020`) |
| Planning documents | Private JSON and YAML mappers in `DefaultPlanningService` | Planning tests for parsing, normalization, quality retry, trace payloads (`DefaultPlanningService.java:60-81`, `DefaultPlanningService.java:637-816`) |
| Schema/input/evidence validation | `OutputSchemaValidator`, `SkillInputContractResolver`, `EvidenceBackedOutputValidator` | Schema, reflected-skill, evidence, and binding tests |
| Provider diagnostics | Private mapper in `SpringAiV11ProviderIntegration` | Provider failure and OpenRouter integration tests (`SpringAiV11ProviderIntegration.java:65`, `SpringAiV11ProviderIntegration.java:243-302`) |
| Journal and canonical NDJSON | `ExecutionJournalProjector`, `DefaultExecutionTraceHandle`, trace writer/reader | Trace JSON tests, NDJSON reader/writer tests, contract tests, Java-to-Go corpus |
| REST, problems, cursors, SSE payloads | Strict private `ObservabilityJsonCodec`; bounded page writer and DTO mapper | REST/SSE fixture corpus and servlet integration tests (`ObservabilityJsonCodec.java:12-31`) |
| Prompt/internal conversion | `MissionInputMessageFormatter`, step parsing/copy helpers | Mission and step-loop tests |

`ObservabilityJsonCodec` is explicit about lower-camel names, unknown-field rejection, ISO-style dates, and non-timestamp durations (`ObservabilityJsonCodec.java:12-23`). NDJSON writer/reader mappers use module discovery without an explicit unknown-field setting (`NdjsonTraceRecordWriter.java:15-18`, `NdjsonExecutionTraceReader.java:22-25`). YAML catalog validation supplements mapper behavior with raw-tree validation and field-specific errors (`YamlSkillCatalog.java:266-328`). These are distinct existing behaviors rather than one global mapper contract.

### 10. REST/SSE, artifact, problem, and Console consumers

The servlet observability adapter is conditional on a servlet web application and `DispatcherServlet`/`Filter`, then publishes framework-owned route, security, serialization, delivery, and controller beans (`LoomspanObservabilityWebAutoConfiguration.java:35-118`). Routes are registered programmatically through `RequestMappingHandlerMapping`, not annotation scanning (`ObservabilityRouteRegistrar.java:34-206`).

Protected protocol consumers and executable fixtures are:

- Java REST producer and exact fixtures: `ObservabilityRestController`, `ObservabilityDtoMapper`, `ObservabilityProblemMapper`, `ConsoleRestFixtureCorpusTest`, and `loomspan-console-fixtures/application-rest/*.json`.
- Java SSE producer and exact fixtures: `ObservabilityActivityStream`, `ObservabilityActivityDelivery`, `ConsoleSseFixtureCorpusTest`, and `loomspan-console-fixtures/application-sse/*.sse`.
- Java artifact/NDJSON producer and fixtures: `ObservabilityArtifactStream`, `ObservabilityArtifactDelivery`, `ConsoleArtifactFixtureCorpusTest`, `ConsoleTraceFixtureCorpusTest`, `loomspan-console-fixtures/application-artifact/*`, and `loomspan-console-fixtures/traces/*.ndjson`.
- Go application adapter and acquisition consumers: `loomspan-console/internal/applicationapi`, `loomspan-console/internal/console`, `loomspan-console/internal/traceanalysis`, and `loomspan-console/internal/artifact`.
- Browser-facing Go/TypeScript consumers: `loomspan-console/internal/browserapi` and `loomspan-console/web/src/api/contracts.ts`.

Observable semantics pinned by these tests include authentication and no-store responses, exact release identity, namespace and method rejection, Accept/query/header validation, bounded pages and continuations, problem classifications, current active/finalized views, SSE handshake/replay/capacity/failure behavior, `application/x-ndjson` artifact streaming, chunk reconstruction, trace semantic validation, and exact `consoleCompatibilityVersion` gating (`ObservabilityRestIntegrationTest.java:62-289`, `ObservabilitySseIntegrationTest.java:62-225`, `ConsoleRestFixtureCorpusTest.java:22-97`, `ConsoleSseFixtureCorpusTest.java:17-87`, `ConsoleTraceFixtureCorpusTest.java:121-537`, `loomspan-console/internal/traceanalysis/processor_test.go:58-180`).

The Console instructions classify the layers as lockstep and explicitly reject compatibility shims; exact compatibility-marker mismatch is terminal for released builds (`loomspan-console/AGENTS.md:1-18`). Consequently, any implementation-time change to REST, SSE, acquisition, problems, or consumed NDJSON is a coordinated Java/Go/TypeScript/fixture change in the same release. The ticket currently settles on retaining canonical trace and REST/SSE behavior unless concrete implementation evidence produces a conflict.

### 11. Boot auto-configuration ownership

`LoomspanAutoConfiguration` is one large composition root. It creates registries, catalog, input resolver/validator, access and visibility services, VFS and attachment services, public `SkillTemplate`, planning/state/usage/metrics, tool surface/callbacks, executors and both mission engines, named provider connections, chat model/options/advisor resolvers, `SpringAiSkillChatClientFactory`, and `ExecutionCoordinator` (`LoomspanAutoConfiguration.java:89-516`).

There are no conditional replacement beans. Optional environment inputs are obtained through `ObjectProvider` for Boot's `ObjectMapper`, `MeterRegistry`, authentication, and observability collaborators, but the produced Loomspan beans remain framework-owned (`LoomspanAutoConfiguration.java:110-120`, `LoomspanAutoConfiguration.java:287-319`, `LoomspanAutoConfiguration.java:489-516`). The servlet-specific adapter is already separated into `LoomspanObservabilityWebAutoConfiguration`; provider/chat/Jackson wiring is still mixed with stable runtime composition in the core auto-configuration.

### 12. Tests, fixtures, samples, and maintained documentation

The current executable characterization surface includes:

- Public/API boundaries: `ApplicationApiValueTest`, `SkillMethodTest`, both architecture tests, `SupportedSurfaceIntegrationTest`.
- Provider and retries: `SpringAiV11ProviderIntegrationTest`, `ConnectionProtocolTest`, `ModelAttemptCallAdvisorIntegrationTest`, `ProviderRetryDeciderTest`, `SensitiveConnectionDataRedactionTest`.
- Chat/advisors: `SpringAiSkillChatClientFactoryTests`, advisor resolver tests, linter/output-schema/evidence advisor tests.
- Tool and reflection contracts: `ToolCallbackFactoryTest`, `SkillMethodBeanPostProcessorTest`, `SkillMethodTargetDiscoveryIntegrationTests`, input contract/schema tests.
- Mission/planning/step orchestration: coordinator integration tests, `MissionExecutionEngineTest`, `PlanningServiceTest`, `StepLoopMissionExecutionEngineTest`, prompt and action validation tests.
- Serialization and protocol: trace JSON/NDJSON/contract/corpus tests, REST/SSE/artifact corpus tests, and servlet integration tests.
- Usage and observability: model usage, session usage, Micrometer, live activity, catalog, retention, and web tests.

Ten test files hand-implement Spring AI `ChatClient` fluent interfaces: `SpringAiSkillChatClientFactoryTests`, `MissionExecutionEngineTest`, `SimpleChatClient`, three coordinator/validator tests, `PlanningServiceTest`, `FakeCoordinatorChatClient`, `StepLoopMissionExecutionEngineTest`, and `ExecutionTraceContractTest`. This is verified in-repository coupling evidence, not supported-contract evidence.

The sample application consumes `SkillTemplate`, `SkillExecutionView`, YAML manifests, named connections/model aliases, and application-facing `@SkillMethod`/Spring AI `@ToolParam`. Maintained docs that describe the affected surfaces are the root README, sample README, and `ai/skill-authoring/*`, especially `mental-model.md`, `model-selection-and-connections.md`, and `traces-and-debugging.md` (`README.md:77-169`, `README.md:242-288`, `loomspan-sample/README.md:1-33`, `loomspan-sample/README.md:858-930`).

## Contract Classification and Evidence Map

| Surface | Lens category today | Technical exposure | Deliberate-contract evidence | Protected / verified consumers | PR 26 settled disposition |
| --- | --- | --- | --- | --- | --- |
| Seven `com.lokiscale.loomspan.api` types | Application API | Public declarations | Explicit architecture allowlist, README, integration test, sample | Application code and sample controllers | Retain useful capability; signatures change only where ticket decisions require it |
| Loomspan-specific SPI / bean overrides | Supported SPI | Many public internal interfaces/constructors/beans | Explicit evidence says none; empty override allowlist; `@ConditionalOnMissingBean` forbidden | No protected SPI consumer identified | No compatibility bridge; internal seams may be replaced atomically |
| `@SkillMethod` and parameter metadata | Application API plus configuration-like author semantics | Loomspan annotation plus current Spring AI `@ToolParam` dependency | README/sample/processor tests | Sample services and application skill authors | Retain `@SkillMethod`; replace application-facing `@ToolParam` with Loomspan `@SkillParam` |
| YAML skill syntax and validation | Configuration and manifest contract | YAML DTOs and catalog implementation | Extensive docs, sample manifests, 89-test corpus | Skill authors, sample, catalog/registrar | Retain capability; characterize and migrate selected schema behavior |
| `loomspan.*` properties/model aliases | Configuration and manifest contract | Public properties classes and generated metadata | README, author guide, sample config, metadata/property tests | Application configuration | Retain named connections/aliases; redesign provider fields listed by ticket |
| Provider retry/failure/attempt behavior | Configuration contract plus internal operational semantics | Internal classes/advisor and metrics | README, author guide, exact-attempt tests, trace/metric tests | Quotas, traces, metrics, diagnostics | Retain Loomspan logical/physical attempt policy; rebuild provider controls |
| REST/SSE/problems/acquisition | Persisted or serialized cross-component protocol for same release | Internal Java DTO/controller types | Exact Java fixtures and Go/TS consumers | Loomspan Console | Retain unless concrete conflict; coordinate all layers for any delta |
| Canonical NDJSON / portable trace | Ephemeral diagnostic format; complete file is narrowly portable same-version artifact | Public internal records/readers and on-disk files | Lens policy, corpus, Go processor, compatibility marker | Java debugger/observability and Console | Retain canonical behavior unless explicit producer/consumer decision |
| `SkillExecutionView` events | Application API carrying ephemeral diagnostics | Public immutable records | API allowlist, sample observer flows, tests | Application observers | Retain useful current-execution diagnostics |
| Loomspan Micrometer names/tags | Observable operational behavior; current classification not separately allowlisted | Production counters | README and metrics tests | Application metrics registries | Compose with Spring AI observations without duplicating Loomspan counters |
| Spring AI chat/advisor/tool/provider types under `.internal` | Internal or accidentally exposed implementation | 31 production files; public internal interfaces/constructors | No API/SPI allowlist; README says not to replace | Only in-repository implementation/tests | Replace or confine behind the ticket's narrow interaction boundary |
| Auto-configuration bean decomposition | Internal or accidentally exposed implementation | Public auto-config classes and many bean methods | Framework integration allowlist only; no replacement seams | Spring Boot startup | May be split as ticket directs; not a supported bean SPI |
| Private Jackson mapper construction | Internal implementation with serialized effects at its outputs | 28 production files | Output fixtures establish behavior, not mapper identity | YAML, REST/SSE, traces, planning, validation | Move to purpose-owned Jackson 3 codecs while retaining/redesigning classified outputs |
| Exact internal provider/framework messages | Internal or accidentally exposed implementation | Exceptions and test assertions | No deliberate allowlist found | Some tests only | Ticket classifies stable categories/actionable public messages as useful, incidental wording as removable |

## Architecture Documentation

### Current ownership boundaries

Loomspan already owns the product semantics that surround model calls: public skill identity, YAML discovery, access and visibility, planning, capability routing, execution frames, semantic validation, quotas, provider retry decisions, normalized failure categories, usage accounting, domain metrics, journal/trace records, and Console projection. Spring AI owns the current `ChatClient`, advisors, message/request/response types, provider model implementations, options, and tool callback representation. The current boundary is porous because those Spring AI types flow through core orchestration interfaces.

Provider integration is centralized but version-specific. `SpringAiV11ProviderIntegration` is the construction and failure-translation boundary; `ProviderConnectionRuntime` transports its result into the neutral retry advisor; `SpringAiSkillChatClientFactory` performs provider-specific option and advisor assembly. This creates three main integration seams: provider construction, chat/options/advisors, and orchestration/tool callback flow.

Serialization is role-based in behavior but not in type ownership. Some application conversion uses Boot's mapper, while YAML, planning, trace, REST/cursor, validation, provider diagnostic, and copy helpers instantiate private mappers. Retained behavior is therefore established by the executable fixtures at each output boundary rather than by a single global mapper.

The Console protocol is a repository-local multi-language boundary. Java produces REST/SSE/NDJSON; Go acquires, validates, projects, stores transient artifacts, and serves browser APIs; TypeScript consumes Go contracts. The committed corpora and exact compatibility marker are the executable link across these layers.

### Ticket-settled target delta, recorded for planning context

The ticket settles the following target inputs; this research does not redesign them:

- One narrow Loomspan model-interaction boundary will be used by mission, planning, and step orchestration, with Spring AI requests/responses/clients/messages/metadata/tool callbacks contained inside its implementation.
- Spring AI 2 `ToolCallingAdvisor` will own the generic tool loop; Loomspan retains discovery/access, execution, quotas, retry decisions, validation, traces, and usage.
- Each provider SDK/framework layer will make exactly one HTTP send per Loomspan attempt.
- OpenAI and Anthropic will use official SDK clients; Gemini and Ollama will use Framework 7's zero-retry core template.
- OpenRouter's opt-in HTTP-200 error-completion rejection remains.
- Loomspan clients will be assembled explicitly rather than implicitly applying global `ChatClientBuilderConfigurer` customizers.
- Application-facing parameter metadata becomes Loomspan `@SkillParam`.
- Loomspan uses Jackson 3 APIs without the Boot Jackson 2 compatibility module; private Jackson 2 pulled by an official SDK is outside Loomspan's direct API/dependency target.
- Canonical trace and REST/SSE behavior remain unless implementation evidence exposes a concrete conflict.

The completed platform experiments provide framework-side evidence for these inputs: target dependency resolution, nine initial compile errors, SDK/core retry counts, provider URL/header construction, OpenRouter behavior, AI 2 advisor recursion/automatic registration, builder-oriented options, `ChatClientBuilderConfigurer` behavior, Jackson 3 representative behavior, and Boot module moves (`ai/thoughts/research/2026-08-12-spring-platform-upgrade-research.md:67-78`, `ai/thoughts/research/2026-08-12-spring-platform-upgrade-research.md:81-119`, `ai/thoughts/research/2026-08-12-spring-platform-upgrade-research.md:243-259`).

## Code References

- `pom.xml:45-50` - Java, Boot, and Spring AI version baseline.
- `loomspan-spring-boot-starter/pom.xml:19-103` - Starter AI, Jackson, Boot, MVC, security, and test dependencies.
- `loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/autoconfigure/LoomspanProperties.java:23-535` - Strict configuration model, validation, provider fields, retries, and model aliases.
- `loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/autoconfigure/LoomspanAutoConfiguration.java:89-516` - Core composition root.
- `loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/springai/v1_1/SpringAiV11ProviderIntegration.java:60-378` - Provider construction, one-send controls, OpenRouter, and failure translation.
- `loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/chat/SpringAiSkillChatClientFactory.java:82-265` - Options and advisor assembly.
- `loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/chat/ProviderAttemptCallAdvisor.java:30-142` - Loomspan physical attempts, quotas, usage, and trace recording.
- `loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/core/ExecutionCoordinator.java:60-176` - Mission entry and current propagation of Spring AI types.
- `loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/core/SkillMethodBeanPostProcessor.java:176-296` - Parameter contract discovery and `@ToolParam` use.
- `loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/runtime/tool/DefaultToolCallbackFactory.java:62-184` - Capability-to-callback conversion and Loomspan tool lifecycle.
- `loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/runtime/trace/NdjsonTraceRecordWriter.java:13-42` - Canonical NDJSON writing.
- `loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/runtime/trace/NdjsonExecutionTraceReader.java:18-182` - Streaming and chunk reconstruction.
- `loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/observability/web/ObservabilityJsonCodec.java:12-31` - Strict REST/cursor JSON behavior.
- `loomspan-spring-boot-starter/src/test/java/com/lokiscale/loomspan/architecture/LoomspanPublicSurfaceArchitectureTest.java:29-316` - Application/framework/internal exposure classification.
- `loomspan-spring-boot-starter/src/test/java/com/lokiscale/loomspan/architecture/LoomspanAutoConfigurationBoundaryTest.java:49-114` - No supported bean overrides.
- `loomspan-spring-boot-starter/src/test/java/com/lokiscale/loomspan/internal/chat/ModelAttemptCallAdvisorIntegrationTest.java:62-409` - Attempt, retry, quota, interruption, and OpenRouter counts.
- `loomspan-spring-boot-starter/src/test/java/com/lokiscale/loomspan/internal/runtime/trace/ConsoleTraceFixtureCorpusTest.java:121-537` - Java-to-Go trace corpus semantics.
- `loomspan-spring-boot-starter/src/test/java/com/lokiscale/loomspan/internal/observability/web/ConsoleRestFixtureCorpusTest.java:22-97` - Exact REST/problem fixtures.
- `loomspan-spring-boot-starter/src/test/java/com/lokiscale/loomspan/internal/observability/web/ConsoleSseFixtureCorpusTest.java:17-87` - Exact SSE fixtures.
- `loomspan-console/AGENTS.md:1-35` - Console lockstep protocol and verification policy.
- `README.md:77-169` - Named connections, retries, application API, and integration guidance.

## Historical Context (from ai/thoughts/)

- `ai/thoughts/research/2026-08-12-spring-platform-upgrade-research.md` - Completed framework/source experiments and preliminary ownership/contract audit. It is supplementary to the fresh live-code map above.
- `ai/thoughts/phases/2026-08-12-loomspan-active-roadmap.md` - Migration charter, atomic sequencing rationale, and completion criteria.
- `ai/thoughts/tickets/loomspan-platform-pr-26-spring-boot-4-spring-ai-2.md` - Settled target architecture, scope, milestones, acceptance signals, and guardrails.
- `ai/thoughts/framework-feature-design-lens.md` - The six contract categories and pre-1.0 evidence rules used by this report.
- `ai/thoughts/phases/loomspan_console_phase_3_llm_runtime_inspector.md` and `ai/thoughts/phases/loomspan_console_workflows.md` - Active Console product/workflow context for the retained observability protocols.

## Related Research

- [Spring Boot 4 / Spring AI 2 / Jackson 3 Upgrade Research](./2026-08-12-spring-platform-upgrade-research.md)

## Open Questions

The following details are intentionally left for the detailed plan/testing plan, as assigned by the ticket:

- The exact then-current Boot 4.1.x and Spring AI 2.0.x patch versions.
- Concrete type names and package layout for the narrow model-interaction and neutral capability-description boundary.
- The exact immutable option-contribution signatures for each provider.
- The complete file-by-file bean split between platform integration and stable runtime composition.
- The final property inventory after removing completion-path and typed Anthropic version fields and translating supported SDK headers/identity fields.
- The exact Java HTTP fixtures and assertions for OpenAI, Anthropic, Gemini, and Ollama one-send behavior on the target implementation.
- Whether implementation exposes any concrete Jackson 3 conflict in a retained JSON/YAML/NDJSON/REST/SSE fixture; none is demonstrated by the current Boot 3 baseline or the completed representative Jackson 3 probes.
- Whether implementation exposes a concrete need to change the Console protocol. If it does, the compatibility-marker and coordinated Java/Go/TypeScript/fixture decision must be recorded explicitly.

No current live-code evidence contradicts the ticket's settled architecture inputs. No compatibility shim or dual-platform consumer was found to be a protected contract.
