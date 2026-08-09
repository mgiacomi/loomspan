# PR 22 - Make Failed Traces Explorable with Frame-Linked Diagnostics

## Status

Implementation-ready ticket. Planning and codebase verification completed on
2026-08-08 against the Loomspan repository. Depends on PR 01, PR 12, PR 13,
and PR 14. No implementation has started.

This ticket changes the canonical Java trace contract, Go analysis contract,
and browser failure experience in lockstep. It intentionally follows PR 21 so
the trace identity changes in that ticket are preserved while fixtures are
regenerated.

## Outcome

Every normally finalized failed or aborted skill trace is a valid, acquirable,
and explorable trace. The developer can inspect the complete frame hierarchy up
to the failure and open the failure attached to the closest frame that observed
it without reading raw NDJSON.

`ERROR_RECORDED` carries bounded typed text diagnostics. The first required
diagnostic is the ordinary Java stack trace, preserved as familiar text rather
than decomposed into a Java/Go/TypeScript stack-frame schema. The ordered
diagnostic shape remains suitable for later provider, client-library, and
transport evidence, but this PR emits only the required Java stack trace. PR 23
owns the first additional evidence producer and the category names justified by
that producer.

## Triggering incident and verified current behavior

The motivating `handleIncident` execution completed planning, classification,
network investigation, and runbook lookup, then failed during the fourth model
call. OpenRouter returned a documented error-shaped completion with
`finish_reason: "error"`. Spring AI 1.1.6 attempted to deserialize that value
as `OpenAiApi.ChatCompletionFinishReason`, rejected the unknown enum value, and
threw `RestClientException` before Loomspan received a model response.

The canonical retained trace was otherwise complete:

- 144 physical records with strictly increasing identity and sequence;
- all opened frames closed, including the failed model, step, and root frames;
- `ERROR_RECORDED` followed by final `TRACE_COMPLETED`;
- outcome `FAILED`, `remainingFrames: 0`, and a complete terminal usage snapshot;
- the interrupted attempt correctly represented by request-prepared and
  request-sent records without a response-received record; and
- the same failure ID present on `ERROR_RECORDED` and
  `TRACE_COMPLETED.terminalFailureId`.

The Go analysis processor nevertheless rejected the artifact as
`INVALID_TERMINAL_FAILURE`. Java's real runtime error path writes only
`failureId` on `ERROR_RECORDED`; Go defaults a missing `terminal` flag to false
and requires the completion-linked error to carry `terminal: true`.

The cross-language fixture generator manually writes `terminal: true` for its
valid failed and aborted traces, so fixture agreement did not exercise the real
runtime producer. Java runtime tests assert that the failure IDs match but do
not assert terminal metadata or run the produced artifact through Go analysis.

The failure record is also associated with the root mission after the actual
model and step frames have unwound. Frame-close metadata records only a safe
exception type and generic message. `TraceFailureMetadata` deliberately omits
the exception message, cause chain, suppressed exceptions, and stack trace.
Consequently the raw artifact identifies the Spring client exception class but
does not contain the Java evidence shown on the HTTP error page.

## Resolved design

### Scope-discipline decisions

The implementation plan deliberately narrows several initially proposed pieces:

- Because one diagnostic is capped at 1 MiB, retrieval returns the selected
  diagnostic in one bounded, deliberate response. This PR does not add a fourth
  range source, cursor operation, or continuation contract.
- The canonical diagnostics array remains generic and accepts unknown nonblank
  kinds, but PR 22 emits only `JAVA_STACK_TRACE`. PR 23 owns the first real
  provider/client producer and the category names justified by that evidence.
- The failure recorder owns bounded diagnostic construction. This PR does not
  add a separately wired attachment service or synthetic omission diagnostics
  before a multi-source producer exists.
- Trace Explorer provides explicit load, wrap/no-wrap, copy, and text download.
  Browser-native find is sufficient for the bounded first version; a custom
  search widget is out of scope.

These reductions preserve the incident-driven goals while avoiding speculative
protocol, service, and UI contracts.

