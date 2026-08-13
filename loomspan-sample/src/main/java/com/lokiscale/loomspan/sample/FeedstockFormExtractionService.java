package com.lokiscale.loomspan.sample;

import tools.jackson.databind.JsonNode;
import tools.jackson.databind.ObjectMapper;
import tools.jackson.databind.node.ArrayNode;
import tools.jackson.databind.node.ObjectNode;
import com.lokiscale.loomspan.api.SkillMethod;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.core.io.Resource;
import org.springframework.core.io.ResourceLoader;
import org.springframework.stereotype.Service;

import java.io.IOException;
import java.io.InputStream;
import java.net.URI;
import java.net.http.HttpClient;
import java.net.http.HttpRequest;
import java.net.http.HttpResponse;
import java.time.Duration;
import java.util.Base64;
import java.util.Locale;

@Service
public class FeedstockFormExtractionService
{

    private static final String SAMPLE_IMAGE = "classpath:/forms/feedstock-p1.jpg";
    private static final Duration OPENAI_REQUEST_TIMEOUT = Duration.ofMinutes(3);
    private static final HttpClient HTTP_CLIENT = HttpClient.newBuilder().connectTimeout(Duration.ofSeconds(15)).build();

    private static final String PROMPT = """
            Act as a high-precision document parser for logistics weighmaster certificates.

            CRITICAL GUIDELINES:

            Handwriting vs. Print: Fields in the 'Parties' section are handwritten in ink. Use context to decode cursive.

            ### EXTRACTION GOALS:
            1. IDENTIFY THE TAG: Search the entire document for a white sticker containing a 'ZHCBZ'+ number. It should be 11 digits total. This is the primary tracking ID.
            2. RED TICKET ID: Extract the red printed number in the top right.
            3. SCALE STAMPS: Extract Gross, Tare, and Net weights from the dot-matrix scale stamps on the right.
            4. HANDWRITTEN DATA: Transcribe the ink pen for parties and totals.

            ### LOGIC & VALIDATION:
            - MATH CHECK: You MUST verify: Gross - Tare = Net. If they do not match, re-examine the scale stamps to find the most visually plausible digits that satisfy the equation.
            - DO NOT GUESS: If a field is missing, illegible, or you are not confident, return null for that field. Never invent values to satisfy the schema. Returning null is strongly preferred over a guess.
            - DATE/TIME FORMAT: Return datetime_in and datetime_out as local datetimes in ISO-8601 format: yyyy-MM-dd'T'HH:mm:ss. If the stamp has a two-digit year, infer 20xx.
            """;

    private static final String SCHEMA = """
            {
              "name": "weight_ticket",
              "strict": true,
              "schema": {
                "type": "object",
                "properties": {
                  "zhcbz_tag": { "type": ["string", "null"], "description": "The ZHCBZ number from the barcode sticker (e.g., ZHCBZ005552). Null if not legible/present." },
                  "ticket_no": { "type": ["string", "null"], "description": "The red printed number in the upper right. Null if not legible/present." },
                  "datetime_in": { "type": ["string", "null"], "description": "Date and time from the INBOUND scale stamp as yyyy-MM-dd'T'HH:mm:ss. Infer 20xx for two-digit years. Null if not legible/present." },
                  "datetime_out": { "type": ["string", "null"], "description": "Date and time from the OUTBOUND scale stamp as yyyy-MM-dd'T'HH:mm:ss. Infer 20xx for two-digit years. Null if not legible/present." },
                  "gross_weight": { "type": ["number", "null"], "description": "Gross weight from the printed scale stamp. Null if not legible/present." },
                  "tare_weight": { "type": ["number", "null"], "description": "Tare weight from the printed scale stamp. Null if not legible/present." },
                  "net_weight": { "type": ["number", "null"], "description": "Net weight from the scale stamp. Verify: Gross - Tare = Net. Null if not legible/present." },
                  "driver_name": { "type": ["string", "null"], "description": "Handwritten name in ink. Null if not legible/present." },
                  "truck_no": { "type": ["string", "null"], "description": "Handwritten truck identifier. Null if not legible/present." },
                  "carrier_name": { "type": ["string", "null"], "description": "Handwritten company/carrier name. Null if not legible/present." },
                  "notes": { "type": ["string", "null"], "description": "Any additional handwritten notes or codes like 'Chips 6997'. Null if none." },
                  "cumulative_total": { "type": ["number", "null"], "description": "Handwritten cumulative value. Null if blank or not legible." },
                  "total_tons": { "type": ["number", "null"], "description": "Handwritten total tons value. Null if blank or not legible." }
                },
                "required": [
                  "zhcbz_tag", "ticket_no", "datetime_in", "datetime_out",
                  "gross_weight", "tare_weight", "net_weight",
                  "driver_name", "truck_no", "carrier_name",
                  "notes", "cumulative_total", "total_tons"
                ],
                "additionalProperties": false
              }
            }
            """;

