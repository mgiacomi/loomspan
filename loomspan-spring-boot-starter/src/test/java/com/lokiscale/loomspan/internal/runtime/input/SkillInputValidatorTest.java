package com.lokiscale.loomspan.internal.runtime.input;

import org.junit.jupiter.api.Test;
import org.springframework.core.io.ByteArrayResource;
import org.springframework.core.io.Resource;

import java.io.ByteArrayInputStream;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;

class SkillInputValidatorTest {

    private final SkillInputValidator validator = new SkillInputValidator();

    @Test
    void validatesAndNormalizesInputContractCases() {
        SkillInputContract contract = new SkillInputContract(
                SkillInputContract.SkillInputContractKind.YAML_EXPLICIT,
                new SkillInputSchemaNode(
                        "object",
                        Map.of(
                                "payload", new SkillInputSchemaNode("string", Map.of(), List.of(), null, null, List.of(), null, null, false),
                                "count", new SkillInputSchemaNode("integer", Map.of(), List.of(), null, null, List.of(), null, null, false),
                                "mode", new SkillInputSchemaNode("string", Map.of(), List.of(), null, null, List.of("A", "B"), null, null, false),
                                "options", new SkillInputSchemaNode(
                                        "object",
                                        Map.of("enabled", new SkillInputSchemaNode("boolean", Map.of(), List.of(), null, null, List.of(), null, null, false)),
                                        List.of("enabled"),
                                        Boolean.FALSE,
                                        null,
                                        List.of(),
                                        null,
                                        null,
                                        false)),
                        List.of("payload", "options"),
                        Boolean.FALSE,
                        null,
                        List.of(),
                        null,
                        null,
                        false));

        SkillInputValidationResult result = validator.validate(Map.of(
                "payload", "hello",
                "count", "3",
                "mode", "C",
                "options", Map.of(),
                "extra", "nope"), contract);

        assertThat(result.valid()).isFalse();
        assertThat(result.normalizedInput().get("count")).isEqualTo(3);
        assertThat(result.issues()).extracting(SkillInputValidationIssue::code)
                .contains("enum_mismatch", "missing_required", "unknown_field");
    }

    @Test
    void normalizesSupportedDateFormatsAndRejectsUnsupportedDates() {
        SkillInputContract contract = new SkillInputContract(
                SkillInputContract.SkillInputContractKind.YAML_EXPLICIT,
                new SkillInputSchemaNode(
                        "object",
                        Map.of("invoiceDate", new SkillInputSchemaNode("string", Map.of(), List.of(), null, null, List.of(), null, "date", false)),
                        List.of("invoiceDate"),
                        Boolean.FALSE,
                        null,
                        List.of(),
                        null,
                        null,
                        false));

        SkillInputValidationResult accepted = validator.validate(Map.of("invoiceDate", "3/30/2026"), contract);
        SkillInputValidationResult rejected = validator.validate(Map.of("invoiceDate", "2026-03-30T10:15:00"), contract);

        assertThat(accepted.valid()).isTrue();
        assertThat(accepted.normalizedInput().get("invoiceDate")).isEqualTo("2026-03-30");
        assertThat(rejected.valid()).isFalse();
        assertThat(rejected.issues()).extracting(SkillInputValidationIssue::code).containsExactly("invalid_date_format");
    }

    @Test
    void genericContractRemainsPermissive() {
        SkillInputValidationResult result = validator.validate(Map.of("anything", List.of("goes")), SkillInputContract.genericObject());

        assertThat(result.valid()).isTrue();
        assertThat(result.issues()).isEmpty();
    }

    @Test
    void preservesNullValuesInsteadOfThrowing() {
        SkillInputContract contract = new SkillInputContract(
                SkillInputContract.SkillInputContractKind.YAML_EXPLICIT,
                new SkillInputSchemaNode(
                        "object",
                        Map.of("optionalField", new SkillInputSchemaNode("string", Map.of(), List.of(), null, null, List.of(), null, null, false)),
                        List.of(),
                        Boolean.TRUE,
                        null,
                        List.of(),
                        null,
                        null,
                        false));

        java.util.LinkedHashMap<String, Object> input = new java.util.LinkedHashMap<>();
        input.put("optionalField", null);

        SkillInputValidationResult result = validator.validate(input, contract);

        assertThat(result.valid()).isFalse();
        assertThat(result.normalizedInput()).containsEntry("optionalField", null);
        assertThat(result.issues()).extracting(SkillInputValidationIssue::code).containsExactly("type_mismatch");
    }

