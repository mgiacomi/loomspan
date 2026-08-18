# LLM-Facing MCP Trace Interface Cleanup

## Status

Proposed first-pass implementation ticket, prepared 2026-08-18 for execution
in a fresh coding context.

This ticket implements accepted ledger items `TRACE-IF-001` through
`TRACE-IF-006` in
[`loomspan_skill_mcp_questions.md`](../phases/loomspan_skill_mcp_questions.md).
It deliberately stops before the remaining candidates so the next design pass
can be informed by live MCP walkthroughs rather than static speculation.

## Outcome

Make the Loomspan MCP interface describe developer intent and trace evidence,
not Console storage and ownership machinery.

The ordinary finalized-trace workflow becomes:

```text
list traces
  -> select traceId
  -> get/query/read using traceId
```

The LLM does not supply, retain, compare, or branch on `sourceFilter`, `source`,
`TARGET`, `IMPORTED`, `artifactHandle`, `targetScopeId`, or `instanceId`.
Console continues to use target scopes, runtime generations, evidence owners,
artifact handles, leases, acquisition, cache state, and expiry internally to
preserve all existing safety guarantees.

## Why this work is being done now

The pre-alpha MCP implementation is mechanically capable but exposes its
internal lifecycle as required model work. A model must currently:

1. choose `TARGET` or `IMPORTED` even when the developer only asked for a
   trace;
2. acquire a `traceId`, retain the returned `artifactHandle`, and switch every
   downstream call to that handle;
3. interpret target scope, runtime instance, catalog, availability, cache,
   retention, expiry, size, and resource-link fields; and
4. recover from storage-lifecycle errors that Console can usually resolve
   itself.

These branches do not represent ordinary developer intent. They increase tool
schema size, failed-call risk, prompt state, token consumption, and dependence
on frontier-model inference. Loomspan has not released an alpha version, so
this is the right time to correct the outward development contract without a
compatibility shim. Internal evidence ownership must not be weakened.

After this ticket, a representative LLM will be connected to the actual MCP
server and used to investigate several traces. That exercise will decide later
changes concerning session identity, content references, pagination, activity
continuity, physical records, duplicate result representations, and method or
resource consolidation.

## Authoritative product decisions

The following directions are settled for this ticket:

- `traceId` is the normal LLM-facing identity for finalized-trace discovery and
  every trace inspection operation.
- `sourceFilter`, `source`, and `TARGET`/`IMPORTED` routing distinctions are not
  part of the LLM-facing MCP contract.
- `artifactHandle` is not an MCP parameter or return field. It remains an
  internal Console lookup and lease mechanism.
- `targetScopeId` and `instanceId` are not MCP parameters, success fields,
  error fields, activity fields, or resource identities. Console still
  enforces their safety semantics internally.
- `resourceUri` and `resources` are removed. Existing custom MCP resource
  templates also expose the rejected identity model and duplicate complete
  tool paths; remove their registration in this first-pass surface. A future
  resource method may be proposed only after it independently passes the
  method test.
- Trace inventory removes catalog, availability, cache, acquisition, retention,
  expiration, and size mechanics.
- A partial trace-discovery result reports a compact domain-level completeness
  limitation. It does not expose which internal inventory segment failed.
- A genuine collision between distinct evidence instances claiming the same
  `traceId` is explicit. Console never silently selects one merely to keep the
  simple interface.
- Internal target changes, artifact expiry, stale continuations, and stale
  content references remain enforced. Their MCP errors explain the caller's
  domain-level recovery action without asking the LLM to repair internal
  ownership state.

## Interface burden-of-proof rules

Apply these rules to every schema touched by the work.

### Method test

1. What developer intent does the tool represent?
2. Why can an existing composable tool not handle it?
3. Does it expose a domain operation or an implementation step?
4. Would an LLM naturally know when to call it?
5. Does it reduce total interface complexity?

### Parameter test

A parameter changes caller intent or materially constrains evidence. Ordinary
behavior is the default; the LLM does not supply parameters merely to restate
normal operation.

### Return-field test

A field changes the LLM's next decision or final answer.

## Current implementation map

The coding context should verify these locations before editing because the
repository may have moved since this ticket was written.

### MCP assembly and contracts

