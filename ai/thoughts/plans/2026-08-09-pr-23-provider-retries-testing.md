# PR 23 Provider Retries Testing Plan

## Change Summary

- Add application-owned, bounded provider retry configuration to each named AI
  connection and a session-wide `max-provider-attempts` quota. YAML skills do
  not configure or override either value.
- Disable Spring AI's dependency-owned application-level retries for the pinned
  1.1.6 OpenAI, Anthropic, Google GenAI, and Ollama call paths so one Loomspan
  attempt equals one downstream provider call.
- Add an explicit OpenRouter compatibility profile that intercepts synchronous
  `finish_reason: "error"` completions before Spring AI's enum decoder, rejects
  partial content, and preserves bounded observed provider diagnostics.
- Replace the one-shot model-attempt advisor with an unchanged-request retry
  loop using a small allowlisted classifier, capped exponential backoff, jitter,
  valid `Retry-After`, interruption, mission timeout, and physical-attempt quota.
- Add `providerAttempts`, `MODEL_ATTEMPT_FAILED`, attempt reason/provider
  numbering, terminal failure-to-attempt linkage, bounded metrics, live
  activity, Go analysis, browser DTOs, and Trace Explorer presentation.
- Change the current-release ephemeral trace contract atomically across Java,
  Go, TypeScript, and fixtures without a legacy reader or new schema marker.

## Impacted Areas

- **Configuration contract**: `LoomspanProperties`, configuration metadata,
  named connection validation, session quotas, README, and focused authoring
  guidance.
- **Pinned dependency integration**: Spring AI 1.1.6 model builders, one-attempt
  `RetryTemplate`, HTTP response interception, response error handling, concrete
  exception translation, and exact attempt ownership.
- **Retry runtime**: provider advisor ordering, identical request reuse,
  classifier, delay/jitter/clock collaborators, cancellation, and propagation.
- **Accounting and metrics**: `SessionUsageService`, `SessionUsageSnapshot`,
  configured limits, quota exceptions, Micrometer tags, live execution usage,
  and REST DTOs.
- **Java trace production**: `ModelTraceContext`, record types, failed-attempt
  payloads, throwable identity association, PR 22 failure recording, and live
  activity projection.
- **Cross-language diagnostic boundary**: Java fixture generator, committed
  NDJSON/expected corpus, Go enums/parser/attempt graph/index/query models,
  browser adapter DTOs, and exact release compatibility.
- **Browser experience**: TypeScript contracts, attempt chronology/outcomes,
  existing payload-range viewer, terminal failure navigation, and bounded live
  narrative.
- **Protected verification boundaries**: Application API exception propagation,
  no new Supported SPI or replaceable bean, no leaked Spring/provider types,
  exact `consoleCompatibilityVersion` rejection, and current-run diagnostic
  coherence.

## Risk Assessment

### Highest-risk behaviors

- **Retry multiplication**: leaving Spring AI's retry template active could turn
  three Loomspan attempts into as many as thirty dependency attempts.
- **False retry classification**: broad exception handling could retry invalid
  requests, authentication/payment/policy failures, TLS trust failures,
  cancellations, deserialization errors, or Loomspan-owned exceptions.
- **Partial-content escape**: an OpenRouter error completion can include partial
  assistant content; returning it to validators, tools, success state, or the
  caller would be a correctness and side-effect risk.
- **Cardinality/accounting mismatch**: endpoint calls, sent attempt facts, and
  `providerAttempts` must stay equal under success, failure, quota contention,
  timeout, and interruption while `modelCalls` retains response-only semantics.
- **Retrying the wrong boundary**: a loop outside the raw provider call could
  repeat semantic advisors or side-effecting tools; an overly broad catch could
  retry trace, usage, quota, or validation failures.
- **Interruption races**: timeout or thread interruption during delay must stop
  promptly, preserve the interrupt flag, and produce neither a phantom attempt
  nor an unexplained successful trace.
- **Diagnostic loss or leakage**: useful status/type/code/body evidence must
  survive within existing bounds without copying credentials, request headers,
  base URLs, provider bodies into metrics/live summaries, or partial content
  into results.
- **Failure-link corruption**: PR 22's throwable/cause deduplication must still
  emit one canonical error, now linked to the final failed attempt, without
  linking recovered failures or matching by class/message.
- **Trace reader/writer drift**: a Java writer accepted by one layer but rejected
  or misinterpreted by Go/browser would make the Console unusable for the same
  release.

### Important edge cases

- Retry disabled; `max-attempts=1`; maximum ten attempts.
- Zero initial/max delay; maximum lower than initial; NaN/infinite multiplier
  and jitter; jitter boundaries 0 and 1; duration multiplication overflow.
- HTTP 408, 429, 500, 502, 503, 504 versus every explicitly permanent status.
- Connection/read timeout, reset, premature EOF, temporary name resolution,
  TLS trust/hostname failure, unknown exception, conflicting precise facts, a
  cyclic cause chain, and a cause deeper than the traversal bound.
- `Retry-After` delta-seconds, HTTP-date, smaller than backoff, larger than
  backoff, malformed, negative, past, and overflowing.
- Interruption before reservation, after reservation but before send, during
  delay, and timeout racing with delay completion.
- Response success followed by trace or usage failure: no provider retry.
- Quota rejection under concurrent sends: rejected work neither increments the
  counter nor records `MODEL_REQUEST_SENT`.
- OpenRouter ordinary success, retryable error, permanent error, partial content,
  multiple choices, missing/malformed choice error, oversized envelope, invalid
  UTF-8/body truncation, and a standard profile with OpenRouter-looking names or
  URLs.
