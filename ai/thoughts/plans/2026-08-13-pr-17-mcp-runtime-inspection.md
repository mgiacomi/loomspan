# PR 17 Runtime, Skill, and Live-Inspection MCP Surface Implementation Plan

## Overview

Add five read-only MCP tools and one supplementary skill resource to the local
Loomspan Console. The implementation adapts the existing target,
observability, and live services; it does not create another runtime model,
subscription, catalog, history, or authorization layer.

The design deliberately favors a small local-development implementation:
typed SDK handlers, one result/error envelope, one simple continuation codec,
one capability table, existing service bounds, and dated client smoke tests.

## Current State Analysis

- `mcpadapter.NewServer` registers only `LOOMSPAN_get_runtime` and accepts only
  status/authentication dependencies
  (`loomspan-console/internal/mcpadapter/server.go:15-21`).
- The current typed runtime handler establishes the SDK pattern for inferred
  strict schemas, structured content, deterministic text, and authentication-
  generation revalidation
  (`loomspan-console/internal/mcpadapter/runtime.go:13-67`).
- `observability.Service` already owns validated, cancellable skill and active-
  execution list/detail operations. Its `Page`, `SkillDetail`,
  `ActiveExecution`, and `ActivePage` DTOs contain nearly all workflow facts
  (`loomspan-console/internal/observability/dto.go:19-111` and
  `service.go:51-148`).
- `live.Service` already owns the one 2,048-item/8-MiB recent window, continuity
  interval, reset fact, per-session filtering, and application cursor ordering.
  `Recent` currently returns a tuple, supplies no query-time observation, and
  does not atomically reject retained data when live monitoring is unavailable
  (`loomspan-console/internal/live/service.go:658-749`).
- Browser handlers are thin adapters over these services and provide the
  existing parity target
  (`loomspan-console/internal/browserapi/observability.go:38-143` and
  `activity.go:163-204`).
- The composition root creates the shared observability and live services but
  does not pass them or `target.Context` to MCP
  (`loomspan-console/internal/console/service.go:137-166`, `:247-248`).
- The pinned `github.com/modelcontextprotocol/go-sdk` `v1.7.0` typed-tool path
  validates inferred input/output schemas. A returned Go error becomes text
  with `isError: true` and does not retain typed structured output. Domain
  errors therefore need a normally returned typed envelope with explicit
  `CallToolResult.IsError`.
- No MCP continuation codec, resource template, PR 17 capability descriptor,
  structured domain-error mapper, or client evidence exists.

## Desired End State

An authenticated local MCP client can:

1. list and retrieve registered skills, including unchanged YAML and a
   scope-bound skill resource;
2. list and retrieve current active execution summaries;
3. query one bounded, single-continuity recent-activity snapshot for an
   execution and explicitly continue forward;
4. distinguish observation time, upstream continuity time, retained-window
   gaps, resets, current live unavailability, target changes, domain failures,
   and protocol failures; and
5. discover the three new capabilities only when their complete tool families
   are installed.

The approved workflows can be completed using the general tools without
workflow-specific DTOs or diagnoses. Browser and MCP expose the same underlying
facts and shared domain codes. `go run ./internal/buildtool verify`, focused MCP
and adapter tests, MCP conformance, the Java public-surface architecture test,
and the dated client matrix supply review evidence.

### Key discoveries

- The workflows explicitly establish one connected inspection experience and
  leave final transport DTO spelling and framing values to implementation
  (`ai/thoughts/phases/loomspan_console_workflows.md:51-99`).
- The slow-execution workflow gives the PR 17 acceptance field set: identity,
  phase, bounded path, timing, usage/limits, recent ordered activity,
  observation freshness, and continuity (`:249-350`).
- The skill-path workflow requires registered name, descriptive `sourcePath`,
  and unchanged YAML, without filesystem interpretation (`:537-576`).
- MCP authentication and query continuation are independent. The continuation
  grants no authority and needs no signing, encryption, persistence, or
  server-side registry in this loopback single-user design.
- The SDK behavior makes a small `{result|error}` envelope less complex than a
  manual untyped tool implementation while preserving structured domain errors.

## What We're NOT Doing

