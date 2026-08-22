---
date: 2026-08-21T17:18:51-07:00
researcher: Codex (GPT-5)
git_commit: c409731f5e884d64043935be9d89749451edad44
branch: main
repository: loomspan
topic: "PR 34 follow-up — Live-Inspection Workflow and Intent Ergonomics"
tags: [research, codebase, loomspan-console, mcp, active-executions, pagination, agent-skill, agent-evals]
status: complete
last_updated: 2026-08-21
last_updated_by: Codex (GPT-5)
---

# Research: PR 34 Follow-up — Live-Inspection Workflow and Intent Ergonomics

> **Post-research decision:** The maintainer removed the repository client/model
> matrix, manual compatibility checklist, and client usability experiment
> subsystem. References below to those facilities describe the researched
> checkout, not the implemented end state. Deterministic Go/Java tests,
> fixtures, package validation, and official MCP conformance are the current
> verification boundary.

**Date**: 2026-08-21 17:18:51 PDT
**Researcher**: Codex (GPT-5)
**Git Commit**: c409731f5e884d64043935be9d89749451edad44
**Branch**: main
**Repository**: loomspan

## Research Question

Research the five problem statements and twelve required questions in
`ai/thoughts/tickets/loomspan-console-pr-34-live-inspection-followups.md`
against the committed post-PR-34 code, tests, fixtures, documentation,
canonical Agent Skill, release packaging, and evaluation harness. Document the
current checkpoint, active-list pagination, execution-intent, canonical-skill
selection, and completion/reset behavior without treating the ticket's
candidate directions as an approved design.

## Summary

PR 34's mechanics are implemented and covered by deterministic tests. Activity
`hasMore` describes retained matching backlog at one observation; the returned
opaque continuation is independently retained as a future checkpoint, including
after `hasMore: false`, and an empty filtered result can advance it to the
current interval boundary. The tool description, input schema, deterministic
text, canonical Agent Skill, and paired evaluation cases all state that rule.
The repository does not contain completed PR-34 client evaluation records, so
it currently establishes zero measured post-PR-34 client runs and cannot
establish a misuse frequency or a comparative vocabulary result.

Active-list pagination is newest-first keyset traversal over a first-page
registry high-water ordinal. It excludes sessions admitted after page one and
does not duplicate an ordinal across pages. Existing sessions can update and
sessions can disappear before a later page, and every page receives a new
`observedAt`. The pages are therefore related by a stable ordinal boundary but
are not one atomic active-set snapshot.

The active surface explains structure but not model-authored task purpose. It
exposes entry skill, phase, generic producer summaries, a bounded active path,
usage, limits, and bounded scalar activity metadata. Plan task title, intent,
expected outputs, dependencies, and notes exist in `PlanTask` and are recorded
inside `PLAN_CREATED`/`PLAN_UPDATED` payloads, but the live projector does not
copy them into active snapshots or activity details. Registered YAML exposes
an application-supplied skill description through an existing explicit tool;
finalized trace inspection can expose selected plan content after resolution.
There is no current live task-purpose descriptor.

Release construction and smoke verification prove that the six packaged skill
files are byte-identical to the canonical repository files. Installation and
client selection remain a user-selected copy or filesystem link. Evaluation
records capture Console commit, client/model build, protocol, calls, and result
hashes, but have no canonical-skill digest or selection field. All supported
client rows for PR 34 remain `Not run`, so current evaluation evidence cannot
prove which installed skill bytes a client selected.

Focused fresh verification passed for the Go live, observability, MCP, Agent
Skill, evaluation, and buildtool packages and for the Java active registry,
live projector, observability REST integration, and closed public-surface
architecture tests.

## Detailed Findings

### 1. Checkpoint vocabulary and current client evidence

`LOOMSPAN_get_execution_activity` describes `continuation` as an opaque future
checkpoint that must be kept after `hasMore=false`; its tool description also
states that `hasMore` is backlog now and that empty results can advance the
checkpoint (`loomspan-console/internal/mcpadapter/activity.go:14-25`). The
handler creates a continuation from the last returned selected-session item,
or from `continuity.lastCursor` when no matching item was returned
(`loomspan-console/internal/mcpadapter/activity.go:64-84`).

