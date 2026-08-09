# PR 23 - Add Bounded, Observable Provider Retries Behind a Spring AI Adapter

## Status

Implementation-ready ticket. Planning and codebase verification completed on
2026-08-08 against Loomspan and Spring AI 1.1.6. Depends on PR 22. No
implementation has started.

Implement after PR 22 lands. This ticket consumes PR 22's frame-linked terminal
failure and bounded opaque diagnostic contracts; it must not invent a parallel
error or blob representation. If PR 22 lands differently, reconcile this ticket
before coding.

This revision deliberately selects the smallest coherent retry feature. It does
not attempt to normalize every provider SDK, redesign execution deadlines and
usage semantics, or promise compatibility with arbitrary Spring AI releases.

## Outcome

Loomspan retries a conservatively classified transient provider failure by
resending the exact same synchronous model request. The default is three total
attempts per unchanged request, with short exponential backoff, jitter,
`Retry-After`, interruption, the existing mission timeout, and a session-wide
physical-attempt quota enforced.

Each physical attempt is visible in the trace. A known failed attempt terminates
with `MODEL_ATTEMPT_FAILED` rather than appearing as a missing-response gap. A
recovered attempt does not create a general execution error. When attempts are
exhausted or a failure is not retryable, the last exception propagates and PR 22
records one terminal `ERROR_RECORDED` linked to the last attempt.

The motivating OpenRouter `finish_reason: "error"` response is decoded inside
an explicit OpenRouter compatibility profile. Its documented typed error is
classified and its bounded raw error response is retained as diagnostic text.
Partial content from an error completion is never treated as success.

Spring AI-specific construction, retry disabling, exception translation, and
OpenRouter response handling are confined to one internal version-scoped
integration boundary. Loomspan's retry, trace, quota, and console contracts do
not depend on Spring AI builder or exception classes.

## Complexity decision

Provider retry has real cost, but most of the value does not require a general
provider-resilience subsystem.

The high-value behavior is:

- retry a short allowlist of common transient failures;
- retry an explicitly recognized OpenRouter provider error;
- prevent nested Spring AI and Loomspan retry multiplication;
- show every Loomspan-observed attempt and its outcome;
- preserve final failure diagnostics through PR 22; and
- bound retries by attempts, backoff, interruption, mission timeout, and quota.

The earlier broad design added disproportionate complexity through a generic
multi-provider classifier registry, a new inherited deadline model, a
three-level logical/cycle/physical attempt hierarchy, redefinition of existing
`modelCalls`, and a second diagnostic-range API for attempt errors. Those pieces
are not required to recover from the motivating incident safely.

This ticket therefore chooses the following 80-percent design:

- one internal Spring AI integration interface and one implementation for the
  supported Spring AI release line;
- one retry loop in the existing innermost model-attempt advisor;
- one compact attempt-failure trace fact;
- one new physical `providerAttempts` counter and quota;
- existing mission timeout cancellation rather than a new deadline subsystem;
- existing trace payload access for attempt diagnostics rather than another
  specialized diagnostic service; and
- provider-specific response decoding only for the evidenced OpenRouter case.

This is not a temporary shortcut whose removal is assumed. It is the intended
minimal architecture. Add broader abstractions only when another concrete
provider/version case supplies evidence that the smaller boundary cannot handle
cleanly.

## Triggering incident

The `handleIncident` execution completed planning, classification, network
investigation, and runbook lookup. Its fourth model call used Qwen through an
OpenAI-compatible OpenRouter connection and emitted `MODEL_REQUEST_PREPARED`
and `MODEL_REQUEST_SENT`, but no response record.

OpenRouter returned an HTTP-success Chat Completions body containing its
documented provider-error form with `finish_reason: "error"`. Spring AI 1.1.6
failed while decoding `OpenAiApi.ChatCompletion`:

```text
org.springframework.web.client.RestClientException:
Error while extracting response for type
[org.springframework.ai.openai.api.OpenAiApi$ChatCompletion]

Caused by: org.springframework.http.converter.HttpMessageNotReadableException:
JSON parse error: Cannot deserialize value of type
OpenAiApi$ChatCompletionFinishReason from String "error"

Caused by: com.fasterxml.jackson.databind.exc.InvalidFormatException:
Cannot deserialize value of type OpenAiApi$ChatCompletionFinishReason from
String "error"
```

