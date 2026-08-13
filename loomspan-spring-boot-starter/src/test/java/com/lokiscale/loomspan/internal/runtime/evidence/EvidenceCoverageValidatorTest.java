package com.lokiscale.loomspan.internal.runtime.evidence;

import tools.jackson.databind.json.JsonMapper;
import com.lokiscale.loomspan.internal.core.ExecutionPlan;
import com.lokiscale.loomspan.internal.core.PlanStatus;
import com.lokiscale.loomspan.internal.core.PlanTask;
import com.lokiscale.loomspan.internal.core.PlanTaskStatus;
import org.junit.jupiter.api.Test;

import java.time.Instant;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;
import java.util.Set;

import static org.assertj.core.api.Assertions.assertThat;

class EvidenceCoverageValidatorTest
{
    private final EvidenceExpressionParser parser = new EvidenceExpressionParser();
    private final EvidenceCoverageValidator validator = new EvidenceCoverageValidator();

    @Test
    void evaluatesNestedBooleanRequirementsAndExplainsUnsatisfiedClauses()
    {
        EvidenceContract contract = contract(Map.of(
                "likelyCause", "classifyIncident and (investigateNetwork or investigateApp)"));

        assertThat(finalCoverage(contract, Set.of("classifyIncident", "investigateNetwork")).complete()).isTrue();
        assertThat(finalCoverage(contract, Set.of("classifyIncident", "investigateApp")).complete()).isTrue();
        assertThat(finalCoverage(contract, Set.of("classifyIncident", "investigateNetwork", "investigateApp")).complete()).isTrue();
        assertThat(finalCoverage(contract, Set.of("classifyincident", "investigateNetwork")).complete()).isFalse();

        EvidenceCoverageIssue classificationOnly = finalCoverage(contract, Set.of("classifyIncident")).issues().getFirst();
        assertThat(classificationOnly.requiredExpression())
                .isEqualTo("classifyIncident and (investigateNetwork or investigateApp)");
        assertThat(classificationOnly.unsatisfiedRequirements()).singleElement().satisfies(all ->
        {
            assertThat(all.mode()).isEqualTo("all");
            assertThat(all.children()).singleElement().satisfies(any ->
            {
                assertThat(any.mode()).isEqualTo("any");
                assertThat(any.skills()).containsExactly("investigateNetwork", "investigateApp");
            });
        });

        EvidenceCoverageIssue investigatorOnly = finalCoverage(contract, Set.of("investigateNetwork")).issues().getFirst();
        assertThat(investigatorOnly.unsatisfiedRequirements()).singleElement().satisfies(all ->
                assertThat(all.children()).singleElement().satisfies(missing ->
                {
                    assertThat(missing.mode()).isEqualTo("skill");
                    assertThat(missing.skills()).containsExactly("classifyIncident");
                }));
    }

    @Test
    void preservesConjunctiveAlternativesWithoutFlatteningAndRemainsMonotonic()
    {
        EvidenceContract contract = contract(Map.of("result", "(a and b) or (c and d)"));

        EvidenceCoverageIssue issue = validator.validateEvidenceForClaims(
                Set.of("result"), Set.of("a"), contract).issues().getFirst();
        EvidenceRequirement any = issue.unsatisfiedRequirements().getFirst();
        assertThat(any.mode()).isEqualTo("any");
        assertThat(any.children()).hasSize(2);
        assertThat(any.children().getFirst().mode()).isEqualTo("all");
        assertThat(any.children().getFirst().skills()).containsExactly("b");
        assertThat(any.children().get(1).skills()).containsExactly("c", "d");

        assertThat(validator.validateEvidenceForClaims(Set.of("result"), Set.of("a", "b"), contract).complete()).isTrue();
        assertThat(validator.validateEvidenceForClaims(Set.of("result"), Set.of("a", "b", "c"), contract).complete()).isTrue();
    }

    @Test
    void planningEvaluatesEveryClaimAgainstExactNonblankCapabilityNames()
    {
        EvidenceContract contract = contract(Map.of(
                "severity", "classifyIncident",
                "likelyCause", "classifyIncident and (investigateNetwork or investigateApp)",
                "optionalSummary", "draftIncidentResponse"));

        EvidenceCoverageResult missingOptional = validator.validatePlanCoverage(plan(
                "classifyIncident", "investigateNetwork", " ", null), contract);
        assertThat(missingOptional.evaluatedClaims()).containsExactlyInAnyOrder("severity", "likelyCause", "optionalSummary");
        assertThat(missingOptional.issues()).extracting(EvidenceCoverageIssue::claimName)
                .containsExactly("optionalSummary");

        assertThat(validator.validatePlanCoverage(plan(
                "classifyIncident", "investigateNetwork", "draftIncidentResponse"), contract).complete()).isTrue();
        assertThat(validator.validatePlanCoverage(plan(
                "classifyIncident", "investigateApp", "draftIncidentResponse"), contract).complete()).isTrue();
        assertThat(validator.validatePlanCoverage(plan(
                "classifyincident", "investigateApp", "draftIncidentResponse"), contract).complete()).isFalse();
    }

    @Test
    void finalValidationEvaluatesOnlyPresentContractBackedClaims() throws Exception
    {
        EvidenceContract contract = contract(Map.of(
                "requiredResult", "classifyIncident",
                "optionalDetail", "investigateNetwork or investigateApp"));
        EvidenceBackedOutputValidator outputValidator = new EvidenceBackedOutputValidator();

        EvidenceCoverageResult requiredOnly = outputValidator.validate(
                JsonMapper.builder().build().readTree("{\"requiredResult\":\"high\"}"),
                contract,
                Set.of("classifyIncident"));
        assertThat(requiredOnly.complete()).isTrue();
        assertThat(requiredOnly.evaluatedClaims()).containsExactly("requiredResult");

        EvidenceCoverageResult unsupportedOptional = outputValidator.validate(
                JsonMapper.builder().build().readTree("{\"requiredResult\":\"high\",\"optionalDetail\":\"network\"}"),
                contract,
                Set.of("classifyIncident"));
        assertThat(unsupportedOptional.issues()).extracting(EvidenceCoverageIssue::claimName)
                .containsExactly("optionalDetail");
    }

    private EvidenceCoverageResult finalCoverage(EvidenceContract contract, Set<String> successfulSkills)
    {
        return validator.validateEvidenceForClaims(Set.of("likelyCause"), successfulSkills, contract);
    }

    private EvidenceContract contract(Map<String, String> expressions)
    {
        LinkedHashMap<String, EvidenceExpression> compiled = new LinkedHashMap<>();
        LinkedHashMap<String, String> canonical = new LinkedHashMap<>();
        expressions.forEach((claim, expression) ->
        {
            compiled.put(claim, parser.parse(expression));
            canonical.put(claim.toLowerCase(java.util.Locale.ROOT), claim);
        });
        return EvidenceContract.compiled(compiled, canonical);
    }

    private static ExecutionPlan plan(String... capabilityNames)
    {
        List<PlanTask> tasks = java.util.stream.IntStream.range(0, capabilityNames.length)
                .mapToObj(index -> new PlanTask(
                        "task-" + index,
                        "Task " + index,
                        PlanTaskStatus.PENDING,
                        capabilityNames[index],
                        "Use capability",
                        List.of(),
                        List.of(),
                        false,
                        null))
                .toList();
        return new ExecutionPlan(
                "plan-1",
                "handleIncident",
                Instant.parse("2026-03-15T12:00:00Z"),
                PlanStatus.VALID,
                null,
                tasks);
    }
}