- `loomspan-console/internal/mcpadapter/server.go` registers twelve tools plus
  one skill and six trace resource templates.
- `loomspan-console/internal/mcpadapter/contracts.go` defines shared success and
  error envelopes, skill/execution/activity DTOs, and repeated
  `targetScopeId`/`instanceId` fields.
- `loomspan-console/internal/mcpadapter/runtime.go` returns the internal
  `consolecore.StatusSnapshot` directly, including target scope and instance.
- `loomspan-console/internal/mcpadapter/skills.go` returns `resourceUri` plus
  target scope and instance metadata.
- `loomspan-console/internal/mcpadapter/executions.go` returns target scope and
  instance metadata.
- `loomspan-console/internal/mcpadapter/activity.go` returns target scope,
  instance, per-item instance, and internal continuity ownership fields.
- `loomspan-console/internal/mcpadapter/trace_contracts.go` defines all current
  trace input/output DTOs.
- `loomspan-console/internal/mcpadapter/traces.go` resolves `source`, maps the
  broad inventory, returns artifact-backed evidence identities, and constructs
  trace resource links.
- `loomspan-console/internal/mcpadapter/resources.go` implements the
  target-scope skill resource template.
- `loomspan-console/internal/mcpadapter/trace_resources.go` implements six
  source/handle-based trace resource templates.

### Internal evidence lifecycle that must remain

- `loomspan-console/internal/evidence/owner.go` distinguishes target and
  imported ownership internally.
- `loomspan-console/internal/artifact/handle.go` creates opaque random handles.
- `loomspan-console/internal/artifact/service.go` owns target-scoped/imported
  entries, lookup, leases, expiry, acquisition, and target-change checks.
- `loomspan-console/internal/artifact/import.go` validates and installs imported
  traces under an internal owner.
- `loomspan-console/internal/traceinventory/service.go` joins installed and
  application-catalog evidence and currently exposes its segment state.
- `loomspan-console/internal/traceanalysis/service.go` and query services use an
  internal evidence reference plus artifact handle to acquire a lease.
- `loomspan-console/internal/traceanalysis/cursor.go` binds opaque
  continuations to internal owner/handle state.
- `loomspan-console/internal/traceanalysis/content_ref.go` currently binds
  opaque payload/diagnostic references to internal source and handle state.

Do not replace those internal guarantees with a global trace-ID-to-path map,
trust imported trace IDs as filesystem identity, make handles durable, or
allow evidence from different owners or target generations to mix.

## Required MCP tool inputs

Tool names remain unchanged in this ticket. Change the trace schemas as
follows.

### `LOOMSPAN_list_traces`

```json
{
  "pageSize": 10,
  "continuation": "opaque-if-continuing"
}
```

- Remove `sourceFilter`.
- Keep `pageSize` and `continuation` behavior unchanged for this ticket.
- A zero/omitted internal filter means the unified inventory.

### `LOOMSPAN_get_trace`

```json
{
  "traceId": "9f0a67b3-2955-4240-b7c1-c5e3263c1f94"
}
```

- Require one nonblank bounded `traceId`.
- Remove `source`, `artifactHandle`, and the current exactly-one branch.
- Resolve reusable or acquirable evidence internally.

### `LOOMSPAN_query_trace_frames`

```json
{
  "traceId": "...",
  "filter": {},
  "order": "CANONICAL",
  "pageSize": 10,
  "continuation": "opaque-if-continuing"
}
```

- Replace required `source` plus `artifactHandle` with required `traceId`.
- Preserve existing filters, order, bounds, and continuation behavior.

### `LOOMSPAN_query_trace_records`

```json
{
  "traceId": "...",
  "filter": {},
  "representation": "PHYSICAL",
  "inlinePayload": false,
  "pageSize": 10,
  "continuation": "opaque-if-continuing"
}
```

- Replace required `source` plus `artifactHandle` with required `traceId`.
- Preserve current record filters, representation, inline-payload, bounds, and
  continuation behavior. Logical-default/physical-record redesign is a later
  candidate, not part of this ticket.

### `LOOMSPAN_read_trace_payload`

```json
{
  "traceId": "...",
  "payloadRef": "opaque-returned-reference",
  "start": 0,
  "maxBytes": 4096
}
```