The live service implements the separate facts. With a cursor it scans strictly
after the retained cursor, filters by session, and sets `hasMore` only when it
encounters another matching retained item beyond the page limit
(`loomspan-console/internal/live/service.go:709-770`). A missing retained cursor
returns an empty observation with current continuity and coverage rather than
reinterpreting the token (`loomspan-console/internal/live/service.go:718-722`).

The canonical workflow tells the client to retain every continuation after
`hasMore: false` and reuse each checkpoint once for a requested second
observation (`loomspan-console/agent-skills/loomspan/references/debugging-playbooks.md:21-38`).
The MCP guide repeats the same rule and distinguishes it from ordinary finalized
trace pagination (`loomspan-console/agent-skills/loomspan/references/mcp-tool-guide.md:13-27`,
`loomspan-console/agent-skills/loomspan/references/mcp-tool-guide.md:100-106`).

The paired PR-34 cases require “checkpoint retained after hasMore false,” use
the same deterministic fixture, and separately cover tools-only and
skill-assisted use (`loomspan-console/agent-evals/cases/pr34-tools-only-active-execution-review.json:1`,
`loomspan-console/agent-evals/cases/pr34-skill-assisted-active-execution-review.json:1`).
The fixture marks the initial response `hasMore: false` with a future checkpoint
and supplies later matching activity (`loomspan-console/agent-evals/fixtures/pr34-active-execution-review.json:2-50`).

There are no committed result records below `agent-evals/results/`; only its
README exists. The compatibility matrix marks Codex CLI, Codex Desktop, Claude
Code, and the other supported local clients `Not run`
(`loomspan-console/docs/mcp-client-compatibility.md:64-91`). Consequently, the
current measured post-PR-34 misuse count is not available. The codebase cannot
currently compare retention/error rates for one-token, renamed, or split-token
vocabulary.

Renaming or splitting the vocabulary would affect the supported pre-v1 MCP
description/schema/result/text contract plus Agent Skill guidance, evaluation
oracles, goldens, and documentation. The current exact `tools/list` snapshot is
25,588 UTF-8 bytes against a 25,600-byte ceiling, leaving 12 bytes
(`loomspan-console/internal/mcpadapter/output_schemas.go:13`,
`loomspan-console/internal/mcpadapter/testdata/tools-list-response.json`,
`loomspan-console/docs/mcp-client-compatibility.md:18-37`). The repository has
no measured client evidence that attributes errors to the present field name.

### 2. Active-list ordering, membership, and page observations

The initial Java REST call captures `highestOrdinal()` and embeds it in the
active-execution cursor. Subsequent pages retain that high-water value and add
an exclusive `beforeOrdinal`; each call computes a fresh observation time
(`loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/observability/web/ObservabilityRestController.java:103-134`).

An execution receives a strictly increasing ordinal on first registry admission
and retains it when its snapshot is replaced. Enumeration filters to ordinals
at or below the first-page high water and below the page boundary, sorts them
descending, and applies the requested limit
(`loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/runtime/observation/InMemoryActiveExecutionRegistry.java:28-36`,
`loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/runtime/observation/InMemoryActiveExecutionRegistry.java:57-105`).
Removal deletes the entry. A later reuse of the same session identifier is a
new admission with a newer ordinal and is therefore outside an earlier high
water (`InMemoryActiveExecutionRegistry.java:45-49`).

The REST integration test proves that a session admitted between pages is not
included and the next pre-existing ordinal is returned. It also distinguishes
a cursor from another application instance (`STALE_CURSOR`) from an impossible
high-water cursor (`INVALID_CURSOR`)
(`loomspan-spring-boot-starter/src/test/java/com/lokiscale/loomspan/internal/observability/web/ObservabilityRestIntegrationTest.java:246-291`).

