# PR 34 — Active-Execution MCP Inspection Ergonomics and Evidence Semantics

## Status

Proposed pre-v1 ticket brief. Run this ticket through the repository's complete
five-step development process in `ai/commands/`:

1. `1_research_codebase.md`
2. `2_create_plan.md`
3. `3_testing_plan.md`
4. `4_implement_plan.md`
5. `5_code_review.md`

Start in a fresh context with step 1 and this ticket as the research input. Do
not treat the observations or candidate directions below as a pre-approved
implementation design.

## Outcome

Make a representative multi-execution review through the read-only Loomspan
Console MCP surface straightforward, bounded, and semantically precise for both
tools-only clients and clients using the portable `loomspan` Agent Skill.

A capable model should be able to discover all currently active executions,
orient to each execution's identity, elapsed time, latest canonical sequence,
active path, recent activity, usage, and mechanically observed evidence-coverage
facts, and then continue observing new activity without schema archaeology or
misleading completeness claims. MCP must not turn those facts into derived
completeness, progress, health, stuck, or diagnostic-message states. The
workflow must remain provisional, read-only, neutral, bounded, and safe for
untrusted diagnostic content.

## Why this ticket exists

This friction was observed while manually testing the Console Overview's
per-active-execution Live Activity feeds and asking Codex Desktop to use the
installed `loomspan` skill to review the same live executions through MCP. The
MCP tools returned enough underlying evidence to perform the review, but using
that evidence exposed discoverability and semantic friction across the compact
tool schemas, text fallbacks, activity continuation, coverage facts, live usage,
and skill workflow.

The manual run used an installed user skill containing obsolete custom version
metadata. The authoritative repository skill at
`loomspan-console/agent-skills/loomspan/` is the source of truth. Loomspan has
not released, so fresh work must keep the canonical skill unversioned and use
the installed observation only as historical workflow evidence.

This work aligns with the active roadmap in
`ai/thoughts/phases/2026-08-15-loomspan-llm-trace-understanding-roadmap.md`,
especially progressive skill routing, the pre-v1 client/evaluation baseline,
and the v1 contract gate. The roadmap explicitly discourages speculative
specialized tools, so a new fleet-specific MCP tool is not presumed by this
ticket.

## Historical live observation

The following facts are a sanitized record of the motivating manual session.
They are not durable fixtures, cannot be queried again by a fresh context, and
must not substitute for reproducible tests.

Runtime discovery at `2026-08-21T20:04:20.5037582Z` reported all six current
capabilities, including the five required skill capabilities and optional raw
artifact inspection. The selected target was reachable, authenticated,
compatible, runtime-established, and live monitoring was available.

An initial `LOOMSPAN_list_executions(pageSize: 64)` observation at
`2026-08-21T20:04:28.3028444Z` returned one active `planTrip` execution. A
second list observation at `2026-08-21T20:04:54.2827866Z` returned two active
executions: `planTrip` and `handleIncident`.

For the historical `planTrip` execution:

- Session ID: `4068bcdf-a7c6-41a5-b41e-2db985c1ad3a`
- Trace ID: `e23428df-4cbd-4fb8-aca7-0fd8081ed5c0`
- It remained `ACTIVE` in phase `MODEL`.
- Its active snapshot advanced from canonical sequence 73 at
  `2026-08-21T20:04:32.4796333Z` to sequence 111 at
  `2026-08-21T20:04:54.2863761Z`, so the evidence supported recent progress
  and did not support a stuck conclusion.
- The later active path ended at `planTransport#step-3-model` with six
  untruncated frames.
- Recent activity contained a provider attempt failure followed by a retry and
  a later model response. This was recovered activity, not terminal evidence.
- The first activity result returned `hasMore: false` and a nonempty
  continuation. Reusing that continuation later returned 16 newly observed
  activity items, demonstrating that the activity continuation also serves as
  a forward observation checkpoint after the current retained backlog is
  exhausted.
