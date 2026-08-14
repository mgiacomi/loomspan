# PR 17 Runtime, Skill, and Live-Inspection MCP Surface Testing Plan

## Change Summary

- Correct the shared recent-activity query so browser and MCP return
  `LIVE_MONITORING_UNAVAILABLE`, rather than retained activity, when live
  monitoring is unavailable.
- Add query-time `observedAt` to the atomic shared recent snapshot while keeping
  upstream `continuity.observedAt` semantically separate.
- Add five strict, typed, read-only MCP tools for skills, active executions, and
  recent activity.
- Add the common typed `{result|error}` output envelope needed to retain
  structured domain errors with MCP `isError: true`.
- Add a stateless, versioned, scope/operation/session-bound MCP continuation
  with required caller-selected `pageSize` from 1 through 64.
- Add one supplementary scope-bound skill resource returning unchanged YAML.
- Add three capability families only when their complete required tool sets and
  semantics are present.
- Preserve existing MCP authentication, target-scope lifecycle, application
  REST/SSE contracts, browser parity, cancellation, and stateless multi-client
  behavior.

## Impacted Areas

- `loomspan-console/internal/live/`
  - shared recent-query signature, atomic availability/clock snapshot, DTO, and
    continuity/gap behavior;
  - existing coordinator and service tests must migrate atomically from the
    tuple return to `RecentResponse` plus domain error.
- `loomspan-console/internal/browserapi/activity.go` and
  `activity_test.go`
  - browser recent activity must expose the corrected shared error and new
    query observation field.
- `loomspan-console/internal/mcpadapter/`
  - typed schemas and outputs, domain errors, text fallbacks, annotations,
    continuations, five tools, resource template/read, capability catalog,
    security-generation publication, and real SDK protocol coverage.
- `loomspan-console/internal/console/`
  - production composition, selected-target integration, cancellation/races,
    browser/MCP parity, and multiple authenticated clients over the same live
    service.
- `loomspan-console/internal/observability/`
  - no intended production semantic change; its validated DTOs and service
    tests are the source fixtures for MCP mapping/parity.
- `loomspan-console/docs/mcp-client-compatibility.md` and
  `loomspan-console/README.md`
  - exact implemented contract and dated manual-client evidence.
- Existing Java REST/SSE fixtures and public-surface tests
  - protected unchanged paths that must continue to pass; no fixture
    regeneration is planned.

## Risk Assessment

### High-risk behaviors

- A typed handler may accidentally return a Go error for a Loomspan domain
  failure, losing structured error content and collapsing it into SDK text.
- An output envelope may populate neither or both `result` and `error`, or mark
  a domain error without `isError: true`.
- A raw application/activity cursor, wrong-operation token, wrong-session token,
  or prior-scope token may be accepted as a valid continuation.
- Recent activity may combine continuity intervals, return retained data while
  live monitoring is unavailable, conflate query time with upstream continuity
  time, or imply durable/lossless history.
- Target or MCP credential rotation may permit a late successful result.
- Resource parsing may accept a noncanonical URI, decode a path separator into
  routing structure, use `sourcePath` as a filesystem path, or leak an internal
  error through JSON-RPC data.
- Capability advertisement may drift from actual tool registration or become
  conditional on target/evidence availability.
- Text fallback may omit continuation/gap facts, duplicate large raw activity
  details, treat diagnostic content as instructions, or add unsupported health,
  causality, completeness, or remediation claims.
- A 64-item page or maximum accepted skill YAML may be truncated or fail in a
  representative client without the compatibility matrix recording it.
- New test seams may accidentally create a second service abstraction,
  subscription, retained store, or compatibility overload.

### Edge cases

- `pageSize` missing, zero, negative, 65, non-integer, or accompanied by an
  unknown property.
- Empty list, empty recent interval, no matching session activity, and a
  continuation usable for a later poll even when `hasMore` is false.
- A 4,096-character application cursor inside the 8,192-character MCP token
  bound; exact-bound and one-byte-over-bound tokens.
- Malformed base64url, padding, invalid/trailing JSON, unknown fields, version,
  or operation; missing scope/cursor/session; cross-tool and cross-session use.
- Prior target scope, target change during an upstream call, changed application
  instance, upstream `STALE_CURSOR`, evicted activity beginning, and reset with
  no returned items.
- `EXECUTION_OBSERVATION_ENDED` with `CORE_FINALIZATION_FAILED`.
- Empty optional fields versus required empty arrays; RFC 3339 nanosecond UTC
  formatting; different query and continuity observation times.
- Unicode registered skill names, encoded path separators, query/fragment,
  extra segments, user info, wrong scheme/authority, noncanonical escaping, and
  URI scope mismatch.
- YAML containing Unicode, newlines, very large content, path-looking text,
  credentials-looking text, or adversarial instructions.
- Activity `details` containing adversarial instructions and maximum accepted
  encoded size.
- `NOT_FOUND`, authentication required, access blocked, target unavailable,
  incompatible target, live unavailable, invalid/stale cursor,
  `LIMIT_EXCEEDED`, `TARGET_CHANGED`, and sanitized `CONSOLE_ERROR`.
- Client cancellation, credential regeneration/disablement, target rotation,
  shutdown, two simultaneous clients, and one client canceling without
  affecting the other.

### Contract classification and compatibility scope

| Surface | Classification | Test obligation |
| --- | --- | --- |
| Java application API | Application API | No delta. Run `LoomspanPublicSurfaceArchitectureTest` and reject any new allowlisted or leaked internal signature. |
| Java extension surface | Supported SPI | No SPI exists and none may be introduced. Existing architecture assertion remains green. |
| Loomspan configuration and YAML syntax | Configuration and manifest contracts | No behavior change. Verify MCP preserves registered name/path/YAML but do not add manifest compatibility variants. |
| MCP tools/capabilities/schemas/envelopes/resource/continuation | Persisted or serialized contracts | Establish one new exact v1 contract through schema/result/resource goldens and real SDK calls. Preserve the existing runtime tool additively. |
| Active snapshots, recent activity, scope, observation, gaps, reset, continuation | Ephemeral diagnostic formats | Test current-run accuracy, ordering, availability, failure visibility, bounds, and security; do not test cross-restart continuation compatibility. |
| `Recent` signature, `NewServer` options, helper DTOs/codecs | Internal or accidentally exposed implementation | Update callers/tests atomically. Assert the obsolete tuple call and old constructor are removed, not retained behind overloads or shims. |

