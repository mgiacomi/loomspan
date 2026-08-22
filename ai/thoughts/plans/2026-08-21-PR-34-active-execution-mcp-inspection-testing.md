# PR 34 Active-Execution MCP Inspection Testing Plan

## Change Summary

- Complete the exact-release Java-to-Go active-usage protocol by carrying
  `providerAttempts` and `maxProviderAttempts` and rejecting omitted required
  usage/limit members instead of decoding them as zero.
- Replace the conflated activity `beginningUnavailable` Boolean with exact,
  mechanically observed coverage cursor facts: global eviction, selected-session
  start, selected-session eviction, and selected-session retained range.
- Make the complete active-execution/activity orientation fields discoverable
  in MCP schemas and deterministic key/value text while keeping arbitrary
  activity `details` out of text.
- Document and protect the existing dual use of an activity continuation:
  `hasMore` reports retained matching backlog now, while the returned opaque
  token is also a future checkpoint after `hasMore: false`.
- Add a bounded multi-execution route to the canonical unversioned Agent Skill and paired
  tools-only/skill-assisted PR 34 evaluations.
- Preserve a facts-only MCP boundary. MCP must not add completeness, progress,
  health, stuck, diagnosis, or recommendation states/messages. Existing
  producer-owned `status`, `phase`, `summary`, timestamps, paths, usage, and
  activity facts remain unchanged and untrusted.

## Impacted Areas

- **Java active usage producer and REST protocol**
  - `loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/observability/web/dto/ObservabilityDtos.java`
  - `loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/observability/web/ObservabilityDtoMapper.java`
  - provider advisor, live projector, observation handle/registry, REST
    controller, and their focused tests.
- **Exact Java-owned fixture corpus and Go wire consumer**
  - `loomspan-console-fixtures/application-rest/active-execution-detail.json`
  - `loomspan-console-fixtures/application-rest/active-executions-page.json`
  - `loomspan-console/internal/observability/dto.go`
  - `loomspan-console/internal/observability/service.go`
- **Shared bounded live activity state**
  - `loomspan-console/internal/live/dto.go`
  - `loomspan-console/internal/live/service.go`
  - browser recent-activity consumers and MCP activity adapter.
- **MCP discovery, results, text, pagination, and snapshots**
  - `loomspan-console/internal/mcpadapter/contracts.go`
  - `loomspan-console/internal/mcpadapter/output_schemas.go`
  - `loomspan-console/internal/mcpadapter/executions.go`
  - `loomspan-console/internal/mcpadapter/activity.go`
  - `loomspan-console/internal/mcpadapter/testdata/*.json`
  - protocol/conformance and exact `tools/list` tests.
- **Browser shared-contract consumers**
  - `loomspan-console/web/src/api/contracts.ts`
  - `loomspan-console/web/src/activity/ActivityProvider.tsx`
  - `loomspan-console/web/src/activity/reducer.ts`
  - `loomspan-console/web/src/activity/LiveActivity.tsx`
  - `loomspan-console/web/src/observability/Overview.tsx` where the shared live
    facts are presented.
- **Portable Agent Skill and package contract**
  - all six files under `loomspan-console/agent-skills/loomspan/`
  - `loomspan-console/internal/agentskills/`
  - release package, smoke, and no-version-declaration tests.
- **Agent evaluation and compatibility evidence**
  - new paired PR 34 cases and sanitized state-sequence fixture under
    `loomspan-console/agent-evals/`
  - `loomspan-console/internal/agenteval/`
  - `loomspan-console/docs/mcp-client-compatibility.md` and exact discovery
    snapshot.
- **Documentation-only authoring boundary**
  - No `ai/skill-authoring/` behavior or coverage classification changes. The
    authoring knowledge base is not a test target for this PR.

## Risk Assessment

### High-risk behaviors

- **Provider attempt corruption**: absent Java JSON members currently become
  plausible Go zeros. The fix could still silently accept a partial wire
  object, reorder fields inconsistently, or misrepresent zero-limit semantics.
- **Coverage overclaiming**: a convenience enum/Boolean or generated sentence
  could reintroduce a derived completeness claim. Missing cursor facts must
  remain absent, not become `UNKNOWN` or another state.
- **Incorrect cursor bookkeeping**: global eviction, selected-session eviction,
  reset, baseline establishment, suffix selection, and filtered continuation
  can be confused. Per-session tracking must stay bounded by the shared ring.
- **Continuation regression**: changes to coverage mapping could accidentally
  make a token unusable after `hasMore: false`, fail to advance an empty filtered
  call, or weaken scope/session binding.
- **Discovery/result divergence**: expanded compact schemas, complete typed
  result validation, structured content, deterministic text, and goldens could
  name different facts.
- **Discovery budget regression**: the exact `tools/list` response may exceed
  the selected 25,600-byte ceiling or documentation may record a different
  byte count.
- **Untrusted-content disclosure**: richer text could leak arbitrary activity
  `details`, payloads, credentials, internal target scope/instance/owner IDs, or
  application cursors.
- **Completion-race overclaiming**: disappearance from active state might be
  treated as proof of finalized trace availability or trigger an unrelated
  inventory scan.
- **Cross-layer drift**: Java, Go, TypeScript, fixtures, skill guidance,
  evaluation cases, and release packaging must move atomically.

### Edge cases

- Genuine observed zero provider attempts before the first physical send.
- Positive provider attempts while `modelCalls`, response precision, and units
  remain zero during an in-flight request.
- Configured `maxProviderAttempts == 0` meaning disabled/unlimited enforcement.
- Global eviction before a later selected-session `TRACE_STARTED`.
- Selected-session start retained, selected-session start evicted while later
  session activity remains, and an already-active baseline with no observed
  start.
