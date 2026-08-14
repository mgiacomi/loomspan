---
date: 2026-08-13T14:09:01-07:00
researcher: Codex (GPT-5)
git_commit: 9040f476f255954f8d2d081496f19c980b12a434
branch: main
repository: loomspan
topic: "Loomspan Console PR 16 — MCP Authentication and Lifecycle Foundation"
tags: [research, codebase, loomspan-console, mcp, authentication, lifecycle, status]
status: complete
last_updated: 2026-08-13
last_updated_by: Codex
last_updated_note: "Resolved PR 16 foundation questions against current MCP, SDK, client, and platform evidence"
---

# Research: Loomspan Console PR 16 — MCP Authentication and Lifecycle Foundation

**Date**: 2026-08-13 14:09:01 PDT
**Researcher**: Codex (GPT-5)
**Git Commit**: `9040f476f255954f8d2d081496f19c980b12a434`
**Branch**: `main`
**Repository**: `loomspan`

## Research Question

Use `ai/commands/1_research_codebase.md` to research the current codebase for
`ai/thoughts/tickets/loomspan-console-pr-16-mcp-foundation.md`, with
`ai/thoughts/phases/loomspan_console_phase_3_llm_runtime_inspector.md` as
background. The ticket covers the MCP SDK and lifecycle spike, a profile-owned
credential store, paired browser key operations, Streamable HTTP security,
`LOOMSPAN_get_runtime`, and the complete `loomspan.runtime-status.v1`
capability.

The ticket file is modified in the working tree at the research commit. This
document describes that working-tree ticket and the live code at the commit
shown above.

## Summary

PR 16 is not implemented in the current checkout. The Go module has no MCP SDK
dependency, MCP adapter package, MCP credential store, MCP session registry,
MCP middleware, MCP tool registration, named Loomspan capability catalog, or
browser MCP settings screen. The shared router currently reserves
`/api/mcp/` as a browser-header, `no-store` 404 branch
(`loomspan-console/internal/webhost/routes.go:11-23`). The Phase 3 background
instead records the intended Streamable HTTP endpoint as `/mcp` and identifies
the official `github.com/modelcontextprotocol/go-sdk/mcp` module, planning-time
SDK release `v1.6.1`, and planning-time stable MCP specification `2025-11-25`
(`ai/thoughts/phases/loomspan_console_phase_3_llm_runtime_inspector.md:125-163`,
`:167-175`). Those planning-time external values are not pinned in `go.mod` and
were not freshly validated by any repository artifact.

The completed Console foundation already provides the major process seams on
which the ticket depends:

- `console.Run` opens and exclusively locks the profile, opens and cleans the
  separately locked workspace, creates the target and browser services, builds
  one handler after the actual listener authority is known, and serves it on
  one loopback-only `net/http` host
  (`loomspan-console/internal/console/service.go:46-122`, `:218-265`).
- The profile package resolves the configuration directory, verifies
  platform-specific protection, takes the fixed `.loomspan-console.lock`, and
  loads strict restart-only YAML before the listener exists
  (`loomspan-console/internal/profile/profile.go:12-52`).
- The workspace startup barrier validates a marked protected root, takes its
  own lock, removes prior transient entries, and captures filesystem identities
  before serving (`loomspan-console/internal/workspace/workspace.go:20-112`).
- Browser API security is route-scoped and ordered Host, special download
  policy or Origin, method, session, then CSRF where required; invalid Host is
  rejected before body reading
  (`loomspan-console/internal/browserapi/router.go:83-110`, `:228-252`;
  `loomspan-console/internal/browserapi/security_integration_test.go:92-106`).
- `consolecore.StatusSnapshot` already models target selection, connection,
  authentication, Java/Go compatibility, runtime identity, instance identity,
  and live-monitoring availability as independent facts
  (`loomspan-console/internal/consolecore/status.go:8-76`). The target context
  owns and snapshots those facts; browser bootstrap and target-status adapt
  them without an application request
  (`loomspan-console/internal/target/context.go:310-326`;
  `loomspan-console/internal/browserapi/router.go:254-285`;
  `loomspan-console/internal/browserapi/target.go:27-34`).
- Process cancellation and target-scope cancellation already have separate
  owners. `lifecycle.Coordinator` retains the first fatal process cause, while
  `target.Context` rotates an opaque target scope, cancels old scope work,
  invalidates registered owners, and prevents late probe publication
  (`loomspan-console/internal/lifecycle/coordinator.go:7-35`;
  `loomspan-console/internal/target/context.go:480-516`;
  `loomspan-console/internal/target/context_test.go:75-112`). No independent MCP
  authentication generation exists yet.

Targeted tests for profile, workspace, web host, browser API, lifecycle, target,
console core, and assembled console packages passed during this research.

## Detailed Findings

### 1. Current module and MCP implementation state

`loomspan-console` is an independent Go module. Its current direct dependencies
are `go.yaml.in/yaml/v4` and `golang.org/x/sys`; `golang.org/x/term` is indirect.
There is no `modelcontextprotocol` or alternative MCP module declaration
(`loomspan-console/go.mod:1-14`). A repository search finds no production MCP
package or protocol implementation.

The one existing MCP-specific production branch is the reserved route check:

1. `/api/console/v1/` is dispatched to the browser API.
2. `/api/mcp/` receives browser security headers, `Cache-Control: no-store`, and
   `404 Not Found`.
3. all other paths must pass the browser Host policy before static handling.

This routing order already selects the future MCP namespace before applying the
browser static-route policy, but it does not authenticate or process MCP
(`loomspan-console/internal/webhost/routes.go:11-32`). The Console README
describes browser/MCP route realms as selected before authentication and labels
the bearer realm as future behavior (`loomspan-console/README.md:207-213`).

The Phase 3 background records a different conceptual MCP path,
`http://127.0.0.1:<console-port>/mcp`, and stateful Streamable HTTP mounted under
Loomspan-owned middleware on the existing listener. It excludes an SDK-owned
listener, event replay, OAuth, sampling, elicitation, prompts, and
server-initiated subscriptions
(`ai/thoughts/phases/loomspan_console_phase_3_llm_runtime_inspector.md:138-163`,
`:167-187`).

