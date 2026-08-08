package com.lokiscale.loomspan.internal.runtime.trace;

import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.fasterxml.jackson.databind.json.JsonMapper;
import com.lokiscale.loomspan.internal.core.ExecutionFrame;
import com.lokiscale.loomspan.internal.core.OperationType;
import com.lokiscale.loomspan.internal.core.TraceCompletion;
import com.lokiscale.loomspan.internal.core.TraceFrameType;
import com.lokiscale.loomspan.internal.core.TraceOutcome;
import com.lokiscale.loomspan.internal.core.TracePersistencePolicy;
import com.lokiscale.loomspan.internal.core.TraceRecordType;
import com.lokiscale.loomspan.internal.runtime.usage.SessionUsageSnapshot;
import com.lokiscale.loomspan.internal.runtime.usage.UsagePrecision;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.MethodOrderer;
import org.junit.jupiter.api.Order;
import org.junit.jupiter.api.TestMethodOrder;
import org.junit.jupiter.api.io.TempDir;

import java.io.IOException;
import java.nio.charset.StandardCharsets;
import java.nio.file.Files;
import java.nio.file.Path;
import java.time.Clock;
import java.time.Instant;
import java.time.ZoneOffset;
import java.util.ArrayList;
import java.util.Comparator;
import java.util.LinkedHashMap;
import java.util.LinkedHashSet;
import java.util.List;
import java.util.Map;
import java.util.Set;
import java.util.concurrent.atomic.AtomicInteger;
import java.util.stream.Stream;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatCode;

@TestMethodOrder(MethodOrderer.OrderAnnotation.class)
class ConsoleTraceFixtureCorpusTest
{
    private static final ObjectMapper JSON = JsonMapper.builder().findAndAddModules().build();
    private static final Clock CLOCK = Clock.fixed(Instant.parse("2026-07-24T12:00:00Z"), ZoneOffset.UTC);
    private static final Set<String> VALID = Set.of(
            "single-attempt-success",
            "terminal-failure",
            "terminal-abort",
            "advisor-retry",
            "nested-retry-sequences",
            "validation-exhaustion",
            "unavailable-usage",
            "missing-response-usage",
            "unattributed-usage",
            "nonterminal-error-then-success",
            "chunked-payload",
            "chunked-json-payload",
            "nested-frame-usage",
            "repeated-skill-invocations",
            "incomplete-frame-duration",
            "overlapping-frame-duration");
    private static final Map<String, String> INVALID = Map.ofEntries(
            Map.entry("malformed-json", "MALFORMED_JSON"),
            Map.entry("inconsistent-identities", "INCONSISTENT_IDENTITY"),
            Map.entry("duplicate-sequence", "NON_MONOTONIC_SEQUENCE"),
            Map.entry("incomplete-chunks", "INCOMPLETE_CHUNKS"),
            Map.entry("missing-completion", "MISSING_COMPLETION"),
            Map.entry("non-final-completion", "NON_FINAL_COMPLETION"),
            Map.entry("unsupported-enum", "UNSUPPORTED_VALUE"),
            Map.entry("contradictory-usage-reconciliation", "CONTRADICTORY_USAGE"),
            Map.entry("duplicate-chunks", "INVALID_CHUNKS"),
            Map.entry("mismatched-chunks", "INVALID_CHUNKS"),
            Map.entry("out-of-order-chunks", "INVALID_CHUNKS"),
            Map.entry("invalid-frame-relationship", "INVALID_FRAME_RELATIONSHIP"),
            Map.entry("cyclic-frame-relationship", "INVALID_FRAME_RELATIONSHIP"),
            Map.entry("invalid-terminal-failure-link", "INVALID_TERMINAL_FAILURE"),
            Map.entry("inconsistent-attempt-identity", "INVALID_ATTEMPT"),
            Map.entry("negative-usage", "INVALID_USAGE"),
            Map.entry("overflowing-usage", "INVALID_USAGE"),
            Map.entry("configured-limits-missing-member", "UNSUPPORTED_VALUE"),
            Map.entry("configured-limits-unknown-member", "UNSUPPORTED_VALUE"),
            Map.entry("configured-limits-float", "UNSUPPORTED_VALUE"),
            Map.entry("configured-limits-negative", "UNSUPPORTED_VALUE"),
            Map.entry("configured-limits-overflow", "UNSUPPORTED_VALUE"),
            Map.entry("configured-limits-duplicate-member", "UNSUPPORTED_VALUE"),
            Map.entry("oversized-physical-record", "LINE_TOO_LARGE"),
            Map.entry("excessive-json-nesting", "EXCESSIVE_JSON_DEPTH"),
            Map.entry("truncated-final-input", "TRUNCATED_INPUT"));

    @TempDir
    Path temporaryDirectory;

    @Test
    @Order(1)
    void generatedCorpusMatchesCommittedFixturesByteForByte() throws Exception
    {
        Path generated = temporaryDirectory.resolve("generated");
        generate(generated);
        Path committed = fixtureRoot();
        Map<String, String> transportFixturesBefore = transportFixtures(committed);

        if (Boolean.getBoolean("loomspan.console.fixtures.regenerate"))
        {
            copyCorpus(generated, committed);
            assertThat(transportFixtures(committed))
                    .as("regeneration must not alter transport fixtures outside traces/ and expected/")
                    .isEqualTo(transportFixturesBefore);
        }

        assertThat(fileNames(committed)).containsExactlyElementsOf(fileNames(generated));
        for (String name : fileNames(generated))
        {
            assertThat(Files.readAllBytes(committed.resolve(name)))
                    .as(name)
                    .isEqualTo(Files.readAllBytes(generated.resolve(name)));
        }
    }

    @Test
    @Order(2)
    void corpusInventoryContainsEveryRequiredSemanticCase() throws Exception
    {
        Set<String> expected = new LinkedHashSet<>();
        Stream.concat(VALID.stream(), INVALID.keySet().stream()).sorted().forEach(name ->
        {
            expected.add("traces/" + name + ".ndjson");
            expected.add("expected/" + name + ".json");
        });
        assertThat(fileNames(fixtureRoot())).containsExactlyInAnyOrderElementsOf(expected);
    }

