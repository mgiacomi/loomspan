# PR 25 - Save and Open Complete Same-Version Traces Testing Plan

## Change Summary

- Add required `TRACE_STARTED.metadata.consoleCompatibilityVersion` production
  in Java and exact-value validation in Go.
- Add one process-local imported evidence owner alongside target owners and
  generalize artifact, lease, storage, query, and continuation behavior to use
  explicit `TARGET` or `IMPORTED` ownership.
- Stream a raw NDJSON browser upload through the existing capacity-accounted,
  atomic artifact processor with a 4 GiB hard request ceiling, further limited
  by finite `trace-workspace.max-bytes`.
- Rename the exact upstream download action to **Save trace file**, add **Open
  trace file**, and allow imported Trace Explorer/Storage use without a target.
- Update current-version fixtures and trace/debugging documentation without
  preserving unmarked traces or old target-only internal formats.

## Impacted Areas

- Java release metadata and canonical writer:
  `LoomspanReleaseVersion`, `DefaultExecutionTraceHandle`, trace-handle factories,
  architecture tests, and `ConsoleTraceFixtureCorpusTest`.
- Persisted current-version boundary: all canonical NDJSON fixtures and their
  Go semantic expectations under `loomspan-console-fixtures/`.
- Go parsing and admission: `internal/traceanalysis` processor/parser/errors,
  `internal/artifact` owner/model/acquire/import/capacity/expiry/lease/service,
  `internal/release`, `internal/consolecore`, and `internal/console` composition.
- Go browser adapter: router security order, raw upload, artifact storage/detail
  and mutation routes, analysis routes, error mapping, and DTO contracts.
- Browser application: TypeScript contracts/client, routes, Trace Detail,
  Trace Storage, Trace Explorer and evidence-detail components, styling, and
  related unit/E2E fixtures.
- Documentation: `loomspan-console/README.md`,
  `loomspan-console-fixtures/README.md`, and
  `ai/skill-authoring/traces-and-debugging.md`.

## Risk Assessment

- **High — cross-language compatibility marker**: a writer/reader field-name,
  source-value, or error-detail mismatch could reject all current traces or
  accidentally admit unmarked/mismatched files.
- **High — atomic import and accounting**: cancellation, oversized content,
  invalid semantics, duplicate IDs, storage failures, or processor failures
  could leak staging directories, handles, goroutines, or raw/derived charges.
- **High — ownership isolation**: target rotation might delete imports, or an
  imported request might accidentally resolve the current target and inherit
  application identity/authority.
- **High — request security/order**: an unauthenticated or CSRF-invalid request
  must be rejected before reading an untrusted, potentially huge body.
- **High — concurrency**: simultaneous imports of the same trace ID must yield
  exactly one installed entry; the rejected upload must not replace, refresh,
  or remove the winner.
- **Medium — upload limits**: fixed and chunked requests must enforce
  `min(4 GiB, finite workspace capacity)` without integer overflow. Derived
  bytes can push an otherwise acceptable raw file over aggregate capacity.
- **Medium — source-aware cursors**: imported cursors must survive target
  rotation, while target cursors retain `TARGET_CHANGED` precedence and all
  cursors remain bound to owner, handle, operation, query fingerprint, and
  installed lifetime.
- **Medium — browser state**: target generation changes currently reset Trace
  Explorer and Trace Storage; source-aware state could cause stale target data
  or incorrectly reset imported exploration.
- **Medium — metadata honesty**: imports must not emit target scope,
  application availability/expiry, application identity, current skill links,
  or save-from-upstream claims.
- **Medium — sensitivity/accessibility**: save/open disclosures and explicit
  diagnostic/payload loading must remain intact, inert, keyboard reachable, and
  announced on errors.
- **Protected compatibility path**: marked complete canonical NDJSON is
  protected for the exact same resolved `consoleCompatibilityVersion`; Java
  writer, Go reader, application transport, and fixture corpus must agree.
- **Approved obsolete paths**: unmarked trace acceptance, target-only owner/DTO/
  cursor shapes, and the **Download Trace** product label are removed. Tests
  must not require simultaneous old/new behavior.
