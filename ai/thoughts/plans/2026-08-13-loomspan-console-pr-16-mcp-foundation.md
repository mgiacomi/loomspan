# PR 16 — MCP Authentication and Lifecycle Foundation Implementation Plan

## Overview

Add an independently authenticated, local-only MCP realm to the existing Loomspan Console listener. The implementation pins the official Go MCP SDK at `v1.7.0`, serves backward-compatible stateless Streamable HTTP at exact route `/mcp`, persists one profile-owned protected access key, drains old-generation work before credential commits, and exposes the side-effect-free `LOOMSPAN_get_runtime` bootstrap tool with capability `loomspan.runtime-status.v1`.

This plan uses the 2026-08-13 decision resolution as the implementation baseline. It supersedes older Phase 3 assumptions about SDK `v1.6.1`, stateful sessions, `/api/mcp/`, an `mcp` YAML section, IPv6 serving, `.loomspan-console-profile.lock`, and the legacy `bfmcp_` prefix.

## Current State Analysis

- The Console is a separate Go module whose only direct dependencies are YAML and `x/sys`; no MCP SDK or protocol implementation exists (`loomspan-console/go.mod:1-14`).
- One loopback listener is opened before handler construction, so the actual selected port is available in `webhost.Host.Prepare` (`loomspan-console/internal/webhost/host.go:25-95`, `loomspan-console/internal/console/service.go:218-265`).
- Route selection currently sends `/api/console/v1/*` to the browser API and reserves `/api/mcp/` as a browser-header 404. There is no `/mcp` handler (`loomspan-console/internal/webhost/routes.go:11-32`).
- The profile is resolved, protected, and exclusively locked before configuration is read. Its canonical directory and the live `.loomspan-console.lock` basename already provide the ownership boundary for a sibling credential file (`loomspan-console/internal/profile/profile.go:12-77`).
- The existing `atomicCreate` helper has the useful write/sync/close shape, but it does not provide exclusive platform-native commit, atomic replacement, delete durability, strict key validation, or safe temporary cleanup (`loomspan-console/internal/profile/profile.go:79-111`).
- Browser requests already apply route-scoped Host, Origin, session, and CSRF checks before bounded JSON decoding. MCP requires a separate policy and credential realm, not reuse of browser authentication (`loomspan-console/internal/browserapi/router.go:83-110`, `:228-252`; `loomspan-console/internal/browserapi/errors.go:10-63`).
- `consolecore.StatusSnapshot` already serializes all target facts needed by the bootstrap tool, and `target.Context.Snapshot` produces it without an application request (`loomspan-console/internal/consolecore/status.go:8-85`; `loomspan-console/internal/target/context.go:310-326`).
- Process cancellation currently proceeds directly into `http.Server.Shutdown`; there is no pre-shutdown MCP admission freeze/drain hook (`loomspan-console/internal/webhost/host.go:68-94`).
- The React application has no Settings/MCP route, DTOs, or client operations. Navigation currently ends with Trace Storage (`loomspan-console/web/src/app/routes.tsx:14-31`, `loomspan-console/web/src/app/App.tsx:24-33`).
- CI verifies all Go and web tests on Linux and packages on Windows, Linux, and macOS ARM, but it does not run platform-native credential tests on every supported operating system or either Darwin architecture (`.github/workflows/console-ci.yml:1-64`, `.github/workflows/console-release.yml:25-73`).

## Desired End State

After this plan is implemented:

1. A profile with no canonical key is MCP-disabled; a valid protected `mcp-access-key` enables MCP after restart; an invalid canonical file is reported to the paired browser and never silently repaired or replaced.
2. Enable, regenerate, disable, and invalid-file removal are serialized by one profile-owned store. The canonical file is always absent or exactly `lsmcp_` plus 43 unpadded base64url characters and LF (50 bytes total).
3. Linux, macOS, and Windows use their specified native commit and durability primitives. Startup cleanup touches only exact Loomspan-owned temporary siblings that pass the same link, owner, type, and protection checks.
4. Exact `/mcp` is selected as a peer route before browser policy. `/api/mcp/` becomes an ordinary non-existent browser/static route with no alias or redirect.
5. MCP validates current-port authority, then any supplied Origin, then enabled state and exactly one bearer credential, before SDK dispatch or body reading. It accepts `127.0.0.1:<port>` and `localhost:<port>`, rejects `[::1]`, forwarded-host substitutes, ambiguous headers, foreign/wrong ports, and invalid supplied origins.
6. Credential mutation freezes admission, cancels and drains admitted requests, defensively closes SDK sessions, commits persistent state, publishes the new key/generation, and then reopens admission. Commit failure preserves the previous authoritative state. Shutdown performs a permanent freeze/drain before bounded HTTP shutdown and leaves the key file intact.
7. The official SDK negotiates MCP `2026-07-28` and compatible `2025-11-25` clients in stateless Streamable HTTP mode. SDK types are confined to the MCP adapter.
8. `LOOMSPAN_get_runtime` rejects non-empty/unknown input, returns sorted `capabilities: ["loomspan.runtime-status.v1"]` and the existing status snapshot as structured content, plus a deterministic bounded text representation. Status states such as no target, disconnected, authentication-required, incompatible, and connected are successful tool results.
9. The paired browser presents disabled, enabled, and disabled-invalid states; requires confirmations for disruptive/destructive operations; reveals a key only from explicit no-store credential-management responses; and provides safe user/global client setup guidance without putting the key in a URL, repository config, or shell-history example.
10. Official conformance, SDK black-box tests, platform-native mutation tests, browser tests, assembled lifecycle tests, and the tiered representative-client procedure provide release evidence.

