package com.lokiscale.loomspan.internal.skill;

import com.lokiscale.loomspan.autoconfigure.LoomspanAutoConfiguration;
import com.lokiscale.loomspan.autoconfigure.AiDriver;
import com.lokiscale.loomspan.api.SkillMethod;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.DynamicTest;
import org.junit.jupiter.api.TestFactory;
import org.junit.jupiter.api.extension.ExtendWith;
import org.springframework.boot.autoconfigure.AutoConfigurations;
import org.springframework.boot.autoconfigure.context.ConfigurationPropertiesAutoConfiguration;
import org.springframework.boot.env.YamlPropertySourceLoader;
import org.springframework.boot.test.context.runner.ApplicationContextRunner;
import org.springframework.boot.test.system.CapturedOutput;
import org.springframework.boot.test.system.OutputCaptureExtension;
import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Configuration;
import org.springframework.core.env.PropertySource;
import org.springframework.core.io.ClassPathResource;

import java.util.List;

import static org.assertj.core.api.Assertions.assertThat;

@ExtendWith(OutputCaptureExtension.class)
class YamlSkillCatalogTests {

    private final ApplicationContextRunner modelFreeContextRunner = new ApplicationContextRunner()
            .withConfiguration(AutoConfigurations.of(
                    ConfigurationPropertiesAutoConfiguration.class,
                    com.lokiscale.loomspan.autoconfigure.LoomspanJacksonAutoConfiguration.class,
                    LoomspanAutoConfiguration.class,
                    com.lokiscale.loomspan.autoconfigure.LoomspanAiAutoConfiguration.class));

    private final ApplicationContextRunner contextRunner = new ApplicationContextRunner()
            .withConfiguration(AutoConfigurations.of(
                    ConfigurationPropertiesAutoConfiguration.class,
                    com.lokiscale.loomspan.autoconfigure.LoomspanJacksonAutoConfiguration.class,
                    LoomspanAutoConfiguration.class,
                    com.lokiscale.loomspan.autoconfigure.LoomspanAiAutoConfiguration.class))
            .withInitializer(context -> {
                try {
                    YamlPropertySourceLoader loader = new YamlPropertySourceLoader();
                    for (PropertySource<?> propertySource : loader.load("application-test", new ClassPathResource("application-test.yml"))) {
                        context.getEnvironment().getPropertySources().addLast(propertySource);
                    }
                }
                catch (java.io.IOException ex) {
                    throw new IllegalStateException("Failed to load application-test.yml", ex);
                }
            });

    @Test
    void acceptsProviderPortablePublicSkillNames() {
        List<String> expectedNames = List.of(
                "A",
                "_",
                "expenseLookup",
                "expense_lookup",
                "_internalStyleAllowed",
                "Skill2",
                "CaseName",
                "caseName",
                "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa");

        contextRunner
                .withPropertyValues("loomspan.skills.locations=classpath:/skills/public-name/valid/*.yaml")
                .run(context -> {
                    assertThat(context).hasNotFailed();
                    YamlSkillCatalog catalog = context.getBean(YamlSkillCatalog.class);
                    assertThat(catalog.getSkills())
                            .extracting(definition -> definition.manifest().getName())
                            .containsExactlyInAnyOrderElementsOf(expectedNames);
                    expectedNames.forEach(name -> assertThat(catalog.getSkill(name))
                            .isNotNull()
                            .extracting(definition -> definition.manifest().getName())
                            .isEqualTo(name));
                    assertThat(catalog.getSkill("expenselookup")).isNull();
                });
    }

