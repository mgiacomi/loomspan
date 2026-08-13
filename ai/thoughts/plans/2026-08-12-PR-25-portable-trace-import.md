# PR 25 - Save and Open Complete Same-Version Traces Implementation Plan

## Overview

Add exact-version portability for complete canonical NDJSON traces. Java will
mark every canonical trace with the authoritative
`consoleCompatibilityVersion`; Console will admit an uploaded file through its
existing bounded, capacity-accounted processor; and the browser will expose one
**Save trace file** action and one **Open trace file** workflow. The saved file
is durable, while an opened copy remains transient evidence shared by browser
and future MCP adapters.

This plan assumes PR 15 and the canonical-trace changes from PRs 21 through 24
are present before implementation begins. Rebase first and update all line
references and affected fixture expectations against that combined state.

## Current State Analysis

- Java writes one canonical UTF-8 JSON object per line and creates
  `TRACE_STARTED` metadata in
  `DefaultExecutionTraceHandle#initialize`, but the metadata currently contains
  only `tracePath` and optional `configuredLimits`
  (`loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/runtime/trace/DefaultExecutionTraceHandle.java:254-267`).
- `LoomspanReleaseVersion.load()` reads the same filtered release property used
  by the application instance endpoint, but the canonical trace writer does not
  consume it
  (`loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/observability/LoomspanReleaseVersion.java:9-49`,
  `loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/observability/web/ObservabilityRestController.java:51-65`).
- Go's artifact service already streams raw bytes into a safe staging bundle,
  accounts raw and derived bytes, invokes the required processor, atomically
  installs on success, and releases all state on failure
  (`loomspan-console/internal/artifact/acquire.go:43-222`).
- Artifact entries, leases, storage views, analysis contexts, and continuations
  are currently keyed only by `target.ScopeID`
  (`loomspan-console/internal/artifact/model.go:64-69`,
  `loomspan-console/internal/traceanalysis/dto.go:8-21`,
  `loomspan-console/internal/traceanalysis/cursor.go:37-61`). Target rotation
  invalidates all entries it owns
  (`loomspan-console/internal/artifact/target_owner.go:19-57`).
- `traceanalysis.Processor.Process` already performs the required bounded
  streaming parse, semantic validation, calculation, and immutable index and
  manifest creation. It currently consumes selected-target metadata and does
  not require a compatibility marker
  (`loomspan-console/internal/traceanalysis/processor.go:41-364`).
- The browser raw attachment is already a fresh exact upstream stream, separate
  from local admission, while the ordinary browser API has no streaming upload
  handler
  (`loomspan-console/internal/browserapi/artifact_download.go:17-100`,
  `loomspan-console/internal/browserapi/router.go:79-190`).
- Trace Detail, Trace Storage, Trace Explorer, TypeScript contracts, and all
  analysis requests currently assume one selected target scope
  (`loomspan-console/web/src/observability/TraceDetail.tsx:32-131`,
  `loomspan-console/web/src/observability/TraceStorage.tsx:16-211`,
  `loomspan-console/web/src/observability/TraceExplorer.tsx:42-107`,
  `loomspan-console/web/src/api/contracts.ts:164-258`).

## Desired End State

A complete trace produced by a released Loomspan application contains a
framework-owned `TRACE_STARTED.metadata.consoleCompatibilityVersion`. A Console
with the exact same version can open the file without a target, validate and
analyze it through the same processor used for target acquisition, and publish
one imported artifact handle under the process-local imported owner. Both
`development` values receive the same best-effort treatment; all other missing,
blank, non-string, or unequal markers fail before publication with distinct
expected and observed version details.

The browser can save a target trace through the unchanged exact upstream
download transport, open one file through a CSRF-protected raw NDJSON upload,
see `TARGET` or `IMPORTED` on all artifact and analysis results, inspect an
import without a selected target, and manage target and imported entries in one
transient Trace Storage view. Target rotation removes only target-owned
evidence. Capacity, TTL, pinning, removal, shutdown, and restart cleanup remain
shared.

Verification is complete when the Java-owned corpus carries the marker, Go
accepts only matching complete marked traces, cross-machine release import
works without a target, all rejection paths leave no installed bundle or
charge, browser and shared-service tests prove lifecycle/source behavior, and
the full Java, Go, React, Playwright, and race checks pass.

