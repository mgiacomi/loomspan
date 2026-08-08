# Trace Catalog Entry Skill Testing Plan

## Change Summary

- Record the exact registered name of the top-level YAML skill as required `entrySkill` session metadata before the first trace record.
- Keep that identity immutable through live projection, finalization, Java REST list/detail responses, Go acquisition metadata, cached browser fallback, and React Trace Catalog/Detail views.
- Add no synthetic frame and no identity field to canonical NDJSON. Change only the current-release application REST trace fixtures.
- Render entry skill as plain text before the linked Trace ID and document its author-facing meaning.

## Impacted Areas

- Java session and invocation: `LoomspanSessionRunner`, `LoomspanSession`, `DefaultSkillTemplate`, `ExecutionCoordinator`, and their test helpers.
- Java observation and trace lifecycle: observation factories, `ExecutionProjectionState`, `LiveActivityProjector`, `ActiveExecutionSnapshot`, trace factories/handles, finalized descriptors, and the in-memory catalog.
- Java application adapter: `ObservabilityDtos.Trace`, `ObservabilityDtoMapper`, REST integration tests, and the application REST fixture generator.
- Executable fixtures: `loomspan-console-fixtures/application-rest/traces-page.json` and `trace-detail.json` only.
- Go application boundary and acquisition: `internal/observability`, `internal/artifact`, `internal/console`, and `internal/browserapi`.
- Browser contract and UI: TypeScript `Trace`, Trace Catalog, Trace Detail, component tests, and E2E response builders.
- Skill-authoring evidence: `ai/skill-authoring/traces-and-debugging.md` claims about exact top-level identity, nested-call immutability, pre-acquisition availability, and recorded-fact semantics.

## Risk Assessment

- **High:** an entry identity can be absent before the first root frame, especially for denied-before-root and standalone no-frame executions.
- **High:** Java, fixture, Go, and TypeScript shapes can drift during the intentional atomic required-field break.
- **High:** acquisition or authentication/unavailability fallback can drop the identity even when reachable responses carry it.
- **High:** root-frame projection can overwrite the supplied identity, or a nested root can replace the top-level name.
- **Medium:** normalization can trim, rewrite, split a supplementary Unicode code point, or be applied inconsistently in downstream layers.
- **Medium:** catalog idempotence can ignore an identity conflict, or list/detail can derive identity from optional live state or artifact parsing.
- **Medium:** the new table value can become a link or render application-authored markup instead of text.
- **Medium:** fixture regeneration can accidentally alter canonical NDJSON or trace-analysis expected output.
- **Low:** the added column can regress semantic header order, horizontal scrolling, keyboard focus, pagination, date formatting, or existing acquisition actions.

Compatibility scope follows the framework design lens:

- **Application API — protected:** `SkillTemplate.invoke` signatures and application caller behavior stay unchanged. Existing public-surface architecture tests remain green.
- **Supported SPI — unaffected:** no supported SPI is added or changed.
- **Configuration and manifest contracts — protected:** exact YAML `name`, lookup, and nested visibility behavior stay unchanged; no manifest migration is introduced.
- **Persisted or serialized contracts — intentional current-release break:** Java/Go/browser trace list/detail JSON requires nonempty `entrySkill`. Old and new shapes are not supported simultaneously, and transient installed entries are not migrated.
- **Ephemeral diagnostic formats — current-run coherence:** active snapshots require identity from `TRACE_STARTED`; canonical NDJSON and trace-analysis fixtures do not gain the field and need no legacy reader.
- **Internal or accidentally exposed implementation — approved removal:** entry-skill-free runner, session, factory, trace-handle, and descriptor construction paths are removed atomically without overload shims.
- **Java-to-Go boundary:** the two application REST fixtures, Go validation, acquisition copy, and browser fallback are executable coordination evidence. The existing exact `consoleCompatibilityVersion` release-string rejection remains protected; no field-level version marker is added.

## Existing Test Coverage

