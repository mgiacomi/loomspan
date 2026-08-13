package com.lokiscale.loomspan.sample.incident;

import com.lokiscale.loomspan.api.SkillMethod;
import com.lokiscale.loomspan.api.SkillParam;
import org.springframework.stereotype.Service;

import java.util.List;
import java.util.Map;

@Service
public class IncidentTelemetryService {

    @SkillMethod(description = "Checks DNS resolution status for the incident scenario.")
    public Map<String, Object> checkDns(
            @SkillParam(description = "Fixture key that selects canned DNS telemetry.") String scenario) {
        return switch (normalize(scenario)) {
            case "network-dns" -> Map.of(
                    "status", "FAIL",
                    "hostname", "api.example.com",
                    "region", "eu-west-1",
                    "notes", "NXDOMAIN / SERVFAIL for EU resolvers; US resolvers still succeed.");
            case "firewall-block" -> Map.of(
                    "status", "OK",
                    "hostname", "wiki.internal.example.com",
                    "region", "us-east-1",
                    "notes", "Public DNS resolves; issue is likely after name resolution.");
            case "ambiguous-slow" -> Map.of(
                    "status", "OK",
                    "hostname", "api.example.com",
                    "region", "multi",
                    "notes", "DNS healthy; no resolution failures in the last hour.");
            case "app-deploy-regression" -> Map.of(
                    "status", "OK",
                    "hostname", "checkout.example.com",
                    "region", "us-east-1",
                    "notes", "DNS healthy for checkout endpoints.");
            default -> Map.of(
                    "status", "OK",
                    "hostname", "unknown",
                    "region", "unknown",
                    "notes", "No scenario-specific DNS signal; treating as healthy.");
        };
    }

    @SkillMethod(description = "Checks service latency percentiles for the incident scenario.")
    public Map<String, Object> checkLatency(
            @SkillParam(description = "Fixture key that selects canned latency telemetry.") String scenario) {
        return switch (normalize(scenario)) {
            case "network-dns" -> Map.of(
                    "p50Ms", 45,
                    "p95Ms", 120,
                    "region", "eu-west-1",
                    "notes", "Latency normal for clients that still resolve; many EU clients never connect.");
            case "ambiguous-slow" -> Map.of(
                    "p50Ms", 380,
                    "p95Ms", 2100,
                    "region", "multi",
                    "notes", "Intermittent p95 spikes across regions; no single region dominates.");
            case "firewall-block" -> Map.of(
                    "p50Ms", 30,
                    "p95Ms", 80,
                    "region", "us-east-1",
                    "notes", "Latency fine for VPN users; wiki path times out for office network.");
            case "app-deploy-regression" -> Map.of(
                    "p50Ms", 90,
                    "p95Ms", 450,
                    "region", "us-east-1",
                    "notes", "Latency elevated but secondary to 5xx errors.");
            default -> Map.of(
                    "p50Ms", 50,
                    "p95Ms", 150,
                    "region", "unknown",
                    "notes", "No scenario-specific latency signal; within normal range.");
        };
    }

    @SkillMethod(description = "Checks recent firewall deny hits for the incident scenario.")
    public Map<String, Object> checkFirewallRules(
            @SkillParam(description = "Fixture key that selects canned firewall telemetry.") String scenario) {
        return switch (normalize(scenario)) {
            case "firewall-block" -> Map.of(
                    "denyHitsLast15m", 842,
                    "ruleId", "ACL-4421",
                    "source", "office-vlan",
                    "destination", "wiki.internal.example.com:443",
                    "notes", "Spike in denies after 09:15 rule change; VPN path still allowed.");
            case "network-dns" -> Map.of(
                    "denyHitsLast15m", 2,
                    "ruleId", "none",
                    "notes", "No meaningful firewall deny pattern for api.example.com.");
            case "ambiguous-slow" -> Map.of(
                    "denyHitsLast15m", 12,
                    "ruleId", "ACL-baseline",
                    "notes", "Background deny noise only; not correlated with slow reports.");
            case "app-deploy-regression" -> Map.of(
                    "denyHitsLast15m", 0,
                    "ruleId", "none",
                    "notes", "No firewall denies for checkout traffic.");
            default -> Map.of(
                    "denyHitsLast15m", 0,
                    "ruleId", "none",
                    "notes", "No scenario-specific firewall signal.");
        };
    }

    @SkillMethod(description = "Returns application 5xx error rate for the incident scenario.")
    public Map<String, Object> getErrorRate(
            @SkillParam(description = "Fixture key that selects canned error-rate telemetry.") String scenario) {
        return switch (normalize(scenario)) {
            case "app-deploy-regression" -> Map.of(
                    "service", "checkout",
                    "errorRate5xx", 0.18,
                    "window", "15m",
                    "notes", "5xx rate jumped from ~0.2% to 18% after 14:02.");
            case "ambiguous-slow" -> Map.of(
                    "service", "api-gateway",
                    "errorRate5xx", 0.012,
                    "window", "15m",
                    "notes", "Mild error elevation; not a clear outage signal.");
            case "network-dns" -> Map.of(
                    "service", "api",
                    "errorRate5xx", 0.001,
                    "window", "15m",
                    "notes", "App error rate normal; failures look pre-application.");
            case "firewall-block" -> Map.of(
                    "service", "wiki",
                    "errorRate5xx", 0.0,
                    "window", "15m",
                    "notes", "No 5xx from wiki service; clients never reach it from office network.");
            default -> Map.of(
                    "service", "unknown",
                    "errorRate5xx", 0.0,
                    "window", "15m",
                    "notes", "No scenario-specific error-rate signal.");
        };
    }