- No trace, frame, record, payload, artifact, or imported-evidence MCP surface.
- No Agent Skill, prompt, workflow-specific diagnosis tool, or model evaluation.
- No server-initiated live feed or long-running MCP subscription.
- No second application client, SSE subscription, catalog, execution registry,
  recent window, paging store, or conversation session.
- No adaptive byte-budget pagination, deferred page storage, cumulative quota,
  signed/encrypted token, OAuth, multi-user authorization, remote transport, or
  enterprise policy framework.
- No execution control, target selection/mutation, arbitrary filesystem,
  network, or shell access.
- No parsing, normalization, effective-definition construction, or filesystem
  use of skill YAML/`sourcePath`.
- No public Java API/SPI, configuration, manifest, or Java-to-Go protocol change.

## Skill-Authoring Documentation Impact

**Impact**: No impact

- **Rationale**: PR 17 adds a Go Console diagnostic adapter. It does not change
  YAML syntax, registered skill identity, validation, defaults, planning,
  execution, limits, trace semantics, or any skill-author-visible capability.
  The unchanged YAML returned by MCP is the existing application-provided
  definition. Portable Agent Skill guidance belongs to PR 19.
- **Documents to update**: None under `ai/skill-authoring/`.
- **Supporting evidence**: Existing registered-skill fixtures and
  `observability.Service` tests establish unchanged name/path/YAML behavior;
  new MCP golden/parity/resource tests will prove that the adapter preserves it.
- **Coverage table update**: Not required because no topic boundary or authoring
  confidence changes.
- **LLM-first usability**: Not applicable. Console MCP user documentation is
  updated in `loomspan-console/`, outside the authoring knowledge base.

## Contract and Compatibility Impact

| Surface | Classification and supporting evidence | Planned compatibility treatment |
| --- | --- | --- |
| Application API | No supported `com.lokiscale.loomspan.api` type changes. The closed allowlist remains the authority. | Preserve; run `LoomspanPublicSurfaceArchitectureTest`. |
| Supported SPI | Loomspan has no supported Java SPI and PR 17 adds none. | No change and no replacement/override seam. |
| Configuration and manifest contracts | No `loomspan.*` configuration or YAML syntax/semantics change. MCP reads existing registered YAML unchanged. | Preserve atomically; no migration. |
| Persisted or serialized contracts | New MCP tool names, capability IDs, JSON schemas, result/error envelope, resource URI, MIME/meta shape, and continuation behavior become serialized client contracts. Existing Java REST/SSE remains unchanged. | Add one coherent v1 contract and golden protocol tests. No legacy form exists. |
| Ephemeral diagnostic formats | Active snapshots, activity envelopes, continuity/reset facts, target scope, observation time, and MCP continuations remain current-process evidence. | Preserve truthfulness, bounds, scope, and reset behavior; no cross-restart promise. |
| Internal or accidentally exposed implementation | All affected Go packages and Java observability internals remain internal. `NewServer`, DTOs, helper interfaces/functions, and service signatures are not supported application surfaces. | Update internal callers/tests atomically; do not add compatibility overloads. |

- **Evidence of supported contracts**: The Java allowlist architecture test,
  approved PR 17 ticket, Phase 3 capability/resource design, workflow catalog,
  MCP golden tests, and Console client compatibility document.
- **Intended breaks**: The browser recent endpoint will stop returning retained
  activity when live monitoring is unavailable and will instead preserve the
  settled `LIVE_MONITORING_UNAVAILABLE` meaning. This fixes current behavior to
  match the approved Phase 3/workflow contract. No protected external release
  contract exists yet.
- **In-repository consumers to update**: Browser recent handler/tests, live
  service/tests, MCP server/runtime/tests, console composition/integration tests,
  Console documentation, and compatibility matrix.
- **Public-surface delta**: None in Java; no Spring beans, constructors, or
  extension points are added or removed.
- **Shim decision**: **No shim.** All changes are internal or establish a new
  MCP surface. Atomic caller/test updates are appropriate.
- **Java-to-Go boundary coordination**: **Not required.** PR 17 consumes the
  existing exact-version REST/SSE boundary. If implementation proves a new
  cross-component field/bound is necessary, stop and amend the ticket and plan
  before changing Java, Go, fixtures, or the compatibility-marker decision.

## Implementation Approach

