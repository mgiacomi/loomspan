package com.lokiscale.loomspan.internal.core;

import java.util.Objects;

public record CapabilityToolDescriptor(
        String name,
        String description,
        String inputSchema)
{
    private static final String GENERIC_INPUT_SCHEMA = "{\"type\":\"object\",\"additionalProperties\":true}";

    public CapabilityToolDescriptor
    {
        name = requireNonBlank(name, "name");
        description = requireNonBlank(description, "description");
        inputSchema = requireNonBlank(inputSchema, "inputSchema");
    }

    public static CapabilityToolDescriptor generic(String name, String description)
    {
        return new CapabilityToolDescriptor(name, description, GENERIC_INPUT_SCHEMA);
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
