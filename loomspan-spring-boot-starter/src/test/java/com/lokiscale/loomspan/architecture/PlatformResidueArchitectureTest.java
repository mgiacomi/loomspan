package com.lokiscale.loomspan.architecture;

import com.tngtech.archunit.core.domain.JavaClass;
import com.tngtech.archunit.core.importer.ClassFileImporter;
import com.tngtech.archunit.core.importer.ImportOption;
import org.junit.jupiter.api.Test;

import java.io.IOException;
import java.nio.file.Files;
import java.nio.file.Path;
import java.util.List;
import java.util.Set;
import java.util.stream.Stream;

import static org.assertj.core.api.Assertions.assertThat;

class PlatformResidueArchitectureTest
{
    private static final List<String> SPRING_AI_PACKAGES = List.of(
            "com.lokiscale.loomspan.internal.chat",
            "com.lokiscale.loomspan.internal.linter",
            "com.lokiscale.loomspan.internal.outputschema",
            "com.lokiscale.loomspan.internal.provider",
            "com.lokiscale.loomspan.internal.runtime.evidence",
            "com.lokiscale.loomspan.internal.runtime.usage",
            "com.lokiscale.loomspan.internal.springai");

    private final Set<JavaClass> productionClasses = new ClassFileImporter()
            .withImportOption(ImportOption.Predefined.DO_NOT_INCLUDE_TESTS)
            .importPackages("com.lokiscale.loomspan")
            .stream()
            .collect(java.util.stream.Collectors.toSet());

    @Test
    void productionContainsNoObsoletePlatformTypesOrDependencies()
    {
        assertThat(productionClasses)
                .noneMatch(javaClass -> javaClass.getName().contains(".internal.springai.v1_1.")
                        || javaClass.getSimpleName().equals("SpringAiV11ProviderIntegration")
                        || javaClass.getSimpleName().equals("SkillChatClientFactory")
                        || javaClass.getSimpleName().equals("SkillChatOptionsAdapter")
                        || javaClass.getSimpleName().equals("ToolCallbackFactory")
                        || javaClass.getSimpleName().equals("ContractAwareToolCallback")
                        || javaClass.getSimpleName().equals("MissionUserMessageSender"));

        assertThat(productionClasses.stream()
                .flatMap(javaClass -> javaClass.getDirectDependenciesFromSelf().stream())
                .map(dependency -> dependency.getTargetClass().getName()))
                .noneMatch(name -> name.startsWith("org.springframework.retry.")
                        || name.startsWith("com.fasterxml.jackson.core.")
                        || name.startsWith("com.fasterxml.jackson.databind.")
                        || name.startsWith("com.fasterxml.jackson.dataformat."));
    }

    @Test
    void springAiUsageIsContainedToApprovedIntegrationPackages()
    {
        assertThat(productionClasses.stream()
                .filter(javaClass -> javaClass.getDirectDependenciesFromSelf().stream()
                        .anyMatch(dependency -> dependency.getTargetClass().getName().startsWith("org.springframework.ai.")))
                .map(JavaClass::getPackageName))
                .allMatch(packageName -> SPRING_AI_PACKAGES.stream().anyMatch(packageName::startsWith));
    }

    @Test
    void testsDoNotReintroduceFluentChatClientDoubles() throws IOException
    {
        Path testSources = Path.of("src", "test", "java");
        try (Stream<Path> files = Files.walk(testSources))
        {
            List<String> offenders = files
                    .filter(path -> path.toString().endsWith(".java"))
                    .filter(path -> containsAny(path,
                            "implements " + "ChatClient",
                            "implements org.springframework.ai.chat.client." + "ChatClient",
                            "ChatClientRequestSpec" + "Defaults",
                            "CallResponseSpec" + "Defaults"))
                    .map(testSources::relativize)
                    .map(Path::toString)
                    .toList();

            assertThat(offenders)
                    .as("Tests must use ModelInteraction fakes or real ChatClient instances, not handwritten fluent clients")
                    .isEmpty();
        }
    }

    private static boolean containsAny(Path path, String... needles)
    {
        try
        {
            String source = Files.readString(path);
            return Stream.of(needles).anyMatch(source::contains);
        }
        catch (IOException ex)
        {
            throw new IllegalStateException("Failed to inspect " + path, ex);
        }
    }
}