Keep protocol policy in `internal/mcpadapter` and runtime semantics in the
existing shared services:

```text
authenticated MCP request
  -> typed input validation
  -> capture target.Scope
  -> decode/validate MCP continuation when present
  -> observability.Service or live.Service
  -> map existing facts to MCP result DTO
  -> recheck target scope and MCP credential generation
  -> typed {result|error} envelope + deterministic text
```

Use concrete existing service dependencies in a single internal server options
structure and pure mapping/formatting helpers for focused tests. Do not create a
new generalized service or public interface solely for mocking.

### Settled wire contract

All five outputs use:

```go
type toolEnvelope[T any] struct {
    Result *T               `json:"result,omitempty"`
    Error  *domainErrorDTO  `json:"error,omitempty"`
}
```

Helpers and tests enforce exactly one populated member. Domain handlers return
the envelope with `IsError: true` and `nil` Go error. Schema/protocol failures
remain SDK errors. `domainErrorDTO.Details` is always an object and serializes
as `{}` when empty; `null`, scalar, and array forms are forbidden.

Common result organization:

- list results: `targetScopeId`, `instanceId`, `observedAt`, `items`,
  `hasMore`, optional `continuation`;
- detail results: `targetScopeId`, `instanceId`, `observedAt`, and `skill` or
  `execution`;
- activity result: common scope/observation fields plus `items`, optional
  `returnedCursorRange`, `hasMore`, optional resumable `continuation`,
  `continuity`, and `beginningUnavailable`.

Execution DTOs mirror every existing `observability.ActiveExecution` field;
items do not repeat the envelope's target scope. Skill summaries contain
`registeredName`, `sourcePath`, and canonical `resourceUri`. Activity items
preserve every existing `live.Activity` field including raw JSON `details`.

Text fallbacks use one shared line writer and exact committed goldens. Common
scope fields come first; operation fields follow structured DTO order; list
items retain service order. Dynamic strings and timestamps are JSON-quoted,
times are UTC RFC3339Nano, booleans/integers are unquoted, and an absent
continuation is `-`. Skill YAML is the only raw multiline content and follows a
terminal `yaml:` line. Activity details never appear in text. Execution detail
uses DTO order for scalar fields, indexed active-path entries, usage, and
configured limits. No map iteration may determine output order.

## Phase 1: Shared Query and MCP Contract Foundations

### Overview

Make the shared recent query truthful for both adapters and add the minimal MCP
contract helpers needed by every new handler.

### Changes Required

#### 1. Return one atomic recent-query result

**Files**:

- `loomspan-console/internal/live/dto.go`
- `loomspan-console/internal/live/service.go`
- `loomspan-console/internal/live/coordinator_test.go`
- `loomspan-console/internal/live/service_test.go`
- `loomspan-console/internal/browserapi/activity.go`
- `loomspan-console/internal/browserapi/activity_test.go`

**Changes**:

- Add top-level `ObservedAt time.Time` to `live.RecentResponse`.
- Add the already-declared `KindModelAttemptFailed` to `allKinds` and
  `KindLabels` with exact label `Model attempt failed` and focused
  validation/label coverage. Do not change the Java event name, REST/SSE
  contract, or fixture data.
- Replace the five-value `Recent(cursor, sessionID, limit)` return with
  `Recent(RecentRequest) (RecentResponse, *consolecore.Error)`.
- Under one service lock, reject `liveUnavailable` with
  `LIVE_MONITORING_UNAVAILABLE`, capture `service.now().UTC()`, snapshot the
  single interval, and preserve current filtering/gap/reset behavior.
- Always serialize `items` as `[]`, not `null`.
- Update the browser adapter to use the returned DTO/error directly while
  retaining its target-scope atomic publication check.
- Add regression coverage proving retained entries are not returned while live
  monitoring is unavailable and `continuity.observedAt` remains distinct from
  query `observedAt`.

#### 2. Add MCP result, error, annotation, and text helpers

**Files**:

- `loomspan-console/internal/mcpadapter/contracts.go` (new)
- `loomspan-console/internal/mcpadapter/contracts_test.go` (new)

**Changes**:

- Define `toolEnvelope[T]`, `domainErrorDTO`, scope/observation metadata,
  cursor-range DTO, and the three list/detail/activity output families.
