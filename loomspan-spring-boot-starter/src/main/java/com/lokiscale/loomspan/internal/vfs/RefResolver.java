package com.lokiscale.loomspan.internal.vfs;

import com.lokiscale.loomspan.internal.core.LoomspanSession;
import com.lokiscale.loomspan.internal.runtime.input.SkillInputContract;
import com.lokiscale.loomspan.internal.runtime.input.SkillInputSchemaNode;

import java.lang.reflect.Array;
import java.util.ArrayList;
import java.util.Collections;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;

public interface RefResolver
{
    Object resolveArgument(Object value, LoomspanSession session);

    default Map<String, Object> resolveArguments(
            Map<String, Object> arguments,
            LoomspanSession session,
            SkillInputContract contract)
    {
        Map<String, Object> resolved = new LinkedHashMap<>();
        Map<String, Object> safeArguments = arguments == null ? Map.of() : arguments;
        if (contract.isGeneric())
        {
            safeArguments.forEach((key, value) -> resolved.put(key, resolveAllRefs(value, session)));
        }
        else
        {
            safeArguments.forEach((key, value) -> resolved.put(
                    key,
                    resolveContractValue(value, propertySchema(contract.schema(), key), session)));
        }
        return Collections.unmodifiableMap(resolved);
    }

    private Object resolveContractValue(Object value, SkillInputSchemaNode schema, LoomspanSession session)
    {
        if (schema == null)
        {
            return value;
        }
        if (value instanceof Map<?, ?> nestedMap && schema.isObject())
        {
            Map<Object, Object> resolvedMap = new LinkedHashMap<>();
            nestedMap.forEach((key, nestedValue) ->
            {
                SkillInputSchemaNode childSchema = propertySchema(schema, String.valueOf(key));
                resolvedMap.put(key, resolveContractValue(nestedValue, childSchema, session));
            });
            return Collections.unmodifiableMap(resolvedMap);
        }
        if (value instanceof List<?> nestedList && schema.isArray())
        {
            List<Object> resolvedList = new ArrayList<>(nestedList.size());
            nestedList.forEach(item -> resolvedList.add(resolveContractValue(item, schema.items(), session)));
            return Collections.unmodifiableList(resolvedList);
        }
        if (schema.runtimeRefCapable() || schema.isAttachment())
        {
            return resolveArgument(value, session);
        }
        return value;
    }

    private SkillInputSchemaNode propertySchema(SkillInputSchemaNode schema, String propertyName)
    {
        SkillInputSchemaNode propertySchema = schema.properties().get(propertyName);
        return propertySchema == null ? schema.additionalPropertiesSchema() : propertySchema;
    }

    private Object resolveAllRefs(Object value, LoomspanSession session)
    {
        if (value instanceof Map<?, ?> nestedMap)
        {
            Map<Object, Object> resolvedMap = new LinkedHashMap<>();
            nestedMap.forEach((key, nestedValue) -> resolvedMap.put(key, resolveAllRefs(nestedValue, session)));
            return Collections.unmodifiableMap(resolvedMap);
        }
        if (value instanceof List<?> nestedList)
        {
            List<Object> resolvedList = new ArrayList<>(nestedList.size());
            nestedList.forEach(item -> resolvedList.add(resolveAllRefs(item, session)));
            return Collections.unmodifiableList(resolvedList);
        }
        if (value != null && value.getClass().isArray())
        {
            int length = Array.getLength(value);
            List<Object> resolvedValues = new ArrayList<>(length);
            for (int index = 0; index < length; index++)
            {
                resolvedValues.add(resolveAllRefs(Array.get(value, index), session));
            }
            return Collections.unmodifiableList(resolvedValues);
        }

        return resolveArgument(value, session);
    }
}
