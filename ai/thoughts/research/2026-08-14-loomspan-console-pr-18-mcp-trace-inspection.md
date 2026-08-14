---
date: 2026-08-14T00:05:07-07:00
researcher: Codex (GPT-5)
model: GPT-5
git_commit: 14a910ca159a1f67a536f0790d7d0553933f6791
branch: main
repository: loomspan
topic: "Loomspan Console PR 18 — Trace-Inspection MCP Surface"
tags: [research, codebase, loomspan-console, mcp, traces, artifacts, imported-evidence, continuations]
status: complete
last_updated: 2026-08-14
last_updated_by: Codex (GPT-5)
last_updated_note: "Resolved the open questions, recorded implementation recommendations and the revised range guidance, and copied the durable planning handoff into the PR 18 ticket."
---

# Research: Loomspan Console PR 18 — Trace-Inspection MCP Surface

**Date**: 2026-08-14 00:05:07 PDT
**Researcher**: Codex (GPT-5)
**Model**: GPT-5
**Git Commit**: `14a910ca159a1f67a536f0790d7d0553933f6791`
**Branch**: `main`
**Repository**: `loomspan`

## Research Question

Research the current codebase for
`ai/thoughts/tickets/loomspan-console-pr-18-mcp-trace-inspection.md`, using
`ai/thoughts/phases/loomspan_console_phase_3_llm_runtime_inspector.md` as
background. Document the existing MCP, target trace, imported evidence,
artifact, query, continuation, resource, error, lifecycle, protocol, fixture,
and test seams that the ticket names. Describe the implementation as it exists;
do not design or implement PR 18.

## Summary

At the researched commit, PR 18 is not present in the current MCP tool catalog.
`mcpadapter.NewServer` registers runtime, skill, active-execution, and recent-
activity tools plus one skill resource template. The server exposes six tools
and four named Loomspan capabilities. `LOOMSPAN_list_traces`,
`LOOMSPAN_get_trace`, both trace query tools, both range tools, the two trace
capabilities, and trace resources are absent from the current adapter
(`loomspan-console/internal/mcpadapter/server.go:31-48`,
`loomspan-console/internal/mcpadapter/capabilities.go:8-27`,
`loomspan-console/internal/mcpadapter/contracts_test.go:106-122`).

The transport-neutral implementation below that adapter already contains most
of the evidence lifecycle and calculation surface named by the ticket:

- `observability.Service` lists and retrieves finalized traces from the
  selected application's current-instance catalog
  (`loomspan-console/internal/observability/service.go:151-201`).
- One `artifact.Service` owns target acquisitions and imported traces, one
  opaque-handle namespace, capacity, TTL, pinning, removal, target rotation,
  and shutdown cleanup (`loomspan-console/internal/artifact/service.go:39-68`,
  `:145-319`, `:359-469`, `:472-515`).
- `evidence.Reference` is the adapter-safe discriminator for `TARGET` versus
  `IMPORTED`. Imported references carry no target scope and resolve to a
  service-owned process-local owner (`loomspan-console/internal/evidence/owner.go:10-52`,
  `loomspan-console/internal/artifact/service.go:456-469`).
- One `traceanalysis.Service` is both the artifact processor and the query
  service. It validates and indexes the complete raw NDJSON before a handle is
  published, then exposes summaries, frames, records, attempts, retries,
  validation links, failures, diagnostics, usage, payloads, gaps,
  uncertainties, search, payload ranges, raw-record ranges, and raw-artifact
  ranges (`loomspan-console/internal/traceanalysis/processor.go:26-32`,
  `loomspan-console/internal/traceanalysis/service.go:35-103`, and the
  `query_*.go` files).
- The composition root gives the browser the artifact and trace-analysis
  services, but does not yet give them to `mcpadapter.ServerOptions`
  (`loomspan-console/internal/console/service.go:165-209`, `:248-275`).

Imported inspection is already target-optional below MCP. Browser import,
storage, removal, and trace-analysis handlers resolve `IMPORTED` without
capturing a selected target; imported artifacts survive target rotation but
participate in the same TTL, capacity, removal, shutdown, and restart-cleanup
lifecycle as target artifacts (`loomspan-console/internal/browserapi/artifact_import.go:11-48`,
`loomspan-console/internal/browserapi/artifacts.go:86-140`, `:211-239`,
`loomspan-console/internal/artifact/target_owner.go:24-61`). Current MCP helper
functions, by contrast, capture a selected target before every PR 17 inspection
operation (`loomspan-console/internal/mcpadapter/server.go:55-67`,
`loomspan-console/internal/mcpadapter/skills.go:42-50`).

The existing trace-analysis continuations already bind the operation, evidence
owner, artifact handle, canonical query fingerprint, and next position. The
fingerprint includes filters, ordering or representation, page/range size, and
other operation-specific query meaning. Tokens are versioned strict base64url
JSON, not signed or encrypted; handle and owner validity are checked again by
the artifact service on each call. The installed copy, owner, and handle
lifetime therefore provide the lifetime boundary
(`loomspan-console/internal/traceanalysis/cursor.go:13-55`, `:57-105`,
`:148-264`). This is a richer binding than the current PR 17 MCP continuation,
which binds operation kind, target scope, underlying cursor, and activity
session only (`loomspan-console/internal/mcpadapter/continuation.go:14-109`).

## Detailed Findings

### 1. Current MCP adapter surface

The MCP server uses the official Go SDK `v1.7.0` and stateless Streamable HTTP
with JSON responses and propagated request cancellation
(`loomspan-console/go.mod:8`,
`loomspan-console/internal/mcpadapter/server.go:36-47`). Security middleware
applies exact loopback authority and supplied-Origin validation, MCP key
authentication, a ten-second request context, a 1 MiB request-body limit,
generation admission, and `Cache-Control: no-store` before SDK handling
(`loomspan-console/internal/mcpadapter/security.go:13-81`).

Current registration is:

| Family | Tools/resources | Capability |
| --- | --- | --- |
| Runtime | `LOOMSPAN_get_runtime` | `loomspan.runtime-status.v1` |
| Skills | `LOOMSPAN_list_skills`, `LOOMSPAN_get_skill`, skill YAML resource | `loomspan.skill-inspection.v1` |
| Active executions | `LOOMSPAN_list_executions`, `LOOMSPAN_get_execution` | `loomspan.active-execution-inspection.v1` |
| Recent activity | `LOOMSPAN_get_execution_activity` | `loomspan.recent-activity-inspection.v1` |

`installedCapabilities` returns the runtime capability followed by every
descriptor in a fixed table. The capability test removes each required tool in
turn and expects conformance to fail; the helper is test-only rather than a
dynamic comparison against the SDK registry
(`loomspan-console/internal/mcpadapter/capabilities.go:3-27`,
`loomspan-console/internal/mcpadapter/capabilities_test.go:5-48`).

All current inspection tools use read-only, idempotent, non-destructive,
closed-world annotations and a structured envelope with exactly one `result`
or `error` arm. Shared domain failures set MCP `isError` and preserve the
shared code, safe message, optional target scope, and permitted details. Text
fallbacks use deterministic JSON string escaping
(`loomspan-console/internal/mcpadapter/contracts.go:17-30`, `:112-149`,
`loomspan-console/internal/mcpadapter/contracts_test.go:14-103`).

