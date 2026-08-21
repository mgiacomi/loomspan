# PR 32 — Faithful Unconstrained Java Input Contracts for Reliable Tool Calls

## Status

Proposed ticket brief. Targets the current post-PR-31 `main` checkout.

## Outcome

Make Loomspan represent Java `Object` values and generic map values faithfully
across reflected tool-schema generation, model-facing argument guidance,
internal input-contract resolution, and runtime validation.

A planner that emits arguments accepted by the mapped Java method must not be
rejected because Loomspan invented a narrower schema. In particular,
`Map<String, Object>` values must not be advertised or validated as though
every value were a JSON object.

The primary product goal is planner success: the framework must provide one
coherent contract and handle its own schema semantics instead of forcing a
model to reconcile contradictory prompt examples, provider tool schemas, and
validation errors.

## Motivation and observed failure

The motivating execution is an imported failed trace described in the
developer-provided debugging transcript:

- `traceId`: `c3cc066a-a9ed-4084-ae88-b7058fa3d05a`
- `sessionId`: `d68b3ab3-e2f1-4c00-af0d-67d84e279c11`
- Final outcome: `FAILED`
- Terminal step frame: `planTransport#step-3`, reported as frame
  `705085c4-...`
- Terminal diagnostic: `ERROR_RECORDED`, sequence 122
- Linked failure: `4065f2c0-9eda-4f1a-84c1-88cf27bf0804`
- Evidence source: `IMPORTED`; imported evidence is not authenticated
  deployment provenance and the workspace cannot prove which exact build
  produced it.

The transcript reports that the planner called `rankTransportOptions` twice
with the same compact scalar-valued options. The first reported model response
was:

```json
{
  "stepAction": "CALL_TOOL",
  "toolName": "rankTransportOptions",
  "toolArguments": {
    "options": [
      {
        "operator": "Northeast Regional",
        "price": 69.0,
        "durationMinutes": 210
      },
      {
        "operator": "Acela Express",
        "price": 149.0,
        "durationMinutes": 165
      },
      {
        "operator": "Scenic Coach",
        "price": 189.0,
        "durationMinutes": 360
      }
    ],
    "sortBy": "price"
  }
}
```

Runtime validation rejected every scalar map value, including:

```text
options[0].operator [type_mismatch]: Expected object input.
options[0].price [type_mismatch]: Expected object input.
options[0].durationMinutes [type_mismatch]: Expected object input.
```

The same pattern was reported for all three options. Validation exhausted and
the failure propagated from the nested `rankTransportOptions` call through
`planTransport` to the root `planTrip` mission.

The trace establishes the recorded failure mechanism if the transcript is
accepted as supplied evidence. The current checkout establishes the following
repository context; together they strongly identify the schema mismatch, but
the imported trace must not be described as authenticated proof that this
checkout produced the run.

## Verified current checkout behavior

### The Java method and planner expect scalar values

`loomspan-sample/src/main/java/com/lokiscale/loomspan/sample/travel/TravelCatalogService.java`
declares:

```java
public Map<String, Object> rankTransportOptions(
        List<Map<String, Object>> options,
        String optionsJson,
        String sortBy)
```

Its `@SkillParam` description shows scalar `airline`, `price`, and
`durationMinutes` values. `TravelCatalogServiceTest` passes scalar-valued maps
through both native `options` and serialized `optionsJson` paths and verifies
successful deterministic ranking.

`loomspan-sample/src/main/resources/skills/travel/plan_transport.yml` tells the
planner to copy compact scalar fields and gives this representative shape:

```text
options=[{"airline":"Pacific Jet","price":210.0,"durationMinutes":125}, ...]
```

The mapped `rank_transport_options.yml` intentionally omits `input_schema`, so
the Java target's reflected contract is the authoritative mapped-skill input
contract.

### Reflected schema generation narrows `Object` incorrectly

`LoomspanMethodInputSchemaGenerator.schemaFor` currently generates map schemas
as:

```json
{
  "type": "object",
  "additionalProperties": "<schema generated for the map value type>"
}
```

The same method maps `Object.class` to:

```json
{"type":"object"}
```

Consequently, `List<Map<String, Object>>` becomes semantically equivalent to:

```json
{
  "type": "array",
  "items": {
    "type": "object",
    "additionalProperties": {"type":"object"}
  }
}
```

Strings, numbers, integers, and booleans inside each option therefore fail
before the Java method is invoked.

### The mismatch is carried through the whole runtime

- `SkillMethodBeanPostProcessor` publishes the generated JSON schema and also
  resolves it into the Java target's internal `SkillInputContract`.
- `YamlSkillCapabilityRegistrar` gives a mapped YAML capability the target's
  reflected schema and contract when the wrapper omits `input_schema`.
- `SkillInputContractResolver` converts object-valued
  `additionalProperties` into an `additionalPropertiesSchema`.
- `SkillInputValidator` validates each unknown map entry against that schema.
  Because it is currently `type=object`, scalar entries produce the observed
  `Expected object input` errors.
- `SkillInputPromptRenderer` renders the same contract into compact and verbose
  planner guidance. With the current contract it can tell the planner that
  arbitrary map values must be objects, reinforcing the incorrect constraint.
