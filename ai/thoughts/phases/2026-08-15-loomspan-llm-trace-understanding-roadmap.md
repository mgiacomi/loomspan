# Loomspan LLM Trace Understanding Roadmap

## Status

Active product roadmap, last updated 2026-08-18.

PRs 16–19 delivered the first local MCP runtime-inspection surface and the
portable `loomspan-runtime-debugging` skill. This roadmap treats that release as
the implemented baseline, not the completed developer experience. It defines
the next work needed for a developer to use an LLM, the portable skill, and the
Loomspan Console MCP server to understand a skill run accurately and
efficiently.

The companion
[LLM Trace Understanding Workflows](./loomspan_llm_trace_understanding_workflows.md)
is the product-facing workflow catalog for this roadmap. The earlier
[Console Developer Workflows](./loomspan_console_workflows.md) remain the
implemented browser-and-console workflow record until their requirements are
mapped into the new catalog. This roadmap incorporates the still-relevant
constraints from the completed LLM Runtime Inspector phase design and
supersedes that removed document as the active authority for future
skill-and-MCP trace-understanding work.

The first implementation step for this roadmap is the
[LLM-facing MCP trace interface cleanup ticket](../tickets/loomspan-mcp-llm-facing-trace-interface-cleanup.md).
It removes accepted Console-internal concepts from the model-facing contract
before further live MCP walkthroughs select additional changes.

## Outcome

A developer can ask an ordinary question about a Loomspan skill run and a
representative LLM can discover the applicable MCP operations, retrieve the
right level of evidence, and produce a concise evidence-backed explanation
without schema archaeology, avoidable failed calls, raw-storage forensics, or
frontier-model-only inference.

The target experience is one connected trace-understanding system:

- the developer expresses a goal in domain language;
- the skill routes the question to a useful evidence path;
- MCP exposes neutral, composable, independently usable evidence operations;
- semantic content is bounded and directly addressable;
- detail is disclosed progressively;
- the LLM distinguishes evidence, deterministic calculation, developer or
  repository context, and inference; and
- exact raw evidence remains available as a forensic fallback rather than the
  ordinary route to model, plan, tool, or validation content.

## Why another roadmap is required

The first implementation established strong evidence, identity, lifecycle,
security, and transport foundations. Evaluation against the developer question
“show me what plan the model came up with for the primary mission” exposed a
different class of gaps: the system is mechanically complete at a low level but
does not yet teach or expose the shortest reliable semantic path.

Three independently evaluated models reported the same broad pattern:

- full tool discovery consumed thousands of lines before the first evidence
  call;
- capability identifiers did not lead directly to the relevant tools;
- the trace acquisition and `traceId` to `artifactHandle` transition exposed
  Console lifecycle machinery and had to be learned through errors;
- record-type vocabulary and search scope were difficult to discover;
- important model-response and plan content was not exposed by the parsed tool
  path and required exact raw NDJSON reads;
- reference routing and canonical evidence paths were too implicit;
- rich frame and schema responses created avoidable context pressure; and
- client error behavior sometimes repeated the full tool catalog.

The first runtime-inspector design used “progressive disclosure” primarily to mean that a
large trace or payload should not be inserted into model context automatically.
That remains correct but incomplete. This roadmap adds **progressive
discovery**: the LLM must also be able to discover which evidence family,
operation, identifier, record type, content representation, and next drill-down
step applies without loading the full interface or recovering from predictable
mistakes.

The evaluations also exposed an implementation/design mismatch. The completed
design described reconstructed logical record `data` as readable through the
ordinary trace-payload surface and reserves raw-artifact reads for exact
storage-level investigation. The current implementation exposes bounded
payload content for envelope-backed records, while material data on records
such as `MODEL_RESPONSE_RECEIVED`, `PLAN_CREATED`, and `PLAN_UPDATED` may only
be reachable through raw record offsets. Ordinary semantic questions therefore
depend on the capability documented as optional and forensic. Resolving this
mismatch is the first correctness priority.

## Product principles

### Optimize for developer questions

Start with what a developer wants to understand, not with existing REST routes,
record structures, or MCP tool names. The workflow catalog determines the
evidence that must be reachable. It does not require one specialized server
tool per question or move diagnosis into Go.

Develop the natural-language question inventory collaboratively in
[`loomspan_skill_mcp_questions.md`](./loomspan_skill_mcp_questions.md) before
turning recurring communication needs into interface changes or evaluation
requirements.