The stack passed through `RetryTemplate.doExecute`, but the failure was not a
retryable type under Spring AI's default policy. The OpenRouter error object was
lost before Loomspan could classify or preserve it. A retry might have
succeeded, but `finish_reason: "error"` alone cannot establish whether the
underlying failure was transient.

## Verified current behavior

### Hidden dependency retry

The four Loomspan connection factories build Spring AI models without supplying
a retry template. OpenAI, Anthropic, Google GenAI, and Ollama consequently use
Spring AI 1.1.6's `RetryUtils.DEFAULT_RETRY_TEMPLATE`:

- up to 10 attempts;
- retry `TransientAiException` and `ResourceAccessException`;
- exponential backoff beginning at 2 seconds, multiplier 5, capped at 3
  minutes; and
- all HTTP 4xx responses, including 408 and 429, classified non-transient by
  the default response error handler.

These retries occur below `ModelAttemptCallAdvisor`. Multiple HTTP calls can
therefore appear as one Loomspan attempt. The OpenRouter Jackson failure is a
`RestClientException`, not one of the configured retry types.

### Current attempt and accounting boundaries

`ModelAttemptCallAdvisor` allocates one attempt, records prepared and sent,
calls `chain.nextCall(request)`, and records response and usage only on success.
Go accepts `PREPARED -> SENT -> RESPONSE_RECEIVED`; a sent attempt without a
response is projected as `MODEL_ATTEMPT_RESPONSE_MISSING`.

`SessionUsageService.recordModelResponse` runs only after a response. The
existing `modelCalls` and `maxModelCalls` therefore count successful response
accounting events, not all physical sends. This ticket does not redefine that
existing contract; it adds an explicit physical-attempt counter and guardrail.

## Spring AI compatibility position

### Do not promise arbitrary dependency overrides

Loomspan necessarily compiles against concrete Spring AI types. It cannot
truthfully guarantee that arbitrary Spring AI versions are source-, binary-, or
behavior-compatible. A user forcing an incompatible release can otherwise see
linkage errors or subtly changed client behavior.

The supported policy is:

- Loomspan's BOM pins one primary tested Spring AI version.
- Release documentation lists the exact Spring AI versions exercised by CI.
- A user may override to another binary-compatible patch version in the same
  supported release line, but an unlisted version is not claimed as tested.
- Do not add a brittle runtime version-string gate. Normal class linkage and
  integration tests remain authoritative.
- Moving to an incompatible Spring AI minor or major line is a deliberate
  Loomspan adapter upgrade.
- If real users require two incompatible release lines concurrently, add
  separate integration artifacts such as `loomspan-spring-ai-1-1` and
  `loomspan-spring-ai-1-2`; do not fill the core with reflection, version
  branches, or compatibility shims in advance.

### Isolate all dependency-specific behavior

Add one internal boundary, named here for clarity:

```java
interface SpringAiProviderIntegration {
    ChatModel createModel(String connectionName, ConnectionProperties properties);
    AttemptOwnership attemptOwnership(String connectionName);
    ProviderFailureDetails classify(Throwable failure, ModelExecutionIdentity identity);
}
```

The exact shape may follow local conventions, but the boundary must preserve
these responsibilities:

- construct/configure dependency models;
- disable dependency-owned retries using that release line's supported API;
- expose whether Loomspan has exact attempt ownership;
- translate supported Spring AI/SDK/HTTP exceptions into a small internal
  dependency-neutral failure fact; and
- install the explicit OpenRouter response decoder.

Only the internal Spring AI adapter package may import Spring AI provider API
builders, Spring Retry types used to configure those builders, or
provider-specific exception types. The retry advisor, trace recorder, usage
service, Go analyzer, and browser operate only on Loomspan-owned types.

Use two internal capability values:

- `EXACT_ATTEMPT_OWNERSHIP`: dependency-level automatic retry is disabled, so
  every call Loomspan makes to `ChatModel` is one observable provider attempt.
- `OPAQUE_CLIENT_RETRIES`: the dependency may make hidden retries that Loomspan
  cannot enumerate.

Provider retry may be enabled only with `EXACT_ATTEMPT_OWNERSHIP`. If a future
adapter reports `OPAQUE_CLIENT_RETRIES`, allow the connection only when
Loomspan provider retry is disabled; otherwise fail startup with the exact
connection property path and an actionable explanation. One opaque client call
then remains one Loomspan-observed attempt and must not be advertised as an
exact physical-wire count.