- **No protected API/SPI/configuration break**: supported Java API/SPI allowlists
  and existing Console configuration syntax/defaults remain unchanged.
- **Authoring evidence**: tests must establish exact-version portability,
  best-effort matching `development`, transient installed lifetime, and
  sensitivity boundaries described in the updated trace-debugging guide.

## Existing Test Coverage

- `ExecutionTraceHandleTest`, `NdjsonTraceRecordWriterTest`, and
  `NdjsonExecutionTraceReaderTest` protect Java trace creation, framing,
  reading, finalization, and retention, but do not assert a compatibility
  marker.
- `LoomspanReleaseVersionTest` and `ObservabilityRestIntegrationTest` protect
  the authoritative Java release value and instance response, but do not prove
  the writer uses the same value.
- `ConsoleTraceFixtureCorpusTest` byte-compares the complete Java-owned fixture
  inventory and creates named invalid traces. Current fixtures are unmarked.
- Go `parser_test.go`, `processor_test.go`, and `fixture_corpus_test.go` cover
  bounded physical parsing, semantic validation, calculations, bundle writing,
  and every Java fixture. They currently accept traces without a version marker.
- Artifact service tests cover joined target acquisition, capacity, cancellation,
  storage failure, pinning, expiry, target invalidation, shutdown, and cleanup.
  They do not cover a target-independent owner or request-reader import.
- Trace-analysis service/cursor tests cover target-bound leases, queries,
  ranges, search, continuations, and error precedence. They do not cover owner
  source or no-target queries.
- Browser API tests cover session/CSRF policy for JSON mutations, exact raw
  download bytes/headers, target-scoped storage, artifact error mappings, and
  target-scoped analysis. They do not cover streaming request bodies.
- React tests cover target Trace Detail acquisition/explorer transitions and
  target Trace Storage actions. Playwright covers artifact storage and
  multi-megabyte range reads, but not cross-process save/open.
- Coverage gaps are therefore the required marker, direct import transport,
  imported ownership/discovery/lifecycle, source-honest DTOs, no-target browser
  operation, upload limit/security order, duplicate imports, and PR 18-ready
  owner-bound continuations.

## Bug Reproduction / Failing Test First

- **Type**: Java unit test.
- **Location**:
  `loomspan-spring-boot-starter/src/test/java/com/lokiscale/loomspan/internal/runtime/trace/ExecutionTraceHandleTest.java`.
- **Name**:
  `writesAuthoritativeConsoleCompatibilityVersionOnTraceStarted`.
- **Arrange**: Create a production `DefaultExecutionTraceHandle` using a
  deterministic clock/path and the same release loader used by the instance
  response.
- **Act**: Read the first canonical record after initialization.
- **Assert**: The record is `TRACE_STARTED` and contains a nonblank string
  `metadata.consoleCompatibilityVersion` exactly equal to
  `LoomspanReleaseVersion.load()`.
- **Expected failure (pre-fix)**: The metadata field is absent.

Add the corresponding Go failure immediately afterward:

- **Type**: Go unit test.
- **Location**: `loomspan-console/internal/traceanalysis/processor_test.go`.
- **Name**: `TestProcessorRejectsTraceWithoutCompatibilityMarker`.
- **Expected failure (pre-fix)**: The current processor accepts the existing
  minimal unmarked complete trace instead of returning `INVALID_ARTIFACT` and
  writing no publishable bundle.

These two tests establish both sides of the behavior gap before broader owner
and browser work begins.

## Tests to Add/Update

### 1. Java writer uses one authoritative marker

- **Type**: Unit and integration.
- **Location**:
  `ExecutionTraceHandleTest.java`, `LoomspanReleaseVersionTest.java`,
  `ObservabilityRestIntegrationTest.java`, and focused observability-disabled
  trace lifecycle tests.
- **What it proves**:
  - every canonical `TRACE_STARTED` has exactly one nonblank string marker;
  - the trace and instance response values are identical;
  - observability disabled/no-op observation still writes the marker;
  - a caller/model/skill cannot override the marker;
  - unresolved/blank/multiple release resources fail at the existing
    authoritative loader boundary.
