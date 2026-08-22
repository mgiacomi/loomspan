# PR 35 — MCP Dual-Output Efficiency and Presentation Boundaries

## Status

Research completed on 2026-08-21 and recorded in
`ai/thoughts/research/2026-08-21-loomspan-console-pr-35-mcp-dual-output-boundaries.md`.
The live follow-up used two independently started executions. No implementation
direction is approved yet. Continue with the remaining repository process only
after deciding that the measured savings justify a production change:

1. Use the completed `1_research_codebase.md` artifact as the evidence base.
2. `2_create_plan.md`
3. `3_testing_plan.md`
4. `4_implement_plan.md`
5. `5_code_review.md`

Do not treat the observations or candidate directions in this brief as an
approved implementation. First establish which layer creates and exposes each
copy of a result and which supported MCP requirement, if any, protects it.

The repository does not maintain client/model usability experiments, a manual
compatibility checklist, or persisted client result records during development.
Use deterministic protocol measurements, Go tests, package validation, and
official MCP conformance; the maintainer handles any additional client checks
manually outside the repository.

## Outcome

Reduce duplicated server-emitted material and its potential model-processing
cost without weakening structured-output support, deterministic text fallback,
boundedness, diagnostic safety, or the supported MCP contract.

A structured-output-capable consumer should not need to process two effectively
complete serializations of the same successful result. A text-only consumer must
still receive sufficient deterministic evidence. The final behavior must be
based on measured MCP SDK, transport, and server behavior rather than an
assumption that either `content` or `structuredContent` can simply be removed.

Repository conclusions must rest on server-owned protocol behavior. Any
maintainer-observed client presentation is contextual evidence, not a
repository compatibility gate.

## Why this ticket exists

During a Codex Desktop review of the PR 34 live-execution surface on
2026-08-21, successful Loomspan calls returned both representations to the
Codex-side callable MCP integration boundary:

- `content` contained a deterministic flattened text representation; and
- `structuredContent` contained the same result as structured JSON.

Two live executions reproduced the active-detail behavior across the
`planTrip` and `handleIncident` entry skills. The first active-detail response
contained 48 structured leaf facts, all repeated line-for-line in the 49-line
text fallback; the extra line was the derived `activePath.count`. Its combined
Codex-side returned object was 3,955 UTF-8 bytes. The second response had the
same 49-line shape and a combined returned-object size of 4,027 characters.

An eight-item `LOOMSPAN_get_execution_activity` response contained 108 text
lines that exactly matched structured leaves, while 23 arbitrary `details`
leaves intentionally remained structured-only. Its combined Codex-side
returned object was 9,998 UTF-8 bytes. These are reproducible research samples,
not maximum-page benchmarks or release budgets.

The live observation proves that both encodings reach Codex's callable MCP
result. It does not prove that every Codex host path places both encodings into
model context: the local orchestration wrapper demonstrably could forward only
the text representation. Do not describe this ticket or its eventual code
review as proving universal model-visible duplication.

No payloads, credentials, application YAML, raw trace bytes, or diagnostic
detail content were opened during the observation.

## Relationship to PR 34

PR 34 intentionally makes active-execution list text richer so text-fallback
consumers can orient without undocumented-key probing. It also keeps complete
structured output for capable consumers. Do not reverse those correctness and
discoverability gains merely to reduce bytes.

PR 34's active-execution semantics, cursor coverage facts, continuation
behavior, usage accounting, and canonical Agent Skill workflow are inputs to
this ticket, not subjects to redesign here. If research uncovers a correctness
defect in one of those contracts, record it separately rather than silently
folding it into a presentation optimization.

## Initial implementation map to verify

Fresh research must re-establish these locations after PR 34 lands:

- `loomspan-console/internal/mcpadapter/` constructs MCP tool results,
  deterministic text, structured envelopes, compact output schemas, and
  complete-output validation.
- `loomspan-console/internal/mcpadapter/executions.go` and `activity.go` render
  the active-execution text fallbacks that exposed the motivating duplication.
- `loomspan-console/internal/mcpadapter/contracts.go` defines complete result
  DTOs.
- `loomspan-console/internal/mcpadapter/output_schemas.go` defines compact
  discovery schemas and protects the exact `tools/list` budget.
- `loomspan-console/internal/mcpadapter/server_test.go` and committed test data
  protect protocol discovery and call behavior.
