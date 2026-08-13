package com.lokiscale.loomspan.sample.insurance;

import com.lokiscale.loomspan.api.SkillMethod;
import com.lokiscale.loomspan.api.SkillParam;
import org.springframework.stereotype.Service;

import java.util.ArrayList;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Locale;
import java.util.Map;
import java.util.Set;

@Service
public class InsurancePolicyService {

    private static final Policy NEUTRAL = new Policy(
            "POL-NEUTRAL", "auto", 5000.0, 500.0, "2025-01-01", "2026-12-31", Set.of());

    private static final Policy AUTO_1001 = new Policy(
            "POL-AUTO-1001", "auto", 5000.0, 500.0, "2025-01-01", "2026-12-31",
            Set.of("racing", "intentional_damage"));

    private static final Policy HOME_2002 = new Policy(
            "POL-HOME-2002", "home", 50000.0, 1000.0, "2025-01-01", "2026-12-31",
            Set.of("flood", "earthquake", "mudslide"));

    private static final List<String> FLOOD_EXCLUSIONS = List.of("flood", "water_damage_from_flood");

    @SkillMethod(description = "Retrieves policy limits, deductible, and product metadata for the claim scenario.")
    public Map<String, Object> getPolicy(
            @SkillParam(description = "Fixture key that selects canned policy data.") String scenario,
            @SkillParam(description = "Optional policy id override (e.g. POL-AUTO-1001).", required = false)
            String policyId) {
        Policy policy = resolvePolicy(scenario, policyId);
        Map<String, Object> result = new LinkedHashMap<>();
        result.put("policyId", policy.policyId());
        result.put("productType", policy.productType());
        result.put("policyLimit", policy.limit());
        result.put("deductible", policy.deductible());
        result.put("effectiveFrom", policy.effectiveFrom());
        result.put("effectiveTo", policy.effectiveTo());
        result.put("active", true);
        result.put("notes", policyNotes(scenario, policy));
        return result;
    }

    @SkillMethod(description = "Lists matched policy exclusions for the claim scenario and optional loss type/keywords.")
    public Map<String, Object> checkExclusions(
            @SkillParam(description = "Fixture key that selects canned exclusion rules.") String scenario,
            @SkillParam(description = "Optional loss type hint (auto, property, theft, liability, other).",
                    required = false) String lossType,
            @SkillParam(description = "Optional free-text keywords from the claim narrative.", required = false)
            String keywords) {
        String key = normalize(scenario);
        List<String> matched = resolveMatchedExclusions(scenario, lossType, keywords);

        Map<String, Object> result = new LinkedHashMap<>();
        result.put("matchedExclusions", matched);
        result.put("scenario", key.isEmpty() ? "unknown" : key);
        result.put("notes", matched.isEmpty()
                ? "No hard exclusions matched for this loss."
                : "Matched exclusions: " + String.join(", ", matched));
        return result;
    }

    @SkillMethod(description = "Estimates payable amount using min(claimed, limit) minus deductible; zero when excluded.")
    public Map<String, Object> estimatePayout(
            @SkillParam(description = "Fixture key that selects canned policy data.") String scenario,
            @SkillParam(description = "Claimed amount for the loss.") double claimedAmount,
            @SkillParam(description = "Optional policy id override.", required = false) String policyId,
            @SkillParam(description = "Optional loss type hint for exclusion matching.", required = false)
            String lossType,
            @SkillParam(description = "Optional free-text keywords for exclusion matching.", required = false)
            String keywords) {
        Policy policy = resolvePolicy(scenario, policyId);
        List<String> exclusions = resolveMatchedExclusions(scenario, lossType, keywords);
        double gross = Math.min(claimedAmount, policy.limit());
        double payable = Math.max(0.0, gross - policy.deductible());
        boolean excluded = !exclusions.isEmpty();
        if (excluded) {
            payable = 0.0;
        }

        Map<String, Object> result = new LinkedHashMap<>();
        result.put("policyId", policy.policyId());
        result.put("claimedAmount", claimedAmount);
        result.put("policyLimit", policy.limit());
        result.put("deductible", policy.deductible());
        result.put("gross", gross);
        result.put("payableAmount", payable);
        result.put("excluded", excluded);
        result.put("matchedExclusions", exclusions);
        result.put("formula", "payable = max(0, min(claimedAmount, policyLimit) - deductible); 0 if excluded");
        result.put("notes", excluded
                ? "Payable zeroed due to matched exclusions: " + String.join(", ", exclusions)
                : "Deterministic estimate after deductible (and limit cap if applicable).");
        return result;
    }

    static List<String> resolveMatchedExclusions(String scenario, String lossType, String keywords) {
        String key = normalize(scenario);
        String haystack = ((lossType == null ? "" : lossType) + " " + (keywords == null ? "" : keywords))
                .toLowerCase(Locale.ROOT);

        if ("exclusion-flood".equals(key) || indicatesFloodExclusion(haystack)) {
            return new ArrayList<>(FLOOD_EXCLUSIONS);
        }
        if ("clear-auto-pay".equals(key)
                || "over-limit".equals(key)
                || "fraud-velocity".equals(key)
                || "ambiguous-liability".equals(key)) {
            return List.of();
        }

        List<String> matched = new ArrayList<>();
        Policy policy = resolvePolicy(scenario, null);
        for (String exclusion : policy.exclusions()) {
            if (haystack.contains(exclusion.replace('_', ' ')) || haystack.contains(exclusion)) {
                matched.add(exclusion);
            }
        }
        return matched;
    }

    private static boolean indicatesFloodExclusion(String haystack) {
        return haystack.contains("flood")
                || haystack.contains("rising water")
                || haystack.contains("rising creek")
                || haystack.contains("water_damage_from_flood");
    }

    private static Policy resolvePolicy(String scenario, String policyId) {
        if (policyId != null && !policyId.isBlank()) {
            String id = policyId.trim().toUpperCase(Locale.ROOT);
            if ("POL-AUTO-1001".equals(id)) {
                return AUTO_1001;
            }
            if ("POL-HOME-2002".equals(id)) {
                return HOME_2002;
            }
        }
        return switch (normalize(scenario)) {
            case "clear-auto-pay", "fraud-velocity", "ambiguous-liability", "over-limit" -> AUTO_1001;
            case "exclusion-flood" -> HOME_2002;
            default -> NEUTRAL;
        };
    }

    private static String policyNotes(String scenario, Policy policy) {
        return switch (normalize(scenario)) {
            case "clear-auto-pay" -> "Active auto collision coverage; clean policy for minor collision.";
            case "exclusion-flood" -> "Homeowners policy with flood/water-from-flood exclusions.";
            case "fraud-velocity" -> "Active auto policy; history review is separate from coverage.";
            case "ambiguous-liability" -> "Active auto policy; liability facts incomplete on claim.";
            case "over-limit" -> "Active auto policy; claim amount may exceed collision limit.";
            default -> "Neutral demo policy (limit " + policy.limit() + ", deductible " + policy.deductible() + ").";
        };
    }

    private static String normalize(String scenario) {
        return scenario == null ? "" : scenario.trim().toLowerCase(Locale.ROOT);
    }

    private record Policy(
            String policyId,
            String productType,
            double limit,
            double deductible,
            String effectiveFrom,
            String effectiveTo,
            Set<String> exclusions) {
    }
}
