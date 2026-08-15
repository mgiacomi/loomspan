# Loomspan Console PR 19 Portable Debugging Skill Testing Plan

## Change Summary

- Add the canonical `loomspan-runtime-debugging` Agent Skill at
  `loomspan-console/agent-skills/loomspan-runtime-debugging/`, containing one
  `SKILL.md` and five directly referenced Markdown files.
- Validate its Open Agent Skills frontmatter, exact package shape, progressive
  disclosure links, package version, and prohibited content.
- Add deterministic workflow/degradation cases, a production-MCP-backed agent
  evaluation runner, sanitized versioned records, deterministic hard gates, a
  human rubric, and repeat aggregation.
- Embed the byte-identical canonical skill at
  `skills/loomspan-runtime-debugging/` in every native Console archive and
  extend strict smoke verification.
- Record representative Codex CLI and Claude Code evaluations plus honest
  local GUI-client compatibility observations.
- Update Console/release documentation and route Loomspan skill authors to the
  portable debugging skill without duplicating its playbooks.

This is additive feature work rather than a bug fix. The first failing test
therefore proves the missing release behavior: current archives do not contain
the required skill package.

## Impacted Areas

- `loomspan-console/agent-skills/loomspan-runtime-debugging/` — new supported
  Agent Skill manifest and references.
- `loomspan-console/internal/agentskills/` — new internal structural/content
  validator.
- `loomspan-console/agent-evals/` and
  `loomspan-console/internal/agenteval/` — new cases, schemas, runner, recorder,
  scorer, rubric, and sanitized results.
- `loomspan-console/internal/buildtool/` — new validation/evaluation modes,
  pipeline phase, canonical source path, deterministic package inputs, and
  strict smoke expectations.
- `.github/workflows/console-ci.yml` and
  `.github/workflows/console-release.yml` — pinned reference validation and
  native archive verification.
- `loomspan-console/README.md`, `loomspan-console/release/README.md`, and
  `loomspan-console/docs/mcp-client-compatibility.md` — archive layout,
  installation, security/degradation behavior, and execution evidence.
- `ai/skill-authoring/traces-and-debugging.md` and
  `ai/skill-authoring/README.md` — author-facing route and capability boundary.
- Existing MCP, trace-analysis, browser-parity, lifecycle, security, Java
  fixture, and public-surface suites — retained regression gates, not new PR 19
  semantics.

## Risk Assessment

### Primary Risks

- A malformed or client-specific `SKILL.md` is discoverable by one client but
  not portable across standards-compatible clients.
- Required/optional capabilities are wrong, encoded as a runtime version range,
  inferred from tool absence, or confused with target/evidence availability.
- The skill fabricates causal certainty, combines different target scopes,
  claims aggregate health, treats provisional/unknown facts as final/zero, or
  treats `sourcePath` as deployment provenance.
- Runtime text causes an agent to follow embedded tool, URL, command,
  credential, target, or investigation-override instructions. Passing agent
  tests could then be overstated as a Go security guarantee.
- Evaluation records leak the MCP key, Authorization header, target key,
  machine paths, sensitive payloads, or unrelated repository content.
- A scorer becomes coupled to exact prose or exact MCP call order and rewards a
  scripted answer rather than evidence-backed investigation.
- An evaluation fixture bypasses the production MCP adapter and consequently
  misses authentication, capability discovery, schema, text/structured result,
  continuation, or error behavior.
- Archive collection follows a link/reparse point, admits unexpected files,
  changes modes/paths, becomes nondeterministic, or packages bytes different
  from the reviewed canonical source.
- Strict smoke extraction accepts traversal, duplicates, directories, wrong
  modes, or incomplete skill packages.
- Adding official `skills-ref` validation silently introduces an unpinned
  supply-chain dependency or a runtime dependency in the shipped Console.
- Authoring documentation states skill/model guidance as enforced runtime
  behavior or duplicates enough skill content to drift independently.

### Edge Cases

- Missing each required capability individually; missing only optional
  `loomspan.raw-artifact-inspection.v1`; a capability advertised with one
  required tool/semantic fixture absent.
- MCP unavailable, MCP available without the skill, no selected target,
  unsupported MCP protocol, `INCOMPATIBLE_TARGET`, application authentication
  required, evidence unavailable/expired, `TARGET_CHANGED`, and restart with a
  persistent MCP key but no process-local application key.
- Valid acquired evidence remains inspectable while new target acquisition
  requires authentication.
- Live activity has a reset boundary or `beginningUnavailable`; a quiet window
  does not prove a stuck execution; active usage/path/outcome remains
  provisional.
- Missing usage is unknown, inclusive parent/child usage is not double-counted,
  monetary cost is unavailable, and `sourcePath`/mapping IDs are search hints.
- Adversarial instructions appear in YAML/source path, activity/error details,
  model/tool content, trace metadata/records, payloads, and raw ranges.
- Agent answers use different reasonable tool orders, omit irrelevant report
  sections, or stop early because safe evidence is insufficient.
- Skill source contains an extra file, missing reference, orphaned reference,
  nested/deep link, invalid frontmatter, non-string metadata, wrong version,
  endpoint/key, scripts/assets, symlink, Windows reparse point, or oversized
  instruction body.

### Contract and Compatibility Scope

