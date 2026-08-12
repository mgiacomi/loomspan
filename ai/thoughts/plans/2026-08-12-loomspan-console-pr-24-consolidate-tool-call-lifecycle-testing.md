# PR 24 Consolidate Tool-Call Lifecycle Testing Plan

## Change Summary

- Replace the adjacent tool-requested/tool-started trace pair with one canonical `TOOL_CALL_STARTED` emitted on the open `TOOL_INVOCATION` frame immediately before capability execution.
- Move execution-journal projection to the retained start record while preserving recursive sensitive-field sanitization, planned/unplanned classification, and one entry per invocation.
- Preserve one bounded live start activity without arguments and one tool-invocation usage increment.
- Remove the obsolete record from the strict Java and Go current-release vocabularies without an alias, legacy reader, migration fixture, or compatibility marker.
- Add Java-owned planned-success and unplanned-failure trace fixtures consumed by Go.
- Add an explicit, collapsed Tool input inspector for finalized `TOOL_CALL_STARTED` records.
- Update author-facing trace/debugging guidance using focused tests and fixtures as evidence.

## Impacted Areas

- **Java writer vocabulary and boundary**
  - `TraceRecordType`
  - `ExecutionTraceRecorder` / `DefaultExecutionTraceRecorder`
  - `DefaultExecutionStateService`
  - `DefaultToolCallbackFactory`
- **Java projections**
  - `ExecutionJournalProjector`
  - `LiveActivityProjector`
  - session quota/usage and tool outcome metrics at the unchanged callback boundary
- **Java tests with constructed traces**
  - `ExecutionStateServiceTest`
  - `ToolCallbackFactoryTest`
  - `ExecutionJournalProjectorTest`
  - `ExecutionJournalProjectionContractTest`
  - `LiveActivityProjectorTest`
  - `LoomspanSessionTest`
  - `LoomspanSessionRunnerTest`
- **Java-to-Go current-release trace contract**
  - `ConsoleTraceFixtureCorpusTest`
  - `loomspan-console-fixtures/traces/`
  - `loomspan-console-fixtures/expected/`
  - `loomspan-console/internal/traceanalysis/enums.go`
  - Go parser, fixture-corpus, record-query, frame, failure, and raw-range behavior
- **Browser finalized-trace inspection**
  - `TraceRecords.tsx`
  - new `TraceRecords.toolInput.test.tsx`
  - `activityPresentation.test.ts`
- **Documentation claims requiring executable evidence**
  - `ai/skill-authoring/traces-and-debugging.md`
  - `ai/skill-authoring/README.md`
  - `ai/thoughts/phases/loomspan_console_phase_2_ui_console.md`
  - `loomspan-console-fixtures/README.md`

## Risk Assessment

### High-risk behaviors

- The retained start could accidentally move after `CapabilityExecutionRouter#execute`, causing capability side effects or failures to occur without start evidence.
- Planned or unplanned identity, event ID, arguments, note, capability, linked task, or tool-frame identity could be lost while deleting the duplicate method.
- The journal could emit zero entries, two entries, or unsanitized arguments after its projection source changes.
- Live projection or session usage could double count or stop counting tool attempts.
- Failure handling could lose the start record or stop producing the existing tool failure and canonical error facts.
- Java could emit a record that strict Go parsing no longer recognizes, or Go could continue accepting the removed record behind an unintended legacy path.
- Regenerated fixtures could introduce unrelated sequence, line-ending, transport-fixture, or expected-model churn.
- The browser could fetch or display tool arguments automatically, expose them in list/live content, interpret malicious-looking text, or mislabel unplanned calls as failures.
- Task-title correlation could invent a title when the owning plan is ambiguous or unavailable.

### Edge cases

- Planned invocation with task ID and resolvable title.
- Planned invocation with a recorded task ID but no mechanically resolvable title.
- Unplanned invocation with `metadata.unplanned: true`, no linked task, and a note.
- Successful invocation with one start and one completion.
- Thrown capability with a retained start, one failure record, canonical error linkage, and a closed frame.
- Start with no terminal record: outcome remains unknown.
- Empty argument object, nested JSON object, array/scalar value, JSON-encoded string, and plain text.
- Credential-like argument field names: finalized Tool input stays unchanged, while the execution journal remains recursively sanitized.
- Malicious-looking HTML/Markdown/script text rendered inertly.
- Multi-range raw-record retrieval, invalid continuation, invalid UTF-8, malformed record data, loading, and retrieval failure.
- Plan history spanning more than one page and unavailable task correlation.
- Keyboard activation, focusable native action, `aria-expanded`, `aria-controls`, labeled detail region, loading status, and error alert.