### 2. Process assembly, listener, and shutdown lifecycle

`console.Run` currently assembles the process in this order:

1. open and lock the selected profile;
2. resolve, open, lock, validate, and clean the workspace;
3. build the application network policy and target context;
4. optionally supply the process-local application credential;
5. create the fatal lifecycle coordinator and profile/workspace monitors;
6. create browser pairing and session registries;
7. create live, observability, artifact, and trace-analysis services and
   register target-scope owners;
8. bind the loopback listener, derive its actual authority, construct the
   browser policy/router, create a pairing value, and compose the route handler;
9. mark target-owner registration closed and run the HTTP host.

The code is at `loomspan-console/internal/console/service.go:46-265`.
Consequently, the actual ephemeral listener port is known inside `Host.Prepare`
before the handler is returned. `AuthorityFromAddress` canonicalizes the bound
IPv4 or IPv6 loopback address into one Host and HTTP Origin
(`loomspan-console/internal/webhost/authority.go:8-25`), and its tests verify
both families and the actual port selected from `:0`
(`loomspan-console/internal/webhost/authority_test.go:13-43`).

The host accepts only an explicit loopback IP as its bind address. It configures
bounded HTTP header/read/write/idle timeouts, starts `http.Server.Serve`, and on
process cancellation gives `Shutdown` five seconds before returning the
coordinator cause (`loomspan-console/internal/webhost/host.go:25-95`). The host
currently binds one listener address; there is no paired IPv4/IPv6 listener set
or handler-wide protocol-session shutdown hook.

The lifecycle coordinator is process-wide and contains only a cancel-cause
context plus first-fatal `sync.Once` behavior
(`loomspan-console/internal/lifecycle/coordinator.go:7-35`). Browser session
shutdown cancels active browser relays and clears the browser registry
(`loomspan-console/internal/browserauth/sessions.go:151-162`). Target shutdown
cancels the current target scope, closes the application client, invalidates
scope owners, clears the application credential, and waits for serialized probe
work (`loomspan-console/internal/target/context.go:583-612`). No current type
tracks MCP `ServerSession` values, request cancellation functions, or an
MCP-authentication generation.

The Phase 3 lifecycle design keeps MCP generation independent from
`targetScopeId`: regeneration and disablement advance the MCP generation, close
old-generation sessions, cancel their operations, and perform a final
generation check before emitting a result without rotating target state
(`ai/thoughts/phases/loomspan_console_phase_3_llm_runtime_inspector.md:250-268`).

### 3. Profile ownership and key-file filesystem foundations

`profile.Open` resolves the configuration path and its safe canonical parent,
verifies the directory, takes an exclusive file lock, and then loads or creates
the config (`loomspan-console/internal/profile/profile.go:24-77`). The owning
`Profile` exposes the resolved profile directory and configuration path to
internal code, retains the lock until `Close`, and periodically checks lock
identity and directory protection (`loomspan-console/internal/profile/profile.go:14-22`,
`:113-137`; `loomspan-console/internal/profile/monitor.go:8-25`). Cross-process
lock exclusion is covered in `profile_test.go:90-121`.

Platform protection is already implemented:

- POSIX requires current-user ownership, mode `0700` for directories and
  `0600` for files, and rejects symlink path components
  (`loomspan-console/internal/profile/permissions_unix.go:12-77`).
- Windows rejects reparse-point path components, requires current-user
  ownership and a protected DACL, and permits ACEs only for the current user,
  `SYSTEM`, and built-in Administrators
  (`loomspan-console/internal/profile/permissions_windows.go:14-128`;
  `loomspan-console/internal/windowsacl/acl_windows.go:9-38`).

The profile has one internal `atomicCreate` helper for first-time configuration
creation. It creates a protected temporary sibling, writes and syncs the file,
closes it, rechecks canonical absence, and renames it into place
(`loomspan-console/internal/profile/profile.go:79-111`). There is no current
credential-store abstraction or production helper for canonical replacement,
deletion, key-file validation, temporary key-file cleanup, mutation
serialization, or generation publication.

The live lock basename is `.loomspan-console.lock`
(`loomspan-console/internal/profile/profile.go:12`). The Phase 3 background
records the planned canonical key name `mcp-access-key` beside the resolved
config and illustrates a `.loomspan-console-profile.lock` basename
(`ai/thoughts/phases/loomspan_console_phase_3_llm_runtime_inspector.md:801-810`).
No MCP sibling file is currently read during startup.

The planned persistent key representation is explicit in the background: 32
random bytes, encoded as `bfmcp_` plus 43 unpadded base64url characters, with
exactly one LF in the canonical file. File presence is the persistent enabled
state; YAML contains neither the key nor an enabled flag
(`ai/thoughts/phases/loomspan_console_phase_3_llm_runtime_inspector.md:228-246`).
None of this format is present in live Go types or tests.

### 4. Workspace startup barrier and persistent-state placement

The workspace is separate from the profile. `workspace.Open` requires or
creates a protected root, validates the exact `.loomspan-console-work` marker,
takes its own `.lock`, cleans `transient/`, and captures the identities of the
root, marker, and transient directory
(`loomspan-console/internal/workspace/workspace.go:20-112`). Restart cleanup is
tested to remove rather than adopt earlier entries
(`loomspan-console/internal/workspace/workspace_test.go:46-68`). `Check` verifies
the lock, path safety, protection, marker and directory identities, then performs
a create/write/sync/remove probe (`loomspan-console/internal/workspace/workspace.go:126-177`).

`console.Run` opens this workspace before creating browser credentials or the
listener, then starts profile and workspace invariant monitors on the process
coordinator (`loomspan-console/internal/console/service.go:47-72`, `:118-120`).
This is the live startup barrier corresponding to the Phase 3 rule that the MCP
credential belongs beside config, not under the disposable workspace. The
README documents that prior transient data is removed before listening and that
loss of the verified workspace invariant terminates the service
(`loomspan-console/README.md:172-182`).