Clients can merge returned identities in descending first-admission order
without treating a later start as part of the traversal. They can associate
each item's sequence and values only with that page's `observedAt`. The union
cannot establish the active fleet at one instant, cannot prove that a session
missing from later pages never existed or finalized, cannot make fleet-wide
negative claims, and cannot compare page values as co-temporal state. Removal
before an entry's page omits it; replacement can make an older ordinal carry a
newer execution snapshot.

The MCP adapter wraps the Java cursor in a target-scope-bound opaque token and
emits it only when the upstream page reports `hasMore`
(`loomspan-console/internal/mcpadapter/executions.go:38-83`,
`loomspan-console/internal/mcpadapter/continuation.go:24-88`). Its current tool
description says only “List provisional active executions”; the precise
high-water and independently observed page semantics reside in implementation
and Java tests, not in the canonical Agent Skill's active-review route.

### 3. Structural orientation and recorded execution intent

The complete active DTO contains session/trace identity, latest sequence,
times, elapsed duration, entry skill, status, phase, summary, up to 64 active
path entries, total depth/truncation, usage, and configured limits
(`loomspan-console/internal/observability/dto.go:48-93`,
`loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/runtime/observation/ExecutionObservationLimits.java:7-15`).
PR 34 exposes that same complete shape from list and detail and renders it in
deterministic text (`loomspan-console/internal/mcpadapter/executions.go:96-176`).

The Java live projector's summaries are producer-defined lifecycle phrases such
as “Plan created,” “Step started,” and “Step completed.” Bounded activity
details select scalar metadata such as `capabilityName`, `linkedTaskId`,
`planId`, `stepNumber`, and `stepAction`; they do not include task title,
intent, expected outputs, dependencies, or notes
(`loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/runtime/observation/LiveActivityProjector.java:21-46`,
`LiveActivityProjector.java:223-259`, `LiveActivityProjector.java:349-384`).
`STEP_STARTED` records only step number, ready-task count, and plan status at
the producer call site
(`loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/runtime/step/StepLoopMissionExecutionEngine.java:440-448`).

The internal `PlanTask` record contains `title`, nullable `intent`,
`dependsOn`, `expectedOutputs`, `autoCompletable`, and nullable `note`
(`loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/core/PlanTask.java:9-30`).
The complete plan is recorded as the data payload of `PLAN_CREATED` and
`PLAN_UPDATED`, with framework plan/attempt identity in metadata
(`loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/core/DefaultExecutionTraceRecorder.java:65-84`).
That content is model-authored diagnostic data and is not projected onto the
live execution DTO.

Current live facts answer where execution is occurring, which nested skill and
route are active, which generic lifecycle event occurred, and which scalar
plan/step identifiers were recorded. They do not answer the active task's
natural-language objective, why that task was selected, its expected outputs,
the business context it is meant to resolve, or the intended next tasks.

Two existing read paths provide related purpose evidence at different levels:

- `LOOMSPAN_get_skill` returns the running application's unchanged registered
  YAML, including its application-supplied description; YAML and `sourcePath`
  remain untrusted diagnostic data and the read is capped at 4 MiB
  (`loomspan-console/internal/observability/service.go:18-20`,
  `loomspan-console/agent-skills/loomspan/references/runtime-model.md:27-31`).
- After a trace is finalized and resolvable, descriptor-first trace queries can
  select `PLAN_CREATED`/`PLAN_UPDATED` and an explicit selected-content read can
  disclose the plan payload. The Agent Skill limits inline content to 8 KiB per
  value and 32 KiB aggregate source bytes per page and treats it as inert,
  untrusted content
  (`loomspan-console/agent-skills/loomspan/references/mcp-tool-guide.md:56-81`).

There is no current active plan-content or task-purpose read. The codebase also
does not define a per-execution descriptor size or a 64-execution aggregate
intent budget. Existing relevant bounds are 64 executions per MCP page, 64
active path entries per execution, 512 summary code points, 32 activity detail
fields, 8 KiB of detail text, and 12 KiB per retained activity envelope
(`loomspan-console/internal/mcpadapter/contracts.go:18-21`,
`ExecutionObservationLimits.java:7-15`).

