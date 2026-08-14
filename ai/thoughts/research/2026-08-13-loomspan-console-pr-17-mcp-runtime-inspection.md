---
date: 2026-08-13T18:23:17-07:00
researcher: Codex (GPT-5)
git_commit: 8332156d73b02a5bb32a0be65492df1dee3d371f
branch: main
repository: loomspan
topic: "Loomspan Console PR 17 — Runtime, Skill, and Live-Inspection MCP Surface"
tags: [research, codebase, loomspan-console, mcp, skills, active-executions, recent-activity]
status: complete
last_updated: 2026-08-13
last_updated_by: Codex (GPT-5)
last_updated_note: "Used the approved developer workflows and local-tool deployment model to simplify the remaining PR 17 DTO, continuation, framing, error, conformance, and client-validation decisions."
---

# Research: Loomspan Console PR 17 — Runtime, Skill, and Live-Inspection MCP Surface

**Date**: 2026-08-13 18:23:17 PDT
**Researcher**: Codex (GPT-5)
**Git Commit**: `8332156d73b02a5bb32a0be65492df1dee3d371f`
**Branch**: `main`
**Repository**: `loomspan`

## Research Question

Research the current codebase for
`ai/thoughts/tickets/loomspan-console-pr-17-mcp-runtime-inspection.md`, using
`ai/thoughts/phases/loomspan_console_phase_3_llm_runtime_inspector.md` as
background and following `ai/commands/1_research_codebase.md`.

The ticket covers MCP tools for registered skills, current active executions,
and bounded recent activity; named capability advertisement; supplementary
resources; structured domain errors, scope, identifiers, continuations,
observation times, gaps, and reset boundaries; and adapter, cancellation,
malformed-input, multi-client, and representative-client coverage.

The working tree already contained a deletion of
`ai/thoughts/tickets/loomspan-console-pr-16-mcp-foundation.md` when this research
began. That user-owned deletion was left unchanged. The file remains available
at `HEAD`, while the supporting PR 16 research was removed by commit `8332156`
and was consulted from its last committed revision for historical context.

## Summary

PR 17 is not implemented in the current checkout. The MCP server currently
registers exactly `LOOMSPAN_get_runtime` and advertises exactly
`loomspan.runtime-status.v1`; it registers no skill, execution, recent-activity,
trace, prompt, or resource operation
(`loomspan-console/internal/mcpadapter/server.go:15-21`,
`loomspan-console/internal/mcpadapter/runtime.go:13-44`). The compatibility
document states the same PR 16-only surface
(`loomspan-console/docs/mcp-client-compatibility.md:1-7`).

The underlying evidence and transport-neutral query seams already exist:

- `observability.Service` owns selected-target skill list/detail and active
  execution list/detail queries, their Go DTOs, upstream response validation,
  page-size policy, and shared domain-error return type
  (`loomspan-console/internal/observability/service.go:13-222`).
- `live.Service` owns the single upstream SSE subscription, active baseline,
  bounded recent-activity window, continuity interval, reset fact, cursor
  ordering, per-session filtering, and independent browser subscriptions
  (`loomspan-console/internal/live/service.go:20-89`, `:148-166`, `:465-747`).
- Browser handlers are adapters over those two services. They capture a target
  scope, call the service, pre-encode the response, and use target-current
  publication checks before emitting it
  (`loomspan-console/internal/browserapi/observability.go:38-143`, `:286-297`;
  `loomspan-console/internal/browserapi/activity.go:163-204`).
- `target.Context` remains the selected-target authority. `Capture` returns one
  immutable scope snapshot, `Scope.Upstream` combines caller and target-scope
  cancellation, and `RequireCurrent`/`PublishCurrentAtomic` prevent a prior
  scope from publishing after rotation
  (`loomspan-console/internal/target/context.go:243-317`;
  `loomspan-console/internal/target/scope.go:39-89`).
- `consolecore.Error` is the shared error vocabulary used below the browser and
  MCP adapters. The browser already maps it to stable JSON and HTTP status;
  there is no corresponding structured MCP domain-error mapper yet
  (`loomspan-console/internal/consolecore/errors.go:5-53`;
  `loomspan-console/internal/browserapi/target.go:99-132`).

The current MCP foundation is stateless Streamable HTTP using the official Go
SDK `v1.7.0`. It supplies a 1 MiB request-body bound, a 10-second request
context, propagated client cancellation, authentication-generation admission,
freeze/cancel/drain behavior, and `no-store` responses
(`loomspan-console/go.mod:8`; `loomspan-console/internal/mcpadapter/server.go:15-21`;
`loomspan-console/internal/mcpadapter/security.go:14-102`;
`loomspan-console/internal/mcpadapter/tracker.go:24-90`). These controls wrap
future PR 17 handlers automatically when they are registered on the same SDK
server. No MCP response-envelope limit, PR 17 continuation format, PR 17 text
fallback, resource template, or structured domain-error envelope exists today.

The targeted Go suites for `mcpadapter`, `observability`, `live`, `browserapi`,
and `console` passed during this research. The Java
`LoomspanPublicSurfaceArchitectureTest` also passed (8 tests), confirming the
current Java API/SPI classification.

## Detailed Findings

### 1. Current MCP adapter and composition root

`mcpadapter.NewServer` creates one SDK server identified as
`loomspan-console`, registers the runtime tool, configures a stateless JSON
Streamable HTTP handler, and places it behind Loomspan's MCP security handler
(`loomspan-console/internal/mcpadapter/server.go:15-21`). The SDK module is
pinned at `github.com/modelcontextprotocol/go-sdk v1.7.0`
(`loomspan-console/go.mod:8`).

