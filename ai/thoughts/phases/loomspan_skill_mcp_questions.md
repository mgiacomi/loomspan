# Loomspan Skill/MCP Developer Question Inventory

## Status

Working question-first design document, last updated 2026-08-18. This document
is intentionally incomplete and will be expanded one investigation area at a
time through human-developer and LLM collaboration.

The first area is how a developer asks the LLM to find, load, select, or reuse a
trace. No MCP or skill change is selected merely because a question appears
here.

Accepted first-pass interface cleanup is handed off through the
[LLM-facing MCP trace interface cleanup ticket](../tickets/loomspan-mcp-llm-facing-trace-interface-cleanup.md).
After implementation, live MCP walkthroughs should supply evidence for the
remaining candidates rather than expanding that ticket speculatively.

## Relationship to the roadmap and workflow catalog

This document expands **Optimize for developer questions** in the
[LLM Trace Understanding Roadmap](./2026-08-15-loomspan-llm-trace-understanding-roadmap.md).
It inventories likely human-to-LLM communication before the roadmap commits to
tool, schema, projection, or skill changes.

The [LLM Trace Understanding Workflows](./loomspan_llm_trace_understanding_workflows.md)
remain the curated workflow and evaluation contract. Questions explored here
may later be grouped into a durable workflow, converted into a requirement or
fixture, deferred, or rejected as a product responsibility.

## CRITICAL — LLM interface burden of proof

Every LLM-visible method, query parameter, and return field must remain clean,
meaningful, and as simple as possible. Apply these tests before accepting an
interface element.

### Method test

For every tool:

1. What developer intent does this represent?
2. Why can’t an existing composable tool handle it?
3. Does it expose a domain operation or an implementation step?
4. Would an LLM naturally know when to call it?
5. Does adding it reduce total interface complexity?

### Parameter test

A parameter should change caller intent or materially constrain evidence.

Ordinary behavior should be the default. The LLM should not need to supply
parameters merely to restate normal operation.

### Return-field test

A field should affect the LLM’s next decision or final answer.

## Interface change ledger

This ledger prevents accepted interface work from being lost while the question
inventory continues to evolve. It is not a replacement for implementation
tickets. A ticket should be created only after the intended behavior and its
acceptance criteria are understood well enough for work in another context.

Status meanings:

- **CANDIDATE** — an observed interface concern that still requires design work;
- **ACCEPTED** — the direction is agreed, although details may remain open;
- **TICKETED** — an implementation-ready ticket exists;
- **IMPLEMENTED** — the change is present but has not completed workflow/model
  evaluation;
- **VERIFIED** — implementation and representative LLM workflows demonstrate
  the intended result;
- **DEFERRED** — valid work intentionally postponed with a recorded reason; and
- **REJECTED** — considered and deliberately not pursued with a recorded reason.

