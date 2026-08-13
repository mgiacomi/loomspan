package com.lokiscale.loomspan.internal.skill;

import tools.jackson.databind.DeserializationFeature;
import tools.jackson.databind.JsonNode;
import tools.jackson.databind.DatabindException;
import tools.jackson.databind.ObjectMapper;
import tools.jackson.databind.exc.UnrecognizedPropertyException;
import tools.jackson.dataformat.yaml.YAMLMapper;
import com.lokiscale.loomspan.autoconfigure.LoomspanProperties;
import com.lokiscale.loomspan.internal.runtime.evidence.EvidenceContract;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.beans.factory.InitializingBean;
import org.springframework.core.io.Resource;
import org.springframework.core.io.support.ResourcePatternResolver;
import org.springframework.core.io.support.PathMatchingResourcePatternResolver;
import org.springframework.util.StringUtils;

import java.io.IOException;
import java.io.InputStream;
import java.io.ByteArrayInputStream;
import java.util.ArrayList;
import java.util.Comparator;
import java.util.LinkedHashMap;
import java.util.LinkedHashSet;
import java.util.List;
import java.util.Locale;
import java.util.Map;
import java.util.Objects;
import java.util.Set;
import java.util.regex.Pattern;
import java.util.regex.PatternSyntaxException;

public class YamlSkillCatalog implements InitializingBean
{
    private static final Logger log = LoggerFactory.getLogger(YamlSkillCatalog.class);

    private static final String DEFAULT_THINKING_LEVEL = "medium";
    private static final int MAX_LINTER_RETRIES = 3;
    private static final int DEFAULT_OUTPUT_SCHEMA_RETRIES = 2;
    private static final int MAX_OUTPUT_SCHEMA_RETRIES = 3;
    private static final Set<String> SUPPORTED_SCHEMA_TYPES = Set.of("object", "array", "string", "number", "integer", "boolean");
    private static final Set<String> SUPPORTED_INPUT_SCHEMA_TYPES = Set.of("object", "array", "string", "number", "integer", "boolean", "attachment");
    private static final Set<String> SUPPORTED_ATTACHMENT_MEDIA_TYPES = Set.of("image", "pdf", "audio", "video", "file");
    private static final int OUTPUT_SCHEMA_WARNING_DEPTH = 4;
    private static final int OUTPUT_SCHEMA_WARNING_PROPERTIES = 12;
    private static final int OUTPUT_SCHEMA_WARNING_REQUIRED = 8;
    private static final String PUBLIC_SKILL_NAME_REGEX = "^[A-Za-z_][A-Za-z0-9_]{0,63}$";
    private static final Pattern PUBLIC_SKILL_NAME_PATTERN = Pattern.compile(PUBLIC_SKILL_NAME_REGEX);
    private final LoomspanProperties modelsProperties;
    private final LoomspanProperties.Skills skillProperties;
    private final ResourcePatternResolver resourcePatternResolver;
    private final ObjectMapper yamlObjectMapper;
    private final Map<String, YamlSkillDefinition> skillsByName = new LinkedHashMap<>();
    private final Map<Resource, String> diagnosticSkillNames = new LinkedHashMap<>();

    public YamlSkillCatalog(LoomspanProperties properties)
    {
        this(properties, new PathMatchingResourcePatternResolver(), defaultYamlObjectMapper());
    }

    public YamlSkillCatalog(LoomspanProperties properties, LoomspanProperties.Skills skills)
    {
        properties.setSkills(skills);
        this.modelsProperties = Objects.requireNonNull(properties, "properties must not be null");
        this.skillProperties = properties.getSkills();
        this.resourcePatternResolver = new PathMatchingResourcePatternResolver();
        this.yamlObjectMapper = defaultYamlObjectMapper();
    }

    YamlSkillCatalog(LoomspanProperties properties,
            LoomspanProperties.Skills skills,
            ResourcePatternResolver resourcePatternResolver,
            ObjectMapper yamlObjectMapper)
    {
        properties.setSkills(skills);
        this.modelsProperties = Objects.requireNonNull(properties, "properties must not be null");
        this.skillProperties = properties.getSkills();
        this.resourcePatternResolver = Objects.requireNonNull(resourcePatternResolver, "resourcePatternResolver must not be null");
        this.yamlObjectMapper = Objects.requireNonNull(yamlObjectMapper, "yamlObjectMapper must not be null");
    }

    public YamlSkillCatalog(LoomspanProperties properties,
            ResourcePatternResolver resourcePatternResolver,
            ObjectMapper yamlObjectMapper)
    {
        this.modelsProperties = Objects.requireNonNull(properties, "properties must not be null");
        this.skillProperties = properties.getSkills();
        this.resourcePatternResolver = Objects.requireNonNull(resourcePatternResolver, "resourcePatternResolver must not be null");
        this.yamlObjectMapper = Objects.requireNonNull(yamlObjectMapper, "yamlObjectMapper must not be null");
    }