For the pinned Spring AI 1.1.6 integration, all four current model builders
expose `retryTemplate(...)`. Supply a shared internal one-attempt template to
OpenAI, Anthropic, Google GenAI, and Ollama, and report exact ownership only
after mock-server/SDK tests demonstrate that no other enabled client retry
occurs on the supported call path.

Do not expose this adapter or capability as an application SPI,
`@ConditionalOnMissingBean`, or public API in this PR.

## Resolved implementation design

### 1. Keep one retry loop at the physical model boundary

Replace `ModelAttemptCallAdvisor` with internal
`ProviderAttemptCallAdvisor`. It remains Loomspan's innermost advisor, after the
linter, output-schema, evidence, and other semantic advisors.

For each invocation by the outer chain, it receives one immutable
`ChatClientRequest` and performs:

1. verify exact attempt ownership for the selected connection;
2. enforce the provider-attempt guardrail before each send;
3. allocate one attempt ID and monotonically increasing attempt number;
4. record `MODEL_REQUEST_PREPARED` and `MODEL_REQUEST_SENT`;
5. call the configured Spring AI `ChatModel` exactly once;
6. on success, record response and existing usage accounting exactly once;
7. on a supported failure, classify it and record `MODEL_ATTEMPT_FAILED`;
8. if retryable and attempts remain, wait interruptibly and invoke the model
   again with the same request object; otherwise propagate the last exception.

The provider retry loop does not re-enter outer semantic advisors. It cannot
add linter/schema/evidence feedback, rebuild a prompt, or consume partial
output. After one successful response returns, outer advisors may independently
request a semantic retry using their current behavior. Existing advisor trace
facts remain the authority for why that later model request exists.

The provider advisor catches only exceptions from the raw model invocation. It
must not retry errors raised later by Loomspan response tracing, usage
guardrails, validators, tools, frame management, or response mutation.

Use injectable internal delay and jitter sources for deterministic tests. Do
not expose a general retry callback or application-defined predicate.

### 2. Add only the attempt identity needed for provider retry

Retain the existing canonical fields:

```json
{
  "retrySequenceId": "retry-opaque",
  "attemptId": "attempt-opaque",
  "attemptNumber": 2
}
```

Add:

```json
{
  "attemptReason": "PROVIDER_RETRY",
  "providerAttemptNumber": 2
}
```

Rules:

- `attemptNumber` remains the positive monotonic physical-attempt number within
  the existing `retrySequenceId`.
- `providerAttemptNumber` starts at 1 for each invocation of the provider
  advisor and increments only when it resends that same request.
- `attemptReason` is `INITIAL` when this is the first attempt in the retry
  sequence, `SEMANTIC_RETRY` when an outer advisor invokes the provider boundary
  again after a prior completed response, and `PROVIDER_RETRY` when the current
  provider loop resends the unchanged request.
- A provider retry receives a new `attemptId` and increments both numbers while
  retaining `retrySequenceId`.
- A semantic retry receives a new `attemptId`, increments `attemptNumber`, and
  resets `providerAttemptNumber` to 1.

Do not add a separately numbered request-cycle hierarchy in this PR. Existing
explicit advisor records plus `attemptReason` provide enough evidence to
distinguish semantic and provider retries.

### 3. Record known failed attempts

Add trace record `MODEL_ATTEMPT_FAILED` with the complete attempt/model identity
and these small structured fields:

```json
{
  "failureClassification": "TRANSIENT",
  "failureCategory": "PROVIDER_UNAVAILABLE",
  "retryDecision": "RETRY",
  "retryDelayMillis": 500,
  "retryDelaySource": "BACKOFF"
}
```

Allowed classifications are `TRANSIENT`, `PERMANENT`, and `UNKNOWN`. Required
categories are:

- `CONNECTIVITY`;
- `TIMEOUT`;
- `RATE_LIMITED`;
- `PROVIDER_OVERLOADED`;
- `PROVIDER_UNAVAILABLE`;
- `SERVER_ERROR`;
- `AUTHENTICATION`;
- `AUTHORIZATION`;
- `PAYMENT_REQUIRED`;
- `INVALID_REQUEST`;
- `CONTEXT_LIMIT`;
- `CONTENT_POLICY`;
- `CLIENT_DECODING`; and
- `UNKNOWN`.

