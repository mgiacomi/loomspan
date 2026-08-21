---
audience: loomspan-skill-builder
status: development
applies_to: current-repository-checkout
coverage: initial-source-verified
---

# Reflected Java Input Contracts

## Applicability

Use this topic when designing or diagnosing the input of a mapped YAML skill whose `mapping.target_id` names a Java `@SkillMethod`. The mapped wrapper MUST omit `input_schema`; Loomspan reflects the Java signature and `@SkillParam` metadata as the provider-facing schema, planner guidance, and runtime validation contract.

This topic does not define the complete pure-YAML `input_schema` language.

## Type Decision Table

| Java declaration | Enforced reflected meaning | Authoring decision |
| --- | --- | --- |
| `Object` | Any JSON-compatible value: JSON null where existing optionality permits it, string, boolean, number, object, or array | Use only when the value is deliberately shape-free |
| `Map<String, Object>` | A JSON object with arbitrary string keys whose values may be any JSON-compatible value | Use for genuinely open metadata or heterogeneous dictionaries |
| `Map<String, T>` | A JSON object whose every entry value follows the reflected contract for `T` | Use when keys are open but values have one stable type |
| Record or DTO | A closed JSON object with reflected named properties | SHOULD be the default when planners need discoverable field names and types |
| Array or `List<T>` | A JSON array whose items follow the reflected contract for `T` | Choose `T` to communicate the real item shape |
| Recursive DTO | A bounded object-shaped schema; cycle protection does not turn the recursive leaf into arbitrary JSON | Use only when the recursive domain shape is intentional |

`@SkillParam(required = false)` controls omission at the method-parameter boundary. It does not make a typed value accept JSON null generally. Optional primitive parameters are invalid because omission binds `null`; use the boxed type. This change does not alter those requiredness rules.

## Enforced Semantics

- Java `Object` is emitted as the JSON Schema empty schema `{}` at that value node. Loomspan preserves this unconstrained meaning through internal resolution and provider publication; it does not rewrite it to `type: object`.
- Unconstrained values preserve scalar kinds and recursively normalize maps and lists into immutable containers. Non-JSON application objects, non-string map keys, and non-finite floating-point values are rejected with path-qualified validation issues.
- `Map<String, Object>` does not infer keys, property names, or domain constraints. Scalars, nested objects, arrays, and JSON null values may appear under its string keys.
- Typed maps remain typed. Existing string, integer, number, boolean, enum, DTO, required-field, coercion, and closed-object rules apply to each entry value.
- Planner examples and verbose rules label unconstrained leaves as `any JSON value`. An open generic map is not a no-argument tool.
- Mapped YAML inheritance uses the same reflected schema and internal contract. Do not duplicate the schema in the mapped manifest.

Minimal mapped target:

```java
@SkillMethod(description = "Rank transport choices.")
Map<String, Object> rank(
        @SkillParam(description = "Compact transport choices.")
        List<Map<String, Object>> options) {
    // deterministic implementation
}
```

The corresponding mapped YAML manifest declares only its public metadata and mapping:

```yaml
name: rankTransportOptions
description: Deterministically ranks transport choices.
mapping:
  target_id: travelCatalogService#rankTransportOptions
```

## Design Guidance

Use a record or DTO when the planner should discover stable fields such as `operator`, `price`, and `durationMinutes`. A generic map truthfully communicates that keys and value kinds are open; a detailed parameter description can give examples, but examples do not become enforced property schemas. Do not use `Object` or a generic map merely to avoid modeling a stable contract.

Use a typed map when arbitrary keys are part of the domain but all values share a type. For example, `Map<String, Integer>` communicates and validates integer values while retaining open keys.

## Known Boundaries

- Loomspan does not infer fixed fields or domain rules from generic-map usage.
- This contract supports JSON-compatible values; arbitrary Java objects and resource/container types are not accepted under unconstrained leaves. Attachment and runtime-reference inputs use their dedicated reflected contracts.
- Requiredness and typed-null behavior outside deliberate unconstrained leaves are unchanged.
- Complete pure-YAML schema syntax and validation remain outside this topic.

## Executable Evidence and Implementation Anchors

- [`LoomspanMethodInputSchemaGeneratorTest`](../../loomspan-spring-boot-starter/src/test/java/com/lokiscale/loomspan/internal/serialization/LoomspanMethodInputSchemaGeneratorTest.java) protects direct `Object`, generic-map transport payloads, typed maps, and recursive DTO generation.
- [`SkillInputContractResolverTest`](../../loomspan-spring-boot-starter/src/test/java/com/lokiscale/loomspan/internal/runtime/input/SkillInputContractResolverTest.java) protects empty-schema parsing/serialization and the distinction between unconstrained, generic top-level, and strict empty-object contracts.
- [`SkillInputValidatorTest`](../../loomspan-spring-boot-starter/src/test/java/com/lokiscale/loomspan/internal/runtime/input/SkillInputValidatorTest.java) protects JSON-kind preservation, immutable recursive normalization, and visible rejection of unsupported values and schema kinds.
- [`StepPromptBuilderTest`](../../loomspan-spring-boot-starter/src/test/java/com/lokiscale/loomspan/internal/runtime/step/StepPromptBuilderTest.java) and [`StepActionValidatorTest`](../../loomspan-spring-boot-starter/src/test/java/com/lokiscale/loomspan/internal/runtime/step/StepActionValidatorTest.java) protect planner guidance and pre-side-effect validation.
- [`YamlSkillCapabilityRegistrarTests`](../../loomspan-spring-boot-starter/src/test/java/com/lokiscale/loomspan/internal/skill/YamlSkillCapabilityRegistrarTests.java) protects mapped inheritance; [`SampleApplicationTests`](../../loomspan-sample/src/test/java/com/lokiscale/loomspan/sample/SampleApplicationTests.java) protects the real native transport invocation through `SkillTemplate`.
- [`LoomspanMethodInputSchemaGenerator`](../../loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/serialization/LoomspanMethodInputSchemaGenerator.java), [`SkillInputContractResolver`](../../loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/runtime/input/SkillInputContractResolver.java), [`SkillInputValidator`](../../loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/runtime/input/SkillInputValidator.java), and [`SkillInputPromptRenderer`](../../loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/runtime/input/SkillInputPromptRenderer.java) define the current production path.
