# Loomspan

A Java 21, Spring Boot 4.1, and Spring AI 2–based agentic framework that uses LLM‑driven skills within a Hierarchical Task Network (HTN) architecture.

Loomspan while still an HTN is fundamentally different from traditional HTNs. Instead of relying on rigid, rule‑based planners, Loomspan blends classical HTN structure with LLM‑powered reasoning, allowing agents to dynamically decompose missions, select skills, and orchestrate complex workflows. 

At its core, Loomspan treats skills as the fundamental building blocks of capability. YAML manifests define every public skill: an LLM-backed skill can reason and call other visible YAML skills, while a mapped YAML skill exposes deterministic application logic implemented by a Java `@SkillMethod`. This creates a flexible planning system that combines LLM reasoning with explicit contracts and ordinary Spring services.


## Why Loomspan?
Most HTN planners (like JSHOP2 or PANDA) rely on static, hand‑coded methods. They’re powerful, but brittle. Loomspan takes a different approach:
- LLM‑driven decomposition
The agent decides how to break down a mission in real time.
- Skill‑based execution
Each skill is a modular, reusable capability that can call others.
- Natural‑language domain modeling
No DSLs or planning languages — skills are written in plain English.
- Spring Boot foundation
Easy integration, dependency injection, configuration, and deployment.
The result is a hybrid system that combines the structure of HTNs with the adaptability of modern LLMs.


## Requirements

- Java 21 or newer
- Maven 3.9 or newer (the included Maven wrapper is recommended)
- At least one named Loomspan AI connection using the Ollama, OpenAI, Anthropic, or Gemini driver

## Project Structure

Loomspan currently contains three projects:

- `loomspan-spring-boot-starter`: the core starter.
- `loomspan-sample`: a sample Spring Boot application.
- `loomspan-console`: an independent Go module with an embedded React application. Its explicit build uses pinned Go, Node.js, and npm toolchains and is not part of the Maven reactor.

Ordinary Java development and `mvn test` do not invoke Console tooling. See
[`loomspan-console/README.md`](loomspan-console/README.md) for the Console build
and local hot-reload workflow.

## Getting Started

Add the starter to your application:

```xml
<dependency>
    <groupId>com.lokiscale.loomspan</groupId>
    <artifactId>loomspan-spring-boot-starter</artifactId>
    <version>0.1.0-SNAPSHOT</version>
</dependency>
```

Configure application-owned AI connections, skill locations, and named Loomspan model aliases in `application.yml`:

```yaml
server:
  port: 8081

logging:
  level:
    com.lokiscale.loomspan.sample: INFO

loomspan:
  connections:
    ollama-main:
      driver: ollama
      base-url: ${OLLAMA_BASE_URL:http://localhost:11434}
    openai-main:
      driver: openai
      api-key: ${OPENAI_API_KEY}
  session:
    mission-timeout: 6000s
  skills:
    locations:
      - classpath:/skills/**/*.yml
      - classpath:/skills/**/*.yaml
  models:
    granite4-tiny:
      connection: ollama-main
      provider-model: ibm/granite4:tiny-h
    default-model:
      connection: ollama-main
      provider-model: ibm/granite4:tiny-h

execution-trace:
  persistence: ALWAYS
```

Every LLM-backed YAML skill must name one of the entries under `loomspan.models`. Mapped YAML skills do not declare a model. `default-model` is an ordinary model key; it is not selected automatically.

A connection is a concrete endpoint/account and chooses a built-in `driver`; a model is a framework alias that chooses a connection and the request-level `provider-model`. Multiple connections may use the same driver. Loomspan does not merge or inherit `spring.ai.*` settings. Keep credentials in environment variables or an external secret store.

Provider retries are owned by each application connection, not by YAML skills. The default is three total attempts with 500 ms initial backoff, a 2.0 multiplier, a 5 s cap, and 0.2 jitter. Set `provider-retry.enabled: false` (or `max-attempts: 1`) for one attempt. Loomspan disables the supported Spring AI clients' own application-level retries so these limits describe actual downstream calls.