- Replace required `source` plus `artifactHandle` with required `traceId`.
- Preserve `payloadRef`, `start`/`continuation`, and `maxBytes` for this ticket.
- Do not redesign `payloadRef` yet. If its internal artifact binding is stale,
  return a focused error telling the caller to refresh the record descriptor;
  never mention or request an artifact handle.

### `LOOMSPAN_read_trace_artifact`

```json
{
  "traceId": "...",
  "start": 0,
  "maxBytes": 4096
}
```

- Replace required `source` plus `artifactHandle` with required `traceId`.
- Preserve the explicit forensic/raw purpose and current range controls.

## Required trace-ID resolution behavior

Introduce or reuse one internal orchestration seam that resolves `traceId` to
the evidence reference and artifact handle required by trace analysis. Do not
duplicate resolution rules independently in each handler.

The seam is internal and may return owner/reference/handle/scope facts to other
Console code. None appear in the MCP schema.

Required behavior:

1. Validate the supplied `traceId` before lookup or acquisition.
2. Consider usable evidence already installed under the current target owner
   and under the process-local imported owner.
3. Reuse a unique usable installed instance without making the LLM reacquire it.
4. When no usable copy is installed and the selected target can provide the
   trace, acquire and validate it through the existing artifact service.
5. Permit a uniquely matching imported trace to be inspected when no target is
   selected or the target cannot provide that trace.
6. Preserve current target-capture and publication checks so target rotation
   before, during, or after resolution cannot publish mixed evidence.
7. Preserve one immutable lease per underlying analysis call.
8. If distinct installed/acquirable evidence owners claim the same `traceId`,
   return `AMBIGUOUS_TRACE` (or an equivalently explicit stable domain code).
   Do not silently prefer target or imported evidence and do not reintroduce a
   required source parameter. The message should tell the developer that the
   conflicting trace identity must be resolved in Console before inspection.
9. If internal evidence expires, attempt only the safe transparent reuse or
   target reacquisition already permitted by the artifact lifecycle. If the
   operation still cannot proceed, return a trace/evidence-unavailable error,
   not `ARTIFACT_EXPIRED` plus an artifact-handle recovery ceremony.
10. A continuation remains opaque and query-bound. If its internal owner or
    handle no longer matches resolved evidence, return a stale/invalid
    continuation error instructing the LLM to restart that query by `traceId`.
11. A stale `payloadRef` returns a content-reference error instructing the LLM
    to re-query the relevant record by `traceId`. Its current encoding may
    remain handle-bound until the separate content-reference decision.

The implementation may add internal `consolecore.Code` values such as
`AMBIGUOUS_TRACE` or `TRACE_UNAVAILABLE`. `consolecore` is internal API, but
tests, tool descriptions, skill guidance, and documentation must agree on any
new outward domain code.

## Required trace inventory result

Return compact candidates. The outward shape should contain no owner, catalog,
cache, acquisition, retention, expiration, or size data.

```json
{
  "observedAt": "2026-08-18T12:00:00Z",
  "items": [
    {
      "traceId": "trace-1",
      "sessionId": "session-1",
      "entrySkill": "handleIncident",
      "outcome": "SUCCEEDED",
      "finalizedAt": "2026-08-18T11:59:00Z"
    }
  ],
  "complete": true,
  "hasMore": false
}
```

Allowed fields:

- top level: `observedAt`, `items`, `complete`, `limitations` when incomplete,
  `hasMore`, and `continuation` when present;
- item: `traceId`, `sessionId`, `entrySkill`, `outcome`, `finalizedAt`, and an
  optional `ambiguous: true` only when multiple evidence instances claim that
  identity.

Remove:

- `applicationCatalog` and all nested fields;
- `source` and `targetScopeId`;
- `sizeBytes` and `persistencePolicy`;
- `applicationTraceExpiresAt` and `applicationAvailability`;
- `localAvailable` and `artifactHandle`;
- `acquiredAt`, `lastUsedAt`, `localExpiresAt`, and `localBytes`.

`complete` is independent of pagination:

- `hasMore` means additional items remain in the same bounded selection;
- `complete` means every evidence family Console was expected to consult for
  this snapshot was available enough to support the selection;
- `limitations` explains why a negative or uniqueness conclusion is unsafe.

