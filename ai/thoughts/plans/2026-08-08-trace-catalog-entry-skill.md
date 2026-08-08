# Make Entry Skill Required Trace Identity Implementation Plan

## Overview

Implement PR 21 by establishing the requested top-level YAML skill name as required, bounded session identity before observation and trace construction, then carrying the same string through live snapshots, finalized Java metadata, the application REST contract, Go acquisition metadata and cached fallback, and the React Trace Catalog and Trace Detail views.

The plan changes current internal signatures and current-release DTOs in place. It does not infer identity from artifacts, create synthetic frames, add compatibility overloads, or change canonical NDJSON. The implementation is planned against repository commit `ca3e611cbad0a6cc84f6883924ca91e2880f87af` on `main`.

## Current State Analysis

- `DefaultSkillTemplate` resolves and validates the YAML capability before starting a session, but calls `LoomspanSessionRunner` without `capability.name()` (`loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/skillapi/DefaultSkillTemplate.java:91-107`).
- `LoomspanSessionRunner` and `LoomspanSession` construct observation and tracing with a session ID but no entry-skill identity (`loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/core/LoomspanSessionRunner.java:112-181`, `LoomspanSession.java:165-205`).
- `DefaultExecutionTraceHandle` publishes `TRACE_STARTED` during construction. `ExecutionProjectionState.entrySkill` is initially null and `LiveActivityProjector` fills it from the first `ROOT_MISSION`, so the first visible snapshot can be missing a value that Go requires (`DefaultExecutionTraceHandle.java:243-257`, `LiveActivityProjector.java:72-85`, `loomspan-console/internal/observability/service.go:297-300`).
- The runner can finalize an action that opens no frame, and access checks can reject a capability before a root mission frame is opened (`LoomspanSessionRunner.java:186-240`, `CapabilityExecutionRouter.java:51-81`, `ExecutionCoordinator.java:65-96`).
- `FinalizedTraceArtifact`, `FinalizedTraceCatalogEntry`, and `ObservabilityDtos.Trace` do not include entry skill (`FinalizedTraceArtifact.java:9-17`, `FinalizedTraceCatalogEntry.java:11-24`, `ObservabilityDtos.java:66-73`).
- Go `observability.Trace`, its validator, `artifact.TraceMetadata`, production acquisition wiring, and `browserapi.cachedTrace` omit the value (`loomspan-console/internal/observability/dto.go:78-91`, `service.go:313-323`, `internal/artifact/model.go:26-39`, `internal/console/service.go:163-178`, `internal/browserapi/observability.go:229-281`).
- TypeScript `Trace`, Trace Catalog, Trace Detail, unit fixtures, E2E mocks, and the two application REST fixtures omit the field (`loomspan-console/web/src/api/contracts.ts:157-170`, `web/src/observability/Traces.tsx:48-79`, `TraceDetail.tsx:142-152`).
- The existing public-name contract already requires exact 1-64-character YAML names. PR 21's 256-code-point session identity boundary is therefore non-lossy for registered YAML entry skills and also covers explicit standalone/internal entry routes (`ai/skill-authoring/mental-model.md`, “Public YAML Skill Identity”).

## Desired End State

Every newly constructed session receives a non-null, nonblank entry skill. A single core-owned normalization helper bounds it to 256 Unicode code points without trimming or otherwise rewriting it. The normalized string is stored once on the session and passed unchanged into observation and trace construction before `TRACE_STARTED` is emitted.

For one session, the following surfaces expose byte-for-byte equal `entrySkill` values:

1. first and subsequent active-execution snapshots;
2. the core finalized artifact descriptor;
3. the in-memory finalized catalog entry;
4. application REST trace list and detail responses;
5. Go `observability.Trace`;
6. acquisition-time `artifact.TraceMetadata`;
7. reachable and installed-copy fallback trace list/detail responses;
8. the TypeScript `Trace` contract, Trace Catalog row, and Trace Detail facts.

The top-level resolved capability name is checked against the session identity before its root frame opens. Nested root frames neither replace the entry skill nor have to match it. Access-denied-before-root and standalone-no-frame paths retain their supplied identity and finalize without synthetic frames. Canonical trace records, trace-analysis fixtures, journal projection, retention, acquisition, and eviction semantics remain unchanged.

### Key Discoveries

