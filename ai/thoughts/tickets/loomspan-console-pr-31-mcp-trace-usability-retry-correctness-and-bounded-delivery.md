# PR 31 — MCP Trace Usability, Retry Correctness, and Bounded Delivery

## Status

Proposed standalone implementation ticket. The completed PR 30 ticket has been
removed from the working tree, so this brief contains the context and decisions
needed to implement the follow-up without consulting that ticket or the
conversation that produced this one.

This ticket is based on two external model walkthroughs performed with GLM 5.2
and Kimi 2.7. Both walkthroughs occurred before the final PR 30 code review and
its corrections. Reproduce each reported issue on the current branch before
changing behavior, but do not dismiss a usability failure merely because the
current implementation already contains an undiscovered option.

## Outcome

Make finalized-trace inspection correct and usable by an MCP consumer that has
only the installed tool descriptions and schemas, while retaining the Agent
Skill as the richer investigation guide.

The finished contract must:

- count actual retries rather than retry sequences;
- make projection-dependent omissions and content-reading options discoverable;
- return complete bounded responses with explicit pagination instead of
  silently truncating or relying on a client overflow file;
- distinguish trace finalization, evidence acquisition, and import times;
- reduce unnecessary content-read round trips without making response size
  unpredictable; and
- preserve exact evidence, read-only behavior, opaque references, evidence
  completeness, and existing security boundaries.

## Why this ticket exists

### GLM 5.2 walkthrough

GLM reported that:

- `tools/list` was large enough to be moved to an overflow file;
- useful trace calls frequently exceeded the host's visible output budget;
- it abandoned the semantic workflow and paged through raw NDJSON to retrieve
  model content;
- it did not discover the intended inline-content path;
- timestamp and duration meanings were unclear;
- warnings were buried in the record stream; and
- literal search and opaque pagination felt harder than dumping and grepping
  the artifact.

Some claims were factually wrong. In particular, the trace record timestamp is
an epoch timestamp, not a monotonic clock, and `LOOMSPAN_get_trace` already
contains structured usage summaries. Those incorrect conclusions are still
contract evidence: the model could not discover or confidently interpret facts
that were present.

### Kimi 2.7 walkthrough

Kimi followed the semantic workflow more successfully but reported that:

- descriptor-first records caused an N+1 sequence of content reads because it
  missed `inlineContent`;
- compact frames showed no usable durations and did not explain that DETAILED
  is required;
- `attemptCount=10` and `retryCount=10` looked like ten retries even though all
  ten model calls succeeded on their initial attempt;
- zero `PLAN_RETRY_REQUESTED` records appeared to contradict the retry count;
- `acquiredAt` occurring after `finalizedAt` looked chronologically invalid;
- the intended workflow was learned primarily from the Agent Skill, not from
  the MCP contract itself;
- the first content read unnecessarily required `start: 0`; and
- plural `outcomes` and opaque content references lacked enough immediately
  visible semantic explanation.

Kimi also confirmed that runtime capability discovery, `complete`, `hasMore`,
limitations, routes, frame parentage, read-only wording, and the closed record
type enumeration worked well. Preserve those strengths.

### Shared diagnosis

The common failure mode is not inability to understand trace identity or frame
structure. It is reliance on inference for hidden switches, projections, and
nearby but distinct concepts. A consumer should not need to infer:

- that `inlineContent` exists;
- that COMPACT intentionally removes duration and usage values;
- whether a retry count includes the initial attempt;
- whether `PLAN_RETRY_REQUESTED` is a model/provider retry event;
- why acquisition can occur after trace finalization; or
- whether missing output was omitted by Loomspan, paged by Loomspan, or
  truncated later by the MCP client.

## Current contract facts that must remain true

- The portable trace workflow is:

  ```text
  LOOMSPAN_list_traces
    -> LOOMSPAN_get_trace
    -> LOOMSPAN_query_trace_frames
    -> LOOMSPAN_query_trace_records
    -> LOOMSPAN_read_trace_content
  ```

