---
audience: loomspan-skill-builder
status: development
applies_to: current-repository-checkout
coverage: initial
---

# Loomspan Skill-Tree Mental Model

## Purpose

Use this document to establish shared vocabulary and choose the broad shape of a Loomspan skill tree. It is not a complete YAML manifest reference.

## Core Model

Loomspan combines public model-driven YAML skills with internal deterministic Spring implementation targets. A skill tree is a hierarchy of YAML capability boundaries, not a static rule tree: an LLM-backed skill reasons about its mission and can invoke only the child YAML capabilities exposed to it.

The normal shape is:

```text
Application code
    -> entry YAML skill
        -> LLM-backed planning or reasoning skill
            -> nested YAML specialist
                -> mapped YAML leaf
                    -> deterministic Java @SkillMethod
```

Not every tree needs every level. A simple skill may be one LLM-backed YAML skill, and a shallow workflow may have one planner with deterministic leaves.

## Public YAML Skill Identity

The YAML manifest `name` is the single public identity for both LLM-backed and mapped skills. It MUST match `^[A-Za-z_][A-Za-z0-9_]{0,63}$`: 1-64 characters, beginning with an ASCII letter or underscore, followed only by ASCII letters, digits, or underscores. Names are case-sensitive. Loomspan validates the exact parsed value during catalog loading and does not trim, sanitize, normalize, truncate, alias, or translate it.

Descriptive lowerCamelCase, such as `expenseLookup`, is the recommended repository style, not an additional runtime restriction. Valid alternatives include `expense_lookup`, `_internalStyleAllowed`, and `Skill2`.

Use the exact validated YAML name for catalog and registry lookup, `SkillTemplate` invocation, `allowed_skills`, plan capability targets, evidence-expression references, metrics, journals, traces, and provider-facing tool definitions. These surfaces share one identity rather than maintaining provider-specific aliases.

`mapping.target_id` belongs to a separate internal namespace. Its `beanName#methodName` syntax is intentionally not valid as a public YAML name and is not governed by the public-name validator.

## Capability Types

### Entry YAML skill

An entry skill is the YAML skill invoked by application code through `SkillTemplate`.

Current behavior:

- `SkillTemplate` accepts a YAML skill name and an object or map input.
- The root input is normalized and validated against the skill's effective input contract before execution.
- Every invocation receives a new Loomspan session.
- `SkillTemplate` does not provide the supported public entry path for a raw Java capability; expose Java behavior through a mapped YAML skill when it must be an entry skill.

The application developer SHOULD be able to determine how to invoke an entry skill from that skill's input contract without understanding the tree below it.

### LLM-backed YAML skill

A YAML skill that omits the `mapping` block is model-driven.

It may:

- receive structured mission input;
- use its `description` as public capability information;
- use an optional private `prompt` for its own reasoning behavior;
- call child capabilities exposed through `allowed_skills`;
- declare input, output, evidence, linter, model, RBAC, and execution settings supported by the current manifest;
- enable the step-based planning executor by explicitly setting `planning_mode: true`.

An LLM-backed skill does not automatically see every registered capability. Its tool surface is governed by its local `allowed_skills` and runtime access checks.

### Mapped YAML skill

A mapped YAML skill declares `mapping.target_id` and delegates execution to a registered Java `@SkillMethod` target.

The YAML capability supplies the tree-facing name, description, and access policy. Java supplies the reflected input contract and deterministic returned-value behavior. A mapped wrapper cannot declare a YAML `output_schema`; the direct Java route does not enforce one.

A mapped YAML skill:

- is still registered as a YAML capability;
- can be invoked by `SkillTemplate` using its YAML name;
- can appear in another YAML skill's `allowed_skills`;
- invokes its Java target instead of an LLM;
- MUST declare `name`, `description`, and a nonblank `mapping.target_id`;
- MAY additionally declare only `rbac_roles`;
- MUST omit model/runtime fields, including `model`, `thinking_level`, `prompt`, schemas, planning, nested-tool selection, linting, retries, and evidence contracts;
- inherits its input and returned-value behavior from the Java target.

Use a mapped YAML skill when deterministic Java behavior must participate in the YAML skill tree or be exposed as an entry skill.

#### Prefer reflected input-contract inheritance for mapped skills