### Contract and compatibility expectations

- **Application API**: no impact; existing module and architecture tests must continue to pass.
- **Supported SPI**: no impact; no new bean, interface, replacement point, or public extension contract may appear.
- **Configuration and manifest contracts**: no impact; no new compatibility test is needed beyond the existing module suite.
- **Persisted or serialized contracts**: no deliberately durable contract is affected; do not test historical trace readability.
- **Ephemeral diagnostic formats**: affected. Tests must prove current writer/reader/projector/browser coherence, accurate ordering and terminal visibility, deliberate input disclosure, and the existing security boundary.
- **Internal or accidentally exposed implementation**: intentionally remove the public-but-internal enum member and recorder method atomically. Tests must not require old and new behavior simultaneously.
- **Protected Java-to-Go path**: the exact release gate, strict parser, Java-owned fixture corpus, and observable record/frame/failure semantics remain protected. Existing exact release-string rejection must still pass; no marker is incremented.
- **Intentionally removed path**: requested-record emission, enum acceptance, recorder method, manually constructed requested test records, labels, fixtures, and documentation references. No alias, fallback, translation, or legacy-specific reader test is allowed.

### Authoring claims requiring evidence

The updated trace/debugging guide may state only what the following executable evidence proves:

- One start is appended before invocation: failing-first state test plus callback/router ordering test.
- Planned and unplanned facts differ mechanically: state tests and Java-owned fixtures.
- Success and failure retain separate terminal records: integration tests and fixtures.
- A start alone does not establish outcome: browser presentation test with no terminal correlation and documented terminal semantics.
- Tool input is explicit finalized-trace detail and absent from live activity: browser component tests plus `LiveActivityProjectorTest`.
- Arguments are inert authenticated diagnostic content, not newly redacted: browser text/inertness tests and existing authenticated raw-range boundary.
- Journal arguments remain sanitized independently of raw Tool input: journal projector tests.

No test should assert prose text merely to exercise documentation. Tests establish the underlying lifecycle, projection, retrieval, and rendering semantics.

## Existing Test Coverage

### Java coverage to reuse

- `ExecutionStateServiceTest#recordsRuntimeTraceEventsAgainstTheActiveFrameAndIncludesRequestedAndRootMissionTyping` currently proves both duplicate records are emitted against the active frame. Rewrite it into focused planned/unplanned start tests.
- `ToolCallbackFactoryTest#routesMappedExecutionsThroughPublicTraceIdentity` already verifies frame-open, log-call, result, and close ordering through mocked collaborators, but it does not include the router in ordered verification.
- `ToolCallbackFactoryTest#recordsToolThrowableOnActiveFrameBeforeClosingIt` protects failure recording before frame closure, but does not assert retained start evidence.
- `ExecutionJournalProjectorTest#derivesSanitizedDeveloperFacingJournalFromTrace` protects recursive argument sanitization.
- `ExecutionJournalProjectorTest#doesNotInferUnplannedToolCallsFromLegacyMessageText` protects metadata-based classification; update its source record without preserving the obsolete lifecycle.
- `ExecutionJournalProjectionContractTest#projectsCanonicalDeveloperFacingJournalFromRepresentativeTraceStream` protects one developer-facing call/result sequence.
- `LiveActivityProjectorTest#projectsExactlyTheSettledVisibleRecordKinds`, `#derivesCountsAndNormalizedModelUsageFromCanonicalFacts`, and `#boundsPathTextAndDoesNotRetainLogicalPayload` protect visible kinds, usage, bounds, and payload exclusion.
- `LoomspanSessionTest` and `LoomspanSessionRunnerTest` contain constructed requested records used by journal/session scenarios; update them to the retained record.
- `SessionUsageServiceTest`, `SessionQuotaTest`, and `MicrometerUsageMetricsRecorderTest` already protect generic quota and metric behavior. They need not be rewritten unless callback integration exposes a regression.

### Cross-language coverage to reuse

