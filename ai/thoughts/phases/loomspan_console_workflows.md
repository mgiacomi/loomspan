# Loomspan Console — Developer Workflows

## Status

Implemented-baseline workflow record for the Loomspan Console browser and first
MCP runtime inspector. This document preserves the approved developer workflows
and requirement IDs used by completed implementation and evaluation work. New
LLM-mediated workflow design lives in
[LLM Trace Understanding Workflows](./loomspan_llm_trace_understanding_workflows.md).

## Related designs

- [LLM Trace Understanding Roadmap](./2026-08-15-loomspan-llm-trace-understanding-roadmap.md)
- [LLM Trace Understanding Workflows](./loomspan_llm_trace_understanding_workflows.md)

The new roadmap incorporates the continuing architecture, ownership, contract,
security, resource-bound, and lifecycle constraints from the completed phase
designs. This file remains authoritative only for its implemented workflow
definitions and stable requirement IDs until they are deliberately mapped or
retired.

## Product north star

The console presents recorded facts, deterministic calculations, and explicit evidence relationships. It may sort, filter, group, and navigate those facts. It does not decide what is important, surprising, excessive, justified, causal, or actionable.

## Settled-design guardrail

New workflow design operates within the implemented baseline constraints
recorded by the active roadmap. It must not reopen those decisions merely
because another design appears attractive. A settled decision is reconsidered
only when a concrete workflow cannot be completed safely or usefully within it.

Any proposed reconsideration must:

1. identify the conflicting workflow requirement;
2. cite the existing decision;
3. explain why alternatives within the settled design are insufficient; and
4. receive an explicit decision before changing the phase plans.

Requirements discovered by a workflow should first be satisfied through the existing ownership and service boundaries. Cross-workflow product decisions belong in the applicable phase plan once they are settled. Concrete components, tasks, sequencing, and estimates belong in future implementation plans.

## Initial workflow set

| ID | Workflow | Status |
|---|---|---|
| `WF-FAILED-EXECUTION` | Diagnose a failed completed execution | Approved |
| `WF-SLOW-EXECUTION` | Diagnose a currently slow execution | Approved |
| `WF-EXPENSIVE-EXECUTION` | Explain unexpectedly high usage or cost | Approved |
| `WF-UNFAMILIAR-SKILL-PATH` | Understand an unfamiliar nested skill path | Approved |

The initial workflow set and its cross-workflow consequences were reviewed and
implemented together. It records the resulting browser information
architecture, global navigation, live-execution presentation, and
hierarchy-first trace-explorer expectations. Future LLM workflow changes are
governed by the new roadmap and companion catalog.

### Verification linkage

This document was the canonical investigation catalog used to verify the first
browser, MCP adapter, and portable debugging skill implementation. The new LLM
workflow catalog extends rather than silently redefines these stable historical
IDs.

Representative fixtures, tests, and agent evaluations reference the applicable workflow ID or the most specific requirement ID already defined here. They live beside their owning Java, Go, browser, MCP, or evaluation code and may be linked from this document after project paths exist. This document does not copy fixture payloads, prescribe exact MCP calls or answer prose, or act as a machine-readable test manifest.

Normal, variant, degraded, and exceptional paths remain part of their parent workflow. A separate workflow is added only for a genuinely different developer goal, not merely for another recorded failure kind or protocol edge case. Protocol conformance, transport security, credentials, malformed input, cache behavior, and cancellation remain implementation test coverage rather than additional developer workflows.

## Cross-workflow synthesis

### One connected inspection experience

The four workflows do not create four independent product areas, data models, or analysis engines.

- `WF-SLOW-EXECUTION` primarily uses the live-execution experience and its bounded current snapshot, active path, and recent narrative.
- Completion preserves the selected terminal context and bridges the live experience to finalized-trace inspection when an artifact is available.
- `WF-FAILED-EXECUTION`, `WF-EXPENSIVE-EXECUTION`, and the finalized portion of `WF-UNFAMILIAR-SKILL-PATH` are coordinated entry states or perspectives over the same acquired immutable trace and shared Go calculations.
- Failure, usage, hierarchy, timeline, records, attempts, validation outcomes, and registered skill YAML remain navigably related facts rather than isolated reports.
- Browser and MCP adapt the same transport-neutral services and artifact handle even when their presentation differs.

The Phase 2 information architecture uses Overview as the stable landing page and Overview, Live Executions, Traces, and Skills as its global navigation. Phase 2 also settles the live current-summary, active-path, recent-narrative, list-ordering, following, restrained-motion, continuity-notice, and terminal-transition behavior. Finalized trace inspection is hierarchy-first, with Timeline, Usage, and Records as coordinated alternate views over one shared selected context and no required permanent split view.

### Workflow transitions

| From | Observed transition | Result |
|---|---|---|
| Slow active execution | Execution remains active | Continue updating the current summary and bounded recent narrative without stealing selection. |
| Slow active execution | Execution completes | Preserve terminal context; offer deliberate trace inspection only when the application reports an available finalized artifact. |
| Failed terminal execution | Trace available | Acquire through the centralized artifact service and open the same trace explorer in a failure-focused state. |
| Failed terminal execution | Trace unavailable | Show only the bounded terminal facts and the precise availability limitation; do not construct retrospective detail. |
| Failure facts | Developer navigates to usage or hierarchy | Retain the same target scope, artifact handle, trace, and applicable frame selection. |
| Usage facts | Developer navigates to a frame or skill path | Reveal the mechanically related hierarchy and frame facts without declaring importance or causality. |
| Skill-path facts | Developer navigates to failure, usage, timeline, or records | Use recorded identifiers and shared calculations from the same acquired artifact. |
| Any current-scope workflow | Target scope changes | Reject prior references with `TARGET_CHANGED` and clear prior application-derived state under the settled lifecycle. |

