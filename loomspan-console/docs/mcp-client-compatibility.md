# MCP client compatibility evidence

Loomspan Console exposes stateless Streamable HTTP at exact `/mcp`, with MCP
`2026-07-28` and compatible `2025-11-25` negotiation. It requires an exact
loopback Host and one bearer key. PR 16 exposes only `LOOMSPAN_get_runtime` and
`loomspan.runtime-status.v1`.

Automated release evidence consists of pinned official conformance scenarios
for initialization, tool listing, caching, and DNS-rebinding protection across
both protocol revisions; SDK black-box discovery and calls against the real
Loomspan tool; native credential/lifecycle tests on Windows x86_64, Linux
x86_64, macOS arm64, and macOS x86_64; and browser/API tests. Official
fixture-specific scenarios for diagnostic tools, prompts, resources, sampling,
and elicitation are intentionally inapplicable and must not be made to pass by
adding production capabilities.

## Manual release matrix

For each release candidate, record the client version, operating system,
configuration scope, protocol observed when available, and pass/fail notes.
Never record the live key or an Authorization header.

| Client | Required evidence | Result for this change |
| --- | --- | --- |
| Codex | Connect, initialize, list/call runtime, regenerate, verify old key fails, reconnect with new key, disable, restart/reconnect | Not run locally; release evidence required |
| Claude Code | Same deep lifecycle as Codex | Not run locally; release evidence required |
| Antigravity | User-level endpoint/header, discovery, runtime call | Not run locally; record when available |
| Cursor | Global endpoint/header, discovery, runtime call | Not run locally; record when available |
| Windsurf/Cascade | Global endpoint/header, discovery, runtime call | Not run locally; record when available |
| Hosted Devin | Loopback reachability | Out of scope; hosted clients cannot reach the local listener |

Use the endpoint displayed in **Settings > MCP Integration** and the client's
current protected or environment-backed bearer-header mechanism. Prefer
user/global configuration. Do not place the key in a repository file, URL,
command line, shell history, screenshot, or committed evidence. After each
test, disable MCP or remove the temporary profile. A local Devin CLI may be
recorded as additive evidence only if its current MCP configuration genuinely
supports the required bearer header.