### Key Discoveries

- The durable compatibility unit is the exact complete product version, not a
  new trace-schema version; the design lens explicitly rejects historical
  readers and version ranges (`ai/thoughts/framework-feature-design-lens.md:26-49`).
- Import must return metadata derived from the validated canonical file because
  there is no authoritative application catalog response. The processor is the
  only component permitted to interpret those bytes.
- PR 18 requires owner-bound handles, discovery, queries, and continuations; it
  must not add another import catalog, cache, parser, or calculation path
  (`ai/thoughts/tickets/loomspan-console-pr-18-mcp-trace-inspection.md:9-56`).
- A hard request ceiling is required even when workspace capacity is
  `unlimited`. The approved ceiling is 4 GiB; a finite workspace uses the
  smaller of its configured maximum and 4 GiB. Derived bytes remain subject to
  aggregate capacity and can still cause atomic admission failure.

## What We're NOT Doing

- No cross-version loading, unmarked legacy reader, migration, compatibility
  range, alias, fallback, or independent trace-schema version.
- No ZIP/container, checksum, signature, encryption, compression, sidecar,
  exported analysis manifest, or derived-index export.
- No Console-owned archive, recent-files list, remembered path, restart
  adoption, background watcher, drag-and-drop, directory scan, or batch import.
- No export from an installed copy after its upstream application artifact is
  unavailable.
- No partial, repaired, quarantined, raw-only, or LLM-salvaged view of invalid
  or incomplete traces.
- No caller-supplied filesystem path or React-side canonical parsing.
- No MCP route, SDK type, tool, resource, or advertised capability. PR 18 adapts
  the shared owner/discovery/query services added here.
- No new `loomspan.*` configuration, YAML skill syntax, or application API/SPI.

## Skill-Authoring Documentation Impact

**Impact**: Affected

- **Rationale**: Skill semantics and manifest authoring do not change, but the
  routed trace-debugging guidance must explain the newly supported same-version
  save/open workflow, the exact-version and `development` limitations, and the
  sensitivity of files before sharing. Leaving the current blanket
  current-run-only wording unchanged would omit a material debugging workflow.
- **Documents to update**:
  `ai/skill-authoring/traces-and-debugging.md`.
- **Supporting evidence**: Java
  `DefaultExecutionTraceHandle` marker tests, Java/Go fixture corpus tests, Go
  compatibility/import integration tests, and browser save/open tests establish
  the behavior described by the guidance.
- **Coverage table update**: Not required. The existing **Traces and debugging**
  topic remains the correct route and remains source-verified; neither its task
  boundary nor confidence classification changes.
- **LLM-first usability**: Add a compact portability subsection near
  applicability/sensitivity. State exact required marker behavior, the
  best-effort `development` rule, transient installed-copy lifetime, and the
  absence of authenticity/provenance claims without duplicating Console UI
  instructions or serialized schema details.

## Contract and Compatibility Impact

| Surface | Classification and supporting evidence | Planned compatibility treatment |
| --- | --- | --- |
| Application API | No supported application-developer import/export entry point exists. Canonical trace implementation classes are under `com.lokiscale.loomspan.internal`; the architecture test owns the supported allowlist. | Preserve the supported API allowlist. Any technically public shared release loader remains explicitly internal and is covered by the architecture test. |
| Supported SPI | No supported SPI package or trace replacement point exists (`LoomspanPublicSurfaceArchitectureTest` asserts this). | No SPI added; change internal constructors and service interfaces atomically. |
| Configuration and manifest contracts | Existing `trace-workspace.max-bytes` and `idle-ttl` retain syntax/defaults; no skill manifest field is added. | Preserve configuration. Derive upload admission as `min(4 GiB, finite max-bytes)` without a new property. |
| Persisted or serialized contracts | A complete marked canonical NDJSON file becomes deliberately portable only to the exact same `consoleCompatibilityVersion`. Protected consumers are the current Java writer/reader, Go parser/processor, application artifact transport, and Java/Go corpus. | Add required marker production and exact matching atomically. Reject missing/unmarked and unequal files; no historical reader or compatibility shim. |
| Ephemeral diagnostic formats | Bundle layout, analysis manifest, handles, owner IDs, storage DTOs, and continuations remain process-local. They gain explicit evidence ownership/source. | Change current formats in place, bind continuations to owner plus handle, and test current-version coherence/security rather than old-format readability. |
| Internal or accidentally exposed implementation | Java release loading/trace construction, Go artifact/query interfaces, target-scoped keys, browser DTOs, routes, and React state all change. | Replace target-only assumptions with one owner model and update every in-repository caller/test atomically. |