- Interval reset after coverage cursors were recorded.
- No matching activity, no ring activity, cursor absent from the ring, initial
  suffix query, retained matching backlog, and future matching activity after a
  drained checkpoint.
- A 64-item active page and 64-item activity page with complete items.
- Activity summary containing adversarial text while `details` contains larger
  instructions or sensitive-looking content.
- Execution completion between list, detail, activity, and second observation;
  finalized trace both resolvable and `TRACE_UNAVAILABLE`.
- Target scope change, wrong-session token, malformed token, upstream stale
  cursor/reset, authentication generation change, and cancellation.

### Contract classification and compatibility expectations

| Surface | Classification | Testing treatment |
| --- | --- | --- |
| Top-level allowlisted Java API | Application API | Assert no signature/surface delta with `LoomspanPublicSurfaceArchitectureTest`. |
| Java SPI | Supported SPI | No affected SPI exists; architecture tests must show none was introduced. |
| `loomspan.session.quotas.*` defaults and zero behavior | Configuration and manifest contracts | Preserve binding/default/enforcement behavior and property metadata; test zero as disabled. |
| Canonical Agent Skill package | Configuration and manifest contracts (unreleased development package) | Protect the absence of skill-version metadata, six-file topology, validation, and byte-identical packaging. Do not add runtime skill/server version negotiation. |
| Active snapshots, activity, coverage cursors, continuation, text | Ephemeral diagnostic formats | Test current writer/reader/adapter coherence, fact accuracy, ordering, boundedness, failure visibility, and security. Do not preserve old fields or historical schemas. |
| Java active REST JSON consumed by Go | Internal or accidentally exposed implementation plus exact-release serialized protocol | Change atomically; regenerate Java-owned fixtures; reject missing members; retain exact release-string mismatch rejection. No legacy reader. |
| Java/Go/TypeScript implementation DTOs and browser state | Internal or accidentally exposed implementation | Remove obsolete `beginningUnavailable` paths and update all callers/tests in place. No dual field or adapter. |

This does not remove technical versions that serve different contracts:
Console build/version output, `consoleCompatibilityVersion`, MCP protocol
version, and evaluation-record `schemaVersion` remain in scope for their
existing tests. Only premature Agent Skill release identity and negotiation are
forbidden.
| Persisted traces/NDJSON | Persisted or serialized contracts | No change expected; standard trace fixture/processor tests are regression-only. No new historical compatibility fixture. |

### Intentionally removed obsolete paths

- `RecentResponse.BeginningUnavailable`, MCP `beginningUnavailable`, browser
  `beginningUnavailable`, and their tests/goldens are removed rather than
  preserved beside the cursor facts.
- Premature Agent Skill version declarations and evaluation fields are removed;
  there is no skill-version validator or runtime negotiation.
- No derived coverage enum, Boolean, result key, or narrative fallback is added
  as a replacement.

## Existing Test Coverage

### Java

- `ObservabilityDtoMapperTest#activeElapsedIsDerivedAtObservationTimeAndNeverCreatesHealthClaims`
  already protects factual active projection and rejects health/stuck claims,
  but does not assert provider-attempt fields.
- `ModelAttemptCallAdvisorIntegrationTest#retriesTransientProviderFailuresAsDistinctPhysicalAttempts`
  and `#exhaustedProviderRetriesRetainExactAttemptQuotaMetricAndTerminalFacts`
  protect upstream attempt accounting, but do not span active registry and REST
  serialization.
- `LiveActivityProjectorTest#derivesCountsAndNormalizedModelUsageFromCanonicalFacts`
  and `#providerRetryActivityContainsOnlyBoundedNeutralFacts` protect projector
  behavior, but not the Java-to-Go DTO omission.
- `ObservabilityRestIntegrationTest#listsAndGetsCurrentActiveExecutionAndFinalizedTrace`
  protects list/detail REST behavior but currently accepts JSON without the two
  provider fields.
- `ConsoleRestFixtureCorpusTest#generatedCorpusMatchesCommittedFixturesByteForByte`
  protects Java ownership of REST fixtures; the current fixtures encode the
  omission and must be regenerated atomically.

### Go live, observability, and MCP

- `TestRecentActivityQueryReportsEvictedBeginningAsFact` protects the obsolete
  global Boolean. It must be replaced by exact global/session cursor assertions.
- `TestRecentActivityQueryFiltersBySessionID`, reset/interval tests, ring count
  and byte eviction tests, and replay-to-live handoff tests provide the correct
  fixture/helper patterns for coverage tracking.
- `TestActivePageDecodesFromFixtureWithResumeCursor` and
  `TestActiveExecutionDetailDecodesFromFixture` confirm fixture decoding, but
  do not distinguish an omitted integer from an observed zero.
- `TestObservabilityServiceRejectsSemanticallyInvalidSuccessResponses` is the
  existing fail-closed validation pattern; usage/limit presence is not covered.
- `TestExecutionActivityGoldenPreservesCompleteEnvelopesAndConciseText`
  protects complete activity JSON, reset facts, and detail non-disclosure, but
  encodes `beginningUnavailable` and does not expose the remaining typed item
  identities in text.
- `TestListExecutionsGoldenPreservesBoundedActiveSummaries` and
  `TestGetExecutionGoldenPreservesProvisionalFactsWithoutDiagnosis` protect list
  and detail results, but list discovery/text omit ordinary orientation fields.
- `TestCompactSchemasRetainDecisionAndNavigationFields`, the exact discovery
  snapshot test, complete-output validation, browser/MCP parity, MCP security,
  maximum-page, continuation-binding, and cancellation tests provide strong
  adjacent coverage but not the PR 34 contract.

### Browser, skill packaging, and evaluations

- Browser provider/reducer tests protect interval resets, replay gaps,
  deduplication, and recent reloads. `LiveActivity` currently tests a synthesized
  “beginning unavailable” notice and must move to direct cursor-fact rendering.