The MCP golden inventory contains only runtime, skills, executions, and
activity results. Server tests assert exactly six tools and one resource
template (`loomspan-console/internal/mcpadapter/contracts_test.go:106-122`,
`loomspan-console/internal/mcpadapter/server_test.go:33-125`). The README also
states that finalized trace discovery and analysis belong to the later trace-
inspection capability (`loomspan-console/README.md:235-281`).

### 2. Target trace discovery and acquisition

`observability.Trace` carries trace/session identity, entry skill, outcome,
finalization time, raw size, persistence policy, application expiry, local
availability, artifact handle, and acquisition-time application availability
(`loomspan-console/internal/observability/dto.go:80-94`).
`observability.Service.ListTraces` forwards a bounded application cursor and
page size, validates the application response, injects the captured target
scope, and rechecks scope currency before publication. `GetTrace` performs the
same identity and scope checks for one trace
(`loomspan-console/internal/observability/service.go:151-201`).

The browser's target trace list is currently an adapter composition rather than
a single transport-neutral joined inventory: it calls the application catalog
and enriches each result with `artifact.Service.Lookup`. Lookup is side-effect
free and does not refresh last use
(`loomspan-console/internal/browserapi/artifacts.go:174-209`,
`loomspan-console/internal/artifact/service.go:414-453`).

`artifact.Service.Acquire` is keyed by `(target evidence owner, traceId)`.
Concurrent browser/future-MCP callers join the same acquisition; an installed
entry returns the same handle without another upstream request. Waiter
cancellation is independent, and the acquisition leader is canceled only when
the scope/service ends or the last waiter leaves
(`loomspan-console/internal/artifact/service.go:145-259`). The service loads
authoritative catalog metadata, opens one exact upstream artifact stream,
copies it under capacity accounting, runs the shared processor, atomically
installs the bundle, and only then publishes the handle
(`loomspan-console/internal/artifact/acquire.go:50-85`, `:87-241`).

Handles are 32 random bytes rendered as exactly 64 lowercase hexadecimal
characters. They encode no trace, scope, or path. A malformed handle is
`INVALID_ARGUMENT`; a well-formed handle absent for the selected owner is
`ARTIFACT_EXPIRED` (`loomspan-console/internal/artifact/handle.go:9-59`,
`loomspan-console/internal/artifact/service.go:262-293`).

### 3. Imported evidence and target-optional access

`evidence.Source` has exactly `TARGET` and `IMPORTED`. `evidence.Reference`
contains a target scope only for target evidence; imported references are valid
only with an empty target scope. The underlying imported `Owner` contains one
random process-local ID created when the artifact service starts, but that ID
is not part of adapter-facing references
(`loomspan-console/internal/evidence/owner.go:10-61`,
`loomspan-console/internal/artifact/service.go:106-135`).

Import behavior is:

1. accept one untrusted NDJSON stream with a declared length used only as an
   admission bound;
2. preflight the first bounded `TRACE_STARTED` record for identity and exact
   `consoleCompatibilityVersion`;
3. reject a duplicate imported `traceId` already present under the imported
   owner;
4. install through the same copy, processor, capacity, bundle, and handle path;
5. replace caller/preflight completion facts with facts derived by complete
   processor validation.

These steps are implemented in
`loomspan-console/internal/artifact/import.go:12-120`,
`loomspan-console/internal/traceanalysis/processor.go:49-92`, and
`loomspan-console/internal/artifact/acquire.go:186-240`. The raw import ceiling
is 4 GiB, reduced when the configured finite aggregate cache is smaller
(`loomspan-console/internal/artifact/import.go:10`, `:103-109`).

`StorageSnapshot` is the current common discovery view for installed target
and imported copies. Each entry reports `source`; target entries additionally
report `targetScopeId`, application expiry, and application availability.
Imported entries omit those target-only facts
(`loomspan-console/internal/artifact/model.go:126-174`,
`loomspan-console/internal/artifact/service.go:359-412`). The browser Trace
Storage page uses this snapshot to link target entries to target trace routes
and imported entries to `/traces/imported/{traceId}`
(`loomspan-console/web/src/observability/TraceStorage.tsx:212-250`,
`loomspan-console/web/src/observability/ImportedTrace.tsx:1-14`).

Target rotation only cancels and removes entries owned by the previous target
scope; imported entries remain. Global expiry, capacity eviction, explicit
removal, `ClearAllUnused`, service close, and workspace restart cleanup include
both sources (`loomspan-console/internal/artifact/target_owner.go:24-61`,
`loomspan-console/internal/artifact/expiry.go:75-139`,
`loomspan-console/internal/artifact/capacity.go:16-81`,
`loomspan-console/internal/artifact/service.go:296-357`, `:472-515`).

The browser's source resolver returns `evidence.ForImported()` before it tries
to capture a target. Consequently imported trace analysis and removal work
without a selected target. The current MCP `captureScope` helper has no
equivalent evidence-source branch (`loomspan-console/internal/browserapi/artifacts.go:211-224`,
`loomspan-console/internal/browserapi/trace_analysis.go:23-47`,
`loomspan-console/internal/mcpadapter/server.go:55-67`).

### 4. Shared trace facts and query operations

The production composition constructs one `traceanalysis.Service`, supplies it
as the artifact processor, then wires the artifact service back into the same
instance for lease-based queries. Browser handlers consume that query service
directly (`loomspan-console/internal/console/service.go:165-205`).

The current fact map is:

| Ticket fact | Current transport-neutral source |
| --- | --- |
| Source and identity | `TraceContext.Evidence`, `Handle`, `TraceID`, `SessionID` in `traceanalysis/dto.go:11-24` |
| Outcome, limits, counts, roots, usage | `GetSummary` / `TraceSummary` in `query_facts.go:15-75` and `dto.go:27-49` |
| Hierarchy, timing, direct/descendant/inclusive usage | `QueryFrames` / `FrameSummary` in `query_frames.go:14-188` and `dto.go:52-83` |
| Physical/logical record ordering and raw addresses | `QueryRecords` / `RecordSummary` in `query_records.go:14-110` and `dto.go:86-112` |
| Attempts and retry usage | `QueryAttempts`, `QueryRetries` in `query_facts.go:145-321` |
| Validation links | `QueryValidationLinks` in `query_facts.go:322-382` |
| Failures and terminal attribution | `QueryFailures`, `GetFailureDiagnostic` in `query_facts.go:383-461` and `query_diagnostics.go:16-72` |
| Reconstructed payload descriptors/content | `QueryPayloads`, `ReadPayloadRange` in `query_facts.go:462-524` and `query_ranges.go:17-90` |
| Gaps and uncertainty | `QueryGaps`, `QueryUncertainties` in `query_facts.go:525-625` |
| Exact raw record/artifact bytes | `ReadRawRecordRange`, `ReadRawArtifactRange` in `query_ranges.go:92-236` |
| Literal search | `Search` in `search.go:33-281` |
| Current application/local availability | `observability.Trace` plus `artifact.LookupResult`; it is not recalculated by `traceanalysis` |

The processor calculates these facts once while validating the artifact and
writes immutable bundle components and a manifest. The query methods project
those stored results; they do not parse a second MCP trace model
(`loomspan-console/internal/traceanalysis/processor.go:26-32`, `:88-139`,
`:347-439`). The committed fixture corpus is explicitly the current-release
Java-to-Go semantic contract and contains no UI or MCP model
(`loomspan-console-fixtures/README.md:1-18`).