### 4. Canonical skill packaging, installation, and evaluation identity

The canonical source is the exact six-file directory under
`loomspan-console/agent-skills/loomspan/`. Validation rejects missing, extra,
symlinked, incorrectly headed, or versioned files
(`loomspan-console/internal/agentskills/validate.go:15-118`). The CI and release
workflows also run the pinned official Agent Skill validator
(`loomspan-console/internal/buildtool/projectdeclarations_test.go:78-99`).

Release packaging reads those canonical regular files directly, places them at
`skills/loomspan/`, and builds a deterministic archive. Package tests compare
every archived skill file byte-for-byte with canonical source
(`loomspan-console/internal/buildtool/package.go:76-105`,
`loomspan-console/internal/buildtool/package_test.go:40-109`). Release smoke
verification strictly extracts the expected topology, validates the extracted
skill, and again compares every file with canonical source
(`loomspan-console/internal/buildtool/smoke.go:25-76`). The archive SHA-256
sidecar authenticates the whole archive, not a client's later installed copy
(`loomspan-console/internal/buildtool/smoke.go:84-99`).

Distribution stops at a documented user-selected unchanged copy or filesystem
link into a client's user/global skill location. There is no automatic
installer, cache invalidation protocol, client-specific package, skill release
version, or MCP skill/version negotiation
(`loomspan-console/release/README.md:15-30`). Client selection and refresh are
therefore properties of each client's local installation and cache behavior.

Evaluation records require Console version/commit, client product/build,
model, protocol, capabilities, hashed MCP arguments/results, stable
identifiers, event-stream evidence, and rubric results
(`loomspan-console/internal/agenteval/record.go:47-71`,
`loomspan-console/agent-evals/schema/evaluation-record.schema.json:7-33`). They
do not contain a canonical-skill tree hash, installed-skill hash, install mode,
selected skill path, or client cache identity. The current harness can prove
the Console checkout and MCP result hashes used by a run, but not byte identity
between the canonical skill and the skill selected by the client.

### 5. Completion, reset, and target-change races

PR 34's canonical workflow bounds disappearance handling to one retained
activity query using the already returned `sessionId`, followed by one trace
resolution attempt using the already returned `traceId`. It preserves
`TRACE_UNAVAILABLE` and forbids scanning unrelated inventory
(`loomspan-console/agent-skills/loomspan/references/debugging-playbooks.md:31-38`).

The PR-34 fixture contains both a disappeared execution whose trace resolves
and one whose trace remains unavailable. The evaluation cases require those
outcomes to remain distinct and forbid the claim that disappearance proves
finalization (`loomspan-console/agent-evals/fixtures/pr34-active-execution-review.json:38-49`,
`loomspan-console/agent-evals/cases/pr34-tools-only-active-execution-review.json:1`).

Activity continuations are bound to operation kind, target scope, and exact
session. Wrong kind/session is `INVALID_ARGUMENT`; a prior target scope is
`TARGET_CHANGED` (`loomspan-console/internal/mcpadapter/continuation.go:48-88`).
A live reset changes `intervalId`, clears retained activity and session coverage,
and records a typed reset cause; current continuity reports interval, cursor
range, observation, and reset separately
(`loomspan-console/internal/live/service.go:808-824`,
`loomspan-console/internal/live/coordinator_test.go:760-779`). A checkpoint
whose cursor is no longer retained yields an empty result plus current
continuity/coverage, which lets the caller observe the interval/reset facts
without token introspection (`loomspan-console/internal/live/service.go:709-722`).

The deterministic harness and focused tests exercise the individual facts,
including checkpoint reuse, target binding, interval reset, retained terminal
activity, available trace resolution, and `TRACE_UNAVAILABLE`. No completed
client record currently demonstrates how a supported model handles all of
those interleavings during an actual client run.

## Required Research Questions — Direct Answers

1. **Checkpoint misuse frequency:** Unknown. There are zero committed completed
   PR-34 client result records; automated cases prove mechanics, not model/client
   behavior frequency.