The composition root creates one `observability.Service` and one `live.Service`.
The live baseline loader obtains all active-execution pages through the
observability service and retains the initial page's `resumeCursor` and
`observedAt`. The live service is registered as a target-scope owner, so target
rotation clears its application-derived state
(`loomspan-console/internal/console/service.go:137-166`). The same composition
root currently gives `NewServer` only the credential store, request tracker,
and side-effect-free status provider; the observability, live, and target
dependencies are not passed into the MCP adapter
(`loomspan-console/internal/console/service.go:247-248`).

The shared listener mounts the MCP handler as a peer to the browser API through
`webhost.Routes` (`loomspan-console/internal/console/service.go:290`). This is
the existing route/security boundary behind which PR 17 tools operate; the MCP
adapter does not have an independent listener.

### 2. Existing structured-output and text-fallback behavior

`LOOMSPAN_get_runtime` demonstrates the current typed-tool pattern:

- an empty Go input struct causes the SDK to infer an empty-object input schema;
- `RuntimeOutput` causes the SDK to infer and validate an output schema;
- the typed output becomes `structuredContent`;
- the handler supplies a deterministic text content block separately; and
- a pre-result MCP authentication-generation check prevents late successful
  publication after credential mutation
  (`loomspan-console/internal/mcpadapter/runtime.go:16-67`).

The pinned SDK's typed `mcp.AddTool` path validates input, rejects unknown
properties when inferred by the current empty input, marshals and validates the
typed output, and places that value in `structuredContent`. A regular handler
error becomes an unsuccessful tool result with `isError: true` and error text;
JSON-RPC errors remain protocol-level errors. Current runtime invariant failures
use the safe text `INTERNAL: runtime status is unavailable`; the adapter has no
general structured Loomspan error output type
(`loomspan-console/internal/mcpadapter/runtime.go:36-45`).

The runtime golden test verifies exact structured JSON and agreement with the
text fallback (`loomspan-console/internal/mcpadapter/runtime_test.go:15-42`).
The assembled SDK test verifies tool discovery, structured content, one text
block, and unknown-input rejection
(`loomspan-console/internal/mcpadapter/server_test.go:84-141`). No equivalent
schemas, fallbacks, or golden files exist for the five PR 17 tools.

### 3. Registered-skill query path

The application-side REST controller exposes:

- a keyset-paginated registered-skill collection; and
- direct registered-name lookup returning the registered name, normalized
  source path, and unchanged YAML
  (`loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/observability/web/ObservabilityRestController.java:67-101`).

The Java wire records are `SkillSummary(registeredName, sourcePath, href)` and
`SkillDetail(registeredName, sourcePath, yaml)`
(`loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/observability/web/dto/ObservabilityDtos.java:30-31`).
`ObservabilityDtoMapper` constructs the API-root-relative `href` and returns the
catalog YAML without parsing or reserialization
(`loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/observability/web/ObservabilityDtoMapper.java:13-23`).

The Go service mirrors those records and adds console-local `targetScopeId` to
detail results and page envelopes
(`loomspan-console/internal/observability/dto.go:19-30`, `:95-111`).
`ListSkills` calls the selected scope's skills endpoint, accepts the application
cursor and page size, enforces the shared response bound, validates the page,
adds the scope, and checks that the scope is still current. `GetSkill` requires
a nonempty name, reads one bounded detail, requires the returned name to equal
the request, requires nonempty YAML, adds the scope, and performs the same
current-scope check
(`loomspan-console/internal/observability/service.go:51-96`, `:255-294`).

The browser's `/skills/list` and `/skills/detail` handlers contain no catalog
semantics beyond input decoding, scope capture, service invocation, and scoped
publication (`loomspan-console/internal/browserapi/observability.go:38-90`).
The TypeScript client consumes the same serialized Go DTOs
(`loomspan-console/web/src/api/contracts.ts:94-106`).

There is no Go-owned second skill catalog. Each current skill operation reaches
the application through `Scope.Upstream`. The only retained skill material is
whatever a caller already received; `live.Service` retains no skill definitions.
No MCP skill resource or `loomspan://` URI is registered in the current code.

### 4. Active-execution query path

The application REST controller requires live monitoring for both collection
and detail operations. The collection captures a registry high-water mark,
returns newest-first snapshots, and puts the current replay-buffer cursor in
`resumeCursor` only on the initial page. Direct lookup uses `sessionId` and
returns `NOT_FOUND` after the active entry no longer exists
(`loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/observability/web/ObservabilityRestController.java:103-145`).

The Java active-execution wire DTO includes session and trace identifiers,
canonical sequence, start/update/elapsed times, entry skill, active status,
phase, summary, bounded active path, depth/truncation facts, provisional usage,
and configured limits
(`loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/observability/web/dto/ObservabilityDtos.java:32-71`).
Elapsed time is calculated at REST observation time by the application mapper
(`loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/observability/web/ObservabilityDtoMapper.java:25-56`).

The Go `ActiveExecution` DTO serializes the same current snapshot plus
`targetScopeId`; `ActivePage` adds `resumeCursor` to the common page envelope
(`loomspan-console/internal/observability/dto.go:61-111`). The shared service
validates identity, timestamps, nonnegative sequence/elapsed/depth, status,
phase, and a present active path. It does not calculate a trace hierarchy or
retain completed-execution history
(`loomspan-console/internal/observability/service.go:98-148`, `:297-309`).

The browser's list/detail endpoints are thin adapters over those methods
(`loomspan-console/internal/browserapi/observability.go:92-143`). The live
service also holds a periodically refreshed copy of the complete active
baseline for live presentation and removes entries when terminal activity is
accepted, but browser list/detail requests continue to use
`observability.Service` directly
(`loomspan-console/internal/live/service.go:64-65`, `:403-458`, `:531-540`).

### 5. Recent-activity ownership and continuity

One `live.Service` owns the upstream application subscription and the shared
recent window. Its fixed bounds are 2,048 activities and 8 MiB for the window,
12 KiB for one encoded activity, 256 frames/1 MiB per browser subscriber, and a
recent-query default/max of 100/256 items
(`loomspan-console/internal/live/dto.go:10-13`;
`loomspan-console/internal/live/service.go:20-29`).

