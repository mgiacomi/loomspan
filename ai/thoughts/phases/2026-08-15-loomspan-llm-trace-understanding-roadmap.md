# Loomspan LLM Trace Understanding Roadmap

## Status

Active roadmap, last updated 2026-08-21. This is the only active phase document
for Loomspan Console, MCP, and the portable runtime-debugging skill.

Completed design and implementation history has been removed from the active
roadmap. Git history, current product documentation, schemas, tests, fixtures,
and evaluation cases remain the authorities for delivered behavior. This file
contains only outstanding product work and intentionally deferred work.

The Console and MCP contracts remain unreleased. Until the v1 contract gate,
change them in place and keep the server, browser, skill, callers, tests,
fixtures, and documentation aligned. The supported Java API remains governed
separately by its closed allowlist.

## Target outcome

Before v1, a developer should be able to ask a representative LLM an ordinary
single-run question, have the applicable skill and MCP evidence path discovered
without avoidable schema archaeology, and receive a concise evidence-backed
answer with honest limitations. The remaining work must preserve neutral MCP
primitives, bounded disclosure, exact evidence access, untrusted-data handling,
and the distinction between evidence, calculation, context, and inference.

## 1. Complete progressive skill routing

The portable skill still emphasizes the original failure, latency, usage, and
nested-path playbooks. Give its always-loaded entry point a compact route for
all supported single-run question families:

| Developer goal | Evidence route |
| --- | --- |
| Explain what happened | Trace overview and compact frame orientation |
| Explain the created or final plan | Plan lineage, plan records, selected semantic content |
| Inspect what a model received or produced | Model request/response records and selected semantic content |
| Explain skills, tools, and their data flow | Frame hierarchy, tool/step records, selected skill YAML or content |
| Explain failure, recovery, retry, or validation | Terminal and recovered failures, attempts, retries, validations, diagnostics |
| Explain latency | Active evidence or finalized detailed frame durations and gaps |
| Explain usage | Trace, frame, attempt, retry, and unattributed usage facts |
| Inspect exact storage/parser evidence | Optional raw-artifact range reads |

Required work:

- Add a small question-routing table or equivalent direct routing guidance to
  the canonical `SKILL.md`.
- Check only the capability families needed by the selected workflow after
  runtime discovery; a missing unrelated capability must not block a permitted
  read.
- Add a concise capability-to-tool map and required identifier/argument
  handoffs without copying full schemas into the skill.
- Link each question family directly to the focused canonical path, degraded
  behavior, and stopping condition it needs.
- Preserve descriptor-first record inspection and use inline content only for
  narrow queries or deliberately small pages.
- Test automatic skill activation where the host supports it, including
  prompts that do not explicitly name the skill.
- Add drift checks for duplicated mechanical facts that are not already
  checked against authoritative tool descriptors.

Completion evidence:

- Every question family above has a short discoverable path and stopping rule.
- A missing optional or unrelated capability degrades only the dependent path.
- Skill-assisted runs do not require raw NDJSON for ordinary semantic content.
- The entry point remains materially smaller than the complete tool schemas.

## 2. Settle trace identification and lifecycle handoffs

PR 34 completed the bounded active-execution review and completion-race path:
one complete active page, one bounded activity call per selected session,
reusable future checkpoints, and one trace-resolution attempt by the already
returned `traceId`. Active disappearance does not prove finalized evidence,
and `TRACE_UNAVAILABLE` does not trigger unrelated inventory scanning. Durable
monitoring, history, and cross-run comparison remain outside this roadmap item.

The remaining interface-design question is how the identifiers developers
actually possess lead to one selected `traceId`. Do not add another method or
field unless measured workflow friction shows that existing inventory filters,
ordering, active-execution facts, and conversation context are insufficient.

Decisions still required:

1. Define the supported `sessionId` to `traceId` cardinality and handoff.
   Determine whether one session can ever identify multiple finalized traces.
2. Define “most recent skill run,” including whether it means entry skill by
   default, how active and finalized runs participate, and when ambiguity must
   be shown to the developer.
3. Set a bounded inventory-traversal and clarification rule for approximate
   time, outcome, skill name, or “the run I just performed.”
4. Decide whether browser-selected or manually imported evidence is visible to
   MCP. Any solution must define client/session ownership and avoid a
   process-global mutable current-trace race.
