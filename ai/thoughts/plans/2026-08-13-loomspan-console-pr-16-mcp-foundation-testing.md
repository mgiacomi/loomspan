# PR 16 — MCP Authentication and Lifecycle Foundation Testing Plan

## Change Summary

- Add the first live Loomspan Console MCP realm at exact `/mcp`, using official Go SDK `v1.7.0`, stateless Streamable HTTP, MCP `2026-07-28`, and compatibility with `2025-11-25` clients.
- Add one profile-owned persistent `mcp-access-key` contract, strict `lsmcp_` key format, native atomic mutation on Linux/macOS/Windows, and disabled-invalid recovery.
- Add an MCP-only Host/Origin/bearer policy, request bounds, authentication generation, admission freeze/cancel/drain, defensive SDK-session closure, and ordered process shutdown.
- Add `LOOMSPAN_get_runtime` with the sole capability `loomspan.runtime-status.v1`, unchanged `consolecore.StatusSnapshot` serialization, structured content, and deterministic bounded text.
- Add paired browser management operations and a Settings → MCP Integration page for enable, reveal, regenerate, disable, invalid-file removal, disclosure, and safe client setup.
- Add official protocol conformance, native platform CI, browser E2E, and tiered representative-client evidence.

The change is a new feature rather than a bug fix, so tests should be introduced as a sequence of small red vertical slices. No test should require simultaneous support for the intentionally obsolete `/api/mcp/`, `bfmcp_`, stateful-session, IPv6, YAML-section, or longer-lock-name designs.

## Impacted Areas

- **Credential persistence**: planned `loomspan-console/internal/mcpcredential/`; existing profile protection helpers in `loomspan-console/internal/profile/`.
- **Transport and lifecycle**: planned `loomspan-console/internal/mcpadapter/`; `loomspan-console/internal/webhost/routes.go`; `host.go`; `loomspan-console/internal/console/service.go`.
- **Shared status contract**: `loomspan-console/internal/consolecore/status.go`; `loomspan-console/internal/target/context.go`.
- **Browser API**: `loomspan-console/internal/browserapi/router.go`, planned MCP handlers/DTO fixtures, and existing session/CSRF/security helpers.
- **Web UI**: `loomspan-console/web/src/api/`, `web/src/app/`, planned `web/src/settings/`, and `web/e2e/`.
- **Serialized fixtures**: planned `loomspan-console/mcp-fixtures/runtime/` and MCP browser fixtures; existing `browser-fixtures/target/` remains protected.
- **Build and CI**: `loomspan-console/go.mod`, `go.sum`, `internal/buildtool/`, `.github/workflows/console-ci.yml`, and declaration tests.
- **Documentation/client evidence**: Console and release READMEs plus planned representative-client evidence.
- **Unaffected boundaries to regress**: Java application API allowlist, Console YAML schema version 1, Java-to-Go REST/SSE/artifact/NDJSON contracts, target scope, browser sessions, and trace formats.

## Risk Assessment

### High-risk behaviors

- A credential-file interruption could leave a partial key, replace a concurrently created canonical file, lose the old key before a replacement is durable, or delete an unsafe path.
- Rotation/disablement could commit new persistent state while an old-key request still emits a successful result.
- Freeze/admission synchronization could deadlock, admit after freeze, leak a goroutine/session, or fail to reopen after a rejected commit.
- Shared-listener routing could apply browser policy to MCP, apply permissive MCP Origin rules to browser/static routes, or retain a hidden alias.
- Authority, supplied-Origin, or bearer parsing could accept duplicate/comma-joined/forwarded values or read an unauthenticated body first.
- A secret could leak through logs, HTTP/MCP errors, structured/text tool output, fixtures, UI state, URL/history, browser storage, setup examples, or CI artifacts.
- SDK negotiation could work only for one target protocol revision or accidentally depend on stateful application sessions.
- `LOOMSPAN_get_runtime` could invent aggregate readiness, make target I/O, alter `StatusSnapshot`, advertise future capabilities, or disagree between structured and text content.
- Browser operations could reveal a key without paired session/CSRF or perform disruptive mutation without confirmation.
- Shutdown order could begin HTTP shutdown before MCP drain, remove the persistent key, or adopt prior-process request/session state after restart.
- Platform behavior could pass mocked unit tests while using the wrong native durability/exclusivity primitive, especially on Darwin or Windows.

### Edge cases

- Canonical file absent, valid, empty, too large, truncated, wrong prefix/alphabet/padding/newline, directory, symlink/reparse point, hard link, wrong owner, weak protection, unreadable, or replaced between observation and removal.
- Recognizable and near-miss temporary sibling names with safe and unsafe ownership/type/protection.
- Concurrent enable/regenerate/disable/reveal calls and commit failures at every preparation/flush/commit/revalidation/directory-sync boundary.
- Missing, malformed, duplicated, comma-joined, wrong-scheme, wrong-port, foreign, IPv6, or forwarded Host/Origin/Authorization inputs.
- `Content-Length` absent, duplicated, negative/invalid, exactly 1 MiB, and greater than 1 MiB; chunked body crossing the limit.
- No target, selected/disconnected, application-authentication-required, incompatible, and connected status snapshots.
- Caller cancellation, 10-second MCP deadline, rotation cancellation, disable cancellation, permanent shutdown, and failed commit reopen.
- Refresh/navigation/unmount while a key is visible; failed copy/mutation; stale async UI response after state transition.
- Multiple simultaneous MCP clients plus browser activity; separate profiles running concurrently; second process attempting the same profile.

### Protected compatibility paths and approved removals