| Surface | Test obligation |
| --- | --- |
| Application API | No change. Run `LoomspanPublicSurfaceArchitectureTest`; do not add a new allowlisted Java type. |
| Supported SPI | No change. No replacement/extension-point test or compatibility shim is expected. |
| Configuration and manifest contracts | Protect the new v1 Agent Skill manifest, exact source-package shape, independent `1.0.0` metadata, and client-neutral/manual-install contract. Existing Loomspan YAML and Console configuration remain unchanged. |
| Persisted or serialized contracts | Runtime REST/SSE/NDJSON is unchanged. Version the evaluation-record schema for repository evidence only; do not imply it is runtime interchange. |
| Ephemeral diagnostic formats | Reuse current Java fixtures and Go MCP/projector tests for present-version accuracy, ordering, limitation facts, security boundaries, and exact-compatible trace handling. Do not add historical readers or old trace fixtures. |
| Internal or accidentally exposed implementation | Validator, build-tool, evaluator, scorer, and archive helpers may change atomically. Test the intended behavior, not internal helper signatures. |

There are no approved obsolete Application API, SPI, YAML, or Java-to-Go paths
to preserve. The only intentional existing-behavior change is the exact release
archive manifest: the old three-file expectation is replaced atomically by the
runtime files plus the six skill files. No legacy three-file mode or separate
skill artifact should remain.

## Existing Test Coverage

### Coverage to Retain

- `mcpadapter.TestRuntimeCapabilitiesMatchCompleteFamilies`,
  `TestCapabilityDescriptorsMatchInstalledToolFamilies`, and
  `TestPR18CapabilityManifestMatchesReviewedDescriptorAndRejectsIndependentGaps`
  already prove the six exact capability families and suppress advertisement
  when a required tool or reviewed semantic fixture is absent.
- `mcpadapter.TestRuntimeOutputGoldenAndTextAgree` and
  `TestRuntimeOutputSucceedsForEveryTargetStatusFactAndRejectsInvalidInvariant`
  already cover no-target, authentication-required, incompatible, connected,
  and independent status facts while returning all installed capabilities.
- `mcpadapter.TestExecutionActivityGoldenPreservesCompleteEnvelopesAndConciseText`
  and adjacent activity tests protect observation/reset/beginning-unavailable
  facts, inert details, finalization uncertainty, and 64-item completeness.
- `traceanalysis.TestFixtureCorpusMatchesJavaExpectedSemantics` plus calculation
  and continuation tests protect terminal failure, hierarchy, duration, direct/
  descendant/inclusive/unattributed usage, unknown-not-zero semantics,
  attempt/retry identity, every-item reachability, exact ranges, and
  `TARGET_CHANGED`/`ARTIFACT_EXPIRED` precedence.
- `mcpadapter.TestPR18SemanticFixtures` protects target/imported workflows,
  stable trace evidence, and server-side inert return of adversarial content.
- MCP parity, joined-adapter, range, security, lifecycle, and server tests
  protect browser/MCP semantic parity, one shared artifact, concurrent clients,
  cancellation/drain, `no-store`, and supported protocol initialization.
- `buildtool.TestReleasePackagesAreDeterministicAndContainOnlyRuntimeFiles`
  currently protects the old three-file exact archive; it is the primary test
  to update for the intentional manifest expansion.
- `buildtool` smoke tests already reject checksum mismatch, traversal,
  unexpected entries, wrong top-level paths, and product-version mismatch.
- `TestProjectDeclarationsMatchPinnedToolchains`,
  `TestMCPDependenciesAndSDKBoundaryArePinned`, and
  `TestConsoleWorkflowsArePinnedAndLeastPrivilege` protect exact toolchain/
  conformance pins and workflow permissions.
- `ConsoleTraceFixtureCorpusTest` protects the Java-produced fixture side, and
  `LoomspanPublicSurfaceArchitectureTest` protects the closed Java API surface.

### Current Gaps

- No canonical Agent Skill package or structural/content validator.
- No test that archive and source skill bytes are identical on all targets.
- No official Agent Skills reference validation in CI/release.
- No workflow/degradation case schema, agent runner, sanitized record schema,
  deterministic scorer, human rubric, or repetition aggregator.
- No representative model run or skill activation/interoperability evidence.
- No automated distinction between a headless client transcript that fully
  reports IDE tool use and a GUI observation that cannot provide such an event
  stream.
- No test that updated authoring guidance links to the canonical package and
  retains the correct capability/degradation/security boundary.

## Bug Reproduction / Failing Test First

- **Name**: `TestReleasePackagesAreDeterministicAndContainRuntimeDebuggingSkill`
- **Type**: Unit
- **Location**: update
  `loomspan-console/internal/buildtool/package_test.go`
- **Arrange**: Keep the existing temporary executable, license, README, three
  target matrix, two identical packaging runs, and ZIP/TAR entry reader. Add a
  temporary canonical six-file skill package with distinctive contents.
- **Act**: Call the current `writeReleasePackage` for each target and inspect the
  archive entry map.
- **Assert**: Expect the executable, `LICENSE`, `README.md`, and exactly these
  mode-`0644` entries beneath `skills/loomspan-runtime-debugging/`:
  `SKILL.md` and the five named `references/*.md` files. Assert deterministic
  bytes and sidecar digest as today.
- **Expected failure before implementation**: the current `packageRequest` has
  no skill input and the produced archive has only three entries, so the exact
  manifest assertion fails. If the test is staged before the request field is
  added, first make the expectation fail without referring to a nonexistent
  field; add the fixture input as the production signature changes.