- The later snapshot reported seven model calls, seven exact model responses,
  and zero provider attempts even though recent activity included a provider
  attempt failure and retry. This is an investigation lead, not proof that the
  counter is defective; snapshot timing, projection timing, disabled limits,
  and counter semantics must be traced before deciding.

For the historical `handleIncident` execution:

- Session ID: `c93a9898-7c8f-4565-a64e-f4d8a49d230a`
- Trace ID: `24a96b9b-3ac6-4dbb-845f-4ff480e70863`
- It was `ACTIVE` in phase `MODEL` at canonical sequence 6.
- Its three-frame active path ended at `handleIncident#planning-model`.
- Its returned session-filtered activity contained `TRACE_STARTED` followed by
  `MODEL_REQUEST_SENT`, yet the result reported `beginningUnavailable: true`.
  The shared activity ring had evicted older global activity. This made the
  flag accurate for the global retained interval but potentially misleading
  if interpreted as saying the beginning of this newly started session was
  unavailable.
- Its live usage remained zero while the model request was in flight. This may
  be valid accrual behavior, but the current skill does not explain when active
  usage counters become observable.

No model/tool payloads, YAML, raw artifacts, credentials, or diagnostic detail
content were opened during the observation.

## Current implementation map

Fresh research must verify these references against the current checkout rather
than relying only on this ticket.

### Compact discovery schemas and complete results

- `loomspan-console/internal/mcpadapter/contracts.go` defines the complete Go
  result DTOs. Active execution DTOs contain canonical sequence, timestamps,
  elapsed time, summary, path, usage, and configured limits. Activity DTOs
  contain canonical sequence, status, frame identity, route, details,
  continuity, returned cursor range, and coverage.
- `loomspan-console/internal/mcpadapter/output_schemas.go` deliberately
  advertises compact, open output schemas. At the time of this ticket,
  `executionListOutputSchema`, `executionDetailOutputSchema`, and
  `activityOutputSchema` declare only selected decision/navigation fields even
  though successful structured content contains additional fields.
- `newCompleteOutputValidator` separately validates every complete typed output
  against its derived full Go schema before publication.
- `maxToolsListResponseBytes` is `23 << 10`. The compact schemas are not an
  accidental omission: `loomspan-console/internal/mcpadapter/server_test.go`
  protects an exact serialized `tools/list` snapshot and a 23,552-byte
  discovery ceiling. The recorded response is already close to that ceiling.
  Do not solve discoverability by blindly publishing every complete field.
- `loomspan-console/internal/mcpadapter/testdata/tools-list-response.json` is
  the committed protocol discovery snapshot.

### Text fallbacks

- `loomspan-console/internal/mcpadapter/executions.go` renders a concise list
  fallback containing identity, entry skill, status, phase, and summary. The
  detail fallback contains the full active snapshot.
- `loomspan-console/internal/mcpadapter/activity.go` renders activity cursor,
  timestamp, kind, and summary but intentionally does not expose arbitrary
  activity details in the text fallback. Complete structured content retains
  the bounded details and typed identity fields.
- All tools must continue to support clients that consume structured content
  and clients that rely on deterministic text fallback. Untrusted application
  content must not be expanded into text merely for convenience.

### Activity pagination, resumption, and coverage

- `loomspan-console/internal/mcpadapter/activity.go` decodes an activity
  continuation into a shared activity cursor. It returns a new continuation
  from the last returned matching item, or from the continuity interval's last
  cursor when no matching item was returned.
- `loomspan-console/internal/live/service.go` implements the shared bounded
  activity ring and session filtering. `Recent` calculates
  `BeginningUnavailable` from the selected start offset, any global eviction,
  or an interval reset. Global eviction therefore affects every filtered
  session result, even when that result includes the session's own
  `TRACE_STARTED` activity.
- The product README explains that activity `hasMore` means more matching items
  are retained now, not that future live activity will never appear. The
  portable skill's MCP guide currently gives general continuation guidance but
  does not provide an equally explicit active polling/resume workflow.

