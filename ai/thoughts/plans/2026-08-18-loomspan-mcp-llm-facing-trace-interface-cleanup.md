# LLM-Facing MCP Trace Interface Cleanup Implementation Plan

## Overview

Replace Loomspan Console's model-facing source/scope/artifact lifecycle with a trace-ID-first MCP contract. Console will resolve target and imported evidence internally while preserving owner isolation, target-generation checks, acquisition single-flight, immutable analysis leases, continuation/content-reference binding, authentication generation, bounded reads, and browser behavior.

This is an intentional atomic pre-alpha MCP contract break implementing `TRACE-IF-001` through `TRACE-IF-006`. It does not change Loomspan's supported Java API, create an SPI, or alter the Java-to-Go observability/artifact protocol.

## Current State Analysis

`mcpadapter.NewServer` registers twelve read-only tools and seven custom resource templates. The six trace tools make the caller select `TARGET` or `IMPORTED`, hand an `artifactHandle` from acquisition into later calls, and interpret storage/catalog state. Runtime, skill, execution, activity, trace, and error DTOs also expose target-scope or runtime-instance identities (`loomspan-console/internal/mcpadapter/server.go:26-69`, `loomspan-console/internal/mcpadapter/contracts.go:20-132`, `loomspan-console/internal/mcpadapter/trace_contracts.go:23-98`).

The current `resolveEvidence` helper only converts the caller's `source` into an owner reference. Only the target `get_trace(traceId)` branch performs acquisition; every other analysis call requires a caller-provided handle (`loomspan-console/internal/mcpadapter/traces.go:123-293`). There is no operation that resolves a `traceId` across installed current-target evidence, imported evidence, and target acquisition.

The underlying safety mechanisms already exist and must remain internal: owner-scoped `Lookup`, target-only idempotent/single-flight `Acquire`, random process-local handles, imported ownership, target invalidation, expiry, and handle-bound leases (`loomspan-console/internal/artifact/service.go:145-293`, `loomspan-console/internal/artifact/service.go:419-474`, `loomspan-console/internal/artifact/import.go:12-89`). Trace-analysis cursors and content references are also bound to owner and handle state (`loomspan-console/internal/traceanalysis/cursor.go:150-269`, `loomspan-console/internal/traceanalysis/content_ref.go:20-101`).

`traceinventory.Service` currently pages installed evidence before application-catalog evidence, suppresses only target-installed/catalog duplicates, and can emit a target/imported collision as two rows. Partial discovery is exposed as a storage-oriented nested catalog status rather than a domain-level completeness limitation (`loomspan-console/internal/traceinventory/service.go:69-223`, `loomspan-console/internal/traceinventory/dto.go:11-68`).

The browser shares artifact, trace-analysis, and trace-inventory services but maps its own source/scope/handle/storage DTOs. Those browser contracts are deliberately outside this MCP projection change (`loomspan-console/internal/console/service.go:207-278`).

## Desired End State

An MCP client can list finalized traces and call every trace inspection tool using `traceId` plus only question-specific filters, pagination, and range controls. The server resolves a unique installed or acquirable owner internally and returns direct domain errors when identity is ambiguous, evidence is unavailable, a continuation is stale, a payload reference must be refreshed, or the target changes.

The server continues to advertise the same twelve tool names and capability families, but advertises no custom resource templates. No MCP-defined input, result, error, fallback text, golden fixture, installed-skill instruction, or evaluation expectation exposes `sourceFilter`, evidence-routing `source`, `artifactHandle`, `targetScopeId`, `instanceId`, `resourceUri`, `resources`, or the removed inventory mechanics. Arbitrary YAML, trace record, diagnostic, payload, and raw-byte content remains inert data and is not recursively rewritten.

Verification consists of semantic schema/result leak tests, focused resolver/inventory/lifecycle tests, the full Go suite, MCP conformance, updated agent-evaluation fixture validation, documentation searches, and browser regression tests. A dedicated testing plan should be created with `ai/commands/3_testing_plan.md` before implementation to establish failing tests and exact exit criteria.

### Key Discoveries

