package com.lokiscale.loomspan.internal.springai;

import com.lokiscale.loomspan.internal.provider.ProviderFailureDetails;

final class ProviderCallException extends RuntimeException
{
    private final ProviderFailureDetails details;

    ProviderCallException(String message, ProviderFailureDetails details)
    {
        super(message);
        this.details = details;
    }

    ProviderFailureDetails details() { return details; }
}