### Usage boundary

- Active execution usage originates in the Java runtime projection under
  `loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/runtime/observation/`.
- The Java observability DTO/REST boundary maps the snapshot under
  `loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/observability/web/`.
- Go consumes that version-locked application boundary through
  `loomspan-console/internal/observability/` and republishes it through the
  browser and MCP adapters.
- Provider-attempt accounting is implemented separately by the Java session
  usage service. Research must trace when the live projector receives usage
  changes and whether a zero value means observed zero, not yet projected,
  disabled/unlimited, or another defined state.

### Skill and evaluation evidence

- The canonical portable skill is
  `loomspan-console/agent-skills/loomspan/SKILL.md` plus its `references/`.
- The existing `WF-SLOW-EXECUTION` playbook and
  `loomspan-console/agent-evals/cases/slow-execution.json` cover one selected
  active execution, provisional evidence, latest sequence, and bounded-window
  limitations. They do not establish a multi-execution fleet-review workflow
  or exercise continuation as a future resume checkpoint after
  `hasMore: false`.
- Release archives copy the canonical skill bytes into `skills/loomspan/`;
  installed user copies are not repository authorities.
- `loomspan-console/docs/mcp-client-compatibility.md` records the discovery
  budgets and client observation matrix. The motivating Codex Desktop run is
  useful research evidence but is not yet a sanitized, reproducibly recorded
  agent-evaluation result.

## Problem statements to resolve

### 1. Important active facts are present but not readily discoverable

The compact output schemas intentionally trade field discovery for a bounded
`tools/list` response. A client inspecting only the advertised schema sees
fewer fields than successful structured content contains. The deterministic
list/activity text fallbacks are also intentionally concise. In the motivating
run this caused extra detail calls and direct result-shape probing before the
model could understand which facts were available.

Research and design must identify the minimum active-execution and activity
facts that need explicit discovery for ordinary orientation while preserving a
measured, justified discovery budget. Consider schema descriptions, selected
additional compact properties, tool descriptions, deterministic text, and
skill routing before proposing a new tool.

### 2. Activity continuation has two operational roles

For a bounded current query, `hasMore` reports retained matching backlog. A
nonempty activity continuation can also be retained and supplied later to read
new matching activity after that backlog is exhausted. The contract is
internally coherent, but this forward-resume use is not obvious from the tool
description or portable skill workflow.

Decide whether clearer descriptions and skill guidance are sufficient or
whether the result vocabulary should distinguish backlog continuation from a
future resume checkpoint. Preserve opaque, current-process, scope/session-bound
tokens and safe stale-token recovery.

### 3. Global and session-specific beginning availability are conflated

`beginningUnavailable` currently describes limitations of the shared retained
interval. A caller of the session-filtered tool can reasonably interpret it as
coverage of the selected execution. For a session whose returned activity
includes `TRACE_STARTED`, global eviction may still set the flag. The contract
needs an unambiguous way to communicate the global-window facts separately from
an observed selected-session start, retained range, and known selected-session
eviction.

Do not infer complete session history merely from a current start activity
without accounting for reset, interval, cursor, and admission semantics.
Prefer exact observed/admitted/retained/evicted cursor facts over a derived
`COMPLETE`, `INCOMPLETE`, `UNKNOWN`, or similarly interpretive state. Missing
facts must remain missing rather than being converted into a message or status.

### 4. Live usage accrual and zero values are insufficiently explained

The observed provider-attempt values and in-flight model counters may reflect a
projection defect, expected timing, or valid zero/unlimited semantics. Establish
the executable behavior before changing it. If the values are correct, the MCP
and skill must make their timing and meaning clear enough to avoid treating
unknown or not-yet-accrued usage as a definitive zero. If they are incorrect,
fix the producer-to-projector-to-REST-to-Go chain atomically.

### 5. The portable skill is optimized for one selected execution

