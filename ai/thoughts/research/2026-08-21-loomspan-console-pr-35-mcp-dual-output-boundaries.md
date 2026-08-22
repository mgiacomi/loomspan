---
date: 2026-08-21T22:34:10-07:00
researcher: Codex (GPT-5)
git_commit: 2fd265a609945e7fcf865471e8d3b30a7c4f561d
branch: main
repository: loomspan
topic: "PR 35 — MCP Dual-Output Efficiency and Presentation Boundaries"
tags: [research, codebase, loomspan-console, mcp, structured-content, text-fallback, response-budgets]
status: complete
last_updated: 2026-08-21
last_updated_by: Codex (GPT-5)
last_updated_note: "Added live Codex MCP presentation evidence for one active execution"
---

# Research: PR 35 — MCP Dual-Output Efficiency and Presentation Boundaries

**Date**: 2026-08-21 22:34:10 PDT
**Researcher**: Codex (GPT-5)
**Git Commit**: 2fd265a609945e7fcf865471e8d3b30a7c4f561d
**Branch**: main
**Repository**: loomspan

## Research Question

Research the current checkout for
`ai/thoughts/tickets/loomspan-console-pr-35-mcp-dual-output-efficiency-and-presentation-boundaries.md`, including ownership of MCP `content` and `structuredContent`, governing MCP and Go SDK behavior, fallback and failure contracts, response-size measurements, protected consumers and tests, and the boundary between server emission and downstream client presentation.

## Summary

The checkout is on `main` at `2fd265a`, after the four commits identified as PR 34 work in current history. Loomspan Console deliberately authors one deterministic text block and one typed result value for every successful tool call. The selected Go SDK then validates and serializes the typed value into `structuredContent`; because Loomspan already supplied `Content`, the SDK retains Loomspan's text instead of synthesizing its own serialized-JSON fallback. The Streamable HTTP layer adds JSON-RPC framing and, for MCP `2026-07-28`, `resultType: "complete"` and server metadata. No repository code controls how a connector or client subsequently presents those two server-emitted representations to a model.

The supported revisions both require the `content` array and make `structuredContent` optional. When a tool advertises an `outputSchema`, any structured result that is returned must conform to it. Both revisions' tool guidance say that a tool returning structured content should also return serialized JSON in text for backward compatibility. The only standard tools capability in these revisions concerns tool support/list changes (and later task or request features); neither revision defines capability negotiation for text-only versus structured-only tool results. The pinned Go SDK v1.7.0 supports both revisions and implements the backward-compatible dual-output default for typed handlers.

The two forms are not uniformly identical:

- Runtime, execution-list, and execution-detail text repeat the complete orientation facts in a deterministic flattened form.
- Activity text repeats cursor, continuity, coverage, identity, timestamp, kind, status, frame, route, and summary facts, but deliberately excludes the untrusted `details` object. Structured activity remains complete and includes `details`.
- Trace navigation text is compact and may bound individual displayed identifiers to 512 bytes, while structured values remain complete. Range-read text deliberately repeats the selected bounded content exactly.
- Domain failures return a concise `CODE: message` text block plus a structured error envelope and `isError: true`; internal causes are not serialized.

Current tests already protect the presence of one text item and non-nil structured content on successful SDK calls, the exact result/error envelope, representative structured schema validation, text ordering/escaping, active-execution and activity facts, safe omission of activity details from text, maximum 64-item pages, and trace response budgets. Checked-in exact raw HTTP result-size tests currently cover trace navigation/range calls, not active-execution result sizes.

Measured server-owned contributions confirm that both representations materially affect the wire response. For the committed small fixtures, reconstructed MCP `2025-11-25` JSON-RPC responses are approximately 2.1 KiB for execution list/detail and 2.4 KiB for activity. A 64-execution fixture is 120,687 bytes. The existing legal 64-activity test shape with 11 KiB of details per item is 772,064 bytes; 743,777 bytes are attributable to adding its structured representation to the text-only result, while 28,239 bytes are attributable to adding the safe text representation to a result with empty `content`. This difference comes from activity `details`, which structured output preserves and text omits.

The smallest code ownership point shared by non-runtime successes and domain failures is `contracts.go`; runtime constructs the same pair directly. However, removing Loomspan's text while retaining a typed `Out` would not produce structured-only output with the current SDK: the SDK would synthesize serialized JSON text. Shortening Loomspan text would reduce bytes but would change the repository's documented fact-complete fallback behavior where currently protected. Client-side suppression or presentation is outside the server repository and was not observable in this research environment.

