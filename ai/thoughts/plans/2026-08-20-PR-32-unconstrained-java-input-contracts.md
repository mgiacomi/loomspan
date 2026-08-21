# PR 32 — Faithful Unconstrained Java Input Contracts Implementation Plan

## Overview

Correct Loomspan's reflected Java input contract so `Object` means any JSON-compatible value and `Map<String, Object>` retains unconstrained entry values across provider-facing JSON Schema, the internal contract model, planner guidance, validation, mapped-skill inheritance, `SkillTemplate`, and Java binding. The change will use one explicit internal unconstrained-value kind, preserve typed-map and recursive-DTO behavior, and update the authoring knowledge base in the same atomic change.

## Current State Analysis

`LoomspanMethodInputSchemaGenerator` currently uses `{"type":"object"}` for both Java `Object` and an already-visiting DTO type. For `List<Map<String, Object>>`, that produces `array -> object -> additionalProperties(type=object)`, even though the Java method accepts scalar map values (`loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/serialization/LoomspanMethodInputSchemaGenerator.java:49-120`).

The generated schema is stored on the Java target and parsed into a second, internal representation during method discovery (`loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/core/SkillMethodBeanPostProcessor.java:172-187,294-380`). `SkillInputSchemaNode` currently requires a nonblank string `type`, while `SkillInputContractResolver` defaults a schema with no textual `type` to `object` and always writes a type during serialization. Consequently, changing only the generator to emit `{}` would still recreate the object constraint internally (`loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/runtime/input/SkillInputSchemaNode.java:7-81`; `loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/runtime/input/SkillInputContractResolver.java:109-226`).

Mapped YAML skills that omit `input_schema` inherit both the target's schema text and internal contract. The provider descriptor receives the schema text, while planner guidance and all runtime validation paths consume the internal contract (`loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/skill/YamlSkillCapabilityRegistrar.java:51-120`). The validator applies a schema-valued `additionalProperties` node to every unknown map entry, producing the reported `Expected object input.` errors for strings and numbers (`loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/runtime/input/SkillInputValidator.java:20-129`). The prompt renderer reinforces the mismatch by rendering an object placeholder and saying each value must be an object (`loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/runtime/input/SkillInputPromptRenderer.java:15-149`).

The travel sample is a concrete in-repository consumer. `TravelCatalogService#rankTransportOptions` accepts `List<Map<String, Object>>`, its parameter description and parent planner prompt show scalar-valued option fields, and `rank_transport_options.yml` deliberately inherits the Java target contract (`loomspan-sample/src/main/java/com/lokiscale/loomspan/sample/travel/TravelCatalogService.java:202-263`; `loomspan-sample/src/main/resources/skills/travel/plan_transport.yml:22-36`; `loomspan-sample/src/main/resources/skills/travel/rank_transport_options.yml:1-9`). Its direct service tests bypass Loomspan's reflected validation, so they do not catch the defect (`loomspan-sample/src/test/java/com/lokiscale/loomspan/sample/travel/TravelCatalogServiceTest.java:90-147`).

## Desired End State

Java `Object` is reflected as the JSON Schema Draft 2020-12 empty schema `{}` at the value node, which accepts any JSON-compatible value without claiming `type=object`. Loomspan parses that syntax into a deliberately named internal unconstrained/`any` node and serializes it back without a `type`, while retaining descriptions and Loomspan metadata when present. Validation has an explicit unconstrained branch that accepts and preserves JSON null, string, boolean, numeric, object, and array value kinds, recursively normalizes collection structures, and rejects non-JSON-compatible Java values rather than relying on the validator's current unknown-type fallthrough.

`Object.class` and recursion protection are separate generator branches. An already-visiting DTO remains an object-shaped bounded fallback and never becomes unconstrained. Typed maps continue to use their value schema and current scalar coercion rules. Planner examples and verbose rules describe unconstrained leaves as JSON values, not objects, and an open map is never mistaken for a no-argument capability.

