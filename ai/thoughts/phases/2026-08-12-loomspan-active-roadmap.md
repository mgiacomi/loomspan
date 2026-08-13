# Loomspan Active Roadmap

## Status

Active roadmap as of 2026-08-12. Completed phase plans and ticket briefs have
been removed from the working tree; Git history remains available when prior
implementation context is genuinely needed.

The immediate objective is to replatform Loomspan on Spring Boot 4, Spring
Framework 7, Spring AI 2, and Jackson 3 while using the change to reassess the
library architecture. The migration is not a compatibility exercise and does
not add product features.

## Migration charter

- Preserve the capabilities that make Loomspan useful, not its current
  implementation structure.
- Freeze product scope during the migration. Adopting a framework facility to
  replace existing Loomspan behavior is simplification, not feature expansion.
- Evaluate Spring AI 2 and Spring Boot 4 facilities for opportunities to delete
  or simplify Loomspan code.
- Preserve no API, configuration, serialization, or operational contract by
  default. Classify every affected contract as retained, redesigned, or removed
  according to whether it serves the target architecture and product.
- Prefer a direct current design over compatibility shims, deprecated bridges,
  or simultaneous Spring Boot 3/Spring Boot 4 and Spring AI 1/Spring AI 2
  support.
- Adopt Jackson 3 directly rather than making the deprecated Spring Boot
  Jackson 2 compatibility module part of the target design.
- Keep provider- and Spring AI-specific types within intentional integration
  boundaries.
- Do not recreate Spring AI behind a comprehensive Loomspan wrapper. Add a
  Loomspan abstraction only when Loomspan owns distinct semantics or the
  boundary materially improves testing and dependency isolation.
- Preserve behavioral characterization until an explicit architecture or
  contract decision supersedes it.
- Permit package moves, API removal, configuration changes, test rewrites,
  module restructuring, and deletion of obsolete code.
- Keep every merged revision buildable and internally coherent. Use one atomic
  migration change when independently merged intermediate states would be
  unsupported or would repeatedly churn the same seams.
- Defer genuinely new product capabilities until after the migration.

## Current state

- The project currently uses Spring Boot 3.5.11, Spring AI 1.1.6, and Jackson 2.
- The Phase 3 product design and developer workflow catalog remain active inputs:
  - [LLM Runtime Inspector](./loomspan_console_phase_3_llm_runtime_inspector.md)
  - [Developer Workflows](./loomspan_console_workflows.md)
- The platform research and resulting migration ticket are ready:
  - [Spring platform upgrade research](../research/2026-08-12-spring-platform-upgrade-research.md)
  - [PR 26 - Spring Boot 4 / Spring AI 2 platform migration](../tickets/loomspan-platform-pr-26-spring-boot-4-spring-ai-2.md)
- The remaining console work is represented by active ticket briefs:
  - [MCP authentication and lifecycle](../tickets/loomspan-console-pr-16-mcp-foundation.md)
  - [Runtime and live-inspection MCP surface](../tickets/loomspan-console-pr-17-mcp-runtime-inspection.md)
  - [Trace-inspection MCP surface](../tickets/loomspan-console-pr-18-mcp-trace-inspection.md)
  - [Portable debugging skill](../tickets/loomspan-console-pr-19-debugging-skill.md)
  - [Structured logging coverage](../tickets/loomspan-console-pr-20-structured-logging.md)

## Near-term sequence

```text
Spring platform research and architecture review
                |
                v
Target architecture and contract decisions
                |
                v
Migration ticket, implementation plan, and testing plan
                |
                v
Atomic Boot 4 / Spring AI 2 / Jackson 3 migration
                |
                v
PR 16 -> PR 17 -> PR 18 -> PR 19

PR 20 is independent and may be scheduled when it will not create conflicting
work in the same files.
```

Research selected one coherent migration ticket because splitting would require
unsupported platform combinations or repeated edits to dependency management,
auto-configuration, provider construction, ChatClient assembly, serialization,
and their tests. PR 26 uses ordered internal milestones and verification gates
to keep that atomic change reviewable.

## Research before ticketing

Research should produce one concise, evidence-backed artifact rather than a new
large phase specification. It must address the following areas.

### Spring AI 2 substitution audit

Evaluate relevant Spring AI 2 facilities as possible replacements for existing
Loomspan machinery. Classify each as **adopt**, **integrate**, **reject**, or
**defer**.

Initial candidates include:

- `ToolCallingAdvisor`, its automatic registration, and its ordering model;
- immutable chat-option builders and mutation semantics;
- official OpenAI and Anthropic SDK construction and HTTP customization;
- provider SDK retry controls and Spring Framework core retry support;
- `ChatClientBuilderConfigurer` for manually assembled clients;
- native structured-output support and JSON Schema facilities;
- advisor, model, tool-execution, and provider HTTP observations; and
- provider usage, finish-reason, tool-call, and diagnostic metadata.

Adoption is justified when it removes code, clarifies ownership, reduces
provider-specific behavior, or makes a brittle abstraction unnecessary. A new
framework capability that expands Loomspan behavior belongs in **defer**.

### Architecture review

Map the current responsibilities and decide their target ownership:

- mission and step execution;
- prompt and model-request construction;
- ChatClient and advisor assembly;
- tool definition, selection, execution, and lifecycle recording;
- provider client construction and connection configuration;
- physical provider attempts, retry policy, quotas, and failure translation;
- output-schema prompting, native enforcement, validation, and semantic retry;
- trace, journal, usage, metrics, and conventional observations;
- JSON, YAML, NDJSON, and HTTP serialization; and
- Spring Boot auto-configuration and public library surfaces.

The review should identify code to retain, reshape, replace, or delete. It
should not assume that an existing package, interface, or extension point must
survive merely because tests currently exercise it.

### Contract review

Classify every affected contract as **retain**, **redesign**, or **remove**, with
a short reason and the required producer, consumer, fixture, test, sample, and
documentation changes. Review at least:

- Java API and extension surfaces;
- YAML skill format and model/connection configuration;
- `loomspan.*` application properties;
- REST and SSE application boundaries;
- canonical NDJSON traces and console compatibility;
- metrics and observations;
- provider failure and retry behavior; and
- error types and messages relied upon by users or tests.

Exact compatibility is a decision, not a default. When a contract changes, all
in-repository producers and consumers should move together without a legacy
bridge unless a current product requirement justifies one.

## Migration completion criteria

The migration is complete when:

- Loomspan builds and tests on the selected Spring Boot 4, Spring AI 2, Spring
  Framework 7, and Jackson 3 versions;
- supported providers retain only the behavior intentionally selected during
  contract review;
- provider-attempt and retry ownership is explicit and has no hidden retry
  multiplier;
- tool-loop ownership and advisor ordering are explicit and verified;
- serialization behavior matches the retained or redesigned contracts;
- obsolete Spring AI 1, Boot 3, Jackson 2, and migration-only code is removed;
- tests depend on Loomspan-owned seams where appropriate instead of reproducing
  large third-party interfaces;
- samples and maintained documentation describe only the new platform; and
- no deferred Spring AI 2 capability has entered the product accidentally.

## Planning workflow

1. Perform focused source, dependency, and API experiments without changing the
   active implementation.
2. Record verified facts, substitution decisions, target ownership, contract
   decisions, and remaining risks in one research artifact.
3. Choose the smallest migration boundary that avoids unsupported intermediate
   states and repeated code churn.
4. Write the migration ticket from those decisions.
5. Create the detailed implementation and testing plans immediately before
   implementation.
6. Implement and review the migration as an architectural change, not as a
   mechanical version bump.
