# Loomspan LLM Trace Understanding Roadmap

## Status

Active product roadmap as of 2026-08-15.

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
- the trace acquisition and `traceId` to `artifactHandle` transition had to be
  learned through errors;
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

- stable scope, trace, session, frame, record, attempt, retry, failure,
  validation, artifact, continuation, and content identifiers;
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

Capabilities evolve additively under the same generation where possible. An
incompatible semantic change requires a new generation. A server must not
advertise a capability unless its complete required operation family and
semantics are present. This roadmap may add an operation to restore or clarify
the promised parsed trace semantics, but ticket planning must decide the
compatible capability-governance treatment explicitly.

Tools remain the complete portable investigation path. MCP resources may
provide stable, relatively small materialized views and useful links, but are
supplementary because client resource support varies. Do not make a workflow
depend on resources, MCP prompts, sampling, elicitation, or an MCP-hosted UI.

### Selected target, identity, and artifact lifecycle

MCP operates on the Console's one currently selected Loomspan target. Calls do
not accept arbitrary target URLs and cannot turn the Console into a network
proxy. Target selection, application credentials, compatibility, runtime
identity, and `targetScopeId` rotation remain owned below the MCP adapter.

Every target-specific result preserves `targetScopeId`, applicable `instanceId`,
stable evidence identifiers, and observation time. Prior-scope handles,
resources, and continuations fail with `TARGET_CHANGED`; the server does not
remap them. The LLM must not combine different scopes unless the developer
explicitly asks for a comparison.

`traceId` identifies application-catalog evidence. Acquiring or reopening a
trace produces a scope-bound immutable `artifactHandle`. Downstream frame,
record, content, payload, and raw operations use the installed artifact handle
so they cannot silently cross acquisitions or changing catalog state. The
browser and MCP share the same centralized artifact service, installed copy,
handle, cache policy, expiry, and cleanup lifecycle.

Application trace availability, execution outcome, installed artifact
availability, and target authentication remain independent. Upstream
authentication failure blocks new acquisition but does not retroactively
invalidate a complete current-scope copy already admitted into the Console.
Artifact or scope expiry ends retrieval explicitly rather than silently
filtering evidence.

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
message, optional target scope, and permitted details. MCP negotiation,
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

Progressive discovery has five layers. A normal investigation should load only
the layers it needs.

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
  -> acquire or reopen artifact
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
capability-to-tool map and the `traceId` to `artifactHandle` lifecycle.

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

### 2. Semantic content addressability

Design one general bounded mechanism for material record content. It should
cover model responses and plans immediately and be extensible to requests,
tool exchanges, thoughts, structured output, validation content, and future
record types.

Prefer a coherent content-reference abstraction over one convenience tool per
record type or workflow. Determine whether existing payload references can be
generalized compatibly, whether a general content-read operation is required,
and how small explicit inlining behaves.

Required outcomes:

- ordinary semantic content is available under
  `loomspan.trace-inspection.v1`;
- `loomspan.raw-artifact-inspection.v1` is genuinely optional for ordinary
  trace understanding;
- an inline-content request behaves consistently across qualifying record
  types or is replaced by a more accurately named contract;
- decoded content type, encoding, length, range, and completeness are explicit;
  and
- raw addresses remain available without becoming the primary semantic route.

### 3. Skill navigation and tool discovery

Revise the portable package around the five discovery layers. Add:

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
record-type values through the schema and skill guide. For literal search,
state at result level which fields were searched, whether matching is
case-sensitive, and which match mode was used so “not searched” cannot be
mistaken for “not present.”

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
intelligence. The primary representative matrix emphasizes the clients'
expected frontier models, initially:

- Opus;
- Sol;
- Gemini Pro; and
- Kimi K3.

Use additional models such as GLM 5.2, Kimi 2.7, GPT Terra, and DeepSeek as
interface-complexity canaries when available. They need not all be hard release
gates or run every workflow. Repeated canary failure should trigger an interface
review when simplifying the path would not degrade correctness, composability,
or frontier-model performance.

