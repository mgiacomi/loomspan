package com.lokiscale.loomspan.internal.core;

import java.io.ByteArrayOutputStream;
import java.io.PrintWriter;
import java.io.Writer;
import java.nio.charset.StandardCharsets;
import java.util.Objects;
import java.util.ArrayDeque;

final class BoundedStackTraceCapture
{
    static final int LIMIT_BYTES = 1024 * 1024;
    static final String KIND = "JAVA_STACK_TRACE";
    static final String CONTENT_TYPE = "text/plain; charset=utf-8";
    private static final String MARKER = "\n... [loomspan stack trace truncated] ...\n";

    private BoundedStackTraceCapture() {}

    static TraceDiagnostic capture(Throwable failure)
    {
        Objects.requireNonNull(failure, "failure must not be null");
        try
        {
            BoundedUtf8Writer writer = new BoundedUtf8Writer(LIMIT_BYTES, MARKER);
            failure.printStackTrace(new PrintWriter(writer, true));
            return new TraceDiagnostic(KIND, CONTENT_TYPE, writer.text(), writer.truncated(), LIMIT_BYTES);
        }
        catch (RuntimeException | Error captureFailure)
        {
            String fallback = failure.getClass().getName()
                    + ": stack trace capture failed ("
                    + captureFailure.getClass().getName()
                    + ")\n";
            return new TraceDiagnostic(KIND, CONTENT_TYPE, fallback, true, LIMIT_BYTES);
        }
    }

    private static final class BoundedUtf8Writer extends Writer
    {
        private final int headLimit;
        private final int tailLimit;
        private final byte[] marker;
        private final ByteArrayOutputStream head;
        private final ArrayDeque<byte[]> tail = new ArrayDeque<>();
        private int tailSize;
        private long totalBytes;
        private char pendingHighSurrogate;
        private boolean pendingCarriageReturn;

        private BoundedUtf8Writer(int limit, String markerText)
        {
            marker = markerText.getBytes(StandardCharsets.UTF_8);
            int contentBudget = limit - marker.length;
            headLimit = contentBudget * 3 / 4;
            tailLimit = contentBudget - headLimit;
            head = new ByteArrayOutputStream(headLimit);
        }

        @Override public void write(char[] chars, int offset, int length)
        {
            for (int i = offset; i < offset + length; i++) accept(chars[i]);
        }

        private void accept(char ch)
        {
            if (pendingCarriageReturn)
            {
                append(new byte[] { '\n' });
                pendingCarriageReturn = false;
                if (ch == '\n') return;
            }
            if (ch == '\r')
            {
                pendingCarriageReturn = true;
                return;
            }
            if (pendingHighSurrogate != 0)
            {
                if (Character.isLowSurrogate(ch))
                {
                    append(new String(new char[] { pendingHighSurrogate, ch }).getBytes(StandardCharsets.UTF_8));
                    pendingHighSurrogate = 0;
                    return;
                }
                append("�".getBytes(StandardCharsets.UTF_8));
                pendingHighSurrogate = 0;
            }
            if (Character.isHighSurrogate(ch)) pendingHighSurrogate = ch;
            else append(String.valueOf(ch).getBytes(StandardCharsets.UTF_8));
        }

        private void append(byte[] bytes)
        {
            totalBytes += bytes.length;
            if (head.size() + bytes.length <= headLimit)
            {
                head.writeBytes(bytes);
                return;
            }
            if (bytes.length > tailLimit) return;
            while (tailSize + bytes.length > tailLimit && !tail.isEmpty())
            {
                tailSize -= tail.removeFirst().length;
            }
            tail.addLast(bytes);
            tailSize += bytes.length;
        }

        boolean truncated() { flushPending(); return totalBytes > headLimit + tailLimit; }

        String text()
        {
            flushPending();
            if (!truncated())
            {
                byte[] all = new byte[head.size() + tailSize];
                System.arraycopy(head.toByteArray(), 0, all, 0, head.size());
                copyTail(all, head.size());
                return new String(all, StandardCharsets.UTF_8);
            }
            byte[] all = new byte[head.size() + marker.length + tailSize];
            System.arraycopy(head.toByteArray(), 0, all, 0, head.size());
            System.arraycopy(marker, 0, all, head.size(), marker.length);
            copyTail(all, head.size() + marker.length);
            return new String(all, StandardCharsets.UTF_8);
        }

        private void copyTail(byte[] destination, int offset)
        {
            for (byte[] bytes : tail)
            {
                System.arraycopy(bytes, 0, destination, offset, bytes.length);
                offset += bytes.length;
            }
        }

        private void flushPending()
        {
            if (pendingCarriageReturn)
            {
                append(new byte[] { '\n' });
                pendingCarriageReturn = false;
            }
            if (pendingHighSurrogate != 0)
            {
                append("�".getBytes(StandardCharsets.UTF_8));
                pendingHighSurrogate = 0;
            }
        }

        @Override public void flush() {}
        @Override public void close() { flushPending(); }
    }
}