- The concrete artifact service already has every low-level primitive needed by a resolver, but the current MCP-facing `TraceArtifactService` exposes only `Acquire` (`loomspan-console/internal/mcpadapter/server.go:44-55`, `loomspan-console/internal/artifact/service.go:419-458`).
- `observability.Service.GetTrace` provides an authoritative, non-artifact-download target existence check and returns stable `NOT_FOUND`, authentication, compatibility, target-change, and availability errors (`loomspan-console/internal/observability/service.go:183-199`). It should be used only when an installed import creates a possible target/import collision; ordinary target-only resolution can continue through `Acquire` without a redundant metadata probe.
- Installed target/import duplicates are visible in one storage snapshot, while imported/catalog collisions require an authoritative target probe. Catalog rows must suppress every installed trace ID, not only target-installed IDs, so pagination cannot later emit the same identity again (`loomspan-console/internal/traceinventory/service.go:124-223`).
- The target context distinguishes no selection (`INVALID_ARGUMENT` from `Capture`) from target rotation (`TARGET_CHANGED`) and supplies the final `RequireCurrent` publication barrier (`loomspan-console/internal/target/context.go:237-281`).
- MCP directly embeds `consolecore.StatusSnapshot`, `consolecore.Details`, and `live.Continuity`; adapter-owned projections are required to prevent future internal fields from leaking automatically (`loomspan-console/internal/mcpadapter/runtime.go:20-23`, `loomspan-console/internal/mcpadapter/contracts.go:20-30`, `loomspan-console/internal/mcpadapter/contracts.go:123-131`).
- Tools are already the complete portable path; resources are documented as optional duplicates, so removing all seven templates does not remove an investigation capability (`loomspan-console/README.md:283-297`).

## What We're NOT Doing

- Removing `sessionId`, changing its relationship to `traceId`, or compacting `LOOMSPAN_list_executions`.
- Renaming, adding, consolidating, or removing any of the twelve MCP tools or changing capability IDs.
- Redesigning `payloadRef`, activity continuation semantics, query filters, record representation defaults, page-size/start defaults, or duplicate structured/text result representations.
- Changing logical/physical record fields, raw forensic reads, canonical NDJSON, trace processing, or the exact `consoleCompatibilityVersion` rule.
- Changing browser DTOs, target selection, artifact import/storage management, or browser resources.
- Changing application REST/SSE/problem/artifact routes, Java observability production code, the supported Java API allowlist, Spring configuration, or manifest syntax.
- Adding a global trace-to-path map, durable handles, a mutable current-trace slot, a caller-visible replacement owner token, a compatibility shim, or a legacy MCP path.
- Broadening the post-implementation LLM walkthrough into another interface redesign; that evidence belongs in the later roadmap pass.

## Skill-Authoring Documentation Impact

**Impact**: Affected

- **Rationale**: The routed trace/debugging guidance currently teaches authors and debugging agents to select target/imported evidence, retain handles, reason about target scopes, navigate resources, and recover from artifact expiry. The executable author-facing workflow changes to `identify traceId -> inspect/query/read by traceId`, with explicit ambiguity, incomplete discovery, unavailable evidence, continuation restart, and payload-descriptor refresh behavior.
- **Documents to update**: `ai/skill-authoring/traces-and-debugging.md` and `ai/skill-authoring/README.md`.
- **Supporting evidence**: `loomspan-console/internal/traceresolution/service_test.go` (new), `loomspan-console/internal/traceinventory/service_test.go`, `loomspan-console/internal/mcpadapter/trace_contracts_test.go`, `loomspan-console/internal/mcpadapter/trace_semantic_fixtures_test.go`, `loomspan-console/internal/mcpadapter/server_test.go`, the installed `loomspan-runtime-debugging` skill/reference tests, and representative agent-evaluation cases.
- **Coverage table update**: Required. Keep the topic `Source-verified`, but replace the stale target/imported/handle/resource coverage description with trace-ID inspection, internal evidence resolution, compact completeness/ambiguity, opaque continuation/content-reference recovery, and the unchanged current-run/raw portability limitations. The routing boundary does not change.
- **LLM-first usability**: Keep the general authoring guide focused on author-facing trace semantics and link to the packaged operational playbooks. Use an exact workflow table and stable domain-code/recovery table; remove storage-lifecycle narrative that no longer changes an LLM decision. Preserve explicit current-run, sensitivity, portability, and unknown-evidence limitations.

## Contract and Compatibility Impact

