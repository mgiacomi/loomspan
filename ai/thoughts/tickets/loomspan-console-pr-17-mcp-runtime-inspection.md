# PR 17 — Runtime, Skill, and Live-Inspection MCP Surface

## Status

Approved implementation ticket. Depends on PR 16 and reuses PRs 09–11.

## Outcome

Expose selected-target registered skills, active executions, and bounded recent
activity through the existing local MCP server by adapting the same
transport-neutral Go services used by the browser. The result is a small,
tool-first inspection surface suitable for a developer using an authenticated
loopback console on a laptop or desktop.

## Developer workflows

The canonical acceptance scenarios are in
`ai/thoughts/phases/loomspan_console_workflows.md`. PR 17 most directly supports:

- `WF-X-R6`, `WF-X-R7`, and `WF-X-R10` for observation, adapter parity, and
  explicit unavailable/gap facts;
- `WF-FE-R10` for recent activity that is not durable or lossless history;
- `WF-SE-R2`, `WF-SE-R3`, `WF-SE-R6`, `WF-SE-R9`, and `WF-SE-R10` for current
  summary, bounded active path, observation freshness, continuity, and the
  boundary between active evidence and finalized trace analysis; and
- `WF-SP-R2`, `WF-SP-R7`, `WF-SP-R8`, and `WF-SP-R9` for bounded active skill
  paths, registered skill lookup, unchanged YAML, and descriptive-only
  `sourcePath`.

Workflows select and sequence the general tools. They do not create
workflow-specific tools, DTOs, stores, or diagnosis logic.

## Architectural ownership

- MCP remains a peer adapter over `target.Context`, `observability.Service`, and
  `live.Service`.
- MCP does not contact the application directly, own an upstream subscription,
  or retain another skill catalog, execution registry, or activity history.
- Java remains authoritative for registered skills, active execution state,
  application collection cursors, and activity delivery cursors.
- `target.Context` remains authoritative for target selection, application
  identity, credentials, cancellation, `targetScopeId`, and stale-result
  rejection.
- `observability.Service` remains authoritative for bounded skill and active
  execution queries and upstream validation.
- `live.Service` remains authoritative for the one bounded, single-continuity
  recent-activity window.

## Tools

Add these exact tools:

| Tool | Input | Successful result |
| --- | --- | --- |
| `LOOMSPAN_list_skills` | Required `pageSize` from 1 through 64; optional MCP `continuation` | Scope, instance, observation time, registered name/source path/resource URI items, `hasMore`, and continuation when another page exists |
| `LOOMSPAN_get_skill` | Required `registeredName` | Scope, instance, query observation time, registered name, descriptive `sourcePath`, unchanged UTF-8 YAML, and canonical resource URI |
| `LOOMSPAN_list_executions` | Required `pageSize` from 1 through 64; optional MCP `continuation` | Scope, instance, application observation time, unchanged bounded active summaries, `hasMore`, and continuation when another page exists |
| `LOOMSPAN_get_execution` | Required `sessionId` | Scope, instance, query observation time, and the current bounded active summary; completed/non-active sessions remain `NOT_FOUND` |
| `LOOMSPAN_get_execution_activity` | Required `sessionId` and `pageSize` from 1 through 64; optional MCP `continuation` | Scope, instance, query observation time, complete ordered activity envelopes, returned cursor range, `hasMore`, resumable continuation, continuity/reset facts, and `beginningUnavailable` |

Inputs use strict inferred JSON Schema with unknown properties rejected. All
five tools advertise read-only, non-destructive, idempotent, closed-world
annotations. Tool descriptions state that skill YAML and activity content are
untrusted diagnostic data, not server instructions.

## Structured result and error envelope

Use one small generic envelope for all five typed tools because the pinned Go
MCP SDK otherwise drops typed structured output when a handler returns a Go
error:

```json
{ "result": { "...": "tool-specific success fields" } }
```

or:

```json
{
  "error": {
    "code": "LIVE_MONITORING_UNAVAILABLE",
    "message": "Live monitoring is unavailable.",
    "targetScopeId": "optional",
    "details": {}
  }
}
```