- **Evidence of supported contracts**: The approved PR 25 ticket and design
  lens establish exact-version marked NDJSON portability. The public-surface
  architecture test establishes the Java API/SPI boundary. Console README and
  fixture corpus establish current artifact and trace behavior.
- **Intended breaks**: Headerless traces stop being valid analysis/import input;
  target-only internal owner/query/DTO/cursor shapes are replaced; current
  trace fixtures are regenerated with the required marker. These are approved
  lockstep changes, not compatibility regressions.
- **In-repository consumers to update**: Java trace construction and tests;
  observability release endpoint wiring; Go parser, processor, artifact service,
  query service, browser adapters, and console composition; TypeScript client,
  contracts, routes, components, and tests; Java/Go fixtures; Console and
  skill-authoring documentation.
- **Public-surface delta**: User-visible **Save trace file** and **Open trace
  file** workflows. No supported Java API, SPI, Spring extension point,
  configuration, or skill-manifest delta. If the shared Java release loader
  must be public for cross-package internal use, record it as technically public
  internal exposure in `LoomspanPublicSurfaceArchitectureTest`.
- **Shim decision**: **No shim.** Exact marker presence is required; do not keep
  old constructors/readers, accept unmarked files, or translate old owner or
  cursor representations.
- **Java-to-Go boundary coordination**: **Required.** The Java writer, Go
  consumed NDJSON processor, release comparison errors, all committed trace
  fixtures and expectations, cross-language corpus tests, and documentation
  must land together after PRs 21-24 are rebased.

## Implementation Approach

Use a single internal evidence-owner value with `TARGET` or `IMPORTED` source,
an opaque process-local owner ID, and an optional target scope. Artifact keys,
leases, lookups, storage entries, query contexts, and cursors carry that value.
Target scope context remains responsible for cancellation of target acquisition;
the imported owner is tied only to request and service lifetime.

Keep target acquisition and import as two transports into one installation
primitive. Target acquisition supplies expected catalog metadata and an
upstream `ArtifactStream`; import supplies an untrusted request reader and no
application metadata. Before an import can reserve capacity, a processor-owned
bounded header preflight reads at most the first permitted physical record,
validates its framing/object/start-record/identity/compatibility fields, and
returns both the identity and a replayable stream containing the exact buffered
bytes followed by the unread request. The artifact service atomically reserves
the imported `(owner, traceId)` key at that point, so capacity eviction cannot
turn a duplicate upload into implicit replacement. The normal processor then
performs the one complete semantic pass over the replayable stream, extracts
completion metadata, cross-checks supplied target metadata when present, and
returns validated metadata with authoritative component sizes. Any later
failure removes the reservation and staged copy.

The browser upload is a raw `application/x-ndjson` POST. This avoids multipart
metadata and filenames becoming another contract. Session and CSRF validation
run before the handler reads the body. `http.MaxBytesReader` and the artifact
copy loop independently enforce the effective request ceiling. The response and
all subsequent storage/analysis requests carry an explicit source; imported
requests resolve the service-owned imported owner and never capture a target.

## Phase 1: Establish the Marked Canonical Trace Contract

### Overview

Create one authoritative Java release-value seam, write the marker on every
canonical trace regardless of observability activation, require and compare it
in Go, and regenerate the executable semantic corpus.

### Changes Required

#### 1. Shared Java release metadata

**Files**:

- Move
  `loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/observability/LoomspanReleaseVersion.java`
  to
  `loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/release/LoomspanReleaseVersion.java`
- `loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/observability/web/ObservabilityRestController.java`
- `loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/runtime/trace/DefaultExecutionTraceHandle.java`
- `loomspan-spring-boot-starter/src/test/java/com/lokiscale/loomspan/architecture/LoomspanPublicSurfaceArchitectureTest.java`
- Move/update
  `loomspan-spring-boot-starter/src/test/java/com/lokiscale/loomspan/internal/observability/LoomspanReleaseVersionTest.java`
  under the matching `internal/release` test package.

