---
date: 2026-08-12T18:29:12-07:00
researcher: Unknown
git_commit: 324db6b0096b7b099610afd7eb878611b958f622
branch: main
repository: loomspan
topic: "PR 25 - Save and Open Complete Same-Version Traces"
tags: [research, codebase, portable-traces, artifact-service, trace-analysis, browser-console]
status: complete
last_updated: 2026-08-12
last_updated_by: Unknown
---

# Research: PR 25 - Save and Open Complete Same-Version Traces

**Date**: 2026-08-12T18:29:12-07:00
**Researcher**: Unknown
**Model**: GPT-5
**Git Commit**: 324db6b0096b7b099610afd7eb878611b958f622
**Branch**: main
**Repository**: loomspan

## Research Question

Research the current Loomspan codebase for the implementation-ready ticket
`ai/thoughts/tickets/loomspan-console-pr-25-portable-trace-import.md`: document
the existing Java canonical-trace writer and release marker, Go artifact and
analysis ownership, browser save/storage/explorer workflows, fixtures, tests,
configuration, documentation, and compatibility surfaces that the ticket
crosses.

## Summary

The repository currently has one canonical NDJSON production path in Java and
one central, transport-neutral admission and analysis path in Go. Java's
`DefaultExecutionTraceHandle` creates `TRACE_STARTED`, writes each `TraceRecord`
as one UTF-8 JSON line, and appends one terminal `TRACE_COMPLETED` during
finalization. Its start metadata currently contains `tracePath` and optional
`configuredLimits`; it does not contain `consoleCompatibilityVersion`
(`loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/runtime/trace/DefaultExecutionTraceHandle.java:254-267`). The committed valid and invalid NDJSON fixtures therefore are currently
unmarked (`loomspan-console-fixtures/traces/single-attempt-success.ndjson:1`).

Java already has an authoritative release value. Maven filters the root project
version into `META-INF/loomspan-release.properties`, and
`LoomspanReleaseVersion.load()` requires exactly one resolved, nonblank value.
The observability instance controller loads that value and returns it as
`consoleCompatibilityVersion` (`loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/observability/LoomspanReleaseVersion.java:9-49`,
`loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/observability/web/ObservabilityRestController.java:51-65`). The trace writer does not currently consume that loader.

Go already stages exact raw bytes, accounts for them, invokes the common trace
processor, writes derived components, atomically renames the staged bundle, and
only then publishes an opaque handle (`loomspan-console/internal/artifact/acquire.go:43-72`,
`loomspan-console/internal/artifact/acquire.go:75-222`). A failed copy, parse,
semantic validation, calculation, storage operation, or cancellation removes
the staged bundle and releases its charge. This is the existing atomic
admission boundary that the ticket names.

Every artifact and every analysis query is currently bound to a selected
`target.ScopeID`. Entry keys are `(scopeID, traceID)`, all service operations
compare against `currentScopeID`, target rotation invalidates matching entries,
query contexts expose `TargetScopeID`, and continuation cursors serialize a
`scopeId` (`loomspan-console/internal/artifact/model.go:64-69`,
`loomspan-console/internal/artifact/service.go:140-157`,
`loomspan-console/internal/artifact/target_owner.go:19-57`,
`loomspan-console/internal/traceanalysis/dto.go:8-21`,
`loomspan-console/internal/traceanalysis/cursor.go:37-56`). There is no imported
evidence owner, import catalog, upload route, multipart parser, or no-target
trace query path in the current checkout.

The browser currently offers an upstream raw attachment labeled **Download
Trace**, not **Download raw trace**, on Trace Detail
(`loomspan-console/web/src/observability/TraceDetail.tsx:105-107`). The GET route
streams a fresh application artifact directly to the browser, bypassing local
artifact installation and capacity accounting
(`loomspan-console/internal/browserapi/artifact_download.go:17-82`). Trace
Storage, Trace Detail, Trace Explorer, browser contracts, and all query routes
currently require a target scope. No file chooser or upload client exists.

## Detailed Findings

### Canonical Java NDJSON production