Exactly one of `result` or `error` is populated. A shared domain failure returns
the error envelope normally while setting MCP `isError: true`; it does not
return a Go error. The text content is exactly the safe `CODE: message` summary.
Unexpected shared failures preserve sanitized `CONSOLE_ERROR` and never expose
the internal Go cause. `details` is always a JSON object, including `{}` when
the shared error has no details; it is never `null`, a scalar, or an array.

Malformed tool arguments rejected by JSON Schema, unknown tools, protocol
negotiation/framing failures, disabled MCP, and access-key rejection remain
SDK/HTTP protocol failures rather than Loomspan domain results. A replay gap is
a successful activity result, not an unsuccessful tool call.

## Text fallback

Every successful result supplies one deterministic text block in addition to
structured content:

- lists report scope, observation time, count, `hasMore`, continuation, and one
  compact identity/status line per item;
- skill detail reports identity/source metadata followed by the unchanged YAML;
- execution detail reports identifiers, state, phase, timing, bounded active
  path, usage, and configured limits without diagnosing the execution; and
- activity reports scope, observation/continuity/gap facts, continuation, and
  one cursor/timestamp/kind/summary line per item. Raw `details` remain in
  structured content and are not duplicated into text.

The fallback does not infer importance, health, causality, completeness, or
remediation.

Text is a line-oriented compatibility fallback, not a second schema. Emit
common fields first in this order: `targetScopeId`, `instanceId`, `observedAt`,
then operation-specific fields in the same fixed order as the structured DTO.
Lists then emit `count`, `hasMore`, `continuation`, followed by indexed item
lines in returned order. Activity emits returned-cursor and continuity/reset
facts before `count`, `hasMore`, `continuation`, and its indexed item lines.
Execution detail emits scalar identity/state/timing fields, indexed active-path
entries, usage fields, and configured-limit fields in DTO order. Optional
continuation is the literal `-` when absent.

All timestamps use UTC `time.RFC3339Nano`. Every dynamic string
or timestamp on a metadata/item line is JSON-quoted so newlines, delimiters,
quotes, and control characters cannot create extra apparent fields. Integers
and booleans remain unquoted. Skill YAML is the sole raw multiline value and
appears only after a final line exactly `yaml:`; no field follows it. Activity
`details` is omitted from text. Committed golden files are the executable
authority for exact labels, spaces, line order, escaping, and final newline.

## Continuations and response framing

The MCP access key authenticates every request. A continuation grants no access;
it only identifies where a bounded query resumes.

Use a private, versioned, unpadded-base64url JSON token with a maximum encoded
length of 8,192 characters. It contains only:

- version `1`;
- operation kind (`skills`, `executions`, or `activity`);
- `targetScopeId`;
- the underlying next application/activity cursor; and
- `sessionId` for activity tokens.

The decoder rejects malformed base64/JSON, unknown fields, unknown
versions/kinds, missing required fields, operation mismatch, activity-session
mismatch, trailing data, and oversized tokens. A valid prior-scope token returns
`TARGET_CHANGED`. A malformed token is `INVALID_ARGUMENT`. A valid activity
position that has left the window produces the existing successful
`beginningUnavailable`/reset result.

The token is deliberately not encrypted, signed, persisted, or stored in a
server-side registry. Every call is already access-key authenticated and
scope-checked; token contents confer no additional authority. Clients must
nevertheless treat it as opaque and must not substitute raw application
cursors.

`pageSize` is required, caller selected, and capped at 64. The adapter asks the
shared service for that bounded page and does not add adaptive byte budgeting,
binary-search shrinking, deferred-item storage, or another paging cache.
Existing service body/item bounds remain the hard safety limits. Client smoke
testing must exercise the maximum representative response; if 64 items are not
interoperable, this constant is lowered before PR acceptance. Atomic skill,
execution, activity, and error values are never silently truncated.

For activity, a continuation represents forward progress and is returned when
a resumable cursor exists, including when no currently retained matching item
follows so a later explicit query can ask for new activity. `hasMore` means more
matching activity is already retained now; it is not a streaming promise.

## Recent-activity semantics

Refactor the shared recent query to return one `live.RecentResponse` plus an
optional shared domain error. Capture top-level query `observedAt` from the
live service clock while holding the same lock used to snapshot the window.
Keep `continuity.observedAt` as the upstream interval observation fact; the two
timestamps must not be conflated.