### Key Discoveries

- The actual listener authority is known at handler preparation time, so the MCP endpoint and accepted authority set do not require a second listener or a configuration field (`loomspan-console/internal/webhost/authority.go:8-25`, `loomspan-console/internal/console/service.go:218-265`).
- Target scope rotation already demonstrates cancellation and late-publication suppression, but MCP authentication generation must remain a separate owner and must not rotate target state (`loomspan-console/internal/target/context.go:480-516`; research decision §7).
- Browser and MCP can serialize the same `StatusSnapshot`; changing that shared type solely for MCP would unnecessarily alter the protected browser JSON contract (`loomspan-console/internal/consolecore/status.go:45-76`; research decision §8).
- Stateless SDK operation removes the need for a durable Loomspan session registry. A request admission/drain tracker plus defensive snapshots from `mcp.Server.Sessions()` are sufficient (research decision §1).
- The existing release matrix packages macOS ARM only, while the credential mechanics require native tests on both Darwin architectures; CI must add an explicit native credential-test matrix rather than relying on cross-compilation (`.github/workflows/console-release.yml:31-73`; ticket acceptance signals).

## What We're NOT Doing

- Runtime inspection beyond the status bootstrap; skill, execution, activity, trace, payload, and resource tools remain PR 17/18 work.
- The portable Agent Skill, installation/distribution, or full agent-quality evaluation; those remain PR 19 work.
- Remote MCP, stdio transport, IPv6 listeners, stateful MCP application sessions, event replay, subscriptions, prompts, resources, OAuth, sampling, elicitation, or MCP Apps.
- Per-client keys, client identity/attribution, selective revocation, or automatic client configuration.
- Any `mcp` YAML section, configurable endpoint, configurable protocol mode, configurable timeout/body bound, persistent enabled flag, or key in YAML.
- Changes to target selection, upstream application authentication, browser session/CSRF behavior, Loomspan execution, Java application endpoints, or trace formats.
- Compatibility aliases for `/api/mcp/`, the legacy `bfmcp_` prefix, or the illustrative longer profile-lock basename.
- Secret scanning or redaction of application-recorded diagnostic content; console-owned credentials remain excluded from diagnostic output.

## Skill-Authoring Documentation Impact

**Impact**: No impact

- **Rationale**: PR 16 adds Console transport authentication and a runtime-status bootstrap capability. It does not change YAML skill syntax, validation, mappings, planning/execution semantics, evidence contracts, model selection, capability visibility/RBAC inside a skill tree, attachments, limits, traces, or skill testing guidance. The debugging Agent Skill and its author-facing use of MCP capabilities are explicitly deferred to PR 19.
- **Documents to update**: None under `ai/skill-authoring/`.
- **Supporting evidence**: The result is sourced from the unchanged `consolecore.StatusSnapshot` (`loomspan-console/internal/consolecore/status.go:45-76`), and the ticket limits the advertised capability to Console runtime status. MCP adapter, credential, browser setup, and conformance tests support the new Console behavior without establishing skill-authoring semantics.
- **Coverage table update**: Not required. No topic is added and no authoring topic's coverage or confidence changes (`ai/skill-authoring/README.md:31-66`).
- **LLM-first usability**: Not applicable.

## Contract and Compatibility Impact

| Surface | Classification and supporting evidence | Planned compatibility treatment |
| --- | --- | --- |
| Application API | No impact. PR 16 changes only the independent Go Console and embedded browser; none of the Java types allowlisted by `LoomspanPublicSurfaceArchitectureTest` changes. | Preserve the closed Java API allowlist; run the architecture test as a regression check if any production Java file changes unexpectedly. |
| Supported SPI | No impact. Loomspan has no supported MCP SPI, and new Go packages remain beneath `loomspan-console/internal/`. Existing internal interfaces are assembly/test seams only. | Add no SPI, public extension point, replaceable bean, or externally importable Go package. |
| Configuration and manifest contracts | Strict Console YAML schema version 1 remains unchanged. No skill manifest changes. The fixed `/mcp` route and file-presence enablement are documented operational contracts, not YAML fields. | Preserve accepted YAML exactly; add no aliases or ignored legacy fields. Document fixed behavior in Console/release guidance. |
| Persisted or serialized contracts | Adds protected profile sibling `mcp-access-key` with exact filename, 50-byte format, file-presence enablement, and atomic mutation semantics. Adds browser credential-management DTOs and MCP initialization/tool schemas. Existing browser `StatusSnapshot` serialization is reused unchanged. | Introduce one canonical key format and one route/tool schema with golden fixtures. Update Go, TypeScript, browser fixtures, tests, and docs atomically. No legacy reader is needed because the contract is new. |
| Ephemeral diagnostic formats | Adds process-local MCP authentication generation, admitted-request records, temporary SDK sessions, and protocol messages. No trace, artifact, activity, or continuation format changes. | Do not persist/adopt session or generation state. Keep errors safe and deterministic, suppress late old-generation results, and keep credentials out of messages/logs/results. |
| Internal or accidentally exposed implementation | Adds credential file operations, store, MCP policy/tracker/adapter, DTO mappings, and shutdown hooks under internal packages. Adjusts internal router and assembly signatures. | Update all in-repository callers/tests atomically. Prefer one coherent implementation; do not add bridges, compatibility constructors, or duplicate state owners. |

