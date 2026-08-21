# Loomspan Console

Loomspan Console is an independent Go module containing an embedded
React/TypeScript application. A production build creates one executable; it is
not a Maven module.

The executable owns one local profile, one disposable workspace, a paired
browser security realm, and one process-local selected Loomspan application
target.

## Exact build prerequisites

- Go 1.26.5
- Node.js 24.18.0
- npm 12.0.2

The repository declares these values in `go.mod`, `.node-version`, and
`web/package.json`. Direct frontend dependencies are exact versions and
`web/package-lock.json` fixes the transitive graph. Node and npm are build-time
dependencies only.

## Canonical production commands

From `loomspan-console/`:

```text
go run ./internal/buildtool verify
go run ./internal/buildtool build
go run ./internal/buildtool package --expected-version VERSION
```

Both commands check exact toolchain patches, read the direct version from the
root Maven `pom.xml`, install the locked frontend graph, type-check and test the
browser, clean and rebuild assets, generate and verify their integrity
manifest, and run all Go tests. `build` then writes the current-platform
executable beneath `build/` with the same complete version injected.

The product version cannot be overridden. A release caller may add
`--expected-version VERSION` to assert that its expected tag/version equals
the root POM value.

`package` performs the same clean native build, accepts only the current
supported release target, and writes one deterministic archive plus its
`.sha256` sidecar beneath `dist/`. Validate an extracted archive on its native
host with:

```text
go run ./internal/buildtool smoke --expected-version VERSION --archive dist/ARCHIVE
```

The release names are
`loomspan-console-VERSION-windows-x86_64.zip`,
`loomspan-console-VERSION-linux-x86_64.tar.gz`, and
`loomspan-console-VERSION-macos-arm64.tar.gz`. Each has one top-level directory
containing the executable, `LICENSE`, the runtime `README.md`, and the exact
six-file portable Agent Skill at `skills/loomspan/`
(`SKILL.md` plus five files in `references/`). The skill's `1.0.0` metadata is
versioned independently from Console and is not target negotiation.
Check `SHA256SUMS` with `sha256sum -c SHA256SUMS` on POSIX systems; in
PowerShell compare `(Get-FileHash -Algorithm SHA256 .\\ARCHIVE).Hash` with the
matching entry. `.github/workflows/console-ci.yml` runs Java fixture/adapter,
canonical Console, and Playwright verification. The tag/manual validation
workflow in `.github/workflows/console-release.yml` builds and smokes all three
native targets; only its final tag-gated job can publish.

Run the browser workflow suite after a canonical build with:

```text
npx playwright install chromium
npm --prefix web run test:e2e
```

Generated browser assets, dependencies, coverage, Playwright output, and
binaries are ignored. Only
`internal/webassets/generated/embed-placeholder.txt` is tracked; it is not a
valid production asset. Never compile or distribute a manually generated or
previous asset directory.

## Runtime configuration

The executable accepts:

```text
loomspan-console --version
loomspan-console [--config FILE] [--work-dir DIRECTORY]
  [--listen 127.0.0.1:7943] [--development-origin http://127.0.0.1:5173]
  [--target-address http://127.0.0.1:8080] [--no-open-browser]
  [--prompt-for-application-key]
```

`--version` validates the release and embedded assets without creating files or
opening a listener. `--config` selects one exact YAML file and its resolved
parent identifies the profile. `--work-dir` selects one exact managed work
root. `--listen` overrides the YAML listener for this process only.
`--development-origin` adds exactly one canonical HTTP loopback Vite
authority/origin pair for this process; it is never persisted or enabled by a
production default. `--no-open-browser` suppresses only the browser-opening
attempt. A pairing URL is still printed. `--prompt-for-application-key`
requires a configured YAML target and reads its application key from an
interactive terminal without echo. The Boolean flag, not the key, is visible
in shell history and the process command line.

