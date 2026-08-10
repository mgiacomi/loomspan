package com.lokiscale.loomspan.internal.autoconfigure;

/**
 * Marks a connection-construction diagnostic whose message contains only a safe
 * configuration path and non-sensitive explanation.
 */
public final class SafeAiConnectionConfigurationException extends IllegalStateException
{
    public SafeAiConnectionConfigurationException(String message)
    {
        super(message);
    }
}
