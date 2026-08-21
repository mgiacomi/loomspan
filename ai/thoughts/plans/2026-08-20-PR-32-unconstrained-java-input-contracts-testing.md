# PR 32 — Faithful Unconstrained Java Input Contracts Testing Plan

## Change Summary

- Reflect Java `Object` as an unconstrained JSON-compatible value rather than `type: object`.
- Preserve that meaning through provider-facing JSON Schema, `SkillInputContractResolver`, the internal schema node, compact/verbose planner guidance, pre-side-effect validation, execution-router validation, `SkillTemplate`, and Java binding.
- Keep typed maps constrained, recursive DTO generation bounded and object-shaped, and required/optional, null, runtime-ref, attachment, placeholder, and strict-object behavior unchanged.
- Correct mapped YAML inheritance atomically without adding an explicit schema to `rank_transport_options.yml`, a compatibility shim, or a parallel validation path.
- Supply executable evidence for the new `ai/skill-authoring/input-contracts.md` guidance and the updated authoring routing/coverage claims.

## Impacted Areas

- **Reflection and provider schema**: `LoomspanMethodInputSchemaGenerator`, `SkillMethodBeanPostProcessor`, and the schema published on `CapabilityToolDescriptor`.
- **Internal schema model and round trip**: `SkillInputSchemaNode` and `SkillInputContractResolver`.
- **Validation and normalization**: `SkillInputValidator`, `StepActionValidator`, `CapabilityExecutionRouter`, and `DefaultSkillTemplate`.
- **Planner contract guidance**: `SkillInputPromptRenderer` as consumed by `StepPromptBuilder` in compact and forced-verbose/retry modes.
- **Mapped-skill inheritance**: `YamlSkillCapabilityRegistrar` and the existing `rank_transport_options.yml` mapping.
- **Java binding and real consumer**: `SkillMethodBeanPostProcessor` argument materialization and `TravelCatalogService#rankTransportOptions`.
- **Supported boundary**: existing `SkillMethod`, `SkillParam`, and `SkillTemplate` usage and the closed allowlist in `LoomspanPublicSurfaceArchitectureTest`.
- **Authoring evidence**: `ai/skill-authoring/README.md`, `mental-model.md`, the new `input-contracts.md`, and the mapped-Java summary in the root `README.md`.

## Risk Assessment

- **High — incomplete end-to-end fix**: emitting `{}` in the generator while the resolver still defaults a missing type to `object` would leave runtime validation broken. The primary failing test must cross generation, parsing, and validation in one test.
- **High — accidental global validation bypass**: representing unconstrained values with an arbitrary unknown string and relying on the validator's default branch would weaken input contracts. Tests must require a named internal unconstrained kind, explicit validation, and deterministic rejection of unsupported schema kinds.
- **High — recursion becomes unconstrained**: the current generator shares one branch for `Object.class` and cycle detection. A regression could turn recursive DTO fallback into an arbitrary-value escape hatch or reintroduce unbounded recursion.
- **High — typed-map regression**: broadening `Map<String, Object>` must not broaden `Map<String, String>`, `Map<String, Integer>`, or `Map<String, SomeRecord>`, nor remove current integer coercion and precise rejection paths.
- **High — consumer disagreement**: provider schema could be correct while prompts, step validation, direct-entry validation, or binding silently narrow or transform the same value. Each consumer needs one focused coherence test, plus one real mapped integration path.
- **Medium — `{}` conflated with generic top-level input**: an unconstrained value node is not the same as `SkillInputContract.GENERIC`, which represents an uncontracted top-level arguments object and intentionally bypasses detailed validation. Resolver tests must keep these cases separate.
- **Medium — prompt regression**: an open map might still be described as a no-argument tool, or verbose guidance might say values “must be an object.” Positive and negative prompt assertions are required.
- **Medium — null and JSON-kind handling**: an unconstrained present value must preserve JSON scalar/container kinds and null where existing requiredness permits it, without changing top-level required/omitted semantics.
- **Medium — mutability and non-JSON Java values**: recursive normalization must retain the validator's immutable map/list convention and reject arbitrary Java objects rather than laundering them through unconstrained input.
- **Medium — accidental public/SPI expansion**: changes to technically public internal types could accidentally alter the closed supported surface. The architecture allowlist must pass unchanged.
- **Low — runtime diagnostics**: no trace schema changes are planned. Valid generic-map calls should stop producing validation-exhaustion failures, while genuinely invalid typed values and placeholders must retain actionable path-qualified errors.

