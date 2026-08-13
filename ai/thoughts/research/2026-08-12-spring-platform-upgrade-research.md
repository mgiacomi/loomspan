# Spring Boot 4 / Spring AI 2 / Jackson 3 Upgrade Research

## Purpose and status

This is the concise research artifact required by the active roadmap. It is a
first-pass architecture and substitution audit, not an implementation plan or
ticket. It records enough evidence to choose the target design before dividing
the work.

The recommended target baseline is Spring AI 2.0.x on Spring Boot 4.1.x, Spring
Framework 7, Java 21, and Jackson 3. Boot 4.1 is GA, includes the fixes from
Boot 4.0.7, and is explicitly supported by Spring AI 2.0.x. Targeting 4.0 first
would create another near-term platform upgrade without giving Loomspan a useful
intermediate state.

No implementation files were changed during this research. A read-only Maven
compile probe used command-line version overrides; it did not edit the POM.

## Executive findings

- This is an architectural migration, not a version bump. The first production
  compile reports only nine errors, but those errors expose changed provider,
  retry, option-builder, tool-loop, Boot module, and Jackson ownership.
- The main migration seam should be one narrow AI execution subsystem. Spring
  AI types may exist inside that subsystem, but mission, planning, and step
  orchestration should not pass `ChatClient`, `ChatResponse`, and
  `ToolCallback` through their own interfaces.
- Loomspan should adopt Spring AI 2's `ToolCallingAdvisor` as the generic tool
  loop. Loomspan still owns capability selection, access checks, execution,
  planning semantics, trace events, quotas, and tool results.
- Loomspan should continue to own logical provider attempts and retry policy.
  Every provider SDK/framework retry layer must be configured for one physical
  send so there is no hidden retry multiplier.
- OpenAI and Anthropic provider construction must be rewritten around their
  official Java SDK clients. Preserving the Spring AI 1 REST-client construction
  contract would create compatibility machinery around APIs that no longer
  exist.
- Jackson 3 is broad rather than conceptually difficult: 28 production files
  refer directly to Jackson, with several private mappers in addition to the
  Boot mapper. The work needs contract fixtures, not merely import changes.
- One atomic implementation PR remains the best default. Boot 4, Spring AI 2,
  provider construction, advisor behavior, and Jackson 3 cross the same seams;
  independently mergeable platform states would either be unsupported or edit
  those seams repeatedly.

## Evidence from the current code

The project currently targets Boot 3.5.11 and Spring AI 1.1.6. The starter has
31 production files importing Spring AI and 28 production files referring
directly to Jackson. Ten test files implement portions of Spring AI's fluent
`ChatClient` surface as handwritten doubles. Those doubles are a useful signal
that third-party interfaces have spread past their useful integration boundary.

The coupling is concentrated in these paths:

| Concern | Current ownership | Main problem exposed by the upgrade |
| --- | --- | --- |
| Provider construction | `SpringAiV11ProviderIntegration` | Removed `OpenAiApi` and `AnthropicApi`; official SDK builders replace the old REST clients. |
| Provider attempts | `ProviderAttemptCallAdvisor` plus provider retry controls | SDK defaults can retry underneath Loomspan, multiplying attempts, quotas, and trace events. |
| Client assembly | `SpringAiSkillChatClientFactory` | AI 2 options are builder-oriented; automatic tool-advisor registration and observations change assembly. |
| Mission and planning | runtime engines, planning service, message sender | Internal Loomspan interfaces expose `ChatClient`, `ChatResponse`, and `ToolCallback`. |
| Tool execution | `DefaultToolCallbackFactory` and Spring AI 1 model loop | AI 2 moves the loop out of `ChatModel` and into `ToolCallingAdvisor`. |
| Output validation | output-schema, linter, and evidence advisors | Their recursive calls must have intentional placement relative to the new tool loop and attempt advisor. |
| Serialization | YAML catalog, planning, traces, web DTOs, validation | Many private mappers can drift from Boot's Jackson 3 mapper and retained wire contracts. |
| Composition | one large `LoomspanAutoConfiguration` | Platform-specific provider/chat wiring and stable Loomspan services are assembled together. |