- **Evidence of supported contracts**: The approved ticket and revalidated research establish `/mcp`, `mcp-access-key`, `lsmcp_`, `loomspan.runtime-status.v1`, and browser management behavior. Existing README/config tests protect strict YAML and browser serialization. The Java allowlist remains executable authority for application API classification.
- **Intended breaks**: Remove the unshipped `/api/mcp/` reservation and supersede planning-only `bfmcp_`, stateful-session, IPv6, YAML-section, and longer-lock-name assumptions. There are no released consumers or valid persisted MCP keys to migrate.
- **In-repository consumers to update**: `go.mod`/`go.sum`; Console assembly; webhost routing/host tests; browser router/DTO/tests; React contracts/client/routes/navigation/settings tests; Console/release documentation; fixtures; CI/build declaration tests.
- **Public-surface delta**: No Java API, Spring bean, or supported SPI delta. New exported Go identifiers, if needed for cross-package assembly, remain module-internal because their packages are under `internal/`.
- **Shim decision**: **No shim.** The removed route reservation and stale design names were never a supported or implemented MCP contract. Atomic update is coherent and the repository is pre-release.
- **Java-to-Go boundary coordination**: **Not required.** PR 16 reads existing Go-owned target status and does not change application-adapter REST/SSE, acquisition, problem, artifact, or NDJSON contracts.

## Implementation Approach

Keep three owners sharply separated:

```text
profile-owned credential store
  -> canonical file validation/mutation
  -> accepted key snapshot + monotonic authentication generation

MCP adapter
  -> route-specific authority/origin/bearer policy
  -> admission/request drain tracker
  -> official SDK server + temporary sessions
  -> LOOMSPAN_get_runtime mapping from target.Context.Snapshot()

browser adapter
  -> paired + CSRF-protected credential-management operations
  -> explicit reveal/confirmation UI and client setup guidance
```

The credential store prepares replacement files before admission is frozen, but all state-changing commits execute through one mutation critical section. The adapter tracker freezes new work, cancels all captured request contexts, snapshots/closes SDK sessions defensively, and waits for admitted handlers. Only then may the store commit and publish. The same permanent drain primitive runs before `http.Server.Shutdown`.

Use fixed implementation constants of 1 MiB for an MCP request body and 10 seconds for an admitted PR 16 MCP request. Enforce the body limit at the outer MCP handler, after authority/origin/authentication but before the SDK reads the body, and retain the shared host's existing header/time bounds. Disabled/invalid state returns a deterministic `503 Service Unavailable` after authority and supplied-Origin validation, without SDK dispatch or OAuth metadata; missing/malformed/incorrect bearer authentication returns `401 Unauthorized` without echoing credential material. These are transport failures, not MCP tool results.

## Phase 1: Profile-Owned Credential Store and Native Commits

### Overview

Create the sole owner of persistent MCP enablement, strict key validation, in-memory authentication state, temporary cleanup, and serialized mutations before any transport or UI depends on it.

### Changes Required

#### 1. Credential representation and store

**Files**: `loomspan-console/internal/mcpcredential/key.go`, `loomspan-console/internal/mcpcredential/store.go`, `loomspan-console/internal/mcpcredential/store_test.go`

**Changes**:

- Define fixed canonical basename `mcp-access-key`, exact temporary sibling pattern, 32-byte entropy source, `lsmcp_` prefix, 43-character raw URL-base64 suffix, LF terminator, and exact 50-byte file parser.
- Reject padding, CRLF/missing/extra newline, wrong alphabet/prefix/length, empty/oversized data, directories, symlinks/reparse points, links, wrong owner, weak protection, and unreadable canonical files.
- Model three paired states: `DISABLED`, `ENABLED`, and `DISABLED_INVALID`, with a safe diagnostic category that never includes file content or the key.
- On construction, require the already-owned profile directory, remove only exact safe Loomspan temporary siblings, inspect the canonical path, and start a fresh process-local generation. Do not read before profile/workspace startup barriers complete.
- Expose internal snapshot/authenticate/prepare/commit operations without returning mutable key storage. Compare the complete key in constant time and clear temporary byte buffers where practical.
- Serialize enable, regenerate, disable, and invalid-file removal. Enable is valid only from clean disabled state; regenerate/reveal/disable only from valid enabled state; invalid removal only from disabled-invalid state.
- Revalidate canonical file safety and content after successful mutations; never repair or replace malformed canonical state silently.

#### 2. Build-tagged file operations

