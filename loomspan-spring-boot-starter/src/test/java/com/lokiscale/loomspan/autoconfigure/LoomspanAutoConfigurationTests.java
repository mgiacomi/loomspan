package com.lokiscale.loomspan.autoconfigure;

import com.lokiscale.loomspan.api.SkillMethod;
import com.lokiscale.loomspan.internal.chat.DefaultSkillAdvisorResolver;
import com.lokiscale.loomspan.internal.chat.SkillAdvisorResolver;
import com.lokiscale.loomspan.internal.model.ModelInteractionFactory;
import com.lokiscale.loomspan.internal.chat.SkillChatModelResolver;
import com.lokiscale.loomspan.internal.springai.SpringAiModelInteractionFactory;
import com.lokiscale.loomspan.internal.core.LoomspanExceptionTransformer;
import com.lokiscale.loomspan.internal.core.LoomspanSessionRunner;
import com.lokiscale.loomspan.internal.core.CapabilityMetadata;
import com.lokiscale.loomspan.internal.core.CapabilityRegistry;
import com.lokiscale.loomspan.internal.core.ExecutionCoordinator;
import com.lokiscale.loomspan.internal.core.InMemorySkillImplementationTargetRegistry;
import com.lokiscale.loomspan.internal.core.SkillMethodBeanPostProcessor;
import com.lokiscale.loomspan.internal.core.SkillImplementationTargetRegistry;
import com.lokiscale.loomspan.internal.runtime.input.SkillInputContractResolver;
import com.lokiscale.loomspan.internal.runtime.input.SkillInputValidator;
import com.lokiscale.loomspan.internal.runtime.planning.DefaultPlanningService;
import com.lokiscale.loomspan.internal.runtime.planning.PlanningService;
import com.lokiscale.loomspan.internal.observability.ObservabilityActivationCoordinator;
import com.lokiscale.loomspan.internal.serialization.LoomspanJacksonCodecs;
import com.lokiscale.loomspan.internal.skill.SkillVisibilityResolver;
import com.lokiscale.loomspan.internal.skill.EffectiveSkillExecutionConfiguration;
import com.lokiscale.loomspan.internal.skill.YamlSkillCatalog;
import com.lokiscale.loomspan.api.SkillTemplate;
import com.lokiscale.loomspan.internal.skillapi.DefaultSkillTemplate;
import com.lokiscale.loomspan.internal.vfs.RefResolver;
import com.lokiscale.loomspan.internal.vfs.VirtualFileSystem;
import org.junit.jupiter.api.Test;
import org.mockito.Mockito;
import org.springframework.ai.ollama.OllamaChatModel;
import org.springframework.ai.openai.OpenAiChatModel;
import org.springframework.ai.chat.model.ChatModel;
import org.springframework.boot.autoconfigure.AutoConfiguration;
import org.springframework.boot.autoconfigure.AutoConfigurations;
import org.springframework.boot.autoconfigure.context.ConfigurationPropertiesAutoConfiguration;
import org.springframework.boot.env.YamlPropertySourceLoader;
import org.springframework.boot.test.context.runner.ApplicationContextRunner;
import org.springframework.boot.test.context.runner.ReactiveWebApplicationContextRunner;
import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Configuration;
import org.springframework.core.env.PropertySource;
import org.springframework.core.io.ClassPathResource;
import org.springframework.test.util.ReflectionTestUtils;

import java.io.IOException;
import java.io.InputStream;
import java.nio.charset.StandardCharsets;
import java.time.Clock;
import java.util.List;
import java.util.Map;

import static org.assertj.core.api.Assertions.assertThat;

class LoomspanAutoConfigurationTests {

    private final ApplicationContextRunner modelFreeContextRunner = new ApplicationContextRunner()
            .withConfiguration(AutoConfigurations.of(
                    ConfigurationPropertiesAutoConfiguration.class,
                    LoomspanJacksonAutoConfiguration.class,
                    LoomspanAutoConfiguration.class,
                    LoomspanAiAutoConfiguration.class));

