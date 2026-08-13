package com.lokiscale.loomspan.internal.skill;

import com.fasterxml.jackson.annotation.JsonIgnore;
import com.fasterxml.jackson.annotation.JsonIgnoreProperties;
import com.fasterxml.jackson.annotation.JsonInclude;
import com.fasterxml.jackson.annotation.JsonProperty;
import com.fasterxml.jackson.annotation.JsonSetter;
import com.fasterxml.jackson.annotation.Nulls;
import tools.jackson.core.JsonParser;
import tools.jackson.core.JsonToken;
import tools.jackson.databind.DeserializationContext;
import tools.jackson.databind.ValueDeserializer;
import tools.jackson.databind.annotation.JsonDeserialize;
import org.springframework.util.StringUtils;

import java.io.IOException;
import java.util.Collections;
import java.util.EnumSet;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Locale;
import java.util.Map;
import java.util.Set;

@JsonIgnoreProperties(ignoreUnknown = false)
public class YamlSkillManifest
{
    enum Field
    {
        MODEL("model"),
        THINKING_LEVEL("thinking_level"),
        PROMPT("prompt"),
        INPUT_SCHEMA("input_schema"),
        OUTPUT_SCHEMA("output_schema"),
        PLANNING_MODE("planning_mode"),
        MAX_STEPS("max_steps"),
        ALLOWED_SKILLS("allowed_skills"),
        LINTER("linter"),
        OUTPUT_SCHEMA_MAX_RETRIES("output_schema_max_retries"),
        MAPPING("mapping");

        private final String yamlName;

        Field(String yamlName)
        {
            this.yamlName = yamlName;
        }

        String yamlName()
        {
            return yamlName;
        }
    }

    private static final List<Field> MAPPED_INAPPLICABLE_FIELDS = List.of(
            Field.MODEL,
            Field.THINKING_LEVEL,
            Field.PROMPT,
            Field.INPUT_SCHEMA,
            Field.OUTPUT_SCHEMA,
            Field.PLANNING_MODE,
            Field.MAX_STEPS,
            Field.ALLOWED_SKILLS,
            Field.LINTER,
            Field.OUTPUT_SCHEMA_MAX_RETRIES);

    private final EnumSet<Field> declaredFields = EnumSet.noneOf(Field.class);
    private String name;
    private String description;
    private String model;
    private String prompt;

    @JsonProperty("thinking_level")
    private String thinkingLevel;

    @JsonProperty("allowed_skills")
    private List<String> allowedSkills = List.of();

    @JsonProperty("rbac_roles")
    private List<String> rbacRoles = List.of();

    @JsonProperty("planning_mode")
    private Boolean planningMode;

    @JsonProperty("max_steps")
    private Integer maxSteps;

    private LinterManifest linter;

    @JsonProperty("output_schema")
    private OutputSchemaManifest outputSchema;

    @JsonProperty("input_schema")
    private InputSchemaManifest inputSchema;

    @JsonProperty("output_schema_max_retries")
    private Integer outputSchemaMaxRetries;

    private MappingManifest mapping = new MappingManifest();

    public String getName()
    {
        return name;
    }

    public void setName(String name)
    {
        this.name = name;
    }

    public String getDescription()
    {
        return description;
    }

    public void setDescription(String description)
    {
        this.description = description;
    }

    public String getModel()
    {
        return model;
    }

    public void setModel(String model)
    {
        declaredFields.add(Field.MODEL);
        this.model = model;
    }

    public String getPrompt()
    {
        return prompt;
    }

    public void setPrompt(String prompt)
    {
        declaredFields.add(Field.PROMPT);
        this.prompt = StringUtils.hasText(prompt) ? prompt.strip() : null;
    }

    public String getThinkingLevel()
    {
        return thinkingLevel;
    }

    public void setThinkingLevel(String thinkingLevel)
    {
        declaredFields.add(Field.THINKING_LEVEL);
        this.thinkingLevel = thinkingLevel;
    }

    public List<String> getAllowedSkills()
    {
        return allowedSkills;
    }

    public void setAllowedSkills(List<String> allowedSkills)
    {
        declaredFields.add(Field.ALLOWED_SKILLS);
        this.allowedSkills = allowedSkills == null ? List.of() : List.copyOf(allowedSkills);
    }

    public List<String> getRbacRoles()
    {
        return rbacRoles;
    }

    public void setRbacRoles(List<String> rbacRoles)
    {
        this.rbacRoles = rbacRoles == null ? List.of() : List.copyOf(rbacRoles);
    }

    public Boolean getPlanningMode()
    {
        return planningMode;
    }

    public void setPlanningMode(Boolean planningMode)
    {
        declaredFields.add(Field.PLANNING_MODE);
        this.planningMode = planningMode;
    }

    public Integer getMaxSteps()
    {
        return maxSteps;
    }

    public void setMaxSteps(Integer maxSteps)
    {
        declaredFields.add(Field.MAX_STEPS);
        this.maxSteps = maxSteps;
    }

