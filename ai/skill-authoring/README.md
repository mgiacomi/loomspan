---
audience: loomspan-skill-builder
status: development
applies_to: current-repository-checkout
coverage: initial
---

# Loomspan Skill Authoring Knowledge Base

## Purpose

This directory is the AI-first knowledge base for an LLM that collaborates with a developer to design, build, review, and diagnose Loomspan skill trees.

Loomspan is under active development and has no production release yet. These documents describe the current repository checkout. They are deliberately incomplete: a missing topic must not be treated as either unsupported or fully understood.

The intended future consumer is a SkillBuilder application, potentially built with Loomspan itself. Until then, an LLM can use these documents directly while working in the repository.

## Authority and Verification

Use the following evidence order:

1. **This knowledge base** explains intended authoring semantics, design judgment, and recommended patterns.
2. **Focused tests** demonstrate behavior the repository deliberately protects.
3. **Valid and invalid fixtures** demonstrate accepted and rejected manifest shapes.
4. **Samples** demonstrate composition in representative applications.
5. **Production source** resolves exact runtime behavior and edge cases.

The executable framework remains authoritative for what the current checkout accepts and does. If this guide conflicts with tests or production code, do not silently choose one. Report the conflict, identify whether it affects authoring advice, and treat it as documentation drift or a possible framework defect.

Read [source-verification.md](source-verification.md) before performing a source-level investigation.

## How to Route an Authoring Task

| Developer need | Read first | Then read |
| --- | --- | --- |
| Understand Loomspan skill trees | [mental-model.md](mental-model.md) | Relevant topic documents below |
| Design or review a new tree | [checklists/evaluate-a-skill-design.md](checklists/evaluate-a-skill-design.md) | [mental-model.md](mental-model.md) |
| Add evidence-backed output claims | [evidence-contracts.md](evidence-contracts.md) | [mental-model.md](mental-model.md) |
| Select a model or configure its connection | [model-selection-and-connections.md](model-selection-and-connections.md) | [mental-model.md](mental-model.md) |
| Diagnose retries, usage, terminal failures, or a nested runtime path with the packaged Agent Skill | [traces-and-debugging.md](traces-and-debugging.md) | The relevant validation or evidence topic |
| Resolve ambiguity or an edge case | [source-verification.md](source-verification.md) | The topic's implementation anchors |

Do not load every document by default. Start with the routing entry most relevant to the developer's goal and expand only when the task crosses another documented concern.

## Coverage

| Topic | Coverage | Notes |
| --- | --- | --- |
| Skill-tree mental model | Initial, source-verified | YAML-only public identity, separate Java target registry, mapping, root invocation, visibility, and nesting |
| Public YAML skill identity | Source-verified | Exact `name` format, case sensitivity, no-rewrite policy, propagation, and separation from `mapping.target_id` |
| Evidence contracts | Source-verified | Immediate-root property annotations, strict scalar/placement rules, Boolean direct-child expressions, planning/final truth sets, metadata isolation, enforcement, and nested mission isolation |
| Source verification | Initial | How an LLM should use guide, tests, fixtures, samples, and production code together |
| Skill-design review | Initial | Cross-cutting questions; not a manifest validator |
| YAML manifest reference | Not yet documented | Inspect current manifest, catalog validation, tests, and samples when required |
| Input contracts | Foundational | The mental model covers strict mapped Java contract ownership; complete schema syntax and pure-YAML input behavior are not yet documented |
| Output contracts | Not yet documented | Evidence documentation covers only property-level supportability annotations and their interaction with candidate output |
| Prompts | Not yet documented | Private prompt composition needs a dedicated topic |
| Planning and nested planning | Foundational | The mental model covers choosing direct versus step-based execution; complete planning, retry, and cost semantics are not yet documented |
| Capability visibility and RBAC | Not yet documented | The mental model contains only foundational behavior |
| Attachments and virtual files | Not yet documented | Requires separate source verification |
| Model selection and connections | Initial, source-verified | Framework model aliases, named connections, drivers, thinking levels, migration, and diagnostics |
| Execution limits and quotas | Foundational | Trace guidance covers model-attempt and usage quota effects plus run-start diagnostic comparison; complete limit configuration remains undocumented |
| Traces and debugging | Source-verified | Independent finalized/acquired/imported discovery, actual-retry cardinality, compact/detailed frames with scalar close outcome, complete physical record-type histogram discovery, descriptor/inline/exact semantic reads, lossless byte-budgeted traversal, page-local search joins, usage/failures, optional raw forensics, and current-run limitations |
| Testing skill trees | Not yet documented | The design checklist gives initial review prompts only |

## Normative Language

These documents use:

- **MUST / MUST NOT** for behavior required by current framework validation, execution semantics, security, or an explicitly stated authoring invariant.
- **SHOULD / SHOULD NOT** for the recommended Loomspan authoring default.
- **MAY** for an optional supported choice.

Each document should distinguish enforced behavior from design guidance and known limitations. Do not present a recommendation as a runtime requirement.

## LLM-First Authoring Standard

The primary consumer of this knowledge base is an LLM collaborating with a developer, not a person reading the documentation from beginning to end. Optimize for correct retrieval, interpretation, and task execution. Human readability remains useful, but it is not the organizing goal.

- Lead with applicability, author-facing semantics, constraints, and required decisions. Add background only when it changes how the guidance should be applied.
- Preserve progressive disclosure. Keep routing in this README accurate, keep topic documents focused, and link to adjacent topics instead of repeating them.
- Make ordinary guidance self-contained enough for an LLM to act without source inspection. Use source anchors to verify or deepen the guidance, not as a substitute for explaining it.
- Use consistent framework terminology and explicit normative language. Clearly separate enforced behavior, recommended design defaults, optional choices, and known limitations.
- Prefer compact, structured representations such as decision tables, checklists, exact field names, and minimal examples when they reduce ambiguity. Do not add structure that merely restates the same content.
- Omit conversational introductions, rhetorical transitions, motivational prose, repeated summaries, and historical narrative that does not affect the current checkout.
- Keep examples minimal and evidence-backed. State when an example is partial, illustrative, invalid, or omits surrounding configuration.
- State unknowns, incomplete coverage, and conflicts explicitly so an LLM does not turn missing information into a claim.
- Prefer stable repository-relative references and named implementation anchors over volatile line-number citations.
- Optimize for signal and precision, not token reduction alone. Do not remove qualifications, edge cases, or distinctions needed to apply the guidance correctly.

Before accepting a change, verify that an LLM loading only the routed documents can determine:

1. When the guidance applies.
2. What is required, prohibited, recommended, and optional.
3. What evidence supports material behavioral claims.
4. What remains unknown, incomplete, or conflicting.
5. Which document to load next if the task crosses the current topic's boundary.

## Development Rules for This Knowledge Base

- Follow the LLM-first authoring standard above for every new or changed topic.
- Reuse tested fixtures and sample manifests instead of duplicating large examples when practical.
- Add a topic only after its current semantics have been researched and can be stated without speculation.
- Update the routing and coverage tables whenever a topic is added, its task boundary changes, or its confidence changes.

At the first production release, the repository tag should version this knowledge base with the corresponding source and tests.