- `TestCanonicalRuntimeDebuggingSkillIsValidAndExact`, release package tests,
  and strict smoke tests protect exact canonical skill bytes/topology without
  assigning a release version.
- `slow-execution.json` and its special evaluation server state cover one active
  execution, not multi-execution review, future checkpoint reuse, facts-only
  coverage, or completion races.
- The current 28-run release matrix has no paired PR 34 tools-only and
  skill-assisted cases.

## Bug Reproduction / Failing Test First

Two independent red tests are required before production changes. Add and run
them separately so each failure proves one problem rather than being masked by
the other.

### A. Provider-attempt REST omission

- **Name**: `activeRestUsageIncludesProviderAttemptFactsAndDisabledLimit`
- **Type**: Java unit/serialization test
- **Location**:
  `loomspan-spring-boot-starter/src/test/java/com/lokiscale/loomspan/internal/observability/web/ObservabilityDtoMapperTest.java`
- **Arrange**:
  - Construct an `ActiveExecutionSnapshot` whose `SessionUsageSnapshot` has
    `providerAttempts == 2` while response-only counters remain zero.
  - Configure `maxProviderAttempts == 0` and leave other quota values at known
    nonzero values.
  - Map with the real `ObservabilityDtoMapper` and encode with the real
    `ObservabilityJsonCodec`.
- **Act**: Decode the JSON into a generic object/map so the test compiles against
  the current record shape.
- **Assert**:
  - `usage.providerAttempts` exists and equals `2`.
  - `configuredLimits.maxProviderAttempts` exists and equals `0`.
  - No availability/health/diagnosis field was introduced.
- **Expected failure pre-fix**: both provider keys are absent from encoded JSON.
- **Mocks**: None; use immutable snapshot/quota values and the production mapper
  and codec.

### B. Facts-only activity coverage contract

- **Name**: `TestExecutionActivityGoldenExposesOnlyCursorCoverageFacts`
- **Type**: Go adapter unit/golden test
- **Location**:
  `loomspan-console/internal/mcpadapter/activity_test.go`
- **Fixture**: Update expected
  `loomspan-console/internal/mcpadapter/testdata/activity.json` first to contain
  a `coverage` object with exact cursor facts and no
  `beginningUnavailable`/derived-state key.
- **Arrange**: Reuse the existing activity item, reset, returned range, and
  continuation setup. The expected coverage facts are:
  `globalEvictedThroughCursor`, `sessionStartCursor`,
  `sessionEvictedThroughCursor`, and `sessionRetainedCursorRange` with concrete
  test cursors.
- **Act**: Marshal the current `activityResult` and render `activityText`.
- **Assert**:
  - Structured content matches the new exact golden.
  - Text contains the same cursor facts as key/value lines.
  - Result property names do not include `complete`, `incomplete`, `unknown`,
    `progress`, `health`, `stuck`, `diagnosis`, or `recommendation`.
  - Text contains no generated coverage/diagnostic sentence and no arbitrary
    `details` value.
- **Expected failure pre-fix**: current JSON/text contains
  `beginningUnavailable`, lacks the coverage cursors, and fails the golden.
- **Mocks**: None; use value objects and the current deterministic renderer.

### Failing-first commands

```powershell
.\mvnw.cmd -pl loomspan-spring-boot-starter "-Dtest=ObservabilityDtoMapperTest#activeRestUsageIncludesProviderAttemptFactsAndDisabledLimit" test
```

```powershell
Set-Location loomspan-console
go test ./internal/mcpadapter -run '^TestExecutionActivityGoldenExposesOnlyCursorCoverageFacts$' -count=1
```

Record the assertion failures. Do not make either test pass by weakening the
expected contract, adding nullable compatibility fields, or accepting both old
and new coverage shapes.

## Tests to Add/Update

### 1. Active DTO maps all six usage-limit dimensions

- **Name**: `activeRestUsageIncludesProviderAttemptFactsAndDisabledLimit`
- **Type**: unit/serialization
- **Location**: `ObservabilityDtoMapperTest.java`
- **What it proves**: The active REST mapper exposes the upstream provider
  attempt value and zero-as-disabled provider limit without inventing an
  availability or health state.
- **Fixtures/data**: Snapshot with two attempts and no response; quota fixture
  with provider limit zero.
- **Mocks**: None.
- **Contract classification**: Internal or accidentally exposed implementation;
  exact-release Java-Go serialized protocol.
- **Compatibility expectation**: Current-version coherence; old omission is
  removed with no fallback.

### 2. Provider send becomes visible in active state before response counters

- **Name**: `providerRequestIsVisibleInActiveRestBeforeResponseAccounting`
- **Type**: Java integration
- **Location**: new
  `loomspan-spring-boot-starter/src/test/java/com/lokiscale/loomspan/internal/observability/web/ActiveProviderAttemptObservabilityIntegrationTest.java`
- **What it proves**: With a real advisor, canonical trace handle, observation
  handle/registry, mapper, and REST serialization, a blocked first provider send
  yields `providerAttempts == 1`, `modelCalls == 0`, zero response units, and a
  request activity only after the active snapshot has been replaced. Releasing
  a transient failure/retry advances attempts to two without fabricating a
  model response.
- **Fixtures/data**: Deterministic blocking/failing `ChatModel`, fixed clock,
  real session usage service, observation registry, and bounded activity replay.
- **Mocks**: Mock only the external provider transport and clock/latches; use
  real Loomspan advisor/projection/REST components.
- **Contract classification**: Ephemeral diagnostic formats plus internal exact-
  release boundary.
- **Compatibility expectation**: Current-run diagnostic accuracy and ordering.

### 3. REST list/detail and Java-owned fixtures contain provider fields

