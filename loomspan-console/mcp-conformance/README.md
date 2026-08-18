# MCP conformance harness

`go run ./internal/buildtool mcp-conformance` creates an isolated protected
profile and credential, starts the production stateless MCP adapter, and runs
the pinned official runner through a local credential-injecting proxy. The
proxy keeps the access key out of command arguments and runner output while
requests still traverse Loomspan's production authority, Origin, bearer,
admission, body-limit, SDK, and installed-tool layers.

The harness runs the protocol-generic scenarios applicable to the MCP foundation for both
supported revisions. Fixture-specific official scenarios that require
non-product test tools, resources, prompts, sampling, or elicitation are
intentionally excluded. Loomspan's real tool discovery and calling are covered
by the SDK and assembled HTTP integration suites across the complete twelve-tool,
zero-custom-resource surface, without an expected-failure baseline.
