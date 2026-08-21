package com.lokiscale.loomspan.internal.runtime.input;

import java.util.List;
import java.util.Map;
import java.util.Objects;

public record SkillInputSchemaNode(
        String type,
        Map<String, SkillInputSchemaNode> properties,
        List<String> required,
        Boolean additionalProperties,
        SkillInputSchemaNode additionalPropertiesSchema,
        SkillInputSchemaNode items,
        List<String> enumValues,
        String description,
        String format,
        boolean runtimeRefCapable,
        boolean attachment,
        String attachmentMediaType,
        List<String> allowedContentTypes)
{
    public static final String ANY_TYPE = "__loomspan_any__";

    public SkillInputSchemaNode
    {
        type = requireNonBlank(type, "type");
        properties = properties == null ? Map.of() : Map.copyOf(properties);
        required = required == null ? List.of() : List.copyOf(required);
        enumValues = enumValues == null ? List.of() : List.copyOf(enumValues);
        allowedContentTypes = allowedContentTypes == null ? List.of() : List.copyOf(allowedContentTypes);
    }

    public SkillInputSchemaNode(String type,
            Map<String, SkillInputSchemaNode> properties,
            List<String> required,
            Boolean additionalProperties,
            SkillInputSchemaNode additionalPropertiesSchema,
            SkillInputSchemaNode items,
            List<String> enumValues,
            String description,
            String format,
            boolean runtimeRefCapable)
    {
        this(type, properties, required, additionalProperties, additionalPropertiesSchema, items, enumValues,
                description, format, runtimeRefCapable, false, null, List.of());
    }

    public SkillInputSchemaNode(String type,
            Map<String, SkillInputSchemaNode> properties,
            List<String> required,
            Boolean additionalProperties,
            SkillInputSchemaNode items,
            List<String> enumValues,
            String description,
            String format,
            boolean runtimeRefCapable)
    {
        this(type, properties, required, additionalProperties, null, items, enumValues, description, format, runtimeRefCapable, false, null, List.of());
    }

    public boolean isObject()
    {
        return "object".equals(type);
    }

    public boolean isUnconstrained()
    {
        return ANY_TYPE.equals(type);
    }

    public boolean isArray()
    {
        return "array".equals(type);
    }

    public boolean isString()
    {
        return "string".equals(type);
    }

    public boolean isAttachment()
    {
        return attachment || "attachment".equals(type);
    }

    public boolean allowsAdditionalProperties()
    {
        return !Boolean.FALSE.equals(additionalProperties) || additionalPropertiesSchema != null;
    }

    private static String requireNonBlank(String value, String fieldName)
    {
        Objects.requireNonNull(value, fieldName + " must not be null");
        if (value.isBlank())
        {
            throw new IllegalArgumentException(fieldName + " must not be blank");
        }
        return value;
    }
}