The exact scalar-valued `rankTransportOptions` payload succeeds through reflected schema generation, internal resolution, step validation, mapped YAML inheritance, `SkillTemplate`, routing, Jackson binding, and invocation. The provider-facing mapped tool descriptor carries the same unconstrained meaning. The starter and sample suites pass, and `LoomspanPublicSurfaceArchitectureTest` confirms that the supported API allowlist is unchanged.

### Key Discoveries

- The generator creates the provider schema and the resolver immediately creates the internal contract from the same JSON, so the fix must update both representations atomically (`SkillMethodBeanPostProcessor.java:172-187`).
- Mapped wrappers without `input_schema` reuse the Java target's schema and node rather than reconstructing either form (`YamlSkillCapabilityRegistrar.java:51-120`).
- Pre-side-effect step validation, execution-router validation, and direct `SkillTemplate` validation all converge on `SkillInputValidator`; one explicit node semantic can keep these paths coherent (`StepActionValidator.java:123-170`; `CapabilityExecutionRouter.java:51-90`; `DefaultSkillTemplate.java:54-132`).
- Jackson binding already materializes `List<Map<String, Object>>` with the complete parameterized `JavaType`; the failure occurs before invocation (`SkillMethodBeanPostProcessor.java:519-585`).
- The authoring knowledge base marks input contracts as only foundational and has no focused input-contract topic (`ai/skill-authoring/README.md:45-65`).
- The imported trace is supplied historical evidence only; the checkout proves the schema mismatch but cannot authenticate which build produced trace `c3cc066a-a9ed-4084-ae88-b7058fa3d05a`.

## What We're NOT Doing

- Inferring property names or domain rules from `Map<String, Object>`.
- Replacing the travel maps with a typed DTO or routing native `options` through `optionsJson`.
- Adding YAML `input_schema` to `rank_transport_options.yml` or changing the YAML input-schema language.
- Weakening object validation, skipping validation, adding a travel-specific exception, repairing arguments, or coercing scalars into objects.
- Adding union/`oneOf`/discriminator support or changing output-schema validation.
- Changing required/optional parameter semantics; any separate top-level-null defect remains out of scope.
- Adding public API, supported SPI, bean replacement hooks, compatibility shims, parallel schema models, legacy readers, or dual behavior.
- Changing trace, Console MCP, application-adapter REST/SSE, acquisition, problem, or consumed-NDJSON formats.

## Skill-Authoring Documentation Impact

**Impact**: Affected

- **Rationale**: Mapped Java contract inheritance is author-facing behavior. Authors need an exact rule for Java `Object`, generic maps, typed maps, and recursive/typed DTOs, plus guidance that a typed record/DTO is preferable when planners need discoverable field names and types.
- **Documents to update**: Add `ai/skill-authoring/input-contracts.md`; update `ai/skill-authoring/README.md`, `ai/skill-authoring/mental-model.md`, and the mapped-Java section of `README.md`.
- **Supporting evidence**: The new reflected-path generator/resolver/validator regression, typed-map and recursive-DTO tests, prompt-rendering tests, mapped-inheritance test, and `SampleApplicationTests` travel invocation will support the guidance. Production anchors are `LoomspanMethodInputSchemaGenerator`, `SkillInputContractResolver`, `SkillInputValidator`, `SkillInputPromptRenderer`, `SkillMethodBeanPostProcessor`, and `YamlSkillCapabilityRegistrar`.
- **Coverage table update**: Required. Add a routing entry for designing or diagnosing reflected Java inputs and raise the Input contracts row from `Foundational` to `Initial, source-verified`, while stating that complete pure-YAML schema syntax remains outside this topic.
- **LLM-first usability**: The new focused topic will lead with an applicability/decision table for `Object`, `Map<String, Object>`, typed maps, DTOs, arrays, requiredness, and JSON null; separate enforced semantics from the DTO recommendation; use minimal examples; name limitations; and link stable focused test and production anchors. `mental-model.md` will retain only the mapping-level summary and route detailed questions to the new topic rather than duplicating it.

## Contract and Compatibility Impact

