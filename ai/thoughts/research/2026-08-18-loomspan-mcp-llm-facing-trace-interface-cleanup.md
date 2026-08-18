---
date: 2026-08-18T00:16:52-07:00
researcher: Codex
git_commit: ff054350fa22537c9ab7321083b7279bda91beab
branch: main
repository: loomspan
topic: "Current codebase surfaces affected by the LLM-facing MCP trace interface cleanup ticket"
tags: [research, codebase, mcp, trace-analysis, trace-inventory, artifact-lifecycle, agent-evals]
status: complete
last_updated: 2026-08-18
last_updated_by: Codex
---

# Research: Current codebase surfaces affected by the LLM-facing MCP trace interface cleanup ticket

**Date**: 2026-08-18T00:16:52-07:00
**Researcher**: Codex
**Model**: GPT-5
**Git Commit**: ff054350fa22537c9ab7321083b7279bda91beab
**Branch**: main
**Repository**: loomspan

## Research Question

Research the live Loomspan codebase for the implementation scope described by `ai/thoughts/tickets/loomspan-mcp-llm-facing-trace-interface-cleanup.md`, documenting the current MCP contracts, handler and lifecycle flow, affected tests/documentation/fixtures, contract classifications, and protected internal and cross-component behavior.

## Summary

The live repository matches the ticket's implementation map. `mcpadapter.NewServer` currently registers twelve read-only tools and seven custom resource templates: one skill resource and six trace resources (`loomspan-console/internal/mcpadapter/server.go:58-69`, `loomspan-console/internal/mcpadapter/server_test.go:81-98`). The six trace tools expose `sourceFilter`, `source`, and/or `artifactHandle`; their results expose source, target scope, handle, storage/catalog state, and resource navigation (`loomspan-console/internal/mcpadapter/trace_contracts.go:23-98`, `loomspan-console/internal/mcpadapter/trace_contracts.go:126-135`). Runtime, skills, executions, activity, and domain errors separately expose target-scope or runtime-instance identifiers (`loomspan-console/internal/mcpadapter/runtime.go:20-23`, `loomspan-console/internal/mcpadapter/contracts.go:25-131`).

There is no current internal operation that accepts only `traceId` and resolves across current-target installed evidence, imported evidence, and target acquisition. Current MCP handlers first resolve the caller's `source`; only the target `get_trace` trace-ID branch invokes artifact acquisition. Every other trace operation consumes a caller-provided handle (`loomspan-console/internal/mcpadapter/traces.go:123-158`, `loomspan-console/internal/mcpadapter/traces.go:161-271`, `loomspan-console/internal/mcpadapter/traces.go:278-290`). The concrete artifact service already supplies the component mechanics needed by such orchestration: owner-scoped lookup, idempotent and single-flight target acquisition, cryptographically random process-local handles, target-generation checks, imported ownership, idle expiry, and one lease per analysis call (`loomspan-console/internal/artifact/service.go:145-235`, `loomspan-console/internal/artifact/service.go:419-474`, `loomspan-console/internal/artifact/import.go:12-89`, `loomspan-console/internal/traceanalysis/service.go:78-107`).

The present inventory joins installed and application-catalog evidence, orders installed entries deterministically, and suppresses application catalog rows already installed for the target. It does not consolidate a target/imported collision: both installed owners can emit rows with the same `traceId`. Partial discovery is represented through the nested `ApplicationCatalog` availability/error object rather than `complete` plus compact limitations (`loomspan-console/internal/traceinventory/service.go:69-223`, `loomspan-console/internal/traceinventory/service.go:248-302`).

The change is confined by the ticket to the Go MCP adapter and its model-facing documentation/evaluations. The browser adapters intentionally expose source, target scope, handles, storage state, and browser trace DTOs, and share the same artifact and analysis services. The Java-to-Go application REST/artifact protocol remains the target catalog and acquisition source; no Java supported Application API or Supported SPI is involved. The portable NDJSON trace corpus and its exact `consoleCompatibilityVersion` remain the current-release ephemeral diagnostic input to analysis.