    @Override
    public void afterPropertiesSet()
    {
        skillsByName.clear();
        diagnosticSkillNames.clear();
        for (DiscoveredYamlSkillResource discovered : discoverResources())
        {
            Resource resource = discovered.resource();
            YamlSkillDefinition definition = loadDefinition(discovered);
            YamlSkillDefinition previous = skillsByName.putIfAbsent(definition.manifest().getName(), definition);
            if (previous != null)
            {
                throw invalidSkill(resource, "name", "duplicate skill name '" + definition.manifest().getName() + "'");
            }
        }
    }

    /**
     * Returns the typed YAML skill definitions discovered during startup in deterministic resource order.
     */
    public List<YamlSkillDefinition> getSkills()
    {
        return List.copyOf(skillsByName.values());
    }

    /**
     * Catalog lookups are the supported access pattern for loaded YAML skills after initialization.
     */
    public YamlSkillDefinition getSkill(String name)
    {
        return skillsByName.get(name);
    }

    private List<DiscoveredYamlSkillResource> discoverResources()
    {
        List<DiscoveredYamlSkillResource> resources = new ArrayList<>();

        for (String location : skillProperties.getLocations())
        {
            try
            {
                Resource[] found = resourcePatternResolver.getResources(location);
                for (Resource resource : found)
                {
                    if (resource.exists())
                    {
                        resources.add(new DiscoveredYamlSkillResource(location, resource));
                    }
                }
            }
            catch (java.io.FileNotFoundException ex)
            {
                // Missing classpath roots simply mean there are no skills at this location.
            }
            catch (IOException ex)
            {
                throw new IllegalStateException("Failed to discover YAML skills from " + location, ex);
            }
        }

        resources.sort(Comparator.comparing(resource -> describe(resource.resource())));
        return resources;
    }

    private YamlSkillDefinition loadDefinition(DiscoveredYamlSkillResource discovered)
    {
        Resource resource = discovered.resource();
        byte[] bytes;
        try (InputStream inputStream = resource.getInputStream())
        {
            bytes = inputStream.readAllBytes();
        }
        catch (IOException ex)
        {
            throw new IllegalStateException("Failed to read YAML skill from " + describe(resource), ex);
        }
        YamlSkillManifest manifest = readManifest(resource, bytes);
        YamlSkillSource source = new YamlSkillSource(resource, discovered.locationPattern(), bytes);
        validateRequiredField(resource, "name", manifest.getName());
        validateRequiredField(resource, "description", manifest.getDescription());

        if (manifest.isDeclared(YamlSkillManifest.Field.MAPPING))
        {
            validateMappedManifest(resource, manifest);
            return new YamlSkillDefinition(resource, manifest, null, EvidenceContract.empty(), source);
        }

        if (!StringUtils.hasText(manifest.getModel()))
        {
            throw invalidNamedSkill(resource, manifest, "model", "required field is missing or blank; declare a configured model");
        }
        validateInputSchema(resource, manifest);
        validateOutputSchema(resource, manifest);
        EvidenceContract evidenceContract = compileEvidenceContract(resource, manifest);
        validateLinter(resource, manifest);

        LoomspanProperties.ModelCatalogEntry catalogEntry = resolveModelCatalogEntry(resource, manifest);
        String effectiveThinkingLevel = resolveEffectiveThinkingLevel(manifest, catalogEntry);

        if (!catalogEntry.supportsThinkingLevel(effectiveThinkingLevel))
        {
            throw invalidSkill(resource, "thinking_level",
                    "unsupported thinking_level '" + effectiveThinkingLevel + "' for model '" + manifest.getModel() + "'");
        }

        EffectiveSkillExecutionConfiguration effectiveConfiguration = new EffectiveSkillExecutionConfiguration(
                manifest.getModel(),
                catalogEntry.getConnection(),
                modelsProperties.getConnections().get(catalogEntry.getConnection()).getDriver(),
                catalogEntry.getProviderModel(),
                effectiveThinkingLevel);

        return new YamlSkillDefinition(
                resource,
                manifest,
                effectiveConfiguration,
                evidenceContract,
                source);
    }

    private void validateMappedManifest(Resource resource, YamlSkillManifest manifest)
    {
        if (!StringUtils.hasText(manifest.getMapping().getTargetId()))
        {
            throw invalidMappedSkill(resource, manifest, "mapping.target_id",
                    "mapping was declared, so target_id must be non-blank; declare a valid Java target or remove mapping for an LLM-backed skill");
        }

        for (YamlSkillManifest.Field field : YamlSkillManifest.mappedInapplicableFields())
        {
            if (manifest.isDeclared(field))
            {
                throw invalidMappedSkill(resource, manifest, field.yamlName(), mappedFieldExplanation(field));
            }
        }
    }

