package com.lokiscale.loomspan.internal.runtime.evidence;

import tools.jackson.core.JacksonException;
import tools.jackson.databind.JsonNode;
import tools.jackson.databind.ObjectMapper;
import tools.jackson.databind.json.JsonMapper;

import java.util.Set;

public final class EvidenceBackedOutputValidator
{
    private final ObjectMapper objectMapper;
    private final EvidenceCoverageValidator coverageValidator;

    public EvidenceBackedOutputValidator()
    {
        this(com.lokiscale.loomspan.internal.serialization.LoomspanJacksonCodecs.defaults().schemaTree(),
                new EvidenceCoverageValidator());
    }

    public EvidenceBackedOutputValidator(ObjectMapper objectMapper,
            EvidenceCoverageValidator coverageValidator)
    {
        this.objectMapper = objectMapper;
        this.coverageValidator = coverageValidator;
    }

    public EvidenceCoverageResult validate(String rawOutput,
            EvidenceContract contract,
            Set<String> successfulSkills)
    {
        if (contract == null || contract.isEmpty())
        {
            return new EvidenceCoverageResult(Set.of(), java.util.Map.of(), successfulSkills, java.util.List.of());
        }
        try
        {
            return validate(objectMapper.readTree(rawOutput == null ? "{}" : rawOutput), contract, successfulSkills);
        }
        catch (JacksonException ex)
        {
            throw new IllegalStateException("Evidence validation expected schema-valid JSON but could not parse it.", ex);
        }
    }

    public EvidenceCoverageResult validate(JsonNode candidate,
            EvidenceContract contract,
            Set<String> successfulSkills)
    {
        if (contract == null || contract.isEmpty())
        {
            return new EvidenceCoverageResult(Set.of(), java.util.Map.of(), successfulSkills, java.util.List.of());
        }

        Set<String> presentClaims = contract.presentClaims(candidate);
        return coverageValidator.validateEvidenceForClaims(presentClaims, successfulSkills, contract);
    }
}