The compile probe was rerun against the actual target, Spring AI 2.0.0 and
Boot 4.1.0, and immediately found the same nine initial errors:

- removed OpenAI and Anthropic API types;
- removed Spring Retry imports in favor of provider SDK retry controls or
  Spring Framework 7 core retry;
- the Boot 4 `WebMvcAutoConfiguration` package move; and
- the removed Anthropic thinking enum/API.

Additional source inspection found that AI 2 `ChatClient.Builder.defaultOptions`
and per-call `options` consume option builders. The existing adapters return
built option values, so they require redesign even though that was not reached
by the first compiler pass.

## Spring AI 2 substitution audit

| Facility | Decision | Consequence for Loomspan |
| --- | --- | --- |
| `ToolCallingAdvisor` and automatic registration | **Adopt** | Delete any assumption that a `ChatModel` owns the tool loop. Use one explicit/auto-registered tool advisor and test its ordering. Keep Loomspan tool callbacks for Loomspan-owned execution semantics. |
| Advisor ordering and recursive advisors | **Integrate** | Define and test which work happens once per mission call, once per tool-loop iteration, once per semantic retry, and once per physical provider attempt. Do not rely only on numeric constants copied from AI 1. |
| Immutable provider option builders | **Adopt** | Replace `SkillChatOptionsAdapter`'s built-value contract with a request/options contribution that can be applied to an AI 2 builder. Avoid a compatibility adapter for AI 1 values. |
| Official OpenAI and Anthropic Java SDKs | **Adopt** | Rewrite client construction and HTTP customization. Keep only connection fields supported by the new clients or justified by a real Loomspan use case. |
| SDK retry controls | **Integrate** | Set OpenAI/Anthropic SDK `maxRetries` to zero when Loomspan owns attempts. HTTP-counting probes verified that zero produces one send and one produces two sends for both SDKs. |
| Spring Framework 7 core retry | **Integrate narrowly** | Gemini and Ollama AI 2 builders still accept core `RetryTemplate`. A probe verified that `RetryPolicy.withMaxRetries(0)` invokes once. |
| `ChatClientBuilderConfigurer` | **Reject as a default Loomspan dependency** | Source inspection shows that it only applies global `ChatClientBuilderCustomizer` values; it does not wire observations or provider settings. Applying global customizers would silently broaden Loomspan's configuration contract. Construct the builder with observation conventions and the tool-advisor builder explicitly instead. |
| Native structured output | **Defer** | It can later remove prompt-formatting and reduce validation failures, but support varies by provider/model and would change current evidence/linter/retry behavior. Migrate current semantics first, then evaluate per model. |
| Spring AI JSON Schema facilities | **Integrate at the edge** | Reuse them for Spring AI tool/model schemas. Keep Loomspan's capability input contract neutral so prompts and validation do not depend on `ToolCallback`. |
| Chat client, advisor, model, tool, and HTTP observations | **Integrate** | Enable conventional Micrometer spans/metrics and propagation. They complement rather than replace Loomspan's canonical journal, quota accounting, portable trace, and diagnostic payloads. |
| Provider usage, finish reason, tool calls, diagnostics | **Integrate selectively** | Normalize only data needed by Loomspan-owned usage, retry, and trace semantics. Do not expose entire provider/Spring metadata objects as contracts. |
| Tool search and other new agent features | **Defer** | They expand platform behavior and are outside the no-feature-creep migration charter. |

### Critical ordering invariant

AI 2 auto-registers `ToolCallingAdvisor` at
`Ordered.HIGHEST_PRECEDENCE + 300`. Its order determines whether another
advisor runs outside the tool loop once or inside it for every model
iteration. Loomspan currently places linter, output-schema, and evidence
advisors near the chat-memory precedence and the provider-attempt advisor at
`LOWEST_PRECEDENCE - 1`.

