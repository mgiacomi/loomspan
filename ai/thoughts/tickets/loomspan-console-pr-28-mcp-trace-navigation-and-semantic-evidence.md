# PR 28 — MCP Trace Navigation and Semantic Evidence

## Status

Proposed next implementation ticket.

This ticket follows the initial LLM-facing MCP cleanup and framework-owned plan
identity work. It is deliberately a pre-v1 contract correction: change the
current Console, MCP, browser, fixtures, documentation, and tests together. Do
not add compatibility aliases, legacy fields, fallback readers, overloads, or
dual behavior.

## Outcome

An LLM should be able to select the intended finalized trace, orient itself in
the trace, and retrieve the semantic evidence needed to answer normal debugging
questions without downloading raw NDJSON, manually decoding records, or
requesting responses so large that they are truncated.

The interface must remain mechanical and evidence-oriented. It should expose
recorded facts and bounded content, not invent diagnoses or add a collection of
question-specific convenience tools.

## Why this is enough work for one ticket

Live MCP walkthroughs of one failed trace and one successful trace showed the
same underlying contract problem from opposite directions:

- small ordinary-data records were inaccessible through parsed inspection and
  required raw-artifact reads;
- envelope payloads could be inlined so aggressively that responses became
  tens of thousands of tokens and were truncated;
- structural and search responses returned far more detail than was needed to
  choose the next piece of evidence; and
- trace discovery worked in a three-trace inventory but would become ambiguous
  as soon as more imported or successful traces existed.

These are not independent feature requests. Together they define the minimum
trace-navigation and semantic-evidence contract required before another useful
LLM walkthrough can dig deeper.

## Evidence from the live walkthroughs

### Failed traces

The prompt under test was:

> I ran `handleIncident`. Pull up the trace and show me what plan the model
> ended up creating for the primary mission.

The recent inventory made the newest candidate unambiguous. That trace failed
during planning, contained three frames and one model attempt, and correctly
contained no `PLAN_CREATED` or `PLAN_UPDATED` records. Runtime discovery, trace
listing, summary, frames, and a plan-record query established the negative
answer without a raw read. Preserve this safe stopping behavior: absence is a
fact only when the bounded query is complete.

A richer failed trace contained ten frames and 59 records. It had one primary
plan creation and two primary plan updates. A logical record query with payload
inlining still returned all three as physical, raw-only records. Recovering the
final plan required three exact raw NDJSON reads and manual decoding. The final
recorded state had classification completed and network investigation, runbook
lookup, and response drafting pending; the execution later failed. The
interface must make it difficult to confuse a recorded plan state with completed
execution.

### Recently imported successful trace

The uploaded trace was discoverable without its supplied trace ID only because
it was the sole `SUCCEEDED` item in a complete three-item inventory. It retained
its original finalization time, sorted behind newer failures, and exposed an
empty entry skill. There was no source-specific acquisition/import time and no
server-side source, outcome, entry-skill, session, or time filter. This is not a reliable
“the successful trace I just uploaded” workflow at realistic inventory size.

The successful trace contained 187 records, 30 frames, 12 model attempts, 11
retry sequences, two validations, twelve reconstructed payloads, complete
usage, and no failures.

- The full canonical frame response was about 15.6K tokens and was truncated.
  Normal orientation needed frame identity, parent, type, route, outcome, and
  selected landmarks, not every duration, usage aggregate, attempt, retry,
  validation, failure, gap, and uncertainty field on every frame.
- Twelve plan records represented a primary plan chain and a nested plan chain.
  They were individually small but collectively required about 24 KiB of raw
  reads. The parsed results exposed no semantic value, content descriptor,
  typed plan ID, or accepting-attempt relationship.
- A whole-trace model-exchange query was about 57K tokens and was truncated.
  Prepared and sent request envelopes repeatedly inlined large, often duplicate
  prompts while model responses remained raw-only. The MCP text fallback then
  duplicated the structured result.
- Frame-scoped tool queries located calls efficiently, but tool input and output
  were ordinary `data` and therefore raw-only. Advisor mutations, structured
  output, validation content, and the final model response had the same problem.
- Literal search for `INC-2401` returned about 10.8K tokens of rich record
  descriptors. It did not state the searched fields, case behavior,
  representation, or referenced-content coverage at page level. Observed
  matches differed between ordinary record data and reconstructed envelopes,
  making a negative result unsafe to interpret.