- `ConsoleTraceFixtureCorpusTest` generates and byte-compares current-release Java fixtures, protects inventory, LF output, transport-fixture isolation, and semantic invariants.
- Go `fixture_corpus_test.go` processes every committed fixture and compares Java-produced expected facts.
- `parser_test.go#TestParserRejectsUnsupportedEnum` already proves strict unknown-record rejection without naming historical values.
- `continuation_test.go#TestAllFixtureFramesRecordsAndPayloadsAreReachableThroughFiniteCalls` protects finite access to every fixture record.
- `range_test.go#TestServiceReadRawRecordRange` and continuation tests protect explicit bounded raw-record retrieval.
- `console/observability_integration_test.go#TestScopeOpenArtifactRejectsIncompatibleTarget` protects exact release-string rejection before artifact use.

### Browser coverage to reuse

- `TraceRecords.results.test.tsx` protects Tool result pretty formatting, task correlation, evidence linkage, and terminal receipt presentation.
- `TraceRecords.plan.test.tsx` protects paginated plan retrieval and comparison patterns.
- `TraceRecords.raw.test.tsx` protects explicit raw-record retrieval and inert `<pre>` presentation.
- `TraceRecords.stepAction.test.tsx` protects paginated correlation, missing related evidence, and action-detail wording.
- `activityPresentation.test.ts` protects the bounded live tool-start facts and currently contains no input rendering.

### Coverage gaps

- No test currently fails solely because one invocation emits two pre-invocation lifecycle records.
- No test observes the trace from inside `router.execute` to prove start-before-side-effect ordering.
- No current fixture contains a tool start, completion, or failure.
- No Go fixture test asserts a one-start/one-terminal tool lifecycle on the same frame.
- No finalized-trace Tool input inspector or component test exists.
- No current browser test proves tool arguments stay unfetched until the explicit Tool input action.
- Trace/debugging guidance has no focused tool-lifecycle evidence anchors.

## Bug Reproduction / Failing Test First

- **Name**: `recordsOneCanonicalStartForPlannedToolCall`
- **Type**: unit
- **Location**: `loomspan-spring-boot-starter/src/test/java/com/lokiscale/loomspan/internal/runtime/state/ExecutionStateServiceTest.java`
- **Contract classification**: Ephemeral diagnostic formats
- **Compatibility expectation**: current-run diagnostic coherence and approved removal
- **Arrange**:
  - Create a session and `DefaultExecutionStateService` using the fixed test clock.
  - Open a root mission frame, then a `TOOL_INVOCATION` frame.
  - Construct a deterministic `TaskExecutionEvent` with a known event ID, capability, linked task, nested arguments, and optional note.
- **Act**: call `logToolCall` once.
- **Assert**:
  - Filter records to the tool frame.
  - Assert the frame's record types are exactly `FRAME_OPENED`, then `TOOL_CALL_STARTED`.
  - Assert the start contains the deterministic event ID, capability, linked task, arguments, and no unplanned flag.
- **Expected failure before the fix**: the exact record-type assertion receives an additional pre-invocation record between `FRAME_OPENED` and `TOOL_CALL_STARTED`.
- **Why this is minimal**: it uses the lowest-level public behavior responsible for duplication, requires no router, Spring context, Go process, browser, or generated fixture, and does not name or preserve the obsolete vocabulary in the test itself.
- **Failing-first procedure**:
  1. Add only this test.
  2. Run `\.\mvnw.cmd -pl loomspan-spring-boot-starter -Dtest=ExecutionStateServiceTest#recordsOneCanonicalStartForPlannedToolCall test -DfailIfNoTests=false`.
  3. Record the failure showing the unexpected extra record.
  4. Implement the Java lifecycle consolidation.
  5. Re-run the same command and require it to pass unchanged.

## Tests to Add or Update

### 1. `recordsOneCanonicalStartForPlannedToolCall`

- **Type**: unit; failing-first
- **Location**: `ExecutionStateServiceTest.java`
- **What it proves**: one planned start, correct frame/order, and complete retained payload.
- **Fixtures/data**: deterministic event ID; nested argument map; linked task; fixed clock.
- **Mocks**: none; real state service and trace recorder.
- **Contract classification**: Ephemeral diagnostic formats
- **Compatibility expectation**: current-run diagnostic coherence and approved removal

### 2. `recordsOneCanonicalStartForUnplannedToolCall`

- **Type**: unit
- **Location**: `ExecutionStateServiceTest.java`
- **What it proves**: one unplanned start has `unplanned: true`, no linked task, preserved arguments, event ID, capability, and note.
- **Fixtures/data**: deterministic unlinked `TaskExecutionEvent` with the canonical no-ready-task note.
- **Mocks**: none.
- **Contract classification**: Ephemeral diagnostic formats
- **Compatibility expectation**: current-run diagnostic coherence