- `DefaultExecutionTraceHandle.initialize()` owns the first two canonical
  records. It writes `TRACE_STARTED` with `tracePath`, optional six-member
  `configuredLimits`, and `{sessionId}` data, followed by
  `TRACE_CAPTURE_POLICY_RECORDED`
  (`loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/runtime/trace/DefaultExecutionTraceHandle.java:254-268`).
- `appendInternal` assigns the next increasing sequence, converts payload data,
  emits chunk envelopes and `PAYLOAD_CHUNK_APPENDED` records when needed, and
  delegates every physical record to the same writer
  (`loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/runtime/trace/DefaultExecutionTraceHandle.java:357-412`).
- `NdjsonTraceRecordWriter` serializes each `TraceRecord` as one JSON object plus
  `\n` using UTF-8 and append mode
  (`loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/runtime/trace/NdjsonTraceRecordWriter.java:14-40`).
- Finalization adds framework-owned `errored` and `persistencePolicy` metadata
  and appends `TRACE_COMPLETED`. Retention policy then either deletes, briefly
  retains, or exposes the resulting file as a `FinalizedTraceArtifact`
  (`loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/runtime/trace/DefaultExecutionTraceHandle.java:294-350`).
- Disabled application observability changes the observation handle to a no-op;
  canonical trace construction remains in `DefaultExecutionTraceHandle`.
  `ObservabilityActivationCoordinator.createObservation()` selects a real or
  no-op observer based on activation state
  (`loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/observability/ObservabilityActivationCoordinator.java:53-58`).
- The Java reader is tolerant of a malformed final partial physical line and
  reconstructs chunked logical payloads for internal projections. It does not
  perform release compatibility checking
  (`loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/runtime/trace/NdjsonExecutionTraceReader.java:28-87`).

### Authoritative compatibility version

- The filtered resource declares
  `consoleCompatibilityVersion=${project.version}` and Maven enables filtering
  for `src/main/resources-filtered`
  (`loomspan-spring-boot-starter/src/main/resources-filtered/META-INF/loomspan-release.properties:1`,
  `loomspan-spring-boot-starter/pom.xml:124-130`).
- `LoomspanReleaseVersion.load()` is the Java release reader. It rejects zero or
  multiple resources, duplicate declarations, blank values, and unresolved
  placeholders (`loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/observability/LoomspanReleaseVersion.java:18-49`).
- The application instance response and its tests establish the current Java
  use of that exact value (`loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/observability/web/ObservabilityRestController.java:51-65`,
  `loomspan-spring-boot-starter/src/test/java/com/lokiscale/loomspan/internal/observability/web/ObservabilityRestIntegrationTest.java:68-76`).
- Go's authoritative runtime string is `release.ProductVersion()`. Source builds
  default it to `development`; release packaging injects the root POM version
  with linker flags. Startup validates a packaged version before serving, and
  the same function feeds target probing and browser bootstrap
  (`loomspan-console/internal/release/version.go:9-28`,
  `loomspan-console/cmd/loomspan-console/main.go:36-38`,
  `loomspan-console/cmd/loomspan-console/main.go:78-79`,
  `loomspan-console/internal/console/service.go:88-90`,
  `loomspan-console/internal/browserapi/router.go:261-268`).
- Current target compatibility is checked before the full instance DTO is
  decoded. A non-string marker is a protocol failure; an unequal string produces
  a failure carrying distinct expected and observed values
  (`loomspan-console/internal/applicationclient/client.go:197-230`). Shared
  error details already have `expectedCompatibilityVersion` and
  `observedCompatibilityVersion` fields
  (`loomspan-console/internal/consolecore/errors.go:26-34`).
- Current Go trace parsing does not read `consoleCompatibilityVersion` from
  `TRACE_STARTED`. Metadata is retained raw, and the processor specifically
  extracts only `configuredLimits` from the start record
  (`loomspan-console/internal/traceanalysis/parser.go:196-208`,
  `loomspan-console/internal/traceanalysis/processor.go:87-100`,
  `loomspan-console/internal/traceanalysis/processor.go:385-434`).

### Central artifact admission and lifecycle

- `artifact.Service` is the sole in-memory owner of raw and derived artifact
  state. It stores entries by `(target scope, trace ID)` and handles by opaque
  random value (`loomspan-console/internal/artifact/service.go:38-67`,
  `loomspan-console/internal/artifact/model.go:64-105`).