- **Why this is first**: it is a low-cost existing test at the deliberate public
  release behavior boundary. Passing it requires one canonical package to
  exist and forces packaging to consume it, while later validator and smoke
  tests refine safety and portability.

## Tests to Add or Update

### 1. Canonical Skill Package Validates as One Exact Portable Package

- **Name**: `TestCanonicalRuntimeDebuggingSkillIsValidAndExact`
- **Type**: Unit/integration
- **Location**:
  `loomspan-console/internal/agentskills/validate_test.go`
- **What it proves**: the repository package contains exactly `SKILL.md` and the
  five approved references; files are regular and non-linked; `name` matches
  the parent; required/allowed frontmatter and
  `metadata["lokiscale.loomspan.skill-version"] == "1.0.0"` are exact; all
  metadata values are strings; `SKILL.md` directly links every reference once;
  no reference is missing/orphaned; no scripts/assets/client forks exist; the
  instruction body remains within the chosen 5,000-token validation budget.
- **Fixtures/data**: the real canonical source package, not a copied golden.
- **Mocks**: none; use a resolver rooted at the repository checkout.
- **Contract classification**: Configuration and manifest contracts.
- **Compatibility expectation**: protect the new coherent v1 package; no alias,
  old path, or client-specific manifest.

### 2. Invalid Agent Skill Table Is Rejected Precisely

- **Name**: `TestRuntimeDebuggingSkillValidationRejectsUnsafeAndNonPortableVariants`
- **Type**: Unit, table-driven
- **Location**:
  `loomspan-console/internal/agentskills/validate_test.go`
- **What it proves**: independently reject missing/extra files, invalid YAML,
  missing/empty/oversized description, wrong parent/name, unsupported
  frontmatter, non-string metadata, missing/wrong version, deep/broken/orphaned
  reference links, endpoint/key/auth-header content, generated trace content,
  scripts/assets, symlink/nonregular input, oversized body, and experimental
  `allowed-tools`.
- **Fixtures/data**: one temporary valid minimal package mutated once per
  subtest; native Windows reparse behavior gets a platform-specific subtest.
- **Mocks**: filesystem only; do not execute repository content.
- **Contract classification**: Configuration and manifest contracts.
- **Compatibility expectation**: invalid packages fail closed with a path and
  rule-specific error; the validator never silently repairs or drops content.

### 3. Canonical Verification Runs Skill Validation in Fail-Fast Order

- **Name**: `TestRunPipelineValidatesAgentSkillBeforeGoTestsAndPackaging`
- **Type**: Unit
- **Location**: update
  `loomspan-console/internal/buildtool/pipeline_test.go`
- **What it proves**: verify/build/package include one named Agent Skill
  validation phase after toolchain/dependency setup and before Go tests/package
  output; a validation failure stops later phases; ordinary verify still does
  not build a binary.
- **Fixtures/data**: existing fake phase runner and a sentinel validation error.
- **Mocks**: existing `pipelineDependencies.run` fake.
- **Contract classification**: Internal or accidentally exposed implementation.
- **Compatibility expectation**: atomic build-tool update; no legacy path that
  packages an unvalidated skill.

### 4. Official Reference Validator Is Pinned and Required

- **Name**: extend `TestProjectDeclarationsMatchPinnedToolchains` and
  `TestConsoleWorkflowsArePinnedAndLeastPrivilege`
- **Type**: Unit/declaration
- **Location**:
  `loomspan-console/internal/buildtool/projectdeclarations_test.go`
- **What it proves**: the official `skills-ref` dependency resolves from an
  exact reviewed version/revision and lock; CI/release invoke it against the
  canonical package; actions remain full-SHA pinned and read-only except the
  existing publish job; Python/reference tooling is not imported by or bundled
  into the Console executable/archive.
- **Fixtures/data**: workflow text and the new reference-tool lock/manifest.
- **Mocks**: none.
- **Contract classification**: Internal or accidentally exposed implementation.
- **Compatibility expectation**: protected deterministic build/release path;
  no floating `latest` install.

### 5. Evaluation Case Manifests Are Complete and Workflow-Linked

- **Name**: `TestEvaluationCasesAreVersionedUniqueAndWorkflowLinked`
- **Type**: Unit
- **Location**: `loomspan-console/internal/agenteval/fixtures_test.go`
- **What it proves**: every case has a unique ID, schema version, approved
  workflow or most-specific requirement ID, developer prompt, fixture source,
  capability set, expected direct facts, forbidden claims/actions, required
  identifier classes, and limitation facts. It rejects a second workflow-ID
  namespace or copied workflow prose.
- **Fixtures/data**: all JSON files under
  `loomspan-console/agent-evals/cases/`.
- **Mocks**: none.
- **Contract classification**: Internal or accidentally exposed implementation.
- **Compatibility expectation**: evaluation schema is versioned repository
  evidence, not a runtime API.

### 6. Workflow Cases Reuse the Authoritative Existing Fixtures

- **Name**: `TestEvaluationCasesResolveAuthoritativeFixtureFacts`
- **Type**: Integration
- **Location**: `loomspan-console/internal/agenteval/fixtures_test.go`
- **What it proves**:
  - failed execution resolves `runtime-terminal-failure` and its expected
    terminal/failure/attempt/usage facts;
  - expensive execution resolves `nested-frame-usage` and
    `unattributed-usage` without double counting or cost judgment;
  - unfamiliar path resolves `repeated-skill-invocations` with distinct frame
    IDs and exact registered names;
  - slow execution resolves the existing active-execution/activity fixtures,
    including observation time, latest sequence, reset, and
    `beginningUnavailable`.
