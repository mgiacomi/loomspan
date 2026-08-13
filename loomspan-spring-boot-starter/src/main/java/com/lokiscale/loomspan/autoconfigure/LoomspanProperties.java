package com.lokiscale.loomspan.autoconfigure;

import jakarta.validation.Valid;
import jakarta.validation.constraints.Min;
import jakarta.validation.constraints.NotBlank;
import jakarta.validation.constraints.NotNull;
import org.springframework.beans.factory.InitializingBean;
import org.springframework.boot.context.properties.ConfigurationProperties;
import org.springframework.util.StringUtils;
import org.springframework.util.unit.DataSize;
import org.springframework.validation.annotation.Validated;

import java.time.Duration;
import java.util.LinkedHashMap;
import java.util.LinkedHashSet;
import java.util.List;
import java.util.Map;
import java.util.Set;
import java.util.regex.Pattern;

/**
 * Strict application-owned Loomspan configuration. Named connections define transport and
 * credentials; model aliases select a connection and a request-scoped provider model ID.
 * Values are never inherited from {@code spring.ai.*}.
 */
@Validated
@ConfigurationProperties(prefix = "loomspan", ignoreUnknownFields = false)
public class LoomspanProperties implements InitializingBean
{
    private static final Pattern HTTP_TOKEN = Pattern.compile("^[!#$%&'*+.^_`|~0-9A-Za-z-]+$");

    @Valid
    private Session session = new Session();

    @Valid
    private Skills skills = new Skills();

    @Valid
    private Observability observability = new Observability();

    @Valid
    private Map<String, ConnectionProperties> connections = new LinkedHashMap<>();

    @Valid
    private Map<String, ModelCatalogEntry> models = new LinkedHashMap<>();

    public Session getSession()
    {
        return session;
    }

    public void setSession(Session session)
    {
        this.session = session == null ? new Session() : session;
    }

    public Skills getSkills()
    {
        return skills;
    }

    public Observability getObservability()
    {
        return observability;
    }

    public void setObservability(Observability observability)
    {
        this.observability = observability == null ? new Observability() : observability;
    }

    public void setSkills(Skills skills)
    {
        this.skills = skills == null ? new Skills() : skills;
    }

    /** Concrete endpoint/account definitions, keyed by application-owned connection name. */
    public Map<String, ConnectionProperties> getConnections()
    {
        return connections;
    }

    public void setConnections(Map<String, ConnectionProperties> connections)
    {
        this.connections = connections == null ? new LinkedHashMap<>() : new LinkedHashMap<>(connections);
    }

    /** Framework model aliases referenced by YAML skills. */
    public Map<String, ModelCatalogEntry> getModels()
    {
        return models;
    }

    public void setModels(Map<String, ModelCatalogEntry> models)
    {
        this.models = models == null ? new LinkedHashMap<>() : new LinkedHashMap<>(models);
    }

    @Override
    public void afterPropertiesSet()
    {
        validateConnections();
        validateModels();
    }

    private void validateConnections()
    {
        for (Map.Entry<String, ConnectionProperties> entry : connections.entrySet())
        {
            String name = entry.getKey();
            if (!StringUtils.hasText(name))
            {
                throw invalid("loomspan.connections", "connection names must not be blank");
            }
            ConnectionProperties connection = entry.getValue();
            String path = "loomspan.connections." + name;
            if (connection == null)
            {
                throw invalid(path, "connection definition must not be null");
            }
            AiDriver driver = connection.getDriver();
            if (driver == null)
            {
                throw invalid(path + ".driver", "is required");
            }
            validateApplicableOptions(path, connection, driver);
            validateRequiredFields(path, connection, driver);
            validateHeaders(path, connection, driver);
            validateProviderRetry(path, connection.getProviderRetry());
        }
    }

