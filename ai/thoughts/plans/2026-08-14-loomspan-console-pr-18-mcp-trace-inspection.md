# Loomspan Console PR 18 — Trace-Inspection MCP Surface Implementation Plan

## Overview

Implement the six settled trace-inspection MCP tools and supplementary trace resources as thin adapters over Loomspan Console's existing target/imported evidence ownership, artifact lifecycle, and trace-analysis services. The change adds one shared trace-inventory service and focused trace-analysis projections needed by both adapters; it does not create an MCP-owned catalog, acquisition path, cache, parser, calculation layer, retention policy, or diagnostic meaning.

The work remains tool-first and caller-directed. Target and imported evidence use the same opaque handles and leases while retaining explicit source identity; imported evidence remains inspectable without a selected target. Every retained frame, canonical record, reconstructed payload/diagnostic, and raw artifact byte remains reachable through finite continuable calls. Automated exactness, cancellation, concurrency, deadline, SDK/HTTP serialization, and host-memory evidence select the per-call byte maximum, with 16 MiB as the minimum.

## Current State Analysis

- The current MCP server registers six runtime, skill, active-execution, and recent-activity tools plus one skill resource template. It does not receive the artifact or trace-analysis services and has no PR 18 tools or resources (`loomspan-console/internal/mcpadapter/server.go:22-48`, `loomspan-console/internal/console/service.go:248-275`).
- `observability.Service` already provides selected-target trace catalog list/detail operations, while `artifact.Service` owns target acquisition, target-free imports, opaque handles, capacity, TTL, pinning, removal, target rotation, and shutdown (`loomspan-console/internal/observability/service.go:151-201`, `loomspan-console/internal/artifact/service.go:145-319`, `:359-469`, `:472-515`).
- Browser trace listing currently joins the application catalog to artifact lookup inside the browser adapter. `StorageSnapshot` covers installed target and imported entries but is unordered and intentionally omits handles, so it is not yet a reusable joined inventory (`loomspan-console/internal/browserapi/artifacts.go:174-209`, `loomspan-console/internal/artifact/model.go:126-174`).
- `evidence.Reference` already models `TARGET` with a target scope and `IMPORTED` without one. The browser resolves imports before capturing a target, but the MCP adapter's current helper captures a target for every inspection operation (`loomspan-console/internal/evidence/owner.go:10-61`, `loomspan-console/internal/browserapi/artifacts.go:211-224`, `loomspan-console/internal/mcpadapter/server.go:55-67`).
- `traceanalysis.Service` validates an entire NDJSON artifact before publication and already supplies summaries, frames, records, attempts, retries, validation links, failures, diagnostics, payload descriptors, gaps, uncertainties, literal search, payload ranges, raw-record ranges, and raw-artifact ranges. The missing shared projections are a canonical record enriched with those typed facts, frame-attributed gap/uncertainty kinds, and a bounded opaque content reference for failure diagnostics (`loomspan-console/internal/traceanalysis/service.go:35-103`, `loomspan-console/internal/traceanalysis/query_facts.go:15-625`, `loomspan-console/internal/traceanalysis/query_diagnostics.go:16-72`, `loomspan-console/internal/traceanalysis/query_ranges.go:17-236`).
- Trace-analysis cursors are strict unpadded-base64url JSON bound to operation, owner, handle, normalized-query fingerprint, and position. Their imported owner key currently contains a process-local internal owner ID and must be replaced with the adapter-safe imported source identity before the token is returned directly through MCP (`loomspan-console/internal/traceanalysis/cursor.go:13-105`, `:148-264`).
- The shared range default is 64 KiB and maximum is 1 MiB. The 1 MiB value is an implementation constant rather than an approved PR 18 product limit (`loomspan-console/internal/traceanalysis/limits.go:19-33`).
- Capability conformance currently checks required-tool membership only. The MCP golden inventory deliberately contains no trace tools or trace resources (`loomspan-console/internal/mcpadapter/capabilities.go:3-27`, `loomspan-console/internal/mcpadapter/capabilities_test.go:5-48`, `loomspan-console/internal/mcpadapter/contracts_test.go:106-122`).
- The current checkout is the research commit `14a910ca159a1f67a536f0790d7d0553933f6791`; the cited research therefore describes the production tree being planned rather than a stale revision.

## Desired End State

The standard Console binary exposes:

- `LOOMSPAN_list_traces`
- `LOOMSPAN_get_trace`
- `LOOMSPAN_query_trace_frames`
- `LOOMSPAN_query_trace_records`
- `LOOMSPAN_read_trace_payload`
- `LOOMSPAN_read_trace_artifact`

`loomspan.trace-inspection.v1` is advertised only with the first five operations and their semantic fixture suite; `loomspan.raw-artifact-inspection.v1` is advertised unconditionally with the raw-artifact operation and its semantic fixtures. The capabilities describe installed server behavior and do not vary with target selection, authentication, compatibility, storage contents, or trace availability.

All artifact-backed results carry the same nested evidence context:

```json
{
  "evidence": {
    "source": "TARGET|IMPORTED",
    "targetScopeId": "present only for TARGET",
    "artifactHandle": "opaque handle",
    "traceId": "trace identity",
    "sessionId": "session identity",
    "observedAt": "RFC 3339 timestamp"
  }
}
```

