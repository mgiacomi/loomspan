---
date: 2026-08-20T15:30:07-07:00
researcher: Codex
research_model: GPT-5
git_commit: 2beb407df8c390ffe568706adce2707540f6a751
branch: main
repository: loomspan
topic: "PR 32 — Faithful Unconstrained Java Input Contracts for Reliable Tool Calls"
tags: [research, codebase, java-reflection, input-contracts, json-schema, mapped-skills]
status: complete
last_updated: 2026-08-20
last_updated_by: Codex
---

# Research: PR 32 — Faithful Unconstrained Java Input Contracts for Reliable Tool Calls

**Date**: 2026-08-20 15:30:07 PDT
**Researcher**: Codex (GPT-5)
**Git Commit**: `2beb407df8c390ffe568706adce2707540f6a751`
**Branch**: `main`
**Repository**: `loomspan`

## Research Question

Research the current post-PR-31 codebase for the PR-32 ticket concerning faithful Java `Object` and generic-map input semantics. Document the existing reflected schema, internal contract, prompt, validation, mapped-skill, direct-entry, Java-binding, test, documentation, and compatibility surfaces without proposing an implementation.

## Summary

The current checkout builds one reflected JSON Schema for each `@SkillMethod`, parses that schema into an internal `SkillInputContract`, and carries both forms through mapped YAML registration. The provider-facing descriptor uses the reflected JSON text, while prompt rendering and runtime validation use the parsed internal contract. Planner calls are contract-validated before side effects and again at capability routing; direct `SkillTemplate` calls are also validated before routing. Java invocation then binds the normalized values with the application `ObjectMapper`.

For a map, `LoomspanMethodInputSchemaGenerator` emits `type: object` and sets `additionalProperties` to the generated value-type schema. The same generator emits `type: object` for both `Object.class` and an already-visiting Java type. Therefore `List<Map<String, Object>>` is represented as an array of objects whose arbitrary entry values each have `type: object`. `SkillInputContractResolver` retains that nested schema as `additionalPropertiesSchema`; `SkillInputValidator` applies it to every unknown map entry and reports `Expected object input.` for scalar values. `SkillInputPromptRenderer` renders an object placeholder and, in verbose mode, says each map value "must be a object." These are separate consumers of the same current classification.

The travel sample's Java method accepts `List<Map<String, Object>>`; its parameter description and parent planner prompt show compact scalar-valued option maps. Its direct unit test successfully ranks native scalar-valued maps because it invokes the service method directly, outside Loomspan's reflected-contract validation path. The mapped `rankTransportOptions` YAML manifest intentionally has no `input_schema`, so registration inherits both schema and internal contract from `travelCatalogService#rankTransportOptions`.

The imported trace identifiers occur only in the ticket. No trace artifact or other repository evidence for that execution is present. The repository proves the current schema and validation mechanics, while the reported trace remains supplied historical evidence rather than authenticated provenance for this checkout.

## Detailed Findings

### 1. Reflected Java schema generation

- `LoomspanMethodInputSchemaGenerator.generate` always creates a strict top-level object, reflects every Java method parameter into `properties`, marks every parameter required initially, and sets top-level `additionalProperties` to `false` (`loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/serialization/LoomspanMethodInputSchemaGenerator.java:30-46`).
- Collections and arrays become `type: array` with recursively generated `items`; maps become `type: object` with recursively generated `additionalProperties` (`LoomspanMethodInputSchemaGenerator.java:54-68`).
- Boolean, integer, number, enum, and string-like Java types have explicit scalar mappings (`LoomspanMethodInputSchemaGenerator.java:69-97`). Typed maps consequently retain their value type at schema generation time.
- `Object.class` and failure to add the raw class to the active `visiting` set share one branch and both emit `{"type":"object"}` (`LoomspanMethodInputSchemaGenerator.java:99-102`). The latter condition is the current recursion bound.
- Other DTO-like values become strict object schemas built from Jackson deserialization properties. Every discovered property is marked required, `additionalProperties` is false, and the raw class is removed from `visiting` after expansion (`LoomspanMethodInputSchemaGenerator.java:103-120`).
- For `List<Map<String, Object>>`, those rules produce the effective nested shape `array -> object -> additionalProperties(type=object)`. The generated `Object` leaf has no declared properties or `additionalProperties` keyword, but it still asserts the JSON object type.

