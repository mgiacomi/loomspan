# Loomspan Console Live-Inspection Follow-ups Implementation Plan

## Decision

During implementation review, the maintainer rejected the client/model matrix,
manual compatibility checklist, and client usability experiment infrastructure.
During unreleased development, the repository verification boundary is:

- deterministic Go regression tests;
- deterministic Java regression tests;
- sanitized deterministic fixtures used by those tests;
- canonical Agent Skill and release-package validation;
- official MCP conformance.

Client installation, selection, cache behavior, and subjective usability are
handled manually by the maintainer and are not persisted as repository release
evidence.

## Implementation

1. Preserve the existing MCP and Java/Go live contracts.
2. Protect stable first-admission ordering, page-local observation, later-start
   exclusion, replacement, removal, and re-admission with Java tests.
3. Update the canonical debugging skill and routed authoring guidance with the
   checkpoint, provisional pagination, purpose, and completion-race rules.
4. Remove the agent-evaluation cases, result schema, rubric, result records,
   hard-coded client matrix, buildtool commands, and evaluation-only MCP
   capability override.
5. Keep package topology, canonical-to-archive equality, result-size ceilings,
   and official MCP conformance unchanged.

## Contract Classification

- **Application API / Supported SPI:** no change.
- **Configuration and manifest contracts:** canonical six-file Agent Skill
  contents change atomically; topology and unversioned development policy stay
  unchanged.
- **Ephemeral diagnostic formats:** MCP names, schemas, continuations, text,
  structured results, Java REST DTOs, and browser facts stay unchanged.
- **Internal implementation:** the client usability experiment subsystem and
  its evaluation-only capability override are removed without a shim.

## Success Criteria

- `go test ./...`
- `go run ./internal/buildtool verify`
- `go run ./internal/buildtool mcp-conformance`
- Focused Java registry, REST pagination, and public-surface tests pass.
- Canonical skill validation and package/smoke tests pass.
- No repository command, documentation, test, or fixture requires a named
  external client, model, repetition count, manual compatibility row, or dated
  client result.
