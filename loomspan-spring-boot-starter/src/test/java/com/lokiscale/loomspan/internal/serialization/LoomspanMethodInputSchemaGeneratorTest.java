package com.lokiscale.loomspan.internal.serialization;

import com.lokiscale.loomspan.internal.runtime.input.SkillInputContract;
import com.lokiscale.loomspan.internal.runtime.input.SkillInputContractResolver;
import com.lokiscale.loomspan.internal.runtime.input.SkillInputValidationResult;
import com.lokiscale.loomspan.internal.runtime.input.SkillInputValidator;
import org.junit.jupiter.api.Test;
import tools.jackson.databind.ObjectMapper;
import tools.jackson.databind.node.ObjectNode;

import java.lang.reflect.Method;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;

import static org.assertj.core.api.Assertions.assertThat;

class LoomspanMethodInputSchemaGeneratorTest
{
    private final ObjectMapper objectMapper = LoomspanJacksonCodecs.defaults().applicationConversion();
    private final LoomspanMethodInputSchemaGenerator generator = new LoomspanMethodInputSchemaGenerator(objectMapper);
    private final SkillInputContractResolver resolver = new SkillInputContractResolver(objectMapper);
    private final SkillInputValidator validator = new SkillInputValidator();

    @Test
    void reflectedListOfGenericMapsAcceptsScalarValuedTransportOptions() throws Exception
    {
        Method method = Fixture.class.getDeclaredMethod("rankTransportOptions", List.class, String.class);
        String schema = objectMapper.writeValueAsString(generator.generate(method));
        SkillInputContract contract = resolver.resolveJavaCapability(schema);

        Map<String, Object> first = option("Northeast Regional", 69.0, 210);
        Map<String, Object> second = option("Acela Express", 149.0, 165);
        Map<String, Object> third = option("Scenic Coach", 189.0, 360);
        SkillInputValidationResult result = validator.validate(Map.of(
                "options", List.of(first, second, third),
                "sortBy", "price"), contract);

        assertThat(result.issues()).isEmpty();
        assertThat(result.valid()).isTrue();
        assertThat(result.normalizedInput().get("options")).isEqualTo(List.of(first, second, third));
        @SuppressWarnings("unchecked")
        List<Map<String, Object>> normalized = (List<Map<String, Object>>) result.normalizedInput().get("options");
        assertThat(normalized.get(0).get("operator")).isInstanceOf(String.class);
        assertThat(normalized.get(0).get("price")).isInstanceOf(Double.class);
        assertThat(normalized.get(0).get("durationMinutes")).isInstanceOf(Integer.class);
    }

    @Test
    void javaObjectGeneratesUnconstrainedProviderSchema() throws Exception
    {
        ObjectNode schema = generator.generate(Fixture.class.getDeclaredMethod("acceptObject", Object.class));

        assertThat(schema.path("type").asText()).isEqualTo("object");
        assertThat(schema.path("additionalProperties").asBoolean()).isFalse();
        assertThat(schema.path("properties").path("value").isObject()).isTrue();
        assertThat(schema.path("properties").path("value").size()).isZero();

        SkillInputContract contract = resolver.resolveJavaCapability(objectMapper.writeValueAsString(schema));
        assertThat(contract.schema().properties().get("value").isUnconstrained()).isTrue();
        for (Object value : List.of("text", 7, 3.5, true, Map.of("nested", "value"), List.of(1, "two")))
        {
            SkillInputValidationResult result = validator.validate(Map.of("value", value), contract);
            assertThat(result.valid()).as("accepts %s", value).isTrue();
            assertThat(result.normalizedInput().get("value")).isEqualTo(value);
        }
    }

    @Test
    void typedMapSchemasRemainConstrainedAfterObjectBroadening() throws Exception
    {
        assertTypedMap("acceptStrings", "string", Map.of("alpha", "value"), Map.of("alpha", 1), String.class);
        assertTypedMap("acceptIntegers", "integer", Map.of("alpha", "7"), Map.of("alpha", false), Integer.class);

        Method method = Fixture.class.getDeclaredMethod("acceptRecords", Map.class);
        ObjectNode schema = generator.generate(method);
        ObjectNode valueSchema = (ObjectNode) schema.path("properties").path("values").path("additionalProperties");
        assertThat(valueSchema.path("type").asText()).isEqualTo("object");
        assertThat(valueSchema.path("properties").has("name")).isTrue();
        assertThat(valueSchema.path("additionalProperties").asBoolean()).isFalse();
        SkillInputContract contract = resolver.resolveJavaCapability(objectMapper.writeValueAsString(schema));
        assertThat(validator.validate(Map.of("values", Map.of("one", Map.of("name", "valid"))), contract).valid()).isTrue();
        SkillInputValidationResult rejected = validator.validate(
                Map.of("values", Map.of("one", Map.of("name", 4))), contract);
        assertThat(rejected.valid()).isFalse();
        assertThat(rejected.issues()).extracting(issue -> issue.path()).containsExactly("values.one.name");
    }

    @Test
    void recursiveDtoFallbackRemainsBoundedAndObjectShaped() throws Exception
    {
        ObjectNode schema = generator.generate(Fixture.class.getDeclaredMethod("acceptRecursive", RecursiveDto.class));
        ObjectNode recursiveChild = (ObjectNode) schema.path("properties").path("value")
                .path("properties").path("child");

        assertThat(recursiveChild.path("type").asText()).isEqualTo("object");
        assertThat(recursiveChild.has("properties")).isFalse();
        assertThat(recursiveChild.size()).isEqualTo(1);
    }

    private void assertTypedMap(String methodName,
            String expectedValueType,
            Map<String, Object> acceptedValue,
            Map<String, Object> rejectedValue,
            Class<?> normalizedValueType) throws Exception
    {
        ObjectNode schema = generator.generate(Fixture.class.getDeclaredMethod(methodName, Map.class));
        assertThat(schema.path("properties").path("values").path("additionalProperties").path("type").asText())
                .isEqualTo(expectedValueType);
        SkillInputContract contract = resolver.resolveJavaCapability(objectMapper.writeValueAsString(schema));
        SkillInputValidationResult accepted = validator.validate(Map.of("values", acceptedValue), contract);
        assertThat(accepted.valid()).isTrue();
        @SuppressWarnings("unchecked")
        Map<String, Object> normalized = (Map<String, Object>) accepted.normalizedInput().get("values");
        assertThat(normalized.get("alpha")).isInstanceOf(normalizedValueType);
        assertThat(validator.validate(Map.of("values", rejectedValue), contract).valid()).isFalse();
    }

    private Map<String, Object> option(String operator, double price, int durationMinutes)
    {
        LinkedHashMap<String, Object> option = new LinkedHashMap<>();
        option.put("operator", operator);
        option.put("price", price);
        option.put("durationMinutes", durationMinutes);
        return option;
    }

    @SuppressWarnings("unused")
    private static final class Fixture
    {
        void rankTransportOptions(List<Map<String, Object>> options, String sortBy)
        {
        }

        void acceptObject(Object value)
        {
        }

        void acceptStrings(Map<String, String> values)
        {
        }

        void acceptIntegers(Map<String, Integer> values)
        {
        }

        void acceptRecords(Map<String, SomeRecord> values)
        {
        }

        void acceptRecursive(RecursiveDto value)
        {
        }
    }

    private record SomeRecord(String name)
    {
    }

    private record RecursiveDto(String name, RecursiveDto child)
    {
    }
}