5. Decide whether local NDJSON import belongs to portable MCP, remains a
   browser or separately authorized client action, or is deliberately outside
   v1.
6. Decide which application correlation identifiers, if any, are common and
   stable enough to expose structurally.
7. Decide whether v1 requires bounded cross-trace semantic search. Prefer
   selecting candidate traces before within-trace content search unless
   concrete evidence proves that insufficient.

Completion evidence:

- Exact trace, copied session, named-skill recency, approximate-time, active
  completion, imported trace, and ambiguous-selection walkthroughs have one
  documented bounded behavior each.
- Trace-identification calls, pagination, scanning, and failed selections are
  included in evaluation cost.
- No workflow guesses a browser selection, silently scans an unbounded
  inventory, or treats `sessionId` and `traceId` as interchangeable.

## 3. Establish the pre-v1 evaluation and regression baseline

PR 31 provides one sanitized current-contract trace and tools-only and
skill-assisted cases. Expand this into a small maintainable pre-v1 baseline;
do not build a large frozen-format corpus while the contract and NDJSON format
are still changing.

Required work:

- Cover representative successful, failed/recovered, retried, imported,
  active, unavailable, and large-content investigations across the question
  families above.
- Record tool calls, failed calls, discovery bytes, maximum and total result
  sizes, continuation use, raw reads, client overflow/externalization, and
  material semantic errors.
- Select and document the primary model/client release runs and the cheaper or
  less capable canaries. Use repeat runs where needed to distinguish systematic
  interface friction from model variance.
- Exercise both explicit skill invocation and automatic activation when the
  client exposes skill discovery.
- Keep tools-only cases proving that MCP remains independently usable.
- Keep fixtures sanitized and current with intentional pre-v1 contract
  changes. Review fixture diffs as contract changes instead of preserving
  compatibility with obsolete development formats.
- Define which results are release gates, compatibility observations, and
  diagnostic canaries. Do not claim support for untested hosts or models.

Completion evidence:

- Representative single-run workflows succeed without ordinary raw-artifact
  reads, manual NDJSON decoding, predictable schema failures, or false
  completeness claims.
- Skill-assisted runs reduce avoidable discovery and failed-call cost without
  weakening evidence or correctness.
- Results and measurements are checked in or reproducibly generated rather
  than existing only in chat transcripts.

After the v1 trace contract and file format stabilize, grow this baseline into
the broader bank of real-shape sanitized trace files used by the long-term
regression suite.

## 4. Pass the v1 contract and compatibility gate

Before declaring the first released MCP/skill contract:

- Close or explicitly defer every decision in this roadmap.
- Freeze the intended capability identifiers, tool names, schemas, defaults,
  limits, continuation behavior, content vocabulary, and error meanings.
- Define compatibility governance for future MCP and skill evolution rather
  than carrying forward the current pre-release change-in-place rule.
- Confirm server descriptions, structured output, text fallbacks, browser
  facts, skill guidance, fixtures, and user documentation agree.
- Run the complete Go, browser, MCP contract, skill packaging, evaluation, and
  applicable Java public-surface verification suites.
- Publish only the client/model compatibility evidence actually observed.

The gate is complete when the single-run experience is bounded, discoverable,
independently usable without the skill, materially easier with the skill, and
covered by a maintainable regression baseline.

## Intentionally deferred until after v1

### Cross-run comparison

Questions such as “why did this run differ from yesterday’s?” require a
separate design for selecting multiple runs, aligning evidence, handling
different trace versions and scopes, and separating mechanical differences
from causal inference. Do not imply a supported comparison workflow through
single-run tools.

### Large trace corpus

Keep only the small representative pre-v1 fixture set necessary to protect the
current contract. Build the larger regression bank after the trace contract and
NDJSON format stabilize so development-format churn does not dominate upkeep.

### Speculative interface expansion

Do not add cross-trace semantic search, application-specific correlation
fields, browser-selection state, import operations, or specialized workflow
tools until the identification decisions above and measured workflow evidence
justify them.

## Recommended sequence

1. Finish progressive skill routing.
2. Run trace-identification walkthroughs and settle the lifecycle decisions.
3. Establish and record the small pre-v1 model/client regression baseline.
4. Apply the v1 contract and compatibility gate.
5. Design cross-run comparison and expand the trace corpus after v1.
