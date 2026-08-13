package com.lokiscale.loomspan.sample.travel;

import tools.jackson.core.type.TypeReference;
import tools.jackson.databind.ObjectMapper;
import com.lokiscale.loomspan.api.SkillMethod;
import com.lokiscale.loomspan.api.SkillParam;
import org.springframework.stereotype.Service;

import java.time.LocalDate;
import java.time.LocalDateTime;
import java.time.format.DateTimeFormatter;
import java.time.format.DateTimeParseException;
import java.time.temporal.ChronoUnit;
import java.util.ArrayList;
import java.util.Comparator;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Locale;
import java.util.Map;

@Service
public class TravelCatalogService {

    private static final ObjectMapper OBJECT_MAPPER = new ObjectMapper();

    @SkillMethod(description = "Searches fake flight inventory and returns multiple priced options for the trip scenario.")
    public Map<String, Object> searchFlights(
            @SkillParam(description = "Fixture key that selects canned flight inventory.") String scenario,
            @SkillParam(description = "Optional origin airport or city code.", required = false) String origin,
            @SkillParam(description = "Optional destination airport or city code.", required = false) String destination,
            @SkillParam(description = "Optional departure date (yyyy-MM-dd preferred; datetime or free text falls back to scenario default).", required = false) String date) {
        String key = normalize(scenario);
        String from = hasText(origin) ? origin.trim() : defaultOrigin(key);
        String to = hasText(destination) ? destination.trim() : defaultDestination(key);
        String departDate = resolveDate(date, defaultDate(key));

        List<Map<String, Object>> options = switch (key) {
            case "budget-nyc-weekend" -> List.of(
                    flight("BudgetAir", from, to, departDate + "T06:30:00", departDate + "T08:45:00", 189.0, 0),
                    flight("Metro Hopper", from, to, departDate + "T09:00:00", departDate + "T13:20:00", 129.0, 1),
                    // Dominated for budget: expensive nonstop
                    flight("Premium Express", from, to, departDate + "T07:00:00", departDate + "T08:15:00", 420.0, 0));
            case "fastest-sfo-sea" -> List.of(
                    flight("Pacific Jet", from, to, departDate + "T06:00:00", departDate + "T08:05:00", 210.0, 0),
                    flight("Coastal Link", from, to, departDate + "T05:30:00", departDate + "T07:40:00", 245.0, 0),
                    // Dominated for speed: cheap multi-stop, much longer
                    flight("Bargain Multi", from, to, departDate + "T05:00:00", departDate + "T12:30:00", 98.0, 2));
            case "loyalty-points-max" -> List.of(
                    flight("SkyMiles Air", from, to, departDate + "T10:00:00", departDate + "T13:15:00", 280.0, 0),
                    flight("Partner Hopper", from, to, departDate + "T11:30:00", departDate + "T16:00:00", 195.0, 1));
            case "underspecified" -> List.of(
                    flight("Generic Air A", from, to, departDate + "T08:00:00", departDate + "T11:00:00", 250.0, 0),
                    flight("Generic Air B", from, to, departDate + "T14:00:00", departDate + "T19:30:00", 180.0, 1));
            default -> List.of(
                    flight("Demo Air", from, to, departDate + "T09:00:00", departDate + "T12:00:00", 220.0, 0),
                    flight("Demo Hopper", from, to, departDate + "T15:00:00", departDate + "T20:00:00", 160.0, 1));
        };

        return Map.of(
                "scenario", key.isEmpty() ? "unknown" : key,
                "origin", from,
                "destination", to,
                "date", departDate,
                "currency", "USD",
                "options", options,
                "notes", "Fake flight inventory for demo; not real availability.");
    }

