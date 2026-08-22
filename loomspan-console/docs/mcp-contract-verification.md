# MCP contract verification

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
32 and 64 MiB candidates.

Checked-in tests measure complete HTTP responses, including the JSON-RPC
response envelope, structured content, and deterministic text fallback. They
enforce a 25 KiB ceiling for `tools/list`, a 32 KiB ceiling for ordinary
complete navigation results, and a 48 KiB ceiling for default range results.
The tests retain and report exact current measurements so that size drift below
those ceilings remains visible during review; this document records the stable
limits and methodology rather than duplicating those change-sensitive values.

Compact schemas name the complete active orientation and activity coverage
facts while full typed-result validation continues to enforce the concrete Go
result shapes.

The release also carries the byte-identical, client-neutral
`skills/loomspan/` package. Installation is a user-selected
copy or filesystem link into a local client's user/global skill location; it
does not auto-install or contain an endpoint or key. The canonical skill is
unversioned during unreleased development and does not negotiate a version with
the MCP server. Live use requires the existing protected MCP configuration.
Five named capabilities are required and raw artifact inspection is optional.
Automated tests keep protocol, capability, target, authentication, evidence,
and target-scope failures distinct.

Automated release evidence consists of pinned official conformance scenarios
for initialization, tool listing, caching, and DNS-rebinding protection across
both protocol revisions; SDK black-box discovery, strict schema rejection,
structured/text results, domain errors, opaque continuations, and trace-ID-only
calls against the real Loomspan surface; native credential/lifecycle tests on Windows x86_64, Linux
x86_64, macOS arm64, and macOS x86_64; and browser/API tests. Official
fixture-specific scenarios for diagnostic tools, prompts, resources, sampling,
and elicitation are intentionally inapplicable and must not be made to pass by
adding production capabilities.
