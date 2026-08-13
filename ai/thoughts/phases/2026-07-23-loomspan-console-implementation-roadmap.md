# Loomspan Console Implementation Roadmap

## Status

Proposed execution roadmap. This document decomposes the settled Loomspan Console
design into mergeable pull requests. It is not a replacement for the focused
research, implementation plan, testing plan, implementation, and code review
required for each ticket.

## Authoritative design inputs

- [Phase 1 Observability Foundation](./LOOMSPAN_console_phase_1_observability_foundation.md)
- [Phase 2 Personal UI Console](./LOOMSPAN_console_phase_2_ui_console.md)
- [Phase 3 LLM Runtime Inspector](./LOOMSPAN_console_phase_3_llm_runtime_inspector.md)
- [Developer Workflows](./LOOMSPAN_console_workflows.md)
- [Framework Feature Design Lens](../framework-feature-design-lens.md)

If a ticket appears to conflict with a settled phase decision, follow the
settled-design guardrail in the workflow document rather than silently changing
the design through implementation.

## Execution model

The roadmap uses rolling-wave planning:

1. Keep all ticket briefs available for scope and dependency control.
2. Run fresh codebase research for the next ticket.
3. Create and approve one detailed implementation plan.
4. Create its focused testing plan before implementation.
5. Implement, verify, and independently review the pull request.
6. Revalidate the next two or three briefs against the repository after merge.

Later briefs may be refined as implementation establishes concrete packages and
APIs. Their outcome, settled design constraints, and phase completion coverage
must not change without an explicit design decision.

## Pull-request sizing policy

Pull-request size is governed by semantic cohesion, atomic boundary changes,
implementation-planning clarity, and context-safe reviewability rather than
elapsed implementation time. Prefer the largest coherent change whose design
inputs, implementation plan, exact diff, tests, and review evidence can be
evaluated together with adequate context headroom. Split work when independent
correctness arguments can be separated cleanly, not merely because a diff is
large.

Each pull request should:

- deliver one coherent behavior or enforce one architectural invariant;
- include production code, tests, fixtures, samples, and documentation needed
  to make that behavior complete;
- leave the repository buildable and avoid dormant half-supported boundaries;
- keep security, lifecycle, concurrency, compatibility, and other independent
  correctness arguments explicit enough to plan and review reliably;
- avoid splitting atomic producer, consumer, fixture, and semantic-test changes
  merely to reduce line count or boundary count;
- account for repeated file and boundary churn when deciding whether a clean
  split improves or weakens reviewability; and
- update protocol producers, in-repository consumers, executable fixtures, and
  semantic tests atomically when a consumed boundary changes.

Large diffs, many affected boundaries, or a plan that consumes most available
implementation or review context are prompts to reassess scope, not automatic
reasons to split. A larger coherent pull request is preferable when splitting
would create dormant boundaries, duplicate file churn, or temporarily
inconsistent behavior. Generated fixtures, lockfiles, and mechanical fixture
updates should be evaluated separately from handwritten correctness-sensitive
changes.

## Dependency overview

```text
Phase 1:
01 -> 02 -> 03 -> 04 -> 05 -> 06

Phase 2:
07 -> 08 -> 09 -> 10 -> 11 -> 12 -> 13 -> 14 -> 15

Phase 3:
16 -> 17 -> 18 -> 19

Portable trace extension:
15 -> 25 -> 18

Cross-phase:
01 -> 07
06 -> 09
09 -> 16
15 -> 16
11 -> 17
12 -> 18
13 -> 18
25 -> 18
```

PR 07 may begin after PR 01 settles the current-release trace direction. PR 09
must not claim target compatibility until the Phase 1 adapter contract is
complete. Phase 3 depends on the transport-neutral seams established throughout
Phase 2.

## Phase 1 — Observability foundation