    @SkillMethod(description = "Searches fake train inventory and returns multiple priced options for the trip scenario.")
    public Map<String, Object> searchTrains(
            @SkillParam(description = "Fixture key that selects canned train inventory.") String scenario,
            @SkillParam(description = "Optional origin city or station.", required = false) String origin,
            @SkillParam(description = "Optional destination city or station.", required = false) String destination,
            @SkillParam(description = "Optional departure date (yyyy-MM-dd preferred; datetime or free text falls back to scenario default).", required = false) String date) {
        String key = normalize(scenario);
        String from = hasText(origin) ? origin.trim() : defaultOrigin(key);
        String to = hasText(destination) ? destination.trim() : defaultDestination(key);
        String departDate = resolveDate(date, defaultDate(key));

        List<Map<String, Object>> options = switch (key) {
            case "budget-nyc-weekend" -> List.of(
                    train("Northeast Regional", from, to, departDate, 210, 69.0),
                    train("Acela Express", from, to, departDate, 165, 149.0),
                    // Dominated for budget: slow + expensive
                    train("Scenic Coach", from, to, departDate, 360, 189.0));
            case "fastest-sfo-sea" -> List.of(
                    train("Coast Starlight", from, to, departDate, 1320, 98.0),
                    train("Cascades", from, to, departDate, 1080, 120.0));
            case "loyalty-points-max" -> List.of(
                    train("Regional Day", from, to, departDate, 240, 85.0),
                    train("Express Rail", from, to, departDate, 180, 130.0));
            case "underspecified" -> List.of(
                    train("Generic Rail A", from, to, departDate, 300, 75.0),
                    train("Generic Rail B", from, to, departDate, 420, 55.0));
            default -> List.of(
                    train("Demo Regional", from, to, departDate, 240, 80.0),
                    train("Demo Express", from, to, departDate, 180, 125.0));
        };

        return Map.of(
                "scenario", key.isEmpty() ? "unknown" : key,
                "origin", from,
                "destination", to,
                "date", departDate,
                "currency", "USD",
                "options", options,
                "notes", "Fake train inventory for demo; not real availability.");
    }

    @SkillMethod(description = "Searches fake hotel inventory and returns multiple priced options for the trip scenario.")
    public Map<String, Object> searchHotels(
            @SkillParam(description = "Fixture key that selects canned hotel inventory.") String scenario,
            @SkillParam(description = "Optional destination city.", required = false) String destination,
            @SkillParam(description = "Optional check-in date (yyyy-MM-dd preferred; datetime or free text falls back to scenario default).", required = false) String startDate,
            @SkillParam(description = "Optional check-out date (yyyy-MM-dd preferred; datetime or free text falls back to scenario default).", required = false) String endDate,
            @SkillParam(description = "Optional party size; catalog may mildly bias room notes.", required = false) Integer partySize) {
        String key = normalize(scenario);
        String city = hasText(destination) ? destination.trim() : defaultDestination(key);
        String checkIn = resolveDate(startDate, defaultDate(key));
        String checkOut = resolveDate(endDate, defaultEndDate(key));
        int party = partySize != null && partySize > 0 ? partySize : 1;

        List<Map<String, Object>> options = switch (key) {
            case "budget-nyc-weekend" -> List.of(
                    hotel("Downtown Hostel Bunk", city, 49.0, 3.8, "Lower East Side", null, party),
                    hotel("Budget Inn Midtown", city, 119.0, 4.0, "Midtown", "ValueStay", party),
                    // Dominated for budget: luxury rate
                    hotel("Park Avenue Grand", city, 389.0, 4.7, "Upper East Side", "Luxury Collection", party));
            case "loyalty-points-max" -> List.of(
                    hotel("Marriott Marquis Demo", city, 279.0, 4.5, "Times Square", "Marriott", party),
                    hotel("Marriott Courtyard Demo", city, 219.0, 4.2, "Midtown East", "Marriott", party),
                    hotel("Independent Boutique", city, 189.0, 4.4, "Chelsea", null, party));
            case "fastest-sfo-sea" -> List.of(
                    hotel("Airport Express Inn", city, 159.0, 4.0, "SeaTac", "ValueStay", party),
                    hotel("Downtown Seattle Lodge", city, 199.0, 4.3, "Downtown", "CityStay", party));
            case "underspecified" -> List.of(
                    hotel("Generic Beach Hotel", city, 140.0, 4.0, "Waterfront", null, party),
                    hotel("Generic City Inn", city, 110.0, 3.9, "Center", "ValueStay", party));
            default -> List.of(
                    hotel("Demo Hotel A", city, 150.0, 4.1, "Center", null, party),
                    hotel("Demo Hotel B", city, 190.0, 4.3, "Riverside", "CityStay", party));
        };

        Map<String, Object> result = new LinkedHashMap<>();
        result.put("scenario", key.isEmpty() ? "unknown" : key);
        result.put("destination", city);
        result.put("startDate", checkIn);
        result.put("endDate", checkOut);
        result.put("partySize", party);
        result.put("currency", "USD");
        result.put("options", options);
        result.put("notes", "Fake hotel inventory for demo; not real availability.");
        return result;
    }

