---
date: 2026-08-14T18:29:40-07:00
researcher: Codex (GPT-5)
git_commit: 60c5c1783106f07488ada6fc664a58a4f75a62b4
branch: main
repository: loomspan
topic: "PR 19 — Portable Debugging Skill and Interoperability"
tags: [research, codebase, loomspan-console, mcp, agent-skills, runtime-debugging, evaluations]
status: complete
last_updated: 2026-08-14
last_updated_by: Codex (GPT-5)
last_updated_note: "Added follow-up research resolving open-question status against the active phase documents"
---

# Research: PR 19 — Portable Debugging Skill and Interoperability

**Date**: 2026-08-14 18:29:40 PDT
**Researcher**: Codex (GPT-5)
**Git Commit**: 60c5c1783106f07488ada6fc664a58a4f75a62b4
**Branch**: main
**Repository**: loomspan

## Research Question

Research the current codebase for the proposed PR 19 ticket, which calls for a portable `loomspan-runtime-debugging` Agent Skill, focused MCP operation guidance, named-capability bootstrap behavior, workflow and adversarial evaluations, representative local IDE/client evidence, and distribution/versioning/install/release documentation.

## Summary

The repository currently contains the complete PR 18 read-only MCP evidence surface on which PR 19 depends, but it does not yet contain the portable Agent Skill package or an agent-evaluation harness. The current MCP server exposes twelve read-only tools, one registered-skill resource template, and six target/imported trace resource templates. `LOOMSPAN_get_runtime` returns the six named Loomspan capability IDs independently from target and evidence state. The adapter provides strict schemas, structured success/error arms, deterministic text fallbacks, stable identifiers, finite continuations, and explicit untrusted-data descriptions.

The canonical current investigation catalog consists of four workflows: failed execution (`WF-FAILED-EXECUTION` / `WF-FE-*`), slow execution (`WF-SLOW-EXECUTION` / `WF-SE-*`), high usage (`WF-EXPENSIVE-EXECUTION` / `WF-UE-*`), and unfamiliar nested skill path (`WF-UNFAMILIAR-SKILL-PATH` / `WF-SP-*`). Existing Java/Go fixtures already cover important mechanical evidence for failed execution, usage, and nested paths. Slow-execution agent evaluation can consume the existing active-execution and recent-activity fixture families. No current tests run a representative IDE agent, score a prose explanation, load a portable `SKILL.md`, or exercise skill-without-MCP/MCP-without-skill behavior.

The Phase 3 design contains the current intended skill structure and behavior: one canonical package with `SKILL.md` plus five progressively loaded references; the first five Loomspan capabilities required and raw-artifact inspection optional; evidence, calculation, repository context, and inference kept distinct; stable identifier citation; explicit uncertainty; and runtime text treated as untrusted evidence rather than instructions. It also names the representative local client families and separates server-enforced inertness from skill guidance and residual IDE/model behavior.

The current release archive contains only `LICENSE`, `README.md`, and the platform executable. The repository has no `SKILL.md`, no Agent Skill distribution directory, no package-version file for the debugging skill, no installation examples for skill directories, and no client observations recorded beyond “not run.” These are current-state absences, while the PR 19 ticket and Phase 3 design describe the intended coverage.

## Detailed Findings

### 1. PR 18 dependency and current implementation baseline

PR 18 is complete in Git history: commit `2f5b750` is `PR 18 — Trace-Inspection MCP Surface`, and current `HEAD` is its cleanup commit `60c5c17`. The removed PR 18 ticket described the same target/imported evidence ownership, trace tools, capability advertisement, semantic fixtures, continuations, and raw-range behavior now present in the live code.

The MCP adapter is assembled as a thin peer over shared Go services. `ServerOptions` receives status, target, observability, live, artifact, trace-analysis, and trace-inventory services; the adapter registers runtime, skill, execution, activity, and trace tools and the skill/trace resources (`loomspan-console/internal/mcpadapter/server.go:26-69`). The server uses the official MCP SDK as stateless Streamable HTTP with JSON responses, a 1 MiB request-body ceiling, and propagated cancellation (`loomspan-console/internal/mcpadapter/server.go:58-69`).

The installed tools are:

| Capability | Required tools in the current server |
| --- | --- |
| `loomspan.runtime-status.v1` | `LOOMSPAN_get_runtime` |
| `loomspan.skill-inspection.v1` | `LOOMSPAN_list_skills`, `LOOMSPAN_get_skill` |
| `loomspan.active-execution-inspection.v1` | `LOOMSPAN_list_executions`, `LOOMSPAN_get_execution` |
| `loomspan.recent-activity-inspection.v1` | `LOOMSPAN_get_execution_activity` |
| `loomspan.trace-inspection.v1` | `LOOMSPAN_list_traces`, `LOOMSPAN_get_trace`, `LOOMSPAN_query_trace_frames`, `LOOMSPAN_query_trace_records`, `LOOMSPAN_read_trace_payload` |
| `loomspan.raw-artifact-inspection.v1` | `LOOMSPAN_read_trace_artifact` |

The exact capability descriptors and their required tools live in `loomspan-console/internal/mcpadapter/capabilities.go:9-30`. Runtime assembly always includes runtime status and advertises another capability only when its entire tool family is present (`capabilities.go:32-55`). Trace and raw-artifact capabilities additionally have a reviewed semantic-fixture manifest; test-time conformance omits a capability if a required semantic fixture is absent (`capabilities.go:17-29,58-75`; `internal/mcpadapter/contracts/trace-capabilities.json:1-38`).

`LOOMSPAN_get_runtime` is side-effect-free and does not contact the target. It returns `capabilities` alongside the shared `consolecore.StatusSnapshot` (`runtime.go:13-47`). The status keeps target selection, connection, authentication, Java/Go compatibility, runtime identity, `instanceId`, live-monitoring state, `targetScopeId`, and `observedAt` as independent facts (`consolecore/status.go:8-55`). This supplies the bootstrap distinction required by PR 19: installed capability, current status, and an individual operation result are separate layers.

### 2. MCP evidence contract available to the skill

Every non-runtime inspection tool returns one `result` or one `error` envelope arm. Domain failures carry a stable `code`, bounded safe `message`, optional `targetScopeId`, and permitted details, set MCP `isError`, and retain a text fallback (`internal/mcpadapter/contracts.go:20-30,143-163`). Page size is constrained to 1–64 and nonblank identifiers receive schema constraints (`contracts.go:242-267`). All tools are annotated read-only, non-destructive, idempotent, and closed-world (`contracts.go:135-141`).

The domain error catalog explicitly distinguishes target authentication, access, availability, target compatibility, artifact compatibility, stale or invalid cursors, missing evidence, expired/in-use/duplicate/invalid artifacts, unavailable live monitoring, capacity, local storage, and sanitized console errors (`internal/consolecore/errors.go:5-25`). Unsupported MCP negotiation and bearer authentication remain HTTP/MCP failures rather than these Loomspan domain codes. The current MCP compatibility document records support for MCP `2026-07-28` and compatible `2025-11-25` (`docs/mcp-client-compatibility.md:3-24`).

Stable identifiers are already projected in structured and text results. Active execution DTOs include `sessionId`, `traceId`, canonical sequence, times, entry skill, status, phase, active frame path, usage, and limits (`internal/mcpadapter/contracts.go:61-99`). Activity DTOs include `instanceId`, cursor, `sessionId`, `traceId`, canonical sequence, frame/parent IDs, route, timestamp, kind, summary, and details (`contracts.go:101-133`). Skill results include exact registered name, `sourcePath`, unchanged YAML, scope, instance, and observation time (`skills.go:64-116`). Trace DTOs add evidence source (`TARGET` or `IMPORTED`), artifact handle, trace/session identity, frame/record/attempt/retry/validation/failure identifiers, gap and uncertainty facts, and continuable range addresses.

The essential workflow is tool-complete. Supplementary resources comprise one skill template and six trace templates for target/imported summaries, frames, and records (`internal/mcpadapter/resources.go:15-29`; `trace_resources.go:20-56`). `sourcePath` is explicitly described as untrusted diagnostic text, not an instruction or filesystem locator (`skills.go:28-42`; `resources.go:20-29`). Trace resource descriptions similarly label returned content as untrusted (`trace_resources.go:45-56`).