### 1. Terminality comes from the completion link

Remove `terminal` from the canonical `ERROR_RECORDED` metadata contract.
Whether a recorded failure ultimately terminated the trace is not known at
every useful capture point and must not be guessed there.

`TRACE_COMPLETED.terminalFailureId` is the sole terminal-failure authority:

- `SUCCEEDED` forbids a terminal failure ID;
- `FAILED` and `ABORTED` require one nonblank terminal failure ID;
- that ID must resolve to exactly one canonical `ERROR_RECORDED` failure fact;
  and
- the derived failure summary reports `terminal: true` only for that linked
  failure. Every other recorded failure is derived as nonterminal.

This is a contract correction, not a compatibility fallback for malformed
traces. Change Java fixtures, Go validation, indexes, DTOs, and tests in place.
Do not accept an unresolvable or duplicate terminal failure ID.

### 2. Record once at the closest failure frame

Capture a failure while the closest meaningful execution frame is still known,
before that frame is closed and the exception unwinds to the root coordinator.
The resulting `ERROR_RECORDED.frameId`, `parentFrameId`, `frameType`, and `route`
come from that frame through the ordinary trace-record mechanism. Do not add an
`ERROR_FRAME` or any other synthetic execution frame.

Add one session-local failure-recording service or equivalent internal state
that:

- assigns a nonblank opaque `failureId` on first recording;
- remembers the failure ID and origin frame for the lifetime of the session;
- recognizes the same throwable again as it propagates and also resolves a
  normal wrapper through its cause chain;
- returns the existing failure ID instead of appending duplicate terminal
  records at each catch boundary; and
- lets the top-level coordinator use the propagated failure ID in frame-close
  metadata and `TraceCompletion`.

Use throwable identity and explicit cause traversal, not exception class or
message equality. Distinct throwable instances remain distinct failures even
when their text is identical. Bound cause traversal and reject cycles. The
session-local registry is discarded with the session and is never static,
cross-session, persisted independently, or exposed as a public API.

Required capture seams include:

- planning-model failure while the planning model frame is active;
- step-model failure while the step model frame is active;
- tool failure while the tool frame is active;
- advisor/linter/output validation exhaustion at the closest owning frame;
- mission or nested-skill failure not already recorded below;
- timeout, interruption, and cancellation before cleanup changes frame state;
- cleanup/finalization failure when it becomes the terminal execution failure;
  and
- the runner fallback for failure before any frame opens.

An error before frame creation remains a valid unframed `ERROR_RECORDED`; do not
invent a root frame. A caught and recovered lower-level error remains recorded
and becomes nonterminal because the final completion does not link to it.

### 3. Keep diagnostic content as bounded text

Do not parse Java stack trace lines or add stack-frame, cause, or suppressed
exception DTOs. Capture `Throwable.printStackTrace(PrintWriter)` output as one
UTF-8 text diagnostic. That familiar representation already preserves the
exception message, frames, causes, suppressed exceptions, and JVM formatting
developers and LLMs expect.

Each `ERROR_RECORDED` data object retains the existing bounded contextual fields
such as `exceptionType`, skill name, objective, and safe summary where
applicable, and adds an ordered `diagnostics` array. PR 22 emits exactly one
diagnostic for each recorded `Throwable`, with this shape:

```json
{
  "kind": "JAVA_STACK_TRACE",
  "contentType": "text/plain; charset=utf-8",
  "text": "java.lang.IllegalStateException: ...",
  "truncated": false,
  "captureLimitBytes": 1048576
}
```

Contract rules:

- `kind` is a nonblank bounded diagnostic-category identifier, not the Java
  exception class. The only framework-owned kind introduced by this PR is
  `JAVA_STACK_TRACE`. Go and browser consumers accept unknown nonblank kinds as
  opaque text so later PRs can add evidence without changing the container
  shape.
- The ordered array may contain multiple entries of the same kind. Do not use a
  map, because retries and layered clients can produce repeated diagnostics and
  ordering is useful evidence.