### Shared product requirements

| ID | Requirement | Consequence |
|---|---|---|
| `WF-X-R1` | Apply the product north star to every workflow and presentation. | The console shows facts and deterministic relationships without importance, causality, correctness, or action judgments. |
| `WF-X-R2` | Use one live-execution experience and one finalized-trace inspection experience. | Workflow-specific entry states do not create separate stores, parsers, handles, or contradictory calculations. |
| `WF-X-R3` | Preserve selected context across live updates and execution completion. | Activity does not steal selection or scroll position, and completion does not navigate automatically. |
| `WF-X-R4` | Keep active evidence visibly provisional and finalized-trace evidence explicitly validated. | A bounded active path or recent interval is never presented as a complete execution hierarchy or history. |
| `WF-X-R5` | Keep execution outcome, application trace availability, local artifact availability, target authentication, connection state, and direct gap or lifecycle facts separate. | No aggregate health, lifecycle, or evidence-completeness label substitutes for the underlying facts. |
| `WF-X-R6` | Expose observation time and continuity boundaries wherever live facts are presented. | A retained or refreshed snapshot is not mislabeled as current, continuous, or lossless. |
| `WF-X-R7` | Give browser and MCP the same transport-neutral deterministic calculations and domain-error meanings. | Adapter presentation may differ, but hierarchy, timing, usage, failure, retry, validation, and continuity facts do not. |
| `WF-X-R8` | Navigate related facts through current-scope identifiers and relationships already established by the plans. | The workflow set adds no copying, exporting, evidence packaging, filesystem lookup, or additional retention feature. |
| `WF-X-R9` | Keep raw records and reconstructed payloads as explicitly requested detail. | Opening a workflow does not automatically disclose or load every raw payload, while complete inspectability remains available through existing bounds. |
| `WF-X-R10` | Present missing, expired, invalid, reset, unavailable, unattributed, and unknown evidence explicitly. | The console does not replace missing facts with zero values, guessed relationships, inferred history, or best-effort semantic results. |
| `WF-X-R11` | Make every supported frame and record in a valid acquired artifact addressable while its handle and target scope remain valid. | Settled pagination and response bounds protect operation without a hierarchy-specific depth or node cap or intentional evidence omission. |
| `WF-X-R12` | Preserve acquisition-time authorization semantics across workflows. | Complete acquired evidence may remain locally inspectable after later upstream rejection, while new target access still requires authentication. |

### Cross-workflow review result

The review found no conflict among the four workflows and no requirement that
could not be satisfied through the implemented ownership and lifecycle
boundaries. In particular, the workflow set does not require active-trace
inspection, durable execution history, a second trace representation, provider
health or pricing, console-side repository access, source provenance, automatic
diagnosis, execution control, or broader evidence retention.

The product and architecture decisions used by the completed implementation are
preserved as baseline constraints in the active roadmap. Future contract
changes must cite a new workflow requirement and explicitly reconsider the
affected baseline.

## `WF-FAILED-EXECUTION`: Diagnose a failed completed execution

### Developer goal

Help the developer establish:

1. what failed;
2. where in the skill and frame hierarchy it failed;
3. what happened immediately before it;
4. whether retries, validation, quota, timeout, or guardrail behavior contributed;
5. where time and usage accumulated; and
6. which direct gaps or lifecycle limitations constrain a conclusion.

The console supplies authoritative mechanical facts and evidence relationships. Root-cause judgment, likely developer mistakes, repository comparison, and code-change recommendations remain developer or IDE-LLM reasoning rather than authoritative console calculations.

### Entry paths

The developer may enter this workflow from:

- a live execution that reaches a terminal failure;
- a finalized trace in the selected application's current-process catalog;
- a current-scope browser deep link to an acquired trace, frame, record, or failure; or
- the implemented MCP runtime inspector.

When a finalized artifact is available, these paths converge on the same transport-neutral Go trace analysis and centralized artifact service. Browser and MCP adapters must not create different failure semantics or independent analysis copies.

### Primary live-to-failure flow

1. **Make the terminal result visible.** The live execution receives a terminal activity that preserves execution outcome and `applicationTraceAvailability` as separate facts.
2. **Preserve immediate context.** The execution remains selected when the developer is watching it. The terminal view shows the last safe activity, session and trace identifiers, failed phase when known, elapsed time, and application trace availability. Failure does not automatically navigate away from the live view.
3. **Offer deliberate trace inspection.** When the application reports a trace as available, the terminal view presents a prominent **Inspect trace** action. The console does not acquire every failed trace automatically because acquisition consumes bounded workspace capacity.
4. **Acquire through the shared service.** Go downloads, validates, installs, and issues the current-scope artifact handle through the centralized artifact service used by browser and MCP. Partial or invalid acquisition never becomes inspectable evidence.
5. **Open a failure-focused trace view.** The initial trace explorer state presents the final execution outcome, execution-ending failure, attributed frame and skill path, preceding validation failures and retries, nearby model or tool activity, timing and usage facts, direct gap or lifecycle limitations, and stable evidence identifiers.
6. **Expand through related evidence.** The developer can move from the selected failure into its surrounding timeline, hierarchy, attempts, records, and explicitly requested raw payloads. The UI exposes mechanically related evidence without presenting a speculative cause as an authoritative fact.
7. **Navigate supporting evidence.** The developer can move among the trace, frame, canonical record sequence, failure, and relevant attempt or validation facts through their displayed identifiers. Browser deep links remain navigation conveniences within the current `targetScopeId`, not durable bookmarks.