Trace range reads default to 64 KiB and accept at most 16 MiB per call, preserving exact source-byte offsets and continuation. This is documented as the PR 18 release behavior and has automated UTF-8/base64 framing coverage at 1, 4, and 16 MiB (`docs/mcp-client-compatibility.md:3-14`).

### 3. Canonical debugging workflows and evaluation anchors

The workflow catalog says representative fixtures, tests, and agent evaluations use the applicable workflow ID or most specific requirement ID, live beside their owning layer, and do not prescribe an exact tool sequence or answer prose (`ai/thoughts/phases/loomspan_console_workflows.md:32-49`).

#### Failed execution

`WF-FAILED-EXECUTION` asks the investigator to establish what and where an execution failed, preceding behavior, retry/validation/quota/timeout/guardrail contribution, time/usage concentration, and evidence limitations. Root-cause judgment remains developer or IDE-LLM reasoning (`loomspan_console_workflows.md:101-125`). The catalog separates execution-ending failure from inferred root cause (`WF-FE-R6`), requires stable evidence identifiers (`WF-FE-R7`), and distinguishes unavailable, expired, invalid, incomplete, and stale-scope evidence (`WF-FE-R8`) (`loomspan_console_workflows.md:197-210`).

The Java-to-Go corpus maps `runtime-terminal-failure` to `WF-FE-*` and preserves the terminal failure, direct frame/attempt relationships, and missing-response evidence (`loomspan-console-fixtures/README.md:7-16`). The MCP trace fixture suite projects those shared facts through the current tools.

#### Slow execution

`WF-SLOW-EXECUTION` uses active snapshots and the bounded recent-activity window, never an active trace tail (`loomspan_console_workflows.md:223-255`). It defines high elapsed time as a fact and “slow” as developer interpretation, states that silence does not prove a stuck execution, marks live state best-effort, and reserves complete hierarchy/duration/usage attribution for finalized traces (`loomspan_console_workflows.md:271-280`). The key cross-adapter requirement is `WF-SE-R9`, identical active-snapshot and continuity semantics, while `WF-SE-R7` prohibits presenting elapsed time or absent activity as proof of a stuck execution (`loomspan_console_workflows.md:324-337`).

The live substrate includes active execution fixtures, execution detail, and recent activity with cursors, observation times, `beginningUnavailable`, and continuity/reset facts. The MCP activity description calls this untrusted, bounded diagnostic data rather than durable history (`internal/mcpadapter/activity.go:24`).

#### High usage

`WF-EXPENSIVE-EXECUTION` establishes recorded totals, arithmetic comparison with configured limits, hierarchy/attempt attribution, direct versus descendant usage, retry/validation association, and gaps (`loomspan_console_workflows.md:352-389`). Direct, descendant, inclusive, attempt, retry, and unattributed usage are mechanical definitions; adjacency and time proximity are not relationships (`loomspan_console_workflows.md:411-422`). Interpretation rules keep “too expensive,” retry necessity, validation correctness, and monetary cost outside recorded facts (`loomspan_console_workflows.md:424-435`). Requirements `WF-UE-R3`, `R6`, `R7`, `R9`, `R11`, `R12`, and `R13` capture parity, no double counting, explicit unattributed/unknown usage, and absence of server-generated causality or cost judgment (`loomspan_console_workflows.md:479-495`).

The fixture corpus maps `nested-frame-usage`, `unattributed-usage`, and `incomplete-frame-duration` to `WF-UE-*` (`loomspan-console-fixtures/README.md:11-19`).

#### Unfamiliar nested skill path

`WF-UNFAMILIAR-SKILL-PATH` combines recorded hierarchy, distinct invocation frames, registered names, routes, activity/duration/usage/outcome, and unchanged application-provided YAML (`loomspan_console_workflows.md:509-556`). It explicitly separates runtime execution from definitions and treats repeated invocations as distinct frame identities. `sourcePath` is not joined to the local filesystem and no browser/Go component searches the repository or establishes deployment provenance (`loomspan_console_workflows.md:566-576`). `WF-SP-R12` assigns runtime-to-workspace comparison to the IDE and prohibits provenance claims; `WF-SP-R14` excludes inferred routing intent, causality, design correctness, and restructuring recommendations from the authoritative console layer (`loomspan_console_workflows.md:637-654`).