- `LoomspanSessionRunnerTest` covers standalone finalization, retain-on-error behavior, failure before frames, concurrency, and observation/finalization failures, but not an explicit entry identity.
- `DefaultSkillTemplateTest` and `ExecutionCoordinatorTest` cover real runner invocation, access denial, top-level/nested routing, and finalization, but do not assert exact identity propagation or mismatch handling.
- `DefaultExecutionObservationHandleTest` and `LiveActivityProjectorTest` cover first-record publication, fail-closed behavior, path/activity/usage projection, and root-derived identity. The root-derived expectation is obsolete and must be replaced with pre-seeded identity assertions.
- `ExecutionTraceHandleTest`, `InMemoryFinalizedTraceCatalogTest`, phase-one/artifact integrations, and scheduled retention tests cover descriptors, publication, equality/conflict, TTL, and artifact lifecycle without entry skill.
- `ObservabilityDtoMapperTest`, `ObservabilityRestIntegrationTest`, and `ConsoleRestFixtureCorpusTest` cover Java mapping/list/detail and executable fixture generation, but the finalized trace shape lacks `entrySkill`.
- Go `internal/observability/dto_test.go` and `service_test.go` cover fixture decoding and semantic response validation. Missing/empty trace entry identity is currently accepted.
- Go artifact/console integration tests cover acquisition, auth rejection, target scope rotation, eviction, and restart; browser API tests cover reachable and installed fallback facts. None proves identity is copied and reconstructed.
- `Traces.test.tsx` and `TraceDetail.test.tsx` cover loaded, empty, loading, error, pagination, dates, facts, acquisition, download, and availability states, but not entry identity or its escaping/link semantics.
- Existing E2E specs cover target context, live executions, artifact storage, acquisition, auth rejection, and application-content escaping; their finalized trace builders need the new required field.
- `applicationclient/client_test.go` and `console/observability_integration_test.go` already protect exact-release incompatibility rejection. They should remain unchanged and green.
- Gap: no test currently spans authoritative YAML name -> first live snapshot -> finalized catalog -> acquired metadata -> cached fallback -> UI.

## Bug Reproduction / Failing Test First

- Type: unit
- Location: `loomspan-console/internal/observability/service_test.go`, extend `TestObservabilityServiceRejectsSemanticallyInvalidSuccessResponses`
- Arrange: supply otherwise-valid trace list and trace detail success JSON, first with `entrySkill` omitted and then with `"entrySkill":""`; include valid identifiers, timestamps, outcome, availability, size, and pagination fields so identity is the only invalid property.
- Act: call `Service.ListTraces` and `Service.GetTrace` through the existing `fakeGetClient` and captured target scope.
- Assert: each call returns a scope-bound `CONSOLE_ERROR` rather than a decoded trace.
- Expected failure (pre-fix): `validateTrace` accepts both responses, so the calls return no domain error and the existing `expected CONSOLE_ERROR` assertion fails.
- Why first: it is a deterministic runtime failure against an existing internal function path, requires no new production signature to compile, and establishes the required Java-to-Go boundary before fixture and DTO changes.

After preserving this red result, add the Java REST list/detail assertion as the next boundary-level red test. Signature-driven Java tests can then be added phase-by-phase as the internal construction paths change.

## Tests to Add/Update

### 1) `normalizeRejectsBlankAndBoundsByUnicodeCodePoint`

- Type: unit
- Location: `loomspan-spring-boot-starter/src/test/java/com/lokiscale/loomspan/internal/core/EntrySkillIdentityTest.java` (new)
- What it proves: null, empty, and Unicode-blank values are rejected; values at 256 code points are unchanged; over-bound values are truncated without splitting a supplementary character; casing and nonblank surrounding characters are not trimmed or rewritten.
- Fixtures/data: ASCII and emoji strings at and around the 256-code-point boundary.
- Mocks: none.
- Contract classification: Configuration and manifest contracts.
- Compatibility expectation: protected exact YAML-name semantics, with a new internal diagnostic bound.

### 2) `sessionSuppliesOneNormalizedEntrySkillToObservationAndTraceBeforeWork`

- Type: unit
- Location: `LoomspanSessionRunnerTest.java` and focused `LoomspanSession` tests in `internal/core`
- What it proves: session creation rejects invalid identity before either factory is invoked; a valid over-bound name is normalized once and the identical value is delivered to observation and trace construction before the action runs; old entry-free overloads are absent from repository call sites.
- Fixtures/data: explicit `test.entry`, invalid blank input, and over-bound supplementary Unicode input.
- Mocks: recording observation and trace factories; no Spring context.
- Contract classification: Internal or accidentally exposed implementation.
- Compatibility expectation: approved removal of entry-free constructors/overloads; no shim test.

### 3) `defaultSkillTemplatePassesExactResolvedCapabilityName`