Protected paths are the supported Java API allowlist, absence of SPI,
configuration/YAML semantics, exact current Java-to-Go REST/SSE behavior,
existing MCP authentication/runtime-status behavior, and current-run diagnostic
truthfulness. The approved obsolete paths are the internal five-value `Recent`
API, the old internal `NewServer` constructor shape, and browser behavior that
returns retained activity while live monitoring is unavailable. Tests must not
require old and new behavior simultaneously.

The implementation plan classifies Java-to-Go boundary coordination as not
required. Existing Java-produced fixtures remain input evidence and must not be
regenerated or changed. Any discovered need to change that boundary stops the
work until the ticket, implementation plan, and this testing plan are amended.

Skill-authoring documentation impact is `No impact`; no test is needed solely
for `ai/skill-authoring/` prose. YAML-fidelity tests protect the existing
configuration/manifest contract consumed by this diagnostic adapter.

## Existing Test Coverage

### Reusable coverage and conventions

- `loomspan-console/internal/live/coordinator_test.go`
  - direct same-package construction of activity/continuity state;
  - duplicate/regression rejection, one-interval reset, session filtering,
    limit clamping, evicted beginning, ring bounds, shutdown, and subscriber
    independence;
  - `fakeClock` and `makeActivity` are the lowest-cost patterns for the first
    failure and atomic observation tests.
- `loomspan-console/internal/live/service_test.go`
  - real SSE `httptest.Server`, target ownership, reconnect/rebaseline,
    invalidation, instance mismatch, and session-filtered pagination.
- `loomspan-console/internal/browserapi/activity_test.go`
  - real browser router/session security over an actual `live.Service`;
  - current empty recent response, malformed input, missing services, SSE
    cancellation, and security headers.
- `loomspan-console/internal/observability/service_test.go`
  - Java-shaped skill and active-execution pages/details, pagination,
    page-size clamping, malformed upstream bodies, live-unavailable,
    `NOT_FOUND`, and cursor errors.
- `loomspan-console/internal/mcpadapter/runtime_test.go`
  - byte-exact structured-output golden plus deterministic text agreement.
- `loomspan-console/internal/mcpadapter/server_test.go`
  - real SDK and raw JSON-RPC initialization, discovery, inferred schema,
    structured content, unknown-argument rejection, and stateless HTTP.
- `loomspan-console/internal/mcpadapter/security_test.go`,
  `lifecycle_test.go`, and `tracker_test.go`
  - authority/origin/bearer ordering, body bounds, revoked generations,
    cancellation, freeze/drain, and shutdown.
- `loomspan-console/internal/browserapi/contracts_test.go`
  - exact inventory and byte-for-byte golden corpus pattern for serialized
    contracts.
- `loomspan-console/internal/console/activity_integration_test.go`,
  `observability_integration_test.go`, and `target_integration_test.go`
  - production composition, Java-produced fixtures, target rotation, no secret
    leakage, and assembled browser behavior.
- `loomspan-console/mcp-conformance/`
  - official protocol-generic initialization, listing, caching, and
    DNS-rebinding coverage behind production middleware.
- `loomspan-console-fixtures/application-rest/` and `application-sse/`
  - deterministic Java-produced skill, active execution, page continuation,
    problem, handshake, replay, completion, and finalization-failed inputs.

### Coverage gaps

- No test currently fails when retained recent activity is returned while live
  monitoring is unavailable.
- No top-level recent-query observation time exists.
- No PR 17 tool, schema, result/error envelope, annotation, text fallback,
  continuation, resource, or capability-family test exists.
- No browser/MCP parity test feeds the same skill/execution/activity/error facts
  through both adapters.
- No MCP target-rotation, multi-client, or maximum-page test exercises PR 17
  handlers.
- The client compatibility matrix contains no completed PR 17 run.

### Planning-time baseline

On 2026-08-13, before adding the planned failing test or production changes,
this focused baseline passed from `loomspan-console/`:

```text
go test ./internal/live ./internal/browserapi ./internal/mcpadapter
```

This green baseline does not cover the live-unavailable retained-window defect;
the first test below is expected to make `internal/live` fail until the shared
query behavior is corrected.

## Bug Reproduction / Failing Test First

- **Name**: `TestRecentActivityQueryDoesNotReturnRetainedStateWhenLiveUnavailable`
- **Type**: Unit
- **Location**: `loomspan-console/internal/live/coordinator_test.go`
- **Contract classification**: Ephemeral diagnostic formats
- **Compatibility expectation**: Current-run diagnostic coherence; the current
  retained-state fallback is an approved internal behavior removal.
- **Arrange**:
  1. Construct `live.Service` with the existing fake clock.
  2. Under the same-package lock, establish one interval, append one valid
     activity, and set `liveUnavailable = true` without clearing the window.
- **Act**: Call the current `Recent("", "session-1", 10)` operation.
- **Assert before the signature refactor**: No retained item may be returned.
- **Expected failure pre-fix**: The current method returns the retained activity,
  so the test fails on the item-count assertion.
- **Post-refactor final assertion**: Call
  `Recent(RecentRequest{SessionID: "session-1", Limit: 10})`; assert an empty
  response and a non-nil shared error whose code is exactly
  `LIVE_MONITORING_UNAVAILABLE`. The test must not be kept in a transitional
  form that accepts either behavior.
- **Fixtures/data**: Existing `fakeClock`, `makeActivity`, test instance/scope.
- **Mocks**: None; use the real in-memory service.

Run this test alone and capture its expected pre-fix failure before modifying
production code:

```text
go test ./internal/live -run TestRecentActivityQueryDoesNotReturnRetainedStateWhenLiveUnavailable -count=1
```

## Test Implementation Order

1. Add and run the minimal failing live-unavailable test.
2. Add shared recent-query response/observation tests; refactor `Recent` and
   migrate existing callers/tests until the live/browser suites pass.
