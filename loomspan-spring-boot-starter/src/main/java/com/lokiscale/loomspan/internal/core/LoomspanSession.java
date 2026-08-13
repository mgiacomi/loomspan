package com.lokiscale.loomspan.internal.core;

import com.fasterxml.jackson.annotation.JsonIgnoreProperties;
import com.fasterxml.jackson.annotation.JsonIgnore;
import com.fasterxml.jackson.annotation.JsonInclude;
import com.fasterxml.jackson.annotation.JsonProperty;
import com.lokiscale.loomspan.internal.linter.LinterOutcome;
import com.lokiscale.loomspan.internal.outputschema.OutputSchemaOutcome;
import com.lokiscale.loomspan.internal.runtime.trace.DefaultExecutionTraceHandle;
import com.lokiscale.loomspan.internal.runtime.trace.ExecutionJournalProjector;
import com.lokiscale.loomspan.internal.runtime.observation.ExecutionObservationHandle;
import com.lokiscale.loomspan.internal.runtime.observation.ExecutionObservationHandleFactory;
import com.lokiscale.loomspan.internal.runtime.observation.NoOpExecutionObservationHandleFactory;
import com.lokiscale.loomspan.internal.runtime.observation.ObservationCompletionDisposition;
import com.lokiscale.loomspan.internal.runtime.usage.SessionUsageSnapshot;
import org.springframework.lang.Nullable;
import org.springframework.security.core.Authentication;

import java.io.IOException;
import java.time.Clock;
import java.util.ArrayDeque;
import java.util.Collection;
import java.util.Collections;
import java.util.Deque;
import java.util.HashMap;
import java.util.IdentityHashMap;
import java.util.List;
import java.util.Map;
import java.util.Objects;
import java.util.Optional;
import java.util.Set;
import java.util.LinkedHashSet;
import java.util.UUID;
import java.util.concurrent.locks.ReentrantLock;
import java.util.function.Consumer;
import java.util.function.Supplier;
import tools.jackson.databind.ObjectMapper;
import java.util.function.UnaryOperator;

@JsonIgnoreProperties(ignoreUnknown = true)
public final class LoomspanSession
{
    private static final Clock DEFAULT_CLOCK = Clock.systemUTC();

    private final String sessionId;
    @JsonIgnore
    private final String entrySkill;
    private final int maxDepth;
    @JsonIgnore
    private final Clock clock;
    @JsonIgnore
    private final ReentrantLock lock;
    @JsonIgnore
    private final Deque<ExecutionFrame> frames;
    @JsonIgnore
    private final Map<String, Integer> toolActivityCountByFrameId;
    @JsonIgnore
    private final IdentityHashMap<Throwable, RecordedFailure> failuresByThrowable;
    @JsonIgnore
    private final IdentityHashMap<Throwable, ProviderAttemptLink> providerAttemptsByThrowable;
    @JsonIgnore
    private final Supplier<String> failureIdSupplier;
    @JsonIgnore
    private RuntimeException failureRecordingFailure;
    @JsonIgnore
    private final @Nullable ExecutionTraceHandle executionTraceHandle;
    @JsonIgnore
    private final ExecutionObservationHandle executionObservationHandle;
    @JsonIgnore
    private final ExecutionJournalProjector journalProjector;
    @JsonIgnore
    private ExecutionJournal finalizedExecutionJournal;
    private ExecutionPlan executionPlan;
    private LinterOutcome lastLinterOutcome;
    private OutputSchemaOutcome lastOutputSchemaOutcome;
    private SessionUsageSnapshot sessionUsage;
    private Set<String> successfulDirectSkills;
    @JsonIgnore
    private Authentication authentication;

    public LoomspanSession(int maxDepth, String entrySkill)
    {
        this(UUID.randomUUID().toString(), entrySkill, maxDepth);
    }

    LoomspanSession(String sessionId, String entrySkill, int maxDepth)
    {
        this(sessionId, entrySkill, maxDepth, List.of(), null, null, null, null, null, TracePersistencePolicy.ONERROR, DEFAULT_CLOCK);
    }