`LOOMSPAN_list_traces` accepts `sourceFilter: ALL|TARGET|IMPORTED` (default `ALL`), `pageSize`, and `continuation`. `ALL` and `IMPORTED` work without a selected target; `TARGET` requires one. Its result reports observation time, an `applicationCatalog` object, and entries whose `source`, target ownership, application availability, local availability, handle, and acquisition facts remain separate. `applicationCatalog` contains `requested`, `available`, optional `targetScopeId`/`instanceId`, and an optional unchanged shared `error`; this distinguishes an unrequested catalog (`IMPORTED`) from an unavailable requested catalog (`ALL` without usable target access).

`LOOMSPAN_get_trace` requires `source: TARGET|IMPORTED` and exactly one of `traceId` or `artifactHandle`. The allowed branches are `TARGET + traceId`, `TARGET + artifactHandle`, and `IMPORTED + artifactHandle`; `IMPORTED + traceId` is invalid. Downstream tools require `source` plus `artifactHandle` and never accept a client-supplied target scope.

Frame and record results expose existing mechanical calculations without adapter-side inference. Frames additionally carry `gapKinds` and `uncertaintyKinds` attributed by the shared analysis layer. Canonical record results add a `facts` object containing non-null arrays named `attempts`, `retries`, `validations`, `failures`, `payloads`, and `searchMatches`; an empty array means no such fact is owned by that record. Payload and failure facts issue an opaque `payloadRef` rather than exposing an internal component locator.

Payload and raw-artifact reads return `actualStart`, `actualEnd`, `totalLength`, `contentType`, `encoding`, `content`, `hasMore`, and `continuation`. Their input accepts either an explicit nonnegative `start` or a continuation, never both, plus optional `maxBytes`. Successful calls refresh the one shared installed copy's TTL; invalid, failed, or canceled calls do not.

The same DTOs are materialized as UTF-8 `application/json` resources at the settled target templates and these imported templates:

```text
loomspan://imports/artifacts/{artifactHandle}/summary
loomspan://imports/artifacts/{artifactHandle}/frames/{frameId}
loomspan://imports/artifacts/{artifactHandle}/records/{sequence}
```

The implementation is complete when automated conformance, lifecycle, parity, exact-range, security, concurrency, deadline, and bounded-memory tests pass with a shared range maximum of at least 16 MiB. The dated local-client matrix is populated afterward as compatibility observation and does not gate implementation completion, merge, or release.

### Key Discoveries

- Target and imported evidence already converge at `artifact.Service.Use` and `traceanalysis.Service`; the owner/reference boundary is sufficient for source-safe MCP access (`loomspan-console/internal/artifact/service.go:262-293`, `loomspan-console/internal/traceanalysis/dto.go:11-24`).
- Concurrent acquisition, independent waiter cancellation, lease pinning, successful-use TTL refresh, and source-selective target rotation are existing shared semantics to exercise, not reimplement (`loomspan-console/internal/artifact/service.go:145-259`, `loomspan-console/internal/artifact/lease.go:76-135`, `loomspan-console/internal/artifact/target_owner.go:24-61`).
- The approved workflows require one connected inspection experience, identical browser/MCP calculations, explicit missing/unavailable facts, and complete bounded addressability without automatic diagnosis (`ai/thoughts/phases/loomspan_console_workflows.md:45-100`).
- Phase 3 fixes the six tool boundaries and two capability families, makes resources supplementary, and prohibits server-side authority triggered by runtime content (`ai/thoughts/phases/loomspan_console_phase_3_llm_runtime_inspector.md:393-535`, `:867-905`).
- A saved trace is only a same-`consoleCompatibilityVersion` portable diagnostic artifact. Imports, handles, derived indexes, cursors, and catalogs remain transient current-process formats (`ai/thoughts/framework-feature-design-lens.md:29-61`).

## What We're NOT Doing

- No Java-side MCP endpoint, Java application API, Java SPI, Spring bean override contract, or new public type.
- No MCP-owned acquisition/import path, artifact copy, catalog, cache, parser, index, calculation, pin, last-use clock, capacity rule, retention policy, or error meaning.
- No active-trace tailing, durable execution history, persistent imported catalog, restart adoption, historical migration, cross-version reader, or compatibility fallback.
- No automatic diagnosis, root-cause ranking, importance/cost judgment, scenario-specific helper such as `get_failure_context`, or full-runtime dump.
- No prompt, sampling, elicitation, write/control tool, target selection, arbitrary URL, arbitrary filesystem path, shell operation, or content-triggered follow-up operation.
- No raw-artifact resource; exact raw bytes remain a caller-ranged tool operation.
- No MCP-specific lower range clamp, cumulative traversal quota, record-depth cap, or intentional omission of valid retained evidence.
- No portable `loomspan-runtime-debugging` Agent Skill implementation; that remains PR 19. This plan updates the existing skill-authoring knowledge base only where PR 18 changes the documented debugging surface.
- No browser visual redesign. The browser adapter may be moved to the shared inventory service without changing its established HTTP contract or UI behavior.

## Skill-Authoring Documentation Impact

**Impact**: Affected