- `contentType` is required and bounded. All diagnostics in this release are
  UTF-8 text even when the media type is `application/json`; no binary content
  is admitted.
- `text` is the opaque captured value. Go and the browser must not parse Java
  stack syntax, provider JSON, or client-library prose to infer cause.
- `truncated` and `captureLimitBytes` are required. The limit records the bound
  used for that item, not a claim about the original content size.
- Unknown nonblank diagnostic kinds remain valid opaque text and receive a
  generic browser label. This is extensible diagnostic content, not a reason to
  reject an otherwise current-version trace.

Use a 1 MiB UTF-8 byte limit per diagnostic, at most 16 diagnostics per error,
and a 4 MiB aggregate diagnostic-text limit per error. Go validates all three
canonical limits, while the PR 22 Java producer emits one stack diagnostic.
Apply the per-item bound while capturing so an unbounded intermediate `String`,
`StringWriter`, provider body, or exception graph is not required.

When the stack trace exceeds its bound, retain a UTF-8-safe head and tail
with an explicit in-band omission marker, set `truncated: true`, and keep the
total captured bytes within the declared limit. Preserve substantially more of
the head than the tail while retaining enough tail for root-cause lines. PR 22
does not define a synthetic "diagnostics omitted" item: count/aggregate omission
semantics belong with the first real multi-source producer in PR 23.

This ticket intentionally does not add secret detection, field redaction,
message suppression, or configurable sensitivity modes. Error messages and
diagnostic blobs are application diagnostic content and may contain sensitive
information. The existing authenticated local-console trust warning must be
updated to state that explicitly. Designing finer-grained capture/redaction
policy is deferred.

### 4. Keep the diagnostic producer narrow and extensible

Introduce the internal diagnostic value shape and keep construction inside the
central failure recorder. The recorder may accept already-bounded additional
diagnostics without changing exception propagation or execution outcome, but
PR 22 does not introduce a separately wired attachment service, provider/client
collector, or speculative producer-specific category catalog.

The Java throwable stack trace is required for every recorded Throwable. Later
catch boundaries that actually observe provider or client textual failure
evidence can attach it without embedding provider-specific fields into the
canonical failure schema.

The triggering Spring AI/OpenRouter failure proves that a stack trace and a
provider error body are different evidence. Preserve a provider/client body
only at a seam that actually observes it before conversion loss. Do not infer or
reconstruct an OpenRouter error from the Jackson exception message, do not use
reflection against Spring AI internals, and do not intercept all successful
model responses merely to search for failures.

The current Spring AI `OpenAiApi` construction seam does not expose the
error-shaped response before conversion loss. Land the generic diagnostic value
shape and Java stack trace in this PR, document that precise limitation, and
leave OpenRouter-aware response decoding, producer-specific kinds, and any
multi-source omission behavior to dependent PR 23,
`loomspan-console-pr-23-provider-retries.md`. Do not delay failed-trace validity
or invent provider evidence. Provider retry, fallback, and recovery policy are
outside this ticket.

### 5. Analyze diagnostics without blocking failed traces

Update Go trace analysis so a valid failed or aborted trace produces a normal
installed bundle and handle. A missing model response after a prepared and sent
request remains a supported analysis gap, not invalid content.

The failure index must retain:

- failure ID and derived terminal status;
- physical record sequence and timestamp;
- frame, route, attempt, retry, and validation links already supported;
- exception type and safe/context summary when present; and
- diagnostic descriptors sufficient to retrieve each blob deliberately.

Do not copy multi-megabyte diagnostic text into failure-list rows, frame rows,
the manifest, summary DTO, cursor, or another derived component. When the error
data is chunked, keep its reconstructed bounded JSON once in the existing
payload store and retain its payload descriptor on the failure index. Failure
processing must not treat a chunk envelope with `data: null` as the complete
semantic error; validate and index its diagnostics after reconstruction. The
4 MiB canonical per-error bound permits bounded materialization of this one
logical error object when it is indexed or deliberately queried.