    @Test
    @Order(3)
    void validFixturesSatisfyCurrentTraceAnalysisInvariants() throws Exception
    {
        for (String name : VALID)
        {
            List<JsonNode> records = parseLines(fixtureRoot().resolve("traces").resolve(name + ".ndjson"));
            assertThat(records).isNotEmpty();
            assertThat(records).extracting(node -> node.path("sequence").asLong()).isSorted();
            assertThat(records.stream().filter(node -> "TRACE_COMPLETED".equals(node.path("recordType").asText())))
                    .hasSize(1);
            assertThat(records.getLast().path("recordType").asText()).isEqualTo("TRACE_COMPLETED");
            assertThat(records).allSatisfy(node -> assertThat(node.has("schemaVersion")).isFalse());

            JsonNode expected = JSON.readTree(fixtureRoot().resolve("expected").resolve(name + ".json").toFile());
            Map<String, List<Integer>> sequenceAttempts = new LinkedHashMap<>();
            Map<String, List<String>> attemptLifecycle = new LinkedHashMap<>();
            List<Map<String, Object>> actualAttempts = new ArrayList<>();
            Usage attributedUsage = Usage.ZERO;
            Usage unframedAttributedUsage = Usage.ZERO;
            for (JsonNode node : records)
            {
                if (!node.path("recordType").asText().startsWith("MODEL_"))
                {
                    continue;
                }
                String attemptId = node.path("metadata").path("attemptId").asText();
                assertThat(attemptId).isNotBlank();
                attemptLifecycle.computeIfAbsent(attemptId, ignored -> new ArrayList<>())
                        .add(node.path("recordType").asText());
                if (!"MODEL_RESPONSE_RECEIVED".equals(node.path("recordType").asText()))
                {
                    continue;
                }
                String retrySequenceId = node.path("metadata").path("retrySequenceId").asText();
                sequenceAttempts.computeIfAbsent(retrySequenceId, ignored -> new ArrayList<>())
                        .add(node.path("metadata").path("attemptNumber").asInt());
                Usage responseUsage = usageFrom(node.at("/metadata/usage"));
                boolean responseUsageComplete = node.at("/metadata/usage").isObject()
                        && !"UNAVAILABLE".equals(node.at("/metadata/usage/precision").asText());
                actualAttempts.add(attemptResult(
                        retrySequenceId,
                        attemptId,
                        node.path("metadata").path("attemptNumber").asInt(),
                        responseUsage,
                        responseUsageComplete));
                attributedUsage = attributedUsage.plus(responseUsage);
                if (node.path("frameId").isNull())
                {
                    unframedAttributedUsage = unframedAttributedUsage.plus(responseUsage);
                }
            }
            sequenceAttempts.values().forEach(numbers -> assertThat(numbers).containsExactlyElementsOf(
                    Stream.iterate(1, number -> number + 1).limit(numbers.size()).toList()));
            attemptLifecycle.values().forEach(recordTypes -> assertThat(recordTypes).containsExactly(
                    TraceRecordType.MODEL_REQUEST_PREPARED.name(),
                    TraceRecordType.MODEL_REQUEST_SENT.name(),
                    TraceRecordType.MODEL_RESPONSE_RECEIVED.name()));

            assertThat(JSON.<JsonNode>valueToTree(actualAttempts)).isEqualTo(expected.path("attempts"));
            assertThat(JSON.<JsonNode>valueToTree(retryResults(actualAttempts))).isEqualTo(expected.path("retries"));
            assertThat(JSON.<JsonNode>valueToTree(attributedUsage.asMap())).isEqualTo(expected.path("attributedUsage"));
            assertThat(JSON.<JsonNode>valueToTree(unframedAttributedUsage.asMap())).isEqualTo(expected.path("unframedAttributedUsage"));
            assertThat(actualAttempts.stream().allMatch(attempt -> (Boolean) attempt.get("usageComplete")))
                    .isEqualTo(expected.path("usageComplete").asBoolean());
            Usage terminalUsage = usageFrom(records.getLast().at("/metadata/sessionUsageSnapshot"));
            assertThat(JSON.<JsonNode>valueToTree(terminalUsage.asMap())).isEqualTo(expected.path("terminalUsage"));
            assertThat(JSON.<JsonNode>valueToTree(terminalUsage.minus(attributedUsage).asMap()))
                    .isEqualTo(expected.path("unattributedUsage"));
            List<Map<String, Object>> actualValidationLinks = new ArrayList<>();
            records.stream().filter(node -> node.path("recordType").asText().startsWith("ADVISOR_"))
                    .forEach(node -> actualValidationLinks.add(expectedValidationLink(
                            node.at("/metadata/status").asText(),
                            node.at("/metadata/retrySequenceId").asText(),
                            node.at("/metadata/attemptId").asText(),
                            node.at("/metadata/attemptNumber").asInt())));
            JsonNode actualValidationLinksNode = JSON.<JsonNode>valueToTree(actualValidationLinks);
            assertThat(actualValidationLinksNode).isEqualTo(expected.path("validationLinks"));
            assertFrameSemantics(records, expected);
            assertPayloadSemantics(records, expected);
            assertThat(expected.has("ui")).isFalse();
            assertThat(expected.has("mcp")).isFalse();
        }
    }

    private static void assertFrameSemantics(List<JsonNode> records, JsonNode expected)
    {
        Map<String, JsonNode> opened = new LinkedHashMap<>();
        Map<String, JsonNode> closed = new LinkedHashMap<>();
        Map<String, String> parents = new LinkedHashMap<>();
        Map<String, Usage> directUsage = new LinkedHashMap<>();
        for (JsonNode record : records)
        {
            String type = record.path("recordType").asText();
            if ("FRAME_OPENED".equals(type))
            {
                String frameId = record.path("frameId").asText();
                assertThat(opened.put(frameId, record)).isNull();
                assertThat(frameId).isNotBlank();
                String parentFrameId = nullableText(record.path("parentFrameId"));
                assertThat(parentFrameId).isNotEqualTo(frameId);
                parents.put(frameId, parentFrameId);
            }
            if ("FRAME_CLOSED".equals(type))
            {
                String frameId = record.path("frameId").asText();
                JsonNode open = opened.get(frameId);
                assertThat(open).isNotNull();
                assertThat(record.path("parentFrameId")).isEqualTo(open.path("parentFrameId"));
                assertThat(record.path("frameType")).isEqualTo(open.path("frameType"));
                assertThat(closed.put(frameId, record)).isNull();
            }
            if ("MODEL_RESPONSE_RECEIVED".equals(type) && !record.path("frameId").isNull())
            {
                String frameId = record.path("frameId").asText();
                directUsage.merge(frameId, usageFrom(record.at("/metadata/usage")), Usage::plus);
            }
        }
        List<Map<String, Object>> actualFrames = new ArrayList<>();
        List<Map<String, Object>> gaps = new ArrayList<>();
        List<Map<String, Object>> uncertainties = new ArrayList<>();
        for (Map.Entry<String, JsonNode> entry : opened.entrySet())
        {
            String frameId = entry.getKey();
            JsonNode open = entry.getValue();
            JsonNode close = closed.get(frameId);
            Integer inclusiveDurationMillis = close == null ? null : (int) (millis(close) - millis(open));
            List<String> children = parents.entrySet().stream()
                    .filter(parent -> frameId.equals(parent.getValue()))
                    .map(Map.Entry::getKey)
                    .toList();
            Integer selfDurationMillis = null;
            if (inclusiveDurationMillis != null)
            {
                List<FrameInterval> childIntervals = new ArrayList<>();
                for (String child : children)
                {
                    JsonNode childClose = closed.get(child);
                    if (childClose == null)
                    {
                        uncertainties.add(ordered("kind", "SELF_DURATION_UNAVAILABLE_INCOMPLETE_CHILD", "frameId", frameId));
                        childIntervals.clear();
                        break;
                    }
                    childIntervals.add(new FrameInterval(millis(opened.get(child)), millis(childClose)));
                }
                if (children.isEmpty())
                {
                    selfDurationMillis = inclusiveDurationMillis;
                }
                else if (!childIntervals.isEmpty())
                {
                    childIntervals.sort(Comparator.comparingLong(FrameInterval::startMillis));
                    boolean overlaps = false;
                    long childDurationMillis = 0;
                    long latestEndMillis = Long.MIN_VALUE;
                    for (FrameInterval interval : childIntervals)
                    {
                        overlaps |= interval.startMillis() < latestEndMillis;
                        latestEndMillis = Math.max(latestEndMillis, interval.endMillis());
                        childDurationMillis += interval.endMillis() - interval.startMillis();
                    }
                    if (overlaps)
                    {
                        uncertainties.add(ordered("kind", "SELF_DURATION_UNAVAILABLE_OVERLAPPING_CHILDREN", "frameId", frameId));
                    }
                    else
                    {
                        selfDurationMillis = (int) (inclusiveDurationMillis - childDurationMillis);
                    }
                }
            }
            if (close == null)
            {
                gaps.add(ordered("kind", "OPEN_FRAME_NOT_CLOSED", "frameId", frameId));
            }
            Usage descendantUsage = Usage.ZERO;
            for (String candidateFrameId : opened.keySet())
            {
                if (isDescendant(candidateFrameId, frameId, parents))
                {
                    descendantUsage = descendantUsage.plus(directUsage.getOrDefault(candidateFrameId, Usage.ZERO));
                }
            }
            Usage frameDirectUsage = directUsage.getOrDefault(frameId, Usage.ZERO);
            Map<String, Object> frameEntry = ordered(
                    "frameId", frameId,
                    "parentFrameId", nullableText(open.path("parentFrameId")),
                    "frameType", open.path("frameType").asText(),
                    "inclusiveDurationMillis", inclusiveDurationMillis,
                    "selfDurationMillis", selfDurationMillis,
                    "directUsage", frameDirectUsage.asMap(),
                    "descendantUsage", descendantUsage.asMap(),
                    "inclusiveUsage", frameDirectUsage.plus(descendantUsage).asMap());
            String route = open.path("route").asText("");
            if (!route.isEmpty())
            {
                frameEntry.put("route", route);
            }
            actualFrames.add(frameEntry);
        }
        assertThat(JSON.<JsonNode>valueToTree(actualFrames)).isEqualTo(expected.path("frames"));
        assertThat(JSON.<JsonNode>valueToTree(gaps)).isEqualTo(expected.path("gaps"));
        assertThat(JSON.<JsonNode>valueToTree(uncertainties)).isEqualTo(expected.path("uncertainties"));
    }