| Surface | Classification and supporting evidence | Planned compatibility treatment |
| --- | --- | --- |
| Application API | No affected type. Loomspan's supported Java API remains the closed top-level allowlist in `LoomspanPublicSurfaceArchitectureTest`; no signature exposes the Go Console implementation. | Preserve unchanged and run the architecture test only if Java production types are touched unexpectedly. |
| Supported SPI | No affected extension point. Repository policy defines no supported Java SPI or bean-replacement surface, and this flow adds none. | Preserve the absence of an SPI; do not expose the resolver as Java or application API. |
| Configuration and manifest contracts | No YAML syntax, validation, default, `loomspan.*` property, capability RBAC, or manifest behavior changes. The skill-authoring trace/debugging guidance is affected because it documents the MCP evidence workflow. | Preserve executable configuration/manifest behavior; update authoring guidance atomically. |
| Persisted or serialized contracts | Canonical raw NDJSON and the exact-version Java-to-Go trace/artifact fixtures remain unchanged. MCP HTTP JSON is observable but is a pre-alpha model-facing diagnostic projection, not a durable cross-version artifact. | Preserve the raw artifact and compatibility marker; intentionally replace MCP schemas/results atomically with no legacy reader or dual envelope. |
| Ephemeral diagnostic formats | MCP tool schemas, trace inventory/analysis projections, errors, opaque-token recovery wording, and resource-template advertisement are affected. The ticket and roadmap explicitly approve the pre-alpha break. | Ship one coherent trace-ID contract; preserve diagnostic accuracy, gaps, uncertainties, safe bounds, inert content, and current-version security checks. |
| Internal or accidentally exposed implementation | Go `internal/` DTOs/interfaces, source-to-owner routing, inventory merge state, artifact handles, and custom resource handlers/tests are affected. Public Go identifiers under `internal/` do not establish supported API. | Add one internal resolver, replace adapter DTOs, delete obsolete resource paths, and update every in-repository caller/test/fixture atomically. Preserve owner/handle/scope facts below the adapter. |

- **Evidence of supported contracts**: the ticket's approved `TRACE-IF-001` through `TRACE-IF-006` decisions; Console README and installed-skill documentation; capability manifest and conformance tests; agent-evaluation cases; the Java API allowlist; and current browser/application consumers.
- **Intended breaks**: remove source selection/filtering and artifact handles from trace inputs/results; compact inventory; remove target/instance IDs from every MCP result/error; remove resource URIs/result links and all seven resource templates; replace storage-lifecycle recovery with trace-ID domain recovery. Tool names, capability IDs, parsed trace facts, range bounds, and authentication remain stable.
- **In-repository consumers to update**: MCP handlers/contracts/fallback text; server assembly; trace inventory and new resolver wiring; MCP unit, semantic, parity, HTTP, lifecycle, conformance, and golden-fixture tests; agent-evaluation server/cases; capability semantic manifest; packaged debugging skill/references; Console and MCP compatibility docs; and `ai/skill-authoring/` trace guidance. Browser consumers receive regression coverage, not contract changes.
- **Public-surface delta**: no Java types, signatures, constructors, Spring beans, or extension points are added or removed. The deliberate externally observable delta is confined to pre-alpha MCP JSON schemas/results/errors and removal of custom MCP resource templates. New Go declarations remain under `loomspan-console/internal`.
- **Shim decision**: **No shim.** There is no released protected MCP contract, and the ticket explicitly requires an atomic replacement. Aliases, optional legacy fields, dual schemas, fallback source preference, or replacement resource URIs would retain the rejected mental model.
- **Java-to-Go boundary coordination**: **Not required.** Target trace catalog/detail/artifact acquisition and canonical NDJSON consumption remain unchanged. The `consoleCompatibilityVersion` marker does not change. If implementation unexpectedly changes any application route, problem body, or consumed NDJSON semantic, stop and create a separate coordinated Java/Go/fixture decision rather than folding it into this work.

## Implementation Approach

Use contract-first tests to make every removal explicit, then add a small `internal/traceresolution` service that is the only MCP path from `traceId` to `(evidence.Reference, artifact.Handle, optional target.Scope)`. Keep trace analysis handle-based internally so each operation still obtains exactly one immutable lease. Handlers resolve immediately before the underlying analysis call and retain the existing target publication and MCP-authentication-generation checks.

Resolution uses installed owner-scoped lookup first. A current-target/import pair is `AMBIGUOUS_TRACE`. A unique target installation is reused. With no selected target, a unique import is usable; otherwise return `TRACE_UNAVAILABLE`. If an import exists with a selected target but no installed target copy, use `observability.GetTrace` to determine whether the target also claims the ID: `NOT_FOUND` permits imported fallback, a successful probe returns ambiguity, and inability to establish uniqueness returns the direct target/authentication/compatibility/target-change error rather than guessing. With no import, call the existing target `Acquire`; translate terminal `NOT_FOUND` and unrecoverable `ARTIFACT_EXPIRED` into `TRACE_UNAVAILABLE`, while retaining actionable target/authentication/compatibility/limit errors. Never retry outside existing safe acquisition behavior.