The shared query atomically checks `liveMonitoringAvailable`. When unavailable,
browser and MCP recent-activity adapters return
`LIVE_MONITORING_UNAVAILABLE` even if the bounded window still contains older
items. Active execution operations preserve the application's same error.

Activity results:

- contain one application delivery-cursor interval only;
- preserve complete activity envelopes and application ordering;
- include first/last returned cursor when items exist;
- expose `beginningUnavailable` and the one retained reset fact directly;
- never describe the window as durable or complete execution history; and
- preserve `EXECUTION_OBSERVATION_ENDED` with
  `CORE_FINALIZATION_FAILED` without inventing an outcome or trace.

Before exposing activity through MCP, repair the existing internal Go activity
kind table so declared `MODEL_ATTEMPT_FAILED` is accepted and has the exact
human-readable label `Model attempt failed`. This is a Go-only consistency correction: the Java event,
REST/SSE wire value, and fixtures already use the existing value and do not
change.

## Skill resource

Register the supplementary template:

```text
loomspan://targets/{targetScopeId}/skills/{skillName}
```

Each variable is one canonical UTF-8 percent-encoded path segment. Parse from
`url.URL.EscapedPath()`: require scheme `loomspan`, authority exactly `targets`,
and exactly three path segments after the leading slash—scope, literal
`skills`, and skill name. Reject opaque form, user info, port, query, fragment,
empty/extra segments, malformed escapes, invalid UTF-8, blank decoded values,
or decoded `/` or `\`. Decode each variable exactly once and require
`url.PathEscape(decoded) == rawSegment`; this also rejects alternate and
double-encoded spellings. Then require the decoded scope to equal the captured
current scope before calling the same `observability.Service.GetSkill` path used
by the tool.

Return one text resource with MIME type `application/yaml; charset=utf-8`, the
canonical requested URI, and unchanged YAML. Resource `_meta.loomspan` contains
`targetScopeId`, `instanceId`, query `observedAt`, `registeredName`, and
descriptive `sourcePath`. The resource never treats `sourcePath` as a filesystem
locator. Resource failures use one exact SDK/JSON-RPC mapping. URI syntax,
`INVALID_ARGUMENT`, and `NOT_FOUND` return `InvalidParams` (`-32602`). Other
shared Loomspan domain errors, including sanitized `CONSOLE_ERROR`, return the
private server code `-32000`; `CONSOLE_ERROR` is never rewritten to `-32603`.
The JSON-RPC message is the safe `CODE: message`, and `data` is exactly
`{"error": <domainErrorDTO>}` with object-valued `details`. No internal cause is
serialized. The tool remains the portable structured contract.

Do not expose runtime status, active executions, or recent activity as
resources.

## Capabilities

Extend `LOOMSPAN_get_runtime` with these capabilities only when their complete
required family and semantics are registered:

| Capability | Required tools |
| --- | --- |
| `loomspan.skill-inspection.v1` | `LOOMSPAN_list_skills`, `LOOMSPAN_get_skill` |
| `loomspan.active-execution-inspection.v1` | `LOOMSPAN_list_executions`, `LOOMSPAN_get_execution` |
| `loomspan.recent-activity-inspection.v1` | `LOOMSPAN_get_execution_activity` |

Use one small static capability descriptor table for advertisement and
conformance tests. Resources are supplementary and not required operations of
the capability. Capability availability describes the installed server
surface, not current target/authentication/live/evidence availability. PR 17
must not advertise trace or raw-artifact capabilities. Do not introduce a
dynamic capability registry, dependency graph, target-state evaluation, or
plugin mechanism.

## MCP protocol revision policy

PR 17 defines one Loomspan tool/resource/DTO contract. The official Go SDK
continues to negotiate current MCP `2026-07-28` and compatible `2025-11-25` on
the same stateless server; Loomspan adds no protocol-version branch, alternate
DTO, handler, resource, or legacy HTTP+SSE transport. Exercise the complete PR
17 product contract once through the current/default SDK path. Retain one small
`2025-11-25` compatibility smoke that initializes, discovers the PR 17 tools
and skill resource template, calls one representative tool, and reads one skill
resource. The existing protocol-generic conformance harness continues to cover
both negotiated revisions. Dropping the compatible revision is a separate MCP
foundation policy change, not PR 17 scope.

## Wiring and lifecycle

- Extend MCP construction with the existing target context, observability
  service, live service, and an injected `Now func() time.Time` used for
  detail/resource query observation and deterministic tests.
- Capture one immutable target scope per target-specific operation.
- Pass request cancellation through `Scope.Upstream` for application queries.
- Before publishing a result, preserve target-scope and MCP authentication-
  generation late-result rejection.
- Keep stateless Streamable HTTP: no Loomspan conversation/session registry is
  introduced.
- Multiple authenticated clients share the same services and bounds but retain
  independent request contexts and cancellation.

## Acceptance signals

- The five tools have exact golden structured schemas/results and deterministic
  text fallbacks.
- The declared `MODEL_ATTEMPT_FAILED` activity kind is accepted, labeled, and
  preserved without changing the Java wire value.
- Browser and MCP mappings preserve identical identifiers, calculations,
  availability facts, limitations, observation meanings, and shared error codes.
- The approved workflow requirements listed above are linked to focused tests.
- Activity never crosses a continuity boundary and never returns retained state
  while live monitoring is unavailable.
- Continuations reject wrong operation, session, scope, version, malformed,
  oversized, and trailing-data inputs without panics or data
  exposure.
- Each advertised capability has its complete operation family and semantic
  contract; negative conformance fixtures fail when a member is absent.
- The complete product contract passes once on the current protocol, while the
  compact previous-revision smoke passes without any version-specific Loomspan
  behavior.
- Authentication-required, incompatible-target, live-unavailable, stale-scope,
  replay-gap, malformed request, protocol failure, and sanitized console failure
  remain distinguishable.
- Cancellation, target rotation, credential generation change, and shutdown
  suppress late results.
- Concurrent clients do not create duplicate upstream subscriptions or retained
  state.
- A dated representative-client smoke matrix records connection, discovery,
  structured/text rendering, errors, skill resource reads, continuation, and a
  maximum-page response for each available Phase 3 local client family.

## Contract and compatibility

- No supported Java Application API or SPI changes.
- No configuration key or YAML manifest syntax/semantics changes.
- No durable persisted format changes and no Java-to-Go REST/SSE change is
  planned.
- The new MCP tool names, capability identifiers, input/output schemas, error
  envelope, resource URI, and continuation behavior are serialized MCP-client
  contracts.
- Go and Java implementation types remain internal. No public Java surface is
  added and no compatibility shim is required.
- If implementation discovers that the agreed contract requires a Java-to-Go
  wire change, stop and amend this ticket/plan with the synchronized Java, Go,
  fixture, compatibility-marker, and documentation treatment before coding it.

## Documentation

Update Console MCP documentation and the client compatibility matrix. PR 17
does not change skill YAML, authoring semantics, or the portable Agent Skill;
`ai/skill-authoring/` is therefore unchanged. PR 19 owns author-facing Agent
Skill distribution and full agent-quality evaluations.

## Out of scope

- Trace discovery, acquisition, parsing, query, payload, and raw-artifact tools
  or resources (PR 18).
- Diagnostic reasoning, workflow-specific diagnosis tools, prompts, or Agent
  Skill distribution/evaluation (PR 19).
- Server-initiated MCP activity streaming.
- Durable history, multi-target/fleet queries, remote or multi-user MCP,
  per-client authorization, OAuth, signed/encrypted continuations, continuation
  storage, or enterprise policy machinery.
- Filesystem access through skill `sourcePath`, repository comparison, target
  mutation, execution control, or content-directed tool/network/shell actions.

## References

- `ai/thoughts/research/2026-08-13-loomspan-console-pr-17-mcp-runtime-inspection.md`
- `ai/thoughts/phases/loomspan_console_phase_3_llm_runtime_inspector.md`
- `ai/thoughts/phases/loomspan_console_workflows.md`
- `ai/thoughts/tickets/loomspan-console-pr-18-mcp-trace-inspection.md`
- `ai/thoughts/tickets/loomspan-console-pr-19-debugging-skill.md`