    private static void assertPayloadSemantics(List<JsonNode> records, JsonNode expected)
    {
        List<Map<String, Object>> payloads = new ArrayList<>();
        for (JsonNode record : records)
        {
            JsonNode metadata = record.path("metadata");
            if (metadata.path("payloadChunked").asBoolean())
            {
                payloads.add(ordered(
                        "logicalRecordSequence", record.path("sequence").asInt(),
                        "payloadId", metadata.path("payloadId").asText(),
                        "contentType", metadata.path("contentType").asText(),
                        "chunkCount", metadata.path("chunkCount").asInt()));
            }
        }
        assertThat(JSON.<JsonNode>valueToTree(payloads)).isEqualTo(expected.path("payloads"));
    }

    private static String nullableText(JsonNode node)
    {
        return node.isNull() ? null : node.asText();
    }

    private static long millis(JsonNode record)
    {
        return record.path("timestamp").decimalValue().movePointRight(3).longValueExact();
    }

    private static boolean isDescendant(String candidate, String ancestor, Map<String, String> parents)
    {
        Set<String> visited = new LinkedHashSet<>();
        String parent = parents.get(candidate);
        while (parent != null)
        {
            if (parent.equals(ancestor))
            {
                return true;
            }
            if (!visited.add(parent))
            {
                return false;
            }
            parent = parents.get(parent);
        }
        return false;
    }

    @Test
    @Order(4)
    void validFixtureUsagePrecisionValuesAreCurrentEnums() throws Exception
    {
        for (String name : VALID)
        {
            for (JsonNode record : parseLines(fixtureRoot().resolve("traces").resolve(name + ".ndjson")))
            {
                if ("MODEL_RESPONSE_RECEIVED".equals(record.path("recordType").asText())
                        && record.at("/metadata/usage").isObject())
                {
                    assertThatCode(() -> UsagePrecision.valueOf(record.at("/metadata/usage/precision").asText()))
                            .doesNotThrowAnyException();
                }
            }
        }
    }

    @Test
    @Order(5)
    void invalidFixturesHaveOneNamedExpectedClassification() throws Exception
    {
        for (Map.Entry<String, String> entry : INVALID.entrySet())
        {
            JsonNode expected = JSON.readTree(
                    fixtureRoot().resolve("expected").resolve(entry.getKey() + ".json").toFile());
            assertThat(expected.path("valid").asBoolean()).isFalse();
            assertThat(expected.path("errorCategory").asText()).isEqualTo(entry.getValue());
        }
    }

    @Test
    @Order(6)
    void expectedUsageReconcilesEveryComponentIndependently() throws Exception
    {
        for (String name : VALID)
        {
            JsonNode expected = JSON.readTree(fixtureRoot().resolve("expected").resolve(name + ".json").toFile());
            for (String component : List.of("promptUnits", "completionUnits", "totalUnits"))
            {
                assertThat(expected.at("/unattributedUsage/" + component).asInt()).isEqualTo(
                        expected.at("/terminalUsage/" + component).asInt()
                                - expected.at("/attributedUsage/" + component).asInt());
            }
        }
    }

    private static void generate(Path root) throws Exception
    {
        Files.createDirectories(root.resolve("traces"));
        Files.createDirectories(root.resolve("expected"));
        for (String name : VALID)
        {
            generateValid(root, name);
        }
        generateInvalid(root);
    }