    private final ApplicationContextRunner contextRunner = new ApplicationContextRunner()
            .withConfiguration(AutoConfigurations.of(
                    ConfigurationPropertiesAutoConfiguration.class,
                    LoomspanJacksonAutoConfiguration.class,
                    LoomspanAutoConfiguration.class,
                    LoomspanAiAutoConfiguration.class))
            .withInitializer(context -> {
                try {
                    YamlPropertySourceLoader loader = new YamlPropertySourceLoader();
                    for (PropertySource<?> propertySource : loader.load("application-test", new ClassPathResource("application-test.yml"))) {
                        context.getEnvironment().getPropertySources().addLast(propertySource);
                    }
                }
                catch (IOException ex) {
                    throw new IllegalStateException("Failed to load application-test.yml", ex);
                }
            });

    private final ReactiveWebApplicationContextRunner reactiveContextRunner =
            new ReactiveWebApplicationContextRunner()
                    .withConfiguration(AutoConfigurations.of(
                            ConfigurationPropertiesAutoConfiguration.class,
                            LoomspanJacksonAutoConfiguration.class,
                            LoomspanAutoConfiguration.class,
                            LoomspanAiAutoConfiguration.class));

    @Test
    void hasAutoConfigurationAnnotation() {
        assertThat(LoomspanAutoConfiguration.class.isAnnotationPresent(AutoConfiguration.class)).isTrue();
        assertThat(LoomspanAiAutoConfiguration.class.isAnnotationPresent(AutoConfiguration.class)).isTrue();
        assertThat(LoomspanObservabilityWebAutoConfiguration.class.isAnnotationPresent(AutoConfiguration.class)).isTrue();
    }

    @Test
    void isRegisteredInAutoConfigurationImports() throws IOException {
        try (InputStream stream = Thread.currentThread()
                .getContextClassLoader()
                .getResourceAsStream("META-INF/spring/org.springframework.boot.autoconfigure.AutoConfiguration.imports")) {
            assertThat(stream).isNotNull();
            String imports = new String(stream.readAllBytes(), StandardCharsets.UTF_8);
            assertThat(imports.lines().filter(line -> !line.isBlank()).toList())
                    .containsExactly(
                            "com.lokiscale.loomspan.autoconfigure.LoomspanJacksonAutoConfiguration",
                            "com.lokiscale.loomspan.autoconfigure.LoomspanAutoConfiguration",
                            "com.lokiscale.loomspan.autoconfigure.LoomspanAiAutoConfiguration",
                            "com.lokiscale.loomspan.autoconfigure.LoomspanObservabilityWebAutoConfiguration");
        }
    }

    @Test
    void autoConfiguresSessionRunnerAndProperties() {
        contextRunner
                .withPropertyValues(
                        "loomspan.session.max-depth=5",
                        "loomspan.skills.locations=classpath:/skills/none/**/*.yaml")
                .run(context -> {
                    assertThat(context).hasSingleBean(LoomspanSessionRunner.class);
                    assertThat(context).hasSingleBean(LoomspanExceptionTransformer.class);
                    assertThat(context).hasSingleBean(ExecutionTraceProperties.class);
                    assertThat(context).hasSingleBean(CapabilityRegistry.class);
                    assertThat(context).hasSingleBean(SkillImplementationTargetRegistry.class);
                    assertThat(context).hasSingleBean(LoomspanProperties.class);
                    assertThat(context).hasSingleBean(YamlSkillCatalog.class);
                    assertThat(context).hasSingleBean(SkillVisibilityResolver.class);
                    assertThat(context).hasSingleBean(VirtualFileSystem.class);
                    assertThat(context).hasSingleBean(RefResolver.class);
                    assertThat(context).hasSingleBean(SkillInputContractResolver.class);
                    assertThat(context).hasSingleBean(SkillInputValidator.class);
                    assertThat(context).hasSingleBean(SkillTemplate.class);
                    assertThat(context.getBean(LoomspanProperties.class).getSession().getMaxDepth()).isEqualTo(5);
                });
    }

