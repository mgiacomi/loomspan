# PR 30 — MCP Contract Efficiency and Trace Semantic Corrections

## Status

Ready for implementation.

This ticket is intentionally self-contained. It records the decisions and live
evidence needed to execute the work in a fresh context. The MCP and Console
contracts are unreleased: change them in place and update the producer,
consumer, fixtures, tests, evaluations, and documentation atomically. Do not
add compatibility shims, aliases, fallback interpretations, dual record types,
or version-negotiation machinery.

## Outcome

Make Loomspan's installed MCP surface materially cheaper for an LLM to learn,
and remove the trace-semantic ambiguities exposed by the latest successful
descriptor-first walkthrough.

After this work:

- `tools/list` is small enough to consume without truncation while remaining an
  accurate MCP contract;
- the complete internal result contract is still validated without advertising
  every nested DTO field to every model;
- ordinary timeouts are classified as timeouts without reading a stack trace;
- failed steps do not claim to be completed;
- one provider request is not recorded twice under indistinguishable names;
- plan landmarks identify their trace root, owning mission, and planning frame
  unambiguously;
- literal-search pages do not repeat a long content reference for every match
  in the same value; and
- text fallbacks never expose Go pointer addresses.

This is a correction and simplification ticket, not a request for new MCP
tools.

## Why this ticket exists

On 2026-08-18 a fresh stateless MCP client found and investigated trace
`6777e217-03af-4a7d-bc2a-c59798fb8f36` without being given its trace ID. The
trace was the sole recent candidate: `handleIncident`, source `TARGET`, outcome
`FAILED`, 104 records, 19 frames, seven model attempts/retries, and one recorded
validation/failure.

The PR-28 navigation contract worked well. Compact frame orientation, typed
facts, ordinary semantic content, inline small values, exact content reads,
and honest positive/negative search answered the investigation without raw
NDJSON. The trace established that classification and nested `checkDns`
completed, `checkDns` returned a DNS failure result, and the following
`investigateNetwork` step-model response timed out. The primary plan ended
`STALE`: classification completed, investigation was blocked, and the runbook
and response steps remained pending.

That walkthrough also exposed the concrete friction addressed below.

## Fixed design decisions

### Keep input schemas complete

Tool input schemas remain strict and complete. They are the model's invocation
contract and are already a modest part of discovery cost. Do not weaken input
validation or move required invocation knowledge elsewhere.

### Advertise compact, authoritative output schemas

Supply deliberately authored `outputSchema` values to the MCP SDK instead of
letting every generic handler advertise its entire generated DTO graph.

These schemas are not summaries, examples, or hints. They are the canonical
MCP schemas for the structured results actually returned, and every response
must validate against them. Describe the stable top-level envelope, the fields
needed to navigate or decide the next call, pagination/completeness facts, and
the stable identity of returned items. Rich nested evidence objects may be
described as objects with open additional properties when enumerating their
complete internal shape would add cost without improving tool selection.

Retain complete generated schemas internally for stronger validation and
contract tests. Validate actual structured results against both:

1. the compact schema advertised through `tools/list`; and
2. the full internal schema derived from the concrete result DTO.

The compact schema must never permit the implementation to drift away from the
full typed contract unnoticed. Keep the existing typed Go handlers; the MCP Go
SDK accepts an explicitly supplied `Tool.OutputSchema` while retaining generic
handler types.

### Do not add schema-discovery machinery

Do **not** add `LOOMSPAN_describe_tool`, an MCP schema resource, a GitHub schema
URL, external `$ref`, or a description such as “call another tool for the full
schema.” MCP defines `outputSchema` as the tool's result contract and has no
standard per-tool detail negotiation. A private discovery convention would add
another tool and another decision for every client while solving a problem the
normal descriptor-first workflow does not have.

Complete schemas may support internal validation and generated developer
documentation, but are not part of the LLM's normal discovery path.

### Prefer removal and exact semantics