The target chain should make these scopes explicit:

```text
one Loomspan model interaction
  -> semantic/output policy (may request a semantic retry)
    -> Spring AI tool loop (may request another model turn)
      -> Loomspan physical-attempt advisor (may retry one provider send)
        -> provider SDK/framework configured for exactly one HTTP attempt
```

The ordering probe verified this shape. With semantic policy immediately
outside the tool advisor and an attempt counter immediately inside, two semantic
attempts that each made one tool call produced four model turns, four attempt
observations, and two tool executions. Automatic `ToolCallingAdvisor`
registration behaved the same as explicit registration. The implementation
must retain that test with Loomspan's real advisors and trace assertions.

## Target ownership

### Retain as Loomspan-owned semantics

- skill discovery, model aliases, capability visibility, and access control;
- mission and step orchestration, planning policy, timeouts, and cancellation;
- capability invocation and input contracts;
- logical provider retry decisions, quotas, failure classification, and
  bounded diagnostics;
- semantic validation/retry, evidence rules, and linter policy;
- execution journal, canonical/portable traces, usage accounting, and console
  projections; and
- public `SkillTemplate` behavior and execution events, subject to the contract
  review below.

### Move behind the AI execution boundary

- `ChatModel` and official provider clients;
- `ChatClient` creation and options;
- Spring AI advisors, requests, responses, messages, and metadata extraction;
- conversion between neutral Loomspan capability descriptors and Spring AI
  `ToolCallback` values; and
- provider-specific HTTP customization and exception translation.

The boundary should be smaller than a parallel AI framework. A likely shape is
a Loomspan-owned model-interaction request/result plus a gateway used by mission,
planning, and step services. The Spring AI implementation can still assemble a
`ChatClient` internally. This would remove the large fluent-interface test
doubles while preserving direct access to Spring AI where the integration needs
it.

### Reshape or delete

- Rename/delete the versioned `internal.springai.v1_1` package rather than
  layering an AI 2 adapter beside it.
- Split provider/chat/Jackson integration wiring out of the oversized
  auto-configuration so stable runtime services do not know provider details.
- Replace multiple ad hoc JSON/YAML mappers with injected, purpose-named Jackson
  3 mappers or codecs. Deterministic trace serialization may intentionally use
  its own mapper, but that exception should be explicit and fixture-tested.
- Stop passing Spring AI tool callbacks through planning and step-prompt code;
  those services need Loomspan capability descriptors, not executable provider
  adapters.
- Delete provider configuration fields that only reproduce removed REST-client
  implementation details unless a current supported endpoint requires them.
- Retain base URL, API key, general headers, OpenAI organization/project, and
  the OpenRouter compatibility profile. Remove OpenAI and Anthropic custom
  completion-path fields; the official SDKs compose their fixed endpoint below
  the configured base URL. Remove the typed Anthropic version override and use
  the SDK default; beta/custom headers can use the existing general header map.

## Preliminary contract classification

This is deliberately preliminary. It identifies decisions the migration ticket
must make, without treating existing compatibility as sacred.