- `Acquire` accepts a captured `target.Scope` and caller-supplied trace ID,
  validates that the scope is current, joins concurrent acquisition for the
  same key, and returns an already-installed handle without another upstream
  request (`loomspan-console/internal/artifact/service.go:134-248`).
- Current duplicate identity behavior is therefore target-keyed reuse rather
  than rejection: a second acquire for an installed `(scopeID, traceID)` returns
  the same handle. There is no imported-owner duplicate policy in live code
  (`loomspan-console/internal/artifact/service.go:157-176`).
- The acquisition leader first loads authoritative catalog metadata, then opens
  a fresh upstream artifact stream. The application supplies trace/session ID,
  entry skill, outcome, finalization time, raw size, persistence policy, and
  application expiry (`loomspan-console/internal/artifact/acquire.go:23-72`,
  `loomspan-console/internal/console/service.go:165-183`).
- Known raw sizes reserve aggregate capacity before copying. Unknown sizes are
  charged incrementally before each disk write. Declared length and catalog
  size are checked against observed bytes before processing
  (`loomspan-console/internal/artifact/acquire.go:80-177`,
  `loomspan-console/internal/artifact/acquire.go:302-361`).
- Raw bytes are stored as `artifact.ndjson`. The processor receives only a
  cancellable reader, immutable metadata, and a logical component sink; neither
  processor nor callers receive the staging path
  (`loomspan-console/internal/artifact/processor.go:16-74`,
  `loomspan-console/internal/artifact/acquire.go:225-299`).
- Derived bytes share the same aggregate capacity. Finite-capacity admission
  removes expired unpinned entries, then least-recently-used unpinned entries.
  Active leases cannot be evicted (`loomspan-console/internal/artifact/capacity.go:15-83`,
  `loomspan-console/internal/artifact/capacity.go:85-119`).
- Successful lease close refreshes idle time. Expiry of a pinned entry defers
  removal; explicit removal rejects active pins
  (`loomspan-console/internal/artifact/lease.go:78-126`,
  `loomspan-console/internal/artifact/expiry.go:17-49`,
  `loomspan-console/internal/artifact/service.go:286-310`).
- Target invalidation cancels matching acquisitions, invalidates matching
  leases, removes matching bundles, and releases charges
  (`loomspan-console/internal/artifact/target_owner.go:19-57`). Service shutdown
  cancels all work and removes all bundles, and the verified workspace cleanup
  runs after the service closes (`loomspan-console/internal/artifact/service.go:455-498`,
  `loomspan-console/internal/console/service.go:193-197`,
  `loomspan-console/internal/console/service.go:264-270`). A fresh service does
  not adopt a previous handle or bundle
  (`loomspan-console/internal/console/artifact_integration_test.go:511-523`).

### Go parse, semantic validation, and shared calculations

- `traceanalysis.Processor.Process` is the required artifact processor. It
  parses and validates in one streaming pass, reconstructs payloads, builds
  frame/attempt/failure/usage facts, writes immutable indexes, and writes the
  manifest before returning success
  (`loomspan-console/internal/traceanalysis/processor.go:21-41`,
  `loomspan-console/internal/traceanalysis/processor.go:55-309`).
- The physical parser holds one bounded line, accepts LF/CRLF/no-final-newline
  framing, requires object-shaped valid UTF-8 JSON, limits a physical line to
  1 MiB, and limits JSON nesting to 128
  (`loomspan-console/internal/traceanalysis/parser.go:23-69`,
  `loomspan-console/internal/traceanalysis/parser.go:139-190`,
  `loomspan-console/internal/traceanalysis/limits.go:7-18`).
- Per-record decoding requires nonblank trace and session IDs, positive integer
  sequences, representable nonnegative numeric timestamps, known record/frame
  enums, and string-or-null optional identifiers
  (`loomspan-console/internal/traceanalysis/parser.go:159-208`).