- Failure → success, failure → failure → success, permanent first failure,
  exhausted failure, later semantic retry, and cancellation after a recorded
  retry decision but before the next attempt.
- Duplicate attempt terminal facts, missing terminal fact, inconsistent
  identity, nonmonotonic numbering, invalid provider reset, retry following a
  permanent/exhausted decision, and terminal error linked to a non-final attempt.

### Protected compatibility paths

- **Application API**: invoking an LLM-backed YAML skill through `SkillTemplate`
  continues to work and the original terminal provider exception is propagated;
  no retry-only wrapper leaks to callers.
- **Supported SPI**: no provider integration, retry classifier, delay source, or
  connection factory becomes an application replacement point; production code
  remains free of `@ConditionalOnMissingBean`.
- **Configuration and manifest contracts**: existing named connections and
  model aliases still bind; new retry/profile/quota properties have strict
  documented defaults and paths; YAML skill manifests gain no retry field.
- **Persisted or serialized contracts**: no durable contract is added. The
  release-coupled application/Console REST and SSE paths remain protected by
  exact release compatibility.
- **Ephemeral diagnostic formats**: the new current-release writer, Go reader,
  browser projections, payload access, fixtures, and live narrative remain
  coherent, bounded, ordered, and failure-visible.
- **Internal or accidentally exposed implementation**: existing connection
  factories/advisors may be replaced atomically. Tests must not demand the old
  internal class graph or both retry owners simultaneously.

### Intentionally removed obsolete paths

- `ModelAttemptCallAdvisor` and its incomplete thrown-attempt lifecycle.
- Spring AI's default application-level retry behavior beneath Loomspan for the
  supported 1.1.6 model call paths.
- The five-field configured-limit shape and attempt summaries that have no
  reason/outcome/provider-attempt fields.
- Treating every sent attempt without a response as a gap even when a known
  `MODEL_ATTEMPT_FAILED` fact exists.
- Any fixture encoding the old current-run diagnostic shape. No legacy reader,
  overload, alias, fallback, or dual attempt form is retained.

### Authoring claims requiring executable evidence

- Provider retry/profile/quota are application-owned; YAML skills continue to
  choose only a model alias and cannot override retry settings.
- Default provider retry is three total unchanged-request attempts and disabled
  means one attempt.
- OpenRouter behavior activates only through the explicit compatibility profile.
- `providerAttempts` counts actual Loomspan-owned sends; `modelCalls` continues
  to count successful response accounting.
- Traces distinguish initial, semantic-retry, and provider-retry attempts;
  known failures are not response gaps; recovered failures are not terminal
  errors; final failures link to the last attempt.

## Existing Test Coverage

### Java

- `ModelAttemptCallAdvisorIntegrationTest` already protects advisor retry
  cardinality, attempt identity, thrown-provider incomplete behavior, response
  usage, and current model-call quota behavior. It should be replaced/expanded
  around `ProviderAttemptCallAdvisor`, not kept as an old-path compatibility
  test.
- `ConnectionProtocolTest` provides `MockWebServer` patterns for real Spring AI
  OpenAI, Anthropic, and Ollama wire behavior. It is the preferred harness for
  OpenRouter and dependency retry ownership tests.
- `LoomspanPropertiesTest`, `LoomspanSessionPropertiesTest`, and
  `ConfigurationMetadataTest` protect strict binding, property paths, defaults,
  and configuration metadata.
- `SessionUsageServiceTest`, `SessionQuotaTest`, and
  `MicrometerUsageMetricsRecorderTest` cover existing counters, zero-disabled
  quotas, guardrail exceptions, and meter tags.
- `LoomspanSessionTest` covers PR 22 throwable identity/cause reuse, bounded
  cyclic/deep traversal, and concurrent session isolation.
- `ExecutionTraceContractTest`, `ConsoleTraceFixtureCorpusTest`, and fixture
  generation helpers protect trace metadata, diagnostic bounds, corpus
  inventory, and byte-deterministic committed fixtures.
- `LiveActivityProjectorTest` covers the closed visible record set, bounded
  details/summaries, usage derivation, and terminal replacement.
- `LoomspanPublicSurfaceArchitectureTest` and
  `LoomspanAutoConfigurationBoundaryTest` protect the API/internal allowlists
  and absence of replaceable beans.

### Go and browser

- `calculations_test.go` covers strict attempt identity/lifecycle, sent-without-
  response gaps, terminal failure resolution, retry aggregation, and usage
  reconciliation.
- `fixture_corpus_test.go` compares Go semantic results against Java-owned
  expected fixtures. `ConsoleTraceFixtureCorpusTest` owns regeneration.
- Go diagnostic and query tests cover bounded failure diagnostic descriptors,
  payload storage/range access, inert text, and terminal failure indexing.
- Browser API contract tests protect neutral DTO mapping.
- `TraceExplorer.test.tsx` covers pagination, deliberate payload reads, failure
  deep links, frame/failure navigation, and artifact lifecycle.
- `TraceViews.test.tsx` covers record evidence actions, inert ranged text, and
  exact limit arithmetic.
- Activity presentation/narrative tests protect the closed activity kind set
  and safe summaries.
- `TestClientConsumesCommittedInstanceFixtureOnlyAfterExactCompatibility` in
  `internal/applicationclient/client_test.go` proves exact release-string
  rejection. It must remain passing; PR 23 does not add a trace-schema bypass.
- `.github/workflows/console-ci.yml` already runs Java fixtures/adapters,
  canonical Console verification, and Playwright.

### Coverage gaps

- No explicit provider retry/profile properties or physical-attempt quota.
- No proof that all four Spring AI builders make one downstream call per direct
  invocation.
