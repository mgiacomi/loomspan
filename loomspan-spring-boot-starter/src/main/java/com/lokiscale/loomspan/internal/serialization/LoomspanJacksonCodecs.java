package com.lokiscale.loomspan.internal.serialization;

import tools.jackson.databind.DeserializationFeature;
import tools.jackson.databind.ObjectMapper;
import tools.jackson.databind.PropertyNamingStrategies;
import tools.jackson.databind.cfg.DateTimeFeature;
import tools.jackson.databind.json.JsonMapper;
import tools.jackson.dataformat.yaml.YAMLMapper;

import java.util.Objects;

public final class LoomspanJacksonCodecs
{
    private static final class Defaults
    {
        private static final LoomspanJacksonCodecs INSTANCE = new LoomspanJacksonCodecs(new ObjectMapper());
    }

    public static LoomspanJacksonCodecs defaults() { return Defaults.INSTANCE; }
    private final ObjectMapper applicationConversion;
    private final ObjectMapper skillYaml;
    private final ObjectMapper planningJson;
    private final ObjectMapper planningYaml;
    private final ObjectMapper schemaTree;
    private final ObjectMapper canonicalTrace;
    private final ObjectMapper strictObservability;

    public LoomspanJacksonCodecs(ObjectMapper applicationConversion)
    {
        this.applicationConversion = Objects.requireNonNull(applicationConversion,
                "applicationConversion must not be null");
        this.skillYaml = YAMLMapper.builder().findAndAddModules()
                .enable(DeserializationFeature.FAIL_ON_UNKNOWN_PROPERTIES).build();
        this.planningJson = JsonMapper.builder().findAndAddModules()
                .enable(DeserializationFeature.FAIL_ON_UNKNOWN_PROPERTIES).build();
        this.planningYaml = YAMLMapper.builder().findAndAddModules()
                .enable(DeserializationFeature.FAIL_ON_UNKNOWN_PROPERTIES).build();
        this.schemaTree = JsonMapper.builder().findAndAddModules().build();
        this.canonicalTrace = JsonMapper.builder().findAndAddModules()
                .enable(DateTimeFeature.WRITE_DATES_AS_TIMESTAMPS)
                .enable(DateTimeFeature.WRITE_DURATIONS_AS_TIMESTAMPS).build();
        this.strictObservability = JsonMapper.builder()
                .propertyNamingStrategy(PropertyNamingStrategies.LOWER_CAMEL_CASE)
                .enable(DeserializationFeature.FAIL_ON_UNKNOWN_PROPERTIES)
                .disable(DateTimeFeature.WRITE_DATES_AS_TIMESTAMPS)
                .disable(DateTimeFeature.WRITE_DURATIONS_AS_TIMESTAMPS)
                .build();
    }

    public ObjectMapper applicationConversion() { return applicationConversion; }
    public ObjectMapper skillYaml() { return skillYaml; }
    public ObjectMapper planningJson() { return planningJson; }
    public ObjectMapper planningYaml() { return planningYaml; }
    public ObjectMapper schemaTree() { return schemaTree; }
    public ObjectMapper canonicalTrace() { return canonicalTrace; }
    public ObjectMapper strictObservability() { return strictObservability; }
}
