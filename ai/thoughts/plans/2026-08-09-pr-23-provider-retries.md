# PR 23 Provider Retries Implementation Plan

## Overview

Implement bounded, observable retries for synchronous model-provider calls while
making Loomspan the sole owner of retry cardinality for the supported Spring AI
1.1.6 call paths. The implementation adds a version-scoped Spring AI integration
boundary, explicit OpenRouter error-completion decoding, per-connection retry
configuration, physical-attempt accounting, failed-attempt trace facts, and a
coherent Java-to-Go-to-browser diagnostic experience.

The feature is runtime and application configuration. YAML skills continue to
select only a framework model alias; they cannot configure or override provider
retry behavior or the session-wide provider-attempt quota.

## Current State Analysis

- `ModelAttemptCallAdvisor` is Loomspan's final pre-provider advisor. It creates
  one attempt, records `MODEL_REQUEST_PREPARED` and `MODEL_REQUEST_SENT`, calls
  `chain.nextCall(request)`, and records `MODEL_RESPONSE_RECEIVED` plus usage only
  after a successful return
  (`loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/chat/ModelAttemptCallAdvisor.java:43`).
- `SpringAiSkillChatClientFactory` appends that advisor after resolved semantic
  advisors, which makes it the correct physical-attempt boundary without
  re-entering linter, schema, or evidence advisors
  (`loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/chat/SpringAiSkillChatClientFactory.java:119`).
- `ModelTraceContext` owns one retry sequence and one monotonic attempt counter,
  but its attempt metadata contains only `retrySequenceId`, `attemptId`, and
  `attemptNumber`
  (`loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/core/ModelTraceContext.java:60`).
- The OpenAI, Anthropic, Gemini, and Ollama connection factories construct their
  Spring AI chat models without supplying a retry template. Spring AI 1.1.6
  therefore supplies `RetryUtils.DEFAULT_RETRY_TEMPLATE` beneath Loomspan's
  attempt advisor. Inspection of the pinned 1.1.6 sources confirms that all four
  chat-model builders expose `retryTemplate(RetryTemplate)`.
- `OpenAiApi.Builder` in Spring AI 1.1.6 exposes `restClientBuilder(...)` and
  `responseErrorHandler(...)`. Its `ChatCompletion.Choice` decodes
  `finish_reason` directly into `ChatCompletionFinishReason`, which does not
  contain OpenRouter's `error` value. This is the dependency seam and failure
  mode described by the ticket.
- `DefaultSessionUsageService.recordModelResponse` increments `modelCalls` only
  after a response and enforces its quota after incrementing
  (`loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/runtime/usage/DefaultSessionUsageService.java:41`).
  Failed sends are therefore absent from current accounting.
- `SessionUsageSnapshot` and `ConfiguredLimitsSnapshot` contain no physical
  provider-attempt counter or limit. Their serialized shapes flow into terminal
  traces, live execution snapshots, REST DTOs, Go analysis, fixtures, and
  TypeScript contracts.
- PR 22 is present on `main`. `LoomspanSession.recordFailure` deduplicates a
  throwable and its bounded cause chain by identity and emits one bounded
  `ERROR_RECORDED` fact at the closest active frame
  (`loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/core/LoomspanSession.java:263`).
  It does not currently offer a way to associate a known provider attempt with
  a throwable before a later execution boundary records that failure.
- Go accepts only `PREPARED -> SENT -> RESPONSE_RECEIVED`. After processing, it
  emits `MODEL_ATTEMPT_RESPONSE_MISSING` for every attempt without a response
  (`loomspan-console/internal/traceanalysis/attempts.go:53`,
  `loomspan-console/internal/traceanalysis/processor.go:261`).
- The browser currently renders attempt number and usage completeness, but has
  no failed-attempt outcome, reason, retry decision, delay, or attempt payload
  (`loomspan-console/web/src/observability/TraceRecords.tsx:7`).
- The executable fixture corpus is the current Java-to-Go semantic contract.
  Trace formats are current-release diagnostics rather than a durable historical
  schema, so writers, readers, fixtures, and browser consumers must change
  atomically without a legacy reader.

## Desired End State

For each unchanged synchronous request, Loomspan performs at most the configured
number of physical provider attempts. Dependency-owned application-level retry
is disabled on every supported Spring AI 1.1.6 model path, so endpoint calls,
`providerAttempts`, and trace attempts have equal cardinality.

Every known attempt ends in exactly one of `MODEL_RESPONSE_RECEIVED` or
`MODEL_ATTEMPT_FAILED`. Retryable failures can schedule an interruptible bounded
delay and resend the identical `ChatClientRequest`; permanent, unknown, quota,
interruption, cancellation, and exhausted outcomes propagate precisely. A
recovered failed attempt does not emit `ERROR_RECORDED`. A terminal provider
failure is recorded once through PR 22 and links to the last failed attempt by
throwable identity/cause.

The explicit OpenRouter profile recognizes documented synchronous error
completions before Spring AI's enum decoder loses them, rejects any partial
assistant content, captures bounded provider diagnostics, and classifies only
the ticket's allowlisted error types as retryable.

Go validates the new lifecycle and retry transitions, and the existing Trace
Explorer explains initial, semantic, and provider-retry attempts chronologically
using bounded summaries and the existing payload range viewer.

Verification is complete when:

- a retryable OpenRouter error completion followed by success produces exactly
  two endpoint calls and two visible attempts;
- a three-attempt policy never produces 30 dependency calls;
- terminal provider failure creates one frame-linked `ERROR_RECORDED` linked to
  the last attempt;
- `providerAttempts` counts every actual Loomspan-owned send while `modelCalls`
  retains its successful-response semantics;
- interruption and mission timeout stop backoff promptly without a phantom
  attempt; and
- the Java, Go, browser, fixture, architecture, and race suites pass.

### Key Discoveries