    private static void generateValid(Path root, String name) throws Exception
    {
        Path trace = root.resolve("traces").resolve(name + ".ndjson");
        AtomicInteger ids = new AtomicInteger();
        DefaultExecutionTraceHandle handle = name.equals("single-attempt-success")
                ? new DefaultExecutionTraceHandle(
                        "trace-" + name, "session-" + name, "test.entry", trace, TracePersistencePolicy.ALWAYS,
                        CLOCK, () -> "payload-" + ids.incrementAndGet(), "fixture-thread",
                        "traces/" + name + ".ndjson", new ConfiguredLimitsSnapshot(7, 11, 3, 5, 1234))
                : new DefaultExecutionTraceHandle(
                        "trace-" + name, "session-" + name, "test.entry", trace, TracePersistencePolicy.ALWAYS,
                        CLOCK, () -> "payload-" + ids.incrementAndGet(), "fixture-thread",
                        "traces/" + name + ".ndjson");

        Usage attributed = Usage.ZERO;
        Usage terminal = Usage.ZERO;
        String outcome = "SUCCEEDED";
        String terminalFailureId = null;

        switch (name)
        {
            case "single-attempt-success" ->
            {
                appendAttempt(handle, "retry-1", "attempt-1", 1, 10, 4, "EXACT");
                attributed = terminal = new Usage(10, 4);
            }
            case "terminal-failure" ->
            {
                appendAttempt(handle, "retry-1", "attempt-1", 1, 7, 2, "EXACT");
                terminalFailureId = "failure-terminal";
                handle.append(TraceRecordType.ERROR_RECORDED,
                        ordered("failureId", terminalFailureId, "terminal", true),
                        Map.of("message", "provider failed"));
                attributed = terminal = new Usage(7, 2);
                outcome = "FAILED";
            }
            case "terminal-abort" ->
            {
                terminalFailureId = "failure-abort";
                handle.append(TraceRecordType.ERROR_RECORDED,
                        ordered("failureId", terminalFailureId, "terminal", true),
                        Map.of("message", "interrupted"));
                outcome = "ABORTED";
            }
            case "advisor-retry" ->
            {
                appendAttempt(handle, "retry-1", "attempt-1", 1, 10, 4, "EXACT");
                handle.append(TraceRecordType.ADVISOR_REQUEST_MUTATION_RECORDED,
                        attempt("retry-1", "attempt-1", 1, Map.of("status", "retrying")),
                        Map.of("validator", "linter"));
                appendAttempt(handle, "retry-1", "attempt-2", 2, 8, 3, "EXACT");
                handle.append(TraceRecordType.ADVISOR_RESPONSE_MUTATION_RECORDED,
                        attempt("retry-1", "attempt-2", 2, Map.of("status", "passed")),
                        Map.of("validator", "linter"));
                attributed = terminal = new Usage(18, 7);
            }
            case "nested-retry-sequences" ->
            {
                appendAttempt(handle, "retry-outer", "attempt-outer-1", 1, 4, 2, "HEURISTIC");
                appendAttempt(handle, "retry-inner", "attempt-inner-1", 1, 5, 1, "EXACT");
                appendAttempt(handle, "retry-inner", "attempt-inner-2", 2, 3, 1, "EXACT");
                attributed = terminal = new Usage(12, 4);
            }
            case "validation-exhaustion" ->
            {
                appendAttempt(handle, "retry-1", "attempt-1", 1, 6, 2, "EXACT");
                handle.append(TraceRecordType.ADVISOR_REQUEST_MUTATION_RECORDED,
                        attempt("retry-1", "attempt-1", 1, Map.of("status", "retrying")),
                        Map.of("validator", "output-schema"));
                appendAttempt(handle, "retry-1", "attempt-2", 2, 5, 2, "EXACT");
                handle.append(TraceRecordType.ADVISOR_RESPONSE_MUTATION_RECORDED,
                        attempt("retry-1", "attempt-2", 2, Map.of("status", "exhausted")),
                        Map.of("validator", "output-schema"));
                terminalFailureId = "failure-validation";
                handle.append(TraceRecordType.ERROR_RECORDED,
                        ordered("failureId", terminalFailureId, "terminal", true),
                        Map.of("message", "validation exhausted"));
                attributed = terminal = new Usage(11, 4);
                outcome = "FAILED";
            }
            case "unavailable-usage" -> appendAttempt(
                    handle, "retry-1", "attempt-1", 1, 0, 0, "UNAVAILABLE");
            case "missing-response-usage" -> appendAttemptWithoutUsage(
                    handle, "retry-1", "attempt-1", 1);
            case "unattributed-usage" ->
            {
                appendAttempt(handle, "retry-1", "attempt-1", 1, new Usage(10, 4, 16), "EXACT");
                attributed = new Usage(10, 4, 16);
                terminal = new Usage(13, 6, 21);
            }
            case "nonterminal-error-then-success" ->
            {
                handle.append(TraceRecordType.ERROR_RECORDED,
                        ordered("failureId", "failure-recovered", "terminal", false),
                        Map.of("message", "recoverable cleanup failure"));
                appendAttempt(handle, "retry-1", "attempt-1", 1, 5, 2, "EXACT");
                attributed = terminal = new Usage(5, 2);
            }
            case "chunked-payload" ->
            {
                handle.append(TraceRecordType.MODEL_REQUEST_PREPARED,
                        attempt("retry-1", "attempt-1", 1, Map.of()),
                        Map.of("messages", List.of("user")));
                handle.append(TraceRecordType.MODEL_REQUEST_SENT,
                        attempt("retry-1", "attempt-1", 1, Map.of()),
                        "x".repeat(5000));
                appendResponse(handle, "retry-1", "attempt-1", 1, 2, 1, "EXACT");
                attributed = terminal = new Usage(2, 1);
            }
            case "chunked-json-payload" ->
            {
                handle.append(TraceRecordType.MODEL_REQUEST_PREPARED,
                        attempt("retry-1", "attempt-1", 1, Map.of()),
                        Map.of("messages", List.of("user")));
                handle.append(TraceRecordType.MODEL_REQUEST_SENT,
                        attempt("retry-1", "attempt-1", 1, Map.of()),
                        Map.of("content", "x".repeat(5000)));
                appendResponse(handle, "retry-1", "attempt-1", 1, 2, 1, "EXACT");
                attributed = terminal = new Usage(2, 1);
            }
            case "nested-frame-usage" ->
            {
                ExecutionFrame rootFrame = frame("root", null, TraceFrameType.ROOT_MISSION, "root.skill");
                ExecutionFrame skill = frame("skill", "root", TraceFrameType.SKILL_EXECUTION, "root.skill");
                appendFrame(handle, TraceRecordType.FRAME_OPENED, rootFrame, CLOCK.instant());
                appendFrame(handle, TraceRecordType.FRAME_OPENED, skill, CLOCK.instant().plusSeconds(1));
                appendAttempt(handle, skill, "retry-framed", "attempt-framed", 1, 4, 2, "EXACT");
                appendAttempt(handle, "retry-unframed", "attempt-unframed", 1, 1, 1, "HEURISTIC");
                appendFrame(handle, TraceRecordType.FRAME_CLOSED, skill, CLOCK.instant().plusSeconds(5));
                appendFrame(handle, TraceRecordType.FRAME_CLOSED, rootFrame, CLOCK.instant().plusSeconds(8));
                attributed = terminal = new Usage(5, 3);
            }
            case "repeated-skill-invocations" ->
            {
                ExecutionFrame rootFrame = frame("root", null, TraceFrameType.ROOT_MISSION, "root.skill");
                ExecutionFrame first = frame("skill-1", "root", TraceFrameType.SKILL_EXECUTION, "root.skill");
                ExecutionFrame second = frame("skill-2", "root", TraceFrameType.SKILL_EXECUTION, "root.skill");
                appendFrame(handle, TraceRecordType.FRAME_OPENED, rootFrame, CLOCK.instant());
                appendFrame(handle, TraceRecordType.FRAME_OPENED, first, CLOCK.instant().plusSeconds(1));
                appendAttempt(handle, first, "retry-1", "attempt-1", 1, 2, 1, "EXACT");
                appendFrame(handle, TraceRecordType.FRAME_CLOSED, first, CLOCK.instant().plusSeconds(3));
                appendFrame(handle, TraceRecordType.FRAME_OPENED, second, CLOCK.instant().plusSeconds(4));
                appendAttempt(handle, second, "retry-2", "attempt-2", 1, 3, 2, "EXACT");
                appendFrame(handle, TraceRecordType.FRAME_CLOSED, second, CLOCK.instant().plusSeconds(6));
                appendFrame(handle, TraceRecordType.FRAME_CLOSED, rootFrame, CLOCK.instant().plusSeconds(7));
                attributed = terminal = new Usage(5, 3);
            }
            case "incomplete-frame-duration" ->
            {
                ExecutionFrame rootFrame = frame("root", null, TraceFrameType.ROOT_MISSION, "root.skill");
                ExecutionFrame incomplete = frame("incomplete", "root", TraceFrameType.TOOL_INVOCATION, "root.tool");
                appendFrame(handle, TraceRecordType.FRAME_OPENED, rootFrame, CLOCK.instant());
                appendFrame(handle, TraceRecordType.FRAME_OPENED, incomplete, CLOCK.instant().plusSeconds(1));
                appendFrame(handle, TraceRecordType.FRAME_CLOSED, rootFrame, CLOCK.instant().plusSeconds(4));
            }
            case "overlapping-frame-duration" ->
            {
                ExecutionFrame rootFrame = frame("root", null, TraceFrameType.ROOT_MISSION, "root.skill");
                ExecutionFrame first = frame("child-1", "root", TraceFrameType.SKILL_EXECUTION, "root.first");
                ExecutionFrame second = frame("child-2", "root", TraceFrameType.SKILL_EXECUTION, "root.second");
                appendFrame(handle, TraceRecordType.FRAME_OPENED, rootFrame, CLOCK.instant());
                appendFrame(handle, TraceRecordType.FRAME_OPENED, first, CLOCK.instant().plusSeconds(1));
                appendFrame(handle, TraceRecordType.FRAME_OPENED, second, CLOCK.instant().plusSeconds(3));
                appendFrame(handle, TraceRecordType.FRAME_CLOSED, first, CLOCK.instant().plusSeconds(5));
                appendFrame(handle, TraceRecordType.FRAME_CLOSED, second, CLOCK.instant().plusSeconds(7));
                appendFrame(handle, TraceRecordType.FRAME_CLOSED, rootFrame, CLOCK.instant().plusSeconds(8));
            }
            default -> throw new IllegalArgumentException(name);
        }

        if (terminalFailureId != null)
        {
            handle.markErrored();
        }
        Map<String, Object> completionDetails = new LinkedHashMap<>();
        completionDetails.put("outcome", outcome);
        completionDetails.put("sessionUsageSnapshot", terminal.asMap());
        handle.finalizeTrace(new TraceCompletion(
                TraceOutcome.valueOf(outcome),
                new SessionUsageSnapshot(
                        0, 0, 0, 0,
                        terminal.promptUnits(), terminal.completionUnits(), terminal.totalUnits(),
                        0, 0, 0),
                terminalFailureId,
                completionDetails));
        writeExpected(root, name, validExpected(name, outcome, terminalFailureId, attributed, terminal));
    }

    private static void appendAttempt(
            DefaultExecutionTraceHandle handle,
            String retryId,
            String attemptId,
            int number,
            int prompt,
            int completion,
            String precision) throws IOException
    {
        appendAttempt(handle, retryId, attemptId, number, new Usage(prompt, completion), precision);
    }