### 5. Browser security realm and future credential operations

`browserapi.Router.ServeHTTP` applies `no-store`, validates Host before body
processing, applies a special same-site navigation policy only to raw artifact
downloads, otherwise requires exact Origin, then requires POST before dispatch
(`loomspan-console/internal/browserapi/router.go:83-110`). `withSession` checks
the `HttpOnly` browser session cookie and, for sensitive operations, requires
exactly one tab ID and CSRF header with a valid in-memory token
(`loomspan-console/internal/browserapi/router.go:228-252`). Request JSON is
bounded and rejects unknown fields and trailing values
(`loomspan-console/internal/browserapi/errors.go:10-63`).

The browser policy accepts only exact configured Host/Origin pairs. It requires
one supplied Origin for ordinary browser API calls; absent Origin is not a
browser exception (`loomspan-console/internal/browserapi/request_policy.go:54-69`).
The Phase 3 MCP policy is distinct: exact current-port loopback authority first,
an absent Origin permitted for non-browser clients, invalid supplied Origin
rejected, then MCP bearer authentication. Browser cookies, pairing values,
CSRF, application keys, and MCP keys remain non-interchangeable
(`ai/thoughts/phases/loomspan_console_phase_3_llm_runtime_inspector.md:189-210`).

No browser MCP routes or DTOs exist. The browser router currently ends with
activity operations and reports unknown console operations after that
(`loomspan-console/internal/browserapi/router.go:111-225`). The React route and
navigation catalogs end at Trace Storage
(`loomspan-console/web/src/app/routes.tsx:14-31`;
`loomspan-console/web/src/app/App.tsx:24-33`). `BootstrapResponse` contains the
browser process/version/workspace/tab/CSRF state, target snapshot, and target
form defaults only (`loomspan-console/web/src/api/contracts.ts:74-86`).

The Phase 3 background defines the future Settings → MCP Integration states as
Disabled, Enabled, and Disabled—invalid key file, plus paired enable, reveal,
regenerate, disable, and invalid-file removal operations
(`ai/thoughts/phases/loomspan_console_phase_3_llm_runtime_inspector.md:276-292`).

### 6. Shared runtime status and bootstrap mapping seam

The live transport-neutral status type is named `consolecore.StatusSnapshot`.
It serializes:

- `observedAt` and optional `targetScopeId`;
- independent target selection, connection, authentication, Java/Go
  compatibility, runtime identity, optional `instanceId`, and live-monitoring
  facts.

Its enum values and validation are in
`loomspan-console/internal/consolecore/status.go:8-85`. `NoTargetStatus` assigns
`NOT_APPLICABLE` to every target-dependent fact. Validation prevents a no-target
snapshot from carrying target facts and requires established runtime identity
and `instanceId` together.

`target.Context.Snapshot` takes the already-held target state and returns a
copy with a fresh observation time; it does not call the application
(`loomspan-console/internal/target/context.go:310-326`, `:563-568`). Browser
bootstrap embeds `targetResponse(router.targetSnapshot())`, and the separate
target-status operation decodes an empty bounded request then returns the same
snapshot mapping (`loomspan-console/internal/browserapi/router.go:254-285`;
`loomspan-console/internal/browserapi/target.go:15-34`). The corresponding
TypeScript `TargetStatus` duplicates the serialized enum contract
(`loomspan-console/web/src/api/contracts.ts:28-65`), and committed target
bootstrap fixtures cover no target, authentication required, and connected
states (`loomspan-console/browser-fixtures/target/`).

The Phase 3 design assigns `LOOMSPAN_get_runtime` to this same snapshot plus
MCP transport metadata and a deterministic named-capability set. It keeps
capabilities, current status facts, and individual operation results separate
(`ai/thoughts/phases/loomspan_console_phase_3_llm_runtime_inspector.md:835-873`).
For PR 16, the ticket names only `loomspan.runtime-status.v1`, whose required
operation is `LOOMSPAN_get_runtime`; later skill, execution, activity, and trace
families are recorded in the broader Phase 3 catalog but are outside this
ticket.

There is no current MCP mapping for `StatusSnapshot`, no MCP structured output
schema or text fallback, and no capability-conformance fixture.

### 7. Existing target lifecycle versus planned MCP authentication lifecycle

`target.Context` demonstrates the repository's current cancellation and
late-publication pattern for a different authority:

- authoritative target changes generate a new opaque scope;
- the old scope context is canceled and its client closed;
- registered owners are invalidated in registration order;
- credentials are preserved or cleared according to the target operation;
- probes serialize through `probeMu`, verify the captured scope, and cannot
  publish after rotation.

The rotation implementation is at
`loomspan-console/internal/target/context.go:480-516`; its authoritative-change
and late-result behavior is exercised at `context_test.go:52-112`. Scope
operations combine caller and target-scope cancellation in
`loomspan-console/internal/target/scope.go:31-105` and related stream methods.

This target lifecycle is independent of the planned MCP generation. The Phase
3 background explicitly keeps MCP key rotation from canceling browser work,
changing the selected target, or rotating `targetScopeId`
(`ai/thoughts/phases/loomspan_console_phase_3_llm_runtime_inspector.md:264-268`).
The current code contains no shared generic generation registry; the target and
browser registries are domain-specific internal implementations.

### 8. Tests, fixtures, and documented verification surfaces

Current executable evidence relevant to the foundation includes:

- profile creation/protection, same-process and cross-process lock exclusion,
  stable profile-derived workspace identity, and invariant monitoring
  (`loomspan-console/internal/profile/profile_test.go:15-146`);
- exact protected workspace layout, refusal of unmarked or weak roots, restart
  cleanup, link-safe cleanup, lock exclusion, and invariant monitoring
  (`loomspan-console/internal/workspace/workspace_test.go:15-219`);
- explicit loopback binding, actual-port authority derivation, and graceful
  HTTP cancellation (`loomspan-console/internal/webhost/authority_test.go:13-43`;
  `loomspan-console/internal/webhost/host_test.go:12-95`);