- Exact raw record-range reads reported `hasMore=true` when unrelated bytes
  followed in the backing artifact, even though the selected record itself had
  been read completely.
- `filter.types` and validation-status filters did not advertise their accepted
  vocabulary.

Raw fallback eventually established that all four primary tasks completed and
that the final answer classified the incident as a SEV2 network issue involving
EU DNS failures and likely regional DNS or propagation delay. The evidence was
present; the parsed MCP path made it unnecessarily costly to retrieve.

The imported successful trace predates framework-owned plan identity. Its
model-authored plan ID and absent accepted-attempt fields are legacy evidence,
not a contract to preserve. Use a trace produced by the current runtime to test
the new plan contract.

## Required contract behavior

### 1. Reliable finalized-trace discovery

Evolve the unified trace inventory rather than introducing “current trace”
state or separate target/import listing tools.

- Add bounded, server-side filters for evidence source, outcome, entry skill,
  session ID, and time. All active filters must participate in continuation
  fingerprinting.
- Preserve explicit `complete`, `limitations`, `hasMore`, and ambiguity
  semantics. A client may claim “latest,” “only,” or “none” only from a complete
  result for the requested filter domain.
- Keep three independent discovery facts instead of overloading one timestamp:
  evidence source, the time this Console obtained the evidence, and the time
  the recorded execution finalized.
  - `evidenceSources` identifies `TARGET`, `IMPORTED`, or both in the fixed
    order `TARGET`, then `IMPORTED` when a trace-ID collision is present. It
    describes only how evidence reached Console; it does not authenticate an
    imported artifact, establish deployment provenance, or expose target
    scope/owner identity.
  - `acquiredAt` applies only to a target artifact successfully fetched and
    installed from the configured framework application. A target catalog
    candidate that has not been acquired has no `acquiredAt`.
  - `importedAt` applies only to an artifact successfully validated and
    installed through Console import. A failed, cancelled, or rejected import
    does not publish a candidate or an `importedAt` value.
  - `finalizedAt` remains the producer-owned time when the recorded execution
    ended. Acquisition or import must never rewrite it.
- Capture `acquiredAt` or `importedAt` at successful evidence publication, when
  the installed artifact first becomes usable by this Console process. Reuse
  of the same installed copy preserves that time; expiry/removal followed by a
  new acquisition or import creates a new time. These remain process-local
  lifecycle facts and are not restored after restart.
- Add closed ordering values `FINALIZED_DESC`, `ACQUIRED_DESC`, and
  `IMPORTED_DESC`. `FINALIZED_DESC` remains the default. A source-time order
  places candidates lacking its timestamp after candidates that have it; ties
  use `finalizedAt` descending and then `traceId` ascending. Ordering does not
  silently filter candidates.
- Add a non-empty any-of `sources` filter plus `finalizedFrom`/`finalizedTo`,
  `acquiredFrom`/`acquiredTo`, and `importedFrom`/`importedTo` time windows.
  Bounds are inclusive RFC 3339 instants and an inverted range is invalid. An
  active source-specific time filter requires that timestamp, so an
  acquired-time filter excludes catalog-only and imported-only evidence and an
  imported-time filter excludes target-only evidence; a collision may still
  contain a matching source instance.
- Evaluate all active source, identity, outcome, and time filters against the
  same underlying evidence instance so fields from target and imported copies
  can never be combined to manufacture a match. Group by trace ID, retain the
  complete collision facts, and emit one candidate when at least one instance
  matches. A match does not remove ambiguity or hide the nonmatching conflicting
  source. When multiple instances match, use the greatest applicable ordering
  timestamp as the candidate's internal sort key; this key does not become a
  canonical shared field.
- All filters, order, page size, work position, and relevant installed-evidence
  facts participate in continuation fingerprinting.
- Keep one candidate per trace ID. Filtering or ordering must never resolve a
  target/import collision: the candidate remains ambiguous, reports both
  evidence sources and their applicable availability timestamps, and cannot be
  inspected until the conflict is resolved. If source instances disagree on
  shared execution metadata such as `finalizedAt`, do not publish one instance's
  value as canonical; report the shared fact unavailable under the ambiguity.
- Populate imported `entrySkill` only from validated trace evidence. If it
  cannot be derived reliably, leave it absent/unknown and state that limitation;
  do not infer it from filenames, model text, or UI state.
- Because the catalog is process-local and starts empty, change its in-memory
  model in place. No migration or metadata backfill is required.

