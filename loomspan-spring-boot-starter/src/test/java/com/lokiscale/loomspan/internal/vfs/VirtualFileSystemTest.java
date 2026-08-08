package com.lokiscale.loomspan.internal.vfs;

import com.lokiscale.loomspan.internal.core.LoomspanSession;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.io.TempDir;
import org.springframework.core.io.Resource;
import org.springframework.util.StreamUtils;

import java.io.IOException;
import java.nio.charset.StandardCharsets;
import java.nio.file.Files;
import java.nio.file.Path;
import java.util.concurrent.atomic.AtomicReference;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;
import static org.mockito.Mockito.mock;
import static org.mockito.Mockito.when;

class VirtualFileSystemTest {

    @TempDir
    Path tempDir;

    @Test
    void resolveStringBridgeDelegatesThroughTypedContract() {
        AtomicReference<VfsRef> resolvedRef = new AtomicReference<>();
        VirtualFileSystem vfs = new VirtualFileSystem() {
            @Override
            public Resource resolve(LoomspanSession session, VfsRef ref) {
                resolvedRef.set(ref);
                return new org.springframework.core.io.ByteArrayResource(ref.raw().getBytes(StandardCharsets.UTF_8));
            }
        };

        Resource resource = vfs.resolve(com.lokiscale.loomspan.internal.core.TestLoomspanSessions.withId("session-bridge", "test.entry", 2), "ref://artifacts/message.txt");

        assertThat(readUtf8(resource)).isEqualTo("ref://artifacts/message.txt");
        assertThat(resolvedRef.get()).isEqualTo(new VfsRef("ref://artifacts/message.txt", "artifacts/message.txt"));
    }

    @Test
    void isolatesRefsBySessionNamespace() throws Exception {
        SessionLocalVirtualFileSystem vfs = new SessionLocalVirtualFileSystem(tempDir);
        LoomspanSession first = com.lokiscale.loomspan.internal.core.TestLoomspanSessions.withId("session-1", "test.entry", 2);
        LoomspanSession second = com.lokiscale.loomspan.internal.core.TestLoomspanSessions.withId("session-2", "test.entry", 2);

        Path firstFile = vfs.sessionRoot(first).resolve("artifacts/message.txt");
        Files.createDirectories(firstFile.getParent());
        Files.writeString(firstFile, "hello", StandardCharsets.UTF_8);

        assertThat(vfs.resolve(first, "ref://artifacts/message.txt").getContentAsString(StandardCharsets.UTF_8))
                .isEqualTo("hello");
        assertThatThrownBy(() -> vfs.resolve(second, "ref://artifacts/message.txt"))
                .isInstanceOf(IllegalArgumentException.class)
                .hasMessageContaining("session-2");
    }

    @Test
    void preservesRawBinaryBytesWithoutTextDecoding() throws Exception {
        SessionLocalVirtualFileSystem vfs = new SessionLocalVirtualFileSystem(tempDir);
        LoomspanSession session = com.lokiscale.loomspan.internal.core.TestLoomspanSessions.withId("session-binary", "test.entry", 2);

        byte[] expected = new byte[]{0x00, 0x01, (byte) 0xFE, (byte) 0xFF, 0x41};
        Path binaryFile = vfs.sessionRoot(session).resolve("artifacts/payload.bin");
        Files.createDirectories(binaryFile.getParent());
        Files.write(binaryFile, expected);

        Resource resource = vfs.resolve(session, "ref://artifacts/payload.bin");

        assertThat(StreamUtils.copyToByteArray(resource.getInputStream())).containsExactly(expected);
    }

    @Test
    void rejectsRefsThatEscapeSessionNamespace() {
        SessionLocalVirtualFileSystem vfs = new SessionLocalVirtualFileSystem(tempDir);
        LoomspanSession session = com.lokiscale.loomspan.internal.core.TestLoomspanSessions.withId("session-escape", "test.entry", 2);

        assertThatThrownBy(() -> vfs.resolve(session, "ref://../other-session/secret.txt"))
                .isInstanceOf(IllegalArgumentException.class)
                .hasMessageContaining("escapes the session namespace");
    }

    @Test
    void rejectsInvalidOrMissingRefSyntax() {
        SessionLocalVirtualFileSystem vfs = new SessionLocalVirtualFileSystem(tempDir);
        LoomspanSession session = com.lokiscale.loomspan.internal.core.TestLoomspanSessions.withId("session-invalid", "test.entry", 2);

        assertThatThrownBy(() -> vfs.resolve(session, "artifacts/message.txt"))
                .isInstanceOf(IllegalArgumentException.class)
                .hasMessageContaining("Malformed ref");
        assertThatThrownBy(() -> vfs.resolve(session, "ref://"))
                .isInstanceOf(IllegalArgumentException.class)
                .hasMessageContaining("Malformed ref");
    }

    @Test
    void missingRefsFailWithSessionAwareMessage() {
        SessionLocalVirtualFileSystem vfs = new SessionLocalVirtualFileSystem(tempDir);
        LoomspanSession session = com.lokiscale.loomspan.internal.core.TestLoomspanSessions.withId("session-missing", "test.entry", 2);

        assertThatThrownBy(() -> vfs.resolve(session, "ref://artifacts/missing.txt"))
                .isInstanceOf(IllegalArgumentException.class)
                .hasMessageContaining("Unknown ref 'ref://artifacts/missing.txt'")
                .hasMessageContaining("session-missing");
    }

    @Test
    void traversalDefenseRemainsEnforcedByBackend() {
        SessionLocalVirtualFileSystem vfs = new SessionLocalVirtualFileSystem(tempDir);
        LoomspanSession session = com.lokiscale.loomspan.internal.core.TestLoomspanSessions.withId("session-backend-escape", "test.entry", 2);

        assertThatThrownBy(() -> vfs.resolve(session, VfsRef.parse("ref://../other-session/secret.txt")))
                .isInstanceOf(IllegalArgumentException.class)
                .hasMessageContaining("escapes the session namespace");
    }

    @Test
    void rejectsSessionIdsThatEscapeTheConfiguredVfsRoot() {
        SessionLocalVirtualFileSystem vfs = new SessionLocalVirtualFileSystem(tempDir);
        LoomspanSession session = mock(LoomspanSession.class);
        when(session.getSessionId()).thenReturn("../escaped-session");

        assertThatThrownBy(() -> vfs.resolve(session, "ref://artifacts/secret.txt"))
                .isInstanceOf(IllegalArgumentException.class)
                .hasMessageContaining("escapes the VFS root");
    }

    @Test
    void resolvesNestedPathsInsideSameSessionNamespace() throws Exception {
        SessionLocalVirtualFileSystem vfs = new SessionLocalVirtualFileSystem(tempDir);
        LoomspanSession session = com.lokiscale.loomspan.internal.core.TestLoomspanSessions.withId("session-nested", "test.entry", 2);

        Path nestedFile = vfs.sessionRoot(session).resolve("artifacts/reports/2026/summary.txt");
        Files.createDirectories(nestedFile.getParent());
        Files.writeString(nestedFile, "nested hello", StandardCharsets.UTF_8);

        Resource resource = vfs.resolve(session, "ref://artifacts/reports/2026/summary.txt");

        assertThat(readUtf8(resource)).isEqualTo("nested hello");
    }

    private String readUtf8(Resource resource) {
        try {
            return StreamUtils.copyToString(resource.getInputStream(), StandardCharsets.UTF_8);
        }
        catch (IOException ex) {
            throw new IllegalStateException(ex);
        }
    }
}