The release is pre-v1. Remove redundant vocabulary and rename misleading fields
at the source. Do not preserve the old record or DTO names beside the corrected
ones.

## Measured baseline

The live `tools/list` JSON response was exactly 37,788 bytes and the client
reported about 10.8K tokens before truncating its view. The previous baseline
was 34,371 bytes, so the latest interface work added 3,417 bytes (about 9.9%).

Breakdown of the current 37,699 bytes of tool objects:

| Component | Bytes | Share |
| --- | ---: | ---: |
| Input schemas | 6,437 | 17% |
| Output schemas | 27,270 | 72% |
| Names, descriptions, annotations, and framing | 3,992 | 11% |

The common error branch alone is about 446 bytes and is repeated across eleven
tools, costing roughly 4.9 KiB. The heaviest tool is
`LOOMSPAN_query_trace_records` at 9,039 bytes: 1,756 bytes of input schema and
6,960 bytes of output schema. Its generated result branch is 6,434 bytes;
roughly 5,000 bytes describe record items and deeply enumerate optional typed
facts.

The current test in `loomspan-console/internal/mcpadapter/server_test.go` checks
an exact discovery document but only enforces the general 64 KiB compact
response ceiling. That ceiling is too loose for a document injected into model
context on connection.

## Scope

### 1. Compact the advertised MCP output contracts

Author compact result schemas for every installed Loomspan tool. Centralize
small reusable schema builders in `internal/mcpadapter`; do not hand-copy a
large envelope into each registration.

Requirements:

- Preserve strict, complete input schemas.
- Preserve the present success-versus-domain-error envelope behavior. Its
  compact schema must express that a response is one or the other, not both.
- Include descriptions only where they explain a decision, invariant,
  completeness boundary, opaque identifier, or continuation rule. Do not turn
  schemas into prose documentation.
- For inventory and query results, expose the result identity, `items`,
  `hasMore`, continuation, and any evidence/completeness object that constrains
  the conclusions an LLM may draw.
- For trace records, expose stable record identity, representation/content
  navigation, search results, and the existence of typed facts. Deep fact
  variants do not all need to be expanded in discovery.
- Keep the full generated schema beside the handler registration or its result
  type and validate serialized structured content against it internally.
- Add a table-driven test proving every real success and domain-error fixture
  validates against both schemas.
- Add negative tests proving malformed envelopes and missing core navigation
  fields fail the compact schema.
- Keep tool descriptions sufficient for tool selection, but do not use prose to
  reproduce fields removed from the advertised schema.

Set a dedicated discovery budget of **20 KiB maximum** for the complete
serialized `tools/list` response. Keep an exact discovery snapshot test and a
separate explanatory budget assertion. Record the new exact byte count in the
test failure message or nearby comment. Do not reuse the 64 KiB response-size
limit for this purpose. The target should be met through schema design, not
minification tricks, loss of input constraints, renamed one-letter fields, or
removal of useful tool descriptions.

Run one real client connection after implementation and record both source
bytes and the client's approximate token count. Source bytes are the stable
automated contract; client tokenization is an acceptance observation.

Likely starting points:

- `loomspan-console/internal/mcpadapter/server.go`
- `loomspan-console/internal/mcpadapter/server_test.go`
- `loomspan-console/internal/mcpadapter/contracts.go`
- tool registration files under `loomspan-console/internal/mcpadapter/`

### 2. Correct timeout classification

The observed terminal model attempt contained an
`OpenAIInvalidDataException`, caused by `java.io.InterruptedIOException:
timeout`, followed by an HTTP/2 stream-reset `CANCEL`. Typed facts nevertheless
reported classification/category `UNKNOWN` and `DO_NOT_RETRY`. The only way to
recognize an ordinary provider read timeout was to read a 6.5 KiB stack trace.

Correct causal-chain translation in
`SpringAiProviderIntegration.translate(Throwable)` so this provider read
timeout becomes the project's transient `TIMEOUT` classification and receives
the retry decision dictated by the existing policy.