The activity envelope carries `instanceId`, application delivery `cursor`,
`sessionId`, `traceId`, optional canonical sequence, timestamp, kind, optional
execution/frame/route facts, summary, and raw JSON details
(`loomspan-console/internal/live/dto.go:82-98`). The current Go validator's
allowlist contains 18 activity kinds. `MODEL_ATTEMPT_FAILED` is declared as an
`ActivityKind` constant but is not a member of that allowlist or its label map
(`loomspan-console/internal/live/dto.go:18-75`).

Activity admission requires positive decimal monotonic cursors, ignores an
exact duplicate cursor/content pair, rejects conflicting reuse or regression,
evicts oldest entries when either shared bound is reached, and records that an
eviction occurred (`loomspan-console/internal/live/service.go:605-655`).

The window is cleared before new evidence is admitted when:

- the target scope changes;
- an SSE frame establishes a changed `instanceId`; or
- the application returns `STALE_CURSOR` during reconnect
  (`loomspan-console/internal/live/service.go:139-166`, `:243-274`, `:465-527`).

Each reset advances an opaque process-local interval ID and preserves at most
one reset fact containing cause, timestamp, and prior last cursor. A continuity
snapshot reports interval ID, target scope, instance, first/last retained
cursor, upstream observation time, and the reset fact
(`loomspan-console/internal/live/dto.go:142-165`;
`loomspan-console/internal/live/service.go:732-749`). Consequently, a recent
result is drawn from one interval only.

`Recent(cursor, sessionId, limit)` is the current transport-neutral snapshot
operation. With no cursor it returns the newest suffix; with a cursor it returns
later retained activities; with a session filter it returns only that
execution's activities in application cursor order. `hasMore` and `nextCursor`
describe additional retained matching items. If the supplied cursor is absent,
the method returns no items and `beginningUnavailable: true`. That fact is also
true when the returned suffix omits earlier retained items, eviction occurred,
or the interval follows a reset
(`loomspan-console/internal/live/service.go:658-729`).

The serialized recent response contains items, `hasMore`, `nextCursor`,
continuity, and `beginningUnavailable`; it has no separate top-level query
observation timestamp (`loomspan-console/internal/live/dto.go:173-185`). The
continuity observation time is sourced from the upstream handshake or baseline,
not generated by `Recent` at query time.

The browser recent handler captures the current target, calls this method,
wraps nil items as an empty array, and atomically publishes only if the scope is
still current (`loomspan-console/internal/browserapi/activity.go:163-204`). It
does not create another history. Browser live delivery is separately adapted
through `SubscribeSnapshotAfter` and per-tab SSE
(`loomspan-console/internal/browserapi/activity.go:47-160`).

`LiveUnavailable()` is a separate current service fact
(`loomspan-console/internal/live/service.go:363-368`). The current browser
recent handler does not consult it before returning the retained snapshot; the
application active-execution endpoints independently return
`LIVE_MONITORING_UNAVAILABLE` when live monitoring is disabled.

### 6. Continuation formats currently in the path

There are two existing continuation forms relevant to PR 17:

1. **Application collection cursor.** Java creates a maximum-4,096-character,
   unpadded base64url JSON keyset cursor bound to `instanceId`, collection,
   order, filter, and high-water/position facts. A different instance produces
   `STALE_CURSOR`; malformed or mismatched collection state produces
   `INVALID_CURSOR`
   (`loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/observability/web/ObservabilityCursorCodec.java:9-96`).
   Go currently forwards this opaque application cursor through
   `observability.Page.NextCursor` and sends it back on the next upstream REST
   request (`loomspan-console/internal/observability/dto.go:95-101`;
   `loomspan-console/internal/observability/service.go:51-60`).
2. **Recent-activity cursor.** The live service uses the application's positive
   decimal delivery cursor directly as its lookup and continuation value
   (`loomspan-console/internal/live/service.go:605-629`, `:658-729`).

No MCP-specific continuation encoder or decoder exists in `mcpadapter`.
Therefore no current token binds a PR 17 MCP continuation to tool family,
query parameters, target scope, or response schema independently of the
underlying application/live cursor. The trace-analysis package has a separate
versioned, scope/handle/query-bound cursor implementation, but no PR 17 code
imports or uses it.

### 7. Target scope, identifiers, and cancellation

`target.Context.Capture` is the current selected-target gate. It returns
`INVALID_ARGUMENT` when no target is selected, returns
`TARGET_CHANGED` during invalidation, and otherwise captures the current scope
ID, target address, application instance ID, scope context, application client,
and current process-local application credential
(`loomspan-console/internal/target/context.go:243-257`).

Every observability result receives `targetScopeId` in Go after the application
response is validated. Application-produced identifiers such as registered
skill name, `sessionId`, `traceId`, frame IDs, canonical sequence, activity
cursor, and `instanceId` remain unchanged. `sourcePath` remains returned text;
neither the shared service nor browser adapter uses it for filesystem access.

`Scope.Upstream` combines the MCP/browser caller context with target-scope
cancellation. Caller cancellation maps to a bounded `TARGET_UNAVAILABLE`
domain error with the safe message "The operation was canceled"; scope
rotation maps to `TARGET_CHANGED`. Instance-header mismatch triggers target
revalidation and returns `TARGET_CHANGED` if no newer classification is
available (`loomspan-console/internal/target/scope.go:39-83`).

At the MCP boundary, every admitted HTTP request already receives a 10-second
context and SDK request cancellation propagation. Credential regeneration,
disablement, and shutdown cancel tracked MCP contexts and wait for them to
drain (`loomspan-console/internal/mcpadapter/security.go:53-79`;
`loomspan-console/internal/mcpadapter/tracker.go:54-90`;
`loomspan-console/internal/mcpadapter/lifecycle.go:42-113`). The recent query is
an in-memory mutex-protected snapshot and currently accepts no context; upstream
skill/execution calls receive and use the caller context.