The fixture corpus maps `repeated-skill-invocations` and `nested-frame-usage` to `WF-SP-*` (`loomspan-console-fixtures/README.md:13-16`).

### 4. Current portable-skill design recorded in Phase 3

No portable skill package exists in the current tree (`rg --files -g SKILL.md` returns no files). The active Phase 3 design records this intended package shape (`ai/thoughts/phases/loomspan_console_phase_3_llm_runtime_inspector.md:638-653`):

```text
loomspan-runtime-debugging/
├── SKILL.md
└── references/
    ├── runtime-model.md
    ├── debugging-playbooks.md
    ├── mcp-tool-guide.md
    ├── evidence-and-confidence.md
    └── common-failure-patterns.md
```

The intended activation scope includes debugging an execution or trace, inspecting runtime status, understanding a skill path, explaining failure/latency/usage/retry/validation behavior, and using Console MCP for diagnosis (`phase_3_llm_runtime_inspector.md:655-665`). The responsibilities are progressive but not a fixed decision tree: establish the entity, start with useful structural summaries, narrow via frames/records, retrieve payloads when needed, compare runtime evidence to workspace candidates, and stop when the evidence answers the question or cannot support a stronger conclusion (`phase_3_llm_runtime_inspector.md:667-686`).

The design declares the first five capability IDs required for essential workflows and `loomspan.raw-artifact-inspection.v1` optional (`phase_3_llm_runtime_inspector.md:867-907`). Missing required capability guidance identifies the exact capability and does not call it target incompatibility. Missing optional raw inspection reduces storage/parser forensics while parsed summary/frame/record/payload debugging remains available. Skill package version is distribution metadata and is neither compared by Go nor used as a runtime compatibility range (`phase_3_llm_runtime_inspector.md:850-865`).

The intended skill is instructions plus focused references only. It has no scripts, independent networking, credential management, or local trace parsing; MCP is its runtime data path (`phase_3_llm_runtime_inspector.md:703-715`). MCP alone remains usable from its schemas/results/errors, skill alone explains practice while directly reporting that live inspection requires MCP, and their combination is the intended experience.

### 5. Evidence, uncertainty, and runtime-to-workspace rules

The Phase 3 evidence contract separates:

- recorded runtime evidence;
- documented Loomspan behavior;
- deterministic calculations supplied by shared Go services; and
- inference requiring repository or developer context.

It calls for limitations when evidence is truncated, unavailable, expired, or provisional and cites `targetScopeId`, `instanceId`, `sessionId`, `traceId`, frame/parent IDs, route, skill name, record sequence/range, and observation time (`phase_3_llm_runtime_inspector.md:595-619`). For live executions, conclusions remain provisional and cite observation time/latest sequence (`phase_3_llm_runtime_inspector.md:621-634,686`).

The application-returned YAML is authoritative only as the running application's supplied skill representation. `sourcePath` and mapping identifiers are search hints for candidate workspace YAML or Java code. A difference is development context or a possible change target, not proof of the deployed revision (`phase_3_llm_runtime_inspector.md:595-603`).

The existing repository `ai/skill-authoring/` knowledge base is about authoring Loomspan YAML skill trees, not packaging portable Agent Skills. Its routing file directs debugging questions to `traces-and-debugging.md` and ambiguity to `source-verification.md` (`ai/skill-authoring/README.md:18-43`). It establishes LLM-first progressive disclosure, focused documents, compact exact fields, explicit unknowns, and stable anchors (`README.md:76-97`). Its current coverage table identifies traces/debugging as source-verified but testing skill trees as not yet documented (`README.md:45-64`). These documents can supply Loomspan runtime semantics, while the portable Agent Skill package format and client installation mechanics are not defined there.

### 6. Untrusted runtime content and adversarial evidence

The current Go boundary treats skill YAML, activity details, trace content, payloads, metadata, paths, and errors as data. Tool/resource descriptions label content untrusted, schemas accept only explicit operation inputs, and the server exposes no repository-browsing operation. A semantic fixture places an MCP-looking request, a network URL, a PowerShell command, a filesystem marker, and a bearer-key phrase into raw trace content; the test asserts exact inert return and verifies that no network request, file creation, credential generation change, target operation, or extra service call occurred (`internal/mcpadapter/trace_semantic_fixtures_test.go:870-946`). This is the existing server-side enforcement evidence.