- Observation is created before tracing, but tracing publishes immediately; both factories must receive identity at construction time (`LoomspanSession.java:192-205`).
- Optional observation failures are fail-closed inside `DefaultExecutionObservationHandle` and do not change canonical execution outcome; the entry-skill consistency work must preserve that isolation (`DefaultExecutionObservationHandle.java:55-105`).
- Catalog publication is driven only by the core-issued `FinalizedTraceArtifact`, and duplicate/conflict behavior uses record equality (`InMemoryFinalizedTraceCatalog.java:66-119`).
- Browser fallback reconstructs trace DTOs from `artifact.Service.Lookup` metadata rather than `StoredEntry`; retained acquisition metadata is therefore the installed-copy identity source (`browserapi/observability.go:229-281`).
- Java generates the application REST corpus and Go consumes it. The required JSON change must be regenerated and committed in lockstep (`ConsoleRestFixtureCorpusTest.java:75-89`, `loomspan-console/internal/observability/dto_test.go:126-148`).

## What We're NOT Doing

- No backfill for traces already cataloged in a running process.
- No artifact scan, path resolution, automatic acquisition, or NDJSON parsing to label catalog rows.
- No synthetic `ROOT_MISSION` for denied, mapped, or standalone execution.
- No nullable finalized field, optional Go field, or browser fallback derivation.
- No compatibility overload, default entry name, legacy JSON reader, alias, bridge, or dual behavior.
- No canonical trace record, `TRACE_STARTED` payload, trace-format version, trace-analysis corpus, or journal semantic change.
- No change to trace persistence policies, completion grace, catalog TTL, acquisition behavior, target-scope cleanup, capacity, idle eviction, or restart cleanup.
- No duplicate entry-skill field on the acquired-artifact response or Trace Storage/`StoredEntry` DTO.
- No link from recorded entry skill to the current registered-skill catalog.
- No MCP implementation; later adapters consume the enriched shared trace service.
- No unrelated activity or console UI cleanup. Existing `formatDateTime`, focus, pagination, table-region, and scope-binding behavior is retained.

## Skill-Authoring Documentation Impact

**Impact**: Affected

- **Rationale**: YAML authoring syntax, public name validation, invocation semantics, RBAC, and nested execution behavior do not change. The author-facing debugging contract does change: the Console will expose the exact top-level YAML invocation name as immutable recorded trace identity before acquisition, and nested skills will not replace it. That belongs in the existing trace-debugging topic.
- **Documents to update**: `ai/skill-authoring/traces-and-debugging.md` only.
- **Supporting evidence**: session identity and top-level consistency tests in `LoomspanSessionRunnerTest`/`ExecutionCoordinatorTest`; first-snapshot tests in `DefaultExecutionObservationHandleTest`; REST corpus tests; Go fallback tests; Trace Catalog and Trace Detail component tests. `ai/skill-authoring/mental-model.md` already defines the exact YAML name as the shared identity used by invocation and traces.
- **Coverage table update**: Not required. No topic is added and the existing “Traces and debugging — Source-verified” boundary and confidence remain accurate.
- **LLM-first usability**: Add a compact “Trace identity” rule near applicability/debugging procedure: `entrySkill` is the exact recorded top-level YAML name for the session, remains stable across nested invocations, is catalog metadata rather than evidence of current registration, and can label a trace without acquisition. Link to existing implementation/test anchors instead of duplicating manifest-name guidance.

## Contract and Compatibility Impact

| Surface | Classification and supporting evidence | Planned compatibility treatment |
| --- | --- | --- |
| Application API | `SkillTemplate.invoke` is the supported entry point. Its signatures and caller behavior remain unchanged; `DefaultSkillTemplate` internally supplies the already-resolved capability name. | Preserve. No application caller change. |
| Supported SPI | No affected supported SPI was identified. Observation and trace factories are architecture-allow-listed for internal package composition, not supported replacement points. | No supported SPI change. |
| Configuration and manifest contracts | Exact YAML `name`, validation, lookup, and nested visibility semantics remain unchanged. The entry identity originates from `capability.name()`. | Preserve exactly; no manifest migration or sample change. |
| Persisted or serialized contracts | Application trace list/detail JSON and the Go/browser `Trace` JSON gain required `entrySkill`. They are executable current-release cross-component contracts, not durable cross-version storage. Installed `TraceMetadata` gains the same in-memory field. | Intentional atomic current-release break; Java, Go, TypeScript, mocks, and REST fixtures change together. Existing transient entries are not migrated. |
| Ephemeral diagnostic formats | Active snapshots become non-null from the first record. Canonical NDJSON and trace-analysis inputs/expected outputs remain byte-for-byte unchanged. | Keep writer, reader, projector, and current-run tools coherent; do not add a record field, version, or legacy reader. |
| Internal or accidentally exposed implementation | `LoomspanSessionRunner`, `LoomspanSession`, observation/trace factory signatures, `DefaultExecutionTraceHandle` constructors, finalized records, and internal test helpers change. The architecture allow-list explicitly identifies these public types as internal cross-package composition. | Update signatures and all repository callers atomically; remove entry-skill-free paths and add no shims. |