Stateless Streamable HTTP creates no Loomspan client session or conversation
registry. Multiple authenticated clients share the one credential, target,
observability service, live service, recent window, and service bounds, while
each HTTP request has independent context, admission entry, and cancellation.

### 8. Shared domain errors and adapter/protocol separation

The current shared domain vocabulary includes `INVALID_ARGUMENT`, target
authentication/access/availability/compatibility, `TARGET_CHANGED`, cursor
errors, `NOT_FOUND`, live unavailability, limit/storage/artifact errors, and the
sanitized `CONSOLE_ERROR`. Messages longer than 512 bytes or blank messages are
replaced with a fixed safe message; internal causes are unexported
(`loomspan-console/internal/consolecore/errors.go:5-53`).

Application problem bodies are normalized by `applicationclient` into that
vocabulary. Recognized `INVALID_REQUEST`, `INVALID_CURSOR`, `STALE_CURSOR`,
`NOT_FOUND`, `LIMIT_EXCEEDED`, `LIVE_MONITORING_UNAVAILABLE`, application-key
rejection, and application error remain distinct; unrecognized transport and
protocol conditions are classified separately
(`loomspan-console/internal/applicationclient/problem.go:8-59`;
`loomspan-console/internal/applicationclient/errors.go:76-104`).

The browser adapter maps one `consolecore.Error` to `{error:{code,message,
targetScopeId?,details?}}` and stable HTTP status without changing the shared
code (`loomspan-console/internal/browserapi/target.go:99-132`). Malformed
browser JSON is rejected earlier as adapter code `INVALID_REQUEST`
(`loomspan-console/internal/browserapi/errors.go:45-67`).

MCP security and protocol failures are already outside the domain mapper:
invalid authority/origin, disabled MCP, bearer rejection, frozen admission,
oversized bodies, malformed JSON-RPC, unsupported methods, and protocol
negotiation remain HTTP/SDK failures
(`loomspan-console/internal/mcpadapter/security.go:22-79`). Typed-tool JSON
Schema failures become unsuccessful SDK tool results before a handler is
called. There is no PR 17 mapper that serializes a `consolecore.Error` into a
bounded structured MCP result while retaining `isError: true`.

Replay loss is not a `consolecore.Error` in the live service. It is represented
inside a successful recent response through `beginningUnavailable`, continuity
cursors, and an optional reset fact.

### 9. Response and evidence bounds

Current bounds at each layer are:

| Layer | Current bound |
|---|---|
| MCP HTTP request | 1 MiB body and 10-second request context (`mcpadapter/security.go:14-15`) |
| Application collection body read by Go | 16 MiB (`observability/service.go:18`) |
| Application skill detail body read by Go | 4 MiB (`observability/service.go:19`) |
| Application active detail body read by Go | 4 MiB (`observability/service.go:20`) |
| Go application collection page size | default 1,000, maximum 5,000 (`observability/service.go:16-17`, `:204-216`) |
| One live activity | 12 KiB encoded JSON (`live/dto.go:12`) |
| Shared recent window | 2,048 activities and 8 MiB (`live/service.go:21-22`) |
| One recent query | default 100, maximum 256 activities (`live/service.go:25-26`) |
| Browser adapter JSON input | 4 KiB for observability and recent activity (`browserapi/observability.go:13`, `browserapi/activity.go:15-16`) |

The MCP handler has no explicit response-byte limiter. Its only current tool
returns a fixed small status envelope. The SDK serializes the complete typed
tool output after the handler returns. No PR 17 per-result framing constant or
server-advertised maximum is present.

Skill YAML and activity `summary`/`details` are returned application data. Go
validates envelope structure and size but does not execute, interpret as tool
input, redact, parse as instructions, or perform content-directed filesystem or
network access. The MCP adapter currently has no code path that receives these
fields, so no PR 17 safe text fallback behavior exists yet.

### 10. Resources and resource interoperability

The pinned SDK supports concrete resources, RFC 6570 resource templates,
resource links in tool content, text/blob resource contents, and resource
listing/reading. The current Loomspan MCP server calls neither `AddResource`
nor `AddResourceTemplate`, and its runtime result contains only text plus
structured content.

The Phase 3 background identifies the skill resource template conceptually as
`loomspan://targets/{targetScopeId}/skills/{skillName}` and keeps runtime
status, active executions, and recent activity tool-only because those views
are volatile or continuable
(`ai/thoughts/phases/loomspan_console_phase_3_llm_runtime_inspector.md:381-406`).
The PR 17 ticket permits essential supplementary resources but does not name an
exact required resource set. No committed resource URI parser, percent-encoding
rule, resource result DTO, or resource error mapping exists in live code.

The application REST `href` on a skill summary is relative to the Java
observability API and is not an MCP resource URI
(`loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/observability/web/ObservabilityDtoMapper.java:13-18`).

### 11. Capability advertisement and conformance

`LOOMSPAN_get_runtime` returns a deterministic `capabilities` array containing
only `loomspan.runtime-status.v1`
(`loomspan-console/internal/mcpadapter/runtime.go:13-44`). The ticket's three
later capability identifiers do not occur in production Go code. The Phase 3
background defines their required tool families:

- `loomspan.skill-inspection.v1` — list and get skill;
- `loomspan.active-execution-inspection.v1` — list and get execution; and
- `loomspan.recent-activity-inspection.v1` — get execution activity
  (`ai/thoughts/phases/loomspan_console_phase_3_llm_runtime_inspector.md:872-893`).

Current official conformance runs initialization, tool listing, caching, and
DNS-rebinding scenarios for the supported protocol revisions. Production
tool discovery/calling is covered separately through SDK and HTTP integration
tests. The conformance harness intentionally has no product test resources,
prompts, sampling, or elicitation
(`loomspan-console/mcp-conformance/README.md:1-14`;
`loomspan-console/internal/buildtool/mcp_conformance.go:82-119`).