    @SkillMethod(description = "Returns loyalty tier and hotel-chain perks for the trip scenario.")
    public Map<String, Object> checkLoyaltyPerks(
            @SkillParam(description = "Fixture key that selects canned loyalty data.") String scenario,
            @SkillParam(description = "Optional loyalty tier (e.g. gold, silver).", required = false) String loyaltyTier,
            @SkillParam(description = "Optional hotel chain name.", required = false) String hotelChain) {
        String key = normalize(scenario);
        String tier = hasText(loyaltyTier) ? loyaltyTier.trim() : defaultTier(key);
        String chain = hasText(hotelChain) ? hotelChain.trim() : defaultChain(key);

        return switch (key) {
            case "loyalty-points-max" -> Map.of(
                    "scenario", key,
                    "loyaltyTier", "gold",
                    "hotelChain", "Marriott",
                    "perks", List.of(
                            "Late checkout until 4pm",
                            "Room upgrade when available",
                            "2x points on stay",
                            "Complimentary breakfast for two",
                            "Lounge access"),
                    "pointsMultiplier", 2.0,
                    "notes", "Strong gold-tier Marriott perks for loyalty-max demo.");
            case "budget-nyc-weekend" -> Map.of(
                    "scenario", key,
                    "loyaltyTier", tier,
                    "hotelChain", chain,
                    "perks", List.of("Member rate (minimal)"),
                    "pointsMultiplier", 1.0,
                    "notes", "Budget path; loyalty perks are thin.");
            case "fastest-sfo-sea" -> Map.of(
                    "scenario", key,
                    "loyaltyTier", tier,
                    "hotelChain", chain,
                    "perks", List.of("Standard member Wi-Fi"),
                    "pointsMultiplier", 1.0,
                    "notes", "Speed trip; loyalty is secondary.");
            default -> Map.of(
                    "scenario", key.isEmpty() ? "unknown" : key,
                    "loyaltyTier", tier,
                    "hotelChain", chain,
                    "perks", List.of("Standard member rate"),
                    "pointsMultiplier", 1.0,
                    "notes", "Neutral loyalty placeholder for demo.");
        };
    }

    @SkillMethod(description = "Deterministically ranks transport options by price or duration. Prefer options as a JSON array of compact option objects (price, durationMinutes, identity). optionsJson string is accepted as a fallback. sortBy is optional (price|duration; default price). Java ranks; the planner still picks. On bad input returns ok=false with a clear error (does not throw).")
    public Map<String, Object> rankTransportOptions(
            @SkillParam(description = "Preferred: JSON array of compact transport option objects with price, durationMinutes, and one identity field (airline, operator, or id). Example: [{\"airline\":\"Pacific Jet\",\"price\":210.0,\"durationMinutes\":125},{\"airline\":\"Bargain Multi\",\"price\":98.0,\"durationMinutes\":450}]", required = false) List<Map<String, Object>> options,
            @SkillParam(description = "Fallback when options cannot be passed as an array: JSON array string of the same compact option objects.", required = false) String optionsJson,
            @SkillParam(description = "Optional sort key: price (ascending) or duration (ascending). Default price when omitted.", required = false) String sortBy) {
        String sort = hasText(sortBy) ? sortBy.trim().toLowerCase(Locale.ROOT) : "price";
        if (!sort.equals("price") && !sort.equals("duration")) {
            sort = "price";
        }

        ParseResult parsed = parseOptions(options, optionsJson);
        if (!parsed.ok()) {
            Map<String, Object> error = new LinkedHashMap<>();
            error.put("ok", false);
            error.put("sortBy", sort);
            error.put("ranked", List.of());
            error.put("count", 0);
            error.put("error", parsed.error());
            error.put("hint", parsed.hint());
            if (parsed.rawPreview() != null) {
                error.put("rawPreview", parsed.rawPreview());
            }
            error.put("notes", "Ranking failed; re-call with options as a JSON array (preferred) or optionsJson string, or pick from the prior search result without ranking.");
            return error;
        }

        List<Map<String, Object>> ranked = new ArrayList<>(parsed.options());
        Comparator<Map<String, Object>> comparator = sort.equals("duration")
                ? Comparator.comparingDouble(TravelCatalogService::durationValue)
                : Comparator.comparingDouble(TravelCatalogService::priceValue);
        ranked.sort(comparator);

        List<Map<String, Object>> withRank = new ArrayList<>();
        for (int i = 0; i < ranked.size(); i++) {
            Map<String, Object> copy = new LinkedHashMap<>(ranked.get(i));
            copy.put("rank", i + 1);
            withRank.add(copy);
        }

        Map<String, Object> result = new LinkedHashMap<>();
        result.put("ok", true);
        result.put("sortBy", sort);
        result.put("ranked", withRank);
        result.put("count", withRank.size());
        result.put("notes", "Deterministic Java ranking only; planner chooses the final option.");
        return result;
    }