3. Add envelope/error and continuation unit tests before their helpers.
4. Add the missing activity-kind allowlist/label regression, then add skill,
   execution, activity, and resource schema/golden tests before each
   corresponding handler.
5. Add capability and assembled real-SDK discovery/call tests before enabling
   advertisement in `LOOMSPAN_get_runtime`.
6. Add parity and lifecycle integration tests before final composition wiring
   is considered complete.
7. Run canonical/race/conformance suites, then execute and record the manual
   client matrix.

## Tests to Add/Update

### 1. Shared recent-query correctness

| Name | Type | Location | What it proves | Fixtures/data | Mocks | Contract classification | Compatibility expectation |
| --- | --- | --- | --- | --- | --- | --- | --- |
| `TestRecentActivityQueryDoesNotReturnRetainedStateWhenLiveUnavailable` | Unit, failing first | `internal/live/coordinator_test.go` | Atomic live-unavailable rejection and no retained fallback | Existing fake clock/activity | None | Ephemeral diagnostic formats | Approved removal/current-run coherence |
| `TestRecentActivityQueryCapturesObservedAtWithWindowSnapshot` | Unit | `internal/live/coordinator_test.go` | Top-level `observedAt` comes from the service clock under the query lock and differs from upstream continuity time | Fake clock advanced after interval observation | None | Ephemeral diagnostic formats | Current-run coherence |
| `TestRecentActivityQueryReturnsEmptyItemsNotNull` | Unit | `internal/live/coordinator_test.go` | Empty successful snapshot has `items: []` and stable gap/continuity fields | Empty current interval | None | Persisted or serialized contracts | New MCP/browser DTO contract |
| `TestRecentActivityQueryPreservesOrderingFilteringAndForwardCursor` | Unit | `internal/live/coordinator_test.go` | Existing cursor order, session filtering, `hasMore`, and next position survive the API refactor | Interleaved session activities | None | Ephemeral diagnostic formats | Protected current behavior |
| `TestRecentActivityQueryPreservesSingleIntervalGapAndResetFacts` | Unit | `internal/live/coordinator_test.go` | No cross-reset evidence and explicit `beginningUnavailable`/reset | Existing reset/eviction builders | None | Ephemeral diagnostic formats | Protected current behavior |
| `TestActivityRecentReturnsLiveMonitoringUnavailable` | Integration | `internal/console/mcp_runtime_integration_test.go` | Browser maps the shared domain error and does not serialize activity when the application's live endpoint becomes unavailable | Production composition with a test application whose activity endpoint returns the committed live-unavailable problem | Real target and live service; no alternate fake service | Persisted or serialized contracts | Approved browser correction |
| `TestActivityRecentReturnsQueryAndContinuityObservationTimes` | Integration | `internal/browserapi/activity_test.go` | Browser JSON preserves both timestamp meanings exactly | Deterministic live clock/interval | Real service | Persisted or serialized contracts | Additive current contract |

All existing `Recent` callers in `coordinator_test.go`, `service_test.go`, and
`browserapi/activity.go` must move to the new result/error signature. Remove all
five-value tuple assertions; do not retain a compatibility wrapper.

### 2. Common MCP envelope, errors, annotations, and fallbacks

| Name | Type | Location | What it proves | Fixtures/data | Mocks | Contract classification | Compatibility expectation |
| --- | --- | --- | --- | --- | --- | --- | --- |
| `TestToolEnvelopeContainsExactlyOneResultOrError` | Unit | `internal/mcpadapter/contracts_test.go` | Success and domain failure populate exactly one arm | Minimal result and `consolecore.Error` | None | Persisted or serialized contracts | New exact v1 contract |
| `TestDomainErrorResultIsStructuredMarkedAndSafe` | Unit | `internal/mcpadapter/contracts_test.go` | `isError: true`, structured code/message/scope/details, exact `CODE: message`, nil Go handler error | Every `consolecore.Details` field plus internal wrapped cause | None | Persisted or serialized contracts | Shared error preservation |
| `TestDomainErrorDetailsAreAlwaysObjects` | Unit/golden | `internal/mcpadapter/contracts_test.go` | Empty details encode as `{}` and no tool error can emit `null`, scalar, or array details | Empty and populated shared errors | None | Persisted or serialized contracts | Settled exact error shape |
| `TestDomainErrorResultNeverLeaksInternalCauseOrUnknownFields` | Unit | `internal/mcpadapter/contracts_test.go` | Stack/path/credential/cause text is absent from JSON and text | Sentinel secret/path/cause | None | Ephemeral diagnostic formats | Security/redaction coherence |
| `TestPR17ToolsUseReadOnlyClosedWorldAnnotations` | Unit | `internal/mcpadapter/contracts_test.go` | All five annotations are read-only, non-destructive, idempotent, and closed-world | Tool definitions | None | Persisted or serialized contracts | New exact v1 contract |
| `TestPR17TextFallbacksMatchStructuredFacts` | Unit | Per-tool tests | Required identity, observation, count/state, gap, and continuation facts agree with structured output; no unsupported diagnosis | Golden success results | None | Persisted or serialized contracts | New exact v1 contract |
| `TestPR17TextFallbacksHaveExactOrderEscapingAndFinalNewline` | Unit/golden | Per-tool tests | Common/operation/item lines are fixed, strings and timestamps are JSON-quoted, time is UTC RFC3339Nano, absent continuation is `-`, YAML follows terminal `yaml:`, and activity details are omitted | Delimiters, quotes, CR/LF, control characters, Unicode, multiline YAML/details | None | Persisted or serialized contracts | Settled deterministic fallback |
| `TestPR17TextFallbackTreatsDiagnosticContentAsData` | Unit | `internal/mcpadapter/contracts_test.go` | Adversarial YAML/activity text produces no extra operation and raw activity details are not copied into fallback | Instruction-like YAML/details | Call counters only around existing services | Ephemeral diagnostic formats | Security boundary coherence |

The SDK validation path must also be tested through a real client: a missing or
unknown input property is an unsuccessful SDK tool result, while a
`consolecore.Error` uses the structured Loomspan envelope. These two paths must
not share a golden.

### 3. Continuation codec and page-size contract

