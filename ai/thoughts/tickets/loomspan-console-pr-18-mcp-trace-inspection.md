# PR 18 — Trace-Inspection MCP Surface

## Status

Proposed ticket brief. Depends on PRs 17 and 25 and reuses PRs 12–13.

## Outcome

Expose progressive, caller-directed, continuable target and imported trace
evidence to MCP through the same evidence ownership, artifact handles,
acquisition/import lifecycle, calculations, and query services used by the
browser.

## In scope

- Add `LOOMSPAN_list_traces` and `LOOMSPAN_get_trace` over selected-target and
  currently imported transient evidence, including an explicit `TARGET` or
  `IMPORTED` source fact. Imported inspection remains available without a
  selected target.
- Add frame and record query tools, payload-range reading, and optional raw
  artifact-range reading.
- Advertise `loomspan.trace-inspection.v1` only with its complete required tool
  family, and advertise `loomspan.raw-artifact-inspection.v1` only when the
  optional raw-artifact operation is present.
- Map failure, hierarchy, timing, usage, attempt, validation, gap, availability,
  uncertainty, and evidence-source facts without recomputation.
- Bind continuations to evidence owner, artifact handle, query, filters,
  ordering, and installed-copy lifetime. Preserve target-scope binding for
  target-owned evidence without inventing a target for imports.
- Add supplementary resource templates while keeping essential workflows
  tool-complete.
- Test joined browser/MCP acquisition and imported-evidence access, shared
  pin/TTL/removal behavior, target rotation, cancellation, expiration,
  malformed data, broad traversal, and multiple clients.

## Guardrails

- No MCP-specific acquisition, import, copy, transient catalog, cache, index,
  parser, calculation, last-use time, capacity policy, or error meaning.
- Default results are concise, but deliberate raw and complete inspection
  remains possible through finite calls.
- Returned runtime content cannot trigger another operation or any server-side
  authority.
- Missing evidence and unavailable evidence are not rewritten as conclusions.
- Imported evidence is not presented as target-owned, authenticated application
  evidence or a durable historical record.

## Acceptance signals

- All required `loomspan.trace-inspection.v1` operations and the optional raw
  capability conform to their advertised semantics for target and imported
  evidence.
- Capability conformance rejects either trace capability when any operation or
  semantic promise required by that capability is absent.
- Browser and MCP observe the same source identity, imported-evidence
  discovery, handle invalidation, target-rotation behavior, and calculated
  facts.
- Imported inspection works without a selected target and disappears under PR
  25's ordinary transient removal, expiry, shutdown, and restart rules.
- Oversized, truncated, expired, removed, incompatible, and unsafe artifacts
  fail with precise shared meanings.

## Detailed-planning focus

Research query schemas, source filtering, target-optional imported discovery,
resource URI design, capability generations, continuation signing/opacity,
response framing limits, raw-range semantics, shared query pinning, client
cancellation, and representative broad-inspection behavior.

## Research handoff

Use
[`2026-08-14-loomspan-console-pr-18-mcp-trace-inspection.md`](../research/2026-08-14-loomspan-console-pr-18-mcp-trace-inspection.md)
as the codebase-research input to implementation planning. The Phase 3 tool
family and semantic boundaries remain authoritative. The follow-up review
settled or recommended the following direction:

- Define six strict MCP adapter DTO pairs with a common evidence context:
  `source`, optional `targetScopeId`, `artifactHandle`, `traceId`, `sessionId`,
  and `observedAt`. Keep deterministic fact-complete text fallbacks and golden
  schema/rendering fixtures.
- Add a transport-neutral, non-retained inventory query below both adapters.
  It joins installed target/imported copies with the current application
  catalog without creating an MCP-owned catalog. Use `ALL`, `TARGET`, and
  `IMPORTED` source filters. `ALL` and `IMPORTED` remain useful with no target;
  `TARGET` requires one. Page installed copies by deterministic keyset before
  catalog-only entries and keep application/local availability separate.
- Require `source: TARGET|IMPORTED` on `LOOMSPAN_get_trace`, plus exactly one of
  `traceId` or `artifactHandle`. Only `TARGET + traceId` acquires. Imported
  access is `IMPORTED + artifactHandle` through `evidence.ForImported()` and
  never captures or invents a target scope.
- Preserve `LOOMSPAN_query_trace_records` as a canonical-record query. Enrich
  records inside the shared trace-analysis layer with their typed attempt,
  retry, validation, failure, payload, and search facts; enrich frames with
  mechanically attributed gap and uncertainty facts. Do not implement these
  as MCP-side joins or unrelated record-query modes.
- Generalize the shared opaque `payloadRef` so failure diagnostic content can
  be read through bounded continuable ranges as well as reconstructed payload
  data. This avoids an unpageable diagnostic result and keeps raw-artifact
  inspection optional.
- Return trace-analysis cursors directly rather than double-wrapping them, but
  first replace the decoded imported `ownerKey`'s process-local owner ID with
  the adapter-safe `IMPORTED` reference. Keep strict unsigned unpadded-base64url
  cursors; handle, source, operation, fingerprint, position, and lifetime
  validation provide the binding. Inventory continuations remain a distinct
  composite token that does not expose the upstream application cursor.
- Preserve the settled target resource templates and add
  `loomspan://imports/artifacts/{artifactHandle}/summary`,
  `/frames/{frameId}`, and `/records/{sequence}`. Materialize the same JSON DTOs
  as tools through the same shared leases and domain-error mapping. Successful
  reads refresh the shared TTL; failures and cancellation do not.
- Register and advertise `loomspan.raw-artifact-inspection.v1`
  unconditionally in the standard Console binary. Its optional status applies
  to skill compatibility, not current target, authentication, storage, or
  trace availability.
- Back capability advertisement with a reviewed contract manifest and
  semantic fixtures, not required-tool membership alone. Cover target and
  target-free imported access, source/handle/cursor binding, parity,
  continuation, exact ranges, lifecycle, cancellation, and concurrent clients.

Do not inherit the current shared 1 MiB range maximum as the PR 18 product
ceiling. It is an implementation constant, not a phase decision. Retain a
modest default, but test caller-selected source-byte windows of at least 1, 4,
16, and 32 MiB, plus 64 MiB when automated host-memory tests remain healthy,
using UTF-8 and worst-case base64. Raise the shared trace-analysis maximum to
the largest exact, cancellable, interoperable value; do not add a lower
MCP-only clamp. At least 16 MiB per call is the useful target established by
automated evidence, and continuations preserve unlimited cumulative traversal.

Representative-client checks happen after the implementation is complete. They
are compatibility observations that do not affect PR completion, merge,
release, or the shared response maximum. Record later checks in
`loomspan-console/docs/mcp-client-compatibility.md` for the then-current local
Codex, Claude Code, Antigravity, Cursor, and Devin Desktop/Windsurf or local
Devin CLI builds without reopening completed automated acceptance.

## Out of scope

Automatic diagnosis, full-runtime dump, server-driven context injection, Agent
Skill instructions, historical trace migration, persistent imported-trace
history, and any MCP-owned evidence lifecycle.
