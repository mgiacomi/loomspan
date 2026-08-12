# Loomspan Console Trace Fixtures

This directory is the current-release Java-to-Go semantic contract for execution traces. The trace format itself is an ephemeral diagnostic format: these fixtures describe the current checkout and do not promise that older trace files remain readable.

`traces/` contains eighteen valid traces and twenty-six deliberately invalid artifacts. `expected/` contains only semantic results needed by Console analysis: identity, outcome, terminal failure, the run-start configured-limit snapshot when present, physical attempts and retry usage, usage completeness, validation-to-attempt links, root/frame hierarchy facts, inclusive/self duration availability, direct/descendant/inclusive and unframed attributed usage, terminal usage, the derived unattributed remainder, payload descriptors, gaps, uncertainties, or one invalidity category. The corpus includes planned-success and unplanned-failure tool lifecycles, nested and repeated frames, incomplete and overlapping duration cases, chunked text and JSON payloads, independently reported component totals, and minimal chunk/frame/failure/attempt/usage/structural-limit mutations. It intentionally contains no UI model, MCP model, or diagnosis.

Representative diagnostic matrix:

| Purpose | Fixture or deterministic generator | Bound/workflow evidence |
| --- | --- | --- |
| Failed execution | `runtime-terminal-failure` | `WF-FE-*`; runtime-produced terminal failure, direct frame/attempt relationships, and missing response evidence |
| Aborted execution | `runtime-terminal-abort` | Runtime-produced abort, stable terminal failure linkage, complete frames, and missing response evidence |
| Expensive execution | `nested-frame-usage`, `unattributed-usage`, `incomplete-frame-duration` | `WF-UE-*`; direct/descendant/inclusive usage without double counting |
| Unfamiliar skill path | `repeated-skill-invocations`, `nested-frame-usage` | `WF-SP-*`; invocation identity and exact recorded skill names |
| Live to completed inspection | `single-attempt-success` | `WF-SE-*`; configured limits and terminal facts |
| Tool lifecycle | `planned-tool-success`, `unplanned-tool-failure` | One canonical pre-invocation start, planned/unplanned linkage, explicit completion/failure, frame identity, and terminal tool usage |
| Deep/page continuation | Go's deterministic 20,000-frame calculation and browser pages over 100 rows | finite continuations; no semantic hierarchy cap |
| Large evidence range | `makeLargeChunkedPayloadArtifact` in `web/e2e/artifact-storage.spec.ts` | multi-megabyte content read in deliberate 64-KiB ranges |
| Structural limits | named invalid line/depth/usage/chunk/frame cases | exact bounded parser rejection |
| Configured-limit strictness | `configured-limits-*` invalid cases | missing, unknown, duplicate, fractional, negative, and overflow rejection |

Failure fixtures carry one or more bounded diagnostic objects in the
`ERROR_RECORDED` data. Terminality is derived only from a matching terminal
completion record; an error record is recovered/nonterminal when no completion
links its `failureId`. Diagnostic text is opaque application content and is
loaded separately from failure summaries.

The Java test generates valid cases through `DefaultExecutionTraceHandle`; invalid cases are minimal named mutations. Normal tests generate into a temporary directory and byte-compare the complete inventory:

```text
mvn -pl loomspan-spring-boot-starter -Dtest=ConsoleTraceFixtureCorpusTest test
```

Regenerate intentionally with:

```text
mvn -pl loomspan-spring-boot-starter -Dtest=ConsoleTraceFixtureCorpusTest -Dloomspan.console.fixtures.regenerate=true test
```

Run regeneration twice and require the second run to produce no diff. PR 06 will stream this same corpus as artifacts, and PR 13 will consume these expected results from Go; neither should copy it elsewhere.

`application-rest/` contains deterministic REST and problem bodies produced by
Java. `application-sse/` contains complete handshake, activity, failure, and
replay frames produced by the application stream framer.

The SSE activity endpoint (`/_loomspan/observability/v1/activity`) streams
`text/event-stream` frames with two event types:

- **handshake**: emitted once on connection open, contains `instanceId`,
  `observedAt`, and `afterCursor` (the replay starting point).
- **activity**: emitted for each activity event, contains `id` (the cursor),
  `instanceId`, `cursor`, `sessionId`, `traceId`, `canonicalSequence`,
  `timestamp`, `kind`, `executionStatus`, `summary`, and `details`.

The Console's Go client (`applicationclient.ActivityStream`) parses these
frames with strict protocol and size limits. The `live.Service` maintains a
2,048-entry/8-MiB ring buffer of recent activities and relays them to the browser via
`/api/console/v1/activity/stream` (SSE) and `/api/console/v1/activity/recent`
(POST JSON).

`application-artifact/download-response.json` records the exact artifact route,
status, and response headers. Its `bodyFixture` points to the existing
`traces/single-attempt-success.ndjson`; transport fixtures never duplicate an
NDJSON body. The JSON is test metadata, not a runtime manifest or a separate
artifact version.

Future Go PRs must first require the exact `consoleCompatibilityVersion` from
`application-rest/instance-status.json`. PR 09 must reject a mismatch before
making any snapshot, SSE, catalog, or artifact request. PRs 11-13 consume these
transport fixtures and the existing semantic corpus without filesystem paths or
a second trace format.
