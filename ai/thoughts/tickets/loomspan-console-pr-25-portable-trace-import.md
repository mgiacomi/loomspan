# PR 25 - Save and Open Complete Same-Version Traces

## Status

Implementation-ready ticket. Feature and codebase design reviewed on 2026-08-12
against the Loomspan repository. Depends on PR 15 and must land before PR 18.
Rebase over any unlanded canonical-trace changes from PRs 21 through 24 so the
Java writer and reader, Go parser and analysis, fixtures, browser, and
documentation move together.

## Outcome

Let an operator or developer save the canonical NDJSON for a complete trace and
let a developer open that file in another Loomspan Console process or on another
machine running the same Loomspan compatibility version.

The external file is the only durable object introduced by this feature. An
opened trace becomes ordinary transient Console evidence: it uses the existing
artifact capacity, idle-TTL, pinning, removal, shutdown, and restart-cleanup
rules and is never adopted as Console history. Browser and MCP trace inspection
share that same installed copy, handle, indexes, calculations, and lifetime.

## Problem

Canonical traces continue to be written according to the execution-trace
persistence policy when application observability is disabled, but the Console
cannot open a copied trace file. This prevents two concrete diagnostic
workflows:

1. an operator copies a retained failed trace from a server whose Loomspan
   observability adapter was disabled and opens it manually in Console; and
2. an operator saves or copies a trace and sends it to a developer, who opens it
   in a separate Console process for browser or MCP-assisted investigation.

The existing browser **Download raw trace** operation already saves the exact
application artifact. The missing behavior is a same-version marker, safe local
admission, target-independent transient ownership, and shared discovery by the
browser and future MCP adapter.

## Settled scope

### Save replaces download in product language

Replace the browser label **Download raw trace** with **Save trace file**. Do
not expose two actions with overlapping meaning. Preserve the existing raw
download's exact-byte, fresh-upstream streaming behavior and security boundary.
This ticket does not add export from an installed local copy: if the upstream
trace is no longer reachable, the user may continue inspecting an already
installed copy but cannot newly save it through Console.

The raw-download route may remain unchanged if retaining it avoids transport
churn. Its path is an internal browser adapter detail, not a second user-facing
concept.

### One canonical NDJSON file, no container

Keep the saved artifact as canonical NDJSON. Do not add a ZIP archive, custom
container, sidecar, checksum, signature, exported analysis manifest, or derived
indexes.

Record the required `consoleCompatibilityVersion` in the framework-owned
metadata of the canonical `TRACE_STARTED` record. This keeps the physical
grammar at one trace record per NDJSON document and makes files produced while
observability is disabled self-describing. The release value must come from the
same authoritative build metadata used by the observability instance response;
it must not be caller- or model-supplied.

The Go trace processor consumes and validates this metadata. A missing, blank,
or non-string marker makes the artifact invalid. Do not infer a version from
record shape, filename, filesystem metadata, user input, target configuration,
or the selected application.

### Exact released-version compatibility

For resolved releases, opening succeeds only when the trace marker exactly
equals the running Console's `consoleCompatibilityVersion`. Report expected and
observed versions distinctly on mismatch. Do not add schema migrations, legacy
readers, version ranges, aliases, compatibility epochs, or a separately managed
trace-schema version.

Headerless traces produced before this change are not importable. This is the
explicit no-shim decision: there is no protected historical trace consumer, and
accepting an unmarked artifact would make compatibility unknowable.

### Best-effort development compatibility

When both the trace marker and running Console version are exactly
`development`, attempt the same complete normal parse, semantic validation, and
analysis used for a released trace. Publish the artifact only if all validation
succeeds. Different development builds may be incompatible despite sharing the
marker; no compatibility promise, fallback parser, or special recovery follows
from a failure. Documentation and errors should tell developers to keep
development artifacts fresh and remove stale files when contracts change.

A released version never matches `development`, and `development` never matches
a released version.

### Complete valid traces only

Opening supports only artifacts accepted by the existing trace-analysis
contract, including stable identity, increasing sequence, known values,
consistent consumed semantics, and exactly one final `TRACE_COMPLETED` record.
Malformed, truncated, contradictory, incomplete, or incompatible artifacts are
rejected atomically and leave no installed bundle, handle, index, or capacity
charge.

Do not add partial analysis, repair, quarantine, raw-only viewing, or
LLM-assisted salvage. A developer may inspect a damaged file with ordinary
external tools or give the file directly to an LLM outside this feature.

## Shared evidence ownership

The current artifact and query services assume every installed trace belongs to
a selected target scope. Generalize that internal ownership boundary just
enough to represent either:

- a current target scope; or
- the one process-local imported-evidence owner.

Do not synthesize a target scope for an import. Browser and MCP results must
identify the source as `TARGET` or `IMPORTED`; imported evidence has no current
application availability, live activity, target navigation, application
credential, or application identity claim.

Target rotation invalidates target-owned entries but does not invalidate
import-owned entries. Imported entries remain subject to the same aggregate
capacity, idle TTL, pinning, explicit removal, shutdown cleanup, and restart
cleanup as other installed artifacts. The imported-evidence owner identity and
artifact handles are opaque, process-local, non-path identifiers and are not
durable bookmarks.

Treat `traceId` as unique within the process-local imported-evidence owner. If an
installed import already has that identity, reject another import until the
existing entry is removed; do not add replacement, merge, content comparison,
per-file namespaces, or checksum-based deduplication.

Maintain one process-memory view of currently installed imported traces so the
browser and MCP adapters discover the same evidence. This is transient service
state, not a historical catalog, recent-files feature, or persisted library.