| PR | Ticket | Outcome |
|---:|---|---|
| 01 | [Canonical trace semantics and executable fixtures](../tickets/loomspan-console-pr-01-canonical-trace-semantics.md) | Establish the current-release evidence contract consumed by the console. |
| 02 | [Observation lifecycle and live projection core](../tickets/loomspan-console-pr-02-observation-lifecycle.md) | Publish bounded per-execution live state after successful canonical append without limiting engine concurrency or changing execution outcomes. |
| 03 | [Skill and finalized-trace catalogs](../tickets/loomspan-console-pr-03-observability-catalogs.md) | Expose registered YAML and current-process finalized artifact metadata with correct ownership and lifetime. |
| 04 | [Spring adapter foundation and REST snapshots](../tickets/loomspan-console-pr-04-spring-rest-adapter.md) | Add the secured, compatible, paginated application observability REST boundary. |
| 05 | [Live SSE delivery](../tickets/loomspan-console-pr-05-live-sse-delivery.md) | Deliver bounded resumable live activity without coupling clients to execution. |
| 06 | [Finalized artifact streaming and Phase 1 integration](../tickets/loomspan-console-pr-06-artifact-streaming-integration.md) | Complete artifact acquisition, integration fixtures, samples, and Phase 1 verification. |

## Phase 2 — Personal UI console

| PR | Ticket | Outcome |
|---:|---|---|
| 07 | [Console project and reproducible build foundation](../tickets/loomspan-console-pr-07-console-build-foundation.md) | Produce a versioned Go executable with freshly built embedded React assets. |
| 08 | [Profile, workspace, and local browser security](../tickets/loomspan-console-pr-08-local-security-workspace.md) | Establish safe profile ownership, disposable storage, loopback serving, pairing, sessions, and CSRF. |
| 09 | [TargetContext and selected-target lifecycle](../tickets/loomspan-console-pr-09-target-context.md) | Create the reusable target authority, credential-entry paths, identity, compatibility, cancellation, status, and error seam. |
| 10 | [Read-only operational views](../tickets/loomspan-console-pr-10-operational-views.md) | Deliver Overview, Skill Catalog, Active Executions, and trace-catalog browsing over shared services. |
| 11 | [Live activity and active-execution detail](../tickets/loomspan-console-pr-11-live-execution-experience.md) | Deliver one continuous bounded activity interval and the slow/live execution workflow. |
| 12 | [Central artifact acquisition and Trace Storage](../tickets/loomspan-console-pr-12-artifact-service.md) | Own one shared acquired copy, handle, capacity charge, TTL, and removal lifecycle per trace, plus the separate unchanged raw-download pass-through. |
| 13 | [Trace parser, indexes, and shared calculations](../tickets/loomspan-console-pr-13-trace-analysis-services.md) | Validate and query traces with authoritative hierarchy, duration, usage, failure, and payload semantics. |
| 14 | [Trace explorer foundation](../tickets/loomspan-console-pr-14-trace-explorer.md) | Present navigable hierarchy, timeline, usage, records, payloads, raw download, and evidence links. |
| 15 | [Diagnostic workflows and Phase 2 hardening](../tickets/loomspan-console-pr-15-diagnostic-workflows.md) | Complete the settled workflows, degraded paths, accessibility, security, E2E, and release verification. |
| 25 | [Save and open complete same-version traces](../tickets/loomspan-console-pr-25-portable-trace-import.md) | Add exact-version portable trace files and transient target-independent import shared by the browser and future MCP adapter. |

## Phase 3 — LLM runtime inspector

| PR | Ticket | Outcome |
|---:|---|---|
| 16 | [MCP authentication and lifecycle foundation](../tickets/loomspan-console-pr-16-mcp-foundation.md) | Prove MCP lifecycle safety and expose authenticated bootstrap/status with the complete runtime-status capability. |
| 17 | [Runtime, skill, and live-inspection MCP surface](../tickets/loomspan-console-pr-17-mcp-runtime-inspection.md) | Adapt shared runtime, skill, execution, and recent-activity services to MCP. |
| 18 | [Trace-inspection MCP surface](../tickets/loomspan-console-pr-18-mcp-trace-inspection.md) | Adapt shared acquisition and trace queries with progressive, continuable evidence retrieval. |
| 19 | [Portable debugging skill and interoperability](../tickets/loomspan-console-pr-19-debugging-skill.md) | Ship the evidence-guided Agent Skill and prove representative client interoperability. |

## Cross-cutting invariants

Every detailed plan must preserve the applicable invariants below.

### Contract and compatibility

- Classify affected surfaces using the six categories in the framework design
  lens before deciding compatibility.
- Permit only the PR 25 same-`consoleCompatibilityVersion` portable trace
  contract; treat Console imports and all derived trace state as transient, not
  historical or cross-version persisted state.