- Map only the allowlisted exported fields of `consolecore.Error`; never expose
  `Unwrap()` or a Go error string.
- Add success/domain-failure constructors. Failure sets `IsError`, emits exactly
  `CODE: message`, and returns `nil` as the handler error.
- Add one shared read-only/non-destructive/idempotent/closed-world annotation
  value and deterministic RFC 3339-nanosecond UTC formatting.
- Implement the ticket's fixed line ordering and JSON scalar quoting; use the
  literal `-` for an absent continuation and allow raw multiline content only
  after the terminal `yaml:` marker.
- Test JSON shape, exactly-one envelope invariant, object-valued safe details,
  nil/empty collection behavior, exact text ordering/escaping/final newline,
  time formatting, and absence of internal causes.

#### 3. Add the simple continuation codec

**Files**:

- `loomspan-console/internal/mcpadapter/continuation.go` (new)
- `loomspan-console/internal/mcpadapter/continuation_test.go` (new)

**Changes**:

- Encode version-1 payloads as unpadded base64url JSON, maximum 8,192 encoded
  characters.
- Carry only version, operation kind, target scope, cursor, and activity
  session ID.
- Decode with unknown-field rejection, exactly one
  JSON value, strict kind/field rules, and canonical nonblank values.
- Bind tokens to the invoked operation and current captured scope. Bind activity
  tokens to the required input `sessionId`; do not bind `pageSize` so callers
  may choose a smaller later page.
- Map malformed/mismatched input to `INVALID_ARGUMENT`; map valid prior-scope
  tokens through the existing `TARGET_CHANGED` contract.
- Test round trips, the 4,096-character application cursor case, boundary
  length, bad alphabet/padding, unknown fields, trailing JSON,
  versions/kinds, operation/session/scope mismatch, and fuzz/no-panic behavior.

### Success Criteria

#### Automated Verification

- [x] `go test ./internal/live ./internal/browserapi ./internal/mcpadapter`
  passes from `loomspan-console/`.
- [x] Browser and live regression tests prove the live-unavailable correction.
- [x] Continuation fuzz/property tests never panic and reject every invalid
  binding deterministically.
- [x] `gofmt` leaves all changed Go files clean.

#### Manual Verification

- [x] Review one live response containing different top-level and continuity
  observation times and confirm their meanings are unambiguous.
- [x] Review the continuation source and confirm it contains no key material,
  signing state, persistence, registry, timer, or authorization decision.

---

## Phase 2: Tools, Skill Resource, and Composition

### Overview

Register the five typed tools and skill resource over the existing services,
then advertise the complete capability families.

### Changes Required

#### 1. Add skill inspection

**Files**:

- `loomspan-console/internal/mcpadapter/skills.go` (new)
- `loomspan-console/internal/mcpadapter/skills_test.go` (new)
- `loomspan-console/internal/mcpadapter/testdata/skills-*.json` (new goldens)

**Changes**:

- Add strict `LOOMSPAN_list_skills` input with required `pageSize` 1–64 and
  optional continuation.
- Add strict `LOOMSPAN_get_skill` input with required nonblank
  `registeredName`.
- Capture one target scope, decode continuation, call the existing service with
  the decoded application cursor, and map scope/instance/observation facts.
- Generate canonical MCP resource URIs; never return the application's REST
  `href` as an MCP URI.
- Preserve `registeredName`, descriptive `sourcePath`, and unchanged YAML.
- For list observation use the application page `ObservedAt`; for detail query
  observation use the injected clock after the validated service result and
  before final current-scope publication.
- Text output follows the ticket contract and does not treat YAML as an
  instruction.

#### 2. Add active execution inspection

**Files**:

- `loomspan-console/internal/mcpadapter/executions.go` (new)
- `loomspan-console/internal/mcpadapter/executions_test.go` (new)
- `loomspan-console/internal/mcpadapter/testdata/executions-*.json` (new goldens)

**Changes**:

- Add list/detail tools with required 1–64 list page and nonblank `sessionId`.
- Mirror every existing active summary field without calculating a hierarchy,
  health state, final outcome, complete duration, or complete attribution.
- Preserve page application observation time and capture detail query
  observation time using the same clock policy as skill detail.
