package com.lokiscale.loomspan.architecture;

import com.tngtech.archunit.core.importer.ClassFileImporter;
import com.tngtech.archunit.core.importer.ImportOption;
import com.lokiscale.loomspan.autoconfigure.LoomspanAutoConfiguration;
import com.lokiscale.loomspan.autoconfigure.LoomspanAiAutoConfiguration;
import com.lokiscale.loomspan.autoconfigure.LoomspanObservabilityWebAutoConfiguration;
import com.lokiscale.loomspan.internal.chat.SkillChatModelResolver;
import com.lokiscale.loomspan.internal.security.AccessGuard;
import com.lokiscale.loomspan.internal.serialization.LoomspanJacksonCodecs;
import org.junit.jupiter.api.Test;
import org.springframework.boot.autoconfigure.condition.ConditionalOnMissingBean;
import org.springframework.context.annotation.Bean;

import java.lang.annotation.Annotation;
import java.lang.reflect.AnnotatedElement;
import java.lang.reflect.Modifier;
import java.util.ArrayList;
import java.util.Arrays;
import java.util.HashSet;
import java.util.List;
import java.util.Set;
import java.util.stream.Collectors;

import static org.assertj.core.api.Assertions.assertThat;
import static com.tngtech.archunit.lang.syntax.ArchRuleDefinition.noClasses;

class LoomspanAutoConfigurationBoundaryTest
{
    private static final Set<String> CORE_BEAN_FACTORIES = Set.of(
            "capabilityRegistry", "skillImplementationTargetRegistry", "LoomspanExceptionTransformer",
            "skillMethodBeanPostProcessor", "LoomspanSessionRunner", "yamlSkillCatalog",
            "yamlSkillCapabilityRegistrar", "skillInputContractResolver", "skillInputValidator",
            "accessGuard", "skillVisibilityResolver", "virtualFileSystem", "refResolver",
            "missionInputMaterializer", "capabilityExecutionRouter",
            "skillTemplate", "planTaskLinker", "modelUsageExtractor", "usageMetricsRecorder",
            "sessionUsageService", "executionStateService", "planningService", "toolSurfaceService",
            "capabilityInvoker", "LoomspanMissionExecutor", "missionExecutionEngine",
            "stepLoopMissionExecutionEngine", "executionCoordinator", "observabilityActivationCoordinator",
            "observabilityNonWebActivation", "observabilityReactiveWebActivation");

    private static final Set<String> AI_BEAN_FACTORIES = Set.of(
            "namedAiConnectionRegistry", "skillChatModelResolver", "springAiChatOptionsContributor",
            "skillAdvisorResolver", "modelInteractionFactory");

    private static final Set<String> WEB_BEAN_FACTORIES = Set.of(
            "observabilityJsonCodec", "observabilityProblemMapper",
            "observabilityAccessService", "observabilityDtoMapper",
            "observabilityCursorCodec", "boundedJsonPageWriter", "observabilityRestController",
            "observabilityRouteCollisionDetector", "observabilityRouteRegistrar",
            "observabilityApiKeyFilter");

    private static final Set<String> SUPPORTED_LOOMSPAN_BEAN_OVERRIDES = Set.of();

    private final Set<String> productionClassNames = new ClassFileImporter()
            .withImportOption(ImportOption.Predefined.DO_NOT_INCLUDE_TESTS)
            .importPackages("com.lokiscale.loomspan")
            .stream()
            .map(javaClass -> javaClass.getName())
            .collect(Collectors.toSet());

    @Test
    void supportedLoomspanBeanOverrideAllowlistIsEmpty()
    {
        assertThat(SUPPORTED_LOOMSPAN_BEAN_OVERRIDES).isEmpty();
    }

    @Test
    void everyBeanFactoryIsClassifiedAndPackagePrivate()
    {
        var beanMethods = Arrays.stream(LoomspanAutoConfiguration.class.getDeclaredMethods())
                .filter(method -> method.isAnnotationPresent(Bean.class))
                .toList();

        assertThat(beanMethods.stream().map(method -> method.getName()).collect(Collectors.toSet()))
                .containsExactlyInAnyOrderElementsOf(CORE_BEAN_FACTORIES);
        assertThat(beanMethods)
                .allSatisfy(method -> assertThat(method.getModifiers())
                        .as("Framework-owned bean method %s must not be public or protected", method.getName())
                        .matches(modifiers -> !Modifier.isPublic(modifiers) && !Modifier.isProtected(modifiers)));
    }

    @Test
    void everyAiBeanFactoryIsClassifiedAndPackagePrivate()
    {
        var beanMethods = Arrays.stream(LoomspanAiAutoConfiguration.class.getDeclaredMethods())
                .filter(method -> method.isAnnotationPresent(Bean.class))
                .toList();

        assertThat(beanMethods.stream().map(method -> method.getName()).collect(Collectors.toSet()))
                .containsExactlyInAnyOrderElementsOf(AI_BEAN_FACTORIES);
        assertThat(beanMethods)
                .allSatisfy(method -> assertThat(method.getModifiers())
                        .as("Framework-owned AI bean method %s must not be public or protected", method.getName())
                        .matches(modifiers -> !Modifier.isPublic(modifiers) && !Modifier.isProtected(modifiers)));
    }

