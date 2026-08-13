package com.lokiscale.loomspan.sample.travel;

import tools.jackson.databind.ObjectMapper;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;

import java.util.List;
import java.util.Map;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatCode;

class TravelCatalogServiceTest {

    private static final ObjectMapper OBJECT_MAPPER = new ObjectMapper();

    private TravelCatalogService service;

    @BeforeEach
    void setUp() {
        service = new TravelCatalogService();
    }

    @Test
    void knownScenariosReturnMultiOptionFlightTrainHotelCatalogs() {
        for (String scenario : List.of(
                "budget-nyc-weekend", "loyalty-points-max", "fastest-sfo-sea", "underspecified")) {
            @SuppressWarnings("unchecked")
            List<Map<String, Object>> flights =
                    (List<Map<String, Object>>) service.searchFlights(scenario, null, null, null).get("options");
            @SuppressWarnings("unchecked")
            List<Map<String, Object>> trains =
                    (List<Map<String, Object>>) service.searchTrains(scenario, null, null, null).get("options");
            @SuppressWarnings("unchecked")
            List<Map<String, Object>> hotels =
                    (List<Map<String, Object>>) service.searchHotels(scenario, null, null, null, null).get("options");

            assertThat(flights).as("flights for %s", scenario).hasSizeGreaterThanOrEqualTo(2);
            assertThat(trains).as("trains for %s", scenario).hasSizeGreaterThanOrEqualTo(2);
            assertThat(hotels).as("hotels for %s", scenario).hasSizeGreaterThanOrEqualTo(2);
        }
    }

    @Test
    void budgetScenarioIncludesDominatedExpensiveFlight() {
        @SuppressWarnings("unchecked")
        List<Map<String, Object>> flights = (List<Map<String, Object>>) service
                .searchFlights("budget-nyc-weekend", null, null, null)
                .get("options");

        assertThat(flights).hasSizeGreaterThanOrEqualTo(2);
        double minPrice = flights.stream()
                .mapToDouble(o -> ((Number) o.get("price")).doubleValue())
                .min()
                .orElseThrow();
        double maxPrice = flights.stream()
                .mapToDouble(o -> ((Number) o.get("price")).doubleValue())
                .max()
                .orElseThrow();
        assertThat(maxPrice).isGreaterThan(minPrice);
        assertThat(maxPrice).isGreaterThanOrEqualTo(400.0);
    }

    @Test
    void fastestScenarioIncludesDominatedSlowMultiStopFlight() {
        @SuppressWarnings("unchecked")
        List<Map<String, Object>> flights = (List<Map<String, Object>>) service
                .searchFlights("fastest-sfo-sea", null, null, null)
                .get("options");

        assertThat(flights).anySatisfy(option -> {
            assertThat(((Number) option.get("stops")).intValue()).isGreaterThanOrEqualTo(2);
            assertThat(((Number) option.get("price")).doubleValue()).isLessThan(150.0);
        });
        assertThat(flights).anySatisfy(option ->
                assertThat(((Number) option.get("stops")).intValue()).isEqualTo(0));
        assertThat(flights).allSatisfy(option ->
                assertThat(option.get("durationMinutes")).isInstanceOf(Number.class));
        int minDuration = flights.stream()
                .mapToInt(o -> ((Number) o.get("durationMinutes")).intValue())
                .min()
                .orElseThrow();
        int maxDuration = flights.stream()
                .mapToInt(o -> ((Number) o.get("durationMinutes")).intValue())
                .max()
                .orElseThrow();
        assertThat(maxDuration).isGreaterThan(minDuration);
    }