    @Test
    void wiresPurposeOwnedCodecRolesIntoProductionConsumers()
    {
        modelFreeContextRunner
                .withPropertyValues("loomspan.skills.locations=classpath:/skills/none/**/*.yaml")
                .run(context ->
                {
                    LoomspanJacksonCodecs codecs = context.getBean(LoomspanJacksonCodecs.class);
                    LoomspanSessionRunner sessionRunner = context.getBean(LoomspanSessionRunner.class);
                    YamlSkillCatalog skillCatalog = context.getBean(YamlSkillCatalog.class);
                    SkillInputContractResolver inputContractResolver =
                            context.getBean(SkillInputContractResolver.class);
                    DefaultSkillTemplate skillTemplate = (DefaultSkillTemplate) context.getBean(SkillTemplate.class);
                    DefaultPlanningService planningService =
                            (DefaultPlanningService) context.getBean(PlanningService.class);
                    SkillMethodBeanPostProcessor beanPostProcessor =
                            context.getBean(SkillMethodBeanPostProcessor.class);

                    assertThat(ReflectionTestUtils.getField(sessionRunner, "canonicalTraceMapper"))
                            .isSameAs(codecs.canonicalTrace());
                    assertThat(ReflectionTestUtils.getField(skillCatalog, "yamlObjectMapper"))
                            .isSameAs(codecs.skillYaml());
                    assertThat(ReflectionTestUtils.getField(inputContractResolver, "objectMapper"))
                            .isSameAs(codecs.applicationConversion());
                    assertThat(ReflectionTestUtils.getField(skillTemplate, "objectMapper"))
                            .isSameAs(codecs.applicationConversion());
                    assertThat(ReflectionTestUtils.getField(planningService, "objectMapper"))
                            .isSameAs(codecs.planningJson());
                    assertThat(ReflectionTestUtils.getField(planningService, "yamlObjectMapper"))
                            .isSameAs(codecs.planningYaml());
                    assertThat(ReflectionTestUtils.getField(beanPostProcessor, "objectMapper"))
                            .isSameAs(codecs.applicationConversion());
                    Object schemaGenerator = ReflectionTestUtils.getField(beanPostProcessor, "schemaGenerator");
                    assertThat(ReflectionTestUtils.getField(schemaGenerator, "mapper"))
                            .isSameAs(codecs.schemaTree());
                });
    }

    @Test
    void nonWebEnablementIsPermanentlyDisabled()
    {
        modelFreeContextRunner
                .withPropertyValues(
                        "loomspan.observability.enabled=true",
                        "loomspan.observability.auth.api-key=0123456789abcdef0123456789abcdef",
                        "loomspan.skills.locations=classpath:/skills/none/**/*.yaml")
                .run(context ->
                {
                    ObservabilityActivationCoordinator activation =
                            context.getBean(ObservabilityActivationCoordinator.class);
                    assertThat(activation.state())
                            .isEqualTo(ObservabilityActivationCoordinator.State.DISABLED);
                    assertThat(activation.runtime()).isEmpty();
                });
    }

    @Test
    void reactiveWebEnablementIsPermanentlyDisabled()
    {
        reactiveContextRunner
                .withPropertyValues(
                        "loomspan.observability.enabled=true",
                        "loomspan.observability.auth.api-key=0123456789abcdef0123456789abcdef",
                        "loomspan.skills.locations=classpath:/skills/none/**/*.yaml")
                .run(context ->
                {
                    ObservabilityActivationCoordinator activation =
                            context.getBean(ObservabilityActivationCoordinator.class);
                    assertThat(activation.state())
                            .isEqualTo(ObservabilityActivationCoordinator.State.DISABLED);
                    assertThat(activation.runtime()).isEmpty();
                });
    }

    @Test
    void reusesSharedSkillInputContractResolverAcrossRegistrationPaths() {
        contextRunner
                .withUserConfiguration(MappedSkillTargetConfiguration.class)
                .withPropertyValues("loomspan.skills.locations=classpath:/skills/valid/mapped-method-skill.yaml")
                .run(context -> {
                    SkillInputContractResolver resolver = context.getBean(SkillInputContractResolver.class);
                    SkillMethodBeanPostProcessor beanPostProcessor = context.getBean(SkillMethodBeanPostProcessor.class);
                    Object registrar = context.getBean("yamlSkillCapabilityRegistrar");

                    assertThat(ReflectionTestUtils.getField(beanPostProcessor, "inputContractResolver")).isSameAs(resolver);
                    assertThat(ReflectionTestUtils.getField(registrar, "inputContractResolver")).isSameAs(resolver);
                });
    }