    @TestFactory
    List<DynamicTest> rejectsNonPortablePublicSkillNames() {
        record Case(String filename, String value) {}
        return List.of(
                new Case("leading-digit.yaml", "2expenseLookup"),
                new Case("dot.yaml", "mapped" + ".method.skill"),
                new Case("dash.yaml", "expense-lookup"),
                new Case("space.yaml", "expense lookup"),
                new Case("leading-space.yaml", " expenseLookup"),
                new Case("trailing-space.yaml", "expenseLookup "),
                new Case("hash.yaml", "expenseService#getLatestExpenses"),
                new Case("slash.yaml", "expense/lookup"),
                new Case("colon.yaml", "expense:lookup"),
                new Case("unicode.yaml", "expénsèLookup"),
                new Case("too-long.yaml", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"))
                .stream()
                .map(testCase -> DynamicTest.dynamicTest(testCase.filename(), () -> contextRunner
                        .withPropertyValues("loomspan.skills.locations=classpath:/skills/public-name/invalid/" + testCase.filename())
                        .run(context -> assertThat(context.getStartupFailure())
                                .isNotNull()
                                .hasMessageContaining(testCase.filename())
                                .hasMessageContaining("field 'name'")
                                .hasMessageContaining("'" + testCase.value() + "'")
                                .hasMessageContaining("^[A-Za-z_][A-Za-z0-9_]{0,63}$")
                                .hasMessageContaining("1-64 characters")
                                .hasMessageContaining("start with a letter or underscore")
                                .hasMessageContaining("only letters, digits, or underscores")
                                .hasMessageContaining("mappedMethodSkill"))))
                .toList();
    }

    @TestFactory
    List<DynamicTest> keepsMissingAndBlankNamesOnRequiredFieldPath() {
        return List.of("missing.yaml", "blank.yaml")
                .stream()
                .map(filename -> DynamicTest.dynamicTest(filename, () -> contextRunner
                        .withPropertyValues("loomspan.skills.locations=classpath:/skills/public-name/invalid/" + filename)
                        .run(context -> assertThat(context.getStartupFailure())
                                .isNotNull()
                                .hasMessageContaining(filename)
                                .hasMessageContaining("field 'name'")
                                .hasMessageContaining("required field is missing or blank")
                                .hasMessageNotContaining("invalid public skill name"))))
                .toList();
    }

    @Test
    void validatesPublicNameBeforeMappedManifestFields() {
        contextRunner
                .withPropertyValues("loomspan.skills.locations=classpath:/skills/public-name/invalid/mapped-invalid-name.yaml")
                .run(context -> assertThat(context.getStartupFailure())
                        .isNotNull()
                        .hasMessageContaining("mapped-invalid-name.yaml")
                        .hasMessageContaining("field 'name'")
                        .hasMessageContaining("mapped" + ".method.skill")
                        .hasMessageContaining("^[A-Za-z_][A-Za-z0-9_]{0,63}$")
                        .hasMessageNotContaining("field 'mapping.target_id'"));
    }

    @Test
    void discoversMappedSkillWhenModelCatalogIsEmpty() {
        modelFreeContextRunner
                .withUserConfiguration(TargetBeanConfiguration.class)
                .withPropertyValues("loomspan.skills.locations=classpath:/skills/valid/model-free-mapped-skill.yaml")
                .run(context -> {
                    assertThat(context).hasNotFailed();
                    YamlSkillDefinition definition = context.getBean(YamlSkillCatalog.class)
                            .getSkill("modelFreeMappedSkill");
                    assertThat(definition).isNotNull();
                    assertThat(definition.implementationType())
                            .isEqualTo(com.lokiscale.loomspan.internal.core.PublicSkillImplementationType.MAPPED_JAVA);
                    assertThat(definition.executionConfiguration()).isNull();
                });
    }

    @Test
    void rejectsLlmSkillWhenModelCatalogIsEmpty() {
        modelFreeContextRunner
                .withPropertyValues("loomspan.skills.locations=classpath:/skills/invalid/llm-missing-model.yaml")
                .run(context -> assertThat(context.getStartupFailure())
                        .isNotNull()
                        .hasMessageContaining("llmMissingModel")
                        .hasMessageContaining("llm-missing-model.yaml")
                        .hasMessageContaining("field 'model'")
                        .hasMessageContaining("required field is missing or blank")
                        .hasMessageContaining("declare a configured model"));
    }

    @TestFactory
    List<DynamicTest> rejectsDeclaredMappingWithoutNonBlankTarget() {
        record Case(String filename, String skillName) {}
        return List.of(
                new Case("mapped-null-mapping.yaml", "mappedNullMapping"),
                new Case("mapped-empty-mapping.yaml", "mappedEmptyMapping"),
                new Case("mapped-non-object-mapping.yaml", "mappedNonObjectMapping"),
                new Case("mapped-non-string-target.yaml", "mappedNonStringTarget"),
                new Case("mapped-null-target.yaml", "mappedNullTarget"),
                new Case("mapped-blank-target.yaml", "mappedBlankTarget"))
                .stream()
                .map(testCase -> DynamicTest.dynamicTest(testCase.filename(), () -> contextRunner
                        .withPropertyValues("loomspan.skills.locations=classpath:/skills/invalid/" + testCase.filename())
                        .run(context -> assertThat(context.getStartupFailure())
                                .isNotNull()
                                .hasMessageContaining(testCase.skillName())
                                .hasMessageContaining(testCase.filename())
                                .hasMessageContaining("field 'mapping.target_id'")
                                .hasMessageContaining("target_id must be non-blank")
                                .hasMessageContaining("declare a valid Java target or remove mapping"))))
                .toList();
    }

    @TestFactory
    List<DynamicTest> rejectsInapplicableMappedFieldByDeclarationPresence() {
        record Case(String filename, String skillName, String field, String explanation, String remedy) {}
        return List.of(
                new Case("mapped-with-model-null.yaml", "mappedWithModelNull", "model", "no model executes", "remove the field"),
                new Case("mapped-with-thinking-level-blank.yaml", "mappedWithThinkingBlank", "thinking_level", "no model executes", "remove the field"),
                new Case("mapped-with-prompt-null.yaml", "mappedWithPromptNull", "prompt", "no model executes", "remove the field"),
                new Case("mapped-with-input-schema-null.yaml", "mappedWithInputNull", "input_schema", "reflected input contract", "create a Java adapter target"),
                new Case("mapped-with-output-schema-null.yaml", "mappedWithOutputNull", "output_schema", "owns the returned value", "create a Java adapter target"),
                new Case("mapped-with-planning-mode-false.yaml", "mappedWithPlanningFalse", "planning_mode", "no model executes", "remove the field"),
                new Case("mapped-with-planning-mode-object.yaml", "mappedWithPlanningObject", "planning_mode", "no model executes", "remove the field"),
                new Case("mapped-with-max-steps-zero.yaml", "mappedWithStepsZero", "max_steps", "no model executes", "remove the field"),
                new Case("mapped-with-max-steps-text.yaml", "mappedWithStepsText", "max_steps", "no model executes", "remove the field"),
                new Case("mapped-with-allowed-skills-empty.yaml", "mappedWithAllowedEmpty", "allowed_skills", "LLM parent", "declare the mapped child"),
                new Case("mapped-with-linter-null.yaml", "mappedWithLinterNull", "linter", "model-output validation", "remove the field"),
                new Case("mapped-with-output-schema-retries-zero.yaml", "mappedWithRetriesZero", "output_schema_max_retries", "model-output validation", "remove the field"))
                .stream()
                .map(testCase -> DynamicTest.dynamicTest(testCase.filename(), () -> contextRunner
                        .withPropertyValues("loomspan.skills.locations=classpath:/skills/invalid/" + testCase.filename())
                        .run(context -> assertThat(context.getStartupFailure())
                                .isNotNull()
                                .hasMessageContaining(testCase.skillName())
                                .hasMessageContaining(testCase.filename())
                                .hasMessageContaining("field '" + testCase.field() + "'")
                                .hasMessageContaining(testCase.explanation())
                                .hasMessageContaining(testCase.remedy()))))
                .toList();
    }

    @Test
    void reportsMappedApplicabilityErrorsInStableOrderBeforeContentAndTargetValidation() {
        contextRunner
                .withPropertyValues("loomspan.skills.locations=classpath:/skills/invalid/mapped-with-multiple-inapplicable-fields.yaml")
                .run(context -> assertThat(context.getStartupFailure())
                        .isNotNull()
                        .hasMessageContaining("mappedWithMultipleInvalid")
                        .hasMessageContaining("mapped-with-multiple-inapplicable-fields.yaml")
                        .hasMessageContaining("field 'model'")
                        .hasMessageContaining("no model executes")
                        .hasMessageNotContaining("unknown implementation target"));
    }

    @Test
    void exposesCompleteStableMappedApplicabilityOrder() {
        assertThat(YamlSkillCatalog.mappedInapplicableFields())
                .extracting(YamlSkillManifest.Field::yamlName)
                .containsExactly(
                        "model",
                        "thinking_level",
                        "prompt",
                        "input_schema",
                        "output_schema",
                        "planning_mode",
                        "max_steps",
                        "allowed_skills",
                        "linter",
                        "output_schema_max_retries");
    }

    @Test
    void defaultsThinkingLevelToMediumWhenModelSupportsThinking() {
        contextRunner
                .withPropertyValues("loomspan.skills.locations=classpath:/skills/valid/default-thinking-skill.yaml")
                .run(context -> {
                    YamlSkillCatalog catalog = context.getBean(YamlSkillCatalog.class);

                    assertThat(catalog.getSkill("thinkingDefaultSkill")).isNotNull();
                    assertThat(catalog.getSkill("thinkingDefaultSkill").executionConfiguration())
                            .extracting(
                                    EffectiveSkillExecutionConfiguration::frameworkModel,
                                    EffectiveSkillExecutionConfiguration::connection,
                                    EffectiveSkillExecutionConfiguration::driver,
                                    EffectiveSkillExecutionConfiguration::providerModel,
                                    EffectiveSkillExecutionConfiguration::thinkingLevel)
                            .containsExactly("gpt-5", "openai-main", AiDriver.OPENAI, "openai/gpt-5", "medium");
                });
    }

    @Test
    void omitsThinkingLevelWhenSelectedModelHasNoThinkingSupport() {
        contextRunner
                .withPropertyValues("loomspan.skills.locations=classpath:/skills/valid/non-thinking-skill.yaml")
                .run(context -> {
                    YamlSkillCatalog catalog = context.getBean(YamlSkillCatalog.class);

                    assertThat(catalog.getSkill("nonThinkingSkill")).isNotNull();
                    assertThat(catalog.getSkill("nonThinkingSkill").executionConfiguration().providerModel()).isEqualTo("llama3.2");
                    assertThat(catalog.getSkill("nonThinkingSkill").executionConfiguration().thinkingLevel()).isNull();
                });
    }

    @Test
    void failsStartupWhenYamlSkillReferencesUnknownModel() {
        contextRunner
                .withPropertyValues("loomspan.skills.locations=classpath:/skills/invalid/unknown-model-skill.yaml")
                .run(context -> {
                    assertThat(context.getStartupFailure())
                            .isNotNull()
                            .hasMessageContaining("unknown-model-skill.yaml")
                            .hasMessageContaining("field 'model'")
                            .hasMessageContaining("unknown model 'missing-model'");
                });
    }

    @Test
    void failsStartupWhenThinkingLevelIsUnsupportedForModel() {
        contextRunner
                .withPropertyValues("loomspan.skills.locations=classpath:/skills/invalid/unsupported-thinking-skill.yaml")
                .run(context -> {
                    assertThat(context.getStartupFailure())
                            .isNotNull()
                            .hasMessageContaining("invalidUnsupportedThinkingSkill")
                            .hasMessageContaining("unsupported-thinking-skill.yaml")
                            .hasMessageContaining("field 'thinking_level'")
                            .hasMessageContaining("unsupported thinking_level 'high'");
                });
    }

    @Test
    void includesPublicSkillNameInPostParseLlmValidationErrors() {
        contextRunner
                .withPropertyValues("loomspan.skills.locations=classpath:/skills/invalid/negative-linter-max-retries-skill.yaml")
                .run(context -> assertThat(context.getStartupFailure())
                        .isNotNull()
                        .hasMessageContaining("invalidNegativeLinterMaxRetriesSkill")
                        .hasMessageContaining("negative-linter-max-retries-skill.yaml")
                        .hasMessageContaining("field 'linter.max_retries'"));
    }

    @Test
    void failsStartupWhenYamlSkillsShareDuplicateName() {
        contextRunner
                .withPropertyValues("loomspan.skills.locations=classpath:/skills/invalid/duplicate-name/*.yaml")
                .run(context -> {
                    assertThat(context.getStartupFailure())
                            .isNotNull()
                            .hasMessageContaining("second-skill.yaml")
                            .hasMessageContaining("field 'name'")
                            .hasMessageContaining("duplicate skill name 'duplicateSkill'");
                });
    }

    @Test
    void loadsYamlSkillsFromClasspathSkillsPattern() {
        contextRunner
                .withPropertyValues("loomspan.skills.locations=classpath:/skills/pattern/**/*.yaml")
                .run(context -> {
                    YamlSkillCatalog catalog = context.getBean(YamlSkillCatalog.class);

                    assertThat(catalog.getSkills()).hasSize(2);
                    assertThat(catalog.getSkills())
                            .extracting(definition -> definition.manifest().getName())
                            .containsExactly("patternTwoSkill", "patternOneSkill");
                });
    }

    @Test
    void loadsNoSkillsWhenConfiguredClasspathRootIsMissing() {
        contextRunner
                .withPropertyValues("loomspan.skills.locations=classpath:/skills/does-not-exist/**/*.yaml")
                .run(context -> {
                    assertThat(context).hasNotFailed();
                    assertThat(context.getBean(YamlSkillCatalog.class).getSkills()).isEmpty();
                });
    }

    @Test
    void loadsNoSkillsWhenClasspathRootExistsButHasNoYamlMatches() {
        contextRunner
                .withPropertyValues("loomspan.skills.locations=classpath:/skills/empty/**/*.yaml")
                .run(context -> {
                    assertThat(context).hasNotFailed();
                    assertThat(context.getBean(YamlSkillCatalog.class).getSkills()).isEmpty();
                });
    }

    @Test
    void loadsTypedManifestFieldsWhenPresent() {
        contextRunner
                .withUserConfiguration(TargetBeanConfiguration.class)
                .withPropertyValues(
                        "loomspan.skills.locations=classpath:/skills/valid/allowed-skills-root.yaml,classpath:/skills/valid/allowed-child-skill.yaml")
                .run(context -> {
                    YamlSkillCatalog catalog = context.getBean(YamlSkillCatalog.class);

                    assertThat(catalog.getSkill("rootVisibleSkill")).isNotNull();
                    assertThat(catalog.getSkill("rootVisibleSkill").allowedSkills())
                            .containsExactly("allowedVisibleSkill", "targetBean#deterministicTarget", "disallowedVisibleSkill");
                    assertThat(catalog.getSkill("allowedVisibleSkill").rbacRoles())
                            .containsExactly("ROLE_ALLOWED");
                });
    }

    @Test
    void returnsDefensiveManifestCopiesFromPublicCatalog() {
        contextRunner
                .withUserConfiguration(TargetBeanConfiguration.class)
                .withPropertyValues("loomspan.skills.locations=classpath:/skills/valid/allowed-child-skill.yaml")
                .run(context -> {
                    YamlSkillCatalog catalog = context.getBean(YamlSkillCatalog.class);
                    YamlSkillDefinition definition = catalog.getSkill("allowedVisibleSkill");

                    definition.manifest().setName("mutated.name");
                    definition.manifest().setRbacRoles(java.util.List.of("ROLE_MUTATED"));
                    definition.manifest().getMapping().setTargetId("mutatedBean#method");

                    assertThat(catalog.getSkill("allowedVisibleSkill").manifest().getName())
                            .isEqualTo("allowedVisibleSkill");
                    assertThat(catalog.getSkill("allowedVisibleSkill").rbacRoles())
                            .containsExactly("ROLE_ALLOWED");
                    assertThat(catalog.getSkill("allowedVisibleSkill").mappingTargetId())
                            .isEqualTo("targetBean#deterministicTarget");
                    assertThat(catalog.getSkill("mutated.name")).isNull();
                });
    }

    @Test
    void returnsDefensiveCopiesFromNestedManifestAccessors() {
        contextRunner
                .withPropertyValues(
                        "loomspan.skills.locations=classpath:/skills/valid/regex-linter-skill.yaml,"
                                + "classpath:/skills/valid/input-schema-skill.yaml,"
                                + "classpath:/skills/valid/output-schema-skill.yaml")
                .run(context -> {
                    YamlSkillCatalog catalog = context.getBean(YamlSkillCatalog.class);

                    catalog.getSkill("lintedSkill").linter().setType("mutated");
                    catalog.getSkill("inputSchemaSkill").inputSchema().setType("string");
                    catalog.getSkill("outputSchemaSkill").outputSchema().setType("string");

                    assertThat(catalog.getSkill("lintedSkill").linter().getType()).isEqualTo("regex");
                    assertThat(catalog.getSkill("inputSchemaSkill").inputSchema().getType()).isEqualTo("object");
                    assertThat(catalog.getSkill("outputSchemaSkill").outputSchema().getType()).isEqualTo("object");
                });
    }

    @Test
    void distinguishesLlmBackedAndMappedJavaImplementationTypes() {
        contextRunner
                .withUserConfiguration(TargetBeanConfiguration.class)
                .withPropertyValues("loomspan.skills.locations=classpath:/skills/valid/default-thinking-skill.yaml,classpath:/skills/valid/mapped-method-skill.yaml")
                .run(context -> {
                    YamlSkillCatalog catalog = context.getBean(YamlSkillCatalog.class);

                    assertThat(catalog.getSkill("thinkingDefaultSkill").implementationType())
                            .isEqualTo(com.lokiscale.loomspan.internal.core.PublicSkillImplementationType.LLM_BACKED);
                    assertThat(catalog.getSkill("mappedMethodSkill").implementationType())
                            .isEqualTo(com.lokiscale.loomspan.internal.core.PublicSkillImplementationType.MAPPED_JAVA);
                    assertThat(catalog.getSkill("mappedMethodSkill").mappingTargetId())
                            .isEqualTo("targetBean#deterministicTarget");
                    assertThat(catalog.getSkill("mappedMethodSkill").executionConfiguration()).isNull();
                });
    }

    @Test
    void loadsYamlSkillPromptAndNormalizesBlankPrompt() {
        contextRunner
                .withPropertyValues("loomspan.skills.locations=classpath:/skills/valid/prompt-skill.yaml,classpath:/skills/valid/blank-prompt-skill.yaml")
                .run(context -> {
                    YamlSkillCatalog catalog = context.getBean(YamlSkillCatalog.class);

                    assertThat(catalog.getSkill("promptSkill").prompt())
                            .contains("LONG_PROMPT_SENTINEL")
                            .contains("Follow the private skill instructions.");
                    assertThat(catalog.getSkill("blankPromptSkill").prompt()).isNull();
                });
    }

    @Test
    void rejectsUnknownPromptLikeFields() {
        contextRunner
                .withPropertyValues("loomspan.skills.locations=classpath:/skills/invalid/unknown-prompt-like-field-skill.yaml")
                .run(context -> assertThat(context.getStartupFailure())
                        .isNotNull()
                        .hasMessageContaining("unknown-prompt-like-field-skill.yaml")
                        .hasMessageContaining("field 'system_prompt'")
                        .hasMessageContaining("unknown field"));
    }

    @Test
    void rejectsPromptOnMappedYamlSkill() {
        contextRunner
                .withUserConfiguration(TargetBeanConfiguration.class)
                .withPropertyValues("loomspan.skills.locations=classpath:/skills/invalid/mapped-skill-with-prompt.yaml")
                .run(context -> assertThat(context.getStartupFailure())
                        .isNotNull()
                        .hasMessageContaining("mapped-skill-with-prompt.yaml")
                        .hasMessageContaining("field 'prompt'")
                        .hasMessageContaining("no model executes"));
    }

    @Test
    void defaultsTypedManifestFieldsWhenMissing() {
        contextRunner
                .withPropertyValues("loomspan.skills.locations=classpath:/skills/valid/default-thinking-skill.yaml")
                .run(context -> {
                    YamlSkillCatalog catalog = context.getBean(YamlSkillCatalog.class);

                    assertThat(catalog.getSkill("thinkingDefaultSkill").allowedSkills()).isEmpty();
                    assertThat(catalog.getSkill("thinkingDefaultSkill").rbacRoles()).isEmpty();
                    assertThat(catalog.getSkill("thinkingDefaultSkill").manifest().getPlanningMode()).isNull();
                });
    }

    @Test
    void loadsPlanningModeOverrideWhenPresent() {
        contextRunner
                .withPropertyValues("loomspan.skills.locations=classpath:/skills/valid/planning-disabled-skill.yaml")
                .run(context -> {
                    YamlSkillCatalog catalog = context.getBean(YamlSkillCatalog.class);

                    assertThat(catalog.getSkill("planningDisabledSkill")).isNotNull();
                    assertThat(catalog.getSkill("planningDisabledSkill").manifest().getPlanningMode()).isFalse();
                    assertThat(catalog.getSkill("planningDisabledSkill").planningModeEnabled(true)).isFalse();
                    assertThat(catalog.getSkill("planningDisabledSkill").planningModeEnabled(false)).isFalse();
                });
    }

    @Test
    void loadsTypedRegexLinterConfigurationWhenPresent() {
        contextRunner
                .withPropertyValues("loomspan.skills.locations=classpath:/skills/valid/regex-linter-skill.yaml")
                .run(context -> {
                    YamlSkillCatalog catalog = context.getBean(YamlSkillCatalog.class);

                    assertThat(catalog.getSkill("lintedSkill")).isNotNull();
                    assertThat(catalog.getSkill("lintedSkill").linter()).isNotNull();
                    assertThat(catalog.getSkill("lintedSkill").linter().getType()).isEqualTo("regex");
                    assertThat(catalog.getSkill("lintedSkill").linter().getMaxRetries()).isEqualTo(2);
                    assertThat(catalog.getSkill("lintedSkill").linter().getRegex()).isNotNull();
                    assertThat(catalog.getSkill("lintedSkill").linter().getRegex().getPattern()).isEqualTo("^```yaml[\\s\\S]*```$");
                    assertThat(catalog.getSkill("lintedSkill").linter().getRegex().getMessage()).isEqualTo("Return fenced YAML only.");
                });
    }

    @Test
    void defaultsLinterToAbsentWhenManifestDoesNotDeclareOne() {
        contextRunner
                .withPropertyValues("loomspan.skills.locations=classpath:/skills/valid/default-thinking-skill.yaml")
                .run(context -> {
                    YamlSkillCatalog catalog = context.getBean(YamlSkillCatalog.class);

                    assertThat(catalog.getSkill("thinkingDefaultSkill")).isNotNull();
                    assertThat(catalog.getSkill("thinkingDefaultSkill").linter()).isNull();
                });
    }

    @Test
    void defaultsOutputSchemaMaxRetriesToTwoWhenSchemaIsPresent() {
        contextRunner
                .withPropertyValues("loomspan.skills.locations=classpath:/skills/valid/output-schema-skill.yaml")
                .run(context -> {
                    YamlSkillCatalog catalog = context.getBean(YamlSkillCatalog.class);

                    assertThat(catalog.getSkill("outputSchemaSkill")).isNotNull();
                    assertThat(catalog.getSkill("outputSchemaSkill").manifest().getOutputSchemaMaxRetries()).isEqualTo(2);
                });
    }

    @Test
    void loadsYamlSkillInputSchemaWhenPresent() {
        contextRunner
                .withPropertyValues("loomspan.skills.locations=classpath:/skills/valid/input-schema-skill.yaml")
                .run(context -> {
                    YamlSkillDefinition definition = context.getBean(YamlSkillCatalog.class).getSkill("inputSchemaSkill");

                    assertThat(definition).isNotNull();
                    assertThat(definition.inputSchema()).isNotNull();
                    assertThat(definition.inputSchema().getType()).isEqualTo("object");
                    assertThat(definition.inputSchema().getRequired()).containsExactly("payload");
                });
    }

    @Test
    void acceptsAttachmentOnlyInInputSchema() {
        contextRunner
                .withPropertyValues("loomspan.skills.locations=classpath:/skills/valid/attachment-input-skill.yaml")
                .run(context -> {
                    YamlSkillDefinition definition = context.getBean(YamlSkillCatalog.class).getSkill("attachmentInputSkill");

                    assertThat(definition).isNotNull();
                    assertThat(definition.inputSchema().getProperties().get("image").getType()).isEqualTo("attachment");
                    assertThat(definition.inputSchema().getProperties().get("image").getMediaType()).isEqualTo("image");
                    assertThat(definition.inputSchema().getProperties().get("image").getAllowedContentTypes())
                            .containsExactly("image/jpeg");
                });
    }

    @Test
    void rejectsAttachmentFieldsOnOutputSchema() {
        contextRunner
                .withPropertyValues("loomspan.skills.locations=classpath:/skills/invalid/attachment-output-skill.yaml")
                .run(context -> assertThat(context.getStartupFailure())
                        .isNotNull()
                        .hasMessageContaining("attachment-output-skill.yaml")
                        .hasMessageContaining("field 'output_schema.properties.image.type'")
                        .hasMessageContaining("unsupported schema type 'attachment'"));
    }

    @Test
    void requiresAllowedContentTypesForAttachmentInputAndRejectsMediaFieldsElsewhere() {
        contextRunner
                .withPropertyValues("loomspan.skills.locations=classpath:/skills/invalid/attachment-missing-allowed-content-types-skill.yaml")
                .run(context -> assertThat(context.getStartupFailure())
                        .isNotNull()
                        .hasMessageContaining("attachment-missing-allowed-content-types-skill.yaml")
                        .hasMessageContaining("field 'input_schema.properties.image.allowed_content_types'")
                        .hasMessageContaining("must declare at least one content type"));

        contextRunner
                .withPropertyValues("loomspan.skills.locations=classpath:/skills/invalid/non-attachment-media-field-skill.yaml")
                .run(context -> assertThat(context.getStartupFailure())
                        .isNotNull()
                        .hasMessageContaining("non-attachment-media-field-skill.yaml")
                        .hasMessageContaining("field 'input_schema.properties.payload.media_type'")
                        .hasMessageContaining("is only supported for attachment schemas"));
    }

    @Test
    void failsStartupWhenInputSchemaUsesUnsupportedKeywordOrNonObjectRoot() {
        contextRunner
                .withPropertyValues("loomspan.skills.locations=classpath:/skills/invalid/input-schema-root-array-skill.yaml")
                .run(context -> assertThat(context.getStartupFailure())
                        .isNotNull()
                        .hasMessageContaining("input-schema-root-array-skill.yaml")
                        .hasMessageContaining("field 'input_schema.type'")
                        .hasMessageContaining("root input_schema type must be 'object'"));

        contextRunner
                .withPropertyValues("loomspan.skills.locations=classpath:/skills/invalid/input-schema-unsupported-keyword-skill.yaml")
                .run(context -> assertThat(context.getStartupFailure())
                        .isNotNull()
                        .hasMessageContaining("input-schema-unsupported-keyword-skill.yaml")
                        .hasMessageContaining("field 'input_schema.properties.payload.oneOf'")
                        .hasMessageContaining("unknown field"));
    }

    @Test
    void mappedYamlSkillWithMismatchedInputSchemaFailsStartup() {
        contextRunner
                .withUserConfiguration(TargetBeanConfiguration.class)
                .withPropertyValues("loomspan.skills.locations=classpath:/skills/invalid/mapped-method-skill-mismatched-input-schema.yaml")
                .run(context -> assertThat(context.getStartupFailure())
                        .isNotNull()
                        .hasMessageContaining("mapped-method-skill-mismatched-input-schema.yaml")
                        .hasMessageContaining("field 'input_schema'")
                        .hasMessageContaining("Java target's reflected input contract"));
    }

    @Test
    void mappedYamlSkillWithFormatMismatchedInputSchemaFailsStartup() {
        contextRunner
                .withUserConfiguration(TargetBeanConfiguration.class)
                .withPropertyValues("loomspan.skills.locations=classpath:/skills/invalid/mapped-method-skill-format-mismatched-input-schema.yaml")
                .run(context -> assertThat(context.getStartupFailure())
                        .isNotNull()
                        .hasMessageContaining("mapped-method-skill-format-mismatched-input-schema.yaml")
                        .hasMessageContaining("field 'input_schema'")
                        .hasMessageContaining("Java target's reflected input contract"));
    }

    @Test
    void loadsEvidenceContractWhenManifestDeclaresOne() {
        contextRunner
                .withPropertyValues("loomspan.skills.locations=classpath:/skills/valid/evidence-contract-skill.yaml")
                .run(context -> {
                    YamlSkillCatalog catalog = context.getBean(YamlSkillCatalog.class);

                    assertThat(catalog.getSkill("evidenceContractSkill")).isNotNull();
                    assertThat(catalog.getSkill("evidenceContractSkill").evidenceContract()
                            .canonicalExpressionForClaim("isDuplicate"))
                            .isEqualTo("invoiceParser and expenseLookup");
                });
    }

    @Test
    void failsStartupWhenEvidenceContractContainsBlankExpression() {
        contextRunner
                .withPropertyValues("loomspan.skills.locations=classpath:/skills/invalid/evidence-contract-blank-evidence-skill.yaml")
                .run(context -> assertThat(context.getStartupFailure())
                        .isNotNull()
                        .hasMessageContaining("evidence-contract-blank-evidence-skill.yaml")
                        .hasMessageContaining("field 'output_schema.properties.vendorName.evidence'")
                        .hasMessageContaining("expression must be a nonblank YAML string"));
    }

    @Test
    void usesExactValidatedPublicNameInEvidenceExpression() {
        contextRunner
                .withPropertyValues("loomspan.skills.locations=classpath:/skills/valid/evidence-contract-hash-public-name-skill.yaml")
                .run(context -> {
                    assertThat(context).hasNotFailed();
                    assertThat(context.getBean(YamlSkillCatalog.class)
                            .getSkill("evidenceHashNameSkill")
                            .evidenceContract()
                            .canonicalExpressionForClaim("result"))
                            .isEqualTo("reviewSkill");
                });
    }

    @Test
    void rejectsListValuedEvidenceExpressionWithoutStringCoercion() {
        contextRunner
                .withPropertyValues("loomspan.skills.locations=classpath:/skills/invalid/evidence-contract-list-expression-skill.yaml")
                .run(context -> assertThat(context.getStartupFailure())
                        .isNotNull()
                        .hasMessageContaining("evidence-contract-list-expression-skill.yaml")
                        .hasMessageContaining("field 'output_schema.properties.vendorName.evidence'"));
    }

    @Test
    void rejectsWrongCaseDirectChildWithSuggestion() {
        contextRunner
                .withPropertyValues("loomspan.skills.locations=classpath:/skills/invalid/evidence-contract-wrong-case-child-skill.yaml")
                .run(context -> assertThat(context.getStartupFailure())
                        .isNotNull()
                        .hasMessageContaining("field 'output_schema.properties.vendorName.evidence'")
                        .hasMessageContaining("column 1")
                        .hasMessageContaining("did you mean 'invoiceParser'?"));
    }

    @Test
    void failsStartupWhenOutputSchemaMaxRetriesIsPresentWithoutSchema() {
        contextRunner
                .withPropertyValues("loomspan.skills.locations=classpath:/skills/invalid/output-schema-max-retries-without-schema-skill.yaml")
                .run(context -> assertThat(context.getStartupFailure())
                        .isNotNull()
                        .hasMessageContaining("output-schema-max-retries-without-schema-skill.yaml")
                        .hasMessageContaining("field 'output_schema_max_retries'")
                        .hasMessageContaining("may only be configured when output_schema is present"));
    }

    @Test
    void failsStartupWhenOutputSchemaUsesUnsupportedKeyword() {
        contextRunner
                .withPropertyValues("loomspan.skills.locations=classpath:/skills/invalid/output-schema-unsupported-keyword-skill.yaml")
                .run(context -> assertThat(context.getStartupFailure())
                        .isNotNull()
                        .hasMessageContaining("output-schema-unsupported-keyword-skill.yaml")
                        .hasMessageContaining("field 'output_schema.properties.vendorName.oneOf'")
                        .hasMessageContaining("unknown field"));
    }

    @Test
    void failsStartupWhenOutputSchemaUsesEnumOnObjectSchema() {
        contextRunner
                .withPropertyValues("loomspan.skills.locations=classpath:/skills/invalid/output-schema-object-enum-skill.yaml")
                .run(context -> assertThat(context.getStartupFailure())
                        .isNotNull()
                        .hasMessageContaining("output-schema-object-enum-skill.yaml")
                        .hasMessageContaining("field 'output_schema.enum'")
                        .hasMessageContaining("is only supported for string schemas in the MVP"));
    }

    @Test
    void failsStartupWhenRootSchemaIsNotObject() {
        contextRunner
                .withPropertyValues("loomspan.skills.locations=classpath:/skills/invalid/output-schema-root-array-skill.yaml")
                .run(context -> assertThat(context.getStartupFailure())
                        .isNotNull()
                        .hasMessageContaining("output-schema-root-array-skill.yaml")
                        .hasMessageContaining("field 'output_schema.type'")
                        .hasMessageContaining("root output_schema type must be 'object'"));
    }

    @Test
    void failsStartupWhenRequiredFieldIsMissingFromProperties() {
        contextRunner
                .withPropertyValues("loomspan.skills.locations=classpath:/skills/invalid/output-schema-missing-required-property-skill.yaml")
                .run(context -> assertThat(context.getStartupFailure())
                        .isNotNull()
                        .hasMessageContaining("output-schema-missing-required-property-skill.yaml")
                        .hasMessageContaining("field 'output_schema.required'")
                        .hasMessageContaining("references unknown property 'vendorName'"));
    }

    @Test
    void failsStartupWhenPropertiesDifferOnlyByCase() {
        contextRunner
                .withPropertyValues("loomspan.skills.locations=classpath:/skills/invalid/output-schema-duplicate-properties-skill.yaml")
                .run(context -> assertThat(context.getStartupFailure())
                        .isNotNull()
                        .hasMessageContaining("output-schema-duplicate-properties-skill.yaml")
                        .hasMessageContaining("field 'output_schema.properties.VendorName'")
                        .hasMessageContaining("duplicates property 'vendorName'"));
    }

    @Test
    void defaultsAdditionalPropertiesToFalseWhenOmitted() {
        contextRunner
                .withPropertyValues("loomspan.skills.locations=classpath:/skills/valid/output-schema-skill.yaml")
                .run(context -> {
                    YamlSkillCatalog catalog = context.getBean(YamlSkillCatalog.class);

                    assertThat(catalog.getSkill("outputSchemaSkill").manifest().getOutputSchema().getAdditionalProperties())
                            .isFalse();
                });
    }

    @Test
    void logsWarningForComplexButSupportedSchema(CapturedOutput output) {
        contextRunner
                .withPropertyValues("loomspan.skills.locations=classpath:/skills/valid/output-schema-complex-skill.yaml")
                .run(context -> {
                    assertThat(context).hasNotFailed();
                    assertThat(context.getBean(YamlSkillCatalog.class).getSkill("outputSchemaComplexSkill")).isNotNull();
                    assertThat(output.getOut())
                            .contains("output_schema")
                            .contains("recommended");
                });
    }

    @Test
    void failsStartupWhenLinterTypeIsMissing() {
        contextRunner
                .withPropertyValues("loomspan.skills.locations=classpath:/skills/invalid/missing-linter-type-skill.yaml")
                .run(context -> {
                    assertThat(context.getStartupFailure())
                            .isNotNull()
                            .hasMessageContaining("missing-linter-type-skill.yaml")
                            .hasMessageContaining("field 'linter.type'")
                            .hasMessageContaining("required field is missing or blank");
                });
    }

    @Test
    void failsStartupWhenLinterTypeIsUnsupported() {
        contextRunner
                .withPropertyValues("loomspan.skills.locations=classpath:/skills/invalid/unsupported-linter-type-skill.yaml")
                .run(context -> {
                    assertThat(context.getStartupFailure())
                            .isNotNull()
                            .hasMessageContaining("unsupported-linter-type-skill.yaml")
                            .hasMessageContaining("field 'linter.type'")
                            .hasMessageContaining("unsupported linter type 'external'");
                });
    }

    @Test
    void failsStartupWhenRegexBlockIsMissingForRegexType() {
        contextRunner
                .withPropertyValues("loomspan.skills.locations=classpath:/skills/invalid/missing-regex-block-skill.yaml")
                .run(context -> {
                    assertThat(context.getStartupFailure())
                            .isNotNull()
                            .hasMessageContaining("missing-regex-block-skill.yaml")
                            .hasMessageContaining("field 'linter.regex'")
                            .hasMessageContaining("required block is missing");
                });
    }

    @Test
    void failsStartupWhenRegexPatternIsMissingOrBlank() {
        contextRunner
                .withPropertyValues("loomspan.skills.locations=classpath:/skills/invalid/missing-regex-pattern-skill.yaml")
                .run(context -> assertThat(context.getStartupFailure())
                        .isNotNull()
                        .hasMessageContaining("missing-regex-pattern-skill.yaml")
                        .hasMessageContaining("field 'linter.regex.pattern'")
                        .hasMessageContaining("required field is missing or blank"));

        contextRunner
                .withPropertyValues("loomspan.skills.locations=classpath:/skills/invalid/blank-regex-pattern-skill.yaml")
                .run(context -> assertThat(context.getStartupFailure())
                        .isNotNull()
                        .hasMessageContaining("blank-regex-pattern-skill.yaml")
                        .hasMessageContaining("field 'linter.regex.pattern'")
                        .hasMessageContaining("required field is missing or blank"));
    }

    @Test
    void failsStartupWhenRegexPatternIsInvalid() {
        contextRunner
                .withPropertyValues("loomspan.skills.locations=classpath:/skills/invalid/invalid-regex-linter-skill.yaml")
                .run(context -> {
                    assertThat(context.getStartupFailure())
                            .isNotNull()
                            .hasMessageContaining("invalid-regex-linter-skill.yaml")
                            .hasMessageContaining("field 'linter.regex.pattern'")
                            .hasMessageContaining("invalid regex pattern");
                });
    }

    @Test
    void failsStartupWhenLinterMaxRetriesIsMissing() {
        contextRunner
                .withPropertyValues("loomspan.skills.locations=classpath:/skills/invalid/missing-linter-max-retries-skill.yaml")
                .run(context -> {
                    assertThat(context.getStartupFailure())
                            .isNotNull()
                            .hasMessageContaining("missing-linter-max-retries-skill.yaml")
                            .hasMessageContaining("field 'linter.max_retries'")
                            .hasMessageContaining("required field is missing");
                });
    }

    @Test
    void failsStartupWhenLinterMaxRetriesIsOutOfRange() {
        contextRunner
                .withPropertyValues("loomspan.skills.locations=classpath:/skills/invalid/negative-linter-max-retries-skill.yaml")
                .run(context -> assertThat(context.getStartupFailure())
                        .isNotNull()
                        .hasMessageContaining("negative-linter-max-retries-skill.yaml")
                        .hasMessageContaining("field 'linter.max_retries'")
                        .hasMessageContaining("must be between 0 and 3"));

        contextRunner
                .withPropertyValues("loomspan.skills.locations=classpath:/skills/invalid/excessive-linter-max-retries-skill.yaml")
                .run(context -> assertThat(context.getStartupFailure())
                        .isNotNull()
                        .hasMessageContaining("excessive-linter-max-retries-skill.yaml")
                        .hasMessageContaining("field 'linter.max_retries'")
                        .hasMessageContaining("must be between 0 and 3"));
    }

    @Test
    void failsStartupWhenLinterMaxRetriesHasWrongType() {
        contextRunner
                .withPropertyValues("loomspan.skills.locations=classpath:/skills/invalid/wrong-type-linter-max-retries-skill.yaml")
                .run(context -> {
                    assertThat(context.getStartupFailure())
                            .isNotNull()
                            .hasMessageContaining("wrong-type-linter-max-retries-skill.yaml")
                            .hasMessageContaining("field 'linter.max_retries'")
                            .hasMessageContaining("Cannot deserialize value of type `java.lang.Integer`");
                });
    }

    @Test
    void failsStartupWhenLinterContainsUnknownFields() {
        contextRunner
                .withPropertyValues("loomspan.skills.locations=classpath:/skills/invalid/unknown-linter-field-skill.yaml")
                .run(context -> {
                    assertThat(context.getStartupFailure())
                            .isNotNull()
                            .hasMessageContaining("unknown-linter-field-skill.yaml")
                            .hasMessageContaining("field 'linter.regex.patterns'")
                            .hasMessageContaining("unknown field");
                });
    }

    @Test
    void failsStartupWhenManifestContainsUnknownRootFields() {
        contextRunner
                .withPropertyValues("loomspan.skills.locations=classpath:/skills/invalid/unknown-root-field-skill.yaml")
                .run(context -> {
                    assertThat(context.getStartupFailure())
                            .isNotNull()
                            .hasMessageContaining("unknown-root-field-skill.yaml")
                            .hasMessageContaining("field 'lintr'")
                            .hasMessageContaining("unknown field");
                });
    }

    @Test
    void failsStartupWhenMappingContainsUnknownFields() {
        contextRunner
                .withUserConfiguration(TargetBeanConfiguration.class)
                .withPropertyValues("loomspan.skills.locations=classpath:/skills/invalid/unknown-mapping-field-skill.yaml")
                .run(context -> {
                    assertThat(context.getStartupFailure())
                            .isNotNull()
                            .hasMessageContaining("unknownMappingFieldSkill")
                            .hasMessageContaining("unknown-mapping-field-skill.yaml")
                            .hasMessageContaining("field 'mapping.target_ids'")
                            .hasMessageContaining("unknown field")
                            .hasMessageContaining("mapping allows only target_id");
                });
    }

    @Configuration(proxyBeanMethods = false)
    static class TargetBeanConfiguration {

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
