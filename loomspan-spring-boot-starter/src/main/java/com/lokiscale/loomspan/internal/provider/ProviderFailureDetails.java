package com.lokiscale.loomspan.internal.provider;

import org.springframework.lang.Nullable;

import java.time.Duration;
import java.util.List;
import java.util.Map;

public record ProviderFailureDetails(
        ProviderFailureClassification classification,
        ProviderFailureCategory category,
        @Nullable Integer httpStatus,
        @Nullable Duration retryAfter,
        @Nullable String providerErrorType,
        @Nullable String providerErrorCode,
        @Nullable String summary,
        List<Map<String, Object>> diagnostics)
{
    public ProviderFailureDetails
    {
        diagnostics = diagnostics == null ? List.of() : List.copyOf(diagnostics);
    }

    public static ProviderFailureDetails unknown()
    {
        return new ProviderFailureDetails(ProviderFailureClassification.UNKNOWN,
                ProviderFailureCategory.UNKNOWN, null, null, null, null, null, List.of());
    }
}
