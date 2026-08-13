package com.lokiscale.loomspan.sample.support;

import com.lokiscale.loomspan.api.SkillMethod;
import com.lokiscale.loomspan.api.SkillParam;
import org.springframework.stereotype.Service;

import java.util.List;
import java.util.Map;

@Service
public class SupportCrmService {

    @SkillMethod(description = "Looks up customer plan, tenure, and prior complaint count for the support scenario.")
    public Map<String, Object> lookupCustomer(
            @SkillParam(description = "Fixture key that selects canned customer data.") String scenario,
            @SkillParam(description = "Optional customer id hint from the demo controller.", required = false) String customerId) {
        String key = normalize(scenario);
        String id = hasText(customerId) ? customerId.trim() : defaultCustomerId(key);
        return switch (key) {
            case "billing-duplicate-charge" -> Map.of(
                    "customerId", id,
                    "name", "Jane Demo",
                    "plan", "Pro Monthly",
                    "tenureMonths", 18,
                    "priorComplaintCount", 0,
                    "notes", "Long-tenure Pro customer; no prior billing complaints on file.");
            case "tech-crash-on-checkout" -> Map.of(
                    "customerId", id,
                    "name", "Alex Demo",
                    "plan", "Business Annual",
                    "tenureMonths", 8,
                    "priorComplaintCount", 1,
                    "notes", "Active Business account; prior complaint was unrelated shipping delay.");
            case "mixed-billing-and-crash" -> Map.of(
                    "customerId", id,
                    "name", "Sam Demo",
                    "plan", "Pro Monthly",
                    "tenureMonths", 12,
                    "priorComplaintCount", 0,
                    "notes", "First-time multi-issue contact; plan active.");
            case "how-to-export" -> Map.of(
                    "customerId", id,
                    "name", "Riley Demo",
                    "plan", "Starter",
                    "tenureMonths", 3,
                    "priorComplaintCount", 0,
                    "notes", "New Starter user asking product how-to questions.");
            case "angry-goodwill" -> Map.of(
                    "customerId", id,
                    "name", "Morgan Demo",
                    "plan", "Pro Monthly",
                    "tenureMonths", 6,
                    "priorComplaintCount", 0,
                    "notes", "First complaint on file; small overcharge dispute.");
            default -> Map.of(
                    "customerId", hasText(customerId) ? customerId.trim() : "CUST-UNKNOWN",
                    "name", "Unknown Demo Customer",
                    "plan", "Unknown",
                    "tenureMonths", 0,
                    "priorComplaintCount", 0,
                    "notes", "No scenario-specific customer record; neutral placeholder.");
        };
    }

    @SkillMethod(description = "Looks up recent invoice charges for the support scenario.")
    public Map<String, Object> lookupInvoices(
            @SkillParam(description = "Fixture key that selects canned invoice data.") String scenario,
            @SkillParam(description = "Optional customer id hint.", required = false) String customerId) {
        String key = normalize(scenario);
        String id = hasText(customerId) ? customerId.trim() : defaultCustomerId(key);
        return switch (key) {
            case "billing-duplicate-charge" -> Map.of(
                    "customerId", id,
                    "invoices", List.of(
                            Map.of("invoiceId", "INV-2026-0301", "period", "2026-03", "amount", 49.0, "status", "paid"),
                            Map.of("invoiceId", "INV-2026-0301-DUP", "period", "2026-03", "amount", 49.0, "status", "paid")),
                    "notes", "Two identical March Pro charges of $49.00.");
            case "mixed-billing-and-crash" -> Map.of(
                    "customerId", id,
                    "invoices", List.of(
                            Map.of("invoiceId", "INV-2026-0308", "period", "2026-03", "amount", 49.0, "status", "paid"),
                            Map.of("invoiceId", "INV-2026-0308-DUP", "period", "2026-03", "amount", 49.0, "status", "paid")),
                    "notes", "Duplicate March charges present alongside technical complaint.");
            case "angry-goodwill" -> Map.of(
                    "customerId", id,
                    "invoices", List.of(
                            Map.of("invoiceId", "INV-2026-0312", "period", "2026-03", "amount", 54.0, "status", "paid"),
                            Map.of("invoiceId", "INV-2026-0212", "period", "2026-02", "amount", 49.0, "status", "paid")),
                    "notes", "March charge is $5 above the usual $49 Pro monthly rate.");
            case "tech-crash-on-checkout", "how-to-export" -> Map.of(
                    "customerId", id,
                    "invoices", List.of(
                            Map.of("invoiceId", "INV-2026-0300", "period", "2026-03", "amount", 49.0, "status", "paid")),
                    "notes", "Single normal charge; no duplicate billing signal.");
            default -> Map.of(
                    "customerId", id,
                    "invoices", List.of(),
                    "notes", "No scenario-specific invoices.");
        };
    }

    @SkillMethod(description = "Returns structured refund policy eligibility facts for the support scenario.")
    public Map<String, Object> lookupRefundPolicy(
            @SkillParam(description = "Fixture key that selects canned policy facts.") String scenario) {
        String key = normalize(scenario);
        return switch (key) {
            case "billing-duplicate-charge" -> Map.of(
                    "maxGoodwillAmount", 50,
                    "firstComplaintEligible", true,
                    "goodwillEligible", true,
                    "notes", "Duplicate charge scenarios are goodwill-eligible up to $50 when priorComplaintCount is 0.");
            case "mixed-billing-and-crash" -> Map.of(
                    "maxGoodwillAmount", 50,
                    "firstComplaintEligible", true,
                    "goodwillEligible", true,
                    "notes", "Billing portion is goodwill-eligible up to $50; technical path is separate.");
            case "angry-goodwill" -> Map.of(
                    "maxGoodwillAmount", 50,
                    "firstComplaintEligible", true,
                    "goodwillEligible", true,
                    "notes", "Small overcharge with first complaint: goodwill eligible up to $50.");
            case "tech-crash-on-checkout", "how-to-export" -> Map.of(
                    "maxGoodwillAmount", 50,
                    "firstComplaintEligible", true,
                    "goodwillEligible", false,
                    "notes", "No billing dispute signal; goodwill not indicated for this scenario.");
            default -> Map.of(
                    "maxGoodwillAmount", 0,
                    "firstComplaintEligible", false,
                    "goodwillEligible", false,
                    "notes", "No scenario-specific policy facts; goodwill not eligible.");
        };
    }