### 3. `logsCanonicalStartBeforeCapabilityExecutionAndCountsAttemptOnce`

- **Type**: component/unit integration
- **Location**: `ToolCallbackFactoryTest.java`
- **What it proves**: after plan resolution and tool-frame opening, `logToolCall` or `logUnplannedToolCall` occurs before `router.execute`; session usage records one attempt; outcome metrics record one success or failure rather than one per trace record.
- **Fixtures/data**: one planned callback and one unplanned callback with simple arguments.
- **Mocks**: mock planning service, execution state service, router, session usage service, and metrics recorder; use Mockito `InOrder` across state service and router plus exact invocation counts.
- **Contract classification**: Internal or accidentally exposed implementation
- **Compatibility expectation**: protected invocation ordering; no new execution state

### 4. `retainsStartAndRecordsFailureWhenCapabilityThrows`

- **Type**: integration-style unit
- **Location**: new `loomspan-spring-boot-starter/src/test/java/com/lokiscale/loomspan/internal/runtime/tool/ToolCallLifecycleTraceTest.java`, or `ToolCallbackFactoryTest.java` if the fixture remains readable there
- **What it proves**: using a real state service/trace recorder around a throwing router, the final tool frame contains one start followed by one failure; the canonical error remains linked; the frame closes after failure recording; no completion is invented.
- **Fixtures/data**: real in-memory session, deterministic planned or unplanned event path, throwing router, fixed clock.
- **Mocks**: mock only planning/router/external collaborators; use real trace state and inspect records.
- **Contract classification**: Ephemeral diagnostic formats
- **Compatibility expectation**: current-run failure visibility and ordering

### 5. `recordsStartAndCompletionForSuccessfulCapability`

- **Type**: integration-style unit
- **Location**: `ToolCallLifecycleTraceTest.java`
- **What it proves**: success retains exactly one start and one completion on the same tool frame, with no failure record and unchanged linkage.
- **Fixtures/data**: real session/state recorder and a deterministic successful router result.
- **Mocks**: planning/router only.
- **Contract classification**: Ephemeral diagnostic formats
- **Compatibility expectation**: current-run success visibility

### 6. Journal projection cases

- **Names**:
  - update `derivesSanitizedDeveloperFacingJournalFromTrace`
  - rename/update `doesNotInferUnplannedToolCallsFromLegacyMessageText` to `classifiesUnplannedToolCallOnlyFromRecordedMetadata`
  - update `projectsCanonicalDeveloperFacingJournalFromRepresentativeTraceStream`
- **Type**: unit/contract
- **Locations**: `ExecutionJournalProjectorTest.java`, `ExecutionJournalProjectionContractTest.java`
- **What they prove**: start produces exactly one call entry; planned/unplanned classification uses recorded metadata/linkage; nested credential-like fields remain recursively redacted; result/failure entries remain distinct.
- **Fixtures/data**: constructed started records with nested arguments, note, task linkage, and `unplanned: true` case.
- **Mocks**: none.
- **Contract classification**: Ephemeral diagnostic formats
- **Compatibility expectation**: current-run projection coherence; approved source-record migration

### 7. Live activity and derived usage preservation

- **Names**:
  - update `projectsExactlyTheSettledVisibleRecordKinds`
  - update/extend `derivesCountsAndNormalizedModelUsageFromCanonicalFacts`
  - add `toolStartActivityExcludesArgumentsAndCountsOnce`
- **Type**: unit
- **Location**: `LiveActivityProjectorTest.java`
- **What they prove**: retained start remains visible; one start increments tool usage once; arguments and note payload are absent from activity/snapshot strings; activity summary remains "Tool call started" with bounded identity facts.
- **Fixtures/data**: one logical start record with conspicuous secret/malicious argument text.
- **Mocks**: none.
- **Contract classification**: Ephemeral diagnostic formats
- **Compatibility expectation**: protected live activity and usage semantics

### 8. Update constructed Java trace scenarios

- **Names**: existing affected cases in `LoomspanSessionTest` and `LoomspanSessionRunnerTest`
- **Type**: unit/regression
- **Locations**: corresponding Java test files
- **What they prove**: session journal projection and runner behavior use the retained current-release record without preserving duplicate behavior.
- **Fixtures/data**: existing constructed trace records changed in place.
- **Mocks**: existing test setup.
- **Contract classification**: Internal or accidentally exposed implementation
- **Compatibility expectation**: approved atomic removal