    public LoomspanSession(int maxDepth, String entrySkill, @Nullable Authentication authentication)
    {
        this(UUID.randomUUID().toString(), entrySkill, maxDepth, List.of(), null, null, null, null, authentication, TracePersistencePolicy.ONERROR, DEFAULT_CLOCK);
    }

    LoomspanSession(String sessionId, String entrySkill, int maxDepth, @Nullable Authentication authentication)
    {
        this(sessionId, entrySkill, maxDepth, List.of(), null, null, null, null, authentication, TracePersistencePolicy.ONERROR, DEFAULT_CLOCK);
    }

    LoomspanSession(String sessionId,
            String entrySkill,
            int maxDepth,
            @Nullable Authentication authentication,
            TracePersistencePolicy persistencePolicy)
    {
        this(sessionId, entrySkill, maxDepth, authentication, persistencePolicy, DEFAULT_CLOCK);
    }

    LoomspanSession(String sessionId,
            String entrySkill,
            int maxDepth,
            @Nullable Authentication authentication,
            TracePersistencePolicy persistencePolicy,
            Clock clock)
    {
        this(sessionId, entrySkill, maxDepth, List.of(), null, null, null, null, authentication, persistencePolicy, clock);
    }

    LoomspanSession(String sessionId,
            String entrySkill,
            int maxDepth,
            @Nullable Authentication authentication,
            TracePersistencePolicy persistencePolicy,
            Clock clock,
            ExecutionObservationHandleFactory observationHandleFactory)
    {
        this(sessionId, entrySkill, maxDepth, List.of(), null, null, null, null, authentication, persistencePolicy, clock,
                observationHandleFactory,
                DefaultExecutionTraceHandle::new);
    }

    LoomspanSession(String sessionId,
            String entrySkill,
            int maxDepth,
            @Nullable Authentication authentication,
            TracePersistencePolicy persistencePolicy,
            Clock clock,
            ExecutionObservationHandleFactory observationHandleFactory,
            InternalExecutionTraceHandleFactory traceHandleFactory)
    {
        this(sessionId, entrySkill, maxDepth, List.of(), null, null, null, null, authentication, persistencePolicy, clock,
                observationHandleFactory, traceHandleFactory);
    }

    LoomspanSession(
            String sessionId,
            String entrySkill,
            int maxDepth,
            List<ExecutionFrame> frames,
            ExecutionPlan executionPlan,
            @Nullable LinterOutcome lastLinterOutcome,
            @Nullable OutputSchemaOutcome lastOutputSchemaOutcome,
            @Nullable SessionUsageSnapshot sessionUsage,
            @Nullable Authentication authentication,
            TracePersistencePolicy persistencePolicy,
            Clock clock)
    {
        this(sessionId, entrySkill, maxDepth, frames, executionPlan, lastLinterOutcome, lastOutputSchemaOutcome,
                sessionUsage, authentication, persistencePolicy, clock,
                NoOpExecutionObservationHandleFactory.INSTANCE,
                DefaultExecutionTraceHandle::new);
    }

    LoomspanSession(
            String sessionId,
            String entrySkill,
            int maxDepth,
            List<ExecutionFrame> frames,
            ExecutionPlan executionPlan,
            @Nullable LinterOutcome lastLinterOutcome,
            @Nullable OutputSchemaOutcome lastOutputSchemaOutcome,
            @Nullable SessionUsageSnapshot sessionUsage,
            @Nullable Authentication authentication,
            TracePersistencePolicy persistencePolicy,
            Clock clock,
            ExecutionObservationHandleFactory observationHandleFactory)
    {
        this(sessionId, entrySkill, maxDepth, frames, executionPlan, lastLinterOutcome, lastOutputSchemaOutcome,
                sessionUsage, authentication, persistencePolicy, clock, observationHandleFactory,
                DefaultExecutionTraceHandle::new);
    }