| Surface | Classification and supporting evidence | Planned compatibility treatment |
| --- | --- | --- |
| Application API | Existing allowlisted `SkillMethod`, `SkillParam`, and `SkillTemplate` users observe corrected behavior, but no affected signature exposes an internal schema type (`LoomspanPublicSurfaceArchitectureTest.java:29-37`). | Preserve all signatures and requiredness behavior; correct reflected runtime semantics in place. |
| Supported SPI | No supported Loomspan SPI or bean-replacement surface exists; relevant public constructors and Spring beans are internal technical exposure (`README.md:157-159`; `LoomspanAutoConfiguration.java:76-110,172-206,239-280`). | No SPI added or preserved. |
| Configuration and manifest contracts | Affected: documented mapped YAML skills inherit their Java target contract, and `rank_transport_options.yml` is a verified consumer that omits `input_schema` (`README.md:227-244`; `rank_transport_options.yml:1-9`). | Intentional corrective semantic change applied atomically to schema, guidance, validation, tests, sample, and documentation; manifest syntax is unchanged. |
| Persisted or serialized contracts | Provider-facing JSON Schema changes at runtime, but it is generated and consumed by the current runtime and is not a durable/cross-version format (`SkillMethodBeanPostProcessor.java:294-324`; `SpringAiToolCallbackAdapter.java:20-28`). | Emit one current-version coherent Draft 2020-12 schema; no legacy reader or migration. |
| Ephemeral diagnostic formats | No record shape changes. Valid calls will produce existing validated/completed outcomes instead of validation-exhaustion failures. | Preserve diagnostic structure, ordering, security, and rejection details for genuinely invalid inputs. |
| Internal or accidentally exposed implementation | Generator, schema node, resolver, validator, prompt renderer, registrar, router, and binding types are under `internal` and explicitly excluded from supported API despite technical visibility (`LoomspanPublicSurfaceArchitectureTest.java:48-255`). | Change current implementation atomically; remove the incorrect shared Object/recursion behavior without a shim. |

- **Evidence of supported contracts**: The public-surface architecture allowlist, mapped-Java inheritance documented in `README.md` and `ai/skill-authoring/mental-model.md`, the approved ticket, and the travel sample consumer.
- **Intended breaks**: The provider schema, prompt wording, and validation behavior intentionally stop treating Java `Object`/generic-map values as objects. Consumers that accidentally depended on the incorrect narrowing are not protected; typed objects and typed maps remain constrained.
- **In-repository consumers to update**: Focused starter tests, `SampleApplicationTests`, `README.md`, `ai/skill-authoring/README.md`, `ai/skill-authoring/mental-model.md`, and the new input-contract topic. The travel Java method and YAML manifests remain unchanged as correctness fixtures.
- **Public-surface delta**: None. No allowlisted type, signature, constructor, annotation member, or supported Spring extension point is added or removed.
- **Shim decision**: **No shim.** The affected schema implementation is internal, the mapped behavior is being corrected before 1.0, and the repository can be updated atomically.
- **Java-to-Go boundary coordination**: **Not required.** No application-adapter REST/SSE, acquisition, problem, or consumed-NDJSON boundary participates in this path.

## Implementation Approach

Use `{}` as the standards-valid provider-facing schema for a reflected Java `Object` value while keeping the top-level tool arguments schema a strict object. Add a named internal unconstrained type (for example, an `ANY_TYPE` constant plus `isUnconstrained()`/factory semantics on `SkillInputSchemaNode`) rather than treating an arbitrary unknown type as permissive. `SkillInputContractResolver` will map a missing JSON Schema `type` to that internal kind and omit `type` when serializing it. Other schema keywords such as `description` must survive the round trip.

Add an explicit unconstrained case to `SkillInputValidator`. It will preserve scalar number classes and value kinds, recursively copy maps/lists using the validator's immutable normalization convention, accept JSON null where existing requiredness permits it, and report a focused validation issue for non-JSON-compatible leaves. Replace the current permissive default switch branch with an unsupported-schema-type failure so no unrecognized string becomes an accidental bypass.

