# Loomspan Console — Phase 2 Personal UI Console

## Status

Initial product and architecture direction. This document records decisions established during early planning and identifies the UI questions still to resolve. It is not yet an implementation plan.

## Related design

Phase 2 depends on the observability contracts and safety properties defined in [loomspan Console — Phase 1 Observability Foundation](./LOOMSPAN_console_phase_1_observability_foundation.md).

Approved end-to-end developer workflows and the product requirements they surface are recorded in [loomspan Console — Developer Workflows](./LOOMSPAN_console_workflows.md). Workflow design consumes the settled Phase 1–3 architecture and does not supersede it.

Phase 1 owns engine publication, activity projection, active-execution state, trace discovery, and the Spring web adapter. Phase 2 owns the separately running developer console and its browser experience.

Phase 1 also owns the execution-observation lifecycle through one optional internal handle per execution. Successful canonical appends update that handle, and the guaranteed core completion boundary closes it exactly once. The handle alone coordinates terminal activity, current-process catalog publication, and active-entry removal; disabled observability uses a no-op handle. Go consumes the resulting contracts and must not create a second execution-lifecycle owner.

Phase 2 also owns **Go console-side trace analysis**. The application exposes its finalized current UTF-8 NDJSON trace files without repackaging; their compatibility is included in `consoleCompatibilityVersion`. The core Java trace subsystem continues to own each canonical file and its persistence or grace deletion, while the optional application adapter owns only current-process catalog metadata and authenticated streaming access. Go shared services parse acquired artifacts and provide frame trees, timelines, summaries, filtering, search, pagination, failure indexes, usage attribution, and other derived developer views to both browser and future MCP adapters. These query and analysis features are not Phase 1 application-server responsibilities.

## Product character

Loomspan Console is initially a **personal developer instrument that uses a browser for its interface**. It is not initially a shared observability service, fleet-management platform, or execution control plane.

Its purpose is to help a developer:

- understand what a running Loomspan application can do;
- see what Loomspan executions are doing now;
- follow a concise live execution narrative;
- understand execution paths, timing, usage, retries, and failures; and
- inspect traces currently cataloged by the selected application process deeply enough to perform a developer audit.

In this context, developer audit means reconstructing and understanding framework behavior. It does not mean business, regulatory, compliance, immutable, or long-term archival audit.

## Phase 2 objective

Deliver a compact standalone console executable that serves a rich browser UI and connects to the supported Phase 1 observability web adapter without depending on Loomspan Java internals or trace filesystem layout.

The initial product should support one developer, one console process, and one selected Loomspan application at a time.

## Chosen architecture

```text
browser
    -> loopback HTTP
    -> Go console host
        -> user-configured HTTP or HTTPS
        -> application Spring server
            -> Loomspan observability web adapter
            -> observability core
```

### Go console host

The console host is a separate Go application responsible for:

- serving the compiled browser application;
- embedding the browser assets into the executable;
- managing the selected Loomspan target connection;
- holding target credentials outside browser storage;
- calling Phase 1 REST endpoints;
- subscribing to the Phase 1 SSE activity stream;
- retaining one bounded current-scope recent-activity window for browser reconnect and transport-neutral on-demand queries;
- relaying activity to the browser through that window and bounded per-tab delivery state;
- reporting connection and protocol compatibility state;
- assigning a console-local opaque target scope and rejecting or discarding stale target work;
- applying target timeouts and certificate trust configuration;
- exposing transport-neutral runtime query and analysis services below browser handlers;
- parsing and analyzing finalized traces from the exactly matched application contract;
- maintaining the current-scope user-controlled transient trace cache and its indexes for interactive browser and future MCP queries; and
- proxying trace downloads.

The console host is a backend-for-frontend and the owner of console-side trace analysis, not another authoritative observability database or execution engine.

### Browser application

The browser UI is a client-rich React and TypeScript single-page application built with Vite. React Router owns browser routing. Tailwind CSS provides layout, spacing, typography, responsive behavior, and interaction-state styling over a small Loomspan semantic token system. A deliberately small authored CSS layer owns resets, global theme and accessibility rules, and specialized visualizations for which utility classes would obscure the design. The initial console does not adopt a Tailwind component or theme library.

A rich client is preferred because the trace and live-execution experiences require hierarchical navigation, incremental updates, selection preservation, filtering, expandable detail, coordinated views, and deep links.

The browser communicates only with its same-origin Go console host. It does not connect directly to the Loomspan application.

The browser owns the in-memory live view it renders. It obtains current snapshots through Go, applies relayed Phase 1 activity envelopes to its local UI store, and owns navigation, filtering, selection, expansion, and other presentation behavior. It does not parse raw NDJSON traces or independently calculate authoritative hierarchy, duration, usage, failure, or availability facts that browser and MCP must share.

### Frontend foundation

The initial frontend uses React, TypeScript, Vite, and React Router. It uses React Aria selectively for interaction-heavy accessible primitives such as menus, dialogs, tabs, tables, and hierarchical navigation while retaining loomspan-owned presentation. The implementation should begin with explicit domain stores built from ordinary React composition, context, and reducers rather than adding a general-purpose global state-management library. A new state dependency requires a demonstrated cross-cutting need that the simpler model cannot serve clearly.

Frontend domain state preserves the console's evidence boundary. It may deterministically format, group, sort, filter, and relate recorded facts, and it may calculate presentation values explicitly authorized by this plan. It does not classify evidence as important, surprising, excessive, justified, causal, or actionable.

The production browser baseline is Vite's pinned release-time **Baseline Widely Available** target. The console does not initially add legacy-browser transforms or polyfills. The build records the exact Vite and frontend dependency versions in the package lock so a later Vite baseline change occurs only through an intentional dependency update.

The initial accessibility target is WCAG 2.2 AA. All operations are keyboard available; focus remains visible and unobscured; updates do not steal focus; routes manage focus and announce navigation appropriately; semantic landmarks, headings, tables, controls, and tree interaction patterns are used; color is not the sole carrier of status or relationships; and status announcements do not turn every live activity item into an interruption. The UI respects reduced-motion and forced-colors preferences. Automated accessibility checks support keyboard, focus, semantic, zoom, responsive, reduced-motion, and forced-colors verification. Phase 2 does not require or claim a manual screen-reader or representative assistive-technology acceptance pass.

The console is desktop-first and responsive rather than fixed-width. Coordinated views may stack or become alternate tabs as space narrows. Normal application navigation does not require whole-page horizontal scrolling; data tables, raw records, and intrinsically wide evidence may scroll within labeled regions. Browser zoom through 200 percent remains usable. Phone-specific workflow optimization is outside the initial release, but pairing, connection status, trace-storage management, and basic evidence views remain readable.

The initial theme choices are light, dark, and follow-system. Theme selection is scope-bound browser presentation state under the existing `sessionStorage` policy. Forced-colors and reduced-motion preferences override decorative choices where required.

The initial frontend does not adopt a general charting, graph, or visualization library. Execution hierarchy uses semantic HTML and accessible tree behavior. Timelines begin with HTML and SVG derived from recorded timestamps and deterministic durations; usage views use tables and simple CSS or SVG presentation; raw records remain structured content. Specialized visualization dependencies require an implementation-proven accessibility or rendering need and must not redefine hierarchy, ordering, evidence relationships, or diagnostic meaning.

Frontend verification uses Vitest, React Testing Library, and Playwright. Component tests cover deterministic presentation and interaction behavior; browser tests cover routing, keyboard paths, accessibility-critical behavior, live updates, target-scope resets, and the approved workflows. The console build runs frontend tests and creates a clean production asset tree before Go tests and compilation; release verification rejects missing or stale embedded assets.

### Spring web adapter

The Phase 1 Spring web adapter registers REST and SSE routes in the application's existing Spring web server. It does not start a second listener, configure TLS, change bind addresses, or expose internal Java objects as the protocol.

The initial adapter may support Spring MVC only. A non-web Loomspan application does not silently become a web application by adding observability core functionality.

## Reasons for keeping the Go host between browser and loomspan

The Go host:

- avoids configuring CORS on every observed application;
- keeps target credentials out of browser storage and out of browser runtime after an intentional credential-submission request completes;
- centralizes target connectivity, certificate trust, timeouts, and compatibility handling;
- prevents the UI from depending on application topology or trace storage layout;
- provides a natural SSE relay and future fan-out boundary;
- enables a compact standalone executable;
- keeps the console operationally independent from the Java framework being monitored; and
- discourages accidentally sharing internal Java types between the engine and console, keeping the protocol explicit.

A static SPA that connects directly to Loomspan is not a planned deployment mode.

## Relationship to the existing CLI

The existing `loomspan-cli` is a deprecated proof of concept. It may be consulted temporarily for selected frame-tree, duration, failure-identification, and retrospective-debugging behavior, but it is not a supported predecessor, compatibility target, distribution companion, or fallback interface for Loomspan Console. It will be removed after the Loomspan Console implementation is complete.

Its filesystem-oriented trace loading, Go record types, filename parsing, and CLI presentation state model predate the supported Phase 1 web protocol. Implementation planning may deliberately salvage an algorithm or test idea, but new console code must not depend on the CLI project or preserve its types, directory discovery, architecture, commands, or behavior merely to ease migration. Phase 2 does not copy artifacts into a CLI directory or provide cross-process CLI discovery. A developer may manually point the legacy CLI at a specific readable directory while it still exists, but that is opportunistic raw-file inspection outside the supported console lifecycle.

## Project placement, build, distribution, and operational independence

The console initially lives in this repository as the top-level `loomspan-console/` project. It is an independent Go module, not a Maven module and not a continuation of `loomspan-cli`. Its initial source layout is:

```text
loomspan-console/
  go.mod
  go.sum
  cmd/loomspan-console/
  internal/
  web/
    package.json
    package-lock.json
```

`cmd/loomspan-console` is the executable entry point, `internal` contains the transport-neutral services plus browser and MCP adapters, and `web` contains the React/TypeScript/Vite source and frontend tests. The exact internal package subdivision and generated-asset directory are implementation details, but generated production assets remain inside the Go module where a dedicated asset package can embed them.

Keeping Java, Go, browser, MCP, fixtures, and release automation in one repository allows one change and one release tag to update the consumed contract and every consumer atomically. Moving the console to another repository would require a future independent-version and cross-repository compatibility design.

The compiled browser assets are embedded into the Go executable so the initial console is distributed as one compact runtime artifact. The production browser and Go host are consequently one atomically built and distributed component, not independently versioned peers. The initial product has no browser API compatibility version or runtime browser-to-Go negotiation.

The Java Loomspan artifacts and Go console executable form one coordinated Loomspan release unit. They receive the same product release version and the repository release pipeline builds and publishes them from the same tag, including when a release changes only the console or only Java. Maven continues to build and publish the Java artifacts; ordinary Java development and `mvn test` do not require Go or Node unless the console build is explicitly requested. The components remain separate at runtime despite release coupling.

### Toolchains and reproducibility

Implementation scaffolding pins one exact current stable Go patch and one exact current Active LTS Node.js patch plus npm version. The Go module/toolchain declaration, `go.sum`, Node version declaration, `package.json` package-manager and engine declarations, and committed `package-lock.json` make those selections explicit. CI and the release build use exactly those versions. The initial project does not claim support for a range of build toolchains; toolchain changes are intentional dependency-maintenance changes.

Node and npm are build-time dependencies only. The runtime executable contains the compiled browser assets and requires neither Node nor a frontend package installation.

### Build and verification composition

A production console build performs this sequence:

1. install the locked frontend dependency graph;
2. run Vitest and React Testing Library coverage;
3. clean the prior generated production asset tree and run the Vite production build;
4. verify that the expected entry document and content-addressed asset manifest were produced;
5. run Go tests, including embedded-asset and shared-service tests; and
6. compile the versioned Go executable using only the freshly generated assets.

Playwright workflow and browser integration tests run in repository CI before release packaging. The release pipeline never trusts a pre-existing generated asset directory, and verification fails when assets are absent, stale, or carry a different product version. Java/Go contract fixtures and adapter-parity tests run in the same tagged source state even though the Java and console build commands remain independently usable.

### Initial release targets and packages

The initial release publishes:

- Windows x86-64;
- Linux x86-64; and
- macOS Apple Silicon.

Other operating systems or architectures may build from source but are not initially published, tested, or claimed as supported. Adding another supported target requires its release build and representative platform validation, not merely a successful untested cross-compilation.

Each target is packaged as one console executable with the applicable license and short runtime README, plus published checksums. The initial release does not add an installer, operating-system package-manager integration, automatic updater, container image, separate browser deployment, or npm package. The deprecated `loomspan-cli` is not included.

Running the console should not require:

- a JVM;
- Node.js or a frontend development toolchain at runtime;
- a separate static web server;
- shared filesystem access to the observed application; or
- a database.

### Development hot reload

Development may run Vite separately for hot reload. Vite binds only to loopback, is the browser's development origin, and proxies only the browser console API and live-event paths to the loopback Go host. Go remains the sole component that contacts the selected Loomspan application, owns application credentials, and supplies browser runtime semantics. MCP clients connect directly to the Go listener and never through Vite. The Go host explicitly accepts only the configured loopback Vite development origin in this mode; the proxy does not weaken production Host, Origin, pairing, CSRF, or authentication rules.

The development proxy and its allowances are excluded from production builds. The production/release artifact serves only compiled embedded assets. Production static assets use content-addressed names and may be cached immutably, while the entry document must be revalidated or not stored so it cannot keep selecting an earlier asset set after a console restart. The initial product does not install a service worker or other offline cache that can preserve an earlier SPA independently of the executable. A page holding a process-local browser session against a restarted console bootstraps again, re-pairs, or reloads; it does not negotiate an old browser DTO contract with the new process. Independently deploying the browser requires a future explicit compatibility design.

## State ownership

State should remain deliberately divided:

- **loomspan application:** authoritative skill catalog, active executions, activity stream, and current-process trace catalog.
- **Go console:** selected target configuration and opaque target scope, in-memory target credential, connection and exact-version compatibility state, one upstream SSE connection, a bounded recent-activity window exposed through transport-neutral query services, per-tab relay state, browser session pairing, and the current-scope user-controlled transient trace cache.
- **Browser:** the current rendered live-execution view derived from snapshots and relayed activity, plus navigation, filters, selections, expanded nodes, pane sizes, and other presentation state.

