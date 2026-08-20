package com.lokiscale.loomspan.internal.springai;

import tools.jackson.databind.JsonNode;
import tools.jackson.databind.ObjectMapper;
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
import com.anthropic.errors.AnthropicServiceException;
import io.micrometer.observation.ObservationRegistry;
import com.google.genai.types.HttpOptions;
import com.google.genai.types.HttpRetryOptions;
import org.springframework.ai.anthropic.AnthropicChatModel;
import org.springframework.ai.anthropic.AnthropicChatOptions;
import org.springframework.ai.google.genai.GoogleGenAiChatModel;
import org.springframework.ai.ollama.OllamaChatModel;
import org.springframework.ai.ollama.api.OllamaApi;
import org.springframework.ai.openai.OpenAiChatModel;
import org.springframework.ai.openai.OpenAiChatOptions;
import org.springframework.core.retry.RetryPolicy;
import org.springframework.core.retry.RetryTemplate;
import org.springframework.http.client.SimpleClientHttpRequestFactory;
import org.springframework.web.client.RestClient;
import org.springframework.core.io.ResourceLoader;
import org.springframework.http.HttpMethod;
import org.springframework.http.client.ClientHttpRequestInterceptor;
import org.springframework.http.client.ClientHttpResponse;
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
import java.io.InterruptedIOException;
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
import java.util.Locale;
import java.util.Map;
import java.util.concurrent.CancellationException;

/** Official Spring AI provider integration boundary. */
public final class SpringAiProviderIntegration
{
    static final int DIAGNOSTIC_LIMIT_BYTES = 1024 * 1024;
    private final ResourceLoader resourceLoader;
    private final Clock clock;
    private final ObservationRegistry observationRegistry;
    private final ObjectMapper objectMapper;

    public SpringAiProviderIntegration(ResourceLoader resourceLoader)
    {
        this(resourceLoader, ObservationRegistry.NOOP);
    }

    public SpringAiProviderIntegration(ResourceLoader resourceLoader, ObservationRegistry observationRegistry)
    {
        this(resourceLoader, Clock.systemUTC(), observationRegistry,
                com.lokiscale.loomspan.internal.serialization.LoomspanJacksonCodecs.defaults().schemaTree());
    }

    public SpringAiProviderIntegration(ResourceLoader resourceLoader, ObservationRegistry observationRegistry,
            ObjectMapper objectMapper)
    {
        this(resourceLoader, Clock.systemUTC(), observationRegistry, objectMapper);
    }

    SpringAiProviderIntegration(ResourceLoader resourceLoader, Clock clock)
    {
        this(resourceLoader, clock, ObservationRegistry.NOOP,
                com.lokiscale.loomspan.internal.serialization.LoomspanJacksonCodecs.defaults().schemaTree());
    }

    SpringAiProviderIntegration(ResourceLoader resourceLoader, Clock clock, ObservationRegistry observationRegistry)
    {
        this(resourceLoader, clock, observationRegistry,
                com.lokiscale.loomspan.internal.serialization.LoomspanJacksonCodecs.defaults().schemaTree());
    }

    SpringAiProviderIntegration(ResourceLoader resourceLoader, Clock clock, ObservationRegistry observationRegistry,
            ObjectMapper objectMapper)
    {
        this.resourceLoader = resourceLoader;
        this.clock = clock;
        this.observationRegistry = java.util.Objects.requireNonNull(observationRegistry,
                "observationRegistry must not be null");
        this.objectMapper = java.util.Objects.requireNonNull(objectMapper, "objectMapper must not be null");
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
        Map<String, String> headers = new java.util.LinkedHashMap<>(properties.getHeaders());
        OpenAiChatOptions.Builder optionsBuilder = OpenAiChatOptions.builder()
                .apiKey(properties.getApiKey())
                .maxRetries(0);
        if (StringUtils.hasText(properties.getBaseUrl())) optionsBuilder.baseUrl(properties.getBaseUrl());
        LoomspanProperties.OpenAiOptions options = properties.getOpenai();
        if (options != null)
        {
            if (StringUtils.hasText(options.getOrganizationId())) optionsBuilder.organizationId(options.getOrganizationId());
            if (StringUtils.hasText(options.getProjectId())) headers.put("OpenAI-Project", options.getProjectId());
        }
        if (!headers.isEmpty()) optionsBuilder.customHeaders(headers);
        OpenAiChatModel.Builder builder = OpenAiChatModel.builder().options(optionsBuilder.build())
                .observationRegistry(observationRegistry);
        if (options != null && options.getCompatibilityProfile() == LoomspanProperties.OpenAiCompatibilityProfile.OPENROUTER)
        {
            builder.httpClientBuilderCustomizer(http -> http.interceptor(chain -> inspectOpenRouter(chain.proceed(chain.request()))));
        }
        return builder.build();
    }