**Changes**:

- Move `LoomspanReleaseVersion` to the shared `internal.release` package and make
  only the class/load method visibility required for cross-package internal use,
  so the instance response and trace runtime consume exactly one loader.
- Load the value from filtered `META-INF/loomspan-release.properties`; never
  accept it from a skill, model, application property, or trace caller.
- Put the nonblank string at
  `TRACE_STARTED.metadata.consoleCompatibilityVersion` before any record is
  written, including when application observability is disabled.
- Update the architecture allowlist explanation only if cross-package Java
  composition requires technical public visibility.

#### 2. Go compatibility validation and canonical metadata extraction

**Files**:

- `loomspan-console/internal/release/version.go`
- `loomspan-console/internal/artifact/processor.go`
- `loomspan-console/internal/traceanalysis/model.go`
- `loomspan-console/internal/traceanalysis/processor.go`
- `loomspan-console/internal/traceanalysis/errors.go`
- `loomspan-console/internal/consolecore/errors.go`
- `loomspan-console/internal/traceanalysis/manifest.go`

**Changes**:

- Construct the shared processor with Console's authoritative
  `release.ProductVersion()` rather than reading global or caller data during a
  parse.
- Expose a processor-owned bounded import-header preflight that reuses the
  physical-record decoder, preserves the exact consumed bytes for replay, and
  returns only validated start identity/version facts. It must not calculate
  trace semantics, allocate beyond the existing physical-line bound, or become
  an alternate full reader.
- Require the first `TRACE_STARTED` record to contain exactly one string marker
  after normal JSON duplicate/shape checks. Reject missing, blank, non-string,
  released/development mismatch, and unequal releases before publication.
- Add `INCOMPATIBLE_ARTIFACT` (do not reuse `INCOMPATIBLE_TARGET`) carrying
  `expectedCompatibilityVersion` and
  `observedCompatibilityVersion`. Missing/non-string markers remain
  `INVALID_ARTIFACT` because no trustworthy observed version exists.
- When both values are exactly `development`, continue through the same full
  processor. Do not enable a fallback if later validation fails.
- Return validated canonical trace/session/outcome/finalization/persistence
  metadata from `ProcessResult`, and cross-check optional target catalog
  metadata inside the same processor path. Keep the analysis manifest internal.

#### 3. Cross-language fixtures

**Files**:

- `loomspan-spring-boot-starter/src/test/java/com/lokiscale/loomspan/internal/runtime/trace/ConsoleTraceFixtureCorpusTest.java`
- `loomspan-console-fixtures/traces/*.ndjson`
- `loomspan-console-fixtures/expected/*.json`
- `loomspan-console-fixtures/README.md`
- `loomspan-console/internal/traceanalysis/fixture_corpus_test.go`
- parser/processor test helpers under `loomspan-console/internal/traceanalysis/`

**Changes**:

- Generate all valid and semantically invalid fixtures with the current
  authoritative marker after rebasing PRs 21-24.
- Add named missing, blank, non-string, and mismatched marker cases. Keep
  expected/observed details in focused Go tests; the neutral invalid corpus may
  continue recording only stable invalidity categories.
- Remove expectations that unmarked historical fixtures are accepted. Do not
  add a historical fixture suite.
- Regenerate twice and require the second pass to produce no diff.

### Success Criteria

#### Automated Verification

- [x] Java trace-handle tests prove the marker is present with observability
  enabled and disabled and cannot be caller-supplied.
- [x] Go processor tests prove exact release matching, released/development
  mismatch, matching-development best effort, missing/blank/non-string
  rejection, and no fallback after semantic failure.
- [x] Java corpus regeneration is deterministic:
  `.\mvnw.cmd -pl loomspan-spring-boot-starter test -Dtest=ConsoleTraceFixtureCorpusTest -Dloomspan.console.fixtures.regenerate=true -DfailIfNoTests=false`.
- [x] Go consumes the regenerated corpus: `go test ./internal/traceanalysis`
  from `loomspan-console`.
- [x] The Java supported API/SPI architecture tests still pass.

#### Manual Verification

