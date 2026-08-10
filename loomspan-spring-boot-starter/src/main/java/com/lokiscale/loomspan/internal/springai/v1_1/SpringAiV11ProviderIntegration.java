package com.lokiscale.loomspan.internal.springai.v1_1;

import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.lokiscale.loomspan.autoconfigure.AiDriver;
import com.lokiscale.loomspan.autoconfigure.LoomspanProperties;
import com.lokiscale.loomspan.internal.autoconfigure.SafeAiConnectionConfigurationException;
import com.lokiscale.loomspan.internal.provider.AttemptOwnership;
import com.lokiscale.loomspan.internal.provider.ProviderConnectionRuntime;
import com.lokiscale.loomspan.internal.provider.ProviderFailureCategory;
import com.lokiscale.loomspan.internal.provider.ProviderFailureClassification;
import com.lokiscale.loomspan.internal.provider.ProviderFailureDetails;
import com.lokiscale.loomspan.internal.provider.ProviderRetryPolicy;
import com.google.auth.oauth2.GoogleCredentials;
import com.google.genai.Client;
import com.google.genai.errors.ApiException;
import com.google.genai.types.HttpOptions;
import com.google.genai.types.HttpRetryOptions;
import org.springframework.ai.anthropic.AnthropicChatModel;
import org.springframework.ai.anthropic.api.AnthropicApi;
import org.springframework.ai.google.genai.GoogleGenAiChatModel;
import org.springframework.ai.ollama.OllamaChatModel;
import org.springframework.ai.ollama.api.OllamaApi;
import org.springframework.ai.openai.OpenAiChatModel;
import org.springframework.ai.openai.api.OpenAiApi;
import org.springframework.core.io.ResourceLoader;
import org.springframework.http.HttpMethod;
import org.springframework.http.client.ClientHttpRequestInterceptor;
import org.springframework.http.client.ClientHttpResponse;
import org.springframework.retry.backoff.NoBackOffPolicy;
import org.springframework.retry.policy.SimpleRetryPolicy;
import org.springframework.retry.support.RetryTemplate;
import org.springframework.util.LinkedMultiValueMap;
import org.springframework.util.MultiValueMap;
import org.springframework.util.StringUtils;
import org.springframework.web.client.DefaultResponseErrorHandler;
import org.springframework.web.client.ResourceAccessException;
import org.springframework.web.client.RestClientResponseException;

import javax.net.ssl.SSLException;
import java.io.ByteArrayInputStream;
import java.io.EOFException;
import java.io.IOException;
import java.io.InputStream;
import java.net.ConnectException;
import java.net.SocketException;
import java.net.SocketTimeoutException;
import java.net.URI;
import java.net.UnknownHostException;
import java.nio.charset.StandardCharsets;
import java.time.Duration;
import java.time.Clock;
import java.time.ZonedDateTime;
import java.time.format.DateTimeFormatter;
import java.util.IdentityHashMap;
import java.util.List;
import java.util.Map;

/** Spring AI 1.1.x integration boundary. */
public final class SpringAiV11ProviderIntegration
{
    static final int DIAGNOSTIC_LIMIT_BYTES = 1024 * 1024;
    private final ResourceLoader resourceLoader;
    private final Clock clock;
    private final ObjectMapper objectMapper = new ObjectMapper();

    public SpringAiV11ProviderIntegration(ResourceLoader resourceLoader)
    {
        this(resourceLoader, Clock.systemUTC());
    }

    SpringAiV11ProviderIntegration(ResourceLoader resourceLoader, Clock clock)
    {
        this.resourceLoader = resourceLoader;
        this.clock = clock;
    }

    public ProviderConnectionRuntime create(String connectionName, LoomspanProperties.ConnectionProperties properties)
    {
        ProviderRetryPolicy retry = ProviderRetryPolicy.from(properties.getProviderRetry());
        return new ProviderConnectionRuntime(switch (properties.getDriver())
        {
            case OPENAI -> openAi(properties);
            case ANTHROPIC -> anthropic(properties);
            case GEMINI -> gemini(connectionName, properties);
            case OLLAMA -> ollama(properties);
        }, properties.getDriver(), AttemptOwnership.EXACT_ATTEMPT_OWNERSHIP, retry, this::translate);
    }