    private final ResourceLoader resourceLoader;
    private final ObjectMapper objectMapper;
    private final String apiKey;
    private final String model;

    public FeedstockFormExtractionService(ResourceLoader resourceLoader, ObjectMapper objectMapper,
            @Value("${openai.api.key:${OPENAI_API_KEY:}}") String apiKey,
            @Value("${sample.feedstock.extraction.model:gpt-5-mini}") String model)
    {
        this.resourceLoader = resourceLoader;
        this.objectMapper = objectMapper;
        this.apiKey = apiKey;
        this.model = model;
    }

    @SkillMethod(description = "Extracts structured weigh-ticket fields from the sample feedstock image.")
    public JsonNode extractSampleFeedstockTicket()
    {
        if (apiKey == null || apiKey.isBlank())
        {
            throw new IllegalStateException("Set openai.api.key or OPENAI_API_KEY before running feedstock extraction.");
        }

        try
        {
            Resource image = resourceLoader.getResource(SAMPLE_IMAGE);
            byte[] imageBytes;
            try (InputStream inputStream = image.getInputStream())
            {
                imageBytes = inputStream.readAllBytes();
            }
            String dataUrl = "data:" + imageMimeType(image.getFilename()) + ";base64,"
                    + Base64.getEncoder().encodeToString(imageBytes);

            ObjectNode format = (ObjectNode) objectMapper.readTree(SCHEMA);
            format.put("type", "json_schema");

            ArrayNode content = objectMapper.createArrayNode()
                    .add(objectMapper.createObjectNode()
                            .put("type", "input_text")
                            .put("text", PROMPT))
                    .add(objectMapper.createObjectNode()
                            .put("type", "input_image")
                            .put("image_url", dataUrl));

            ObjectNode userMessage = objectMapper.createObjectNode()
                    .put("role", "user")
                    .set("content", content);

            ObjectNode payload = objectMapper.createObjectNode()
                    .put("model", model)
                    .set("input", objectMapper.createArrayNode().add(userMessage));

            payload.set("text", objectMapper.createObjectNode().set("format", format));

            HttpRequest request = HttpRequest.newBuilder()
                    .uri(URI.create("https://api.openai.com/v1/responses"))
                    .header("Authorization", "Bearer " + apiKey)
                    .header("Content-Type", "application/json")
                    .timeout(OPENAI_REQUEST_TIMEOUT)
                    .POST(HttpRequest.BodyPublishers.ofString(objectMapper.writeValueAsString(payload)))
                    .build();

            HttpResponse<String> response = HTTP_CLIENT.send(request, HttpResponse.BodyHandlers.ofString());
            
            if (response.statusCode() != 200)
            {
                throw new IllegalStateException("OpenAI Responses API HTTP " + response.statusCode() + ": " + response.body());
            }

            return objectMapper.readTree(extractOutputText(response.body()));
        }
        catch (InterruptedException ex)
        {
            Thread.currentThread().interrupt();
            throw new IllegalStateException("Interrupted while extracting sample feedstock ticket.", ex);
        }
        catch (IOException ex)
        {
            throw new IllegalStateException("Failed to extract sample feedstock ticket.", ex);
        }
    }

    private String extractOutputText(String responseBody) throws IOException
    {
        JsonNode output = objectMapper.readTree(responseBody).path("output");
        if (output.isArray())
        {
            for (JsonNode item : output)
            {
                JsonNode content = item.path("content");
                if (!content.isArray())
                {
                    continue;
                }
                for (JsonNode contentItem : content)
                {
                    if ("output_text".equals(contentItem.path("type").asText()))
                    {
                        return contentItem.path("text").asText();
                    }
                }
            }
        }
        throw new IllegalStateException("No output_text found in Responses API result.");
    }

    private static String imageMimeType(String filename)
    {
        String lower = filename == null ? "" : filename.toLowerCase(Locale.ROOT);
        if (lower.endsWith(".jpg") || lower.endsWith(".jpeg"))
        {
            return "image/jpeg";
        }
        if (lower.endsWith(".png"))
        {
            return "image/png";
        }
        if (lower.endsWith(".gif"))
        {
            return "image/gif";
        }
        if (lower.endsWith(".webp"))
        {
            return "image/webp";
        }
        throw new IllegalArgumentException("Unsupported image type for OpenAI vision input: " + filename);
    }
}