`TraceContext` carries `evidence.Reference` on every reusable query result, so
the same service output distinguishes target and imported evidence without
inventing a target scope. `artifact.Service.Use` validates the reference and
handle together before issuing a lease (`loomspan-console/internal/traceanalysis/dto.go:11-24`,
`loomspan-console/internal/artifact/service.go:262-293`).

### 5. Current query schemas and bounds

Frame queries accept exact frame IDs and filters for parent, frame type, route,
skill, outcome, attempt, retry, validation status, and failure. Orders are
`CANONICAL`, `DURATION_DESC`, and `USAGE_DESC`. Multiple filters are ANDed;
multi-value frame IDs form the one set membership test
(`loomspan-console/internal/traceanalysis/query_frames.go:14-58`, `:64-188`).

Record queries accept record types, frame, route, sequence and timestamp
ranges, attempt, retry, validation status, failure, and bounded literal text.
Representation is `PHYSICAL` or `LOGICAL`; logical results omit individual
chunk records. Explicit inline payloads are included only for envelopes whose
reconstructed payload is at most 8 KiB
(`loomspan-console/internal/traceanalysis/query_records.go:14-65`, `:70-85`,
`loomspan-console/internal/traceanalysis/limits.go:20-39`).

Service-level query bounds are:

| Bound | Current value |
| --- | --- |
| Default / maximum trace query page | 100 / 1,000 items |
| Maximum explicitly inlined payload | 8 KiB |
| Default / maximum byte range | 64 KiB / 1 MiB |
| Literal search text | 1 KiB and 256 code points |
| Per-call search work | 8 MiB and 10,000 records |
| Physical NDJSON line | 1 MiB |
| JSON nesting | 128 |

These values live in `loomspan-console/internal/traceanalysis/limits.go:8-48`.
The current MCP adapter separately requires list/activity `pageSize` from 1
through 64 and caps its HTTP request body at 1 MiB
(`loomspan-console/internal/mcpadapter/contracts.go:14`, `:248-267`,
`loomspan-console/internal/mcpadapter/security.go:13-14`). No PR 18 MCP
response-framing constant or trace-specific MCP schema exists in the current
tree.

Payload, raw-record, and raw-artifact reads report actual byte offsets, total
length, content type, `TEXT` or `BASE64`, `hasMore`, and a next cursor. Valid
UTF-8 JSON/text/NDJSON slices are text; other exact bytes are standard base64.
The next offset is the returned `actualEnd`, so traversal does not discard
bytes at a UTF-8 boundary (`loomspan-console/internal/traceanalysis/dto.go:251-288`,
`loomspan-console/internal/traceanalysis/range.go:33-257`).

The current range request names payloads with `PayloadID`, records with a
positive canonical sequence, and raw artifacts by absolute byte start. A
caller supplies either `Start` or `ContinueCursor`; both together are invalid
(`loomspan-console/internal/traceanalysis/range.go:13-31`,
`loomspan-console/internal/traceanalysis/query_ranges.go:17-63`, `:92-134`,
`:191-257`).

### 6. Continuation representation and lifetime binding

There are two current continuation layers:

1. PR 17 MCP continuations wrap application/live cursors. Their strict JSON
   payload contains version `1`, operation kind, `targetScopeId`, cursor, and
   an activity `sessionId` when applicable. They are unpadded base64url,
   limited to 8,192 characters, reject unknown/trailing fields, and return
   `TARGET_CHANGED` for a prior scope. They have no signature, encryption,
   server registry, handle, or query fingerprint
   (`loomspan-console/internal/mcpadapter/continuation.go:12-109`).
2. Trace-analysis cursors contain schema `v1`, operation, owner key, handle,
   SHA-256 query fingerprint, next position, and optional bounded search state.
   They are unpadded base64url JSON and are also unsigned and unencrypted
   (`loomspan-console/internal/traceanalysis/cursor.go:13-55`, `:57-105`).

Trace query canonicalization hashes the fields that define meaning. Frame
fingerprints include normalized filters, order, and resolved page size; record
fingerprints include filters, representation, inline-payload choice, and
resolved page size; range fingerprints include source, payload/record identity,
and maximum bytes (`loomspan-console/internal/traceanalysis/query_frames.go:46-58`,
`loomspan-console/internal/traceanalysis/query_records.go:53-85`,
`loomspan-console/internal/traceanalysis/query_ranges.go:244-263`).

The owner key is `source + ":" + owner.ID()`. For target evidence that ID is
the target scope. For imported evidence it is the service's random process-
local imported-owner ID. That owner key is encoded inside the cursor token,
while adapter-facing `evidence.Reference` continues to omit the imported owner
ID (`loomspan-console/internal/traceanalysis/cursor.go:262-264`,
`loomspan-console/internal/evidence/owner.go:22-52`).

Every continued call first resolves the evidence owner and acquires a lease for
the handle. A missing or removed handle is therefore expired, a target-owner
mismatch is stale scope, and a changed query fingerprint is an invalid cursor.
Process shutdown removes every entry and creates a different imported owner in
the next process, so neither handles nor trace cursors survive restart
(`loomspan-console/internal/traceanalysis/service.go:74-103`,
`loomspan-console/internal/traceanalysis/cursor.go:148-264`,
`loomspan-console/internal/artifact/service.go:472-515`).

### 7. Pinning, last use, removal, and cancellation

Every trace-analysis call acquires one `artifact.Lease`. The lease increments
the entry pin count, preventing capacity eviction and explicit removal. A
successful close refreshes the single shared `lastUsedAt`; a failed or canceled
close does not. Expiry during a pin marks deferred removal, completed when the
last lease closes (`loomspan-console/internal/artifact/lease.go:11-27`,
`:76-135`).

Target-scope invalidation synchronously cancels target acquisitions and
invalidates target leases/readers before removing their bundles. Imported
leases are not selected by that owner comparison
(`loomspan-console/internal/artifact/target_owner.go:24-61`). Ordinary removal
returns `ARTIFACT_IN_USE` rather than canceling an active call
(`loomspan-console/internal/artifact/service.go:296-319`).

All trace query loops check `ctx.Err()` and their component readers can be
closed on cancellation. The MCP SDK propagates request cancellation, while
MCP credential regeneration/disablement and console shutdown cancel and drain
admitted request contexts through the shared tracker/lifecycle
(`loomspan-console/internal/traceanalysis/query_frames.go:113-155`,
`loomspan-console/internal/traceanalysis/query_records.go:95-163`,
`loomspan-console/internal/mcpadapter/server.go:43-47`,
`loomspan-console/internal/mcpadapter/lifecycle.go:40-177`).

Multiple MCP clients use one stateless SDK server and the same process-local
services. Existing MCP tests prove two clients share the live window while
canceling independently; artifact tests prove concurrent acquisition joins one
copy and cancellation of one waiter does not fail the other
(`loomspan-console/internal/mcpadapter/server_test.go:278-351`,
`loomspan-console/internal/artifact/acquire_test.go:22-89`).

### 8. Resources