- `LOOMSPAN_read_trace_artifact` remains an optional exact-forensics escape
  hatch, not the normal way to inspect model requests and responses.
- Trace content descriptors are safe, opaque, current-process references.
  Consumers must not infer identity, authority, storage location, or offsets
  from their encoded value.
- Continuations remain opaque and bound to their operation, state, and owner.
- Exact content and artifact reads report source-byte offsets and total length.
- Oversized explicit range requests fail with `LIMIT_EXCEEDED`; a successful
  response is never silently shortened to the maximum.
- Inlined content is bounded per value and across the response. An omitted
  inline value retains its descriptor and an explicit omission reason.
- Every tool returns exactly one structured success or domain-error envelope
  and a deterministic, fact-complete text fallback for clients that do not
  consume structured output.
- Trace evidence remains inert diagnostic data. No returned trace, content, or
  skill text is interpreted as an instruction by the server.
- No custom MCP resources or resource templates are introduced.

## Terminology and invariants

### Attempt

One physical model/provider attempt identified by `attemptId` within a
`retrySequenceId`. Its validated `attemptNumber` starts at 1.

### Retry sequence

The group containing an initial attempt and any later attempts made for the
same retryable operation. A retry sequence exists even when its initial attempt
succeeds and no retry occurs.

### Retry

An attempt after the initial attempt in its retry sequence. The initial attempt
is never a retry.

For each retry sequence:

```text
retry count = max(0, number of validated attempts in the sequence - 1)
```

For a whole trace:

```text
retryCount = sum of the retry counts of all retry sequences
```

Examples:

| Retry sequences | Attempt count | Retry count |
| --- | ---: | ---: |
| one sequence with one attempt | 1 | 0 |
| one sequence with two attempts | 2 | 1 |
| one sequence with three attempts | 3 | 2 |
| ten sequences with one attempt each | 10 | 0 |
| two sequences with 1 and 3 attempts | 4 | 2 |

`PLAN_RETRY_REQUESTED` is a planning-quality workflow record. Its count is not
the trace's provider/model retry count and it must not be documented as such.

### Finalized time

When the traced execution produced its terminal trace fact. This is a property
of the trace execution and can predate Console inspection.

### Acquired time

When Console completed acquiring and installing a local evidence copy. It can
legitimately be later than `finalizedAt`.

### Imported time

When imported evidence entered Console through the import workflow. It is
independent of both execution finalization and target evidence acquisition.

### Truncation

Silent truncation means returning a successful response that drops requested
bytes, items, fields, fallback facts, or descriptions without an explicit
machine-readable indication and recovery path. Loomspan must never do this.

Pagination is not truncation when the response clearly includes `hasMore`, an
opaque continuation, exact range boundaries where applicable, and enough
context to resume the same operation safely.

An MCP host may independently collapse, truncate, or move a response to an
overflow file after Loomspan returns it. Loomspan cannot guarantee host UI
behavior. It must nevertheless shape defaults and bounded pages so normal
semantic inspection calls fit measured client budgets wherever practical.

## Workstream 1 — Correct retry accounting

### Required behavior

1. Change trace-summary `retryCount` from the number of retry sequence IDs to
   the number of actual attempts after an initial attempt.
2. Change frame `directRetryCount` to count actual retry attempts directly
   attributed to that frame.
3. Do not rename either public MCP field. Their natural names are correct once
   their values are corrected.
4. Do not expose retry-sequence count as a replacement. A separate count may be
   added only if a demonstrated consumer question requires it.
5. Derive retry membership only from the already validated attempt graph. Do
   not infer a retry from record adjacency, route, model text, provider error
   wording, or `PLAN_RETRY_REQUESTED`.

### Frame attribution rule

A retry attempt is direct to the frame recorded on that attempt's evidence.
Count a validated attempt as a direct retry when it belongs directly to the
frame and is later than the initial attempt in its retry sequence. Do not use
`len(frame.RetrySequenceIDs)`, and do not subtract frame-local distinct
sequences from frame-local attempts: an initial attempt and its retry may be
attributed to different frames.