- No OpenRouter error-completion interception or partial-content rejection.
- No Loomspan-owned retry classifier/backoff/interruption tests.
- No failed-attempt terminal record or throwable-to-attempt association.
- No Go lifecycle/transition rules for `MODEL_ATTEMPT_FAILED`.
- No enriched attempt browser presentation or failed-attempt payload action.

## Bug Reproduction / Failing Test First

### First red test: explicit OpenRouter profile is rejected today

- **Name**: `bindsExplicitOpenRouterProfileAndProviderRetryDefaults`
- **Type**: integration (`ApplicationContextRunner` configuration binding).
- **Location**:
  `loomspan-spring-boot-starter/src/test/java/com/lokiscale/loomspan/autoconfigure/LoomspanPropertiesTest.java`
- **Arrange**: configure a valid OpenAI connection with
  `loomspan.connections.openrouter.openai.compatibility-profile=openrouter` and
  no explicit retry tuning.
- **Act**: start the existing strict configuration-binding context.
- **Assert**: context starts; profile is `OPENROUTER`; retry is enabled with
  max attempts 3, 500ms/2.0/5s/0.20 defaults; session
  `maxProviderAttempts` is 192.
- **Expected failure before implementation**: strict binding rejects
  `compatibility-profile` as an unknown `loomspan.*` field. This test compiles
  against the existing test harness and fails reliably without network or
  timing.

### Minimal incident test after the configuration value is scaffolded

- **Name**: `openRouterErrorCompletionRetriesThenReturnsOnlyTheSuccessfulResponse`
- **Type**: integration with real Spring AI OpenAI decoding and `MockWebServer`.
- **Location**: new focused
  `loomspan-spring-boot-starter/src/test/java/com/lokiscale/loomspan/internal/springai/v1_1/OpenRouterCompatibilityIntegrationTest.java`
- **Arrange**: explicit OpenRouter profile; deterministic retry policy with zero
  delay/jitter; first 200 response contains partial assistant content,
  `finish_reason: "error"`, and a documented retryable choice error; second 200
  response is a normal success.
- **Act**: make one synchronous model call through the Loomspan provider attempt
  boundary.
- **Assert**: returned content is only the second response; endpoint request
  count is exactly 2; first attempt is failed/retry and second succeeds; partial
  content is absent from results and downstream validator/tool probes.
- **Expected failure before decoder/retry implementation**: the first response
  throws Spring's `RestClientException` rooted in Jackson
  `InvalidFormatException`; request count remains 1 and no successful response
  is returned.

Do not preserve a characterization test requiring `InvalidFormatException`.
That exception is obsolete internal dependency behavior, not a protected
contract.

## Tests to Add/Update

### 1. `validatesProviderRetryConfigurationBoundariesAndExactPaths`

- **Type**: parameterized unit/integration.
- **Location**: `LoomspanPropertiesTest.java`,
  `LoomspanSessionPropertiesTest.java`, and `ConfigurationMetadataTest.java`.
- **What it proves**: documented defaults; attempts 1–10; nonnegative delays;
  max ≥ initial; finite multiplier ≥ 1; finite jitter 0–1; zero-disabled quota;
  negative quota rejection; driver applicability; exact property path; metadata
  and enum hints.
- **Fixtures/data**: table of boundary property values including NaN/infinity
  strings where Binder accepts them for validation.
- **Mocks**: none; use existing `ApplicationContextRunner` patterns.
- **Contract classification**: Configuration and manifest contracts.
- **Compatibility expectation**: protected new configuration contract; existing
  connection/model binding remains valid and skill manifests remain unchanged.

### 2. `skillsSelectAliasesButCannotDeclareProviderRetryOverrides`

- **Type**: integration/manifest validation.
- **Location**: existing YAML catalog validation tests plus
  `SupportedSurfaceIntegrationTest.java`.
- **What it proves**: an ordinary skill still selects a model alias and runs
  through its application-owned connection defaults; a hypothetical skill-level
  retry field remains unknown/invalid rather than becoming ambient policy.
- **Fixtures/data**: one existing valid LLM skill and one minimal invalid YAML
  manifest containing a retry-like field.
- **Mocks**: local OpenAI-compatible mock endpoint for the supported surface
  invocation.
- **Contract classification**: Configuration and manifest contracts.
- **Compatibility expectation**: protected YAML behavior and evidence for the
  authoring statement that skills cannot manage provider retry.

### 3. `eachSpringAiBuilderUsesOneAttemptTemplate`

- **Type**: parameterized integration.
- **Location**: new tests under
  `src/test/java/com/lokiscale/loomspan/internal/springai/v1_1/`, reusing
  `ConnectionProtocolTest` mock-server/SDK-fake patterns.
- **What it proves**: one direct adapter invocation causes exactly one
  downstream call for OpenAI, Anthropic, Gemini, and Ollama on a failure the
  dependency would normally retry.
- **Fixtures/data**: real HTTP mock responses for HTTP clients and deterministic
  SDK fake/interceptor for Gemini where necessary.
- **Mocks**: `MockWebServer` and provider SDK fakes at the transport boundary;
  do not mock the Loomspan retry advisor.
- **Contract classification**: Internal or accidentally exposed implementation.
- **Compatibility expectation**: approved replacement of dependency-owned
  retries; no test preserves Spring AI default retry behavior.

### 4. `loomspanThreeAttemptPolicyProducesExactlyThreeEndpointCalls`

- **Type**: integration.
- **Location**: `ProviderAttemptCallAdvisorIntegrationTest.java` plus a real
  OpenAI mock endpoint adapter test.
- **What it proves**: three retryable failures yield three request/sent/failure
  facts and `providerAttempts == 3`, never 30 calls.
- **Fixtures/data**: three 503 responses, zero delay/jitter, max attempts 3.
- **Mocks**: real Spring AI adapter against `MockWebServer`; injected no-wait
  delay source.