### Compatibility Scope

- **Application API — protected**: Existing `@SkillMethod`/`@SkillParam` declarations and `SkillTemplate.invoke` remain source/signature compatible and gain corrected behavior. Protect with the real sample invocation and unchanged public-surface architecture test.
- **Supported SPI — none**: Do not add a test that legitimizes an internal constructor, bean, or replacement seam as supported SPI.
- **Configuration and manifest contracts — protected with an intentional correction**: A mapped YAML skill that omits `input_schema` continues to inherit the Java target contract. Protect inheritance and single-source ownership; intentionally remove the obsolete object-only interpretation of Java `Object`.
- **Persisted or serialized contracts — current runtime only**: The provider-facing schema must be coherent and standards-valid now, but no historical schema fixture, legacy reader, or cross-version compatibility test is required.
- **Ephemeral diagnostic formats — current-run coherence**: Preserve precise validation issue paths/codes and placeholder rejection. No trace/Console fixture changes or historical trace-readability tests are in scope.
- **Internal or accidentally exposed implementation — atomic replacement**: Tests should assert the new single unconstrained representation. They must not preserve both `Object -> type: object` and `Object -> any` behind flags, fallbacks, aliases, or dual resolver behavior.

### Authoring Claims Requiring Executable Evidence

- Java `Object` accepts JSON null, string, boolean, integral/decimal number, object, and array values subject to existing requiredness rules.
- `Map<String, Object>` retains arbitrary JSON-compatible entry values, including nested objects and arrays, without scalar coercion.
- Typed maps retain their reflected value constraints and existing coercion/rejection behavior.
- Mapped YAML wrappers inherit the reflected Java input contract and must not duplicate `input_schema`.
- A typed DTO/record exposes discoverable properties and constraints, while a generic map cannot supply stable field names or domain rules.
- Planner guidance describes unconstrained leaves accurately and does not call an open generic map a no-argument capability.

## Existing Test Coverage

- `SkillMethodBeanPostProcessorTest` covers target registration, `@SkillParam` optionality/descriptions, Java binding, nested typed values, schema-valued `additionalProperties`, runtime refs, and strict empty-object classification. It does not cover reflected `Object`, generic-map values, or recursive DTO generation.
- `SkillInputContractResolverTest` currently protects attachment metadata round trips only. It has no missing-type/unconstrained schema coverage.
- `SkillInputValidatorTest` covers generic top-level permissiveness, current null behavior, typed `additionalProperties`, scalar coercion, arrays without `items`, runtime refs, and attachments. It has no explicit unconstrained node or unsupported-schema-type coverage.
- `StepActionValidatorTest` covers required arguments, nested/typed/open objects, typed maps, placeholders, and strict empty schemas. It does not prove the motivating generic-map payload passes before side effects.
- `StepPromptBuilderTest` covers concrete examples, compact/verbose nested rules, typed maps, complex map values, and arrays without `items`. It does not cover unconstrained leaves or protect against false no-argument guidance for an open generic map.
- `YamlSkillCapabilityRegistrarTests#mappedYamlSkillWithoutInputSchemaInheritsJavaDerivedContract` proves basic inheritance but does not inspect an `Object` leaf or provider schema coherence.
- `DefaultSkillTemplateTest` protects direct-entry validation and router delegation with mocked routing, but no unconstrained contract.
- `TravelCatalogServiceTest` proves native scalar-valued maps rank correctly when the Java service is called directly; it bypasses Loomspan reflection and validation.
- `SampleApplicationTests#invokesMappedYamlSkillThroughSupportedFacade` proves a mapped deterministic skill can be called through `SkillTemplate`; it is the existing pattern for the real travel integration test.
- `LoomspanPublicSurfaceArchitectureTest` is the executable authority for the closed Java API and internal-type classification.
- Baseline research on 2026-08-20 recorded 106 passing focused starter tests across the seven adjacent classes and 9 passing `TravelCatalogServiceTest` tests. The exact reflection→resolver→validator sequence is the uncovered gap.