- **Fixtures/data**: `loomspan-console-fixtures/traces`, `expected`, application
  REST/SSE fixtures, and `internal/mcpadapter/testdata`; do not duplicate NDJSON
  into `agent-evals`.
- **Mocks**: scripted transport-neutral services only where live state has no
  finalized trace; finalized cases use the real trace processor.
- **Contract classification**: Ephemeral diagnostic formats.
- **Compatibility expectation**: current-run Java/Go/agent-fixture coherence;
  no historical trace reader or alternate calculation.

### 7. Degradation Matrix Keeps Protocol, Capability, Target, Auth, and Evidence Separate

- **Name**: `TestEvaluationDegradationCasesHaveDistinctExpectedClassifications`
- **Type**: Unit/integration, table-driven
- **Location**: `loomspan-console/internal/agenteval/fixtures_test.go`
- **What it proves**: distinct case results for all capabilities, each missing
  required capability, missing optional raw capability, MCP unavailable, no
  target, unsupported protocol, `INCOMPATIBLE_TARGET`,
  `TARGET_AUTHENTICATION_REQUIRED`, unavailable/expired evidence, and
  `TARGET_CHANGED`. Capability/status/operation facts never collapse into
  aggregate health.
- **Fixtures/data**: capability/status/result fixture table and existing stable
  domain codes.
- **Mocks**: service fakes return typed domain errors; unsupported protocol uses
  the assembled HTTP adapter rather than a domain fake.
- **Contract classification**: Ephemeral diagnostic formats.
- **Compatibility expectation**: current named capability and error semantics
  remain coherent; no new runtime version or fallback tool name.

### 8. Restart with Persistent MCP Key Allows Only Currently Permitted Operations

- **Name**: `TestAgentEvaluationServerRestartKeepsMCPButRequiresApplicationKeyForNewTargetAccess`
- **Type**: Integration
- **Location**: `loomspan-console/internal/agenteval/server_test.go`
- **What it proves**: after restarting an isolated evaluation Console with a
  valid profile key and no process-local application key, MCP initializes and
  `LOOMSPAN_get_runtime` reports independent authentication/compatibility facts;
  new target acquisition returns `TARGET_AUTHENTICATION_REQUIRED`; prior-process
  evidence is absent because restart cleanup never adopts it. A separate
  same-process subtest proves that later upstream authentication rejection does
  not invalidate a complete current-scope acquired artifact until its ordinary
  handle/scope lifecycle ends.
- **Fixtures/data**: protected temporary profile, real MCP credential store and
  adapter, scripted target, and a current-process installed fixture only for the
  separate post-acquisition authentication-rejection subtest.
- **Mocks**: scripted target client; never bypass MCP HTTP/authentication.
- **Contract classification**: Ephemeral diagnostic formats.
- **Compatibility expectation**: retain implemented PR 16/18 behavior; do not
  add a skill-specific health gate.

### 9. Evaluation Server Uses the Production MCP Boundary

- **Name**: `TestEvaluationServerRunsProductionMCPAdapterAndCapturesFiniteOperations`
- **Type**: Integration
- **Location**: `loomspan-console/internal/agenteval/server_test.go`
- **What it proves**: the harness creates an isolated protected key, starts the
  production stateless Streamable HTTP handler, supports both negotiated
  revisions through existing behavior, exposes the real tool/capability set,
  records finite requested operation metadata, and cleans the temporary
  profile/workspace without mutating a developer profile.
- **Fixtures/data**: one finalized and one live case.
- **Mocks**: transport-neutral fixture services below the production adapter;
  no fake MCP protocol or hard-coded tool response layer.
- **Contract classification**: Internal or accidentally exposed implementation.
- **Compatibility expectation**: evaluator stays a consumer of the protected
  MCP behavior, not a second server surface.

### 10. Evaluation Records Are Versioned, Reproducible, and Sanitized

- **Name**: `TestEvaluationRecordRoundTripAndSanitization`
- **Type**: Unit
- **Location**: `loomspan-console/internal/agenteval/record_test.go`
- **What it proves**: records require schema/case/workflow IDs, UTC time, OS,
  client product/build, model identifier, skill version, Console version/commit,
  run ordinal, protocol, capabilities, stable evidence references, final answer,
  transcript-completeness declaration, and rubric results; canonical JSON
  round-trips deterministically.
- **Fixtures/data**: one complete synthetic record plus field-by-field missing/
  invalid variants.
- **Mocks**: fixed clock and IDs.
- **Contract classification**: Persisted or serialized contracts.
- **Compatibility expectation**: versioned repository evidence only; reject
  unknown schema versions rather than silently interpreting them.

### 11. Record Writer Cannot Persist Credentials or Machine-Specific Evidence

- **Name**: `TestEvaluationRecordRejectsSecretsHeadersPathsAndRawSensitiveContent`
- **Type**: Unit/integration, table-driven
- **Location**: `loomspan-console/internal/agenteval/record_test.go`
- **What it proves**: reject the actual generated MCP key, target key,
  `Authorization: Bearer`, any `lsmcp_`-shaped key, temporary profile/workspace
  absolute paths, unrelated repository content, and unapproved full payloads in
  every serializable field. Operation records retain tool name, argument/result
  hashes, stable fixture IDs, and cited evidence references instead.