No executable capability-family table currently checks that an advertised PR
17 capability and all of its tools appear together. Runtime golden and server
tests currently assert a one-capability/one-tool catalog
(`loomspan-console/internal/mcpadapter/runtime_test.go:15-76`;
`loomspan-console/internal/mcpadapter/server_test.go:25-141`).

### 12. Existing tests and fixtures relevant to PR 17

Current executable coverage includes:

- skill, instance, active page/resume cursor, detail, page clamping, malformed
  upstream data, `STALE_CURSOR`, `NOT_FOUND`, and live-unavailable service tests
  (`loomspan-console/internal/observability/service_test.go:62-489`);
- duplicate/regressed activity, single-interval reset, session filtering,
  query clamping, evicted beginnings, subscriber independence, byte bounds,
  shutdown, continuity identity, and atomic snapshot/replay tests
  (`loomspan-console/internal/live/coordinator_test.go:30-650`);
- SSE receipt/reconnect, stale-cursor rebaseline, late baseline suppression,
  target invalidation, instance mismatch, and filtered recent pagination tests
  (`loomspan-console/internal/live/service_test.go:72-446`);
- browser session/input/header/canonical-DTO coverage for observability and
  recent activity
  (`loomspan-console/internal/browserapi/observability_test.go:90-374`;
  `loomspan-console/internal/browserapi/activity_test.go:41-398`);
- assembled Java-to-Go-to-browser SSE and terminal-activity tests
  (`loomspan-console/internal/console/activity_integration_test.go:22-452`);
- MCP negotiation, discovery, runtime call, structured output, unknown input,
  security ordering, cancellation generation, `no-store`, lifecycle drain,
  and shutdown tests
  (`loomspan-console/internal/mcpadapter/server_test.go:25-141`;
  `loomspan-console/internal/mcpadapter/security_test.go:58-218`;
  `loomspan-console/internal/mcpadapter/lifecycle_test.go:15-255`); and
- Java REST/SSE fixture generation and integration tests, including active
  resume cursors, stale continuations, and live unavailability
  (`loomspan-spring-boot-starter/src/test/java/com/lokiscale/loomspan/internal/observability/web/ObservabilityRestIntegrationTest.java:156-282`;
  `loomspan-spring-boot-starter/src/test/java/com/lokiscale/loomspan/internal/observability/web/ConsoleRestFixtureCorpusTest.java:38-119`).

The committed cross-language corpus includes skill list/detail, active
list/detail, continuation/empty pages, stable problem bodies, SSE handshake,
replay, successful completion, and core-finalization-failed terminal activity
under `loomspan-console-fixtures/application-rest/` and
`loomspan-console-fixtures/application-sse/`. Its README identifies those files
as deterministic Java-produced bodies consumed by Go
(`loomspan-console-fixtures/README.md:42-70`).

No current test maps the same shared skill/execution/activity result through
both browser and MCP, invokes the five PR 17 tools, reads a skill MCP resource,
checks PR 17 structured/text parity, or verifies PR 17 capability-family
completeness. The representative-client matrix currently records all local
manual PR 16 runs as not run/release evidence required
(`loomspan-console/docs/mcp-client-compatibility.md:17-37`).

## Contract and Compatibility Classification

The classifications below use the exact categories from
`ai/thoughts/framework-feature-design-lens.md`.

### Application API

PR 17 changes no supported Java Application API in the current ticket shape.
The closed allowlist is enforced by
`LoomspanPublicSurfaceArchitectureTest`, and the observability types live under
`com.lokiscale.loomspan.internal`
(`loomspan-spring-boot-starter/src/test/java/com/lokiscale/loomspan/architecture/LoomspanPublicSurfaceArchitectureTest.java:27-339`).

### Supported SPI

The repository declares no supported Java SPI. The architecture test explicitly
asserts that no SPI package or type exists
(`loomspan-spring-boot-starter/src/test/java/com/lokiscale/loomspan/architecture/LoomspanPublicSurfaceArchitectureTest.java:317-322`).
There is no `@ConditionalOnMissingBean` declaration in production Java code.

### Configuration and manifest contracts

The existing documented `loomspan.observability.*` properties and registered
skill YAML syntax are configuration/manifest contracts. PR 17 reads the
application-provided registered YAML but does not change its syntax or create a
parsed effective-definition contract. PR 16 added no Console MCP YAML setting;
MCP enablement remains canonical key-file presence
(`loomspan-console/README.md:215-240`). No PR 17 configuration key exists.

### Persisted or serialized contracts

The Java observability REST/SSE boundary and its problem bodies are serialized
cross-component contracts consumed by the Go console for the matching exact
`consoleCompatibilityVersion`. Protected consumers are:

- `applicationclient` for HTTP/SSE framing, instance headers, problems, and
  cancellation;
- `observability.Service` for instance, skill, active-execution, trace, and
  page bodies;
- `live.Service` for handshake/activity envelopes and continuity; and
- browser and future MCP adapters for the resulting Go service DTOs.

Executable fixtures under `loomspan-console-fixtures/application-rest/` and
`application-sse/`, Java fixture writers, Go DTO/service tests, and assembled
console tests protect this boundary. The exact product release string remains
the compatibility marker checked before target operations
(`loomspan-console-fixtures/README.md:67-70`).

MCP tool names, named Loomspan capability identifiers, typed input/output
schemas, resource URI shapes, and structured error fields become serialized
Go-to-MCP-client contracts when implemented. At the current commit, only the
runtime tool/capability/schema is present.

### Ephemeral diagnostic formats

Active snapshots, recent activity, continuity intervals, reset facts, Go
target-scope IDs, application collection cursors as consumed by Go, future MCP
continuations, and returned skill YAML are current-run diagnostic evidence.
The recent window is process-local, bounded, and cleared at continuity
boundaries; it is not durable execution history. The skill YAML itself remains
the unchanged application-provided manifest content, while its MCP transport
envelope is diagnostic presentation.