The only current MCP resource template is
`loomspan://targets/{targetScopeId}/skills/{skillName}`. Its parser requires a
canonical `loomspan` URI, exactly one canonical percent-encoded UTF-8 segment
for each variable, no query/fragment/user info, and the current target scope.
Resource errors retain the shared domain envelope in JSON-RPC error data
(`loomspan-console/internal/mcpadapter/resources.go:13-132`).

The Phase 3 background names target-scoped artifact summary, frame, and record
templates and explicitly excludes raw-artifact resources in favor of a bounded
tool (`ai/thoughts/phases/loomspan_console_phase_3_llm_runtime_inspector.md:393-412`).
The current repository contains no trace resource registration, parser, MIME
contract, resource DTO, or imported-evidence URI family.

### 9. Error and unsafe-content boundaries

Shared domain codes include target authentication/access/availability and
compatibility, target change, invalid/stale cursors, not found, artifact
expiry/in-use/duplicate/invalidity/incompatibility, live unavailability,
limits, local storage failure, and sanitized console failure
(`loomspan-console/internal/consolecore/errors.go:5-31`). MCP's current mapping
copies those values and never serializes the internal cause
(`loomspan-console/internal/mcpadapter/contracts.go:119-141`).

Application problems are normalized once in `applicationclient`: Java
authentication, invalid request/cursor, stale cursor, not found, limit, live
unavailability, access, and transport failures become the shared Go meanings
(`loomspan-console/internal/applicationclient/problem.go:8-55`,
`loomspan-console/internal/applicationclient/errors.go:70-100`). Artifact
processing produces `INCOMPATIBLE_ARTIFACT`, `INVALID_ARTIFACT`,
`LIMIT_EXCEEDED`, or `LOCAL_STORAGE_UNAVAILABLE` without an adapter-specific
meaning.

The MCP adapter currently registers no prompts, sampling, elicitation,
filesystem operations, target mutation, or execution control. Tool annotations
are closed-world hints, and descriptions identify YAML/activity as untrusted
diagnostic data. Raw artifacts and reconstructed payloads are only read from
closed logical bundle components selected by an authenticated owner-bound
handle; component names cannot be caller paths
(`loomspan-console/internal/mcpadapter/contracts.go:112-118`,
`loomspan-console/internal/mcpadapter/skills.go:28-40`,
`loomspan-console/internal/artifact/processor.go:13-55`, `:83-105`,
`loomspan-console/internal/artifact/lease.go:27-74`).

### 10. Java-to-Go protocol and fixtures

The Java observability adapter owns the authenticated current-instance trace
catalog and exact artifact stream. It pages catalog entries through a
startup-instance-bound cursor, verifies trace existence, admits artifact
delivery, leases the canonical core artifact, and emits
`application/x-ndjson` (`loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/observability/web/ObservabilityRestController.java:204-287`,
`:408-419`). Go's `applicationclient.OpenArtifact` requires the exact media
type, rejects compression, redirects, ranges and conditionals, validates the
application instance header, bounds problem bodies, and leaves successful
bytes streaming (`loomspan-console/internal/applicationclient/artifact.go:15-32`,
`:100-227`).

The protected protocol consumers are:

```text
Java canonical trace writer and finalized catalog
  -> Java observability trace JSON and NDJSON artifact endpoint
  -> Go applicationclient and observability services
  -> shared artifact service and trace-analysis processor/query service
  -> browser adapter today
  -> MCP adapter after PR 18 registration/mapping
```

The executable boundary corpus has Java-produced NDJSON traces and expected Go
semantic results. The Java `ConsoleTraceFixtureCorpusTest` protects writer
output and the Go fixture-corpus tests protect parsing, hierarchy, timing,
usage, failures, attempts, validation, gaps, uncertainty, chunks, compatibility,
and invalidity (`loomspan-console-fixtures/README.md:1-40`,
`loomspan-spring-boot-starter/src/test/java/com/lokiscale/loomspan/internal/runtime/trace/ConsoleTraceFixtureCorpusTest.java`,
`loomspan-console/internal/traceanalysis/fixture_corpus_test.go`). Browser JSON
fixtures additionally pin current trace-analysis DTOs byte-for-byte
(`loomspan-console/internal/browserapi/contracts_test.go:138-196`).

### 11. Existing test coverage relevant to PR 18

Current tests already cover the following shared behaviors:

- one joined target acquisition and stable handle reuse
  (`artifact/acquire_test.go:22-115`);
- imported publication, duplicate identity, size bounds, cancellation,
  target-rotation survival, and concurrent duplicate imports
  (`artifact/import_test.go:23-171`);
- TTL, successful/failed lease last-use refresh, deferred removal, `never`,
  expired clearing, LRU capacity, and in-use removal
  (`artifact/expiry_test.go`, `artifact/capacity_test.go`,
  `artifact/service_test.go`);
- target rotation, authentication rejection, credential replacement, instance
  change, shutdown, restart non-adoption, storage recovery, path/credential
  non-disclosure, and raw-byte checksums
  (`console/artifact_integration_test.go`, `artifact/storage_test.go`);
- trace queries, filters, ordering, cursor fingerprint mismatch, expired
  handles, physical/logical records, attempts/retries/validation/failures,
  payloads/gaps/uncertainties/usage, raw ranges, and cancellation
  (`traceanalysis/service_test.go`, `continuation_test.go`, `range_test.go`,
  `search_test.go`);
- browser imported upload and target-optional imported analysis
  (`browserapi/artifact_import_test.go`, `browserapi/trace_analysis_test.go`);
- current MCP schemas, structured/text envelopes, capability families,
  resources, target rotation, cancellation, multiple clients, authentication
  generation, and official protocol operation
  (`mcpadapter/*_test.go`).

There are currently no MCP trace golden files, browser/MCP trace parity tests,
trace-resource tests, joined browser/MCP acquisition tests, or PR 18
capability-family tests. The MCP golden inventory test deliberately accepts
only the six current artifacts (`loomspan-console/internal/mcpadapter/contracts_test.go:106-122`).

## Contract and Compatibility Classification

### Application API

No current or ticket-named Java application API type is involved. The supported
Java API remains the closed allowlist in
`LoomspanPublicSurfaceArchitectureTest`; the trace writer, observability web
adapter, and Console services are outside `com.lokiscale.loomspan.api`
(`loomspan-spring-boot-starter/src/test/java/com/lokiscale/loomspan/architecture/LoomspanPublicSurfaceArchitectureTest.java:27-37`).

### Supported SPI

The repository exposes no supported Java SPI or internal bean-replacement
surface for MCP or trace analysis. Go interfaces such as `artifact.Processor`,
the browser service interfaces, and MCP `ServerOptions` are internal
composition seams, not supported application SPIs
(`loomspan-console/internal/artifact/processor.go:57-81`,
`loomspan-console/internal/browserapi/router.go:21-82`).

### Configuration and manifest contracts

The existing user-visible configuration involved is the shared
`trace-workspace.max-bytes` and `trace-workspace.idle-ttl` contract, including
the `unlimited` and `never` sentinels. Defaults are 4 GiB and 4 hours. These
settings govern target and imported artifacts together
(`loomspan-console/internal/config/config.go:18-29`, `:56-74`,
`loomspan-console/README.md:378-395`). There is no MCP YAML section; MCP enabled
state remains the canonical key-file presence described by the PR 16 surface.