### Live-page transition behavior

The Live Executions experience includes active executions and a clearly transient **Recent completions** area derived from Go's bounded, single-continuity recent-activity window.

- A watched execution remains selected when it terminates.
- Terminal failure does not trigger automatic navigation.
- An available trace produces the prominent **Inspect trace** action.
- An unavailable trace produces a terminal-summary detail view with a precise explanation rather than a broken or disabled trace action.
- Recent completions are temporary live context, not durable execution history.
- A replay-gap reset may remove earlier recent completions; the UI must preserve the existing explicit continuity-boundary semantics rather than imply lossless history.

### Trace explorer initial state

Failure-focused mode should initially expose:

- final execution outcome;
- the execution-ending failure without labeling it a proven root cause;
- attributed frame, route, and nested skill path;
- preceding validation failures, attempts, and retries;
- temporally and hierarchically related model or tool activity;
- settled inclusive and self duration plus direct and descendant usage;
- direct invalid, unavailable, unattributed, continuation, continuity, lifecycle, and payload-reference facts when applicable; and
- stable identifiers suitable for navigation and citation.

Raw records and reconstructed payloads remain explicitly requested detail. They are not placed automatically into summaries, but every successfully parsed item remains addressable through the existing bounded pagination and payload-range contracts while its artifact handle and target scope remain valid.

### Degraded and exceptional paths

#### Trace not retained

Show the bounded terminal live summary and explain that detailed retrospective evidence is unavailable under the application's trace-retention behavior. Do not present a broken trace link or imply that the console can reconstruct missing detail from the live activity projection.

#### Trace expires before acquisition

Explain that the trace is no longer obtainable from the selected application process. Do not reinterpret absence as proof that the trace or execution never existed. If a complete copy was already acquired by Go, its handle remains usable under its ordinary local lifecycle.

#### Artifact is invalid

Report the shared `INVALID_ARTIFACT` domain error and do not present partial content as a valid analysis. Raw attachment download may remain available when the application still retains the artifact.

#### Target scope changed

Reject the old reference with `TARGET_CHANGED`. A stale deep link resets to the console root and explains that its previous target context is no longer available.

#### Upstream authentication is later rejected

Allow valid handle-based inspection of a complete current-scope artifact already acquired into Go. Require replacement application authentication for a new catalog query, acquisition, or other upstream operation, and retain the evidence's original observation facts.

#### Core finalization failed

Present `EXECUTION_OBSERVATION_ENDED` as an incomplete observation with reason `CORE_FINALIZATION_FAILED`. Do not label the execution itself failed unless its outcome was independently established, and do not invent a finalized trace or diagnostic cause.

#### Live monitoring is unavailable

Do not present active state or live activity as trustworthy. Current-process trace catalog, acquisition, and analysis may remain usable through their independent settled contracts.

#### No retained evidence remains

If the developer was not watching, the trace was not retained, and the bounded recent-activity interval no longer contains the terminal event, the console may have no remaining evidence for the execution. State that lifecycle limitation honestly; do not introduce or imply a durable execution-history store.

### Surfaced product requirements

| ID | Requirement | Existing design boundary |
|---|---|---|
| `WF-FE-R1` | Preserve selection and immediate terminal context when a watched execution ends. | Browser presentation state over Phase 1 activity and Go relay. |
| `WF-FE-R2` | Present active executions and bounded recent completions as visibly different lifecycle categories. | Browser UI over Go's bounded recent-activity window; no durable history. |
| `WF-FE-R3` | Keep execution outcome and application trace availability visibly separate. | Phase 1 terminal activity contract. |
| `WF-FE-R4` | Require deliberate artifact acquisition through **Inspect trace** rather than acquiring every failed trace automatically. | Centralized Go artifact service and bounded workspace. |
| `WF-FE-R5` | Support a failure-focused trace-explorer entry state using shared mechanical calculations. | Transport-neutral Go trace services shared by browser and MCP. |
| `WF-FE-R6` | Distinguish execution-ending failure from inferred root cause. | Go supplies facts; developer or LLM supplies contextual reasoning. |
| `WF-FE-R7` | Expose evidence identifiers suitable for navigation and citation. | Current-scope trace, frame, record, failure, attempt, and validation identifiers. |
| `WF-FE-R8` | Explain unavailable, expired, invalid, incomplete, and stale-scope evidence distinctly. | Existing shared domain errors and Phase 1 lifecycle facts. |
| `WF-FE-R9` | Preserve acquired evidence under acquisition-time authorization without presenting it as current target state. | `TargetContext`, artifact-handle lifecycle, and shared status facts. |
| `WF-FE-R10` | Never turn recent activity into an implied durable or lossless failure history. | Bounded single-continuity activity window and explicit replay-gap behavior. |

