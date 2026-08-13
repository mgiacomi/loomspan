package com.lokiscale.loomspan.internal.skill;

import tools.jackson.databind.ObjectMapper;
import com.lokiscale.loomspan.internal.core.PublicSkillImplementationType;
import com.lokiscale.loomspan.internal.runtime.evidence.EvidenceContract;
import org.springframework.core.io.Resource;
import org.springframework.lang.Nullable;
import org.springframework.util.StringUtils;

import java.util.List;

/**
 * Stable typed catalog entry for a YAML skill manifest and its resolved execution configuration.
 */
public record YamlSkillDefinition(
        Resource resource,
        YamlSkillManifest manifest,
        @Nullable EffectiveSkillExecutionConfiguration executionConfiguration,
        EvidenceContract evidenceContract,
        YamlSkillSource source)
{
    private static final ObjectMapper COPY_MAPPER =
            com.lokiscale.loomspan.internal.serialization.LoomspanJacksonCodecs.defaults().applicationConversion();

    public YamlSkillDefinition
    {
        manifest = copyManifest(manifest);
        evidenceContract = evidenceContract == null ? EvidenceContract.empty() : evidenceContract;
        source = source == null
                ? new YamlSkillSource(resource, resource.getDescription(), new byte[0])
                : source;
        PublicSkillImplementationType implementationType = implementationType(manifest);
        if (implementationType == PublicSkillImplementationType.MAPPED_JAVA
                && !StringUtils.hasText(manifest.getMapping().getTargetId()))
        {
            throw new IllegalArgumentException("Mapped YAML skill definitions require a non-blank mapping target");
        }
        if (implementationType == PublicSkillImplementationType.MAPPED_JAVA)
        {
            for (YamlSkillManifest.Field field : YamlSkillManifest.mappedInapplicableFields())
            {
                if (manifest.isDeclared(field))
                {
                    throw new IllegalArgumentException(
                            "Mapped YAML skill definitions must not declare field '" + field.yamlName() + "'");
                }
            }
            if (!evidenceContract.isEmpty())
            {
                throw new IllegalArgumentException("Mapped YAML skill definitions must not have an evidence contract");
            }
        }
        if (implementationType == PublicSkillImplementationType.MAPPED_JAVA && executionConfiguration != null)
        {
            throw new IllegalArgumentException("Mapped YAML skill definitions must not have an execution configuration");
        }
        if (implementationType == PublicSkillImplementationType.LLM_BACKED && executionConfiguration == null)
        {
            throw new IllegalArgumentException("LLM-backed YAML skill definitions require an execution configuration");
        }
    }

    public YamlSkillDefinition(Resource resource,
            YamlSkillManifest manifest,
            EffectiveSkillExecutionConfiguration executionConfiguration)
    {
        this(resource, manifest, executionConfiguration, EvidenceContract.empty(), null);
    }

    public YamlSkillDefinition(Resource resource,
            YamlSkillManifest manifest,
            EffectiveSkillExecutionConfiguration executionConfiguration,
            EvidenceContract evidenceContract)
    {
        this(resource, manifest, executionConfiguration, evidenceContract, null);
    }

    public List<String> allowedSkills()
    {
        return manifest.getAllowedSkills();
    }

    public List<String> rbacRoles()
    {
        return manifest.getRbacRoles();
    }

    public YamlSkillManifest.LinterManifest linter()
    {
        return copyValue(manifest.getLinter(), YamlSkillManifest.LinterManifest.class);
    }

    public YamlSkillManifest.OutputSchemaManifest outputSchema()
    {
        return copyValue(manifest.getOutputSchema(), YamlSkillManifest.OutputSchemaManifest.class);
    }

    public String prompt()
    {
        return manifest.getPrompt();
    }

    public YamlSkillManifest.InputSchemaManifest inputSchema()
    {
        return copyValue(manifest.getInputSchema(), YamlSkillManifest.InputSchemaManifest.class);
    }

    public boolean hasDeclaredInputSchema()
    {
        return manifest.getInputSchema() != null;
    }

    public boolean hasInheritedInputContract()
    {
        return !hasDeclaredInputSchema() && mappingTargetId() != null && !mappingTargetId().isBlank();
    }

    public boolean hasGenericInputContract()
    {
        return !hasDeclaredInputSchema() && !hasInheritedInputContract();
    }

    public int outputSchemaMaxRetries()
    {
        return manifest.getOutputSchemaMaxRetries() == null ? 0 : manifest.getOutputSchemaMaxRetries();
    }

    public String mappingTargetId()
    {
        String targetId = manifest.getMapping().getTargetId();
        return StringUtils.hasText(targetId) ? targetId.trim() : null;
    }

    /** Returns a defensive copy so catalog state cannot be mutated after registration. */
    public YamlSkillManifest manifest()
    {
        return copyManifest(manifest);
    }

    public PublicSkillImplementationType implementationType()
    {
        return implementationType(manifest);
    }

    public EffectiveSkillExecutionConfiguration requireExecutionConfiguration()
    {
        if (executionConfiguration == null)
        {
            throw new IllegalStateException("LLM-backed execution requires an execution configuration");
        }
        return executionConfiguration;
    }

    public boolean planningModeEnabled(boolean defaultValue)
    {
        return manifest.getPlanningMode() == null ? defaultValue : manifest.getPlanningMode();
    }

    public boolean planningModeExplicitlyEnabled()
    {
        return Boolean.TRUE.equals(manifest.getPlanningMode());
    }

    public int maxSteps(int defaultValue)
    {
        return manifest.getMaxSteps() == null ? defaultValue : manifest.getMaxSteps();
    }

    private static YamlSkillManifest copyManifest(YamlSkillManifest source)
    {
        if (source == null)
        {
            throw new NullPointerException("manifest must not be null");
        }
        YamlSkillManifest copy = COPY_MAPPER.convertValue(source, YamlSkillManifest.class);
        copy.restoreDeclaredFields(source.declaredFields());
        return copy;
    }

    private static PublicSkillImplementationType implementationType(YamlSkillManifest manifest)
    {
        return manifest.isDeclared(YamlSkillManifest.Field.MAPPING)
                ? PublicSkillImplementationType.MAPPED_JAVA
                : PublicSkillImplementationType.LLM_BACKED;
    }

    private static <T> T copyValue(T source, Class<T> type)
    {
        return source == null ? null : COPY_MAPPER.convertValue(source, type);
    }
}
