# PR 33 — Factual Attempt-Failure Visibility and Retry Frame Filtering

## Status

Proposed implementation ticket. This is the next Loomspan ticket after PR 32.

This ticket is intentionally self-contained so it can be implemented in a
fresh context. The motivating observation came from an Antigravity/Gemini
trace investigation and was independently reproduced with Codex against the
same live Loomspan Console MCP server. The implementation must remain
client-neutral and model-neutral.

## Outcome

Make two small improvements to the existing read-only trace-inspection
contract:

1. Make the deterministic text fallback for `MODEL_ATTEMPT_FAILED` expose the
   normalized attempt-failure facts already present in the structured MCP
   result.
2. Let callers select frames by the existing directly attributed retry count
   with `filter.minDirectRetries` on `LOOMSPAN_query_trace_frames`.

Both changes must preserve Loomspan's distinction between recorded evidence,
mechanical calculation, context, and inference. They must not synthesize a
root cause, diagnostic message, retry explanation, or new trace fact.

## Why this belongs in the framework

These are not accommodations for one weak model or one client:

- MCP clients are allowed to consume the deterministic text fallback instead
  of `structuredContent`. Important recorded failure facts must remain useful
  on that portable path.
- Finding frames with retries is a general trace-navigation operation. Without
  a count filter, every client must page through all detailed frames and apply
  the same selection locally even though Console already owns the validated
  direct retry count.
- The runtime has authoritative parsed attempt identity and frame attribution.
  It is safer and more consistent for Console to expose and select those facts
  than for each model to infer them from adjacency, repeated routes, timing, or
  record text.

The proposed additions are deliberately narrower than the original feedback.
They add one factual fallback projection and one query predicate, not a new
workflow abstraction.

## Verified motivating evidence

The investigation used finalized trace
`f6d9fe9a-0473-4241-84cc-3b154da21e60`, session
`0f84f043-6197-470f-a698-c917b88d1ad8`. At inspection time, Console reported:

- outcome `SUCCEEDED`;
- 278 physical records;
- 47 frames;
- 21 attempts;
- 3 retries;
- 3 validations;
- 2 `MODEL_ATTEMPT_FAILED` records;
- no gaps or uncertainties.

This live trace is supporting product evidence only. It is ephemeral and must
not become a test dependency, fixture identity, compatibility promise, or
source of copied application content.

### Attempt-failure observation

Physical records 230 and 234 belong to frame
`ae0ed5f9-2597-4706-b4ce-8d8f20a9761f`, route
`planTrip#step-4-model`.

The MCP text fallback returned only sequence, type, frame ID, representation,
and an empty content reference. The structured `facts.attempts` values and raw
record metadata already contained these normalized facts:

| Sequence | Recorded attempt facts |
| ---: | --- |
| 230 | attempt 1, `INITIAL`, provider attempt 1, `TRANSIENT`, `TIMEOUT`, `RETRY`, 419 ms `BACKOFF` |
| 234 | attempt 2, `PROVIDER_RETRY`, provider attempt 2, `TRANSIENT`, `TIMEOUT`, `RETRY`, 1111 ms `BACKOFF` |

Both records also contained exact `attemptId` and `retrySequenceId` values.
Neither record contained an HTTP status, provider error type/code, exception
message, error reason, or diagnostic data payload. The implementation must not
invent any of those absent values.

The gap is in the fallback projection, not in trace parsing. Attempt parsing
already reads classification, category, retry decision, delay, optional HTTP
status, and optional provider error fields in
`loomspan-console/internal/traceanalysis/attempts.go`. The MCP adapter already
maps those values into `attemptDTO` in
`loomspan-console/internal/mcpadapter/traces.go`, but
`recordFallbackLine` currently does not render them.

The current README says every success has a deterministic, fact-complete text
fallback for clients that do not consume structured results. PR 33 must make
the attempt-failure behavior agree with that stated client-neutral contract.

### Retry-navigation observation

The same trace contained two model-call frames with directly attributed later
attempts:

| Frame | Route | Attempts | Direct retries | Nature visible in recorded attempt/validation facts |
| --- | --- | ---: | ---: | --- |
| `ae0ed5f9-2597-4706-b4ce-8d8f20a9761f` | `planTrip#step-4-model` | 3 | 2 | provider retries after recorded `MODEL_ATTEMPT_FAILED` facts |
| `0a4d04a2-050c-40e1-9780-5e8b37a077fd` | `assembleItinerary#mission-model` | 2 | 1 | semantic retry with recorded `retrying` then `passed` validation facts |