    LoomspanSession(
            String sessionId,
            String entrySkill,
            int maxDepth,
            List<ExecutionFrame> frames,
            ExecutionPlan executionPlan,
            @Nullable LinterOutcome lastLinterOutcome,
            @Nullable OutputSchemaOutcome lastOutputSchemaOutcome,
            @Nullable SessionUsageSnapshot sessionUsage,
            @Nullable Authentication authentication,
            TracePersistencePolicy persistencePolicy,
            Clock clock,
            ExecutionObservationHandleFactory observationHandleFactory,
            InternalExecutionTraceHandleFactory traceHandleFactory)
    {
        this(sessionId, entrySkill, maxDepth, frames, executionPlan, lastLinterOutcome, lastOutputSchemaOutcome,
                sessionUsage, authentication, persistencePolicy, clock, observationHandleFactory, traceHandleFactory,
                () -> UUID.randomUUID().toString());
    }

    LoomspanSession(
            String sessionId,
            String entrySkill,
            int maxDepth,
            List<ExecutionFrame> frames,
            ExecutionPlan executionPlan,
            @Nullable LinterOutcome lastLinterOutcome,
            @Nullable OutputSchemaOutcome lastOutputSchemaOutcome,
            @Nullable SessionUsageSnapshot sessionUsage,
            @Nullable Authentication authentication,
            TracePersistencePolicy persistencePolicy,
            Clock clock,
            ExecutionObservationHandleFactory observationHandleFactory,
            InternalExecutionTraceHandleFactory traceHandleFactory,
            Supplier<String> failureIdSupplier)
    {
        this(sessionId, entrySkill, maxDepth, frames, executionPlan, lastLinterOutcome, lastOutputSchemaOutcome,
                sessionUsage, authentication, persistencePolicy, clock, observationHandleFactory, traceHandleFactory,
                failureIdSupplier,
                com.lokiscale.loomspan.internal.serialization.LoomspanJacksonCodecs.defaults().canonicalTrace());
    }

    LoomspanSession(
            String sessionId,
            String entrySkill,
            int maxDepth,
            List<ExecutionFrame> frames,
            ExecutionPlan executionPlan,
            @Nullable LinterOutcome lastLinterOutcome,
            @Nullable OutputSchemaOutcome lastOutputSchemaOutcome,
            @Nullable SessionUsageSnapshot sessionUsage,
            @Nullable Authentication authentication,
            TracePersistencePolicy persistencePolicy,
            Clock clock,
            ExecutionObservationHandleFactory observationHandleFactory,
            InternalExecutionTraceHandleFactory traceHandleFactory,
            Supplier<String> failureIdSupplier,
            ObjectMapper canonicalTraceMapper)
    {
        this.sessionId = requireNonBlank(sessionId, "sessionId");
        this.entrySkill = EntrySkillIdentity.normalize(entrySkill);
        if (maxDepth <= 0)
        {
            throw new IllegalArgumentException("maxDepth must be greater than zero");
        }

        this.maxDepth = maxDepth;
        this.clock = Objects.requireNonNull(clock, "clock must not be null");
        this.lock = new ReentrantLock();
        this.frames = new ArrayDeque<>(frames == null ? List.of() : List.copyOf(frames));
        this.toolActivityCountByFrameId = new HashMap<>();
        this.failuresByThrowable = new IdentityHashMap<>();
        this.providerAttemptsByThrowable = new IdentityHashMap<>();
        this.failureIdSupplier = Objects.requireNonNull(failureIdSupplier, "failureIdSupplier must not be null");
        this.failureRecordingFailure = null;
        // The runtime supports one session lifecycle: a live in-process session with a canonical trace handle.
        this.journalProjector = new ExecutionJournalProjector(
                Objects.requireNonNull(canonicalTraceMapper, "canonicalTraceMapper must not be null"));
        this.executionObservationHandle = Objects.requireNonNull(observationHandleFactory,
                "observationHandleFactory must not be null").create(this.sessionId, this.entrySkill);
        ExecutionTraceHandle traceHandle;
        try
        {
            traceHandle = Objects.requireNonNull(traceHandleFactory, "traceHandleFactory must not be null").create(
                    this.sessionId, this.entrySkill, persistencePolicy, this.clock, this.executionObservationHandle);
        }
        catch (RuntimeException | Error ex)
        {
            closeObservation(ObservationCompletionDisposition.Status.CORE_FINALIZATION_FAILED, null, Optional.empty());
            throw ex;
        }
        this.executionTraceHandle = traceHandle;
        this.finalizedExecutionJournal = null;
        this.executionPlan = executionPlan;
        this.lastLinterOutcome = lastLinterOutcome;
        this.lastOutputSchemaOutcome = lastOutputSchemaOutcome;
        this.sessionUsage = sessionUsage;
        this.successfulDirectSkills = new LinkedHashSet<>();
        this.authentication = authentication;
    }