- **Fixtures/data**: Filtered release resource and temporary trace paths.
- **Mocks**: Deterministic clock/ID suppliers only; use production writer and
  release loader.
- **Contract classification**: Persisted or serialized contracts.
- **Compatibility expectation**: Protected exact-version marked trace path.

### 2. Java public API/SPI boundary remains closed

- **Type**: Architecture test.
- **Location**:
  `loomspan-spring-boot-starter/src/test/java/com/lokiscale/loomspan/architecture/LoomspanPublicSurfaceArchitectureTest.java`.
- **What it proves**: No supported API/SPI or Spring replacement point was
  introduced; any technically public shared release loader is explicitly
  classified internal and no obsolete constructor overload remains.
- **Fixtures/data**: Compiled production classes.
- **Mocks**: None.
- **Contract classification**: Application API and Supported SPI.
- **Compatibility expectation**: Protected allowlist; approved atomic internal
  signature changes.

### 3. Java-owned corpus carries the current marker deterministically

- **Type**: Cross-language fixture generation test.
- **Location**:
  `ConsoleTraceFixtureCorpusTest.java` and
  `loomspan-console-fixtures/traces/*.ndjson`.
- **What it proves**: Every valid/current invalid mutation starts from a marked
  current trace; named invalid cases cover missing, blank, non-string, and
  unequal markers; PR 21-24 trace semantics remain present; regeneration is
  byte deterministic.
- **Fixtures/data**: Complete committed corpus and expected JSON facts.
- **Mocks**: Existing deterministic clock/IDs/runtime adapters.
- **Contract classification**: Persisted or serialized contracts.
- **Compatibility expectation**: Protected current exact-version corpus;
  unmarked old corpus intentionally removed.

### 4. Go compatibility comparison and metadata extraction

- **Type**: Unit.
- **Location**:
  `loomspan-console/internal/traceanalysis/processor_test.go`,
  `parser_test.go`, and `fixture_corpus_test.go`.
- **What it proves**:
  - exact released values pass;
  - unequal releases and release/development pairs return the trace-specific
  `INCOMPATIBLE_ARTIFACT` with distinct expected/observed values;
  - matching `development` runs the complete normal processor;
  - a stale development artifact failing later semantics gets no fallback;
  - missing, blank, null, numeric, object, duplicate, or malformed markers are
    invalid artifacts;
  - canonical trace/session/completion metadata is returned only after full
    validation and optional target metadata is cross-checked;
  - bounded header preflight rejects an invalid/non-start/mismatched first
    record, returns validated identity plus byte-exact replay, and does not act
    as an alternate full semantic reader;
  - no components are publishable on rejection.
- **Fixtures/data**: Small marked minimal traces plus the regenerated Java
  corpus.
- **Mocks**: Fake component sink with authoritative byte accounting.
- **Contract classification**: Persisted or serialized contracts.
- **Compatibility expectation**: Protected exact-match path and approved
  rejection of unmarked/unequal traces.

### 5. Evidence owner value and key isolation

- **Type**: Unit.
- **Location**: New focused owner tests and
  `loomspan-console/internal/artifact/service_test.go`.
- **What it proves**:
  - only `TARGET` and `IMPORTED` values are constructible/accepted;
  - owner IDs are opaque, nonblank, process-local, and never paths;
  - identical trace IDs under a target owner and imported owner are distinct;
  - target rotation invalidates only its target owner;
  - imported ownership remains valid without/currently across target changes;
  - service restart creates a new imported owner and adopts no prior entries.
- **Fixtures/data**: Deterministic entropy and two target scopes.
- **Mocks**: Fake clock/timers/filesystem following existing artifact helpers.
- **Contract classification**: Internal or accidentally exposed implementation.
- **Compatibility expectation**: Approved atomic replacement of target-only
  internal ownership.

### 6. Imported lifecycle matches shared artifact semantics

- **Type**: Unit.
- **Location**:
  `loomspan-console/internal/artifact/import_test.go`, `capacity_test.go`,
  `expiry_test.go`, `lease_test.go`, and `service_test.go`.