### 2. Schema publication and Java target registration

- `SkillMethodBeanPostProcessor` owns a `LoomspanMethodInputSchemaGenerator` and the shared `SkillInputContractResolver` (`loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/core/SkillMethodBeanPostProcessor.java:47-52,102-114`).
- During target registration it builds the input schema once, stores that JSON text on `SkillImplementationTarget`, and resolves the same text as a `JAVA_REFLECTED` internal contract (`SkillMethodBeanPostProcessor.java:172-187`).
- Schema post-processing renames implementation parameters to contract parameter names, copies nonblank `@SkillParam` descriptions, and changes top-level requiredness according to `@SkillParam.required` (`SkillMethodBeanPostProcessor.java:294-360`). Runtime-reference metadata is then applied according to the Java type (`SkillMethodBeanPostProcessor.java:363-380`).
- Spring auto-configuration creates the target registry, post-processor, YAML registrar, contract resolver, validator, execution router, and `SkillTemplate` as infrastructure beans (`loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/autoconfigure/LoomspanAutoConfiguration.java:76-110,172-206,239-280`). The inspected input-contract beans have no `@ConditionalOnMissingBean` declaration.

### 3. Internal input-contract representation and round trip

- `SkillInputSchemaNode` requires a nonblank string `type` and represents object properties, required fields, boolean `additionalProperties`, a schema-valued `additionalPropertiesSchema`, array items, enum values, descriptions, formats, runtime-ref metadata, and attachment metadata (`loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/runtime/input/SkillInputSchemaNode.java:7-29`). There is no explicit current node kind for an unconstrained JSON value.
- A node is an object only when its `type` equals `object`; open-object behavior is derived from `additionalProperties != false` or the presence of `additionalPropertiesSchema` (`SkillInputSchemaNode.java:59-81`).
- Contract kinds distinguish `JAVA_REFLECTED`, `YAML_INHERITED`, `YAML_EXPLICIT`, and `GENERIC`. `genericObject()` is an open object contract and the validator bypasses detailed validation for the `GENERIC` kind (`loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/runtime/input/SkillInputContract.java:7-39`; `SkillInputValidator.java:20-36`).
- JSON Schema parsing defaults a missing/non-textual `type` to `object` (`loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/runtime/input/SkillInputContractResolver.java:125-128`). Thus an empty schema object currently resolves as an object constraint.
- Boolean `additionalProperties` is recorded directly. Object-valued `additionalProperties` is recursively parsed into `additionalPropertiesSchema` (`SkillInputContractResolver.java:152-165`). The current `{"type":"object"}` value schema therefore survives parsing as an object node rather than becoming generic.
- Serialization always writes a `type` from the internal node and recursively writes properties, `additionalPropertiesSchema`, and items (`SkillInputContractResolver.java:200-226`). The current representation cannot serialize a node while omitting its type.
- `resolveFromToolSchema` recognizes only an open, empty object schema as the special `GENERIC` contract; schemas with properties, required fields, `additionalProperties: false`, or an additional-properties value schema remain concrete (`SkillInputContractResolver.java:60-72`).

### 4. Mapped YAML inheritance and provider-facing schema