The analysis manifest and derived component names are process-local bundle
formats. They are not external import inputs and are never adopted after
restart (`loomspan-console/internal/traceanalysis/manifest.go`,
`loomspan-console/internal/traceanalysis/index_format.go`,
`loomspan-console/internal/artifact/service.go:472-515`).

### Persisted or serialized contracts

The Java REST/SSE/problem/NDJSON boundary is a coordinated Java-to-Go protocol
protected by the exact `consoleCompatibilityVersion`, current fixtures, and
same-release producer/consumer updates. A saved complete raw NDJSON trace is a
narrow same-version portable diagnostic artifact; it is not cross-version
history (`ai/thoughts/framework-feature-design-lens.md:29-61`,
`loomspan-console-fixtures/README.md:1-3`).

MCP tool names, capability IDs, strict input schemas, structured output fields,
resource URI shapes, error envelopes, and fallback text are externally
serialized MCP-facing behavior. The current four capability families and six
tools have executable and README evidence. The PR 18 names and capability
semantics currently exist only in the ticket and Phase 3 design, not production
registration.

The embedded browser API is an atomically distributed Go/browser protocol with
byte-for-byte fixtures. Its current target/imported source fields, handle
fields, ranges, cursors, and error meanings are verified in-repository
consumers, but it has no independent browser API version
(`loomspan-console/internal/browserapi/contracts_test.go:27-196`).

### Ephemeral diagnostic formats

Canonical traces, raw record addresses, bundle indexes, analysis manifests,
artifact handles, imported catalogs, trace cursors, browser analysis DTOs, and
MCP inspection results are current-run diagnostics. Handles, imports, indexes,
and cursors disappear on ordinary removal/expiry/shutdown/restart; target-owned
ones additionally disappear on target-scope rotation
(`loomspan-console/README.md:397-412`, `:452-467`).

### Internal or accidentally exposed implementation

All Go packages under `loomspan-console/internal`, Java types under
`com.lokiscale.loomspan.internal`, and Spring integration machinery are
implementation details. Their exported Go/Java declarations enable internal
package composition and framework wiring; visibility alone does not establish
an Application API or Supported SPI.

### Compatibility-marker and shim status

The current code has one exact release-string `consoleCompatibilityVersion`
for the Java-to-Go observability and trace boundary. Import preflight and full
processing both enforce it; trace records have no second MCP/schema version.
No legacy reader, compatibility range, MCP-specific trace format, or Java API
shim exists. Named MCP capability generations are separate semantic discovery
identifiers rather than replacements for the Java/Go release marker
(`loomspan-console/internal/traceanalysis/processor.go:37-85`, `:139-153`,
`ai/thoughts/phases/loomspan_console_phase_3_llm_runtime_inspector.md:850-905`).

## Architecture Documentation

The current ownership chain is:

```text
selected target catalog (`observability.Service`) ----+
                                                      |
browser import stream ------------------------------> artifact.Service
                                                        - TARGET/IMPORTED owners
                                                        - acquisition/import
                                                        - handle namespace
                                                        - capacity + TTL
                                                        - pins + removal
                                                        - installed bundle
                                                               |
                                                               v
                                                     traceanalysis.Service
                                                        - complete validation
                                                        - immutable indexes
                                                        - shared calculations
                                                        - handle-bound queries
                                                               |
                                               +---------------+---------------+
                                               |                               |
                                          browser API                    MCP adapter
                                          (implemented)                  (PR 18 absent)
```

The evidence owner is the central boundary. Target evidence couples owner
identity to `targetScopeId`; imported evidence uses one unexposed process-local
owner and no target scope. Handles are valid only for their owner. Query cursors
bind to the same owner and handle. This lets target invalidation remove only
target-derived evidence while global cache lifecycle treats both sources
uniformly.

Acquisition and analysis are separate phases but one installation transaction:
no query handle exists until raw-copy validation, semantic processing, derived
component sync, and atomic directory installation all succeed. Query calls pin
that immutable bundle for one finite request and refresh shared last use only
on success.

Browser and MCP remain peer adapters. The browser currently resolves a
`(source, traceId)` to a handle before calling analysis, while the ticket's MCP
downstream operations are handle-directed. Both paths meet at
`artifact.Service.Use` and `traceanalysis.Service`; neither service contains
browser or MCP SDK types.

## Historical Context (from `ai/thoughts/` and Git)

- `ai/thoughts/phases/loomspan_console_phase_3_llm_runtime_inspector.md:393-535`
  establishes the tool-first trace surface, progressive disclosure, separate
  application/local availability, shared artifact lifecycle, and exact range
  semantics.
- `ai/thoughts/phases/loomspan_console_phase_3_llm_runtime_inspector.md:537-593`
  establishes Go-owned mechanical hierarchy, duration, usage, failure,
  validation, gap, and uncertainty facts with no diagnosis or aggregate
  completeness model.
- `ai/thoughts/phases/loomspan_console_phase_3_llm_runtime_inspector.md:867-905`
  defines the two PR 18 capability families and their required tools.
- `ai/thoughts/phases/loomspan_console_workflows.md:51-90`, `:107-221`, and
  `:352-422` connect failed, expensive, and unfamiliar-skill investigations to
  the same acquired artifact, calculations, identifiers, and explicit raw
  detail.
- Git commit `945540f` (PR 12) introduced the centralized acquisition/cache/
  handle lifecycle; commit `406a981` (PR 13) introduced the parser, indexes,
  calculations, queries, ranges, and fixture protections; commit `b33d467`
  (PR 25) generalized evidence ownership and added same-version imports; commit
  `20e4a82` (PR 17) introduced the current MCP adapter patterns, continuations,
  resources, capability table, and parity tests. Their detailed plans and
  research were deliberately removed from the working tree; Git history is the
  retained historical source (`ai/thoughts/phases/2026-08-12-loomspan-active-roadmap.md:3-7`).

## Related Research

The current working tree contains no other files under `ai/thoughts/research/`.
Relevant removed research remains available in Git history:

- `20e4a82:ai/thoughts/research/2026-08-13-loomspan-console-pr-17-mcp-runtime-inspection.md`
- `b33d467:ai/thoughts/research/2026-08-12-PR-25-portable-trace-import.md`
- `406a981:ai/thoughts/research/2026-07-30-PR-13-trace-parser-indexes-shared-calculations.md`

## Open Questions

These items are not answered by current production code or executable
contracts:

1. The exact PR 18 input/output DTOs, required/optional fields, concise text
   fallbacks, and golden fixtures for the six ticket-named trace tools.
2. How one `LOOMSPAN_list_traces` result represents and pages the selected
   target's application catalog together with the unpaged process-local
   imported entries from `StorageSnapshot`, including exact source-filter
   behavior and ordering.
3. The exact `LOOMSPAN_get_trace` schema for the mutually exclusive `traceId`
   acquisition path and `artifactHandle` installed-copy path, including how
   source is selected without requiring a target for imported handles.
4. How the existing attempts, retries, validation, failures, diagnostics,
   usage, payload-descriptor, gap, uncertainty, and search services are exposed
   through the settled five-tool `loomspan.trace-inspection.v1` family without
   adding scenario-specific tools or adapter-side calculations.