    private String mappedFieldExplanation(YamlSkillManifest.Field field)
    {
        return switch (field)
        {
            case MODEL, THINKING_LEVEL, PROMPT, PLANNING_MODE, MAX_STEPS ->
                    "cannot be declared because no model executes at the mapped boundary; remove the field";
            case INPUT_SCHEMA ->
                    "cannot be declared because mapped skills inherit the Java target's reflected input contract; remove the field or create a Java adapter target";
            case OUTPUT_SCHEMA ->
                    "cannot be declared because the deterministic Java target owns the returned value; remove the field or create a Java adapter target";
            case ALLOWED_SKILLS ->
                    "cannot be declared because a mapped wrapper does not perform nested model tool selection; declare the mapped child on its LLM parent instead";
            case LINTER, OUTPUT_SCHEMA_MAX_RETRIES ->
                    "cannot be declared because model-output validation and retry do not run on direct Java routing; remove the field";
            default -> throw new IllegalArgumentException("No mapped-field explanation for " + field);
        };
    }

    private LoomspanProperties.ModelCatalogEntry resolveModelCatalogEntry(Resource resource, YamlSkillManifest manifest)
    {
        LoomspanProperties.ModelCatalogEntry catalogEntry = modelsProperties.getModels().get(manifest.getModel());
        if (catalogEntry == null)
        {
            throw invalidNamedSkill(resource, manifest, "model",
                    "unknown model '" + manifest.getModel() + "'; declare a model from loomspan.models");
        }
        return catalogEntry;
    }

    private String resolveEffectiveThinkingLevel(YamlSkillManifest manifest,
            LoomspanProperties.ModelCatalogEntry catalogEntry)
    {
        if (StringUtils.hasText(manifest.getThinkingLevel()))
        {
            return manifest.getThinkingLevel();
        }
        return catalogEntry.supportsThinking() ? DEFAULT_THINKING_LEVEL : null;
    }

    private YamlSkillManifest readManifest(Resource resource, byte[] bytes)
    {
        try (InputStream inputStream = new ByteArrayInputStream(bytes))
        {
            JsonNode root = yamlObjectMapper.readTree(inputStream);
            if (root == null || !root.isObject())
            {
                throw invalidSkill(resource, "manifest", "root document must be an object");
            }

            String skillName = root.path("name").asText(null);
            String description = root.path("description").asText(null);
            if (StringUtils.hasText(skillName))
            {
                diagnosticSkillNames.put(resource, skillName);
            }
            validateRequiredField(resource, "name", skillName);
            validatePublicSkillName(resource, skillName);
            validateRequiredField(resource, "description", description);

            boolean mappingDeclared = root.has("mapping");
            if (mappingDeclared)
            {
                validateRawMappedManifest(resource, root, skillName);
            }

            try
            {
                return yamlObjectMapper.treeToValue(root, YamlSkillManifest.class);
            }
            catch (UnrecognizedPropertyException ex)
            {
                if (mappingDeclared)
                {
                    throw invalidNamedSkill(resource, skillName, toFieldPath(ex),
                            "unknown field; remove it because mapped wrappers allow only name, description, rbac_roles, and mapping.target_id");
                }
                throw invalidSkill(resource, toFieldPath(ex), "unknown field");
            }
            catch (DatabindException ex)
            {
                if (mappingDeclared)
                {
                    throw invalidNamedSkill(resource, skillName, toFieldPath(ex),
                            describeMappingFailure(ex) + "; correct the field value for the mapped wrapper");
                }
                throw invalidSkill(resource, toFieldPath(ex), describeMappingFailure(ex));
            }
        }
        catch (DatabindException ex)
        {
            throw invalidSkill(resource, toFieldPath(ex), describeMappingFailure(ex));
        }
        catch (IOException ex)
        {
            throw new IllegalStateException("Failed to read YAML skill from " + describe(resource), ex);
        }
    }

    private void validateRawMappedManifest(Resource resource, JsonNode root, String skillName)
    {
        JsonNode mapping = root.get("mapping");
        JsonNode targetIdNode = mapping != null && mapping.isObject()
                ? mapping.path("target_id")
                : null;
        String targetId = targetIdNode != null && targetIdNode.isTextual()
                ? targetIdNode.textValue()
                : null;
        if (!StringUtils.hasText(targetId))
        {
            throw invalidNamedSkill(resource, skillName, "mapping.target_id",
                    "mapping was declared, so target_id must be non-blank; declare a valid Java target or remove mapping for an LLM-backed skill");
        }

        for (YamlSkillManifest.Field field : YamlSkillManifest.mappedInapplicableFields())
        {
            if (root.has(field.yamlName()))
            {
                throw invalidNamedSkill(resource, skillName, field.yamlName(), mappedFieldExplanation(field));
            }
        }

        mapping.propertyNames().forEach(fieldName ->
        {
            if (!"target_id".equals(fieldName))
            {
                throw invalidNamedSkill(resource, skillName, "mapping." + fieldName,
                        "unknown field; remove it because mapping allows only target_id");
            }
        });
    }

