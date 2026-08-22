# MCP client compatibility evidence

Loomspan Console exposes stateless Streamable HTTP at exact `/mcp`, with MCP
`2026-07-28` and compatible `2025-11-25` negotiation. It requires an exact
loopback Host and one bearer key. The installed surface contains twelve
read-only tools and no custom resource templates. Runtime discovery advertises
six capability IDs, including
`loomspan.trace-inspection.v1` and
`loomspan.raw-artifact-inspection.v1`. Trace reads retain exact source-byte
offsets with a 1 KiB default and a 16 MiB (16,777,216-byte) shared per-call
maximum. Ordinary complete navigation results use a 32 KiB ceiling and default
range results use 48 KiB; pagination admits only complete items. Automated
MCP-over-HTTP tests cover exact UTF-8 and worst-case base64
framing at 1, 4, and 16 MiB, concurrent clients, and explicit rejection of the
32 and 64 MiB candidates; representative-client checks are post-implementation
compatibility observations.

Current checked-in full HTTP measurements include the JSON-RPC response envelope,
structured content, and the deterministic text fallback:

| Response class | Post-change bytes | Committed ceiling |
| --- | ---: | ---: |
| `tools/list` | 25,588 | 25,600 |
| Inventory page (byte-stopped) | 12,012 | 32,768 |
| COMPACT frame page (byte-stopped) | 21,241 | 32,768 |
| DETAILED frame page (byte-stopped) | 20,751 | 32,768 |
| Record-descriptor page (byte-stopped) | 20,077 | 32,768 |
| Inline-content page (byte-stopped) | 18,472 | 32,768 |
| Literal-search page (byte-stopped) | 20,624 | 32,768 |
| Default adversarial TEXT semantic range | 29,225 | 49,152 |
| Default adversarial BASE64 raw range | 26,177 | 49,152 |

The twelve-tool discovery snapshot therefore has 12 bytes of deliberate
headroom under the 25 KiB ceiling. Compact schemas name the complete active
orientation and activity coverage facts while full typed-result validation
continues to enforce the concrete Go result shapes.

The pre-change repository recorded only the complete 20,304-byte `tools/list`;
the other baseline classes were not measured before implementation and are not
fabricated here. No executable Codex or second supported host threshold was
available during this implementation, so the manual host rows remain `Not
run`; the committed ceilings are implementation safety bounds, not claims of
25% headroom below an unobserved host threshold.

The release also carries the byte-identical, client-neutral
`skills/loomspan/` package. Installation is a user-selected
copy or filesystem link into a local client's user/global skill location; it
does not auto-install or contain an endpoint or key. The canonical skill is
unversioned during unreleased development and does not negotiate a version with
the MCP server. Live use requires the existing protected MCP configuration.
Five named capabilities are required and raw
artifact inspection is optional. Skill-only, MCP-only, missing-required, and
missing-optional behavior is evaluated separately so protocol, capability,
target, authentication, evidence, and target-scope failures are not collapsed.

Automated release evidence consists of pinned official conformance scenarios
for initialization, tool listing, caching, and DNS-rebinding protection across
both protocol revisions; SDK black-box discovery, strict schema rejection,
structured/text results, domain errors, opaque continuations, and trace-ID-only
calls against the real Loomspan surface; native credential/lifecycle tests on Windows x86_64, Linux
x86_64, macOS arm64, and macOS x86_64; and browser/API tests. Official
fixture-specific scenarios for diagnostic tools, prompts, resources, sampling,
and elicitation are intentionally inapplicable and must not be made to pass by
adding production capabilities.

## Post-implementation client matrix

As post-implementation checks are performed, record the client version,
operating system, configuration scope, protocol observed when available, and
results. Never record the live key or an Authorization header.

Validation date for this change: **2026-08-21**. Platform for unexecuted rows:
Windows x86_64 development workstation. Replace “not run” entries when the
corresponding client becomes available, recording product/build version,
protocol observed, configuration mechanism, and concise results. Incomplete
GUI rows are not failures. The required headless agent matrix is tracked by the
evaluation summary and is not inferred from an unexecuted row.

| Client family | Skill/configuration and selected evidence | Result for this change |
| --- | --- | --- |
| Codex CLI | User/global skill plus protected authenticated Streamable HTTP; repeated cases and continuations | Not run; date/OS/product/model build, protocol, case IDs, and result links not yet recorded |
| Codex desktop and IDE extension | User/global skill discovery/activation plus native MCP configuration | Not run; executable local build observation unavailable |
| Claude Code | User/global skill plus protected MCP; failed, slow, unfamiliar-path, and adversarial repeated cases | Not run; date/OS/product/model build, protocol, case IDs, and result links not yet recorded |
| Antigravity local app, IDE, or CLI | User/global endpoint/header and skill; one workflow and continuation observation | Not run; client build/version not recorded; record when available |
| Cursor | Global endpoint/header and skill; one workflow and continuation observation | Not run; client build/version not recorded; record when available |
| Devin Desktop / Windsurf / Cascade | Global endpoint/header or protected file interpolation and skill; one representative workflow | Not run; executable local build observation unavailable |
| Local Devin CLI | Protected file or environment interpolation and skill; one representative workflow | Not run; executable local build observation unavailable |
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