### Implementation choices

This workflow establishes the required failure-focused behavior but does not yet choose:

- exact typography, animation, component library, or frontend framework;
- exact recent-activity and interoperable one-response framing values.

Those choices were implementation work that consumed this workflow as a
requirement. Shared duration calculations and flat failure and validation
indexes are part of the implemented runtime-inspector baseline; the product
intentionally has no aggregate evidence-completeness model.

The implemented general trace, frame, record, payload, execution, activity, and
skill operations were validated against this workflow without creating a
scenario-specific failure tool. Their future evolution is governed by the
active roadmap.

## `WF-SLOW-EXECUTION`: Diagnose a currently slow execution

### Developer goal

Help the developer establish:

1. what phase and nested skill path are active now;
2. what the most recent observable activity was;
3. whether the execution continues to make observable progress;
4. whether model, tool, or validation activity appears repeatedly in the retained interval;
5. how much time and usage have accumulated relative to configured limits; and
6. what evidence remains unavailable until the execution completes.

“Slow” remains the developer's characterization. The console has no historical baseline, provider-health probe, or authoritative threshold that proves an execution is slow, unhealthy, or stuck.

### Entry paths

The developer may enter this workflow from:

- the active-execution list, commonly after noticing high elapsed time;
- a live activity item;
- direct current-scope navigation to an active `sessionId`; or
- the implemented MCP runtime inspector.

All entry paths use the Phase 1 active snapshot and Go's bounded, single-continuity recent-activity service. They do not read or tail an active trace file.

### Primary flow

1. **Identify the execution.** The active list shows entry skill, elapsed time, current phase, active skill path, latest activity, usage, and execution status. Sorting or filtering by elapsed time aids discovery but does not classify an execution as unhealthy.
2. **Open live execution detail.** The initial view presents session and trace identifiers, entry skill and start time, elapsed time, current phase, bounded active skill and frame path, latest concise activity, model/tool/skill invocation counts, current usage and configured limits, observation time, and live continuity state.
3. **Follow current progress.** The detail view applies ordered activity as it arrives. Current phase, active path, counts, usage, elapsed time, and latest activity update without replacing the developer's selected context.
4. **Inspect recent activity.** A bounded narrative shows retained planning, nested-skill, model, tool, validation, retry, quota, timeout, and guardrail activity. It identifies clearly when the retained interval does not include the beginning of the execution.
5. **Inspect current and repeated activity.** The developer can see mechanically that the execution is currently in, or has recently repeated, a particular phase, skill, model call, tool call, or validation attempt. The console may show elapsed intervals and activity timestamps, but it does not infer an external provider outage, deadlock, or root cause from silence alone.
6. **Continue observing or leave.** The developer may keep following live activity without preventing navigation elsewhere. Returning to the execution restores its latest current state rather than attempting to replay activity that has left the bounded window.
7. **Transition on completion.** If the selected execution completes, the console preserves the terminal context without automatic navigation. It offers **Inspect trace** when a finalized trace is available and otherwise shows the bounded terminal summary under the approved live-to-completed behavior.

### Live-detail presentation

The live execution detail has three synchronized information areas:

- **Sticky current summary:** phase, active skill path, elapsed time, usage and limits, invocation counts, latest activity, and observation freshness.
- **Active-path view:** the bounded currently active nested path, without presenting it as a complete execution tree.
- **Recent narrative:** the bounded ordered activity interval with timestamps and retry or validation transitions.

The recent narrative follows the newest activity by default. If the developer scrolls backward or selects an earlier item, automatic following pauses and a clear **Resume live** action appears. Incoming activity continues updating the current summary without stealing selection or scroll position.

Updates emphasize meaningful phase, path, retry, failure, and completion changes. Elapsed time and counters update quietly without animating every increment.

### Interpretation rules

- High elapsed time is a fact; “slow” is an interpretation.
- No recent activity does not prove that the execution is stuck.
- The active path is not a completed frame tree.
- The recent-activity window is not complete execution history.
- A live snapshot is best-effort and carries its observation time.
- Intermediate activity may be missing after a replay gap.
- Detailed hierarchy, complete duration attribution, self time, and complete usage attribution require a finalized trace.
- The console does not expose or claim hidden model reasoning.

### Degraded and transition paths

#### Recent window starts during the execution

Show the earliest retained cursor or time and state that earlier activity is unavailable. Do not infer how the execution reached that point.

#### One browser tab has a local replay gap

Refresh that tab's current baseline and explain that it missed some live activity. Do not reset the shared upstream interval or unrelated consumers.

#### The upstream activity interval resets

Clear the shared recent-activity window, establish the new continuity interval, and never combine events from opposite sides of the reset. Present the existing reset-boundary facts to browser and MCP consistently.

#### The target disconnects temporarily

Preserve complete current-scope evidence with its original observation time, mark the displayed live state as not current, and reconnect through the existing `TargetContext` lifecycle. Do not relabel the retained snapshot as current.

#### Application authentication is rejected

Block operations requiring new current target state. Do not present the previous active snapshot as current or purge otherwise valid complete current-scope evidence solely because later upstream authentication failed.

#### Live monitoring is unavailable

Do not present active-execution state or live activity as trustworthy. Current-process trace catalog and valid already-acquired artifact operations may remain independently usable.

#### The execution disappears during baseline traversal