Add one bounded `GetFailureDiagnostic`-style query that resolves the failure's
existing payload descriptor, decodes that bounded logical error object, selects
one diagnostic ordinal, and returns its complete decoded UTF-8 text. The
canonical 1 MiB per-diagnostic limit is also the response limit, so this query
does not need a range source, cursor operation, byte offsets, or continuation
fingerprint. Do not persist a second decoded copy. The operation must still bind
scope, artifact handle, failure ID, and diagnostic ordinal while reusing existing
lease, capacity-accounting, cancellation, and target-scope rules.

Content-invalid artifacts continue to be rejected atomically. This ticket does
not add best-effort parsing of malformed JSON, inconsistent identity,
nonmonotonic sequence, invalid frame relationships, incomplete chunks, or an
unresolved terminal failure. The observed trace becomes valid because the
producer and terminal-link contract are corrected, not because validation is
globally weakened.

### 6. Present failures as first-class frame evidence

Trace Explorer keeps the hierarchy-first experience. A frame with associated
failures visibly indicates that fact without requiring the developer to open
the raw-record view. Selecting a failure shows:

- terminal or recovered status;
- exception type and contextual summary;
- originating frame and route with navigation back to that frame;
- attempt/retry/validation relationships when recorded;
- each diagnostic's kind, media type, and truncation state; and
- deliberate loading of the selected diagnostic text.

Render diagnostic content as inert preformatted text, never HTML or Markdown.
Provide explicit load, wrap/no-wrap, copy, and text-download actions. Preserve
whitespace and ordinary JVM stack formatting. Browser-native find is sufficient
for this bounded first version; a custom diagnostic-search UI is deferred. Keep
the truncation notice visible independently of the in-band marker.

`Acquire for analysis` and `Download raw attachment` remain distinct. A valid
failed trace acquires normally. When any artifact is rejected for a genuine
content invalidity, the UI must state that raw download remains available when
the domain error carries `rawDownloadAvailable: true`; do not show only the
generic validation message.

## Java implementation map

Production areas expected to change include:

- `internal/core/TraceFailureMetadata` or its replacement;
- `internal/core/DefaultExecutionTraceRecorder`;
- `internal/core/ExecutionTraceRecorder`;
- `internal/core/LoomspanSession` and its session-local failure registry;
- `internal/core/LoomspanSessionRunner`;
- `internal/core/ExecutionCoordinator`;
- `internal/runtime/state/ExecutionStateService` and
  `DefaultExecutionStateService`;
- planning and step-model catch/close paths;
- `internal/runtime/tool/DefaultToolCallbackFactory`;
- advisor, linter, and structured-output exhausted paths;
- `internal/runtime/trace/DefaultExecutionTraceHandle` for large diagnostic
  chunking through the existing canonical payload mechanism; and
- observability warnings/documentation that enumerate recorded error details.

Keep failure capture internal. Do not expose `Throwable`, diagnostic builders,
or failure-registry types through Loomspan's public skill API.

## Go and browser implementation map

Production areas expected to change include:

- `loomspan-console/internal/traceanalysis/failures.go`;
- `loomspan-console/internal/traceanalysis/model.go`;
- `loomspan-console/internal/traceanalysis/processor.go`;
- failure indexes/components with diagnostic descriptors;
- `loomspan-console/internal/traceanalysis/dto.go`;
- `loomspan-console/internal/traceanalysis/query_facts.go` and one bounded
  whole-diagnostic query;
- `loomspan-console/internal/browserapi/router.go` and trace-analysis handlers;
- `loomspan-console/web/src/api/contracts.ts`;
- Trace Explorer frame, failure, and diagnostic views; and
- Trace Detail's invalid-artifact/raw-download guidance.

Browser and future MCP adapters consume the shared transport-neutral analysis
service. Do not create browser-only stack-trace parsing or a second failure
index.

## Contract and fixture changes

Regenerate the Java-owned trace-analysis corpus after intentionally changing
the canonical `ERROR_RECORDED` contract:

- valid failed and aborted fixtures no longer write or require an error-record
  `terminal` flag;
- their completion records continue to carry resolvable terminal failure IDs;
- valid failure fixtures include at least one bounded `JAVA_STACK_TRACE`
  diagnostic;
- nonterminal-error-then-success remains valid and derives nonterminal status;
- invalid terminal-link fixtures retain missing, duplicate, and contradictory
  cases; and
- add large, truncated, unknown-kind, and malformed diagnostic cases. Repeated
  kinds remain structurally valid, but multi-source production and omission
  behavior are deferred to PR 23.

Most importantly, add a cross-runtime integration fixture produced by an actual
failing `LoomspanSessionRunner` or skill execution path. Feed those exact bytes
through the Go processor and assert a published analysis result. A hand-built
fixture alone is insufficient for terminal-failure conformance.

Because PR 21 changes application trace identity but not canonical NDJSON, land
or rebase after PR 21 and preserve its required `entrySkill` contracts. Inspect
fixture regeneration rather than accepting unrelated corpus churn.

## Required semantic tests

### Java failure lifecycle tests

- A model exception is recorded against the active model frame before it closes.
- A tool exception is recorded against the active tool frame before it closes.
- The same throwable observed at nested, mission, and runner boundaries produces
  one canonical error record and one stable failure ID.
- A normal wrapper whose cause was already recorded resolves to the existing
  failure without class/message matching.
- Distinct throwable instances with identical text remain distinct failures.
- Cause cycles and deep wrapper chains are bounded without hanging.
- A failure before the first frame produces a valid unframed error.
- Recovered errors are not referenced by a successful completion.
- Failed and aborted completion records reference exactly one existing failure.
- Stack output includes the exception message, cause chain, and suppressed
  exceptions exactly as ordinary `printStackTrace` text.
- UTF-8 byte bounds preserve valid text, explicit head/tail omission, and a true
  truncation flag without allocating an unbounded intermediate string.
- Diagnostic count and aggregate bounds remain deterministic and cannot cause
  unbounded capture; PR 22 does not emit synthetic omission diagnostics.
- Failure recording cannot replace, suppress, or mutate the exception delivered
  to existing callers.

### Java/Go trace contract tests

- A real runtime-produced failed trace is accepted by Go and yields outcome
  `FAILED`, complete frames, one terminal failure, and the interrupted-attempt
  response gap.
- A real runtime-produced aborted trace is accepted with the same linkage.
- Go derives terminality exclusively from the completion link.
- Success with a terminal failure ID, failure without one, an unknown ID, and
  ambiguous duplicate IDs are rejected as `INVALID_TERMINAL_FAILURE`.
- Unknown diagnostic kinds remain retrievable opaque text.
- Missing required diagnostic fields, invalid UTF-8, invalid bounds, or content
  beyond declared/canonical limits are rejected deterministically.
- Large diagnostics are stored once and do not inflate summary or failure-list
  rows.
- Whole-diagnostic retrieval is capped at 1 MiB, cancellable, and bound to the
  target scope, artifact handle, failure ID, and diagnostic ordinal.

### Browser tests

- The failed trace opens in Trace Explorer and shows all frames through the
  failed model/step/root path.
- A frame-linked failure navigates in both directions between the frame and
  failure detail.
- Terminal and recovered failures are clearly distinguishable.
- Diagnostic text preserves whitespace and renders inertly.
- Explicit load, wrap/no-wrap, copy, download, and truncation notice work for
  representative large text; browser-native find works on the loaded text.
- Unknown diagnostic kinds receive a generic text presentation.
- An analysis rejection with raw download available explicitly offers the raw
  attachment path.
- Keyboard navigation, focus restoration, and screen-reader labels cover the
  new failure and diagnostic controls.

## Acceptance signals

- Real normally finalized failed and aborted traces acquire and open in the
  console without raw-NDJSON inspection.
- The triggering missing-`terminal` failure cannot recur because terminality is
  derived from `TRACE_COMPLETED.terminalFailureId`.