### 9. Java fixture lifecycle semantics

- **Name**: `toolLifecycleFixturesContainOneCanonicalStartAndTerminalRecord`
- **Type**: cross-language contract fixture test
- **Location**: `ConsoleTraceFixtureCorpusTest.java`
- **What it proves**:
  - `planned-tool-success` has one start then one completion on the same tool frame, linked task, arguments/event identity, successful completion, and terminal tool count one.
  - `unplanned-tool-failure` has one start then one failure, `unplanned: true`, no linked task, note, canonical error/final failure linkage, and terminal tool count one.
  - Neither case relies on record adjacency to infer outcome beyond explicitly checking ordered canonical records.
- **Fixtures/data**: two deterministic Java-generated NDJSON and expected semantic artifacts.
- **Mocks**: no cross-process mocks; generator may use deterministic state/recorder helpers.
- **Contract classification**: Ephemeral diagnostic formats
- **Compatibility expectation**: required Java-to-Go current-release coherence

### 10. Go fixture ingestion and record reachability

- **Name**: `TestToolLifecycleFixturesExposeOneCanonicalStartAndTerminalRecord`
- **Type**: integration/contract
- **Location**: `loomspan-console/internal/traceanalysis/fixture_corpus_test.go`
- **What it proves**: Go parses both Java-owned tool fixtures, exposes each retained start and terminal record through record queries, preserves canonical sequence and frame identity, and resolves failure/final outcome facts without a legacy reader.
- **Fixtures/data**: generated `planned-tool-success` and `unplanned-tool-failure` artifacts.
- **Mocks**: existing fake artifact sink/service harness only.
- **Contract classification**: Ephemeral diagnostic formats
- **Compatibility expectation**: required Java-to-Go current-release coherence

### 11. Strict Go vocabulary and compatibility gate

- **Names**:
  - existing `TestParserRejectsUnsupportedEnum`
  - existing `TestScopeOpenArtifactRejectsIncompatibleTarget`
- **Type**: unit/integration regression
- **Locations**: `parser_test.go`, `console/observability_integration_test.go`
- **What they prove**: unknown current-release values remain rejected generically, and an exact release mismatch prevents artifact acquisition before parsing.
- **Fixtures/data**: existing future enum and incompatible instance responses.
- **Mocks**: existing parser/test HTTP server harness.
- **Contract classification**: Ephemeral diagnostic formats for parsing; protected release protocol for the gate
- **Compatibility expectation**: protected strictness; no new marker or legacy-specific acceptance

### 12. `showsPlannedToolInputOnlyAfterExplicitSelection`

- **Type**: browser component
- **Location**: new `loomspan-console/web/src/observability/TraceRecords.toolInput.test.tsx`
- **What it proves**: a start row initially shows no arguments and performs no raw-range request; activating Tool input loads the full record, displays tool, planned status, task title/ID, event ID, optional note, and pretty JSON arguments.
- **Fixtures/data**: planned start record; multi-page plan history with matching owning skill/task; raw record response.
- **Mocks**: mock `getRawRecordRange` and `getTraceRecords`; use Testing Library user events.
- **Contract classification**: Ephemeral diagnostic formats
- **Compatibility expectation**: current-run debugging-tool coherence and deliberate disclosure

### 13. `showsUnplannedScalarToolInputWithoutInventingFailureOrTask`

- **Type**: browser component
- **Location**: `TraceRecords.toolInput.test.tsx`
- **What it proves**: `unplanned: true` with no linked task is labeled Unplanned, explicitly states no plan task was linked, preserves note/event ID, renders scalar/plain-text arguments, and makes no success/failure claim.
- **Fixtures/data**: unplanned start records with JSON scalar, JSON-encoded string, and plain text argument values (table-driven if readable).
- **Mocks**: raw-range client only; assert no plan query is necessary without a task.
- **Contract classification**: Ephemeral diagnostic formats
- **Compatibility expectation**: current-run diagnostic accuracy

### 14. `loadsCompleteToolInputAcrossRangesAndReportsInvalidContent`