- **What it proves**:
  - successful import publishes raw plus derived bundle and opaque handle once;
  - lease close refreshes idle time; active pins prevent eviction/removal;
  - expiry, LRU eviction, explicit removal, clear operations, shutdown, and
    restart cleanup apply to imports;
  - aggregate capacity is shared across target and imported entries;
  - application availability/expiry are absent for imports;
  - no target context/credential/client is consulted.
- **Fixtures/data**: Small complete matching trace and injected clock/timer.
- **Mocks**: Existing in-memory/failing filesystem and processor doubles; add
  assertions that target loaders/openers receive zero calls.
- **Contract classification**: Ephemeral diagnostic formats.
- **Compatibility expectation**: Current-run lifecycle coherence shared by both
  sources.

### 7. Upload/request and capacity bounds

- **Type**: Unit and HTTP integration.
- **Location**:
  `internal/artifact/import_test.go` and
  `internal/browserapi/artifacts_test.go` (or a focused
  `artifact_import_test.go`).
- **What it proves**:
  - effective raw ceiling is 4 GiB for unlimited workspaces and the smaller
    configured value for finite workspaces;
  - known length at limit is admitted and one byte over is rejected before
    body consumption/staging;
  - unknown/chunked length stops exactly at limit plus one and cleans up;
  - integer arithmetic cannot overflow near `math.MaxInt64`;
  - derived writes remain aggregate-capacity-accounted and can atomically reject
    an otherwise allowed raw file;
  - copy uses a fixed buffer and does not allocate content-sized memory.
- **Fixtures/data**: Small counting/synthetic readers and injected small limits;
  never allocate or write a 4 GiB test file.
- **Mocks**: Counting reader, short reader, cancellation reader, failing writer,
  and fake processor/component sink.
- **Contract classification**: Configuration and manifest contracts plus
  ephemeral diagnostic formats.
- **Compatibility expectation**: Existing `max-bytes` semantics protected; new
  fixed request bound enforced without configuration change.

### 8. Atomic cleanup on every import failure

- **Type**: Unit and integration.
- **Location**:
  `internal/artifact/import_test.go`, `storage_test.go`, and
  `internal/console/artifact_integration_test.go`.
- **What it proves**: Cancellation, interrupted read, short/failed write,
  raw-limit rejection, invalid JSON, invalid semantic trace, missing completion,
  version mismatch, derived-capacity failure, sync/close failure, rename failure,
  duplicate ID, and shutdown leave no entry, handle, installed/staged directory,
  capacity charge, active timer, or goroutine.
- **Fixtures/data**: Existing valid corpus plus minimal invalid/mismatch traces.
- **Mocks**: Failure-injecting filesystem/readers and leak-sensitive processor
  helpers.
- **Contract classification**: Ephemeral diagnostic formats.
- **Compatibility expectation**: Current-run atomic admission and security
  coherence.

### 9. Duplicate imported trace identity, including races

- **Type**: Unit and race/integration.
- **Location**:
  `internal/artifact/import_test.go` and
  `internal/console/artifact_integration_test.go`.
- **What it proves**:
  - a second installed import with the same trace ID is rejected until removal;
  - different bytes/metadata do not replace or merge the first;
  - concurrent same-ID uploads publish exactly one winner;
  - the imported key is reserved after bounded header preflight and before any
    capacity eviction, so the new upload cannot evict and replace its duplicate;
  - the loser releases all charges and cannot remove/refresh the winner;
  - after explicit removal, the ID can be imported again with a new handle.
- **Fixtures/data**: Same identity with equal and deliberately differing valid
  contents.
- **Mocks**: Barriers around processor completion/publication for deterministic
  race ordering.
- **Contract classification**: Ephemeral diagnostic formats.
- **Compatibility expectation**: Current-run imported owner uniqueness.

### 10. Owner-bound leases, queries, and continuations

- **Type**: Unit and service integration.
- **Location**:
  `internal/traceanalysis/service_test.go`, `cursor_test.go`,
  `continuation_test.go`, range/search/query tests.