### Minimize required inference

Do not optimize for a particular model's intelligence. Minimize the amount of
inference required to operate the interface correctly. A model should not need
to guess identifier transitions, invent enum values, infer content locations
from absent fields, or decode nested raw storage for an ordinary semantic
question.

This improves frontier-model efficiency and gives less capable models a fair
chance without creating separate surfaces or weakening the evidence contract.

### Preserve neutral, composable MCP primitives

The server continues to return recorded facts, deterministic calculations, and
explicit evidence relationships. The skill may teach canonical paths, but the
server does not diagnose, decide importance, or create one tool per workflow.
MCP remains independently usable when a client does not support Agent Skills.

### Teach a path without requiring it

Documenting a canonical happy path is not the same as making that call sequence
mandatory. Evaluations should accept any correct, safe, efficient sequence,
while skills and tool descriptions should show the shortest expected route and
the identifiers handed from one operation to the next.

### Make semantic content first-class and bounded

Plans, model exchanges, tool inputs and outputs, thoughts, structured outputs,
validation material, and other meaningful record data must be discoverable and
readable through the ordinary parsed trace capability. Small content may be
inlined only under explicit bounds. Large content must be represented by an
opaque, scope-bound content reference with exact length and continuation.

Raw offsets and raw-artifact reads remain precise low-level primitives for
storage, parser, unsupported-field, and exact-byte questions.

### Preserve evidence boundaries

Retain the successful parts of the first implementation:

- stable trace, session, frame, record, attempt, retry, failure, validation,
  continuation, and content identifiers at the layer where each is meaningful;
- frame, record, and lazily retrieved content separation;
- direct, descendant, inclusive, attributed, unattributed, and unavailable
  usage distinctions;
- independent protocol, capability, target, authentication, compatibility,
  liveness, evidence, and expiry facts;
- exact bounded reads and continuation;
- evidence/calculation/context/inference separation; and
- untrusted runtime content handling.

No usability enhancement should flatten these distinctions or create a hidden
aggregate diagnosis.

## Implemented baseline constraints

The completed runtime-inspector work established the following architecture and
contract boundaries. They remain in force unless this roadmap records an
explicit reconsideration prompted by a workflow that cannot otherwise be
completed safely and usefully.

### Ownership and adapter architecture

The product remains a matched experience consisting of a portable Agent Skill
and a local read-only MCP server hosted by the Go console:

```text
developer question
  -> IDE or agent LLM
     -> loomspan-runtime-debugging skill
     -> local Go Console MCP adapter
        -> transport-neutral Console query services
           -> selected Loomspan application
```

Responsibilities remain separated:

- the LLM performs contextual reasoning and may relate runtime evidence to
  separately authorized repository context;
- the skill supplies navigation, investigation guidance, evidence discipline,
  and answer guidance; and
- the MCP server supplies authenticated, structured, caller-directed,
  continuable evidence and deterministic calculations.

The skill is not a security boundary or an authoritative calculation engine.
Browser and MCP adapters remain peers over the same target context, skill,
active-execution, recent-activity, trace inventory/acquisition, artifact cache,
parser, indexes, calculations, query, continuation, and domain-error services.
Neither adapter creates a competing evidence model, independently contacts the
application, or changes shared fact meanings. Protocol wrappers and
presentation may differ; Loomspan identifiers, calculations, availability,
limitations, and error semantics do not.

### Plan identity and planning lineage

PR 27 established the producer contract that downstream Console, MCP, and skill
work must consume:

- `planId` is generated by the framework rather than supplied or controlled by
  the model;
- the same `planId` is recorded on `PLAN_CREATED` and every related
  `PLAN_UPDATED`;
- `PLAN_CREATED` records the accepting `attemptId` and planning
  `retrySequenceId`; and
- a rejected proposal remains model-response content associated with its
  attempt and produces no recorded plan state.

Frame attachment remains structural evidence rather than plan identity. Mission
and frame lineage selects the relevant `PLAN_CREATED`; its framework-owned
`planId` then establishes chain membership, and its accepting attempt/retry
fields join the accepted plan to planning history. Ordinary plan-quality errors
may still be accepted with `PLAN_QUALITY_WARNING` after retries are exhausted,
while deterministic evidence-coverage failure produces no `PLAN_CREATED`.

Artifacts produced before PR 27 may contain model-authored `planId` values and
may omit accepting-attempt fields. Mission lineage plus the old value is only a
degraded heuristic for those artifacts and must preserve ambiguity.