    private void validateProviderRetry(String connectionPath, ProviderRetryProperties retry)
    {
        String path = connectionPath + ".provider-retry";
        if (retry.getMaxAttempts() < 1 || retry.getMaxAttempts() > 10)
        {
            throw invalid(path + ".max-attempts", "must be between 1 and 10");
        }
        if (retry.getInitialBackoff().isNegative())
        {
            throw invalid(path + ".initial-backoff", "must not be negative");
        }
        if (retry.getMaxBackoff().isNegative())
        {
            throw invalid(path + ".max-backoff", "must not be negative");
        }
        if (retry.getMaxBackoff().compareTo(retry.getInitialBackoff()) < 0)
        {
            throw invalid(path + ".max-backoff", "must not be less than " + path + ".initial-backoff");
        }
        if (!Double.isFinite(retry.getMultiplier()) || retry.getMultiplier() < 1.0d)
        {
            throw invalid(path + ".multiplier", "must be finite and at least 1.0");
        }
        if (!Double.isFinite(retry.getJitter()) || retry.getJitter() < 0.0d || retry.getJitter() > 1.0d)
        {
            throw invalid(path + ".jitter", "must be finite and between 0.0 and 1.0");
        }
    }

    private void validateApplicableOptions(String path, ConnectionProperties connection, AiDriver driver)
    {
        if (StringUtils.hasText(connection.getBaseUrl()) && driver == AiDriver.GEMINI)
        {
            throw invalid(path + ".base-url", "is not supported for driver GEMINI");
        }
        if (StringUtils.hasText(connection.getApiKey()) && driver == AiDriver.OLLAMA)
        {
            throw invalid(path + ".api-key", "is not supported for driver OLLAMA");
        }
        if (connection.getOpenai() != null && driver != AiDriver.OPENAI)
        {
            throw invalid(path + ".openai", "is only supported for driver OPENAI");
        }
        if (connection.getGemini() != null && driver != AiDriver.GEMINI)
        {
            throw invalid(path + ".gemini", "is only supported for driver GEMINI");
        }
    }

    private void validateRequiredFields(String path, ConnectionProperties connection, AiDriver driver)
    {
        switch (driver)
        {
            case OPENAI, ANTHROPIC -> requireText(connection.getApiKey(), path + ".api-key", driver);
            case OLLAMA -> requireText(connection.getBaseUrl(), path + ".base-url", driver);
            case GEMINI -> validateGeminiMode(path, connection);
        }
    }

    private void validateGeminiMode(String path, ConnectionProperties connection)
    {
        GeminiOptions gemini = connection.getGemini();
        boolean apiKeyMode = StringUtils.hasText(connection.getApiKey());
        boolean vertexMode = gemini != null && Boolean.TRUE.equals(gemini.getVertexAi());
        if (apiKeyMode == vertexMode)
        {
            throw invalid(path, "driver GEMINI requires exactly one of api-key mode or gemini.vertex-ai=true");
        }
        if (apiKeyMode && gemini != null)
        {
            throw invalid(path + ".gemini", "is only supported when gemini.vertex-ai=true");
        }
        if (vertexMode)
        {
            requireText(gemini.getProjectId(), path + ".gemini.project-id", AiDriver.GEMINI);
            requireText(gemini.getLocation(), path + ".gemini.location", AiDriver.GEMINI);
        }
    }

    private void validateHeaders(String path, ConnectionProperties connection, AiDriver driver)
    {
        if (!connection.getHeaders().isEmpty() && driver != AiDriver.OPENAI && driver != AiDriver.ANTHROPIC)
        {
            throw invalid(path + ".headers", "is only supported for drivers OPENAI and ANTHROPIC");
        }
        for (Map.Entry<String, String> header : connection.getHeaders().entrySet())
        {
            if (!StringUtils.hasText(header.getKey()) || !HTTP_TOKEN.matcher(header.getKey()).matches())
            {
                throw invalid(path + ".headers", "contains an invalid HTTP header name");
            }
            if (header.getValue() == null)
            {
                throw invalid(path + ".headers." + header.getKey(), "value must not be null");
            }
        }
    }

