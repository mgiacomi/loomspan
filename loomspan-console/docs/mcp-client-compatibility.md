# MCP client compatibility evidence

Loomspan Console exposes stateless Streamable HTTP at exact `/mcp`, with MCP
`2026-07-28` and compatible `2025-11-25` negotiation. It requires an exact
loopback Host and one bearer key. The installed PR 18 surface contains twelve
read-only tools, the skill resource plus six target/imported trace resource
templates. Runtime discovery advertises six capability IDs, including
`loomspan.trace-inspection.v1` and
`loomspan.raw-artifact-inspection.v1`. Trace reads retain exact source-byte
offsets with a 64 KiB default and a 16 MiB (16,777,216-byte) shared per-call
maximum. Automated MCP-over-HTTP tests cover exact UTF-8 and worst-case base64
framing at 1, 4, and 16 MiB, concurrent clients, and explicit rejection of the
32 and 64 MiB candidates; representative-client checks are post-implementation
compatibility observations.

Automated release evidence consists of pinned official conformance scenarios
for initialization, tool listing, caching, and DNS-rebinding protection across
both protocol revisions; SDK black-box discovery, strict schema rejection,
structured/text results, domain errors, opaque continuation, and skill-resource
reads against the real Loomspan surface; native credential/lifecycle tests on Windows x86_64, Linux
x86_64, macOS arm64, and macOS x86_64; and browser/API tests. Official
fixture-specific scenarios for diagnostic tools, prompts, resources, sampling,
and elicitation are intentionally inapplicable and must not be made to pass by
adding production capabilities.

## Post-implementation client matrix

As post-implementation checks are performed, record the client version,
operating system, configuration scope, protocol observed when available, and
results. Never record the live key or an Authorization header.

Validation date for this change: **2026-08-14**. Platform for unexecuted rows:
Windows x86_64 development workstation. Replace “not run” entries when the
corresponding client becomes available, recording product/build version,
protocol observed, configuration mechanism, and concise results. Incomplete
rows do not block implementation completion, merge, or release.

| Client family | Suggested PR 18 observation | Result for this change |
| --- | --- | --- |
| Codex CLI | Twelve-tool/schema discovery; structured/text and domain errors; target and target-free import flows; resource reads when exposed; 64-item continuation; exact UTF-8/base64 traversal at the configured range ceiling | Not run; post-implementation observation |
| Codex desktop and IDE extension | Same contract through native MCP configuration and resource surfaces | Not run; post-implementation observation |
| Claude Code | Complete PR 18 contract plus key regeneration/reconnect lifecycle | Not run; post-implementation observation |
| Antigravity local app, IDE, or CLI | User/global endpoint and header; complete PR 18 discovery/call/resource/continuation/range checks | Not run; client build/version not recorded; record when available |
| Cursor | Global endpoint/header; complete PR 18 discovery/call/resource/continuation/range checks | Not run; client build/version not recorded; record when available |
| Devin Desktop / Windsurf / Cascade | Global endpoint/header or protected file interpolation; complete PR 18 checks | Not run; post-implementation observation |
| Local Devin CLI | Protected file or environment interpolation; complete PR 18 checks | Not run; post-implementation observation |
| Hosted Codex or hosted Devin | Loopback reachability | Out of scope; hosted clients cannot reach the local listener |

The table intentionally separates the completed automated contract from later
manual client observations. An untested row is not a failed or blocking row;
do not infer an observation from the adapter, HTTP, exactness, deadline,
cancellation, concurrency, or allocation suites alone.

Use the endpoint displayed in **Settings > MCP Integration** and the client's
current protected or environment-backed bearer-header mechanism. Prefer
user/global configuration. Do not place the key in a repository file, URL,
command line, shell history, screenshot, or committed evidence. After each
test, disable MCP or remove the temporary profile. Record incompatibilities for
separate follow-up design; do not silently alter the completed global contract
or introduce a client-specific schema or fallback.