    @Test
    void allowsRuntimeRefBackedValuesForRefFriendlyStringContracts() {
        SkillInputContract contract = new SkillInputContract(
                SkillInputContract.SkillInputContractKind.JAVA_REFLECTED,
                new SkillInputSchemaNode(
                        "object",
                        Map.of("payload", new SkillInputSchemaNode(
                                "string",
                                Map.of(),
                                List.of(),
                                null,
                                null,
                                List.of(),
                                "Provide a ref:// URI for binary content or an inline string value when appropriate.",
                                null,
                                true)),
                        List.of("payload"),
                        Boolean.FALSE,
                        null,
                        List.of(),
                        null,
                        null,
                        false));

        SkillInputValidationResult result = validator.validate(
                Map.of("payload", new ByteArrayResource(new byte[]{1, 2, 3})),
                contract);

        assertThat(result.valid()).isTrue();
        assertThat(result.normalizedInput().get("payload")).isInstanceOf(ByteArrayResource.class);
    }

    @Test
    void doesNotInferRuntimeRefSupportFromDescriptionTextAlone() {
        SkillInputContract contract = new SkillInputContract(
                SkillInputContract.SkillInputContractKind.YAML_EXPLICIT,
                new SkillInputSchemaNode(
                        "object",
                        Map.of("payload", new SkillInputSchemaNode(
                                "string",
                                Map.of(),
                                List.of(),
                                null,
                                null,
                                List.of(),
                                "This help text mentions ref:// but is not a runtime binding contract.",
                                null,
                                false)),
                        List.of("payload"),
                        Boolean.FALSE,
                        null,
                        List.of(),
                        null,
                        null,
                        false));

        SkillInputValidationResult result = validator.validate(
                Map.of("payload", new ByteArrayResource(new byte[]{1, 2, 3})),
                contract);

        assertThat(result.valid()).isFalse();
        assertThat(result.issues()).extracting(SkillInputValidationIssue::code).containsExactly("type_mismatch");
    }

    @Test
    void validatesMapLikeAdditionalPropertiesAgainstNestedSchema() {
        SkillInputContract contract = new SkillInputContract(
                SkillInputContract.SkillInputContractKind.JAVA_REFLECTED,
                new SkillInputSchemaNode(
                        "object",
                        Map.of("payload", new SkillInputSchemaNode(
                                "object",
                                Map.of(),
                                List.of(),
                                null,
                                new SkillInputSchemaNode("string", Map.of(), List.of(), null, null, List.of(), null, null, false),
                                null,
                                List.of(),
                                null,
                                null,
                                false)),
                        List.of("payload"),
                        Boolean.FALSE,
                        null,
                        List.of(),
                        null,
                        null,
                        false));

        LinkedHashMap<String, Object> payload = new LinkedHashMap<>();
        payload.put("vendor", "Acme");
        payload.put("count", 3);

        SkillInputValidationResult result = validator.validate(Map.of("payload", payload), contract);

        assertThat(result.valid()).isFalse();
        assertThat(result.normalizedInput()).containsKey("payload");
        assertThat(result.issues()).extracting(SkillInputValidationIssue::path).containsExactly("payload.count");
        assertThat(result.issues()).extracting(SkillInputValidationIssue::code).containsExactly("type_mismatch");
    }

    @Test
    void acceptsLongSizedIntegerStrings() {
        SkillInputContract contract = new SkillInputContract(
                SkillInputContract.SkillInputContractKind.JAVA_REFLECTED,
                new SkillInputSchemaNode(
                        "object",
                        Map.of("count", new SkillInputSchemaNode("integer", Map.of(), List.of(), null, null, List.of(), null, null, false)),
                        List.of("count"),
                        Boolean.FALSE,
                        null,
                        List.of(),
                        null,
                        null,
                        false));

        SkillInputValidationResult result = validator.validate(Map.of("count", "5000000000"), contract);

        assertThat(result.valid()).isTrue();
        assertThat(result.normalizedInput().get("count")).isEqualTo(5_000_000_000L);
    }

    @Test
    void acceptsArraysWithoutItemsSchema() {
        SkillInputContract contract = new SkillInputContract(
                SkillInputContract.SkillInputContractKind.JAVA_REFLECTED,
                new SkillInputSchemaNode(
                        "object",
                        Map.of("values", new SkillInputSchemaNode("array", Map.of(), List.of(), null, null, List.of(), null, null, false)),
                        List.of("values"),
                        Boolean.FALSE,
                        null,
                        List.of(),
                        null,
                        null,
                        false));

        SkillInputValidationResult result = validator.validate(Map.of("values", List.of("alpha", 2, true)), contract);

        assertThat(result.valid()).isTrue();
        assertThat(result.normalizedInput().get("values")).isEqualTo(List.of("alpha", 2, true));
    }