    private void validateModels()
    {
        for (Map.Entry<String, ModelCatalogEntry> entry : models.entrySet())
        {
            String name = entry.getKey();
            if (!StringUtils.hasText(name))
            {
                throw invalid("loomspan.models", "model names must not be blank");
            }
            ModelCatalogEntry model = entry.getValue();
            String path = "loomspan.models." + name;
            if (model == null)
            {
                throw invalid(path, "model definition must not be null");
            }
            if (!StringUtils.hasText(model.getConnection()))
            {
                throw invalid(path + ".connection", "is required");
            }
            if (!connections.containsKey(model.getConnection()))
            {
                throw invalid(path + ".connection", "references unknown connection '" + model.getConnection() + "'");
            }
            if (!StringUtils.hasText(model.getProviderModel()))
            {
                throw invalid(path + ".provider-model", "is required");
            }
        }
    }

    private static void requireText(String value, String path, AiDriver driver)
    {
        if (!StringUtils.hasText(value))
        {
            throw invalid(path, "is required for driver " + driver);
        }
    }

    private static IllegalStateException invalid(String path, String detail)
    {
        return new IllegalStateException(path + " " + detail);
    }

    public static class Session
    {
        private static final int DEFAULT_MAX_DEPTH = 32;
        private static final Duration DEFAULT_MISSION_TIMEOUT = Duration.ofSeconds(60);
        private static final DataSize DEFAULT_MAX_ATTACHMENT_SIZE = DataSize.ofMegabytes(20);

        @Min(1)
        private int maxDepth = DEFAULT_MAX_DEPTH;

        @NotNull
        private Duration missionTimeout = DEFAULT_MISSION_TIMEOUT;

        @Valid
        private Quotas quotas = new Quotas();

        @Valid
        private Attachments attachments = new Attachments();

        public int getMaxDepth() { return maxDepth; }
        public void setMaxDepth(int maxDepth) { this.maxDepth = maxDepth; }
        public Duration getMissionTimeout() { return missionTimeout; }
        public void setMissionTimeout(Duration missionTimeout)
        {
            if (missionTimeout == null || missionTimeout.isZero() || missionTimeout.isNegative())
            {
                throw new IllegalArgumentException("missionTimeout must be greater than zero");
            }
            this.missionTimeout = missionTimeout;
        }
        public Quotas getQuotas() { return quotas; }
        public void setQuotas(Quotas quotas) { this.quotas = quotas == null ? new Quotas() : quotas; }
        public Attachments getAttachments() { return attachments; }
        public void setAttachments(Attachments attachments) { this.attachments = attachments == null ? new Attachments() : attachments; }

        public static class Attachments
        {
            @NotNull
            private DataSize maxSize = DEFAULT_MAX_ATTACHMENT_SIZE;
            public DataSize getMaxSize() { return maxSize; }
            public void setMaxSize(DataSize maxSize)
            {
                if (maxSize == null || maxSize.toBytes() < 0)
                {
                    throw new IllegalArgumentException("attachments.maxSize must be zero or greater");
                }
                this.maxSize = maxSize;
            }
        }

        public static class Quotas
        {
            @Min(0) private int maxSkillInvocations = 64;
            @Min(0) private int maxToolInvocations = 128;
            @Min(0) private int maxLinterRetries = 32;
            @Min(0) private int maxModelCalls = 64;
            @Min(0) private int maxProviderAttempts = 192;
            @Min(0) private int maxUsageUnits = 200_000;
            public int getMaxSkillInvocations() { return maxSkillInvocations; }
            public void setMaxSkillInvocations(int value) { maxSkillInvocations = value; }
            public int getMaxToolInvocations() { return maxToolInvocations; }
            public void setMaxToolInvocations(int value) { maxToolInvocations = value; }
            public int getMaxLinterRetries() { return maxLinterRetries; }
            public void setMaxLinterRetries(int value) { maxLinterRetries = value; }
            public int getMaxModelCalls() { return maxModelCalls; }
            public void setMaxModelCalls(int value) { maxModelCalls = value; }
            public int getMaxProviderAttempts() { return maxProviderAttempts; }
            public void setMaxProviderAttempts(int value) { maxProviderAttempts = value; }
            public int getMaxUsageUnits() { return maxUsageUnits; }
            public void setMaxUsageUnits(int value) { maxUsageUnits = value; }
        }
    }