    private static ParseResult parseOptions(List<Map<String, Object>> options, String optionsJson) {
        if (options != null && !options.isEmpty()) {
            List<Map<String, Object>> copied = new ArrayList<>();
            for (Map<String, Object> option : options) {
                if (option != null) {
                    copied.add(new LinkedHashMap<>(option));
                }
            }
            if (!copied.isEmpty()) {
                return ParseResult.success(copied);
            }
        }
        return parseOptionsJson(optionsJson);
    }

    private static ParseResult parseOptionsJson(String optionsJson) {
        if (!hasText(optionsJson)) {
            return ParseResult.failure(
                    "options/optionsJson is missing or blank",
                    "Pass options as a JSON array of option objects (preferred), or optionsJson as a JSON array string (price + durationMinutes + identity).");
        }
        String trimmed = optionsJson.trim();
        try {
            List<Map<String, Object>> parsed = OBJECT_MAPPER.readValue(
                    trimmed,
                    new TypeReference<List<Map<String, Object>>>() {
                    });
            if (parsed == null || parsed.isEmpty()) {
                return ParseResult.failure(
                        "optionsJson parsed to an empty array",
                        "Include at least two option objects copied from the search result options array.",
                        preview(trimmed));
            }
            return ParseResult.success(parsed);
        }
        catch (Exception ex) {
            return ParseResult.failure(
                    "Could not parse optionsJson as a JSON array of objects: " + ex.getClass().getSimpleName(),
                    "Prefer the options array parameter. If using optionsJson, copy only compact fields (price, durationMinutes, airline|operator|id). Do not wrap the whole search response; optionsJson must be a [...] array string.",
                    preview(trimmed));
        }
    }

    private static String preview(String raw) {
        if (raw == null) {
            return null;
        }
        int max = 240;
        return raw.length() <= max ? raw : raw.substring(0, max) + "...";
    }

    private record ParseResult(boolean ok, List<Map<String, Object>> options, String error, String hint, String rawPreview) {
        static ParseResult success(List<Map<String, Object>> options) {
            return new ParseResult(true, options, null, null, null);
        }

        static ParseResult failure(String error, String hint) {
            return new ParseResult(false, List.of(), error, hint, null);
        }

        static ParseResult failure(String error, String hint, String rawPreview) {
            return new ParseResult(false, List.of(), error, hint, rawPreview);
        }
    }

    private static double priceValue(Map<String, Object> option) {
        return numericValue(option.get("price"));
    }

    private static double durationValue(Map<String, Object> option) {
        Object duration = option.get("durationMinutes");
        if (duration == null) {
            duration = option.get("duration");
        }
        return numericValue(duration);
    }

    private static double numericValue(Object raw) {
        if (raw instanceof Number number) {
            return number.doubleValue();
        }
        if (raw instanceof String text && hasText(text)) {
            try {
                return Double.parseDouble(text.trim());
            }
            catch (NumberFormatException ignored) {
                return Double.MAX_VALUE;
            }
        }
        return Double.MAX_VALUE;
    }

    private static Map<String, Object> flight(
            String airline, String origin, String destination, String depart, String arrive, double price, int stops) {
        Map<String, Object> option = new LinkedHashMap<>();
        option.put("mode", "flight");
        option.put("airline", airline);
        option.put("origin", origin);
        option.put("destination", destination);
        option.put("depart", depart);
        option.put("arrive", arrive);
        option.put("durationMinutes", durationMinutesBetween(depart, arrive));
        option.put("price", price);
        option.put("stops", stops);
        option.put("currency", "USD");
        return option;
    }

    private static int durationMinutesBetween(String depart, String arrive) {
        LocalDateTime start = LocalDateTime.parse(depart, DateTimeFormatter.ISO_LOCAL_DATE_TIME);
        LocalDateTime end = LocalDateTime.parse(arrive, DateTimeFormatter.ISO_LOCAL_DATE_TIME);
        return (int) ChronoUnit.MINUTES.between(start, end);
    }