- browser Host/Origin policy and security-before-body ordering
  (`loomspan-console/internal/browserapi/request_policy_test.go:8-50`;
  `loomspan-console/internal/browserapi/security_integration_test.go:92-149`);
- independent status combinations
  (`loomspan-console/internal/consolecore/status_test.go:8-31`);
- target rotation, cancellation, owner invalidation, retry coordination, and
  late-result suppression (`loomspan-console/internal/target/context_test.go:52-512`);
- assembled pairing/bootstrap/shutdown and release of both profile and
  workspace locks (`loomspan-console/internal/console/security_integration_test.go:70-169`).

No `loomspan-console-fixtures` directory currently contains MCP protocol
messages, capability declarations, key files, client configurations, or MCP
conformance outputs. Existing application REST/SSE/artifact and NDJSON fixtures
belong to the Java-to-Go observability and diagnostic boundaries, not the
not-yet-created MCP boundary.

The targeted command run for this research was:

```text
go test ./internal/profile ./internal/workspace ./internal/webhost ./internal/browserapi ./internal/lifecycle ./internal/target ./internal/consolecore ./internal/console
```

All listed packages passed on Windows.

## Contract and Compatibility Inventory

### Application API

No current or planned PR 16 type changes the allowlisted Java Application API.
The live work is confined to the independent Go Console and its embedded
browser. No public Java signature exposes an MCP, Console internal, or
autoconfiguration type.

### Supported SPI

No supported Java or Go SPI exists for MCP. All live Go packages discussed here
are under the module's `internal/` tree. Interfaces such as browser
`ArtifactService`, `TraceAnalysisService`, `target.ScopeOwner`, and the host's
`ListenFunc` are internal assembly/test seams rather than a supported consumer
extension surface (`loomspan-console/internal/browserapi/router.go:23-59`;
`loomspan-console/internal/target/context.go:32-43`;
`loomspan-console/internal/webhost/host.go:12-20`).

### Configuration and manifest contracts

Console YAML schema version 1 is a strict, documented configuration contract.
Its live top-level fields are `version`, `listener`, `trace-workspace`, and
optional `target`; unknown fields are rejected
(`loomspan-console/internal/config/config.go:15-57`;
`loomspan-console/internal/config/decode.go:13-62`). There is no live `mcp`
section. The Phase 3 background assigns future restart-only, non-secret MCP
operational settings to such a section while excluding an enabled flag, key,
or authentication generation
(`ai/thoughts/phases/loomspan_console_phase_3_llm_runtime_inspector.md:789-816`).

The embedded browser asset manifest and release manifest are not currently MCP
capability manifests. Named Loomspan capabilities are not stored in any live
manifest.

### Persisted or serialized contracts

The live browser JSON contract has protected in-repository consumers in
`internal/browserapi`, `web/src/api/contracts.ts`, `web/src/api/client.ts`, and
the committed browser fixtures. `StatusSnapshot` is part of that serialized
boundary today.

The planned `mcp-access-key` filename, exact key/file format, and file-presence
enablement rule are persistent local-state contracts recorded only in the Phase
3 background. Standard MCP negotiation plus the Loomspan tool and capability
schemas will form a Go-to-MCP-client serialized boundary, but no live schema or
fixture exists yet.

The Java application REST/SSE/problem/artifact boundary remains a coordinated
current-release contract consumed by Go. PR 16's bootstrap reads Go's existing
status facts and does not add a Java endpoint or change consumed NDJSON.

### Ephemeral diagnostic formats

Current NDJSON traces, Go indexes, artifact handles, continuations, activity
windows, target scopes, browser sessions, and future MCP sessions are
current-process or current-release diagnostic state. The MCP access-key file is
different: the Phase 3 design deliberately persists it across process restart,
while MCP sessions and generations do not survive restart.

### Internal or accidentally exposed implementation

The exact Go package layout, credential-store constructor, middleware types,
session registry, SDK adapter DTOs, handler wiring, and lifecycle hooks are not
supported APIs. Their eventual exported Go identifiers, if any, remain hidden
from other modules by the `internal/` boundary. A Go `public` identifier inside
those packages would be technical exposure only within the module.

## Architecture Documentation

The current process and planned insertion point are:

```text
profile.Open + workspace.Open
    -> process lifecycle coordinator
    -> TargetContext + shared status/runtime services
    -> one explicit-loopback net/http listener
        -> route selection
            -> current browser policy/session/CSRF adapter
            -> reserved /api/mcp/ 404 branch

Phase 3 recorded boundary:
        -> MCP authority policy
        -> supplied-Origin policy
        -> independent bearer-key authentication generation
        -> stateful Streamable HTTP SDK adapter
        -> LOOMSPAN_get_runtime
        -> shared consolecore.StatusSnapshot
```

The MCP adapter is a peer of the browser adapter over shared Go services. It is
not another application client, target lifecycle owner, artifact cache, status
derivation, or Java observability endpoint. SDK types remain absent from the
shared service packages in the current code and are recorded as confined to the
future adapter in the Phase 3 design
(`ai/thoughts/phases/loomspan_console_phase_3_llm_runtime_inspector.md:55-100`,
`:136-152`).

## Historical Context (from `ai/thoughts/` and repository history)

- `ai/thoughts/phases/loomspan_console_phase_3_llm_runtime_inspector.md` records
  the settled product direction, security realms, credential persistence,
  lifecycle generation, tool/capability model, and client-surface assumptions.
  It repeatedly requires implementation-time revalidation of the evolving MCP
  ecosystem (`:3-7`, `:125-163`, `:948-974`).
- Commit `7963431` implemented PR 09, introducing the current `TargetContext`,
  application credential lifecycle, opaque target scopes, status snapshot,
  shared domain errors, browser target entry, and late-result tests. Those live
  components remain the shared target/status basis for PR 16.
