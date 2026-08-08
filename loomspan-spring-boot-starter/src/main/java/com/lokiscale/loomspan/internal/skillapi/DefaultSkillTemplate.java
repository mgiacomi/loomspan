package com.lokiscale.loomspan.internal.skillapi;

import com.fasterxml.jackson.core.type.TypeReference;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.lokiscale.loomspan.internal.core.LoomspanSession;
import com.lokiscale.loomspan.internal.core.LoomspanSessionRunner;
import com.lokiscale.loomspan.internal.core.CapabilityExecutionRouter;
import com.lokiscale.loomspan.internal.core.CapabilityMetadata;
import com.lokiscale.loomspan.internal.core.CapabilityRegistry;
import com.lokiscale.loomspan.internal.runtime.input.SkillInputContract;
import com.lokiscale.loomspan.internal.runtime.input.SkillInputValidationResult;
import com.lokiscale.loomspan.internal.runtime.input.SkillInputValidator;
import com.lokiscale.loomspan.api.SkillException;
import com.lokiscale.loomspan.api.SkillExecutionView;
import com.lokiscale.loomspan.api.SkillInputValidationException;
import com.lokiscale.loomspan.api.SkillInputValidationIssue;
import com.lokiscale.loomspan.api.SkillTemplate;
import org.springframework.security.access.AccessDeniedException;
import org.springframework.security.core.Authentication;
import org.springframework.security.core.context.SecurityContextHolder;

import java.util.List;
import java.util.Map;
import java.util.Objects;
import java.util.function.Consumer;

public class DefaultSkillTemplate implements SkillTemplate
{
    private static final TypeReference<Map<String, Object>> MAP_TYPE = new TypeReference<>()
    {
    };

    private final CapabilityRegistry capabilityRegistry;
    private final CapabilityExecutionRouter executionRouter;
    private final LoomspanSessionRunner sessionRunner;
    private final ObjectMapper objectMapper;
    private final SkillInputValidator inputValidator;
    private final SkillExecutionViewMapper executionViewMapper;

    public DefaultSkillTemplate(CapabilityRegistry capabilityRegistry,
            CapabilityExecutionRouter executionRouter,
            LoomspanSessionRunner sessionRunner,
            ObjectMapper objectMapper,
            SkillInputValidator inputValidator)
    {
        this.capabilityRegistry = Objects.requireNonNull(capabilityRegistry, "capabilityRegistry must not be null");
        this.executionRouter = Objects.requireNonNull(executionRouter, "executionRouter must not be null");
        this.sessionRunner = Objects.requireNonNull(sessionRunner, "sessionRunner must not be null");
        this.objectMapper = Objects.requireNonNull(objectMapper, "objectMapper must not be null");
        this.inputValidator = Objects.requireNonNull(inputValidator, "inputValidator must not be null");
        this.executionViewMapper = new SkillExecutionViewMapper(objectMapper);
    }

    @Override
    public String invoke(String skillName, Object input)
    {
        return invoke(skillName, input, null);
    }

    @Override
    public String invoke(String skillName, Map<String, Object> input)
    {
        return invoke(skillName, input, null);
    }

    @Override
    public String invoke(String skillName, Object input, Consumer<SkillExecutionView> observer)
    {
        if (input == null)
        {
            throw new SkillInputValidationException("Skill input must not be null.", List.of());
        }
        Map<String, Object> convertedInput;
        try
        {
            convertedInput = objectMapper.convertValue(input, MAP_TYPE);
        }
        catch (RuntimeException ex)
        {
            throw new SkillException("Skill '" + skillName + "' execution failed.", ex);
        }
        return invoke(skillName, convertedInput, observer);
    }

    @Override
    public String invoke(String skillName, Map<String, Object> input, Consumer<SkillExecutionView> observer)
    {
        ExecutionResult execution;
        try
        {
            CapabilityMetadata capability = requireYamlSkill(skillName);
            SkillInputContract contract = capability.inputContract();
            Map<String, Object> safeInput = normalizeNullInput(input, contract);
            SkillInputValidationResult validation = inputValidator.validate(safeInput, contract);

            if (!validation.valid())
            {
                List<SkillInputValidationIssue> issues = validation.issues().stream()
                        .map(issue -> new SkillInputValidationIssue(issue.path(), issue.code(), issue.message()))
                        .toList();
                throw new SkillInputValidationException(buildValidationMessage(skillName, validation), issues);
            }

            Authentication authentication = SecurityContextHolder.getContext().getAuthentication();
            execution = sessionRunner.callWithNewSession(
                    capability.name(),
                    authentication,
                    session -> executeValidated(capability, validation, session));
        }
        catch (AccessDeniedException | SkillException ex)
        {
            throw ex;
        }
        catch (RuntimeException ex)
        {
            throw new SkillException("Skill '" + skillName + "' execution failed.", ex);
        }

        if (observer != null)
        {
            observer.accept(executionViewMapper.map(execution.session()));
        }

        return execution.result();
    }

    private ExecutionResult executeValidated(CapabilityMetadata capability,
            SkillInputValidationResult validation,
            LoomspanSession session)
    {
        Object result = executionRouter.execute(capability, validation.normalizedInput(), session, null);
        return new ExecutionResult(String.valueOf(result), session);
    }

    private CapabilityMetadata requireYamlSkill(String skillName)
    {
        CapabilityMetadata capability = capabilityRegistry.getCapability(skillName);
        if (capability == null)
        {
            throw new SkillException("Unknown YAML skill '" + skillName + "'");
        }
        if (capability.kind() != com.lokiscale.loomspan.internal.core.CapabilityKind.YAML_SKILL)
        {
            throw new SkillException("SkillTemplate only supports YAML skills. '" + skillName + "' is a " + capability.kind());
        }
        if (!skillName.equals(capability.name()))
        {
            throw new SkillException("CapabilityRegistry returned YAML skill '" + capability.name()
                    + "' for requested name '" + skillName + "'");
        }

        return capability;
    }

    private Map<String, Object> normalizeNullInput(Map<String, Object> input, SkillInputContract contract)
    {
        if (input != null)
        {
            return input;
        }
        if (contract.isGeneric() || contract.allowsEmptyInput())
        {
            return Map.of();
        }

        throw new SkillInputValidationException("Skill input must not be null for contract-backed skills.", List.of());
    }

    private String buildValidationMessage(String skillName, SkillInputValidationResult validation)
    {
        String detail = validation.issues().stream()
                .map(issue -> (issue.path() == null || issue.path().isBlank() ? "<root>" : issue.path()) + ": " + issue.message())
                .reduce((left, right) -> left + "; " + right)
                .orElse("Invalid skill input.");

        return "Invalid input for skill '" + skillName + "': " + detail;
    }

    private record ExecutionResult(String result, LoomspanSession session)
    {
    }
}