- Preserve `NOT_FOUND`, live-unavailable, authentication, compatibility,
  cancellation, target-change, and sanitized console errors exactly.

#### 3. Add bounded recent-activity inspection

**Files**:

- `loomspan-console/internal/mcpadapter/activity.go` (new)
- `loomspan-console/internal/mcpadapter/activity_test.go` (new)
- `loomspan-console/internal/mcpadapter/testdata/activity-*.json` (new goldens)

**Changes**:

- Require `sessionId` and `pageSize` 1–64; accept optional continuation.
- Call the refactored shared recent query. Do not subscribe to the application
  or browser relay.
- Preserve complete activity JSON in structured content and output only
  cursor/timestamp/kind/summary in text.
- Report returned first/last cursor only when items exist.
- Issue a resumable MCP continuation from the last returned item cursor, or the
  continuity last cursor when no matching item was returned. On `hasMore`, the
  token resumes after the last returned item.
- Keep `hasMore` as “more retained now,” not a streaming indication.
- Preserve successful gap/reset facts and finalization-failed observation
  semantics without inference.

#### 4. Register the skill resource template

**Files**:

- `loomspan-console/internal/mcpadapter/resources.go` (new)
- `loomspan-console/internal/mcpadapter/resources_test.go` (new)

**Changes**:

- Register exactly
  `loomspan://targets/{targetScopeId}/skills/{skillName}` with name/title,
  concise description, and `application/yaml; charset=utf-8`.
