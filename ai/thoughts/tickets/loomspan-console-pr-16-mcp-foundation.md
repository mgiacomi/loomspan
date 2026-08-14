# PR 16 — MCP Authentication and Lifecycle Foundation

## Status

Ready for detailed planning. Depends on PRs 09 and 15, which are completed.
The implementation baseline was revalidated on 2026-08-13 in
[`../research/2026-08-13-loomspan-console-pr-16-mcp-foundation.md`](../research/2026-08-13-loomspan-console-pr-16-mcp-foundation.md#decision-resolution-2026-08-13).
Those decisions supersede the older planning-time MCP details in the Phase 3
architecture document where they differ.

## Outcome

Establish Loomspan Console's independent local MCP security boundary on the
current stable official MCP stack, prove credential mutation and request-drain
lifecycle behavior, and expose authenticated runtime status with the complete
`loomspan.runtime-status.v1` capability.

## In scope

- Pin the official `github.com/modelcontextprotocol/go-sdk/mcp` module at
  `v1.7.0`, target MCP `2026-07-28`, and run stateless Streamable HTTP with
  compatibility for `2025-11-25` clients.
- Mount the exact `/mcp` route on the existing IPv4 loopback listener and
  advertise `http://127.0.0.1:<actual-port>/mcp`.
- Complete the lifecycle spike for admission freeze, active-request
  cancellation and drain, temporary SDK-session closure, key-generation
  publication, late-result suppression, reconnect, and shutdown. Do not add a
  persistent Loomspan MCP session registry.
- Add the single profile-owned MCP credential store and protected canonical
  sibling `mcp-access-key` file. Keep the live profile lock basename
  `.loomspan-console.lock`.
- Generate exactly `lsmcp_` plus 43 unpadded base64url characters from 32
  cryptographically random bytes, followed by one LF in the canonical file.
- Add paired browser enable, reveal, regenerate, and disable operations with
  atomic crash-safe Linux, macOS, and Windows mutation.
- Put Loomspan-owned exact-authority, supplied-Origin, and bearer-key middleware
  ahead of the SDK handler. Accept the current-port `127.0.0.1` and `localhost`
  authorities; do not accept `[::1]` without an IPv6 listener.
- Add `LOOMSPAN_get_runtime` with empty-object input, the existing serialized
  `consolecore.StatusSnapshot` under `status`, and a sorted `capabilities`
  array containing only `loomspan.runtime-status.v1`.
- Identify the MCP server as `loomspan-console` with the existing complete
  Loomspan product release string, and return both structured content and a
  deterministic concise text fallback.
- Keep strict YAML schema version 1 unchanged. MCP enablement is represented
  only by canonical key-file presence; PR 16 adds no `mcp` YAML section.

## Guardrails

- Browser, MCP, and upstream application credentials and authentication realms
  are non-interchangeable.
- Validate authority before authentication, SDK processing, or body reading.
- An absent `Origin` may support non-browser clients but weakens no other control.
- SDK types remain inside the thin MCP adapter.
- Stateless SDK sessions are request-scoped adapter mechanics, not shared or
  persistent application state.
- The negotiated MCP protocol version, endpoint, transport, authentication mode,
  and server identity are not duplicated in `LOOMSPAN_get_runtime` output.
- Never place the access key in YAML, a URL, repository/project configuration,
  logs, results, errors, shell-history examples, or automatic client-config
  mutation.
- Do not advertise a named Loomspan capability until every operation and semantic
  promise required by that capability is present.
- Do not advertise OAuth, sampling, elicitation, prompts, subscriptions, event
  replay, or an SDK-owned listener.

## Acceptance signals

- Enable, regenerate, and disable serialize through one profile-owned store.
  The canonical file is always absent or one complete valid key; malformed
  canonical state is never silently replaced.
- Linux uses exclusive no-replace creation, atomic rename-over, and file plus
  parent-directory durability; macOS receives a dedicated implementation using
  exclusive rename and `F_FULLFSYNC` where supported; Windows uses protected
  create-new files, `FlushFileBuffers`, and write-through move/replace behavior.
  Platform-native tests cover interruption outcomes and protected cleanup.
- Regeneration and disablement freeze new MCP admission, cancel and drain
  old-generation requests, close current temporary SDK sessions defensively,
  commit the filesystem mutation, publish the new generation, and only then
  reopen admission. No old-key result is emitted after commit, and browser or
  target state is unaffected.
- Process shutdown permanently freezes admission, drains MCP work before the
  shared bounded HTTP shutdown, and leaves a valid enabled key file in place.
  Restart preserves enabled key-file state but no MCP request/session state or
  upstream application credential.
- Exact `/mcp` routing, IPv4 loopback binding, current-port Host acceptance,
  supplied-Origin rejection, authentication-before-body behavior, disabled
  admission, and request bounds are covered by assembled HTTP tests.
- The official server conformance requirements for MCP `2025-11-25` and
  `2026-07-28` pass against the Streamable HTTP endpoint without baselining any
  required initialization, tool-listing, or tool-call failure.
- SDK in-memory and black-box tests prove tool discovery, structured output,
  text fallback, cancellation, reconnect, and shutdown.
- `LOOMSPAN_get_runtime` succeeds for no-target, disconnected,
  authentication-required, incompatible, and connected states because those
  are status facts rather than tool failures. Unknown input is rejected and an
  unexpected adapter failure is a safe unsuccessful `INTERNAL` result.
- Golden capability fixtures prove exact property and enum spellings, sorted
  capability order, structured/text agreement, a bounded response, and absence
  of secrets.
- Capability conformance proves PR 16 advertises
  `loomspan.runtime-status.v1` without prematurely advertising later runtime or
  trace families.
- Codex and Claude Code receive deep connection, key-rotation, disablement,
  shutdown, and reconnect smoke tests. Current local Antigravity, Cursor, and
  Windsurf/Cascade receive connection, bearer-header, discovery, and tool-call
  smoke tests where supported automation permits. Hosted clients that cannot
  reach loopback are reported out of transport scope rather than compatible.

## Detailed-planning focus

Define the adapter package seams, admission/request tracker, build-tagged
credential commit primitives, paired browser DTOs and UI states, exact middleware
ordering and disabled response, request/body bounds, deterministic text fallback,
conformance harness, and representative-client test procedure. Treat the
revalidated baseline above as fixed unless implementation evidence demonstrates
a material blocker and the ticket is deliberately revised.

## Out of scope

Runtime inspection tools beyond status bootstrap, trace tools, Agent Skill,
remote MCP, stdio, IPv6 listeners, stateful MCP operation, per-client keys or
identity, OAuth, automatic client configuration, and new YAML configuration.