    @Test
    void rankTransportOptionsIsDeterministicByPriceAndDuration() throws Exception {
        List<Map<String, Object>> options = List.of(
                Map.of("id", "slow-cheap", "price", 50.0, "durationMinutes", 400),
                Map.of("id", "fast-pricey", "price", 200.0, "durationMinutes", 90),
                Map.of("id", "mid", "price", 100.0, "durationMinutes", 180));
        String optionsJson = OBJECT_MAPPER.writeValueAsString(options);

        Map<String, Object> byPrice = service.rankTransportOptions(null, optionsJson, "price");
        Map<String, Object> byDuration = service.rankTransportOptions(null, optionsJson, "duration");
        Map<String, Object> byPriceAgain = service.rankTransportOptions(null, optionsJson, "price");
        Map<String, Object> byNativeOptions = service.rankTransportOptions(options, null, "price");
        Map<String, Object> byDefaultSort = service.rankTransportOptions(options, null, null);

        @SuppressWarnings("unchecked")
        List<Map<String, Object>> priceRanked = (List<Map<String, Object>>) byPrice.get("ranked");
        @SuppressWarnings("unchecked")
        List<Map<String, Object>> durationRanked = (List<Map<String, Object>>) byDuration.get("ranked");
        @SuppressWarnings("unchecked")
        List<Map<String, Object>> priceRankedAgain = (List<Map<String, Object>>) byPriceAgain.get("ranked");
        @SuppressWarnings("unchecked")
        List<Map<String, Object>> nativeRanked = (List<Map<String, Object>>) byNativeOptions.get("ranked");

        assertThat(byPrice.get("ok")).isEqualTo(true);
        assertThat(byPrice.get("sortBy")).isEqualTo("price");
        assertThat(byDefaultSort.get("ok")).isEqualTo(true);
        assertThat(byDefaultSort.get("sortBy")).isEqualTo("price");
        assertThat(priceRanked).extracting(o -> o.get("id"))
                .containsExactly("slow-cheap", "mid", "fast-pricey");
        assertThat(durationRanked).extracting(o -> o.get("id"))
                .containsExactly("fast-pricey", "mid", "slow-cheap");
        assertThat(priceRanked).isEqualTo(priceRankedAgain);
        assertThat(nativeRanked).extracting(o -> o.get("id"))
                .containsExactly("slow-cheap", "mid", "fast-pricey");
        assertThat(priceRanked.getFirst().get("rank")).isEqualTo(1);
    }

    @Test
    void rankTransportOptionsCoercesNumericStrings() {
        List<Map<String, Object>> options = List.of(
                Map.of("id", "slow-cheap", "price", "50.0", "durationMinutes", "400"),
                Map.of("id", "fast-pricey", "price", "200", "durationMinutes", "90"),
                Map.of("id", "mid", "price", "100.5", "durationMinutes", "180"));

        Map<String, Object> byPrice = service.rankTransportOptions(options, null, "price");
        Map<String, Object> byDuration = service.rankTransportOptions(options, null, "duration");

        @SuppressWarnings("unchecked")
        List<Map<String, Object>> priceRanked = (List<Map<String, Object>>) byPrice.get("ranked");
        @SuppressWarnings("unchecked")
        List<Map<String, Object>> durationRanked = (List<Map<String, Object>>) byDuration.get("ranked");

        assertThat(byPrice.get("ok")).isEqualTo(true);
        assertThat(priceRanked).extracting(o -> o.get("id"))
                .containsExactly("slow-cheap", "mid", "fast-pricey");
        assertThat(durationRanked).extracting(o -> o.get("id"))
                .containsExactly("fast-pricey", "mid", "slow-cheap");
    }