The Phase 3 design assigns a separate behavior to the Agent Skill: disregard embedded tool/command/URL/credential/target/investigation override requests, use non-MCP IDE tools only when relevant to the developer's explicit question and normal authorization, avoid unrelated repository disclosure, keep evidence/context/calculation/inference distinct, and stop or state uncertainty when safe evidence is insufficient (`phase_3_llm_runtime_inspector.md:746-789`). It explicitly classifies this as defense-in-depth evaluation guidance rather than Go enforcement. Residual IDE/model tool use and model obedience remain outside Go's authority (`phase_3_llm_runtime_inspector.md:791-795`).

### 7. Evaluation and client-interoperability state

The current tree has no agent-evaluation runner, rubric, recorded model transcript format, or portable-skill loading test. Searches for evaluation/rubric terminology find only the PR 19 ticket and Phase 3/workflow requirements. Existing tests cover lower layers: MCP conformance, strict schemas, structured/text responses, errors, continuations, resources, target/imported flows, semantic parity, exact ranges, lifecycle, cancellation, concurrent clients, and server-side inertness.

The conformance harness starts the production stateless MCP adapter with an isolated protected credential and runs the pinned official runner through a local credential-injecting proxy, keeping the key out of arguments and output (`loomspan-console/mcp-conformance/README.md:1-15`). Its pinned dependency is an exact MCP conformance Git commit (`mcp-conformance/package.json:1-7`).

The representative client matrix names Codex CLI/Desktop/IDE, Claude Code, Antigravity, Cursor, Devin Desktop/Windsurf/Cascade, and local Devin CLI. Every local-client row is currently “Not run”; hosted clients are out of scope because they cannot reach loopback (`docs/mcp-client-compatibility.md:26-52`). The evidence procedure calls for product/build version, OS, configuration scope, observed protocol where available, and results, without recording keys or Authorization headers (`docs/mcp-client-compatibility.md:26-60`).

The Phase 3 evaluation model does not require every workflow variant on every client and does not prescribe exact calls or prose. It places protocol/security/service assertions below clients and uses a small representative client set for interoperability plus selected agent evaluations for factual grounding, useful diagnosis, stable identifiers, uncertainty, and adversarial resistance (`phase_3_llm_runtime_inspector.md:383-391`).

### 8. Distribution, versioning, installation, and release state

Current release packaging is deterministic and platform-specific, but the archive input list contains only `LICENSE`, the runtime `README.md`, and the executable (`loomspan-console/internal/buildtool/package.go:31-52,68-118`). Tests assert exactly those three entries (`package_test.go:40-88`), and smoke extraction expects only those entries. The current release README describes executable startup, profiles, MCP keys, archive names, and checksums, but no portable skill installation (`loomspan-console/release/README.md:1-15`).

The Phase 3 design says the canonical skill is version-controlled and distributed with Console source or release materials, with manual copy/link or a user-chosen export destination as possible workflows; it prohibits silent IDE configuration changes or automatic installation and excludes credentials, machine URLs, and trace data from the skill (`phase_3_llm_runtime_inspector.md:725-736`). The current tree does not select between those distribution shapes and contains no skill version metadata.

### 9. Contract and compatibility classification

#### Application API

No current PR 19 implementation exists and no relevant Java type in `com.lokiscale.loomspan.api` is changed. The closed supported Java API allowlist therefore has no current delta. The Agent Skill is an IDE/distribution artifact rather than a Java Application API.

#### Supported SPI

The repository declares no supported Java SPI or internal bean-replacement surface. No current PR 19 artifact introduces one. The MCP adapter's Go interfaces and constructors are internal assembly seams, not an application SPI (`internal/mcpadapter/server.go:21-52`).

#### Configuration and manifest contracts

The application YAML skill format remains the existing configuration/manifest contract and is transported unchanged for inspection. The proposed portable `SKILL.md` would be a separate Agent Skill distribution manifest, but no such manifest currently exists. Console has no MCP YAML setting; MCP enablement is the protected key file and browser-managed state described in the current Console README. Client MCP and skill-directory configuration is client-owned and currently documented only as a future/manual compatibility activity.