- **Names**:
  - update `listsAndGetsCurrentActiveExecutionAndFinalizedTrace`
  - update `generatedCorpusMatchesCommittedFixturesByteForByte`
- **Type**: Java integration and fixture corpus
- **Locations**:
  - `ObservabilityRestIntegrationTest.java`
  - `ConsoleRestFixtureCorpusTest.java`
- **What it proves**: Both active list and detail JSON require identical usage/
  limit shapes, including provider fields; the committed fixtures are generated
  from Java and byte-exact.
- **Fixtures/data**: Nonzero provider attempt/limit in the corpus and a separate
  zero provider-limit case where practical.
- **Mocks**: Existing Spring/registry test wiring only.
- **Contract classification**: Internal exact-release serialized protocol.
- **Compatibility expectation**: Atomic fixture replacement; no old fixture
  compatibility.

### 4. Go fails closed on omitted usage and limit members

- **Name**: `TestActiveExecutionRequiresEveryUsageAndConfiguredLimitMember`
- **Type**: unit, table-driven
- **Locations**:
  - `loomspan-console/internal/observability/dto_test.go`
  - `loomspan-console/internal/observability/service_test.go`
- **What it proves**: Removing each required usage/limit field one at a time
  produces a semantic invalid-response failure; present zeros remain accepted
  and distinct from omission.
- **Fixtures/data**: Start from the regenerated active detail/page fixture;
  mutate decoded JSON maps in memory rather than committing malformed copies.
- **Mocks**: Existing `httptest.Server` service pattern.
- **Contract classification**: Internal exact-release serialized protocol.
- **Compatibility expectation**: Fail closed on partial/mixed Java builds;
  preserve exact release-string mismatch rejection.

### 5. Go preserves observed and disabled provider zeros

- **Name**: `TestActiveExecutionPreservesObservedProviderZeroAndDisabledLimit`
- **Type**: unit
- **Location**: `loomspan-console/internal/observability/dto_test.go`
- **What it proves**: Explicit JSON zeros decode as observed counter zero and
  disabled limit zero; no nullable availability state is introduced.
- **Fixtures/data**: In-memory copy of the active fixture with both provider
  values set to zero.
- **Mocks**: None.
- **Contract classification**: Configuration and manifest contracts plus
  internal boundary.
- **Compatibility expectation**: Preserve `loomspan.session.quotas.*` zero
  semantics.

### 6. Live service records exact global and session cursor facts

- **Name**: `TestRecentActivityReportsExactGlobalAndSessionCoverageCursors`
- **Type**: unit, table-driven subtests
- **Location**: `loomspan-console/internal/live/coordinator_test.go`
- **What it proves**:
  - unrelated global eviction records only the global evicted-through cursor;
  - a later selected-session `TRACE_STARTED` records its exact start cursor and
    retained range;
  - selected-session eviction records its exact evicted-through cursor while
    later selected-session items remain;
  - an already-active baseline with no admitted start omits `sessionStartCursor`;
  - reset starts a new interval and removes prior interval coverage cursors;
  - no completeness/progress/health enum or Boolean exists in encoded response.
- **Fixtures/data**: Existing `makeActivity`, direct bounded-ring admission, two
  session IDs, exact numeric cursors, and reset helpers.
- **Mocks**: Fake clock only.
- **Contract classification**: Ephemeral diagnostic formats.
- **Compatibility expectation**: Current-run fact accuracy; obsolete
  `TestRecentActivityQueryReportsEvictedBeginningAsFact` is replaced, not kept.

### 7. Coverage bookkeeping remains ring-bounded

- **Name**: `TestSessionCoverageCursorBookkeepingIsBoundedByRetainedRing`
- **Type**: unit
- **Location**: `loomspan-console/internal/live/coordinator_test.go`
- **What it proves**: After count/byte eviction across many session IDs, no
  per-session coverage entry survives when the ring retains no item for that
  session; reset/shutdown clears all bookkeeping.
- **Fixtures/data**: More than `ringMaxCount` sessions and oversized details for
  byte eviction, using existing ring-bound helpers.
- **Mocks**: None beyond fake clock.
- **Contract classification**: Internal implementation supporting ephemeral
  diagnostics.
- **Compatibility expectation**: No durable/process-lifetime activity catalog.

### 8. Activity checkpoint works after backlog is drained

- **Name**: `TestExecutionActivityContinuationRemainsFutureCheckpointAfterHasMoreFalse`
- **Type**: Go adapter integration
- **Location**: `loomspan-console/internal/mcpadapter/activity_test.go`
- **What it proves**: First call returns current matching items,
  `hasMore == false`, and a token; after admitting later matching and unrelated
  items, a second call with the same session/token returns only later matching
  items and a new checkpoint. An empty filtered call can advance to the global
  interval boundary without exposing the application cursor.
- **Fixtures/data**: Real `live.Service`, two sessions, exact cursors, and real
  MCP continuation encoder/decoder.
- **Mocks**: In-memory target/live service; no network needed unless using the
  existing handler harness.
- **Contract classification**: Ephemeral diagnostic formats.
- **Compatibility expectation**: Protect one-token dual semantics; do not add a
  second token or alias.

### 9. Activity continuation remains target/scope/session bound

- **Names**: update
  - `TestContinuationRejectsMalformedAndMismatchedInput`
  - `TestContinuationReturnsTargetChangedForPriorScope`
  - reset/old-cursor activity tests
- **Type**: unit/integration
- **Locations**:
  - `loomspan-console/internal/mcpadapter/continuation_test.go`
  - `loomspan-console/internal/mcpadapter/activity_test.go`
- **What it proves**: Facts-only coverage changes do not weaken token opacity,
  size limit, operation/session binding, `TARGET_CHANGED`, or current-interval
  reset recovery.
- **Fixtures/data**: Existing continuation fixtures plus the PR 34 coverage
  result.