Keep inventory's installed-first, recent-first-within-segment deterministic ordering and opaque pagination, but group the installed snapshot by `traceId` before page boundaries. Probe a selected target for imported-only installed candidates so known catalog collisions are marked once as ambiguous; record probe/catalog failures as one `TRACE_DISCOVERY_INCOMPLETE` limitation with the safe message `Some trace evidence could not be checked; results may be incomplete.` Suppress catalog rows for all installed IDs so an identity emitted on an earlier page cannot reappear later. Internal owner and cursor fingerprints remain opaque and may retain scope/handle facts. `complete` is true when the storage snapshot succeeds and every evidence family available for the captured selection was checked; no selected target makes imported evidence the complete available family, while a selected target whose probe/catalog cannot be checked makes the result incomplete.

## Phase 1: Freeze the New Contract with Failing Tests

### Overview

Encode the intended tool surface and leak-prevention rules before production changes, without preserving the old contract as compatibility behavior.

### Changes Required

#### 1. Trace schema and result contract tests

**Files**: `loomspan-console/internal/mcpadapter/trace_contracts_test.go`, `loomspan-console/internal/mcpadapter/contracts_test.go`, `loomspan-console/internal/mcpadapter/server_test.go`

**Changes**:

- Assert that `LOOMSPAN_list_traces` accepts only `pageSize` and `continuation` and every other trace operation requires one bounded nonblank `traceId`.
- Preserve existing filter/order/representation/inline/range/continuation bounds and exactly-one `start`/`continuation` range behavior.
- Walk every MCP-defined input schema and representative structured result/error recursively, reporting the exact JSON path for forbidden property names. Apply the rule to DTO structure only; do not inspect arbitrary YAML, activity `details`, record payloads, diagnostics, or raw content values.
- Assert exactly twelve tools, unchanged read-only annotations/capability membership, and zero custom resource templates.
- Add explicit result-shape tests for compact inventory, compact evidence identity, runtime status, continuity, and error details.

```go
var forbiddenMCPProperties = map[string]bool{
    "sourceFilter": true, "source": true, "artifactHandle": true,
    "targetScopeId": true, "instanceId": true,
    "resourceUri": true, "resources": true,
}
```

#### 2. Behavior test matrix scaffolding

**Files**: `loomspan-console/internal/mcpadapter/traces_test.go`, `loomspan-console/internal/mcpadapter/trace_semantic_fixtures_test.go`, `loomspan-console/internal/traceinventory/service_test.go`, `loomspan-console/internal/mcpadapter/runtime_test.go`, `loomspan-console/internal/mcpadapter/skills_test.go`, `loomspan-console/internal/mcpadapter/executions_test.go`, `loomspan-console/internal/mcpadapter/activity_test.go`

**Changes**:

- Add failing cases for installed-target reuse, target acquisition, target-free import, import fallback after authoritative `NOT_FOUND`, collision, unavailable/expired evidence, target rotation, stale continuation, stale payload reference, and concurrent acquisition.
- Add inventory cases for installed owner grouping, imported/catalog collision, no duplicate across page boundaries, complete/incomplete empty results, compact limitations, and catalog suppression for every installed ID.
- Change cross-cutting expectations so internal target-generation checks still execute even though their IDs are absent from structured and text output.

### Success Criteria

#### Automated Verification

- [x] New tests compile and fail only because the old MCP contract/behavior remains: `cd loomspan-console && go test ./internal/mcpadapter ./internal/traceinventory`
- [x] Tests identify forbidden leaks by exact property path rather than broad serialized substring matching.
- [x] Existing authentication, annotation, range-bound, inert-content, and request-limit assertions remain represented.

#### Manual Verification

- [x] Review the expected schemas against every JSON example in the ticket.
- [x] Confirm test exclusions cover arbitrary untrusted content values but not adapter-owned wrapper objects.

---

## Phase 2: Add Central Trace-ID Resolution and Compact Inventory Semantics

### Overview

Implement the internal orchestration and inventory merge behavior without weakening the artifact lifecycle or changing browser DTOs.

### Changes Required

#### 1. Internal resolver

**Files**: `loomspan-console/internal/traceresolution/service.go` (new), `loomspan-console/internal/traceresolution/service_test.go` (new), `loomspan-console/internal/consolecore/errors.go`

**Changes**:

- Add `CodeAmbiguousTrace = "AMBIGUOUS_TRACE"` and `CodeTraceUnavailable = "TRACE_UNAVAILABLE"`.
- Define narrow internal interfaces for owner-scoped `Lookup`, target `Acquire`, target `GetTrace`, and target `Capture`/`RequireCurrent`; do not expose a general SPI.
- Validate the nonblank bounded `traceId` before any lookup/network call.
- Return a resolved internal value containing the reference, handle, and captured target scope only when unique; never publish those fields through MCP.
- Implement the exact decision table described in the implementation approach, including no-target imports, authoritative target `NOT_FOUND` fallback, explicit collisions, safe expiry translation, and no preference between owners.
- Test target rotation before/during resolution and leave final after-analysis publication validation to the handler.