- **Evidence of supported contracts**: public `SkillTemplate`; `ai/skill-authoring/mental-model.md`; architecture allow-list descriptions in `LoomspanPublicSurfaceArchitectureTest.java:82-100` and `:184-210`; Java REST fixture generator; committed application REST corpus; Go decoder/service tests; TypeScript/browser consumers; approved PR 21 ticket.
- **Intended breaks**: required entry-skill parameters on internal session/runner/factory/trace constructors; required record/struct components on finalized Java and Go trace metadata; required `entrySkill` JSON in application and browser trace responses. Missing/empty upstream values become invalid.
- **In-repository consumers to update**: `DefaultSkillTemplate`; Spring-created session composition; all direct runner/session/trace constructors; Java observation/catalog/REST tests; Go observability, artifact, console, browser, and trace-analysis test literals; TypeScript view fixtures; three E2E response builders; the two application REST fixtures; trace-debugging guidance.
- **Public-surface delta**: existing public internal Java constructors and `ExecutionObservationHandleFactory#create` gain a required `String entrySkill`; public internal Java records gain a component; Go and TypeScript DTOs gain a field. Add only a package-private core normalization helper and package-private session accessor. No new public Java type, bean, `@ConditionalOnMissingBean`, configuration property, manifest field, endpoint, or Spring extension point.
- **Shim decision**: **No shim.** The repository is pre-release, the trace catalog is current-process-only, installed state is transient, and Java/Go/browser versions are shipped together. Every old construction and response shape is replaced atomically.
- **Java-to-Go boundary coordination**: **Required.** Add `entrySkill` to Java list/detail DTOs and fixture generation, regenerate only `application-rest/traces-page.json` and `trace-detail.json`, require the field in Go decoding/validation, copy it into acquisition metadata, return it through cached fallback, and update TypeScript/browser mocks in the same change.
- **Compatibility-marker decision**: Keep the existing exact product-version/`consoleCompatibilityVersion` release gate. Do not add or change an independent trace-format or field-level marker for PR 21; the required DTO change ships as part of the same release version.

## Implementation Approach

Use one package-private core helper, `EntrySkillIdentity`, as the only normalization point. Its `normalize(String)` method rejects null and blank values and truncates by Unicode code points to a core-owned limit of 256. It does not trim, case-fold, alias, sanitize, or otherwise rewrite the string. Normal registered YAML names already fit within this bound.

Pass the normalized `String` explicitly through constructors and factories. Downstream layers defensively require nonblank input but do not truncate it again. `LoomspanSession` stores the value in a final field and exposes a package-private accessor used only for the `ExecutionCoordinator` top-level consistency check. The identity is not added to session JSON.

Seed `ExecutionProjectionState` with the entry skill. `LiveActivityProjector` stops selecting identity from root frames; for a root frame opened while the projected frame stack is empty, it checks that the route equals the pre-seeded identity. Nested roots update the active path and usage but never mutate identity. `ActiveExecutionSnapshot` changes from nullable/blank-to-null normalization to required nonblank preservation.

`DefaultExecutionTraceHandle` stores the same supplied value and copies it to `FinalizedTraceArtifact`. Catalog and REST mapping then copy only from the core descriptor. Go follows the same explicit copy chain into installed metadata and fallback. No layer reads optional live projection state or canonical artifact content to recover it.

Implementation should begin with the dedicated testing plan from `ai/commands/3_testing_plan.md`, then add focused failing tests for the invariant before changing signatures. Because the repository change is atomic, intermediate compilation breakage is expected while construction sites move; keep each phase locally coherent before running its focused suite.

## Phase 1: Establish Required Core Session Identity

### Overview

Create the single normalization boundary, require identity on every session-runner and session construction path, pass the resolved YAML name from `DefaultSkillTemplate`, and assert top-level coordinator consistency.

### Changes Required

#### 1. Core-owned entry identity normalization

**Files**:

- `loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/core/EntrySkillIdentity.java` (new, package-private)
- `loomspan-spring-boot-starter/src/test/java/com/lokiscale/loomspan/internal/core/EntrySkillIdentityTest.java` (new)

**Changes**:

- Define the sole 256-code-point entry identity bound in core.
- Reject null and Unicode-blank input before any observation or trace object is constructed.
- Return input unchanged when it is within the bound; truncate at a code-point boundary when it exceeds the bound.
- Test null, empty, whitespace-only, exact-bound, over-bound, and supplementary Unicode code points.

```java
final class EntrySkillIdentity {
    static final int MAX_CODE_POINTS = 256;
    static String normalize(String value) { /* validate once, bound by code point */ }
}
```

#### 2. Require identity in the runner and session

**Files**:

- `loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/core/LoomspanSessionRunner.java`
- `loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/core/LoomspanSession.java`
- `loomspan-spring-boot-starter/src/test/java/com/lokiscale/loomspan/internal/core/TestLoomspanSessions.java`

**Changes**:

- Change every `runWithNewSession`/`callWithNewSession` overload to require entry skill, with no old overload retained. Use a consistent identity-first order, e.g. `callWithNewSession(String entrySkill, @Nullable Authentication authentication, Function<...> action)`.
- Change every `LoomspanSession` constructor to require entry skill. Normalize in the deepest constructor before observation construction, store the result in a final field, and pass that exact value to both factories.
- Add a package-private `entrySkill()` accessor for core consistency checks; do not add a JavaBean getter or serialized session property.
- Change `TestLoomspanSessions` helper signatures so callers supply an explicit entry route; do not hide a default in the helper.
- Update all direct session and runner call sites. Standalone/internal tests should use a stable explicit route such as `test.entry` or the actual tested skill name.

Direct runner-call groups include core runner/property tests, linter and output-schema advisor tests, evidence tests, advisor resolver tests, and observation concurrency tests. Direct session-constructor groups include coordinator/router tests, attachment materialization tests, session state/holder tests, and the shared test helper.

#### 3. Supply the authoritative application entry name

**File**: `loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/skillapi/DefaultSkillTemplate.java`

**Changes**:

- After `requireYamlSkill` and input validation, pass `capability.name()` into the runner together with authentication.
- Preserve existing exception, observer, validation, and security-context behavior.

#### 4. Check the resolved top-level capability

**Files**:

- `loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/core/ExecutionCoordinator.java`
- `loomspan-spring-boot-starter/src/test/java/com/lokiscale/loomspan/internal/core/ExecutionCoordinatorTest.java`

**Changes**:

- Reuse the existing `topLevelInvocation` decision made from the initially empty frame stack.
- Before access checking and root-frame opening for a top-level coordinator invocation, require `rootCapability.name().equals(session.entrySkill())`; fail visibly on mismatch.
- Do not apply this equality check to nested invocations.
- Test matching top-level identity, mismatched top-level identity before frame creation, and a different nested root that leaves the session entry skill unchanged.

#### 5. Protect root-free paths

**Files**:

- `loomspan-spring-boot-starter/src/test/java/com/lokiscale/loomspan/internal/core/LoomspanSessionRunnerTest.java`
- `loomspan-spring-boot-starter/src/test/java/com/lokiscale/loomspan/internal/skillapi/DefaultSkillTemplateTest.java`

**Changes**:

- Extend standalone success/failure tests to assert their explicit entry route reaches both factories/finalized metadata while no frame is opened.
- Add a restricted top-level skill case where access is denied before root-frame creation and the retained error trace still finalizes with the requested entry identity.
- Assert no synthetic root frame appears and the original `AccessDeniedException` behavior is preserved.

### Success Criteria

#### Automated Verification

- [x] Focused core tests pass: `./mvnw.cmd -pl loomspan-spring-boot-starter -Dtest=EntrySkillIdentityTest,LoomspanSessionRunnerTest,LoomspanSessionTest,ExecutionCoordinatorTest,DefaultSkillTemplateTest test -DfailIfNoTests=false`
- [x] Null/blank session creation fails before observation or trace factory invocation.
- [x] Over-bound Unicode input is normalized once and both factories receive the same byte-for-byte value.
- [x] Standalone/no-frame and access-denied-before-root traces retain identity without a root frame.
- [x] All direct runner/session callers compile with explicit identity.

#### Manual Verification

- [ ] Inspect a denied invocation trace and confirm it contains no invented `ROOT_MISSION`.
- [x] Confirm no application-facing `SkillTemplate` signature or YAML manifest syntax changed.

---

## Phase 2: Pre-seed Live Observation and Finalized Java Metadata

### Overview

Carry the normalized identity into observation before the first canonical record and into the core finalized descriptor, catalog, and REST DTO.

### Changes Required

#### 1. Change observation factory signatures and seed projection state

**Files**:

- `internal/runtime/observation/ExecutionObservationHandleFactory.java`
- `internal/runtime/observation/NoOpExecutionObservationHandleFactory.java`
- `internal/runtime/observation/DefaultExecutionObservationHandleFactory.java`
- `internal/runtime/observation/DefaultExecutionObservationHandle.java`
- `internal/observability/ObservabilityActivationCoordinator.java`
- `internal/runtime/observation/ExecutionProjectionState.java`

All paths are beneath `loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/`.

**Changes**:

- Change `create(String sessionId)` to `create(String sessionId, String entrySkill)` everywhere, including enabled, disabled, and activation-delegating paths.
- Require nonblank entry skill defensively but do not truncate it outside core.
- Construct `ExecutionProjectionState(sessionId, entrySkill)` and make its identity final/non-null rather than mutable discovery state.
- Preserve disabled-observation behavior and activation switching.

