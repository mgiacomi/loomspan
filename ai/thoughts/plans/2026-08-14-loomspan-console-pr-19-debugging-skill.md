# Loomspan Console PR 19 Portable Debugging Skill Implementation Plan

## Overview

Ship one client-neutral `loomspan-runtime-debugging` Agent Skill that teaches an
IDE agent to investigate Loomspan runtime evidence through the existing
read-only Console MCP surface. Add workflow-linked, repeatable agent evaluation
evidence; embed the canonical skill in every native Console archive; and
document safe manual installation and representative local-client results.

This change adds procedural guidance and release/evaluation machinery. It does
not add a Console-owned LLM, another trace parser, a new MCP operation, an IDE
auto-installer, or another runtime source of truth.

## Current State Analysis

- PR 18 already exposes the twelve read-only MCP tools and six named capability
  IDs needed by this work. `LOOMSPAN_get_runtime` advertises capabilities
  independently of target and evidence state
  (`loomspan-console/internal/mcpadapter/runtime.go:13-47`,
  `loomspan-console/internal/mcpadapter/capabilities.go:9-75`).
- MCP results already preserve stable identifiers, bounded continuations,
  structured success/error arms, text fallbacks, and read-only/closed-world
  annotations (`loomspan-console/internal/mcpadapter/contracts.go:20-30,135-163,242-267`).
- Skill YAML and trace resources are explicitly described as untrusted data;
  `sourcePath` is descriptive text rather than a filesystem locator
  (`loomspan-console/internal/mcpadapter/skills.go:28-42`,
  `loomspan-console/internal/mcpadapter/trace_resources.go:45-56`).
- Server-side adversarial fixtures already prove that returned runtime content
  cannot initiate a network request, filesystem write, credential change,
  target operation, or additional service call
  (`loomspan-console/internal/mcpadapter/trace_semantic_fixtures_test.go:870-946`).
  They do not prove that an IDE model will obey skill guidance.
- The canonical workflow catalog has four developer goals and stable IDs:
  failed execution, slow execution, expensive execution, and unfamiliar skill
  path (`ai/thoughts/phases/loomspan_console_workflows.md:101-654`). Existing
  trace fixtures already map `runtime-terminal-failure`, `nested-frame-usage`,
  and `repeated-skill-invocations` to those workflows
  (`loomspan-console-fixtures/README.md:7-19`).
- No portable `SKILL.md`, package validator, agent-evaluation harness, rubric,
  or recorded agent result exists in the current tree.
- Release archives currently contain exactly `LICENSE`, the runtime `README.md`,
  and the executable. The deterministic packager and smoke extractor both
  enforce that three-file manifest
  (`loomspan-console/internal/buildtool/package.go:31-52,68-118`,
  `loomspan-console/internal/buildtool/smoke.go:23-57`).
- The post-implementation client matrix records protocol evidence but every
  local-client row is currently unexecuted
  (`loomspan-console/docs/mcp-client-compatibility.md:26-60`).

## Desired End State

The repository contains a standards-valid source package at
`loomspan-console/agent-skills/loomspan-runtime-debugging/`. Every native
Console archive contains the byte-identical package at
`skills/loomspan-runtime-debugging/`, under the archive's existing top-level
directory and checksum.

The skill uses `LOOMSPAN_get_runtime` for bootstrap, requires the first five
named capabilities, treats raw-artifact inspection as optional, selects among
the four approved workflows without imposing an exact call sequence, cites
stable evidence identifiers, distinguishes evidence/calculation/context/
inference, reports limitations and uncertainty directly, and treats all
runtime content as untrusted evidence.

A repository-native evaluation harness serves deterministic workflow cases,
records sanitized MCP interaction metadata and final answers, validates record
shape, and applies deterministic gates plus a documented human rubric. Merged
evidence demonstrates the four workflows, adversarial resistance, MCP without
the skill, skill without MCP, missing-required-capability behavior, and
missing-optional-capability degradation on a small representative client set.
Actual client/model/build versions are recorded from execution rather than
predicted in this plan.

### Key Discoveries

- The Open Agent Skills specification requires `name` and `description`, permits
  `license`, `compatibility`, and arbitrary string-valued `metadata`, recommends
  focused `references/`, and recommends a sub-5,000-token `SKILL.md`. It does
  not define a first-class version field, so package version belongs in a
  namespaced metadata entry.