- **Mocks**: Existing target scope fake.
- **Contract classification**: Ephemeral diagnostic formats.
- **Compatibility expectation**: Preserve security/authority semantics; tokens
  remain current-process only.

### 10. MCP activity result and text expose facts only

- **Name**: `TestExecutionActivityGoldenExposesOnlyCursorCoverageFacts`
- **Type**: unit/golden
- **Locations**:
  - `loomspan-console/internal/mcpadapter/activity_test.go`
  - `loomspan-console/internal/mcpadapter/testdata/activity.json`
- **What it proves**: Structured result and text expose the same typed activity
  identity, continuity, returned range, and exact coverage cursors; text omits
  arbitrary `details`; neither layer adds derived state or narrative diagnosis.
- **Fixtures/data**: Existing adversarial summary/details golden with concrete
  cursor facts.
- **Mocks**: None.
- **Contract classification**: Ephemeral diagnostic formats; supported pre-v1
  MCP contract.
- **Compatibility expectation**: Approved removal of `beginningUnavailable`;
  no simultaneous old/new result.

### 11. Execution list discovery and text contain complete orientation facts

- **Names**:
  - update `TestListExecutionsGoldenPreservesBoundedActiveSummaries` to
    `TestListExecutionsGoldenPreservesCompleteOrientationFactsWithoutDiagnosis`
  - update `TestGetExecutionGoldenPreservesProvisionalFactsWithoutDiagnosis`
  - update `TestListExecutionsMaximumPageHas64WholeItems`
- **Type**: unit/golden and bounded-page integration
- **Locations**:
  - `loomspan-console/internal/mcpadapter/executions_test.go`
  - execution list/detail goldens under
    `loomspan-console/internal/mcpadapter/testdata/`
- **What it proves**: List and detail share the same execution fact shape;
  structured/text clients see identity, sequence, times/elapsed, entry skill,
  status/phase, summary, path/depth/truncation, usage, and limits. No adapter-
  generated progress/health/stuck field or sentence appears.
- **Fixtures/data**: Regenerated Java active fixtures and a full 64-item page.
- **Mocks**: Existing observability fake/fixture server.
- **Contract classification**: Ephemeral diagnostic formats; supported pre-v1
  MCP contract.
- **Compatibility expectation**: Current-version coherence, no fleet tool.

### 12. Compact schemas name every selected fact and reject derived keys

- **Names**:
  - update `TestCompactSchemasRetainDecisionAndNavigationFields`
  - add `TestActiveCompactSchemasExposeFactsAndNoDerivedDiagnosticStates`
  - update representative complete-output validation cases
- **Type**: schema unit
- **Location**: `loomspan-console/internal/mcpadapter/output_schemas_test.go`
- **What it proves**: Execution list/detail and activity discovery explicitly
  name the selected nested facts; the coverage schema permits only the four
  cursor/range properties; no derived-state property is advertised; every
  complete typed result validates against compact and full schemas. Static tool
  descriptions explain pagination/checkpoint semantics without asserting a
  run-specific diagnosis.
- **Fixtures/data**: Representative success/error envelopes plus a table of
  forbidden derived property names.
- **Mocks**: None.
- **Contract classification**: Ephemeral diagnostic formats; supported pre-v1
  MCP discovery contract.
- **Compatibility expectation**: Approved in-place schema replacement.

### 13. Exact discovery snapshot stays under 25 KiB

- **Name**: update
  `TestCompatible2025ProtocolInitializesListsAndCallsRealRuntimeTool`
- **Type**: black-box protocol/snapshot
- **Locations**:
  - `loomspan-console/internal/mcpadapter/server_test.go`
  - `loomspan-console/internal/mcpadapter/testdata/tools-list-response.json`
  - `loomspan-console/docs/mcp-client-compatibility.md`
- **What it proves**: Exact HTTP JSON-RPC `tools/list` bytes match the committed
  snapshot, are at most 25,600 bytes, contain all selected fact names, contain
  no derived-state properties, still advertise exactly the existing twelve
  read-only/idempotent tools, and match the byte count/headroom in docs.
- **Fixtures/data**: Intentionally regenerated exact snapshot.
- **Mocks**: Real MCP server with existing test options.
- **Contract classification**: Supported pre-v1 MCP contract over ephemeral
  diagnostics.
- **Compatibility expectation**: New measured ceiling replaces 23,552 bytes;
  no old snapshot assertion remains.

### 14. Browser and MCP preserve the same raw coverage facts

- **Name**: update
  `TestBrowserAndMCPPreserveSameRecentContinuityAndGapFacts` to
  `TestBrowserAndMCPPreserveSameRecentContinuityAndCoverageCursorFacts`
- **Type**: Go parity integration
- **Location**: `loomspan-console/internal/mcpadapter/parity_test.go`
- **What it proves**: Browser and MCP consumers receive identical observation,
  interval/reset, returned range, global eviction, session start/eviction, and
  retained-range cursors; only MCP hides internal scope/instance/application
  cursors and wraps its checkpoint opaquely.
- **Fixtures/data**: Shared real `live.Service` with two sessions and eviction.
- **Mocks**: Existing browser/MCP adapters and target fake.
- **Contract classification**: Ephemeral diagnostic formats.
- **Compatibility expectation**: Current-run cross-consumer coherence.

### 15. MCP text and structured output remain safe

- **Names**:
  - extend `TestMCPSecurityPolicyAndFailureOrder`
  - extend activity/list golden tests
  - retain maximum 64-item tests
- **Type**: security and bounded-result regression
- **Locations**:
  - `loomspan-console/internal/mcpadapter/security_test.go`
  - `loomspan-console/internal/mcpadapter/activity_test.go`
  - `loomspan-console/internal/mcpadapter/executions_test.go`