    public String getSessionId()
    {
        return sessionId;
    }

    /** Records a throwable once for this session and returns its stable failure identity. */
    public String recordFailure(Throwable failure, Map<String, Object> context)
    {
        Objects.requireNonNull(failure, "failure must not be null");
        lock.lock();
        try
        {
            Throwable current = failure;
            ProviderAttemptLink attemptLink = null;
            Set<Throwable> visited = Collections.newSetFromMap(new IdentityHashMap<>());
            for (int depth = 0; current != null && depth < 64 && visited.add(current); depth++, current = current.getCause())
            {
                if (attemptLink == null) attemptLink = providerAttemptsByThrowable.get(current);
                RecordedFailure existing = failuresByThrowable.get(current);
                if (existing != null)
                {
                    failuresByThrowable.put(failure, existing);
                    return existing.failureId();
                }
            }

            String failureId;
            try
            {
                failureId = requireNonBlank(failureIdSupplier.get(), "failureId");
            }
            catch (RuntimeException | Error ignored)
            {
                failureId = UUID.randomUUID().toString();
            }
            Map<String, Object> source = context == null ? Map.of() : context;
            java.util.LinkedHashMap<String, Object> payload = new java.util.LinkedHashMap<>(source);
            Object message = payload.get("message");
            TraceFailureMetadata.addTo(payload, failure,
                    message instanceof String text && !text.isBlank() ? text : "Execution failed");
            payload.put("diagnostics", List.of(BoundedStackTraceCapture.capture(failure)));
            ExecutionFrame origin = frames.peek();
            failuresByThrowable.put(failure, new RecordedFailure(failureId, origin == null ? null : origin.frameId(), attemptLink));
            try
            {
                markTraceErrored();
                java.util.LinkedHashMap<String, Object> metadata = new java.util.LinkedHashMap<>();
                metadata.put("failureId", failureId);
                if (attemptLink != null)
                {
                    metadata.put("attemptId", attemptLink.attemptId());
                    metadata.put("retrySequenceId", attemptLink.retrySequenceId());
                }
                appendTraceRecord(TraceRecordType.ERROR_RECORDED, immutableMetadata(metadata),
                        Collections.unmodifiableMap(new java.util.LinkedHashMap<>(payload)));
            }
            catch (RuntimeException | Error recordingFailure)
            {
                failureRecordingFailure = new FailureRecordingUnavailableException(sessionId, recordingFailure);
            }
            return failureId;
        }
        finally
        {
            lock.unlock();
        }
    }

    public void registerProviderFailure(Throwable failure, Map<String, Object> attempt)
    {
        Objects.requireNonNull(failure, "failure must not be null");
        ProviderAttemptLink link = new ProviderAttemptLink(
                Objects.toString(attempt.get("attemptId")), Objects.toString(attempt.get("retrySequenceId")));
        lock.lock();
        try
        {
            Set<Throwable> visited = Collections.newSetFromMap(new IdentityHashMap<>());
            for (Throwable current = failure; current != null && visited.size() < 64 && visited.add(current); current = current.getCause())
            {
                providerAttemptsByThrowable.putIfAbsent(current, link);
            }
        }
        finally { lock.unlock(); }
    }

    private record ProviderAttemptLink(String attemptId, String retrySequenceId) {}
    private record RecordedFailure(String failureId, @Nullable String originFrameId, @Nullable ProviderAttemptLink attemptLink) {}

    boolean hasFailureRecordingFailure()
    {
        lock.lock();
        try
        {
            return failureRecordingFailure != null;
        }
        finally
        {
            lock.unlock();
        }
    }

    private static final class FailureRecordingUnavailableException extends IllegalStateException
    {
        private FailureRecordingUnavailableException(String sessionId, Throwable cause)
        {
            super("Failed to record canonical execution failure for session '" + sessionId + "'", cause);
        }
    }

    String entrySkill()
    {
        return entrySkill;
    }