The application remains authoritative. Go must not continuously materialize a complete duplicate of the skill catalog, active-execution registry, or execution history merely to serve browser-shaped state. It may request or briefly coalesce current snapshots, retain a bounded ring of already-received activity envelopes for browser reconnect and on-demand recent-activity queries, and use its disposable local work directory to acquire, parse, and index traces selected by the developer from the application's current-process catalog. The trace cache follows its configured aggregate byte and idle-TTL policy, including the explicit `unlimited` and `never` choices; it is not application history or a durable cross-process archive. The activity ring is a short-lived best-effort window, not execution history. All application-derived relay, catalog, execution, and trace state belongs to the current opaque `targetScopeId` and follows the authoritative [`TargetContext` ownership](#go-targetcontext-ownership) lifecycle. It is cleared on scope rotation or console shutdown and is never authoritative. A database is explicitly out of initial scope.

The console is deliberately current-process-only. Core filesystem retention is not console history: `ALWAYS` may leave a trace file on disk after the producing application process exits, and a shutdown or crash may abandon a grace-held `NEVER` or successful `ONERROR` file before its process-local deletion task runs. This is an accepted initial lifecycle tradeoff. Neither the application adapter nor Go scans, adopts, serves, or cleans such a file in a later process. Application restart empties the supported application catalog, rotates `targetScopeId` after authoritative identity revalidation, invalidates prior-scope browser state and deep links, and clears prior-scope Go-acquired copies. Physical file survival does not preserve console discoverability.

Within a running application process, the application-side trace catalog retains small bounded metadata entries by age. `loomspan.observability.trace-catalog-metadata-ttl`, configured on the observed application and defaulting to `24h`, determines that lifetime. The catalog has no independent entry-count or aggregate-metadata-memory cap; its total memory is proportional to current-process completions still inside the configured window. Catalog expiry only removes discoverability and acquisition metadata. It never deletes, shortens, or extends the canonical trace file's core retention. This application setting is separate from the Go console's local `trace-workspace.max-bytes` and `trace-workspace.idle-ttl` policy.

## Local configuration

Phase 2 owns one versioned, strictly validated local YAML configuration file in the platform-appropriate user configuration directory, with a `--config` option for an alternate location. The resolved configuration file's parent directory defines one console profile. That profile owns the YAML file, the Phase 3 sibling MCP credential, one exclusive profile lock, and one resolved managed work directory. It covers console listener behavior, selected-target defaults, target timeouts and trust configuration, the trace-cache byte and idle-TTL policy, and other non-secret operational settings.

Unknown fields are rejected, ordinary durations and byte sizes use explicit units, and unsafe or nonpositive numeric values are rejected clearly. `trace-workspace.max-bytes` additionally accepts the explicit value `unlimited`, and `trace-workspace.idle-ttl` accepts `never`; numeric zero means neither. Changes to this static YAML initially require restart. Secrets are not stored inline in it. Before using the profile, the Go process acquires its exclusive operating-system profile lock and retains it until shutdown. Failure to acquire that lock means another console owns the profile and fails startup clearly; the later process must select a different profile rather than reading configuration or mutating credentials concurrently. Phase 3 extends the YAML with restart-only, non-secret MCP operational settings and places the persistent MCP access key in a separate protected sibling file. Runtime enablement, regeneration, and disablement mutate only that credential file and the process-local MCP authentication snapshot under the same profile ownership; they are not YAML configuration changes and take effect immediately.

Persistent configuration, trust material, the profile lock, and the Phase 3 MCP access-key file never live inside the disposable work directory described below. There are no user-data or configuration exceptions inside its `transient` subtree. One owning console process may serve multiple paired browser sessions and multiple authenticated MCP clients; those clients do not acquire the profile lock or become additional console/MCP server processes.

## Disposable local work directory

The Go console uses one visible console-owned work directory rather than scattering acquired traces across platform temporary directories. By default it resolves one stable directory for the resolved config profile beneath the platform-appropriate per-user local state or cache location and captures its absolute path once at process startup:

```text
<platform-user-local-root>/<profile-scoped-loomspan-console-work>/
  .loomspan-console-work
  .lock
  transient/
```

Each resolved config profile gets its own default work directory; changing the process current working directory does not change it. The exact platform root and profile-to-directory encoding are implementation details, but the mapping must be deterministic and must not cause two distinct profiles to share a default directory. `--work-dir <path>` replaces the default with that exact work-directory path rather than adding another child. A developer who wants a repository-visible directory may explicitly select one with this option. The console prints the resolved absolute path at startup and exposes it in paired status so developers can find it.

The marker identifies a directory created and managed by Loomspan Console; it is not an authentication credential. On first use, Go creates the root, marker, lock file, and `transient` child with owner-only access or the closest enforceable platform equivalent. On every later startup it resolves and verifies the exact root, rejects a root that is a symbolic link or Windows reparse point, requires the expected marker, and acquires an exclusive operating-system lock before deleting anything. If the named directory already exists without the valid marker, Go treats it as unrelated user data and refuses to clean or use it. It never guesses ownership from the directory name alone.

After acquiring the profile lock and then the work-directory lock, startup deletes and recreates only the known `transient` child. This fixed lock order avoids competing startup paths. Cleanup must remain beneath the verified work root and must not follow symbolic links, junctions, or other reparse points encountered within it. The marker and work-directory lock are console ownership metadata and are not part of the trace cache. A second console process cannot share this work directory, even through a different profile: failure to acquire the lock produces a clear startup conflict and requires a separate profile with a different work directory; it must never clean an active console's files.

Every acquired artifact, partial download, index, reconstructed-payload file, and other application-derived disk state belongs under `transient`. Go never scans, adopts, indexes, or serves its contents from a prior console process. Startup cleanup occurs before target connection, trace acquisition, browser service, MCP service, or any listener begins serving. Thus a crash may leave protected files until this same profile-scoped work directory is used again, but the next successful startup removes them rather than recovering them. This is ordinary cleanup, not secure erasure.

Target-scope rotation and graceful shutdown delete current transient artifacts best-effort after cancelling their users; graceful shutdown may leave the empty managed root, marker, and unlocked lock file. Explicit raw downloads saved by a browser belong to the developer-selected download location and are not console-workspace files, so workspace cleanup does not remove them.

The work directory is a mandatory console dependency, not an optional trace-analysis feature. Failure to resolve, create, identify, protect, lock, safely clean, or verify it is a fatal startup error, and the process exits before opening its listener or serving browser or MCP requests. Go does not fall back to a shared, weakly protected, or uncertain directory and does not silently reuse partially cleaned contents.

After startup, any loss of the work-directory lock, path-safety guarantees, or general required ability to manage console-owned workspace content is a fatal console error. Go stops admitting work, cancels in-flight operations, closes browser and MCP service, attempts ordinary best-effort transient cleanup where it remains safe, releases its locks, and exits with an error. It does not remain available in a reduced-function mode. A malformed artifact, configured-capacity rejection, artifact-specific validation failure, or disk-full/write-capacity failure confined to one acquisition remains an ordinary request failure when its partial state can be removed and safe workspace operation is restored. If the console cannot restore the ability to use the managed workspace correctly after that cleanup, the process-wide fatal rule applies. Console workspace failure must not alter or terminate the observed Loomspan application.

## Transport policy

### Browser to Go console

Initial local operation uses HTTP with a strict loopback boundary:

- bind explicitly to `127.0.0.1` and optionally `::1`;
- never bind the HTTP listener to `0.0.0.0`, a LAN address, or a public interface;
- validate expected loopback `Host` and `Origin` values;
- use a one-time browser pairing mechanism;
- never put target credentials in browser URLs, browser storage, or logs; and
- do not provide a configuration switch that exposes the plaintext console listener remotely.

HTTPS is deferred until remote console access becomes a real requirement. That future feature may include user-provided certificates and Let's Encrypt/ACME integration for owned domain names.

### Go console to Loomspan application

The observed application owns its transport policy. The console accepts an intentionally configured HTTP or HTTPS target.

- HTTP targets are permitted.
- HTTPS targets use ordinary certificate and hostname verification.
- Private infrastructure may supply a custom CA bundle.
- The console identifies an HTTP target persistently as unencrypted without blocking it or repeatedly demanding confirmation. The warning explains that the application observability key and returned diagnostic data may be observed or modified by parties with access to the network path.
- The console must not silently weaken HTTPS verification.

Loomspan controls the safe formation and authorization of the observability data it emits. The host application controls how its observability routes are transported and exposed; transport confidentiality and integrity depend on the configured HTTP or HTTPS target.

## Local browser pairing

The personal console should not require user accounts, passwords, or OIDC. The current direction is a lightweight per-process pairing flow:

1. The console generates a high-entropy one-time pairing secret at startup.
2. It prints or opens a pairing URL.
3. The browser exchanges the secret once.
4. The console establishes a short-lived, `HttpOnly`, `SameSite=Strict` browser session.
5. The one-time secret becomes invalid.
6. Browser sessions disappear when the console exits.

The exact URL/exchange design must avoid placing reusable credentials in browser history or server logs.

## Target authentication and authorization ownership

The Go target client supports only authenticated Loomspan observability endpoints. A configured target URI identifies the application base including any externally visible servlet or reverse-proxy context path; Go appends the fixed `/_loomspan/observability/v1/` namespace. The application always authenticates and authorizes the supplied application observability key. The browser talks only to its paired loopback Go host and does not require CORS access or send the application key directly to the observed application.

For the personal console, the initial upstream authentication mechanism is the high-entropy static application observability key configured by the Loomspan application. Go supplies it to the application as `X-loomspan-Api-Key: <key>`; it is entered through the paired browser workflow or a protected interactive console prompt and retained only in Go process memory. If entered in the browser, the key necessarily exists briefly in the input element, browser process, JavaScript submission state, and same-origin HTTP request to the loopback Go server; browser developer tools and sufficiently privileged extensions may observe it. It is not persisted in browser storage and is cleared from browser application state after the request completes. The protected interactive console prompt is the alternative for a developer who does not want the key to pass through the browser. Go uses the key for both browser-initiated and enabled MCP-initiated operations against the selected target. It must not be placed in the console's ordinary configuration file, command-line arguments, URLs, browser storage, or logs.

Possession of the application observability key establishes the application's sole `LOOMSPAN_OPERATOR` authority. This is a deliberately narrow machine-to-machine authentication mechanism for the initial personal developer product, not a user identity, role matrix, or integration with every authentication scheme used by the host application.

The target-client boundary accepts a credential provider and passes the key in the fixed `X-loomspan-Api-Key` header to the application, which remains the only authority that validates it. A `401` carrying Phase 1 `LOOMSPAN_API_KEY_REJECTED` transitions the selected target to an unauthenticated state and maps to `TARGET_AUTHENTICATION_REQUIRED`; the developer must supply a new key. A generic `401` or `403` without that code maps to `TARGET_ACCESS_BLOCKED` and is presented as probable host Spring Security or upstream-proxy interference rather than mislabeled as an invalid Loomspan key. User accounts, refresh tokens, automatic login flows, Basic authentication, mTLS client identity, OAuth, alternative authentication headers, and provider-specific authentication are future credential-provider features.

The console deliberately uses **acquisition-time authorization** for application-derived evidence. `LOOMSPAN_API_KEY_REJECTED` blocks every later operation that requires a new application request or SSE connection, but it does not rotate `targetScopeId`, invalidate a complete Go-acquired artifact handle, erase the bounded recent-activity window, or revoke another complete current-scope result already admitted into Go. Those copies remain accessible to an otherwise authorized paired browser or MCP client until their normal idle or capacity eviction, target-scope rotation, or console shutdown. Supplying replacement application credentials rotates `targetScopeId` under the existing rule and therefore clears prior-scope evidence.

Acquisition-time authorization never turns an incomplete transfer into cached evidence. If application authentication fails before or during a multi-request acquisition, or a trace download does not complete and validate, Go closes and deletes the partial state and issues no artifact handle. Evidence retained after rejection must preserve its original observation and cursor facts. An operation that promises current application state or requires refresh still returns `TARGET_AUTHENTICATION_REQUIRED`; adapters must not relabel an older cached observation as current merely because its scope has not yet rotated.

Successful browser pairing/session authentication is logged without recording the pairing secret, cookie, upstream application observability key, or diagnostic content. Per-request or operation-level access logging is not initially required.

### Application-data forwarding rule

The application-to-consumer rule is: **console authentication secrets are never returned as diagnostic data; Loomspan does not detect or redact secrets embedded by the observed application in recorded content.** Console authentication secrets include the upstream application observability key and `X-loomspan-Api-Key` header, plus the console's pairing secret, browser session cookie, CSRF token, MCP access key, and other authentication headers. Those values remain part of their respective authentication or explicit credential-management flows rather than runtime, skill, activity, or trace results.

This rule does not classify the contents of application data. A value inside a skill YAML file, prompt, model request or response, application-provided model configuration, activity event, trace record, payload, tool input or output, error, or metadata remains application observability content even if it looks like a password, token, key, credential, personal data, or other sensitive value. Go must not scan, redact, sanitize, or omit it in the initial release. Browser and MCP adapters expose the same underlying application content; summaries, pagination, and caller-selected detail may shape individual responses but must not create adapter-specific disclosure filters. All application-provided diagnostic text is untrusted data. Go may parse it and calculate deterministic results from it, but must never interpret it as commands. The initial release does not add a universal provenance wrapper or content-classification model.

## Protocol handling inside the Go host

The Go host should not create a second semantic activity protocol merely because it relays SSE. The starting bias is:

- Phase 1 activity envelopes pass through with their startup-scoped `instanceId`, delivery cursor, canonical sequence, timestamp, frame, route, kind, and details intact.
- The Phase 1 `EXECUTION_OBSERVATION_ENDED` exception also passes through intact but is not presented as a canonical trace projection. It may omit canonical sequence and execution outcome and reports only that observation ended incompletely because core diagnostic finalization failed.
- The Go host may add console-owned connection events, but those must be namespaced or transported separately so they cannot be confused with Loomspan execution activity.
- REST responses exposed to the browser may be proxied or mapped into explicit Go/browser DTOs, but the mapping must preserve Phase 1 semantics.
- The Go console and application adapter ship as a coordinated Loomspan release pair. Their complete shared product release string, including qualifiers such as `-SNAPSHOT`, is also their exact `consoleCompatibilityVersion`. They must match before using REST snapshots, SSE activity, or downloaded NDJSON traces. The authenticated instance-status request is the compatibility probe, and Go reads only its stable top-level compatibility field until a match is established. This release-derived umbrella covers all Java-to-Go runtime compatibility. There are no separately reported or negotiated engine, adapter, Go-release, trace-schema, trace-container, or independent protocol compatibility versions.
- Phase 1 removes `TraceRecord.schemaVersion`; raw NDJSON records contain no independent version property. Go interprets a trace only under the exact umbrella match and `targetScopeId` from which it was acquired.
- A `consoleCompatibilityVersion` mismatch produces `INCOMPATIBLE_TARGET` before the UI attempts partial rendering or trace analysis. Invalid trace content produces `INVALID_ARTIFACT`, not negotiation of another version.