**Files**: `loomspan-console/internal/mcpcredential/fileops_linux.go`, `fileops_darwin.go`, `fileops_windows.go`, platform-specific native tests

**Changes**:

- Linux: protected create-new temporary file; file `fsync`; `renameat2(RENAME_NOREPLACE)` for enable with tested `linkat` fallback only for unsupported syscall; atomic rename-over for regenerate; unlink for disable; directory `fsync` after namespace mutation.
- Darwin: protected create-new temporary file; `F_FULLFSYNC` where supported; `renameatx_np(RENAME_EXCL)` for enable; atomic rename-over for regenerate; unlink for disable; directory `fsync`; keep Darwin separate from a generic Unix implementation.
- Windows: create-new with protected DACL; `FlushFileBuffers`; `MoveFileExW` with write-through and replacement flag only for regenerate; `DeleteFileW`; reject reparse points and revalidate owner/DACL/type/content after commit.
- Add injectable failure points around prepare, flush, commit, revalidation, and directory durability so tests prove canonical absence or a complete valid old/new key after every simulated interruption.

#### 3. Profile integration seam

**Files**: `loomspan-console/internal/profile/profile.go`, `loomspan-console/internal/profile/permissions_unix.go`, `permissions_windows.go` (only if narrow reusable verification helpers are required)

**Changes**:

- Provide the credential store only the resolved, exclusively owned profile boundary and narrow protection checks; do not move key state into `Profile`, YAML configuration, or workspace storage.
- Preserve `.loomspan-console.lock` and current configuration creation behavior.

### Success Criteria

#### Automated Verification

- [x] Exact valid key generation/parser and exhaustive malformed variants pass: `go test ./internal/mcpcredential`
- [x] Enable/regenerate/disable/invalid-removal state-transition and commit-failure tests pass without silent replacement.
- [x] Startup cleanup tests prove only exact safe owned temporary siblings are removed.
- [ ] Native Linux, Darwin ARM/x64, and Windows tests prove exclusive create, replace, delete, protection, and interruption outcomes on their actual operating systems.
- [x] Race tests pass for concurrent store snapshots, authentication, reveal, and serialized mutations: `go test -race ./internal/mcpcredential`

#### Manual Verification

- [ ] Inspect enabled key permissions/owner on Linux and macOS and owner/protected DACL on Windows.
- [ ] Kill the process around prepared and committed mutations and confirm restart observes absence or one complete valid old/new key.

---

## Phase 2: MCP Admission, Generation, and Drain Lifecycle

### Overview

Implement the independent request-generation boundary and exact freeze/cancel/session-close/wait behavior before introducing key-changing browser operations.

### Changes Required

#### 1. Admission/request tracker

**Files**: `loomspan-console/internal/mcpadapter/tracker.go`, `tracker_test.go`

**Changes**:

- Track an admission state, captured authentication generation, per-request cancel function, active-handler wait group, and permanent-shutdown state.
- Make admission atomic with freeze so no request can enter between the freeze decision and tracker registration.
- Freeze rejects new work, cancels all admitted contexts, invokes a supplied defensive SDK-session closer, and waits for all outer handlers, including long-lived legacy HTTP requests.
- A temporary freeze may reopen after successful or failed credential mutation; permanent shutdown cannot reopen.
- Provide a final generation check immediately before successful tool result construction as defense in depth.

#### 2. Credential mutation coordinator

**Files**: `loomspan-console/internal/mcpadapter/lifecycle.go`, `lifecycle_test.go`

**Changes**:

- Regenerate: prepare and flush a replacement first; freeze/drain; commit; publish key/generation; reopen. On commit failure retain/publish old state and reopen.
- Enable: prepare key, freeze/drain the disabled realm, exclusive commit, publish, reopen.
- Disable: freeze/drain, delete/durably commit absence, publish disabled generation, reopen. If delete fails, preserve enabled state.
- Invalid-file removal: freeze/drain, re-check that the exact canonical pathname still names the same startup-observed non-link filesystem object, delete that object without reading/revealing its invalid contents, publish clean disabled state, and reopen. Refuse removal if identity or link status changed.
- Reveal returns the currently accepted key through the paired API only and does not change generation.
- Do not rotate `targetScopeId`, clear target/application credentials, cancel browser work, or alter runtime execution.

### Success Criteria

#### Automated Verification

- [x] Deterministic barrier tests prove no admission after freeze, cancellation reaches admitted handlers, drain waits, and permanent shutdown cannot reopen.
- [ ] Race tests prove no old-generation success is constructed/emitted after a successful regenerate or disable commit.
- [ ] Failed commit tests prove old state remains authoritative and admission reopens.
- [ ] Browser/target sentinel tests prove MCP mutations do not rotate target scope or close browser sessions.
- [x] Tracker race suite passes: `go test -race ./internal/mcpadapter`

#### Manual Verification

- [ ] Exercise a deliberately blocked request during regeneration and confirm the old client disconnects while browser target state remains unchanged.

---

## Phase 3: Secured Stateless MCP Adapter and Runtime Bootstrap

### Overview

Pin the official SDK, mount exact `/mcp`, enforce the independent security chain and bounds, and register the sole PR 16 capability/tool.