    public int getMaxDepth()
    {
        return maxDepth;
    }

    public Optional<ExecutionPlan> getExecutionPlan()
    {
        lock.lock();
        try
        {
            return Optional.ofNullable(executionPlan);
        }
        finally
        {
            lock.unlock();
        }
    }

    public void replaceExecutionPlan(ExecutionPlan plan)
    {
        lock.lock();
        try
        {
            executionPlan = Objects.requireNonNull(plan, "plan must not be null");
        }
        finally
        {
            lock.unlock();
        }
    }

    public void clearExecutionPlan()
    {
        lock.lock();
        try
        {
            executionPlan = null;
        }
        finally
        {
            lock.unlock();
        }
    }

    public Optional<ExecutionPlan> updateExecutionPlan(UnaryOperator<ExecutionPlan> updater)
    {
        Objects.requireNonNull(updater, "updater must not be null");
        lock.lock();
        try
        {
            if (executionPlan == null)
            {
                return Optional.empty();
            }
            executionPlan = Objects.requireNonNull(updater.apply(executionPlan), "updated plan must not be null");
            return Optional.of(executionPlan);
        }
        finally
        {
            lock.unlock();
        }
    }

    public Optional<Authentication> getAuthentication()
    {
        lock.lock();
        try
        {
            return Optional.ofNullable(authentication);
        }
        finally
        {
            lock.unlock();
        }
    }

    public Optional<LinterOutcome> getLastLinterOutcome()
    {
        lock.lock();
        try
        {
            return Optional.ofNullable(lastLinterOutcome);
        }
        finally
        {
            lock.unlock();
        }
    }

    public void setLastLinterOutcome(@Nullable LinterOutcome lastLinterOutcome)
    {
        lock.lock();
        try
        {
            this.lastLinterOutcome = lastLinterOutcome;
        }
        finally
        {
            lock.unlock();
        }
    }

    public Optional<OutputSchemaOutcome> getLastOutputSchemaOutcome()
    {
        lock.lock();
        try
        {
            return Optional.ofNullable(lastOutputSchemaOutcome);
        }
        finally
        {
            lock.unlock();
        }
    }

    public void setLastOutputSchemaOutcome(@Nullable OutputSchemaOutcome lastOutputSchemaOutcome)
    {
        lock.lock();
        try
        {
            this.lastOutputSchemaOutcome = lastOutputSchemaOutcome;
        }
        finally
        {
            lock.unlock();
        }
    }

    public void setAuthentication(@Nullable Authentication authentication)
    {
        lock.lock();
        try
        {
            this.authentication = authentication;
        }
        finally
        {
            lock.unlock();
        }
    }

    public void pushFrame(ExecutionFrame frame)
    {
        Objects.requireNonNull(frame, "frame must not be null");
        lock.lock();
        try
        {
            if (countsTowardMaxDepth(frame) && currentMaxDepthUsage() >= maxDepth)
            {
                throw new LoomspanStackOverflowException(sessionId, maxDepth, frame.route());
            }
            frames.push(frame);
        }
        finally
        {
            lock.unlock();
        }
    }

    public ExecutionFrame popFrame()
    {
        lock.lock();
        try
        {
            if (frames.isEmpty())
            {
                throw new IllegalStateException("Cannot pop execution frame from an empty session stack.");
            }
            ExecutionFrame frame = frames.pop();
            toolActivityCountByFrameId.remove(frame.frameId());
            return frame;
        }
        finally
        {
            lock.unlock();
        }
    }

    public ExecutionFrame peekFrame()
    {
        lock.lock();
        try
        {
            if (frames.isEmpty())
            {
                throw new IllegalStateException("Cannot peek execution frame from an empty session stack.");
            }
            return frames.peek();
        }
        finally
        {
            lock.unlock();
        }
    }

    @JsonProperty("frames")
    public List<ExecutionFrame> getFramesSnapshot()
    {
        lock.lock();
        try
        {
            return List.copyOf(frames);
        }
        finally
        {
            lock.unlock();
        }
    }

    @JsonIgnore
    public List<JournalEntry> getJournalSnapshot()
    {
        return getExecutionJournal().getEntriesSnapshot();
    }

