package com.lokiscale.loomspan.autoconfigure;

import com.lokiscale.loomspan.internal.autoconfigure.NamedAiConnectionRegistry;
import com.lokiscale.loomspan.internal.springai.v1_1.SpringAiV11ProviderIntegration;
import com.lokiscale.loomspan.internal.chat.DefaultSkillAdvisorResolver;
import com.lokiscale.loomspan.internal.chat.DefaultSkillChatModelResolver;
import com.lokiscale.loomspan.internal.chat.SkillAdvisorResolver;
import com.lokiscale.loomspan.internal.chat.SkillChatClientFactory;
import com.lokiscale.loomspan.internal.chat.SkillChatModelResolver;
import com.lokiscale.loomspan.internal.chat.SkillChatOptionsAdapter;
import com.lokiscale.loomspan.internal.chat.SpringAiSkillChatClientFactory;
import com.lokiscale.loomspan.internal.core.LoomspanExceptionTransformer;
import com.lokiscale.loomspan.internal.core.CapabilityRegistry;
import com.lokiscale.loomspan.internal.core.LoomspanSessionRunner;
import com.lokiscale.loomspan.internal.core.CapabilityExecutionRouter;
import com.lokiscale.loomspan.internal.core.DefaultLoomspanExceptionTransformer;
import com.lokiscale.loomspan.internal.core.DefaultPlanTaskLinker;
import com.lokiscale.loomspan.internal.core.ExecutionCoordinator;
import com.lokiscale.loomspan.internal.core.InMemoryCapabilityRegistry;
import com.lokiscale.loomspan.internal.core.InMemorySkillImplementationTargetRegistry;
import com.lokiscale.loomspan.internal.core.PlanTaskLinker;
import com.lokiscale.loomspan.internal.core.SkillMethodBeanPostProcessor;
import com.lokiscale.loomspan.internal.core.SkillImplementationTargetRegistry;
import com.lokiscale.loomspan.internal.runtime.DefaultMissionExecutionEngine;
import com.lokiscale.loomspan.internal.runtime.MissionExecutionEngine;
import com.lokiscale.loomspan.internal.runtime.attachment.DefaultMissionInputMaterializer;
import com.lokiscale.loomspan.internal.runtime.attachment.MissionInputMaterializer;
import com.lokiscale.loomspan.internal.runtime.attachment.MissionUserMessageSender;
import com.lokiscale.loomspan.internal.runtime.attachment.SpringAiMissionUserMessageSender;
import com.lokiscale.loomspan.internal.runtime.planning.DefaultPlanningService;
import com.lokiscale.loomspan.internal.runtime.planning.PlanningService;
import com.lokiscale.loomspan.internal.observability.ObservabilityActivationCoordinator;
import com.lokiscale.loomspan.internal.runtime.input.SkillInputContractResolver;
import com.lokiscale.loomspan.internal.runtime.input.SkillInputValidator;
import com.lokiscale.loomspan.internal.runtime.state.DefaultExecutionStateService;
import com.lokiscale.loomspan.internal.runtime.state.ExecutionStateService;
import com.lokiscale.loomspan.internal.runtime.tool.DefaultToolCallbackFactory;
import com.lokiscale.loomspan.internal.runtime.tool.DefaultToolSurfaceService;
import com.lokiscale.loomspan.internal.runtime.tool.ToolCallbackFactory;
import com.lokiscale.loomspan.internal.runtime.tool.ToolSurfaceService;
import com.lokiscale.loomspan.internal.runtime.usage.DefaultSessionUsageService;
import com.lokiscale.loomspan.internal.runtime.usage.MicrometerUsageMetricsRecorder;
import com.lokiscale.loomspan.internal.runtime.usage.ModelUsageExtractor;
import com.lokiscale.loomspan.internal.runtime.usage.NoOpUsageMetricsRecorder;
import com.lokiscale.loomspan.internal.runtime.usage.SessionUsageService;
import com.lokiscale.loomspan.internal.runtime.usage.UsageMetricsRecorder;
import com.lokiscale.loomspan.internal.security.AccessGuard;
import com.lokiscale.loomspan.internal.security.DefaultAccessGuard;
import com.lokiscale.loomspan.internal.skill.DefaultSkillVisibilityResolver;
import com.lokiscale.loomspan.internal.skill.SkillVisibilityResolver;
import com.lokiscale.loomspan.internal.skill.YamlSkillCapabilityRegistrar;
import com.lokiscale.loomspan.internal.skill.YamlSkillCatalog;
import com.lokiscale.loomspan.internal.skillapi.DefaultSkillTemplate;
import com.lokiscale.loomspan.api.SkillTemplate;
import com.lokiscale.loomspan.internal.vfs.DefaultRefResolver;
import com.lokiscale.loomspan.internal.vfs.RefResolver;
import com.lokiscale.loomspan.internal.vfs.SessionLocalVirtualFileSystem;
import com.lokiscale.loomspan.internal.vfs.VirtualFileSystem;
import io.micrometer.core.instrument.MeterRegistry;
import org.springframework.beans.factory.ObjectProvider;
import org.springframework.beans.factory.config.BeanDefinition;
import org.springframework.beans.factory.annotation.Qualifier;
import org.springframework.boot.context.properties.EnableConfigurationProperties;
import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Role;
import org.springframework.core.io.ResourceLoader;
import org.springframework.boot.autoconfigure.AutoConfiguration;
import org.springframework.boot.autoconfigure.condition.ConditionalOnNotWebApplication;
import org.springframework.boot.autoconfigure.condition.ConditionalOnWebApplication;
import org.springframework.beans.factory.SmartInitializingSingleton;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import com.fasterxml.jackson.databind.ObjectMapper;
import java.nio.file.Paths;
import java.time.Clock;
import java.util.List;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;