- **What it proves**: No arbitrary details, credentials, target scope ID,
  instance ID, owner, application cursor, payload, or authority-bearing handle
  leaks through new schema/text fields; full items are never truncated.
- **Fixtures/data**: Adversarial summaries/details, oversized details, 64 active
  executions, and 64 activity items.
- **Mocks**: Existing HTTP/MCP harness.
- **Contract classification**: Ephemeral diagnostic formats and internal
  security boundary.
- **Compatibility expectation**: Preserve read-only/idempotent/target-safe
  behavior.

### 16. Completion race preserves facts and bounded trace-ID handoff

- **Name**: `TestActiveInspectionCompletionRacePreservesTraceIdentityAndAvailabilityOutcome`
- **Type**: Go black-box MCP integration
- **Location**: new
  `loomspan-console/internal/mcpadapter/active_inspection_test.go`
- **What it proves**: A list result returns `sessionId`/`traceId`; later detail
  may be not found; retained terminal activity remains queryable by session;
  `LOOMSPAN_get_trace` by the returned trace ID either succeeds or returns exact
  `TRACE_UNAVAILABLE`. None of the tools claims completion solely from active
  absence, and no tool automatically scans trace inventory.
- **Fixtures/data**: Mutable fake active registry, real live ring, traceresolution
  fixtures for available and unavailable evidence, and operation recorder.
- **Mocks**: Mock application target/evidence availability only; use real MCP
  handlers and trace resolver.
- **Contract classification**: Ephemeral diagnostic formats.
- **Compatibility expectation**: Current-version lifecycle coherence; no new
  workflow-specific tool or hidden state.

### 17. Browser stores and renders cursor facts without coverage state

- **Names**:
  - `ActivityProvider` “dispatches exact recent coverage cursors”
  - reducer “stores exact coverage cursors and clears them on reset”
  - `LiveActivity` “renders coverage cursor facts without a completeness state”
  - update Overview fixtures
- **Type**: TypeScript unit/component
- **Locations**:
  - `loomspan-console/web/src/api/client.test.ts`
  - `loomspan-console/web/src/activity/ActivityProvider.test.tsx`
  - `loomspan-console/web/src/activity/reducer.test.ts`
  - `loomspan-console/web/src/activity/LiveActivity.test.tsx`
  - `loomspan-console/web/src/observability/Overview.test.tsx`
- **What it proves**: The exact coverage object survives API/provider/reducer
  boundaries, resets remove old interval facts, and UI output contains only
  static labels plus exact cursors/reset facts—not complete/incomplete/unknown,
  progress, health, or stuck conclusions.
- **Fixtures/data**: Typed recent response fixtures for all optional-field
  combinations and interval reset.
- **Mocks**: Existing mocked client/stream callbacks and Testing Library.
- **Contract classification**: Internal browser implementation consuming
  ephemeral diagnostics.
- **Compatibility expectation**: Remove old Boolean/notice; no dual rendering.

### 18. Canonical unversioned Agent Skill teaches fact interpretation, not MCP inference

- **Names**:
  - update `TestCanonicalRuntimeDebuggingSkillIsValidAndExact`
  - update `TestRuntimeDebuggingSkillDoesNotTeachRemovedMCPWorkflow`
  - add assertions for `WF-ACTIVE-EXECUTION-REVIEW`
- **Type**: package/content validation
- **Location**: `loomspan-console/internal/agentskills/validate_test.go`
- **What it proves**: The exact six-file unversioned package validates; the new workflow is
  bounded; checkpoint and completion-race guidance is present; guidance tells
  the client to interpret exact facts and forbids treating missing coverage
  cursors as MCP-provided states.
- **Fixtures/data**: Canonical skill directory and unsafe/nonportable mutation
  table.
- **Mocks**: None.
- **Contract classification**: Configuration and manifest contracts (unreleased
  development package).
- **Compatibility expectation**: No skill-version metadata is accepted or
  negotiated before Loomspan's first release.

### 19. Release archive contains exact canonical unversioned skill bytes

- **Names**: update
  - `TestReleaseAndAuthoringDocumentationReferenceCanonicalSkillContract`
  - `TestReleasePackagesAreDeterministicAndContainRuntimeDebuggingSkill`
  - `TestStrictSmokeRequiresExactRuntimeDebuggingSkill`
- **Type**: packaging/smoke integration
- **Locations**: `loomspan-console/internal/buildtool/*_test.go`
- **What it proves**: Root/release documentation, validator behavior, archive
  contents, and canonical source bytes all agree that the skill is unversioned
  and has exact six-file topology.
- **Fixtures/data**: Temporary release archive built by existing helpers.
- **Mocks**: Build runner/toolchain fakes already used by buildtool tests.
- **Contract classification**: Configuration and manifest contracts.
- **Compatibility expectation**: No version declaration or negotiation; exact
  current-source packaging only.

### 20. PR 34 evaluation fixture exercises the complete fact sequence

- **Name**: `TestPR34EvaluationFixtureExercisesActiveInspectionFactSequence`
- **Type**: evaluation server integration
- **Locations**:
  - `loomspan-console/internal/agenteval/server_test.go`
  - `loomspan-console/agent-evals/fixtures/pr34-active-execution-review.json`
- **What it proves**: The deterministic server exposes multiple active
  executions, a second observation with changed sequence/activity facts,
  backlog then future checkpoint, unrelated global eviction followed by an
  observed selected-session start, selected-session eviction, baseline without
  a start fact, valid in-flight usage, and both completion-race trace outcomes.
- **Fixtures/data**: One sanitized, time-ordered PR 34 state-sequence fixture;
  no historical live transcript, credentials, headers, absolute paths, or full
  model/tool payloads.
- **Mocks**: Production MCP adapter over the isolated evaluation HTTP target.
- **Contract classification**: Ephemeral diagnostic/evaluation evidence.
- **Compatibility expectation**: Current-checkout reproducibility only.