Teach `SkillInputPromptRenderer` to render an unconstrained node as an “any JSON value” placeholder/rule and to use that description for map values and array items. Keep no-argument detection limited to an actual object contract with no named or schema-valued properties. `StepPromptBuilder`'s depth/property calculations should naturally treat an unconstrained leaf as terminal; protect both compact and forced-verbose output with tests.

Separate `Object.class` from `!visiting.add(raw)` in the generator. The Object branch returns the empty schema; the recursion branch continues returning an object schema and terminates traversal. Do not change binding or add per-entry domain inference: existing Jackson materialization is the final consumer once validation succeeds.

## Phase 1: Establish the Failing Reflected-Path Regression

### Overview

Add the narrow regression first and demonstrate that it fails on the current production code for the reported reason. The test must construct the contract from a real reflected Java signature rather than a hand-authored permissive node.

### Changes Required

#### 1. Reflected schema-to-validation regression

**File**: `loomspan-spring-boot-starter/src/test/java/com/lokiscale/loomspan/internal/serialization/LoomspanMethodInputSchemaGeneratorTest.java` (new)

**Changes**:

- Define a test fixture method with an `options` parameter of `List<Map<String, Object>>` and obtain its real generic `Method` metadata.
- Generate the schema with `LoomspanMethodInputSchemaGenerator`, resolve it through `SkillInputContractResolver.resolveJavaCapability`, and validate the exact three-option `rankTransportOptions` payload from the ticket.
- Assert success and preservation of `operator` strings, integral `durationMinutes`, decimal `price`, list order, and concrete value kinds.
- Before the production fix, retain evidence that this test fails with paths such as `options[0].operator` and `Expected object input.`; do not weaken the assertion to accept either behavior.

#### 2. Baseline preservation cases

**File**: `loomspan-spring-boot-starter/src/test/java/com/lokiscale/loomspan/internal/serialization/LoomspanMethodInputSchemaGeneratorTest.java`

**Changes**:

- Add fixture signatures for direct `Object`, `Map<String, String>`, `Map<String, Integer>`, `Map<String, SomeRecord>`, and a self-referential DTO.
- Capture the expected post-fix acceptance/rejection matrix without hand-authoring the generated contracts: unconstrained JSON value kinds accepted; typed values enforced/coerced according to current rules; recursive schema generation bounded and object-shaped.
- Keep the first commit/test run focused enough to show the motivating regression fails before any production changes.

### Success Criteria

#### Automated Verification

- [x] The new motivating test fails against the unmodified checkout for the current object mismatch: `.\mvnw.cmd -pl loomspan-spring-boot-starter -Dtest=LoomspanMethodInputSchemaGeneratorTest test`.
- [x] Existing focused tests remain green before production work: `.\mvnw.cmd -pl loomspan-spring-boot-starter '-Dtest=SkillMethodBeanPostProcessorTest,SkillInputContractResolverTest,SkillInputValidatorTest,StepActionValidatorTest,StepPromptBuilderTest,YamlSkillCapabilityRegistrarTests,DefaultSkillTemplateTest' test`.

#### Manual Verification

- [x] Review the failure output and confirm it exercises generated schema plus resolver plus validator, not a direct service call or hand-authored contract.
- [x] Confirm the payload exactly matches the ticket's scalar-valued transport options and uses native `options`, not `optionsJson`.

---

## Phase 2: Implement One Explicit Unconstrained Contract Across All Consumers

### Overview

Change schema generation, internal parsing/serialization, validation, and prompt rendering as one coherent contract update, then add focused preservation coverage for typed maps, recursion, and surrounding validation semantics.

### Changes Required

#### 1. Separate unconstrained Java values from recursive DTO fallback

**File**: `loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/serialization/LoomspanMethodInputSchemaGenerator.java`

**Changes**:

- Give `Object.class` its own branch that returns an empty schema object at that value node.
- Keep the already-visiting branch separate and object-shaped so recursion stays bounded without becoming an arbitrary-value escape hatch.
- Preserve arrays/collections, typed `additionalProperties`, DTO property reflection, top-level required fields, and top-level `additionalProperties: false`.