Reconcile through terminal activity when still available or a later periodic refresh. Do not invent the missing transition or imply an atomic snapshot boundary.

#### The target scope changes

Cancel the prior inspection, discard its application-derived live state, and return `TARGET_CHANGED` for prior-scope references.

#### The execution completes

Preserve the selected terminal context and use the approved live-to-completed transition. A failed execution follows `WF-FAILED-EXECUTION`; any available finalized trace is inspected only after deliberate acquisition.

#### Core finalization fails

Present `EXECUTION_OBSERVATION_ENDED` with `CORE_FINALIZATION_FAILED` without inventing an execution outcome, canonical terminal record, or available trace.

### Surfaced product requirements

| ID | Requirement | Existing design boundary |
|---|---|---|
| `WF-SE-R1` | Allow active executions to be sorted or filtered by elapsed time without creating a “slow” health classification. | Browser collection presentation over Phase 1 active summaries. |
| `WF-SE-R2` | Separate the current bounded summary, active path, and recent narrative in live detail. | Browser presentation over active snapshots and Go recent activity. |
| `WF-SE-R3` | Expose observation freshness and continuity state in every live detail view. | Snapshot `observedAt`, cursors, and shared reset-boundary facts. |
| `WF-SE-R4` | Pause automatic narrative following during manual inspection without stopping live ingestion. | Browser-owned selection and scroll state over bounded relay delivery. |
| `WF-SE-R5` | Ensure incoming activity does not steal selection or scroll position. | Browser presentation state. |
| `WF-SE-R6` | Distinguish the bounded active path from a complete trace hierarchy. | Phase 1 active snapshot versus finalized Go trace analysis. |
| `WF-SE-R7` | Never present elapsed time or absent recent activity as proof that an execution is stuck. | Evidence and uncertainty contract; no health model or provider probe. |
| `WF-SE-R8` | Transition a selected execution in place when it completes. | Shared live-to-completed behavior and terminal activity contract. |
| `WF-SE-R9` | Give browser and MCP the same active snapshot and recent-activity continuity semantics. | Transport-neutral Go runtime and recent-activity services. |
| `WF-SE-R10` | Reserve complete hierarchy, duration, and usage attribution for finalized-trace analysis. | Finalized immutable artifact and shared Go trace services. |

### Implementation choices

This workflow does not yet choose:

- the exact responsive arrangement of the three live-detail areas;
- precise activity animation, refresh intervals, or recent-window limits;
- the frontend framework, component library, or visualization libraries;
- the final transport DTOs for observation freshness and continuity notices.

Those choices remain implementation-level UI and DTO work. They should satisfy this workflow without adding active-trace tailing, historical baselines, provider probes, or an execution-health classification.

The implemented surface exposes active summaries and recent activity through
`LOOMSPAN_list_executions`, `LOOMSPAN_get_execution`, and
`LOOMSPAN_get_execution_activity`. Additive schema evolution must preserve
those operation boundaries unless an active workflow explicitly reconsiders
them.

## `WF-EXPENSIVE-EXECUTION`: Explain unexpectedly high usage or cost

### Developer goal

Make the recorded usage and its execution attribution visible so the developer can establish:

1. what total usage was recorded;
2. how that total compares arithmetically with configured execution limits;
3. how usage is distributed across skills, frames, model interactions, and attempts;
4. how much usage is direct versus accumulated through descendants;
5. what usage is associated with recorded retries and validation attempts; and
6. which direct missing-response or attribution facts constrain those calculations.

“Unexpectedly expensive” remains the developer's characterization. The console does not classify usage as important, surprising, excessive, wasteful, or justified, and the initial product has no historical baseline or expected-cost model.

### Usage and monetary cost boundary

The settled plans establish usage attribution but do not establish authoritative provider pricing or billing calculations. The initial workflow therefore:

- reports usage in the units recorded by loomspan;
- compares recorded usage with configured limits when available;
- exposes the execution paths and attempts to which usage is mechanically attributed;
- displays a monetary value only if that value was already recorded canonically with defined semantics; and
- otherwise states that monetary cost is not calculated.

The console does not multiply usage by embedded or current provider price tables. Prices may vary by provider, model version, time, contract, caching behavior, and billing policy. Console-calculated currency requires a future explicit contract if it becomes a real product requirement.

### Entry paths

The developer may enter this workflow from:

- a completed execution's terminal summary;
- a finalized trace in the selected application's current-process catalog;
- a failure-focused trace investigation;
- a current-scope deep link to a trace or frame; or
- the implemented MCP runtime inspector.

A currently active execution may expose provisional accumulated usage through its bounded snapshot. Complete attribution requires a finalized, successfully validated trace.

### Primary flow

