package com.lokiscale.loomspan.internal.runtime.step;

import com.lokiscale.loomspan.internal.core.ExecutionPlan;
import com.lokiscale.loomspan.internal.core.PlanStatus;
import com.lokiscale.loomspan.internal.core.PlanTask;
import com.lokiscale.loomspan.internal.core.PlanTaskStatus;
import tools.jackson.databind.json.JsonMapper;
import com.lokiscale.loomspan.internal.runtime.tool.BoundCapability;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Nested;
import org.junit.jupiter.api.Test;

import java.time.Instant;
import java.util.List;
import java.util.Map;

import static org.assertj.core.api.Assertions.assertThat;
import static com.lokiscale.loomspan.testkit.TestBoundCapabilities.capability;
import static com.lokiscale.loomspan.testkit.TestBoundCapabilities.contractAware;

class StepActionValidatorTest {

    private static final JsonMapper JSON = JsonMapper.builder().findAndAddModules().build();

    private ExecutionPlan plan;
    private List<BoundCapability> visibleTools;

    private static BoundCapability mockTool(String name) {
        return capability(name);
    }

    private static BoundCapability mockTool(String name, String inputSchema) {
        return capability(name, inputSchema);
    }

    private static BoundCapability contractAwareTool(String name, String inputSchema, String contractSchema) {
        return contractAware(name, inputSchema, contractSchema);
    }

    @BeforeEach
    void setUp() {
        PlanTask task1 = new PlanTask("t-1", "Parse invoice", PlanTaskStatus.PENDING,
                "invoiceParser", "Extract vendor, amount, date",
                List.of(), List.of("parsedInvoice"), false, null);
        PlanTask task2 = new PlanTask("t-2", "Look up expenses", PlanTaskStatus.PENDING,
                "expenseLookup", "Find matching expenses",
                List.of("t-1"), List.of("expenses"), false, null);
        plan = new ExecutionPlan("plan-1", "duplicateInvoiceChecker", Instant.now(),
                PlanStatus.VALID, null, List.of(task1, task2));
        visibleTools = List.of(mockTool("invoiceParser"), mockTool("expenseLookup"));
    }

    @Nested
    class NullAndMissingInput {
        @Test
        void nullActionRejected() {
            StepValidationResult result = StepActionValidator.validate(null, plan, visibleTools, true);
            assertThat(result.valid()).isFalse();
            assertThat(result.rejectionReason()).contains("null action");
        }

        @Test
        void nullActionTypeRejected() {
            StepAction action = new StepAction(null, "t-1", "invoiceParser", Map.of(), null);
            StepValidationResult result = StepActionValidator.validate(action, plan, visibleTools, true);
            assertThat(result.valid()).isFalse();
            assertThat(result.rejectionReason()).contains("Step action type is null");
        }
    }

    @Nested
    class CallToolValidation {
        @Test
        void validCallToolReadyTaskPasses() {
            StepAction action = StepAction.callTool("t-1", "invoiceParser", Map.of("input", "text"));
            StepValidationResult result = StepActionValidator.validate(action, plan, visibleTools, true);
            assertThat(result.valid()).isTrue();
        }

        @Test
        void bareObjectToolSchemaRemainsPermissive() {
            List<BoundCapability> genericObjectTools = List.of(mockTool("invoiceParser", "{}"));

            StepAction action = StepAction.callTool("t-1", "invoiceParser", Map.of("input", "text"));
            StepValidationResult result = StepActionValidator.validate(action, plan, genericObjectTools, true);

            assertThat(result.valid()).isTrue();
        }

        @Test
        void missingTaskIdRejected() {
            StepAction action = new StepAction(StepActionType.CALL_TOOL, null, "invoiceParser", Map.of(), null);
            StepValidationResult result = StepActionValidator.validate(action, plan, visibleTools, true);
            assertThat(result.valid()).isFalse();
            assertThat(result.rejectionReason()).contains("taskId");
        }

        @Test
        void missingToolNameRejected() {
            StepAction action = new StepAction(StepActionType.CALL_TOOL, "t-1", null, Map.of(), null);
            StepValidationResult result = StepActionValidator.validate(action, plan, visibleTools, true);
            assertThat(result.valid()).isFalse();
            assertThat(result.rejectionReason()).contains("toolName");
        }

        @Test
        void unknownTaskIdRejected() {
            StepAction action = StepAction.callTool("t-99", "invoiceParser", Map.of());
            StepValidationResult result = StepActionValidator.validate(action, plan, visibleTools, true);
            assertThat(result.valid()).isFalse();
            assertThat(result.rejectionReason()).contains("does not exist");
        }