#### 2. Represent and round-trip unconstrained nodes deliberately

**Files**:

- `loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/runtime/input/SkillInputSchemaNode.java`
- `loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/runtime/input/SkillInputContractResolver.java`
- `loomspan-spring-boot-starter/src/test/java/com/lokiscale/loomspan/internal/runtime/input/SkillInputContractResolverTest.java`

**Changes**:

- Add a named internal unconstrained kind/helper; do not use `GENERIC`, because `GENERIC` means an uncontracted top-level argument object and bypasses detailed validation.
- Parse a JSON Schema object without textual `type` as unconstrained rather than as `object`.
- Serialize an unconstrained node without a `type` keyword while retaining applicable `description` and Loomspan metadata.
- Keep `{ "type": "object" }`, open objects, strict empty objects, typed `additionalProperties`, arrays without `items`, and attachment schemas distinct.
- Add resolver tests for `{}` and described unconstrained nodes, nested `additionalProperties: {}`, round-trip stability, and the existing generic-object classification boundary.

#### 3. Validate unconstrained JSON values explicitly

**Files**:

- `loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/runtime/input/SkillInputValidator.java`
- `loomspan-spring-boot-starter/src/test/java/com/lokiscale/loomspan/internal/runtime/input/SkillInputValidatorTest.java`
- `loomspan-spring-boot-starter/src/test/java/com/lokiscale/loomspan/internal/runtime/step/StepActionValidatorTest.java`

**Changes**:

- Add an explicit unconstrained validation branch that accepts JSON null, strings, booleans, numeric values, maps, and lists; recursively produces stable immutable map/list structures without scalar coercion.
- Reject unsupported Java leaf/container values with a deterministic validation issue rather than silently accepting them.
- Make an unknown internal schema type fail explicitly so permissiveness cannot depend on the current switch default.
- Prove a direct `Object` parameter and `Map<String, Object>` accept all supported JSON kinds, including nested objects and arrays.
- Preserve `Map<String, String>`, `Map<String, Integer>`, `Map<String, SomeRecord>`, requiredness, optionality, strict objects, placeholder detection, runtime refs, attachments, scalar coercion, and issue paths.
- Add a step-action case using the resolved reflected contract so pre-side-effect validation accepts the motivating arguments and still rejects unresolved placeholder strings nested under unconstrained values.

#### 4. Render accurate compact and verbose planner guidance

**Files**:

- `loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/runtime/input/SkillInputPromptRenderer.java`
- `loomspan-spring-boot-starter/src/test/java/com/lokiscale/loomspan/internal/runtime/step/StepPromptBuilderTest.java`

**Changes**:

- Render an unconstrained node as an arbitrary JSON value rather than an object placeholder.
- In compact and verbose map/array guidance, state that the entry/item may be any JSON value and never emit “must be an object” for a Java `Object` leaf.
- Ensure an open map with unconstrained values is not rendered as a no-argument tool.
- Preserve current field sorting, enum guidance, required-field lists, closed-object warnings, typed-map examples, maximum-depth behavior, and generic-contract omission.

#### 5. Protect target publication and binding

**Files**:

- `loomspan-spring-boot-starter/src/test/java/com/lokiscale/loomspan/internal/core/SkillMethodBeanPostProcessorTest.java`
- `loomspan-spring-boot-starter/src/test/java/com/lokiscale/loomspan/internal/skill/YamlSkillCapabilityRegistrarTests.java`

**Changes**:

- Verify discovered Java targets publish the unconstrained value schema and resolve the matching internal node while retaining `@SkillParam` descriptions and optionality.
- Verify a mapped YAML wrapper without `input_schema` inherits both forms, and its provider-facing descriptor does not claim generic-map values are objects.
- Invoke fixture targets to confirm normalized unconstrained structures reach Jackson binding unchanged while typed map/DTO binding remains intact.
- Do not alter mapped manifests or introduce a second schema source.

### Success Criteria

#### Automated Verification