- The validator requires stable trace/session identity, strictly increasing
  sequence, and exactly one completion. The processor separately requires that
  completion to be the last physical record and cross-checks outcome,
  finalization timestamp, persistence policy, failures, usage, frames, attempts,
  chunks, and calculations (`loomspan-console/internal/traceanalysis/validate.go:8-65`,
  `loomspan-console/internal/traceanalysis/processor.go:207-281`).
- Content invalidity maps outward to `INVALID_ARTIFACT`; the precise category is
  internal diagnostic/test evidence. The current error also says raw upstream
  download remains available (`loomspan-console/internal/traceanalysis/errors.go:9-61`).
  That availability fact reflects current target acquisition semantics.
- Manifest and index components are current-process bundle formats. The
  manifest explicitly describes its schema as internal and not durable
  (`loomspan-console/internal/traceanalysis/manifest.go:11-20`).

### Ownership assumptions in queries and continuations

- `Lease`, `Use`, `Lookup`, `Remove`, both clear operations, and storage
  snapshots all accept `target.ScopeID`; service methods reject a scope other
  than the single `currentScopeID` (`loomspan-console/internal/artifact/lease.go:14-20`,
  `loomspan-console/internal/artifact/service.go:251-452`).
- `TraceContext` names its owner field `TargetScopeID`, and every reusable
  result carries it with the opaque handle, trace ID, and session ID
  (`loomspan-console/internal/traceanalysis/dto.go:8-21`).
- Query methods acquire a scope-bound lease, load the same installed manifest
  and indexes, and return the same calculated facts. For example, summary
  construction takes `target.ScopeID` and emits it in `TraceContext`
  (`loomspan-console/internal/traceanalysis/query_facts.go:24-45`).
- Continuations contain schema, operation, `scopeId`, handle, query fingerprint,
  and position. Their validation precedence treats scope mismatch as
  `TARGET_CHANGED`, then relies on lease acquisition for handle expiry, then
  checks query meaning (`loomspan-console/internal/traceanalysis/cursor.go:37-61`,
  `loomspan-console/internal/traceanalysis/cursor.go:157-216`).
- Capacity eviction scans the aggregate entry map without a scope filter, while
  idle expiry currently filters to `currentScopeID`
  (`loomspan-console/internal/artifact/capacity.go:89-112`,
  `loomspan-console/internal/artifact/expiry.go:107-145`). Storage snapshots
  filter entries to the requested current target scope
  (`loomspan-console/internal/artifact/service.go:360-410`).

### Browser save, acquire, storage, and explorer paths

- The raw attachment is a special authenticated GET navigation route. It
  validates exact host and navigation-shaped fetch metadata, but does not use
  the POST origin/CSRF path (`loomspan-console/internal/browserapi/router.go:79-101`,
  `loomspan-console/internal/browserapi/request_policy.go:71-106`).
- Raw download rejects queries, ranges, conditional requests, and path-shaped
  trace IDs; captures the current target; opens a fresh upstream stream; sets
  `application/x-ndjson`, `no-store`, a sanitized attachment filename, and the
  declared length; then copies with a 32 KiB buffer
  (`loomspan-console/internal/browserapi/artifact_download.go:17-100`). It does
  not consult or install a local artifact.
- Trace Detail currently loads application trace metadata through
  `getTraceDetail`, verifies `targetScopeId`, offers **Download Trace**, and
  separately offers CSRF-protected **Acquire for analysis**. Trace Explorer is
  mounted only after `localAvailable` becomes true
  (`loomspan-console/web/src/observability/TraceDetail.tsx:32-96`,
  `loomspan-console/web/src/observability/TraceDetail.tsx:105-131`).
- Trace Explorer resets and reloads when target scope generation changes and
  verifies every response against the current target scope. Its queries use
  trace ID; the browser adapter resolves that to the current scope's handle
  (`loomspan-console/web/src/observability/TraceExplorer.tsx:42-107`,
  `loomspan-console/internal/browserapi/trace_analysis.go:20-45`).
- Trace Storage requests a current target, returns one target-scoped snapshot,
  and keys/removes rows by trace ID. Its DTO contains application availability
  and application expiry for every row
  (`loomspan-console/internal/browserapi/artifacts.go:81-109`,
  `loomspan-console/web/src/observability/TraceStorage.tsx:30-95`,
  `loomspan-console/web/src/api/contracts.ts:192-219`).