The `openai` and `anthropic` drivers use Spring AI 2's official SDK-backed clients. Their optional `base-url` is the SDK service root: OpenAI appends `/chat/completions`, so include `/v1` in the root when the service requires it; Anthropic appends `/v1/messages`. Both accept common static `headers`; use that map for Anthropic beta or other supported custom headers. OpenAI additionally supports organization/project IDs and the explicit OpenRouter compatibility profile. The former OpenAI completion-path override and Anthropic completion-path/version/beta fields are rejected rather than aliased. The `ollama` driver uses its native `/api/chat` protocol. Gemini supports either API-key mode or Vertex AI mode (`project-id` and `location`, with optional credentials resource), but not both on one connection.

Several model aliases can share one connection while choosing different provider model IDs. An OpenAI-compatible gateway is another named connection using `driver: openai`; it does not need a vendor-specific driver:

```yaml
loomspan:
  connections:
    openrouter:
      driver: openai
      base-url: https://openrouter.ai/api/v1
      api-key: ${OPENROUTER_API_KEY}
      headers:
        HTTP-Referer: ${OPENROUTER_SITE_URL}
      openai:
        compatibility-profile: openrouter
      provider-retry:
        enabled: true
        max-attempts: 3
        initial-backoff: 500ms
        multiplier: 2.0
        max-backoff: 5s
        jitter: 0.2
  models:
    fast:
      connection: openai-main
      provider-model: gpt-4o-mini
    deep:
      connection: openai-main
      provider-model: gpt-5
    routed-sonnet:
      connection: openrouter
      provider-model: anthropic/claude-sonnet-4
```

Endpoint compatibility is feature-specific: verify tools, media, structured output, reasoning fields, and usage reporting against the selected service.

By default, Loomspan discovers `classpath:/skills/**/*.yaml`. Add the `.yml` pattern, as above, when your application uses that extension.

### Invoking a skill

Inject `SkillTemplate` and invoke a YAML skill with a map (or an object that can be converted to a map). The result is returned as text; use an `output_schema` when the caller needs a predictable JSON shape.

```java
import com.lokiscale.loomspan.api.SkillTemplate;
import org.springframework.stereotype.Service;

import java.util.Map;

@Service
public class InvoiceWorkflow {
    private final SkillTemplate skills;

    public InvoiceWorkflow(SkillTemplate skills) {
        this.skills = skills;
    }

    public String checkInvoice(String invoiceText) {
        return skills.invoke("duplicateInvoiceChecker", Map.of("payload", invoiceText));
    }
}
```

The supported starter Java API is closed to these eight types in `com.lokiscale.loomspan.api`: `SkillTemplate`, `SkillExecutionView`, `SkillExecutionEvent`, `SkillMethod`, `SkillParam`, `SkillException`, `SkillInputValidationException`, and `SkillInputValidationIssue`. A Java `public` modifier does not add a type to this API: everything under `com.lokiscale.loomspan.internal` is implementation detail and may change without a compatibility shim, while `com.lokiscale.loomspan.autoconfigure` contains Spring-facing integration and configuration-binding machinery rather than an application extension API. Documented configuration keys and behavior remain user-facing contracts. `SkillTemplate` is injectable and easy to mock in application tests, but replacing its framework bean or implementing Loomspan internals is unsupported. There are currently no supported Loomspan-specific SPIs or bean overrides.

For integration testing, configure a real or local protocol-compatible named connection and invoke the YAML skill through `SkillTemplate`. Loomspan's supported-surface integration test follows this pattern: it supplies a local OpenAI-compatible endpoint through `loomspan.connections`, invokes an LLM-backed YAML skill that calls a mapped `@SkillMethod`/`@SkillParam` leaf, and observes only `SkillExecutionView` values. Tests should not replace internal resolvers, coordinators, model factories, registries, or virtual-file-system beans.

