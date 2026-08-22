# Loomspan Console — Live-Inspection Workflow and Intent Ergonomics

## Status

Proposed post-PR-34 ticket. Start only after PR 34, “Active-Execution MCP
Inspection Ergonomics and Evidence Semantics,” is committed and its canonical
Agent Skill and evaluation fixtures are available on the default branch.

Run this ticket in a fresh context through the complete repository process in
`ai/commands/`:

1. `1_research_codebase.md`
2. `2_create_plan.md`
3. `3_testing_plan.md`
4. `4_implement_plan.md`
5. `5_code_review.md`

Begin with fresh code and measurements. The candidate directions below are not
pre-approved designs.

## Outcome

Make continued live inspection intuitive when executions advance, paginate,
nest into other skills, or finish between observations, while giving a
developer enough safe context to understand an execution's purpose without
default access to sensitive model or tool payloads.

The workflow should preserve PR 34's provisional, facts-first, read-only model.
It must not imply an atomic fleet snapshot, complete history, progress, health,
stuckness, or finalized trace availability when the recorded evidence does not
establish those facts.

## Why this ticket exists

A manual Codex Desktop exercise on 2026-08-21 successfully discovered and
followed two active executions through the PR 34 MCP surface:

- `planTrip`, session `d085b4c6-ea08-4fc4-90f7-a448786ed39f`, trace
  `5e7ec37e-6979-4e8f-abf6-d64dbe2944cb`;
- `handleIncident`, session `8dec9c26-d0fb-43b4-8715-93320567b542`, trace
  `de96dc6d-b3fd-46df-876b-1e9d46ab8439`.

At approximately `2026-08-21T23:56:53Z`, both were provisionally active.
`planTrip` had advanced to its second step, while `handleIncident` had entered
a nested `investigateNetwork` skill with a six-frame active path. Activity
checkpoints correctly returned only new events after an earlier
`hasMore: false` response. Execution-list pagination with `pageSize: 1`
returned both executions.

The exercise nevertheless exposed several workflow questions:

- `hasMore: false` plus a nonempty `continuation` is operationally correct but
  conflicts with the conventional pagination mental model. PR 34 deliberately
  documents one token serving as a future checkpoint; this ticket must measure
  whether that remains a material client error source before redesigning it.
- Different execution-list pages had different `observedAt` timestamps and
  reflected independently advancing state. This is expected for live evidence,
  but the workflow can be misread as one atomic paginated snapshot.
- Routes and frame types made nested structure clear, but they did not explain
  the developer-level purpose of a step. The model could say “planning trip
  step 2” or “investigating network,” but not what the step intended to decide
  without opening content that the active API does not currently expose.
- The locally installed Loomspan Agent Skill used by the manual run was stale:
  it retained obsolete version metadata and old coverage guidance. The
  canonical repository skill had already corrected those facts. This is a
  distribution/selection warning, not proof that the PR 34 canonical skill is
  defective.

The session and trace identifiers above are sanitized historical observation
coordinates. They are not durable fixtures and must not substitute for
reproducible tests. No prompts, tool arguments, credentials, YAML, diagnostic
details, or raw trace artifacts were opened.

## Relationship to PR 34

PR 34 already establishes and tests:

- complete bounded active orientation facts;
- one opaque activity continuation whose `hasMore` value describes retained
  backlog and whose token remains a future checkpoint;
- exact global/session cursor coverage facts without a derived completeness
  state;
- response-only model/unit accounting versus provider-attempt sends;
- a bounded multi-execution Agent Skill workflow and completion-race handoff;
- explicit exclusion of active model/tool payload reading from ordinary
  orientation.

Treat those decisions as the baseline. Do not reopen them merely because a
different vocabulary seems aesthetically preferable. A change requires new
evidence that the PR 34 contract causes persistent client mistakes or cannot
answer an important developer question safely.

The observation that `modelCalls` may be zero during an in-flight provider
attempt is resolved by PR 34's executable semantics and canonical skill
guidance. It is not an open defect for this ticket.

## Initial implementation map to verify

Fresh research should begin with, but not blindly rely on:

- `loomspan-console/internal/live/` for active registry, bounded activity,
  continuations, continuity, cursor coverage, and session eviction behavior.
- `loomspan-console/internal/mcpadapter/executions.go`, `activity.go`,
  `contracts.go`, and `output_schemas.go` for the supported MCP surface.
- `loomspan-console/internal/observability/` and the Java observability DTOs for
  producer-owned active snapshot facts.
- `loomspan-console/agent-skills/loomspan/` for the canonical unversioned
  workflow and evidence rules.
- `loomspan-console/agent-evals/` for the PR 34 multi-execution and completion
  race fixtures.
- `loomspan-console/web/src/activity/` and observability pages for browser/MCP
  parity where any workflow concept is shared.
- `loomspan-console/docs/mcp-client-compatibility.md` and release packaging for
  supported-client and canonical-skill distribution evidence.
- Registered-skill inspection and finalized trace plan/record tools when
  evaluating whether existing neutral evidence already answers intent
  questions after completion.