### Current capability and tool baseline

`LOOMSPAN_get_runtime` is the stable side-effect-free bootstrap operation. The
implemented named capability families and required tools are:

| Capability | Required tools |
| --- | --- |
| `loomspan.runtime-status.v1` | `LOOMSPAN_get_runtime` |
| `loomspan.skill-inspection.v1` | `LOOMSPAN_list_skills`, `LOOMSPAN_get_skill` |
| `loomspan.active-execution-inspection.v1` | `LOOMSPAN_list_executions`, `LOOMSPAN_get_execution` |
| `loomspan.recent-activity-inspection.v1` | `LOOMSPAN_get_execution_activity` |
| `loomspan.trace-inspection.v1` | `LOOMSPAN_list_traces`, `LOOMSPAN_get_trace`, `LOOMSPAN_query_trace_frames`, `LOOMSPAN_query_trace_records`, `LOOMSPAN_read_trace_payload` |
| `loomspan.raw-artifact-inspection.v1` | `LOOMSPAN_read_trace_artifact` |

A capability describes the installed server's semantic ability, not current
target readiness or evidence availability. Capability advertisement, the
current status snapshot, and an individual operation result are separate
layers. Missing capability, unsupported MCP protocol, target authentication,
`INCOMPATIBLE_TARGET`, evidence expiry, and operation failure must not be
collapsed into one health or compatibility state.

Loomspan has not released an alpha version of this surface. Until the first
release, capability identifiers describe a development contract and may be
corrected, expanded, renamed, or replaced without a compatibility shim or new
generation. The repository must still remain internally coherent: the skill,
server, tests, and documentation move together, and an advertised development
capability must match the operations and semantics present in that revision.

The first released version establishes the compatibility-governance baseline.
Before that release, record the rules for additive evolution, incompatible
semantic changes, generation changes, required operation membership, and
skill/server compatibility. Do not spend the pre-release design on preserving
an incomplete development-only contract, and do not claim compatibility with
an earlier unreleased revision. After the baseline release, capability changes
must follow those recorded rules deliberately.

Tools remain the complete portable investigation path. Remove `resourceUri`
and `resources` from tool results and remove the current custom MCP resource
templates; they duplicate callable operations and expose rejected ownership
and artifact mechanics. A future resource method must independently pass the
method test. No workflow may depend on resources, MCP prompts, sampling,
elicitation, or an MCP-hosted UI.

### Selected target, identity, and internal artifact lifecycle

MCP operates on the Console's one currently selected Loomspan target. Calls do
not accept arbitrary target URLs and cannot turn the Console into a network
proxy. Target selection, application credentials, compatibility, runtime
identity, and `targetScopeId` rotation remain owned below the MCP adapter. Those
internal identities must continue to prevent cross-target or cross-generation
evidence mixing, but `targetScopeId` and `instanceId` are not LLM routing
inputs or ordinary return fields. A changed target or runtime is reported as a
direct domain error or evidence limitation that tells the caller to restart.

`traceId` is the normal LLM-facing identity for discovery and all finalized
trace operations. Console internally resolves it to the correct owner,
installed artifact, lease, and immutable evidence instance. `artifactHandle`,
`source`, `TARGET`/`IMPORTED`, acquisition, cache, retention, and expiry remain
internal. A genuine collision between distinct evidence instances claiming the
same `traceId` must fail explicitly rather than silently select one; an
exceptional disambiguation design may be added only if real workflows require
one.

The browser and MCP continue to share the centralized artifact service and all
scope, cache, expiry, and cleanup guarantees. Upstream authentication failure
does not retroactively invalidate complete usable evidence already admitted to
Console. When internal availability prevents discovery or inspection, return a
compact domain-level limitation; do not expose storage lifecycle fields so the
LLM can reconstruct Console state.

### Live and finalized evidence

Active execution evidence is a bounded provisional snapshot and recent
activity is one bounded continuous upstream interval. Observation time, latest
sequence, beginning availability, gaps, and reset boundaries remain explicit.
A reset, stale cursor, changed instance, or target rotation prevents combining
events from opposite sides of a known discontinuity. MCP does not expose an
active trace tail or durable activity history.

Finalized trace inspection uses immutable acquired evidence, complete
addressability through bounded pages or ranges, and explicit continuations.
Each operation returns one finite result. Caller-selected bounds protect
interoperability and service stability; they are not hidden semantic filters or
cumulative evidence quotas.

