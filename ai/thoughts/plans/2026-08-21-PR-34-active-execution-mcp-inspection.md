# PR 34 Active-Execution MCP Inspection Implementation Plan

## Overview

Improve the existing read-only active-execution MCP primitives so a capable
client can review one bounded page of live executions, understand each
execution's orientation and evidence coverage, retain checkpoints for a later
observation, and hand a completed execution off to trace inspection without
probing undocumented result fields. The change keeps live evidence provisional,
bounded, target-bound, and safe for untrusted diagnostic content; it does not
add a fleet-specific tool.

## Current State Analysis

`LOOMSPAN_list_executions` and `LOOMSPAN_get_execution` already return the same
complete `executionDTO`, but the compact discovery schema and list text expose
only a subset. A structured-output client can orient from the list plus one
activity call per execution, while schema-driven and text-only clients need
result-shape probing or additional detail calls
(`loomspan-console/internal/mcpadapter/contracts.go:65-99`,
`loomspan-console/internal/mcpadapter/executions.go:61-78`,
`loomspan-console/internal/mcpadapter/executions.go:112-179`).

The exact committed `tools/list` response is 23,495 bytes against a 23,552-byte
ceiling, leaving 57 bytes. The compatibility document is stale at 23,390 bytes.
Complete orientation fields therefore cannot simply be added under the current
ceiling (`loomspan-console/internal/mcpadapter/output_schemas.go:13-60`,
`loomspan-console/internal/mcpadapter/output_schemas.go:165-185`,
`loomspan-console/internal/mcpadapter/server_test.go:89-108`).

Activity has one useful opaque token with two behaviors: `hasMore` says whether
another matching item is retained now, while the returned token remains a
forward checkpoint even after `hasMore: false`. The result's
`beginningUnavailable` flag instead describes the shared global ring, so
unrelated global eviction can misleadingly appear to describe the selected
session (`loomspan-console/internal/mcpadapter/activity.go:66-86`,
`loomspan-console/internal/live/service.go:658-752`). The service does not yet
retain enough per-session admission state to distinguish a session observed
continuously from `TRACE_STARTED`, one known to have lost activity, and one
whose beginning was never observed.

Java correctly increments provider attempts before publishing the matching
request activity, but its active observability `Usage` and `QuotaLimits` DTOs
omit `providerAttempts` and `maxProviderAttempts`. Go decodes the absent members
as zero and republishes those zeros through MCP. This is a deterministic
Java-to-Go protocol omission, not projector timing
(`loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/runtime/observation/LiveActivityProjector.java:48-112`,
`loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/observability/web/dto/ObservabilityDtos.java:33-49`,
`loomspan-console/internal/observability/dto.go:38-58`).

The canonical Agent Skill remains a six-file, exact-topology package. Loomspan
has not released, so the skill remains unversioned during development rather
than declaring or negotiating a release version.
It has a single-execution slow workflow but no bounded all-active route,
second-observation checkpoint procedure, session-start coverage vocabulary, or
completion-race handoff. The evaluation server and release matrix likewise
exercise only one active execution
(`loomspan-console/agent-skills/loomspan/SKILL.md:1-104`,
`loomspan-console/internal/agentskills/validate.go:18-30`,
`loomspan-console/internal/agenteval/server.go:154-282`,
`loomspan-console/internal/agenteval/score.go:51-97`).

## Desired End State

- `LOOMSPAN_list_executions` advertises and returns every ordinary orientation
  field: stable identities, entry skill, status/phase, latest canonical
  sequence, timestamps/elapsed time, summary, bounded active path and
  truncation, observed usage, and configured limits. List and detail keep one
  shared explicitly described execution shape.
- Activity discovery names canonical sequence, execution/frame/route identity,
  returned cursor range, continuity, and mechanically observed coverage facts.
  The pre-v1 `beginningUnavailable` field is replaced atomically, without an
  alias, by a `coverage` object containing only cursors that Console actually
  observed:
  - optional `globalEvictedThroughCursor` — the latest cursor physically
    evicted from the shared ring in the current continuity interval;
  - optional `sessionStartCursor` — the admitted `TRACE_STARTED` cursor for the
    selected session when Console observed it in the current interval;
  - optional `sessionEvictedThroughCursor` — the latest physically evicted
    cursor belonging to the selected session; and
  - optional `sessionRetainedCursorRange` — the first and last selected-session
    cursors physically retained at `observedAt`.
- MCP does not turn those facts into `COMPLETE`, `INCOMPLETE`, `UNKNOWN`, health,
  progress, or stuck states. Missing optional coverage facts remain missing and
  must not be converted into a positive or negative conclusion. The existing
  continuity reset and returned cursor range remain separate recorded facts.
