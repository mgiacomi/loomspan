package com.lokiscale.loomspan.internal.runtime.input;

import java.time.DateTimeException;
import java.time.LocalDate;
import java.time.format.DateTimeFormatter;
import java.util.ArrayList;
import java.util.Collections;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Locale;
import java.util.Map;
import java.util.Objects;

import org.springframework.core.io.Resource;

import java.io.InputStream;

public class SkillInputValidator
{
    public SkillInputValidationResult validate(Map<String, Object> input, SkillInputContract contract)
    {
        Objects.requireNonNull(contract, "contract must not be null");
        Map<String, Object> safeInput = input == null ? Map.of() : input;

        if (contract.isGeneric())
        {
            return new SkillInputValidationResult(true, immutableMap(safeInput), List.of());
        }

        List<SkillInputValidationIssue> issues = new ArrayList<>();
        Object normalized = validateNode(safeInput, contract.schema(), "", issues);
        Map<String, Object> normalizedMap = normalized instanceof Map<?, ?> map
                ? castMap(map)
                : Map.of();

        return new SkillInputValidationResult(issues.isEmpty(), normalizedMap, List.copyOf(issues));
    }

    @SuppressWarnings("unchecked")
    private Map<String, Object> castMap(Map<?, ?> map)
    {
        return (Map<String, Object>) map;
    }

    private Object validateNode(Object value,
            SkillInputSchemaNode schema,
            String path,
            List<SkillInputValidationIssue> issues)
    {
        return switch (schema.type())
        {
            case SkillInputSchemaNode.ANY_TYPE -> validateUnconstrained(value, path, issues);
            case "object" -> validateObject(value, schema, path, issues);
            case "array" -> validateArray(value, schema, path, issues);
            case "integer" -> validateInteger(value, schema, path, issues);
            case "number" -> validateNumber(value, schema, path, issues);
            case "boolean" -> validateBoolean(value, schema, path, issues);
            case "string" -> validateString(value, schema, path, issues);
            case "attachment" -> validateAttachment(value, schema, path, issues);
            default -> {
                issues.add(issue(path, "unsupported_schema_type",
                        "Unsupported input schema type '" + schema.type() + "'.", value));
                yield value;
            }
        };
    }

    private Object validateUnconstrained(Object value,
            String path,
            List<SkillInputValidationIssue> issues)
    {
        if (value == null || value instanceof String || value instanceof Boolean)
        {
            return value;
        }
        if (value instanceof Float floatValue)
        {
            if (Float.isFinite(floatValue))
            {
                return value;
            }
            issues.add(issue(path, "non_json_value", "Expected a JSON-compatible value.", value));
            return value;
        }
        if (value instanceof Double doubleValue)
        {
            if (Double.isFinite(doubleValue))
            {
                return value;
            }
            issues.add(issue(path, "non_json_value", "Expected a JSON-compatible value.", value));
            return value;
        }
        if (value instanceof Number)
        {
            return value;
        }
        if (value instanceof Map<?, ?> mapValue)
        {
            LinkedHashMap<String, Object> normalized = new LinkedHashMap<>();
            for (Map.Entry<?, ?> entry : mapValue.entrySet())
            {
                if (!(entry.getKey() instanceof String fieldName))
                {
                    issues.add(issue(path, "non_json_value", "JSON object keys must be strings.", entry.getKey()));
                    continue;
                }
                normalized.put(fieldName,
                        validateUnconstrained(entry.getValue(), join(path, fieldName), issues));
            }
            return immutableMap(normalized);
        }
        if (value instanceof List<?> listValue)
        {
            List<Object> normalized = new ArrayList<>();
            for (int i = 0; i < listValue.size(); i++)
            {
                normalized.add(validateUnconstrained(listValue.get(i), path + "[" + i + "]", issues));
            }
            return immutableList(normalized);
        }

        issues.add(issue(path, "non_json_value", "Expected a JSON-compatible value.", value));
        return value;
    }

    private Object validateObject(Object value,
            SkillInputSchemaNode schema,
            String path,
            List<SkillInputValidationIssue> issues)
    {
        if (!(value instanceof Map<?, ?> mapValue))
        {
            issues.add(issue(path, "type_mismatch", "Expected object input.", value));
            return Map.of();
        }

        LinkedHashMap<String, Object> normalized = new LinkedHashMap<>();
        for (String required : schema.required())
        {
            if (!mapValue.containsKey(required) || mapValue.get(required) == null)
            {
                issues.add(issue(join(path, required), "missing_required", "Required field is missing.", null));
            }
        }
        for (Map.Entry<?, ?> entry : mapValue.entrySet())
        {
            String fieldName = String.valueOf(entry.getKey());
            SkillInputSchemaNode child = schema.properties().get(fieldName);
            if (child == null)
            {
                if (!schema.allowsAdditionalProperties())
                {
                    issues.add(issue(join(path, fieldName), "unknown_field", "Unknown field is not allowed.", entry.getValue()));
                }
                else if (schema.additionalPropertiesSchema() != null)
                {
                    normalized.put(
                            fieldName,
                            validateNode(entry.getValue(), schema.additionalPropertiesSchema(), join(path, fieldName), issues));
                }
                else
                {
                    normalized.put(fieldName, entry.getValue());
                }
                continue;
            }
            normalized.put(fieldName, validateNode(entry.getValue(), child, join(path, fieldName), issues));
        }

        return immutableMap(normalized);
    }