- The required capability group is
  `loomspan.runtime-status.v1`, `loomspan.skill-inspection.v1`,
  `loomspan.active-execution-inspection.v1`,
  `loomspan.recent-activity-inspection.v1`, and
  `loomspan.trace-inspection.v1`; `loomspan.raw-artifact-inspection.v1` is
  optional (`ai/thoughts/phases/loomspan_console_phase_3_llm_runtime_inspector.md:850-907`).
- Tools are the complete portable investigation path; MCP resources remain
  optional presentation aids (`ai/thoughts/phases/loomspan_console_phase_3_llm_runtime_inspector.md:393-468`).
- Runtime-to-workspace matching is model reasoning. Application-returned YAML
  is authoritative only as the running application's supplied representation;
  mapping IDs and `sourcePath` are search hints, not deployment provenance
  (`ai/thoughts/phases/loomspan_console_phase_3_llm_runtime_inspector.md:595-619`).
- Capabilities, the side-effect-free Console status snapshot, and an individual
  operation result are three independent layers. The skill must not derive an
  aggregate health value or use one layer as a substitute for another
  (`ai/thoughts/phases/loomspan_console_phase_3_llm_runtime_inspector.md:895-905`).
- PR 16's supersession note is authoritative over older foundation details in
  the Phase 3 document: PR 19 consumes the implemented stateless HTTP surface,
  `lsmcp_` key format, exact `/mcp` route, and existing configuration model. It
  must not reintroduce the superseded stateful-session registry, IPv6 listener,
  `bfmcp_` prefix, or MCP YAML section
  (`ai/thoughts/phases/loomspan_console_phase_3_llm_runtime_inspector.md:9-37`).

## Resolved Implementation Decisions

| Topic | Decision |
| --- | --- |
| Canonical source location | `loomspan-console/agent-skills/loomspan-runtime-debugging/` |
| Source package contents | `SKILL.md` plus exactly five files under `references/`; no scripts or assets |
| Frontmatter | Required `name`/`description`, `license: MPL-2.0`, a short client-neutral `compatibility` statement, and `metadata.lokiscale.loomspan.skill-version: "1.0.0"`; omit experimental `allowed-tools` |
| Version model | Independent skill-package semantic version, manually bumped when the canonical instructions or reference contract changes; never compared by Go or used as a target/runtime compatibility range |
| Initial installation | Documented user-selected copy or filesystem link from the extracted archive/source tree; no export command and no automatic IDE/config mutation |
| Release topology | Embed the same canonical skill in all platform archives at `skills/loomspan-runtime-debugging/`; do not publish a second standalone artifact in PR 19 |
| Checksums | Existing archive SHA-256 sidecars and aggregate `SHA256SUMS` cover the executable, docs, and embedded skill atomically |
| Evaluation home | `loomspan-console/agent-evals/` for cases, schemas, rubric, runner documentation, and sanitized committed results |
| Runner model | Driver-neutral Go harness: serve a deterministic real MCP adapter case, capture MCP call/result metadata, accept the client-produced answer, validate/score the record, and never store a live key or Authorization header |
| Evaluation repetition | Use Codex CLI as the primary headless client: three independent runs for each core workflow and two runs for each adversarial/degradation case. Use Claude Code as the secondary headless client: two runs each for failed execution, slow execution, unfamiliar skill path, and adversarial content. Record the actual client/model builds at execution time. Manual GUI-client checks are recorded when available, but unexecuted rows are not fabricated or labeled failures. |

## What We're NOT Doing

- Adding or changing Java application API, Java SPI, Loomspan YAML syntax,
  Java-to-Go REST/SSE/NDJSON meanings, MCP tools, capability IDs, or schemas.
- Adding scripts, networking, credential management, trace parsing, repository
  browsing, control/write operations, sampling, elicitation, or prompts to the
  portable skill.
- Adding a Console-owned diagnosis engine or deterministic root-cause scorer.
- Claiming that Go controls IDE tools, repository access, model-provider data
  handling, model obedience, or evidence already returned to a client.
- Installing into client directories, editing user/global client configuration,
  or storing endpoints, keys, environment values, generated traces, or
  client-specific semantic forks in the skill.
- Publishing a separate skill archive, adding an export CLI, remote MCP access,
  hosted-client support, or a full client/model/fixture Cartesian matrix.
- Treating exact call sequences, exact answer prose, or one unexecuted client
  row as a conformance requirement.