- **Rationale**: PR 18 changes how a skill author can inspect current-run trace evidence: target and imported source identity, target-free imported access, handle lifetime, bounded continuation, opaque diagnostic payload references, and optional exact raw-artifact inspection become supported Console debugging behavior. These facts belong in the existing trace/debugging topic even though authoring syntax and the PR 19 Agent Skill are unchanged.
- **Documents to update**: `ai/skill-authoring/traces-and-debugging.md` and the `Traces and debugging` row in `ai/skill-authoring/README.md`.
- **Supporting evidence**: MCP schema/rendering goldens; target/imported trace tool tests; imported-without-target tests; trace resource tests; browser/MCP parity tests; capability semantic fixtures; shared artifact lifecycle tests; `traceanalysis` cursor/range tests; the Java-produced `loomspan-console-fixtures` corpus; and the production tool/resource registration.
- **Coverage table update**: Required. Keep coverage `Source-verified`, but expand its note to identify target/imported MCP inspection, handle/continuation lifetime, explicit source/availability semantics, and optional raw ranges. README routing remains `Diagnose retries, usage, or terminal failures -> traces-and-debugging.md` because the task boundary does not change.
- **LLM-first usability**: Add one compact MCP inspection subsection and a decision table instead of duplicating tool descriptions. It must distinguish `TARGET` from `IMPORTED`, application availability from local-handle availability, recorded facts from inference, and ordinary parsed inspection from optional raw forensics; state exact limitations and route readers back to source-verification anchors.

## Contract and Compatibility Impact

| Surface | Classification and supporting evidence | Planned compatibility treatment |
| --- | --- | --- |
| Application API | No supported Java API type is affected. The allowlist in `LoomspanPublicSurfaceArchitectureTest` remains unchanged. | Preserve; add no public Java surface. Run the architecture test as a final guard. |
| Supported SPI | No supported SPI exists for MCP, evidence, artifact, or trace analysis. Go service interfaces and `ServerOptions` are internal composition seams. | No compatibility shim. Update internal consumers atomically. |
| Configuration and manifest contracts | Existing `trace-workspace.max-bytes`/`idle-ttl` semantics and `unlimited`/`never` sentinels continue to govern target and imported evidence together. No YAML, key-file, skill-manifest, or MCP enablement setting changes. | Preserve existing configuration and manifest behavior. Do not add a raw-capability toggle or MCP range setting. |
| Persisted or serialized contracts | Adds externally visible MCP tool names, strict schemas, output fields, capability IDs, resource URI templates, deterministic fallbacks, and semantic fixtures. The Java REST/SSE/problem/NDJSON boundary is consumed unchanged. | Introduce the new MCP contracts coherently in one release; schemas remain strict and may evolve additively under the named capability generation. No Java/Go marker change. |
| Ephemeral diagnostic formats | Changes current-run trace cursor owner encoding, adds inventory cursors and opaque payload references, exposes transient imported results/resources, and raises the current-version range maximum after measurement. | Make current server, browser/MCP consumers, fixtures, and docs coherent atomically. Reject old/internal cursor forms; do not add legacy readers because cursors, handles, imports, and indexes do not survive process lifetime. Preserve exact bytes, source identity, security, and explicit uncertainty. |
| Internal or accidentally exposed implementation | Adds/reworks Go inventory, trace projection, DTO, cursor, resource, capability, and composition types under `loomspan-console/internal`. | Freely reshape internal types and update all in-repository callers/tests. No overload, alias, duplicate query mode, or compatibility bridge. |

- **Evidence of supported contracts**: The approved PR 18 ticket and Phase 3 tool/capability design establish the new MCP contract. Existing Console README, MCP contract tests, capability tests, and client compatibility matrix establish the current MCP families. The Java API allowlist establishes that no Java application API is involved.
- **Intended breaks**: The internal trace cursor changes imported ownership from `IMPORTED:<process-local-owner-id>` to the adapter-safe `IMPORTED` identity. This intentionally rejects pre-change process-local tokens; they are ephemeral and cannot validly survive the installed-copy/process lifetime. No protected contract is broken.
- **In-repository consumers to update**: MCP server/options/registrations, capability tests and goldens, browser trace inventory handler/tests, Console composition, trace-analysis DTO/query/cursor/range tests, artifact/observability fakes, integration fixtures, Console README, MCP client compatibility evidence, and the skill-authoring guide.
- **Public-surface delta**: No Java types, signatures, constructors, or Spring extension points are added or removed. The intended external delta is the six MCP tools, six trace resource templates (three target and three imported), and two named capability IDs.
- **Shim decision**: **No shim.** All changed Go and trace-analysis seams are internal, and cursors/payload references are ephemeral installed-copy tokens. Atomic producer/consumer/test updates are the coherent treatment.
- **Java-to-Go boundary coordination**: **Not required.** PR 18 consumes the existing application catalog, acquisition stream, problem meanings, and NDJSON fixture semantics without changing them. Existing Java-produced fixtures remain parity inputs; if implementation discovers a required NDJSON or application-adapter change, stop and create an explicit compatibility-marker decision rather than expanding this plan silently.

## Implementation Approach

Build from the shared semantics outward. First add a transport-neutral inventory and the minimal trace-analysis enrichments so both adapters receive authoritative, already-related facts. Then add strict MCP DTOs and handlers that perform only evidence-source resolution, service calls, protocol mapping, and deterministic rendering. Add resources and capability semantics over those same DTOs and leases. Finally measure response framing through the automated host and SDK/HTTP harness, commit the one shared maximum, update documentation, and run the full lifecycle/security/conformance matrix.

The adapter schema decisions are fixed as follows:

| Operation | Input | Result shape |
| --- | --- | --- |
| `LOOMSPAN_list_traces` | `sourceFilter`, `pageSize`, `continuation` | `observedAt`, `applicationCatalog`, `items`, `hasMore`, `continuation?` |
| `LOOMSPAN_get_trace` | required `source`; strict one-of `traceId` / `artifactHandle` | `evidence`, `summary`, `resources` |
| `LOOMSPAN_query_trace_frames` | `source`, `artifactHandle`, `filter`, `order`, `pageSize`, `continuation` | `evidence`, `items`, `hasMore`, `continuation?` |
| `LOOMSPAN_query_trace_records` | `source`, `artifactHandle`, `filter`, `representation`, `inlinePayload`, `pageSize`, `continuation` | `evidence`, enriched canonical `items`, `hasMore`, `continuation?` |
| `LOOMSPAN_read_trace_payload` | `source`, `artifactHandle`, `payloadRef`, strict one-of `start` / `continuation`, `maxBytes` | `evidence` plus exact range fields |
| `LOOMSPAN_read_trace_artifact` | `source`, `artifactHandle`, strict one-of `start` / `continuation`, `maxBytes` | `evidence` plus exact range fields |

All inputs reject unknown fields. `source`, filters, orders, representations, encodings, and fact kinds use closed enums. Page size uses the existing MCP maximum of 64. Later representative-client observations may motivate a separate contract change, but they do not alter this implementation; no client-specific schema is allowed.

Each inventory item has the exact common identity fields `source`, optional `targetScopeId`, `traceId`, `sessionId`, `entrySkill`, `outcome`, `finalizedAt`, `sizeBytes`, and `persistencePolicy`. Target catalog facts are optional `applicationTraceExpiresAt` and `applicationAvailability`; local-copy facts are `localAvailable`, optional `artifactHandle`, `acquiredAt`, `lastUsedAt`, `localExpiresAt`, and `localBytes`. Fields that do not apply to imported evidence are omitted rather than filled with a fabricated target, application availability, or zero timestamp. Acquisition failures are not retained as inventory state; a failed acquisition remains the shared error returned by the acquiring call.

Inventory continuations use strict unpadded-base64url JSON with this internal v1 meaning: operation, normalized query fingerprint (source filter plus resolved page size and target/no-target scope), segment (`INSTALLED` or `APPLICATION`), optional installed keyset (`finalizedAt`, `source`, `traceId`), and optional nested application cursor. They are capped at the existing 8,192-character MCP continuation boundary, unsigned, opaque, and never expose an upstream cursor as a separate client field.

Trace query/range continuations are returned directly after changing their owner binding to use target scope for `TARGET` and the literal adapter-safe `IMPORTED` identity for imports. They remain unsigned because authentication and owner/handle/operation/fingerprint/position/lifetime validation prevent tampering from granting authority. Opaque `payloadRef` values use the same strict token discipline and bind schema, evidence source, artifact handle, content kind (`PAYLOAD` or `FAILURE_DIAGNOSTIC`), and the applicable payload ID or failure ID/ordinal.

## Phase 1: Shared Inventory and Trace-Analysis Projections

### Overview

Create the transport-neutral service and projections required for adapter parity, with exhaustive service-level tests before adding MCP registration.

### Changes Required

#### 1. Transport-neutral trace inventory

**Files**: `loomspan-console/internal/traceinventory/{dto.go,service.go,cursor.go,service_test.go,cursor_test.go}` (new), `loomspan-console/internal/browserapi/artifacts.go`, `loomspan-console/internal/browserapi/router.go`, and related browser tests/fakes.

**Changes**:

- Define `SourceFilter` as `ALL`, `TARGET`, or `IMPORTED`; default to `ALL` and reject unknown values.
- Read installed entries from `artifact.StorageSnapshot`, resolve each through `artifact.Service.Lookup` to obtain its handle, and read target catalog pages through `observability.Service`. Retain these services as the owners of their facts.
- For `ALL`/`IMPORTED`, allow no selected target and explicitly report `applicationCatalogAvailable: false`; for `TARGET`, preserve the shared no-target error.
- Emit installed entries first in deterministic keyset order: `finalizedAt` descending, `TARGET` before `IMPORTED`, then `traceId` ascending. Follow with application-catalog entries not currently installed, preserving application cursor order and skipping entries that became installed.
- Keep application availability, local availability, handle presence, source, target scope, and acquisition metadata as separate fields. Do not synthesize an aggregate lifecycle or health state.
- Encode the composite cursor described above; bind it to the filter, resolved page size, and captured target/no-target scope. Document weak consistency across mutations while preventing offset-shift pagination.
- Move the browser's current adapter-local catalog/artifact join onto this service without changing its browser JSON contract.

#### 2. Enriched canonical records and frame limitations

**Files**: `loomspan-console/internal/traceanalysis/dto.go`, `query_records.go`, `query_frames.go`, `query_facts.go`, `search.go`, and focused tests.

**Changes**:

- Add an enriched record result that preserves the canonical record descriptor and attaches existing typed attempt, retry, validation, failure, payload, and literal-search facts in canonical record order.
- Perform index joins once inside `traceanalysis`; do not add adapter joins or a `view` switch that turns the record operation into unrelated query modes.
- Add mechanically attributed `GapKinds` and `UncertaintyKinds` to frame results, using existing indexed facts and recorded identifiers only.
- Preserve existing filters, logical/physical representation, optional <=8 KiB inline payload behavior, and query fingerprinting. Include every new filter/representation field in canonicalization.
- Keep missing facts empty/unknown and preserve direct usage-completeness flags; do not create an aggregate completeness score or diagnosis.

#### 3. Opaque content references and imported-safe cursors

**Files**: `loomspan-console/internal/traceanalysis/cursor.go`, `query_ranges.go`, `query_diagnostics.go`, `dto.go`, `limits.go`, and cursor/range/diagnostic tests.