Allowed retry decisions are `RETRY`, `DO_NOT_RETRY`, and
`ATTEMPTS_EXHAUSTED`. Cancellation/interruption during backoff is recorded by
the normal PR 22 terminal failure path rather than expanding the attempt state
machine.

Allowed delay sources are `NONE`, `BACKOFF`, and `RETRY_AFTER`.
`retryDelayMillis` is positive only for `RETRY`. When a valid `Retry-After` is
larger than calculated backoff, it controls the delay and source.

Record a bounded data object containing only observed provider facts:

```json
{
  "httpStatus": 502,
  "providerErrorType": "provider_unavailable",
  "providerErrorCode": "provider_disconnected",
  "summary": "Provider disconnected during generation",
  "diagnostics": [
    {
      "kind": "PROVIDER_ERROR",
      "contentType": "application/json; charset=utf-8",
      "text": "{...bounded observed provider error...}",
      "truncated": false,
      "captureLimitBytes": 1048576
    }
  ]
}
```

Reuse PR 22's diagnostic object and bounds. The small structured fields exist
only for retry decisions and filtering; Java, Go, and TypeScript do not parse
the diagnostic text.

Do not add a new attempt-diagnostic range endpoint. The attempt record already
participates in the existing logical-payload descriptor/range service. The
browser may deliberately open that record's bounded payload using the existing
payload viewer. PR 22's specialized failure diagnostic viewer remains for
`ERROR_RECORDED`.

Attempt lifecycle becomes exactly one of:

```text
PREPARED -> SENT -> RESPONSE_RECEIVED
PREPARED -> SENT -> ATTEMPT_FAILED
```

An attempt may not have both terminal facts. A sent attempt with neither remains
a genuine analysis gap.

A recovered `MODEL_ATTEMPT_FAILED` does not emit `ERROR_RECORDED`. When the
provider advisor propagates the final exception, register the last attempt link
with PR 22's session-local failure recorder by throwable identity/cause. The one
terminal error then carries `attemptId` and `retrySequenceId`. Do not wrap an
exception solely to carry metadata and do not match by class/message.

If `MODEL_ATTEMPT_FAILED` records `RETRY` and the outer mission timeout or an
interrupt cancels the subsequent backoff, no next attempt is emitted. The
append-only retry decision remains true at the time it was made; PR 22's
terminal cancellation/interruption evidence explains why the scheduled retry
did not start. Go permits this shape only for an aborted/interrupted terminal
execution.

### 4. Use a deliberately small retry classifier

The dependency adapter returns a Loomspan-owned `ProviderFailureDetails` with
optional status, retry-after, provider type/code, summary, and diagnostics. The
provider advisor applies one internal policy. Do not create a public registry
or a provider-by-provider plugin framework.

Retry by default:

- supported connection/read timeout, connection reset, premature EOF, and
  temporary name-resolution transport exceptions;
- HTTP 408, 429, 500, 502, 503, and 504; and
- OpenRouter `error_type` values `rate_limit_exceeded`,
  `provider_overloaded`, `provider_unavailable`, `timeout`, `server`, and
  `unmapped`.

Do not retry by default:

- authentication, authorization, payment/credit, invalid request,
  context/token/payload limits, moderation, content policy, and refusal;
- TLS certificate, trust, or hostname-verification failures;
- cancellation or interruption;
- a generic Jackson/message-converter/deserialization failure;
- Loomspan validation, quota, tracing, tool, or execution exceptions; and
- an unknown exception without an observed retryable status or explicitly
  supported transport type.

Walk a bounded, cycle-safe cause chain only for a short explicit transport-type
allowlist. Do not retry based on exception class names represented as strings,
exception-message matching, or Spring AI's `TransientAiException` label when a
more precise observed fact contradicts it.

This PR does not implement rich Anthropic, Gemini, or Ollama error-envelope
decoders. Their ordinary HTTP/typed transport failures may retry when the
adapter can expose a supported status or transport type without reflection or
message parsing. Unknown provider-specific bodies remain permanent/unknown and
visible through PR 22 if terminal.

### 5. Decode only the evidenced OpenRouter extension

Add explicit configuration:

```yaml
loomspan:
  connections:
    openrouter:
      driver: openai
      base-url: https://openrouter.ai/api/v1
      api-key: ${OPENROUTER_API_KEY}
      openai:
        compatibility-profile: openrouter
```

The values are `standard` and `openrouter`; default is `standard`. Never infer
the profile from connection name, URL, host, headers, or model.

Inside the version-scoped Spring AI OpenAI adapter, use the supported
`OpenAiApi.Builder.restClientBuilder(...)` seam to observe a non-streaming Chat
Completions response before Spring AI's enum decoder loses it. The bounded
decoder must:

- preserve an ordinary successful response for exactly one downstream decode;
- recognize a choice with `finish_reason: "error"`;
- capture the documented choice-level `error` object;
- normalize observed `code`, `message`, `metadata.error_type`, and
  `provider_code`;
- retain the bounded error-shaped response as `PROVIDER_ERROR` diagnostic text;
- reject error completions even when partial assistant content exists;
- never deliver partial error content to validators, tools, session success
  state, or callers; and
- classify malformed/oversized error envelopes as `CLIENT_DECODING` without
  retry unless another observed fact is independently retryable.

The current Loomspan skill path is synchronous and materializes responses.
Streaming and retry after delivered partial tokens are out of scope.

For non-2xx OpenAI-compatible responses, install a dependency-adapter response
handler that preserves status, valid `Retry-After`, and a bounded body in the
internal normalized exception. Do not copy request headers, credentials, or
connection secrets into diagnostics. Use equivalent supported seams for other
current HTTP drivers only where necessary to expose status; do not build
provider-specific schema models for them.

If a later Spring AI version natively supports OpenRouter error completions,
only its version-scoped adapter changes. Remove the redundant decoder from that
adapter; the Loomspan failure, retry, trace, and console contracts remain
unchanged.

### 6. Keep retry configuration bounded and local to a connection

Add:

```yaml
loomspan:
  connections:
    openrouter:
      provider-retry:
        enabled: true
        max-attempts: 3
        initial-backoff: 500ms
        multiplier: 2.0
        max-backoff: 5s
        jitter: 0.20
```

Defaults for every connection are:

- `enabled: true`;
- `max-attempts: 3`, meaning one initial send plus at most two retries for the
  unchanged request;
- `initial-backoff: 500ms`;
- `multiplier: 2.0`;
- `max-backoff: 5s`; and
- `jitter: 0.20`, a uniformly bounded plus/minus 20 percent adjustment after
  capping exponential delay.

Validation:

- attempts: 1 through 10;
- nonnegative initial/maximum delay, with maximum not less than initial;
- finite multiplier at least 1.0;
- finite jitter from 0.0 through 1.0; and
- disabled means one attempt regardless of other values.

Use overflow-safe duration arithmetic. Honor valid HTTP `Retry-After`
delta-seconds and HTTP-date for retryable responses by taking the larger of
backoff and server delay. Ignore malformed, negative, or past values.

Backoff is interruptible. The retry loop runs inside the existing mission
future, so it does not reset or extend `missionTimeout`; the existing timeout
cancels/interrupts provider work and backoff. Check interruption before each send
and after each failed wait and preserve the interrupt flag.

Do not introduce a new inherited deadline abstraction in this PR. If later
evidence shows that avoiding a retry shortly before timeout materially matters,
design deadline propagation as its own execution-semantics change rather than
coupling it to provider retry.

### 7. Add physical-attempt accounting without redefining model calls

Add `providerAttempts` to the session usage snapshot. Increment it atomically
immediately before every actual Loomspan-owned model send, including sends that
later fail. Add quota:

```yaml
loomspan.session.quotas.max-provider-attempts: 192
```

Default 192 is three times the current default `maxModelCalls` of 64. Enforce it
before sending the request that would exceed the limit. A rejected send does
not increment the counter and must not emit `MODEL_REQUEST_SENT`.

Preserve existing `modelCalls`/`maxModelCalls` behavior in this PR. It continues
to be updated through successful model-response accounting. Renaming or
redefining it as a logical retry-sequence count would be a separate
configuration, quota, trace, and observability decision unrelated to making
provider retries safe.