| Name | Type | Location | What it proves | Fixtures/data | Mocks | Contract classification | Compatibility expectation |
| --- | --- | --- | --- | --- | --- | --- | --- |
| `TestContinuationRoundTripForEveryOperationKind` | Unit | `internal/mcpadapter/continuation_test.go` | Version, kind, scope, cursor, and activity session survive unpadded-base64url round trip | Skills/executions/activity cases | None | Ephemeral diagnostic formats | New current-process contract |
| `TestContinuationRejectsMalformedAndMismatchedInput` | Unit table | `internal/mcpadapter/continuation_test.go` | Bad base64/padding/JSON/trailing value/unknown field/version/kind/missing field/operation/session fails as `INVALID_ARGUMENT` | Table literals | None | Ephemeral diagnostic formats | Strict new contract |
| `TestContinuationReturnsTargetChangedForPriorScope` | Unit | `internal/mcpadapter/continuation_test.go` | Structurally valid prior-scope token is not treated as malformed | Two scope IDs and current target context | Real target context | Ephemeral diagnostic formats | Scope truthfulness |
| `TestContinuationSupportsMaximumApplicationCursorWithinTokenBound` | Unit | `internal/mcpadapter/continuation_test.go` | A 4,096-character upstream cursor encodes/decodes below the 8,192 token cap | Repeated safe cursor data | None | Ephemeral diagnostic formats | Existing Java cursor compatibility |
| `TestContinuationRejectsEncodedValueOver8192Characters` | Unit | `internal/mcpadapter/continuation_test.go` | Bound is applied before unbounded decoding/allocation | 8,193-character input | None | Ephemeral diagnostic formats | Resource-bound coherence |
| `TestContinuationDoesNotBindPageSizeOrContainCredential` | Unit | `internal/mcpadapter/continuation_test.go` | Caller can lower later page size and serialized payload contains only allowlisted fields | Decode emitted JSON payload; sentinel MCP key | None | Ephemeral diagnostic formats | Settled simple design |
| `FuzzDecodeContinuationNeverPanicsOrEscapesAllowlist` | Fuzz/unit | `internal/mcpadapter/continuation_test.go` | Arbitrary bytes never panic and successful decodes meet all invariants | Valid seed corpus plus malformed seeds | None | Ephemeral diagnostic formats | Security/current-run coherence |
| `TestPR17PageSizeSchemaRequiresOneThrough64` | SDK integration | `internal/mcpadapter/server_test.go` | Missing, 0, negative, 65, fractional, string, and unknown property fail; 1 and 64 reach handler | Raw/SDK tool calls | Fake application only for valid cases | Persisted or serialized contracts | New exact v1 schema |

Do not test token readability across console restart or prior implementation
versions. Such behavior is explicitly unsupported.

### 4. Skill tools and skill resource

| Name | Type | Location | What it proves | Fixtures/data | Mocks | Contract classification | Compatibility expectation |
| --- | --- | --- | --- | --- | --- | --- | --- |
| `TestListSkillsGoldenStructuredResultAndText` | Unit/golden | `internal/mcpadapter/skills_test.go`, `testdata/skills-list.json` | Exact envelope, scope/instance/application observation, item fields, resource URI, `hasMore`, opaque continuation, and text | Java-shaped `skills-page.json` values | Pure mapper; no network | Persisted or serialized contracts | New exact v1 contract |
| `TestGetSkillGoldenPreservesUnchangedYAML` | Unit/golden | `internal/mcpadapter/skills_test.go`, `testdata/skill-detail.json` | Exact detail/query observation/resource URI and byte-for-byte YAML in structured/text output | Unicode/multiline/adversarial YAML | Pure mapper | Configuration and manifest contracts | Protected unchanged YAML |
| `TestSkillToolsUseExistingObservabilityServiceAndMapErrors` | Integration | `internal/mcpadapter/skills_test.go` | Real target capture/service call, page cursor wrapping, `NOT_FOUND`, authentication, stale/invalid cursor, target change, and sanitized failure | `httptest.Server` serving committed REST fixture shapes | Existing fake probe/client pattern only | Persisted or serialized contracts | Shared Java-to-Go behavior preserved |
| `TestSkillResourceTemplateDiscovery` | SDK integration | `internal/mcpadapter/resources_test.go` | Exactly one PR 17 resource template with canonical URI, name/title/description, and YAML MIME | Real SDK session | No application call | Persisted or serialized contracts | New exact v1 contract |
| `TestSkillResourceReadReturnsCanonicalYAMLAndMetadata` | SDK integration | `internal/mcpadapter/resources_test.go` | Canonical URI, unchanged YAML, MIME, and `_meta.loomspan` scope/instance/time/name/sourcePath | Unicode skill fixture | Real observability service over fake application | Persisted or serialized contracts | New exact v1 contract |
| `TestSkillResourceURIRejectsNoncanonicalOrUnsafeForms` | Unit table | `internal/mcpadapter/resources_test.go` | `EscapedPath` parsing rejects wrong scheme/authority, opaque form, port/user info, query/fragment, missing/extra/empty segments, malformed escape, invalid UTF-8, blank values, decoded slash/backslash, double encoding, and any segment whose `PathEscape(decoded)` differs | URI table including Unicode and mixed-case escapes | None/real target context for scope | Ephemeral diagnostic formats | Settled canonical parser |
| `TestSkillResourceErrorUsesExactJSONRPCMapping` | SDK/raw JSON-RPC integration | `internal/mcpadapter/resources_test.go` | URI syntax/`INVALID_ARGUMENT`/`NOT_FOUND` use `-32602`; other domain errors including `CONSOLE_ERROR` use `-32000`, never `-32603`; message is safe and data is exactly `{error: DTO}` with object details and no cause | Full shared error-code table plus malformed URI | Fake application failures | Persisted or serialized contracts | Settled resource error contract |
| `TestSkillSourcePathIsReturnedOnlyAsDescriptiveData` | Unit/integration | `internal/mcpadapter/skills_test.go`, `resources_test.go` | Path-looking source text is returned unchanged, never accepted as input, opened, or turned into URI routing | Windows/POSIX/URL-like sentinel paths | Application call counter; no filesystem mock needed | Configuration and manifest contracts | Protected descriptive-only contract |
| `TestGetSkillMaximumAcceptedYAMLRoundTripsWithoutTruncation` | Integration | `internal/mcpadapter/skills_test.go` | Near-service-bound YAML survives structured and text output or fails explicitly, never silently truncates | Generated deterministic near-bound YAML | `httptest.Server` | Persisted or serialized contracts | Response-framing acceptance |