#### Persisted or serialized contracts

MCP negotiation is governed by the standard protocol, while named Loomspan capability IDs and tool names are the deliberate Go-to-client semantic discovery surface. Java-to-Go REST/SSE, acquisition, problem meanings, and consumed NDJSON semantics remain an exact coordinated-release boundary protected by `consoleCompatibilityVersion`, fixtures, and Java/Go tests. PR 19 currently adds no producer or consumer to those formats.

#### Ephemeral diagnostic formats

Execution traces, parsed indexes, handles, continuations, imported catalogs, activity windows, and MCP diagnostic projections are current-process/current-release diagnostic evidence. Complete raw trace files are portable only to an exact matching Console compatibility version. The fixture README explicitly classifies trace format as ephemeral rather than durable application API (`loomspan-console-fixtures/README.md:1-5,67-85`). The IDE explanation generated with the skill is interpretive output, not a new authoritative runtime or trace format.

#### Internal or accidentally exposed implementation

Go adapter DTOs, service interfaces, constructors, continuation encodings, artifact internals, resource implementation, build tooling, and evaluation harness internals are implementation details. Their current behavior is evidence for PR 19 planning but does not make them Java Application API or Supported SPI.

### 10. Protected protocol consumers and coordinated fixtures

The protected Java-to-Go consumers remain:

- the Go application client for REST, SSE, catalog, problem, and artifact download;
- shared Go observability/live/artifact/trace-analysis services;
- the browser adapter and React UI;
- the MCP adapter;
- the Java-generated `loomspan-console-fixtures` corpus and expected semantic projections; and
- current-release import/open behavior for exact-compatible NDJSON.

Observable semantics relevant to PR 19 include exact capability names, tool names, status dimensions, domain error meanings, `TARGET` versus `IMPORTED` evidence source, stable identifiers, current-scope identity, continuity/reset/gap facts, calculated duration/usage relationships, unchanged YAML, descriptive-only `sourcePath`, range offsets, truncation/continuation, and artifact lifecycle. Existing parity and fixture tests already protect those semantics. A documentation-and-evaluation-only skill can consume them without changing Java-to-Go wire formats; any later wire/schema change would involve the producer, Go consumers, fixtures, tests, and documentation in the same coordinated release.

## Code References

- `loomspan-console/internal/mcpadapter/server.go:26-69` — MCP server assembly over shared services.
- `loomspan-console/internal/mcpadapter/runtime.go:13-73` — bootstrap/status tool and text fallback.
- `loomspan-console/internal/mcpadapter/capabilities.go:9-75` — capability catalog, required tools, and semantic fixture gates.
- `loomspan-console/internal/mcpadapter/contracts.go:20-30,135-163,242-267` — result/error envelopes, read-only annotations, and schema bounds.
- `loomspan-console/internal/mcpadapter/skills.go:28-42,91-149` — exact YAML retrieval and untrusted `sourcePath` semantics.
- `loomspan-console/internal/mcpadapter/resources.go:15-29` — registered-skill resource template.
- `loomspan-console/internal/mcpadapter/trace_resources.go:20-56` — target/imported trace resource templates.
- `loomspan-console/internal/mcpadapter/trace_semantic_fixtures_test.go:870-946` — inert adversarial raw-content server fixture.
- `loomspan-console/internal/consolecore/status.go:8-55` — independent runtime status dimensions.
- `loomspan-console/internal/consolecore/errors.go:5-36` — stable domain codes and safe details.
- `loomspan-console/internal/buildtool/package.go:31-52,68-118` — current three-file release archive.
- `loomspan-console/internal/buildtool/package_test.go:40-88` — exact current archive-content assertion.
- `loomspan-console/docs/mcp-client-compatibility.md:1-60` — implemented MCP baseline and unexecuted client matrix.
- `loomspan-console/mcp-conformance/README.md:1-15` — official conformance harness boundary.
- `loomspan-console-fixtures/README.md:1-26` — workflow-linked current-release trace semantics.
- `ai/thoughts/phases/loomspan_console_phase_3_llm_runtime_inspector.md:595-907` — active skill, evidence, security, compatibility, and distribution design.
- `ai/thoughts/phases/loomspan_console_workflows.md:101-654` — canonical workflow and requirement IDs.
- `ai/skill-authoring/README.md:18-64,76-106` — current authoring knowledge-base routing and LLM-first content conventions.