- If YAML declares `input_schema`, the resolver creates a `YAML_EXPLICIT` contract from the manifest. If a mapped target exists and YAML omits `input_schema`, it creates a `YAML_INHERITED` contract using the target's existing schema node. Otherwise it returns the generic open-object contract (`SkillInputContractResolver.java:40-58`).
- `YamlSkillCapabilityRegistrar` resolves the mapped Java target, resolves the input contract, and registers both the tool descriptor and contract in `CapabilityMetadata` (`loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/skill/YamlSkillCapabilityRegistrar.java:51-72`).
- A mapped wrapper without a declared input schema publishes the target's original reflected JSON text; its internal contract is the inherited target schema (`YamlSkillCapabilityRegistrar.java:101-120`). Its invoker delegates to the target's Java invoker (`YamlSkillCapabilityRegistrar.java:76-91`).
- `BoundCapability.inputSchema()` exposes the metadata tool schema (`loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/runtime/tool/BoundCapability.java:28-38`). `SpringAiToolCallbackAdapter` passes it directly to Spring AI's `FunctionToolCallback.inputSchema`, which is the provider/model-facing tool schema boundary (`loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/springai/SpringAiToolCallbackAdapter.java:20-28`).

### 5. Planner prompt guidance

- `StepPromptBuilder` selects the ready task's `BoundCapability`, reads its internal `inputContract`, skips generic contracts, and renders compact or verbose argument guidance. Deep or property-heavy schemas use verbose guidance automatically, and a validation retry forces verbose guidance (`loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/runtime/step/StepPromptBuilder.java:424-480`).
- `SkillInputPromptRenderer` treats a concrete object with no named properties and no `additionalPropertiesSchema` as a no-argument tool, returning an empty-object instruction (`loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/runtime/input/SkillInputPromptRenderer.java:15-27`).
- Object examples enumerate declared properties and add a `<key>` entry for schema-valued additional properties. Rendering the current generic-map value schema recursively yields an object-shaped value (`SkillInputPromptRenderer.java:40-75`).
- In verbose guidance, any `additionalPropertiesSchema` produces the sentence ``<path>.<key> must be a <type>``. With the reflected `Object` leaf, the rendered type is `object` (`SkillInputPromptRenderer.java:97-149`).
- On rejected model output, `StepLoopMissionExecutionEngine` appends the full rejection reason to the next system prompt and sets `forceVerboseToolArgumentGuidance` (`loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/runtime/step/StepLoopMissionExecutionEngine.java:453-482,537-555`).

### 6. Step validation, routing validation, and direct entry

- `StepActionValidator` validates a proposed `CALL_TOOL` before side effects. For a concrete tool contract it runs the shared `SkillInputValidator`; rejection text includes each issue path, code, and message (`loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/runtime/step/StepActionValidator.java:123-170`).
- `SkillInputValidator` dispatches strictly by the node's string type. Unknown types pass through via the default branch, while `object` invokes object validation (`loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/runtime/input/SkillInputValidator.java:45-60`).
- Object validation requires a Java `Map`; otherwise it emits `type_mismatch` with `Expected object input.` For entries not present in named properties, a schema-valued `additionalProperties` is recursively applied to each entry value (`SkillInputValidator.java:63-107`). This is the direct source of paths such as `options[0].operator` when the map-value node has `type=object`.
- Array validation recursively validates each list element against `items`, preserving path indexes such as `[0]` (`SkillInputValidator.java:110-129`). Typed scalar validators retain the current coercion rules for integer, number, and boolean strings and reject incompatible value kinds (`SkillInputValidator.java:132-213`).
- After step validation, a bound tool invokes `DefaultCapabilityInvoker`, which calls `CapabilityExecutionRouter` (`loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/runtime/tool/DefaultCapabilityInvoker.java:55-68,99-118`). The router checks access, validates the arguments again against the same capability contract, uses normalized input, resolves runtime references, and invokes the registered capability (`loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/core/CapabilityExecutionRouter.java:51-90`).
- `DefaultSkillTemplate` is the direct application entry. Object input is first converted to `Map<String,Object>`; map input is validated against the YAML capability's contract before a session is created and routed (`loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/skillapi/DefaultSkillTemplate.java:54-108`). The router then performs its own validation again (`DefaultSkillTemplate.java:127-132`; `CapabilityExecutionRouter.java:59-72`).

### 7. Java binding and invocation