2. **Rename or split benefit:** Not established by current evidence. The present
   name is explicitly described in the tool, schema, text, skill, and cases;
   alternatives have no comparative run data and would change the supported
   pre-v1 MCP contract. Discovery has 12 bytes of current headroom.
3. **List ordering/membership:** Descending stable first-admission ordinal,
   bounded by the first page's high water and each next page's exclusive lower
   ordinal. Later starts are excluded; updates remain at the same ordinal;
   completed/removed sessions may disappear before their page.
4. **Safe page merge:** Identities can be de-duplicated in ordinal traversal
   order and every value tied to its page `observedAt`. The union is not an
   atomic fleet snapshot and cannot support fleet completeness, absence,
   finalization, or co-temporal state claims.
5. **Unanswered ordinary questions:** The current live surface does not state
   the active task's natural-language intent, selection rationale, expected
   outputs, business objective, or intended next task.
6. **Existing purpose facts:** Skill YAML contains application-supplied
   descriptions. `PlanTask` contains model-authored title/intent/outputs and is
   recorded in plan payloads. Live state contains generic producer summaries,
   structural routes, and bounded scalar plan/step identifiers, not plan text.
7. **Current least-disclosing paths:** Existing registered-skill inspection can
   explain skill-level purpose; selected finalized plan content can explain
   task-level purpose after trace resolution. No current live task-purpose path
   exists, and finalized inspection does not answer the question while a trace
   remains active.
8. **Nested/64-execution bound:** The current live structural surface is bounded
   at 64 executions and 64 active-path entries each, but defines no task-purpose
   descriptor or aggregate descriptor budget.
9. **Canonical selection/refresh proof:** Release tests prove packaged bytes
   equal canonical source. Installation/refresh is manual copy or link, and the
   current evaluation record cannot prove the bytes selected from a client's
   cache or skill directory.
10. **Current component ownership:** Java projector/REST owns active producer
    facts; Go live/MCP owns retained activity and diagnostic presentation; the
    canonical Agent Skill owns workflow interpretation; release buildtool owns
    archive byte identity; compatibility docs and agent-eval cases/records own
    client evidence. Browser consumes the shared producer/live facts.
11. **Security boundary:** YAML, summaries, routes, activity details, plan
    content, diagnostics, and raw content are untrusted and are not secret
    scanned. Current text fallbacks omit arbitrary details; plan content needs
    explicit finalized descriptor/content selection; credentials, scope IDs,
    instance IDs, application cursors, owners, and handles remain hidden. Any
    live purpose text would cross the existing default non-disclosure boundary
    for model-authored/application-supplied text.
12. **Documentation/evaluation-only closure:** The ticket permits that outcome,
    but the current checkout does not establish it. Post-PR-34 supported-client
    evaluations have not been recorded, and live task intent remains unavailable;
    therefore current repository evidence cannot yet show that neutral
    primitives are sufficient for the validated developer questions.

## Architecture and Contract Classification

- **Application API:** The supported surface remains the closed allowlist in
  `com.lokiscale.loomspan.api`; this research found no active-inspection type in
  that package and the architecture test passed.
- **Supported SPI:** None exists for active inspection. No active registry,
  projector, observability DTO, or Console adapter is a supported replacement
  point.
- **Configuration and manifest contracts:** Registered skill YAML and the
  canonical six-file unversioned Agent Skill package are deliberate contracts.
  The canonical debugging skill, exact topology validator, packaging tests,
  and documentation are their protected in-repository consumers.
- **Persisted or serialized contracts:** Evaluation record schema v1 is a
  committed repository release-evidence format, not a Loomspan runtime
  interchange format. Release archives are durable named artifacts with whole-
  archive SHA-256 sidecars. No active snapshot/activity format is durable.
- **Ephemeral diagnostic formats:** MCP execution/activity names, descriptions,
  schemas, structured results, text fallbacks, continuations, live coverage,
  active Java REST DTOs, and browser projections describe current-process
  diagnostic evidence. Their protected consumers include Console Go,
  deterministic fixtures/goldens, browser code, Agent Skill guidance,
  evaluation cases, and compatibility documentation.
