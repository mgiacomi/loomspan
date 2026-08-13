package com.lokiscale.loomspan.internal.observability.web;

import tools.jackson.databind.ObjectMapper;
import org.junit.jupiter.api.Test;

import java.util.List;
import java.util.Map;
import java.util.concurrent.atomic.AtomicInteger;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;

class BoundedJsonPageWriterTest
{
    private final ObjectMapper mapper = new ObjectMapper();
    private final BoundedJsonPageWriter writer = new BoundedJsonPageWriter(new ObservabilityJsonCodec());

    @Test
    void acceptsDefaultMinimumAndMaximumAndRejectsInvalidPageSizes()
    {
        assertThat(writer.pageSize(null)).isEqualTo(1000);
        assertThat(writer.pageSize("1")).isEqualTo(1);
        assertThat(writer.pageSize("5000")).isEqualTo(5000);
        for (String invalid : List.of("0", "-1", "5001", "999999999999999", "one"))
        {
            assertThatThrownBy(() -> writer.pageSize(invalid))
                    .isInstanceOf(ObservabilityException.class);
        }
    }

    @Test
    void writesTheExactMeasuredWholeItemJson()
            throws Exception
    {
        byte[] bytes = writer.write(List.of("å", "two"), 1,
                emitted -> Map.of("items", emitted, "hasMore", true));
        assertThat(bytes.length).isLessThanOrEqualTo(BoundedJsonPageWriter.MAX_RESPONSE_BYTES);
        assertThat(mapper.readTree(bytes).get("items")).hasSize(1);
        assertThat(new String(bytes, java.nio.charset.StandardCharsets.UTF_8)).contains("å");
    }

    @Test
    void oneOversizedItemReturnsLimitExceeded()
    {
        String oversized = "x".repeat(BoundedJsonPageWriter.MAX_RESPONSE_BYTES);
        assertThatThrownBy(() -> writer.write(List.of(oversized), 1, emitted -> Map.of("items", emitted)))
                .isInstanceOf(ObservabilityException.class)
                .extracting(failure -> ((ObservabilityException) failure).problem().code())
                .isEqualTo(ObservabilityProblem.Code.LIMIT_EXCEEDED);
    }

    @Test
    void trimsLargePagesWithLogarithmicSerializationWork() throws Exception
    {
        List<String> fetched = java.util.Collections.nCopies(5000, "x".repeat(4096));
        AtomicInteger serializations = new AtomicInteger();

        byte[] bytes = writer.write(fetched, 5000, emitted ->
        {
            serializations.incrementAndGet();
            return Map.of("items", emitted);
        });

        assertThat(bytes.length).isLessThanOrEqualTo(BoundedJsonPageWriter.MAX_RESPONSE_BYTES);
        assertThat(mapper.readTree(bytes).get("items").size()).isBetween(1, 4999);
        assertThat(serializations).hasValueLessThan(30);
    }
}