| Surface | Test expectation |
| --- | --- |
| Application API | Existing Java public-surface architecture test remains unchanged and passing; no MCP/internal Console type may enter an allowlisted Java signature. |
| Supported SPI | No new supported SPI or external Go package. Architecture/dependency tests keep SDK and new extension seams inside `loomspan-console/internal/`. |
| Configuration and manifest contracts | Existing strict YAML version 1 fixtures/tests continue to accept the same documents and reject unknown `mcp` fields. No skill manifest test changes. |
| Persisted or serialized contracts | Add exact key-file, browser MCP DTO, initialization/tool, and capability fixtures. Preserve existing target/bootstrap JSON byte contracts unchanged; load MCP management state through its dedicated paired status operation. Ordinary responses never contain a key. |
| Ephemeral diagnostic formats | Test current-process request/generation/session coherence and redaction. Do not add historical session/generation readers or restart fixtures that adopt them. Existing trace/activity/artifact fixtures remain byte-coherent. |
| Internal or accidentally exposed implementation | Update internal constructor/router/host tests atomically. Assert `/api/mcp/`, `/mcp/`, stateful Loomspan session registries, `bfmcp_`, and IPv6 acceptance are absent rather than maintained behind fallbacks. |

The Java-to-Go boundary is not changed. Existing fixture-corpus and exact-release compatibility tests are regression coverage only; PR 16 needs no new Java fixture or compatibility-marker value.

## Existing Test Coverage

- `loomspan-console/internal/profile/profile_test.go` covers protected profile creation, lock exclusion, default strict config, weak protection, cross-process locking, and lock-path monitoring.
- Platform-specific profile path/protection tests cover Unix symlink ancestry, Linux roots, Windows paths, and Darwin paths, but there are no native atomic key create/replace/delete tests.
- `loomspan-console/internal/browserapi/security_integration_test.go` supplies the reusable read-spy pattern for security-before-body and independent Origin/session/CSRF checks.
- `loomspan-console/internal/webhost/host_test.go` covers loopback validation, actual listener lifecycle, and HTTP shutdown, but has no pre-shutdown drain hook.
- `loomspan-console/internal/webhost/static_test.go` protects static deep-link/reserved-path behavior, security headers, and method policy.
- `loomspan-console/internal/target/context_test.go` provides barrier-driven cancellation, authoritative generation rotation, owner invalidation, and late-publication patterns. These are analogous but not a substitute for an independent MCP generation.
- `loomspan-console/internal/consolecore/status_test.go` proves independent status facts and validation.
- `loomspan-console/internal/console/security_integration_test.go` launches a real Console and verifies pairing/bootstrap/lock release and credential non-leakage.
- `loomspan-console/internal/browserapi/contracts_test.go` and `browser-fixtures/` enforce exact Go-produced browser JSON.
- `loomspan-console/web/src/api/client.test.ts` verifies URLs, request bodies, in-memory security headers, and error mapping.
- `loomspan-console/web/src/observability/TraceStorage.test.tsx` provides confirmation-dialog and destructive-action UX patterns.
- `loomspan-console/web/e2e/fixtures/consoleProcess.ts` launches an isolated real process/profile and exposes its actual origin/pairing URL.
- `loomspan-console/internal/buildtool/projectdeclarations_test.go` pins toolchains, dependencies, CI runners/actions, release targets, and least-privilege workflow declarations.
- `.github/workflows/console-ci.yml` runs full Console verification and Playwright on Linux; release packaging runs Windows x64, Linux x64, and macOS ARM64.

### Current gaps

- No MCP handler, SDK, route, conformance runner, security policy, request tracker, runtime tool, credential store, browser operation, UI, fixture, or client evidence exists.
- No full-suite native CI executes credential mutation on Windows, Linux, macOS ARM64, and macOS x64.
- No test establishes no-success-after-credential-commit or drain-before-HTTP-shutdown ordering.
- No secret-sentinel scan spans MCP HTTP errors, JSON-RPC/tool output, UI, logs, fixtures, and CI artifacts.

## Bug Reproduction / Failing Test First

- **Name**: `TestRoutesDispatchesExactMCPRealmBeforeBrowserPolicy`
- **Type**: unit
- **Location**: `loomspan-console/internal/webhost/routes_test.go`
- **Arrange**: Construct browser, MCP, and static sentinel handlers. Give the browser/static policy a Host it would reject, so successful MCP dispatch proves route selection occurs first.
- **Act**: Send a request to exact `/mcp` with the planned `Routes(policy, browser, mcp, files)` composition.
- **Assert**: Only the MCP sentinel is called and its response is returned. `/mcp/` and `/api/mcp/` do not call it.
- **Expected failure pre-fix**: The test does not compile because `Routes` has no MCP handler seam; if initially adapted to the current signature, `/mcp` falls through to static handling and `/api/mcp/` remains a reserved 404. This is the smallest test that proves the missing peer realm without first requiring SDK, filesystem, or UI implementation.

After this first red test is green, add the remaining tests one behavior at a time. Tests for later slices may initially fail to compile because their deliberately planned internal types do not yet exist; do not land a broad skipped suite or expected-failure baseline.

## Failing-First Sequence

1. Exact `/mcp` peer routing and approved absence of aliases.
2. Exact key parser/generator and absent/valid/invalid startup states.
3. Exclusive enable, atomic regenerate, disable, invalid removal, and commit failure recovery.
4. Tracker freeze/cancel/wait/reopen/permanent-close race behavior.
5. Host → Origin → enabled → bearer → admission → body-limit → SDK ordering.
6. SDK initialization/list/tool call for `2026-07-28`, then `2025-11-25` compatibility.
7. `LOOMSPAN_get_runtime` schemas, successful status variants, deterministic text, and secret-free golden fixtures.
8. Paired browser API security and mutation orchestration.
9. React state/confirmation/key lifetime, then real-process Playwright flow.
10. Assembled rotation/disable/shutdown/restart races.
11. Official conformance, native OS matrix, architecture/dependency guards, and representative clients.

