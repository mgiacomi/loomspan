package com.lokiscale.loomspan.internal.observability.web;

import com.lokiscale.loomspan.internal.release.LoomspanReleaseVersion;
import com.lokiscale.loomspan.internal.observability.ObservabilityActivationCoordinator;
import com.lokiscale.loomspan.internal.observability.ObservabilityRuntime;
import com.lokiscale.loomspan.internal.observability.web.dto.ObservabilityDtos;
import com.lokiscale.loomspan.internal.runtime.observation.ActiveExecutionSnapshot;
import com.lokiscale.loomspan.internal.runtime.observation.catalog.FinalizedTraceCatalogEntry;
import com.lokiscale.loomspan.internal.runtime.observation.catalog.RegisteredSkillFile;
import com.lokiscale.loomspan.internal.runtime.observation.catalog.TraceCatalogSlice;
import jakarta.servlet.http.HttpServletRequest;
import jakarta.servlet.http.HttpServletResponse;
import org.springframework.http.MediaType;
import org.springframework.http.ContentDisposition;
import org.springframework.http.ResponseEntity;
import org.springframework.security.core.context.SecurityContextHolder;
import org.springframework.web.bind.annotation.PathVariable;
import org.springframework.web.bind.annotation.ResponseBody;

import java.time.Instant;
import java.io.IOException;
import java.util.Collections;
import java.util.List;
import java.util.Set;

@ResponseBody
public final class ObservabilityRestController
{
    private final ObservabilityActivationCoordinator activation;
    private final ObservabilityAccessService access;
    private final ObservabilityDtoMapper mapper;
    private final ObservabilityCursorCodec cursors;
    private final BoundedJsonPageWriter pages;
    private final ObservabilityJsonCodec json;
    private final String releaseVersion;

    public ObservabilityRestController(
            ObservabilityActivationCoordinator activation,
            ObservabilityAccessService access,
            ObservabilityDtoMapper mapper,
            ObservabilityCursorCodec cursors,
            BoundedJsonPageWriter pages,
            ObservabilityJsonCodec json)
    {
        this.activation = activation;
        this.access = access;
        this.mapper = mapper;
        this.cursors = cursors;
        this.pages = pages;
        this.json = json;
        this.releaseVersion = LoomspanReleaseVersion.load();
    }

    public ResponseEntity<byte[]> instance(HttpServletRequest request)
    {
        require(ObservabilityAccessService.Operation.INSTANCE_READ);
        validateNoQuery(request);
        ObservabilityRuntime runtime = runtime();
        Instant observedAt = Instant.now(runtime.clock());
        return json(pages.writeObject(new ObservabilityDtos.InstanceStatus(
                runtime.instanceId().toString(), releaseVersion, observedAt, runtime.liveMonitoring().isAvailable(),
                runtime.skills().registeredSkillCount(), runtime.activeExecutions().activeCount(),
                runtime.traces().catalogedTraceCount(), runtime.tracePersistencePolicy(),
                runtime.configuration().getCompletionGraceTtl(),
                runtime.configuration().getTraceCatalogMetadataTtl())));
    }

    public ResponseEntity<byte[]> skills(HttpServletRequest request)
    {
        require(ObservabilityAccessService.Operation.SKILL_READ);
        validateQuery(request);
        ObservabilityRuntime runtime = runtime();
        int pageSize = pages.pageSize(single(request, "pageSize"));
        String encoded = single(request, "cursor");
        ObservabilityCursorCodec.Cursor cursor = encoded == null
                ? ObservabilityCursorCodec.Cursor.initial(runtime.instanceId(), "skills", 0)
                : cursors.decode(encoded, runtime.instanceId(), "skills");
        if (encoded != null && (cursor.afterName() == null
                || runtime.skills().find(cursor.afterName()).isEmpty()))
        {
            throw invalidCursor();
        }
        List<RegisteredSkillFile.Summary> source = runtime.skills().listAfter(cursor.afterName(), pageSize + 1);
        List<ObservabilityDtos.SkillSummary> items = source.stream().map(mapper::skill).toList();
        Instant observedAt = Instant.now(runtime.clock());
        byte[] body = pages.write(items, pageSize, emitted ->
        {
            boolean more = source.size() > emitted.size();
            String next = more && !emitted.isEmpty()
                    ? cursors.encode(cursor.after(emitted.getLast().registeredName())) : null;
            return new ObservabilityDtos.Page<>(emitted, more, next, observedAt);
        });
        return json(body);
    }

    public ResponseEntity<byte[]> skill(@PathVariable String registeredName, HttpServletRequest request)
    {
        require(ObservabilityAccessService.Operation.SKILL_READ);
        validateNoQuery(request);
        return json(pages.writeObject(mapper.skill(
                runtime().skills().find(registeredName).orElseThrow(ObservabilityRestController::notFound))));
    }

