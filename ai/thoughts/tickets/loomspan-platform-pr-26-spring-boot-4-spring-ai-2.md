# PR 26 - Spring Boot 4 / Spring AI 2 Platform Migration

## Status

Proposed ticket brief. Follows completed PR 25 and precedes PRs 16-19. PR 20
may proceed independently only when it does not overlap files being changed by
this migration.

Research basis:
[Spring Boot 4 / Spring AI 2 / Jackson 3 Upgrade Research](../research/2026-08-12-spring-platform-upgrade-research.md).

## Fresh-context handoff

Use the repository's five-step development process for this ticket:

1. `ai/commands/1_research_codebase.md`
2. `ai/commands/2_create_plan.md`
3. `ai/commands/3_testing_plan.md`
4. `ai/commands/4_implement_plan.md`
5. `ai/commands/5_code_review.md`

Each fresh context must begin with this ticket and the linked research artifact.
Framework contract classification must also use
`ai/thoughts/framework-feature-design-lens.md`. The first step should produce a
fresh, as-is code and consumer map; it should reuse the completed platform
experiments rather than repeat them without a specific verification need.

The migration charter and architecture decisions in this ticket are settled
inputs. The detailed plan still owns concrete type names, package layout,
file-level sequencing, exact current patch versions, and the complete test
matrix. If live code or current framework behavior contradicts a settled input,
record the evidence and return for a ticket/plan decision instead of silently
adding compatibility machinery or changing scope.

PR 26's number reflects when this ticket was created, not its execution order.
It intentionally precedes active PRs 16-19. PR 25 is complete. PR 20 may run
independently only when file-level ownership does not overlap this migration.
Planning artifacts may be prepared in the current planning worktree, but
implementation must use an isolated branch/worktree based on the intended
integration commit and must not absorb unrelated in-progress changes.

The research experiments were disposable and lived under ignored Maven
`target` output. Their recorded results are evidence for planning; durable
versions of relevant probes must be implemented as repository tests where the
testing plan requires them.

## Outcome

Replatform Loomspan directly on Spring Boot 4.1.x, Spring Framework 7, Spring
AI 2.0.x, Java 21, and Jackson 3 while simplifying the AI integration
architecture. Preserve Loomspan's useful capabilities, not obsolete framework
coupling or compatibility contracts.

Deliver this as one atomic implementation PR with reviewable internal
milestones. The milestones are not separately supported platform states and
should not become merge-dependent tickets that repeatedly change the same
provider, chat-client, advisor, and serialization seams.

## Architecture decisions

- Introduce one narrow Loomspan model-interaction boundary used by mission,
  planning, and step orchestration. Keep Spring AI requests, responses,
  clients, messages, metadata, and tool callbacks inside its implementation.
- Do not build a general Loomspan facade over Spring AI. Loomspan-owned types
  exist only for Loomspan semantics or useful dependency isolation.
- Adopt Spring AI 2 `ToolCallingAdvisor` as the generic tool loop while
  retaining Loomspan ownership of capability discovery and access, execution,
  quotas, retry decisions, semantic validation, traces, and usage.
- Keep one Loomspan logical/physical attempt policy. Configure every provider
  SDK or framework retry layer for exactly one HTTP send per Loomspan attempt.
- Build OpenAI and Anthropic integrations with their official SDK clients. Use
  Spring Framework 7's zero-retry core template for Gemini and Ollama.
- Keep the OpenRouter compatibility profile and reject its HTTP-200 error
  completion before normal response decoding.
- Do not apply global `ChatClientBuilderConfigurer` customizers implicitly.
  Assemble Loomspan clients with the selected advisor and observation
  components explicitly.
- Replace Spring AI `@ToolParam` in Loomspan's application-facing skill API
  with a Loomspan-owned `@SkillParam` carrying description and requiredness.
- Use Jackson 3 throughout Loomspan and do not add Boot's Jackson 2
  compatibility module. Jackson 2 pulled privately by an official provider SDK
  is acceptable; direct Loomspan Jackson 2 dependencies and API usage are not.
- Retain canonical trace and REST/SSE behavior unless implementation evidence
  exposes a concrete conflict. Other affected contracts may be retained,
  redesigned, or removed without a compatibility bridge.

## In scope

- Upgrade dependency management, compiler baseline, renamed Boot modules, and
  Boot 4 auto-configuration/test imports in both Maven modules.
- Characterize and then migrate JSON, YAML, NDJSON, Java-time, omission,
  ordering, and unknown-field behavior to purpose-owned Jackson 3 codecs.
- Replace the versioned Spring AI 1 integration and removed provider APIs with
  the target AI execution boundary and AI 2 provider construction.