    private OpenAiChatModel openAi(LoomspanProperties.ConnectionProperties properties)
    {
        OpenAiApi.Builder builder = OpenAiApi.builder().apiKey(properties.getApiKey());
        if (StringUtils.hasText(properties.getBaseUrl())) builder.baseUrl(properties.getBaseUrl());
        MultiValueMap<String, String> headers = new LinkedMultiValueMap<>();
        properties.getHeaders().forEach(headers::add);
        LoomspanProperties.OpenAiOptions options = properties.getOpenai();
        if (options != null)
        {
            if (StringUtils.hasText(options.getOrganizationId())) headers.add("OpenAI-Organization", options.getOrganizationId());
            if (StringUtils.hasText(options.getProjectId())) headers.add("OpenAI-Project", options.getProjectId());
            if (StringUtils.hasText(options.getChatCompletionsPath())) builder.completionsPath(options.getChatCompletionsPath());
            if (options.getCompatibilityProfile() == LoomspanProperties.OpenAiCompatibilityProfile.OPENROUTER)
            {
                ClientHttpRequestInterceptor interceptor = (request, body, execution) ->
                        inspectOpenRouter(execution.execute(request, body));
                builder.restClientBuilder(org.springframework.web.client.RestClient.builder().requestInterceptor(interceptor));
            }
        }
        if ((options == null || !StringUtils.hasText(options.getChatCompletionsPath()))
                && baseUrlAlreadyEndsWithV1(properties.getBaseUrl())) builder.completionsPath("/chat/completions");
        if (!headers.isEmpty()) builder.headers(headers);
        builder.responseErrorHandler(new CapturingResponseErrorHandler());
        return OpenAiChatModel.builder().openAiApi(builder.build()).retryTemplate(oneAttemptTemplate()).build();
    }

    private AnthropicChatModel anthropic(LoomspanProperties.ConnectionProperties properties)
    {
        AnthropicApi.Builder builder = AnthropicApi.builder().apiKey(properties.getApiKey());
        if (StringUtils.hasText(properties.getBaseUrl())) builder.baseUrl(properties.getBaseUrl());
        LoomspanProperties.AnthropicOptions options = properties.getAnthropic();
        if (options != null)
        {
            if (StringUtils.hasText(options.getCompletionsPath())) builder.completionsPath(options.getCompletionsPath());
            if (StringUtils.hasText(options.getVersion())) builder.anthropicVersion(options.getVersion());
            if (StringUtils.hasText(options.getBetaVersion())) builder.anthropicBetaFeatures(options.getBetaVersion());
        }
        builder.responseErrorHandler(new CapturingResponseErrorHandler());
        return AnthropicChatModel.builder().anthropicApi(builder.build()).retryTemplate(oneAttemptTemplate()).build();
    }

    private GoogleGenAiChatModel gemini(String connectionName, LoomspanProperties.ConnectionProperties properties)
    {
        Client.Builder builder = Client.builder().httpOptions(oneAttemptGoogleHttpOptions());
        LoomspanProperties.GeminiOptions options = properties.getGemini();
        if (StringUtils.hasText(properties.getApiKey())) builder.apiKey(properties.getApiKey());
        else
        {
            builder.vertexAI(true).project(options.getProjectId()).location(options.getLocation());
            if (StringUtils.hasText(options.getCredentialsUri())) builder.credentials(loadCredentials(connectionName, options.getCredentialsUri()));
        }
        return GoogleGenAiChatModel.builder().genAiClient(builder.build()).retryTemplate(oneAttemptTemplate()).build();
    }

    private OllamaChatModel ollama(LoomspanProperties.ConnectionProperties properties)
    {
        OllamaApi api = OllamaApi.builder().baseUrl(properties.getBaseUrl())
                .responseErrorHandler(new CapturingResponseErrorHandler()).build();
        return OllamaChatModel.builder().ollamaApi(api).retryTemplate(oneAttemptTemplate()).build();
    }

    static HttpOptions oneAttemptGoogleHttpOptions()
    {
        return HttpOptions.builder().retryOptions(HttpRetryOptions.builder().attempts(1).build()).build();
    }

    private RetryTemplate oneAttemptTemplate()
    {
        RetryTemplate template = new RetryTemplate();
        template.setRetryPolicy(new SimpleRetryPolicy(1));
        template.setBackOffPolicy(new NoBackOffPolicy());
        return template;
    }