The initial implementation keeps protocol tooling deliberately small:

- the application adapter uses explicit hand-authored Java boundary DTOs rather than serializing internal classes;
- the Go target client uses corresponding hand-authored wire structs and maps them into transport-neutral services;
- the browser-facing Go and TypeScript DTOs are hand-authored for the local same-release API rather than generated from the Java boundary;
- Java adapter integration tests produce reviewed representative JSON snapshot, JSON problem, SSE activity, and native NDJSON fixtures;
- Go contract tests consume the Java-produced application fixtures and assert compatibility gating, parsing, cursor/problem behavior, and shared semantic results;
- frontend mocks and tests consume browser-facing Go fixtures rather than pretending to call the Phase 1 Java API directly; and
- exact route names, serialized field spelling, fixture-directory layout, and test-harness organization are settled while implementing the boundary.

The initial release does not add OpenAPI, a separate SSE schema language, JSON Schema, a schema registry, or generated Java, Go, or TypeScript protocol models. The plans already define the semantic agreement and the coordinated release model makes executable fixtures the useful drift guard. Generation may be reconsidered only if implementation demonstrates meaningful repetition or drift that the reviewed fixture tests do not control cleanly.

## Go `TargetContext` ownership

The Go console has one authoritative `TargetContext` boundary for the selected application. It is the sole owner allowed to create or rotate the console-local `targetScopeId` and to commit the target identity used by shared Console, browser, and MCP services. Status handlers, the upstream SSE connection manager, trace acquisition, analysis services, browser handlers, and MCP handlers must not independently adopt a target, credential context, or application `instanceId`.

`TargetContext` owns the normalized target address and connection-authority settings, an opaque credential generation and its protected credential provider, current authentication, connection, and exact-version compatibility status, the established application `instanceId`, and the lifecycle of the one upstream SSE connection. Connection-authority settings include the target scheme and authority, externally visible context path, redirect or proxy policy where supported, and certificate-trust policy. The raw application credential remains behind the target-client or credential-provider boundary and is never included in snapshots given to browser, MCP, analysis, or other consumers.

Every target operation captures an immutable `TargetContext` scope snapshot when it begins. The snapshot supplies the current `targetScopeId`, established runtime identity and compatibility facts, and access to an appropriately scoped target client without exposing the credential. Before returning a result, installing an artifact or index, updating shared state, or publishing an event, the operation verifies that its captured scope is still current. `TargetContext` may publish successive immutable snapshots with the same `targetScopeId` as connection, authentication, compatibility, or availability status changes; equality of scope does not imply that those status facts remain unchanged.

A new Go console process creates a fresh `targetScopeId`. Replacing the selected target, changing connection-authority settings, or accepting replacement target credentials rotates it immediately. Credential replacement rotates the scope without comparing the old and new secret values. The first successful authenticated status check for that newly selected context establishes its application `instanceId` without rotating the scope a second time.

After identity has been established, only a serialized reconnect status check or new upstream SSE handshake may propose a changed `instanceId` to `TargetContext`. A different value is committed only through scope rotation. If another authenticated target response carries a different `instanceId`, the receiving service rejects that response and requests serialized status revalidation; it does not adopt whichever concurrent response finishes last. The SSE manager may report handshake identity to `TargetContext`, but it cannot commit that identity itself.

Ordinary timeout changes, authentication failure, exact-version incompatibility, and temporary transport disconnect or reconnect update current status but do not rotate the scope. A reconnect also preserves the scope when the selected target, credential context, and revalidated `instanceId` remain unchanged. Supplying replacement credentials after an authentication failure does rotate it. Operational timeout changes apply to later operations without invalidating otherwise usable current-scope diagnostic state. In particular, explicit upstream authentication rejection prevents new target access but leaves complete previously acquired evidence under its normal local lifecycle; this is the authoritative acquisition-time authorization rule, not an accidental cache behavior.

Rotating `targetScopeId` cancels prior-scope operations, closes the obsolete upstream stream, and directs scope-bound services to clear all application-derived state: skill-file entries and YAML content, active snapshots, activity relay and replay state, acquired trace files, indexes, derived analysis, and browser/MCP runtime views. Those services continue to own their bounded data; `TargetContext` coordinates the boundary rather than becoming a store for every catalog, activity envelope, or trace. Cancellation is advisory, so completed late work is still rejected by the scope check. A partial artifact is closed and deleted rather than admitted to the workspace. A browser download already in progress is cancelled; bytes already delivered cannot be recalled, but the incomplete result must not be accepted or cached as a valid artifact.

Every target-specific REST response and SSE event includes the scope snapshot's `targetScopeId`. It is an opaque UUID whose only supported operation is equality; it is not a numeric counter. All Go-owned target continuations, acquired-artifact handles, cached entries, and derived references bind to the scope in which they were created. Presenting any prior-scope reference or continuation produces `TARGET_CHANGED` rather than an empty page, a generic stale-context result, or reinterpretation against the current target. The binding may be an opaque server-side reference and does not require a signature because the local APIs are already authenticated.

Browser pairing and session state, MCP authentication state, selected-target configuration, and the protected in-memory target credential are console-owned and may survive an application-runtime identity reset. Browser and MCP consumers remain connected to the console but lose all prior application-derived data and must query the new target scope. A temporary disconnect may retain current-scope state while application identity is unknown; if authoritative revalidation reveals a different identity, `TargetContext` completes the full rotation before any new-runtime result is published.

A new Go console process always creates a fresh `targetScopeId` and never reloads the upstream application credential, even when non-secret selected-target defaults and the separate MCP access key persist. If a target is selected but no application credential has yet been entered, the shared status reports `targetSelection: SELECTED`, `targetAuthentication: REQUIRED`, `javaGoCompatibility: NOT_CHECKED`, `runtimeIdentity: NOT_ESTABLISHED`, and `liveMonitoring: UNKNOWN`. MCP transport initialization may still succeed with its valid local key, while every operation requiring new application access returns `TARGET_AUTHENTICATION_REQUIRED`. The paired developer supplies the application key again without changing the IDE's MCP credential.

## Transport-neutral console status snapshot

Browser and MCP status must adapt from one transport-neutral Go `ConsoleStatusSnapshot` rather than independently interpreting `TargetContext` or application availability state. This is a small successful-result contract paired with the shared service error contract below. It is a read-only projection of facts the console already owns, not a health subsystem, target probe, recovery workflow, or additional source of truth. A process serving this snapshot necessarily passed workspace startup validation and has not detected a fatal workspace failure.

The initial conceptual fields and states are:

| Fact | States or value | Source and meaning |
|---|---|---|
| `observedAt` | Timestamp | Time Go assembled the snapshot. It does not assert that Go probed every subsystem at that instant. |
| `targetScopeId` | Opaque ID or absent | Current `TargetContext` scope; absent when no target is selected. |
| `targetSelection` | `NONE`, `SELECTED` | Whether a target configuration is currently selected. |
| `targetConnection` | `NOT_APPLICABLE`, `UNKNOWN`, `REACHABLE`, `UNAVAILABLE` | Most recently committed connection fact from `TargetContext`, not an active status request performed for this snapshot. |
| `targetAuthentication` | `NOT_APPLICABLE`, `UNKNOWN`, `REQUIRED`, `ESTABLISHED`, `BLOCKED` | Distinguishes no target, not yet established, missing or rejected Loomspan key, successful application authentication, and rejection by host security or an upstream proxy. |
| `javaGoCompatibility` | `NOT_APPLICABLE`, `NOT_CHECKED`, `COMPATIBLE`, `INCOMPATIBLE` | Result of the exact `consoleCompatibilityVersion` check. `NOT_CHECKED` is not a mismatch. |
| `runtimeIdentity` | `NOT_APPLICABLE`, `NOT_ESTABLISHED`, `ESTABLISHED` | Whether the current scope has committed the application's startup-scoped `instanceId`. This is explicit rather than inferred from a missing ID. |
| `instanceId` | Opaque ID or absent | Generated by the application on startup, changed on every restart, and present only after compatible authenticated status establishes runtime identity. |
| `liveMonitoring` | `NOT_APPLICABLE`, `UNKNOWN`, `AVAILABLE`, `UNAVAILABLE` | The last authoritative `liveMonitoringAvailable` fact from a compatible application instance. It is not inferred from whether an SSE connection happens to be open. |

The exact serialized DTO spelling may be settled with the local API, but these distinctions and meanings are shared-service semantics. In particular, false-like or absent values must not conflate no selected target, an unattempted check, authentication rejection, exact-version mismatch, temporary target unavailability, or disabled live monitoring. Runtime identity fields appear or disappear together and remain bound to the reported `targetScopeId`.

Snapshot assembly performs no upstream request, workspace repair, credential mutation, SSE reconnect, artifact acquisition, or catalog query. It captures one current immutable `TargetContext` snapshot and reads the current live-monitoring fact from its existing owner. These independently owned facts need not be made transactional with each other; `observedAt` describes a best-current-known projection, and a later operation may observe a newer state. Consumers may refresh status when useful, but the initial product adds no status polling service, historical health log, hysteresis, or recovery state machine.

There is deliberately no aggregate `healthy`, `ready`, or `degraded` field. Different target operations require different facts: a query over an existing artifact handle may succeed while upstream authentication is required, and skill or trace-catalog access may continue while live monitoring is unavailable. Browser and MCP presentation may summarize these independent target facts for a human, but shared services and adapters must not branch on an invented combined state. Workspace failure is not represented here because it terminates the console.

`traceCatalogReachable` is deliberately not a retained status fact. The application trace catalog is not an independently enabled health subsystem, and checking it merely to build status would add network work and stale last-success state. Selection, connection, authentication, and compatibility explain whether a catalog request can be attempted; the catalog operation's current success or shared domain error remains authoritative. Skill-catalog reachability, current artifact availability, and other per-operation outcomes follow the same rule.

MCP enablement is also not a target-readiness fact in this shared snapshot. The paired browser may add the credential store's enablement or invalid-key-file state as a separate console-configuration block. A client that successfully invokes the MCP bootstrap is necessarily connected to an enabled MCP endpoint. In both cases, that adapter or configuration fact must remain separate from target and service status.

The status snapshot is informational and cannot authorize access, reserve resources, or guarantee that a later operation will succeed. Each operation still authenticates its adapter request, captures and revalidates its scope, performs its own admission checks, and returns the existing shared error when current conditions prevent it. Browser bootstrap/status and the future MCP bootstrap/status operation must expose the same shared facts even when their surrounding transport metadata differs.

## Transport-neutral Go service error contract

Target, catalog, activity, acquisition, and trace-analysis services below the browser and MCP adapters share one small domain-error contract. A service error has a stable `code`, a safe human-readable `message`, the operation's `targetScopeId` when it is target-specific, and only bounded code-specific details needed to interpret that error. Adapters and callers branch on `code`, never on message text or an internal Go error string. Internal causes remain available for sanitized console diagnostics but are not automatically returned to clients.

The initial shared codes are:

| Code | Meaning |
|---|---|
| `INVALID_ARGUMENT` | A non-cursor service input is malformed or unsupported. |
| `TARGET_AUTHENTICATION_REQUIRED` | No usable in-memory application credential is present, or Phase 1 returned `LOOMSPAN_API_KEY_REJECTED`. A credential is required for any operation needing new application access. Complete previously acquired evidence may remain locally inspectable under the acquisition-time authorization rule. |
| `TARGET_ACCESS_BLOCKED` | A host security layer or upstream proxy returned `401` or `403` without the Loomspan rejection code. The console must not claim that the Loomspan key is invalid. |
| `TARGET_UNAVAILABLE` | The target could not be reached or completed with a usable response because of DNS, connection, TLS, timeout, or upstream-server failure. |
| `INCOMPATIBLE_TARGET` | Authenticated instance status reported a different `consoleCompatibilityVersion`; expected and observed values may be included as code-specific details. |
| `TARGET_CHANGED` | Work or a reference belongs to a `targetScopeId` that is no longer current. The caller must discard prior-scope state; the current scope may be included as a code-specific detail. |
| `INVALID_CURSOR` | A cursor is malformed or does not match its endpoint, query, ordering, filter, artifact handle, or payload range. |
| `STALE_CURSOR` | A previously valid collection traversal can no longer continue. The caller begins that collection or baseline query again. |
| `NOT_FOUND` | A resource is not currently available and no stronger lifecycle fact is known. It must not be presented as proof of expiration. |
| `ARTIFACT_EXPIRED` | A valid current-scope `artifactHandle` no longer names a retained Go copy. The caller may reacquire only if the current application catalog still offers the trace. |
| `INVALID_ARTIFACT` | Downloaded trace bytes violate the Java writer invariants or consumed semantics exercised by the current-release fixtures. Code-specific details may state whether unchanged raw download remains available, but no partial analysis result is valid. |
| `LIVE_MONITORING_UNAVAILABLE` | The application reported that its live projection is unavailable. Skill and finalized-trace operations remain independent. |
| `LIMIT_EXCEEDED` | A configured request, response, artifact, workspace, or concurrency bound prevents the operation. Bounded details may identify the relevant limit and observed value when safe. |
| `LOCAL_STORAGE_UNAVAILABLE` | A local trace acquisition or derived-file operation could not complete because the managed filesystem lacked usable write capacity or returned an equivalent storage failure, but artifact-specific cleanup restored safe workspace operation. The result may identify the affected local operation without exposing filesystem paths. |
| `CONSOLE_ERROR` | An unexpected Go shared-service failure occurred and no more specific shared code applies. The message is sanitized and bounded; internal error text, stack traces, paths, credentials, and diagnostic payloads are not returned. |

