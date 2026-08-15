# Loomspan Console runtime package

This archive is self-contained for its named operating system and architecture. It needs no JVM, Node.js, npm, database, separate web server, or shared Loomspan application filesystem.

Run `loomspan-console --version` (`.\\loomspan-console.exe --version` on Windows) to verify the executable. Start with `loomspan-console --no-open-browser`; open the printed loopback pairing URL in a browser and connect to a supported Loomspan Spring Boot target with its application key. The Console supports only the target/API version coordinated with this package version.

Configuration defaults to the operating-system profile location and the disposable analysis workspace defaults to the operating-system application state/cache location. Use `--config FILE` and `--work-dir DIRECTORY` for isolated locations. Target keys and custom trust roots are process-local inputs: protect the key, use a verified `ca-bundle` when required, and never place credentials in URLs or workspace files.

MCP is disabled by default and is managed from the paired browser under Settings. Enabling it creates the protected `mcp-access-key` beside the profile configuration. That key persists across Console restarts, is accepted only at the exact loopback `/mcp` endpoint, and is separate from Target keys and browser credentials. Reveal it only for protected user/global client configuration. Regeneration invalidates every old client configuration; disabling removes the key after active MCP work drains.

On shutdown the current transient workspace is removed best-effort, but an enabled MCP key remains in the profile. To remove remaining state, first disable MCP in Settings when possible, stop the Console, then delete only the profile and marked workspace directories you deliberately selected. See the repository `loomspan-console/README.md` for exact locations, configuration, security boundaries, and troubleshooting.

Release archives are named `loomspan-console-VERSION-windows-x86_64.zip`, `loomspan-console-VERSION-linux-x86_64.tar.gz`, and `loomspan-console-VERSION-macos-arm64.tar.gz`. Verify a downloaded archive against `SHA256SUMS` with `sha256sum -c SHA256SUMS` on POSIX systems, or on PowerShell compare `(Get-FileHash -Algorithm SHA256 .\\ARCHIVE).Hash.ToLowerInvariant()` with its entry in `SHA256SUMS`.

Each archive also contains `skills/loomspan-runtime-debugging/`, with one
`SKILL.md` and five focused `references/*.md` files. Its package version 1.0.0
is independent distribution metadata. To opt in, copy or filesystem-link that
unchanged directory into a local client's user/global Agent Skill location.
There is no automatic installer or client-specific package. Configure the
loopback MCP endpoint and bearer header separately through the client's
protected mechanism; neither value belongs in the skill.

The skill needs runtime-status, skill-inspection, active-execution,
recent-activity, and trace-inspection capabilities. Raw-artifact inspection is
optional: without it parsed debugging continues, while exact storage/parser
forensics is unavailable. A missing required capability is not protocol or
target incompatibility. Without MCP the skill can provide only general
practice; MCP remains independently usable without the skill. Hosted clients
that cannot reach the local loopback listener are out of scope.