### 21. Paired tools-only and skill-assisted cases enforce facts-only answers

- **Names**:
  - `pr34-tools-only-active-execution-review`
  - `pr34-skill-assisted-active-execution-review`
  - update `TestEvaluationCasesAreVersionedUniqueAndWorkflowLinked`
  - update `TestEvaluationCasesResolveAuthoritativeFixtureFacts`
- **Type**: case validation/scoring
- **Locations**:
  - new case JSON under `loomspan-console/agent-evals/cases/`
  - `loomspan-console/internal/agenteval/fixtures.go`
  - `loomspan-console/internal/agenteval/fixtures_test.go`
  - `loomspan-console/internal/agenteval/score_test.go`
- **What it proves**: Both cases share identical facts; tools-only can complete
  without undocumented-key probing; skill-assisted follows the bounded route;
  forbidden claims include MCP-provided completeness/progress/health/stuck
  state, invented finalization, and missing-as-zero; forbidden operations cover
  mutation, unrelated inventory scans, and non-MCP tools.
- **Fixtures/data**: The PR 34 state sequence, workflow
  `WF-ACTIVE-EXECUTION-REVIEW`, and `PR34-*` requirement IDs.
- **Mocks**: None for loader/scorer; evaluation server for protocol behavior.
- **Contract classification**: Configuration/manifest debugging package plus
  ephemeral evaluation evidence.
- **Compatibility expectation**: Add current PR 34 cases; do not preserve the
  obsolete single-Boolean oracle in `slow-execution.json`.

### 22. Release evaluation matrix requires actual supported headless runs

- **Name**: update `TestEvaluationSummaryRequiresSelectedRunsAndNeverDropsFailures`
- **Type**: scorer/release-evidence unit plus manual client execution
- **Location**: `loomspan-console/internal/agenteval/score_test.go`
- **What it proves**: Matrix is exactly 38 runs, adding three Codex CLI and two
  Claude Code runs for each paired PR 34 case; completed failures cannot be
  dropped; client/model builds and sanitized events are required.
- **Fixtures/data**: Synthetic summary records for automated validation; actual
  records only after supported-client runs.
- **Mocks**: Synthetic records in unit test; no mocks for manual headless runs.
- **Contract classification**: Compatibility/evaluation evidence for supported
  pre-v1 MCP and skill clients.
- **Compatibility expectation**: GUI/Desktop remains explicitly `Not run`
  unless captured by the repository harness; historical ticket observation is
  not promoted to a passing result.

### 23. Public API, configuration, and exact-release boundaries remain intact

- **Names**:
  - run `LoomspanPublicSurfaceArchitectureTest`
  - retain quota binding/default/enforcement tests
  - retain application-client exact compatibility rejection tests
- **Type**: architecture, configuration, and integration regression
- **Locations**:
  - `loomspan-spring-boot-starter/src/test/java/com/lokiscale/loomspan/architecture/LoomspanPublicSurfaceArchitectureTest.java`
  - quota property/service tests under `loomspan-spring-boot-starter/src/test/`
  - `loomspan-console/internal/applicationclient/client_test.go`
- **What it proves**: No supported Java API/SPI or Spring extension point is
  added; quota defaults/zero semantics are unchanged; mismatched released
  Java/Go builds are still rejected before active JSON consumption.
- **Fixtures/data**: Existing allowlists, property metadata, and compatibility
  version fixtures.
- **Mocks**: Existing Spring/application HTTP test harnesses.
- **Contract classification**: Application API, Supported SPI, Configuration
  and manifest contracts, and internal exact-release boundary.
- **Compatibility expectation**: Protected paths remain unchanged.

## How to Run

Run from `C:\opendev\code\loomspan` unless a command changes directory.

### 1. Demonstrate the two red tests before implementation

```powershell
.\mvnw.cmd -pl loomspan-spring-boot-starter "-Dtest=ObservabilityDtoMapperTest#activeRestUsageIncludesProviderAttemptFactsAndDisabledLimit" test
```

```powershell
Set-Location loomspan-console
go test ./internal/mcpadapter -run '^TestExecutionActivityGoldenExposesOnlyCursorCoverageFacts$' -count=1
Set-Location ..
```

### 2. Focused Java producer/protocol tests

```powershell
.\mvnw.cmd -pl loomspan-spring-boot-starter "-Dtest=ModelAttemptCallAdvisorIntegrationTest,LiveActivityProjectorTest,ActiveProviderAttemptObservabilityIntegrationTest,ObservabilityDtoMapperTest,ObservabilityRestIntegrationTest,ConsoleRestFixtureCorpusTest" test
```

Regenerate Java-owned REST fixtures only after the intentional DTO change:

```powershell
.\mvnw.cmd -pl loomspan-spring-boot-starter "-Dtest=ConsoleRestFixtureCorpusTest" "-Dloomspan.console.fixtures.regenerate=true" test
```

Then rerun the corpus test without regeneration and require byte equality.

### 3. Focused Go live/wire/MCP/evaluation tests

```powershell
Set-Location loomspan-console
go test ./internal/live ./internal/observability ./internal/mcpadapter ./internal/console ./internal/traceresolution ./internal/agentskills ./internal/agenteval ./internal/buildtool
```

Run exact checkpoint and discovery tests uncached while iterating:

```powershell
go test ./internal/mcpadapter -run 'TestExecutionActivityContinuationRemainsFutureCheckpointAfterHasMoreFalse|TestCompatible2025ProtocolInitializesListsAndCallsRealRuntimeTool' -count=1
```

### 4. Browser tests

```powershell
Set-Location web
npm test
npm run typecheck
Set-Location ..
```

### 5. MCP conformance and standard Console verification