- **Contract classification**: Internal or accidentally exposed implementation.
- **Compatibility expectation**: exact-attempt ownership invariant.

### 5. `opaqueOwnershipRequiresRetryDisabled`

- **Type**: unit/startup integration.
- **Location**: integration registry/configuration tests.
- **What it proves**: opaque ownership plus enabled retry fails startup with the
  exact connection path; retry disabled permits one observable client call.
- **Fixtures/data**: test integration descriptor reporting
  `OPAQUE_CLIENT_RETRIES`.
- **Mocks**: small internal fake adapter, not a replaceable Spring bean.
- **Contract classification**: Configuration and manifest contracts.
- **Compatibility expectation**: new safe startup rule; no public adapter SPI.

### 6. `springAiRetryPropertiesCannotChangeLoomspanCardinality`

- **Type**: application-context integration.
- **Location**: Spring AI integration/configuration tests.
- **What it proves**: extreme `spring.ai.retry.*` values do not merge into or
  alter Loomspan retry behavior.
- **Fixtures/data**: two contexts with different Spring AI retry properties and
  identical Loomspan connection settings.
- **Mocks**: one counting endpoint/fake per context.
- **Contract classification**: Configuration and manifest contracts.
- **Compatibility expectation**: preserves documented application-owned
  `loomspan.*` isolation.

### 7. `classifiesOnlyExplicitRetryableProviderFailures`

- **Type**: parameterized unit.
- **Location**: new `ProviderRetryClassifierTest.java` outside the versioned
  dependency package, consuming Loomspan-owned failure facts.
- **What it proves**: retry for supported transport failures, HTTP 408/429/500/
  502/503/504, and allowed OpenRouter error types; no retry for auth,
  authorization, payment, invalid/context/policy/refusal, TLS,
  cancellation/interruption, generic decoding, Loomspan exceptions, and unknown
  exceptions. Precise permanent evidence overrides a broad transient label.
- **Fixtures/data**: closed table of normalized facts and expected
  classification/category/decision.
- **Mocks**: none.
- **Contract classification**: Internal or accidentally exposed implementation.
- **Compatibility expectation**: approved internal policy; no public predicate
  extension.

### 8. `boundedCauseTraversalIsCycleSafeAndMessageIndependent`

- **Type**: unit.
- **Location**: adapter exception translation/classifier tests.
- **What it proves**: only concrete allowlisted transport types are recognized;
  traversal stops at the depth bound, terminates cycles, and does not classify
  matching class-name/message strings.
- **Fixtures/data**: deep cause chain, identity cycle, lookalike exception, and
  supported transport exception below a wrapper.
- **Mocks**: concrete test exceptions/transport types.
- **Contract classification**: Internal or accidentally exposed implementation.
- **Compatibility expectation**: security/correctness invariant.

### 9. `calculatesBoundedBackoffJitterAndRetryAfter`

- **Type**: parameterized unit.
- **Location**: new retry policy/delay calculator tests.
- **What it proves**: default base delays 500ms then 1s; multiplier/cap ordering;
  jitter applied after cap; 0/1 jitter boundaries; overflow saturation; larger
  valid `Retry-After` wins with the correct source; smaller/malformed/negative/
  past/overflow values are ignored.
- **Fixtures/data**: fixed clock, deterministic jitter sequence, delta-seconds
  and RFC HTTP-date table.
- **Mocks**: injected clock and jitter source; never sleep in this unit test.
- **Contract classification**: Internal or accidentally exposed implementation.
- **Compatibility expectation**: bounded runtime behavior.

### 10. `openRouterProfilePreservesSuccessAndNormalizesErrorCompletion`

- **Type**: integration.
- **Location**: `OpenRouterCompatibilityIntegrationTest.java`.
- **What it proves**: ordinary success reaches Spring AI once; retryable and
  permanent documented choice errors become normalized failures; partial
  content is never returned; observed type/code/summary and bounded provider
  diagnostic survive.
- **Fixtures/data**: documented non-streaming success/error JSON, including
  partial content and structured error metadata.
- **Mocks**: `MockWebServer`; real Spring AI `OpenAiApi` and `OpenAiChatModel`.
- **Contract classification**: Internal or accidentally exposed implementation.
- **Compatibility expectation**: explicit OpenRouter behavior under the new
  configuration contract.

### 11. `standardProfileNeverInfersOpenRouter`

- **Type**: parameterized integration.
- **Location**: `OpenRouterCompatibilityIntegrationTest.java`.
- **What it proves**: an OpenRouter-looking connection name, base URL, header,
  and model do not activate the decoder; standard successful OpenAI behavior is
  unchanged.
- **Fixtures/data**: each misleading identity fact with standard profile plus an
  ordinary success response.
- **Mocks**: `MockWebServer`.
- **Contract classification**: Configuration and manifest contracts.
- **Compatibility expectation**: protected explicit opt-in rule.

### 12. `malformedOrOversizedOpenRouterEnvelopeFailsClosedAndBounded`

- **Type**: parameterized integration/unit.
- **Location**: OpenRouter integration and bounded response-capture tests.
- **What it proves**: malformed choice errors, invalid JSON, oversized bodies,
  and invalid UTF-8 produce bounded `CLIENT_DECODING` evidence and no retry
  absent independent retryable status; normal bodies are decoded exactly once.
- **Fixtures/data**: boundary-size payloads at limit−1, limit, and limit+1,
  malformed JSON, and split UTF-8 boundary.
- **Mocks**: `MockWebServer`; test response body stream that detects a second
  read.
- **Contract classification**: Ephemeral diagnostic formats.
- **Compatibility expectation**: current-run diagnostic coherence and bounds.