**Changes**:

- Replace imported cursor owner serialization with the literal adapter-safe `IMPORTED` key while retaining target scope binding for target evidence.
- Add strict maximum encoded-token validation and preserve unpadded base64url, unknown-field rejection, operation/fingerprint/handle/position checks, and installed-copy lifetime validation.
- Add opaque, handle/source-bound `payloadRef` encoding for reconstructed payloads and failure diagnostics.
- Refactor payload range reading to resolve a `payloadRef`; add bounded UTF-8-safe range reading for the selected failure diagnostic. Keep internal raw record range support but do not expose another MCP tool.
- Retain the 64 KiB default. Add test/benchmark hooks for larger maxima but do not finalize the production maximum until Phase 4 evidence is complete.

### Success Criteria

#### Automated Verification

- [x] Inventory unit tests cover every filter, selected/no-target behavior, deterministic order, duplicate suppression, weakly consistent mutation, page boundaries, malformed/wrong-query continuations, and target rotation: `cd loomspan-console; go test ./internal/traceinventory ./internal/browserapi`.
- [x] Trace-analysis tests cover all enriched fact memberships, frame gap/uncertainty attribution, empty facts, exact canonical ordering, query-fingerprint changes, imported cursor opacity, wrong source/handle/query, malformed/oversized tokens, and ranged diagnostics: `cd loomspan-console; go test ./internal/traceanalysis`.
- [x] Existing artifact acquisition, import, TTL, pinning, capacity, removal, and cancellation behavior remains green: `cd loomspan-console; go test ./internal/artifact`.
- [x] Browser contract fixtures remain byte-for-byte compatible after adopting the shared inventory: `cd loomspan-console; go test ./internal/browserapi`.

#### Manual Verification

- [x] Inspect decoded test-only inventory and trace cursors to confirm no upstream cursor is separately exposed and no process-local imported owner ID appears.
- [x] Review representative enriched records and frames against the workflow catalog to confirm they state mechanical facts without diagnosis, importance, or aggregate completeness.

---

## Phase 2: Strict MCP Trace Tools

### Overview

Add six deterministic structured/text tool contracts over the Phase 1 services, including target-optional imported access and exact source/handle validation.

### Changes Required

#### 1. DTOs, schemas, mapping, and deterministic rendering

**Files**: `loomspan-console/internal/mcpadapter/contracts.go`, `trace_contracts.go` (new), `trace_mapping.go` (new), `trace_render.go` (new), `testdata/trace-*.json` (new), and contract tests.

**Changes**:

- Implement the six input/result pairs and common `evidence` context defined above without exporting `traceanalysis` or MCP SDK types across package boundaries.
- Add explicit schema constraints for handles, nonblank IDs, closed enums, one-of branches, page/range bounds, unknown-field rejection, and the 8,192-character continuation/payload-reference maximum.
- Map timestamps, nullable calculations, availability, source, typed facts, range offsets, and resource links without recomputation.
- Represent arbitrary bytes as standard base64 only when the shared range result says `BASE64`; preserve exact source-byte offsets and total length.
- Reuse `toolEnvelope`, `domainFailure`, `mapDomainError`, read-only annotations, and `lineWriter`. Text fallbacks are deterministic, line-oriented, fact-complete for the finite result, concise, and contain no instructions or inferred diagnosis.
- Commit goldens for target success, target-free imported success, empty/final page, continuation, maximum page/range, UTF-8, base64, every valid input branch, and representative shared errors.

#### 2. Evidence resolution and handlers

**Files**: `loomspan-console/internal/mcpadapter/traces.go` (new), `trace_queries.go` (new), `trace_ranges.go` (new), and focused tests.

**Changes**:

- Add one helper that maps `TARGET` to a freshly captured current target reference and `IMPORTED` directly to `evidence.ForImported()` without target capture.
- Implement `LOOMSPAN_list_traces` through the shared inventory service.
- Implement `LOOMSPAN_get_trace` with the three allowed branches. Only `TARGET + traceId` calls shared acquisition; handle branches reopen the installed copy and never search owners globally.
- Implement frame, enriched-record, payload, and raw-artifact handlers as one service call plus mapping. Return shared continuations directly.
- Recheck target publication and MCP authentication generation before emitting target results. Imported results require the MCP generation check but no invented target publication check.
- Preserve shared domain codes and safe details; malformed schema/URI input remains a protocol invalid-argument failure.

#### 3. Registration and composition

**Files**: `loomspan-console/internal/mcpadapter/server.go`, `loomspan-console/internal/console/service.go`, `loomspan-console/internal/buildtool/mcp_conformance.go`, and test helpers/fakes.

**Changes**:

- Extend internal `ServerOptions` with the inventory, artifact, and trace-analysis services and wire the existing production instances from the Console composition root.
- Register all six tools in the standard binary. Do not add runtime toggles or conditional raw-tool registration.
- Update build-tool and test server composition with faithful fakes; keep SDK types confined to `internal/mcpadapter`.

### Success Criteria

#### Automated Verification

- [x] Strict schema and golden rendering tests pass for all six tools: `cd loomspan-console; go test ./internal/mcpadapter`.
- [x] Target-free imported list/get/query/range calls pass with no selected target; `TARGET` calls preserve no-target, changed-target, authentication, and compatibility errors.
- [x] Wrong source/handle, imported `traceId`, both/neither identifier branches, both start/continuation, malformed/oversized tokens, unknown fields, and out-of-range values are rejected precisely.
- [x] Exact UTF-8 and base64 traversal reconstructs the original bytes without gaps, overlap, or truncation.
- [x] Cancellation suppresses publication and does not refresh TTL; successful tool reads refresh the shared entry once.
- [x] Tool inventory now contains exactly twelve tools with read-only/idempotent/non-destructive/closed-world annotations.