## Tests to Add or Update

### 1. Exact MCP peer routing and realm isolation

- **Type**: unit/integration
- **Location**: `loomspan-console/internal/webhost/routes_test.go`, `static_test.go`
- **Tests**:
  - `TestRoutesDispatchesExactMCPRealmBeforeBrowserPolicy`
  - `TestRoutesDoesNotAliasMCPPath`
  - `TestMCPPolicyDoesNotApplyToBrowserOrStaticRoutes`
- **What it proves**: Only exact `/mcp` reaches MCP; `/mcp/` and `/api/mcp/` are absent; browser and static security remain unchanged; cookies/bearer values do not cross realms.
- **Fixtures/data**: Sentinel handlers, rejected browser policy, exact and near-miss paths/methods.
- **Mocks**: In-memory `http.Handler` spies only; no SDK.
- **Contract classification**: Configuration and manifest contracts for fixed route behavior; Internal or accidentally exposed implementation for router signature.
- **Compatibility expectation**: Protect exact `/mcp`; approved removal of `/api/mcp/`; no alias/redirect dual behavior.

### 2. Exact key format and secret comparison

- **Type**: unit
- **Location**: `loomspan-console/internal/mcpcredential/key_test.go`
- **Tests**:
  - `TestGenerateKeyUsesThirtyTwoRandomBytesAndExactCanonicalEncoding`
  - `TestParseCanonicalKeyRejectsEveryNonCanonicalVariant`
  - `TestAuthenticateRequiresOneCompleteKeyAndNeverFormatsSecret`
- **What it proves**: Exactly 32 entropy bytes become `lsmcp_` + 43 raw URL-base64 characters + LF; canonical bytes total 50; padding, other alphabets, prefixes, lengths, CRLF, multiple/missing LF, whitespace, and partial keys fail; comparison is over the complete key.
- **Fixtures/data**: Deterministic entropy; table of one-byte mutations and secret sentinels.
- **Mocks**: Injected `io.Reader`; no filesystem.
- **Contract classification**: Persisted or serialized contracts.
- **Compatibility expectation**: Protect only `lsmcp_`; explicitly reject `bfmcp_` without a migration reader.

### 3. Credential startup state and safe temporary cleanup

- **Type**: unit/integration
- **Location**: `loomspan-console/internal/mcpcredential/store_test.go`, native platform test files
- **Tests**:
  - `TestOpenStoreDistinguishesAbsentValidAndInvalidCanonicalState`
  - `TestOpenStoreNeverRepairsOrReplacesInvalidCanonicalFile`
  - `TestStartupCleanupRemovesOnlyExactSafeOwnedTemporarySiblings`
  - `TestSecondProfileOwnerCannotReadOrMutateCredential`
- **What it proves**: Absent → disabled, valid → enabled with fresh generation, malformed/unsafe/unreadable → disabled-invalid with safe category; invalid bytes remain untouched; exact safe temporary siblings are cleaned, while near-miss, link/reparse, directory, foreign-owner, weakly protected, and unrelated files remain untouched; profile lock prevents a second process from reading/mutating.
- **Fixtures/data**: Temporary protected profile directories; valid/malformed key files; exact/near-miss temp names; helper subprocess pattern from `profile_test.go`.
- **Mocks**: Injectable file inspection only for deterministic error categories; native ownership/link checks remain real.
- **Contract classification**: Persisted or serialized contracts.
- **Compatibility expectation**: File presence is the sole persistent enabled state; no YAML/state-file fallback.

### 4. Native exclusive create, replacement, deletion, and durability

- **Type**: native integration
- **Location**: `loomspan-console/internal/mcpcredential/fileops_linux_test.go`, `fileops_darwin_test.go`, `fileops_windows_test.go`
- **Tests**:
  - `TestEnableUsesExclusiveCanonicalCommit`
  - `TestRegeneratePublishesOnlyCompleteOldOrNewKey`
  - `TestDisableCommitsCanonicalAbsence`
  - `TestMutationRevalidatesCanonicalProtectionAndContents`
  - `TestInterruptionAtEveryCommitBoundaryLeavesCanonicalOutcome`
- **What it proves**: Linux uses no-replace enable and directory `fsync`; Darwin uses `renameatx_np(RENAME_EXCL)`, `F_FULLFSYNC` where supported, and directory sync; Windows uses protected create-new, `FlushFileBuffers`, write-through `MoveFileExW`, replacement only for regenerate, and `DeleteFileW`; interruption outcomes are absence or one complete old/new key.
- **Fixtures/data**: Native temporary volumes/directories; deterministic old/new keys; failpoint table before/after write, file flush, close, namespace commit, revalidation, and directory flush.
- **Mocks**: Failpoint wrapper around syscalls for interruption placement; at least one success path per primitive uses real native calls. Do not accept cross-compiled execution as evidence.
- **Contract classification**: Persisted or serialized contracts.
- **Compatibility expectation**: Protect documented crash outcomes and native protection; do not claim durability beyond supported primitives.

### 5. Store mutation state machine and concurrency

- **Type**: unit/race
- **Location**: `loomspan-console/internal/mcpcredential/store_test.go`
- **Tests**:
  - `TestStoreSerializesEnableRevealRegenerateDisableAndInvalidRemoval`
  - `TestCommitFailureRetainsOldAuthoritativeSnapshot`
  - `TestRemoveInvalidRequiresSameObservedNonLinkIdentity`
  - `TestConcurrentSnapshotsAndAuthenticationNeverObservePartialState`