Use a compact limitation DTO containing only a stable domain code and safe
message unless a third field demonstrably changes the LLM's next decision. Do
not name internal target/imported segments or return internal errors nested
inside a successful result.

Inventory must not emit indistinguishable duplicate rows with the same
`traceId`. Consolidate them and set `ambiguous: true`, or otherwise guarantee a
single row whose ambiguity is explicit. Pagination must not split ownership
duplicates into apparently independent trace identities.

## Required trace-operation results

The existing neutral summaries, frames, records, ranges, usage, failures,
gaps, uncertainties, and continuations remain in scope. This ticket removes
only the rejected ownership/navigation fields.

- Replace `evidenceDTO` fields `source`, `targetScopeId`, and `artifactHandle`
  with a compact trace identity containing `traceId`, `sessionId`, and the
  currently retained observation fact.
- Remove `traceResourcesDTO` and `getTraceResult.resources`.
- Do not return a `resourceUri` substitute.
- Preserve summary/frame/record/range facts and bounded content behavior.
- Do not expose a new owner, cache key, installed-copy ID, URI, or trace
  reference under another name.

## Cross-cutting MCP return cleanup

Remove `targetScopeId` and `instanceId` from all LLM-visible MCP data while
retaining them internally.

### Runtime

Do not return `consolecore.StatusSnapshot` directly. Map it to an MCP DTO that
keeps only:

- observation time;
- target selected/none;
- connection state;
- authentication state;
- Java/Go compatibility state;
- runtime identity established/not established; and
- live-monitoring availability.

Keep capability IDs. Remove target scope and runtime instance IDs.

### Skills

- Remove top-level target scope and instance fields.
- Remove `resourceUri` from list and detail items.
- Keep registered name, current `sourcePath`, YAML, observation time, and
  pagination behavior. Moving `sourcePath` out of list results is a later
  candidate.

### Active executions

- Remove top-level target scope and instance fields.
- Keep `sessionId` and `traceId`; their final division of responsibility is an
  explicit post-cleanup candidate.
- Do not compact `list_executions` in this ticket even though its structured
  items currently duplicate detail; evaluate that with the live server.

### Recent activity

- Remove top-level target scope and instance fields.
- Remove per-item `instanceId`.
- Map internal continuity to an MCP-specific DTO that omits target scope and
  instance while retaining the current interval, cursor, reset, observation,
  and beginning-unavailable semantics.
- Do not otherwise redesign activity cursors or continuity in this ticket.

### Errors

- Remove `targetScopeId` from `domainErrorDTO`.
- Do not expose `currentTargetScopeId` or `transportCategory` through the MCP
  details object.
- Map internal `consolecore.Details` to an MCP-specific allowed subset. Keep
  details that affect recovery, such as compatibility versions, limit name and
  value, and raw-download availability.
- Preserve stable codes and safe messages. `TARGET_CHANGED` remains meaningful;
  it simply does not reveal or require scope IDs.

These changes are MCP adapter changes. Do not remove target scope, instance,
source, handles, retention, or storage facts from browser APIs or internal DTOs
that legitimately use them.

## MCP resources

Remove registration of the current custom resource templates from
`mcpadapter.Server`:

- `loomspan://targets/{targetScopeId}/skills/{skillName}`;
- all target trace templates containing `targetScopeId` and `artifactHandle`;
- all imported trace templates containing `artifactHandle`.

Remove their result links and update/delete tests that require seven resource
templates. Tools remain the complete portable path for the same evidence.

Do not replace them with `loomspan://traces/{traceId}` in this ticket. A future
resource surface must independently justify why it reduces total complexity
instead of duplicating `get_skill`, `get_trace`, frame, and record tools.

## Skill, documentation, and evaluation updates

Update all material shipped with or documenting the MCP server in the same
change. At minimum inspect and update:

- `loomspan-console/agent-skills/loomspan-runtime-debugging/SKILL.md`;
- `loomspan-console/agent-skills/loomspan-runtime-debugging/references/`;
- `ai/skill-authoring/traces-and-debugging.md`;
- `loomspan-console/README.md`;
- MCP schema/contract/parity tests;
- agent evaluation cases that currently require `targetScopeId`,
  `artifactHandle`, resource use, or `ARTIFACT_EXPIRED` recovery; and