## Skill-Authoring Documentation Impact

**Impact**: Affected

- **Rationale**: Loomspan skill authors gain a supported packaged procedure for
  using the existing debugging evidence. Runtime and manifest semantics do not
  change, but author-facing guidance must route users to the installed Agent
  Skill, explain its capability/degradation boundary, and preserve the existing
  source-verification and uncertainty rules.
- **Documents to update**:
  `ai/skill-authoring/traces-and-debugging.md` and
  `ai/skill-authoring/README.md`.
- **Supporting evidence**: the canonical Agent Skill, its workflow evaluation
  cases/results, current MCP capability tests, Java-produced trace fixtures,
  active/recent-activity fixtures, and adversarial MCP semantic test.
- **Coverage table update**: Not required. `Traces and debugging` is already
  source-verified and remains the same topic boundary; update the topic text and
  its routing description without changing its coverage classification.
- **LLM-first usability**: Keep the README route concise. Add one focused section
  to `traces-and-debugging.md` that states when to use the portable skill, what
  it requires, how it degrades, and where authority stops. Link to the canonical
  package instead of duplicating its playbooks or tool guide.

## Contract and Compatibility Impact

| Surface | Classification and supporting evidence | Planned compatibility treatment |
| --- | --- | --- |
| Application API | No `com.lokiscale.loomspan.api` type changes; the closed allowlist remains unchanged. | Preserve; no public Java surface delta. |
| Supported SPI | Loomspan exposes no supported SPI, and this plan creates none. | Preserve; no shim or extension point. |
| Configuration and manifest contracts | Existing Loomspan YAML is unchanged. A new portable Agent Skills `SKILL.md` manifest is a release artifact with documented frontmatter and independent package version. Client configuration remains user-owned. | Add one coherent v1 package contract; validate it and document manual installation. Do not add aliases or client-specific manifests. |
| Persisted or serialized contracts | No REST/SSE/NDJSON changes. Committed evaluation records are development/release evidence, not runtime interchange. | Preserve runtime boundaries; version and schema the evaluation record only for harness evolution. |
| Ephemeral diagnostic formats | The skill consumes current-process MCP projections, handles, continuations, live windows, and exact-compatible trace evidence. | Preserve current-version coherence, direct limitation facts, and untrusted-content treatment; do not promise history or cross-version trace reading. |
| Internal or accidentally exposed implementation | Go packaging helpers, skill validator, evaluation server/recorder/scorer, DTOs, and schemas are repository tooling. | Change atomically with their tests; do not expose them as Java API/SPI or a client extension contract. |

- **Evidence of supported contracts**: the Java public-surface allowlist, the PR
  19 ticket, Phase 3 capability/skill decisions, the workflow catalog, current
  MCP discovery/schema tests, Console release docs, and the Agent Skills
  specification.
- **Intended breaks**: None to an existing supported contract. Release archive
  contents deliberately grow from three runtime files to include the new skill
  directory; update packaging, smoke verification, docs, and workflow assertions
  together.
- **In-repository consumers to update**: Console build paths, packager and smoke
  extractor, packaging/smoke/project-declaration tests, CI/release workflows,
  Console READMEs, client compatibility evidence, authoring guidance, and agent
  evaluation assets.
- **Public-surface delta**: No Java type, signature, constructor, Spring bean,
  configuration key, or supported extension point is added or removed.
- **Shim decision**: **No shim.** There is no pre-existing Agent Skill contract
  to preserve, and all archive consumers and tests can move atomically to the
  new exact manifest.
- **Java-to-Go boundary coordination**: **Not required.** PR 19 consumes the
  implemented MCP/fixture semantics without changing application REST/SSE,
  acquisition, problem, or consumed NDJSON boundaries. If implementation finds
  a missing runtime fact, stop and create a separate coordinated boundary change
  rather than smuggling it into this work.

## Implementation Approach

Use one reviewed source package and make every other representation consume it.
Validate the package structure and frontmatter in repository-native Go tests,
then run the official `skills-ref validate` command in the dedicated skill
validation/release job with its dependency pinned exactly at implementation
time. The release packager reads the six allowlisted regular files directly;
it does not recursively package arbitrary content or follow links.