5. Whether PR 18 returns trace-analysis cursors directly as opaque MCP
   continuations or wraps them in another adapter token, and whether the
   existing unsigned/base64 representation remains the final PR 18 wire form.
   The current trace cursor already binds owner, handle, operation, query
   fingerprint, and position.
6. The exact MCP per-result page and byte-range maxima. Current MCP collection
   pages stop at 64, while shared trace services allow 1,000 items and 1 MiB
   raw ranges; base64 results also expand in the final JSON response.
7. The artifact summary/frame/record resource URI grammar for imported
   evidence. The Phase 3 examples are target-scoped, while imported evidence
   deliberately has no `targetScopeId` and must not expose its internal owner
   ID.
8. The resource MIME types and exact materialized result/error mapping for
   summary, frame, and record resources, including the relationship between
   resource reads and the same successful-use TTL refresh as tools.
9. The exact capability-conformance fixture that proves both required-tool
   presence and the semantic promises of target/imported source identity,
   target-optional access, handle binding, availability, cancellation, and
   error parity. Current conformance checks tool-family membership only.
10. Whether `loomspan.raw-artifact-inspection.v1` is always compiled and
    advertised with the current Console binary or conditionally registered.
    The shared `ReadRawArtifactRange` method always exists; no MCP registration
    condition currently exists.
11. Which dated representative-client versions establish the final PR 18
    structured-content, resource-template, continuation, and broad-range
    interoperability limits. The compatibility document records this evidence
    as release-required and currently records no completed Codex native-client
    run (`loomspan-console/docs/mcp-client-compatibility.md:21-54`).

## Verification Performed

The following live packages passed at commit
`14a910ca159a1f67a536f0790d7d0553933f6791`:

```text
go test ./internal/mcpadapter ./internal/artifact ./internal/traceanalysis ./internal/browserapi ./internal/console
```

The research also ran `bash ai/scripts/spec_metadata.sh` and verified a clean
working tree before creating this document.

## Follow-up Research 2026-08-14T00:22:52-07:00

### Question

Determine which of the eleven open questions are already answered by the
active phase documents. For choices the phase documents deliberately leave to
implementation, recommend a direction based on the current artifact,
evidence, trace-analysis, browser, and MCP seams.

### Overall finding

The phase documents settle the product and semantic boundaries more completely
than the original open-question list suggested. They do **not** settle most
field spellings, pagination framing, MIME declarations, or dated client
limits. Those are explicitly left as implementation work
(`ai/thoughts/phases/loomspan_console_workflows.md:97-99`,
`ai/thoughts/phases/loomspan_console_phase_3_llm_runtime_inspector.md:978-1004`).

The questions classify as follows:

| # | Phase-document status | Result |
| --- | --- | --- |
| 1 | Partly answered | Tool purposes and common response requirements are settled; exact DTOs, text rendering, and goldens are implementation choices. |
| 2 | Partly answered | One shared inventory and separate application/local facts are settled; source filtering, merge order, and a composite cursor are not. |
| 3 | Mostly answered | Exactly one of `traceId` or `artifactHandle`, acquisition versus reopen behavior, and downstream handle use are settled; the source selector is not. |
| 4 | Partly answered | The facts, general tool family, and no-adapter-calculation rule are settled; the exact schema that maps the existing fact indexes into that family is not. |
| 5 | Partly answered | Continuations must be opaque, scope/handle/query bound, and must not leak application cursors; direct versus wrapped encoding and signing are not prescribed. |
| 6 | Explicitly unanswered | The phases require representative-client measurement before choosing one-response limits. |
| 7 | Partly answered | Target resource templates and materialized views are settled; imported URI grammar is absent. |
| 8 | Partly answered | Resource content, tool-first status, shared errors, and shared artifact lifecycle are settled; MIME spelling and exact resource envelopes are not. |
| 9 | Partly answered | Capability meanings and required semantic test cases are settled; the executable fixture organization is not. |
| 10 | Answered | The exact initial catalog includes raw inspection, while the skill treats it as optional. It is not a runtime availability flag. |
| 11 | Explicitly unanswered | Dated local-client validation is a release task, not a phase-design decision. |

### Resolutions and recommendations

#### 1. Tool DTOs, text fallbacks, and goldens

The phase settles all six trace tool names, their authoritative purposes, and
the common requirements for strict input schemas, structured output, concise
text fallback, observation time, stable identifiers, availability/truncation
facts, continuations, errors, and useful resource links
(`ai/thoughts/phases/loomspan_console_phase_3_llm_runtime_inspector.md:414-481`).
It explicitly leaves structured-output and size contracts to implementation
planning (`:978-1004`).

**Recommendation:** define six adapter DTO pairs rather than exporting
`traceanalysis` structs or MCP SDK types across package boundaries. Give every
artifact-backed result one common evidence context containing `source`,
optional `targetScopeId`, `artifactHandle`, `traceId`, `sessionId`, and
`observedAt`. Use closed enums and strict `oneOf` branches for mutually
exclusive inputs. Keep the text fallback deterministic, line-oriented, and
fact-complete for the finite result; it should contain no diagnosis or
instructions. Reuse the current `lineWriter` conventions rather than create a
second prose renderer (`loomspan-console/internal/mcpadapter/contracts.go:150-229`).

Commit schema and rendering goldens for each tool covering: target success,
imported success without a selected target, empty/final page, continuation,
maximum complete page or range, base64 content, each input branch, and a
representative shared domain error. Also assert strict-schema rejection of
unknown fields and invalid branch combinations. Browser/MCP parity tests should
start from the same neutral result rather than compare separately assembled
fixtures, as required by Phase 3 (`:118-130`).

#### 2. `LOOMSPAN_list_traces` merge, filtering, ordering, and pagination

Phase 3 answers the important semantic part: return one inventory and keep
application-catalog availability, local acquired-copy availability, artifact
handle, and parsing/acquisition status as independent facts
(`ai/thoughts/phases/loomspan_console_phase_3_llm_runtime_inspector.md:441-458`).
Portable imported evidence must remain usable without a target. The current
code does not yet provide that inventory: the application list is paged, while
`StorageSnapshot` is an unordered map projection and deliberately omits handles
(`loomspan-console/internal/observability/service.go:151-174`,
`loomspan-console/internal/artifact/service.go:361-402`,
`loomspan-console/internal/artifact/model.go:157-176`).

**Recommendation:** add a transport-neutral trace-inventory service below both
adapters; do not join these catalogs in `mcpadapter`. Its source filter should
be the closed enum `ALL`, `TARGET`, or `IMPORTED`, with `ALL` as the default.
`IMPORTED` must never capture a target. `ALL` with no selected target should
return imported entries plus an explicit unavailable application-catalog fact;
`TARGET` with no target should return the existing target-selection domain
error.

Use a segmented, deterministic order that does not require reading the entire
application catalog into memory:

1. installed copies, ordered by `finalizedAt` descending, then `source`
   (`TARGET` before `IMPORTED`), then `traceId` ascending; and
2. current application-catalog entries not presently installed, preserving
   application cursor order.

Resolve each installed snapshot entry through `artifact.Service.Lookup` to
obtain its opaque handle rather than weakening the storage-snapshot contract.
The composite continuation should bind filter and target scope, record the
current segment, carry a keyset position for installed entries, and carry the
application cursor only inside the opaque composite token. On a later page,
skip application items that are currently installed. The inventory is a
weakly consistent observed view, so mutation between pages may change
availability, but keyset pagination avoids the offset-shift behavior of the
current unordered snapshot. This creates no MCP-owned catalog and keeps all
entries addressable through finite calls.