| ID | Area | Interface element | Decision or question | Rationale | Status | Follow-up |
| --- | --- | --- | --- | --- | --- | --- |
| TRACE-IF-001 | Trace discovery | `LOOMSPAN_list_traces.sourceFilter` | Remove `sourceFilter` from the LLM-facing MCP contract. Ordinary trace discovery should use one unified inventory without requiring the LLM to choose a storage or provenance category. Console may retain internal source distinctions, but should express any material evidence limitation directly instead of exposing a routing enum. | `TARGET` versus `IMPORTED` describes where evidence came from, not the developer’s ordinary intent. Requiring this choice makes the LLM restate an implementation detail and creates avoidable branching. | **TICKETED** | Implement and verify through the linked first-pass cleanup ticket. |
| TRACE-IF-002 | Trace identity | LLM-visible `artifactHandle` parameters and return fields | Remove `artifactHandle` from the LLM-facing MCP contract. Use `traceId` as the normal identity for trace discovery and every downstream trace operation. Console may retain artifact handles internally to bind an exact installed evidence instance. If distinct evidence instances claim the same `traceId`, return an explicit exceptional disambiguation response rather than making every caller manage artifact identity. | `artifactHandle` represents Console acquisition, cache, expiry, and ownership mechanics rather than developer intent. Making the LLM transition from `traceId` to a temporary handle adds state and failure modes to every normal investigation. | **TICKETED** | Implement the internal resolver, collision behavior, target-change safety, imported evidence path, and evaluations in the linked ticket. |
| TRACE-IF-003 | Trace discovery | Trace-inventory catalog, availability, cache, acquisition, retention, and size fields | Remove `applicationCatalog`, `applicationAvailability`, `localAvailable`, `acquiredAt`, `lastUsedAt`, `localExpiresAt`, `localBytes`, `persistencePolicy`, `applicationTraceExpiresAt`, and `sizeBytes` from the LLM-facing trace inventory. Replace a failed or incomplete discovery source with a compact domain-level completeness limitation only when it changes what the LLM may conclude. | These fields describe how Console merges, acquires, retains, and stores evidence rather than which trace answers the developer’s question. Console should manage those mechanics and report only a material inability to discover or inspect evidence. | **TICKETED** | Implement the compact inventory result and incomplete-discovery behavior in the linked ticket. |
| TRACE-IF-004 | Trace operations | LLM-visible `source` parameters, fields, and `TARGET`/`IMPORTED` routing distinctions | Remove `source` from the LLM-facing MCP trace contract. Console should resolve evidence internally from `traceId`; the LLM should not select or branch on storage origin. If origin creates a material limitation, return the limitation in domain terms rather than requiring or returning an implementation-facing source enum. | Source is an acquisition, storage, and ownership concern rather than developer intent. Keeping it visible would preserve unnecessary branching after `sourceFilter` and `artifactHandle` are removed. | **TICKETED** | Implement deterministic resolution and domain-level limitations alongside TRACE-IF-001 and TRACE-IF-002. |
| TRACE-IF-005 | Cross-cutting identity | LLM-visible `targetScopeId` and `instanceId` return and error fields | Remove `targetScopeId` and `instanceId` from the LLM-facing MCP contract. Console must continue to enforce target ownership, target-change, runtime-generation, and stale-evidence safety internally, returning a direct domain error or limitation when the LLM must restart or cannot proceed. | These identifiers exist so Console can prevent evidence from being mixed across targets and runtime generations. The LLM should not retain, compare, or route with them to receive those guarantees. | **TICKETED** | Implement MCP-specific runtime, continuity, result, and error DTOs plus leak tests in the linked ticket. |
| TRACE-IF-006 | MCP navigation | `resourceUri`, `resources`, and current custom MCP resource templates | Remove `resourceUri` and `resources` from LLM-facing tool results. Remove the current custom resource templates because they duplicate complete tool paths and expose rejected scope/source/handle identities. A future resource surface must independently pass the method test. | These fields and templates duplicate callable inspection tools, increase branching, and currently expose `targetScopeId`, `source`, and `artifactHandle` through URI construction. | **TICKETED** | Remove result fields and current resource registrations in the linked ticket; reconsider resources only from observed post-cleanup need. |
| TRACE-IF-007 | Execution-to-trace identity | `sessionId` in discovery and downstream operations | Determine whether `sessionId` should remain only a developer-facing discovery/correlation key that resolves to `traceId`, after which live and finalized inspection use `traceId`. Do not remove it as Console noise: the supported Java `SkillExecutionView` exposes it to application developers and it identifies a skill session even though trace evidence has a separate identity. | `sessionId` represents a real developer entry point, but repeating or routing with both `sessionId` and `traceId` throughout an investigation may create an avoidable identity transition. | **CANDIDATE** | Walk through copied-session, active execution, finalization failure, and completed-trace conversations after the cleanup ticket; determine whether one session can map to multiple traces. |

Expected flow:

`question inventory → accepted ledger item → roadmap dependency → ticket → implementation → workflow/model evaluation → VERIFIED`

Keep this ledger in this document while it remains manageable. Extract it into a
separate work-tracking document only if its size begins to obscure the developer
question analysis.

## How to use this document

For each investigation area:

1. collect natural developer wording, including imprecise and conversational
   wording;
2. identify what evidence or conversational context the developer has supplied;
3. separate what the LLM may safely infer from what it must verify;
4. describe the minimum useful behavior and material degraded cases;
5. record open product questions without prematurely choosing an interface; and
6. derive interface pressures only after the question inventory is broad enough.

The examples are not commands and do not define exact answer prose. A canonical
path will eventually provide a reliable baseline, but the LLM remains free to
use another safe, bounded, evidence-grounded path.

## Shared communication principles

- Developers and LLMs should not need to know `targetScopeId`, `instanceId`,
  `artifactHandle`, trace inventory, or acquisition terminology to complete a
  normal investigation. These may remain internal Console concepts but should
  not appear in the LLM-facing contract.