Do not classify every `InterruptedIOException` or every HTTP/2 `CANCEL` as a
retryable timeout. Caller cancellation and actual thread interruption must
remain distinguishable from a provider request/read deadline. Inspect wrapper
messages, causes, and the concrete provider/HTTP exception shapes necessary to
make that distinction, and encode it once in the Spring AI integration.

Tests must cover:

- the exact observed wrapper/cause shape;
- a direct socket timeout;
- a provider read timeout expressed as `InterruptedIOException`;
- caller cancellation or thread interruption that must not become retryable;
- an unrelated unknown I/O failure; and
- the resulting model-attempt facts and retry decision, not only the private
  translation helper.

Update the Java-produced console corpus and Go fixture assertions so an LLM can
answer “why did this attempt fail and can it retry?” from typed facts alone.

Likely starting points:

- `loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/springai/SpringAiProviderIntegration.java`
- its integration/unit tests
- model-attempt fixture and evaluator expectations

### 3. Replace false `STEP_COMPLETED` records with `STEP_FAILED`

The failed tool step in the observed trace emitted a `STEP_COMPLETED` record
whose semantic data was `{"error":"Tool execution failed"}` while the frame
outcome was `failed`. `StepLoopMissionExecutionEngine` currently emits that
record in its tool-exception path. Record type alone therefore states the
opposite of what happened, and live projection renders “Step completed.”

Add `STEP_FAILED` as the failed terminal step record and stop emitting
`STEP_COMPLETED` from failure paths. This is an in-place vocabulary correction:
remove the old failed interpretation rather than teaching consumers that
“completed may mean failed.”

Requirements:

- A normally successful step emits exactly one `STEP_COMPLETED`.
- A step that terminates because its model or tool work fails emits exactly one
  `STEP_FAILED` with useful structured failure identity/category when known.
- An aborted/cancelled execution must retain its distinct abort semantics; do
  not relabel caller cancellation as a failure merely to force a terminal pair.
- `ERROR_RECORDED` remains diagnostic evidence and must not cause duplicate
  step-terminal events.
- Frame outcome, activity projection, journal projection, browser/MCP filtering,
  and record facts agree on the outcome.

Update the Java `TraceRecordType`, engine emission sites, execution activity and
journal vocabularies, Go trace-analysis enums/parsers, filters, projections,
fixtures, evaluators, and documentation in lockstep. Search exhaustively for
switches and allowlists. Do not retain a compatibility alias for failed
`STEP_COMPLETED`.

### 4. Remove redundant `MODEL_REQUEST_PREPARED`

`ProviderAttemptCallAdvisor` currently records `MODEL_REQUEST_PREPARED` and
`MODEL_REQUEST_SENT` back-to-back from the same `requestPayload`, with no
transformation or decision between them. The walkthrough showed equal lengths
and identical content stored under separate sequence-scoped content
references. The distinction consumes records, storage, search matches, schema
vocabulary, and model context without preserving a real boundary.

Remove `MODEL_REQUEST_PREPARED` from the trace producer and supported trace
vocabulary. Keep `MODEL_REQUEST_SENT` as the provider-attempt request event.
There is no need for content deduplication machinery to preserve an event that
has no distinct semantics.

Requirements:

- Every provider attempt emits one `MODEL_REQUEST_SENT` with the request content
  and stable attempt/retry identity before the provider call.
- Attempts that fail before entering the provider must still produce coherent
  typed attempt/failure evidence; use an existing truthful event rather than a
  fictitious “prepared” phase.
- Attempt reconstruction, sequence-gap reasoning, usage attribution, search,
  projections, and journal summaries continue to work with the smaller
  vocabulary.
- Remove the enum value and all producer, consumer, fixture, documentation, and
  evaluation references. Do not accept both names.