- [x] Inspect representative successful, failed, chunked, and
  observability-disabled NDJSON files and confirm the first record has the
  resolved marker and no caller-derived compatibility value.
- [x] Confirm the fixture diff contains the marker and intentional PR 21-24
  rebased changes only.

---

## Phase 2: Generalize Evidence Ownership and Add Atomic Import

### Overview

Replace target-only internal ownership with a shared evidence owner, add one
process-local imported owner and discovery view, and route uploaded bytes into
the existing staging/processing/lifecycle implementation.

### Changes Required

#### 1. Evidence owner and metadata model

**Files**:

- New `loomspan-console/internal/evidence/owner.go`
- `loomspan-console/internal/artifact/model.go`
- `loomspan-console/internal/artifact/service.go`
- `loomspan-console/internal/artifact/lease.go`
- `loomspan-console/internal/artifact/capacity.go`
- `loomspan-console/internal/artifact/expiry.go`
- `loomspan-console/internal/artifact/target_owner.go`

**Changes**:

- Define closed `TARGET` and `IMPORTED` source values and an opaque non-path
  owner ID. Create exactly one random imported owner per service process.
- Replace `(target.ScopeID, traceID)` entry keys with `(owner, traceID)` and
  carry owner through lookup, use, remove, snapshots, leases, errors, and
  acquisition coordination.
- Keep target scope context only on target acquisition. Target invalidation
  cancels/removes entries with the matching target owner and leaves the imported
  owner untouched.
- Make TTL scans and storage snapshots owner-neutral. Capacity eviction already
  spans the aggregate entry map and must continue doing so.
- Make application availability and expiry optional/not-applicable for imports;
  never represent an import as an unavailable application artifact.

#### 2. Shared install primitive and import operation

**Files**:

- `loomspan-console/internal/artifact/acquire.go`
- New `loomspan-console/internal/artifact/import.go`
- `loomspan-console/internal/artifact/processor.go`
- `loomspan-console/internal/artifact/storage.go`
- `loomspan-console/internal/console/service.go`

**Changes**:

- Extract target acquisition's copy, capacity reservation, processor, sync,
  atomic rename, publish, and cleanup sequence into one internal install path.
- Add `Import(ctx, reader, declaredLength)` tied to request and service lifetime,
  not target scope. It accepts no path, filename, trace ID, version, or metadata
  from the caller.
- Enforce a 4 GiB hard raw-request ceiling. When `max-bytes` is finite, use the
  smaller value at the request and copy boundaries. Continue accounting derived
  components so total installation may fail even when raw input fits.
- Use the processor-owned header preflight to reserve the imported
  `(owner, traceId)` key before capacity reservation/eviction. If the ID is
  already installed or reserved, return `ARTIFACT_ALREADY_EXISTS` before
  staging and without altering the existing entry or charge. Derive all
  remaining immutable import metadata only from successful full processor
  output and publish against the existing reservation.
- Preserve joined acquisition for target traces. Imports are individual
  uploads; do not merge or compare their content.
- Ensure cancellation, copy failure, parser/processor failure, version
  mismatch, duplicate identity, capacity failure, sync/rename failure, and
  shutdown release every raw/derived charge and staging directory.

#### 3. Owner-bound trace analysis and continuations

**Files**:

- `loomspan-console/internal/traceanalysis/dto.go`
- `loomspan-console/internal/traceanalysis/service.go`
- `loomspan-console/internal/traceanalysis/cursor.go`
- all query implementation files under
  `loomspan-console/internal/traceanalysis/query_*.go`

**Changes**:

- Replace `TargetScopeID`-only query inputs/context with an evidence reference
  containing source, opaque owner, handle, trace ID, session ID, and optional
  target scope.
- Acquire leases by owner and handle for every query. Imported queries work
  with no selected target; target queries retain current-scope validation.
- Replace cursor `scopeId` with owner source/ID while retaining handle,
  operation, fingerprint, and position. For target owners, preserve
  `TARGET_CHANGED` precedence; for imports, removal/expiry produces
  `ARTIFACT_EXPIRED` and target rotation is irrelevant.
- Change cursor schemas in place and update tests; do not accept the old
  target-only cursor.

### Success Criteria

#### Automated Verification

- [x] Artifact unit tests cover owner keys, target-only invalidation, imported
  TTL/pinning/eviction/removal, global capacity, duplicate imported ID races,
  cancellation, shutdown, and zero leaked bytes/directories.