- fixtures or browser fixtures copied from MCP results.

Guidance must teach:

```text
identify traceId -> inspect/query/read by traceId
```

It must not teach source selection, handle handoff, scope comparison, resource
URI navigation, or cache expiry recovery. It may explain direct domain errors,
continuation restart, incomplete discovery, trace ambiguity, and unavailable
evidence.

Update evaluation expectations such as:

- `evidence-expired.json`: expect transparent reacquisition or a domain-level
  trace-unavailable result, not `ARTIFACT_EXPIRED`/handle repair;
- `target-changed.json`: require `TARGET_CHANGED` and no evidence mixing, but
  do not require target scope or artifact-handle identifiers;
- `incompatible-target.json`, `target-authentication-required.json`, and
  `slow-execution.json`: remove target-scope identifier requirements while
  preserving the meaningful state/limitation;
- `mcp-without-skill.json`: remove resource assumptions; tools alone remain
  sufficient.

## Suggested implementation order

1. Capture the current tool list and schemas in tests so the intended removals
   are explicit rather than accidental.
2. Add the internal trace-ID resolution seam and unit-test target, imported,
   collision, unavailable, expiry, and target-change behavior without changing
   the outward handlers yet.
3. Change trace input schemas and handlers to call the resolver.
4. Compact trace results and inventory completeness/ambiguity behavior.
5. Introduce MCP-specific runtime, continuity, and error DTOs; remove repeated
   scope and instance fields across all tool families.
6. Remove resource links and unregister/delete the current resource templates.
7. Update tool descriptions and capability conformance tests.
8. Update the installed skill, reference guides, README, fixtures, and agent
   eval cases.
9. Run focused tests, full Go tests, applicable Java tests, and agent evals.
10. Inspect actual `tools/list` schemas and representative success/error JSON to
    ensure no rejected field survives under another wrapper.

## Required test matrix

### Contract tests

- No tool input schema contains `sourceFilter`, `source`, or `artifactHandle`.
- Every finalized trace operation requires `traceId`.
- No MCP-defined structured result or error property contains `targetScopeId`,
  `instanceId`, `artifactHandle`, the evidence-routing field `source`,
  `resourceUri`, or `resources`. Arbitrary untrusted YAML/record/payload content
  is data and is not recursively rewritten merely because it contains the same
  property spelling.
- Trace inventory contains none of the explicitly removed storage fields.
- The server advertises no current custom resource templates.
- Tool names and capability membership remain coherent.

Prefer semantic schema assertions over fragile substring checks, but add one
recursive forbidden-property test over every MCP-defined input/output schema
and representative envelope. Exclude arbitrary untrusted content values from
the property-name rule. The test should report the exact path of a contract
leak.

### Trace resolution tests

- Unique installed current-target evidence is reused.
- Unique imported evidence works without a selected target.
- Missing installed evidence is acquired from the selected target.
- An imported trace is usable when the target has no matching trace.
- A target/imported collision returns explicit ambiguity and never silently
  chooses an owner.
- Target rotation before publication returns `TARGET_CHANGED` without IDs.
- Internal expiry transparently reacquires when safe.
- Unrecoverable expiry returns trace/evidence unavailable without handle terms.
- Stale continuation tells the caller to restart the query by `traceId`.
- Stale payload reference tells the caller to refresh the record descriptor by
  `traceId`.
- Concurrent calls for one trace retain existing single-flight acquisition and
  lease/capacity behavior.

### Inventory tests

- Default inventory includes every currently supported evidence family without
  an LLM filter.
- Results preserve deterministic recent-first ordering and pagination.
- Target and installed duplicates do not produce duplicate `traceId` rows.
- Cross-owner collisions are represented once and marked ambiguous.
- Partial discovery returns useful items plus `complete: false` and a compact
  limitation.
- Empty complete results support a bounded “none found” conclusion.
- Empty incomplete results do not.

### Cross-cutting tests

- Runtime, skill, execution, activity, trace, and error examples contain no
  scope/instance fields.
- Internal target-change and generation checks still execute.
- Browser APIs retain their existing internal/detail contracts unless a shared
  internal refactor requires behavior-preserving changes.
- Authentication, read-only annotations, request limits, and untrusted-content
  handling are unchanged.

### Documentation/evaluation tests

