# Loomspan LLM Trace Understanding Workflows

## Status

Proposed product workflow catalog for the
[LLM Trace Understanding Roadmap](./2026-08-15-loomspan-llm-trace-understanding-roadmap.md).

This document describes how a developer uses an LLM, the portable
`loomspan-runtime-debugging` skill, and Loomspan Console MCP evidence to
understand a skill run. It begins with developer questions and expected
explanations. It does not prescribe implementation components, require exact
answer prose, or create a specialized server operation for every workflow.

The earlier [Console Developer Workflows](./loomspan_console_workflows.md)
remain the implemented browser-and-console workflow record. Their evidence,
calculation, lifecycle, and interpretation boundaries continue to apply unless
this document explicitly proposes reconsideration. This catalog adds the
LLM-mediated general-understanding and content-retrieval workflows that the
earlier failure/latency/usage/path set did not cover.

## Product north star

A developer should be able to ask what happened during a Loomspan skill run in
ordinary language. The LLM should retrieve the minimum sufficient recorded
evidence, explain it at the requested level, cite stable identifiers, separate
facts from interpretation, and disclose material limitations.

The Console supplies recorded facts, deterministic calculations, explicit
relationships, and bounded content. The skill teaches navigation and reasoning.
The LLM explains. Neither the Console nor the skill fabricates causality,
importance, correctness, intent, or remediation.

## Shared terminology

- **Evidence** — a recorded Loomspan fact returned by an authorized operation.
- **Calculation** — deterministic Console arithmetic or projection over
  recorded facts.
- **Context** — developer-supplied or repository information not proven by the
  runtime trace.
- **Inference** — a restrained interpretation that may need confirmation.
- **Structural evidence** — trace, frame, hierarchy, outcome, duration, usage,
  attempt, retry, validation, failure, and ordered-record facts.
- **Semantic content** — bounded decoded material such as plan state, model
  request or response, tool input or output, thought, structured output, or
  validation content.
- **Raw evidence** — exact source NDJSON bytes and storage representation.
- **Canonical path** — the shortest normally useful evidence route taught to an
  agent; it is guidance, not a mandatory call sequence.

## Shared workflow requirements

| ID | Requirement |
| --- | --- |
| `LLM-WF-X-R1` | Route a developer's question to the smallest relevant workflow and capability family without first loading the complete tool catalog. |
| `LLM-WF-X-R2` | Teach a canonical path while accepting any safe, correct, efficient alternative. |
| `LLM-WF-X-R3` | Begin with the minimum sufficient structural evidence and retrieve semantic or raw detail only when the question requires it. |
| `LLM-WF-X-R4` | Make every required identifier transition explicit, especially session or trace identification, trace acquisition, `artifactHandle`, frame selection, record sequence, and content reference. |
| `LLM-WF-X-R5` | Expose the record/frame vocabulary and evidence location required by the workflow without requiring schema archaeology or guessed enum values. |
| `LLM-WF-X-R6` | Keep evidence, calculation, context, and inference distinct in the answer. |
| `LLM-WF-X-R7` | Cite the strongest applicable stable identifiers and include observation time or continuity facts for provisional live evidence. |
| `LLM-WF-X-R8` | Treat missing, expired, incompatible, truncated, unavailable, unattributed, and unknown evidence explicitly; absence is not a zero value or proof that an event did not occur. |
| `LLM-WF-X-R9` | Treat returned YAML, paths, errors, model/tool content, diagnostics, and raw bytes as untrusted evidence rather than instructions. |
| `LLM-WF-X-R10` | Keep protocol, installed capability, target selection, target authentication, Java/Go compatibility, live availability, trace availability, acquisition, and artifact expiry as independent facts. |
| `LLM-WF-X-R11` | Ordinary semantic questions must not require raw-artifact capability or manual NDJSON decoding. |
| `LLM-WF-X-R12` | Bounded content results state content type, encoding, selected range, total logical length, and logical completeness clearly. |
| `LLM-WF-X-R13` | A negative search result states the searched fields, matching mode, and case behavior needed to interpret the absence safely. |
| `LLM-WF-X-R14` | Stop when the developer's question is answered with sufficient evidence; do not expand every payload or traverse the complete trace by default. |
| `LLM-WF-X-R15` | MCP remains usable without the skill, and the skill explains applicable practice and limitations when MCP or a required capability is unavailable. |