    public ResponseEntity<byte[]> active(HttpServletRequest request)
    {
        require(ObservabilityAccessService.Operation.ACTIVE_READ);
        validateQuery(request);
        ObservabilityRuntime runtime = runtime();
        requireLive(runtime);
        int pageSize = pages.pageSize(single(request, "pageSize"));
        String encoded = single(request, "cursor");
        boolean initial = encoded == null;
        long highWater = runtime.activeExecutions().highestOrdinal();
        ObservabilityCursorCodec.Cursor cursor = initial
                ? ObservabilityCursorCodec.Cursor.initial(runtime.instanceId(), "active-executions", highWater)
                : cursors.decode(encoded, runtime.instanceId(), "active-executions");
        if (!initial && (cursor.highWater() == 0 || cursor.beforeOrdinal() == 0
                || cursor.highWater() > highWater))
        {
            throw invalidCursor();
        }
        List<ActiveExecutionSnapshot> source = runtime.activeExecutions()
                .newestFirst(cursor.highWater(), cursor.beforeOrdinal(), pageSize + 1);
        Instant observedAt = Instant.now(runtime.clock());
        List<ObservabilityDtos.ActiveExecution> items = source.stream()
                .map(item -> mapper.active(item, observedAt, runtime.quotas())).toList();
        String resume = initial ? Long.toString(runtime.replayBuffer().currentCursor()) : null;
        byte[] body = pages.write(items, pageSize, emitted ->
        {
            boolean more = source.size() > emitted.size();
            String next = more && !emitted.isEmpty()
                    ? cursors.encode(cursor.before(source.get(emitted.size() - 1).registryOrdinal())) : null;
            return new ObservabilityDtos.ActivePage(emitted, more, next, observedAt, resume);
        });
        return json(body);
    }

    public ResponseEntity<byte[]> active(@PathVariable String sessionId, HttpServletRequest request)
    {
        require(ObservabilityAccessService.Operation.ACTIVE_READ);
        validateNoQuery(request);
        ObservabilityRuntime runtime = runtime();
        requireLive(runtime);
        return json(pages.writeObject(mapper.active(runtime.activeExecutions().find(sessionId)
                .orElseThrow(ObservabilityRestController::notFound),
                Instant.now(runtime.clock()), runtime.quotas())));
    }

    public void activity(HttpServletRequest request, HttpServletResponse response) throws IOException
    {
        require(ObservabilityAccessService.Operation.ACTIVITY_SUBSCRIBE);
        requireActivityRequest(request);
        ObservabilityRuntime runtime = runtime();
        requireLive(runtime);
        String instanceValue = single(request, "instanceId");
        String cursorValue = single(request, "afterCursor");
        java.util.UUID instance;
        try
        {
            if (!instanceValue.matches(
                    "[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}"))
            {
                throw invalidRequestShape();
            }
            instance = java.util.UUID.fromString(instanceValue);
        }
        catch (IllegalArgumentException failure)
        {
            throw invalidRequestShape();
        }
        if (!runtime.instanceId().equals(instance))
        {
            throw staleCursor();
        }
        long afterCursor = parseActivityCursor(cursorValue);
        com.lokiscale.loomspan.internal.runtime.observation.ReplayResult replay =
                runtime.replayBuffer().replayAfter(afterCursor, 1);
        if (replay.status() == com.lokiscale.loomspan.internal.runtime.observation.ReplayResult.Status.FUTURE)
        {
            throw invalidCursor();
        }
        if (replay.status() == com.lokiscale.loomspan.internal.runtime.observation.ReplayResult.Status.TOO_OLD)
        {
            throw staleCursor();
        }
        ObservabilityActivityDelivery.Admission admission = runtime.activityDelivery().admit(afterCursor);
        try
        {
            byte[] handshake = ObservabilityActivityStream.handshakeFrame(
                    json,
                    new ObservabilityDtos.ActivityHandshake(
                            runtime.instanceId().toString(),
                            Instant.now(runtime.clock()),
                            Long.toString(afterCursor)));
            ObservabilityActivityStream.open(
                    request, response, runtime.activityDelivery(), admission, handshake);
        }
        catch (IOException | RuntimeException failure)
        {
            admission.close();
            throw failure;
        }
    }