Successful observers receive a session ID and immutable, current-version `SkillExecutionEvent` values. These events are intended for trusted development and debugging, may contain application business data, and are not a durable or comprehensively sanitized trace contract. Invalid caller input raises `SkillInputValidationException`, authorization failures remain Spring Security `AccessDeniedException`, and other runtime failures crossing the facade become a safe `SkillException`.

## Defining Skills

### YAML skills

An LLM-backed YAML skill omits `mapping`, declares a configured `model`, and may use model execution settings. `prompt` supplies private instructions in addition to the public `description`.

The YAML `name` is the skill's single public identity and must match `^[A-Za-z_][A-Za-z0-9_]{0,63}$`: use 1-64 characters, start with an ASCII letter or underscore, and then use only ASCII letters, digits, or underscores. Names are case-sensitive and Loomspan does not trim, sanitize, normalize, truncate, or alias them. Descriptive lowerCamelCase names such as `duplicateInvoiceChecker` and `expenseLookup` are the recommended authoring style, though underscores and uppercase starts are also valid. Use the exact YAML name in `SkillTemplate`, `allowed_skills`, and property-level `evidence` expressions.

This public-name rule does not apply to `mapping.target_id`. That field is internal mapping metadata and intentionally uses separate `beanName#methodName` syntax.

```yaml
name: duplicateInvoiceChecker
description: >
  Checks whether a given invoice already exists in the expense system.
  First, parses the raw invoice text to extract vendor, amount, and date.
  Then, retrieves existing expenses and compares them to determine
  if the invoice is a duplicate.
model: granite4-tiny
planning_mode: true
max_steps: 10
allowed_skills: [invoiceParser, expenseLookup]
output_schema:
  type: object
  properties:
    isDuplicate:
      type: boolean
      evidence: invoiceParser and expenseLookup
      description: True if a matching expense was found in the system
    vendorName:
      type: string
      evidence: invoiceParser
      description: Vendor name extracted from the invoice
    totalAmount:
      type: number
      evidence: invoiceParser
      description: Total amount extracted from the invoice
    invoiceDate:
      type: string
      evidence: invoiceParser
      description: Invoice date in ISO-8601 format (YYYY-MM-DD)
    reasoning:
      type: string
      evidence: invoiceParser and expenseLookup
      description: Brief explanation of why the invoice was or was not considered a duplicate
  required: [isDuplicate, vendorName, totalAmount, invoiceDate, reasoning]
  additionalProperties: false
output_schema_max_retries: 2
```

Important execution settings:

- `planning_mode`: enables the step-based HTN executor only when set to `true`. It is disabled by default.
- `allowed_skills`: limits the YAML skills visible to a planning skill. Use public YAML manifest names only; Java target IDs are internal mapping metadata.
- `max_steps`: bounds planning-loop steps.
- `prompt`: optional private instructions for an LLM-backed skill.
- `thinking_level`: selects a configured thinking level for models that support it.
- `input_schema`: validates and describes the expected input. Supported types are `object`, `array`, `string`, `number`, `integer`, `boolean`, and `attachment`.
- `output_schema`: validates the model response. When present, `output_schema_max_retries` defaults to `2` and accepts values from `0` through `3`.
- `linter`: currently supports a `regex` linter with `max_retries` from `0` through `3`.
- `output_schema.properties.<name>.evidence`: attaches a nonblank Boolean expression over exact direct `allowed_skills` names to an immediate root output property. Operators `and` and `or` are case-insensitive; skill names remain case-sensitive; `and` binds more tightly than `or`, and parentheses override precedence. Plan validation checks every annotated property against planned child names; final validation checks only annotated properties present in the candidate against successfully completed direct children. Nested child internals do not leak upward. The annotation is orchestration metadata, not candidate JSON, and enforces supportability rather than factual truth or workflow order. Nested-schema annotations are unsupported.
- `rbac_roles`: requires the current Spring Security authentication to have one of the listed roles before the skill is visible or executable.