- The method post-processor binds arguments by contract parameter name. Missing optional parameters become null; other raw values are passed to `convertArgument` (`SkillMethodBeanPostProcessor.java:519-545`).
- Simple values already assignable to their non-container target type are returned directly. Container values, including `List<Map<String,Object>>`, are materialized and converted with Jackson using the complete parameterized `JavaType` (`SkillMethodBeanPostProcessor.java:548-585`).
- This binding layer accepts ordinary map/list/scalar structures for the travel method. In the motivating path, contract validation occurs before binding, so scalar map values rejected by the current internal schema do not reach the Java method.

### 8. Travel sample as an in-repository consumer

- `TravelCatalogService.rankTransportOptions` declares optional `List<Map<String,Object>> options`, fallback `String optionsJson`, and optional `String sortBy`. Its `@SkillParam` example contains scalar identity, price, and duration values (`loomspan-sample/src/main/java/com/lokiscale/loomspan/sample/travel/TravelCatalogService.java:202-206`).
- Native options are copied as maps and used before the JSON fallback; ranking reads scalar price/duration values and returns ranked copies (`TravelCatalogService.java:207-263`).
- `TravelCatalogServiceTest` invokes the Java service directly with scalar-valued native maps and with serialized JSON, verifies deterministic price/duration ordering, and verifies numeric-string behavior (`loomspan-sample/src/test/java/com/lokiscale/loomspan/sample/travel/TravelCatalogServiceTest.java:90-147`). This test does not register or resolve the reflected Loomspan input contract.
- The parent planner explicitly instructs the model to pass a native array of compact option objects with scalar identity, price, and duration fields, with `optionsJson` only as fallback (`loomspan-sample/src/main/resources/skills/travel/plan_transport.yml:22-36`).
- `rank_transport_options.yml` declares only public metadata and `mapping.target_id`; it contains no `input_schema` (`loomspan-sample/src/main/resources/skills/travel/rank_transport_options.yml:1-9`). Its current schema and validation behavior therefore come from reflected Java inheritance.

### 9. Existing tests and executable fixtures

- `SkillMethodBeanPostProcessorTest` covers registration, invocation binding, optionality, ref-friendly nested values, parsing typed `additionalProperties`, and strict empty-object classification. It does not contain a direct `LoomspanMethodInputSchemaGenerator` test for `Object`, generic maps, or recursion (`loomspan-spring-boot-starter/src/test/java/com/lokiscale/loomspan/internal/core/SkillMethodBeanPostProcessorTest.java:35-310`).
- `SkillInputContractResolverTest` currently contains one attachment metadata round-trip test (`loomspan-spring-boot-starter/src/test/java/com/lokiscale/loomspan/internal/runtime/input/SkillInputContractResolverTest.java:19-35`).
- `SkillInputValidatorTest` protects generic openness, null preservation, typed map-value validation, scalar coercion, untyped array items, and attachment input shapes (`loomspan-spring-boot-starter/src/test/java/com/lokiscale/loomspan/internal/runtime/input/SkillInputValidatorTest.java:19-303`).
- `StepActionValidatorTest` protects concrete required arguments, generic objects, nested typed validation, open nested objects, typed maps, placeholders, and strict empty objects (`loomspan-spring-boot-starter/src/test/java/com/lokiscale/loomspan/internal/runtime/step/StepActionValidatorTest.java:74-363`).
- `StepPromptBuilderTest` protects concrete argument examples, contract-aware guidance, nested rules, typed-map examples, complex typed-map values, and arrays without item schemas (`loomspan-spring-boot-starter/src/test/java/com/lokiscale/loomspan/internal/runtime/step/StepPromptBuilderTest.java:176-395`).
- `YamlSkillCapabilityRegistrarTests.mappedYamlSkillWithoutInputSchemaInheritsJavaDerivedContract` is the focused inheritance evidence (`loomspan-spring-boot-starter/src/test/java/com/lokiscale/loomspan/internal/skill/YamlSkillCapabilityRegistrarTests.java:252-263`). `DefaultSkillTemplateTest` separately covers validated direct entry and invalid-input behavior (`loomspan-spring-boot-starter/src/test/java/com/lokiscale/loomspan/internal/skillapi/DefaultSkillTemplateTest.java:172-242,267-319`).
- Fresh baseline verification on 2026-08-20 ran 106 focused starter tests across the seven relevant classes with zero failures, and all 9 `TravelCatalogServiceTest` tests with zero failures. No existing test executes the exact sequence of reflecting a `List<Map<String,Object>>` Java parameter, resolving its generated schema, and validating the motivating scalar-valued transport payload. No existing focused test was found for recursive DTO schema generation.