### Changes Required

#### 1. Official SDK and adapter boundary

**Files**: `loomspan-console/go.mod`, `loomspan-console/go.sum`, `loomspan-console/internal/mcpadapter/server.go`, `server_test.go`

**Changes**:

- Pin `github.com/modelcontextprotocol/go-sdk` at exact `v1.7.0` and use only `github.com/modelcontextprotocol/go-sdk/mcp` inside `internal/mcpadapter`.
- Construct server identity `loomspan-console` with `release.ProductVersion()` and `StreamableHTTPOptions{Stateless: true}`.
- Retain no durable application session registry. Snapshot `Server.Sessions()` and close current temporary sessions only as a defensive drain hook.
- Keep protocol negotiation, malformed JSON-RPC, unsupported versions, and initialization/list/tool-call framing in the SDK layer.

#### 2. MCP authority, Origin, bearer, enabled-state, and body policy

**Files**: `loomspan-console/internal/mcpadapter/security.go`, `security_test.go`

**Changes**:

- Build the authority policy from the actual listener port. Accept exact `127.0.0.1:<port>` and `localhost:<port>` Host values; reject missing, duplicated/comma-joined, malformed, foreign, wrong-port, `[::1]`, absolute-form ambiguity, and all forwarded-host substitution.
- Validate Host before Origin, enabled state, authentication, body limit, SDK parsing, or status access.
- Permit no Origin header. When supplied, require exactly one valid HTTP loopback Origin at the current port from the same accepted set; reject `null`, opaque/non-HTTP, foreign, wrong-port, duplicated, or ambiguous values.
- Require exactly one `Authorization: Bearer <complete-key>` value and constant-time authentication. Reject browser cookies, pairing/CSRF values, target keys, query-string keys, and malformed/duplicate bearer input.
- Return deterministic no-store HTTP failures: 400 for authority/request-shape rejection, 403 for supplied-Origin rejection, 503 for disabled/invalid state, 401 for missing/invalid bearer, and 413 for authenticated oversized bodies. Do not include OAuth discovery/challenge metadata or credential values.
- Apply the fixed body limit only after security admission and wrap the request context with the tracker/request timeout before SDK handling.

#### 3. Runtime tool schema and mapping

**Files**: `loomspan-console/internal/mcpadapter/runtime.go`, `runtime_test.go`, `loomspan-console/mcp-fixtures/runtime/*.json`

**Changes**:

- Register exact tool `LOOMSPAN_get_runtime` with an empty-object input schema and `additionalProperties: false`.
- Obtain exactly one side-effect-free `target.Context.Snapshot().Status` value; validate it and serialize it unchanged under `status`.
- Return a lexically sorted `capabilities` array containing only `loomspan.runtime-status.v1`.
- Emit equivalent structured content and deterministic concise text in fixed field order: capability, scope, selection, connection, authentication, compatibility, runtime identity, instance identity, live monitoring, and RFC3339 observation time. Omit optional values with an explicit stable placeholder and enforce a small fixed output bound.
- Treat every valid target-status combination as success. Map only unexpected adapter/snapshot invariants to an unsuccessful tool result with stable `INTERNAL` code and safe concise text.
- Add a golden fixture corpus asserting exact properties/enums, capability order, structured/text agreement, response bound, and absence of access, application, pairing, session, CSRF, and authorization sentinels.

#### 4. Peer route selection

**Files**: `loomspan-console/internal/webhost/routes.go`, `routes_test.go`, `static_test.go`

**Changes**:

- Change `Routes` to dispatch exact `/mcp` (including valid transport method handling inside the adapter) before browser/static policy.
- Remove the `/api/mcp/` reservation completely; do not alias or redirect it and do not accept `/mcp/` as the endpoint.
- Keep `/api/console/v1/*` and static assets under their existing browser policy with no credential crossover.

### Success Criteria

#### Automated Verification

- [x] Dependency test asserts exact SDK `v1.7.0` and adapter architecture test rejects SDK imports outside `internal/mcpadapter`.
- [ ] Security table tests cover ordering and every accepted/rejected Host, Origin, authorization, disabled, malformed, and oversized case without reading protected bodies early.
- [ ] SDK in-memory and real-HTTP black-box tests prove initialize, tool listing, tool call, cancellation, reconnect, and shutdown for both protocol revisions.
- [ ] Golden runtime fixtures pass for no-target, disconnected, authentication-required, incompatible, and connected snapshots.
- [x] Capability conformance fails if the named capability is absent, unsorted, duplicated, or advertised without `LOOMSPAN_get_runtime`, and proves later capability families are absent.
- [x] Webhost tests prove exact `/mcp`, no `/mcp/`, no `/api/mcp/` compatibility route, and unchanged browser/static security.

#### Manual Verification

- [ ] Use an SDK client against a live ephemeral-port Console and inspect initialization identity, discovery, structured output, and text fallback.

---

## Phase 4: Paired Browser Credential Management and Settings UX

### Overview

Expose the store only through paired, CSRF-protected browser operations and add the explicit disclosure/confirmation/setup experience.

### Changes Required

#### 1. Browser API operations and DTOs

