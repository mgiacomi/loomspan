package com.lokiscale.loomspan.internal.runtime.observation;

public interface ExecutionObservationHandleFactory
{
    ExecutionObservationHandle create(String sessionId, String entrySkill);
}