- **What it proves**: Operations are valid only from their prescribed states; reveal does not change generation; successful mutation publishes only after commit; commit/revalidation failure preserves old state; invalid removal refuses identity/link changes and never reveals contents; concurrent readers see coherent immutable snapshots.
- **Fixtures/data**: Deterministic store backend with barrier/failpoint controls and old/new key sentinels.
- **Mocks**: Fake file-operations interface for ordering/failure; retain separate native tests for syscall truth.
- **Contract classification**: Persisted or serialized contracts; Internal implementation for synchronization mechanics.
- **Compatibility expectation**: One state owner and one canonical contract, with no repair/legacy path.

### 6. Admission freeze, cancellation, drain, and reopen

- **Type**: unit/race
- **Location**: `loomspan-console/internal/mcpadapter/tracker_test.go`, `lifecycle_test.go`
- **Tests**:
  - `TestFreezeIsAtomicWithAdmissionAndRejectsNewRequests`
  - `TestFreezeCancelsClosesSessionsAndWaitsForOuterHandlers`
  - `TestTemporaryFreezeReopensAfterSuccessAndFailure`
  - `TestPermanentShutdownCannotReopen`
  - `TestFinalGenerationCheckSuppressesLateSuccess`
- **What it proves**: No request slips between freeze and registration; all captured contexts cancel; defensive session closers execute; drain waits for long-lived handlers; failed mutations reopen with old generation; permanent shutdown is terminal; old generation cannot construct success.
- **Fixtures/data**: Channels/barriers rather than sleeps; two admitted handlers, one legacy/long-lived; fake session objects; generation values.
- **Mocks**: Fake session closer and fake store commit callbacks. Use deadlines only to fail deadlocks, not to order the test.
- **Contract classification**: Ephemeral diagnostic formats/current-process lifecycle.
- **Compatibility expectation**: Current-run coherence; no persisted/adopted session or generation state.

### 7. Credential mutation choreography and target/browser independence

- **Type**: integration/race
- **Location**: `loomspan-console/internal/mcpadapter/lifecycle_test.go`, `loomspan-console/internal/console/mcp_lifecycle_integration_test.go`
- **Tests**:
  - `TestRegeneratePreparesBeforeFreezeAndCommitsAfterDrain`
  - `TestDisableDrainsBeforeCanonicalDeletion`
  - `TestNoOldKeyResultIsEmittedAfterCommit`
  - `TestMCPMutationDoesNotRotateTargetOrCloseBrowserSession`
  - `TestFailedCommitKeepsOldKeyUsableAfterReconnect`
- **What it proves**: Exact prepare → freeze → cancel/session-close → wait → commit → publish → reopen order; no old-key result after commit; target scope/application key/browser session remain intact; commit failure only forces reconnect and preserves old authentication.
- **Fixtures/data**: Blocking tool handler, store commit barrier, paired browser session, target-scope sentinel, old/new keys.
- **Mocks**: Fake commit backend for deterministic ordering; assembled real HTTP test for emission/reconnect.
- **Contract classification**: Persisted or serialized contract plus Ephemeral lifecycle coherence.
- **Compatibility expectation**: Protect realm independence and atomic transition; no target-generation coupling.

### 8. Authority, Origin, bearer, disabled state, and body ordering

- **Type**: unit/integration
- **Location**: `loomspan-console/internal/mcpadapter/security_test.go`
- **Tests**:
  - `TestMCPSecurityAcceptsOnlyCurrentIPv4AndLocalhostAuthorities`
  - `TestMCPSecurityAllowsAbsentOriginAndValidatesEverySuppliedOrigin`
  - `TestMCPSecurityRequiresExactlyOneBearerCredential`
  - `TestMCPSecurityFailureOrderNeverReadsProtectedBody`
  - `TestMCPDisabledAndInvalidStatesFailBeforeAuthenticationAndSDK`
  - `TestMCPBodyLimitAcceptsOneMiBAndRejectsLargerOrChunkedOverflow`
  - `TestMCPRequestDeadlineCancelsAtTenSeconds`
- **What it proves**: Exact current-port `127.0.0.1`/`localhost` only; no `[::1]`; forwarded headers ignored; absent Origin allowed, supplied Origin exact; duplicate/comma-joined headers rejected; browser/target credentials rejected; order is Host → Origin → enabled → bearer → admission → body bound → SDK; deterministic 400/403/503/401/413 responses; no OAuth metadata or secrets.
- **Fixtures/data**: Full table of missing/malformed/foreign/wrong-port/IPv6/duplicate values; `readSpy`; exact/over-limit fixed and chunked bodies; fake clock or configurable test timeout.
- **Mocks**: Spy authenticator, tracker, SDK handler, and body; security grammar itself is real.
- **Contract classification**: Configuration and manifest contracts for fixed endpoint policy; Persisted contract for bearer key; Internal adapter policy.
- **Compatibility expectation**: Protect exact policy; approved absence of IPv6/OAuth/cookie/key alternatives.

### 9. SDK identity, negotiation, discovery, and black-box lifecycle

- **Type**: SDK integration/real HTTP
- **Location**: `loomspan-console/internal/mcpadapter/server_test.go`
- **Tests**:
  - `TestServerIdentifiesAsLoomspanConsoleWithCompleteProductVersion`
  - `TestStatelessStreamableHTTPInitializesListsAndCallsWith20260728`
  - `TestStatelessServerNegotiatesCompatible20251125Client`
  - `TestMalformedAndUnsupportedProtocolRemainMCPTransportFailures`
  - `TestClientCancellationReconnectAndShutdownCloseTemporarySessions`