The skill has strong single-execution slow/stuck guardrails but lacks a concise
path for reviewing every active execution, using activity checkpoints for a
second observation, and handling an execution that completes between list,
detail, and activity calls. The desired workflow should minimize calls without
creating an unbounded scan or a specialized abstraction unsupported by measured
evidence.

## Contract and compatibility classification

The research and plan must re-evaluate these classifications using
`ai/thoughts/framework-feature-design-lens.md`:

- MCP tool names, inputs, advertised output schemas, descriptions, structured
  results, text fallbacks, continuation behavior, and errors are a deliberately
  supported pre-v1 Console diagnostic contract. They are expected to change in
  place before v1, but every in-repository consumer, fixture, snapshot,
  document, skill, and evaluation must move atomically.
- Live activity, active snapshots, continuations, and trace projections are
  current-process **Ephemeral diagnostic formats**, not persisted history or a
  cross-version interchange promise.
- The Java observability REST/SSE boundary consumed by Console is an internal,
  exact-release cross-component protocol. If its observable semantics change,
  Java, Go, fixtures, and compatibility-version reasoning must be coordinated
  in the same change.
- No application-facing Java API or supported Java SPI change is expected.
  Research must call out any unexpected public-surface delta before planning.
- The portable Agent Skill is an unreleased, unversioned development artifact
  with exact packaging/validation tests. It must not declare or negotiate a
  skill release version before Loomspan's first release.
- This policy is scoped to Agent Skill release identity. Console build versions,
  `consoleCompatibilityVersion`, MCP protocol versions, and evaluation-record
  schema versions retain their existing technical roles; none should be
  repurposed as a skill/server version handshake.

## In scope

- Research the five problem statements against current production code, tests,
  documentation, roadmap decisions, and canonical skill.
- Measure the current `tools/list` size and the byte effect of each proposed
  compact-schema change.
- Define and implement the smallest coherent active-execution discovery and
  continuation contract supported by the evidence.
- Make session-filtered activity coverage unambiguous using mechanically
  observed cursor/admission/eviction/reset facts, not an MCP-derived coverage
  classification or diagnostic message.
- Verify and, if necessary, correct active usage projection semantics across
  Java and Go.
- Add a compact multi-execution review route, second-observation guidance, and
  completion-race handling to the canonical skill.
- Update affected MCP descriptions, deterministic text fallbacks, product
  documentation, client compatibility evidence, fixtures, and canonical Agent
  Skill contents as required by the chosen contract. Remove premature
  skill-version metadata and evaluation fields rather than replacing them.
- Add focused Go/Java/browser tests and agent evaluations appropriate to the
  final design.
- Record an actual supported headless-client evaluation. Record a Codex Desktop
  observation only if the repository harness can capture it without secrets or
  unsupported claims; otherwise leave that client row explicitly not run and
  preserve the historical observation only as ticket context.

## Out of scope

- Mutating, pausing, canceling, retrying, or otherwise controlling executions
  through MCP.
- Durable execution history, cross-run comparison, analytics, or audit logs.
- Cross-trace semantic search or browser-selected process-global MCP state.
- Reading model/tool payloads by default during active orientation.
- Exposing credentials, target scope IDs, instance IDs, internal owners,
  application cursors, or authority-bearing handles to the model.
- Adding a specialized fleet-review MCP tool unless research demonstrates that
  the existing neutral primitives cannot meet the acceptance criteria within
  reasonable bounded calls.
- Preserving obsolete pre-v1 MCP behavior through aliases, dual schemas,
  fallback readers, or compatibility shims without an independently protected
  consumer and an explicit removal condition.
- Changing the supported top-level Java application API or creating a Java SPI.
- Treating a quiet live interval as proof of deadlock or future failure.

## Required research questions

The step-1 research artifact must answer at least the following:

1. Which complete active-execution/activity fields are absent from compact MCP
   discovery, and which are necessary for ordinary orientation rather than
   optional detail?
2. What is the current exact `tools/list` byte budget and contribution by tool
   schema/description? What measured alternatives fit the budget?