- `loomspan-console/docs/mcp-contract-verification.md` records deterministic
  discovery and result-size budgets.
- The version of `github.com/modelcontextprotocol/go-sdk` selected by the
  current checkout governs server result construction and protocol behavior.

Do not assume the local installed skill under a developer's Codex home is the
canonical repository skill. The repository authority remains
`loomspan-console/agent-skills/loomspan/`.

## Problem statements to resolve

### 1. Server ownership is isolated; downstream presentation is not

Loomspan authors the deterministic text and the typed result; the selected MCP
Go SDK carries the typed result as `structuredContent`. Both survive into the
Codex-side callable MCP result. Downstream selection, suppression, display, and
model-context inclusion remain client-owned. A client event may provide
contextual evidence, but it is not repository evidence or a release gate.

### 2. Protocol and fallback constraints limit the design space

The completed research found no negotiated capability by which this server can
select structured-only output for a capable caller. The MCP contract recommends
a text serialization alongside structured output for backward compatibility,
and the selected Go SDK can synthesize JSON text when content is absent. Any
plan must verify these findings against the selected dependency and preserve
generic text-fallback and structured-result behavior without creating a
named-client matrix.

### 3. Representative cost is measured; maximum-page cost remains open

The research artifact records representative runtime, trace, execution-list,
execution-detail, and activity measurements plus duplicated-field counts.
Planning must still establish stable raw-protocol measurements for small and
maximum legal pages at server-owned serialization boundaries. Keep `tools/list`
discovery size separate from `tools/call` result size and do not calculate
currency cost.

### 4. Text fallback has independent safety and compatibility value

The deterministic text representation supports consumers that cannot use
structured output and deliberately omits or bounds some untrusted content.
Any optimization must preserve evidence fidelity, stable identifiers, typed
absence, result limits, and sensitive-data restraint. A shorter narrative that
drops required facts is not an acceptable optimization.

### 5. Client presentation thresholds are not repository contracts

Keep Console's source-byte and result budgets separate from client display,
externalization, or truncation thresholds. Do not create a repository gate for
client-owned thresholds that cannot be verified deterministically. A maintainer
observation may motivate server-side measurement but does not establish a
Loomspan-owned contract.

## Required research questions

1. What exact JSON-RPC response does Loomspan emit for a representative success,
   and which fields are authored by Loomspan versus added or transformed by the
   selected Go SDK?
2. Which current MCP specification and Go SDK rules govern `content`,
   `structuredContent`, and output schemas?
3. Is either representation mandatory, and is there capability negotiation for
   structured-only or text-only results?
4. What generic text-fallback and structured-result guarantees must the
   supported MCP contract preserve?
5. What are the raw response bytes, server-owned serialization bytes, and
   repeated-field contribution for representative small and maximum legal
   result pages?
6. Can deterministic text become a compact orientation/index while structured
   output remains complete without weakening the text-fallback contract or
   contradicting PR 34 acceptance criteria?
7. Could the server or SDK safely select one representation based on an
   explicit protocol capability, or would that create divergent contracts?
8. How do errors and domain failures behave? Optimizing successes must not make
   failure evidence harder to understand.
9. Do current conformance tests, exact snapshots, and deterministic adapter
   tests already protect dual-output behavior?
10. What is the smallest change at the correct ownership layer? If the waste is
    client-owned, should Loomspan make no production change and instead record
    that ownership limitation in MCP contract documentation?

## Planning decision gate

Before approving an implementation plan, choose and justify exactly one of
these outcomes with deterministic measurements:

- Make no Loomspan production change because the actionable duplication is at
  a downstream presentation layer or the available server-owned savings do not
  justify weakening the fallback.
- Compact the Loomspan-authored text fallback while explicitly naming the
  protected text-only workflows and facts it must retain.
- Change representation behavior through a standards-based protocol mechanism
  only if fresh dependency and conformance evidence identifies one.

Do not plan unconditional removal of `content`, assume structured-output client
capability negotiation that does not exist, or claim that the live samples
prove both encodings always enter model context. A no-production-change result
is a successful resolution when supported by the evidence.

## In scope

- Raw protocol measurement of successful and failed tool results.
- MCP adapter, SDK integration, deterministic text, and structured-output
  behavior where Loomspan owns the result.
- Focused changes that reduce duplicated server-emitted material while
  preserving the supported MCP result contract.
- Exact regression tests for result shape, result bytes, and safe omission when
  the selected design makes those assertions stable and meaningful.