Focused baseline verification passed on this commit:

```text
go test ./internal/mcpadapter ./internal/traceinventory ./internal/artifact ./internal/traceanalysis
```

## Detailed Findings

### 1. MCP server assembly and advertised surface

`ServerOptions` injects status, target, observability, live activity, artifact, trace-analysis, and trace-inventory collaborators. Its three trace-facing interfaces currently expose inventory `List`, target artifact `Acquire`, and the five analysis operations used by MCP (`loomspan-console/internal/mcpadapter/server.go:26-56`). `NewServer` registers runtime, two skill, two execution, one activity, and six trace tools, followed by the skill resource and all trace resources (`loomspan-console/internal/mcpadapter/server.go:58-69`).

The server tests establish the current technical behavior:

- exactly twelve tools are listed (`loomspan-console/internal/mcpadapter/server_test.go:81-82`, `loomspan-console/internal/mcpadapter/server_test.go:160-165`);
- exactly seven resource templates are listed (`loomspan-console/internal/mcpadapter/server_test.go:95-98`, `loomspan-console/internal/mcpadapter/server_test.go:174-176`);
- all tools carry read-only, non-destructive, idempotent, closed-world annotations (`loomspan-console/internal/mcpadapter/contracts.go:135-141`, `loomspan-console/internal/mcpadapter/server_test.go:167-172`); and
- the installed capability map assigns five parsed trace tools to `loomspan.trace-inspection.v1` and the raw reader to `loomspan.raw-artifact-inspection.v1` (`loomspan-console/internal/mcpadapter/capabilities_test.go:29-35`).

The README describes those capability identifiers as an installed server-surface promise independent of current target/evidence state (`loomspan-console/README.md:255-263`).

### 2. Current trace tool schemas and handler behavior

All six trace schemas are Go DTO-derived JSON Schemas, then tightened with enum, length, integer, and one-of constraints (`loomspan-console/internal/mcpadapter/traces.go:21-97`, `loomspan-console/internal/mcpadapter/trace_contracts.go:294-336`).

| Tool | Current caller identity/routing | Current handler path |
| --- | --- | --- |
| `LOOMSPAN_list_traces` | Optional `sourceFilter`; optional `pageSize` and `continuation` (`trace_contracts.go:23-27`) | Passes filter directly to `traceinventory.Service.List`; validates current target at publication when a target catalog was consulted (`traces.go:99-120`). |
| `LOOMSPAN_get_trace` | Required `source`; exactly one of `traceId` or `artifactHandle` (`trace_contracts.go:28-32`, `traces.go:32-40`) | `TARGET` plus `traceId` captures scope and calls `Artifacts.Acquire`; imported trace IDs are rejected; a handle branch reopens analysis directly (`traces.go:123-158`). |
| `LOOMSPAN_query_trace_frames` | Required `source` and `artifactHandle`; filters/order/page controls (`trace_contracts.go:33-40`) | Converts the handle and calls `TraceAnalysis.QueryFrames`, then performs target publication and MCP-authentication generation checks (`traces.go:161-190`). |
| `LOOMSPAN_query_trace_records` | Required `source` and `artifactHandle`; filters/representation/inline/page controls (`trace_contracts.go:41-49`) | Calls `TraceAnalysis.QueryRecords` with the supplied handle and preserves the same publication checks (`traces.go:193-222`). |
| `LOOMSPAN_read_trace_payload` | Required `source`, `artifactHandle`, and `payloadRef`; exactly one of `start`/`continuation`; bounded `maxBytes` (`trace_contracts.go:50-57`, `traces.go:64-69`, `traces.go:84-97`) | Passes the handle, opaque payload reference, and range controls to `ReadPayloadRange` (`traces.go:225-271`). |
| `LOOMSPAN_read_trace_artifact` | Required `source` and `artifactHandle`; exactly one of `start`/`continuation`; bounded `maxBytes` (`trace_contracts.go:50-57`, `traces.go:71-76`, `traces.go:84-97`) | Passes the handle and range controls to `ReadRawArtifactRange`; `payloadRef` is removed from this tool's generated schema (`traces.go:94-96`, `traces.go:246-257`). |

