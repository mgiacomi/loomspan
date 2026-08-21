package com.lokiscale.loomspan.internal.runtime.input;

import tools.jackson.databind.JsonNode;
import tools.jackson.databind.ObjectMapper;
import com.lokiscale.loomspan.internal.skill.YamlSkillManifest;
import org.junit.jupiter.api.Test;

import java.util.List;
import java.util.Map;

import static org.assertj.core.api.Assertions.assertThat;

class SkillInputContractResolverTest
{
    private final SkillInputContractResolver resolver = new SkillInputContractResolver();
    private final ObjectMapper objectMapper = new ObjectMapper();

    @Test
    void preservesAttachmentMetadataInInputContractAndJsonSchema() throws Exception
    {
        SkillInputSchemaNode schema = resolver.fromManifest(attachmentInputManifest());

        SkillInputSchemaNode image = schema.properties().get("image");
        assertThat(image.isAttachment()).isTrue();
        assertThat(image.attachmentMediaType()).isEqualTo("image");
        assertThat(image.allowedContentTypes()).containsExactly("image/jpeg");

        String jsonSchema = resolver.toJsonSchema(
                new SkillInputContract(SkillInputContract.SkillInputContractKind.YAML_EXPLICIT, schema));
        JsonNode imageNode = objectMapper.readTree(jsonSchema).path("properties").path("image");
        assertThat(imageNode.path("type").asText()).isEqualTo("string");
        assertThat(imageNode.path("x-loomspan-attachment").asBoolean()).isTrue();
        assertThat(imageNode.path("x-loomspan-media-type").asText()).isEqualTo("image");
        assertThat(imageNode.path("x-loomspan-allowed-content-types").get(0).asText()).isEqualTo("image/jpeg");
    }

    @Test
    void preservesUnconstrainedNodesAcrossJsonSchemaRoundTrip() throws Exception
    {
        SkillInputSchemaNode schema = resolver.fromJsonSchema("""
                {
                  "type": "object",
                  "properties": {
                    "value": {
                      "description": "Any caller-supplied JSON value",
                      "x-loomspan-runtime-ref-capable": true
                    },
                    "values": {
                      "type": "object",
                      "additionalProperties": {}
                    },
                    "items": {
                      "type": "array",
                      "items": {}
                    }
                  },
                  "additionalProperties": false
                }
                """);

        SkillInputSchemaNode value = schema.properties().get("value");
        assertThat(value.isUnconstrained()).isTrue();
        assertThat(value.description()).isEqualTo("Any caller-supplied JSON value");
        assertThat(value.runtimeRefCapable()).isTrue();
        assertThat(schema.properties().get("values").additionalPropertiesSchema().isUnconstrained()).isTrue();
        assertThat(schema.properties().get("items").items().isUnconstrained()).isTrue();

        String serialized = resolver.toJsonSchema(
                new SkillInputContract(SkillInputContract.SkillInputContractKind.JAVA_REFLECTED, schema));
        JsonNode roundTrip = objectMapper.readTree(serialized);
        assertThat(roundTrip.path("properties").path("value").has("type")).isFalse();
        assertThat(roundTrip.path("properties").path("value").path("description").asText())
                .isEqualTo("Any caller-supplied JSON value");
        assertThat(roundTrip.path("properties").path("values").path("additionalProperties").size()).isZero();
        assertThat(roundTrip.path("properties").path("items").path("items").size()).isZero();
        assertThat(resolver.fromJsonSchema(serialized)).isEqualTo(schema);
    }

    @Test
    void keepsGenericTopLevelAndStrictEmptyObjectContractsDistinctFromAny()
    {
        SkillInputContract generic = resolver.resolveFromToolSchema("{\"type\":\"object\"}");
        SkillInputContract strict = resolver.resolveFromToolSchema(
                "{\"type\":\"object\",\"additionalProperties\":false}");
        SkillInputContract any = resolver.resolveFromToolSchema("{}");

        assertThat(generic.isGeneric()).isTrue();
        assertThat(strict.isGeneric()).isFalse();
        assertThat(strict.schema().isObject()).isTrue();
        assertThat(strict.schema().allowsAdditionalProperties()).isFalse();
        assertThat(any.isGeneric()).isFalse();
        assertThat(any.schema().isUnconstrained()).isTrue();
    }

    static YamlSkillManifest.InputSchemaManifest attachmentInputManifest()
    {
        YamlSkillManifest.InputSchemaManifest root = new YamlSkillManifest.InputSchemaManifest();
        root.setType("object");
        root.setRequired(List.of("image"));
        root.setAdditionalProperties(false);

        YamlSkillManifest.InputSchemaManifest image = new YamlSkillManifest.InputSchemaManifest();
        image.setType("attachment");
        image.setMediaType("image");
        image.setAllowedContentTypes(List.of("image/jpeg"));
        image.setDescription("Ticket image");
        root.setProperties(Map.of("image", image));
        return root;
    }
}