1. **Select the execution.** The terminal or trace summary shows recorded total usage and configured limits when available. It does not label the execution expensive or excessive.
2. **Acquire the finalized trace.** The developer deliberately opens the trace through the centralized artifact service. Browser and MCP calculations use the same immutable acquired copy.
3. **Open the usage view.** The view presents total usage by recorded unit, configured limits and arithmetic proportion consumed, direct and descendant usage by skill or frame path, model interactions sorted by attributed usage, usage by attempt, recorded retry and validation relationships, duration as a separate fact, and direct missing-response or unattributed-usage facts.
4. **Inspect usage attribution.** The developer can sort or navigate skill paths, frames, model interactions, and attempts by mechanically attributed usage. Selecting an item reveals its position in the execution hierarchy and timeline without labeling it important or causal.
5. **Inspect attempts, retries, and validation sequences.** The view presents recorded relationships and exact usage amounts for the applicable attempts. For example, it may show three model attempts, the usage attributed to each, and that two preceded recorded validation failures. It does not decide whether the attempts were necessary or the validation behavior was correct.
6. **Inspect supporting evidence.** The developer can navigate related records, concise summaries, validation outcomes, model metadata, and explicitly requested payloads. Raw prompts and responses are not included automatically merely because the usage view was opened.
7. **Navigate supporting evidence.** The developer can move among exact recorded facts through their displayed trace, frame, record, attempt, and validation identifiers. The console does not produce a causal explanation or recommendation.

### Usage presentation

The trace explorer provides three coordinated information layers:

- **Usage summary:** recorded totals, configured limits, recorded units, arithmetic proportions, and direct missing-response or unattributed-usage facts.
- **Attribution view:** hierarchical direct and descendant usage by skill or frame path.
- **Contributor detail:** the recorded model interactions, attempts, retries, validation outcomes, timing, and evidence identifiers associated with the selected item.

A usage-sorted view may complement the hierarchy, but it preserves the underlying path. Flattened presentation must not make descendant usage appear to be direct usage or allow overlapping inclusive totals to be added together incorrectly.

### Attribution definitions

The shared Go calculations use these exact mechanical definitions:

- **Direct usage:** usage attributed to activity owned by the selected frame, excluding descendants.
- **Descendant usage:** usage attributed to frames below the selected frame.
- **Inclusive usage:** direct plus descendant usage for the selected subtree.
- **Attempt usage:** normalized usage carried by a model response with the selected `attemptId`; a validation outcome may relate to that attempt but does not itself create model usage.
- **Retry usage:** the sum of attempt usage whose records carry the selected `retrySequenceId`, without adding overlapping frame-inclusive totals.
- **Unattributed usage:** recorded usage that cannot be assigned safely to a more specific supported hierarchy.

Direct attribution follows the model response's recorded `frameId`; ancestor traversal supplies descendant and inclusive values. Attempt and retry membership follows only the recorded `attemptId` and `retrySequenceId`. Record adjacency, matching messages, equal attempt numbers, and timestamp proximity are not relationships. The calculations prevent double counting and expose unattributed usage rather than silently discarding it. The terminal recorded usage snapshot is the reconciliation total, and browser and MCP consume the same shared Go arithmetic.

### Interpretation rules

- High usage is a recorded fact; “too expensive” is an interpretation.
- Duration and usage are separate evidence dimensions.
- Inclusive values from overlapping parent and child paths must not be summed.
- Usage associated with a retry does not prove that the retry was unnecessary.
- Usage associated with validation does not prove that validation was misconfigured.
- Missing usage is unknown, not zero.
- Current active-execution usage is provisional.
- Complete attribution requires a finalized valid trace.
- Currency cost is unavailable unless it was canonically recorded under defined semantics.
- Sorting by attributed usage does not declare the first item important, causal, excessive, or actionable.

### Degraded and exceptional paths

#### The execution is still active

Show current accumulated usage and configured limits as provisional. Defer complete hierarchy and attempt attribution until a finalized trace becomes available.

#### The trace is not retained or expires

Show only the bounded terminal or active usage facts still available and explain that detailed attribution cannot be reconstructed.

#### The artifact is invalid

Return `INVALID_ARTIFACT`. Do not calculate totals from a partially trusted trace or present partial content as valid analysis.

#### Some recorded usage cannot be attributed

Include it explicitly as unattributed usage with its recorded unit. Do not silently drop it or force it into a frame relationship that the evidence does not support.

#### A required usage unit or consumed semantic value is unsupported

Reject the artifact when correct interpretation is required for a shared calculation. Opaque unconsumed metadata remains available for raw inspection under the existing trace agreement.

#### A configured limit is absent

Show the recorded total without inventing a percentage, expected value, or limit.

#### No monetary value is recorded

State that Loomspan recorded usage but the console does not calculate currency cost.

#### The artifact handle expires during inspection

Return `ARTIFACT_EXPIRED`. Reacquisition remains subject to current application authentication and catalog availability.

#### The target scope changes

Return `TARGET_CHANGED` and never apply prior-scope attribution or references to the new runtime context.

#### Upstream authentication is later rejected

Continue valid handle-based local analysis while reporting current target authentication as the separate fact it is. Require authentication for any new upstream acquisition.

### Surfaced product requirements