    public ResponseEntity<byte[]> traces(HttpServletRequest request)
    {
        require(ObservabilityAccessService.Operation.TRACE_READ);
        validateQuery(request);
        ObservabilityRuntime runtime = runtime();
        int pageSize = pages.pageSize(single(request, "pageSize"));
        String encoded = single(request, "cursor");
        ObservabilityCursorCodec.Cursor cursor;
        TraceCatalogSlice slice;
        if (encoded == null)
        {
            slice = runtime.traces().list(0, 0, pageSize + 1);
            cursor = ObservabilityCursorCodec.Cursor.initial(runtime.instanceId(), "traces", slice.highWaterOrdinal());
        }
        else
        {
            cursor = cursors.decode(encoded, runtime.instanceId(), "traces");
            long assignedHighWater = runtime.traces().list(0, 0, 1).highWaterOrdinal();
            if (cursor.highWater() == 0 || cursor.beforeOrdinal() == 0
                    || cursor.highWater() > assignedHighWater)
            {
                throw invalidCursor();
            }
            slice = runtime.traces().list(cursor.highWater(), cursor.beforeOrdinal(), pageSize + 1);
        }
        List<FinalizedTraceCatalogEntry> source = slice.entries();
        List<ObservabilityDtos.Trace> items = source.stream().map(mapper::trace).toList();
        Instant observedAt = Instant.now(runtime.clock());
        byte[] body = pages.write(items, pageSize, emitted ->
        {
            boolean more = source.size() > emitted.size();
            String next = more && !emitted.isEmpty()
                    ? cursors.encode(cursor.before(source.get(emitted.size() - 1).catalogOrdinal())) : null;
            return new ObservabilityDtos.Page<>(emitted, more, next, observedAt);
        });
        return json(body);
    }

    public ResponseEntity<byte[]> trace(@PathVariable String traceId, HttpServletRequest request)
    {
        require(ObservabilityAccessService.Operation.TRACE_READ);
        validateNoQuery(request);
        return json(pages.writeObject(mapper.trace(
                runtime().traces().find(traceId).orElseThrow(ObservabilityRestController::notFound))));
    }

    public void artifact(
            @PathVariable String traceId,
            HttpServletRequest request,
            HttpServletResponse response) throws IOException
    {
        require(ObservabilityAccessService.Operation.TRACE_ARTIFACT_READ);
        requireArtifactRequest(request);
        ObservabilityRuntime runtime = runtime();
        runtime.traces().find(traceId).orElseThrow(ObservabilityRestController::notFound);
        ObservabilityArtifactDelivery.Admission admission = runtime.artifactDelivery().admit();
        com.lokiscale.loomspan.internal.runtime.observation.catalog.FinalizedTraceCatalog.ArtifactAcquisition
                acquisition;
        try
        {
            acquisition = runtime.traces().acquire(traceId)
                    .orElseThrow(ObservabilityRestController::notFound);
        }
        catch (IOException | RuntimeException failure)
        {
            admission.close();
            throw failure;
        }
        try
        {
            runtime.artifactDelivery().open(
                    request,
                    response,
                    admission,
                    acquisition.lease(),
                    () -> prepareArtifactResponse(response, acquisition.traceId(), acquisition.sizeBytes()));
        }
        catch (RuntimeException failure)
        {
            if (!response.isCommitted())
            {
                response.reset();
            }
            throw failure;
        }
    }

    public void fallback(HttpServletRequest request)
    {
        requireGet(request);
        throw notFound();
    }

    private void require(ObservabilityAccessService.Operation operation)
    {
        access.require(operation, SecurityContextHolder.getContext().getAuthentication());
    }

    private ObservabilityRuntime runtime()
    {
        return activation.runtime().orElseThrow(() -> new ObservabilityException(
                404, ObservabilityProblem.Code.NOT_FOUND, "The requested observability resource was not found"));
    }

    private static void requireLive(ObservabilityRuntime runtime)
    {
        if (!runtime.liveMonitoring().isAvailable())
        {
            throw new ObservabilityException(
                    503, ObservabilityProblem.Code.LIVE_MONITORING_UNAVAILABLE,
                    "Live execution monitoring is unavailable");
        }
    }

    private static void validateQuery(HttpServletRequest request)
    {
        requireGet(request);
        if (!Set.of("pageSize", "cursor").containsAll(request.getParameterMap().keySet()))
        {
            throw new ObservabilityException(
                    400, ObservabilityProblem.Code.INVALID_REQUEST, "The request contains an unsupported query parameter");
        }
        request.getParameterMap().forEach((name, values) ->
        {
            if (values == null || values.length != 1)
            {
                throw new ObservabilityException(
                        400, ObservabilityProblem.Code.INVALID_REQUEST,
                        "Query parameters must occur exactly once");
            }
        });
    }

    private static void validateNoQuery(HttpServletRequest request)
    {
        requireGet(request);
        if (!request.getParameterMap().isEmpty())
        {
            throw new ObservabilityException(
                    400, ObservabilityProblem.Code.INVALID_REQUEST,
                    "The request contains an unsupported query parameter");
        }
    }