- Evidence source and storage origin are also internal routing concepts. Console
  should resolve them without LLM input and surface only a domain-level warning
  or limitation when the distinction materially affects interpretation.
- The LLM may translate natural language into Console concepts, but it must not
  silently invent an identifier or target.
- An exact identifier should avoid an unnecessary broad inventory scan.
- Relative language such as “latest,” “the one I just ran,” or “that trace” must
  be resolved against explicit evidence or clearly identified conversational
  context.
- Finding a trace and resolving immutable evidence are different internal
  lifecycle steps even when the user experiences them as “loading the trace.”
  Console owns the latter; the LLM continues with `traceId`.
- A trace selected in the browser, evidence already reusable by Console, a
  `traceId` referenced earlier in the conversation, and a local file are not
  assumed to be the same state.
- The LLM should ask a clarifying question only when unresolved ambiguity could
  materially select the wrong evidence. It should not turn every ordinary
  request into a confirmation ceremony.
- Empty or partial search results support only conclusions within the stated
  target, filters, time window, pagination, and searchable fields.
- Trace selection and downstream inspection should use a stable `traceId`.
  Console should resolve acquisition and the exact installed evidence instance
  internally, surfacing disambiguation only when distinct evidence genuinely
  shares the same `traceId`.

## Area 1 — Find, load, select, or reuse a trace

### What the developer may mean by “load” or “review”

The verbs are not precise product operations:

- **find** may mean locate a trace ID from recency, skill, session, time, outcome,
  or remembered content;
- **load** may mean make target evidence available, reuse evidence Console
  already has, import a file, or simply make a trace the subject of the
  conversation;
- **review** may mean select the trace and wait for a follow-up, or immediately
  provide a compact overview; and
- **search** may mean search inventory metadata, records inside one trace, or
  content across many traces.

The LLM should resolve the smallest interpretation that makes progress without
performing a broad or expensive search the developer did not request.

### Exact trace identity

Representative developer wording:

- “I would like to review trace ID
  `9f0a67b3-2955-4240-b7c1-c5e3263c1f94`.”
- “Load trace `9f0a67b3-2955-4240-b7c1-c5e3263c1f94`.”
- “Pull up this trace: `9f0a67b3-2955-4240-b7c1-c5e3263c1f94`.”
- “Can you see trace `9f0a67b3-2955-4240-b7c1-c5e3263c1f94`?”
- “Continue investigating trace
  `9f0a67b3-2955-4240-b7c1-c5e3263c1f94`.”

Likely communication contract:

- The developer has supplied the strongest catalog identity; do not enumerate
  unrelated recent traces first.
- The current selected target still matters because a `traceId` is not treated
  as authorization to search arbitrary targets.
- If inspection is requested, use the supplied `traceId`; Console should acquire
  or reuse the correct evidence internally for downstream calls.
- If “review” has no narrower subject, a compact orientation may be useful, but
  loading every frame, record, and content value is not.
- If the ID is unavailable, distinguish not found, wrong/current target,
  authentication failure, incompatible target, acquisition failure, and expired
  installed evidence rather than reporting one generic failure.

Open question: should “review trace ID …” default to a compact trace overview,
or should the LLM only confirm selection and ask what the developer wants to
inspect? This may be evaluated as a communication preference rather than fixed
in MCP.

### Skill name plus recency

Representative developer wording:

- “I would like to review the most recent `handleIncident` skill run.”
- “Show me the latest completed `handleIncident` trace.”
- “Pull up the last successful `handleIncident` run.”
- “Find the most recent failed `handleIncident` run.”
- “Show me the previous `handleIncident` run, not the one currently executing.”
- “Load the second-most-recent `handleIncident` trace.”
- “Find the `handleIncident` run from about ten minutes ago.”
- “Show me the last `handleIncident` run before the deployment.”

Terms that require defined semantics:

- whether “most recent” orders by start, completion, finalization, or catalog
  admission time;
- whether an active execution is eligible or only finalized traces are eligible;
- whether the skill name means the entry skill or any nested invocation;
- whether “successful” refers to trace outcome, mission outcome, or absence of a
  particular failure record;
- what time zone and tolerance apply to approximate time; and
- whether the search examined enough inventory pages to support “most recent.”

Minimum useful LLM behavior:

- Prefer recorded entry-skill, lifecycle, outcome, and timestamp facts over
  route-name or content heuristics.