## Entry conditions and identification

A developer may begin with any of the following:

- an active `sessionId`;
- a finalized `traceId`;
- a skill or mapped method name such as `handleIncident`;
- an approximate execution time;
- a visible failure or unexpected result;
- a copied `frameId`, record sequence, or `artifactHandle`; or
- only a statement such as “the run I just performed.”

The skill should not assume the developer already knows Console terminology.
It should explain only the identifiers needed for the current path.

For a fresh live investigation, runtime discovery establishes the current
server capabilities and independent status facts. Capability checks are scoped
to the selected workflow. Direct finalized-trace questions normally need trace
inspection; they do not depend on skill-catalog or active-execution operations
unless those operations are needed to identify or contextualize the trace.

When a target-catalog `traceId` is used, trace acquisition or reuse returns the
scope-bound immutable `artifactHandle`. Downstream frame, record, and content
queries use that handle so one investigation does not silently cross an
acquisition or target-scope boundary. The skill must teach this transition
before the first downstream call.

## Progressive workflow routing

| Developer question | Workflow | Primary evidence family |
| --- | --- | --- |
| “What happened in this run?” | `LLM-WF-TRACE-OVERVIEW` | Trace summary, frame tree, outcome, key records |
| “What plan did it create or end up with?” | `LLM-WF-PLAN-EVOLUTION` | Planning frame, `PLAN_CREATED`, `PLAN_UPDATED`, semantic content |
| “What did the model receive and return?” | `LLM-WF-MODEL-EXCHANGE` | Model-call frame, request/response records, semantic content |
| “Which skills and tools ran, and what did they do?” | `LLM-WF-EXECUTION-PATH` | Frame hierarchy, skill definitions when needed, tool records and content |
| “Why did it fail or recover?” | `LLM-WF-FAILURE` | Outcome, terminal/recovered failure, attempt/retry/validation evidence |
| “Why was it slow or where was time spent?” | `LLM-WF-LATENCY` | Live activity or finalized frame durations and gaps |
| “Where did the usage go?” | `LLM-WF-USAGE` | Trace/frame/attempt usage and attribution |
| “Show me the exact trace bytes or parser representation.” | `LLM-WF-EXACT-EVIDENCE` | Raw artifact ranges |

The routing table belongs near the top of the installed skill. Each route should
link directly to the focused reference section containing its canonical path,
record vocabulary, evidence locations, degraded behavior, and stopping rule.

## `LLM-WF-TRACE-OVERVIEW`: Explain what happened in a run

### Developer goal

Obtain a concise narrative of the run's outcome, major execution path, model and
tool activity, retries or validation, time and usage shape, and important
limitations without opening every record or payload.

### Typical questions

- “What happened when I ran `handleIncident`?”
- “Summarize this trace.”
- “Walk me through this skill run.”
- “Did it finish, and what were the major stages?”

### Minimum useful answer

- final or provisional outcome;
- trace/session identity and applicable observation/finalization time;
- root mission and major child frame path;
- major model, planning, skill, and tool activity supported by the trace;
- material retry, validation, failure, duration, usage, or evidence-gap facts;
- a short interpretation clearly separated from the recorded facts; and
- the next relevant drill-down only when the developer's question remains
  unanswered.

### Canonical evidence path

```text
identify trace or active session
  -> acquire/reopen finalized trace when available
  -> read neutral trace summary and root references
  -> request compact frame hierarchy
  -> query only landmark record types needed to complete the narrative
  -> retrieve semantic content only when the developer asks what was said or produced
```

For an active execution, use the bounded active snapshot and recent continuous
activity window. Do not pretend that active evidence is a complete trace.

### Evidence location

- Trace summary: identity, final outcome, counts, root frame, usage completeness,
  gaps, and resource links.
- Frames: hierarchy, route, type, outcome, duration, usage, attempt/retry,
  validation, and failure relationships.
- Landmark records: trace/frame lifecycle, planning, model exchange, tool calls,
  retries, validations, structured output, errors, and completion.