    private static void appendAttempt(
            DefaultExecutionTraceHandle handle,
            String retryId,
            String attemptId,
            int number,
            Usage usage,
            String precision) throws IOException
    {
        Map<String, Object> metadata = attempt(retryId, attemptId, number, Map.of());
        handle.append(TraceRecordType.MODEL_REQUEST_PREPARED, metadata, Map.of("messages", List.of("user")));
        handle.append(TraceRecordType.MODEL_REQUEST_SENT, metadata, Map.of("messages", List.of("user")));
        appendResponse(handle, retryId, attemptId, number, usage, precision);
    }

    private static void appendAttemptWithoutUsage(
            DefaultExecutionTraceHandle handle,
            String retryId,
            String attemptId,
            int number) throws IOException
    {
        Map<String, Object> metadata = attempt(retryId, attemptId, number, Map.of());
        handle.append(TraceRecordType.MODEL_REQUEST_PREPARED, metadata, Map.of("messages", List.of("user")));
        handle.append(TraceRecordType.MODEL_REQUEST_SENT, metadata, Map.of("messages", List.of("user")));
        handle.append(TraceRecordType.MODEL_RESPONSE_RECEIVED, metadata, Map.of("content", "fixture response"));
    }

    private static void appendAttempt(
            DefaultExecutionTraceHandle handle,
            ExecutionFrame frame,
            String retryId,
            String attemptId,
            int number,
            int prompt,
            int completion,
            String precision) throws IOException
    {
        Map<String, Object> metadata = attempt(retryId, attemptId, number, Map.of());
        handle.append(TraceRecordType.MODEL_REQUEST_PREPARED, frame, frame.traceFrameType(), metadata, Map.of("messages", List.of("user")));
        handle.append(TraceRecordType.MODEL_REQUEST_SENT, frame, frame.traceFrameType(), metadata, Map.of("messages", List.of("user")));
        appendResponse(handle, frame, retryId, attemptId, number, prompt, completion, precision);
    }

    private static void appendResponse(
            DefaultExecutionTraceHandle handle,
            String retryId,
            String attemptId,
            int number,
            int prompt,
            int completion,
            String precision) throws IOException
    {
        appendResponse(handle, retryId, attemptId, number, new Usage(prompt, completion), precision);
    }

    private static void appendResponse(
            DefaultExecutionTraceHandle handle,
            String retryId,
            String attemptId,
            int number,
            Usage usage,
            String precision) throws IOException
    {
        handle.append(TraceRecordType.MODEL_RESPONSE_RECEIVED,
                attempt(retryId, attemptId, number, Map.of("usage", usage(usage, precision))),
                Map.of("content", "fixture response"));
    }

    private static void appendResponse(
            DefaultExecutionTraceHandle handle,
            ExecutionFrame frame,
            String retryId,
            String attemptId,
            int number,
            int prompt,
            int completion,
            String precision) throws IOException
    {
        Map<String, Object> usage = usage(new Usage(prompt, completion), precision);
        handle.append(TraceRecordType.MODEL_RESPONSE_RECEIVED, frame, frame.traceFrameType(),
                attempt(retryId, attemptId, number, Map.of("usage", usage)),
                Map.of("content", "fixture response"));
    }

    private static ExecutionFrame frame(
            String frameId,
            String parentFrameId,
            TraceFrameType frameType,
            String route)
    {
        return new ExecutionFrame(frameId, parentFrameId, OperationType.SKILL, frameType, route, Map.of(), CLOCK.instant());
    }

    private static void appendFrame(
            DefaultExecutionTraceHandle handle,
            TraceRecordType recordType,
            ExecutionFrame frame,
            Instant timestamp) throws IOException
    {
        handle.append(recordType, frame, frame.traceFrameType(), ordered(
                "timestampOverride", timestamp.toString(),
                "skillName", frame.route()), null);
    }

    private static Map<String, Object> attempt(
            String retryId,
            String attemptId,
            int number,
            Map<String, Object> extra)
    {
        Map<String, Object> result = new LinkedHashMap<>();
        result.put("retrySequenceId", retryId);
        result.put("attemptId", attemptId);
        result.put("attemptNumber", number);
        result.putAll(extra);
        return result;
    }

    private static Map<String, Object> usage(Usage usage, String precision)
    {
        Map<String, Object> result = new LinkedHashMap<>(usage.asMap());
        result.put("precision", precision);
        return result;
    }

    private static Map<String, Object> ordered(Object... keysAndValues)
    {
        Map<String, Object> result = new LinkedHashMap<>();
        for (int index = 0; index < keysAndValues.length; index += 2)
        {
            result.put((String) keysAndValues[index], keysAndValues[index + 1]);
        }
        return result;
    }

    private static Map<String, Object> validExpected(
            String name,
            String outcome,
            String terminalFailureId,
            Usage attributed,
            Usage terminal)
    {
        Map<String, Object> result = new LinkedHashMap<>();
        result.put("case", name);
        result.put("valid", true);
        result.put("traceId", "trace-" + name);
        result.put("sessionId", "session-" + name);
        result.put("outcome", outcome);
        result.put("terminalFailureId", terminalFailureId);
        if (name.equals("single-attempt-success"))
        {
            result.put("configuredLimits", ordered(
                    "maxSkillInvocations", 7,
                    "maxToolInvocations", 11,
                    "maxLinterRetries", 3,
                    "maxModelCalls", 5,
                    "maxUsageUnits", 1234));
        }
        result.put("attributedUsage", attributed.asMap());
        result.put("terminalUsage", terminal.asMap());
        result.put("unattributedUsage", terminal.minus(attributed).asMap());
        result.put("usageComplete", usageComplete(name));
        result.put("attempts", expectedAttempts(name));
        result.put("retries", expectedRetries(name));
        result.put("validationLinks", expectedValidationLinks(name));
        result.put("frames", expectedFrames(name));
        result.put("unframedAttributedUsage", expectedUnframedUsage(name).asMap());
        result.put("payloads", expectedPayloads(name));
        result.put("gaps", expectedGaps(name));
        result.put("uncertainties", expectedUncertainties(name));
        return result;
    }

    private static List<Map<String, Object>> expectedAttempts(String name)
    {
        return switch (name)
        {
            case "advisor-retry", "validation-exhaustion" -> List.of(
                    expectedAttempt(name, "retry-1", "attempt-1", 1),
                    expectedAttempt(name, "retry-1", "attempt-2", 2));
            case "nested-retry-sequences" -> List.of(
                    expectedAttempt(name, "retry-outer", "attempt-outer-1", 1),
                    expectedAttempt(name, "retry-inner", "attempt-inner-1", 1),
                    expectedAttempt(name, "retry-inner", "attempt-inner-2", 2));
            case "terminal-abort", "incomplete-frame-duration", "overlapping-frame-duration" -> List.of();
            case "missing-response-usage" -> List.of(expectedAttempt(name, "retry-1", "attempt-1", 1));
            case "nested-frame-usage" -> List.of(
                    expectedAttempt(name, "retry-framed", "attempt-framed", 1),
                    expectedAttempt(name, "retry-unframed", "attempt-unframed", 1));
            case "repeated-skill-invocations" -> List.of(
                    expectedAttempt(name, "retry-1", "attempt-1", 1),
                    expectedAttempt(name, "retry-2", "attempt-2", 1));
            default -> List.of(expectedAttempt(name, "retry-1", "attempt-1", 1));
        };
    }

    private static boolean usageComplete(String name)
    {
        return !name.equals("unavailable-usage") && !name.equals("missing-response-usage");
    }