`resolveEvidence` is only a source-to-owner-reference mapper. `IMPORTED` creates an imported reference without target capture; `TARGET` captures the selected target; any other value returns `INVALID_ARGUMENT` (`loomspan-console/internal/mcpadapter/traces.go:278-293`). It does not inspect a trace ID, compare owners, reuse imported evidence, or acquire evidence.

The current trace result identity is `source`, optional `targetScopeId`, `artifactHandle`, `traceId`, `sessionId`, and adapter observation time (`loomspan-console/internal/mcpadapter/trace_contracts.go:59-66`, `loomspan-console/internal/mcpadapter/traces.go:330-335`). `get_trace` additionally returns three resource links constructed from scope/source and handle (`loomspan-console/internal/mcpadapter/trace_contracts.go:126-135`, `loomspan-console/internal/mcpadapter/traces.go:391-400`). Frame, record, and range results repeat the evidence object while retaining the domain summaries, record facts, raw addresses, bounded content, and continuations (`loomspan-console/internal/mcpadapter/trace_contracts.go:137-288`).

### 3. Current trace inventory

`traceinventory.Query` treats a zero source filter as `ALL` and a zero page size as the service default. Its outward internal result includes a distinct application-catalog status plus storage-rich entries (`loomspan-console/internal/traceinventory/dto.go:11-68`).

`Service.List` currently performs these steps:

1. validate/default filter and page size (`service.go:69-85`);
2. capture the target unless the request is imported-only, retaining a capture error as partial catalog state for `ALL` (`service.go:88-107`);
3. snapshot artifact storage and resolve usable installed entries through owner-scoped `Lookup` (`service.go:124-150`);
4. sort installed evidence newest-first, with target before imported for equal timestamps (`service.go:151-152`, `service.go:295-302`);
5. page through installed evidence before application catalog evidence (`service.go:157-180`); and
6. list target catalog pages and suppress catalog traces already present as installed target evidence (`service.go:185-223`).

The duplicate suppression map is populated only for target-owned installed entries (`service.go:129-149`). Consequently, a current-target installed trace and an imported installed trace with the same `traceId` remain separate rows; the catalog copy of the target trace is the row that is suppressed. The current cursor fingerprints the filter, page size, scope, installed target IDs, and target handles, and rejects a changed installed target set during catalog traversal (`service.go:109-121`, `service.go:152-155`, `service.go:277-292`).

The MCP mapper reproduces application catalog requested/available/scope/instance/error fields and every inventory entry's source, scope, size, persistence, application availability/expiry, local availability/handle, acquisition/last-use/local-expiry, and local-byte fields (`loomspan-console/internal/mcpadapter/traces.go:306-328`). There are no current `complete`, `limitations`, or `ambiguous` DTO members (`loomspan-console/internal/mcpadapter/trace_contracts.go:67-98`).

### 4. Artifact ownership, acquisition, expiry, and leases

Evidence has two internal source kinds. A target reference carries a target scope; an imported reference has no target scope. Artifact owners use the current target scope or a generated process-local imported-owner ID (`loomspan-console/internal/evidence/owner.go:11-70`, `loomspan-console/internal/artifact/service.go:66-67`, `loomspan-console/internal/artifact/service.go:110-134`).

Artifact handles are 32 random bytes represented as 64 lowercase hexadecimal characters. They contain no path, trace ID, or scope structure and are valid only in their issuing process and owner context (`loomspan-console/internal/artifact/handle.go:9-18`, `loomspan-console/internal/artifact/handle.go:20-58`).