- Redesign provider connection properties around supported SDK fields. Retain
  base URL, API key, general headers, OpenAI organization/project, and the
  OpenRouter profile. Remove OpenAI/Anthropic completion-path properties and
  the typed Anthropic version override.
- Replace built-value chat-option adaptation with contributions compatible
  with AI 2's immutable option builders.
- Assemble and verify semantic validation, tool calling, physical attempts,
  observations, usage extraction, failure translation, and trace recording in
  their intentional scopes.
- Replace `@ToolParam` usage in the processor, public examples, samples, tests,
  and maintained documentation with `@SkillParam`.
- Split provider/chat/Jackson integration wiring from stable runtime service
  composition where that reduces the current auto-configuration coupling.
- Delete obsolete Spring AI 1, Boot 3, Spring Retry, Loomspan Jackson 2,
  compatibility, fluent test-double, configuration, and documentation code.

## Ordered implementation milestones

1. Freeze retained contracts with focused fixtures and exact attempt/tool-loop
   counts; record any contract redesign discovered during implementation.
2. Move the dependency and Boot module baseline and migrate the named Jackson
   codec roles, keeping retained fixtures green.
3. Establish the narrow model-interaction and capability-description boundary
   and stop orchestration code from carrying Spring AI integration types.
4. Rebuild all provider clients, connection translation, one-send controls,
   OpenRouter handling, and normalized failure mapping.
5. Assemble the AI 2 options/advisor/tool chain and prove recursive ordering,
   semantic retry, quotas, usage, and trace behavior.
6. Complete Boot auto-configuration, Web MVC/security tests, observations,
   sample application, configuration metadata, and maintained documentation.
7. Remove migration residue and enforce the target dependency and package
   boundaries before the full verification run.

These milestones are the working sequence for the detailed implementation
plan. They may be represented by reviewable commits, but the PR is complete
only when all milestones pass together.

## Acceptance signals

- The reactor builds and all tests pass on the selected Boot 4.1.x, Spring AI
  2.0.x, Framework 7, Java 21, and Jackson 3 baseline.
- Architecture tests prevent Spring AI integration types from leaking back
  into mission, planning, and step orchestration.
- OpenAI, Anthropic, Gemini, and Ollama request tests prove one physical HTTP
  send per Loomspan attempt; Loomspan retry tests prove the exact total sends,
  quota effects, attempt events, and terminal failure category.
- OpenRouter HTTP-200 error completions are rejected with bounded safe
  diagnostics and cannot surface partial content as success.
- Advisor integration tests prove semantic policy outside the AI 2 tool loop,
  Loomspan physical attempts inside it, and exactly one selected tool advisor.
- Skill discovery, schema generation, requiredness, descriptions, binding, and
  invocation pass using `@SkillParam` without application-facing `@ToolParam`.
- Golden JSON/YAML/NDJSON and HTTP fixtures pass for every retained contract,
  including time, null/default, ordering, numeric, and unknown-field cases.
- Conventional Spring AI observations integrate without duplicating Loomspan
  domain counters, traces, quota accounting, or sensitive-content defaults.
- No production code imports Spring AI 1-only APIs, Spring Retry, or Jackson 2
  APIs; no direct Jackson 2 dependency or Boot compatibility module remains.
- The sample starts and its representative skill, planning, provider, REST,
  SSE, trace, and console-facing flows pass on the new platform.
- Maintained documentation and generated configuration metadata describe only
  the new platform and surviving contracts.

## Detailed-planning focus

Immediately before implementation, turn the milestones into a file-level plan
and test matrix. Pin the then-current Boot 4.1.x and Spring AI 2.0.x patch
versions; enumerate each affected property and public signature; identify every
producer, consumer, fixture, sample, and document for redesigned contracts;
and define the exact commands and provider HTTP fixtures used at each gate.

Any newly illuminated contract or architecture problem should be decided in
the plan as **retain**, **redesign**, or **remove**. Do not preserve it merely to
reduce the migration diff.

## Guardrails

- No Boot 3/4, Spring AI 1/2, or Jackson 2/3 dual-support layer.
- No deprecated compatibility bridge without a current product requirement.
- No feature expansion under the label of migration; structured model output,
  tool search, and other new agent capabilities remain deferred.
- No provider retry beneath a counted Loomspan attempt.
- No broad wrapper that recreates Spring AI APIs in Loomspan types.
- No rewrite of canonical trace or external web contracts without an explicit
  producer/consumer decision and updated fixtures.
- Keep PRs 16-20 out of this change except for unavoidable conflict resolution
  or updates required because this ticket intentionally changes their active
  design assumptions.

## Out of scope

PRs 16-20 implementation, new providers, native structured-output adoption,
tool search, new mission behavior, new console capabilities, dual-platform
support, and compatibility guarantees for contracts removed by an explicit
migration decision.
