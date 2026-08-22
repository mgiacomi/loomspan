# Loomspan Console Live-Inspection Follow-ups Testing Plan

## Verification Boundary

Only deterministic repository-controlled verification is required:

1. Go unit and integration tests for live state, MCP adapters, observability,
   canonical skill validation, packaging, and build declarations.
2. Java tests for registry admission ordering, replacement, removal,
   re-admission, page-local observation, cursor errors, and public API surface.
3. Checked-in deterministic fixtures used by automated tests.
4. Canonical six-file Agent Skill validation and archive byte equality.
5. Official MCP initialization, tool-listing, caching, and DNS-rebinding
   conformance scenarios.

## Excluded Verification

- Named-client matrices or fixed repetition counts.
- Model-behavior scoring or human rubrics.
- Persisted client transcripts or dated usability results.
- Manual compatibility checklists maintained in the repository.
- Skill-selection, cache, or client-installation attestations.

The maintainer may perform any additional client checks manually; they are not
repository gates or durable product evidence during development.

## Commands

From `loomspan-console/`:

```text
go test ./...
go run ./internal/buildtool verify
go run ./internal/buildtool mcp-conformance
```

From the repository root:

```text
.\mvnw.cmd -pl loomspan-spring-boot-starter "-Dtest=InMemoryActiveExecutionRegistryTest,ObservabilityRestIntegrationTest,LoomspanPublicSurfaceArchitectureTest" test
```

## Exit Criteria

- All commands pass.
- `tools/list` remains twelve tools and within its committed ceiling.
- No production Java type or Java/Go observability DTO changes.
- No agent-evaluation command, case corpus, result schema, result record,
  client matrix, or manual compatibility table remains.