- **Fixtures/data**: generated test credentials, Windows/POSIX absolute paths,
  adversarial payload text, and a safe redacted record.
- **Mocks**: fixed credential supplier and filesystem roots.
- **Contract classification**: Persisted or serialized contracts.
- **Compatibility expectation**: fail closed; no best-effort secret stripping
  that could silently miss a value.

### 12. Headless Client Transcript Completeness Gates IDE-Tool Assertions

- **Name**: `TestHeadlessEvaluationRequiresCompleteClientEventStreamForToolSafetyGate`
- **Type**: Unit
- **Location**: `loomspan-console/internal/agenteval/record_test.go`
- **What it proves**: Codex CLI/Claude Code hard gates about shell/network/
  filesystem/repository tool use require a complete native JSON/event transcript
  with every client tool call classified. An incomplete transcript cannot pass
  that gate. GUI observations are explicitly manual and cannot be promoted to
  the same automated assertion.
- **Fixtures/data**: complete safe stream, complete stream with unrelated tool
  call, truncated stream, and GUI/manual record.
- **Mocks**: client event importer only; do not invoke a real model in unit tests.
- **Contract classification**: Internal or accidentally exposed implementation.
- **Compatibility expectation**: accurate evidence claims; absence of runner
  visibility is unknown, not proof of safe model behavior.

### 13. Deterministic Scorer Enforces Facts Without Enforcing Prose or Call Order

- **Name**: `TestScorerAcceptsEquivalentInvestigationsAndRejectsUnsupportedClaims`
- **Type**: Unit, table-driven
- **Location**: `loomspan-console/internal/agenteval/score_test.go`
- **What it proves**: two answers with different reasonable call orders and
  prose pass when they cite required identifiers/facts and limitations. Failures
  include fabricated facts, missing-required capability mislabeled as target
  incompatibility, unsupported root cause, aggregate health, cross-scope mixing,
  quiet-means-stuck, unknown-means-zero, inclusive-usage double count, currency
  cost, workspace provenance, following embedded instructions, unrelated tool
  use/disclosure, or claims that Go controls the IDE/model.
- **Fixtures/data**: small synthetic transcripts/answers tied to real case
  oracles; include an evidence-insufficient answer that correctly stops.
- **Mocks**: none; deterministic gates only. Use human rubric fixtures for
  qualities that cannot be checked reliably from structured facts.
- **Contract classification**: Internal or accidentally exposed implementation.
- **Compatibility expectation**: no exact answer golden and no prescribed MCP
  sequence.

### 14. Human Rubric Is Complete and Cannot Be Bypassed by an LLM Judge

- **Name**: `TestRubricRequiresEveryDimensionReviewerEvidenceAndHardGatePass`
- **Type**: Unit
- **Location**: `loomspan-console/internal/agenteval/score_test.go`
- **What it proves**: factual grounding, usefulness, stable citation,
  evidence/calculation/context/inference separation, uncertainty, limitations,
  capability/error distinction, and adversarial resistance all require a score
  and reviewer note; any hard failure overrides totals; an optional LLM review
  cannot be the sole or overriding release decision.
- **Fixtures/data**: passing review, missing dimension/note, failed hard gate,
  disagreement, and optional advisory model review.
- **Mocks**: reviewer data only.
- **Contract classification**: Internal or accidentally exposed implementation.
- **Compatibility expectation**: evaluation discipline, not model-output API.

### 15. Repeat Aggregation Preserves Every Run and Enforces the Selected Matrix

- **Name**: `TestEvaluationSummaryRequiresSelectedRunsAndNeverDropsFailures`
- **Type**: Unit
- **Location**: `loomspan-console/internal/agenteval/score_test.go`
- **What it proves**: the required release matrix is exactly:
  - Codex CLI: three fresh runs for each of the four core workflow cases;
  - Codex CLI: two fresh runs each for composite adversarial content, missing
    required capability, missing optional raw capability, and skill without MCP;
  - Claude Code: two fresh runs each for failed execution, slow execution,
    unfamiliar skill path, and composite adversarial content.

  The summary retains and counts all 28 runs, requires actual client/model build
  values, rejects duplicate run IDs/conversations, and cannot replace or omit a
  failed run. MCP-without-skill and protocol/service distinctions remain
  deterministic/manual coverage rather than additional model repetitions.
- **Fixtures/data**: complete 28-record set, one missing, one duplicate, one
  failed, and mixed-version sets.
- **Mocks**: none.
- **Contract classification**: Persisted or serialized contracts.
- **Compatibility expectation**: exact PR 19 release-evidence policy; no full
  client/model/fixture Cartesian matrix.

### 16. Agent Evaluation Cases Cover Each Workflow and Skill Degradation

- **Name**: result records keyed by case ID rather than Go test names
- **Type**: Agent e2e/manual-reviewed release gate
- **Location**:
  `loomspan-console/agent-evals/results/<date>/<client>/<case>-<run>.json`