Omitting `input_schema` from a mapped YAML skill is an explicit authoring choice, not an absence of validation. Loomspan derives the effective public input contract from the mapped Java target's reflected tool schema. The generated tool descriptor and entry-input validation use that inherited contract.

The Java method signature and parameter metadata therefore MUST describe the input that callers and parent planners are allowed to supply. Verify requiredness, field names, types, descriptions, nested shapes, and runtime markers at the Java boundary rather than assuming reflection will express the intended public contract.

For the exact reflected meanings of Java `Object`, generic and typed maps, DTOs, arrays, requiredness, and JSON null, read [input-contracts.md](input-contracts.md). In particular, `Object` and `Map<String, Object>` deliberately permit heterogeneous JSON-compatible values; use a typed record or DTO when the planner needs discoverable stable fields.

A mapped skill MUST omit `input_schema` and `output_schema`. Do not copy a schema into a mapped wrapper merely to repeat reflected fields. Loomspan rejects those declarations so Java remains the single authoritative contract source.

If two public mapped capabilities need genuinely different input shapes, use separate deterministic Java adapter targets so transformation and validation remain explicit and testable.

This inheritance applies only across an explicit YAML-to-Java mapping. It does not imply automatic inheritance of business inputs between parent and child YAML missions.

Mapping classification is syntactic. Omitting `mapping` selects LLM-backed validation; declaring it requires a nonblank `mapping.target_id`, even when the block or target was explicitly null or blank. Likewise, false, zero, empty, blank, and null model/runtime declarations are still forbidden declarations on a mapped wrapper.

### Java `@SkillMethod`

`@SkillMethod` registers deterministic Spring method behavior as an internal implementation target keyed by `beanName#methodName`. It does not declare a public name or enter the public `CapabilityRegistry`; the reflected method signature supplies the target schema and input contract.

Annotated method names MUST be unique within one Spring bean because overloads would produce the same target ID. Multiple independently named and governed YAML skills MAY map to one target. Spring proxy, interface, and bridge methods are canonicalized, and invocation resolves the final bean by name so configured advice remains active.

`CapabilityRegistry`, `SkillTemplate`, and `allowed_skills` expose YAML names only. Wrap a Java method with a mapped YAML skill whenever application code or an LLM-backed skill must call it. A target ID is trusted mapping/diagnostic metadata, never a public invocation alias.

Java leaves SHOULD own operations whose correctness should not depend on model reasoning, such as database lookups, calculations, controlled API calls, policy enforcement, and stable fixture access.

Use Loomspan's application-owned `@SkillParam` on Java parameters when the reflected contract needs a description or optionality. Description defaults to empty and `required` defaults to `true`; set `required = false` for inputs the model or caller may omit. An optional parameter MUST use a nullable reference type such as `Integer`, not a primitive such as `int`, because omission binds `null`; Loomspan rejects optional primitive declarations during target discovery. Do not use Spring AI's parameter annotation at this boundary.

```java
@SkillMethod(description = "Look up one account.")
Account lookup(
        @SkillParam(description = "Stable account identifier.") String accountId,
        @SkillParam(description = "Optional region hint.", required = false) String region) {
    // deterministic application logic
}
```

## Root, Planner, Specialist, and Leaf Are Roles

These words describe how a capability is used, not distinct framework classes.

- **Root or entry skill:** invoked by application code.
- **Planner:** an LLM-backed YAML skill with `planning_mode: true` that creates and executes a bounded task plan.
- **Specialist:** a child YAML skill responsible for a narrower mission. It may itself be a planner.
- **Leaf:** a capability that performs work without further decomposition, commonly a mapped deterministic Java capability.
- **Synthesis skill:** a skill whose primary responsibility is composing evidence and results into the final output.

A YAML skill can be a root in one tree and a nested specialist in another if its contract and visibility boundaries make sense in both contexts.

## Visibility Is Local

`allowed_skills` defines the candidate child YAML capabilities for one LLM-backed skill. It is not transitive.

If a root allows `investigateNetwork`, and `investigateNetwork` allows `checkDns`, the root sees `investigateNetwork`; it does not automatically see `checkDns`.

Runtime authorization filters the declared tool surface again. Visibility is not authorization by itself, and authorization is enforced at execution as well as discovery.

A skill SHOULD expose only the capabilities needed for its responsibility. Narrow tool surfaces improve security, make plans easier to understand, and preserve HTN boundaries.

