# MCP client compatibility evidence

Loomspan Console exposes stateless Streamable HTTP at exact `/mcp`, with MCP
`2026-07-28` and compatible `2025-11-25` negotiation. It requires an exact
loopback Host and one bearer key. The installed PR 17 surface contains
`LOOMSPAN_get_runtime`, the five skill/active/recent inspection tools, the
scope-bound skill resource template, and the four runtime/inspection
capabilities documented in the Console README.

Automated release evidence consists of pinned official conformance scenarios
for initialization, tool listing, caching, and DNS-rebinding protection across
both protocol revisions; SDK black-box discovery, strict schema rejection,
structured/text results, domain errors, opaque continuation, and skill-resource
reads against the real Loomspan surface; native credential/lifecycle tests on Windows x86_64, Linux
x86_64, macOS arm64, and macOS x86_64; and browser/API tests. Official
fixture-specific scenarios for diagnostic tools, prompts, resources, sampling,
and elicitation are intentionally inapplicable and must not be made to pass by
adding production capabilities.

## Manual release matrix

For each release candidate, record the client version, operating system,
configuration scope, protocol observed when available, and pass/fail notes.
Never record the live key or an Authorization header.

Validation date for this change: **2026-08-13**. Platform for unexecuted rows:
Windows x86_64 development workstation. A release reviewer must replace each
applicable “not run” with the tested product/build version, protocol observed,
configuration mechanism, and concise pass/fail notes.

| Client family | Required PR 17 evidence | Result for this change |
| --- | --- | --- |
| Codex CLI | Authenticated connection; tool/resource-template discovery; structured and concise-text rendering; domain `isError`; Unicode skill resource; continuation round trip; 64-item result | **Fail**, Codex CLI 0.130.0, Windows x86_64, 2026-08-13, ephemeral inline `mcp_servers` URL plus bearer-token environment-variable configuration. Runtime and six-tool discovery passed after all tools advertised read-only annotations. Skill pagination, Unicode skill detail, active list/detail, a 64-item activity result, activity continuation to cursor 65, and page-size schema rejection passed. The client did not expose resource-template discovery/read to the model, and its missing-skill presentation did not expose the structured Loomspan `isError` plus text result. Protocol revision was not surfaced. No key was recorded. |
| Codex desktop and IDE extension | Same contract through their native MCP configuration and resource surfaces | Not run; client build/version not recorded; release evidence required |
| Claude Code | Same PR 17 contract plus key regeneration/reconnect lifecycle | Not run; client build/version not recorded; release evidence required |
| Antigravity local app, IDE, or CLI | User/global endpoint and header; complete PR 17 discovery/call/resource/continuation/64-item checks | Not run; client build/version not recorded; record when available |
| Cursor | Global endpoint/header; complete PR 17 discovery/call/resource/continuation/64-item checks | Not run; client build/version not recorded; record when available |
| Devin Desktop / Windsurf / Cascade | Global endpoint/header or protected file interpolation; complete PR 17 checks | Not run; client build/version not recorded; record when available |
| Local Devin CLI | Protected file or environment interpolation; complete PR 17 checks | Not run; client build/version not recorded; record when available |
| Hosted Codex or hosted Devin | Loopback reachability | Out of scope; hosted clients cannot reach the local listener |

No other named local client was installed or normally authenticated on the
Windows validation workstation. The successful representative-client
acceptance criterion therefore remains open; closing it requires access to a
client that exposes resource templates/reads and structured domain-error
results, or an approved ticket/plan amendment.

Use the endpoint displayed in **Settings > MCP Integration** and the client's
current protected or environment-backed bearer-header mechanism. Prefer
user/global configuration. Do not place the key in a repository file, URL,
command line, shell history, screenshot, or committed evidence. After each
test, disable MCP or remove the temporary profile. If any representative client
cannot consume a complete 64-item result, lower the one global MCP page maximum
and repeat every affected automated and manual check; do not introduce a
client-specific schema or fallback.