## Architecture Documentation

The current investigation path is:

```text
developer question
    -> local IDE agent
        -> proposed portable Agent Skill (not yet present)
        -> current Loomspan MCP adapter
            -> shared Go status / observability / live / artifact / trace services
                -> selected Java application through coordinated REST/SSE/artifact protocols
```

The MCP adapter does not own an LLM, diagnosis engine, repository browser, independent trace parser, or separate evidence cache. Browser and MCP are peer adapters over common Go calculations and lifecycle. The proposed skill owns procedure and interpretation guidance; the IDE model owns contextual reasoning; Go owns authenticated, bounded, deterministic evidence retrieval.

## Historical Context (from `ai/thoughts/` and Git history)

- `ai/thoughts/phases/loomspan_console_phase_3_llm_runtime_inspector.md` — active architecture direction for the matched MCP-plus-skill product, capability catalog, uncertainty contract, distribution, client families, and threat-boundary split.
- `ai/thoughts/phases/loomspan_console_workflows.md` — canonical workflow catalog and stable requirement IDs.
- `ai/thoughts/phases/2026-08-12-loomspan-active-roadmap.md` — orders PR 19 after PRs 16–18 and identifies PR 18 as the trace-inspection dependency.
- Git commit `2f5b750`, deleted path `ai/thoughts/tickets/loomspan-console-pr-18-mcp-trace-inspection.md` — historical PR 18 handoff describing the capability manifest, trace operations, semantic fixture set, and post-implementation client observations that now exist in current code/docs.
- `ai/skill-authoring/` — current-checkout Loomspan YAML skill-tree knowledge base. It provides runtime/trace semantics and LLM-first documentation conventions but is not a portable Agent Skills packaging guide.

## Related Research

- Git commit `2f5b750`, deleted path `ai/thoughts/research/2026-08-14-loomspan-console-pr-18-mcp-trace-inspection.md` — historical research for the implemented trace-inspection surface.
- No other research document is present in the current `ai/thoughts/research/` tree at this commit.

## Open Questions

The following items are not established by the current tree and remain implementation-planning inputs in the PR 19 ticket:

1. The canonical repository directory and release-archive path for `loomspan-runtime-debugging/`.
2. The exact portable Agent Skills metadata fields and package-version representation to use at implementation time.
3. Which representative local client builds are available and what skill-installation and authenticated Streamable HTTP configuration each build actually supports.
4. The agent-evaluation runner, transcript/evidence format, rubric schema, repeat policy, and durable way to record model/client versions without treating prose variability as protocol failure.
5. The exact representative fixture chosen for each workflow and the subset run on each client/model combination.
6. The documented user flow for manual copy/link versus any explicit export mechanism; neither exists today.
7. The final release artifact relationship between the executable archives and the portable skill package.

## Follow-up Research 2026-08-14 18:34 PDT

### Do the open questions need to be closed before implementation?

They do not all have the same closure point. Questions 1, 2, 4, 5, 6, and 7 are concrete PR 19 design or testing-plan choices and should be settled before the implementation step that depends on them. Question 3 is partly a planning choice and partly empirical release evidence: the client families, configuration boundary, and proposed setup forms are already settled, while exact available builds and observed behavior can only be recorded by performing the representative-client checks.

The phase documents explicitly say that the Phase 1–3 product and architecture decisions are settled while fixture paths, test harnesses, and representative client interoperability values remain implementation work (`ai/thoughts/phases/loomspan_console_workflows.md:95-99`). Therefore these open questions do not reopen the product model. They close the remaining implementation-level choices inside that model.

### Resolution status by question