### Read-only and untrusted-content boundary

The MCP surface is strictly observational. It does not invoke skills, cancel or
retry executions, edit configuration, change retention, mutate traces, enable
observability, select targets, reveal credentials, or perform arbitrary shell,
filesystem, or network operations.

Runtime strings never become tool calls, target URLs, filesystem locators,
commands, credentials, or configuration. The Go server performs only the
authenticated schema-constrained read requested by the caller. Returned
content cannot initiate another operation. Skill instructions reinforce this
boundary for the LLM but do not claim control over an IDE, model provider, or
content already returned to a client.

Recorded paths and mapping identifiers remain diagnostic text and search hints,
not local filesystem locators or deployment provenance. Repository correlation
is LLM reasoning over separately authorized context with explicit uncertainty.

### Error, compatibility, and representation boundaries

Valid tool calls preserve shared bounded domain errors with stable code, safe
message, and only details that change the caller's next decision. Target scope,
runtime instance, artifact, storage, and transport-routing details remain
internal. MCP negotiation,
framing, schema validation, access-key rejection, and disabled-server behavior
remain protocol or transport failures rather than Loomspan target errors.
Unexpected internal failures use sanitized `CONSOLE_ERROR` behavior and do not
expose stack traces, credentials, or internal paths.

The Java observability adapter and Go console remain a coordinated compatible
release pair. MCP clients use standard protocol negotiation and operation
discovery. Loomspan named capabilities govern semantic tool-family
compatibility; the skill package version is distribution metadata rather than
a runtime compatibility gate.

Structured results remain primary, with a concise useful text representation
for clients that do not expose structured content well. Every partial or
continuable result states the applicable range or cursor, whether additional
data from the same logical selection remains, and the evidence identity and
observation facts needed to continue safely.

### Continuing exclusions

This roadmap does not add remote or multi-user MCP, OAuth, control/write tools,
a full-runtime dump, continuous event injection into model context,
server-initiated model sampling, a Console-owned diagnostic LLM, automatic IDE
installation, multiple simultaneous selected targets, compliance audit
behavior, secret scanning or DLP, model-provider detection, or cross-version
historical trace reading. Any future control plane requires a separate design,
authorization model, explicit confirmation behavior, and threat assessment.

## Progressive discovery model

Progressive discovery has a skill-activation layer followed by five navigation
and evidence layers. A normal investigation should load only
the layers it needs.

### Layer 0 — Skill activation

The portable skill's name, frontmatter description, and trigger language must
cover general run understanding, plan evolution, model exchanges, tool data,
structured output, and the diagnostic workflows. A developer should not need to
name the skill explicitly for an ordinary question that clearly falls within
its scope.

Activation quality is part of evaluation. A perfect routing table provides no
value when the client never selects the skill.

### Layer 1 — Question routing

The top-level skill maps developer language to an investigation family and the
minimum relevant capability. Examples include trace overview, plan evolution,
model exchange, skill/tool path, failure, latency, usage, and exact evidence.

This layer should be short enough to load whenever the skill triggers.

### Layer 2 — Canonical evidence path

The selected workflow reference shows a short, non-mandatory happy path, such
as:

```text
identify trace
  -> inspect by traceId while Console resolves evidence internally
  -> select frame or record types
  -> read bounded semantic content if needed
```

It identifies required capabilities, starting identifiers, identifier handoffs,
stopping conditions, and the normal degraded path.

### Layer 3 — Evidence location and representation

The reference explains where the desired fact normally lives and how it is
represented. Examples:

- `PLAN_CREATED` and `PLAN_UPDATED` contain plan state and evolution;
- `MODEL_REQUEST_PREPARED`, `MODEL_REQUEST_SENT`, and
  `MODEL_RESPONSE_RECEIVED` describe a model exchange;
- frames supply hierarchy and aggregate calculations, while records supply
  ordered events;
- semantic content references expose bounded decoded content;
- raw record ranges expose exact NDJSON only when exact storage is relevant.

This is a map of the evidence model, not a duplicate of every output schema.

### Layer 4 — Compact tool mechanics

A focused guide lists each tool's one-line purpose, required arguments, the
small set of optional arguments that materially changes common investigations,
important output landmarks, and continuation behavior. It includes the
capability-to-tool map and teaches `traceId` as the normal trace identity
without exposing Console acquisition or artifact ownership.

### Layer 5 — Full schema and forensic detail