- [x] Import tests cover known, unknown/chunked, exactly-at-limit, one-byte-over,
  finite-workspace-smaller-than-4-GiB, and derived-capacity failure paths without
  allocating multi-gigabyte fixtures.
- [x] Trace-analysis tests cover target and imported leases, contexts, pages,
  ranges, search, and owner-bound cursor validation/error precedence.
- [x] Console integration tests prove imported evidence works without a target,
  survives target rotation, disappears on restart, and shares the same manifest
  and calculated facts as target acquisition.
- [x] `go test ./internal/artifact ./internal/traceanalysis ./internal/console`
  passes from `loomspan-console`.

#### Manual Verification

- [ ] Inspect the verified workspace during a deliberately paused import and
  confirm only opaque staging/bundle names appear and no supplied filename/path
  is retained.
- [ ] Rotate/disconnect the selected target and confirm an imported explorer
  remains usable while target-owned evidence is invalidated.

---

## Phase 3: Add the Secure Browser Save/Open Experience

### Overview

Expose the shared import and owner-aware queries through browser APIs and add
source-honest, accessible React workflows without parsing trace semantics in
the browser.

### Changes Required

#### 1. Browser API upload and source-aware contracts

**Files**:

- `loomspan-console/internal/browserapi/router.go`
- `loomspan-console/internal/browserapi/request_policy.go`
- `loomspan-console/internal/browserapi/artifacts.go`
- `loomspan-console/internal/browserapi/trace_analysis.go`
- `loomspan-console/internal/browserapi/errors.go`
- `loomspan-console/internal/browserapi/observability.go`

**Changes**:

- Add `POST /api/console/v1/artifacts/import` accepting exactly
  `application/x-ndjson` with no query string, multipart fields, range, path, or
  caller metadata. Authenticate session and validate CSRF before reading any
  body byte.
- Wrap the body with `http.MaxBytesReader` using the effective upload limit,
  accept known or chunked lengths, reject invalid/oversized lengths, stream to
  `ArtifactService.Import`, and close promptly on cancellation/error.
- Add source to acquisition, trace detail, storage, and analysis DTOs. Use an
  optional `targetScopeId` only for target evidence; never emit application
  availability, expiry, identity, or navigation claims for imports.
- Make storage/detail/remove/clear operations work without a selected target.
  Resolve imported ownership inside the service from `source: IMPORTED`; do not
  accept an owner ID from the browser.
- Extend all analysis request/response envelopes with source. Resolve TARGET
  from the captured current scope and IMPORTED from the service-owned imported
  owner before finding its handle.
- Map compatibility mismatch, duplicate import, upload-limit, invalid artifact,
  expiry/removal, and cancellation to precise browser error codes/details. Do
  not claim raw upstream download is available for an import rejection.

#### 2. TypeScript API and navigation

**Files**:

- `loomspan-console/web/src/api/contracts.ts`
- `loomspan-console/web/src/api/client.ts`
- `loomspan-console/web/src/app/routes.tsx`
- trace-analysis callers under `loomspan-console/web/src/observability/`

**Changes**:

- Add `TraceSource = "TARGET" | "IMPORTED"`, source-aware evidence references,
  optional target/application fields, import response, and new browser error
  codes.
- Add a file-body client using `fetch` with `credentials: "same-origin"`, exact
  NDJSON content type, tab/CSRF headers, no-store, and redirect rejection. Do
  not use `FileReader`, `FormData`, or parse/read the file in React.
- Pass source through every detail/analysis/range/search request and verify the
  same source in responses.
- Retain `/traces/:traceId` for target catalog detail and add a distinct
  imported route such as `/traces/imported/:traceId`; route shape is internal
  browser state, not a durable bookmark.

#### 3. Save, open, storage, and explorer presentation

**Files**:

- `loomspan-console/web/src/observability/TraceDetail.tsx`
- `loomspan-console/web/src/observability/TraceStorage.tsx`
- `loomspan-console/web/src/observability/TraceExplorer.tsx`
- `loomspan-console/web/src/observability/TraceFailureDiagnostic.tsx`
- `loomspan-console/web/src/observability/TraceRecords.tsx`
- `loomspan-console/web/src/app/App.tsx`
- `loomspan-console/web/src/styles/index.css`