#### Manual Verification

- [ ] Inspect tool descriptions and text fallbacks in an MCP inspector to confirm an unfamiliar client can discover the workflow without the future Agent Skill.
- [ ] Verify returned application content is visibly data and cannot act as a target, URL, path, command, credential operation, or follow-up MCP invocation.

---

## Phase 3: Resources, Capabilities, and Cross-Adapter Semantics

### Overview

Add the supplementary immutable resource views and make capability advertisement depend on both complete tool membership and reviewed semantic fixtures.

### Changes Required

#### 1. Target and imported trace resources

**Files**: `loomspan-console/internal/mcpadapter/resources.go`, `trace_resources.go` (new), and resource tests.

**Changes**:

- Register target summary/frame/record templates at the settled `loomspan://targets/{targetScopeId}/artifacts/...` URIs and the three imported templates listed in Desired End State.
- Parse every URI canonically: exact scheme/authority/path shape, canonical UTF-8 percent encoding, no query/fragment/userinfo, valid handle, positive record sequence, and nonblank frame ID.
- Materialize `application/json` using the exact summary/frame/record DTO used by tools. Summary calls shared `GetSummary`; frame uses an exact-ID query that must return exactly one item; record uses an exact-sequence `LOGICAL` enriched-record query without expanding a large payload.
- Use shared analysis leases and `resourceDomainError`; success refreshes shared last use, while failure/cancellation does not. Imported reads never capture a target.
- Keep resources supplementary and omit a raw-artifact resource.

#### 2. Capability contract manifest

**Files**: `loomspan-console/internal/mcpadapter/capabilities.go`, `capabilities_test.go`, `testdata/capability-contracts.json` (new), and semantic fixture tests.

**Changes**:

- Add `loomspan.trace-inspection.v1` with its five required tools and `loomspan.raw-artifact-inspection.v1` with its one required tool to the fixed production capability table.
- Define a reviewed JSON test manifest whose entries contain capability ID, exact required tools, and stable semantic fixture IDs. Test that the manifest, production descriptors, registered tools, and executed semantic fixtures agree.
- For trace inspection, require fixtures for target acquisition, target-free import, explicit source identity, independent availability facts, browser/MCP parity, fact calculations, continuation, lifecycle, cancellation, multiple clients, schemas, and safe errors.
- For raw inspection, require exact bytes, both sources, continuation, lifecycle/error parity, and proof of no acquisition.
- Prove conformance fails when one required tool is removed and independently when one semantic fixture is absent/failing even though tool membership remains complete.

#### 3. Joined lifecycle, parity, and workflow coverage

**Files**: `loomspan-console/internal/mcpadapter/parity_test.go`, MCP/integration tests, `loomspan-console/internal/console/artifact_integration_test.go`, and workflow-tagged fixtures/tests.

**Changes**:

- Start browser and MCP mappings from the same neutral inventory/summary/frame/record/range result or Java-produced trace fixture and compare identifiers, calculations, availability, limitation facts, and shared error codes.
- Cover simultaneous browser/MCP acquisition joining one download/copy/handle/capacity charge; independent waiter cancellation; shared pinning and last-use; TTL/capacity/removal invalidation; target rotation that removes only target evidence; and imported survival until ordinary removal/expiry/shutdown.
- Cover malformed, truncated, incompatible, oversized, unsafe, expired, removed, and in-use artifacts; multiple MCP clients; console shutdown; and authentication-generation cancellation.
- Link representative tests to `WF-FAILED-EXECUTION`, `WF-EXPENSIVE-EXECUTION`, `WF-UNFAMILIAR-SKILL-PATH`, and their most specific requirement IDs without creating a second scenario catalog.

### Success Criteria

#### Automated Verification

- [x] Tool/resource discovery and exact URI parsing tests pass for target and imported evidence: `cd loomspan-console; go test ./internal/mcpadapter`.
- [x] Capability manifest tests reject missing tools and missing semantics and advertise both new capabilities only for the complete assembled surface.
- [x] Browser/MCP parity and joined acquisition/lifecycle tests pass: `cd loomspan-console; go test ./internal/mcpadapter ./internal/browserapi ./internal/console ./internal/artifact ./internal/traceanalysis`.
- [x] Official MCP conformance remains green for both supported protocol revisions through the repository's existing conformance build tool.
- [x] Adversarial runtime strings trigger no additional server-side operation or authority in instrumented security tests.

#### Manual Verification

- [ ] Browse and read every target/imported resource template from a client that exposes resources, confirming identical JSON facts and current lifecycle errors.
- [ ] Run one failed-execution, high-usage, and unfamiliar-skill-path investigation using tools only; confirm all cited facts are reachable without a scenario-specific helper.

---

## Phase 4: Response Framing and Post-Implementation Client Validation

### Overview

Measure the real one-response boundary and select one shared trace-analysis maximum from automated exactness, cancellation, concurrency, deadline, serialization, and bounded-memory evidence. Record representative-client observations after implementation completion.

### Changes Required

#### 1. Exact range and host-resource harness

**Files**: `loomspan-console/internal/traceanalysis/limits.go`, range tests/benchmarks, MCP integration tests, and test fixtures/generators.