Target acquisition keys state by `(owner, traceId)`. An installed unexpired entry returns the same handle; concurrent first acquisition callers join a single leader; stale/canceled acquisition state is removed before a new leader is installed (`loomspan-console/internal/artifact/service.go:145-235`). The acquisition leader loads authoritative metadata, streams and validates the artifact, processes derived components, atomically installs the bundle, and only then publishes it (`loomspan-console/internal/artifact/acquire.go:30-79`, `loomspan-console/internal/artifact/acquire.go:82-241`).

Imports are preflighted and fully processed as untrusted canonical NDJSON under the process-local imported owner. The trace and session identities come from processor validation, and a duplicate imported trace ID is rejected before installation (`loomspan-console/internal/artifact/import.go:12-15`, `loomspan-console/internal/artifact/import.go:35-89`). Imported evidence survives target rotation, as covered by `TestImportedEvidenceSurvivesTargetScopeRotation` (`loomspan-console/internal/artifact/import_test.go:98`).

`Lookup` is owner-scoped and side-effect-free. It reports no local copy for absent or expired evidence; it resolves imports to the process-local owner and rejects a stale target reference with `TARGET_CHANGED` (`loomspan-console/internal/artifact/service.go:419-474`). `Acquire` transparently starts a fresh target installation after ordinary unpinned expiry, but returns `ARTIFACT_EXPIRED` when the expired entry is still pinned for deferred removal (`loomspan-console/internal/artifact/service.go:168-195`).

Each analysis operation calls artifact `Use` through `leaseForHandle`. `Use` validates the reference owner and handle, enforces target generation and expiry, pins the installed entry, and returns a lease; the analysis operation closes that lease after success or failure (`loomspan-console/internal/artifact/service.go:265-293`, `loomspan-console/internal/traceanalysis/service.go:78-107`). This is the current immutable-evidence boundary shared by MCP and browser analysis.

### 5. Continuations and content references

Trace-analysis cursors encode operation, internal owner key, artifact handle, query fingerprint, and position/search state. Cursor validation first relies on current-owner lease acquisition, then checks owner, handle, and query fingerprint (`loomspan-console/internal/traceanalysis/cursor.go:150-215`, `loomspan-console/internal/traceanalysis/cursor.go:228-269`). Current messages are `TARGET_CHANGED`, `ARTIFACT_EXPIRED`, or `INVALID_CURSOR` according to which boundary fails.

Payload and failure-diagnostic references are base64url-encoded JSON containing the evidence source, artifact handle, content kind, and semantic content identifier. Validation requires both source and handle to match the caller-supplied evidence (`loomspan-console/internal/traceanalysis/content_ref.go:20-45`, `loomspan-console/internal/traceanalysis/content_ref.go:64-101`). `ReadPayloadRange` currently reports a mismatched or malformed reference as `INVALID_ARGUMENT: The payload reference is invalid.` before or after acquiring the handle-bound lease (`loomspan-console/internal/traceanalysis/query_ranges.go:17-60`).

The ticket's retained opaque-token behavior therefore has an existing internal owner/handle binding. The model-facing changes described by the ticket alter the recovery wording and remove caller-visible ownership inputs; the current token encoding itself is internal current-process state.

### 6. Cross-cutting runtime, skill, execution, activity, and error DTOs

Runtime currently embeds `consolecore.StatusSnapshot` directly in `RuntimeOutput` (`loomspan-console/internal/mcpadapter/runtime.go:20-23`). Its text fallback includes `targetScopeId` and `instanceId` alongside selection, connection, authentication, compatibility, runtime identity, live monitoring, and observation time (`runtime.go:58-81`). Capability identifiers are kept separately in the same result.

Shared MCP DTOs expose:

- `resourceUri` on skill list/detail items and target-scope/instance fields on skill results (`loomspan-console/internal/mcpadapter/contracts.go:32-59`);
- target-scope/instance fields on active-execution list/detail results, while each execution retains `sessionId` and `traceId` (`contracts.go:67-99`); and
- per-item instance IDs, top-level target-scope/instance IDs, and the internal `live.Continuity` type directly on recent activity (`contracts.go:101-132`).