- **Type**: browser component
- **Location**: `TraceRecords.toolInput.test.tsx`
- **What it proves**: raw continuation is followed exactly once per cursor; invalid/repeated continuation, invalid UTF-8, malformed metadata/data, or missing arguments produces an alert instead of partial/misleading detail.
- **Fixtures/data**: two-range UTF-8 record plus representative invalid cases.
- **Mocks**: sequential `getRawRecordRange` responses.
- **Contract classification**: Ephemeral diagnostic formats
- **Compatibility expectation**: protected bounded retrieval and visible failure

### 15. `rendersToolInputAsInertAccessibleDetail`

- **Type**: browser component/accessibility
- **Location**: `TraceRecords.toolInput.test.tsx`
- **What it proves**: script/HTML/Markdown-looking input is text, not DOM markup; the button works by keyboard; `aria-expanded`/`aria-controls` track state; the detail region has a record-specific label; loading uses status and failure uses alert.
- **Fixtures/data**: malicious-looking argument string and delayed/rejected raw request.
- **Mocks**: deferred and rejected raw-range promises; Testing Library keyboard/user events.
- **Contract classification**: Ephemeral diagnostic formats
- **Compatibility expectation**: protected diagnostic security and accessibility boundary

### 16. Live browser presentation remains input-free

- **Name**: update/add `presentsToolStartWithoutArguments`
- **Type**: browser unit
- **Location**: `loomspan-console/web/src/activity/activityPresentation.test.ts`
- **What it proves**: live `TOOL_CALL_STARTED` displays capability/task/unplanned bounded facts only and never promotes an `arguments` value into facts or summary.
- **Fixtures/data**: activity details containing a conspicuous nested `arguments` object to ensure it is ignored.
- **Mocks**: none.
- **Contract classification**: Ephemeral diagnostic formats
- **Compatibility expectation**: protected live-content minimization

### 17. Removal and public-surface audit

- **Type**: static verification
- **Location**: repository-wide command; existing `LoomspanPublicSurfaceArchitectureTest` in the full Java suite
- **What it proves**: active source/tests/fixtures/guidance contain no obsolete writer/reader vocabulary; no replacement public API/SPI, Spring extension point, alias, bridge, or fallback appears.
- **Fixtures/data**: final working tree and diff; historical ticket/plan artifacts excluded from the vocabulary scan.
- **Mocks**: none.
- **Contract classification**: Internal or accidentally exposed implementation
- **Compatibility expectation**: approved atomic removal

## How to Run

Run commands from `C:\opendev\code\loomspan` unless a different directory is stated.

### Failing-first Java test

```powershell
.\mvnw.cmd -pl loomspan-spring-boot-starter -Dtest=ExecutionStateServiceTest#recordsOneCanonicalStartForPlannedToolCall test -DfailIfNoTests=false
```

Require a pre-fix failure caused by the extra record, then a post-fix pass with the test unchanged.

### Focused Java lifecycle and projection tests

```powershell
.\mvnw.cmd -pl loomspan-spring-boot-starter -Dtest=ExecutionStateServiceTest,ToolCallbackFactoryTest,ToolCallLifecycleTraceTest,ExecutionJournalProjectorTest,ExecutionJournalProjectionContractTest,LiveActivityProjectorTest test -DfailIfNoTests=false
```

If the implementation does not create `ToolCallLifecycleTraceTest`, omit that class from the focused selector and keep its cases in `ToolCallbackFactoryTest`.

### Full Java module

```powershell
.\mvnw.cmd -pl loomspan-spring-boot-starter test -DfailIfNoTests=false
```

### Fixture validation and deterministic regeneration

```powershell
.\mvnw.cmd -pl loomspan-spring-boot-starter test -Dtest=ConsoleTraceFixtureCorpusTest -DfailIfNoTests=false
.\mvnw.cmd -pl loomspan-spring-boot-starter test -Dtest=ConsoleTraceFixtureCorpusTest -Dloomspan.console.fixtures.regenerate=true -DfailIfNoTests=false
git diff -- loomspan-console-fixtures
.\mvnw.cmd -pl loomspan-spring-boot-starter test -Dtest=ConsoleTraceFixtureCorpusTest -Dloomspan.console.fixtures.regenerate=true -DfailIfNoTests=false
git diff --exit-code -- loomspan-console-fixtures
```

The final `--exit-code` check is valid only after the intentional first-generation changes have been reviewed and staged or otherwise compared against a captured post-first-generation baseline. During implementation, use `git status --short` and a before/after hash or staged baseline so the second run is checked for additional changes rather than requiring the PR's intended fixture changes to disappear.