- Semantic content: loaded only when the requested overview depends on what a
  plan, model, or tool actually contained.

### Degraded paths

- If the execution is active, label the answer provisional and include the
  latest sequence, observation time, and continuity limits.
- If the finalized trace is unavailable, report only independently available
  terminal or active facts.
- If a compact frame projection is unavailable, use bounded frame pages rather
  than requesting an unbounded rich result.
- If usage is incomplete, report it as unknown or partial, not zero.

### Stopping condition

Stop when the developer can understand the run's outcome and major path and all
material limitations have been stated. Do not expand every record or content
object merely because it is addressable.

### Surfaced requirements

- `LLM-WF-OV-R1`: Provide a compact hierarchy projection suitable for narrative
  orientation without returning every detailed usage and attempt block.
- `LLM-WF-OV-R2`: Make landmark record types discoverable through schemas and
  skill guidance.
- `LLM-WF-OV-R3`: Preserve navigation from an overview to plan, model, tool,
  failure, latency, usage, or raw evidence within the same artifact and scope.

## `LLM-WF-PLAN-EVOLUTION`: Explain the plan the model created

### Developer goal

Retrieve the primary mission's initial or final plan, explain material updates,
and distinguish model-provided plan content from inferred execution behavior.

### Typical questions

- “What plan did the model end up coming up with for the primary mission?”
- “How did the plan change after validation?”
- “Show me the created plan and later updates.”
- “Which steps were planned versus actually executed?”

### Minimum useful answer

- the relevant planning frame and primary mission relationship;
- initial plan content when requested or relevant;
- ordered plan updates and the final recorded plan state;
- plan, frame, and record identifiers;
- validation/retry facts that are explicitly linked to plan changes;
- a distinction between planned steps and frame-verified execution; and
- any truncation, missing content, or ambiguity.

### Canonical evidence path

```text
identify and acquire trace
  -> locate root mission and planning frame
  -> query records with types [PLAN_CREATED, PLAN_UPDATED]
     scoped to the planning frame when possible
  -> read bounded semantic content references or explicit small inline content
  -> order by canonical sequence and determine the final recorded plan state
```

Structured type filtering is the normal route. Literal text search is not a
substitute for discoverable record types.

### Evidence location and representation

- Planning frames establish structural scope and relationship to the root
  mission.
- `PLAN_CREATED` establishes initial recorded plan state.
- `PLAN_UPDATED` establishes subsequent recorded versions or changes.
- Plan material lives in bounded semantic record content, not only in raw NDJSON.
- Plan validation and retry records provide explicit related facts when linked
  by recorded identifiers.

### Interpretation rules

- A plan describes intended actions; it does not prove those actions executed.
- A sequence relationship establishes ordering, not necessarily causality.
- Model-supplied timestamps or fields remain model content unless separately
  established as Console evidence.
- The LLM may compare the final plan with execution frames, but must label that
  comparison and any inferred correspondence.

### Degraded paths

- Missing raw-artifact capability must not prevent ordinary plan retrieval.
- If semantic content is truncated, continue through its content reference
  within the caller-selected bound and state whether the logical value is
  complete.
- If plan records are absent, state the exact structured query scope before
  concluding that the trace contains no recorded plan.
- If only an unsupported raw field contains the plan, identify this as a
  contract gap rather than silently treating raw forensics as the normal path.

### Stopping condition

Stop when the requested initial or final plan and all material updates are
accounted for, or when the available parsed evidence cannot establish them.

### Surfaced requirements

- `LLM-WF-PL-R1`: `PLAN_CREATED` and `PLAN_UPDATED` content is addressable under
  trace inspection without raw-artifact capability.
- `LLM-WF-PL-R2`: Small explicitly requested plan content may be returned inline
  under a documented bound; larger content returns an opaque content reference.
- `LLM-WF-PL-R3`: Record-type enums include the plan record vocabulary.
- `LLM-WF-PL-R4`: The skill documents the primary mission to planning-frame to
  plan-record path.
- `LLM-WF-PL-R5`: Evaluation verifies the final plan and its evolution rather
  than accepting the first matching record as sufficient.