        @Test
        void dependencyBlockedTaskRejected() {
            StepAction action = StepAction.callTool("t-2", "expenseLookup", Map.of());
            StepValidationResult result = StepActionValidator.validate(action, plan, visibleTools, true);
            assertThat(result.valid()).isFalse();
            assertThat(result.rejectionReason()).contains("not ready");
        }

        @Test
        void missingDependencyTaskRejected() {
            ExecutionPlan invalidPlan = new ExecutionPlan("plan-1", "duplicateInvoiceChecker", Instant.now(),
                    PlanStatus.VALID, null, List.of(
                    new PlanTask("t-2", "Look up expenses", PlanTaskStatus.PENDING,
                            "expenseLookup", "Find matching expenses",
                            List.of("missing-task"), List.of("expenses"), false, null)));
            StepAction action = StepAction.callTool("t-2", "expenseLookup", Map.of());
            StepValidationResult result = StepActionValidator.validate(action, invalidPlan, List.of(mockTool("expenseLookup")), true);
            assertThat(result.valid()).isFalse();
            assertThat(result.rejectionReason()).contains("not ready");
        }

        @Test
        void completedTaskRejected() {
            ExecutionPlan updated = plan.updateTask("t-1", task -> task.complete("done"));
            StepAction action = StepAction.callTool("t-1", "invoiceParser", Map.of());
            StepValidationResult result = StepActionValidator.validate(action, updated, visibleTools, true);
            assertThat(result.valid()).isFalse();
            assertThat(result.rejectionReason()).contains("not ready");
        }

        @Test
        void unknownToolRejected() {
            StepAction action = StepAction.callTool("t-1", "nonExistentTool", Map.of());
            StepValidationResult result = StepActionValidator.validate(action, plan, visibleTools, true);
            assertThat(result.valid()).isFalse();
            assertThat(result.rejectionReason()).contains("not in the available tools");
        }

        @Test
        void wrongToolForTaskRejected() {
            StepAction action = StepAction.callTool("t-1", "expenseLookup", Map.of());
            StepValidationResult result = StepActionValidator.validate(action, plan, visibleTools, true);
            assertThat(result.valid()).isFalse();
            assertThat(result.rejectionReason()).contains("expects tool 'invoiceParser'");
        }

        @Test
        void taskWithoutCapabilityBindingRejected() {
            ExecutionPlan invalidPlan = new ExecutionPlan("plan-1", "duplicateInvoiceChecker", Instant.now(),
                    PlanStatus.VALID, null, List.of(
                    new PlanTask("t-1", "Parse invoice", PlanTaskStatus.PENDING,
                            null, "Extract vendor, amount, date",
                            List.of(), List.of("parsedInvoice"), false, null)));
            StepAction action = StepAction.callTool("t-1", "invoiceParser", Map.of());
            StepValidationResult result = StepActionValidator.validate(action, invalidPlan, visibleTools, true);
            assertThat(result.valid()).isFalse();
            assertThat(result.rejectionReason()).contains("does not declare an allowed tool capability");
        }

        @Test
        void dependencySatisfiedTaskPasses() {
            ExecutionPlan updated = plan.updateTask("t-1", task -> task.complete("done"));
            StepAction action = StepAction.callTool("t-2", "expenseLookup", Map.of());
            StepValidationResult result = StepActionValidator.validate(action, updated, visibleTools, true);
            assertThat(result.valid()).isTrue();
        }

        @Test
        void missingRequiredArgumentsFromConcreteToolSchemaRejected() {
            List<BoundCapability> schemaAwareTools = List.of(mockTool("invoiceParser", """
                    {
                      "type": "object",
                      "properties": {
                        "rawText": { "type": "string" }
                      },
                      "required": ["rawText"],
                      "additionalProperties": false
                    }
                    """));
            StepAction action = StepAction.callTool("t-1", "invoiceParser", Map.of());
            StepValidationResult result = StepActionValidator.validate(action, plan, schemaAwareTools, true);
            assertThat(result.valid()).isFalse();
            assertThat(result.rejectionReason()).contains("missing_required");
            assertThat(result.rejectionReason()).contains("rawText");
        }

        @Test
        void requiredArgumentsFromConcreteToolSchemaPassWhenPresent() {
            List<BoundCapability> schemaAwareTools = List.of(mockTool("invoiceParser", """
                    {
                      "type": "object",
                      "properties": {
                        "rawText": { "type": "string" }
                      },
                      "required": ["rawText"],
                      "additionalProperties": false
                    }
                    """));
            StepAction action = StepAction.callTool("t-1", "invoiceParser", Map.of("rawText", "INV-1"));
            StepValidationResult result = StepActionValidator.validate(action, plan, schemaAwareTools, true);
            assertThat(result.valid()).isTrue();
        }

