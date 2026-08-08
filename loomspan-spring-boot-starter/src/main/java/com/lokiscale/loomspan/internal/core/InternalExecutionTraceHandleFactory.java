package com.lokiscale.loomspan.internal.core;

import com.lokiscale.loomspan.internal.runtime.observation.ExecutionObservationHandle;

import java.time.Clock;

@FunctionalInterface
interface InternalExecutionTraceHandleFactory
{
    ExecutionTraceHandle create(
            String sessionId,
            String entrySkill,
            TracePersistencePolicy persistencePolicy,
            Clock clock,
            ExecutionObservationHandle observationHandle);
}