- Type: integration
- Location: `loomspan-spring-boot-starter/src/test/java/com/lokiscale/loomspan/internal/skillapi/DefaultSkillTemplateTest.java`
- What it proves: the unchanged Application API resolves a YAML skill and supplies `capability.name()` exactly, including case, to the runner; access denial still occurs before planning/model work.
- Fixtures/data: a registered mixed-case skill name and the existing denied invocation fixture.
- Mocks: existing resolver/runner and authorization collaborators.
- Contract classification: Application API.
- Compatibility expectation: protected `SkillTemplate.invoke` behavior and exact configuration name.

### 4) `coordinatorRequiresTopLevelRouteToMatchSessionEntrySkill`

- Type: unit
- Location: `ExecutionCoordinatorTest.java`
- What it proves: the top-level resolved route must equal the session identity; mismatch fails before execution; a nested skill/root may have a different route without changing session identity.
- Fixtures/data: `CheckDns` top-level route, a mismatched route, and a different nested route.
- Mocks: existing planner/router/model test doubles.
- Contract classification: Internal or accidentally exposed implementation.
- Compatibility expectation: current internal invariant; nested manifest routing remains protected.

### 5) `firstTraceStartedSnapshotHasEntrySkillWithoutRootFrame`

- Type: unit
- Location: `DefaultExecutionObservationHandleTest.java`
- What it proves: the active registry snapshot published for `TRACE_STARTED` already contains the exact entry skill; denied/no-frame executions do not need a synthetic `ROOT_MISSION`.
- Fixtures/data: session `session-1`, entry `CheckDns`, and a canonical `TRACE_STARTED` record with no frame.
- Mocks: existing active registry/activity store/catalog doubles.
- Contract classification: Ephemeral diagnostic formats.
- Compatibility expectation: current-run diagnostic coherence; NDJSON record shape is unchanged.

### 6) `projectorPreservesPreseededIdentityAcrossRootAndNestedFrames`

- Type: unit
- Location: `LiveActivityProjectorTest.java`, replacing the obsolete root-derived assertion in `frameVisibilityIsLimitedToSkillExecutionButAllFramesUpdatePath`
- What it proves: matching top-level root leaves the pre-seeded identity unchanged; mismatched top-level root is rejected; nested roots/skill frames update path and activity without replacing identity; existing path truncation and usage behavior remains intact.
- Fixtures/data: pre-seeded `CheckDns`, matching root, mismatching root, and nested `LookupProvider` route.
- Mocks: none.
- Contract classification: Ephemeral diagnostic formats.
- Compatibility expectation: current-run writer/projector coherence; approved removal of root-route identity discovery.

### 7) `finalizedDescriptorAndCatalogRequireEntrySkill`

- Type: unit
- Location: `ExecutionTraceHandleTest.java` and `InMemoryFinalizedTraceCatalogTest.java`
- What it proves: finalization copies the supplied identity into `FinalizedTraceArtifact` and catalog entries; equal duplicate publication remains idempotent; the same trace descriptor with a different entry identity conflicts through normal record equality; no active-state or artifact-content lookup occurs.
- Fixtures/data: equal descriptors and one identity-only conflict.
- Mocks: temporary artifact path and existing catalog clock where applicable.
- Contract classification: Persisted or serialized contracts.
- Compatibility expectation: intentional required current-release descriptor shape; no legacy descriptor constructor.

### 8) `retainedDeniedAndStandaloneTracesKeepExplicitEntrySkillWithoutFrames`

- Type: integration
- Location: `LoomspanSessionRunnerTest.java`, `ObservabilityPhaseOneIntegrationTest.java`, and/or `ObservabilityArtifactIntegrationTest.java`
- What it proves: denied-before-root and standalone no-frame actions finalize/catalog under their supplied identity according to retention policy, with no invented frame and unchanged failure visibility.
- Fixtures/data: retained error policy, explicit routes, successful and failed no-frame actions.
- Mocks: existing observation runtime and temporary trace storage.
- Contract classification: Ephemeral diagnostic formats.
- Compatibility expectation: current-run diagnostic accuracy, ordering, and failure visibility.

### 9) `listsAndGetsFinalizedTraceWithRequiredEntrySkill`

- Type: integration
- Location: `ObservabilityRestIntegrationTest.java`, extend `listsAndGetsCurrentActiveExecutionAndFinalizedTrace`
- What it proves: Java trace list and detail JSON both contain the same required camelCase `entrySkill`; active execution still exposes it; internal artifact path and catalog ordinal remain absent; exact release identity and no-store headers remain unchanged.
- Fixtures/data: publish a finalized trace with `CheckDns` and the existing live snapshot.
- Mocks: Spring `MockMvc`, temporary artifact file, existing runtime/catalog.
- Contract classification: Persisted or serialized contracts.
- Compatibility expectation: intentional atomic current-release REST break; protected endpoint/security semantics.