#### 3. `LOOMSPAN_get_trace` input and target-optional imports

Phase 3 settles that the input accepts exactly one of `traceId` or
`artifactHandle`; `traceId` acquires or reuses the selected application's copy,
while `artifactHandle` reopens the immutable copy without application access.
All later tools use the handle (`:456-458`).

**Recommendation:** require a separate `source: TARGET|IMPORTED` discriminator
and a strict `oneOf` for the two identifiers:

- `TARGET + traceId` captures the current target and acquires/reuses it;
- `TARGET + artifactHandle` captures the current scope and reopens that copy;
- `IMPORTED + artifactHandle` uses `evidence.ForImported()` and never captures
  a target; and
- `IMPORTED + traceId` is invalid because an import is already an installed
  copy and must be selected by the handle returned from inventory/import.

Do not accept `targetScopeId` from the client and do not search all owners by
handle. The adapter-safe `evidence.Reference` already models exactly this
choice, and `artifact.Service.Use` deliberately requires both reference and
handle (`loomspan-console/internal/evidence/owner.go:11-45`,
`loomspan-console/internal/artifact/service.go:265`). Return the common evidence
context so every continuation and later call can preserve source identity.

#### 4. Mapping all existing trace facts into the five-tool capability

The phases settle the five required trace tools, prohibit scenario-specific
helpers, require Go-owned calculations, and require flat failure/validation
facts with no diagnosis (`:429-468`, `:537-593`, `:882-905`). The current shared
service already exposes separate typed queries for attempts, retries,
validations, failures, diagnostics, payload descriptors, gaps, uncertainties,
usage, and search (`loomspan-console/internal/traceanalysis/query_facts.go:24-653`,
`query_diagnostics.go:16-96`, `search.go:33-260`). The plain record result does
not carry those complete typed facts even though its filter can reference some
of their IDs (`query_records.go:20-63`, `dto.go:56-89`).

**Recommendation:** preserve the settled authoritative purpose of
`LOOMSPAN_query_trace_records`: it should always return canonical record
summaries, not unrelated fact types hidden behind a `view` switch. Add a
transport-neutral enriched-record query below both adapters that joins the
existing indexes once inside `traceanalysis`. Its result should add optional
typed `attempt`, `retry`, `validation`, `failure`, `payload`, and search-match
facts to the canonical record that owns them. The tool's existing structured
filters select by those fact identifiers and text matches. This makes each
fact finitely traversable in canonical record order without creating a
failure-specific tool or asking the MCP adapter to calculate relationships.

Map the other facts as follows:

- `LOOMSPAN_get_trace` returns the existing `TraceSummary`, including roots,
  counts, attributed/terminal/unattributed usage, and completeness;
- `LOOMSPAN_query_trace_frames` returns `FrameSummary`, including hierarchy,
  durations, usage, attempt/retry/validation/failure identifiers, and the gap
  or uncertainty kinds mechanically attributed to that frame or its attempts;
- the enriched canonical records expose flat attempt, retry, validation,
  failure, payload-descriptor, and search-match facts from the existing
  indexes; and
- `LOOMSPAN_read_trace_payload` consumes an opaque content reference issued by
  record/payload/failure results.

Generalize that opaque payload reference inside the shared analysis layer so
it can address reconstructed record data and a failure diagnostic by
`failureId`/ordinal as well as a payload-store entry. Add a bounded range read
for diagnostic text rather than returning a potentially 1 MiB diagnostic from
`GetFailureDiagnostic` in an unpageable query result. Keep the wire name
`payloadRef`, because the phase already defines it as opaque; clients must not
infer the underlying component. This is the one recommended shared-service
addition needed for tool-complete diagnostic access.

#### 5. Trace continuations, wrapping, and signing

Phase 3 requires continuations to remain opaque, scope/handle/query bound, and
not expose application pagination cursors as interchangeable tokens
(`:470-487`, `:515-523`). The current trace cursor already validates schema,
operation, owner, handle, query fingerprint, and position, but its decoded
`ownerKey` includes the process-local imported owner ID
(`loomspan-console/internal/traceanalysis/cursor.go:45-56`, `:173-186`,
`:262-264`).

**Recommendation:** return trace-analysis continuations directly; do not wrap
them in another base64 JSON token. Before doing so, change the imported owner
cursor key to the adapter-safe `IMPORTED` reference rather than the internal
owner ID. The unique handle, operation, fingerprint, and installed-copy
lifetime already provide the necessary binding. Retain strict unpadded
base64url JSON and no HMAC. The local endpoint is authenticated, cursor
tampering grants no new authority, every decoded field is revalidated, and a
signature would add key lifecycle without protecting evidence. This matches
the threat model already chosen for PR 17 continuations. Add a hard encoded
token-length limit at the adapter boundary and tests for unknown fields,
wrong source, wrong handle, wrong query, out-of-range state, and malformed
base64.

The composite inventory cursor is different: it should be an MCP/inventory
token because it coordinates local keyset and application-cursor state. It
must not be passed to a trace-analysis query or expose the upstream cursor as
a top-level field.

#### 6. MCP page and byte-range maxima

The phase documents deliberately do not choose these values. They say that
representative-client validation determines the largest interoperable result
and specifically reject guessing a `100`-record or `256 KiB` design default
(`:470-487`, `:828-846`). Current code provides useful test points: MCP pages
are 64 items, shared trace pages permit 1,000, shared ranges default to 64 KiB
and permit 1 MiB (`loomspan-console/internal/mcpadapter/contracts.go:18`,
`loomspan-console/internal/traceanalysis/limits.go:19-33`).

**Recommendation:** keep 64 as the candidate global MCP page maximum because
it is already fixture-tested, but make the range limit an empirical release
gate. Test raw byte requests of 64 KiB, 256 KiB, and 1 MiB with both UTF-8 and
worst-case base64 through every required client. Select the largest value that
all required clients can receive, render, and continue without truncation or
timeout, never exceeding the shared 1 MiB limit. Report the selected maximum
in tool descriptions/results and return explicit `LIMIT_EXCEEDED` details for
larger requests. Do not silently clamp. If the client matrix is unavailable,
the question remains open rather than converting 256 KiB into an unevidenced
contract.

#### 7. Imported resource URI grammar

The target templates and materialized meanings are settled at `:401-412`, but
there is no imported grammar.

**Recommendation:** preserve the settled target URIs and add parallel imported
templates:

```text
loomspan://imports/artifacts/{artifactHandle}/summary
loomspan://imports/artifacts/{artifactHandle}/frames/{frameId}
loomspan://imports/artifacts/{artifactHandle}/records/{sequence}
```

The plural `imports` parallels `targets`; the URI exposes neither a fabricated
target scope nor the process-local imported owner ID. The handler constructs
`evidence.ForImported()`. Keep skill URIs unchanged and do not add a raw
artifact resource.

#### 8. Resource MIME, result/error mapping, and TTL refresh