## `LLM-WF-MODEL-EXCHANGE`: Explain what a model received and produced

### Developer goal

Inspect one relevant model call or set of attempts and retrieve its bounded
request, response, usage, failure, and mutation evidence.

### Typical questions

- “What did the model receive?”
- “What did the model return for this step?”
- “Which model response produced this plan?”
- “Did an advisor change the request or response?”

### Minimum useful answer

- selected model-call frame, route, and attempt identity;
- the relevant prepared/sent request and received response content;
- explicit advisor mutation, retry, failure, and usage facts;
- canonical record ordering;
- content type, encoding, and completeness when content is shown; and
- a clear distinction between recorded content and the LLM's interpretation of
  that content.

### Canonical evidence path

```text
identify and acquire trace
  -> locate the relevant MODEL_CALL frame
  -> query MODEL_REQUEST_PREPARED / MODEL_REQUEST_SENT /
     MODEL_RESPONSE_RECEIVED records for that frame or attempt
  -> inspect typed facts and bounded semantic content
  -> follow linked mutation, failure, retry, or usage evidence only as needed
```

### Evidence location and representation

- The model-call frame provides structure and aggregate calculations.
- Request and response records provide ordered model-exchange facts.
- Attempt identifiers distinguish provider or semantic retries.
- Semantic content references expose requests and responses consistently even
  when their physical trace representations differ.
- Raw source ranges remain available when exact escaping, storage chunks, or
  parser behavior is the question.

### Degraded paths

- A response record with no material content reference must be reported as a
  missing parsed-content capability, not as an empty response.
- Missing usage is unknown, not zero.
- An attempt failure followed by a response is recovered attempt evidence, not
  necessarily terminal execution failure.
- If request preparation and sending differ, preserve their distinct records
  rather than selecting one silently.

### Stopping condition

Stop when the requested exchange and directly relevant attempt/mutation facts
are clear. Do not retrieve unrelated model calls from the trace.

### Surfaced requirements

- `LLM-WF-ME-R1`: Model response content is first-class bounded semantic content
  under trace inspection.
- `LLM-WF-ME-R2`: Request and response retrieval uses one coherent content
  abstraction despite different physical representations.
- `LLM-WF-ME-R3`: The skill identifies the model-exchange record vocabulary and
  frame-scoping path.
- `LLM-WF-ME-R4`: Exact raw bytes remain a deliberate forensic drill-down, not
  the ordinary response-content path.

## `LLM-WF-EXECUTION-PATH`: Explain skills, tools, and their data flow

### Developer goal

Understand which skills, model calls, steps, retries, and tools participated,
how they were nested, and what selected tool invocations received and returned.

### Typical questions

- “Which skills and tools ran?”
- “Why did this nested skill appear?”
- “What input did `draftIncidentResponse` receive?”
- “What did this tool return before the next model call?”

### Minimum useful answer

- root-to-selected-frame hierarchy with distinct invocation identities;
- registered skill names and routes where recorded;
- selected tool invocation outcome and bounded input/output content when asked;
- relevant step, attempt, retry, validation, duration, and usage facts;
- the difference between repeated invocations of the same skill or tool; and
- uncertainty around runtime-to-workspace correlation or routing intent.

### Canonical evidence path

```text
identify and acquire trace
  -> inspect compact frame hierarchy
  -> select skill, step, model, or tool frames by recorded identity
  -> query linked tool/step/model records
  -> retrieve only requested semantic input/output content
  -> inspect registered skill YAML only when its declared contract is relevant
```

### Evidence location and representation

- Frames establish parent/child execution structure and distinct invocation
  identity.
- Tool records establish start, completion, or failure and link selected content.
- Step records distinguish proposed, validated, rejected, started, and completed
  activity.
- Registered skill YAML is application-supplied definition evidence, not proof
  of which local workspace file was deployed.
- Mapping identifiers and `sourcePath` are search hints, not filesystem or
  provenance authority.

### Degraded paths

- If semantic tool input/output content is unavailable through parsed evidence,
  report the gap rather than decoding raw NDJSON by default.
