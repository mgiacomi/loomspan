---
audience: loomspan-skill-builder
status: development
applies_to: current-repository-checkout
coverage: initial-source-verified
---

# Model Selection and Named AI Connections

## Applicability

Use this document when an LLM-backed YAML skill needs a model, when an application must route models to endpoints/accounts, or when diagnosing model resolution. Mapped YAML skills route directly to Java targets and MUST NOT declare model execution fields.

## Authoring model

The author-facing chain is:

`skill model` → `loomspan.models` framework alias → `loomspan.connections` connection → built-in driver → provider model ID

- An LLM-backed skill MUST set `model` to a configured framework model alias.
- A model alias MUST set `connection` and `provider-model`.
- The connection MUST exist and MUST choose exactly one built-in driver: `openai`, `anthropic`, `gemini`, or `ollama`.
- Connection names and framework model names are application-owned and case-sensitive map keys.
- Multiple connections MAY use the same driver. Use separate names when endpoints, accounts, credentials, headers, or protocol options differ.
- `default-model` has no special fallback behavior; it is an ordinary alias.

```yaml
loomspan:
  connections:
    local-primary:
      driver: ollama
      base-url: http://localhost:11434
    local-secondary:
      driver: ollama
      base-url: http://localhost:11435
  models:
    summarizer:
      connection: local-primary
      provider-model: qwen3:8b
    planner:
      connection: local-secondary
      provider-model: granite4:latest
```

The YAML skill declares `model: summarizer` or `model: planner`; it does not declare the driver or endpoint.

## Connection rules

| Driver | Required mode | Optional connection settings |
| --- | --- | --- |
| `openai` | `api-key` | `base-url`, static `headers`, `openai.organization-id`, `openai.project-id`, `openai.compatibility-profile` |
| `anthropic` | `api-key` | `base-url`, static `headers` |
| `ollama` | `base-url` | None; this driver uses Ollama's native API |
| `gemini` | Exactly one of API-key mode or Vertex AI mode | API-key mode uses `api-key`; Vertex mode uses `gemini.vertex-ai: true`, `project-id`, `location`, and optional `credentials-uri` |

Static `headers` are accepted only by the OpenAI and Anthropic drivers. Provider-specific option blocks MUST match their driver. Unknown `loomspan.*` fields are rejected at startup. Loomspan does not read, merge, or inherit `spring.ai.*` configuration.

The OpenAI and Anthropic drivers use Spring AI 2's official SDK-backed clients. A configured `base-url` is the SDK service root, not a completion-path override. OpenAI appends `/chat/completions`, so a compatible service that uses the conventional versioned route should provide a root ending in `/v1`. Anthropic appends `/v1/messages` and supplies its required version header through the official client. Put supported Anthropic beta values and other custom headers in the common `headers` map. Compatibility is a service contract, not inferred by Loomspan. The Ollama driver uses the native `/api/chat` protocol.

The removed OpenAI completion-path override and Anthropic completion-path/version/beta fields have no aliases or fallback behavior and fail strict startup binding. Move any supported Anthropic beta/custom values to `headers`; choose a complete compatible `base-url` instead of overriding a completion path.

OpenRouter error-completion handling is enabled only by `openai.compatibility-profile: openrouter`; Loomspan does not infer it from a connection name or URL. This profile rejects `finish_reason: error` responses, including any partial assistant content, and preserves bounded provider diagnostics for tracing.

Every connection owns a `provider-retry` policy. Defaults are enabled, three total unchanged-request attempts, 500 ms initial backoff, a 2.0 multiplier, a 5 s cap, and 0.2 jitter. Skills select only the model alias and cannot declare or override this policy. The session-wide `loomspan.session.quotas.max-provider-attempts` safeguard defaults to 192; zero disables that quota.

One Loomspan model interaction is one semantic request from mission, planning, or step execution. The single Spring AI tool loop may require multiple model turns inside that interaction. A semantic validator retry starts another semantic attempt; a provider retry repeats one unchanged model turn. Every actual downstream send is one physical provider attempt, is counted against quota once, and is owned by Loomspan because provider-native retries are disabled.

Credentials SHOULD come from environment placeholders or an external secret source and MUST NOT be committed. Connection diagnostics, traces, and metrics identify framework model, connection, and driver; they do not expose API keys, header values, base URLs, or credential contents.

## Thinking levels

A model alias MAY declare `thinking-levels`. A skill's `thinking_level` MUST be one of those configured values. If the alias has no configured levels, the skill MUST omit `thinking_level`. Thinking options are adapted to the selected driver at request time; the connection remains reusable across model aliases.

## Migration and diagnosis

1. Create a named entry under `loomspan.connections` for each endpoint/account.
2. Move base URLs, credentials, headers, and provider-specific protocol settings from `spring.ai.*` into that connection.
3. Replace each `loomspan.models.<name>.provider` with `connection` and keep `provider-model`.
4. If two aliases use different endpoints or accounts for the same driver, point them at different named connections.

Startup errors include the complete property path for missing, unknown, or driver-inapplicable settings. Runtime resolution errors identify the skill, framework model, connection, driver, and provider model. Start diagnosis at the named connection, then verify its driver-specific requirements and the alias reference.

## Source verification anchors

- Configuration and validation: `LoomspanProperties`, `LoomspanPropertiesTest`.
- Skill model resolution: `YamlSkillCatalog`, `EffectiveSkillExecutionConfiguration`.
- Connection construction and lookup: `NamedAiConnectionRegistry`, `SpringAiProviderIntegration`, `DefaultSkillChatModelResolver`.
- Retry/profile ownership: `LoomspanPropertiesTest`, `ProviderRetryDeciderTest`,
  `SpringAiProviderIntegrationTest`, `ConnectionProtocolTest`, and
  `ModelAttemptCallAdvisorIntegrationTest`.
- Request options and tool-loop assembly: `SpringAiChatOptionsContributorTest` and `SpringAiChatClientAssemblerIntegrationTest`.
- Wire behavior: `ConnectionProtocolTest`.
- Operational identity: `ModelExecutionIdentity`, `ExecutionTraceContractTest`, `MicrometerUsageMetricsRecorderTest`.

This topic does not fully specify provider SDK behavior or third-party compatibility guarantees. Inspect the pinned Spring AI version and the target service's protocol documentation when those details determine correctness.
