package com.lokiscale.loomspan.internal.skill;

import com.lokiscale.loomspan.autoconfigure.LoomspanAutoConfiguration;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.params.ParameterizedTest;
import org.junit.jupiter.params.provider.CsvSource;
import org.springframework.boot.autoconfigure.AutoConfigurations;
import org.springframework.boot.autoconfigure.context.ConfigurationPropertiesAutoConfiguration;
import org.springframework.boot.env.YamlPropertySourceLoader;
import org.springframework.boot.test.context.runner.ApplicationContextRunner;
import org.springframework.core.env.PropertySource;
import org.springframework.core.io.ClassPathResource;

import static org.assertj.core.api.Assertions.assertThat;

class YamlSkillEvidencePropertyCatalogTest
{
    private final ApplicationContextRunner contextRunner = new ApplicationContextRunner()
            .withConfiguration(AutoConfigurations.of(
                    ConfigurationPropertiesAutoConfiguration.class,
                    com.lokiscale.loomspan.autoconfigure.LoomspanJacksonAutoConfiguration.class,
                    LoomspanAutoConfiguration.class,
                    com.lokiscale.loomspan.autoconfigure.LoomspanAiAutoConfiguration.class))
            .withInitializer(context -> {
                try
                {
                    YamlPropertySourceLoader loader = new YamlPropertySourceLoader();
                    for (PropertySource<?> propertySource : loader.load(
                            "application-test",
                            new ClassPathResource("application-test.yml")))
                    {
                        context.getEnvironment().getPropertySources().addLast(propertySource);
                    }
                }
                catch (java.io.IOException ex)
                {
                    throw new IllegalStateException("Failed to load application-test.yml", ex);
                }
            });

    @Test
    void loadsEvidenceAnnotationFromImmediateRootOutputProperty()
    {
        contextRunner
                .withPropertyValues("loomspan.skills.locations=classpath:/skills/valid/output-schema-property-evidence-skill.yaml")
                .run(context -> {
                    assertThat(context).hasNotFailed();
                    YamlSkillDefinition definition = context.getBean(YamlSkillCatalog.class)
                            .getSkill("outputSchemaPropertyEvidenceSkill");

                    assertThat(definition.evidenceContract().claims()).containsExactly("result");
                    assertThat(definition.evidenceContract().canonicalExpressionForClaim("result"))
                            .isEqualTo("reviewSkill");
                    assertThat(definition.outputSchema().getProperties().get("result").getType())
                            .isEqualTo("string");
                    assertThat(definition.outputSchema().getRequired()).containsExactly("result");
                });
    }

    @Test
    void compilesEveryAnnotatedRootPropertyAndLeavesUnannotatedPropertiesUnconstrained()
    {
        contextRunner
                .withPropertyValues("loomspan.skills.locations=classpath:/skills/valid/output-schema-property-evidence-types-skill.yaml")
                .run(context -> {
                    assertThat(context).hasNotFailed();
                    YamlSkillDefinition definition = context.getBean(YamlSkillCatalog.class)
                            .getSkill("outputSchemaPropertyEvidenceTypesSkill");

                    assertThat(definition.evidenceContract().claims())
                            .containsExactly("ResultValue", "details", "rows");
                    assertThat(definition.evidenceContract().canonicalExpressionForClaim("resultvalue"))
                            .isEqualTo("reviewSkill");
                    assertThat(definition.evidenceContract().canonicalExpressionForClaim("unconstrained"))
                            .isNull();
                    assertThat(definition.outputSchema().getProperties().get("details").getProperties())
                            .containsKey("note");
                    assertThat(definition.outputSchema().getProperties().get("rows").getItems().getType())
                            .isEqualTo("string");
                });
    }

    @Test
    void outputSchemaWithoutAnnotationsProducesEmptyEvidenceContract()
    {
        contextRunner
                .withPropertyValues("loomspan.skills.locations=classpath:/skills/valid/output-schema-skill.yaml")
                .run(context -> assertThat(context.getBean(YamlSkillCatalog.class)
                        .getSkill("outputSchemaSkill").evidenceContract().isEmpty()).isTrue());
    }

    @ParameterizedTest
    @CsvSource({
            "output-schema-root-evidence-skill.yaml,output_schema.evidence",
            "output-schema-nested-property-evidence-skill.yaml,output_schema.properties.result.properties.detail.evidence",
            "output-schema-item-evidence-skill.yaml,output_schema.properties.results.items.evidence",
            "output-schema-item-property-evidence-skill.yaml,output_schema.properties.results.items.properties.detail.evidence"
    })
    void rejectsEvidenceOutsideImmediateRootPropertiesWithFullPath(String filename, String fieldPath)
    {
        contextRunner
                .withPropertyValues("loomspan.skills.locations=classpath:/skills/invalid/" + filename)
                .run(context -> assertThat(context.getStartupFailure())
                        .isNotNull()
                        .hasMessageContaining("field '" + fieldPath + "'")
                        .hasMessageContaining("evidence is currently supported only on immediate root output properties"));
    }
}