        @Test
        void genericObjectSchemaDoesNotTriggerRequiredArgumentRejection() {
            List<BoundCapability> genericTools = List.of(mockTool("invoiceParser", """
                    {
                      "type": "object",
                      "properties": {},
                      "additionalProperties": true
                    }
                    """));
            StepAction action = StepAction.callTool("t-1", "invoiceParser", Map.of());
            StepValidationResult result = StepActionValidator.validate(action, plan, genericTools, true);
            assertThat(result.valid()).isTrue();
        }

        @Test
        void nestedAndTypedToolArgumentsUseSharedValidator() {
            List<BoundCapability> schemaAwareTools = List.of(mockTool("invoiceParser", """
                    {
                      "type": "object",
                      "properties": {
                        "options": {
                          "type": "object",
                          "additionalProperties": false,
                          "required": ["enabled"],
                          "properties": {
                            "enabled": { "type": "boolean" }
                          }
                        }
                      },
                      "required": ["options"],
                      "additionalProperties": false
                    }
                    """));
            StepAction action = StepAction.callTool("t-1", "invoiceParser", Map.of("options", Map.of("enabled", "nope", "extra", true)));

            StepValidationResult result = StepActionValidator.validate(action, plan, schemaAwareTools, true);

            assertThat(result.valid()).isFalse();
            assertThat(result.rejectionReason()).contains("coercion_failed");
            assertThat(result.rejectionReason()).contains("unknown_field");
        }

        @Test
        void nestedObjectWithoutAdditionalPropertiesKeywordRemainsOpen() {
            List<BoundCapability> schemaAwareTools = List.of(mockTool("invoiceParser", """
                    {
                      "type": "object",
                      "properties": {
                        "options": {
                          "type": "object",
                          "properties": {
                            "enabled": { "type": "boolean" }
                          }
                        }
                      },
                      "required": ["options"],
                      "additionalProperties": false
                    }
                    """));

            StepAction action = StepAction.callTool("t-1", "invoiceParser", Map.of(
                    "options", Map.of("enabled", true, "extra", "allowed")));

            StepValidationResult result = StepActionValidator.validate(action, plan, schemaAwareTools, true);

            assertThat(result.valid()).isTrue();
        }

        @Test
        void contractAwareToolRejectsMissingArgumentsEvenWhenPublishedSchemaIsGeneric() {
            List<BoundCapability> tools = List.of(contractAwareTool("invoiceParser", """
                    {
                      "type": "object",
                      "properties": {},
                      "additionalProperties": true
                    }
                    """, """
                    {
                      "type": "object",
                      "properties": {
                        "rawText": { "type": "string" }
                      },
                      "required": ["rawText"],
                      "additionalProperties": false
                    }
                    """));

            StepAction action = StepAction.callTool("t-1", "invoiceParser", Map.of());
            StepValidationResult result = StepActionValidator.validate(action, plan, tools, true);

            assertThat(result.valid()).isFalse();
            assertThat(result.rejectionReason()).contains("missing_required");
            assertThat(result.rejectionReason()).contains("rawText");
        }

        @Test
        void placeholderToolArgumentsAreRejectedEvenWhenTypeValid() {
            List<BoundCapability> schemaAwareTools = List.of(mockTool("invoiceParser", """
                    {
                      "type": "object",
                      "properties": {
                        "payload": { "type": "string" }
                      },
                      "required": ["payload"],
                      "additionalProperties": false
                    }
                    """));

            StepAction action = StepAction.callTool("t-1", "invoiceParser",
                    Map.of("payload", "<canonical mission input>"));

            StepValidationResult result = StepActionValidator.validate(action, plan, schemaAwareTools, true);

            assertThat(result.valid()).isFalse();
            assertThat(result.rejectionReason()).contains("unresolved placeholder values");
            assertThat(result.rejectionReason()).contains("payload");
        }

        @Test
        void stepValidationAcceptsGenericMapValuesAndStillRejectsNestedPlaceholders()
        {
            List<BoundCapability> tools = List.of(mockTool("invoiceParser", """
                    {
                      "type": "object",
                      "properties": {
                        "options": {
                          "type": "array",
                          "items": {
                            "type": "object",
                            "additionalProperties": {}
                          }
                        }
                      },
                      "required": ["options"],
                      "additionalProperties": false
                    }
                    """));
            Map<String, Object> validOption = Map.of(
                    "operator", "Northeast Regional",
                    "price", 69.0,
                    "durationMinutes", 210);

            StepValidationResult accepted = StepActionValidator.validate(
                    StepAction.callTool("t-1", "invoiceParser", Map.of("options", List.of(validOption))),
                    plan, tools, true);
            StepValidationResult placeholder = StepActionValidator.validate(
                    StepAction.callTool("t-1", "invoiceParser", Map.of(
                            "options", List.of(Map.of("operator", "<value>", "price", 69.0)))),
                    plan, tools, true);

            assertThat(accepted.valid()).isTrue();
            assertThat(placeholder.valid()).isFalse();
            assertThat(placeholder.rejectionReason()).contains("unresolved placeholder values");
            assertThat(placeholder.rejectionReason()).contains("options[0].operator");
        }