    private Object validateArray(Object value,
            SkillInputSchemaNode schema,
            String path,
            List<SkillInputValidationIssue> issues)
    {
        if (!(value instanceof List<?> listValue))
        {
            issues.add(issue(path, "type_mismatch", "Expected array input.", value));
            return List.of();
        }
        if (schema.items() == null)
        {
            return immutableList(new ArrayList<>(listValue));
        }
        List<Object> normalized = new ArrayList<>();
        for (int i = 0; i < listValue.size(); i++)
        {
            normalized.add(validateNode(listValue.get(i), schema.items(), path + "[" + i + "]", issues));
        }
        return immutableList(normalized);
    }

    private Object validateInteger(Object value,
            SkillInputSchemaNode schema,
            String path,
            List<SkillInputValidationIssue> issues)
    {
        if (value instanceof Byte || value instanceof Short || value instanceof Integer || value instanceof Long
                || value instanceof java.math.BigInteger)
        {
            return value;
        }
        if (value instanceof String text)
        {
            try
            {
                long parsed = Long.parseLong(text);
                if (parsed >= Integer.MIN_VALUE && parsed <= Integer.MAX_VALUE)
                {
                    return (int) parsed;
                }
                return parsed;
            }
            catch (NumberFormatException ex)
            {
                issues.add(issue(path, "coercion_failed", "Could not coerce value to integer.", value));
                return value;
            }
        }
        issues.add(issue(path, "type_mismatch", "Expected integer input.", value));
        return value;
    }

    private Object validateNumber(Object value,
            SkillInputSchemaNode schema,
            String path,
            List<SkillInputValidationIssue> issues)
    {
        if (value instanceof Number)
        {
            return value;
        }
        if (value instanceof String text)
        {
            try
            {
                return Double.parseDouble(text);
            }
            catch (NumberFormatException ex)
            {
                issues.add(issue(path, "coercion_failed", "Could not coerce value to number.", value));
                return value;
            }
        }
        issues.add(issue(path, "type_mismatch", "Expected number input.", value));
        return value;
    }

    private Object validateBoolean(Object value,
            SkillInputSchemaNode schema,
            String path,
            List<SkillInputValidationIssue> issues)
    {
        if (value instanceof Boolean)
        {
            return value;
        }
        if (value instanceof String text)
        {
            String normalized = text.trim().toLowerCase(Locale.ROOT);
            if ("true".equals(normalized))
            {
                return true;
            }
            if ("false".equals(normalized))
            {
                return false;
            }
            issues.add(issue(path, "coercion_failed", "Could not coerce value to boolean.", value));
            return value;
        }
        issues.add(issue(path, "type_mismatch", "Expected boolean input.", value));
        return value;
    }

    private Object validateString(Object value,
            SkillInputSchemaNode schema,
            String path,
            List<SkillInputValidationIssue> issues)
    {
        if (acceptsRuntimeRefValue(schema, value))
        {
            return value;
        }
        if (!(value instanceof String text))
        {
            issues.add(issue(path, "type_mismatch", "Expected string input.", value));
            return value;
        }
        if (!schema.enumValues().isEmpty() && !schema.enumValues().contains(text))
        {
            issues.add(issue(path, "enum_mismatch", "Value must be one of " + schema.enumValues() + ".", value));
        }
        if ("date".equals(schema.format()))
        {
            String normalized = normalizeDate(text);
            if (normalized == null)
            {
                issues.add(issue(path, "invalid_date_format", "Date must match YYYY-MM-DD or MM/DD/YYYY style input.", value));
                return value;
            }
            return normalized;
        }
        return text;
    }

    private boolean acceptsRuntimeRefValue(SkillInputSchemaNode schema, Object value)
    {
        if (value == null || !schema.isString() || !schema.runtimeRefCapable())
        {
            return false;
        }
        return value instanceof byte[]
                || value instanceof Resource
                || value instanceof InputStream;
    }

    private Object validateAttachment(Object value,
            SkillInputSchemaNode schema,
            String path,
            List<SkillInputValidationIssue> issues)
    {
        if (value instanceof String text && text.matches("^ref://\\S+$"))
        {
            return text;
        }
        if (value instanceof Resource)
        {
            return value;
        }

        issues.add(issue(path, "type_mismatch",
                "Expected attachment input as a strict ref:// value or Spring Resource.", value));
        return value;
    }

    private String normalizeDate(String value)
    {
        if (value == null || value.isBlank())
        {
            return null;
        }

        List<DateTimeFormatter> formatters = List.of(
                DateTimeFormatter.ISO_LOCAL_DATE,
                DateTimeFormatter.ofPattern("M/d/uuuu"),
                DateTimeFormatter.ofPattern("M-d-uuuu"));

        for (DateTimeFormatter formatter : formatters)
        {
            try
            {
                return LocalDate.parse(value, formatter).toString();
            }
            catch (DateTimeException ignored)
            {
            }
        }
        return null;
    }

    private SkillInputValidationIssue issue(String path, String code, String message, Object rejectedValue)
    {
        return new SkillInputValidationIssue(path == null ? "" : path, code, message, rejectedValue);
    }

    private Map<String, Object> immutableMap(Map<String, Object> values)
    {
        return Collections.unmodifiableMap(new LinkedHashMap<>(values));
    }

    private List<Object> immutableList(List<Object> values)
    {
        return Collections.unmodifiableList(new ArrayList<>(values));
    }

    private String join(String parent, String child)
    {
        return parent == null || parent.isBlank() ? child : parent + "." + child;
    }
}