| ID | Requirement | Existing design boundary |
|---|---|---|
| `WF-UE-R1` | Base complete usage attribution on a finalized, validated artifact. | Centralized Go artifact and trace-analysis services. |
| `WF-UE-R2` | Mark active-execution usage as provisional. | Phase 1 bounded active snapshot versus finalized trace. |
| `WF-UE-R3` | Give browser and MCP identical direct, descendant, inclusive, attempt, retry, and unattributed-usage definitions. | Transport-neutral Go shared calculations. |
| `WF-UE-R4` | Preserve recorded usage units in totals and attribution. | Current-release Java/Go trace agreement. |
| `WF-UE-R5` | Present configured limits separately from observed totals and calculate only supported arithmetic proportions. | Active and trace usage facts. |
| `WF-UE-R6` | Prevent double counting across overlapping hierarchy levels. | Shared Go attribution definitions and fixtures. |
| `WF-UE-R7` | Expose unattributed usage rather than dropping or guessing it. | Evidence and uncertainty contract. |
| `WF-UE-R8` | Keep hierarchical attribution and usage-sorted presentation navigably connected. | Browser trace-explorer presentation over shared results. |
| `WF-UE-R9` | Present retry and validation relationships without judging necessity, correctness, or causality. | Recorded trace relationships and product north star. |
| `WF-UE-R10` | Keep duration separate from usage. | Shared Go timing and usage calculations. |
| `WF-UE-R11` | Represent missing usage as unknown rather than zero. | Direct unavailable-response and unattributed-usage facts. |
| `WF-UE-R12` | Do not calculate monetary cost without a future authoritative pricing contract. | Settled current-release scope and canonical trace facts. |
| `WF-UE-R13` | Do not generate causal explanations, importance judgments, or recommendations from usage facts. | Product north star; reasoning remains with the developer or IDE LLM. |

### Implementation choices

This workflow does not yet choose:

- the visual form of hierarchical and usage-sorted attribution;
- response and search limits;
- the frontend visualization library.

Those choices remain implementation-level presentation and framing work. They must follow the product north star and must not introduce historical baselines, provider pricing, billing estimates, automatic importance rankings, causal diagnoses, or change recommendations.

Direct, descendant, inclusive, attempt, retry, and unattributed usage and their
recorded model-usage inputs are settled baseline semantics. The implemented
trace, frame-query, record-query, and payload-read operations expose those
facts and evidence references; additive schemas and interoperable response
framing must preserve their meanings.

## `WF-UNFAMILIAR-SKILL-PATH`: Understand an unfamiliar nested skill path

### Developer goal

Make the recorded execution hierarchy, nested skill path, and associated registered skill YAML visible so the developer can establish:

1. which skill was the execution entry point;
2. which nested skills and frames were entered;
3. their recorded parent-child relationships and canonical order;
4. the route and identifiers recorded for each frame;
5. the activity, duration, usage, and outcome facts belonging to each frame;
6. the registered YAML corresponding to a named skill; and
7. where the available evidence is incomplete or provisional.

The console shows the path Loomspan recorded. It does not decide whether that path was expected, appropriate, efficient, or correctly designed.

### Entry paths

The developer may enter this workflow from:

- an active execution's current skill path;
- a finalized trace's hierarchy;
- a selected frame, record, failure, or usage attribution;
- a current-scope deep link; or
- the implemented MCP runtime inspector.

An active execution exposes only its bounded current path and recent activity. A complete hierarchy requires a finalized, successfully validated trace.

### Primary flow

1. **Select an execution or frame.** The developer begins with an execution, selected nested frame, or evidence link from another workflow.
2. **Show the recorded hierarchy.** The trace view displays the deterministic hierarchy calculated from frame and parent-frame identifiers. It preserves canonical ordering and distinguishes skills, plans, model interactions, tools, validations, and other supported frame types.
3. **Orient the selected frame.** The selected frame shows a root-to-selection breadcrumb containing recorded skill names, frame types, routes, and identifiers. Repeated or recursive invocations remain distinct because their frame IDs are distinct.
4. **Inspect frame facts.** The selected frame exposes its frame and parent-frame identifiers, frame type, route, registered skill name when recorded, canonical record range, timestamps, supported duration and usage calculations, attempts, retries, validation outcomes, failure locations, final frame outcome when established, and applicable direct gap or lifecycle facts.
5. **Inspect the registered skill definition.** When the frame identifies a registered skill, the developer can open the application-provided skill entry by registered name. The console displays the unique registered skill name, normalized skills-root-relative `sourcePath`, and unchanged UTF-8 YAML content. It may display or syntax-highlight the YAML as text but does not construct an effective-definition model or resolve defaults.
6. **Navigate related runtime facts.** The developer can move among the selected frame, its parent, children, related timeline interval, records, failures, attempts, validation outcomes, and usage attribution. Every relationship is supported by recorded identifiers or deterministic shared calculations.

The workflow adds no copying, exporting, evidence packaging, or retention behavior. Identifiers are displayed where they identify runtime facts and support the current-scope navigation already established by Phase 2.

### Coordinated information

The workflow requires three coordinated information layers:

- **Execution hierarchy:** the complete finalized frame tree or the bounded active path.
- **Selected-frame detail:** recorded and mechanically calculated facts for one invocation.
- **Registered skill YAML:** the unchanged application-provided definition when available.

Selection in one layer updates the related facts in the others without changing the underlying evidence. The settled trace explorer is hierarchy-first, with Timeline, Usage, and Records as coordinated alternate views that preserve the closest applicable selection.

The hierarchy clearly distinguishes:

- a skill definition from a runtime skill invocation;
- multiple invocations of the same registered skill;
- direct children from deeper descendants;
- frame hierarchy from canonical time ordering; and
- an active bounded path from a complete finalized hierarchy.

### Runtime-to-workspace boundary

The browser console displays the registered `sourcePath` and YAML returned by the running application. It does not:

- join `sourcePath` to a local directory;
- search the developer's repository;
- claim that a workspace file is the deployed source;
- infer a Java implementation from a name; or
- establish build or Git provenance.