**Changes**:

- Exercise source-byte windows of 1, 4, 16, and 32 MiB, plus 64 MiB when the preceding sizes remain healthy, for valid UTF-8 and worst-case base64.
- For every size, verify exact checksums, continuation boundaries, cancellation, request deadline behavior, JSON/SDK serialization, bounded peak memory, and simultaneous browser/MCP use.
- Keep the default range modest (64 KiB). Select the largest size supported by the automated server/SDK exactness, deadline, concurrency, cancellation, and bounded-memory evidence.
- Raise the shared `traceanalysis` maximum to that selected source-byte value and expose the same number in tool descriptions/results and `LIMIT_EXCEEDED` details. Do not add a lower MCP-only clamp or silently clamp callers.
- Require at least 16 MiB from the automated host and SDK tests; do not retain the old 1 MiB ceiling by default.

#### 2. Post-implementation client compatibility matrix

**File**: `loomspan-console/docs/mcp-client-compatibility.md`.

**Changes**:

- Replace the PR 17-only wording and unexecuted rows with dated PR 18 evidence for the then-current stable local Codex surfaces, Claude Code, Antigravity, Cursor, and Devin Desktop/Windsurf/Cascade or local Devin CLI.
- Record client/build, OS, configuration mechanism, observed protocol when exposed, and pass/fail notes without recording credentials.
- Verify twelve-tool discovery; output schemas; structured content and deterministic text; shared domain `isError`; all six trace calls; selected-target and target-free imported flows; resource-template discovery/read; continuation round trips; 64-item pages; and every candidate UTF-8/base64 range.
- Keep hosted clients explicitly out of scope because they cannot reach the loopback endpoint. Do not add client-specific tool schemas or capability forks.

### Success Criteria

#### Automated Verification

- [x] Every selected-size UTF-8/base64 case reconstructs the source bytes exactly and passes cancellation/concurrency/deadline checks.
- [x] Requests one byte over the final maximum return explicit shared `LIMIT_EXCEEDED` details; no successful result is silently shortened.
- [x] Peak memory and latency measurements for the final size are recorded and satisfy the test harness thresholds.
- [x] Full Go suite passes with the final shared constant: `cd loomspan-console; go test ./...`.
- [x] Go static analysis passes after the final shared constant is selected: `cd loomspan-console; go vet ./...`.

#### Post-Implementation Observation (Non-Gating)

- Populate dated PR 18 client observations after implementation completion as clients become available.
- Record whether client rendering distinguishes base64 expansion from source-byte range length and preserves continuations.

The selected automated maximum is at least 16 MiB and is cited by the compatibility document and tool contract documentation. Client observations do not gate or reopen that completed decision.

---

## Phase 5: Documentation and Release Verification

### Overview

Document the final installed surface and author-facing debugging semantics, then run the complete repository verification without implementing the separate PR 19 Agent Skill.

### Changes Required

#### 1. Console consumer documentation

**Files**: `loomspan-console/README.md` and `loomspan-console/docs/mcp-client-compatibility.md`.

**Changes**:

- Replace the statement that trace inspection is deferred with the exact six tools, two capabilities, source semantics, resource templates, continuation/range behavior, and sensitivity/lifecycle limitations.
- State that raw-artifact capability is always present in the standard binary but optional to a compatible portable skill.
- Document that imported evidence is transient, unauthenticated as application provenance, target-optional, and removed through the ordinary shared lifecycle.
- Document the final page/range maxima in source-byte terms and the exact `LIMIT_EXCEEDED` behavior.

#### 2. Skill-authoring knowledge base

**Files**: `ai/skill-authoring/traces-and-debugging.md` and `ai/skill-authoring/README.md`.

**Changes**:

- Add a compact MCP trace-inspection decision table: use target catalog/`traceId` for acquisition; use source plus handle for an installed copy; use imported handle without a target; prefer parsed summary/frame/record/payload evidence; use optional raw ranges only for exact storage/parser questions.
- Explain source identity, separate application/local availability, handle and continuation expiration, exact-range traversal, successful-use TTL refresh, and the absence of authenticity/provenance for imports.
- Preserve the recorded-fact/inference boundary and warn that diagnostic content is untrusted and may be sensitive.
- Add stable implementation/test anchors for MCP contracts, semantic fixtures, inventory, cursor/range, and lifecycle coverage.
- Update the README coverage row while retaining the existing routing entry and `Source-verified` status. Verify the changed guidance against the LLM-first authoring checklist.

#### 3. Final repository verification

**Files**: test suites and fixtures changed in prior phases; no production Java change expected.

**Changes**:

- Run formatters for changed Go files and confirm no generated/golden drift.
- Run focused and full Go tests, official MCP conformance, repository metadata checks, and the Java public-surface architecture guard.
- Review the final diff for SDK leakage outside `mcpadapter`, accidental Java/public/Spring surface changes, undocumented configuration, legacy cursor readers, duplicate catalogs, or adapter-side calculation logic.

### Success Criteria

#### Automated Verification