### 13. `capturesNon2xxStatusRetryAfterAndBoundedBodyWithoutSecrets`

- **Type**: integration.
- **Location**: versioned response-handler tests and
  `SensitiveConnectionDataRedactionTest.java`.
- **What it proves**: status, valid retry-after, content type, bounded body, and
  truncation survive; API key, static headers, base URL, and request body are
  absent from exception display, trace metadata, metrics, and live summaries.
- **Fixtures/data**: 429/503 responses with bounded/oversized bodies and sentinel
  secrets in request-only locations.
- **Mocks**: `MockWebServer`, captured trace, and `SimpleMeterRegistry`.
- **Contract classification**: Ephemeral diagnostic formats.
- **Compatibility expectation**: protected diagnostic security boundary.

### 14. `providerRetryReusesTheSameRequestAndSkipsSemanticAdvisors`

- **Type**: integration.
- **Location**: `ProviderAttemptCallAdvisorIntegrationTest.java` and
  `SpringAiSkillChatClientFactoryTests.java`.
- **What it proves**: a failure then success invokes the raw model twice with
  object-identical `ChatClientRequest`; outer linter/schema/evidence advisors
  enter once; no partial content or mutation is fed into retry.
- **Fixtures/data**: counting semantic advisor and queue model/failure translator.
- **Mocks**: in-memory model at the raw call boundary and injected zero-delay
  source.
- **Contract classification**: Internal or accidentally exposed implementation.
- **Compatibility expectation**: preserves semantic advisor/tool behavior.

### 15. `providerAdvisorDoesNotRetryPostCallLoomspanFailures`

- **Type**: parameterized integration.
- **Location**: `ProviderAttemptCallAdvisorIntegrationTest.java`.
- **What it proves**: response tracing, usage accounting/quota, validators,
  tools, frame management, and response mutation exceptions are propagated
  without another provider send.
- **Fixtures/data**: one successful raw response plus a separately injected
  failure at each post-call seam.
- **Mocks**: focused failing service/advisor doubles; counting raw model.
- **Contract classification**: Internal or accidentally exposed implementation.
- **Compatibility expectation**: protected side-effect boundary.

### 16. `interruptionAndMissionTimeoutStopBackoffWithoutPhantomAttempt`

- **Type**: unit plus mission integration.
- **Location**: provider advisor tests,
  `MissionExecutionEngineTest.java`, and
  `StepLoopMissionExecutionEngineTest.java`.
- **What it proves**: interruption before send and during delay stops promptly,
  preserves the flag, creates no next prepared/sent attempt, and the existing
  mission timeout is never reset. Terminal abort/interruption evidence explains
  the scheduled-but-not-started retry.
- **Fixtures/data**: latch-controlled delay source and short mission timeout;
  never rely on wall-clock sleeps.
- **Mocks**: latch/future-based delay collaborator and counting model.
- **Contract classification**: Application API for mission behavior and
  Ephemeral diagnostic formats for evidence.
- **Compatibility expectation**: protected timeout/exception behavior with new
  current-run facts.

### 17. `providerAttemptQuotaReservesAtomicallyBeforeSend`

- **Type**: unit/concurrency integration.
- **Location**: `SessionUsageServiceTest.java`, `SessionQuotaTest.java`, and
  provider advisor tests.
- **What it proves**: every real send increments exactly once including failures;
  a send beyond the limit is rejected before `MODEL_REQUEST_SENT` and endpoint
  invocation; simultaneous contenders cannot exceed the configured quota; zero
  disables enforcement.
- **Fixtures/data**: limit 1 with two barrier-synchronized callers and a counting
  model.
- **Mocks**: barrier/latches and `NoOpUsageMetricsRecorder`.
- **Contract classification**: Configuration and manifest contracts.
- **Compatibility expectation**: protected new quota semantics.

### 18. `providerAttemptsCoexistsWithResponseOnlyModelCalls`

- **Type**: unit/integration.
- **Location**: `SessionUsageServiceTest.java`, trace completion metadata tests,
  observability DTO mapper tests, and live projector tests.
- **What it proves**: two failed sends plus one successful send produce
  `providerAttempts=3` and `modelCalls=1`; tokens and response precision are
  counted only for the success; terminal/live/REST/configured-limit projections
  contain the exact new fields.
- **Fixtures/data**: deterministic three-attempt sequence and terminal snapshot.
- **Mocks**: in-memory session and usage recorder.
- **Contract classification**: Configuration and manifest contracts plus
  Ephemeral diagnostic formats.
- **Compatibility expectation**: preserves `modelCalls`; adds coherent physical
  accounting.

### 19. `providerAttemptMetricsUseOnlyBoundedDimensions`

- **Type**: unit.
- **Location**: `MicrometerUsageMetricsRecorderTest.java`.
- **What it proves**: driver, framework model, outcome, category, and decision
  tags are present; exception/provider text, raw code/body, attempt IDs,
  credentials, and model content are absent from all meter IDs.
- **Fixtures/data**: sentinel sensitive/unbounded strings in failure details.
- **Mocks**: `SimpleMeterRegistry`.
- **Contract classification**: Internal or accidentally exposed implementation.
- **Compatibility expectation**: bounded operational telemetry.

### 20. `failedAttemptLifecycleAndTerminalFailureLinkAreExact`

- **Type**: Java integration.
- **Location**: `ProviderAttemptCallAdvisorIntegrationTest.java`,
  `ExecutionTraceContractTest.java`, `LoomspanSessionTest.java`, mission/step
  engine tests.