- The advisor is already ordered at the correct retry boundary; replacing it is
  smaller and safer than adding another outer execution loop.
- The landed PR 22 implementation requires one additional internal association:
  a bounded, session-local throwable/cause identity mapping to the last provider
  attempt. The provider advisor registers it only when propagating a terminal
  provider exception; `recordFailure` consumes it when creating the canonical
  error fact.
- `providerAttempts` must be added to both terminal session usage and the
  immutable run-start configured-limit snapshot. Go currently requires exactly
  five configured-limit keys, so its decoder and invalid corpus must change in
  the same commit as Java fixtures.
- The current public-surface architecture allowlist classifies the connection
  factories and related interfaces as technically public internal
  collaboration types, not Supported SPI. The new adapter types must stay
  internal and must not become replaceable Spring beans.
- The authoring knowledge base is affected for configuration ownership and
  diagnosis only. No skill YAML syntax or per-skill retry policy is introduced.

## What We're NOT Doing

- Adding provider retry fields to YAML skill manifests.
- Allowing a skill, advisor, or application callback to provide a retry
  predicate.
- Retrying tool calls, validation failures, trace failures, usage failures, or
  any work outside the raw synchronous model invocation.
- Supporting streaming retries or retries after delivered partial tokens.
- Adding cross-provider fallback, hedging, or parallel requests.
- Inferring OpenRouter from a connection name, URL, host, header, or model.
- Creating a general provider classifier registry or public adapter SPI.
- Promising compatibility with arbitrary Spring AI versions or adding runtime
  reflection/version branching.
- Adding a new deadline propagation subsystem; the existing mission future and
  cancellation remain authoritative.
- Redefining `modelCalls`, estimating usage for failed attempts, or claiming
  cost certainty for failed provider calls.
- Adding a request-cycle numbering hierarchy, retry dashboard, provider-specific
  console, attempt-diagnostic endpoint, or duplicate diagnostic store.
- Adding legacy trace readers, dual attempt formats, a new trace schema version,
  or compatibility shims.
- Secret-scanning diagnostic bodies beyond PR 22's documented bounded-capture
  policy.

## Skill-Authoring Documentation Impact

**Impact**: Affected, but no skill-managed retry configuration.

- **Rationale**: Provider retry policy, the compatibility profile, and the
  physical-attempt quota are application-owned runtime configuration. A YAML
  skill continues to select a model alias and cannot override these values.
  Skill developers do need accurate guidance when a connection retries beneath
  their model alias and when they diagnose attempt, failure, and quota evidence.
- **Documents to update**:
  - `ai/skill-authoring/model-selection-and-connections.md`: document
    application ownership of `provider-retry` and
    `openai.compatibility-profile`, defaults and bounds, and the explicit rule
    that skills cannot override either setting.
  - `ai/skill-authoring/traces-and-debugging.md`: distinguish semantic attempts
    from unchanged-request provider retries; explain `providerAttempts`, failed
    attempt outcomes, terminal linkage, incomplete gaps, and quota diagnosis.
- **Supporting evidence**: `LoomspanPropertiesTest`, connection mock-server
  tests, provider-advisor integration tests, session usage/quota tests,
  `ConsoleTraceFixtureCorpusTest`, Go fixture-corpus and invalidity tests, and
  Trace Explorer component tests.
- **Coverage table update**: Not required. The existing README already routes
  connection configuration to `model-selection-and-connections.md` and retry,
  usage, quota, and failure diagnosis to `traces-and-debugging.md`. No topic
  boundary or confidence classification changes.
- **LLM-first usability**: Lead with ownership: application configuration sets
  retry behavior and session quota; skill YAML selects only a model alias. Use
  exact property/record names, a compact defaults table, and explicit
  distinctions among semantic retry, provider retry, failed attempt, terminal
  failure, and missing evidence. Avoid duplicating implementation details from
  the ticket.

## Contract and Compatibility Impact

| Surface | Classification and supporting evidence | Planned compatibility treatment |
| --- | --- | --- |
| Application API | No change to the allowlisted `com.lokiscale.loomspan.api` entry points. Skills are still invoked through `SkillTemplate`; returned values and exceptions remain unchanged. | Preserve. Propagate the original final provider exception rather than introducing an application-facing wrapper. |
| Supported SPI | No supported customization or replacement point is added. Existing internal factories/advisors are not documented SPI, and architecture tests classify them as framework-owned collaboration. | Preserve the absence of a provider-retry SPI and `@ConditionalOnMissingBean` seams. |
| Configuration and manifest contracts | Add documented `loomspan.connections.*.provider-retry.*`, `loomspan.connections.*.openai.compatibility-profile`, and `loomspan.session.quotas.max-provider-attempts`. No YAML skill-manifest change. | Add strict defaults and bounds atomically across properties, metadata, tests, README, and authoring guidance. Existing connections receive enabled retry defaults with three total attempts; retry remains application-owned. |
| Persisted or serialized contracts | No durable cross-version format changes. Terminal/live REST usage DTOs change within the same application/Console release, but they are internal version-coupled observability transport. | Update in-repository producer and consumers atomically; make no durable compatibility promise. |
| Ephemeral diagnostic formats | Add `MODEL_ATTEMPT_FAILED`, attempt reason/provider number, provider-attempt usage/limit fields, bounded observed failure facts, payload descriptors, and terminal attempt linkage. Java writes NDJSON; Go, fixtures, browser APIs, and UI consume it. | Current-version coherent break. Regenerate fixtures, reject the old incomplete configured-limit shape where required, preserve bounds/security, and add no legacy reader. Keep `consoleCompatibilityVersion` derived from the release version; do not introduce a separate trace schema marker. |
| Internal or accidentally exposed implementation | Replace `ModelAttemptCallAdvisor`; reorganize connection factories behind a version-scoped Spring AI integration; add internal failure classification, delay/jitter, normalized exceptions, and throwable-attempt association. Existing factory types are technically public only for cross-package framework wiring. | Update/remove obsolete paths atomically. New cross-package types may be technically public only if Java package collaboration requires it and must be added to the architecture allowlist with an internal-only rationale. Prefer package-private types where possible. |