**Files**: `loomspan-console/internal/browserapi/router.go`, `mcp.go`, `mcp_test.go`, `contracts_test.go`

**Changes**:

- Add the lifecycle coordinator/store dependency to internal browser router options.
- Add paired operations for status, enable, reveal, regenerate, disable, and remove-invalid. Require session on all and CSRF on every reveal/mutation; reject unknown fields/trailing JSON with existing bounded decoding.
- Return endpoint, state, safe invalid diagnostic, and setup metadata without a key from ordinary status/bootstrap. Return the key only from explicit enable/reveal/regenerate responses.
- Require explicit confirmation values for regenerate, disable, and invalid-file removal at the API boundary, not only in React.
- Keep `Cache-Control: no-store` and ensure errors never include the key or invalid file contents.

#### 2. TypeScript contracts and client

**Files**: `loomspan-console/web/src/api/contracts.ts`, `client.ts`, `client.test.ts`

**Changes**:

- Add discriminated MCP state and explicit credential-response types.
- Add status, enable, reveal, regenerate, disable, and invalid-removal calls using tab/CSRF headers.
- Keep key-bearing values only in operation results/component state; do not persist them in local/session storage or place them into navigation state/URLs.

#### 3. Settings → MCP Integration page

**Files**: `loomspan-console/web/src/settings/MCPIntegration.tsx`, `MCPIntegration.test.tsx`, `loomspan-console/web/src/app/routes.tsx`, `App.tsx`, relevant styles and E2E specs

**Changes**:

- Add Settings navigation and `/settings/mcp` route with accessible disabled, enabled, and disabled-invalid presentations.
- Keep the diagnostic-data/model-provider disclosure warning visible in every state.
- Enable reveals the new key once with copy affordance and endpoint/setup tabs. Reveal is always an explicit action.
- Regenerate requires confirmation that clients disconnect and old configuration fails; only reveal the new key after the full commit/generation transition succeeds.
- Disable requires confirmation, clears any rendered key immediately on success, and returns to disabled.
- Disabled-invalid shows only the safe diagnostic, never contents; removal requires separate confirmation and successful revalidation/deletion before enable becomes available.
- Provide Codex, Claude Code, Antigravity, Cursor, and Windsurf/Cascade user/global setup shapes with environment-backed bearer headers where supported and otherwise placeholders. Never render a repository config, key-bearing URL, automatic mutation, or shell-history-bearing command.
- Clear revealed key state on route exit, page refresh, state transition, operation failure, and component unmount.

### Success Criteria

#### Automated Verification

- [x] Browser API tests independently prove Origin/session/CSRF/confirmation requirements and no-store key responses.
- [x] Contract fixtures prove ordinary bootstrap/status never carries a key and invalid diagnostics are safe.
- [ ] React tests cover all three states, persistent warning, confirmation flows, key clearing, accessible focus/status, copy/setup behavior, and error recovery.
- [x] Client tests prove credentials remain in headers/body as designed and never enter URLs.
- [x] Browser unit/type/build pipeline passes: `npm --prefix web run typecheck`, `npm --prefix web test`, `npm --prefix web run build:web`.

#### Manual Verification

- [ ] Pair the browser, enable MCP, copy/configure a client, reveal again, regenerate, disable, and recover from a deliberately invalid canonical file.
- [ ] Confirm secrets do not remain visible after navigation/refresh and are absent from browser history and URLs.

---

## Phase 5: Console Assembly and Ordered Shutdown

### Overview

Wire one credential store, lifecycle tracker, SDK adapter, browser management surface, and peer route into `console.Run`, with drain-before-HTTP-shutdown ordering.

### Changes Required

#### 1. Process assembly

**Files**: `loomspan-console/internal/console/service.go`, `security_integration_test.go`, new MCP assembly tests

**Changes**:

- Construct the credential store after profile/workspace locking and cleanup, before listener service; fail startup only for store construction failures, while malformed canonical state remains a paired disabled-invalid state.
- Construct one tracker/lifecycle coordinator and one SDK server. Pass only narrow dependencies to browser and MCP adapters.
- In `Host.Prepare`, derive advertised `http://127.0.0.1:<actual-port>/mcp` and accepted authorities from the actual IPv4 listener, then assemble browser and MCP peer handlers.
- Preserve IPv4-only default/overrides and reject an IPv6 listen override for this PR so advertised reachability and accepted authority cannot diverge.
- Do not print the MCP key. Console output may print only the endpoint after enablement is observable through the paired browser.

#### 2. Pre-shutdown hook

**Files**: `loomspan-console/internal/webhost/host.go`, `host_test.go`, `loomspan-console/internal/console/service.go`

**Changes**:

- Add an internal pre-shutdown hook invoked once after process cancellation and before `http.Server.Shutdown`.
- Permanently freeze admission, cancel/drain requests, close current SDK sessions, and wait within the bounded shutdown choreography; then execute existing HTTP shutdown and browser/target cleanup.
- Preserve the canonical key on ordinary shutdown. Restart creates a fresh generation and no session/request state.
- Ensure profile/workspace invariant failure follows the same drain path.

### Success Criteria

#### Automated Verification