3. How do structured-output-capable and text-fallback-only clients currently
   experience the active workflow?
4. What are the precise semantics of execution-list pagination, activity
   backlog pagination, activity forward resumption, empty activity results,
   stale continuations, target changes, and completion races?
5. Which exact start, retained-range, eviction, reset, and interval cursor facts
   can current live state report authoritatively? What minimal bounded tracking
   is needed to expose those facts without deriving a session-completeness
   state?
6. Why did the motivating active snapshot show zero provider attempts alongside
   provider retry activity? Reproduce or falsify this with a deterministic Java
   integration test and trace the live projector update order.
7. When do each of the active usage counters accrue for an in-flight operation,
   and how are disabled or unlimited configured limits represented?
8. Can an agent review up to one full active page using the existing list
   structured content plus one bounded activity call per execution, or is a
   detail call still necessary for correctness?
9. What should a client do when a listed execution becomes inactive before its
   detail or activity call? How does it safely hand off to trace inspection by
   the already returned `traceId` without claiming finalized evidence exists?
10. Which canonical skill files, validation rules, evaluation cases, product
    docs, snapshots, fixtures, and client compatibility records must change?

## Acceptance signals

- A fresh capable agent can use tools alone to enumerate a bounded active page
  and identify each execution's session ID, trace ID, entry skill, status,
  phase, elapsed time, latest canonical sequence, active route/path status, and
  latest activity and coverage facts without probing undocumented result keys.
- The same task is materially easier with the canonical `loomspan` skill. The
  skill contains a concise multi-execution route and does not reproduce full
  MCP schemas.
- Activity backlog pagination and future resumption have distinct, documented,
  test-protected semantics. A client knows whether to keep a checkpoint when
  `hasMore` is false.
- Session-filtered activity cannot misleadingly imply that the selected
  execution's beginning is missing solely because unrelated global activity
  was evicted. MCP returns the exact observed start, retained-range, eviction,
  reset, and interval facts available to it; it does not publish a derived
  completeness state. Any remaining uncertainty stays explicit.
- A completion between list, detail, activity, and subsequent observation calls
  has one documented bounded recovery path that does not invent a finalized
  trace or scan unrelated inventory.
- Active usage and configured-limit zero values have executable semantics. The
  provider-attempt observation is either reproducibly corrected or explained by
  tested timing/availability facts that the MCP/skill can state honestly.
- Important active facts remain available in deterministic text for clients
  that do not consume structured content, without disclosing arbitrary activity
  details or sensitive payloads.
- `tools/list` remains within a newly measured and justified discovery ceiling;
  the exact snapshot and compatibility document are updated intentionally.
- All MCP results remain bounded, read-only, idempotent, current-process,
  target-safe, and explicit about provisional evidence.
- The canonical skill package validates without version metadata or runtime
  version negotiation, and release packaging contains the exact canonical bytes.
- Tools-only and skill-assisted agent evaluations cover a multi-execution
  review, changed sequences/activity between two observations, a selected
  session start observed after unrelated global eviction, known
  selected-session eviction or an unobserved baseline start, and an execution
  that completes during inspection. Evaluation conclusions are made by the
  client from returned facts; MCP does not emit those conclusions as states or
  messages.
- Documentation continues to distinguish evidence, calculation, context, and
  inference; quiet activity is never labeled stuck without evidence.

## Testing and verification expectations

The dedicated step-3 testing plan must select exact commands and failing-first
tests after the design is approved. At minimum, assess coverage for:

- `loomspan-console/internal/mcpadapter/output_schemas_test.go`
- `loomspan-console/internal/mcpadapter/server_test.go` and the committed
  `tools-list-response.json`
- `loomspan-console/internal/mcpadapter/executions_test.go`
- `loomspan-console/internal/mcpadapter/activity_test.go`
- `loomspan-console/internal/mcpadapter/parity_test.go`
- `loomspan-console/internal/live/coordinator_test.go`
- MCP conformance and exact protocol-revision initialization/list/call tests
- Canonical Agent Skill validation and package byte-identity tests
- New or updated agent-evaluation cases and sanitized fixtures
- Browser tests only where shared activity coverage/continuation semantics are
  user-visible