- State the selected trace ID and the facts used to disambiguate it.
- When the requested ordering and eligibility rules produce one clear candidate,
  proceed without asking the developer to confirm the obvious result.
- When two plausible candidates remain, show a compact distinction—normally
  trace ID, entry skill, outcome, start/finalization time, and session ID—and ask
  only for the missing choice.
- Do not claim “latest” or “none exists” when pagination or a bounded window
  leaves that conclusion unproven.

### “The run I just performed”

Representative developer wording:

- “I just ran `handleIncident`; show me the trace.”
- “Review the skill run I just performed.”
- “What happened in that run?”
- “Open the run that just finished.”
- “I reran it—use the new trace.”

This phrasing combines conversational context with recency. The LLM may know the
skill name from the current message or an earlier turn, but the runtime remains
the authority for which execution or finalized trace exists.

Potential ambiguity includes:

- another user or automated process ran the same skill more recently;
- the execution is still active and has no finalized trace;
- finalization or catalog visibility is delayed;
- the current target changed since the run; or
- the developer reran the skill and the conversation still contains an older
  `traceId`.

The LLM should prefer a bounded skill-plus-recency lookup when the skill is
known. If the run is active, it should distinguish provisional execution
evidence from a finalized trace rather than pretending the trace is missing or
complete. A previously selected trace must not silently win over the developer's
clear statement that a new run occurred.

### Session or execution identity

Representative developer wording:

- “Find the trace for session `…`.”
- “Review the completed trace for this active execution.”
- “I copied this session ID from the application; can you find its trace?”
- “Which trace came from execution `…`?”
- “The run failed in session `…`; pull up the evidence.”

The developer has supplied a relationship key rather than the final trace
identity. The desired path is a bounded recorded join to one trace or a small
candidate set, not literal search over arbitrary record bytes.

Open questions:

- Is `sessionId` available and filterable in trace inventory today?
- Can an active execution provide its eventual `traceId` directly when
  finalized?
- Can one session legitimately produce multiple trace artifacts, and if so what
  additional lifecycle facts distinguish them?

### A trace manually loaded in the Console

Representative developer wording:

- “I just manually loaded a trace that I would like to review.”
- “Use the trace I opened in the Console.”
- “I already loaded the trace; don't download it again.”
- “Review the NDJSON trace I imported.”
- “Use the trace currently displayed in the browser.”

At least four starting states may be hidden behind this wording:

1. the developer selected a catalog trace in the browser and the Console
   acquired or reused an immutable artifact;
2. the artifact is present in the shared Console artifact service, but no
   portable MCP operation identifies it as “the currently displayed trace”;
3. the developer previously supplied a `traceId` in this LLM conversation; or
4. the developer opened a local NDJSON file that may not exist in the target
   catalog or MCP evidence scope.

Safe LLM behavior depends on which state the interface can prove. It should not
guess that browser selection is visible to MCP or treat a recorded `sourcePath`
as a readable local path. Once it has a `traceId`, Console—not the LLM—decides
whether existing evidence can be reused or must be reacquired.

Product questions to resolve:

- Does the Console expose the `traceId` of the most recently admitted or
  browser-selected trace to MCP, and would that state be per browser session, per MCP client,
  or process-global?
- Would exposing mutable “current trace” state create cross-client races or
  surprising evidence changes?
- Is the unified trace inventory sufficient, or is a single current-trace
  pointer ever justified?
- When the developer imports a local artifact, how is its validated `traceId`
  made discoverable without exposing its storage owner?
- What concise clarification should the skill recommend when none of those
  relationships is observable?

Until those questions are answered, “manually loaded” is not treated as a
portable identifier.

### Browsing available traces

Representative developer wording:

- “What traces are available?”
- “Show me the ten most recent traces.”
- “List today’s `handleIncident` runs.”
- “Which skill runs failed in the last hour?”
- “Show me traces from session `…`.”
- “Are there any completed traces after 14:00?”
- “Let me choose from the recent `handleIncident` runs.”

The minimum useful result is a compact candidate list, not full expansion of
every trace. Candidate facts may include trace ID, entry skill, session ID,
outcome, and start/finalization time. The result must
state ordering, filter scope, page/window completeness, and continuation.

The LLM should normally inspect only the selected candidate. A developer asking
what is available has not implicitly requested that every trace be resolved or
all trace content be loaded into model context.

### Selection by remembered incident, input, output, or content