### 2. Compact structural orientation

Make the default frame query suitable for finding roots, paths, and relevant
frames in a medium or large trace.

- Provide a compact projection containing only identity, parent/child
  relationship, frame type, route, timestamps or outcome needed for ordering,
  and compact landmarks needed to choose follow-up evidence.
- Keep expensive duration, usage, attempt/retry, validation, failure, gap, and
  uncertainty calculations available through one explicit detailed projection.
- Projection choice must be bounded, schema-enumerated, and included in the
  continuation fingerprint.
- A compact query over the 30-frame successful fixture must fit comfortably in
  one client response without truncation.

Do not add separate hierarchy, usage, retry, or failure tools merely to achieve
the projection.

### 3. One semantic-content abstraction

Give every record-associated semantic value the same descriptor/read model,
whether the value originated in a chunked envelope or an ordinary record's
`data` field.

The abstraction must cover at least:

- plans and plan updates;
- model prepared/sent/received values;
- tool call input and output;
- advisor request/response mutations;
- validation evidence and structured output; and
- future record types without adding a specialized MCP method.

Each returned descriptor must provide enough information to decide whether to
read the value and to retrieve it exactly. At minimum, expose content kind or
field, content type/encoding, retained byte length, completeness/truncation,
and an opaque content reference when the bytes are available. A small value may
also be inline under the deterministic budgets below.

Use one exact bounded semantic-content reader. Prefer generalizing the existing
payload-reference path and `LOOMSPAN_read_trace_payload` instead of adding
plan-, model-, or tool-specific readers. `LOOMSPAN_read_trace_artifact` remains
optional raw storage/parser forensics, not the normal semantic path.

Logical representation must be truthful:

- a logical query returns a logical record descriptor even when its semantic
  content originated in ordinary physical `data`;
- if a requested logical value cannot be reconstructed, return an explicit
  availability/completeness fact rather than silently labeling it physical; and
- physical representation remains the deliberate record-storage view.

### 4. Bounded descriptor-first content

Replace the coarse page-wide `inlinePayload` behavior in place.

- Define deterministic per-value and aggregate inline-byte budgets.
- Always retain the descriptor when content is omitted because of a limit.
- Make default queries descriptor-first and small enough to select one or a few
  values before reading them.
- Include all content-selection and budget inputs in continuation
  fingerprinting.
- Never silently truncate an inline value. Report complete inline content or a
  descriptor that can be read exactly.
- Avoid returning the same reconstructed prompt multiple times when records can
  refer to the same semantic content descriptor.

The exact byte limits should be centralized with the existing trace-analysis
limits and covered by boundary tests. Token counting is not a server contract;
source-byte bounds and response measurements are.

### 5. Typed plan and attempt landmarks

Expose the mechanical facts needed to select and follow a plan chain without
decoding arbitrary JSON:

- framework-owned plan ID on current `PLAN_CREATED` and `PLAN_UPDATED` records;
- plan version or ordering facts already guaranteed by the producer;
- owning frame/root relationship; and
- accepted attempt and retry-sequence lineage when recorded by the current
  producer contract.

Do not infer or preserve legacy model-authored plan IDs. Do not synthesize plan
success from task states or execution outcome. The semantic plan value remains
available through the general content abstraction; typed facts are selection
landmarks, not a second plan representation.

### 6. Explicit filter vocabulary and honest search

- Advertise the complete accepted enum for record types and every closed filter
  vocabulary, including validation status and frame projection.
- Validate unknown enum values consistently at the application boundary.
- For literal-text search, return page-level metadata stating case behavior,
  searched fields, representation, whether referenced semantic content was
  covered, and whether the searched domain is complete.
- Search results should be compact match descriptors, not full rich records.
  They must contain stable information needed to query/read the matching value.
- Continuation must cover both result pagination and bounded search work. A
  page with no matches but more work remaining must not imply absence.
- Search ordinary semantic `data` and reconstructed envelope content under one
  documented policy. If any content is unavailable or intentionally excluded,
  report that limitation explicitly.

Keep literal search mechanical. Do not add fuzzy, semantic, embedding, or
cross-trace search in this ticket.

### 7. Separate selected-value completeness from artifact continuation

The semantic reader's continuation and `hasMore` must describe only the
selected semantic value. Reading a complete record value must return complete
even when unrelated bytes follow in the raw artifact.

The raw-artifact reader may continue to describe bytes remaining in the entire
artifact, but name and document that meaning unambiguously. Do not make callers
infer selected-value completeness from a backing-artifact offset.