### 10) `applicationRestTraceFixturesContainRequiredEntrySkill`

- Type: integration / executable contract fixture
- Location: `ConsoleRestFixtureCorpusTest.java`, `loomspan-console-fixtures/application-rest/traces-page.json`, and `trace-detail.json`
- What it proves: fixture generation emits nonempty `entrySkill: "CheckDns"` in list and detail using canonical Jackson order; only these two fixture files change.
- Fixtures/data: the existing stable trace DTO fixture with `CheckDns` added.
- Mocks: deterministic fixture clock/data already used by the generator.
- Contract classification: Persisted or serialized contracts.
- Compatibility expectation: Java-to-Go current-release lockstep; no old fixture variant.

### 11) `traceFixturesDecodeEntrySkillAndInvalidResponsesRejectMissingOrEmptyValues`

- Type: unit
- Location: `loomspan-console/internal/observability/dto_test.go` and `service_test.go`
- What it proves: both committed fixtures decode `CheckDns`; list and detail reject omitted and empty identity with scope-bound `CONSOLE_ERROR`; valid responses preserve exact text.
- Fixtures/data: the two application REST fixtures plus four inline invalid response bodies.
- Mocks: existing `fakeGetClient` and target scope setup.
- Contract classification: Persisted or serialized contracts.
- Compatibility expectation: intentional required Go reader shape synchronized with Java; this includes the failing-first cases.

### 12) `acquisitionCopiesEntrySkillIntoImmutableTraceMetadata`

- Type: integration
- Location: `loomspan-console/internal/console/artifact_integration_test.go`; update `internal/artifact/helpers_test.go` and affected valid metadata literals
- What it proves: `TraceLoader` copies source identity into installed `TraceMetadata`; auth rejection, restart/lookup, target rotation, and eviction retain or dispose of it with the existing installed entry rather than creating a separate storage contract.
- Fixtures/data: source trace `CheckDns`, existing artifact server, workspace, auth-rejection, rotation, and eviction scenarios.
- Mocks: existing Java-compatible HTTP test server and filesystem-backed artifact service.
- Contract classification: Persisted or serialized contracts.
- Compatibility expectation: current transient installed metadata changes atomically; `AcquiredArtifact`, `StoredEntry`, and storage snapshot JSON remain unchanged.

### 13) `traceRoutesPreserveEntrySkillAcrossReachableAndInstalledFallbackResponses`

- Type: integration
- Location: `loomspan-console/internal/browserapi/observability_test.go`, extend `TestObservabilityRoutesReturnCanonicalDTOs` and `TestTraceRoutesFallBackToInstalledAcquisitionFacts`
- What it proves: reachable list/detail and authentication/unavailability fallback list/detail return the identical entry skill from installed metadata; availability/local facts may differ; no artifact parsing or extra path exposure occurs.
- Fixtures/data: `TraceMetadata.EntrySkill = "CheckDns"`, reachable response, then `failRequests = true`.
- Mocks: existing fixture observability client, fake artifact service, browser auth registry, and target context.
- Contract classification: Persisted or serialized contracts.
- Compatibility expectation: current-release browser boundary coherence and protected fallback semantics.

### 14) `exactReleaseMismatchStillRejectsApplicationBeforeTraceConsumption`

- Type: unit / integration regression
- Location: existing `loomspan-console/internal/applicationclient/client_test.go` and `internal/console/observability_integration_test.go::TestScopeOpenArtifactRejectsIncompatibleTarget`
- What it proves: a non-exact `consoleCompatibilityVersion` still produces incompatibility and prevents application data/artifact consumption; PR 21 does not add a trace-field compatibility marker.
- Fixtures/data: existing mismatched release strings and incompatible test server.
- Mocks: existing HTTP/client fixtures.
- Contract classification: Persisted or serialized contracts.
- Compatibility expectation: protected exact release-string gate; tests should remain unchanged and green.

### 15) `traceCatalogLeadsWithPlainTextEntrySkillAndKeepsTraceIdAsLink`

- Type: unit (React component)
- Location: `loomspan-console/web/src/observability/Traces.test.tsx`
- What it proves: `Entry skill` is the first semantic header/cell, Trace ID follows and remains the only detail link, application-authored `<img ...>`-like text creates no element/markup, and existing focusable table region, pagination, scope URL, and date formatting remain intact.
- Fixtures/data: required `entrySkill` on all trace fixtures and one markup-shaped name.
- Mocks: existing view-model hook mock and Testing Library DOM.
- Contract classification: Persisted or serialized contracts.
- Compatibility expectation: intentional required browser DTO field; protected accessible interaction behavior.