    private static Usage expectedAttemptUsage(String name, String attemptId)
    {
        return switch (name)
        {
            case "advisor-retry" -> attemptId.equals("attempt-1") ? new Usage(10, 4) : new Usage(8, 3);
            case "nested-retry-sequences" -> switch (attemptId)
            {
                case "attempt-outer-1" -> new Usage(4, 2);
                case "attempt-inner-1" -> new Usage(5, 1);
                default -> new Usage(3, 1);
            };
            case "validation-exhaustion" -> attemptId.equals("attempt-1") ? new Usage(6, 2) : new Usage(5, 2);
            case "unavailable-usage", "missing-response-usage" -> Usage.ZERO;
            case "unattributed-usage" -> new Usage(10, 4, 16);
            case "nonterminal-error-then-success" -> new Usage(5, 2);
            case "chunked-payload", "chunked-json-payload" -> new Usage(2, 1);
            case "nested-frame-usage" -> attemptId.equals("attempt-framed") ? new Usage(4, 2) : new Usage(1, 1);
            case "repeated-skill-invocations" -> attemptId.equals("attempt-1") ? new Usage(2, 1) : new Usage(3, 2);
            case "terminal-failure" -> new Usage(7, 2);
            default -> new Usage(10, 4);
        };
    }

    private static List<Map<String, Object>> expectedRetries(String name)
    {
        return retryResults(expectedAttempts(name));
    }

    private static List<Map<String, Object>> retryResults(List<Map<String, Object>> attempts)
    {
        Map<String, Usage> usage = new LinkedHashMap<>();
        Map<String, Boolean> complete = new LinkedHashMap<>();
        for (Map<String, Object> attempt : attempts)
        {
            String retrySequenceId = (String) attempt.get("retrySequenceId");
            Usage attemptUsage = usageFrom((Map<String, Object>) attempt.get("usage"));
            usage.merge(retrySequenceId, attemptUsage, Usage::plus);
            complete.merge(retrySequenceId, (Boolean) attempt.get("usageComplete"), (left, right) -> left && right);
        }
        List<Map<String, Object>> retries = new ArrayList<>();
        usage.forEach((retrySequenceId, retryUsage) -> retries.add(ordered(
                "retrySequenceId", retrySequenceId,
                "usage", retryUsage.asMap(),
                "usageComplete", complete.get(retrySequenceId))));
        return retries;
    }

    private static List<Map<String, Object>> expectedValidationLinks(String name)
    {
        return switch (name)
        {
            case "advisor-retry" -> List.of(
                    expectedValidationLink("retrying", "retry-1", "attempt-1", 1),
                    expectedValidationLink("passed", "retry-1", "attempt-2", 2));
            case "validation-exhaustion" -> List.of(
                    expectedValidationLink("retrying", "retry-1", "attempt-1", 1),
                    expectedValidationLink("exhausted", "retry-1", "attempt-2", 2));
            default -> List.of();
        };
    }

    private static List<Map<String, Object>> expectedFrames(String name)
    {
        return switch (name)
        {
            case "nested-frame-usage" -> List.of(
                    expectedFrame("root", null, "ROOT_MISSION", "root.skill", 8000, 4000, Usage.ZERO, new Usage(4, 2), new Usage(4, 2)),
                    expectedFrame("skill", "root", "SKILL_EXECUTION", "root.skill", 4000, 4000, new Usage(4, 2), Usage.ZERO, new Usage(4, 2)));
            case "repeated-skill-invocations" -> List.of(
                    expectedFrame("root", null, "ROOT_MISSION", "root.skill", 7000, 3000, Usage.ZERO, new Usage(5, 3), new Usage(5, 3)),
                    expectedFrame("skill-1", "root", "SKILL_EXECUTION", "root.skill", 2000, 2000, new Usage(2, 1), Usage.ZERO, new Usage(2, 1)),
                    expectedFrame("skill-2", "root", "SKILL_EXECUTION", "root.skill", 2000, 2000, new Usage(3, 2), Usage.ZERO, new Usage(3, 2)));
            case "incomplete-frame-duration" -> List.of(
                    expectedFrame("root", null, "ROOT_MISSION", "root.skill", 4000, null, Usage.ZERO, Usage.ZERO, Usage.ZERO),
                    expectedFrame("incomplete", "root", "TOOL_INVOCATION", "root.tool", null, null, Usage.ZERO, Usage.ZERO, Usage.ZERO));
            case "overlapping-frame-duration" -> List.of(
                    expectedFrame("root", null, "ROOT_MISSION", "root.skill", 8000, null, Usage.ZERO, Usage.ZERO, Usage.ZERO),
                    expectedFrame("child-1", "root", "SKILL_EXECUTION", "root.first", 4000, 4000, Usage.ZERO, Usage.ZERO, Usage.ZERO),
                    expectedFrame("child-2", "root", "SKILL_EXECUTION", "root.second", 4000, 4000, Usage.ZERO, Usage.ZERO, Usage.ZERO));
            default -> List.of();
        };
    }

    private static Map<String, Object> expectedFrame(
            String frameId,
            String parentFrameId,
            String frameType,
            String route,
            Integer inclusiveDurationMillis,
            Integer selfDurationMillis,
            Usage directUsage,
            Usage descendantUsage,
            Usage inclusiveUsage)
    {
        Map<String, Object> entry = ordered(
                "frameId", frameId,
                "parentFrameId", parentFrameId,
                "frameType", frameType,
                "inclusiveDurationMillis", inclusiveDurationMillis,
                "selfDurationMillis", selfDurationMillis,
                "directUsage", directUsage.asMap(),
                "descendantUsage", descendantUsage.asMap(),
                "inclusiveUsage", inclusiveUsage.asMap());
        if (route != null && !route.isEmpty())
        {
            entry.put("route", route);
        }
        return entry;
    }

    private static Usage expectedUnframedUsage(String name)
    {
        return switch (name)
        {
            case "single-attempt-success" -> new Usage(10, 4);
            case "terminal-failure" -> new Usage(7, 2);
            case "advisor-retry" -> new Usage(18, 7);
            case "nested-retry-sequences" -> new Usage(12, 4);
            case "validation-exhaustion" -> new Usage(11, 4);
            case "unattributed-usage" -> new Usage(10, 4, 16);
            case "nonterminal-error-then-success" -> new Usage(5, 2);
            case "chunked-payload", "chunked-json-payload" -> new Usage(2, 1);
            case "nested-frame-usage" -> new Usage(1, 1);
            default -> Usage.ZERO;
        };
    }

    private static List<Map<String, Object>> expectedPayloads(String name)
    {
        return switch (name)
        {
            case "chunked-payload" -> List.of(expectedPayload(4, "payload-1", "text/plain", 2));
            case "chunked-json-payload" -> List.of(expectedPayload(4, "payload-1", "application/json", 2));
            default -> List.of();
        };
    }

    private static Map<String, Object> expectedPayload(
            int logicalRecordSequence,
            String payloadId,
            String contentType,
            int chunkCount)
    {
        return ordered(
                "logicalRecordSequence", logicalRecordSequence,
                "payloadId", payloadId,
                "contentType", contentType,
                "chunkCount", chunkCount);
    }

    private static List<Map<String, Object>> expectedGaps(String name)
    {
        return name.equals("incomplete-frame-duration")
                ? List.of(ordered("kind", "OPEN_FRAME_NOT_CLOSED", "frameId", "incomplete"))
                : List.of();
    }

    private static List<Map<String, Object>> expectedUncertainties(String name)
    {
        return switch (name)
        {
            case "incomplete-frame-duration" -> List.of(ordered(
                    "kind", "SELF_DURATION_UNAVAILABLE_INCOMPLETE_CHILD", "frameId", "root"));
            case "overlapping-frame-duration" -> List.of(ordered(
                    "kind", "SELF_DURATION_UNAVAILABLE_OVERLAPPING_CHILDREN", "frameId", "root"));
            default -> List.of();
        };
    }

    private static Map<String, Object> expectedAttempt(
            String name,
            String retrySequenceId,
            String attemptId,
            int attemptNumber)
    {
        return attemptResult(
                retrySequenceId,
                attemptId,
                attemptNumber,
                expectedAttemptUsage(name, attemptId),
                usageComplete(name));
    }