The golden corpus uses committed JSON files under
`internal/mcpadapter/testdata/`; tests must verify exact inventory so obsolete
or accidental alternate schemas cannot coexist unnoticed.

### 5. Active execution tools

| Name | Type | Location | What it proves | Fixtures/data | Mocks | Contract classification | Compatibility expectation |
| --- | --- | --- | --- | --- | --- | --- | --- |
| `TestListExecutionsGoldenPreservesBoundedActiveSummaries` | Unit/golden | `internal/mcpadapter/executions_test.go`, `testdata/executions-list.json` | Every existing active field, scope/instance/application observation, opaque continuation, and no duplicate item scope | Committed active page shape | Pure mapper | Persisted or serialized contracts | New exact v1 contract over protected facts |
| `TestGetExecutionGoldenPreservesProvisionalFactsWithoutDiagnosis` | Unit/golden | `internal/mcpadapter/executions_test.go`, `testdata/execution-detail.json` | Detail query observation, identity, phase/status, bounded path/truncation, usage/limits, and neutral text | Active detail fixture including truncated path | Pure mapper | Ephemeral diagnostic formats | Workflow/current-run coherence |
| `TestExecutionToolsMapSharedDomainErrorsExactly` | Integration table | `internal/mcpadapter/executions_test.go` | `NOT_FOUND`, live unavailable, auth required, incompatible, target unavailable/change, cancellation, and console error remain distinct | Existing problem fixtures plus canceled context | Real observability service/fake application | Persisted or serialized contracts | Shared error preservation |
| `TestExecutionToolsNeverExposeCompletedHistoryOrTraceHierarchy` | Unit | `internal/mcpadapter/executions_test.go` | Output schema/text contains no completed list, inferred outcome, full hierarchy, self-duration, or complete attribution field | Schema and golden inspection | None | Ephemeral diagnostic formats | Workflow boundary coherence |
| `TestExecutionListContinuationWrapsButNeverEmitsApplicationCursor` | Integration | `internal/mcpadapter/executions_test.go` | Raw application cursor is accepted only inside codec and output uses MCP token | Continuation page fixture | Fake application records request cursor | Ephemeral diagnostic formats | Settled opaque-token contract |

### 6. Recent-activity MCP tool

| Name | Type | Location | What it proves | Fixtures/data | Mocks | Contract classification | Compatibility expectation |
| --- | --- | --- | --- | --- | --- | --- | --- |
| `TestExecutionActivityGoldenPreservesCompleteEnvelopesAndConciseText` | Unit/golden | `internal/mcpadapter/activity_test.go`, `testdata/activity.json` | Exact scope/instance/query time/items/range/continuity/gap/continuation; raw details structured but absent from text | Every supported relevant field and adversarial details | Pure mapper | Persisted or serialized contracts | New exact v1 contract |
| `TestModelAttemptFailedIsAcceptedLabeledAndPreserved` | Unit/integration | `internal/live/dto_test.go`, `internal/mcpadapter/activity_test.go` | Declared `MODEL_ATTEMPT_FAILED` passes validation, has exact label `Model attempt failed`, and reaches structured/text activity output with its unchanged wire value; unknown kinds still fail | Java-shaped model-attempt failure plus unknown sentinel | Real DTO/mapper | Persisted or serialized contracts | Go-only correction to existing wire value |
| `TestExecutionActivityContinuationSupportsForwardPolling` | Integration | `internal/mcpadapter/activity_test.go` | Token from last item or continuity last cursor resumes later matching activity; `hasMore` means retained now | Interleaved session activity admitted in stages | Real live service | Ephemeral diagnostic formats | Settled activity semantics |
| `TestExecutionActivityMissingCursorReturnsSuccessfulGap` | Integration | `internal/mcpadapter/activity_test.go` | Expired/unknown valid activity position returns `result`, `isError: false`, no items, and explicit beginning/reset facts | Evicted/reset window | Real live service | Ephemeral diagnostic formats | Protected replay-gap meaning |
| `TestExecutionActivityNeverCrossesResetBoundary` | Integration | `internal/mcpadapter/activity_test.go` | No pre/post-reset items coexist and continuation observes current interval | Two intervals | Real live service | Ephemeral diagnostic formats | Protected continuity behavior |
| `TestExecutionActivityReturnsLiveUnavailableAsStructuredError` | Integration | `internal/mcpadapter/activity_test.go` | Retained items are suppressed and safe structured error is marked | Retained window plus unavailable state | Real live service | Persisted or serialized contracts | Approved correction |
| `TestExecutionActivityPreservesCoreFinalizationFailedWithoutOutcome` | Unit/integration | `internal/mcpadapter/activity_test.go` | Exceptional activity remains complete but output/text invent no outcome, trace, cause, or retry | Committed finalization-failed SSE-derived envelope | Real DTO/mapper | Ephemeral diagnostic formats | Workflow failure visibility |
| `TestExecutionActivityMaximumPageHas64CompleteItemsWithoutTruncation` | SDK integration | `internal/mcpadapter/activity_test.go` | Maximum page preserves 64 whole envelopes, concise text lines, valid JSON, and no 65th item | 65 near-12-KiB activities with distinct cursors | Real live service and SDK | Persisted or serialized contracts | Response-framing acceptance |

### 7. Capabilities, discovery, and SDK protocol boundary

