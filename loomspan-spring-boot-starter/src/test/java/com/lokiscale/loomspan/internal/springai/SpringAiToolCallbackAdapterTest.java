package com.lokiscale.loomspan.internal.springai;

import com.lokiscale.loomspan.internal.runtime.tool.BoundCapability;
import org.junit.jupiter.api.Test;
import org.springframework.ai.tool.ToolCallback;
import tools.jackson.databind.ObjectMapper;

import java.util.List;

import static com.lokiscale.loomspan.testkit.TestBoundCapabilities.capability;
import static org.assertj.core.api.Assertions.assertThat;

class SpringAiToolCallbackAdapterTest
{
    private final ObjectMapper objectMapper = new ObjectMapper();

    @Test
    void adaptsUnconstrainedSchemaVerbatimToSpringAiToolCallback() throws Exception
    {
        String schema = """
                {
                  "type": "object",
                  "properties": {
                    "options": {
                      "type": "object",
                      "additionalProperties": {}
                    }
                  },
                  "required": ["options"],
                  "additionalProperties": false
                }
                """;
        BoundCapability capability = capability("rankTransportOptions", schema);

        List<ToolCallback> callbacks = SpringAiToolCallbackAdapter.adapt(List.of(capability));

        assertThat(callbacks).hasSize(1);
        assertThat(callbacks.getFirst().getToolDefinition().name()).isEqualTo("rankTransportOptions");
        assertThat(callbacks.getFirst().getToolDefinition().description()).isEqualTo("rankTransportOptions");
        assertThat(objectMapper.readTree(callbacks.getFirst().getToolDefinition().inputSchema()))
                .isEqualTo(objectMapper.readTree(schema));
        assertThat(objectMapper.readTree(callbacks.getFirst().getToolDefinition().inputSchema())
                .path("properties").path("options").path("additionalProperties").has("type")).isFalse();
    }
}