| Original question | What the phase documents settle | What remains to close | Closure point |
| --- | --- | --- | --- |
| 1. Canonical repository and release path | The canonical package is one `loomspan-runtime-debugging/` directory containing `SKILL.md` and five named references. It is version-controlled and distributed with Console source or release materials (`phase_3_llm_runtime_inspector.md:638-653,725-727`). | The exact repository-relative directory and archive-relative destination. | Detailed implementation plan, before build/package edits. |
| 2. Portable metadata and version representation | The package follows the portable Agent Skills format. Its version is distribution metadata, not a Go runtime gate or compatibility range (`phase_3_llm_runtime_inspector.md:653,850-865`). | Revalidation of the current portable specification and the exact supported metadata fields/version spelling. | At planning/authoring start, before committing `SKILL.md` or package validation. |
| 3. Representative client builds and setup | The local client families, loopback-only scope, user/global configuration boundary, credential constraints, and initial per-client MCP configuration forms are specified (`phase_3_llm_runtime_inspector.md:326-343,797-803`). Hosted clients remain out of scope. | Actual product/build versions available for testing, skill-installation support, negotiated/presented behavior, and results. | Record during representative-client execution; exact build availability is empirical evidence rather than an architecture decision. |
| 4. Evaluation harness and rubric mechanics | Evaluations use approved workflow/requirement IDs, do not prescribe exact call sequences or prose, need not run every variant on every client, and assess factual grounding, useful diagnosis, stable identifiers, uncertainty, and adversarial resistance (`phase_3_llm_runtime_inspector.md:383-391`). Server enforcement remains separate from agent behavior (`phase_3_llm_runtime_inspector.md:746-795`). | Runner, transcript/evidence record, scoring rubric representation, repeat policy, and version recording. | Detailed testing plan, before evaluation implementation. |
| 5. Fixture-to-workflow/client matrix | The four approved workflows are canonical; normal/degraded variants remain within them; tests cite the most specific workflow requirement ID and live beside the owning layer (`loomspan_console_workflows.md:32-49`). Phase 3 requires only a small representative client set and selected agent evaluations. | Exact representative fixture per workflow and which selected evaluations run on which client/model. | Detailed testing plan. It does not require a full Cartesian matrix. |
| 6. Manual copy/link versus export | Manual copy/link and a user-chosen export command are listed as possible workflows. Silent IDE mutation and automatic installation are excluded (`phase_3_llm_runtime_inspector.md:725-736`). The initial skill contains no scripts (`phase_3_llm_runtime_inspector.md:703-705`). | Selection of the initial supported installation flow and its documentation. | Implementation plan before CLI/build scope is chosen. The phase does not require an export command. |
| 7. Release artifact relationship | Distribution with source or release materials is required, and the skill remains one client-neutral canonical package (`phase_3_llm_runtime_inspector.md:725-743,797-803`). | Whether the skill is embedded in each platform archive, shipped as a separate versioned artifact, or both, plus checksum and release-document implications. | Implementation plan before changing deterministic archive manifests and smoke tests. |

### Additional phase guidance that narrows implementation

The phase design also closes several surrounding questions that were not stated explicitly in the original open list:

- The required capability set is the first five capability IDs; raw-artifact inspection is optional (`phase_3_llm_runtime_inspector.md:867-907`).
- Missing required capability, missing optional capability, unsupported MCP protocol, `INCOMPATIBLE_TARGET`, authentication, and evidence unavailability are distinct evaluation cases (`phase_3_llm_runtime_inspector.md:956-974,1041-1046`).
- Skill-without-MCP and MCP-without-skill are required degradation cases (`phase_3_llm_runtime_inspector.md:707-715,1066`).
- The canonical skill is identical across clients and contains no endpoint, credential, environment value, or client-specific semantic fork (`phase_3_llm_runtime_inspector.md:326-343`).
- Evaluations may vary in calls and prose; they are linked by workflow/requirement IDs and evaluated on evidence behavior rather than exact output (`phase_3_llm_runtime_inspector.md:383-391`).
- Adversarial agent results are defense-in-depth observations and cannot be reported as Go control over IDE tools or guaranteed model behavior (`phase_3_llm_runtime_inspector.md:778-795,1062-1063`).

### Revised conclusion

The phase documents resolve the product-level intent behind all seven questions and provide detailed boundaries for capability discovery, client setup, evaluation semantics, installation safety, and distribution. They intentionally do not choose the remaining file paths, portable metadata details, harness format, fixture matrix, installation mechanism, or archive topology. Those six design/testing choices should be closed in the PR 19 implementation and testing plans. Representative client build versions and observed results should be collected during execution and recorded as release evidence rather than guessed in advance.