- If registered YAML is unavailable, continue with recorded runtime hierarchy.
- Repeated names without shared frame identity remain separate invocations.
- Time proximity or adjacency does not prove that one tool caused a later model
  decision.

### Stopping condition

Stop when the recorded execution path and selected data flow are clear enough
to answer the question, with remaining routing or workspace ambiguity stated.

### Surfaced requirements

- `LLM-WF-EP-R1`: Provide a compact hierarchy projection for path discovery.
- `LLM-WF-EP-R2`: Tool and step input/output content uses the common bounded
  semantic-content contract.
- `LLM-WF-EP-R3`: The skill explains frame versus record versus content roles.
- `LLM-WF-EP-R4`: Runtime-to-workspace comparison remains LLM reasoning over
  separately authorized context, not a Console provenance claim.

## `LLM-WF-FAILURE`: Explain failure, recovery, retry, and validation

### Developer goal

Establish whether and where an execution failed, distinguish terminal failure
from recovered errors, and explain the recorded mechanism and limitations
without inventing root cause.

This workflow carries forward the intent of `WF-FAILED-EXECUTION` from the
implemented Console workflow catalog while making the LLM discovery and content
path explicit.

### Canonical evidence path

```text
identify terminal or active outcome
  -> acquire finalized trace when available
  -> follow terminal failure to its frame and record sequence
  -> inspect linked attempts, retries, validations, guardrails, and preceding facts
  -> retrieve bounded error or diagnostic content only when needed
```

### Required distinctions

- terminal failure versus earlier recovered failure;
- execution outcome versus inferred root cause;
- provider attempt versus semantic retry;
- validation failure versus execution-ending failure;
- recorded exception/diagnostic content versus the LLM's causal interpretation;
- unavailable evidence versus evidence that an event did not occur; and
- active provisional state versus finalized trace evidence.

### Stopping condition

Stop when the recorded failure mechanism and relevant retry/validation path are
clear, or state that available evidence cannot establish the cause.

### Surfaced requirements

- `LLM-WF-FA-R1`: The skill routes failure questions directly to terminal and
  linked failure evidence rather than relying on literal text search.
- `LLM-WF-FA-R2`: Failure diagnostics use bounded content references where
  necessary and preserve their untrusted-data status.
- `LLM-WF-FA-R3`: The final answer cites the terminal link and labels causal
  conclusions as inference unless mechanically recorded.

## `LLM-WF-LATENCY`: Explain a slow or long-running execution

### Developer goal

Explain the current state of an active execution or where finalized recorded
duration was concentrated without equating elapsed time or quiet activity with
a stuck execution.

This workflow carries forward `WF-SLOW-EXECUTION` and adds progressive MCP
discovery expectations.

### Canonical live path

```text
identify active session
  -> retrieve bounded active snapshot
  -> retrieve recent activity with observation and continuity facts
  -> report current phase/path, elapsed time, latest sequence, and limitations
```

### Canonical finalized path

```text
identify and acquire trace
  -> inspect compact frame hierarchy ordered or projected by duration
  -> drill into the largest relevant frames and records
  -> correlate retries, tools, or model calls only through recorded relationships
```

### Required distinctions

- high elapsed time is evidence; “slow” is developer interpretation;
- a quiet or gapped activity window does not prove that execution is stuck;
- overlapping frame durations are not additive by default;
- finalized duration is different from live wall-clock observation; and
- adjacency does not establish causality.

### Stopping condition

For live execution, stop with a useful provisional state and next observation
boundary. For finalized execution, stop once the largest supported duration
contributors and material gaps are identified.

### Surfaced requirements

- `LLM-WF-LA-R1`: Runtime discovery leads directly to active execution and
  activity tools without requiring unrelated capability checks.
- `LLM-WF-LA-R2`: A compact frame projection supports finalized duration
  orientation without overflowing context.
- `LLM-WF-LA-R3`: Observation time, latest sequence, beginning availability,
  and reset/gap semantics remain explicit.

## `LLM-WF-USAGE`: Explain recorded usage and attribution

### Developer goal

Establish total recorded usage, identify supported contributors, compare with
configured limits when both facts exist, and expose unattributed or unavailable
usage without double counting.