For attachment inputs, declare `type: attachment`, a `media_type` (`image`, `pdf`, `audio`, `video`, or `file`), and permitted `allowed_content_types`. Pass a Spring `Resource` or a `ref://...` virtual-file reference as the input value.

### Mapping a YAML skill to Java

YAML manifest `name` is the only public Loomspan skill identity. Use `mapping.target_id` to connect that public YAML skill to an internal Java implementation target identified by `beanName#methodName`. A mapped wrapper must declare `name`, `description`, and a nonblank `mapping.target_id`; its only optional field is `rbac_roles`.

Declaring `mapping`, even as `null` or an empty block, selects mapped validation and requires a nonblank target. Mapped input and output behavior is owned by the Java target: Loomspan publishes its reflected input contract, and a different public shape requires a separate Java adapter target. Model/runtime fields such as `model`, `prompt`, schemas, planning, tool allowlists, linting, retries, and evidence annotations are rejected on the mapped child. An LLM parent may still list the child in its own `allowed_skills` and property-level `evidence` expressions.

The public YAML name may equal the Java method name because public skills and implementation targets use separate namespaces. Multiple mapped YAML skills may also share one Java target. Within a single Spring bean, however, annotated method names must be unique: overloaded `@SkillMethod`s would produce the same `beanName#methodName` target ID and fail startup.

```yaml
name: expenseLookup
description: Retrieves the most recent expenses.
mapping:
  target_id: expenseService#getLatestExpenses
```

### Java `@SkillMethod` implementation targets

Use `@SkillMethod` when the implementation should run deterministic Java logic. It registers an internal target, not a public capability or alias. Expose it through a mapped YAML manifest before application code can invoke it or another YAML skill can list it in `allowed_skills`; both surfaces accept YAML names only.

```java
import com.lokiscale.loomspan.api.SkillMethod;
import com.lokiscale.loomspan.api.SkillParam;
import org.springframework.stereotype.Service;

import java.util.List;
import java.util.Map;

@Service
public class ExpenseService {

    @SkillMethod(description = "Returns a fake list of recent expenses.")
    public List<Map<String, Object>> getLatestExpenses(
            @SkillParam(description = "Optional category filter.", required = false) String category) {
        return List.of(
            Map.of("category", "Software", "amount", 120.00, "date", "2026-03-20")
        );
    }
}
```

## Operations and limits

`loomspan.session` provides execution safeguards. Defaults are a 60-second mission timeout, maximum depth 32, 64 skill invocations, 128 tool invocations, 32 linter retries, 64 model calls, 192 physical provider attempts, and 200,000 usage units. Attachments default to a 20 MB maximum size.

```yaml
loomspan:
  session:
    mission-timeout: 60s
    max-depth: 32
    attachments:
      max-size: 20MB
    quotas:
      max-skill-invocations: 64
      max-tool-invocations: 128
      max-linter-retries: 32
      max-model-calls: 64
      max-provider-attempts: 192
      max-usage-units: 200000

execution-trace:
  persistence: ONERROR # NEVER, ONERROR, or ALWAYS
```

When Micrometer is on the application classpath, Loomspan records usage metrics automatically. Execution traces and the `SkillTemplate` observer callback can be used to inspect a completed skill execution.

### Opt-in Console observability REST API

Servlet applications can expose the read-only operator API under
`/_loomspan/observability/v1/**`. It is disabled by default and is not exposed
through Actuator or CORS. Use HTTPS whenever the listener is reachable beyond a
trusted local boundary, generate at least 32 random bytes, encode them as
unpadded base64url, and keep the resulting key in external configuration:

```yaml
loomspan:
  observability:
    enabled: true
    auth:
      api-key: ${LOOMSPAN_OBSERVABILITY_API_KEY}
    completion-grace-ttl: 15m
    trace-catalog-metadata-ttl: 24h
```