### Internal or accidentally exposed implementation

Java `ObservabilityRuntime`, controller, wire DTOs, cursor codec, registries,
catalogs, projector, replay buffer, and Spring beans are internal framework
machinery. Their public Java modifiers support framework composition, not an
application extension contract. The architecture allowlist records reasons for
technically public internal types and separately proves observation DTOs do not
expose paths, resources, trace records, throwables, streams, publishers, or
runtime usage types
(`loomspan-spring-boot-starter/src/test/java/com/lokiscale/loomspan/architecture/LoomspanPublicSurfaceArchitectureTest.java:371-412`).

All Go packages under `loomspan-console/internal/`, including service structs,
constructors, DTOs, adapters, cursor handling, and MCP SDK types, are internal
implementation. The Spring observability web auto-configuration creates
infrastructure-role beans directly and exposes no missing-bean replacement
surface
(`loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/autoconfigure/LoomspanObservabilityWebAutoConfiguration.java:35-124`).

### Public declarations, interfaces, constructors, beans, and usage inventory

- **Public Java declarations:** technically public internal observability
  runtime/controller/mapper/cursor/catalog/registry types; none is in the
  supported `com.lokiscale.loomspan.api` allowlist.
- **Interfaces and constructors:** Java runtime observation interfaces and
  public internal constructors are framework collaboration seams. Go uses
  concrete `*observability.Service` and `*live.Service` values in the browser
  `Options`; MCP currently accepts only a `StatusProvider` function plus an
  internal credential interface.
- **Spring beans:** observability codec, mapper, cursor, page writer,
  controller, collision detector, route registrar, access filter, activation
  coordinator, and session runner are infrastructure beans. No
  `@ConditionalOnMissingBean` is present.
- **Verified in-repository consumers:** Java REST/SSE writers feed Go
  `applicationclient`; Go shared services feed browser handlers and the live
  baseline; `LOOMSPAN_get_runtime` alone feeds MCP. No application code uses
  internal Go/Java types as a supported extension surface.
- **Documentation:** root README documents the Java observability boundary;
  Console README documents shared live behavior and MCP foundation; MCP client
  compatibility documents the one-tool current surface; Phase 3 and the PR 17
  ticket document the later intended surface.

## Architecture Documentation

The current data path for PR 17 evidence is:

```text
Java runtime registries/catalogs/replay buffer
    -> authenticated /_loomspan/observability/v1 REST + SSE
        -> applicationclient
            -> target.Scope cancellation and identity checks
                -> observability.Service (skills and active snapshots)
                -> live.Service (one SSE subscription and one recent window)
                    -> browserapi adapters

MCP request
    -> MCP authority/origin/bearer/admission middleware
        -> official SDK typed tool adapter
            -> currently only consolecore.StatusSnapshot
```

The missing PR 17 portion is the peer MCP mapping from the SDK tool handlers to
the already-existing target, observability, and live services. The current
source tree contains no alternative MCP catalog, registry, subscription,
history, or direct application client.

The ownership boundaries are:

- Java owns authoritative registered skills, active execution state, delivery
  cursors, and SSE replay admission.
- `target.Context` owns selected target, credential capability, instance
  commitment, `targetScopeId`, rotation, and late-result rejection.
- `observability.Service` owns bounded REST retrieval and Go-side response
  validation.
- `live.Service` owns the single continuous Go activity interval, reconnect,
  reset, bounded recent window, and browser subscription fan-out.
- Browser and MCP own only protocol validation and presentation.
- MCP credential lifecycle is independent of target scope and runtime evidence.

## Historical Context (from `ai/thoughts/`)

- `ai/thoughts/phases/loomspan_console_phase_3_llm_runtime_inspector.md` records
  the peer-adapter architecture, tool-first surface, named capability families,
  untrusted-content boundary, finite continuable results, and one-continuity
  recent-activity model. Its PR 16 supersession note is consistent with the
  current SDK `v1.7.0`, stateless `/mcp`, IPv4-only, `lsmcp_`, and minimal
  runtime envelope implementation.
- `ai/thoughts/tickets/loomspan-console-pr-17-mcp-runtime-inspection.md` narrows
  this change to status, skills, active executions, and recent activity; trace
  acquisition/parsing and Agent Skill distribution remain outside it.
- `ai/thoughts/phases/loomspan_console_workflows.md` supplies the canonical
  workflow requirements most directly represented here: `WF-X-R7`,
  `WF-FE-R10`, `WF-SE-R2`, `WF-SE-R3`, `WF-SE-R6`, `WF-SE-R9`, `WF-SE-R10`,
  `WF-SP-R2`, `WF-SP-R7`, `WF-SP-R8`, and `WF-SP-R9`.
- `ai/thoughts/phases/2026-08-12-loomspan-active-roadmap.md` orders PR 17 after
  the MCP foundation and before trace inspection and the debugging skill.
- The PR 16 ticket is deleted in the working tree but present at `HEAD`; its
  historical supporting research was removed by commit `8332156`. The resolved
  PR 16 decisions are embodied by the current MCP code and tests.

## Related Research

No other research document is present in `ai/thoughts/research/` in the current
working tree. The prior PR 16 research is available only in Git history at this
commit lineage.

## Open Questions

The following details are not represented by current production code, tests,
or a more specific committed PR 17 contract:

1. The exact JSON field sets, required/optional rules, annotations, and concise
   text fallback for each of the five PR 17 tools.
2. The exact MCP continuation representation for application collection pages
   and recent-activity progress, including what it binds beyond the current
   raw upstream cursor.
3. Whether PR 17 includes the conceptual skill resource template, and if so,
   its exact URI encoding, discovery metadata, returned MIME type/content, and
   domain-error representation.
4. The exact per-result MCP response-envelope bound and behavior when a caller
   requests a page whose fully serialized structured/text result exceeds that
   bound.