### Required tests

- One initial success produces `attemptCount=1`, `retryCount=0`.
- Initial failure followed by success produces `attemptCount=2`,
  `retryCount=1`.
- Three attempts in one sequence produce `retryCount=2`.
- Ten independent initial successes produce `attemptCount=10`,
  `retryCount=0`.
- Multiple sequences sum their retries rather than their sequence count.
- Frame direct counts remain correct when the initial attempt and retry are in
  the same frame.
- Frame direct counts remain correct when attempts in one retry sequence are
  attributed to different frames.
- A planning retry record does not alter model retry counts.
- Existing invalid attempt identity, numbering, and lifecycle cases remain
  rejected.

## Workstream 2 — Make projections self-describing

### Required behavior

1. State in the MCP tool description—not only in the Agent Skill—that
   `LOOMSPAN_query_trace_frames` defaults to COMPACT.
2. State that COMPACT is for orientation and omits duration and usage detail.
3. State that DETAILED returns duration, usage attribution, retry identities,
   validations, failures, gaps, and uncertainty details.
4. In structured COMPACT output, omit projection-excluded optional fields or
   return JSON `null` according to the established schema convention. Do not
   synthesize numeric sentinel values.
5. In the text fallback, do not render projection-excluded values as `-` or as
   values that resemble invalid milliseconds. Either omit them or say
   `omittedByProjection=COMPACT` once in the page header.
6. Millisecond fields must be epoch milliseconds for timestamps and elapsed
   milliseconds for durations. Raw trace timestamps remain epoch seconds with
   fractional nanoseconds; do not describe them as monotonic ticks.

### Required tests

- COMPACT and DETAILED schemas and fallbacks make the difference observable.
- COMPACT does not emit a misleading duration sentinel.
- DETAILED returns the known inclusive and self durations from a nested-frame
  fixture.
- Timestamp conversion tests prove epoch semantics.
- Tool discovery output contains the terms `COMPACT`, `DETAILED`, `duration`,
  and `usage` near the frame-query operation.

## Workstream 3 — Reduce content-read friction without unpredictable output

### Required behavior

1. Keep descriptor-first output as the default. Do not automatically inline
   arbitrary model prompts or responses.
2. Make `inlineContent` visible and understandable in the MCP input schema and
   tool description. The consumer must be able to learn, without the Agent
   Skill, that it requests bounded complete values inline.
3. Describe the per-value limit, aggregate response budget, record-order
   selection, and inline omission behavior compactly enough to remain within
   the measured `tools/list` budget.
4. Preserve descriptor metadata beside every opaque `contentRef`, including
   content role/type, retained byte size, availability, completeness, and
   inline omission reason where applicable.
5. Do not expose or document the internal encoding of `contentRef`.
6. For both semantic content and raw-artifact range reads, treat omitted
   `start` and omitted `continuation` as the initial read at source offset zero.
7. Continue rejecting an input that supplies both `start` and `continuation`.
8. Continue validating an explicitly supplied start, including explicit zero.

### Auto-inline decision

Do not add implicit tiny-content inlining in this ticket. It makes default
response size and data exposure less predictable and may worsen host overflow.
First make the explicit option discoverable and measure the resulting workflow.
A later ticket may introduce a documented fixed tiny-value policy if evaluation
still shows material N+1 friction.

### Required tests

- A first read with neither `start` nor `continuation` starts at zero.
- An explicit `start: 0` remains equivalent.
- Supplying both controls fails with `INVALID_ARGUMENT`.
- Inline content honors the per-value and aggregate budgets without silently
  shortening any value.
- Every non-inlined eligible value has an explicit omission reason.
- An evaluator using only `tools/list` can identify how to request inline
  content and how to read a selected descriptor exactly.

## Workstream 4 — Prevent silent truncation and minimize host overflow

### Delivery invariants

1. A successful list or query response contains only complete items. Never
   slice an encoded JSON item, content descriptor, string, or fallback line.