- **What it proves**:
  - every summary/page/range/search result carries the correct source/owner and
    optional target scope;
  - target and imported artifacts produce identical calculated facts;
  - imported queries work without a target and survive target rotation;
  - target owner mismatch reports `TARGET_CHANGED` before handle/cursor errors;
  - imported removal/expiry reports `ARTIFACT_EXPIRED`, never
    `TARGET_CHANGED`;
  - cursors reject changed owner, handle, operation, filters, order, or position;
  - old scope-only cursor schema is rejected rather than translated.
- **Fixtures/data**: One fixture installed under both owner sources and existing
  paginated/range datasets.
- **Mocks**: Real artifact/analysis services with deterministic owner IDs.
- **Contract classification**: Ephemeral diagnostic formats.
- **Compatibility expectation**: Current-run query coherence; approved old
  cursor removal.

### 11. Browser authorization-before-body and upload framing

- **Type**: HTTP integration.
- **Location**:
  `loomspan-console/internal/browserapi/artifact_import_test.go` and
  `security_integration_test.go`.
- **What it proves**:
  - wrong host, origin, method, session, tab, or CSRF is rejected before one
    body byte is read or an import service call occurs;
  - only the exact raw NDJSON media type is accepted;
  - queries, multipart, invalid lengths, and oversize requests are rejected;
  - valid fixed-length and chunked bodies stream once and cancellation reaches
    the artifact service;
  - response headers remain no-store and security headers remain present.
- **Fixtures/data**: Counting/blocking request bodies and real paired session
  state.
- **Mocks**: Spy `ArtifactService` plus assembled router policy.
- **Contract classification**: Internal or accidentally exposed implementation.
- **Compatibility expectation**: Existing browser security boundary protected
  and extended to streaming bodies.

### 12. Browser source and error DTO contracts

- **Type**: Unit/contract.
- **Location**:
  `internal/browserapi/artifacts_test.go`, `trace_analysis_test.go`,
  `contracts_test.go`, `errors_test.go`, and TypeScript client tests.
- **What it proves**:
  - TARGET responses include source and current `targetScopeId`;
  - IMPORTED responses include source but omit target/application fields;
  - browser cannot supply an imported owner ID or path;
  - `INCOMPATIBLE_ARTIFACT` exposes expected/observed versions;
  - missing marker/invalid content does not say upstream raw download remains
    available;
  - `ARTIFACT_ALREADY_EXISTS`, limit, cancellation, expiry, and removal map to
    distinct stable browser errors;
  - every analysis request and response verifies source.
- **Fixtures/data**: Golden response fragments for both sources and domain error
  table cases.
- **Mocks**: Artifact/analysis service spies.
- **Contract classification**: Ephemeral diagnostic formats.
- **Compatibility expectation**: Current-run adapter coherence; target-only DTO
  shapes intentionally replaced.

### 13. Save action remains an exact upstream attachment

- **Type**: Go HTTP integration and React unit.
- **Location**:
  `artifact_download_test.go`, `TraceDetail.test.tsx`, and TypeScript client
  tests.
- **What it proves**:
  - label becomes **Save trace file**;
  - URL, exact bytes, content type, declared length, filename sanitization,
    navigation security, fresh upstream request, and local-cache bypass remain
    unchanged;
  - action is unavailable after upstream loss even if an installed copy remains;
  - sensitivity/path disclosure appears before saving.
- **Fixtures/data**: Existing raw download fixtures plus hostile filename/content
  cases.
- **Mocks**: Existing application test server and browser API mocks.
- **Contract classification**: Persisted or serialized contracts.
- **Compatibility expectation**: Protected exact-byte save transport; product
  label intentionally replaces the old wording.

### 14. Open-file React workflow

- **Type**: React component and client unit.
- **Location**:
  `TraceStorage.test.tsx`, new focused client upload tests, and route tests.
- **What it proves**:
  - one labeled file chooser and **Open trace file** action exist;
  - no file read/parse occurs in React and the `File` is sent as the raw body;
  - tab/CSRF credentials, exact content type, no-store, and redirect policy are
    set;
  - progress/disabled state prevents duplicate submission;
  - successful import navigates directly to the imported route;
  - mismatch, invalid, duplicate, limit, and cancellation errors are announced;
  - disclosure states sensitive diagnostics/paths and exact-version limitation.