## Bug Reproduction / Failing Test First

- **Name**: `reflectedListOfGenericMapsAcceptsScalarValuedTransportOptions`
- **Type**: Unit regression spanning three real internal components.
- **Location**: `loomspan-spring-boot-starter/src/test/java/com/lokiscale/loomspan/internal/serialization/LoomspanMethodInputSchemaGeneratorTest.java` (new).
- **Arrange**:
  - Define a local fixture method whose `options` parameter is `List<Map<String, Object>>`.
  - Reflect the real generic `Method` and generate its schema with `LoomspanMethodInputSchemaGenerator` using Loomspan's normal schema `ObjectMapper`.
  - Resolve the generated JSON through `SkillInputContractResolver.resolveJavaCapability`.
  - Build the exact ticket payload under `options`: Northeast Regional (`69.0`, `210`), Acela Express (`149.0`, `165`), and Scenic Coach (`189.0`, `360`). Include `sortBy=price` only if the fixture includes the complete motivating method signature.
- **Act**: Validate the arguments with `SkillInputValidator`.
- **Assert**:
  - `valid()` is true and issues are empty.
  - List order and all scalar fields survive in `normalizedInput()`.
  - `operator` remains `String`, `price` remains the supplied decimal numeric kind, and `durationMinutes` remains the supplied integral numeric kind.
  - Nested normalized maps/lists are immutable according to the existing validator convention.
- **Expected failure (pre-fix)**:
  - The test fails at `valid()` because the generated `additionalProperties` leaf resolves to `type=object`.
  - Issues include paths `options[0].operator`, `options[0].price`, and `options[0].durationMinutes` with `type_mismatch` / `Expected object input.`; equivalent paths repeat for all options.
  - Record this pre-fix output in the implementation handoff. Do not turn it into an assertion that permits both old and new behavior.
- **Mocks**: None. Use real generator, resolver, validator, reflection metadata, and mapper; no Spring context or provider is needed.
- **Contract classification**: Configuration and manifest contracts, because it proves the reflected contract that mapped YAML inherits.
- **Compatibility expectation**: Protected mapped-inheritance semantics with the approved removal of the erroneous `Object -> object-only` behavior.

## Tests to Add/Update

### 1. `javaObjectGeneratesUnconstrainedProviderSchema`

- **Type**: Unit.
- **Location**: New `LoomspanMethodInputSchemaGeneratorTest`.
- **What it proves**: A direct Java `Object` parameter and `Object` leaves inside maps/collections emit a value schema with no `type: object`; the top-level tool arguments schema remains `type: object`, has declared properties/requiredness, and remains closed.
- **Fixtures/data**: Fixture methods for `Object payload`, `Map<String, Object> payload`, and `List<Object> values`; inspect parsed `JsonNode`s rather than string formatting.
- **Mocks**: None.
- **Contract classification**: Persisted or serialized contracts (runtime-generated, non-durable provider schema).
- **Compatibility expectation**: Current-version schema coherence; no legacy schema assertion.

### 2. `recursiveDtoFallbackRemainsBoundedAndObjectShaped`

- **Type**: Unit.
- **Location**: New `LoomspanMethodInputSchemaGeneratorTest`.
- **What it proves**: A self-referential record/DTO terminates schema generation, keeps the cycle leaf object-shaped, and does not classify it as unconstrained merely because Java `Object` now is.
- **Fixtures/data**: A minimal recursive fixture such as `record Node(String name, Node child) {}` and a method accepting `Node`.
- **Mocks**: None.
- **Contract classification**: Internal or accidentally exposed implementation.
- **Compatibility expectation**: Preserve the recursion safeguard while atomically separating it from Java `Object` semantics.

### 3. `typedMapSchemasRemainConstrainedAfterObjectBroadening`