- **What it proves**: failure→success emits alternate terminal then response and
  no error; permanent/exhausted failure emits one PR 22 error linked to the last
  attempt/sequence and closest model frame; original exception identity is
  propagated; wrappers with registered causes reuse the same link; recovered
  failures never register a terminal error.
- **Fixtures/data**: queue failures/responses, nested frames, normal cause
  wrapper, and equal-message distinct throwables.
- **Mocks**: deterministic model and trace reader; no class/message matching.
- **Contract classification**: Ephemeral diagnostic formats and Application API
  exception propagation.
- **Compatibility expectation**: current-run diagnostic coherence while
  preserving PR 22 identity semantics.

### 21. `attemptReasonAndProviderNumberResetAcrossSemanticRetry`

- **Type**: integration.
- **Location**: provider advisor plus linter/schema advisor integration tests.
- **What it proves**: first attempt is `INITIAL/1`; unchanged resend is
  `PROVIDER_RETRY/2`; a later semantic advisor invocation is
  `SEMANTIC_RETRY/1`; all have unique IDs and monotonic physical attempt numbers
  in one retry sequence.
- **Fixtures/data**: provider failure→success followed by one linter semantic
  retry.
- **Mocks**: queue model and existing linter advisor.
- **Contract classification**: Ephemeral diagnostic formats.
- **Compatibility expectation**: new current-run attempt identity contract.

### 22. `goAcceptsFailedAttemptsAndRejectsImpossibleRetryTransitions`

- **Type**: parameterized Go unit.
- **Location**: `loomspan-console/internal/traceanalysis/calculations_test.go`
  and focused attempt tests if split for readability.
- **What it proves**: alternate response/failure terminal lifecycle; no gap for
  known failures; gap for genuinely incomplete send; strict reason/provider
  numbering; no duplicate terminals; no retry after permanent/exhausted;
  canceled scheduled retry only with matching terminal abort/interruption;
  terminal error resolves to the last failure.
- **Fixtures/data**: minimal NDJSON strings, one invariant mutation per case.
- **Mocks**: in-memory artifact sink used by existing processor tests.
- **Contract classification**: Ephemeral diagnostic formats.
- **Compatibility expectation**: coherent new reader; old dual shape is not
  accepted or preserved.

### 23. `javaFixtureCorpusDefinesProviderRetrySemantics`

- **Type**: Java generator plus Go contract integration.
- **Location**: `ConsoleTraceFixtureCorpusTest.java`, committed
  `loomspan-console-fixtures`, and `fixture_corpus_test.go`.
- **What it proves**: valid recovery, exhaustion, permanent failure,
  semantic reset, and canceled backoff; invalid duplicate terminal, identity,
  numbering, transition, linkage, configured-limit, and diagnostic cases;
  deterministic Java-to-Go expected results.
- **Fixtures/data**: Java-owned valid traces and minimal invalid mutations. Add
  `providerAttempts` and `maxProviderAttempts` to all applicable current shapes.
- **Mocks**: deterministic IDs/clock and fixture generation helpers.
- **Contract classification**: Ephemeral diagnostic formats.
- **Compatibility expectation**: atomic current-release break, no historical
  fixtures or legacy reader. Preserve LF line endings.

### 24. `browserAttemptDtoMapsNeutralFailureFactsWithoutReclassification`

- **Type**: Go unit/contract fixture.
- **Location**: `loomspan-console/internal/browserapi/trace_analysis_test.go` and
  `contracts_test.go`.
- **What it proves**: every enriched attempt/configured-limit/usage field maps
  exactly; absent optional provider facts remain absent; no Go/browser adapter
  inference or OpenRouter parsing.
- **Fixtures/data**: neutral `AttemptSummary` for success, failed retry, exhausted,
  and incomplete cases.
- **Mocks**: existing fake trace-analysis service.
- **Contract classification**: Ephemeral diagnostic formats.
- **Compatibility expectation**: synchronized Go-to-TypeScript current-release
  DTO.

### 25. `traceExplorerShowsChronologicalAttemptsAndUsesExistingPayloadViewer`

- **Type**: React component/integration.
- **Location**: `TraceViews.test.tsx` and `TraceExplorer.test.tsx`.
- **What it proves**: initial/semantic/provider labels, succeeded/failed/
  incomplete outcomes, category/decision/delay/source, bounded observed fields,
  chronological ordering, deliberate failed-attempt payload range loading, and
  no raw-record/provider-specific decoder path.
- **Fixtures/data**: paged attempts with one failed payload descriptor and inert
  HTML/script-like diagnostic text.
- **Mocks**: existing mocked browser API; assert only `getPayloadRange` is called
  for the failed-attempt action.
- **Contract classification**: Ephemeral diagnostic formats.
- **Compatibility expectation**: current-run debugging usefulness and inert
  content handling.

### 26. `terminalFailureAndLastAttemptNavigateBidirectionally`

- **Type**: React component/integration.
- **Location**: `TraceExplorer.test.tsx` and `TraceFailureFocus` tests.
- **What it proves**: failure selection resolves the linked last attempt, attempt
  selection resolves terminal failure, deep pages continue as necessary, and
  navigation remains scope-bound.
- **Fixtures/data**: terminal failure and last failed attempt on different result
  pages.
- **Mocks**: paged browser API responses.
- **Contract classification**: Ephemeral diagnostic formats.
- **Compatibility expectation**: PR 22 navigation extended coherently.

### 27. `providerRetryLiveActivityIsBoundedAndContainsNoRawFailureContent`

- **Type**: Java projector, Go live DTO, and React presentation unit tests.
- **Location**: `LiveActivityProjectorTest.java`, Go live/application-client
  tests, `activityPresentation.test.ts`, and `ActivityNarrative.test.tsx`.