    public static class Skills
    {
        private List<String> locations = List.of("classpath:/skills/**/*.yaml");
        public List<String> getLocations() { return locations; }
        public void setLocations(List<String> locations)
        {
            this.locations = locations == null || locations.isEmpty()
                    ? List.of("classpath:/skills/**/*.yaml") : List.copyOf(locations);
        }
    }

    public static class Observability
    {
        private boolean enabled;
        @Valid
        private Auth auth = new Auth();
        private Duration completionGraceTtl = Duration.ofMinutes(15);
        private Duration traceCatalogMetadataTtl = Duration.ofHours(24);

        public boolean isEnabled() { return enabled; }
        public void setEnabled(boolean enabled) { this.enabled = enabled; }
        public Auth getAuth() { return auth; }
        public void setAuth(Auth auth) { this.auth = auth == null ? new Auth() : auth; }
        public Duration getCompletionGraceTtl() { return completionGraceTtl; }
        public void setCompletionGraceTtl(Duration value) { completionGraceTtl = value; }
        public Duration getTraceCatalogMetadataTtl() { return traceCatalogMetadataTtl; }
        public void setTraceCatalogMetadataTtl(Duration value) { traceCatalogMetadataTtl = value; }

        @Override
        public String toString()
        {
            return "Observability[enabled=" + enabled + ", credentialsConfigured="
                    + StringUtils.hasText(auth.apiKey) + ", completionGraceTtl=" + completionGraceTtl
                    + ", traceCatalogMetadataTtl=" + traceCatalogMetadataTtl + "]";
        }

        public static class Auth
        {
            private String apiKey;
            public String getApiKey() { return apiKey; }
            public void setApiKey(String apiKey) { this.apiKey = apiKey; }

            @Override
            public String toString()
            {
                return "Auth[credentialsConfigured=" + StringUtils.hasText(apiKey) + "]";
            }
        }
    }

    public static class ConnectionProperties
    {
        @NotNull
        private AiDriver driver;
        private String baseUrl;
        private String apiKey;
        private Map<String, String> headers = new LinkedHashMap<>();
        @Valid private OpenAiOptions openai;
        @Valid private GeminiOptions gemini;
        @Valid private ProviderRetryProperties providerRetry = new ProviderRetryProperties();

        public AiDriver getDriver() { return driver; }
        public void setDriver(AiDriver driver) { this.driver = driver; }
        public String getBaseUrl() { return baseUrl; }
        public void setBaseUrl(String baseUrl) { this.baseUrl = baseUrl; }
        public String getApiKey() { return apiKey; }
        public void setApiKey(String apiKey) { this.apiKey = apiKey; }
        public Map<String, String> getHeaders() { return headers; }
        public void setHeaders(Map<String, String> headers)
        {
            this.headers = headers == null ? new LinkedHashMap<>() : new LinkedHashMap<>(headers);
        }
        public OpenAiOptions getOpenai() { return openai; }
        public void setOpenai(OpenAiOptions openai) { this.openai = openai; }
        public GeminiOptions getGemini() { return gemini; }
        public void setGemini(GeminiOptions gemini) { this.gemini = gemini; }
        public ProviderRetryProperties getProviderRetry() { return providerRetry; }
        public void setProviderRetry(ProviderRetryProperties retry)
        {
            providerRetry = retry == null ? new ProviderRetryProperties() : retry;
        }