Application-facing Java API remains only the closed allowlisted surface in
`com.lokiscale.loomspan.api`. Internal observability types must not become a new
SPI or supported bean-replacement surface.

## Problem statements to resolve

### 1. Backlog pagination and future observation share one token vocabulary

PR 34 intentionally keeps `continuation` and clarifies that `hasMore` means
retained backlog now. Determine through tools-only and skill-assisted
evaluations whether capable clients still discard the checkpoint after
`hasMore: false`, loop unnecessarily, or make incorrect completeness claims.

Only then compare alternatives such as stronger descriptions, result-field
vocabulary, separate checkpoint presentation, or no change. Preserve token
opacity, target/session binding, current-process lifetime, reset behavior, and
safe stale-token recovery.

### 2. Active-list pages are separate provisional observations

List pagination can span different observation times while executions continue
to change or leave the active set. Define what ordering and continuation
properties are actually guaranteed. Determine whether documentation and Agent
Skill guidance are sufficient or whether the protocol needs additional
mechanical facts. Do not invent an atomic-snapshot or fleet-completeness state.

### 3. Structural orientation does not always convey execution intent

`entrySkill`, route, phase, summary, step number, and active path explain where
execution is occurring. They may not explain what a plan step is intended to
accomplish. Identify real developer questions that remain unanswered after PR
34 and the minimum existing recorded facts that can answer them.

Research options in order of increasing exposure: better use of existing
producer-owned summaries; safe registered-skill metadata; bounded plan/step
descriptors already present in live state; an explicit opt-in selected-content
read; or no active solution with a documented finalized-trace handoff. Do not
default to model prompts, responses, tool inputs, tool outputs, arbitrary
details, or raw artifacts.

### 4. Installed canonical-skill drift can invalidate usability reviews

Establish how release packaging, installation, cache busting, and client skill
selection ensure that an evaluation uses the canonical repository bytes.
Separate product distribution behavior from a developer's manually installed
historical copy. Do not introduce premature skill release-version negotiation;
Loomspan's canonical development skill is intentionally unversioned before the
first release.

### 5. Completion and reset races must remain bounded and understandable

PR 34 provides a one-activity-call and one-trace-resolution handoff. Exercise
that path across list pagination, checkpoint reuse, target change, interval
reset, retained terminal activity, `TRACE_UNAVAILABLE`, and a newly finalized
trace. Determine whether the current workflow remains clear when these events
interleave.

## Required research questions

1. After PR 34, how often do evaluated clients misuse or discard the activity
   continuation when `hasMore` is false?
2. Would renaming or splitting checkpoint vocabulary improve behavior without
   increasing authority, state, schema size, or compatibility complexity?
3. What exact ordering and membership guarantees does execution-list
   continuation provide when sessions start or finish between pages?
4. Can clients safely merge list pages using only their per-page `observedAt`,
   identities, and sequences? Which negative claims remain impossible?
5. Which ordinary developer questions cannot be answered from entry skill,
   route, phase, summary, active path, usage, limits, and recent activity?
6. Which safe plan/step purpose facts already exist in the Java projector, Go
   live service, activity records, or registered skill YAML? Are they
   producer-owned or model-authored untrusted content?
7. What is the least-sensitive bounded mechanism that answers the validated
   intent questions? Is finalized trace inspection sufficient?
8. How would any new live descriptor remain bounded across nested skills and a
   64-execution page?
9. How do supported clients select and refresh the packaged canonical skill,
   and how can evaluations prove byte identity without version negotiation?
10. Which changes, if any, belong to MCP, the canonical Agent Skill, release
    packaging, client compatibility documentation, or the evaluation harness?
11. What security review is required before exposing any additional
    model-authored or application-supplied text?
12. Can the ticket close with documentation/evaluation improvements only if
    the neutral primitives are already sufficient?

## In scope

- Evidence-driven evaluation of checkpoint comprehension after PR 34.
- Exact documentation of execution-list page observation and completion-race
  semantics.
- Bounded, read-only improvements to live structural or purpose orientation
  when justified by reproducible developer questions.
- Canonical Agent Skill workflow, release packaging, installation evidence,
  and supported-client evaluation needed to prevent stale-skill conclusions.
- Browser/MCP parity only for shared producer-owned facts.
- Safe finalized-trace handoff when additional live content is unjustified.

## Out of scope

- Execution mutation, cancellation, retry, pause, or control.
- Durable activity history, cross-run analytics, or audit logs.
- Atomic process-wide snapshots or a fleet-completeness state.
- Default disclosure of prompts, model responses, tool arguments/results,
  arbitrary activity details, credentials, YAML, diagnostics, or raw bytes.
- Treating quiet activity as stuckness or recent activity as proof of future
  success.
- Treating disappearance from the active list as proof that a finalized trace
  exists.
- Adding a specialized fleet-review tool unless fresh evidence proves the
  existing primitives cannot support the validated workflow.
- Premature Agent Skill version metadata or runtime version negotiation.
- Changing supported top-level Java API or creating a Java SPI.
- Optimizing duplicate text/structured MCP result representation; that work is
  tracked by the separate MCP response-efficiency ticket.