```go
type Resolved struct {
    Reference evidence.Reference
    Handle    artifact.Handle
    Scope     target.Scope // zero for imported evidence
}

func (s *Service) Resolve(ctx context.Context, traceID string) (Resolved, *consolecore.Error)
```

#### 2. Resolver assembly

**Files**: `loomspan-console/internal/console/service.go`, `loomspan-console/internal/agenteval/server.go`, `loomspan-console/internal/mcpadapter/server.go`

**Changes**:

- Construct one resolver from the existing artifact, observability, and target services in production and evaluation assembly.
- Replace the MCP adapter's acquisition-only collaborator with a single `TraceResolver` interface; keep `TraceAnalysisService` handle/reference based.
- Do not register the resolver as a target owner or introduce retained mutable selection state; artifact and live services remain the owners invalidated by target rotation.

#### 3. Unified compact inventory

**Files**: `loomspan-console/internal/traceinventory/dto.go`, `loomspan-console/internal/traceinventory/service.go`, `loomspan-console/internal/traceinventory/cursor.go`, `loomspan-console/internal/traceinventory/service_test.go`

**Changes**:

- Remove the caller-facing source filter from `Query`; retain only page size and opaque continuation.
- Make `Result` expose `ObservedAt`, compact items, `Complete`, compact limitations, `HasMore`, and continuation. Keep owner/storage facts private to merge logic.
- Group installed entries by `traceId` before ordering/page boundaries. Consolidate target/imported owner claims into one `Ambiguous` item.
- For an imported-only installed row with a selected target, probe `GetTrace`; mark a successful target claim ambiguous, treat `NOT_FOUND` as a unique import, and convert other failures into safe compact limitations without nesting raw internal errors.
- Suppress application-catalog rows for every installed trace ID. Preserve opaque cursor query binding, deterministic ordering, installed-set fingerprint checks, cancellation, and bounded page size.
- Define an internal `LimitationCode` string type and the single outward code/message `TRACE_DISCOVERY_INCOMPLETE: Some trace evidence could not be checked; results may be incomplete.` Do not expose segment names, target scope, transport category, or nested errors.
- Keep `EnrichTargetCatalogPage` and browser-visible catalog/storage behavior unchanged; adapt shared private helpers only when behavior-preserving.

```go
type LimitationCode string

const LimitationTraceDiscoveryIncomplete LimitationCode = "TRACE_DISCOVERY_INCOMPLETE"

type Limitation struct {
    Code    LimitationCode
    Message string
}

type Entry struct {
    TraceID, SessionID, EntrySkill, Outcome string
    FinalizedAt time.Time
    Ambiguous   bool
}
```

### Success Criteria

#### Automated Verification

- [x] Resolver decision-table tests pass: `cd loomspan-console && go test ./internal/traceresolution`
- [x] Inventory grouping, pagination, completeness, and target-rotation tests pass: `cd loomspan-console && go test ./internal/traceinventory`
- [x] Artifact acquisition single-flight, expiry, lease, import ownership, and capacity tests remain green: `cd loomspan-console && go test ./internal/artifact ./internal/traceanalysis`
- [x] Browser inventory enrichment tests remain green: `cd loomspan-console && go test ./internal/browserapi`

#### Manual Verification

- [x] Review the resolver table to confirm every successful path establishes one unique owner and every uncertain collision path fails or reports incomplete discovery rather than guessing.
- [x] Confirm no process-global current trace or durable trace-ID mapping was introduced.

---

## Phase 3: Replace MCP Schemas, Projections, Errors, and Resources

### Overview

Switch all trace handlers to the resolver, compact every MCP-owned DTO, and remove the duplicate resource surface in one atomic adapter change.

### Changes Required

#### 1. Trace inputs, handlers, results, and recovery mapping

**Files**: `loomspan-console/internal/mcpadapter/trace_contracts.go`, `loomspan-console/internal/mcpadapter/traces.go`, `loomspan-console/internal/mcpadapter/traces_test.go`, `loomspan-console/internal/mcpadapter/trace_range_http_test.go`, `loomspan-console/internal/mcpadapter/trace_joined_adapters_test.go`, `loomspan-console/internal/mcpadapter/trace_semantic_fixtures_test.go`

**Changes**:

- Replace source/handle fields in all five inspection inputs with required `traceId`; remove `sourceFilter` from listing.
- Resolve once immediately before each underlying analysis call and pass the resolved internal reference/handle into the unchanged analysis request.
- Preserve one analysis lease, target `RequireCurrent` after successful analysis, MCP authentication-generation validation, query/range bounds, and inert content handling.
- Map analysis context to `{traceId, sessionId, observedAt}` only; remove resource links and all ownership/navigation/storage fields.
- Translate handle-bound stale cursor/payload failures at the adapter/resolver boundary into stable messages: restart the same query by `traceId`, or re-query the relevant record descriptor by `traceId`. Do not expose or request a handle.
- Update deterministic fallback text to the same compact semantics as structured JSON.

#### 2. Adapter-owned runtime, skill, execution, activity, and error DTOs

**Files**: `loomspan-console/internal/mcpadapter/contracts.go`, `loomspan-console/internal/mcpadapter/runtime.go`, `loomspan-console/internal/mcpadapter/skills.go`, `loomspan-console/internal/mcpadapter/executions.go`, `loomspan-console/internal/mcpadapter/activity.go`, associated tests, and `loomspan-console/internal/mcpadapter/testdata/*.json`

**Changes**:

- Introduce an MCP runtime status projection containing only observation time, target selection, connection, authentication, compatibility, runtime-identity state, and live-monitoring state; keep capability IDs.
- Remove scope/instance result fields and `resourceUri` from skills while preserving registered name, source path, YAML, observation time, and pagination.
- Remove scope/instance result fields from active executions while preserving all current execution facts, including `sessionId` and `traceId`.
- Remove top-level/per-item scope/instance fields from activity. Map `live.Continuity` to an MCP DTO with interval, cursors, observation, and reset facts only; keep scope/instance binding inside continuation validation.
- Replace direct `consolecore.Details` embedding with an allowlisted MCP details DTO containing compatibility versions, limit name/value, and raw-download availability. Remove top-level scope, `currentTargetScopeId`, and `transportCategory`; preserve stable code and safe message.
- Replace `appendCommon` with observation-only formatting and update all golden fixtures.

#### 3. Remove custom resources

**Files**: `loomspan-console/internal/mcpadapter/server.go`; delete `loomspan-console/internal/mcpadapter/resources.go`, `resources_test.go`, `trace_resources.go`, and `trace_resources_test.go`

**Changes**:

- Remove skill and trace resource registration and all URI/link construction.
- Assert `resources/templates/list` advertises no Loomspan templates while tools remain complete.
- Do not add `loomspan://traces/{traceId}` or any replacement resource URI.

#### 4. Capability and conformance fixtures

**Files**: `loomspan-console/internal/mcpadapter/capabilities.go`, `loomspan-console/internal/mcpadapter/contracts/trace-capabilities.json`, `loomspan-console/internal/mcpadapter/capabilities_test.go`, `loomspan-console/internal/mcpadapter/trace_semantic_fixtures_test.go`

**Changes**:

- Keep capability IDs and required tool membership unchanged.
- Replace source-binding/acquisition/expiry fixture semantics with trace-ID resolution, ambiguity, incomplete discovery, unavailable evidence, stale opaque-token recovery, and internal lifecycle-safety semantics; update manifest IDs and runner mappings atomically.
- Keep joined browser/MCP coverage to prove the browser can retain source/handle behavior while MCP projects trace identity only.

### Success Criteria

#### Automated Verification

- [x] MCP adapter and semantic fixture suites pass: `cd loomspan-console && go test ./internal/mcpadapter`
- [x] Production/evaluation server assembly compiles and tests pass: `cd loomspan-console && go test ./internal/console ./internal/agenteval`
- [x] Recursive contract tests find no forbidden adapter-owned property and no custom resource template.
- [x] Browser/MCP joined tests prove shared internal lifecycle behavior with distinct outward projections.
- [x] Tool count, tool names, read-only annotations, capability IDs, and authentication/request limits remain unchanged.

#### Manual Verification

- [x] Inspect actual `tools/list` schemas for all twelve tools and representative success/error JSON/text for runtime, skills, executions, activity, inventory, summary, frames, records, payload, and raw reads.
- [x] Verify that no removed concept reappears under a synonym such as owner, cache key, installed ID, evidence URI, or trace reference.

---

## Phase 4: Synchronize Skill Guidance, Documentation, Evaluations, and Release Evidence

### Overview

Teach the same trace-ID workflow everywhere shipped with or documenting the MCP server, then run repository-level verification.

### Changes Required

#### 1. Packaged Agent Skill and references

**Files**: `loomspan-console/agent-skills/loomspan-runtime-debugging/SKILL.md` and every file under `loomspan-console/agent-skills/loomspan-runtime-debugging/references/`, with behavioral rewrites centered in `mcp-tool-guide.md` and `runtime-model.md`

**Changes**:

- Replace source selection, handle handoff, target-scope comparison, resource navigation, and artifact-expiry repair with `traceId` discovery and direct inspection.
- Add concise decision guidance for incomplete inventory, ambiguity, trace unavailable, `TARGET_CHANGED`, stale continuation restart, and stale payload-reference descriptor refresh.
- Preserve capability bootstrap, tool neutrality, untrusted/sensitive content handling, current-run limits, and raw-forensics optionality.

#### 2. Skill-authoring knowledge base

**Files**: `ai/skill-authoring/traces-and-debugging.md`, `ai/skill-authoring/README.md`

**Changes**:

- Update the MCP inspection table and lifetime/recovery text to describe trace-ID-facing behavior while stating that opaque continuations/payload references remain current-process and query/content bound.
- Update implementation/test anchors to the resolver, inventory, contract, lifecycle, and joined-adapter tests.
- Revise the README coverage-table note as described in the impact assessment; retain the current routing entry and `Source-verified` confidence.

#### 3. Console and MCP compatibility documentation

**Files**: `loomspan-console/README.md`, `loomspan-console/docs/mcp-client-compatibility.md`, `loomspan-console/mcp-conformance/README.md`

**Changes**:

- Document the twelve trace-ID-first tools, compact discovery completeness, stable domain errors, no custom resources, and unchanged capabilities/security boundaries.
- Remove seven-template/resource walkthrough claims and storage-rich MCP field descriptions without changing browser documentation.
- Remove the conformance README's stale claim that the assembled HTTP suite includes a skill resource; retain its explanation that resource-specific official scenarios are outside a tools-only server.

#### 4. Agent-evaluation cases and harness fixtures

**Files**: `loomspan-console/agent-evals/cases/evidence-expired.json`, `target-changed.json`, `incompatible-target.json`, `target-authentication-required.json`, `slow-execution.json`, `mcp-without-skill.json`, `evidence-unavailable.json`, a new `ambiguous-trace.json`, `loomspan-console/internal/agenteval/server.go`, `server_test.go`, and `fixtures_test.go`

**Changes**:

- Expect transparent reacquisition or `TRACE_UNAVAILABLE`, never `ARTIFACT_EXPIRED`/handle repair.
- Preserve `TARGET_CHANGED`, compatibility, authentication, activity progress, and no-mixed-evidence facts while removing scope/handle/instance identifier requirements.
- State that tools—not optional resources—are the complete MCP path.
- Add or adapt an ambiguity case that requires `AMBIGUOUS_TRACE`, conflict resolution in Console, and no silent owner preference.
- Update evaluation server tool arguments and imported-trace setup to use `traceId` only.

#### 5. Final leak audit and repository verification

**Files**: all touched files; remaining search hits inspected individually

**Changes**:

- Search for every removed field/code/instruction in MCP, skill, evaluation, and documentation surfaces. Classify remaining hits as internal lifecycle, browser-only contract, historical ticket/research/plan, arbitrary test data, or explicit negative assertion.
- Run formatting, focused tests, full Go tests, conformance, agent-evaluation fixture validation, and browser tests if shared fixtures or parity code changed.

### Success Criteria

#### Automated Verification

- [x] Go formatting is clean: `cd loomspan-console && gofmt -w <touched-go-files> && gofmt -l <touched-go-files>` returns no files.
- [x] Focused suites pass: `cd loomspan-console && go test ./internal/traceresolution ./internal/traceinventory ./internal/artifact ./internal/traceanalysis ./internal/mcpadapter ./internal/agenteval ./internal/browserapi`
- [x] Full Console suite passes: `cd loomspan-console && go test ./...`
- [x] MCP conformance passes for both supported protocol revisions: `cd loomspan-console && go run ./internal/buildtool mcp-conformance`
- [x] Agent-evaluation case loading/server tests pass as part of `go test ./internal/agenteval`; any selected live model runs are recorded through the documented harness rather than treated as deterministic unit tests.
- [x] If browser fixtures/parity code changed, web verification passes: `cd loomspan-console/web && npm run typecheck && npm test`.
- [x] If any Java production type changes unexpectedly, run `./mvnw -pl loomspan-spring-boot-starter -Dtest=LoomspanPublicSurfaceArchitectureTest test` and the applicable Java integration tests; otherwise record Java testing as not required.
- [x] Skill-authoring guidance is supported by the cited tests/source, its coverage row is updated, and it satisfies the LLM-First Authoring Standard.

#### Manual Verification