- Commit `13acd9c` implemented the main PR 15 diagnostic workflow hardening, and
  follow-up commits through `0ee98ef` recorded hosted release evidence. Its live
  effects include the current browser workflows, response bounds, trace service
  boundaries, packaging, and CI/release documentation that PR 16 follows rather
  than duplicates.
- `ai/thoughts/framework-feature-design-lens.md:13-70` supplies the contract
  categories used above and distinguishes deliberately supported contracts from
  technical visibility, tests, fixtures, and existing behavior.

## Related Research

The earlier PR 09 and PR 15 research documents were removed from the current
working tree during later cleanup, but remain available in repository history:

- commit `7963431`,
  `ai/thoughts/research/2026-07-26-bifrost-console-pr-09-target-context.md`;
- commit `13acd9c`,
  `ai/thoughts/research/2026-08-01-diagnostic-workflows-phase-2-hardening.md`.

They use the repository's former Bifrost name. Live Loomspan code and current
documents are the primary source for this research.

## Open Questions

The following facts are not established by live repository code or executable
evidence:

- the then-current stable MCP specification and exact stable official Go SDK
  release, compatibility table, public session APIs, and conformance behavior;
- whether the final route is `/mcp`, the currently reserved `/api/mcp/`
  namespace, or another exact path;
- the exact static YAML `mcp` fields and defaults, if any, within schema version
  1;
- the final profile-lock basename relationship between the live
  `.loomspan-console.lock` and the Phase 3 illustrative
  `.loomspan-console-profile.lock`;
- the exact Go ownership and shutdown ordering among credential store, MCP
  generation/session registry, SDK handler, HTTP host, target context, browser
  registries, and process coordinator;
- atomic create/replace/delete and directory-durability behavior for the MCP key
  on each supported platform, including interruption outcomes;
- how the single-listener host represents the Phase 3 accepted-authority set
  when IPv4 and IPv6 loopback are both enabled;
- the exact MCP structured output, concise text fallback, error mapping, server
  identity, transport metadata, and capability-conformance fixtures for
  `LOOMSPAN_get_runtime`;
- representative client reconnect/reinitialization behavior after regeneration,
  disablement, and Console shutdown; and
- current correctness of the client-specific configuration examples recorded in
  the Phase 3 background.

## Follow-up Research 2026-08-13 14:15 PDT

### Follow-up question

Do the active documents under `ai/thoughts/phases/` or the future PR 17–20
tickets answer the open questions above? Where they do not, identify the exact
questions that remain so PR 16 planning does not silently inherit ambiguity.

### Document set reviewed

- `ai/thoughts/phases/2026-08-12-loomspan-active-roadmap.md`
- `ai/thoughts/phases/loomspan_console_phase_3_llm_runtime_inspector.md`
- `ai/thoughts/phases/loomspan_console_workflows.md`
- `ai/thoughts/tickets/loomspan-console-pr-17-mcp-runtime-inspection.md`
- `ai/thoughts/tickets/loomspan-console-pr-18-mcp-trace-inspection.md`
- `ai/thoughts/tickets/loomspan-console-pr-19-debugging-skill.md`
- `ai/thoughts/tickets/loomspan-console-pr-20-structured-logging.md`

The active roadmap preserves the Phase 3 design and workflow catalog as active
inputs, and sequences PR 16 before PRs 17–19. PR 20 is independent structured
logging work and does not answer any MCP foundation question
(`ai/thoughts/phases/2026-08-12-loomspan-active-roadmap.md:49-78`). The workflow
catalog explicitly leaves protocol conformance, transport security,
credentials, malformed input, and cancellation to implementation testing and
leaves exact routes, field spelling, harnesses, and interoperable response
values as implementation work
(`ai/thoughts/phases/loomspan_console_workflows.md:43-49`, `:97-101`).

### Resolution audit of the original open questions

| Original question | Documentary status | What the documents establish |
|---|---|---|
| Current stable MCP specification and official SDK | **Still open; intentionally requires fresh external validation** | Phase 3 records only the planning-time `2025-11-25` specification and official Go SDK `v1.6.1`, then explicitly requires implementation-time revalidation of the stable specification, SDK compatibility table, and representative client support (`loomspan_console_phase_3_llm_runtime_inspector.md:7`, `:125-163`, `:952-957`). No later ticket supplies fresher values. |
| Final HTTP route | **Partially answered, but exact spelling still needs confirmation** | Phase 3 consistently illustrates and generates `http://127.0.0.1:<actual-port>/mcp` (`:167-175`, `:294-311`). The workflow catalog says exact route spelling remains implementation work (`loomspan_console_workflows.md:97-101`), and live code reserves `/api/mcp/`. No future ticket reconciles those facts. |
| Exact static YAML `mcp` fields and defaults | **Still open** | Phase 3 establishes a strict restart-only `mcp` section containing only non-secret listener/protocol settings, with no `enabled`, key, or generation field (`loomspan_console_phase_3_llm_runtime_inspector.md:789-816`). It does not name any fields or defaults. PRs 17–20 do not do so. |
| Profile-lock basename | **Target design answered; live-code transition unrecorded** | Phase 3 shows `.loomspan-console-profile.lock` and says the profile-lock and key-file basenames are fixed rather than configurable (`:801-810`). Live code uses `.loomspan-console.lock`. No document states whether PR 16 changes the live name or the Phase 3 name. |
| Credential store, generation registry, and shutdown ownership | **Responsibility boundaries answered; concrete ordering remains spike work** | One profile-owned credential store owns file validation/mutation, in-memory key state, serialization, and generation (`:228-266`). A Loomspan-owned generation/session registry retains SDK sessions and request cancellations; the SDK handler does not own listener lifecycle (`:138-163`). Shutdown must end sessions and streams, but the exact call/order choreography with `http.Server.Shutdown`, registry closure, and process cancellation is not specified. |
| Atomic file mutation and durability | **Observable semantics answered; platform mechanics still open** | Phase 3 defines enable/regenerate/disable commit points, protection rules, old-or-new crash outcomes, and “durably flush as supported” (`:242-266`). It does not establish the exact POSIX directory-sync or Windows replacement/deletion APIs and guarantees. PR 16 explicitly retains this research focus. |
| IPv4/IPv6 listener and accepted-authority topology | **Authority policy answered; serving topology still open** | The accepted authority set is exact current-port `127.0.0.1`, `localhost`, and `[::1]` only when IPv6 loopback was enabled (`:189-210`). Setup guidance emits a `127.0.0.1` endpoint (`:294-311`). No document defines how the current single-listener host enables a second IPv6 listener on the same selected port or whether the initial implementation does so. |
| `LOOMSPAN_get_runtime` semantics, output, server identity, and metadata | **Core semantics answered; exact adapter contract still open** | The operation returns the shared side-effect-free status, selected scope, named capabilities, and needed MCP transport metadata; capability, status, and operation result remain separate (`:401`, `:835-873`). Standard MCP initialization owns protocol negotiation, server identity, and operation discovery (`:818-833`). Exact structured schema, concise text fallback, transport metadata fields, server identity values, and error wrapper are not spelled out. |
| Reconnect/reinitialization after key changes and shutdown | **Required behavior answered; actual clients remain unverified** | Old-generation work must be canceled and suppressed; sessions close; clients reconnect and initialize again; a process upgrade retains the key but not the session/tool catalog (`:156-163`, `:250-268`, `:833`). Actual behavior remains a required PR 16 lifecycle and representative-client spike (`:976-978`). |
| Client-specific setup examples | **Concrete intended examples exist; currency remains intentionally unverified** | Phase 3 records local Codex, Claude Code, Cursor, Antigravity, Devin Desktop/Windsurf/Cascade, and local Devin CLI configuration shapes and forbids repository/URL/shell-history key placement (`:294-311`). It also explicitly requires revalidating actual client surfaces (`:952-953`, `:1002`). PR 19 later owns final installation/distribution and broader interoperability evidence (`loomspan-console-pr-19-debugging-skill.md:20-26`, `:52-57`), but PR 16 still owns proving its generated connection guidance and reconnect behavior. |