- The closest known failing frame owns the canonical error record; no synthetic
  error frame is introduced.
- Every recorded Throwable has a bounded ordinary Java stack-trace diagnostic.
- Large diagnostic text is deliberately retrievable without being copied into
  list, summary, manifest, or cursor responses.
- The console presents complete pre-failure frames, failure linkage, and text
  diagnostics with explicit missing/truncated facts.
- Java-produced runtime artifacts, Java fixtures, Go analysis, browser DTOs, and
  UI tests agree on one contract.
- Raw artifact download remains available independently of analysis validity.

## Guardrails

- Do not model exceptions as execution frames.
- Do not parse or normalize Java stack trace text in Java, Go, TypeScript, or
  MCP-facing services.
- Do not infer failure identity, terminality, origin, or causality from text,
  adjacency, exception class, or message equality.
- Do not duplicate diagnostic blobs across derived indexes or eagerly return
  them in summaries.
- Do not weaken unrelated artifact validation to admit failed traces.
- Do not make optional console observation affect canonical trace finalization
  or exception propagation.
- Do not add secret scanning, diagnostic redaction, or a sensitivity-mode matrix
  in this PR; update warnings and defer that policy explicitly.
- Do not capture Loomspan console pairing secrets, browser cookies, CSRF tokens,
  MCP credentials, or application observability authentication headers as
  diagnostics.
- Preserve existing user-authored console UI changes in the working tree and
  avoid unrelated cleanup.

## Compatibility position

Loomspan is pre-release and Java, Go, TypeScript, fixtures, and the browser move
in lockstep under the exact release-derived `consoleCompatibilityVersion`.
Change the canonical failure contract in place. Do not add legacy readers,
dual terminality rules, nullable shims, or artifact migration.

Already finalized artifacts from the prior release are not cataloged across an
application restart and are not a protected compatibility surface. The raw
attachment remains the unchanged source artifact for any manually retained old
trace.

## Verification sequence

Run focused tests during implementation, then complete:

```powershell
.\mvnw.cmd -pl loomspan-spring-boot-starter test -DfailIfNoTests=false
.\mvnw.cmd -pl loomspan-spring-boot-starter test -Dtest=ConsoleTraceFixtureCorpusTest -Dloomspan.console.fixtures.regenerate=true -DfailIfNoTests=false
Set-Location loomspan-console
go test ./...
go run ./internal/buildtool verify
$env:PATH = "C:\msys64\mingw64\bin;" + $env:PATH
$env:CGO_ENABLED = "1"
go test -race ./...
```

Inspect regenerated fixtures and verify that changes are limited to the intended
failure and diagnostic contract plus PR 21 facts already present on the base.
Run the real-runtime-failure integration fixture through Go rather than assuming
hand-authored corpus success proves producer conformance.

## Out of scope

- Provider retry, OpenRouter-aware response decoding, routing fallback, or
  recovery behavior; dependent PR 23 owns the retry and decoding work.
- Provider/client/transport diagnostic producers and their framework-owned kind
  names; PR 23 introduces only those justified by an actual evidence seam.
- Synthetic diagnostic-omission items and multi-source attachment policy.
- Diagnostic range/cursor protocols and a custom in-page search interface.
- Claiming a provider error code or body that the current client integration did
  not actually expose.
- Replacing or forking Spring AI solely to decode OpenRouter error envelopes;
  create a focused follow-up ticket if the existing transport has no supported
  failure-body seam.
- Structured Java stack-frame, cause, suppressed-exception, or source-code DTOs.
- Stack-frame source links, IDE protocol links, or repository/source correlation.
- Secret detection, application-content redaction, sensitivity classification,
  or configurable failure-detail modes.
- Best-effort analysis of genuinely malformed or incomplete artifacts.
- Durable cross-process trace history.
- MCP adapter or debugging-skill changes. PR 18 and PR 19 should later consume
  the shared bounded failure/diagnostic service without inventing another
  representation.