    private ProviderFailureDetails translate(Throwable failure)
    {
        IdentityHashMap<Throwable, Boolean> seen = new IdentityHashMap<>();
        Throwable current = failure;
        for (int depth = 0; current != null && depth < 12 && seen.put(current, Boolean.TRUE) == null; depth++)
        {
            if (current instanceof ProviderCallException normalized) return normalized.details();
            if (current instanceof ApiException response)
            {
                return googleHttpFailure(response);
            }
            if (current instanceof RestClientResponseException response)
            {
                byte[] body = response.getResponseBodyAsByteArray();
                boolean truncated = body.length > DIAGNOSTIC_LIMIT_BYTES;
                if (truncated) body = java.util.Arrays.copyOf(body, DIAGNOSTIC_LIMIT_BYTES);
                return httpFailure(response.getStatusCode().value(),
                        response.getResponseHeaders() == null ? null : response.getResponseHeaders().getFirst("Retry-After"),
                        body, truncated);
            }
            if (current instanceof InterruptedException || current instanceof java.util.concurrent.CancellationException
                    || current instanceof SSLException)
            {
                return new ProviderFailureDetails(ProviderFailureClassification.PERMANENT,
                        ProviderFailureCategory.UNKNOWN, null, null, null, null, null, List.of());
            }
            if (current instanceof SocketTimeoutException)
            {
                return transientTransport(ProviderFailureCategory.TIMEOUT);
            }
            if (current instanceof ConnectException || current instanceof SocketException
                    || current instanceof EOFException || current instanceof UnknownHostException)
            {
                return transientTransport(ProviderFailureCategory.CONNECTIVITY);
            }
            current = current.getCause();
        }
        return ProviderFailureDetails.unknown();
    }

    private ProviderFailureDetails googleHttpFailure(ApiException response)
    {
        byte[] bytes;
        try
        {
            bytes = objectMapper.writeValueAsBytes(Map.of(
                    "status", response.status() == null ? "" : response.status(),
                    "message", response.message() == null ? "" : response.message()));
        }
        catch (IOException impossible)
        {
            bytes = new byte[0];
        }
        boolean truncated = bytes.length > DIAGNOSTIC_LIMIT_BYTES;
        if (truncated) bytes = java.util.Arrays.copyOf(bytes, DIAGNOSTIC_LIMIT_BYTES);
        ProviderFailureDetails base = httpFailure(response.code(), null, bytes, truncated);
        return new ProviderFailureDetails(base.classification(), base.category(), base.httpStatus(), null,
                null, response.status(), null, base.diagnostics());
    }

    private ProviderFailureDetails transientTransport(ProviderFailureCategory category)
    {
        return new ProviderFailureDetails(ProviderFailureClassification.TRANSIENT, category,
                null, null, null, null, null, List.of());
    }

    private ProviderFailureDetails httpFailure(int status, String retryAfterValue, byte[] bytes, boolean truncated)
    {
        Duration retryAfter = parseRetryAfter(retryAfterValue);
        ProviderFailureClassification classification = java.util.Set.of(408, 429, 500, 502, 503, 504).contains(status)
                ? ProviderFailureClassification.TRANSIENT : ProviderFailureClassification.PERMANENT;
        ProviderFailureCategory category = status == 429 ? ProviderFailureCategory.RATE_LIMITED
                : status == 401 ? ProviderFailureCategory.AUTHENTICATION
                : status == 403 ? ProviderFailureCategory.AUTHORIZATION
                : status == 402 ? ProviderFailureCategory.PAYMENT_REQUIRED
                : status >= 500 ? ProviderFailureCategory.SERVER_ERROR : ProviderFailureCategory.INVALID_REQUEST;
        return new ProviderFailureDetails(classification, category, status, retryAfter, null, null, null,
                List.of(diagnostic(bytes, truncated)));
    }

    private ClientHttpResponse inspectOpenRouter(ClientHttpResponse response) throws IOException
    {
        if (!response.getStatusCode().is2xxSuccessful()) return response;
        byte[] bytes = response.getBody().readNBytes(DIAGNOSTIC_LIMIT_BYTES + 1);
        if (bytes.length > DIAGNOSTIC_LIMIT_BYTES)
        {
            throw decodingFailure("OpenRouter response exceeded diagnostic capture limit", bytes, true);
        }
        try
        {
            JsonNode root = objectMapper.readTree(bytes);
            for (JsonNode choice : root.path("choices"))
            {
                if ("error".equals(choice.path("finish_reason").asText()))
                {
                    JsonNode error = choice.path("error");
                    if (!error.isObject()) throw decodingFailure("Malformed OpenRouter error completion", bytes, false);
                    String type = text(error.path("metadata").path("error_type"));
                    String code = text(error.path("code"));
                    if (code == null) code = text(error.path("provider_code"));
                    String summary = text(error.path("message"));
                    ProviderFailureCategory category = category(type);
                    ProviderFailureClassification classification = retryableOpenRouter(type)
                            ? ProviderFailureClassification.TRANSIENT : ProviderFailureClassification.PERMANENT;
                    throw new ProviderCallException("OpenRouter returned an error completion",
                            new ProviderFailureDetails(classification, category, 200, null, type, code, summary,
                                    List.of(diagnostic(bytes, false))));
                }
            }
        }
        catch (ProviderCallException ex) { throw ex; }
        catch (IOException | RuntimeException ex)
        {
            throw decodingFailure("Unable to decode OpenRouter response", bytes, false);
        }
        return new BufferedClientHttpResponse(response, bytes);
    }