- `StepLoopMissionExecutionEngine` appends the complete rejection reason to
  the retry prompt. The repeated model output does not prove that feedback was
  absent; the current code explicitly supplies it. A contradictory original
  example and reflected schema can still make correction impossible or
  counterproductive.

## Required behavior

### 1. Preserve Java `Object` semantics

For reflected Java tool inputs, `Object` means an unconstrained JSON-compatible
value, subject to Loomspan's existing requiredness and null/omission rules. A
non-null present value may be a string, number, boolean, object, or array.

The implementation may choose the standards-valid provider-schema encoding,
but must preserve the same meaning after Loomspan parses the schema into its
internal contract. Merely emitting syntax that is later reinterpreted as
`type=object` is not a fix.

### 2. Preserve generic map semantics

For `Map<String, Object>` and nested occurrences such as
`List<Map<String, Object>>`, arbitrary entry values must pass through unchanged
when their values are otherwise valid JSON-compatible inputs. The contract
must not require those values to be objects.

Typed maps must remain typed. For example:

- `Map<String, String>` accepts string values and rejects non-strings.
- `Map<String, Integer>` accepts/coerces values according to existing integer
  rules and rejects incompatible values.
- `Map<String, SomeRecord>` retains the reflected object/property constraints
  for `SomeRecord`.

### 3. Keep every contract consumer coherent

The following representations must agree:

1. The JSON Schema published to the model/provider tool descriptor.
2. The internal `SkillInputContract` produced from that schema.
3. Compact and verbose tool-argument guidance rendered into planner prompts.
4. Step-action validation before a mapped Java tool call.
5. Direct mapped-skill entry validation through `SkillTemplate`.
6. Java argument binding and invocation.

No layer may silently narrow an unconstrained value back to `object`.

### 4. Do not conflate unconstrained values with recursion protection

`LoomspanMethodInputSchemaGenerator` currently handles `Object.class` and an
already-visiting Java type in the same branch. These are different semantics:

- `Object.class` is an intentionally unconstrained value.
- An already-visiting type is recursion/cycle protection during schema
  generation.

The implementation must separate them. Recursive DTO handling must remain
bounded and must not become an arbitrary-value escape hatch merely because
`Object` becomes unconstrained.

### 5. Favor planner success without inventing information

Loomspan must faithfully communicate constraints it can derive from the Java
type and `@SkillParam` metadata. It must not invent a narrower constraint to
make internal representation easier.

Conversely, the framework cannot infer stable property names or domain rules
from `Map<String, Object>`. Authors who need a richer model-facing contract
should continue to use a typed Java DTO/record and precise parameter
description. This ticket does not add inferred map keys, domain-specific
schemas, or hidden argument repair.

## Explicitly rejected incomplete fix

Do not implement only this generator change:

```java
else if (raw == Object.class || !visiting.add(raw)) {
    // return an empty ObjectNode
}
```

It is incomplete for two independent reasons:

1. `SkillInputContractResolver.fromJsonNode` currently defaults a schema with
   no textual `type` to `object`. An emitted `{}` therefore becomes an object
   constraint internally and recreates the same validation failure.
2. Keeping `Object.class` and recursion detection in one branch would also
   make recursive-type fallbacks unconstrained.

The fix must be verified through the entire schema-generation-to-validation
path, not by asserting only the generator's raw JSON text.

## In scope

- Correct reflected schema generation for Java `Object` and
  `Map<String, Object>` values, including nested arrays/collections/maps.
- Add or adjust an explicit internal representation for an unconstrained value
  if required; do not rely on an unknown string type as an accidental validator
  bypass.
- Update JSON-schema parsing and serialization so the unconstrained meaning
  survives round trips used by Loomspan.
- Update model prompt rendering so unconstrained values are described as
  values, not objects, and open maps are not described as no-argument objects.
- Preserve typed-map validation and recursive-type bounds.
- Add focused unit and integration coverage, including the exact motivating
  `rankTransportOptions` arguments.
- Update skill-authoring guidance for reflected Java input contracts so an LLM
  author understands both:
  - Loomspan's enforced semantics for `Object` and generic maps; and
  - why a typed DTO/record is preferred when the planner needs discoverable
    field names and types.
- Update `ai/skill-authoring/README.md` routing/coverage only if the approved
  plan adds or materially expands an input-contract topic.

## Guardrails

- Do not solve this by weakening all object validation or by skipping tool
  argument validation.
- Do not add a `rankTransportOptions`-specific validator exception.
- Do not duplicate an `input_schema` into the mapped
  `rank_transport_options.yml`; Java remains the single authoritative contract
  for the mapped target.
- Do not ask the model to serialize `options` as a string merely to bypass the
  reflected native-array contract. `optionsJson` remains an application-level
  fallback, not the framework fix.
- Do not silently coerce scalars into singleton objects or otherwise repair
  model intent.
- Do not change existing required/optional parameter behavior as part of this
  ticket. If explicit top-level JSON `null` exposes a separate null-preservation
  defect, record it independently unless it is strictly necessary to implement
  the chosen faithful `Object` representation.