| Contract | Initial classification | Reason and migration obligation |
| --- | --- | --- |
| `SkillTemplate`, `SkillMethod`, execution view/events | **Retain capability; redesign only with evidence** | They form Loomspan's small useful application API and do not inherently obstruct AI 2. Update signatures only where the target execution boundary materially improves them. |
| Spring AI `@ToolParam` on application methods | **Redesign** | Replace it with a small Loomspan-owned `@SkillParam` contract carrying description and requiredness. Those values drive Loomspan schema generation and argument binding, so this is a real Loomspan semantic boundary rather than a duplicate AI framework. Update processor, sample, and contract tests together. |
| YAML skill capability and format | **Retain capability; permit schema redesign** | It is core product behavior. Jackson 3 parsing plus unknown-field, defaults, and validation fixtures must define the chosen format. |
| `loomspan.connections.*` and model aliases | **Retain concept, redesign provider fields** | Named independent connections serve Loomspan and prevent accidental inheritance from global `spring.ai.*`. Reconcile fields with official SDK capabilities and remove obsolete endpoint details. |
| Provider retries and failure categories | **Retain semantics, redesign implementation** | They drive quotas and diagnostics. Tests must prove exact HTTP attempt counts and normalized failures across all providers. |
| REST/SSE boundaries | **Retain unless a concrete architecture conflict appears** | Changing them adds consumer migration without currently simplifying the AI boundary. Re-run endpoint and stream tests on Boot 4's modular Web MVC stack. |
| Canonical NDJSON and portable trace | **Retain initially** | It is consumed by the console. Jackson 3 golden fixtures must prove ordering, omission, temporal, numeric, and unknown-field behavior. Redesign only via an explicit producer/consumer version decision. |
| Metrics and observations | **Redesign/additive composition** | Keep Loomspan domain metrics and trace facts; adopt Spring AI conventional observations without duplicating counters or exporting sensitive content by default. |
| Internal Spring AI-facing interfaces | **Remove/redesign** | They are implementation seams, not product contracts, and cause broad upgrade/test churn. Replace with narrow Loomspan-owned interaction and capability types. |
| Exact internal exception messages | **Remove as a contract** | Keep stable error categories and actionable public messages where useful; stop tests from pinning incidental provider/framework wording. |

## Jackson 3 migration shape

Boot 4 makes Jackson 3 (`tools.jackson.*`) the preferred/default mapper while
Jackson annotations remain under `com.fasterxml.jackson.annotation`. Loomspan
should use Boot's Jackson 3 support directly and must not retain the deprecated
Jackson 2 compatibility module.

The dependency probe exposed an important distinction: Spring AI 2 itself uses
Jackson 3, while the official OpenAI SDK currently brings Jackson 2 transitively
for its private codec. Loomspan therefore cannot promise that no Jackson 2 jar
exists at runtime. The enforceable target is no Loomspan Jackson 2 API usage, no
direct Loomspan Jackson 2 dependency, and no Boot Jackson 2 compatibility
module. The SDK's isolated transitive implementation dependency is acceptable.

Before changing imports, identify mapper roles:

1. application HTTP JSON;
2. canonical and portable NDJSON trace codecs;
3. YAML skill and planning-document codecs;
4. schema/validation tree processing; and
5. internal copy/conversion helpers.

For each retained wire format, add or preserve golden read/write fixtures before
the mapper replacement. Pay particular attention to record construction,
unknown fields, case/enums, Java time, optional/null omission, numeric types,
map ordering, and newline-delimited output. A Jackson 3 probe verified record
binding, Java time output, null omission, insertion-ordered maps, annotations,
and YAML record binding. The existing characterization suites are also green:
89 YAML catalog tests, 12 trace/NDJSON contract and corpus tests, one REST
fixture corpus test, and 15 reflected-skill contract tests. Port these fixtures
before removing Loomspan's Jackson 2 imports and direct dependencies.

## Proposed migration boundary

Create one migration ticket and one atomic implementation PR, organized into
reviewable commits rather than multiple merge-dependent tickets:

1. characterize retained contracts and HTTP attempt/tool-loop counts;
2. change dependency/module baselines and Jackson codecs;
3. replace provider construction and make one-send retry controls explicit;
4. introduce the narrow model-interaction boundary and move Spring AI types to
   its implementation;
5. assemble the AI 2 client/advisor/tool chain and prove ordering;
6. update auto-configuration, observations, samples, and documentation; and
7. remove all Boot 3, Spring AI 1, Loomspan-owned Jackson 2, compatibility, and
   obsolete test code.

A separate preparatory PR is justified only if it is independently valuable,
green on the old platform, and will not touch the same provider/chat/Jackson
seams again. At present no such split is compelling.

## Experiment results