- **What it proves**: Official SDK `v1.7.0`; stateless option; exact identity/version; both protocol revisions; required initialize/list/call behavior; unsupported protocol distinct from Loomspan capability/target facts; cancellation and reconnect work without a durable Loomspan session registry.
- **Fixtures/data**: Official SDK client, real `httptest.Server` at `/mcp`, valid bearer, protocol-version table.
- **Mocks**: Fake status provider only; use real SDK server/client and HTTP stack.
- **Contract classification**: Persisted or serialized MCP protocol/tool contract; Ephemeral session lifecycle.
- **Compatibility expectation**: Protect two negotiated protocol revisions; no expected-failure baseline for required operations.

### 10. Runtime status tool and golden capability contract

- **Type**: unit/fixture integration
- **Location**: `loomspan-console/internal/mcpadapter/runtime_test.go`, `loomspan-console/mcp-fixtures/runtime/*.json`
- **Tests**:
  - `TestGetRuntimeRejectsUnknownOrNonObjectArguments`
  - `TestGetRuntimeSucceedsForEveryIndependentStatusCombination`
  - `TestGetRuntimeUsesOneSideEffectFreeSnapshot`
  - `TestGetRuntimeStructuredAndTextContentAgreeExactly`
  - `TestGetRuntimeUnexpectedInvariantFailureIsSafeInternalResult`
  - `TestRuntimeCapabilityFixtureIsExactSortedBoundedAndSecretFree`
- **What it proves**: Empty object only; no target/disconnected/auth-required/incompatible/connected are successful facts; no target request or mutation; unchanged `StatusSnapshot` under `status`; only sorted `loomspan.runtime-status.v1`; stable text order/placeholders/time; bounded output; safe unsuccessful `INTERNAL`; no duplicated protocol/server/endpoint/auth metadata.
- **Fixtures/data**: Fixed-time snapshots for all five representative states; secret sentinels for MCP key, application key, pairing, cookie, CSRF, and Authorization; exact JSON/text golden files with LF.
- **Mocks**: Counting status provider that fails on any operation besides `Snapshot`; injected invalid snapshot for invariant path.
- **Contract classification**: Persisted or serialized MCP tool/capability contract; existing browser status contract remains protected.
- **Compatibility expectation**: One new canonical schema; no alteration to `StatusSnapshot`, aggregate health, schema version, or future capability advertisement.

### 11. SDK dependency confinement and absence of public surface

- **Type**: architecture/declaration
- **Location**: `loomspan-console/internal/buildtool/projectdeclarations_test.go`, a focused Go architecture test, `loomspan-console/internal/config/decode_test.go`, `loomspan-console/internal/profile/profile_test.go`, and the existing Java architecture suite
- **Tests**:
  - `TestOfficialMCPDependencyIsPinnedExactly`
  - `TestMCPSDKImportsAreConfinedToInternalAdapter`
  - `TestMCPAddsNoExternalGoPackageOrJavaPublicSurface`
  - `TestStrictSchemaVersionOneRejectsMCPConfiguration`
  - `TestMCPFoundationKeepsCanonicalProfileLockBasename`
  - existing `LoomspanPublicSurfaceArchitectureTest`
- **What it proves**: Exact SDK version; SDK types do not leak into credential/shared services/browser/consolecore/target; all new packages remain under Go `internal`; no Java API/SPI delta; schema version 1 continues to reject an `mcp` section and fields; the lock remains `.loomspan-console.lock`.
- **Fixtures/data**: Parse `go.mod` and Go imports/AST; invalid strict-config YAML table in `internal/config`; existing Java allowlist; exact profile layout assertion.
- **Mocks**: None.
- **Contract classification**: Application API, Supported SPI, and Internal implementation boundaries.
- **Compatibility expectation**: Protected Java API unchanged; no new supported SPI or accidental exported boundary.

### 12. Browser credential-management API

- **Type**: unit/integration
- **Location**: `loomspan-console/internal/browserapi/mcp_test.go`, `security_integration_test.go`, `contracts_test.go`, `browser-fixtures/mcp/`
- **Tests**:
  - `TestMCPStatusNeverReturnsCredential`
  - `TestMCPEnableRevealRegenerateDisableRequirePairedCSRF`
  - `TestDisruptiveMCPOperationsRequireExactConfirmation`
  - `TestMCPInvalidRemovalRevalidatesIdentityAndNeverRevealsContents`
  - `TestMCPKeyBearingResponsesAreNoStoreAndOnlyExplicit`
  - `TestMCPBrowserOperationsRejectUnknownTrailingAndOversizedJSON`
- **What it proves**: Session plus CSRF on reveal/mutations; confirmations enforced server-side; ordinary bootstrap/status contains endpoint/state/safe diagnostic but never key; key only in enable/reveal/regenerate response; invalid contents never exposed; existing bounded strict JSON/no-store semantics.
- **Fixtures/data**: Disabled/enabled/disabled-invalid DTO fixtures; fake lifecycle service; cookie/tab/CSRF matrix; key/invalid-content sentinels.
- **Mocks**: Narrow fake MCP management service with call recorder/barriers; browser security remains real.
- **Contract classification**: Persisted or serialized browser JSON contract.
- **Compatibility expectation**: Add canonical DTOs atomically; existing target/bootstrap fixture semantics remain byte-correct and no credential crossover is accepted.

### 13. TypeScript client contract and secret placement

- **Type**: unit
- **Location**: `loomspan-console/web/src/api/client.test.ts`, planned MCP fixture-consumption test
- **Tests**:
  - `mcp status uses session without leaking credentials`
  - `mcp reveal and mutations use tab and CSRF headers`
  - `mcp confirmation values and empty bodies are exact`
  - `mcp endpoint and key never enter a request URL`
  - `consumes exact disabled enabled and disabled-invalid fixtures`