Full JSON Schemas, all optional filters, raw byte mechanics, parser behavior,
and unusual lifecycle/error details remain available when a difficult or exact
investigation requires them. They are not the entry cost for a normal question.

## Skill and MCP responsibility split

The skill owns:

- question-to-workflow routing;
- canonical but non-mandatory evidence paths;
- evidence-location and representation hints;
- capability selection for the chosen workflow;
- reasoning discipline, uncertainty, and stopping guidance;
- safe treatment of returned content; and
- concise answer expectations.

The MCP server owns:

- accurate names, descriptions, schemas, enums, and annotations;
- strict validation with focused errors;
- stable capability and identifier semantics;
- bounded neutral projections and content references;
- explicit search, pagination, range, encoding, and completeness semantics;
- structured results and concise interoperable text fallbacks; and
- independently usable behavior without the skill.

The skill must not manually restate every schema. High-value mechanical facts
that it does restate must be checked against authoritative server descriptors or
generated from a shared source so documentation drift is detected.

## Workstreams

### 1. Developer trace-understanding workflows

Use the companion workflow catalog to establish the complete set of developer
goals before selecting tool changes. Cover general understanding as well as
failure-oriented diagnosis. Link eventual fixtures, prompts, evaluations, and
tickets to the applicable workflow or requirement ID.

Treat trace identification as part of each workflow rather than free setup.
Establish bounded paths from the identifiers developers actually possess—such
as session ID, trace ID, skill name plus recency, approximate completion time,
or “the run I just performed”—to the selected trace. Measure inventory paging
and scanning as investigation cost. Decide from evidence whether documented
recent-first traversal is sufficient or server-side inventory filters are
required.

### 1A. LLM-facing interface cleanup baseline

Implement the accepted interface decisions recorded in the question
inventory before using live walkthroughs to select further changes. The
[cleanup ticket](../tickets/loomspan-mcp-llm-facing-trace-interface-cleanup.md)
is authoritative for this bounded implementation step:

- use `traceId` for finalized-trace operations while resolving ownership and
  installed evidence internally;
- remove `sourceFilter`, `source`, `artifactHandle`, `targetScopeId`, and
  `instanceId` from the LLM-facing contract;
- return compact trace candidates without catalog, availability, cache,
  acquisition, retention, expiration, or size fields;
- remove `resourceUri` and `resources` from tool results; and
- preserve internal safety through direct domain errors and limitations rather
  than exposed routing state.

Do not fold unresolved candidates—session identity, content-reference design,
pagination defaults, activity continuity, physical-record representation,
fallback duplication, or tool consolidation—into this cleanup merely because
the implementation touches nearby code.

### 2. Semantic content addressability

Extend the existing typed, opaque, scope-and-artifact-bound content-reference
mechanism to material content stored in ordinary record `data`. The current
reader already returns bounded reconstructed logical content for
envelope-backed payloads and failure diagnostics; the missing behavior is
coverage, not a new low-level range model. Ordinary record data includes some
model requests as well as model responses, plans, tool activity, advisor
mutations, structured output, validation content, and future record types.

Rename the outward pre-release contract around its general meaning:
`contentRef`, `LOOMSPAN_read_trace_content`, and `inlineContent` replace the
payload-only vocabulary. Prefer one coherent content abstraction over one
convenience tool per record type or workflow. Return the complete logical
record `data` JSON value neutrally rather than teaching the server to extract a
different semantic leaf for every record type. The LLM interprets model-authored
JSON or text inside that value; raw operations preserve the exact NDJSON
representation. Preserve the evidence distinction between an explicitly
recorded JSON `null` and an absent `data` member. Because physical representation
can vary per record instance even within one record type, expose content
availability on each returned record rather than teaching type-based location
rules.

Implementation research still determines whether record-data ranges re-decode
the bounded physical record, add a data-offset index, or materialize another
logical store representation. It must define stable length, content type,
continuation, JSON scalar/object/array/null behavior, UTF-8 boundaries, and the
cost of repeated reads before choosing the storage technique.

Required outcomes:

- ordinary semantic content is available under the development trace-inspection
  capability that will become part of the first release baseline;
- `loomspan.raw-artifact-inspection.v1` is genuinely optional for ordinary
  trace understanding;
- an inline-content request behaves consistently across qualifying record
  types or is replaced by a more accurately named contract, with both
  per-value and aggregate per-response bounds;
- decoded content type, encoding, length, range, and completeness are explicit;
  and