    private static Map<String, Object> attemptResult(
            String retrySequenceId,
            String attemptId,
            int attemptNumber,
            Usage usage,
            boolean usageComplete)
    {
        return ordered(
                "retrySequenceId", retrySequenceId,
                "attemptId", attemptId,
                "attemptNumber", attemptNumber,
                "usage", usage.asMap(),
                "usageComplete", usageComplete);
    }

    private static Usage usageFrom(Map<String, Object> values)
    {
        return new Usage(
                ((Number) values.get("promptUnits")).intValue(),
                ((Number) values.get("completionUnits")).intValue(),
                ((Number) values.get("totalUnits")).intValue());
    }

    private static Usage usageFrom(JsonNode values)
    {
        return new Usage(
                values.path("promptUnits").asInt(),
                values.path("completionUnits").asInt(),
                values.path("totalUnits").asInt());
    }

    private static Map<String, Object> expectedValidationLink(
            String status,
            String retrySequenceId,
            String attemptId,
            int attemptNumber)
    {
        return ordered(
                "status", status,
                "retrySequenceId", retrySequenceId,
                "attemptId", attemptId,
                "attemptNumber", attemptNumber);
    }

    private static void generateInvalid(Path root) throws Exception
    {
        List<String> base = Files.readAllLines(
                root.resolve("traces/single-attempt-success.ndjson"), StandardCharsets.UTF_8);
        writeInvalid(root, "malformed-json", List.of("{not-json"));

        List<String> inconsistent = new ArrayList<>(base);
        inconsistent.set(1, inconsistent.get(1).replace(
                "\"sessionId\":\"session-single-attempt-success\"",
                "\"sessionId\":\"different-session\""));
        writeInvalid(root, "inconsistent-identities", inconsistent);

        List<String> duplicate = new ArrayList<>(base);
        duplicate.set(1, duplicate.get(1).replace("\"sequence\":2", "\"sequence\":1"));
        writeInvalid(root, "duplicate-sequence", duplicate);

        List<String> chunks = Files.readAllLines(root.resolve("traces/chunked-payload.ndjson"), StandardCharsets.UTF_8);
        List<String> incomplete = new ArrayList<>(chunks);
        incomplete.removeIf(line -> line.contains("\"recordType\":\"PAYLOAD_CHUNK_APPENDED\"")
                && line.contains("\"chunkIndex\":1"));
        writeInvalid(root, "incomplete-chunks", incomplete);

        writeInvalid(root, "missing-completion", base.subList(0, base.size() - 1));

        List<String> nonFinal = new ArrayList<>(base);
        JsonNode completion = JSON.readTree(nonFinal.getLast());
        long next = completion.path("sequence").asLong() + 1;
        nonFinal.add(base.getFirst()
                .replace("\"sequence\":1", "\"sequence\":" + next)
                .replace("\"recordType\":\"TRACE_STARTED\"", "\"recordType\":\"ERROR_RECORDED\""));
        writeInvalid(root, "non-final-completion", nonFinal);

        List<String> unsupported = new ArrayList<>(base);
        unsupported.set(unsupported.size() - 1,
                unsupported.getLast().replace("\"outcome\":\"SUCCEEDED\"", "\"outcome\":\"FUTURE\""));
        writeInvalid(root, "unsupported-enum", unsupported);

        List<String> contradictory = new ArrayList<>(base);
        contradictory.set(contradictory.size() - 1,
                contradictory.getLast()
                        .replace("\"promptUnits\":10", "\"promptUnits\":1")
                        .replace("\"totalUnits\":14", "\"totalUnits\":2"));
        writeInvalid(root, "contradictory-usage-reconciliation", contradictory);

        List<String> duplicateChunks = new ArrayList<>(chunks);
        replaceFirstLineContaining(duplicateChunks, "\"chunkIndex\":1", "\"chunkIndex\":1", "\"chunkIndex\":0");
        writeInvalid(root, "duplicate-chunks", duplicateChunks);

        List<String> mismatchedChunks = new ArrayList<>(chunks);
        replaceFirstLineContaining(mismatchedChunks, "\"chunkIndex\":1", "\"chunkCount\":2", "\"chunkCount\":3");
        writeInvalid(root, "mismatched-chunks", mismatchedChunks);

        List<String> outOfOrderChunks = new ArrayList<>(chunks);
        replaceFirstLineContaining(outOfOrderChunks, "\"chunkIndex\":0", "\"chunkIndex\":0", "\"chunkIndex\":1");
        replaceLastLineContaining(outOfOrderChunks, "\"chunkIndex\":1", "\"chunkIndex\":1", "\"chunkIndex\":0");
        writeInvalid(root, "out-of-order-chunks", outOfOrderChunks);

        List<String> nestedFrames = Files.readAllLines(root.resolve("traces/nested-frame-usage.ndjson"), StandardCharsets.UTF_8);
        List<String> invalidFrame = new ArrayList<>(nestedFrames);
        replaceAllLines(invalidFrame, "\"frameId\":\"root\",\"parentFrameId\":null", "\"frameId\":\"root\",\"parentFrameId\":\"root\"");
        writeInvalid(root, "invalid-frame-relationship", invalidFrame);

        List<String> cyclicFrame = new ArrayList<>(nestedFrames);
        replaceAllLines(cyclicFrame, "\"frameId\":\"root\",\"parentFrameId\":null", "\"frameId\":\"root\",\"parentFrameId\":\"skill\"");
        writeInvalid(root, "cyclic-frame-relationship", cyclicFrame);

        List<String> terminalFailure = Files.readAllLines(root.resolve("traces/terminal-failure.ndjson"), StandardCharsets.UTF_8);
        List<String> invalidTerminalFailure = new ArrayList<>(terminalFailure);
        replaceFirstLineContaining(invalidTerminalFailure, "\"recordType\":\"TRACE_COMPLETED\"", "failure-terminal", "missing-terminal-failure");
        writeInvalid(root, "invalid-terminal-failure-link", invalidTerminalFailure);

        List<String> inconsistentAttempt = new ArrayList<>(base);
        replaceFirstLineContaining(inconsistentAttempt, "\"recordType\":\"MODEL_RESPONSE_RECEIVED\"", "\"attemptId\":\"attempt-1\"", "\"attemptId\":\"attempt-other\"");
        writeInvalid(root, "inconsistent-attempt-identity", inconsistentAttempt);

        List<String> negativeUsage = new ArrayList<>(base);
        replaceFirstLineContaining(negativeUsage, "\"recordType\":\"MODEL_RESPONSE_RECEIVED\"", "\"promptUnits\":10", "\"promptUnits\":-1");
        writeInvalid(root, "negative-usage", negativeUsage);

        List<String> overflowingUsage = new ArrayList<>(base);
        replaceFirstLineContaining(overflowingUsage, "\"recordType\":\"MODEL_RESPONSE_RECEIVED\"", "\"promptUnits\":10", "\"promptUnits\":9223372036854775808");
        writeInvalid(root, "overflowing-usage", overflowingUsage);

        List<String> missingLimit = new ArrayList<>(base);
        missingLimit.set(0, missingLimit.getFirst().replace(",\"maxUsageUnits\":1234", ""));
        writeInvalid(root, "configured-limits-missing-member", missingLimit);

        List<String> unknownLimit = new ArrayList<>(base);
        unknownLimit.set(0, unknownLimit.getFirst().replace("\"maxUsageUnits\":1234", "\"maxUsageUnits\":1234,\"futureLimit\":1"));
        writeInvalid(root, "configured-limits-unknown-member", unknownLimit);

        List<String> floatLimit = new ArrayList<>(base);
        floatLimit.set(0, floatLimit.getFirst().replace("\"maxUsageUnits\":1234", "\"maxUsageUnits\":1.5"));
        writeInvalid(root, "configured-limits-float", floatLimit);

        List<String> negativeLimit = new ArrayList<>(base);
        negativeLimit.set(0, negativeLimit.getFirst().replace("\"maxUsageUnits\":1234", "\"maxUsageUnits\":-1"));
        writeInvalid(root, "configured-limits-negative", negativeLimit);

        List<String> overflowLimit = new ArrayList<>(base);
        overflowLimit.set(0, overflowLimit.getFirst().replace("\"maxUsageUnits\":1234", "\"maxUsageUnits\":2147483648"));
        writeInvalid(root, "configured-limits-overflow", overflowLimit);

        List<String> duplicateLimit = new ArrayList<>(base);
        duplicateLimit.set(0, duplicateLimit.getFirst().replace("\"maxUsageUnits\":1234", "\"maxUsageUnits\":1234,\"maxUsageUnits\":1234"));
        writeInvalid(root, "configured-limits-duplicate-member", duplicateLimit);

        List<String> oversizedRecord = new ArrayList<>(base);
        oversizedRecord.set(0, oversizedRecord.getFirst().replace("\"threadName\":\"fixture-thread\"", "\"threadName\":\"" + "x".repeat(1024 * 1024) + "\""));
        writeInvalid(root, "oversized-physical-record", oversizedRecord);

        List<String> excessiveDepth = new ArrayList<>(base);
        excessiveDepth.set(0, excessiveDepth.getFirst().replace("{\"sessionId\":\"session-single-attempt-success\"}", nestedJson(129)));
        writeInvalid(root, "excessive-json-nesting", excessiveDepth);

        List<String> truncatedFinal = new ArrayList<>(base);
        truncatedFinal.set(truncatedFinal.size() - 1, truncatedFinal.getLast().substring(0, truncatedFinal.getLast().length() - 1));
        writeInvalid(root, "truncated-final-input", truncatedFinal);
    }