- Build and parse with `net/url`. Inspect `EscapedPath()` and require scheme
  `loomspan`, authority exactly `targets`, and three segments after the leading
  slash: scope, literal `skills`, and name. Reject opaque form, user info, port,
  query, fragment, empty/extra segments, malformed escapes, invalid UTF-8,
  blank decoded values, and decoded `/` or `\`.
- Decode scope/name exactly once and require `url.PathEscape(decoded)` to equal
  the raw segment so noncanonical and double-encoded forms are rejected.
- Require the URI scope to equal the captured current scope, then call the same
  skill detail service.
- Return unchanged YAML and `_meta.loomspan` containing target scope, instance,
  observed time, registered name, and descriptive source path.
- Map URI syntax, `INVALID_ARGUMENT`, and `NOT_FOUND` to JSON-RPC
  `InvalidParams` (`-32602`). Map every other shared domain error, including
  sanitized `CONSOLE_ERROR`, to private server code `-32000`, never `-32603`.
  Use safe `CODE: message` and data exactly
  `{"error": <domainErrorDTO>}` with object-valued details; expose no cause.
- Test Unicode/escaped names, slash rejection/escaping, malformed and
  noncanonical URIs, stale scope, not found, YAML fidelity, MIME, and metadata.

#### 5. Wire services and capabilities

**Files**:

- `loomspan-console/internal/mcpadapter/server.go`
- `loomspan-console/internal/mcpadapter/runtime.go`
- `loomspan-console/internal/mcpadapter/server_test.go`
- `loomspan-console/internal/mcpadapter/runtime_test.go`
- `loomspan-console/internal/console/service.go`
- affected `loomspan-console/internal/console/*_test.go`

**Changes**:

- Replace the growing `NewServer` parameter list with one internal options
  structure containing credentials, tracker, status provider, target context,
  observability service, live service, and clock.
- Update the sole composition-root caller atomically; add no compatibility
  overload.
- Register all PR 17 tools/resources on the same stateless SDK server behind the
  existing security/admission handler.
- Define one static capability descriptor table and derive the deterministic
  runtime capability list from it.
- Add no dynamic registry, dependency graph, plugin mechanism, or target-state
  capability evaluation.
- Retain `loomspan.runtime-status.v1`; add exactly the three PR 17 capabilities;
  advertise no PR 18 capabilities.
- Before successful publication, enforce both current target scope and admitted
  MCP authentication generation. Domain failures are still returned to the
  authenticated caller unless credential generation invalidation suppresses
  the request.

### Success Criteria

#### Automated Verification

- [x] `go test ./internal/mcpadapter ./internal/console` passes.
- [x] Tool listing contains exactly the runtime tool plus the five PR 17 tools,
  with strict schemas and the settled annotations.
- [x] Resource template listing/read tests pass through the real SDK client.
- [x] Every golden result contains structured content and exactly one text
  block; domain failures contain structured error data and `isError: true`.
- [x] Unknown input properties, missing/invalid page sizes, blank identifiers,
  raw cursors, and malformed continuations are rejected.
- [x] Existing runtime golden/status behavior remains unchanged except for the
  additive deterministic capability list.

#### Manual Verification

- [x] Read tool descriptions as an LLM consumer and confirm the general
  discovery → detail → activity workflow is clear without the future Agent
  Skill.
- [x] Inspect returned YAML/activity containing adversarial instruction text and
  confirm it is returned only as data and triggers no server-side operation.

---

## Phase 3: Parity, Conformance, and Lifecycle Hardening

### Overview

Prove the new surface preserves shared semantics under normal, degraded,
malformed, concurrent, and lifecycle-changing conditions.

### Changes Required

#### 1. Add adapter parity fixtures

**Files**:

- `loomspan-console/internal/mcpadapter/parity_test.go` (new)
- `loomspan-console/internal/browserapi/contracts_test.go`
- `loomspan-console/internal/console/target_integration_test.go`

**Changes**:

- Feed the same shared skill, active execution, recent activity, and
  `consolecore.Error` fixtures through browser and MCP mappings.
- Assert equal Loomspan identifiers, timestamps and their documented meanings,
  usage/limits, active path/truncation, availability, gaps/resets, and domain
  codes. Permit only wrapper/text/resource-link presentation differences.
- Reference the most specific `WF-*` IDs in test names/comments or fixture
  tables; do not create another scenario namespace.

#### 2. Add capability-family conformance

**Files**:

- `loomspan-console/internal/mcpadapter/capabilities.go` (new or colocated with
  `runtime.go`)
- `loomspan-console/internal/mcpadapter/capabilities_test.go` (new)

**Changes**:

- Keep one table mapping each capability to required tool names.
- Assert installed tool discovery, schemas, annotations, and the focused
  semantic tests required by the family.
- Use a test-only conformance validator with negative table cases omitting each
  required operation and prove each mismatch is a server-definition defect.
  Do not add runtime suppression or dependency evaluation.
- Prove target/authentication/live/evidence state does not dynamically remove
  an installed capability.

#### 3. Exercise cancellation, races, and multiple clients

**Files**:

- `loomspan-console/internal/mcpadapter/server_test.go`
- `loomspan-console/internal/mcpadapter/lifecycle_test.go`
- `loomspan-console/internal/console/target_integration_test.go`

**Changes**:

- Cancel in-flight skill/execution calls through client cancellation, target
  rotation, credential regeneration/disablement, and shutdown; prove late
  results are suppressed.
- Exercise a target change between scope capture, service completion, and result
  publication.
- Connect multiple authenticated SDK clients and prove they share the same live
  service/window without adding another upstream subscription or cross-client
  cancellation.
- Distinguish schema/protocol failure, application authentication,
  incompatibility, live unavailability, stale scope, replay gap, `NOT_FOUND`,
  and sanitized unexpected service failure.
- Confirm text/result sizes for a 64-item maximum activity fixture and maximum
  representative skill/execution pages without silent truncation.

#### 4. Run protocol and repository contract checks

**Changes**:

- Keep the official MCP conformance harness protocol-generic; product-specific
  tools/resources remain covered by SDK/integration tests.
- Preserve one Loomspan product contract across protocol revisions. Exercise
  the complete PR 17 contract once using the current/default `2026-07-28` SDK
  path. Update the existing raw `2025-11-25` compatibility test into one compact
  smoke that initializes, discovers the PR 17 tools/resource template, calls a
  representative tool, and reads a skill resource.
- Keep SDK-provided `2025-11-25` negotiation and the existing official
  conformance invocations for both revisions, but add no version-specific DTO,
  handler, resource, feature branch, or legacy HTTP+SSE transport. Removing the
  compatible revision requires a separate MCP-foundation decision.
- Run the Java public-surface architecture test even though no Java production
  type is planned to change.
- Do not regenerate Java REST/SSE fixtures unless an unexpected cross-component
  change is explicitly approved and the ticket/plan is amended first.

### Success Criteria

#### Automated Verification

- [x] `go test ./...` passes from `loomspan-console/`.
- [x] `go test -race ./internal/mcpadapter ./internal/live ./internal/console`
  passes on a supported race-detector host.
- [x] `go run ./internal/buildtool mcp-conformance` passes.
- [x] The complete current-protocol product suite and compact `2025-11-25`
  compatibility smoke both pass without version-specific production code.
- [x] `go run ./internal/buildtool verify` passes.
- [x] `..\mvnw.cmd -pl loomspan-spring-boot-starter -Dtest=LoomspanPublicSurfaceArchitectureTest test`
  passes from `loomspan-console/` on Windows (or the equivalent root wrapper on
  POSIX).
- [x] `git diff --check` passes.

#### Manual Verification

- [x] Review the workflow-to-test mapping and confirm every cited PR 17
  requirement has executable evidence.
- [x] Review race/lifecycle evidence and confirm no MCP-owned subscription,
  retained catalog/history, or cross-client session state was introduced.

---

## Phase 4: Interoperability and Documentation

### Overview

Record practical local-client evidence and make the implemented surface usable
without relying on implementation context.

### Changes Required

#### 1. Update the client compatibility matrix

**File**: `loomspan-console/docs/mcp-client-compatibility.md`

**Changes**:

- Update the supported Loomspan surface from PR 16-only to PR 17.
- For each available Phase 3 local client family, record product/build version,
  validation date, platform, and result for:
  - authenticated Streamable HTTP connection;
  - tool/resource-template discovery;
  - structured-content and concise-text presentation;
  - domain `isError` presentation;
  - skill resource read and Unicode URI handling;
  - continuation round trip; and
  - a representative 64-item maximum-page result.
- Mark unavailable/unautomatable clients honestly; do not infer compatibility
  from another client or turn the matrix into a permanent vendor guarantee.
- If a client fails the 64-item result, lower the single MCP maximum and rerun
  all affected automated/manual evidence before acceptance.

#### 2. Update Console MCP documentation

**Files**:

- `loomspan-console/README.md`
- any focused runtime-only/release README source used by the build, if it lists
  the MCP surface

**Changes**:

- Document the five tools, three capabilities, skill resource URI, required
  page size, opaque continuation use, result/error envelope, observation versus
  continuity time, gap/reset meaning, and live-unavailable behavior.
- State the local/single-user scope, tool-first contract, untrusted data
  boundary, descriptive-only source path, absence of durable history, and PR 18
  trace boundary.
- Keep setup/authentication guidance aligned with the existing PR 16 key flow;
  do not introduce new credentials or configuration.

#### 3. Record review handoff

**Files**: ticket, plan, and test output/review notes as used by the repository
workflow.

**Changes**:

- Ensure implementation and review LLMs can recover exact tool names, schemas,
  envelope, continuation, resource, errors, capabilities, workflow links,
  lifecycle behavior, commands, and out-of-scope boundaries from the ticket and
  this plan alone.
- Keep PR 18 trace continuation/resource work and PR 19 Agent Skill/evaluation
  work explicitly separate.

### Success Criteria

#### Automated Verification

- [x] Documentation references exactly the implemented tool/capability names
  and contains no obsolete PR 16-only claim.
- [x] Documentation/link checks included in canonical verification pass.
- [x] `go run ./internal/buildtool verify` and `git diff --check` pass after the
  final documentation edits.

#### Manual Verification

- [x] The dated client matrix contains actual evidence or explicit “not run” for
  every named local client family.
- [ ] At least one representative local client completes skill list/detail,
  active list/detail, recent activity, domain-error display, resource read, and
  continuation without relying on repository source knowledge.
- [ ] A separate reviewer can check the implementation against the ticket and
  plan without consulting this conversation.

---

## Testing Strategy

Create the dedicated PR 17 testing artifact with
`ai/commands/3_testing_plan.md` before implementation. It should turn the
requirements below into exact test cases, fixtures, failing-test order, and exit
commands.

### Unit Tests

- Envelope and domain-error serialization/sanitization.
- Tool input/output schema goldens and text agreement.
- Continuation round trip, binding, limits, malformed input, and fuzz cases.
- Resource URI canonical encoding/parsing, YAML fidelity, metadata, and errors.
- Recent-query observation, live-unavailable, ordering, filters, gaps, resets,
  empty values, and forward resumption.
- Capability table completeness and negative conformance.

### Integration Tests

- Real SDK initialization, discovery, calls, structured/text results, resources,
  and domain failures through the production security handler.
- Same Java-produced/shared-service fixture through browser and MCP adapters.
- Target/application authentication and compatibility states.
- Target rotation, cancellation, credential generation, shutdown, and multiple
  clients.
- Maximum page size and no silent truncation.

### Manual Testing Steps

1. Start a Console with a selected compatible application and valid MCP key.
2. Connect a representative local MCP client and call `LOOMSPAN_get_runtime`.
3. List/get a skill and read its canonical resource; compare YAML bytes/text.
4. List/get an active execution and confirm bounded/provisional facts.
5. Query activity, follow a continuation, and observe explicit gap/reset facts.
6. Disable live monitoring and confirm both active and activity operations show
   `LIVE_MONITORING_UNAVAILABLE` without retained-state fallback.
7. Rotate target scope and confirm old detail/resource/continuation references
   return `TARGET_CHANGED`.
8. Record structured/text/error/resource/maximum-page behavior in the matrix.

## Performance Considerations

- Maximum MCP page size is 64; the live service already caps each activity at
  12 KiB and the shared window at 2,048 items/8 MiB.
- Continue using bounded upstream reads and the 10-second MCP request context.
- Recent activity remains one mutex-protected in-memory snapshot; capture the
  clock and availability under the existing lock without adding I/O.
- Continuation encoding is stateless and proportional only to a bounded cursor.
- Text fallbacks avoid duplicating raw activity `details`; skill YAML is
  necessarily returned once in structured content and once in the fallback for
  clients that cannot present structured content.
- Do not optimize or add caching until profiling/client evidence demonstrates a
  problem.

## Migration Notes

There is no released PR 17 MCP contract or persisted state to migrate. Update
the sole internal `NewServer` caller and all tests atomically. Existing MCP
credentials, listener behavior, runtime tool name, runtime-status capability,
and stateless sessions remain valid. Continuations are current-process and may
be rejected after restart without migration or compatibility support.
The SDK continues to negotiate MCP `2026-07-28` and compatible `2025-11-25`
against the same Loomspan contract; this is not a second API or transport.

## Review Checklist

- Tool/resource handlers call only the captured target scope and existing shared
  services.
- No raw application cursor is emitted as an MCP continuation.
- No continuation key, registry, cache, persistence, or authorization logic was
  added.
- Every domain error is safe, structured, and marked `isError`; every protocol
  error remains outside the domain envelope.
- Resource errors use the settled `-32602`/`-32000` mapping and never expose a
  cause or convert `CONSOLE_ERROR` to `-32603`.
- `observedAt` and `continuity.observedAt` retain distinct meanings.
- Live-unavailable never returns retained active/activity state.
- Skill YAML and activity fields remain untrusted output and never drive server
  operations.
- Capability advertisement matches the exact registered tool family.
- No protocol-revision conditional exists in Loomspan production code; the
  prior-revision check remains a compact compatibility smoke.
- No trace/Agent Skill/public Java/configuration/manifest scope leaked into PR
  17.
- Tests reference canonical `WF-*` requirements instead of inventing scenarios.

## References

- Ticket: `ai/thoughts/tickets/loomspan-console-pr-17-mcp-runtime-inspection.md`
- Research: `ai/thoughts/research/2026-08-13-loomspan-console-pr-17-mcp-runtime-inspection.md`
- Phase 3: `ai/thoughts/phases/loomspan_console_phase_3_llm_runtime_inspector.md`
- Workflows: `ai/thoughts/phases/loomspan_console_workflows.md`
- Framework design lens: `ai/thoughts/framework-feature-design-lens.md`
- Follow-on trace ticket: `ai/thoughts/tickets/loomspan-console-pr-18-mcp-trace-inspection.md`
- Follow-on Agent Skill ticket: `ai/thoughts/tickets/loomspan-console-pr-19-debugging-skill.md`
- Current MCP compatibility evidence: `loomspan-console/docs/mcp-client-compatibility.md`