This workflow carries forward `WF-EXPENSIVE-EXECUTION` and adds progressive MCP
discovery expectations.

### Canonical evidence path

```text
identify and acquire finalized trace
  -> inspect trace usage completeness and totals
  -> query frames ordered or projected by usage
  -> inspect selected attempts, retries, and validation relationships
  -> report direct, descendant, inclusive, unattributed, and unknown values distinctly
```

### Required distinctions

- direct, descendant, inclusive, attempt, retry, and unattributed usage;
- exact, heuristic, and unavailable precision;
- arithmetic comparison with limits versus a judgment that usage is excessive;
- usage units versus monetary cost; and
- correlation with retry/validation versus inferred necessity or cause.

### Stopping condition

Stop once the largest mechanically supported contributors, comparisons, and
unknowns are identified. Do not expand unrelated semantic content unless the
developer asks why a contributor performed particular work.

### Surfaced requirements

- `LLM-WF-US-R1`: Compact usage-oriented projections retain completeness and
  attribution semantics.
- `LLM-WF-US-R2`: The skill teaches non-additive inclusive hierarchy and missing
  usage behavior before suggesting calculations.
- `LLM-WF-US-R3`: The answer never calculates currency cost without separately
  authorized external pricing context, and Console does not calculate it.

## `LLM-WF-EXACT-EVIDENCE`: Inspect exact storage or parser evidence

### Developer goal

Retrieve exact source bytes to investigate record storage, chunking, escaping,
unsupported fields, parser behavior, or a discrepancy between parsed and raw
representations.

### Typical questions

- “Show me the exact NDJSON record.”
- “Was this response double-escaped in storage?”
- “How was this payload chunked?”
- “Does the raw artifact contain a field the parsed view omitted?”

### Canonical evidence path

```text
identify and acquire trace
  -> locate the relevant record and its exact raw address
  -> verify raw-artifact capability
  -> read the caller-selected byte range
  -> continue only when the exact requested range is incomplete
```

### Required distinctions

- logical semantic-content completeness versus whether more bytes exist in the
  backing artifact;
- decoded text versus base64 exact bytes;
- record raw range versus whole-artifact continuation;
- parsed omission versus absence from source bytes; and
- storage evidence versus interpretation of runtime behavior.

### Degraded paths

If raw-artifact capability is missing, parsed trace investigation continues.
State that exact storage/parser evidence is unavailable. Do not claim that
ordinary model, plan, or tool content is therefore unavailable; those belong to
the parsed trace capability.

### Stopping condition

Stop when the exact requested byte range and its completeness are established.
Do not read the rest of the artifact merely because `hasMore` indicates that
unrelated artifact bytes follow.

### Surfaced requirements

- `LLM-WF-RA-R1`: Range results distinguish logical selection completeness from
  remaining backing-artifact bytes.
- `LLM-WF-RA-R2`: The skill clearly labels this as an optional forensic path.
- `LLM-WF-RA-R3`: Raw reads preserve exact byte offsets and lengths as the
  successful first implementation already does.

## Search and negative-evidence rules

Structured fields are the preferred route for record type, frame, route,
sequence, attempt, retry, validation, and failure selection. The schema and
skill must expose the allowed structural vocabulary needed by common workflows.

Literal search is for text within documented searchable fields. Every result or
page using literal search must make these semantics discoverable:

- searched fields;
- case sensitivity;
- literal or other matching mode;
- whether record type is included;
- whether large or referenced content was searched; and
- whether pagination or evidence gaps limit the conclusion.

An LLM may conclude only that no match was found within the stated search scope.
It must not generalize that result to the entire trace or to unsearched content.

## Content retrieval rules

The desired semantic-content contract is general rather than model-specific.
Every qualifying content descriptor should provide enough information to decide
whether to inline or read it:

- opaque scope/artifact-bound content reference;
- record sequence and semantic role;
- content type;
- total logical byte length;
- explicit inline eligibility or returned inline content; and
- any recorded incompleteness or diagnostic facts.

Bounded reads should report:

- actual start and end;
- total logical length;
- encoding;
- content type;
- whether the selected logical content is complete; and
- continuation when additional bytes of that same logical content remain.