- **Internal or accidentally exposed implementation:** Java active registry,
  projector, observability controller/DTOs, Go live ring, target scopes,
  application cursors, and evidence-resolution machinery are internal. Several
  Java types are technically public for framework composition and are
  explicitly classified that way by
  `LoomspanPublicSurfaceArchitectureTest`, not as Application API or SPI.

The Java observability REST boundary is an internal exact-release serialized
protocol consumed by Console Go. Observable changes require coordinated Java
producer DTOs/controller, Java tests, generated REST fixtures, Go decoder and
validation, MCP/browser consumers, and exact compatibility reasoning in one
repository change. No current `@ConditionalOnMissingBean` active-inspection
extension point was found.

## Verification Results

- PASS — `go test ./internal/live ./internal/observability ./internal/mcpadapter ./internal/agentskills ./internal/agenteval ./internal/buildtool -count=1`
- PASS — `.\\mvnw.cmd -pl loomspan-spring-boot-starter "-Dtest=InMemoryActiveExecutionRegistryTest,LiveActivityProjectorTest,ObservabilityRestIntegrationTest,LoomspanPublicSurfaceArchitectureTest" test` (33 tests)
- MEASURED — committed `tools-list-response.json`: 25,588 UTF-8 bytes; executable ceiling: 25,600 bytes.
- NOT RUN — supported headless or GUI PR-34 client evaluations; the repository has no completed result records or locally documented executable-client run for this research context.

## Code References

- `loomspan-console/internal/mcpadapter/activity.go:14-84` — checkpoint input,
  description, and resume-cursor construction.
- `loomspan-console/internal/live/service.go:681-824` — retained activity,
  filtering, backlog, coverage, and continuity semantics.
- `loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/observability/web/ObservabilityRestController.java:103-134` — active page high water and fresh observation.
- `loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/runtime/observation/InMemoryActiveExecutionRegistry.java:28-105` — stable ordinals, updates, removal, and newest-first traversal.
- `loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/runtime/observation/LiveActivityProjector.java:21-46` — visible kinds and bounded scalar detail keys.
- `loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/core/PlanTask.java:9-30` — recorded task-purpose fields.
- `loomspan-console/internal/buildtool/package_test.go:40-109` and
  `smoke.go:25-76` — canonical-to-archive byte identity.
- `loomspan-console/internal/agenteval/record.go:47-71` — current evaluation
  identity fields and absence of selected-skill byte identity.

## Historical Context

PR 34 was committed in `ec7a0b0` and its temporary research/plan/testing
artifacts were deliberately removed by cleanup commit `c409731`. The committed
implementation retains the PR-34 fixture, paired evaluation cases, canonical
workflow, production changes, tests, and product documentation. The follow-up
ticket's motivating desktop session remains sanitized historical context and
is not a reproducible result record.

## Related Research

- Git object `ec7a0b0:ai/thoughts/research/2026-08-21-PR-34-active-execution-mcp-inspection.md`
  — pre-implementation PR-34 codebase research; removed from the default
  branch by the documented cleanup commit.
- `ai/thoughts/phases/2026-08-15-loomspan-llm-trace-understanding-roadmap.md`
  — current trace-understanding and pre-v1 client/evaluation roadmap.
- `ai/thoughts/framework-feature-design-lens.md` — canonical contract
  classification and evidence lens used by this research.

## Open Questions

- No completed supported-client run establishes whether capable clients still
  discard or misuse a post-PR-34 checkpoint after `hasMore: false`.
- No current record field proves canonical skill byte identity at client
  selection time or distinguishes a refreshed copy/link from a stale cached
  installation.
- The current live surface has no task-purpose descriptor, so repository
  evidence does not show whether skill-level YAML plus structural facts answer
  the ordinary intent questions identified by the ticket.
- Existing automated fixtures cover component facts, but no completed client
  record covers the full interleaving of multi-page traversal, checkpoint
  reuse, reset/target change, retained terminal activity, successful trace
  resolution, and `TRACE_UNAVAILABLE`.