2. When more matching items exist, return `hasMore=true` and a valid opaque
   continuation positioned after the last complete returned item.
3. A successful range response contains exactly `actualEnd-actualStart` source
   bytes after decoding. When bytes remain, return `hasMore=true`, total length,
   exact boundaries, and a continuation.
4. An explicit request above the supported range maximum returns
   `LIMIT_EXCEEDED`; it is not silently clamped.
5. Any intentional omission has a typed indication such as a projection,
   completeness flag, limitation, descriptor availability, or inline omission
   reason. Human prose alone is insufficient.
6. Text fallback and structured output must describe the same returned page.
   Neither may contain a partial item that the other presents as complete.

### Response-budget investigation

Before selecting new defaults, measure serialized MCP responses—not only the
domain DTO—for these current worst cases:

- `tools/list`;
- a 64-item trace inventory page;
- a 64-item compact frame page;
- a 64-item detailed frame page;
- a 64-item record descriptor page;
- a record page exhausting the inline-content aggregate budget;
- a default semantic content range;
- a default raw-artifact range; and
- the same responses including both structured content and text fallback.

Record byte sizes in a test or checked-in implementation note. Exercise at
least the Codex MCP host used for the original walkthrough; exercise another
supported host if one is available. Identify the lowest observed threshold at
which the host truncates, collapses, or moves output to overflow.

Then choose defaults that leave meaningful headroom below that measured
threshold. The implementation may use smaller default item counts, smaller
default range sizes, an encoded-response byte budget, or a combination.

The following are decision constraints, not optional suggestions:

- Do not reduce a response by cutting its serialized output after construction.
- If an encoded-response budget ends a page early, stop before adding the item,
  set `hasMore`, and produce a continuation that resumes with that item.
- A single item that cannot fit the selected semantic page budget must return a
  typed limit/representation error or a descriptor for a separate exact read;
  it must not be partially serialized.
- Caller-specified `pageSize` remains a maximum item count, not a guarantee that
  the server will violate its encoded-response safety budget to fill the page.
  Document this if byte-budgeted early pagination is adopted.
- Exact range callers may explicitly request large responses up to the
  documented maximum. Loomspan must return the complete requested bounded
  range. Avoiding a host's handling of a deliberately large explicit request
  is the caller's responsibility.
- Defaults should optimize the ordinary model workflow. Large exact export is
  not the ordinary workflow.

### `tools/list` requirements

The current compact authored schemas exist to keep discovery usable. Preserve a
tested serialized `tools/list` budget, but do not meet it by deleting semantics
that the two evaluations proved necessary.

Prefer, in order:

1. removing duplicated schema prose;
2. sharing compact schema construction internally;
3. shortening wording without changing meaning;
4. removing redundant fields or tools when contract analysis permits; and
5. only then revisiting the measured byte threshold.

Never emit a server-side truncated tool schema. If the full installed surface
cannot fit the supported discovery budget, reduce the schema representation or
surface area and keep required meaning intact.

### Required tests

- Serialize full MCP call results and assert the selected default response
  budgets, including text fallback overhead.
- Force byte-budget pagination at every item boundary and prove lossless
  reconstruction with continuations.
- Include one item immediately below and immediately above the boundary.
- Prove multibyte UTF-8 and base64 content are never split incorrectly.
- Prove explicit oversized ranges fail rather than clamp.
- Prove default range pagination reconstructs the exact retained bytes.
- Keep a `tools/list` size test and semantic-presence assertions for the
  workflow, projections, and inline-content affordance.

## Workstream 5 — Clarify inventory lifecycle times

### Required behavior

1. Keep `finalizedAt`, `acquiredAt`, and `importedAt` as independent facts.
2. Define all three in the inventory tool description and Agent Skill using the
   terminology in this ticket.
3. Explicitly state that `acquiredAt` may be later than `finalizedAt` because
   Console can acquire already-finalized evidence.
4. Do not infer one timestamp from another and do not use acquisition/import
   time as execution time.