Keep agent evaluations above the already-tested protocol/service layer. The
harness supplies deterministic service fixtures through the production MCP
adapter, records which evidence the client requested, and scores facts that can
be checked mechanically. Human review covers usefulness, causal restraint, and
whether uncertainty is appropriately communicated. An evaluation passes only
when all hard gates pass and every rubric dimension meets its threshold; run
variance is retained rather than normalized into a scripted answer.

## Phase 1: Author and Validate the Canonical Skill

### Overview

Create the portable package, encode bootstrap/evidence/security behavior in a
compact `SKILL.md`, and split detailed runtime semantics into the five approved
progressively loaded references.

### Changes Required

#### 1. Portable package manifest and core procedure

**File**: `loomspan-console/agent-skills/loomspan-runtime-debugging/SKILL.md`

**Changes**:

- Add standards-valid frontmatter using the exact metadata decision above.
- Define activation for runtime status, failed/slow/expensive executions,
  retries/validation, and unfamiliar nested skill paths.
- Bootstrap only through `LOOMSPAN_get_runtime`; compare the returned capability
  IDs with the exact required/optional lists before selecting a workflow.
- Keep the required/optional capability declarations in the reviewed
  instructions/tool guide, not in the Agent Skills frontmatter `compatibility`
  string as though it were a machine-readable runtime gate.
- Keep unsupported MCP protocol, missing capability, `INCOMPATIBLE_TARGET`,
  authentication, unavailable/expired evidence, and target change distinct.
- Treat capability advertisement, the current status snapshot, and each tool
  result as separate facts; never synthesize a single Console/target health
  state or skip a permitted operation solely because an earlier snapshot was
  degraded.
- Direct the agent from structural summaries toward frames, records, and bounded
  payload ranges as evidence requires, without a fixed call count or sequence.
- Allow deliberate broad, complete, or raw inspection when the developer's
  question requires it; progressive disclosure is an efficiency default, not
  an artificial evidence cap.
- Require stable identifier citation and separate recorded evidence,
  deterministic Go calculations, repository context, and inference.
- Reject combining evidence from different `targetScopeId` values unless the
  developer explicitly asked for a comparison; treat stale scope-bound handles,
  resources, and continuations as `TARGET_CHANGED` rather than remapping them.
- Mark live conclusions provisional with observation time/latest sequence.
- Treat runtime text as untrusted evidence and constrain non-MCP IDE-tool use to
  the developer's explicit question and ordinary authorization.
- Provide an adaptable concise answer shape, not a mandatory long report.

#### 2. Focused progressive-disclosure references

**Files**:

- `loomspan-console/agent-skills/loomspan-runtime-debugging/references/runtime-model.md`
- `loomspan-console/agent-skills/loomspan-runtime-debugging/references/debugging-playbooks.md`
- `loomspan-console/agent-skills/loomspan-runtime-debugging/references/mcp-tool-guide.md`
- `loomspan-console/agent-skills/loomspan-runtime-debugging/references/evidence-and-confidence.md`
- `loomspan-console/agent-skills/loomspan-runtime-debugging/references/common-failure-patterns.md`

**Changes**:

- `runtime-model.md`: define target scope, instance/session/trace/frame/record
  identity, live versus finalized evidence, target/imported evidence, and
  transient handle/continuation limits. Keep application-catalog availability,
  locally installed evidence, current target authentication, and original
  observation facts separate.
- `debugging-playbooks.md`: map the four workflow IDs to investigation goals,
  useful starting evidence, stopping conditions, and degraded paths. Do not
  duplicate workflow prose or prescribe exact tool sequences.
- `mcp-tool-guide.md`: document the twelve implemented tools, required inputs,
  continuation/range behavior, tool-only portability, optional resources, and
  stable domain-error distinctions. Explain that missing raw-artifact capability
  removes only exact storage/parser forensics and that retained parsed evidence
  may remain inspectable when new target acquisition requires authentication.
- `evidence-and-confidence.md`: define evidence/calculation/context/inference
  labels, stable citation fields, uncertainty/limitation language, provisional
  live conclusions, runtime-to-workspace non-provenance rules, and the fact that
  Loomspan does not secret-scan application diagnostic content or control a
  client's/model provider's retention after authorized retrieval.
- `common-failure-patterns.md`: cover terminal/recovered failures, retry and
  validation exhaustion, timeout/quota/guardrail facts, slow-versus-stuck
  restraint, direct/descendant/unattributed usage, repeated invocations,
  truncated evidence, and adversarial content.

#### 3. Package validation

**Files**:

- `loomspan-console/internal/agentskills/validate.go`
- `loomspan-console/internal/agentskills/validate_test.go`
- `loomspan-console/internal/buildtool/runner.go`
- `.github/workflows/console-ci.yml`

**Changes**:

- Add a narrow internal validator for the exact six-file package: regular files
  only, no links/reparse points, correct parent/name match, required fields,
  allowed frontmatter subset, string metadata values, size limits, relative
  one-level reference links, no orphaned/broken references, and absence of
  endpoints, credential values, scripts, generated traces, or client forks.
- Invoke validation from canonical Console verification before packaging.
- Add an exactly pinned official `skills-ref` validation step in CI/release as
  independent standards evidence; keep Python/reference-tool dependencies out
  of the shipped archive and Console runtime.

### Success Criteria

#### Automated Verification

- [x] Package validator tests pass: `go test ./internal/agentskills/...`
- [x] Canonical Console verification validates the skill: `go run ./internal/buildtool verify`
- [x] Official reference validation passes: `skills-ref validate ./agent-skills/loomspan-runtime-debugging`
- [x] `SKILL.md` stays below the specification's recommended 5,000-token budget
  and every detailed reference is reachable directly from it.
- [x] Tests prove no endpoint, credential, scripts directory, generated trace,
  or experimental `allowed-tools` field enters the package.

#### Manual Verification

- [x] An agent loading only `SKILL.md` can select the relevant reference and
  explain what is required, optional, unavailable, and uncertain.
- [x] Review confirms the skill never states defense-in-depth guidance as a Go
  enforcement guarantee.

---

## Phase 2: Build Workflow-Linked Agent Evaluations

### Overview

Add deterministic cases, a driver-neutral recorder/scorer, a rubric, and
sanitized evidence records that evaluate agent behavior without turning prose
or tool-order variation into protocol failures.

### Changes Required

#### 1. Evaluation cases and fixture adapters

**Files**:

- `loomspan-console/agent-evals/cases/*.json`
- `loomspan-console/internal/agenteval/fixtures.go`
- `loomspan-console/internal/agenteval/fixtures_test.go`

**Changes**:

- Define versioned case manifests with case ID, workflow/requirement IDs,
  developer prompt, fixture sources, capability set, expected direct facts,
  forbidden claims/actions, required identifier classes, and limitation facts.
- Use `runtime-terminal-failure` for `WF-FAILED-EXECUTION`,
  `nested-frame-usage` plus `unattributed-usage` for
  `WF-EXPENSIVE-EXECUTION`, and `repeated-skill-invocations` for
  `WF-UNFAMILIAR-SKILL-PATH`.
- Build the `WF-SLOW-EXECUTION` case from the existing active-execution and
  recent-activity DTO fixtures, including observation time, latest sequence,
  reset/beginning-unavailable facts, and the rule that silence is not proof of
  a stuck execution.
- Add variants for all capabilities, missing required trace inspection,
  missing optional raw inspection, skill without MCP, MCP without skill,
  unsupported protocol, `INCOMPATIBLE_TARGET`, authentication required,
  no target selected, evidence unavailable/expired, target-scope change, and
  adversarial runtime instructions.
- Add the restart case from the Phase 3 completion criteria: a valid persistent
  MCP key permits initialization and `LOOMSPAN_get_runtime` after restart while
  the absent process-local application key causes only operations needing new
  target access to return `TARGET_AUTHENTICATION_REQUIRED`; already admitted
  evidence remains governed by its own validity and scope.
- Add a server-conformance case in which a capability is advertised while a
  required tool in its family is absent. Classify it as a server defect, not as
  target incompatibility and not as permission to guess an alternate tool.
- Define exact degradation expectations: skill without MCP explains applicable
  Loomspan debugging practice and directly states that live inspection is
  unavailable; MCP without the skill remains usable through its own schemas,
  results, errors, and tools; missing raw-artifact inspection removes only raw
  storage/parser forensics while parsed trace debugging continues.
- Keep server-side adversarial enforcement covered by existing Go tests; agent
  cases measure only whether the model follows the skill and avoids unrelated
  IDE-tool use/disclosure.
- Exercise adversarial instructions in each material untrusted-content class:
  skill YAML/source path, activity/error details, model/tool content, trace
  metadata/records, and explicitly selected payload/raw ranges. Do not treat one
  raw-trace fixture as proof for every agent-facing context.