- **What it proves**: Correct paths/bodies/headers/error mapping; no key in URL or client persistence; TypeScript discriminated union matches Go fixtures.
- **Fixtures/data**: Committed browser MCP fixtures and `fetch` call captures.
- **Mocks**: Mock `fetch`; no React.
- **Contract classification**: Persisted or serialized browser contract.
- **Compatibility expectation**: Go/TypeScript/fixture atomic coherence; no dual DTO variants.

### 14. MCP Settings UX and in-memory key lifetime

- **Type**: React component/accessibility unit
- **Location**: `loomspan-console/web/src/settings/MCPIntegration.test.tsx`, app routing/navigation tests
- **Tests**:
  - `shows disclosure and distinct disabled enabled and invalid states`
  - `enable reveals key only after success and provides safe setup tabs`
  - `reveal is explicit and clears on navigation refresh unmount and state change`
  - `regenerate and disable require confirmation and handle cancellation`
  - `invalid removal requires separate confirmation and identity errors remain actionable`
  - `setup examples are user-global placeholder-based and never project URL or shell-history based`
  - `operation and copy failures are announced accessibly without retaining stale key`
- **What it proves**: Full state machine and disclosure; disruptive confirmations; ephemeral key ownership; safe client guidance; keyboard/focus/status/alert behavior.
- **Fixtures/data**: Disabled/enabled/invalid states, key sentinel, current five client setup shapes.
- **Mocks**: Mock API functions and clipboard; use fake timers only for transient UI, never lifecycle ordering.
- **Contract classification**: Persisted or serialized browser behavior; Configuration contract for fixed endpoint/setup policy.
- **Compatibility expectation**: One settings experience; no automatic/project configuration or persisted secret.

### 15. Real-process browser E2E

- **Type**: Playwright E2E
- **Location**: `loomspan-console/web/e2e/mcp-settings.spec.ts`, `web/e2e/fixtures/consoleProcess.ts`
- **Tests**:
  - `paired developer enables reveals regenerates and disables MCP`
  - `invalid canonical file is reported and explicitly removed without disclosure`
  - `revealed key leaves no URL history or web-storage residue`
  - `browser target and session remain usable across MCP rotation`
- **What it proves**: Embedded assets, real profile file, browser API, React state, confirmations, and process composition work end to end.
- **Fixtures/data**: Per-test isolated profile/workspace; helper to seed protected valid/invalid files before process start; browser storage/history inspection.
- **Mocks**: No browser/API mocks; target application may remain the existing E2E fake when target-state independence is exercised.
- **Contract classification**: Persisted key and browser serialized contracts.
- **Compatibility expectation**: Protected browser pairing/target behavior remains intact.

### 16. Assembled HTTP mutation, shutdown, and restart

- **Type**: integration/race
- **Location**: `loomspan-console/internal/console/mcp_integration_test.go`, `internal/webhost/host_test.go`
- **Tests**:
  - `TestLiveConsoleAdvertisesActualIPv4MCPURLAndExactAuthorities`
  - `TestConsoleRejectsIPv6ListenerBeforeCredentialOrListenerMutation`
  - `TestLiveConsoleRotatesKeyAfterDrainingOldRequestAndReconnects`
  - `TestLiveConsoleDisableRejectsLaterAuthenticationWithoutAffectingBrowser`
  - `TestHostRunsMCPPreShutdownDrainBeforeHTTPShutdown`
  - `TestRestartPreservesKeyButAdoptsNoRequestSessionOrApplicationCredential`
  - `TestProfileAndWorkspaceFailuresPreventMCPListenerAndCredentialMutation`
  - `TestSeparateProfilesRunConcurrentlyWithSeparateKeys`
- **What it proves**: Real composition and actual port; no old result after commit; browser/target independence; ordered shutdown; key survives restart while session/generation/application key do not; startup barriers and lock exclusion; concurrent profile isolation.
- **Fixtures/data**: Real loopback listeners, protected temp profiles/workspaces, SDK client, blocking tool hook, paired browser requests, target credential sentinel.
- **Mocks**: Injected listener/clock/entropy and blocking status hook where needed; HTTP, files, store, tracker, and SDK remain production composition.
- **Contract classification**: Persisted or serialized contracts and Ephemeral lifecycle coherence.
- **Compatibility expectation**: Preserve profile/workspace/browser/target contracts; no process-state adoption.

### 17. Applicable official MCP conformance

- **Type**: external black-box conformance
- **Location**: planned `loomspan-console/mcp-conformance/`, buildtool conformance mode, `.github/workflows/console-ci.yml`
- **Tests**:
  - Applicable protocol-generic official server scenarios for `2026-07-28` stateless Streamable HTTP.
  - Applicable protocol-generic official server scenarios for compatible `2025-11-25` clients against the same `/mcp` endpoint.
  - SDK and assembled HTTP tests perform discovery and calling of the real `LOOMSPAN_get_runtime` tool for both revisions.
- **What it proves**: Loomspan does not merely interoperate with its own SDK client for protocol-generic initialization, tool listing, DNS-rebinding, and caching behavior; SDK and assembled HTTP tests cover stateless lifecycle plus product-specific discovery and calling against the real closed Loomspan tool surface. The official runner's stateless aggregate and other fixture-specific tool, resource, prompt, sampling, and elicitation scenarios are inapplicable because they require diagnostic operations PR 16 intentionally does not advertise.
- **Fixtures/data**: Isolated temporary profile/key, actual endpoint/port, pinned official conformance runner revision, redacted runner output.
- **Mocks**: None across protocol boundary. Harness may automate credential setup but must exercise the production store semantics.
- **Contract classification**: Persisted or serialized MCP protocol contract.
- **Compatibility expectation**: No expected-failure baseline for applicable scenarios; a runner/SDK upgrade cannot merge with unexplained regression. Inapplicable fixture-specific scenarios must not be satisfied by adding production test tools or future capabilities.