- The installed skill contains no caller instructions involving source,
  artifact handles, target scopes, instance IDs, or MCP resources.
- Capability/tool maps match the advertised server.
- MCP remains independently usable without the skill.
- Representative exact-ID, latest-skill, imported, target-changed,
  unavailable, and raw-read paths succeed or degrade with the documented
  domain behavior.

## Acceptance criteria

1. An LLM can list traces and inspect any unique available trace using only
   `traceId` plus question-specific filters/ranges.
2. No advertised MCP parameter requires the LLM to select evidence origin or
   provide an installed-artifact identity.
3. No MCP result or error exposes the explicitly rejected ownership, resource,
   inventory-storage, retention, expiration, or size fields.
4. Console still rejects cross-target, cross-owner, stale, expired, and
   ambiguous evidence safely.
5. Imported evidence remains usable when it is the unique match, including
   without a selected target.
6. Inventory communicates pagination and evidence completeness without
   exposing its internal merge plan.
7. Tool descriptions and the installed skill teach the same trace-ID-based
   path.
8. Current custom MCP resources are no longer advertised; every supported
   investigation remains possible through tools.
9. Browser behavior and internal artifact lifecycle remain intact.
10. Focused and full repository tests pass, and applicable agent eval fixtures
    no longer depend on removed concepts.

## Explicit non-goals

Do not include these nearby candidates in this ticket:

- removing `sessionId` or deciding whether active execution detail/activity
  should use only `traceId`;
- redesigning `payloadRef` or general semantic content references;
- changing required/default `pageSize`, first-range `start`, or other ordinary
  parameter defaults;
- compacting `list_executions`;
- simplifying activity continuity beyond removing scope/instance identities;
- changing `query_trace_records` from physical to logical default or removing
  raw-address fields;
- changing frame/record filter vocabulary;
- eliminating duplicate text and structured MCP result representations;
- renaming, consolidating, or adding tools;
- semantic plan/model/tool content addressability work from later roadmap
  stages;
- changing the supported Java API, adding a Java SPI, or adding bean override
  contracts;
- changing browser-only storage management or trace import UI behavior; or
- capability compatibility governance before the first release.

## Guardrails

- The supported Java API is the closed allowlist in
  `LoomspanPublicSurfaceArchitectureTest`. This ticket should require no Java
  API expansion or compatibility shim.
- Internal `public` Java or Go types are not supported application API; refactor
  them only as needed without accidentally promoting them.
- Keep MCP read-only and loopback-authenticated.
- Treat all YAML, paths, model/tool content, diagnostics, errors, and raw bytes
  as untrusted evidence.
- Preserve bounded pages/ranges, maximum request sizes, continuation query
  binding, cancellation, single-flight acquisition, lease pinning, capacity,
  and cleanup.
- Do not hide a known evidence gap or collision to produce a simpler success
  response.
- Do not create process-global mutable “current trace” state.
- Do not encode rejected fields into a renamed visible token. Opaque
  continuations and current `payloadRef` may retain internal binding only under
  the explicit deferred-candidate behavior above.

## Verification commands

Run commands appropriate to the final touched surface. At minimum:

```text
cd loomspan-console
go test ./...
```

If Java production or observability adapter code changes, also run the
applicable Maven tests from the repository root, including
`LoomspanPublicSurfaceArchitectureTest`. Run the repository's agent evaluation
harness and any web tests whose fixtures or MCP/browser parity assumptions were
changed. Finish with a repository-wide search for every removed LLM-facing
field and inspect each remaining occurrence to confirm it is internal,
browser-only, historical documentation, or an explicit negative test.

## Post-implementation handoff

Do not immediately broaden the interface redesign. Connect a representative
LLM client to the updated MCP server and walk through:

1. the most recent successful `handleIncident` trace and its final primary
   plan;
2. a failed trace with retries or validation;
3. a manually imported trace;
4. an active execution;
5. missing, stale, target-changed, and ambiguous evidence; and
6. a large model/tool/plan payload requiring bounded retrieval.

Record tool choice, parameter hesitation, ignored fields, repeated evidence,
failed calls, recovery clarity, and approximate context cost in
`loomspan_skill_mcp_questions.md`. Use that evidence to decide the remaining
ledger candidates in a later context.