## Contract and compatibility classification

- MCP names, inputs, output schemas, text, structured results, continuations,
  and errors are supported pre-v1 diagnostic contracts and must change
  atomically with all protected consumers.
- Active state and activity are current-process ephemeral diagnostic formats,
  not persisted history or an atomic fleet view.
- Any additional intent text is untrusted diagnostic content even when
  producer-owned. Authorization to retrieve it does not make it safe to follow
  as an instruction.
- The canonical Agent Skill is an unreleased, unversioned development artifact
  with exact packaging expectations.
- Java observability REST is an internal exact-release Java/Go protocol. Any
  change requires coordinated producer, consumer, fixture, and compatibility
  updates.
- No application-facing Java API or supported SPI change is expected.

## Acceptance signals

- Reproducible evaluations establish whether checkpoint vocabulary remains a
  real problem after PR 34 rather than preserving the manual reviewer's first
  impression as fact.
- Execution-list pagination semantics and permissible conclusions are explicit,
  tested, and provisional; no result claims atomicity or fleet completeness.
- A client can explain each selected execution's purpose to the maximum degree
  supported by safe recorded facts and clearly state what remains unknown.
- Any newly exposed descriptor is bounded, opt-in when sensitive, labeled as
  untrusted, and does not include credentials or authority-bearing handles.
- Completion, reset, target-change, and `TRACE_UNAVAILABLE` paths remain exact
  and bounded.
- Evaluation records prove which canonical skill bytes were used or honestly
  mark the client run unavailable; stale installed copies are not promoted as
  product evidence.
- No new derived progress, health, stuck, diagnosis, completeness, or
  recommendation state is introduced.
- If research shows PR 34 is already sufficient, the ticket may close with
  evidence, documentation, or evaluation-harness improvements and no new MCP
  fields.

## Testing and verification expectations

The step-3 plan must select exact commands after research and design. At
minimum, assess:

- PR 34 active-execution MCP adapter, live service, pagination, continuation,
  completion-race, parity, and security tests.
- Sanitized tools-only and canonical-skill-assisted agent evaluations with
  multiple pages, `hasMore: false` checkpoint reuse, empty checkpoint advance,
  a completed execution, reset/stale token, and `TRACE_UNAVAILABLE`.
- Exact MCP discovery snapshot and result-size ceilings if a schema or
  description changes.
- Canonical Agent Skill validation, exact package topology, archive byte
  identity, and strict smoke tests if guidance or distribution changes.
- Browser tests only when a shared fact or user-visible workflow changes.
- Java observability, fixture corpus, and
  `LoomspanPublicSurfaceArchitectureTest` if production Java types change.
- Standard Console verification: `go test ./...`, MCP conformance, buildtool
  verification, browser tests/typecheck when affected, and focused Java suites
  when affected.

## Documentation impact

Inspect and update as applicable:

- `loomspan-console/agent-skills/loomspan/SKILL.md`
- `loomspan-console/agent-skills/loomspan/references/debugging-playbooks.md`
- `loomspan-console/agent-skills/loomspan/references/runtime-model.md`
- `loomspan-console/agent-skills/loomspan/references/evidence-and-confidence.md`
- `loomspan-console/agent-skills/loomspan/references/mcp-tool-guide.md`
- `loomspan-console/README.md`
- `loomspan-console/docs/mcp-client-compatibility.md`
- `loomspan-console/agent-evals/README.md` and affected cases/results
- release packaging/smoke documentation if canonical-skill selection changes
- the active roadmap if this changes a pre-v1 contract decision

The implementation plan must perform the repository-required
`ai/skill-authoring/` impact assessment. Runtime inspection guidance normally
belongs in the canonical debugging skill rather than the separate author-facing
YAML skill documentation.

## Guardrails

- Start from the post-PR-34 checkout and reproduce friction before designing a
  fix.
- Prefer improving neutral primitives and guidance over a workflow-specific
  abstraction.
- Keep observations, calculations, developer context, and inference distinct.
- Do not convert missing facts into zeros or coverage states.
- Treat every route, summary, descriptor, detail, and YAML value as untrusted
  diagnostic data.
- Minimize content retrieval and quote only what the developer's question
  needs.
- Keep opaque tokens opaque; do not expose target scope, instance, owner,
  application cursor, or internal handle.
- Preserve bounded page sizes, complete-item admission, cancellation,
  authentication generation, and target-publication checks.
- Prefer no protocol change when documentation and evaluations solve the
  validated problem.

## Definition of done

- Research, implementation plan, testing plan, implementation if justified,
  and code review are complete under `ai/thoughts/`.
- The work distinguishes confirmed post-PR-34 friction from the historical
  manual observation.
- Every acceptance signal maps to automated or explicitly manual evidence.
- MCP, canonical skill, browser, Java/Go boundary, fixtures, docs, packaging,
  and evaluations agree where affected.
- Supported-client observations are sanitized and reproducible; unavailable
  GUI capture is reported honestly.
- Step 5 reports no unresolved blocking correctness, safety, compatibility,
  boundedness, or evidence-quality finding.