    public LinterManifest getLinter()
    {
        return linter;
    }

    public void setLinter(LinterManifest linter)
    {
        declaredFields.add(Field.LINTER);
        this.linter = linter;
    }

    public MappingManifest getMapping()
    {
        return mapping;
    }

    public void setMapping(MappingManifest mapping)
    {
        declaredFields.add(Field.MAPPING);
        this.mapping = mapping == null ? new MappingManifest() : mapping;
    }

    public OutputSchemaManifest getOutputSchema()
    {
        return outputSchema;
    }

    public void setOutputSchema(OutputSchemaManifest outputSchema)
    {
        declaredFields.add(Field.OUTPUT_SCHEMA);
        this.outputSchema = outputSchema;
    }

    public InputSchemaManifest getInputSchema()
    {
        return inputSchema;
    }

    public void setInputSchema(InputSchemaManifest inputSchema)
    {
        declaredFields.add(Field.INPUT_SCHEMA);
        this.inputSchema = inputSchema;
    }

    public Integer getOutputSchemaMaxRetries()
    {
        return outputSchemaMaxRetries;
    }

    public void setOutputSchemaMaxRetries(Integer outputSchemaMaxRetries)
    {
        declaredFields.add(Field.OUTPUT_SCHEMA_MAX_RETRIES);
        this.outputSchemaMaxRetries = outputSchemaMaxRetries;
    }

    @JsonIgnore
    boolean isDeclared(Field field)
    {
        return declaredFields.contains(field);
    }

    @JsonIgnore
    Set<Field> declaredFields()
    {
        return Set.copyOf(declaredFields);
    }

    static List<Field> mappedInapplicableFields()
    {
        return MAPPED_INAPPLICABLE_FIELDS;
    }

    @JsonIgnore
    void restoreDeclaredFields(Set<Field> fields)
    {
        declaredFields.clear();
        declaredFields.addAll(fields);
    }

    @JsonIgnoreProperties(ignoreUnknown = false)
    public static class LinterManifest
    {
        // ENG-016 supports regex linting only; future modes can be added here as siblings.
        private String type;

        @JsonProperty("max_retries")
        private Integer maxRetries;

        private RegexManifest regex;

        public String getType()
        {
            return type;
        }

        public void setType(String type)
        {
            this.type = StringUtils.hasText(type) ? type.trim().toLowerCase(Locale.ROOT) : type;
        }

        public Integer getMaxRetries()
        {
            return maxRetries;
        }

        public void setMaxRetries(Integer maxRetries)
        {
            this.maxRetries = maxRetries;
        }

        public RegexManifest getRegex()
        {
            return regex;
        }

        public void setRegex(RegexManifest regex)
        {
            this.regex = regex;
        }
    }

    @JsonIgnoreProperties(ignoreUnknown = false)
    public static class RegexManifest
    {
        private String pattern;
        private String message;

        public String getPattern()
        {
            return pattern;
        }

        public void setPattern(String pattern)
        {
            this.pattern = StringUtils.hasText(pattern) ? pattern.trim() : pattern;
        }

        public String getMessage()
        {
            return message;
        }

        public void setMessage(String message)
        {
            this.message = StringUtils.hasText(message) ? message : null;
        }
    }

    @JsonIgnoreProperties(ignoreUnknown = false)
    public static class MappingManifest
    {
        @JsonProperty("target_id")
        private String targetId;

        public String getTargetId()
        {
            return targetId;
        }

        public void setTargetId(String targetId)
        {
            this.targetId = targetId;
        }
    }

    public static final class StrictStringScalarDeserializer extends ValueDeserializer<String>
    {
        @Override
        public String deserialize(JsonParser parser, DeserializationContext context)
        {
            if (parser.currentToken() != JsonToken.VALUE_STRING)
            {
                return (String) context.handleUnexpectedToken(String.class, parser);
            }
            return parser.getString();
        }
    }

    @JsonIgnoreProperties(ignoreUnknown = false)
    public static class InputSchemaManifest
    {
        private String type;
        private Map<String, InputSchemaManifest> properties = Map.of();
        private List<String> required = List.of();
        private Boolean additionalProperties;
        private InputSchemaManifest items;

        @JsonProperty("enum")
        private List<String> enumValues = List.of();

        private String description;
        private String format;

        @JsonProperty("media_type")
        private String mediaType;

        @JsonProperty("allowed_content_types")
        private List<String> allowedContentTypes = List.of();

        public String getType()
        {
            return type;
        }

        public void setType(String type)
        {
            this.type = StringUtils.hasText(type) ? type.trim().toLowerCase(Locale.ROOT) : type;
        }

        public Map<String, InputSchemaManifest> getProperties()
        {
            return properties;
        }

        public void setProperties(Map<String, InputSchemaManifest> properties)
        {
            if (properties == null || properties.isEmpty())
            {
                this.properties = Map.of();
                return;
            }
            this.properties = Collections.unmodifiableMap(new LinkedHashMap<>(properties));
        }