Continue aggregating token usage only when the provider exposes it. A failed
attempt without usage must not invent token counts. The failed attempt summary
reports usage unavailable; terminal usage remains the existing observed usage
snapshot. A future cost-accounting ticket may add explicit uncertainty across
failed billable calls if provider evidence demonstrates a useful portable
contract.

Add bounded provider-attempt metrics for driver, framework model, outcome,
failure category, and retry decision. Do not tag exception messages, provider
body text, attempt IDs, raw provider codes, credentials, or model content.

### 8. Make the console useful without building another subsystem

Update Go attempt analysis so `MODEL_ATTEMPT_FAILED` is the alternate terminal
fact for an attempt and no missing-response gap is emitted for that attempt.
Validate:

- consistent retry/attempt identity;
- positive monotonic `attemptNumber`;
- valid `providerAttemptNumber` and reason;
- exactly one response-or-failure terminal fact;
- a provider retry retains its retry sequence and increments provider attempt;
- no provider retry follows permanent or exhausted failure; and
- final PR 22 error linkage resolves to the last failed attempt.

Permit `RETRY` without a subsequent attempt only when the completed trace's
terminal abort/interruption evidence explains cancellation during backoff. A
sent attempt with no response or failure remains an explicit gap.

Extend attempt summaries with:

- `attemptReason` and `providerAttemptNumber`;
- outcome `SUCCEEDED`, `FAILED`, or `INCOMPLETE`;
- failure classification/category;
- retry decision and delay/source; and
- a payload descriptor when a failed-attempt payload exists.

In Trace Explorer, show attempts chronologically using the existing attempt
view. Label initial, semantic, and provider retry attempts; show failure
category, decision, delay, and observed provider fields. Selecting a failed
attempt may open its existing logical payload viewer for bounded diagnostic
text. Do not add semantic-cycle grouping, a provider-specific UI, or another
diagnostic storage/query path.

Project a bounded live activity such as `Provider attempt 1 failed; retrying in
500 ms`. Do not include exception messages or provider bodies in live summaries.

## Java implementation map

Expected production areas:

- `autoconfigure/LoomspanProperties` and configuration validation;
- `META-INF/additional-spring-configuration-metadata.json`;
- README and `ai/skill-authoring/model-selection-and-connections.md`;
- internal Spring AI integration package and current-version implementation;
- the four connection model factories, routed through that implementation;
- one internal no-retry Spring `RetryTemplate`;
- OpenRouter compatibility-profile decoder and normalized failure type;
- `ModelAttemptCallAdvisor`, replaced by `ProviderAttemptCallAdvisor`;
- `SpringAiSkillChatClientFactory` advisor wiring;
- `ModelTraceContext`, `TraceRecordType`, trace recorder, and execution state;
- PR 22 failure-attempt linkage and diagnostic attachment seams;
- `SessionUsageService`, `SessionUsageSnapshot`, quotas, configured-limit
  snapshots, metrics, and observability DTOs; and
- live activity kinds/projector.

Keep all new Java types internal. The public-surface architecture tests must
show no new Application API, Supported SPI, public signature type, or replaceable
Spring bean.

## Go and browser implementation map

Expected production areas:

- `loomspan-console/internal/traceanalysis/enums.go`;
- `attempts.go`, attempt result construction, and gap calculation;
- `model.go`, `dto.go`, query mapping, and fixture validation;
- failure-to-final-attempt linkage;
- browser API attempt DTOs;
- `web/src/api/contracts.ts`;
- existing Trace Explorer attempt/record views; and
- live activity labels.

Do not create a browser-only classifier, OpenRouter parser, diagnostic store, or
retry state model. Future MCP consumers use the same Go attempt summaries and
payload access.

## Contract and compatibility impact

| Surface | Classification and evidence | Treatment |
| --- | --- | --- |
| Provider retry properties | New documented configuration contract. | Add strict bounded per-connection defaults. |
| OpenRouter compatibility profile | New documented OpenAI connection configuration. | Default `standard`; require explicit `openrouter`; never infer vendor. |
| `providerAttempts` and quota | New configuration/observability contract. | Count actual Loomspan-owned sends and enforce before send. |
| Existing `modelCalls` | Existing configuration/observability contract. | Preserve current behavior; do not redefine in this PR. |
| `MODEL_ATTEMPT_FAILED` and added attempt metadata | Ephemeral current-run diagnostic format consumed by Java, Go, browser, fixtures, and future MCP. | Change writer/readers atomically under release compatibility; no legacy form. |
| Spring AI adapter, retry disabling, exception translation, decoder | Internal dependency integration. | Isolate imports and replace atomically; no public SPI or compatibility shim. |
| Spring AI version support statement | Documented dependency compatibility policy. | Pin/test one primary version, list CI-tested overrides, and avoid arbitrary-version claims. |