- [ ] Assembled tests prove startup ordering, exact endpoint, disabled/valid/invalid restart state, and no listener if profile/workspace ownership fails.
- [ ] Shutdown barrier tests prove MCP drain completes before `http.Server.Shutdown`, the key remains valid, locks release, and no request/session state is adopted after restart.
- [ ] Regeneration/disablement integration races prove no old-key response after commit and successful new-key reconnect.
- [ ] Browser bootstrap/target sentinel tests prove no regression or credential-realm crossover.
- [x] Go race suite passes on supported native runners: `go test -race ./...`.

#### Manual Verification

- [ ] Terminate the Console with an active client call, restart the same profile, and verify the same key reconnects while target application authentication is independently required.

---

## Phase 6: Conformance, Cross-Platform CI, Client Evidence, and Documentation

### Overview

Make protocol/platform behavior release-gating and document the fixed operational contract plus reproducible tiered-client evidence.

### Changes Required

#### 1. Official conformance harness

**Files**: `loomspan-console/mcp-conformance/` pinned runner declaration/config, harness launcher/tests, `.github/workflows/console-ci.yml`

**Changes**:

- Pin the official conformance runner revision/toolchain rather than using a floating install.
- Launch a real Console with an isolated temporary profile, enable it through an in-process setup hook that exercises the same store semantics, and run the applicable protocol-generic official scenarios for MCP `2025-11-25` and `2026-07-28` against exact `/mcp`.
- Carry no expected-failure baseline for applicable initialization, tool listing, DNS-rebinding, or caching cases. The official suite's stateless aggregate currently includes fixture-specific diagnostic tools alongside protocol checks and is therefore not runnable against PR 16's deliberately closed one-tool product surface; stateless lifecycle, discovery, and real `LOOMSPAN_get_runtime` calling remain mandatory in SDK and assembled HTTP tests. The same exclusion applies to fixture-specific tools, resources, prompts, sampling, and elicitation scenarios. Preserve actionable runner output as CI evidence on failure without exposing the key.
- Add a repository-standard wrapper entry so local and CI invocations use the same harness.

#### 2. Native platform and declaration CI

**Files**: `.github/workflows/console-ci.yml`, `loomspan-console/internal/buildtool/projectdeclarations_test.go`, optional buildtool runner phase

**Changes**:

- Add least-privilege native jobs for Linux x64, Windows x64, macOS ARM64, and macOS x64 credential/lifecycle tests; run Darwin commit tests on actual machines for both architectures.
- Keep full verification/build/browser E2E on the existing primary job, and add official conformance as a blocking job.
- Update workflow declaration tests to pin all new actions, runner/tool versions, required OS coverage, and conformance invocation.

#### 3. Representative-client procedure and evidence