Likely starting points include `ProviderAttemptCallAdvisor.java`,
`TraceRecordType`, `traceanalysis/attempts.go`, the Java fixture corpus, and
`ai/skill-authoring/traces-and-debugging.md`.

### 5. Make plan landmarks unambiguous and stable

The nested `investigateNetwork` plan's typed `rootFrameId` pointed to the
top-level `handleIncident` trace root, not to the nested mission that owned the
plan. Its owning mission was recoverable only by walking the planning-frame
hierarchy. In addition, plan materialization initializes `planningFrameId` from
the current record frame; a later `PLAN_UPDATED` emitted outside the original
planning frame can therefore change the apparent planning-frame identity.

Replace the ambiguous landmark fields with this exact vocabulary:

- `traceRootFrameId`: the top-level root frame of the trace;
- `missionFrameId`: the `ROOT_MISSION` frame that owns this plan, including a
  nested mission when the plan belongs to a nested skill;
- `planningFrameId`: the original `PLANNING` frame in which `PLAN_CREATED` was
  accepted, stable for every later update to the same `planId`;
- `planId` and plan `sequence`; and
- accepted attempt/retry identity where it is actually recorded.

Remove `rootFrameId` from plan landmarks. Build plan lineage by `planId` during
trace processing from producer-owned frame hierarchy and the creation record,
then carry it into updates. Do not infer mission ownership from model text,
routes, or a query page that happens to include the creation record. If a plan
update lacks a valid creation lineage in a current-format trace, surface a
typed uncertainty/contract error rather than silently assigning the current
record frame.

Update MCP and browser DTOs together. Add corpus cases for:

- a primary plan and update;
- a nested-skill plan and update;
- multiple plan IDs in one trace; and
- a malformed update without creation lineage.

Assertions must show that all three frame identities remain stable and mean
exactly the same thing on `PLAN_CREATED` and every `PLAN_UPDATED`.

### 6. Stop repeating content references per literal-search occurrence

A positive ten-match page for `api.example.com` was several thousand client
tokens. Search was honest and bounded, but a long opaque `contentRef` was
repeated for every occurrence, including several offsets in the same semantic
value.

Keep match-level pagination and exact offsets, but normalize content references
within each page:

- return a page-level `contentDescriptors` collection;
- assign each unique content value/reference a short page-local `contentId`;
- put `contentId`, sequence/record identity, searched field, offset, and length
  on each match; and
- emit each opaque `contentRef` only once in `contentDescriptors`.

`contentId` is page-local and must not be accepted by
`LOOMSPAN_read_trace_content`; the descriptor contains the opaque reference
used for that call. Continuation and `pageSize` continue to count matches, not
unique content values. Deduplication must not hide offsets, alter case behavior,
claim incomplete work is complete, or require scanning beyond the configured
work bound.

Update shared trace-analysis/browser contracts if necessary to preserve MCP and
browser semantic parity. Add tests with multiple matches in one value, matches
across several values, continuation between pages, and no matches. Assert that
the same opaque reference occurs only once in the serialized page.

### 7. Fix optional-pointer rendering in MCP text fallbacks

The structured results were correct, but MCP fallback text rendered optional
string pointers with `%v`, producing values such as `outcome=0x...` in trace
inventory and `parentFrameId=0x...` in compact frames. Current sites include
`internal/mcpadapter/traces.go` around the inventory and frame fallback
formatters; `fallbackField` only accepts a plain string.

Create one explicit optional-value formatter and use it at every fallback site.
Define readable output for present, absent, and intentionally unknown values.
Do not rely on `fmt` pointer behavior.

Add table-driven tests for nil and non-nil optional strings and timestamps, plus
a serialized fallback assertion that rejects pointer-address patterns such as
`0x[0-9a-fA-F]+`.

## Cross-layer files and contracts to inspect

This list is a starting map, not an exhaustive edit list. Search before editing
and move every affected layer atomically.

- Java producer and vocabulary under
  `loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/`