- raw addresses remain available without becoming the primary semantic route.

Plan evolution must be correlatable across frames. Representative evidence
places `PLAN_CREATED` on a planning frame and later `PLAN_UPDATED` records on
the corresponding root-mission frame. PR 27 fixed identity at the producer: the
existing `planId` is a framework-generated identity for an accepted plan and
all of its updates, and `PLAN_CREATED` records the accepting `attemptId` and
planning `retrySequenceId`. MCP and skill behavior must consume this contract.
Do not add a second `planInstanceId` or plan-chain identity.

Mission and frame lineage selects which `PLAN_CREATED` belongs to the primary
mission. After that selection, framework-owned `planId` establishes chain
membership across frame transitions, and the accepting attempt/retry fields
join the plan to validation history. Frame attachment remains structural
evidence rather than identity. The skill must teach that `ROOT_MISSION` recurs
for nested invocations and that the primary root comes from recorded trace roots
and parent relationships, not frame-type uniqueness or route-name matching.

The current representative feedback trace predates PR 27: its
`planId` is model-authored and its `PLAN_CREATED` lacks accepting-attempt fields.
Mission lineage plus that old value is only a degraded heuristic for such
artifacts and must preserve ambiguity.

### 3. Skill navigation and tool discovery

Revise the portable package around skill activation and the five progressive
discovery layers. Add:

- frontmatter name and description language that triggers for general trace
  understanding and semantic-content questions;
- a question-routing table near the top of `SKILL.md`;
- workflow-specific capability checks rather than a uniform five-capability
  ceremony;
- a capability-to-tool map;
- compact required-argument tables;
- canonical trace acquisition and identifier handoffs;
- record and frame vocabulary needed by common workflows;
- explicit content-location guidance; and
- deterministic links from each question family to the appropriate reference
  section.

Measure the top-level skill body and the references loaded before the first
evidence call. The entry layer must remain small enough that using the skill is
cheaper than rediscovering the interface through schemas and failed calls.

Runtime discovery remains the normal start of a fresh live investigation, but
the skill checks only the capability families needed by the selected workflow.
A missing unrelated capability does not block a permitted trace read.

### 4. Tool and schema efficiency

Measure and reduce interface cost without weakening machine-readable contracts.
Investigate:

- total `tools/list` bytes and tokens;
- repeated output/error schema definitions;
- whether schemas can share definitions or omit low-value repetition;
- concise descriptions that contain the identifier handoff and content
  semantics agents otherwise guess;
- compact structured and text projections;
- validation errors that show the failed tool and relevant input contract; and
- the ownership of clients that append or redisplay the full tool catalog after
  a failed call.

A new compact-discovery tool is not automatically useful because an agent must
first discover it. Prefer a skill entry layer, standard tool metadata, or a
small resource that clients can reach without loading the full catalog.

### 5. Search, vocabulary, and projection clarity

Make structured filters the reliable route for structural questions. Expose
record-type values as the schema item enum for `filter.types` and in the skill
guide, derived from or checked against the authoritative parser vocabulary. For literal search,
state the query semantics once at page/result-envelope level: which fields were
searched, whether matching is case-sensitive, which match mode was used, and
whether referenced content participated. This metadata is mandatory on an
empty result so “not searched” cannot be mistaken for “not present.” Individual
matches may retain their field and offset without repeating the query contract.

The current record filter searches case-sensitive JSON-encoded metadata/data
bytes rather than decoded semantic content and excludes reconstructed payloads.
Research whether to preserve that behavior as an explicitly named encoded mode
or add decoded semantic-content search; do not leave the representation
implicit.

Consider a compact frame tree or summary projection containing only identity,
parent, type, route, outcome, duration, and other deliberately selected
landmarks. Rich usage, attempt, retry, validation, and failure blocks remain
available through detailed projection or targeted queries.

Clarify range `hasMore` semantics so a caller can distinguish “more bytes exist
in the artifact or content” from “the selected logical record is incomplete.”
Ensure large text fallbacks remain recoverable when a client truncates or saves
them.

### 6. Cross-model evaluation

Evaluate interface complexity rather than attempting to certify model
intelligence. Use the model roles, correctness dimensions,
interaction-complexity measures, repeat expectations, and initial acceptance
scenario defined by the companion workflow catalog. Do not create
model-specific skill forks, alternate tool surfaces, or weaker evidence
semantics.

### 7. Contract coherence and regression prevention