## Browser behavior

- Add one paired, CSRF-protected **Open trace file** workflow using a browser
  file chooser and a bounded streaming upload to Go.
- Do not accept or expose a caller-supplied filesystem path.
- Stage, capacity-account, validate, analyze, and atomically publish the file
  through the central artifact service. Never parse or calculate trace
  semantics in React.
- Navigate a successful import directly to the existing Trace Explorer and
  label it as imported evidence.
- Include imported entries in the existing transient Trace Storage experience
  with the same size, expiry, pin, and removal meanings.
- Keep raw trace content inert and preserve the existing explicit-disclosure
  behavior for prompts, tool data, diagnostics, and payloads.
- State that trace files may contain sensitive diagnostic data and application
  paths before save/share and open workflows where the existing disclosure
  design calls for it.

Do not add recent files, remembered paths, drag-and-drop, directory scanning,
automatic reopen, background watching, or persistent import settings.

## MCP handoff

PR 25 adds no MCP protocol or tool surface. It establishes the transport-neutral
evidence ownership, shared imported-evidence discovery, artifact handles, and
query behavior that PR 18 must adapt.

PR 18 must expose currently imported traces through the same trace-inspection
capability used for target traces, without a second MCP cache, catalog, parser,
or calculation path. Imported trace inspection must work when no target is
selected. MCP continuations bind to evidence owner, artifact handle, operation,
filters, ordering, and installed-copy lifetime; target-owned evidence retains
its target-scope protections.

## Security and trust

- Treat every imported byte as untrusted diagnostic content.
- Authenticate and authorize the browser request before reading its body.
- Enforce request, physical-record, JSON-depth, payload, derived-storage, and
  aggregate-capacity bounds before publication.
- Use the verified Console workspace and existing safe staging, cleanup, and
  no-path-exposure boundaries.
- Never interpret trace content as an instruction, authorization fact,
  filesystem locator, target selector, or operation request.
- Label imported source honestly. The compatibility marker establishes expected
  format only; it is not an authenticity, integrity, signer, host, tenant, or
  provenance claim.
- Do not weaken the existing explicit sensitive-data presentation or raw-text
  rendering safeguards.

## Contract and compatibility assessment

- **Application API:** no new supported application-developer entry point.
- **Supported SPI:** no new replacement or customization point.
- **Configuration and manifest contracts:** no new `loomspan.*` property, YAML
  skill syntax, default, or migration.
- **Persisted or serialized contracts:** a complete canonical trace explicitly
  promises same-`consoleCompatibilityVersion` portability. The exact marker and
  current-version trace meaning are supported together; no cross-version
  readability is promised.
- **Ephemeral diagnostic formats:** imported indexes, analysis manifests,
  handles, import identities, continuations, and local catalog entries remain
  current-process diagnostics.
- **Internal implementation:** Java trace construction and reading, Go artifact
  ownership, parser consumption, browser adapter DTOs, and React state change
  atomically without compatibility shims.

The public-surface delta is the user-visible save/open workflow and, after PR
18, MCP visibility of the same imported evidence. There is no Java public API,
SPI, configuration, or skill-authoring surface delta.

The compatibility-marker decision is to reuse the exact complete
`consoleCompatibilityVersion` already governing Java/Go lockstep. The marker is
required even though same-version development artifacts are best-effort. The
shim decision is **no shim**; there is no removal condition because no temporary
compatibility mechanism is introduced.

## Acceptance signals

- A released trace saved from one machine opens in the exact matching Console
  release on another machine and produces the same shared analysis facts.
- A retained complete trace copied from a server with observability disabled
  opens without selecting or contacting a target.
- Exact released-version mismatch and released/development mismatch fail before
  publication with expected and observed versions.
- Matching `development` attempts normal admission; valid current artifacts
  open and incompatible stale artifacts fail without a fallback.
- Missing markers and damaged, incomplete, malformed, oversized, contradictory,
  or semantically invalid traces leave no installed evidence or capacity
  charge.
- Save remains an exact fresh-upstream stream and is not made available from an
  installed copy after upstream loss.
- Target rotation removes target-owned evidence without removing imported
  evidence.
- Imported evidence expires, evicts, pins, removes, shuts down, and disappears
  on restart under the existing transient workspace rules.
- Browser and shared service tests prove source labeling, no target claims,
  duplicate imported trace-ID rejection, cancellation, capacity behavior, and
  direct Trace Explorer navigation.
- The Java/Go fixture corpus proves required marker production and consumption
  for observability-enabled and observability-disabled trace creation paths.
- PR 18 conformance later proves browser and MCP share the same imported handle,
  calculations, invalidation, pinning, TTL, and removal behavior.

## Out of scope

- Cross-version loading, migration, adapters, compatibility ranges, or legacy
  unmarked traces.
- Console-owned trace archive, history, durable catalog, restart adoption,
  recent-files state, or automatic reopening.
- Export from an installed local copy after upstream loss.
- Damaged or incomplete trace recovery, partial semantic views, raw-only
  quarantine, repair, or LLM orchestration.
- Checksums, signatures, encryption, compression, containers, sidecars, or
  authenticity/provenance claims.
- Filesystem path import, server directory scanning, remote upload, multi-file
  batches, drag-and-drop, or watched folders.
- Exported derived indexes, manifests, continuations, handles, or imported
  metadata.
- MCP protocol implementation, debugging-skill changes, or a separate MCP
  evidence lifecycle; PR 18 and PR 19 consume the shared result later.