- **Evidence of supported contracts**: the approved PR 23 ticket; README and
  Spring configuration metadata for `loomspan.*`; authoring guidance for named
  connections and traces; the API and technically-public-internal architecture
  allowlists; and executable Java/Go/browser fixtures and tests.
- **Intended breaks**: the current-run trace attempt shape and configured-limit
  object change atomically. Old current-run artifacts are not supported by the
  new Console analyzer. There is no Application API, Supported SPI, or YAML
  skill-manifest break.
- **In-repository consumers to update**: auto-configuration, properties and
  metadata, connection factories, chat-client wiring, trace recorder/state,
  session usage and limits, live observation, REST DTO mapping, Java fixtures,
  Go analyzer/index/query/browser adapters, browser TypeScript/UI, tests,
  README, and two focused authoring documents.
- **Public-surface delta**: no Application API or Supported SPI types or methods.
  Any technically public internal adapter type needed across packages must be
  explicitly classified in `LoomspanPublicSurfaceArchitectureTest`; no public
  signature may expose Spring Retry or provider SDK types outside the
  version-scoped integration package.
- **Shim decision**: **No shim.** The affected trace is an ephemeral
  current-release diagnostic contract, all consumers live in this repository,
  and the pre-1.0 policy directs an atomic writer/reader/fixture update.
- **Java-to-Go boundary coordination**: **Required.** Java's NDJSON record types,
  attempt metadata, failure linkage, session usage, and configured limits must
  ship with the Go enum/lifecycle/index/query changes, regenerated corpus,
  browser DTO mapping, TypeScript contracts, and UI tests. The release-derived
  `consoleCompatibilityVersion` remains the compatibility marker; no separate
  trace schema version is added.

## Implementation Approach

Keep dependency-specific construction and translation in a package scoped to
the pinned Spring AI 1.1 release line. Let that integration construct models
with a shared one-attempt `RetryTemplate`, report exact versus opaque ownership,
and translate supported HTTP/SDK failures to Loomspan-owned facts. The retry
advisor consumes only Loomspan types.

The provider advisor loops around only `chain.nextCall(request)`, with the same
request object on each provider retry. Before each send it atomically reserves a
provider attempt; only then does it allocate/record the attempt and invoke the
model. It catches only the raw invocation exception, classifies and records the
failed attempt, decides whether to retry, waits through injected interruptible
delay and jitter collaborators, and either retries or propagates the same final
exception.

Attempt identity remains one retry sequence with a single monotonic physical
`attemptNumber`. The advisor derives `attemptReason` from prior attempts in the
sequence and uses a call-local `providerAttemptNumber` that resets for each
semantic invocation. No extra request-cycle identifier is introduced.

For terminal provider failure, register the last attempt identity against the
propagated throwable in `LoomspanSession`. Registration and lookup walk a
bounded, cycle-safe identity cause chain consistent with PR 22 deduplication.
When a later execution boundary calls `recordFailure`, it merges only the
registered `attemptId` and `retrySequenceId` into the canonical error metadata.
Recovered failures never register terminal context and never emit
`ERROR_RECORDED`.

## Phase 1: Configuration and Version-Scoped Integration Boundary

### Overview

Establish strict application configuration and make Loomspan the exact owner of
application-level call attempts before adding the retry loop.

### Changes Required

#### 1. Provider retry and compatibility properties

**Files**:

- `loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/autoconfigure/LoomspanProperties.java`
- `loomspan-spring-boot-starter/src/main/resources/META-INF/additional-spring-configuration-metadata.json`
- `loomspan-spring-boot-starter/src/test/java/com/lokiscale/loomspan/autoconfigure/LoomspanPropertiesTest.java`
- `loomspan-spring-boot-starter/src/test/java/com/lokiscale/loomspan/autoconfigure/ConfigurationMetadataTest.java`
- `loomspan-spring-boot-starter/src/test/java/com/lokiscale/loomspan/internal/core/LoomspanSessionPropertiesTest.java`

**Changes**:

- Add one `ProviderRetryProperties` value to every connection with defaults:
  enabled `true`, max attempts `3`, initial backoff `500ms`, multiplier `2.0`,
  max backoff `5s`, and jitter `0.20`.
- Validate max attempts 1–10; nonnegative durations; max backoff not below
  initial; finite multiplier at least 1; and finite jitter in 0–1.
- Treat disabled retry as one total attempt regardless of retained tuning
  values.
- Add OpenAI compatibility enum values `STANDARD` and `OPENROUTER`, defaulting
  to `STANDARD`; bind it only under the OpenAI option block.
- Add `maxProviderAttempts` default 192 to session quotas. Preserve the existing
  quota convention that zero disables enforcement and negatives fail startup.
- Add metadata entries and value hints for all new fields.
- Assert exact property paths in all validation errors and ensure unknown
  `spring.ai.*` settings do not affect Loomspan behavior.

#### 2. Spring AI 1.1 integration package

**Files**:

- New package under
  `loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/springai/v1_1/`
- Existing files under
  `loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/autoconfigure/*ConnectionChatModelFactory.java`
- `loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/autoconfigure/NamedAiConnectionRegistry.java`
- `loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/autoconfigure/LoomspanAutoConfiguration.java`

**Changes**:

- Introduce Loomspan-owned `AttemptOwnership` with
  `EXACT_ATTEMPT_OWNERSHIP` and `OPAQUE_CLIENT_RETRIES`.
- Introduce a compact internal integration result containing the `ChatModel`,
  ownership, immutable provider retry policy, and dependency-neutral failure
  translation capability required by the advisor.