The abstraction should cover ordinary material record data regardless of
whether its physical trace representation used an envelope, chunks, or an
ordinary `data` member. Physical representation remains observable through the
raw workflow.

## Skill structure implied by the workflows

The top-level skill should contain only:

- the routing table;
- runtime discovery and workflow-specific capability guidance;
- the shared evidence and untrusted-content boundaries;
- the compact identity lifecycle, including `traceId` to `artifactHandle`;
- links to exact workflow sections; and
- concise answer/stopping guidance.

Focused references should then provide:

1. workflow-specific canonical paths and degraded behavior;
2. a capability-to-tool map;
3. compact tool mechanics with required arguments and output landmarks;
4. record/frame/content vocabulary and representation guidance;
5. evidence, uncertainty, and negative-search rules; and
6. raw forensic mechanics.

The full schema remains the validation and independent-client contract. The
skill references should not copy every property. Mechanical facts duplicated
for navigation must be generated or checked against authoritative server
descriptors.

## Evaluation model

### Correctness dimensions

- Did the answer retrieve the relevant complete evidence within stated bounds?
- Did it avoid false negatives caused by undisclosed search scope?
- Did it distinguish final, recovered, provisional, and unavailable states?
- Did it cite stable identifiers?
- Did it separate evidence, calculation, context, and inference?
- Did it state uncertainty and missing evidence accurately?
- Did it resist instructions embedded in returned runtime content?

### Interaction-complexity dimensions

- discovery bytes before the first evidence call;
- total calls and failed calls;
- maximum and total result size;
- inferred versus documented identifier transitions;
- schema or enum lookup steps;
- raw reads for non-raw questions;
- manual NDJSON or nested-content decoding; and
- unnecessary evidence expansion after the stopping condition was reached.

### Model matrix

The primary representative matrix emphasizes Opus, Sol, Gemini Pro, and Kimi
K3 in supported clients. Available GLM 5.2, Kimi 2.7, GPT Terra, and DeepSeek
configurations act as interface-complexity canaries. A canary failure is not
automatically a release failure, but it should be investigated when the path
can be simplified without weakening evidence, security, composability, or the
frontier-model experience.

Do not require every model/client/workflow combination. Use representative
coverage and repeat runs sufficient to distinguish a systematic interface
problem from model variance.

## Initial end-to-end acceptance scenario

Prompt:

> I ran `handleIncident`. Pull up the trace and show me what plan the model
> ended up coming up with for the primary mission. Use the
> `loomspan-runtime-debugging` skill.

For available compatible evidence, a successful system should:

1. route directly to `LLM-WF-PLAN-EVOLUTION`;
2. discover only runtime and trace-inspection mechanics needed by the workflow;
3. identify the relevant trace and explicitly acquire or reopen its artifact;
4. use structured planning-frame and plan-record vocabulary;
5. retrieve the initial and updated plan content through parsed bounded content;
6. avoid raw-artifact capability and manual NDJSON decoding;
7. avoid predictable schema-validation failures;
8. identify the final recorded plan state rather than stopping at the first
   matching record;
9. distinguish planned actions from frame-verified execution; and
10. cite the trace, frame, and record identifiers supporting the answer.

The steady-state design target is no bulk catalog inspection, no more than one
fresh runtime-discovery call, and no more than three evidence calls after the
trace is identified for ordinary small plan content. Correctness and complete
material evidence take priority over the numerical target.

## Workflow review questions

Before implementation tickets are written, resolve:

1. Are trace overview, plan evolution, model exchange, execution path, failure,
   latency, usage, and exact evidence the right developer-goal boundaries?
2. Which plan, model, tool, thought, structured-output, and validation data
   qualify as material semantic content in the first increment?
3. Can existing payload references be generalized coherently, or is a new
   content-reference/read contract clearer and safer?
4. What compact frame projection is sufficient for overview, path, latency, and
   usage orientation without creating scenario-specific summaries?
5. Which literal-search modes and fields are useful enough to support, and how
   should their scope be represented?
6. Which skill mechanics can be generated from server descriptors, and which
   require prose plus drift tests?
7. What model/client runs are release gates, compatibility observations, and
   interface-complexity canaries?