    @JsonProperty("executionTrace")
    public ExecutionTrace getExecutionTrace()
    {
        lock.lock();
        try
        {
            return requireExecutionTraceHandle().snapshot();
        }
        finally
        {
            lock.unlock();
        }
    }

    @JsonIgnore
    public ExecutionJournal getExecutionJournal()
    {
        lock.lock();
        try
        {
            if (finalizedExecutionJournal != null)
            {
                return finalizedExecutionJournal;
            }
            return journalProjector.project(requireExecutionTraceHandle());
        }
        catch (IOException ex)
        {
            throw new IllegalStateException("Failed to project execution journal for session '" + sessionId + "'", ex);
        }
        finally
        {
            lock.unlock();
        }
    }

    @JsonProperty("executionJournal")
    @JsonInclude(JsonInclude.Include.NON_NULL)
    public ExecutionJournal getSerializedExecutionJournal()
    {
        lock.lock();
        try
        {
            return finalizedExecutionJournal != null ? finalizedExecutionJournal : getExecutionJournal();
        }
        finally
        {
            lock.unlock();
        }
    }

    @JsonProperty("executionPlan")
    public ExecutionPlan getExecutionPlanSnapshot()
    {
        lock.lock();
        try
        {
            return executionPlan;
        }
        finally
        {
            lock.unlock();
        }
    }

    @JsonProperty("lastLinterOutcome")
    public LinterOutcome getLastLinterOutcomeSnapshot()
    {
        lock.lock();
        try
        {
            return lastLinterOutcome;
        }
        finally
        {
            lock.unlock();
        }
    }

    @JsonProperty("lastOutputSchemaOutcome")
    public OutputSchemaOutcome getLastOutputSchemaOutcomeSnapshot()
    {
        lock.lock();
        try
        {
            return lastOutputSchemaOutcome;
        }
        finally
        {
            lock.unlock();
        }
    }

    public Optional<SessionUsageSnapshot> getSessionUsage()
    {
        lock.lock();
        try
        {
            return Optional.ofNullable(sessionUsage);
        }
        finally
        {
            lock.unlock();
        }
    }

    public void setSessionUsage(@Nullable SessionUsageSnapshot sessionUsage)
    {
        lock.lock();
        try
        {
            this.sessionUsage = sessionUsage;
        }
        finally
        {
            lock.unlock();
        }
    }

    public Optional<SessionUsageSnapshot> updateSessionUsage(UnaryOperator<SessionUsageSnapshot> updater)
    {
        Objects.requireNonNull(updater, "updater must not be null");
        lock.lock();
        try
        {
            if (sessionUsage == null)
            {
                return Optional.empty();
            }
            sessionUsage = Objects.requireNonNull(updater.apply(sessionUsage), "updated session usage must not be null");
            return Optional.of(sessionUsage);
        }
        finally
        {
            lock.unlock();
        }
    }

    @JsonProperty("sessionUsage")
    public SessionUsageSnapshot getSessionUsageSnapshot()
    {
        lock.lock();
        try
        {
            return sessionUsage;
        }
        finally
        {
            lock.unlock();
        }
    }

    public static LoomspanSession getCurrentSession()
    {
        return LoomspanSessionHolder.requireCurrentSession();
    }

    public Set<String> getSuccessfulDirectSkills()
    {
        lock.lock();
        try
        {
            return Collections.unmodifiableSet(new LinkedHashSet<>(successfulDirectSkills));
        }
        finally
        {
            lock.unlock();
        }
    }

    public void addSuccessfulDirectSkill(String skillName)
    {
        Objects.requireNonNull(skillName, "skillName must not be null");
        lock.lock();
        try
        {
            if (successfulDirectSkills == null)
            {
                successfulDirectSkills = new LinkedHashSet<>();
            }
            successfulDirectSkills.add(skillName);
        }
        finally
        {
            lock.unlock();
        }
    }

    public void clearSuccessfulDirectSkills()
    {
        lock.lock();
        try
        {
            if (successfulDirectSkills == null)
            {
                successfulDirectSkills = new LinkedHashSet<>();
                return;
            }
            successfulDirectSkills.clear();
        }
        finally
        {
            lock.unlock();
        }
    }