**Changes**:

- Rename the existing user action to **Save trace file** while retaining its
  fresh-upstream exact-byte URL and security boundary. State before saving that
  trace files can contain sensitive diagnostics and application paths.
- Put one labeled file input and **Open trace file** action in Trace Storage (or
  the adjacent shared trace workspace surface). State the same sensitivity and
  exact-version limitation before selection/submission; show progress and
  precise actionable errors.
- On success, navigate directly to the imported route and render the existing
  Trace Explorer with an **Imported evidence** label. Do not call target detail,
  target catalog, skill-link, save, or acquisition operations for this route.
- Remove Trace Storage's target-route guard, show target and imported entries in
  one table with a Source column, key actions by `(source, traceId)`, omit
  application facts for imports, and retain common size/expiry/pin/removal
  meanings.
- Make Trace Explorer reset on owner/handle invalidation rather than every
  target generation. Imported evidence remains mounted through target rotation;
  target evidence retains current-scope verification.
- Continue rendering raw/reconstructed content as inert React text and preserve
  explicit payload/tool/diagnostic disclosure.

### Success Criteria

#### Automated Verification

- [x] Browser API tests prove host/origin/session/CSRF/content-type/size checks
  occur in the required order and unauthorized requests consume zero body bytes.
- [x] Handler tests cover fixed and chunked uploads, cancellation, all domain
  error mappings, source/optional-field JSON shapes, and no path/filename
  exposure.
- [x] React tests cover save-label continuity, sensitive-data disclosure, file
  chooser/upload, progress/error states, direct navigation, imported labels,
  no-target exploration, source-aware removal, and target-rotation survival.
- [x] TypeScript typecheck and unit tests pass: `npm run typecheck` and
  `npm test` from `loomspan-console/web`.
- [x] Playwright covers save bytes and same-version open on another Console
  process, no-target import, mismatch rejection, storage lifecycle, and target
  rotation: `npm run test:e2e` from `loomspan-console/web` after the canonical
  Console build.

#### Manual Verification

- [ ] Save a retained trace from one matching release, open it in another local
  Console process with no target, and compare summary, frames, records, usage,
  failures, payloads, gaps, and uncertainties.
- [ ] Confirm an imported page never displays target address, application
  availability/expiry, current skill links, or save-from-copy controls.
- [ ] Verify file chooser, focus transfer, live error announcements, keyboard
  operation, long IDs, and narrow viewport behavior.

---

## Phase 4: Synchronize Documentation and Complete Verification

### Overview

Document the narrow portability contract and run the full lockstep validation
after all layers are complete.

### Changes Required

#### 1. Console and fixture documentation

**Files**:

- `loomspan-console/README.md`
- `loomspan-console-fixtures/README.md`

**Changes**:

- Document save versus installed-copy behavior, raw NDJSON upload, exact
  released-version matching, best-effort matching `development`, 4 GiB hard
  request ceiling, finite workspace interaction, source labeling, duplicate
  imported trace IDs, and transient lifecycle.
- Replace statements that all artifacts are target-bound. Retain the statement
  that bundle layouts, handles, cursors, catalogs, and indexes are not durable.
- Explain that the version marker is compatibility metadata, not integrity,
  authenticity, provenance, tenant, or host evidence.

#### 2. Skill-authoring trace guidance

**File**: `ai/skill-authoring/traces-and-debugging.md`

**Changes**:

- Add a compact same-version portability subsection supported by the focused
  corpus/import tests.
- Preserve the prohibition on application dependencies on serialized trace
  shape and the warning that trace content is not secret-scanned/redacted.
- State that imports are transient Console evidence and `development` is
  best-effort. Do not turn the topic into a UI manual.

### Success Criteria

#### Automated Verification

- [ ] Full Java suite passes:
  `.\mvnw.cmd -pl loomspan-spring-boot-starter test -DfailIfNoTests=false`.
- [x] Full Console verification passes from `loomspan-console`:
  `go test ./...` and `go run ./internal/buildtool verify`.
- [x] Race suite passes with the repository-required Windows gcc/cgo setup:
  `$env:PATH = "C:\msys64\mingw64\bin;" + $env:PATH; $env:CGO_ENABLED = "1"; go test -race ./...`.