@AutoConfiguration
@EnableConfigurationProperties({
        ExecutionTraceProperties.class,
        LoomspanProperties.class
})
@Role(BeanDefinition.ROLE_INFRASTRUCTURE)
public class LoomspanAutoConfiguration
{
    private static final Logger LOGGER = LoggerFactory.getLogger(LoomspanAutoConfiguration.class);
    @Bean
    @Role(BeanDefinition.ROLE_INFRASTRUCTURE)
    CapabilityRegistry capabilityRegistry()
    {
        return new InMemoryCapabilityRegistry();
    }

    @Bean
    @Role(BeanDefinition.ROLE_INFRASTRUCTURE)
    SkillImplementationTargetRegistry skillImplementationTargetRegistry()
    {
        return new InMemorySkillImplementationTargetRegistry();
    }

    @Bean
    @Role(BeanDefinition.ROLE_INFRASTRUCTURE)
    LoomspanExceptionTransformer LoomspanExceptionTransformer()
    {
        return new DefaultLoomspanExceptionTransformer();
    }

    @Bean
    @Role(BeanDefinition.ROLE_INFRASTRUCTURE)
    static SkillMethodBeanPostProcessor skillMethodBeanPostProcessor(
            SkillImplementationTargetRegistry skillImplementationTargetRegistry,
            ObjectProvider<ObjectMapper> objectMapperProvider,
            LoomspanExceptionTransformer LoomspanExceptionTransformer,
            SkillInputContractResolver skillInputContractResolver)
    {
        return SkillMethodBeanPostProcessor.create(
                skillImplementationTargetRegistry,
                objectMapperProvider.getIfAvailable(ObjectMapper::new),
                LoomspanExceptionTransformer,
                skillInputContractResolver);
    }

    @Bean
    @Role(BeanDefinition.ROLE_INFRASTRUCTURE)
    ObservabilityActivationCoordinator observabilityActivationCoordinator()
    {
        return new ObservabilityActivationCoordinator();
    }

    @Bean
    @Role(BeanDefinition.ROLE_INFRASTRUCTURE)
    @ConditionalOnNotWebApplication
    SmartInitializingSingleton observabilityNonWebActivation(
            LoomspanProperties properties,
            ObservabilityActivationCoordinator observabilityActivationCoordinator)
    {
        return unsupportedObservabilityActivation(properties, observabilityActivationCoordinator);
    }

    @Bean
    @Role(BeanDefinition.ROLE_INFRASTRUCTURE)
    @ConditionalOnWebApplication(type = ConditionalOnWebApplication.Type.REACTIVE)
    SmartInitializingSingleton observabilityReactiveWebActivation(
            LoomspanProperties properties,
            ObservabilityActivationCoordinator observabilityActivationCoordinator)
    {
        return unsupportedObservabilityActivation(properties, observabilityActivationCoordinator);
    }

    private static SmartInitializingSingleton unsupportedObservabilityActivation(
            LoomspanProperties properties,
            ObservabilityActivationCoordinator observabilityActivationCoordinator)
    {
        return () ->
        {
            if (properties.getObservability().isEnabled())
            {
                LOGGER.warn("loomspan observability disabled: a servlet web application is required");
            }
            observabilityActivationCoordinator.disable();
        };
    }