The disposable probes live only under ignored Maven `target` output and did not
change production code or project POMs.

| Experiment | Result | Ticket consequence |
| --- | --- | --- |
| Boot 4.1 / AI 2 dependency and compile probe | Dependency resolution succeeds; compilation reaches nine initial provider/retry/Web MVC package failures. | Use Boot 4.1.x directly; no Boot 4.0 intermediate. |
| OpenAI and Anthropic retry counting | For both SDKs, `maxRetries=0` sent once and `maxRetries=1` sent twice. | Configure zero SDK retries and keep the Loomspan attempt advisor. |
| Framework 7 core retry counting | `RetryPolicy.withMaxRetries(0)` invoked once. | Use this one-send template for Gemini and Ollama. |
| Provider request construction | OpenAI composes `/chat/completions`; Anthropic composes `/v1/messages`; base paths and custom headers are retained. | Remove custom completion-path properties; translate useful identity/header fields. |
| OpenRouter HTTP-200 error completion | The SDK returned unsafe partial content and normalized `finish_reason:error` to `_UNKNOWN`; an AI 2 OkHttp customizer successfully rejected it before decoding. | Retain the OpenRouter compatibility profile and port the bounded interceptor/failure translation. |
| Advisor/tool-loop recursion | Two semantic attempts, each with a tool call, produced four model turns and four inner attempt-advisor calls. Auto-registration worked. | Put semantic validators outside and physical attempts inside `ToolCallingAdvisor`; retain an integration test. |
| `ChatClientBuilderConfigurer` source audit | It only applies global client customizers. Observability and tool-advisor components are supplied when constructing the builder. | Do not apply it by default; explicitly wire Loomspan clients with observation registries/conventions and the selected tool advisor. |
| Jackson dependency and behavior probe | Spring AI uses Jackson 3; the OpenAI SDK retains private Jackson 2 dependencies. Representative Jackson 3 JSON/YAML contracts pass. | Remove Loomspan's Jackson 2 usage, not the SDK's transitive codec; port existing golden fixtures. |
| Boot modularization inventory | Production has one moved Web MVC auto-configuration import. Tests have additional moved Web MVC, Security, and test-auto-configuration imports; both module POMs use renamed web starters. | Replace `spring-boot-starter-web` with `spring-boot-starter-webmvc`, use Boot 4 module packages, and expect most mechanical Boot churn in web integration tests. |

These experiments close the architecture questions needed before ticketing.
Implementation still needs provider-specific construction tests for Gemini and
Ollama, but their architectural ownership is no longer uncertain and those
tests belong in the migration acceptance suite rather than another research
phase.

## Primary sources

- [Spring AI getting started and compatibility](https://docs.spring.io/spring-ai/reference/getting-started.html)
- [Spring AI 2 upgrade notes](https://docs.spring.io/spring-ai/reference/upgrade-notes.html)
- [Spring AI tool calling](https://docs.spring.io/spring-ai/reference/api/tools.html)
- [Spring AI `ToolCallingAdvisor`](https://docs.spring.io/spring-ai/reference/2.0-SNAPSHOT/api/tools/tool-calling-advisor.html)
- [Spring AI structured output](https://docs.spring.io/spring-ai/reference/api/structured-output-converter.html)
- [Spring AI observability](https://docs.spring.io/spring-ai/reference/observability/)
- [Spring AI Anthropic SDK migration](https://docs.spring.io/spring-ai/reference/api/chat/anthropic-migration.html)
- [Spring Boot 4 migration guide](https://github.com/spring-projects/spring-boot/wiki/Spring-Boot-4.0-Migration-Guide)
- [Spring Boot 4.1 release notes](https://github.com/spring-projects/spring-boot/wiki/Spring-Boot-4.1-Release-Notes)
- [Spring Boot Jackson support](https://docs.spring.io/spring-boot/reference/features/json.html)
- [Spring Framework 7 resilience](https://docs.spring.io/spring-framework/reference/core/resilience.html)