### 18. Native CI and project declaration guards

- **Type**: CI/declaration integration
- **Location**: `.github/workflows/console-ci.yml`, `loomspan-console/internal/buildtool/projectdeclarations_test.go`
- **Tests**:
  - Linux x64, Windows x64, macOS ARM64, and macOS x64 run native credential/lifecycle tests.
  - Primary Linux job runs full verify and Playwright; blocking conformance job runs both revisions.
  - Declaration test requires both Darwin architectures, pinned actions/toolchains/runner, exact SDK/conformance versions, and least-privilege permissions.
- **What it proves**: Platform-native mechanics are continuously executed; CI configuration itself cannot silently drop a platform or float a security-sensitive dependency.
- **Fixtures/data**: Workflow source and dependency manifests.
- **Mocks**: None.
- **Contract classification**: Internal release assurance; supports persisted contract evidence.
- **Compatibility expectation**: All supported release platforms block on credential correctness.

### 19. Secret-sentinel and regression corpus

- **Type**: cross-layer integration/static scan
- **Location**: MCP adapter/browser/console tests plus a focused fixture/log scan test
- **Tests**:
  - `TestMCPSecretsNeverAppearInRuntimeResponsesErrorsOrLogs`
  - `TestCommittedMCPFixturesAndSetupExamplesContainNoLiveCredential`
  - existing browser target, artifact, trace, and Java fixture-corpus tests
- **What it proves**: MCP/application/pairing/cookie/CSRF/Authorization sentinels appear only in explicit paired credential-management responses; no diagnostic/tool/transport/log/fixture/setup leakage; unrelated serialized contracts remain coherent.
- **Fixtures/data**: Unique sentinel per credential realm; captured process output and HTTP/MCP bodies; committed-file scan limited to relevant generated evidence/config examples.
- **Mocks**: Captured log/output writers; otherwise real mapping/handlers.
- **Contract classification**: Persisted/serialized security contract and Ephemeral diagnostic redaction.
- **Compatibility expectation**: Preserve existing diagnostic usefulness while excluding Console-owned credentials.

### 20. Representative client matrix

- **Type**: manual/deep smoke with recorded evidence
- **Location**: planned `loomspan-console/docs/mcp-client-compatibility.md` and release evidence template
- **Tests**:
  - Codex and Claude Code: connect, initialize, discover, call runtime, rotate, observe old failure, reconfigure/reconnect, disable, shutdown, restart, reconnect.
  - Antigravity, Cursor, and Windsurf/Cascade: current-version local connection, bearer header, discovery, and tool call where automation permits.
  - Hosted Devin: record loopback transport out-of-scope; optional local Devin CLI is additive only when its actual current MCP configuration exists.
- **What it proves**: Product-specific header/config and reconnect behavior beyond protocol conformance.
- **Fixtures/data**: Exact client versions, environment-backed authorization where supported, user/global placeholder configs, clean profile, result checklist; no committed live key.
- **Mocks**: None. Manual gaps must be stated rather than converted to a pass.
- **Contract classification**: Configuration/operational contract.
- **Compatibility expectation**: Tiered policy only; PR 19 retains full skill/presentation evaluation.

## Acceptance-Signal Traceability

| Ticket acceptance signal | Primary coverage |
| --- | --- |
| Serialized store; canonical absent or complete | Tests 2–5 |
| Linux/macOS/Windows atomic/durable mechanics | Test 4, Test 18 |
| Freeze, cancel, drain, session close, commit, publish, reopen | Tests 6–7, Test 16 |
| No old-key result after commit; browser/target unaffected | Test 7, Test 16 |
| Shutdown drains and preserves key; restart adopts no process state | Test 6, Test 16 |
| Exact route, IPv4 authority, Origin, auth-before-body, bounds | Tests 1, 8, 16 |
| Applicable official conformance for both protocol revisions | Tests 9, 17 |
| SDK discovery, structured/text, cancellation, reconnect, shutdown | Tests 9–10, 16 |
| Status facts succeed across all target states; invalid input/internal error | Test 10 |
| Golden schema/order/agreement/bound/secrets | Tests 10, 19 |
| Only `loomspan.runtime-status.v1` advertised | Tests 9–10, 17 |
| Paired browser states/operations/protection/confirmation | Tests 12–15 |
| Tiered Codex/Claude/other local client evidence | Test 20 |
| YAML version 1 and Java-to-Go/public API unchanged | Regression suite, Test 11, exit criteria |

## Mocking and Determinism Rules

- Use channels and explicit barriers for concurrency ordering. Sleeps may cap a hung test but must never establish correctness.
- Use deterministic entropy, clocks, status snapshots, and unique secret sentinels.
- Split file-operation testing: fake backends establish store ordering/failure recovery; actual native tests establish syscall, protection, and durability behavior.
- Use real official SDK client/server and real HTTP for protocol integration; mock only the status provider or lifecycle barrier.
- Use exact byte fixtures for serialized contracts. Generate expected JSON with production DTOs and compare the directory inventory so extra/obsolete fixtures fail.
- Never log a real generated key from a failed test. Failure messages may report state/category/digest, never key bytes, authorization headers, or invalid canonical contents.
- Run race-sensitive tests repeatedly during development (`-count=100`) before relying on the full race suite.

## How to Run

### Focused local red/green loop

From `loomspan-console/`:

```powershell
go test ./internal/webhost -run TestRoutesDispatchesExactMCPRealmBeforeBrowserPolicy
go test ./internal/mcpcredential ./internal/mcpadapter
go test ./internal/browserapi ./internal/console ./internal/webhost
npm --prefix web run typecheck
npm --prefix web test
```