    @Test
    void everyWebBeanFactoryIsClassifiedAndPackagePrivate()
    {
        var beanMethods = Arrays.stream(LoomspanObservabilityWebAutoConfiguration.class.getDeclaredMethods())
                .filter(method -> method.isAnnotationPresent(Bean.class))
                .toList();

        assertThat(beanMethods.stream().map(method -> method.getName()).collect(Collectors.toSet()))
                .containsExactlyInAnyOrderElementsOf(WEB_BEAN_FACTORIES);
        assertThat(beanMethods)
                .allSatisfy(method -> assertThat(method.getModifiers())
                        .matches(modifiers -> !Modifier.isPublic(modifiers) && !Modifier.isProtected(modifiers)));
    }

    @Test
    void productionTypesDoNotUseConditionalOnMissingBean() throws Exception
    {
        List<String> offenders = new ArrayList<>();
        ClassLoader classLoader = Thread.currentThread().getContextClassLoader();

        for (String className : productionClassNames)
        {
            Class<?> type = Class.forName(className, false, classLoader);
            if (hasAnnotationOrMetaAnnotation(type, ConditionalOnMissingBean.class, new HashSet<>()))
            {
                offenders.add(type.getName());
            }
            Arrays.stream(type.getDeclaredMethods())
                    .filter(method -> hasAnnotationOrMetaAnnotation(
                            method, ConditionalOnMissingBean.class, new HashSet<>()))
                    .map(method -> type.getName() + "#" + method.getName())
                    .forEach(offenders::add);
        }

        assertThat(offenders)
                .as("No production Loomspan type or method may declare a direct or composed @ConditionalOnMissingBean replacement seam")
                .isEmpty();
    }

    @Test
    void accessGuardAndChatModelResolverAreInternalFrameworkOwnedTypes()
    {
        assertThat(AccessGuard.class.getPackageName()).startsWith("com.lokiscale.loomspan.internal.");
        assertThat(SkillChatModelResolver.class.getPackageName()).startsWith("com.lokiscale.loomspan.internal.");
        assertThat(CORE_BEAN_FACTORIES).contains("accessGuard");
        assertThat(AI_BEAN_FACTORIES).contains("skillChatModelResolver");
    }

    @Test
    void neutralCoreDoesNotUseSpringAiSchemaInfrastructure()
    {
        noClasses().that().resideInAPackage("com.lokiscale.loomspan.internal.core..")
                .should().dependOnClassesThat().resideInAPackage("org.springframework.ai.util.json.schema..")
                .check(new ClassFileImporter().withImportOption(ImportOption.Predefined.DO_NOT_INCLUDE_TESTS)
                        .importPackages("com.lokiscale.loomspan.internal.core"));
    }

    @Test
    void purposeOwnedCodecRolesAreRequiredByEveryConfiguredSerializationBoundary()
    {
        Set<String> codecBackedCoreFactories = Set.of(
                "skillMethodBeanPostProcessor", "LoomspanSessionRunner", "yamlSkillCatalog",
                "skillInputContractResolver", "skillTemplate", "planningService",
                "stepLoopMissionExecutionEngine");
        assertThat(Arrays.stream(LoomspanAutoConfiguration.class.getDeclaredMethods())
                .filter(method -> codecBackedCoreFactories.contains(method.getName())))
                .allSatisfy(method -> assertThat(method.getParameterTypes())
                        .as("%s must consume a purpose-owned codec role", method.getName())
                        .contains(LoomspanJacksonCodecs.class));

        Set<String> codecBackedAiFactories = Set.of("namedAiConnectionRegistry", "skillAdvisorResolver");
        assertThat(Arrays.stream(LoomspanAiAutoConfiguration.class.getDeclaredMethods())
                .filter(method -> codecBackedAiFactories.contains(method.getName())))
                .allSatisfy(method -> assertThat(method.getParameterTypes())
                        .as("%s must consume a purpose-owned codec role", method.getName())
                        .contains(LoomspanJacksonCodecs.class));

        var observabilityCodecFactory = Arrays.stream(LoomspanObservabilityWebAutoConfiguration.class.getDeclaredMethods())
                .filter(method -> method.getName().equals("observabilityJsonCodec"))
                .findFirst().orElseThrow();
        assertThat(observabilityCodecFactory.getParameterTypes()).contains(LoomspanJacksonCodecs.class);
    }

    private boolean hasAnnotationOrMetaAnnotation(AnnotatedElement element,
            Class<? extends Annotation> target,
            Set<Class<? extends Annotation>> visited)
    {
        for (Annotation annotation : element.getDeclaredAnnotations())
        {
            Class<? extends Annotation> annotationType = annotation.annotationType();
            if (annotationType.equals(target))
            {
                return true;
            }
            if (visited.add(annotationType)
                    && hasAnnotationOrMetaAnnotation(annotationType, target, visited))
            {
                return true;
            }
        }
        return false;
    }
}