- Remove approved obsolete internal paths atomically; do not add an overload,
  alias, legacy reader, fallback, bridge, or dual behavior without an identified
  protected contract and explicit approval.
- Coordinate Java, Go, fixtures, semantic tests, and documentation whenever a
  REST, SSE, acquisition, problem, or consumed-NDJSON meaning changes.
- Use the exact complete Loomspan release string as
  `consoleCompatibilityVersion`; do not add an independent trace version.
- Permit matching `development` trace imports to attempt normal complete
  validation without promising compatibility or adding fallback behavior.

### Execution and evidence

- Optional observability must not change execution outcome or canonical trace
  and journal failure semantics.
- Live projection publishes only after complete canonical append succeeds.
- Browser and MCP share Go-owned deterministic calculations and domain errors.
- Recorded facts, deterministic relationships, uncertainty, and interpretations
  remain distinguishable.
- The console never claims hidden model reasoning or a speculative root cause.

### Security and lifecycle

- Application, browser, and MCP credentials remain separate and never enter
  diagnostic results, URLs, ordinary configuration, or logs.
- Application content is untrusted data and cannot trigger server-side
  execution, filesystem access, network access, credential use, target changes,
  configuration changes, or additional MCP operations.
- Target scope, application instance, activity continuity, artifact handles,
  browser sessions, and MCP authentication generations have explicit invalidation
  behavior.
- Network delivery and slow-client work never run under execution locks.
- Resources follow the exact settled bound: some use concrete tested limits,
  some allow an explicit developer-controlled `unlimited` or `never` choice,
  and some are lifecycle-proportional to authoritative engine or retention
  state. In particular, observability must not add an independent concurrency
  cap, admission limit, sampling rule, or omission behavior to the active
  registry.

### Documentation and verification

- Each detailed plan contains an evidence-backed skill-authoring documentation
  impact assessment.
- Automated and manual success criteria remain separate.
- Boundary fixtures assert consumed semantics, not parse success alone.
- Representative fixtures, tests, and agent evaluations reference the
  applicable approved workflow ID or most specific requirement ID.
- Code review independently evaluates correctness before checking plan
  conformance.

## Workflow coverage

| Workflow | Primary PRs |
|---|---|
| Diagnose a currently slow execution | 02, 04, 05, 09, 10, 11, 17 |
| Diagnose a failed completed execution | 01, 03, 06, 12, 13, 14, 15, 18, 19 |
| Explain unexpectedly high usage or cost | 01, 13, 14, 15, 18, 19 |
| Understand an unfamiliar nested skill path | 03, 10, 13, 14, 15, 17, 18, 19 |

## Phase gates

### Phase 1 gate

Do not begin target-facing Phase 2 behavior until the application adapter has
executable authentication, compatibility, pagination, SSE, artifact-streaming,
and consumed-trace fixtures. PR 07 and non-target local foundations in PR 08 may
proceed earlier.

Completion is tracked by the
[Phase 1 evidence matrix](LOOMSPAN_console_phase_1_completion_evidence.md).

### Phase 2 gate

Do not add MCP protocol adapters until target state, status, shared domain errors,
recent activity, artifact acquisition, and trace analysis are transport-neutral
services below browser handlers. The MCP lifecycle spike in PR 16 verifies the
chosen SDK before the remaining MCP surface is implemented.

### Phase 3 gate

Do not declare Phase 3 complete until MCP works without the skill, the skill
degrades safely without MCP or optional capabilities, required capability
absence is explicit, representative clients pass, and browser behavior remains
unaffected.

## Ticket-to-plan handoff

Before implementing any ticket:

1. Read the ticket and all authoritative design sections it cites.
2. Run fresh repository research and write or update a research artifact.
3. Resolve every contract classification, compatibility decision, public-surface
   delta, skill-authoring impact, and implementation uncertainty.
4. Write and approve the detailed implementation plan.
5. Write the focused testing plan, including applicable workflow or requirement
   IDs in representative fixture, test, and evaluation coverage.
6. Implement on a dedicated branch and update the detailed plan checkboxes.
7. Run an independent review using the exact diff, ticket, implementation plan,
   testing plan, and phase design.

Ticket briefs intentionally do not contain speculative file paths, exhaustive
test names, or unresolved compatibility conclusions. Those belong in the
evidence-backed detailed plan created against the repository state at execution
time.