### 10. Documentation and authoring guidance

- The root README states that mapped Java behavior is owned by the Java target and that a mapped wrapper publishes its reflected contract (`README.md:227-244`). It provides a `List<Map<String,Object>>` return example but does not define reflected input semantics for Java `Object` or generic-map values (`README.md:242-264`).
- The authoring knowledge-base routing table classifies input-contract coverage as foundational: strict mapped-Java ownership is covered, while complete schema and pure-YAML input behavior are not yet documented (`ai/skill-authoring/README.md:45-65`). There is no dedicated current input-contract topic in `ai/skill-authoring/`.

## Contract and Compatibility Classification

The categories below use the canonical names from `ai/thoughts/framework-feature-design-lens.md`.

### Application API

- The deliberately supported Java surface is the eight-type allowlist in `LoomspanPublicSurfaceArchitectureTest`, including `SkillTemplate`, `SkillMethod`, and `SkillParam` (`loomspan-spring-boot-starter/src/test/java/com/lokiscale/loomspan/architecture/LoomspanPublicSurfaceArchitectureTest.java:29-37`).
- The affected behavior is observed through existing `@SkillMethod`/`@SkillParam` declarations and `SkillTemplate`; no affected public signature currently exposes an internal schema type.
- The schema generator, resolver, schema node, prompt renderer, validator, registrar, router, and binding types are not allowlisted Application API.

### Supported SPI

- The repository documents no supported Loomspan SPI or bean replacement surface (`README.md:157-159`).
- The relevant infrastructure beans are constructed directly by auto-configuration and are not marked `@ConditionalOnMissingBean` (`LoomspanAutoConfiguration.java:76-110,172-206,239-280`). Their public Java visibility is technical exposure, not separate evidence of a supported SPI.

### Configuration and manifest contracts

- YAML `mapping.target_id` and the rule that mapped wrappers inherit the Java target's reflected input behavior are documented author-facing semantics (`README.md:227-244`).
- The travel mapped manifest is a verified consumer of inheritance because it omits `input_schema` (`rank_transport_options.yml:1-9`).
- No configuration property key is involved in the reflected `Object` classification.

### Persisted or serialized contracts

- The provider-facing tool JSON Schema is serialized runtime data, but no repository evidence classifies it as a durable or cross-version format. It is generated and consumed within the current runtime (`SkillMethodBeanPostProcessor.java:294-324`; `SpringAiToolCallbackAdapter.java:20-28`).
- The internal schema node and contract records are current implementation models and are not exposed in the supported API allowlist.

### Ephemeral diagnostic formats

- Validation rejection text and trace outcomes are current-run diagnostics. The ticket proposes no trace record or Console protocol shape change.
- The retry loop already records rejected actions, propagates the rejection text into retry guidance, and records exhausted failure (`StepLoopMissionExecutionEngine.java:494-505,516-555`). A successful call would traverse existing validated/completed records; a rejected call traverses existing rejection/failure records.
- No application-adapter REST/SSE, acquisition, problem, or consumed NDJSON code participates in the input schema flow. There are therefore no protected Java-to-Go protocol consumers or executable cross-language fixtures in this change area.

### Internal or accidentally exposed implementation