`--target-address` prefills the Target screen without selecting, probing, or
connecting to the application. When `LOOMSPAN_OBSERVABILITY_API_KEY` is set,
its value prefills the Application key field after the browser is paired. These
defaults leave connection under explicit user control: the user must still
select **Connect**. Both defaults are process-only and are never persisted.

The schema-version 1 configuration is strict and restart-only:

```yaml
version: 1
listener:
  address: 127.0.0.1:7943
trace-workspace:
  max-bytes: 4GiB
  idle-ttl: 4h
target:
  address: https://application.example/context
  connect-timeout: 5s
  response-header-timeout: 10s
  request-timeout: 30s
```

Unknown fields, duplicate keys, multiple documents, aliases, unsafe bounds, and
unsupported versions are rejected. Listener addresses must contain the
explicit IPv4 loopback literal `127.0.0.1` and a port. `max-bytes` accepts positive
integer `KiB`, `MiB`, `GiB`, or `TiB` values and the exact sentinel
`unlimited`. `idle-ttl` accepts positive integer `s`, `m`, or `h` values and the
exact sentinel `never`. Numeric zero is invalid. Configuration never contains
pairing, session, CSRF, application, or MCP credentials.

The entire `target` mapping is optional; the generated default omits it and
performs no application request. When present, `address` is required and may
be an exact hierarchical HTTP or HTTPS origin with an optional clean context
path. Browser-selected addresses reuse the restart-only timeout and trust
policy and are not written to YAML. Network durations are positive canonical
integer `s`, `m`, or `h` values; unlike workspace retention they do not accept
`never`.

`ca-bundle` is resolved relative to the configuration file, is loaded and
validated before listening, and may contain up to 1 MiB of PEM certificates.
Those certificates augment operating-system roots. They never replace system
trust, disable hostname or validity checks, or enable an insecure mode.
Application requests are made directly to the normalized selected authority,
ignore environment proxy variables, never follow redirects, and have bounded
headers and bodies. HTTP targets are allowed but the Overview persistently
warns that the key and observability data cross the network without encryption.

The application key exists only in Console and paired-browser memory and is
forgotten at Console shutdown. It can be entered in a paired browser, supplied
through `LOOMSPAN_OBSERVABILITY_API_KEY`, or read through the protected
terminal prompt; all paths use the same credential provider and selected-target
lifecycle. Selecting another target, replacing an existing key, or observing a
changed application instance resets the opaque target scope and discards
target-derived browser state. Supplying the first key for a configured
credential-less target preserves its scope. Authentication rejection, access
blocking, incompatibility, and temporary connection failures remain separate
facts and do not rotate the scope by themselves.

Default configuration files are:

- Windows: `%AppData%\loomspan\Console\config.yaml`
- macOS: `~/Library/Application Support/loomspan Console/config.yaml`
- Linux: `$XDG_CONFIG_HOME/loomspan-console/config.yaml`, or
  `~/.config/loomspan-console/config.yaml`

Default workspace parents are:

- Windows: `%LocalAppData%\loomspan\Console\workspaces`
- macOS: `~/Library/Caches/loomspan Console/workspaces`
- Linux: `$XDG_STATE_HOME/loomspan-console/workspaces`, or
  `~/.local/state/loomspan-console/workspaces`

The workspace leaf is the full lowercase SHA-256 identity of the resolved
profile directory. A managed root contains the exact
`.loomspan-console-work` marker, `.lock`, and `transient/`. Existing unmarked or
wrongly marked directories are refused without mutation. Profile and work
locks exclude another Console process. `transient/` is disposable: prior
process contents are never adopted, indexed, or served and are removed before
listening. Cleanup does not follow symbolic links, junctions, or reparse
points, and it is not secure erasure. On shutdown, current transient content is
removed best-effort; the empty root, marker, and unlocked lock file may remain.
Loss of the verified workspace invariant terminates the service instead of
creating a degraded or fallback workspace.

## Browser pairing and sessions