Requesting all detailed `MODEL_CALL` frames with `pageSize: 64` still required
two response-budgeted pages (9 frames each) before both retrying frames could
be selected. The existing `validationStatus: "retrying"` filter found the
validation case directly, but there is no corresponding direct-retry-count
filter for the provider-retry case.

Console already calculates `DirectRetryCount` from validated later attempts
explicitly attributed to the exact frame. That calculation is populated in
`loomspan-console/internal/traceanalysis/processor.go` and already appears in
both compact and detailed frame results. PR 33 adds selection over that
existing value; it does not create a new trace fact.

## Required behavior

### 1. Factual `MODEL_ATTEMPT_FAILED` text fallback

Extend the deterministic record fallback so a returned
`MODEL_ATTEMPT_FAILED` record includes the normalized attempt-failure fields
already present in that record's `facts.attempts` entry.

The fallback must expose, in a deterministic order:

- `attemptId`;
- `retrySequenceId`;
- `attemptNumber`;
- `attemptReason`;
- `providerAttemptNumber`;
- `failureClassification`;
- `failureCategory`;
- `retryDecision`;
- `retryDelayMillis`;
- `retryDelaySource`;
- `httpStatus` only when recorded;
- `providerErrorType` only when recorded;
- `providerErrorCode` only when recorded.

Use the adapter's already mapped `attemptDTO`; do not re-read raw NDJSON or
duplicate parsing in the MCP layer. Render untrusted string values with the
same safe deterministic quoting discipline as other fallback fields. Preserve
the existing one-record-per-line property and response-budget admission.

The implementation must not:

- add or synthesize a `cause`, `errorReason`, summary sentence, exception
  message, or diagnostic payload;
- translate `TIMEOUT`, HTTP status, classification, retry decision, or provider
  error values into causal prose;
- infer missing fields from delays, attempt order, route, adjacent records, or
  eventual success;
- claim that an earlier failed attempt is the terminal execution failure;
- expose application content that is not already part of the returned
  structured attempt facts;
- broaden this ticket into a redesign of every record fallback.

When an optional recorded field is absent, omit that field rather than
printing a fabricated default such as zero, `unknown`, or an empty diagnostic.

### 2. `filter.minDirectRetries` for frame queries

Add this optional input to the `LOOMSPAN_query_trace_frames` frame filter:

```json
{
  "filter": {
    "minDirectRetries": 1
  }
}
```

Exact semantics:

> Return a frame only when its existing `directRetryCount` is greater than or
> equal to `minDirectRetries`.

The name is deliberately `minDirectRetries`, not `hasRetries`, `minAttempts`,
or `retrying`. It must match the existing `directRetryCount` field and preserve
the fact that retry attribution is exact-frame-only.

Requirements:

- The MCP schema accepts integers of at least 1 when the field is present.
- An omitted/zero internal value means no minimum filter; malformed or
  negative values must not silently change query meaning.
- The filter composes with every existing frame filter using the existing AND
  semantics.
- It works with `COMPACT` and `DETAILED` projections and all supported orders.
- It matches only the frame's existing `DirectRetryCount`; do not propagate
  descendant retries or aggregate by route, skill, parent, or retry sequence.
- It participates in the canonical query fingerprint so a continuation cannot
  be reused after changing the minimum.
- It does not require or introduce a new stored index. Scanning the selected
  existing frame order while returning only matching frames is sufficient.
- It does not change `frameType`. A retrying model frame remains
  `MODEL_CALL`; `RETRY` is a structural frame type, not an anomaly label.
- It does not add `hasValidationIssues`. Existing `validationStatus` filtering
  remains the supported targeted validation query.

The tool description and schema property description must state that the
filter uses later attempts explicitly attributed to the exact frame. Do not
describe the result as a root-cause or anomaly determination.

## Likely implementation locations

The fresh implementation context must verify these locations against current
`main` before editing:

- `loomspan-console/internal/mcpadapter/traces.go`
  - `mapRecord`, `recordFallbackLine`, `traceRecordsText`;
  - `LOOMSPAN_query_trace_frames` description and schema customization.
- `loomspan-console/internal/mcpadapter/trace_contracts.go`
  - `queryTraceFramesInput` consumes `traceanalysis.FrameFilter` directly;
  - `attemptDTO` already contains all required failure fields.