### Scope clarified by future tickets

PR 17 does not absorb unfinished PR 16 bootstrap design. It begins after PR 16
and adds skills, executions, and recent activity. Its detailed planning owns the
general structured-content, continuation, resource-interoperability, safe-text,
response-envelope, protocol-error, and capability-conformance patterns for
those new families (`ai/thoughts/tickets/loomspan-console-pr-17-mcp-runtime-inspection.md:11-32`,
`:50-54`). PR 16 therefore still needs one complete structured and fallback
contract for its own `LOOMSPAN_get_runtime` operation.

PR 18 owns trace query/resource/continuation and raw-range questions, not MCP
authentication, bootstrap, or session lifecycle
(`ai/thoughts/tickets/loomspan-console-pr-18-mcp-trace-inspection.md:10-33`,
`:63-70`). PR 19 owns the canonical skill, final client evaluations,
installation, distribution, and missing-capability guidance, while preserving
MCP-independent usability (`ai/thoughts/tickets/loomspan-console-pr-19-debugging-skill.md:10-26`).
Neither ticket supplies missing PR 16 foundation decisions.

### Questions that should be surfaced before detailed PR 16 planning

The document review reduces the unresolved set to the following concrete
questions:

1. **External baseline:** What is the current stable MCP specification, what
   exact stable release of `github.com/modelcontextprotocol/go-sdk` matches it,
   what lifecycle APIs does that release actually expose, and does it pass the
   applicable official server conformance suite with Loomspan's stateful
   Streamable HTTP shape?
2. **Canonical route:** Is `/mcp` the exact released endpoint, superseding the
   current `/api/mcp/` reservation, or is the route deliberately different?
3. **Static MCP configuration:** Does the initial release need any `mcp` YAML
   fields beyond the existing shared listener configuration? If so, what are
   their exact names, types, defaults, bounds, and restart semantics?
4. **Persistent names:** Does PR 16 change the live profile lock from
   `.loomspan-console.lock` to the Phase 3
   `.loomspan-console-profile.lock`, or does the canonical design retain the
   live basename? Separately, is the Phase 3 key prefix `bfmcp_` intentionally
   retained after the Bifrost-to-Loomspan rename, or must the persisted key
   format use a Loomspan-specific prefix?
5. **Filesystem commit mechanics:** Which exact POSIX and Windows operations
   implement exclusive enable, atomic replacement, durable commit, deletion,
   parent-directory durability where available, safe temporary cleanup, and the
   documented crash outcomes?
6. **Listener topology:** Is the initial server IPv4-only with `localhost`
   accepted as request authority, or does it create coordinated IPv4 and IPv6
   loopback listeners? If it creates both, how are one advertised endpoint,
   actual port selection, partial bind failure, and shutdown represented?
7. **Lifecycle choreography:** What exact registry hooks and ordering close
   SDK `ServerSession` objects, cancel request contexts, suppress late results,
   stop new authentication admission, and coordinate key mutation with HTTP and
   process shutdown?
8. **Bootstrap contract:** What are the exact MCP server name/version identity,
   `LOOMSPAN_get_runtime` input/output schema, capability ordering,
   transport-metadata fields, concise text fallback, unsuccessful-result/error
   mapping, one-response bound, and conformance fixture?
9. **Representative clients:** For each initially named local client, what
   current configuration actually connects, how does it carry the bearer key,
   does it display structured output/resources, and how does it behave after
   regeneration, disablement, shutdown, and restart?

These questions are all within PR 16's existing outcome or detailed-planning
focus. The future tickets refine later runtime, trace, skill, and logging
surfaces; they do not defer these foundation questions away from PR 16.

## Decision Resolution 2026-08-13

This section resolves the nine surfaced questions using the product preferences
recorded after the follow-up audit and fresh implementation-time validation.
It supersedes planning-time assumptions where noted; it does not rewrite the
historical codebase findings above.

### 1. MCP specification, Go SDK, and transport mode

Use the official `github.com/modelcontextprotocol/go-sdk/mcp` module pinned to
`v1.7.0`, target MCP `2026-07-28`, and configure
`StreamableHTTPOptions.Stateless = true`.