5. The exact structured unsuccessful-result schema used to preserve
   `consolecore.Error` fields while retaining a concise safe text result and
   `isError: true`.
6. The query-time observation field for recent activity, because the current
   shared response exposes upstream continuity observation time but no separate
   time at which `Recent` was called.
7. The exact MCP behavior when `live.Service.LiveUnavailable()` is true while
   the shared recent window still contains retained activities.
8. The capability-conformance fixture or test table that binds each new named
   capability to its complete required operation family and semantics.
9. Which representative client versions and resource/structured-content
   behaviors will be recorded for PR 17; the current manual matrix contains no
   completed local-client run.

These are unclassified implementation-contract details at the current commit,
not behavior supplied by another live package.

## Verification Performed

- `go test ./internal/mcpadapter ./internal/observability ./internal/live ./internal/browserapi ./internal/console` — passed.
- `.\\mvnw.cmd -pl loomspan-spring-boot-starter -Dtest=LoomspanPublicSurfaceArchitectureTest test` — passed, 8 tests.
- `bash ai/scripts/spec_metadata.sh` — supplied the document timestamp, commit,
  branch, and repository metadata above.

## Follow-up Research [2026-08-13 18:29:36 PDT]

### Question

Can the nine open questions above be answered by the phase documents or future
tickets in `ai/thoughts/tickets/`?

### Resolution

Yes, but at different levels. The Phase 3 design settles product semantics and
ownership boundaries that PR 17 must not reopen. It deliberately leaves several
MCP wire-format and representative-client validation details to PR 17 detailed
planning. PR 18 and PR 19 clarify later ownership, but they do not supply the
missing PR 17 schemas.

| # | Status after document audit | Answer and remaining work |
| --- | --- | --- |
| 1 | Partially answered | Phase 3 requires precise tool purposes, strict input JSON Schema, structured output where appropriate, concise text fallback, observation timestamps, stable identifiers, availability/truncation facts, errors, and useful resource links (`loomspan_console_phase_3_llm_runtime_inspector.md:470-481`). It also defines the five tools' semantic results (`:446-454`). Exact field names, required/optional rules, annotations, output schemas, and fallback wording remain PR 17 work, explicitly assigned by its detailed-planning focus (`loomspan-console-pr-17-mcp-runtime-inspection.md:50-54`). |
| 2 | Partially answered | The continuation contract is settled semantically: finite caller-selected pages, explicit continuation, scope binding, `TARGET_CHANGED` after rotation, and no exposure of application pagination cursors as interchangeable MCP tokens (`loomspan_console_phase_3_llm_runtime_inspector.md:357`, `:470-483`). The token encoding and exact bindings remain PR 17 design work. PR 18's continuation-signing/opacity task (`loomspan-console-pr-18-mcp-trace-inspection.md:63-68`) is trace-specific and should inform consistency, not defer PR 17 runtime continuation design. |
| 3 | Inclusion answered; wire details open | Phase 3 explicitly includes `loomspan://targets/{targetScopeId}/skills/{skillName}` and requires it to return registered metadata plus unchanged UTF-8 YAML (`loomspan_console_phase_3_llm_runtime_inspector.md:401-412`). Therefore PR 17 should include the skill resource; it is supplementary and not part of capability conformance (`:893`). Exact URI-component encoding, advertised resource metadata, MIME declaration, and protocol-level error encoding remain PR 17 interoperability decisions. PR 18 owns only trace-resource URI work. |
| 4 | Policy answered; numeric bound open | Phase 3 rejects an invented MCP default such as 100 records or 256 KiB. Representative-client validation must determine the largest interoperable per-result frame, which is advertised/reported when continuation is required (`loomspan_console_phase_3_llm_runtime_inspector.md:483`). Partial results must be explicit and continuable (`:513-523`). The exact byte/envelope bound and serialization-overflow algorithm remain PR 17 validation work; PR 18 repeats this research for trace responses rather than deciding PR 17's value. |
| 5 | Largely answered | A valid authenticated domain failure returns a bounded unsuccessful tool result preserving the stable `code`, safe `message`, optional `targetScopeId`, and permitted code-specific details. Unexpected failures preserve sanitized `CONSOLE_ERROR`; malformed/schema/auth/protocol failures remain transport failures; replay gaps remain successful results (`loomspan_console_phase_3_llm_runtime_inspector.md:485-495`). Only the exact SDK DTO/output-schema wrapper and concise text rendering remain to define. |
| 6 | Semantic requirement answered | `LOOMSPAN_get_execution_activity` must report the returned cursor range and query observation time (`loomspan_console_phase_3_llm_runtime_inspector.md:452`), and every tool has an explicit observation timestamp (`:470-481`). Thus PR 17 must add a query-time observation value rather than reuse only the upstream continuity timestamp. Its exact JSON field name/type belongs with question 1. |
| 7 | Fully answered | When the application reports `liveMonitoringAvailable: false`, both active-execution and recent-activity tools return `LIVE_MONITORING_UNAVAILABLE`, even if retained or reconstructed-looking state exists; skill and finalized-trace inspection may continue (`loomspan_console_phase_3_llm_runtime_inspector.md:629-634`). Retained live-window entries must not be returned in that state. |
| 8 | Contract answered; fixture shape open | Phase 3 gives the exact mapping: skill capability to list/get skill, active-execution capability to list/get execution, and recent-activity capability to get activity (`loomspan_console_phase_3_llm_runtime_inspector.md:871-893`). Advertisement is forbidden until every required operation and semantic promise exists, and inconsistency is a conformance defect (`:899-905`). PR 17 still needs to choose the concrete table-driven fixture/test organization. PR 19 consumes required/optional capabilities for the skill and final evaluations; it does not own server capability conformance. |
| 9 | Partially answered; intentionally requires revalidation | Phase 3 names the target local client surfaces and requires one common tool/resource/identifier surface (`loomspan_console_phase_3_llm_runtime_inspector.md:330-343`, `:797-801`). It explicitly directs implementation planning to revalidate then-current client transport/skill capabilities and record them (`:982-989`). PR 19 owns broad representative agent evaluations and final Phase 3 conformance (`loomspan-console-pr-19-debugging-skill.md:13-26`, `:52-57`), but PR 17 still owns adapter-level representative-client checks for its resources, structured content, and fallback behavior. Exact versions cannot be answered from the committed docs because they are time-sensitive validation evidence. |