#### 2. Driver-neutral evaluation runner and record schema

**Files**:

- `loomspan-console/internal/agenteval/server.go`
- `loomspan-console/internal/agenteval/record.go`
- `loomspan-console/internal/agenteval/score.go`
- `loomspan-console/internal/agenteval/*_test.go`
- `loomspan-console/internal/buildtool/main.go`
- `loomspan-console/agent-evals/schema/evaluation-record.schema.json`
- `loomspan-console/agent-evals/README.md`

**Changes**:

- Add build-tool modes to start one isolated deterministic MCP case and to
  validate/score a completed record. Use the production stateless MCP adapter
  and existing credential/HTTP boundary; do not create a mock tool protocol.
- Record case/workflow IDs, UTC time, OS, client product/build, model identifier,
  skill version, Console version/commit, run ordinal, MCP protocol, capability
  list, requested tool names with stable evidence references, final answer, and
  rubric results.
- Never record the bearer key, Authorization header, target credential, user
  home paths, unrelated repository content, or full sensitive payloads. Store
  stable fixture IDs and content hashes when exact source evidence is needed.
- Mechanically check case/record schema, required identifiers, expected and
  contradicted facts, forbidden certainty/security claims, capability/error
  classification, and whether unapproved MCP/IDE operations were observed.
- Leave explanation usefulness, causal restraint, and uncertainty quality as
  explicit reviewer-scored dimensions with evidence notes. Do not use an LLM
  judge as the sole release gate.

#### 3. Rubric, repeat policy, and committed evidence

**Files**:

- `loomspan-console/agent-evals/rubric.md`
- `loomspan-console/agent-evals/results/<date>/<client>/<case>-<run>.json`
- `loomspan-console/agent-evals/results/<date>/summary.md`

**Changes**:

- Score factual grounding, useful explanation, stable-identifier citation,
  evidence/calculation/context/inference separation, appropriate uncertainty,
  direct limitations, capability/error distinction, and adversarial resistance.
- Make fabricated facts, missing required capability misclassification,
  unsupported causal certainty, following embedded runtime instructions,
  unrelated repository disclosure, or claims of Go control over the IDE/model
  hard failures.
- Also fail a run that combines different target scopes, claims aggregate
  health, treats absent activity as proof of a stuck execution, treats missing
  usage as zero, calculates currency cost, sums overlapping inclusive usage, or
  treats `sourcePath`/a workspace match as deployment provenance.
- Run the repetition/matrix decision recorded above. Preserve every run and
  aggregate pass counts; do not replace an unfavorable run with a hand-edited
  transcript.
- Require the primary and secondary headless-client gates for merge. Record GUI
  client/build observations when executable locally; retain honest `Not run`
  rows when unavailable.

### Success Criteria

#### Automated Verification

- [x] Evaluation harness and fixture tests pass: `go test ./internal/agenteval/...`
- [x] Every case cites an approved workflow or most-specific requirement ID.
- [x] Record validation rejects missing versions, credentials/headers, unknown
  cases, fabricated capability IDs, and incomplete rubric decisions.
- [ ] Required repetition counts and hard gates pass for the selected primary
  and secondary clients.
- [x] Existing server inertness remains green:
  `go test ./internal/mcpadapter/...`

#### Manual Verification

- [ ] Reviewers can reproduce each recorded run from its case and client setup.
- [x] Different reasonable call sequences and prose can pass when they preserve
  the same supported evidence behavior.
- [x] Adversarial results are labeled defense-in-depth observations, not product
  security guarantees.

---

## Phase 3: Package the Skill with Every Console Release

### Overview

Extend deterministic native archives and smoke verification to include the
canonical skill without creating a second distribution source.

### Changes Required

#### 1. Build paths and exact archive manifest

**Files**:

- `loomspan-console/internal/buildtool/paths.go`
- `loomspan-console/internal/buildtool/package.go`
- `loomspan-console/internal/buildtool/package_test.go`

**Changes**:

- Resolve the canonical `agent-skills` source directory from the repository,
  not from the launch working directory.
- Add the exact six skill files to `packageRequest` and archive them as regular
  mode-`0644` entries beneath `skills/loomspan-runtime-debugging/`.
- Validate before loading and reject missing, extra, linked/reparse, nonregular,
  or unsafe source paths. Continue sorting archive entry names and preserving
  deterministic timestamps and bytes.