- **What it proves**: `MODEL_ATTEMPT_FAILED` is a recognized closed activity
  kind; retry summary contains attempt number and delay only; details remain
  within byte/field bounds; body, exception message, credentials, and partial
  model content are absent.
- **Fixtures/data**: maximum-length structured fields plus sentinel secrets/raw
  body.
- **Mocks**: canonical trace record through existing projection/transport
  fixtures.
- **Contract classification**: Ephemeral diagnostic formats.
- **Compatibility expectation**: bounded Java→SSE→Go→browser live semantics.

### 28. `publicSurfaceAndImportsExposeNoRetrySpiOrProviderTypes`

- **Type**: architecture.
- **Location**: `LoomspanPublicSurfaceArchitectureTest.java`,
  `LoomspanAutoConfigurationBoundaryTest.java`, and a focused import-boundary
  ArchUnit test.
- **What it proves**: no Application API/Supported SPI delta; no replaceable
  bean; Spring Retry/provider builder/provider exception imports stay inside the
  versioned package; no public signature leaks them; no reflection or runtime
  version branching.
- **Fixtures/data**: compiled production classes.
- **Mocks**: none.
- **Contract classification**: Supported SPI and Internal or accidentally
  exposed implementation.
- **Compatibility expectation**: protected absence of extension points and
  approved atomic internal reorganization.

### 29. `exactReleaseMismatchStillStopsBeforeObservabilityConsumption`

- **Type**: Go application-client regression/integration.
- **Location**: retain
  `TestClientConsumesCommittedInstanceFixtureOnlyAfterExactCompatibility` in
  `internal/applicationclient/client_test.go` and add a route-sentinel assertion
  to the target integration test proving no post-probe route is requested.
- **What it proves**: a different exact `consoleCompatibilityVersion` is rejected
  before snapshot, SSE, catalog, artifact, or new attempt fields are consumed.
- **Fixtures/data**: committed current instance fixture and one-character/
  release-suffix mismatch.
- **Mocks**: HTTP server that fails the test if any route after instance probe is
  requested.
- **Contract classification**: Persisted or serialized contracts for the
  release-coupled application/Console boundary.
- **Compatibility expectation**: protected exact lockstep gate; no trace-schema
  fallback.

### 30. `supportedSurfacePropagatesOriginalFinalProviderException`

- **Type**: Application API integration.
- **Location**: `SupportedSurfaceIntegrationTest.java` or a focused adjacent
  test using only `SkillTemplate` and public API values.
- **What it proves**: exhausted/permanent provider failure reaches the public
  facade as the original provider exception/cause identity expected by current
  execution behavior; retry internals and normalized types do not enter public
  signatures.
- **Fixtures/data**: local compatible endpoint returning a permanent failure or
  deterministic adapter failure.
- **Mocks**: endpoint only; test does not replace internal runtime beans.
- **Contract classification**: Application API.
- **Compatibility expectation**: protected public invocation and exception
  propagation path.

## How to Run

All commands are PowerShell commands from the repository root unless a working
directory is stated.

### Red-test sequence

1. Add only `bindsExplicitOpenRouterProfileAndProviderRetryDefaults` and run:

   ```powershell
   .\mvnw.cmd -pl loomspan-spring-boot-starter -Dtest=LoomspanPropertiesTest test
   ```

   Confirm it fails because strict configuration rejects the new field.

2. After scaffolding only the new configuration model, add the incident test and
   run:

   ```powershell
   .\mvnw.cmd -pl loomspan-spring-boot-starter -Dtest=OpenRouterCompatibilityIntegrationTest#openRouterErrorCompletionRetriesThenReturnsOnlyTheSuccessfulResponse test
   ```

   Confirm it fails on the first `finish_reason: "error"` decode with one
   endpoint call.

### Focused Java loops

```powershell
.\mvnw.cmd -pl loomspan-spring-boot-starter -Dtest=LoomspanPropertiesTest,LoomspanSessionPropertiesTest,ConfigurationMetadataTest test
.\mvnw.cmd -pl loomspan-spring-boot-starter -Dtest='*SpringAi*Test,*Connection*Test,*OpenRouter*Test' test
.\mvnw.cmd -pl loomspan-spring-boot-starter -Dtest='*ProviderAttempt*Test,*ProviderRetry*Test' test
.\mvnw.cmd -pl loomspan-spring-boot-starter -Dtest=SessionUsageServiceTest,SessionQuotaTest,MicrometerUsageMetricsRecorderTest test
.\mvnw.cmd -pl loomspan-spring-boot-starter -Dtest=LoomspanSessionTest,ExecutionTraceContractTest,LiveActivityProjectorTest test
.\mvnw.cmd -pl loomspan-spring-boot-starter -Dtest=MissionExecutionEngineTest,StepLoopMissionExecutionEngineTest test
.\mvnw.cmd -pl loomspan-spring-boot-starter -Dtest=LoomspanPublicSurfaceArchitectureTest,LoomspanAutoConfigurationBoundaryTest test
```

If Surefire wildcard selection differs on Windows, run the named classes
individually rather than weakening or skipping the focused test.

### Fixture generation and cross-language checks

Regenerate only after the Java producer tests are green:

```powershell
.\mvnw.cmd -pl loomspan-spring-boot-starter test -Dtest=ConsoleTraceFixtureCorpusTest -Dloomspan.console.fixtures.regenerate=true -DfailIfNoTests=false
git diff --check
git diff -- loomspan-console-fixtures
$fixtureDiffBefore = git diff --binary -- loomspan-console-fixtures | Out-String
.\mvnw.cmd -pl loomspan-spring-boot-starter test -Dtest=ConsoleTraceFixtureCorpusTest -Dloomspan.console.fixtures.regenerate=true -DfailIfNoTests=false
$fixtureDiffAfter = git diff --binary -- loomspan-console-fixtures | Out-String
if ($fixtureDiffBefore -cne $fixtureDiffAfter) { throw "Fixture regeneration is not deterministic" }
```