    @Test
    void rankTransportOptionsReturnsClearErrorForInvalidJson() {
        Map<String, Object> blank = service.rankTransportOptions(null, "  ", "price");
        Map<String, Object> missing = service.rankTransportOptions(null, null, null);
        Map<String, Object> invalid = service.rankTransportOptions(null, "not-json", "duration");
        Map<String, Object> emptyArray = service.rankTransportOptions(null, "[]", "price");
        Map<String, Object> objectNotArray = service.rankTransportOptions(null, "{\"options\":[]}", "price");

        assertThat(blank.get("ok")).isEqualTo(false);
        assertThat(blank.get("ranked")).isEqualTo(List.of());
        assertThat(blank.get("error").toString()).containsIgnoringCase("blank");
        assertThat(blank.get("hint").toString()).isNotBlank();

        assertThat(missing.get("ok")).isEqualTo(false);
        assertThat(missing.get("sortBy")).isEqualTo("price");

        assertThat(invalid.get("ok")).isEqualTo(false);
        assertThat(invalid.get("error").toString()).containsIgnoringCase("parse");
        assertThat(invalid.get("hint").toString()).containsIgnoringCase("array");
        assertThat(invalid.get("rawPreview").toString()).contains("not-json");

        assertThat(emptyArray.get("ok")).isEqualTo(false);
        assertThat(emptyArray.get("error").toString()).containsIgnoringCase("empty");

        assertThat(objectNotArray.get("ok")).isEqualTo(false);
        assertThat(objectNotArray.get("ranked")).isEqualTo(List.of());
    }

    @Test
    void searchLeavesAcceptDatetimeAndFallbackOnUnparseableDates() {
        assertThatCode(() -> {
            Map<String, Object> flights = service.searchFlights(
                    "budget-nyc-weekend", "BOS", "NYC", "2026-03-14T08:00:00");
            Map<String, Object> trains = service.searchTrains(
                    "budget-nyc-weekend", "BOS", "NYC", "3/14/2026");
            Map<String, Object> hotels = service.searchHotels(
                    "budget-nyc-weekend", "NYC", "2026-03-14 09:00:00", "not-a-date", 1);

            assertThat(flights.get("date")).isEqualTo("2026-03-14");
            assertThat(trains.get("date")).isEqualTo("2026-03-14");
            assertThat(hotels.get("startDate")).isEqualTo("2026-03-14");
            assertThat(hotels.get("endDate")).isEqualTo("2026-03-16");
            assertThat((List<?>) flights.get("options")).hasSizeGreaterThanOrEqualTo(2);
            assertThat((List<?>) trains.get("options")).hasSizeGreaterThanOrEqualTo(2);
            assertThat((List<?>) hotels.get("options")).hasSizeGreaterThanOrEqualTo(2);
        }).doesNotThrowAnyException();
    }

    @Test
    void loyaltyScenarioReturnsStrongPerks() {
        Map<String, Object> perks = service.checkLoyaltyPerks("loyalty-points-max", null, null);

        assertThat(perks.get("loyaltyTier")).isEqualTo("gold");
        assertThat(perks.get("hotelChain").toString()).containsIgnoringCase("Marriott");
        @SuppressWarnings("unchecked")
        List<String> perkList = (List<String>) perks.get("perks");
        assertThat(perkList).hasSizeGreaterThanOrEqualTo(3);
        assertThat(((Number) perks.get("pointsMultiplier")).doubleValue()).isGreaterThan(1.0);
    }

    @Test
    void unknownScenarioReturnsNeutralMultiOptionDataWithoutThrowing() {
        String unknown = "unknown-key-xyz";

        assertThatCode(() -> {
            Map<String, Object> flights = service.searchFlights(unknown, null, null, null);
            Map<String, Object> trains = service.searchTrains(unknown, null, null, null);
            Map<String, Object> hotels = service.searchHotels(unknown, null, null, null, null);
            Map<String, Object> perks = service.checkLoyaltyPerks(unknown, null, null);

            List<?> flightOptions = (List<?>) flights.get("options");
            List<?> trainOptions = (List<?>) trains.get("options");
            List<?> hotelOptions = (List<?>) hotels.get("options");

            assertThat(flightOptions).hasSizeGreaterThanOrEqualTo(2);
            assertThat(trainOptions).hasSizeGreaterThanOrEqualTo(2);
            assertThat(hotelOptions).hasSizeGreaterThanOrEqualTo(2);
            assertThat(perks).isNotEmpty();
            assertThat(perks.get("perks")).isInstanceOf(List.class);
        }).doesNotThrowAnyException();
    }
}