- [x] The Phase 1 regression and generator matrix pass: `.\mvnw.cmd -pl loomspan-spring-boot-starter -Dtest=LoomspanMethodInputSchemaGeneratorTest test`.
- [x] Resolver, validator, step-validation, prompt, discovery, binding, and mapped-inheritance tests pass: `.\mvnw.cmd -pl loomspan-spring-boot-starter '-Dtest=SkillMethodBeanPostProcessorTest,SkillInputContractResolverTest,SkillInputValidatorTest,StepActionValidatorTest,StepPromptBuilderTest,YamlSkillCapabilityRegistrarTests,DefaultSkillTemplateTest' test`.
- [x] Generated schemas remain valid JSON and every unconstrained value node round-trips without acquiring `type: object`.
- [x] Typed-map and recursive-DTO tests prove that only deliberate Java `Object` leaves become unconstrained.

#### Manual Verification

- [x] Inspect representative compact and forced-verbose prompts and confirm their examples and prose agree with the generated provider schema.
- [x] Inspect one direct `Object`, one `Map<String, Object>`, and one recursive DTO schema to confirm the three shapes are visibly distinct.
- [x] Confirm validation errors for typed maps and unsupported Java values remain precise and path-qualified.

---

## Phase 3: Verify the Real Mapped Travel Path and Publish Authoring Guidance

### Overview

Exercise the exact repository consumer through the supported application facade, document the now-source-verified semantics, and run module/public-surface verification.

### Changes Required

#### 1. End-to-end mapped travel acceptance

**Files**:

- `loomspan-sample/src/test/java/com/lokiscale/loomspan/sample/SampleApplicationTests.java`
- `loomspan-sample/src/test/java/com/lokiscale/loomspan/sample/travel/TravelCatalogServiceTest.java`

**Changes**:

- Add a `SkillTemplate.invoke("rankTransportOptions", ...)` integration test with the exact native three-option payload and `sortBy=price`.
- Assert successful deterministic ordering and returned scalar fields, proving mapped YAML discovery, inherited schema, direct-entry validation, router validation, Java binding, and invocation all agree.
- Keep `optionsJson` absent so the test cannot pass through the fallback.
- Retain the existing direct service tests as application-logic coverage; no production sample code or manifest change is expected.

#### 2. Source-verified input-contract authoring topic

**Files**:

- `ai/skill-authoring/input-contracts.md` (new)
- `ai/skill-authoring/README.md`
- `ai/skill-authoring/mental-model.md`
- `README.md`

**Changes**:

- Document the enforced reflected semantics for direct `Object`, `Map<String, Object>`, nested collections, typed maps, DTOs/records, requiredness, and JSON-compatible values.
- State the design recommendation: use a typed DTO/record and precise `@SkillParam` description when a planner needs discoverable keys and value types; Loomspan does not infer stable keys or domain rules from a generic map.
- State that mapped YAML wrappers inherit the Java contract and must not duplicate `input_schema`.
- Include a compact decision table, minimal examples, explicit limitations, and stable named test/source anchors following `source-verification.md`.
- Add the README routing entry and update the coverage row to `Initial, source-verified`; link from `mental-model.md` and keep the root README summary concise.

#### 3. Compatibility and repository verification

**File**: `loomspan-spring-boot-starter/src/test/java/com/lokiscale/loomspan/architecture/LoomspanPublicSurfaceArchitectureTest.java` (verification only)

**Changes**:

- Do not modify the supported API allowlist.
- Run the architecture test after production type changes and inspect the diff for accidental public/SPI expansion.

### Success Criteria

#### Automated Verification

- [x] Exact mapped travel invocation passes without `optionsJson`: `.\mvnw.cmd -pl loomspan-sample -am -Dtest=SampleApplicationTests,TravelCatalogServiceTest -Dsurefire.failIfNoSpecifiedTests=false test`.
- [x] Public surface remains unchanged: `.\mvnw.cmd -pl loomspan-spring-boot-starter -Dtest=LoomspanPublicSurfaceArchitectureTest test`.
- [x] Full starter suite passes: `.\mvnw.cmd -pl loomspan-spring-boot-starter test`.
- [x] Full reactor, including the sample suite, passes: `.\mvnw.cmd test`.
- [x] `git diff --check` reports no whitespace errors.
- [x] The new authoring topic's claims are supported by the named focused tests and production anchors, and the routing/coverage table points to it.