### Race-sensitive tests on this Windows workstation

```powershell
$env:PATH = "C:\msys64\mingw64\bin;" + $env:PATH
$env:CGO_ENABLED = "1"
go test -race ./internal/mcpcredential ./internal/mcpadapter ./internal/console ./internal/webhost
go test -race ./...
```

On Unix native runners:

```text
go test -race ./internal/mcpcredential ./internal/mcpadapter ./internal/console ./internal/webhost
go test -race ./...
```

### Concurrency stress

```text
go test ./internal/mcpcredential ./internal/mcpadapter ./internal/console -run 'Mutation|Freeze|Drain|Generation|Shutdown' -count=100
```

### Official conformance

After adding the planned pinned wrapper:

```text
go run ./internal/buildtool mcp-conformance
```

The wrapper must start isolated production composition and execute required server scenarios for both `2025-11-25` and `2026-07-28` without a required-case expected-failure baseline.

### Full Console verification

```text
go run ./internal/buildtool verify
npm --prefix web run test:e2e
```

### Repository compatibility regression

From the repository root on Windows:

```powershell
.\mvnw.cmd test
```

On Unix:

```text
./mvnw test
```

This retains `LoomspanPublicSurfaceArchitectureTest` and the Java-produced Console fixture corpus even though PR 16 does not change the Java-to-Go boundary.

### Required environments and data

- Go `1.26.5`, Node `24.18.0`, npm `12.0.2`, Java 21, and repository-pinned frontend dependencies.
- Native Linux x64, Windows x64, macOS ARM64, and macOS x64 runners for credential primitives. Cross-compilation is compilation evidence only.
- GCC/CGO for Go race tests.
- Chromium for Playwright.
- Pinned official MCP conformance runner and network access only for dependency installation; conformance itself targets local loopback.
- Isolated temporary protected profile/workspace per test; never use a developer's live profile or key.
- Installed current representative clients only for the manual matrix. Record versions and redact keys.

## Exit Criteria

- [x] The first routing test is observed failing before implementation and passes afterward; each later vertical slice begins with a focused red test rather than a skipped bulk suite.
- [ ] Exact key grammar, canonical file states, mutation transitions, cleanup, protection, and interruption outcomes pass.
- [ ] Native credential tests pass on Linux x64, Windows x64, macOS ARM64, and macOS x64 using actual platform primitives.
- [x] Race and 100-iteration barrier suites show no admission leak, late old-generation success, deadlock, goroutine/session leak, or partial state.
- [x] Exact `/mcp` works; `/mcp/` and `/api/mcp/` are absent with no alias, redirect, or fallback.
- [ ] Host, supplied-Origin, enabled state, bearer authentication, admission, and body parsing execute in the planned order; IPv6, forwarded/ambiguous headers, cross-realm credentials, OAuth discovery, oversized bodies, and wrong ports fail closed.
- [x] Applicable protocol-generic official server conformance passes for MCP `2025-11-25` and `2026-07-28` with no expected-failure baseline, while SDK/assembled tests pass real Loomspan discovery and tool calling.
- [ ] SDK black-box tests pass initialization, tool discovery/call, cancellation, reconnect, and shutdown.
- [ ] `LOOMSPAN_get_runtime` succeeds for every specified status state, rejects unknown input, maps unexpected invariants safely, performs no target I/O, and returns exact agreeing bounded structured/text output.
- [x] Golden fixtures advertise only `loomspan.runtime-status.v1`, preserve exact status property/enum spellings, and contain no console credential.
- [x] Browser API and React tests cover disabled, enabled, disabled-invalid, enable, reveal, regenerate, disable, invalid removal, confirmations, disclosure, accessibility, and key clearing.
- [x] Real-process Playwright covers the paired lifecycle and proves no key remains in URL/history/web storage after explicit display ends.
- [ ] Shutdown permanently freezes/drains MCP before HTTP shutdown, leaves the valid key intact, releases locks, and restart adopts no request/session/generation/application credential.
- [x] Browser sessions, target scope/application authentication, strict YAML schema version 1, trace/activity/artifact fixtures, Java-to-Go protocols, and the Java public API allowlist remain passing and unchanged in meaning.
- [x] SDK types remain confined to `internal/mcpadapter`; no supported SPI, Java public API, YAML field, or externally importable Go package is added.
- [ ] Approved obsolete paths and assumptions (`/api/mcp/`, `/mcp/`, `bfmcp_`, stateful Loomspan session registry, IPv6 authority, MCP YAML fields, longer lock name) are asserted absent rather than supported simultaneously.
- [ ] Secret-sentinel review passes for HTTP/MCP errors, structured/text results, logs, fixtures, UI, URL/history/storage, setup examples, and CI artifacts.
- [ ] `go run ./internal/buildtool verify`, Playwright E2E, repository Maven tests, race suite, native matrix, and conformance are green.
- [ ] Codex and Claude Code deep lifecycle evidence and available Antigravity/Cursor/Windsurf smoke evidence record exact versions and results; unavailable automation is explicit; hosted loopback clients are reported out of scope.
- [x] No `ai/skill-authoring/` test or documentation change is introduced because the implementation plan classifies authoring impact as no impact.

### Local execution note (2026-08-13)

Automated local results are green for buildtool verification, all Go race tests,
the 100-iteration lifecycle stress selection, both applicable official
conformance revisions, 382 browser unit tests, 30 Playwright tests, and the
complete Maven reactor/public API architecture suite. Native execution on
Linux and both Darwin architectures remains a CI result, not a local pass;
cross-compilation for Linux x86_64 and Darwin x86_64/arm64 succeeded. Manual
process-kill, representative-client, and independent disclosure-review items
remain unchecked and are not inferred from automation.