    static List<YamlSkillManifest.Field> mappedInapplicableFields()
    {
        return YamlSkillManifest.mappedInapplicableFields();
    }

    private void validateRequiredField(Resource resource, String fieldName, String value)
    {
        if (!StringUtils.hasText(value))
        {
            throw invalidSkill(resource, fieldName, "required field is missing or blank");
        }
    }

    private void validatePublicSkillName(Resource resource, String skillName)
    {
        if (!PUBLIC_SKILL_NAME_PATTERN.matcher(skillName).matches())
        {
            throw invalidNamedSkill(resource, skillName, "name",
                    "invalid public skill name '" + skillName + "'. Names must match " + PUBLIC_SKILL_NAME_REGEX
                            + " (1-64 characters; start with a letter or underscore; then use only letters, digits, or underscores). "
                            + "Example: mappedMethodSkill.");
        }
    }

    private void validateLinter(Resource resource, YamlSkillManifest manifest)
    {
        YamlSkillManifest.LinterManifest linter = manifest.getLinter();
        if (linter == null)
        {
            return;
        }

        validateRequiredField(resource, "linter.type", linter.getType());

        Integer maxRetries = linter.getMaxRetries();
        if (maxRetries == null)
        {
            throw invalidSkill(resource, "linter.max_retries", "required field is missing");
        }
        if (maxRetries < 0 || maxRetries > MAX_LINTER_RETRIES)
        {
            throw invalidSkill(resource, "linter.max_retries",
                    "must be between 0 and " + MAX_LINTER_RETRIES);
        }

        if (!"regex".equals(linter.getType()))
        {
            throw invalidSkill(resource, "linter.type", "unsupported linter type '" + linter.getType() + "'");
        }

        validateRegexLinter(resource, linter.getRegex());
    }

    private void validateOutputSchema(Resource resource, YamlSkillManifest manifest)
    {
        YamlSkillManifest.OutputSchemaManifest outputSchema = manifest.getOutputSchema();
        Integer maxRetries = manifest.getOutputSchemaMaxRetries();

        if (outputSchema == null)
        {
            if (maxRetries != null)
            {
                throw invalidSkill(resource, "output_schema_max_retries",
                        "may only be configured when output_schema is present");
            }
            return;
        }

        if (maxRetries == null)
        {
            manifest.setOutputSchemaMaxRetries(DEFAULT_OUTPUT_SCHEMA_RETRIES);
            maxRetries = DEFAULT_OUTPUT_SCHEMA_RETRIES;
        }
        if (maxRetries < 0 || maxRetries > MAX_OUTPUT_SCHEMA_RETRIES)
        {
            throw invalidSkill(resource, "output_schema_max_retries",
                    "must be between 0 and " + MAX_OUTPUT_SCHEMA_RETRIES);
        }

        validateSchemaNode(resource, outputSchema, "output_schema", SchemaPlacement.ROOT, 1);
    }

    private void validateInputSchema(Resource resource, YamlSkillManifest manifest)
    {
        YamlSkillManifest.InputSchemaManifest inputSchema = manifest.getInputSchema();
        if (inputSchema == null)
        {
            return;
        }
        validateInputSchemaNode(resource, inputSchema, "input_schema", true, 1);
    }

    private EvidenceContract compileEvidenceContract(Resource resource, YamlSkillManifest manifest)
    {
        if (manifest.getOutputSchema() == null)
        {
            return EvidenceContract.empty();
        }
        Map<String, com.lokiscale.loomspan.internal.runtime.evidence.EvidenceExpression> compiled = new LinkedHashMap<>();
        com.lokiscale.loomspan.internal.runtime.evidence.EvidenceExpressionParser parser =
                new com.lokiscale.loomspan.internal.runtime.evidence.EvidenceExpressionParser();
        Set<String> directChildren = new LinkedHashSet<>(manifest.getAllowedSkills());

        for (Map.Entry<String, YamlSkillManifest.OutputSchemaManifest> entry
                : manifest.getOutputSchema().getProperties().entrySet())
        {
            String claimName = entry.getKey();
            String evidence = entry.getValue().getEvidence();
            if (evidence == null)
            {
                continue;
            }
            String fieldPath = "output_schema.properties." + claimName + ".evidence";
            if (!StringUtils.hasText(evidence))
            {
                throw invalidSkill(resource, fieldPath,
                        "expression must be a nonblank YAML string");
            }

            com.lokiscale.loomspan.internal.runtime.evidence.EvidenceExpression expression;
            try
            {
                expression = parser.parse(evidence);
            }
            catch (com.lokiscale.loomspan.internal.runtime.evidence.EvidenceExpressionParser.ParseException ex)
            {
                throw invalidSkill(resource, fieldPath,
                        "invalid evidence expression at column " + ex.column() + ": " + ex.getMessage());
            }

            for (String reference : expression.referencedSkills())
            {
                if (directChildren.contains(reference))
                {
                    continue;
                }
                List<String> caseMatches = directChildren.stream()
                        .filter(child -> child.equalsIgnoreCase(reference))
                        .toList();
                String suggestion = caseMatches.size() == 1 ? "; did you mean '" + caseMatches.getFirst() + "'?" : "";
                throw invalidSkill(resource, fieldPath,
                        "invalid evidence expression at column " + referenceColumn(expression, reference)
                                + ": skill '" + reference
                                + "' is not a direct allowed child of '" + manifest.getName() + "'" + suggestion);
            }
            compiled.put(claimName, expression);
        }

        Map<String, String> normalizedClaims = new LinkedHashMap<>();
        compiled.keySet().forEach(claim -> normalizedClaims.put(claim.toLowerCase(Locale.ROOT), claim));
        return EvidenceContract.compiled(compiled, normalizedClaims);
    }