    @SkillMethod(description = "Returns recent deploys for services related to the incident scenario.")
    public Map<String, Object> getRecentDeploys(
            @SkillParam(description = "Fixture key that selects canned deploy history.") String scenario) {
        return switch (normalize(scenario)) {
            case "app-deploy-regression" -> Map.of(
                    "deploys", List.of(
                            Map.of(
                                    "service", "checkout",
                                    "version", "checkout-2.14.0",
                                    "timestamp", "2026-07-14T14:02:00Z",
                                    "actor", "ci-pipeline")),
                    "notes", "Checkout deploy at 14:02 correlates with error spike.");
            case "ambiguous-slow" -> Map.of(
                    "deploys", List.of(),
                    "notes", "No production deploys in the last 24h.");
            case "network-dns" -> Map.of(
                    "deploys", List.of(),
                    "notes", "No relevant app deploys; DNS path is independent.");
            case "firewall-block" -> Map.of(
                    "deploys", List.of(),
                    "notes", "No wiki deploys today; network change is the main event.");
            default -> Map.of(
                    "deploys", List.of(),
                    "notes", "No scenario-specific deploy history.");
        };
    }

    @SkillMethod(description = "Returns service health status for the incident scenario.")
    public Map<String, Object> getServiceHealth(
            @SkillParam(description = "Fixture key that selects canned health status.") String scenario) {
        return switch (normalize(scenario)) {
            case "app-deploy-regression" -> Map.of(
                    "service", "checkout",
                    "status", "DEGRADED",
                    "checks", List.of("liveness=UP", "readiness=FAIL", "dependency=payments=UP"),
                    "notes", "Checkout readiness failing; payments dependency still up.");
            case "ambiguous-slow" -> Map.of(
                    "service", "api-gateway",
                    "status", "DEGRADED",
                    "checks", List.of("liveness=UP", "readiness=UP", "dependency=cache=DEGRADED"),
                    "notes", "Cache dependency flapping; overall service still accepting traffic.");
            case "network-dns" -> Map.of(
                    "service", "api",
                    "status", "UP",
                    "checks", List.of("liveness=UP", "readiness=UP"),
                    "notes", "Service health green where traffic arrives.");
            case "firewall-block" -> Map.of(
                    "service", "wiki",
                    "status", "UP",
                    "checks", List.of("liveness=UP", "readiness=UP"),
                    "notes", "Wiki service healthy from monitoring probes on allowed paths.");
            default -> Map.of(
                    "service", "unknown",
                    "status", "UP",
                    "checks", List.of(),
                    "notes", "No scenario-specific health signal.");
        };
    }

    @SkillMethod(description = "Looks up a short incident runbook excerpt for the scenario and optional category.")
    public Map<String, Object> lookupRunbook(
            @SkillParam(description = "Fixture key that selects canned runbook text.") String scenario,
            @SkillParam(description = "Optional incident category hint (network, application, mixed, unknown).",
                    required = false) String category) {
        String key = normalize(scenario);
        String cat = category == null ? "" : category.trim().toLowerCase();
        String text = switch (key) {
            case "network-dns" ->
                    "DNS outage runbook: verify resolver health, check zone records for api.example.com, "
                            + "compare EU vs US resolver answers, escalate to network on-call if NXDOMAIN persists.";
            case "app-deploy-regression" ->
                    "Deploy regression runbook: identify last checkout deploy, compare error budget, "
                            + "consider rollback of checkout-2.14.0, verify payments dependency remains healthy.";
            case "firewall-block" ->
                    "Firewall change runbook: review ACL-4421 change window, confirm office-vlan path to wiki, "
                            + "temporary allowlist if change is confirmed bad, open change ticket for permanent fix.";
            case "ambiguous-slow" ->
                    "Intermittent latency runbook: check multi-region p95, cache dependency health, "
                            + "and recent config changes; avoid full rollback without a deploy signal.";
            default -> cat.isEmpty()
                    ? "Generic incident runbook: classify impact, gather one investigation branch, draft user status."
                    : "Generic " + cat + " runbook: gather supporting telemetry, avoid inventing probe results, draft user status.";
        };
        return Map.of(
                "scenario", key.isEmpty() ? "unknown" : key,
                "category", cat.isEmpty() ? "unspecified" : cat,
                "runbookText", text);
    }

    private static String normalize(String scenario) {
        return scenario == null ? "" : scenario.trim().toLowerCase();
    }
}
