package com.lokiscale.loomspan.internal.springai;

import com.lokiscale.loomspan.internal.chat.SkillAdvisorResolver;
import com.lokiscale.loomspan.internal.chat.SkillChatModelResolver;
import com.lokiscale.loomspan.internal.model.ModelInteraction;
import com.lokiscale.loomspan.internal.model.ModelInteractionFactory;
import com.lokiscale.loomspan.internal.model.ModelInteractionMode;
import com.lokiscale.loomspan.internal.skill.YamlSkillDefinition;
import com.lokiscale.loomspan.internal.runtime.state.ExecutionStateService;
import com.lokiscale.loomspan.internal.runtime.usage.ModelUsageExtractor;
import com.lokiscale.loomspan.internal.runtime.usage.SessionUsageService;
import io.micrometer.observation.ObservationRegistry;

import java.util.Objects;

public final class SpringAiModelInteractionFactory implements ModelInteractionFactory
{
    private final SpringAiChatClientAssembler assembler;

    public SpringAiModelInteractionFactory(SkillChatModelResolver chatModelResolver,
            SkillAdvisorResolver skillAdvisorResolver,
            ExecutionStateService executionStateService,
            ModelUsageExtractor modelUsageExtractor,
            SessionUsageService sessionUsageService,
            SpringAiChatOptionsContributor optionsContributor,
            ObservationRegistry observationRegistry)
    {
        this.assembler = new SpringAiChatClientAssembler(chatModelResolver, optionsContributor,
                skillAdvisorResolver, executionStateService, modelUsageExtractor, sessionUsageService,
                observationRegistry);
    }

    @Override
    public ModelInteraction create(YamlSkillDefinition definition, ModelInteractionMode mode)
    {
        return new SpringAiModelInteraction(mode == ModelInteractionMode.STEP_EXECUTION
                ? assembler.createForStepExecution(definition)
                : assembler.create(definition));
    }
}