- Construct one one-attempt Spring `RetryTemplate` and pass it to the OpenAI,
  Anthropic, Google GenAI, and Ollama chat-model builders.
- Route all four connection constructions through the version-scoped
  integration. Keep Spring Retry, provider builders, HTTP integration seams,
  and provider exception imports inside that package except for unavoidable
  `ChatModel` collaboration.
- Store connection runtime descriptors rather than bare models in the internal
  registry/resolver path so the selected model call can reach ownership,
  retry-policy, and classification data without a public SPI.
- Validate startup: retry enabled plus opaque ownership fails with the exact
  connection property path and an actionable explanation; retry disabled
  permits one Loomspan-observed client call.
- Do not declare adapter components as replaceable beans.

#### 3. Exact-ownership integration tests

**Files**:

- Extend `ConnectionProtocolTest` or split focused tests under
  `src/test/java/com/lokiscale/loomspan/internal/springai/v1_1/`
- Update `NamedAiConnectionRegistryTests`
- Update `SpringAiSkillChatClientFactoryTests`

**Changes**:

- Verify each builder receives the one-attempt template by observing one mock
  endpoint/SDK call for a retryable failure.
- Verify three Loomspan attempts later produce three calls rather than thirty.
- Verify opaque ownership startup behavior for enabled and disabled retry.
- Verify external `spring.ai.retry.*` properties cannot alter behavior.

### Success Criteria

#### Automated Verification

- [x] Strict property default/boundary/path tests pass:
  `./mvnw.cmd -pl loomspan-spring-boot-starter -Dtest=LoomspanPropertiesTest,LoomspanSessionPropertiesTest,ConfigurationMetadataTest test`.
- [ ] All four exact-ownership tests observe one downstream call per direct
  adapter invocation.
- [x] Architecture tests show no new Supported SPI or replaceable bean:
  `./mvnw.cmd -pl loomspan-spring-boot-starter -Dtest=LoomspanPublicSurfaceArchitectureTest,LoomspanAutoConfigurationBoundaryTest test`.
- [x] No production import of Spring Retry/provider builder/provider exception
  types exists outside the approved integration package, apart from documented
  unavoidable chat-client model types.

#### Manual Verification

- [ ] Review startup errors for invalid retry ranges and opaque ownership; each
  names the exact connection property path without credentials or base URLs.
- [ ] Inspect model-construction logs and exceptions for secret leakage.

---

## Phase 2: Normalized Failures and OpenRouter Decoding

### Overview

Preserve observable provider facts before Spring AI loses them and translate
only supported failure evidence into Loomspan-owned values.

### Changes Required

#### 1. Dependency-neutral failure model and classifier inputs

**Files**:

- New internal types in the Spring AI integration package and a Loomspan-owned
  retry-policy package used by the provider advisor.

**Changes**:

- Define `ProviderFailureDetails` with classification/category inputs, optional
  HTTP status, valid retry-after delay, provider type/code, bounded summary,
  diagnostics, and explicit transport evidence.
- Define closed enums for the ticket's classifications, categories, decisions,
  and delay sources.
- Translate only supported concrete Spring/SDK/HTTP exceptions. Walk a short
  explicit cause allowlist with identity cycle detection and a hard depth cap.
- Make cancellation/interruption, TLS trust/hostname failures, generic decoding,
  validation/quota/tool/tracing exceptions, and unknown failures non-retryable
  unless an independently observed retryable status exists.
- Never classify from exception class-name strings or message text.

#### 2. Bounded non-2xx capture and `Retry-After`

**Files**:

- Version-scoped HTTP response handler/interceptor classes.
- Reuse or factor PR 22's bounded UTF-8 diagnostic capture primitives without
  creating another public storage service.

**Changes**:

- Preserve status, valid `Retry-After`, bounded content type/body, truncation,
  and capture limit in an internal normalized exception.
- Parse delta-seconds and HTTP-date using an injected clock. Ignore malformed,
  negative, overflowed, and past values.
- Do not copy request headers, credentials, base URLs, or connection secrets.
- Keep diagnostic text opaque to Java policy, Go analysis, and TypeScript.

#### 3. Explicit OpenRouter synchronous decoder

**Files**:

- Version-scoped OpenAI integration classes.
- Focused mock-server tests beside `ConnectionProtocolTest`.

**Changes**:

- Install the decoder only for `compatibility-profile: openrouter` through
  `OpenAiApi.Builder.restClientBuilder(...)`.
- Buffer at most the PR 22 diagnostic capture limit and provide an ordinary
  successful response body exactly once to Spring AI's downstream decoder.
- Recognize a choice whose `finish_reason` is `error`; capture the documented
  choice-level error object and normalize `code`, `message`,
  `metadata.error_type`, and `provider_code`.
- Reject the response as a normalized provider failure even when partial
  assistant content exists. Do not allow that content into response validation,
  tools, session success state, or the caller.
- Classify malformed or oversized error envelopes as `CLIENT_DECODING` and do
  not retry unless independent retryable HTTP/transport evidence exists.
- Confirm the standard profile is byte/behavior neutral for ordinary responses
  and never activates from an OpenRouter-looking name, URL, host, or model.

### Success Criteria

#### Automated Verification

- [ ] Classifier tests cover every required retryable/non-retryable status,
  transport, TLS, cancellation, generic decoding, unknown, and bounded/cyclic
  cause case.
- [ ] `Retry-After` delta/date/malformed/past/overflow tests are deterministic.
- [x] OpenRouter success passes through once; documented error completion
  becomes a normalized failure instead of `InvalidFormatException`.
- [x] Partial error content never reaches a returned `ChatResponse`.
- [ ] Diagnostic body tests prove UTF-8 validity, byte bound, truncation state,
  and absence of request secrets.