Do not create model-specific skill forks, alternate tool surfaces, or weaker
evidence semantics.

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
| `traceId` to `artifactHandle` was learned through errors | Confirmed mechanics gap; lifecycle itself remains valuable | Teach acquisition handoff; do not automatically erase evidence ownership |
| Failed calls redisplay the full catalog | Client-dependent until reproduced at the protocol boundary | Attribute ownership, then reduce server-controlled repetition where useful |
| Full frame results are expensive for tree discovery | Confirmed projection gap | Evaluate compact tree/summary projection |
| Single-line truncated JSON is difficult to recover | Client and fallback-format interoperability gap | Evaluate concise/line-oriented fallback and structured-result behavior |
| Range `hasMore` was read as logical incompleteness | Confirmed semantics/documentation gap | Make selected-content versus backing-artifact completeness explicit |
| Runtime text differs from other structured results | Minor interoperability inconsistency | Normalize or document concise text fallback behavior |

## Interaction-complexity measures

Record at least these values for representative evaluations:

- tool-discovery bytes or estimated tokens before the first evidence call;
- number of tool calls and failed calls;
- number of required identifier handoffs;
- largest individual result and total result bytes;
- whether raw-artifact capability was required;
- whether manual NDJSON or nested JSON decoding was required;
- whether the answer found all material matching records;
- factual grounding and stable identifier citation;
- correct evidence/calculation/context/inference separation;
- correct degraded-capability and evidence-availability behavior; and
- consistency across the selected repeat policy.

These are diagnostic measurements, not incentives to omit necessary evidence.
A correct broad investigation may legitimately cost more than a direct content
lookup.

## Initial benchmark

Use the prompt that exposed the current gap as the first end-to-end benchmark:

> I ran `handleIncident`. Pull up the trace and show me what plan the model
> ended up coming up with for the primary mission. Use the
> `loomspan-runtime-debugging` skill.

For a trace that is available and compatible, the intended steady-state path
has:

- no bulk tool-catalog inspection;
- no predictable argument-recovery call;
- no raw-artifact capability or raw NDJSON decoding;
- no invented record types or search behavior;
- at most one runtime/discovery call for a fresh investigation;
- no more than three evidence calls after the trace is identified for the
  ordinary small-content case;
- complete relevant plan state or evolution within explicit bounds; and
- stable evidence identifiers and appropriate uncertainty in the answer.

The call-count target is a design signal, not a rule that permits skipped
evidence or an incorrect answer.

## Delivery sequence

### Stage 1 — Workflow and baseline evidence

- Approve the companion workflow catalog.
- Reproduce representative feedback against the protocol boundary and selected
  clients.
- Record current interaction-complexity measurements.
- Identify the exact design/implementation divergences that block ordinary
  workflows.

### Stage 2 — Semantic content path

- Settle the general bounded record-content contract.
- Implement it through shared trace-analysis services and MCP.
- Preserve browser/MCP fact parity where they share the same service.
- Prove ordinary plan and model-output retrieval without raw capability.

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
3. material semantic record content is bounded and addressable through parsed
   trace inspection;
4. raw artifact access is not required for ordinary plan, model, tool,
   structured-output, or validation questions;
5. record vocabulary, search scope, identifier handoffs, encoding, and
   completeness are explicit;
6. the skill progressively reveals mechanics without duplicating the full MCP
   schemas or drifting from them;
7. MCP remains independently usable and neutral;
8. representative evaluations show materially fewer discovery bytes, failed
   calls, raw reads, and manual decoding steps without reducing correctness;
9. primary frontier models succeed consistently on representative workflows;
10. available canary models have a reasonable path to success when no product
    principle or evidence guarantee must be weakened; and
11. retained strengths in identity, evidence boundaries, lifecycle, security,
    bounded reads, and uncertainty remain covered by regression tests.

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

Before creating implementation tickets:

1. review and approve the workflow boundaries in the companion catalog;
2. research the current trace record and payload storage model to select the
   smallest coherent semantic-content abstraction;
3. capture actual MCP `tools/list`, structured result, text fallback, and
   validation-error behavior in representative clients;
4. decide which skill reference facts are generated and which are checked by
   conformance tests;
5. choose initial projection and search changes based on workflow evidence;
6. define the evaluation matrix, repeat policy, and release-gate versus canary
   distinction; and
7. split tickets by independently reviewable outcomes rather than by document
   section or individual model complaint.