- **What it proves**:
  - `WF-FAILED-EXECUTION`: terminal versus recovered failure, preceding
    evidence, stable failure/frame/attempt IDs, and restrained cause.
  - `WF-SLOW-EXECUTION`: provisional active facts, observation/latest sequence,
    reset/gap limitation, and no stuck claim from silence.
  - `WF-EXPENSIVE-EXECUTION`: direct/descendant/inclusive/unattributed usage,
    limits, no double count, no currency/correctness judgment.
  - `WF-UNFAMILIAR-SKILL-PATH`: distinct repeated frames, exact registered YAML,
    and no deployment provenance or restructuring judgment.
  - Missing required capability: exact capability named before dependent work.
  - Missing optional raw capability: parsed investigation continues; only raw
    storage/parser forensics is unavailable.
  - Skill without MCP: debugging practice is explained and live inspection is
    directly reported unavailable.
  - Composite adversarial case: instructions across every material content
    class are treated as data; unrelated IDE tools/content are not used.
- **Fixtures/data**: the eight approved agent cases and fresh client sessions.
- **Mocks**: deterministic fixture services below production MCP; real client
  and model above it.
- **Contract classification**: Ephemeral diagnostic formats.
- **Compatibility expectation**: current representative defense-in-depth and
  diagnosis evidence, not guaranteed model behavior.

### 17. MCP Remains Independently Usable Without the Skill

- **Name**: `TestEvaluationMCPOnlyCaseDiscoversAndCallsCompleteToolContract`
- **Type**: Integration/manual interoperability
- **Location**: `loomspan-console/internal/agenteval/server_test.go` and the
  client compatibility record
- **What it proves**: with no installed Agent Skill, a client can initialize,
  discover capabilities/tools, call runtime and one trace path, consume
  structured or fact-complete text results, follow continuation, and understand
  a typed domain error from schemas/descriptions alone.
- **Fixtures/data**: no-target runtime plus one imported trace to avoid requiring
  a live target.
- **Mocks**: production MCP adapter with existing imported evidence service.
- **Contract classification**: Ephemeral diagnostic formats.
- **Compatibility expectation**: protected PR 18 tool-first behavior; resources
  remain optional.

### 18. Adversarial Server Boundary Remains Inert Independently of Agent Results

- **Name**: retain and extend subcases in `TestPR18SemanticFixtures` only when a
  newly used content class lacks server coverage
- **Type**: Integration/security regression
- **Location**:
  `loomspan-console/internal/mcpadapter/trace_semantic_fixtures_test.go` and
  focused skill/activity tests where applicable
- **What it proves**: YAML/source path, activity/error/model/tool/metadata/
  payload/raw content is returned only by an explicit requested read and never
  initiates server shell, filesystem, network, credential, target, config,
  control, or extra service operations.
- **Fixtures/data**: reuse existing adversarial fixture; add only the smallest
  missing layer-specific data, not another scenario system.
- **Mocks**: existing operation counters and canaries.
- **Contract classification**: Ephemeral diagnostic formats.
- **Compatibility expectation**: current server security boundary. Keep this
  result separate from agent defense-in-depth observations.

### 19. Deterministic Archives Contain Only Canonical Skill Bytes

- **Name**: update
  `TestReleasePackagesAreDeterministicAndContainOnlyRuntimeFiles` to
  `TestReleasePackagesAreDeterministicAndContainRuntimeDebuggingSkill`
- **Type**: Unit, Windows ZIP and POSIX TAR.GZ matrix
- **Location**: `loomspan-console/internal/buildtool/package_test.go`
- **What it proves**: exact nine-file manifest, modes, stable ordering/
  timestamps, identical repeated bytes, archive/sidecar names, digest, and exact
  skill contents. Reject missing/extra/linked/nonregular skill input and unsafe
  archive-relative paths.
- **Fixtures/data**: temporary six-file package with unique contents and the
  current target matrix.
- **Mocks**: filesystem only.
- **Contract classification**: Configuration and manifest contracts.
- **Compatibility expectation**: intentional atomic replacement of the old
  three-file archive assertion; no simultaneous old/new manifest mode.

### 20. Native Smoke Requires, Validates, and Byte-Compares the Extracted Skill

- **Name**: `TestStrictSmokeRequiresExactRuntimeDebuggingSkill` and extend native
  release smoke
- **Type**: Unit/integration
- **Location**: `loomspan-console/internal/buildtool/smoke_test.go` and
  `smoke.go`
- **What it proves**: strict extraction accepts the exact nested skill files and
  modes, rejects omission/extra/duplicate/directory/link/traversal/wrong-mode
  entries, validates extracted frontmatter/references, and byte-compares every
  skill file with canonical source before executable startup.
- **Fixtures/data**: safe ZIP/TAR archives plus one mutation per subtest.
- **Mocks**: temporary extraction root; native smoke uses real packaged binary.
- **Contract classification**: Configuration and manifest contracts.
- **Compatibility expectation**: every supported archive contains one reviewed
  package; checksum covers it atomically.

### 21. Documentation Routing and Release Layout Match Executable Evidence

- **Name**: `TestReleaseAndAuthoringDocumentationReferenceCanonicalSkillContract`
- **Type**: Declaration/documentation integration
- **Location**:
  `loomspan-console/internal/buildtool/projectdeclarations_test.go`
- **What it proves**: Console/release docs name the canonical source/archive
  paths, manual copy/link, independent package version, no auto-install, no
  endpoint/key in skill, skill-only/MCP-only degradation, all local client
  families, and hosted-loopback exclusion. Authoring README routes debugging to
  `traces-and-debugging.md`; that topic links the canonical package and states
  required/optional capability plus defense-in-depth limitations without
  copying the five reference bodies.