### 16) `traceDetailShowsPlainTextEntrySkillWithoutAcquisition`

- Type: unit (React component)
- Location: `loomspan-console/web/src/observability/TraceDetail.test.tsx`
- What it proves: entry skill appears near Trace ID and Session ID as a text fact before acquisition; markup-shaped text is escaped; acquisition, raw-download confirmation/focus, expiry, availability, and stale-deep-link behavior are unchanged.
- Fixtures/data: loaded trace with normal and markup-shaped entry names.
- Mocks: existing API/view-model and navigation mocks.
- Contract classification: Persisted or serialized contracts.
- Compatibility expectation: intentional required browser DTO field with protected UI actions.

### 17) `traceIdentitySurvivesCatalogDetailAcquisitionAndFallback`

- Type: e2e
- Location: representative flows in `loomspan-console/web/e2e/artifact-storage.spec.ts`; add the required field to builders in `target-context.spec.ts` and `live-executions.spec.ts`
- What it proves: entry skill is visible in catalog/detail before acquisition, remains after deliberate acquisition, and is unchanged after application authorization rejection/cached fallback; target switching and application-content escaping still work.
- Fixtures/data: finalized trace response with `CheckDns`, acquisition response, installed metadata, and auth rejection.
- Mocks: existing Playwright route handlers/test application server.
- Contract classification: Persisted or serialized contracts.
- Compatibility expectation: current-release end-to-end Java/Go/browser observable semantics.

### 18) `canonicalNdjsonCorpusRemainsUnchangedAndAnalysisStillPasses`

- Type: integration / regression audit
- Location: existing Go trace-analysis fixture corpus tests plus a git-diff audit of `loomspan-console-fixtures/traces`, `expected`, and `invalid`
- What it proves: `TRACE_STARTED` NDJSON shape, parser/processor behavior, ordering, redaction, and analysis output remain coherent without `entrySkill`; no historical reader or format version is introduced.
- Fixtures/data: existing canonical, expected, and invalid trace corpora unchanged.
- Mocks: none.
- Contract classification: Ephemeral diagnostic formats.
- Compatibility expectation: current-run diagnostic coherence, not historical schema compatibility.

### 19) `documentedEntrySkillClaimsHaveExecutableEvidence`

- Type: evidence review backed by tests 3, 5, 6, 9, 13, 15, and 16
- Location: `ai/skill-authoring/traces-and-debugging.md` references to the corresponding implementation/tests
- What it proves: the guidance's claims are backed by behavior tests: exact top-level registered name, availability from the first record and without acquisition, immutability across nested work, persistence into cached fallback, and plain-text presentation. It also preserves the caveat that a recorded name does not prove current registration or importance.
- Fixtures/data: none beyond the cited tests.
- Mocks: none.
- Contract classification: Configuration and manifest contracts.
- Compatibility expectation: protected author-facing meaning; do not add a prose-only test.

## How to Run

Use PowerShell from `C:\opendev\code\loomspan` unless a command says otherwise.

1. Preserve the red-first result before production changes:

   ```powershell
   Set-Location loomspan-console
   go test ./internal/observability -run TestObservabilityServiceRejectsSemanticallyInvalidSuccessResponses
   ```

   The new missing/empty `entrySkill` cases must fail for the expected reason, not due to malformed unrelated fixture fields.

2. Run focused Java tests during core/observation/REST work:

   ```powershell
   Set-Location C:\opendev\code\loomspan
   .\mvnw.cmd -pl loomspan-spring-boot-starter -Dtest=EntrySkillIdentityTest,LoomspanSessionRunnerTest,DefaultSkillTemplateTest,ExecutionCoordinatorTest,DefaultExecutionObservationHandleTest,LiveActivityProjectorTest,ExecutionTraceHandleTest,InMemoryFinalizedTraceCatalogTest,ObservabilityRestIntegrationTest test -DfailIfNoTests=false
   ```

