package com.lokiscale.loomspan.internal.runtime.input;

import java.util.List;
import java.util.Map;
import java.util.TreeMap;

public class SkillInputPromptRenderer
{
    public enum DetailLevel
    {
        COMPACT,
        VERBOSE
    }

    public String renderToolArgumentsExample(SkillInputContract contract, DetailLevel detailLevel)
    {
        if (contract == null || contract.isGeneric())
        {
            return "";
        }

        if (contract.schema().isObject()
                && contract.schema().properties().isEmpty()
                && contract.schema().additionalPropertiesSchema() == null
                && !contract.schema().allowsAdditionalProperties())
        {
            return "{}\n(Note: This tool takes no arguments. You must pass an empty object.)";
        }

        StringBuilder builder = new StringBuilder();
        builder.append(renderValue(contract.schema(), 0));

        if (detailLevel == DetailLevel.VERBOSE)
        {
            appendVerboseRules(builder, contract.schema(), "", 0, 3);
        }

        return builder.toString();
    }

    private String renderValue(SkillInputSchemaNode schema, int depth)
    {
        String indent = "  ".repeat(depth);
        if (schema.isObject())
        {
            StringBuilder builder = new StringBuilder();
            builder.append("{\n");
            List<String> propertyNames = schema.properties().keySet().stream().sorted().toList();
            int entryCount = propertyNames.size() + (schema.additionalPropertiesSchema() != null ? 1 : 0);
            int entryIndex = 0;

            for (String propertyName : propertyNames)
            {
                builder.append(indent)
                        .append("  \"")
                        .append(propertyName)
                        .append("\": ")
                        .append(renderValue(schema.properties().get(propertyName), depth + 1));
                entryIndex++;
                if (entryIndex < entryCount)
                {
                    builder.append(",");
                }
                builder.append("\n");
            }

            if (schema.additionalPropertiesSchema() != null)
            {
                builder.append(indent)
                        .append("  \"<key>\": ")
                        .append(renderValue(schema.additionalPropertiesSchema(), depth + 1))
                        .append("\n");
            }

            builder.append(indent).append("}");
            return builder.toString();
        }
        if (schema.isArray())
        {
            return schema.items() == null
                    ? "[ \"<value>\" ]"
                    : "[ " + renderValue(schema.items(), depth + 1) + " ]";
        }
        if (!schema.enumValues().isEmpty())
        {
            return "\"<one of: " + String.join(", ", schema.enumValues()) + ">\"";
        }
        
        return switch (schema.type())
        {
            case SkillInputSchemaNode.ANY_TYPE -> "<any JSON value>";
            case "string" -> "\"<string>\"";
            case "number", "integer" -> "<number>";
            case "boolean" -> "<boolean>";
            default -> "\"<value>\"";
        };
    }

    private void appendVerboseRules(StringBuilder builder,
            SkillInputSchemaNode schema,
            String path,
            int depth,
            int maxDepth)
    {
        if (schema == null || depth > maxDepth)
        {
            return;
        }
        if (!schema.required().isEmpty())
        {
            String requiredFields = schema.required().stream()
                    .map(field -> path == null || path.isBlank() ? field : path + "." + field)
                    .reduce((left, right) -> left + ", " + right)
                    .orElse("");
            builder.append("\nRequired fields: ").append(requiredFields);
        }
        if (schema.isObject() && !schema.allowsAdditionalProperties())
        {
            builder.append("\n");
            if (path == null || path.isBlank())
            {
                builder.append("Do not add fields not shown above.");
            }
            else
            {
                builder.append("Do not add fields under `").append(path).append("` beyond those shown above.");
            }
        }
        if (schema.additionalPropertiesSchema() != null)
        {
            String mapPath = path == null || path.isBlank() ? "<key>" : path + ".<key>";
            SkillInputSchemaNode additionalSchema = schema.additionalPropertiesSchema();
            builder.append("\n`").append(mapPath).append("` must be ").append(typeDescription(additionalSchema));
            if (!additionalSchema.enumValues().isEmpty())
            {
                builder.append(" with one of ").append(additionalSchema.enumValues());
            }
            if (additionalSchema.isArray() && additionalSchema.items() != null)
            {
                builder.append("\n`").append(mapPath).append("[]` items must be ")
                        .append(typeDescription(additionalSchema.items()));
            }
            if (additionalSchema.additionalPropertiesSchema() != null)
            {
                builder.append("\n`").append(mapPath).append(".*` values must be ")
                        .append(typeDescription(additionalSchema.additionalPropertiesSchema()));
            }
            if (additionalSchema.isObject() || additionalSchema.isArray())
            {
                appendVerboseRules(builder, nestedSchema(additionalSchema),
                        additionalSchema.isArray() ? mapPath + "[]" : mapPath, depth + 1, maxDepth);
            }
        }
        for (Map.Entry<String, SkillInputSchemaNode> entry : new TreeMap<>(schema.properties()).entrySet())
        {
            SkillInputSchemaNode child = entry.getValue();
            String childPath = path == null || path.isBlank() ? entry.getKey() : path + "." + entry.getKey();
            builder.append("\n`").append(childPath).append("` must be ").append(typeDescription(child));

            if (!child.enumValues().isEmpty())
            {
                builder.append(" with one of ").append(child.enumValues());
            }
            if (child.isArray() && child.items() != null)
            {
                builder.append("\n`").append(childPath).append("[]` items must be ")
                        .append(typeDescription(child.items()));
            }
            if (child.additionalPropertiesSchema() != null)
            {
                builder.append("\n`").append(childPath).append(".*` values must be ")
                        .append(typeDescription(child.additionalPropertiesSchema()));
            }
            if (child.isObject() || child.isArray())
            {
                appendVerboseRules(builder, nestedSchema(child),
                        child.isArray() ? childPath + "[]" : childPath, depth + 1, maxDepth);
            }
        }
    }

    private SkillInputSchemaNode nestedSchema(SkillInputSchemaNode schema)
    {
        if (schema == null)
        {
            return null;
        }
        return schema.isArray() ? schema.items() : schema;
    }

    private String typeDescription(SkillInputSchemaNode schema)
    {
        return schema.isUnconstrained() ? "any JSON value" : "a " + schema.type();
    }
}
