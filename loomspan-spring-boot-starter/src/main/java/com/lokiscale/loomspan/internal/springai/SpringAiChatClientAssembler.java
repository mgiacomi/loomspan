package com.lokiscale.loomspan.internal.springai;

import com.lokiscale.loomspan.internal.chat.SkillAdvisorResolver;
import com.lokiscale.loomspan.internal.chat.ProviderAttemptCallAdvisor;
import com.lokiscale.loomspan.internal.chat.SkillChatModelResolver;
import com.lokiscale.loomspan.internal.linter.LinterCallAdvisor;
import com.lokiscale.loomspan.internal.outputschema.OutputSchemaCallAdvisor;
import com.lokiscale.loomspan.internal.runtime.evidence.EvidenceContractCallAdvisor;
import com.lokiscale.loomspan.internal.runtime.state.ExecutionStateService;
import com.lokiscale.loomspan.internal.runtime.usage.ModelUsageExtractor;
import com.lokiscale.loomspan.internal.runtime.usage.SessionUsageService;
import com.lokiscale.loomspan.internal.provider.ProviderConnectionRuntime;
import com.lokiscale.loomspan.internal.skill.EffectiveSkillExecutionConfiguration;
import com.lokiscale.loomspan.internal.skill.YamlSkillDefinition;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.ai.chat.client.ChatClient;
import org.springframework.ai.chat.client.advisor.api.Advisor;
import org.springframework.ai.chat.model.ChatModel;
import io.micrometer.observation.ObservationRegistry;

import java.util.ArrayList;
import java.util.List;
import java.util.Objects;

final class SpringAiChatClientAssembler
{
    private static final Logger log = LoggerFactory.getLogger(SpringAiChatClientAssembler.class);

    private final SkillChatModelResolver chatModelResolver;
    private final ChatClientBuilderFactory chatClientBuilderFactory;
    private final SpringAiChatOptionsContributor optionsContributor;
    private final SkillAdvisorResolver skillAdvisorResolver;
    private final ExecutionStateService executionStateService;
    private final ModelUsageExtractor modelUsageExtractor;
    private final SessionUsageService sessionUsageService;

    SpringAiChatClientAssembler(SkillChatModelResolver chatModelResolver,
            SpringAiChatOptionsContributor optionsContributor,
            SkillAdvisorResolver skillAdvisorResolver,
            ExecutionStateService executionStateService,
            ModelUsageExtractor modelUsageExtractor,
            SessionUsageService sessionUsageService,
            ObservationRegistry observationRegistry)
    {
        this(chatModelResolver, optionsContributor, skillAdvisorResolver, executionStateService,
                modelUsageExtractor, sessionUsageService,
                model -> ChatClient.builder(model, observationRegistry, null, null));
    }

    SpringAiChatClientAssembler(SkillChatModelResolver chatModelResolver,
            SpringAiChatOptionsContributor optionsContributor,
            SkillAdvisorResolver skillAdvisorResolver,
            ExecutionStateService executionStateService,
            ModelUsageExtractor modelUsageExtractor,
            SessionUsageService sessionUsageService,
            ChatClientBuilderFactory chatClientBuilderFactory)
    {
        this.chatModelResolver = Objects.requireNonNull(chatModelResolver, "chatModelResolver must not be null");
        this.optionsContributor = Objects.requireNonNull(optionsContributor, "optionsContributor must not be null");
        this.skillAdvisorResolver = Objects.requireNonNull(skillAdvisorResolver, "skillAdvisorResolver must not be null");
        this.executionStateService = Objects.requireNonNull(executionStateService, "executionStateService must not be null");
        this.modelUsageExtractor = Objects.requireNonNull(modelUsageExtractor, "modelUsageExtractor must not be null");
        this.sessionUsageService = Objects.requireNonNull(sessionUsageService, "sessionUsageService must not be null");
        this.chatClientBuilderFactory = Objects.requireNonNull(chatClientBuilderFactory, "chatClientBuilderFactory must not be null");
    }

    ChatClient create(YamlSkillDefinition definition)
    {
        return create(definition, true);
    }

    ChatClient createForStepExecution(YamlSkillDefinition definition)
    {
        return create(definition, false);
    }

    private ChatClient create(YamlSkillDefinition definition, boolean includeFinalResponseValidators)
    {
        Objects.requireNonNull(definition, "definition must not be null");
        EffectiveSkillExecutionConfiguration executionConfiguration = definition.requireExecutionConfiguration();
        String skillName = definition.manifest().getName();
        ProviderConnectionRuntime runtime = chatModelResolver.resolve(skillName, executionConfiguration);
        ChatModel chatModel = runtime.chatModel();
        List<Advisor> advisors = resolvedAdvisors(skillAdvisorResolver.resolve(definition), includeFinalResponseValidators,
                new ProviderAttemptCallAdvisor(runtime, executionStateService, modelUsageExtractor, sessionUsageService));
        ChatClient.Builder builder = chatClientBuilderFactory.create(chatModel);
        builder.defaultOptions(optionsContributor.createOptions(executionConfiguration));
        if (!advisors.isEmpty())
        {
            builder.defaultAdvisors(advisors);
        }
        ChatClient delegate = builder.build();
        log.debug(
                "Created skill ChatClient for skill '{}' frameworkModel={} connection={} driver={} chatModelType={} delegateType={} advisors={}",
                skillName,
                executionConfiguration.frameworkModel(),
                executionConfiguration.connection(),
                executionConfiguration.driver(),
                chatModel.getClass().getName(),
                delegate.getClass().getName(),
                includeFinalResponseValidators ? advisorNames(advisors) : advisorNames(advisors) + " (step execution)");
        return delegate;
    }

    private List<Advisor> resolvedAdvisors(List<Advisor> advisors, boolean includeFinalResponseValidators,
            ProviderAttemptCallAdvisor providerAttemptCallAdvisor)
    {
        List<Advisor> resolved = advisors == null ? List.of() : advisors;
        if (!includeFinalResponseValidators)
        {
            resolved = resolved.stream()
                    .filter(advisor -> !(advisor instanceof OutputSchemaCallAdvisor))
                    .filter(advisor -> !(advisor instanceof EvidenceContractCallAdvisor))
                    .filter(advisor -> !(advisor instanceof LinterCallAdvisor))
                    .toList();
        }
        ArrayList<Advisor> instrumented = new ArrayList<>(resolved);
        instrumented.add(providerAttemptCallAdvisor);
        return List.copyOf(instrumented);
    }

    interface ChatClientBuilderFactory
    {

        ChatClient.Builder create(ChatModel chatModel);
    }

    private static String advisorNames(List<Advisor> advisors)
    {
        return advisors.stream()
                .map(advisor -> advisor.getName() + ":" + advisor.getClass().getSimpleName())
                .toList()
                .toString();
    }

}