    private int referenceColumn(com.lokiscale.loomspan.internal.runtime.evidence.EvidenceExpression expression,
            String reference)
    {
        if (expression instanceof com.lokiscale.loomspan.internal.runtime.evidence.EvidenceExpression.Skill skill)
        {
            return skill.name().equals(reference) ? skill.column() : 1;
        }
        List<com.lokiscale.loomspan.internal.runtime.evidence.EvidenceExpression> children =
                expression instanceof com.lokiscale.loomspan.internal.runtime.evidence.EvidenceExpression.AllOf allOf
                        ? allOf.expressions()
                        : ((com.lokiscale.loomspan.internal.runtime.evidence.EvidenceExpression.AnyOf) expression).expressions();
        return children.stream()
                .filter(child -> child.referencedSkills().contains(reference))
                .findFirst()
                .map(child -> referenceColumn(child, reference))
                .orElse(1);
    }

    private void validateSchemaNode(Resource resource,
            YamlSkillManifest.OutputSchemaManifest schema,
            String fieldPath,
            SchemaPlacement placement,
            int depth)
    {
        if (schema == null)
        {
            throw invalidSkill(resource, fieldPath, "required block is missing");
        }

        validateRequiredField(resource, fieldPath + ".type", schema.getType());

        if (!SUPPORTED_SCHEMA_TYPES.contains(schema.getType()))
        {
            throw invalidSkill(resource, fieldPath + ".type", "unsupported schema type '" + schema.getType() + "'");
        }
        if (!schema.getEnumValues().isEmpty() && !"string".equals(schema.getType()))
        {
            throw invalidSkill(resource, fieldPath + ".enum", "is only supported for string schemas in the MVP");
        }
        if (schema.getEvidence() != null && placement != SchemaPlacement.IMMEDIATE_ROOT_PROPERTY)
        {
            throw invalidSkill(resource, fieldPath + ".evidence",
                    "evidence is currently supported only on immediate root output properties");
        }
        if (placement == SchemaPlacement.ROOT && !"object".equals(schema.getType()))
        {
            throw invalidSkill(resource, fieldPath + ".type", "root " + fieldPath + " type must be 'object'");
        }

        warnOnSchemaComplexity(resource, fieldPath, schema, depth);

        switch (schema.getType())
        {
            case "object" -> validateObjectSchema(resource, schema, fieldPath, placement, depth);
            case "array" -> validateArraySchema(resource, schema, fieldPath, depth);
            default -> validateScalarSchema(resource, schema, fieldPath);
        }
    }

    private void validateInputSchemaNode(Resource resource,
            YamlSkillManifest.InputSchemaManifest schema,
            String fieldPath,
            boolean root,
            int depth)
    {
        if (schema == null)
        {
            throw invalidSkill(resource, fieldPath, "required block is missing");
        }

        validateRequiredField(resource, fieldPath + ".type", schema.getType());

        if (!SUPPORTED_INPUT_SCHEMA_TYPES.contains(schema.getType()))
        {
            throw invalidSkill(resource, fieldPath + ".type", "unsupported schema type '" + schema.getType() + "'");
        }
        if (root && !"object".equals(schema.getType()))
        {
            throw invalidSkill(resource, fieldPath + ".type", "root " + fieldPath + " type must be 'object'");
        }
        if ("attachment".equals(schema.getType()))
        {
            validateAttachmentInputSchema(resource, schema, fieldPath);
            return;
        }
        if (StringUtils.hasText(schema.getMediaType()))
        {
            throw invalidSkill(resource, fieldPath + ".media_type", "is only supported for attachment schemas");
        }
        if (!schema.getAllowedContentTypes().isEmpty())
        {
            throw invalidSkill(resource, fieldPath + ".allowed_content_types", "is only supported for attachment schemas");
        }
        if (!schema.getEnumValues().isEmpty() && !"string".equals(schema.getType()))
        {
            throw invalidSkill(resource, fieldPath + ".enum", "is only supported for string schemas in the MVP");
        }

        switch (schema.getType())
        {
            case "object" -> validateInputObjectSchema(resource, schema, fieldPath, depth);
            case "array" -> validateInputArraySchema(resource, schema, fieldPath, depth);
            default -> validateInputScalarSchema(resource, schema, fieldPath);
        }
    }