## Nested Execution Preserves Capability Boundaries

When an LLM-backed YAML skill invokes another LLM-backed YAML skill:

- the child receives the arguments supplied for the child contract;
- the child opens its own mission frame inside the current session;
- the child uses its own model, prompt, allowed skills, plan, and property-level output evidence requirements;
- the parent's plan and successful-direct-skill set are saved and restored around the child mission;
- the parent observes the child capability result, not the child's internal tool surface or evidence ledger.

This isolation is intentional. A parent contract should describe the child capability it invokes rather than coupling itself to the child's internal leaves.

## Choose Direct Reasoning or Step-Based Planning Deliberately

The current execution coordinator selects the step-based planning executor only when the YAML manifest explicitly declares:

```yaml
planning_mode: true
```

Do not assume that a global planning default turns an undeclared skill into a step-loop planner in the current implementation.

An LLM-backed YAML skill without `planning_mode: true` uses direct mission execution. This remains true when the skill is nested. A nested specialist therefore does not need to become a planner merely because it is part of a deeper tree.

Use direct mission execution when one focused model-directed mission interaction is appropriate and the skill does not need Loomspan's explicit plan-and-step lifecycle. The direct executor still receives the skill's visible tools, so direct execution does not mean that the skill must have no children. It means Loomspan does not first create a task plan and then ask the model for one bounded step action at a time.

Direct execution is the normal choice for a focused specialist that does not benefit from an explicit plan. Do not interpret "direct" as a guarantee of exactly one physical provider request: tool-calling protocols, advisors, and provider behavior may involve additional internal interactions.

Use step-based planning when the skill genuinely needs dynamic decomposition, selective child-capability use, or multi-step progress toward its result. Planning adds an initial plan-model interaction and a bounded model-driven step loop. A planning step may invoke a nested skill, and that child independently chooses direct or planning execution from its own manifest.

Each additional planning boundary increases potential model calls, latency, validation retries, and failure locations. Nesting itself is not a reason to avoid a useful planner, but every planning level SHOULD provide a meaningful mission boundary and decomposition benefit.

Do not select direct execution merely to conceal that a mission needs decomposition, and do not select planning merely because it appears more capable. Choose the least complex execution semantics that truthfully fit the skill's responsibility.

A planner SHOULD have:

- a narrow mission;
- a bounded `max_steps` appropriate to the mission;
- a deliberate `allowed_skills` surface;
- explicit input and output contracts when callers depend on structured behavior;
- property-level `evidence` requirements only on immediate root output claims that need deterministic supportability enforcement.

A direct specialist SHOULD have:

- one focused reasoning or synthesis responsibility;
- a contract it can fulfill without an explicit, framework-managed task plan and step loop;
- a narrow `allowed_skills` surface if direct tool or child-skill use is part of that responsibility;
- explicit input and output contracts when callers depend on structured behavior;
- model and validation settings appropriate to that responsibility.

Sample claims about model compatibility MUST be scoped to the sample and configuration actually tested. Success on a shallow direct skill does not establish that the same model can reliably execute a multi-level planning tree. Loomspan skill architecture SHOULD target capable production models; small local models may be useful for experimentation but are not a reason to weaken planning semantics or runtime safeguards.

## Inputs, Runtime Metadata, and Evolving State

Keep these categories distinct when designing a tree:

- **Business input:** mission data such as ticket text, identifiers, dates, requested actions, and scenario names. It normally travels through declared skill and tool inputs.
- **Trusted execution metadata:** authoritative identity, authorization, tenant, correlation, deadline, or provenance information. It should not become model-controlled merely for propagation convenience. No general framework feature for arbitrary trusted metadata is documented here.
- **Evolving mission state:** plans, tool results, evidence, artifacts, and trace records. Use the framework concept appropriate to the information rather than inventing an ambient mutable bag.

Do not automatically treat repeated business inputs as runtime metadata. Explicit contracts preserve local comprehension and traceability.

## Initial Design Heuristics

- Keep the entry contract obvious to application developers.
- Give each skill one coherent responsibility.
- Use the model for decomposition, selection, interpretation, and synthesis.
- Use deterministic Java leaves for controlled side effects and operations with programmatic correctness rules.
- Keep child visibility local and narrow.
- Preserve child abstraction boundaries; do not make parents depend on internal leaves.
- Prefer explicit business inputs over implicit inheritance.
- Add contracts when the framework can enforce a meaningful invariant, not merely to decorate a manifest.
- Validate a proposed shape against representative successful and failure branches.