#### 2. Make live snapshots required from the first record

**Files**:

- `internal/runtime/observation/LiveActivityProjector.java`
- `internal/runtime/observation/ActiveExecutionSnapshot.java`

**Changes**:

- Remove first-root assignment from `LiveActivityProjector`.
- When a `ROOT_MISSION` opens against an empty projected frame stack, verify its route matches the pre-seeded entry skill before adding it. Allow nested root routes to differ and never mutate identity.
- Remove `@Nullable` and blank-to-null behavior from `ActiveExecutionSnapshot.entrySkill`; require nonblank input and preserve it exactly without a second truncation.
- Keep all existing path, activity, usage, phase, summary, registry, and fail-closed semantics.

#### 3. Change trace factory/constructors and finalized descriptor

**Files**:

- `internal/core/InternalExecutionTraceHandleFactory.java`
- `internal/runtime/trace/DefaultExecutionTraceHandle.java`
- `internal/core/FinalizedTraceArtifact.java`

**Changes**:

- Add required `entrySkill` after `sessionId` in the internal trace factory and every `DefaultExecutionTraceHandle` constructor; remove all entry-free constructor paths.
- Store the supplied value before `initialize()` and validate nonblank without re-normalizing.
- Keep `TRACE_STARTED` metadata unchanged.
- Add required `String entrySkill` to `FinalizedTraceArtifact` and copy the stored value in `descriptor()`.
- Update all trace-handle and finalized-artifact construction sites, including retention, observation, catalog, artifact REST, phase-one, and REST integration tests.

#### 4. Carry the descriptor through the catalog and REST mapper

**Files**:

- `internal/runtime/observation/catalog/FinalizedTraceCatalogEntry.java`
- `internal/runtime/observation/catalog/InMemoryFinalizedTraceCatalog.java`
- `internal/observability/web/dto/ObservabilityDtos.java`
- `internal/observability/web/ObservabilityDtoMapper.java`

**Changes**:

- Add required `entrySkill` to the catalog record and REST `Trace` record.
- Copy only from `FinalizedTraceArtifact` during publication and from the catalog entry during DTO mapping.
- Keep duplicate publication idempotent for equal descriptors; a different entry skill becomes an ordinary conflicting descriptor through record equality.
- Do not consult active projection state, storage paths, or artifact content.

#### 5. Extend Java semantic tests

**Files**:

- `LiveActivityProjectorTest.java`
- `DefaultExecutionObservationHandleTest.java`
- `InMemoryActiveExecutionRegistryTest.java`
- `InMemoryFinalizedTraceCatalogTest.java`
- `ExecutionTraceHandleTest.java`
- `ObservabilityDtoMapperTest.java`
- `ObservabilityRestIntegrationTest.java`
- `ObservabilityPhaseOneIntegrationTest.java`
- `ObservabilityArtifactIntegrationTest.java`
- `ScheduledCompletionGraceRetentionTest.java`

**Changes**:

- Update every `ExecutionProjectionState`, `ActiveExecutionSnapshot`, trace handle, catalog entry, and artifact descriptor fixture with explicit identity.
- Assert the `TRACE_STARTED` projection already has nonblank identity.
- Assert a matching first root preserves identity and nested roots cannot replace it.
- Assert descriptor, repeated catalog publication, list/detail mapping, artifact acquisition, and expiration retain the same value.
- Retain tests proving core finalization failure cannot publish an artifact and optional observation/catalog failure cannot change execution outcome.

### Success Criteria

#### Automated Verification

- [x] Focused observation/trace tests pass: `./mvnw.cmd -pl loomspan-spring-boot-starter -Dtest=LiveActivityProjectorTest,DefaultExecutionObservationHandleTest,InMemoryActiveExecutionRegistryTest,InMemoryFinalizedTraceCatalogTest,ExecutionTraceHandleTest,ObservabilityDtoMapperTest,ObservabilityRestIntegrationTest test -DfailIfNoTests=false`
- [x] The first snapshot emitted from `TRACE_STARTED` contains the normalized entry skill.
- [x] Finalized descriptor, catalog list, detail, and artifact acquisition retain exactly the same string.
- [x] Nested root frames do not alter identity.
- [x] `TRACE_STARTED` and all other canonical NDJSON record bytes remain structurally unchanged.

#### Manual Verification

- [x] Review constructor/factory call sites and confirm no entry-free overload remains.
- [x] Review catalog publication and confirm identity comes only from the core descriptor.

---

## Phase 3: Change the Application REST Corpus Atomically

### Overview

Make `entrySkill` a required field in Java trace list/detail JSON and synchronize the executable Java/Go application REST corpus without touching trace-analysis fixtures.

