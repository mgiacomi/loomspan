package com.lokiscale.loomspan.internal.core;

import com.lokiscale.loomspan.internal.runtime.input.SkillInputValidationResult;
import com.lokiscale.loomspan.internal.runtime.input.SkillInputValidator;
import com.lokiscale.loomspan.internal.runtime.state.SuccessfulSkillSnapshot;
import com.lokiscale.loomspan.internal.runtime.state.ExecutionStateService;
import com.lokiscale.loomspan.internal.runtime.state.PlanSnapshot;
import com.lokiscale.loomspan.api.SkillInputValidationException;
import com.lokiscale.loomspan.api.SkillInputValidationIssue;
import com.lokiscale.loomspan.internal.security.AccessGuard;
import com.lokiscale.loomspan.internal.vfs.RefResolver;
import org.springframework.beans.factory.ObjectProvider;
import org.springframework.lang.Nullable;
import org.springframework.security.core.Authentication;

import java.util.Map;
import java.util.Objects;

public class CapabilityExecutionRouter
{
    private final RefResolver refResolver;
    private final ObjectProvider<ExecutionCoordinator> executionCoordinatorProvider;
    private final ExecutionStateService executionStateService;
    private final AccessGuard accessGuard;
    private final SkillInputValidator inputValidator;

    public CapabilityExecutionRouter(RefResolver refResolver,
            ObjectProvider<ExecutionCoordinator> executionCoordinatorProvider,
            ExecutionStateService executionStateService,
            AccessGuard accessGuard)
    {
        this(refResolver, executionCoordinatorProvider, executionStateService, accessGuard, new SkillInputValidator());
    }

    public CapabilityExecutionRouter(RefResolver refResolver,
            ObjectProvider<ExecutionCoordinator> executionCoordinatorProvider,
            ExecutionStateService executionStateService,
            AccessGuard accessGuard,
            SkillInputValidator inputValidator)
    {
        this.refResolver = Objects.requireNonNull(refResolver, "refResolver must not be null");
        this.executionCoordinatorProvider = Objects.requireNonNull(
                executionCoordinatorProvider,
                "executionCoordinatorProvider must not be null");

        this.executionStateService = Objects.requireNonNull(executionStateService, "executionStateService must not be null");
        this.accessGuard = Objects.requireNonNull(accessGuard, "accessGuard must not be null");
        this.inputValidator = Objects.requireNonNull(inputValidator, "inputValidator must not be null");
    }

    public Object execute(CapabilityMetadata capability,
            Map<String, Object> arguments,
            LoomspanSession session,
            @Nullable Authentication authentication)
    {
        Objects.requireNonNull(capability, "capability must not be null");
        Objects.requireNonNull(session, "session must not be null");

        accessGuard.checkAccess(capability, session, authentication);
        Map<String, Object> safeArguments = arguments == null ? Map.of() : arguments;
        SkillInputValidationResult validation = inputValidator.validate(safeArguments, capability.inputContract());

        if (!validation.valid())
        {
            throw new SkillInputValidationException(
                    "Invalid input for capability '" + capability.name() + "'",
                    validation.issues().stream()
                            .map(issue -> new SkillInputValidationIssue(issue.path(), issue.code(), issue.message()))
                            .toList());
        }

        Map<String, Object> normalizedInput = validation.normalizedInput();

        if (capability.kind() == CapabilityKind.YAML_SKILL && capability.mappedTargetId() == null)
        {
            PlanSnapshot parentPlan = executionStateService.snapshotPlan(session);
            SuccessfulSkillSnapshot parentSkills = executionStateService.snapshotSuccessfulSkills(session);
            try
            {
                return executionCoordinatorProvider.getObject()
                        .execute(capability.name(), objectiveFor(capability, normalizedInput), normalizedInput, session, authentication);
            }
            finally
            {
                executionStateService.restorePlan(session, parentPlan);
                executionStateService.restoreSuccessfulSkills(session, parentSkills);
            }
        }

        return capability.invoker().invoke(
                refResolver.resolveArguments(normalizedInput, session, capability.inputContract()));
    }

    private String objectiveFor(CapabilityMetadata capability, Map<String, Object> arguments)
    {
        return "Execute YAML skill '%s' using the provided mission input object.".formatted(capability.name());
    }
}