- **Type**: Unit parameterized or grouped fixture test.
- **Location**: New `LoomspanMethodInputSchemaGeneratorTest`, with validator assertions using the resolved generated contract.
- **What it proves**:
  - `Map<String, String>` accepts strings and rejects numbers.
  - `Map<String, Integer>` accepts integers/currently supported integer strings and rejects incompatible decimals/objects.
  - `Map<String, SomeRecord>` enforces the record's reflected properties, required fields, scalar types, and closed-object behavior.
- **Fixtures/data**: Real reflected fixture methods and a small `SomeRecord`; one accepted and one rejected payload per type, with path/code assertions.
- **Mocks**: None.
- **Contract classification**: Configuration and manifest contracts.
- **Compatibility expectation**: Protected typed reflected behavior; only deliberate `Object` leaves broaden.

### 4. `preservesUnconstrainedNodesAcrossJsonSchemaRoundTrip`

- **Type**: Unit.
- **Location**: `loomspan-spring-boot-starter/src/test/java/com/lokiscale/loomspan/internal/runtime/input/SkillInputContractResolverTest.java`.
- **What it proves**:
  - `{}` parses as the named internal unconstrained value node rather than object.
  - A nested `additionalProperties: {}` and `items: {}` preserve unconstrained meaning.
  - Serialization omits `type` for those nodes and reparsing returns the same semantics.
  - `description` and applicable Loomspan metadata survive the round trip.
  - The surrounding concrete tool arguments object is not reclassified as `GENERIC` merely because one descendant is unconstrained.
- **Fixtures/data**: Small JSON schemas for a direct `payload`, a generic map, and an array; parse the serialized JSON for structural assertions.
- **Mocks**: None; use the real resolver and mapper.
- **Contract classification**: Internal or accidentally exposed implementation.
- **Compatibility expectation**: Approved atomic replacement; do not preserve the missing-type-to-object fallback for value nodes.

### 5. `keepsGenericTopLevelAndStrictEmptyObjectContractsDistinctFromAny`

- **Type**: Unit.
- **Location**: `SkillInputContractResolverTest`, with existing strict-empty coverage retained in `SkillMethodBeanPostProcessorTest` or moved only if doing so improves ownership.
- **What it proves**: `CapabilityToolDescriptor.generic`/open empty object remains `GENERIC`, `{ "type":"object", "additionalProperties":false }` remains a concrete no-argument object, and the internal unconstrained node is neither case.
- **Fixtures/data**: Generic open-object schema, strict empty-object schema, and an unconstrained child schema.
- **Mocks**: None.
- **Contract classification**: Internal or accidentally exposed implementation.
- **Compatibility expectation**: Preserve intentional generic and strict-object paths; remove only the obsolete conflation of absent type with object.

### 6. `acceptsAndPreservesJsonKindsForUnconstrainedValues`

- **Type**: Unit.
- **Location**: `loomspan-spring-boot-starter/src/test/java/com/lokiscale/loomspan/internal/runtime/input/SkillInputValidatorTest.java`.
- **What it proves**: The explicit unconstrained branch accepts null where the field is not required, strings, booleans, integral numbers, decimal numbers, nested string-keyed maps, and lists; it performs no scalar coercion and recursively returns immutable normalized containers.
- **Fixtures/data**: One ordered map containing every JSON kind, including a nested map/list and an explicit null created with `LinkedHashMap` rather than `Map.of`.
- **Mocks**: None.
- **Contract classification**: Internal or accidentally exposed implementation.
- **Compatibility expectation**: New single internal behavior; existing top-level required/omitted semantics stay protected by the surrounding object contract tests.

### 7. `rejectsNonJsonValuesAndUnknownSchemaKindsExplicitly`

- **Type**: Unit.
- **Location**: `SkillInputValidatorTest`.
- **What it proves**: An unconstrained node rejects an arbitrary application object/resource/container kind that is not part of the supported JSON input model, and an unknown internal schema type produces a deterministic issue instead of passing through the switch default.
- **Fixtures/data**: A minimal arbitrary object and a deliberately constructed unsupported internal node; assert issue path, code, and message category without coupling to irrelevant formatting.
- **Mocks**: None.
- **Contract classification**: Internal or accidentally exposed implementation.
- **Compatibility expectation**: Approved removal of accidental unknown-type permissiveness; no fallback behavior retained.