        @Override
        public String toString()
        {
            return "ConnectionProperties[driver=" + driver + ", credentialsConfigured="
                    + StringUtils.hasText(apiKey) + ", headersConfigured=" + !headers.isEmpty() + "]";
        }
    }

    public static class OpenAiOptions
    {
        @NotNull private OpenAiCompatibilityProfile compatibilityProfile = OpenAiCompatibilityProfile.STANDARD;
        private String organizationId;
        private String projectId;
        public String getOrganizationId() { return organizationId; }
        public void setOrganizationId(String organizationId) { this.organizationId = organizationId; }
        public String getProjectId() { return projectId; }
        public void setProjectId(String projectId) { this.projectId = projectId; }
        public OpenAiCompatibilityProfile getCompatibilityProfile() { return compatibilityProfile; }
        public void setCompatibilityProfile(OpenAiCompatibilityProfile value)
        {
            compatibilityProfile = value == null ? OpenAiCompatibilityProfile.STANDARD : value;
        }
    }

    public enum OpenAiCompatibilityProfile
    {
        STANDARD,
        OPENROUTER
    }

    public static class ProviderRetryProperties
    {
        private boolean enabled = true;
        private int maxAttempts = 3;
        @NotNull private Duration initialBackoff = Duration.ofMillis(500);
        private double multiplier = 2.0d;
        @NotNull private Duration maxBackoff = Duration.ofSeconds(5);
        private double jitter = 0.20d;

        public boolean isEnabled() { return enabled; }
        public void setEnabled(boolean value) { enabled = value; }
        public int getMaxAttempts() { return maxAttempts; }
        public void setMaxAttempts(int value) { maxAttempts = value; }
        public Duration getInitialBackoff() { return initialBackoff; }
        public void setInitialBackoff(Duration value) { initialBackoff = value == null ? Duration.ofMillis(500) : value; }
        public double getMultiplier() { return multiplier; }
        public void setMultiplier(double value) { multiplier = value; }
        public Duration getMaxBackoff() { return maxBackoff; }
        public void setMaxBackoff(Duration value) { maxBackoff = value == null ? Duration.ofSeconds(5) : value; }
        public double getJitter() { return jitter; }
        public void setJitter(double value) { jitter = value; }
        public int effectiveMaxAttempts() { return enabled ? maxAttempts : 1; }
    }

    public static class GeminiOptions
    {
        private Boolean vertexAi;
        private String projectId;
        private String location;
        private String credentialsUri;
        public Boolean getVertexAi() { return vertexAi; }
        public void setVertexAi(Boolean value) { vertexAi = value; }
        public String getProjectId() { return projectId; }
        public void setProjectId(String value) { projectId = value; }
        public String getLocation() { return location; }
        public void setLocation(String value) { location = value; }
        public String getCredentialsUri() { return credentialsUri; }
        public void setCredentialsUri(String value) { credentialsUri = value; }
    }

    public static class ModelCatalogEntry
    {
        @NotBlank private String connection;
        @NotBlank private String providerModel;
        private Set<@NotBlank String> thinkingLevels = new LinkedHashSet<>();
        public String getConnection() { return connection; }
        public void setConnection(String connection) { this.connection = connection; }
        public String getProviderModel() { return providerModel; }
        public void setProviderModel(String providerModel) { this.providerModel = providerModel; }
        public Set<String> getThinkingLevels() { return Set.copyOf(thinkingLevels); }
        public void setThinkingLevels(Set<String> values)
        {
            thinkingLevels = values == null ? new LinkedHashSet<>() : new LinkedHashSet<>(values);
        }
        public boolean supportsThinking() { return !thinkingLevels.isEmpty(); }
        public boolean supportsThinkingLevel(String level)
        {
            return !StringUtils.hasText(level) ? !supportsThinking() : thinkingLevels.contains(level);
        }
    }
}