Representative developer wording:

- “Find the trace for incident `INC-2401`.”
- “Load the run where the input mentioned EU DNS failures.”
- “Find the trace where the model returned `SINGLE_TOOL_OVERUSE`.”
- “Which run produced customer ID `12345`?”
- “Find the trace containing this error message.”

These questions may require very different evidence:

- inventory metadata may already contain the requested value;
- a session, ticket, or application correlation ID may provide a structured
  relationship;
- the value may exist only in record metadata or ordinary record `data`;
- it may exist only in referenced/chunked content; or
- it may require selecting and searching multiple candidate traces.

The LLM must not describe inventory-metadata search as a complete cross-trace
content search. Before we design support, we need to decide which structured
correlation fields are common enough for inventory and whether bounded
cross-trace semantic-content search belongs in this product increment.

### Approximate time, outcome, and other remembered facts

Representative developer wording:

- “Find the run from around 3:15 PM.”
- “Show me yesterday's failed billing run.”
- “Load the trace that took about two minutes.”
- “Find the run that completed after the service restarted.”
- “I need the successful run immediately after the failed one.”

These requests combine filters and may still return multiple candidates. Time
zone, daylight-saving behavior, tolerance, lifecycle timestamp, outcome
definition, and inventory completeness must be explicit enough for the LLM to
avoid false precision. Repository or deployment context supplied by the
developer is context, not runtime evidence, unless the trace records a matching
fact.

### References carried by the conversation

Representative developer wording:

- “Load the first one.”
- “Use that trace.”
- “Go back to the previous run.”
- “Keep investigating the same trace.”
- “Actually, use the failed one instead.”

The LLM may resolve these expressions from its conversation when the referent is
unambiguous. It should retain and cite the selected `traceId`. Console remains
responsible for determining whether matching evidence is currently available
and belongs to the current target scope; conversation memory does not make
expired or prior-scope evidence valid.

If a user switches traces, the LLM should make the transition visible and avoid
combining evidence from the old and new traces unless comparison was
explicitly requested.

### Active, delayed, missing, and unavailable traces

Representative developer wording:

- “Why can't you find the run that is still executing?”
- “The skill completed, but its trace is not listed yet.”
- “That trace ID definitely exists in the application.”
- “Reload the trace; the evidence you used earlier is no longer available.”
- “I changed targets—find the corresponding run here.”
- “Can you review this trace even though the application is offline?”

The response must keep these states distinct:

- active execution with provisional evidence;
- completed execution awaiting or lacking finalization;
- finalized trace absent from the bounded inventory search;
- trace unavailable on the currently selected target;
- target authentication or compatibility failure;
- trace evidence that Console cannot currently resolve;
- usable evidence despite upstream unavailability;
- evidence that expired and cannot be transparently reacquired; and
- prior-target evidence rejected after target change.

A useful degraded response says what was checked, what remains available, and
the smallest next fact needed. It does not collapse every case into “trace not
found” or repeatedly broaden the search without a bound.

### Local paths, attachments, and copied trace data

Representative developer wording:

- “Review `C:\\downloads\\trace.ndjson`.”
- “I attached the trace file.”
- “Here are the first hundred trace records.”
- “The trace says its source path is `/tmp/trace.ndjson`; open it.”

Portable skill/MCP behavior must distinguish an explicitly supplied file or
attachment from a `sourcePath` merely recorded inside untrusted runtime
evidence. A recorded path is not proof that the MCP server or LLM host can or
should read that location.

Whether direct local-file import belongs to Console MCP is an open product
question. A particular agent host may have separately authorized filesystem or
attachment access, but the portable Loomspan skill cannot assume it.

## Initial trace-selection communication matrix

| Developer supplies | Example | LLM may do directly | Must still establish |
| --- | --- | --- | --- |
| Exact `traceId` | “Review trace `…`” | Inspect that trace directly | Whether Console can resolve unambiguous usable evidence |
| Skill plus recency | “Latest `handleIncident` run” | Perform bounded inventory selection | Ordering, active/finalized eligibility, completeness |
| Session/execution ID | “Trace for session `…`” | Use a recorded relationship or filter | Whether the relationship is unique and available |
| Approximate time/outcome | “Failed run around 15:15” | Narrow to compact candidates | Time zone/window, outcome meaning, pagination |
| Conversational reference | “Use that trace” | Reuse the prior explicit `traceId` | Whether matching evidence remains valid in the current Console context |
| Browser/manual-load statement | “Use the trace I opened” | Reuse only if the relationship is observable | Which `traceId`, client/session ownership of the selection |
| Remembered content | “Run containing `INC-2401`” | Use structured correlation when present | Search fields, referenced-content coverage, bounds |
| Local file/attachment | “Review this NDJSON” | Use separately authorized import/access when available | Provenance, scope, parsing/admission path |
| No identifying clue | “Show me the trace” | Present a small recent candidate set | Developer's intended run |