### Changes Required

#### 1. Update REST fixture generation and assertions

**Files**:

- `loomspan-spring-boot-starter/src/test/java/com/lokiscale/loomspan/internal/observability/web/ConsoleRestFixtureCorpusTest.java`
- `loomspan-spring-boot-starter/src/test/java/com/lokiscale/loomspan/internal/observability/web/ObservabilityRestIntegrationTest.java`
- `loomspan-console-fixtures/application-rest/traces-page.json`
- `loomspan-console-fixtures/application-rest/trace-detail.json`

**Changes**:

- Add a stable entry skill such as `CheckDns` to the fixture `ObservabilityDtos.Trace`.
- Assert required camelCase `entrySkill` in Java list and detail endpoint tests.
- Regenerate the application REST corpus using only `ConsoleRestFixtureCorpusTest`.
- Inspect the fixture diff and retain LF endings.

#### 2. Prove the trace-analysis corpus did not change

**Files/directories to verify only**:

- `loomspan-console-fixtures/traces/`
- `loomspan-console-fixtures/expected/`
- `loomspan-console-fixtures/invalid/` when present

**Changes**: None expected. Do not regenerate the trace-analysis corpus for this feature.

### Success Criteria

#### Automated Verification

- [x] REST corpus regenerates and passes: `./mvnw.cmd -pl loomspan-spring-boot-starter -Dtest=ConsoleRestFixtureCorpusTest -Dloomspan.console.fixtures.regenerate=true test -DfailIfNoTests=false`
- [x] Only `application-rest/traces-page.json` and `application-rest/trace-detail.json` change under `loomspan-console-fixtures`.
- [x] Both JSON files contain nonempty `entrySkill` in the canonical field order generated by Jackson.
- [x] Trace-analysis corpus remains unchanged: `git diff --exit-code -- loomspan-console-fixtures/traces loomspan-console-fixtures/expected`

#### Manual Verification

- [x] Inspect the two generated JSON lines and confirm Java list/detail shapes agree.
- [x] Confirm no NDJSON record, expected analysis JSON, or trace-format version changed.

---

## Phase 4: Propagate Required Identity Through Go Acquisition and Fallback

### Overview

Require the new upstream field, retain it for installed evidence, and reconstruct the same value in reachable and cached browser responses.

### Changes Required

#### 1. Extend and validate the shared trace DTO

**Files**:

- `loomspan-console/internal/observability/dto.go`
- `loomspan-console/internal/observability/service.go`
- `loomspan-console/internal/observability/dto_test.go`
- `loomspan-console/internal/observability/service_test.go`

**Changes**:

- Add `EntrySkill string \`json:"entrySkill"\`` to `observability.Trace`.
- Make `validateTrace` reject missing or empty identity for list and detail responses.
- Assert fixture decoding yields the stable value.
- Add invalid-response cases for missing and explicitly empty `entrySkill`, preserving existing domain-error mapping.

#### 2. Retain acquisition-time identity

**Files**:

- `loomspan-console/internal/artifact/model.go`
- `loomspan-console/internal/console/service.go`
- `loomspan-console/internal/artifact/helpers_test.go`

**Changes**:

- Add required `EntrySkill string` to immutable `artifact.TraceMetadata`.
- Copy `trace.EntrySkill` in the production `TraceLoader`.
- Give the common valid test helper a stable entry skill so artifact lifecycle tests remain valid by default.
- Where a test intentionally exercises incomplete metadata, set the empty value explicitly rather than relying on an omitted initializer.

#### 3. Restore identity in cached list/detail

**Files**:

- `loomspan-console/internal/browserapi/observability.go`
- `loomspan-console/internal/browserapi/observability_test.go`

**Changes**:

- Copy `lookup.Metadata.EntrySkill` in `cachedTrace`; `cachedTracePage` then inherits it through lookup.
- Extend fallback tests to compare reachable and authentication/unavailable fallback list/detail responses and assert identical entry skill.
- Keep fallback eligibility, sorting, target scope, local availability, handle, and application-availability behavior unchanged.

#### 4. Update Go metadata construction sites

**Files/groups**:

- `internal/browserapi/artifact_download_test.go`
- `internal/browserapi/artifacts_test.go`
- `internal/console/artifact_integration_test.go`
- `internal/console/target_integration_test.go`
- `internal/traceanalysis/{bundle,calculations,fixture_corpus,index,payload,processor,service}_test.go`

**Changes**:

- Add explicit stable entry skill to valid `TraceMetadata` literals or route them through the common helper.
- Add acquisition integration assertions that `Lookup.Metadata.EntrySkill` equals the source trace.
- Keep `AcquiredArtifact`, `StoredEntry`, StorageSnapshot JSON, trace-analysis summaries, target rotation, and eviction contracts unchanged.