- Existing producer-owned fields such as execution `status`, `phase`, `summary`,
  and activity `summary` remain unchanged and are exposed as untrusted runtime
  facts. The Go live/MCP layers do not create a new status, rewrite a summary,
  or emit a “making progress”/“stuck” conclusion. A client may compare observed
  canonical sequences, cursors, and timestamps, but that comparison is outside
  the MCP result contract.
- Activity retains one opaque, target/scope/session-bound `continuation` field.
  `hasMore` is explicitly the retained-backlog signal; the continuation is
  explicitly a reusable future checkpoint regardless of `hasMore`. Empty calls
  may advance it to the current continuity boundary. No alias or second token
  is introduced.
- Java REST includes all six usage-limit dimensions, Go rejects an active DTO
  that omits required usage/limit members instead of silently manufacturing
  zero, and MCP/text render the observed values. Usage zero means an observed
  counter that has not accrued; configured-limit zero means disabled/unlimited
  enforcement. In-flight provider attempts can be positive while response-only
  counters remain zero.
- A full structured-output page needs no per-execution detail calls for shape
  completeness. Deterministic list text also includes the same orientation
  facts, while activity text continues to omit arbitrary `details`.
- The canonical skill adds `WF-ACTIVE-EXECUTION-REVIEW`: list at most one
  64-item page, make one bounded activity call per execution, retain every
  checkpoint, optionally perform one later observation, and stop or report
  pagination instead of scanning without a developer request.
- If an execution disappears between calls, the workflow uses the already
  returned `sessionId` for retained activity and the already returned `traceId`
  for one trace-resolution attempt. `TRACE_UNAVAILABLE` remains an honest
  result; disappearance never proves finalized evidence exists and never
  triggers unrelated trace inventory scanning.
- Deterministic text serializes the same named facts as stable key/value lines;
  it does not add a synthesized per-execution diagnosis, coverage label, or
  recommendation. Tool descriptions explain contract semantics but do not
  create run-specific evidence.
- The exact `tools/list` snapshot fits a deliberate `25 << 10` (25,600-byte)
  ceiling, and its actual post-change byte count and remaining headroom are
  recorded in tests and client-compatibility documentation.

### Key Discoveries

- List and detail share `mapExecution`; detail is a later freshness/existence
  observation, not a richer structured projection
  (`loomspan-console/internal/mcpadapter/executions.go:61-102`).
- An initial activity query returns the newest suffix and no retained older
  backlog; a checkpointed query pages forward through retained matches
  (`loomspan-console/internal/live/service.go:682-744`).
- A successful close publishes the trace catalog and terminal activity before
  removing the active registry entry, but failed finalization can end
  observation without available trace evidence
  (`loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/runtime/observation/DefaultExecutionObservationHandle.java:97-214`).
- Provider attempts accrue on every physical send, while model calls and token
  units accrue only on a response; zero is therefore valid during an in-flight
  request (`loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/chat/ProviderAttemptCallAdvisor.java:58-96`).
- Skill packaging requires exactly `SKILL.md` plus five named references and
  copies their bytes unchanged into release archives
  (`loomspan-console/internal/agentskills/validate.go:52-162`,
  `loomspan-console/internal/buildtool/package.go:70-95`).

## What We're NOT Doing

- Adding a fleet-review MCP tool, custom MCP resources, or process-global
  browser-selected MCP state.
- Adding execution mutation, cancellation, retry, pause, or control operations.
- Creating durable activity history, analytics, audit logs, cross-run
  comparison, or cross-version live-format compatibility.
- Reading model/tool payloads, arbitrary activity details, YAML, raw artifacts,
  credentials, or internal ownership/scope identifiers during ordinary active
  orientation.
- Treating a quiet interval as stuckness, missing usage as zero, or an inactive
  session as proof that a finalized trace exists.
- Adding pre-v1 field aliases, dual coverage behavior, legacy readers, or
  compatibility shims.
- Changing the supported top-level Java application API, adding a Java SPI, or
  changing `loomspan.session.quotas.*` configuration defaults/enforcement.
- Expanding `ai/skill-authoring/` with Console operating playbooks that belong
  in the portable runtime-debugging skill.

## Skill-Authoring Documentation Impact

**Impact**: No impact

- **Rationale**: The change affects Console runtime inspection and the packaged
  debugging Agent Skill, not YAML manifest syntax, skill inputs/outputs,
  mappings, planning semantics, capability visibility, quotas, or author-facing
  execution behavior. The existing authoring guide already states the correct
  provider-attempt/response accrual semantics; the Java REST fix restores the
  diagnostic consumer to that behavior. `ai/skill-authoring/README.md:41-64`
  deliberately routes runtime diagnosis to the packaged Agent Skill, and
  `ai/skill-authoring/traces-and-debugging.md:10-32` avoids duplicating Console
  playbooks.