    @Bean
    @Role(BeanDefinition.ROLE_INFRASTRUCTURE)
    LoomspanSessionRunner LoomspanSessionRunner(LoomspanProperties properties,
            ExecutionTraceProperties executionTraceProperties,
            ObservabilityActivationCoordinator observabilityActivationCoordinator)
    {
        return new LoomspanSessionRunner(
                properties.getSession().getMaxDepth(),
                executionTraceProperties.getPersistence(),
                Clock.systemUTC(),
                observabilityActivationCoordinator.observationFactory(),
                observabilityActivationCoordinator.completionRetention(),
                properties.getSession().getQuotas());
    }

    @Bean
    @Role(BeanDefinition.ROLE_INFRASTRUCTURE)
    YamlSkillCatalog yamlSkillCatalog(LoomspanProperties properties)
    {
        // The catalog is the YAML discovery/loading boundary that downstream runtime beans build on.
        return new YamlSkillCatalog(properties);
    }

    @Bean
    @Role(BeanDefinition.ROLE_INFRASTRUCTURE)
    YamlSkillCapabilityRegistrar yamlSkillCapabilityRegistrar(CapabilityRegistry capabilityRegistry,
            SkillImplementationTargetRegistry skillImplementationTargetRegistry,
            YamlSkillCatalog yamlSkillCatalog,
            SkillInputContractResolver skillInputContractResolver)
    {
        return new YamlSkillCapabilityRegistrar(
                capabilityRegistry,
                skillImplementationTargetRegistry,
                yamlSkillCatalog,
                skillInputContractResolver);
    }

    @Bean
    @Role(BeanDefinition.ROLE_INFRASTRUCTURE)
    SkillInputContractResolver skillInputContractResolver(ObjectProvider<ObjectMapper> objectMapperProvider)
    {
        return new SkillInputContractResolver(objectMapperProvider.getIfAvailable(ObjectMapper::new));
    }

    @Bean
    @Role(BeanDefinition.ROLE_INFRASTRUCTURE)
    SkillInputValidator skillInputValidator()
    {
        return new SkillInputValidator();
    }

    @Bean
    @Role(BeanDefinition.ROLE_INFRASTRUCTURE)
    AccessGuard accessGuard()
    {
        return new DefaultAccessGuard();
    }

    @Bean
    @Role(BeanDefinition.ROLE_INFRASTRUCTURE)
    SkillVisibilityResolver skillVisibilityResolver(YamlSkillCatalog yamlSkillCatalog,
            CapabilityRegistry capabilityRegistry,
            AccessGuard accessGuard)
    {
        return new DefaultSkillVisibilityResolver(yamlSkillCatalog, capabilityRegistry, accessGuard);
    }

    @Bean
    @Role(BeanDefinition.ROLE_INFRASTRUCTURE)
    VirtualFileSystem virtualFileSystem()
    {
        return new SessionLocalVirtualFileSystem(Paths.get(System.getProperty("java.io.tmpdir"), "loomspan-vfs"));
    }

    @Bean
    @Role(BeanDefinition.ROLE_INFRASTRUCTURE)
    RefResolver refResolver(VirtualFileSystem virtualFileSystem)
    {
        return new DefaultRefResolver(virtualFileSystem);
    }

    @Bean
    @Role(BeanDefinition.ROLE_INFRASTRUCTURE)
    MissionInputMaterializer missionInputMaterializer(RefResolver refResolver,
            SkillInputContractResolver skillInputContractResolver,
            LoomspanProperties properties)
    {
        return new DefaultMissionInputMaterializer(
                refResolver,
                skillInputContractResolver,
                properties.getSession().getAttachments().getMaxSize());
    }

    @Bean
    @Role(BeanDefinition.ROLE_INFRASTRUCTURE)
    MissionUserMessageSender missionUserMessageSender()
    {
        return new SpringAiMissionUserMessageSender();
    }

    @Bean
    @Role(BeanDefinition.ROLE_INFRASTRUCTURE)
    CapabilityExecutionRouter capabilityExecutionRouter(RefResolver refResolver,
            org.springframework.beans.factory.ObjectProvider<ExecutionCoordinator> executionCoordinatorProvider,
            ExecutionStateService executionStateService,
            AccessGuard accessGuard,
            SkillInputValidator skillInputValidator)
    {
        return new CapabilityExecutionRouter(
                refResolver,
                executionCoordinatorProvider,
                executionStateService,
                accessGuard,
                skillInputValidator);
    }