## Baseline conversations to evaluate

These are communication sketches, not required answer text.

### Exact trace

```text
Developer: I would like to review trace 9f0a67b3-2955-4240-b7c1-c5e3263c1f94.
LLM: inspects that exact trace, states the selected identity and
     compact orientation, then invites or follows the requested focus.
```

Failure variant: the LLM reports the specific domain-level availability condition and
does not scan unrelated traces as if the developer had omitted the ID.

### Most recent named skill

```text
Developer: I would like to review the most recent handleIncident skill run.
LLM: applies documented recency and entry-skill semantics, selects the unique
     candidate when proven, states its trace ID/time/outcome, and inspects it.
```

Ambiguous variant: the LLM presents only the few facts needed to distinguish the
candidates instead of expanding both traces.

### Manually loaded trace

```text
Developer: I just manually loaded a trace that I would like to review.
LLM: uses a visible trace-selection relationship if the product exposes one;
     otherwise asks for the trace ID or clarifies whether the user
     means a Console-selected catalog trace or a local imported file.
```

The evaluation must not award a guessed match merely because it happened to be
the most recent trace.

## Preliminary interface pressures, not decisions

The question inventory currently suggests investigating:

- direct exact-ID inspection without prior inventory enumeration;
- documented recent-first ordering and active-versus-finalized eligibility;
- bounded inventory filters for entry skill, time window, outcome, and session
  when measurements show traversal is otherwise costly;
- compact candidate facts and explicit search/pagination completeness;
- a safe way to discover a browser-selected or manually imported trace by
  `traceId` without a process-global mutable “current trace” race;
- an explicit active-execution-to-finalized-trace relationship;
- clear separation of inventory metadata search, within-trace record/content
  search, and any future cross-trace semantic search; and
- skill guidance for clarification thresholds, session-to-trace discovery,
  target continuity, and degraded evidence states.

None of these pressures yet proves that a new MCP operation is necessary.
Existing tools, schemas, ordering, or skill guidance may satisfy some after
measurement.

## Open questions for the human/LLM design team

1. When a developer says only “review this trace,” should the baseline response
   include a compact overview or stop after confirmed selection?
2. What exact lifecycle timestamp defines “most recent” for finalized traces?
3. Does “most recent skill run” mean entry skill by default, with nested-skill
   search requiring explicit wording?
4. How should an active run transition to its finalized trace without polling
   unrelated inventory?
5. Which inventory filters already exist at the application boundary, and which
   would require new upstream support rather than only Console/MCP work?
6. Can MCP discover the `traceId` of evidence manually loaded through the
   browser today? If so, what client/session ownership prevents cross-user or
   cross-client surprise?
7. Is the unified trace inventory sufficient for manually loaded evidence, or
   can a mutable “current trace” pointer ever pass the method test?
8. Is local NDJSON import part of the portable MCP product, browser-only
   behavior, or deliberately outside this roadmap?
9. How much inventory may the LLM traverse before it should narrow the time
   window or ask the developer for another clue?
10. Which application correlation identifiers—ticket, request, execution, or
    business key—are sufficiently stable and common to expose structurally?
11. Is bounded cross-trace semantic-content search needed, or should the first
    release require selecting candidate traces before content search?
12. What evidence must a result return before the LLM may say “latest,” “only,”
    or “no matching trace exists”?

## Next collaborative step

Implement the linked first-pass cleanup ticket in a fresh context. Then connect
an LLM client to the resulting MCP server and walk through exact-ID,
skill-plus-recency, copied-session, imported, active, failed/retried,
unavailable, and large-content traces.

Record which methods are naturally discoverable, which parameters cause
hesitation, which return fields are ignored, how errors are recovered, and the
approximate call/context cost. Use that evidence to decide the remaining
session identity, content-reference, pagination, activity-continuity,
physical-record, fallback-duplication, and method/resource candidates one
section at a time.
