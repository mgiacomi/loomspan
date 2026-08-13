package com.lokiscale.loomspan.internal.outputschema;

import tools.jackson.core.JacksonException;
import tools.jackson.databind.JsonNode;
import tools.jackson.databind.ObjectMapper;
import com.lokiscale.loomspan.internal.skill.YamlSkillManifest;
import org.springframework.util.StringUtils;

import java.util.ArrayList;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Locale;
import java.util.Map;
import java.util.Objects;

public final class OutputSchemaValidator
{
    private final ObjectMapper objectMapper;

    public OutputSchemaValidator()
    {
        this(com.lokiscale.loomspan.internal.serialization.LoomspanJacksonCodecs.defaults().schemaTree());
    }

    public OutputSchemaValidator(ObjectMapper objectMapper)
    {
        this.objectMapper = Objects.requireNonNull(objectMapper, "objectMapper must not be null");
    }

    public OutputSchemaValidationResult validate(String rawOutput, YamlSkillManifest.OutputSchemaManifest schema)
    {
        Objects.requireNonNull(schema, "schema must not be null");
        if (!StringUtils.hasText(rawOutput))
        {
            return OutputSchemaValidationResult.failed(
                    OutputSchemaFailureMode.INVALID_JSON,
                    List.of(new OutputSchemaValidationIssue("$", "Response is not valid JSON.", null)));
        }

        JsonNode root;
        try
        {
            root = objectMapper.readTree(rawOutput);
        }
        catch (JacksonException ex)
        {
            return OutputSchemaValidationResult.failed(
                    OutputSchemaFailureMode.INVALID_JSON,
                    List.of(new OutputSchemaValidationIssue("$", "Response is not valid JSON.", null)));
        }

        if (root == null)
        {
            return OutputSchemaValidationResult.failed(
                    OutputSchemaFailureMode.INVALID_JSON,
                    List.of(new OutputSchemaValidationIssue("$", "Response is not valid JSON.", null)));
        }

        List<OutputSchemaValidationIssue> issues = new ArrayList<>();
        validateNode(root, schema, "$", null, issues);

        if (issues.isEmpty())
        {
            return OutputSchemaValidationResult.passed();
        }

        return OutputSchemaValidationResult.failed(OutputSchemaFailureMode.SCHEMA_VALIDATION_FAILED, issues);
    }

    private void validateNode(JsonNode node,
            YamlSkillManifest.OutputSchemaManifest schema,
            String path,
            String canonicalField,
            List<OutputSchemaValidationIssue> issues)
    {
        if (node.isNull() && Boolean.TRUE.equals(schema.getNullable()))
        {
            return;
        }
        switch (schema.getType())
        {
            case "object" -> validateObject(node, schema, path, canonicalField, issues);
            case "array" -> validateArray(node, schema, path, canonicalField, issues);
            case "string" -> validateString(node, schema, path, canonicalField, issues);
            case "number" -> validateType(node.isNumber(), path, canonicalField, "should be a number", issues);
            case "integer" -> validateType(node.isIntegralNumber(), path, canonicalField, "should be an integer", issues);
            case "boolean" -> validateType(node.isBoolean(), path, canonicalField, "should be a boolean", issues);
            default -> issues.add(new OutputSchemaValidationIssue(path, "Unsupported schema type '" + schema.getType() + "'.", canonicalField));
        }
    }

    private void validateObject(JsonNode node,
            YamlSkillManifest.OutputSchemaManifest schema,
            String path,
            String canonicalField,
            List<OutputSchemaValidationIssue> issues)
    {
        if (!node.isObject())
        {
            issues.add(new OutputSchemaValidationIssue(path, "should be an object", canonicalField));
            return;
        }

        Map<String, String> canonicalByLowercase = new LinkedHashMap<>();
        schema.getProperties().keySet().forEach(property -> canonicalByLowercase.put(property.toLowerCase(Locale.ROOT), property));

        Map<String, List<String>> actualByLowercase = new LinkedHashMap<>();
        node.propertyNames().forEach(fieldName -> actualByLowercase.computeIfAbsent(fieldName.toLowerCase(Locale.ROOT), ignored -> new ArrayList<>()).add(fieldName));

        for (Map.Entry<String, List<String>> entry : actualByLowercase.entrySet())
        {
            String canonicalName = canonicalByLowercase.get(entry.getKey());
            if (entry.getValue().size() > 1)
            {
                issues.add(new OutputSchemaValidationIssue(
                        path,
                        "ambiguous fields " + entry.getValue() + " differ only by case",
                        canonicalName));
                continue;
            }

            String actualField = entry.getValue().getFirst();
            if (canonicalName == null)
            {
                if (!Boolean.TRUE.equals(schema.getAdditionalProperties()))
                {
                    issues.add(new OutputSchemaValidationIssue(pathOf(path, actualField), "unknown field '" + actualField + "'", actualField));
                }
                continue;
            }

            validateNode(node.get(actualField), schema.getProperties().get(canonicalName), pathOf(path, canonicalName), canonicalName, issues);
        }

        for (String requiredField : schema.getRequired())
        {
            if (!actualByLowercase.containsKey(requiredField.toLowerCase(Locale.ROOT)))
            {
                issues.add(new OutputSchemaValidationIssue(
                        pathOf(path, requiredField),
                        "missing required field '" + requiredField + "'",
                        requiredField));
            }
        }
    }

    private void validateArray(JsonNode node,
            YamlSkillManifest.OutputSchemaManifest schema,
            String path,
            String canonicalField,
            List<OutputSchemaValidationIssue> issues)
    {
        if (!node.isArray())
        {
            issues.add(new OutputSchemaValidationIssue(path, "should be an array", canonicalField));
            return;
        }
        for (int index = 0; index < node.size(); index++)
        {
            validateNode(node.get(index), schema.getItems(), path + "[" + index + "]", canonicalField, issues);
        }
    }

    private void validateString(JsonNode node, YamlSkillManifest.OutputSchemaManifest schema, String path, String canonicalField, List<OutputSchemaValidationIssue> issues)
    {
        if (!node.isTextual())
        {
            issues.add(new OutputSchemaValidationIssue(path, "should be a string", canonicalField));
            return;
        }
        if (!schema.getEnumValues().isEmpty() && !schema.getEnumValues().contains(node.textValue()))
        {
            issues.add(new OutputSchemaValidationIssue(
                    path,
                    "must be one of " + schema.getEnumValues(),
                    canonicalField));
        }
    }

    private void validateType(boolean condition, String path, String canonicalField, String message, List<OutputSchemaValidationIssue> issues)
    {
        if (!condition)
        {
            issues.add(new OutputSchemaValidationIssue(path, message, canonicalField));
        }
    }

    private String pathOf(String parent, String child)
    {
        return "$".equals(parent) ? "$." + child : parent + "." + child;
    }
}