Activity still uses target scope internally to bind and decode its opaque MCP continuation, verify upstream continuity ownership, and run publication checks (`loomspan-console/internal/mcpadapter/activity.go:31-93`). The text formatter also exposes the internal continuity target scope and instance (`activity.go:96-120`).

MCP domain errors currently map `consolecore.Error` directly to code, message, `targetScopeId`, and the entire `consolecore.Details` object (`loomspan-console/internal/mcpadapter/contracts.go:25-30`, `loomspan-console/internal/mcpadapter/contracts.go:148-163`). `consolecore.Details` includes expected/observed compatibility versions, `currentTargetScopeId`, `transportCategory`, limit name/value, and raw-download availability (`loomspan-console/internal/consolecore/errors.go:28-36`). Browser problem DTOs use those same internal details independently, including current-target and transport facts in checked fixtures such as `browser-fixtures/target/error-target-changed.json` and `browser-fixtures/target/error-unavailable.json`.

### 7. Custom MCP resources

The skill resource template is `loomspan://targets/{targetScopeId}/skills/{skillName}` and returns YAML plus scope, instance, registered name, and source path (`loomspan-console/internal/mcpadapter/resources.go:16-66`).

The six trace templates are target/imported variants of summary, frame, and record reads, all addressed by artifact handle; target variants also carry target scope (`loomspan-console/internal/mcpadapter/trace_resources.go:20-25`). They reuse the same trace-analysis operations and the same tool DTO mapping (`trace_resources.go:55-104`). Current tests cover URI canonicalization, target/imported parsing, target-free imported reads, and resource-domain error mapping (`loomspan-console/internal/mcpadapter/resources_test.go:14-67`, `loomspan-console/internal/mcpadapter/trace_resources_test.go:15-61`).

Tools are already documented as the portable complete contract, with resources described as supplementary/optional and with no raw-artifact resource (`loomspan-console/README.md:283-297`, `loomspan-console/agent-skills/loomspan-runtime-debugging/references/mcp-tool-guide.md:34-36`).

### 8. Tests and executable fixtures

Current contract and behavior coverage is distributed across these groups:

- schema closure, required branches, enum/bounds, and range limits: `loomspan-console/internal/mcpadapter/trace_contracts_test.go:12-50`;
- imported-without-target, invalid source/identity branches, acquisition, result facts, continuations, and both sources: `loomspan-console/internal/mcpadapter/traces_test.go:95-138`;
- full tools/list and resource-template advertisement: `loomspan-console/internal/mcpadapter/server_test.go:30-176`;
- trace semantic fixtures and browser/MCP semantic alignment: `loomspan-console/internal/mcpadapter/trace_semantic_fixtures_test.go:92`, `loomspan-console/internal/mcpadapter/trace_joined_adapters_test.go:55`;
- inventory source filtering, no-target partial results, deterministic pagination, catalog deduplication, changing installed sets, and target rotation: `loomspan-console/internal/traceinventory/service_test.go:46-198`;
- target acquisition single-flight/idempotence and installation safety: `loomspan-console/internal/artifact/acquire_test.go:22-477`;
- expiry and lease pinning: `loomspan-console/internal/artifact/expiry_test.go:12-283`, `loomspan-console/internal/artifact/lease_test.go:16-151`;
- imported ownership and target-rotation independence: `loomspan-console/internal/artifact/import_test.go:23-118`; and
- cursor/content-reference binding and rejection: `loomspan-console/internal/traceanalysis/cursor_test.go:11-268`, `loomspan-console/internal/traceanalysis/content_ref_test.go:12-106`.

Golden MCP fixtures currently contain the fields selected for removal in skill, execution, and activity results (`loomspan-console/internal/mcpadapter/testdata/skills-list.json:1`, `skill-detail.json:1`, `executions-list.json:1`, `execution-detail.json:1`, `activity.json:1`). These are MCP adapter fixtures, distinct from the browser fixtures that intentionally retain browser storage and ownership detail.