    private void validateAttachmentInputSchema(Resource resource,
            YamlSkillManifest.InputSchemaManifest schema,
            String fieldPath)
    {
        if (!schema.getProperties().isEmpty())
        {
            throw invalidSkill(resource, fieldPath + ".properties", "is not supported for attachment schemas");
        }
        if (!schema.getRequired().isEmpty())
        {
            throw invalidSkill(resource, fieldPath + ".required", "is not supported for attachment schemas");
        }
        if (schema.getAdditionalProperties() != null)
        {
            throw invalidSkill(resource, fieldPath + ".additionalProperties", "is not supported for attachment schemas");
        }
        if (schema.getItems() != null)
        {
            throw invalidSkill(resource, fieldPath + ".items", "is not supported for attachment schemas");
        }
        if (!schema.getEnumValues().isEmpty())
        {
            throw invalidSkill(resource, fieldPath + ".enum", "is not supported for attachment schemas");
        }
        if (!StringUtils.hasText(schema.getMediaType()))
        {
            throw invalidSkill(resource, fieldPath + ".media_type", "required field is missing or blank");
        }
        if (!SUPPORTED_ATTACHMENT_MEDIA_TYPES.contains(schema.getMediaType()))
        {
            throw invalidSkill(resource, fieldPath + ".media_type", "unsupported attachment media_type '" + schema.getMediaType() + "'");
        }
        if (schema.getAllowedContentTypes().isEmpty())
        {
            throw invalidSkill(resource, fieldPath + ".allowed_content_types", "must declare at least one content type");
        }
        for (String contentType : schema.getAllowedContentTypes())
        {
            if (!StringUtils.hasText(contentType))
            {
                throw invalidSkill(resource, fieldPath + ".allowed_content_types", "content types must not be blank");
            }
        }
    }

    private void validateInputObjectSchema(Resource resource,
            YamlSkillManifest.InputSchemaManifest schema,
            String fieldPath,
            int depth)
    {
        if (schema.getAdditionalProperties() == null)
        {
            schema.setAdditionalProperties(Boolean.FALSE);
        }
        if (schema.getItems() != null)
        {
            throw invalidSkill(resource, fieldPath + ".items", "is only supported for array schemas");
        }

        Map<String, YamlSkillManifest.InputSchemaManifest> properties = schema.getProperties();
        Map<String, String> canonicalByLowercase = new LinkedHashMap<>();

        for (Map.Entry<String, YamlSkillManifest.InputSchemaManifest> entry : properties.entrySet())
        {
            String propertyName = entry.getKey();
            if (!StringUtils.hasText(propertyName))
            {
                throw invalidSkill(resource, fieldPath + ".properties", "property names must not be blank");
            }
            String normalized = propertyName.toLowerCase(Locale.ROOT);
            String previous = canonicalByLowercase.putIfAbsent(normalized, propertyName);
            if (previous != null)
            {
                throw invalidSkill(resource, fieldPath + ".properties." + propertyName,
                        "duplicates property '" + previous + "' when compared case-insensitively");
            }
            validateInputSchemaNode(resource, entry.getValue(), fieldPath + ".properties." + propertyName, false, depth + 1);
        }

        for (String requiredField : schema.getRequired())
        {
            if (!StringUtils.hasText(requiredField))
            {
                throw invalidSkill(resource, fieldPath + ".required", "required field names must not be blank");
            }
            if (!properties.containsKey(requiredField))
            {
                throw invalidSkill(resource, fieldPath + ".required",
                        "references unknown property '" + requiredField + "'");
            }
        }
    }

    private void validateInputArraySchema(Resource resource,
            YamlSkillManifest.InputSchemaManifest schema,
            String fieldPath,
            int depth)
    {
        if (!schema.getProperties().isEmpty())
        {
            throw invalidSkill(resource, fieldPath + ".properties", "is only supported for object schemas");
        }
        if (!schema.getRequired().isEmpty())
        {
            throw invalidSkill(resource, fieldPath + ".required", "is only supported for object schemas");
        }
        if (schema.getAdditionalProperties() != null)
        {
            throw invalidSkill(resource, fieldPath + ".additionalProperties", "is only supported for object schemas");
        }
        if (schema.getItems() == null)
        {
            throw invalidSkill(resource, fieldPath + ".items", "required block is missing for array schemas");
        }
        validateInputSchemaNode(resource, schema.getItems(), fieldPath + ".items", false, depth + 1);
        if ("array".equals(schema.getItems().getType()))
        {
            throw invalidSkill(resource, fieldPath + ".items.type", "nested array items are not supported");
        }
    }