| Name | Type | Location | What it proves | Fixtures/data | Mocks | Contract classification | Compatibility expectation |
| --- | --- | --- | --- | --- | --- | --- | --- |
| `TestRuntimeCapabilitiesMatchCompletePR17ToolFamilies` | Unit | `internal/mcpadapter/capabilities_test.go` | Static table maps exact three capabilities to exact five tools; runtime status remains first/retained | Descriptor table | None | Persisted or serialized contracts | Additive existing runtime contract |
| `TestCapabilityConformanceRejectsEveryMissingRequiredTool` | Unit table | `internal/mcpadapter/capabilities_test.go` | A test-only validator reports every missing family member as a static server-definition defect; production adds no runtime suppression/dependency machinery | One case per tool | None | Persisted or serialized contracts | New exact capability promise |
| `TestCapabilityAdvertisementDoesNotDependOnTargetState` | Unit/integration | `internal/mcpadapter/runtime_test.go` | No target, auth required, incompatible, live unavailable, and connected states report the same installed set | Existing status matrix | Fake status provider | Persisted or serialized contracts | Capability/status separation |
| `TestStatelessServerDiscoversExactPR17Surface` | SDK integration | `internal/mcpadapter/server_test.go` | Runtime plus five tools, one resource template, schemas, annotations, no prompts/trace tools, and same stateless transport | Real SDK session | Real server dependencies over test target | Persisted or serialized contracts | New exact v1 surface |
| `TestCompatible2025ProtocolDiscoversAndUsesPR17Surface` | Raw JSON-RPC compatibility smoke | `internal/mcpadapter/server_test.go` | `2025-11-25` initializes on the same server, discovers the PR 17 tools/resource template, calls one representative tool, and reads one skill resource without alternate DTOs or behavior | One target/skill fixture and existing authenticated transport | Production SDK/server | Persisted or serialized contracts | SDK-provided previous-revision compatibility only |
| `TestProtocolFailuresRemainSeparateFromDomainFailures` | Raw JSON-RPC/SDK integration | `internal/mcpadapter/server_test.go` | Unknown tool, malformed arguments, access rejection, and framing use SDK/HTTP paths; service error uses Loomspan envelope | Raw requests and one fake service failure | Production security handler | Persisted or serialized contracts | Protected adapter separation |
| `TestRuntimeGoldenAddsCapabilitiesWithoutChangingStatusShape` | Golden | `internal/mcpadapter/runtime_test.go`, runtime golden | Existing runtime result/text/status fields remain byte-stable except approved additive capability array | Existing no-target/status matrix | None | Persisted or serialized contracts | Protected runtime-status path |

The official conformance harness remains protocol-generic. Do not add fake
product tools/resources solely to satisfy unrelated official fixture scenarios.
The complete product contract is tested once through the current/default
`2026-07-28` SDK path. The `2025-11-25` row above is the only PR 17-specific
previous-revision smoke; do not parameterize every product test across both
revisions or add production protocol-version branches. Existing official
conformance invocations continue to cover both negotiated revisions.

### 8. Browser/MCP parity and production composition

| Name | Type | Location | What it proves | Fixtures/data | Mocks | Contract classification | Compatibility expectation |
| --- | --- | --- | --- | --- | --- | --- | --- |
| `TestBrowserAndMCPPreserveSameSkillFacts` | Integration | `internal/console/mcp_runtime_integration_test.go` | Same application response yields equal registered name/source/YAML/observation/scope facts; wrappers/links may differ | Committed skill REST fixtures | One `httptest` application behind production target context | Persisted or serialized contracts | Adapter parity (`WF-X-R7`, `WF-SP-R7-R9`) |
| `TestBrowserAndMCPPreserveSameActiveExecutionFacts` | Integration | `internal/console/mcp_runtime_integration_test.go` | Every active identity/state/path/usage/limit fact agrees | Active list/detail fixtures | Same | Ephemeral diagnostic formats | Adapter parity (`WF-SE-R2/R6/R9/R10`) |
| `TestBrowserAndMCPPreserveSameRecentContinuityAndGapFacts` | Integration | `internal/console/mcp_runtime_integration_test.go` | Same shared window produces equal items/order/query time/continuity/reset/beginning facts | Replay/reset/eviction activities | Same shared live service | Ephemeral diagnostic formats | Adapter parity (`WF-X-R6/R7/R10`, `WF-FE-R10`, `WF-SE-R3/R9`) |
| `TestBrowserAndMCPPreserveSameDomainErrorMeanings` | Integration table | `internal/console/mcp_runtime_integration_test.go` | Shared code/message/scope/details survive both wrappers, including sanitized `CONSOLE_ERROR` | Existing problem fixtures and sentinel internal cause | Same | Persisted or serialized contracts | Shared error compatibility |
| `TestProductionCompositionRegistersOneSharedRuntimeInspectionPath` | Integration | `internal/console/mcp_runtime_integration_test.go` | Browser and MCP use the same target/observability/live instances and MCP creates no direct application/subscription path | Request/subscription counters | Production composition with test hooks already used by console tests | Internal or accidentally exposed implementation | No duplicate state/path |

Comparisons should decode protocol wrappers and compare the documented Loomspan
facts explicitly. They must not demand byte-identical browser and MCP JSON.

### 9. Cancellation, target rotation, credential lifecycle, and clients

| Name | Type | Location | What it proves | Fixtures/data | Mocks | Contract classification | Compatibility expectation |
| --- | --- | --- | --- | --- | --- | --- | --- |
| `TestPR17ToolCancellationCancelsUpstreamAndSuppressesResult` | Integration | `internal/mcpadapter/lifecycle_test.go` | Client cancellation reaches `Scope.Upstream`; no late structured result | Blocking fake application request | Channels, real target scope/tracker | Ephemeral diagnostic formats | Protected cancellation behavior |
| `TestPR17TargetRotationDuringToolCallReturnsTargetChanged` | Integration | `internal/console/mcp_runtime_integration_test.go` | Rotation between capture/service/publication cancels prior work and suppresses stale success | Blocking skill/execution endpoint and two targets | Channels/real target context | Ephemeral diagnostic formats | Protected scope behavior |
| `TestPR17CredentialRegenerationSuppressesOldGenerationResult` | Integration | `internal/mcpadapter/lifecycle_test.go` | Old authenticated request cannot publish after key generation changes | Blocking handler and credential store | Existing lifecycle harness | Persisted or serialized contracts | Protected PR 16 authentication behavior |
| `TestPR17ShutdownCancelsAndDrainsRequests` | Integration | `internal/mcpadapter/lifecycle_test.go` | Shutdown cancels handlers, closes sessions, and leaves no result/state | Blocking tool call | Existing tracker/lifecycle harness | Internal or accidentally exposed implementation | Protected shutdown behavior |
| `TestTwoMCPClientsShareLiveWindowButCancelIndependently` | Integration/race | `internal/console/mcp_runtime_integration_test.go` | Two authenticated clients observe the same interval, one cancellation does not affect the other, and only one upstream SSE subscription exists | Subscription counter and interleaved calls | Production composition/test application | Ephemeral diagnostic formats | Stateless multi-client coherence |
| `TestPR17ResultsNeverContainApplicationOrMCPCredentials` | Integration | `internal/console/mcp_runtime_integration_test.go` | Structured/text/resource/error/continuation bytes omit both sentinel secrets | Distinct sentinel keys and all result forms | Test application/profile | Persisted or serialized contracts | Security/redaction coherence |

