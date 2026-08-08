package com.lokiscale.loomspan.internal.runtime.attachment;

import com.lokiscale.loomspan.autoconfigure.AiDriver;
import com.lokiscale.loomspan.internal.core.LoomspanSession;
import com.lokiscale.loomspan.internal.runtime.input.SkillInputContractResolver;
import com.lokiscale.loomspan.internal.skill.EffectiveSkillExecutionConfiguration;
import com.lokiscale.loomspan.internal.skill.YamlSkillDefinition;
import com.lokiscale.loomspan.internal.skill.YamlSkillManifest;
import com.lokiscale.loomspan.internal.vfs.DefaultRefResolver;
import com.lokiscale.loomspan.internal.vfs.SessionLocalVirtualFileSystem;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.io.TempDir;
import org.springframework.core.io.ByteArrayResource;
import org.springframework.core.io.FileSystemResource;
import org.springframework.core.io.InputStreamResource;
import org.springframework.util.unit.DataSize;

import java.io.ByteArrayInputStream;
import java.nio.file.Files;
import java.nio.file.Path;
import java.util.Map;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;

class DefaultMissionInputMaterializerTest
{
    @TempDir
    Path tempDir;

    @Test
    void materializesRefAttachmentToDescriptorAndResource() throws Exception
    {
        LoomspanSession session = new LoomspanSession(8, "test.entry");
        Path file = tempDir.resolve(session.getSessionId()).resolve("forms").resolve("ticket.jpg");
        Files.createDirectories(file.getParent());
        Files.write(file, jpegBytes());
        DefaultMissionInputMaterializer materializer = materializer(DataSize.ofMegabytes(1));

        RenderedMissionInput rendered = materializer.materialize(
                session,
                definition(),
                "Extract ticket",
                Map.of("image", "ref://forms/ticket.jpg"));

        assertThat(rendered.attachments()).hasSize(1);
        assertThat(rendered.attachments().get(0).resource()).isInstanceOf(FileSystemResource.class);
        assertThat(rendered.traceSafeInput().get("image")).isInstanceOf(Map.class);
        assertThat(rendered.userText()).contains("\"attachment\" : true", "\"contentType\" : \"image/jpeg\"");
        assertThat(rendered.userText()).doesNotContain("image-bytes");
    }

    @Test
    void rejectsAttachmentWhenContentTypePolicyDoesNotMatch()
    {
        ByteArrayResource text = new ByteArrayResource("not image".getBytes())
        {
            @Override
            public String getFilename()
            {
                return "not-image.txt";
            }
        };

        assertThatThrownBy(() -> materializer(DataSize.ofMegabytes(1)).materialize(
                new LoomspanSession(8, "test.entry"),
                definition(),
                "Extract ticket",
                Map.of("image", text)))
                .isInstanceOf(IllegalArgumentException.class)
                .hasMessageContaining("image")
                .hasMessageContaining("image/jpeg");
    }

    @Test
    void rejectsImageAttachmentWhenFilenameAndBytesDoNotMatch()
    {
        ByteArrayResource textNamedJpeg = new ByteArrayResource("not really a jpeg".getBytes())
        {
            @Override
            public String getFilename()
            {
                return "ticket.jpg";
            }
        };

        assertThatThrownBy(() -> materializer(DataSize.ofMegabytes(1)).materialize(
                new LoomspanSession(8, "test.entry"),
                definition(),
                "Extract ticket",
                Map.of("image", textNamedJpeg)))
                .isInstanceOf(IllegalArgumentException.class)
                .hasMessageContaining("Attachment field 'image'")
                .hasMessageContaining("content does not match declared image type 'image/jpeg'");
    }

    @Test
    void rejectsOpenStreamResourceBeforeProviderCall()
    {
        InputStreamResource streamResource = new InputStreamResource(new ByteArrayInputStream(jpegBytes()))
        {
            @Override
            public String getFilename()
            {
                return "ticket.jpg";
            }
        };

        assertThatThrownBy(() -> materializer(DataSize.ofMegabytes(1)).materialize(
                new LoomspanSession(8, "test.entry"),
                definition(),
                "Extract ticket",
                Map.of("image", streamResource)))
                .isInstanceOf(IllegalArgumentException.class)
                .hasMessageContaining("repeatable Resource");
    }

    @Test
    void rejectsMissingRefWithFieldSpecificMessage()
    {
        assertThatThrownBy(() -> materializer(DataSize.ofMegabytes(1)).materialize(
                new LoomspanSession(8, "test.entry"),
                definition(),
                "Extract ticket",
                Map.of("image", "ref://forms/missing.jpg")))
                .isInstanceOf(IllegalArgumentException.class)
                .hasMessageContaining("Attachment field 'image'")
                .hasMessageContaining("ref://forms/missing.jpg");
    }

    @Test
    void rejectsAttachmentAboveConfiguredSizeLimit()
    {
        ByteArrayResource image = new ByteArrayResource(new byte[]{(byte) 0xFF, (byte) 0xD8, (byte) 0xFF, 1, 2, 3, 4, 5})
        {
            @Override
            public String getFilename()
            {
                return "ticket.jpg";
            }
        };

        assertThatThrownBy(() -> materializer(DataSize.ofBytes(3)).materialize(
                new LoomspanSession(8, "test.entry"),
                definition(),
                "Extract ticket",
                Map.of("image", image)))
                .isInstanceOf(IllegalArgumentException.class)
                .hasMessageContaining("image")
                .hasMessageContaining("exceeding configured limit");
    }

    private DefaultMissionInputMaterializer materializer(DataSize maxSize)
    {
        SessionLocalVirtualFileSystem vfs = new SessionLocalVirtualFileSystem(tempDir);
        return new DefaultMissionInputMaterializer(new DefaultRefResolver(vfs), new SkillInputContractResolver(), maxSize);
    }

    private YamlSkillDefinition definition()
    {
        YamlSkillManifest manifest = new YamlSkillManifest();
        manifest.setName("attachmentSkill");
        manifest.setDescription("Attachment skill");
        manifest.setModel("model");

        YamlSkillManifest.InputSchemaManifest root = new YamlSkillManifest.InputSchemaManifest();
        root.setType("object");
        root.setRequired(java.util.List.of("image"));
        root.setAdditionalProperties(false);
        YamlSkillManifest.InputSchemaManifest image = new YamlSkillManifest.InputSchemaManifest();
        image.setType("attachment");
        image.setMediaType("image");
        image.setAllowedContentTypes(java.util.List.of("image/jpeg"));
        root.setProperties(Map.of("image", image));
        manifest.setInputSchema(root);

        return new YamlSkillDefinition(
                new ByteArrayResource(new byte[0]),
                manifest,
                new EffectiveSkillExecutionConfiguration("model", "test-connection", AiDriver.OPENAI, "gpt-4.1-mini", null));
    }

    private byte[] jpegBytes()
    {
        return new byte[]{(byte) 0xFF, (byte) 0xD8, (byte) 0xFF, 0x45, 0x23, 0x11, (byte) 0xFF, (byte) 0xD9};
    }
}