### Success Criteria

#### Automated Verification

- [x] Focused Go tests pass: `go test ./internal/observability ./internal/artifact ./internal/browserapi ./internal/console` from `loomspan-console`.
- [x] Missing/empty upstream trace identity is rejected for both list and detail.
- [x] Acquisition retains entry skill in immutable lookup metadata.
- [x] Reachable and cached-fallback trace list/detail return the same entry skill.
- [x] Existing scope rotation, authentication fallback, removal, expiry, and eviction tests remain green.

#### Manual Verification

- [x] Inspect browser JSON for a reachable trace and then for the same installed trace with application access blocked; confirm only availability/local facts differ, not entry skill.
- [x] Confirm no filesystem path or new storage DTO field is exposed.

---

## Phase 5: Present Trace Identity in React and Update Debugging Guidance

### Overview

Add the required TypeScript field, lead the catalog with plain-text entry skill, state it in Trace Detail, update mocks and semantic tests, and document its debugging meaning.

### Changes Required

#### 1. Extend the browser TypeScript contract

**File**: `loomspan-console/web/src/api/contracts.ts`

**Changes**:

- Add required `entrySkill: string` to `Trace`.
- Do not add it to `AcquiredArtifact`, `StoredEntry`, or trace-analysis DTOs.

#### 2. Update Trace Catalog presentation

**Files**:

- `loomspan-console/web/src/observability/Traces.tsx`
- `loomspan-console/web/src/observability/Traces.test.tsx`

**Changes**:

- Insert semantic `Entry skill` as the first column before Trace ID.
- Render `{t.entrySkill}` as plain React text.
- Keep Trace ID as the only detail-navigation link and retain the focusable/labeled scroll region, semantic headers, pagination, scope-bound URL, and `formatDateTime` rendering.
- Assert the first header and first cell are entry skill, the Trace ID link follows it, and application-authored characters render as text without producing markup/elements.
- Preserve all current user-authored date formatting and activity presentation work; edit this component narrowly.

#### 3. Update Trace Detail identity facts

**Files**:

- `loomspan-console/web/src/observability/TraceDetail.tsx`
- `loomspan-console/web/src/observability/TraceDetail.test.tsx`

**Changes**:

- Add `Entry skill` beside the Trace ID and Session ID facts near the beginning of the definition list.
- Extend trace fixtures and assertions, including a text-not-markup case.
- Preserve acquisition, raw download, explorer, dialog focus, availability, and scope-reset behavior.

#### 4. Update end-to-end application response builders

**Files**:

- `loomspan-console/web/e2e/target-context.spec.ts`
- `loomspan-console/web/e2e/live-executions.spec.ts`
- `loomspan-console/web/e2e/artifact-storage.spec.ts`

**Changes**:

- Add stable `entrySkill` to every finalized trace metadata builder/mock.
- Extend representative Trace Catalog/Detail workflows to assert entry-skill visibility before acquisition and persistence after authentication rejection/cached fallback.
- Preserve target rotation, deliberate acquisition, raw download, and existing application-content escaping scenarios.

#### 5. Document trace identity for skill authors

**File**: `ai/skill-authoring/traces-and-debugging.md`

**Changes**:

- Add a compact source-verified rule explaining that `entrySkill` is the exact registered name of the top-level YAML skill whose invocation owns the session.
- State that it is immutable across nested skills, is available in catalog/detail without acquisition, and is a recorded fact rather than proof of current registration or importance.
- Add focused implementation/test anchors for the session identity boundary and REST/browser behavior. Do not duplicate the YAML name grammar from `mental-model.md`.

### Success Criteria

#### Automated Verification

- [x] Frontend unit tests pass from `loomspan-console`: `npm --prefix web test -- src/observability/Traces.test.tsx src/observability/TraceDetail.test.tsx`.
- [x] TypeScript typecheck passes from `loomspan-console`: `npm --prefix web run typecheck`.
- [x] Catalog's first semantic header/cell is Entry skill and Trace ID remains the navigation link.
- [x] Detail states entry skill near trace/session identity.
- [x] Markup-shaped entry text remains inert in both views.
- [x] Updated guidance is supported by the cited Java, fixture, Go, and React tests and satisfies the LLM-first authoring standard.

#### Manual Verification

- [ ] Keyboard-focus the Trace Catalog region and confirm the added column does not change navigation or focus order.
- [ ] View catalog/detail at narrow and wide viewport sizes and confirm the existing scrollable table behavior remains usable.
- [x] Confirm a recorded entry skill is plain text and is not linked to the current Skill Catalog.

---

## Phase 6: Full Lockstep Verification

### Overview