Phase 1 problems map into this contract before reaching either adapter: `LOOMSPAN_API_KEY_REJECTED` maps to `TARGET_AUTHENTICATION_REQUIRED`; a generic upstream `401` or `403` without that code maps to `TARGET_ACCESS_BLOCKED`; connection, TLS, timeout, `APPLICATION_ERROR`, and other unusable upstream failures map to `TARGET_UNAVAILABLE`; compatibility mismatch maps to `INCOMPATIBLE_TARGET`; `INVALID_REQUEST` maps to `INVALID_ARGUMENT`; and the Phase 1 cursor, not-found, live-monitoring, and `LIMIT_EXCEEDED` codes retain their corresponding Go meanings. Absence of an in-memory application credential produces the same Go-owned `TARGET_AUTHENTICATION_REQUIRED` code before an upstream request is attempted. Trace validation, scope rotation, handle retention, local continuations, configured Go limits, and recoverable artifact-specific local-storage failures produce their Go-owned codes directly. A workspace-wide safety or I/O failure that remains after artifact-specific cleanup is outside this request-error contract because it terminates the console process.

`CONSOLE_ERROR` is the final Go-owned fallback, not a wrapper for errors that already have a specific code and not a substitute for target, application, browser-authentication, or MCP protocol failures. Go records the internal cause only through sanitized local diagnostics. Browser handlers normally map this shared code to HTTP `500` while preserving `CONSOLE_ERROR` in the response envelope.

The contract deliberately has no universal `retryable`, `requiredAction`, `restartRequired`, `configurationRequired`, or `evidenceAvailable` fields. Those values would be unreliable across causes and would turn a developer tool into a troubleshooting state model. Recovery semantics stay with the specific code. Evidence availability is reported only where known, such as raw availability on `INVALID_ARTIFACT` or current application trace availability alongside an expired handle.

An application `STALE_CURSOR` response during SSE connection is a recoverable upstream continuity result. Go converts it into its local replay-gap behavior rather than presenting it as target failure. A browser that has fallen behind Go's still-continuous local window obtains a fresh active baseline and continues with the retained suffix. An upstream stale cursor or changed `instanceId` resets the shared Go window before baseline recovery, so browser and MCP can receive only the new post-reset interval and must describe it as incomplete activity rather than a complete execution narrative. Likewise, ordinary empty collections and a successful status reporting an unavailable independent feature are not converted into generic errors.

Browser HTTP handlers map shared errors to suitable HTTP status codes and a common local error envelope that preserves `code`, safe `message`, optional `targetScopeId`, and bounded details. HTTP status remains a coarse transport representation; browser behavior is keyed by the shared code. Pairing, browser-session, Host/Origin, and CSRF failures occur before shared target services and remain browser-adapter security errors rather than being mislabeled as target failures. Phase 3 defines the corresponding MCP mapping, including the same domain codes.

## Console-local browser API boundary

The browser-to-Go API is an explicit local adapter over transport-neutral Go services. Browser HTTP handlers must not become the owners of target connection semantics, trace parsing, failure and usage calculations, or other runtime facts that MCP later consumes. Conversely, Go is not required to build a complete browser-ready materialized mirror of Loomspan state. Future MCP SDK types remain confined to the Phase 3 adapter and do not enter these services. Browser/MCP adapter parity fixtures must be able to assert the same identifiers, calculations, availability, direct limitation facts, and shared domain-error meanings from one service result while allowing only protocol-wrapper and presentation differences.

This API is still explicit and tested, but it is not an independently supported cross-release protocol. Its browser caller is the asset set embedded in the same Go executable. Browser API DTO changes therefore require an atomic Go-and-assets build plus browser adapter tests, not a new compatibility number. A stale or unauthenticated page must be sent through reload, bootstrap, or pairing behavior and must never be classified as `INCOMPATIBLE_TARGET`, which is reserved for the Java-to-Go boundary.

The initial local API uses:

- REST for paired bootstrap, target and compatibility status, current snapshots, skills, current-process trace discovery, trace queries, and sensitive local operations; and
- SSE for relayed Loomspan activity and Go-owned connection or target-lifecycle events.

The browser builds its current live view from a best-effort application snapshot obtained through Go plus later activity envelopes. Go may map REST contracts into explicit browser DTOs, but it should pass Phase 1 activity identity and semantics through without asking the browser to reverse-engineer Go internals. The browser never receives the application credential and never parses the NDJSON artifact. Catalogs, links, and ordinary browser DTOs use opaque trace identifiers rather than filesystem paths. Authenticated raw-record inspection and raw artifact download remain explicit exceptions: they may show a filesystem path already present in the canonical trace, and Go does not add redaction or rewriting solely to remove it.

Go-owned events must remain distinguishable from Loomspan execution activity. At minimum, relayed execution envelopes occupy a `loomspan.activity` namespace and local connection, target-change, and replay-gap events occupy a `console.*` namespace. Names such as `console.connection`, `console.target_changed`, and `console.replay_gap` are the starting vocabulary; exact wire spelling may be settled during local API planning, but the namespace separation is mandatory. A local event must not look like a canonical Loomspan trace or activity record. Only `loomspan.activity` envelopes participate in the upstream delivery-cursor and bounded replay contract. `console.*` events report current local lifecycle state and may be refreshed through paired bootstrap or status REST responses rather than inserted into the Loomspan cursor sequence.