Agent evaluation cases named by the ticket currently encode the old model-facing expectations:

- `evidence-expired.json` expects `ARTIFACT_EXPIRED` and requires `artifactHandle`;
- `target-changed.json` requires both `targetScopeId` and `artifactHandle`;
- `incompatible-target.json` and `target-authentication-required.json` require `targetScopeId`;
- `slow-execution.json` requires `targetScopeId`, session, and sequence; and
- `mcp-without-skill.json` states that resources are optional.

The evaluation loader and server are under `loomspan-console/internal/agenteval`; the build-tool entry point is `loomspan-console/internal/buildtool/agent_eval.go`. The conformance harness creates a real MCP server from repository components in `loomspan-console/internal/buildtool/mcp_conformance.go:55-56`.

### 9. Documentation and installed skill

The current documentation consistently teaches the source/handle/scope lifecycle:

- the Console README lists all twelve tools, seven resources, source distinctions, handle expiry, and scope-bound continuations (`loomspan-console/README.md:238-314`);
- the skill-authoring trace guide tells callers to list target evidence, acquire by trace ID, then reopen target/imported copies by source plus handle (`ai/skill-authoring/traces-and-debugging.md:54-81`);
- the installed runtime-debugging skill names target scope and artifact handle as stable explanation identifiers and tells the model not to remap stale handles/resources/continuations (`loomspan-console/agent-skills/loomspan-runtime-debugging/SKILL.md:62-76`);
- the installed MCP tool guide says to respect each schema's source and identity fields and distinguishes `ARTIFACT_EXPIRED` from `TARGET_CHANGED` (`loomspan-console/agent-skills/loomspan-runtime-debugging/references/mcp-tool-guide.md:17-47`); and
- `references/runtime-model.md` describes target-scope identity, instance identity, and handle/resource/continuation lifetime (`loomspan-console/agent-skills/loomspan-runtime-debugging/references/runtime-model.md:1-29`).

`loomspan-console/docs/mcp-client-compatibility.md:6-9` also records the twelve tools and seven resources as the current compatibility-test target; its client matrix includes future resource walkthrough observations (`mcp-client-compatibility.md:51-55`).

### 10. Browser and Java-to-Go boundaries

The browser and MCP adapters share the concrete artifact service, trace-analysis service, and trace-inventory service in production assembly (`loomspan-console/internal/console/service.go:207`, `loomspan-console/internal/console/service.go:250-278`). The browser trace-analysis API accepts explicit source, target scope, and artifact handle and maps analysis results back to browser-specific DTOs. Browser fixtures under `loomspan-console/browser-fixtures/artifacts`, `trace-analysis`, and `target` record those existing contracts.

Target catalog and acquisition flow through the existing Console application adapter:

- `traceinventory.Service` calls `observability.Service.ListTraces` for the selected target (`loomspan-console/internal/traceinventory/service.go:28-30`, `service.go:191-220`);
- artifact acquisition uses `TraceLoader` for authoritative trace metadata and `StreamOpener` for the artifact bytes (`loomspan-console/internal/artifact/acquire.go:30-36`, `acquire.go:50-79`);
- `observability.Service` provides `ListTraces` and `GetTrace` (`loomspan-console/internal/observability/service.go:151-199`); and
- `applicationclient` uses `/_loomspan/observability/v1/traces`, `/traces/{traceId}`, and `/traces/{traceId}/artifact` (`loomspan-console/internal/applicationclient/address.go:146-151`).

The protected Java-to-Go fixtures are documented as the current-release semantic trace contract. `loomspan-console-fixtures/application-rest/` contains deterministic REST/problem bodies, `application-artifact/download-response.json` captures the artifact route, and all application snapshot/catalog/artifact access is gated by exact runtime `consoleCompatibilityVersion` (`loomspan-console-fixtures/README.md:42-69`). The ticket does not describe a change to those routes or payloads; the new behavior is an MCP-side orchestration and projection over them.