The trace format changes in place under the exact release-derived
`consoleCompatibilityVersion`. Regenerate Java-owned fixtures and update Go and
browser consumers atomically. Do not add a schema version, legacy reader, or
dual attempt shape.

## Required tests

### Dependency isolation and version behavior

- Production imports of Spring AI provider builders, retry template types, and
  provider exceptions are confined to the internal Spring AI integration
  package and existing unavoidable ChatClient-facing seams.
- The pinned Spring AI version builds and passes the full suite.
- Every additional version claimed in release documentation runs the same
  integration suite in CI.
- No runtime reflection or version-string branching exists.
- Public-surface and Spring-extension-point tests show no new supported SPI.

### Exact attempt ownership

- Each current model builder receives a one-attempt dependency retry template.
- Mock endpoints/SDK fakes observe exactly one underlying call for one direct
  adapter invocation on retryable failure.
- A three-attempt Loomspan policy yields exactly three endpoint calls, not 30.
- An opaque-attempt adapter with retries enabled fails configuration; with
  retries disabled it performs one observable client call.
- `spring.ai.retry.*` properties do not alter Loomspan retry behavior.

### Retry policy

- Supported network timeout/reset/EOF/name-resolution failures retry.
- TLS trust/hostname failures do not retry.
- HTTP 408, 429, 500, 502, 503, and 504 retry; auth, permission, payment,
  invalid request, context limit, and policy failures do not.
- Generic deserialization and unknown exceptions do not retry.
- Cause traversal is bounded and cycle safe.
- `Retry-After` delta/date and malformed cases behave deterministically.
- Disabled and one-attempt configurations make one total send.
- Default deterministic delays are 500 ms then 1 second before injected jitter;
  arithmetic caps and cannot overflow.
- Interruption before send or during delay stops promptly, preserves interrupt
  state, and emits no phantom attempt.
- The existing mission timeout interrupts a retry wait and is never reset by
  the retry loop.

### OpenRouter

- Explicit profile preserves ordinary successful responses for Spring AI.
- A documented non-streaming response with partial content,
  `finish_reason: "error"`, and choice error becomes a normalized failure rather
  than `InvalidFormatException`.
- Retryable `error_type` retries; permanent type does not.
- Partial error content never reaches validators, tools, success state, or
  caller.
- Bounded provider JSON and observed structured fields survive in the failed
  attempt; malformed/oversized envelopes fail safely.
- Standard profile is not changed by an OpenRouter-looking URL/name/model.

### Attempt tracing and final failure

- Failure then success emits two attempts: first failed with `RETRY`, second
  successful with `PROVIDER_RETRY` and provider attempt 2.
- A later semantic advisor retry resets provider attempt to 1 and records
  `SEMANTIC_RETRY` while retaining the existing retry sequence.
- Provider retry passes the same request without mutation or partial output.
- Every known attempt has exactly one response-or-failure terminal fact.
- Recovered attempts do not emit `ERROR_RECORDED`.
- Permanent/exhausted failure produces one PR 22 error linked to the last
  attempt and closest active model frame.
- Cancellation during backoff produces no next attempt and is explained by the
  terminal trace rather than a missing-response gap.
- Original final exception propagation is not replaced by retry/trace failure.

### Quota, metrics, Go, and browser

- `providerAttempts` increments once per actual Loomspan send, including failed
  sends.
- Quota enforcement is atomic and blocks the send that would exceed it.
- Existing `modelCalls` tests remain unchanged except for explicitly documented
  coexistence with provider attempts.
- Metrics use bounded dimensions and contain no diagnostic text or secrets.
- Go accepts failed attempt lifecycle and does not label it a response gap.
- Go rejects duplicate terminals, invalid identity/numbering, and impossible
  retry transitions.
- Trace Explorer shows failed/retried attempts and opens the existing bounded
  payload view without raw NDJSON.
- Final failure and last attempt navigate bidirectionally after PR 22.
- Live activity reports retry scheduling without raw failure content.