### 8. `stepValidationAcceptsGenericMapValuesAndStillRejectsNestedPlaceholders`

- **Type**: Unit.
- **Location**: `loomspan-spring-boot-starter/src/test/java/com/lokiscale/loomspan/internal/runtime/step/StepActionValidatorTest.java`.
- **What it proves**: Pre-side-effect validation accepts the motivating scalar-valued generic-map action, but placeholder scanning still rejects strings such as `<value>` even when they occur under an unconstrained map/list leaf. Typed-map failures retain path-qualified `type_mismatch` diagnostics.
- **Fixtures/data**: A ready task/bound capability following the existing `mockTool` pattern, once with the resolved unconstrained contract and once with a nested placeholder.
- **Mocks**: Existing in-memory `ExecutionPlan` and bound-capability helper only; no model/provider calls.
- **Contract classification**: Ephemeral diagnostic formats.
- **Compatibility expectation**: Current-run diagnostic coherence and preserved pre-side-effect safeguard; no trace schema test.

### 9. `buildStepPromptDescribesUnconstrainedValuesWithoutObjectOrNoArgumentClaims`

- **Type**: Unit.
- **Location**: `loomspan-spring-boot-starter/src/test/java/com/lokiscale/loomspan/internal/runtime/step/StepPromptBuilderTest.java`.
- **What it proves**:
  - Compact guidance renders direct Object, generic-map entries, and unconstrained array items as arbitrary/any JSON values.
  - Forced-verbose guidance says map values may be any JSON value and never says they “must be an object.”
  - A map with `additionalProperties: {}` is not rendered with the no-argument note.
  - Existing typed-map guidance remains unchanged.
- **Fixtures/data**: Bound capabilities using the existing `mockTool` helper with a concrete top-level schema and unconstrained child nodes; exercise both normal and `forceVerboseToolArgumentGuidance=true` overloads.
- **Mocks**: Existing bound-capability helper; no live model.
- **Contract classification**: Configuration and manifest contracts, because this is model-facing behavior inherited from the author-declared Java contract.
- **Compatibility expectation**: Correct the author/model-visible semantics atomically; preserve typed and strict-object prompt rules.

### 10. `publishesDescriptionsAndBindsUnconstrainedReflectedInputs`

- **Type**: Unit/component.
- **Location**: `loomspan-spring-boot-starter/src/test/java/com/lokiscale/loomspan/internal/core/SkillMethodBeanPostProcessorTest.java`.
- **What it proves**: Method discovery publishes an unconstrained leaf while retaining `@SkillParam` name, description, and optionality; invocation binds direct Object and `List<Map<String, Object>>` values without changing scalar kinds. Existing typed DTO/map and runtime-ref materialization continue to pass.
- **Fixtures/data**: Add an annotated fixture bean method following existing test bean/registry patterns; inspect `SkillImplementationTarget.inputSchema()`/`inputContract()` and invoke its target with mixed values.
- **Mocks**: Existing lightweight bean factory and in-memory target registry; no Spring application context.
- **Contract classification**: Internal or accidentally exposed implementation.
- **Compatibility expectation**: Preserve application annotation behavior while replacing the internal schema shape; do not expose the unconstrained node through public API.

### 11. `mappedGenericMapTargetInheritsMatchingToolSchemaAndContract`

- **Type**: Spring context integration.
- **Location**: `loomspan-spring-boot-starter/src/test/java/com/lokiscale/loomspan/internal/skill/YamlSkillCapabilityRegistrarTests.java`.
- **What it proves**: A mapped YAML wrapper with no declared `input_schema` inherits both the Java target's provider-facing schema and internal unconstrained node; `CapabilityMetadata.tool().inputSchema()` and `inputContract()` agree, while the wrapper remains `YAML_INHERITED` and publishes its YAML name/description.
- **Fixtures/data**: Extend the existing target configuration and add a focused valid mapped manifest only if the existing fixture cannot safely gain a generic-map method; the manifest must omit `input_schema`.
- **Mocks**: Existing `ApplicationContextRunner` with real Loomspan auto-configuration; no provider/network.
- **Contract classification**: Configuration and manifest contracts.
- **Compatibility expectation**: Protected mapped inheritance and Java single-source ownership; no duplicated YAML schema.