Both values became final on 2026-07-28. The official SDK release states that
`v1.7.0` fully supports `2026-07-28`, preserves compatibility with
`2025-11-25` and earlier, and negotiates older stateful clients down. The new
wire version removes the initialization session from each tool call and carries
protocol/client metadata per request. This is a better match for Loomspan than
the planning-time stateful design because the initial server deliberately has
no sampling, elicitation, roots, server-initiated calls, subscriptions, or
event replay. Each request may therefore own a temporary SDK session and close
it on completion.

The official conformance runner distinguishes the stateful `2025-11-25`
requirements from the stateless `2026-07-28` requirements. PR 16 should run the
server requirements for both revisions against the same `/mcp` endpoint. It
should not carry an expected-failure baseline for required initialization,
tool-listing, or tool-call scenarios.

This decision removes the proposed long-lived Loomspan session registry. The
adapter still owns an admission/request tracker for credential rotation,
disablement, and process shutdown. `mcp.Server.Sessions()` and
`ServerSession.Close()` are available as defensive drain hooks, including for
temporary sessions, but they are not a new application persistence model.

Primary external evidence:

- https://github.com/modelcontextprotocol/go-sdk/releases/tag/v1.7.0
- https://github.com/modelcontextprotocol/conformance
- https://blog.modelcontextprotocol.io/posts/2026-07-28-release-candidate/

### 2. Canonical route

Release exactly `/mcp`. It is the conventional endpoint used by the MCP
specification examples, official conformance runner, SDK examples, and current
client documentation. Remove the live `/api/mcp/` reservation rather than
shipping an alias or redirect. MCP is a peer route realm to the browser API,
not a browser-API operation, so `/api/console/v1/*` remains the browser realm
and `/mcp` remains the MCP realm.

### 3. Static YAML configuration

PR 16 should add no `mcp` YAML section or fields. Examples of fields that were
being considered by the phase wording were an MCP path, IPv6 enablement,
maximum request-body size, request timeout, session idle timeout, or protocol
mode. None needs to be user-configurable initially:

- path is fixed at `/mcp`;
- serving is initially IPv4 loopback only;
- the request-body bound and timeouts are safe implementation constants;
- protocol negotiation belongs to MCP and stateless mode is an implementation
  contract, not a profile choice;
- persistent enablement remains solely the presence of `mcp-access-key`.

Strict schema version 1 therefore remains unchanged. A future field should be
added only when a demonstrated user need cannot be satisfied safely by a fixed
default. The access key, `enabled`, and credential generation must never become
YAML fields.

### 4. Persistent names and key format

Keep the live profile lock basename `.loomspan-console.lock`; the longer phase
name was illustrative and provides no migration benefit. Keep the canonical
credential filename `mcp-access-key`.

Use a Loomspan key prefix, not the legacy Bifrost prefix:

```text
lsmcp_<43 unpadded base64url characters>\n
```

The suffix encodes 32 cryptographically random bytes. The canonical file is
exactly 50 bytes: six ASCII prefix characters, 43 base64url characters, and
one LF. Parsers reject any other length, prefix, alphabet, padding, newline
shape, file type, owner, or protection.

### 5. Cross-platform credential-file commits, including macOS

Use one profile-owned credential store and serialize all mutation under the
already-exclusive profile. Create a protected random temporary sibling, write
the complete key, flush and close it, perform one same-directory commit, then
flush the parent directory where the platform supports that operation. Never
truncate or rewrite the canonical key in place.

Platform implementations should be split behind small build-tagged file
operations:

- **Linux:** `renameat2(..., RENAME_NOREPLACE)` for enable, ordinary atomic
  rename-over for regenerate, `fsync` for the temporary file and profile
  directory, and unlink plus directory `fsync` for disable. A carefully tested
  same-directory `linkat` fallback may be used only when `renameat2` is not
  supported.
- **macOS:** `renameatx_np(..., RENAME_EXCL)` for enable, ordinary atomic
  rename-over for regenerate, `F_FULLFSYNC` for the temporary key where
  supported, directory `fsync` after namespace changes, and unlink plus
  directory `fsync` for disable. Test both Darwin architectures on an actual
  macOS runner; do not classify macOS merely as a generic `!windows` case for
  commit mechanics.
- **Windows:** create the temporary sibling with create-new semantics and its
  protected DACL, call `FlushFileBuffers`, then use `MoveFileExW` without
  `MOVEFILE_REPLACE_EXISTING` plus `MOVEFILE_WRITE_THROUGH` for enable and with
  both flags for regenerate. Disable uses `DeleteFileW`. Revalidate the
  canonical owner, DACL, regular-file/reparse status, and contents after each
  successful mutation. Windows has no equivalent portable parent-directory
  flush guarantee, so the contract remains “durably flush as supported.”

On startup, remove only Loomspan-owned temporary siblings that match the exact
temporary basename pattern and pass the same regular-file, ownership,
protection, and no-link checks. A malformed canonical file is never silently
replaced: startup reports the disabled-invalid state, and only the paired
remove-invalid operation may delete it.

The externally testable interruption outcomes remain canonical absence or one
complete valid old/new key. Power-loss durability beyond the strongest
documented platform primitive is not claimed.

### 6. IPv6

Ship PR 16 IPv4-loopback-only and advertise
`http://127.0.0.1:<actual-port>/mcp`. Accept exact `127.0.0.1:<port>` and
`localhost:<port>` Host authorities, but do not accept `[::1]` until an IPv6
listener is actually bound.

IPv6 offers only a modest local-product benefit here: it helps clients that
resolve `localhost` to `::1` and cannot fall back to IPv4. Using the literal
advertised IPv4 URL avoids that problem. Supporting both families would require
coordinated same-port binds, platform-specific dual-stack socket behavior,
partial-bind failure rules, two serve loops, combined shutdown, authority-set
publication, and a larger Windows/macOS/Linux test matrix. That is meaningful
complexity without adding reachability for the initial local-only product.