## Acceptance signals

- The motivating OpenRouter response no longer fails merely because Spring AI
  1.1.6 lacks an `error` finish-reason enum value.
- A typed transient OpenRouter failure can recover through at most three visible
  Loomspan attempts; a permanent or exhausted failure remains precise and
  debuggable.
- Mock endpoint calls equal Loomspan attempt records and `providerAttempts`; no
  hidden retry multiplier remains for supported connections.
- Retry and trace semantics outside the internal adapter contain no Spring AI
  provider builder, retry, or exception types.
- Upgrading a supported Spring AI release line changes/tests the adapter rather
  than the retry, trace, Go, or browser contracts.
- The console explains each observed attempt without a new retry dashboard,
  diagnostic service, or provider-specific UI.
- Existing mission timeout and `modelCalls` semantics are not redesigned.

## Guardrails

- Do not promise compatibility with arbitrary Spring AI versions.
- Do not use reflection, runtime version branching, dependency-internal fields,
  or exception-message parsing to simulate version flexibility.
- Do not expose the dependency adapter or retry classifier as public API/SPI.
- Do not retry all `RestClientException`, deserialization errors, or runtime
  exceptions.
- Do not allow nested Loomspan and Spring AI application-level retries.
- Do not infer OpenRouter from endpoint or naming conventions.
- Do not mutate provider retry requests or consume partial error output.
- Do not retry tools or other side-effecting capabilities.
- Do not add a new deadline subsystem, semantic request-cycle hierarchy,
  provider-specific classifier framework, or attempt-diagnostic service.
- Do not redefine `modelCalls` or token-cost certainty in this PR.
- Do not create an `ERROR_RECORDED` for every recovered attempt.
- Do not duplicate provider diagnostics into indexes, metrics, logs, summaries,
  cursors, or live activity.
- Do not add compatibility aliases, legacy trace readers, or dual attempt forms.
- Preserve unrelated user-authored console changes and avoid unrelated cleanup.

## Verification sequence

Run focused Java adapter, retry, trace, OpenRouter mock-server, interruption,
and quota tests during implementation. Then:

```powershell
.\mvnw.cmd -pl loomspan-spring-boot-starter test -DfailIfNoTests=false
.\mvnw.cmd -pl loomspan-spring-boot-starter test -Dtest=ConsoleTraceFixtureCorpusTest -Dloomspan.console.fixtures.regenerate=true -DfailIfNoTests=false
Set-Location loomspan-console
go test ./...
go run ./internal/buildtool verify
$env:PATH = "C:\msys64\mingw64\bin;" + $env:PATH
$env:CGO_ENABLED = "1"
go test -race ./...
```

Inspect regenerated fixtures. Perform one local end-to-end run against a mock
OpenRouter endpoint that returns a documented retryable error completion then a
success, acquire the trace, and verify the visible attempt timeline and payload.

Live-provider tests are optional and not deterministic CI requirements. Use
external credentials and never log them.

## Verified references

- OpenRouter errors and debugging:
  `https://openrouter.ai/docs/api/reference/errors-and-debugging`
- Spring AI OpenAI Chat retry configuration:
  `https://docs.spring.io/spring-ai/reference/api/chat/openai-chat.html`
- Pinned local sources for Spring AI 1.1.6 modules `spring-ai-openai`,
  `spring-ai-anthropic`, `spring-ai-google-genai`, `spring-ai-ollama`, and
  `spring-ai-retry`.

## Deferred follow-ups

Create a separate ticket only when evidence justifies it:

- another Spring AI release line requiring a separate integration artifact;
- typed provider-envelope decoding beyond OpenRouter;
- deadline-aware suppression of a retry before the outer timeout fires;
- logical-model-call and uncertain failed-call cost accounting;
- streaming retry before/after partial token delivery; or
- a richer semantic-cycle visualization requested by real trace usage.

## Out of scope

- Arbitrary Spring AI version support.
- Cross-connection/model/provider fallback.
- Hedged, speculative, or parallel model calls.
- Streaming retry.
- Retrying tool execution.
- Application-provided retry predicates or public classifier SPIs.
- Broad provider schema normalization.
- Historical trace compatibility or durable retry analytics.
- New secret scanning/redaction policy beyond PR 22's agreed bounds and warning.