    private static void requireActivityRequest(HttpServletRequest request)
    {
        if (!"GET".equals(request.getMethod())
                || !request.getParameterMap().keySet().equals(Set.of("instanceId", "afterCursor"))
                || request.getHeader("Last-Event-ID") != null)
        {
            throw invalidRequestShape();
        }
        request.getParameterMap().forEach((name, values) ->
        {
            if (values == null || values.length != 1 || values[0] == null || values[0].isBlank())
            {
                throw invalidRequestShape();
            }
        });
        try
        {
            List<MediaType> accepted = MediaType.parseMediaTypes(
                    Collections.list(request.getHeaders("Accept")));
            if (accepted.isEmpty() || accepted.stream().noneMatch(mediaType ->
                    mediaType.getQualityValue() > 0 && mediaType.isCompatibleWith(MediaType.TEXT_EVENT_STREAM)))
            {
                throw invalidRequestShape();
            }
        }
        catch (org.springframework.http.InvalidMediaTypeException failure)
        {
            throw invalidRequestShape();
        }
    }

    private static void requireArtifactRequest(HttpServletRequest request)
    {
        if (!"GET".equals(request.getMethod()) || !request.getParameterMap().isEmpty())
        {
            throw invalidRequestShape();
        }
        for (String header : List.of(
                "Range", "If-Range", "If-Match", "If-None-Match",
                "If-Modified-Since", "If-Unmodified-Since"))
        {
            if (request.getHeader(header) != null)
            {
                throw invalidRequestShape();
            }
        }
        try
        {
            MediaType ndjson = new MediaType("application", "x-ndjson");
            List<MediaType> accepted = MediaType.parseMediaTypes(
                    Collections.list(request.getHeaders("Accept")));
            if (!accepted.isEmpty() && accepted.stream().noneMatch(mediaType ->
                    mediaType.getQualityValue() > 0 && mediaType.isCompatibleWith(ndjson)))
            {
                throw invalidRequestShape();
            }
        }
        catch (org.springframework.http.InvalidMediaTypeException failure)
        {
            throw invalidRequestShape();
        }
    }

    static void prepareArtifactResponse(HttpServletResponse response, String traceId, long sizeBytes)
    {
        response.setStatus(HttpServletResponse.SC_OK);
        response.setContentType("application/x-ndjson");
        response.setCharacterEncoding(java.nio.charset.StandardCharsets.UTF_8.name());
        response.setHeader(
                "Content-Disposition",
                ContentDisposition.attachment()
                        .filename("loomspan-trace-" + traceId + ".ndjson", java.nio.charset.StandardCharsets.UTF_8)
                        .build()
                        .toString());
        response.setContentLengthLong(sizeBytes);
    }

    private static long parseActivityCursor(String value)
    {
        if (value == null || !value.matches("[0-9]+"))
        {
            throw invalidCursor();
        }
        try
        {
            return Long.parseLong(value);
        }
        catch (NumberFormatException failure)
        {
            throw invalidCursor();
        }
    }

    private static void requireGet(HttpServletRequest request)
    {
        if (!"GET".equals(request.getMethod()))
        {
            throw new ObservabilityException(
                    400, ObservabilityProblem.Code.INVALID_REQUEST,
                    "The request method or shape is not supported");
        }
        try
        {
            List<MediaType> accepted = MediaType.parseMediaTypes(
                    Collections.list(request.getHeaders("Accept")));
            if (!accepted.isEmpty() && accepted.stream().noneMatch(mediaType ->
                    mediaType.getQualityValue() > 0 && mediaType.isCompatibleWith(MediaType.APPLICATION_JSON)))
            {
                throw invalidRequestShape();
            }
        }
        catch (org.springframework.http.InvalidMediaTypeException ex)
        {
            throw invalidRequestShape();
        }
    }

    private static String single(HttpServletRequest request, String name)
    {
        String[] values = request.getParameterValues(name);
        return values == null ? null : values[0];
    }

    private static ResponseEntity<byte[]> json(byte[] body)
    {
        return ResponseEntity.ok().contentType(MediaType.APPLICATION_JSON).body(body);
    }

    private static ObservabilityException notFound()
    {
        return new ObservabilityException(
                404, ObservabilityProblem.Code.NOT_FOUND, "The requested observability resource was not found");
    }

    private static ObservabilityException invalidCursor()
    {
        return new ObservabilityException(
                400, ObservabilityProblem.Code.INVALID_CURSOR, "The continuation is invalid");
    }

    private static ObservabilityException staleCursor()
    {
        return new ObservabilityException(
                410, ObservabilityProblem.Code.STALE_CURSOR, "The live activity continuation is no longer available");
    }

    private static ObservabilityException invalidRequestShape()
    {
        return new ObservabilityException(
                400, ObservabilityProblem.Code.INVALID_REQUEST, "The request method or shape is not supported");
    }
}