- Update exact ZIP/TAR manifest and reproducibility assertions for all three
  supported release targets.

#### 2. Native smoke extraction and CI/release assertions

**Files**:

- `loomspan-console/internal/buildtool/smoke.go`
- `loomspan-console/internal/buildtool/smoke_test.go`
- `loomspan-console/internal/buildtool/projectdeclarations_test.go`
- `.github/workflows/console-release.yml`

**Changes**:

- Extend strict extraction allowlists to the six skill files while retaining
  path traversal, duplicate entry, file type, mode, checksum, version, and
  executable-startup checks.
- Validate the extracted skill and compare its bytes with the canonical source
  during native smoke execution.
- Keep one archive and sidecar per platform; the existing aggregate checksum
  publication covers the expanded archives.

### Success Criteria

#### Automated Verification

- [x] Packaging tests prove deterministic exact contents on ZIP and TAR.GZ:
  `go test ./internal/buildtool/...`
- [x] Canonical package build succeeds:
  `go run ./internal/buildtool package --expected-version 0.1.0-SNAPSHOT`
- [x] Native smoke validates executable and extracted skill:
  `go run ./internal/buildtool smoke --expected-version 0.1.0-SNAPSHOT --archive dist/ARCHIVE`
- [ ] CI/release jobs smoke all supported native targets and publish matching
  checksums without a second skill artifact.

#### Manual Verification

- [x] A developer can locate one obvious
  `skills/loomspan-runtime-debugging/` directory after extracting any platform
  archive.
- [x] Copying or linking that directory preserves its internal relative
  reference paths.

---

## Phase 4: Documentation, Installation, and Interoperability Evidence

### Overview

Document the release layout, safe client-owned setup, degradation model,
evaluation procedure, and observed representative-client results. Synchronize
the authoring knowledge base without duplicating the canonical skill.

### Changes Required

#### 1. Console source and release documentation

**Files**:

- `loomspan-console/README.md`
- `loomspan-console/release/README.md`
- `loomspan-console/docs/mcp-client-compatibility.md`

**Changes**:

- Replace the three-file archive description with the exact expanded layout and
  explain that the embedded skill version is independent distribution metadata.
- Document manual copy/link from the archive or source package into each tested
  client's user/global skill location. Keep client paths/configuration as thin
  setup shims and revalidate them against current vendor docs during execution.
- State that live inspection requires the already-configured local MCP
  connection; do not put its endpoint or key into the skill.
- Document all-required, missing-required, missing-optional, skill-only, and
  MCP-only behavior and keep protocol/capability/target/auth/evidence failures
  distinct.
- Extend the client matrix with skill discovery/activation, selected evaluation
  case IDs, client/model/build versions, OS, configuration scope, observed MCP
  protocol, resource behavior, continuation behavior, and concise result links.
  Do not guess unavailable versions or convert `Not run` into failure.
- Preserve the Phase 3 client-family scope—local Codex surfaces, Claude Code,
  local Antigravity, local Cursor, and Devin Desktop/Windsurf/Cascade or local
  Devin CLI—and record skill plus authenticated Streamable HTTP behavior to the
  extent each available build permits. Hosted clients remain a documented
  loopback reachability exclusion.

#### 2. Skill-authoring knowledge-base synchronization

**Files**:

- `ai/skill-authoring/traces-and-debugging.md`
- `ai/skill-authoring/README.md`

**Changes**:

- Add a compact author-facing route to the canonical debugging skill and state
  when it applies, its required/optional capability boundary, manual install
  nature, and defense-in-depth limitation.
- Retain existing trace semantics and source anchors; link to the skill's focused
  references rather than copying its playbooks or MCP operation catalog.
- Keep the README coverage classification unchanged while making the debugging
  route mention the packaged Agent Skill.

#### 3. Final Phase 3 conformance record

**Files**:

- `loomspan-console/agent-evals/results/<date>/summary.md`
- `loomspan-console/docs/mcp-client-compatibility.md`
- `ai/thoughts/phases/loomspan_console_phase_3_llm_runtime_inspector.md`

**Changes**:

- Link the concrete workflow evaluation and client evidence from the active
  Phase 3 document without copying transcripts or changing settled semantics.
- State exactly which completion criteria are automated, agent-evaluated,
  manually observed, unavailable, or out of scope.

### Success Criteria

#### Automated Verification