After the listener is bound, Console prints a five-minute, one-use pairing URL
and normally asks the operating system to open it. Browser-opening failure is
nonfatal. The 256-bit pairing value is carried only in the URL fragment; the
SPA removes it from the current address/history entry before exchanging it in
a same-origin JSON body. This short-lived fragment is the sole URL exception:
reusable credentials must never appear in URLs, logs, YAML, or browser
storage.

Successful pairing creates a process-local `HttpOnly`, `SameSite=Strict`,
nonpersistent `LOOMSPAN_console_session` cookie. It intentionally omits
`Secure` because the listener is plaintext HTTP bound to an explicit loopback
IP; remote and wildcard binding remain prohibited. A process admits eight
browser sessions and sixteen tabs total. Sessions expire after eight idle
hours, and disconnected tab registrations expire after two minutes. Open tabs
send an authenticated in-memory heartbeat; a resumed or restored tab
re-registers automatically if its prior registration expired. Each tab holds
its independent CSRF token only in React memory. Refresh reuses a valid cookie
and receives fresh bootstrap/tab state. If pairing expires, the
same-origin unpaired page can request a rate-limited new value printed only to
the owning terminal, or another paired tab can create a new fragment link.

Every browser API request requires the exact bound Host and matching Origin.
Sensitive operations additionally require the browser cookie, tab identity,
and CSRF header. Browser/MCP route realms are selected before authentication,
so an MCP bearer credential cannot substitute for browser controls.
Authenticated and security responses are `no-store`; only verified
content-addressed static assets are immutable. The browser stores only the
presentation theme in `sessionStorage`.

## MCP integration

MCP is disabled until a paired developer enables it on **Settings > MCP
Integration**. Console then creates the protected profile sibling
`mcp-access-key`; its exact contents are `lsmcp_`, 43 unpadded base64url
characters, and one LF. The key is independent from browser, pairing, CSRF,
and target-application credentials. It survives Console restart until
explicitly disabled. An existing malformed or insufficiently protected
canonical file keeps MCP disabled and must be removed explicitly in Settings;
Console never repairs or reveals its contents.

The only endpoint is exact `http://127.0.0.1:PORT/mcp` (a client may use
`localhost` with the same port). There is no `/mcp/` or `/api/mcp/` alias and no
MCP YAML setting. Clients send exactly one `Authorization: Bearer KEY` header.
Requests are stateless Streamable HTTP, bounded to 1 MiB and ten seconds, and
negotiate MCP `2026-07-28` or compatible `2025-11-25`. Console validates the
current-port Host, any supplied Origin, enabled state, and bearer key before
protocol body processing. Forwarded host headers, IPv6 authorities, foreign
origins, wrong ports, OAuth discovery, and cross-realm credentials are rejected.

The read-only tool surface is:

- `LOOMSPAN_get_runtime` for side-effect-free Console/target status;
- `LOOMSPAN_list_skills` and `LOOMSPAN_get_skill` for registered skill
  metadata and unchanged YAML;
- `LOOMSPAN_list_executions` and `LOOMSPAN_get_execution` for bounded,
  provisional active-execution snapshots; and
- `LOOMSPAN_get_execution_activity` for a bounded, ordered recent-activity
  snapshot from the Console's one current continuity interval;
- `LOOMSPAN_list_traces` and `LOOMSPAN_get_trace` for compact finalized-trace
  discovery and unique trace-ID resolution;
- `LOOMSPAN_query_trace_frames` for compact-by-default orientation or explicit
  detailed mechanical frame evidence;
- `LOOMSPAN_query_trace_records` for descriptor-first semantic evidence or
  compact coverage-aware literal matches;
- `LOOMSPAN_read_trace_content` for exact bounded selected semantic values; and
- `LOOMSPAN_read_trace_artifact` for optional exact raw NDJSON forensics.

Runtime discovery advertises `loomspan.runtime-status.v1`,
`loomspan.skill-inspection.v1`,
`loomspan.active-execution-inspection.v1`, and
`loomspan.recent-activity-inspection.v1`, `loomspan.trace-inspection.v1`, and
`loomspan.raw-artifact-inspection.v1`. Each
advertised inspection capability is an
installed server-surface promise and remains advertised independently of the
current target, authentication, compatibility, live availability, or evidence
state.