Target scoping, identity commitment, credential-context replacement, and stale-result rejection follow the authoritative [`TargetContext` ownership](#go-targetcontext-ownership) rules. Browser handlers consume scope snapshots and target-scoped services; they do not implement their own target commit or rotation logic.

The browser treats any `targetScopeId` change or mismatch as a whole-application reset boundary. It stops its local event stream, discards all in-memory application-derived and presentation state, and navigates with replacement semantics to the console root so paired bootstrap loads a clean application rather than reopening a stale trace, frame, record, filter, or selection route. The paired `HttpOnly` session cookie may remain valid because it is console-owned; no application-derived browser store survives the reset. This deliberately favors a simple reliable reload over surgical client-state reconciliation. Each open tab performs the same reset independently.

Go retains one small bounded ring of recently received Phase 1 activity envelopes for the current `targetScopeId`, in addition to bounded pending delivery per tab. The ring supports browser reconnect and transport-neutral bounded recent-activity queries; it is not authoritative or durable execution history. The ring contains events from exactly one continuous upstream interval. An upstream `STALE_CURSOR`, changed `instanceId`, target-scope rotation, or console shutdown clears it before new activity is admitted. Go retains only small window-level reset facts needed to report why and when the current interval began and the first accepted upstream cursor; it does not retain multiple continuity segments.

Queries may filter the current window by execution identity and cursor range and return only complete envelopes in application delivery-cursor order. Every successful result identifies the observed cursor range and time, states when the requested beginning is no longer present, and reports any known reset boundary preceding the current interval, so a caller cannot mistake the returned suffix for complete execution activity. A new or reconnecting tab obtains a fresh active baseline and may request events after its last cursor. If the requested range remains in the same continuous ring interval, Go replays it. If only that tab's requested beginning has left the otherwise continuous local ring, the successful reconnect result reports a local replay gap and the tab proceeds from a fresh baseline without forcing other consumers to reset. A replay gap is not a shared service error. Multiple tabs maintain independent UI state and cursors. One slow tab may be disconnected and refreshed without delaying the upstream connection or another tab.

After pairing, the local `HttpOnly`, `SameSite=Strict` session cookie authenticates browser requests. Expected loopback `Host` and same-origin `Origin` validation remain mandatory. Sensitive or state-changing browser operations additionally require a session-bound CSRF token returned through the paired bootstrap response, retained only in browser memory, and sent in a custom request header. This applies at least to submitting or replacing the selected target and application observability key; enabling or disabling MCP; revealing or regenerating its key; and any future local configuration mutation. Secrets and CSRF tokens must not appear in URLs, logs, or persistent browser storage. Every authenticated browser response carrying application diagnostic data, including trace attachments and diagnostic errors, uses `Cache-Control: no-store`; credential-management responses do as well. This prevents ordinary HTTP caching but does not control what the paired browser, extensions, or developer tools retain after receipt. Pairing, Host/Origin validation, the session cookie, and CSRF validation are complementary controls.

Browser and future MCP routes share the loopback listener but belong to distinct, fail-closed request-validation realms. Routing must select the realm before authentication rather than applying a listener-wide “browser session or API key” policy. Browser routes accept only the paired browser session and apply their browser-specific `Host`, `Origin`, and CSRF rules; they must not accept the MCP access key or upstream application key as substitutes. MCP routes accept only the MCP access key through its documented authorization header and apply the MCP-specific `Host` and `Origin` rules defined by Phase 3; they must not accept the browser cookie, pairing secret, CSRF token, or upstream application key as substitutes. Accommodating legitimate non-browser MCP clients that omit `Origin` must never weaken MCP `Host` validation, MCP access-key authentication, or browser-route validation. The realms may share low-level credential-comparison, request-bound, and logging utilities without sharing acceptance policy.

## Live activity relay and recent-activity queries

The Go host subscribes to the Phase 1 SSE activity stream and relays it to its browser client.

The relay must preserve:

- activity identity and ordering;
- cursor semantics;
- replay-gap signals;
- connection and reconnection state; and
- end-of-execution activity, including the exceptional `EXECUTION_OBSERVATION_ENDED` lifecycle activity when core finalization prevents a trustworthy canonical completion projection.

The same bounded current-scope, single-continuity window is exposed below adapters through a transport-neutral recent-activity query service. Browser handlers may use it for reconnect and current-window inspection. Phase 3 MCP may use it for bounded on-demand snapshots of recent activity for one execution. This shared service owns the reset-boundary facts consumed by both adapters; neither adapter derives different continuity semantics. It does not create another upstream subscription, extend retention, reconstruct missing events, retain multiple continuity segments, or promise that the beginning of an execution remains present.

The browser should recover from an expired cursor by obtaining a fresh paginated active-execution baseline and resuming from its first page's `resumeCursor`, rather than pretending the missing activity was delivered. After an upstream replay gap, Go emits or returns a `console.replay_gap` notice carrying the bounded reset facts; the browser replaces its current active-execution state from the baseline and explains that current state was refreshed while activity before the new interval may be missing.

The application provides one instance-local stream cursor scoped by its startup-generated `instanceId`. The first active-execution page captures a registry high-water mark and includes that `instanceId`, a `resumeCursor` observed near baseline collection, and an `observedAt` time. Go begins or resumes the upstream stream with `afterCursor` and `instanceId` while traversing remaining high-water-bound pages. New executions arrive through SSE rather than shifting later pages; executions that complete during traversal may disappear before their page is read, with their post-cursor activity supplying the transition when replay remains available. This is a best-effort baseline, not an atomic cut through registry state and the stream. An expired cursor or changed `instanceId` returns application `STALE_CURSOR` and triggers a fresh paginated baseline. The initial Java-to-Go stream does not support `Last-Event-ID`.

The Phase 1 stream is a best-effort, eventually consistent developer-monitoring projection rather than a durable or exactly-once feed. The Go host tracks the last successfully received upstream cursor, applies activity in cursor order, and tolerates duplicates. An ordinary disconnect may use bounded upstream replay when available. If the application returns `STALE_CURSOR` or `instanceId` changed, Go first clears the recent-activity window, records the new continuity boundary, obtains a fresh paginated baseline, resumes after its first page's new `resumeCursor`, and admits only later activity into the cleared window. It may report that current execution state has been refreshed while some intermediate live activity may be missing. It must not imply that baseline recovery recreated the missing narrative or combine pre-reset and post-reset events in one result.

Go also refreshes the active baseline at a low frequency while connected so ordinary pagination and stream races heal without production-streaming coordination. Exact refresh timing remains an implementation-planning choice; an initial interval on the order of tens of seconds is sufficient. A live execution may appear slightly late or remain visible briefly after completion, and that transient behavior is acceptable.

The application closes a slow subscriber when its bounded pending-delivery allowance or write deadline is exceeded. Go treats that closure like any other reconnect and uses replay or paginated-baseline recovery. Likewise, bounded fan-out to browser tabs must not let one slow tab delay the upstream connection or another tab; a lagging tab may be disconnected and refreshed from current Go state. Neither Go nor the browser attempts to reconstruct application projector state, and a finalized trace available from the current process catalog or an already acquired Go copy remains the detailed debugging fallback.

Phase 1 may reject a new upstream SSE subscription with `LIMIT_EXCEEDED` when its fixed process-wide subscription capacity is occupied. Go preserves that code, keeps its current target scope, and retries with bounded backoff rather than treating the response as target unavailability, authentication failure, or a replay gap. Because Go owns only one upstream subscription, this normally indicates other independent consoles or an incompletely closed prior connection rather than browser-tab pressure.

If the application reports `liveMonitoringAvailable: false`, Go must stop presenting the active baseline or activity as trustworthy current state. A request for either produces `LIVE_MONITORING_UNAVAILABLE`; reconnect, cursor replay, and periodic baseline refresh are not presented as recovery. Skill-catalog, current-process trace-catalog, acquisition, and analysis operations may continue because they do not depend on the application's live in-memory projection. Application restart may be required to make live monitoring available again; Go must not attempt to rebuild application projection state from the activity window or trace files.

The Go target connection manager operates the one upstream SSE connection owned by `TargetContext`, independent of browser-tab presence. It appends received envelopes to the bounded current-scope activity window and performs bounded fan-out to paired browser tabs. It reports handshake identity to `TargetContext` and never commits or rotates identity itself. Browser tabs do not own upstream subscriptions. MCP remains snapshot-oriented: it may query the shared recent-activity window when called but does not subscribe to the upstream stream or receive continuous event injection.

### Collection pagination through Go

Go consumes the Phase 1 keyset-pagination contract for trace, active-execution, and skill-summary collections. It should normally request large pages and must not turn the application defaults into many tiny upstream requests: the starting application and local-browser collection defaults are `1,000` items, the maximum requested size is `5,000`, and one uncompressed JSON response is limited to `16 MiB`. Go preserves `hasMore`, current-scope identity, observation time, and explicit stale-cursor behavior. Its browser DTOs may adapt cursor representation but must not convert keyset traversal into offset pagination or imply a transactional collection snapshot.

The browser network page size is independent of visible row count. It may receive thousands of concise summaries while rendering only a virtualized visible window. Skill YAML content, trace files, execution detail, records, and large payloads remain separate detail, streaming, pagination, or range operations. A target-scope reset invalidates every local continuation and reloads the application as already specified.

Finalized trace artifacts currently available from the selected application's current-instance catalog should be streamed into the Go host's bounded `loomspan-console-work/transient` workspace rather than buffered entirely in memory. Go shared services, not the application server or browser, own frame, record, search, pagination, timeline, failure, and usage analysis.

Go consumes the raw file incrementally according to the current Java writer, enums, documented global invariants, and Java-produced fixtures paired with `consoleCompatibilityVersion`. It reconstructs chunked logical payloads while preserving raw payload envelope and `PAYLOAD_CHUNK_APPENDED` records for framework inspection. It enforces the configured aggregate cache policy plus finite line-size, nesting, and structural-processing bounds and rejects malformed JSON, unknown record or consumed semantic enum values, inconsistent trace/session identities, invalid consumed sequence or frame relationships, a missing or non-final canonical `TRACE_COMPLETED`, and missing, duplicate, or mismatched chunks with `INVALID_ARTIFACT`. Metadata and data that Go does not interpret remain opaque diagnostic JSON and do not require a Go model. Parser failure affects that artifact or view rather than terminating the console process; code-specific details may report that raw attachment download remains available.

### Centralized artifact service and user-controlled retention

One adapter-neutral Go artifact service is the sole path for acquiring and analyzing a finalized trace for interactive use. Browser handlers and MCP handlers call this service; they do not download into adapter-specific caches, create their own indexes, issue independent analysis handles, reserve separate workspace pools, or implement retention and eviction. Explicit raw attachment streaming to a developer-selected browser download remains a separate pass-through operation and does not create an analysis copy or handle unless the caller separately requests console analysis.

Within one `targetScopeId`, the service maintains at most one installed immutable analysis copy for a given scope-bound application trace reference. If that trace is already installed, either adapter receives the existing `artifactHandle`. If acquisition of it is already in progress, concurrent browser and MCP requests join the same acquisition and receive the same installed handle or the same acquisition failure rather than downloading, validating, indexing, and charging capacity twice. Cancellation of one waiting adapter request stops that caller's wait but does not cancel work still needed by another admitted waiter. The implementation may choose its synchronization mechanism; adapters must not be able to bypass it.

Workspace admission, partial-file cleanup, validation, handle issuance, query pinning, last-used refresh, TTL cleanup, capacity cleanup, manual removal, and byte accounting all occur once in this service. The configured aggregate byte ceiling and idle TTL apply globally across browser and MCP use, including their explicit `unlimited` and `never` values. There is no trace-count or per-trace byte limit. A query from either adapter pins the same installed copy for that query and refreshes the same last-used time on success. Cleanup or invalidation expires the shared handle consistently for both adapters; there are no per-adapter leases, reference counts exposed to clients, or reserved browser-versus-MCP capacity partitions.

Go installs a trace into its analysis workspace only after the complete artifact has been downloaded, its operation still belongs to the current `targetScopeId`, and the current trace record, identity, ordering, and chunk contracts have been validated. A target transport interruption produces `TARGET_UNAVAILABLE`, scope rotation produces `TARGET_CHANGED`, and invalid content produces `INVALID_ARTIFACT`; none receives an analysis handle or is presented as a partial valid trace. Under a finite `max-bytes`, aggregate workspace `LIMIT_EXCEEDED` occurs only after expired-first and then least-recently-used unpinned cleanup cannot make the complete artifact and required derived files fit. Caller cancellation ends that request through the adapter's ordinary cancellation path rather than inventing another domain code. Raw attachment download may remain a separate option when the application artifact is still available.

A successfully installed immutable copy receives an opaque `artifactHandle` bound to its `targetScopeId`. Go retains the copy according to the configured aggregate cache ceiling and idle TTL. A successful query or continuation refreshes its last-used time. The defaults are `4GiB` and `4h`; explicit `unlimited` and `never` values disable aggregate-capacity and time-based cleanup respectively. Numeric zero is invalid. Browser and MCP callers do not acquire or manage a separate retention object, while the paired Trace Storage page may deliberately remove unused local copies through the centralized service.

Once installed, that immutable copy has completed its upstream authorization boundary. A later `LOOMSPAN_API_KEY_REJECTED` does not invalidate its handle or require Go to reauthenticate before each local query. Paired-browser or MCP authentication still applies to every local request, and the copy remains subject to its ordinary handle, target scope, configured cache policy, manual removal, and console-process lifetime. Reacquisition after handle expiration is a new upstream operation and therefore requires valid application authentication as well as current catalog availability.

Go must not delete an acquired copy during an in-flight query. Under a finite idle TTL, an expired unpinned copy is removed and its handle expires. Under a finite aggregate ceiling, admission first removes expired unpinned copies and then least-recently-successfully-used unpinned copies until the new work fits; only inability to reclaim enough configured capacity produces workspace `LIMIT_EXCEEDED`. Manual browser cleanup likewise removes only unused copies. `targetScopeId` rotation and console shutdown invalidate all affected handles immediately, cancel in-flight analysis, and clear their files and indexes regardless of configured TTL. Application-side expiration does not affect a fully acquired Go copy, but it can prevent an expired or removed handle from being reacquired.

Every record-query, search, and payload continuation is opaque and bound to the target scope, artifact handle, query or filter fingerprint, and next canonical record or byte position. A malformed token or reuse after a query or filter change produces `INVALID_CURSOR`; scope rotation produces `TARGET_CHANGED`; and TTL cleanup, capacity cleanup, or manual handle removal produces `ARTIFACT_EXPIRED`. Go never silently restarts or applies a continuation to another query, artifact, or target. Tokens need no cryptographic signature because the loopback API is already authenticated and Go may manage the bounded state server-side.

Record pagination alone is insufficient for a reconstructed payload larger than one response. Such a record returns a payload descriptor containing its logical size, content type, inline status, opaque payload reference, and supported range information. Callers retrieve the payload through bounded ranges tied to the same artifact handle. Text delivery must preserve valid UTF-8 boundaries; otherwise the range contract returns encoded bytes rather than malformed partial text.

Search operates over the immutable acquired copy and orders results deterministically by canonical record sequence. Reaching a result, byte, or time bound returns the fully completed results so far plus a continuation from the last fully examined record when progress can continue. Search does not require a database snapshot or durable index version because the artifact handle identifies immutable bytes. Its continuation expires when the handle does.

Go shared trace services provide **complete inspectability while the artifact handle remains valid**, not guaranteed completion of an arbitrarily long traversal. No successfully parsed record or payload is intentionally excluded according to guessed relevance or sensitivity, and bounded pagination plus payload ranges make every item addressable. Completion remains contingent on the handle, target scope, console process, configured cache policy, manual removal, and local filesystem remaining valid long enough. Per-response bounds protect console stability and client interoperability; they are not redaction or authorization rules.

The production console does not depend on filename parsing or shared filesystem paths used by the deprecated proof-of-concept CLI. It uses opaque catalog identifiers and ordinary HTTP response metadata. No manifest parser, archive reader, digest verifier, application-level decompressor, or second logical-trace wire representation is required initially.

## Multiple consoles observing one application

The expected initial topology is one developer console observing one Loomspan application. Phase 1 should nevertheless allow multiple independent Go console processes to observe the same application.

Each console:

- obtains its own snapshot;
- owns its own SSE subscription and cursor;
- reconnects independently;
- shares no selection or UI state with other consoles; and
- cannot delay or disrupt Loomspan or another console.

Independent consoles may observe the same application, but each requires a separate config profile, MCP credential, and managed work directory. One profile and its work directory are exclusively locked by one Go console process. A later process selecting either the active profile or active work directory fails clearly and must use a different profile and work directory. This local ownership rule does not reduce Phase 1's ability to serve multiple independent observers, and it does not limit the owning console's browser sessions or MCP clients.

Phase 1's finite fixed process-wide admission limits still apply across those independent observers. If either the open observability-SSE or concurrent trace-download capacity is occupied, a new operation receives the shared `LIMIT_EXCEEDED` result and does not displace an admitted operation.

Phase 2 does not include console discovery, leader election, user presence, shared selections, cross-console synchronization, or per-console durable delivery.

## Information architecture and navigation

The initial browser application has four primary evidence areas:

1. **Overview**
2. **Live Executions**
3. **Traces**
4. **Skills**

These destinations follow the authoritative evidence categories rather than developer interpretations. Slow executions, failed executions, unexpected usage, and unfamiliar skill paths are approved workflows or entry states over Live Executions and Traces; they are not separate top-level destinations, stores, or analysis engines. The initial console does not add global Failures, Diagnostics, Recommendations, or similar interpretation-oriented areas.

### Stable Overview landing view

Overview is the stable landing destination. The console does not redirect automatically because a target connects, an execution starts or ends, or an active-execution count changes.

Overview presents the selected target address or label, independent connection, authentication, Java/Go compatibility, runtime-identity, and live-monitoring facts, the established application `instanceId`, registered-skill count, active-execution count, current-process cataloged-trace count, and applicable trace persistence and availability settings. It does not combine those facts into an aggregate health, readiness, or severity value.

The page retains its identity while its facts and available actions change:

- with no selected target, it presents the target-connection entry state;
- when application authentication is required, it presents that fact and the credential-entry action;
- when the target is unavailable, access-blocked, or incompatible, it presents the applicable independent facts; and
- when the target is authenticated and compatible, it presents the instance facts and navigation into available live, trace, and skill evidence.

### Global target and live context

The global application chrome presents a compact current-target context sufficient to prevent confusion about which runtime is being inspected. It includes the selected target address or label and the independent connection, authentication, compatibility, and runtime-identity facts appropriate to the available space. Live-monitoring availability remains a separate fact where relevant. An established `instanceId` remains available without requiring the developer to infer it from another status.

The global chrome also presents the current active-execution count and a direct path to Live Executions. It does not keep the full active-execution list, activity narrative, or continuously changing execution details visible while the developer inspects another area. The one upstream SSE connection and bounded recent-activity window continue independently of the current browser route, so returning to Live Executions shows the latest available current-scope state.

A changed active count may use restrained visual emphasis but does not create a toast, modal, automatic navigation, or global failure notification. A future notification system requires a separately demonstrated workflow.

Changing the selected target is an intentional operation through the target-connection experience. The resulting `targetScopeId` rotation clears prior application-derived state under the settled lifecycle and returns browser navigation to Overview. It never attempts to reinterpret an old execution, trace, frame, record, or skill reference in the new scope.

### Inspection context

Within a selected execution or trace, a persistent contextual area beneath the global navigation keeps the applicable identity and lifecycle facts visible while the developer navigates related evidence. Depending on availability, those facts include:

- entry skill;
- active, completed, or observation-ended fact;
- session and trace identifiers;
- elapsed or completed time;
- application and local artifact availability;
- observation freshness for live evidence; and
- the current-scope execution or trace breadcrumb.

The contextual area displays identifiers where they identify the evidence and support existing current-scope navigation. It does not add copying, exporting, evidence packaging, or retention behavior.

Failure-, usage-, hierarchy-, timeline-, record-, attempt-, validation-, and registered-skill facts remain coordinated views or drill-downs from the same execution context. Navigating among them preserves the current target scope, artifact handle, trace, and applicable frame selection. The exact trace explorer layout remains a separate open decision.

### Persistent and drill-down information

Persistent global space is limited to selected-target context, independent target/runtime status facts, primary navigation, active-execution count, and current execution or trace identity when inside an inspection context.

Area-level persistent space contains the current bounded live summary while viewing a live execution or the selected trace and frame identity while viewing finalized-trace detail.

Activity narrative, hierarchy, timeline, failures, validation sequences, usage attribution, individual records, raw payloads, framework metadata, and registered skill YAML are area content or explicit drill-down detail. Raw payloads and framework metadata do not occupy global or default persistent space.

### Workflow placement

The approved workflows map into this information architecture as follows:

| Workflow | Primary entry | Continued inspection |
|---|---|---|
| Currently slow execution | Live Executions | Current summary, active path, and recent narrative. |
| Failed completed execution | Live Executions or Traces | Failure-focused state over the shared trace explorer when an artifact is available. |
| Unexpected usage or cost | Traces | Usage facts over the shared trace explorer; the console does not calculate unrecorded monetary cost. |
| Unfamiliar nested skill path | Live Executions or Traces | Active path or finalized hierarchy plus application-provided registered skill YAML. |

Skills remains independently useful for discovering the definitions registered by the application. Runtime skill-path inspection begins from execution evidence; the skill catalog does not become or imply execution history.

## Live execution presentation

The live experience presents every activity kind deliberately emitted by the Phase 1 initial visible set while keeping current execution facts separate from the chronological activity narrative. It does not apply an importance filter, create a health classification, or imply that the bounded live projection is a complete trace.

### Live collections and selection

Live Executions contains two visibly separate collections:

- **Active executions:** bounded current snapshots from the application registry.
- **Recent completions:** terminal activity still present in Go's bounded current continuity interval.

Recent completions are explicitly temporary live context rather than durable execution history. They may disappear as the bounded interval advances or resets.

The initial list behavior is:

- opening Live Executions does not select an execution automatically;
- a direct current-scope link selects its referenced execution;
- the default active order remains the Phase 1 newest-started-first order;
- a new execution enters at the top;
- changes to phase, usage, elapsed time, or latest activity do not reorder rows automatically;
- developer-selected sorting or filtering is browser presentation state;
- recent completions appear newest first; and
- a selected execution remains selected when it terminates.

### Selected live execution

A selected execution has three coordinated logical information areas. Their responsive screen arrangement remains a frontend-design choice.

1. **Current summary**
2. **Active skill path**
3. **Recent narrative**

The current summary remains visible while the developer inspects live activity. It shows the entry skill, execution status when established, current phase, current active skill path, start time and elapsed time, model/tool/skill invocation counts, current usage and configured limits, latest concise activity, session and trace identifiers, snapshot observation time, continuity state, and connection freshness.

The summary is replaced from refreshed authoritative snapshots and updated from ordered activity. It is not reconstructed solely from envelopes that remain in the browser, and it does not add slow, stuck, healthy, degraded, expensive, or likely-failing classifications.

The active-path area shows only the bounded currently active nested skill path supplied by the snapshot. Skill-frame opening and closing update it, and every invocation remains distinguishable by frame identity. The presentation may use breadcrumbs or indentation, but it does not add inactive siblings, completed branches, planning/model/tool/validation nodes, or any other structure that would make it appear to be a complete execution tree. Complete hierarchy remains finalized-trace functionality.

### Activity narrative

The recent narrative presents every envelope in the Phase 1 initial visible activity set in application delivery-cursor order. One envelope remains one individually accessible narrative item. Adjacent items may be grouped visually, but grouping does not merge, discard, reorder, or rewrite them.

| Activity kind | Live presentation |
|---|---|
| `TRACE_STARTED` | Execution-start row and current-summary update. |
| Skill `FRAME_OPENED` | Narrative row and active-path transition. |
| Skill `FRAME_CLOSED` | Narrative row and active-path transition. |
| `MODEL_REQUEST_SENT` | Model-interaction row without prompt content. |
| `MODEL_RESPONSE_RECEIVED` | Receipt row with supported concise facts and no response content. |
| `PLAN_CREATED` | Plan-created row with its bounded summary; full detail requires a finalized trace. |
| `PLAN_UPDATED` | Distinct row preserving that an update occurred. |
| `PLAN_VALIDATION_FAILED` | Validation-failure row without a console-generated explanation. |
| `PLAN_RETRY_REQUESTED` | Retry-request row without judging whether retry was appropriate. |
| `TOOL_CALL_STARTED` | Tool-start row identifying the tool when recorded, without tool input. |
| `TOOL_CALL_COMPLETED` | Tool-completion row with supported concise outcome facts and no tool output. |
| `TOOL_CALL_FAILED` | Tool-failure row that does not automatically become the execution-ending failure. |
| `STEP_STARTED` | Step-start row identifying the recorded step. |
| `STEP_ACTION_REJECTED` | Rejection row showing the recorded classification. |
| `STEP_COMPLETED` | Step-completion row. |
| `ERROR_RECORDED` | Recorded-error row that does not declare the execution terminal. |
| `TRACE_COMPLETED` | Terminal row carrying execution outcome and separate application-trace-availability facts. |
| `EXECUTION_OBSERVATION_ENDED` | Distinct noncanonical terminal row stating incomplete observation and `CORE_FINALIZATION_FAILED`. |

Each row shows only applicable concise facts: timestamp, activity label, bounded summary, recorded skill/frame/route/tool/phase identity, supported status or outcome, and recorded retry, validation, or failure classification. Optional bounded event details remain collapsed until the developer selects the row. Documented semantic details may receive labeled presentation; other display-only details remain untrusted text and cannot drive browser state or calculations.

Live rows do not automatically include prompts, model responses, tool inputs or outputs, full plans, raw records, reconstructed payloads, arbitrary metadata, or records outside the Phase 1 live set. Those remain explicit finalized-trace detail. The browser does not promote additional trace records independently.

The narrative orders retained activity chronologically, oldest first and newest last. A row may show or indent its recorded frame context when that relationship is available, but the browser does not infer a missing parent, construct hierarchy from adjacency, or hide an item whose parent is outside the retained interval. The view states when its earliest retained item is not the beginning of the execution.

### Following, updates, and motion

Live following is enabled initially. Scrolling backward or selecting an earlier row pauses automatic following and presents a **Resume live** action. Incoming events continue updating the current summary while following is paused, but they do not change the selected row or scroll position. Navigating elsewhere does not stop Go's upstream subscription; returning obtains the latest snapshot and available continuous interval.

Semantic activity appears promptly and in order. The browser may batch a short event burst into one render pass, but it does not semantically coalesce or drop envelopes. Quietly changing facts such as elapsed time, counts, and usage update without animated transitions. Elapsed time may use a steady display cadence while visible and remains a browser calculation from the recorded start time rather than new runtime evidence; the exact cadence is an implementation-planning value.

A newly received activity row may receive a brief restrained highlight. The initial presentation does not use pulsing execution rows, animated tree movement, bouncing counters, success animation, automatic expansion, per-event toast notifications, or sound. Reduced-motion preferences suppress nonessential motion. Exact durations, colors, iconography, and component transitions remain frontend and accessibility implementation choices.

### Connection and continuity presentation

Go-owned connection and continuity facts remain visually and semantically separate from Loomspan activity:

- a temporary disconnect is an inline connection-status notice;
- a tab-local replay gap is a local-gap notice;
- an upstream continuity reset inserts a clear divider before the new interval;
- refreshed state shows its new observation time;
- retained stale state is not presented as current; and
- `LIVE_MONITORING_UNAVAILABLE` replaces the live evidence with an explicit unavailable state rather than a reconnect animation.

These notices never masquerade as Loomspan execution records. An upstream reset clears the shared recent-activity interval before new activity is admitted and never presents events from opposite sides as continuous.

### Terminal transition

Selecting a recent terminal item opens its terminal summary in place. `TRACE_COMPLETED` shows execution outcome and application trace availability as separate facts. `EXECUTION_OBSERVATION_ENDED` shows incomplete observation without inventing an outcome. **Inspect trace** appears only when a finalized application trace is available for acquisition. Completion never navigates automatically, and deliberate acquisition follows the centralized artifact-service lifecycle.

## Trace explorer organization

The trace explorer is hierarchy-first. It uses one acquired immutable artifact, one shared selection context, and the same transport-neutral Go calculations across four coordinated views:

1. **Hierarchy** — the default view;
2. **Timeline**;
3. **Usage**; and
4. **Records**.

These are browser views over the same artifact handle and analysis services, not separate parsers, caches, evidence models, or retention lifecycles. The initial product does not require the hierarchy and timeline to remain visible in a permanent synchronized split. Switching views preserves the closest applicable frame or record selection, and returning to Hierarchy restores its browser-owned selection and expansion state.

### Hierarchy-first default

Opening a trace from the catalog normally selects the execution root in Hierarchy. Each hierarchy item presents supported recorded or mechanically calculated facts such as frame or skill name, frame type, route, recorded outcome, timestamps, inclusive and self duration, direct and descendant usage, and recorded failure or validation indicators.

Shared Go services calculate frame `inclusiveDuration` from matching canonical `FRAME_OPENED` and `FRAME_CLOSED` timestamps and `selfDuration` by subtracting immediate-child inclusive durations. They also build the settled flat failure and validation reference indexes during parsing. Direct, descendant, inclusive, attempt, retry, and unattributed usage use the settled workflow definitions and Phase 1's explicit trace relationships; final outcome and terminal-failure attribution likewise consume the Phase 1 contract. The hierarchy displays those common Go results and never calculates competing browser-specific values.

The hierarchy distinguishes runtime invocation from registered skill definition, direct child from descendant, repeated invocations of the same skill, canonical order from parent-child structure, and recorded failure from inferred root cause. Every frame in a valid acquired trace remains available. The initial product imposes no hierarchy-specific depth limit, node limit, truncation rule, or partial-tree mode. Specialized very-deep-tree rendering or hierarchy virtualization is deferred until demonstrated necessary.

### Workflow-specific entry states

The navigation context determines the initial view and selection without changing the underlying trace analysis:

| Entry | Initial explorer state |
|---|---|
| Trace catalog | Hierarchy with the execution root selected. |
| Failed execution | Hierarchy with the execution-ending failure's frame or record selected when mechanically established. |
| Usage workflow | Usage with the applicable execution, frame, or attempt selected. |
| Nested skill path | Hierarchy with the referenced frame selected and its recorded path expanded. |
| Timeline link | Timeline with the referenced frame or record selected. |
| Record deep link | Records with the referenced canonical sequence selected. |
| Stale or invalid reference | Existing explicit `TARGET_CHANGED`, `ARTIFACT_EXPIRED`, `INVALID_CURSOR`, or other applicable domain behavior. |

A failure-focused entry does not label the selected item a root cause. It presents the mechanically established execution-ending failure and related recorded facts.

### Shared selected-item detail

Every view coordinates with one selected-item detail area. Depending on the selected execution, frame, interval, attempt, or record, it presents supported facts including stable identifiers, parent and child relationships, route and frame type, timestamps and duration, direct/descendant/inclusive usage, attempts and retries, validation outcomes, recorded errors, plan creation and update facts, model and tool transitions, evidence/quota/timeout/guardrail outcomes, related canonical record sequences, direct gap or lifecycle limitations, and registered skill YAML when a recorded skill name maps to a currently available registered definition.

The detail area presents facts and explicit relationships. It does not generate a diagnosis, importance ranking, causal narrative, root-cause claim, or recommended action.

### Timeline

Timeline presents supported recorded temporal relationships: execution and frame intervals, nested skill intervals, model and tool activity, attempts and retries, validation activity, errors, and terminal outcome. Selecting an interval updates the shared frame selection; selecting a point activity updates the applicable record selection.

The timeline does not infer time for missing events, equate wall-clock duration with model or tool usage, or label the longest interval problematic.

### Usage

Usage presents the common Go attribution facts: recorded totals and units, configured limits, direct/descendant/inclusive frame usage, attempt and retry usage, unattributed usage, and model interactions sorted by attributed usage when requested. Selecting a usage row updates the applicable frame or attempt context, and returning to Hierarchy reveals the same frame.

Sorting is arithmetic presentation. It does not label the first result important, excessive, causal, wasteful, or actionable. Monetary cost remains unavailable unless recorded canonically under separately defined semantics.

### Records

Records presents canonical records in sequence order. It supports filtering by supported record type and frame, navigation from hierarchy/timeline/failure/usage/attempt/validation facts, deterministic search results in canonical record order, bounded record pagination, and explicit payload access. It never reorders records into a guessed narrative.

Records not consumed by Go semantic calculations remain available as opaque diagnostic JSON under the current-release trace agreement. Their presence does not cause the browser to invent a typed interpretation.

`TOOL_CALL_STARTED` has one explicit **Tool input** action in finalized traces.
The action retrieves the complete record only when selected and presents the
capability, planned or unplanned linkage, recorded task ID and mechanically
available title, event ID, optional note, and inertly rendered arguments. The
start does not establish an outcome; completion and failure remain separate
terminal tool records, and missing terminal evidence remains unknown.

### Progressive detail

Trace information uses three disclosure layers:

1. **Concise facts:** hierarchy rows, timeline items, usage rows, recorded outcomes, and short summaries.
2. **Structured detail:** selected frame or record metadata, relationships, common calculated facts, validation outcomes, attempts, and indexes.
3. **Raw detail:** raw record JSON, reconstructed payloads, and unchanged raw artifact download.

Raw detail always requires an explicit developer action. Opening a failure, usage, timeline, hierarchy, or selected-item detail does not automatically retrieve or display prompts, model responses, tool inputs or outputs, or large payloads. All diagnostic content renders as text rather than HTML or Markdown.

### Large artifacts and complete inspectability

The hierarchy remains complete without a product-level depth or node cap. Existing operational bounds continue to protect delivery and the Go process:

- records use bounded pagination;
- search uses deterministic continuation;
- large reconstructed payloads use explicit ranges;
- text ranges preserve valid UTF-8 boundaries;
- other byte ranges use the defined encoded representation;
- configured cache, local-storage, and response constraints produce their explicit errors; and
- handle expiration stops later traversal with `ARTIFACT_EXPIRED`.

These mechanisms do not omit hierarchy frames, create a guessed relevance filter, or authorize a best-effort partial semantic tree. Specialized hierarchy virtualization is not an initial requirement and may be designed later if representative traces demonstrate a rendering problem without changing the complete-hierarchy contract.

### Failure and validation navigation

The shared Go failure and validation indexes are flat record-reference lists built during parsing. Failure references cover `ERROR_RECORDED`, `TOOL_CALL_FAILED`, and failed or aborted `FRAME_CLOSED` records; validation references cover the settled planning, evidence-validation, linter, structured-output, and step-action record types. Each reference retains its canonical sequence, timestamp, record type, frame, and only the applicable recorded route, failure, attempt, retry, status, or severity facts. The explorer may navigate these references and marks a failure terminal only through the recorded `terminalFailureId`. It does not group, deduplicate, rank, normalize, or collapse them into a root cause.

### Browser-owned explorer state

Browser presentation state may include current view, selected frame or record, expanded hierarchy nodes, filters, search query and continuation, detail expansion, scroll position, and local pane sizing if panes are used. This state does not extend artifact retention, survive `targetScopeId` rotation as valid evidence, or become a durable cross-process workspace.

## Target connection and browser pairing experience

The initial console uses a paired-browser-first target workflow with a protected terminal prompt as an explicit alternative for application-key entry. Non-secret target defaults may live in the static profile YAML; browser-entered target changes remain process-local.

### Startup pairing

After profile and workspace validation, the console starts its loopback listener, generates a cryptographically strong short-lived one-time pairing secret, attempts to open the default browser for an interactive desktop launch, and always prints the pairing URL as a fallback. A non-secret `--no-open-browser` option suppresses automatic opening. Failure to open a browser does not fail the console.

The pairing URL carries the one-time secret in its fragment, conceptually:

```text
http://127.0.0.1:<port>/#/pair/<one-time-secret>
```

The fragment is not sent in the initial HTTP request and therefore does not enter ordinary server access logs. The browser application reads it in memory, immediately removes it from the visible URL and current browser-history entry with `history.replaceState`, submits it once in the body of a same-origin pairing request, and clears it from application state after the exchange.

Successful exchange invalidates the secret and establishes the existing `HttpOnly`, `SameSite=Strict` browser session. The paired bootstrap response also returns the session-bound CSRF token retained only in browser memory. The one-time secret also becomes invalid on expiry or console shutdown and never becomes a reusable browser credential. Exact entropy and lifetime are implementation-planning constants, but generation must be cryptographically secure and lifetime short.

Tabs in the same browser profile share the paired session cookie and do not pair independently. A paired, CSRF-authenticated browser may deliberately create another short-lived single-use pairing link for a different browser profile. This operation does not reveal or replace an existing browser session, expose the application key, rotate `targetScopeId`, or interrupt another tab, and its response uses `Cache-Control: no-store`. Exact browser-session count, lifetime, refresh, expiry, and recovery behavior remains the separate Browser Session Behavior decision.

### Target defaults and persistence

The versioned static YAML may contain a non-secret default target's HTTP or HTTPS scheme, host and port, externally visible servlet or reverse-proxy context path, timeouts, custom CA configuration, and other settled connection-authority settings. Here, scheme means the URL transport prefix `http` or `https`. The application observability key never appears in YAML.

A target entered or changed through the browser is process-local and is not written back to static YAML. On restart, the YAML default is selected again when configured, a browser-only target selection is forgotten, and the application key is always absent. This preserves the restart-only configuration model and prevents the browser from becoming a configuration-file editor.

After pairing, Overview presents the applicable target entry state:

- **No target selected:** target-address form and connection guidance.
- **Default target selected with authentication required:** application-key entry and protected-terminal alternative.
- **Target established in this process:** independent target facts and navigation into available evidence.

The target address identifies the application base, including any externally visible context path; Go appends the fixed observability namespace. Before selection or scope rotation, Go rejects unsupported schemes, embedded user information, malformed or ambiguous authority, unsupported path form, fragments, and other invalid syntax. The initial target client does not discover a target by following redirects.

### Browser application-key entry

The paired form keeps the target address and application key as distinct inputs. Submission is same-origin and CSRF-protected. Go accepts the target into `TargetContext`, places the key behind its process-memory credential provider, and performs the authenticated instance-status probe. The browser clears the key input and submission state when the request completes. Exact Java/Go compatibility is established before any other observability operation.

The browser never displays the current application key after submission. Replacement requires entering a complete new key; there is no partial reveal or retained visible suffix. Documentation states that browser entry necessarily exposes the key briefly to the browser process, JavaScript request state, developer tools, and sufficiently privileged extensions even though it is never persisted in browser storage.

### Protected terminal alternative

A developer may request a no-echo interactive prompt through a non-secret option such as:

```text
loomspan-console --prompt-for-application-key
```

The key does not appear in command-line arguments, shell history, logs, or ordinary configuration. This path requires a target default from YAML or another already selected non-secret target address and populates the same in-memory credential provider used by browser and enabled MCP operations. The paired browser observes only the resulting authentication and compatibility facts. There is no terminal-owned target session or separate credential lifecycle.

### Runtime replacement

Replacing the selected target, its connection-authority settings, or its application credential follows the settled `TargetContext` rotation rules. It cancels prior-scope operations and clears prior-scope live state, recent activity, acquired traces, indexes, and browser runtime views.

When current-scope evidence exists, the paired browser explains that consequence before the developer confirms replacement. The operation is CSRF-protected. Go does not compare old and new credential values; accepting a replacement credential rotates scope even if the submitted value happens to equal the previous secret.

Runtime replacement remains supported rather than requiring console restart. Application-runtime identity changes already require the same scope, cancellation, and stale-result protections, while restart-only replacement would unnecessarily discard the entire console session, interrupt browser and MCP clients, and make a mistyped or rotated key harder to correct.

### Target result presentation

Overview presents independent target facts rather than one combined connection-health state:

| Fact or result | Presentation |
|---|---|
| Reachable and authenticated | Show connection, authentication, exact compatibility, and runtime identity separately. |
| `TARGET_AUTHENTICATION_REQUIRED` | State that the Loomspan observability key is missing or rejected and request a complete replacement key. |
| `TARGET_ACCESS_BLOCKED` | Explain that host application security or an upstream proxy likely rejected access before Loomspan key authentication. |
| `TARGET_UNAVAILABLE` | Show a sanitized transport category and retain the selected target for retry. |
| `INCOMPATIBLE_TARGET` | Show the console and application release strings and prohibit partial protocol use. |
| HTTP target | Display a persistent **Unencrypted** label and the precise network-exposure warning without repeated confirmation. |
| TLS trust failure | Distinguish safe categories such as untrusted issuer, hostname mismatch, expired or not-yet-valid certificate, or other handshake failure when available; never offer a verification bypass. |
| Redirect response | State that redirects are disabled and were not followed. |
| Observability namespace not found | Suggest checking the target base/context path and whether the application adapter is enabled. |
| Live monitoring unavailable | Preserve successful target, skill, and trace facts while showing live monitoring as independently unavailable. |

Errors remain sanitized and never echo credentials, authentication headers, raw diagnostic payloads, or sensitive internal exception details.

### Retry and reconnect

Temporary transport failure retains the selected target and current `targetScopeId`. Go retries with bounded backoff, and Overview also provides a manual retry action.

- Authentication rejection waits for replacement credentials rather than retrying the same key continuously.
- Exact incompatibility remains until a later explicit recheck or target/release change.
- A reconnect preserving target, credential context, and revalidated `instanceId` preserves scope.
- A revalidated changed `instanceId` rotates scope before any new-runtime result is published.
- Previously acquired complete evidence retains its settled acquisition-time authorization behavior.

Retry never weakens TLS verification, follows a redirect, changes the target origin silently, or treats a host-security rejection as an invalid Loomspan key.

## Browser sessions and local trace storage

### Browser session limits and lifetime

The initial process admits at most eight paired browser sessions and sixteen concurrently registered browser tabs across those sessions. Tabs in one browser profile normally share the same session cookie but retain independent navigation, selection, activity cursor, follow state, filters, hierarchy expansion, and scroll state. Each registered tab owns at most one live relay connection. An excess pairing or tab registration receives an actionable browser-local `LIMIT_EXCEEDED` result without displacing another session or tab or affecting Go's one upstream SSE connection.

A browser session has an eight-hour idle timeout and no lifetime beyond the current console process. A successful authenticated browser request refreshes the idle time, and an admitted authenticated live relay keeps the session active while connected. Expiration rejects later browser requests, closes that session's tab relays, and clears its server-owned session/tab state. It does not rotate `targetScopeId`, clear Go's shared recent-activity interval, stop the upstream SSE connection, delete acquired traces merely because that browser expired, or affect another browser or MCP client.

A page refresh reuses a valid `HttpOnly` session cookie and calls paired bootstrap again. Bootstrap supplies the current `ConsoleStatusSnapshot`, `targetScopeId`, a fresh session-bound CSRF token for browser memory, and the current facts needed to reload the requested route. Refresh does not require re-pairing while the server session remains valid.

The browser may use `sessionStorage` only for scope-bound presentation state: route, selected trace/frame/record identifiers, selected explorer view, expanded hierarchy identifiers, filters and search text, pane sizes, and scroll-restoration hints. It never stores pairing secrets, application or MCP keys, session cookies, CSRF tokens, activity envelopes, skill YAML, trace records, raw payloads, prompts, model responses, tool inputs or outputs, or complete diagnostic responses. Stored presentation state carries enough console-process and target-scope context to be discarded before use after console restart or scope mismatch.

Each tab registers an in-memory tab identifier during bootstrap. A slow or suspended tab may lose its local relay allowance and recover from the current baseline without affecting another tab or the upstream connection. Closing a tab releases its registration and relay state after ordinary connection closure or a bounded stale-tab timeout; the exact detection cadence is an implementation constant.

### Re-pairing after session expiration

An expired browser returns to the unpaired page without restarting the console. Another paired, CSRF-authenticated session may create a new one-time fragment link. If no paired session remains, the unpaired same-origin page may request a manual pairing challenge: Go prints a new one-time secret to the owning console terminal but does not return it to the unpaired HTTP response, and the developer pastes it into the pairing page.

The unauthenticated challenge request still passes loopback Host and same-origin Origin validation, permits only one current challenge, is rate-limited against terminal-output spam, returns no secret or diagnostic data, and does not touch the target, application credential, upstream SSE connection, acquired traces, or other sessions. Successful exchange creates a new browser session but does not revive expired server-owned presentation state.

### User-controlled local trace cache

Acquired trace copies and their derived analysis files form a visible console-local cache rather than a small hidden trace-count pool. The initial cache has no trace-count limit and no separate per-trace byte ceiling. Two restart-time YAML settings control aggregate retention:

```yaml
trace-workspace:
  max-bytes: 4GiB
  idle-ttl: 4h
```

`max-bytes` accepts an explicit `unlimited` value, and `idle-ttl` accepts an explicit `never` value. Numeric zero is invalid and is not an alias for either. The default `4GiB` ceiling applies to the aggregate bytes of complete trace copies, partial acquisitions, indexes, reconstructed-payload files, validation state, search-supporting files, and other application-derived content under `transient`. Marker and lock metadata outside `transient` are not charged. The four-hour TTL is measured from the installed trace's last successful browser or MCP use; both adapters refresh the same last-used time.

With `max-bytes: unlimited`, the console performs no aggregate-capacity eviction and accepts that available filesystem space is the practical capacity. With `idle-ttl: never`, time-based expiration is disabled. Target-scope rotation and console shutdown still cancel users, invalidate handles, and remove current transient contents, and startup still cleans rather than adopts prior-process state.

### Admission and automatic cleanup

The centralized artifact service admits a new trace against the current configured aggregate ceiling. A known response length may permit an early capacity decision; unknown or growing content is charged incrementally. Before returning workspace `LIMIT_EXCEEDED`, the service attempts to create enough room in this order:

1. remove expired unpinned traces; and
2. remove least-recently-successfully-used unpinned traces until the new complete copy and required derived files fit.

There is no count-based eviction. A query pins its installed trace for that query, and Go never removes a pinned trace merely to admit another acquisition. Workspace `LIMIT_EXCEEDED` therefore occurs only when cleanup cannot create enough configured capacity, including when pinned traces occupy the required space or the new trace and required derived files cannot fit even in an otherwise empty cache. Bounded error details identify the workspace ceiling and safe observed values. Other application, response, payload, or concurrency limits may independently return the same shared code with details identifying their own bound.

When `max-bytes` is `unlimited`, aggregate workspace policy does not return `LIMIT_EXCEEDED`. An operating-system disk-full or write-capacity failure instead fails that acquisition with an actionable sanitized local-storage error and removes its partial state when possible. If cleanup restores safe workspace operation, already complete traces remain available and the console may continue. If the workspace can no longer satisfy its process-wide lock, path-safety, or required I/O invariants, the existing fatal workspace-failure rule applies.

Removing or evicting a trace invalidates its shared browser/MCP handle, and a later handle-based call receives `ARTIFACT_EXPIRED`. Reacquisition remains possible only when the selected application's current-instance catalog still offers the artifact and current application authentication permits a new download. Partial acquisition state is removed on failure, cancellation, validation rejection, or target-scope rotation.

### Trace Storage page

The paired browser provides a console-local **Trace Storage** page. It presents facts about the resolved work directory, configured maximum or `Unlimited`, current aggregate bytes, configured idle TTL or `Never`, number of acquired traces, and for each local trace its trace ID, acquired time, last-used time, aggregate local bytes including derived files, calculated expiry when applicable, application availability, local-handle availability, and whether an active query currently pins it.

The page may remove one or more selected unused traces, clear expired traces, or clear all unused traces. Removal invalidates the shared handle. An in-use trace is not removable until its active queries finish; the initial page does not force-cancel them. These operations affect only Go's local cache. They never delete the application's canonical trace or a raw artifact the developer downloaded to another location.

The initial cache does not add permanent pinning, per-browser or per-MCP ownership, reserved adapter capacity, a never-evict flag separate from the configured policies, or adoption of prior-process files. Browser and MCP use the same installed copy, handle, pinning, last-used time, cleanup result, and capacity policy.

## Initial product areas

### Instance overview

Candidate information includes:

- target name and address;
- the application's startup-scoped `instanceId`;
- the application `consoleCompatibilityVersion`;
- connection, authentication, and transport state;
- number of registered skills;
- number of active executions;
- number of traces cataloged by the current application instance; and
- completion grace TTL, application trace-catalog metadata TTL, and trace-retention policy, with an explicit explanation that catalog metadata and core file retention are independent and neither provides cross-restart console history.

### Skill catalog

The skill catalog displays the unique registered skill name and normalized skills-root-relative `sourcePath`, such as `incidents/check_dns.yml`. Skill detail displays the unchanged UTF-8 YAML content received from the application. It does not display an effective-definition DTO or add parsed defaults, resolved model connections, provider identifiers, compiled evidence contracts, Java objects, or registration facts.

`sourcePath` is descriptive metadata for display, grouping, and ordering. It is never treated as a filesystem locator, joined to a local path, or accepted as arbitrary file input. Go follows the server-generated link keyed by the registered skill name. Go and the browser may syntax-highlight or search the YAML as text but do not normalize, reserialize, or maintain an authoritative parsed skill model. Recent activity facts remain separate from the YAML content. The same transport-neutral skill file is available to the future MCP adapter.

The UI must not present recent execution failure as proof that the skill itself is unhealthy.

### Active executions

Candidate information includes:

- entry skill;
- session and trace identifiers;
- start time and elapsed time;
- current phase and active skill path;
- execution status;
- invocation and usage counts;
- configured limits; and
- latest concise activity summary.

Opening an execution should show a concise, continuously updating narrative of observable execution behavior. The UI must not claim to display hidden model thought or chain-of-thought.

Go may refresh one selected active execution through Phase 1 lookup by `sessionId`. That lookup returns the same bounded current registry snapshot used by the collection, not an event history, active trace, or frame tree. Once the execution leaves the registry, lookup returns `NOT_FOUND`; completed diagnostic inspection uses `traceId` when the current-instance catalog or a valid Go-acquired copy still provides it.

Detailed frames, records, and payloads are not available for an active execution. They become inspectable after its trace is finalized and can be acquired while the application retains it. A configured `completion-grace-ttl`, initially defaulting to `15m`, tells the core Java trace subsystem to delay a deletion that `NEVER` or successful `ONERROR` would otherwise perform, providing a bounded acquisition window before the normal persistence policy applies. The adapter catalogs the core-issued finalized descriptor but does not own or extend that retention. A TTL of `0` preserves immediate core deletion and provides no grace window.

The UI must keep execution outcome separate from diagnostic artifact availability. It should not invent combined states such as “completed with trace” or expose a `FINALIZING` execution phase. An execution-completion activity reports the ordinary execution outcome plus `applicationTraceAvailability`, effective `applicationTraceExpiresAt`, and any concise known unavailability reason. The effective expiration is the earlier of application catalog-metadata expiry and any core artifact expiry. When application availability is `AVAILABLE`, Go may acquire the artifact immediately because Phase 1 has already placed it in the current-instance catalog. `UNAVAILABLE` does not change or qualify the execution outcome and says nothing about core filesystem retention or whether Go already holds an acquired copy.

There is no initial pending-artifact workflow or later trace-available event. If the execution-completion activity is missed under the best-effort live model, ordinary trace-catalog refresh determines whether an artifact is currently available. Once activity and transient Go state are gone, the UI must say that an identifier is currently unavailable unless it has evidence that the artifact specifically expired; it must not infer expiration from a generic not-found response.

If Phase 1 reports `EXECUTION_OBSERVATION_ENDED` with reason `CORE_FINALIZATION_FAILED`, the UI removes the execution from its active view and states only that execution observation ended incompletely and no finalized application trace is available. It must not render the event as an execution success or execution failure unless a separate reliably established outcome is present, and it must not fabricate a retry, finalizing, or artifact-recovery workflow. The detailed finalization cause remains in application diagnostics. Failure of Phase 1 to publish this exceptional terminal event makes live monitoring unavailable under the existing fail-closed rule rather than allowing another execution to disappear silently from a supposedly trustworthy view.

Once Go has acquired a finalized trace, its transient cache may continue serving that copy after the application artifact expires while its artifact handle, current console process, and `targetScopeId` remain valid. The copy remains non-authoritative and follows the configured aggregate byte ceiling and idle TTL, including explicit `unlimited` and `never`, plus deliberate Trace Storage removal; it is also removed on target-scope rotation or console shutdown. A console restart cleans rather than adopts the prior process's copies before serving diagnostic operations. Responses report application trace availability separately and include an `artifactHandle` only when Go holds a usable acquired copy. The application adapter stops admitting new downloads at the core-calculated expiration; a download opened before expiration may finish even though core deletion becomes due.

The same acquired copy may remain locally inspectable after the application rejects the currently held upstream credential. The UI must show the selected target's authentication-required status and retain the evidence's original observation facts; it must not suggest that successful cached inspection proves the application is still reachable or authorized. Replacing the application credential rotates target scope and removes the old copy under the ordinary lifecycle rule.

Application trace availability is limited to the selected application's current-instance catalog. A catalog entry expires after the configured application catalog-metadata TTL even when `ALWAYS` or errored `ONERROR` leaves the underlying file in place; metadata expiry does not delete that file. Process-local completion-grace deletion is best-effort: shutdown or crash before it runs may abandon a `NEVER` or successful `ONERROR` file, and no later console process cleans it. After an application restart, the application catalog is empty until the new instance finalizes and publishes new descriptors; the console must not expect the application adapter to scan the core trace location or rediscover files left by an earlier crashed or stopped process. Such files remain core-owned but are intentionally invisible to the protocol. When authoritative revalidation observes the new `instanceId`, `TargetContext` rotates `targetScopeId` under its ownership rules and the trace service deletes Go copies and indexes from the prior instance, so Go does not provide cross-restart trace recovery or history.

### Trace explorer

The detailed developer-audit experience is expected to include:

- application-side current-process trace discovery plus Go console-side filtering and analysis;
- execution summary;
- hierarchical frame and skill tree;
- timeline or waterfall;
- plan creation and evolution;
- model and tool transitions;
- validation, evidence, quota, guardrail, and failure outcomes;
- usage attribution by skill path;
- search and failure-focused navigation;
- raw record inspection for framework developers;
- current trace artifact download under the exact umbrella compatibility match; and
- deep links to a trace, frame, record, or failure.

The Phase 2 UI should consume server DTOs and opaque identifiers rather than filesystem paths or internal Java trace types. A path encountered inside an explicitly requested raw trace record is diagnostic text only and must not become navigation, lookup, or resource-addressing state.

Deep links are navigation conveniences within the current `targetScopeId`, not durable bookmarks. They carry or otherwise retain the scope that produced them so the browser can recognize an old link without reinterpreting its trace ID, frame ID, or record sequence against the current target. Any `TargetContext` scope rotation or Go console restart makes the link stale. Direct navigation to a stale link resets to the console root and reports that the previous target context is no longer available; the console does not preserve old data solely to keep the link working.

## Initial scope decisions

The first console supports:

- one personal developer;
- one console process;
- one selected Loomspan target at a time;
- one embedded browser application;
- REST snapshot and trace access;
- relayed SSE live activity;
- HTTP or HTTPS Loomspan targets, according to host policy;
- application authentication material retained in Go process memory; and
- multiple independent console processes observing the same application.

## Explicit non-goals

Phase 2 initially excludes:

- shared team-service deployment;
- a remotely accessible Go listener;
- HTTPS or Let's Encrypt integration for the Go listener;
- user accounts, OIDC, or a console-owned role system;
- multi-user SSE fan-out as a product workflow;
- multiple simultaneous target instances or fleet aggregation;
- a database or durable console cache;
- a complete Go-materialized mirror of application runtime state or browser presentation state;
- direct browser-to-loomspan connections;
- compatibility with, migration support for, or continued distribution of the deprecated `loomspan-cli` after the console implementation is complete;
- editing skills or application configuration;
- starting, cancelling, retrying, or mutating executions;
- compliance-grade audit features;
- secret scanning, automatic redaction, per-query disclosure confirmation, disclosure tiers, data-loss prevention, provider detection, or a generalized content-classification system;
- durable, lossless, or exactly-once live activity history;
- retroactive invalidation of complete current-scope evidence solely because the application rejects a later upstream request; and
- exposure of hidden model reasoning.

## Architecture invariants

The following should remain true throughout UI design and implementation:

1. The browser never depends directly on Loomspan application endpoints.
2. The REST/SSE console protocol remains explicit and independent of internal Java types. The NDJSON boundary is the current Java writer and enums plus documented consumed invariants and Java-produced executable fixtures; it is not a separately modeled trace format.
3. The Go host remains operationally independent from the observed Java application.
4. The Go plaintext listener remains restricted to loopback.
5. Target transport and authorization policy remain owned by the host application.
6. Credentials do not enter browser storage, URLs, ordinary configuration, or logs.
7. The console does not become a second authoritative store for executions or traces.
8. Live delivery failure does not erase the ability to refresh best-effort current operational state through snapshots and cursors; missing intermediate activity need not be reconstructed.
9. The product remains observational and read-only initially.
10. UI terminology describes observable activity rather than model thinking.
11. Browser handlers adapt transport-neutral Go services; they do not own runtime semantics needed by MCP.
12. Every target-specific response and event follows the authoritative `TargetContext` scope snapshot; stale scopes are discarded, and browser tabs reset after `TargetContext` rotation.
13. Loomspan activity and Go-owned connection events remain in distinct namespaces.
14. Browser and MCP adapters preserve the same transport-neutral Go domain-error code for the same shared-service failure; adapter authentication and protocol failures remain separate.
15. Application authentication is enforced for each new upstream acquisition, while complete evidence already admitted into Go remains governed by local authentication and its normal scope, handle, capacity, and process lifecycle. Upstream rejection does not silently purge it or permit it to masquerade as current state.
16. One config profile, including its static YAML, sibling MCP credential, and managed work directory, is exclusively locked by one Go console process. Static YAML remains restart-only and contains no MCP enablement or MCP access key. The sibling key file is the sole persistent MCP enabled state, and one credential store inside the owning process manages its mutation and process-local authentication generation. Multiple browser sessions and MCP clients may use that process; additional console processes require separate profiles, credentials, and work directories.
17. The production browser assets and Go browser API ship atomically in one executable. The initial product has no browser/API version negotiation, service-worker-preserved application, or browser failure that is reported as Java target incompatibility.
18. Browser and MCP interactive trace analysis use one centralized Go artifact service, one installed immutable copy and handle per current-scope trace reference, and one shared user-controlled cache, TTL, cleanup, pinning, and manual-removal policy. Neither adapter maintains an independent analysis cache or retention lifecycle.
19. Browser and MCP status adapt from the same side-effect-free `ConsoleStatusSnapshot` of independent target, identity, and live-monitoring facts. The snapshot performs no probes, contains no aggregate health state, and never replaces operation-specific admission or errors. Workspace health is a process invariant rather than a reported degraded state.
20. Go, browser handlers, and browser state do not become execution-lifecycle owners. They consume Phase 1 snapshots, activity, and catalog results produced through the exactly-once per-execution observation handle and never independently declare that a Loomspan execution started, completed, or failed.
21. Go's recent-activity window contains events from exactly one continuous upstream interval. An upstream `STALE_CURSOR`, changed `instanceId`, or target-scope rotation clears the window before new activity is admitted. Browser and MCP results may report the reset boundary, but never combine events from opposite sides of a discontinuity.
22. A verified, exclusively locked work directory is a Go process-lifetime invariant. Failure to establish it prevents startup; detected loss of safe access after startup terminates the console. There is no workspace-degraded serving mode, workspace availability status, or workspace-unavailable request error.

## Browser and network safety requirements

Observability data is not trusted presentation content merely because it came from loomspan. Skill descriptions, model output, tool results, errors, trace payloads, routes, and metadata may contain attacker-controlled or malformed text.

The implementation and testing plans must preserve these requirements:

1. Render diagnostic strings as text by default; never inject trace or model content as HTML.
2. Do not render diagnostic content as Markdown or embedded HTML in the initial release; richer content handling is a future feature.
3. Establish a restrictive Content Security Policy suitable for the embedded SPA.
4. Prevent framing and MIME sniffing and use an appropriately restrictive referrer policy.
5. Do not construct executable links or resource URLs directly from trace payloads.
6. Treat downloaded traces as attachments with safe filenames and explicit content types.
7. Bound JSON/SSE message sizes, decompression, nesting, list rendering, and retained in-memory event state.
8. Ensure malformed or oversized records fail one view or request safely rather than destabilizing the console process.
9. Never log credentials, pairing secrets, authorization headers, raw sensitive payloads, or URLs containing user information.
10. Keep browser pairing, host/origin validation, and loopback binding independent; none substitutes for the others.
11. Require a session-bound CSRF header, in addition to pairing and Host/Origin validation, for target or credential changes, MCP enablement or disablement, MCP-key reveal or regeneration, and other sensitive or state-changing local operations.
12. Keep the CSRF token in browser memory only and apply `Cache-Control: no-store` to all authenticated diagnostic and credential-management responses.
13. Keep browser and MCP route authentication and request validation fail-closed and non-interchangeable even though both use the same listener.

The target URL is also a network authority boundary. The console should accept only supported `http` and `https` schemes, reject embedded URL credentials, disable upstream redirects initially, and avoid allowing an unpaired browser request to turn the Go process into a general-purpose network proxy. Target authentication material must never be forwarded to a different origin. The user may intentionally choose an internal or unencrypted target, but that choice must come through the paired console workflow. HTTP is an accepted target transport rather than an override requiring repeated confirmation; the UI must keep its unencrypted state visible and explain the exposure precisely.

The configured application target may be loopback, local-network, or remote and may use HTTP or HTTPS according to the application owner's transport policy. The loopback-only restriction applies to browser and MCP access to the Go console, not to the Go console's upstream target.

The initial threat model protects the Go listener from unauthenticated local or remote callers and malicious web origins through loopback binding, browser pairing, host/origin validation, and the MCP key. Loomspan endpoints separately require application authentication. Compromise of the developer's operating-system account or a process running as that account is outside the initial scope. Diagnostic strings remain untrusted presentation content; comprehensive disclosure controls, redaction, and sanitization are deferred to a future undetermined feature.

## Resource and lifecycle expectations

The console must remain operationally controlled and disposable. The trace cache is bounded by default and may be made explicitly unlimited by the developer; all other resources retain their required bounds:

- no unbounded event queues, goroutines, browser sessions, upstream connections, response payloads, continuations, or search work;
- acquired trace and derived-file bytes follow `trace-workspace.max-bytes`, defaulting to `4GiB` and permitting the explicit developer choice `unlimited`; no acquired copy is deleted during an in-flight query merely to admit new workspace work;
- cancellation and timeouts propagate when the browser disconnects or `targetScopeId` rotates;
- `TargetContext` rotation cancels operations from the prior scope, closes obsolete SSE streams, and clears all application-derived operational and transient trace state as defined by the authoritative ownership section;
- the console uses only its verified, exclusively locked `loomspan-console-work/transient` subtree for disk-backed analysis, cleans it before serving each process, and never adopts prior-process cache entries;
- inability to establish that workspace at startup prevents the listener from opening, and later loss of safe workspace access terminates the console rather than leaving a reduced-function host;
- console shutdown closes listeners, upstream responses, and streams cleanly, removes current transient workspace content best-effort, and releases the work-directory and profile locks;
- browser refresh and temporary disconnect behavior are explicit rather than accidental;
- large traces do not require loading the entire trace into Go or browser memory; and
- ordinary errors in one browser session, artifact, or target response do not terminate the console host; detected workspace-wide failure does.

These console-local controls and Phase 1's two fixed long-lived-operation admission limits are not a claim of comprehensive aggregate resource-exhaustion protection. An explicitly unlimited trace cache accepts filesystem exhaustion as a developer-controlled risk. General request-rate limiting, authenticated-client fairness, adaptive budgets, bandwidth governance, and coordinated capacity management across the application, Go console, browser, and MCP clients remain outside the initial personal developer product.

Implementation planning should set and test concrete values for the resources that remain bounded rather than leave “bounded” qualitative. It must also test the explicit unlimited-cache disk-full path and the distinction between a recoverable acquisition failure and an unrecoverable workspace-wide failure.

## Phase 2 design status

The Phase 2 product and architecture decisions required before implementation planning are settled. The initial implementation uses hand-authored boundary DTOs and executable cross-boundary fixtures without adding a schema or code-generation system. Exact route and field spelling, fixture paths, test harnesses, response-framing constants, and representative client interoperability values are implementation work within the settled contracts rather than additional product-design topics.

## Handoff to future implementation planning

Do not begin Phase 2 integration by reverse-engineering controllers. A future implementation-planning context should first revalidate this document, then use the Phase 1 external agreement, current Java writer and enums, Java-produced JSON, problem-response, SSE, and NDJSON fixtures, cursor behavior, and exact `consoleCompatibilityVersion` policy as explicit inputs. Fixtures assert transport behavior plus the logical reconstruction, hierarchy, timing, usage attribution, failure attribution, validation/retry outcomes, and invalid-artifact results the console actually provides rather than proving only JSON parsing. Go shared services own parsing, chunk reconstruction, and derived analysis; unconsumed metadata remains opaque and requires no parallel Go model.

UI work may proceed against reviewed protocol fixtures or a mock Phase 1 server, but final integration must use the supported Spring web adapter. The mock must model reconnect gaps, trace expiration, failures in nested skill frames, large payloads, exact-version rejection, and unavailable targets rather than only the happy path.

The implementation plan should separate at least these workstreams even if one team delivers them sequentially:

1. project/build/release foundation;
2. Go listener, pairing, target client, and protocol compatibility;
3. REST/SSE relay and bounded lifecycle management;
4. browser application shell and state model;
5. skills and instance experiences;
6. live execution experience;
7. trace explorer and large-trace behavior;
8. content/network security hardening; and
9. cross-boundary contract, failure, and end-to-end testing.

The initial release should be judged by whether a developer can understand a real Loomspan execution safely and reliably, not by how many framework, visualization, or deployment options it contains.