- `loomspan-console/internal/traceanalysis/query_frames.go`
  - `FrameFilter`;
  - input validation and canonical fingerprinting;
  - `frameMatchesFilter`.
- `loomspan-console/internal/traceanalysis/processor.go`
  - existing `populateDirectRetryCounts`; this should remain the authority for
    the calculation and normally should not need behavioral changes.
- `loomspan-console/internal/traceanalysis/attempts.go`
  - existing normalized attempt-failure parsing; no new inference belongs
    here for this ticket.

## Tests and executable evidence

Implement failing tests first where practical. Prefer deterministic contract
tests over a model-specific evaluation.

### Trace-analysis tests

Extend focused tests under
`loomspan-console/internal/traceanalysis/`, especially `service_test.go`, to
prove:

- `minDirectRetries: 1` returns frames with one or more explicitly attributed
  later attempts and excludes frames with zero;
- a threshold of 2 excludes a frame with one retry and includes a frame with
  two;
- descendant retries do not make a parent match;
- the filter composes with at least one existing filter;
- changing the threshold while reusing a continuation fails through the
  existing continuation-fingerprint protection;
- invalid negative internal input is rejected rather than treated as unset.

Reuse the existing attempt/frame fixtures and the existing
`DirectRetryCount` tests where sensible. Do not derive matches from repeated
routes or record adjacency.

### MCP adapter and contract tests

Extend tests under `loomspan-console/internal/mcpadapter/` to prove:

- the advertised frame-filter schema contains integer
  `minDirectRetries` with minimum 1;
- the handler passes the filter through and returns only matching frames;
- `MODEL_ATTEMPT_FAILED` fallback text contains every present normalized
  failure field listed in this ticket;
- absent optional HTTP/provider fields are omitted, not rendered as zero,
  blank, `unknown`, or inferred text;
- untrusted provider strings remain safely quoted and one physical record
  remains one fallback line;
- the structured result is unchanged by the fallback enhancement;
- response admission and continuation remain complete-item based with the
  larger fallback line.

Use a checked-in sanitized fixture with recorded failure metadata, such as the
existing timeout-step-failure corpus, or construct the smallest existing-style
test value. Do not copy prompts, model output, or other application content
from the live motivating trace.

### Discovery snapshot and byte budgets

Adding one schema property changes the exact `tools/list` response. Update all
of the following from generated test evidence rather than hand-estimation:

- the checked-in `internal/mcpadapter/testdata/tools-list-response.json`;
- the exact expected serialized byte count in `server_test.go`;
- the measured `tools/list` value in
  `docs/mcp-client-compatibility.md` when that table remains the current
  release evidence.

The response must remain below the existing committed discovery ceiling. Do
not raise a ceiling merely to make the test pass without first demonstrating
that the new minimal schema cannot fit and obtaining an explicit scope change.

### Verification commands

At minimum, run from `loomspan-console/`:

```text
go test ./internal/traceanalysis ./internal/mcpadapter
go test ./...
uv run --frozen --project skills-ref-validation skills-ref validate ./agent-skills/loomspan
```

Also run repository formatting/check commands required by the changed files,
regenerate the MCP snapshot through its existing guarded test mechanism, and
run `git diff --check` from the repository root.

Do not add a Gemini-, Codex-, or Antigravity-specific golden answer. The Go
contract, fixture, schema, fallback, budget, and continuation tests are the
portable acceptance evidence.

## Documentation requirements

Update only documentation that states the affected MCP/debugging contract:

- `loomspan-console/README.md`: document `minDirectRetries` alongside existing
  frame-query filters and keep the evidence/calculation distinction explicit.
- `loomspan-console/agent-skills/loomspan/references/mcp-tool-guide.md`: mention
  that `minDirectRetries` selects frames with explicitly attributed later
  attempts. Keep this concise; the tool schema remains the mechanical
  authority.
- `loomspan-console/docs/mcp-client-compatibility.md`: update exact discovery
  measurements as described above.

The `ai/skill-authoring/` knowledge base has **no impact**. This change affects
the separate Console debugging skill and MCP diagnostic interface; it does not
change how application authors define Loomspan skills, manifests, mappings,
inputs, outputs, planning, or runtime behavior. Do not update the authoring
coverage table.

## Contract and compatibility classification