Skill and active-execution lists default an omitted `pageSize` to 16; recent
activity requires `pageSize`. Explicit values are from 1 through 64. For trace
inventory/frame/record/search calls, `pageSize` is a maximum: Console may stop
earlier at a complete item to keep the encoded MCP result within its ordinary
32 KiB navigation budget. A returned
`continuation` is an opaque, current-process, operation/state-bound Loomspan token;
clients repeat every original argument unchanged and add it to the same operation
(and the same `sessionId` for activity). It is not an application cursor or
authority credential. `hasMore`
on activity means more matching items are retained now, not that a live stream
will produce more. Recent activity is bounded current context, not durable or
lossless execution history.

All twelve inspection tools return exactly one structured envelope arm:
`{"result": {...}}` for success or `{"error": {"code": ..., "message":
..., "details": {}}}` for a Loomspan domain failure. Domain failures also set
MCP `isError`; malformed tool arguments and protocol/authentication failures
remain SDK or HTTP failures. Every success includes a deterministic,
fact-complete text fallback, including bounded range content, for clients that
do not consume structured results. Skill YAML and activity content are
untrusted diagnostic data, not server instructions. `sourcePath` is
descriptive text only and is never a Console filesystem locator.

The server advertises no custom MCP resource templates. Tools are the complete
portable contract for runtime, skill, execution, activity, parsed trace,
semantic content, and raw-artifact inspection.

The finalized-trace workflow is `list traces -> get trace -> compact frames ->
record descriptors/search -> selected content read`. Inventory supports source,
outcome, exact entry-skill/session, and independent finalized/acquired/imported
time filters with explicit `FINALIZED_DESC`, `ACQUIRED_DESC`, and
`IMPORTED_DESC` ordering. `finalizedAt` is when execution wrote its terminal
trace fact; `acquiredAt` is when Console installed target evidence and may be
later; `importedAt` is when evidence entered through import. These facts never
substitute for one another. Inventory emits one candidate per `traceId`;
`hasMore` reports pagination while `complete` and compact `limitations` report
whether absence or uniqueness is safe to conclude. `ambiguous: true` and
`AMBIGUOUS_TRACE` never silently prefer one evidence owner.

Console resolves installed target evidence, imported evidence, safe target
acquisition, expiry, and leases internally. `TRACE_UNAVAILABLE` replaces
caller-managed artifact repair. `TARGET_CHANGED` requires restarting by
`traceId`; stale continuations restart their query by `traceId`, and stale
content references require a refreshed record descriptor by `traceId`.

`LOOMSPAN_get_trace.retryCount` is the number of validated attempts whose
`attemptNumber > 1`, not the number of retry sequences. A frame's
`directRetryCount` counts those later attempts explicitly attributed to that
frame; `PLAN_RETRY_REQUESTED` is unrelated planning-quality evidence.
For a failed or aborted trace, `terminalFailureId` is the recorded terminal
failure pointer; use it as an exact record-query `failureId` filter to retrieve
the terminal failure fact and its sequence before reading selected diagnostics.
Use frame-query `filter.minDirectRetries` to select frames whose existing
`directRetryCount` meets a minimum. The filter uses only validated later
attempts explicitly attributed to the exact frame; it does not propagate
descendant retries or determine a cause or anomaly.
`recordCountsByType` is the complete histogram of nonzero physical record
types, so omitted known keys mean zero and its values sum to `recordCount`.
Use selected keys with the paginated record query; do not infer logical
failures, gaps, uncertainties, usage completeness, or terminal outcome from
the physical histogram.

Frame queries default to `COMPACT`, which retains orientation/count facts but
omits duration, usage, and identity detail. `DETAILED` supplies elapsed-
millisecond duration and rich usage/retry/validation/failure/gap/uncertainty
facts. A frame has an optional scalar close `outcome`; no close status means it
is absent. Frame filters compose with AND semantics, including
`minDirectRetries` against the existing exact-frame count. Record timestamps
are epoch milliseconds; frame durations are elapsed milliseconds.