    @SkillMethod(description = "Looks up account status and feature flags for the support scenario.")
    public Map<String, Object> lookupAccountStatus(
            @SkillParam(description = "Fixture key that selects canned account status.") String scenario) {
        String key = normalize(scenario);
        return switch (key) {
            case "tech-crash-on-checkout" -> Map.of(
                    "status", "active",
                    "featureFlags", List.of("checkout_v2=true", "payments_retry=true"),
                    "notes", "Account active; checkout_v2 enabled for this customer.");
            case "mixed-billing-and-crash" -> Map.of(
                    "status", "active",
                    "featureFlags", List.of("checkout_v2=true", "payments_retry=true"),
                    "notes", "Account active with checkout_v2; crash reports possible on pay step.");
            case "billing-duplicate-charge", "how-to-export", "angry-goodwill" -> Map.of(
                    "status", "active",
                    "featureFlags", List.of("checkout_v2=true"),
                    "notes", "Account healthy; no suspension.");
            default -> Map.of(
                    "status", "active",
                    "featureFlags", List.of(),
                    "notes", "No scenario-specific account signal; treating as healthy.");
        };
    }

    @SkillMethod(description = "Searches known issues and KB entries for the support scenario.")
    public Map<String, Object> searchKnownIssues(
            @SkillParam(description = "Fixture key that selects canned known-issue matches.") String scenario) {
        String key = normalize(scenario);
        return switch (key) {
            case "tech-crash-on-checkout" -> Map.of(
                    "issues", List.of(
                            Map.of(
                                    "issueId", "KI-CHECKOUT-441",
                                    "title", "Checkout crash on pay with card vault timeout",
                                    "severity", "high")),
                    "notes", "Known issue matches checkout crash reports since 2026-03-10.");
            case "mixed-billing-and-crash" -> Map.of(
                    "issues", List.of(),
                    "notes", "No confirmed known issue for this crash signature yet.");
            default -> Map.of(
                    "issues", List.of(),
                    "notes", "No known issues matched for this scenario.");
        };
    }

    @SkillMethod(description = "Creates a deterministic fake bug ticket id for the support scenario.")
    public Map<String, Object> createBugTicket(
            @SkillParam(description = "Fixture key that selects canned ticket id.") String scenario,
            @SkillParam(description = "Optional short summary echoed on the ticket.", required = false) String summary) {
        String key = normalize(scenario);
        String ticketId = switch (key) {
            case "tech-crash-on-checkout" -> "BUG-TECH-1002";
            case "mixed-billing-and-crash" -> "BUG-TECH-1003";
            default -> key.isEmpty() ? "BUG-TECH-UNKNOWN" : "BUG-TECH-" + key.toUpperCase().replace('-', '_');
        };
        String echoed = hasText(summary) ? summary.trim() : "Support demo ticket for scenario " + (key.isEmpty() ? "unknown" : key);
        return Map.of(
                "ticketId", ticketId,
                "summary", echoed,
                "status", "open",
                "notes", "Stateless demo ticket; not persisted across requests.");
    }

    @SkillMethod(description = "Searches help-center articles for the support scenario.")
    public Map<String, Object> searchHelpCenter(
            @SkillParam(description = "Fixture key that selects canned help articles.") String scenario,
            @SkillParam(description = "Optional free-text query hint.", required = false) String query) {
        String key = normalize(scenario);
        String q = hasText(query) ? query.trim() : "";
        return switch (key) {
            case "how-to-export" -> Map.of(
                    "articles", List.of(
                            Map.of(
                                    "title", "Export orders to CSV",
                                    "url", "https://help.demo.example.com/export-csv"),
                            Map.of(
                                    "title", "Schedule recurring CSV exports",
                                    "url", "https://help.demo.example.com/export-schedule")),
                    "query", q.isEmpty() ? "export csv" : q,
                    "notes", "Top help articles for CSV export.");
            case "billing-duplicate-charge", "angry-goodwill" -> Map.of(
                    "articles", List.of(
                            Map.of(
                                    "title", "Understanding your invoice",
                                    "url", "https://help.demo.example.com/invoices")),
                    "query", q.isEmpty() ? "invoice" : q,
                    "notes", "Billing help articles only; not a how-to primary path.");
            default -> Map.of(
                    "articles", List.of(),
                    "query", q,
                    "notes", "No help-center matches for this scenario.");
        };
    }

    private static String defaultCustomerId(String scenario) {
        return switch (scenario) {
            case "billing-duplicate-charge" -> "CUST-1001";
            case "tech-crash-on-checkout" -> "CUST-1002";
            case "mixed-billing-and-crash" -> "CUST-1003";
            case "how-to-export" -> "CUST-1004";
            case "angry-goodwill" -> "CUST-1005";
            default -> "CUST-UNKNOWN";
        };
    }

    private static String normalize(String scenario) {
        return scenario == null ? "" : scenario.trim().toLowerCase();
    }

    private static boolean hasText(String value) {
        return value != null && !value.isBlank();
    }
}