- [ ] Run representative MCP calls for exact target trace, target-free import, target/import collision, missing evidence, expired evidence, target rotation, stale continuation, stale payload reference, and a large bounded payload/raw range.
- [x] Confirm a complete empty inventory supports “none found,” while an incomplete empty inventory does not.
- [x] Confirm the packaged skill and MCP-only workflow both choose tools using only `traceId` and question-specific parameters.
- [x] Inspect every remaining repository search hit for `sourceFilter`, `artifactHandle`, `targetScopeId`, `instanceId`, `resourceUri`, `resources`, and `ARTIFACT_EXPIRED` in context.

---

## Testing Strategy

### Unit Tests

- Resolver decision table: unique installed target, unique target-free import, target acquisition, authoritative import fallback, collision, no evidence, expiry, cancellation, and target rotation.
- Inventory merge/pagination: owner grouping, catalog/import collision, no duplicate identity across pages, deterministic ordering, compact limitation, and complete/incomplete negative conclusions.
- Adapter DTO/schema mapping: required `traceId`, preserved bounds/defaults, allowlisted runtime/error/continuity fields, compact identity, and deterministic fallback text.
- Lifecycle invariants: acquisition single-flight, owner isolation, lease pinning, stale cursors/content references, capacity, and authentication generation.

### Integration Tests

- Production MCP server discovery/calls with zero resources and unchanged twelve-tool/capability surface.
- Joined browser/MCP tests showing one shared artifact remains storage-rich in browser responses and trace-ID-only in MCP responses.
- Agent-evaluation server scenarios for imported evidence, target acquisition, unavailable/expired evidence, ambiguity, and target changes.
- Official MCP conformance for both supported protocol revisions.

**Note**: Run `ai/commands/3_testing_plan.md` before implementation to create the dedicated failing-test order, exact fixture updates, commands, and exit criteria.

### Manual Testing Steps

1. Start an isolated MCP instance with one target trace and one imported trace; inspect each from listing through payload/raw reads without retaining any identity except `traceId` and returned opaque range/content tokens.
2. Install an imported trace whose ID is also available from the selected target and confirm listing marks it ambiguous and inspection returns `AMBIGUOUS_TRACE` without selecting an owner.
3. Rotate the target during resolution and during analysis; confirm `TARGET_CHANGED` contains no scope/instance/handle and no old evidence is published.
4. Expire evidence, retry by `traceId`, and confirm safe reacquisition or `TRACE_UNAVAILABLE`; restart stale queries and refresh stale record descriptors using only `traceId`.
5. Inspect runtime, skill, execution, activity, trace, and error JSON/text plus `resources/templates/list` for removed fields and templates.

## Performance Considerations

- Reuse installed target/import handles before network access and retain artifact-service single-flight acquisition for concurrent first reads.
- Only issue an authoritative `GetTrace` probe when an installed import creates a possible target collision; do not double-probe ordinary target-only acquisition.
- Keep inventory page size at 64, reuse the storage snapshot, group in memory by bounded installed capacity, and avoid artifact downloads during listing.
- Preserve one lease per underlying analysis operation and existing range/page/request/token limits.
- Add tests that concurrent same-trace MCP calls still converge on one acquisition and do not increase lease/capacity usage unexpectedly.

## Migration Notes

There is no runtime data migration and no compatibility shim. Existing process-local handles, continuations, payload references, imported artifacts, and browser state keep their internal lifecycle, but old MCP clients must adopt the new schemas atomically. Console restart already discards transient installed evidence; canonical raw traces remain same-version portable under the unchanged compatibility marker.

Roll back only as a complete MCP adapter/documentation/evaluation unit. Do not partially restore source/handle fields or resource registration because a mixed surface would teach contradictory workflows while sharing the same capability IDs.

## References

- Original ticket: `ai/thoughts/tickets/loomspan-mcp-llm-facing-trace-interface-cleanup.md`
- Related research: `ai/thoughts/research/2026-08-18-loomspan-mcp-llm-facing-trace-interface-cleanup.md`
- Framework design lens: `ai/thoughts/framework-feature-design-lens.md`
- Skill-authoring routing and standard: `ai/skill-authoring/README.md`
- Current author-facing trace guidance: `ai/skill-authoring/traces-and-debugging.md`
- Accepted interface ledger: `ai/thoughts/phases/loomspan_skill_mcp_questions.md:30-139`
- Trace-understanding roadmap: `ai/thoughts/phases/2026-08-15-loomspan-llm-trace-understanding-roadmap.md:7-126`
- Current MCP assembly and handlers: `loomspan-console/internal/mcpadapter/server.go:26-69`, `loomspan-console/internal/mcpadapter/traces.go:99-293`
- Current inventory and artifact lifecycle: `loomspan-console/internal/traceinventory/service.go:69-302`, `loomspan-console/internal/artifact/service.go:145-293`, `loomspan-console/internal/artifact/service.go:419-474`