The in-memory diff comparison proves the second regeneration adds no new
changes without staging the first result. Inspect LF line endings for all
committed fixture JSON/NDJSON.

Then, from `loomspan-console`:

```powershell
go test ./internal/traceanalysis ./internal/browserapi ./internal/applicationclient ./internal/live
go test ./...
go run ./internal/buildtool verify
```

### Full verification

From the repository root:

```powershell
.\mvnw.cmd -pl loomspan-spring-boot-starter test -DfailIfNoTests=false
```

From `loomspan-console`:

```powershell
go test ./...
go run ./internal/buildtool verify
$env:PATH = "C:\msys64\mingw64\bin;" + $env:PATH
$env:CGO_ENABLED = "1"
go test -race ./...
```

Run browser E2E after a canonical build:

```powershell
npx playwright install chromium
npm --prefix web run test:e2e
```

No external credential is required for automated verification. Live-provider
tests are optional and must never log credentials. Use fixed clocks, injected
jitter/delay sources, latches, local mock servers, and deterministic IDs instead
of sleeps or Internet calls.

## Manual Verification

1. Run a local mock OpenRouter-compatible endpoint that responds first with a
   documented retryable error completion containing partial content and then
   with a normal success.
2. Configure an application-owned connection with explicit
   `openai.compatibility-profile: openrouter`, deterministic short retry values,
   and a model alias. Keep the YAML skill unchanged except for selecting that
   alias.
3. Invoke the skill and verify exactly two byte-equivalent request
   prompt/options bodies, successful content only, two visible attempts,
   `providerAttempts=2`, `modelCalls=1`, and no `ERROR_RECORDED`.
4. Acquire the trace in Console. Verify chronological labels, failed-attempt
   decision/delay, bounded payload through the existing viewer, and no raw body
   in summaries/metrics/live narrative.
5. Repeat with a permanent OpenRouter error and with three retryable failures.
   Verify one call for permanent failure, three for exhaustion, and one terminal
   PR 22 error linked bidirectionally to the final attempt.
6. Repeat with a retry delay longer than a short mission timeout. Verify prompt
   cancellation, no second send, preserved abort/interruption evidence, and no
   missing-response gap for the known failed attempt.
7. Review startup errors for invalid retry/profile/quota configuration and
   opaque ownership. Confirm exact property paths and absence of secret values.

## Exit Criteria

- [ ] The first configuration red test fails on the current checkout for the
  expected unknown-field reason.
- [ ] After configuration scaffolding, the minimal OpenRouter incident test
  fails on the current decoder with one endpoint call before the decoder/retry
  implementation.
- [ ] Both red tests pass after implementation without preserving a test that
  requires `InvalidFormatException`.
- [ ] Every supported Spring AI 1.1.6 model builder proves one dependency call
  per direct adapter invocation.
- [ ] A three-attempt Loomspan policy proves exactly three endpoint calls,
  attempt records, and physical-attempt increments—not thirty.
- [ ] Retry classification tests cover all positive and negative ticket cases,
  bounded/cyclic causes, precise-fact precedence, and no message-based matching.
- [ ] Backoff, jitter, `Retry-After`, overflow, interruption, and timeout tests
  are deterministic and contain no timing sleeps.
- [ ] OpenRouter tests prove explicit opt-in, ordinary success preservation,
  partial-content rejection, bounded diagnostics, malformed/oversized failure,
  and standard-profile non-inference.
- [ ] `providerAttempts` is atomic and equals actual sends; rejected sends do
  not increment or emit `MODEL_REQUEST_SENT`; `modelCalls` remains response-only.
- [ ] Recovered attempts emit no `ERROR_RECORDED`; terminal failure emits one
  PR 22 error linked to the last attempt and propagates the original exception.
- [ ] Java trace writer, committed fixtures, Go analyzer/index/query layer,
  browser DTOs, and TypeScript/UI accept one coherent current-release shape.
- [ ] Go rejects every invalid lifecycle/numbering/transition/linkage mutation
  and does not label known failed attempts as response gaps.
- [ ] The exact `consoleCompatibilityVersion` mismatch is still rejected before
  any snapshot, SSE, catalog, artifact, or trace-attempt consumption.
- [ ] Browser tests prove chronological outcomes, existing bounded payload
  access, bidirectional terminal navigation, inert diagnostic rendering, and
  bounded live activity without raw failure content.
- [ ] Tests cited by `model-selection-and-connections.md` establish application
  ownership and explicit OpenRouter opt-in; tests cited by
  `traces-and-debugging.md` establish attempt, counter, failure, gap, and quota
  semantics.
- [ ] Existing YAML skill/model-alias behavior and public `SkillTemplate`
  invocation pass; no skill-level retry field or public retry type is added.
- [ ] Architecture/import tests prove no new Supported SPI,
  `@ConditionalOnMissingBean`, leaked provider/Spring Retry signature,
  reflection, or runtime version branching.
- [ ] Obsolete advisor/default-retry/current-trace paths are removed rather than
  retained behind overloads, aliases, fallbacks, bridges, or legacy readers.
- [ ] Fixture regeneration is deterministic on a second run and committed
  JSON/NDJSON uses LF.
- [ ] Full Java, Go, canonical Console, browser E2E, and Windows race-detector
  commands pass.
- [ ] Manual mock OpenRouter recovery, exhaustion, permanent failure, and
  timeout scenarios are complete.
- [ ] `git diff --check` passes and the worktree contains only intentional PR 23
  changes.