- Current trace, storage, and analysis DTOs identify only `targetScopeId`; they
  have no `source: TARGET|IMPORTED` discriminator. Current imported-source
  labeling and the absence of application identity/availability claims are
  therefore not represented in live browser contracts
  (`loomspan-console/web/src/api/contracts.ts:164-219`,
  `loomspan-console/web/src/api/contracts.ts:229-258`).
- All ordinary browser API operations are POST-only after exact origin
  validation. Mutations opt into CSRF; current artifact acquire/remove/clear
  operations do, while storage and analysis reads do not
  (`loomspan-console/internal/browserapi/router.go:94-190`). Current JSON artifact
  request bodies are limited to 4 KiB
  (`loomspan-console/internal/browserapi/artifacts.go:13-13`). There is no
  streaming body-upload handler today.
- The TypeScript client has JSON functions for acquire/storage/remove/clear and
  only a URL constructor for raw download; it has no file or `FormData` API
  (`loomspan-console/web/src/api/client.ts:176-208`).

### Tests and executable fixtures

- Java owns corpus generation and byte-compares the complete committed
  inventory. It also checks valid semantic invariants and named invalidity cases
  (`loomspan-spring-boot-starter/src/test/java/com/lokiscale/loomspan/internal/runtime/trace/ConsoleTraceFixtureCorpusTest.java:116-160`,
  `loomspan-spring-boot-starter/src/test/java/com/lokiscale/loomspan/internal/runtime/trace/ConsoleTraceFixtureCorpusTest.java:505-522`).
- Go's `fixture_corpus_test.go` processes every committed valid and invalid Java
  fixture through the real processor and compares neutral facts/categories
  (`loomspan-console/internal/traceanalysis/fixture_corpus_test.go:23-130`).
- Parser and processor unit tests cover structural bounds, cancellation, blank
  and incomplete artifacts, ordering, identity, terminality, chunking, usage,
  frame relationships, and derived-bundle publication
  (`loomspan-console/internal/traceanalysis/parser_test.go:13-497`,
  `loomspan-console/internal/traceanalysis/processor_test.go:64-242`).
- Artifact service tests cover joined acquisition, cancellation, capacity,
  expiry, active leases, removal, scope invalidation, storage failures, and
  shutdown. Console integration tests cover Java-compatible acquisition, target
  rotation, restart non-adoption, and bounded cleanup
  (`loomspan-console/internal/artifact/acquire_test.go:22-835`,
  `loomspan-console/internal/artifact/service_test.go:17-566`,
  `loomspan-console/internal/console/artifact_integration_test.go:439-640`).
- Browser API tests cover session/CSRF ordering, raw-download navigation
  security, exact bytes and headers, scoped storage DTOs, and artifact error
  mapping (`loomspan-console/internal/browserapi/artifact_download_test.go:17-367`,
  `loomspan-console/internal/browserapi/artifacts_test.go:19-756`). React and
  Playwright tests cover Trace Detail acquisition/explorer transitions and Trace
  Storage actions (`loomspan-console/web/src/observability/TraceDetail.test.tsx`,
  `loomspan-console/web/src/observability/TraceStorage.test.tsx`,
  `loomspan-console/web/e2e/artifact-storage.spec.ts`).
- The fixture README currently classifies traces as current-checkout ephemeral
  diagnostics and says older trace readability is not promised. It documents
  Java-to-Go fixture ownership and the absence of an independent schema version
  (`loomspan-console-fixtures/README.md:1-3`,
  `loomspan-console-fixtures/README.md:53-68`).

### Documentation and sensitive content

- Console documentation describes all local artifacts as target-bound,
  process-local, and removed on target rotation, shutdown, or restart. It
  documents aggregate capacity, idle TTL, opaque handles, five artifact POST
  operations, and the separate fresh upstream raw download
  (`loomspan-console/README.md:287-365`).
- Skill-authoring guidance states that exceptions, stack text, and tool
  arguments are application diagnostic content that may contain sensitive
  values, are not secret-scanned or redacted, and are loaded through explicit
  finalized-trace actions (`ai/skill-authoring/traces-and-debugging.md:93-102`).