## Out of scope

- Redesigning PR 34's active-execution evidence semantics.
- Removing text fallback unless the supported MCP result contract and
  Loomspan's deliberate fallback guarantee are explicitly changed.
- Removing structured output or complete-output validation.
- Increasing or bypassing result limits to hide duplication.
- Adding derived health, progress, stuckness, completeness, diagnosis, or
  recommendation fields.
- Reading or embedding model prompts, tool payloads, credentials, YAML, raw
  artifacts, or arbitrary diagnostic details for measurement.
- Calculating token prices or monetary savings.
- Changing the supported top-level Java API, adding a Java SPI, or changing the
  Java application-to-Console protocol unless fresh research proves that layer
  is directly involved.

## Contract and compatibility classification

- MCP tool results, text fallbacks, structured content, schemas, and errors are
  a deliberately supported pre-v1 Console diagnostic contract. They may change
  coherently before v1, but all protected consumers, fixtures, docs, and
  automated checks must move together.
- Client presentation is not automatically a Loomspan-owned contract. Preserve
  the boundary between server evidence and client rendering.
- Trace and live-execution contents remain ephemeral diagnostic formats.
- No supported application-facing Java API or SPI change is expected. If a
  production Java type changes unexpectedly, run
  `LoomspanPublicSurfaceArchitectureTest` and explain why the change belongs.

## Acceptance signals

- The research artifact identifies the exact layer responsible for each
  server-emitted representation and keeps downstream presentation outside the
  repository evidence boundary.
- Before/after measurements cover raw JSON-RPC and every server-owned
  serialization path affected by the change.
- Structured output retains the complete validated result and deterministic
  text fallback retains every fact required by its protected workflows.
- The chosen behavior is MCP-conformant and does not rely on undocumented
  client heuristics.
- Maximum legal pages remain bounded and consist only of complete items.
- Domain errors remain concise, exact, and recoverable.
- Untrusted details and sensitive content are not expanded into text or logs.
- MCP contract documentation states what Loomspan emits, how it is bounded and
  verified, and which presentation behavior remains outside Loomspan's control.
- If no safe Loomspan-owned optimization exists, the ticket may correctly end
  with measured evidence and no production change.

## Testing and verification expectations

The step-3 testing plan must choose exact commands after the design is known.
At minimum, assess:

- MCP protocol/conformance tests and raw JSON-RPC call fixtures.
- `loomspan-console/internal/mcpadapter/*_test.go`, especially success/failure
  envelopes, text goldens, complete-output validation, security, and maximum
  page behavior.
- Exact `tools/list` snapshot tests only if discovery changes; do not conflate
  discovery bytes with call-result bytes.
- `go test ./...`, `go run ./internal/buildtool mcp-conformance`, and
  `go run ./internal/buildtool verify` from `loomspan-console/`.
- Browser or Java suites only if research finds a real affected boundary.

## Documentation impact

Inspect and update as applicable:

- `loomspan-console/docs/mcp-contract-verification.md`
- `loomspan-console/README.md`
- the canonical Loomspan Agent Skill only if its workflow must distinguish
  result representations; do not teach client-specific wire internals without
  a user-facing reason
- the active roadmap if this work changes the pre-v1 contract gate

This is runtime-debugging transport/presentation work, not ordinary Loomspan
YAML skill authoring. The implementation plan must still perform the required
`ai/skill-authoring/` impact assessment.

## Guardrails

- Treat the live Codex Desktop samples as proof that both server-emitted
  encodings reach the callable MCP result, not as proof of universal downstream
  presentation or model-context behavior.
- Measure before changing the server.
- Preserve deterministic, testable evidence; do not replace duplication with
  lossy prose.
- Keep structured and text contracts coherent and prevent silent field drift.
- Do not expose additional diagnostic content merely to make a benchmark look
  representative.
- Prefer a no-change conclusion over an optimization at the wrong layer.

## Definition of done

- Research, implementation plan, testing plan, implementation if justified,
  and final code review are recorded under `ai/thoughts/`.
- Every retained or omitted result representation has an explicit protocol and
  fallback rationale.
- Protocol measurements and conclusions are reproducible and sanitized.
- Automated checks protect the selected representation behavior and budgets.
- Documentation and automated evidence agree with the implementation.
- Step 5 reports no unresolved blocking correctness, safety, compatibility, or
  evidence-fidelity finding.