5. Preserve `FINALIZED_DESC`, `ACQUIRED_DESC`, and `IMPORTED_DESC`; briefly
   associate each ordering with its corresponding lifecycle question.

### Naming decision

Do not rename `acquiredAt` in this ticket. The field is semantically correct,
is already paired with explicit acquisition ordering, and can be made clear
without another contract rename. Reconsider only if the no-skill evaluation
still confuses the field after the description is present in `tools/list`.

### Required tests

- A trace finalized earlier and acquired later preserves both values.
- Ordering by finalization and acquisition produces independently correct
  results.
- Discovery output states the independent meanings without requiring the Agent
  Skill.

## Workstream 6 — Add complete record-type discovery

`LOOMSPAN_get_trace` already returns trace outcome, aggregate semantic counts,
configured limits, and structured usage summaries. GLM nevertheless had to
scan the record stream to discover one `PLAN_QUALITY_WARNING`. Solve that
discovery problem without adding an arbitrary prefix of warning descriptors or
an opinionated definition of a "clean" trace.

### Required behavior

1. Add `recordCountsByType` to `LOOMSPAN_get_trace`.
2. Populate it with the complete count of every nonzero physical trace record
   type in the processed artifact. Omitted known record types mean zero.
3. The sum of all `recordCountsByType` values must equal `recordCount`.
4. Derive the histogram while processing the entire validated trace. Do not
   populate it from a query page, a bounded prefix, reconstructed model text,
   or only a curated set of records considered problematic.
5. Keep existing semantic summary fields separate, including `outcome`,
   `terminalFailureId`, `attemptCount`, corrected `retryCount`, `failureCount`,
   `validationCount`, `gapCount`, `uncertaintyCount`, and `usageComplete`.
6. Do not infer those semantic counts from the histogram at the MCP adapter.
   They have different cardinality and meaning:
   - a retry is a validated attempt after the initial attempt and is not a
     record type;
   - several failure-related records may describe one logical failure;
   - a failed model attempt may be recovered by a successful retry;
   - gaps and uncertainties are analysis findings rather than physical trace
     record types; and
   - terminal `outcome` remains authoritative for final execution success or
     failure.
7. Do not add `clean`, `unclean`, `hasProblems`, or another composite verdict.
   A successful trace with a recovered retry, a complete trace containing a
   plan-quality warning, and an evidence-incomplete trace are materially
   different conditions. Return the facts and let the consumer answer the
   user's question.
8. Emit every nonzero histogram entry in the deterministic text fallback. Do
   not shorten it to a preferred subset of record types.
9. Use `LOOMSPAN_query_trace_records` with selected histogram keys to retrieve
   complete paginated record details. Do not inline warning or failure bodies
   into `LOOMSPAN_get_trace`.
10. Do not synthesize a model-authored narrative, diagnosis, or recommended
    fix.

### Natural bound

The histogram is naturally bounded by Loomspan's closed trace record-type
enumeration rather than by an arbitrary number of record instances. Adding
more records increases integer values, not the number of summary entries.
Adding a new supported record type requires the existing closed-enum contract
update and then makes that type eligible to appear automatically when nonzero.

This is complete aggregation, not truncation: the summary reports the exact
cardinality of every observed record type, while the existing paginated query
returns every individual record when details are requested.

### Required tests

- An empty/non-present type is omitted and is documented to mean zero.
- A trace containing one `PLAN_QUALITY_WARNING` reports that exact histogram
  entry without scanning raw NDJSON.
- Multiple warning, validation, attempt-failure, tool-failure, and terminal
  records each receive their exact physical counts.
- Histogram values sum exactly to `recordCount` for the semantic fixture corpus.
- A recovered failed attempt can coexist with `outcome=SUCCEEDED`; the summary
  does not classify the whole trace as failed or unclean.
- Corrected `retryCount` remains zero for independent initial successes even
  though retry sequence IDs are present in attempt records.
- Distinct `failureCount`, analysis gap/uncertainty counts, and usage
  completeness remain independent from physical record counts.