## Implementation Anchors

Use these only when current behavior or an edge case needs verification:

- [`SkillTemplate.java`](../../loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/api/SkillTemplate.java) defines the public invocation surface.
- [`DefaultSkillTemplate.java`](../../loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/skillapi/DefaultSkillTemplate.java) validates YAML-only entry invocation and creates sessions.
- [`SkillMethod.java`](../../loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/api/SkillMethod.java) defines the deterministic Java target annotation.
- [`SkillParam.java`](../../loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/api/SkillParam.java), `SkillParamTest`, `SkillMethodBeanPostProcessorTest`, and `SkillMethodTargetDiscoveryIntegrationTests` define and protect parameter description, requiredness, schema, binding, proxy, interface, and bridge behavior.
- [`YamlSkillManifest.java`](../../loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/skill/YamlSkillManifest.java) defines the accepted manifest object shape.
- [`YamlSkillCatalog.java`](../../loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/skill/YamlSkillCatalog.java) validates the exact public YAML name before other manifest-specific validation and stores definitions by that identity.
- [`YamlSkillDefinition.java`](../../loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/skill/YamlSkillDefinition.java) exposes normalized YAML skill settings.
- [`YamlSkillCapabilityRegistrar.java`](../../loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/skill/YamlSkillCapabilityRegistrar.java) registers pure and mapped YAML capabilities.
- [`SkillImplementationTarget.java`](../../loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/core/SkillImplementationTarget.java) defines internal Java target metadata without provider-facing tool identity.
- [`SkillImplementationTargetRegistry.java`](../../loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/core/SkillImplementationTargetRegistry.java) defines the internal `beanName#methodName` registry boundary.
- [`SkillMethodBeanPostProcessor.java`](../../loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/core/SkillMethodBeanPostProcessor.java) discovers canonical annotated methods and builds proxy-safe target invokers.
- [`YamlSkillCapabilityRegistrarTests.java`](../../loomspan-spring-boot-starter/src/test/java/com/lokiscale/loomspan/internal/skill/YamlSkillCapabilityRegistrarTests.java) covers equal YAML/Java names, shared targets, contract inheritance, advice, errors, and public metadata.
- [`YamlSkillCatalogTests.java`](../../loomspan-spring-boot-starter/src/test/java/com/lokiscale/loomspan/internal/skill/YamlSkillCatalogTests.java) covers `acceptsProviderPortablePublicSkillNames`, `rejectsNonPortablePublicSkillNames`, required-field ordering, and mapped-manifest validation ordering.
- [`SkillInputContractResolver.java`](../../loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/runtime/input/SkillInputContractResolver.java) selects explicit YAML, inherited Java, or generic input contracts.
- [`DefaultSkillVisibilityResolver.java`](../../loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/skill/DefaultSkillVisibilityResolver.java) defines the current local YAML child surface and access filtering.
- [`CapabilityExecutionRouter.java`](../../loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/core/CapabilityExecutionRouter.java) distinguishes nested LLM-backed YAML execution from mapped/Java invocation and preserves parent state.
- [`ExecutionCoordinator.java`](../../loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/core/ExecutionCoordinator.java) selects the execution engine and constructs each YAML mission boundary.
- [`CapabilityExecutionRouterTest.java`](../../loomspan-spring-boot-starter/src/test/java/com/lokiscale/loomspan/internal/core/CapabilityExecutionRouterTest.java) covers nested routing, authorization fallback, plan restoration, successful-skill isolation, and canonical mission input.
- [`SupportedSurfaceIntegrationTest.java`](../../loomspan-spring-boot-starter/src/test/java/com/lokiscale/loomspan/integration/SupportedSurfaceIntegrationTest.java) proves that an LLM-backed YAML entry skill can call a mapped `@SkillMethod`/`@SkillParam` leaf through `SkillTemplate` without replacing internal Loomspan infrastructure.

## Coverage Boundary

This document does not define every manifest field or the complete behavior of planning, inputs, outputs, RBAC, attachments, retries, quotas, or traces. Consult [README.md](README.md) for current topic coverage before advising on those areas.