    public void replaceSuccessfulDirectSkills(Collection<String> skillNames)
    {
        Objects.requireNonNull(skillNames, "skillNames must not be null");
        lock.lock();
        try
        {
            successfulDirectSkills = new LinkedHashSet<>(skillNames);
        }
        finally
        {
            lock.unlock();
        }
    }

    public void markToolActivityForCurrentFrame()
    {
        lock.lock();
        try
        {
            if (frames.isEmpty())
            {
                return;
            }
            toolActivityCountByFrameId.merge(frames.peek().frameId(), 1, Integer::sum);
        }
        finally
        {
            lock.unlock();
        }
    }

    public int consumeToolActivityCountForCurrentFrame()
    {
        lock.lock();
        try
        {
            if (frames.isEmpty())
            {
                return 0;
            }
            Integer count = toolActivityCountByFrameId.remove(frames.peek().frameId());
            return count == null ? 0 : count;
        }
        finally
        {
            lock.unlock();
        }
    }

    public void markTraceErrored()
    {
        lock.lock();
        try
        {
            if (failureRecordingFailure != null)
            {
                return;
            }
            requireExecutionTraceHandle().markErrored();
        }
        finally
        {
            lock.unlock();
        }
    }

    public void finalizeTrace(TraceCompletion completion)
    {
        Objects.requireNonNull(completion, "completion must not be null");
        lock.lock();
        ExecutionJournal projectedJournal = null;
        IOException projectionFailure = null;
        IOException finalizationFailure = null;
        RuntimeException uncheckedFinalizationFailure = null;
        Error finalizationError = null;
        TraceOutcome observationOutcome = completion.outcome();
        Optional<FinalizedTraceArtifact> finalizedArtifact = Optional.empty();

        try
        {
            ExecutionTraceHandle handle = requireExecutionTraceHandle();
            if (failureRecordingFailure != null)
            {
                throw failureRecordingFailure;
            }
            if (handle.snapshot().completed() && finalizedExecutionJournal != null)
            {
                return;
            }
            try
            {
                projectedJournal = journalProjector.project(handle);
            }
            catch (IOException ex)
            {
                projectionFailure = ex;
            }
            TraceCompletion effectiveCompletion = completion;
            if (projectionFailure != null)
            {
                if (completion.terminalFailureId() == null)
                {
                    String failureId = recordFailure(projectionFailure,
                            Map.of("message", "Execution journal projection failed"));
                    effectiveCompletion = completion.asFailed(failureId);
                }
                observationOutcome = effectiveCompletion.outcome();
            }
            try
            {
                finalizedArtifact = handle.finalizeTrace(effectiveCompletion);
            }
            catch (IOException ex)
            {
                finalizationFailure = ex;
            }
            if (finalizationFailure == null && projectionFailure == null && projectedJournal != null)
            {
                finalizedExecutionJournal = projectedJournal;
            }
        }
        catch (RuntimeException ex)
        {
            uncheckedFinalizationFailure = ex;
        }
        catch (Error ex)
        {
            finalizationError = ex;
        }
        finally
        {
            lock.unlock();
        }
        closeObservation(
                finalizationFailure == null
                        && projectionFailure == null
                        && uncheckedFinalizationFailure == null
                        && finalizationError == null
                        ? ObservationCompletionDisposition.Status.CORE_FINALIZATION_SUCCEEDED
                        : ObservationCompletionDisposition.Status.CORE_FINALIZATION_FAILED,
                observationOutcome,
                finalizedArtifact);
        if (finalizationError != null)
        {
            throw finalizationError;
        }
        if (uncheckedFinalizationFailure != null)
        {
            throw uncheckedFinalizationFailure;
        }
        if (finalizationFailure != null)
        {
            if (projectionFailure != null)
            {
                finalizationFailure.addSuppressed(projectionFailure);
            }
            throw new IllegalStateException("Failed to finalize execution trace for session '" + sessionId + "'", finalizationFailure);
        }
        if (projectionFailure != null)
        {
            throw new IllegalStateException("Failed to finalize execution trace for session '" + sessionId + "'", projectionFailure);
        }
    }