## Detailed Findings

### 1. Current checkout and PR 34 context

- `main` points at `2fd265a609945e7fcf865471e8d3b30a7c4f561d` (`Cleanup PR 34 (p1)`). Its immediate history includes `9010f84`, `c409731`, and `ec7a0b0`, all labeled as PR 34 work.
- The active roadmap records PR 34's delivered behavior as one complete active page, one bounded activity call per selected session, reusable future checkpoints, and one trace-resolution attempt by returned `traceId` (`ai/thoughts/phases/2026-08-15-loomspan-llm-trace-understanding-roadmap.md:71`).
- The roadmap says the Console and MCP contracts are unreleased and changed in place until the v1 gate (`ai/thoughts/phases/2026-08-15-loomspan-llm-trace-understanding-roadmap.md:13`). It also makes deterministic Go/Java tests, fixtures, package validation, and official MCP conformance the repository boundary rather than a client/model usability matrix (`ai/thoughts/phases/2026-08-15-loomspan-llm-trace-understanding-roadmap.md:80`).
- The user-owned worktree already contained a rename/rewrite of the PR 35 ticket: the tracked `loomspan-console-pr-35-mcp-response-efficiency.md` is deleted and the requested `loomspan-console-pr-35-mcp-dual-output-efficiency-and-presentation-boundaries.md` is untracked. This research did not alter either ticket.

### 2. Exact ownership flow for a successful result

The server-owned flow is:

1. An MCP handler maps application/live/trace facts into a concrete Go result DTO.
2. The handler generates deterministic fallback text.
3. `successResult` returns an `mcp.CallToolResult` containing one `TextContent` plus a separate typed `toolEnvelope[T]{Result: &value}` (`loomspan-console/internal/mcpadapter/contracts.go:155`). Runtime performs the same construction directly because its structured output is `RuntimeOutput` rather than a result/error envelope (`loomspan-console/internal/mcpadapter/runtime.go:40`).
4. `addValidatedTool` installs a compact advertised `OutputSchema`, validates the complete typed result, and passes both values to `mcp.AddTool` (`loomspan-console/internal/mcpadapter/output_schemas.go:17`, `loomspan-console/internal/mcpadapter/output_schemas.go:33`).
5. In go-sdk v1.7.0, the typed handler wrapper marshals and validates `Out`, assigns those JSON bytes to `res.StructuredContent`, and leaves existing object-valued `Content` unchanged. If `Content` were nil, it would instead add a `TextContent` containing the serialized structured JSON. This behavior is in the selected SDK's [`mcp/server.go`](https://github.com/modelcontextprotocol/go-sdk/blob/v1.7.0/mcp/server.go#L403-L444).
6. Loomspan configures a stateless JSON-response Streamable HTTP handler (`loomspan-console/internal/mcpadapter/server.go:54`, `loomspan-console/internal/mcpadapter/server.go:65`). The SDK/transport writes the JSON-RPC response. MCP `2026-07-28` additionally carries `resultType: "complete"` and response `_meta` server information.
7. Connector and client presentation happens after the response leaves this handler. No connector-specific rendering, model-event persistence, or client presentation threshold exists in this repository.

The pinned dependency is `github.com/modelcontextprotocol/go-sdk v1.7.0` (`loomspan-console/go.mod:8`). Its `CallToolResult` declares `content` as the unstructured result, `structuredContent` as optional, and `isError` for tool execution failures; the typed handler owns automatic structured population. See the selected SDK's [`mcp/protocol.go`](https://github.com/modelcontextprotocol/go-sdk/blob/v1.7.0/mcp/protocol.go#L261-L306).

#### Representative exact raw response

The following is the exact 1,055-byte minified response body emitted by the real `NewServer` path for a successful runtime call negotiated as MCP `2025-11-25` against the fixed selected-target test status:

```json
{"jsonrpc":"2.0","id":2,"result":{"content":[{"type":"text","text":"capability: loomspan.runtime-status.v1\ncapability: loomspan.skill-inspection.v1\ncapability: loomspan.active-execution-inspection.v1\ncapability: loomspan.recent-activity-inspection.v1\ncapability: loomspan.trace-inspection.v1\ncapability: loomspan.raw-artifact-inspection.v1\ntargetSelection: SELECTED\ntargetConnection: REACHABLE\ntargetAuthentication: ESTABLISHED\njavaGoCompatibility: COMPATIBLE\nruntimeIdentity: ESTABLISHED\nliveMonitoring: AVAILABLE\nobservedAt: 2026-08-13T20:00:00Z"}],"structuredContent":{"capabilities":["loomspan.runtime-status.v1","loomspan.skill-inspection.v1","loomspan.active-execution-inspection.v1","loomspan.recent-activity-inspection.v1","loomspan.trace-inspection.v1","loomspan.raw-artifact-inspection.v1"],"status":{"javaGoCompatibility":"COMPATIBLE","liveMonitoring":"AVAILABLE","observedAt":"2026-08-13T20:00:00Z","runtimeIdentity":"ESTABLISHED","targetAuthentication":"ESTABLISHED","targetConnection":"REACHABLE","targetSelection":"SELECTED"}}}}
```

For the same fixed status under MCP `2026-07-28`, the real response was 1,178 bytes. The SDK added response `_meta.io.modelcontextprotocol/serverInfo` and `resultType: "complete"`; the Loomspan-authored text and SDK-populated structured value remained present.

### 3. MCP specification and SDK rules

The current supported revision behavior is documented by the official MCP specification and pinned SDK:

- MCP `2025-11-25` defines `CallToolResult.content` as required and `structuredContent` as optional. The tool documentation describes `outputSchema` as optional, requires a returned structured result to conform when it is present, and says structured results should also return serialized JSON text for backward compatibility. The same rule was introduced in the official [`2025-06-18` tools specification](https://modelcontextprotocol.io/specification/2025-06-18/server/tools#structured-content) and remains applicable to `2025-11-25`.
- MCP `2026-07-28` widens `structuredContent` from an object to any JSON value conforming to the optional output schema, while retaining the backward-compatible text recommendation. See the official [`2026-07-28` tools specification](https://modelcontextprotocol.io/specification/2026-07-28/server/tools#structured-content).
- The `2026-07-28` tool capability advertises tool support/list-change behavior; it does not negotiate result representation. The official page lists the capability and output-schema semantics but no structured-only/text-only consumer flag.
- go-sdk v1.7.0 supports `2026-07-28`, `2025-11-25`, and earlier revisions according to its [version compatibility table](https://github.com/modelcontextprotocol/go-sdk/blob/v1.7.0/README.md#version-compatibility).
- Loomspan advertises an object-shaped output schema for every installed tool. Therefore, both supported revisions can carry the same structured envelope shape.

Under these rules:

- A successful `CallToolResult` must contain `content`, although the array can be empty at the schema level.
- `structuredContent` is optional at the base protocol level.
- Once Loomspan advertises `outputSchema` and returns structured content, that content must conform.
- Backward-compatible text duplication is a specification `SHOULD`, not a negotiated per-client selection.
- The current typed SDK API makes structured output automatic and text fallback automatic when the handler leaves `Content` nil.

### 4. Current fallback guarantees and representation differences

#### Runtime

`runtimeText` writes every capability and each status dimension in fixed order. It has no trailing JSON object or untrusted diagnostic content (`loomspan-console/internal/mcpadapter/runtime.go:62`). The structured value contains the same facts.

#### Execution list and detail

`executionListText`, `executionDetailText`, and `appendExecutionText` flatten every current `executionDTO` field: identifiers, canonical sequence, timestamps, elapsed time, entry skill, status, phase, summary, active path, depth/truncation, usage, and configured limits (`loomspan-console/internal/mcpadapter/executions.go:112`, `loomspan-console/internal/mcpadapter/executions.go:124`, `loomspan-console/internal/mcpadapter/executions.go:132`). The list adds `count`, `hasMore`, and continuation. These are complete deterministic orientation fallbacks for the current DTO.

#### Activity

The structured `activityDTO` requires `Details any` (`loomspan-console/internal/mcpadapter/contracts.go:101`, `loomspan-console/internal/mcpadapter/contracts.go:114`). `mapActivity` decodes it with `UseNumber`, preserving large integers and decimals. `activityText` renders result timing, returned range, continuity/reset, coverage, pagination, and every item's core identity/status/frame/summary facts, but it never renders `Details` (`loomspan-console/internal/mcpadapter/activity.go:95`). The golden test explicitly verifies that an instruction-like detail is absent from text while the summary remains (`loomspan-console/internal/mcpadapter/activity_test.go:22`).

Consequently, activity's two representations overlap on orientation facts but are not complete duplicates: structured content is the only representation containing the arbitrary untrusted `details` object.

#### Trace tools

Trace text uses purpose-specific compact lines. `fallbackField` bounds an individual displayed string at 512 bytes, so structured output can retain longer exact identifiers while text remains bounded (`loomspan-console/internal/mcpadapter/traces.go:531` and the helper later in that file). Record text includes descriptors and selected inline content/omission facts; range text repeats the returned bounded content exactly. The response budget admits an item based on the combined structured JSON and JSON-escaped fallback-line cost (`loomspan-console/internal/mcpadapter/response_budget.go:35`).

#### Generic guarantee

The README documents exactly one structured envelope arm for all twelve tools and a deterministic, fact-complete text fallback for consumers that do not consume structured results (`loomspan-console/README.md:280`, `loomspan-console/README.md:284`). The canonical Agent Skill relies on facts and workflows rather than naming either wire representation; it starts with runtime discovery and says tools are the complete MCP path (`loomspan-console/agent-skills/loomspan/SKILL.md:16`, `loomspan-console/agent-skills/loomspan/SKILL.md:64`).

### 5. Deterministic size measurements

#### Method

Measurements used minified UTF-8 JSON as emitted by Go's `encoding/json` and the selected SDK types. They contain no credentials, YAML, prompt/model content, raw trace artifacts, or arbitrary real diagnostic content.

- The runtime raw responses were sent through the real `NewServer`, security handler, SDK, and Streamable HTTP transport for both supported revisions.
- Small execution/activity/trace values came from committed MCP goldens or deterministic test DTOs. Their `content`, `structuredContent`, and MCP `2025-11-25` JSON-RPC framing were serialized through `mcp.CallToolResult` using the same SDK wire type.
- Maximum execution and activity shapes reproduce the existing 64-item tests. Activity uses the existing test's 11 KiB synthetic details payload per item.
- Trace maximum/default measurements are existing checked-in test assertions from `response_budget_test.go`, including actual production HTTP calls.
- "Text wire contribution" is `dual result bytes - result bytes with content: []`; it includes the text content object and escaping.
- "Structured wire contribution" is `dual result bytes - content-only result bytes`; it includes the `structuredContent` property and value.

#### Small and active-execution measurements

| Result shape | Fallback UTF-8 | Structured JSON | Reconstructed 2025 JSON-RPC | Text wire contribution | Structured wire contribution |
| --- | ---: | ---: | ---: | ---: | ---: |
| Runtime no-target golden | 495 | 486 | 1,087 | 532 | 507 |
| Execution list, 1 item | 1,168 | 787 | 2,101 | 1,245 | 808 |
| Execution detail, 1 item | 1,168 | 783 | 2,094 | 1,242 | 804 |
| Activity, 1 item | 1,178 | 1,019 | 2,376 | 1,288 | 1,040 |
| Trace summary, small failed trace | 309 | 743 | 1,164 | 352 | 764 |
| Domain failure, `NOT_FOUND` | 62 | 107 | 278 | 87 | 128 |

The separately observed real selected-target runtime response was 1,055 bytes for `2025-11-25` and 1,178 bytes for `2026-07-28`; its facts differ in length from the no-target golden in the table.

#### Maximum legal active pages

| Result shape | Fallback UTF-8 | Structured JSON | Reconstructed 2025 JSON-RPC | Text wire contribution | Structured wire contribution |
| --- | ---: | ---: | ---: | ---: | ---: |
| Execution list, 64 complete items | 71,783 | 45,858 | 120,687 | 74,760 | 45,879 |
| Activity, 64 complete items with 11 KiB details each | 25,988 | 743,756 | 772,064 | 28,239 | 743,777 |

The current execution and activity tools enforce a page-size maximum of 64 but do not use the trace response-byte admission budget. Their maximum-page tests protect all 64 complete items rather than byte-limited admission (`loomspan-console/internal/mcpadapter/executions_test.go:84`, `loomspan-console/internal/mcpadapter/activity_test.go:161`).

#### Existing trace budget measurements

`defaultTraceResultBudget` is 32 KiB and `defaultRangeResultBudget` is 48 KiB (`loomspan-console/internal/mcpadapter/response_budget.go:8`). The checked-in synthetic full-result test records:

| Synthetic complete result | Bytes |
| --- | ---: |
| Inventory | 21,091 |
| Detailed frames | 27,035 |
| Compact frames | 26,886 |
| Record descriptors | 25,440 |
| Inline records | 17,337 |
| Semantic text range | 45,527 |
| Raw base64 range | 36,007 |

The checked-in real HTTP budget test records these separate fixture shapes:

| Full HTTP response | Bytes |
| --- | ---: |
| Inventory | 12,012 |
| Compact frames | 20,872 |
| Detailed frames | 20,751 |
| Record descriptors | 20,077 |
| Inline records | 18,023 |
| Search | 20,624 |
| Semantic text range | 29,225 |
| Raw base64 range | 26,177 |

Those two trace tables use different deterministic fixture shapes and are not before/after measurements of the same response. Both include structured and text forms; the HTTP table also includes JSON-RPC framing. The documentation records the stable ceilings rather than the change-sensitive exact values (`loomspan-console/docs/mcp-contract-verification.md:18`).

### 6. Text as orientation/index versus the current fallback contract

The present implementation already uses two fallback styles:

- Active execution text is fact-complete for every structured orientation field.
- Activity and trace text are safety/boundedness projections: they preserve the protected workflow facts but omit or bound content that remains available structurally.

Therefore, a compact orientation/index is technically expressible in the current adapter—the fallback functions already independently author text—but changing execution/runtime text from fact-complete to index-only would change documented behavior and current golden/field-presence expectations. For activity, the current text already behaves as a safe orientation projection while structured output remains complete. For range reads, omitting the returned content from text would change the currently protected exact text-fallback retrieval path.

This is a contract distinction rather than an SDK limitation. The SDK accepts Loomspan-authored text of any deterministic shape, but it does not provide client capability information that would allow that shape to vary between structured-capable and text-only consumers.

### 7. Capability-based selection and divergent contracts

No current MCP field tells Loomspan that a caller can safely consume only `structuredContent` or only `content`. Loomspan's six capability identifiers describe its own runtime inspection surface, not client result-format support. The standard MCP capabilities passed by a client likewise do not include a result-representation selector.

The current server consequently has one result contract for all clients at a negotiated protocol revision. Selecting a representation would require information outside the existing protocol contract. Named-client detection or downstream presentation heuristics do not exist in the adapter.

### 8. Errors and failures

There are three distinct paths:

1. **Loomspan domain failure:** `domainFailure` authors concise text, `isError: true`, and a typed `{"error": ...}` envelope (`loomspan-console/internal/mcpadapter/contracts.go:160`). `mapDomainError` exposes only reviewed details fields. The safety test verifies that the internal cause/path is absent (`loomspan-console/internal/mcpadapter/contracts_test.go:58`). A measured `NOT_FOUND` response is 278 bytes.
2. **Invalid tool arguments:** SDK validation returns an error tool result with `isError` and no Loomspan `structuredContent`; `server_test.go` protects that distinction (`loomspan-console/internal/mcpadapter/server_test.go:250`).
3. **Protocol, authentication, cancellation, or internal publication failure:** these remain SDK/JSON-RPC, HTTP, or suppressed-call failures rather than Loomspan domain envelopes. Cancellation and changed authentication generation suppress publication.

The exact result/error one-arm invariant is tested in `contracts_test.go:21`, and every representative success/domain-error envelope is validated against both compact discovery and complete typed schemas in `output_schemas_test.go:93`.

### 9. Existing conformance, snapshots, and deterministic tests

Current protection includes:

- Exact `tools/list` raw response snapshot and 25 KiB discovery ceiling; its current expected size is 25,588 bytes (`loomspan-console/internal/mcpadapter/server_test.go:31`, `loomspan-console/internal/mcpadapter/server_test.go:34`).
- SDK black-box calls requiring non-nil `StructuredContent` and exactly one text item for runtime, skills, and maximum activity (`loomspan-console/internal/mcpadapter/server_test.go:220`, `loomspan-console/internal/mcpadapter/activity_test.go:235`).
- Exact structured goldens for runtime, skills, execution list/detail, and activity in `internal/mcpadapter/testdata/`.
- Deterministic text order, JSON-compatible escaping, final newline, safe error text, and one-arm envelope tests (`loomspan-console/internal/mcpadapter/contracts_test.go`).
- Execution and activity maximum-page completeness tests (`loomspan-console/internal/mcpadapter/executions_test.go:84`, `loomspan-console/internal/mcpadapter/activity_test.go:161`).
- Complete typed output plus compact advertised-schema validation for every installed tool (`loomspan-console/internal/mcpadapter/output_schemas_test.go:93`).
- Exact combined structured/text synthetic budgets and actual HTTP trace-result budgets (`loomspan-console/internal/mcpadapter/response_budget_test.go:91`, `loomspan-console/internal/mcpadapter/response_budget_test.go:187`).
- Browser/application-to-MCP fact parity for active execution, recent continuity/coverage, and domain meanings (`loomspan-console/internal/mcpadapter/parity_test.go:46`, `loomspan-console/internal/mcpadapter/parity_test.go:69`, `loomspan-console/internal/mcpadapter/parity_test.go:114`).
- A pinned official MCP conformance harness for `2025-11-25` initialization/listing and `2026-07-28` listing, caching, and DNS-rebinding protection (`loomspan-console/internal/buildtool/mcp_conformance.go:107`). It does not contain a fixture-specific dual-output conformance scenario.

Fresh verification for this research:

- Focused adapter measurement/budget tests passed.
- `go test ./...` passed from `loomspan-console/`.
- Temporary measurement tests were removed after execution; the only new artifact from this research is this document.

### 10. Ownership layer and smallest current code delta surface

The common Loomspan-owned construction point for all non-runtime success/domain results is `successResult`/`domainFailure` in `contracts.go`; runtime has the analogous direct construction in `runtime.go`. The common structured-output handoff and schema validation point is `addValidatedTool` in `output_schemas.go`. The SDK owns population of `CallToolResult.StructuredContent`, JSON schema application, protocol-revision result metadata, and JSON-RPC encoding.

Current behavior at those boundaries is:

- Leaving typed `Out` in place preserves complete structured output.
- Leaving Loomspan `Content` nil causes SDK-generated serialized JSON text, not structured-only output.
- Supplying Loomspan text prevents SDK JSON-text synthesis for the object-shaped results used here.
- Removing typed `Out` or its output schema would change complete structured-output validation and discovery.
- Changing only client presentation cannot be implemented or verified in this repository because the connector/client is downstream of the raw response.

Thus, the smallest source locations differ by ownership question: `contracts.go`/`runtime.go` own the authored fallback, `output_schemas.go` and go-sdk own structured population and validation, and no Loomspan production source owns downstream model presentation.

## Architecture Documentation

### Data flow and boundaries

```text
Java application REST/SSE facts
        |
        v
Go applicationclient -> observability/live/trace services
        |
        v
mcpadapter DTO + deterministic fallback text        (Loomspan-owned)
        |
        v
addValidatedTool complete validation + compact schema (Loomspan-owned)
        |
        v
go-sdk typed Out -> structuredContent                (SDK-owned)
        |
        v
Streamable HTTP + JSON-RPC/result metadata           (SDK/transport-owned)
        |
        v
connector/client/model presentation                  (outside repository evidence)
```

Execution list/detail call `observability.Service.ListActiveExecutions` and `GetActiveExecution` (`loomspan-console/internal/observability/service.go:98`, `loomspan-console/internal/observability/service.go:128`). Activity calls `live.Service.Recent` (`loomspan-console/internal/live/service.go:681`). Application endpoint construction remains `/active-executions`, `/active-executions/{sessionId}`, and `/activity` (`loomspan-console/internal/applicationclient/address.go:134`, `loomspan-console/internal/applicationclient/address.go:140`). Those boundaries do not construct either MCP representation.

### Contract classification

Using the repository's exact framework-design categories:

| Category | Current classification and evidence |
| --- | --- |
| Application API | No affected Java application-facing API. No type under `com.lokiscale.loomspan.api` participates. |
| Supported SPI | None. No Java SPI, MCP result-replacement SPI, or supported bean-replacement surface exists. |
| Configuration and manifest contracts | MCP endpoint/key setup and the canonical Agent Skill remain documented, but neither currently declares a text/structured selection. No YAML skill syntax or `loomspan.*` property is involved. |
| Persisted or serialized contracts | MCP tool discovery/results are serialized cross-process protocol messages, deliberately supported as a coherent pre-v1 Console diagnostic contract. They are not persisted client-result records in this repository. Java REST/SSE fixtures are a separate protected serialized adapter boundary and are unchanged by representation construction. |
| Ephemeral diagnostic formats | Tool result facts, active execution/activity values, and trace projections are current-run diagnostic formats. Their supported MCP envelope, fallback, schemas, errors, and limits move coherently pre-v1. |
| Internal or accidentally exposed implementation | All Go types/functions under `internal/mcpadapter`, including exported `NewServer` and `ServerOptions`, are internal Go implementation. Java observability web/controller DTOs are internal integration machinery, not application API or SPI. |

### Public declarations, constructors, beans, fixtures, and usage

- **Public declarations/interfaces/constructors:** No supported Java public-surface delta. Go `mcpadapter.NewServer`, `ServerOptions`, and service interfaces are technically exported but package-internal under Go's `internal` import rule.
- **Spring beans and `@ConditionalOnMissingBean`:** None are involved in MCP result construction.
- **Tests and fixtures:** MCP goldens, Java application REST/SSE fixtures, schema tests, parity tests, max-page tests, size tests, and conformance harnesses are listed above.
- **Documentation:** `loomspan-console/README.md` documents the envelope and fact-complete text guarantee. `loomspan-console/docs/mcp-contract-verification.md` documents protocol revisions, combined-result budgets, and the client-neutral verification boundary.
- **Configuration/manifests:** `go.mod` pins SDK v1.7.0. The conformance `package-lock.json` pins the official runner revision. The canonical Agent Skill is under `loomspan-console/agent-skills/loomspan/`.
- **Serialized formats:** MCP `tools/list` and `tools/call`, internal Java-to-Go REST/SSE fixtures, and ephemeral trace/activity DTOs. No persisted client presentation record exists.
- **Verified in-repository use:** `internal/console/service.go` constructs `mcpadapter.NewServer`; browser and MCP adapters consume shared services but serialize different outer wrappers.

## Code References

- `loomspan-console/go.mod:8` — Selected official Go SDK v1.7.0.
- `loomspan-console/internal/mcpadapter/contracts.go:23` — Generic result/error envelope.
- `loomspan-console/internal/mcpadapter/contracts.go:101` — Activity structured DTO, including untrusted `details`.
- `loomspan-console/internal/mcpadapter/contracts.go:155` — Successful deterministic text plus typed envelope construction.
- `loomspan-console/internal/mcpadapter/contracts.go:160` — Structured/text domain-failure construction.
- `loomspan-console/internal/mcpadapter/output_schemas.go:17` — Compact output schema, complete validation, and typed SDK registration.
- `loomspan-console/internal/mcpadapter/server.go:54` — Twelve-tool server assembly.
- `loomspan-console/internal/mcpadapter/server.go:65` — Stateless JSON Streamable HTTP configuration.
- `loomspan-console/internal/mcpadapter/executions.go:112` — Execution-list text fallback.
- `loomspan-console/internal/mcpadapter/executions.go:132` — Complete flattened execution facts.
- `loomspan-console/internal/mcpadapter/activity.go:95` — Activity safe orientation fallback.
- `loomspan-console/internal/mcpadapter/response_budget.go:8` — Trace/range result ceilings.
- `loomspan-console/internal/mcpadapter/response_budget_test.go:91` — Exact combined-result synthetic measurements.
- `loomspan-console/internal/mcpadapter/response_budget_test.go:187` — Actual HTTP trace-result measurements.
- `loomspan-console/internal/mcpadapter/server_test.go:31` — Raw discovery and compatible 2025 call coverage.
- `loomspan-console/internal/mcpadapter/output_schemas_test.go:93` — Representative compact/complete schema validation for all tools.
- `loomspan-console/README.md:280` — Supported envelope, error, and fallback description.
- `loomspan-console/docs/mcp-contract-verification.md:18` — Combined response-budget methodology.
- `loomspan-console/agent-skills/loomspan/SKILL.md:64` — Canonical tools-only runtime-debugging workflow.

## Historical Context (from ai/thoughts/)

- `ai/thoughts/tickets/loomspan-console-pr-35-mcp-dual-output-efficiency-and-presentation-boundaries.md` — Current proposed ticket. It treats the Codex Desktop observation as a lead and narrows repository evidence to server-owned protocol behavior.
- `ai/thoughts/phases/2026-08-15-loomspan-llm-trace-understanding-roadmap.md:71` — Records PR 34 active-execution/activity semantics as completed inputs to this ticket.
- `ai/thoughts/phases/2026-08-15-loomspan-llm-trace-understanding-roadmap.md:80` — Records deterministic repository verification rather than client/model usability experiments as the current boundary.
- Git history for the old tracked `loomspan-console-pr-35-mcp-response-efficiency.md` contains a broader client-presentation version of the ticket. The current requested ticket removes client/model usability and client-event measurements as repository gates and focuses on server ownership and deterministic protocol evidence.

## Related Research

No prior document under `ai/thoughts/research/` exists in the current checkout. The active roadmap and current PR 35 ticket are the relevant historical/context artifacts.

## Follow-up Research 2026-08-21 23:03 PDT

The maintainer started Loomspan Console and one execution so the configured
Loomspan MCP connection could be inspected from Codex while following the
runtime-debugging skill. No YAML, model/tool content, raw artifacts, or
arbitrary activity detail values were emitted into this document.

Runtime discovery at `2026-08-22T06:01:32.7111047Z` returned all five required
capabilities and the optional raw-artifact capability. Target selection,
connection, authentication, Java/Go compatibility, runtime identity, and live
monitoring were all available. Inspecting the Codex-side returned MCP object
showed both a one-item `content` array and `structuredContent`; this confirms
that both server-emitted representations survive into the callable MCP result
available to Codex's orchestration layer.

The presentation layer can choose what to forward further. In the first
runtime call, the local orchestration script forwarded only returned text
content, so only that text appeared in the tool output. When the script instead
serialized the whole returned MCP object, both `content` and
`structuredContent` appeared. This demonstrates a real distinction between
the MCP return object and the narrower tool output selected by a client-side
wrapper.

### Live measurements

| Observation | Text UTF-8 bytes | Structured JSON bytes | Codex-side returned object bytes | Structural overlap |
| --- | ---: | ---: | ---: | --- |
| Active list, 1 execution (`06:02:03Z`) | 2,281 | 1,515 | 3,962 | Same execution orientation facts in both forms |
| Active detail (`06:02:19Z`) | 2,290 | 1,502 | 3,955 | 48 of 49 text lines exactly matched all 48 structured leaf values; the extra line was `activePath.count` |
| Activity, 8 items (`06:02:38Z`) | 4,811 | 4,819 | 9,998 | 108 of 110 text lines exactly matched structured leaves; structured output had 23 additional `details` leaves |
| Finalized trace summary (`06:03:07Z`) | 1,202 | 1,521 | 2,861 | Both representations present; content was not opened |

The active execution was `sessionId`
`d6c09173-b647-45e6-a165-36458ed4566c`, with `traceId`
`938cdfb4-76cf-457a-b3c2-ee3ea6757e33`. Because it was still running, the
observations are provisional and not one atomic snapshot: the initial active
list reported latest canonical sequence 65, detail reported 103, and the
eight-item activity page reached sequence 141. The activity call returned
cursors 49 through 56, `hasMore: false`, and a reusable continuation.

The execution completed before a second active-list measurement. Resolving the
already-returned `traceId` then reported a finalized `FAILED` trace with 184
records, 32 frames, 11 attempts, zero validated later-attempt retries, one
failure, and complete usage. Failure records or diagnostic content were not
opened because they were unrelated to the dual-output question.

A second, independently started execution reproduced the active-detail result
at `2026-08-22T06:05:01.4757335Z`. The `handleIncident` execution was active in
the model phase at canonical sequence 80. Its fallback contained 49 flattened
lines and 2,326 characters; its structured JSON contained 1,538 characters;
and the combined Codex-side returned object contained 4,027 characters. Visual
comparison of the returned object showed the same execution fields, six-frame
active path, usage, and configured limits in both encodings, with the fallback
again adding the derived `activePath.count` line. This corroborates the first
active-detail measurement across a different entry skill and execution shape.

### Follow-up conclusion

This observation confirms the motivating phenomenon more directly than the
original ticket wording: Codex's callable MCP result receives both
server-emitted representations, and active detail is effectively a complete
second encoding of the same leaf facts. Activity also carries substantial
overlap, while retaining a meaningful boundary because arbitrary `details`
exist only in structured content. The observation does not establish that
every Codex host path automatically places both forms into model context; the
local wrapper demonstrably can forward only one selected form. It does
establish that the dual object reaches the Codex-side integration boundary and
that a wrapper which forwards the entire object will expose the duplication.

## Open Questions

1. The live Codex-side call established that both representations reach the callable MCP result, and a local wrapper established that it can select only one for subsequent output. The repository still cannot establish what every downstream connector/client presents or suppresses, and no client event store was inspected; the ticket classifies that broader presentation behavior as contextual rather than a repository gate.
2. The MCP backward-compatibility `SHOULD` and Loomspan's documented fact-complete fallback are known constraints; the eventual pre-v1 contract choice about retaining, shortening, or otherwise shaping authored text is not made by the current codebase.
3. Checked-in raw HTTP size assertions cover trace navigation/range calls. Active-execution and activity raw sizes are protected structurally and by maximum-item tests, while the byte measurements in this document are research measurements rather than committed gates.
4. The canonical repository skill does not distinguish `content` from `structuredContent`. Whether a future contract change would create a user-facing workflow difference is not established by current skill text.