Run the race detector over `mcpadapter`, `live`, and `console`. Tests must use
channels/deadlines rather than arbitrary sleeps for new lifecycle assertions.

### 10. Maximum framing and manual client interoperability

| Name | Type | Location | What it proves | Fixtures/data | Mocks | Contract classification | Compatibility expectation |
| --- | --- | --- | --- | --- | --- | --- | --- |
| `TestListSkillsMaximumPageHas64WholeItems` | SDK integration | `internal/mcpadapter/skills_test.go` | 64 summaries, exact count, valid continuation, and no truncation | Generated bounded unique skills plus page cursor | Test application | Persisted or serialized contracts | Response-framing acceptance |
| `TestListExecutionsMaximumPageHas64WholeItems` | SDK integration | `internal/mcpadapter/executions_test.go` | 64 complete active summaries and no inferred fields/truncation | Generated valid execution DTOs | Test application | Persisted or serialized contracts | Response-framing acceptance |
| `TestExecutionActivityMaximumPageHas64CompleteItemsWithoutTruncation` | SDK integration | `internal/mcpadapter/activity_test.go` | 64 near-bound complete activity envelopes and concise text | Generated activities | Real live service | Persisted or serialized contracts | Response-framing acceptance |
| Manual matrix row per client | Manual interoperability | `docs/mcp-client-compatibility.md` | Actual connection/discovery/structured-text/error/resource/continuation/64-item behavior with version/date/platform | Running compatible target and deterministic fixture scenario | None | Persisted or serialized contracts | Dated evidence, not permanent guarantee |

If any representative client fails the maximum page, lower the one MCP maximum
below 64, update the ticket/implementation/testing plans and schema goldens, and
rerun all maximum-page and client cases. Do not add client-specific servers,
schemas, or fallbacks.

## Requirement-to-Test Traceability

| Requirement | Primary evidence |
| --- | --- |
| `WF-X-R6` | Atomic query/continuity observation unit test and browser/MCP recent parity |
| `WF-X-R7` | All four browser/MCP parity integration tests |
| `WF-X-R10` | Domain-error table, successful replay-gap test, live-unavailable test |
| `WF-FE-R10` | Gap/reset/eviction tests and text golden that never claims durable history |
| `WF-SE-R2` | Active detail golden plus recent activity golden |
| `WF-SE-R3` | Distinct observation-time tests and continuity parity |
| `WF-SE-R6` | Execution golden/schema absence test for complete hierarchy |
| `WF-SE-R9` | Active/recent browser-MCP parity tests |
| `WF-SE-R10` | Execution output absence test for finalized calculations |
| `WF-SP-R2` | Active bounded path golden and schema test |
| `WF-SP-R7` | Skill detail/resource integration tests |
| `WF-SP-R8` | Byte-for-byte YAML golden and maximum YAML test |
| `WF-SP-R9` | Descriptive-only source path and unsafe URI tests |
| Capability completeness | Capability descriptor and missing-tool negative table |
| Structured safe errors | Envelope golden, cause-leak test, protocol/domain separation test |
| Stateless local simplicity | Continuation allowlist test, one-shared-path composition test, two-client test |

## Test Data and Mocking Rules

- Prefer current committed Java-shaped REST/SSE fixtures and existing DTO
  builders. Do not introduce a second “MCP scenario” corpus for the workflows.
- MCP golden files contain only MCP serialized contracts and have exact
  inventory checks.
- Use `httptest.Server` for application REST/SSE behavior, real
  `target.Context`, real `observability.Service`, real `live.Service`, and the
  real MCP SDK/server wherever testing the assembled boundary.
- Use pure DTO mapping tests for exhaustive field/text goldens; do not require a
  network server for every formatting case.
- Use injected clocks and channel gates. Avoid new wall-clock sleeps except in
  existing polling tests that cannot yet be refactored within scope.
- Never place a real MCP/application key in fixtures, command arguments, logs,
  snapshots, or failure output. Use conspicuous sentinel secrets and assert
  their absence.
- Do not mock filesystem access for `sourcePath`; prove no filesystem method is
  in the handler path and that path-looking values remain returned data.
- Do not alter Java fixture writers or compatibility versions for PR 17.

## Tests Explicitly Not Added

- No historical/cross-restart continuation compatibility or migration test.
- No old/new `Recent` signature compatibility test or `NewServer` overload test.
- No browser UI/Playwright test unless implementation changes frontend code;
  browser API tests cover the only planned browser behavior correction.
- No Agent Skill or model-quality evaluation; PR 19 owns it.
- No trace/resource/payload/raw-artifact tests; PR 18 owns them.
- No remote, OAuth, multi-user, role, per-client permission, or denial-policy
  matrix.
- No arbitrary load/endurance benchmark or cumulative traversal quota test.
- No test that decodes MCP continuation as a supported client contract; only
  server codec tests may inspect its private payload.

## How to Run

### Preconditions

- Run Go commands from `loomspan-console/` with the exact repository Go
  toolchain.
- Canonical verification additionally requires the pinned Node.js/npm versions
  documented in `loomspan-console/README.md`.
- Automated tests use temporary profiles, `httptest` listeners, sentinel keys,
  and committed fixtures. They require no developer MCP key, application key,
  external service, or internet access beyond whatever the existing canonical
  build/conformance tooling already requires.
- Manual client validation requires a locally running compatible Loomspan
  application, Console, canonical MCP key configured through the documented
  user/global client mechanism, and the selected client builds. Never put the
  key in a repository file, URL, shell-history command, test log, or screenshot.

### Failing test first

From `loomspan-console/`:

```text
go test ./internal/live -run TestRecentActivityQueryDoesNotReturnRetainedStateWhenLiveUnavailable -count=1
```

Record the expected pre-fix item-count failure. After the shared fix, the same
test must pass with the final domain-error assertion.

### Focused implementation loop

```text
go test ./internal/live ./internal/browserapi
go test ./internal/mcpadapter
go test ./internal/console
```

For the single Loomspan product contract while iterating:

```text
go test ./internal/mcpadapter -run 'Test(ListSkills|GetSkill|ListExecutions|GetExecution|ExecutionActivity|SkillResource|Continuation|Capability|Stateless)' -count=1
```

### Full automated verification

From `loomspan-console/`:

```text
go test ./...
go test -race ./internal/mcpadapter ./internal/live ./internal/console
go run ./internal/buildtool mcp-conformance
go run ./internal/buildtool verify
```

From the repository root on Windows:

```text
.\mvnw.cmd -pl loomspan-spring-boot-starter -Dtest=LoomspanPublicSurfaceArchitectureTest test
git diff --check
```

On POSIX, use `./mvnw` with the same Maven arguments.

### Manual client matrix

For each available local Codex surface, Claude Code, Cursor, Antigravity, and
Devin Desktop/Windsurf/Cascade or local Devin CLI:

1. Record client build/version, platform, date, and configuration mechanism.
2. Authenticate to the loopback Streamable HTTP endpoint and call
   `LOOMSPAN_get_runtime`.
3. Confirm exactly the runtime plus three PR 17 capabilities.
4. Discover the five tools and skill resource template.
5. Call skill list/detail and read the corresponding Unicode-safe resource.
6. Call active list/detail and verify provisional/bounded presentation.
7. Query recent activity, save the opaque continuation, admit later activity,
   and continue forward.
8. Observe one shared domain failure and record structured/text/`isError`
   presentation separately from one malformed-input failure.
9. Exercise a representative 64-item result and record success/failure without
   pasting sensitive returned content into the matrix.
10. Mark unavailable or unautomatable cases explicitly; do not infer a pass.

## Exit Criteria

- [x] The minimal live-unavailable test was observed failing on pre-fix code and
  passes post-fix with exact `LIVE_MONITORING_UNAVAILABLE` and no retained data.
- [x] All old tuple-style `Recent` calls and the old `NewServer` constructor use
  are removed; no shim/overload preserves obsolete internal behavior.
- [x] All five tools expose exact strict input/output schemas, annotations,
  structured success/error envelopes, and deterministic text fallbacks through
  a real SDK session.
- [x] Every domain error has exactly one structured `error`, `isError: true`, a
  safe `CODE: message` text block, and no internal cause/path/credential leak.
- [x] Protocol/schema/authentication failures remain outside the Loomspan domain
  envelope, and replay gaps remain successful results.
- [x] Resource URI parsing uses the canonical single-decode `EscapedPath`
  algorithm, and resource errors use the exact `-32602`/`-32000` mapping with
  object-valued safe data.
- [x] Continuation tests cover valid round trips, operation/scope/session
  binding, 4,096/8,192 boundaries, malformed input, target change, and fuzzing;
  no signing, encryption, persistence, cache, registry, or credential field is
  introduced.
- [x] Skill tools/resource preserve unchanged YAML and descriptive-only
  `sourcePath`; URI parsing and JSON-RPC error data are canonical and safe.
- [x] Active execution results preserve every bounded current fact and add no
  completion history, full hierarchy, health, causality, or final attribution.
- [x] Activity results preserve complete ordered envelopes, one interval,
  distinct observation meanings, cursor range, forward continuation,
  gap/reset/finalization-failed facts, and live-unavailable behavior.
- [x] `MODEL_ATTEMPT_FAILED` is accepted, labeled, and preserved with no Java
  event, REST/SSE, or fixture change.
- [x] Capability conformance proves every family is complete, target-state
  independent, and limited to PR 17.
- [x] The complete product contract passes on current `2026-07-28`; one compact
  `2025-11-25` initialize/discover/call/read smoke passes on the same production
  server with no version-specific Loomspan DTO, handler, resource, or branch.
- [x] Browser/MCP parity tests cover the cited `WF-*` requirements and compare
  semantic facts rather than requiring identical protocol wrappers.
- [x] Cancellation, target rotation, credential mutation, shutdown, multiple
  clients, one upstream subscription, and credential non-leakage pass,
  including the race detector.
- [x] Maximum YAML and 64-item skill/execution/activity tests contain complete
  values with no silent truncation.
- [x] `go test ./...`, focused race tests, MCP conformance, canonical Console
  verification, `LoomspanPublicSurfaceArchitectureTest`, and
  `git diff --check` all pass.
- [x] Existing Java REST/SSE fixtures and compatibility marker are unchanged.
- [x] No supported Java API/SPI, configuration, manifest, or skill-authoring
  guidance changed; the protected architecture/configuration paths still pass.
- [x] The dated manual compatibility matrix contains actual evidence or an
  explicit “not run” for every named client family, and any failure at 64 items
  was resolved by one globally updated lower maximum rather than client-specific
  behavior.
- [x] Documentation and golden inventories contain only the implemented PR 17
  surface and no trace, Agent Skill, workflow-specific, or legacy alternate
  contract.
- [ ] A separate reviewer can trace every ticket acceptance signal and cited
  workflow requirement to the tests in this artifact without consulting the
  originating conversation.

## References

- Implementation plan:
  `ai/thoughts/plans/2026-08-13-pr-17-mcp-runtime-inspection.md`
- Ticket:
  `ai/thoughts/tickets/loomspan-console-pr-17-mcp-runtime-inspection.md`
- Research:
  `ai/thoughts/research/2026-08-13-loomspan-console-pr-17-mcp-runtime-inspection.md`
- Phase 3 design:
  `ai/thoughts/phases/loomspan_console_phase_3_llm_runtime_inspector.md`
- Canonical workflows:
  `ai/thoughts/phases/loomspan_console_workflows.md`
- Compatibility policy:
  `ai/thoughts/framework-feature-design-lens.md`
- Current MCP compatibility evidence:
  `loomspan-console/docs/mcp-client-compatibility.md`