- Raw and reconstructed content is rendered as text or explicit range content;
  React does not parse canonical trace semantics. Go creates the authoritative
  facts, and browser adapters map them to DTOs
  (`loomspan-console/internal/traceanalysis/processor.go:21-41`,
  `loomspan-console/web/src/observability/TraceExplorer.tsx`).

## Contract and Compatibility Classification

### Application API

There is no existing supported application-developer API for importing or
exporting a trace. Canonical trace classes and constructors live under
`com.lokiscale.loomspan.internal`. The architecture test maintains a closed
seven-type supported API allowlist and separately records reasons for
technically public internal types
(`loomspan-spring-boot-starter/src/test/java/com/lokiscale/loomspan/architecture/LoomspanPublicSurfaceArchitectureTest.java:30-87`,
`loomspan-spring-boot-starter/src/test/java/com/lokiscale/loomspan/architecture/LoomspanPublicSurfaceArchitectureTest.java:174-223`).
The ticket's save/open actions are Console product behavior, not a current Java
Application API.

### Supported SPI

No supported SPI package or trace replacement point exists. The architecture
test explicitly asserts that no supported SPI package exists
(`loomspan-spring-boot-starter/src/test/java/com/lokiscale/loomspan/architecture/LoomspanPublicSurfaceArchitectureTest.java:225-230`).
Trace construction, observation activation, artifact processing, and browser
interfaces are framework/internal composition seams.

### Configuration and manifest contracts

The current documented Console configuration surface is
`trace-workspace.max-bytes` and `trace-workspace.idle-ttl`, with `unlimited` and
`never` sentinels. Those values feed the single artifact service
(`loomspan-console/internal/config/config.go:15-44`,
`loomspan-console/internal/config/config.go:68-74`,
`loomspan-console/internal/console/service.go:157-162`). No current property,
YAML skill field, manifest field, or persisted import setting names portable
trace import.

### Persisted or serialized contracts

Before PR 25, the canonical trace is documented and tested as a current-release
Java-to-Go diagnostic contract, not a durable cross-version format. The ticket
and updated framework design lens deliberately classify a complete marked
canonical NDJSON file as portable only to an exact matching
`consoleCompatibilityVersion`
(`ai/thoughts/framework-feature-design-lens.md:26-49`,
`ai/thoughts/tickets/loomspan-console-pr-25-portable-trace-import.md:55-94`).
The protected consumers of the current serialized trace protocol are the Go
parser/processor and the Java-to-Go fixture corpus. The application artifact
REST stream, Console acquisition, and raw browser attachment transport the same
bytes. Current fixtures demonstrate existing behavior but are explicitly not a
historical-reader promise.

### Ephemeral diagnostic formats

Artifact bundle directory names, `artifact.ndjson` component layout, indexes,
`manifest.json`, opaque handles, storage snapshots, browser DTOs, query
contexts, and continuation cursors are process-local diagnostic formats. The
manifest and handle documentation explicitly state that status
(`loomspan-console/internal/traceanalysis/manifest.go:11-20`,
`loomspan-console/internal/artifact/handle.go:9-18`,
`loomspan-console/README.md:326-331`).

### Internal or accidentally exposed implementation

The Java trace record/handle/writer/reader, release loader, Go artifact service
interfaces, target-scope keys, browser adapter DTOs, React state, and
transport-neutral analysis DTOs are internal implementation surfaces. Some
Java classes and constructors are public for cross-package internal
composition, but the architecture allowlist records that technical exposure as
internal rather than supported API. No `@ConditionalOnMissingBean` trace
replacement contract was found; relevant auto-configuration beans are
infrastructure-role package-private methods
(`loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/autoconfigure/LoomspanAutoConfiguration.java:125-179`).

## Cross-Component Protocol Consumers

The canonical NDJSON boundary currently connects these in-repository consumers:

1. Java `DefaultExecutionTraceHandle` and `NdjsonTraceRecordWriter` produce the
   bytes.
2. Java `NdjsonExecutionTraceReader` and journal/observation projections read
   the current file during execution and finalization.
3. The application observability artifact endpoint streams the finalized file
   with instance identity and content metadata.
4. Go `applicationclient.ArtifactStream` transports exact bytes after target
   compatibility has already been established.