- All schema generation, internal contract representation, resolver, prompt renderer, validator, mapped registration, routing, and Java binding classes are under `com.lokiscale.loomspan.internal`.
- `LoomspanPublicSurfaceArchitectureTest` explicitly classifies technically public internal types such as `LoomspanMethodInputSchemaGenerator`, `SkillInputContractResolver`, `SkillInputPromptRenderer`, `SkillInputValidator`, and `DefaultSkillTemplate` as internal collaboration/implementation types (`LoomspanPublicSurfaceArchitectureTest.java:48-255`).

### Public declarations, interfaces, constructors, and Spring beans

- Public/internal declarations involved include the generator constructor and `generate`, resolver constructors and resolution/serialization methods, schema/contract records, prompt renderer, validator, registrar, router, and tool-binding types. Their package and architecture allowlist classify them as internal despite public modifiers.
- `CapabilityBindingFactory` is a public internal interface and `BoundCapability.Invocation` is a public nested functional interface (`loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/runtime/tool/CapabilityBindingFactory.java:11-16`; `BoundCapability.java:11-25`). They transport metadata/arguments but do not define an application SPI.
- Spring registers one shared resolver and validator for most runtime paths, while `StepActionValidator` currently owns a static validator instance for pre-side-effect validation (`LoomspanAutoConfiguration.java:195-206`; `StepActionValidator.java:32`). Existing visibility and bean construction are technical exposure rather than documented replacement contracts.

### Public-surface delta and shim status recorded by the ticket

- The ticket classifies the expected public-surface delta as none and the shim decision as no shim. The current allowlist contains no schema implementation type and no supported SPI.
- `LoomspanPublicSurfaceArchitectureTest` is the executable supported-surface authority described by repository guidance.

## Architecture Documentation

The current input contract has two synchronized representations and multiple consumers:

```text
@SkillMethod Java signature
        |
        v
LoomspanMethodInputSchemaGenerator
        |
        +--> reflected JSON Schema --> SkillImplementationTarget.inputSchema
        |                                   |
        |                                   +--> mapped YAML tool descriptor
        |                                   |        |
        |                                   |        +--> Spring AI/provider schema
        |                                   |
        |                                   +--> mapped public capability metadata
        |
        +--> SkillInputContractResolver --> SkillInputContract/SkillInputSchemaNode
                                                |
                                                +--> compact/verbose planner guidance
                                                +--> pre-side-effect StepAction validation
                                                +--> CapabilityExecutionRouter validation
                                                +--> SkillTemplate direct-entry validation
                                                             |
                                                             v
                                                Jackson Java argument binding/invocation
```

The JSON descriptor and internal contract are created together at Java target discovery. A mapped YAML wrapper without `input_schema` reuses both target forms rather than independently reconstructing a contract. Validation normalization happens before ref resolution and Java binding. Planner retry feedback is generated from validation issues and rendered alongside more detailed guidance from the same internal schema.

## Framework Design Lens Context

- **Developer problem**: the ticket concerns a reflected contract that currently describes scalar-capable Java `Object` values as objects at every model/runtime contract consumer.
- **Framework responsibility**: Loomspan owns reflection, provider schema publication, prompt rendering, pre-call validation, direct-entry validation, and Java binding. The Java method and planner do not independently create the narrower schema.
- **Developer experience**: Java authors currently declare the ordinary Java type and `@SkillParam` metadata; mapped YAML authors omit `input_schema` and inherit it. Application callers use `SkillTemplate` against the public YAML name.
- **State, dataflow, trust, and mutability**: this flow carries ordinary business input. It introduces no trusted execution metadata or ambient state. Validation returns copied, unmodifiable map/list structures for traversed objects and arrays (`SkillInputValidator.java:306-314`).
- **Safeguards**: access checking, requiredness, closed-object validation, scalar coercion rules, placeholder detection, reference resolution, retry limits, and Java binding are distinct current stages. The ticket leaves their surrounding contracts in scope as preservation signals.
- **Evidence size**: the repository contains a concrete mapped travel consumer, scalar-valued planner instructions, direct Java unit behavior, a coherent multi-layer contract flow, and adjacent tests. The execution trace itself is not stored in the repository.
- **Alternatives recorded in the ticket**: the ticket explicitly excludes map-key inference, a typed travel DTO redesign, `optionsJson` rerouting, argument auto-repair, a duplicated YAML schema, and validator exceptions. These are ticket boundaries, not current implementation paths.
- **Compatibility**: affected author-facing mapped inheritance is a Configuration and manifest contract; implementation models are internal; provider schemas are current-runtime serialized data; diagnostics keep their current formats; no Java-to-Go protocol or public API signature is involved.