- **Documents to update**: None under `ai/skill-authoring/`.
- **Supporting evidence**: `ModelAttemptCallAdvisorIntegrationTest`,
  `LiveActivityProjectorTest`, `ObservabilityDtoMapperTest`, the Java-generated
  active REST fixtures, Go observability tests, MCP goldens, and the canonical
  skill/evaluation cases will establish the changed diagnostic presentation.
- **Coverage table update**: Not required; no authoring topic is added and no
  coverage/confidence classification changes.
- **LLM-first usability**: Not applicable to the separate authoring knowledge
  base. Runtime guidance remains self-contained and progressively routed in
  `loomspan-console/agent-skills/loomspan/`.

## Contract and Compatibility Impact

| Surface | Classification and supporting evidence | Planned compatibility treatment |
| --- | --- | --- |
| Application API | No affected top-level allowlisted type in `com.lokiscale.loomspan.api`; `LoomspanPublicSurfaceArchitectureTest` is the executable authority. | Preserve; zero public API delta. |
| Supported SPI | Loomspan exposes no supported SPI involved in observation, live state, or Console mapping. | Preserve; do not create an SPI or bean replacement contract. |
| Configuration and manifest contracts | `loomspan.session.quotas.*` nonnegative values, defaults, and zero-as-disabled behavior remain unchanged. The portable Agent Skill is an unreleased, unversioned development artifact whose exact files are packaged with the Console. | Preserve quota behavior; update the canonical skill contents, validator, package/readme declarations, tests, and evaluations atomically without introducing skill release metadata or runtime version negotiation. |
| Persisted or serialized contracts | No durable/cross-version trace format changes. The active REST JSON is an exact-release Java-Go protocol, not durable interchange. | Change Java/Go REST members atomically; regenerate current fixtures. No migration or legacy reader. |
| Ephemeral diagnostic formats | Active snapshots, activity coverage, text, compact discovery, and continuations are current-process evidence. MCP is a deliberately supported pre-v1 Console contract. | Intentional in-place break: replace `beginningUnavailable`, expose full orientation names, and document the existing checkpoint behavior in one coherent version. Preserve boundedness, target checks, opacity, and untrusted-content handling. |
| Internal or accidentally exposed implementation | Java observability records/mappers, Go live state, browser DTO/state, and adapter structs are internal even where Java visibility is public. | Update/remove obsolete fields and constructors atomically; no overloads, aliases, or bridges. |

The unversioned decision is scoped to the canonical Agent Skill's release
identity. Existing technical identifiers such as the Console build version,
`consoleCompatibilityVersion`, MCP protocol version, and evaluation-record
`schemaVersion` remain because they identify executable or serialized contracts;
none is a skill/server version handshake.

- **Evidence of supported contracts**: the closed Java allowlist and README API
  summary; the PR 34 ticket; property metadata; MCP exact snapshot, conformance,
  and compatibility docs; canonical Agent Skill validator/package tests; and
  verified browser/MCP/Go consumers.
- **Intended breaks**: replace MCP/browser `beginningUnavailable` with explicit
  global/session cursor facts; enlarge and rename the exact compact discovery
  snapshot; remove premature skill-version metadata while updating the canonical
  Agent Skill content. These
  are approved pre-v1 changes and all repository consumers move together.
- **In-repository consumers to update**: Java DTO mapper/tests and generated REST
  fixtures; Go observability/live/MCP/browser adapters and tests; TypeScript
  activity contracts/state/presentation/tests; MCP goldens and discovery
  snapshot; README and compatibility docs; all six canonical skill files as
  applicable; skill validation/package/smoke declarations; agent-eval cases,
  server, scorer matrix, fixtures, docs, and recorded result summaries; active
  roadmap status.
- **Public-surface delta**: none in `com.lokiscale.loomspan.api`; no supported
  constructor, signature, or Spring extension point is added or removed.
- **Shim decision**: **No shim.** MCP and the portable skill are pre-v1, the
  Java-Go protocol is exact-release, and no independently protected old
  consumer justifies aliases or dual behavior.
- **Java-to-Go boundary coordination**: **Required.** Add
  `providerAttempts`/`maxProviderAttempts` to Java active REST DTOs and mapping,
  validate presence in Go, regenerate the Java-owned fixtures, update Go/MCP
  fixtures and tests, and ship all changes together. The compatibility marker
  remains derived from the common project release; no separate marker constant
  is introduced or manually bumped because exact release equality already
  rejects mixed released builds.

## Implementation Approach

Evolve the existing primitives in dependency order. First make the producer
boundary complete and record per-session live coverage cursors. Then
publish one explicit MCP contract and deterministic text representation, with
an exact measured discovery budget. Finally teach and evaluate the bounded
workflow, update all package/documentation authorities, and run the full
cross-language verification. Tests should be added failing-first as detailed by
the subsequent `3_testing_plan.md` artifact.