### 12. `adaptsUnconstrainedSchemaVerbatimToSpringAiToolCallback`

- **Type**: Unit/provider-boundary adapter.
- **Location**: `loomspan-spring-boot-starter/src/test/java/com/lokiscale/loomspan/internal/springai/SpringAiToolCallbackAdapterTest.java` (new).
- **What it proves**: `SpringAiToolCallbackAdapter` passes the mapped `BoundCapability` schema into Spring AI's `ToolDefinition` without regenerating it, dropping the unconstrained child, or adding `type: object` at that child. Tool name and description also remain unchanged.
- **Fixtures/data**: A bound capability with a strict top-level arguments object containing `Map<String, Object>`-equivalent `additionalProperties: {}`; compare parsed JSON structures rather than formatting.
- **Mocks**: No provider or network. Construct a lightweight `BoundCapability`, call the package-private adapter from the same package, and inspect the returned Spring AI callback definition.
- **Contract classification**: Persisted or serialized contracts (runtime-generated, non-durable provider schema).
- **Compatibility expectation**: Current-version Java-to-provider coherence; no historical/provider-specific fallback schema.

### 13. `invokesRankTransportOptionsWithNativeScalarMapsThroughSkillTemplate`

- **Type**: Application integration/e2e within the sample Spring context.
- **Location**: `loomspan-sample/src/test/java/com/lokiscale/loomspan/sample/SampleApplicationTests.java`.
- **What it proves**: The supported `SkillTemplate` facade finds the real `rankTransportOptions` mapped YAML capability, validates the inherited reflected contract at direct entry and again in routing, binds the native list of maps, invokes `TravelCatalogService`, and returns deterministic price order with scalar fields intact.
- **Fixtures/data**: The exact three-option ticket payload and `sortBy=price`; omit `optionsJson`. Parse the returned JSON with the application `ObjectMapper` and assert the ranked operators and numeric values/types rather than only substring presence.
- **Mocks**: None. Use the existing `@SpringBootTest` sample context. This deterministic mapped skill must not call a model or external service.
- **Contract classification**: Application API.
- **Compatibility expectation**: Protected `SkillTemplate` and mapped-YAML application path with corrected behavior.

### 14. Existing travel logic and surrounding validation regressions

- **Type**: Unit/regression suite.
- **Location**: Existing `TravelCatalogServiceTest`, `DefaultSkillTemplateTest`, `CapabilityExecutionRouterTest`, and adjacent focused suites; update assertions only where they encode the approved old `Object` narrowing.
- **What it proves**: Native and JSON fallback ranking remain deterministic, numeric-string application behavior is unchanged, invalid direct-entry inputs still stop before router invocation, router validation still runs, and no unrelated validation safeguard regresses.
- **Fixtures/data**: Existing fixtures; do not add `optionsJson` to the new native-path test and do not rewrite the travel service around a DTO.
- **Mocks**: Existing Mockito use in `DefaultSkillTemplateTest`/router tests only.
- **Contract classification**: Application API.
- **Compatibility expectation**: Protected facade/application logic; approved obsolete internal expectations are updated rather than duplicated.

### 15. `LoomspanPublicSurfaceArchitectureTest` unchanged allowlist verification

- **Type**: Architecture boundary test.
- **Location**: Existing `loomspan-spring-boot-starter/src/test/java/com/lokiscale/loomspan/architecture/LoomspanPublicSurfaceArchitectureTest.java`.
- **What it proves**: No internal schema type, constructor, annotation member, or Spring extension point is added to the supported `com.lokiscale.loomspan.api` allowlist; all affected implementation types remain internal.
- **Fixtures/data**: Existing architecture scan and allowlists; no new allowed production type.
- **Mocks**: None.
- **Contract classification**: Application API.
- **Compatibility expectation**: Protected closed public surface; no supported SPI is introduced.

## Coverage Traceability