    @Test
    void validatesFirstPassAttachmentInputShapes() {
        SkillInputContract contract = new SkillInputContract(
                SkillInputContract.SkillInputContractKind.YAML_EXPLICIT,
                new SkillInputSchemaNode(
                        "object",
                        Map.of("image", new SkillInputSchemaNode(
                                "attachment",
                                Map.of(),
                                List.of(),
                                null,
                                null,
                                null,
                                List.of(),
                                "Ticket image",
                                null,
                                false,
                                true,
                                "image",
                                List.of("image/jpeg"))),
                        List.of("image"),
                        Boolean.FALSE,
                        null,
                        List.of(),
                        null,
                        null,
                        false));

        Resource resource = new ByteArrayResource(new byte[]{1, 2, 3});
        assertThat(validator.validate(Map.of("image", "ref://forms/ticket.jpg"), contract).valid()).isTrue();
        assertThat(validator.validate(Map.of("image", resource), contract).valid()).isTrue();

        List<Object> rejectedValues = List.of(
                "forms/ticket.jpg",
                "file:/tmp/ticket.jpg",
                "classpath:/forms/ticket.jpg",
                "data:image/jpeg;base64,AAAA",
                new byte[]{1, 2, 3},
                new ByteArrayInputStream(new byte[]{1, 2, 3}),
                42);

        for (Object rejectedValue : rejectedValues) {
            SkillInputValidationResult result = validator.validate(Map.of("image", rejectedValue), contract);
            assertThat(result.valid()).as("rejects %s", rejectedValue.getClass().getSimpleName()).isFalse();
            assertThat(result.issues()).extracting(SkillInputValidationIssue::code).containsExactly("type_mismatch");
        }
    }

    @Test
    void acceptsAndPreservesJsonKindsForUnconstrainedValues()
    {
        LinkedHashMap<String, Object> values = new LinkedHashMap<>();
        values.put("nothing", null);
        values.put("text", "alpha");
        values.put("flag", true);
        values.put("integer", 7);
        values.put("decimal", new java.math.BigDecimal("3.25"));
        values.put("object", new LinkedHashMap<>(Map.of("nested", List.of("one", 2))));
        values.put("array", new java.util.ArrayList<>(List.of(false, "two")));

        SkillInputValidationResult result = validator.validate(Map.of("value", values), contractWith(anyNode()));

        assertThat(result.valid()).isTrue();
        assertThat(result.normalizedInput().get("value")).isEqualTo(values);
        @SuppressWarnings("unchecked")
        Map<String, Object> normalized = (Map<String, Object>) result.normalizedInput().get("value");
        assertThat(normalized.get("integer")).isInstanceOf(Integer.class);
        assertThat(normalized.get("decimal")).isInstanceOf(java.math.BigDecimal.class);
        assertThatThrownBy(() -> normalized.put("later", "nope")).isInstanceOf(UnsupportedOperationException.class);
        @SuppressWarnings("unchecked")
        Map<String, Object> nested = (Map<String, Object>) normalized.get("object");
        assertThatThrownBy(() -> nested.put("later", "nope")).isInstanceOf(UnsupportedOperationException.class);
        @SuppressWarnings("unchecked")
        List<Object> array = (List<Object>) normalized.get("array");
        assertThatThrownBy(() -> array.add("nope")).isInstanceOf(UnsupportedOperationException.class);
    }

    @Test
    void rejectsNonJsonValuesAndUnknownSchemaKindsExplicitly()
    {
        SkillInputValidationResult nonJson = validator.validate(
                Map.of("value", new Object()), contractWith(anyNode()));
        SkillInputValidationResult unknown = validator.validate(
                Map.of("value", "text"), contractWith(node("mystery")));

        assertThat(nonJson.valid()).isFalse();
        assertThat(nonJson.issues()).extracting(SkillInputValidationIssue::path).containsExactly("value");
        assertThat(nonJson.issues()).extracting(SkillInputValidationIssue::code).containsExactly("non_json_value");
        assertThat(unknown.valid()).isFalse();
        assertThat(unknown.issues()).extracting(SkillInputValidationIssue::path).containsExactly("value");
        assertThat(unknown.issues()).extracting(SkillInputValidationIssue::code)
                .containsExactly("unsupported_schema_type");
    }

    private SkillInputContract contractWith(SkillInputSchemaNode child)
    {
        return new SkillInputContract(
                SkillInputContract.SkillInputContractKind.JAVA_REFLECTED,
                new SkillInputSchemaNode("object", Map.of("value", child), List.of(), Boolean.FALSE,
                        null, List.of(), null, null, false));
    }

    private SkillInputSchemaNode anyNode()
    {
        return node(SkillInputSchemaNode.ANY_TYPE);
    }

    private SkillInputSchemaNode node(String type)
    {
        return new SkillInputSchemaNode(type, Map.of(), List.of(), null, null, List.of(), null, null, false);
    }
}