### 8. Concise MCP fallback text

All tools must remain usable by clients that ignore structured content, but the
text arm must not serialize and repeat a large structured result verbatim.

- Produce a concise, fact-complete navigation fallback: evidence identity,
  item/match counts, completeness/continuation, limitations, and compact item
  lines or bounded content as appropriate.
- Preserve untrusted-data treatment and never turn trace content into
  instructions.
- Add response-size regression tests for representative large pages.

### 9. Measure and reduce tool-discovery cost

Capture the serialized byte size of `tools/list` before and after this work.
Remove redundant descriptions/schema expansion where doing so does not weaken
validation or machine discoverability. Keep the existing small, general tool
surface; do not trade schema size for undocumented stringly typed inputs.

Commit the measurement and a regression assertion or documented budget so the
cost does not silently grow back.

## Implementation surfaces

At minimum, inspect and update these layers atomically where applicable:

- `loomspan-console/internal/traceinventory`: query/filter model, evidence
  sources, acquired/imported/finalized timestamps, ordering, completeness,
  cursor fingerprinting, and imported
  entry-skill facts;
- `loomspan-console/internal/artifact` and import/catalog paths: process-local
  acquisition/import publication metadata and validated imported trace metadata;
- `loomspan-console/internal/traceanalysis`: DTOs, parser/index components,
  record facts, general content references/store, bounded reads, frame
  projections, search coverage, limits, and continuations;
- `loomspan-console/internal/mcpadapter`: input/output DTOs, JSON Schemas, enum
  publication, compact text results, tool descriptions, contract tests, and
  browser/MCP parity tests;
- `loomspan-console/internal/browserapi` and `loomspan-console/web` where they
  share the changed internal contracts;
- Java producer and fixture corpus for current `TRACE_STARTED.entrySkill`,
  framework-owned plan identity, and accepted-attempt facts;
- `loomspan-console/agent-skills/loomspan-runtime-debugging`, Console README,
  compatibility documentation, capability contract, and deterministic agent
  evaluations.

The exact executable authority is the code and tests. Documentation must be
updated in the same change and must describe the new path without mentioning
removed parameters or legacy behavior.

## Required tests

### Inventory

- Each source, outcome, identity, and finalized/acquired/imported time filter
  independently and in combination.
- Stable `FINALIZED_DESC`, `ACQUIRED_DESC`, and `IMPORTED_DESC` ordering,
  including missing timestamps and deterministic tie-breakers.
- Recently imported evidence with an older `finalizedAt`, a target catalog
  candidate with no `acquiredAt`, installed-copy reuse, reacquisition after
  removal, and failed imports that publish no timestamp.
- Target/import collisions retain both source facts, cannot be disambiguated by
  filtering or ordering, and do not publish conflicting shared metadata as
  canonical.
- Valid and unavailable imported entry-skill derivation.
- Complete vs incomplete discovery and ambiguous trace IDs.
- Continuations rejected when filters, ordering, page size, catalog work
  position, evidence sources, or source timestamps change.

### Frames

- Compact is the default projection and omits rich calculations.
- Detailed projection retains all existing mechanical evidence.
- Projection-bound continuations cannot be reused across projections.
- A 30-or-more-frame fixture stays below the chosen serialized response budget.

### Records and semantic content

- Ordinary JSON `data` and reconstructed envelopes expose the same descriptor
  contract.
- Plan, model, tool, advisor, validation, and structured-output examples are
  reachable without raw artifact reads.
- Empty, scalar, object, array, text, binary/base64, unavailable, and truncated
  values have unambiguous descriptors.
- Per-value and aggregate inline limits are tested immediately below, at, and
  above the boundary.
- Omitted inline content retains a readable content reference.
- Exact continuation reconstructs precisely one selected value.
- Logical and physical representation behavior is explicit and deterministic.
- Current producer fixture exposes framework plan identity and accepted-attempt
  lineage; legacy fixtures do not receive inferred compatibility facts.

### Search

- Ordinary data and reconstructed envelope content use the documented search
  policy.
- Case behavior and searched-field metadata are asserted.
- Compact matches identify the matching record/content without returning full
  records.
- Work-limited zero-match pages retain continuation and cannot be mistaken for
  a complete negative result.
- Unavailable or excluded content produces a visible coverage limitation.

### MCP and cross-layer parity