| Surface | Classification | Treatment |
| --- | --- | --- |
| Public Java Application API | No impact | No Java production type changes; no allowlist change |
| Supported Java SPI | No impact | Loomspan exposes no supported SPI here |
| Configuration and manifest contracts | No impact | No `loomspan.*` property or skill YAML change |
| Persisted or serialized contracts | No durable-contract impact | Do not change the Java trace writer, NDJSON records, compatibility marker, or portable-file policy |
| Ephemeral diagnostic formats | Affected | Add one pre-v1 MCP query input and improve current-version fallback coherence |
| Internal implementation | Affected | Update internal Go filter, adapter projection, tests, snapshots, and docs atomically |

The MCP contract is unreleased and explicitly pre-v1. Make the change in place
with **no shim, alias, dual field, fallback reader, or deprecated input**.

Java-to-Go application-adapter boundary coordination is **not required**. The
Go analyzer already consumes the required attempt metadata and already stores
`DirectRetryCount`; this ticket changes only how current Console evidence is
selected and projected through MCP. If implementation discovers that a Java
writer, application-adapter REST/SSE response, consumed NDJSON field, or
compatibility marker must change, stop and treat that as a ticket-scope
mismatch rather than silently broadening PR 33.

## Guardrails

- Return recorded values and existing mechanical counts only.
- Keep evidence and calculation labeled accurately; a count-based selection is
  not a causal claim.
- Do not create a `cause` field or natural-language failure summary.
- Do not add `inlineTypes`, `omitPromptBodies`, `hasValidationIssues`,
  `retryFrames`, `minAttempts`, or another specialized trace tool.
- Do not change `LOOMSPAN_get_trace` or add an unbounded retry-frame array.
- Do not change frame-type semantics.
- Do not add raw-artifact traversal to the ordinary debugging workflow.
- Do not add model/client-specific production behavior or schemas.
- Do not expose new application content or weaken untrusted-data handling.
- Do not raise response budgets without an explicitly approved ticket change.
- Preserve all existing filters, ordering, pagination, continuation, response
  admission, and structured-result behavior outside the narrow additions.

## Acceptance signals

- A text-only MCP consumer can identify the recorded timeout classification
  and retry decision for a returned `MODEL_ATTEMPT_FAILED` record without raw
  artifact inspection.
- The fallback contains no cause or diagnostic value absent from the structured
  attempt facts.
- `LOOMSPAN_query_trace_frames` with `minDirectRetries: 1` returns only frames
  whose existing `directRetryCount >= 1`.
- Threshold, exact-frame attribution, AND composition, projection/order, and
  continuation semantics are covered by focused tests.
- The tools snapshot and measured discovery bytes match the implemented schema
  and remain within the existing ceiling.
- Console README, portable skill guidance, tool descriptions, schemas,
  structured output, and text fallback agree.
- `go test ./...`, the portable skill validator, and `git diff --check` pass.
- No Java public surface, supported SPI, configuration/manifest contract,
  durable trace contract, or application-adapter boundary changed.

## Out of scope

- Synthesized root-cause summaries or exception messages.
- New failure capture in the Java runtime.
- Changes to provider retry policy.
- Validation-filter aliases or generalized anomaly search.
- Selective inline-content controls.
- Retry-frame indexes in `LOOMSPAN_get_trace`.
- Cross-trace search, historical analytics, or durable Console history.
- Model-specific prompts, schemas, or compatibility workarounds.
- Changes to browser trace presentation unless a failing current contract test
  proves the browser consumes and requires the same MCP-only behavior.

## Recommended development process

Do **not** run the full five-step research/plan/testing-plan/implementation/
review process for this ticket.

The motivating behavior, current implementation path, design choice, contract
classification, non-goals, test matrix, documentation impact, and acceptance
criteria are already captured here. Separate research, implementation-plan,
and testing-plan artifacts would largely repeat this ticket without reducing a
material unknown.

Recommended workflow in a fresh context:

1. Read this ticket completely, then read
   `ai/thoughts/framework-feature-design-lens.md` and the referenced production
   and test files.
2. Verify that current `main` still matches the ticket's assumptions.
3. Implement directly with failing focused tests first.
4. Run the focused and full verification commands above.
5. Run the independent `ai/commands/5_code_review.md` process against the
   completed diff.

Escalate to the full planning process only if fresh research reveals a need to
change the Java writer, NDJSON/application-adapter boundary, compatibility
marker, response ceilings, or a broader record-fallback contract than the
narrow behavior approved here.