    private void validateInputScalarSchema(Resource resource, YamlSkillManifest.InputSchemaManifest schema, String fieldPath)
    {
        if (!schema.getProperties().isEmpty())
        {
            throw invalidSkill(resource, fieldPath + ".properties", "is only supported for object schemas");
        }
        if (!schema.getRequired().isEmpty())
        {
            throw invalidSkill(resource, fieldPath + ".required", "is only supported for object schemas");
        }
        if (schema.getAdditionalProperties() != null)
        {
            throw invalidSkill(resource, fieldPath + ".additionalProperties", "is only supported for object schemas");
        }
        if (schema.getItems() != null)
        {
            throw invalidSkill(resource, fieldPath + ".items", "is only supported for array schemas");
        }
    }

    private void validateObjectSchema(Resource resource,
            YamlSkillManifest.OutputSchemaManifest schema,
            String fieldPath,
            SchemaPlacement placement,
            int depth)
    {
        if (schema.getAdditionalProperties() == null)
        {
            schema.setAdditionalProperties(Boolean.FALSE);
        }
        if (schema.getItems() != null)
        {
            throw invalidSkill(resource, fieldPath + ".items", "is only supported for array schemas");
        }

        Map<String, YamlSkillManifest.OutputSchemaManifest> properties = schema.getProperties();
        Map<String, String> canonicalByLowercase = new LinkedHashMap<>();

        for (Map.Entry<String, YamlSkillManifest.OutputSchemaManifest> entry : properties.entrySet())
        {
            String propertyName = entry.getKey();

            if (!StringUtils.hasText(propertyName))
            {
                throw invalidSkill(resource, fieldPath + ".properties", "property names must not be blank");
            }

            String normalized = propertyName.toLowerCase(Locale.ROOT);
            String previous = canonicalByLowercase.putIfAbsent(normalized, propertyName);

            if (previous != null)
            {
                throw invalidSkill(resource, fieldPath + ".properties." + propertyName,
                        "duplicates property '" + previous + "' when compared case-insensitively");
            }

            SchemaPlacement childPlacement = placement == SchemaPlacement.ROOT
                    ? SchemaPlacement.IMMEDIATE_ROOT_PROPERTY
                    : SchemaPlacement.NESTED;
            validateSchemaNode(resource, entry.getValue(), fieldPath + ".properties." + propertyName,
                    childPlacement, depth + 1);
        }

        for (String requiredField : schema.getRequired())
        {
            if (!StringUtils.hasText(requiredField))
            {
                throw invalidSkill(resource, fieldPath + ".required", "required field names must not be blank");
            }
            if (!properties.containsKey(requiredField))
            {
                throw invalidSkill(resource, fieldPath + ".required",
                        "references unknown property '" + requiredField + "'");
            }
        }
    }

    private void validateArraySchema(Resource resource,
            YamlSkillManifest.OutputSchemaManifest schema,
            String fieldPath,
            int depth)
    {
        if (!schema.getProperties().isEmpty())
        {
            throw invalidSkill(resource, fieldPath + ".properties", "is only supported for object schemas");
        }
        if (!schema.getRequired().isEmpty())
        {
            throw invalidSkill(resource, fieldPath + ".required", "is only supported for object schemas");
        }
        if (schema.getAdditionalProperties() != null)
        {
            throw invalidSkill(resource, fieldPath + ".additionalProperties", "is only supported for object schemas");
        }
        if (schema.getItems() == null)
        {
            throw invalidSkill(resource, fieldPath + ".items", "required block is missing for array schemas");
        }

        validateSchemaNode(resource, schema.getItems(), fieldPath + ".items", SchemaPlacement.NESTED, depth + 1);

        if ("array".equals(schema.getItems().getType()))
        {
            throw invalidSkill(resource, fieldPath + ".items.type", "nested array items are not supported");
        }
    }

    private void validateScalarSchema(Resource resource, YamlSkillManifest.OutputSchemaManifest schema, String fieldPath)
    {
        if (!schema.getProperties().isEmpty())
        {
            throw invalidSkill(resource, fieldPath + ".properties", "is only supported for object schemas");
        }
        if (!schema.getRequired().isEmpty())
        {
            throw invalidSkill(resource, fieldPath + ".required", "is only supported for object schemas");
        }
        if (schema.getAdditionalProperties() != null)
        {
            throw invalidSkill(resource, fieldPath + ".additionalProperties", "is only supported for object schemas");
        }
        if (schema.getItems() != null)
        {
            throw invalidSkill(resource, fieldPath + ".items", "is only supported for array schemas");
        }
    }