**Files**: `loomspan-console/docs/mcp-client-compatibility.md` (or the repository's chosen Console documentation location), release-check evidence template

**Changes**:

- Record exact tested client versions and user/global setup for Codex, Claude Code, Antigravity, Cursor, and Windsurf/Cascade.
- Deep-test Codex and Claude Code connection, discovery, tool call, rotation, disablement, shutdown, restart, and reconnect.
- For available Antigravity, Cursor, and Windsurf/Cascade versions, smoke connection, bearer header, discovery, and tool call. Treat automation gaps as recorded manual evidence, not silent pass.
- Report hosted Devin as out of loopback transport scope. If a local Devin CLI independently supports MCP at implementation time, record it as an additional smoke surface.
- Never commit live credentials in evidence/configs; use redacted placeholders and environment-backed headers where supported.

#### 4. Consumer and operator documentation

**Files**: `loomspan-console/README.md`, `loomspan-console/release/README.md`, relevant security/architecture documentation

**Changes**:

- Document exact `/mcp`, IPv4 loopback scope, accepted authorities, fixed endpoint/body/time behavior, independent bearer realm, key persistence/location/format at the appropriate operator level, and restart/rotation/disable semantics.
- Document that YAML schema version 1 is unchanged and contains no MCP key/enabled/config section.
- Explain the disclosure warning, supported setup policy, no automatic/project configuration, hosted-loopback limitation, and the sole initial tool/capability.
- Remove the old future `/api/mcp/` reservation wording and stale stateful/IPv6/YAML assumptions where they appear in live consumer documentation; do not rewrite historical planning documents except through an explicit separately requested design-doc correction.

### Success Criteria

#### Automated Verification

- [x] Applicable protocol-generic official server conformance passes for both MCP revisions with no expected-failure baseline; SDK and assembled HTTP tests pass real `LOOMSPAN_get_runtime` discovery and calling.
- [ ] Native credential/lifecycle jobs pass on Linux x64, Windows x64, macOS ARM64, and macOS x64.
- [x] Standard Console verification passes: `go run ./internal/buildtool verify`.
- [x] Browser E2E passes against the assembled executable: `npm --prefix web run test:e2e`.
- [x] Repository-wide Java/API regression passes: `.\mvnw.cmd test` (or `./mvnw test` on Unix CI), including `LoomspanPublicSurfaceArchitectureTest`.
- [x] Declaration tests verify pinned SDK/conformance/toolchain/workflow versions and least-privilege permissions.

#### Manual Verification

- [ ] Tiered client matrix is completed with versions, results, reconnect observations, and transport-scope limitations.
- [ ] Review logs, CI artifacts, docs, fixtures, screenshots, browser storage/history, and committed files for credential sentinels.
- [ ] Validate the disclosure and setup copy with a developer who has not read the implementation.

---

## Testing Strategy

Create the dedicated test plan with `ai/commands/3_testing_plan.md` before implementation. It should map each ticket acceptance signal to a failing-first test, fixture/evidence owner, native runner, and exit criterion.

### Unit Tests

- Exact key parsing/generation, safe file inspection, state machine, commit error recovery, temporary cleanup, and platform operations.
- Tracker admission/freeze/drain/permanent-close races and generation checks.
- Authority/Origin/bearer grammar, disabled response, authentication-before-body, bounds, and failure redaction.
- Tool schema, status mapping, deterministic fallback, capability ordering, and invariant failure.
- Browser DTO/confirmation requirements and React state/key-lifetime behavior.

### Integration Tests

- Real HTTP SDK initialization/list/call under both protocol revisions.
- Enable → connect → in-flight call → regenerate → reconnect; enabled → in-flight call → disable; process shutdown/restart.
- Cross-realm rejection among MCP bearer, browser session/CSRF, pairing secret, and application key.
- Profile lock exclusion and independent concurrent profiles.
- Official conformance and native filesystem mechanics.

### Manual Testing Steps

1. Start from clean, valid-enabled, and invalid-key profiles and verify paired UI plus `/mcp` behavior.
2. Run the tiered representative-client procedure with exact released client versions.
3. Exercise kill/restart and mutation interruption scenarios on Linux, macOS, and Windows.
4. Inspect output, logs, network traces, browser state, and evidence artifacts for secret leakage.

## Performance Considerations

- Stateless MCP avoids a long-lived application session registry, but the outer tracker must cover slow/legacy requests without leaking goroutines.
- Authentication should use an immutable in-memory key snapshot and constant-time comparison; ordinary requests must not reopen or restat the key file.
- Freeze/drain is intentionally disruptive only for credential mutation and shutdown. Keep store preparation outside the frozen interval and bound shutdown/request waits.
- The bootstrap tool takes one in-memory status snapshot and has a fixed small response; it must perform no target I/O, catalog query, workspace repair, or other mutation.
- Body/header/time bounds apply before expensive parsing and prevent unauthenticated callers from consuming request-body resources.

## Migration Notes

There is no existing released MCP endpoint or valid key population to migrate. PR 16 creates only `mcp-access-key`; it keeps `.loomspan-console.lock`, configuration schema version 1, and all current target/browser state. `/api/mcp/` receives no alias because it was a reservation rather than a supported endpoint. If implementation encounters an `bfmcp_` file created by experimental local work, it is invalid canonical state and requires explicit paired removal; it is never upgraded silently.

Rollback to a build without PR 16 leaves the sibling key file untouched and ignored while retaining the profile lock/configuration. Reapplying PR 16 revalidates that canonical file normally. Operators may disable through the paired UI before rollback when they want canonical absence.

## References

- Original ticket: `ai/thoughts/tickets/loomspan-console-pr-16-mcp-foundation.md`
- Revalidated research and fixed decisions: `ai/thoughts/research/2026-08-13-loomspan-console-pr-16-mcp-foundation.md`
- Phase 3 architecture background: `ai/thoughts/phases/loomspan_console_phase_3_llm_runtime_inspector.md`
- Active roadmap: `ai/thoughts/phases/2026-08-12-loomspan-active-roadmap.md`
- Developer workflow catalog: `ai/thoughts/phases/loomspan_console_workflows.md`
- Compatibility policy: `ai/thoughts/framework-feature-design-lens.md`
- Console repository guidance and verification: `loomspan-console/AGENTS.md`
- Official SDK v1.7.0 release: https://github.com/modelcontextprotocol/go-sdk/releases/tag/v1.7.0
- Official MCP conformance: https://github.com/modelcontextprotocol/conformance

## Implementation Status (2026-08-13)

The production foundation, browser management surface, pinned conformance
harness, CI matrix, operator documentation, and automated Windows verification
are implemented. The local verification evidence is: canonical buildtool
verification green; 382 browser unit tests green; 30 Playwright tests green;
all Go packages green under the race detector; the focused lifecycle suite
green for 100 iterations; MCP conformance green for both revisions; Linux
x86_64 and Darwin x86_64/arm64 cross-compilation green; and the Maven reactor
green, including 862 starter tests and the public-surface architecture tests.

Unchecked criteria remain deliberately open where this workstation cannot
supply the required evidence or where the broader planned failure-injection
corpus was not added: native Linux/macOS execution, process-kill/interruption
testing, exhaustive commit-failure injection, the full representative-client
matrix, and independent developer review of disclosure copy. The new blocking
CI jobs provide native execution on Windows x86_64, Linux x86_64, macOS arm64,
and macOS x86_64. Manual client evidence is recorded as pending in
`loomspan-console/docs/mcp-client-compatibility.md`; no unavailable client was
reported as passing.