#### Manual Verification

- [x] Review provider schema, compact prompt, verbose retry prompt, validation result, and bound Java result for the same transport payload; all describe and preserve the same value kinds.
- [x] Confirm the knowledge-base topic lets an LLM decide between `Object`, a generic map, a typed map, and a DTO without loading unrelated authoring documents.
- [x] Confirm no production manifest, trace format, Console code, Go code, public API allowlist, or Spring extension point changed. A test-only mapped fixture was added as required by the testing plan.

---

## Testing Strategy

Create the dedicated testing-plan artifact with `ai/commands/3_testing_plan.md` before implementation. It must preserve the ticket's failing-test-first sequence, record the pre-fix failure output, and define exit criteria for every consumer below.

### Unit Tests

- Generate, resolve, and validate real reflected signatures for direct `Object`, generic maps, nested `List<Map<String, Object>>`, typed maps, and recursive DTOs.
- Round-trip unconstrained schemas with and without descriptions and as `additionalProperties`/`items` children.
- Accept all JSON-compatible kinds without coercing them; reject unsupported Java values and unknown internal schema kinds.
- Preserve typed scalar coercion/rejection, strict-object behavior, requiredness/optionality, null handling, attachments, runtime refs, and placeholders.
- Render compact and verbose guidance for direct unconstrained parameters, generic-map values, arrays, typed maps, and no-argument contracts.

### Integration Tests

- Register a reflected Java target and verify its provider schema and internal contract agree.
- Register a mapped YAML wrapper with no explicit input schema and verify inheritance remains the single source of truth.
- Invoke the real `rankTransportOptions` mapped skill through `SkillTemplate` with the exact native payload and assert successful ranking and scalar preservation.
- Run the architecture test and complete starter/reactor suites.

### Manual Testing Steps

1. Compare one generated tool descriptor with its compact and verbose prompt rendering.
2. Submit the motivating transport payload and inspect validation/binding results at the focused test boundary.
3. Submit an invalid typed-map value and an unsupported Java value and verify deterministic path-qualified rejection.
4. Inspect the recursive DTO schema and confirm traversal terminates with an object fallback, not an unconstrained node.

## Performance Considerations

The schema-generation and contract-resolution changes occur during target registration. Runtime unconstrained validation may recursively copy nested maps/lists, matching the existing validator's immutable normalization pattern and adding work linear in the supplied JSON value size. No new network calls, caches, schema duplication, or repeated reflection are planned. Focused tests should include nested values but no separate load benchmark is warranted unless implementation profiling reveals a regression.

## Migration Notes

No data or manifest migration is required. Existing mapped YAML skills continue to omit `input_schema`; Java remains authoritative. The behavioral correction broadens only Java `Object` leaves to their declared meaning. Authors who need planner-discoverable keys and types should migrate their own generic maps to typed records/DTOs in a separate application change, not as a prerequisite for this fix. There is no shim or cross-version schema migration.

## References

- Original ticket: `ai/thoughts/tickets/loomspan-pr-32-unconstrained-java-input-contracts.md`
- Related research: `ai/thoughts/research/2026-08-20-PR-32-unconstrained-java-input-contracts.md`
- Framework design lens: `ai/thoughts/framework-feature-design-lens.md`
- Authoring source-verification protocol: `ai/skill-authoring/source-verification.md`
- Current reflected generator: `loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/serialization/LoomspanMethodInputSchemaGenerator.java:49-120`
- Current resolver/model: `loomspan-spring-boot-starter/src/main/java/com/lokiscale/loomspan/internal/runtime/input/SkillInputContractResolver.java:109-226`
- Motivating application consumer: `loomspan-sample/src/main/java/com/lokiscale/loomspan/sample/travel/TravelCatalogService.java:202-263`