Trace pages accept at most 64 complete items. Content descriptors are returned
without bytes by default. Explicit `inlineContent` includes complete values no
larger than 8 KiB in record order under a 32 KiB aggregate source-byte budget;
an omitted value retains its descriptor and typed reason. Content and raw
reads use exact source-byte offsets. Omit both `start` and `continuation` for
the initial read at zero; supply at most one. The default is 1 KiB, while an
explicit request may select at most 16 MiB (16,777,216 source bytes)
per call. A larger request returns `LIMIT_EXCEEDED` with `rangeBytes` and the
shared limit; successful responses are never silently shortened to the limit.
Continue while `hasMore` is true to reconstruct all retained bytes. Base64
expansion is response encoding and does not change the reported source-byte
offsets or total length.

Top-level activity `observedAt` is the time the shared window was queried;
`continuity.observedAt` is the upstream interval observation fact. A reset or
`beginningUnavailable` explicitly limits the returned interval. When live
monitoring is unavailable, active-execution application operations and recent
activity return `LIVE_MONITORING_UNAVAILABLE`; retained activity is not exposed
as current. Finalized trace discovery and analysis use the trace-inspection
tools above; active execution snapshots remain provisional and are not tailed
as traces.

Regenerating or disabling freezes new work, cancels and drains admitted work,
and closes temporary SDK sessions before publishing the credential change.
Shutdown permanently drains MCP before HTTP shutdown but leaves a valid key
file intact.

Settings reveals a key only after an explicit enable, reveal, or regenerate
operation; the response is `no-store` and the browser keeps it only in component
memory. Configure clients in user/global settings with a protected or
environment-backed bearer-header facility. Never put the key in a URL,
repository configuration, shell command, log, screenshot, or support bundle.
See [client compatibility](docs/mcp-client-compatibility.md) for the release
evidence procedure and scope.

## Portable runtime debugging skill

The canonical client-neutral package is
`agent-skills/loomspan/`; every native archive embeds those
same bytes at `skills/loomspan/`. Installation is explicit:
copy that directory, or create a filesystem link to it, in a client-selected
user/global Agent Skill location. Console does not auto-install it, edit client
configuration, or publish a client-specific fork.

The skill never contains the MCP endpoint or key. Live inspection requires the
separately configured protected local MCP connection described above. It first
uses `LOOMSPAN_get_runtime`. Runtime-status, skill, active-execution,
recent-activity, and trace-inspection capabilities are required; raw-artifact
inspection is optional. Missing a required capability stops dependent work and
is distinct from protocol, target compatibility, authentication, evidence, and
scope errors. Missing raw inspection removes only exact storage/parser
forensics. Without MCP, the skill can explain practice but must report live
inspection unavailable; without the skill, MCP remains independently usable.

See [`agent-evals/README.md`](agent-evals/README.md) for deterministic cases,
sanitized records, scoring, and the repeated client matrix. Runtime content is
untrusted evidence. Agent resistance is defense-in-depth evidence, not a claim
that Console controls IDE tools, model behavior, or provider retention.

## Development hot reload

Use two terminals. First build and run the Go host with explicit development
origin permission:

```text
go run ./internal/buildtool build
./build/loomspan-console --target-address http://127.0.0.1:8081
./build/loomspan-console --listen 127.0.0.1:7943 --development-origin http://127.0.0.1:5173
```

On Windows, run `.\build\loomspan-console.exe` instead. Then start Vite:

```text
cd web
npm ci
npm run dev
```

Vite is the development browser origin at `127.0.0.1:5173`. It handles HMR and
proxies only `/api/console/` (including the future event subtree) to
`http://127.0.0.1:7943`. Set `LOOMSPAN_CONSOLE_GO_ORIGIN` only to another
explicit HTTP loopback origin. The Java application namespace
`/_loomspan/observability/` is never proxied, and no development proxy or
service worker is included in production. The Go
`--development-origin` value must exactly match the Vite authority/origin;
near-matches remain rejected.