5. Go `artifact.Service` stages and atomically publishes the raw plus derived
   bundle.
6. Go `traceanalysis.Processor` parses, validates, and calculates the shared
   facts used by browser adapters and the planned MCP adapter.
7. The browser raw attachment streams the upstream bytes independently of
   semantic admission.
8. `loomspan-console-fixtures` and Java/Go corpus tests are the executable
   cross-language agreement.

The observable semantics shared across the boundary are physical NDJSON
framing, record identity and order, known enums, chunk reconstruction,
configured limits, frame/attempt/failure relationships, terminal completion,
outcome, persistence policy, usage reconciliation, gaps, uncertainties, and
derived payload/range addressing. PRs 21 through 24 have ticketed changes to
entry skill, failure diagnostics, provider attempts, and tool-call lifecycle;
the PR 25 ticket explicitly places its marker and import work after those
canonical-trace changes.

## Architecture Documentation

The existing flow is:

```text
Java session
  -> DefaultExecutionTraceHandle
  -> canonical NDJSON file
  -> application artifact GET
  -> Go ArtifactStream
  -> capacity-accounted staging bundle
  -> shared traceanalysis.Processor
  -> atomic installed bundle + opaque handle
  -> shared lease/query services
  -> browser adapter -> Trace Explorer / Trace Storage

Java canonical NDJSON
  -> browser raw-download GET
  -> exact fresh upstream attachment (no local installation)
```

The first path is an admitted analysis-copy lifecycle. The second is an exact
attachment lifecycle. The central artifact service already supplies the common
staging, capacity, validation, derived-storage, atomic publication, TTL,
pinning, eviction, explicit removal, shutdown, and restart-cleanup mechanics.
Its owner abstraction is presently concrete target scope rather than a general
evidence owner.

## Historical Context (from ai/thoughts/)

- `ai/thoughts/tickets/loomspan-console-pr-12-artifact-service.md` established
  one central target-scoped copy/handle/lifecycle and kept raw attachment
  pass-through separate from analysis admission.
- `ai/thoughts/tickets/loomspan-console-pr-13-trace-analysis-services.md`
  established current-release streaming parsing, strict validation, derived
  indexes, shared calculations, and atomic rejection.
- `ai/thoughts/tickets/loomspan-console-pr-14-trace-explorer.md` established the
  browser Trace Explorer and explicit unchanged raw-artifact attachment.
- `ai/thoughts/tickets/loomspan-console-pr-15-diagnostic-workflows.md` describes
  Phase 2 hardening and explicitly excludes cross-version traces.
- `ai/thoughts/tickets/loomspan-console-pr-18-mcp-trace-inspection.md` describes
  the future MCP consumer of the same ownership, handles, queries, and
  continuations; PR 25 itself adds no MCP protocol.
- `ai/thoughts/framework-feature-design-lens.md` now records the settled narrow
  same-version portability classification and keeps imported bundles, handles,
  continuations, and catalogs ephemeral.
- `ai/thoughts/phases/2026-07-23-loomspan-console-implementation-roadmap.md`
  places PR 25 between current browser hardening and MCP trace inspection.

## Related Research

No earlier document exists under `ai/thoughts/research/` in this checkout. The
directly related current-state descriptions are the tickets and phase documents
listed above.

## Open Questions

- The repository contains no current imported-evidence DTO vocabulary, so the
  exact internal type names and route shapes for owner/source labeling are not
  yet established in live code.
- The ticket requires a bounded streaming upload, while the current browser API
  has only small bounded JSON POST bodies and the special raw-download GET. No
  existing upload media type or framing convention is present to document.
- Imported Trace Detail navigation cannot use the current target-backed
  `getTraceDetail` entry path. The live code does not yet contain the imported
  discovery/detail response shape that will supply that page.
- The current source default `development` is rejected by packaged-console
  startup validation. Development mode in the repository is exercised through
  build/test tooling rather than a documented packaged runtime accepting that
  value; no trace-marker development test exists yet.
- Current invalid-artifact details assert that raw upstream download remains
  available. An imported file has no upstream application, and the live code
  does not yet define the corresponding imported rejection detail envelope.