The phases settle that summary/frame/record resources are immutable
materialized views, that a record resource contains the logical envelope plus
payload descriptor without expanding a large payload, and that resources are
supplementary (`:393-412`). They also require resource failures to preserve the
same shared domain codes and safe details as tools (`:489-513`). Shared leases
refresh last use only on successful close, so this behavior should not be
reimplemented in a resource handler.

**Recommendation:** return UTF-8 JSON with MIME `application/json` and the same
adapter DTO used by the corresponding tool item; do not invent
resource-specific models or text fallbacks. Summary maps `GetSummary`, frame
maps an exact-ID frame query requiring exactly one result, and record maps an
exact-sequence `LOGICAL` record query. Each handler must call the shared
analysis service, which pins the artifact and refreshes TTL only after a
successful materialization. Map shared failures through the existing
`resourceDomainError` path; malformed URI/template arguments remain protocol
invalid-parameter errors. Assert that success refreshes TTL, failure and
cancellation do not, removal/expiry invalidates both tools and resources, and
imported reads do not capture a target.

#### 9. Capability-conformance fixture

Phase 3 settles that a capability promises all listed tools **and semantics**,
and that advertising an incomplete capability is a conformance defect
(`:871-905`). Its required coverage already names target/imported lifecycle,
parity, cancellation, multi-client, exact calculations, failure/validation
membership, continuation, and handle expiration (`:1007-1066`). The current
test checks only family membership.

**Recommendation:** create a reviewed capability-contract manifest used by
tests, with each capability's exact required tool names and stable semantic
fixture IDs. Keep runtime advertisement derived from the production capability
table; do not run probes or make capabilities depend on current target state.
For `loomspan.trace-inspection.v1`, execute the manifest's fixture suite against
the assembled MCP server for:

- target catalog acquisition and imported inspection with no target;
- explicit source identity and wrong-source/handle/cursor rejection;
- separate application/local availability facts;
- browser/MCP parity for summary, duration, usage, failure, and validation;
- complete continuation traversal, UTF-8/base64 ranges, and expiry;
- cancellation, concurrent clients, and shared error preservation; and
- strict schemas and deterministic structured/text goldens.

For `loomspan.raw-artifact-inspection.v1`, require exact byte preservation,
range continuation, both evidence sources, lifecycle/error parity, and no new
acquisition. A test must remove one required tool and independently break one
semantic fixture, proving that either defect fails conformance even though the
capability string can still be assembled.

#### 10. Raw capability registration

This is answered by the phase document. The exact initial capability catalog
contains `loomspan.raw-artifact-inspection.v1`, and its required tool is
`LOOMSPAN_read_trace_artifact`; “optional” describes the portable skill's
behavior against another compatible server, not a runtime toggle in this
Console (`:871-895`).

**Recommendation:** register the tool and advertise the capability
unconditionally in the standard Console binary. Do not add configuration or
change advertisement based on target, authentication, trace availability, or
storage state. A future intentionally reduced build would omit both the tool
and capability at assembly time and would need its own tested distribution
contract.

#### 11. Dated representative-client versions

The phase explicitly leaves this to implementation and names local Codex,
Claude Code, Antigravity, Cursor, and Devin Desktop/Windsurf or local Devin CLI
as the matrix (`:978-988`). The repository currently records only a failed
Codex CLI 0.130.0 run on Windows x86_64 for PR 17; native Codex desktop and all
other representative rows remain unrun
(`loomspan-console/docs/mcp-client-compatibility.md:21-54`). Therefore the phase
docs cannot supply final PR 18 versions or limits.

**Recommendation:** update the existing compatibility document rather than
create a PR 18-only matrix. For each release candidate, record the then-current
stable client build, OS, configuration mechanism, observed protocol, and exact
pass/fail notes for tool/output-schema discovery, structured content, text
fallback, domain `isError`, all six trace calls, target and no-target imported
flows, resource-template discovery/read, continuation round trips, 64-item
pages, and each candidate UTF-8/base64 range size. Hosted clients remain out of
scope because they cannot reach loopback. The final framing constants should
cite these dated rows.

### Follow-up conclusion

Questions 10 and the core of 3 are already decided. Questions 1, 2, 4, 5, 7,
8, and 9 are constrained implementation design; the recommendations above fit
the settled surface and current code without adding a public Java API, a new
MCP-owned catalog, scenario tools, or adapter-side calculations. Questions 6
and 11 require empirical client evidence and should remain explicit release
gates until that evidence exists.

## Follow-up Research 2026-08-14T00:32:15-07:00

### Range-size clarification

The earlier 64 KiB, 256 KiB, and 1 MiB test ladder stopped at 1 MiB only
because `traceanalysis.maxRangeBytes` is currently 1 MiB
(`loomspan-console/internal/traceanalysis/limits.go:29-33`). Neither the phase
design nor the intended complete-evidence workflow establishes 1 MiB as the
right MCP maximum. Treating the existing constant as the final product limit
would be circular.

A finite per-result maximum is still necessary because an MCP tool result is
one materialized JSON-RPC value rather than a streaming byte response. The Go
service, SDK/JSON encoder, HTTP stack, client, and often the client-to-model
bridge may each hold a representation of it. Arbitrary bytes expand by roughly
one third under base64 before JSON and protocol overhead. An unbounded request
would therefore permit one authenticated local caller to make an unbounded
allocation and could terminate the Console. That is a one-response safety
bound, not a cumulative evidence, traffic, or model-context quota;
continuation still makes every byte addressable.

**Revised recommendation:** retain a modest default range for progressive
disclosure, but test and support a substantially larger caller-selected
maximum. The release matrix should exercise source-byte windows of at least
1 MiB, 4 MiB, 16 MiB, and 32 MiB, with a 64 MiB case when the required clients
and host-memory tests remain healthy. Run every size as UTF-8 and worst-case
base64, through continuation and cancellation, and under representative
simultaneous browser/MCP use. Select the largest value that all required local
clients receive exactly and that keeps bounded server memory and latency under
the documented request deadline. Raise the shared `traceanalysis` maximum to
that value; do not add a lower MCP-only clamp.

The likely useful target is at least 16 MiB per call, not 1 MiB. The final
number should remain evidence-driven rather than being fixed in this research
without the client and memory measurements. Results should state the maximum
in source bytes and the actual returned range so base64 expansion is never
mistaken for lost or truncated evidence.

## Follow-up Research 2026-08-14T00:34:30-07:00

### Future-context handoff audit

The ticket was compared with the completed code research, phase resolution,
and accepted recommendations. Its original detailed-planning section still
listed the topics as open, so a future planning context could have missed the
accepted direction or repeated this research.

The durable handoff was copied into
`ai/thoughts/tickets/loomspan-console-pr-18-mcp-trace-inspection.md` under
`Research handoff`. It records the common evidence DTO context, target-optional
source schema, non-retained shared inventory query, canonical-record enrichment,
ranged diagnostic `payloadRef`, direct cursor use after removing the imported
owner-ID exposure, imported resource grammar, shared resource lease/error
behavior, unconditional raw capability, semantic conformance fixtures, revised
large-range validation, and the dated representative-client evidence gate.

No other discovered code or phase constraint needs promotion into the ticket.
The following remain intentionally for implementation and test planning rather
than further research: exact additive field spelling, the concrete composite
inventory-cursor JSON, the chosen measured response maximum, fixture file
locations, test-workstream sequencing, and the dated client builds available
to the release candidate.