    private ProviderCallException decodingFailure(String message, byte[] body, boolean truncated)
    {
        byte[] bounded = body.length <= DIAGNOSTIC_LIMIT_BYTES ? body : java.util.Arrays.copyOf(body, DIAGNOSTIC_LIMIT_BYTES);
        return new ProviderCallException(message,
                new ProviderFailureDetails(ProviderFailureClassification.PERMANENT,
                        ProviderFailureCategory.CLIENT_DECODING, 200, null, null, null, message,
                        List.of(diagnostic(bounded, truncated))));
    }

    private static Map<String, Object> diagnostic(byte[] body, boolean truncated)
    {
        return Map.of("kind", "PROVIDER_ERROR", "contentType", "application/json; charset=utf-8",
                "text", new String(body, StandardCharsets.UTF_8), "truncated", truncated,
                "captureLimitBytes", DIAGNOSTIC_LIMIT_BYTES);
    }

    private static boolean retryableOpenRouter(String type)
    {
        return type != null && java.util.Set.of("rate_limit_exceeded", "provider_overloaded",
                "provider_unavailable", "timeout", "server", "unmapped").contains(type);
    }

    private static ProviderFailureCategory category(String type)
    {
        if (type == null) return ProviderFailureCategory.UNKNOWN;
        return switch (type)
        {
            case "rate_limit_exceeded" -> ProviderFailureCategory.RATE_LIMITED;
            case "provider_overloaded" -> ProviderFailureCategory.PROVIDER_OVERLOADED;
            case "provider_unavailable", "unmapped" -> ProviderFailureCategory.PROVIDER_UNAVAILABLE;
            case "timeout" -> ProviderFailureCategory.TIMEOUT;
            case "server" -> ProviderFailureCategory.SERVER_ERROR;
            default -> ProviderFailureCategory.UNKNOWN;
        };
    }

    private static String text(JsonNode node) { return node.isTextual() && !node.asText().isBlank() ? node.asText() : null; }

    private GoogleCredentials loadCredentials(String connectionName, String uri)
    {
        try (InputStream input = resourceLoader.getResource(uri).getInputStream())
        {
            return GoogleCredentials.fromStream(input);
        }
        catch (IOException ex)
        {
            throw new SafeAiConnectionConfigurationException("loomspan.connections." + connectionName
                    + ".gemini.credentials-uri could not be loaded");
        }
    }

    private static boolean baseUrlAlreadyEndsWithV1(String baseUrl)
    {
        if (!StringUtils.hasText(baseUrl)) return false;
        String path = URI.create(baseUrl).getPath();
        return path != null && path.replaceFirst("/+$", "").endsWith("/v1");
    }

    private final class CapturingResponseErrorHandler extends DefaultResponseErrorHandler
    {
        @Override
        public void handleError(URI url, HttpMethod method, ClientHttpResponse response) throws IOException
        {
            byte[] bytes = response.getBody().readNBytes(DIAGNOSTIC_LIMIT_BYTES + 1);
            boolean truncated = bytes.length > DIAGNOSTIC_LIMIT_BYTES;
            if (truncated) bytes = java.util.Arrays.copyOf(bytes, DIAGNOSTIC_LIMIT_BYTES);
            int status = response.getStatusCode().value();
            throw new ProviderCallException("Provider returned HTTP " + status,
                    httpFailure(status, response.getHeaders().getFirst("Retry-After"), bytes, truncated));
        }
    }

    private Duration parseRetryAfter(String value)
    {
        if (!StringUtils.hasText(value)) return null;
        try
        {
            long seconds = Long.parseLong(value.trim());
            return seconds < 0 ? null : Duration.ofSeconds(seconds);
        }
        catch (RuntimeException ignored)
        {
            try
            {
                Duration delay = Duration.between(clock.instant(),
                        ZonedDateTime.parse(value, DateTimeFormatter.RFC_1123_DATE_TIME).toInstant());
                return delay.isNegative() || delay.isZero() ? null : delay;
            }
            catch (RuntimeException invalid) { return null; }
        }
    }

    private record BufferedClientHttpResponse(ClientHttpResponse delegate, byte[] bytes) implements ClientHttpResponse
    {
        @Override public org.springframework.http.HttpStatusCode getStatusCode() throws IOException { return delegate.getStatusCode(); }
        @Override public String getStatusText() throws IOException { return delegate.getStatusText(); }
        @Override public void close() { delegate.close(); }
        @Override public InputStream getBody() { return new ByteArrayInputStream(bytes); }
        @Override public org.springframework.http.HttpHeaders getHeaders() { return delegate.getHeaders(); }
    }
}
