package com.lokiscale.loomspan.autoconfigure;

import com.lokiscale.loomspan.internal.autoconfigure.NamedAiConnectionRegistry;
import com.lokiscale.loomspan.internal.chat.DefaultSkillAdvisorResolver;
import com.lokiscale.loomspan.internal.chat.DefaultSkillChatModelResolver;
import com.lokiscale.loomspan.internal.chat.SkillAdvisorResolver;
import com.lokiscale.loomspan.internal.chat.SkillChatModelResolver;
import com.lokiscale.loomspan.internal.model.ModelInteractionFactory;
import com.lokiscale.loomspan.internal.runtime.state.ExecutionStateService;
import com.lokiscale.loomspan.internal.runtime.usage.ModelUsageExtractor;
import com.lokiscale.loomspan.internal.runtime.usage.SessionUsageService;
import com.lokiscale.loomspan.internal.serialization.LoomspanJacksonCodecs;
import com.lokiscale.loomspan.internal.springai.SpringAiChatOptionsContributor;
import com.lokiscale.loomspan.internal.springai.SpringAiModelInteractionFactory;
import com.lokiscale.loomspan.internal.springai.SpringAiProviderIntegration;
import io.micrometer.observation.ObservationRegistry;
import org.springframework.beans.factory.ObjectProvider;
import org.springframework.beans.factory.config.BeanDefinition;
import org.springframework.boot.autoconfigure.AutoConfiguration;
import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Role;
import org.springframework.core.io.ResourceLoader;

@AutoConfiguration(after = {LoomspanJacksonAutoConfiguration.class, LoomspanAutoConfiguration.class})
@Role(BeanDefinition.ROLE_INFRASTRUCTURE)
public class LoomspanAiAutoConfiguration
{
    @Bean
    @Role(BeanDefinition.ROLE_INFRASTRUCTURE)
    NamedAiConnectionRegistry namedAiConnectionRegistry(LoomspanProperties properties,
            ResourceLoader resourceLoader,
            ObjectProvider<ObservationRegistry> observationRegistryProvider,
            LoomspanJacksonCodecs codecs)
    {
        return new NamedAiConnectionRegistry(properties.getConnections(),
                new SpringAiProviderIntegration(resourceLoader,
                        observationRegistryProvider.getIfAvailable(() -> ObservationRegistry.NOOP),
                        codecs.schemaTree()));
    }

    @Bean
    @Role(BeanDefinition.ROLE_INFRASTRUCTURE)
    SkillChatModelResolver skillChatModelResolver(NamedAiConnectionRegistry registry)
    {
        return new DefaultSkillChatModelResolver(registry.asMap());
    }

    @Bean
    @Role(BeanDefinition.ROLE_INFRASTRUCTURE)
    SpringAiChatOptionsContributor springAiChatOptionsContributor()
    {
        return new SpringAiChatOptionsContributor();
    }

    @Bean
    @Role(BeanDefinition.ROLE_INFRASTRUCTURE)
    SkillAdvisorResolver skillAdvisorResolver(ExecutionStateService executionStateService,
            LoomspanJacksonCodecs codecs)
    {
        return new DefaultSkillAdvisorResolver(executionStateService, codecs.schemaTree());
    }

    @Bean
    @Role(BeanDefinition.ROLE_INFRASTRUCTURE)
    ModelInteractionFactory modelInteractionFactory(
            SkillChatModelResolver chatModelResolver,
            SkillAdvisorResolver skillAdvisorResolver,
            ExecutionStateService executionStateService,
            ModelUsageExtractor modelUsageExtractor,
            SessionUsageService sessionUsageService,
            SpringAiChatOptionsContributor optionsContributor,
            ObjectProvider<ObservationRegistry> observationRegistryProvider)
    {
        return new SpringAiModelInteractionFactory(
                chatModelResolver, skillAdvisorResolver, executionStateService,
                modelUsageExtractor, sessionUsageService, optionsContributor,
                observationRegistryProvider.getIfAvailable(() -> ObservationRegistry.NOOP));
    }
}