- [x] Go formatting is clean: `cd loomspan-console; gofmt -w <changed-go-files>` followed by a clean diff check.
- [x] Full Console suite passes: `cd loomspan-console; go test ./...`.
- [x] Go static analysis passes: `cd loomspan-console; go vet ./...`.
- [x] Java supported-surface classification remains unchanged: `./mvnw -q -Dtest=LoomspanPublicSurfaceArchitectureTest test`.
- [x] Repository specification metadata passes: `bash ai/scripts/spec_metadata.sh`.
- [x] MCP golden inventory, capability semantic manifest, official conformance, browser/MCP parity, fixture corpus, and lifecycle/security suites all pass, including deterministic joined browser/MCP lifecycle coverage.
- [x] Skill-authoring guidance is supported by the cited source/tests/fixtures, the README coverage row is updated, and an LLM loading only the routed documents can identify applicability, requirements, prohibitions, limitations, and next references.

#### Manual Verification

- [ ] A target trace can be discovered, acquired, queried, continued, and read through both tools and resources; browser and MCP show the same source identity, handle, calculations, availability facts, and invalidation.
- [ ] An imported trace can be listed and fully inspected with no selected target, survives target rotation, and disappears after removal, expiry, shutdown, or restart as documented.
- [ ] A broad traversal reaches every matching frame/record and every payload/raw byte while the handle remains valid, with no diagnosis or hidden omission by the server.
- [x] Documentation contains no key, application credential, machine-specific secret, false provenance claim, or suggestion that runtime content is executable instruction.

## Testing Strategy

Create the dedicated test plan with `ai/commands/3_testing_plan.md` before implementation. It should turn the phase criteria above into failing-first test slices and retain these coverage layers:

### Unit Tests

- Inventory filter/order/merge/cursor semantics, including no-target imports and mutation between pages.
- Evidence resolver branch matrix and strict JSON schemas.
- Enriched record ownership, frame gap/uncertainty attribution, opaque payload references, cursor binding, UTF-8/base64 ranges, exact limits, and cancellation.
- URI canonicalization and deterministic structured/text/resource rendering.
- Capability tool membership plus semantic fixture membership.

### Integration Tests

- Java-produced fixture -> shared Go analysis -> browser/MCP parity.
- Selected target acquisition and target-free imported inspection.
- Browser/MCP joined acquisition, concurrent clients, independent cancellation, pins, TTL, capacity, removal, target rotation, shutdown, and restart cleanup.
- Shared errors for malformed, truncated, incompatible, expired, removed, in-use, oversized, unsafe, authentication-required, no-target, and unexpected internal failures.
- Official MCP protocol conformance and adversarial-content non-execution.
- Large UTF-8/base64 range tests at 1/4/16/32 MiB and conditionally 64 MiB.

### Manual Testing Steps

These steps happen after implementation completion and are observational. They
are not exit criteria for the PR, merge, or release.

1. Run the full six-tool target workflow in each required local MCP client and record the exact build and protocol behavior.
2. Clear the target, import a valid same-version trace through the browser, and repeat list/get/frame/record/payload/raw/resource inspection from MCP.
3. Continue a 64-item page and every candidate byte window; compare reconstructed checksums with the installed source.
4. Rotate the target during a target query and verify `TARGET_CHANGED`; confirm imported evidence remains usable.
5. Remove and expire entries and verify both browser and MCP receive the same handle invalidation; cancel a read and verify last-use time is unchanged.
6. Place adversarial instructions in YAML, errors, metadata, records, and payloads and verify the server only returns requested data.

## Performance Considerations

- Inventory pagination must not load the entire application catalog; installed entries use deterministic keyset traversal and catalog-only entries preserve the upstream cursor.
- Record enrichment must use the existing immutable indexes inside `traceanalysis`, not repeated whole-artifact scans or per-item MCP joins.
- One artifact lease pins one finite call. Successful materialization refreshes the shared TTL once; failure and cancellation release without refreshing.
- Large range results may coexist as raw bytes, base64, JSON, SDK, HTTP, and client/model-bridge representations. The Phase 4 harness must measure peak memory and latency under simultaneous browser/MCP calls before choosing the maximum.
- The range maximum bounds one response, not cumulative evidence. Continuations preserve unlimited cumulative traversal while the transient handle remains valid.
- No new request-rate, traffic, client-count, trace-count, depth, or cumulative-byte quota is introduced.

## Migration Notes

- No application data, configuration file, skill manifest, Java trace, imported artifact, or durable store migration is required.
- Existing process-local trace cursors and payload identifiers need no compatibility reader. A running old process cannot be upgraded in place, and shutdown already invalidates handles, imported evidence, indexes, and continuations.
- The standard binary begins advertising the two capabilities only when the complete PR 18 implementation and semantic manifest land together. Do not merge a state that advertises either family before all required operations and fixtures are present.
- Existing browser API behavior remains compatible; its internal trace-list assembly moves to the shared inventory service atomically with its tests.
- `consoleCompatibilityVersion` remains the unchanged complete release string because the Java application-adapter and NDJSON boundary do not change.

## References

- Original ticket: `ai/thoughts/tickets/loomspan-console-pr-18-mcp-trace-inspection.md`
- Related research: `ai/thoughts/research/2026-08-14-loomspan-console-pr-18-mcp-trace-inspection.md`
- Phase 3 design: `ai/thoughts/phases/loomspan_console_phase_3_llm_runtime_inspector.md`
- Approved workflow catalog: `ai/thoughts/phases/loomspan_console_workflows.md`
- Active roadmap: `ai/thoughts/phases/2026-08-12-loomspan-active-roadmap.md`
- Framework compatibility lens: `ai/thoughts/framework-feature-design-lens.md`
- Skill-authoring debugging guidance: `ai/skill-authoring/traces-and-debugging.md`
- Post-implementation MCP client observations: `loomspan-console/docs/mcp-client-compatibility.md`