| Required behavior | Primary automated evidence |
| --- | --- |
| Exact motivating payload succeeds and preserves kinds | Failing-test-first regression; `SampleApplicationTests#invokesRankTransportOptionsWithNativeScalarMapsThroughSkillTemplate` |
| Provider schema does not claim Object is object | `javaObjectGeneratesUnconstrainedProviderSchema`; mapped registrar integration |
| Resolver round trip preserves unconstrained semantics | `preservesUnconstrainedNodesAcrossJsonSchemaRoundTrip` |
| Every supported JSON kind passes without coercion | `acceptsAndPreservesJsonKindsForUnconstrainedValues` |
| Non-JSON values/unknown internal kinds fail visibly | `rejectsNonJsonValuesAndUnknownSchemaKindsExplicitly` |
| Typed maps remain typed | `typedMapSchemasRemainConstrainedAfterObjectBroadening`; existing typed-map validator tests |
| Recursive DTO remains bounded/object-shaped | `recursiveDtoFallbackRemainsBoundedAndObjectShaped` |
| Compact/verbose guidance remains coherent | `buildStepPromptDescribesUnconstrainedValuesWithoutObjectOrNoArgumentClaims` |
| Step validation and placeholder safeguard remain active | `stepValidationAcceptsGenericMapValuesAndStillRejectsNestedPlaceholders` |
| Mapping inheritance remains single-source | `mappedGenericMapTargetInheritsMatchingToolSchemaAndContract` |
| Spring AI receives the same provider schema | `adaptsUnconstrainedSchemaVerbatimToSpringAiToolCallback` |
| Direct entry, router validation, binding, invocation agree | Sample `SkillTemplate` integration plus bean-post-processor component test |
| Requiredness, optionality, descriptions, refs, attachments, strict objects remain intact | Existing focused suites plus targeted bean-post-processor/resolver/validator assertions |
| Public surface remains closed | `LoomspanPublicSurfaceArchitectureTest` |
| New authoring guidance is evidence-backed | Generator, typed-map, prompt, registrar, and sample tests named above |

## How to Run

### Prerequisites

- Java 21 or newer.
- Maven 3.9 or newer; use the checked-in Windows wrapper.
- No API keys, live model, network service, special profile, database, or imported trace artifact is required.
- Run from `C:\opendev\code\loomspan`.

### 1. Capture the pre-fix failure

After adding only the minimal failing test and before changing production code:

```powershell
.\mvnw.cmd -pl loomspan-spring-boot-starter '-Dtest=LoomspanMethodInputSchemaGeneratorTest#reflectedListOfGenericMapsAcceptsScalarValuedTransportOptions' test
```

Expected: nonzero exit caused by the asserted successful validation; test output shows object type mismatches at scalar option fields. Preserve the failure output in the implementation notes, then implement the fix without weakening the assertion.

### 2. Run the new generator regression/matrix

```powershell
.\mvnw.cmd -pl loomspan-spring-boot-starter -Dtest=LoomspanMethodInputSchemaGeneratorTest test
```

### 3. Run all focused starter consumers

```powershell
.\mvnw.cmd -pl loomspan-spring-boot-starter '-Dtest=LoomspanMethodInputSchemaGeneratorTest,SkillMethodBeanPostProcessorTest,SkillInputContractResolverTest,SkillInputValidatorTest,StepActionValidatorTest,StepPromptBuilderTest,YamlSkillCapabilityRegistrarTests,SpringAiToolCallbackAdapterTest,DefaultSkillTemplateTest,CapabilityExecutionRouterTest,LoomspanPublicSurfaceArchitectureTest' test
```

### 4. Run the real mapped travel path and direct service coverage

```powershell
.\mvnw.cmd -pl loomspan-sample -am '-Dtest=SampleApplicationTests,TravelCatalogServiceTest' -Dsurefire.failIfNoSpecifiedTests=false test
```

### 5. Run module and repository gates

```powershell
.\mvnw.cmd -pl loomspan-spring-boot-starter test
.\mvnw.cmd test
git diff --check
```

If a Maven/Surefire version requires the fail-if-no-tests property before `-Dtest`, keep the same property values; do not replace the reactor run with ad hoc classpath execution.

## Manual Verification

