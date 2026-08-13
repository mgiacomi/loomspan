package com.lokiscale.loomspan.internal.springai;

import com.lokiscale.loomspan.internal.runtime.tool.BoundCapability;
import org.springframework.ai.tool.ToolCallback;
import org.springframework.ai.tool.function.FunctionToolCallback;
import org.springframework.core.ParameterizedTypeReference;

import java.util.List;
import java.util.Map;

final class SpringAiToolCallbackAdapter
{
    private SpringAiToolCallbackAdapter() {}

    static List<ToolCallback> adapt(List<BoundCapability> capabilities)
    {
        return capabilities.stream().map(SpringAiToolCallbackAdapter::adapt).toList();
    }

    private static ToolCallback adapt(BoundCapability capability)
    {
        return FunctionToolCallback.<Map<String, Object>, Object>builder(
                        capability.name(),
                        (arguments, context) -> capability.invoke(arguments, null))
                .description(capability.description())
                .inputType(new ParameterizedTypeReference<Map<String, Object>>() {})
                .inputSchema(capability.inputSchema())
                .build();
    }
}