    private enum SchemaPlacement
    {
        ROOT,
        IMMEDIATE_ROOT_PROPERTY,
        NESTED
    }

    private void warnOnSchemaComplexity(Resource resource,
            String fieldPath,
            YamlSkillManifest.OutputSchemaManifest schema,
            int depth)
    {
        if (depth > OUTPUT_SCHEMA_WARNING_DEPTH)
        {
            log.warn("YAML skill '{}' output_schema at '{}' exceeds recommended nesting depth {}",
                    describe(resource), fieldPath, OUTPUT_SCHEMA_WARNING_DEPTH);
        }
        if (schema.getProperties().size() > OUTPUT_SCHEMA_WARNING_PROPERTIES)
        {
            log.warn("YAML skill '{}' output_schema at '{}' defines {} properties; recommended maximum is {}",
                    describe(resource), fieldPath, schema.getProperties().size(), OUTPUT_SCHEMA_WARNING_PROPERTIES);
        }
        if (schema.getRequired().size() > OUTPUT_SCHEMA_WARNING_REQUIRED)
        {
            log.warn("YAML skill '{}' output_schema at '{}' defines {} required fields; recommended maximum is {}",
                    describe(resource), fieldPath, schema.getRequired().size(), OUTPUT_SCHEMA_WARNING_REQUIRED);
        }
        if ("array".equals(schema.getType()) && schema.getItems() != null && "object".equals(schema.getItems().getType()))
        {
            log.warn("YAML skill '{}' output_schema at '{}' uses arrays of objects; keep item objects shallow for best model reliability",
                    describe(resource), fieldPath);
        }
    }

    private void validateRegexLinter(Resource resource, YamlSkillManifest.RegexManifest regex)
    {
        if (regex == null)
        {
            throw invalidSkill(resource, "linter.regex", "required block is missing for linter type 'regex'");
        }

        validateRequiredField(resource, "linter.regex.pattern", regex.getPattern());

        try
        {
            Pattern.compile(regex.getPattern());
        }
        catch (PatternSyntaxException ex)
        {
            throw invalidSkill(resource, "linter.regex.pattern", "invalid regex pattern: " + ex.getDescription());
        }
    }

    private IllegalStateException invalidSkill(Resource resource, String fieldName, String detail)
    {
        String skillName = diagnosticSkillNames.get(resource);
        if (StringUtils.hasText(skillName))
        {
            return invalidNamedSkill(resource, skillName, fieldName, detail);
        }
        return new IllegalStateException("Invalid YAML skill '" + describe(resource) + "' for field '" + fieldName + "': " + detail);
    }

    private IllegalStateException invalidMappedSkill(Resource resource,
            YamlSkillManifest manifest,
            String fieldName,
            String detail)
    {
        return invalidNamedSkill(resource, manifest, fieldName, detail);
    }

    private IllegalStateException invalidNamedSkill(Resource resource,
            YamlSkillManifest manifest,
            String fieldName,
            String detail)
    {
        return invalidNamedSkill(resource, manifest.getName(), fieldName, detail);
    }

    private IllegalStateException invalidNamedSkill(Resource resource,
            String skillName,
            String fieldName,
            String detail)
    {
        return new IllegalStateException("Invalid YAML skill '" + skillName + "' in '" + describe(resource)
                + "' for field '" + fieldName + "': " + detail);
    }

    private String toFieldPath(UnrecognizedPropertyException ex)
    {
        return toFieldPath((DatabindException) ex, ex.getPropertyName());
    }

    private String toFieldPath(DatabindException ex)
    {
        return toFieldPath(ex, "manifest");
    }

    private String toFieldPath(DatabindException ex, String fallbackField)
    {
        StringBuilder fieldPath = new StringBuilder();

        for (DatabindException.Reference reference : ex.getPath())
        {
            if (reference.getPropertyName() == null)
            {
                continue;
            }
            if (!fieldPath.isEmpty())
            {
                fieldPath.append('.');
            }
            fieldPath.append(reference.getPropertyName());
        }

        if (fieldPath.isEmpty())
        {
            return fallbackField;
        }

        return fieldPath.toString();
    }

    private String describeMappingFailure(DatabindException ex)
    {
        String originalMessage = ex.getOriginalMessage();
        if (!StringUtils.hasText(originalMessage))
        {
            return "invalid value";
        }
        return originalMessage;
    }

    private String describe(Resource resource)
    {
        try
        {
            return resource.getURI().toString();
        }
        catch (IOException ex)
        {
            return resource.getDescription();
        }
    }

    private static ObjectMapper defaultYamlObjectMapper()
    {
        return com.lokiscale.loomspan.internal.serialization.LoomspanJacksonCodecs.defaults().skillYaml();
    }
}