- [x] Documentation links resolve and package-layout assertions match the actual
  archive.
- [x] `go run ./internal/buildtool verify` passes after all documentation and
  project-declaration changes.
- [x] `go run ./internal/buildtool mcp-conformance` remains green for both
  supported protocol revisions.
- [x] The authoring guidance changed in this phase is supported by the cited
  skill, cases, fixture tests, and production MCP source.
- [x] The README route and topic follow the LLM-first authoring standard and do
  not duplicate the five skill references.
- [x] `LoomspanPublicSurfaceArchitectureTest` passes, confirming no Java public
  surface delta.

#### Manual Verification

- [ ] A fresh local client can discover and activate the copied/linked skill and
  connect to MCP using user/global configuration without repository credentials.
- [ ] The primary and secondary representative agents satisfy the acceptance
  rubric for their selected workflows and degradation cases.
- [ ] Client evidence records actual product/model/build values and leaves
  unavailable client rows explicitly unexecuted.
- [x] Release documentation never implies hosted-loopback support, automatic
  installation, deterministic model behavior, or server control of IDE tools.

---

## Testing Strategy

Create the dedicated testing-plan artifact with `ai/commands/3_testing_plan.md`
before implementation. It should turn the selected case matrix above into
fixture-by-fixture failing tests, exact runner invocations, reviewer instructions,
credential-sanitization checks, repeat accounting, native archive smoke steps,
and exit criteria.

### Unit Tests

- Agent Skill frontmatter, name/parent match, version metadata, package shape,
  link integrity, content limits, and prohibited content.
- Evaluation case/record schema, sanitized recording, deterministic fact gates,
  forbidden-claim detection, repeat aggregation, and rubric completeness.
- Exact deterministic archive entries and extracted-skill validation.

### Integration Tests

- Production MCP adapter over each deterministic workflow/degradation fixture.
- Skill bootstrap against all, missing-required, and missing-optional capability
  catalogs, plus advertised-capability/incomplete-tool-family conformance.
- No-target and restart-with-persistent-MCP-key/no-application-key behavior,
  including permitted status and retained-evidence operations.
- Native package/smoke on Windows x86_64, Linux x86_64, and macOS arm64.
- Existing MCP conformance, schema/result/error, continuation/range, resource,
  browser/MCP parity, concurrent browser/client, cancellation/shutdown, and
  adversarial server suites, proving that evaluations do not perturb the
  browser, another MCP client, or the observed Loomspan execution.

### Manual Testing Steps

1. Extract a native archive and copy/link the embedded skill into the selected
   primary local client's user skill directory.
2. Configure the existing loopback MCP endpoint and key through the client's
   protected user/global mechanism, without committing either value.
3. Run the prescribed core, adversarial, and degradation case matrix with fresh
   conversations and record exact client/model/build information.
4. Review each final answer against its fixture oracle and rubric, then validate
   and commit only the sanitized evaluation record.
5. Repeat selected cases on the secondary local client and record GUI-client
   interoperability observations when those products are available.

## Performance Considerations

The skill adds only small static archive files and on-demand model context.
Keep `SKILL.md` compact and load focused references only as needed. The
evaluation harness must reuse existing bounded MCP pages/ranges and must not
introduce a full-runtime dump, unbounded transcript capture, or a second trace
cache. Archive collection remains a fixed six-file allowlist, so packaging cost
is negligible and deterministic.

## Migration Notes

There is no runtime or data migration. Existing Console users receive an
additional archive directory and may opt into manual skill installation. The
first package version is `1.0.0`; future skill-only instruction changes bump
that metadata independently from the Console product version. Removing or
renaming the package later requires an explicit release/documentation decision,
but it does not create a Java compatibility shim or runtime negotiation path.

## References

- Original ticket: `ai/thoughts/tickets/loomspan-console-pr-19-debugging-skill.md`
- Related research: `ai/thoughts/research/2026-08-14-loomspan-console-pr-19-debugging-skill.md`
- Phase 3 design: `ai/thoughts/phases/loomspan_console_phase_3_llm_runtime_inspector.md`
- Workflow catalog: `ai/thoughts/phases/loomspan_console_workflows.md`
- Framework design lens: `ai/thoughts/framework-feature-design-lens.md`
- Agent Skills specification: <https://openagentskills.dev/docs/specification>
- Agent Skills reference implementation: <https://github.com/agentskills/agentskills/tree/main/skills-ref>