- Do not add compatibility shims, parallel schema models, legacy readers, or
  dual validation behavior. Update the current pre-1.0 implementation
  coherently and atomically.
- Do not expand the supported Java API or expose internal schema types through
  `com.lokiscale.loomspan.api`.
- Do not change trace/MCP formats or the Console debugging skill in this PR.

## Contract and compatibility classification

- **Application API:** No signature change is expected to the allowlisted
  `com.lokiscale.loomspan.api` types. Existing `@SkillMethod` and `@SkillParam`
  usage obtains corrected behavior.
- **Supported SPI:** No supported SPI exists or should be introduced.
- **Configuration and manifest contracts:** Affected. Mapped YAML skills that
  omit `input_schema` deliberately inherit the reflected Java target contract.
  Correcting `Object`/generic-map semantics changes author-facing validation
  and model guidance to match the Java declaration. Update relevant
  skill-authoring guidance in the same PR.
- **Persisted or serialized contracts:** No durable or cross-version format is
  intended. Provider tool schemas are runtime-generated model-facing data and
  must remain coherent with the current runtime.
- **Ephemeral diagnostic formats:** No trace schema change is planned. After
  the fix, valid generic-map calls should produce validated/completed tool
  records rather than validation-exhaustion failures.
- **Internal or accidentally exposed implementation:** Generator, resolver,
  schema-node, prompt-renderer, and validator changes are internal and may be
  changed atomically without a compatibility shim.
- **Public-surface delta:** None expected. Run
  `LoomspanPublicSurfaceArchitectureTest` after changing production types.
- **Shim decision:** No shim.
- **Java-to-Go boundary coordination:** Not required. No application-adapter
  REST/SSE, acquisition, problem, or consumed NDJSON contract is changed.

## Required tests and acceptance signals

### Failing regression first

Add a focused test that fails on the current checkout and exercises the
complete reflected path rather than constructing a hand-authored permissive
contract. At minimum it must:

1. Register or generate the contract for a Java method accepting
   `List<Map<String, Object>>`.
2. Resolve the generated schema into Loomspan's internal input contract.
3. Validate the exact `rankTransportOptions` payload from the motivating
   failure.
4. Assert that validation succeeds and preserves each scalar value and its
   value kind.

### Focused coverage

- The generated provider-facing schema represents `Object` without claiming
  `type=object`.
- A generated schema round trip through `SkillInputContractResolver` preserves
  unconstrained semantics.
- `Map<String, Object>` accepts string, integral number, decimal number,
  boolean, nested object, and array values without unwanted coercion.
- Direct `Object` parameters accept the supported non-null JSON value kinds.
- `List<Map<String, Object>>` accepts the motivating transport payload.
- `Map<String, String>`, `Map<String, Integer>`, and
  `Map<String, SomeRecord>` retain their existing type enforcement.
- Recursive DTO generation remains bounded and retains deliberate object
  semantics rather than becoming unconstrained.
- Compact and verbose prompt rendering do not say that unconstrained map
  values "must be an object" and do not mistake an open object for a
  no-argument tool.
- A mapped YAML capability inheriting the Java target contract validates and
  invokes the scalar-valued transport payload successfully.
- Existing requiredness, optionality, descriptions, attachment/runtime-ref
  markers, and strict object behavior remain covered.

### Repository verification

- The narrow failing test demonstrably fails before the production fix and
  passes after it.
- Relevant starter unit/integration tests pass.
- `TravelCatalogServiceTest` passes without routing the native array through
  `optionsJson`.
- The starter module test suite passes.
- `LoomspanPublicSurfaceArchitectureTest` passes and its supported API allowlist
  is unchanged.
- Skill-authoring documentation accurately describes the implemented behavior
  and cites focused executable evidence.
- No production type is added to the supported public API allowlist.

## Out of scope

- Inferring a fixed property schema from arbitrary `Map<String, Object>` usage.
- Adding union, `oneOf`, discriminator, or domain-specific schema generation
  unless the implementation plan proves it is necessary for faithful
  unconstrained-value semantics.
- Redesigning `rankTransportOptions` around a typed transport DTO. That may be
  considered separately to provide richer domain guidance, but it does not
  replace the framework correctness fix.
- General argument auto-repair, prompt retry redesign, or model-specific
  workarounds.
- Changing the YAML `input_schema` language.
- Changing output-schema validation.
- Console MCP, trace storage, or runtime-debugging skill changes.
- Historical or cross-version schema compatibility.

## Implementation-process handoff

Run the repository's full five-step development process against this ticket:

1. `ai/commands/1_research_codebase.md`
2. `ai/commands/2_create_plan.md`
3. `ai/commands/3_testing_plan.md`
4. `ai/commands/4_implement_plan.md`
5. `ai/commands/5_code_review.md`

The research and plan must read `ai/thoughts/framework-feature-design-lens.md`
and preserve the contract classification above. The testing plan must require
the failing end-to-end reflected-contract regression before implementation.

The implementation must not assume that changing only
`LoomspanMethodInputSchemaGenerator` is sufficient. It must trace and verify
every consumer named under **Keep every contract consumer coherent**.