- **Fixtures/data**: Small browser `File` objects whose text method throws if
  called, plus API responses/errors.
- **Mocks**: Mock `fetch`, browser session, and router navigation.
- **Contract classification**: Internal or accidentally exposed implementation.
- **Compatibility expectation**: New workflow; no legacy upload behavior.

### 15. Trace Storage presents both sources honestly

- **Type**: React component and Go handler integration.
- **Location**:
  `TraceStorage.test.tsx` and `internal/browserapi/artifacts_test.go`.
- **What it proves**:
  - storage loads with no target selected;
  - TARGET and IMPORTED rows share size, expiry, pin, and removal meanings;
  - Source is visible and row/action identity is `(source, traceId)`;
  - imported rows omit application availability/expiry rather than showing
    false/unavailable claims;
  - clear expired/all-unused covers both sources;
  - target rotation removes target rows and retains imported rows.
- **Fixtures/data**: Mixed-source snapshot with duplicate trace IDs across
  owners, active pins, and finite/never expiry.
- **Mocks**: Browser session and source-aware API client.
- **Contract classification**: Ephemeral diagnostic formats.
- **Compatibility expectation**: Current-run shared storage coherence.

### 16. Imported Trace Explorer works without target authority

- **Type**: React component/integration.
- **Location**:
  `TraceExplorer.test.tsx`, `TraceFailureDiagnostic.test.tsx`, relevant
  `TraceRecords.*.test.tsx`, and route tests.
- **What it proves**:
  - imported route displays **Imported evidence** and loads every explorer view;
  - source is included and verified for summary, frames, records, attempts,
    retries, failures, diagnostics, payloads, ranges, search, gaps, and
    uncertainties;
  - no target is required and target rotation does not reset the imported
    explorer;
  - imported records do not link recorded skills to the current target catalog;
  - expiry/removal exits cleanly with an artifact-unavailable state;
  - raw, payload, tool, and diagnostic content remains inert and explicitly
    loaded.
- **Fixtures/data**: Existing rich explorer fixtures wrapped in both source
  contexts.
- **Mocks**: Source-aware API functions and target generation changes.
- **Contract classification**: Ephemeral diagnostic formats.
- **Compatibility expectation**: Current-run diagnostic usefulness and
  security coherence.

### 17. End-to-end same-release portability and lifecycle

- **Type**: Playwright E2E.
- **Location**:
  `loomspan-console/web/e2e/artifact-storage.spec.ts` or a focused
  `portable-trace-import.spec.ts`.
- **What it proves**:
  - save bytes equal the upstream canonical artifact;
  - another matching Console process opens the file with no target and shows
    the same calculated evidence;
  - mismatch and damaged/incomplete files leave storage/charge unchanged;
  - imported evidence survives target selection/rotation but expires, removes,
    shuts down, and disappears after restart;
  - duplicate import is rejected until removal;
  - sensitive-data disclosures are visible before both workflows.
- **Fixtures/data**: Java-produced marked complete trace, mismatched-marker copy,
  missing-marker copy, truncated copy, and multi-megabyte valid trace.
- **Mocks**: Real local Console processes and existing deterministic application
  test server; no external services.
- **Contract classification**: Persisted or serialized contracts plus
  ephemeral diagnostic formats.
- **Compatibility expectation**: Protected exact-version file portability and
  transient installed-copy lifetime.

### 18. Documentation claims remain evidence-backed

- **Type**: Documentation review backed by focused automated tests.
- **Location**:
  `loomspan-console/README.md`, `loomspan-console-fixtures/README.md`, and
  `ai/skill-authoring/traces-and-debugging.md`.
- **What it proves**: The guide's exact-match, matching-development,
  transient-lifetime, sensitive-content, and no-provenance statements match
  executable behavior. No test parses prose; the underlying tests above are the
  cited evidence.
- **Fixtures/data**: Named Java/Go/browser test references.
- **Mocks**: None.
- **Contract classification**: Configuration and manifest contracts plus
  persisted/serialized diagnostic guidance.