        @Test
        void typedMapToolSchemaIsNotTreatedAsGeneric() {
            List<BoundCapability> typedMapTools = List.of(mockTool("invoiceParser", """
                    {
                      "type": "object",
                      "additionalProperties": {
                        "type": "string"
                      }
                    }
                    """));

            StepAction action = StepAction.callTool("t-1", "invoiceParser", Map.of("count", 3));
            StepValidationResult result = StepActionValidator.validate(action, plan, typedMapTools, true);

            assertThat(result.valid()).isFalse();
            assertThat(result.rejectionReason()).contains("count [type_mismatch]");
        }

        @Test
        void strictEmptyObjectToolSchemaRejectsInventedArguments() {
            List<BoundCapability> strictEmptyTools = List.of(mockTool("invoiceParser", """
                    {
                      "type": "object",
                      "additionalProperties": false
                    }
                    """));

            StepAction action = StepAction.callTool("t-1", "invoiceParser", Map.of("extra", "nope"));
            StepValidationResult result = StepActionValidator.validate(action, plan, strictEmptyTools, true);

            assertThat(result.valid()).isFalse();
            assertThat(result.rejectionReason()).contains("unknown_field");
            assertThat(result.rejectionReason()).contains("extra");
        }
    }

    @Nested
    class FinalResponseValidation {
        @Test
        void strictPolicyReadyTasksRemainRejected() {
            StepAction action = StepAction.finalResponse(JSON.createObjectNode().put("message", "All done!"));
            StepValidationResult result = StepActionValidator.validate(action, plan, visibleTools, true);
            assertThat(result.valid()).isFalse();
            assertThat(result.rejectionReason()).contains("remain incomplete");
            assertThat(result.rejectionReason()).contains("t-1(PENDING)");
            assertThat(result.rejectionReason()).contains("t-2(PENDING)");
        }

        @Test
        void strictPolicyWaitingTasksRemainRejected() {
            ExecutionPlan updated = plan.updateTask("t-1", task -> task.complete("done"));
            StepAction action = StepAction.finalResponse(JSON.createObjectNode().put("message", "All done!"));
            StepValidationResult result = StepActionValidator.validate(action, updated, visibleTools, true);
            assertThat(result.valid()).isFalse();
            assertThat(result.rejectionReason()).contains("t-2(PENDING)");
        }

        @Test
        void strictPolicyBlockedTaskRejected() {
            ExecutionPlan updated = plan
                    .updateTask("t-1", task -> task.complete("done"))
                    .updateTask("t-2", task -> task.block("tool failed"));
            StepAction action = StepAction.finalResponse(JSON.createObjectNode().put("message", "Done!"));
            StepValidationResult result = StepActionValidator.validate(action, updated, visibleTools, true);
            assertThat(result.valid()).isFalse();
            assertThat(result.rejectionReason()).contains("t-2(BLOCKED)");
        }

        @Test
        void strictPolicyAllCompletePasses() {
            ExecutionPlan updated = plan
                    .updateTask("t-1", task -> task.complete("done"))
                    .updateTask("t-2", task -> task.complete("done"));
            StepAction action = StepAction.finalResponse(JSON.createObjectNode().put("message", "All done!"));
            StepValidationResult result = StepActionValidator.validate(action, updated, visibleTools, true);
            assertThat(result.valid()).isTrue();
        }

        @Test
        void advisoryPolicyReadyTasksRemainPasses() {
            StepAction action = StepAction.finalResponse(JSON.createObjectNode().put("message", "All done!"));
            StepValidationResult result = StepActionValidator.validate(action, plan, visibleTools, false);
            assertThat(result.valid()).isTrue();
        }

        @Test
        void emptyFinalResponseRejected() {
            StepAction action = StepAction.finalResponse(JSON.nullNode());
            StepValidationResult result = StepActionValidator.validate(action, plan, visibleTools, false);
            assertThat(result.valid()).isFalse();
            assertThat(result.rejectionReason()).contains("non-empty finalResponse");
        }

        @Test
        void strictPolicyInProgressTaskRejected() {
            ExecutionPlan updated = plan
                    .updateTask("t-1", task -> task.complete("done"))
                    .updateTask("t-2", task -> task.bindInProgress("working"));
            StepAction action = StepAction.finalResponse(JSON.createObjectNode().put("message", "Done!"));
            StepValidationResult result = StepActionValidator.validate(action, updated, visibleTools, true);
            assertThat(result.valid()).isFalse();
            assertThat(result.rejectionReason()).contains("t-2(IN_PROGRESS)");
        }
    }
}