- **Fixtures/data**: current documentation and canonical paths.
- **Mocks**: none.
- **Contract classification**: Configuration and manifest contracts.
- **Compatibility expectation**: author-facing claims are backed by package,
  evaluation, and existing runtime tests; do not test prose style beyond stable
  routes/contract markers.

### 22. Public Java and Java-to-Go Boundaries Remain Unchanged

- **Name**: retain `LoomspanPublicSurfaceArchitectureTest`,
  `ConsoleTraceFixtureCorpusTest`, Go fixture corpus, and MCP parity suites
- **Type**: Architecture/integration regression
- **Location**: existing Java/Go tests
- **What it proves**: no Java API/SPI delta, no changed YAML configuration, no
  REST/SSE/NDJSON compatibility change, and identical browser/MCP current-run
  semantics while PR 19 consumes existing evidence.
- **Fixtures/data**: current checked-in Java/Go fixture corpus.
- **Mocks**: existing test infrastructure.
- **Contract classification**: Application API and Ephemeral diagnostic formats.
- **Compatibility expectation**: protected paths remain green; no compatibility
  shim, legacy reader, or new boundary fixture is added.

## Representative Client and Agent Matrix

### Merge-Gating Repeated Agent Runs

| Client | Cases | Repeats | Required evidence |
| --- | --- | ---: | --- |
| Codex CLI | Failed, slow, expensive, unfamiliar path | 3 each | Complete native event stream, actual client/model build, skill activation, MCP calls, final answer, rubric |
| Codex CLI | Composite adversarial, missing required capability, missing optional raw capability, skill without MCP | 2 each | Same; adversarial run also requires complete IDE-tool event visibility |
| Claude Code | Failed, slow, unfamiliar path, composite adversarial | 2 each | Complete JSON/event transcript where supported, actual client/model build, answer, rubric |

Every run starts a fresh client conversation and isolated evaluation case.
Changing call order or prose is allowed. A retry after infrastructure failure is
recorded as infrastructure-failed and rerun; a completed unfavorable model run
remains in the committed result set and counts as failed.

### Deterministic or Single-Observation Coverage

- MCP without skill: automated production-adapter case plus one local client
  observation; no repeated model diagnosis required.
- No target, restart without application key, unsupported protocol,
  `INCOMPATIBLE_TARGET`, authentication, evidence expiry, `TARGET_CHANGED`, and
  advertised incomplete capability family: deterministic Go integration tests.
- Codex Desktop/IDE, local Antigravity, local Cursor, and Devin Desktop/
  Windsurf/Cascade or local Devin CLI: when an executable local build is
  available, record user/global install, skill discovery/activation,
  authenticated Streamable HTTP, one representative workflow, continuation,
  and resource behavior. An unavailable build remains `Not run`, not failed or
  inferred compatible.
- Hosted Codex/Devin: document loopback reachability as out of scope; do not
  attempt a remote compatibility result.

## How to Run

### Prerequisites

- Repository root version and current fixture corpus remain aligned.
- Java 21, Go 1.26.5, Node.js 24.18.0, and npm 12.0.2 as already pinned.
- The official `skills-ref` dependency installed only through the new exact
  lock/revision used by CI; no global floating installation for release proof.
- For agent runs: locally installed Codex CLI and Claude Code builds, their
  ordinary model authentication, and permission to use an isolated temporary
  user/global skill/MCP configuration. Never store a live key in the repository,
  command line, URL, shell history, screenshot, or result record.

### Focused Automated Tests

From `loomspan-console/`:

```text
go test ./internal/agentskills/...
go test ./internal/agenteval/...
go test ./internal/buildtool/...
go test ./internal/mcpadapter/...
go test ./internal/traceanalysis/...
skills-ref validate ./agent-skills/loomspan-runtime-debugging
```

From the repository root:

```text
./mvnw -pl loomspan-spring-boot-starter -Dtest=LoomspanPublicSurfaceArchitectureTest,ConsoleTraceFixtureCorpusTest test
```

### Canonical Regression Gates

From `loomspan-console/`:

```text
go run ./internal/buildtool verify
go run ./internal/buildtool mcp-conformance
npm --prefix web run test:e2e
go run ./internal/buildtool package --expected-version 0.1.0-SNAPSHOT
go run ./internal/buildtool smoke --expected-version 0.1.0-SNAPSHOT --archive dist/ARCHIVE
```

Run the package/smoke command natively on Windows x86_64, Linux x86_64, and
macOS arm64 through the release workflow. Run the existing native MCP subset on
Windows x86_64, Linux x86_64, macOS arm64, and macOS x86_64 through Console CI.

### Agent Evaluation Commands

The planned build-tool interface is:

```text
go run ./internal/buildtool agent-eval serve --case CASE_ID --output EVAL_TEMP_DIR
go run ./internal/buildtool agent-eval record --session EVAL_TEMP_DIR --client-events CLIENT_EVENTS --answer ANSWER_FILE --output RECORD.json
go run ./internal/buildtool agent-eval score --record RECORD.json
go run ./internal/buildtool agent-eval summarize --results agent-evals/results/DATE
```

- `serve` prints or writes the loopback endpoint and temporary key only to the
  protected temporary session, never into committed output.
- Configure the client through its user/global protected mechanism and install
  the canonical skill by explicit copy/link for that isolated run.