- The structured schema accepts every current closed record-type key with a
  nonnegative integer value and rejects non-integer/negative counts.
- The text fallback emits all nonzero entries in deterministic order.
- Querying each selected histogram type and following continuations recovers
  all matching records without raw-artifact inspection.

## Workstream 7 — Resolve plural frame outcomes deliberately

Before changing the schema, prove whether the validated frame graph can contain
zero, one, or multiple distinct authoritative close outcomes for one frame.

- If the invariant is at most one outcome, replace `outcomes` with optional
  scalar `outcome` throughout the semantic DTO, MCP schema, fallback, filters,
  documentation, and tests.
- If multiple observed outcomes are intentionally preserved to represent
  contradictory evidence, retain `outcomes` and state that reason explicitly
  in the contract. Include a fixture demonstrating the valid multi-outcome
  case.
- Do not retain plural shape solely because the current implementation stores a
  set internally.

This investigation must end in one documented decision and test; it must not be
left as an implementation accident.

## Agent Skill and MCP contract responsibilities

The MCP surface must be locally understandable without the Agent Skill:

- immediate purpose of each tool;
- required predecessor identity such as `traceId` or `contentRef`;
- important defaults;
- pagination and completeness behavior;
- COMPACT versus DETAILED behavior;
- descriptor versus inline versus exact-read behavior; and
- recovery from stale continuations and content references.

The Agent Skill remains responsible for richer investigative judgment:

- how to select the relevant trace for the user's question;
- when ambiguity requires clarification;
- how to follow plan, attempt, failure, and validation joins;
- how to distinguish active execution from finalized trace evidence;
- when raw artifact forensics are justified; and
- how to communicate evidence limitations.

Do not add `LOOMSPAN_help`, `LOOMSPAN_describe_trace`, or a narrative
`LOOMSPAN_summarize_trace`. Another discovery or synthesis tool would itself
need discovery, increase `tools/list`, and duplicate deterministic semantic
operations.

## Compatibility and architecture

- This work changes the Console's MCP contract and internal Go implementation;
  it must not expand Loomspan's supported Java API.
- Do not add application-facing Java types outside the closed
  `com.lokiscale.loomspan.api` allowlist.
- If production Java types are touched, run
  `LoomspanPublicSurfaceArchitectureTest` as required by repository guidance.
- Treat opaque continuation and content-reference formats as implementation
  details. Do not add compatibility promises for their encoding.
- Evaluate MCP schema changes against current consumers and fixtures. Where a
  serialized field shape changes, update structured output, text fallback,
  authored output schema, README, Agent Skill, and semantic fixtures together.
- Preserve domain-error codes and recovery semantics unless this ticket
  explicitly requires a new typed limit/representation error for an
  individually oversized semantic item.

## Likely implementation areas

The implementer should confirm current ownership rather than blindly editing
this list:

- `loomspan-console/internal/traceanalysis/attempts.go`
- `loomspan-console/internal/traceanalysis/processor.go`
- `loomspan-console/internal/traceanalysis/query_frames.go`
- `loomspan-console/internal/traceanalysis/query_records.go`
- `loomspan-console/internal/traceanalysis/dto.go`
- `loomspan-console/internal/traceanalysis/model.go`
- `loomspan-console/internal/mcpadapter/trace_contracts.go`
- `loomspan-console/internal/mcpadapter/traces.go`
- `loomspan-console/internal/mcpadapter/output_schemas.go`
- `loomspan-console/internal/mcpadapter/trace_contracts_test.go`
- `loomspan-console/internal/mcpadapter/trace_semantic_fixtures_test.go`
- `loomspan-console/internal/traceinventory/`
- `loomspan-console/agent-skills/loomspan-runtime-debugging/`
- `loomspan-console/README.md`

## Evaluation plan

### Baseline before implementation

On the current branch, record:

- serialized `tools/list` bytes;
- default and maximum response bytes for each trace operation;
- which current calls trigger host overflow behavior;
- actual trace and frame retry counts for the evaluation trace;
- COMPACT and DETAILED frame output and fallback differences;
- whether a no-skill consumer can discover `inlineContent`; and
- whether first range read requires explicit zero.

This separates issues already fixed by the final PR 30 review from remaining
contract gaps.

### Post-implementation model walkthroughs

Run the same finalized-trace question in two modes:

1. MCP tools, schemas, and descriptions only; no Loomspan Agent Skill.
2. MCP tools plus the packaged Loomspan runtime-debugging Agent Skill.

The no-skill run passes when the evaluator can:

- select or request clarification for the correct trace;
- understand that ten successful initial calls mean zero retries;
- request DETAILED frames when it needs durations;
- retrieve selected content through inline content or exact semantic reads;
- distinguish finalization from later evidence acquisition;
- use `recordCountsByType` to identify notable record categories and retrieve
  every selected record through the semantic paginated query without scanning
  raw NDJSON;
- paginate every incomplete response without losing items or bytes; and
- avoid raw artifact reads unless the question is about storage/parser
  forensics.

The skill-assisted run passes when it performs the same tasks with fewer
unnecessary calls and preserves all evidence limitations.

Record calls, serialized response sizes, overflow incidents, continuation use,
incorrect semantic claims, and raw-artifact fallbacks. Do not score only the
final prose answer.

## Verification commands

Run the repository-standard formatting, static analysis, and tests discovered
from the current build files. At minimum, for the Go Console:

```text
gofmt on changed Go files
go test ./...
go vet ./...
```

Also run focused trace-analysis, MCP contract/schema, semantic fixture,
inventory-ordering, exact-range, pagination, and Agent Skill packaging tests.
If production Java types change, run the Java architecture test described
above plus the affected Java test suite.

## Acceptance checklist

- [ ] Initial attempts are not counted as retries.
- [ ] Trace and frame retry counts follow the examples in this ticket.
- [ ] Planning retry events are not conflated with provider/model retries.
- [ ] COMPACT omissions and DETAILED availability are discoverable in MCP.
- [ ] No projection-excluded duration is rendered as a misleading sentinel.
- [ ] `inlineContent` is discoverable without the Agent Skill.
- [ ] First content/artifact reads default to offset zero.
- [ ] Structured and fallback responses contain complete items only.
- [ ] Every partial page/range has explicit recovery metadata.
- [ ] Explicit oversized ranges fail rather than clamp.
- [ ] Default response sizes are based on recorded serialized measurements.
- [ ] Normal trace inspection does not trigger overflow in the measured host.
- [ ] Lifecycle timestamps are independently defined and correctly ordered.
- [ ] `recordCountsByType` completely counts every nonzero physical record type
      and its values sum to `recordCount`.
- [ ] Semantic retries, failures, gaps, uncertainties, usage completeness, and
      terminal outcome remain distinct from the physical record histogram.
- [ ] No arbitrary warning prefix or composite clean/unclean verdict is added.
- [ ] The plural/scalar frame outcome decision is documented and tested.
- [ ] Tool discovery remains within its tested budget without deleting required
      semantics.
- [ ] README, MCP schemas/descriptions, text fallbacks, and Agent Skill agree.
- [ ] No-skill and skill-assisted walkthrough results are recorded.
- [ ] Full relevant test suites pass.

## Out of scope

- A generic help or tool-description operation.
- An LLM-generated trace narrative or diagnosis endpoint.
- Streaming MCP responses.
- Whole-artifact retrieval as the normal trace-inspection path.
- Case-insensitive, fuzzy, regex, or contextual semantic search redesign.
- Transparent or caller-authored continuation tokens.
- Decoding or documenting opaque content-reference internals.
- Storing reconstructed model payloads as a replacement for exact raw NDJSON.
- Enforcing model-authored JSON or removing Markdown fences from captured model
  output. Trace inspection must report what was actually recorded.
- Changing the read-only/inert evidence security model.
- Adding a Java SPI, bean-replacement contract, or application-facing Java API.