        public List<String> getRequired()
        {
            return required;
        }

        public void setRequired(List<String> required)
        {
            this.required = required == null ? List.of() : List.copyOf(required);
        }

        public Boolean getAdditionalProperties()
        {
            return additionalProperties;
        }

        public void setAdditionalProperties(Boolean additionalProperties)
        {
            this.additionalProperties = additionalProperties;
        }

        public InputSchemaManifest getItems()
        {
            return items;
        }

        public void setItems(InputSchemaManifest items)
        {
            this.items = items;
        }

        public List<String> getEnumValues()
        {
            return enumValues;
        }

        public void setEnumValues(List<String> enumValues)
        {
            this.enumValues = enumValues == null ? List.of() : List.copyOf(enumValues);
        }

        public String getDescription()
        {
            return description;
        }

        public void setDescription(String description)
        {
            this.description = StringUtils.hasText(description) ? description.trim() : null;
        }

        public String getFormat()
        {
            return format;
        }

        public void setFormat(String format)
        {
            this.format = StringUtils.hasText(format) ? format.trim() : null;
        }

        public String getMediaType()
        {
            return mediaType;
        }

        public void setMediaType(String mediaType)
        {
            this.mediaType = StringUtils.hasText(mediaType) ? mediaType.trim().toLowerCase(Locale.ROOT) : null;
        }

        public List<String> getAllowedContentTypes()
        {
            return allowedContentTypes;
        }

        public void setAllowedContentTypes(List<String> allowedContentTypes)
        {
            this.allowedContentTypes = allowedContentTypes == null ? List.of() : List.copyOf(allowedContentTypes);
        }
    }

    @JsonIgnoreProperties(ignoreUnknown = false)
    public static class OutputSchemaManifest
    {
        @JsonInclude(JsonInclude.Include.NON_NULL)
        @JsonDeserialize(using = StrictStringScalarDeserializer.class)
        @JsonSetter(nulls = Nulls.FAIL)
        private String evidence;
        private String type;
        private Map<String, OutputSchemaManifest> properties = Map.of();
        private List<String> required = List.of();
        private Boolean additionalProperties;
        private OutputSchemaManifest items;

        @JsonProperty("enum")
        private List<String> enumValues = List.of();

        private String description;
        private String format;
        private Boolean nullable;

        public String getEvidence()
        {
            return evidence;
        }

        public void setEvidence(String evidence)
        {
            this.evidence = evidence;
        }

        public String getType()
        {
            return type;
        }

        public void setType(String type)
        {
            this.type = StringUtils.hasText(type) ? type.trim().toLowerCase(Locale.ROOT) : type;
        }

        public Map<String, OutputSchemaManifest> getProperties()
        {
            return properties;
        }

        public void setProperties(Map<String, OutputSchemaManifest> properties)
        {
            if (properties == null || properties.isEmpty())
            {
                this.properties = Map.of();
                return;
            }
            this.properties = Collections.unmodifiableMap(new LinkedHashMap<>(properties));
        }

        public List<String> getRequired()
        {
            return required;
        }

        public void setRequired(List<String> required)
        {
            this.required = required == null ? List.of() : List.copyOf(required);
        }

        public Boolean getAdditionalProperties()
        {
            return additionalProperties;
        }

        public void setAdditionalProperties(Boolean additionalProperties)
        {
            this.additionalProperties = additionalProperties;
        }

        public OutputSchemaManifest getItems()
        {
            return items;
        }

        public void setItems(OutputSchemaManifest items)
        {
            this.items = items;
        }

        public List<String> getEnumValues()
        {
            return enumValues;
        }

        public void setEnumValues(List<String> enumValues)
        {
            this.enumValues = enumValues == null ? List.of() : List.copyOf(enumValues);
        }

        public String getDescription()
        {
            return description;
        }

        public void setDescription(String description)
        {
            this.description = StringUtils.hasText(description) ? description.trim() : null;
        }

        public String getFormat()
        {
            return format;
        }

        public void setFormat(String format)
        {
            this.format = StringUtils.hasText(format) ? format.trim() : null;
        }

        public Boolean getNullable()
        {
            return nullable;
        }

        public void setNullable(Boolean nullable)
        {
            this.nullable = nullable;
        }
    }

    private static Map<String, List<String>> normalizeStringListMap(Map<String, List<String>> rawMap)
    {
        if (rawMap == null || rawMap.isEmpty())
        {
            return Map.of();
        }

        LinkedHashMap<String, List<String>> normalized = new LinkedHashMap<>();

        rawMap.forEach((key, values) -> normalized.put(
                key == null ? null : key.trim(),
                normalizeStringList(values)));

                return Collections.unmodifiableMap(normalized);
    }

    private static List<String> normalizeStringList(List<String> values)
    {
        if (values == null || values.isEmpty())
        {
            return List.of();
        }

        return values.stream()
                .map(value -> value == null ? null : value.trim())
                .toList();
    }
}
