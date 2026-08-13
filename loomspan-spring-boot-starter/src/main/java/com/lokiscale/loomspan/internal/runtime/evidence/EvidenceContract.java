package com.lokiscale.loomspan.internal.runtime.evidence;

import tools.jackson.databind.JsonNode;

import java.util.Collections;
import java.util.LinkedHashMap;
import java.util.LinkedHashSet;
import java.util.Locale;
import java.util.Map;
import java.util.Set;

public final class EvidenceContract
{
    private static final EvidenceContract EMPTY = new EvidenceContract(Map.of(), Map.of());

    private final Map<String, EvidenceExpression> expressionsByClaim;
    private final Map<String, String> canonicalClaimByNormalized;

    private EvidenceContract(Map<String, EvidenceExpression> expressionsByClaim,
            Map<String, String> canonicalClaimByNormalized)
    {
        this.expressionsByClaim = Collections.unmodifiableMap(new LinkedHashMap<>(expressionsByClaim));
        this.canonicalClaimByNormalized = Collections.unmodifiableMap(new LinkedHashMap<>(canonicalClaimByNormalized));
    }

    public static EvidenceContract empty()
    {
        return EMPTY;
    }

    public static EvidenceContract compiled(Map<String, EvidenceExpression> expressionsByClaim,
            Map<String, String> canonicalClaimByNormalized)
    {
        return expressionsByClaim == null || expressionsByClaim.isEmpty()
                ? empty()
                : new EvidenceContract(expressionsByClaim, canonicalClaimByNormalized);
    }

    public boolean isEmpty()
    {
        return expressionsByClaim.isEmpty();
    }

    public Set<String> claims()
    {
        return expressionsByClaim.keySet();
    }

    public EvidenceExpression expressionForClaim(String claimName)
    {
        String canonical = canonicalClaimByNormalized.get(normalizeKey(claimName));
        return canonical == null ? null : expressionsByClaim.get(canonical);
    }

    public String canonicalExpressionForClaim(String claimName)
    {
        EvidenceExpression expression = expressionForClaim(claimName);
        return expression == null ? null : expression.canonical();
    }

    public Set<String> presentClaims(JsonNode candidate)
    {
        if (candidate == null || !candidate.isObject())
        {
            return Set.of();
        }
        LinkedHashSet<String> presentClaims = new LinkedHashSet<>();
        candidate.propertyNames().forEach(fieldName ->
        {
            String canonicalClaim = canonicalClaimByNormalized.get(normalizeKey(fieldName));
            if (canonicalClaim != null)
            {
                presentClaims.add(canonicalClaim);
            }
        });
        return Collections.unmodifiableSet(presentClaims);
    }

    public Map<String, EvidenceExpression> expressionsByClaim()
    {
        return expressionsByClaim;
    }

    private static String normalizeKey(String value)
    {
        return value == null ? "" : value.trim().toLowerCase(Locale.ROOT);
    }
}