    @Bean
    @Role(BeanDefinition.ROLE_INFRASTRUCTURE)
    SkillTemplate skillTemplate(CapabilityRegistry capabilityRegistry,
            CapabilityExecutionRouter capabilityExecutionRouter,
            LoomspanSessionRunner LoomspanSessionRunner,
            ObjectProvider<ObjectMapper> objectMapperProvider,
            SkillInputValidator skillInputValidator)
    {
        return new DefaultSkillTemplate(
                capabilityRegistry,
                capabilityExecutionRouter,
                LoomspanSessionRunner,
                objectMapperProvider.getIfAvailable(ObjectMapper::new),
                skillInputValidator);
    }

    @Bean
    @Role(BeanDefinition.ROLE_INFRASTRUCTURE)
    PlanTaskLinker planTaskLinker()
    {
        return new DefaultPlanTaskLinker();
    }

    @Bean
    @Role(BeanDefinition.ROLE_INFRASTRUCTURE)
    ModelUsageExtractor modelUsageExtractor()
    {
        return new ModelUsageExtractor();
    }

    @Bean
    @Role(BeanDefinition.ROLE_INFRASTRUCTURE)
    UsageMetricsRecorder usageMetricsRecorder(ObjectProvider<MeterRegistry> meterRegistryProvider)
    {
        MeterRegistry meterRegistry = meterRegistryProvider.getIfAvailable();
        return meterRegistry == null
                ? new NoOpUsageMetricsRecorder()
                : new MicrometerUsageMetricsRecorder(meterRegistry);
    }

    @Bean
    @Role(BeanDefinition.ROLE_INFRASTRUCTURE)
    SessionUsageService sessionUsageService(LoomspanProperties properties,
            UsageMetricsRecorder usageMetricsRecorder)
    {
        return new DefaultSessionUsageService(properties.getSession().getQuotas(), usageMetricsRecorder);
    }

    @Bean
    @Role(BeanDefinition.ROLE_INFRASTRUCTURE)
    ExecutionStateService executionStateService(SessionUsageService sessionUsageService)
    {
        return new DefaultExecutionStateService(Clock.systemUTC(), sessionUsageService);
    }

    @Bean
    @Role(BeanDefinition.ROLE_INFRASTRUCTURE)
    PlanningService planningService(PlanTaskLinker planTaskLinker,
            ExecutionStateService executionStateService)
    {
        return new DefaultPlanningService(planTaskLinker, executionStateService);
    }

    @Bean
    @Role(BeanDefinition.ROLE_INFRASTRUCTURE)
    ToolSurfaceService toolSurfaceService(SkillVisibilityResolver skillVisibilityResolver)
    {
        return new DefaultToolSurfaceService(skillVisibilityResolver);
    }

    @Bean
    @Role(BeanDefinition.ROLE_INFRASTRUCTURE)
    ToolCallbackFactory toolCallbackFactory(CapabilityExecutionRouter capabilityExecutionRouter,
            PlanningService planningService,
            ExecutionStateService executionStateService,
            SessionUsageService sessionUsageService,
            UsageMetricsRecorder usageMetricsRecorder)
    {
        return new DefaultToolCallbackFactory(
                capabilityExecutionRouter,
                planningService,
                executionStateService,
                sessionUsageService,
                usageMetricsRecorder);
    }

    @Bean(name = "LoomspanMissionExecutor", destroyMethod = "close")
    @Role(BeanDefinition.ROLE_INFRASTRUCTURE)
    ExecutorService LoomspanMissionExecutor()
    {
        return Executors.newVirtualThreadPerTaskExecutor();
    }

    @Bean
    @Role(BeanDefinition.ROLE_INFRASTRUCTURE)
    MissionExecutionEngine missionExecutionEngine(PlanningService planningService,
            ExecutionStateService executionStateService,
            LoomspanProperties properties,
            SessionUsageService sessionUsageService,
            MissionInputMaterializer missionInputMaterializer,
            MissionUserMessageSender missionUserMessageSender,
            @Qualifier("LoomspanMissionExecutor") ExecutorService LoomspanMissionExecutor)
    {
        return new DefaultMissionExecutionEngine(
                planningService,
                executionStateService,
                properties.getSession().getMissionTimeout(),
                LoomspanMissionExecutor,
                sessionUsageService,
                missionInputMaterializer,
                missionUserMessageSender);
    }

    @Bean
    @Role(BeanDefinition.ROLE_INFRASTRUCTURE)
    NamedAiConnectionRegistry namedAiConnectionRegistry(LoomspanProperties properties,
            ResourceLoader resourceLoader)
    {
        return new NamedAiConnectionRegistry(properties.getConnections(),
                new SpringAiV11ProviderIntegration(resourceLoader));
    }