- [x] Focused adapter tests pass:
  `./mvnw.cmd -pl loomspan-spring-boot-starter -Dtest=ConnectionProtocolTest,*ProviderFailure*,*OpenRouter* test`.

#### Manual Verification

- [ ] Inspect representative bounded OpenRouter diagnostics for useful observed
  fields without copied credentials or headers.
- [ ] Compare a standard-profile successful response with current behavior.

---

## Phase 3: Provider Retry Execution and Physical-Attempt Accounting

### Overview

Replace the single-attempt advisor with the bounded retry owner and enforce the
session-wide physical-attempt guardrail before each real send.

### Changes Required

#### 1. Provider attempt advisor

**Files**:

- Replace
  `loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/chat/ModelAttemptCallAdvisor.java`
  with `ProviderAttemptCallAdvisor.java`.
- Update `SpringAiSkillChatClientFactory.java` and tests.
- Update `ModelTraceContext.java` and its focused tests.

**Changes**:

- Retain the current innermost advisor order.
- Resolve the selected connection's exact ownership, retry policy, and failure
  translator from call-local internal context.
- For each physical call: check interruption, reserve quota, allocate identity,
  record prepared/sent, call `chain.nextCall(request)` once, and on success
  trace/account the response once.
- Catch only exceptions from `chain.nextCall(request)`. Do not catch response
  tracing, usage accounting, validators, tools, frame management, or other
  outer work.
- Record the failed attempt before deciding whether to wait/retry or propagate.
- Reuse the exact same `ChatClientRequest` object for provider retries and never
  use partial provider content as input.
- Preserve the final exception object; do not wrap it merely to transport
  attempt metadata.

#### 2. Backoff, jitter, and interruption

**Files**:

- New small internal retry policy, delay, jitter, and clock collaborators.
- Provider advisor unit/integration tests.

**Changes**:

- Use overflow-safe capped exponential delay; apply uniformly bounded plus/minus
  jitter after capping.
- Default deterministic base delays are 500ms then 1s.
- Use the larger of calculated backoff and valid `Retry-After`; record the
  source that controls.
- Wait interruptibly. Check interruption before every send and after every
  failed wait; preserve the interrupt flag and stop immediately.
- Continue running inside the existing mission future so timeout cancellation
  interrupts backoff without resetting the mission timeout.

#### 3. Physical-attempt usage, quota, and metrics

**Files**:

- `SessionUsageService.java`, `DefaultSessionUsageService.java`, and no-op test
  implementations.
- `SessionUsageSnapshot.java`.
- `GuardrailType.java` and quota exception tests.
- `UsageMetricsRecorder.java`, `MicrometerUsageMetricsRecorder.java`, and
  `NoOpUsageMetricsRecorder.java`.
- Live and REST usage DTO/mapping classes.

**Changes**:

- Add `reserveProviderAttempt(...)` that atomically checks and increments before
  any `MODEL_REQUEST_SENT` or downstream call. A rejected reservation changes
  neither counter nor trace send lifecycle.
- Preserve zero-as-disabled quota behavior.
- Add `providerAttempts` to session snapshots and serialization while retaining
  `modelCalls` as successful response accounting.
- Add `MAX_PROVIDER_ATTEMPTS` guardrail.
- Record bounded provider-attempt metrics with driver, framework model, outcome,
  failure category, and retry decision. Do not tag attempt IDs, exception text,
  provider bodies, provider raw codes, model content, or credentials.

### Success Criteria

#### Automated Verification

- [x] Failure then success calls the model twice with the same request object and
  records one response/accounting event.
- [x] Disabled and max-attempts-one configurations call once.
- [ ] Delay, cap, jitter, and `Retry-After` behavior is deterministic through
  injected collaborators.
- [ ] Interruption before send and during wait preserves interrupt state and
  emits no phantom attempt.
- [ ] Existing mission-timeout tests prove retry waiting is interrupted and the
  timeout is not reset.
- [x] Concurrent quota tests prove the blocked send neither increments
  `providerAttempts` nor invokes the endpoint.
- [x] Existing `modelCalls` tests retain their meaning and explicitly coexist
  with `providerAttempts`.
- [x] Metric tests prove the required bounded tags and absence of sensitive or
  unbounded values.

#### Manual Verification

- [ ] Review a debug trace to confirm endpoint calls, sent records, and
  `providerAttempts` have identical counts.
- [ ] Confirm a short mission timeout cancels a visible retry wait promptly.

---

## Phase 4: Failed-Attempt Trace Lifecycle and Terminal Linkage

### Overview

Make every known provider failure explicit and connect the eventual terminal
PR 22 failure to the final attempt without producing errors for recovered
attempts.

### Changes Required

#### 1. Attempt metadata and failure records

**Files**:

- `ModelTraceContext.java`.
- `TraceRecordType.java`.
- `ExecutionTraceRecorder.java` and `DefaultExecutionTraceRecorder.java`.
- `ExecutionStateService.java` and `DefaultExecutionStateService.java`.
- `ProviderAttemptCallAdvisor.java`.
- Trace contract and advisor integration tests.

**Changes**:

- Add `MODEL_ATTEMPT_FAILED`.
- Add `attemptReason` (`INITIAL`, `SEMANTIC_RETRY`, `PROVIDER_RETRY`) and positive
  `providerAttemptNumber` to all attempt lifecycle facts.
- Determine `INITIAL` only for the first physical attempt in the retry sequence;
  later outer invocations start at provider attempt 1 with `SEMANTIC_RETRY`;
  unchanged-request resends use `PROVIDER_RETRY`.
- Record classification, category, decision, delay/source, optional observed
  provider fields, and PR 22-shaped bounded diagnostics.
- Store diagnostic text through the existing logical payload machinery and
  expose a payload descriptor on the failed-attempt summary; add no specialized
  range endpoint.
- Ensure an attempt cannot receive both response and failure terminal facts.

#### 2. Throwable-to-attempt association

**Files**:

- `LoomspanSession.java` and focused `LoomspanSessionTest` cases.
- `ExecutionStateService.java` and `DefaultExecutionStateService.java` if a
  cross-package internal method is required.
- Mission and step execution terminal failure tests.

**Changes**:

- Add a session-local identity map for pending provider failure context with only
  `attemptId` and `retrySequenceId`.
- Register the final propagated provider exception after its failed-attempt fact
  succeeds. Use the same bounded, cycle-safe cause identity rules as failure
  deduplication.
- When `recordFailure` creates the canonical `ERROR_RECORDED`, merge the closest
  registered context into metadata and retain PR 22's closest-frame behavior.
- When an existing failure identity is reused, preserve its original attempt
  context and failure ID.
- Avoid message/class matching and do not wrap the exception.
- Do not register recovered failures. Clear association state with the session
  lifecycle.

#### 3. Configured limits and live projection

**Files**:

- `ConfiguredLimitsSnapshot.java`, `DefaultExecutionTraceHandle.java`, runner
  and fixture tests.
- `LiveActivityProjector.java`, `ExecutionActivityKind.java`, usage mapping, and
  projector tests.
- Observability REST DTOs and mapper tests.

**Changes**:

- Add `maxProviderAttempts` to the immutable run-start configured-limit object.
- Add `providerAttempts` to live and terminal usage projections.
- Project `MODEL_ATTEMPT_FAILED` as bounded live activity. For `RETRY`, summarize
  only the attempt number and delay; never include exception messages or body
  text.
- Keep interruption/cancellation during backoff represented by PR 22 terminal
  evidence rather than a third attempt terminal state.

### Success Criteria

#### Automated Verification

- [x] Failure then success emits two complete attempts and no `ERROR_RECORDED`.
- [x] Permanent/exhausted failure emits one `ERROR_RECORDED` linked to the last
  failed attempt and closest model frame.
- [ ] Semantic retry resets provider attempt number and preserves retry sequence
  monotonicity.
- [ ] Cancellation during a recorded `RETRY` delay emits no next attempt and is
  explained by terminal abort/interruption evidence.
- [x] Trace/usage/configured-limit serialization tests cover the new exact
  fields.
- [x] Live activity contains no raw diagnostic content.

#### Manual Verification

- [ ] Inspect recovered and terminal traces to confirm one alternate terminal
  fact per attempt.
- [ ] Confirm the terminal failure and last attempt share explicit identity and
  that earlier failed attempts are not marked terminal errors.

---

## Phase 5: Go Analysis and Executable Fixture Contract

### Overview

Teach the neutral analyzer the new attempt state machine and update the
version-coupled corpus atomically.

### Changes Required

#### 1. Attempt parsing and validation

**Files**:

- `loomspan-console/internal/traceanalysis/enums.go`
- `loomspan-console/internal/traceanalysis/attempts.go`
- `loomspan-console/internal/traceanalysis/processor.go`
- `loomspan-console/internal/traceanalysis/model.go`
- `loomspan-console/internal/traceanalysis/dto.go`
- `loomspan-console/internal/traceanalysis/query_facts.go`
- Focused calculations, processor, query, and service tests.

**Changes**:

- Accept `MODEL_ATTEMPT_FAILED` as the alternate terminal fact.
- Parse and strictly validate attempt reason, provider attempt number, failure
  enums, delay invariants, optional observed fields, and payload descriptor.
- Require exactly one response-or-failure terminal per complete attempt; only a
  sent attempt with neither remains `MODEL_ATTEMPT_RESPONSE_MISSING`.
- Validate unique attempt IDs, consistent retry identity, globally positive
  monotonic attempt number within a retry sequence, provider numbering resets,
  provider retry retention/increment, and impossible transitions after
  permanent/exhausted failure.
- Permit `RETRY` without a subsequent attempt only when final terminal
  abort/interruption evidence explains cancellation during backoff.
- Validate final `ERROR_RECORDED` linkage to the last failed attempt.
- Extend attempt summaries with reason, provider number, outcome, failure facts,
  retry decision/delay/source, observed provider fields, and payload descriptor.
- Failed attempts contribute zero known usage and make aggregate retry usage
  incomplete; do not synthesize tokens.

#### 2. Usage/configured-limit parsing and browser adapter DTOs

**Files**:

- Go configured-limit and terminal-usage models/decoders.
- `loomspan-console/internal/browserapi/trace_analysis.go` and tests.
- Active execution/live DTOs and tests.

**Changes**:

- Require `maxProviderAttempts` when `configuredLimits` is present in the new
  current-release shape.
- Carry `providerAttempts` in usage DTOs without redefining `modelCalls`.
- Map the enriched attempt summary directly; do not reclassify provider errors
  in the browser adapter.

#### 3. Regenerated fixture corpus

**Files**:

- `loomspan-console-fixtures/traces/`
- `loomspan-console-fixtures/expected/`
- `loomspan-console-fixtures/README.md`
- Java `ConsoleTraceFixtureCorpusTest`.
- Go `fixture_corpus_test.go` and invalidity tests.

**Changes**:

- Add valid failure-then-success, permanent failure, exhausted failure,
  semantic-reset, and cancellation-during-backoff fixtures.
- Mutate corpus cases for duplicate terminals, invalid reason/provider number,
  nonmonotonic attempts, mismatched retry sequence, retry after permanent or
  exhausted failure, unexplained missing retry, bad final linkage, and malformed
  failure payload/diagnostic descriptors.
- Regenerate Java-owned expected results and inspect them before accepting.
- Update corpus counts and semantic coverage in the fixture README.

### Success Criteria

#### Automated Verification

- [x] Java fixture regeneration succeeds:
  `./mvnw.cmd -pl loomspan-spring-boot-starter test -Dtest=ConsoleTraceFixtureCorpusTest -Dloomspan.console.fixtures.regenerate=true -DfailIfNoTests=false`.