Keep the authority policy capable of adding `[::1]:<port>` later. IPv6 should
become a follow-up only if representative client evidence shows a real
IPv4-loopback interoperability problem.

### 7. Credential mutation and shutdown choreography

The concrete race to prevent is: an old-key tool request begins, the user
regenerates or disables MCP, and that request otherwise emits evidence after
the credential-file commit. Use this ordering:

1. Prepare and flush a replacement temporary key before disrupting clients
   when regenerating.
2. Freeze new MCP admission. Requests that have not passed authentication do
   not enter the SDK.
3. Cancel all tracked MCP request contexts, close the current snapshot of SDK
   sessions defensively, and wait for admitted handlers to exit. The outer
   tracker includes long-lived or legacy HTTP requests.
4. Commit the key-file mutation atomically. If commit fails, retain/publish the
   old credential state and reopen admission; already connected clients merely
   reconnect.
5. Publish the new in-memory credential state and monotonically increasing
   authentication generation only after the filesystem commit succeeds.
6. Reopen admission.

Because draining completes before commit, no old-key result can be written
after the new persistent state becomes authoritative. Tool handlers still
compare their captured generation immediately before constructing a successful
result as defense in depth. Generation is independent of `targetScopeId` and
does not alter browser sessions, target selection, application authentication,
or Loomspan execution.

Process shutdown uses the same primitive: permanently freeze admission, cancel
tracked MCP requests, close current SDK sessions, wait for the tracker, and
then let the shared HTTP server complete its bounded shutdown. The credential
file is not removed on normal process shutdown.

### 8. Bootstrap schema, identity, and errors

“Schema” here does not mean a new Loomspan schema version or a replacement for
the existing status model. MCP tool registration unavoidably declares JSON
Schema for tool arguments and structured results. Keep that adapter contract
minimal:

- server identity name: `loomspan-console`;
- server identity version: the existing complete Loomspan product release
  string;
- tool: `LOOMSPAN_get_runtime`;
- input: an empty object with unknown properties rejected;
- structured result: a sorted `capabilities` array plus the existing serialized
  `consolecore.StatusSnapshot` under `status`;
- sole initial capability: `loomspan.runtime-status.v1`.

Do not add another surface-wide MCP schema version. Do not copy the negotiated
MCP protocol version, server identity, endpoint, transport, or authentication
mode into the tool result: initialization and the connected HTTP transport
already carry those facts. Do not alter `StatusSnapshot` merely for MCP.

Return the same result as structured content and a deterministic concise text
fallback containing the capability, target scope, selection, connection,
authentication, compatibility, runtime identity, instance identity, live
status, and observation time. The fallback contains no access key and is
bounded to a small fixed size.

`LOOMSPAN_get_runtime` is side-effect-free and succeeds for no-target,
disconnected, authentication-required, incompatible, and connected states;
those are status facts, not tool failures. Unknown arguments are invalid MCP
parameters. An unexpected adapter/invariant failure returns an unsuccessful
tool result with a safe stable `INTERNAL` code and concise text, while transport,
authentication, malformed JSON-RPC, and unsupported-protocol failures remain
HTTP/MCP-layer failures. A golden fixture should assert exact property names,
enum spellings, sorted capability order, structured/text agreement, and absence
of secrets.

### 9. Representative client policy and choices

There are three reasonable compatibility policies:

1. **Protocol-only:** gate on official conformance and the official Go SDK
   client. This is reproducible but can miss product-specific header/config and
   reconnect problems.
2. **Tiered clients:** gate on official conformance, SDK black-box tests, and
   deep lifecycle smoke tests in Codex plus Claude Code; also run and document a
   connection/tool-discovery smoke test for every available named local client.
   PR 19 later performs the full skill and presentation evaluation.
3. **All-client hard gate:** make every lifecycle and presentation behavior in
   all five product families block PR 16. This offers broad evidence but makes
   Loomspan's protocol foundation depend on volatile, partly unautomatable
   third-party UI behavior.

Choose **tiered clients**. Codex and Claude Code both currently document
Streamable HTTP and bearer-header configuration and provide the clearest
repeatable lifecycle baseline. Antigravity currently documents `serverUrl` and
custom headers. Cursor documents Streamable HTTP, but its product-specific
behavior has changed enough that an actual-version smoke test is more reliable
than treating a copied config shape as a permanent contract. Use local
Windsurf/Cascade as the fifth local client: its current documentation supports
remote HTTP `serverUrl`/`url`, headers, and environment interpolation. Hosted
Devin cannot reach loopback and is reported as out of transport scope rather
than counted as a server failure. If a local Devin CLI exposes independent MCP
client configuration at implementation time, smoke-test it as an additional
surface rather than substituting undocumented assumptions for Windsurf.

For every client, prefer an environment-backed authorization header when the
client supports it. Otherwise emit a user/global, locally protected config
snippet with a placeholder and let the user paste the revealed key there.
Never emit a project/repository config containing the key, put the key in the
URL, automatically modify client configuration, or recommend a command that
places the key in shell history. Regeneration intentionally requires clients to
receive the new key and reconnect; disablement produces an authentication
failure until MCP is re-enabled and configured again.

Current primary client documentation:

- Codex: https://learn.chatgpt.com/docs/extend/mcp?surface=cli
- Claude Code: https://code.claude.com/docs/en/mcp
- Antigravity: https://antigravity.google/docs/mcp
- Cursor: https://docs.cursor.com/context/model-context-protocol
- Windsurf/Cascade: https://docs.windsurf.com/plugins/cascade/mcp

### Resulting PR 16 baseline

The resolved baseline is: official Go SDK `v1.7.0`; MCP `2026-07-28` with
backward-compatible stateless Streamable HTTP; exact `/mcp`; no new YAML fields;
IPv4 loopback only; `.loomspan-console.lock`; `mcp-access-key` containing an
`lsmcp_` key; build-tagged Linux, macOS, and Windows atomic file operations; a
request admission/drain generation rather than a persistent MCP session
registry; and one minimal adapter envelope around the existing runtime status.