    @Bean
    @Role(BeanDefinition.ROLE_INFRASTRUCTURE)
    SkillChatModelResolver skillChatModelResolver(NamedAiConnectionRegistry registry)
    {
        return new DefaultSkillChatModelResolver(registry.asMap());
    }

    @Bean
    @Role(BeanDefinition.ROLE_INFRASTRUCTURE)
    SkillChatOptionsAdapter openAiSkillChatOptionsAdapter()
    {
        return SpringAiSkillChatClientFactory.defaultAdapters().get(0);
    }

    @Bean
    @Role(BeanDefinition.ROLE_INFRASTRUCTURE)
    SkillChatOptionsAdapter anthropicSkillChatOptionsAdapter()
    {
        return SpringAiSkillChatClientFactory.defaultAdapters().get(1);
    }

    @Bean
    @Role(BeanDefinition.ROLE_INFRASTRUCTURE)
    SkillChatOptionsAdapter geminiSkillChatOptionsAdapter()
    {
        return SpringAiSkillChatClientFactory.defaultAdapters().get(2);
    }

    @Bean
    @Role(BeanDefinition.ROLE_INFRASTRUCTURE)
    SkillChatOptionsAdapter ollamaSkillChatOptionsAdapter()
    {
        return SpringAiSkillChatClientFactory.defaultAdapters().get(3);
    }

    @Bean
    @Role(BeanDefinition.ROLE_INFRASTRUCTURE)
    SkillAdvisorResolver skillAdvisorResolver(ExecutionStateService executionStateService)
    {
        return new DefaultSkillAdvisorResolver(executionStateService);
    }

    @Bean
    @Role(BeanDefinition.ROLE_INFRASTRUCTURE)
    SkillChatClientFactory skillChatClientFactory(SkillChatModelResolver chatModelResolver,
            List<SkillChatOptionsAdapter> adapters,
            SkillAdvisorResolver skillAdvisorResolver,
            ExecutionStateService executionStateService,
            ModelUsageExtractor modelUsageExtractor,
            SessionUsageService sessionUsageService)
    {
        return new SpringAiSkillChatClientFactory(
                chatModelResolver,
                adapters,
                skillAdvisorResolver,
                executionStateService,
                modelUsageExtractor,
                sessionUsageService);
    }

    @Bean
    @Role(BeanDefinition.ROLE_INFRASTRUCTURE)
    com.lokiscale.loomspan.internal.runtime.step.StepLoopMissionExecutionEngine stepLoopMissionExecutionEngine(
            PlanningService planningService,
            ExecutionStateService executionStateService,
            CapabilityRegistry capabilityRegistry,
            YamlSkillCatalog yamlSkillCatalog,
            LoomspanProperties properties,
            SessionUsageService sessionUsageService,
            MissionInputMaterializer missionInputMaterializer,
            MissionUserMessageSender missionUserMessageSender,
            @Qualifier("LoomspanMissionExecutor") ExecutorService LoomspanMissionExecutor)
    {
        return new com.lokiscale.loomspan.internal.runtime.step.StepLoopMissionExecutionEngine(
                planningService,
                executionStateService,
                capabilityRegistry,
                yamlSkillCatalog,
                properties.getSession().getMissionTimeout(),
                LoomspanMissionExecutor,
                sessionUsageService,
                missionInputMaterializer,
                missionUserMessageSender);
    }

    @Bean
    @Role(BeanDefinition.ROLE_INFRASTRUCTURE)
    ExecutionCoordinator executionCoordinator(YamlSkillCatalog yamlSkillCatalog,
            CapabilityRegistry capabilityRegistry,
            SkillChatClientFactory skillChatClientFactory,
            ToolSurfaceService toolSurfaceService,
            ToolCallbackFactory toolCallbackFactory,
            MissionExecutionEngine missionExecutionEngine,
            com.lokiscale.loomspan.internal.runtime.step.StepLoopMissionExecutionEngine stepLoopMissionExecutionEngine,
            ExecutionStateService executionStateService,
            AccessGuard accessGuard)
    {
        return new ExecutionCoordinator(
                yamlSkillCatalog,
                capabilityRegistry,
                skillChatClientFactory,
                toolSurfaceService,
                toolCallbackFactory,
                missionExecutionEngine,
                stepLoopMissionExecutionEngine,
                executionStateService,
                accessGuard);
    }

}