    @Test
    void invokesMappedSkillWithoutModelsOrChatModel() {
        modelFreeContextRunner
                .withUserConfiguration(MappedSkillTargetConfiguration.class)
                .withPropertyValues("loomspan.skills.locations=classpath:/skills/valid/model-free-mapped-skill.yaml")
                .run(context -> {
                    assertThat(context).hasNotFailed();
                    assertThat(context).hasSingleBean(YamlSkillCatalog.class);
                    assertThat(context).hasSingleBean(SkillImplementationTargetRegistry.class);
                    assertThat(context).hasSingleBean(CapabilityRegistry.class);
                    assertThat(context).hasSingleBean(SkillTemplate.class);
                    assertThat(context.getBeansOfType(ChatModel.class)).isEmpty();

                    CapabilityMetadata metadata = context.getBean(CapabilityRegistry.class)
                            .getCapability("modelFreeMappedSkill");
                    assertThat(metadata.skillExecution().configured()).isFalse();
                    assertThat(context.getBean(SkillTemplate.class)
                            .invoke("modelFreeMappedSkill", Map.of("input", "alpha")))
                            .isEqualTo("\"mapped:alpha\"");
                });
    }

    @Test
    void autoConfiguresExecutionCoordinatorWhenModelInteractionFactoryIsAvailable() {
        contextRunner
                .withPropertyValues("loomspan.skills.locations=classpath:/skills/valid/default-thinking-skill.yaml")
                .run(context -> assertThat(context).hasSingleBean(ExecutionCoordinator.class));
    }

    @Test
    void autoConfiguresDefaultSkillAdvisorResolver() {
        OpenAiChatModel openAiChatModel = Mockito.mock(OpenAiChatModel.class);
        contextRunner
                .withBean(OpenAiChatModel.class, () -> openAiChatModel)
                .withPropertyValues("loomspan.skills.locations=classpath:/skills/valid/default-thinking-skill.yaml")
                .run(context -> {
                    assertThat(context).hasSingleBean(SkillAdvisorResolver.class);
                    assertThat(context).hasSingleBean(SkillChatModelResolver.class);
                    assertThat(context).hasSingleBean(ModelInteractionFactory.class);
                    assertThat(context.getBean(SkillAdvisorResolver.class)).isInstanceOf(DefaultSkillAdvisorResolver.class);
                });
    }

    @Test
    void doesNotBackOffWhenApplicationRegistersInternalModelResolver() {
        SkillChatModelResolver unsupportedResolver = (skillName, configuration) -> new com.lokiscale.loomspan.internal.provider.ProviderConnectionRuntime(
                Mockito.mock(ChatModel.class), AiDriver.OPENAI,
                com.lokiscale.loomspan.internal.provider.AttemptOwnership.EXACT_ATTEMPT_OWNERSHIP,
                com.lokiscale.loomspan.internal.provider.ProviderRetryPolicy.from(new LoomspanProperties.ProviderRetryProperties()),
                ignored -> com.lokiscale.loomspan.internal.provider.ProviderFailureDetails.unknown());

        modelFreeContextRunner
                .withBean("unsupportedResolver", SkillChatModelResolver.class, () -> unsupportedResolver)
                .withPropertyValues("loomspan.skills.locations=classpath:/skills/none/**/*.yaml")
                .run(context -> assertThat(context).hasFailed());
    }

    @Test
    void bindsProviderAwareModelCatalogFromApplicationTestYaml() {
        contextRunner
                .withPropertyValues("loomspan.skills.locations=classpath:/skills/valid/default-thinking-skill.yaml")
                .run(context -> {
                    LoomspanProperties properties = context.getBean(LoomspanProperties.class);

                    assertThat(properties.getModels()).containsKeys("gpt-5", "claude-sonnet", "gemini-pro", "ollama-llama3");
                    assertThat(properties.getConnections().get(properties.getModels().get("gpt-5").getConnection()).getDriver()).isEqualTo(AiDriver.OPENAI);
                    assertThat(properties.getConnections().get(properties.getModels().get("claude-sonnet").getConnection()).getDriver()).isEqualTo(AiDriver.ANTHROPIC);
                    assertThat(properties.getConnections().get(properties.getModels().get("gemini-pro").getConnection()).getDriver()).isEqualTo(AiDriver.GEMINI);
                    assertThat(properties.getConnections().get(properties.getModels().get("ollama-llama3").getConnection()).getDriver()).isEqualTo(AiDriver.OLLAMA);
                    assertThat(properties.getModels().get("gpt-5").getProviderModel()).isEqualTo("openai/gpt-5");
                    assertThat(properties.getModels().get("claude-sonnet").getThinkingLevels()).containsExactlyInAnyOrder("low", "medium", "high");
                    assertThat(properties.getModels().get("ollama-llama3").getThinkingLevels()).isEmpty();
                });
    }

