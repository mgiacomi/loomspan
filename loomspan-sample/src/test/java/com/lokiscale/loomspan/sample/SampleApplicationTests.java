package com.lokiscale.loomspan.sample;

import com.lokiscale.loomspan.api.SkillTemplate;
import org.junit.jupiter.api.Test;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.context.ApplicationContext;
import org.springframework.core.io.ClassPathResource;
import org.springframework.boot.test.context.SpringBootTest;

import java.io.IOException;
import java.nio.charset.StandardCharsets;
import java.util.List;
import java.util.Map;
import tools.jackson.databind.JsonNode;
import tools.jackson.databind.ObjectMapper;

import static org.assertj.core.api.Assertions.assertThat;

@SpringBootTest(classes = SampleApplication.class, webEnvironment = SpringBootTest.WebEnvironment.NONE)
class SampleApplicationTests {

    private static final ObjectMapper OBJECT_MAPPER = new ObjectMapper();

    @Autowired
    private ApplicationContext applicationContext;

    @Autowired
    private SkillTemplate skillTemplate;

    @Test
    void contextLoads() {
        assertThat(applicationContext).isNotNull();
    }

    @Test
    void loadsAllMigratedRootEvidenceContracts() throws IOException {
        assertManifestContains("skills/basics/duplicate_invoice_checker.yml", "isDuplicate", "invoiceParser and expenseLookup");
        assertManifestContains("skills/incidents/handle_incident.yml", "likelyCause", "classifyIncident and (investigateNetwork or investigateApp)");
        assertManifestContains("skills/insurance/process_claim.yml", "disposition", "assessCoverage and fraudScreen and recommendDisposition");
        assertManifestContains("skills/support/resolve_support_case.yml", "disposition", "understandIntent and (handleBilling or handleTechnical or handleHowTo) and composeReply");
        assertManifestContains("skills/travel/plan_trip.yml", "rationale", "understandPreferences and planTransport and planStay and assembleItinerary");
    }

    @Test
    void loadsSupportedSkillTemplateFacade() {
        assertThat(skillTemplate).isNotNull();
    }

    @Test
    void invokesMappedYamlSkillThroughSupportedFacade() {
        assertThat(skillTemplate.invoke("expenseLookup", Map.of()))
                .contains("Software")
                .contains("Hardware");
    }

    @Test
    void invokesRankTransportOptionsWithNativeScalarMapsThroughSkillTemplate() throws Exception
    {
        List<Map<String, Object>> options = List.of(
                Map.of("operator", "Northeast Regional", "price", 69.0, "durationMinutes", 210),
                Map.of("operator", "Acela Express", "price", 149.0, "durationMinutes", 165),
                Map.of("operator", "Scenic Coach", "price", 189.0, "durationMinutes", 360));

        String result = skillTemplate.invoke("rankTransportOptions", Map.of(
                "options", options,
                "sortBy", "price"));

        JsonNode response = OBJECT_MAPPER.readTree(result);
        assertThat(response.path("ok").asBoolean()).isTrue();
        assertThat(response.path("ranked").get(0).path("operator").asText()).isEqualTo("Northeast Regional");
        assertThat(response.path("ranked").get(1).path("operator").asText()).isEqualTo("Acela Express");
        assertThat(response.path("ranked").get(2).path("operator").asText()).isEqualTo("Scenic Coach");
        assertThat(response.path("ranked").get(0).path("price").isFloatingPointNumber()).isTrue();
        assertThat(response.path("ranked").get(0).path("durationMinutes").isIntegralNumber()).isTrue();
    }

    private static void assertManifestContains(String path, String property, String expression) throws IOException {
        assertThat(new ClassPathResource(path).getContentAsString(StandardCharsets.UTF_8))
                .contains("    " + property + ":")
                .contains("      evidence: " + expression);
    }
}