- [x] Browser end-to-end suite passes after a canonical build:
  `npm --prefix web run test:e2e` from `loomspan-console`.
- [x] Java fixture regeneration run twice leaves the second run clean.
- [x] Documentation claims are backed by the named Java, Go, browser, and E2E
  tests and satisfy the `ai/skill-authoring/README.md` LLM-first standard.

#### Manual Verification

- [x] Inspect the complete diff for obsolete `Download Trace`, target-only
  artifact ownership, and unmarked-fixture assumptions.
- [x] Confirm no legacy reader, old cursor decoder, owner alias, duplicate
  parser/cache/catalog, MCP protocol surface, or durable local import state was
  introduced.

## Testing Strategy

### Unit Tests

- Start with a Java trace-handle test that fails because
  `TRACE_STARTED.metadata.consoleCompatibilityVersion` is currently absent.
- Add focused Go processor tests for marker shape/equality and metadata
  extraction; artifact tests for owner lifecycle, upload bounds, duplicate
  identity, accounting, cancellation, and cleanup; cursor/query tests for both
  owner types; browser handler security/contract tests; and React component/API
  tests for the source-aware workflow.

### Integration Tests

- Use the Java-owned fixture corpus as the executable canonical boundary.
- Exercise target acquisition and direct import through the same real processor
  and compare their neutral facts.
- Assemble the browser router with real session/CSRF policy and artifact
  services to prove authorization-before-body and atomic publication.
- Use Playwright with two Console processes for save/open portability and a
  no-target destination.

**Note**: Full cases, fixtures, failure-first sequencing, commands, and exit
criteria are specified in
`ai/thoughts/plans/2026-08-12-PR-25-portable-trace-import-testing.md`.

### Manual Testing Steps

1. Build a resolved release, create and finalize a retained trace with
   application observability disabled, copy the file, and open it in a matching
   Console with no target.
2. Save an available target trace through Console, compare saved bytes with the
   upstream artifact, and open it in a second matching Console.
3. Attempt missing-marker, mismatched-release, released/development, truncated,
   malformed, oversized, contradictory, and duplicate-ID imports; confirm no
   new row, handle, bundle, or charge remains.
4. Pin/use an imported trace, exercise removal/expiry behavior, rotate the
   target, close/restart Console, and verify the specified lifetime boundaries.

## Performance Considerations

- Upload and processor paths retain bounded buffers: a 32 KiB copy buffer, one
  bounded physical line, bounded JSON depth, and disk-backed payload/index data.
- Never buffer the upload, whole raw artifact, reconstructed payloads, or
  derived indexes in HTTP or React memory.
- Enforce the 4 GiB request limit without allocating a limit-sized fixture;
  tests use declared lengths and small injected limits/readers.
- Imported and target bundles share one capacity and LRU/TTL policy; no second
  cache or background scan is introduced.
- The duplicate imported-ID check must be atomic under concurrent uploads and
  must not replace or refresh the existing entry.

## Migration Notes

There is no migration. Previously produced unmarked files are intentionally not
importable, existing transient bundles/cursors are not adopted across restart,
and all Java, Go, TypeScript, tests, fixtures, and documentation change in one
repository update. Rebuild the application and Console at the same resolved
version and remove stale `development` trace files after canonical changes.

## References

- Original ticket:
  `ai/thoughts/tickets/loomspan-console-pr-25-portable-trace-import.md`
- Research:
  `ai/thoughts/research/2026-08-12-PR-25-portable-trace-import.md`
- Framework design lens:
  `ai/thoughts/framework-feature-design-lens.md`
- Future MCP consumer:
  `ai/thoughts/tickets/loomspan-console-pr-18-mcp-trace-inspection.md`
- Canonical prerequisite tickets:
  `ai/thoughts/tickets/loomspan-console-pr-21-trace-catalog-entry-skill.md`,
  `loomspan-console-pr-22-failed-trace-diagnostics.md`,
  `loomspan-console-pr-23-provider-retries.md`, and
  `loomspan-console-pr-24-consolidate-tool-call-lifecycle.md`
- Existing admission path:
  `loomspan-console/internal/artifact/acquire.go:43-299`
- Existing processor:
  `loomspan-console/internal/traceanalysis/processor.go:41-364`
