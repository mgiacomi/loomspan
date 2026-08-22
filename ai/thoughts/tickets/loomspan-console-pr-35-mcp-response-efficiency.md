# PR 35 — MCP Dual-Output Efficiency and Client Presentation

## Status

Proposed PR 35 ticket. Begin this work only after PR 34, “Active-Execution
MCP Inspection Ergonomics and Evidence Semantics,” is committed. Start from the
then-current default branch in a fresh context and run the repository's complete
five-step process in `ai/commands/`:

1. `1_research_codebase.md`
2. `2_create_plan.md`
3. `3_testing_plan.md`
4. `4_implement_plan.md`
5. `5_code_review.md`

Do not treat the observations or candidate directions in this brief as an
approved implementation. First establish which layer creates and exposes each
copy of a result and which supported clients depend on it.

## Outcome

Reduce the model-visible size and processing cost of Loomspan MCP responses
without weakening structured-output support, deterministic text fallback,
boundedness, diagnostic safety, or supported-client compatibility.

A structured-output-capable client should not need to consume two effectively
complete serializations of the same successful result. A text-only client must
still receive sufficient deterministic evidence. The final behavior must be
based on measured MCP SDK, transport, server, and client behavior rather than
an assumption that either `content` or `structuredContent` can simply be
removed.

## Why this ticket exists

During a Codex Desktop review of the PR 34 live-execution surface on
2026-08-21, every successful Loomspan call was presented to the model twice:

- `content` contained a deterministic flattened text representation; and
- `structuredContent` contained the same result as structured JSON.

The duplication was most noticeable for
`LOOMSPAN_get_execution_activity`. A response containing only a few activity
items repeated identities, timestamps, frame paths, summaries, continuity,
coverage, usage, limits, and checkpoint facts in both forms. The calls were
functionally correct, but the repeated representation materially enlarged the
model-visible tool result and made the important facts harder to scan.

This was a manual client observation, not a reproducible benchmark. It does not
establish that Loomspan alone caused the duplication: the MCP Go SDK, server
adapter, transport envelope, Codex connector, or client presentation layer may
each affect what reaches the model. It also does not prove that omitting one
representation is protocol-valid or compatible with Loomspan's supported
clients.

No payloads, credentials, application YAML, raw trace bytes, or diagnostic
detail content were opened during the observation.

## Relationship to PR 34

PR 34 intentionally makes active-execution list text richer so text-fallback
clients can orient without undocumented-key probing. It also keeps complete
structured output for capable clients. Do not reverse those correctness and
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
- `loomspan-console/docs/mcp-client-compatibility.md` records supported-client
  behavior and discovery budgets.
- `loomspan-console/agent-evals/` contains tools-only and skill-assisted client
  evaluations that may provide a safe measurement harness.
- The version of `github.com/modelcontextprotocol/go-sdk` selected by the
  current checkout governs server result construction and protocol behavior.

Do not assume the local installed skill under a developer's Codex home is the
canonical repository skill. The repository authority remains
`loomspan-console/agent-skills/loomspan/`.

## Problem statements to resolve

### 1. The source of duplication is not yet isolated

Determine whether both representations are authored by Loomspan, synthesized
or normalized by the MCP SDK, repeated by the connector, or deliberately
presented by the client. Inspect the raw JSON-RPC response separately from the
model-visible client event when the supported harness permits it.

### 2. Client capability and fallback behavior are unclear

Establish which supported clients consume `structuredContent`, which consume
only text `content`, which expose both to the model, and whether MCP provides a
reliable negotiated capability that can alter result representation. Do not
infer client behavior from one Codex Desktop observation.

### 3. Byte cost and model cost are unmeasured

Measure representative runtime, trace, execution-list, execution-detail, and
activity calls at the raw protocol and client-event boundaries. Separate
`tools/list` discovery size from `tools/call` result size. Report bytes and
duplicated fields; do not calculate currency cost.

### 4. Text fallback has independent safety and compatibility value

The deterministic text representation supports clients that cannot use
structured output and deliberately omits or bounds some untrusted content.
Any optimization must preserve evidence fidelity, stable identifiers, typed
absence, result limits, and sensitive-data restraint. A shorter narrative that
drops required facts is not an acceptable optimization.

### 5. Large results may be externalized or truncated by clients