The design intentionally chooses richer discovery over a new tool. It also
keeps one continuation rather than inventing two authority-bearing tokens:
`hasMore` answers the present-backlog question and `continuation` is the
position/checkpoint. The new coverage object contains only admitted, retained,
or evicted cursor facts. Completeness and progress remain client interpretations,
not MCP result states.

## Phase 1: Restore Complete Active Usage Across Java and Go

### Overview

Repair the exact-release active REST protocol so every usage and configured
limit dimension reaches Console, and make missing wire members fail closed
instead of becoming plausible zero values.

### Changes Required

#### 1. Java observability DTO and mapper

**Files**:

- `loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/observability/web/dto/ObservabilityDtos.java`
- `loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/observability/web/ObservabilityDtoMapper.java`

**Changes**:

- Add `providerAttempts` to `Usage` and `maxProviderAttempts` to `QuotaLimits`
  in the same ordering used by runtime snapshots and MCP DTOs.
- Map the existing `SessionUsageSnapshot.providerAttempts()` and
  `LoomspanProperties.Session.Quotas.getMaxProviderAttempts()` values.
- Keep all values as required nonnegative integers; do not add nullable or
  availability variants.

#### 2. Java behavioral and fixture evidence

**Files**:

- `loomspan-spring-boot-starter/src/test/java/com/lokiscale/loomspan/internal/observability/web/ObservabilityDtoMapperTest.java`
- `loomspan-spring-boot-starter/src/test/java/com/lokiscale/loomspan/internal/observability/web/ObservabilityRestIntegrationTest.java`
- `loomspan-spring-boot-starter/src/test/java/com/lokiscale/loomspan/internal/observability/web/ConsoleRestFixtureCorpusTest.java`
- `loomspan-spring-boot-starter/src/test/java/com/lokiscale/loomspan/internal/chat/ModelAttemptCallAdvisorIntegrationTest.java`
- `loomspan-spring-boot-starter/src/test/java/com/lokiscale/loomspan/internal/runtime/observation/LiveActivityProjectorTest.java`
- `loomspan-console-fixtures/application-rest/active-execution-detail.json`
- `loomspan-console-fixtures/application-rest/active-executions-page.json`

**Changes**:

- Add a deterministic retry/in-flight assertion proving the registry snapshot
  contains a provider attempt before response-only counters accrue and before
  the matching activity is externally observed.
- Assert DTO/REST JSON carries a nonzero provider count and a configured
  provider limit, plus zero-limit serialization for disabled enforcement.
- Regenerate the Java-owned active REST fixtures rather than editing generated
  JSON by hand.

#### 3. Go wire validation and downstream fixtures

**Files**:

- `loomspan-console/internal/observability/dto.go`
- `loomspan-console/internal/observability/service.go`
- `loomspan-console/internal/observability/dto_test.go`
- `loomspan-console/internal/observability/service_test.go`
- `loomspan-console/internal/mcpadapter/testdata/execution-detail.json`
- `loomspan-console/internal/mcpadapter/testdata/executions-list.json`

**Changes**:

- Decode active usage/limit JSON through presence-aware wire members (or an
  equivalent strict decoder) and reject an active snapshot missing any required
  counter/limit, especially the two provider fields.
- Normalize only after presence and nonnegative validation so an observed zero
  remains distinguishable from an omitted member at the boundary.
- Update derived Go/MCP goldens to carry the Java fixture's real provider
  values.

### Success Criteria

#### Automated Verification

- [x] Focused Java usage/projection/REST tests pass:
  `.\mvnw.cmd -pl loomspan-spring-boot-starter "-Dtest=ModelAttemptCallAdvisorIntegrationTest,LiveActivityProjectorTest,ObservabilityDtoMapperTest,ObservabilityRestIntegrationTest,ConsoleRestFixtureCorpusTest" test`
- [x] Go rejects omitted provider fields and preserves observed/disabled zeros:
  `go test ./internal/observability ./internal/mcpadapter` from `loomspan-console/`.
- [x] Java-generated REST fixture bytes and Go consumers agree.

#### Manual Verification

- [ ] Inspect one in-flight provider request and confirm active usage can show
  `providerAttempts > 0` with `modelCalls == 0` without describing a defect.
- [ ] Confirm a configured `maxProviderAttempts == 0` is described as disabled,
  not unavailable or exhausted.

---

## Phase 2: Record Session Coverage Facts in Live Activity

### Overview

Separate shared-ring facts from selected-session facts and retain only the
minimal bounded cursor evidence needed for a client to state limitations
without Console deriving a completeness classification.

### Changes Required

#### 1. Live domain contract and accounting

**Files**:

- `loomspan-console/internal/live/dto.go`
- `loomspan-console/internal/live/service.go`
- `loomspan-console/internal/live/service_test.go`
- `loomspan-console/internal/live/coordinator_test.go`

**Changes**:

- Replace `RecentResponse.BeginningUnavailable` with a coverage record carrying
  optional `GlobalEvictedThroughCursor`, `SessionStartCursor`,
  `SessionEvictedThroughCursor`, and `SessionRetainedCursorRange` facts.
- On physical ring eviction, record the evicted global cursor and, while later
  items for that session remain retained, the selected-session cursor. On
  `TRACE_STARTED` admission, record its exact cursor. Calculate the retained
  selected-session range from actual ring items at read time.
- Reset these cursor facts with the continuity interval and remove per-session
  bookkeeping when the bounded ring no longer contains an item for that
  session. Do not grow a process-lifetime session catalog.
- For an already-active baseline session whose `TRACE_STARTED` was not admitted,
  omit `sessionStartCursor`. For a session with no observed eviction, omit
  `sessionEvictedThroughCursor`. Absence stays absence; do not emit a Boolean,
  enum, status, explanatory message, or inferred completeness value.
- Keep cursor selection, exact complete-item admission, ring byte/item limits,
  cancellation, and checkpoint construction unchanged.
- Add focused cases for unrelated global eviction followed by a later selected
  session start, selected-session eviction, baseline-without-start, reset,
  cursor not found, empty filtered results, and terminal activity retained
  after active registry removal. Assert exact cursors rather than classifications.

#### 2. Browser API and presentation

**Files**:

- `loomspan-console/web/src/api/contracts.ts`
- `loomspan-console/web/src/activity/ActivityProvider.tsx`
- `loomspan-console/web/src/activity/reducer.ts`
- `loomspan-console/web/src/activity/LiveActivity.tsx`
- `loomspan-console/web/src/api/client.test.ts`
- `loomspan-console/web/src/activity/ActivityProvider.test.tsx`
- `loomspan-console/web/src/activity/reducer.test.ts`
- `loomspan-console/web/src/activity/LiveActivity.test.tsx`
- `loomspan-console/web/src/observability/Overview.test.tsx`

**Changes**:

- Consume the new coverage cursor object atomically and remove the old Boolean.
- Present the observed/reset/retained/evicted cursor facts directly. Static UI
  labels may identify the field meaning, but the provider/reducer must not
  synthesize a completeness, progress, health, or stuck state.
- Preserve existing untrusted summary/detail rendering and trace-link gating.

### Success Criteria

#### Automated Verification

- [x] Live service tests prove unrelated global eviction and selected-session
  admission/eviction have distinct exact cursors: `go test ./internal/live`.
- [x] Live service tests prove reset/baseline cases omit facts that were not
  observed and emit no completeness enum or Boolean.
- [x] Browser unit tests pass: `npm test` from
  `loomspan-console/web/`.
- [x] Browser type checking passes: `npm run typecheck` from
  `loomspan-console/web/`.

#### Manual Verification

- [ ] The Overview displays global and selected-session cursor facts separately
  without a synthesized coverage state.
- [ ] An already-running execution first seen during baseline has no invented
  start cursor or “unknown/complete/incomplete” message.

---

## Phase 3: Publish One Discoverable MCP Active-Inspection Contract

### Overview

Expose the existing orientation and new coverage facts through compact schemas,
descriptions, structured results, and safe deterministic text while preserving
one bounded primitive set.

### Changes Required

#### 1. MCP DTOs, schemas, descriptions, and text

**Files**:

- `loomspan-console/internal/mcpadapter/contracts.go`
- `loomspan-console/internal/mcpadapter/output_schemas.go`
- `loomspan-console/internal/mcpadapter/executions.go`
- `loomspan-console/internal/mcpadapter/activity.go`

**Changes**:

- Define reusable compact schemas for the complete execution orientation,
  active-path entry, usage, configured limits, activity identity,
  returned-cursor range, continuity/reset, and coverage shapes. Use the same
  execution schema for list items and detail.
- Name every complete ordinary-orientation property in discovery while keeping
  open only genuinely extensible/untrusted objects such as activity `details`.
- Replace `beginningUnavailable` with the exact cursor-fact coverage object;
  retain complete
  typed-output validation against Go result DTOs.
- Update activity input/tool descriptions to say that `hasMore` means retained
  matching backlog and that every returned continuation is also a future
  checkpoint after `hasMore: false`; document empty-result advancement.
- Expand list text to include sequence, elapsed/timestamps, path/depth/
  truncation, usage, and limits for each item. Expand activity text with typed
  identity and coverage cursor facts, but never render arbitrary `details` or
  generate a diagnostic sentence/state from the facts.
- Keep list/detail/activity page limits, read-only annotations, target
  publication/authentication checks, and opaque continuation binding unchanged.

#### 2. MCP contract, parity, race, and security tests

**Files**:

- `loomspan-console/internal/mcpadapter/output_schemas_test.go`
- `loomspan-console/internal/mcpadapter/executions_test.go`
- `loomspan-console/internal/mcpadapter/activity_test.go`
- `loomspan-console/internal/mcpadapter/parity_test.go`
- `loomspan-console/internal/mcpadapter/continuation_test.go`
- `loomspan-console/internal/mcpadapter/security_test.go`
- `loomspan-console/internal/console/activity_integration_test.go`
- MCP protocol-revision and conformance tests under
  `loomspan-console/internal/mcpadapter/` and `loomspan-console/mcp-conformance/`

**Changes**:

- Assert every required orientation/coverage field is advertised and every
  complete result still validates.
- Add a two-observation test that reuses a continuation after
  `hasMore: false`, plus retained-backlog, empty advancement, wrong-session,
  target-change, reset, and stale/invalid recovery cases.
- Add completion-race tests for list then missing detail, retained terminal
  activity, successful trace-ID handoff, and `TRACE_UNAVAILABLE` without an
  inventory scan.
- Assert text/structured parity for stable facts and continued omission of
  arbitrary details, credentials, internal target scope, instance, and owner
  identifiers.

#### 3. Exact discovery budget and goldens

**Files**:

- `loomspan-console/internal/mcpadapter/server_test.go`
- `loomspan-console/internal/mcpadapter/testdata/tools-list-response.json`
- `loomspan-console/internal/mcpadapter/testdata/activity.json`
- other affected execution/activity goldens under
  `loomspan-console/internal/mcpadapter/testdata/`

**Changes**:

- Set `maxToolsListResponseBytes` to `25 << 10` and update the exact expected
  byte count after all descriptions/schemas settle.
- Regenerate the exact snapshot intentionally and assert the response is at or
  below 25,600 bytes. Record actual headroom; do not weaken complete output
  validation to meet discovery size.
- If the selected schema initially exceeds 25,600 bytes, compact duplicated
  prose/schema construction without removing the orientation/coverage names or
  raising the chosen ceiling.

### Success Criteria

#### Automated Verification

- [x] MCP adapter and black-box protocol tests pass:
  `go test ./internal/mcpadapter ./internal/console`.
- [x] Exact `tools/list` bytes are `<= 25600`, match the committed snapshot, and
  advertise the full selected active contract.
- [x] A continuation returned with `hasMore: false` retrieves later activity in
  a deterministic test.
- [x] Text fallback contains every ordinary orientation/coverage fact as
  deterministic key/value output, with no arbitrary activity details or
  synthesized diagnosis.
- [x] MCP conformance runs through the repository build tool:
  `go run ./internal/buildtool mcp-conformance` from `loomspan-console/`.

#### Manual Verification

- [ ] Inspect the discovery response as a schema-driven client and confirm no
  undocumented result-key probing is needed for a 64-execution orientation.
- [ ] Inspect text-only output and confirm it supports the same bounded review
  without disclosing payload/detail content.

---

## Phase 4: Teach, Package, and Evaluate the Bounded Review Workflow

### Overview

Update the canonical runtime-debugging skill and reproducible evaluation
corpus so tools-only and skill-assisted clients demonstrate the new semantics.

### Changes Required

#### 1. Canonical Agent Skill

**Files**:

- `loomspan-console/agent-skills/loomspan/SKILL.md`
- `loomspan-console/agent-skills/loomspan/references/debugging-playbooks.md`
- `loomspan-console/agent-skills/loomspan/references/runtime-model.md`
- `loomspan-console/agent-skills/loomspan/references/evidence-and-confidence.md`
- `loomspan-console/agent-skills/loomspan/references/mcp-tool-guide.md`
- `loomspan-console/agent-skills/loomspan/references/common-failure-patterns.md`
- `loomspan-console/internal/agentskills/validate.go`
- `loomspan-console/internal/agentskills/validate_test.go`

**Changes**:

- Remove custom skill-version metadata and exact-version validation. The skill
  remains unversioned until Loomspan establishes a release policy.
- Preserve exact package topology, content validation, and byte-identical
  release-archive packaging without treating those checks as versioning.
- Add `WF-ACTIVE-EXECUTION-REVIEW` as a concise route, not a copied MCP schema:
  get runtime, list at most one requested/full 64-item page, orient directly
  from the list, call activity once per execution, retain checkpoints, and make
  at most one later observation unless the developer requests monitoring.
- Explain `hasMore` versus checkpoint, the meaning and limitations of each
  coverage cursor, live usage accrual, zero-limit semantics, and provisional
  conclusions. Explicitly forbid converting missing cursors into a coverage
  state.
- Add the bounded completion race: use retained activity by `sessionId`, try
  finalized resolution once by the returned `traceId`, preserve
  `TRACE_UNAVAILABLE`, and never scan unrelated inventory.
- Preserve the exact six-file package, one link per reference, instruction/file
  size limits, untrusted-content boundary, and no embedded endpoints/secrets.

#### 2. Packaging and version declarations

**Files**:

- `loomspan-console/README.md`
- `loomspan-console/release/README.md`
- `loomspan-console/internal/buildtool/projectdeclarations_test.go`
- `loomspan-console/internal/buildtool/package_test.go`
- `loomspan-console/internal/buildtool/smoke_test.go`
- evaluation record/transcript/schema fields that treat a skill version as a
  required fact, including `loomspan-console/internal/agenteval/record.go` and
  tests

**Changes**:

- Remove skill-version declarations from product/release/client documentation
  and evaluations. State explicitly that the canonical skill is unversioned
  during unreleased development and does not negotiate a version with MCP.
- Retain exact byte-identity/package validation for the canonical source.
- Keep installed user copies non-authoritative; only the canonical source is
  packaged and evaluated.

#### 3. Reproducible PR 34 evaluation scenarios

**Files**:

- `loomspan-console/agent-evals/cases/pr34-tools-only-active-execution-review.json`
- `loomspan-console/agent-evals/cases/pr34-skill-assisted-active-execution-review.json`
- `loomspan-console/agent-evals/fixtures/pr34-active-execution-review.json`
- `loomspan-console/internal/agenteval/fixtures.go`
- `loomspan-console/internal/agenteval/fixtures_test.go`
- `loomspan-console/internal/agenteval/server.go`
- `loomspan-console/internal/agenteval/server_test.go`
- `loomspan-console/internal/agenteval/score.go`
- `loomspan-console/internal/agenteval/score_test.go`
- `loomspan-console/agent-evals/README.md`
- `loomspan-console/agent-evals/fixtures/README.md`
- `loomspan-console/agent-evals/results/README.md`

**Changes**:

- Approve `WF-ACTIVE-EXECUTION-REVIEW` and `PR34-*` requirement IDs.
- Serve deterministic time-ordered list/REST/SSE facts for: multiple active
  executions; progress between two observations; retained backlog then future
  checkpoint; global eviction followed by an observed selected-session start;
  selected-session eviction and an unobserved baseline start; valid in-flight usage; and
  completion between list/detail/activity with both available and unavailable
  trace outcomes.
- Add paired tools-only and skill-assisted cases over the same
  `pr34-active-execution-review.json` state sequence. The
  tools-only oracle proves the MCP contract is independently usable; the
  skill-assisted oracle additionally checks bounded call routing and recovery.
- Add the paired PR 34 cases to the supported headless release matrix: three
  Codex CLI runs per case and two Claude Code runs per case, expanding the
  protected matrix from 28 to 38 runs while retaining all completed
  unfavorable results.
- Record actual Codex CLI (and other currently supported headless client rows)
  only through the sanitized harness. Keep Codex Desktop explicitly `Not run`
  unless the harness can capture its events without secrets or unsupported
  claims.

### Success Criteria

#### Automated Verification

- [x] Canonical skill validation and package byte-identity tests pass:
  `go test ./internal/agentskills ./internal/buildtool`.
- [x] Evaluation loader/server/scorer tests pass and require all PR 34 cases:
  `go test ./internal/agenteval`.
- [x] Tools-only and skill-assisted fixtures exercise every ticket lifecycle and
  exact coverage cursor without relying on the historical manual session or a
  derived coverage state.
- [x] Skill body/package limits and exact six-file topology remain valid.

#### Manual Verification

- [ ] One supported headless client is run through both paired PR 34 cases and
  records real client/model build, bounded calls, final answer, and limitations.
- [ ] Skill-assisted output is materially clearer than tools-only output while
  remaining provisional, neutral, read-only, and safe for untrusted content.
- [ ] Any unavailable GUI/Desktop execution is recorded as `Not run`, not
  inferred from the historical ticket observation.

---

## Phase 5: Synchronize Product Documentation and Complete Verification

### Overview

Make documentation, roadmap status, compatibility evidence, and all repository
verification agree with the final active-inspection contract.

### Changes Required

#### 1. Product and compatibility documentation

**Files**:

- `loomspan-console/README.md`
- `loomspan-console/docs/mcp-client-compatibility.md`
- `ai/thoughts/phases/2026-08-15-loomspan-llm-trace-understanding-roadmap.md`

**Changes**:

- Document list/detail equivalence for structured orientation, text behavior,
  activity backlog/checkpoint semantics, empty advancement, coverage cursor facts,
  usage accrual/zero semantics, and bounded completion handoff.
- Replace every old `beginningUnavailable` statement with the new global and
  session-specific cursor vocabulary and an explicit no-derived-state rule.
- Record exact post-change discovery bytes, 25,600-byte ceiling, headroom, tool
  count, protocol revision, and actual client/model observations.
- Mark the PR 34 roadmap lifecycle/discovery/evaluation decisions complete and
  retain future monitoring/history work as out of scope.

#### 2. Repository-wide verification

**Files**: No additional production files expected.

**Changes**:

- Run focused checks first, then standard Console verification, the relevant
  Java suite, architecture classification, and MCP/skill/evaluation packaging.
- Review generated/snapshot diffs for intentional semantic changes only.

### Success Criteria

#### Automated Verification

- [x] Standard Console tests pass: `go test ./...` from `loomspan-console/`.
- [x] Standard Console verification passes:
  `go run ./internal/buildtool verify` from `loomspan-console/`.
- [x] Java focused and public-surface tests pass:
  `.\mvnw.cmd -pl loomspan-spring-boot-starter "-Dtest=ModelAttemptCallAdvisorIntegrationTest,LiveActivityProjectorTest,ObservabilityDtoMapperTest,ObservabilityRestIntegrationTest,ConsoleRestFixtureCorpusTest,LoomspanPublicSurfaceArchitectureTest" test`.
- [x] Exact discovery snapshot, protocol revision/conformance, canonical skill,
  release archive, evaluation matrix, Java fixtures, Go fixtures, and browser
  contracts all agree.
- [x] No `ai/skill-authoring/` file changed and its routing remains accurate.

#### Manual Verification

- [ ] Review a representative 64-item page and verify calls remain bounded to
  one list page plus one activity call per execution and, when requested, one
  checkpoint-based second observation.
- [ ] Verify a completion race yields either retained terminal evidence and a
  trace result or an explicit `TRACE_UNAVAILABLE`, never invented finality.
- [ ] Verify documentation labels evidence, calculation, context, inference,
  gaps, and live provisionality consistently.

## Testing Strategy

Create the dedicated testing plan next with `ai/commands/3_testing_plan.md`.
That artifact should choose the exact failing-first tests and command ordering;
this section records the high-level coverage only.

### Unit Tests

- Java DTO construction/mapping for provider attempt usage and disabled limits.
- Go strict wire-presence validation for every active usage/limit member.
- Live per-session cursor facts under admission, unrelated eviction,
  selected-session eviction, reset, baseline, empty query, and completion.
- MCP compact/full-schema parity, text formatting, opaque token binding, and
  result-security boundaries.
- Skill validator/version/topology and evaluation case-oracle validation.

### Integration Tests

- Provider advisor through canonical record, live projection, active registry,
  REST DTO/JSON, Go client, and MCP result.
- Two activity observations using a checkpoint returned with `hasMore: false`.
- Multi-execution list plus per-session activity, including a session that
  completes between calls and trace resolution that is available/unavailable.
- Browser rendering of the shared Go cursor facts without derived state.
- Exact MCP protocol-revision initialization/list/call, discovery snapshot,
  conformance, release packaging, and agent-eval server flows.

### Manual Testing Steps

1. Run one sanitized supported headless-client tools-only evaluation over the
   PR 34 multi-execution fixture.
2. Repeat with the canonical unversioned skill and compare bounded calls, correct
   checkpoint retention, coverage fact handling, and completion handoff.
3. Inspect a text-fallback-only client path and verify complete orientation
   without arbitrary detail disclosure.
4. Record exact client/model versions and any unsupported GUI row honestly.

## Performance Considerations

- Keep execution pages and activity pages capped at 64 and the shared ring at
  2,048 envelopes/8 MiB with exact complete-item admission.
- Bound session cursor bookkeeping to sessions/items represented by the ring;
  do not create a process-lifetime execution history.
- The richer text and schema increase response/discovery bytes intentionally.
  Protect `tools/list` with the exact snapshot and 25,600-byte ceiling, and
  preserve all existing activity/result budgets.
- A full skill-assisted review is O(number of executions in one selected page)
  and performs at most one initial activity call per execution. It does not
  automatically traverse all execution pages or poll indefinitely.

## Migration Notes

This is an atomic pre-v1 contract change. Remove the old
`beginningUnavailable` field and premature skill-version declarations in the same change;
do not support both forms. Java, Go, TypeScript, fixtures, schemas, docs, skill,
and evaluations must be deployed from the same release. Existing opaque
continuations are current-process only and need no migration. No persisted
trace or application data migration is required.

Rollback, if required before release, is a full repository/release rollback of
the Java producer and Console consumers together. Mixed Java/Go releases remain
rejected by exact `consoleCompatibilityVersion` equality.

## References

- Original ticket: `ai/thoughts/tickets/loomspan-console-pr-34-active-execution-mcp-inspection.md`
- Research: `ai/thoughts/research/2026-08-21-PR-34-active-execution-mcp-inspection.md`
- Framework design lens: `ai/thoughts/framework-feature-design-lens.md`
- Active roadmap: `ai/thoughts/phases/2026-08-15-loomspan-llm-trace-understanding-roadmap.md`
- Console guidance: `loomspan-console/AGENTS.md`
- Skill-authoring routing: `ai/skill-authoring/README.md`
- Source-verification protocol: `ai/skill-authoring/source-verification.md`