## Historical Context (from `ai/thoughts/`)

- `ai/thoughts/tickets/loomspan-pr-32-unconstrained-java-input-contracts.md` is the only `ai/thoughts/` document containing the reported trace IDs or discussing this exact unconstrained/generic-map behavior. The trace IDs do not occur elsewhere in the checkout.
- `ai/thoughts/framework-feature-design-lens.md` supplies the compatibility categories and states that public modifiers, interfaces, constructors, Spring beans, tests, and fixtures are exposure/behavior evidence rather than independent proof of supported API or SPI status.
- Git history shows the input-contract implementation originated before the current PR-31 checkout (`073072d`, `70fb0e3`), while the current schema generator file was later introduced during the Spring Boot 4/Spring AI 2 migration (`9057636`). Live code at the researched commit is the primary source used above.
- `ai/thoughts/future/possible-trusted-execution-metadata.md` explicitly separates ordinary business inputs from possible trusted runtime metadata. The transport option maps in this ticket are ordinary model/caller-supplied business input.

## Related Research

No related research document was found under `ai/thoughts/research/` in the current checkout.

## Code References

- `loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/serialization/LoomspanMethodInputSchemaGenerator.java:49-120` — recursive Java-type-to-schema mapping, including maps, `Object`, DTOs, and recursion protection.
- `loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/runtime/input/SkillInputSchemaNode.java:7-81` — current internal schema-node representation.
- `loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/runtime/input/SkillInputContractResolver.java:33-85,109-250` — contract classification, JSON parsing, and serialization.
- `loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/runtime/input/SkillInputValidator.java:20-129` — generic bypass and recursive object/array validation.
- `loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/runtime/input/SkillInputPromptRenderer.java:15-182` — compact and verbose model-facing argument guidance.
- `loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/core/SkillMethodBeanPostProcessor.java:172-187,294-380,519-585` — registration, schema metadata, and Java argument binding.
- `loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/skill/YamlSkillCapabilityRegistrar.java:51-120` — mapped YAML inheritance and descriptor selection.
- `loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/runtime/step/StepActionValidator.java:123-170` — pre-side-effect tool argument validation.
- `loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/core/CapabilityExecutionRouter.java:51-90` — access, second validation, ref resolution, and invocation.
- `loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/skillapi/DefaultSkillTemplate.java:54-132` — direct mapped-skill validation path.
- `loomspan-sample/src/main/java/com/lokiscale/loomspan/sample/travel/TravelCatalogService.java:202-290` — motivating Java method and native/JSON option handling.
- `loomspan-sample/src/main/resources/skills/travel/plan_transport.yml:22-36` — scalar-valued planner instructions and example.
- `loomspan-sample/src/main/resources/skills/travel/rank_transport_options.yml:1-9` — mapped wrapper with inherited input contract.

## Open Questions

- The supplied imported trace is not present as an artifact in the repository, so the exact producing build and complete recorded prompt/schema payload remain unverified from checkout evidence.
- The current test suite does not establish which provider-specific representation is used for an unconstrained JSON value because no such reflected representation exists in the internal model today.
- The current tests do not classify or demonstrate intended recursive DTO fallback semantics beyond the generator's `visiting` branch.
- The authoring knowledge base marks input contracts as foundational and has no dedicated topic, so current author-facing documentation does not state Java `Object` or generic-map input semantics.