### Focused Go trace and compatibility checks

```powershell
Set-Location loomspan-console
go test ./internal/traceanalysis
go test ./internal/console -run TestScopeOpenArtifactRejectsIncompatibleTarget
```

### Focused browser tests

```powershell
Set-Location loomspan-console/web
npm test -- TraceRecords.toolInput.test.tsx activityPresentation.test.ts
npm run typecheck
```

Use the repository-pinned Node/npm versions declared in `loomspan-console/web/package.json`. The integrated build tool remains authoritative if local global versions differ.

### Full Console verification

```powershell
Set-Location loomspan-console
go test ./...
go run ./internal/buildtool verify
$env:PATH = "C:\msys64\mingw64\bin;" + $env:PATH
$env:CGO_ENABLED = "1"
go test -race ./...
```

### Static removal and diff checks

```powershell
Set-Location C:\opendev\code\loomspan
rg -n "TOOL_CALL_REQUESTED|recordToolRequested" `
  loomspan-spring-boot-starter/src `
  loomspan-console/internal `
  loomspan-console/web `
  loomspan-console-fixtures `
  ai/skill-authoring `
  ai/thoughts/phases
git diff --check
```

The search must return no matches. Historical ticket and plan documents and ignored `loomspan-console/build/` outputs are intentionally outside this active-contract audit.

## Manual Verification

1. Inspect the Java-generated planned-success fixture:
   - one tool frame;
   - one start before completion;
   - linked task and arguments preserved;
   - terminal usage reports one tool invocation.
2. Inspect the unplanned-failure fixture:
   - one start before failure;
   - `unplanned: true`, no linked task, and note preserved;
   - canonical error/final failure linkage retained;
   - no completion or inferred success.
3. Inspect generated diffs for only intended new lifecycle cases, expected inventory/count updates, and LF line endings.
4. In Trace Explorer, verify Tool input is absent until selected, then verify planned and unplanned presentations.
5. Inspect JSON, scalar/plain text, and malicious-looking arguments; confirm all render as inert text.
6. Verify keyboard activation, focus, labels, loading, and error presentation.
7. Inspect a start without terminal evidence and confirm the Console does not assign an outcome.
8. Review live activity and confirm no argument content appears.
9. Compare equivalent execution usage before and after: tool invocation counts remain unchanged while physical trace record count decreases by one per invocation.
10. Review updated skill-authoring guidance against the focused tests and fixtures; verify it distinguishes enforced lifecycle facts from limitations and does not promise historical readability.

## Exit Criteria

- [x] The failing-first unit test is added before the implementation, fails for the extra record, and passes unchanged after consolidation.
- [x] Planned and unplanned invocations each emit exactly one start with all required recorded facts.
- [x] The start is observable before router side effects.
- [x] Success retains one completion; failure retains one failure and canonical error visibility; missing terminal evidence remains unknown.
- [x] Session quota accounting, outcome metrics, terminal usage, and live-derived tool usage remain one per attempted invocation.
- [x] The journal contains exactly one sanitized tool-call entry and does not infer unplanned status from prose.
- [x] Live Java and browser projections contain no arguments.
- [x] Java-owned planned-success and unplanned-failure fixtures are deterministic, LF-normalized, and accepted by Go.
- [x] Go exposes retained lifecycle records and preserves frame, sequence, failure, and terminal facts without adjacency inference.
- [x] Existing exact release-string rejection still prevents acquisition from a mismatched target.
- [x] Tool input remains collapsed and unfetched until explicit selection.
- [x] Browser tests cover pretty JSON, scalar/plain text, planned resolution, missing title, unplanned messaging, inert rendering, pagination, invalid content, loading/errors, keyboard, and ARIA state.
- [x] Tests cited as evidence for `traces-and-debugging.md` establish every new author-facing lifecycle and sensitivity claim.
- [x] Full Java, Go, browser/buildtool, and race suites pass.
- [x] Fixture regeneration is deterministic and all intended generated changes are reviewed.
- [x] Active source, tests, fixtures, routed guidance, and phase docs contain no obsolete vocabulary.
- [x] No alias, dual writer, fallback, bridge, legacy reader, historical fixture suite, compatibility marker, public API/SPI, configuration, manifest, constructor, bean, or Spring extension point is introduced.
- [ ] Manual Trace Explorer, fixture, usage, accessibility, and documentation-evidence checks are complete.