- [x] Regenerated fixture diff contains only deliberate current-contract
  changes and new cases.
- [x] Go analyzer tests pass: `go test ./...` from `loomspan-console`.
- [ ] Every required invalid transition maps to the intended stable invalidity
  category.
- [x] Failed attempts no longer create response-missing gaps.

#### Manual Verification

- [ ] Inspect at least the recovery, exhaustion, and cancellation NDJSON plus
  expected JSON side by side.
- [ ] Confirm diagnostics remain opaque bounded text and are not copied into
  indexes beyond their descriptor/payload reference.

---

## Phase 6: Browser and Live Activity Presentation

### Overview

Expose the neutral attempt facts in the existing Trace Explorer and live
narrative without creating a provider-specific subsystem.

### Changes Required

#### 1. Browser API and TypeScript contracts

**Files**:

- `loomspan-console/web/src/api/contracts.ts`
- `loomspan-console/web/src/api/client.ts`
- Browser API contract fixtures/tests.

**Changes**:

- Extend usage/configured-limit types with provider attempt fields.
- Extend `TraceAttempt` with reason, provider number, outcome, failure facts,
  retry decision/delay/source, observed provider fields, and optional payload
  descriptor.
- Add `MODEL_ATTEMPT_FAILED` to the live activity kind/label union.
- Keep all fields adapter-neutral and do not add OpenRouter parsing to
  TypeScript.

#### 2. Trace Explorer attempt presentation and navigation

**Files**:

- `TraceExplorer.tsx`
- `TraceRecords.tsx`
- `TraceFailureFocus.tsx`
- Existing evidence-detail/range components.
- Component and end-to-end tests.

**Changes**:

- Render attempts chronologically and label initial, semantic retry, and
  provider retry.
- Show `SUCCEEDED`, `FAILED`, or `INCOMPLETE`; failure category/classification;
  retry decision and delay/source; and small observed provider fields.
- Open a failed attempt's existing logical payload through `getPayloadRange`.
  Do not expose raw NDJSON as the normal diagnostic path.
- Add navigation from terminal failure to last attempt and from that attempt to
  the linked failure using existing explorer selection/filter mechanisms.
- Preserve bounded pagination and avoid client-side provider classification.

#### 3. Live activity presentation

**Files**:

- Go live DTO validation/mapping.
- `activityPresentation.ts` and narrative components/tests.

**Changes**:

- Present bounded text such as `Provider attempt 1 failed; retrying in 500 ms`.
- Treat activity details as already bounded facts. Never display provider body
  text or exception messages in the live narrative.

### Success Criteria

#### Automated Verification

- [x] TypeScript typecheck and component tests pass through
  `go run ./internal/buildtool verify`.
- [ ] Accessibility tests cover chronological labels, outcome text, payload
  actions, and bidirectional navigation.
- [x] UI tests prove failed attempts use the payload-range viewer and never a
  provider-specific decoder.
- [x] Live activity tests prove raw provider/exception content is absent.

#### Manual Verification

- [ ] In Trace Explorer, follow failure → last attempt → bounded diagnostic and
  back without using raw download.
- [ ] Verify long values and truncated diagnostics remain readable and do not
  break the records view.
- [ ] Verify the live retry narrative is understandable with assistive
  technology announcements.

---

## Phase 7: Documentation, Compatibility Checks, and End-to-End Verification

### Overview

Finish the application-owned configuration contract, skill-developer diagnostic
guidance, dependency support statement, and complete verification sequence.

### Changes Required

#### 1. Application and authoring documentation

**Files**:

- `README.md`
- `ai/skill-authoring/model-selection-and-connections.md`
- `ai/skill-authoring/traces-and-debugging.md`
- `ai/skill-authoring/README.md` only if editing reveals a routing/coverage
  boundary change; none is currently planned.

**Changes**:

- Document exact provider retry defaults, validation bounds, disabled behavior,
  session-wide physical-attempt quota, and explicit OpenRouter profile.
- State that YAML skills cannot manage or override provider retry or
  `max-provider-attempts`; they continue to choose only a model alias.
- Document exact attempt/reason/outcome semantics, quota interpretation,
  recovered versus terminal failure, and missing-response gaps.
- State the exact primary tested Spring AI version and the policy for tested
  binary-compatible patch overrides. Do not claim arbitrary-version support.
- Keep the authoring docs focused on decisions and diagnosis, using named
  implementation/test anchors rather than volatile line citations.

#### 2. Architecture and dependency isolation verification

**Files**:

- Architecture tests.
- CI workflow only if the release documentation claims another Spring AI patch
  version.

**Changes**:

- Assert Spring AI provider builders, Spring Retry types, and provider
  exceptions remain inside the version-scoped integration boundary.
- Assert no reflection or runtime version-string branching.
- Assert no new supported API/SPI or replaceable bean.
- If no additional Spring AI version is actually exercised in CI, document only
  1.1.6 as tested.

#### 3. Full verification and mock OpenRouter scenario

**Changes**:

- Run the full starter test suite.
- Regenerate and inspect fixtures once, then rerun without regeneration to prove
  a clean deterministic corpus.
- Run Go tests, canonical Console verification, and race tests with the required
  Windows CGO toolchain.
- Run a local mock OpenRouter endpoint that returns a documented retryable error
  completion and then success. Acquire the finalized trace and verify endpoint,
  usage, attempt, failure, live activity, payload, and terminal outcome facts.
- Keep live-provider tests optional and never log external credentials.

### Success Criteria

#### Automated Verification

- [x] Starter suite passes:
  `./mvnw.cmd -pl loomspan-spring-boot-starter test -DfailIfNoTests=false`.
- [x] Fixture regeneration and clean rerun both pass.
- [x] Go suite passes: `go test ./...`.
- [x] Canonical Console verification passes:
  `go run ./internal/buildtool verify`.