    @Test
    void supportsMultiProviderResolverRegistrationWithoutTypeCollapse() {
        OpenAiChatModel openAiChatModel = Mockito.mock(OpenAiChatModel.class);
        OllamaChatModel ollamaChatModel = Mockito.mock(OllamaChatModel.class);

        contextRunner
                .withBean(OpenAiChatModel.class, () -> openAiChatModel)
                .withBean(OllamaChatModel.class, () -> ollamaChatModel)
                .withPropertyValues("loomspan.skills.locations=classpath:/skills/valid/default-thinking-skill.yaml")
                .run(context -> {
                    SkillChatModelResolver resolver = context.getBean(SkillChatModelResolver.class);

                    assertThat(resolver.resolve("openaiSkill", new EffectiveSkillExecutionConfiguration(
                            "gpt-5", "openai-main", AiDriver.OPENAI, "openai/gpt-5", "medium")))
                            .extracting(com.lokiscale.loomspan.internal.provider.ProviderConnectionRuntime::chatModel)
                            .isInstanceOf(OpenAiChatModel.class);
                    assertThat(resolver.resolve("ollamaSkill", new EffectiveSkillExecutionConfiguration(
                            "ollama-llama3", "ollama-main", AiDriver.OLLAMA, "llama3.2", null)))
                            .extracting(com.lokiscale.loomspan.internal.provider.ProviderConnectionRuntime::chatModel)
                            .isInstanceOf(OllamaChatModel.class);
                });
    }

    @Test
    void exposesModelInteractionFactoryBackedByResolver() {
        OpenAiChatModel openAiChatModel = Mockito.mock(OpenAiChatModel.class);

        contextRunner
                .withBean(OpenAiChatModel.class, () -> openAiChatModel)
                .withPropertyValues("loomspan.skills.locations=classpath:/skills/valid/default-thinking-skill.yaml")
                .run(context -> {
                    assertThat(context).hasSingleBean(SkillChatModelResolver.class);
                    assertThat(context).hasSingleBean(ModelInteractionFactory.class);
                    assertThat(context.getBean(ModelInteractionFactory.class)).isInstanceOf(SpringAiModelInteractionFactory.class);
                });
    }

    @Test
    void registersYamlCapabilityMetadataWithEffectiveExecutionDescriptor() {
        contextRunner
                .withPropertyValues("loomspan.skills.locations=classpath:/skills/valid/default-thinking-skill.yaml")
                .run(context -> {
                    CapabilityRegistry capabilityRegistry = context.getBean(CapabilityRegistry.class);
                    CapabilityMetadata metadata = capabilityRegistry.getCapability("thinkingDefaultSkill");

                    assertThat(metadata).isNotNull();
                    assertThat(metadata.skillExecution().configured()).isTrue();
                    assertThat(metadata.skillExecution().frameworkModel()).isEqualTo("gpt-5");
                    assertThat(metadata.skillExecution().driver()).isEqualTo(AiDriver.OPENAI);
                    assertThat(metadata.skillExecution().providerModel()).isEqualTo("openai/gpt-5");
                    assertThat(metadata.skillExecution().thinkingLevel()).isEqualTo("medium");
                });
    }

    @Test
    void publishesYamlSkillAndKeepsDiscoveredTargetInternal() {
        contextRunner
                .withUserConfiguration(MappedSkillTargetConfiguration.class)
                .withPropertyValues("loomspan.skills.locations=classpath:/skills/valid/mapped-method-skill.yaml")
                .run(context -> {
                    CapabilityRegistry capabilityRegistry = context.getBean(CapabilityRegistry.class);
                    SkillImplementationTargetRegistry targetRegistry = context.getBean(SkillImplementationTargetRegistry.class);

                    assertThat(capabilityRegistry.getCapability("deterministicTarget")).isNull();
                    assertThat(capabilityRegistry.getCapability("mappedMethodSkill")).isNotNull();
                    assertThat(targetRegistry.getTarget("targetBean#deterministicTarget")).isNotNull();
                });
    }

    @Configuration(proxyBeanMethods = false)
    static class MappedSkillTargetConfiguration {

        @Bean
        TargetBean targetBean() {
            return new TargetBean();
        }
    }

    static class TargetBean {

        @SkillMethod(description = "Deterministic target")
        String deterministicTarget(String input) {
            return "mapped:" + input;
        }
    }
}