Run the repository's complete Java, Go, build, browser, and race checks and audit the fixture/working-tree diff before handoff.

### Changes Required

No feature code is introduced in this phase. Resolve only failures attributable to PR 21 and avoid unrelated cleanup.

### Success Criteria

#### Automated Verification

- [x] Full Java module tests pass: `./mvnw.cmd -pl loomspan-spring-boot-starter test -DfailIfNoTests=false`.
- [x] REST corpus comparison passes without regeneration: `./mvnw.cmd -pl loomspan-spring-boot-starter -Dtest=ConsoleRestFixtureCorpusTest test -DfailIfNoTests=false`.
- [x] All Go tests pass from `loomspan-console`: `go test ./...`.
- [x] Repository console verification passes: `go run ./internal/buildtool verify`.
- [x] Race suite passes with the documented Windows environment: `$env:PATH = "C:\msys64\mingw64\bin;" + $env:PATH; $env:CGO_ENABLED = "1"; go test -race ./...`.
- [x] Fixture audit confirms exactly the two intentional application REST files changed and no trace-analysis corpus changed.
- [x] `git diff --check` passes.

#### Manual Verification

- [x] Review the final diff for accidental changes to existing activity/date-formatting work.
- [ ] Run one successful nested skill, one denied-before-root skill, and one standalone no-frame action; compare live, finalized, acquired, and cached entry identity.
- [x] Disconnect or reject application authorization after acquisition and confirm Trace Catalog and Trace Detail retain the same entry skill.
- [x] Confirm no compatibility shim, synthetic frame, artifact parser, new version field, or Trace Storage column was introduced.

## Testing Strategy

Create a dedicated testing plan with `ai/commands/3_testing_plan.md` before implementation. That artifact should specify the failing-test order and exact exit criteria. At a high level:

### Unit Tests

- Core normalization: null, blank, exact/over code-point bound, supplementary Unicode.
- Session construction: rejection before factories; identical normalized values delivered to observation and tracing.
- Coordinator/projector consistency: matching top-level route, mismatch, and different nested route.
- Snapshot and descriptor invariants: required identity at `TRACE_STARTED` and finalization.
- Catalog equality/conflict behavior with differing identity.
- Go decoding/validation: valid, missing, empty, list, and detail.
- Go metadata/fallback: acquisition copy and reachable/cached equality.
- React: column order, plain text, link ownership, detail identity, escaping.

### Integration Tests

- `DefaultSkillTemplate` passes the exact registered name.
- Access denial before root finalizes/catalogs retained error metadata without a synthetic frame.
- Standalone action with no frames finalizes under its explicit route.
- Java list/detail REST and Go corpus decoding agree.
- Browser fallback after authentication/unavailability reconstructs entry identity from installed metadata.
- Existing target rotation and eviction behavior disposes of identity with the installed entry.

### Manual Testing Steps

1. Invoke a top-level YAML skill with a nested skill and inspect the active snapshot immediately and after nested execution; entry skill remains the top-level name.
2. Finalize and open Trace Catalog; entry skill appears before the linked Trace ID without artifact acquisition.
3. Open Trace Detail and acquire the artifact; entry skill remains unchanged.
4. Make the application unavailable or unauthorized and reload list/detail; installed-copy fallback shows the same entry skill.
5. Exercise a denied top-level invocation retained under error policy and verify it catalogs with no root frame.

## Performance Considerations

- Entry identity is bounded to 256 code points once at session creation.
- The change retains one small string reference/value across session, live snapshot, finalized descriptor/catalog, Go metadata, and browser DTOs.
- Catalog listing remains metadata-only and performs no artifact I/O, scanning, acquisition, or parsing.
- Browser fallback continues to use bounded installed entries and one `Lookup` per storage snapshot entry.
- The added table cell has no new network request or stateful interaction.

## Migration Notes

No migration or backfill is required. The application trace catalog is in-memory and empty after restart; the console workspace and installed artifact entries follow existing transient lifecycle and target-scope cleanup. Java, Go, TypeScript, and fixtures ship together behind the exact product-version compatibility gate. Existing running processes or installed copies from a previous binary are not adapted by a legacy reader.

## References

- Original ticket: `ai/thoughts/tickets/loomspan-console-pr-21-trace-catalog-entry-skill.md`
- Codebase research: `ai/thoughts/research/2026-08-08-trace-catalog-entry-skill.md`
- Framework compatibility policy: `ai/thoughts/framework-feature-design-lens.md`
- Skill-authoring routing and standard: `ai/skill-authoring/README.md`
- Existing entry identity guidance: `ai/skill-authoring/mental-model.md`
- Existing trace guidance: `ai/skill-authoring/traces-and-debugging.md`
- Console repository constraints: `loomspan-console/AGENTS.md`