1. Inspect the generated schema for direct `Object`, `Map<String, Object>`, and recursive DTO fixtures. Confirm only deliberate `Object` leaves omit `type`, while the tool arguments root and recursion fallback remain objects.
2. Compare the mapped capability's provider schema, compact prompt, forced-verbose prompt, validator result, and bound Java result for the same transport payload. Confirm all representations agree on scalar map values.
3. Inspect the sample integration input and verify `optionsJson` is absent and `rank_transport_options.yml` remains unchanged with no `input_schema`.
4. Review one invalid typed-map result, one unsupported Java-value result, and one nested placeholder result for deterministic paths/codes and useful messages.
5. Attempt to mutate normalized nested generic-map/list values in the focused validator test and confirm immutability is enforced.
6. Review `ai/skill-authoring/input-contracts.md` against the traceability table. Every enforced semantic claim must cite a focused test/source anchor; the typed DTO recommendation must be labeled guidance rather than runtime enforcement.
7. Inspect the production/test diff for aliases, fallbacks, duplicate schema models, legacy readers, configuration flags, or public API allowlist additions; none are permitted.

## Exit Criteria

- [x] The minimal reflected-path regression exists and demonstrably fails on pre-fix production code for `Expected object input.` at scalar `options[*]` fields.
- [x] The same regression passes post-fix without changing its success or value-kind assertions.
- [x] Generated Java `Object` value schemas omit `type: object`, while the top-level arguments schema remains a strict object.
- [x] Resolver parsing and serialization preserve an explicit unconstrained internal meaning, descriptions, and metadata without using `GENERIC` or unknown-type fallthrough.
- [x] Direct Object and `Map<String, Object>` validation accepts every supported JSON kind, preserves scalar kinds, recursively normalizes immutable containers, and rejects non-JSON values visibly.
- [x] `Map<String, String>`, `Map<String, Integer>`, and `Map<String, SomeRecord>` retain their constraints, coercion rules, required fields, issue paths, and closed-object semantics.
- [x] Recursive DTO generation terminates and remains object-shaped rather than unconstrained.
- [x] Compact and forced-verbose planner guidance describes unconstrained values accurately and never mistakes an open generic map for a no-argument tool.
- [x] Step-action validation accepts the motivating payload before side effects while nested placeholder rejection remains active.
- [x] Mapped YAML inheritance publishes matching provider/internal contracts without adding `input_schema` to `rank_transport_options.yml`.
- [x] `SpringAiToolCallbackAdapter` exposes that same schema to Spring AI without narrowing or regeneration.
- [x] The exact native transport payload succeeds through `SkillTemplate`, router validation, Jackson binding, and `TravelCatalogService`; `optionsJson` is absent.
- [x] Existing requiredness, optionality, null behavior outside deliberate unconstrained leaves, descriptions, runtime-ref markers, attachments, generic top-level inputs, strict objects, and travel ranking tests pass.
- [x] Tests cited as evidence for `ai/skill-authoring/input-contracts.md` establish every enforced author-facing semantic; routing and coverage updates accurately state the remaining pure-YAML coverage boundary.
- [x] The supported public API allowlist is unchanged and `LoomspanPublicSurfaceArchitectureTest` passes; no supported SPI or internal schema type is exposed.
- [x] The incorrect internal `Object -> type: object` behavior and unknown-type validator bypass are removed, with no old/new dual behavior or compatibility shim.
- [x] No trace/Console/Go/protocol fixture changes are present; current invalid-input diagnostics remain accurate and path-qualified.
- [x] All focused tests, the full starter suite, the sample suite, the full Maven reactor, and `git diff --check` pass.
- [x] All manual verification steps are complete.

## References

- Implementation plan: `ai/thoughts/plans/2026-08-20-PR-32-unconstrained-java-input-contracts.md`
- Ticket: `ai/thoughts/tickets/loomspan-pr-32-unconstrained-java-input-contracts.md`
- Research: `ai/thoughts/research/2026-08-20-PR-32-unconstrained-java-input-contracts.md`
- Framework design lens: `ai/thoughts/framework-feature-design-lens.md`
- Authoring source-verification protocol: `ai/skill-authoring/source-verification.md`