The key must be 32–512 printable, non-whitespace ASCII characters and is loaded
at startup; rotate it by restarting the application. Every request must present
exactly one `X-loomspan-Api-Key` header. Authenticated responses use
`Cache-Control: no-store` and identify the current process with
`X-loomspan-Instance-Id`.

The current routes are `instance`, `skills`, `skills/{registeredName}`,
`active-executions`, `active-executions/{sessionId}`, `activity`, `traces`,
`traces/{traceId}`, and `traces/{traceId}/artifact` beneath the API root. The
artifact route accepts only GET with no query, range, or conditional headers
and an absent, wildcard, or NDJSON-compatible `Accept` header:

```bash
curl -H "X-loomspan-Api-Key: $LOOMSPAN_OBSERVABILITY_API_KEY" \
  -H "Accept: application/x-ndjson" \
  -OJ "http://localhost:8081/_loomspan/observability/v1/traces/$TRACE_ID/artifact"
```

A successful download is
`application/x-ndjson; charset=utf-8`, has the exact cataloged
`Content-Length`, and uses a standards-encoded attachment disposition whose
`filename`/`filename*` values represent
`loomspan-trace-<traceId>.ndjson`. Clients should use the decoded attachment
filename rather than comparing the serialized header text.
The process admits eight downloads independently from the 16 SSE subscriptions;
a ninth receives `429/LIMIT_EXCEEDED` without queuing. Transfers time out after
five minutes. A transfer admitted before expiration may finish, but new
requests for unknown, expired, deleted, or raced resources receive
`404/NOT_FOUND`.

The body is the exact finalized diagnostic file: Loomspan does not parse,
rewrite, normalize, redact, compress, or buffer it in full. Authenticated traces
may contain application business data and paths already recorded by canonical
diagnostics. Ordinary DTOs, lookup identifiers, response headers, and safe
download filenames never expose or derive from the internal artifact path.

Applications using Spring Security must let the reserved namespace reach
loomspan's filter while retaining their normal rules elsewhere. Loomspan does not
create or reorder the application's `SecurityFilterChain`:

```java
@Bean
SecurityFilterChain applicationSecurity(HttpSecurity http) throws Exception {
    return http.authorizeHttpRequests(requests -> requests
            .requestMatchers("/_loomspan/observability/v1/**").permitAll()
            .anyRequest().authenticated())
        .build();
}
```

The `permitAll` rule does not make the API unauthenticated; the Loomspan key is
still mandatory. A generic proxy or host-security `401`/`403` occurs before the
adapter and is distinct from the adapter's `LOOMSPAN_API_KEY_REJECTED` problem.
The normal servlet context path applies to every route. If startup detects an
invalid configuration or an overlapping application mapping, it logs a
sanitized diagnostic and leaves the entire optional adapter, observation, and
completion grace behavior disabled.

TLS and any host/proxy rejection remain application infrastructure
responsibilities. A server-to-server Console client on the same listener needs
no CORS configuration. Rotating the key prevents later requests but does not
retroactively revoke a transfer that was already authenticated and admitted.

## Running The Sample

The OpenAI-backed feedstock examples read the API key from `OPENAI_API_KEY`; keep the value in your environment rather than committing it to configuration.

On Windows PowerShell:

```powershell
$env:OPENAI_API_KEY = "sk-..."
```

To persist it for your Windows user account:

```powershell
setx OPENAI_API_KEY "sk-..."
```

From the repository root:

```bash
./mvnw -pl loomspan-sample spring-boot:run
```

On Windows PowerShell:

```powershell
.\mvnw.cmd -pl loomspan-sample spring-boot:run
```

The sample app loads skills from `classpath:/skills/**/*.yml` and `classpath:/skills/**/*.yaml` and configures named Ollama and OpenAI connections in [application.yml](/C:/opendev/code/loomspan/loomspan-sample/src/main/resources/application.yml).
