package com.lokiscale.loomspan.internal.serialization;

import tools.jackson.databind.JavaType;
import tools.jackson.databind.ObjectMapper;
import tools.jackson.databind.introspect.BeanPropertyDefinition;
import tools.jackson.databind.node.ArrayNode;
import tools.jackson.databind.node.ObjectNode;

import java.lang.reflect.Method;
import java.lang.reflect.Parameter;
import java.math.BigDecimal;
import java.math.BigInteger;
import java.time.temporal.TemporalAccessor;
import java.util.HashSet;
import java.util.Objects;
import java.util.Set;

/** Generates Loomspan's mapped-capability input schema without a Spring AI dependency. */
public final class LoomspanMethodInputSchemaGenerator
{
    private static final String DRAFT_2020_12 = "https://json-schema.org/draft/2020-12/schema";

    private final ObjectMapper mapper;

    public LoomspanMethodInputSchemaGenerator(ObjectMapper mapper)
    {
        this.mapper = Objects.requireNonNull(mapper, "mapper must not be null");
    }

    public ObjectNode generate(Method method)
    {
        Objects.requireNonNull(method, "method must not be null");
        ObjectNode root = mapper.createObjectNode();
        root.put("$schema", DRAFT_2020_12);
        root.put("type", "object");
        ObjectNode properties = root.putObject("properties");
        ArrayNode required = root.putArray("required");

        for (Parameter parameter : method.getParameters())
        {
            JavaType parameterType = mapper.constructType(parameter.getParameterizedType());
            properties.set(parameter.getName(), schemaFor(parameterType, new HashSet<>()));
            required.add(parameter.getName());
        }
        root.put("additionalProperties", false);
        return root;
    }

    private ObjectNode schemaFor(JavaType type, Set<Class<?>> visiting)
    {
        ObjectNode schema = mapper.createObjectNode();
        Class<?> raw = type.getRawClass();

        if (raw == byte[].class)
        {
            schema.put("type", "array");
            schema.set("items", scalar("integer"));
        }
        else if (type.isArrayType() || type.isCollectionLikeType())
        {
            schema.put("type", "array");
            schema.set("items", schemaFor(type.getContentType(), visiting));
        }
        else if (type.isMapLikeType())
        {
            schema.put("type", "object");
            schema.set("additionalProperties", schemaFor(type.getContentType(), visiting));
        }
        else if (raw == boolean.class || raw == Boolean.class)
        {
            schema.put("type", "boolean");
        }
        else if (raw == byte.class || raw == short.class || raw == int.class || raw == long.class
                || raw == Byte.class || raw == Short.class || raw == Integer.class || raw == Long.class
                || raw == BigInteger.class)
        {
            schema.put("type", "integer");
        }
        else if (raw == float.class || raw == double.class || raw == Float.class || raw == Double.class
                || raw == BigDecimal.class || Number.class.equals(raw))
        {
            schema.put("type", "number");
        }
        else if (raw.isEnum())
        {
            schema.put("type", "string");
            ArrayNode values = schema.putArray("enum");
            for (Object value : raw.getEnumConstants())
            {
                values.add(String.valueOf(value));
            }
        }
        else if (CharSequence.class.isAssignableFrom(raw) || Character.class.equals(raw) || raw == char.class
                || TemporalAccessor.class.isAssignableFrom(raw) || raw.getPackageName().startsWith("java.time")
                || raw.getPackageName().startsWith("java.net") || raw.getPackageName().startsWith("java.nio.file"))
        {
            schema.put("type", "string");
        }
        else if (raw == Object.class)
        {
            // The empty JSON Schema is the Draft 2020-12 representation of an
            // unconstrained JSON-compatible value.
        }
        else if (!visiting.add(raw))
        {
            schema.put("type", "object");
        }
        else
        {
            schema.put("type", "object");
            ObjectNode properties = schema.putObject("properties");
            ArrayNode required = schema.putArray("required");
            for (BeanPropertyDefinition property : mapper._deserializationContext()
                    .introspectBeanDescriptionForCreation(type).findProperties())
            {
                if (property.getPrimaryMember() == null)
                {
                    continue;
                }
                properties.set(property.getName(), schemaFor(property.getPrimaryMember().getType(), visiting));
                required.add(property.getName());
            }
            schema.put("additionalProperties", false);
            visiting.remove(raw);
        }
        return schema;
    }

    private ObjectNode scalar(String type)
    {
        ObjectNode schema = mapper.createObjectNode();
        schema.put("type", type);
        return schema;
    }
}