    private static Map<String, Object> train(
            String operator, String origin, String destination, String date, int durationMinutes, double price) {
        Map<String, Object> option = new LinkedHashMap<>();
        option.put("mode", "train");
        option.put("operator", operator);
        option.put("origin", origin);
        option.put("destination", destination);
        option.put("date", date);
        option.put("durationMinutes", durationMinutes);
        option.put("price", price);
        option.put("currency", "USD");
        return option;
    }

    private static Map<String, Object> hotel(
            String name, String city, double nightlyRate, double rating, String neighborhood, String chain, int partySize) {
        Map<String, Object> option = new LinkedHashMap<>();
        option.put("name", name);
        option.put("destination", city);
        option.put("nightlyRate", nightlyRate);
        option.put("rating", rating);
        option.put("neighborhood", neighborhood);
        option.put("currency", "USD");
        if (chain != null) {
            option.put("chain", chain);
        }
        option.put("partySizeNote", partySize > 2 ? "May need two rooms or suite for party of " + partySize : "Standard room ok");
        return option;
    }

    private static String defaultOrigin(String scenario) {
        return switch (scenario) {
            case "budget-nyc-weekend" -> "BOS";
            case "fastest-sfo-sea" -> "SFO";
            case "loyalty-points-max" -> "ORD";
            default -> "XXX";
        };
    }

    private static String defaultDestination(String scenario) {
        return switch (scenario) {
            case "budget-nyc-weekend" -> "NYC";
            case "fastest-sfo-sea" -> "SEA";
            case "loyalty-points-max" -> "NYC";
            case "underspecified" -> "somewhere warm";
            default -> "YYY";
        };
    }

    private static String defaultDate(String scenario) {
        return switch (scenario) {
            case "budget-nyc-weekend" -> "2026-03-14";
            case "fastest-sfo-sea" -> "2026-03-20";
            case "loyalty-points-max" -> "2026-04-10";
            case "underspecified" -> "2026-03-01";
            default -> "2026-03-15";
        };
    }

    private static String defaultEndDate(String scenario) {
        return switch (scenario) {
            case "budget-nyc-weekend" -> "2026-03-16";
            case "fastest-sfo-sea" -> "2026-03-21";
            case "loyalty-points-max" -> "2026-04-13";
            case "underspecified" -> "2026-03-05";
            default -> "2026-03-17";
        };
    }

    private static String defaultTier(String scenario) {
        return "loyalty-points-max".equals(scenario) ? "gold" : "member";
    }

    private static String defaultChain(String scenario) {
        return "loyalty-points-max".equals(scenario) ? "Marriott" : "ValueStay";
    }

    private static String resolveDate(String raw, String fallback) {
        String normalized = normalizeDateOnly(raw);
        return normalized != null ? normalized : fallback;
    }

    private static String normalizeDateOnly(String raw) {
        if (!hasText(raw)) {
            return null;
        }
        String trimmed = raw.trim();
        // Prefer a leading ISO date prefix so datetime values like "2026-03-14T08:00:00"
        // or "2026-03-14 09:00:00" normalize without throwing.
        if (trimmed.length() >= 10) {
            String prefix = trimmed.substring(0, 10);
            try {
                return LocalDate.parse(prefix, DateTimeFormatter.ISO_LOCAL_DATE).toString();
            }
            catch (DateTimeParseException ignored) {
                // fall through to alternate whole-string formats
            }
        }
        try {
            return LocalDate.parse(trimmed, DateTimeFormatter.ISO_LOCAL_DATE).toString();
        }
        catch (DateTimeParseException ignored) {
            // try alternate formats below
        }
        for (DateTimeFormatter formatter : List.of(
                DateTimeFormatter.ofPattern("M/d/uuuu"),
                DateTimeFormatter.ofPattern("M-d-uuuu"))) {
            try {
                return LocalDate.parse(trimmed, formatter).toString();
            }
            catch (DateTimeParseException ignored) {
                // continue
            }
        }
        return null;
    }

    private static String normalize(String scenario) {
        return scenario == null ? "" : scenario.trim().toLowerCase(Locale.ROOT);
    }

    private static boolean hasText(String value) {
        return value != null && !value.isBlank();
    }
}