### Revised open set for PR 17 planning

The original questions should therefore not remain as nine undifferentiated
unknowns. The genuinely open PR 17 implementation decisions are:

1. exact input/output JSON schemas, annotations, and concise text renderings;
2. opaque runtime continuation encoding and bindings;
3. skill-resource URI encoding, metadata/MIME, and resource-error wire mapping;
4. measured per-result framing bound and serialized-overflow behavior;
5. exact SDK representation of structured unsuccessful tool results;
6. concrete query-observation field name/type;
7. concrete capability-conformance fixture/test organization; and
8. the then-current representative-client versions and observed resource,
   structured-content, and fallback behavior.

Question 7 from the original list is closed. Questions 3, 5, 6, and 8 have
settled semantic answers and only protocol/test representation remains. PR 18
and PR 19 provide consistency constraints and later acceptance coverage; they
do not justify deferring PR 17's own adapter contracts.

## Follow-up Research [2026-08-13 20:54:30 PDT]

### Workflow and simplicity review

The workflow catalog materially simplifies PR 17. It is the canonical catalog
for browser, MCP, and skill verification, but explicitly does not prescribe
exact MCP calls or create separate workflow data models
(`loomspan_console_workflows.md:43-49`). The four workflows are coordinated
views over the same services and facts, not separate product areas, stores, or
analysis engines (`:51-63`). Exact field spelling, response-framing constants,
and representative one-response interoperability values remain implementation
work (`:95-99`).

For PR 17, the slow-execution workflow supplies the most useful DTO acceptance
test. It requires an active summary containing identity, entry skill, timing,
phase, bounded active path, latest activity, usage/limits, observation time,
and continuity state, plus a bounded ordered activity interval with explicit
beginning-unavailable and reset behavior (`:249-279`, `:282-322`). The
skill-path workflow requires only registered name, descriptive `sourcePath`,
and unchanged YAML for the PR 17 skill surface (`:537-546`, `:566-576`). The
workflow document explicitly leaves final transport DTOs additive and open
without changing the settled five operations (`:339-350`).

The existing shared DTOs already carry most of this contract:

- `observability.SkillSummary`, `SkillDetail`, `ActiveExecution`, `Page`, and
  `ActivePage` contain the workflow facts and application pagination;
- `live.Activity`, `Continuity`, and `RecentResponse` contain ordered activity,
  interval identity, cursors, reset, and beginning-unavailable facts; and
- the main missing live result fact is the query-time `observedAt` required by
  the workflow, separate from `Continuity.ObservedAt`.

Therefore the recommended MCP DTO approach is a direct, explicit adapter over
these service results, not workflow-specific DTOs and not an independent MCP
model. Add only MCP-boundary facts required by Phase 3: current scope/instance
and query observation where absent, opaque continuation formatting, resource
links where useful, and structured/text error presentation.

### Simplified continuation decision

The access key and a continuation solve different problems. The access key
authenticates every MCP request. A continuation only identifies the next
bounded position in a multi-result query and preserves scope and continuity
meaning; it grants no additional access.

For this loopback, single-user development tool, cryptographic continuation
signing and a server-side continuation registry are unnecessary complexity.
Use a bounded, versioned, base64url-encoded adapter token whose representation
is private to MCP. It needs only:

- token version and operation kind;
- `targetScopeId`;
- the underlying next cursor; and
- `sessionId` for execution-activity continuation.

The adapter strictly decodes, size-bounds, and validates these fields, then
rechecks the authenticated current target scope on every call. A malformed
token is invalid input, a prior-scope token produces `TARGET_CHANGED`, and an
activity cursor that has left the window uses the existing successful gap
semantics. No HMAC, encryption, database, expiry scheduler, query fingerprint,
or separate authorization concept is needed. “Opaque” means callers must not
depend on or substitute the internal cursor representation; it does not require
the token to be a security credential.

### Simplified remaining recommendations

1. Reuse the current shared-service field shapes through small MCP mapping
   functions. Do not create a DTO per workflow or a universal success wrapper.
2. Use direct tool-specific structured success results. For domain failure,
   return the existing safe `code`, `message`, optional `targetScopeId`, and
   permitted details with MCP `isError`; do not add an `ok` discriminator unless
   the selected SDK demonstrably requires it for output-schema validation.
3. Keep the skill resource a thin supplementary view: one canonical
   scope-bound URI, unchanged YAML text, ordinary YAML MIME type, and the tool
   as the dependable structured path.
4. Require or clearly bound caller-selected page size at the MCP boundary and
   validate it against representative clients. Start without adaptive
   reserialization, binary-search page shrinking, or a second paging store;
   add such machinery only if interoperability evidence proves the existing
   bounded service pages are insufficient.
5. Add one query-time `observedAt` to recent activity, captured at the shared
   snapshot boundary. Do not redesign the rest of `live.RecentResponse`.
6. Use one small capability-to-required-tools table plus focused semantic
   contract tests. A general capability framework is not needed.
7. Keep PR 17 client evidence to a dated protocol smoke matrix: connect,
   discover/call tools, structured-versus-text display, error display,
   resource read, and continuation. Agent-quality evaluation remains PR 19.

This simplicity is consistent with the product scope: a local developer tool
still needs strict validation and truthful scope/continuity semantics, but it
does not need multi-tenant token security, durable continuation state,
workflow-specific APIs, or an enterprise compatibility framework.