- **Compatibility expectation**: Existing authoring semantics preserved; new
  debugging workflow accurately documented.

## How to Run

Run focused failures before production changes:

```powershell
.\mvnw.cmd -pl loomspan-spring-boot-starter test -Dtest=ExecutionTraceHandleTest -DfailIfNoTests=false
Set-Location loomspan-console
go test ./internal/traceanalysis -run 'Compatibility|Marker'
```

Regenerate and verify the Java-owned corpus after the writer and Go processor
move together:

```powershell
.\mvnw.cmd -pl loomspan-spring-boot-starter test -Dtest=ConsoleTraceFixtureCorpusTest -Dloomspan.console.fixtures.regenerate=true -DfailIfNoTests=false
.\mvnw.cmd -pl loomspan-spring-boot-starter test -Dtest=ConsoleTraceFixtureCorpusTest -Dloomspan.console.fixtures.regenerate=true -DfailIfNoTests=false
Set-Location loomspan-console
go test ./internal/traceanalysis
```

Run focused Go and browser tests while implementing:

```powershell
Set-Location loomspan-console
go test ./internal/artifact ./internal/traceanalysis ./internal/browserapi ./internal/console
Set-Location web
npm run typecheck
npm test
```

Run the full repository verification before completion:

```powershell
Set-Location C:\opendev\code\loomspan
.\mvnw.cmd -pl loomspan-spring-boot-starter test -DfailIfNoTests=false
Set-Location loomspan-console
go test ./...
go run ./internal/buildtool verify
npm --prefix web run test:e2e
$env:PATH = "C:\msys64\mingw64\bin;" + $env:PATH
$env:CGO_ENABLED = "1"
go test -race ./...
```

No external credentials or live providers are required. Use the canonical
build tool before Playwright so the executable and embedded browser assets have
the same resolved release version.

## Exit Criteria

- [ ] The Java failing test and Go missing-marker test fail on the pre-change
  implementation for the expected reasons.
- [ ] All focused and full tests pass after implementation.
- [ ] Every Java-generated canonical trace carries the authoritative marker,
  and Java/Go fixture regeneration is deterministic with a clean second run.
- [ ] Exact matching releases pass; unequal releases, release/development
  pairs, and missing/blank/non-string markers fail atomically with the required
  error details.
- [ ] Matching `development` receives normal full validation with no fallback
  reader or repair path.
- [ ] Imports never read caller paths/metadata, never parse in React, and never
  claim target/application identity, availability, expiry, or provenance.
- [ ] Unauthorized, wrong-origin, and CSRF-invalid uploads read zero body bytes.
- [ ] The effective upload limit is `min(4 GiB, finite max-bytes)` and is
  independently enforced by HTTP and artifact-copy boundaries without overflow
  or large test allocations.
- [ ] Every failure path leaves no installed entry, handle, staging directory,
  derived component, timer, goroutine, or capacity charge.
- [ ] Concurrent duplicate imported IDs publish exactly one winner and do not
  alter the existing installed entry.
- [ ] Target rotation removes target evidence and leaves imported evidence;
  imported evidence still follows shared TTL, LRU, pin, removal, clear,
  shutdown, and restart-cleanup rules.
- [ ] All queries and continuations bind source/owner/handle/operation/query
  meaning/lifetime; target and imported error precedence is correct.
- [ ] Save remains an exact fresh-upstream stream and cannot export an installed
  copy after upstream loss.
- [ ] Browser tests cover no-target import, direct imported navigation, mixed
  storage, sensitivity disclosures, inert rendering, accessibility, and
  source-aware error recovery.
- [ ] The tests cited for `ai/skill-authoring/traces-and-debugging.md` establish
  every new author-facing debugging claim, and the updated guidance satisfies
  the LLM-first standard.
- [ ] Supported API/SPI and configuration boundary tests pass; approved
  unmarked traces, old target-only cursor/DTO forms, and the old download label
  are absent rather than retained behind compatibility behavior.
- [ ] Manual cross-process save/open, mismatch/invalid rejection, target
  rotation, lifecycle, no-target identity honesty, keyboard/focus, and narrow
  viewport checks are complete.