    private static void replaceAllLines(List<String> lines, String target, String replacement)
    {
        for (int index = 0; index < lines.size(); index++)
        {
            lines.set(index, lines.get(index).replace(target, replacement));
        }
    }

    private static void replaceFirstLineContaining(List<String> lines, String marker, String target, String replacement)
    {
        for (int index = 0; index < lines.size(); index++)
        {
            if (lines.get(index).contains(marker))
            {
                lines.set(index, lines.get(index).replace(target, replacement));
                return;
            }
        }
        throw new IllegalArgumentException("No fixture line contains '" + marker + "'");
    }

    private static void replaceLastLineContaining(List<String> lines, String marker, String target, String replacement)
    {
        for (int index = lines.size() - 1; index >= 0; index--)
        {
            if (lines.get(index).contains(marker))
            {
                lines.set(index, lines.get(index).replace(target, replacement));
                return;
            }
        }
        throw new IllegalArgumentException("No fixture line contains '" + marker + "'");
    }

    private static String nestedJson(int depth)
    {
        return "{\"value\":".repeat(depth) + "null" + "}".repeat(depth);
    }

    private static void writeInvalid(Path root, String name, List<String> lines) throws Exception
    {
        // Join with explicit LF to match the production NdjsonTraceRecordWriter
        // and the committed fixture corpus, regardless of platform.
        String joined = String.join("\n", lines) + "\n";
        Files.write(
                root.resolve("traces").resolve(name + ".ndjson"),
                joined.getBytes(StandardCharsets.UTF_8));
        Map<String, Object> expected = new LinkedHashMap<>();
        expected.put("case", name);
        expected.put("valid", false);
        expected.put("errorCategory", INVALID.get(name));
        writeExpected(root, name, expected);
    }

    private static void writeExpected(Path root, String name, Map<String, Object> expected) throws Exception
    {
        // Force LF line endings so the generated file matches the committed
        // corpus on every platform. Jackson's DefaultPrettyPrinter uses
        // System.lineSeparator() which is CRLF on Windows.
        String serialized = JSON.writerWithDefaultPrettyPrinter()
                .writeValueAsString(expected)
                .replace("\r\n", "\n")
                .replace("\r", "\n") + "\n";
        byte[] bytes = serialized.getBytes(StandardCharsets.UTF_8);
        try (java.io.OutputStream out = new java.io.FileOutputStream(
                root.resolve("expected").resolve(name + ".json").toFile()))
        {
            out.write(bytes);
        }
    }

    private static List<JsonNode> parseLines(Path path) throws Exception
    {
        List<JsonNode> nodes = new ArrayList<>();
        for (String line : Files.readAllLines(path, StandardCharsets.UTF_8))
        {
            nodes.add(JSON.readTree(line));
        }
        return nodes;
    }

    private static Path fixtureRoot()
    {
        Path cwd = Path.of(System.getProperty("user.dir")).toAbsolutePath();
        Path direct = cwd.resolve("loomspan-console-fixtures");
        if (Files.isDirectory(direct) || Files.isDirectory(cwd.resolve("loomspan-spring-boot-starter")))
        {
            return direct;
        }
        return cwd.getParent().resolve("loomspan-console-fixtures");
    }

    private static List<String> fileNames(Path root) throws IOException
    {
        if (Files.notExists(root))
        {
            return List.of();
        }
        try (Stream<Path> files = Files.walk(root))
        {
            return files.filter(Files::isRegularFile)
                    .filter(path -> !path.getFileName().toString().equals("README.md"))
                    .map(root::relativize)
                    .map(path -> path.toString().replace('\\', '/'))
                    .filter(path -> path.startsWith("traces/") || path.startsWith("expected/"))
                    .sorted()
                    .toList();
        }
    }

    private static Map<String, String> transportFixtures(Path root) throws IOException
    {
        Map<String, String> fixtures = new LinkedHashMap<>();
        try (Stream<Path> files = Files.walk(root))
        {
            for (Path path : files.filter(Files::isRegularFile).sorted().toList())
            {
                String name = root.relativize(path).toString().replace('\\', '/');
                if (!name.startsWith("traces/") && !name.startsWith("expected/") && !"README.md".equals(name))
                {
                    fixtures.put(name, Files.readString(path, StandardCharsets.UTF_8));
                }
            }
        }
        return fixtures;
    }

    private static void copyCorpus(Path source, Path target) throws IOException
    {
        Files.createDirectories(target.resolve("traces"));
        Files.createDirectories(target.resolve("expected"));
        Set<String> sourceFiles = Set.copyOf(fileNames(source));
        for (String name : sourceFiles)
        {
            Path destination = target.resolve(name);
            Files.createDirectories(destination.getParent());
            Files.copy(source.resolve(name), destination, java.nio.file.StandardCopyOption.REPLACE_EXISTING);
        }
        try (Stream<Path> existing = Files.walk(target))
        {
            for (Path path : existing.filter(Files::isRegularFile)
                    .filter(path ->
                    {
                        String name = target.relativize(path).toString().replace('\\', '/');
                        return name.startsWith("traces/") || name.startsWith("expected/");
                    })
                    .sorted(Comparator.reverseOrder())
                    .toList())
            {
                String name = target.relativize(path).toString().replace('\\', '/');
                if (!sourceFiles.contains(name))
                {
                    Files.delete(path);
                }
            }
        }
    }

    private record FrameInterval(long startMillis, long endMillis)
    {
    }

    private record Usage(int promptUnits, int completionUnits, int totalUnits)
    {
        private static final Usage ZERO = new Usage(0, 0);

        private Usage(int promptUnits, int completionUnits)
        {
            this(promptUnits, completionUnits, promptUnits + completionUnits);
        }

        private Map<String, Object> asMap()
        {
            Map<String, Object> result = new LinkedHashMap<>();
            result.put("promptUnits", promptUnits);
            result.put("completionUnits", completionUnits);
            result.put("totalUnits", totalUnits);
            return result;
        }

        private Usage minus(Usage other)
        {
            return new Usage(
                    promptUnits - other.promptUnits,
                    completionUnits - other.completionUnits,
                    totalUnits - other.totalUnits);
        }

        private Usage plus(Usage other)
        {
            return new Usage(
                    promptUnits + other.promptUnits,
                    completionUnits + other.completionUnits,
                    totalUnits + other.totalUnits);
        }
    }
}