Determine whether duplicated representations push otherwise valid bounded
Loomspan results across known client presentation thresholds. Keep Console's
source-byte/result budgets separate from client display or externalization
thresholds.

## Required research questions

1. What exact JSON-RPC response does Loomspan emit for a representative success,
   and which fields are added or transformed before the model sees it?
2. Which current MCP specification and Go SDK rules govern `content`,
   `structuredContent`, and output schemas?
3. Is either representation mandatory, and is there capability negotiation for
   structured-only or text-only results?
4. Which supported clients consume text, structured content, or both? Which
   expose both copies to the model context?
5. What are the raw bytes, client-event bytes, and repeated-field contribution
   for representative small and maximum legal result pages?
6. Can deterministic text become a compact orientation/index while structured
   output remains complete without harming text-only clients or contradicting
   PR 34 acceptance criteria?
7. Could a server, connector, or client safely select one representation based
   on an explicit capability, or would that create divergent contracts?
8. How do errors and domain failures behave? Optimizing successes must not make
   failure evidence harder to understand.
9. Do current conformance tests, exact snapshots, agent evaluations, or release
   compatibility records already protect dual-output behavior?
10. What is the smallest change at the correct ownership layer? If the waste is
    client-owned, should Loomspan make no production change and instead record
    a client compatibility limitation?

## In scope

- Raw protocol and supported-client measurement of successful and failed tool
  results.
- MCP adapter, SDK integration, deterministic text, and structured-output
  behavior where Loomspan owns the result.
- Supported-client compatibility documentation and reproducible evaluation
  evidence.
- Focused changes that reduce duplicated model-visible material while
  preserving complete evidence for every protected client class.
- Exact regression tests for result shape, result bytes, and safe omission when
  the selected design makes those assertions stable and meaningful.

## Out of scope

- Redesigning PR 34's active-execution evidence semantics.
- Removing text fallback without proving all protected consumers can use
  structured output.
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
  evaluations must move together.
- Client presentation is not automatically a Loomspan-owned contract. Preserve
  the boundary between server evidence and client rendering.
- Trace and live-execution contents remain ephemeral diagnostic formats.
- No supported application-facing Java API or SPI change is expected. If a
  production Java type changes unexpectedly, run
  `LoomspanPublicSurfaceArchitectureTest` and explain why the change belongs.

## Acceptance signals

- The research artifact identifies the exact layer responsible for each copy
  of a representative result.
- Before/after measurements cover raw JSON-RPC and at least one protected
  model-visible client-event path.
- Structured-output clients retain the complete validated result and text-only
  clients retain every fact required by their protected workflows.
- The chosen behavior is MCP-conformant and does not rely on undocumented
  client heuristics.
- Maximum legal pages remain bounded and consist only of complete items.
- Domain errors remain concise, exact, and recoverable.
- Untrusted details and sensitive content are not expanded into text or logs.
- Compatibility documentation states which clients receive one or both
  representations and records any limitation that Loomspan cannot control.
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
- Tools-only and skill-assisted agent-evaluation cases using sanitized data.
- Supported-client event capture described by
  `loomspan-console/agent-evals/README.md`.
- `go test ./...`, `go run ./internal/buildtool mcp-conformance`, and
  `go run ./internal/buildtool verify` from `loomspan-console/`.
- Browser or Java suites only if research finds a real affected boundary.

## Documentation impact

Inspect and update as applicable:

- `loomspan-console/docs/mcp-client-compatibility.md`
- `loomspan-console/README.md`
- `loomspan-console/agent-evals/README.md`
- affected agent-evaluation cases/results
- the canonical Loomspan Agent Skill only if its workflow must distinguish
  result representations; do not teach client-specific wire internals without
  a user-facing reason
- the active roadmap if this work changes the pre-v1 contract gate

This is runtime-debugging transport/presentation work, not ordinary Loomspan
YAML skill authoring. The implementation plan must still perform the required
`ai/skill-authoring/` impact assessment.

## Guardrails

- Treat the Codex Desktop observation as a lead, not proof of ownership or
  general client behavior.
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
- Every supported client class has an explicit result-consumption expectation.
- Measurements and compatibility conclusions are reproducible and sanitized.
- Automated checks protect the selected representation behavior and budgets.
- Documentation and evaluation evidence agree with the implementation.
- Step 5 reports no unresolved blocking correctness, safety, compatibility, or
  evidence-fidelity finding.