The repository's supported Java Application API is the closed eight-type allowlist described in the root README and enforced by `LoomspanPublicSurfaceArchitectureTest` (`README.md:157`, `loomspan-spring-boot-starter/src/test/java/com/lokiscale/loomspan/architecture/LoomspanPublicSurfaceArchitectureTest.java`). No affected Go type is part of that Java API, and the affected flow does not use or define a Java Supported SPI or Spring `@ConditionalOnMissingBean` replacement point.

## Contract Classification

The categories below use `ai/thoughts/framework-feature-design-lens.md:15-44`.

| Surface | Technical exposure and current evidence | Current classification |
| --- | --- | --- |
| Supported top-level Java types in `com.lokiscale.loomspan.api` | Closed allowlist and README documentation; no signature in the researched MCP flow exposes Go/internal/autoconfigure types. | **Application API**, unaffected by the ticket. |
| Java customization/bean replacement | Repository policy states there is no supported Loomspan SPI or internal bean-replacement surface. No affected Spring bean or `@ConditionalOnMissingBean` contract was found. | No affected **Supported SPI**. |
| YAML skill syntax and documented `loomspan.*` configuration | Skill YAML is returned as untrusted evidence, but this ticket does not alter YAML syntax, validation, configuration keys, or defaults. | **Configuration and manifest contracts**, consumed but not changed. |
| MCP tool names, schemas, result/error envelopes, capabilities, and custom resource templates | Loopback HTTP technical surface; advertised by tools/list/resources/templates/list, documented in the Console README, exercised by conformance tests, installed skill, golden fixtures, and agent evals. The roadmap records the implementation as pre-alpha and the ticket explicitly defines a coordinated breaking replacement without a shim. | Deliberately model-facing current behavior, but not Java Application API or Supported SPI. The trace payloads/results are best grouped with **Ephemeral diagnostic formats**; the adapter DTOs and Go interfaces are **Internal or accidentally exposed implementation**. |
| Raw canonical NDJSON, derived indexes, handles, cursors, payload references | Exact-version portable raw file is the narrow durable object; derived indexes, installed handles, cursors, content references, and catalogs are transient process state. | Raw file: narrow **Persisted or serialized contract** at exact compatibility version. Trace representation: **Ephemeral diagnostic format**. Derived state: **Internal or accidentally exposed implementation**. |
| `mcpadapter`, `traceinventory`, `artifact`, `traceanalysis`, `evidence`, `consolecore` exported Go declarations | Exported identifiers live under Go `internal/`; they are wired only inside `loomspan-console` and exposed to tests/build tools, not external Go consumers. | **Internal or accidentally exposed implementation**. |
| Browser REST DTOs and fixtures | Observable Console web contract with in-repository TypeScript/browser consumers. It intentionally contains ownership/storage detail and is separately tested. | Internal Console serialized/application-adapter contract; protected in-repository consumer, outside the ticket's MCP projection change. |
| Java-to-Go observability REST/SSE/problem/artifact routes and fixtures | Executable application and Console consumers, current-release compatibility marker, deterministic fixtures. | Cross-component serialized protocol contract; coordinated Java-to-Go changes would be required if altered. Current ticket leaves it behaviorally intact. |

No compatibility shim exists or is requested for the current MCP contract. The historical ticket states Loomspan has not released alpha and explicitly selects an atomic outward contract change while retaining the internal lifecycle.

## Architecture Documentation

The current trace path can be summarized as:

```text
MCP tool input
  -> mcpadapter source resolution / target capture
  -> optional target-only artifact Acquire(traceId)
  -> traceanalysis call(reference, artifactHandle)
  -> artifact Use(reference, handle)
  -> one owner-checked pinned lease
  -> parsed summary/page/range
  -> target publication check + MCP auth-generation check
  -> MCP DTO + deterministic text fallback
```

Inventory is a separate merge path:

```text
artifact StorageSnapshot + owner-scoped Lookup
  -> installed target/imported rows
  -> selected-target application catalog pages
  -> suppress catalog copies already installed for target
  -> storage-rich MCP inventory projection
```

The ticket's trace-ID behavior inserts an internal orchestration responsibility between MCP input and analysis. In the current repository, that responsibility is split across `artifact.Lookup`, `artifact.Acquire`, target capture/publication, and inventory/catalog access. The narrow `mcpadapter.TraceArtifactService` interface exposes only `Acquire`, while the concrete artifact service also exposes `Lookup`; the current `TraceInventoryService` exposes only paginated `List` (`loomspan-console/internal/mcpadapter/server.go:44-55`, `loomspan-console/internal/artifact/service.go:419-458`). There is therefore no existing single interface that performs all of the ticket's unique-owner, collision, installed-reuse, target-acquisition, and imported-fallback decision.

Security and consistency checks exist at several layers:

- loopback/bearer request admission and authentication-generation checks in MCP;
- target capture before an operation and `RequireCurrent` before publication;
- owner-scoped artifact keying and lookup;
- target-scope cancellation of in-flight acquisition and leases;
- immutable lease pinning per analysis call;
- cursor owner/handle/query fingerprint binding;
- content-reference source/handle binding;
- exact compatibility-marker validation during trace processing; and
- bounded page sizes, token lengths, request bodies, payload reads, and raw reads.

These layers explain why source/scope/handle facts can be removed from the MCP DTO projection without removing them from internal state or browser/application protocols.

## Historical Context (from ai/thoughts/)

- `ai/thoughts/phases/loomspan_skill_mcp_questions.md:30-55` establishes the method, parameter, and return-field burden-of-proof tests used by the ticket.
- `ai/thoughts/phases/loomspan_skill_mcp_questions.md:76-83` records `TRACE-IF-001` through `TRACE-IF-006` as ticketed: remove source filtering, artifact handles, storage-rich inventory, source routing, target/instance identifiers, and current MCP resources.
- `ai/thoughts/phases/loomspan_skill_mcp_questions.md:110-139` states the intended ordinary mental model: Console owns evidence resolution; the LLM continues with `traceId`; genuine identity ambiguity remains explicit.
- `ai/thoughts/phases/2026-08-15-loomspan-llm-trace-understanding-roadmap.md:7-27` identifies PRs 16-19 as the implemented baseline and this cleanup as the roadmap's first implementation step.
- `ai/thoughts/phases/2026-08-15-loomspan-llm-trace-understanding-roadmap.md:49-77` records the evaluated baseline behavior: large discovery surface, lifecycle machinery, the trace-ID-to-handle transition, and schema/context pressure.
- `ai/thoughts/phases/2026-08-15-loomspan-llm-trace-understanding-roadmap.md:114-126` preserves neutral composable MCP primitives and MCP usability without the installed skill.
- `ai/thoughts/framework-feature-design-lens.md:15-44` classifies compatibility surfaces and defines trace formats, exact-version portable artifacts, and derived handles/cursors/catalogs as current-process diagnostic machinery.

## Related Research

No prior files existed in `ai/thoughts/research/` at the time of this research. The directly related historical design sources are listed above.

## Open Questions

The following implementation-location facts are not selected by the current codebase; the ticket permits the implementation to choose among them:

- which internal package will own the new trace-ID resolver/orchestration seam;
- which exact internal interface will expose current-target and imported `Lookup` plus target acquisition to that seam;
- whether target availability probing for collision detection will use the existing catalog service, authoritative `GetTrace`, acquisition error classification, or a composed operation;
- which stable `consolecore.Code` spellings will be used for ambiguity and unrecoverable trace unavailability; and
- which subset DTO will replace direct `consolecore.Details` and direct `live.Continuity` exposure in MCP.

These are unimplemented choices rather than missing descriptions of current behavior. The ticket fixes their outward semantics while leaving their internal package/type placement open.
