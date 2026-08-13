# Loomspan Framework Feature Design Lens

## Purpose

This document is a lens for evaluating requested Loomspan framework features. It is intended to help maintainers decide whether a feature belongs in the framework and, if it does, what shape it should take.

It is not an architecture specification, implementation guide, product roadmap, or list of immutable rules. Its purpose is to make the project's values and goals explicit so that features that initially appear helpful are evaluated against their long-term effects on loomspan.

## Living Document

This is a living document. It should change as the framework matures, customers teach us more about real usage, or experience shows that one of these principles is incomplete or producing the wrong outcomes.

Updates should preserve the document's role as a judgment aid. A principle may be refined, added, or removed when there is a clear reason; the current text should not be treated as precedent that outweighs better evidence or reasoning.

These principles are strong defaults, not mechanical gates. A proposed feature may reasonably depart from one of them when the benefit and tradeoff are made explicit.

## Current Pre-1.0 Compatibility Posture

Until Loomspan adopts a version 1.0 compatibility policy, classify every affected framework surface before deciding whether to preserve or change it. Deliberately supported contracts are assessed and protected according to evidence. Configuration and manifest contracts are always assessed, and current-run diagnostic usefulness and security remain protected product goals.

A public modifier, interface, constructor, Spring bean, `@ConditionalOnMissingBean`, existing test, fixture, or previous implementation does not by itself establish a supported contract. These are evidence of technical exposure or existing behavior, not independent proof of a compatibility promise.

Use these categories consistently:

1. **Application API** — deliberately supported entry points used by ordinary application developers.
2. **Supported SPI** — deliberately supported customization or replacement points.
3. **Configuration and manifest contracts** — documented `loomspan.*` properties, YAML skill syntax, validation, defaults, and author-facing semantics.
4. **Persisted or serialized contracts** — formats deliberately intended for durable or cross-version use.
5. **Ephemeral diagnostic formats** — traces and related representations intended to debug and understand executions from the current implementation.
6. **Internal or accidentally exposed implementation** — runtime decomposition, wiring seams, implementation DTOs, constructors, beans, and behaviors not deliberately classified above.

Documentation, an explicit API/SPI allowlist, an approved ticket, and verified consumer usage are evidence of a supported contract. Record the evidence and the protected consumers rather than inferring support from visibility or replaceability.

For internal or accidentally exposed implementation, prefer one coherent design. Remove intentionally obsolete paths completely and update all in-repository callers, tests, samples, fixtures, configuration, manifests, and documentation atomically. Before adding an overload, alias, fallback, adapter, deprecated path, legacy reader, duplicate interface, compatibility constructor, bridge type, or dual behavior, identify the protected contract and explain why an atomic change is inappropriate. If temporary compatibility machinery is explicitly required, state its removal condition.

Documented configuration and manifest behavior is deliberate and must always receive an impact assessment. An intentional pre-1.0 break must be identified in the ticket and plan, explain the developer or skill-author impact, and update every in-repository configuration, manifest, fixture, sample, test, and guidance reference atomically. Prefer one coherent new contract; add migration machinery only when the ticket explicitly requires it.

Execution traces are debugging and execution-understanding tools, not historical analytics, archival, audit-history, or trend-analysis formats. Preserve diagnostic usefulness, accuracy, ordering, failure visibility, security boundaries, and sensitive-data redaction, and keep the current writer, reader, projector, and debugging tools coherent. Trace schemas, record types, field names, storage layout, and projection behavior may change to improve the current tool.

A complete canonical trace may be deliberately saved and opened as a portable diagnostic artifact only by a Console with the exact same `consoleCompatibilityVersion`. The trace carries that required framework-owned marker, and the external file is the durable object; opening it creates only ordinary transient Console evidence governed by the existing capacity, expiry, removal, shutdown, and restart-cleanup rules. This narrow same-version transfer does not create Console-owned history, restart adoption, historical discovery, or cross-version interchange. Derived indexes, analysis manifests, handles, continuations, and imported catalogs remain ephemeral current-process formats.

For resolved releases, reject a missing or unequal compatibility marker without inference or fallback. When both producer and consumer identify as `development`, normal complete validation may be attempted for developer testing, but compatibility between development builds is not promised and failure does not justify a fallback reader or repair path. Historical or cross-version readability remains unsupported; do not add legacy readers, schema migrations, version adapters, compatibility ranges, dual record formats, or historical compatibility fixtures unless a future ticket explicitly changes this policy. Update or remove obsolete trace fixtures and compatibility tests atomically.