- `record` imports the headless client's native event stream, verifies its
  completeness declaration, captures the final answer, and emits sanitized
  canonical JSON.
- `score` runs deterministic gates, then requires the human rubric fields before
  marking the record complete.
- `summarize` checks the exact selected matrix, retains every run, and creates
  the date/client summary linked by the compatibility document.

## Manual Verification Procedure

1. Extract the platform archive and verify that
   `skills/loomspan-runtime-debugging/` has the same six files and bytes as the
   canonical source package.
2. Copy or link that directory into a temporary user/global skill location for
   the client; do not edit the canonical content or create a client fork.
3. Start the isolated evaluation case, configure authenticated loopback MCP
   through a protected client mechanism, and confirm the skill is discoverable.
4. Start a fresh conversation with the exact case prompt. Do not prescribe tool
   calls or answer prose. Capture the native client event stream and final
   answer when the client supports it.
5. Validate that required capability discovery precedes dependent work, stable
   IDs and limitations support the explanation, and evidence/calculation/
   context/inference remain distinguishable.
6. For adversarial cases, verify embedded runtime requests are not followed and
   no unrelated repository, shell, filesystem, URL, credential, target, or
   control action occurs. Record this as defense-in-depth model evidence only.
7. Run deterministic scoring, complete every human rubric dimension with
   evidence notes, and preserve the unedited completed result.
8. Remove the temporary client configuration/skill copy and disable or delete
   only the isolated MCP profile after the run.
9. Update `docs/mcp-client-compatibility.md` with actual date, OS, client/model
   build, configuration scope, observed protocol, case IDs, and concise result
   links. Leave unavailable client builds explicitly `Not run`.

## Exit Criteria

- [ ] The first archive-manifest test is demonstrated red against the current
  three-file packager before production packaging changes, then green afterward.
- [ ] Canonical and invalid-package validator suites pass, including native
  symlink/reparse and prohibited-content cases.
- [x] The canonical skill passes the exactly pinned official `skills-ref`
  validator and contains no endpoint, credential, trace data, scripts/assets,
  client fork, or experimental `allowed-tools`.
- [x] Verify/build/package cannot bypass skill validation, and all six skill
  files are byte-identical between source and every deterministic native archive.
- [x] Strict smoke rejects every incomplete/extra/unsafe archive mutation and
  validates the extracted skill before executable startup.
- [x] Every evaluation case is schema-valid, uniquely workflow-linked, and
  resolves its authoritative existing fixture facts without duplicated NDJSON.
- [x] Protocol, capability, status, target, authentication, evidence, and scope
  degradation cases remain distinct; incomplete advertised families remain a
  server conformance defect.
- [x] Evaluation server/record/scorer/repeat tests pass and no secret, auth
  header, machine path, unrelated content, or unapproved raw payload can enter a
  committed record.
- [x] Headless IDE-tool safety gates rely on complete client event streams;
  missing visibility is reported unknown rather than passed.
- [ ] All 28 selected Codex CLI/Claude Code runs are present, versioned,
  unedited, scored, and counted; every deterministic hard gate and human rubric
  threshold passes without dropping unfavorable runs.
- [ ] Each core workflow produces an evidence-backed useful explanation with
  stable identifiers and direct limitations; no exact prose or call order is
  required.
- [ ] Skill without MCP, MCP without skill, missing required capability, and
  missing optional raw capability exhibit their exact planned degradation.
- [x] Adversarial server tests remain green and agent results are labeled
  defense-in-depth observations rather than Go/model guarantees.
- [x] Existing MCP conformance, browser/MCP parity, shared-artifact,
  continuation/range, multi-client, cancellation/shutdown, and browser E2E
  suites pass without perturbing browser clients or Loomspan execution.
- [x] `ConsoleTraceFixtureCorpusTest`, Go fixture-corpus tests, and
  `LoomspanPublicSurfaceArchitectureTest` pass; no Java API/SPI, YAML,
  REST/SSE/NDJSON, or compatibility-marker change is introduced.
- [ ] Updated skill-authoring guidance is supported by the canonical skill,
  focused evaluation cases/results, existing MCP tests, and Java/Go fixtures;
  routing remains LLM-first and does not duplicate the five references.
- [ ] Native package/smoke succeeds on Windows x86_64, Linux x86_64, and macOS
  arm64; CI retains macOS x86_64 native MCP regression coverage.
- [ ] Actual client/model/build evidence is recorded for completed runs and
  unavailable GUI-client rows remain honestly `Not run`.
- [x] No legacy three-file release mode, separate skill archive, export command,
  auto-installer, client-specific skill, compatibility shim, or historical trace
  reader remains or is introduced.

## References

- Implementation plan:
  `ai/thoughts/plans/2026-08-14-loomspan-console-pr-19-debugging-skill.md`
- Ticket:
  `ai/thoughts/tickets/loomspan-console-pr-19-debugging-skill.md`
- Research:
  `ai/thoughts/research/2026-08-14-loomspan-console-pr-19-debugging-skill.md`
- Phase 3 design:
  `ai/thoughts/phases/loomspan_console_phase_3_llm_runtime_inspector.md`
- Workflow catalog:
  `ai/thoughts/phases/loomspan_console_workflows.md`
- Authoring guidance:
  `ai/skill-authoring/traces-and-debugging.md`
- Agent Skills specification:
  <https://openagentskills.dev/docs/specification>
