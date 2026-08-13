package com.lokiscale.loomspan.sample.insurance;

import com.lokiscale.loomspan.api.SkillMethod;
import com.lokiscale.loomspan.api.SkillParam;
import org.springframework.stereotype.Service;

import java.util.ArrayList;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Locale;
import java.util.Map;

@Service
public class ClaimsHistoryService {

    @SkillMethod(description = "Looks up prior claims count, recency, and similar loss types for the scenario.")
    public Map<String, Object> priorClaimsLookup(
            @SkillParam(description = "Fixture key that selects canned claims history.") String scenario) {
        String key = normalize(scenario);
        Map<String, Object> result = new LinkedHashMap<>();
        result.put("scenario", key.isEmpty() ? "unknown" : key);

        return switch (key) {
            case "clear-auto-pay" -> {
                result.put("priorClaimsCount", 0);
                result.put("claimsLast60Days", 0);
                result.put("similarLossTypes", List.of());
                result.put("notes", "No prior claims on file; clean history.");
                yield result;
            }
            case "exclusion-flood" -> {
                result.put("priorClaimsCount", 1);
                result.put("claimsLast60Days", 0);
                result.put("similarLossTypes", List.of("property"));
                result.put("notes", "One older property claim; not velocity-related.");
                yield result;
            }
            case "fraud-velocity" -> {
                result.put("priorClaimsCount", 3);
                result.put("claimsLast60Days", 2);
                result.put("similarLossTypes", List.of("auto", "auto", "auto"));
                result.put("recentClaims", List.of(
                        Map.of("lossType", "auto", "daysAgo", 18, "amount", 4100),
                        Map.of("lossType", "auto", "daysAgo", 41, "amount", 3900),
                        Map.of("lossType", "auto", "daysAgo", 120, "amount", 2200)));
                result.put("notes", "Third similar auto claim within 60 days; high velocity pattern.");
                yield result;
            }
            case "ambiguous-liability" -> {
                result.put("priorClaimsCount", 1);
                result.put("claimsLast60Days", 0);
                result.put("similarLossTypes", List.of("liability"));
                result.put("notes", "One older liability-related claim; no recent velocity.");
                yield result;
            }
            case "over-limit" -> {
                result.put("priorClaimsCount", 0);
                result.put("claimsLast60Days", 0);
                result.put("similarLossTypes", List.of());
                result.put("notes", "No prior claims; clean fraud history.");
                yield result;
            }
            default -> {
                result.put("priorClaimsCount", 0);
                result.put("claimsLast60Days", 0);
                result.put("similarLossTypes", List.of());
                result.put("notes", "No scenario-specific history; treating as clean.");
                yield result;
            }
        };
    }

    @SkillMethod(description = "Returns a 0.0–1.0 anomaly score and short reason for the claim scenario.")
    public Map<String, Object> anomalyScore(
            @SkillParam(description = "Fixture key that selects canned anomaly scoring.") String scenario) {
        String key = normalize(scenario);
        Map<String, Object> result = new LinkedHashMap<>();
        result.put("scenario", key.isEmpty() ? "unknown" : key);

        return switch (key) {
            case "clear-auto-pay" -> {
                result.put("anomalyScore", 0.08);
                result.put("fraudRiskHint", "low");
                result.put("reason", "Routine minor collision pattern; no velocity or amount anomalies.");
                yield result;
            }
            case "exclusion-flood" -> {
                result.put("anomalyScore", 0.15);
                result.put("fraudRiskHint", "low");
                result.put("reason", "Property water claim; anomaly low — coverage exclusion is the main issue.");
                yield result;
            }
            case "fraud-velocity" -> {
                result.put("anomalyScore", 0.91);
                result.put("fraudRiskHint", "high");
                result.put("reason", "Repeated similar auto claims in 60 days with rising amounts.");
                yield result;
            }
            case "ambiguous-liability" -> {
                result.put("anomalyScore", 0.28);
                result.put("fraudRiskHint", "low");
                result.put("reason", "Incomplete facts raise mild uncertainty; not a strong fraud signal.");
                yield result;
            }
            case "over-limit" -> {
                result.put("anomalyScore", 0.12);
                result.put("fraudRiskHint", "low");
                result.put("reason", "High claimed amount vs limit is a coverage math issue, not fraud velocity.");
                yield result;
            }
            default -> {
                result.put("anomalyScore", 0.1);
                result.put("fraudRiskHint", "low");
                result.put("reason", "No scenario-specific anomaly; neutral low score.");
                yield result;
            }
        };
    }

    @SkillMethod(description = "Returns optional address/velocity risk flags for the claim scenario.")
    public Map<String, Object> addressRiskSignals(
            @SkillParam(description = "Fixture key that selects canned address risk signals.") String scenario) {
        String key = normalize(scenario);
        Map<String, Object> result = new LinkedHashMap<>();
        result.put("scenario", key.isEmpty() ? "unknown" : key);
        List<String> signals = new ArrayList<>();

        if ("fraud-velocity".equals(key)) {
            signals.add("same_mailing_address_as_two_recent_claims");
            signals.add("claim_filed_from_high_velocity_zip");
            signals.add("repair_shop_shared_with_prior_claim");
        }

        result.put("riskSignals", signals);
        result.put("notes", signals.isEmpty()
                ? "No address or velocity risk flags for this scenario."
                : "Address/velocity flags present; review for SIU referral.");
        return result;
    }

    private static String normalize(String scenario) {
        return scenario == null ? "" : scenario.trim().toLowerCase(Locale.ROOT);
    }
}