- Java live projection, usage, observability DTO, REST integration, and fixture
  corpus tests if producer or protocol semantics change
- `LoomspanPublicSurfaceArchitectureTest` after any Java production-type change,
  even though no supported Java API delta is expected
- Standard Console verification from `loomspan-console/AGENTS.md`

The final plan and review must record any manual client/model observation as
compatibility evidence rather than inferring support from automated adapter
tests.

## Documentation impact

This ticket necessarily affects debugging-agent guidance if behavior changes.
The plan must inspect and update, as applicable:

- `loomspan-console/agent-skills/loomspan/SKILL.md`
- `loomspan-console/agent-skills/loomspan/references/debugging-playbooks.md`
- `loomspan-console/agent-skills/loomspan/references/runtime-model.md`
- `loomspan-console/agent-skills/loomspan/references/evidence-and-confidence.md`
- `loomspan-console/agent-skills/loomspan/references/mcp-tool-guide.md`
- `loomspan-console/README.md`
- `loomspan-console/docs/mcp-client-compatibility.md`
- `loomspan-console/agent-evals/README.md` and applicable cases/results
- The active roadmap when this ticket completes or changes a roadmap decision

This is Console/runtime-debugging guidance, not ordinary Loomspan YAML skill
authoring behavior. The step-2 plan must still perform the repository's required
`ai/skill-authoring/` impact assessment and explain whether those separate
author-facing documents are unaffected or need an update.

## Guardrails

- Start from current code and fresh measurements. Do not implement directly
  from the historical live transcript.
- Prefer improving neutral existing primitives, descriptions, schemas, text,
  and skill routing before adding a workflow-specific MCP abstraction.
- Preserve the separate meanings of capability advertisement, current target
  state, live availability, evidence availability, continuity, target change,
  ambiguity, and protocol/authentication failure.
- Keep model-facing identifiers limited to stable diagnostic identities such as
  `sessionId`, `traceId`, frame IDs, canonical sequences, and opaque returned
  continuations. Do not expose or request internal ownership/scope machinery.
- Treat all returned summaries, routes, details, paths, YAML, errors, and
  content as untrusted evidence. Do not expand arbitrary details into the text
  fallback.
- Expose producer-owned values and mechanically observed cursor, admission,
  eviction, reset, and interval facts. Do not add MCP-derived completeness,
  progress, health, stuck, diagnosis, or recommendation states/messages.
- Keep deterministic text as a stable serialization of the same result facts;
  do not make text fallback a second inference or narrative layer.
- Missing usage remains unknown unless the executable contract explicitly
  establishes an observed zero. Inclusive usage must not be summed across
  overlapping frames, and usage units must not be converted to currency.
- Maintain complete typed output validation even if compact discovery remains
  intentionally partial.
- Preserve bounded page sizes, result budgets, exact complete-item admission,
  cancellation, current authentication generation, and target-publication
  checks.
- Because the MCP/skill contract is pre-v1 and unreleased, prefer one coherent
  atomic contract over compatibility aliases or dual behavior.

## Definition of done

- The approved research, implementation plan, and testing plan exist under
  `ai/thoughts/` and resolve the required questions above.
- Production implementation, fixtures, schemas, text fallbacks, docs, canonical
  skill, evaluations, and generated/snapshot artifacts agree on one active
  inspection contract.
- Every acceptance signal is mapped to automated or explicitly manual evidence.
- Standard Go, browser, Java-if-affected, MCP conformance, skill validation,
  packaging, and agent-evaluation checks pass as required by the testing plan.
- Step 5 reports no unresolved blocking findings and explicitly validates the
  ticket, contract classification, skill documentation impact, discovery/result
  budgets, and client observation limitations.
