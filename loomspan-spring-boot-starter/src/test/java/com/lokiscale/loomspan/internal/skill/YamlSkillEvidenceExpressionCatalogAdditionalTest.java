package com.lokiscale.loomspan.internal.skill;

import com.lokiscale.loomspan.autoconfigure.LoomspanAutoConfiguration;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.params.ParameterizedTest;
import org.junit.jupiter.params.provider.ValueSource;
import org.springframework.boot.autoconfigure.AutoConfigurations;
import org.springframework.boot.autoconfigure.context.ConfigurationPropertiesAutoConfiguration;
import org.springframework.boot.env.YamlPropertySourceLoader;
import org.springframework.boot.test.context.runner.ApplicationContextRunner;
import org.springframework.core.env.PropertySource;
import org.springframework.core.io.ClassPathResource;

import static org.assertj.core.api.Assertions.assertThat;

class YamlSkillEvidenceExpressionCatalogAdditionalTest
{
    private final ApplicationContextRunner contextRunner = new ApplicationContextRunner()
            .withConfiguration(AutoConfigurations.of(
                    ConfigurationPropertiesAutoConfiguration.class,
                    com.lokiscale.loomspan.autoconfigure.LoomspanJacksonAutoConfiguration.class,
                    LoomspanAutoConfiguration.class,
                    com.lokiscale.loomspan.autoconfigure.LoomspanAiAutoConfiguration.class))
            .withInitializer(context ->
            {
                try
                {
                    YamlPropertySourceLoader loader = new YamlPropertySourceLoader();
                    for (PropertySource<?> propertySource : loader.load(
                            "application-test", new ClassPathResource("application-test.yml")))
                    {
                        context.getEnvironment().getPropertySources().addLast(propertySource);
                    }
                }
                catch (java.io.IOException ex)
                {
                    throw new IllegalStateException("Failed to load application-test.yml", ex);
                }
            });

    @ParameterizedTest
    @ValueSource(strings = {
            "evidence-contract-list-expression-skill.yaml",
            "evidence-contract-object-expression-skill.yaml",
            "evidence-contract-number-expression-skill.yaml",
            "evidence-contract-decimal-expression-skill.yaml",
            "evidence-contract-boolean-expression-skill.yaml",
            "evidence-contract-null-expression-skill.yaml"
    })
    void rejectsEveryNonStringExpressionShapeWithoutCoercion(String filename)
    {
        contextRunner
                .withPropertyValues("loomspan.skills.locations=classpath:/skills/invalid/" + filename)
                .run(context -> assertThat(context.getStartupFailure())
                        .isNotNull()
                        .hasMessageContaining(filename)
                        .hasMessageContaining("field 'output_schema.properties.vendorName.evidence'"));
    }

    @Test
    void rejectsNondirectChildAtItsExactColumn()
    {
        contextRunner
                .withPropertyValues("loomspan.skills.locations=classpath:/skills/invalid/evidence-contract-nondirect-child-skill.yaml")
                .run(context -> assertThat(context.getStartupFailure())
                        .isNotNull()
                        .hasMessageContaining("field 'output_schema.properties.likelyCause.evidence'")
                        .hasMessageContaining("column 1")
                        .hasMessageContaining("skill 'checkDns' is not a direct allowed child"));
    }

    @Test
    void rejectsReservedOperatorAsAChildReference()
    {
        contextRunner
                .withPropertyValues("loomspan.skills.locations=classpath:/skills/invalid/evidence-contract-reserved-child-skill.yaml")
                .run(context -> assertThat(context.getStartupFailure())
                        .isNotNull()
                        .hasMessageContaining("field 'output_schema.properties.result.evidence'")
                        .hasMessageContaining("column 1")
                        .hasMessageContaining("reserved operator 'and'"));
    }

    @Test
    void omitsSuggestionWhenCaseInsensitiveReferenceIsAmbiguous()
    {
        contextRunner
                .withPropertyValues("loomspan.skills.locations=classpath:/skills/invalid/evidence-contract-ambiguous-case-child-skill.yaml")
                .run(context -> assertThat(context.getStartupFailure())
                        .isNotNull()
                        .hasMessageContaining("skill 'INVOICEPARSER' is not a direct allowed child")
                        .hasMessageNotContaining("did you mean"));
    }

    @Test
    void rejectsRemovedTopLevelEvidenceContractAsAnUnknownField()
    {
        contextRunner
                .withPropertyValues("loomspan.skills.locations=classpath:/skills/invalid/evidence-contract-missing-output-schema-skill.yaml")
                .run(context -> assertThat(context.getStartupFailure())
                        .isNotNull()
                        .hasMessageContaining("field 'evidence_contract'")
                        .hasMessageContaining("unknown field"));
    }

    @Test
    void retainsMalformedExpressionDiagnosticsAtPropertyPath()
    {
        contextRunner
                .withPropertyValues("loomspan.skills.locations=classpath:/skills/invalid/evidence-contract-malformed-expression-skill.yaml")
                .run(context -> assertThat(context.getStartupFailure())
                        .isNotNull()
                        .hasMessageContaining("field 'output_schema.properties.vendorName.evidence'")
                        .hasMessageContaining("column 18")
                        .hasMessageContaining("expected a skill name or '('"));
    }

    @Test
    void rejectsTrailingTokensAtPropertyPath()
    {
        contextRunner
                .withPropertyValues("loomspan.skills.locations=classpath:/skills/invalid/evidence-contract-trailing-token-skill.yaml")
                .run(context -> assertThat(context.getStartupFailure())
                        .isNotNull()
                        .hasMessageContaining("field 'output_schema.properties.vendorName.evidence'")
                        .hasMessageContaining("column 15")
                        .hasMessageContaining("expected 'and', 'or', or end of expression"));
    }

    @Test
    void acceptsOperatorSubstringsInExactDirectChildNames()
    {
        contextRunner
                .withPropertyValues("loomspan.skills.locations=classpath:/skills/valid/evidence-operator-substring-skill.yaml")
                .run(context -> {
                    assertThat(context).hasNotFailed();
                    assertThat(context.getBean(YamlSkillCatalog.class)
                            .getSkill("evidenceOperatorSubstringSkill")
                            .evidenceContract()
                            .canonicalExpressionForClaim("result"))
                            .isEqualTo("androidCheck and (orderLookup or candyParser)");
                });
    }
}