    private AnthropicChatModel anthropic(LoomspanProperties.ConnectionProperties properties)
    {
        AnthropicChatOptions.Builder optionsBuilder = AnthropicChatOptions.builder()
                .apiKey(properties.getApiKey())
                .maxRetries(0)
                .customHeaders(properties.getHeaders());
        if (StringUtils.hasText(properties.getBaseUrl())) optionsBuilder.baseUrl(properties.getBaseUrl());
        return AnthropicChatModel.builder().options(optionsBuilder.build())
                .observationRegistry(observationRegistry).build();
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
        return GoogleGenAiChatModel.builder().genAiClient(builder.build()).retryTemplate(oneAttemptTemplate())
                .observationRegistry(observationRegistry).build();
    }

    private OllamaChatModel ollama(LoomspanProperties.ConnectionProperties properties)
    {
        RestClient.Builder oneSendRestClient = RestClient.builder()
                .requestFactory(new SimpleClientHttpRequestFactory());
        OllamaApi api = OllamaApi.builder().baseUrl(properties.getBaseUrl())
                .restClientBuilder(oneSendRestClient)
                .responseErrorHandler(new CapturingResponseErrorHandler()).build();
        return OllamaChatModel.builder().ollamaApi(api).retryTemplate(oneAttemptTemplate())
                .observationRegistry(observationRegistry).build();
    }

    static HttpOptions oneAttemptGoogleHttpOptions()
    {
        return HttpOptions.builder().retryOptions(HttpRetryOptions.builder().attempts(1).build()).build();
    }

    private RetryTemplate oneAttemptTemplate()
    {
        return new RetryTemplate(RetryPolicy.withMaxRetries(0));
    }

    okhttp3.Response inspectOpenRouter(okhttp3.Response response) throws IOException
    {
        okhttp3.ResponseBody body = response.body();
        if (!response.isSuccessful() || body == null) return response;
        okhttp3.MediaType contentType = body.contentType();
        byte[] bytes;
        try (body; InputStream stream = body.byteStream())
        {
            bytes = stream.readNBytes(DIAGNOSTIC_LIMIT_BYTES + 1);
        }
        if (bytes.length > DIAGNOSTIC_LIMIT_BYTES)
        {
            throw decodingFailure("OpenRouter response exceeded diagnostic capture limit", bytes, true);
        }
        inspectOpenRouterBody(bytes);
        return response.newBuilder().body(okhttp3.ResponseBody.create(contentType, bytes)).build();
    }

    private void inspectOpenRouterBody(byte[] bytes)
    {
        JsonNode root = objectMapper.readTree(bytes);
        for (JsonNode choice : root.path("choices"))
        {
            if (!"error".equals(choice.path("finish_reason").asText())) continue;
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
            if (current instanceof AnthropicServiceException response)
            {
                byte[] body = String.valueOf(response.body()).getBytes(StandardCharsets.UTF_8);
                boolean truncated = body.length > DIAGNOSTIC_LIMIT_BYTES;
                if (truncated) body = java.util.Arrays.copyOf(body, DIAGNOSTIC_LIMIT_BYTES);
                String retryAfter = response.headers().values("Retry-After").stream().findFirst().orElse(null);
                return httpFailure(response.statusCode(), retryAfter, body, truncated);
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
            if (current instanceof InterruptedIOException && isProviderReadTimeout(failure))
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

    private boolean isProviderReadTimeout(Throwable failure)
    {
        if (Thread.currentThread().isInterrupted()) return false;
        IdentityHashMap<Throwable, Boolean> seen = new IdentityHashMap<>();
        boolean timeout = false;
        Throwable current = failure;
        for (int depth = 0; current != null && depth < 12 && seen.put(current, Boolean.TRUE) == null; depth++)
        {
            if (current instanceof CancellationException || current instanceof InterruptedException) return false;
            if (current instanceof InterruptedIOException interrupted)
            {
                String message = interrupted.getMessage();
                if (message != null)
                {
                    String normalized = message.toLowerCase(Locale.ROOT);
                    timeout |= normalized.contains("timeout") || normalized.contains("timed out");
                }
            }
            current = current.getCause();
        }
        return timeout;
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
        catch (tools.jackson.core.JacksonException impossible)
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

    ClientHttpResponse inspectOpenRouter(ClientHttpResponse response) throws IOException
    {
        if (!response.getStatusCode().is2xxSuccessful()) return response;
        byte[] bytes = response.getBody().readNBytes(DIAGNOSTIC_LIMIT_BYTES + 1);
        if (bytes.length > DIAGNOSTIC_LIMIT_BYTES)
        {
            response.close();
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
                    response.close();
                    throw new ProviderCallException("OpenRouter returned an error completion",
                            new ProviderFailureDetails(classification, category, 200, null, type, code, summary,
                                    List.of(diagnostic(bytes, false))));
                }
            }
        }
        catch (ProviderCallException ex) { throw ex; }
        catch (RuntimeException ex)
        {
            response.close();
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