- [x] Race suite passes after setting MinGW and CGO:
  `$env:PATH = "C:\msys64\mingw64\bin;" + $env:PATH; $env:CGO_ENABLED = "1"; go test -race ./...`.
- [x] Architecture tests report no unsupported public-surface or Spring
  extension-point delta.
- [x] Documentation statements are backed by the cited focused tests and
  satisfy the authoring README's LLM-first standard.
- [x] `git status --short` contains only intentional PR 23 changes.

#### Manual Verification

- [ ] Mock OpenRouter error-then-success produces exactly two endpoint calls,
  two attempt records, `providerAttempts == 2`, one successful `modelCalls`
  increment, no `ERROR_RECORDED`, and a successful terminal trace.
- [ ] Mock exhausted failure produces the configured number of endpoint calls,
  the same number of attempts/provider-attempt counts, and one terminal error
  linked to the last attempt.
- [ ] Trace Explorer displays the failed attempt and bounded provider payload,
  and terminal failure navigation works in both directions.
- [ ] No credential, request header, base URL, raw body, exception message, or
  model content appears in metrics or live summaries.

## Testing Strategy

A dedicated testing plan should be created with `ai/commands/3_testing_plan.md`
before implementation. It should turn the following risks into named tests and
explicit exit criteria.

### Unit Tests

- Configuration defaults, all numeric/duration bounds, exact property paths,
  and disabled semantics.
- Overflow-safe exponential delay, jitter bounds, injected randomness, and
  `Retry-After` parsing.
- Allowlisted status/transport/OpenRouter classification and explicit negative
  cases including TLS, cancellation, decoding, and unknown exceptions.
- Bounded cycle-safe cause traversal.
- Attempt identity/reason/provider numbering and immutable request reuse.
- Atomic provider-attempt reservation and existing model-response accounting.
- Throwable-to-attempt registration/deduplication.
- Go lifecycle, transition, final linkage, usage completeness, and malformed
  diagnostic validation.
- Browser presentation and bounded navigation behavior.

### Integration Tests

- Each Spring AI builder makes one underlying call per direct invocation.
- OpenRouter ordinary response and error completion through a mock server.
- Non-2xx response body/status/`Retry-After` capture.
- Provider failure → recovery, permanent failure, exhaustion, semantic retry,
  interruption, timeout, and quota rejection.
- Java fixture producer → Go analyzer → browser DTO/UI contract.
- Full architecture/import boundary.

### Manual Testing Steps

1. Start a local mock OpenRouter-compatible server whose first synchronous chat
   response is a documented retryable error completion and whose second is a
   valid success.
2. Configure one explicit OpenAI connection with
   `compatibility-profile: openrouter`, default provider retry, and a model alias;
   invoke a YAML skill that selects only that alias.
3. Confirm two requests carry equivalent serialized prompts/options and no
   partial error content reaches the skill result.
4. Acquire the trace in Console and verify chronological attempts, failed
   payload, retry delay, success, counters, and absence of a recovered terminal
   error.
5. Repeat with permanent and exhausted responses; verify one terminal failure
   linked to the last attempt.
6. Repeat with a short mission timeout during backoff and verify prompt
   interruption, no phantom send, and explanatory terminal evidence.

## Performance Considerations

- Retry state is bounded by at most ten attempts per unchanged request and the
  session-wide physical-attempt quota.
- Backoff arithmetic must saturate/cap without overflow and must not retain
  scheduled tasks after interruption.
- Cause traversal is depth-bounded and identity cycle-safe.
- Response/diagnostic buffering is capped at PR 22's 1 MiB limit and should
  avoid additional unbounded copies. Ordinary successful OpenRouter responses
  are buffered only when the explicit profile is enabled.
- Metrics use a closed bounded tag vocabulary; raw provider codes, IDs, bodies,
  and messages are excluded.
- Go continues streaming the NDJSON and stores only compact attempt facts and
  payload descriptors in indexes.

## Migration Notes

- Existing application connections receive enabled provider retry defaults of
  three total attempts after upgrade. Applications that require one call set
  `loomspan.connections.<name>.provider-retry.enabled=false` or
  `max-attempts=1`.
- Existing YAML skills require no changes and have no retry override.
- Existing `modelCalls` configuration and semantics remain unchanged. Operators
  may additionally configure `max-provider-attempts`; zero follows the existing
  quota convention and disables that guardrail.
- The OpenRouter decoder is opt-in. Existing OpenAI-compatible connections
  remain `standard` until explicitly configured.
- Current-run trace fixtures and analyzer contracts change atomically. Old
  artifacts need not be readable by the new Console; no migration or dual
  reader is provided.
- The supported primary dependency remains Spring AI 1.1.6 unless CI is
  deliberately expanded to test another binary-compatible patch.

## References

- Original ticket:
  `ai/thoughts/tickets/loomspan-console-pr-23-provider-retries.md`
- Dependency ticket:
  `ai/thoughts/tickets/loomspan-console-pr-22-failed-trace-diagnostics.md`
- Framework design policy:
  `ai/thoughts/framework-feature-design-lens.md`
- Authoring configuration guidance:
  `ai/skill-authoring/model-selection-and-connections.md`
- Authoring diagnostic guidance:
  `ai/skill-authoring/traces-and-debugging.md`
- Existing physical-attempt boundary:
  `loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/chat/ModelAttemptCallAdvisor.java:43`
- Existing failure identity recorder:
  `loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/core/LoomspanSession.java:263`
- Existing Go attempt lifecycle:
  `loomspan-console/internal/traceanalysis/attempts.go:53`
- Existing Go missing-response projection:
  `loomspan-console/internal/traceanalysis/processor.go:261`
- Existing browser attempt view:
  `loomspan-console/web/src/observability/TraceRecords.tsx:7`
- Pinned dependency version: `pom.xml:50`