```powershell
go run ./internal/buildtool mcp-conformance
go test ./...
go run ./internal/buildtool verify
Set-Location ..
```

### 6. Java architecture and protected configuration regression

```powershell
.\mvnw.cmd -pl loomspan-spring-boot-starter "-Dtest=LoomspanPublicSurfaceArchitectureTest,SessionUsageServiceTest,ModelAttemptCallAdvisorIntegrationTest,ObservabilityDtoMapperTest,ObservabilityRestIntegrationTest,ConsoleRestFixtureCorpusTest" test
```

### 7. Manual/supported-client evaluation

From `loomspan-console/`, run the tools-only and skill-assisted cases in fresh
conversations using the protected harness described in
`loomspan-console/agent-evals/README.md`:

```text
go run ./internal/buildtool agent-eval serve --case CASE_ID --output TEMP_DIR
go run ./internal/buildtool agent-eval record --session TEMP_DIR --client-events CLIENT_EVENTS.json --answer ANSWER.txt --output RECORD.json
go run ./internal/buildtool agent-eval score --record RECORD.json
go run ./internal/buildtool agent-eval summarize --results agent-evals/results/DATE
```

Do not print or commit the temporary endpoint key. Record Codex Desktop as
`Not run` unless the same harness can capture its event stream without secrets
or unsupported claims.

### Environment and test-data requirements

- Java and Go use repository wrappers/toolchains; do not substitute a globally
  different Maven build.
- Browser dependencies use the pinned Node/npm versions in `web/package.json`.
- No external target, network service, credential, or historical live session
  is required for automated tests.
- The optional Go race suite requires MSYS2 GCC on this Windows host:

```powershell
Set-Location loomspan-console
$env:PATH = "C:\msys64\mingw64\bin;" + $env:PATH
$env:CGO_ENABLED = "1"
go test -race ./...
Set-Location ..
```

Run the race suite before final review when the new live bookkeeping is stable.

## Exit Criteria

- [x] Both failing-first tests exist and fail for the documented pre-fix reason
  before production changes.
- [x] Both failing-first tests pass after implementation without weakening the
  expected result or accepting both old/new contracts.
- [x] Java active list/detail JSON includes `providerAttempts` and
  `maxProviderAttempts`; Go rejects their omission and accepts explicit zeros.
- [x] Deterministic in-flight integration evidence proves provider attempts are
  visible before response-only counters accrue.
- [x] Java-generated REST fixtures, Go DTOs, MCP goldens, and browser fixtures
  agree on all six usage/limit dimensions.
- [x] Activity results expose only exact observed/admitted/retained/evicted/
  reset/interval cursor facts; no derived coverage enum/Boolean is present.
- [x] MCP schema property names and adapter-generated text contain no new
  completeness, progress, health, stuck, diagnosis, or recommendation state/
  message. Existing producer-owned status/phase/summary values remain unchanged
  and explicitly untrusted.
- [x] Missing optional cursor facts remain absent and are never rendered as
  `UNKNOWN`, unavailable-as-complete, or another synthesized conclusion.
- [x] Global eviction, selected-session start, selected-session eviction,
  baseline without start, reset, suffix, empty, and missing-cursor cases have
  exact automated assertions.
- [x] Per-session cursor bookkeeping is bounded by retained ring state and is
  cleared on reset/shutdown.
- [x] `hasMore` protects retained-backlog meaning, and a token returned with
  `hasMore: false` retrieves later matching activity as a future checkpoint.
- [x] Malformed, wrong-kind, wrong-session, prior-target, and reset/stale token
  behavior remains exact and target-safe.
- [x] List and detail discovery/structured/text outputs expose the complete
  selected orientation facts without undocumented-key probing or generated
  diagnosis.
- [x] Arbitrary activity details, payloads, credentials, scope/instance/owner
  IDs, application cursors, and authority handles do not leak through new text
  or schema paths.
- [x] A full 64-item execution page and 64-item activity page return only whole,
  validated items within existing result bounds.
- [x] Exact `tools/list` bytes match the committed snapshot, remain at or below
  25,600 bytes, advertise exactly twelve read-only/idempotent tools, and equal
  the measurement recorded in client compatibility docs.
- [x] Completion-race tests preserve list-returned identity, retained terminal
  facts, exact trace availability/`TRACE_UNAVAILABLE`, and no automatic
  unrelated inventory scan.
- [x] Browser and MCP preserve the same raw cursor facts; obsolete
  `beginningUnavailable` API/state/rendering/tests are absent.
- [x] The canonical unversioned Agent Skill validates, contains exactly six files, teaches
  the bounded facts-first workflow, and packages byte-for-byte into release
  archives. No skill-version validator, declaration, or evaluation field remains.
- [x] Paired tools-only and skill-assisted PR 34 evaluation cases use the same
  sanitized fixture and enforce facts-only, bounded, provisional conclusions.
- [x] The protected 38-run release matrix includes three Codex CLI and two
  Claude Code runs for each paired PR 34 case, retains completed failures, and
  records unavailable GUI rows honestly.
- [x] `go test ./...`, `go run ./internal/buildtool verify`, MCP conformance,
  browser tests/typecheck, focused Java tests, and fixture corpus checks pass.
- [x] `LoomspanPublicSurfaceArchitectureTest` confirms no supported Java API/SPI
  delta; quota configuration/default/zero behavior remains protected.
- [x] Exact released Java/Go compatibility mismatch rejection still passes;
  obsolete REST/MCP/browser/skill paths are removed rather than hidden behind
  aliases, fallbacks, or dual behavior.
- [x] No `ai/skill-authoring/` document or coverage-table change is required;
  canonical runtime-debugging skill evidence supports all new guidance.
- [ ] Manual supported-headless-client evaluations are complete and sanitized,
  or any client that cannot be run is explicitly recorded as `Not run`.