- Tool input schemas enumerate every closed vocabulary and enforce all bounds.
- Structured and text responses agree without duplicating large JSON.
- Browser and MCP adapters return equivalent semantic facts.
- Target, imported, ambiguous, unavailable, expired, stale-continuation, and
  stale-content-reference cases retain their existing safe error semantics.
- `tools/list` serialized size is measured and guarded.
- MCP protocol conformance continues to pass for both supported revisions.

## Live acceptance walkthrough

After implementation, start the production Console MCP adapter and repeat these
questions through a real stateless MCP client. Record call count and serialized
response size for every call. Do not use a trace ID until discovery has selected
one.

1. “Show me the successful trace I just imported.” The inventory must select it
   using source `IMPORTED`, an imported-time window, and `IMPORTED_DESC`, even
   when it finalized before other traces. It must not match target-only evidence
   or rewrite `finalizedAt`.
2. “Show me the final plan for the primary mission.” The answer must identify
   the correct plan chain and final recorded version without raw NDJSON.
3. “Which attempt produced the accepted plan?” The answer must come from typed
   current-producer lineage, not chronological inference.
4. “What did this tool receive and return?” A frame-scoped query plus semantic
   content read must answer it without raw artifact access.
5. “What was the final model response and structured output?” Both must be
   directly addressable and bounded.
6. “Find `INC-2401`.” Positive matches must explain where they came from; a
   complete negative search must be safe to state.
7. Run the primary-plan question against a trace that failed before plan
   acceptance. A complete empty result must stop cleanly without inventing an
   unaccepted plan.
8. Query the 30-frame hierarchy and all model-exchange descriptors. Neither
   navigation response may be truncated or dominated by duplicate content.

Success means each ordinary understanding question follows:

`discover trace -> orient compactly -> query compact descriptors -> read only selected semantic content`

Raw-artifact access must not appear in the normal path.

## Acceptance criteria

- All nine required contract areas above are implemented as one coherent
  destructive contract update.
- The live walkthrough answers the successful and failed trace questions
  without manual NDJSON decoding and without truncated navigation responses.
- No specialized plan/model/tool inspection method, mutable current-trace
  pointer, compatibility alias, or legacy fallback is introduced.
- Tool discovery and representative response sizes are measured and protected
  by regression tests or explicit budgets.
- Agent Skill guidance and eval cases teach and verify the new
  descriptor-first workflow.
- Documentation describes only the new contract.
- The standard Console verification and Java fixture corpus pass:

```powershell
Set-Location loomspan-console
go test ./...
go run ./internal/buildtool verify
Set-Location ..
.\mvnw.cmd -pl loomspan-spring-boot-starter test -Dtest=ConsoleTraceFixtureCorpusTest -DfailIfNoTests=false
```

- Run the race suite when the touched concurrency/storage paths warrant it:

```powershell
Set-Location loomspan-console
$env:PATH = "C:\msys64\mingw64\bin;" + $env:PATH
$env:CGO_ENABLED = "1"
go test -race ./...
```

## Guardrails

- Preserve the supported Java application API boundary. Public API changes are
  governed by `LoomspanPublicSurfaceArchitectureTest`; this ticket should remain
  in internal producer/Console contracts unless a genuine consumer need is
  identified.
- Keep MCP read-only, stateless, bounded, trace-ID based, and independent of the
  optional Agent Skill.
- Keep opaque references and continuations process-, owner-, operation-, and
  request-fingerprint bound. They are not authority credentials.
- Never expose artifact handles, target scope IDs, instance IDs, source paths,
  credentials, or storage topology to the LLM contract.
- Preserve safe target/import collision behavior and untrusted-content handling.
- Centralize limits; never silently clamp or truncate exact content.
- Prefer deletion and direct replacement over compatibility machinery.

## Out of scope

- Specialized plan, model, tool, advisor, or diagnosis tools.
- Server-generated incident summaries, diagnoses, recommendations, or plan
  comparisons.
- Mutable “selected” or “current” trace state shared between clients.
- Fuzzy, semantic/vector, or unbounded cross-trace search.
- Live-execution tailing or automatic transition from active execution to
  finalized trace.
- Browser-only local-path semantics or accepting arbitrary client filesystem
  paths through MCP.
- New MCP resources, prompts, sampling, elicitation, or write operations.
- Compatibility shims for the pre-v1 MCP/Console contract or legacy trace
  inference.
- General UI redesign unrelated to keeping shared contracts correct.