3. Regenerate and then compare the application REST corpus:

   ```powershell
   .\mvnw.cmd -pl loomspan-spring-boot-starter -Dtest=ConsoleRestFixtureCorpusTest -Dloomspan.console.fixtures.regenerate=true test -DfailIfNoTests=false
   .\mvnw.cmd -pl loomspan-spring-boot-starter -Dtest=ConsoleRestFixtureCorpusTest test -DfailIfNoTests=false
   git diff --exit-code -- loomspan-console-fixtures/traces loomspan-console-fixtures/expected loomspan-console-fixtures/invalid
   ```

   Inspect `git diff --name-only -- loomspan-console-fixtures`; exactly the two application REST trace files may be modified.

4. Run focused Go and frontend tests from `loomspan-console`:

   ```powershell
   Set-Location loomspan-console
   go test ./internal/observability ./internal/artifact ./internal/browserapi ./internal/console
   npm --prefix web test -- src/observability/Traces.test.tsx src/observability/TraceDetail.test.tsx
   npm --prefix web run build
   ```

5. Run representative E2E coverage using the repository's existing Playwright setup and prerequisites:

   ```powershell
   npm --prefix web run test:e2e -- artifact-storage.spec.ts target-context.spec.ts live-executions.spec.ts
   ```

   If the repository script does not accept file arguments, run the documented full E2E script instead.

6. Run the complete verification set:

   ```powershell
   Set-Location C:\opendev\code\loomspan
   .\mvnw.cmd -pl loomspan-spring-boot-starter test -DfailIfNoTests=false
   Set-Location loomspan-console
   go test ./...
   go run ./internal/buildtool verify
   $env:PATH = "C:\msys64\mingw64\bin;" + $env:PATH
   $env:CGO_ENABLED = "1"
   go test -race ./...
   Set-Location ..
   git diff --check
   ```

No new profile, database, migration, or network service is required. Fixture regeneration uses the existing Maven property. The race suite requires the documented MSYS2 MinGW GCC path and `CGO_ENABLED=1` on Windows.

## Manual Verification

1. Invoke a top-level YAML skill that calls a nested skill. Inspect the first active snapshot and the later nested frame: both show the exact top-level registered name.
2. Exercise a denied-before-root invocation retained under error policy and a standalone no-frame action. Confirm both catalog successfully under their explicit identity and contain no synthetic root frame.
3. In Trace Catalog, confirm Entry skill is the first plain-text column, Trace ID remains the detail link, dates remain localized, and keyboard focus/scrolling work at narrow and wide widths.
4. Open Trace Detail without acquiring the artifact and confirm the entry skill is present. Acquire it, reject/disconnect application authorization, reload list/detail, and confirm the same identity is restored from installed metadata.
5. Confirm the recorded name is not linked to Skill Catalog and is not presented as proof that the skill remains registered.
6. Review the final diff for absence of a compatibility shim, synthetic frame, artifact parser, storage-table field, NDJSON field/version marker, or unrelated activity/date changes.

## Exit Criteria

- [ ] The Go semantic-validation test is captured failing pre-fix for missing and empty `entrySkill`, then passes post-fix.
- [ ] Java tests prove normalization, exact-name propagation, first-record availability, mismatch handling, nested immutability, no-frame finalization, descriptor/catalog copying, and list/detail JSON.
- [ ] Go tests prove fixture decoding, missing/empty rejection, acquisition copy, and reachable/cached fallback equality.
- [ ] React unit and E2E tests prove catalog/detail visibility before acquisition, plain-text escaping, link ownership, acquisition persistence, and fallback behavior.
- [ ] `SkillTemplate.invoke`, exact YAML naming, access denial, target scope, availability, pagination, date formatting, focus, download, retention, and catalog TTL/idempotence regressions remain green.
- [ ] Existing exact release-string incompatibility tests pass unchanged; no independent trace compatibility marker exists.
- [ ] Exactly `application-rest/traces-page.json` and `application-rest/trace-detail.json` change in the fixture corpus, both with nonempty `entrySkill`.
- [ ] Canonical NDJSON, expected trace-analysis output, and invalid corpora are unchanged; all trace-analysis tests pass.
- [ ] Entry-skill-free internal construction paths are absent, with no simultaneous old/new overload or reader behavior.
- [ ] No `entrySkill` is added to `AcquiredArtifact`, `StoredEntry`, storage snapshot JSON, canonical trace records, or session JSON.
- [ ] Tests cited by `traces-and-debugging.md` establish every new author-facing claim.
- [ ] Focused Java, Go, frontend, and E2E checks pass; full Maven, `go test ./...`, buildtool verification, race suite, and `git diff --check` pass.
- [ ] Manual nested, denied/no-frame, catalog/detail, acquisition/fallback, accessibility, and final-diff checks are complete.