The Agent Skill may guide the IDE LLM to compare application-provided YAML with
candidate YAML or Java code in the workspace. A mapping identifier is only a
search hint. A difference is development context or a possible change target,
not proof of which source revision produced the running application.

### Interpretation rules

- The recorded hierarchy shows what executed, not why it was chosen.
- A route is a recorded fact, not automatically an explanation of routing intent.
- A parent-child relationship does not by itself establish semantic causality.
- Repeated invocation does not imply recursion unless the recorded hierarchy establishes it.
- Two invocations of the same skill remain different runtime frames.
- The registered YAML is the application-provided skill representation, not an effective-definition DTO.
- `sourcePath` is descriptive metadata, not a filesystem locator or deployment attestation.
- The active path is provisional and incomplete.
- The finalized trace hierarchy is complete only when the artifact passes shared validation.
- The console does not judge whether the skill composition is correct or recommend restructuring it.

### Degraded and exceptional paths

#### The execution is active

Show only the bounded current active path and recent activity. Do not manufacture inactive siblings, completed branches, or a full frame hierarchy.

#### The trace is unavailable or expires

Retain only available terminal, active-path, or recent-activity facts and explain that the complete hierarchy cannot be reconstructed.

#### The artifact is invalid

Return `INVALID_ARTIFACT`. Do not present a best-effort partial hierarchy as valid analysis.

#### Registered skill YAML is unavailable

Continue showing trace facts while marking the registered definition unavailable. Do not substitute a guessed local file.

#### Current application authentication is rejected

Continue valid acquired-trace inspection, but require authentication for a new skill-catalog request and keep current target authentication visible as a separate fact.

#### The target scope changes

Return `TARGET_CHANGED`. Never resolve old frame or skill references in the new runtime context.

#### The application instance changes

Clear prior-instance skill and trace state through the existing `TargetContext` scope rotation.

#### The hierarchy is large or deep

Present the complete validated hierarchy without a product-level depth or node cap. The existing bounded transport may deliver it incrementally, but no frame is intentionally omitted. Specialized virtualization or very-deep-tree behavior is deferred until demonstrated necessary.

#### A skill is invoked multiple times

Display every invocation as its own frame even when names and registered definitions are identical.

#### A mapping identifier has no workspace match

The IDE investigation reports no candidate match rather than treating the runtime evidence as invalid.

#### Workspace content differs from application YAML

The IDE investigation presents the difference as comparison context, not deployment provenance or proof of a defect.

### Surfaced product requirements

| ID | Requirement | Existing design boundary |
|---|---|---|
| `WF-SP-R1` | Expose the deterministic frame hierarchy for a finalized valid trace. | Shared Go trace analysis. |
| `WF-SP-R2` | Expose only the bounded current path for an active execution. | Phase 1 active snapshot and recent activity. |
| `WF-SP-R3` | Retain a distinct frame identity for every runtime invocation. | Current Java trace frame and parent-frame identifiers. |
| `WF-SP-R4` | Build root-to-selection orientation from recorded identifiers and routes. | Shared Go hierarchy calculation. |
| `WF-SP-R5` | Keep hierarchy, canonical time ordering, duration, usage, and outcome as separate facts. | Product north star and transport-neutral trace services. |
| `WF-SP-R6` | Connect a selected frame to its related timeline, records, failures, attempts, validations, and usage facts. | Recorded identifiers and deterministic shared calculations. |
| `WF-SP-R7` | Link a recorded registered skill name to the application-provided YAML when available. | Phase 1 registered-skill catalog. |
| `WF-SP-R8` | Display skill YAML unchanged without constructing an effective-definition model. | Settled skill representation contract. |
| `WF-SP-R9` | Never use `sourcePath` as a browser or Go filesystem locator. | Settled descriptive-metadata contract. |
| `WF-SP-R10` | Give browser and MCP identical hierarchy and frame calculations. | Transport-neutral Go shared services. |
| `WF-SP-R11` | Make every frame in a valid acquired trace available without a hierarchy-specific depth or node cap. | Complete inspectability through bounded transport while the artifact handle remains valid. |
| `WF-SP-R12` | Keep runtime-to-workspace comparison in the IDE and avoid deployment-provenance claims. | Agent Skill and runtime-to-workspace boundary. |
| `WF-SP-R13` | Do not add copy, export, evidence-package, or additional retention behavior through this workflow. | Existing current-scope navigation and artifact lifecycle. |
| `WF-SP-R14` | Do not explain routing intent, semantic causality, design correctness, or recommended restructuring. | Product north star. |

### Implementation choices

This workflow does not yet choose:

- the exact visual treatment of frame types, routes, repeated invocations, and breadcrumbs;
- specialized rendering behavior for very deep hierarchies if a demonstrated need emerges;
- the frontend framework or component strategy.

Duration, flat failure and validation indexes, usage, attempt/retry membership,
final outcome, terminal-failure attribution, and the absence of an aggregate
evidence-completeness model are implemented baseline semantics. Future choices
must follow the product north star and must not add a parsed effective skill
model, active-trace inspection, console-side repository access, source mapping,
deployment attestation, hierarchy-specific truncation, or console-generated
judgments without explicit reconsideration.

The implemented trace, frame-query, record-query, and skill-inspection
operations expose the hierarchy, frame, and skill facts this workflow consumes.
Their schemas may evolve additively; workspace comparison remains client-owned.