    private void closeObservation(
            ObservationCompletionDisposition.Status status,
            @Nullable TraceOutcome outcome,
            Optional<FinalizedTraceArtifact> finalizedArtifact)
    {
        try
        {
            executionObservationHandle.close(new ObservationCompletionDisposition(
                    status,
                    outcome,
                    java.time.Instant.now(clock),
                    status == ObservationCompletionDisposition.Status.CORE_FINALIZATION_SUCCEEDED
                            ? finalizedArtifact
                            : Optional.empty()));
        }
        catch (RuntimeException ignored)
        {
            // Optional observation must never change canonical finalization behavior.
        }
    }

    public void readTraceRecords(Consumer<TraceRecord> consumer)
    {
        Objects.requireNonNull(consumer, "consumer must not be null");
        lock.lock();
        try
        {
            requireExecutionTraceHandle().readRecords(consumer);
        }
        catch (IOException ex)
        {
            throw new IllegalStateException("Failed to read execution trace for session '" + sessionId + "'", ex);
        }
        finally
        {
            lock.unlock();
        }
    }

    public void appendTraceRecord(TraceRecordType type, Map<String, Object> metadata, Object payload)
    {
        appendTrace(type, immutableMetadata(metadata), payload);
    }

    public void appendTraceRecord(TraceRecordType type, ExecutionFrame frame, Map<String, Object> metadata, Object payload)
    {
        Objects.requireNonNull(frame, "frame must not be null");
        lock.lock();

        try
        {
            if (failureRecordingFailure != null)
            {
                return;
            }
            ExecutionTraceHandle handle = requireExecutionTraceHandle();
            handle.append(type, frame, frame.traceFrameType(), immutableMetadata(metadata), payload);
        }
        catch (IOException ex)
        {
            throw new IllegalStateException("Failed to append execution trace record for session '" + sessionId + "'", ex);
        }
        finally
        {
            lock.unlock();
        }
    }

    private void appendTrace(TraceRecordType type, Map<String, Object> metadata, Object payload)
    {
        lock.lock();
        try
        {
            if (failureRecordingFailure != null)
            {
                return;
            }
            ExecutionTraceHandle handle = requireExecutionTraceHandle();
            ExecutionFrame activeFrame = frames.peek();
            TraceFrameType frameType = activeFrame == null ? null : activeFrame.traceFrameType();
            if (activeFrame == null)
            {
                handle.append(type, immutableMetadata(metadata), payload);
            }
            else
            {
                handle.append(type, activeFrame, frameType, immutableMetadata(metadata), payload);
            }
        }
        catch (IOException ex)
        {
            throw new IllegalStateException("Failed to append execution trace record for session '" + sessionId + "'", ex);
        }
        finally
        {
            lock.unlock();
        }
    }

    private static Map<String, Object> immutableMetadata(Map<String, Object> metadata)
    {
        if (metadata == null || metadata.isEmpty())
        {
            return Map.of();
        }
        java.util.LinkedHashMap<String, Object> copy = new java.util.LinkedHashMap<>();
        metadata.forEach((key, value) -> copy.put(
                Objects.requireNonNull(key, "metadata key must not be null"),
                Objects.requireNonNull(value, "metadata value must not be null")));
        return Collections.unmodifiableMap(copy);
    }

    private static String requireNonBlank(String value, String fieldName)
    {
        Objects.requireNonNull(value, fieldName + " must not be null");
        if (value.isBlank())
        {
            throw new IllegalArgumentException(fieldName + " must not be blank");
        }
        return value;
    }

    private int currentMaxDepthUsage()
    {
        int depth = 0;
        for (ExecutionFrame frame : frames)
        {
            if (countsTowardMaxDepth(frame))
            {
                depth++;
            }
        }
        return depth;
    }

    private static boolean countsTowardMaxDepth(ExecutionFrame frame)
    {
        return frame != null && switch (frame.traceFrameType())
        {
            case MODEL_CALL, PLANNING, TOOL_INVOCATION, STEP_EXECUTION -> false;
            default -> true;
        };
    }

    private ExecutionTraceHandle requireExecutionTraceHandle()
    {
        if (executionTraceHandle == null)
        {
            throw new IllegalStateException("LoomspanSession requires a live execution trace handle");
        }
        return executionTraceHandle;
    }
}