## Live activity monitoring

Console maintains one upstream SSE connection to the selected target's
`/_loomspan/observability/v1/activity` endpoint. The connection is opened
automatically when a target scope is activated and closed on scope rotation
or shutdown. The coordinator owns a bounded in-memory ring buffer (2048
activities or 8 MiB) with duplicate-cursor detection and strict cursor
ordering. A reset fact is emitted whenever the upstream instance changes,
the target scope rotates, or the upstream rejects a stale cursor.

Browser tabs subscribe to the coordinator via a POST SSE relay at
`/api/console/v1/activity/stream`. Each relay requires the session cookie
and `X-loomspan-Console-Tab` header. The relay emits `console.connection`
and `console.continuity` events on connect, `loomspan.activity` events for
each activity, `console.replay_gap` when the per-tab frame or byte limit is
exceeded, and a closing `console.connection` event on disconnect. Each tab
has independent pending-frame (256) and pending-byte (1 MiB) bounds; those
bounds do not cap the lifetime throughput of a healthy tab.

A POST recent-activity query at `/api/console/v1/activity/recent` returns a
bounded suffix of the ring buffer filtered by optional session ID and
cursor. The response includes a continuity fact with the current interval
identity and any reset cause, plus a `beginningUnavailable` flag when
earlier activity was evicted.

The coordinator periodically refreshes the active-execution baseline (every
30 seconds) and signals adapters to reload authoritative snapshots. If the
upstream reports `LIVE_MONITORING_UNAVAILABLE`, the coordinator enters a
terminal state and stops reconnecting. Reconnect backoff is exponential with
jitter, capped at 30 seconds.

The React activity experience renders a target-wide recent narrative on the
overview and filters the current summary, bounded active path, and
follow/pause narrative to the execution selected in the active-execution
detail route. Completion preserves that selected route and exposes trace
inspection only when the terminal activity reports availability. Active and
temporary recent-completion collections remain separate. Connection, reset,
freshness, and replay-gap facts are announced
via ARIA live regions. The SSE decoder handles split UTF-8 chunks
incrementally and validates event names against the `loomspan.activity` and
`console.*` namespaces.

## Artifact cache, saving, and opening trace files

Console maintains one local evidence cache beneath the verified workspace
`transient/` subtree. Acquiring a trace artifact downloads the finalized
NDJSON trace from the upstream application, validates it through the trace
analysis processor, and installs one randomly named immutable bundle directory
containing the raw artifact component and processor-derived components (such as
the analysis manifest). Each installed entry carries an opaque handle, an
aggregate charged byte count (raw plus derived), and an idle TTL deadline.
Entries with active leases (pins) cannot be removed; expiry during a pin defers
removal until the last lease closes.

### Capacity and eviction

The `trace-workspace.max-bytes` setting bounds the aggregate bytes charged
across target and imported partial reservations and complete installed bundles. The charged
byte count for one entry is the sum of its raw artifact bytes and all derived
component bytes. When the cache is full, expired unused entries are evicted
first, followed by least-recently-successfully-used (LRU) unpinned entries.
Active leases are never evicted. The sentinel `unlimited` disables
aggregate-capacity eviction entirely.

### Idle TTL

The `trace-workspace.idle-ttl` setting controls how long an unused entry
remains after its last successful handle use. The TTL clock starts at the
moment a lease closes successfully; viewing the storage snapshot or listing
traces does not refresh it. The sentinel `never` disables idle expiry.
Neither `unlimited` nor `never` changes scope rotation, shutdown, or restart
cleanup.

### Lifecycle and cleanup

Each local artifact is explicitly labeled `TARGET` or `IMPORTED`. Target
entries are bound to their acquiring target scope, and rotation invalidates
only entries for the prior scope. Imported entries use one process-local owner
and remain available across target rotation. On process shutdown the entire
`transient/` subtree is cleaned. On restart, any prior-process bundle
directories beneath `transient/` are removed before the workspace is served;
no prior-process cache metadata is adopted.