Add checks that keep skill claims, MCP capabilities, tool schemas, and runtime
behavior aligned. Candidate checked facts include:

- capability-to-tool membership;
- tool names and required arguments;
- record and frame type vocabularies;
- supported filters and search scope;
- content-reference and inline-content behavior;
- raw capability degradation; and
- canonical workflow prompts reaching the expected evidence without relying on
  exact answer prose.

## Feedback classification

| Observation | Current classification | Roadmap response |
| --- | --- | --- |
| Full schema discovery is very large | Confirmed usability gap; exact cost must be measured | Compact skill mechanics, schema-size audit, client evaluation |
| Capability IDs do not map to tools in the skill | Confirmed documentation gap | Capability-to-tool map and workflow-specific checks |
| `inlinePayload` does not expose model responses or plans | Confirmed semantic contract gap | General bounded record-content design |
| “What did the model produce?” requires raw NDJSON | Confirmed workflow blocker | Make semantic content part of trace inspection |
| Question-to-reference routing is unclear | Confirmed skill navigation gap | Top-level routing table and deterministic reference links |
| Bootstrap checks every capability family | Confirmed proportionality gap | Check only workflow dependencies after runtime discovery |
| Literal search missed plan records | Confirmed discoverability/correctness risk | Expose type enums and explicit search scope/case semantics |
| `traceId` to `artifactHandle` was learned through errors | Confirmed leakage of Console acquisition mechanics | Use `traceId` throughout the LLM contract and preserve evidence ownership internally |
| Failed calls redisplay the full catalog | Client-dependent until reproduced at the protocol boundary | Attribute ownership, then reduce server-controlled repetition where useful |
| Full frame results are expensive for tree discovery | Confirmed projection gap | Evaluate compact tree/summary projection |
| Single-line truncated JSON is difficult to recover | Client and fallback-format interoperability gap | Evaluate concise/line-oriented fallback and structured-result behavior |
| Range `hasMore` was read as logical incompleteness | Confirmed semantics/documentation gap | Make selected-content versus backing-artifact completeness explicit |
| Runtime text differs from other structured results | Minor interoperability inconsistency | Normalize or document concise text fallback behavior |
| The current skill description may not trigger for plans or model/tool content | Confirmed activation gap | Add Layer 0 skill scope and activation evaluation |
| “I just ran this skill” has no bounded trace-identification contract | Confirmed workflow gap | Measure identification cost and evaluate inventory filters |
| Per-value inline bounds can accumulate across a record page | Confirmed response-bound and exposure risk | Add an aggregate per-response inline budget |
| Some prepared/sent model requests use ordinary `data` rather than chunked payloads | Confirmed semantic-content gap | Apply the general record-data contract to both physical representations |
| Plan identity was model-authored, plan records relied on frame-placement behavior, and `PLAN_CREATED` omitted its accepting attempt | Producer gap resolved by PR 27; MCP/skill consumption remains | Expose and teach framework-owned `planId` plus accepting `attemptId`/`retrySequenceId` |

## Evaluation authority

The companion workflow catalog is the single authority for evaluation
dimensions, model roles, call accounting, and the initial `handleIncident`
acceptance scenario. Roadmap stages and completion decisions consume those
definitions rather than duplicating them. Evaluation measurements are
diagnostic signals, not incentives to omit necessary evidence; a correct broad
investigation may legitimately cost more than a direct content lookup.

## Delivery sequence

### Stage 1 — Workflow and baseline evidence

- Approve the companion workflow catalog.
- Implement the accepted LLM-facing cleanup ticket so walkthroughs begin from
  the intended `traceId`-based contract rather than known storage leakage.
- Reproduce representative workflows against the protocol boundary and
  selected clients after that cleanup.
- Record post-cleanup interaction-complexity measurements and retain the
  current implementation only as the historical comparison baseline.
- Identify the exact design/implementation divergences that block ordinary
  workflows.
- Derive a sanitized synthetic fixture preserving the observed nested primary
  and child plan topology, validation/retry cycle, cross-frame plan updates, and
  mixed envelope/ordinary request representations without copying sensitive
  trace content. Add an adversarial same-skill recursion case, identical or
  adversarial model proposal content that still yields distinct framework plan
  IDs, an unrecorded rejected proposal, and enough plan versions to force
  deterministic inline omission.

### Stage 2 — Semantic content path

- Extend and rename the existing bounded content-reference contract for
  ordinary record data.