## North Star

Loomspan should make production-grade hierarchical skill development understandable and dependable without hiding important behavior or burdening developers with orchestration plumbing.

We value:

- an intuitive mental model for skill developers;
- an obvious entry-point contract for developers running a skill;
- production safety, security, observability, and predictable boundaries;
- explicit, locally understandable behavior;
- a small number of durable concepts over many narrowly helpful conveniences;
- long-term framework coherence over short-term reductions in typing or sample code.

## Feature Design Principles

### 1. Design for capable production models

Loomspan should be designed for the capable, economical models customers are likely to use in production. Compatibility with small local models is valuable for learning and experimentation, but it is not an architectural target.

A framework feature should not be added solely to compensate for a small model's inability to follow instructions, preserve ordinary arguments, or use a clear tool contract. When model weakness exposes a problem, ask whether the problem remains with a highly capable planner. If it does not, improve examples, diagnostics, or model guidance before expanding the framework.

This does not mean trusting a capable model with responsibilities that belong to the runtime. Security decisions, trusted identity, authorization, isolation, deadlines, and other runtime invariants remain framework concerns regardless of model quality.

### 2. Optimize for understandable developer experiences

The ordinary developer running a skill should be able to understand how to call it from its entry-point contract. They should not need to understand Loomspan's internal planning or propagation mechanics to construct a valid request.

The skill developer should be able to understand a skill's inputs, dependencies, visibility, and important behavior by reading that skill's local contract. Understanding a leaf should not require reconstructing undocumented state established by distant ancestors.

Less typing is not necessarily greater simplicity. Prefer fewer concepts and clearer semantics over conveniences that reduce declarations while introducing hidden behavior.

### 3. Keep ordinary business input explicit

Business values such as ticket identifiers, scenario names, requested dates, and hostnames should normally travel through declared skill and tool inputs. Explicit inputs support validation, tracing, reuse, and comprehension.

Do not automatically inherit or merge parent inputs into child calls merely because keys share a name. Such behavior creates implicit coupling, ambiguous precedence, and possible data disclosure across skill boundaries.

Repeated forwarding across real skill trees is useful design evidence, but it is not by itself proof that inheritance belongs in the framework. Any propagation feature must remain understandable and valuable even with a capable planner.

### 4. Separate business input from trusted execution metadata

Some values describe the execution rather than the business request. Examples include authenticated identity, authorization claims, trusted tenant identity, correlation identifiers, deadlines, cancellation state, and request provenance.

The model should not be responsible for choosing, copying, or preserving trusted execution metadata. These values should come from authoritative runtime sources and should not become model-overridable merely for invocation convenience.

Avoid treating all such values as entries in an undifferentiated context map. Prefer narrow, named, typed concepts whose ownership, visibility, trust, and propagation rules are clear.

### 5. Prefer explicit dataflow over ambient behavior

A feature should make it possible to determine where a value came from, who supplied it, and who may change it. Hidden dependencies on global state, thread-local bags, caller history, or automatic name matching make skills harder to test, reuse, audit, and reason about.

Run-scoped framework facilities can be appropriate, but scope alone does not make ambient state safe or intuitive. Access should be deliberate, and capabilities should receive only the information they are entitled to use.

### 6. Default run-scoped state to immutability

Values established for a run should be immutable unless mutation is an essential and deliberately modeled part of the feature. This includes nested collections and objects, not only the outer container.

Avoid globally shared mutable state and general-purpose mutable session bags. They create order dependence, concurrency hazards, unclear ownership, difficult replay semantics, and accidental coupling between otherwise isolated skills.

Information that evolves during a mission should use an explicit framework concept appropriate to its meaning, such as tool results, evidence, plans, artifacts, or another deliberately designed state transition.

### 7. Preserve production boundaries

Convenience must not weaken:

- authorization and confused-deputy protections;
- tenant and resource isolation;
- input and output contracts;
- traceability and auditability;
- deterministic validation boundaries;
- replay and diagnostic fidelity;
- nesting, usage, timeout, and execution limits.

If a feature changes who supplies a value or where validation occurs, its security and trace semantics are part of the feature, not follow-up implementation details.

### 8. Prefer visible failure over silent repair

When a planner or caller violates a clear contract, a precise validation error and useful trace are often better than framework behavior that silently guesses, inherits, or repairs the request.

Recovery features are appropriate when the runtime has authoritative knowledge and unambiguous semantics. They are risky when they conceal a bad contract, invent intent, or make execution depend on implicit precedence rules.

