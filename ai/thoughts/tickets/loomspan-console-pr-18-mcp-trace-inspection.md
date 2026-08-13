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

## Out of scope

Automatic diagnosis, full-runtime dump, server-driven context injection, Agent
Skill instructions, historical trace migration, persistent imported-trace
history, and any MCP-owned evidence lifecycle.