- Implement it through shared trace-analysis services and MCP.
- Preserve browser/MCP fact parity where they share the same service.
- Prove ordinary plan and model-output retrieval without raw capability.
- Expose sufficient `planId` facts or relationships to reconstruct complete
  plan evolution across planning and root-mission frames using the
  framework-owned producer identity.
- Expose the accepting `attemptId` and planning `retrySequenceId` recorded on
  `PLAN_CREATED` so validation history does not require frame/order inference.
- Make descriptor-first followed by targeted content reads an explicit bounded
  success path when aggregate limits prevent the final or material value from
  being inlined.
- Distinguish no recorded plan from a rejected proposal found only in model
  response content.

### Stage 3 — Progressive skill discovery

- Restructure the canonical skill around question routing and progressive
  references.
- Add canonical paths, capability maps, mechanics, evidence locations, and
  degraded paths.
- Add drift checks against authoritative MCP descriptors.

### Stage 4 — Efficiency and clarity

- Improve structured filter vocabulary and search semantics.
- Add compact structural projection if workflow evidence justifies it.
- Reduce schema and fallback-output cost where compatible.
- Clarify continuation and completeness semantics.

### Stage 5 — Representative validation

- Run the primary frontier-model/client matrix on representative workflows.
- Run available interface-complexity canaries.
- Compare correctness, calls, failures, and context cost with the Stage 1
  baseline.
- Publish observed compatibility evidence without claiming untested support or
  model guarantees.

## Completion criteria

This roadmap is complete when:

1. the approved LLM workflow catalog covers general trace understanding as
   well as failure, latency, usage, and unfamiliar-path investigation;
2. each ordinary workflow has a discoverable canonical path, required
   capability family, evidence-location map, degraded behavior, and stopping
   condition;
3. the skill activates for general trace-understanding and semantic-content
   questions without requiring the developer to name it explicitly where the
   client supports automatic skill selection;
4. trace identification from common developer-supplied context is bounded,
   documented, and included in interaction measurements;
5. material semantic record content is bounded and addressable through parsed
   trace inspection;
6. the first-release outward contract uses general content vocabulary rather
   than retaining payload-only names for record data and diagnostics;
7. raw artifact access is not required for ordinary plan, model, tool,
   structured-output, or validation questions;
8. record vocabulary, search scope, developer-meaningful identifiers, encoding, and
   completeness are explicit;
9. inline content has deterministic per-value and aggregate response bounds;
10. the skill progressively reveals mechanics without duplicating the full MCP
   schemas or drifting from them;
11. MCP remains independently usable and neutral;
12. representative evaluations show materially fewer discovery bytes, failed
   calls, raw reads, and manual decoding steps without reducing correctness;
13. primary frontier models succeed consistently on representative workflows;
14. available canary models have a reasonable path to success when no product
    principle or evidence guarantee must be weakened; and
15. retained strengths in identity, evidence boundaries, lifecycle, security,
    bounded reads, and uncertainty remain covered by regression tests.

Capability compatibility governance is a gate for the first release, not for
pre-alpha design iterations under this roadmap.

## Non-goals

- Server-generated diagnosis, importance, correctness, or remediation advice
- One MCP tool per workflow, record type, model provider, or question wording
- Automatic loading of complete traces or arbitrarily large content
- Model-specific skills or protocol surfaces
- Guaranteeing the behavior of an IDE, MCP host, or language model
- Hiding evidence gaps, lifecycle failures, or unknown usage to reduce tokens
- Replacing exact raw-artifact access
- Treating recorded `sourcePath` values as local filesystem or deployment
  provenance
- Expanding the supported Loomspan Java API or introducing a Java SPI

## Planning handoff

The next context should:

1. implement the
   [LLM-facing MCP trace interface cleanup ticket](../tickets/loomspan-mcp-llm-facing-trace-interface-cleanup.md)
   without absorbing unresolved candidates;
2. connect a representative LLM client to the resulting MCP server and walk
   through successful, failed/retried, imported, active, unavailable, and large
   content traces;
3. record tool discovery, unnecessary calls, ignored fields, error recovery,
   and approximate context cost in the question inventory and evaluation
   evidence;
4. decide the `sessionId`, content-reference, pagination, activity-continuity,
   physical-record, text-fallback, and tool/resource candidates from observed
   workflow friction;
5. research the smallest correct storage/index implementation for ordinary
   semantic record-data ranges; and
6. create later tickets only for independently reviewable outcomes supported
   by those walkthroughs.
