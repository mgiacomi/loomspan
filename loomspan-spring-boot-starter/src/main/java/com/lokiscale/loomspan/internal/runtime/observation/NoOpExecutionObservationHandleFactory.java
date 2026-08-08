package com.lokiscale.loomspan.internal.runtime.observation;

public enum NoOpExecutionObservationHandleFactory implements ExecutionObservationHandleFactory
{
    INSTANCE;

    @Override
    public ExecutionObservationHandle create(String sessionId, String entrySkill)
    {
        if (entrySkill == null || entrySkill.isBlank())
        {
            throw new IllegalArgumentException("entrySkill must not be blank");
        }
        return NoOpExecutionObservationHandle.INSTANCE;
    }
}
