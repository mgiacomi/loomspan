package com.lokiscale.loomspan.internal.runtime.step;

import tools.jackson.core.JacksonException;
import tools.jackson.databind.JsonNode;
import tools.jackson.databind.ObjectMapper;
import tools.jackson.databind.json.JsonMapper;
import org.junit.jupiter.api.Test;

import java.util.Map;

import static org.assertj.core.api.Assertions.assertThat;

class StepActionTest {

    private final ObjectMapper objectMapper = JsonMapper.builder().findAndAddModules().build();

    @Test
    void callTool_factoryMethod_setsFields() {
        StepAction action = StepAction.callTool("t-1", "invoiceParser", Map.of("input", "text"));
        assertThat(action.stepAction()).isEqualTo(StepActionType.CALL_TOOL);
        assertThat(action.taskId()).isEqualTo("t-1");
        assertThat(action.toolName()).isEqualTo("invoiceParser");
        assertThat(action.toolArguments()).containsEntry("input", "text");
        assertThat(action.finalResponse()).isNull();
    }

    @Test
    void finalResponse_factoryMethod_setsFields() throws JacksonException {
        JsonNode payload = objectMapper.readTree("""
                {"isDuplicate":false}
                """);
        StepAction action = StepAction.finalResponse(payload);
        assertThat(action.stepAction()).isEqualTo(StepActionType.FINAL_RESPONSE);
        assertThat(action.taskId()).isNull();
        assertThat(action.toolName()).isNull();
        assertThat(action.toolArguments()).isNull();
        assertThat(action.finalResponse()).isEqualTo(payload);
    }

    @Test
    void callTool_json_roundTrips() throws JacksonException {
        StepAction original = StepAction.callTool("t-1", "invoiceParser", Map.of("input", "text"));
        String json = objectMapper.writeValueAsString(original);
        StepAction deserialized = objectMapper.readValue(json, StepAction.class);
        assertThat(deserialized.stepAction()).isEqualTo(StepActionType.CALL_TOOL);
        assertThat(deserialized.taskId()).isEqualTo("t-1");
        assertThat(deserialized.toolName()).isEqualTo("invoiceParser");
    }

    @Test
    void finalResponse_json_roundTrips() throws JacksonException {
        JsonNode payload = objectMapper.readTree("""
                {"result":"done"}
                """);
        StepAction original = StepAction.finalResponse(payload);
        String json = objectMapper.writeValueAsString(original);
        StepAction deserialized = objectMapper.readValue(json, StepAction.class);
        assertThat(deserialized.stepAction()).isEqualTo(StepActionType.FINAL_RESPONSE);
        assertThat(deserialized.finalResponse()).isEqualTo(payload);
    }

    @Test
    void callTool_deserializesFromModelOutput() throws JacksonException {
        String modelJson = """
                {
                  "stepAction": "CALL_TOOL",
                  "taskId": "t-1",
                  "toolName": "invoiceParser",
                  "toolArguments": {"rawText": "INV-001 Acme $100"}
                }
                """;
        StepAction action = objectMapper.readValue(modelJson, StepAction.class);
        assertThat(action.stepAction()).isEqualTo(StepActionType.CALL_TOOL);
        assertThat(action.taskId()).isEqualTo("t-1");
        assertThat(action.toolName()).isEqualTo("invoiceParser");
        assertThat(action.toolArguments()).containsEntry("rawText", "INV-001 Acme $100");
    }

    @Test
    void finalResponse_deserializesFromModelOutput() throws JacksonException {
        String modelJson = """
                {
                  "stepAction": "FINAL_RESPONSE",
                  "finalResponse": {"result":"The invoice INV-001 is not a duplicate."}
                }
                """;
        StepAction action = objectMapper.readValue(modelJson, StepAction.class);
        assertThat(action.stepAction()).isEqualTo(StepActionType.FINAL_RESPONSE);
        assertThat(action.finalResponse().get("result").asText()).isEqualTo("The invoice INV-001 is not a duplicate.");
    }

    @Test
    void unknownFields_areIgnored() throws JacksonException {
        String modelJson = """
                {
                  "stepAction": "CALL_TOOL",
                  "taskId": "t-1",
                  "toolName": "invoiceParser",
                  "toolArguments": {},
                  "extraField": "should be ignored"
                }
                """;
        StepAction action = objectMapper.readValue(modelJson, StepAction.class);
        assertThat(action.stepAction()).isEqualTo(StepActionType.CALL_TOOL);
    }
}