- Java fixture generation in `ConsoleTraceFixtureCorpusTest`
- generated fixture corpus under `loomspan-console/internal/traceanalysis/testdata/`
- Go parsing, enums, attempts, facts, plans, search, and continuation under
  `loomspan-console/internal/traceanalysis/`
- MCP registrations, contracts, schemas, fallbacks, and tests under
  `loomspan-console/internal/mcpadapter/`
- browser trace-analysis DTOs and contract tests under
  `loomspan-console/internal/browserapi/`
- live and journal activity projections
- `loomspan-console/agent-evals/` fixtures and workflows
- trace-authoring and developer-facing documentation

No change in this ticket should expand Loomspan's supported Java API. Run
`LoomspanPublicSurfaceArchitectureTest` after production-type changes; its
allowlists remain authoritative.

## Acceptance walkthrough

After automated verification, restart the current development server and use a
fresh stateless MCP client. Do not seed the trace ID into the first call.

1. Fetch `tools/list` once. Record exact bytes and approximate client tokens;
   confirm the complete list and every input schema are visible and the source
   document is at most 20 KiB.
2. Find the newest test trace through normal inventory search.
3. Orient the trace with compact frames and typed facts.
4. Identify a primary plan and a nested plan. Confirm each result directly
   exposes correct and stable `traceRootFrameId`, `missionFrameId`, and
   `planningFrameId` without hierarchy reconstruction by the caller.
5. Trigger or inspect a provider read timeout. Confirm typed facts say
   `TIMEOUT` with the correct retry decision without reading diagnostic content.
6. Inspect a failed step. Confirm it has `STEP_FAILED`, a failed frame outcome,
   and no contradictory `STEP_COMPLETED`.
7. Inspect one provider attempt. Confirm exactly one request event/value exists
   and there is no `MODEL_REQUEST_PREPARED`.
8. Search a repeated literal in one content value. Confirm all offsets remain,
   the content descriptor appears once, continuation is honest, and exact
   content reading works from that descriptor.
9. Inspect fallback text for inventory and frames. Confirm identifiers/outcomes
   are readable values and no pointer address appears.
10. Repeat a complete negative search and confirm coverage and
    `workComplete=true` remain explicit.

The walkthrough should still require no raw-artifact read or manual NDJSON
decoding.

## Required verification

Run at minimum:

```text
cd <repository-root>
.\mvnw.cmd -pl loomspan-spring-boot-starter test -Dtest=ConsoleTraceFixtureCorpusTest -DfailIfNoTests=false

cd loomspan-console
go test ./...
go run ./internal/buildtool verify
```

Also run focused Spring integration tests for timeout translation, step
terminal events, provider request events, journal/live projection, and
`LoomspanPublicSurfaceArchitectureTest`. Run `go test -race ./...` when the
implementation changes shared caches or concurrent MCP registration/validation
state.

Use the repository's intentional fixture-regeneration workflow. Review corpus
diffs as contract changes; do not hand-edit around a failing producer fixture.

## Non-goals

- No schema-description tool, schema resource, external schema URL, or runtime
  schema negotiation.
- No new trace-navigation tool.
- No raw-artifact fallback in the ordinary workflow.
- No compatibility parsing for removed `MODEL_REQUEST_PREPARED`, ambiguous plan
  `rootFrameId`, or failed `STEP_COMPLETED` records.
- No redesign of the general trace envelope, content-reference security model,
  search algorithm, or trace-selection workflow beyond the corrections above.
- No compression/minification protocol whose only purpose is hiding discovery
  bytes from the budget.
- No new public Java API or SPI.

## Definition of done

The work is complete only when the advertised tool surface is at most 20 KiB,
all structured outputs validate against compact and full internal schemas, all
seven semantic corrections are exercised by cross-layer tests, the generated
corpus and evaluations agree with the new exact vocabulary, the live acceptance
walkthrough succeeds, and obsolete development-era names and interpretations
have been removed rather than retained as baggage.