### Handles and paths

Artifact handles are opaque, process-local, and evidence-owner-bound identifiers.
They are never durable across restarts, never shared between processes, and
never derived from filesystem paths. Local filesystem paths never appear in
handles, browser DTOs, HTTP headers, error messages, fixtures, or logs.

### Browser API operations

The browser API exposes six artifact operations:

- **Acquire** (`POST /api/console/v1/artifacts/acquire`) — installs or joins
  an existing local copy for a trace ID. Returns the opaque handle, local
  byte count, and expiry facts. Requires session cookie, tab ID, and CSRF.
- **Import** (`POST /api/console/v1/artifacts/import`) — accepts one raw
  `application/x-ndjson` request body, validates the complete trace through the
  same processor, and installs it as `IMPORTED` evidence. It requires session,
  tab, and CSRF authentication and accepts no multipart metadata or query.
- **Storage snapshot** (`POST /api/console/v1/artifacts/storage`) — returns a
  side-effect-free view of the cache including charged bytes, entry count,
  and per-entry metadata. Requires session cookie only (no CSRF).
- **Remove** (`POST /api/console/v1/artifacts/remove`) — removes one unused
  entry. Returns `ARTIFACT_IN_USE` (409) if the entry has an active lease.
  Requires session cookie, tab ID, and CSRF.
- **Clear expired** (`POST /api/console/v1/artifacts/clear-expired`) — removes
  all expired unpinned entries. Requires session cookie, tab ID, and CSRF.
- **Clear all unused** (`POST /api/console/v1/artifacts/clear-all-unused`) —
  removes all unused entries regardless of expiry. Pinned entries are
  preserved. Requires session cookie, tab ID, and CSRF.

### Save and open trace files

**Save trace file** uses `GET /api/console/v1/artifacts/{traceId}/raw` to stream a
finalized trace artifact directly from the upstream application without
consulting or mutating the local cache. It requires a valid `SameSite=Strict`
session cookie and exact Host match but no CSRF token. Cross-site and
same-site fetch metadata is rejected. Query parameters, `Range`, and
conditional request headers are rejected. When Fetch Metadata is present, the
request must use navigation mode; ordinary same-origin `fetch()` calls are
rejected. The response is
`application/x-ndjson` with a safe `Content-Disposition: attachment`
filename and `no-store` cache control. Raw download is separate from
analysis acquisition: it never installs, handles, pins, or charges bytes,
and it performs a fresh authenticated upstream stream each time.

**Open trace file** streams the selected file directly from the browser to the
import route; React does not parse it or load it through `FileReader`. A file is
portable only when its first `TRACE_STARTED` record has the exact same nonblank
`consoleCompatibilityVersion` as Console. Two `development` values are allowed
only as a best-effort current-checkout match; released/development pairs and
unequal releases are rejected. The request ceiling is 4 GiB, further reduced
by a finite `trace-workspace.max-bytes`. Derived indexes share that same global
capacity, so an otherwise valid raw file can still fail installation. Importing
the same trace ID twice in one Console process returns
`ARTIFACT_ALREADY_EXISTS`; it never replaces the installed copy.

Saved trace files may contain sensitive diagnostics, tool arguments, stack
text, and application paths. The compatibility marker establishes reader
compatibility only—not authenticity, integrity, or provenance. Imported copies,
handles, indexes, and cursors remain transient and are discarded at shutdown
or restart.

### Trace enrichment

Trace list and detail responses are enriched with `localAvailable` (whether
an installed copy exists), `artifactHandle` (the opaque handle when
installed), and `applicationAvailability` (the last observed upstream
availability at acquisition time). These facts are distinct from the
observability service's current application-side metadata. The Trace Storage
view in the browser shows the full cache snapshot with per-entry removal
actions and bulk clear operations.
