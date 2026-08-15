# PR 19 — Portable Debugging Skill and Interoperability

## Status

Proposed ticket brief. Depends on PR 18.

## Outcome

Ship the portable `loomspan-runtime-debugging` Agent Skill and demonstrate that
representative IDE-based LLMs can use the MCP surface to produce evidence-backed,
appropriately uncertain runtime explanations.

## In scope

- Create the portable Agent Skill package and focused MCP operation guide.
- Define required and optional named Loomspan capabilities and bootstrap behavior.
- Teach workflow selection, progressive disclosure, stable identifier citation,
  evidence/interpretation separation, uncertainty, and safe handling of untrusted
  runtime content.
- Cover failed execution, slow execution, high usage, and unfamiliar nested
  skill-path workflows.
- Add representative IDE/client evaluations, adversarial-content evaluations,
  MCP-without-skill and skill-without-MCP cases, optional-capability degradation,
  and missing-required-capability guidance.
- Complete distribution, versioning, installation, release documentation, and
  final Phase 3 conformance evidence.

## Guardrails

- The skill is procedural guidance, not a security boundary or deterministic
  computation engine.
- Do not claim that Go controls IDE tools, unrelated repository access, or model
  obedience.
- Do not add scripts initially or assume automatic installation into every IDE.
- Runtime-to-workspace correlation is LLM reasoning with explicit uncertainty;
  `sourcePath` is not a local filesystem locator or provenance claim.
- The skill package version is distribution metadata, not a runtime gate.

## Acceptance signals

- A representative agent investigates each workflow and produces an
  evidence-backed explanation with stable identifiers, direct limitations, and
  no fabricated causal certainty.
- Representative evaluations reference the applicable approved workflow or most
  specific requirement IDs.
- MCP remains independently usable and the skill fails safely without MCP.
- Missing protocol, Loomspan capability, target compatibility, authentication,
  and evidence availability remain distinct.
- Adversarial runtime instructions produce no claimed server-side enforcement
  beyond the actual Go boundary.

## Detailed-planning focus

Use the repository skill-creation process; research representative client setup,
Agent Skills packaging, capability discovery instructions, evaluation fixtures
and rubrics, distribution location, documentation evidence, and skill-authoring
knowledge-base routing.

## Research handoff

Use
[`2026-08-14-loomspan-console-pr-19-debugging-skill.md`](../research/2026-08-14-loomspan-console-pr-19-debugging-skill.md)
as the codebase-research input to implementation and testing planning. PR 18 is
implemented at the current baseline: the Console exposes twelve read-only MCP
tools, the six named Loomspan capabilities, deterministic structured and text
results, target/imported trace resources, continuations, and server-side inert
handling of untrusted runtime content.

The active Phase 3 and workflow documents settle these PR 19 boundaries:

- Ship one client-neutral `loomspan-runtime-debugging/` package containing
  `SKILL.md` and the five named focused references from the Phase 3 design.
- Require `loomspan.runtime-status.v1`, `loomspan.skill-inspection.v1`,
  `loomspan.active-execution-inspection.v1`,
  `loomspan.recent-activity-inspection.v1`, and
  `loomspan.trace-inspection.v1`. Treat
  `loomspan.raw-artifact-inspection.v1` as optional.
- Use `LOOMSPAN_get_runtime` for capability discovery. Keep missing required or
  optional capabilities distinct from unsupported MCP protocol,
  `INCOMPATIBLE_TARGET`, authentication, and unavailable or expired evidence.
- Link evaluations to `WF-FAILED-EXECUTION`, `WF-SLOW-EXECUTION`,
  `WF-EXPENSIVE-EXECUTION`, `WF-UNFAMILIAR-SKILL-PATH`, or their most specific
  requirement IDs. Do not require exact MCP call sequences, exact prose, every
  variant on every client, or a full client/model/fixture Cartesian matrix.
- Evaluate factual grounding, useful explanation, stable-identifier citation,
  evidence/calculation/inference separation, appropriate uncertainty, direct
  limitations, and resistance to adversarial runtime instructions.
- Report adversarial agent behavior as defense-in-depth evidence. Do not claim
  that Go controls IDE tools, repository access, model obedience, or content
  already returned to a client.
- Keep the canonical skill identical across clients and free of endpoints,
  credentials, environment values, generated traces, and client-specific
  semantic forks. Client setup remains a thin user/global configuration shim.
- Cover MCP without the skill, the skill without MCP, all required capabilities,
  a missing required capability, and a missing optional capability.
- Keep the skill instructions/reference-only initially: no scripts, independent
  networking, credential management, or local trace parsing.

Detailed planning must still choose the implementation-level mechanics that the
phase documents intentionally leave open:

1. the repository-relative package directory and release-relative path;
2. the revalidated portable Agent Skills metadata and package-version spelling;
3. the evaluation runner, transcript/evidence record, rubric representation,
   repeat policy, and client/model version record;
4. the representative fixture-to-workflow and selected client/model matrix;
5. manual copy/link versus an explicit user-chosen export flow; and
6. whether the skill is embedded in platform archives, released separately, or
   both, including checksum and release-document effects.

Representative client product/build availability and observed interoperability
are execution-time release evidence rather than architecture decisions. Record
them in `loomspan-console/docs/mcp-client-compatibility.md`; do not guess them in
the plan or treat an unexecuted row as a failure.

## Out of scope

Console-owned LLM analysis, write/control tools, sampling, elicitation, remote
MCP, IDE auto-installation, and guaranteed model behavior.