### 9. Require evidence proportional to the abstraction

Samples are valuable customer proxies and should be taken seriously. Friction found while building them may reveal a real framework gap, even when the first proposed solution is not appropriate.

At the same time, one awkward sample should not automatically create a general abstraction. Consider:

- whether the same issue appears in multiple substantially different skill trees;
- whether production users are likely to encounter it;
- whether the issue persists with capable models;
- whether better contracts, diagnostics, or documentation address it;
- whether the proposed abstraction is smaller and clearer than the friction it removes.

### 10. Account for the long-term cost of helpfulness

Every public feature becomes part of the developer's mental model and the framework's compatibility, testing, security, and documentation surface.

Evaluate not only whether a feature helps its motivating case, but also what concepts, precedence rules, failure modes, and future constraints it introduces. Prefer a narrow feature with clear semantics over a flexible mechanism whose eventual uses and risks are difficult to bound.

## Feature Review Questions

Use the questions that are relevant to the proposal. They are prompts for discussion, not a scorecard.

### Framework responsibility

- Would this feature still provide meaningful value with a highly capable planner?
- Is it solving a framework responsibility or compensating for model weakness?
- Does the runtime possess authoritative information that the model or caller does not?
- Could a clearer contract, error, trace, sample, or prompt resolve the issue without a new abstraction?

### Developer understanding

- Can a developer understand how to invoke the entry skill from its contract?
- Can a skill developer understand the skill's dependencies locally?
- Does the feature remove orchestration plumbing from the public developer experience?
- Does it reduce the number of concepts, or merely the number of declarations?
- Is important behavior implicit, surprising, or dependent on a distant caller?

### Data and state

- Is the value business input, trusted execution metadata, or evolving mission state?
- Who creates the value, who owns it, and who may change it?
- Is it immutable for the run, including nested values?
- Is propagation explicit and bounded?
- Could the feature introduce name collisions, ambiguous precedence, order dependence, or cross-skill leakage?

### Production safeguards

- Can the model override a value it should not control?
- Are authorization and tenant boundaries preserved at execution time?
- Are validation and failure behavior deterministic and actionable?
- Can traces distinguish caller-, model-, and framework-supplied information?
- Are sensitive values redacted appropriately?
- Are nested, concurrent, asynchronous, cancellation, and replay behavior well defined?

### Evidence and cost

- How many distinct real or representative cases demonstrate the need?
- What is the smallest coherent feature that addresses them?
- What new public concepts and rules will developers have to learn?
- What ongoing compatibility, testing, documentation, and security obligations does it create?
- If we decline the feature, what concrete friction remains?

### Contract and compatibility

- Which affected surfaces are Application API, Supported SPI, Configuration and manifest contracts, Persisted or serialized contracts, Ephemeral diagnostic formats, or Internal or accidentally exposed implementation?
- What evidence establishes supported status, and what is only technical exposure or evidence of existing behavior?
- Which protected consumers exist, and which in-repository consumers must be updated atomically?
- What is the public-surface delta, including public signature types and Spring extension points?
- Which breaking changes are intentional, and why are they appropriate under the current lifecycle posture?
- Is the decision shim or no shim? If a shim is proposed, what protected contract requires it, why is atomic change inappropriate, and what is its removal condition?
- If the feature changes a cross-component protocol, which consumers, fixtures, tests, and documentation must be updated together? If that protocol has an independently managed compatibility marker, where is its change decision recorded?

## Applying the Lens

Feature research and plans should summarize the relevant tradeoffs rather than claiming that a proposal simply "follows" this document. At minimum, the analysis should identify:

1. the developer problem being solved;
2. why it is a framework responsibility;
3. the effect on the skill developer and the developer running the entry skill;
4. any new state, dataflow, trust, or mutability semantics;
5. safeguards that could be weakened or strengthened;
6. evidence that justifies the size of the proposed abstraction;
7. meaningful alternatives, including making no framework change;
8. the classification and supporting evidence for every affected contract surface;
9. intended breaking changes and the protected and in-repository consumers affected;
10. the public-surface delta, including signature and extension-point exposure;
11. an explicit shim/no-shim decision, with justification and a removal condition for any temporary mechanism.
12. for a versioned cross-component protocol, an explicit compatibility-marker decision with its semantic rationale.

When principles pull in different directions, document the tradeoff. The purpose of this lens is not to eliminate judgment; it is to make that judgment deliberate, consistent, and reviewable.
