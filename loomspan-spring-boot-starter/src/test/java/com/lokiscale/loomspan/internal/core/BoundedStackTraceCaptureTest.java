package com.lokiscale.loomspan.internal.core;

import org.junit.jupiter.api.Test;
import java.nio.charset.StandardCharsets;
import static org.assertj.core.api.Assertions.assertThat;

class BoundedStackTraceCaptureTest
{
    @Test void capturesOrdinaryStackMessageCauseAndSuppressedText()
    {
        IllegalArgumentException cause = new IllegalArgumentException("cause-message");
        IllegalStateException failure = new IllegalStateException("failure-message", cause);
        failure.addSuppressed(new RuntimeException("suppressed-message"));
        TraceDiagnostic diagnostic = BoundedStackTraceCapture.capture(failure);
        assertThat(diagnostic.kind()).isEqualTo("JAVA_STACK_TRACE");
        assertThat(diagnostic.text()).contains("failure-message", "Caused by:", "cause-message", "Suppressed:", "suppressed-message");
        assertThat(diagnostic.text()).doesNotContain("\r");
        assertThat(diagnostic.truncated()).isFalse();
    }

    @Test void retainsUtf8SafeHeadAndTailWithinOneMiB()
    {
        String message = "head-😀-" + "é".repeat(700_000) + "-tail-root-cause";
        TraceDiagnostic diagnostic = BoundedStackTraceCapture.capture(new IllegalStateException(message));
        byte[] bytes = diagnostic.text().getBytes(StandardCharsets.UTF_8);
        assertThat(diagnostic.truncated()).isTrue();
        assertThat(bytes.length).isLessThanOrEqualTo(BoundedStackTraceCapture.LIMIT_BYTES);
        assertThat(diagnostic.text()).contains("loomspan stack trace truncated", "tail-root-cause").doesNotContain("�");
    }

    @Test void captureFailureProducesBoundedFallbackWithoutEscaping()
    {
        IllegalStateException failure = new IllegalStateException("application failure")
        {
            @Override public void printStackTrace(java.io.PrintWriter writer)
            {
                throw new IllegalArgumentException("renderer failed");
            }
        };

        TraceDiagnostic diagnostic = BoundedStackTraceCapture.capture(failure);

        assertThat(diagnostic.text())
                .contains(failure.getClass().getName(), "stack trace capture failed", IllegalArgumentException.class.getName());
        assertThat(diagnostic.text().getBytes(StandardCharsets.UTF_8).length)
                .isLessThanOrEqualTo(BoundedStackTraceCapture.LIMIT_BYTES);
        assertThat(diagnostic.truncated()).isTrue();
    }
}
